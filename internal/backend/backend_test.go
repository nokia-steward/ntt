package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParameterMap(t *testing.T) {
	f := ParameterMap(map[string]string{"m.X": "42"})

	v, ok := f("m.tc", "m.X")
	assert.True(t, ok)
	assert.Equal(t, "42", v)

	_, ok = f("m.tc", "m.Y")
	assert.False(t, ok)
}
