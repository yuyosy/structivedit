package bubbletea

import (
	"os"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
)

type lineSegment struct {
	text  string
	style lipgloss.Style
}

type terminalStyles struct {
	plain          lipgloss.Style
	header         lipgloss.Style
	muted          lipgloss.Style
	saved          lipgloss.Style
	modified       lipgloss.Style
	cursor         lipgloss.Style
	tree           lipgloss.Style
	key            lipgloss.Style
	focus          lipgloss.Style
	shortcutKey    lipgloss.Style
	readOnly       lipgloss.Style
	stringValue    lipgloss.Style
	booleanValue   lipgloss.Style
	numberValue    lipgloss.Style
	nullValue      lipgloss.Style
	containerValue lipgloss.Style
	referenceValue lipgloss.Style
	error          lipgloss.Style
	warning        lipgloss.Style
	info           lipgloss.Style
	inputLabel     lipgloss.Style
	inputPath      lipgloss.Style
	inputText      lipgloss.Style
	contextArea    lipgloss.Style
	inputArea      lipgloss.Style
}

func newTerminalStyles(dark bool) terminalStyles {
	color := func(light, darkColor string) lipgloss.Style {
		if dark {
			return lipgloss.NewStyle().Foreground(lipgloss.Color(darkColor))
		}
		return lipgloss.NewStyle().Foreground(lipgloss.Color(light))
	}
	inputBackground := lipgloss.Color("254")
	contextBackground := lipgloss.Color("252")
	if dark {
		inputBackground = lipgloss.Color("236")
		contextBackground = lipgloss.Color("238")
	}
	contextArea := lipgloss.NewStyle().Background(contextBackground)
	inputArea := lipgloss.NewStyle().Background(inputBackground)
	return terminalStyles{
		plain:          lipgloss.NewStyle(),
		header:         color("4", "14").Bold(true),
		muted:          color("0", "8"),
		saved:          color("2", "10").Bold(true),
		modified:       color("3", "11").Bold(true),
		cursor:         color("3", "11").Bold(true),
		tree:           color("4", "12"),
		key:            color("6", "14"),
		focus:          color("3", "11").Bold(true),
		shortcutKey:    lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true),
		readOnly:       color("0", "8"),
		stringValue:    color("2", "10"),
		booleanValue:   color("6", "14"),
		numberValue:    color("5", "13"),
		nullValue:      color("0", "8"),
		containerValue: color("4", "12"),
		referenceValue: color("130", "208"),
		error:          color("1", "9").Bold(true),
		warning:        color("3", "11").Bold(true),
		info:           color("6", "14"),
		inputLabel:     color("0", "8").Background(contextBackground),
		inputPath:      color("4", "14").Background(contextBackground).Bold(true),
		inputText:      color("0", "15").Background(inputBackground),
		contextArea:    contextArea,
		inputArea:      inputArea,
	}
}

func (model *Model) renderStyledLine(width int, segments ...lineSegment) string {
	if width <= 0 {
		return ""
	}
	length := 0
	for _, segment := range segments {
		length += len([]rune(segment.text))
	}
	truncated := length > width
	if truncated && width == 1 {
		return "…"
	}
	remaining := width
	if truncated {
		remaining--
	}

	colors := model.colorsActive()
	var output strings.Builder
	for _, segment := range segments {
		if remaining == 0 {
			break
		}
		runes := []rune(segment.text)
		count := len(runes)
		if count > remaining {
			count = remaining
		}
		text := string(runes[:count])
		if colors {
			text = segment.style.Render(text)
		}
		output.WriteString(text)
		remaining -= count
	}
	if truncated {
		output.WriteRune('…')
	}
	return output.String()
}

func (model *Model) colorsActive() bool {
	return model != nil && model.colorsEnabled && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
}

func (model *Model) valueStyle(value string) lipgloss.Style {
	switch {
	case strings.HasPrefix(value, "[str]"):
		return model.styles.stringValue
	case strings.HasPrefix(value, "[bool]"):
		return model.styles.booleanValue
	case strings.HasPrefix(value, "[int]") || strings.HasPrefix(value, "[float]"):
		return model.styles.numberValue
	case strings.HasPrefix(value, "[null]"):
		return model.styles.nullValue
	case strings.HasPrefix(value, "[map]") || strings.HasPrefix(value, "[seq]"):
		return model.styles.containerValue
	case strings.HasPrefix(value, "[ref]") || strings.HasPrefix(value, "->"):
		return model.styles.referenceValue
	default:
		return model.styles.plain
	}
}

func (model *Model) issueStyle(issue string) lipgloss.Style {
	switch {
	case strings.HasPrefix(issue, "[E]"):
		return model.styles.error
	case strings.HasPrefix(issue, "[W]"):
		return model.styles.warning
	case issue != "":
		return model.styles.info
	default:
		return model.styles.plain
	}
}
