package verify

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseExpectedOutput(t *testing.T) {
	tests := []struct {
		name    string
		comment string
		want    string
		wantOk  bool
		wantErr bool
	}{
		{name: "plain", comment: "// @output: 1", want: "1", wantOk: true},
		{name: "no space after slashes", comment: "//@output:1", want: "1", wantOk: true},
		{name: "charstring", comment: `// @output: "hello"`, want: `"hello"`, wantOk: true},
		{name: "several arguments", comment: `// @output: "value received: "123`, want: `"value received: "123`, wantOk: true},
		{name: "bar kept", comment: `// @output: "a|b""c"`, want: `"a|b""c"`, wantOk: true},
		{name: "escaped bar kept", comment: `// @output: a\u007cb`, want: `a\u007cb`, wantOk: true},
		{name: "surrounding space trimmed", comment: "//  \t@output: \t {1, 2}  ", want: "{1, 2}", wantOk: true},
		{name: "inner space kept", comment: `// @output: "a  b"`, want: `"a  b"`, wantOk: true},
		{name: "empty", comment: "// @output:", wantErr: true},
		{name: "blank", comment: "// @output:   ", wantErr: true},
		{name: "two on one line", comment: "// @output: 1 // @output: 2", wantErr: true},
		{name: "block comment", comment: "/* @output: 1 */"},
		{name: "not at the start", comment: "// see @output: 1"},
		{name: "other tag", comment: "// @outputs: 1"},
		{name: "without colon", comment: "// @output 1"},
		{name: "k3 tag", comment: "// @k3:verdict pass"},
		{name: "plain comment", comment: "// output: 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := ParseExpectedOutput(tt.comment)
			assert.Equal(t, tt.wantOk, ok)
			assert.Equal(t, tt.want, got)
			if tt.wantErr {
				var derr *DirectiveError
				assert.ErrorAs(t, err, &derr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
