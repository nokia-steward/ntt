#!/usr/bin/env python3
"""Generate ../schema.go from the Annex B Events and Log schemas.

Usage:
    python3 gen_schema.py XSDDIR > ../schema.go && gofmt -w ../schema.go

XSDDIR is the output of extract_xsd.py. For each event element of the Log
schema's Body, lists the child elements its Events type declares, in schema
order, following extension bases up to Events:Event.

An element inside an xsd:choice is one alternative of it. Every choice in
the Events schema has an alternative that may be empty (an optional
encoder-failure or decoder-failure), so each of its elements is optional
by itself; what the schema forbids is elements of two alternatives.
"""
import sys
import xml.etree.ElementTree as ET

X = "{http://www.w3.org/2001/XMLSchema}"

HEADER = '''// Code generated from ETSI ES 201 873-6 V4.12.1 Annex B (TCI-TL XML
// mapping, schemas *_v4_10_1.xsd) by gen/gen_schema.py; DO NOT EDIT.
//
// For every event element of the Log schema's Body, the child elements its
// Events type declares, in schema order, after the common Event content
// (the "am" element and the ts/src/line/name/id/type attributes). Element
// names are the schema's, not the abstract signature's (for example
// "transmission-failure", not "transmissionFailure"). See doc.go.

package tl

// Field is one child element of an event, as the schema declares it.
type Field struct {
	Name     string // element name
	Type     string // schema type, prefixed with its schema
	Optional bool   // minOccurs="0", or one alternative of a choice
	// Choice numbers the event's xsd:choice the element is an alternative
	// of, from 1; 0 outside any. Alt numbers the alternative, from 1.
	Choice, Alt int
}

// Schema lists each event's fields in the order the schema requires.
var Schema = map[string][]Field{'''


def main():
    d = sys.argv[1]
    events = ET.parse(d + "/Events_v4_10_1.xsd").getroot()
    types = {ct.get("name"): ct for ct in events.findall(X + "complexType")}

    def fields(name):
        ct = types[name]
        cc = ct.find(X + "complexContent")
        out, seq = [], ct.find(X + "sequence")
        if cc is not None:
            ext = cc.find(X + "extension")
            base = ext.get("base").split(":")[1]
            if base != "Event":
                out += fields(base)
            seq = ext.find(X + "sequence")
        if seq is None:
            return out
        choice = 0
        for c in seq:
            if c.tag == X + "element":
                out.append(field(c, 0, 0))
            elif c.tag == X + "choice":
                choice += 1
                for alt, a in enumerate(c, 1):
                    for e in ([a] if a.tag == X + "element" else a.iter(X + "element")):
                        out.append(field(e, choice, alt))
            else:
                sys.exit("unexpected %s in %s" % (c.tag, name))
        return out

    def field(e, choice, alt):
        opt = choice > 0 or e.get("minOccurs", "1") == "0"
        return (e.get("name").strip(), e.get("type").replace(" ", ""), opt, choice, alt)

    log = ET.parse(d + "/Log_v4_10_1.xsd").getroot()
    body = next(c for c in log.findall(X + "complexType") if c.get("name") == "Body")
    print(HEADER)
    for e in body.iter(X + "element"):
        print('\t"%s": {' % e.get("name"))
        for n, t, opt, choice, alt in fields(e.get("type").split(":")[1]):
            extra = ", Choice: %d, Alt: %d" % (choice, alt) if choice else ""
            print('\t\t{Name: "%s", Type: "%s", Optional: %s%s},' % (n, t, "true" if opt else "false", extra))
        print("\t},")
    print("}")


if __name__ == "__main__":
    main()
