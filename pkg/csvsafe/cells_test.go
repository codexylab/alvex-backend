package csvsafe

import "testing"

func TestCellNeutralizesSpreadsheetFormulas(t *testing.T) {
	testCases := map[string]string{
		"=HYPERLINK(\"https://example.com\")": "'=HYPERLINK(\"https://example.com\")",
		" +SUM(1,2)":                          "' +SUM(1,2)",
		"-2+3":                                "'-2+3",
		"@IMPORTXML(A1)":                      "'@IMPORTXML(A1)",
		"ordinary text":                       "ordinary text",
		"":                                    "",
	}
	for input, expected := range testCases {
		if actual := Cell(input); actual != expected {
			t.Errorf("Cell(%q): expected %q, got %q", input, expected, actual)
		}
	}
}
