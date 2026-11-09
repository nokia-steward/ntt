package runtime

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Binary is a bitstring, hexstring, or octetstring value. Unit is 'B', 'H',
// or 'O'. Digits holds one character per bit or nibble, upper case; an octet
// is two nibbles.
type Binary struct {
	Unit   byte
	Digits string
}

func (b Binary) String() string { return "'" + b.Digits + "'" + string(b.Unit) }

// Len returns the number of bits, nibbles, or octets.
func (b Binary) Len() int { return len(b.Digits) / b.width() }

// width is the number of digits per element.
func (b Binary) width() int {
	if b.Unit == 'O' {
		return 2
	}
	return 1
}

func (b Binary) base() int {
	if b.Unit == 'B' {
		return 2
	}
	return 16
}

// bits returns b as a string of '0' and '1'.
func (b Binary) bits() string {
	if b.Unit == 'B' {
		return b.Digits
	}
	var s strings.Builder
	for _, c := range b.Digits {
		d, _ := strconv.ParseUint(string(c), 16, 8)
		fmt.Fprintf(&s, "%04b", d)
	}
	return s.String()
}

// fromBits encodes bits as unit, padding with leading zeros to whole digits.
func fromBits(unit byte, bits string) Binary {
	if unit == 'B' {
		return Binary{Unit: unit, Digits: bits}
	}
	n := 4
	if unit == 'O' {
		n = 8
	}
	if r := len(bits) % n; r != 0 {
		bits = strings.Repeat("0", n-r) + bits
	}
	var s strings.Builder
	for i := 0; i < len(bits); i += 4 {
		d, _ := strconv.ParseUint(bits[i:i+4], 2, 8)
		s.WriteByte(hexDigits[d])
	}
	return Binary{Unit: unit, Digits: s.String()}
}

const hexDigits = "0123456789ABCDEF"

// Element returns element i of a string or binary string.
func Element(v any, i int) (any, bool) {
	switch s := v.(type) {
	case string:
		if i >= 0 && i < len(s) {
			return s[i : i+1], true
		}
	case Binary:
		w := s.width()
		if i >= 0 && i < s.Len() {
			return Binary{Unit: s.Unit, Digits: s.Digits[i*w : (i+1)*w]}, true
		}
	}
	return nil, false
}

// SetElement replaces element i of a string or binary string with e. i may
// be the length, which appends.
func SetElement(v any, i int, e any) (any, error) {
	switch s := v.(type) {
	case string:
		c, ok := e.(string)
		if !ok || len(c) != 1 || i < 0 || i > len(s) {
			return nil, fmt.Errorf("invalid string element assignment")
		}
		return s[:i] + c + s[min(i+1, len(s)):], nil
	case Binary:
		c, ok := e.(Binary)
		w := s.width()
		if !ok || c.Unit != s.Unit || c.Len() != 1 || i < 0 || i > s.Len() {
			return nil, fmt.Errorf("invalid binary string element assignment")
		}
		return Binary{Unit: s.Unit, Digits: s.Digits[:i*w] + c.Digits + s.Digits[min((i+1)*w, len(s.Digits)):]}, nil
	}
	return nil, fmt.Errorf("element assignment to %T", v)
}

// Cat concatenates two strings, binary strings, or record of values.
func Cat(x, y any) (any, error) {
	switch a := x.(type) {
	case string:
		if b, ok := y.(string); ok {
			return a + b, nil
		}
	case Binary:
		if b, ok := y.(Binary); ok && a.Unit == b.Unit {
			return Binary{Unit: a.Unit, Digits: a.Digits + b.Digits}, nil
		}
	case []any:
		if b, ok := y.([]any); ok {
			return append(append([]any(nil), a...), b...), nil
		}
	}
	return nil, fmt.Errorf("cannot concatenate %T and %T", x, y)
}

// Bitwise applies the digit operation f to x and y of equal length. y is
// ignored by not4b.
func Bitwise(x, y Binary, f func(a, b uint64) uint64) (Binary, error) {
	if y.Unit != 0 && (x.Unit != y.Unit || len(x.Digits) != len(y.Digits)) {
		return Binary{}, fmt.Errorf("bitwise operands differ: %s, %s", x, y)
	}
	mask := uint64(x.base() - 1)
	d := []byte(x.Digits)
	for i := range d {
		a, _ := strconv.ParseUint(x.Digits[i:i+1], 16, 8)
		var b uint64
		if y.Unit != 0 {
			b, _ = strconv.ParseUint(y.Digits[i:i+1], 16, 8)
		}
		d[i] = hexDigits[f(a, b)&mask]
	}
	return Binary{Unit: x.Unit, Digits: string(d)}, nil
}

// Shift moves the elements of v by n: left or right, and rotating or
// filling with zeros. Only binary strings shift; strings and record of
// values rotate.
func Shift(v any, n int, left, rotate bool) (any, error) {
	if n < 0 {
		n, left = -n, !left
	}
	l, err := LengthOf(v)
	if err != nil || l == 0 {
		return v, err
	}
	if rotate {
		k := n % l
		if !left {
			k = (l - k) % l
		}
		return Cat(slice(v, k, l), slice(v, 0, k))
	}
	s, ok := v.(Binary)
	if !ok {
		return nil, fmt.Errorf("cannot shift %T", v)
	}
	k := min(n, l)
	zeros := Binary{Unit: s.Unit, Digits: strings.Repeat("0", k*s.width())}
	if left {
		return Cat(slice(s, k, l), zeros)
	}
	return Cat(zeros, slice(s, 0, l-k))
}

// Convert implements the predefined conversion and string functions named
// by their T3XF opcode.
func Convert(name string, args ...any) (any, error) {
	switch name {
	case "int2str":
		return strconv.Itoa(args[0].(int)), nil
	case "int2char":
		i := args[0].(int)
		if i < 0 || i > 127 {
			return nil, fmt.Errorf("int2char: %d out of range", i)
		}
		return string(rune(i)), nil
	case "char2int":
		s := args[0].(string)
		if len(s) != 1 {
			return nil, fmt.Errorf("char2int: %q is not one character", s)
		}
		return int(s[0]), nil
	case "int2float":
		return float64(args[0].(int)), nil
	case "float2int":
		f := args[0].(float64)
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return nil, fmt.Errorf("float2int: %v", f)
		}
		return int(f), nil
	case "str2int":
		s := args[0].(string)
		i, err := strconv.ParseInt(strings.TrimPrefix(s, "+"), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("str2int: %q", s)
		}
		return int(i), nil
	case "str2float":
		s := args[0].(string)
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, fmt.Errorf("str2float: %q", s)
		}
		return f, nil
	case "int2bit", "int2hex", "int2oct":
		unit := map[string]byte{"int2bit": 'B', "int2hex": 'H', "int2oct": 'O'}[name]
		i, n := args[0].(int), args[1].(int)
		b := fromBits(unit, strconv.FormatUint(uint64(i), 2))
		w := n * b.width()
		if i < 0 || len(strings.TrimLeft(b.Digits, "0")) > w {
			return nil, fmt.Errorf("%s: %d does not fit in %d", name, i, n)
		}
		b.Digits = strings.Repeat("0", max(w-len(b.Digits), 0)) + b.Digits[max(len(b.Digits)-w, 0):]
		return b, nil
	case "bit2int", "hex2int", "oct2int":
		b := args[0].(Binary)
		if b.Digits == "" {
			return 0, nil
		}
		i, err := strconv.ParseUint(b.Digits, b.base(), 63)
		return int(i), err
	case "bit2hex", "oct2hex":
		return fromBits('H', args[0].(Binary).bits()), nil
	case "bit2oct", "hex2oct":
		return fromBits('O', args[0].(Binary).bits()), nil
	case "hex2bit", "oct2bit":
		return Binary{Unit: 'B', Digits: args[0].(Binary).bits()}, nil
	case "bit2str", "hex2str", "oct2str":
		return args[0].(Binary).Digits, nil
	case "str2hex", "str2oct":
		s := strings.ToUpper(args[0].(string))
		b := Binary{Unit: 'H', Digits: s}
		if name == "str2oct" {
			b.Unit = 'O'
		}
		if strings.Trim(s, hexDigits) != "" || len(s)%b.width() != 0 {
			return nil, fmt.Errorf("%s: %q", name, args[0])
		}
		return b, nil
	case "char2oct":
		return Binary{Unit: 'O', Digits: fmt.Sprintf("%X", args[0].(string))}, nil
	case "oct2chr":
		d := args[0].(Binary).Digits
		var s strings.Builder
		for i := 0; i < len(d); i += 2 {
			c, _ := strconv.ParseUint(d[i:i+2], 16, 8)
			s.WriteByte(byte(c))
		}
		return s.String(), nil
	case "enum2int":
		return args[0].(EnumValue).Val, nil
	case "substr":
		i, n := args[1].(int), args[2].(int)
		l, err := LengthOf(args[0])
		if err != nil || i < 0 || n < 0 || i+n > l {
			return nil, fmt.Errorf("substr: index %d, length %d out of range", i, n)
		}
		return slice(args[0], i, i+n), nil
	case "replace":
		i, n := args[1].(int), args[2].(int)
		l, err := LengthOf(args[0])
		if err != nil || i < 0 || n < 0 || i+n > l {
			return nil, fmt.Errorf("replace: index %d, length %d out of range", i, n)
		}
		head, _ := Cat(slice(args[0], 0, i), args[3])
		return Cat(head, slice(args[0], i+n, l))
	}
	return nil, fmt.Errorf("unsupported conversion %s", name)
}

// slice returns elements [i, j) of a string, binary string, or record of.
func slice(v any, i, j int) any {
	switch s := v.(type) {
	case string:
		return s[i:j]
	case Binary:
		w := s.width()
		return Binary{Unit: s.Unit, Digits: s.Digits[i*w : j*w]}
	case []any:
		return append([]any(nil), s[i:j]...)
	}
	return nil
}
