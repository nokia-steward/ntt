package ttcn3_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/ttcn3"
)

const sampleASN1 = `Sample-Mod DEFINITIONS ::= BEGIN
Counter ::= INTEGER (0..65535)
Status ::= ENUMERATED { ok, error }
END`

func TestDB_ASN1Location(t *testing.T) {
	fs.SetContent("sample-mod.asn", []byte(sampleASN1))

	db := ttcn3.DB{}
	db.Index("sample-mod.asn")

	// Both top-level assignments should be indexed under their bare
	// name and locatable via ASN1Location.
	for _, name := range []string{"Counter", "Status"} {
		path, off, ok := db.ASN1Location(name)
		if !ok {
			t.Errorf("ASN1Location(%q): not found in db (Names=%v)", name, db.Names)
			continue
		}
		if !strings.HasSuffix(path, "sample-mod.asn") {
			t.Errorf("ASN1Location(%q): got file %q, want sample-mod.asn", name, path)
		}
		got := sampleASN1[off : off+len(name)]
		if got != name {
			t.Errorf("ASN1Location(%q): offset %d points to %q", name, off, got)
		}
	}

	// A name not in the file should miss cleanly.
	if _, _, ok := db.ASN1Location("Nope"); ok {
		t.Error("ASN1Location(Nope) returned ok=true")
	}
}
