/*
Package session provides machine-wide unique sessions.

A session is a small unique integer, which is tied to a process id (PID) and may
typically be used to create a unique local IP address.

Note, this package was implemented to provide a smooth migration from Nokia
internal test tool (k3-run) to ntt and maybe replaced in the future.
*/
package session

var SharedDir = "/tmp/k3"

// Get returns the next free unqiue seesion id. If an error occures, Get returns
// -1 and the error.
func Get() (int, error) {
	storage, err := New(SharedDir)
	if err != nil {
		return -1, err
	}

	if err := storage.Lock(); err != nil {
		return -1, err
	}
	defer storage.Unlock()
	s, err := storage.Acquire()
	if err != nil {
		return -1, err
	}
	return s, nil
}

func Release(s int) {
	if storage, err := New(SharedDir); err == nil {
		if err := storage.Lock(); err == nil {
			defer storage.Unlock()
			storage.Release(s)
		}
	}
}

// Clean returns a slice of alive sessions
func Clean(sessions []session) []session {
	list := make([]session, 0)
	for _, s := range sessions {
		if s.Alive() {
			list = append(list, s)
		}
	}
	return list
}
