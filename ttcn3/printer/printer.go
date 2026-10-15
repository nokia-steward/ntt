package printer

import (
	"fmt"
	"io"

	"github.com/nokia/ntt/ttcn3/syntax"
)

type whiteSpace byte

const (
	ignore   = whiteSpace(0)
	blank    = whiteSpace(' ')
	vtab     = whiteSpace('\v')
	newline  = whiteSpace('\n')
	formfeed = whiteSpace('\f')
	indent   = whiteSpace('>')
	unindent = whiteSpace('<')
)

func Print(w io.Writer, n syntax.Node) error {
	p := printer{w: w, lineStart: true}
	p.print(n)
	return p.err
}

type printer struct {
	w                io.Writer
	indent           int
	lineStart        bool
	ignoreNextSpace  bool
	printNewlineNext bool
	err              error
}

func (p *printer) print(values ...interface{}) {
	for _, v := range values {
		switch n := v.(type) {
		case whiteSpace:
			switch n {
			case indent:
				p.indent++
			case unindent:
				p.indent--
			default:
				fmt.Fprint(p.w, n)
			}

		case *syntax.ErrorNode:
			if n == nil {
				return
			}

			p.print(n.From)
			p.print(n.To)

		case *syntax.Ident:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.Tok2)

		case *syntax.ParametrizedIdent:
			if n == nil {
				return
			}
			p.print(n.Ident)
			p.print(n.Params)

		case *syntax.ValueLiteral:
			if n == nil {
				return
			}
			p.print(n.Tok)

		case *syntax.CompositeLiteral:
			if n == nil {
				return
			}
			p.print(n.LBrace, indent)
			p.print(n.List)
			p.print(unindent, n.RBrace)

		case *syntax.ObjidLiteral:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.LBrace, indent)
			p.print(n.List)
			p.print(unindent, n.RBrace)

		case *syntax.UnaryExpr:
			if n == nil {
				return
			}
			p.print(n.Op)
			p.print(n.X)

		case *syntax.BinaryExpr:
			if n == nil {
				return
			}
			p.print(n.X)
			p.print(n.Op)
			p.print(n.Y)

		case *syntax.ParenExpr:
			if n == nil {
				return
			}
			p.print(n.LParen)
			p.print(n.List)
			p.print(n.RParen)

		case *syntax.SelectorExpr:
			if n == nil {
				return
			}
			p.print(n.X)
			p.print(n.Dot)
			p.print(n.Sel)

		case *syntax.IndexExpr:
			if n == nil {
				return
			}
			p.print(n.X)
			p.print(n.LBrack)
			p.print(n.Index)
			p.print(n.RBrack)

		case *syntax.CallExpr:
			if n == nil {
				return
			}
			p.print(n.Fun)
			p.print(n.Args)

		case *syntax.LengthExpr:
			if n == nil {
				return
			}
			p.print(n.X)
			p.print(n.Len)
			p.print(n.Size)

		case *syntax.RedirectExpr:
			if n == nil {
				return
			}
			p.print(n.X)
			p.print(n.Tok)
			p.print(n.ValueTok)
			p.print(n.Value)
			p.print(n.ParamTok)
			p.print(n.Param)
			p.print(n.SenderTok)
			p.print(n.Sender)
			p.print(n.IndexTok)
			p.print(n.IndexValueTok)
			p.print(n.Index)
			p.print(n.TimestampTok)
			p.print(n.Timestamp)

		case *syntax.ValueExpr:
			if n == nil {
				return
			}
			p.print(n.X)
			p.print(n.Tok)
			p.print(n.Y)

		case *syntax.ParamExpr:
			if n == nil {
				return
			}
			p.print(n.X)
			p.print(n.Tok)
			p.print(n.Y)

		case *syntax.FromExpr:
			if n == nil {
				return
			}
			p.print(n.KindTok)
			p.print(n.FromTok)
			p.print(n.X)

		case *syntax.ModifiesExpr:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.X)
			p.print(n.Assign)
			p.print(n.Y)

		case *syntax.RegexpExpr:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.NoCase)
			p.print(n.X)

		case *syntax.PatternExpr:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.NoCase)
			p.print(n.X)

		case *syntax.DecmatchExpr:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.Params)
			p.print(n.X)

		case *syntax.DecodedExpr:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.Params)
			p.print(n.X)

		case *syntax.DefKindExpr:
			if n == nil {
				return
			}
			p.print(n.KindTok)
			p.print(n.List)

		case *syntax.ExceptExpr:
			if n == nil {
				return
			}
			p.print(n.X)
			p.print(n.ExceptTok)
			p.print(n.LBrace, indent)
			p.print(n.List)
			p.print(unindent, n.RBrace)

		case *syntax.BlockStmt:
			if n == nil {
				return
			}
			p.print(n.LBrace, indent)
			p.print(n.Stmts)
			p.print(unindent, n.RBrace)

		case *syntax.DeclStmt:
			if n == nil {
				return
			}
			p.print(n.Decl)

		case *syntax.ExprStmt:
			if n == nil {
				return
			}
			p.print(n.Expr)

		case *syntax.BranchStmt:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.Label)

		case *syntax.ReturnStmt:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.Result)

		case *syntax.AltStmt:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.Body)

		case *syntax.CallStmt:
			if n == nil {
				return
			}
			p.print(n.Stmt)
			p.print(n.Body)

		case *syntax.ForStmt:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.LParen)
			p.print(n.Init)
			p.print(n.InitSemi)
			p.print(n.Cond)
			p.print(n.CondSemi)
			p.print(n.Post)
			p.print(n.RParen)
			p.print(n.Body)

		case *syntax.ForRangeStmt:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.LParen)
			p.print(n.Init)
			p.print(n.InTok)
			p.print(n.Range)
			p.print(n.RParen)
			p.print(n.Body)

		case *syntax.WhileStmt:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.Cond)
			p.print(n.Body)

		case *syntax.DoWhileStmt:
			if n == nil {
				return
			}
			p.print(n.DoTok)
			p.print(n.Body)
			p.print(n.WhileTok)
			p.print(n.Cond)

		case *syntax.IfStmt:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.Cond)
			p.print(n.Then)
			p.print(n.ElseTok)
			p.print(n.Else)

		case *syntax.SelectStmt:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.Union)
			p.print(n.Tag)
			p.print(n.LBrace, indent)
			p.print(n.Body)
			p.print(unindent, n.RBrace)

		case *syntax.CaseClause:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.Case)
			p.print(n.Body)

		case *syntax.CommClause:
			if n == nil {
				return
			}
			p.print(n.LBrack)
			p.print(n.X)
			p.print(n.Else)
			p.print(n.RBrack)
			p.print(n.Comm)
			p.print(n.Body)

		case *syntax.Field:
			if n == nil {
				return
			}
			p.print(n.DefaultTok)
			p.print(n.Type)
			p.print(n.Name)
			p.print(n.ArrayDef)
			p.print(n.TypePars)
			p.print(n.ValueConstraint)
			p.print(n.LengthConstraint)
			p.print(n.Optional)

		case *syntax.RefSpec:
			if n == nil {
				return
			}
			p.print(n.X)

		case *syntax.StructSpec:
			if n == nil {
				return
			}
			p.print(n.KindTok)
			p.print(n.LBrace, indent)
			p.print(n.Fields)
			p.print(unindent, n.RBrace)

		case *syntax.MapSpec:
			if n == nil {
				return
			}
			p.print(n.MapTok)
			p.print(n.FromTok)
			p.print(n.FromType)
			p.print(n.ToTok)
			p.print(n.ToType)

		case *syntax.ListSpec:
			if n == nil {
				return
			}
			p.print(n.KindTok)
			p.print(n.Length)
			p.print(n.OfTok)
			p.print(n.ElemType)

		case *syntax.EnumSpec:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.LBrace, indent)
			p.print(n.Enums)
			p.print(unindent, n.RBrace)

		case *syntax.BehaviourSpec:
			if n == nil {
				return
			}
			p.print(n.KindTok)
			p.print(n.Params)
			p.print(n.Return)
			p.print(n.System)
			p.print(n.Return)

		case *syntax.ValueDecl:
			if n == nil {
				return
			}
			p.print(n.KindTok)
			p.print(n.TemplateRestriction)
			p.print(n.Modif)
			p.print(n.Type)
			p.print(n.Decls)
			p.print(n.With)

		case *syntax.Declarator:
			if n == nil {
				return
			}
			p.print(n.Name)
			p.print(n.ArrayDef)
			p.print(n.AssignTok)
			p.print(n.Value)

		case *syntax.TemplateDecl:
			if n == nil {
				return
			}
			p.print(&n.RestrictionSpec)
			p.print(n.Modif)
			p.print(n.Type)
			p.print(n.Name)
			p.print(n.TypePars)
			p.print(n.Params)
			p.print(n.ModifiesTok)
			p.print(n.Base)
			p.print(n.AssignTok)
			p.print(n.Value)
			p.print(n.With)

		case *syntax.ModuleParameterGroup:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.LBrace, indent)
			p.print(n.Decls)
			p.print(unindent, n.RBrace)
			p.print(n.With)

		case *syntax.FuncDecl:
			if n == nil {
				return
			}
			p.print(n.External)
			p.print(n.KindTok)
			p.print(n.Name)
			p.print(n.Modif)
			p.print(n.TypePars)
			p.print(n.Params)
			p.print(n.RunsOn)
			p.print(n.Mtc)
			p.print(n.System)
			p.print(n.Return)
			p.print(n.Body)
			p.print(n.With)

		case *syntax.SignatureDecl:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.Name)
			p.print(n.TypePars)
			p.print(n.Params)
			p.print(n.NoBlock)
			p.print(n.Return)
			p.print(n.ExceptionTok)
			p.print(n.Exception)
			p.print(n.With)

		case *syntax.SubTypeDecl:
			if n == nil {
				return
			}
			p.print(n.TypeTok)
			p.print(n.Field)
			p.print(n.With)

		case *syntax.StructTypeDecl:
			if n == nil {
				return
			}
			p.print(n.TypeTok)
			p.print(n.KindTok)
			p.print(n.Name)
			p.print(n.TypePars)
			p.print(n.LBrace, indent)
			p.print(n.Fields)
			p.print(unindent, n.RBrace)
			p.print(n.With)

		case *syntax.MapTypeDecl:
			if n == nil {
				return
			}
			p.print(n.TypeTok)
			p.print(n.Spec)
			p.print(n.Name)
			p.print(n.TypePars)
			p.print(n.With)

		case *syntax.EnumTypeDecl:
			if n == nil {
				return
			}
			p.print(n.TypeTok)
			p.print(n.EnumTok)
			p.print(n.Name)
			p.print(n.TypePars)
			p.print(n.LBrace, indent)
			p.print(n.Enums)
			p.print(unindent, n.RBrace)
			p.print(n.With)

		case *syntax.BehaviourTypeDecl:
			if n == nil {
				return
			}
			p.print(n.TypeTok)
			p.print(n.KindTok)
			p.print(n.Name)
			p.print(n.TypePars)
			p.print(n.Params)
			p.print(n.RunsOn)
			p.print(n.System)
			p.print(n.Return)
			p.print(n.With)

		case *syntax.PortTypeDecl:
			if n == nil {
				return
			}
			p.print(n.TypeTok)
			p.print(n.PortTok)
			p.print(n.Name)
			p.print(n.TypePars)
			p.print(n.KindTok)
			p.print(n.Realtime)
			p.print(n.LBrace, indent)
			p.print(n.Attrs)
			p.print(unindent, n.RBrace)
			p.print(n.With)

		case *syntax.PortAttribute:
			if n == nil {
				return
			}
			p.print(n.KindTok)
			p.print(n.Types)

		case *syntax.PortMapAttribute:
			if n == nil {
				return
			}
			p.print(n.MapTok)
			p.print(n.ParamTok)
			p.print(n.Params)

		case *syntax.ComponentTypeDecl:
			if n == nil {
				return
			}
			p.print(n.TypeTok)
			p.print(n.CompTok)
			p.print(n.Name)
			p.print(n.TypePars)
			p.print(n.ExtendsTok)
			p.print(n.Extends)
			p.print(n.Body)
			p.print(n.With)

		case *syntax.Module:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.Name)
			p.print(n.Language)
			p.print(n.LBrace, indent)
			p.print(n.Defs)
			p.print(unindent, n.RBrace)
			p.print(n.With)

		case *syntax.ModuleDef:
			if n == nil {
				return
			}
			p.print(n.Def)
			p.print(n.Visibility)

		case *syntax.ControlPart:
			if n == nil {
				return
			}
			p.print(n.Name)
			p.print(n.Body)
			p.print(n.With)

		case *syntax.ImportDecl:
			if n == nil {
				return
			}
			p.print(n.ImportTok)
			p.print(n.FromTok)
			p.print(n.Module)
			p.print(n.Language)
			p.print(n.LBrace, indent)
			p.print(n.List)
			p.print(unindent, n.RBrace)
			p.print(n.With)

		case *syntax.GroupDecl:
			if n == nil {
				return
			}

			p.print(n.Tok)
			p.print(n.Name)
			p.print(n.LBrace, indent)
			p.print(n.Defs)
			p.print(unindent, n.RBrace)
			p.print(n.With)

		case *syntax.FriendDecl:
			if n == nil {
				return
			}
			p.print(n.FriendTok)
			p.print(n.ModuleTok)
			p.print(n.Module)
			p.print(n.With)

		case *syntax.LanguageSpec:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.List)

		case *syntax.RestrictionSpec:
			if n == nil {
				return
			}
			p.print(n.TemplateTok)
			p.print(n.LParen)
			p.print(n.Tok)
			p.print(n.RParen)

		case *syntax.RunsOnSpec:
			if n == nil {
				return
			}
			p.print(n.RunsTok)
			p.print(n.OnTok)
			p.print(n.Comp)

		case *syntax.SystemSpec:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.Comp)

		case *syntax.MtcSpec:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.Comp)

		case *syntax.ReturnSpec:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.Restriction)
			p.print(n.Modif)
			p.print(n.Type)

		case *syntax.FormalPars:
			if n == nil {
				return
			}
			p.print(n.LParen)
			p.print(n.List)
			p.print(n.RParen)

		case *syntax.FormalPar:
			if n == nil {
				return
			}
			p.print(n.Direction)
			p.print(n.TemplateRestriction)
			p.print(n.Modif)
			p.print(n.Type)
			p.print(n.Name)
			p.print(n.ArrayDef)
			p.print(n.AssignTok)
			p.print(n.Value)

		case *syntax.WithSpec:
			if n == nil {
				return
			}
			p.print(n.Tok)
			p.print(n.LBrace, indent)
			p.print(n.List)
			p.print(unindent, n.RBrace)

		case *syntax.WithStmt:
			if n == nil {
				return
			}
			p.print(n.KindTok)
			p.print(n.Override)
			p.print(n.LParen)
			p.print(n.List)
			p.print(n.RParen)
			p.print(n.Value)

		case []*syntax.CaseClause:
			for _, item := range n {
				p.print(item, "")
			}
		case []*syntax.DefKindExpr:
			for i, item := range n {
				p.print(item)
				if i < len(n)-1 {
					p.print(",")
				}
			}
		case []*syntax.Field:
			for i, item := range n {
				p.print(item)
				if i < len(n)-1 {
					p.print(",")
				}
			}
		case []*syntax.FormalPar:
			for i, item := range n {
				p.print(item)
				if i < len(n)-1 {
					p.print(",")
				}
			}
		case []*syntax.ModuleDef:
			for _, item := range n {
				p.print(item, ";", "\n")
			}
		case []*syntax.ParenExpr:
			for i, item := range n {
				p.print(item)
				if i < len(n)-1 {
					p.print(",")
				}
			}
		case []*syntax.ValueDecl:
			for _, item := range n {
				p.print(item, ";", "\n")
			}
		case []*syntax.Declarator:
			for i, item := range n {
				p.print(item)
				if i < len(n)-1 {
					p.print(",")
				} else {
					p.print(";", "\n")
				}
			}
		case []*syntax.WithStmt:
			for _, item := range n {
				p.print(item, ";", "\n")
			}
		case []syntax.Decl:
			for i, item := range n {
				p.print(item)
				if i < len(n)-1 {
					p.print(",")
				}
			}
		case []syntax.Expr:
			for i, item := range n {
				p.print(item)
				if i < len(n)-1 {
					p.print(",")
				}
			}
		case []syntax.Node:
			for i, item := range n {
				p.print(item)
				if i < len(n)-1 {
					p.print(",")
				}
			}
		case []syntax.Stmt:
			for _, item := range n {
				p.print(item, ";", "\n")
			}
		case []syntax.Token:
			for i, item := range n {
				p.print(item)
				if i < len(n)-1 {
					p.print(",")
				}
			}

		case syntax.Token:
			p.print(n.String())

		default:
			if n == nil {
				return
			}

			switch {
			case p.lineStart && n != "\n":
				for i := 0; i < p.indent; i++ {
					fmt.Fprint(p.w, "	")
				}
				p.lineStart = false
			case n == "{", n == "}":
				p.print("\n")
				p.print(n)
				return
			case n == ")", n == ",", n == ";", n == "\n":
				p.ignoreNextSpace = false
			case n == "(":
				p.ignoreNextSpace = true
			case p.ignoreNextSpace:
				p.ignoreNextSpace = false
			default:
				fmt.Fprint(p.w, " ")
			}
			if p.printNewlineNext && (n != ";" && n != ",") {
				p.printNewlineNext = false
				p.print("\n")
				p.print(n)
				return
			}
			fmt.Fprint(p.w, n)
			switch {
			case n == "\n":
				p.lineStart = true
				p.printNewlineNext = false
			case n == "{", n == "}":
				p.printNewlineNext = true
			default:
				p.printNewlineNext = false
			}
		}
	}
}
