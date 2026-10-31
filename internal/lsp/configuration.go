package lsp

import (
	"context"

	"github.com/nokia/ntt/internal/lsp/protocol"
)

const DIAGNOSTICS_CONFIG_KEY = "ttcn3.experimental.diagnostics.enabled"
const FORMATTER_CONFIG_KEY = "ttcn3.experimental.format.enabled"
const SEMANTIC_TOKENS_CONFIG_KEY = "ttcn3.experimental.semanticTokens.enabled"
const INLAY_HINT_CONFIG_KEY = "ttcn3.experimental.inlayHint.enabled"

func (s *Server) Config(section string) interface{} {
	v, err := s.client.Configuration(context.TODO(), &protocol.ParamConfiguration{
		ConfigurationParams: protocol.ConfigurationParams{
			Items: []protocol.ConfigurationItem{
				{Section: section},
			},
		},
	})
	if err != nil {
		s.Log(context.TODO(), err.Error())
	}
	if len(v) == 1 {
		return v[0]
	}
	return v
}

func (s *Server) didChangeConfiguration(ctx context.Context, _ *protocol.DidChangeConfigurationParams) error {
	regList := make([]protocol.Registration, 0, 3)
	unregList := make([]protocol.Unregistration, 0, 3)

	confRes, ok := s.Config(FORMATTER_CONFIG_KEY).(bool)
	if !ok {
		confRes = false
	}
	if s.clientCapability.HasDynRegForFormatter && s.serverConfig.FormatEnabled != confRes {
		s.serverConfig.FormatEnabled = confRes
		if confRes {
			regList = append(regList, protocol.Registration{
				ID:     "TEXTDOCUMENT_FORMATTING",
				Method: "textDocument/formatting",
				RegisterOptions: protocol.TextDocumentRegistrationOptions{
					DocumentSelector: protocol.DocumentSelector{
						protocol.DocumentFilter{Language: "ttcn3", Scheme: "file", Pattern: "**/*.{ttcn,ttcn3}"},
					},
				}})
		} else {
			unregList = append(unregList, protocol.Unregistration{
				ID:     "TEXTDOCUMENT_FORMATTING",
				Method: "textDocument/formatting"})
		}
	}
	confRes, ok = s.Config(SEMANTIC_TOKENS_CONFIG_KEY).(bool)
	if !ok {
		confRes = false
	}
	if s.clientCapability.HasDynRegForSemTok && s.serverConfig.SemanticTokensEnabled != confRes {
		s.serverConfig.SemanticTokensEnabled = confRes
		if confRes {
			regList = append(regList, protocol.Registration{
				ID:              "TEXTDOCUMENT_SEMANTICTOKENS",
				Method:          "textDocument/semanticTokens",
				RegisterOptions: newSemanticTokens()})
		} else {
			unregList = append(unregList, protocol.Unregistration{
				ID:     "TEXTDOCUMENT_SEMANTICTOKENS",
				Method: "textDocument/semanticTokens"})
		}
	}
	confRes, ok = s.Config(DIAGNOSTICS_CONFIG_KEY).(bool)
	if !ok {
		confRes = false
	}

	if s.serverConfig.DiagnosticsEnabled != confRes {
		s.serverConfig.DiagnosticsEnabled = confRes
		// NOTE: dynamic registration of diagnostics is only available from lsp 3.17 on
	}

	confRes, ok = s.Config(INLAY_HINT_CONFIG_KEY).(bool)
	if !ok {
		confRes = false
	}
	if s.clientCapability.HasDynRegForInlayHint && s.serverConfig.InlayHintEnabled != confRes {
		s.serverConfig.InlayHintEnabled = confRes
		if confRes {
			regList = append(regList, protocol.Registration{
				ID:              "TEXTDOCUMENT_INLAYHINT",
				Method:          "textDocument/inlayHint",
				RegisterOptions: newInlayHintRegistrationOptions()})
		} else {
			unregList = append(unregList, protocol.Unregistration{
				ID:     "TEXTDOCUMENT_INLAYHINT",
				Method: "textDocument/inlayHint"})
		}
	}

	if len(regList) > 0 {
		s.client.RegisterCapability(ctx, &protocol.RegistrationParams{Registrations: regList})
	}
	if len(unregList) > 0 {
		s.client.UnregisterCapability(ctx, &protocol.UnregistrationParams{Unregisterations: unregList})
	}
	return nil
}
