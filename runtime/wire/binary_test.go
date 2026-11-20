package wire_test

import (
	"bytes"
	"testing"

	"github.com/nokia/ntt/runtime/wire"
)

func TestBinary_RoundTrip(t *testing.T) {
	in := wire.Message{Kind: wire.RunKind, ID: "req-1", Body: []byte(`{"case":"M.tc"}`)}

	var buf bytes.Buffer
	if err := wire.EncodeBinary(&buf, in); err != nil {
		t.Fatalf("EncodeBinary: %v", err)
	}
	// First two bytes are the magic tag "nt".
	if buf.Bytes()[0] != 'n' || buf.Bytes()[1] != 't' {
		t.Errorf("magic = %x %x, want 'n' 't'", buf.Bytes()[0], buf.Bytes()[1])
	}

	out, err := wire.DecodeBinary(&buf)
	if err != nil {
		t.Fatalf("DecodeBinary: %v", err)
	}
	if out.Kind != in.Kind || out.ID != in.ID || string(out.Body) != string(in.Body) {
		t.Errorf("round trip mismatch: %+v != %+v", out, in)
	}
}

func TestBinary_RejectsBadMagic(t *testing.T) {
	// Send raw JSON, expect ErrBadMagic.
	src := bytes.NewBuffer([]byte("{\"kind\":\"run\"}\n"))
	_, err := wire.DecodeBinary(src)
	if err != wire.ErrBadMagic {
		t.Errorf("want ErrBadMagic, got %v", err)
	}
}

func TestBinary_RejectsOverlongLength(t *testing.T) {
	var buf bytes.Buffer
	// Magic, version, length=MaxBinaryFrameSize+1
	hdr := []byte{
		'n', 't',
		0x00, 0x01,
		0x01, 0x00, 0x00, 0x01,
	}
	buf.Write(hdr)
	_, err := wire.DecodeBinary(&buf)
	if err == nil {
		t.Errorf("expected error for overlong frame")
	}
}
