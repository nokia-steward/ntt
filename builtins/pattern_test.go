package builtins

import (
	"testing"

	"github.com/nokia/ntt/runtime"
)

// Character patterns follow table B.1 and B.1.5.1 to B.1.5.3.
func TestCharacterPatterns(t *testing.T) {
	for _, tt := range []struct {
		pat, val string
		want     bool
	}{
		{`a?c`, "aűc", true},  // ? is one universal character
		{`ABC`, "abc", false}, // case is kept
		{`\w`, "_", false},    // [0-9a-zA-Z]
		{`\w`, "Z", true},
		{`\d`, "7", true},
		{`\n`, "\v", true}, // LF, VT, FF, CR
		{`\n`, " ", false},
		{`\s`, "\v", true},
		{`\s`, " ", true}, // a space character
		{`\s`, "x", false},
		{`abc|xyz`, "xyz", true}, // alternatives, anchored
		{`abc|xyz`, "abcxyz", false},
		{`[^]a]`, "^", true}, // ^ followed by ] is literal
		{`[^]a]`, "]", true},
		{`[^]a]`, "b", false},
		{`[]a]`, "]", true},
		{`[^-a]`, "-", false}, // - after the negating ^ is literal
		{`[^-a]`, "b", true},
		{`[a^b]`, "^", true},
		{`[ab[r-z]]`, "[]", true}, // NOTE 1: no embedded lists
		{`[ab[r-z]]`, "s]", true},
		{`[\d]`, "5", true},
		{`a#12`, "a2", true}, // #n is a single digit
		{`[a-z]#(2, 3)`, "abc", true},
		{`[a-z]#(2, )`, "abcdef", true},
		{`[a-z]#(, 2)`, "abc", false},
		{`x\q{0, 0, 1, 113}`, "xű", true},
		{`x\q{U0171, U0041}`, "xűA", true},
		{`a#`, "a#", true}, // no metacharacter: literal
		{`[ab`, "[ab", true},
		{`+a`, "+a", true}, // nothing to repeat
		{`+a`, "+A", false},
		{`a(b`, "a(b", true}, // no partner
		{`a(b\?`, "a(b?", true},
		{`a+#3`, "aa#3", true},        // a repetition doesn't repeat
		{`a\q{0,0,0,42}`, "a*", true}, // \q is a character, not a metacharacter
		{`a\q{0,0,0,42}`, "abc", false},
		{`[a\q{0,0,0,93}]`, "]", true},
		{`\q{U+0041}`, "A", true},
		{`\q{U0041,U0042}#2`, "ABAB", true}, // a string, repeated whole
		{`(ab)#2`, "abab", true},
		{`?#3`, "xyz", true},
		{`[a-z]+`, "abc", true},
		{`\d#(2,3)`, "123", true},
		{`(a|b)+`, "abba", true},
	} {
		val := runtime.NewUniversalString(tt.val)
		pat := &runtime.String{Value: []rune(tt.pat), IsPattern: true}
		got, err := match(val, pat)
		if err != nil {
			t.Errorf("pattern %q, %q: %v", tt.pat, tt.val, err)
			continue
		}
		if got != tt.want {
			t.Errorf("pattern %q matches %q: got %v, want %v", tt.pat, tt.val, got, tt.want)
		}
	}
}
