package wire

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// MaxBinaryFrameSize is the upper bound on a single binary-framed
// message. 16 MiB is comfortably above anything ntt produces today
// and well below what any sane peer would emit; values above it
// indicate a corrupted stream or a misaligned reader.
const MaxBinaryFrameSize = 16 * 1024 * 1024

// MagicTag marks every binary frame so a peer can quickly tell
// JSON-line traffic apart from binary traffic when both share a
// socket (for example during a protocol upgrade negotiation).
const MagicTag uint16 = 0x6E74 // "nt"

// EncodeBinary writes m to w in the binary framing format:
//
//	+--------+--------+--------+--------+
//	| magic (2)| version(2) | length(4) |
//	+--------+--------+--------+--------+
//	|       payload (length bytes)        |
//	+--------+--------+--------+----------+
//
// The payload is the same JSON document that NewEncoder.Encode would
// produce; the difference is that the length-prefix lets readers
// skip messages they don't understand without re-tokenising.
func EncodeBinary(w io.Writer, m Message) error {
	body, err := marshalMessage(m)
	if err != nil {
		return err
	}
	if len(body) > MaxBinaryFrameSize {
		return fmt.Errorf("wire: message of %d bytes exceeds %d max", len(body), MaxBinaryFrameSize)
	}
	var hdr [8]byte
	binary.BigEndian.PutUint16(hdr[0:], MagicTag)
	binary.BigEndian.PutUint16(hdr[2:], 1) // protocol version
	binary.BigEndian.PutUint32(hdr[4:], uint32(len(body)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err = w.Write(body)
	return err
}

// DecodeBinary reads one binary-framed message from r. It validates
// the magic and version bytes; a mismatch returns ErrBadMagic or
// ErrBadVersion so callers can fall back to JSON-line decoding if
// they want to support both.
func DecodeBinary(r io.Reader) (Message, error) {
	var hdr [8]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Message{}, err
	}
	magic := binary.BigEndian.Uint16(hdr[0:])
	if magic != MagicTag {
		return Message{}, ErrBadMagic
	}
	version := binary.BigEndian.Uint16(hdr[2:])
	if version != 1 {
		return Message{}, ErrBadVersion
	}
	length := binary.BigEndian.Uint32(hdr[4:])
	if length > MaxBinaryFrameSize {
		return Message{}, fmt.Errorf("wire: declared length %d exceeds max %d", length, MaxBinaryFrameSize)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return Message{}, err
	}
	return unmarshalMessage(body)
}

// ErrBadMagic is returned by DecodeBinary when the first two bytes
// do not match MagicTag. Callers can use it to detect that the peer
// is speaking JSON-line and switch decoders.
var ErrBadMagic = errors.New("wire: not a binary frame")

// ErrBadVersion is returned for an unrecognised protocol version.
var ErrBadVersion = errors.New("wire: unsupported binary protocol version")
