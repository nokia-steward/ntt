package vm

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidFormat      = errors.New("invalid image format")
	ErrInvalidAddress     = errors.New("invalid address")
	ErrInvalidInstruction = errors.New("invalid instruction")
)

type Block struct {
	Begin int
	End   int
}

func (b Block) String() string {
	return fmt.Sprintf("[0x%04x:0x%04x]", b.Begin, b.End)
}

type Error struct {
	Err error
	PC  int
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s at offset 0x%04x", e.Err.Error(), e.PC)
}

func (e *Error) Unwrap() error {
	return e.Err
}
