// Package tl implements TTCN-3 test logging as ETSI ES 201 873-6 V4.12.1
// (TCI) defines it: the TCI-TL operations of clause 7.3.4.1, one event per
// TTCN-3 operation the executable performs, written in the XML mapping of
// Annex B.
//
// # Formats
//
// NewXMLWriter writes a `logfile` of the Log schema (B.6): a header with a
// version string and start time, then a body of events. Every event is the
// element the Log schema declares for its operation, with the content its
// Events type declares, in schema order; parameter values are Values (B.3)
// and templates Templates (B.4). A log is valid against the Annex B
// schemas; TestXMLValidatesAgainstAnnexB checks that with xmllint when the
// schemas are available (see gen/README.md).
//
// NewJSONLWriter writes the same element trees as JSON Lines, a header line
// and then one event per line, each element an object with tag, attrs,
// text and kids. Namespaces are implicit, since Annex B fixes each
// element's by its position. It converts to the XML form without loss,
// except for characters XML 1.0 cannot carry, such as most control
// characters, which XML writes as U+FFFD; JSON Lines keeps them.
//
// # Choices the standard leaves to the tool
//
//   - ts is in microseconds since 1970-01-01T00:00:00Z, and the header's
//     version string says so. Under the virtual clock an event's ts is its
//     testcase's start time plus the virtual time elapsed.
//   - A scalar value is written in TTCN-3 notation without literal
//     decoration, since the element names its type: 5, 1.5, true, pass,
//     0101 for a bitstring, 0A1B for an octetstring, the characters of a
//     charstring, the identifier of an enumerated value.
//   - Where the runtime does not know a value's declared type, the optional
//     type attribute is left out and the value's element is inferred from
//     its content; see value conversion in the interpreter.
//
// # Where Annex B and clause 7 differ
//
// Where the XML mapping and the abstract signatures of clause 7.3.4.1
// disagree, the log follows Annex B, because that is what it is validated
// against:
//
//   - D1: elements are named transmission-failure and encoder-failure, not
//     transmissionFailure and encoderFailure.
//   - D2: tliCStart and tliCCall name the behaviour element "name", not
//     "beh".
//   - D3: tliCDone and tliCKilled carry no verdict element.
//   - D4: tliCCreate has an optional hostId element.
//   - D5: Port/index is optional in B.2 and required in clause 11.3.2.4.
//   - D6: tliMDetected_m names its encoded message (Types:TriMessageType)
//     msgValue, and requires it; the abstract operation calls it msg.
//     The executor holds decoded values, so its octets are given only for
//     an octetstring payload.
//   - D7: the getreply events name the reply template replTmpl; the
//     abstract operations call it replyTmpl.
//
// # Errata in the published schemas
//
// The Annex B listings do not compile as printed. gen/extract_xsd.py
// applies these corrections, and nothing else:
//
//   - E1: whitespace at either end of a QName attribute value (Values
//     `type=" SimpleTypes:TEmpty"`, twice; Log
//     `type="Events:tliPrCatchTimeoutDetected "`). XSD collapses the
//     whitespace, but common validators reject it.
//   - E2: Events tliTcStop self-closes its xsd:extension and then places
//     the extending xsd:sequence outside it.
//   - E3: Events `type="Templates: TciNonValueTemplate"`, a space inside a
//     QName.
//   - E4: Values declares the complex type Value twice, identically.
//   - E5: Log references Events:tliCheckAnyMismatch_m and _c, which Events
//     does not define; they are defined with the parameters clause
//     7.3.4.1.118 and .119 give them, those of tliCheckedAny_m and _c.
//   - E6: Events tliCtrlStartWithParameters and tliCtrlTerminatedWithResult
//     are not mixed, although their base type Event is.
//   - E7: trailing space in an element name (Templates "any_element ",
//     Events "addrValue ").
package tl
