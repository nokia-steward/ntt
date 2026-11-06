package vm

type Container struct {
	Elements []any
}

func (c *Container) Get(index int) any {
	if index < 0 || index >= len(c.Elements) {
		return nil
	}
	return c.Elements[index]
}

func (c *Container) Set(index int, value any) {
	if cap(c.Elements) < index+1 {
		newElements := make([]any, index+1, index*2)
		copy(newElements, c.Elements)
		c.Elements = newElements
	}
	c.Elements[index] = value
}

func (c *Container) Len() int {
	return len(c.Elements)
}

type Reference struct {
	Base  *[]any
	Index int
	Value any // Copy
}

func (r *Reference) Get() any {
	s := *r.Base
	if r.Index < 0 || r.Index >= len(s) {
		return nil
	}
	return s[r.Index]
}

func (r *Reference) Set(value any) {
	s := *r.Base
	if r.Index >= len(s) {
		newSlice := make([]any, r.Index+1)
		copy(newSlice, s)
		*(*[]any)(r.Base) = newSlice
	}
	s[r.Index] = value
}
