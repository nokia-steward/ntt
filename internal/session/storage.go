package session

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/nokia/ntt/internal/session/flock"
)

// storage represents a directory which is shared between different
// processes.
type storage struct {
	dir          string
	sessionsFile string

	lock *flock.Flock
}

// New returns a new storage. The backing path is evaulated lazily and may
// contain active sessions, if path does not exist it will be created.
func New(path string) (*storage, error) {
	if err := os.MkdirAll(path, os.ModePerm); err != nil {
		return nil, err
	}

	os.Chmod(path, 0777)

	// TODO(5nord) session directory should be created.
	return &storage{
		dir:          path,
		sessionsFile: filepath.Join(path, "sessions"),
		lock:         flock.New(filepath.Join(path, "sessions.lock")),
	}, nil
}

// Acquire returns the smallest unused integer from storage.
func (s *storage) Acquire() (int, error) {
	sessions, err := s.Sessions()
	if err != nil {
		return 0, err
	}

	next := 1
	for _, s := range sessions {
		if next == s.num {
			next++
		}
	}
	sessions = append(sessions, session{num: next, pid: os.Getpid()})
	s.SetSessions(sessions)
	return next, nil
}

// Release returns a session back to the storage.
func (s *storage) Release(num int) {
	sessions, err := s.Sessions()
	if err != nil {
		return
	}

	list := make([]session, 0)
	for _, s := range sessions {
		if s.num != num {
			list = append(list, s)
		}
	}
	s.SetSessions(list)
}

// Sessions returns a sorted slice of active sessions from storage. If storage
// could not be read a error is returned.
func (s *storage) Sessions() ([]session, error) {
	sessions := make([]session, 0)

	if _, err := os.Stat(s.sessionsFile); os.IsNotExist(err) {
		return sessions, nil
	}

	file, err := os.Open(s.sessionsFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	line := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line++
		num := 0
		pid := 0
		if _, err := fmt.Sscanf(scanner.Text(), "%d %d", &num, &pid); err != nil {
			return nil, err
		}
		sessions = append(sessions, session{num: num, pid: pid})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	Sort(sessions)
	return Clean(sessions), nil
}

// SetSessions overwrites sessions in storage.
func (s *storage) SetSessions(sessions []session) error {
	file, err := os.Create(s.sessionsFile)
	if err != nil {
		return err
	}
	defer file.Close()

	// Some users might have a restrictive umask setting. It is okay to make the
	// sessions file world wide writable.
	os.Chmod(s.sessionsFile, 0666)

	for _, s := range sessions {
		if _, err := file.WriteString(s.String() + "\n"); err != nil {
			return err
		}
	}
	file.Sync()
	return nil
}

// Lock blocks until flock could be acquired for storage.
func (s *storage) Lock() error {
	return s.lock.Lock()
}

// Unlock releases the storage flock.
func (s *storage) Unlock() {
	s.lock.Unlock()
}

type session struct {
	num int
	pid int
}

func (s *session) String() string {
	return strconv.Itoa(s.num) + "\t" + strconv.Itoa(s.pid)
}

// Alive returns true if asssociated process is alive. Note, this test is unix
// specific.
func (s *session) Alive() bool {
	proc := filepath.Join("/proc", strconv.Itoa(s.pid))
	if _, err := os.Stat(proc); os.IsNotExist(err) {
		return false
	}
	return true
}
