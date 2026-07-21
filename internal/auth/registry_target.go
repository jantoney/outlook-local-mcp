package auth

import (
	"fmt"

	"github.com/desek/outlook-local-mcp/internal/resource"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
)

// ResolveRegisteredTarget atomically resolves one account target and captures
// the Graph client that is current for the same registry state.
//
// The label selects the registered account, request declares the required
// family and capability, and codec verifies optional signed provenance. It
// returns the authorized target snapshot and current client. No Graph request
// is made. An error is returned for a missing account, missing client, or any
// target authorization failure.
func (r *AccountRegistry) ResolveRegisteredTarget(label string, request TargetRequest, codec *resource.ReferenceCodec) (ResolvedTarget, *msgraphsdk.GraphServiceClient, error) {
	if r == nil {
		return ResolvedTarget{}, nil, fmt.Errorf("account registry is unavailable")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.accounts[label]
	if !ok {
		return ResolvedTarget{}, nil, fmt.Errorf("account %q not found", label)
	}
	if entry.Client == nil {
		return ResolvedTarget{}, nil, fmt.Errorf("account %q has no Graph client", entry.Label)
	}
	resolved, err := ResolveTarget(entry, request, codec)
	if err != nil {
		return ResolvedTarget{}, nil, err
	}
	return resolved, entry.Client, nil
}
