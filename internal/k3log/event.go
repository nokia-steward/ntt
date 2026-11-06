//go:generate go run gen.go

package k3log

import (
	"fmt"
	"io"
	"strings"
	"time"

	errors "golang.org/x/xerrors"
)

// Event represent a K3 runtime event.
type Event struct {
	Category
	fields []string
}

// tokens maps text events to their category
var tokens map[string]Category

// ParseEvent parses string s and returns an event. If the string could not be
// parsed sucessfully, ParseEvent will return an error
func ParseEvent(s string) (Event, error) {
	var e Event

	e.fields = strings.Split(strings.TrimSpace(s), "|")
	id := e.Field(1)
	if len(id) < 4 {
		return Event{}, errors.Errorf("unknown event %q", id)
	}
	id = id[:4]

	c, ok := tokens[id]
	if !ok {
		return Event{}, errors.Errorf("unknown event %q", id)
	}

	e.Category = c
	return e, nil
}

// Stamp will return the events timestamp
func (e Event) Stamp() (time.Time, error) {
	return time.Parse("20060102T150405.999999", e.Field(0))
}

// Component will return the component where this event was raised. If no
// component information is present Component will return an empty string.
func (e Event) Component() string {
	c := strings.SplitN(e.Field(2), "=", 2)
	if len(c) > 0 {
		return c[0]
	}
	return ""
}

// Source will return the source location where this event was raised. if no
// location is available Source wil;l return an empty string.
func (e Event) Source() string {
	c := strings.SplitN(e.Field(2), "=", 2)
	if len(c) == 2 {
		return c[1]
	}
	return ""
}

// Field returns the i-th field. Field does boundary checks.
func (e Event) Field(i int) string {
	if i < len(e.fields) {
		return e.fields[i]
	}
	return ""
}

// String returns the text form of the event, without its line ending.
func (e Event) String() string {
	return strings.Join(e.fields, "|")
}

// Locator resolves a T3XF code offset to its source location "file:line", or
// "" when unknown.
type Locator interface {
	Pos(pc int) string
}

// NoPos is the code offset of an event without a source location.
const NoPos = -1

// WriteEvent writes one k3log line for cat to w. component is the raising
// component name (e.g. "k3r" or "MTC"). pc is the T3XF offset that raised the
// event; loc resolves it and the location is appended as component=file:line.
func WriteEvent(w io.Writer, loc Locator, cat Category, component string, pc int, fields ...string) error {
	ts := time.Now().Format("20060102T150405.000000")
	comp := component
	if pc != NoPos && loc != nil {
		if pos := loc.Pos(pc); pos != "" {
			comp = component + "=" + pos
		}
	}
	parts := []string{ts, cat.ID(), comp}
	parts = append(parts, fields...)
	_, err := fmt.Fprintln(w, strings.Join(parts, "|"))
	return err
}
