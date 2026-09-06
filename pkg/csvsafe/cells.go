package csvsafe

import (
	"strings"
	"unicode"
)

// Row neutralizes values that spreadsheet applications could interpret as
// formulas while preserving the original data for human-readable exports.
func Row(values ...string) []string {
	safeValues := make([]string, len(values))
	for index, value := range values {
		safeValues[index] = Cell(value)
	}
	return safeValues
}

// Cell prefixes formula-like content with an apostrophe. Leading whitespace
// is inspected because spreadsheet parsers may ignore it before evaluation.
func Cell(value string) string {
	trimmed := strings.TrimLeftFunc(value, unicode.IsSpace)
	if trimmed == "" {
		return value
	}
	switch trimmed[0] {
	case '=', '+', '-', '@':
		return "'" + value
	default:
		return value
	}
}
