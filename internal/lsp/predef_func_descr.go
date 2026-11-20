package lsp

import "github.com/nokia/ntt/internal/lsp/protocol"

type PredefFunctionDetails struct {
	Label          string
	InsertText     string
	Signature      string
	Documentation  string
	NrOfParameters int
	TextFormat     protocol.InsertTextFormat
}

var PredefinedFunctions = []PredefFunctionDetails{
	{
		Label:          "int2char(...)",
		InsertText:     "int2char(${1:invalue})$0",
		Signature:      "function int2char(in integer invalue) return charstring",
		Documentation:  "## (TTCN-3)\nThe __int2char__ function converts an __integer__ value in the range of 0 to 127 (8-bit encoding) into a single-character-length\n __charstring__ value. The __integer__ value describes the 8-bit encoding of the character",
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "int2unichar(...)",
		InsertText: "int2unichar(${1:invalue})$0",
		Signature: `function int2unichar(
	in integer invalue
) return universal charstring`,
		Documentation:  "## (TTCN-3)\nThe __int2unichar__ function n converts an __integer__ value in the range of 0 to 2147483647 (32-bit encoding) into a single-character-length __universal charstring__ value. The __integer__ value describes the 32-bit encoding of the character.",
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "int2bit(...)",
		InsertText: "int2bit(${1:invalue}, ${2:length})$0",
		Signature: `function int2bit(
	in integer invalue,
	in integer length
) return bitstring`,
		Documentation:  "## (TTCN-3)\nThe __int2bit__ function converts a single __integer__ value to a single __bitstring__ value. The resulting string is length bits long. Error causes are:\n* invalue is less than zero;\n* the conversion yields a return value with more bits than specified by length.",
		NrOfParameters: 2,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "int2enum(...)",
		InsertText: "int2enum(${1:invalue}, ${2:outpar})$0",
		Signature: `function int2enum(
	in integer inpar,
	out Enumerated_type outpar)`,
		Documentation:  "## (TTCN-3)\nThe __int2enum__ function converts an integer value into an enumerated value of a given enumerated type. The integer value shall be provided as in parameter and the result of the conversion shall be stored in an out parameter. The type of the out parameter determines the type into which the in parameter is converted.",
		NrOfParameters: 2,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "int2hex(...)",
		InsertText: "int2hex(${1:invalue},${2:length})$0",
		Signature: `function int2hex(
	in integer invalue,
	in integer length
) return hexstring`,
		Documentation:  "## (TTCN-3)\nThe __int2hex__ function converts a single __integer__ value to a single __hexstring__ value. The resulting string is length hexadecimal digits long. Error causes are:\n* invalue is less than zero;\n* the conversion yields a return value with more hexadecimal characters than specified by length.",
		NrOfParameters: 2,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "int2oct(...)",
		InsertText: "int2oct(${1:invalue},${2:length})$0",
		Signature: `function int2oct(
	in integer invalue,
	in integer length
) return octetstring`,
		Documentation:  "## (TTCN-3)\nThe __int2oct__ function converts a single __integer__ value to a single __octetstring__ value. The resulting string is length octets long. Error causes are:\n* invalue is less than zero;\n* the conversion yields a return value with more octets than specified by length.",
		NrOfParameters: 2,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:          "int2str(...)",
		InsertText:     "int2str(${1:invalue})$0",
		Signature:      "function int2str(in integer invalue) return charstring",
		Documentation:  "## (TTCN-3)\nThe __int2str__ function converts the __integer__ value into its string equivalent (the base of the return string is always decimal). ",
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:          "int2float(...)",
		InsertText:     "int2float(${1:invalue})$0",
		Signature:      "function int2float(in integer invalue) return float",
		Documentation:  "## (TTCN-3)\nThe __int2float__ function converts an __integer__ value into a __float__ value.",
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:          "float2int(...)",
		InsertText:     "float2int(${1:invalue})$0",
		Signature:      "function float2int(in float invalue) return integer",
		Documentation:  "## (TTCN-3)\nThe __float2int__ function converts a __float__ value into an __integer__ value by removing the fractional part of the argument and returning the resulting __integer__. Error causes are:\n*invalue is __infinity__, __-infinity__ or not_a_number.",
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:          "char2int(...)",
		InsertText:     "char2int(${1:invalue})$0",
		Signature:      "function char2int(in charstring invalue) return integer",
		Documentation:  "## (TTCN-3)\nThe __char2int__ function converts a single-character-length __charstring__ value into an __integer__ value in the range of 0 to 127. The __integer__ value describes the 8-bit encoding of the character. Error causes are:\n* length of invalue does not equal 1.",
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:          "char2oct(...)",
		InsertText:     "char2oct(${1:invalue})$0",
		Signature:      "function char2oct(in charstring invalue) return octetstring",
		Documentation:  "## (TTCN-3)\nThe __char2oct__ function converts a __charstring__ invalue to an __octetstring__. Each octet of the octetstring will contain the Recommendation _ITU-T T.50 [4]_ codes (according to the IRV) of the appropriate characters of invalue.",
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "unichar2int(...)",
		InsertText: "unichar2int(${1:invalue})$0",
		Signature: `function unichar2int(
	in universal charstring invalue
) return integer`,
		Documentation: `## (TTCN-3)
The __unichar2int__ function converts a single-character-length __universal charstring__ value into an __integer__ value in the
range of 0 to 2147483647. The __integer__ value describes the 32-bit encoding of the character. Error causes are:
* length of invalue does not equal 1.`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "unichar2oct(...)",
		InsertText: "unichar2oct(${1:invalue})$0",
		Signature: `function unichar2oct(
	in universal charstring invalue,
	in charstring string_encoding := "UTF-8"
) return octetstring`,
		Documentation: `## (TTCN-3)
The __unichar2oct__ function This function converts a universal charstring invalue to an octetstring. Each octet of the octetstring
will contain the octets mandated by mapping the characters of invalue using the standardized mapping associated with
the given string_encoding in the same order as the characters appear in inpar. If the optional string_encoding parameter
is omitted, the default value "UTF-8".
The following values (see _ISO/IEC 10646 [2]_) are allowed as string_encoding actual parameter:

a) __"UTF-8"__ b) __"UTF-16"__ c) __"UTF-16LE"__ d) __"UTF-16BE"__ e) __"UTF-32"__ f) __"UTF-32LE"__ g) __"UTF-32BE"__`,
		NrOfParameters: 2,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "bit2int(...)",
		InsertText: "bit2int(${1:invalue})$0",
		Signature:  "function bit2int(in bitstring invalue) return integer",
		Documentation: `## (TTCN-3)
The __bit2int__ function This function converts a single __bitstring__ value to a single __integer__ value.`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "bit2hex(...)",
		InsertText: "bit2hex(${1:invalue})$0",
		Signature:  "function bit2hex(in bitstring invalue) return hexstring",
		Documentation: `## (TTCN-3)
The __bit2hex__ function converts a single __bitstring__ value to a single __hexstring__. The resulting __hexstring__
represents the same value as the __bitstring__. For the purpose of this conversion, a __bitstring__ shall be converted into a
__hexstring__, where the __bitstring__ is divided into groups of four bits beginning with the rightmost bit.
When the leftmost group of bits does contain less than 4 bits, this group is filled with _'0'B_ from the left until it contains
exactly 4 bits and is converted afterwards. The consecutive order of hex digits in the resulting __hexstring__ is the same as
the order of groups of 4 bits in the __bitstring__`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "bit2oct(...)",
		InsertText: "bit2oct(${1:invalue})$0",
		Signature:  "function bit2oct(in bitstring invalue) return octetstring",
		Documentation: `## (TTCN-3)
The __bit2oct__ function converts a single __bitstring__ value to a single __octetstring__. The resulting __octetstring__
represents the same value as the __bitstring__. For the conversion the following holds: * bit2oct(value)=hex2oct(bit2hex(value)).`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "bit2str(...)",
		InsertText: "bit2str(${1:invalue})$0",
		Signature:  "function bit2str(in bitstring invalue) return charstring",
		Documentation: `## (TTCN-3)
The __bit2str__ function converts a single __bitstring__ value to a single __charstring__.
The resulting __charstring__ has the same length as the __bitstring__ and contains only the __characters__ '0' and '1'.`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "hex2int(...)",
		InsertText: "hex2int(${1:invalue})$0",
		Signature:  "function hex2int(in hexstring invalue) return integer",
		Documentation: `## (TTCN-3)
The __hex2int__ function  converts a single __hexstring__ value to a single __integer__ value.
For the purposes of this conversion, a __hexstring__ shall be interpreted as a positive base 16 __integer__ value. The
rightmost hexadecimal digit is least significant, the leftmost hexadecimal digit is the most significant.`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "hex2bit(...)",
		InsertText: "hex2bit(${1:invalue})$0",
		Signature:  "function hex2bit(in hexstring invalue) return bitstring",
		Documentation: `## (TTCN-3)
The __hex2bit__ function converts a single __hexstring__ value to a single __bitstring__. The resulting __bitstring__
 represents the same value as the __hexstring__.`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "hex2oct(...)",
		InsertText: "hex2oct(${1:invalue})$0",
		Signature:  "function hex2oct(in hexstring invalue) return octetstring",
		Documentation: `## (TTCN-3)
The __hex2oct__ function This function converts a single __hexstring__ value to a single __octetstring__.
The resulting __octetstring__ represents the same value as the __hexstring__.
For the purpose of this conversion, a __hexstring__ shall be converted into a __octetstring__, where the
__octetstring__ contains the same sequence of hex digits as the __hexstring__ when the length of the __hexstring__
modulo 2 is 0. Otherwise, the resulting __octetstring__ contains 0 as leftmost hex digit followed by the same sequence
of hex digits as in the __hexstring__. `,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "hex2str(...)",
		InsertText: "hex2str(${1:invalue})$0",
		Signature:  "function hex2str(in hexstring invalue) return charstring",
		Documentation: `## (TTCN-3)
The __hex2str__ function converts a single __hexstring__ value to a single __charstring__. The resulting __charstring__
has the same length as the __hexstring__ and contains only the characters '0' to '9'and 'A' to 'F'.`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "oct2int(...)",
		InsertText: "oct2int(${1:invalue})$0",
		Signature:  "function oct2int(in octetstring invalue) return integer",
		Documentation: `## (TTCN-3)
The __oct2int__ function converts a single __octetstring__ value to a single __integer__ value.
For the purposes of this conversion, an __octetstring__ shall be interpreted as a positive base 16 integer value. The
rightmost hexadecimal digit is least significant, the leftmost hexadecimal digit is the most significant. The number of
hexadecimal digits provided shall be multiples of 2 since one octet is composed of two hexadecimal digits. The
hexadecimal digits 0 to F represent the decimal values 0 to 15 respectively.`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "oct2bit(...)",
		InsertText: "oct2bit(${1:invalue})$0",
		Signature:  "function oct2bit(in octetstring invalue) return bitstring",
		Documentation: `## (TTCN-3)
The __oct2bit__ function converts a single __octetstring__ value to a single __bitstring__.
The resulting __bitstring__ represents the same value as the octetstring.
For the conversion the following holds:
* oct2bit(value)=hex2bit(oct2hex(value))`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "oct2hex(...)",
		InsertText: "oct2hex(${1:invalue})$0",
		Signature:  "function oct2hex(in octetstring invalue) return hexstring",
		Documentation: `## (TTCN-3)
The __oct2hex__ function converts a single __octetstring__ value to a single __hexstring__.
The resulting __hexstring__ represents the same value as the __octetstring__.
For the purpose of this conversion, a __octetstring__ shall be converted into a __hexstring__ containing the same
sequence of hex digits as the __octetstring__.`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "oct2str(...)",
		InsertText: "oct2str(${1:invalue})$0",
		Signature:  "function oct2str(in octetstring invalue) return charstring",
		Documentation: `## (TTCN-3)
The __oct2str__ function converts an __octetstring__ invalue to an __charstring__ representing the string equivalent of the
input value. The resulting __charstring__ shall have double the length as the incoming __octetstring__.
For the purpose of this conversion each hex digit of invalue is converted into a character '0', '1', '2', '3', '4', '5', '6', '7',
'8', '9', 'A', 'B', 'C', 'D', 'E' or 'F' echoing the value of the hex digit. The consecutive order of __characters__
in the resulting __charstring__ is the same as the order of hex digits in the __octetstring__.`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "oct2char(...)",
		InsertText: "oct2char(${1:invalue})$0",
		Signature:  "function oct2char(in octetstring invalue) return charstring",
		Documentation: `## (TTCN-3)
The __oct2char__ function converts an __octetstring__ invalue to a __charstring__. The input parameter invalue shall not
contain octet values higher than __7F__. The resulting __charstring__ shall have the same length as the input
__octetstring__. The octets are interpreted as Recommendation _ITU-T T.50 [4]_ codes (according to the IRV) and the
resulting characters are appended to the returned value.`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "oct2unichar(...)",
		InsertText: "oct2unichar(${1:invalue})$0",
		Signature: `function oct2unichar(
	in octetstring invalue,
	in charstring string_encoding := "UTF-8"
) return universal charstring`,
		Documentation: `## (TTCN-3)
The __oct2unichar__ function converts an __octetstring__ invalue to a __universal charstring__ by use of the given
string_encoding. The octets are interpreted as mandated by the standardized mapping associated with the given
string_encoding and the resulting characters are appended to the returned value. If the optional string_encoding
parameter is omitted, the default value "UTF-8".
The following values (see _ISO/IEC 10646 [2]_) are allowed as string_encoding actual parameters (for the description of
the codepoints see clause 27.5):

a) __"UTF-8"__ b) __"UTF-16"__ c) __"UTF-16LE"__ d) __"UTF-16BE"__ e) __"UTF-32"__ f) __"UTF-32LE"__ g) __"UTF-32BE"__`,
		NrOfParameters: 2,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "str2int(...)",
		InsertText: "str2int(${1:invalue})$0",
		Signature:  "function str2int(in charstring invalue) return integer",
		Documentation: `## (TTCN-3)
The __str2int__ function converts a __charstring__ representing an __integer__ value to the equivalent integer.
Error causes are:
* invalue contains characters other than "0", "1", "2", "3", "4", "5", "6", "7", "8", "9" and "-".
* invalue contains the character "-" at another position than the leftmost one.`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "str2hex(...)",
		InsertText: "str2hex(${1:invalue})$0",
		Signature:  "function str2hex(in charstring invalue) return hexstring",
		Documentation: `## (TTCN-3)
The __str2hex__ function converts a string of the type __charstring__ to a __hexstring__. The string invalue shall contain the
"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "a", "b", "c", "d", "e" "f", "A", "B", "C", "D", "E" or "F" graphical
characters only. Each character of invalue shall be converted to the corresponding hexadecimal digit. The resulting
hexstring will have the same length as the incoming charstring.
Error cause is:
* invalue contains characters other than specified above.`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "str2oct(...)",
		InsertText: "str2oct(${1:invalue})$0",
		Signature:  "function str2oct(in charstring invalue) return octetstring",
		Documentation: `## (TTCN-3)
The __str2oct__ function converts a string of the type __charstring__ to an __octetstring__. The string invalue shall contain
the "0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "a", "b", "c", "d", "e" "f", "A", "B", "C", "D", "E" or "F" graphical
characters only. When the string invalue contains even number characters the resulting octetstring contains 0
as leftmost character followed by the same sequence of characters as in the charstring.
lengthof (see clause C.2.1 for the resulting octetstring) will return half of lengthof of the incoming
charstring. In addition to the general error causes in clause 16.1.2, error causes is:
* invalue contains characters other than specified above.`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "str2float(...)",
		InsertText: "str2float(${1:invalue})$0",
		Signature:  "function str2float(in charstring invalue) return float",
		Documentation: `## (TTCN-3)
The __str2float__ function converts a __charstring__ comprising a number into a __float__ value. The format of the number in the
__charstring__ shall follow rules of plain ttcn-3 float values with the following exceptions:
* leading zeros are allowed;
* leading "+" sign before positive values is allowed;
* "-0.0" is allowed;
* no numbers after the dot in the decimal notation are allowed.
In addition to the general error causes in clause 16.1.2, error causes are:
* the format of invalue is different than defined above.`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "enum2int(...)",
		InsertText: "enum2int(${1:invalue})$0",
		Signature:  "function enum2int(in Enumerated_type inpar) return integer",
		Documentation: `## (TTCN-3)
The __enum2int__ function accepts an enumerated value and returns the integer value associated to the enumerated value.
The actual parameter passed to inpar always shall be a typed object.`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "any2unistr(...)",
		InsertText: "any2unistr(${1:invalue})$0",
		Signature: `function any2unistr(
	in template any_type invalue,
	in universal charstring format := ""
) return universal charstring`,
		Documentation: `## (TTCN-3)
The __any2unistr__ function converts the content of a value or template to a single __universal charstring__. The resulting
__universal charstring__ is the same as the string produced by the __log__ operation containing the same operand as
the one passed to the __any2unistr__ function. The value or template passed as a parameter to the __any2unichar__
function may be uninitialized, partially or completely initialized.
The optional format parameter is used for dynamic selection of how the resulting __universal charstring__
should be produced from the provided invalue.
* "": the resulting universal charstring is the same as the string produced by the __log__ operation for the same
operand.
* "canonical": unbound fields are represented in the output as "-", the fields and members of structured types are represented recursively
in assignment notation.
* <custom string>: tool specific output`,
		NrOfParameters: 2,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "lengthof(...)",
		InsertText: "lengthof(${1:invalue})$0",
		Signature: `function lengthof(
	in template (present) any_string_or_list_type inpar
) return integer`,
		Documentation: `## (TTCN-3)
The __lengthof__ function returns the length of a value or template that is of type __bitstring__, __hexstring__,
__octetstring__, __charstring__, __universal charstring__, __record of__, __set of__, or __array__.`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "sizeof(...)",
		InsertText: "sizeof(${1:invalue})$0",
		Signature: `function sizeof(
	in template (present) any_record_set_type inpar
) return integer`,
		Documentation: `## (TTCN-3)
The __sizeof__ function returns the actual number of elements of a value or template of a __record__ or __set__ type (see note).
The function __sizeof__ is applicable to templates of __record__ and __set__ types. The function is applicable only if
the __sizeof__ function gives the same result on all values that match the template.

NOTE: Only elements of the TTCN-3 object, which is the parameter of the function are calculated; i.e. no
elements of nested types/values are taken into account at determining the return value.

Error causes are:
* when inpar is a template and it can match values of different sizes.`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "ispresent(...)",
		InsertText: "ispresent(${1:invalue})$0",
		Signature:  "function ispresent(in template any_ type inpar) return boolean",
		Documentation: `## (TTCN-3)
The __ispresent__ function is allowed for templates of all data types and returns:
* the value __true__ if the data object reference fulfils the (present) template restriction as described in clause 15.8;
* the value __false__ otherwise.`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "ischosen(...)",
		InsertText: "ischosen(${1:invalue})$0",
		Signature: `function ischosen(
	in template any_union_type_field inpar
) return boolean`,
		Documentation: `## (TTCN-3)
The __ischosen__ function is allowed for templates of all data types that are a union-field-reference or a type alternative of an
anytype. This function returns:
* the value __true__ if and only if the data object reference specifies the variant of the union type or the type
alternative of the anytype that is actually selected for the given data object;
* in all other cases __false__.`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "isvalue(...)",
		InsertText: "isvalue(${1:invalue})$0",
		Signature:  "function isvalue(in template any_type inpar) return boolean",
		Documentation: `## (TTCN-3)
The __isvalue__ function is allowed for templates of all data types, component and address types and default values. The function
shall return __true__, if inpar is completely initialized and resolves to a specific value. If inpar is of __record__
or __set__ type, omitted optional fields shall be considered as initialized, i.e. the function shall also return __true__ if optional fields of
inpar are set to omit. The function shall return __false__ otherwise.

The __null__ value assigned to default and component references shall be considered as concrete values.`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "isbound(...)",
		InsertText: "isbound(${1:invalue})$0",
		Signature:  "function isbound(in template any_type inpar) return boolean",
		Documentation: `## (TTCN-3)
The __isbound__ function is allowed for templates of all data types. The function shall return __true__, if inpar is at least partially
initialized. If inpar is of a __record__ or __set__ type, omitted optional fields shall be considered as initialized, i.e. the
function shall also return __true__ if at least one optional field of inpar is set to omit. The function shall return __false__
otherwise. Inaccessible fields (e.g. non-selected alternatives of __union__ types, subfields of omitted __record__ and __set__ types
or subfields of non-selected __union__ fields) shall be considered as uninitialized, i.e. __isbound__ shall return for them __false__.
The __null__ value assigned to default and component references shall be considered as concrete values.`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "istemplatekind(...)",
		InsertText: "istemplatekind(${1:invalue}, ${2:kind})$0",
		Signature: `function istemplatekind (
	in template any_type invalue,
	in charstring kind
) return boolean`,
		Documentation: `## (TTCN-3)
The __istemplatekind__ function allows to examine if a template contains a certain kind of the matching mechanisms.
If the matching mechanism kind enquired is matching a _specific value_, a _matching mechanism instead of
values_ or matching _character pattern_, the function shall return __true__ if the content of the
invalue parameter is of the same kind.

If the matching mechanism kind enquired is a matching mechanism _inside values_, the function shall
return __true__ if the template in the invalue parameter contains this kind of matching mechanism on the first level of
nesting.
If the matching mechanism kind enquired is a matching attribute, the function shall return __true__ if the
template in the invalue parameter has this kind of matching attribute attached to it directly (i.e. it doesn't count if the
attribute is attached to a field of invalue at any level of nesting).
In all other cases the function returns __false__.

| Value of kind parameter | Searched matching mechanism |
| ----------------------- | --------------------------- |
| "list"                  | Template list               |
| "complement" | Complemented template list |
| "AnyValue", "?" | Any value |
| "AnyValueOrNone", "*" | Any value or none |
| "range" | Value range |
| "superset" | SuperSet |
| "subset" | SubSet |
| "omit" | Omit |
| "decmatch" |Matching decoded content |
| "AnyElement" | Any element |
| "AnyElementsOrNone" | Any number of elements or none |
| "permutation" | Permutation |
| "length" | Length restriction |
| "ifpresent" | The IfPresent indicator |
| "pattern" | Matching character pattern |
`,
		NrOfParameters: 2,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "regexp(...)",
		InsertText: "regexp${1}(${2:inpar}, ${3:expression}, ${4:groupno})$0",
		Signature: `function regexp [@nocase] (
	in template (value) any_character_string_type inpar,
	in template (present) any_character_string_type expression,
	in integer groupno
) return any_character_string_type`,
		Documentation: `## (TTCN-3)
The __regexp__ function first matches the parameter inpar (or in case inpar is a template, its value equivalent) against the
expression in the second parameter. If the __@nocase__ modifier is present, this and all subsequent matchings
shall be done in a case-insensitive way. If this matching is unsuccessful, an empty string shall be returned.
If this matching is successful, the substring of inpar shall be returned, which matched the groupno-s group of
expression during the matching. Group numbers are assigned by the order of occurrences of the opening bracket of
a group and counted starting from 0 by step 1.

_NOTE_: This function differs from other well-known regular expression matching implementations in that:
1. It shall match the whole inpar string instead of only a substring.
2. It starts counting groups from 0, while in some other implementations the first group is referenced
by 1 and the whole substring matched by the expression is referenced by 0.
`,
		NrOfParameters: 3,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "substr(...)",
		InsertText: "substr(${1:inpar}, ${2:index}, ${3:count})$0",
		Signature: `function substr(
	in template (present) any inpar,
	in integer index,
	in integer count
) return input_string_or_sequence_type`,
		Documentation: `## (TTCN-3)
This function returns a substring or subsequence from a value that is of a binary string type (__bitstring__,
__hexstring__, __octetstring__), a character string type (__charstring__, __universal charstring__), or a sequence
type (__record of__, __set of__ or array). If _inpar_ is a literal (i.e. type is not explicitly given) the corresponding type
shall be retrieved from the value contents. The type of the substring or subsequence returned is the root type of the input
parameter. The starting point of substring or subsequence to return is defined by the second parameter (_index_).
Indexing starts from zero. The third input parameter (_count_) defines the length of the substring or subsequence to be
returned. The units of length for string types are as defined in table 4 of the TTCN-3 core language specification. For sequence types, the
unit of length is element.

Please note that the root types of arrays is __record of__, therefore if _inpar_ is an array the returned type
is __record of__. This, in some cases, may lead to different indexing in _inpar_ and in the returned value.

When used on templates of character string types, specific values and patterns that contain literal characters and the
following metacharacters: "?", "*" are allowed in _inpar_ and the function shall return the character representation of
the matching mechanisms. When _inpar_ is a template of binary string or sequence type or is an array, only the specific
value combined templates whose elements are specific values, and AnyElement matching mechanisms or combined
templates are allowed and the substring or subsequence to be returned shall not contain AnyElement or combined
template.

In addition to the general error causes in clause 16.1.2, error causes are:

* _index_ is less than zero;
* _count_ is less than zero;
* _index_ + _count___ is greater than __lengthof__(_inpar_);
* _inpar_ is a template of a character string type and contains a matching mechanism other than a specific value
or pattern; or if the pattern contains other metacharacters than "?", "*";
* _inpar_ is a template of a binary string or sequence type or array and it contains other matching mechanism as
specific value or combined template; or if the elements of combined template are any other matching
mechanisms than specific values, and AnyElement or combined templates;
* _inpar_ is a template of a binary string or sequence type or array and the substring or subsequence to be
returned contains the AnyElement matching mechanism or combined templates;
* the template passed to the _inpar_ parameter is not of type bitstring, hexstring, octetstring,
charstring, universal charstring, __record of__, __set of__, or array.

Examples:

	substr('00100110'B, 3, 4) // returns '0011'B
	substr('ABCDEF'H, 2, 3) // returns 'CDE'H
	substr('01AB23CD'O, 1, 2) // returns 'AB23'O
	substr("My name is JJ", 11, 2) // returns "JJ"
	substr({ 4, 5, 6 }, 1, 2) // returns {5, 6}`,
		NrOfParameters: 3,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "replace(...)",
		InsertText: "replace(${1:inpar}, (${2:index}, (${3:len}, (${4:repl})$0",
		Signature: `function replace(
	in any inpar,
	in integer index,
	in integer len,
	in any repl
) return any_string_or_sequence type`,
		Documentation: `## (TTCN-3)
This function replaces the substring or subsequence of value _inpar_ at index _index_ of length _len_ with the string or
sequence value _repl_ and returns the resulting string or sequence. _inpar_ shall not be modified. If _len_ is 0 the string
or sequence _repl_ is inserted. If _index_ is 0, _repl_ is inserted at the beginning of _inpar_. If _index_ is
__lengthof(_inpar_), _repl_ is inserted at the end of _inpar_. If _inpar_ is a literal (i.e. type is not explicitly given) the
corresponding type shall be retrieved from the value contents. _inpar_ and _repl_, and the returned string or sequence
shall be of the same root type. The function replace can be applied to _bitstring_, _hexstring_, _octetstring_, or
any character string, __record of__, __set of__, or arrays. Note that indexing in strings starts from zero.

Please note that the root types of arrays is __record of__, therefore if _inpar_ or _repl_ or both are an
array, the returned type is __record of__. This, in some cases, may lead to different indexing in _inpar_
and/or _repl_ and in the returned value.

In addition to the general error causes in clause 16.1.2, error causes are:
* _inpar_ or _repl_ are not of string, __record of__, __set of__, or array type;
* _inpar_ and _repl_ are of different root type;
* _index_ is less than 0 or greater than __lengthof(_inpar_);
* _len_ is less than 0 or greater than __lengthof(_inpar_);
* _index_+_len_ is greater than __lengthof(_inpar_).

Examples:

	replace ('00000110'B, 1, 3, '111'B) // returns '01110110'B
	replace ('ABCDEF'H, 0, 2, '123'H) // returns '123CDEF'H
	replace ('01AB23CD'O, 2, 1, 'FF96'O) // returns '01ABFF96CD'O
	replace ("My name is JJ", 11, 1, "xx") // returns "My name is xxJ"
	replace ("My name is JJ", 11, 0, "xx") // returns "My name is xxJJ"
	replace ("My name is JJ", 2, 2, "x") // returns "Myxame is JJ",
	replace ("My name is JJ", 12, 2, "xx") // produces test case error
	replace ("My name is JJ", 13, 2, "xx") // produces test case error
	replace ("My name is JJ", 13, 0, "xx") // returns "My name is JJxx"`,
		NrOfParameters: 4,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "encvalue(...)",
		InsertText: "encvalue(${1:inpar}, ${2:encoding_info}, ${3:dynamic_encoding})$0",
		Signature: `function encvalue(
	in template (value) any inpar,
	in universal charstring encoding_info := "",
	in universal charstring dynamic_encoding := ""
) return bitstring`,
		Documentation: `## (TTCN-3)
The __encvalue__ function encodes a value or template into a bitstring. When the actual parameter that is passed to
_inpar_ is a template, it shall resolve to a specific value (the same restrictions apply as for the argument of the send
statement). The returned bitstring represents the encoded value of _inpar_, however, the TTCN-3 test system need not
make any check on its correctness. The optional _encoding_info_ parameter is used for passing additional encoding
information to the codec and, if it is omitted, no additional information is sent to the codec.

The optional _dynamic_encoding_ parameter is used for dynamic selection of encode attribute of the _inpar_ value
for this single __encvalue__ call. The rules for dynamic selection of the encode attribute are described in clause 27.9 of the TTCN-3 core language specification.

In addition to the general error causes in clause 16.1.2, error causes are:

* Encoding fails due to a runtime system problem (i.e. no encoding function exists for the actual type of
_inpar_).`,
		NrOfParameters: 3,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "decvalue(...)",
		InsertText: "decvalue(${1:encoded_value}, ${2:decoded_value}, ${3:decoding_info}, ${4:dynamic_encoding})$0",
		Signature: `function decvalue(
	inout bitstring encoded_value,
	out any decoded_value,
	in universal charstring decoding_info := "",
	in universal charstring dynamic_encoding := ""
) return integer`,
		Documentation: `## (TTCN-3)
The __decvalue__ function decodes a bitstring into a value. The test system shall suppose that the bitstring
_encoded_value_ represents an encoded instance of the actual type of _decoded_value_. The optional
_decoding_info_ parameter is used for passing additional decoding information to the codec and, if it is omitted, no
additional information is sent to the codec.

The optional _dynamic_encoding_ parameter is used for dynamic selection of encode attribute of the
_decoded_value_ parameter for this single __decvalue__ call. The rules for dynamic selection of the encode attribute
are described in clause 27.9 of the TTCN-3 core language specification.

If the decoding was successful, then the used bits are removed from the parameter _encoded_value_, the rest is
returned (in the parameter _encoded_value_), and the decoded value is returned in the parameter _decoded_value_.
If the decoding was unsuccessful, the actual parameters for _encoded_value_ and _decoded_value_ are not
changed. The function shall return an integer value to indicate success or failure of the decoding below:

* The return value 0 indicates that decoding was successful.
* The return value 1 indicates an unspecified cause of decoding failure. This value is also returned if the
_encoded_value_ parameter contains an unitialized value.
* The return value 2 indicates that decoding could not be completed as _encoded_value_ did not contain
enough bits.`,
		NrOfParameters: 4,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "encvalue_unichar(...)",
		InsertText: "encvalue_unichar(${1:inpar}, ${2:string_serialization}, ${3:encoding_info}, ${4:dynamic_encoding})$0",
		Signature: `function encvalue_unichar(
	in template (value) any inpar,
	in charstring string_serialization := "UTF-8",
	in universal charstring encoding_info := "",
	in universal charstring dynamic_encoding := ""
) return universal charstring`,
		Documentation: `## (TTCN-3)
The __encvalue_unichar__ function encodes a value or template into a universal charstring. When the actual
parameter that is passed to _inpar_ is a template, it shall resolve to a specific value (the same restrictions apply as for
the argument of the send statement). The returned universal charstring represents the encoded value of _inpar_,
however, the TTCN-3 test system need not make any check on its correctness. If the optional _string_serialization_
parameter is omitted, the default value "UTF-8" is used. The optional _encoding_info_ parameter is used for passing
additional encoding information to the codec and, if it is omitted, no additional information is sent to the codec.

The optional _dynamic_encoding_ parameter is used for dynamic selection of encode attribute of the _inpar_ value
for this single __encvalue_unichar__ call. The rules for dynamic selection of the encode attribute are described in
clause 27.9 of the TTCN-3 core language specification.

The following values (see ISO/IEC 10646 [2]) are allowed as _string_serialization_ actual parameters (for the description
of the UCS encoding scheme see clause 27.5):
* "UTF-8"
* "UTF-16"
* "UTF-16LE"
* "UTF-16BE"
* "UTF-32"
* "UTF-32LE"
* "UTF-32BE"

The serialized bitstring shall not include the optional signature (see clause 10 of ISO/IEC 10646 [2], also known as byte
order mark).

In case of "UTF-16" and "UTF-32" big-endian ordering shall be used (as described in clauses 10.4 and 10.7 of
ISO/IEC 10646 [2]).

The specific semantics of this function are explained by the following TTCN-3 definition:

	function encvalue_unichar(in template(value) any inpar,
	            in charstring enc
	            in universal charstring encoding_info := "",
	            in universal charstring dynamic_encoding := "") return universal charstring {
		return oct2unichar(bit2oct(encvalue(inpar, encoding_info, dynamic_encoding)), enc);
	}`,
		NrOfParameters: 4,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "decvalue_unichar(...)",
		InsertText: "decvalue_unichar(${1:encoded_value}, ${2:decoded_value}, ${3:string_serialization}, ${4:decoding_info}, ${5:dynamic_encoding},)$0",
		Signature: `function decvalue_unichar(
	inout universal charstring encoded_value,
	out any decoded_value,
	in charstring string_serialization:= "UTF-8",
	in universal charstring decoding_info := "",
	in universal charstring dynamic_encoding := ""
) return integer`,
		Documentation: `## (TTCN-3)
The __decvalue_unichar__ function decodes (part of) a universal charstring into a value. The test system shall
suppose that a prefix of the universal charstring _encoded_value_ represents an encoded instance of the actual type of
_decoded_value_. The optional _decoding_info_ parameter is used for passing additional decoding information to
the codec and, if it is omitted, no additional information is sent to the codec.

The optional _dynamic_encoding_ parameter is used for dynamic selection of encode attribute of the
_decoded_value_ parameter for this single __decvalue_unichar__ call. The rules for dynamic selection of the
encode attribute are described in clause 27.9.

If the decoding was successful, then the characters used for decoding are removed from the parameter
_encoded_value_, the rest is returned (in the parameter _encoded_value_), and the decoded value is returned in the
parameter _decoded_value_. If the decoding was unsuccessful, the actual parameters for _encoded_value_ and
_decoded_value_ are not changed. The function shall return an integer value to indicate success or failure of the
decoding below:

* The return value 0 indicates that decoding was successful.
* The return value 1 indicates an unspecified cause of decoding failure. This value is also returned if the
_encoded_value_ parameter contains an unitialized value.
* The return value 2 indicates that decoding could not be completed as _encoded_value_ did not contain
enough bits.

If the optional _string_serialization_ parameter is omitted, the default value "UTF-8" is used.

The following values (see ISO/IEC 10646 [2]) are allowed as _string_serialization_ actual parameters (for the description
of the UCS encoding scheme see clause 27.5 of TTCN-3 core language specification):

* "UTF-8"
* "UTF-16"
* "UTF-16LE"
* "UTF-16BE"
* "UTF-32"
* "UTF-32LE"
* "UTF-32BE"

The serialized bitstring shall not include the optional signature (see clause 10 of ISO/IEC 10646 [2], also known as byte
order mark).

In case of "UTF-16" and "UTF-32" big-endian ordering shall be used (as described in clauses 10.4 and 10.7 of
ISO/IEC 10646 [2]).
The semantics of the function can be explained by the following TTCN-3 function:

	function decvalue_unichar (
	            inout universal charstring encoded_value,
	            out any decoded_value,
	            in charstring string_encoding := "UTF-8",
	            in universal charstring decoding_info := "",
	            in universal charstring dynamic_encoding := "") return integer {
		var bitstring v_str = oct2bit(unichar2oct(encoded_value, string_encoding));
		var integer v_result := decvalue(v_str, decoded_value, decoding_info, dynamic_encoding);
		if (v_result == 0) { // success
			encoded_value := oct2unichar(bit2oct(v_str), string_encoding);
		}
		return v_result;
	}`,
		NrOfParameters: 5,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "encvalue_o(...)",
		InsertText: "encvalue_o(${1:inpar}, ${2:encoding_info}, ${3:dynamic_encoding}, ${4:bit_length})$0",
		Signature: `function encvalue_o(
	in template (value) any inpar,
	in universal charstring encoding_info := "",
	in universal charstring dynamic_encoding := "",
	out integer bit_length
) return octetstring`,
		Documentation: `## (TTCN-3)
The __encvalue_o__ function encodes a value or template into an octetstring. When the actual parameter that is passed
to _inpar_ is a template, it shall resolve to a specific value (the same restrictions apply as for the argument of the send
statement). The returned octetstring represents the encoded value of _inpar_, however, the TTCN-3 test system need not
make any check on its correctness. In case the encoded message is not octet-based and has a bit length not divisable by
8, the encoded message will be left-aligned in the returned octetstring and the least significant (8 - (bit length mod 8))
bits in the least significant octet will be 0. The bit length can be assigned to a variable by usage of the formal out
parameter _bit_length_. The optional _encoding_info_ parameter is used for passing additional encoding
information to the codec and, if it is omitted, no additional information is sent to the codec.

The optional _dynamic_encoding_ parameter is used for dynamic selection of encode attribute of the _inpar_ value
for this single __encvalue_o__ call. The rules for dynamic selection of the encode attribute are described in clause 27.9.

In addition to the general error causes in clause 16.1.2 of the TTCN-3 core language specification, error causes are:
* Encoding fails due to a runtime system problem (i.e. no encoding function exists for the actual type of
_inpar_).`,
		NrOfParameters: 4,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "decvalue_o(...)",
		InsertText: "decvalue_o(${1:encoded_value}, ${2:decoded_value}, ${3:decoding_info}, ${4:dynamic_encoding})$0",
		Signature: `function decvalue_o(
    inout octetstring encoded_value,
    out any decoded_value,
    in universal charstring decoding_info := "",
    in universal charstring dynamic_encoding := ""
) return integer`,
		Documentation: `## (TTCN-3)
The __decvalue_o__ function decodes an octetstring into a value. The test system shall suppose that the octetstring
_encoded_value_ represents an encoded instance of the actual type of _decoded_value_. The optional
_decoding_info_ parameter is used for passing additional decoding information to the codec and, if it is omitted, no
additional information is sent to the codec.

The optional _dynamic_encoding_ parameter is used for dynamic selection of __encode__ attribute of the
decoded_value parameter for this single __decvalue_o__ call. The rules for dynamic selection of the __encode__
attribute are described in clause 27.9 of the TTCN-3 core language specification.

If the decoding was successful, then the used octets are removed from the parameter _encoded_value_, the rest is
returned (in the parameter _encoded_value_), and the decoded value is returned in the parameter _decoded_value_.
If the decoding was unsuccessful, the actual parameters for _encoded_value_ and _decoded_value_ are not
changed. The function shall return an integer value to indicate success or failure of the decoding below:

* The return value 0 indicates that decoding was successful.
* The return value 1 indicates an unspecified cause of decoding failure. This value is also returned if the
_encoded_value_ parameter contains an unitialized value.
* The return value 2 indicates that decoding could not be completed as _encoded_value_ did not contain
enough octets.
`,
		NrOfParameters: 4,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "get_stringencoding(...)",
		InsertText: "get_stringencoding(${1:encoded_value})$0",
		Signature: `function get_stringencoding(
	in octettstring encoded_value
) return octettstring`,
		Documentation: `## (TTCN-3)
The __get_stringencoding__ function analyses the encoded_value and returns the UCS encoding scheme according to
clause 10 of ISO/IEC 10646 [2] (see also clause 27.5 of the TTCN-3 core language specification). The identified encoding scheme, or the
value "<unknown>", if the type of encoding cannot be determined unanimously, shall be returned as a character string.

The initial octet sequence (also known as byte order mark, BOM), when present, allows identifying the
encoding scheme unanimously. When it is not present, other symptoms may be used to identify the
encoding scheme unanimously; for example, only UTF-8 may have odd number of octets and bit
distribution according to table 2 of clause 9.1 of ISO/IEC 10646 [2].

Example:

    match ( get_stringencoding('6869C3BA7A'O),charstring:"UTF-8") // true
    //(the octetstring contains the UTF-8 encoding of the character sequence "hiúz")
`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "remove_bom(...)",
		InsertText: "remove_bom(${1:encoded_value})$0",
		Signature: `function remove_bom(
	in octettstring encoded_value
) return octettstring`,
		Documentation: `## (TTCN-3)
The __remove_bom__ function removes the optional FEFF ZERO WIDTH NO-BREAK SPACE sequence that may be
present at the beginning of a stream of serialized (encoded) universal character strings to indicate the order of the octets
within the encoding form, as defined in clause 10 of ISO/IEC 10646 [2]. If no FEFF ZERO WIDTH NO-BREAK
SPACE sequence present in the _encoded_value_ parameter, the function shall return the value of the parameter
without change.`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "rnd(...)",
		InsertText: "rnd(${1:seed})$0",
		Signature:  "function rnd([in float seed]) return float",
		Documentation: `## (TTCN-3)
The __rnd__ function returns a (pseudo) random number less than 1 but greater or equal to 0. The random number
generator is initialized per test component and for the control part by means of an optional seed value (a numerical float
value). If no new seed is provided, the last generated number will be used as seed for the next random number. Without
a previous initialization a value calculated from the system time will be used as seed value when __rnd__ is used the first
time in a test component or the control part.

Each time the __rnd__ function is initialized with the same seed value, it shall repeat the same sequence of random
numbers.

For the purpose of keeping parallel testing deterministic, each test component, as well as the control part
has its own random seed. This allows for better reproducibility of test executions. Thus, the __rnd__ function
will always use the seed of the component or control part which calls it.

To produce a random integers in a given range, the following formula can be used:

    float2int(int2float(upperbound - lowerbound +1)*rnd()) + lowerbound
    // Here, upperbound and lowerbound denote highest and lowest number in range.

`,
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "testcasename()",
		InsertText: "testcasename()$0",
		Signature:  "function testcasename() return charstring",
		Documentation: `## (TTCN-3)
The __testcasename__ function shall return the unqualified name of the actually executing test case.

When the function __testcasename__ is called if the control part is being executed but no testcase, it shall return the
empty string.`,

		NrOfParameters: 0,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "hostid(...)",
		InsertText: "hostid($1)$0",
		Signature: `function hostid(
	in charstring idkind := "Ipv4orIPv6"
) return charstring`,
		Documentation:  "## (TTCN-3)\nThe __hostid__ function shall return the host id of the test component or module control executing the hostid function	in form of a character string. The in parameter idkind allows to specify the expected id format to be returned.	Predefined _idkind_ values are:\n* \"Ipv4orIPv6\": The contents of the returned character string is an Ipv4 address. If no Ipv4 address, but an	Ipv6 address is available, a character string representation of the Ipv6 address is returned.\n* \"Ipv4\": The contents of the returned character string shall be an Ipv4 address.\n* \"Ipv6\": The contents of the returned characterstring shall be an Ipv6 address.",
		NrOfParameters: 1,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "match(...)",
		InsertText: "match(${1:expression}, ${2:templateInstance})$0",
		Signature: `function match(
	in expression,
	in template instance
) return boolean`,
		Documentation: `## (TTCN-3)
The __match__ operation returns a __boolean__ value. It matches an expression, which shall denote a value or a field of a value
against a template instance. Types of the expression and the template instance shall be compatible (see clause 6.3). The
return value of the match operation indicates whether the expression matches the specified template instance. In the
special case, matching a non-optional value expression (e.g. a value variable or non-optional field of a value) with a
template instance that matches an omitted field (i.e. one of the matching mechanisms Omit, AnyValueOrNone,
IfPresent) shall be allowed and shall be treated as if the value expression were an optional field. Thus, matching a value
expression against a template instance which evaluates to the omit matching mechanism shall return false.`,
		NrOfParameters: 2,
		TextFormat:     protocol.SnippetTextFormat},
	{
		Label:      "setverdict(...)",
		InsertText: "setverdict(${1:verdict})$0",
		Signature: `function setverdict(
	in expression
	{, in charstring|templateInstance reason})`,
		Documentation: `## (TTCN-3)
The value of the local verdict is changed with the __setverdict__ operation.
* The first parameter is either an expression or a literal providing one of the values:
__pass__, __fail__, __inconc__ or __none__
* The optional parameters allow to provide information that explain the reasons for assigning the verdict. This
information is composed to a string and stored in an implicit __charstring__ variable. On termination of the test
component, the actual local verdict is logged together with the implicit __charstring__ variable. Since the optional
parameters can be seen as log information, the same rules and restrictions as for the parameters of the __log__ statement
apply`,
		NrOfParameters: 2,
		TextFormat:     protocol.SnippetTextFormat}}
