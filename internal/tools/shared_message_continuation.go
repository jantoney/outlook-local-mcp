package tools

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	abstractions "github.com/microsoft/kiota-abstractions-go"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
)

const publicGraphOrigin = "graph.microsoft.com"

// validateSharedMessageContinuation verifies a Graph-provided next link stays
// on the public HTTPS Graph origin and the exact decoded owner-view collection
// selected by the current request. The owner and expectedCollection parameters
// are immutable local routing inputs; the function performs no I/O.
func validateSharedMessageContinuation(nextLink, owner, expectedCollection string) error {
	ownerPrefix := "/v1.0/users/" + owner + "/"
	if owner == "" || !strings.HasPrefix(expectedCollection, ownerPrefix) {
		return fmt.Errorf("shared pagination requires an owner-view collection")
	}
	parsed, err := url.Parse(nextLink)
	if err != nil || parsed.User != nil || parsed.Fragment != "" ||
		!strings.EqualFold(parsed.Scheme, "https") ||
		!strings.EqualFold(parsed.Host, publicGraphOrigin) {
		return fmt.Errorf("shared pagination continuation must use the public HTTPS Graph origin")
	}
	decodedPath, err := url.PathUnescape(parsed.EscapedPath())
	if err != nil || decodedPath != expectedCollection {
		return fmt.Errorf("shared pagination continuation changed the authorized owner-view collection")
	}
	return nil
}

// sharedMessageCollectionPath returns the decoded owner-view collection path
// authorized for one shared message read. An empty folderID selects the owner's
// mailbox-wide message collection; otherwise it selects that exact folder.
func sharedMessageCollectionPath(owner, folderID string) string {
	collection := "/v1.0/users/" + owner
	if folderID != "" {
		return collection + "/mailFolders/" + folderID + "/messages"
	}
	return collection + "/messages"
}

// iteratePinnedSharedMessages enumerates the initial shared message page and
// follows subsequent pages only after validating every next link against the
// immutable owner-view collection. The callback controls the result cap; the
// function performs one Graph GET for each accepted continuation and returns
// transport, serialization, or continuation-validation errors.
func iteratePinnedSharedMessages(
	ctx context.Context,
	first models.MessageCollectionResponseable,
	adapter abstractions.RequestAdapter,
	owner, collection string,
	headers *abstractions.RequestHeaders,
	callback func(models.Messageable) bool,
) error {
	page := first
	for {
		for _, message := range page.GetValue() {
			if !callback(message) {
				return nil
			}
		}
		nextLink := page.GetOdataNextLink()
		if nextLink == nil || *nextLink == "" {
			return nil
		}
		if err := validateSharedMessageContinuation(*nextLink, owner, collection); err != nil {
			return err
		}
		parsed, err := url.Parse(*nextLink)
		if err != nil {
			return fmt.Errorf("parse shared pagination continuation: %w", err)
		}
		requestInfo := abstractions.NewRequestInformation()
		requestInfo.Method = abstractions.GET
		requestInfo.SetUri(*parsed)
		if headers != nil {
			requestInfo.Headers.AddAll(headers)
		}
		response, err := adapter.Send(ctx, requestInfo, models.CreateMessageCollectionResponseFromDiscriminatorValue, nil)
		if err != nil {
			return fmt.Errorf("fetch shared message continuation: %w", err)
		}
		var ok bool
		page, ok = response.(models.MessageCollectionResponseable)
		if !ok {
			return fmt.Errorf("shared message continuation returned an invalid collection")
		}
	}
}
