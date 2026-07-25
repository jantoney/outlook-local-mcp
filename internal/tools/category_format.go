package tools

import (
	"fmt"
	"strings"

	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
)

// serializeOutlookCategory converts one Graph category into the complete raw
// category response shape used by list_categories.
//
// Parameters:
//   - category: Graph category returned from the master category collection.
//
// Returns the category ID, display name, and preset color. Nil Graph fields
// become empty strings. The function has no side effects.
func serializeOutlookCategory(category models.OutlookCategoryable) map[string]any {
	color := ""
	if value := category.GetColor(); value != nil {
		color = value.String()
	}
	return map[string]any{
		"id":          graph.SafeStr(category.GetId()),
		"displayName": graph.SafeStr(category.GetDisplayName()),
		"color":       color,
	}
}

// summarizeOutlookCategory selects the intentional compact fields returned by
// list_categories summary mode.
//
// Parameters:
//   - category: raw serialized category map.
//
// Returns a new map containing only displayName and color. The function has no
// side effects.
func summarizeOutlookCategory(category map[string]any) map[string]any {
	return map[string]any{
		"displayName": category["displayName"],
		"color":       category["color"],
	}
}

// FormatOutlookCategoriesText renders category maps as a compact numbered
// human-readable list.
//
// Parameters:
//   - categories: serialized category maps containing displayName and color.
//
// Returns plain text with one category per line and a final total. Empty input
// returns a clear no-categories message. The function has no side effects.
func FormatOutlookCategoriesText(categories []map[string]any) string {
	if len(categories) == 0 {
		return "No Outlook categories found."
	}

	var builder strings.Builder
	for index, category := range categories {
		name, _ := category["displayName"].(string)
		if name == "" {
			name = "(Unnamed category)"
		}
		color, _ := category["color"].(string)
		if color == "" {
			color = "none"
		}
		fmt.Fprintf(&builder, "%d. %s (%s)\n", index+1, name, color)
	}
	fmt.Fprintf(&builder, "\nTotal: %d categories", len(categories))
	return builder.String()
}
