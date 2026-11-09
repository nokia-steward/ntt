package verify

import "strings"

// K3logRuntimeFlags are the k3r flags the legacy ttcn3-runner always passes in
// k3log mode (before the per-test @k3:k3rflags) to record a vlist log and
// silence noisy/non-deterministic events. Mirrors build-aux/ttcn3-runner.in.
const K3logRuntimeFlags = "--record-fmt=vlist -A " +
	"-d plod -d perf -d vrsn -d sdbg -d deco -d vach " +
	"-d fxen -d fxlv -d pllg -d tycr -d tyrs -d tyde"

// Directives holds the per-test configuration declared by @k3: documentation
// tags, as understood by the legacy ttcn3-runner. Flags accumulate across all
// tags of a source file, mirroring the runner's whole-file directive scan.
type Directives struct {
	K3RFLAGS string   // additional k3r runtime flags (@k3:k3rflags)
	K3CFLAGS string   // additional k3c/mtc compiler flags (@k3:k3cflags)
	Env      []string // additional environment variables (@k3:env KEY=VALUE)
	Failure  bool     // test is a known breakage (@k3:failure)
}

// ParseDirectives extracts @k3: test directives from documentation tags (as
// produced by github.com/nokia/ntt/ttcn3/doc, which splits a tag on its first
// ':' into key and value, e.g. "@k3:k3rflags -D -a ulog" -> ["@k3", "k3rflags
// -D -a ulog"]).
func ParseDirectives(tags [][]string) Directives {
	var d Directives
	for _, tag := range tags {
		if len(tag) < 2 || tag[0] != "@k3" {
			continue
		}
		fields := shellFields(tag[1])
		if len(fields) == 0 {
			continue
		}
		args := fields[1:]
		switch fields[0] {
		case "k3rflags":
			d.K3RFLAGS = joinFlags(d.K3RFLAGS, args)
		case "k3cflags":
			d.K3CFLAGS = joinFlags(d.K3CFLAGS, args)
		case "env":
			d.Env = append(d.Env, args...)
		case "failure":
			d.Failure = true
		}
	}
	return d
}

// Merge returns the combination of d and o (flags concatenated, env appended).
func (d Directives) Merge(o Directives) Directives {
	return Directives{
		K3RFLAGS: joinFlags(d.K3RFLAGS, strings.Fields(o.K3RFLAGS)),
		K3CFLAGS: joinFlags(d.K3CFLAGS, strings.Fields(o.K3CFLAGS)),
		Env:      append(append([]string(nil), d.Env...), o.Env...),
		Failure:  d.Failure || o.Failure,
	}
}

func joinFlags(existing string, more []string) string {
	if len(more) == 0 {
		return existing
	}
	joined := strings.Join(more, " ")
	if existing == "" {
		return joined
	}
	return existing + " " + joined
}

// shellFields splits s into fields the way the legacy ttcn3-runner does with
// `eval set -- $CMD`: runs of whitespace separate fields, single and double
// quotes group their contents (and are removed), and a backslash escapes the
// next character (e.g. "setv\|ulog" -> "setv|ulog", or a whole flag list
// wrapped in quotes -> its individual flags). It is a small POSIX-like
// tokenizer, sufficient for @k3: directive values.
func shellFields(s string) []string {
	var (
		fields  []string
		b       strings.Builder
		inField bool
		quote   byte // 0, '\'' or '"'
	)
	flush := func() {
		if inField {
			fields = append(fields, b.String())
			b.Reset()
			inField = false
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			} else {
				b.WriteByte(c)
			}
		case quote == '"':
			if c == '"' {
				quote = 0
			} else if c == '\\' && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\\') {
				i++
				b.WriteByte(s[i])
			} else {
				b.WriteByte(c)
			}
		case c == '\'' || c == '"':
			quote = c
			inField = true
		case c == '\\':
			if i+1 < len(s) {
				i++
				b.WriteByte(s[i])
			}
			inField = true
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			flush()
		default:
			b.WriteByte(c)
			inField = true
		}
	}
	flush()
	return fields
}
