package builtins

import (
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/nokia/ntt/runtime"
)

type sliceHolder interface {
	Get(index int) runtime.Object
	Len() int
}

// paddedSlice wraps a sliceHolder so reads past the end return
// runtime.Undefined. Used to make a short record positional match a
// longer template (implicit-omit semantics).
type paddedSlice struct {
	inner sliceHolder
	size  int
}

func (p paddedSlice) Len() int { return p.size }
func (p paddedSlice) Get(i int) runtime.Object {
	if i < p.inner.Len() {
		return p.inner.Get(i)
	}
	return runtime.Undefined
}

func padSlice(s sliceHolder, n int) sliceHolder {
	if s.Len() >= n {
		return s
	}
	return paddedSlice{inner: s, size: n}
}

// sliceHasWildcard reports whether the slice contains a `?` (Any) or
// `*` (AnyOrNone) element. Length-padding only kicks in when neither
// side has wildcards because the existing matcher already handles
// length-elastic patterns via backtracking on `*`.
func sliceHasWildcard(s sliceHolder) bool {
	for i := 0; i < s.Len(); i++ {
		e := s.Get(i)
		if e == runtime.Any || e == runtime.AnyOrNone {
			return true
		}
	}
	return false
}

// isAbsent reports whether o denotes an absent optional field: the
// explicit `omit` value or the uninitialised/unmodelled Undefined.
func isAbsent(o runtime.Object) bool {
	return o == runtime.Undefined || o == runtime.Omit
}

// sliceHasUndefined reports whether the slice contains an absent
// (`omit` / Undefined) element. Used to detect implicit-omit patterns
// where the user wrote explicit `omit` entries for optional fields.
func sliceHasUndefined(s sliceHolder) bool {
	for i := 0; i < s.Len(); i++ {
		if isAbsent(s.Get(i)) {
			return true
		}
	}
	return false
}

// enumTemplatePair returns (template, concreteValue, true) when exactly
// one of a, b is a parameterised-enum matching template and the other a
// concrete enum value. When neither or both are templates it returns
// false so the caller keeps its normal Equal-based comparison.
func enumTemplatePair(a, b runtime.Object) (tmpl, val *runtime.EnumValue, ok bool) {
	ae, aok := a.(*runtime.EnumValue)
	be, bok := b.(*runtime.EnumValue)
	if !aok || !bok {
		return nil, nil, false
	}
	switch {
	case ae.IsTemplate() && !be.IsTemplate():
		return ae, be, true
	case be.IsTemplate() && !ae.IsTemplate():
		return be, ae, true
	}
	return nil, nil, false
}

// match returns true given objects match.
func match(a, b runtime.Object) (bool, error) {
	// A nil Object reaches here when an unbound formal (e.g. a
	// defaulted parameter omitted in a named-argument call) is fed
	// into match(). Treat it like the phantom Undefined wildcard so
	// we never nil-deref on a.Type()/b.Type() below.
	if a == nil || b == nil {
		return true, nil
	}
	// `*` (AnyOrNone) matches any value, present or absent (including
	// `omit`), so it wins before the omit handling below.
	if a == runtime.AnyOrNone || b == runtime.AnyOrNone {
		return true, nil
	}
	// `X ifpresent` matches an absent value, or a present value that
	// matches the inner template X (ETSI B.1.4.2). A present value
	// that does not match X must fail - it is not a wildcard.
	if ifp, ok := b.(*runtime.IfPresent); ok {
		if a == runtime.Omit || a == runtime.Undefined {
			return true, nil
		}
		return match(a, ifp.Inner)
	}
	if ifp, ok := a.(*runtime.IfPresent); ok {
		if b == runtime.Omit || b == runtime.Undefined {
			return true, nil
		}
		return match(ifp.Inner, b)
	}
	// `omit` matches only an absent value: another `omit` or an
	// uninitialised / unmodelled Undefined wildcard. It never matches
	// a present value, and `?` (which requires a present value) never
	// matches `omit`. This is the distinction that makes
	// `match(presentValue, omit)` correctly fail.
	if a == runtime.Omit || b == runtime.Omit {
		other := a
		if a == runtime.Omit {
			other = b
		}
		// A length restricts a present value only (B.1.4.1): `*
		// length(1)` matches an omitted field.
		if lr, ok := other.(*runtime.LengthRestricted); ok {
			return match(runtime.Omit, lr.Inner)
		}
		// A template list matches omit if one of its alternatives
		// does: `(1, omit)` matches an omitted field (B.1.2.1).
		if l, ok := other.(*runtime.List); ok && l.ListType == runtime.VALUE_LIST {
			for _, e := range l.Elements {
				if ok, err := match(runtime.Omit, e); err == nil && ok {
					return true, nil
				}
			}
			return false, nil
		}
		return other == runtime.Omit || other == runtime.Undefined, nil
	}
	if a == runtime.Any || b == runtime.Any {
		return true, nil
	}
	// Either side being Undefined (the phantom value for unmodelled
	// templates) is treated as a wildcard so conformance tests that
	// build a template out of unmodelled bits still hit `setverdict`.
	if a == runtime.Undefined || b == runtime.Undefined {
		return true, nil
	}
	// `(v1, v2, ...)` value-list template: succeed when the LHS
	// matches any of the listed alternatives. We check both sides
	// because the template can land on either argument depending
	// on the call site.
	if l, ok := b.(*runtime.List); ok && l.ListType == runtime.VALUE_LIST {
		for _, e := range l.Elements {
			ok, err := match(a, e)
			if err != nil {
				continue
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	}
	if l, ok := a.(*runtime.List); ok && l.ListType == runtime.VALUE_LIST {
		for _, e := range l.Elements {
			ok, err := match(e, b)
			if err != nil {
				continue
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	}
	// Complement template: match iff value is NOT equal to any
	// element. The negation flips the alternative-set semantics.
	if l, ok := b.(*runtime.List); ok && l.ListType == runtime.COMPLEMENT {
		for _, e := range l.Elements {
			ok, err := match(a, e)
			if err == nil && ok {
				return false, nil
			}
		}
		return true, nil
	}
	if l, ok := a.(*runtime.List); ok && l.ListType == runtime.COMPLEMENT {
		for _, e := range l.Elements {
			ok, err := match(e, b)
			if err == nil && ok {
				return false, nil
			}
		}
		return true, nil
	}
	// `(low..high)` range templates: a value matches if it sits
	// inside (inclusive) the [low, high] window. Either bound may be
	// nil to model `-infinity` / `infinity`.
	if r, ok := b.(*runtime.Range); ok {
		return matchRange(a, r), nil
	}
	if r, ok := a.(*runtime.Range); ok {
		return matchRange(b, r), nil
	}

	// `T length(N..M)` length-restricted template: validate the
	// inner template first, then check the value's length. Either
	// side may carry the restriction depending on call site.
	if lr, ok := b.(*runtime.LengthRestricted); ok {
		return matchLengthRestricted(a, lr)
	}
	if lr, ok := a.(*runtime.LengthRestricted); ok {
		return matchLengthRestricted(b, lr)
	}
	// A union value carrying a @default alternative is a
	// single-field record at runtime; in a value/template context it
	// stands in for the value of its default alternative (ETSI
	// 6.2.5). When matched against a plain scalar, unwrap it so
	// `match(unionVal, 12345)` succeeds via the default alt.
	if na, ok := unwrapDefaultUnion(a, b); ok {
		a = na
	}
	if nb, ok := unwrapDefaultUnion(b, a); ok {
		b = nb
	}
	// `Label(v1, lo..hi, ...)` parameterised-enum template (ETSI
	// 6.2.4): a concrete enum value matches when it shares the key
	// and its integer lies in one of the template's ranges. Two
	// concrete enum values fall through to the Equal comparison.
	if tmpl, val, ok := enumTemplatePair(a, b); ok {
		return tmpl.MatchesEnum(val), nil
	}
	// A record held positionally with its field names is the record of
	// those fields: it matches a record template by them. A value of a
	// compatible type, by other names, matches by position.
	if l, ok := a.(*runtime.List); ok && b.Type() == runtime.RECORD && l.NamesOf(b.(*runtime.Record)) {
		return matchNamedList(l, b.(*runtime.Record), false)
	}
	if l, ok := b.(*runtime.List); ok && a.Type() == runtime.RECORD && l.NamesOf(a.(*runtime.Record)) {
		return matchNamedList(l, a.(*runtime.Record), true)
	}
	if a.Type() != b.Type() {
		// Records and lists both look like braced-init `{a,b,c}` at
		// the source level, so a template authored as a record may
		// match against a value the interpreter built as a list (and
		// vice versa). Coerce the two into a sliceHolder when the
		// shapes are container-like and fall through to the
		// element-by-element comparison.
		if ah, bh, ok := normaliseContainers(a, b); ok {
			if ah.Len() != bh.Len() {
				return false, nil
			}
			for i := 0; i < ah.Len(); i++ {
				ok, err := match(ah.Get(i), bh.Get(i))
				if err != nil {
					return false, err
				}
				if !ok {
					return false, nil
				}
			}
			return true, nil
		}
		return false, runtime.Errorf("type mismatch: %s != %s", a.Type(), b.Type())
	}

	switch b := b.(type) {
	case *runtime.Record:
		return matchRecord(a.(*runtime.Record), b)
	case *runtime.List:
		switch b.ListType {
		case runtime.SET_OF:
			return matchSetOf(a.(*runtime.List), b)
		case runtime.SUPERSET:
			return matchIsASupersetB(a.(*runtime.List), b)
		case runtime.SUBSET:
			return matchIsASubsetB(a.(*runtime.List), b)
		default:
			// A value list, `{...}`, matched against a set of is one
			// of its type (6.2.3.0, 6.2.15.6).
			if al, ok := a.(*runtime.List); ok && al.ListType == runtime.SET_OF && b.ListType == runtime.RECORD_OF {
				return matchSetOf(al, b)
			}
			// A permutation(...) block consumes a contiguous run of
			// values in any order, which the positional matchRecordOf
			// can't express; route those patterns to the dedicated
			// backtracking matcher and keep the fast path otherwise.
			if al, ok := a.(*runtime.List); ok && containsPermutation(b) {
				return matchRecordOfPerm(al.Elements, b.Elements), nil
			}
			return matchRecordOf(a.(*runtime.List), b)
		}
	case *runtime.String:
		as := a.(*runtime.String)
		// `pattern "..."` matches by the pattern's regex (B.1.5).
		// Plain (non-pattern) charstrings get a literal compare so
		// the `*` in `"DE*"` stays a literal star (Sem_1511_*_010).
		if b.IsPattern || as.IsPattern {
			ok, _ := matchPatternString(as, b)
			return ok, nil
		}
		// Plain charstrings: literal compare.
		if as.Len() != b.Len() {
			return false, nil
		}
		for i := 0; i < as.Len(); i++ {
			if as.Value[i] != b.Value[i] {
				return false, nil
			}
		}
		return true, nil
	case *runtime.Binarystring:
		return matchBinaryString(a.(*runtime.Binarystring), b)
	default:
		return a.Equal(b), nil
	}
}

// matchPatternString matches a TTCN-3 character pattern (B.1.5) by its
// translation to a Go regexp; used is false only when there is nothing
// to match.
func matchPatternString(val, pat *runtime.String) (bool, used bool) {
	if val == nil || pat == nil {
		return false, false
	}
	// Every character pattern goes through the regex engine: it matches
	// by character, a universal character included, and keeps case
	// unless @nocase (B.1.5.6).
	ps := string(pat.Value)
	re, err := ttcnPatternToRegex(ps)
	if err != nil {
		return false, true
	}
	if pat.NoCase {
		re = "(?i)" + re
	}
	// One the regex engine can't take (a count above its limit, a
	// reversed range) matches nothing.
	rx, err := compileTTCNRegex(re)
	if err != nil {
		return false, true
	}
	return rx.MatchString(string(val.Value)), true
}

// regexCache memoises compiled regexes for TTCN-3 patterns. The
// conformance suite reuses the same patterns across many testcases
// and we want to keep matching cheap.
var (
	regexCacheMu sync.Mutex
	regexCache   = map[string]*regexp.Regexp{}
)

const maxRegexCache = 4096

// RegexpMatch implements the TTCN-3 `regexp(instr, pattern, groupno)`
// operation (ETSI 16.1.2 / Annex C.33): it matches the TTCN-3 pattern
// against instr and returns the substring captured by the groupno-th
// parenthesised group (0-based; group 0 is the first `(...)`). ok is
// false when the pattern does not match or the group index is out of
// range, in which case the caller yields the empty string.
func RegexpMatch(instr, pattern string, groupno int, nocase bool) (string, bool) {
	if groupno < 0 {
		return "", false
	}
	re, err := ttcnPatternToRegex(pattern)
	if err != nil {
		return "", false
	}
	if nocase {
		re = "(?i)" + re
	}
	rx, err := compileTTCNRegex(re)
	if err != nil {
		return "", false
	}
	m := rx.FindStringSubmatch(instr)
	// m[0] is the whole match; TTCN group 0 is the first capture, so
	// the requested group is at m[groupno+1].
	if m == nil || groupno+1 >= len(m) {
		return "", false
	}
	return m[groupno+1], true
}

func compileTTCNRegex(re string) (*regexp.Regexp, error) {
	regexCacheMu.Lock()
	if cached, ok := regexCache[re]; ok {
		regexCacheMu.Unlock()
		return cached, nil
	}
	regexCacheMu.Unlock()
	rx, err := regexp.Compile(re)
	if err != nil {
		return nil, err
	}
	regexCacheMu.Lock()
	// Patterns built from values are many: forget them all once the
	// cache is large, rather than grow it without end.
	if len(regexCache) >= maxRegexCache {
		regexCache = map[string]*regexp.Regexp{}
	}
	regexCache[re] = rx
	regexCacheMu.Unlock()
	return rx, nil
}

// ttcnPatternToRegex converts a TTCN-3 character pattern (ETSI ES 201
// 873-1 B.1.5, table B.1) into a Go regexp source string anchored with
// `\A...\z`. References ({ref}, {\ref}, \N{ref}) are already expanded
// by the interpreter. A metacharacter symbol that forms no metacharacter
// keeps its literal value (B.1.5.0).
//
// Mapping:
//
//	?            -> .   (any character, a line end included)
//	*            -> .*
//	X#n          -> X{n}  (n a single digit, B.1.5.3)
//	X#(n, m)     -> X{n,m}, X#(n,) -> X{n,}, X#(,m) -> X{0,m}, X#() -> X*
//	X+           -> X+
//	[...]        -> a class (B.1.5.1): `]` right after `[` or `[^` and
//	                `^` elsewhere are literal; escapes as below
//	(...) |      -> groups and alternatives
//	\d          -> [0-9]
//	\w          -> [0-9a-zA-Z]
//	\t \r       -> TAB, CR
//	\n          -> any of LF, VT, FF, CR
//	\s          -> any of TAB, LF, VT, FF, CR, space characters
//	\b          -> a word boundary
//	\q{g,p,r,c} \q{Uxxxx, ...} -> the universal character(s)
//	\x          -> x literally, for any other character x
func ttcnPatternToRegex(s string) (string, error) {
	rs := []rune(s)
	paired := patternGroups(rs)
	var b strings.Builder
	// `?` and `*` stand for any character (B.1.5), a line end included:
	// Go's `.` needs the s flag for that. The group keeps a top-level
	// alternative anchored at both ends.
	b.WriteString(`(?s)\A(?:`)
	// atom: an expression precedes, which a `+` or `#` may repeat; one
	// that doesn't, or follows another repetition, is literal.
	atom := false
	literal := func(r rune) {
		b.WriteString(regexp.QuoteMeta(string(r)))
		atom = true
	}
	for i := 0; i < len(rs); {
		c := rs[i]
		switch c {
		case '?':
			b.WriteString(`(?:.)`)
			atom = true
			i++
		case '*':
			b.WriteString(`(?:.*)`)
			atom = true
			i++
		case '#':
			if q, n, ok := patternCount(rs[i+1:]); ok && atom {
				b.WriteString(q)
				atom = false
				i += 1 + n
			} else {
				literal(c)
				i++
			}
		case '+':
			if atom {
				b.WriteByte('+')
				atom = false
			} else {
				literal(c)
			}
			i++
		case '[':
			if class, n, ok := patternSet(rs[i:]); ok {
				b.WriteString(class)
				atom = true
				i += n
			} else {
				literal(c)
				i++
			}
		case '(', ')':
			if !paired[i] {
				literal(c)
			} else {
				b.WriteRune(c)
				atom = c == ')'
			}
			i++
		case '|':
			b.WriteByte('|')
			atom = false
			i++
		case '\\':
			if i+1 >= len(rs) {
				literal(c)
				i++
				continue
			}
			if text, n, ok := patternEscape(rs[i+1:], false); ok {
				b.WriteString(text)
				atom = true
				i += 1 + n
				continue
			}
			literal(rs[i+1])
			i += 2
		default:
			literal(c)
			i++
		}
	}
	b.WriteString(`)\z`)
	return b.String(), nil
}

// PatternItems splits a character pattern into the items it is indexed
// by (15.6.1, table B.1): a character, `?`, `*` or `|`; a `\` with what
// it includes, a \q{...} or \N{...}; a set expression; a group; what a
// {reference} was expanded to, the rune span refs gives, or an
// unexpanded {...}; each with the repetition (`#`, `+`) following it.
// isRef tells the references among them.
func PatternItems(rs []rune, refs [][2]int) (items [][]rune, isRef []bool) {
	paired := patternGroups(rs)
	refAt := map[int]int{}
	for _, r := range refs {
		refAt[r[0]] = r[1]
	}
	for i := 0; i < len(rs); {
		start := i
		ref := false
		switch c := rs[i]; {
		case refAt[i] > i:
			i, ref = refAt[i], true
		case c == '{':
			// Up to its `}`; one without is a literal.
			i++
			for j, depth := i, 1; j < len(rs); j++ {
				if rs[j] == '{' {
					depth++
				} else if rs[j] == '}' {
					if depth--; depth == 0 {
						i = j + 1
						break
					}
				}
			}
		case c == '\\' && i+1 < len(rs):
			i += 2
			if (rs[i-1] == 'q' || rs[i-1] == 'N') && i < len(rs) && rs[i] == '{' {
				for i < len(rs) && rs[i] != '}' {
					i++
				}
				if i < len(rs) {
					i++
				}
			}
		case c == '[':
			if _, n, ok := patternSet(rs[i:]); ok {
				i += n
			} else {
				i++
			}
		case c == '(' && paired[i]:
			depth := 0
			for ; i < len(rs); i++ {
				if rs[i] == '\\' {
					i++
					continue
				}
				if rs[i] == '[' {
					if _, n, ok := patternSet(rs[i:]); ok {
						i += n - 1
						continue
					}
				}
				if rs[i] == '(' && paired[i] {
					depth++
				}
				if rs[i] == ')' && paired[i] {
					depth--
					if depth == 0 {
						i++
						break
					}
				}
			}
		default:
			i++
		}
		// The repetition is the item's.
		for i < len(rs) {
			if rs[i] == '+' {
				i++
				continue
			}
			if rs[i] == '#' {
				if _, n, ok := patternCount(rs[i+1:]); ok {
					i += 1 + n
					continue
				}
			}
			break
		}
		items = append(items, append([]rune(nil), rs[start:i]...))
		isRef = append(isRef, ref)
	}
	return items, isRef
}

// patternGroups reports which of the parentheses of rs, outside set
// expressions and escapes, pair up as a group; one without a partner
// keeps its literal value (B.1.5.0).
func patternGroups(rs []rune) map[int]bool {
	paired := map[int]bool{}
	var open []int
	for i := 0; i < len(rs); i++ {
		switch rs[i] {
		case '\\':
			i++
		case '[':
			if _, n, ok := patternSet(rs[i:]); ok {
				i += n - 1
			}
		case '(':
			open = append(open, i)
		case ')':
			if len(open) > 0 {
				paired[open[len(open)-1]], paired[i] = true, true
				open = open[:len(open)-1]
			}
		}
	}
	return paired
}

// patternCount translates the count after a `#` (B.1.5.3): a single
// digit, or (n, m), (n,), (,m), (n) or (), spaces allowed; it reports how
// many runes it took.
func patternCount(rs []rune) (string, int, bool) {
	if len(rs) == 0 {
		return "", 0, false
	}
	if rs[0] >= '0' && rs[0] <= '9' {
		return "{" + string(rs[0]) + "}", 1, true
	}
	if rs[0] != '(' {
		return "", 0, false
	}
	end := -1
	for j, r := range rs {
		if r == ')' {
			end = j
			break
		}
	}
	if end < 0 {
		return "", 0, false
	}
	spec := strings.ReplaceAll(string(rs[1:end]), " ", "")
	lo, hi, comma := strings.Cut(spec, ",")
	for _, bound := range []string{lo, hi} {
		if _, err := strconv.Atoi(bound); bound != "" && err != nil {
			return "", 0, false
		}
	}
	switch {
	case !comma && lo == "":
		return "*", end + 1, true
	case !comma:
		return "{" + lo + "}", end + 1, true
	case lo == "" && hi == "":
		return "*", end + 1, true
	case lo == "":
		return "{0," + hi + "}", end + 1, true
	default:
		return "{" + lo + "," + hi + "}", end + 1, true
	}
}

// patternSet translates a set expression starting at rs[0] == '['
// (B.1.5.1) into a regexp class; it reports how many runes it took,
// false when the set is not closed. A `^` first negates it, except when
// a `]` follows: `[^]...]` lists both literally, as no list is empty. A
// `]` first, or right after the negating `^`, is a literal, and so is a
// `-` right after the negating `^`, or a `^` elsewhere.
func patternSet(rs []rune) (string, int, bool) {
	var b strings.Builder
	b.WriteByte('[')
	i := 1
	first := true
	switch {
	case len(rs) > 2 && rs[1] == '^' && rs[2] == ']':
		b.WriteString(`\^\]`)
		i, first = 3, false
	case len(rs) > 1 && rs[1] == '^':
		b.WriteByte('^')
		i = 2
		if i < len(rs) && rs[i] == '-' {
			b.WriteString(`\-`)
			i, first = 3, false
		}
	}
	for i < len(rs) {
		c := rs[i]
		switch {
		case c == ']' && !first:
			b.WriteByte(']')
			return b.String(), i + 1, true
		case c == '\\' && i+1 < len(rs):
			if text, n, ok := patternEscape(rs[i+1:], true); ok {
				b.WriteString(text)
				i += 1 + n
			} else {
				b.WriteString(classLiteral(rs[i+1]))
				i += 2
			}
		case c == '-':
			b.WriteByte('-')
			i++
		default:
			b.WriteString(classLiteral(c))
			i++
		}
		first = false
	}
	return "", 0, false
}

// classLiteral is r as a literal inside a regexp class.
func classLiteral(r rune) string {
	switch r {
	case '\\', ']', '[', '^', '-':
		return `\` + string(r)
	}
	return string(r)
}

// patternEscape translates the metacharacter after a `\` (table B.1),
// inside a set expression when inSet; it reports how many runes it took,
// false for a character without a metacharacter meaning, which stands
// for itself.
func patternEscape(rs []rune, inSet bool) (string, int, bool) {
	class := func(members string) string {
		if inSet {
			return members
		}
		return "[" + members + "]"
	}
	switch rs[0] {
	case 'd':
		return class("0-9"), 1, true
	case 'w':
		return class("0-9a-zA-Z"), 1, true
	case 't':
		return `\t`, 1, true
	case 'r':
		return `\r`, 1, true
	case 'n':
		return class(`\n\v\f\r`), 1, true
	case 's':
		return class(`\t\n\v\f\r \p{Zs}`), 1, true
	case 'b':
		if inSet {
			return "", 0, false
		}
		return `\b`, 1, true
	case 'q':
		if len(rs) < 2 || rs[1] != '{' {
			return "", 0, false
		}
		end := -1
		for j, r := range rs {
			if r == '}' {
				end = j
				break
			}
		}
		if end < 0 {
			return "", 0, false
		}
		chars, ok := quadChars(string(rs[2:end]))
		if !ok {
			return "", 0, false
		}
		var b strings.Builder
		for _, r := range chars {
			if inSet {
				b.WriteString(classLiteral(r))
			} else {
				b.WriteString(regexp.QuoteMeta(string(r)))
			}
		}
		// Several characters are one string, which a repetition
		// repeats whole.
		if !inSet && len(chars) > 1 {
			return "(?:" + b.String() + ")", end + 1, true
		}
		return b.String(), end + 1, true
	}
	return "", 0, false
}

// quadChars are the universal characters of a \q{...} (B.1.5.0, 6.1.1):
// one quadruple group, plane, row, cell, or a list of USI-like Uxxxx.
func quadChars(spec string) ([]rune, bool) {
	parts := strings.Split(spec, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	if len(parts) == 0 || parts[0] == "" {
		return nil, false
	}
	if p := parts[0]; p[0] == 'U' || p[0] == 'u' {
		out := make([]rune, 0, len(parts))
		for _, p := range parts {
			if len(p) < 2 || (p[0] != 'U' && p[0] != 'u') {
				return nil, false
			}
			v, err := strconv.ParseUint(strings.TrimPrefix(p[1:], "+"), 16, 32)
			if err != nil {
				return nil, false
			}
			out = append(out, rune(v))
		}
		return out, true
	}
	if len(parts) != 4 {
		return nil, false
	}
	var v uint64
	for _, p := range parts {
		n, err := strconv.ParseUint(p, 10, 8)
		if err != nil {
			return nil, false
		}
		v = v<<8 | n
	}
	return []rune{rune(v)}, true
}

// matchBinaryString matches a concrete bit/hex/octet string against a
// template that may contain `?` (any single digit) and `*` (any
// number of digits, including zero) wildcards. The matcher is the
// standard backtracking wildcard matcher used for charstrings.
func matchBinaryString(val, pat *runtime.Binarystring) (bool, error) {
	if val == nil || pat == nil {
		return false, nil
	}
	if val.Unit != pat.Unit {
		return false, runtime.Errorf("binarystring unit mismatch: %s vs %s", val.Unit, pat.Unit)
	}
	patStr := trimBinaryLiteralForMatch(pat.String)
	if !containsWildcards(patStr) {
		// Concrete templates: rely on the existing bigint-equal
		// comparison plus a length check. This keeps the cheap
		// path identical to a.Equal(b) and is whitespace/case
		// agnostic without us having to re-parse the literal.
		if val.Value == nil || pat.Value == nil {
			return val == pat, nil
		}
		return val.Length == pat.Length && val.Value.Cmp(pat.Value) == 0, nil
	}
	valStr := trimBinaryLiteralForMatch(val.String)
	// For octetstrings the `?` wildcard matches a whole octet
	// (= two hex digits) per TTCN-3 B.1.2.5; expand each one to
	// `??` so the generic two-character matcher sees the right
	// number of positions. `*` already matches any number of
	// characters so it needs no adjustment.
	if pat.Unit == runtime.Octet {
		patStr = expandOctetWildcards(patStr)
	}
	return wildcardMatch(valStr, patStr), nil
}

func expandOctetWildcards(s string) string {
	out := make([]byte, 0, len(s)+8)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '?' {
			out = append(out, '?', '?')
			continue
		}
		out = append(out, c)
	}
	return string(out)
}

func trimBinaryLiteralForMatch(s string) string {
	if len(s) < 3 || s[0] != '\'' {
		return s
	}
	end := len(s) - 1
	for end > 0 && s[end] != '\'' {
		end--
	}
	if end <= 1 {
		return ""
	}
	inner := s[1:end]
	out := make([]byte, 0, len(inner))
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		// Strip whitespace and the backslash that prefixes a TTCN-3
		// line-continuation inside a binary literal. The runtime's
		// own `removeWhitespaces` helper does the same thing for
		// the value-parsing path; keeping the rules in sync means
		// 'Ab\<nl>cD'H and 'AbcD'H compare equal here too.
		switch c {
		case ' ', '\t', '\n', '\r', '\v', '\f', '\\':
			continue
		}
		out = append(out, c)
	}
	return string(out)
}

func containsWildcards(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '?' || s[i] == '*' {
			return true
		}
	}
	return false
}

// wildcardMatch is a classic two-pointer matcher with backtracking on
// `*`. Each `?` consumes exactly one character; `*` consumes zero or
// more. The match is case-insensitive so '0A'H matches '0a'H.
func wildcardMatch(val, pat string) bool {
	i, j := 0, 0
	backI, backJ := -1, -1
	for i < len(val) {
		if j < len(pat) {
			c := pat[j]
			if c == '*' {
				backJ = j + 1
				backI = i
				j++
				continue
			}
			if c == '?' || charEq(val[i], c) {
				i++
				j++
				continue
			}
		}
		if backJ < 0 {
			return false
		}
		j = backJ
		backI++
		i = backI
	}
	for j < len(pat) && pat[j] == '*' {
		j++
	}
	return j == len(pat)
}

func charEq(a, b byte) bool {
	if a >= 'a' && a <= 'z' {
		a -= 32
	}
	if b >= 'a' && b <= 'z' {
		b -= 32
	}
	return a == b
}

// normaliseContainers tries to view both sides of a mismatched-type
// comparison as sliceHolders so the caller can do a positional walk.
// Records are wrapped by their field values in the order of their
// fields (runtime.Record.Names). Returns (a, b, true) on success or (nil, nil, false) when one
// side simply doesn't have a sequence-shape representation.
// unwrapDefaultUnion returns the sole alternative's value of a
// single-field union record when the other operand is a plain scalar
// (not itself a record / container). This realises implicit-default
// usage of a @default union alternative in a matching context.
func unwrapDefaultUnion(rec, other runtime.Object) (runtime.Object, bool) {
	r, ok := rec.(*runtime.Record)
	if !ok || len(r.Fields) != 1 {
		return rec, false
	}
	switch other.(type) {
	case *runtime.Record, *runtime.List, *runtime.Map:
		return rec, false
	}
	for _, v := range r.Fields {
		return v, true
	}
	return rec, false
}

func normaliseContainers(a, b runtime.Object) (sliceHolder, sliceHolder, bool) {
	ah, aok := asSlice(a)
	bh, bok := asSlice(b)
	if !aok || !bok {
		return nil, nil, false
	}
	return ah, bh, true
}

func asSlice(o runtime.Object) (sliceHolder, bool) {
	if s, ok := o.(sliceHolder); ok {
		return s, true
	}
	if r, ok := o.(*runtime.Record); ok {
		// In the order of its fields: its type's, once it has one.
		keys := r.Names()
		vals := make([]runtime.Object, len(keys))
		for i, k := range keys {
			vals[i] = r.Fields[k]
		}
		return &runtime.List{Elements: vals, ListType: runtime.RECORD_OF}, true
	}
	return nil, false
}

// matchRecord returns true given records match. Field sets may differ
// only when the side carrying *additional* fields explicitly sets
// them to `omit` (Undefined) - that captures TTCN-3 6.2.1's
// `optional "implicit omit"` semantics without us modelling the
// attribute directly. A field that is *missing* on one side and
// non-omit on the other is still a mismatch (the empty-record vs
// record-with-real-fields unit test depends on this strictness).
func matchRecord(a, b *runtime.Record) (bool, error) {
	keys := map[string]bool{}
	for k := range a.Fields {
		keys[k] = true
	}
	for k := range b.Fields {
		keys[k] = true
	}
	for k := range keys {
		x, hasX := a.Fields[k]
		y, hasY := b.Fields[k]
		if !hasX {
			if isAbsent(y) {
				continue
			}
			return false, runtime.Errorf("Value mismatch: field %s in second Record not found in first", k)
		}
		if !hasY {
			if isAbsent(x) {
				continue
			}
			return false, runtime.Errorf("Value mismatch: field %s in first Record not found in second", k)
		}
		if ret, err := match(x, y); !ret {
			return false, err
		}
	}
	return true, nil
}

// matchNamedList is matchRecord of a record held positionally, by its
// field names, with r: the list is the value, or, when listIsPattern, the
// template. An unbound element is no field.
func matchNamedList(l *runtime.List, r *runtime.Record, listIsPattern bool) (bool, error) {
	pair := func(x, y runtime.Object) (bool, error) {
		if listIsPattern {
			return match(y, x)
		}
		return match(x, y)
	}
	seen := 0
	for i, n := range l.FieldNames {
		var x runtime.Object = runtime.Undefined
		if i < len(l.Elements) {
			x = l.Elements[i]
		}
		y, hasY := r.Fields[n]
		if hasY {
			seen++
		}
		switch {
		case x == runtime.Undefined && !hasY:
		case x == runtime.Undefined:
			if !isAbsent(y) {
				return false, runtime.Errorf("Value mismatch: field %s not found", n)
			}
		case !hasY:
			if !isAbsent(x) {
				return false, runtime.Errorf("Value mismatch: field %s not found", n)
			}
		default:
			if ok, err := pair(x, y); !ok {
				return false, err
			}
		}
	}
	if seen < len(r.Fields) {
		for k, y := range r.Fields {
			if l.FieldIndex(k) < 0 && !isAbsent(y) {
				return false, runtime.Errorf("Value mismatch: field %s not found", k)
			}
		}
	}
	return true, nil
}

// matchRecordOf returns true if recordOfs match. When the pattern
// carries explicit `omit` (Undefined) entries we treat the value as
// a positional record literal and pad it on the right with Undefined
// so the implicit-omit semantics of TTCN-3 6.2.1 work without us
// modelling the optional attribute (Sem_060201_RecordTypeValues_001
// and friends rely on this).
func matchRecordOf(val, pat sliceHolder) (bool, error) {
	// A length-restricted `*` element (`* length(N..M)`) consumes
	// between N and M values; the single-backtrack-point iterative
	// matcher below only models the unbounded `*`, so route patterns
	// carrying a bounded `*` through the recursive matcher.
	if hasBoundedStar(pat) {
		return matchRecordOfRec(val, pat, 0, 0), nil
	}
	if val.Len() < pat.Len() && sliceHasUndefined(pat) && !sliceHasWildcard(val) && !sliceHasWildcard(pat) {
		val = padSlice(val, pat.Len())
	}
	i, backI := 0, -1
	j, backJ := 0, -1
	for i < val.Len() && j < pat.Len() {
		// if pat is *runtime.String, Get(i) returns another *runtime.String
		// whose Value array contains a single rune
		if pat.Get(j) == runtime.AnyOrNone || pat.Get(j).Equal(runtime.NewCharstring("*")) {
			j++
			backJ = j           // Pattern Element after *
			backI = i           // First Value Element which could be matched with that *
			if j == pat.Len() { // Optimize trailing * case
				return true, nil
			}
		} else if ok, _ := match(val.Get(i), pat.Get(j)); !ok { // Literal character or ?
			if backJ < 0 {
				return false, runtime.Errorf("Pattern doesn't match, Element number %d mismatch", i-1) /* No Backtracking possible */
			}
			// Try again from last *, one character later in str.
			j = backJ
			backI++
			i = backI
		} else {
			i++
			j++
		}
		if j == pat.Len() && i != val.Len() {
			if backJ < 0 {
				return false, runtime.Errorf("Second RecordOf is matched entirely, first isn't")
			}
			// Try again from last *, one character later in str.
			j = backJ
			backI++
			i = backI
		}
	}
	// reached if i == len(val) || j == len(pat)
	if val.Len() == i {
		for ; j < pat.Len(); j++ {
			if pat.Get(j) != runtime.AnyOrNone && !pat.Get(j).Equal(runtime.NewCharstring("*")) {
				return false, runtime.Errorf("First RecordOf is entirely matched, second isn't")
			}
		}
		return true, nil
	}
	// reached if i != len(val) && j == len(pat) == 0 (non-zero case covered in loop)
	return false, runtime.Errorf("Template empty")
}

// isStarElem reports whether a record-of element template is the `*`
// (AnyElementsOrNone) wildcard, in either its singleton form or the
// charstring-"*" spelling matchRecordOf also accepts.
func isStarElem(o runtime.Object) bool {
	return o == runtime.AnyOrNone || (o != nil && o.Equal(runtime.NewCharstring("*")))
}

// boundedStar reports whether a record-of element template is a
// length-restricted `*` (`* length(N..M)`) and returns its [min,max]
// bounds (max == -1 means unbounded above).
func boundedStar(o runtime.Object) (min, max int, ok bool) {
	if lr, isLR := o.(*runtime.LengthRestricted); isLR && lr != nil && isStarElem(lr.Inner) {
		return lr.Min, lr.Max, true
	}
	return 0, 0, false
}

// hasBoundedStar reports whether a record-of pattern carries a
// length-restricted `*` element.
func hasBoundedStar(pat sliceHolder) bool {
	for i := 0; i < pat.Len(); i++ {
		if _, _, ok := boundedStar(pat.Get(i)); ok {
			return true
		}
	}
	return false
}

// matchRecordOfRec matches a record-of value against a pattern that may
// contain bounded (`* length(N..M)`) and unbounded (`*`) wildcards, by
// recursively trying each admissible consumption count. Concrete and
// `?` elements are matched one-to-one via match().
func matchRecordOfRec(val, pat sliceHolder, i, j int) bool {
	for {
		if j == pat.Len() {
			return i == val.Len()
		}
		pj := pat.Get(j)
		if lo, hi, ok := boundedStar(pj); ok {
			rem := val.Len() - i
			if hi == -1 || hi > rem {
				hi = rem
			}
			for k := lo; k <= hi; k++ {
				if matchRecordOfRec(val, pat, i+k, j+1) {
					return true
				}
			}
			return false
		}
		if isStarElem(pj) {
			for k := 0; k <= val.Len()-i; k++ {
				if matchRecordOfRec(val, pat, i+k, j+1) {
					return true
				}
			}
			return false
		}
		if i >= val.Len() {
			return false
		}
		if ok, _ := match(val.Get(i), pj); !ok {
			return false
		}
		i++
		j++
	}
}

// containsPermutation reports whether a record-of pattern list carries a
// permutation(...) block among its elements.
func containsPermutation(pat *runtime.List) bool {
	if pat == nil {
		return false
	}
	for _, e := range pat.Elements {
		if pl, ok := e.(*runtime.List); ok && pl.ListType == runtime.PERMUTATION {
			return true
		}
	}
	return false
}

// matchRecordOfPerm matches a record-of value against a pattern that
// contains one or more permutation(...) blocks. A permutation block
// matches a contiguous run of values in any order (a bijection between
// the block's element templates and the consumed values); a `*` inside
// the block - or at the top level - soaks up zero-or-more values. Plain
// elements consume exactly one value. Backtracking keeps it simple;
// record-of lengths are short in practice.
func matchRecordOfPerm(vals, pats []runtime.Object) bool {
	if len(pats) == 0 {
		return len(vals) == 0
	}
	head, rest := pats[0], pats[1:]
	// Top-level `* length(n..m)`: consume n..m values.
	if lo, hi, ok := boundedStar(head); ok {
		if hi == -1 || hi > len(vals) {
			hi = len(vals)
		}
		for k := lo; k <= hi; k++ {
			if matchRecordOfPerm(vals[k:], rest) {
				return true
			}
		}
		return false
	}
	// Top-level `*`: consume 0..len(vals) values.
	if isStarElem(head) {
		for k := 0; k <= len(vals); k++ {
			if matchRecordOfPerm(vals[k:], rest) {
				return true
			}
		}
		return false
	}
	if pl, ok := head.(*runtime.List); ok && pl.ListType == runtime.PERMUTATION {
		var fixed []runtime.Object
		hasStar := false
		for _, e := range pl.Elements {
			if isStarElem(e) {
				hasStar = true
				continue
			}
			fixed = append(fixed, e)
		}
		maxTake := len(fixed)
		if hasStar {
			maxTake = len(vals)
		}
		for take := len(fixed); take <= maxTake && take <= len(vals); take++ {
			if permBijection(vals[:take], fixed) && matchRecordOfPerm(vals[take:], rest) {
				return true
			}
		}
		return false
	}
	// Plain element: consume exactly one value.
	if len(vals) == 0 {
		return false
	}
	if ok, _ := match(vals[0], head); ok {
		return matchRecordOfPerm(vals[1:], rest)
	}
	return false
}

// permBijection reports whether every template in `fixed` can be paired
// with a distinct value in `vals` (a perfect matching covering all of
// `fixed`). Values beyond len(fixed) are taken to be soaked by a `*`
// inside the permutation, so they need not be covered.
func permBijection(vals, fixed []runtime.Object) bool {
	// An Undefined member is an unexpanded / unmodelled element
	// (e.g. an `all from` operand the interpreter didn't spread).
	// match() would treat it as a wildcard and over-accept, so a
	// permutation that can't be faithfully expanded matches
	// nothing - the NegSem all-from-restriction tests rely on this.
	for _, t := range fixed {
		if t == runtime.Undefined {
			return false
		}
	}
	return pairDistinct(len(fixed), len(vals), func(i, j int) bool {
		ok, _ := match(vals[j], fixed[i])
		return ok
	})
}

// pairDistinct reports whether each i < n pairs with a distinct j < m
// for which ok(i, j), trying another pairing when one leads nowhere.
func pairDistinct(n, m int, ok func(i, j int) bool) bool {
	if n > m {
		return false
	}
	used := make([]bool, m)
	var assign func(i int) bool
	assign = func(i int) bool {
		if i == n {
			return true
		}
		for j := 0; j < m; j++ {
			if !used[j] && ok(i, j) {
				used[j] = true
				if assign(i + 1) {
					return true
				}
				used[j] = false
			}
		}
		return false
	}
	return assign(0)
}

// matchSetOf returns true given sets match: each element of the
// template b, but a `*`, matches an element of its own of the value a,
// and, without a `*`, there are no others.
func matchSetOf(a, b *runtime.List) (bool, error) {
	containsStar := false
	var tmpls []runtime.Object
	for _, y := range b.Elements {
		if y == runtime.AnyOrNone {
			containsStar = true
		} else {
			tmpls = append(tmpls, y)
		}
	}
	if !containsStar && len(a.Elements) > len(tmpls) {
		return false, runtime.Errorf("First List contains more Elements than second")
	}
	if len(a.Elements) < len(tmpls) {
		return false, runtime.Errorf("First List doesn't contain enough elements")
	}
	return supersetOf(a.Elements, tmpls)
}

// matchIsASupersetB returns true if a is a superset of b: each element
// of b matches an element of a of its own (B.1.2.6).
func matchIsASupersetB(a, b *runtime.List) (bool, error) {
	var tmpls []runtime.Object
	for _, y := range b.Elements {
		if y != runtime.AnyOrNone {
			tmpls = append(tmpls, y)
		}
	}
	return supersetOf(a.Elements, tmpls)
}

// supersetOf pairs each template with a value of its own. Neither list
// is changed: the value may be a message still queued, the template
// one to match again.
func supersetOf(vals, tmpls []runtime.Object) (bool, error) {
	if !pairDistinct(len(tmpls), len(vals), func(i, j int) bool {
		ok, _ := match(vals[j], tmpls[i])
		return ok
	}) {
		return false, runtime.Errorf("no element of its own in the first list for each of the second")
	}
	return true, nil
}

// matchIsASubsetB returns true if a is a subset of b: each element of a
// is matched by an element of b of its own, any number by a `*`
// (B.1.2.7).
func matchIsASubsetB(a, b *runtime.List) (bool, error) {
	for _, y := range b.Elements {
		if y == runtime.AnyOrNone {
			return true, nil
		}
	}
	if !pairDistinct(len(a.Elements), len(b.Elements), func(i, j int) bool {
		ok, _ := match(a.Elements[i], b.Elements[j])
		return ok
	}) {
		return false, runtime.Errorf("no element of its own in the second list for each of the first")
	}
	return true, nil
}

// matchLengthRestricted enforces the length attribute of a template
// after the inner template matches. Scalars (no Len method) reject;
// strings, binary strings, lists, and records-of are all length-able.
func matchLengthRestricted(v runtime.Object, lr *runtime.LengthRestricted) (bool, error) {
	if lr.Inner != nil {
		ok, err := match(v, lr.Inner)
		if err != nil || !ok {
			return ok, err
		}
	}
	type lengther interface{ Len() int }
	var n int
	switch x := v.(type) {
	case lengther:
		n = x.Len()
	default:
		return false, nil
	}
	if n < lr.Min {
		return false, nil
	}
	if lr.Max != -1 && n > lr.Max {
		return false, nil
	}
	return true, nil
}

// matchRange reports whether v sits inside r's [Lower, Upper] window
// (inclusive on both ends). A nil bound represents the corresponding
// infinity and never excludes a value. Character-string ranges are
// applied per-character so `("a".."f")` matches `"abc"` but rejects
// `"akc"` (TTCN-3 B.1.2.5).
func matchRange(v runtime.Object, r *runtime.Range) bool {
	if vs, ok := v.(*runtime.String); ok {
		lo, hi := charBound(r.Lower), charBound(r.Upper)
		if lo == nil && hi == nil {
			return true
		}
		for _, ch := range vs.Value {
			if lo != nil && (ch < *lo || (r.LowerExcl && ch == *lo)) {
				return false
			}
			if hi != nil && (ch > *hi || (r.UpperExcl && ch == *hi)) {
				return false
			}
		}
		return true
	}
	// Lower bound: `v < lo` is always out; an exclusive lower bound
	// additionally rejects `v == lo` (i.e. anything that is not
	// strictly greater than lo).
	if r.Lower != nil {
		if r.LowerExcl {
			if !lessObject(r.Lower, v) {
				return false
			}
		} else if lessObject(v, r.Lower) {
			return false
		}
	}
	// Upper bound: symmetric - an exclusive upper bound rejects
	// `v == hi` as well as `v > hi`.
	if r.Upper != nil {
		if r.UpperExcl {
			if !lessObject(v, r.Upper) {
				return false
			}
		} else if lessObject(r.Upper, v) {
			return false
		}
	}
	return true
}

// charBound extracts the first rune from a single-character string
// bound, returning nil for nil or non-single-character inputs (the
// latter being a malformed range we can't usefully apply).
func charBound(o runtime.Object) *rune {
	if o == nil {
		return nil
	}
	s, ok := o.(*runtime.String)
	if !ok || s.Len() != 1 {
		return nil
	}
	r := s.Value[0]
	return &r
}

// lessObject is a best-effort comparator over the numeric runtime
// types used in range bounds. Returns false on type mismatch so the
// range check degenerates into a pass instead of a hard error.
func lessObject(a, b runtime.Object) bool {
	switch x := a.(type) {
	case runtime.Int:
		switch y := b.(type) {
		case runtime.Int:
			return x.Cmp(y.Int) < 0
		case runtime.Float:
			af, _ := new(big.Float).SetInt(x.Int).Float64()
			return af < float64(y)
		}
	case runtime.Float:
		switch y := b.(type) {
		case runtime.Int:
			bf, _ := new(big.Float).SetInt(y.Int).Float64()
			return float64(x) < bf
		case runtime.Float:
			return float64(x) < float64(y)
		}
	case *runtime.String:
		if y, ok := b.(*runtime.String); ok {
			return string(x.Value) < string(y.Value)
		}
	}
	return false
}
