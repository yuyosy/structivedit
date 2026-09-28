package selector

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Pattern is the opaque parsed form used by the Core. Its AST stays private to
// this internal package.
type Pattern struct {
	segments []segment
}

type segment interface {
	selectorSegment()
}

type keySegment struct{ key string }
type indexSegment struct{ index int }
type anyKeySegment struct{}
type anyIndexSegment struct{}
type recursiveSegment struct{}

func (keySegment) selectorSegment()       {}
func (indexSegment) selectorSegment()     {}
func (anyKeySegment) selectorSegment()    {}
func (anyIndexSegment) selectorSegment()  {}
func (recursiveSegment) selectorSegment() {}

// ParseError identifies a syntax error using a 0-based UTF-8 byte offset.
type ParseError struct {
	Offset  int
	Message string
}

func (e *ParseError) Error() string {
	if e == nil {
		return "selector syntax error"
	}
	return fmt.Sprintf("selector syntax error at byte %d: %s", e.Offset, e.Message)
}

// Parse parses the small, non-JSONPath selector grammar used by StructiveEdit.
func Parse(input string) (Pattern, error) {
	if !utf8.ValidString(input) {
		return Pattern{}, parseError(firstInvalidUTF8(input), "invalid UTF-8")
	}
	if len(input) == 0 || input[0] != '$' {
		return Pattern{}, parseError(0, "selector must start with '$'")
	}
	parsed := Pattern{segments: make([]segment, 0)}
	for offset := 1; offset < len(input); {
		var next int
		var item segment
		var err *ParseError
		switch input[offset] {
		case '.':
			item, next, err = parseDotSegment(input, offset)
		case '[':
			item, next, err = parseBracketSegment(input, offset)
		default:
			return Pattern{}, parseError(offset, "expected a path segment")
		}
		if err != nil {
			return Pattern{}, err
		}
		parsed.segments = append(parsed.segments, item)
		offset = next
	}
	return parsed, nil
}

func parseDotSegment(input string, offset int) (segment, int, *ParseError) {
	start := offset + 1
	if start >= len(input) {
		return nil, 0, parseError(start, "expected a key or wildcard after '.'")
	}
	switch input[start] {
	case '*':
		if start+1 < len(input) && input[start+1] == '*' {
			return recursiveSegment{}, start + 2, nil
		}
		return anyKeySegment{}, start + 1, nil
	default:
		if !isIdentifierStart(input[start]) {
			return nil, 0, parseError(start, "expected an ASCII key identifier or wildcard")
		}
		end := start + 1
		for end < len(input) && isIdentifierContinue(input[end]) {
			end++
		}
		return keySegment{key: input[start:end]}, end, nil
	}
}

func parseBracketSegment(input string, offset int) (segment, int, *ParseError) {
	start := offset + 1
	if start >= len(input) {
		return nil, 0, parseError(start, "unterminated bracket segment")
	}
	switch input[start] {
	case '*':
		if start+1 >= len(input) || input[start+1] != ']' {
			return nil, 0, parseError(start+1, "expected ']' after '*'")
		}
		return anyIndexSegment{}, start + 2, nil
	case '"':
		key, end, err := parseJSONString(input, start)
		if err != nil {
			return nil, 0, err
		}
		if end >= len(input) || input[end] != ']' {
			return nil, 0, parseError(end, "expected ']' after JSON string key")
		}
		return keySegment{key: key}, end + 1, nil
	default:
		if input[start] < '0' || input[start] > '9' {
			return nil, 0, parseError(start, "expected an index, wildcard, or JSON string key")
		}
		end := start + 1
		for end < len(input) && input[end] >= '0' && input[end] <= '9' {
			end++
		}
		if end-start > 1 && input[start] == '0' {
			return nil, 0, parseError(start+1, "index must not contain leading zeroes")
		}
		if end >= len(input) || input[end] != ']' {
			return nil, 0, parseError(end, "expected ']' after index")
		}
		index, err := strconv.Atoi(input[start:end])
		if err != nil {
			return nil, 0, parseError(start, "index is out of range")
		}
		return indexSegment{index: index}, end + 1, nil
	}
}

func parseJSONString(input string, quote int) (string, int, *ParseError) {
	for offset := quote + 1; offset < len(input); {
		current := input[offset]
		switch {
		case current == '"':
			literal := input[quote : offset+1]
			var value string
			if err := json.Unmarshal([]byte(literal), &value); err != nil {
				return "", 0, parseError(quote, "invalid JSON string key")
			}
			return value, offset + 1, nil
		case current == '\\':
			next, errOffset, message := scanEscape(input, offset)
			if message != "" {
				return "", 0, parseError(errOffset, message)
			}
			offset = next
		case current < 0x20:
			return "", 0, parseError(offset, "unescaped control character in JSON string")
		default:
			_, width := utf8.DecodeRuneInString(input[offset:])
			offset += width
		}
	}
	return "", 0, parseError(quote, "unterminated JSON string key")
}

func scanEscape(input string, slash int) (next int, errOffset int, message string) {
	if slash+1 >= len(input) {
		return 0, slash, "incomplete escape sequence"
	}
	switch input[slash+1] {
	case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
		return slash + 2, 0, ""
	case 'u':
		unit, after, badOffset := scanHexUnit(input, slash)
		if badOffset >= 0 {
			return 0, badOffset, "invalid or incomplete Unicode escape"
		}
		switch {
		case unit >= 0xD800 && unit <= 0xDBFF:
			if after+1 >= len(input) || input[after] != '\\' || input[after+1] != 'u' {
				return 0, slash, "high surrogate must be followed by a low surrogate"
			}
			low, afterLow, badOffset := scanHexUnit(input, after)
			if badOffset >= 0 || low < 0xDC00 || low > 0xDFFF {
				if badOffset >= 0 {
					return 0, badOffset, "invalid or incomplete Unicode escape"
				}
				return 0, after, "expected a low surrogate"
			}
			return afterLow, 0, ""
		case unit >= 0xDC00 && unit <= 0xDFFF:
			return 0, slash, "unpaired low surrogate"
		default:
			return after, 0, ""
		}
	default:
		return 0, slash + 1, "invalid JSON escape"
	}
}

func scanHexUnit(input string, slash int) (unit uint16, next int, badOffset int) {
	digits := slash + 2
	if digits+4 > len(input) {
		return 0, 0, len(input)
	}
	for index := digits; index < digits+4; index++ {
		value, ok := hexValue(input[index])
		if !ok {
			return 0, 0, index
		}
		unit = unit<<4 | uint16(value)
	}
	return unit, digits + 4, -1
}

func hexValue(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	case value >= 'A' && value <= 'F':
		return value - 'A' + 10, true
	default:
		return 0, false
	}
}

func parseError(offset int, message string) *ParseError {
	return &ParseError{Offset: offset, Message: strings.TrimSpace(message)}
}

func firstInvalidUTF8(input string) int {
	for offset := 0; offset < len(input); {
		_, width := utf8.DecodeRuneInString(input[offset:])
		if width == 1 && input[offset] >= utf8.RuneSelf {
			return offset
		}
		offset += width
	}
	return 0
}

func isIdentifierStart(value byte) bool {
	return value == '_' || (value >= 'A' && value <= 'Z') || (value >= 'a' && value <= 'z')
}

func isIdentifierContinue(value byte) bool {
	return isIdentifierStart(value) || (value >= '0' && value <= '9') || value == '-'
}
