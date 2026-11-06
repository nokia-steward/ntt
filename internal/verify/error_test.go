package verify

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseExpectedError(t *testing.T) {
	tests := []struct {
		name    string
		comment string
		want    string
		wantOk  bool
		wantErr bool
	}{
		{name: "bare", comment: "// @error", wantOk: true},
		{name: "bare with trailing space", comment: "// @error  ", wantOk: true},
		{name: "category", comment: "// @error: CONV", want: "CONV", wantOk: true},
		{name: "no space", comment: "//@error:DOME", want: "DOME", wantOk: true},
		{name: "surrounding space trimmed", comment: "//  \t@error: \t TYPE  ", want: "TYPE", wantOk: true},
		{name: "category with digit", comment: "// @error: DIV0", want: "DIV0", wantOk: true},
		{name: "unknown category", comment: "// @error: XXXX", wantErr: true},
		{name: "lowercase category", comment: "// @error: conv", wantErr: true},
		{name: "ordinary event", comment: "// @error: tcfi", wantErr: true},
		{name: "category written by k3 run", comment: "// @error: VMIS", wantErr: true},
		{name: "own mismatch category", comment: "// @error: EMIS", wantErr: true},
		{name: "empty category", comment: "// @error:", wantErr: true},
		{name: "trailing text after category", comment: "// @error: CONV value too big", wantErr: true},
		{name: "two on one line", comment: "// @error: CONV // @error: DOME", wantErr: true},
		{name: "text after bare tag", comment: "// @error CONV", wantErr: true},
		{name: "block comment", comment: "/* @error: CONV */"},
		{name: "not at the start", comment: "// see @error: CONV"},
		{name: "longer tag", comment: "// @errors: CONV"},
		{name: "other tag", comment: "// @output: 1"},
		{name: "plain comment", comment: "// error: CONV"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := ParseExpectedError(tt.comment)
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
