package yaml

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuyosy/structivedit/codec"
	"github.com/yuyosy/structivedit/document"
	yamlv3 "go.yaml.in/yaml/v3"
)

func (current *session) Document() *document.Document {
	if current == nil {
		return nil
	}
	return current.document
}

// Encode writes the current snapshot using the source Session's YAML metadata.
// The snapshot must belong to the same Document lineage as the decoded source.
func (current *session) Encode(writer io.Writer, doc *document.Document) error {
	if current == nil || current.document == nil {
		return codec.ErrForeignDocument
	}
	if doc == nil {
		return document.ErrInvalidDocument
	}
	if !current.document.SameLineage(doc) {
		return codec.ErrForeignDocument
	}
	if writer == nil {
		return fmt.Errorf("yaml: nil writer")
	}
	state := &encodeState{
		session:    current,
		document:   doc,
		nodes:      make(map[document.NodeID]*yamlv3.Node),
		references: make(map[document.NodeID]document.NodeID),
	}
	root, err := state.build(doc.Root())
	if err != nil {
		return err
	}
	for referenceID, targetID := range state.references {
		reference := state.nodes[referenceID]
		target := state.nodes[targetID]
		if reference == nil || target == nil || target.Anchor == "" {
			return ErrInvalidYAML
		}
		reference.Alias = target
		reference.Value = target.Anchor
	}
	documentNode := &yamlv3.Node{Kind: yamlv3.DocumentNode, Content: []*yamlv3.Node{root}}
	applyNodeMetadata(documentNode, current.sourceMetadata, false)
	var output bytes.Buffer
	encoder := yamlv3.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(documentNode); err != nil {
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	encoded, err := preserveChompMarkers(current, doc, output.Bytes())
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, bytes.NewReader(encoded))
	return err
}

type encodeState struct {
	session    *session
	document   *document.Document
	nodes      map[document.NodeID]*yamlv3.Node
	references map[document.NodeID]document.NodeID
}

func (state *encodeState) build(id document.NodeID) (*yamlv3.Node, error) {
	if existing := state.nodes[id]; existing != nil {
		return existing, nil
	}
	docNode, ok := state.document.Node(id)
	if !ok {
		return nil, document.ErrNodeNotFound
	}
	metadata := state.session.metadata[id]
	var node *yamlv3.Node
	switch docNode.Kind() {
	case document.NodeScalar:
		node = &yamlv3.Node{Kind: yamlv3.ScalarNode}
		kind, value, scalar := state.document.Scalar(id)
		if !scalar {
			return nil, document.ErrInvalidDocument
		}
		text, err := encodeScalar(kind, value)
		if err != nil {
			return nil, err
		}
		node.Value = text
		applyNodeMetadata(node, metadata, true)
		if kind == document.ScalarString && isBlockScalarStyle(metadata.ScalarStyle) && !blockChompCompatible(text, metadata.Chomp) {
			node.Style = clearScalarStyle(node.Style) | yamlv3.DoubleQuotedStyle
		} else if kind == document.ScalarString && shouldQuoteString(text, metadata) {
			node.Style = clearScalarStyle(node.Style) | yamlv3.DoubleQuotedStyle
		}
	case document.NodeSequence:
		node = &yamlv3.Node{Kind: yamlv3.SequenceNode}
		state.nodes[id] = node
		items, _ := state.document.SequenceItems(id)
		node.Content = make([]*yamlv3.Node, 0, len(items))
		for _, itemID := range items {
			child, err := state.build(itemID)
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, child)
		}
		applyNodeMetadata(node, metadata, false)
		if metadata.CollectionStyle == collectionFlow && !hasCommentsInSubtree(state.document, state.session.metadata, id) {
			node.Style |= yamlv3.FlowStyle
		}
	case document.NodeMapping:
		node = &yamlv3.Node{Kind: yamlv3.MappingNode}
		state.nodes[id] = node
		entries, _ := state.document.MappingEntries(id)
		node.Content = make([]*yamlv3.Node, 0, len(entries)*2)
		for _, entry := range entries {
			key, err := state.build(entry.Key)
			if err != nil {
				return nil, err
			}
			value, err := state.build(entry.Value)
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, key, value)
		}
		applyNodeMetadata(node, metadata, false)
		if metadata.CollectionStyle == collectionFlow && !hasCommentsInSubtree(state.document, state.session.metadata, id) {
			node.Style |= yamlv3.FlowStyle
		}
	case document.NodeReference:
		node = &yamlv3.Node{Kind: yamlv3.AliasNode}
		state.nodes[id] = node
		targetID, ok := state.document.ReferenceTarget(id)
		if !ok {
			return nil, document.ErrInvalidDocument
		}
		state.references[id] = targetID
		applyNodeMetadata(node, metadata, false)
		if node.Value == "" {
			node.Value = metadata.AliasName
		}
	default:
		return nil, ErrInvalidYAML
	}
	state.nodes[id] = node
	return node, nil
}

func applyNodeMetadata(node *yamlv3.Node, metadata nodeMetadata, scalar bool) {
	node.HeadComment = metadata.HeadComment
	node.LineComment = metadata.LineComment
	node.FootComment = metadata.FootComment
	node.Anchor = metadata.Anchor
	if metadata.ExplicitTag {
		node.Tag = metadata.Tag
		node.Style |= yamlv3.TaggedStyle
	}
	if !scalar {
		return
	}
	switch metadata.ScalarStyle {
	case scalarSingleQuoted:
		node.Style |= yamlv3.SingleQuotedStyle
	case scalarDoubleQuoted:
		node.Style |= yamlv3.DoubleQuotedStyle
	case scalarLiteral:
		node.Style |= yamlv3.LiteralStyle
	case scalarFolded:
		node.Style |= yamlv3.FoldedStyle
	}
}

func clearScalarStyle(style yamlv3.Style) yamlv3.Style {
	return style &^ (yamlv3.SingleQuotedStyle | yamlv3.DoubleQuotedStyle | yamlv3.LiteralStyle | yamlv3.FoldedStyle)
}

func isBlockScalarStyle(style scalarStyle) bool {
	return style == scalarLiteral || style == scalarFolded
}

func blockChompCompatible(value string, mode chompMode) bool {
	trailing := len(value) - len(strings.TrimRight(value, "\n"))
	switch mode {
	case chompStrip:
		return trailing == 0
	case chompClip:
		return trailing == 1
	case chompKeep:
		return trailing > 0
	default:
		return false
	}
}

func preserveChompMarkers(current *session, doc *document.Document, encoded []byte) ([]byte, error) {
	var syntax yamlv3.Node
	if err := yamlv3.Unmarshal(encoded, &syntax); err != nil {
		return nil, err
	}
	if syntax.Kind != yamlv3.DocumentNode || len(syntax.Content) != 1 {
		return nil, ErrInvalidYAML
	}
	byNodeID := make(map[document.NodeID]*yamlv3.Node)
	if err := mapEncodedNodes(doc, doc.Root(), syntax.Content[0], byNodeID); err != nil {
		return nil, err
	}
	lines := strings.Split(string(encoded), "\n")
	for id, metadata := range current.metadata {
		if !isBlockScalarStyle(metadata.ScalarStyle) {
			continue
		}
		node := byNodeID[id]
		if node == nil {
			continue
		}
		kind, value, scalar := doc.Scalar(id)
		if !scalar || kind != document.ScalarString || node.Kind != yamlv3.ScalarNode || !blockChompCompatible(value.(string), metadata.Chomp) {
			continue
		}
		if node.Line < 1 || node.Line > len(lines) {
			return nil, ErrInvalidYAML
		}
		lines[node.Line-1] = setChompIndicator(lines[node.Line-1], node.Column, metadata.Chomp)
	}
	return []byte(strings.Join(lines, "\n")), nil
}

func mapEncodedNodes(doc *document.Document, id document.NodeID, syntax *yamlv3.Node, result map[document.NodeID]*yamlv3.Node) error {
	if syntax == nil {
		return ErrInvalidYAML
	}
	node, ok := doc.Node(id)
	if !ok {
		return document.ErrNodeNotFound
	}
	result[id] = syntax
	switch node.Kind() {
	case document.NodeScalar:
		if syntax.Kind != yamlv3.ScalarNode {
			return ErrInvalidYAML
		}
	case document.NodeReference:
		if syntax.Kind != yamlv3.AliasNode {
			return ErrInvalidYAML
		}
	case document.NodeSequence:
		items, _ := doc.SequenceItems(id)
		if syntax.Kind != yamlv3.SequenceNode || len(items) != len(syntax.Content) {
			return ErrInvalidYAML
		}
		for index, childID := range items {
			if err := mapEncodedNodes(doc, childID, syntax.Content[index], result); err != nil {
				return err
			}
		}
	case document.NodeMapping:
		entries, _ := doc.MappingEntries(id)
		if syntax.Kind != yamlv3.MappingNode || len(entries)*2 != len(syntax.Content) {
			return ErrInvalidYAML
		}
		for index, entry := range entries {
			if err := mapEncodedNodes(doc, entry.Key, syntax.Content[index*2], result); err != nil {
				return err
			}
			if err := mapEncodedNodes(doc, entry.Value, syntax.Content[index*2+1], result); err != nil {
				return err
			}
		}
	default:
		return ErrInvalidYAML
	}
	return nil
}

func setChompIndicator(line string, column int, mode chompMode) string {
	start := column - 1
	if start < 0 || start >= len(line) {
		start = 0
	}
	relativeMarker := blockScalarMarker(line[start:])
	marker := -1
	if relativeMarker >= 0 {
		marker = start + relativeMarker
	}
	if marker < 0 {
		return line
	}
	end := marker + 1
	for end < len(line) && line[end] != ' ' && line[end] != '\t' && line[end] != '#' {
		end++
	}
	token := strings.ReplaceAll(strings.ReplaceAll(line[marker:end], "+", ""), "-", "")
	indicator := ""
	switch mode {
	case chompStrip:
		indicator = "-"
	case chompKeep:
		indicator = "+"
	}
	return line[:marker] + token[:1] + indicator + token[1:] + line[end:]
}

func encodeScalar(kind document.ScalarKind, value any) (string, error) {
	switch kind {
	case document.ScalarString:
		text, ok := value.(string)
		if !ok {
			return "", document.ErrInvalidDocument
		}
		if !utf8.ValidString(text) {
			return "", ErrInvalidYAML
		}
		return text, nil
	case document.ScalarBool:
		boolean, ok := value.(bool)
		if !ok {
			return "", document.ErrInvalidDocument
		}
		return strconv.FormatBool(boolean), nil
	case document.ScalarInteger:
		integer, ok := value.(int64)
		if !ok {
			return "", document.ErrInvalidDocument
		}
		return strconv.FormatInt(integer, 10), nil
	case document.ScalarFloat:
		floating, ok := value.(float64)
		if !ok {
			return "", document.ErrInvalidDocument
		}
		switch {
		case math.IsNaN(floating):
			if math.Signbit(floating) {
				return "-.nan", nil
			}
			return ".nan", nil
		case math.IsInf(floating, 1):
			return ".inf", nil
		case math.IsInf(floating, -1):
			return "-.inf", nil
		}
		text := strconv.FormatFloat(floating, 'g', -1, 64)
		if !strings.ContainsAny(text, ".eE") {
			text += ".0"
		}
		return text, nil
	case document.ScalarNull:
		if value != nil {
			return "", document.ErrInvalidDocument
		}
		return "null", nil
	default:
		return "", document.ErrInvalidDocument
	}
}

func shouldQuoteString(value string, metadata nodeMetadata) bool {
	if metadata.HasSource && metadata.ScalarStyle != scalarPlain {
		return false
	}
	if !utf8.ValidString(value) || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return true
	}
	if value == "" || value == "-" || value == "---" || value == "..." || strings.TrimSpace(value) != value {
		return true
	}
	resolved, resolveErr := implicitScalarKind(value)
	if resolveErr == nil && resolved != document.ScalarString {
		return true
	}
	if looksLikeLegacyOctal(value) || isLegacyBoolean(value) {
		return true
	}
	if strings.HasPrefix(value, "-") && (value == "-" || len(value) > 1 && isYAMLIndicator(value[1])) {
		return true
	}
	if hasAmbiguousColon(value) || strings.Contains(value, " #") || strings.HasPrefix(value, "#") {
		return true
	}
	if strings.ContainsAny(value[:1], "[]{} ,&*!|>'\"%@`") || startsWithAmbiguousIndicator(value) {
		return true
	}
	return false
}

func hasAmbiguousColon(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] == ':' && (index+1 == len(value) || isSeparationOrFlowIndicator(value[index+1])) {
			return true
		}
	}
	return false
}

func startsWithAmbiguousIndicator(value string) bool {
	if value == "?" || value == ":" {
		return true
	}
	if value[0] != '?' && value[0] != ':' {
		return false
	}
	return len(value) == 1 || isSeparationOrFlowIndicator(value[1])
}

func isSeparationOrFlowIndicator(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n' || strings.ContainsRune(",[]{}?", rune(value))
}

func implicitScalarKind(value string) (document.ScalarKind, error) {
	if isCoreNull(value) {
		return document.ScalarNull, nil
	}
	if _, ok := parseCoreBool(value); ok {
		return document.ScalarBool, nil
	}
	if _, ok, err := parseCoreInteger(value); ok || err != nil {
		return document.ScalarInteger, err
	}
	if _, ok, err := parseCoreFloat(value); ok || err != nil {
		return document.ScalarFloat, err
	}
	return document.ScalarString, nil
}

func isYAMLIndicator(value byte) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\r'
}

func looksLikeLegacyOctal(value string) bool {
	value = strings.TrimPrefix(strings.TrimPrefix(value, "+"), "-")
	if len(value) < 2 || value[0] != '0' {
		return false
	}
	for _, char := range value[1:] {
		if (char < '0' || char > '9') && char != '_' {
			return false
		}
	}
	return true
}

func isLegacyBoolean(value string) bool {
	switch strings.ToLower(value) {
	case "yes", "no", "on", "off":
		return true
	default:
		return false
	}
}

func hasCommentsInSubtree(doc *document.Document, metadata map[document.NodeID]nodeMetadata, root document.NodeID) bool {
	stack := []document.NodeID{root}
	visited := make(map[document.NodeID]struct{})
	for len(stack) > 0 {
		last := len(stack) - 1
		id := stack[last]
		stack = stack[:last]
		if _, seen := visited[id]; seen {
			continue
		}
		visited[id] = struct{}{}
		comment := metadata[id]
		if comment.HeadComment != "" || comment.LineComment != "" || comment.FootComment != "" {
			return true
		}
		stack = append(stack, doc.Children(id)...)
	}
	return false
}

var _ codec.Session = (*session)(nil)
var _ codec.Codec = Codec{}
