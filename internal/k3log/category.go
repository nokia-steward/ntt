// Package k3log defines the K3 runtime events
package k3log

import "strings"

// Category describes all event classes available in K3 runtime.
type Category string

// ID returns the 4 letter code identifying K3 runtime log event
func (c Category) ID() string {
	return string(c)[0:4]
}

// IsError returns true if c is describes an error event.
func (c Category) IsError() bool {
	return Descriptions[c.ID()].IsError
}

// Len returns the total number of fields an event has.
func (c Category) Len() int {
	return 2 + strings.Count(string(c), "|")
}

// String returns an one-line description of the event class
func (c Category) String() string {
	return string(c)
}
