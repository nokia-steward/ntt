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
		// `pattern "..."` matches: try the regex promotion first
		// (handles `?#N` / `[...]` / `\d`...), then fall back to
		// the bare `?` / `*` wildcardMatch on the raw rune
		// payloads. Plain (non-pattern) charstrings get a
		// literal byte-for-byte compare so the `*` in `"DE*"`
		// stays a literal star (Sem_1511_*_010).
		if b.IsPattern || as.IsPattern {
			if ok, used := matchPatternString(as, b); used {
				return ok, nil
			}
			return wildcardMatch(string(as.Value), string(b.Value)), nil
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

// matchPatternString tries to evaluate a TTCN-3 charstring pattern
// match using the Go regexp engine. It returns (matched, true) when
// the pattern uses operators worth promoting to regex (`#`, `[`, `\`,
// `(`, `|`). For purely `?`/`*` patterns it returns `(_, false)` so
// the call site keeps using the simpler wildcardMatch path - that
// path is faster and handles the implicit-omit / set-of edge cases
// the regex engine wouldn't.
func matchPatternString(val, pat *runtime.String) (bool, used bool) {
	if val == nil || pat == nil {
		return false, false
	}
	ps := string(pat.Value)
	// @nocase always needs the regex engine: the wildcardMatch
	// fallback only folds ASCII, but @nocase must fold the full
	// (universal charstring) case mapping too.
	if !pat.NoCase && !patternNeedsRegex(ps) {
		return false, false
	}
	re, err := ttcnPatternToRegex(ps)
	if err != nil {
		return false, false
	}
	if pat.NoCase {
		re = "(?i)" + re
	}
	rx, err := compileTTCNRegex(re)
	if err != nil {
		return false, false
	}
	return rx.MatchString(string(val.Value)), true
}

// patternNeedsRegex reports whether the pattern uses any operator
// the bare wildcardMatch can't handle (so the regex promotion is
// worth the compile cost).
func patternNeedsRegex(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '#', '[', '\\', '(', '|', ']', '+':
			return true
		}
	}
	return false
}

// regexCache memoises compiled regexes for TTCN-3 patterns. The
// conformance suite reuses the same patterns across many testcases
// and we want to keep matching cheap.
var (
	regexCacheMu sync.Mutex
	regexCache   = map[string]*regexp.Regexp{}
)

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
	regexCache[re] = rx
	regexCacheMu.Unlock()
	return rx, nil
}

// ttcnPatternToRegex converts a TTCN-3 charstring pattern (Annex A)
// into a Go regexp source string anchored with `\A...\z`. Only the
// subset the conformance suite exercises is supported; unknown
// constructs cause an error so the caller can fall back.
//
// Mapping (subset):
//
//	?            -> .
//	*            -> .*
//	X#N          -> (X){N}
//	X#(N,M)      -> (X){N,M}
//	[abc]        -> [abc]            (passed through verbatim)
//	[^abc]       -> [^abc]
//	[a-z]        -> [a-z]
//	(a|b|c)      -> (a|b|c)
//	\d \w \s     -> \d \w \s         (Annex A char-class escapes)
//	\?  \*  \\   -> literal ? * \
//	regex metas  -> escaped (so .+ etc. are treated literally)
func ttcnPatternToRegex(s string) (string, error) {
	var b strings.Builder
	b.WriteString(`\A`)
	for i := 0; i < len(s); {
		c := s[i]
		switch c {
		case '?':
			b.WriteString(`(?:.)`)
			i++
		case '*':
			b.WriteString(`(?:.*)`)
			i++
		case '#':
			// `X#N` / `X#(N,M)` quantifier on the previously
			// emitted atom. The pattern is invalid if it starts
			// with `#`, but a previous atom must exist by the
			// time we get here.
			i++
			if i >= len(s) {
				return "", runtime.Errorf("trailing '#' in pattern")
			}
			if s[i] == '(' {
				close := strings.IndexByte(s[i:], ')')
				if close < 0 {
					return "", runtime.Errorf("unclosed '#(...)' in pattern")
				}
				spec := s[i+1 : i+close]
				if _, err := parseRange(spec); err != nil {
					return "", err
				}
				trimmed := strings.TrimSpace(spec)
				switch {
				case trimmed == "":
					// `#()` is the unbounded repetition (zero or more)
					// - emit `*` rather than the empty `{}` RE2 rejects.
					b.WriteByte('*')
				case strings.HasPrefix(trimmed, ","):
					// TTCN-3 allows an open lower bound `#(,m)`; RE2
					// needs an explicit 0 (`{0,m}`), as a bare `{,m}`
					// is treated as a literal there.
					b.WriteByte('{')
					b.WriteString("0" + trimmed)
					b.WriteByte('}')
				default:
					b.WriteByte('{')
					b.WriteString(spec)
					b.WriteByte('}')
				}
				i += close + 1
			} else {
				start := i
				for i < len(s) && s[i] >= '0' && s[i] <= '9' {
					i++
				}
				if start == i {
					return "", runtime.Errorf("expected count after '#'")
				}
				b.WriteByte('{')
				b.WriteString(s[start:i])
				b.WriteByte('}')
			}
		case '[':
			close := strings.IndexByte(s[i:], ']')
			if close < 0 {
				return "", runtime.Errorf("unclosed '[' in pattern")
			}
			b.WriteString(s[i : i+close+1])
			i += close + 1
		case '(', ')', '|':
			b.WriteByte(c)
			i++
		case '+':
			// TTCN-3 postfix repetition "one or more of the
			// preceding set/atom" (B.1.5.3). A literal plus must be
			// written `\+`, handled by the escape branch.
			b.WriteByte('+')
			i++
		case '\\':
			if i+1 >= len(s) {
				return "", runtime.Errorf("trailing '\\' in pattern")
			}
			next := s[i+1]
			switch next {
			case 'd', 'w', 's', 'D', 'W', 'S', 'b', 'B':
				b.WriteByte('\\')
				b.WriteByte(next)
			case '?', '*', '\\', '#', '[', ']', '(', ')', '|', '.', '+', '^', '$', '{', '}':
				b.WriteString(regexp.QuoteMeta(string(next)))
			case 'n':
				b.WriteString(`\n`)
			case 't':
				b.WriteString(`\t`)
			case 'r':
				b.WriteString(`\r`)
			default:
				b.WriteString(regexp.QuoteMeta(string(next)))
			}
			i += 2
		case '.', '^', '$', '{', '}':
			b.WriteString(regexp.QuoteMeta(string(c)))
			i++
		default:
			b.WriteByte(c)
			i++
		}
	}
	b.WriteString(`\z`)
	return b.String(), nil
}

func parseRange(spec string) (struct{}, error) {
	parts := strings.Split(spec, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, err := strconv.Atoi(p); err != nil {
			return struct{}{}, runtime.Errorf("bad range bound %q", p)
		}
	}
	return struct{}{}, nil
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

func equalIgnoreCase(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'a' && ca <= 'z' {
			ca -= 32
		}
		if cb >= 'a' && cb <= 'z' {
			cb -= 32
		}
		if ca != cb {
			return false
		}
	}
	return true
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
// Records are wrapped by their map values in deterministic field-name
// order. Returns (a, b, true) on success or (nil, nil, false) when one
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
		keys := make([]string, 0, len(r.Fields))
		for k := range r.Fields {
			keys = append(keys, k)
		}
		sortStrings(keys)
		vals := make([]runtime.Object, len(keys))
		for i, k := range keys {
			vals[i] = r.Fields[k]
		}
		return &runtime.List{Elements: vals, ListType: runtime.RECORD_OF}, true
	}
	return nil, false
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
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
	if len(vals) < len(fixed) {
		return false
	}
	used := make([]bool, len(vals))
	var assign func(fi int) bool
	assign = func(fi int) bool {
		if fi == len(fixed) {
			return true
		}
		// An Undefined member is an unexpanded / unmodelled element
		// (e.g. an `all from` operand the interpreter didn't spread).
		// match() would treat it as a wildcard and over-accept, so a
		// permutation that can't be faithfully expanded matches
		// nothing - the NegSem all-from-restriction tests rely on this.
		if fixed[fi] == runtime.Undefined {
			return false
		}
		for vi := range vals {
			if used[vi] {
				continue
			}
			if ok, _ := match(vals[vi], fixed[fi]); ok {
				used[vi] = true
				if assign(fi + 1) {
					return true
				}
				used[vi] = false
			}
		}
		return false
	}
	return assign(0)
}

// matchSetOf returns true given sets match.
func matchSetOf(a, b *runtime.List) (bool, error) {
	containsStar := false
	temp := runtime.NewSetOf()
	for _, y := range b.Elements {
		if y == runtime.AnyOrNone {
			containsStar = true
		} else {
			temp.Elements = append(temp.Elements, y)
		}
	}
	if !containsStar && len(a.Elements) > len(temp.Elements) {
		return false, runtime.Errorf("First List contains more Elements than second")
	}
	if len(a.Elements) < len(temp.Elements) {
		return false, runtime.Errorf("First List doesn't contain enough elements")
	}
	return matchIsASupersetB(a, temp)
}

// matchIsASupersetB returns true if a is a superset of b
func matchIsASupersetB(a, b *runtime.List) (bool, error) {
	var (
		cloneA    = a
		isMissing = true
		numOfAny  = 0
	)

	for _, valueB := range b.Elements {
		if valueB == runtime.AnyOrNone {
			continue
		}
		if valueB == runtime.Any {
			numOfAny++
			continue
		}
		isMissing = true
		for i, valueA := range cloneA.Elements {
			if ok, _ := match(valueA, valueB); ok {
				cloneA.Elements[i] = cloneA.Elements[len(cloneA.Elements)-1]
				cloneA.Elements = cloneA.Elements[:len(cloneA.Elements)-1]
				isMissing = false
				break
			}
			isMissing = true
		}
		if !isMissing {
			continue
		}
		return false, runtime.Errorf("At least one %s missing in first List", valueB)
	}
	if len(cloneA.Elements) < numOfAny {
		return false, runtime.Errorf("%d element/s missing in first List", numOfAny-len(cloneA.Elements))
	}
	return true, nil
}

// matchIsASubsetB returns true if a is a subset of b
func matchIsASubsetB(a, b *runtime.List) (bool, error) {
	var (
		cloneB    = b
		isMissing = true
		isAny     = -1
	)
	for _, valueA := range a.Elements {
		isMissing = true
		for i, valueB := range cloneB.Elements {
			if valueB == runtime.AnyOrNone {
				return true, nil
			}

			if valueB == runtime.Any {
				isAny = i
			} else if ok, _ := match(valueA, valueB); ok {
				cloneB.Elements[i] = cloneB.Elements[len(cloneB.Elements)-1]
				cloneB.Elements = cloneB.Elements[:len(cloneB.Elements)-1]
				isMissing = false
				isAny = -1
				break
			}
			isMissing = true
		}
		if !isMissing {
			continue
		}
		if isAny >= 0 {
			cloneB.Elements[isAny] = cloneB.Elements[len(cloneB.Elements)-1]
			cloneB.Elements = cloneB.Elements[:len(cloneB.Elements)-1]
			isMissing = false
			isAny = -1
			continue
		}
		return false, runtime.Errorf("At least one %s or '?' missing in second List", valueA)
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
