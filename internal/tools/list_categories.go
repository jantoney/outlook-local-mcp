package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/logging"
	"github.com/mark3labs/mcp-go/mcp"
	msgraphcore "github.com/microsoftgraph/msgraph-sdk-go-core"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// NewHandleListCategories creates a read-only handler that retrieves the
// selected account's own Outlook master category list.
//
// Parameters:
//   - retryCfg: retry configuration for the initial Graph collection request.
//   - timeout: maximum duration for Graph category retrieval and pagination.
//
// Returns an MCP handler supporting text, summary, and raw output tiers.
// Side effects are limited to GET requests against
// /me/outlook/masterCategories. Shared mailbox targets are rejected. Graph and
// timeout errors use the existing redacted tool-error contracts.
func NewHandleListCategories(retryCfg graph.RetryConfig, timeout time.Duration) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		logger := logging.Logger(ctx)
		start := time.Now()

		target, err := mailTargetFromContext(ctx)
		if err != nil {
			return mcp.NewToolResultError("no account selected"), nil
		}
		client, err := GraphClient(ctx)
		if err != nil {
			return mcp.NewToolResultError("no account selected"), nil
		}
		if target.isShared() {
			return mcp.NewToolResultError("list_categories supports only the selected account's own mailbox"), nil
		}
		outputMode, err := ValidateOutputMode(request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		timeoutCtx, cancel := graph.WithTimeout(ctx, timeout)
		defer cancel()

		query := &users.ItemOutlookMasterCategoriesRequestBuilderGetQueryParameters{
			Select: []string{"id", "displayName", "color"},
		}
		config := &users.ItemOutlookMasterCategoriesRequestBuilderGetRequestConfiguration{
			QueryParameters: query,
		}

		var response models.OutlookCategoryCollectionResponseable
		err = graph.RetryGraphCall(ctx, retryCfg, func() error {
			var graphErr error
			response, graphErr = target.root.Outlook().MasterCategories().Get(timeoutCtx, config)
			return graphErr
		})
		if err != nil {
			if graph.IsTimeoutError(err) {
				return mcp.NewToolResultError(graph.TimeoutErrorMessage(int(timeout.Seconds()))), nil
			}
			return mcp.NewToolResultError(target.graphError(err)), nil
		}
		if response == nil {
			return mcp.NewToolResultError("Microsoft Graph returned an empty category collection response"), nil
		}

		categories := make([]map[string]any, 0, len(response.GetValue()))
		iterator, err := msgraphcore.NewPageIterator[models.OutlookCategoryable](
			response,
			client.GetAdapter(),
			models.CreateOutlookCategoryCollectionResponseFromDiscriminatorValue,
		)
		if err == nil {
			err = iterator.Iterate(timeoutCtx, func(category models.OutlookCategoryable) bool {
				categories = append(categories, serializeOutlookCategory(category))
				return true
			})
		}
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to iterate Outlook categories: %s", err.Error())), nil
		}

		logger.Info("tool completed", "duration", time.Since(start), "count", len(categories))
		if outputMode == "text" {
			return mcp.NewToolResultText(FormatOutlookCategoriesText(categories)), nil
		}

		payload := categories
		if outputMode == "summary" {
			payload = make([]map[string]any, len(categories))
			for index, category := range categories {
				payload[index] = summarizeOutlookCategory(category)
			}
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to serialize Outlook categories: %s", err.Error())), nil
		}
		return mcp.NewToolResultText(string(encoded)), nil
	}
}
