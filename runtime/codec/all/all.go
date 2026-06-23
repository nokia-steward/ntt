// Package all is a side-effect-only umbrella that imports every
// codec implementation so a single `import _ "runtime/codec/all"`
// wires the global codec.Registry up. The list is the source of
// truth for what the runtime ships with; new codec families are
// added by appending one underscore-import here.
package all

import (
	_ "github.com/nokia/ntt/runtime/codec/asn1"
	_ "github.com/nokia/ntt/runtime/codec/ber"
	_ "github.com/nokia/ntt/runtime/codec/json"
	_ "github.com/nokia/ntt/runtime/codec/oer"
	_ "github.com/nokia/ntt/runtime/codec/raw"
	_ "github.com/nokia/ntt/runtime/codec/text"
)
