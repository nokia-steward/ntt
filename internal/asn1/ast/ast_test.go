package ast

import "testing"

func TestNode_PosEnd(t *testing.T) {
	n := Span{P: 5, E: 12}
	if n.Pos() != 5 || n.End() != 12 {
		t.Errorf("got Pos=%d End=%d", n.Pos(), n.End())
	}
	n.SetRange(1, 9)
	if n.Pos() != 1 || n.End() != 9 {
		t.Errorf("after SetRange: Pos=%d End=%d", n.Pos(), n.End())
	}
}

func TestTypeInterfaces(t *testing.T) {
	var _ Type = &BuiltinType{}
	var _ Type = &IntegerType{}
	var _ Type = &BitStringType{}
	var _ Type = &EnumeratedType{}
	var _ Type = &SequenceType{}
	var _ Type = &SetType{}
	var _ Type = &ChoiceType{}
	var _ Type = &SequenceOfType{}
	var _ Type = &SetOfType{}
	var _ Type = &TaggedType{}
	var _ Type = &ReferencedType{}
	var _ Type = &ConstrainedType{}
	var _ Type = &OpenTypeFieldType{}
	var _ Type = &AnyType{}
}

func TestValueInterfaces(t *testing.T) {
	var _ Value = &IntegerValue{}
	var _ Value = &RealValue{}
	var _ Value = &BooleanValue{}
	var _ Value = &NullValue{}
	var _ Value = &StringValue{}
	var _ Value = &OIDValue{}
	var _ Value = &ReferenceValue{}
	var _ Value = &ChoiceValue{}
	var _ Value = &SequenceValue{}
	var _ Value = &SequenceOfValue{}
}

func TestAssignmentInterfaces(t *testing.T) {
	var _ Assignment = &TypeAssignment{Name: "X"}
	var _ Assignment = &ValueAssignment{Name: "x"}
	var _ Assignment = &ValueSetTypeAssignment{Name: "X"}
	var _ Assignment = &ObjectClassAssignment{Name: "X"}
	var _ Assignment = &ObjectAssignment{Name: "x"}
	var _ Assignment = &ObjectSetAssignment{Name: "X"}
}

func TestConstraintElementInterfaces(t *testing.T) {
	var _ ConstraintElement = &SingleValueConstraint{}
	var _ ConstraintElement = &ValueRangeConstraint{}
	var _ ConstraintElement = &SizeConstraint{}
	var _ ConstraintElement = &AlphabetConstraint{}
	var _ ConstraintElement = &TypeConstraint{}
	var _ ConstraintElement = &ContainedSubtype{}
	var _ ConstraintElement = &PatternConstraint{}
	var _ ConstraintElement = &PropertySettings{}
	var _ ConstraintElement = &InnerTypeConstraint{}
	var _ ConstraintElement = &TableConstraint{}
	var _ ConstraintElement = &UserDefinedConstraint{}
	var _ ConstraintElement = &ExceptConstraint{}
	var _ ConstraintElement = &AllExceptConstraint{}
}

func TestFieldSpecInterfaces(t *testing.T) {
	var _ FieldSpec = &TypeFieldSpec{Name: "&Foo"}
	var _ FieldSpec = &FixedTypeValueFieldSpec{Name: "&foo"}
	var _ FieldSpec = &VariableTypeValueFieldSpec{Name: "&foo"}
	var _ FieldSpec = &FixedTypeValueSetFieldSpec{Name: "&Foo"}
	var _ FieldSpec = &VariableTypeValueSetFieldSpec{Name: "&Foo"}
	var _ FieldSpec = &ObjectFieldSpec{Name: "&foo"}
	var _ FieldSpec = &ObjectSetFieldSpec{Name: "&Foo"}
}

func TestAssignmentName(t *testing.T) {
	a := &TypeAssignment{Name: "Foo"}
	if AssignmentName(a) != "Foo" {
		t.Errorf("got %q", AssignmentName(a))
	}
}

func TestFieldName(t *testing.T) {
	f := &TypeFieldSpec{Name: "&Foo"}
	if FieldName(f) != "&Foo" {
		t.Errorf("got %q", FieldName(f))
	}
}
