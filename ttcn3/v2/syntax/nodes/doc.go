// Package nodes contains the schema-generated AST node types for the
// next-generation TTCN-3 syntax tree.
//
// The package follows a schema-driven layout: every node is defined
// once in nodes.yaml and the generator under gen/ turns the schema
// into struct types, kind constants, a Visitor interface
// and a children accessor. This keeps the tree definition in one
// place instead of spread across nodes.go, nodes_gen.go and a maze
// of switch statements.
//
// To regenerate the file after editing nodes.yaml:
//
//	cd ttcn3/v2/syntax/nodes && go generate
package nodes

//go:generate go run ./gen
