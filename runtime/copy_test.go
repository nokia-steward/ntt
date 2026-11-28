package runtime

import "testing"

// TestShare: a shared value and all it holds are shared; a copy to change
// (Unshare) is not, and holds the same members, so that a change to it is
// not seen through the value it was taken of.
func TestShare(t *testing.T) {
	inner := NewRecordOf(NewInt(1), NewInt(2))
	rec := NewRecord()
	rec.Set("xs", inner)
	rec.Set("n", NewInt(3))
	m := NewMap()
	m.Set(NewCharstring("k"), rec)

	if IsShared(rec) || IsShared(inner) {
		t.Fatal("a value is shared before it is given to another holder")
	}
	Share(m)
	for name, v := range map[string]Object{"map": m, "record": rec, "list": inner} {
		if !IsShared(v) {
			t.Errorf("%s held by a shared map is not shared", name)
		}
	}

	own := Unshare(rec).(*Record)
	if own == rec || IsShared(own) {
		t.Fatal("Unshare of a shared record did not give a copy of its own")
	}
	if own.Fields["xs"] != inner {
		t.Error("the copy does not hold the same, shared, members")
	}
	own.Set("n", NewInt(4))
	if got := rec.Fields["n"].Inspect(); got != "3" {
		t.Errorf("the original's field changed to %s through its copy", got)
	}
	if Unshare(own) != own {
		t.Error("Unshare of a value of its own is not the value itself")
	}

	l := Unshare(inner).(*List)
	l.Elements[0] = NewInt(9)
	if got := inner.Elements[0].Inspect(); got != "1" {
		t.Errorf("the original's element changed to %s through its copy", got)
	}
}

// TestCheckShared: with the check on, a shared value changed in place is
// reported, and one left alone is not.
func TestCheckShared(t *testing.T) {
	was := sharedCheck
	sharedCheck = true
	defer func() { sharedCheck = was }()
	CheckShared()

	kept := NewRecordOf(NewInt(1))
	Share(kept)
	if bad := CheckShared(); len(bad) != 0 {
		t.Fatalf("CheckShared() = %v for a value left alone", bad)
	}

	changed := NewRecordOf(NewInt(1))
	Share(changed)
	changed.Elements[0] = NewInt(2)
	if bad := CheckShared(); len(bad) != 1 {
		t.Fatalf("CheckShared() = %v, want the value changed in place", bad)
	}
}
