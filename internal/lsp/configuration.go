package lsp

import (
	"context"

	"github.com/nokia/ntt/internal/lsp/protocol"
)

// Configuration keys. The new `ttcn3.<feature>.enabled` form is the
// canonical name and defaults to true (i.e. opt-out). The legacy
// `ttcn3.experimental.*` keys are still honoured for backwards
// compatibility with existing user settings.
const (
	DIAGNOSTICS_CONFIG_KEY    = "ttcn3.diagnostics.enabled"
	FORMATTER_CONFIG_KEY      = "ttcn3.format.enabled"
	SEMANTIC_TOKENS_CONFIG_KEY = "ttcn3.semanticTokens.enabled"
	INLAY_HINT_CONFIG_KEY     = "ttcn3.inlayHint.enabled"

	LEGACY_DIAGNOSTICS_CONFIG_KEY    = "ttcn3.experimental.diagnostics.enabled"
	LEGACY_FORMATTER_CONFIG_KEY      = "ttcn3.experimental.format.enabled"
	LEGACY_SEMANTIC_TOKENS_CONFIG_KEY = "ttcn3.experimental.semanticTokens.enabled"
	LEGACY_INLAY_HINT_CONFIG_KEY     = "ttcn3.experimental.inlayHint.enabled"
)

// configBool reads a boolean configuration value from the client.
// It first tries the canonical key; if the client doesn't have one set
// it falls back to the legacy `experimental.` key; and if neither is
// configured it returns def. This is how we ship the previously-gated
// features as default-on without breaking users who had explicitly set
// the old key.
func (s *Server) configBool(canonical, legacy string, def bool) bool {
	if v, ok := s.Config(canonical).(bool); ok {
		return v
	}
	if v, ok := s.Config(legacy).(bool); ok {
		return v
	}
	return def
}

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

	confRes := s.configBool(FORMATTER_CONFIG_KEY, LEGACY_FORMATTER_CONFIG_KEY, true)
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
	confRes = s.configBool(SEMANTIC_TOKENS_CONFIG_KEY, LEGACY_SEMANTIC_TOKENS_CONFIG_KEY, true)
	if s.clientCapability.HasDynRegForSemTok && s.serverConfig.SemantikTokensEnabled != confRes {
		s.serverConfig.SemantikTokensEnabled = confRes
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
	confRes = s.configBool(DIAGNOSTICS_CONFIG_KEY, LEGACY_DIAGNOSTICS_CONFIG_KEY, true)

	if s.serverConfig.DiagnosticsEnabled != confRes {
		s.serverConfig.DiagnosticsEnabled = confRes
		// NOTE: dynamic registration of diagnostics is only available from lsp 3.17 on
	}

	confRes = s.configBool(INLAY_HINT_CONFIG_KEY, LEGACY_INLAY_HINT_CONFIG_KEY, true)
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
