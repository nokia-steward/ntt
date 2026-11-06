// Package text implements the TTCN-3 TEXT codec per Annex E.2. TEXT is
// the line-oriented text codec used for IETF-style protocols (SIP,
// HTTP, RTSP). This baseline covers integer / charstring scalars and
// records as `key=value` line groups; full TEXT (with BEGIN / END
// tokens, separators, and the rich Annex E.2 grammar) lands in a later
// iteration once we have a concrete telco use case driving requirements.
package text

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/nokia/ntt/runtime/codec"
	"github.com/nokia/ntt/ttcn3/attr"
)

// Codec is the TEXT codec implementation.
type Codec struct{}

func init() { codec.Register(Codec{}) }

// Name implements codec.Codec.
func (Codec) Name() string { return "TEXT" }

// Plan stores the TEXT directives that affect record encoding. The
// current MVP recognises separator overrides; future versions will also
// surface BEGIN / END tokens and per-field encoding hints.
type Plan struct {
	FieldSep   string
	RecordSep  string
	KeyValSep  string
}

// EncodingName implements codec.Plan.
func (Plan) EncodingName() string { return "TEXT" }

// Compile builds a Plan from a TEXT attribute set.
func (Codec) Compile(set *attr.AttributeSet) (codec.Plan, error) {
	plan := &Plan{FieldSep: "\r\n", RecordSep: "\r\n\r\n", KeyValSep: ": "}
	if set == nil {
		return plan, nil
	}
	for _, v := range set.Variants() {
		raw := strings.TrimSpace(v.Value)
		switch {
		case strings.HasPrefix(strings.ToUpper(raw), "FIELDSEP("):
			plan.FieldSep = trimParen(raw)
		case strings.HasPrefix(strings.ToUpper(raw), "RECORDSEP("):
			plan.RecordSep = trimParen(raw)
		case strings.HasPrefix(strings.ToUpper(raw), "KEYVALSEP("):
			plan.KeyValSep = trimParen(raw)
		}
	}
	return plan, nil
}

// Encode serialises v as text.
func (Codec) Encode(p codec.Plan, v codec.Value) ([]byte, error) {
	plan, ok := p.(*Plan)
	if !ok {
		return nil, fmt.Errorf("TEXT: plan %T is not a TEXT plan", p)
	}
	var buf bytes.Buffer
	if err := writeText(&buf, plan, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Decode parses bytes as text into a Value. Records are reconstructed
// from the key/value separator the plan defines.
func (Codec) Decode(p codec.Plan, data []byte) (codec.Value, error) {
	plan, ok := p.(*Plan)
	if !ok {
		return nil, fmt.Errorf("TEXT: plan %T is not a TEXT plan", p)
	}
	text := string(data)
	if !strings.Contains(text, plan.KeyValSep) {
		return codec.StringValue(text), nil
	}
	rec := codec.RecordValue{Fields: map[string]codec.Value{}}
	for _, line := range strings.Split(text, plan.FieldSep) {
		if line == "" {
			continue
		}
		idx := strings.Index(line, plan.KeyValSep)
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+len(plan.KeyValSep):])
		rec.Names = append(rec.Names, key)
		// Try integer first so round-trips of numeric fields don't
		// silently widen to charstring.
		if n, err := strconv.ParseInt(val, 10, 64); err == nil {
			rec.Fields[key] = codec.IntValue(n)
			continue
		}
		rec.Fields[key] = codec.StringValue(val)
	}
	sort.Strings(rec.Names)
	return rec, nil
}

func writeText(w *bytes.Buffer, plan *Plan, v codec.Value) error {
	switch val := v.(type) {
	case codec.IntValue:
		fmt.Fprintf(w, "%d", int64(val))
	case codec.BoolValue:
		if val {
			w.WriteString("true")
		} else {
			w.WriteString("false")
		}
	case codec.StringValue:
		w.WriteString(string(val))
	case codec.RecordValue:
		for i, name := range val.Names {
			if i > 0 {
				w.WriteString(plan.FieldSep)
			}
			w.WriteString(name)
			w.WriteString(plan.KeyValSep)
			if err := writeText(w, plan, val.Fields[name]); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("TEXT: cannot encode kind %v", v.Kind())
	}
	return nil
}

func trimParen(raw string) string {
	open := strings.Index(raw, "(")
	close := strings.LastIndex(raw, ")")
	if open < 0 || close < 0 || close <= open {
		return raw
	}
	return raw[open+1 : close]
}
