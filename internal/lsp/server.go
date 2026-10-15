// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package lsp implements LSP for gopls.
package lsp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"unicode"

	"github.com/acarl005/stripansi"
	"github.com/nokia/ntt/internal/env"
	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/log"
	"github.com/nokia/ntt/internal/lsp/jsonrpc2"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	errors "golang.org/x/xerrors"
)

func NewServer(stream jsonrpc2.Stream) *Server {
	return &Server{
		conn:  jsonrpc2.NewConn(stream),
		files: make(map[*fs.File]bool),
		// Default-on for the features that used to live behind
		// `ttcn3.experimental.*.enabled`. Users can still opt out
		// via the new `ttcn3.<feature>.enabled` keys (see
		// configuration.go).
		serverConfig: Config{
			DiagnosticsEnabled:    true,
			FormatEnabled:         true,
			SemantikTokensEnabled: true,
			InlayHintEnabled:      true,
		},
	}
}

func (s *Server) Serve(ctx context.Context) error {
	// Ignore client settings when NTT_DEBUG is enabled
	if env := env.Getenv("NTT_DEBUG_FILE"); env == "" {
		log.SetGlobalLogger(s)
	}
	s.client = protocol.ClientDispatcher(s.conn)
	ctx = protocol.WithClient(ctx, s.client)
	handler := protocol.ServerHandler(s, jsonrpc2.MethodNotFound)
	s.conn.Go(ctx, protocol.Handlers(handler))
	<-s.conn.Done()
	return s.conn.Err()
}

type serverState int

const (
	serverCreated      = serverState(iota)
	serverInitializing // set once the server has received "initialize" request
	serverInitialized  // set once the server has received "initialized" request
	serverShutDown
)

func (s serverState) String() string {
	switch s {
	case serverCreated:
		return "created"
	case serverInitializing:
		return "initializing"
	case serverInitialized:
		return "initialized"
	case serverShutDown:
		return "shutDown"
	}
	return fmt.Sprintf("(unknown state: %d)", int(s))
}

type HoverContentProvider interface {
	Print(sign string, comment string, posRef string) protocol.MarkupContent
	LinkForNode(def *ttcn3.Node) string
	GatherMapOrConnectLinks(fileName string, line uint32, uri protocol.DocumentURI)
	Reset()
}
type ClientCapability struct {
	HoverContent            HoverContentProvider
	HasDynRegForDiagnostics bool
	HasDynRegForFormatter   bool
	HasDynRegForSemTok      bool
	HasDynRegForInlayHint   bool
}
type Config struct {
	DiagnosticsEnabled    bool
	FormatEnabled         bool
	SemantikTokensEnabled bool
	InlayHintEnabled      bool

	// FormatPrintWidth is the soft right margin used by the wrapping
	// formatter. Zero falls back to the default (100 columns).
	FormatPrintWidth int
}

// Server implements the protocol.Server interface.
type Server struct {
	conn             jsonrpc2.Conn
	client           protocol.Client
	clientCapability ClientCapability
	stateMu          sync.Mutex
	state            serverState

	// folders is only valid between initialize and initialized, and holds the
	// set of folders to build views for when we are ready
	pendingFolders []protocol.WorkspaceFolder

	Suites

	filesMu sync.Mutex
	files   map[*fs.File]bool

	diagsMu sync.Mutex
	diags   map[string][]protocol.Diagnostic

	serverConfig Config
}

func (s *Server) Fatal(ctx context.Context, msg string) {
	s.client.ShowMessage(ctx, &protocol.ShowMessageParams{
		Type:    protocol.Error,
		Message: msg,
	})
}

func (s *Server) Info(ctx context.Context, msg string) {
	s.client.ShowMessage(ctx, &protocol.ShowMessageParams{
		Type:    protocol.Info,
		Message: msg,
	})
}

func (s *Server) Log(ctx context.Context, msg string) {
	s.client.LogMessage(ctx, &protocol.LogMessageParams{
		Type:    protocol.Log,
		Message: stripansi.Strip(msg),
	})
}

func (s *Server) Output(level log.Level, msg string) error {
	if level <= log.GlobalLevel() {
		s.Log(context.TODO(), strings.TrimRightFunc(msg, unicode.IsSpace))
	}
	return nil
}

func (s *Server) Write(p []byte) (n int, err error) {
	str := string(p)
	s.Log(context.Background(), str)
	return len(str), nil

}

func (s *Server) cancelRequest(ctx context.Context, params *protocol.CancelParams) error {
	return nil
}

func (s *Server) nonstandardRequest(ctx context.Context, method string, params interface{}) (interface{}, error) {
	return nil, notImplemented(method)
}

func notImplemented(method string) error {
	return errors.Errorf("%w: %q not yet implemented", jsonrpc2.ErrMethodNotFound, method)
}

//go:generate helper/helper -d protocol/tsserver.go -o server_gen.go -u .
