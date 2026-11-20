#!/usr/bin/env python3
"""Extract the TCI-TL XML schemas from ETSI ES 201 873-6 Annex B.

Usage:
    pdftotext -layout es_20187306v041201p.pdf es.txt
    python3 extract_xsd.py es.txt OUTDIR

Writes SimpleTypes, Types, Values, Templates, Events and Log _v4_10_1.xsd to
OUTDIR: the six schema listings of Annex B.1 to B.6, in order, with page
headers and footers removed. The published listings do not compile as
printed; each correction below is applied and reported, and each is listed
in ../doc.go. Nothing else is changed.
"""
import re
import sys

NAMES = ["SimpleTypes", "Types", "Values", "Templates", "Events", "Log"]


def listings(lines):
    """The six <?xml ... </xsd:schema> listings after the Annex B heading."""
    start = next(i for i, l in enumerate(lines)
                 if re.match(r"^\s*B\.1\s+TCI-TL XML Schema for Simple Types\s*$", l))
    out, cur = [], None
    for l in lines[start:]:
        if cur is None and l.lstrip().startswith("<?xml"):
            cur = []
        if cur is not None:
            s = l.strip()
            furniture = (not s or s == "ETSI" or re.match(r"^\d{1,3}$", s)
                         or re.match(r"^(\d+\s+)?ETSI ES 201 873-6 V", s))
            if not furniture:
                cur.append(l.rstrip())
            if s.endswith("</xsd:schema>"):
                out.append("\n".join(cur) + "\n")
                cur = None
                if len(out) == len(NAMES):
                    return out
    raise SystemExit("found %d schema listings, want %d" % (len(out), len(NAMES)))


def fix(name, txt):
    """Apply the corrections the published text needs, reporting each."""
    def sub(label, pattern, repl, flags=0):
        nonlocal txt
        new, n = re.subn(pattern, repl, txt, flags=flags)
        if n:
            print("%s: %s (%d)" % (name, label, n))
        txt = new

    # E1, E3: whitespace inside a QName-valued attribute.
    sub("E1 leading space in QName", r'((?:type|base|ref)=")\s+', r"\1")
    sub("E1 trailing space in QName", r'((?:type|base|ref)="[^"]*?)\s+"', r'\1"')
    sub("E3 space after QName prefix", r'((?:type|base|ref)="\w+:)\s+(\w+")', r"\1\2")
    # E7: trailing space in an element name.
    sub("E7 trailing space in element name", r'(name="[^"\s]+)\s+"', r'\1"')
    if name == "Values":
        # E4: the complex type Value is declared twice, identically.
        dup = re.compile(r'<xsd:complexType name="Value" mixed="true">\s*'
                         r'<xsd:group ref="Values:Value"/>\s*'
                         r'<xsd:attributeGroup ref="Values:ValueAtts"/>\s*'
                         r'</xsd:complexType>\n')
        found = dup.findall(txt)
        if len(found) == 2:
            txt = dup.sub("", txt, count=1)
            print("Values: E4 duplicate complexType Value removed (1)")
    if name == "Events":
        # E2: tliTcStop closes its extension before the sequence it extends with.
        sub("E2 tliTcStop sequence outside its extension",
            r'(<xsd:complexType name="tliTcStop">\s*<xsd:complexContent mixed="true">\s*'
            r'<xsd:extension base="Events:Event")/>(\s*<xsd:sequence>.*?</xsd:sequence>)',
            r"\1>\2\n        </xsd:extension>", re.S)
        # E6: two event types are not mixed although their base type is.
        sub("E6 missing mixed=\"true\"",
            r'(<xsd:complexType name="tliCtrl(?:StartWithParameters|TerminatedWithResult)">\s*'
            r'<xsd:complexContent)>', r'\1 mixed="true">')
        # E5: the Log schema references tliCheckAnyMismatch_m/_c, which are
        # not defined; clause 7.3.4.1.118/119 gives them the parameters of
        # tliCheckedAny_m/_c.
        if 'name="tliCheckAnyMismatch_m"' not in txt:
            m = re.search(r'<xsd:complexType name="tliCheckedAny_m">.*?'
                          r'<xsd:complexType name="tliCheckedAny_c">.*?</xsd:complexType>', txt, re.S)
            block = m.group(0).replace("tliCheckedAny_", "tliCheckAnyMismatch_")
            txt = txt[:m.end()] + "\n" + block + txt[m.end():]
            print("Events: E5 tliCheckAnyMismatch_m/_c defined (2)")
    return txt


def main():
    if len(sys.argv) != 3:
        raise SystemExit(__doc__)
    lines = open(sys.argv[1], encoding="utf-8").read().split("\n")
    for name, txt in zip(NAMES, listings(lines)):
        with open("%s/%s_v4_10_1.xsd" % (sys.argv[2], name), "w", encoding="utf-8") as f:
            f.write(fix(name, txt))


if __name__ == "__main__":
    main()
