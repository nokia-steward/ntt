# Codec Next Slices

Generated after the JSON B309 default materialisation and XML loopback-transform slices.

## Current Codec Miss Pockets

- JSON is down to `6` misses: `5` under `json/B_encoding_instructions` and `1` object-identifier parse gap under `json/07_using_json_to_exchange_data`.
- XML/XER has `2` misses: name conversion and one anytype parse gap.

## XML Minimum Plan

1. Promote the new receive-side JSON default hook into a codec-neutral transform boundary:
   - keep `receiveTemplateTypeDesc` as the source of type and attribute context;
   - split default materialisation from JSON-specific directive parsing;
   - allow a codec transform to rewrite the shallow runtime value used for matching without mutating the queued payload.

2. Close the XML loopback transform fixtures before building a full XML parser:
   - `Pos_060106_whitespace_002/003`: complete via XSD whiteSpace facet loopback transforms;
   - `Pos_060112_fraction_digits_002`: complete via XSD fractionDigits loopback transform;
   - `Pos_07060606_effect_of_minoccurs_and_maxoccurs_005`: complete via sequence `minOccurs="0"` all-omit collapse.

3. XML header control is complete:
   - `B0329_xml_header_control_001/002` now pass through a minimal `encvalue_unichar` XML header-control path;
   - keep future XML work focused on typed receive transforms unless another `encvalue_*` parameter fixture appears.

4. Remaining XML work is parser/runtime schema edge handling:
   - `Pos_050202_name_conversion_rules_028`;
   - `Pos_0608_anytype_and_anysimpletype_types_008`;
   - JSON object-identifier and `B311_no_type_021` parser gaps.

## Validation Gate

For each slice, run focused XML/JSON fixture directories, `go test ./interpreter ./runtime/codec/...`, full conformance with `--baseline testdata/conformance-baseline.json --regress 0.5`, and refresh `docs/conformance/current-misses.*` only on a net gain.
