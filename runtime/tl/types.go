package tl

import (
	"strconv"
)

// ComponentID identifies a test component (TriComponentIdType, Id; ES 201
// 873-6 clauses 11.3.2.2 and 11.3.2.5). Name is the component's name, ID its
// internal representation and Type its component type. Null stands for "no
// component".
type ComponentID struct {
	Name, ID, Type string
	Null           bool
}

// idContent is the content of Types:Id.
func idContent(name, id, typ string) Content {
	kids := []*Node{text(NSTypes, "name", name)}
	if id != "" {
		kids = append(kids, text(NSTypes, "id", id))
	}
	if typ != "" {
		kids = append(kids, text(NSTypes, "type", typ))
	}
	return Content{Kids: kids}
}

// Content returns c as a Types:TriComponentIdType.
func (c ComponentID) Content() Content {
	if c.Null {
		return Content{Kids: []*Node{el(NSTypes, "null")}}
	}
	return Content{Kids: []*Node{idContent(c.Name, c.ID, c.Type).in(NSTypes, "id")}}
}

// PortID identifies a port instance of a component (TriPortIdType, Port).
// Index is the port-array index, or negative for a port that is not an
// array element.
type PortID struct {
	Comp  ComponentID
	Name  string
	Index int
}

// Content returns p as a Types:TriPortIdType.
func (p PortID) Content() Content {
	port := el(NSTypes, "port", idContent(p.Name, "", "").in(NSTypes, "id"))
	if p.Index >= 0 {
		port.Kids = append(port.Kids, text(NSTypes, "index", strconv.Itoa(p.Index)))
	}
	return Content{Kids: []*Node{p.Comp.Content().in(NSTypes, "comp"), port}}
}

// PortIDList is a Types:TriPortIdListType.
func PortIDList(ports ...PortID) Content {
	var kids []*Node
	for _, p := range ports {
		kids = append(kids, p.Content().in(NSTypes, "port"))
	}
	return Content{Kids: kids}
}

// TimerID is a Types:TriTimerIdType.
func TimerID(name, id, typ string) Content {
	return Content{Kids: []*Node{idContent(name, id, typ).in(NSTypes, "id")}}
}

// QualifiedName is a Types:QualifiedName.
func QualifiedName(module, name string) Content {
	return Content{Attrs: []Attr{{"moduleName", module}, {"baseName", name}}}
}

// TestcaseID is a Types:TciTestCaseIdType; BehaviourID a
// Types:TciBehaviourIdType. Both wrap a qualified name.
func TestcaseID(module, name string) Content {
	return Content{Kids: []*Node{QualifiedName(module, name).in(NSTypes, "name")}}
}

func BehaviourID(module, name string) Content { return TestcaseID(module, name) }

// Param is one actual parameter of a testcase, function or altstep.
type Param struct {
	Name string
	Mode string // "in", "inout" or "out"; empty when unknown
	Val  Value
}

// Params is a Types:TciParameterListType.
func Params(ps ...Param) Content {
	var kids []*Node
	for _, p := range ps {
		par := el(NSTypes, "par", p.Val.valueContent().in(NSTypes, "val"))
		if p.Name != "" {
			par.Attrs = append(par.Attrs, Attr{"name", p.Name})
		}
		if p.Mode != "" {
			par.Attrs = append(par.Attrs, Attr{"mode", p.Mode})
		}
		kids = append(kids, par)
	}
	return Content{Kids: kids}
}

// String, Integer, Boolean and Duration are the simple types TString,
// TInteger, TBoolean and TriTimerDurationType (xsd:float, seconds).
func String(s string) Content { return Content{Text: s} }

func Integer(n int64) Content { return Content{Text: strconv.FormatInt(n, 10)} }

func Boolean(b bool) Content { return Content{Text: strconv.FormatBool(b)} }

func Duration(seconds float64) Content {
	return Content{Text: strconv.FormatFloat(seconds, 'g', -1, 64)}
}

// Enumerations of SimpleTypes (clause 11.3.1).
const (
	TriOk    = "TRI_Ok"
	TriError = "TRI_Error"
	TciOk    = "TCI_Ok"
	TciError = "TCI_Error"

	ComponentInactive = "inactiveC"
	ComponentRunning  = "runningC"
	ComponentStopped  = "stoppedC"
	ComponentKilled   = "killedC"
	ComponentNull     = "nullC"

	TimerRunning  = "runningT"
	TimerInactive = "inactiveT"
	TimerExpired  = "expiredT"
	TimerNull     = "nullT"
)

// Verdict is a Values:VerdictValue: "none", "pass", "inconc", "fail" or
// "error".
func Verdict(v string) Content {
	return Content{Kids: []*Node{text(NSValues, "value", v)}}
}

// Message is a Types:TriMessageType: an encoded message, its octets in
// hex. With no octets, the element records that a message was there but
// its encoded form is not available.
func Message(hexOctets string) Content {
	if hexOctets == "" {
		return Content{}
	}
	return Content{Attrs: []Attr{{"val", hexOctets}}}
}

// Signature is a Types:TriSignatureIdType: the name of a procedure
// signature.
func Signature(name string) Content { return Content{Attrs: []Attr{{"val", name}}} }
