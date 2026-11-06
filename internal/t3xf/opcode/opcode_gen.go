// Code generated from opcode.tmpl.go and by 'go generate'; DO NOT EDIT.

// Package opcode defines the opcodes used in T3XF.
package opcode

const (
	REF       Opcode = 0b00_0000000000000000000000000000_00
	GOTO      Opcode = 0b00_0000000000000000000000000000_01
	LINE      Opcode = 0b00_0000000000000000000000000000_10
	instrMask Opcode = 0b00_0000000000000000000000000000_11

	reserved4 Opcode = 0b01_0000000000000000000000000000_00
	CALL      Opcode = 0b01_0000000000000000000000000000_01
	reseved6  Opcode = 0b01_0000000000000000000000000000_10
	reseved7  Opcode = 0b01_0000000000000000000000000000_11

	FROZEN_REF Opcode = 0b10_0000000000000000000000000000_00
	ISCAN      Opcode = 0b10_0000000000000000000000000000_01
	reseved10  Opcode = 0b10_0000000000000000000000000000_10
	reseved11  Opcode = 0b10_0000000000000000000000000000_11

	reseved12 Opcode = 0b11_0000000000000000000000000000_00
	reseved13 Opcode = 0b11_0000000000000000000000000000_01
	reseved14 Opcode = 0b11_0000000000000000000000000000_10
	reseved15 Opcode = 0b11_0000000000000000000000000000_11

	// Request the action described in str from the user.
	//
	// 	[str] ACTION []
	//
	ACTION Opcode = 0x1003

	// Activate an altstep and return a default object. Block blk
	// shall contain an altstep call, including any required
	// parameters.
	//
	// 	[blk] ACTIVATE [default]
	//
	ACTIVATE Opcode = 0x0d03

	// Evaluate a + b, pushing the result.
	//
	// 	[a b] ADD [res]
	//
	ADD Opcode = 0x0413

	// When a module defines a type named ‘address’ then this
	// definition is returned from the address operation.
	//
	// When a module fails to define an address type then this
	// operation returns a built-in opaque address type object.
	//
	// Each defined module has its own unique definition of address.
	//
	// 	[] ADDRESS [obj]
	//
	ADDRESS Opcode = 0x0ea3

	// Test the state of component comp for the alive condition. Push
	// the boolean result onto the stack.
	//
	// 	[comp] ALIVE [res]
	//
	ALIVE Opcode = 0x1103

	// Test the state of all parallel test components for any that
	// are alive. Push the boolean result onto the stack.
	//
	// 	[] ALIVE1 [res]
	//
	ALIVE1 Opcode = 0x1113

	// Test the state of all parallel test components checking that
	// all are alive. Push the boolean result onto the stack.
	//
	// 	[] ALIVEA [res]
	//
	ALIVEA Opcode = 0x1123

	//
	// 	[obj] ALLFROM [res]
	//
	ALLFROM Opcode = 0x06a3

	//
	// 	[obj] ALLFROMP [res]
	//
	ALLFROMP Opcode = 0x06b3

	// Evaluate the contents of blk until one of the contained
	// alternatives succeeds.
	//
	// An empty block is allowed: activated defaults will still be
	// included in the operation.
	//
	// If the successful alternative executes a repeat statement then
	// the whole block execution will be re-evaluated.
	//
	// 	[blk] ALT []
	//
	ALT Opcode = 0x0383

	// Same as `alt` with disabled defaults (`@nodefault`)
	//
	// 	[blk] ALTND []
	//
	ALTND Opcode = 0x2023

	// Define an altstep object from a block pblk containing its
	// parameters, bblk containing its behaviour and a name, and then
	// attach it to the module object owning the current block.
	//
	// 	[pblk bblk name] ALTSTEP []
	//
	ALTSTEP Opcode = 0x0803

	// Define a bound altstep object from a component type comp, a
	// block pblk containing its parameters, bblk containing its
	// behaviour and a name, then attach it to the module object
	// owning the current block.
	//
	// 	[comp pblk bblk name] ALTSTEPB []
	//
	ALTSTEPB Opcode = 0x0813

	// Define a bound altstep object from a component type comp, a
	// block pblk containing its parameters, bblk containing its
	// behaviour and a name, attach the attributes from block ablk,
	// and then attach it to the module object owning the current
	// block.
	//
	// 	[comp ablk pblk bblk name] ALTSTEPBW []
	//
	ALTSTEPBW Opcode = 0x0833

	// Define an altstep object from a block pblk containing its
	// parameters, bblk containing its behaviour and a name, attach
	// the attributes from block ablk, and then attach it to the
	// module object owning the current block.
	//
	// 	[ablk pblk bblk name] ALTSTEPW []
	//
	ALTSTEPW Opcode = 0x0823

	// With a boolean first argument, perform the logical and
	// operation and return the boolean result. The second argument
	// may be either another boolean or a block. If the first
	// argument is false then the second argument shall not be
	// evaluated.
	//
	// With bitstring, hexstring or octetstring inputs, perform the
	// bitwise and operation and return the result. The result type
	// will be of the same base type as the inputs.
	//
	// 	[obj obj] AND [res]
	//
	AND Opcode = 0x0483

	// Push a special any-value literal on the stack, corresponding
	// to the TTCN-3 ‘?’ literal.
	//
	// 	[] ANY [lit]
	//
	ANY Opcode = 0x0603

	// Push a special any-value-or-none literal on the stack,
	// corresponding to the TTCN-3 ‘*’ literal.
	//
	// 	[] ANYN [lit]
	//
	ANYN Opcode = 0x0613

	//
	// 	[closure] APPLY []
	//
	APPLY Opcode = 0x0193

	// Create an array type object from an element type type and a
	// dimension dim. Push the resulting type object on the stack.
	// The array dimension shall be either a positive integer or a
	// range of integers (positive, no infinities.)
	//
	// 	[type dim] ARRAY [obj]
	//
	ARRAY Opcode = 0x0ee3

	// Assign the value of b to a.
	//
	// The types of a and b must be compatible.
	//
	// 	[a b] ASSIGN []
	//
	ASSIGN Opcode = 0x0583

	// Assign the value of b to a. Leave object a on the stack.
	//
	// The types of a and b must be compatible.
	//
	// 	[a b] ASSIGND [a]
	//
	ASSIGND Opcode = 0x0593

	//
	// 	[obj] AT_DEFAULT []
	//
	AT_DEFAULT Opcode = 0x2013

	// Convert bitstring bit into a hexstring and push it onto the
	// stack.
	//
	// 	[bit] BIT2HEX [res]
	//
	BIT2HEX Opcode = 0x18c3

	// Convert bitstring bit into an integer and push it onto the
	// stack.
	//
	// 	[bit] BIT2INT [res]
	//
	BIT2INT Opcode = 0x18b3

	// Convert bitstring bit into an octetstring and push it onto the
	// stack.
	//
	// 	[bit] BIT2OCT [res]
	//
	BIT2OCT Opcode = 0x18d3

	// Convert bitstring bit into a charstring and push it onto the
	// stack.
	//
	// 	[bit] BIT2STR [res]
	//
	BIT2STR Opcode = 0x18e3

	// Push a bitstring literal onto the stack. This is a
	// variable-length instruction with the details of the literal
	// bitstring following the instruction itself: first, a length
	// field, and then the bytes of the bitstring.
	//
	// The length field is encoded with the same byte order as has
	// been selected for instructions. The length contains the number
	// of bits.
	//
	// The string itself is always encoded with the first bit of the
	// string at the lowest file offset – the natural ordering for
	// strings – and in the most-significant bit of that byte.
	//
	// Padding shall be placed at the end of the string bytes to
	// restore the correct alignment for following instructions.
	//
	// 	[] BITS [lit]
	//
	BITS Opcode = 0x0113

	// Push the built-in bitstring type object.
	//
	// 	[] BITSTRING [obj]
	//
	BITSTRING Opcode = 0x0e03

	// Switch from scanning back to execution mode pushing a block
	// object onto the stack.
	//
	// 	[] BLOCK [blk]
	//
	BLOCK Opcode = 0x0083

	// Push the built-in boolean type object.
	//
	// 	[] BOOLEAN [obj]
	//
	BOOLEAN Opcode = 0x0e13

	// Leave the enclosing loop construct.
	//
	// 	[] BREAK []
	//
	BREAK Opcode = 0x03c3

	// Concatenates strings a and b and returns the result. Objects a
	// and b must both be strings of the same base type.
	//
	// 	[a b] CAT [res]
	//
	CAT Opcode = 0x0563

	// Convert single character string char into an integer and push
	// it onto the stack.
	//
	// 	[char] CHAR2INT [res]
	//
	CHAR2INT Opcode = 0x1883

	// Convert charstring char into an octetstring and push it onto
	// the stack.
	//
	// 	[char] CHAR2OCT [res]
	//
	CHAR2OCT Opcode = 0x1893

	// Push the built-in charstring type object.
	//
	// 	[] CHARSTRING [obj]
	//
	CHARSTRING Opcode = 0x0e23

	// Push the built-in universal charstring type object.
	//
	// 	[] CHARSTRINGU [obj]
	//
	CHARSTRINGU Opcode = 0x0e33

	// Create a pure check condition on port port. Block mblk shall
	// contain code to perform the matching of input. When the
	// condition is true (matched) then the enclosing alt will
	// execute block ablk to perform variable assignments.
	//
	// 	[mblk ablk port] CHECK []
	//
	CHECK Opcode = 0x14a3

	// Any port check
	//
	// 	[match assign] CHECK1 []
	//
	CHECK1 Opcode = 0x14b3

	//
	// 	[port string] CHECKSTATE [bool]
	//
	CHECKSTATE Opcode = 0x1783

	// All port checkstate
	//
	// 	[string] CHECKSTATEAL [bool]
	//
	CHECKSTATEAL Opcode = 0x1793

	// Any port checkstate
	//
	// 	[string] CHECKSTATEAN [bool]
	//
	CHECKSTATEAN Opcode = 0x17a3

	// Clear all contents from the input queue of port p.
	//
	// 	[p] CLEAR []
	//
	CLEAR Opcode = 0x1383

	// Clear all contents from the input queues of all ports on the
	// current component.
	//
	// 	[] CLEARA []
	//
	CLEARA Opcode = 0x1393

	// Clone obj and push back clone.
	//
	// 	[obj] CLONE [res]
	//
	CLONE Opcode = 0x0403

	// Wrap obj as closure
	//
	// 	[ref] CLOSURE [closure]
	//
	CLOSURE Opcode = 0x11f3

	//
	// 	[type_ref] CLOSURETYPE []
	//
	CLOSURETYPE Opcode = 0x0e93

	// Build a collection from a marked listed of objects,
	// potentially empty. The objects and the mark are removed from
	// the stack and replaced with a collection object.
	//
	// 	[mark object1 .. objectn] COLLECT [col]
	//
	COLLECT Opcode = 0x0093

	// Make a complement template literal from the input value list
	// vlist. Push the resulting object on the stack.
	//
	// 	[vlist] COMPLEMENT [res]
	//
	COMPLEMENT Opcode = 0x0623

	// Construct a component type object with initialisation from
	// block iblk and push on to the stack.
	//
	// 	[iblk] COMPONENT [ctype]
	//
	COMPONENT Opcode = 0x0f23

	// Construct a component type object with initialisation from
	// block iblk, extending components listed in collection coll and
	// push on to the stack.
	//
	// 	[coll iblk] COMPONENTX [ctype]
	//
	COMPONENTX Opcode = 0x0f33

	// Connect ports p1 and p2.
	//
	// 	[p1 p2] CONNECT []
	//
	CONNECT Opcode = 0x1083

	// Create a constant data definition from a block vblk containing
	// the statements of its value, a type type and a name.
	//
	// 	[vblk type name] CONST []
	//
	CONST Opcode = 0x0843

	// Create a constant data definition from a block vblk containing
	// the statements of its value, a type type and a name, applying
	// attributes from block ablk.
	//
	// 	[ablk vblk type name] CONSTW []
	//
	CONSTW Opcode = 0x0853

	// Repeat the enclosing loop construct.
	//
	// 	[] CONTINUE []
	//
	CONTINUE Opcode = 0x03d3

	// Create a control part definition from a block blk containing
	// the statements of its behaviour.
	//
	// 	[blk] CONTROL []
	//
	CONTROL Opcode = 0x0863

	// A control part definition with attributes attached.
	//
	// 	[blk] CONTROLW []
	//
	CONTROLW Opcode = 0x0873

	// Create an instance of a component of type ctype and push the
	// result onto the stack.
	//
	// 	[ctype] CREATE [cinst]
	//
	CREATE Opcode = 0x1183

	// Create an alive-type instance of a component of type ctype and
	// push the result onto the stack.
	//
	// 	[ctype] CREATEA [cinst]
	//
	CREATEA Opcode = 0x1193

	// Create a named alive-type instance of a component of type
	// ctype and push the result onto the stack. The component
	// instance name is taken from the name parameter.
	//
	// 	[name ctype] CREATEAN [cinst]
	//
	CREATEAN Opcode = 0x11b3

	// Create a named instance of a component of type ctype and push
	// the result onto the stack. The component instance name is
	// taken from the name parameter.
	//
	// 	[name ctype] CREATEN [cinst]
	//
	CREATEN Opcode = 0x11a3

	// Deactivate default altstep default.
	//
	// 	[default] DEACTIVATE []
	//
	DEACTIVATE Opcode = 0x0d23

	// Deactivate all altsteps activated on the current component.
	//
	// 	[] DEACTIVATEA []
	//
	DEACTIVATEA Opcode = 0x0d33

	// The encoded value enc is decoded and the decoded content
	// will be matched against the provided decTemplate returning
	// a boolean value res on the stack
	//
	// 	[enc decTemplate] DECMATCH [res]
	//
	DECMATCH Opcode = 0x2033

	// Decode encoded bitstring enc placing the result in dec and
	// returning integer res holding the outcome of the operation: 0
	// indicates success, 1 indicates unspecified failure and 2
	// indicates insufficient data (retry after adding bits to enc.)
	//
	// 	[enc dec] DECVALUE [res]
	//
	DECVALUE Opcode = 0x1a83

	//
	// 	[from what] DEF [ref]
	//
	DEF Opcode = 0x05b3

	// Push the built-in default type object.
	//
	// 	[] DEFAULT [ref]
	//
	DEFAULT Opcode = 0x0ec3

	// Break the connection between port p1 and port p2.
	//
	// 	[p1 p2] DISCONNECT []
	//
	DISCONNECT Opcode = 0x1093

	// Break all connections from component comp.
	//
	// 	[comp] DISCONNECTA []
	//
	DISCONNECTA Opcode = 0x10a3

	// Break all connections on all components.
	//
	// 	[] DISCONNECTAA []
	//
	DISCONNECTAA Opcode = 0x10b3

	// Evaluate a / b, pushing the result on the stack.
	//
	// The types of a and b must be compatible. The returned object
	// has the base type of the input parameters and is constant.
	//
	// 	[a b] DIV [res]
	//
	DIV Opcode = 0x0453

	// Test the state of component comp for the done condition,
	// contributing this to an enclosing alt or interleave statement.
	//
	// 	[comp] DONE []
	//
	DONE Opcode = 0x1143

	// Test the state of all parallel test components for any that
	// are done, contributing this to an enclosing alt or interleave
	// statement.
	//
	// 	[] DONE1 []
	//
	DONE1 Opcode = 0x1153

	// Test the state of all parallel test components checking that
	// all are done, contributing this to an enclosing alt or
	// interleave statement.
	//
	// 	[] DONEA []
	//
	DONEA Opcode = 0x1163

	// Implements the TTCN-3 do/while statement using two block
	// arguments.
	//
	// Block body is executed and then loop condition block cond is
	// evaluated. If the condition returns true then the process is
	// repeated.
	//
	// Execution terminates when the loop condition returns a false
	// value.
	//
	// 	[body cond] DOWHILE []
	//
	DOWHILE Opcode = 0x0363

	// Remove the item at the top of the stack and discard it.
	//
	// 	[obj] DROP []
	//
	DROP Opcode = 0x00a3

	// Duplicate the item at the top of the stack.
	//
	// 	[obj] DUP [obj obj]
	//
	DUP Opcode = 0x00b3

	// Create an alternative for an enclosing alt operation that is
	// immediately successful. Block blk contains the code to execute
	// if this alternative is chosen by the containing alt.
	//
	// 	[blk] ELSE []
	//
	ELSE Opcode = 0x03a3

	// Attach the encode attribute val to the object owning the
	// current block.
	//
	// 	[val] ENCODE []
	//
	ENCODE Opcode = 0x0c43

	// Attach the overriding encode attribute val to the object
	// owning the current block.
	//
	// 	[val] ENCODEO []
	//
	ENCODEO Opcode = 0x0c63

	// Encode value value returning bitstring res holding the encoded
	// result.
	//
	// 	[value] ENCVALUE [res]
	//
	ENCVALUE Opcode = 0x1a73

	// Convert enumerated enum into an integer and push it onto the
	// stack.
	//
	// 	[enum] ENUM2INT [res]
	//
	ENUM2INT Opcode = 0x1a53

	// Build an enumerated object from the terms described in block
	// blk. Push the resulting object onto the stack.
	//
	// 	[blk] ENUMERATED [obj]
	//
	ENUMERATED Opcode = 0x0f03

	// Evaluate a == b, pushing the boolean result.
	//
	// 	[a b] EQ [res]
	//
	EQ Opcode = 0x0503

	// Push a special literal on the stack corresponding to the error
	// term of verdicttype.
	//
	// 	[] ERROR [lit]
	//
	ERROR Opcode = 0x02e3

	// Execute block blk. This is an optimisation of if(true), giving
	// a space saving of two operations with considerably better
	// performance.
	//
	// The motivation for this operation is the translation of a
	// select statement containing only a single else statement.
	// Placing the block inline is not possible because of the issue
	// of variable scoping.
	//
	// 	[blk] EXEC []
	//
	EXEC Opcode = 0x0373

	// Execute testcase and return its verdict. The input parameter
	// is a block blk containing any actual parameters to the
	// testcase, followed by the invocation of the testcase itself.
	//
	// 	[blk] EXECUTE [verdict]
	//
	EXECUTE Opcode = 0x1063

	// Same as execute, but with logging disabled.
	//
	// 	[blk] EXECUTED [verdict]
	//
	EXECUTED Opcode = 0x11d3

	// Execute testcase and return its verdict. The input parameter
	// is a block blk containing any actual parameters to be supplied
	// to the testcase, followed by the invocation of the testcase
	// itself. Time limit limit shall be applied to the testcase
	// execution.
	//
	// 	[limit blk] EXECUTEL [verdict]
	//
	EXECUTEL Opcode = 0x1073

	// Same as executel, but with logging disabled.
	//
	// 	[limit blk] EXECUTELD [verdict]
	//
	EXECUTELD Opcode = 0x11c3

	// Attach the extension attribute val to the object owning the
	// current block.
	//
	// 	[val] EXTENSION []
	//
	EXTENSION Opcode = 0x0c83

	// Attach the overriding extension attribute val to the object
	// owning the current block.
	//
	// 	[val] EXTENSIONO []
	//
	EXTENSIONO Opcode = 0x0ca3

	// Push a special literal on the stack corresponding to the fail
	// term of verdicttype.
	//
	// 	[] FAIL [lit]
	//
	FAIL Opcode = 0x02d3

	// Push a special false literal onto the stack.
	//
	// 	[] FALSE [lit]
	//
	FALSE Opcode = 0x0283

	// Build a field description from type and name and attach it to
	// the structured type object owning the current block.
	//
	// 	[type name] FIELD []
	//
	FIELD Opcode = 0x0723

	// Build an optional field description from type and name and
	// attach it to the structured type object owning the current
	// block.
	//
	// 	[type name] FIELDO []
	//
	FIELDO Opcode = 0x0733

	// Push the built-in float type object.
	//
	// 	[] FLOAT [obj]
	//
	FLOAT Opcode = 0x0e43

	// Convert float flt into an integer and push it onto the stack.
	//
	// 	[flt] FLOAT2INT [res]
	//
	FLOAT2INT Opcode = 0x1873

	// Implements the TTCN-3 for statement using four block
	// arguments.
	//
	// First, block init is executed to establish initial values.
	//
	// Block cond is evaluated to test the loop condition. If the
	// block yields a true value then block body is executed followed
	// by block incr to change the loop condition before looping to
	// evaluate the loop condition again.
	//
	// Execution terminates when the loop condition returns a false
	// value.
	//
	// Variables created within block init are visible within blocks
	// cond, incr and body. Such variables will be destroyed when the
	// operation terminates.
	//
	// 	[init cond body incr] FOR []
	//
	FOR Opcode = 0x0343

	// Implements the TTCN-3 .from operation applied to mapv of
	// type: `type map from A to B MapT`. A setofFromType is provided
	// for creation of the return value.
	// Return value is of type `set of A` containing all keys
	// from the map.
	//
	// 	[mapv setofFromType] FROM [res]
	//
	FROM Opcode = 0x06c3

	// Push a float literal onto the stack, with the value coded as
	// an ASCII string to allow for (nearly) arbitrary size and
	// precision. This is a variable-length instruction with the
	// details of the literal string following the instruction
	// itself: first, a length field, and then the bytes of the
	// string.
	//
	// The length field is encoded with the same byte order as has
	// been selected for instructions. The string itself is always
	// encoded with the first byte of the string at the lowest file
	// offset – the natural ordering for strings.
	//
	// Padding shall be placed at the end of the string bytes to
	// restore the correct alignment for following instructions.
	//
	// 	[] FSTR [lit]
	//
	FSTR Opcode = 0x0173

	// Define a function object from a block pblk containing its
	// parameters, bblk containing its behaviour and a name, and then
	// attach it to the module object owning the current block.
	//
	// 	[pblk bblk name] FUNCTION []
	//
	FUNCTION Opcode = 0x0903

	// Define a bound function object from a component type comp, a
	// block pblk containing its parameters, bblk containing its
	// behaviour and a name, then attach it to the module object
	// owning the current block.
	//
	// 	[comp pblk bblk name] FUNCTIONB []
	//
	FUNCTIONB Opcode = 0x0913

	// Define a value-returning function object from return type, a
	// block pblk containing its parameters, bblk containing its
	// behaviour and a name, and then attach it to the module object
	// owning the current block.
	//
	// 	[type pblk bblk name] FUNCTIONV []
	//
	FUNCTIONV Opcode = 0x0943

	// Define a value-returning bound function object from return
	// type, a component type comp, a block pblk containing its
	// parameters, bblk containing its behaviour and a name, then
	// attach it to the module object owning the current block.
	//
	// 	[type comp pblk bblk name] FUNCTIONVB []
	//
	FUNCTIONVB Opcode = 0x0953

	// Define an external function object from a block pblk
	// containing its parameters and a name, and then attach it to
	// the module object owning the current block.
	//
	// 	[pblk name] FUNCTIONX []
	//
	FUNCTIONX Opcode = 0x0983

	// Define a value-returning external function object from return
	// type, a block pblk containing its parameters and a name, and
	// then attach it to the module object owning the current block.
	//
	// 	[type pblk name] FUNCTIONXV []
	//
	FUNCTIONXV Opcode = 0x09a3

	// Define a value-returning external function object from return
	// type, a block pblk containing its parameters and a name,
	// attach the attributes from block ablk, and then attach it to
	// the module object owning the current block.
	//
	// 	[type ablk pblk name] FUNCTIONXVW []
	//
	FUNCTIONXVW Opcode = 0x09b3

	// Define an external function object from a block pblk
	// containing its parameters and a name, attach the attributes
	// from block ablk, and then attach it to the module object
	// owning the current block.
	//
	// 	[ablk pblk name] FUNCTIONXW []
	//
	FUNCTIONXW Opcode = 0x0993

	// Evaluate a >= b, pushing the boolean result.
	//
	// 	[a b] GE [res]
	//
	GE Opcode = 0x0513

	// When object obj is of type record, record-of, set, set-of,
	// union, array or string, fetch a reference to the element
	// indexed by idx, where idx is an unsigned integer value.
	//
	// When object obj is of type component fetch a reference to the
	// field name.
	//
	// 	[obj field] GET [obj]
	//
	GET Opcode = 0x0573

	// Get the local verdict of the current component and push it on
	// the stack. Use within the control part is a runtime error.
	//
	// 	[] GETVERDICT [lver]
	//
	GETVERDICT Opcode = 0x1043

	// Evaluate a > b, pushing the boolean result.
	//
	// 	[a b] GT [res]
	//
	GT Opcode = 0x0523

	// Instruct port p to accept no further input.
	//
	// 	[p] HALT []
	//
	HALT Opcode = 0x13a3

	// Instruct all ports on the current component to accept no
	// further input.
	//
	// 	[] HALTA []
	//
	HALTA Opcode = 0x13b3

	// Convert hexstring hex into a bitstring and push it onto the
	// stack.
	//
	// 	[hex] HEX2BIT [res]
	//
	HEX2BIT Opcode = 0x1903

	// Convert hexstring hex into an integer and push it onto the
	// stack.
	//
	// 	[hex] HEX2INT [res]
	//
	HEX2INT Opcode = 0x18f3

	// Convert hexstring hex into an octetstring and push it onto the
	// stack.
	//
	// 	[hex] HEX2OCT [res]
	//
	HEX2OCT Opcode = 0x1913

	// Convert hexstring hex into a charstring and push it onto the
	// stack.
	//
	// 	[hex] HEX2STR [res]
	//
	HEX2STR Opcode = 0x1923

	// Push the built-in hexstring type object.
	//
	// 	[] HEXSTRING [obj]
	//
	HEXSTRING Opcode = 0x0e53

	// Same behaviour as 'def'.
	//
	// 	[from] IDEF [ref]
	//
	IDEF Opcode = 0x05c3

	// Push a float literal onto the stack. The encoding of the
	// floating-point value follows the instruction word and shall be
	// in IEEE754 double-precision format.
	//
	// IEEE754 double precision has a fixed size of 64-bits. Padding
	// will be required if the current instruction width is greater
	// than 64-bit.
	//
	// The eight bytes of the representation must be swapped if
	// swapping is enabled.
	//
	// 	[] IEEE754DP [lit]
	//
	IEEE754DP Opcode = 0x0153

	// Execute block blk if cond is true.
	//
	// 	[cond blk] IF []
	//
	IF Opcode = 0x0303

	// Execute block tblk if cond is true else execute block fblk.
	//
	// 	[cond tblk fblk] IFELSE []
	//
	IFELSE Opcode = 0x0313

	//
	// 	[type name] IFIELD []
	//
	IFIELD Opcode = 0x05d3

	// Modify obj to allow it to match optional fields in record or
	// set objects. Leave the object on the stack.
	//
	// 	[obj] IFPRESENT [obj]
	//
	IFPRESENT Opcode = 0x0633

	// When object obj is of type record, record-of, set, set-of,
	// union, array or string, fetch a reference to the element
	// indexed by x, where x is an unsigned integer value in the
	// range 0 to 32767.
	//
	// This is the performance-optimised version of the get operation
	// above.
	//
	// 	[obj] IGET [obj]
	//
	IGET Opcode = 0x05a3

	// Bind a type and name into an input value parameter object and
	// then attach this to the owning function (altstep, testcase)
	// object.
	//
	// 	[type name] IN []
	//
	IN Opcode = 0x0743

	// Push a special literal on the stack corresponding to the
	// inconc term of verdicttype.
	//
	// 	[] INCONC [lit]
	//
	INCONC Opcode = 0x02c3

	// Push a special -infinity literal onto the stack.
	//
	// 	[] INFINITYN [lit]
	//
	INFINITYN Opcode = 0x0263

	// Push a special +infinity literal onto the stack.
	//
	// 	[] INFINITYP [lit]
	//
	INFINITYP Opcode = 0x0273

	// Bind a type and name into an input/output value parameter
	// object and then attach this to the owning function (altstep,
	// testcase) object.
	//
	// 	[type name] INOUT []
	//
	INOUT Opcode = 0x0753

	// Convert integer int into a bitstring and push it onto the
	// stack. len supplies a minimum length for the resulting string.
	//
	// 	[int len] INT2BIT [res]
	//
	INT2BIT Opcode = 0x1823

	// Convert integer int into a single character string and push it
	// onto the stack.
	//
	// 	[int] INT2CHAR [res]
	//
	INT2CHAR Opcode = 0x1803

	//
	// 	[a] INT2ENUM [res]
	//
	INT2ENUM Opcode = 0x1aa3

	// Convert integer int into a float and push it onto the stack.
	//
	// 	[int] INT2FLOAT [res]
	//
	INT2FLOAT Opcode = 0x1863

	// Convert integer int into a hexstring and push it onto the
	// stack. len supplies a minimum length for the resulting string.
	//
	// 	[int len] INT2HEX [res]
	//
	INT2HEX Opcode = 0x1833

	// Convert integer int into an octetstring and push it onto the
	// stack. len supplies a minimum length for the resulting string.
	//
	// 	[int len] INT2OCT [res]
	//
	INT2OCT Opcode = 0x1843

	// Convert integer int into a charstring and push it onto the
	// stack.
	//
	// 	[int] INT2STR [res]
	//
	INT2STR Opcode = 0x1853

	// Push the built-in integer type object.
	//
	// 	[] INTEGER [obj]
	//
	INTEGER Opcode = 0x0e63

	// Evaluate the contents of blk until all of the contained
	// alternatives succeed.
	//
	// An empty block is allowed: activated defaults will still be
	// included in the operation.
	//
	// Actions attached to successful alternatives may extend the set
	// of alternatives to complete.
	//
	// 	[blk] INTERLEAVE []
	//
	INTERLEAVE Opcode = 0x0393

	// Define an interleavestep object from a block pblk containing its
	// parameters, bblk containing its behaviour and a name, and then
	// attach it to the module object owning the current block.
	//
	// 	[pblk bblk name] INTERLEAVESTEP []
	//
	INTERLEAVESTEP Opcode = 0x2043

	// Define a bound interleavestep object from a component type comp, a
	// block pblk containing its parameters, bblk containing its
	// behaviour and a name, then attach it to the module object
	// owning the current block.
	//
	// 	[comp pblk bblk name] INTERLEAVESTEPB []
	//
	INTERLEAVESTEPB Opcode = 0x2053

	// Define a bound interleavestep object from a component type comp, a
	// block pblk containing its parameters, bblk containing its
	// behaviour and a name, attach the attributes from block ablk,
	// and then attach it to the module object owning the current
	// block.
	//
	// 	[comp ablk pblk bblk name] INTERLEAVESTEPBW []
	//
	INTERLEAVESTEPBW Opcode = 0x2073

	// Define an interleavestep object from a block pblk containing its
	// parameters, bblk containing its behaviour and a name, attach
	// the attributes from block ablk, and then attach it to the
	// module object owning the current block.
	//
	// 	[ablk pblk bblk name] INTERLEAVESTEPW []
	//
	INTERLEAVESTEPW Opcode = 0x2063

	// Stack operations might be dynamic.
	//
	// 	[] ISBOUND []
	//
	ISBOUND Opcode = 0x1ad3

	// Tests the object obj to determine if it is the selected field
	// of a union and pushes the boolean result. obj must reference a
	// field of a union.
	//
	// 	[obj] ISCHOSEN [res]
	//
	ISCHOSEN Opcode = 0x19f3

	// Tests the presence of object obj and pushes the boolean
	// result. obj must reference an optional field of a record or
	// set.
	//
	// 	[obj] ISPRESENT [res]
	//
	ISPRESENT Opcode = 0x19e3

	// Push an integer literal onto the stack, with the integer coded
	// as an ASCII string to allow for (nearly) arbitrary size. This
	// is a variable-length instruction with the details of the
	// literal string following the instruction itself: first, a
	// length field, and then the bytes of the string.
	//
	// The length field is encoded with the same byte order as has
	// been selected for instructions. The string itself is always
	// encoded with the first byte of the string at the lowest file
	// offset – the natural ordering for strings.
	//
	// Padding shall be placed at the end of the string bytes to
	// restore the correct alignment for following instructions.
	//
	// 	[] ISTR [lit]
	//
	ISTR Opcode = 0x0163

	// Tests the object obj to determine is it is concrete value
	// (defined, not a template) and pushes the boolean result.
	//
	// 	[obj] ISVALUE [res]
	//
	ISVALUE Opcode = 0x1a63

	// Jumps over next instruction if the boolean on the stack is not true.
	//
	// 	[cond] JNT []
	//
	JNT Opcode = 0x2593

	// Jumps over next instruction if the boolean on the stack is true.
	//
	// 	[cond] JT []
	//
	JT Opcode = 0x2583

	// Stop behaviour on component comp and move it to the killed
	// state.
	//
	// 	[comp] KILL []
	//
	KILL Opcode = 0x13c3

	// Stop behaviour on all parallel test components, moving them to
	// the killed state.
	//
	// 	[] KILLA []
	//
	KILLA Opcode = 0x13d3

	// Test the state of component comp for the killed condition,
	// contributing this to an enclosing alt or interleave statement.
	//
	// 	[comp] KILLED []
	//
	KILLED Opcode = 0x1243

	// Test the state of all parallel test components for any that
	// are killed, contributing this to an enclosing alt or
	// interleave statement.
	//
	// 	[] KILLED1 []
	//
	KILLED1 Opcode = 0x1253

	// Test the state of all parallel test components checking that
	// all are killed, contributing this to an enclosing alt or
	// interleave statement.
	//
	// 	[] KILLEDA []
	//
	KILLEDA Opcode = 0x1263

	// Evaluate a <= b, pushing the boolean result.
	//
	// 	[a b] LE [res]
	//
	LE Opcode = 0x0533

	// Create a length restriction with length len and apply to obj,
	// leaving the result on the stack. len may be a range
	// constructed with the range operation.
	//
	// 	[obj len] LENGTH [obj]
	//
	LENGTH Opcode = 0x0643

	// Takes the length of the string-type object str and pushes it
	// onto the stack.
	//
	// 	[str] LENGTHOF [len]
	//
	LENGTHOF Opcode = 0x19b3

	// Loads a local object indexed by x, where x is an unsigned
	// integer value in the range 0 to 32767.
	//
	// 	[] LOAD [obj]
	//
	LOAD Opcode = 0x0053

	// Log the items in collection coll or log the single object obj.
	//
	// 	[obj] LOG []
	//
	LOG Opcode = 0x1013

	// Evaluate a < b, pushing the boolean result.
	//
	// 	[a b] LT [res]
	//
	LT Opcode = 0x0543

	// Map port p1 to port p2.
	//
	// 	[p1 p2] MAP []
	//
	MAP Opcode = 0x10c3

	// Construct a map type object with key typek and value typev
	// and push on to the stack.
	//
	// 	[typek typev] MAPT [obj]
	//
	MAPT Opcode = 0x0fc3

	// Push a mark object onto the stack.
	//
	// 	[] MARK [mark]
	//
	MARK Opcode = 0x00c3

	// Perform template matching of expression expr against template
	// tmpl returning a boolean result on the stack.
	//
	// 	[expr tmpl] MATCH [res]
	//
	MATCH Opcode = 0x1023

	// Evaluate a mod b, pushing the result on the stack. The TTCN-3
	// definition of mod applies.
	//
	// The types of a and b must be compatible. The returned object
	// has the base type of the input parameters and is constant.
	//
	// 	[a b] MOD [res]
	//
	MOD Opcode = 0x0463

	// Define a module object and attach it to the source object
	// owning the current block. Parameters are a block dblk of
	// sub-definitions and the group name.
	//
	// 	[bblk name] MODULE []
	//
	MODULE Opcode = 0x08e3

	// Define a module object with attributes.
	//
	// 	[ablk bblk name] MODULEW []
	//
	MODULEW Opcode = 0x08f3

	// Move b to a
	//
	// 	[b a] MOVE []
	//
	MOVE Opcode = 0x05e3

	// Create a module parameter definition from its type and a name.
	//
	// 	[type name] MPAR []
	//
	MPAR Opcode = 0x0a03

	// Create a module parameter definition from a block vblk
	// containing the statements of its default value, its type and a
	// name.
	//
	// 	[vblk type name] MPARD []
	//
	MPARD Opcode = 0x0a13

	// Fetch a reference to the current MTC and push it on the stack.
	//
	// 	[] MTC [obj]
	//
	MTC Opcode = 0x0213

	// Evaluate a * b, pushing the result on the stack.
	//
	// The types of a and b must be compatible. The returned object
	// has the base type of the input parameters and is constant.
	//
	// 	[a b] MUL [res]
	//
	MUL Opcode = 0x0443

	// Push an object name literal onto the stack, coded as an ASCII
	// string. This is a variable-length instruction with the details
	// of the name following the instruction itself: first, a length
	// field, and then the bytes of the string.
	//
	// The length field is encoded with the same byte order as has
	// been selected for instructions. The string itself is always
	// encoded with the first byte of the string at the lowest file
	// offset – the natural ordering for strings.
	//
	// Padding shall be placed at the end of the string bytes to
	// restore the correct alignment for following instructions.
	//
	// 	[] NAME [lit]
	//
	NAME Opcode = 0x0183

	// Push a signed integer literal onto the stack. This is a
	// variable-length instruction with the value of the literal
	// integer following the instruction itself as a native long.
	//
	// The literal integer value is encoded at the same width and
	// byte ordering as is currently selected for instructions: is
	// must be swapped when swapping is enabled.
	//
	// If the currently selected instruction width is too small for
	// the encoding of a particular value then the istr literal
	// operation shall be used instead.
	//
	// 	[] NATLONG [lit]
	//
	NATLONG Opcode = 0x0143

	// Evaluate a != b, pushing the boolean result.
	//
	// 	[a b] NE [res]
	//
	NE Opcode = 0x0553

	// Evaluate -a (unary - operator) pushing the result.
	//
	// 	[a] NEG [res]
	//
	NEG Opcode = 0x0423

	// Encoding of 9 nibbles, showing nibble endian-ness:
	// ‘BADC0FFEE’H
	//
	// Note: 9th nibble in the upper half of most significant byte
	//
	// 0000 0123
	// 0000 0009
	// BADC 0FFE
	// E000 0000
	//
	// 	[] NIBBLES [lit]
	//
	NIBBLES Opcode = 0x0123

	// Push a special literal on the stack corresponding to the none
	// term of verdicttype.
	//
	// 	[] NONE [lit]
	//
	NONE Opcode = 0x02a3

	// No operation.
	//
	// nop shall normally be combined with eswap. Refer to the eswap
	// operation.
	//
	// 	[] NOP []
	//
	NOP Opcode = 0x0003

	// With boolean input, perform the logical not operation and
	// return the boolean result.
	//
	// With bitstring, hexstring or octetstring input, perform the
	// bitwise not operation and return the result. The result type
	// will be of the same base type as the inputs.
	//
	// 	[a] NOT [res]
	//
	NOT Opcode = 0x0493

	//
	// 	[] NOW [float]
	//
	NOW Opcode = 0x1343

	//
	// 	[] NULL [nullref]
	//
	NULL Opcode = 0x0203

	// Convert octetstring oct into a bitstring and push it onto the
	// stack.
	//
	// 	[oct] OCT2BIT [res]
	//
	OCT2BIT Opcode = 0x1943

	//
	// 	[oct] OCT2CHR [charstring]
	//
	OCT2CHR Opcode = 0x1973

	// Convert octetstring oct into a hexstring and push it onto the
	// stack.
	//
	// 	[oct] OCT2HEX [res]
	//
	OCT2HEX Opcode = 0x1953

	// Convert octetstring oct into an integer and push it onto the
	// stack.
	//
	// 	[oct] OCT2INT [res]
	//
	OCT2INT Opcode = 0x1933

	// Convert octetstring oct into a charstring and push it onto the
	// stack.
	//
	// 	[oct] OCT2STR [res]
	//
	OCT2STR Opcode = 0x1963

	// Push an octetstring literal onto the stack. This is a
	// variable-length instruction with the details of the literal
	// octetstring following the instruction itself: first, a length
	// field, and then the bytes of the octetstring.
	//
	// The length field is encoded with the same byte order as has
	// been selected for instructions. The length contains the number
	// of octets.
	//
	// The string itself is always encoded with the first octet of
	// the string at the lowest file offset – the natural ordering
	// for strings.
	//
	// Padding shall be placed at the end of the string bytes to
	// restore the correct alignment for following instructions.
	//
	// 	[] OCTETS [lit]
	//
	OCTETS Opcode = 0x0133

	// Push the built-in octetstring type object.
	//
	// 	[] OCTETSTRING [obj]
	//
	OCTETSTRING Opcode = 0x0e73

	// Push a special omit literal onto the stack.
	//
	// 	[] OMIT [lit]
	//
	OMIT Opcode = 0x0253

	// With a boolean first argument perform the logical or operation
	// and return the boolean result. The second argument shall be
	// either another boolean or a block. If the first argument is
	// true then the second argument shall not be evaluated.
	//
	// With bitstring, hexstring or octetstring inputs, perform the
	// bitwise or operation and return the result. The result type
	// will be of the same base type as the inputs.
	//
	// 	[obj obj] OR [res]
	//
	OR Opcode = 0x04a3

	// Bind a type and name into an output value parameter object and
	// then attach this to the owning function (altstep, testcase)
	// object.
	//
	// 	[type name] OUT []
	//
	OUT Opcode = 0x0763

	// Push a special literal on the stack corresponding to the pass
	// term of verdicttype.
	//
	// 	[] PASS [lit]
	//
	PASS Opcode = 0x02b3

	// Make string str into a pattern template literal and push this
	// on the stack.
	//
	// In addition to the obvious use with charstring and universal
	// charstring types, pattern shall be used for bitstring,
	// hexstring and octetstring literals when they contain
	// pattern-matching characters.
	//
	// 	[str] PATTERN [pat]
	//
	PATTERN Opcode = 0x0653

	// Modifies a type object to allow it to have the special omit
	// value.
	//
	// 	[type] PERMITO [type]
	//
	PERMITO Opcode = 0x0783

	// Modifies a type object to allow it to have ifpresent values.
	//
	// 	[type] PERMITP [type]
	//
	PERMITP Opcode = 0x07a3

	// Modifies a type object to allow it to have template values.
	//
	// 	[type] PERMITT [type]
	//
	PERMITT Opcode = 0x0793

	// Make a permutation template literal from the input value list
	// vlist. Push the resulting object on the stack.
	//
	// 	[vlist] PERMUTATION [res]
	//
	PERMUTATION Opcode = 0x0663

	// Construct a message port type object push onto the stack.
	// Block iblk shall contain a list of types allowed in through
	// the port and block oblk shall contain a list of types allowed
	// out through the port. The same type may appear in both lists.
	//
	// Refer to the TTCN-3 specification for the precise meaning of
	// the terms in and out under various circumstances.
	//
	// 	[iblk oblk] PORTM [ptype]
	//
	PORTM Opcode = 0x0f43

	// Create message port type
	//
	// 	[in out address map_params unmap_params] PORTMA []
	//
	PORTMA Opcode = 0x0f63

	// Construct a range template literal from bounds lower and
	// upper. Push the resulting object on the stack.
	//
	// 	[lower upper] RANGE [obj]
	//
	RANGE Opcode = 0x0673

	// Read the remaining duration of timer tmr and push the float
	// result onto the stack.
	//
	// 	[tmr] READ [rem]
	//
	READ Opcode = 0x1303

	// Create a receive condition on port port. Block mblk shall
	// contain code to perform the matching of input. When the
	// condition is true (matched) then the enclosing alt will
	// execute block ablk to perform variable assignments.
	//
	// 	[mblk ablk port] RECEIVE []
	//
	RECEIVE Opcode = 0x1443

	// Receive from any port
	//
	// 	[match assign] RECEIVE1 []
	//
	RECEIVE1 Opcode = 0x1463

	// Create a checked receive condition on port port. Block mblk
	// shall contain code to perform the matching of input. When the
	// condition is true (matched) then the enclosing alt will
	// execute block ablk to perform variable assignments.
	//
	// 	[mblk ablk port] RECEIVEC []
	//
	RECEIVEC Opcode = 0x1453

	//
	// 	[match assign] RECEIVEC1 []
	//
	RECEIVEC1 Opcode = 0x1473

	// Construct a record object from field definitions in block fblk
	// and push on to the stack.
	//
	// 	[fblk] RECORD [obj]
	//
	RECORD Opcode = 0x0f83

	// Construct a record-of object with element of type type and
	// push on to the stack.
	//
	// 	[type] RECORDOF [obj]
	//
	RECORDOF Opcode = 0x0fa3

	//
	// 	[ref] REF_BIT2HEX []
	//
	REF_BIT2HEX Opcode = 0x1dc3

	//
	// 	[ref] REF_BIT2INT []
	//
	REF_BIT2INT Opcode = 0x1db3

	//
	// 	[ref] REF_BIT2OCT []
	//
	REF_BIT2OCT Opcode = 0x1dd3

	//
	// 	[ref] REF_BIT2STR []
	//
	REF_BIT2STR Opcode = 0x1de3

	//
	// 	[ref] REF_CHAR2INT []
	//
	REF_CHAR2INT Opcode = 0x1d83

	//
	// 	[ref] REF_CHAR2OCT []
	//
	REF_CHAR2OCT Opcode = 0x1d93

	//
	// 	[ref] REF_DECVALUE []
	//
	REF_DECVALUE Opcode = 0x1f83

	//
	// 	[ref] REF_ENCVALUE []
	//
	REF_ENCVALUE Opcode = 0x1f73

	//
	// 	[ref] REF_ENUM2INT []
	//
	REF_ENUM2INT Opcode = 0x1f53

	//
	// 	[ref] REF_FLOAT2INT []
	//
	REF_FLOAT2INT Opcode = 0x1d73

	//
	// 	[ref] REF_HEX2BIT []
	//
	REF_HEX2BIT Opcode = 0x1e03

	//
	// 	[ref] REF_HEX2INT []
	//
	REF_HEX2INT Opcode = 0x1df3

	//
	// 	[ref] REF_HEX2OCT []
	//
	REF_HEX2OCT Opcode = 0x1e13

	//
	// 	[ref] REF_HEX2STR []
	//
	REF_HEX2STR Opcode = 0x1e23

	//
	// 	[ref] REF_INT2BIT []
	//
	REF_INT2BIT Opcode = 0x1d23

	//
	// 	[ref] REF_INT2CHAR []
	//
	REF_INT2CHAR Opcode = 0x1d03

	//
	// 	[ref] REF_INT2ENUM []
	//
	REF_INT2ENUM Opcode = 0x1fa3

	//
	// 	[ref] REF_INT2FLOAT []
	//
	REF_INT2FLOAT Opcode = 0x1d63

	//
	// 	[ref] REF_INT2HEX []
	//
	REF_INT2HEX Opcode = 0x1d33

	//
	// 	[ref] REF_INT2OCT []
	//
	REF_INT2OCT Opcode = 0x1d43

	//
	// 	[ref] REF_INT2STR []
	//
	REF_INT2STR Opcode = 0x1d53

	//
	// 	[ref] REF_ISBOUND []
	//
	REF_ISBOUND Opcode = 0x1fd3

	//
	// 	[ref] REF_ISPRESENT []
	//
	REF_ISPRESENT Opcode = 0x1ee3

	//
	// 	[ref] REF_ISVALUE []
	//
	REF_ISVALUE Opcode = 0x1f63

	//
	// 	[ref] REF_OCT2BIT []
	//
	REF_OCT2BIT Opcode = 0x1e43

	//
	// 	[ref] REF_OCT2CHR []
	//
	REF_OCT2CHR Opcode = 0x1e73

	//
	// 	[ref] REF_OCT2HEX []
	//
	REF_OCT2HEX Opcode = 0x1e53

	//
	// 	[ref] REF_OCT2INT []
	//
	REF_OCT2INT Opcode = 0x1e33

	//
	// 	[ref] REF_OCT2STR []
	//
	REF_OCT2STR Opcode = 0x1e63

	//
	// 	[ref] REF_REGEXP []
	//
	REF_REGEXP Opcode = 0x1f03

	//
	// 	[ref] REF_STR2FLOAT []
	//
	REF_STR2FLOAT Opcode = 0x1ea3

	//
	// 	[ref] REF_STR2HEX []
	//
	REF_STR2HEX Opcode = 0x1fc3

	//
	// 	[ref] REF_STR2INT []
	//
	REF_STR2INT Opcode = 0x1e83

	//
	// 	[ref] REF_STR2OCT []
	//
	REF_STR2OCT Opcode = 0x1e93

	//
	// 	[ref] REF_TESTCASENAME []
	//
	REF_TESTCASENAME Opcode = 0x1f93

	//
	// 	[ref] REF_VAL2STR []
	//
	REF_VAL2STR Opcode = 0x1ed3

	// Perform a regular expression match of str against expr.
	// Returns the gno-th matching bracketed sub-expression or an
	// empty string if no match.
	//
	// 	[str expr gno] REGEXP [res]
	//
	REGEXP Opcode = 0x1a03

	// Evaluate a rem b, pushing the result on the stack. The TTCN-3
	// definition of rem applies.
	//
	// The types of a and b must be compatible. The returned object
	// has the base type of the input parameters and is constant.
	//
	// 	[a b] REM [res]
	//
	REM Opcode = 0x0473

	// Cause the immediate repeat of the evaluation of the current
	// enclosing alt.
	//
	// 	[] REPEAT []
	//
	REPEAT Opcode = 0x03b3

	// Replace the substring of str, from index idx, for length len,
	// with the string rep and return the resulting string.
	//
	// 	[str idx len rep] REPLACE [res]
	//
	REPLACE Opcode = 0x1a23

	// Causes immediate exit from the current function, altstep,
	// testcase or control.
	//
	// 	[] RETURN []
	//
	RETURN Opcode = 0x0323

	// Compute a random float in the range 0 and 1 exclusive, and
	// return the result.
	//
	// 	[] RND [num]
	//
	RND Opcode = 0x1a33

	// Rotate string str left by num items, and return the result.
	//
	// 	[str num] ROL [res]
	//
	ROL Opcode = 0x04e3

	// Rotate string str right by num items, and return the result.
	//
	// 	[str num] ROR [res]
	//
	ROR Opcode = 0x04f3

	// When called with a component reference comp, evaluate the
	// running state of the component and push the boolean result
	// onto the stack.
	//
	// When called with a timer reference tmr, evaluate the running
	// state of the timer and push the boolean result onto the stack.
	//
	// 	[obj] RUNNING [res]
	//
	RUNNING Opcode = 0x1203

	// Evaluate the state of parallel test components to determine if
	// any are in the running state. Push the boolean result on the
	// stack.
	//
	// 	[] RUNNING1C [res]
	//
	RUNNING1C Opcode = 0x1213

	// Evaluate the state of timers on the current component to
	// determine if any are in the running state. Push the boolean
	// result on the stack.
	//
	// 	[] RUNNING1T [res]
	//
	RUNNING1T Opcode = 0x1313

	// Evaluate the state of parallel test components to determine if
	// all are in the running state. Push the boolean result on the
	// stack.
	//
	// 	[] RUNNINGAC [res]
	//
	RUNNINGAC Opcode = 0x1223

	// Switch from instruction execution to scanning until a matching
	// block is seen.
	//
	// 	[] SCAN [blk]
	//
	SCAN Opcode = 0x00d3

	// Fetch a reference to the current component and push it on the
	// stack.
	//
	// 	[] SELF [obj]
	//
	SELF Opcode = 0x0223

	// Send message msg on port port with no explicit addressing.
	//
	// 	[msg port] SEND []
	//
	SEND Opcode = 0x1403

	// Send message msg on port port to single destination dest
	// (unicast.)
	//
	// 	[dest msg port] SEND1 []
	//
	SEND1 Opcode = 0x1413

	// Send message msg on port port to all associated destinations
	// (broadcast.)
	//
	// 	[msg port] SENDA []
	//
	SENDA Opcode = 0x1433

	// Fetch the sender (from) address of the current communication
	// operation.
	//
	// 	[] SENDER [obj]
	//
	SENDER Opcode = 0x17e3

	// Send message msg on port port to all destinations in block
	// dblk (multicast.)
	//
	// 	[dblk msg port] SENDN []
	//
	SENDN Opcode = 0x1423

	// Construct a set object from field definitions in block fblk
	// and push on to the stack.
	//
	// 	[fblk] SET [obj]
	//
	SET Opcode = 0x0f93

	// Construct a set-of object with element of type type and push
	// on to the stack.
	//
	// 	[type] SETOF [obj]
	//
	SETOF Opcode = 0x0fb3

	// Set the local verdict of the current component from parameter
	// lver. Use within the control part is a runtime error.
	//
	// 	[lver] SETVERDICT []
	//
	SETVERDICT Opcode = 0x1053

	// Shift string str left by num items, and return the result.
	//
	// 	[str num] SHL [res]
	//
	SHL Opcode = 0x04c3

	// Shift string str right by num items, and return the result.
	//
	// 	[str num] SHR [res]
	//
	SHR Opcode = 0x04d3

	// Pushes the number of elements in obj onto the stack as an
	// integer value.
	//
	// 	[obj] SIZEOF [size]
	//
	SIZEOF Opcode = 0x19c3

	// Push a special literal onto the stack, corresponding to the
	// TTCN-3 ‘-‘ and meaning ‘skip this item.’
	//
	// As an optimisation, skip may be used instead of an empty
	// block, saving one operation and avoid invocation of the block
	// scanning mechanism.
	//
	// 	[] SKIP [lit]
	//
	SKIP Opcode = 0x0243

	// Silent match
	//
	// 	[expr tmpl] SMATCH [bool]
	//
	SMATCH Opcode = 0x00f3

	// Define a source file object with file name name and module
	// definitions contained in block mblk.
	//
	// 	[mblk name] SOURCE []
	//
	SOURCE Opcode = 0x08d3

	// Special place
	//
	// 	[blk] SPECPLC []
	//
	SPECPLC Opcode = 0x0d43

	// When called with a port reference port, start activity on the
	// port.
	//
	// When called with a timer reference tmr, start the timer
	// running (or restart.)
	//
	// 	[obj] START []
	//
	START Opcode = 0x1283

	// Start activity on all ports.
	//
	// 	[] STARTAP []
	//
	STARTAP Opcode = 0x12b3

	// Start behaviour on parallel test component comp. Block blk
	// contains the instructions to execute in the context of the
	// newly started component – the behaviour. This shall take the
	// form of any parameters followed by a function call.
	//
	// 	[blk comp] STARTC []
	//
	STARTC Opcode = 0x1293

	// Start timer tmr, with explicit duration dur.
	//
	// 	[dur tmr] STARTD []
	//
	STARTD Opcode = 0x12a3

	// Create an alternative for an enclosing alt operation,
	// associating some condition with statements to perform should
	// the condition be chosen by the alt operation. Block cblk
	// contains code to execute to evaluate the condition of the
	// alternative. Block sblk contains code to execute should be
	// condition be chosen.
	//
	// 	[cblk sblk] STEP []
	//
	STEP Opcode = 0x03e3

	// When called with a component reference comp, stop behaviour on
	// the component.
	//
	// When called with a port reference port, stop activity on the
	// port.
	//
	// When called with a timer reference tmr, stop the timer.
	//
	// 	[obj] STOP []
	//
	STOP Opcode = 0x12c3

	// Stop all parallel test components. Permitted only from the
	// master test component.
	//
	// 	[] STOPAC []
	//
	STOPAC Opcode = 0x12d3

	// Stop all ports.
	//
	// 	[] STOPAP []
	//
	STOPAP Opcode = 0x12e3

	// Stop all timers on the current component.
	//
	// 	[] STOPAT []
	//
	STOPAT Opcode = 0x12f3

	// From within the behaviour of a component, causes immediate
	// termination of the behaviour of that component.
	//
	// From within the behaviour of a control part, causes immediate
	// termination of the execution of that control part.
	//
	// 	[] STOPI []
	//
	STOPI Opcode = 0x0333

	// Stores a local object indexed by x, where x in an unsigned
	// integer value in the range 0 to 32767.
	//
	// 	[obj] STORE []
	//
	STORE Opcode = 0x0063

	// Convert charstring char into a float, and push it onto the
	// stack.
	//
	// 	[char] STR2FLOAT [res]
	//
	STR2FLOAT Opcode = 0x19a3

	//
	// 	[string] STR2HEX [hex]
	//
	STR2HEX Opcode = 0x1ac3

	// Convert charstring char into an integer, and push it onto the
	// stack.
	//
	// 	[char] STR2INT [res]
	//
	STR2INT Opcode = 0x1983

	// Convert charstring char into an octetstring, and push it onto
	// the stack.
	//
	// 	[char] STR2OCT [res]
	//
	STR2OCT Opcode = 0x1993

	// Evaluate a – b, pushing the result on the stack.
	//
	// The types of a and b must be compatible. The returned object
	// has the base type of the input parameters and is constant.
	//
	// 	[a b] SUB [res]
	//
	SUB Opcode = 0x0433

	// Make a subset template literal from the input value list
	// vlist. Push the resulting object on the stack.
	//
	// 	[vlist] SUBSET [res]
	//
	SUBSET Opcode = 0x0683

	// Return the substring of str, from index idx, for length len.
	//
	// 	[str idx len] SUBSTR [res]
	//
	SUBSTR Opcode = 0x1a13

	// Create a subtype from type applying constraint cons, pushing
	// the resulting type object on the stack. Constraint cons may be
	// a single object or a collection of objects.
	//
	// 	[type cons] SUBTYPE [res]
	//
	SUBTYPE Opcode = 0x0ef3

	// Make a superset template literal from the input value list
	// vlist. Push the resulting object on the stack.
	//
	// 	[vlist] SUPERSET [res]
	//
	SUPERSET Opcode = 0x0693

	// Fetch a reference to the current test system interface
	// component and push it on the stack.
	//
	// 	[] SYSTEM [obj]
	//
	SYSTEM Opcode = 0x0233

	// Stop testcase.
	//
	// 	[reason] TCSTOP []
	//
	TCSTOP Opcode = 0x11e3

	// Define a template object from return type, a block pblk
	// containing its parameters, bblk containing its behaviour and a
	// name, and then attach it to the module object owning the
	// current block.
	//
	// 	[type pblk bblk name] TEMPLATE []
	//
	TEMPLATE Opcode = 0x0a43

	// Bind an integer value and name into an enumeration term
	// definition and then attach this to the owning enumeration
	// object.
	//
	// 	[tval name] TERM []
	//
	TERM Opcode = 0x0713

	// Define a testcase object from a runs-on component type rcomp,
	// a block pblk containing its parameters, bblk containing its
	// behaviour and a name, and then attach it to the module object
	// owning the current block.
	//
	// 	[rcomp pblk bblk name] TESTCASE []
	//
	TESTCASE Opcode = 0x0a83

	// Retrieve the current testcase name and push onto the stack.
	// Returns an empty string if called outside any executing
	// testcase.
	//
	// 	[] TESTCASENAME [name]
	//
	TESTCASENAME Opcode = 0x1a93

	// Define a testcase object from a system component type scomp, a
	// runs-on component type rcomp, a block pblk containing its
	// parameters, bblk containing its behaviour and a name, and then
	// attach it to the module object owning the current block.
	//
	// 	[scomp rcomp pblk bblk name] TESTCASES []
	//
	TESTCASES Opcode = 0x0a93

	// Define a testcase object from a system component type scomp, a
	// runs-on component type rcomp, a block pblk containing its
	// parameters, bblk containing its behaviour and a name, and then
	// attach it to the module object owning the current block.
	// Attach attributes from block ablk.
	//
	// 	[ablk scomp rcomp pblk bblk name] TESTCASESW []
	//
	TESTCASESW Opcode = 0x0ab3

	// Define a testcase object from a runs-on component type rcomp,
	// a block pblk containing its parameters, bblk containing its
	// behaviour and a name, and then attach it to the module object
	// owning the current block. Attach attributes from block ablk.
	//
	// 	[ablk rcomp pblk bblk name] TESTCASEW []
	//
	TESTCASEW Opcode = 0x0aa3

	// Create a timeout alternative bound to timer tmr and attach
	// this to the enclosing alt operation.
	//
	// 	[tmr] TIMEOUT []
	//
	TIMEOUT Opcode = 0x1323

	// Create a timeout alternative bound to the expiry of any
	// current timer and attach this to the enclosing alt operation.
	//
	// 	[] TIMEOUT1 []
	//
	TIMEOUT1 Opcode = 0x1333

	// Push the built-in timer type object.
	//
	// 	[] TIMER [obj]
	//
	TIMER Opcode = 0x0ed3

	//
	// 	[] TIMESTAMP [timestamp_id]
	//
	TIMESTAMP Opcode = 0x17f3

	// Implements the TTCN-3 .to operation applied to mapv of
	// type: `type map from A to B MapT`. A setofToType is provided
	// for creation of the return value.
	// Return value is of type `set of B` containing all values
	// from the map.
	//
	// 	[mapv setofToType] TO [res]
	//
	TO Opcode = 0x06d3

	// Trap the current execution. This is used to debug or test the runtime.
	//
	// 	[] TRAP []
	//
	TRAP Opcode = 0x2bc3

	// Create a trigger condition on port port. Block mblk shall
	// contain code to perform the matching of input. When the
	// trigger condition is true (matched) then the enclosing alt
	// will execute block ablk to perform variable assignment.
	//
	// 	[mblk ablk port] TRIGGER []
	//
	TRIGGER Opcode = 0x1483

	//
	// 	[match assign] TRIGGER1 []
	//
	TRIGGER1 Opcode = 0x1493

	// Push a special true literal onto the stack.
	//
	// 	[] TRUE [lit]
	//
	TRUE Opcode = 0x0293

	// Associate a type object and name into a type definition object
	// and then attach this to the owning module or group definition.
	//
	// 	[obj name] TYPE []
	//
	TYPE Opcode = 0x0a63

	// Associate a type object and name into a type definition object
	// and then attach this to the owning module or group definition.
	// Attributes from block ablk are attached.
	//
	// 	[ablk obj name] TYPEW []
	//
	TYPEW Opcode = 0x0a73

	// Construct a union object from field definitions in block fblk
	// and push on to the stack.
	//
	// 	[fblk] UNION [obj]
	//
	UNION Opcode = 0x0ff3

	// Break the mapping between port p1 and port p2.
	//
	// 	[p1 p2] UNMAP []
	//
	UNMAP Opcode = 0x10d3

	// Break all mappings from component comp.
	//
	// 	[comp] UNMAPA []
	//
	UNMAPA Opcode = 0x10e3

	// Break all mappings on all components.
	//
	// 	[] UNMAPAA []
	//
	UNMAPAA Opcode = 0x10f3

	// remove key from mapval.
	//
	// 	[mapval key] UNMAPFROMTO []
	//
	UNMAPFROMTO Opcode = 0x0fd3

	// Push a string literal onto the stack. This is a
	// variable-length instruction with the details of the literal
	// string following the instruction itself: first, a length
	// field, then the bytes of the string, encoded UTF-8.
	//
	// The length field is encoded with the same byte order as has
	// been selected for instructions. The string itself is always
	// encoded with the first byte of the string at the lowest file
	// offset – the natural ordering for strings.
	//
	// Padding shall be placed at the end of the string bytes to
	// restore the correct alignment for following instructions.
	//
	// 	[] UTF8 [lit]
	//
	UTF8 Opcode = 0x0103

	// Return a printable representation of val.
	//
	// 	[val] VAL2STR [string]
	//
	VAL2STR Opcode = 0x1ae3

	// Fetch the value of the current communication operation.
	//
	// For receive and trigger operations this is the current
	// message.
	//
	// For getreply this is the return value of the current
	// procedure.
	//
	// For catch this is the exception of the current procedure.
	//
	// 	[] VALUE [obj]
	//
	VALUE Opcode = 0x17c3

	// Check template tmpl. If it has a single value then return this
	// on the stack otherwise raise a runtime error.
	//
	// 	[tmpl] VALUEOF [value]
	//
	VALUEOF Opcode = 0x1033

	// Create a variable with the given type and name. Bind this
	// variable to the current function (control, altstep, testcase.)
	//
	// 	[type name] VAR []
	//
	VAR Opcode = 0x0703

	// Create a variable with eager template evaluation.
	//
	// 	[type init name] VARDUP []
	//
	VARDUP Opcode = 0x06f3

	// Attach the variant attribute val to the object owning the
	// current block.
	//
	// 	[val] VARIANT []
	//
	VARIANT Opcode = 0x0cc3

	// Attach the overriding variant attribute val to the object
	// owning the current block.
	//
	// 	[val] VARIANTO []
	//
	VARIANTO Opcode = 0x0ce3

	// Push the built-in verdicttype type object.
	//
	// 	[] VERDICTTYPE [obj]
	//
	VERDICTTYPE Opcode = 0x0e83

	// Specify the version number of t3xf to which this file
	// conforms. Version numbers start from one.
	//
	// 	[num] VERSION []
	//
	VERSION Opcode = 0x0033

	// Build a value list from a marked listed of objects,
	// potentially empty. The objects and the mark are removed from
	// the stack and replaced with a value list object.
	//
	// 	[mark object1 .. objectn] VLIST [vlist]
	//
	VLIST Opcode = 0x0073

	// Wait for float seconds.
	//
	// 	[float] WAIT []
	//
	WAIT Opcode = 0x1353

	// Implements the TTCN-3 while statement using two block
	// arguments.
	//
	// Block cond is evaluated to test the loop condition. If the
	// block yields a true value then block body is executed before
	// looping to evaluate the condition again.
	//
	// Execution terminates when the loop condition returns a false
	// value.
	//
	// 	[cond body] WHILE []
	//
	WHILE Opcode = 0x0353

	// With boolean inputs, perform the logical exclusive or
	// operation and return the boolean result.
	//
	// With bitstring, hexstring or octetstring inputs perform the
	// bitwise exclusive or operation and return the result. The
	// result type will be of the same base type as the inputs.
	//
	// 	[a b] XOR [res]
	//
	XOR Opcode = 0x04b3
)

var Descriptions = map[Opcode]*Descriptor{

	ACTION: {
		Opcode:      0x1003,
		Description: "Request the action described in str from the user.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"str"},
			Post: []string{},
		},
	},

	ACTIVATE: {
		Opcode:      0x0d03,
		Description: "Activate an altstep and return a default object. Block blk\nshall contain an altstep call, including any required\nparameters.\n\n",
		Context:     []string{"component_op"},
		Operations: Operation{
			Pre:  []string{"blk"},
			Post: []string{"default"},
		},
	},

	ADD: {
		Opcode:      0x0413,
		Description: "Evaluate a + b, pushing the result.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"a", "b"},
			Post: []string{"res"},
		},
	},

	ADDRESS: {
		Opcode:      0x0ea3,
		Description: "When a module defines a type named ‘address’ then this\ndefinition is returned from the address operation.\n\nWhen a module fails to define an address type then this\noperation returns a built-in opaque address type object.\n\nEach defined module has its own unique definition of address.\n\n",
		Context:     []string{"type_ref"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"obj"},
		},
	},

	ALIVE: {
		Opcode:      0x1103,
		Description: "Test the state of component comp for the alive condition. Push\nthe boolean result onto the stack.\n\n",
		Context:     []string{"component_op"},
		Operations: Operation{
			Pre:  []string{"comp"},
			Post: []string{"res"},
		},
	},

	ALIVE1: {
		Opcode:      0x1113,
		Description: "Test the state of all parallel test components for any that\nare alive. Push the boolean result onto the stack.\n\n",
		Context:     []string{"component_op", "any"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"res"},
		},
	},

	ALIVEA: {
		Opcode:      0x1123,
		Description: "Test the state of all parallel test components checking that\nall are alive. Push the boolean result onto the stack.\n\n",
		Context:     []string{"component_op", "all"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"res"},
		},
	},

	ALLFROM: {
		Opcode:      0x06a3,
		Description: "",
		Context:     []string{"value", "all"},
		Operations: Operation{
			Pre:  []string{"obj"},
			Post: []string{"res"},
		},
	},

	ALLFROMP: {
		Opcode:      0x06b3,
		Description: "",
		Context:     []string{"value", "all"},
		Operations: Operation{
			Pre:  []string{"obj"},
			Post: []string{"res"},
		},
	},

	ALT: {
		Opcode:      0x0383,
		Description: "Evaluate the contents of blk until one of the contained\nalternatives succeeds.\n\nAn empty block is allowed: activated defaults will still be\nincluded in the operation.\n\nIf the successful alternative executes a repeat statement then\nthe whole block execution will be re-evaluated.\n\n",
		Context:     []string{"eval"},
		Operations: Operation{
			Pre:  []string{"blk"},
			Post: []string{},
		},
	},

	ALTND: {
		Opcode:      0x2023,
		Description: "Same as `alt` with disabled defaults (`@nodefault`)\n\n",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"blk"},
			Post: []string{},
		},
	},

	ALTSTEP: {
		Opcode:      0x0803,
		Arg:         ArgImm,
		Description: "Define an altstep object from a block pblk containing its\nparameters, bblk containing its behaviour and a name, and then\nattach it to the module object owning the current block.\n\n",
		Context:     []string{"beha_def"},
		Operations: Operation{
			Pre:  []string{"pblk", "bblk", "name"},
			Post: []string{},
		},
	},

	ALTSTEPB: {
		Opcode:      0x0813,
		Arg:         ArgImm,
		Description: "Define a bound altstep object from a component type comp, a\nblock pblk containing its parameters, bblk containing its\nbehaviour and a name, then attach it to the module object\nowning the current block.\n\n",
		Context:     []string{"beha_def"},
		Operations: Operation{
			Pre:  []string{"comp", "pblk", "bblk", "name"},
			Post: []string{},
		},
	},

	ALTSTEPBW: {
		Opcode:      0x0833,
		Arg:         ArgImm,
		Description: "Define a bound altstep object from a component type comp, a\nblock pblk containing its parameters, bblk containing its\nbehaviour and a name, attach the attributes from block ablk,\nand then attach it to the module object owning the current\nblock.\n\n",
		Context:     []string{"beha_def"},
		Operations: Operation{
			Pre:  []string{"comp", "ablk", "pblk", "bblk", "name"},
			Post: []string{},
		},
	},

	ALTSTEPW: {
		Opcode:      0x0823,
		Arg:         ArgImm,
		Description: "Define an altstep object from a block pblk containing its\nparameters, bblk containing its behaviour and a name, attach\nthe attributes from block ablk, and then attach it to the\nmodule object owning the current block.\n\n",
		Context:     []string{"beha_def"},
		Operations: Operation{
			Pre:  []string{"ablk", "pblk", "bblk", "name"},
			Post: []string{},
		},
	},

	AND: {
		Opcode:      0x0483,
		Description: "With a boolean first argument, perform the logical and\noperation and return the boolean result. The second argument\nmay be either another boolean or a block. If the first\nargument is false then the second argument shall not be\nevaluated.\n\nWith bitstring, hexstring or octetstring inputs, perform the\nbitwise and operation and return the result. The result type\nwill be of the same base type as the inputs.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"obj", "obj"},
			Post: []string{"res"},
		},
	},

	ANY: {
		Opcode:      0x0603,
		Description: "Push a special any-value literal on the stack, corresponding\nto the TTCN-3 ‘?’ literal.\n\n",
		Context:     []string{"literal"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"lit"},
		},
	},

	ANYN: {
		Opcode:      0x0613,
		Description: "Push a special any-value-or-none literal on the stack,\ncorresponding to the TTCN-3 ‘*’ literal.\n\n",
		Context:     []string{"literal"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"lit"},
		},
	},

	APPLY: {
		Opcode:      0x0193,
		Description: "",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{"closure"},
			Post: []string{},
		},
	},

	ARRAY: {
		Opcode:      0x0ee3,
		Description: "Create an array type object from an element type type and a\ndimension dim. Push the resulting type object on the stack.\nThe array dimension shall be either a positive integer or a\nrange of integers (positive, no infinities.)\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"type", "dim"},
			Post: []string{"obj"},
		},
	},

	ASSIGN: {
		Opcode:      0x0583,
		Description: "Assign the value of b to a.\n\nThe types of a and b must be compatible.\n\n",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{"a", "b"},
			Post: []string{},
		},
	},

	ASSIGND: {
		Opcode:      0x0593,
		Description: "Assign the value of b to a. Leave object a on the stack.\n\nThe types of a and b must be compatible.\n\n",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{"a", "b"},
			Post: []string{"a"},
		},
	},

	AT_DEFAULT: {
		Opcode:      0x2013,
		Description: "",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{"obj"},
			Post: []string{},
		},
	},

	BIT2HEX: {
		Opcode:      0x18c3,
		Description: "Convert bitstring bit into a hexstring and push it onto the\nstack.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"bit"},
			Post: []string{"res"},
		},
	},

	BIT2INT: {
		Opcode:      0x18b3,
		Description: "Convert bitstring bit into an integer and push it onto the\nstack.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"bit"},
			Post: []string{"res"},
		},
	},

	BIT2OCT: {
		Opcode:      0x18d3,
		Description: "Convert bitstring bit into an octetstring and push it onto the\nstack.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"bit"},
			Post: []string{"res"},
		},
	},

	BIT2STR: {
		Opcode:      0x18e3,
		Description: "Convert bitstring bit into a charstring and push it onto the\nstack.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"bit"},
			Post: []string{"res"},
		},
	},

	BITS: {
		Opcode:      0x0113,
		Arg:         ArgBits,
		Description: "Push a bitstring literal onto the stack. This is a\nvariable-length instruction with the details of the literal\nbitstring following the instruction itself: first, a length\nfield, and then the bytes of the bitstring.\n\nThe length field is encoded with the same byte order as has\nbeen selected for instructions. The length contains the number\nof bits.\n\nThe string itself is always encoded with the first bit of the\nstring at the lowest file offset – the natural ordering for\nstrings – and in the most-significant bit of that byte.\n\nPadding shall be placed at the end of the string bytes to\nrestore the correct alignment for following instructions.\n\n",
		Context:     []string{"literal", "multibyte"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"lit"},
		},
	},

	BITSTRING: {
		Opcode:      0x0e03,
		Description: "Push the built-in bitstring type object.\n\n",
		Context:     []string{"type_ref"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"obj"},
		},
	},

	BLOCK: {
		Opcode:      0x0083,
		Description: "Switch from scanning back to execution mode pushing a block\nobject onto the stack.\n\n",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"blk"},
		},
	},

	BOOLEAN: {
		Opcode:      0x0e13,
		Description: "Push the built-in boolean type object.\n\n",
		Context:     []string{"type_ref"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"obj"},
		},
	},

	BREAK: {
		Opcode:      0x03c3,
		Description: "Leave the enclosing loop construct.\n\n",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{},
		},
	},

	CAT: {
		Opcode:      0x0563,
		Description: "Concatenates strings a and b and returns the result. Objects a\nand b must both be strings of the same base type.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"a", "b"},
			Post: []string{"res"},
		},
	},

	CHAR2INT: {
		Opcode:      0x1883,
		Description: "Convert single character string char into an integer and push\nit onto the stack.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"char"},
			Post: []string{"res"},
		},
	},

	CHAR2OCT: {
		Opcode:      0x1893,
		Description: "Convert charstring char into an octetstring and push it onto\nthe stack.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"char"},
			Post: []string{"res"},
		},
	},

	CHARSTRING: {
		Opcode:      0x0e23,
		Description: "Push the built-in charstring type object.\n\n",
		Context:     []string{"type_ref"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"obj"},
		},
	},

	CHARSTRINGU: {
		Opcode:      0x0e33,
		Description: "Push the built-in universal charstring type object.\n\n",
		Context:     []string{"type_ref"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"obj"},
		},
	},

	CHECK: {
		Opcode:      0x14a3,
		Description: "Create a pure check condition on port port. Block mblk shall\ncontain code to perform the matching of input. When the\ncondition is true (matched) then the enclosing alt will\nexecute block ablk to perform variable assignments.\n\n",
		Context:     []string{"port_op"},
		Operations: Operation{
			Pre:  []string{"mblk", "ablk", "port"},
			Post: []string{},
		},
	},

	CHECK1: {
		Opcode:      0x14b3,
		Description: "Any port check",
		Context:     []string{"port_op", "any"},
		Operations: Operation{
			Pre:  []string{"match", "assign"},
			Post: []string{},
		},
	},

	CHECKSTATE: {
		Opcode:      0x1783,
		Description: "",
		Context:     []string{"port_op"},
		Operations: Operation{
			Pre:  []string{"port", "string"},
			Post: []string{"bool"},
		},
	},

	CHECKSTATEAL: {
		Opcode:      0x1793,
		Description: "All port checkstate",
		Context:     []string{"port_op", "all"},
		Operations: Operation{
			Pre:  []string{"string"},
			Post: []string{"bool"},
		},
	},

	CHECKSTATEAN: {
		Opcode:      0x17a3,
		Description: "Any port checkstate",
		Context:     []string{"port_op", "any"},
		Operations: Operation{
			Pre:  []string{"string"},
			Post: []string{"bool"},
		},
	},

	CLEAR: {
		Opcode:      0x1383,
		Description: "Clear all contents from the input queue of port p.\n\n",
		Context:     []string{"port_op"},
		Operations: Operation{
			Pre:  []string{"p"},
			Post: []string{},
		},
	},

	CLEARA: {
		Opcode:      0x1393,
		Description: "Clear all contents from the input queues of all ports on the\ncurrent component.\n\n",
		Context:     []string{"port_op", "all"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{},
		},
	},

	CLONE: {
		Opcode:      0x0403,
		Description: "Clone obj and push back clone.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"obj"},
			Post: []string{"res"},
		},
	},

	CLOSURE: {
		Opcode:      0x11f3,
		Description: "Wrap obj as closure",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{"closure"},
		},
	},

	CLOSURETYPE: {
		Opcode:      0x0e93,
		Description: "",
		Context:     []string{"type_ref"},
		Operations: Operation{
			Pre:  []string{"type_ref"},
			Post: []string{},
		},
	},

	COLLECT: {
		Opcode:      0x0093,
		Description: "Build a collection from a marked listed of objects,\npotentially empty. The objects and the mark are removed from\nthe stack and replaced with a collection object.\n\n",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{"mark", "object1", "..", "objectn"},
			Post: []string{"col"},
		},
	},

	COMPLEMENT: {
		Opcode:      0x0623,
		Description: "Make a complement template literal from the input value list\nvlist. Push the resulting object on the stack.\n\n",
		Context:     []string{"value"},
		Operations: Operation{
			Pre:  []string{"vlist"},
			Post: []string{"res"},
		},
	},

	COMPONENT: {
		Opcode:      0x0f23,
		Description: "Construct a component type object with initialisation from\nblock iblk and push on to the stack.\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"iblk"},
			Post: []string{"ctype"},
		},
	},

	COMPONENTX: {
		Opcode:      0x0f33,
		Description: "Construct a component type object with initialisation from\nblock iblk, extending components listed in collection coll and\npush on to the stack.\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"coll", "iblk"},
			Post: []string{"ctype"},
		},
	},

	CONNECT: {
		Opcode:      0x1083,
		Description: "Connect ports p1 and p2.\n\n",
		Context:     []string{"port_op"},
		Operations: Operation{
			Pre:  []string{"p1", "p2"},
			Post: []string{},
		},
	},

	CONST: {
		Opcode:      0x0843,
		Arg:         ArgImm,
		Description: "Create a constant data definition from a block vblk containing\nthe statements of its value, a type type and a name.\n\n",
		Context:     []string{"decl"},
		Operations: Operation{
			Pre:  []string{"vblk", "type", "name"},
			Post: []string{},
		},
	},

	CONSTW: {
		Opcode:      0x0853,
		Arg:         ArgImm,
		Description: "Create a constant data definition from a block vblk containing\nthe statements of its value, a type type and a name, applying\nattributes from block ablk.\n\n",
		Context:     []string{"decl"},
		Operations: Operation{
			Pre:  []string{"ablk", "vblk", "type", "name"},
			Post: []string{},
		},
	},

	CONTINUE: {
		Opcode:      0x03d3,
		Description: "Repeat the enclosing loop construct.\n\n",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{},
		},
	},

	CONTROL: {
		Opcode:      0x0863,
		Arg:         ArgImm,
		Description: "Create a control part definition from a block blk containing\nthe statements of its behaviour.\n\n",
		Context:     []string{"beha_def"},
		Operations: Operation{
			Pre:  []string{"blk"},
			Post: []string{},
		},
	},

	CONTROLW: {
		Opcode:      0x0873,
		Description: "A control part definition with attributes attached.\n\n",
		Context:     []string{"beha_def"},
		Operations: Operation{
			Pre:  []string{"blk"},
			Post: []string{},
		},
	},

	CREATE: {
		Opcode:      0x1183,
		Description: "Create an instance of a component of type ctype and push the\nresult onto the stack.\n\n",
		Context:     []string{"component_new"},
		Operations: Operation{
			Pre:  []string{"ctype"},
			Post: []string{"cinst"},
		},
	},

	CREATEA: {
		Opcode:      0x1193,
		Description: "Create an alive-type instance of a component of type ctype and\npush the result onto the stack.\n\n",
		Context:     []string{"component_new"},
		Operations: Operation{
			Pre:  []string{"ctype"},
			Post: []string{"cinst"},
		},
	},

	CREATEAN: {
		Opcode:      0x11b3,
		Description: "Create a named alive-type instance of a component of type\nctype and push the result onto the stack. The component\ninstance name is taken from the name parameter.\n\n",
		Context:     []string{"component_new"},
		Operations: Operation{
			Pre:  []string{"name", "ctype"},
			Post: []string{"cinst"},
		},
	},

	CREATEN: {
		Opcode:      0x11a3,
		Description: "Create a named instance of a component of type ctype and push\nthe result onto the stack. The component instance name is\ntaken from the name parameter.\n\n",
		Context:     []string{"component_new"},
		Operations: Operation{
			Pre:  []string{"name", "ctype"},
			Post: []string{"cinst"},
		},
	},

	DEACTIVATE: {
		Opcode:      0x0d23,
		Description: "Deactivate default altstep default.\n\n",
		Context:     []string{"component_op"},
		Operations: Operation{
			Pre:  []string{"default"},
			Post: []string{},
		},
	},

	DEACTIVATEA: {
		Opcode:      0x0d33,
		Description: "Deactivate all altsteps activated on the current component.\n\n",
		Context:     []string{"component_op", "all"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{},
		},
	},

	DECMATCH: {
		Opcode:      0x2033,
		Description: "The encoded value enc is decoded and the decoded content\nwill be matched against the provided decTemplate returning\na boolean value res on the stack\n\n",
		Context:     []string{"value"},
		Operations: Operation{
			Pre:  []string{"enc", "decTemplate"},
			Post: []string{"res"},
		},
	},

	DECVALUE: {
		Opcode:      0x1a83,
		Description: "Decode encoded bitstring enc placing the result in dec and\nreturning integer res holding the outcome of the operation: 0\nindicates success, 1 indicates unspecified failure and 2\nindicates insufficient data (retry after adding bits to enc.)\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"enc", "dec"},
			Post: []string{"res"},
		},
	},

	DEF: {
		Opcode:      0x05b3,
		Description: "",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{"from", "what"},
			Post: []string{"ref"},
		},
	},

	DEFAULT: {
		Opcode:      0x0ec3,
		Description: "Push the built-in default type object.\n\n",
		Context:     []string{"type_ref"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"ref"},
		},
	},

	DISCONNECT: {
		Opcode:      0x1093,
		Description: "Break the connection between port p1 and port p2.\n\n",
		Context:     []string{"port_op"},
		Operations: Operation{
			Pre:  []string{"p1", "p2"},
			Post: []string{},
		},
	},

	DISCONNECTA: {
		Opcode:      0x10a3,
		Description: "Break all connections from component comp.\n\n",
		Context:     []string{"port_op", "all"},
		Operations: Operation{
			Pre:  []string{"comp"},
			Post: []string{},
		},
	},

	DISCONNECTAA: {
		Opcode:      0x10b3,
		Description: "Break all connections on all components.\n\n",
		Context:     []string{"port_op", "component_op", "all"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{},
		},
	},

	DIV: {
		Opcode:      0x0453,
		Description: "Evaluate a / b, pushing the result on the stack.\n\nThe types of a and b must be compatible. The returned object\nhas the base type of the input parameters and is constant.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"a", "b"},
			Post: []string{"res"},
		},
	},

	DONE: {
		Opcode:      0x1143,
		Description: "Test the state of component comp for the done condition,\ncontributing this to an enclosing alt or interleave statement.\n\n",
		Context:     []string{"component_op"},
		Operations: Operation{
			Pre:  []string{"comp"},
			Post: []string{},
		},
	},

	DONE1: {
		Opcode:      0x1153,
		Description: "Test the state of all parallel test components for any that\nare done, contributing this to an enclosing alt or interleave\nstatement.\n\n",
		Context:     []string{"component_op", "any"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{},
		},
	},

	DONEA: {
		Opcode:      0x1163,
		Description: "Test the state of all parallel test components checking that\nall are done, contributing this to an enclosing alt or\ninterleave statement.\n\n",
		Context:     []string{"component_op", "all"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{},
		},
	},

	DOWHILE: {
		Opcode:      0x0363,
		Description: "Implements the TTCN-3 do/while statement using two block\narguments.\n\nBlock body is executed and then loop condition block cond is\nevaluated. If the condition returns true then the process is\nrepeated.\n\nExecution terminates when the loop condition returns a false\nvalue.\n\n",
		Context:     []string{"vm", "control_flow"},
		Operations: Operation{
			Pre:  []string{"body", "cond"},
			Post: []string{},
		},
	},

	DROP: {
		Opcode:      0x00a3,
		Description: "Remove the item at the top of the stack and discard it.\n\n",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{"obj"},
			Post: []string{},
		},
	},

	DUP: {
		Opcode:      0x00b3,
		Description: "Duplicate the item at the top of the stack.\n\n",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{"obj"},
			Post: []string{"obj", "obj"},
		},
	},

	ELSE: {
		Opcode:      0x03a3,
		Description: "Create an alternative for an enclosing alt operation that is\nimmediately successful. Block blk contains the code to execute\nif this alternative is chosen by the containing alt.\n\n",
		Context:     []string{"eval"},
		Operations: Operation{
			Pre:  []string{"blk"},
			Post: []string{},
		},
	},

	ENCODE: {
		Opcode:      0x0c43,
		Description: "Attach the encode attribute val to the object owning the\ncurrent block.\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"val"},
			Post: []string{},
		},
	},

	ENCODEO: {
		Opcode:      0x0c63,
		Description: "Attach the overriding encode attribute val to the object\nowning the current block.\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"val"},
			Post: []string{},
		},
	},

	ENCVALUE: {
		Opcode:      0x1a73,
		Description: "Encode value value returning bitstring res holding the encoded\nresult.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"value"},
			Post: []string{"res"},
		},
	},

	ENUM2INT: {
		Opcode:      0x1a53,
		Description: "Convert enumerated enum into an integer and push it onto the\nstack.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"enum"},
			Post: []string{"res"},
		},
	},

	ENUMERATED: {
		Opcode:      0x0f03,
		Description: "Build an enumerated object from the terms described in block\nblk. Push the resulting object onto the stack.\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"blk"},
			Post: []string{"obj"},
		},
	},

	EQ: {
		Opcode:      0x0503,
		Description: "Evaluate a == b, pushing the boolean result.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"a", "b"},
			Post: []string{"res"},
		},
	},

	ERROR: {
		Opcode:      0x02e3,
		Description: "Push a special literal on the stack corresponding to the error\nterm of verdicttype.\n\n",
		Context:     []string{"literal"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"lit"},
		},
	},

	EXEC: {
		Opcode:      0x0373,
		Description: "Execute block blk. This is an optimisation of if(true), giving\na space saving of two operations with considerably better\nperformance.\n\nThe motivation for this operation is the translation of a\nselect statement containing only a single else statement.\nPlacing the block inline is not possible because of the issue\nof variable scoping.\n\n",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{"blk"},
			Post: []string{},
		},
	},

	EXECUTE: {
		Opcode:      0x1063,
		Description: "Execute testcase and return its verdict. The input parameter\nis a block blk containing any actual parameters to the\ntestcase, followed by the invocation of the testcase itself.\n\n",
		Context:     []string{"testcase_new"},
		Operations: Operation{
			Pre:  []string{"blk"},
			Post: []string{"verdict"},
		},
	},

	EXECUTED: {
		Opcode:      0x11d3,
		Description: "Same as execute, but with logging disabled.\n\n",
		Context:     []string{"testcase_new"},
		Operations: Operation{
			Pre:  []string{"blk"},
			Post: []string{"verdict"},
		},
	},

	EXECUTEL: {
		Opcode:      0x1073,
		Description: "Execute testcase and return its verdict. The input parameter\nis a block blk containing any actual parameters to be supplied\nto the testcase, followed by the invocation of the testcase\nitself. Time limit limit shall be applied to the testcase\nexecution.\n\n",
		Context:     []string{"testcase_new"},
		Operations: Operation{
			Pre:  []string{"limit", "blk"},
			Post: []string{"verdict"},
		},
	},

	EXECUTELD: {
		Opcode:      0x11c3,
		Description: "Same as executel, but with logging disabled.\n\n",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"limit", "blk"},
			Post: []string{"verdict"},
		},
	},

	EXTENSION: {
		Opcode:      0x0c83,
		Description: "Attach the extension attribute val to the object owning the\ncurrent block.\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"val"},
			Post: []string{},
		},
	},

	EXTENSIONO: {
		Opcode:      0x0ca3,
		Description: "Attach the overriding extension attribute val to the object\nowning the current block.\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"val"},
			Post: []string{},
		},
	},

	FAIL: {
		Opcode:      0x02d3,
		Description: "Push a special literal on the stack corresponding to the fail\nterm of verdicttype.\n\n",
		Context:     []string{"literal"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"lit"},
		},
	},

	FALSE: {
		Opcode:      0x0283,
		Description: "Push a special false literal onto the stack.\n\n",
		Context:     []string{"literal"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"lit"},
		},
	},

	FIELD: {
		Opcode:      0x0723,
		Description: "Build a field description from type and name and attach it to\nthe structured type object owning the current block.\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"type", "name"},
			Post: []string{},
		},
	},

	FIELDO: {
		Opcode:      0x0733,
		Description: "Build an optional field description from type and name and\nattach it to the structured type object owning the current\nblock.\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"type", "name"},
			Post: []string{},
		},
	},

	FLOAT: {
		Opcode:      0x0e43,
		Description: "Push the built-in float type object.\n\n",
		Context:     []string{"type_ref"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"obj"},
		},
	},

	FLOAT2INT: {
		Opcode:      0x1873,
		Description: "Convert float flt into an integer and push it onto the stack.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"flt"},
			Post: []string{"res"},
		},
	},

	FOR: {
		Opcode:      0x0343,
		Description: "Implements the TTCN-3 for statement using four block\narguments.\n\nFirst, block init is executed to establish initial values.\n\nBlock cond is evaluated to test the loop condition. If the\nblock yields a true value then block body is executed followed\nby block incr to change the loop condition before looping to\nevaluate the loop condition again.\n\nExecution terminates when the loop condition returns a false\nvalue.\n\nVariables created within block init are visible within blocks\ncond, incr and body. Such variables will be destroyed when the\noperation terminates.\n\n",
		Context:     []string{"vm", "control_flow"},
		Operations: Operation{
			Pre:  []string{"init", "cond", "body", "incr"},
			Post: []string{},
		},
	},

	FROM: {
		Opcode:      0x06c3,
		Description: "Implements the TTCN-3 .from operation applied to mapv of \ntype: `type map from A to B MapT`. A setofFromType is provided\nfor creation of the return value. \nReturn value is of type `set of A` containing all keys\nfrom the map.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"mapv", "setofFromType"},
			Post: []string{"res"},
		},
	},

	FSTR: {
		Opcode:      0x0173,
		Arg:         ArgString,
		Description: "Push a float literal onto the stack, with the value coded as\nan ASCII string to allow for (nearly) arbitrary size and\nprecision. This is a variable-length instruction with the\ndetails of the literal string following the instruction\nitself: first, a length field, and then the bytes of the\nstring.\n\nThe length field is encoded with the same byte order as has\nbeen selected for instructions. The string itself is always\nencoded with the first byte of the string at the lowest file\noffset – the natural ordering for strings.\n\nPadding shall be placed at the end of the string bytes to\nrestore the correct alignment for following instructions.\n\n",
		Context:     []string{"literal", "multibyte"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"lit"},
		},
	},

	FUNCTION: {
		Opcode:      0x0903,
		Arg:         ArgImm,
		Description: "Define a function object from a block pblk containing its\nparameters, bblk containing its behaviour and a name, and then\nattach it to the module object owning the current block.\n\n",
		Context:     []string{"beha_def"},
		Operations: Operation{
			Pre:  []string{"pblk", "bblk", "name"},
			Post: []string{},
		},
	},

	FUNCTIONB: {
		Opcode:      0x0913,
		Arg:         ArgImm,
		Description: "Define a bound function object from a component type comp, a\nblock pblk containing its parameters, bblk containing its\nbehaviour and a name, then attach it to the module object\nowning the current block.\n\n",
		Context:     []string{"beha_def"},
		Operations: Operation{
			Pre:  []string{"comp", "pblk", "bblk", "name"},
			Post: []string{},
		},
	},

	FUNCTIONV: {
		Opcode:      0x0943,
		Arg:         ArgImm,
		Description: "Define a value-returning function object from return type, a\nblock pblk containing its parameters, bblk containing its\nbehaviour and a name, and then attach it to the module object\nowning the current block.\n\n",
		Context:     []string{"beha_def"},
		Operations: Operation{
			Pre:  []string{"type", "pblk", "bblk", "name"},
			Post: []string{},
		},
	},

	FUNCTIONVB: {
		Opcode:      0x0953,
		Arg:         ArgImm,
		Description: "Define a value-returning bound function object from return\ntype, a component type comp, a block pblk containing its\nparameters, bblk containing its behaviour and a name, then\nattach it to the module object owning the current block.\n\n",
		Context:     []string{"beha_def"},
		Operations: Operation{
			Pre:  []string{"type", "comp", "pblk", "bblk", "name"},
			Post: []string{},
		},
	},

	FUNCTIONX: {
		Opcode:      0x0983,
		Description: "Define an external function object from a block pblk\ncontaining its parameters and a name, and then attach it to\nthe module object owning the current block.\n\n",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"pblk", "name"},
			Post: []string{},
		},
	},

	FUNCTIONXV: {
		Opcode:      0x09a3,
		Description: "Define a value-returning external function object from return\ntype, a block pblk containing its parameters and a name, and\nthen attach it to the module object owning the current block.\n\n",
		Context:     []string{"beha_def"},
		Operations: Operation{
			Pre:  []string{"type", "pblk", "name"},
			Post: []string{},
		},
	},

	FUNCTIONXVW: {
		Opcode:      0x09b3,
		Description: "Define a value-returning external function object from return\ntype, a block pblk containing its parameters and a name,\nattach the attributes from block ablk, and then attach it to\nthe module object owning the current block.\n\n",
		Context:     []string{"beha_def"},
		Operations: Operation{
			Pre:  []string{"type", "ablk", "pblk", "name"},
			Post: []string{},
		},
	},

	FUNCTIONXW: {
		Opcode:      0x0993,
		Description: "Define an external function object from a block pblk\ncontaining its parameters and a name, attach the attributes\nfrom block ablk, and then attach it to the module object\nowning the current block.\n\n",
		Context:     []string{"beha_def"},
		Operations: Operation{
			Pre:  []string{"ablk", "pblk", "name"},
			Post: []string{},
		},
	},

	GE: {
		Opcode:      0x0513,
		Description: "Evaluate a >= b, pushing the boolean result.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"a", "b"},
			Post: []string{"res"},
		},
	},

	GET: {
		Opcode:      0x0573,
		Description: "When object obj is of type record, record-of, set, set-of,\nunion, array or string, fetch a reference to the element\nindexed by idx, where idx is an unsigned integer value.\n\nWhen object obj is of type component fetch a reference to the\nfield name.\n\n",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{"obj", "field"},
			Post: []string{"obj"},
		},
	},

	GETVERDICT: {
		Opcode:      0x1043,
		Description: "Get the local verdict of the current component and push it on\nthe stack. Use within the control part is a runtime error.\n\n",
		Context:     []string{"component_op"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"lver"},
		},
	},

	GT: {
		Opcode:      0x0523,
		Description: "Evaluate a > b, pushing the boolean result.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"a", "b"},
			Post: []string{"res"},
		},
	},

	HALT: {
		Opcode:      0x13a3,
		Description: "Instruct port p to accept no further input.\n\n",
		Context:     []string{"port_op"},
		Operations: Operation{
			Pre:  []string{"p"},
			Post: []string{},
		},
	},

	HALTA: {
		Opcode:      0x13b3,
		Description: "Instruct all ports on the current component to accept no\nfurther input.\n\n",
		Context:     []string{"port_op", "all"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{},
		},
	},

	HEX2BIT: {
		Opcode:      0x1903,
		Description: "Convert hexstring hex into a bitstring and push it onto the\nstack.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"hex"},
			Post: []string{"res"},
		},
	},

	HEX2INT: {
		Opcode:      0x18f3,
		Description: "Convert hexstring hex into an integer and push it onto the\nstack.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"hex"},
			Post: []string{"res"},
		},
	},

	HEX2OCT: {
		Opcode:      0x1913,
		Description: "Convert hexstring hex into an octetstring and push it onto the\nstack.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"hex"},
			Post: []string{"res"},
		},
	},

	HEX2STR: {
		Opcode:      0x1923,
		Description: "Convert hexstring hex into a charstring and push it onto the\nstack.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"hex"},
			Post: []string{"res"},
		},
	},

	HEXSTRING: {
		Opcode:      0x0e53,
		Description: "Push the built-in hexstring type object.\n\n",
		Context:     []string{"type_ref"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"obj"},
		},
	},

	IDEF: {
		Opcode:      0x05c3,
		Arg:         ArgImm,
		Description: "Same behaviour as 'def'.\n\n",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{"from"},
			Post: []string{"ref"},
		},
	},

	IEEE754DP: {
		Opcode:      0x0153,
		Arg:         ArgFloat,
		Description: "Push a float literal onto the stack. The encoding of the\nfloating-point value follows the instruction word and shall be\nin IEEE754 double-precision format.\n\nIEEE754 double precision has a fixed size of 64-bits. Padding\nwill be required if the current instruction width is greater\nthan 64-bit.\n\nThe eight bytes of the representation must be swapped if\nswapping is enabled.\n\n",
		Context:     []string{"literal", "multibyte"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"lit"},
		},
	},

	IF: {
		Opcode:      0x0303,
		Description: "Execute block blk if cond is true.\n\n",
		Context:     []string{"vm", "control_flow"},
		Operations: Operation{
			Pre:  []string{"cond", "blk"},
			Post: []string{},
		},
	},

	IFELSE: {
		Opcode:      0x0313,
		Description: "Execute block tblk if cond is true else execute block fblk.\n\n",
		Context:     []string{"vm", "control_flow"},
		Operations: Operation{
			Pre:  []string{"cond", "tblk", "fblk"},
			Post: []string{},
		},
	},

	IFIELD: {
		Opcode:      0x05d3,
		Arg:         ArgImm,
		Description: "",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"type", "name"},
			Post: []string{},
		},
	},

	IFPRESENT: {
		Opcode:      0x0633,
		Description: "Modify obj to allow it to match optional fields in record or\nset objects. Leave the object on the stack.\n\n",
		Context:     []string{"value"},
		Operations: Operation{
			Pre:  []string{"obj"},
			Post: []string{"obj"},
		},
	},

	IGET: {
		Opcode:      0x05a3,
		Arg:         ArgImm,
		Description: "When object obj is of type record, record-of, set, set-of,\nunion, array or string, fetch a reference to the element\nindexed by x, where x is an unsigned integer value in the\nrange 0 to 32767.\n\nThis is the performance-optimised version of the get operation\nabove.\n\n",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{"obj"},
			Post: []string{"obj"},
		},
	},

	IN: {
		Opcode:      0x0743,
		Description: "Bind a type and name into an input value parameter object and\nthen attach this to the owning function (altstep, testcase)\nobject.\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"type", "name"},
			Post: []string{},
		},
	},

	INCONC: {
		Opcode:      0x02c3,
		Description: "Push a special literal on the stack corresponding to the\ninconc term of verdicttype.\n\n",
		Context:     []string{"literal"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"lit"},
		},
	},

	INFINITYN: {
		Opcode:      0x0263,
		Description: "Push a special -infinity literal onto the stack.\n\n",
		Context:     []string{"literal"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"lit"},
		},
	},

	INFINITYP: {
		Opcode:      0x0273,
		Description: "Push a special +infinity literal onto the stack.\n\n",
		Context:     []string{"literal"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"lit"},
		},
	},

	INOUT: {
		Opcode:      0x0753,
		Description: "Bind a type and name into an input/output value parameter\nobject and then attach this to the owning function (altstep,\ntestcase) object.\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"type", "name"},
			Post: []string{},
		},
	},

	INT2BIT: {
		Opcode:      0x1823,
		Description: "Convert integer int into a bitstring and push it onto the\nstack. len supplies a minimum length for the resulting string.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"int", "len"},
			Post: []string{"res"},
		},
	},

	INT2CHAR: {
		Opcode:      0x1803,
		Description: "Convert integer int into a single character string and push it\nonto the stack.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"int"},
			Post: []string{"res"},
		},
	},

	INT2ENUM: {
		Opcode:      0x1aa3,
		Description: "",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"a"},
			Post: []string{"res"},
		},
	},

	INT2FLOAT: {
		Opcode:      0x1863,
		Description: "Convert integer int into a float and push it onto the stack.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"int"},
			Post: []string{"res"},
		},
	},

	INT2HEX: {
		Opcode:      0x1833,
		Description: "Convert integer int into a hexstring and push it onto the\nstack. len supplies a minimum length for the resulting string.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"int", "len"},
			Post: []string{"res"},
		},
	},

	INT2OCT: {
		Opcode:      0x1843,
		Description: "Convert integer int into an octetstring and push it onto the\nstack. len supplies a minimum length for the resulting string.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"int", "len"},
			Post: []string{"res"},
		},
	},

	INT2STR: {
		Opcode:      0x1853,
		Description: "Convert integer int into a charstring and push it onto the\nstack.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"int"},
			Post: []string{"res"},
		},
	},

	INTEGER: {
		Opcode:      0x0e63,
		Description: "Push the built-in integer type object.\n\n",
		Context:     []string{"type_ref"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"obj"},
		},
	},

	INTERLEAVE: {
		Opcode:      0x0393,
		Description: "Evaluate the contents of blk until all of the contained\nalternatives succeed.\n\nAn empty block is allowed: activated defaults will still be\nincluded in the operation.\n\nActions attached to successful alternatives may extend the set\nof alternatives to complete.\n\n",
		Context:     []string{"eval"},
		Operations: Operation{
			Pre:  []string{"blk"},
			Post: []string{},
		},
	},

	INTERLEAVESTEP: {
		Opcode:      0x2043,
		Description: "Define an interleavestep object from a block pblk containing its\nparameters, bblk containing its behaviour and a name, and then\nattach it to the module object owning the current block.\n\n",
		Context:     []string{"beha_def"},
		Operations: Operation{
			Pre:  []string{"pblk", "bblk", "name"},
			Post: []string{},
		},
	},

	INTERLEAVESTEPB: {
		Opcode:      0x2053,
		Description: "Define a bound interleavestep object from a component type comp, a\nblock pblk containing its parameters, bblk containing its\nbehaviour and a name, then attach it to the module object\nowning the current block.\n\n",
		Context:     []string{"beha_def"},
		Operations: Operation{
			Pre:  []string{"comp", "pblk", "bblk", "name"},
			Post: []string{},
		},
	},

	INTERLEAVESTEPBW: {
		Opcode:      0x2073,
		Description: "Define a bound interleavestep object from a component type comp, a\nblock pblk containing its parameters, bblk containing its\nbehaviour and a name, attach the attributes from block ablk,\nand then attach it to the module object owning the current\nblock.\n\n\n",
		Context:     []string{"beha_def"},
		Operations: Operation{
			Pre:  []string{"comp", "ablk", "pblk", "bblk", "name"},
			Post: []string{},
		},
	},

	INTERLEAVESTEPW: {
		Opcode:      0x2063,
		Description: "Define an interleavestep object from a block pblk containing its\nparameters, bblk containing its behaviour and a name, attach\nthe attributes from block ablk, and then attach it to the\nmodule object owning the current block.\n\n",
		Context:     []string{"beha_def"},
		Operations: Operation{
			Pre:  []string{"ablk", "pblk", "bblk", "name"},
			Post: []string{},
		},
	},

	ISBOUND: {
		Opcode:      0x1ad3,
		Description: "Stack operations might be dynamic.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{},
		},
	},

	ISCHOSEN: {
		Opcode:      0x19f3,
		Description: "Tests the object obj to determine if it is the selected field\nof a union and pushes the boolean result. obj must reference a\nfield of a union.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"obj"},
			Post: []string{"res"},
		},
	},

	ISPRESENT: {
		Opcode:      0x19e3,
		Description: "Tests the presence of object obj and pushes the boolean\nresult. obj must reference an optional field of a record or\nset.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"obj"},
			Post: []string{"res"},
		},
	},

	ISTR: {
		Opcode:      0x0163,
		Arg:         ArgString,
		Description: "Push an integer literal onto the stack, with the integer coded\nas an ASCII string to allow for (nearly) arbitrary size. This\nis a variable-length instruction with the details of the\nliteral string following the instruction itself: first, a\nlength field, and then the bytes of the string.\n\nThe length field is encoded with the same byte order as has\nbeen selected for instructions. The string itself is always\nencoded with the first byte of the string at the lowest file\noffset – the natural ordering for strings.\n\nPadding shall be placed at the end of the string bytes to\nrestore the correct alignment for following instructions.\n\n",
		Context:     []string{"literal", "multibyte"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"lit"},
		},
	},

	ISVALUE: {
		Opcode:      0x1a63,
		Description: "Tests the object obj to determine is it is concrete value\n(defined, not a template) and pushes the boolean result.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"obj"},
			Post: []string{"res"},
		},
	},

	JNT: {
		Opcode:      0x2593,
		Arg:         ArgImm,
		Description: "Jumps over next instruction if the boolean on the stack is not true.\n\n",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{"cond"},
			Post: []string{},
		},
	},

	JT: {
		Opcode:      0x2583,
		Arg:         ArgImm,
		Description: "Jumps over next instruction if the boolean on the stack is true.\n\n",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{"cond"},
			Post: []string{},
		},
	},

	KILL: {
		Opcode:      0x13c3,
		Description: "Stop behaviour on component comp and move it to the killed\nstate.\n\n",
		Context:     []string{"component_op"},
		Operations: Operation{
			Pre:  []string{"comp"},
			Post: []string{},
		},
	},

	KILLA: {
		Opcode:      0x13d3,
		Description: "Stop behaviour on all parallel test components, moving them to\nthe killed state.\n\n",
		Context:     []string{"component_op", "all"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{},
		},
	},

	KILLED: {
		Opcode:      0x1243,
		Description: "Test the state of component comp for the killed condition,\ncontributing this to an enclosing alt or interleave statement.\n\n",
		Context:     []string{"component_op"},
		Operations: Operation{
			Pre:  []string{"comp"},
			Post: []string{},
		},
	},

	KILLED1: {
		Opcode:      0x1253,
		Description: "Test the state of all parallel test components for any that\nare killed, contributing this to an enclosing alt or\ninterleave statement.\n\n",
		Context:     []string{"component_op", "any"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{},
		},
	},

	KILLEDA: {
		Opcode:      0x1263,
		Description: "Test the state of all parallel test components checking that\nall are killed, contributing this to an enclosing alt or\ninterleave statement.\n\n",
		Context:     []string{"component_op", "all"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{},
		},
	},

	LE: {
		Opcode:      0x0533,
		Description: "Evaluate a <= b, pushing the boolean result.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"a", "b"},
			Post: []string{"res"},
		},
	},

	LENGTH: {
		Opcode:      0x0643,
		Description: "Create a length restriction with length len and apply to obj,\nleaving the result on the stack. len may be a range\nconstructed with the range operation.\n\n",
		Context:     []string{"value"},
		Operations: Operation{
			Pre:  []string{"obj", "len"},
			Post: []string{"obj"},
		},
	},

	LENGTHOF: {
		Opcode:      0x19b3,
		Description: "Takes the length of the string-type object str and pushes it\nonto the stack.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"str"},
			Post: []string{"len"},
		},
	},

	LOAD: {
		Opcode:      0x0053,
		Arg:         ArgInt,
		Description: "Loads a local object indexed by x, where x is an unsigned\ninteger value in the range 0 to 32767.\n\n",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"obj"},
		},
	},

	LOG: {
		Opcode:      0x1013,
		Description: "Log the items in collection coll or log the single object obj.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"obj"},
			Post: []string{},
		},
	},

	LT: {
		Opcode:      0x0543,
		Description: "Evaluate a < b, pushing the boolean result.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"a", "b"},
			Post: []string{"res"},
		},
	},

	MAP: {
		Opcode:      0x10c3,
		Description: "Map port p1 to port p2.\n\n",
		Context:     []string{"port_op"},
		Operations: Operation{
			Pre:  []string{"p1", "p2"},
			Post: []string{},
		},
	},

	MAPT: {
		Opcode:      0x0fc3,
		Description: "Construct a map type object with key typek and value typev\nand push on to the stack.\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"typek", "typev"},
			Post: []string{"obj"},
		},
	},

	MARK: {
		Opcode:      0x00c3,
		Description: "Push a mark object onto the stack.\n\n",
		Context:     []string{"literal"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"mark"},
		},
	},

	MATCH: {
		Opcode:      0x1023,
		Description: "Perform template matching of expression expr against template\ntmpl returning a boolean result on the stack.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"expr", "tmpl"},
			Post: []string{"res"},
		},
	},

	MOD: {
		Opcode:      0x0463,
		Description: "Evaluate a mod b, pushing the result on the stack. The TTCN-3\ndefinition of mod applies.\n\nThe types of a and b must be compatible. The returned object\nhas the base type of the input parameters and is constant.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"a", "b"},
			Post: []string{"res"},
		},
	},

	MODULE: {
		Opcode:      0x08e3,
		Description: "Define a module object and attach it to the source object\nowning the current block. Parameters are a block dblk of\nsub-definitions and the group name.\n\n",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{"bblk", "name"},
			Post: []string{},
		},
	},

	MODULEW: {
		Opcode:      0x08f3,
		Description: "Define a module object with attributes.\n\n",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{"ablk", "bblk", "name"},
			Post: []string{},
		},
	},

	MOVE: {
		Opcode:      0x05e3,
		Description: "Move b to a\n\n",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"b", "a"},
			Post: []string{},
		},
	},

	MPAR: {
		Opcode:      0x0a03,
		Arg:         ArgImm,
		Description: "Create a module parameter definition from its type and a name.\n\n",
		Context:     []string{"decl"},
		Operations: Operation{
			Pre:  []string{"type", "name"},
			Post: []string{},
		},
	},

	MPARD: {
		Opcode:      0x0a13,
		Arg:         ArgImm,
		Description: "Create a module parameter definition from a block vblk\ncontaining the statements of its default value, its type and a\nname.\n\n",
		Context:     []string{"decl"},
		Operations: Operation{
			Pre:  []string{"vblk", "type", "name"},
			Post: []string{},
		},
	},

	MTC: {
		Opcode:      0x0213,
		Description: "Fetch a reference to the current MTC and push it on the stack.\n\n",
		Context:     []string{"testcase_op"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"obj"},
		},
	},

	MUL: {
		Opcode:      0x0443,
		Description: "Evaluate a * b, pushing the result on the stack.\n\nThe types of a and b must be compatible. The returned object\nhas the base type of the input parameters and is constant.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"a", "b"},
			Post: []string{"res"},
		},
	},

	NAME: {
		Opcode:      0x0183,
		Arg:         ArgString,
		Description: "Push an object name literal onto the stack, coded as an ASCII\nstring. This is a variable-length instruction with the details\nof the name following the instruction itself: first, a length\nfield, and then the bytes of the string.\n\nThe length field is encoded with the same byte order as has\nbeen selected for instructions. The string itself is always\nencoded with the first byte of the string at the lowest file\noffset – the natural ordering for strings.\n\nPadding shall be placed at the end of the string bytes to\nrestore the correct alignment for following instructions.\n\n",
		Context:     []string{"literal", "multibyte"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"lit"},
		},
	},

	NATLONG: {
		Opcode:      0x0143,
		Arg:         ArgInt,
		Description: "Push a signed integer literal onto the stack. This is a\nvariable-length instruction with the value of the literal\ninteger following the instruction itself as a native long.\n\nThe literal integer value is encoded at the same width and\nbyte ordering as is currently selected for instructions: is\nmust be swapped when swapping is enabled.\n\nIf the currently selected instruction width is too small for\nthe encoding of a particular value then the istr literal\noperation shall be used instead.\n\n",
		Context:     []string{"literal", "multibyte"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"lit"},
		},
	},

	NE: {
		Opcode:      0x0553,
		Description: "Evaluate a != b, pushing the boolean result.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"a", "b"},
			Post: []string{"res"},
		},
	},

	NEG: {
		Opcode:      0x0423,
		Description: "Evaluate -a (unary - operator) pushing the result.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"a"},
			Post: []string{"res"},
		},
	},

	NIBBLES: {
		Opcode:      0x0123,
		Arg:         ArgNibbles,
		Description: "Encoding of 9 nibbles, showing nibble endian-ness:\n‘BADC0FFEE’H\n\nNote: 9th nibble in the upper half of most significant byte\n\n0000 0123\n0000 0009\nBADC 0FFE\nE000 0000\n\n",
		Context:     []string{"literal", "multibyte"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"lit"},
		},
	},

	NONE: {
		Opcode:      0x02a3,
		Description: "Push a special literal on the stack corresponding to the none\nterm of verdicttype.\n\n",
		Context:     []string{"literal"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"lit"},
		},
	},

	NOP: {
		Opcode:      0x0003,
		Description: "No operation.\n\nnop shall normally be combined with eswap. Refer to the eswap\noperation.\n\n",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{},
		},
	},

	NOT: {
		Opcode:      0x0493,
		Description: "With boolean input, perform the logical not operation and\nreturn the boolean result.\n\nWith bitstring, hexstring or octetstring input, perform the\nbitwise not operation and return the result. The result type\nwill be of the same base type as the inputs.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"a"},
			Post: []string{"res"},
		},
	},

	NOW: {
		Opcode:      0x1343,
		Description: "",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"float"},
		},
	},

	NULL: {
		Opcode:      0x0203,
		Description: "",
		Context:     []string{"literal"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"nullref"},
		},
	},

	OCT2BIT: {
		Opcode:      0x1943,
		Description: "Convert octetstring oct into a bitstring and push it onto the\nstack.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"oct"},
			Post: []string{"res"},
		},
	},

	OCT2CHR: {
		Opcode:      0x1973,
		Description: "",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"oct"},
			Post: []string{"charstring"},
		},
	},

	OCT2HEX: {
		Opcode:      0x1953,
		Description: "Convert octetstring oct into a hexstring and push it onto the\nstack.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"oct"},
			Post: []string{"res"},
		},
	},

	OCT2INT: {
		Opcode:      0x1933,
		Description: "Convert octetstring oct into an integer and push it onto the\nstack.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"oct"},
			Post: []string{"res"},
		},
	},

	OCT2STR: {
		Opcode:      0x1963,
		Description: "Convert octetstring oct into a charstring and push it onto the\nstack.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"oct"},
			Post: []string{"res"},
		},
	},

	OCTETS: {
		Opcode:      0x0133,
		Arg:         ArgOctets,
		Description: "Push an octetstring literal onto the stack. This is a\nvariable-length instruction with the details of the literal\noctetstring following the instruction itself: first, a length\nfield, and then the bytes of the octetstring.\n\nThe length field is encoded with the same byte order as has\nbeen selected for instructions. The length contains the number\nof octets.\n\nThe string itself is always encoded with the first octet of\nthe string at the lowest file offset – the natural ordering\nfor strings.\n\nPadding shall be placed at the end of the string bytes to\nrestore the correct alignment for following instructions.\n\n",
		Context:     []string{"literal", "multibyte"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"lit"},
		},
	},

	OCTETSTRING: {
		Opcode:      0x0e73,
		Description: "Push the built-in octetstring type object.\n\n",
		Context:     []string{"type_ref"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"obj"},
		},
	},

	OMIT: {
		Opcode:      0x0253,
		Description: "Push a special omit literal onto the stack.\n\n",
		Context:     []string{"literal"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"lit"},
		},
	},

	OR: {
		Opcode:      0x04a3,
		Description: "With a boolean first argument perform the logical or operation\nand return the boolean result. The second argument shall be\neither another boolean or a block. If the first argument is\ntrue then the second argument shall not be evaluated.\n\nWith bitstring, hexstring or octetstring inputs, perform the\nbitwise or operation and return the result. The result type\nwill be of the same base type as the inputs.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"obj", "obj"},
			Post: []string{"res"},
		},
	},

	OUT: {
		Opcode:      0x0763,
		Description: "Bind a type and name into an output value parameter object and\nthen attach this to the owning function (altstep, testcase)\nobject.\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"type", "name"},
			Post: []string{},
		},
	},

	PASS: {
		Opcode:      0x02b3,
		Description: "Push a special literal on the stack corresponding to the pass\nterm of verdicttype.\n\n",
		Context:     []string{"literal"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"lit"},
		},
	},

	PATTERN: {
		Opcode:      0x0653,
		Description: "Make string str into a pattern template literal and push this\non the stack.\n\nIn addition to the obvious use with charstring and universal\ncharstring types, pattern shall be used for bitstring,\nhexstring and octetstring literals when they contain\npattern-matching characters.\n\n",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{"str"},
			Post: []string{"pat"},
		},
	},

	PERMITO: {
		Opcode:      0x0783,
		Description: "Modifies a type object to allow it to have the special omit\nvalue.\n\n",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{"type"},
			Post: []string{"type"},
		},
	},

	PERMITP: {
		Opcode:      0x07a3,
		Description: "Modifies a type object to allow it to have ifpresent values.\n\n",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{"type"},
			Post: []string{"type"},
		},
	},

	PERMITT: {
		Opcode:      0x0793,
		Description: "Modifies a type object to allow it to have template values.\n\n",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{"type"},
			Post: []string{"type"},
		},
	},

	PERMUTATION: {
		Opcode:      0x0663,
		Description: "Make a permutation template literal from the input value list\nvlist. Push the resulting object on the stack.\n\n",
		Context:     []string{"value"},
		Operations: Operation{
			Pre:  []string{"vlist"},
			Post: []string{"res"},
		},
	},

	PORTM: {
		Opcode:      0x0f43,
		Description: "Construct a message port type object push onto the stack.\nBlock iblk shall contain a list of types allowed in through\nthe port and block oblk shall contain a list of types allowed\nout through the port. The same type may appear in both lists.\n\nRefer to the TTCN-3 specification for the precise meaning of\nthe terms in and out under various circumstances.\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"iblk", "oblk"},
			Post: []string{"ptype"},
		},
	},

	PORTMA: {
		Opcode:      0x0f63,
		Description: "Create message port type\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"in", "out", "address", "map_params", "unmap_params"},
			Post: []string{},
		},
	},

	RANGE: {
		Opcode:      0x0673,
		Description: "Construct a range template literal from bounds lower and\nupper. Push the resulting object on the stack.\n\n",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{"lower", "upper"},
			Post: []string{"obj"},
		},
	},

	READ: {
		Opcode:      0x1303,
		Description: "Read the remaining duration of timer tmr and push the float\nresult onto the stack.\n\n",
		Context:     []string{"timer_op"},
		Operations: Operation{
			Pre:  []string{"tmr"},
			Post: []string{"rem"},
		},
	},

	RECEIVE: {
		Opcode:      0x1443,
		Description: "Create a receive condition on port port. Block mblk shall\ncontain code to perform the matching of input. When the\ncondition is true (matched) then the enclosing alt will\nexecute block ablk to perform variable assignments.\n\n",
		Context:     []string{"port_op"},
		Operations: Operation{
			Pre:  []string{"mblk", "ablk", "port"},
			Post: []string{},
		},
	},

	RECEIVE1: {
		Opcode:      0x1463,
		Description: "Receive from any port\n\n",
		Context:     []string{"port_op"},
		Operations: Operation{
			Pre:  []string{"match", "assign"},
			Post: []string{},
		},
	},

	RECEIVEC: {
		Opcode:      0x1453,
		Description: "Create a checked receive condition on port port. Block mblk\nshall contain code to perform the matching of input. When the\ncondition is true (matched) then the enclosing alt will\nexecute block ablk to perform variable assignments.\n\n",
		Context:     []string{"port_op"},
		Operations: Operation{
			Pre:  []string{"mblk", "ablk", "port"},
			Post: []string{},
		},
	},

	RECEIVEC1: {
		Opcode:      0x1473,
		Description: "",
		Context:     []string{"port_op", "any"},
		Operations: Operation{
			Pre:  []string{"match", "assign"},
			Post: []string{},
		},
	},

	RECORD: {
		Opcode:      0x0f83,
		Description: "Construct a record object from field definitions in block fblk\nand push on to the stack.\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"fblk"},
			Post: []string{"obj"},
		},
	},

	RECORDOF: {
		Opcode:      0x0fa3,
		Description: "Construct a record-of object with element of type type and\npush on to the stack.\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"type"},
			Post: []string{"obj"},
		},
	},

	REF_BIT2HEX: {
		Opcode:      0x1dc3,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_BIT2INT: {
		Opcode:      0x1db3,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_BIT2OCT: {
		Opcode:      0x1dd3,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_BIT2STR: {
		Opcode:      0x1de3,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_CHAR2INT: {
		Opcode:      0x1d83,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_CHAR2OCT: {
		Opcode:      0x1d93,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_DECVALUE: {
		Opcode:      0x1f83,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_ENCVALUE: {
		Opcode:      0x1f73,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_ENUM2INT: {
		Opcode:      0x1f53,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_FLOAT2INT: {
		Opcode:      0x1d73,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_HEX2BIT: {
		Opcode:      0x1e03,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_HEX2INT: {
		Opcode:      0x1df3,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_HEX2OCT: {
		Opcode:      0x1e13,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_HEX2STR: {
		Opcode:      0x1e23,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_INT2BIT: {
		Opcode:      0x1d23,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_INT2CHAR: {
		Opcode:      0x1d03,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_INT2ENUM: {
		Opcode:      0x1fa3,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_INT2FLOAT: {
		Opcode:      0x1d63,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_INT2HEX: {
		Opcode:      0x1d33,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_INT2OCT: {
		Opcode:      0x1d43,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_INT2STR: {
		Opcode:      0x1d53,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_ISBOUND: {
		Opcode:      0x1fd3,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_ISPRESENT: {
		Opcode:      0x1ee3,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_ISVALUE: {
		Opcode:      0x1f63,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_OCT2BIT: {
		Opcode:      0x1e43,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_OCT2CHR: {
		Opcode:      0x1e73,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_OCT2HEX: {
		Opcode:      0x1e53,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_OCT2INT: {
		Opcode:      0x1e33,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_OCT2STR: {
		Opcode:      0x1e63,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_REGEXP: {
		Opcode:      0x1f03,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_STR2FLOAT: {
		Opcode:      0x1ea3,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_STR2HEX: {
		Opcode:      0x1fc3,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_STR2INT: {
		Opcode:      0x1e83,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_STR2OCT: {
		Opcode:      0x1e93,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_TESTCASENAME: {
		Opcode:      0x1f93,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REF_VAL2STR: {
		Opcode:      0x1ed3,
		Description: "",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ref"},
			Post: []string{},
		},
	},

	REGEXP: {
		Opcode:      0x1a03,
		Description: "Perform a regular expression match of str against expr.\nReturns the gno-th matching bracketed sub-expression or an\nempty string if no match.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"str", "expr", "gno"},
			Post: []string{"res"},
		},
	},

	REM: {
		Opcode:      0x0473,
		Description: "Evaluate a rem b, pushing the result on the stack. The TTCN-3\ndefinition of rem applies.\n\nThe types of a and b must be compatible. The returned object\nhas the base type of the input parameters and is constant.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"a", "b"},
			Post: []string{"res"},
		},
	},

	REPEAT: {
		Opcode:      0x03b3,
		Description: "Cause the immediate repeat of the evaluation of the current\nenclosing alt.\n\n",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{},
		},
	},

	REPLACE: {
		Opcode:      0x1a23,
		Description: "Replace the substring of str, from index idx, for length len,\nwith the string rep and return the resulting string.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"str", "idx", "len", "rep"},
			Post: []string{"res"},
		},
	},

	RETURN: {
		Opcode:      0x0323,
		Description: "Causes immediate exit from the current function, altstep,\ntestcase or control.\n\n",
		Context:     []string{"vm", "control_flow"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{},
		},
	},

	RND: {
		Opcode:      0x1a33,
		Description: "Compute a random float in the range 0 and 1 exclusive, and\nreturn the result.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"num"},
		},
	},

	ROL: {
		Opcode:      0x04e3,
		Description: "Rotate string str left by num items, and return the result.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"str", "num"},
			Post: []string{"res"},
		},
	},

	ROR: {
		Opcode:      0x04f3,
		Description: "Rotate string str right by num items, and return the result.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"str", "num"},
			Post: []string{"res"},
		},
	},

	RUNNING: {
		Opcode:      0x1203,
		Description: "When called with a component reference comp, evaluate the\nrunning state of the component and push the boolean result\nonto the stack.\n\nWhen called with a timer reference tmr, evaluate the running\nstate of the timer and push the boolean result onto the stack.\n\n",
		Context:     []string{"timer_op", "component_op"},
		Operations: Operation{
			Pre:  []string{"obj"},
			Post: []string{"res"},
		},
	},

	RUNNING1C: {
		Opcode:      0x1213,
		Description: "Evaluate the state of parallel test components to determine if\nany are in the running state. Push the boolean result on the\nstack.\n\n",
		Context:     []string{"component_op", "timer_op", "any"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"res"},
		},
	},

	RUNNING1T: {
		Opcode:      0x1313,
		Description: "Evaluate the state of timers on the current component to\ndetermine if any are in the running state. Push the boolean\nresult on the stack.\n\n",
		Context:     []string{"timer_op", "any"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"res"},
		},
	},

	RUNNINGAC: {
		Opcode:      0x1223,
		Description: "Evaluate the state of parallel test components to determine if\nall are in the running state. Push the boolean result on the\nstack.\n\n",
		Context:     []string{"component_op", "timer_op", "all"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"res"},
		},
	},

	SCAN: {
		Opcode:      0x00d3,
		Description: "Switch from instruction execution to scanning until a matching\nblock is seen.\n\n",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"blk"},
		},
	},

	SELF: {
		Opcode:      0x0223,
		Description: "Fetch a reference to the current component and push it on the\nstack.\n\n",
		Context:     []string{"testcase_op"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"obj"},
		},
	},

	SEND: {
		Opcode:      0x1403,
		Description: "Send message msg on port port with no explicit addressing.\n\n",
		Context:     []string{"port_op"},
		Operations: Operation{
			Pre:  []string{"msg", "port"},
			Post: []string{},
		},
	},

	SEND1: {
		Opcode:      0x1413,
		Description: "Send message msg on port port to single destination dest\n(unicast.)\n\n",
		Context:     []string{"port_op"},
		Operations: Operation{
			Pre:  []string{"dest", "msg", "port"},
			Post: []string{},
		},
	},

	SENDA: {
		Opcode:      0x1433,
		Description: "Send message msg on port port to all associated destinations\n(broadcast.)\n\n",
		Context:     []string{"port_op"},
		Operations: Operation{
			Pre:  []string{"msg", "port"},
			Post: []string{},
		},
	},

	SENDER: {
		Opcode:      0x17e3,
		Description: "Fetch the sender (from) address of the current communication\noperation.\n\n",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"obj"},
		},
	},

	SENDN: {
		Opcode:      0x1423,
		Description: "Send message msg on port port to all destinations in block\ndblk (multicast.)\n\n",
		Context:     []string{"port_op"},
		Operations: Operation{
			Pre:  []string{"dblk", "msg", "port"},
			Post: []string{},
		},
	},

	SET: {
		Opcode:      0x0f93,
		Description: "Construct a set object from field definitions in block fblk\nand push on to the stack.\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"fblk"},
			Post: []string{"obj"},
		},
	},

	SETOF: {
		Opcode:      0x0fb3,
		Description: "Construct a set-of object with element of type type and push\non to the stack.\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"type"},
			Post: []string{"obj"},
		},
	},

	SETVERDICT: {
		Opcode:      0x1053,
		Description: "Set the local verdict of the current component from parameter\nlver. Use within the control part is a runtime error.\n\n",
		Context:     []string{"component_op"},
		Operations: Operation{
			Pre:  []string{"lver"},
			Post: []string{},
		},
	},

	SHL: {
		Opcode:      0x04c3,
		Description: "Shift string str left by num items, and return the result.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"str", "num"},
			Post: []string{"res"},
		},
	},

	SHR: {
		Opcode:      0x04d3,
		Description: "Shift string str right by num items, and return the result.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"str", "num"},
			Post: []string{"res"},
		},
	},

	SIZEOF: {
		Opcode:      0x19c3,
		Description: "Pushes the number of elements in obj onto the stack as an\ninteger value.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"obj"},
			Post: []string{"size"},
		},
	},

	SKIP: {
		Opcode:      0x0243,
		Description: "Push a special literal onto the stack, corresponding to the\nTTCN-3 ‘-‘ and meaning ‘skip this item.’\n\nAs an optimisation, skip may be used instead of an empty\nblock, saving one operation and avoid invocation of the block\nscanning mechanism.\n\n",
		Context:     []string{"literal"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"lit"},
		},
	},

	SMATCH: {
		Opcode:      0x00f3,
		Description: "Silent match\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"expr", "tmpl"},
			Post: []string{"bool"},
		},
	},

	SOURCE: {
		Opcode:      0x08d3,
		Description: "Define a source file object with file name name and module\ndefinitions contained in block mblk.\n\n",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{"mblk", "name"},
			Post: []string{},
		},
	},

	SPECPLC: {
		Opcode:      0x0d43,
		Description: "Special place\n\n",
		Context:     []string{"component_op"},
		Operations: Operation{
			Pre:  []string{"blk"},
			Post: []string{},
		},
	},

	START: {
		Opcode:      0x1283,
		Description: "When called with a port reference port, start activity on the\nport.\n\nWhen called with a timer reference tmr, start the timer\nrunning (or restart.)\n\n",
		Context:     []string{"port_op", "timer_op"},
		Operations: Operation{
			Pre:  []string{"obj"},
			Post: []string{},
		},
	},

	STARTAP: {
		Opcode:      0x12b3,
		Description: "Start activity on all ports.\n\n",
		Context:     []string{"port_op", "all"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{},
		},
	},

	STARTC: {
		Opcode:      0x1293,
		Description: "Start behaviour on parallel test component comp. Block blk\ncontains the instructions to execute in the context of the\nnewly started component – the behaviour. This shall take the\nform of any parameters followed by a function call.\n\n",
		Context:     []string{"component_op"},
		Operations: Operation{
			Pre:  []string{"blk", "comp"},
			Post: []string{},
		},
	},

	STARTD: {
		Opcode:      0x12a3,
		Description: "Start timer tmr, with explicit duration dur.\n\n",
		Context:     []string{"timer_op"},
		Operations: Operation{
			Pre:  []string{"dur", "tmr"},
			Post: []string{},
		},
	},

	STEP: {
		Opcode:      0x03e3,
		Description: "Create an alternative for an enclosing alt operation,\nassociating some condition with statements to perform should\nthe condition be chosen by the alt operation. Block cblk\ncontains code to execute to evaluate the condition of the\nalternative. Block sblk contains code to execute should be\ncondition be chosen.\n\n",
		Context:     []string{"eval"},
		Operations: Operation{
			Pre:  []string{"cblk", "sblk"},
			Post: []string{},
		},
	},

	STOP: {
		Opcode:      0x12c3,
		Description: "When called with a component reference comp, stop behaviour on\nthe component.\n\nWhen called with a port reference port, stop activity on the\nport.\n\nWhen called with a timer reference tmr, stop the timer.\n\n",
		Context:     []string{"timer_op", "component_op", "port_op"},
		Operations: Operation{
			Pre:  []string{"obj"},
			Post: []string{},
		},
	},

	STOPAC: {
		Opcode:      0x12d3,
		Description: "Stop all parallel test components. Permitted only from the\nmaster test component.\n\n",
		Context:     []string{"component_op"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{},
		},
	},

	STOPAP: {
		Opcode:      0x12e3,
		Description: "Stop all ports.\n\n",
		Context:     []string{"port_op", "all"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{},
		},
	},

	STOPAT: {
		Opcode:      0x12f3,
		Description: "Stop all timers on the current component.\n\n",
		Context:     []string{"timer_op", "all"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{},
		},
	},

	STOPI: {
		Opcode:      0x0333,
		Description: "From within the behaviour of a component, causes immediate\ntermination of the behaviour of that component.\n\nFrom within the behaviour of a control part, causes immediate\ntermination of the execution of that control part.\n\n",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{},
		},
	},

	STORE: {
		Opcode:      0x0063,
		Description: "Stores a local object indexed by x, where x in an unsigned\ninteger value in the range 0 to 32767.\n\n",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"obj"},
			Post: []string{},
		},
	},

	STR2FLOAT: {
		Opcode:      0x19a3,
		Description: "Convert charstring char into a float, and push it onto the\nstack.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"char"},
			Post: []string{"res"},
		},
	},

	STR2HEX: {
		Opcode:      0x1ac3,
		Description: "",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"string"},
			Post: []string{"hex"},
		},
	},

	STR2INT: {
		Opcode:      0x1983,
		Description: "Convert charstring char into an integer, and push it onto the\nstack.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"char"},
			Post: []string{"res"},
		},
	},

	STR2OCT: {
		Opcode:      0x1993,
		Description: "Convert charstring char into an octetstring, and push it onto\nthe stack.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"char"},
			Post: []string{"res"},
		},
	},

	SUB: {
		Opcode:      0x0433,
		Description: "Evaluate a – b, pushing the result on the stack.\n\nThe types of a and b must be compatible. The returned object\nhas the base type of the input parameters and is constant.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"a", "b"},
			Post: []string{"res"},
		},
	},

	SUBSET: {
		Opcode:      0x0683,
		Description: "Make a subset template literal from the input value list\nvlist. Push the resulting object on the stack.\n\n",
		Context:     []string{"value"},
		Operations: Operation{
			Pre:  []string{"vlist"},
			Post: []string{"res"},
		},
	},

	SUBSTR: {
		Opcode:      0x1a13,
		Description: "Return the substring of str, from index idx, for length len.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"str", "idx", "len"},
			Post: []string{"res"},
		},
	},

	SUBTYPE: {
		Opcode:      0x0ef3,
		Description: "Create a subtype from type applying constraint cons, pushing\nthe resulting type object on the stack. Constraint cons may be\na single object or a collection of objects.\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"type", "cons"},
			Post: []string{"res"},
		},
	},

	SUPERSET: {
		Opcode:      0x0693,
		Description: "Make a superset template literal from the input value list\nvlist. Push the resulting object on the stack.\n\n",
		Context:     []string{"value"},
		Operations: Operation{
			Pre:  []string{"vlist"},
			Post: []string{"res"},
		},
	},

	SYSTEM: {
		Opcode:      0x0233,
		Description: "Fetch a reference to the current test system interface\ncomponent and push it on the stack.\n\n",
		Context:     []string{"testcase_op"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"obj"},
		},
	},

	TCSTOP: {
		Opcode:      0x11e3,
		Description: "Stop testcase.\n\n",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"reason"},
			Post: []string{},
		},
	},

	TEMPLATE: {
		Opcode:      0x0a43,
		Arg:         ArgImm,
		Description: "Define a template object from return type, a block pblk\ncontaining its parameters, bblk containing its behaviour and a\nname, and then attach it to the module object owning the\ncurrent block.\n\n",
		Context:     []string{"beha_def"},
		Operations: Operation{
			Pre:  []string{"type", "pblk", "bblk", "name"},
			Post: []string{},
		},
	},

	TERM: {
		Opcode:      0x0713,
		Description: "Bind an integer value and name into an enumeration term\ndefinition and then attach this to the owning enumeration\nobject.\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"tval", "name"},
			Post: []string{},
		},
	},

	TESTCASE: {
		Opcode:      0x0a83,
		Arg:         ArgImm,
		Description: "Define a testcase object from a runs-on component type rcomp,\na block pblk containing its parameters, bblk containing its\nbehaviour and a name, and then attach it to the module object\nowning the current block.\n\n",
		Context:     []string{"beha_def"},
		Operations: Operation{
			Pre:  []string{"rcomp", "pblk", "bblk", "name"},
			Post: []string{},
		},
	},

	TESTCASENAME: {
		Opcode:      0x1a93,
		Description: "Retrieve the current testcase name and push onto the stack.\nReturns an empty string if called outside any executing\ntestcase.\n\n",
		Context:     []string{"testcase_op"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"name"},
		},
	},

	TESTCASES: {
		Opcode:      0x0a93,
		Arg:         ArgImm,
		Description: "Define a testcase object from a system component type scomp, a\nruns-on component type rcomp, a block pblk containing its\nparameters, bblk containing its behaviour and a name, and then\nattach it to the module object owning the current block.\n\n",
		Context:     []string{"beha_def"},
		Operations: Operation{
			Pre:  []string{"scomp", "rcomp", "pblk", "bblk", "name"},
			Post: []string{},
		},
	},

	TESTCASESW: {
		Opcode:      0x0ab3,
		Description: "Define a testcase object from a system component type scomp, a\nruns-on component type rcomp, a block pblk containing its\nparameters, bblk containing its behaviour and a name, and then\nattach it to the module object owning the current block.\nAttach attributes from block ablk.\n\n",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ablk", "scomp", "rcomp", "pblk", "bblk", "name"},
			Post: []string{},
		},
	},

	TESTCASEW: {
		Opcode:      0x0aa3,
		Description: "Define a testcase object from a runs-on component type rcomp,\na block pblk containing its parameters, bblk containing its\nbehaviour and a name, and then attach it to the module object\nowning the current block. Attach attributes from block ablk.\n\n",
		Context:     []string{},
		Operations: Operation{
			Pre:  []string{"ablk", "rcomp", "pblk", "bblk", "name"},
			Post: []string{},
		},
	},

	TIMEOUT: {
		Opcode:      0x1323,
		Description: "Create a timeout alternative bound to timer tmr and attach\nthis to the enclosing alt operation.\n\n",
		Context:     []string{"timer_op"},
		Operations: Operation{
			Pre:  []string{"tmr"},
			Post: []string{},
		},
	},

	TIMEOUT1: {
		Opcode:      0x1333,
		Description: "Create a timeout alternative bound to the expiry of any\ncurrent timer and attach this to the enclosing alt operation.\n\n",
		Context:     []string{"timer_op", "any"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{},
		},
	},

	TIMER: {
		Opcode:      0x0ed3,
		Description: "Push the built-in timer type object.\n\n",
		Context:     []string{"type_ref"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"obj"},
		},
	},

	TIMESTAMP: {
		Opcode:      0x17f3,
		Description: "",
		Context:     []string{"port_op"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"timestamp_id"},
		},
	},

	TO: {
		Opcode:      0x06d3,
		Description: "Implements the TTCN-3 .to operation applied to mapv of \ntype: `type map from A to B MapT`. A setofToType is provided\nfor creation of the return value. \nReturn value is of type `set of B` containing all values\nfrom the map.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"mapv", "setofToType"},
			Post: []string{"res"},
		},
	},

	TRAP: {
		Opcode:      0x2bc3,
		Description: "Trap the current execution. This is used to debug or test the runtime. \n",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{},
		},
	},

	TRIGGER: {
		Opcode:      0x1483,
		Description: "Create a trigger condition on port port. Block mblk shall\ncontain code to perform the matching of input. When the\ntrigger condition is true (matched) then the enclosing alt\nwill execute block ablk to perform variable assignment.\n\n",
		Context:     []string{"port_op"},
		Operations: Operation{
			Pre:  []string{"mblk", "ablk", "port"},
			Post: []string{},
		},
	},

	TRIGGER1: {
		Opcode:      0x1493,
		Description: "",
		Context:     []string{"port_op", "any"},
		Operations: Operation{
			Pre:  []string{"match", "assign"},
			Post: []string{},
		},
	},

	TRUE: {
		Opcode:      0x0293,
		Description: "Push a special true literal onto the stack.\n\n",
		Context:     []string{"literal"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"lit"},
		},
	},

	TYPE: {
		Opcode:      0x0a63,
		Description: "Associate a type object and name into a type definition object\nand then attach this to the owning module or group definition.\n\n",
		Context:     []string{"type_def"},
		Operations: Operation{
			Pre:  []string{"obj", "name"},
			Post: []string{},
		},
	},

	TYPEW: {
		Opcode:      0x0a73,
		Description: "Associate a type object and name into a type definition object\nand then attach this to the owning module or group definition.\nAttributes from block ablk are attached.\n\n",
		Context:     []string{"type_def"},
		Operations: Operation{
			Pre:  []string{"ablk", "obj", "name"},
			Post: []string{},
		},
	},

	UNION: {
		Opcode:      0x0ff3,
		Description: "Construct a union object from field definitions in block fblk\nand push on to the stack.\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"fblk"},
			Post: []string{"obj"},
		},
	},

	UNMAP: {
		Opcode:      0x10d3,
		Description: "Break the mapping between port p1 and port p2.\n\n",
		Context:     []string{"port_op"},
		Operations: Operation{
			Pre:  []string{"p1", "p2"},
			Post: []string{},
		},
	},

	UNMAPA: {
		Opcode:      0x10e3,
		Description: "Break all mappings from component comp.\n\n",
		Context:     []string{"port_op", "all"},
		Operations: Operation{
			Pre:  []string{"comp"},
			Post: []string{},
		},
	},

	UNMAPAA: {
		Opcode:      0x10f3,
		Description: "Break all mappings on all components.\n\n",
		Context:     []string{"port_op", "component_op", "all"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{},
		},
	},

	UNMAPFROMTO: {
		Opcode:      0x0fd3,
		Description: "remove key from mapval.\n\n",
		Context:     []string{"value"},
		Operations: Operation{
			Pre:  []string{"mapval", "key"},
			Post: []string{},
		},
	},

	UTF8: {
		Opcode:      0x0103,
		Arg:         ArgString,
		Description: "Push a string literal onto the stack. This is a\nvariable-length instruction with the details of the literal\nstring following the instruction itself: first, a length\nfield, then the bytes of the string, encoded UTF-8.\n\nThe length field is encoded with the same byte order as has\nbeen selected for instructions. The string itself is always\nencoded with the first byte of the string at the lowest file\noffset – the natural ordering for strings.\n\nPadding shall be placed at the end of the string bytes to\nrestore the correct alignment for following instructions.\n\n",
		Context:     []string{"literal", "multibyte"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"lit"},
		},
	},

	VAL2STR: {
		Opcode:      0x1ae3,
		Description: "Return a printable representation of val.\n\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"val"},
			Post: []string{"string"},
		},
	},

	VALUE: {
		Opcode:      0x17c3,
		Description: "Fetch the value of the current communication operation.\n\nFor receive and trigger operations this is the current\nmessage.\n\nFor getreply this is the return value of the current\nprocedure.\n\nFor catch this is the exception of the current procedure.\n\n",
		Context:     []string{"port_op"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"obj"},
		},
	},

	VALUEOF: {
		Opcode:      0x1033,
		Description: "Check template tmpl. If it has a single value then return this\non the stack otherwise raise a runtime error.\n\n",
		Context:     []string{"value"},
		Operations: Operation{
			Pre:  []string{"tmpl"},
			Post: []string{"value"},
		},
	},

	VAR: {
		Opcode:      0x0703,
		Arg:         ArgImm,
		Description: "Create a variable with the given type and name. Bind this\nvariable to the current function (control, altstep, testcase.)\n\n",
		Context:     []string{"decl"},
		Operations: Operation{
			Pre:  []string{"type", "name"},
			Post: []string{},
		},
	},

	VARDUP: {
		Opcode:      0x06f3,
		Arg:         ArgImm,
		Description: "Create a variable with eager template evaluation.",
		Context:     []string{"decl"},
		Operations: Operation{
			Pre:  []string{"type", "init", "name"},
			Post: []string{},
		},
	},

	VARIANT: {
		Opcode:      0x0cc3,
		Description: "Attach the variant attribute val to the object owning the\ncurrent block.\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"val"},
			Post: []string{},
		},
	},

	VARIANTO: {
		Opcode:      0x0ce3,
		Description: "Attach the overriding variant attribute val to the object\nowning the current block.\n\n",
		Context:     []string{"type_new"},
		Operations: Operation{
			Pre:  []string{"val"},
			Post: []string{},
		},
	},

	VERDICTTYPE: {
		Opcode:      0x0e83,
		Description: "Push the built-in verdicttype type object.\n\n",
		Context:     []string{"type_ref"},
		Operations: Operation{
			Pre:  []string{},
			Post: []string{"obj"},
		},
	},

	VERSION: {
		Opcode:      0x0033,
		Description: "Specify the version number of t3xf to which this file\nconforms. Version numbers start from one.\n\n",
		Context:     []string{"beha_def"},
		Operations: Operation{
			Pre:  []string{"num"},
			Post: []string{},
		},
	},

	VLIST: {
		Opcode:      0x0073,
		Description: "Build a value list from a marked listed of objects,\npotentially empty. The objects and the mark are removed from\nthe stack and replaced with a value list object.\n\n",
		Context:     []string{"vm"},
		Operations: Operation{
			Pre:  []string{"mark", "object1", "..", "objectn"},
			Post: []string{"vlist"},
		},
	},

	WAIT: {
		Opcode:      0x1353,
		Description: "Wait for float seconds.",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"float"},
			Post: []string{},
		},
	},

	WHILE: {
		Opcode:      0x0353,
		Description: "Implements the TTCN-3 while statement using two block\narguments.\n\nBlock cond is evaluated to test the loop condition. If the\nblock yields a true value then block body is executed before\nlooping to evaluate the condition again.\n\nExecution terminates when the loop condition returns a false\nvalue.\n\n",
		Context:     []string{"vm", "control_flow"},
		Operations: Operation{
			Pre:  []string{"cond", "body"},
			Post: []string{},
		},
	},

	XOR: {
		Opcode:      0x04b3,
		Description: "With boolean inputs, perform the logical exclusive or\noperation and return the boolean result.\n\nWith bitstring, hexstring or octetstring inputs perform the\nbitwise exclusive or operation and return the result. The\nresult type will be of the same base type as the inputs.\n",
		Context:     []string{"func"},
		Operations: Operation{
			Pre:  []string{"a", "b"},
			Post: []string{"res"},
		},
	},
}

var opcodeStrings = map[Opcode]string{
	REF:        "ref",
	LINE:       "line",
	GOTO:       "goto",
	FROZEN_REF: "frozen_ref",
	CALL:       "call",
	ISCAN:      "iscan",

	ACTION:           "action",
	ACTIVATE:         "activate",
	ADD:              "add",
	ADDRESS:          "address",
	ALIVE:            "alive",
	ALIVE1:           "alive1",
	ALIVEA:           "alivea",
	ALLFROM:          "allfrom",
	ALLFROMP:         "allfromp",
	ALT:              "alt",
	ALTND:            "altnd",
	ALTSTEP:          "altstep",
	ALTSTEPB:         "altstepb",
	ALTSTEPBW:        "altstepbw",
	ALTSTEPW:         "altstepw",
	AND:              "and",
	ANY:              "any",
	ANYN:             "anyn",
	APPLY:            "apply",
	ARRAY:            "array",
	ASSIGN:           "assign",
	ASSIGND:          "assignd",
	AT_DEFAULT:       "at_default",
	BIT2HEX:          "bit2hex",
	BIT2INT:          "bit2int",
	BIT2OCT:          "bit2oct",
	BIT2STR:          "bit2str",
	BITS:             "bits",
	BITSTRING:        "bitstring",
	BLOCK:            "block",
	BOOLEAN:          "boolean",
	BREAK:            "break",
	CAT:              "cat",
	CHAR2INT:         "char2int",
	CHAR2OCT:         "char2oct",
	CHARSTRING:       "charstring",
	CHARSTRINGU:      "charstringu",
	CHECK:            "check",
	CHECK1:           "check1",
	CHECKSTATE:       "checkstate",
	CHECKSTATEAL:     "checkstateal",
	CHECKSTATEAN:     "checkstatean",
	CLEAR:            "clear",
	CLEARA:           "cleara",
	CLONE:            "clone",
	CLOSURE:          "closure",
	CLOSURETYPE:      "closuretype",
	COLLECT:          "collect",
	COMPLEMENT:       "complement",
	COMPONENT:        "component",
	COMPONENTX:       "componentx",
	CONNECT:          "connect",
	CONST:            "const",
	CONSTW:           "constw",
	CONTINUE:         "continue",
	CONTROL:          "control",
	CONTROLW:         "controlw",
	CREATE:           "create",
	CREATEA:          "createa",
	CREATEAN:         "createan",
	CREATEN:          "createn",
	DEACTIVATE:       "deactivate",
	DEACTIVATEA:      "deactivatea",
	DECMATCH:         "decmatch",
	DECVALUE:         "decvalue",
	DEF:              "def",
	DEFAULT:          "default",
	DISCONNECT:       "disconnect",
	DISCONNECTA:      "disconnecta",
	DISCONNECTAA:     "disconnectaa",
	DIV:              "div",
	DONE:             "done",
	DONE1:            "done1",
	DONEA:            "donea",
	DOWHILE:          "dowhile",
	DROP:             "drop",
	DUP:              "dup",
	ELSE:             "else",
	ENCODE:           "encode",
	ENCODEO:          "encodeo",
	ENCVALUE:         "encvalue",
	ENUM2INT:         "enum2int",
	ENUMERATED:       "enumerated",
	EQ:               "eq",
	ERROR:            "error",
	EXEC:             "exec",
	EXECUTE:          "execute",
	EXECUTED:         "executed",
	EXECUTEL:         "executel",
	EXECUTELD:        "executeld",
	EXTENSION:        "extension",
	EXTENSIONO:       "extensiono",
	FAIL:             "fail",
	FALSE:            "false",
	FIELD:            "field",
	FIELDO:           "fieldo",
	FLOAT:            "float",
	FLOAT2INT:        "float2int",
	FOR:              "for",
	FROM:             "from",
	FSTR:             "fstr",
	FUNCTION:         "function",
	FUNCTIONB:        "functionb",
	FUNCTIONV:        "functionv",
	FUNCTIONVB:       "functionvb",
	FUNCTIONX:        "functionx",
	FUNCTIONXV:       "functionxv",
	FUNCTIONXVW:      "functionxvw",
	FUNCTIONXW:       "functionxw",
	GE:               "ge",
	GET:              "get",
	GETVERDICT:       "getverdict",
	GT:               "gt",
	HALT:             "halt",
	HALTA:            "halta",
	HEX2BIT:          "hex2bit",
	HEX2INT:          "hex2int",
	HEX2OCT:          "hex2oct",
	HEX2STR:          "hex2str",
	HEXSTRING:        "hexstring",
	IDEF:             "idef",
	IEEE754DP:        "ieee754dp",
	IF:               "if",
	IFELSE:           "ifelse",
	IFIELD:           "ifield",
	IFPRESENT:        "ifpresent",
	IGET:             "iget",
	IN:               "in",
	INCONC:           "inconc",
	INFINITYN:        "infinityn",
	INFINITYP:        "infinityp",
	INOUT:            "inout",
	INT2BIT:          "int2bit",
	INT2CHAR:         "int2char",
	INT2ENUM:         "int2enum",
	INT2FLOAT:        "int2float",
	INT2HEX:          "int2hex",
	INT2OCT:          "int2oct",
	INT2STR:          "int2str",
	INTEGER:          "integer",
	INTERLEAVE:       "interleave",
	INTERLEAVESTEP:   "interleavestep",
	INTERLEAVESTEPB:  "interleavestepb",
	INTERLEAVESTEPBW: "interleavestepbw",
	INTERLEAVESTEPW:  "interleavestepw",
	ISBOUND:          "isbound",
	ISCHOSEN:         "ischosen",
	ISPRESENT:        "ispresent",
	ISTR:             "istr",
	ISVALUE:          "isvalue",
	JNT:              "jnt",
	JT:               "jt",
	KILL:             "kill",
	KILLA:            "killa",
	KILLED:           "killed",
	KILLED1:          "killed1",
	KILLEDA:          "killeda",
	LE:               "le",
	LENGTH:           "length",
	LENGTHOF:         "lengthof",
	LOAD:             "load",
	LOG:              "log",
	LT:               "lt",
	MAP:              "map",
	MAPT:             "mapt",
	MARK:             "mark",
	MATCH:            "match",
	MOD:              "mod",
	MODULE:           "module",
	MODULEW:          "modulew",
	MOVE:             "move",
	MPAR:             "mpar",
	MPARD:            "mpard",
	MTC:              "mtc",
	MUL:              "mul",
	NAME:             "name",
	NATLONG:          "natlong",
	NE:               "ne",
	NEG:              "neg",
	NIBBLES:          "nibbles",
	NONE:             "none",
	NOP:              "nop",
	NOT:              "not",
	NOW:              "now",
	NULL:             "null",
	OCT2BIT:          "oct2bit",
	OCT2CHR:          "oct2chr",
	OCT2HEX:          "oct2hex",
	OCT2INT:          "oct2int",
	OCT2STR:          "oct2str",
	OCTETS:           "octets",
	OCTETSTRING:      "octetstring",
	OMIT:             "omit",
	OR:               "or",
	OUT:              "out",
	PASS:             "pass",
	PATTERN:          "pattern",
	PERMITO:          "permito",
	PERMITP:          "permitp",
	PERMITT:          "permitt",
	PERMUTATION:      "permutation",
	PORTM:            "portm",
	PORTMA:           "portma",
	RANGE:            "range",
	READ:             "read",
	RECEIVE:          "receive",
	RECEIVE1:         "receive1",
	RECEIVEC:         "receivec",
	RECEIVEC1:        "receivec1",
	RECORD:           "record",
	RECORDOF:         "recordof",
	REF_BIT2HEX:      "ref_bit2hex",
	REF_BIT2INT:      "ref_bit2int",
	REF_BIT2OCT:      "ref_bit2oct",
	REF_BIT2STR:      "ref_bit2str",
	REF_CHAR2INT:     "ref_char2int",
	REF_CHAR2OCT:     "ref_char2oct",
	REF_DECVALUE:     "ref_decvalue",
	REF_ENCVALUE:     "ref_encvalue",
	REF_ENUM2INT:     "ref_enum2int",
	REF_FLOAT2INT:    "ref_float2int",
	REF_HEX2BIT:      "ref_hex2bit",
	REF_HEX2INT:      "ref_hex2int",
	REF_HEX2OCT:      "ref_hex2oct",
	REF_HEX2STR:      "ref_hex2str",
	REF_INT2BIT:      "ref_int2bit",
	REF_INT2CHAR:     "ref_int2char",
	REF_INT2ENUM:     "ref_int2enum",
	REF_INT2FLOAT:    "ref_int2float",
	REF_INT2HEX:      "ref_int2hex",
	REF_INT2OCT:      "ref_int2oct",
	REF_INT2STR:      "ref_int2str",
	REF_ISBOUND:      "ref_isbound",
	REF_ISPRESENT:    "ref_ispresent",
	REF_ISVALUE:      "ref_isvalue",
	REF_OCT2BIT:      "ref_oct2bit",
	REF_OCT2CHR:      "ref_oct2chr",
	REF_OCT2HEX:      "ref_oct2hex",
	REF_OCT2INT:      "ref_oct2int",
	REF_OCT2STR:      "ref_oct2str",
	REF_REGEXP:       "ref_regexp",
	REF_STR2FLOAT:    "ref_str2float",
	REF_STR2HEX:      "ref_str2hex",
	REF_STR2INT:      "ref_str2int",
	REF_STR2OCT:      "ref_str2oct",
	REF_TESTCASENAME: "ref_testcasename",
	REF_VAL2STR:      "ref_val2str",
	REGEXP:           "regexp",
	REM:              "rem",
	REPEAT:           "repeat",
	REPLACE:          "replace",
	RETURN:           "return",
	RND:              "rnd",
	ROL:              "rol",
	ROR:              "ror",
	RUNNING:          "running",
	RUNNING1C:        "running1c",
	RUNNING1T:        "running1t",
	RUNNINGAC:        "runningac",
	SCAN:             "scan",
	SELF:             "self",
	SEND:             "send",
	SEND1:            "send1",
	SENDA:            "senda",
	SENDER:           "sender",
	SENDN:            "sendn",
	SET:              "set",
	SETOF:            "setof",
	SETVERDICT:       "setverdict",
	SHL:              "shl",
	SHR:              "shr",
	SIZEOF:           "sizeof",
	SKIP:             "skip",
	SMATCH:           "smatch",
	SOURCE:           "source",
	SPECPLC:          "specplc",
	START:            "start",
	STARTAP:          "startap",
	STARTC:           "startc",
	STARTD:           "startd",
	STEP:             "step",
	STOP:             "stop",
	STOPAC:           "stopac",
	STOPAP:           "stopap",
	STOPAT:           "stopat",
	STOPI:            "stopi",
	STORE:            "store",
	STR2FLOAT:        "str2float",
	STR2HEX:          "str2hex",
	STR2INT:          "str2int",
	STR2OCT:          "str2oct",
	SUB:              "sub",
	SUBSET:           "subset",
	SUBSTR:           "substr",
	SUBTYPE:          "subtype",
	SUPERSET:         "superset",
	SYSTEM:           "system",
	TCSTOP:           "tcstop",
	TEMPLATE:         "template",
	TERM:             "term",
	TESTCASE:         "testcase",
	TESTCASENAME:     "testcasename",
	TESTCASES:        "testcases",
	TESTCASESW:       "testcasesw",
	TESTCASEW:        "testcasew",
	TIMEOUT:          "timeout",
	TIMEOUT1:         "timeout1",
	TIMER:            "timer",
	TIMESTAMP:        "timestamp",
	TO:               "to",
	TRAP:             "trap",
	TRIGGER:          "trigger",
	TRIGGER1:         "trigger1",
	TRUE:             "true",
	TYPE:             "type",
	TYPEW:            "typew",
	UNION:            "union",
	UNMAP:            "unmap",
	UNMAPA:           "unmapa",
	UNMAPAA:          "unmapaa",
	UNMAPFROMTO:      "unmapfromto",
	UTF8:             "utf8",
	VAL2STR:          "val2str",
	VALUE:            "value",
	VALUEOF:          "valueof",
	VAR:              "var",
	VARDUP:           "vardup",
	VARIANT:          "variant",
	VARIANTO:         "varianto",
	VERDICTTYPE:      "verdicttype",
	VERSION:          "version",
	VLIST:            "vlist",
	WAIT:             "wait",
	WHILE:            "while",
	XOR:              "xor",
}

var opcodeNames = map[string]Opcode{
	"ref":        REF,
	"line":       LINE,
	"goto":       GOTO,
	"frozen_ref": FROZEN_REF,
	"iscan":      ISCAN,
	"call":       CALL,

	"action":           ACTION,
	"activate":         ACTIVATE,
	"add":              ADD,
	"address":          ADDRESS,
	"alive":            ALIVE,
	"alive1":           ALIVE1,
	"alivea":           ALIVEA,
	"allfrom":          ALLFROM,
	"allfromp":         ALLFROMP,
	"alt":              ALT,
	"altnd":            ALTND,
	"altstep":          ALTSTEP,
	"altstepb":         ALTSTEPB,
	"altstepbw":        ALTSTEPBW,
	"altstepw":         ALTSTEPW,
	"and":              AND,
	"any":              ANY,
	"anyn":             ANYN,
	"apply":            APPLY,
	"array":            ARRAY,
	"assign":           ASSIGN,
	"assignd":          ASSIGND,
	"at_default":       AT_DEFAULT,
	"bit2hex":          BIT2HEX,
	"bit2int":          BIT2INT,
	"bit2oct":          BIT2OCT,
	"bit2str":          BIT2STR,
	"bits":             BITS,
	"bitstring":        BITSTRING,
	"block":            BLOCK,
	"boolean":          BOOLEAN,
	"break":            BREAK,
	"cat":              CAT,
	"char2int":         CHAR2INT,
	"char2oct":         CHAR2OCT,
	"charstring":       CHARSTRING,
	"charstringu":      CHARSTRINGU,
	"check":            CHECK,
	"check1":           CHECK1,
	"checkstate":       CHECKSTATE,
	"checkstateal":     CHECKSTATEAL,
	"checkstatean":     CHECKSTATEAN,
	"clear":            CLEAR,
	"cleara":           CLEARA,
	"clone":            CLONE,
	"closure":          CLOSURE,
	"closuretype":      CLOSURETYPE,
	"collect":          COLLECT,
	"complement":       COMPLEMENT,
	"component":        COMPONENT,
	"componentx":       COMPONENTX,
	"connect":          CONNECT,
	"const":            CONST,
	"constw":           CONSTW,
	"continue":         CONTINUE,
	"control":          CONTROL,
	"controlw":         CONTROLW,
	"create":           CREATE,
	"createa":          CREATEA,
	"createan":         CREATEAN,
	"createn":          CREATEN,
	"deactivate":       DEACTIVATE,
	"deactivatea":      DEACTIVATEA,
	"decmatch":         DECMATCH,
	"decvalue":         DECVALUE,
	"def":              DEF,
	"default":          DEFAULT,
	"disconnect":       DISCONNECT,
	"disconnecta":      DISCONNECTA,
	"disconnectaa":     DISCONNECTAA,
	"div":              DIV,
	"done":             DONE,
	"done1":            DONE1,
	"donea":            DONEA,
	"dowhile":          DOWHILE,
	"drop":             DROP,
	"dup":              DUP,
	"else":             ELSE,
	"encode":           ENCODE,
	"encodeo":          ENCODEO,
	"encvalue":         ENCVALUE,
	"enum2int":         ENUM2INT,
	"enumerated":       ENUMERATED,
	"eq":               EQ,
	"error":            ERROR,
	"exec":             EXEC,
	"execute":          EXECUTE,
	"executed":         EXECUTED,
	"executel":         EXECUTEL,
	"executeld":        EXECUTELD,
	"extension":        EXTENSION,
	"extensiono":       EXTENSIONO,
	"fail":             FAIL,
	"false":            FALSE,
	"field":            FIELD,
	"fieldo":           FIELDO,
	"float":            FLOAT,
	"float2int":        FLOAT2INT,
	"for":              FOR,
	"from":             FROM,
	"fstr":             FSTR,
	"function":         FUNCTION,
	"functionb":        FUNCTIONB,
	"functionv":        FUNCTIONV,
	"functionvb":       FUNCTIONVB,
	"functionx":        FUNCTIONX,
	"functionxv":       FUNCTIONXV,
	"functionxvw":      FUNCTIONXVW,
	"functionxw":       FUNCTIONXW,
	"ge":               GE,
	"get":              GET,
	"getverdict":       GETVERDICT,
	"gt":               GT,
	"halt":             HALT,
	"halta":            HALTA,
	"hex2bit":          HEX2BIT,
	"hex2int":          HEX2INT,
	"hex2oct":          HEX2OCT,
	"hex2str":          HEX2STR,
	"hexstring":        HEXSTRING,
	"idef":             IDEF,
	"ieee754dp":        IEEE754DP,
	"if":               IF,
	"ifelse":           IFELSE,
	"ifield":           IFIELD,
	"ifpresent":        IFPRESENT,
	"iget":             IGET,
	"in":               IN,
	"inconc":           INCONC,
	"infinityn":        INFINITYN,
	"infinityp":        INFINITYP,
	"inout":            INOUT,
	"int2bit":          INT2BIT,
	"int2char":         INT2CHAR,
	"int2enum":         INT2ENUM,
	"int2float":        INT2FLOAT,
	"int2hex":          INT2HEX,
	"int2oct":          INT2OCT,
	"int2str":          INT2STR,
	"integer":          INTEGER,
	"interleave":       INTERLEAVE,
	"interleavestep":   INTERLEAVESTEP,
	"interleavestepb":  INTERLEAVESTEPB,
	"interleavestepbw": INTERLEAVESTEPBW,
	"interleavestepw":  INTERLEAVESTEPW,
	"isbound":          ISBOUND,
	"ischosen":         ISCHOSEN,
	"ispresent":        ISPRESENT,
	"istr":             ISTR,
	"isvalue":          ISVALUE,
	"jnt":              JNT,
	"jt":               JT,
	"kill":             KILL,
	"killa":            KILLA,
	"killed":           KILLED,
	"killed1":          KILLED1,
	"killeda":          KILLEDA,
	"le":               LE,
	"length":           LENGTH,
	"lengthof":         LENGTHOF,
	"load":             LOAD,
	"log":              LOG,
	"lt":               LT,
	"map":              MAP,
	"mapt":             MAPT,
	"mark":             MARK,
	"match":            MATCH,
	"mod":              MOD,
	"module":           MODULE,
	"modulew":          MODULEW,
	"move":             MOVE,
	"mpar":             MPAR,
	"mpard":            MPARD,
	"mtc":              MTC,
	"mul":              MUL,
	"name":             NAME,
	"natlong":          NATLONG,
	"ne":               NE,
	"neg":              NEG,
	"nibbles":          NIBBLES,
	"none":             NONE,
	"nop":              NOP,
	"not":              NOT,
	"now":              NOW,
	"null":             NULL,
	"oct2bit":          OCT2BIT,
	"oct2chr":          OCT2CHR,
	"oct2hex":          OCT2HEX,
	"oct2int":          OCT2INT,
	"oct2str":          OCT2STR,
	"octets":           OCTETS,
	"octetstring":      OCTETSTRING,
	"omit":             OMIT,
	"or":               OR,
	"out":              OUT,
	"pass":             PASS,
	"pattern":          PATTERN,
	"permito":          PERMITO,
	"permitp":          PERMITP,
	"permitt":          PERMITT,
	"permutation":      PERMUTATION,
	"portm":            PORTM,
	"portma":           PORTMA,
	"range":            RANGE,
	"read":             READ,
	"receive":          RECEIVE,
	"receive1":         RECEIVE1,
	"receivec":         RECEIVEC,
	"receivec1":        RECEIVEC1,
	"record":           RECORD,
	"recordof":         RECORDOF,
	"ref_bit2hex":      REF_BIT2HEX,
	"ref_bit2int":      REF_BIT2INT,
	"ref_bit2oct":      REF_BIT2OCT,
	"ref_bit2str":      REF_BIT2STR,
	"ref_char2int":     REF_CHAR2INT,
	"ref_char2oct":     REF_CHAR2OCT,
	"ref_decvalue":     REF_DECVALUE,
	"ref_encvalue":     REF_ENCVALUE,
	"ref_enum2int":     REF_ENUM2INT,
	"ref_float2int":    REF_FLOAT2INT,
	"ref_hex2bit":      REF_HEX2BIT,
	"ref_hex2int":      REF_HEX2INT,
	"ref_hex2oct":      REF_HEX2OCT,
	"ref_hex2str":      REF_HEX2STR,
	"ref_int2bit":      REF_INT2BIT,
	"ref_int2char":     REF_INT2CHAR,
	"ref_int2enum":     REF_INT2ENUM,
	"ref_int2float":    REF_INT2FLOAT,
	"ref_int2hex":      REF_INT2HEX,
	"ref_int2oct":      REF_INT2OCT,
	"ref_int2str":      REF_INT2STR,
	"ref_isbound":      REF_ISBOUND,
	"ref_ispresent":    REF_ISPRESENT,
	"ref_isvalue":      REF_ISVALUE,
	"ref_oct2bit":      REF_OCT2BIT,
	"ref_oct2chr":      REF_OCT2CHR,
	"ref_oct2hex":      REF_OCT2HEX,
	"ref_oct2int":      REF_OCT2INT,
	"ref_oct2str":      REF_OCT2STR,
	"ref_regexp":       REF_REGEXP,
	"ref_str2float":    REF_STR2FLOAT,
	"ref_str2hex":      REF_STR2HEX,
	"ref_str2int":      REF_STR2INT,
	"ref_str2oct":      REF_STR2OCT,
	"ref_testcasename": REF_TESTCASENAME,
	"ref_val2str":      REF_VAL2STR,
	"regexp":           REGEXP,
	"rem":              REM,
	"repeat":           REPEAT,
	"replace":          REPLACE,
	"return":           RETURN,
	"rnd":              RND,
	"rol":              ROL,
	"ror":              ROR,
	"running":          RUNNING,
	"running1c":        RUNNING1C,
	"running1t":        RUNNING1T,
	"runningac":        RUNNINGAC,
	"scan":             SCAN,
	"self":             SELF,
	"send":             SEND,
	"send1":            SEND1,
	"senda":            SENDA,
	"sender":           SENDER,
	"sendn":            SENDN,
	"set":              SET,
	"setof":            SETOF,
	"setverdict":       SETVERDICT,
	"shl":              SHL,
	"shr":              SHR,
	"sizeof":           SIZEOF,
	"skip":             SKIP,
	"smatch":           SMATCH,
	"source":           SOURCE,
	"specplc":          SPECPLC,
	"start":            START,
	"startap":          STARTAP,
	"startc":           STARTC,
	"startd":           STARTD,
	"step":             STEP,
	"stop":             STOP,
	"stopac":           STOPAC,
	"stopap":           STOPAP,
	"stopat":           STOPAT,
	"stopi":            STOPI,
	"store":            STORE,
	"str2float":        STR2FLOAT,
	"str2hex":          STR2HEX,
	"str2int":          STR2INT,
	"str2oct":          STR2OCT,
	"sub":              SUB,
	"subset":           SUBSET,
	"substr":           SUBSTR,
	"subtype":          SUBTYPE,
	"superset":         SUPERSET,
	"system":           SYSTEM,
	"tcstop":           TCSTOP,
	"template":         TEMPLATE,
	"term":             TERM,
	"testcase":         TESTCASE,
	"testcasename":     TESTCASENAME,
	"testcases":        TESTCASES,
	"testcasesw":       TESTCASESW,
	"testcasew":        TESTCASEW,
	"timeout":          TIMEOUT,
	"timeout1":         TIMEOUT1,
	"timer":            TIMER,
	"timestamp":        TIMESTAMP,
	"to":               TO,
	"trap":             TRAP,
	"trigger":          TRIGGER,
	"trigger1":         TRIGGER1,
	"true":             TRUE,
	"type":             TYPE,
	"typew":            TYPEW,
	"union":            UNION,
	"unmap":            UNMAP,
	"unmapa":           UNMAPA,
	"unmapaa":          UNMAPAA,
	"unmapfromto":      UNMAPFROMTO,
	"utf8":             UTF8,
	"val2str":          VAL2STR,
	"value":            VALUE,
	"valueof":          VALUEOF,
	"var":              VAR,
	"vardup":           VARDUP,
	"variant":          VARIANT,
	"varianto":         VARIANTO,
	"verdicttype":      VERDICTTYPE,
	"version":          VERSION,
	"vlist":            VLIST,
	"wait":             WAIT,
	"while":            WHILE,
	"xor":              XOR,
}
