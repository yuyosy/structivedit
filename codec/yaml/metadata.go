package yaml

import (
	"strings"

	"github.com/yuyosy/structivedit/document"
	yamlv3 "go.yaml.in/yaml/v3"
)

type scalarStyle uint8

const (
	scalarPlain scalarStyle = iota
	scalarSingleQuoted
	scalarDoubleQuoted
	scalarLiteral
	scalarFolded
)

type collectionStyle uint8

const (
	collectionBlock collectionStyle = iota
	collectionFlow
)

type chompMode uint8

const (
	chompClip chompMode = iota
	chompStrip
	chompKeep
)

// nodeMetadata preserves the YAML presentation details associated with one
// stable Document NodeID. yaml.Node remains private to this Codec package.
type nodeMetadata struct {
	HeadComment     string
	LineComment     string
	FootComment     string
	Anchor          string
	Tag             string
	AliasName       string
	ScalarStyle     scalarStyle
	CollectionStyle collectionStyle
	Chomp           chompMode
	SourceValue     string
	ExplicitTag     bool
	HasSource       bool
}

type session struct {
	document       *document.Document
	metadata       map[document.NodeID]nodeMetadata
	sourceMetadata nodeMetadata
}

func metadataFromNode(node *yamlv3.Node, sourceLines []string) nodeMetadata {
	metadata := nodeMetadata{
		HeadComment: node.HeadComment,
		LineComment: node.LineComment,
		FootComment: node.FootComment,
		Anchor:      node.Anchor,
		Tag:         node.Tag,
		SourceValue: node.Value,
		ExplicitTag: node.Style&yamlv3.TaggedStyle != 0,
		HasSource:   true,
	}
	if node.Kind == yamlv3.AliasNode {
		metadata.AliasName = node.Value
	}
	switch {
	case node.Style&yamlv3.SingleQuotedStyle != 0:
		metadata.ScalarStyle = scalarSingleQuoted
	case node.Style&yamlv3.DoubleQuotedStyle != 0:
		metadata.ScalarStyle = scalarDoubleQuoted
	case node.Style&yamlv3.LiteralStyle != 0:
		metadata.ScalarStyle = scalarLiteral
		metadata.Chomp = sourceChomp(node, sourceLines)
	case node.Style&yamlv3.FoldedStyle != 0:
		metadata.ScalarStyle = scalarFolded
		metadata.Chomp = sourceChomp(node, sourceLines)
	default:
		metadata.ScalarStyle = scalarPlain
	}
	if node.Style&yamlv3.FlowStyle != 0 {
		metadata.CollectionStyle = collectionFlow
	}
	return metadata
}

func sourceChomp(node *yamlv3.Node, lines []string) chompMode {
	if node == nil || node.Line < 1 {
		return chompClip
	}
	if node.Line > len(lines) {
		return chompClip
	}
	line := lines[node.Line-1]
	column := node.Column - 1
	if column < 0 || column >= len(line) {
		column = 0
	}
	tail := line[column:]
	marker := blockScalarMarker(tail)
	if marker < 0 {
		return chompClip
	}
	for _, modifier := range tail[marker+1:] {
		switch modifier {
		case '+':
			return chompKeep
		case '-':
			return chompStrip
		case ' ', '\t', '#':
			return chompClip
		}
	}
	return chompClip
}

func blockScalarMarker(header string) int {
	for index := 0; index < len(header); {
		for index < len(header) && (header[index] == ' ' || header[index] == '\t') {
			index++
		}
		if index >= len(header) || header[index] == '#' {
			return -1
		}
		if header[index] == '!' && index+1 < len(header) && header[index+1] == '<' {
			end := strings.IndexByte(header[index+2:], '>')
			if end < 0 {
				return -1
			}
			index += end + 3
			continue
		}
		switch header[index] {
		case '!', '&':
			for index < len(header) && header[index] != ' ' && header[index] != '\t' && header[index] != '#' {
				index++
			}
		case '|', '>':
			return index
		default:
			return -1
		}
	}
	return -1
}
