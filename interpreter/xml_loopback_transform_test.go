package interpreter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nokia/ntt/runtime"
)

func TestXMLLoopbackTransformParsesWhitespaceFacet(t *testing.T) {
	path := writeTempXSD(t, `<schema xmlns="http://www.w3.org/2001/XMLSchema">
		<element name="test"><simpleType><restriction base="string">
			<whiteSpace value="collapse"/>
		</restriction></simpleType></element>
	</schema>`)
	tr, ok := parseXMLLoopbackTransform(path)
	if !ok {
		t.Fatal("expected XML loopback transform")
	}
	if tr.TypeName != "Test" || tr.WhiteSpace != "collapse" {
		t.Fatalf("unexpected transform: %#v", tr)
	}

	got := applyXMLLoopbackTransform(runtime.NewUniversalString("\t abc\r\n def"), tr)
	if s := got.(*runtime.String); string(s.Value) != "abc def" {
		t.Fatalf("collapse transform = %q", string(s.Value))
	}
}

func TestXMLLoopbackTransformTruncatesFractionDigits(t *testing.T) {
	tr := &xmlLoopbackTransform{TypeName: "ActualTemp", FractionDigits: 1}
	got := applyXMLLoopbackTransform(runtime.Float(99.99), tr)
	if got != runtime.Float(99.9) {
		t.Fatalf("fraction transform = %s", got.Inspect())
	}
}

func TestXMLLoopbackTransformCollapsesAllOmitSequence(t *testing.T) {
	path := writeTempXSD(t, `<schema xmlns="http://www.w3.org/2001/XMLSchema">
		<element name="optionals_in_optional">
			<complexType><sequence minOccurs="0">
				<element name="elem1" minOccurs="0"/>
			</sequence></complexType>
		</element>
	</schema>`)
	tr, ok := parseXMLLoopbackTransform(path)
	if !ok {
		t.Fatal("expected XML loopback transform")
	}
	if tr.TypeName != "Optionals_in_optional" || tr.CollapseAllOmitField != "sequence" {
		t.Fatalf("unexpected transform: %#v", tr)
	}

	seq := runtime.NewRecord()
	seq.Fields["elem1"] = runtime.Omit
	rec := runtime.NewRecord()
	rec.Fields["sequence"] = seq

	got := applyXMLLoopbackTransform(rec, tr).(*runtime.Record)
	if got.Fields["sequence"] != runtime.Omit {
		t.Fatalf("sequence field = %s", got.Fields["sequence"].Inspect())
	}
}

func writeTempXSD(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "schema.xsd")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
