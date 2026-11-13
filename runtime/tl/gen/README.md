# TCI-TL schema tooling

`../schema.go` and the schema check in `../tl_test.go` both come from the
XML schemas of ETSI ES 201 873-6 V4.12.1 Annex B. The schemas are ETSI's
text and are not included here; these scripts rebuild everything from a
copy of the standard.

```sh
# the standard, from ETSI's delivery site
curl -LO https://www.etsi.org/deliver/etsi_es/201800_201899/20187306/04.12.01_60/es_20187306v041201p.pdf
pdftotext -layout es_20187306v041201p.pdf es.txt

mkdir xsd
python3 extract_xsd.py es.txt xsd          # reports each correction it applies
python3 gen_schema.py xsd > ../schema.go && gofmt -w ../schema.go

# validate logs against the normative schemas
NTT_TCI_TL_XSD=$PWD/xsd go test ./runtime/tl/

# or any log ntt wrote
xmllint --noout --schema xsd/Log_v4_10_1.xsd run.xml
```

The published listings do not compile as printed. `extract_xsd.py` applies
seven corrections, E1 to E7, and prints each one as it applies it. They
are described in `../doc.go`, together with the places where the XML
mapping and the abstract operations of clause 7 differ.
