package runtime

import (
	"bytes"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/nokia/ntt/ttcn3/syntax"
)

type Object interface {
	// Inspect returns a string-representation of the object for in TTCN-3 syntax.
	Inspect() string

	Type() ObjectType
	Equal(Object) bool
}

type ObjectType string

const (
	UNKNOWN      ObjectType = "unknown object"
	UNDEFINED    ObjectType = "undefined value"
	OMIT         ObjectType = "omit"
	ERROR        ObjectType = "runtime error"
	BREAK        ObjectType = "break event"
	CONTINUE     ObjectType = "continue event"
	REPEAT       ObjectType = "repeat event"
	GOTO_EVENT   ObjectType = "goto event"
	RETURN_VALUE ObjectType = "return value"
	RAISED_VALUE ObjectType = "raised exception"
	INTEGER      ObjectType = "integer"
	FLOAT        ObjectType = "float"
	BOOL         ObjectType = "boolean"

	// Charstring types
	CHARSTRING ObjectType = "string"

	// Binarystring types
	BITSTRING   ObjectType = "bitstring"
	HEXSTRING   ObjectType = "hexstring"
	OCTETSTRING ObjectType = "octetstring"

	FUNCTION          ObjectType = "function"
	LIST              ObjectType = "list"
	RECORD            ObjectType = "record"
	MAP               ObjectType = "map"
	BUILTIN_OBJ       ObjectType = "builtin function"
	VERDICT           ObjectType = "verdict"
	ENUM_VALUE        ObjectType = "enumerated value"
	ENUM_TYPE         ObjectType = "enumerated type"
	ANY               ObjectType = "?"
	ANY_OR_NONE       ObjectType = "*"
	IF_PRESENT        ObjectType = "ifpresent"
	RANGE             ObjectType = "range"
	TYPE_DESC         ObjectType = "type descriptor"
	TIMER_OBJ         ObjectType = "timer"
	LAZY_THUNK        ObjectType = "lazy thunk"
	NULL              ObjectType = "null"
	LENGTH_RESTRICTED ObjectType = "length-restricted template"
	COMPONENT_REF     ObjectType = "component reference"
	CLASS_DESC        ObjectType = "class descriptor"
	CLASS_INSTANCE    ObjectType = "object reference"
	PORT_REF          ObjectType = "port reference"
)

// PortRef is the runtime value of a component's port instance. Binding
// a port name to a PortRef (rather than the old phantom Undefined) lets
// a port be passed as an actual parameter and aliased: the formal
// carries the originating instance name so an operation on the formal
// (`p_port.stop`) acts on the same port the caller named (`p`), and
// distinguishes a port `.stop`/`.start`/`.halt` from the component and
// timer operations that share those selectors (ETSI 5.4.2 / 22.1).
type PortRef struct {
	Name string
}

func (p *PortRef) Type() ObjectType { return PORT_REF }
func (p *PortRef) Inspect() string  { return "port " + p.Name }
func (p *PortRef) Equal(o Object) bool {
	other, ok := o.(*PortRef)
	return ok && other.Name == p.Name
}

// LazyThunk wraps an unevaluated expression for `@lazy` and `@fuzzy`
// formal parameters. The interpreter recognises the type when an
// identifier is looked up and either evaluates it once and replaces
// the binding (lazy) or evaluates it every time (fuzzy).
//
// The Expr field is `interface{}` rather than syntax.Expr to avoid
// pulling the syntax package into runtime; the interpreter casts
// back when it owns the call site.
type LazyThunk struct {
	Expr   interface{}
	Env    Scope
	Fuzzy  bool
	Once   bool
	Cached Object
}

func (l *LazyThunk) Type() ObjectType { return LAZY_THUNK }
func (l *LazyThunk) Inspect() string  { return "<lazy>" }
func (l *LazyThunk) Equal(o Object) bool {
	if other, ok := o.(*LazyThunk); ok {
		return l == other
	}
	return false
}

// TimerHandle is the runtime representation of a TTCN-3 timer value.
// We don't model a real clock yet, but enough state lives here to
// answer `.running`, `.read`, `.start`, and `.stop` against the same
// object - including across parameter passing. Timer parameters are
// passed by reference in TTCN-3, so the same pointer flows from the
// declaration site into each callee.
//
// The Ticks field implements a coarse virtual-clock: each `.read` and
// each `.running` access bumps the counter, and once it crosses
// MaxTicks the timer flips to expired. Without this, loops like
// `while (t.running) { ... t.read ... }` would never terminate because
// nothing advances time.
type TimerHandle struct {
	Name     string
	Running  bool
	Duration float64
	Ticks    int
	MaxTicks int
	// StartedAt is the wall-clock time at which `.start` was
	// called. We use it so `.timeout` can block until the timer
	// has actually expired. Zero when the timer
	// has never been started or has been stopped.
	StartedAt time.Time
	// DefaultDuration is the declared duration on the `timer T
	// := N` declaration site. `.start` without an argument
	// restores Duration to DefaultDuration (ETSI 23.2): an
	// earlier `.start(M)` override does NOT persist into a later
	// bare `.start`.
	DefaultDuration float64
	// StartedAtVirtual is the per-testcase virtual-clock reading at
	// the moment `.start` was called. `.read` reports the virtual
	// elapsed time (VirtualClock - StartedAtVirtual) so a sibling
	// `T2.timeout` that advances the virtual clock by T2's duration
	// is observable as elapsed time on T1 deterministically, without
	// depending on wall-clock jitter (ETSI 23.4, Sem_2304_003).
	StartedAtVirtual float64
}

func (t *TimerHandle) Type() ObjectType { return TIMER_OBJ }
func (t *TimerHandle) Inspect() string  { return "timer " + t.Name }
func (t *TimerHandle) Equal(o Object) bool {
	if other, ok := o.(*TimerHandle); ok {
		return t == other
	}
	return false
}

// Range is a template-style range value, the runtime representation of
// `(low..high)` written in TTCN-3 source. Either bound may be nil to
// model `-infinity` / `infinity`. The interpreter produces a Range when
// it sees a `..` operator and the match operator uses it for the
// "value is inside the range" check.
type Range struct {
	Lower Object
	Upper Object
	// LowerExcl / UpperExcl mark an exclusive boundary written with `!`
	// in TTCN-3 (`(!0..2)` excludes 0, `(0..!2)` excludes 2). A nil
	// bound denotes -infinity / +infinity and is never exclusive.
	LowerExcl bool
	UpperExcl bool
}

func (r *Range) Type() ObjectType { return RANGE }

// LengthRestricted wraps an inner template with a length attribute,
// e.g. `?length(1..3)` or `"abc" length(3)`. The matcher checks the
// inner template first and then validates the value's length against
// [Min, Max]; Max == -1 means "no upper bound" (i.e. `length(N..infinity)`).
type LengthRestricted struct {
	Inner Object
	Min   int
	Max   int
}

func (l *LengthRestricted) Type() ObjectType { return LENGTH_RESTRICTED }
func (l *LengthRestricted) Inspect() string {
	if l.Max == -1 {
		return fmt.Sprintf("%s length(%d..infinity)", inspectOrNil(l.Inner), l.Min)
	}
	if l.Min == l.Max {
		return fmt.Sprintf("%s length(%d)", inspectOrNil(l.Inner), l.Min)
	}
	return fmt.Sprintf("%s length(%d..%d)", inspectOrNil(l.Inner), l.Min, l.Max)
}
func (l *LengthRestricted) Equal(other Object) bool {
	o, ok := other.(*LengthRestricted)
	if !ok {
		return false
	}
	return l.Min == o.Min && l.Max == o.Max && objectsEqual(l.Inner, o.Inner)
}

func inspectOrNil(o Object) string {
	if o == nil {
		return "<nil>"
	}
	return o.Inspect()
}
func (r *Range) Inspect() string {
	lo, hi := "-infinity", "infinity"
	if r.Lower != nil {
		lo = r.Lower.Inspect()
	}
	if r.Upper != nil {
		hi = r.Upper.Inspect()
	}
	return "(" + lo + ".." + hi + ")"
}
func (r *Range) Equal(other Object) bool {
	o, ok := other.(*Range)
	if !ok {
		return false
	}
	return objectsEqual(r.Lower, o.Lower) && objectsEqual(r.Upper, o.Upper)
}

func objectsEqual(a, b Object) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(b)
}

// TypeDesc is the runtime stand-in for a TTCN-3 type declaration. Most
// of the interpreter does not need a concrete representation of the
// type itself - the language is structurally typed at runtime - but the
// Annex E attribute-access syntax `MyType.encode`, `MyType.variant`,
// ... requires *something* that can answer to selector lookups. We bind
// every named type to a TypeDesc whose Attrs map holds the recursively
// resolved `with { ... }` clauses (specific overrides win over the
// enclosing module's defaults).
type TypeDesc struct {
	Name  string
	Attrs map[string][]string
	// Override records, per Annex E attribute kind (lower-cased), that
	// the effective attribute was declared with the `override` modifier
	// (ETSI 27.7). An override attribute on a record propagates to its
	// fields, superseding the encode/variant of the field's own type;
	// a plain attribute does not (the field keeps its type's value).
	Override map[string]bool
	// OwnAttrs holds the attributes declared DIRECTLY on this type
	// (its own `with { ... }` clause), as opposed to Attrs which also
	// carries values inherited from enclosing scopes. OwnLocal marks,
	// per kind, an attribute confined with `@local`. The field
	// attribute-inheritance chain (ETSI 27.1.2) consults a type's OWN
	// non-@local attribute, never an inherited or @local one.
	OwnAttrs map[string][]string
	OwnLocal map[string]bool
	// Underlying names the referenced type of a type synonym
	// (`type R S` makes S's Underlying "R"). A synonym carries no
	// Struct of its own; field access and field attribute inheritance
	// follow the underlying type, with the synonym's own attributes
	// forming an enclosing layer above it.
	Underlying string
	// CharLo/CharHi bound a charstring subtype declared with a single
	// ("lo".."hi") range; HasCharRange reports whether they are set.
	// Used to expand a `\N{TypeRef}` pattern reference (B.1.5.4) into
	// a [lo-hi] character class.
	CharLo, CharHi rune
	HasCharRange   bool
	// Struct carries the declaration of a record/set/union type so the
	// interpreter can map a positional value literal onto the declared
	// field names (type-directed coercion). Nil for non-struct types.
	Struct *syntax.StructTypeDecl
	// ListKind is SET_OF for a `set of` type so values/templates of
	// the type are tagged unordered (order-independent matching);
	// empty for record-of and non-list types.
	ListKind ListType
	// IndexOffset is the declared lower index bound of a constrained
	// array subtype (`type integer T[1..2]` => 1), or 0 for a plain
	// `[N]` dimension or a non-array type. A value assigned to a
	// variable of such a type adopts this offset so `v[lo]` reads the
	// first element (ETSI 6.2.7 / 6.3.1).
	IndexOffset int
}

func (t *TypeDesc) Type() ObjectType { return TYPE_DESC }
func (t *TypeDesc) Inspect() string  { return "type " + t.Name }
func (t *TypeDesc) Equal(obj Object) bool {
	other, ok := obj.(*TypeDesc)
	return ok && other.Name == t.Name
}

// Lookup returns the recorded list of strings for an attribute kind
// (`encode`, `variant`, `extension`, `display`, `optional`) or false
// when nothing is recorded. The lookup is case-insensitive on the
// kind to mirror TTCN-3's case-insensitive keywords.
func (t *TypeDesc) Lookup(kind string) ([]string, bool) {
	if t == nil || t.Attrs == nil {
		return nil, false
	}
	v, ok := t.Attrs[strings.ToLower(kind)]
	return v, ok
}

// ComponentRef is the runtime stand-in for a TTCN-3 component
// reference, the value produced by `MyComp.create` and bound to a
// `var MyComp v_ptc`. The interpreter does not run components as
// separate threads (function bodies are evaluated synchronously
// against a shared loopback port queue) but tagging messages with
// the originating component lets `from <ref>` constraints and
// `-> sender v` redirects line up the way the standard requires.
//
// ID is monotonically increasing within a TestcaseExec; two refs are
// Equal iff their IDs agree. TypeName is the component type the
// caller used at create time and is purely informational (the
// loopback model does no type checking).
type ComponentRef struct {
	ID       int64
	TypeName string
	Module   string
	Name     string // optional `name := "..."` from MyComp.create(name)
	// mu guards the mutable lifecycle fields (alive, done, verdict) that
	// a forked PTC goroutine writes while the MTC concurrently observes
	// them via `comp.done` / `comp.alive` / `comp.running` /
	// `comp.done -> value`. All other fields are set on the owning
	// goroutine before/without concurrency and need no lock.
	mu sync.Mutex
	// alive is false once `.stop`/`.kill` ran against the ref. Access via
	// IsAlive/SetAlive (mu-guarded).
	alive bool
	// AliveModifier records whether the component was created with
	// the `alive` keyword (`MyComp.create alive`). Per TTCN-3 21.3
	// `comp.stop` on an alive component only suspends behaviour -
	// the ref stays Alive=true and can be re-`.start`-ed; only
	// `comp.kill` actually flips Alive=false.
	AliveModifier bool
	// done is set once a `.start(...)` body finished (or `.stop`
	// ran) on this ref. It lets `comp.done` / `all component.done`
	// answer true while the ref is still alive=true (the alive
	// modifier keeps the component reachable for restart). Access via
	// IsDone/SetDone (mu-guarded) — a forked PTC writes it on its own
	// goroutine while the MTC polls it.
	done bool

	// Started records that a `.start(...)` ran on this ref, even when
	// the loopback model skipped the body (e.g. a finite-timer PTC).
	// ModeledDuration/StartedAt then drive virtual completion: a PTC
	// whose body only blocks on a finite `timer.timeout` is treated
	// as terminated once that much wall-clock has elapsed since the
	// start (the MTC's own blocking `timeout` provides the real
	// observation window). ModeledDuration == 0 means "no finite
	// model" - such a body stays running until an explicit stop/kill.
	Started         bool
	StartedAt       time.Time
	ModeledDuration float64

	// ModeledKill records that the skipped finite-timer body ends in a
	// `kill` (rather than a natural return or `stop`). Once the modelled
	// duration elapses such a component counts as killed and no longer
	// alive even though it was created with the `alive` modifier, which
	// would otherwise keep it reusable (ETSI 21.3.4/21.3.8).
	ModeledKill bool

	// verdict is this component's local verdict, accumulated from the
	// `setverdict(...)` calls executed while it was the running
	// component. `comp.done -> value v` reads it (ETSI 21.3.7). Access
	// via GetVerdict/MergeVerdict (mu-guarded) — a forked PTC merges into
	// it via setverdict on its own goroutine.
	verdict Verdict

	// Scope holds per-component member state for PTC execution. Alive
	// components reuse it across restarts; non-alive components get a
	// fresh scope for each run.
	Scope Scope

	// LastCallStopped records whether the most recent component .call
	// ended via stop/kill. Redirect clauses are suppressed in that
	// incomplete-execution case (ETSI 21.3.10).
	LastCallStopped bool
}

// MergeVerdict folds v into the component's local verdict using the
// TTCN-3 22.4.1 ordering (none < pass < inconc < fail < error): a
// verdict can only ever get worse.
func (c *ComponentRef) MergeVerdict(v Verdict) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.verdict == "" {
		c.verdict = NoneVerdict
	}
	if verdictRank(v) > verdictRank(c.verdict) {
		c.verdict = v
	}
}

// IsDone reports whether the component's started behaviour has finished.
// Safe to call from any goroutine (mu-guarded).
func (c *ComponentRef) IsDone() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.done
}

// SetDone records completion of the component's started behaviour.
func (c *ComponentRef) SetDone(v bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.done = v
	c.mu.Unlock()
}

// IsAlive reports whether the component is still reachable (not
// stopped/killed). Safe to call from any goroutine (mu-guarded).
func (c *ComponentRef) IsAlive() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.alive
}

// SetAlive sets the component's reachability.
func (c *ComponentRef) SetAlive(v bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.alive = v
	c.mu.Unlock()
}

// GetVerdict returns the component's accumulated local verdict.
func (c *ComponentRef) GetVerdict() Verdict {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.verdict
}

func (c *ComponentRef) Type() ObjectType { return COMPONENT_REF }
func (c *ComponentRef) Inspect() string {
	if c == nil {
		return "null"
	}
	if c.Name != "" {
		return fmt.Sprintf("%s(\"%s\")", c.TypeName, c.Name)
	}
	return fmt.Sprintf("%s#%d", c.TypeName, c.ID)
}
func (c *ComponentRef) Equal(other Object) bool {
	o, ok := other.(*ComponentRef)
	if !ok {
		return false
	}
	if c == nil || o == nil {
		return c == o
	}
	return c.ID == o.ID
}

type Unit int

const (
	Bit   Unit = 1
	Hex   Unit = 4
	Octet Unit = 8
)

func (u Unit) Base() int {
	switch u {
	case Bit:
		return 2
	case Hex, Octet:
		return 16
	default:
		return -1
	}
}

var (
	ErrSyntax = errors.New("invalid syntax")
	Undefined = &singelton{typ: UNDEFINED}
	Break     = &singelton{typ: BREAK}
	Continue  = &singelton{typ: CONTINUE}
	// Repeat is the sentinel a `repeat` statement evaluated
	// inside an alt clause body returns. The alt scheduler
	// catches it and re-evaluates every clause from the top
	// per TTCN-3 21.2.4. Outside an alt the interpreter raises
	// a runtime error because a bare `repeat` has no meaning
	// (the spec forbids it).
	Repeat    = &singelton{typ: REPEAT}
	Any       = &singelton{typ: ANY}
	AnyOrNone = &singelton{typ: ANY_OR_NONE}
	// Null is the value of TTCN-3 `null` (the empty default
	// reference, the empty address, the null component reference,
	// and the null object reference). It is *bound* and *present*,
	// distinguishing it from Undefined which represents
	// uninitialised / omitted values.
	Null = &singelton{typ: NULL}
	// Omit is the value of the TTCN-3 `omit` special value: an
	// optional field that is explicitly absent. It is distinct from
	// Undefined (uninitialised / unmodelled): `match(presentValue,
	// omit)` is false, whereas Undefined behaves as a wildcard. For
	// equality and presence predicates Omit and Undefined both denote
	// an absent field and compare equal.
	Omit = &singelton{typ: OMIT}
)

// IfPresent wraps a template carrying the `ifpresent` modifier
// (ETSI B.1.4.2): it matches an absent value (omit / uninitialised),
// or a present value that matches the wrapped inner template.
type IfPresent struct {
	Inner Object
}

func (i *IfPresent) Type() ObjectType { return IF_PRESENT }
func (i *IfPresent) Inspect() string {
	if i.Inner == nil {
		return "ifpresent"
	}
	return i.Inner.Inspect() + " ifpresent"
}
func (i *IfPresent) Equal(obj Object) bool {
	other, ok := obj.(*IfPresent)
	if !ok {
		return false
	}
	if i.Inner == nil || other.Inner == nil {
		return i.Inner == other.Inner
	}
	return i.Inner.Equal(other.Inner)
}

type singelton struct {
	typ ObjectType
}

func (s *singelton) Inspect() string  { return string(s.typ) }
func (s *singelton) Type() ObjectType { return s.typ }

func (s *singelton) Equal(obj Object) bool {
	if other, ok := obj.(*singelton); ok {
		if s.typ == other.typ {
			return true
		}
		// `omit` and the uninitialised/absent value denote the same
		// thing - an absent optional field - so they compare equal
		// (e.g. `v == { omit, "abc" }` against an implicitly-omitted
		// field).
		if isAbsentType(s.typ) && isAbsentType(other.typ) {
			return true
		}
	}
	return false
}

// isAbsentType reports whether an ObjectType denotes an absent value
// (the explicit `omit` or the uninitialised/unmodelled Undefined).
func isAbsentType(t ObjectType) bool {
	return t == OMIT || t == UNDEFINED
}

type Error struct {
	Err error
}

func (e *Error) Error() string    { return e.Err.Error() }
func (e *Error) Unwrap() error    { return e.Err }
func (e *Error) Type() ObjectType { return ERROR }
func (e *Error) Inspect() string  { return fmt.Sprintf("Error: %s", e.Error()) }
func (e *Error) Equal(obj Object) bool {
	if other, ok := obj.(*Error); ok {
		return errors.Is(e, other)
	}
	return false
}

func Errorf(format string, a ...interface{}) *Error {
	return &Error{Err: fmt.Errorf(format, a...)}
}

func IsError(v interface{}) bool {
	_, ok := v.(*Error)
	return ok
}

type Bool bool

func (b Bool) Type() ObjectType { return BOOL }
func (b Bool) Inspect() string  { return fmt.Sprintf("%t", b) }
func (b Bool) Bool() bool       { return bool(b) }

func (b Bool) Equal(obj Object) bool {
	if other, ok := obj.(Bool); ok {
		return b == other
	}
	return false
}

func (b Bool) hashKey() hashKey {
	var value uint64
	if b {
		value = 1
	} else {
		value = 0
	}
	return hashKey{Type: b.Type(), Value: value}
}

func NewBool(b bool) Bool {
	return Bool(b)
}

type Float float64

func (f Float) Type() ObjectType { return FLOAT }
func (f Float) Inspect() string  { return fmt.Sprint(float64(f)) }

func (f Float) Equal(obj Object) bool {
	other, ok := obj.(Float)
	if !ok {
		return false
	}
	// TTCN-3 7.1.1: `not_a_number == not_a_number` is true (unlike
	// IEEE 754). Two NaNs compare equal regardless of their bit
	// representation so any pair of `not_a_number` literals match.
	if math.IsNaN(float64(f)) && math.IsNaN(float64(other)) {
		return true
	}
	return f == other
}

func NewFloat(s string) Float {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		panic(err.Error())
	}
	return Float(f)
}

type Int struct{ *big.Int }

func (i Int) Type() ObjectType { return INTEGER }
func (i Int) Inspect() string  { return i.String() }
func (i Int) Value() *big.Int  { return i.Int }

func (i Int) Equal(obj Object) bool {
	if other, ok := obj.(Int); ok {
		return i.Cmp(other.Int) == 0
	}
	return false
}

func (i Int) hashKey() hashKey {
	h := fnv.New64a()
	h.Write(i.Bytes())
	return hashKey{Type: i.Type(), Value: h.Sum64()}
}

func NewInt(v interface{}) Int {
	switch v := v.(type) {
	case int:
		return Int{big.NewInt(int64(v))}
	case int8:
		return Int{big.NewInt(int64(v))}
	case int16:
		return Int{big.NewInt(int64(v))}
	case int32:
		return Int{big.NewInt(int64(v))}
	case int64:
		return Int{big.NewInt(v)}
	case uint:
		return Int{new(big.Int).SetUint64(uint64(v))}
	case uint8:
		return Int{new(big.Int).SetUint64(uint64(v))}
	case uint16:
		return Int{new(big.Int).SetUint64(uint64(v))}
	case uint32:
		return Int{new(big.Int).SetUint64(uint64(v))}
	case uint64:
		return Int{new(big.Int).SetUint64(v)}
	case string:
		i := &big.Int{}
		i.SetString(v, 10)
		return Int{i}
	case *big.Int:
		if v == nil {
			return Int{new(big.Int)}
		}
		return Int{new(big.Int).Set(v)}
	case big.Int:
		return Int{new(big.Int).Set(&v)}
	default:
		panic(fmt.Sprintf("cannot convert %T to Int", v))
	}
}

type EnumRange struct {
	First, Last int
}

func (er *EnumRange) Contains(x int) bool {
	return x >= er.First && x <= er.Last
}
func (er EnumRange) ToString() string {
	if er.First == er.Last {
		return fmt.Sprintf("%d", er.First)
	}
	return fmt.Sprintf("%d..%d", er.First, er.Last)
}

type EnumElements map[string][]EnumRange

type EnumType struct {
	Name     string
	Elements EnumElements
}

func (et *EnumType) Type() ObjectType { return ENUM_TYPE }
func (et *EnumType) Inspect() string {
	var ret []string
	for name, val := range et.Elements {
		var retE []string
		for _, r := range val {
			retE = append(retE, r.ToString())
		}
		ret = append(ret, fmt.Sprintf("%s(%s)", name, strings.Join(retE, ", ")))
	}
	return et.Name + "{" + strings.Join(ret, ", ") + "}"
}

func (et *EnumType) Equal(obj Object) bool {
	other, ok := obj.(*EnumType)
	if !ok {
		return false
	}
	return reflect.DeepEqual(et, other)
}

func NewEnumType(enumTypeName string, Enums ...string) *EnumType {
	ret := EnumType{}
	ret.Name = enumTypeName
	ret.Elements = make(EnumElements)
	for EnumId, EnumName := range Enums {
		ranges := []EnumRange{{First: EnumId, Last: EnumId}}
		ret.Elements[EnumName] = ranges
	}
	return &ret
}

type EnumValue struct {
	typeRef *EnumType
	key     string
	value   int
	// matchRanges, when non-empty, turns this enum value into a
	// matching template: the `Label(v1, lo..hi, ...)` parameterised
	// form of ETSI 6.2.4. A concrete enum value matches the template
	// when it shares the key and its integer falls in any range.
	matchRanges []EnumRange
}

func (ev *EnumValue) Type() ObjectType { return ENUM_VALUE }
func (ev *EnumValue) Inspect() string {
	if len(ev.matchRanges) > 0 {
		var rs []string
		for _, r := range ev.matchRanges {
			rs = append(rs, r.ToString())
		}
		return fmt.Sprintf("%s.%s(%s)", ev.typeRef.Name, ev.key, strings.Join(rs, ", "))
	}
	return fmt.Sprintf("%s.%s(%d)", ev.typeRef.Name, ev.key, ev.value)
}

// IsTemplate reports whether the enum value carries match ranges and
// thus acts as a matching template rather than a concrete value.
func (ev *EnumValue) IsTemplate() bool { return ev != nil && len(ev.matchRanges) > 0 }

// WithMatchRanges returns a copy of the enum value turned into a
// parameterised-enum matching template constrained to ranges.
func (ev *EnumValue) WithMatchRanges(ranges []EnumRange) *EnumValue {
	return &EnumValue{typeRef: ev.typeRef, key: ev.key, value: ev.value, matchRanges: ranges}
}

// MatchesEnum reports whether the concrete enum value other is matched
// by this enum template: same key and an integer inside one of the
// template's associated ranges (ETSI 6.2.4).
func (ev *EnumValue) MatchesEnum(other *EnumValue) bool {
	if ev == nil || other == nil || ev.key != other.key {
		return false
	}
	for _, r := range ev.matchRanges {
		if r.Contains(other.value) {
			return true
		}
	}
	return false
}

// IntValue returns the enum's numeric value.
func (ev *EnumValue) IntValue() int { return ev.value }

// Key returns the enum's textual label.
func (ev *EnumValue) Key() string { return ev.key }
func (ev *EnumValue) Equal(obj Object) bool {
	other, ok := obj.(*EnumValue)
	if !ok {
		return false
	}
	return ev.key == other.key && ev.value == other.value
}
func (ev *EnumValue) SetValueByKey(key string) *Error {
	keyRanges, ok := ev.typeRef.Elements[key]
	if !ok {
		return Errorf("%s does not exist in Enum %s", key, ev.typeRef.Name)
	}
	if len(keyRanges) != 1 {
		return Errorf("Provided key has more than one value")
	}
	if keyRanges[0].First != keyRanges[0].Last {
		return Errorf("Provided key has range")
	}
	ev.key = key
	ev.value = keyRanges[0].First
	return nil
}
func (ev *EnumValue) SetValueById(id int) *Error {
	for key, enumRanges := range ev.typeRef.Elements {
		for _, enumRange := range enumRanges {
			if enumRange.Contains(id) {
				ev.key = key
				ev.value = id
				return nil
			}
		}
	}
	return Errorf("id %d does not exist in any ranges of Enum %s", id, ev.typeRef.Name)
}

// WithExplicitInt returns a copy of the enum value carrying the
// explicit integer i (the `Label(i)` notation of ETSI 6.2.4). i must
// fall within one of the integer ranges associated with the value's
// own key; otherwise an error is returned.
func (ev *EnumValue) WithExplicitInt(i int) (*EnumValue, *Error) {
	if ev == nil || ev.typeRef == nil {
		return nil, Errorf("not an enumerated value")
	}
	ranges, ok := ev.typeRef.Elements[ev.key]
	if !ok {
		return nil, Errorf("%s does not exist in Enum %s", ev.key, ev.typeRef.Name)
	}
	for _, r := range ranges {
		if r.Contains(i) {
			return &EnumValue{typeRef: ev.typeRef, key: ev.key, value: i}, nil
		}
	}
	return nil, Errorf("%d is not in any range associated with %s.%s", i, ev.typeRef.Name, ev.key)
}

func NewEnumValueByKey(enumType *EnumType, key string) (*EnumValue, error) {

	ret := &EnumValue{typeRef: enumType, key: key, value: 0}
	if err := ret.SetValueByKey(key); err != nil {
		return nil, err
	}
	return ret, nil
}

func NewEnumValue(enumType *EnumType, key string, id int) (*EnumValue, error) {

	keyRanges, ok := enumType.Elements[key]
	if !ok {
		return nil, fmt.Errorf("%s does not exist in Enum %s", key, enumType.Name)
	}
	for _, enumRange := range keyRanges {
		if enumRange.Contains(id) {
			return &EnumValue{typeRef: enumType, key: key, value: int(id)}, nil
		}
	}
	return nil, fmt.Errorf("%s does no contain %d", key, id)
}

type String struct {
	Value []rune
	ascii bool
	// IsPattern marks the string as the result of a `pattern "..."`
	// expression; the matcher treats `?` / `*` / `#N` etc. as
	// TTCN-3 pattern operators only when this flag is set. Plain
	// charstring values (e.g. from `"ABC"` literals or `&`-
	// concatenations of non-pattern strings) match byte-for-byte.
	IsPattern bool
	// NoCase marks a `pattern @nocase "..."` template; the matcher
	// performs case-insensitive comparison for these (B.1.5.6).
	NoCase bool
	// PatternComplex marks a pattern whose source used a matching
	// mechanism beyond literal characters / `?` / `*` (a set [..],
	// reference {ref}/\N{}, quadruple \q{}, repetition #/+, etc.).
	// substr (16.1.2) is only allowed on `?`/`*` patterns, so it
	// rejects a complex one.
	PatternComplex bool
	// Interned signals that this *String is shared from
	// asciiSingleRuneCache and MUST be copy-on-write before any
	// mutation. The cache exists to avoid the per-call malloc that
	// `body[i]` and one-character literal evaluation otherwise
	// trigger millions of times per testcase (the hot path).
	Interned bool
}

// asciiSingleRuneCache holds one preallocated *String per ASCII code
// point. NewCharstring / NewUniversalString return one of these when
// the input is a single ASCII character, so every `body[i]` read and
// every one-byte literal compare reuses the same pointer instead of
// allocating a fresh String. Mutators clone-on-first-write
// (see cloneIfInterned) so sharing stays safe.
var asciiSingleRuneCache [128]*String

func init() {
	for i := 0; i < len(asciiSingleRuneCache); i++ {
		asciiSingleRuneCache[i] = &String{
			Value:    []rune{rune(i)},
			ascii:    true,
			Interned: true,
		}
	}
}

func NewCharstring(s string) *String {
	if len(s) == 1 && s[0] < 128 {
		return asciiSingleRuneCache[s[0]]
	}
	return &String{Value: []rune(s), ascii: true}
}
func NewUniversalString(s string) *String {
	if len(s) == 1 && s[0] < 128 {
		return asciiSingleRuneCache[s[0]]
	}
	return &String{Value: []rune(s)}
}

// AsciiSingleRuneString returns the interned cache entry for r when r
// is ASCII (0..127); returns nil otherwise. Lets hot-path callers
// (e.g. the interpreter's `body[i]` evaluator) get to the pool
// without re-checking the bounds themselves.
func AsciiSingleRuneString(r rune) *String {
	if r < 0 || r >= 128 {
		return nil
	}
	return asciiSingleRuneCache[r]
}

// NewCharstringFromRunes is the zero-copy variant of NewCharstring
// for callers that already hold a []rune slice (substr, charstring
// concatenation, body[i:j]). It avoids the round-trip through Go's
// `string(...)` conversion (one alloc) and `[]rune(...)` re-parse
// (another alloc) the NewCharstring(string(s[i:j])) form pays. The
// caller must not mutate src after the call; callers that need
// independent storage should pass a freshly copied slice.
//
// The single-ASCII-rune cache is honoured so `s[i:i+1]` (one char)
// returns the interned pointer even on this hot path.
func NewCharstringFromRunes(src []rune) *String {
	if len(src) == 1 && src[0] < 128 {
		return asciiSingleRuneCache[src[0]]
	}
	return &String{Value: src, ascii: true}
}

// cloneIfInterned returns s if it is privately owned, otherwise a
// freshly allocated *String with the same content. Call from any
// path that intends to mutate s.Value so a write to an interned
// cache entry doesn't corrupt every other holder. The boolean
// reports whether the caller should swap its reference (true) or
// continue with the original pointer (false).
func (s *String) cloneIfInterned() (*String, bool) {
	if s == nil || !s.Interned {
		return s, false
	}
	cp := make([]rune, len(s.Value))
	copy(cp, s.Value)
	return &String{Value: cp, ascii: s.ascii, IsPattern: s.IsPattern}, true
}

// CloneIfInterned is the exported form of cloneIfInterned for callers
// outside this package (the interpreter's index-assignment path
// needs it to copy-on-write before mutating).
func (s *String) CloneIfInterned() (*String, bool) { return s.cloneIfInterned() }

// Type returns the object type CHARSTRING. This type is used for both
// charstrings and universal charstrings.
func (s *String) Type() ObjectType {
	return CHARSTRING
}

// IsASCII returns true if the string only contains ASCII characters.
func (s *String) IsASCII() bool {
	return s.ascii
}

// Inpect returns the string value as a TTCN-3 string literal.
func (s *String) Inspect() string {
	return fmt.Sprintf("%q", string(s.Value))
}

// String returns the string a Go string.
func (s *String) String() string {
	return string(s.Value)
}

func (s *String) Equal(obj Object) bool {
	b, ok := obj.(*String)
	if !ok || len(s.Value) != len(b.Value) {
		return false
	}
	for i, v := range s.Value {
		if v != b.Value[i] {
			return false
		}
	}
	return true
}

func (s *String) Len() int {
	return len(s.Value)
}

func (s *String) Get(i int) Object {
	if 0 <= i || i < len(s.Value) {
		ch := NewUniversalString(string(s.Value[i]))
		ch.ascii = s.ascii
		return ch
	}
	return Undefined
}

func (s *String) hashKey() hashKey {
	h := fnv.New64a()
	h.Write([]byte(string(s.Value)))
	return hashKey{Type: s.Type(), Value: h.Sum64()}
}

type Binarystring struct {
	String string
	Value  *big.Int
	Unit   Unit
	Length int
}

func (b *Binarystring) Type() ObjectType {
	switch b.Unit {
	case Bit:
		return BITSTRING
	case Octet:
		return OCTETSTRING
	case Hex:
		return HEXSTRING
	default:
		panic("Unknown unit")
	}
}

func (b *Binarystring) Inspect() string {
	// Template literals (Value == -1) carry the original source
	// representation in String; render that verbatim so log lines
	// stay readable. For concrete values we format the big.Int
	// using the unit's natural width.
	if b.Value != nil && b.Value.Sign() < 0 && b.String != "" {
		return b.String
	}
	switch b.Unit {
	case Bit:
		return fmt.Sprintf("'%0*b'B", b.Length, b.Value)
	case Octet:
		return fmt.Sprintf("'%0*X'O", b.Length*2, b.Value)
	default:
		return fmt.Sprintf("'%0*X'H", b.Length, b.Value)
	}
}

func (b *Binarystring) Equal(obj Object) bool {
	if other, ok := obj.(*Binarystring); ok {
		return b.Value.Cmp(other.Value) == 0
	}
	return false
}

func (b *Binarystring) hashKey() hashKey {
	h := fnv.New64a()
	h.Write(b.Value.Bytes())
	return hashKey{Type: b.Type(), Value: h.Sum64()}
}

func NewBinarystring(s string) (*Binarystring, error) {

	if len(s) < 3 || s[0] != '\'' || s[len(s)-2] != '\'' {
		return nil, ErrSyntax
	}

	var unit Unit
	s = s[:len(s)-1] + strings.ToUpper(string(s[len(s)-1])) // Capitalize unit
	switch s[len(s)-1] {
	case 'B':
		unit = Bit
	case 'H':
		unit = Hex
	case 'O':
		unit = Octet
	default:
		return nil, ErrSyntax
	}
	n := removeWhitespaces(s[1 : len(s)-2])

	if i, ok := new(big.Int).SetString(n, unit.Base()); ok {
		return &Binarystring{String: s, Value: i, Unit: unit, Length: lengthInUnits(n, unit)}, nil
	}

	return NewBinarystringWithWildcards(s, unit)
}

// lengthInUnits reports the number of logical positions (bits, hex
// digits, or octets) the source string `n` encodes for the given unit.
// `len(n)` is in characters - for octets we divide by two because each
// octet is two hex digits.
func lengthInUnits(n string, unit Unit) int {
	if unit == Octet {
		return (len(n) + 1) / 2
	}
	return len(n)
}

func NewBinarystringWithWildcards(s string, unit Unit) (*Binarystring, error) {
	n := removeWhitespaces(s[1 : len(s)-2])

	switch unit {
	case Bit, Hex:
	case Octet:
		// Each octet is two hex digits, so a literal without
		// length-changing wildcards must have an even count. The
		// `?` wildcard *does* change length (it stands for any
		// single hex digit) - this is documented in TTCN-3 B.1.2.5
		// and used by tests like 'EE?FF'O - so we treat it the
		// same way as `*` here.
		if !strings.ContainsAny(n, "*?") && len(n)%2 != 0 {
			return nil, ErrSyntax
		}
	default:
		return nil, ErrSyntax
	}

	for _, r := range n {
		switch r {
		case '*', '?', '0', '1':
		case 'A', 'B', 'C', 'D', 'E', 'F', 'a', 'b', 'c', 'd', 'e', 'f', '2', '3', '4', '5', '6', '7', '8', '9':
			if unit == Bit {
				return nil, ErrSyntax
			}
		default:
			return nil, ErrSyntax
		}
	}
	return &Binarystring{String: s, Value: new(big.Int).SetInt64(-1), Unit: unit, Length: lengthInUnits(n, unit)}, nil
}

func (b *Binarystring) Len() int { return b.Length }

func (b *Binarystring) Get(index int) Object {
	width := int(b.Unit)/8 + 1
	// If b.Unit is Octett, each "digit" is two bytes wide
	s := removeWhitespaces(b.String)
	s = "'" + s[1+index*width:1+(index+1)*width] + s[len(s)-2:]
	n, _ := new(big.Int).SetString(s[1:len(s)-2], b.Unit.Base())
	return &Binarystring{String: s, Value: n, Unit: b.Unit, Length: 1} //Length one, even for Octett
}

func BigIntToBinaryString(b *big.Int, unit Unit) string {
	return "'" + b.Text(unit.Base()) + "'" + unit.String()
}

func (u Unit) String() string {
	switch u {
	case 1:
		return "B"
	case 4:
		return "H"
	case 8:
		return "O"
	default: // Will never happen
		return "-"
	}
}

func removeWhitespaces(s string) string {
	// TTCN-3 v4.11.1 6.1.1 allows `\<newline>` line-continuations
	// inside binary string literals. Both the backslash and the
	// whitespace characters are stripped from the value's content.
	removeWhitespaces := func(r rune) rune {
		if unicode.IsSpace(r) || r == '\\' {
			return -1
		}
		return r
	}
	return strings.Map(removeWhitespaces, s)
}

type ListType string

const (
	RECORD_OF   ListType = "" //default
	SET_OF      ListType = "set of"
	COMPLEMENT  ListType = "complement"
	SUBSET      ListType = "subset"
	SUPERSET    ListType = "superset"
	PERMUTATION ListType = "permutation"
	// VALUE_LIST is the parenthesised alternative-set template,
	// e.g. `(v1, v2, v3)`. A value matches iff it is equal to one
	// of the list elements.
	VALUE_LIST ListType = "value list"
)

type List struct {
	ListType
	Elements []Object
	// FieldNames, when non-empty, records the declared field names of
	// a record/set value held positionally in Elements (type-directed
	// coercion). It lets `.field` access, completeness checks and named
	// rendering work without changing the positional storage or the
	// element-wise Equal semantics. Empty for plain lists / record-of.
	FieldNames []string
	// IndexOffset is the declared lower index bound of a fixed-size
	// array declared with an index range (`v[2..5]`, ETSI 6.2.7): the
	// first element lives at index IndexOffset, so index access
	// subtracts it. Zero (the default) means the ordinary 0-based
	// indexing used by record-of / set-of / plain `v[N]` arrays.
	// Element storage and the element-wise Equal semantics are
	// unaffected, so an offset array still compares equal to a 0-based
	// value literal of the same elements.
	IndexOffset int
}

func (l *List) IsOrdered() bool {
	return l.ListType == ""
}

// FieldIndex returns the position of the named field in a record/set
// value, or -1 when this list has no field-name metadata or the name
// is unknown.
func (l *List) FieldIndex(name string) int {
	for i, n := range l.FieldNames {
		if n == name {
			return i
		}
	}
	return -1
}

func (l *List) Type() ObjectType { return LIST }
func (l *List) Inspect() string {
	var ss []string
	for i, obj := range l.Elements {
		var rendered string
		switch {
		case obj == nil:
			rendered = "null"
		case obj == Undefined:
			// An unbound element of a structured value renders as
			// "-" in assignment notation (ETSI Annex C.1.33,
			// any2unistr "canonical" form); the bare "UNINITIALIZED"
			// spelling is reserved for a top-level unbound scalar.
			rendered = "-"
		case obj == Omit:
			rendered = "omit"
		default:
			rendered = obj.Inspect()
		}
		// A record/set value held positionally with field-name
		// metadata renders in the `field := value` form (ETSI
		// Annex C value notation) rather than as a bare list.
		if i < len(l.FieldNames) && l.FieldNames[i] != "" {
			rendered = l.FieldNames[i] + " := " + rendered
		}
		ss = append(ss, rendered)
	}
	return "{" + strings.Join(ss, ", ") + "}"
}

func (l *List) Equal(obj Object) bool {
	other, ok := obj.(*List)
	if !ok {
		return false
	}

	// The order of elements is ignored, when at least one list is
	// unordered.
	//
	// The standard explicitly forbids this. Relaxing this restriction
	// makes untyped assignment lists easier to handle.
	//
	// We assume the proper semeantic checks are done before the runtime.
	if l.IsOrdered() && other.IsOrdered() {
		return EqualObjects(l.Elements, other.Elements)
	}

	return l.ListType == other.ListType && EqualObjectSet(l.Elements, other.Elements)

}

func (l *List) Get(index int) Object {
	return l.Elements[index]
}

func (l *List) Len() int {
	return len(l.Elements)
}

// NewList creates a new ordered list.
func NewList(objs ...Object) *List        { return &List{Elements: objs} }
func NewRecordOf(objs ...Object) *List    { return &List{Elements: objs} }
func NewSetOf(objs ...Object) *List       { return &List{Elements: objs, ListType: SET_OF} }
func NewSuperset(objs ...Object) *List    { return &List{Elements: objs, ListType: SUPERSET} }
func NewSubset(objs ...Object) *List      { return &List{Elements: objs, ListType: SUBSET} }
func NewPermutation(objs ...Object) *List { return &List{Elements: objs, ListType: PERMUTATION} }
func NewComplement(objs ...Object) *List  { return &List{Elements: objs, ListType: COMPLEMENT} }

type Function struct {
	Params *syntax.FormalPars
	Body   *syntax.BlockStmt
	Env    Scope
	// IsAltstep marks altstep declarations. Their body is a mix of
	// local declarations and CommClause statements; the interpreter
	// runs the leading declarations normally and then schedules the
	// trailing comm clauses with the same best-effort alt strategy
	// used by the AltStmt branch.
	IsAltstep bool

	// IsTemplate marks parametric template declarations bound as
	// Functions for the call path. The Ident-lookup path uses this
	// to auto-apply the template with no actuals when every formal
	// has a default - TTCN-3 allows `match(x, tname)` (no parens)
	// in that case.
	IsTemplate bool

	// Catch and Finally carry the object-oriented exception handlers
	// declared after the body (ETSI 5.2): `... } catch (T e) { ... }
	// finally { ... }`. They are run by the call path once the body
	// produces a RaisedValue (catch) or unconditionally (finally).
	Catch   []*syntax.CatchClause
	Finally *syntax.BlockStmt
}

func (f *Function) Type() ObjectType { return FUNCTION }
func (f *Function) Inspect() string {
	var buf bytes.Buffer
	buf.WriteString("function(\"")
	for i, p := range f.Params.List {
		if i != 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(p.Name.String())
	}
	buf.WriteString(")")
	return buf.String()
}

func (f *Function) Equal(obj Object) bool {
	if other, ok := obj.(*Function); ok {
		// Two function values are equal when they refer to the same
		// declaration. (The struct is no longer comparable by value
		// since it carries catch-clause slices.)
		return f == other
	}
	return false
}

type ReturnValue struct {
	Value Object
	// Stopped is set when the unwind was triggered by `stop` /
	// `self.stop` / `self.kill` rather than a plain `return` -
	// callers (e.g. `comp.call` writeback) need to skip inout
	// param writeback per TTCN-3 21.3.10 in that case.
	Stopped bool
}

func (r *ReturnValue) Type() ObjectType { return RETURN_VALUE }
func (r *ReturnValue) Inspect() string  { return r.Value.Inspect() }

// Goto is the control-flow signal produced by a `goto L` statement
// (ETSI 19.8). It bubbles up through block and loop evaluation until a
// block that contains the matching `label L` catches it and resumes
// execution at the statement after the label.
type Goto struct {
	Label string
}

func (g *Goto) Type() ObjectType { return GOTO_EVENT }
func (g *Goto) Inspect() string  { return "goto " + g.Label }
func (g *Goto) Equal(obj Object) bool {
	other, ok := obj.(*Goto)
	return ok && other.Label == g.Label
}

func (r *ReturnValue) Equal(obj Object) bool {
	if other, ok := obj.(*ReturnValue); ok {
		return r.Value.Equal(other.Value)
	}
	return false
}

// RaisedValue is the control-flow object produced by an object-oriented
// `raise <expr>` statement (ETSI ES 201 873-1 clause 5.2.3). It unwinds
// the call stack like a ReturnValue until a matching `catch` clause
// handles it; an uncaught RaisedValue reaching the testcase boundary is
// a dynamic test-case error. TypeName records the static type written
// at the raise site (when known) so catch clauses can match by type.
type RaisedValue struct {
	Value    Object
	TypeName string
}

func (r *RaisedValue) Type() ObjectType { return RAISED_VALUE }
func (r *RaisedValue) Inspect() string  { return "raised " + r.Value.Inspect() }

func (r *RaisedValue) Equal(obj Object) bool {
	if other, ok := obj.(*RaisedValue); ok {
		return r.Value.Equal(other.Value)
	}
	return false
}

type Verdict string

const (
	NoneVerdict   Verdict = "none"
	PassVerdict   Verdict = "pass"
	InconcVerdict Verdict = "inconc"
	FailVerdict   Verdict = "fail"
	ErrorVerdict  Verdict = "error"
)

func (v Verdict) Type() ObjectType { return VERDICT }
func (v Verdict) Inspect() string  { return string(v) }
func (v Verdict) Equal(obj Object) bool {
	if other, ok := obj.(Verdict); ok {
		return v == other
	}
	return false
}

func (v Verdict) hashKey() hashKey {
	var value uint64
	switch v {
	case NoneVerdict:
		value = 0
	case PassVerdict:
		value = 1
	case InconcVerdict:
		value = 2
	case FailVerdict:
		value = 3
	case ErrorVerdict:
		value = 4
	default:
		panic(Errorf("unknown verdict"))
	}
	return hashKey{Type: v.Type(), Value: value}

}

type Builtin struct {
	Fn func(args ...Object) Object
}

func (b *Builtin) Type() ObjectType { return BUILTIN_OBJ }
func (b *Builtin) Inspect() string  { return "builtin function" }
func (b *Builtin) Equal(obj Object) bool {
	if other, ok := obj.(*Builtin); ok {
		return b == other
	}
	return false
}

type hashable interface {
	hashKey() hashKey
}

type hashKey struct {
	Type  ObjectType
	Value uint64
}

type pair struct {
	Key   Object
	Value Object
}

// Map is a map of objects.
type Map struct {
	pairs map[hashKey][]pair
}

// MapPair exposes a single key/value pair held in a Map. Returned by
// Pairs() so callers (e.g. the interpreter's map-to-list coercion) can
// iterate without poking at the unexported storage layout.
type MapPair = pair

// Len reports the number of entries in the map. TTCN-3 6.2.8 defines
// lengthof(map) as the number of key/value pairs.
func (m *Map) Len() int {
	n := 0
	for _, ps := range m.pairs {
		n += len(ps)
	}
	return n
}

// Pairs returns a flat slice of every key/value pair currently held in
// the map. Order is unspecified.
func (m *Map) Pairs() []pair {
	var out []pair
	for _, bucket := range m.pairs {
		out = append(out, bucket...)
	}
	return out
}

// Get returns the value for the given key.
func (m *Map) Get(key Object) (Object, bool) {
	k, ok := key.(hashable)
	if !ok {
		return Errorf("%s is not hashable", key.Type()), false
	}

	for _, p := range m.pairs[k.hashKey()] {
		if p.Key.Equal(key) {
			return p.Value, true
		}
	}
	return nil, false
}

func (m *Map) Set(key Object, val Object) Object {
	k, ok := key.(hashable)
	if !ok {
		return Errorf("%s is not hashable", key.Type())
	}
	h := k.hashKey()
	m.pairs[h] = append(m.pairs[h], pair{Key: key, Value: val})
	return val
}

// Delete removes the entry for `key`. Returns true when an entry was
// removed; false when the key wasn't present (or the key isn't a
// hashable type). The TTCN-3 6.2.15.3 `unmap(M, k)` operation lowers
// to this method.
func (m *Map) Delete(key Object) bool {
	k, ok := key.(hashable)
	if !ok {
		return false
	}
	h := k.hashKey()
	bucket := m.pairs[h]
	for i, p := range bucket {
		if p.Key.Equal(key) {
			m.pairs[h] = append(bucket[:i], bucket[i+1:]...)
			if len(m.pairs[h]) == 0 {
				delete(m.pairs, h)
			}
			return true
		}
	}
	return false
}

func (m *Map) Type() ObjectType { return MAP }
func (m *Map) Inspect() string {
	var buf bytes.Buffer
	pairs := []string{}
	for _, bucket := range m.pairs {
		for _, pair := range bucket {
			pairs = append(pairs, fmt.Sprintf("[%s] := %s", pair.Key.Inspect(), pair.Value.Inspect()))
		}
	}
	buf.WriteString("{")
	buf.WriteString(strings.Join(pairs, ", "))
	buf.WriteString("}")

	return buf.String()
}

func (m *Map) Equal(obj Object) bool {
	other, ok := obj.(*Map)
	if !ok {
		return false
	}
	if len(m.pairs) != len(other.pairs) {
		return false
	}

	for k, a := range m.pairs {
		b, ok := other.pairs[k]
		if !ok {
			return false
		}
		if len(a) != len(b) {
			return false
		}

		for i, v := range a {
			if !v.Value.Equal(b[i].Value) {
				return false
			}
		}
	}

	return true
}

func NewMap() *Map {
	return &Map{pairs: make(map[hashKey][]pair)}
}

// TODO(5nord) For simplicity we reuse the Map implementation. We should implement proper record semantics later.
type Record struct {
	Fields map[string]Object
}

func (r *Record) Get(name string) (Object, bool) {
	val, ok := r.Fields[name]
	return val, ok
}

func (r *Record) Set(name string, val Object) Object {
	r.Fields[name] = val
	return nil
}

func (r *Record) Type() ObjectType { return RECORD }
func (r *Record) Inspect() string {
	var buf bytes.Buffer
	fields := []string{}
	for key, val := range r.Fields {
		fields = append(fields, fmt.Sprintf("%s := %s", key, val.Inspect()))
	}
	buf.WriteString("{")
	buf.WriteString(strings.Join(fields, ", "))
	buf.WriteString("}")

	return buf.String()
}

func (r *Record) Equal(obj Object) bool {
	other, ok := obj.(*Record)
	if !ok {
		return false
	}
	if len(r.Fields) != len(other.Fields) {
		return false
	}

	for k, a := range r.Fields {
		b, ok := other.Fields[k]
		if !ok {
			return false
		}
		if !a.Equal(b) {
			return false
		}
	}

	return true
}

func NewRecord() *Record {
	return &Record{Fields: make(map[string]Object)}
}

// ClassDesc is the runtime representation of a `type class`
// definition (ETSI ES 201 873-1 clause 5.1). It carries the AST
// declaration so the interpreter can resolve the field list,
// methods, constructor and parent class on demand, plus the scope
// the class was defined in (used as the closure for method bodies
// and to resolve the `extends` parent).
type ClassDesc struct {
	Name string
	Decl *syntax.ClassTypeDecl
	Env  Scope
}

func (c *ClassDesc) Type() ObjectType { return CLASS_DESC }
func (c *ClassDesc) Inspect() string  { return "class " + c.Name }
func (c *ClassDesc) Equal(obj Object) bool {
	other, ok := obj.(*ClassDesc)
	return ok && other != nil && other.Name == c.Name
}

// ClassInstance is a constructed object of a class type (ETSI
// 5.1.2). Fields holds the per-instance member values (the union of
// the class's own and all inherited fields); Class links back to the
// describing ClassDesc so method dispatch and `select class` can
// walk the inheritance chain. ClassInstance satisfies Scope so the
// existing `obj.field` / `this.field := v` selector machinery works
// unchanged.
type ClassInstance struct {
	Class  *ClassDesc
	Fields map[string]Object
}

func (ci *ClassInstance) Get(name string) (Object, bool) {
	val, ok := ci.Fields[name]
	return val, ok
}

func (ci *ClassInstance) Set(name string, val Object) Object {
	ci.Fields[name] = val
	return nil
}

func (ci *ClassInstance) Type() ObjectType { return CLASS_INSTANCE }

func (ci *ClassInstance) Inspect() string {
	name := ""
	if ci.Class != nil {
		name = ci.Class.Name
	}
	return "object " + name
}

// Equal uses reference identity: two object references are equal
// only when they denote the same instance (ETSI 5.1.2.2).
func (ci *ClassInstance) Equal(obj Object) bool {
	other, ok := obj.(*ClassInstance)
	return ok && other == ci
}

// EqualObjects compares two Object slices for equality.
func EqualObjects(a, b []Object) bool {
	if len(a) != len(b) {
		return false
	}

	for i, v := range a {
		if !v.Equal(b[i]) {
			return false
		}
	}

	return true
}

// EqualObjectSet compares two Object slices for equality ignoring the order of
// the elements.
//
// Current implementation is O(n^2).
func EqualObjectSet(a, b []Object) bool {
	if len(a) != len(b) {
		return false
	}

	for _, v := range a {
		for _, v2 := range b {
			if !v.Equal(v2) {
				return false
			}
		}
	}
	return true
}
