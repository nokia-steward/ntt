package interpreter_test

import (
	"fmt"
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3/syntax"
	"github.com/stretchr/testify/assert"
)

func TestInt(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"0", 0},
		{"-0", 0},
		{"+0", 0},
		{"10", 10},
		{"-10", -10},
		{"+10", 10},
		{"1+2*3", 7},
		{"(1+2)*3", 9},
	}
	for _, tt := range tests {
		val := testEval(t, tt.input)
		if val == nil {
			t.Errorf("Evaluation of %q returned nil", tt.input)
			continue
		}
		testInt(t, val, tt.expected)
	}

}

func TestBool(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"true", true},
		{"false", false},
		{"not true", false},
		{"not not true", true},
		{"not not not true", false},
		{"not false", true},
		{"not not false", false},
		{"not not not false", true},
		{"1<1", false},
		{"1<=1", true},
		{"1<2", true},
		{"1==1", true},
		{"1==2", false},
		{"1!=1", false},
		{"1!=2", true},
		{"2-1 < 2", true},
		{"2+1==1+2", true},
		{"true==false", false},
		{"true!=false", true},
	}
	for _, tt := range tests {
		val := testEval(t, tt.input)
		if val == nil {
			t.Errorf("Evaluation of %q returned nil", tt.input)
			continue
		}
		testBool(t, val, tt.expected)
	}

}

func TestIfStmt(t *testing.T) {
	tests := []struct {
		input    string
		expected interface{}
	}{
		{"if (true) { 10 }", 10},
		{"if (false) { 10 }", nil},
		{"if (1 < 2) { 10 }", 10},
		{"if (1 > 2) { 10 }", nil},
		{"if (1 > 2) { 10 } else { 20 }", 20},
		{"if (1 < 2) { 10 } else { 20 }", 10},
	}

	for _, tt := range tests {
		val := testEval(t, tt.input)
		switch expected := tt.expected.(type) {
		case int:
			testInt(t, val, int64(expected))
		default:
			if val != nil {
				t.Errorf("object is not nil. got=%T (%+v)", val, val)
			}
		}
	}
}

func TestReturnStmt(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"return 1;", 1},
		{"return 2; 9", 2},
		{"return 3*4;9", 12},
		{"9; return 5*6; 9", 30},
		{"if (true) { if (true) { return 7 } return 9 }", 7},
		// Root cause: `return` from inside a nested
		// loop body must bubble up through every enclosing
		// WhileStmt / DoWhileStmt / ForStmt instead of being
		// silently dropped and spinning the outer loop forever.
		{"var integer i := 0; while (true) { while (true) { return 11 } } return 22", 11},
		{"var integer i := 0; do { do { return 13 } while (true) } while (true) return 24", 13},
		{"var integer i := 0; for (i := 0; i < 100; i := i + 1) { for (var integer j := 0; j < 100; j := j + 1) { return 15 } }; return 26", 15},
		{"var integer i := 0; while (true) { for (var integer j := 0; j < 100; j := j + 1) { return 17 } }; return 28", 17},
	}

	for _, tt := range tests {
		val := testEval(t, tt.input)
		testInt(t, val, tt.expected)
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"if(1){}", "boolean expression expected. Got integer (1)"},
		{"-true", "unknown operator: -true"},
		{"true==1", "type mismatch: boolean == integer"},
		{"true+true", "unknown operator: boolean + boolean"},
		{"1&1", "unknown operator: integer & integer"},
		{`"a"+"b"`, "unknown operator: charstring + charstring"},
		{"x", "identifier not found: x"},
		{"break", "break or continue statements not allowed outside loops"},
		{"continue", "break or continue statements not allowed outside loops"},
	}

	for _, tt := range tests {
		val := testEval(t, tt.input)
		err, ok := val.(*runtime.Error)
		if !ok {
			t.Errorf("no error object returned. got=%T (%+v)", val, val)
			continue
		}
		if err.Error() != tt.expected {
			t.Errorf("wrong error message. got=%q, want=%q", err.Error(), tt.expected)
		}
	}
}

func TestVars(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"var integer x := 5; x", 5},
		{"var integer x := 5*5; x", 25},
		{"var integer x := 23; var integer y := x; y", 23},
		{"var integer x := 8; var integer y := x; var integer z := x+y+3; z", 19},
	}
	for _, tt := range tests {
		val := testEval(t, tt.input)
		testInt(t, val, tt.expected)
	}
}

func TestFunc(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"function f(integer x) { x }; f(5)", 5},
		{"function f(integer x) { return x }; f(6)", 6},
		{"function add(integer x, integer y) { return x+y }; add(1,2)", 3},
		{"function add(integer x, integer y) { return x+y }; add(1,add(2,4))", 7},
	}

	for _, tt := range tests {
		val := testEval(t, tt.input)
		testInt(t, val, tt.expected)
	}
}

func TestAssignment(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"var integer i; i := 2; i", 2},
		{"var integer i := 2; i := i + 1; i", 3},
	}
	for _, tt := range tests {
		val := testEval(t, tt.input)
		testInt(t, val, tt.expected)
	}
}

func TestLoop(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"var integer i := 2; while(false){i:=1};i", 2},
		{"var integer i := 0; while (i<3) {i := i + 1}; i", 3},
		{"var integer i := 1; do { i := 4 } while (false); i", 4},
		{"var integer i; for (i := 0; i < 3; i := i + 1) {}; i", 3},
		{"var integer x; for (var integer i := 0; i < 3; i := i + 1) {x:=i}; x", 2},
		{"var integer i := 5; while(true) { break; i := 2}; i", 5},
		{"var integer i := 1; while(true) { while(true) { break; i := 2} i:= 6; break}; i", 6},
		{"var integer x := 7; for (var integer i := 0; i< 3; i:= i + 1) {continue; x:=i}; x", 7},
		{"var integer x := 9; do { continue } while(false); x", 9},
		{"var integer i; for ( i:= 0; true; i := i + 1) {break}; i", 0},
	}
	for _, tt := range tests {
		val := testEval(t, tt.input)
		testInt(t, val, tt.expected)
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"Hello Wörld!"`, `Hello Wörld!`},
		{`"Hello" & " " & "World"`, `Hello World`},
	}
	for _, tt := range tests {
		val := testEval(t, tt.input)
		str, ok := val.(*runtime.String)
		if !ok {
			t.Errorf("object is not runtime.String. got=%T (%+v)", val, val)
			continue
		}
		if string(str.Value) != tt.expected {
			t.Errorf("object has wrong value. got=%s, want=%s", string(str.Value), tt.expected)
		}
	}
}

func TestBinarystring(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"'011'b", "'011'B"},
		{"not4b '01'b", "'10'B"},
		{"'0011'b and4b '0101'b", "'1'B"},
		{"'0011'b or4b  '0101'b", "'111'B"},
		{"'0011'b xor4b '0101'b", "'110'B"},
	}
	for _, tt := range tests {
		val := testEval(t, tt.input)
		str, ok := val.(*runtime.Binarystring)
		if !ok {
			t.Errorf("object is not runtime.Binarystring. got=%T (%+v)", val, val)
			continue
		}
		if str.Inspect() != tt.expected {
			t.Errorf("object has wrong value. got=%s, want=%s", str.Inspect(), tt.expected)
		}
	}
}

func TestBuiltinFunctions(t *testing.T) {
	tests := []struct {
		input    string
		expected interface{}
	}{
		{`lengthof("")`, `0`},
		{`lengthof("fnord")`, `5`},
		{`lengthof(1)`, runtime.Errorf("integer types have no length")},

		{`int2bit(1, 4)`, `'0001'B`},
		{`int2bit(4, 1)`, runtime.Errorf("4 value requires more than 1 bits")},
		{`int2bit(4, -1)`, runtime.Errorf("length must be greater or equal than zero")},
		{`int2bit(0, 0)`, `'0'B`},
		{`int2bit(1, 0)`, runtime.Errorf("1 value requires more than 0 bits")},
		{`int2bit(1, -1)`, runtime.Errorf("length must be greater or equal than zero")},
		{`int2bit(-1, 8)`, runtime.Errorf("integer invalue is less than zero")},
		{`int2bit(33569, 16)`, `'1000001100100001'B`},
		{`int2bit(1, 3)`, `'001'B`},
		{`int2bit(0, 2)`, `'00'B`},

		{`int2str(9223372036854775808)`, `"9223372036854775808"`},
		{`int2str(0)`, `"0"`},
		{`int2str(-9223372036854775809)`, `"-9223372036854775809"`},

		{`int2char(9223372036854775808)`, runtime.Errorf("Argument is out of range. Range is from 0 to 127. Int = 9223372036854775808")},
		{`int2char(128)`, runtime.Errorf("Argument is out of range. Range is from 0 to 127. Int = 128")},
		{`int2char(-1)`, runtime.Errorf("Argument is out of range. Range is from 0 to 127. Int = -1")},
		{`int2char(70)`, `"F"`},
		{`int2char(0)`, runtime.NewCharstring(fmt.Sprintf("%c", 0))},
		{`int2char(127)`, runtime.NewCharstring(fmt.Sprintf("%c", 127))},

		{`str2int("wrong")`, runtime.Errorf("invalid syntax: wrong")},
		{`str2int("2-3")`, runtime.Errorf("invalid syntax: 2-3")},
		{`str2int("150")`, runtime.NewInt("150")},
		{`str2int("-150")`, runtime.NewInt("-150")},

		{`str2float("wrong")`, runtime.Errorf("invalid syntax: wrong")},
		{`str2float("2-3")`, runtime.Errorf("invalid syntax: 2-3")},
		{`str2float("150")`, runtime.NewFloat("150")},
		{`str2float("-150")`, runtime.NewFloat("-150")},

		{`type enumerated E1 {red, green, blue}; var E1 testVar := blue; int2enum(0, testVar); testVar`, `E1.red(0)`},
		{`type enumerated E1 {red(10), green(0), blue(1)}; var E1 testVar := blue; int2enum(10, testVar); testVar`, `E1.red(10)`},
		{`type enumerated E1 {red(10..20), green, blue}; var E1 testVar := blue; int2enum(11, testVar); testVar`, `E1.red(11)`},
		{`type enumerated E1 {red, green, blue}; var E1 testVar := blue; int2enum(9223372036854775808, testVar)`, runtime.Errorf("integer value out of range (int64)")},
		{`type enumerated E1 {red, green, blue}; var E1 testVar := blue; int2enum(0, "wrong")`, runtime.Errorf("second argument must be an enum value")},
		{`type enumerated E1 {red, green, blue}; var E1 testVar := blue; int2enum(4, testVar)`, runtime.Errorf("id 4 does not exist in any ranges of Enum E1")},

		{`int2unichar(-1)`, runtime.Errorf("Argument is out of range. Range is from 0 to 2147483647. Int = -1")},
		{`int2unichar(2147483648)`, runtime.Errorf("Argument is out of range. Range is from 0 to 2147483647. Int = 2147483648")},
		{`int2unichar(9786)`, runtime.NewUniversalString("☺")},

		{`unichar2int("too long")`, runtime.Errorf("argument must be of length=1")},
		{`unichar2int("t")`, `116`},
		{`unichar2int("☺")`, `9786`},
	}
	for _, tt := range tests {
		t.Run(t.Name(), func(t *testing.T) {
			t.Logf("%s", tt.input)
			got := testEval(t, tt.input)
			if got == nil {
				t.Fatalf("testEval(%q) = nil", tt.input)
			}

			switch want := tt.expected.(type) {
			case string:
				assert.Equal(t, want, got.Inspect())

			case error:
				// TODO(5nord) replace message comparison with
				// proper error type comparison (errors.Is(e,
				// want))
				if e, ok := got.(error); !ok || e.Error() != want.Error() {
					t.Errorf("got %v, want %v", got, want)
				}

			case runtime.Object:
				if !got.Equal(want) {
					t.Errorf("objects not equal. got %v, want %v", got, want)
				}

			default:
				t.Fatalf("unsupported type %T", tt.expected)
			}

		})
	}
}

func TestList(t *testing.T) {
	input := "var integer a[3] := {1, 1+1, 3}; a"
	val := testEval(t, input)
	l, ok := val.(*runtime.List)
	if !ok {
		t.Errorf("object is not runtime.List. got=%T (%+v)", val, val)
		return
	}
	testInt(t, l.Elements[0], 1)
	testInt(t, l.Elements[1], 2)
	testInt(t, l.Elements[2], 3)
}

func TestIndexExpr(t *testing.T) {
	tests := []struct {
		input    string
		expected interface{}
	}{
		{"var integer a[3] := {1, 1+1, 3}; a[0] + a[1] + a[2]", 6},
		{"var integer a[3] := {1, 1+1, 3}; a[3]", nil},
		{"var integer a[3] := {1, 1+1, 3}; var integer i := 2; a[i]", 3},
		{"var integer x := {2,4,8}[1]; x", 4},
		{`var integer m := { ["foo"] := 23, [ 1+2 ] := 5}; m["foo"] + m[3]`, 28},
		{`var integer r := { x := 2, y := 3}; r.x+r.y`, 5},
	}
	for _, tt := range tests {
		val := testEval(t, tt.input)
		expected, ok := tt.expected.(int)
		if ok {
			testInt(t, val, int64(expected))
		} else {
			if val != runtime.Undefined {
				t.Errorf("object is not undefined. got=%T (%+v)", val, val)
			}
		}
	}
}

func TestEqual(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{`1==1`, true},
		{`2==1`, false},
		{`"one"=="one"`, true},
		{`"one"=="two"`, false},
		{`true==true`, true},
		{`false==false`, true},
		{`var RoI a:={1}, b:={1}; a==b`, true},
		{`var RoI a:={1,2}, b:={2,1}; a==b`, false},
		{`var RoI a:={}, b:={2,1}; a==b`, false},
		{`var RoI a:={}, b:={}; a==b`, true},
	}

	for _, tt := range tests {
		val := testEval(t, tt.input)
		testBool(t, val, tt.expected)
	}
}

func TestMapExpr(t *testing.T) {
	input := `var Map m := { ["foo"] := 1, ["bar"] := 2 }; m`
	val := testEval(t, input)
	m, ok := val.(*runtime.Map)
	if !ok {
		t.Errorf("object is not runtime.Map. got=%T (%+v)", val, val)
		return
	}

	tests := []struct {
		key      runtime.Object
		expected int64
	}{
		{&runtime.String{Value: []rune("foo")}, 1},
		{&runtime.String{Value: []rune("bar")}, 2},
	}
	for _, tt := range tests {
		val, ok := m.Get(tt.key)
		if !ok {
			t.Errorf("key %s not found", tt.key.(runtime.Object).Inspect())
			continue
		}
		testInt(t, val, tt.expected)
	}
}

func testEval(t *testing.T, input string) runtime.Object {
	nodes := syntax.Parse([]byte(input))
	if err := nodes.Err(); err != nil {
		t.Fatalf("%s\n %s", input, err.Error())
	}
	return interpreter.Eval(nodes, runtime.NewEnv(nil))
}

func testInt(t *testing.T, obj runtime.Object, expected int64) bool {
	i, ok := obj.(runtime.Int)
	if !ok {
		t.Errorf("object is not runtime.Int. got=%T (%+v)", obj, obj)
		return false
	}

	if !i.IsInt64() {
		t.Errorf("object is to big to compare. got=%s", i)
		return false
	}

	if val := i.Int64(); val != expected {
		t.Errorf("object has wrong value. got=%d, want=%d", val, expected)
		return false
	}

	return true
}

func testBool(t *testing.T, obj runtime.Object, expected bool) bool {
	b, ok := obj.(runtime.Bool)
	if !ok {
		t.Errorf("object is not runtime.Bool. got=%T (%+v)", obj, obj)
		return false
	}

	if val := b.Bool(); val != expected {
		t.Errorf("object has wrong value. got=%t, want=%t", val, expected)
		return false
	}

	return true
}

func TestEnums(t *testing.T) {

	NewEnumTypeWithIds := func(enumTypeName string, elements ...interface{}) *runtime.EnumType {
		ret := runtime.NewEnumType(enumTypeName)
		for i := 0; i < len(elements); i += 2 {
			eKeyName, _ := elements[i+0].(string)
			eKeyRanges, _ := elements[i+1].([]runtime.EnumRange)
			ret.Elements[eKeyName] = eKeyRanges
		}
		return ret
	}

	NewEnumValue := func(enumTypeKey string, enumTypeValue int, et *runtime.EnumType) *runtime.EnumValue {
		ret, _ := runtime.NewEnumValue(
			et,
			enumTypeKey,
			enumTypeValue)
		return ret
	}

	tests := []struct {
		input    string
		expected runtime.Object
	}{
		{"type enumerated E1 {};", runtime.Errorf("this enum has no elements")},
		{"type enumerated E2 {red}; E2", runtime.NewEnumType(
			"E2",
			"red")},
		{"type enumerated E3 {red, green, blue}; E3", runtime.NewEnumType(
			"E3",
			"red", "green", "blue")},
		{"type enumerated E4 {red(1,2)}; E4", NewEnumTypeWithIds(
			"E4",
			"red", []runtime.EnumRange{{First: 1, Last: 1}, {First: 2, Last: 2}})},
		{"type enumerated E5 {red(\"1\")};", runtime.Errorf("can't add key red, enum element has unexpected CallExpr argument, ValueLiteral unexpected STRING")},
		{"type enumerated E6 {red(-1)}; E6", NewEnumTypeWithIds(
			"E6",
			"red", []runtime.EnumRange{{First: -1, Last: -1}})},
		{"type enumerated E7 {red(1),blue(1)}; E7", NewEnumTypeWithIds(
			"E7",
			"red", []runtime.EnumRange{{First: 1, Last: 1}})},
		{"type enumerated E8 {red(1),blue(2)}; E8", NewEnumTypeWithIds(
			"E8",
			"red", []runtime.EnumRange{{First: 1, Last: 1}},
			"blue", []runtime.EnumRange{{First: 2, Last: 2}})},
		// TTCN-3 6.2.4 (v4.15.1) clause e: with mixed
		// implicit/explicit numbering each implicit value is
		// the *lowest non-negative integer not yet associated
		// with another identifier*. So green takes 1 (the
		// first free slot after red=0), not 11 (the
		// previous-plus-one heuristic from older editions).
		{"type enumerated E9 {red,blue(10),green}; E9", NewEnumTypeWithIds(
			"E9",
			"red", []runtime.EnumRange{{First: 0, Last: 0}},
			"blue", []runtime.EnumRange{{First: 10, Last: 10}},
			"green", []runtime.EnumRange{{First: 1, Last: 1}})},
		{"type enumerated EA {red(-4..0),green,blue(-3)}; EA", NewEnumTypeWithIds(
			"EA",
			"red", []runtime.EnumRange{{First: -4, Last: 0}},
			"green", []runtime.EnumRange{{First: 1, Last: 1}})},
		{"type enumerated EA {red(-4..0),green(1,2),blue(3..4)}; EA", NewEnumTypeWithIds(
			"EA",
			"red", []runtime.EnumRange{{First: -4, Last: 0}},
			"green", []runtime.EnumRange{{First: 1, Last: 1}, {First: 2, Last: 2}},
			"blue", []runtime.EnumRange{{First: 3, Last: 4}})},

		//TODO:
		//{"type record of enumerated { E } RoE", ??},

		{
			"type enumerated EB {red(0..10)}; var EB v := red(1); v;",
			NewEnumValue("red", 1,
				NewEnumTypeWithIds("EB", "red", []runtime.EnumRange{{First: 0, Last: 10}}),
			),
		},
		{
			"type enumerated EC {red(0..10), blue}; var EC v := blue; v;",
			NewEnumValue("blue", 11,
				NewEnumTypeWithIds("EC",
					"red", []runtime.EnumRange{{First: 0, Last: 10}},
					"blue", []runtime.EnumRange{{First: 11, Last: 11}}),
			),
		},
		{
			"type record of enumerated { red } ED; ED;",
			runtime.NewRecordOf(
				NewEnumTypeWithIds("",
					"red", []runtime.EnumRange{{First: 0, Last: 0}},
				)),
		},
		{
			"type set of enumerated { blue } EE; EE;",
			runtime.NewSetOf(
				NewEnumTypeWithIds("",
					"blue", []runtime.EnumRange{{First: 0, Last: 0}},
				)),
		},
	}
	for _, tt := range tests {

		val := testEval(t, tt.input)

		switch expected := tt.expected.(type) {
		case *runtime.Error:
			err, ok := val.(*runtime.Error)
			if !ok {
				t.Errorf("\n%s\nobject is not runtime.Error.\ngot=%+v(%T)\nwant=%s", tt.input, val, val, expected.Error())
				continue
			}
			if err.Error() != expected.Error() {
				t.Errorf("\n%s\nwrong error message.\ngot=%q,\nwant=%s", tt.input, err.Error(), expected.Error())
			}
		case *runtime.EnumType:
			if !tt.expected.Equal(val) {
				t.Errorf("\n%s\ngot=%v,\nwant=%v", tt.input, val, expected)
			}
		case *runtime.EnumValue:
			if !tt.expected.Equal(val) {
				t.Errorf("\n%s\ngot=%v,\nwant=%v", tt.input, val, expected)
			}
		case *runtime.List:
			if !tt.expected.Equal(val) {
				t.Errorf("\n%s\ngot=%v,\nwant=%v", tt.input, val, expected)
			}
		default:
			t.Errorf("\n%s\ntest error, unhandeled type:%T", tt.input, expected)
		}
	}
}
