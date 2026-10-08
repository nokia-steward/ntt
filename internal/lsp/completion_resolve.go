package lsp

import (
	"context"

	"github.com/nokia/ntt/internal/lsp/protocol"
)

// resolveCompletionItem implements completionItem/resolve. The completion
// handler currently returns fully-populated CompletionItems so there is
// nothing more to resolve, but advertising the capability lets editors
// avoid a server roundtrip for the (eventual) lazy-load fallback path.
//
// We do, however, populate the deprecated `Detail` field from the item's
// label when it is empty, since some editors (notably VS Code) collapse
// to "no description" otherwise.
func (s *Server) resolveCompletionItem(ctx context.Context, item *protocol.CompletionItem) (*protocol.CompletionItem, error) {
	if item == nil {
		return nil, nil
	}
	if item.Detail == "" {
		item.Detail = item.Label
	}
	return item, nil
}
