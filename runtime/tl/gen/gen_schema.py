#!/usr/bin/env python3
"""Generate ../schema.go from the Annex B Events and Log schemas.

Usage:
    python3 gen_schema.py XSDDIR > ../schema.go && gofmt -w ../schema.go

XSDDIR is the output of extract_xsd.py. For each event element of the Log
schema's Body, lists the child elements its Events type declares, in schema
order, following extension bases up to Events:Event.
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
	Optional bool   // minOccurs="0"
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
        if seq is not None:
            for e in seq.iter(X + "element"):
                out.append((e.get("name").strip(), e.get("type").replace(" ", ""),
                            e.get("minOccurs", "1") == "0"))
        return out

    log = ET.parse(d + "/Log_v4_10_1.xsd").getroot()
    body = next(c for c in log.findall(X + "complexType") if c.get("name") == "Body")
    print(HEADER)
    for e in body.iter(X + "element"):
        print('\t"%s": {' % e.get("name"))
        for n, t, opt in fields(e.get("type").split(":")[1]):
            print('\t\t{Name: "%s", Type: "%s", Optional: %s},' % (n, t, "true" if opt else "false"))
        print("\t},")
    print("}")


if __name__ == "__main__":
    main()
