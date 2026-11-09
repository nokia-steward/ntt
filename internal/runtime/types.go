package runtime

type Type interface {
	isType()
}

type (
	NamedType struct {
		Name string
		Type
		Attributes
	}

	Subtype struct {
		BaseType   Type
		Constraint any
	}

	UnionType struct {
		Fields []*Var
	}

	RecordType struct {
		Fields []*Var
	}

	SetType struct {
		Fields []*Var
	}

	RecordOfType struct {
		ElementType Type
	}

	SetOfType struct {
		ElementType Type
	}

	ArrayType struct {
		ElementType Type
		Length      any
	}

	EnumeratedType struct {
		Body any
	}

	MapType struct {
		KeyType   Type
		ValueType Type
	}

	ComponentType struct {
		Extends []Type
		Body    any
	}

	PortType struct {
		Address     Type
		InTypes     []Type
		OutTypes    []Type
		MapParams   []*Var
		UnmapParams []*Var
	}

	Signature struct {
		Parameters []*Var
		ReturnType Type
		RunsOn     Type
		System     Type
	}

	TemplateType struct {
		Flags
		BaseType Type
	}
)

func (typ *NamedType) isType()     {}
func (typ Subtype) isType()        {}
func (typ UnionType) isType()      {}
func (typ RecordType) isType()     {}
func (typ SetType) isType()        {}
func (typ RecordOfType) isType()   {}
func (typ SetOfType) isType()      {}
func (typ ArrayType) isType()      {}
func (typ EnumeratedType) isType() {}
func (typ MapType) isType()        {}
func (typ ComponentType) isType()  {}
func (typ PortType) isType()       {}
func (typ TemplateType) isType()   {}

type Attributes struct {
	Encode     Attribute
	Variant    Attribute
	Extensions []Attribute
}

type Attribute struct {
	Value    string
	Override bool
}

var (
	BitstringType           = &NamedType{Name: "bitstring"}
	BooleanType             = &NamedType{Name: "boolean"}
	CharstringType          = &NamedType{Name: "charstring"}
	UniversalCharstringType = &NamedType{Name: "universal charstring"}
	Closure                 = &NamedType{Name: "closure"}
	DefaultType             = &NamedType{Name: "default"}
	FloatType               = &NamedType{Name: "float"}
	HexstringType           = &NamedType{Name: "hexstring"}
	IntegerType             = &NamedType{Name: "integer"}
	OctetstringType         = &NamedType{Name: "octetstring"}
	TimerType               = &NamedType{Name: "timer"}
	VerdictType             = &NamedType{Name: "verdicttype"}
)
