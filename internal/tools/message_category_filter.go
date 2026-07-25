package tools

import (
	"fmt"
	"strings"

	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/validate"
)

// parseMessageCategoryFilter validates and normalizes list_messages category
// arguments.
//
// Parameters:
//   - categories: comma-separated exact Outlook category display names.
//   - match: requested match mode, either "any", "all", or empty for "any".
//
// Returns trimmed category names, the normalized match mode, and an error for
// excessive input, an unsupported mode, or a mode supplied without categories.
// The function has no side effects.
func parseMessageCategoryFilter(categories, match string) ([]string, string, error) {
	if err := validate.ValidateStringLength(categories, "categories", validate.MaxCategoriesLen); err != nil {
		return nil, "", err
	}
	names := splitCategories(categories)
	matchProvided := strings.TrimSpace(match) != ""
	if match == "" {
		match = "any"
	}
	if match != "any" && match != "all" {
		return nil, "", fmt.Errorf("category_match must be one of: any, all")
	}
	if len(names) == 0 {
		if matchProvided {
			return nil, "", fmt.Errorf("category_match requires categories")
		}
		return nil, match, nil
	}
	return names, match, nil
}

// buildMessageCategoryFilter creates exact OData predicates for the requested
// Outlook category names.
//
// Parameters:
//   - categories: normalized category display names.
//   - match: "any" to join category predicates with OR or "all" for AND.
//
// Returns an empty string for no categories or a parenthesized OData expression.
// Category values are escaped as OData literals. The function has no side effects.
func buildMessageCategoryFilter(categories []string, match string) string {
	if len(categories) == 0 {
		return ""
	}
	joiner := " or "
	if match == "all" {
		joiner = " and "
	}
	predicates := make([]string, 0, len(categories))
	for index, category := range categories {
		variable := fmt.Sprintf("c%d", index)
		predicates = append(predicates, fmt.Sprintf(
			"categories/any(%s:%s eq '%s')",
			variable, variable, graph.EscapeOData(category),
		))
	}
	return "(" + strings.Join(predicates, joiner) + ")"
}
