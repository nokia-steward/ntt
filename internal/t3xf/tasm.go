package t3xf

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"github.com/nokia/ntt/internal/t3xf/opcode"
	"os"
)

type RefMapper interface {
	GetTextRefAndLength() (uint32, uint32)
}

type T3xfDumper interface {
	PrintInstruction(int, int, []byte, opcode.Opcode, interface{})
	AcquireRefs(int, int)
	IncIndent()
	DecIndent()
	ToggleExpandBlocks(bool)
}

type StringRef string

func (StringRef) GetTextRefAndLength() (offset uint32, length uint32) {
	return 0, 0
}

type fileMap struct {
	T3xf             uint32
	Text             uint32
	Int32s           uint32
	Names            uint32
	Modules          uint32
	TypeAliass       uint32
	RecordTypes      uint32
	SetTypes         uint32
	RecordOfTypes    uint32
	SetOfTypes       uint32
	UnionTypes       uint32
	EnumeratedTypes  uint32
	ArrayTypes       uint32
	ClosureTypes     uint32
	MessagePortTypes uint32
	ComponentTypes   uint32
	Consts           uint32
	ModulePars       uint32
	Templates        uint32
	Testcases        uint32
	Functions        uint32
	ExtFunctions     uint32
	Altsteps         uint32
	Interleavesteps  uint32
	Blocks           uint32
	Controls         uint32
	Strings          uint32
	Collections      uint32
	MapTypes         uint32
	Data             uint32
}

type Int32Entry struct {
	AssetOffset uint32
	Value       int32
}

type NameEntry struct {
	AssetOffset uint32
	StringId    uint32
}

type ModuleEntry struct {
	ModuleStringId    uint32
	SourceStringId    uint32
	BlockId           uint32
	HasControlSection uint16
}

type TypeAliasEntry struct {
	AssetOffset        uint32
	ModuleId           uint16
	NameStringId       uint32
	RootTypeFileOffset uint32
	WithBlockId        uint32
	ConstraintBlockId  uint32
	LineNumber         uint32
}

type RecordTypeEntry struct {
	AssetOffset       uint32
	ModuleId          uint16
	NameStringId      uint32
	DefinitionBlockId uint32
	WithBlockId       uint32
	ConstraintBlockId uint32
	LineNumber        uint32
}

type SetTypeEntry struct {
	AssetOffset       uint32
	ModuleId          uint16
	NameStringId      uint32
	DefinitionBlockId uint32
	ConstraintBlockId uint32
	WithBlockId       uint32
	LineNumber        uint32
}

type RecordOfTypeEntry struct {
	AssetOffset       uint32
	ModuleId          uint16
	NameStringId      uint32
	DefinitionBlockId uint32
	ConstraintBlockId uint32
	WithBlockId       uint32
	LineNumber        uint32
}

type SetOfTypeEntry struct {
	AssetOffset           uint32
	ModuleId              uint16
	NameStringId          uint32
	ElementTypeFileOffset uint32
	ConstraintBlockId     uint32
	WithBlockId           uint32
	LineNumber            uint32
}

type MapTypeEntry struct {
	AssetOffset            uint32
	ModuleId               uint16
	NameStringId           uint32
	FromDefinitionBlockId  uint32
	ToDefinitionBlockId    uint32
	FromRefFileOffset      uint32
	SetOfFromRefFileOffset uint32
	ToRefFileOffset        uint32
	SetOfToRefFileOffset   uint32
	WithBlockId            uint32
	LineNumber             uint32
}

type UnionTypeEntry struct {
	AssetOffset       uint32
	ModuleId          uint16
	NameStringId      uint32
	DefinitionBlockId uint32
	WithBlockId       uint32
	ConstraintBlockId uint32
	LineNumber        uint32
}

type EnumeratedTypeEntry struct {
	AssetOffset       uint32
	ModuleId          uint16
	NameStringId      uint32
	DefinitionBlockId uint32
	WithBlockId       uint32
	ConstraintBlockId uint32
	LineNumber        uint32
}

type ArrayTypeEntry struct {
	AssetOffset           uint32
	ModuleId              uint16
	NameStringId          uint32
	ElementTypeFileOffset uint32
	DimensionBlockId      uint32
	WithBlockId           uint32
	LineNumber            uint32
}

type ClosureTypeEntry struct {
	NameStringId uint32
	ModuleId     uint16
	AssetOffset  uint32
	WithBlockId  uint32
	LineNumber   uint32
}

type MessagePortTypeEntry struct {
	AssetOffset                  uint32
	ModuleId                     uint16
	NameStringId                 uint32
	InDefinitionBlockId          uint32
	OutDefinitionBlockId         uint32
	MapParamsDefinitionBlockId   uint32
	UnmapParamsDefinitionBlockId uint32
	AddressType                  uint32
	WithBlockId                  uint32
	LineNumber                   uint32
}

type ComponentTypeEntry struct {
	AssetOffset                uint32
	ModuleId                   uint16
	NameStringId               uint32
	ExtendingComponentsBlockId uint32
	InitialisationBlockId      uint32
	WithBlockId                uint32
	LineNumber                 uint32
}

type ConstEntry struct {
	AssetOffset           uint32
	ModuleId              uint16
	NameStringId          uint32
	TypeFileOffset        uint32
	InitialisationBlockId uint32
	WithBlockId           uint32
	LineNumber            uint32
}

type ModuleParEntry struct {
	AssetOffset           uint32
	ModuleId              uint16
	NameStringId          uint32
	TypeFileOffset        uint32
	InitialisationBlockId uint32
	WithBlockId           uint32
	LineNumber            uint32
}

type TemplateEntry struct {
	AssetOffset           uint32
	ModuleId              uint16
	NameStringId          uint32
	TypeFileOffset        uint32
	ParamsBlockId         uint32
	InitialisationBlockId uint32
	LineNumber            uint32
}

type TestcaseEntry struct {
	NameStringId     uint32
	ModuleId         uint16
	RunsOnFileOffset uint32
	SystemFileOffset uint32
	BodyBlockId      uint32
	ParamsBlockId    uint32
	AssetOffset      uint32
	LineNumber       uint32
}

type FunctionEntry struct {
	NameStringId         uint32
	ModuleId             uint16
	RunsOnFileOffset     uint32
	ReturnTypeFileOffset uint32
	ReturnTypeFlags      uint16
	BodyBlockId          uint32
	ParamsBlockId        uint32
	AssetOffset          uint32
	LineNumber           uint32
}

type ExtFunctionEntry struct {
	NameStringId         uint32
	ModuleId             uint16
	ReturnTypeFileOffset uint32
	ReturnTypeFlags      uint16
	ParamsBlockId        uint32
	AssetOffset          uint32
	WithBlockId          uint32
	LineNumber           uint32
}

type AltstepEntry struct {
	NameStringId     uint32
	ModuleId         uint16
	RunsOnFileOffset uint32
	BodyBlockId      uint32
	ParamsBlockId    uint32
	AssetOffset      uint32
	LineNumber       uint32
}

type BlockEntry struct {
	AssetOffset uint32
	TextOffset  uint32
	Length      uint32
}

func (block BlockEntry) GetTextRefAndLength() (offset uint32, length uint32) {
	return block.TextOffset, block.Length
}

type ControlEntry struct {
	AssetOffset uint32
	ModuleId    uint16
	BlockId     uint32
	Length      uint32
	LineNumber  uint32
}

type StringEntry struct {
	DataOffset uint32
	Length     uint32
}

type CollectionEntry struct {
	DataOffset uint32
	Length     uint32
}

type File struct {
	T3xf             []byte
	Text             []byte
	Int32s           []Int32Entry
	Names            []NameEntry
	Modules          []ModuleEntry
	TypeAliass       []TypeAliasEntry
	RecordTypes      []RecordTypeEntry
	SetTypes         []SetTypeEntry
	RecordOfTypes    []RecordOfTypeEntry
	SetOfTypes       []SetOfTypeEntry
	UnionTypes       []UnionTypeEntry
	EnumeratedTypes  []EnumeratedTypeEntry
	ArrayTypes       []ArrayTypeEntry
	ClosureTypes     []ClosureTypeEntry
	MessagePortTypes []MessagePortTypeEntry
	ComponentTypes   []ComponentTypeEntry
	Consts           []ConstEntry
	ModulePars       []ModuleParEntry
	Templates        []TemplateEntry
	Testcases        []TestcaseEntry
	Functions        []FunctionEntry
	ExtFunctions     []ExtFunctionEntry
	Altsteps         []AltstepEntry
	Interleavesteps  []AltstepEntry
	Blocks           []BlockEntry
	Controls         []ControlEntry
	Strings          []StringEntry
	Collections      []CollectionEntry
	MapTypes         []MapTypeEntry
	Data             []byte
	assetOffsets     map[uint32]RefMapper
}

func Open(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Read(f)
}

func Read(r io.Reader) (*File, error) {

	magic := make([]byte, 8)
	if err := binary.Read(r, binary.LittleEndian, &magic); err != nil {
		return nil, err
	}
	if bytes.Compare(magic, []byte("T3XFASM\x00")) != 0 {
		return nil, errors.New("invalid magic number")
	}

	header := fileMap{}
	if err := binary.Read(r, binary.LittleEndian, &header); err != nil {
		return nil, err
	}

	file := File{
		T3xf:             make([]byte, header.T3xf),
		Text:             make([]byte, header.Text),
		Int32s:           make([]Int32Entry, int(header.Int32s)/binary.Size(Int32Entry{})),
		Names:            make([]NameEntry, int(header.Names)/binary.Size(NameEntry{})),
		Modules:          make([]ModuleEntry, int(header.Modules)/binary.Size(ModuleEntry{})),
		TypeAliass:       make([]TypeAliasEntry, int(header.TypeAliass)/binary.Size(TypeAliasEntry{})),
		RecordTypes:      make([]RecordTypeEntry, int(header.RecordTypes)/binary.Size(RecordTypeEntry{})),
		SetTypes:         make([]SetTypeEntry, int(header.SetTypes)/binary.Size(SetTypeEntry{})),
		RecordOfTypes:    make([]RecordOfTypeEntry, int(header.RecordOfTypes)/binary.Size(RecordOfTypeEntry{})),
		SetOfTypes:       make([]SetOfTypeEntry, int(header.SetOfTypes)/binary.Size(SetOfTypeEntry{})),
		UnionTypes:       make([]UnionTypeEntry, int(header.UnionTypes)/binary.Size(UnionTypeEntry{})),
		EnumeratedTypes:  make([]EnumeratedTypeEntry, int(header.EnumeratedTypes)/binary.Size(EnumeratedTypeEntry{})),
		ArrayTypes:       make([]ArrayTypeEntry, int(header.ArrayTypes)/binary.Size(ArrayTypeEntry{})),
		ClosureTypes:     make([]ClosureTypeEntry, int(header.ClosureTypes)/binary.Size(ClosureTypeEntry{})),
		MessagePortTypes: make([]MessagePortTypeEntry, int(header.MessagePortTypes)/binary.Size(MessagePortTypeEntry{})),
		ComponentTypes:   make([]ComponentTypeEntry, int(header.ComponentTypes)/binary.Size(ComponentTypeEntry{})),
		Consts:           make([]ConstEntry, int(header.Consts)/binary.Size(ConstEntry{})),
		ModulePars:       make([]ModuleParEntry, int(header.ModulePars)/binary.Size(ModuleParEntry{})),
		Templates:        make([]TemplateEntry, int(header.Templates)/binary.Size(TemplateEntry{})),
		Testcases:        make([]TestcaseEntry, int(header.Testcases)/binary.Size(TestcaseEntry{})),
		Functions:        make([]FunctionEntry, int(header.Functions)/binary.Size(FunctionEntry{})),
		ExtFunctions:     make([]ExtFunctionEntry, int(header.ExtFunctions)/binary.Size(ExtFunctionEntry{})),
		Altsteps:         make([]AltstepEntry, int(header.Altsteps)/binary.Size(AltstepEntry{})),
		Interleavesteps:  make([]AltstepEntry, int(header.Interleavesteps)/binary.Size(AltstepEntry{})),
		MapTypes:         make([]MapTypeEntry, int(header.MapTypes)/binary.Size(MapTypeEntry{})),
		Blocks:           make([]BlockEntry, int(header.Blocks)/binary.Size(BlockEntry{})),
		Controls:         make([]ControlEntry, int(header.Controls)/binary.Size(ControlEntry{})),
		Strings:          make([]StringEntry, int(header.Strings)/binary.Size(StringEntry{})),
		Collections:      make([]CollectionEntry, int(header.Collections)/binary.Size(CollectionEntry{})),
		Data:             make([]byte, header.Data),
		assetOffsets:     make(map[uint32]RefMapper),
	}

	var err error
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.T3xf))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.Text))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.Int32s))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.Names))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.Modules))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.TypeAliass))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.RecordTypes))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.SetTypes))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.RecordOfTypes))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.SetOfTypes))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.UnionTypes))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.EnumeratedTypes))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.ArrayTypes))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.ClosureTypes))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.MessagePortTypes))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.ComponentTypes))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.Consts))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.ModulePars))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.Templates))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.Testcases))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.Functions))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.ExtFunctions))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.Altsteps))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.Interleavesteps))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.Blocks))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.Controls))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.Strings))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.Collections))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.MapTypes))
	err = errors.Join(err, binary.Read(r, binary.LittleEndian, &file.Data))

	if err != nil {
		return nil, err
	}

	return &file, nil
}

func TasmHeaderSize() int {
	return binary.Size(fileMap{}) + 8
}

func (f *File) assetBase() uint32 {
	return uint32(TasmHeaderSize()) + uint32(len(f.T3xf)) + uint32(len(f.Text))
}

// dataBase returns the absolute file offset of the Data section.
// StringEntry.DataOffset and CollectionEntry.DataOffset are relative to this base.
func (f *File) dataBase() uint32 {
	return f.assetBase() +
		uint32(binary.Size(Int32Entry{})*len(f.Int32s)) +
		uint32(binary.Size(NameEntry{})*len(f.Names)) +
		uint32(binary.Size(ModuleEntry{})*len(f.Modules)) +
		uint32(binary.Size(TypeAliasEntry{})*len(f.TypeAliass)) +
		uint32(binary.Size(RecordTypeEntry{})*len(f.RecordTypes)) +
		uint32(binary.Size(SetTypeEntry{})*len(f.SetTypes)) +
		uint32(binary.Size(RecordOfTypeEntry{})*len(f.RecordOfTypes)) +
		uint32(binary.Size(SetOfTypeEntry{})*len(f.SetOfTypes)) +
		uint32(binary.Size(UnionTypeEntry{})*len(f.UnionTypes)) +
		uint32(binary.Size(EnumeratedTypeEntry{})*len(f.EnumeratedTypes)) +
		uint32(binary.Size(ArrayTypeEntry{})*len(f.ArrayTypes)) +
		uint32(binary.Size(ClosureTypeEntry{})*len(f.ClosureTypes)) +
		uint32(binary.Size(MessagePortTypeEntry{})*len(f.MessagePortTypes)) +
		uint32(binary.Size(ComponentTypeEntry{})*len(f.ComponentTypes)) +
		uint32(binary.Size(ConstEntry{})*len(f.Consts)) +
		uint32(binary.Size(ModuleParEntry{})*len(f.ModulePars)) +
		uint32(binary.Size(TemplateEntry{})*len(f.Templates)) +
		uint32(binary.Size(TestcaseEntry{})*len(f.Testcases)) +
		uint32(binary.Size(FunctionEntry{})*len(f.Functions)) +
		uint32(binary.Size(ExtFunctionEntry{})*len(f.ExtFunctions)) +
		uint32(binary.Size(AltstepEntry{})*len(f.Altsteps)) +
		uint32(binary.Size(AltstepEntry{})*len(f.Interleavesteps)) +
		uint32(binary.Size(BlockEntry{})*len(f.Blocks)) +
		uint32(binary.Size(ControlEntry{})*len(f.Controls)) +
		uint32(binary.Size(StringEntry{})*len(f.Strings)) +
		uint32(binary.Size(CollectionEntry{})*len(f.Collections)) +
		uint32(binary.Size(MapTypeEntry{})*len(f.MapTypes))
}

func (f *File) printBlockContent(w io.Writer, dumper T3xfDumper, blockId uint32) {
	if blockId == 0 || int(blockId) > len(f.Blocks) {
		return
	}
	blk := f.Blocks[blockId-1]
	for offset := int(blk.TextOffset); offset < int(blk.TextOffset)+int(blk.Length); {
		n, op, arg := Decode(f.Text[offset:])
		dumper.IncIndent()
		dumper.PrintInstruction(TasmHeaderSize()+len(f.T3xf), offset, f.Text, op, arg)
		dumper.DecIndent()
		fmt.Fprintf(w, "\n")
		offset += n
	}
}

func (f *File) PrintAltstepsSection(w io.Writer, dumper T3xfDumper, expand bool) {
	fmt.Fprintln(w, "Altsteps Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Offset  : Name (StringId) : ParamsBlockId : BodyBlockId")
	dumper.ToggleExpandBlocks(true)
	ab := f.assetBase()
	for i, fn := range f.Altsteps {
		correspondingStr := f.Strings[fn.NameStringId-1]
		fmt.Fprintf(w, "  %4d: @%08x %q (%d) : %d : %d\n", i+1, fn.AssetOffset+ab,
			string(f.Data[correspondingStr.DataOffset:correspondingStr.DataOffset+correspondingStr.Length]), fn.NameStringId, fn.ParamsBlockId, fn.BodyBlockId)
		if expand {
			f.printBlockContent(w, dumper, fn.ParamsBlockId)
			f.printBlockContent(w, dumper, fn.BodyBlockId)
		}
	}
	if len(f.Altsteps) != 0 {
		fmt.Fprintf(w, "\n")
	}
}

func (f *File) PrintBlocksSection(w io.Writer, textSectionOffset int) {
	fmt.Fprintln(w, "Blocks Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Asset Offset : Text Offset, Length")
	ab := f.assetBase()
	for i, n := range f.Blocks {
		fmt.Fprintf(w, "  %4d: @%08x Text@%08x %8d\n", i+1, n.AssetOffset+ab, n.TextOffset+uint32(textSectionOffset), n.Length)
	}
}

func (f *File) PrintFunctionsSection(w io.Writer, dumper T3xfDumper, expand bool) {
	fmt.Fprintln(w, "Functions Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Asset Offset  : Name (StringId) : ParamsBlockId : BodyBlockId")
	dumper.ToggleExpandBlocks(true)
	ab := f.assetBase()
	for i, fn := range f.Functions {
		correspondingStr := f.Strings[fn.NameStringId-1]
		fmt.Fprintf(w, "  %4d: @%08x %q (%d) : %d : %d\n", i+1, fn.AssetOffset+ab,
			string(f.Data[correspondingStr.DataOffset:correspondingStr.DataOffset+correspondingStr.Length]), fn.NameStringId, fn.ParamsBlockId, fn.BodyBlockId)
		if expand {
			f.printBlockContent(w, dumper, fn.ParamsBlockId)
			f.printBlockContent(w, dumper, fn.BodyBlockId)
		}
	}
	if len(f.Functions) != 0 {
		fmt.Fprintf(w, "\n")
	}
}

func (f *File) PrintInterleaveStepsSection(w io.Writer, dumper T3xfDumper, expand bool) {
	fmt.Fprintln(w, "Interleave Steps Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Offset  : Name (StringId) : ParamsBlockId : BodyBlockId")
	dumper.ToggleExpandBlocks(true)
	ab := f.assetBase()
	for i, fn := range f.Interleavesteps {
		correspondingStr := f.Strings[fn.NameStringId-1]
		fmt.Fprintf(w, "  %4d: @%08x %q (%d) : %d : %d\n", i+1, fn.AssetOffset+ab,
			string(f.Data[correspondingStr.DataOffset:correspondingStr.DataOffset+correspondingStr.Length]), fn.NameStringId, fn.ParamsBlockId, fn.BodyBlockId)
		if expand {
			f.printBlockContent(w, dumper, fn.ParamsBlockId)
			f.printBlockContent(w, dumper, fn.BodyBlockId)
		}
	}
	if len(f.Interleavesteps) != 0 {
		fmt.Fprintf(w, "\n")
	}
}

func (f *File) PrintModulesSection(w io.Writer) {
	fmt.Fprintln(w, "Modules Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Name (StringId) : Source (StringId) : BlockId : HasControlSection")
	for i, m := range f.Modules {
		nameStr := f.Strings[m.ModuleStringId-1]
		sourceStr := f.Strings[m.SourceStringId-1]
		fmt.Fprintf(w, "  %4d: %q (%d) : %q (%d) : %d : %t\n", i+1,
			string(f.Data[nameStr.DataOffset:nameStr.DataOffset+nameStr.Length]), m.ModuleStringId,
			string(f.Data[sourceStr.DataOffset:sourceStr.DataOffset+sourceStr.Length]), m.SourceStringId,
			m.BlockId, m.HasControlSection != 0)
	}
}

func (f *File) AcquireAssetOffsets() map[uint32]RefMapper {
	assetBase := f.assetBase()
	dataBase := f.dataBase()
	for _, fn := range f.Names {
		correspondingStr := f.Strings[fn.StringId-1]
		f.assetOffsets[fn.AssetOffset+assetBase] = StringRef(f.Data[correspondingStr.DataOffset : correspondingStr.DataOffset+correspondingStr.Length])
	}
	for _, fn := range f.Strings {
		f.assetOffsets[fn.DataOffset+dataBase] = StringRef(f.Data[fn.DataOffset : fn.DataOffset+fn.Length])
	}
	for _, fn := range f.Blocks {
		f.assetOffsets[fn.AssetOffset+assetBase] = fn
	}

	// helper: register a module-qualified name for an asset offset
	addNamed := func(assetOffset uint32, moduleId uint16, nameStringId uint32) {
		moduleStr := f.Strings[f.Modules[moduleId-1].ModuleStringId-1]
		nameStr := f.Strings[nameStringId-1]
		f.assetOffsets[assetOffset+assetBase] = StringRef(fmt.Sprintf("%s.%s",
			f.Data[moduleStr.DataOffset:moduleStr.DataOffset+moduleStr.Length],
			f.Data[nameStr.DataOffset:nameStr.DataOffset+nameStr.Length]))
	}

	for _, fn := range f.Functions {
		addNamed(fn.AssetOffset, fn.ModuleId, fn.NameStringId)
	}
	for _, fn := range f.ExtFunctions {
		addNamed(fn.AssetOffset, fn.ModuleId, fn.NameStringId)
	}
	for _, fn := range f.Altsteps {
		addNamed(fn.AssetOffset, fn.ModuleId, fn.NameStringId)
	}
	for _, fn := range f.Interleavesteps {
		addNamed(fn.AssetOffset, fn.ModuleId, fn.NameStringId)
	}
	for _, tc := range f.Testcases {
		addNamed(tc.AssetOffset, tc.ModuleId, tc.NameStringId)
	}
	for _, t := range f.Templates {
		addNamed(t.AssetOffset, t.ModuleId, t.NameStringId)
	}
	for _, c := range f.Consts {
		addNamed(c.AssetOffset, c.ModuleId, c.NameStringId)
	}
	for _, mp := range f.ModulePars {
		addNamed(mp.AssetOffset, mp.ModuleId, mp.NameStringId)
	}
	for _, t := range f.TypeAliass {
		addNamed(t.AssetOffset, t.ModuleId, t.NameStringId)
	}
	for _, t := range f.RecordTypes {
		addNamed(t.AssetOffset, t.ModuleId, t.NameStringId)
	}
	for _, t := range f.SetTypes {
		addNamed(t.AssetOffset, t.ModuleId, t.NameStringId)
	}
	for _, t := range f.RecordOfTypes {
		addNamed(t.AssetOffset, t.ModuleId, t.NameStringId)
	}
	for _, t := range f.SetOfTypes {
		addNamed(t.AssetOffset, t.ModuleId, t.NameStringId)
	}
	for _, t := range f.UnionTypes {
		addNamed(t.AssetOffset, t.ModuleId, t.NameStringId)
	}
	for _, t := range f.EnumeratedTypes {
		addNamed(t.AssetOffset, t.ModuleId, t.NameStringId)
	}
	for _, t := range f.ArrayTypes {
		addNamed(t.AssetOffset, t.ModuleId, t.NameStringId)
	}
	for _, t := range f.ClosureTypes {
		addNamed(t.AssetOffset, t.ModuleId, t.NameStringId)
	}
	for _, t := range f.MessagePortTypes {
		addNamed(t.AssetOffset, t.ModuleId, t.NameStringId)
	}
	for _, t := range f.ComponentTypes {
		addNamed(t.AssetOffset, t.ModuleId, t.NameStringId)
	}
	for _, t := range f.MapTypes {
		addNamed(t.AssetOffset, t.ModuleId, t.NameStringId)
	}
	for i, c := range f.Controls {
		moduleStr := f.Strings[f.Modules[c.ModuleId-1].ModuleStringId-1]
		f.assetOffsets[c.AssetOffset+assetBase] = StringRef(fmt.Sprintf("%s.control[%d]",
			f.Data[moduleStr.DataOffset:moduleStr.DataOffset+moduleStr.Length], i))
	}

	return f.assetOffsets
}

func (f *File) PrintNamesSection(w io.Writer) {
	fmt.Fprintln(w, "Names Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Asset Offset  : Name (StringId)")
	ab := f.assetBase()
	for i, n := range f.Names {
		correspondingStr := f.Strings[n.StringId-1]
		fmt.Fprintf(w, "  %4d: @%08x %q (%d)\n", i+1, n.AssetOffset+ab,
			string(f.Data[correspondingStr.DataOffset:correspondingStr.DataOffset+correspondingStr.Length]), n.StringId)
	}
}

func (f *File) PrintStringsSection(w io.Writer) {
	fmt.Fprintln(w, "Strings Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Offset  : String")
	db := f.dataBase()
	for i, s := range f.Strings {
		fmt.Fprintf(w, "  %4d: @%08x %q\n", i+1, s.DataOffset+db, string(f.Data[s.DataOffset:s.DataOffset+s.Length]))
	}
}

func (f *File) PrintTypeAliasSection(w io.Writer, dumper T3xfDumper, expand bool) {
	fmt.Fprintln(w, "TypeAliases Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Asset Offset  : Name (StringId) : RootTypeFileOffset : WithBlockId : ConstraintBlockId")
	ab := f.assetBase()
	for i, t := range f.TypeAliass {
		correspondingStr := f.Strings[t.NameStringId-1]
		fmt.Fprintf(w, "  %4d: @%08x %q (%d) : @%08x : %d : %d\n", i+1, t.AssetOffset+ab,
			string(f.Data[correspondingStr.DataOffset:correspondingStr.DataOffset+correspondingStr.Length]), t.NameStringId,
			t.RootTypeFileOffset, t.WithBlockId, t.ConstraintBlockId)
		if expand {
			f.printBlockContent(w, dumper, t.WithBlockId)
			f.printBlockContent(w, dumper, t.ConstraintBlockId)
		}
	}
}

func (f *File) PrintSectionStats(w io.Writer) {
	fmt.Fprintf(w, "# Section overview\n--------------------\n")
	fmt.Fprintf(w, "  T3xf section:              %8d bytes (hex: %x)\n", len(f.T3xf), len(f.T3xf))
	fmt.Fprintf(w, "  Text section:              %8d bytes (hex: %x) TextOffset: %08x\n", len(f.Text), len(f.Text), binary.Size(fileMap{}))
	fmt.Fprintf(w, "  Int32s section:            %8d entries (size: hex: %x)\n", len(f.Int32s), len(f.Int32s)*binary.Size(Int32Entry{}))
	fmt.Fprintf(w, "  Names section:             %8d entries (size: hex: %x)\n", len(f.Names), len(f.Names)*binary.Size(NameEntry{}))
	fmt.Fprintf(w, "  Modules section:           %8d entries (size: hex: %x)\n", len(f.Modules), len(f.Modules)*binary.Size(ModuleEntry{}))
	fmt.Fprintf(w, "  TypeAliases section:       %8d entries (size: hex: %x)\n", len(f.TypeAliass), len(f.TypeAliass)*binary.Size(TypeAliasEntry{}))
	fmt.Fprintf(w, "  RecordTypes section:       %8d entries (size: hex: %x)\n", len(f.RecordTypes), len(f.RecordTypes)*binary.Size(RecordTypeEntry{}))
	fmt.Fprintf(w, "  SetTypes section:          %8d entries (size: hex: %x)\n", len(f.SetTypes), len(f.SetTypes)*binary.Size(SetTypeEntry{}))
	fmt.Fprintf(w, "  RecordOfTypes section:     %8d entries (size: hex: %x)\n", len(f.RecordOfTypes), len(f.RecordOfTypes)*binary.Size(RecordOfTypeEntry{}))
	fmt.Fprintf(w, "  SetOfTypes section:        %8d entries (size: hex: %x)\n", len(f.SetOfTypes), len(f.SetOfTypes)*binary.Size(SetOfTypeEntry{}))
	fmt.Fprintf(w, "  UnionTypes section:        %8d entries (size: hex: %x)\n", len(f.UnionTypes), len(f.UnionTypes)*binary.Size(UnionTypeEntry{}))
	fmt.Fprintf(w, "  EnumeratedTypes section:   %8d entries (size: hex: %x)\n", len(f.EnumeratedTypes), len(f.EnumeratedTypes)*binary.Size(EnumeratedTypeEntry{}))
	fmt.Fprintf(w, "  ArrayTypes section:        %8d entries (size: hex: %x)\n", len(f.ArrayTypes), len(f.ArrayTypes)*binary.Size(ArrayTypeEntry{}))
	fmt.Fprintf(w, "  ClosureTypes section:      %8d entries (size: hex: %x)\n", len(f.ClosureTypes), len(f.ClosureTypes)*binary.Size(ClosureTypeEntry{}))
	fmt.Fprintf(w, "  MessagePortTypes section:  %8d entries (size: hex: %x)\n", len(f.MessagePortTypes), len(f.MessagePortTypes)*binary.Size(MessagePortTypeEntry{}))
	fmt.Fprintf(w, "  ComponentTypes section:    %8d entries (size: hex: %x)\n", len(f.ComponentTypes), len(f.ComponentTypes)*binary.Size(ComponentTypeEntry{}))
	fmt.Fprintf(w, "  Consts section:            %8d entries (size: hex: %x)\n", len(f.Consts), len(f.Consts)*binary.Size(ConstEntry{}))
	fmt.Fprintf(w, "  ModulePars section:        %8d entries (size: hex: %x)\n", len(f.ModulePars), len(f.ModulePars)*binary.Size(ModuleParEntry{}))
	fmt.Fprintf(w, "  Templates section:         %8d entries (size: hex: %x)\n", len(f.Templates), len(f.Templates)*binary.Size(TemplateEntry{}))
	fmt.Fprintf(w, "  Testcases section:         %8d entries (size: hex: %x)\n", len(f.Testcases), len(f.Testcases)*binary.Size(TestcaseEntry{}))
	fmt.Fprintf(w, "  Functions section:         %8d entries (size: hex: %x)\n", len(f.Functions), len(f.Functions)*binary.Size(FunctionEntry{}))
	fmt.Fprintf(w, "  ExtFunctions section:      %8d entries (size: hex: %x)\n", len(f.ExtFunctions), len(f.ExtFunctions)*binary.Size(ExtFunctionEntry{}))
	fmt.Fprintf(w, "  Altsteps section:          %8d entries (size: hex: %x)\n", len(f.Altsteps), len(f.Altsteps)*binary.Size(AltstepEntry{}))
	fmt.Fprintf(w, "  Interleaved steps section: %8d entries (size: hex: %x)\n", len(f.Interleavesteps), len(f.Interleavesteps)*binary.Size(AltstepEntry{}))
	fmt.Fprintf(w, "  Blocks section:            %8d entries (size: hex: %x)\n", len(f.Blocks), len(f.Blocks)*binary.Size(BlockEntry{}))
	fmt.Fprintf(w, "  Controls section:          %8d entries (size: hex: %x)\n", len(f.Controls), len(f.Controls)*binary.Size(ControlEntry{}))
	fmt.Fprintf(w, "  Strings section:           %8d entries (size: hex: %x)\n", len(f.Strings), len(f.Strings)*binary.Size(StringEntry{}))
	fmt.Fprintf(w, "  Collections section:       %8d entries (size: hex: %x)\n", len(f.Collections), len(f.Collections)*binary.Size(CollectionEntry{}))
	fmt.Fprintf(w, "  MapTypes section:          %8d entries (size: hex: %x)\n", len(f.MapTypes), len(f.MapTypes)*binary.Size(MapTypeEntry{}))
	fmt.Fprintf(w, "  Data section:              %8d bytes (hex: %x)\n\n", len(f.Data), len(f.Data))
}

func (f *File) PrintTestcasesSection(w io.Writer, dumper T3xfDumper, expand bool) {
	fmt.Fprintln(w, "Testcases Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Asset Offset  : Name (StringId) : ParamsBlockId : BodyBlockId")
	dumper.ToggleExpandBlocks(true)
	ab := f.assetBase()
	for i, tc := range f.Testcases {
		correspondingStr := f.Strings[tc.NameStringId-1]
		fmt.Fprintf(w, "  %4d: @%08x %q (%d) : %d : %d\n", i+1, tc.AssetOffset+ab,
			string(f.Data[correspondingStr.DataOffset:correspondingStr.DataOffset+correspondingStr.Length]), tc.NameStringId, tc.ParamsBlockId, tc.BodyBlockId)
		if expand {
			f.printBlockContent(w, dumper, tc.ParamsBlockId)
			f.printBlockContent(w, dumper, tc.BodyBlockId)
		}
	}
	if len(f.Testcases) != 0 {
		fmt.Fprintf(w, "\n")
	}
}

func (f *File) PrintConstsSection(w io.Writer, dumper T3xfDumper, expand bool) {
	fmt.Fprintln(w, "Consts Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Asset Offset  : Name (StringId) : TypeFileOffset : InitBlockId")
	dumper.ToggleExpandBlocks(true)
	ab := f.assetBase()
	for i, c := range f.Consts {
		correspondingStr := f.Strings[c.NameStringId-1]
		fmt.Fprintf(w, "  %4d: @%08x %q (%d) : @%08x : %d\n", i+1, c.AssetOffset+ab,
			string(f.Data[correspondingStr.DataOffset:correspondingStr.DataOffset+correspondingStr.Length]), c.NameStringId, c.TypeFileOffset, c.InitialisationBlockId)
		if expand {
			f.printBlockContent(w, dumper, c.InitialisationBlockId)
		}
	}
	if len(f.Consts) != 0 {
		fmt.Fprintf(w, "\n")
	}
}

func (f *File) PrintModuleParsSection(w io.Writer, dumper T3xfDumper, expand bool) {
	fmt.Fprintln(w, "ModulePars Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Asset Offset  : Name (StringId) : TypeFileOffset : InitBlockId")
	dumper.ToggleExpandBlocks(true)
	ab := f.assetBase()
	for i, mp := range f.ModulePars {
		correspondingStr := f.Strings[mp.NameStringId-1]
		fmt.Fprintf(w, "  %4d: @%08x %q (%d) : @%08x : %d\n", i+1, mp.AssetOffset+ab,
			string(f.Data[correspondingStr.DataOffset:correspondingStr.DataOffset+correspondingStr.Length]), mp.NameStringId, mp.TypeFileOffset, mp.InitialisationBlockId)
		if expand {
			f.printBlockContent(w, dumper, mp.InitialisationBlockId)
		}
	}
	if len(f.ModulePars) != 0 {
		fmt.Fprintf(w, "\n")
	}
}

func (f *File) PrintTemplatesSection(w io.Writer, dumper T3xfDumper, expand bool) {
	fmt.Fprintln(w, "Templates Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Asset Offset  : Name (StringId) : TypeFileOffset : ParamsBlockId : InitBlockId")
	dumper.ToggleExpandBlocks(true)
	ab := f.assetBase()
	for i, t := range f.Templates {
		correspondingStr := f.Strings[t.NameStringId-1]
		fmt.Fprintf(w, "  %4d: @%08x %q (%d) : @%08x : %d : %d\n", i+1, t.AssetOffset+ab,
			string(f.Data[correspondingStr.DataOffset:correspondingStr.DataOffset+correspondingStr.Length]), t.NameStringId, t.TypeFileOffset, t.ParamsBlockId, t.InitialisationBlockId)
		if expand {
			f.printBlockContent(w, dumper, t.ParamsBlockId)
			f.printBlockContent(w, dumper, t.InitialisationBlockId)
		}
	}
	if len(f.Templates) != 0 {
		fmt.Fprintf(w, "\n")
	}
}

func (f *File) PrintExtFunctionsSection(w io.Writer, dumper T3xfDumper, expand bool) {
	fmt.Fprintln(w, "ExtFunctions Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Asset Offset  : Name (StringId) : ReturnTypeFileOffset : ParamsBlockId")
	dumper.ToggleExpandBlocks(true)
	ab := f.assetBase()
	for i, fn := range f.ExtFunctions {
		correspondingStr := f.Strings[fn.NameStringId-1]
		fmt.Fprintf(w, "  %4d: @%08x %q (%d) : @%08x : %d\n", i+1, fn.AssetOffset+ab,
			string(f.Data[correspondingStr.DataOffset:correspondingStr.DataOffset+correspondingStr.Length]), fn.NameStringId, fn.ReturnTypeFileOffset, fn.ParamsBlockId)
		if expand {
			f.printBlockContent(w, dumper, fn.ParamsBlockId)
		}
	}
	if len(f.ExtFunctions) != 0 {
		fmt.Fprintf(w, "\n")
	}
}

func (f *File) PrintControlsSection(w io.Writer, dumper T3xfDumper, expand bool) {
	fmt.Fprintln(w, "Controls Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Asset Offset  : ModuleId : BlockId")
	dumper.ToggleExpandBlocks(true)
	ab := f.assetBase()
	for i, c := range f.Controls {
		fmt.Fprintf(w, "  %4d: @%08x %d : %d\n", i+1, c.AssetOffset+ab, c.ModuleId, c.BlockId)
		if expand {
			f.printBlockContent(w, dumper, c.BlockId)
		}
	}
	if len(f.Controls) != 0 {
		fmt.Fprintf(w, "\n")
	}
}

func (f *File) PrintRecordTypesSection(w io.Writer, dumper T3xfDumper, expand bool) {
	fmt.Fprintln(w, "RecordTypes Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Asset Offset  : Name (StringId) : DefinitionBlockId : WithBlockId : ConstraintBlockId")
	dumper.ToggleExpandBlocks(true)
	ab := f.assetBase()
	for i, t := range f.RecordTypes {
		correspondingStr := f.Strings[t.NameStringId-1]
		fmt.Fprintf(w, "  %4d: @%08x %q (%d) : %d : %d : %d\n", i+1, t.AssetOffset+ab,
			string(f.Data[correspondingStr.DataOffset:correspondingStr.DataOffset+correspondingStr.Length]), t.NameStringId, t.DefinitionBlockId, t.WithBlockId, t.ConstraintBlockId)
		if expand {
			f.printBlockContent(w, dumper, t.DefinitionBlockId)
			f.printBlockContent(w, dumper, t.WithBlockId)
			f.printBlockContent(w, dumper, t.ConstraintBlockId)
		}
	}
	if len(f.RecordTypes) != 0 {
		fmt.Fprintf(w, "\n")
	}
}

func (f *File) PrintSetTypesSection(w io.Writer, dumper T3xfDumper, expand bool) {
	fmt.Fprintln(w, "SetTypes Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Asset Offset  : Name (StringId) : DefinitionBlockId : ConstraintBlockId : WithBlockId")
	dumper.ToggleExpandBlocks(true)
	ab := f.assetBase()
	for i, t := range f.SetTypes {
		correspondingStr := f.Strings[t.NameStringId-1]
		fmt.Fprintf(w, "  %4d: @%08x %q (%d) : %d : %d : %d\n", i+1, t.AssetOffset+ab,
			string(f.Data[correspondingStr.DataOffset:correspondingStr.DataOffset+correspondingStr.Length]), t.NameStringId, t.DefinitionBlockId, t.ConstraintBlockId, t.WithBlockId)
		if expand {
			f.printBlockContent(w, dumper, t.DefinitionBlockId)
			f.printBlockContent(w, dumper, t.ConstraintBlockId)
			f.printBlockContent(w, dumper, t.WithBlockId)
		}
	}
	if len(f.SetTypes) != 0 {
		fmt.Fprintf(w, "\n")
	}
}

func (f *File) PrintRecordOfTypesSection(w io.Writer, dumper T3xfDumper, expand bool) {
	fmt.Fprintln(w, "RecordOfTypes Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Asset Offset  : Name (StringId) : DefinitionBlockId : ConstraintBlockId : WithBlockId")
	dumper.ToggleExpandBlocks(true)
	ab := f.assetBase()
	for i, t := range f.RecordOfTypes {
		correspondingStr := f.Strings[t.NameStringId-1]
		fmt.Fprintf(w, "  %4d: @%08x %q (%d) : %d : %d : %d\n", i+1, t.AssetOffset+ab,
			string(f.Data[correspondingStr.DataOffset:correspondingStr.DataOffset+correspondingStr.Length]), t.NameStringId, t.DefinitionBlockId, t.ConstraintBlockId, t.WithBlockId)
		if expand {
			f.printBlockContent(w, dumper, t.DefinitionBlockId)
			f.printBlockContent(w, dumper, t.ConstraintBlockId)
			f.printBlockContent(w, dumper, t.WithBlockId)
		}
	}
	if len(f.RecordOfTypes) != 0 {
		fmt.Fprintf(w, "\n")
	}
}

func (f *File) PrintSetOfTypesSection(w io.Writer, dumper T3xfDumper, expand bool) {
	fmt.Fprintln(w, "SetOfTypes Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Asset Offset  : Name (StringId) : ElementTypeFileOffset : ConstraintBlockId : WithBlockId")
	dumper.ToggleExpandBlocks(true)
	ab := f.assetBase()
	for i, t := range f.SetOfTypes {
		correspondingStr := f.Strings[t.NameStringId-1]
		fmt.Fprintf(w, "  %4d: @%08x %q (%d) : @%08x : %d : %d\n", i+1, t.AssetOffset+ab,
			string(f.Data[correspondingStr.DataOffset:correspondingStr.DataOffset+correspondingStr.Length]), t.NameStringId, t.ElementTypeFileOffset, t.ConstraintBlockId, t.WithBlockId)
		if expand {
			f.printBlockContent(w, dumper, t.ConstraintBlockId)
			f.printBlockContent(w, dumper, t.WithBlockId)
		}
	}
	if len(f.SetOfTypes) != 0 {
		fmt.Fprintf(w, "\n")
	}
}

func (f *File) PrintMapTypesSection(w io.Writer, dumper T3xfDumper, expand bool) {
	fmt.Fprintln(w, "MapTypes Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Asset Offset  : Name (StringId) : FromDefBlockId : ToDefBlockId : FromRefFileOffset : SetOfFromRefFileOffset : ToRefFileOffset : SetOfToRefFileOffset : WithBlockId")
	dumper.ToggleExpandBlocks(true)
	ab := f.assetBase()
	for i, t := range f.MapTypes {
		correspondingStr := f.Strings[t.NameStringId-1]
		fmt.Fprintf(w, "  %4d: @%08x %q (%d) : %d : %d : @%08x : @%08x : @%08x : @%08x : %d\n", i+1, t.AssetOffset+ab,
			string(f.Data[correspondingStr.DataOffset:correspondingStr.DataOffset+correspondingStr.Length]), t.NameStringId,
			t.FromDefinitionBlockId, t.ToDefinitionBlockId, t.FromRefFileOffset, t.SetOfFromRefFileOffset, t.ToRefFileOffset, t.SetOfToRefFileOffset, t.WithBlockId)
		if expand {
			f.printBlockContent(w, dumper, t.FromDefinitionBlockId)
			f.printBlockContent(w, dumper, t.ToDefinitionBlockId)
			f.printBlockContent(w, dumper, t.WithBlockId)
		}
	}
	if len(f.MapTypes) != 0 {
		fmt.Fprintf(w, "\n")
	}
}

func (f *File) PrintUnionTypesSection(w io.Writer, dumper T3xfDumper, expand bool) {
	fmt.Fprintln(w, "UnionTypes Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Asset Offset  : Name (StringId) : DefinitionBlockId : WithBlockId : ConstraintBlockId")
	dumper.ToggleExpandBlocks(true)
	ab := f.assetBase()
	for i, t := range f.UnionTypes {
		correspondingStr := f.Strings[t.NameStringId-1]
		fmt.Fprintf(w, "  %4d: @%08x %q (%d) : %d : %d : %d\n", i+1, t.AssetOffset+ab,
			string(f.Data[correspondingStr.DataOffset:correspondingStr.DataOffset+correspondingStr.Length]), t.NameStringId, t.DefinitionBlockId, t.WithBlockId, t.ConstraintBlockId)
		if expand {
			f.printBlockContent(w, dumper, t.DefinitionBlockId)
			f.printBlockContent(w, dumper, t.WithBlockId)
			f.printBlockContent(w, dumper, t.ConstraintBlockId)
		}
	}
	if len(f.UnionTypes) != 0 {
		fmt.Fprintf(w, "\n")
	}
}

func (f *File) PrintEnumeratedTypesSection(w io.Writer, dumper T3xfDumper, expand bool) {
	fmt.Fprintln(w, "EnumeratedTypes Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Asset Offset  : Name (StringId) : DefinitionBlockId : WithBlockId : ConstraintBlockId")
	dumper.ToggleExpandBlocks(true)
	ab := f.assetBase()
	for i, t := range f.EnumeratedTypes {
		correspondingStr := f.Strings[t.NameStringId-1]
		fmt.Fprintf(w, "  %4d: @%08x %q (%d) : %d : %d : %d\n", i+1, t.AssetOffset+ab,
			string(f.Data[correspondingStr.DataOffset:correspondingStr.DataOffset+correspondingStr.Length]), t.NameStringId, t.DefinitionBlockId, t.WithBlockId, t.ConstraintBlockId)
		if expand {
			f.printBlockContent(w, dumper, t.DefinitionBlockId)
			f.printBlockContent(w, dumper, t.WithBlockId)
			f.printBlockContent(w, dumper, t.ConstraintBlockId)
		}
	}
	if len(f.EnumeratedTypes) != 0 {
		fmt.Fprintf(w, "\n")
	}
}

func (f *File) PrintArrayTypesSection(w io.Writer, dumper T3xfDumper, expand bool) {
	fmt.Fprintln(w, "ArrayTypes Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Asset Offset  : Name (StringId) : ElementTypeFileOffset : DimensionBlockId : WithBlockId")
	dumper.ToggleExpandBlocks(true)
	ab := f.assetBase()
	for i, t := range f.ArrayTypes {
		correspondingStr := f.Strings[t.NameStringId-1]
		fmt.Fprintf(w, "  %4d: @%08x %q (%d) : @%08x : %d : %d\n", i+1, t.AssetOffset+ab,
			string(f.Data[correspondingStr.DataOffset:correspondingStr.DataOffset+correspondingStr.Length]), t.NameStringId, t.ElementTypeFileOffset, t.DimensionBlockId, t.WithBlockId)
		if expand {
			f.printBlockContent(w, dumper, t.DimensionBlockId)
			f.printBlockContent(w, dumper, t.WithBlockId)
		}
	}
	if len(f.ArrayTypes) != 0 {
		fmt.Fprintf(w, "\n")
	}
}

func (f *File) PrintClosureTypesSection(w io.Writer, dumper T3xfDumper, expand bool) {
	fmt.Fprintln(w, "ClosureTypes Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Asset Offset  : Name (StringId) : WithBlockId")
	dumper.ToggleExpandBlocks(true)
	ab := f.assetBase()
	for i, t := range f.ClosureTypes {
		correspondingStr := f.Strings[t.NameStringId-1]
		fmt.Fprintf(w, "  %4d: @%08x %q (%d) : %d\n", i+1, t.AssetOffset+ab,
			string(f.Data[correspondingStr.DataOffset:correspondingStr.DataOffset+correspondingStr.Length]), t.NameStringId, t.WithBlockId)
		if expand {
			f.printBlockContent(w, dumper, t.WithBlockId)
		}
	}
	if len(f.ClosureTypes) != 0 {
		fmt.Fprintf(w, "\n")
	}
}

func (f *File) PrintMessagePortTypesSection(w io.Writer, dumper T3xfDumper, expand bool) {
	fmt.Fprintln(w, "MessagePortTypes Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Asset Offset  : Name (StringId) : InDefBlockId : OutDefBlockId : MapParamsBlockId : UnmapParamsBlockId : WithBlockId")
	dumper.ToggleExpandBlocks(true)
	ab := f.assetBase()
	for i, t := range f.MessagePortTypes {
		correspondingStr := f.Strings[t.NameStringId-1]
		fmt.Fprintf(w, "  %4d: @%08x %q (%d) : %d : %d : %d : %d : %d\n", i+1, t.AssetOffset+ab,
			string(f.Data[correspondingStr.DataOffset:correspondingStr.DataOffset+correspondingStr.Length]), t.NameStringId,
			t.InDefinitionBlockId, t.OutDefinitionBlockId, t.MapParamsDefinitionBlockId, t.UnmapParamsDefinitionBlockId, t.WithBlockId)
		if expand {
			f.printBlockContent(w, dumper, t.InDefinitionBlockId)
			f.printBlockContent(w, dumper, t.OutDefinitionBlockId)
			f.printBlockContent(w, dumper, t.MapParamsDefinitionBlockId)
			f.printBlockContent(w, dumper, t.UnmapParamsDefinitionBlockId)
			f.printBlockContent(w, dumper, t.WithBlockId)
		}
	}
	if len(f.MessagePortTypes) != 0 {
		fmt.Fprintf(w, "\n")
	}
}

func (f *File) PrintComponentTypesSection(w io.Writer, dumper T3xfDumper, expand bool) {
	fmt.Fprintln(w, "ComponentTypes Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Asset Offset  : Name (StringId) : ExtendingBlockId : InitBlockId : WithBlockId")
	dumper.ToggleExpandBlocks(true)
	ab := f.assetBase()
	for i, t := range f.ComponentTypes {
		correspondingStr := f.Strings[t.NameStringId-1]
		fmt.Fprintf(w, "  %4d: @%08x %q (%d) : %d : %d : %d\n", i+1, t.AssetOffset+ab,
			string(f.Data[correspondingStr.DataOffset:correspondingStr.DataOffset+correspondingStr.Length]), t.NameStringId,
			t.ExtendingComponentsBlockId, t.InitialisationBlockId, t.WithBlockId)
		if expand {
			f.printBlockContent(w, dumper, t.ExtendingComponentsBlockId)
			f.printBlockContent(w, dumper, t.InitialisationBlockId)
			f.printBlockContent(w, dumper, t.WithBlockId)
		}
	}
	if len(f.ComponentTypes) != 0 {
		fmt.Fprintf(w, "\n")
	}
}

func (f *File) PrintInt32sSection(w io.Writer) {
	fmt.Fprintln(w, "Int32s Section:\n----------------")
	fmt.Fprintln(w, "  Id  : Asset Offset  : Value")
	ab := f.assetBase()
	for i, e := range f.Int32s {
		fmt.Fprintf(w, "  %4d: @%08x %d\n", i+1, e.AssetOffset+ab, e.Value)
	}
	if len(f.Int32s) != 0 {
		fmt.Fprintf(w, "\n")
	}
}

func (f *File) PrintCollectionsSection(w io.Writer) {
	fmt.Fprintln(w, "Collections Section:\n----------------")
	fmt.Fprintln(w, "  Id  : DataOffset : Length")
	db := f.dataBase()
	for i, e := range f.Collections {
		fmt.Fprintf(w, "  %4d: @%08x %d\n", i+1, e.DataOffset+db, e.Length)
	}
	if len(f.Collections) != 0 {
		fmt.Fprintf(w, "\n")
	}
}
