package vm

import (
	"context"
	"encoding/binary"
	"fmt"
	"github.com/nokia/ntt/internal/runtime"
	"github.com/nokia/ntt/internal/t3xf"
	"github.com/nokia/ntt/internal/t3xf/opcode"
	"os"
	"runtime/trace"
	"sort"
)

type Image struct {
	Path string
	Code []byte

	PortAdapter runtime.PortAdapter

	Sources struct {
		Addr []int
		Name []string
	}
	Locations struct {
		Addr []int
		Line []int
	}
	Modules struct {
		Addr []int
		Name []string
	}

	Tags             []any
	Tests            []*runtime.Function
	Controls         []*runtime.Function
	Builtins         []*runtime.Function
	ModuleParameters []*runtime.Var
	Names            map[string]any

	// locals is one descriptor per VAR instruction, keyed by its PC.
	// Tags stays immutable after load; values live on each machine's stack.
	locals map[int]*runtime.Var
}

func LoadFile(path string) (*Image, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	img, err := Load(b)
	if err != nil {
		return nil, err
	}
	img.Path = path
	return img, nil
}

func Load(b []byte) (img *Image, err error) {
	defer func() {
		if r := recover(); r != nil {
			img, err = nil, fmt.Errorf("%w: %v", ErrInvalidFormat, r)
		}
	}()
	if len(b) < 16 {
		return nil, ErrInvalidFormat
	}

	if _, op, _ := t3xf.Decode(b[0:]); op != opcode.NOP {
		return nil, ErrInvalidFormat
	}

	n, op, version := t3xf.Decode(b[4:])
	if op != opcode.NATLONG {
		return nil, ErrInvalidFormat
	}
	if _, op, _ = t3xf.Decode(b[4+n:]); op != opcode.VERSION {
		return nil, ErrInvalidFormat
	}

	// We need to patch if the T3XF file is still version 2
	if version.(int) < 3 {
		binary.LittleEndian.PutUint32(b[8:], 3)
		t3xf.PatchBlocks(b)
	}
	return load(b)
}

func load(b []byte) (*Image, error) {
	region := trace.StartRegion(context.Background(), "t3xf.Image.Load")
	defer region.End()

	l := loader{Image: NewImage(b)}

	// `load` skips the first 4 bytes, which is usually the ISCAN
	// instruction. However the T3XF file is not bracketed in a
	// ISCAN-BLOCK, so we compensate by passing -4.
	if err := l.load(Block{Begin: -4, End: len(b)}); err != nil {
		return nil, err
	}
	return l.Image, nil
}

// NewImage indexes every LINE in b, including those inside bodies the loader
// does not visit, so any code offset resolves to a source line.
func NewImage(b []byte) *Image {
	img := &Image{
		Code:   b,
		Tags:   make([]any, len(b)/4),
		Names:  make(map[string]any),
		locals: make(map[int]*runtime.Var),
	}
	for pc := 0; pc+4 <= len(b); {
		n, op, arg := t3xf.Decode(b[pc:])
		if op == opcode.LINE {
			img.Locations.Addr = append(img.Locations.Addr, pc)
			img.Locations.Line = append(img.Locations.Line, arg.(int))
		}
		pc += n
	}
	return img
}

func (img *Image) Module(addr int) string {
	n := len(img.Modules.Addr)
	idx := sort.Search(n, func(i int) bool {
		return addr <= img.Modules.Addr[i]
	})
	if idx < n {
		return img.Modules.Name[idx]
	}
	return ""
}

func (img *Image) Source(addr int) string {
	n := len(img.Sources.Addr)
	idx := sort.Search(n, func(i int) bool {
		return addr <= img.Sources.Addr[i]
	})
	if idx < n {
		return img.Sources.Name[idx]
	}
	return img.Path
}

func (img *Image) Line(addr int) int {
	n := len(img.Locations.Addr)
	idx := sort.Search(n, func(i int) bool {
		return addr < img.Locations.Addr[i]
	})
	if idx > 0 {
		return img.Locations.Line[idx-1]
	}
	return 0
}

// Pos returns "file:line" for addr. Without a source line it returns the T3XF
// offset as "t3xf:0x0058".
func (img *Image) Pos(addr int) string {
	if line := img.Line(addr); line > 0 {
		if src := img.Source(addr); src != "" {
			return fmt.Sprintf("%s:%d", src, line)
		}
		return fmt.Sprintf("%d", line)
	}
	return fmt.Sprintf("t3xf:0x%04x", addr)
}

type loader struct {
	*Image
	stack []any
}

func (l *loader) push(v any) {
	l.stack = append(l.stack, v)
}

func (l *loader) pop() any {
	v := l.stack[len(l.stack)-1]
	l.stack = l.stack[:len(l.stack)-1]
	return v
}

func (l *loader) get(op opcode.Opcode, addr int) any {
	if addr < 0 || addr>>2 >= len(l.Tags) {
		panic(&Error{PC: addr, Err: fmt.Errorf("%w (0x%04x)", ErrInvalidAddress, addr)})
	}
	if obj := l.Tags[addr>>2]; obj != nil {
		return obj
	}

	c, ok := constructors[op]
	if !ok {
		panic(&Error{PC: addr, Err: fmt.Errorf("no constructor for opcode %s", op.String())})
	}
	obj := c()
	l.Tags[addr>>2] = obj
	return obj
}

func (img *loader) load(blk Block) (err error) {
	idx := 0
	b := img.Code
	pc := blk.Begin + 4
	end := blk.End

	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(*Error); ok {
				err = e
				return
			}
			if e, ok := r.(error); ok {
				err = &Error{PC: pc, Err: e}
				return
			}
			err = &Error{PC: pc, Err: fmt.Errorf("%v", r)}
		}
	}()

	push := func(v any) {
		img.push(v)
	}

	pop := func() any {
		return img.pop()
	}

	popType := func() runtime.Type {
		typ, ok := pop().(runtime.Type)
		if !ok {
			panic(&Error{PC: pc, Err: fmt.Errorf("type expected")})
		}
		return typ
	}

	qualifiedName := func(s string) string {
		if mods := len(img.Modules.Name); mods > 0 {
			return img.Modules.Name[mods-1] + "." + s
		}
		return s
	}

	for pc < end {
		n, op, arg := t3xf.Decode(b[pc:])

		//fmt.Printf("0x%04x: %s", pc, op.String())
		//if arg != nil {
		//	fmt.Printf(" %v", arg)
		//}
		//fmt.Println()

		switch op {
		case opcode.NOP, opcode.LINE:
			// nothing to do here.

		case opcode.DROP:
			pop()

		case opcode.VERSION:
			ver := pop().(int)
			if ver != 3 {
				return ErrInvalidFormat
			}

		case opcode.MARK:
			first := len(img.stack)
			if err := img.load(Block{pc, end}); err != nil {
				return err
			}
			pc, n = pop().(int)+4, 0
			last := len(img.stack)
			N := last - first
			objs := make([]any, N)
			copy(objs, img.stack[first:last])
			img.stack = img.stack[:first]
			push(objs)

		case opcode.VLIST, opcode.COLLECT:
			push(pc)
			return nil

		case opcode.SOURCE:
			img.Sources.Addr = append(img.Sources.Addr, pc)
			img.Sources.Name = append(img.Sources.Name, pop().(string))
			if err := img.load(pop().(Block)); err != nil {
				return err
			}

		case opcode.MODULE:
			img.Modules.Addr = append(img.Modules.Addr, pc)
			img.Modules.Name = append(img.Modules.Name, pop().(string))
			if err := img.load(pop().(Block)); err != nil {
				return err
			}

		case opcode.ISCAN:
			blk := Block{Begin: pc, End: int(arg.(t3xf.Reference))}
			push(blk)
			pc, n = blk.End+4, 0

		case opcode.TRUE:
			push(true)
		case opcode.FALSE:
			push(false)
		case opcode.NATLONG:
			push(arg.(int))
		case opcode.UTF8:
			push(arg.(string))
		case opcode.IEEE754DP:
			push(arg.(float64))
		case opcode.NAME:
			push(arg.(string))

		case opcode.REF, opcode.FROZEN_REF:
			addr := int(arg.(t3xf.Reference))
			if addr < 0 || addr >= len(img.Code) {
				return &Error{PC: pc, Err: fmt.Errorf("%w (0x%04x)", ErrInvalidAddress, addr)}
			}
			if v, ok := img.locals[addr]; ok {
				push(v)
				break
			}

			_, op, _ := t3xf.Decode(b[addr:])
			push(img.get(op, addr))

		case opcode.ASSIGN:
			v, ok := pop().(*runtime.Var)
			if !ok {
				return &Error{PC: pc, Err: fmt.Errorf("variable expected")}
			}
			v.Value = pop()

		case opcode.TYPE, opcode.TYPEW:
			typ := img.get(op, pc).(*runtime.NamedType)
			typ.Name = qualifiedName(pop().(string))
			typ.Type = popType()
			if op == opcode.TYPEW {
				typ.Attributes = img.loadAttributes(pop().(Block))
			}

		case opcode.CONST, opcode.CONSTW, opcode.MPAR, opcode.MPARD, opcode.VAR:
			v := img.get(op, pc).(*runtime.Var)
			v.Name = qualifiedName(pop().(string))
			v.Type = popType()
			v.Index = arg.(int)
			if op == opcode.CONST || op == opcode.CONSTW || op == opcode.MPARD {
				v.Value = pop().(Block)
			}
			if op == opcode.CONSTW {
				v.Attributes = img.loadAttributes(pop().(Block))
			}
			if op == opcode.MPAR || op == opcode.MPARD {
				img.ModuleParameters = append(img.ModuleParameters, v)
			}
			img.Names[v.Name] = v

		case opcode.TEMPLATE:
			tmpl := img.get(op, pc).(*runtime.Function)
			if n, ok := arg.(int); ok {
				tmpl.NLocals = n
			}
			tmpl.Flags |= runtime.Template
			tmpl.Name = qualifiedName(pop().(string))
			tmpl.Body = pop().(Block)
			tmpl.Parameters = loadObjs[*runtime.Var](img, pop().(Block))
			tmpl.ReturnType = popType()
			img.Names[tmpl.Name] = tmpl

		case opcode.CONTROL:
			fn := img.get(op, pc).(*runtime.Function)
			if n, ok := arg.(int); ok {
				fn.NLocals = n
			}
			fn.Name = qualifiedName("control")
			fn.Body = pop().(Block)
			img.Controls = append(img.Controls, fn)
			img.Names[fn.Name] = fn

		case opcode.FUNCTION, opcode.FUNCTIONB, opcode.FUNCTIONV, opcode.FUNCTIONVB,
			opcode.ALTSTEP, opcode.ALTSTEPB, opcode.ALTSTEPW, opcode.ALTSTEPBW,
			opcode.TESTCASE, opcode.TESTCASES:
			fn := img.get(op, pc).(*runtime.Function)
			if n, ok := arg.(int); ok {
				fn.NLocals = n
			}
			fn.Name = qualifiedName(pop().(string))
			fn.Body = pop().(Block)
			fn.Parameters = loadObjs[*runtime.Var](img, pop().(Block))
			// Optional slots, top of the stack first: the reverse of the YAML
			// pre list. functionvb is [type, comp, ...]; altstepbw is [comp, ablk, ...].
			if op == opcode.ALTSTEPW || op == opcode.ALTSTEPBW {
				fn.Attributes = img.loadAttributes(pop().(Block))
			}
			if op == opcode.FUNCTIONB || op == opcode.FUNCTIONVB ||
				op == opcode.ALTSTEPB || op == opcode.ALTSTEPBW ||
				op == opcode.TESTCASE || op == opcode.TESTCASES {
				fn.RunsOn = popType()
			}
			if op == opcode.FUNCTIONV || op == opcode.FUNCTIONVB {
				fn.ReturnType = popType()
			}
			if op == opcode.TESTCASES {
				fn.System = popType()
			}
			switch op {
			case opcode.TESTCASE, opcode.TESTCASES:
				fn.Flags |= runtime.Testcase
				img.Tests = append(img.Tests, fn)
				img.Names[fn.Name] = fn
			case opcode.FUNCTION, opcode.FUNCTIONB, opcode.FUNCTIONV, opcode.FUNCTIONVB:
				img.Names[fn.Name] = fn
			default:
				fn.Flags |= runtime.Altstep
			}

		case opcode.FUNCTIONX, opcode.FUNCTIONXV, opcode.FUNCTIONXW, opcode.FUNCTIONXVW:
			fn := img.get(op, pc).(*runtime.Function)
			fn.Name = qualifiedName(pop().(string))
			fn.Parameters = loadObjs[*runtime.Var](img, pop().(Block))
			if op == opcode.FUNCTIONXW || op == opcode.FUNCTIONXVW {
				fn.Attributes = img.loadAttributes(pop().(Block))
			}
			if op == opcode.FUNCTIONXV || op == opcode.FUNCTIONXVW {
				fn.ReturnType = popType()
			}
			img.Builtins = append(img.Builtins, fn)

		case opcode.IN, opcode.OUT, opcode.INOUT:
			v := img.get(op, pc).(*runtime.Var)
			v.Name = qualifiedName(pop().(string))
			v.Type = popType()
			v.Index = idx
			v.Flags |= runtime.Parameter
			if op == opcode.OUT {
				v.Flags |= runtime.Out
			} else if op == opcode.INOUT {
				v.Flags |= runtime.Inout
			}
			push(v)
			idx++

		case opcode.FIELD, opcode.FIELDO, opcode.IFIELD:
			v := img.get(op, pc).(*runtime.Var)
			v.Name = pop().(string)
			v.Type = popType()
			if op == opcode.IFIELD {
				v.Index = arg.(int)
			} else {
				v.Index = idx
				idx++
			}
			if op == opcode.FIELDO {
				v.Flags |= runtime.Optional
			}
			push(v)

		case opcode.BITSTRING:
			push(runtime.BitstringType)
		case opcode.BOOLEAN:
			push(runtime.BooleanType)
		case opcode.CHARSTRING:
			push(runtime.CharstringType)
		case opcode.CHARSTRINGU:
			push(runtime.UniversalCharstringType)
		case opcode.CLOSURETYPE:
			push(runtime.Closure)
		case opcode.DEFAULT:
			push(runtime.DefaultType)
		case opcode.FLOAT:
			push(runtime.FloatType)
		case opcode.HEXSTRING:
			push(runtime.HexstringType)
		case opcode.INTEGER:
			push(runtime.IntegerType)
		case opcode.OCTETSTRING:
			push(runtime.OctetstringType)
		case opcode.TIMER:
			push(runtime.TimerType)
		case opcode.VERDICTTYPE:
			push(runtime.VerdictType)

		case opcode.PERMITO, opcode.PERMITP, opcode.PERMITT:
			var flags runtime.Flags
			if op == opcode.PERMITO {
				flags = runtime.Omit
			} else if op == opcode.PERMITP {
				flags = runtime.Present
			}
			push(runtime.TemplateType{
				BaseType: popType(),
				Flags:    flags,
			})

		case opcode.SUBTYPE:
			push(runtime.Subtype{
				Constraint: pop().(Block),
				BaseType:   popType(),
			})

		case opcode.UNION:
			push(runtime.UnionType{Fields: loadObjs[*runtime.Var](img, pop().(Block))})

		case opcode.RECORD:
			push(runtime.RecordType{Fields: loadObjs[*runtime.Var](img, pop().(Block))})

		case opcode.SET:
			push(runtime.SetType{Fields: loadObjs[*runtime.Var](img, pop().(Block))})

		case opcode.RECORDOF, opcode.SETOF:
			if _, ok := img.stack[len(img.stack)-1].(Block); ok {
				types := loadObjs[runtime.Type](img, pop().(Block))
				if len(types) != 1 {
					return &Error{PC: pc, Err: fmt.Errorf("expected one type, got %d", len(types))}
				}
				push(types[0])
			}
			elem := popType()
			if op == opcode.RECORDOF {
				push(runtime.RecordOfType{ElementType: elem})
			} else {
				push(runtime.SetOfType{ElementType: elem})
			}

		case opcode.ARRAY:
			push(runtime.ArrayType{
				Length:      pop().(Block),
				ElementType: popType(),
			})

		case opcode.ENUMERATED:
			push(img.loadEnum(pop().(Block)))

		case opcode.MAPT:
			popMapType := func() runtime.Type {
				switch v := pop().(type) {
				case runtime.Type:
					return v
				case Block:
					types := loadObjs[runtime.Type](img, v)
					if len(types) != 1 {
						panic(&Error{PC: pc, Err: fmt.Errorf("expected one type, got %d", len(types))})
					}
					return types[0]
				default:
					panic(&Error{PC: pc, Err: fmt.Errorf("type expected, got %T", v)})
				}
			}
			valueType := popMapType()
			keyType := popMapType()
			push(runtime.MapType{
				ValueType: valueType,
				KeyType:   keyType,
			})

		case opcode.COMPONENT:
			body := pop().(Block)
			typ := runtime.ComponentType{
				Body: body,
			}
			push(typ)

		case opcode.COMPONENTX:
			body := pop().(Block)
			objs := pop().([]any)
			extends := make([]runtime.Type, len(objs))
			for i, obj := range objs {
				typ, ok := obj.(runtime.Type)
				if !ok {
					return &Error{PC: pc, Err: fmt.Errorf("type expected")}
				}
				extends[i] = typ
			}
			typ := runtime.ComponentType{
				Body:    body,
				Extends: extends,
			}
			push(typ)

		case opcode.PORTM, opcode.PORTMA:
			pt := runtime.PortType{
				UnmapParams: loadObjs[*runtime.Var](img, pop().(Block)),
				MapParams:   loadObjs[*runtime.Var](img, pop().(Block)),
			}
			if op == opcode.PORTMA {
				pt.Address = popType()
			}
			pt.OutTypes = loadObjs[runtime.Type](img, pop().(Block))
			pt.InTypes = loadObjs[runtime.Type](img, pop().(Block))
			push(pt)

		default:
			return &Error{PC: pc, Err: fmt.Errorf("%w (%s)", ErrInvalidInstruction, op.String())}
		}
		pc += n
	}

	return nil
}

var (
	constructors = map[opcode.Opcode]func() any{
		opcode.TYPE:        func() any { return new(runtime.NamedType) },
		opcode.TYPEW:       func() any { return new(runtime.NamedType) },
		opcode.CONST:       func() any { return new(runtime.Var) },
		opcode.CONSTW:      func() any { return new(runtime.Var) },
		opcode.MPAR:        func() any { return new(runtime.Var) },
		opcode.MPARD:       func() any { return new(runtime.Var) },
		opcode.VAR:         func() any { return new(runtime.Var) },
		opcode.IN:          func() any { return new(runtime.Var) },
		opcode.OUT:         func() any { return new(runtime.Var) },
		opcode.INOUT:       func() any { return new(runtime.Var) },
		opcode.FIELD:       func() any { return new(runtime.Var) },
		opcode.FIELDO:      func() any { return new(runtime.Var) },
		opcode.IFIELD:      func() any { return new(runtime.Var) },
		opcode.TEMPLATE:    func() any { return new(runtime.Function) },
		opcode.CONTROL:     func() any { return new(runtime.Function) },
		opcode.FUNCTION:    func() any { return new(runtime.Function) },
		opcode.FUNCTIONB:   func() any { return new(runtime.Function) },
		opcode.FUNCTIONV:   func() any { return new(runtime.Function) },
		opcode.FUNCTIONVB:  func() any { return new(runtime.Function) },
		opcode.FUNCTIONX:   func() any { return new(runtime.Function) },
		opcode.FUNCTIONXV:  func() any { return new(runtime.Function) },
		opcode.FUNCTIONXW:  func() any { return new(runtime.Function) },
		opcode.FUNCTIONXVW: func() any { return new(runtime.Function) },
		opcode.ALTSTEP:     func() any { return new(runtime.Function) },
		opcode.ALTSTEPB:    func() any { return new(runtime.Function) },
		opcode.ALTSTEPBW:   func() any { return new(runtime.Function) },
		opcode.ALTSTEPW:    func() any { return new(runtime.Function) },
		opcode.TESTCASE:    func() any { return new(runtime.Function) },
		opcode.TESTCASES:   func() any { return new(runtime.Function) },
	}
)

func loadObjs[T any](l *loader, blk Block) []T {
	first := len(l.stack)
	if err := l.load(blk); err != nil {
		panic(err)
	}
	last := len(l.stack)
	n := last - first
	if n < 0 {
		panic(&Error{
			PC:  blk.Begin,
			Err: fmt.Errorf("stack underrun during execution of block (0x%x-0x%x)", blk.Begin, blk.End),
		})
	}
	objs := make([]T, n)
	for i := 0; i < n; i++ {
		v, ok := l.stack[first+i].(T)
		if !ok {
			panic(&Error{
				PC:  blk.Begin,
				Err: fmt.Errorf("unexpected %T in block (0x%x-0x%x)", l.stack[first+i], blk.Begin, blk.End),
			})
		}
		objs[i] = v
	}
	l.stack = l.stack[:first]
	return objs
}

func (m *loader) loadAttributes(blk Block) runtime.Attributes {
	attrs := runtime.Attributes{}
	pc := blk.Begin + 4
	for pc < blk.End {
		n, op, arg := t3xf.Decode(m.Code[pc:])
		switch op {
		case opcode.UTF8:
			m.push(arg.(string))

		case opcode.ENCODE, opcode.ENCODEO:
			attrs.Encode = runtime.Attribute{
				Value:    m.pop().(string),
				Override: op == opcode.ENCODEO,
			}

		case opcode.VARIANT, opcode.VARIANTO:
			attrs.Variant = runtime.Attribute{
				Value:    m.pop().(string),
				Override: op == opcode.VARIANTO,
			}

		case opcode.EXTENSION, opcode.EXTENSIONO:
			attrs.Extensions = append(attrs.Extensions, runtime.Attribute{
				Value:    m.pop().(string),
				Override: op == opcode.EXTENSIONO,
			})

		default:
			panic(&Error{
				PC:  pc,
				Err: fmt.Errorf("%s: %w", op.String(), ErrInvalidInstruction),
			})
		}
		pc += n
	}
	return attrs
}

func (img *Image) loadEnum(blk Block) runtime.EnumeratedType {
	localsMu.Lock()
	defer localsMu.Unlock()
	if img.locals == nil {
		img.locals = make(map[int]*runtime.Var)
	}
	var terms []*runtime.Var
	var val int
	var name string
	for pc := blk.Begin + 4; pc < blk.End; {
		n, op, arg := t3xf.Decode(img.Code[pc:])
		switch op {
		case opcode.NATLONG:
			val = int(int32(arg.(int)))
		case opcode.NEG:
			val = -val
		case opcode.NAME:
			name = arg.(string)
		case opcode.TERM:
			v := &runtime.Var{
				Name:  "(anon)",
				Index: -1,
				Value: runtime.EnumValue{Name: name, Val: val},
			}
			img.locals[pc] = v
			terms = append(terms, v)
		}
		pc += n
	}
	return runtime.EnumeratedType{Body: terms}
}
