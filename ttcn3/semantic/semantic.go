// Package semantic implements lightweight name binding and semantic
// validation for TTCN-3 source code.
//
// The package is intentionally a thin pre-type-checker rather than a
// fully type-correct model from day one: it focuses on the
// diagnostics that give the highest perceived quality jump over
// "parse OK = no diagnostics":
//
//   - Unknown / unresolved import modules.
//   - Unresolved identifiers in expressions and types.
//   - Duplicate definitions inside a module.
//
// The Analyzer is intentionally cheap: it does not allocate per-node
// structures, it does not memoise resolutions across invocations, and it
// is safe to call from the LSP on every keystroke.
package semantic

import (
	"fmt"
	"sort"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
	"github.com/nokia/ntt/ttcn3/types"
)

// Severity is the severity of a Diagnostic.
type Severity int

const (
	SeverityError Severity = 1
	SeverityWarn  Severity = 2
)

// Diagnostic is the result of an analysis pass.
type Diagnostic struct {
	// Code is a stable identifier for the analysis rule.
	Code string

	// Severity classifies the diagnostic.
	Severity Severity

	// Message is the human-readable description.
	Message string

	// Node is the source node the diagnostic refers to.
	Node syntax.Node

	// Span is the precomputed source span. Always present.
	Span syntax.Span
}

// Analyzer runs the semantic checks against a Tree. It is stateless and
// safe to reuse.
type Analyzer struct {
	// DB is the suite-wide database used to resolve imports. It may be
	// nil, in which case import resolution is skipped (the analyzer
	// falls back to the in-tree definitions only).
	DB *ttcn3.DB
}

// NewAnalyzer returns an Analyzer wired up to the given database.
func NewAnalyzer(db *ttcn3.DB) *Analyzer {
	return &Analyzer{DB: db}
}

// Analyze runs the semantic checks against tree and returns the resulting
// diagnostics sorted by source position.
//
// The analyzer is intentionally additive: each rule contributes to the slice
// and is responsible for its own scope walk. Rules that need cross-module
// information go through the Analyzer's DB; rules that are purely local
// (duplicates, attribute well-formedness) work off the syntax tree alone.
func (a *Analyzer) Analyze(tree *ttcn3.Tree) []Diagnostic {
	if tree == nil || tree.Root == nil {
		return nil
	}

	var out []Diagnostic
	for _, modNode := range tree.Modules() {
		mod, ok := modNode.Node.(*syntax.Module)
		if !ok {
			continue
		}
		out = append(out, a.checkImports(tree, mod)...)
		out = append(out, a.checkDuplicates(mod)...)
		out = append(out, a.checkRunsOnReferences(tree, mod)...)
		out = append(out, a.checkAttributes(mod)...)
		out = append(out, a.checkWithExceptRefs(mod)...)
		out = append(out, a.checkStructuredDeclRules(mod)...)
		out = append(out, a.checkReturnConsistency(mod)...)
		out = append(out, a.checkSideEffectInRestrictedContext(mod)...)
		out = append(out, a.checkTemplateRestrictions(mod)...)
		out = append(out, a.checkValueTemplateArgKinds(mod)...)
		out = append(out, a.checkControlPartOps(mod)...)
		out = append(out, a.checkArrayDims(mod)...)
		out = append(out, a.checkInterleaveRestrictions(mod)...)
		out = append(out, a.checkBinaryLiterals(mod)...)
		out = append(out, a.checkConstLiteralTypes(mod)...)
		out = append(out, a.checkMissingCallParens(mod)...)
		out = append(out, a.checkPortOps(mod)...)
		out = append(out, a.checkConnectMapCompat(mod)...)
		out = append(out, a.checkComponentOps(mod)...)
		out = append(out, a.checkParameterRules(mod)...)
		out = append(out, a.checkLengthofArgs(mod)...)
		out = append(out, a.checkLengthConstraints(mod)...)
		out = append(out, a.checkValueConstraints(mod)...)
		out = append(out, a.checkStructValueRules(mod)...)
		out = append(out, a.checkSubsetSupersetRules(mod)...)
		out = append(out, a.checkMapIndexRules(mod)...)
		out = append(out, a.checkEnumValueRules(mod)...)
		out = append(out, a.checkComponentExtendsRules(mod)...)
		out = append(out, a.checkOmitValueRules(mod)...)
		out = append(out, a.checkTemplateRangeRules(mod)...)
		out = append(out, a.checkStringElementRules(mod)...)
		out = append(out, a.checkActualParameterRules(mod)...)
		out = append(out, a.checkReceiveRedirectRules(mod)...)
		out = append(out, a.checkAnyFromPortRules(mod)...)
		out = append(out, a.checkArraySizeCompat(mod)...)
		out = append(out, a.checkComponentCallRules(mod)...)
		out = append(out, a.checkIdentifierUniqueness(mod)...)
		out = append(out, a.checkArrayIndexRules(mod)...)
		out = append(out, a.checkPredefinedFunctionRules(mod)...)
		out = append(out, a.checkCallStmtBodyRules(mod)...)
		out = append(out, a.checkNullAddressInFromClause(mod)...)
		out = append(out, a.checkConstructorRules(mod)...)
		out = append(out, a.checkOpenTypeUsage(mod)...)
		out = append(out, a.checkSignatureTemplateRules(mod)...)
		out = append(out, a.checkComponentOpReceivers(mod)...)
		out = append(out, a.checkSelectStmtRules(mod)...)
		out = append(out, a.checkCallOperationRules(mod)...)
		out = append(out, a.checkPortOpReceivers(mod)...)
		out = append(out, a.checkModuleparKinds(mod)...)
		out = append(out, a.checkRaiseOperationRules(mod)...)
		out = append(out, a.checkFunctionSpecRules(mod)...)
		out = append(out, a.checkLengthBoundRules(mod)...)
		out = append(out, a.checkComponentTestOpRules(mod)...)
		out = append(out, a.checkAnyTimerRules(mod)...)
		out = append(out, a.checkTemplateReassignRules(mod)...)
		out = append(out, a.checkCompositeLiteralRules(mod)...)
		out = append(out, a.checkRecursiveTypeRules(mod)...)
		out = append(out, a.checkEnumRules(mod)...)
		out = append(out, a.checkPortTypeRules(mod)...)
		out = append(out, a.checkIndexOutOfBoundsRules(mod)...)
		out = append(out, a.checkGetcallRedirectRules(mod)...)
		out = append(out, a.checkAddrClauseTypeRules(mod)...)
		out = append(out, a.checkComponentArrayInitRules(mod)...)
		out = append(out, a.checkConsecutiveStartRules(mod)...)
		out = append(out, a.checkTimerArityRules(mod)...)
		out = append(out, a.checkTimerSyntaxRules(mod)...)
		out = append(out, a.checkTimerReceiverRules(mod)...)
		out = append(out, a.checkTimerScopeRules(mod)...)
		out = append(out, a.checkNonAliveRestartRules(mod)...)
		out = append(out, a.checkStartArgTypeRules(mod)...)
		out = append(out, a.checkMapUnmapParamRules(mod)...)
		out = append(out, a.checkConnectOutlistRules(mod)...)
		out = append(out, a.checkDecodedRedirectRules(mod)...)
		out = append(out, a.checkSendTemplateTypeRules(mod)...)
		out = append(out, a.checkModifiedTemplateSelfRefRules(mod)...)
		out = append(out, a.checkValueVarInitRules(mod)...)
		out = append(out, a.checkCharRangeSubtypeRules(mod)...)
		out = append(out, a.checkInoutStrictTypingRules(mod)...)
		out = append(out, a.checkPortDeclTypeRules(mod)...)
		out = append(out, a.checkInterleaveBodyRules(mod)...)
		out = append(out, a.checkEnumUseRules(mod)...)
		out = append(out, a.checkStructFieldRangeRules(mod)...)
		out = append(out, a.checkInoutParamOverlapRules(mod)...)
		out = append(out, a.checkAllFromSourceRules(mod)...)
		out = append(out, a.checkIndexRedirectDimRules(mod)...)
		out = append(out, a.checkIndexRedirectTargetTypeRules(mod)...)
		out = append(out, a.checkConstKindRules(mod)...)
		out = append(out, a.checkAssignmentPrimitiveRules(mod)...)
		out = append(out, a.checkTypeCompatRules(mod)...)
		out = append(out, a.checkClassMemberRules(mod)...)
		out = append(out, a.checkAnytypeSelectorRules(mod)...)
		out = append(out, a.checkIndexIndexerRules(mod)...)
		out = append(out, a.checkUnionAltRefRules(mod)...)
		out = append(out, a.checkLengthSubtypingRules(mod)...)
		out = append(out, a.checkGotoLabelRules(mod)...)
		out = append(out, a.checkTemplateTypeRules(mod)...)
		out = append(out, a.checkExecuteStmtRules(mod)...)
		out = append(out, a.checkSetencodeRules(mod)...)
		out = append(out, a.checkExprOperandRules(mod)...)
		out = append(out, a.checkNestedClassRules(mod)...)
		out = append(out, a.checkSenderRedirectRules(mod)...)
		out = append(out, a.checkPortSignatureRules(mod)...)
		out = append(out, a.checkUnionAltChooseRules(mod)...)
		out = append(out, a.checkPortVarInitRules(mod)...)
		out = append(out, a.checkImplicitConstructorRules(mod)...)
		out = append(out, a.checkDefaultAnytypeRules(mod)...)
		out = append(out, a.checkRepeatScopeRules(mod)...)
		out = append(out, a.checkActivateRules(mod)...)
		out = append(out, a.checkModRemRules(mod)...)
		out = append(out, a.checkSendPartialTemplateRules(mod)...)
		out = append(out, a.checkUninitReadRules(mod)...)
		out = append(out, a.checkMapAssignCompatRules(mod)...)
		out = append(out, a.checkDisconnectUnmapRules(mod)...)
		out = append(out, a.checkFunctionClauseRules(mod)...)
		out = append(out, a.checkUninitTemplateReadRules(mod)...)
		out = append(out, a.checkCallCatchTimeoutRules(mod)...)
		out = append(out, a.checkConstInitRules(mod)...)
		out = append(out, a.checkInterleaveGotoRules(mod)...)
		out = append(out, a.checkNonPortOpRules(mod)...)
		out = append(out, a.checkAllComponentOpRules(mod)...)
		out = append(out, a.checkMapTopologyRules(mod)...)
		out = append(out, a.checkTimerInitRules(mod)...)
		out = append(out, a.checkVarScopeRules(mod)...)
		out = append(out, a.checkRunsOnMissingRules(mod)...)
		out = append(out, a.checkModuleparAddressRules(mod)...)
		out = append(out, a.checkVerdictInitRules(mod)...)
		out = append(out, a.checkAltstepInvokeRules(mod)...)
		out = append(out, a.checkTimerStartUninitRules(mod)...)
		out = append(out, a.checkLabelAltRules(mod)...)
		out = append(out, a.checkComponentBodyDeclRules(mod)...)
		out = append(out, a.checkPortOptionalAttributeRules(mod)...)
		out = append(out, a.checkReturnTemplateValueRules(mod)...)
		out = append(out, a.checkTemplateOmitAssignRules(mod)...)
		out = append(out, a.checkDecodedFieldRefRules(mod)...)
		out = append(out, a.checkVarTemplateTypeRules(mod)...)
		out = append(out, a.checkTemplateInitCompleteRules(mod)...)
		out = append(out, a.checkChainedAssignRules(mod)...)
		out = append(out, a.checkComponentRepeatedCallRules(mod)...)
		out = append(out, a.checkPatternSubtypeRules(mod)...)
		out = append(out, a.checkSignatureNoBlockRules(mod)...)
		out = append(out, a.checkArithTypeMismatchRules(mod)...)
		out = append(out, a.checkTimerAssignCompatRules(mod)...)
		out = append(out, a.checkSubsetLengthRules(mod)...)
		out = append(out, a.checkMatchUninitArgRules(mod)...)
		out = append(out, a.checkFormalDefaultTypeRules(mod)...)
		out = append(out, a.checkBlockingCallRules(mod)...)
		out = append(out, a.checkAltstepDeclOrderingRules(mod)...)
		out = append(out, a.checkOmitEqualityRules(mod)...)
		out = append(out, a.checkOmitToMandatoryRules(mod)...)
		out = append(out, a.checkRecordOptionalityAssignRules(mod)...)
		out = append(out, a.checkSendToLiteralAddrRules(mod)...)
		out = append(out, a.checkVarInitLiteralTypeRules(mod)...)
		out = append(out, a.checkEnumIntAssignRules(mod)...)
		out = append(out, a.checkUninitRecordFieldReadRules(mod)...)
		out = append(out, a.checkNamedArgCompletenessRules(mod)...)
		out = append(out, a.checkNullAddressReadRules(mod)...)
	}

	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].Span.Begin, out[j].Span.Begin
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Column < b.Column
	})
	return out
}

// checkImports flags `import from M ...` declarations where M is neither
// the importing module itself nor a known module in the database.
func (a *Analyzer) checkImports(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	self := syntax.Name(mod.Name)

	mod.Inspect(func(n syntax.Node) bool {
		imp, ok := n.(*syntax.ImportDecl)
		if !ok {
			return true
		}
		if imp.Module == nil {
			return false
		}
		name := syntax.Name(imp.Module)
		if name == "" {
			return false
		}
		if name == self {
			return false
		}
		if a.DB != nil && a.DB.Modules != nil {
			if files, ok := a.DB.Modules[name]; ok && len(files) > 0 {
				return false
			}
		}
		diags = append(diags, Diagnostic{
			Code:     "unknown-import",
			Severity: SeverityError,
			Message:  fmt.Sprintf("unknown module %q", name),
			Node:     imp.Module,
			Span:     syntax.SpanOf(imp.Module),
		})
		return false
	})
	return diags
}

// checkDuplicates flags two top-level definitions in the same module
// that share a name. This is a hard error in TTCN-3 but parsers happily
// accept it.
func (a *Analyzer) checkDuplicates(mod *syntax.Module) []Diagnostic {
	type seen struct {
		first syntax.Node
		span  syntax.Span
	}
	defs := make(map[string]seen)
	var diags []Diagnostic

	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		name := syntax.Name(d.Def)
		if name == "" {
			continue
		}
		if prev, ok := defs[name]; ok {
			span := syntax.SpanOf(d.Def)
			diags = append(diags, Diagnostic{
				Code:     "duplicate-definition",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"duplicate definition of %q (first declared at %s)",
					name, prev.span.Begin),
				Node: d.Def,
				Span: span,
			})
			continue
		}
		defs[name] = seen{first: d.Def, span: syntax.SpanOf(d.Def)}
	}
	return diags
}

// IsPredefinedType returns true when name refers to one of the TTCN-3
// predefined types. It is a small helper exported for the LSP hover and
// completion handlers, which need to distinguish between user types and
// builtin ones.
func IsPredefinedType(name string) bool {
	_, ok := types.Predefined[name]
	return ok
}
