package lsp

import (
	"context"
	"os"
	"path"

	"github.com/nokia/ntt/internal/log"
	"github.com/nokia/ntt/internal/lsp/jsonrpc2"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/project"
	errors "golang.org/x/xerrors"
)

type PlainTextHover struct {
	mapOrConnectRefs string
}
type MarkdownHover struct {
	mapOrConnectLinks string
}

func (s *Server) registerSemanticTokensIfNoDynReg() *protocol.SemanticTokensRegistrationOptions {
	if s.clientCapability.HasDynRegForSemTok {
		// NOTE:
		// if client has the capability, postpone registration and
		// make it dependent on the provided configuration which is
		// available earlyest during `ìnitialized` notification handling
		return nil
	}
	return newSemanticTokens()
}

func (s *Server) registerFormatterIfNoDynReg() bool {
	return !s.clientCapability.HasDynRegForFormatter
}

func (s *Server) registerInlayHintIfNoDynReg() *protocol.InlayHintRegistrationOptions {
	if s.clientCapability.HasDynRegForInlayHint {
		return nil
	}
	return newInlayHintRegistrationOptions()
}

func newInlayHintRegistrationOptions() *protocol.InlayHintRegistrationOptions {
	return &protocol.InlayHintRegistrationOptions{
		InlayHintOptions: protocol.InlayHintOptions{
			ResolveProvider: false,
			WorkDoneProgressOptions: protocol.WorkDoneProgressOptions{
				WorkDoneProgress: false,
			},
		},
		TextDocumentRegistrationOptions: protocol.TextDocumentRegistrationOptions{
			DocumentSelector: protocol.DocumentSelector{
				protocol.DocumentFilter{Language: "ttcn3", Scheme: "file", Pattern: "**/*.{ttcn,ttcn3}"},
			},
		},
		StaticRegistrationOptions: protocol.StaticRegistrationOptions{
			ID: "TEXTDOCUMENT_INLAYHINT",
		},
	}
}

func newSemanticTokens() *protocol.SemanticTokensRegistrationOptions {
	return &protocol.SemanticTokensRegistrationOptions{

		TextDocumentRegistrationOptions: protocol.TextDocumentRegistrationOptions{
			DocumentSelector: protocol.DocumentSelector{
				protocol.DocumentFilter{Language: "ttcn3", Scheme: "file", Pattern: "**/*.{ttcn,ttcn3}"},
			},
		},
		SemanticTokensOptions: protocol.SemanticTokensOptions{
			Legend: protocol.SemanticTokensLegend{
				TokenTypes:     TokenTypes,
				TokenModifiers: TokenModifiers,
			},
			Range: true,
			Full:  true,
		}}
}
func (s *Server) initialize(ctx context.Context, params *protocol.ParamInitialize) (*protocol.InitializeResult, error) {
	s.stateMu.Lock()
	state := s.state
	s.stateMu.Unlock()
	if state >= serverInitializing {
		return nil, errors.Errorf("%w: initialize called while server in %v state", jsonrpc2.ErrInvalidRequest, s.state)
	}
	s.stateMu.Lock()
	s.state = serverInitializing
	s.stateMu.Unlock()
	s.evaluateClientCapabilities(params)
	s.pendingFolders = params.WorkspaceFolders
	if len(s.pendingFolders) == 0 && params.RootURI != "" {
		s.pendingFolders = []protocol.WorkspaceFolder{{
			URI:  string(params.RootURI),
			Name: path.Base(string(params.RootURI.SpanURI())),
		}}
	}

	setTrace(params.Trace)

	version := "devel"
	if v, ok := ctx.Value("version").(string); ok {
		version = v
	}

	return &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			InlayHintProvider:               s.registerInlayHintIfNoDynReg(),
			// Advertise the kinds we actually emit so clients can
			// surface them in their menus (e.g. "Source Action"
			// in VS Code) and pre-filter via the `Only` field.
			CodeActionProvider: protocol.CodeActionOptions{
				CodeActionKinds: []protocol.CodeActionKind{
					protocol.QuickFix,
					protocol.SourceOrganizeImports,
				},
			},
			CompletionProvider: protocol.CompletionOptions{
				TriggerCharacters: []string{"."},
				ResolveProvider:   true,
			},
			CallHierarchyProvider: true,
			DefinitionProvider:              true,
			TypeDefinitionProvider:          true,
			ImplementationProvider:          false,
			DocumentFormattingProvider:      s.registerFormatterIfNoDynReg(),
			DocumentRangeFormattingProvider: false,
			DocumentSymbolProvider:          true,
			WorkspaceSymbolProvider:         true,
			FoldingRangeProvider:            true,
			HoverProvider:                   true,
			DocumentHighlightProvider:       true,
			DocumentLinkProvider:            protocol.DocumentLinkOptions{},
			ReferencesProvider:              true,
			RenameProvider: protocol.RenameOptions{
				PrepareProvider: true,
			},
			SignatureHelpProvider: protocol.SignatureHelpOptions{
				TriggerCharacters:   []string{"(", ","},
				RetriggerCharacters: []string{","},
			},
			TextDocumentSync: &protocol.TextDocumentSyncOptions{
				// Incremental sync sends only the edited range
				// per keystroke. We splice it into our cached
				// content (see didChange) instead of having the
				// client retransmit the whole file every time.
				Change:    protocol.Incremental,
				OpenClose: true,
				Save: protocol.SaveOptions{
					IncludeText: false,
				},
			},
			SemanticTokensProvider: s.registerSemanticTokensIfNoDynReg(),
			Workspace: protocol.Workspace5Gn{
				WorkspaceFolders: protocol.WorkspaceFolders4Gn{
					Supported:           true,
					ChangeNotifications: "workspace/didChangeWorkspaceFolders",
				},
			},
		},
		ServerInfo: struct {
			Name    string `json:"name"`
			Version string `json:"version,omitempty"`
		}{Name: "ntt", Version: version},
	}, nil
}

func (s *Server) evaluateClientCapabilities(params *protocol.ParamInitialize) {
	for _, format := range params.Capabilities.TextDocument.Hover.ContentFormat {
		if format == "markdown" {
			s.clientCapability.HoverContent = new(MarkdownHover)
			break
		}
	}
	if s.clientCapability.HoverContent == nil {
		s.clientCapability.HoverContent = new(PlainTextHover)
	}
	s.clientCapability.HasDynRegForDiagnostics = false // NOTE: available only from LSP 3.17 on
	s.clientCapability.HasDynRegForFormatter = params.Capabilities.TextDocument.Formatting.DynamicRegistration
	s.clientCapability.HasDynRegForSemTok = params.Capabilities.TextDocument.SemanticTokens.DynamicRegistration
	s.clientCapability.HasDynRegForInlayHint = params.Capabilities.TextDocument.InlayHint.DynamicRegistration
}

func (s *Server) initialized(ctx context.Context, params *protocol.InitializedParams) error {
	s.stateMu.Lock()
	s.state = serverInitialized
	s.stateMu.Unlock()

	s.didChangeConfiguration(ctx, &protocol.DidChangeConfigurationParams{})
	for _, folder := range s.pendingFolders {
		log.Printf("Scanning %q for possible TTCN-3 suites\n", folder.URI)
		for _, root := range project.Discover(folder.URI) {
			s.AddSuite(root)
		}
	}
	return nil
}

func (s *Server) shutdown(ctx context.Context) error {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.state < serverInitialized {
		log.Verbose("language server shutdown without initialization")
	}
	s.state = serverShutDown
	return nil
}

func (s *Server) exit(ctx context.Context) error {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.state != serverShutDown {
		os.Exit(1)
	}
	os.Exit(0)
	return nil
}
