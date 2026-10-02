package bubbletea

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/rivo/uniseg"
	"github.com/yuyosy/structivedit/document"
	"github.com/yuyosy/structivedit/schema"
)

type textInput struct {
	runes           []rune
	cursor          int
	touched         bool
	preferredColumn int
	verticalMove    bool
}

type inputLine struct {
	start int
	end   int
}

func newTextInput(value string) textInput {
	runes := []rune(value)
	return textInput{runes: runes, cursor: len(runes)}
}

func (input textInput) String() string { return string(input.runes) }

func displayInputText(runes []rune) string {
	var output strings.Builder
	for _, character := range runes {
		if character == '\t' {
			output.WriteRune('→')
		} else {
			output.WriteRune(character)
		}
	}
	return terminalText(output.String())
}

func (input *textInput) handleKey(key tea.Key, multiline bool) bool {
	if input == nil {
		return false
	}
	input.clampCursor()
	switch key.Code {
	case tea.KeyLeft:
		if input.cursor > 0 {
			input.cursor = input.previousBoundary()
		}
		input.resetVerticalMove()
		return true
	case tea.KeyRight:
		if input.cursor < len(input.runes) {
			input.cursor = input.nextBoundary()
		}
		input.resetVerticalMove()
		return true
	case tea.KeyUp:
		if multiline {
			input.moveVertical(-1)
			return true
		}
		return false
	case tea.KeyDown:
		if multiline {
			input.moveVertical(1)
			return true
		}
		return false
	case tea.KeyHome:
		if multiline {
			lines := input.lines()
			line, _ := input.cursorLineColumn(lines)
			input.cursor = lines[line].start
		} else {
			input.cursor = 0
		}
		input.resetVerticalMove()
		return true
	case tea.KeyEnd:
		if multiline {
			lines := input.lines()
			line, _ := input.cursorLineColumn(lines)
			input.cursor = lines[line].end
		} else {
			input.cursor = len(input.runes)
		}
		input.resetVerticalMove()
		return true
	case tea.KeyBackspace:
		if input.cursor > 0 {
			removeStart := input.previousBoundary()
			input.runes = append(input.runes[:removeStart], input.runes[input.cursor:]...)
			input.cursor = removeStart
			input.touched = true
		}
		input.resetVerticalMove()
		return true
	case tea.KeyDelete:
		if input.cursor < len(input.runes) {
			removeEnd := input.nextBoundary()
			input.runes = append(input.runes[:input.cursor], input.runes[removeEnd:]...)
			input.touched = true
		}
		input.resetVerticalMove()
		return true
	}
	if key.Text == "" || key.Mod.Contains(tea.ModCtrl) || key.Mod.Contains(tea.ModAlt) || !utf8.ValidString(key.Text) {
		return false
	}
	input.insert([]rune(key.Text))
	return true
}

func (input *textInput) insertNewline() {
	if input != nil {
		input.insert([]rune{'\n'})
	}
}

func (input textInput) previousBoundary() int {
	previous, position := 0, 0
	graphemes := uniseg.NewGraphemes(input.String())
	for graphemes.Next() {
		position += len([]rune(graphemes.Str()))
		if position >= input.cursor {
			return previous
		}
		previous = position
	}
	return previous
}

func (input textInput) nextBoundary() int {
	position := 0
	graphemes := uniseg.NewGraphemes(input.String())
	for graphemes.Next() {
		position += len([]rune(graphemes.Str()))
		if position > input.cursor {
			return position
		}
	}
	return len(input.runes)
}

func (input *textInput) insert(inserted []rune) {
	if input == nil || len(inserted) == 0 {
		return
	}
	input.clampCursor()
	result := make([]rune, 0, len(input.runes)+len(inserted))
	result = append(result, input.runes[:input.cursor]...)
	result = append(result, inserted...)
	result = append(result, input.runes[input.cursor:]...)
	input.runes = result
	input.cursor += len(inserted)
	input.touched = true
	input.resetVerticalMove()
}

func (input *textInput) clampCursor() {
	if input.cursor < 0 {
		input.cursor = 0
	}
	if input.cursor > len(input.runes) {
		input.cursor = len(input.runes)
	}
}

func (input *textInput) resetVerticalMove() {
	input.verticalMove = false
}

func (input textInput) lines() []inputLine {
	lines := make([]inputLine, 0, strings.Count(input.String(), "\n")+1)
	start := 0
	for index := 0; index < len(input.runes); index++ {
		character := input.runes[index]
		if character != '\n' && character != '\r' {
			continue
		}
		lines = append(lines, inputLine{start: start, end: index})
		if character == '\r' && index+1 < len(input.runes) && input.runes[index+1] == '\n' {
			index++
		}
		start = index + 1
	}
	lines = append(lines, inputLine{start: start, end: len(input.runes)})
	return lines
}

func (input textInput) cursorLineColumn(lines []inputLine) (int, int) {
	if len(lines) == 0 {
		return 0, 0
	}
	cursor := input.cursor
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(input.runes) {
		cursor = len(input.runes)
	}
	for index, line := range lines {
		if cursor <= line.end {
			return index, cursor - line.start
		}
		if index+1 < len(lines) && cursor < lines[index+1].start {
			return index, line.end - line.start
		}
	}
	last := len(lines) - 1
	return last, lines[last].end - lines[last].start
}

func (input *textInput) moveVertical(delta int) {
	lines := input.lines()
	lineIndex, _ := input.cursorLineColumn(lines)
	if !input.verticalMove {
		line := lines[lineIndex]
		input.preferredColumn = lipgloss.Width(displayInputText(input.runes[line.start:input.cursor]))
		input.verticalMove = true
	}
	target := lineIndex + delta
	if target < 0 || target >= len(lines) {
		return
	}
	line := lines[target]
	input.cursor = line.start + runeIndexAtCellColumn(input.runes[line.start:line.end], input.preferredColumn)
}

func runeIndexAtCellColumn(runes []rune, column int) int {
	if column <= 0 {
		return 0
	}
	position, cells := 0, 0
	graphemes := uniseg.NewGraphemes(string(runes))
	for graphemes.Next() {
		cells += lipgloss.Width(displayInputText([]rune(graphemes.Str())))
		if cells > column {
			return position
		}
		position += len([]rune(graphemes.Str()))
	}
	return position
}

func scalarText(kind document.ScalarKind, value any) string {
	switch kind {
	case document.ScalarString:
		return value.(string)
	case document.ScalarBool:
		return strconv.FormatBool(value.(bool))
	case document.ScalarInteger:
		return strconv.FormatInt(value.(int64), 10)
	case document.ScalarFloat:
		return strconv.FormatFloat(value.(float64), 'g', -1, 64)
	case document.ScalarNull:
		return "null"
	default:
		return ""
	}
}

func parseScalarInput(kind document.ScalarKind, text string) (any, error) {
	switch kind {
	case document.ScalarString:
		return text, nil
	case document.ScalarBool:
		switch strings.ToLower(strings.TrimSpace(text)) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return nil, fmt.Errorf("enter true or false")
		}
	case document.ScalarInteger:
		value, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("enter a signed 64-bit integer")
		}
		return value, nil
	case document.ScalarFloat:
		trimmed := strings.TrimSpace(strings.ToLower(text))
		switch trimmed {
		case ".inf", "+.inf", "inf", "+inf":
			return math.Inf(1), nil
		case "-.inf", "-inf":
			return math.Inf(-1), nil
		case ".nan", "+.nan", "nan", "+nan":
			return math.NaN(), nil
		case "-.nan", "-nan":
			return math.Copysign(math.NaN(), -1), nil
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
		if err != nil {
			return nil, fmt.Errorf("enter a number")
		}
		return value, nil
	case document.ScalarNull:
		if strings.EqualFold(strings.TrimSpace(text), "null") || strings.TrimSpace(text) == "~" {
			return nil, nil
		}
		return nil, fmt.Errorf("enter null")
	default:
		return nil, document.ErrInvalidScalar
	}
}

func parseSchemaInput(kind schema.Kind, text string) (any, error) {
	switch kind {
	case schema.StringKind:
		return text, nil
	case schema.BoolKind:
		return parseScalarInput(document.ScalarBool, text)
	case schema.IntegerKind:
		return parseScalarInput(document.ScalarInteger, text)
	case schema.FloatKind:
		return parseScalarInput(document.ScalarFloat, text)
	case schema.NullKind:
		return parseScalarInput(document.ScalarNull, text)
	default:
		return nil, document.ErrInvalidScalar
	}
}

func defaultInputText(value any) string {
	if value == nil {
		return "null"
	}
	return fmt.Sprint(value)
}
