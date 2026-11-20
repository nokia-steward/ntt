package lsp

import (
	"context"
	"os"

	"github.com/nokia/ntt/internal/env"
	"github.com/nokia/ntt/internal/log"
	"github.com/nokia/ntt/internal/lsp/protocol"
)

func (s *Server) setTrace(ctx context.Context, params *protocol.SetTraceParams) error {
	setTrace(params.Value)
	return nil
}

func setTrace(s string) {
	// Ignore client settings when NTT_DEBUG is enabled
	if env := env.Getenv("NTT_DEBUG"); env != "" {
		log.SetGlobalLevel(log.DebugLevel)
		return
	}

	switch s {
	case "verbose":
		log.SetGlobalLevel(log.DebugLevel)
	case "off":
		log.SetGlobalLevel(log.PrintLevel)
	}
}

func (s *Server) toggleDebug(ctx context.Context) (interface{}, error) {
	switch log.GlobalLevel() {
	case log.PrintLevel:
		os.Setenv("NTT_DEBUG", "1")
		log.SetGlobalLevel(log.DebugLevel)
		log.Println("Loglevel set to debug.")
	default:
		os.Unsetenv("NTT_DEBUG")
		log.SetGlobalLevel(log.PrintLevel)
		log.Println("Loglevel set to default.")
	}
	return nil, nil
}
