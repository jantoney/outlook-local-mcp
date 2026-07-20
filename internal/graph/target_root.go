package graph

import (
	"fmt"

	"github.com/desek/outlook-local-mcp/internal/resource"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	graphusers "github.com/microsoftgraph/msgraph-sdk-go/users"
)

// SelectUserRoot validates target and returns exactly one typed generated-SDK
// user root. Recipient views use Me; owner views use Users.ByUserId. The
// function never performs HTTP traffic and never falls back between roots.
func SelectUserRoot(client *msgraphsdk.GraphServiceClient, target resource.Target) (*graphusers.UserItemRequestBuilder, error) {
	if client == nil {
		return nil, fmt.Errorf("resolved target requires a non-nil Graph client")
	}
	if err := target.Validate(); err != nil {
		return nil, fmt.Errorf("invalid resolved target: %w", err)
	}
	switch target.View {
	case resource.MailboxViewRecipient:
		return client.Me(), nil
	case resource.MailboxViewOwner:
		if target.Owner == "" {
			return nil, fmt.Errorf("owner-view target requires an owner locator")
		}
		return client.Users().ByUserId(target.Owner), nil
	default:
		return nil, fmt.Errorf("invalid mailbox view %q", target.View)
	}
}
