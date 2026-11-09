package opcode

// Arg describes the kind of operand encoded with an opcode in the instruction stream.
type Arg string

const (
	ArgNone    Arg = ""
	ArgRef     Arg = "ref"
	ArgLine    Arg = "line"
	ArgImm     Arg = "imm"
	ArgInt     Arg = "int"
	ArgFloat   Arg = "float"
	ArgString  Arg = "string"
	ArgBits    Arg = "bits"
	ArgNibbles Arg = "nibbles"
	ArgOctets  Arg = "octets"
)

// Descriptor describes a t3xf opcode.
type Descriptor struct {
	Opcode int // The opcode number
	Arg    Arg `json:",omitempty"`

	// Context describe the contexts in which the instruction can be used.
	Context []string `json:",omitempty"`

	// Operations describe the stack operations that the instruction performs.
	Operations  Operation
	Description string // A description of the opcode
}

// Operation describes a stack operation. Pre is the stack before the
// operation, Post is the stack after the operation.
type Operation struct {
	Pre  []string `yaml:",flow"`
	Post []string `yaml:",flow"`
}
