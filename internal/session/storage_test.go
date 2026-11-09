package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEmptyStorage(t *testing.T) {
	s, _ := New(".")
	list, err := s.Sessions()
	assert.Equal(t, 0, len(list))
	assert.Nil(t, err)

}

func TestDeepStorage(t *testing.T) {
	s, _ := New("/tmp/ntt/sessions/does-not-exist")
	list, err := s.Sessions()
	assert.Equal(t, 0, len(list))
	assert.Nil(t, err)
}

func TestInvalidStorage(t *testing.T) {
	_, err := New("storage_test.go")
	assert.NotNil(t, err)
}
