package bubbletea

import (
	"strconv"
	"strings"
	"unicode"
)

// terminalText escapes input controls before styles add their own ANSI codes.
// Bidirectional formatting controls are made visible as well; ZWJ and other
// characters used by ordinary grapheme clusters remain intact.
func terminalText(value string) string {
	if strings.IndexFunc(value, terminalControl) < 0 {
		return value
	}
	var output strings.Builder
	for _, r := range value {
		if terminalControl(r) {
			quoted := strconv.QuoteRune(r)
			output.WriteString(quoted[1 : len(quoted)-1])
		} else {
			output.WriteRune(r)
		}
	}
	return output.String()
}

func terminalControl(r rune) bool {
	return unicode.IsControl(r) || r == '\u061c' || r == '\u200e' || r == '\u200f' ||
		(r >= '\u202a' && r <= '\u202e') || (r >= '\u2066' && r <= '\u2069')
}
