package vm

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/nokia/ntt/internal/k3log"
	"github.com/nokia/ntt/internal/runtime"
	"github.com/nokia/ntt/internal/t3xf"
	"github.com/nokia/ntt/internal/t3xf/opcode"
	"io"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestVariables(t *testing.T) {
	t.Run("assign globals", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "x"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 23},
			{Op: opcode.REF, Arg: 2},
			{Op: opcode.ASSIGN},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		m := testMachine(t, input, &Machine{})
		assert.Equal(t, 23, m.Globals[0])
	})

	t.Run("read globals", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "x"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.REF, Arg: 2},
			{Op: opcode.REF, Arg: 2},
			{Op: opcode.ADD},
			{Op: opcode.REF, Arg: 2},
			{Op: opcode.ASSIGN},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		m := testMachine(t, input, &Machine{
			Globals: []any{23},
		})
		assert.Equal(t, 46, m.Globals[0])
	})

	t.Run("assign from ref", func(t *testing.T) {
		// x := y — RHS is ref (value), LHS is ref (target), assign must deref RHS.
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "y"},
			{Op: opcode.VAR},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "x"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.REF, Arg: 2},
			{Op: opcode.REF, Arg: 5},
			{Op: opcode.ASSIGN},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		m, err := testMachineWithError(t, input, &Machine{
			Globals: []any{23},
		})
		assert.NoError(t, err)
		assert.Equal(t, 23, m.Globals[0])
		assert.Equal(t, 23, m.Globals[1])
	})

	t.Run("self assign", func(t *testing.T) {
		// x := x — assign must store the integer value, not *runtime.Var.
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "x"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.REF, Arg: 2},
			{Op: opcode.REF, Arg: 2},
			{Op: opcode.ASSIGN},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		m, err := testMachineWithError(t, input, &Machine{
			Globals: []any{23},
		})
		assert.NoError(t, err)
		assert.Equal(t, 23, m.Globals[0])
		_, isVar := m.Globals[0].(*runtime.Var)
		assert.False(t, isVar, "self-assign must store int value, not *runtime.Var")
	})

	t.Run("array element assignment", func(t *testing.T) {
		// Test: arr[0] = 42
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.RECORDOF},
			{Op: opcode.NAME, Arg: "arr"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 42},
			{Op: opcode.REF, Arg: 3},
			{Op: opcode.NATLONG, Arg: 0},
			{Op: opcode.DEF},
			{Op: opcode.ASSIGN},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		m := testMachine(t, input, &Machine{
			Globals: []any{
				[]any{0, 0, 0},
			},
		})
		arr := m.Globals[0].([]any)
		assert.Equal(t, 42, arr[0])
	})

	t.Run("array grow on assign", func(t *testing.T) {
		// Uninitialized record-of: arr[2] := 42 expands storage and persists in the var.
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.RECORDOF},
			{Op: opcode.NAME, Arg: "arr"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 42},
			{Op: opcode.REF, Arg: 3},
			{Op: opcode.NATLONG, Arg: 2},
			{Op: opcode.DEF},
			{Op: opcode.ASSIGN},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		m, err := testMachineWithError(t, input, &Machine{})
		assert.NoError(t, err)
		arr := m.Globals[0].([]any)
		assert.Len(t, arr, 3)
		assert.Equal(t, 42, arr[2])
	})

	t.Run("iget read without expand", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.RECORDOF},
			{Op: opcode.NAME, Arg: "arr"},
			{Op: opcode.VAR},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "elem"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.REF, Arg: 3},
			{Op: opcode.IGET, Arg: 0},
			{Op: opcode.REF, Arg: 6},
			{Op: opcode.ASSIGN},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		m, err := testMachineWithError(t, input, &Machine{})
		assert.ErrorContains(t, err, "UNDF")
		assert.Equal(t, runtime.Undefined{}, m.Globals[0])
		assert.Equal(t, runtime.Undefined{}, m.Globals[1])
	})

	t.Run("iget out of range without expand", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.RECORDOF},
			{Op: opcode.NAME, Arg: "arr"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.REF, Arg: 3},
			{Op: opcode.IGET, Arg: 10},
			{Op: opcode.DROP},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		m, err := testMachineWithError(t, input, &Machine{
			Globals: []any{[]any{1, 2, 3}},
		})
		assert.ErrorContains(t, err, "ARRV")
		arr := m.Globals[0].([]any)
		assert.Len(t, arr, 3)
	})

	t.Run("idef assign expands", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.RECORDOF},
			{Op: opcode.NAME, Arg: "arr"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 42},
			{Op: opcode.REF, Arg: 3},
			{Op: opcode.IDEF, Arg: 2},
			{Op: opcode.ASSIGN},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		m, err := testMachineWithError(t, input, &Machine{})
		assert.NoError(t, err)
		arr := m.Globals[0].([]any)
		assert.Len(t, arr, 3)
		assert.Equal(t, 42, arr[2])
	})

	t.Run("nested array element assignment", func(t *testing.T) {
		// Test: nested[0][1] = 99
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.RECORDOF},
			{Op: opcode.NAME, Arg: "nested"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 99},
			{Op: opcode.REF, Arg: 3},
			{Op: opcode.NATLONG, Arg: 0},
			{Op: opcode.DEF},
			{Op: opcode.NATLONG, Arg: 1},
			{Op: opcode.DEF},
			{Op: opcode.ASSIGN},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		m := testMachine(t, input, &Machine{
			Globals: []any{
				[]any{
					[]any{10, 20, 30},
					[]any{40, 50, 60},
				},
			},
		})
		nested := m.Globals[0].([]any)
		inner := nested[0].([]any)
		assert.Equal(t, 99, inner[1])
	})

	t.Run("three-level array element assignment", func(t *testing.T) {
		// a[0][0][5] := 42
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.RECORDOF},
			{Op: opcode.NAME, Arg: "a"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 42},
			{Op: opcode.REF, Arg: 3},
			{Op: opcode.NATLONG, Arg: 0},
			{Op: opcode.DEF},
			{Op: opcode.NATLONG, Arg: 0},
			{Op: opcode.DEF},
			{Op: opcode.NATLONG, Arg: 5},
			{Op: opcode.DEF},
			{Op: opcode.ASSIGN},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		m := testMachine(t, input, &Machine{})
		a := m.Globals[0].([]any)[0].([]any)[0].([]any)
		assert.Len(t, a, 6)
		assert.Equal(t, 42, a[5])
	})

	t.Run("read array element", func(t *testing.T) {
		// Test: result = arr[1]
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.RECORDOF},
			{Op: opcode.NAME, Arg: "arr"},
			{Op: opcode.VAR},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "result"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.REF, Arg: 3},
			{Op: opcode.NATLONG, Arg: 1},
			{Op: opcode.GET},
			{Op: opcode.REF, Arg: 6},
			{Op: opcode.ASSIGN},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		m := testMachine(t, input, &Machine{
			Globals: []any{
				[]any{10, 20, 30},
			},
		})
		assert.Equal(t, 20, m.Globals[1])
	})

	t.Run("read nested array element", func(t *testing.T) {
		// Test: result = nested[1][2]
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.RECORDOF},
			{Op: opcode.NAME, Arg: "nested"},
			{Op: opcode.VAR},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "result"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.REF, Arg: 3},
			{Op: opcode.NATLONG, Arg: 1},
			{Op: opcode.GET},
			{Op: opcode.NATLONG, Arg: 2},
			{Op: opcode.GET},
			{Op: opcode.REF, Arg: 6},
			{Op: opcode.ASSIGN},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		m := testMachine(t, input, &Machine{
			Globals: []any{
				[]any{
					[]any{10, 20, 30},
					[]any{40, 50, 60},
				},
			},
		})
		assert.Equal(t, 60, m.Globals[1])
	})

	t.Run("type check - assign boolean to integer", func(t *testing.T) {
		// Test: assigning true (boolean) to an integer variable should fail
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "x"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.TRUE},
			{Op: opcode.REF, Arg: 2},
			{Op: opcode.ASSIGN},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		_, err := testMachineWithError(t, input, &Machine{})
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "type mismatch")
		var vmErr *Error
		if assert.ErrorAs(t, err, &vmErr) {
			assert.Equal(t, 0x20, vmErr.PC)
		}
	})

	t.Run("type check - assign integer to boolean", func(t *testing.T) {
		// Test: assigning 42 (integer) to a boolean variable should fail
		input := []t3xf.Instruction{
			{Op: opcode.BOOLEAN},
			{Op: opcode.NAME, Arg: "b"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 42},
			{Op: opcode.REF, Arg: 2},
			{Op: opcode.ASSIGN},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		_, err := testMachineWithError(t, input, &Machine{})
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "type mismatch")
		var vmErr *Error
		if assert.ErrorAs(t, err, &vmErr) {
			assert.Equal(t, 0x24, vmErr.PC)
		}
	})

	t.Run("type check - assign integer to integer", func(t *testing.T) {
		// Test: assigning 42 (integer) to an integer variable should succeed
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "x"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 42},
			{Op: opcode.REF, Arg: 2},
			{Op: opcode.ASSIGN},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		m := testMachine(t, input, &Machine{})
		assert.Equal(t, 42, m.Globals[0])
	})

	t.Run("sub from refs", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "x"},
			{Op: opcode.VAR},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "y"},
			{Op: opcode.VAR},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "z"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.REF, Arg: 2},
			{Op: opcode.REF, Arg: 5},
			{Op: opcode.SUB},
			{Op: opcode.REF, Arg: 8},
			{Op: opcode.ASSIGN},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		m, err := testMachineWithError(t, input, &Machine{
			Globals: []any{10, 3},
		})
		assert.NoError(t, err)
		assert.Equal(t, 7, m.Globals[2])
	})

	t.Run("eq refs", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "x"},
			{Op: opcode.VAR},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "y"},
			{Op: opcode.VAR},
			{Op: opcode.BOOLEAN},
			{Op: opcode.NAME, Arg: "same"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.REF, Arg: 2},
			{Op: opcode.REF, Arg: 5},
			{Op: opcode.EQ},
			{Op: opcode.REF, Arg: 8},
			{Op: opcode.ASSIGN},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		m, err := testMachineWithError(t, input, &Machine{
			Globals: []any{5, 5},
		})
		assert.NoError(t, err)
		assert.Equal(t, true, m.Globals[2])
	})

	t.Run("copy-on-write shared ref", func(t *testing.T) {
		// b := a shares storage; b[1] := 99 unshares b only.
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.RECORDOF},
			{Op: opcode.NAME, Arg: "a"},
			{Op: opcode.VAR},
			{Op: opcode.INTEGER},
			{Op: opcode.RECORDOF},
			{Op: opcode.NAME, Arg: "b"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.REF, Arg: 3},
			{Op: opcode.REF, Arg: 7},
			{Op: opcode.ASSIGN},
			{Op: opcode.NATLONG, Arg: 99},
			{Op: opcode.REF, Arg: 7},
			{Op: opcode.IDEF, Arg: 1},
			{Op: opcode.ASSIGN},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		m, err := testMachineWithError(t, input, &Machine{
			Globals: []any{[]any{10, 20, 30}},
		})
		assert.NoError(t, err)
		a := m.Globals[0].([]any)
		b := m.Globals[1].([]any)
		assert.Equal(t, 20, a[1])
		assert.Equal(t, 99, b[1])
		assert.Equal(t, 10, a[0])
		assert.Equal(t, 10, b[0])
	})

	t.Run("three-level copy-on-write", func(t *testing.T) {
		// b := a; b[0][0][1] := 99; b[0][0][5] := 42 leaves a unchanged.
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.RECORDOF},
			{Op: opcode.NAME, Arg: "a"},
			{Op: opcode.VAR},
			{Op: opcode.INTEGER},
			{Op: opcode.RECORDOF},
			{Op: opcode.NAME, Arg: "b"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.REF, Arg: 3},
			{Op: opcode.REF, Arg: 7},
			{Op: opcode.ASSIGN},
			{Op: opcode.NATLONG, Arg: 99},
			{Op: opcode.REF, Arg: 7},
			{Op: opcode.IDEF, Arg: 0},
			{Op: opcode.IDEF, Arg: 0},
			{Op: opcode.IDEF, Arg: 1},
			{Op: opcode.ASSIGN},
			{Op: opcode.NATLONG, Arg: 42},
			{Op: opcode.REF, Arg: 7},
			{Op: opcode.IDEF, Arg: 0},
			{Op: opcode.IDEF, Arg: 0},
			{Op: opcode.IDEF, Arg: 5},
			{Op: opcode.ASSIGN},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		m, err := testMachineWithError(t, input, &Machine{
			Globals: []any{[]any{[]any{[]any{1, 2, 3}}}},
		})
		assert.NoError(t, err)
		a := m.Globals[0].([]any)[0].([]any)[0].([]any)
		b := m.Globals[1].([]any)[0].([]any)[0].([]any)
		assert.Equal(t, []any{1, 2, 3}, a)
		assert.Len(t, b, 6)
		assert.Equal(t, 99, b[1])
		assert.Equal(t, 42, b[5])
	})
}

func TestFlowControl(t *testing.T) {
	t.Parallel()

	t.Run("jumps", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.NATLONG, Arg: 0},
			{Op: opcode.NATLONG, Arg: 1},
			{Op: opcode.ADD},
			{Op: opcode.DUP},
			{Op: opcode.NATLONG, Arg: 2},
			{Op: opcode.NATLONG, Arg: 2},
			{Op: opcode.ADD},
			{Op: opcode.LT},
			{Op: opcode.JNT, Arg: 4},
			{Op: opcode.GOTO, Arg: 1},
			{Op: opcode.RETURN},
		}
		m := testMachine(t, input, &Machine{})
		assert.Equal(t, 4, popResult(m))
	})

	t.Run("if-true", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.TRUE},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 1},
			{Op: opcode.BLOCK},
			{Op: opcode.IF},
			{Op: opcode.RETURN},
		}
		m := testMachine(t, input, &Machine{})
		assert.Equal(t, 1, popResult(m))
	})

	t.Run("if-false", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.FALSE},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 1},
			{Op: opcode.BLOCK},
			{Op: opcode.IF},
			{Op: opcode.RETURN},
		}
		m := testMachine(t, input, &Machine{})
		assert.True(t, m.SP == 0)
	})

	t.Run("if-true-empty", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.TRUE},
			{Op: opcode.SKIP},
			{Op: opcode.IF},
			{Op: opcode.RETURN},
		}
		m := testMachine(t, input, &Machine{})
		assert.True(t, m.SP == 0)
	})

	t.Run("if-false-empty", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.FALSE},
			{Op: opcode.SKIP},
			{Op: opcode.IF},
			{Op: opcode.RETURN},
		}
		m := testMachine(t, input, &Machine{})
		assert.True(t, m.SP == 0)
	})

	t.Run("if-true-else", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.TRUE},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 1},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 2},
			{Op: opcode.BLOCK},
			{Op: opcode.IFELSE},
			{Op: opcode.RETURN},
		}
		m := testMachine(t, input, &Machine{})
		assert.Equal(t, 1, popResult(m))
	})

	t.Run("if-false-else", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.FALSE},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 1},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 2},
			{Op: opcode.BLOCK},
			{Op: opcode.IFELSE},
			{Op: opcode.RETURN},
		}
		m := testMachine(t, input, &Machine{})
		assert.Equal(t, 2, popResult(m))
	})

	t.Run("if-true-else-empty", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.TRUE},
			{Op: opcode.SKIP},
			{Op: opcode.SKIP},
			{Op: opcode.IFELSE},
			{Op: opcode.RETURN},
		}
		m := testMachine(t, input, &Machine{})
		assert.True(t, m.SP == 0)
	})

	t.Run("if-false-else-empty", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.FALSE},
			{Op: opcode.SKIP},
			{Op: opcode.SKIP},
			{Op: opcode.IFELSE},
			{Op: opcode.RETURN},
		}
		m := testMachine(t, input, &Machine{})
		assert.True(t, m.SP == 0)
	})

	t.Run("while", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.NATLONG, Arg: 0},
			{Op: opcode.SCAN},
			{Op: opcode.DUP},
			{Op: opcode.NATLONG, Arg: 3},
			{Op: opcode.LT},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 1},
			{Op: opcode.ADD},
			{Op: opcode.BLOCK},
			{Op: opcode.WHILE},
			{Op: opcode.RETURN},
		}
		m := testMachine(t, input, &Machine{})
		assert.Equal(t, 3, popResult(m))
	})

	t.Run("while-empty", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.SCAN},
			{Op: opcode.FALSE},
			{Op: opcode.BLOCK},
			{Op: opcode.SKIP},
			{Op: opcode.WHILE},
			{Op: opcode.RETURN},
		}
		m := testMachine(t, input, &Machine{})
		assert.True(t, m.SP == 0)
	})

	t.Run("for-loop", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 0},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.DUP},
			{Op: opcode.NATLONG, Arg: 3},
			{Op: opcode.LT},
			{Op: opcode.BLOCK},
			{Op: opcode.SKIP},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 1},
			{Op: opcode.ADD},
			{Op: opcode.BLOCK},
			{Op: opcode.FOR},
			{Op: opcode.RETURN},
		}
		m := testMachine(t, input, &Machine{})
		assert.Equal(t, 3, popResult(m))
	})

	t.Run("do-while", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.NATLONG, Arg: 0},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 1},
			{Op: opcode.ADD},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.DUP},
			{Op: opcode.NATLONG, Arg: 3},
			{Op: opcode.LT},
			{Op: opcode.BLOCK},
			{Op: opcode.DOWHILE},
			{Op: opcode.RETURN},
		}
		m := testMachine(t, input, &Machine{})
		assert.Equal(t, 3, popResult(m))
	})

	t.Run("jt/goto", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.NATLONG, Arg: 0},
			{Op: opcode.NATLONG, Arg: 1},
			{Op: opcode.ADD},
			{Op: opcode.DUP},
			{Op: opcode.NATLONG, Arg: 2},
			{Op: opcode.NATLONG, Arg: 2},
			{Op: opcode.ADD},
			{Op: opcode.LT},
			{Op: opcode.JNT, Arg: 4},
			{Op: opcode.GOTO, Arg: 1},
			{Op: opcode.RETURN},
		}
		m := testMachine(t, input, &Machine{})
		assert.Equal(t, 4, popResult(m))
	})
}

func TestFunctions(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.SKIP},
			{Op: opcode.SKIP},
			{Op: opcode.NAME, Arg: "f"},
			{Op: opcode.FUNCTION},
			{Op: opcode.SCAN},
			{Op: opcode.REF, Arg: 3},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		testMachine(t, input, &Machine{})
	})

	t.Run("call", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.SKIP},
			{Op: opcode.SCAN},
			{Op: opcode.BLOCK},
			{Op: opcode.NAME, Arg: "f"},
			{Op: opcode.FUNCTION},
			{Op: opcode.SCAN},
			{Op: opcode.REF, Arg: 4},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		m := testMachine(t, input, &Machine{})
		assert.Empty(t, m.Frames)
	})

	t.Run("return", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.SKIP},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 23},
			{Op: opcode.BLOCK},
			{Op: opcode.NAME, Arg: "f"},
			{Op: opcode.FUNCTIONV},
			{Op: opcode.SCAN},
			{Op: opcode.REF, Arg: 6},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		m := testMachine(t, input, &Machine{})
		assert.Equal(t, 23, popResult(m))
	})

	t.Run("no return", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.SKIP},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 23},
			{Op: opcode.BLOCK},
			{Op: opcode.NAME, Arg: "f"},
			{Op: opcode.FUNCTION},
			{Op: opcode.SCAN},
			{Op: opcode.REF, Arg: 5},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		m := testMachine(t, input, &Machine{})
		assert.Equal(t, 23, m.Stack[0], "function was not called")
		assert.True(t, m.SP == 0, "stack must be empty")
	})

	t.Run("parameters", func(t *testing.T) {
		const (
			placeP = t3xf.Reference(0x100)
			placeF = t3xf.Reference(0x200)
			placeV = t3xf.Reference(0x300)
			placeW = t3xf.Reference(0x400)
		)
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.SCAN},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "p"},
			{Op: opcode.IN},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 1},
			{Op: opcode.REF, Arg: placeP},
			{Op: opcode.ADD},
			{Op: opcode.REF, Arg: placeP},
			{Op: opcode.ASSIGN},
			{Op: opcode.REF, Arg: placeP},
			{Op: opcode.RETURN},
			{Op: opcode.BLOCK},
			{Op: opcode.NAME, Arg: "f"},
			{Op: opcode.FUNCTIONV},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "v"},
			{Op: opcode.VAR},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "w"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 1},
			{Op: opcode.REF, Arg: placeV},
			{Op: opcode.ASSIGN},
			{Op: opcode.REF, Arg: placeV},
			{Op: opcode.REF, Arg: placeF},
			{Op: opcode.REF, Arg: placeW},
			{Op: opcode.ASSIGN},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		l := mustLoad(t, mustAssemble(t, input))
		pRef := tagRef(t, l, func(o any) bool {
			v, ok := o.(*runtime.Var)
			return ok && v.Name == "p" && v.Flags&runtime.Parameter != 0
		})
		fRef := tagRef(t, l, func(o any) bool {
			f, ok := o.(*runtime.Function)
			return ok && f.Name == "f"
		})
		vRef := tagRef(t, l, func(o any) bool {
			v, ok := o.(*runtime.Var)
			return ok && v.Name == "v"
		})
		wRef := tagRef(t, l, func(o any) bool {
			v, ok := o.(*runtime.Var)
			return ok && v.Name == "w"
		})
		for i := range input {
			if input[i].Op != opcode.REF {
				continue
			}
			switch input[i].Arg {
			case placeP:
				input[i].Arg = pRef
			case placeF:
				input[i].Arg = fRef
			case placeV:
				input[i].Arg = vRef
			case placeW:
				input[i].Arg = wRef
			}
		}

		m := testMachine(t, input, &Machine{})
		assert.Equal(t, 1, m.Globals[0])
		assert.Equal(t, 2, m.Globals[1])
	})

	t.Run("out parameter copy back", func(t *testing.T) {
		const (
			placeX = t3xf.Reference(0x100)
			placeY = t3xf.Reference(0x200)
			placeF = t3xf.Reference(0x300)
			placeA = t3xf.Reference(0x400)
			placeB = t3xf.Reference(0x500)
		)
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.SCAN},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "x"},
			{Op: opcode.OUT},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "y"},
			{Op: opcode.OUT},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 11},
			{Op: opcode.REF, Arg: placeX},
			{Op: opcode.ASSIGN},
			{Op: opcode.RETURN},
			{Op: opcode.BLOCK},
			{Op: opcode.NAME, Arg: "f"},
			{Op: opcode.FUNCTION},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "a"},
			{Op: opcode.VAR},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "b"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 1},
			{Op: opcode.REF, Arg: placeA},
			{Op: opcode.ASSIGN},
			{Op: opcode.NATLONG, Arg: 2},
			{Op: opcode.REF, Arg: placeB},
			{Op: opcode.ASSIGN},
			{Op: opcode.REF, Arg: placeA},
			{Op: opcode.REF, Arg: placeB},
			{Op: opcode.REF, Arg: placeF},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		l := mustLoad(t, mustAssemble(t, input))
		xRef := tagRef(t, l, func(o any) bool {
			v, ok := o.(*runtime.Var)
			return ok && v.Name == "x" && v.Flags&runtime.Out != 0
		})
		fRef := tagRef(t, l, func(o any) bool {
			f, ok := o.(*runtime.Function)
			return ok && f.Name == "f"
		})
		aRef := tagRef(t, l, func(o any) bool {
			v, ok := o.(*runtime.Var)
			return ok && v.Name == "a" && v.Flags&runtime.Parameter == 0
		})
		bRef := tagRef(t, l, func(o any) bool {
			v, ok := o.(*runtime.Var)
			return ok && v.Name == "b" && v.Flags&runtime.Parameter == 0
		})
		for i := range input {
			if input[i].Op != opcode.REF {
				continue
			}
			switch input[i].Arg {
			case placeX:
				input[i].Arg = xRef
			case placeF:
				input[i].Arg = fRef
			case placeA:
				input[i].Arg = aRef
			case placeB:
				input[i].Arg = bRef
			}
		}

		m := testMachine(t, input, &Machine{})
		assert.Equal(t, 11, m.Globals[0])
		assert.Equal(t, 2, m.Globals[1])
	})

	t.Run("out untouched", func(t *testing.T) {
		const (
			placeY = t3xf.Reference(0x100)
			placeG = t3xf.Reference(0x200)
			placeC = t3xf.Reference(0x300)
		)
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.SCAN},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "y"},
			{Op: opcode.OUT},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.RETURN},
			{Op: opcode.BLOCK},
			{Op: opcode.NAME, Arg: "g"},
			{Op: opcode.FUNCTION},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "c"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 2},
			{Op: opcode.REF, Arg: placeC},
			{Op: opcode.ASSIGN},
			{Op: opcode.REF, Arg: placeC},
			{Op: opcode.REF, Arg: placeG},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		l := mustLoad(t, mustAssemble(t, input))
		yRef := tagRef(t, l, func(o any) bool {
			v, ok := o.(*runtime.Var)
			return ok && v.Name == "y" && v.Flags&runtime.Out != 0
		})
		gRef := tagRef(t, l, func(o any) bool {
			f, ok := o.(*runtime.Function)
			return ok && f.Name == "g"
		})
		cRef := tagRef(t, l, func(o any) bool {
			v, ok := o.(*runtime.Var)
			return ok && v.Name == "c" && v.Flags&runtime.Parameter == 0
		})
		for i := range input {
			if input[i].Op != opcode.REF {
				continue
			}
			switch input[i].Arg {
			case placeY:
				input[i].Arg = yRef
			case placeG:
				input[i].Arg = gRef
			case placeC:
				input[i].Arg = cRef
			}
		}

		m := testMachine(t, input, &Machine{})
		assert.Equal(t, 2, m.Globals[0])
	})

	t.Run("inout assign-through", func(t *testing.T) {
		const (
			placeP = t3xf.Reference(0x100)
			placeF = t3xf.Reference(0x200)
			placeV = t3xf.Reference(0x300)
		)
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.SCAN},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "p"},
			{Op: opcode.INOUT},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 1},
			{Op: opcode.REF, Arg: placeP},
			{Op: opcode.ADD},
			{Op: opcode.REF, Arg: placeP},
			{Op: opcode.ASSIGN},
			{Op: opcode.REF, Arg: placeP},
			{Op: opcode.RETURN},
			{Op: opcode.BLOCK},
			{Op: opcode.NAME, Arg: "f"},
			{Op: opcode.FUNCTIONV},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "v"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 1},
			{Op: opcode.REF, Arg: placeV},
			{Op: opcode.ASSIGN},
			{Op: opcode.REF, Arg: placeV},
			{Op: opcode.REF, Arg: placeF},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		l := mustLoad(t, mustAssemble(t, input))
		pRef := tagRef(t, l, func(o any) bool {
			v, ok := o.(*runtime.Var)
			return ok && v.Name == "p" && v.Flags&runtime.Inout != 0
		})
		fRef := tagRef(t, l, func(o any) bool {
			f, ok := o.(*runtime.Function)
			return ok && f.Name == "f"
		})
		vRef := tagRef(t, l, func(o any) bool {
			v, ok := o.(*runtime.Var)
			return ok && v.Name == "v"
		})
		for i := range input {
			if input[i].Op != opcode.REF {
				continue
			}
			switch input[i].Arg {
			case placeP:
				input[i].Arg = pRef
			case placeF:
				input[i].Arg = fRef
			case placeV:
				input[i].Arg = vRef
			}
		}

		m := testMachine(t, input, &Machine{})
		assert.Equal(t, 2, m.Globals[0], "inout assign must update caller variable")
	})

	t.Run("inout compound re-resolve after expand", func(t *testing.T) {
		const expandIdx = 5
		const (
			placeA  = t3xf.Reference(0x100)
			placeB  = t3xf.Reference(0x200)
			placeF  = t3xf.Reference(0x300)
			placeGA = t3xf.Reference(0x400)
		)
		input := []t3xf.Instruction{
			{Op: opcode.SCAN},
			{Op: opcode.INTEGER},
			{Op: opcode.RECORDOF},
			{Op: opcode.NAME, Arg: "a"},
			{Op: opcode.INOUT},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "b"},
			{Op: opcode.INOUT},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 99},
			{Op: opcode.REF, Arg: placeA},
			{Op: opcode.IDEF, Arg: expandIdx},
			{Op: opcode.ASSIGN},
			{Op: opcode.NATLONG, Arg: 11},
			{Op: opcode.REF, Arg: placeB},
			{Op: opcode.ASSIGN},
			{Op: opcode.RETURN},
			{Op: opcode.BLOCK},
			{Op: opcode.NAME, Arg: "f"},
			{Op: opcode.FUNCTION},
			{Op: opcode.INTEGER},
			{Op: opcode.RECORDOF},
			{Op: opcode.NAME, Arg: "a"},
			{Op: opcode.VAR},
			{Op: opcode.SCAN},
			{Op: opcode.REF, Arg: placeGA},
			{Op: opcode.REF, Arg: placeGA},
			{Op: opcode.IDEF, Arg: 0},
			{Op: opcode.REF, Arg: placeF},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		l := mustLoad(t, mustAssemble(t, input))
		aParamRef := tagRef(t, l, func(o any) bool {
			v, ok := o.(*runtime.Var)
			return ok && v.Name == "a" && v.Flags&runtime.Inout != 0
		})
		bParamRef := tagRef(t, l, func(o any) bool {
			v, ok := o.(*runtime.Var)
			return ok && v.Name == "b" && v.Flags&runtime.Inout != 0
		})
		fRef := tagRef(t, l, func(o any) bool {
			f, ok := o.(*runtime.Function)
			return ok && f.Name == "f"
		})
		gaRef := tagRef(t, l, func(o any) bool {
			v, ok := o.(*runtime.Var)
			return ok && v.Name == "a" && v.Flags&runtime.Local == 0 && v.Flags&runtime.Parameter == 0
		})
		for i := range input {
			if input[i].Op != opcode.REF {
				continue
			}
			switch input[i].Arg {
			case placeA:
				input[i].Arg = aParamRef
			case placeB:
				input[i].Arg = bParamRef
			case placeF:
				input[i].Arg = fRef
			case placeGA:
				input[i].Arg = gaRef
			}
		}

		m := testMachine(t, input, &Machine{
			Globals: []any{[]any{1, 2, 3}},
		})
		a := m.Globals[0].([]any)
		assert.GreaterOrEqual(t, len(a), expandIdx+1)
		assert.Equal(t, 99, a[expandIdx])
		assert.Equal(t, 11, a[0], "inout element actual must re-resolve after record-of expand")
	})
}

func TestBuiltins(t *testing.T) {
	t.Run("int2str", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.NATLONG, Arg: 42},
			{Op: opcode.INT2STR},
			{Op: opcode.RETURN},
		}
		m := testMachine(t, input, &Machine{})
		assert.Equal(t, "42", popResult(m))
	})

	t.Run("lengthof", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.UTF8, Arg: "hello"},
			{Op: opcode.LENGTHOF},
			{Op: opcode.RETURN},
		}
		m := testMachine(t, input, &Machine{})
		assert.Equal(t, 5, popResult(m))
	})

	t.Run("sizeof", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.MARK},
			{Op: opcode.NATLONG, Arg: 1},
			{Op: opcode.TRUE},
			{Op: opcode.VLIST},
			{Op: opcode.SIZEOF},
			{Op: opcode.RETURN},
		}
		m := testMachine(t, input, &Machine{})
		assert.Equal(t, 2, popResult(m))
	})

	t.Run("encvalue", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.NATLONG, Arg: 42},
			{Op: opcode.ENCVALUE},
			{Op: opcode.RETURN},
		}
		m := testMachine(t, input, &Machine{})
		assert.Equal(t, "'0011010000110010'B", popResult(m).(runtime.Binary).String())
	})

	t.Run("log", func(t *testing.T) {
		var buf bytes.Buffer
		input := []t3xf.Instruction{
			{Op: opcode.NATLONG, Arg: 42},
			{Op: opcode.LOG},
			{Op: opcode.UTF8, Arg: "hi"},
			{Op: opcode.LOG},
			{Op: opcode.MARK},
			{Op: opcode.NATLONG, Arg: 1},
			{Op: opcode.UTF8, Arg: "a"},
			{Op: opcode.COLLECT},
			{Op: opcode.LOG},
			{Op: opcode.RETURN},
		}
		testMachine(t, input, &Machine{Out: &buf})
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if assert.Len(t, lines, 3) {
			for i, want := range []string{"42", `"hi"`, `1"a"`} {
				e, err := k3log.ParseEvent(lines[i])
				if assert.NoError(t, err) {
					assert.Equal(t, k3log.ULOG.ID(), e.Field(1))
					assert.Equal(t, "k3r", e.Field(2))
					assert.Equal(t, want, e.Field(3))
				}
			}
		}
	})
}

func TestIntegerOverflow(t *testing.T) {
	neg := func(v int) []t3xf.Instruction {
		return []t3xf.Instruction{
			{Op: opcode.NATLONG, Arg: 0},
			{Op: opcode.NATLONG, Arg: v},
			{Op: opcode.SUB},
		}
	}
	overflow := func(op opcode.Opcode, setup ...[]t3xf.Instruction) []t3xf.Instruction {
		instrs := []t3xf.Instruction{{Op: opcode.SCAN}}
		for _, s := range setup {
			instrs = append(instrs, s...)
		}
		return append(instrs,
			t3xf.Instruction{Op: op},
			t3xf.Instruction{Op: opcode.BLOCK},
			t3xf.Instruction{Op: opcode.CONTROL},
		)
	}

	t.Run("add above MaxInt32", func(t *testing.T) {
		_, err := testMachineWithError(t, overflow(opcode.ADD,
			[]t3xf.Instruction{{Op: opcode.NATLONG, Arg: math.MaxInt32}, {Op: opcode.NATLONG, Arg: 1}},
		), &Machine{})
		assert.ErrorIs(t, err, ErrOverflow)
	})
	t.Run("add below MinInt32", func(t *testing.T) {
		_, err := testMachineWithError(t, overflow(opcode.ADD,
			neg(2_000_000_000),
			neg(500_000_000),
		), &Machine{})
		assert.ErrorIs(t, err, ErrOverflow)
	})
	t.Run("sub below MinInt32", func(t *testing.T) {
		_, err := testMachineWithError(t, overflow(opcode.SUB,
			neg(2_000_000_000),
			[]t3xf.Instruction{{Op: opcode.NATLONG, Arg: 500_000_000}},
		), &Machine{})
		assert.ErrorIs(t, err, ErrOverflow)
	})
	t.Run("sub above MaxInt32", func(t *testing.T) {
		_, err := testMachineWithError(t, overflow(opcode.SUB,
			[]t3xf.Instruction{{Op: opcode.NATLONG, Arg: math.MaxInt32}},
			neg(1),
		), &Machine{})
		assert.ErrorIs(t, err, ErrOverflow)
	})
	t.Run("neg MinInt32", func(t *testing.T) {
		_, err := testMachineWithError(t, overflow(opcode.NEG,
			[]t3xf.Instruction{{Op: opcode.NATLONG, Arg: math.MinInt32}},
		), &Machine{})
		assert.ErrorIs(t, err, ErrOverflow)
	})
	t.Run("mul above MaxInt32", func(t *testing.T) {
		_, err := testMachineWithError(t, overflow(opcode.MUL,
			[]t3xf.Instruction{{Op: opcode.NATLONG, Arg: 65536}, {Op: opcode.NATLONG, Arg: 65536}},
		), &Machine{})
		assert.ErrorIs(t, err, ErrOverflow)
	})
	t.Run("mul below MinInt32", func(t *testing.T) {
		_, err := testMachineWithError(t, overflow(opcode.MUL,
			neg(46341),
			[]t3xf.Instruction{{Op: opcode.NATLONG, Arg: 46342}},
		), &Machine{})
		assert.ErrorIs(t, err, ErrOverflow)
	})
}

func tagRef(t *testing.T, l *loader, match func(any) bool) t3xf.Reference {
	t.Helper()
	for addr, obj := range l.Tags {
		if obj != nil && match(obj) {
			return t3xf.Reference(addr << 2)
		}
	}
	t.Fatal("tag not found")
	return 0
}

func testMachine(t *testing.T, instrs []t3xf.Instruction, m *Machine) *Machine {
	m, err := testMachineWithError(t, instrs, m)
	if err != nil {
		t.Fatalf("failed to execute: %v", err)
	}
	return m
}
func testMachineWithError(t *testing.T, instrs []t3xf.Instruction, m *Machine) (*Machine, error) {
	if m.Stack == nil {
		m.Stack = make([]any, 100)
	}
	if m.Out == nil {
		m.Out = io.Discard
	}

	if m.Image == nil {
		b := mustAssemble(t, instrs)
		if len(instrs) > 0 && instrs[len(instrs)-1].Op == opcode.CONTROL {
			m.Image = mustLoad(t, b).Image
			return m, m.Run()
		}
		t3xf.PatchBlocks(b)
		m.Image = NewImage(b)
	}

	return m, m.Execute(&runtime.Function{
		Body: Block{-4, len(m.Code)},
	})
}

func popResult(m *Machine) any {
	switch m.SP {
	case 0:
		return fmt.Errorf("stack is empty")
	case 1:
		return m.deref(m.Pop())
	default:
		return m.Stack[:m.SP]
	}
}

func TestExecute(t *testing.T) {
	root := mustParse(t, `module M {
type component C {}
testcase tc(integer a, charstring s) runs on C {
  log(a);
}
testcase tc2() runs on C {
  setverdict(fail);
}
control {
  execute(tc(1, "a"), 5.0);
  log(execute(tc2()));
}
}`)
	root.Filename = "M.ttcn3"
	c := t3xf.NewCompiler()
	if err := c.Compile(root); err != nil {
		t.Fatal(err)
	}
	b, err := c.Assemble()
	if err != nil {
		t.Fatal(err)
	}
	l := mustLoad(t, b)
	var out bytes.Buffer
	if err := Run(l.Image, &out); err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		got = append(got, line[strings.Index(line, "|")+1:])
	}
	assert.Equal(t, []string{
		"cpen|k3r|M.control()",
		"tcst|k3r|M.tc|5.0",
		"cocr|k3r|MTC|once",
		"cost|MTC=M.ttcn3:3|M.tc",
		`tcen|MTC|M.tc(a=1,s="a")`,
		"ulog|MTC=M.ttcn3:4|1",
		"tclv|MTC|M.tc(a=-,s=-)",
		"cofi|MTC|none",
		"tcfi|k3r|M.tc|none",
		"tcst|k3r|M.tc2|unlimited",
		"cocr|k3r|MTC|once",
		"cost|MTC=M.ttcn3:6|M.tc2",
		"tcen|MTC|M.tc2()",
		"setv|MTC=M.ttcn3:7|none|fail",
		"tclv|MTC|M.tc2()",
		"cofi|MTC|fail",
		"tcfi|k3r|M.tc2|fail",
		"ulog|k3r|fail",
		"cplv|k3r|M.control()",
	}, got)
}

func TestExecuteSetVerdict(t *testing.T) {
	root := mustParse(t, `module M {
type component C {}
testcase tc() runs on C {
  setverdict(fail);
  setverdict(pass, "kept");
}
control {
  execute(tc());
}
}`)
	root.Filename = "M.ttcn3"
	c := t3xf.NewCompiler()
	if err := c.Compile(root); err != nil {
		t.Fatal(err)
	}
	b, err := c.Assemble()
	if err != nil {
		t.Fatal(err)
	}
	l := mustLoad(t, b)
	var out bytes.Buffer
	if err := Run(l.Image, &out); err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		got = append(got, line[strings.Index(line, "|")+1:])
	}
	assert.Equal(t, []string{
		"cpen|k3r|M.control()",
		"tcst|k3r|M.tc|unlimited",
		"cocr|k3r|MTC|once",
		"cost|MTC=M.ttcn3:3|M.tc",
		"tcen|MTC|M.tc()",
		"setv|MTC=M.ttcn3:4|none|fail",
		`setv|MTC=M.ttcn3:5|fail|pass|"kept"`,
		"tclv|MTC|M.tc()",
		"cofi|MTC|fail",
		"tcfi|k3r|M.tc|fail",
		"cplv|k3r|M.control()",
	}, got)
}

// The behaviour function is compiled after control, so its body sits past the
// execute argument block. Evaluating the argument must still call it.
func TestExecuteArgCallsLaterFunction(t *testing.T) {
	root := mustParse(t, `module M {
type component C {}
testcase tc(integer n) runs on C {
  log(n);
}
control {
  execute(tc(id(1)));
}
function id(integer x) return integer {
  return x + 10;
}
}`)
	root.Filename = "M.ttcn3"
	c := t3xf.NewCompiler()
	if err := c.Compile(root); err != nil {
		t.Fatal(err)
	}
	b, err := c.Assemble()
	if err != nil {
		t.Fatal(err)
	}
	l := mustLoad(t, b)
	var out bytes.Buffer
	if err := Run(l.Image, &out); err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		got = append(got, line[strings.Index(line, "|")+1:])
	}
	assert.Equal(t, []string{
		"cpen|k3r|M.control()",
		"fnen|k3r=M.ttcn3:7|M.id(x=1)",
		"fnlv|k3r=M.ttcn3:7|M.id(x=-)->11",
		"tcst|k3r|M.tc|unlimited",
		"cocr|k3r|MTC|once",
		"cost|MTC=M.ttcn3:3|M.tc",
		"tcen|MTC|M.tc(n=11)",
		"ulog|MTC=M.ttcn3:4|11",
		"tclv|MTC|M.tc(n=-)",
		"cofi|MTC|none",
		"tcfi|k3r|M.tc|none",
		"cplv|k3r|M.control()",
	}, got)
}

func TestNestedElementVachType(t *testing.T) {
	out, err := runModule(t, `module M {
type record of integer RI;
type record of RI RRI;
type record R { RI l }
type component C {}
function f(inout integer e) runs on C { e := 5; }
testcase tc() runs on C {
  var RRI n := { { 1, 2 } }; n[0][1] := 5;
  var R s := { { 1 } }; s.l[0] := 5;
  f(s.l[0]);
}
control { execute(tc()); }
}`)
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	for _, want := range []string{
		"vach|MTC=M.ttcn3:8|M.RI[-]|M.tc.n[0][1]|5",
		"vach|MTC=M.ttcn3:9|M.RI[-]|M.tc.s.l[0]|5",
		"vach|MTC=M.ttcn3:6|M.RI[-]|M.f.e|5",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %s\n%s", want, out)
		}
	}
}

func TestUnaryAndLogicalOperandsDeref(t *testing.T) {
	// Operands are variable references; each opcode must read their values.
	tests := []struct {
		name    string
		typ     opcode.Opcode
		globals []any
		refs    []int
		op      opcode.Opcode
		want    any
	}{
		{"not", opcode.BOOLEAN, []any{true}, []int{2}, opcode.NOT, false},
		{"and", opcode.BOOLEAN, []any{true, true}, []int{2, 5}, opcode.AND, true},
		{"or", opcode.BOOLEAN, []any{false, true}, []int{2, 5}, opcode.OR, true},
		{"neg", opcode.INTEGER, []any{5}, []int{2}, opcode.NEG, -5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input []t3xf.Instruction
			for _, name := range []string{"y", "x"}[2-len(tt.refs):] {
				input = append(input,
					t3xf.Instruction{Op: tt.typ},
					t3xf.Instruction{Op: opcode.NAME, Arg: name},
					t3xf.Instruction{Op: opcode.VAR})
			}
			input = append(input, t3xf.Instruction{Op: opcode.SCAN})
			for _, ref := range tt.refs {
				input = append(input, t3xf.Instruction{Op: opcode.REF, Arg: ref})
			}
			input = append(input,
				t3xf.Instruction{Op: tt.op},
				t3xf.Instruction{Op: opcode.BLOCK},
				t3xf.Instruction{Op: opcode.CONTROL})
			m := testMachine(t, input, &Machine{Globals: tt.globals})
			assert.Equal(t, tt.want, popResult(m))
		})
	}
}

func TestRunModule(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "parameter and local",
			src: `module M {
  function f(integer p) { var integer x := 7; log(p) }
  control { f(42) }
}
`,
			want: []string{"42"},
		},
		{
			name: "multiple log arguments",
			src: `module M {
  control { log("a", 1, "b") }
}
`,
			want: []string{`"a"1"b"`},
		},
		{
			name: "single log argument",
			src: `module M {
  control { log("hi") }
}
`,
			want: []string{`"hi"`},
		},
		{
			name: "nested collections",
			src: `module M {
  function f() return integer { log("x", 1); return 2 }
  control { log("r=", f()) }
}
`,
			want: []string{`"x"1`, `"r="2`},
		},
		{
			name: "nested block local",
			src: `module M {
  control { var integer a := 1; if (true) { var integer b := 2 }; log(a) }
}
`,
			want: []string{"1"},
		},
		{
			name: "runs on call",
			src: `module M {
  type component C {}
  function g() runs on C { log(1); log(2) }
  testcase tc() runs on C { g(); setverdict(pass) }
  control { execute(tc()) }
}
`,
			want: []string{
				"cpen|k3r|M.control()",
				"tcst|k3r|M.tc|unlimited",
				"cocr|k3r|MTC|once",
				"cost|MTC=M.ttcn3:4|M.tc",
				"tcen|MTC|M.tc()",
				"ulog|MTC=M.ttcn3:3|1",
				"ulog|MTC=M.ttcn3:3|2",
				"setv|MTC=M.ttcn3:4|none|pass",
				"tclv|MTC|M.tc()",
				"cofi|MTC|pass",
				"tcfi|k3r|M.tc|pass",
				"cplv|k3r|M.control()",
			},
		},
		{
			name: "return in alt branch",
			src: `module M {
  type component C {}
  function p() runs on C {}
  function f(C c) runs on C return integer {
    alt { [] c.done { log("in"); return 7 } }
    log("NOT REACHED");
    return 0
  }
  testcase tc() runs on C system C {
    var C c := C.create;
    c.start(p());
    log(f(c));
    log("back");
    setverdict(pass)
  }
  control { execute(tc()) }
}
`,
			want: []string{`"in"`, "7", `"back"`},
		},
		{
			name: "if else",
			src: `module M {
  control { var boolean b := true; if (b) { log(1) } else { log(2) } }
}
`,
			want: []string{"1"},
		},
		{
			name: "else if",
			src: `module M {
  control {
    var integer x := 2;
    if (x == 1) { log(1) }
    else if (x == 2) { log(2) }
    else { log(3) }
  }
}
`,
			want: []string{"2"},
		},
		{
			name: "else if without final else",
			src: `module M {
  control {
    var integer x := 2;
    if (x == 1) { log(1) }
    else if (x == 2) { log(2) }
  }
}
`,
			want: []string{"2"},
		},
		{
			name: "while",
			src: `module M {
  control { var boolean b := true; while (b) { log(1); b := false } }
}
`,
			want: []string{"1"},
		},
		{
			name: "increments after body",
			src: `module M {
  control {
    for (var integer i := 0; i != 3; i := i + 1) {
      log(i)
    }
  }
}
`,
			want: []string{"0", "1", "2"},
		},
		{
			name: "body local",
			src: `module M {
  control {
    for (var integer i := 0; i != 3; i := i + 1) {
      var integer n := i;
      log(n)
    }
  }
}
`,
			want: []string{"0", "1", "2"},
		},
		{
			name: "continue in if inside for",
			src: `module M {
  control {
    for (var integer i := 0; i < 3; i := i + 1) {
      if (i == 1) { continue; }
      log(i);
    }
  }
}
`,
			want: []string{"0", "2"},
		},
		{
			name: "break in if inside while",
			src: `module M {
  control {
    var integer i := 0;
    while (i < 5) {
      i := i + 1;
      if (i == 3) { break; }
      log(i);
    }
  }
}
`,
			want: []string{"1", "2"},
		},
		{
			name: "binary string literals",
			src: `module M {
  control {
    log('0101'B);
    log('E1F0A'H);
    log('0A'O);
    log(''B);
    log('1010 1100'B);
    log('ab'H);
    log('01?1'B);
  }
}
`,
			want: []string{`'0101'B`, `'E1F0A'H`, `'0A'O`, `''B`, `'10101100'B`, `'AB'H`, `"01?1"`},
		},
		{
			name: "arithmetic",
			src: `module M {
  control {
    log(7 / 2);
    log((-7) / 2);
    log(5 mod 3);
    log((-5) mod 3);
    log(5 mod (-3));
    log((-7) rem 2);
    log(7 rem (-2));
  }
}
`,
			want: []string{"3", "-3", "2", "1", "2", "-1", "1"},
		},
		{
			name: "comparisons and logic",
			src: `module M {
  function f() return boolean { log("side"); return true }
  control {
    log(1 < 2);
    log(2 >= 3);
    log(not false);
    log(-3);
    log(+4);
    log(false and f());
    log(true and f());
    log(true or f());
    var boolean x := false;
    var boolean y := true;
    log(x and y);
    log(false and (y));
  }
}
`,
			want: []string{"true", "false", "true", "-3", "4", "false", `"side"`, "true", "true", "false", "false"},
		},
		{
			name: "typed locals",
			src: `module M {
  control {
    var charstring s := "hi";
    var bitstring b := '10'B;
    var hexstring h := 'AB'H;
    var octetstring o := '0A'O;
    var float f := 1.5;
    timer t;
    log(s);
    log(b);
    log(h);
    log(o);
    log(f);
    log(1);
  }
}
`,
			want: []string{`"hi"`, `'10'B`, `'AB'H`, `'0A'O`, "1.5", "1"},
		},
		{
			name: "record of value",
			src: `module M {
  type record of integer Ro;
  control {
    var Ro r := {1, 2};
    log(r);
  }
}
`,
			want: []string{"{1,2}"},
		},
		{
			name: "range",
			src: `module M {
  control { log(1 .. 3) }
}
`,
			want: []string{"(1..3)"},
		},
		{
			name: "module globals",
			src: `module M {
  const integer c := 5;
  modulepar integer mp := 10;
  modulepar integer mpu;
  var integer x := 1;
  var integer y;
  control {
    log(c, mp, mpu, x, y);
    y := x + c;
    log(y);
  }
}
`,
			want: []string{"510-1-", "6"},
		},
		{
			name: "three-level element assign",
			src: `module M {
  type record of integer R1;
  type record of R1 R2;
  type record of R2 R3;
  type record A { integer z }
  type record B { A b }
  type record C { B a }
  control {
    var R3 a;
    a[0][0][5] := 42;
    var C r;
    r.a.b.z := 99;
    log(a[0][0][5], r.a.b.z);
  }
}
`,
			want: []string{"4299"},
		},
		{
			name: "compound and binary equality",
			src: `module M {
  type record of integer RoI;
  type record R { integer a, charstring b }
  control {
    var RoI a := {1, 2};
    var RoI b := {1, 2};
    var RoI c := {1, 3};
    var R r1 := {1, "x"};
    var R r2 := {1, "x"};
    var R r3 := {1, "y"};
    log(a == b, a != b, a == c, a != c);
    log(r1 == r2, r1 != r3);
    log('0101'B == '0101'B, '0101'B == '0100'B);
  }
}
`,
			want: []string{"truefalsefalsetrue", "truetrue", "truefalse"},
		},
		{
			name: "float formatting",
			src: `module M {
  control {
    log(0.0);
    log(-0.0);
    log(5.0);
    log(1.5);
    log(0.125);
    log(1e10);
    log(1e20);
  }
}
`,
			want: []string{"0.0", "0.0", "5.0", "1.5", "0.125", "10000000000.0", "100000000000000000000.0"},
		},
		{
			name: "ptc compound arg and global isolation",
			src: `module M {
  type component C {}
  type record of integer RoI;
  var RoI g := {10, 20};
  function f(RoI p) runs on C {
    p[0] := 99;
    g[0] := 77;
    log(p, g);
  }
  testcase tc() runs on C system C {
    var RoI a := {1, 2};
    var C c := C.create;
    c.start(f(a));
    c.done;
    log(a, g);
    setverdict(pass);
  }
  control {
    execute(tc());
  }
}
`,
			want: []string{"{99,2}{77,20}", "{1,2}{10,20}"},
		},
		{
			name: "eager var template vs template evaluation",
			src: `module M {
  var integer n := 0;
  function next() return integer {
    n := n + 1;
    return n;
  }
  template integer m_t := next();
  template integer m_p(integer d) := next() + d;
  control {
    var template integer v_t := next();
    log(v_t, v_t);
    log(m_t, m_t);
    log(m_p(10), m_p(20));
    template integer l_t := next();
    log(l_t, l_t);
  }
}
`,
			want: []string{"11", "23", "1425", "67"},
		},
		{
			name: "template restrictions combinators and modifiers",
			src: `module M {
  control {
    var template integer t1 := ?;
    var template(omit) integer t2 := omit;
    var template(present) integer t3 := (1, 2, 3);
    var template(value) integer t4 := 5;
    log(t1, t2, t3, t4);
    var template integer c1 := complement(1, 2);
    var template integer c2 := subset(3, 4);
    var template integer c3 := superset(5);
    var template integer c4 := permutation(6, ?);
    log(c1, c2, c3, c4);
    var template integer r := -infinity .. infinity;
    var template charstring m1 := ? ifpresent;
    var template charstring m2 := ? length(1 .. 4) ifpresent;
    var template charstring m3 := "abc" length(3) ifpresent;
    var template charstring p := pattern "ab?c*";
    log(r, m1, m2, m3, p);
    log(t1 == ?, t3 == (1, 2, 3), c1 == complement(1, 2));
  }
}
`,
			want: []string{
				"?omit(1,2,3)5",
				"complement(1,2)subset(3,4)superset(5)permutation(6,?)",
				`(-infinity..infinity)** length(1..4)"abc" length(3) ifpresent"ab?c*"`,
				"truetruetrue",
			},
		},
		{
			name: "compound templates inline templates and wildcard expansion",
			src: `module M {
  type record R { integer a, integer b optional }
  type record of integer RoI;
  template R m_r := { a := 1, b := 2 ifpresent };
  template R m_any := ?;
  const R c_r := { a := 3, b := omit };
  control {
    log(m_r, m_any.a, m_any.b, c_r, R:{ a := 10, b := omit });
    var template R vr := ?;
    log(vr.a, vr.b);
    vr.a := 10;
    log(vr);
    var template RoI vro := { [0] := 1, [2] := 3 };
    var template RoI vany := ?;
    vany[2] := 99;
    log(vro, vany);
  }
}
`,
			want: []string{
				"{1,2 ifpresent}?*{3,omit}{10,omit}",
				"?*",
				"{10,*}",
				"{1,-,3}{?,?,99,*}",
			},
		},
		{
			name: "valueof success and faults",
			src: `module M {
  type component C {}
  type record R { integer a, integer b optional }
  template R m_ok(integer x) := { a := x, b := omit };
  template R m_bad := { a := (1, 2), b := omit };
  testcase tc_ok() runs on C {
    var template integer vt := 42;
    var R r := valueof(m_ok(10));
    log(valueof(vt), r, valueof(R:{ a := 7, b := omit }));
    setverdict(pass);
  }
  testcase tc_many() runs on C {
    var template integer vt := ?;
    log(valueof(vt));
  }
  testcase tc_omit() runs on C {
    var template integer vt := omit;
    log(valueof(vt));
  }
  testcase tc_undf() runs on C {
    var template R vr := { a := 1, b := - };
    log(valueof(vr));
  }
  testcase tc_anon() runs on C {
    log(valueof(m_bad));
  }
  control {
    execute(tc_ok());
    execute(tc_many());
    execute(tc_omit());
    execute(tc_undf());
    execute(tc_anon());
  }
}
`,
			want: []string{
				"cpen|k3r|M.control()",
				"tcst|k3r|M.tc_ok|unlimited",
				"cocr|k3r|MTC|once",
				"cost|MTC=M.ttcn3:6|M.tc_ok",
				"tcen|MTC|M.tc_ok()",
				"ulog|MTC=M.ttcn3:9|42{10,omit}{7,omit}",
				"setv|MTC=M.ttcn3:10|none|pass",
				"tclv|MTC|M.tc_ok()",
				"cofi|MTC|pass",
				"tcfi|k3r|M.tc_ok|pass",
				"tcst|k3r|M.tc_many|unlimited",
				"cocr|k3r|MTC|once",
				"cost|MTC=M.ttcn3:12|M.tc_many",
				"tcen|MTC|M.tc_many()",
				"MANY|MTC=M.ttcn3:14|M.tc_many.vt",
				"bctr|MTC|M.ttcn3:29:tc_many()",
				"tcfi|k3r|M.tc_many|error",
				"tcst|k3r|M.tc_omit|unlimited",
				"cocr|k3r|MTC|once",
				"cost|MTC=M.ttcn3:16|M.tc_omit",
				"tcen|MTC|M.tc_omit()",
				"OMIT|MTC=M.ttcn3:18|M.tc_omit.vt",
				"bctr|MTC|M.ttcn3:30:tc_omit()",
				"tcfi|k3r|M.tc_omit|error",
				"tcst|k3r|M.tc_undf|unlimited",
				"cocr|k3r|MTC|once",
				"cost|MTC=M.ttcn3:20|M.tc_undf",
				"tcen|MTC|M.tc_undf()",
				"UNDF|MTC=M.ttcn3:22|M.tc_undf.vr.b",
				"bctr|MTC|M.ttcn3:31:tc_undf()",
				"tcfi|k3r|M.tc_undf|error",
				"tcst|k3r|M.tc_anon|unlimited",
				"cocr|k3r|MTC|once",
				"cost|MTC=M.ttcn3:24|M.tc_anon",
				"tcen|MTC|M.tc_anon()",
				"MANY|MTC=M.ttcn3:25|(anon).a",
				"bctr|MTC|M.ttcn3:32:tc_anon()",
				"tcfi|k3r|M.tc_anon|error",
				"cplv|k3r|M.control()",
			},
		},
		{
			name: "match scalars ranges lists and modifiers",
			src: `module M {
  type component C {}
  type record Inner { integer x }
  type record Outer { Inner b }
  type record ROpt { integer a, integer b optional }
  type record of integer RoI;
  var integer g_a := 1, g_b := 2, g_c;
  function checkParams(in integer p_in, inout integer p_inout, out integer p_out) runs on C {
    p_out := 3;
    log(match(p_in, 99), match(p_inout, 99), match(p_out, 99));
  }
  testcase tc() runs on C {
    var integer x := 20;
    template integer m_lt10 := (-infinity..9);
    log(match(-20, m_lt10), match(x, m_lt10));
    log(match(x, (1, 2, 3)), match(x, complement(10, 20)));
    var charstring s := "abc";
    var template charstring ts_len := "abc" length(4);
    var template charstring ts_val_len := "xyz" length(3);
    log(match(s, ts_len), match(s, ts_val_len));
    var ROpt ro_omit := { a := 1, b := omit };
    var ROpt ro_val := { a := 1, b := 5 };
    var template ROpt tro_ifp := { a := 1, b := 2 ifpresent };
    var template ROpt tro_any := { a := 1, b := ? };
    var template ROpt tro_omit := { a := 1, b := omit };
    log(match(ro_omit, tro_ifp), match(ro_val, tro_ifp), match(ro_omit, tro_any), match(ro_val, tro_omit));
    var Outer v_out := { b := { x := 10 } };
    var RoI va := { 8, 9 };
    log(match(v_out.b.x, 99), match(va[1], 77));
    checkParams(g_a, g_b, g_c);
    setverdict(pass);
  }
  control { execute(tc()) }
}
`,
			want: []string{
				"cpen|k3r|M.control()",
				"tcst|k3r|M.tc|unlimited",
				"cocr|k3r|MTC|once",
				"cost|MTC=M.ttcn3:12|M.tc",
				"tcen|MTC|M.tc()",
				"matc|MTC=M.ttcn3:15|-20|(-infinity..9)|",
				"matc|MTC=M.ttcn3:15|20|(-infinity..9)|M.tc.x:20~(-infinity..9)",
				"ulog|MTC=M.ttcn3:15|truefalse",
				"matc|MTC=M.ttcn3:16|20|(1,2,3)|M.tc.x:20~(1,2,3)",
				"matc|MTC=M.ttcn3:16|20|complement(10,20)|M.tc.x:20~complement(10,20)",
				"ulog|MTC=M.ttcn3:16|falsefalse",
				`matc|MTC=M.ttcn3:20|"abc"|"abc" length(4)|M.tc.s:length(3)~length(4)`,
				`matc|MTC=M.ttcn3:20|"abc"|"xyz" length(3)|M.tc.s:"abc"~"xyz" length(3)`,
				"ulog|MTC=M.ttcn3:20|falsefalse",
				"matc|MTC=M.ttcn3:26|{1,omit}|{1,2 ifpresent}|",
				"matc|MTC=M.ttcn3:26|{1,5}|{1,2 ifpresent}|M.tc.ro_val.b:5~2 ifpresent",
				"matc|MTC=M.ttcn3:26|{1,omit}|{1,?}|M.tc.ro_omit.b:omit~?",
				"matc|MTC=M.ttcn3:26|{1,5}|{1,omit}|M.tc.ro_val.b:5~omit",
				"ulog|MTC=M.ttcn3:26|truefalsefalsefalse",
				"matc|MTC=M.ttcn3:29|10|99|M.tc.v_out.b.x:10~99",
				"matc|MTC=M.ttcn3:29|9|77|M.tc.va[1]:9~77",
				"ulog|MTC=M.ttcn3:29|falsefalse",
				"matc|MTC=M.ttcn3:10|1|99|M.checkParams.p_in:1~99",
				"matc|MTC=M.ttcn3:10|2|99|M.checkParams.p_inout:2~99",
				"matc|MTC=M.ttcn3:10|3|99|M.checkParams.p_out:3~99",
				"ulog|MTC=M.ttcn3:10|falsefalsefalse",
				"setv|MTC=M.ttcn3:31|none|pass",
				"tclv|MTC|M.tc()",
				"cofi|MTC|pass",
				"tcfi|k3r|M.tc|pass",
				"cplv|k3r|M.control()",
			},
		},
		{
			name: "match patterns compounds and enums",
			src: `module M {
  type enumerated E1 { A, B }
  type enumerated E2 { A, C }
  type set S { charstring f1, boolean f2 }
  type union U { integer a, charstring b }
  type record of integer RoI;
  type set of integer SoI;
  control {
    var charstring cs := "abcde";
    var template charstring tcs1 := pattern "ab*de";
    var template charstring tcs2 := pattern "ab*xy";
    var bitstring bs := '10110'B;
    var template bitstring tbs1 := '10?1*'B;
    var template bitstring tbs2 := '11?1*'B;
    var hexstring hs := 'AB12'H;
    var template hexstring ths := 'A?1*'H;
    var octetstring os := '0A1B2C'O;
    var template octetstring tos := '0A?*'O;
    log(match(cs, tcs1), match(cs, tcs2), match(bs, tbs1), match(bs, tbs2), match(hs, ths), match(os, tos));

    var S vs := { f1 := "abc", f2 := true };
    var template S tvs := { f1 := "ab*de", f2 := ? };
    log(vs, match(vs, tvs));

    var U vu := { a := 1 };
    vu.b := "hi";
    var template U tvu1 := { a := 1 };
    var template U tvu2 := { b := "bye" };
    log(vu, match(vu, tvu1), match(vu, tvu2));

    var RoI ro := { 1, 2, 3 };
    var template RoI tro_len := { 1, 2 };
    var template RoI tro_elem := { 1, 9, 3 };
    var template RoI tro_wild := { 1, *, 3 };
    var template RoI tro_perm := { permutation(2, 1), 3 };
    log(match(ro, tro_len), match(ro, tro_elem), match(ro, tro_wild), match(ro, tro_perm));

    var SoI so := { 1, 2, 3 };
    var template SoI tso_ok := { 3, 1, 2 };
    var template SoI tso_less := { 1, 2 };
    var template SoI tso_more := { 1, 2, 3, 4 };
    var template SoI tso_sub := subset(1, 2, 3, 4);
    var template SoI tso_sup := superset(1, 3);
    log(match(so, tso_ok), match(so, tso_less), match(so, tso_more), match(so, tso_sub), match(so, tso_sup));

    var E1 ve := A;
    log(ve, match(A, ve), match(B, ve), match(ve, E2:C));
  }
}
`,
			want: []string{
				"cpen|k3r|M.control()",
				`matc|k3r|"abcde"|"ab*de"|`,
				`matc|k3r|"abcde"|"ab*xy"|M.control.cs:"abcde"~"ab*xy"`,
				`matc|k3r|'10110'B|"10?1*"|`,
				`matc|k3r|'10110'B|"11?1*"|M.control.bs:'10110'B~"11?1*"`,
				`matc|k3r|'AB12'H|"A?1*"|`,
				`matc|k3r|'0A1B2C'O|"0A?*"|`,
				"ulog|k3r|truefalsetruefalsetruetrue",
				`matc|k3r|{f1:="abc",f2:=true}|{f1:="ab*de",f2:=?}|M.control.vs.f1:"abc"~"ab*de"`,
				`ulog|k3r|{f1:="abc",f2:=true}false`,
				`matc|k3r|{b:="hi"}|{a:=1}|M.control.vu:b~a`,
				`matc|k3r|{b:="hi"}|{b:="bye"}|M.control.vu.b:"hi"~"bye"`,
				`ulog|k3r|{b:="hi"}falsefalse`,
				"matc|k3r|{1,2,3}|{1,2}|M.control.ro:length(3)~length(2)",
				"matc|k3r|{1,2,3}|{1,9,3}|M.control.ro[1]:2~9",
				"matc|k3r|{1,2,3}|{1,*,3}|",
				"matc|k3r|{1,2,3}|{permutation(2,1),3}|",
				"ulog|k3r|falsefalsetruetrue",
				"matc|k3r|{1,2,3}|{3,1,2}|",
				"matc|k3r|{1,2,3}|{1,2}|M.control.so:{3}~{}",
				"matc|k3r|{1,2,3}|{1,2,3,4}|M.control.so:{}~4",
				"matc|k3r|{1,2,3}|subset(1,2,3,4)|",
				"matc|k3r|{1,2,3}|superset(1,3)|",
				"ulog|k3r|truefalsefalsetruetrue",
				"matc|k3r|A|A|",
				"matc|k3r|B|A|value:B~A",
				"matc|k3r|A|C|M.control.ve:A~C",
				"ulog|k3r|Atruefalsefalse",
				"cplv|k3r|M.control()",
			},
		},
		{
			name: "match undefined faults",
			src: `module M {
  type component C {}
  type record R { integer a, integer b }
  testcase tc_expr() runs on C {
    var R r;
    r.a := 1;
    log(match(r, { a := 1, b := 2 }));
  }
  testcase tc_tmpl() runs on C {
    var template integer vt;
    log(match(1, vt));
  }
  control {
    execute(tc_expr());
    execute(tc_tmpl());
  }
}
`,
			want: []string{
				"cpen|k3r|M.control()",
				"tcst|k3r|M.tc_expr|unlimited",
				"cocr|k3r|MTC|once",
				"cost|MTC=M.ttcn3:4|M.tc_expr",
				"tcen|MTC|M.tc_expr()",
				"UNDF|MTC=M.ttcn3:7|M.tc_expr.r.b",
				"bctr|MTC|M.ttcn3:14:tc_expr()",
				"tcfi|k3r|M.tc_expr|error",
				"tcst|k3r|M.tc_tmpl|unlimited",
				"cocr|k3r|MTC|once",
				"cost|MTC=M.ttcn3:9|M.tc_tmpl",
				"tcen|MTC|M.tc_tmpl()",
				"UNDF|MTC=M.ttcn3:11|M.tc_tmpl.vt",
				"bctr|MTC|M.ttcn3:15:tc_tmpl()",
				"tcfi|k3r|M.tc_tmpl|error",
				"cplv|k3r|M.control()",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := runModule(t, tt.src)
			if err != nil {
				t.Fatalf("execute: %v\n%s", err, out)
			}
			var events, ulogs []string
			for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
				e, err := k3log.ParseEvent(line)
				if !assert.NoError(t, err, line) {
					continue
				}
				switch e.Category {
				case k3log.PLOD, k3log.PERF, k3log.VRSN:
					continue
				}
				events = append(events, line[strings.Index(line, "|")+1:])
				if e.Category == k3log.ULOG {
					ulogs = append(ulogs, e.Field(3))
				}
			}
			// vach, fnen, and fnlv have focused tests in TestReferenceEvents.
			events = slices.DeleteFunc(events, func(e string) bool {
				return strings.HasPrefix(e, "vach|") || strings.HasPrefix(e, "fnen|") || strings.HasPrefix(e, "fnlv|")
			})
			if len(tt.want) > 0 && strings.Contains(tt.want[0], "|") {
				assert.Equal(t, tt.want, events)
				return
			}
			assert.Equal(t, tt.want, ulogs)
			if !strings.Contains(tt.src, "testcase ") {
				wantEvents := []string{"cpen|k3r|M.control()"}
				for _, u := range tt.want {
					wantEvents = append(wantEvents, "ulog|k3r|"+u)
				}
				wantEvents = append(wantEvents, "cplv|k3r|M.control()")
				assert.Equal(t, wantEvents, events)
			}
		})
	}
}

func TestTestcaseFaultContinuesRun(t *testing.T) {
	out, err := runModule(t, `module M {
type component C {}
testcase tc1() runs on C {
  var integer x := 0;
  log(1 / x);
  setverdict(pass);
}
testcase tc2() runs on C {
  log("ok");
  setverdict(pass);
}
control {
  log(execute(tc1()));
  log(execute(tc2()));
}
}
`)
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	var got []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		got = append(got, line[strings.Index(line, "|")+1:])
	}
	assert.Equal(t, []string{
		"cpen|k3r|M.control()",
		"tcst|k3r|M.tc1|unlimited",
		"cocr|k3r|MTC|once",
		"cost|MTC=M.ttcn3:3|M.tc1",
		"tcen|MTC|M.tc1()",
		"vach|MTC=M.ttcn3:4|integer|M.tc1.x|0",
		"DIV0|MTC=M.ttcn3:5|(anon)",
		"bctr|MTC|M.ttcn3:13:tc1()",
		"tcfi|k3r|M.tc1|error",
		"ulog|k3r|error",
		"tcst|k3r|M.tc2|unlimited",
		"cocr|k3r|MTC|once",
		"cost|MTC=M.ttcn3:8|M.tc2",
		"tcen|MTC|M.tc2()",
		`ulog|MTC=M.ttcn3:9|"ok"`,
		"setv|MTC=M.ttcn3:10|none|pass",
		"tclv|MTC|M.tc2()",
		"cofi|MTC|pass",
		"tcfi|k3r|M.tc2|pass",
		"ulog|k3r|pass",
		"cplv|k3r|M.control()",
	}, got)
}

func runModule(t *testing.T, src string) (string, error) {
	t.Helper()
	root := mustParse(t, src)
	root.Filename = "M.ttcn3"
	c := t3xf.NewCompiler()
	if err := c.Compile(root); err != nil {
		t.Fatal(err)
	}
	b, err := c.Assemble()
	if err != nil {
		t.Fatal(err)
	}
	l := mustLoad(t, b)
	if len(l.Controls) == 0 {
		t.Fatal("no control")
	}
	var out bytes.Buffer
	errCh := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errCh <- fmt.Errorf("panic: %v", r)
			}
		}()
		errCh <- Run(l.Image, &out)
	}()
	select {
	case err := <-errCh:
		return out.String(), err
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
		return "", nil
	}
}

func eventsByComponent(out string) map[string][]string {
	groups := make(map[string][]string)
	out = strings.TrimSpace(out)
	if out == "" {
		return groups
	}
	for _, line := range strings.Split(out, "\n") {
		rest := line[strings.Index(line, "|")+1:]
		fields := strings.Split(rest, "|")
		comp := fields[1]
		if i := strings.Index(comp, "="); i >= 0 {
			comp = comp[:i]
		}
		groups[comp] = append(groups[comp], rest)
	}
	return groups
}

func assertEither(t *testing.T, got, a, b []string) {
	t.Helper()
	gotS := strings.Join(got, "\n")
	if gotS == strings.Join(a, "\n") || gotS == strings.Join(b, "\n") {
		return
	}
	t.Fatalf("got:\n%s\n\nwant either:\n%s\n\nor:\n%s", gotS, strings.Join(a, "\n"), strings.Join(b, "\n"))
}

func TestPTCDone(t *testing.T) {
	out, err := runModule(t, `module M {
type component C {}
function f() runs on C {
  log("ptc");
  setverdict(pass);
}
testcase tc() runs on C {
  var C c := C.create;
  c.start(f());
  c.done;
}
control {
  execute(tc());
}
}
`)
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	groups := eventsByComponent(out)
	assert.Equal(t, []string{
		"cost|$1=M.ttcn3:3|M.f",
		"fnen|$1=M.ttcn3:9|M.f()",
		`ulog|$1=M.ttcn3:4|"ptc"`,
		"setv|$1=M.ttcn3:5|none|pass",
		"fnlv|$1=M.ttcn3:9|M.f()",
		"cofi|$1|pass",
	}, groups["$1"])
	assert.Equal(t, []string{
		"cpen|k3r|M.control()",
		"tcst|k3r|M.tc|unlimited",
		"cocr|k3r|MTC|once",
		"tcfi|k3r|M.tc|pass",
		"cplv|k3r|M.control()",
	}, groups["k3r"])
	mtc := []string{
		"cost|MTC=M.ttcn3:7|M.tc",
		"tcen|MTC|M.tc()",
		"cocr|MTC=M.ttcn3:8|$1|once",
		"vach|MTC=M.ttcn3:8|M.C|M.tc.c|$1(inactive)",
		"alen|MTC=M.ttcn3:10",
	}
	tail := []string{
		"tclv|MTC|M.tc()",
		"cofi|MTC|pass",
	}
	ready := append(append([]string{}, mtc...),
		"codo|MTC=M.ttcn3:10|$1|ready",
		"allv|MTC=M.ttcn3:10",
	)
	ready = append(ready, tail...)
	wait := append(append([]string{}, mtc...),
		"codo|MTC=M.ttcn3:10|$1|wait",
		"alwt|MTC=M.ttcn3:10|maximum",
		"codo|MTC=M.ttcn3:10|$1|ready",
		"allv|MTC=M.ttcn3:10",
	)
	wait = append(wait, tail...)
	assertEither(t, groups["MTC"], ready, wait)
}

func TestPTCFailOverridesPass(t *testing.T) {
	out, err := runModule(t, `module M {
type component C {}
function f() runs on C {
  setverdict(fail);
}
testcase tc() runs on C {
  var C c := C.create;
  c.start(f());
  c.done;
  setverdict(pass);
}
control {
  execute(tc());
}
}
`)
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	groups := eventsByComponent(out)
	assert.Equal(t, []string{
		"cost|$1=M.ttcn3:3|M.f",
		"fnen|$1=M.ttcn3:8|M.f()",
		"setv|$1=M.ttcn3:4|none|fail",
		"fnlv|$1=M.ttcn3:8|M.f()",
		"cofi|$1|fail",
	}, groups["$1"])
	assert.Equal(t, []string{
		"cpen|k3r|M.control()",
		"tcst|k3r|M.tc|unlimited",
		"cocr|k3r|MTC|once",
		"tcfi|k3r|M.tc|fail",
		"cplv|k3r|M.control()",
	}, groups["k3r"])
	mtc := []string{
		"cost|MTC=M.ttcn3:6|M.tc",
		"tcen|MTC|M.tc()",
		"cocr|MTC=M.ttcn3:7|$1|once",
		"vach|MTC=M.ttcn3:7|M.C|M.tc.c|$1(inactive)",
		"alen|MTC=M.ttcn3:9",
	}
	tail := []string{
		"setv|MTC=M.ttcn3:10|fail|pass",
		"tclv|MTC|M.tc()",
		"cofi|MTC|fail",
	}
	ready := append(append([]string{}, mtc...),
		"codo|MTC=M.ttcn3:9|$1|ready",
		"allv|MTC=M.ttcn3:9",
	)
	ready = append(ready, tail...)
	wait := append(append([]string{}, mtc...),
		"codo|MTC=M.ttcn3:9|$1|wait",
		"alwt|MTC=M.ttcn3:9|maximum",
		"codo|MTC=M.ttcn3:9|$1|ready",
		"allv|MTC=M.ttcn3:9",
	)
	wait = append(wait, tail...)
	assertEither(t, groups["MTC"], ready, wait)
}

func TestPTCReturnsWithoutDone(t *testing.T) {
	out, err := runModule(t, `module M {
type component C {}
function f() runs on C {
  log("ptc");
}
testcase tc() runs on C {
  var C c := C.create;
  c.start(f());
}
control {
  execute(tc());
}
}
`)
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	if !strings.Contains(out, "tcfi|") {
		t.Fatalf("log missing tcfi:\n%s", out)
	}
	if !strings.Contains(out, "cofi|$1|") {
		t.Fatalf("ptc goroutine did not finish:\n%s", out)
	}
}

func TestPTCNamedCreate(t *testing.T) {
	out, err := runModule(t, `module M {
type component C {}
function f() runs on C {
  setverdict(pass);
}
testcase tc() runs on C {
  var C a := C.create("worker");
  var C b := C.create;
  a.start(f());
  a.done;
}
control {
  execute(tc());
}
}
`)
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	if !strings.Contains(out, "|worker|once") {
		t.Fatalf("named create missing:\n%s", out)
	}
	if !strings.Contains(out, "|$1|once") {
		t.Fatalf("unnamed create missing:\n%s", out)
	}
	if strings.Contains(out, "|$2|") {
		t.Fatalf("named create consumed a number:\n%s", out)
	}
	if !strings.Contains(out, "cost|worker=") {
		t.Fatalf("start did not use the given name:\n%s", out)
	}
	if !strings.Contains(out, "tcfi|k3r|M.tc|pass") {
		t.Fatalf("tcfi:\n%s", out)
	}
}

func TestPTCGuard(t *testing.T) {
	out, err := runModule(t, `module M {
type component C {}
function f() runs on C {
  setverdict(pass);
}
testcase tc() runs on C {
  var C c := C.create;
  c.start(f());
  alt {
    [g()] c.done { log("body"); }
    [] c.done { log("second"); }
  }
}
function g() return boolean {
  return true;
}
control {
  execute(tc());
}
}
`)
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	groups := eventsByComponent(out)
	var mtc []string
	for _, line := range groups["MTC"] {
		if strings.HasPrefix(line, "ulog|") {
			mtc = append(mtc, line)
		}
	}
	if len(mtc) != 1 || !strings.Contains(mtc[0], `"body"`) {
		t.Fatalf("guard body:\n%s", out)
	}
	if !strings.Contains(out, "tcfi|k3r|M.tc|pass") {
		t.Fatalf("tcfi:\n%s", out)
	}
}

// TestAltSnapshotComponentDone checks that every alternative of an alt pass
// sees one snapshot (ETSI ES 201 873-1 20.1): the PTC ends while the second
// alternative's guard computes, after the first alternative found it
// running. The second's done does not see it end; the next pass takes the
// first. Were the guard quicker than the PTC, both would see it end on a
// later pass and the first would be taken all the same.
func TestAltSnapshotComponentDone(t *testing.T) {
	out, err := runModule(t, `module M {
type component C {}
function f() runs on C {
  timer t := 0.01;
  t.start;
  t.timeout;
  setverdict(pass);
}
function busy() return boolean {
  var integer i, s := 0;
  for (i := 0; i < 10000; i := i + 1) { s := s + 1; }
  return s > 0;
}
testcase tc() runs on C {
  var C c := C.create;
  c.start(f());
  alt {
    [] c.done { log("first"); }
    [busy()] c.done { log("second"); }
  }
}
control {
  execute(tc());
}
}
`)
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	var logs []string
	for _, line := range eventsByComponent(out)["MTC"] {
		if strings.HasPrefix(line, "ulog|") {
			logs = append(logs, line)
		}
	}
	if len(logs) != 1 || !strings.Contains(logs[0], `"first"`) {
		t.Fatalf("alternative taken:\n%s", out)
	}
}

// TestAltSnapshotTimeout checks that timers are read at the time of the alt
// pass (ETSI ES 201 873-1 20.1): both expire while the second alternative's
// guard computes, after the first alternative found its timer running. The
// second's timeout does not see its timer expire; the next pass takes the
// first.
func TestAltSnapshotTimeout(t *testing.T) {
	out, err := runModule(t, `module M {
type component C {}
function busy() return boolean {
  var integer i, s := 0;
  for (i := 0; i < 10000; i := i + 1) { s := s + 1; }
  return s > 0;
}
testcase tc() runs on C {
  timer t1 := 0.01, t2 := 0.01;
  t1.start;
  t2.start;
  alt {
    [] t1.timeout { log("first"); }
    [busy()] t2.timeout { log("second"); }
  }
  setverdict(pass);
}
control {
  execute(tc());
}
}
`)
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	var logs []string
	for _, line := range eventsByComponent(out)["MTC"] {
		if strings.HasPrefix(line, "ulog|") {
			logs = append(logs, line)
		}
	}
	if len(logs) != 1 || !strings.Contains(logs[0], `"first"`) {
		t.Fatalf("alternative taken:\n%s", out)
	}
}

func TestPTCStopsOnError(t *testing.T) {
	out, err := runModule(t, `module M {
type component C {}
function f() runs on C {
  while (true) {}
}
testcase tc() runs on C {
  var C c := C.create;
  c.start(f());
  log(2147483647 + 1);
}
control {
  execute(tc());
}
}
`)
	if !errors.Is(err, ErrOverflow) {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	if strings.Contains(out, "tcfi|") {
		t.Fatalf("tcfi logged on error:\n%s", out)
	}
	if !strings.Contains(out, "cofi|$1|none") {
		t.Fatalf("ptc was not joined with verdict none:\n%s", out)
	}
}

func TestPTCStopWakesAlt(t *testing.T) {
	out, err := runModule(t, `module M {
type component C {}
function loop() runs on C {
  while (true) {}
}
function f() runs on C {
  var C other := C.create;
  other.start(loop());
  other.done;
}
testcase tc() runs on C {
  var C c := C.create;
  c.start(f());
}
control {
  execute(tc());
}
}
`)
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	if !strings.Contains(out, "cofi|$1|none") {
		t.Fatalf("stopped ptc verdict:\n%s", out)
	}
	if strings.Contains(out, "cofi|$1|error") {
		t.Fatalf("stop became error:\n%s", out)
	}
}

func TestPTCVarGuard(t *testing.T) {
	out, err := runModule(t, `module M {
type component C {}
function f() runs on C {
  setverdict(pass);
}
testcase tc() runs on C {
  var C c := C.create;
  var boolean b := true;
  c.start(f());
  alt {
    [b] c.done { log("body"); }
  }
}
control {
  execute(tc());
}
}
`)
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"body"`) {
		t.Fatalf("guard body:\n%s", out)
	}
	if !strings.Contains(out, "tcfi|k3r|M.tc|pass") {
		t.Fatalf("tcfi:\n%s", out)
	}
}

func TestDeadlock(t *testing.T) {
	out, err := runModule(t, `module M {
type component C {}
type component PTC {}
testcase tc() runs on C {
  var PTC c := PTC.create;
  log("Hello Test");
  c.done;
  log("after");
}
control {
  execute(tc());
  log("continues");
}
}
`)
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	for _, want := range []string{
		"codo|MTC=M.ttcn3:7|$1|never",
		"allv|MTC=M.ttcn3:7",
		"DEAD|MTC=M.ttcn3:7",
		"bctr|MTC|M.ttcn3:11:tc()",
		"tcfi|k3r|M.tc|error",
		`"continues"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %s\n%s", want, out)
		}
	}
	if strings.Contains(out, "tclv|") || strings.Contains(out, `"after"`) {
		t.Fatalf("testcase continued after deadlock:\n%s", out)
	}
	allv := strings.Index(out, "allv|")
	dead := strings.Index(out, "DEAD|")
	if allv < 0 || dead < allv {
		t.Fatalf("allv before DEAD:\n%s", out)
	}
}

func TestPTCDeadlock(t *testing.T) {
	out, err := runModule(t, `module M {
type component C {}
function f() runs on C {
  var C other := C.create;
  other.done;
}
testcase tc() runs on C {
  var C c := C.create;
  c.start(f());
  c.done;
  log("after");
}
control {
  execute(tc());
}
}
`)
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	if !strings.Contains(out, "DEAD|") {
		t.Fatalf("DEAD:\n%s", out)
	}
	if !strings.Contains(out, `"after"`) {
		t.Fatalf("MTC did not continue:\n%s", out)
	}
	if strings.Contains(out, "cofi|$1|") {
		t.Fatalf("deadlocked component logged cofi:\n%s", out)
	}
	if !strings.Contains(out, "tcfi|k3r|M.tc|error") {
		t.Fatalf("tcfi:\n%s", out)
	}
}

func TestAltInactiveGuard(t *testing.T) {
	out, err := runModule(t, `module M {
type component C {}
function g() return boolean { return false; }
testcase tc() runs on C {
  var C c := C.create;
  alt {
    [g()] c.done { log("took"); }
  }
  log("after");
}
control {
  execute(tc());
}
}
`)
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	if strings.Contains(out, "DEAD|") || strings.Contains(out, `"took"`) {
		t.Fatalf("inactive guard:\n%s", out)
	}
	if !strings.Contains(out, `"after"`) || !strings.Contains(out, "tcfi|k3r|M.tc|none") {
		t.Fatalf("alt did not finish:\n%s", out)
	}
}

func TestTemplates(t *testing.T) {
	logLines := func(t *testing.T, instrs []t3xf.Instruction) []string {
		t.Helper()
		var buf bytes.Buffer
		testMachine(t, instrs, &Machine{Out: &buf})
		var got []string
		for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
			if line == "" {
				continue
			}
			e, err := k3log.ParseEvent(line)
			if assert.NoError(t, err) && e.Category == k3log.ULOG {
				got = append(got, e.Field(3))
			}
		}
		return got
	}

	t.Run("literals ranges combinators and modifiers", func(t *testing.T) {
		input := []t3xf.Instruction{
			{Op: opcode.OMIT},
			{Op: opcode.LOG},
			{Op: opcode.ANY},
			{Op: opcode.LOG},
			{Op: opcode.ANYN},
			{Op: opcode.LOG},
			{Op: opcode.INFINITYN},
			{Op: opcode.INFINITYP},
			{Op: opcode.RANGE},
			{Op: opcode.LOG},
			{Op: opcode.UTF8, Arg: "ab*"},
			{Op: opcode.PATTERN},
			{Op: opcode.LOG},
			// Value list via MARK ... COLLECT + ASSIGND
			{Op: opcode.MARK},
			{Op: opcode.NATLONG, Arg: 1},
			{Op: opcode.NATLONG, Arg: 2},
			{Op: opcode.NATLONG, Arg: 3},
			{Op: opcode.COLLECT},
			{Op: opcode.INTEGER},
			{Op: opcode.PERMITT},
			{Op: opcode.ASSIGND},
			{Op: opcode.LOG},
			// Combinators
			{Op: opcode.MARK},
			{Op: opcode.NATLONG, Arg: 1},
			{Op: opcode.NATLONG, Arg: 2},
			{Op: opcode.COLLECT},
			{Op: opcode.COMPLEMENT},
			{Op: opcode.LOG},
			{Op: opcode.MARK},
			{Op: opcode.NATLONG, Arg: 3},
			{Op: opcode.NATLONG, Arg: 4},
			{Op: opcode.COLLECT},
			{Op: opcode.SUBSET},
			{Op: opcode.LOG},
			{Op: opcode.MARK},
			{Op: opcode.NATLONG, Arg: 5},
			{Op: opcode.COLLECT},
			{Op: opcode.SUPERSET},
			{Op: opcode.LOG},
			{Op: opcode.MARK},
			{Op: opcode.NATLONG, Arg: 6},
			{Op: opcode.NATLONG, Arg: 7},
			{Op: opcode.COLLECT},
			{Op: opcode.PERMUTATION},
			{Op: opcode.LOG},
			// Modifiers: length and ifpresent
			{Op: opcode.ANY},
			{Op: opcode.IFPRESENT},
			{Op: opcode.LOG},
			{Op: opcode.ANY},
			{Op: opcode.NATLONG, Arg: 1},
			{Op: opcode.NATLONG, Arg: 4},
			{Op: opcode.RANGE},
			{Op: opcode.LENGTH},
			{Op: opcode.IFPRESENT},
			{Op: opcode.LOG},
			{Op: opcode.UTF8, Arg: "abc"},
			{Op: opcode.NATLONG, Arg: 3},
			{Op: opcode.LENGTH},
			{Op: opcode.IFPRESENT},
			{Op: opcode.LOG},
			{Op: opcode.RETURN},
		}
		assert.Equal(t, []string{
			"omit",
			"?",
			"*",
			"(-infinity..infinity)",
			`"ab*"`,
			"(1,2,3)",
			"complement(1,2)",
			"subset(3,4)",
			"superset(5)",
			"permutation(6,7)",
			"*",
			"* length(1..4)",
			`"abc" length(3) ifpresent`,
		}, logLines(t, input))
	})

	t.Run("compound value block and vardup", func(t *testing.T) {
		// Record R { integer a, integer b optional }
		// Local template variable initialized with VALUE/IDEF/ASSIGN block, then modified
		const (
			placeR  = t3xf.Reference(0x100)
			placeVT = t3xf.Reference(0x200)
		)
		input := []t3xf.Instruction{
			{Op: opcode.SCAN},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "a"},
			{Op: opcode.FIELD},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "b"},
			{Op: opcode.FIELDO},
			{Op: opcode.BLOCK},
			{Op: opcode.RECORD},
			{Op: opcode.NAME, Arg: "R"},
			{Op: opcode.TYPE},
			{Op: opcode.SCAN},
			{Op: opcode.REF, Arg: placeR},
			{Op: opcode.PERMITT},
			{Op: opcode.REF, Arg: placeR},
			{Op: opcode.PERMITT},
			{Op: opcode.NAME, Arg: "vt"},
			{Op: opcode.VARDUP},
			// vt := ?; log(vt.a, vt.b); vt.a := 10; log(vt);
			{Op: opcode.ANY},
			{Op: opcode.REF, Arg: placeVT},
			{Op: opcode.ASSIGN},
			{Op: opcode.MARK},
			{Op: opcode.REF, Arg: placeVT},
			{Op: opcode.IGET, Arg: 0},
			{Op: opcode.REF, Arg: placeVT},
			{Op: opcode.IGET, Arg: 1},
			{Op: opcode.COLLECT},
			{Op: opcode.LOG},
			{Op: opcode.NATLONG, Arg: 10},
			{Op: opcode.REF, Arg: placeVT},
			{Op: opcode.IDEF, Arg: 0},
			{Op: opcode.ASSIGN},
			{Op: opcode.REF, Arg: placeVT},
			{Op: opcode.LOG},
			// Compound assignment block via VALUE: vt := { a := 20, b := - }
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 20},
			{Op: opcode.VALUE},
			{Op: opcode.IDEF, Arg: 0},
			{Op: opcode.ASSIGN},
			{Op: opcode.SKIP},
			{Op: opcode.VALUE},
			{Op: opcode.IDEF, Arg: 1},
			{Op: opcode.ASSIGN},
			{Op: opcode.BLOCK},
			{Op: opcode.REF, Arg: placeVT},
			{Op: opcode.ASSIGN},
			{Op: opcode.REF, Arg: placeVT},
			{Op: opcode.LOG},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		// Resolve offsets for R and vt
		b := mustAssemble(t, input)
		l := mustLoad(t, b)
		rRef := tagRef(t, l, func(o any) bool {
			nt, ok := o.(*runtime.NamedType)
			return ok && nt.Name == "R"
		})
		// vt is at VARDUP instruction
		var vtRef t3xf.Reference
		for pc := 0; pc < len(b); {
			n, op, _ := t3xf.Decode(b[pc:])
			if op == opcode.VARDUP {
				vtRef = t3xf.Reference(pc)
				break
			}
			pc += n
		}
		for i := range input {
			if input[i].Op != opcode.REF {
				continue
			}
			switch input[i].Arg {
			case placeR:
				input[i].Arg = rRef
			case placeVT:
				input[i].Arg = vtRef
			}
		}
		assert.Equal(t, []string{
			"?*",
			"{10,*}",
			"{20,-}",
		}, logLines(t, input))
	})

	t.Run("template definition and valueof", func(t *testing.T) {
		const (
			placeP = t3xf.Reference(0x100)
			placeT = t3xf.Reference(0x200)
		)
		input := []t3xf.Instruction{
			{Op: opcode.INTEGER},
			{Op: opcode.PERMITT},
			{Op: opcode.SCAN},
			{Op: opcode.INTEGER},
			{Op: opcode.NAME, Arg: "p"},
			{Op: opcode.IN},
			{Op: opcode.BLOCK},
			{Op: opcode.SCAN},
			{Op: opcode.REF, Arg: placeP},
			{Op: opcode.NATLONG, Arg: 1},
			{Op: opcode.ADD},
			{Op: opcode.BLOCK},
			{Op: opcode.NAME, Arg: "t"},
			{Op: opcode.TEMPLATE},
			{Op: opcode.SCAN},
			{Op: opcode.NATLONG, Arg: 41},
			{Op: opcode.REF, Arg: placeT},
			{Op: opcode.VALUEOF},
			{Op: opcode.LOG},
			{Op: opcode.BLOCK},
			{Op: opcode.CONTROL},
		}
		l := mustLoad(t, mustAssemble(t, input))
		pRef := tagRef(t, l, func(o any) bool {
			v, ok := o.(*runtime.Var)
			return ok && v.Name == "p"
		})
		tRef := tagRef(t, l, func(o any) bool {
			fn, ok := o.(*runtime.Function)
			return ok && fn.Name == "t"
		})
		for i := range input {
			if input[i].Op == opcode.REF {
				switch input[i].Arg {
				case placeP:
					input[i].Arg = pRef
				case placeT:
					input[i].Arg = tRef
				}
			}
		}
		assert.Equal(t, []string{"42"}, logLines(t, input))
	})

	t.Run("valueof faults", func(t *testing.T) {
		for _, tc := range []struct {
			op   opcode.Opcode
			want string
		}{
			{opcode.ANY, "MANY"},
			{opcode.OMIT, "OMIT"},
		} {
			input := []t3xf.Instruction{
				{Op: tc.op},
				{Op: opcode.VALUEOF},
				{Op: opcode.RETURN},
			}
			_, err := testMachineWithError(t, input, &Machine{})
			assert.ErrorIs(t, err, errFault)
			assert.ErrorContains(t, err, tc.want)
		}
	})

	t.Run("smatch silent vs match logged", func(t *testing.T) {
		var buf bytes.Buffer
		input := []t3xf.Instruction{
			{Op: opcode.NATLONG, Arg: 10},
			{Op: opcode.NATLONG, Arg: 20},
			{Op: opcode.SMATCH},
			{Op: opcode.LOG},
			{Op: opcode.NATLONG, Arg: 10},
			{Op: opcode.NATLONG, Arg: 10},
			{Op: opcode.MATCH},
			{Op: opcode.LOG},
			{Op: opcode.RETURN},
		}
		testMachine(t, input, &Machine{Out: &buf})
		var events []string
		for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
			events = append(events, line[strings.Index(line, "|")+1:])
		}
		assert.Equal(t, []string{
			"ulog|k3r|false",
			"matc|k3r|10|10|",
			"ulog|k3r|true",
		}, events)
	})
}
