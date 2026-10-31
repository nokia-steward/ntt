package ttcn3_test

import (
	"testing"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/ttcn3"
)

var (
	file1 = `
	module M1 { import from M2 all }
	module M2 { type enumerated E { E1 } }`

	file2 = `
	module M1 { type component C { var integer x := E}; const int E }
	module M3 { const integer E, E1 := 1 }`

	file3 = `module MX { }`
)

func init() {
	fs.SetContent("file1.ttcn3", []byte(file1))
	fs.SetContent("file2.ttcn3", []byte(file2))
	fs.SetContent("file3.ttcn3", []byte(file3))
}

func TestIndex(t *testing.T) {
	db := ttcn3.DB{}
	db.Index("file1.ttcn3", "file2.ttcn3")
	testMapsEqual(t, makeSliceMap(db.Modules), SliceMap{
		"M1": []string{"file1.ttcn3", "file2.ttcn3"},
		"M2": []string{"file1.ttcn3"},
		"M3": []string{"file2.ttcn3"},
	})
	testMapsEqual(t, makeSliceMap(db.Names), SliceMap{
		"M1": []string{"file1.ttcn3", "file2.ttcn3"},
		"M2": []string{"file1.ttcn3"},
		"x":  []string{"file2.ttcn3"},
		"E":  []string{"file1.ttcn3", "file2.ttcn3"},
		"E1": []string{"file1.ttcn3", "file2.ttcn3"},
		"C":  []string{"file2.ttcn3"},
		"M3": []string{"file2.ttcn3"},
	})

}

func TestFindVisibleModules(t *testing.T) {

	t.Run("empty", func(t *testing.T) {
		db := ttcn3.DB{}
		mod := moduleFrom("file1.ttcn3", "M1")
		if defs := db.VisibleModules("E", mod); len(defs) != 0 {
			t.Errorf("Expected 0 definitions, got %v", defs)
		}
	})

	t.Run("regular", func(t *testing.T) {
		db := ttcn3.DB{}
		db.Index("file1.ttcn3", "file2.ttcn3", "file3.ttcn3")

		expected := []string{"M2:file1.ttcn3", "M1:file1.ttcn3", "M1:file2.ttcn3"}
		actual := importedDefs(&db, "E", "M1")
		if !equal(actual, expected) {
			t.Errorf("Mismatch:\n\twant=%v,\n\t got=%v", expected, actual)
		}
	})

	t.Run("false positive", func(t *testing.T) {
		db := ttcn3.DB{}
		db.Index("file1.ttcn3", "file2.ttcn3", "file3.ttcn3")

		db.Modules["M2"]["file3.ttcn3"] = true // false positive entry
		db.Names["E"]["file3.ttcn3"] = true    // false positive entry

		expected := []string{"M2:file1.ttcn3", "M1:file1.ttcn3", "M1:file2.ttcn3"}
		actual := importedDefs(&db, "E", "M1")
		if !equal(actual, expected) {
			t.Errorf("Mismatch:\n\twant=%v,\n\t got=%v", expected, actual)
		}
	})

}
