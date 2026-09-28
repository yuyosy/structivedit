package bubbletea

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/yuyosy/structivedit/document"
	"github.com/yuyosy/structivedit/schema"
)

type textInput struct {
	runes   []rune
	cursor  int
	touched bool
}

func newTextInput(value string) textInput {
	runes := []rune(value)
	return textInput{runes: runes, cursor: len(runes)}
}

func (input textInput) String() string { return string(input.runes) }

func (input *textInput) handleKey(key tea.Key) bool {
	if input == nil {
		return false
	}
	switch key.Code {
	case tea.KeyLeft:
		if input.cursor > 0 {
			input.cursor--
		}
		return true
	case tea.KeyRight:
		if input.cursor < len(input.runes) {
			input.cursor++
		}
		return true
	case tea.KeyHome:
		input.cursor = 0
		return true
	case tea.KeyEnd:
		input.cursor = len(input.runes)
		return true
	case tea.KeyBackspace:
		if input.cursor > 0 {
			input.runes = append(input.runes[:input.cursor-1], input.runes[input.cursor:]...)
			input.cursor--
			input.touched = true
		}
		return true
	case tea.KeyDelete:
		if input.cursor < len(input.runes) {
			input.runes = append(input.runes[:input.cursor], input.runes[input.cursor+1:]...)
			input.touched = true
		}
		return true
	}
	if key.Text == "" || key.Mod.Contains(tea.ModCtrl) || key.Mod.Contains(tea.ModAlt) || !utf8.ValidString(key.Text) {
		return false
	}
	inserted := []rune(key.Text)
	result := make([]rune, 0, len(input.runes)+len(inserted))
	result = append(result, input.runes[:input.cursor]...)
	result = append(result, inserted...)
	result = append(result, input.runes[input.cursor:]...)
	input.runes = result
	input.cursor += len(inserted)
	input.touched = true
	return true
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
