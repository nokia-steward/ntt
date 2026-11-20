package tl

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
	"sync"

	"github.com/nokia/ntt/internal/env"
)

var (
	runtimeIDOnce sync.Once
	runtimeID     string
)

// RuntimeID is the identity of this runtime in a structured line log, where
// it stands for the test system: what runs the control part and starts the
// testcases. It is unique to the process, so that the logs of runtimes
// running side by side, or streaming to one place, tell whose a testcase
// is: "ntt-" and 16 characters of 80 random bits, or NTT_RUNTIME_ID when it
// is set, with the characters a log field cannot hold replaced by '_'.
func RuntimeID() string {
	runtimeIDOnce.Do(func() {
		if id := env.Getenv("NTT_RUNTIME_ID"); id != "" {
			runtimeID = fieldSafe(id)
			return
		}
		b := make([]byte, 10)
		if _, err := rand.Read(b); err != nil {
			runtimeID = "ntt"
			return
		}
		runtimeID = "ntt-" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
	})
	return runtimeID
}

// fieldSafe is id with what would end or split a field of a line, or the
// identity part of one, replaced by '_'.
func fieldSafe(id string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '|', '=', ' ', '\t', '\n', '\r':
			return '_'
		}
		return r
	}, strings.ReplaceAll(id, "->", "__"))
}
