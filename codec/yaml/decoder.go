package yaml

import (
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuyosy/structivedit/codec"
	"github.com/yuyosy/structivedit/document"
	yamlv3 "go.yaml.in/yaml/v3"
)

var (
	// ErrIntegerOutOfRange reports a YAML integer that cannot be represented by
	// the Document's int64 scalar kind.
	ErrIntegerOutOfRange = errors.New("yaml: integer out of range")
	// ErrMultipleDocuments reports input containing more than one YAML document.
	ErrMultipleDocuments = errors.New("yaml: multiple documents are not supported")
	ErrInvalidYAML       = errors.New("yaml: unsupported or invalid node")
	// ErrLimitExceeded reports that a configured DecodeOptions limit was hit.
	ErrLimitExceeded = errors.New("yaml: configured decode limit exceeded")
	// ErrInvalidDecodeOptions reports negative or otherwise invalid limits.
	ErrInvalidDecodeOptions = errors.New("yaml: invalid decode options")

	yamlIntegerPattern = regexp.MustCompile(`^(?:[+-]?(?:0|[1-9](?:_?[0-9])*)|[+-]?0[bB][01](?:_?[01])*|[+-]?0[oO][0-7](?:_?[0-7])*|[+-]?0[xX][0-9a-fA-F](?:_?[0-9a-fA-F])*)$`)
	yamlDecimalInteger = regexp.MustCompile(`^[+-]?[0-9](?:_?[0-9])*$`)
	yamlFloatPattern   = regexp.MustCompile(`^[+-]?(?:\.(?:inf|Inf|INF|nan|NaN|NAN)|(?:[0-9](?:_?[0-9])*)?\.[0-9](?:_?[0-9])*(?:[eE][+-]?[0-9](?:_?[0-9])*)?|[0-9](?:_?[0-9])*\.|[0-9](?:_?[0-9])*[eE][+-]?[0-9](?:_?[0-9])*)$`)
)

// Codec implements codec.Codec for one YAML 1.2 Core Schema document.
type Codec struct{}

// DecodeOptions bounds resources consumed while reading a YAML document.
// A zero limit is unlimited. MaxDepth counts the root node as depth one.
type DecodeOptions struct {
	MaxInputBytes int64
	MaxNodes      int
	MaxDepth      int
}

// Decode reads a single YAML document and binds it to a format Session.
func Decode(reader io.Reader) (codec.Session, error) {
	return DecodeWithOptions(reader, DecodeOptions{})
}

// DecodeWithOptions reads one YAML document while enforcing any positive
// input byte, node count, and nesting depth limits in options.
func DecodeWithOptions(reader io.Reader, options DecodeOptions) (codec.Session, error) {
	if reader == nil {
		return nil, fmt.Errorf("yaml: nil reader")
	}
	if options.MaxInputBytes < 0 || options.MaxNodes < 0 || options.MaxDepth < 0 {
		return nil, ErrInvalidDecodeOptions
	}
	limitedReader := io.Reader(reader)
	maxInt64 := int64(^uint64(0) >> 1)
	if options.MaxInputBytes > 0 && options.MaxInputBytes < maxInt64 {
		limitedReader = io.LimitReader(reader, options.MaxInputBytes+1)
	}
	sourceBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, err
	}
	if options.MaxInputBytes > 0 && int64(len(sourceBytes)) > options.MaxInputBytes {
		return nil, fmt.Errorf("%w: maximum input size is %d bytes", ErrLimitExceeded, options.MaxInputBytes)
	}
	sourceText := string(sourceBytes)
	sourceLines := strings.Split(sourceText, "\n")
	decoder := yamlv3.NewDecoder(strings.NewReader(sourceText))
	var source yamlv3.Node
	if err := decoder.Decode(&source); err != nil {
		if !errors.Is(err, io.EOF) {
			return nil, err
		}
		source = emptyYAMLDocument()
	} else {
		var trailing yamlv3.Node
		if err := decoder.Decode(&trailing); err == nil {
			return nil, ErrMultipleDocuments
		} else if !errors.Is(err, io.EOF) {
			return nil, err
		}
	}
	if source.Kind != yamlv3.DocumentNode || len(source.Content) > 1 {
		return nil, ErrInvalidYAML
	}
	if len(source.Content) == 0 {
		source.Content = []*yamlv3.Node{{Kind: yamlv3.ScalarNode, Tag: "!!null", Value: "null"}}
	}
	root := source.Content[0]
	if root == nil {
		return nil, ErrInvalidYAML
	}

	builder := document.NewBuilder()
	state := &decodeState{
		builder:      builder,
		options:      options,
		sourceLines:  sourceLines,
		nodeIDs:      make(map[*yamlv3.Node]document.NodeID),
		metadata:     make(map[document.NodeID]nodeMetadata),
		restrictions: make(map[document.NodeID]document.NodeRestrictions),
		reserved:     make(map[*yamlv3.Node]bool),
		defined:      make(map[*yamlv3.Node]bool),
	}
	if err := state.reserveTree(root, 1); err != nil {
		return nil, err
	}
	if err := state.defineTree(root); err != nil {
		return nil, err
	}
	rootID := state.nodeIDs[root]
	if err := builder.SetRoot(rootID); err != nil {
		return nil, err
	}
	for id, restrictions := range state.restrictions {
		if err := builder.SetRestrictions(id, restrictions); err != nil {
			return nil, err
		}
	}
	doc, err := builder.Build()
	if err != nil {
		return nil, err
	}
	return &session{
		document:       doc,
		metadata:       state.metadata,
		sourceMetadata: metadataFromNode(&source, sourceLines),
	}, nil
}

// Decode implements codec.Codec.
func (Codec) Decode(reader io.Reader) (codec.Session, error) { return Decode(reader) }

// DecodeWithOptions reads a YAML document with caller-specified resource
// limits. It complements the format-independent Codec interface.
func (Codec) DecodeWithOptions(reader io.Reader, options DecodeOptions) (codec.Session, error) {
	return DecodeWithOptions(reader, options)
}

type decodeState struct {
	builder      *document.Builder
	options      DecodeOptions
	nodeCount    int
	sourceLines  []string
	nodeIDs      map[*yamlv3.Node]document.NodeID
	metadata     map[document.NodeID]nodeMetadata
	restrictions map[document.NodeID]document.NodeRestrictions
	reserved     map[*yamlv3.Node]bool
	defined      map[*yamlv3.Node]bool
}

func emptyYAMLDocument() yamlv3.Node {
	return yamlv3.Node{
		Kind: yamlv3.DocumentNode,
		Content: []*yamlv3.Node{{
			Kind:  yamlv3.ScalarNode,
			Tag:   "!!null",
			Value: "null",
		}},
	}
}

func (state *decodeState) reserveTree(node *yamlv3.Node, depth int) error {
	if node == nil {
		return ErrInvalidYAML
	}
	if state.reserved[node] {
		return nil
	}
	if state.options.MaxDepth > 0 && depth > state.options.MaxDepth {
		return fmt.Errorf("%w: maximum nesting depth is %d", ErrLimitExceeded, state.options.MaxDepth)
	}
	if state.options.MaxNodes > 0 && state.nodeCount >= state.options.MaxNodes {
		return fmt.Errorf("%w: maximum node count is %d", ErrLimitExceeded, state.options.MaxNodes)
	}
	state.nodeCount++
	id, err := state.builder.Reserve()
	if err != nil {
		return err
	}
	state.reserved[node] = true
	state.nodeIDs[node] = id
	state.metadata[id] = metadataFromNode(node, state.sourceLines)
	if node.Kind == yamlv3.SequenceNode || node.Kind == yamlv3.MappingNode {
		if node.Kind == yamlv3.MappingNode && len(node.Content)%2 != 0 {
			return ErrInvalidYAML
		}
		for _, child := range node.Content {
			if err := state.reserveTree(child, depth+1); err != nil {
				return err
			}
		}
	} else if node.Kind != yamlv3.ScalarNode && node.Kind != yamlv3.AliasNode {
		return ErrInvalidYAML
	}
	return nil
}

func (state *decodeState) defineTree(node *yamlv3.Node) error {
	if node == nil {
		return ErrInvalidYAML
	}
	if state.defined[node] {
		return nil
	}
	id := state.nodeIDs[node]
	if id == 0 {
		return ErrInvalidYAML
	}
	switch node.Kind {
	case yamlv3.ScalarNode:
		value, customTag, err := decodeScalar(node)
		if err != nil {
			return err
		}
		if err := state.builder.DefineScalar(id, value); err != nil {
			return err
		}
		if customTag {
			state.markReadOnly(id)
		}
	case yamlv3.SequenceNode:
		customTag, err := validateCollectionTag(node, yamlv3.SequenceNode)
		if err != nil {
			return err
		}
		items := make([]document.NodeID, len(node.Content))
		for index, child := range node.Content {
			if err := state.defineTree(child); err != nil {
				return err
			}
			items[index] = state.nodeIDs[child]
		}
		if err := state.builder.DefineSequence(id, items); err != nil {
			return err
		}
		if customTag {
			state.markReadOnly(id)
		}
	case yamlv3.MappingNode:
		customTag, err := validateCollectionTag(node, yamlv3.MappingNode)
		if err != nil {
			return err
		}
		entries := make([]document.MappingEntry, len(node.Content)/2)
		for index := range entries {
			keyNode := node.Content[index*2]
			valueNode := node.Content[index*2+1]
			if err := state.defineTree(keyNode); err != nil {
				return err
			}
			if err := state.defineTree(valueNode); err != nil {
				return err
			}
			entries[index] = document.MappingEntry{Key: state.nodeIDs[keyNode], Value: state.nodeIDs[valueNode]}
			state.markReadOnly(state.nodeIDs[keyNode])
			if isMergeKey(keyNode) {
				state.markReadOnly(state.nodeIDs[valueNode])
			}
		}
		if err := state.builder.DefineMapping(id, entries); err != nil {
			return err
		}
		if customTag {
			state.markReadOnly(id)
		}
	case yamlv3.AliasNode:
		targetID := state.nodeIDs[node.Alias]
		if node.Alias == nil || targetID == 0 {
			return ErrInvalidYAML
		}
		if err := state.builder.DefineReference(id, targetID); err != nil {
			return err
		}
		state.markReadOnly(id)
	default:
		return ErrInvalidYAML
	}
	state.defined[node] = true
	return nil
}

func (state *decodeState) markReadOnly(id document.NodeID) {
	state.restrictions[id] |= document.RestrictionReadOnly
}

func isMergeKey(node *yamlv3.Node) bool {
	if node == nil || node.Kind != yamlv3.ScalarNode || node.Value != "<<" {
		return false
	}
	if node.Style&(yamlv3.SingleQuotedStyle|yamlv3.DoubleQuotedStyle) != 0 {
		return false
	}
	if node.Style&yamlv3.TaggedStyle != 0 {
		return canonicalTag(node.Tag) == "!!merge"
	}
	return true
}

func validateCollectionTag(node *yamlv3.Node, expected yamlv3.Kind) (bool, error) {
	if node.Style&yamlv3.TaggedStyle == 0 {
		return false, nil
	}
	tag := canonicalTag(node.Tag)
	if expected == yamlv3.SequenceNode && tag == "!!seq" || expected == yamlv3.MappingNode && tag == "!!map" {
		return false, nil
	}
	switch tag {
	case "!!str", "!!bool", "!!int", "!!float", "!!null", "!!seq", "!!map":
		return false, ErrInvalidYAML
	default:
		return true, nil
	}
}

func decodeScalar(node *yamlv3.Node) (any, bool, error) {
	explicit := node.Style&yamlv3.TaggedStyle != 0
	if explicit {
		switch canonicalTag(node.Tag) {
		case "!!str":
			return node.Value, false, nil
		case "!!bool":
			value, ok := parseCoreBool(node.Value)
			if !ok {
				return nil, false, document.ErrInvalidScalar
			}
			return value, false, nil
		case "!!int":
			value, ok, err := parseCoreInteger(node.Value)
			if err != nil {
				return nil, false, err
			}
			if !ok {
				return nil, false, document.ErrInvalidScalar
			}
			return value, false, nil
		case "!!float":
			value, ok, err := parseExplicitFloat(node.Value)
			if err != nil {
				return nil, false, err
			}
			if !ok {
				return nil, false, document.ErrInvalidScalar
			}
			return value, false, nil
		case "!!null":
			if !isCoreNull(node.Value) {
				return nil, false, document.ErrInvalidScalar
			}
			return nil, false, nil
		case "!!seq", "!!map":
			return nil, false, ErrInvalidYAML
		default:
			if node.Style&(yamlv3.SingleQuotedStyle|yamlv3.DoubleQuotedStyle|yamlv3.LiteralStyle|yamlv3.FoldedStyle) != 0 {
				return node.Value, true, nil
			}
			value, err := resolveImplicitScalar(node)
			return value, true, err
		}
	}
	if node.Style&(yamlv3.SingleQuotedStyle|yamlv3.DoubleQuotedStyle|yamlv3.LiteralStyle|yamlv3.FoldedStyle) != 0 {
		return node.Value, false, nil
	}
	value, err := resolveImplicitScalar(node)
	return value, false, err
}

func resolveImplicitScalar(node *yamlv3.Node) (any, error) {
	if isCoreNull(node.Value) {
		return nil, nil
	}
	if value, ok := parseCoreBool(node.Value); ok {
		return value, nil
	}
	if value, ok, err := parseCoreInteger(node.Value); ok || err != nil {
		return value, err
	}
	if value, ok, err := parseCoreFloat(node.Value); ok || err != nil {
		return value, err
	}
	return node.Value, nil
}

func isCoreNull(value string) bool {
	switch value {
	case "", "~", "null", "Null", "NULL":
		return true
	default:
		return false
	}
}

func parseCoreBool(value string) (bool, bool) {
	switch value {
	case "true", "True", "TRUE":
		return true, true
	case "false", "False", "FALSE":
		return false, true
	default:
		return false, false
	}
}

func parseCoreInteger(value string) (int64, bool, error) {
	if !yamlIntegerPattern.MatchString(value) {
		return 0, false, nil
	}
	clean := strings.ReplaceAll(value, "_", "")
	sign := ""
	if strings.HasPrefix(clean, "+") || strings.HasPrefix(clean, "-") {
		sign, clean = clean[:1], clean[1:]
	}
	base := 10
	if len(clean) > 1 && clean[0] == '0' {
		switch clean[1] {
		case 'b', 'B':
			base, clean = 2, clean[2:]
		case 'o', 'O':
			base, clean = 8, clean[2:]
		case 'x', 'X':
			base, clean = 16, clean[2:]
		}
	}
	parsed, err := strconv.ParseInt(sign+clean, base, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return 0, true, fmt.Errorf("%w: %s", ErrIntegerOutOfRange, value)
		}
		return 0, true, document.ErrInvalidScalar
	}
	return parsed, true, nil
}

func parseCoreFloat(value string) (float64, bool, error) {
	if !yamlFloatPattern.MatchString(value) {
		return 0, false, nil
	}
	clean := strings.ReplaceAll(value, "_", "")
	sign := 1.0
	unsigned := clean
	if strings.HasPrefix(unsigned, "+") || strings.HasPrefix(unsigned, "-") {
		if unsigned[0] == '-' {
			sign = -1
		}
		unsigned = unsigned[1:]
	}
	switch unsigned {
	case ".inf", ".Inf", ".INF":
		return sign * math.Inf(1), true, nil
	case ".nan", ".NaN", ".NAN":
		return math.Copysign(math.NaN(), sign), true, nil
	}
	parsed, err := strconv.ParseFloat(clean, 64)
	if err != nil || math.IsInf(parsed, 0) || math.IsNaN(parsed) {
		return 0, true, document.ErrInvalidScalar
	}
	return parsed, true, nil
}

func parseExplicitFloat(value string) (float64, bool, error) {
	parsed, ok, err := parseCoreFloat(value)
	if ok || err != nil || !yamlDecimalInteger.MatchString(value) {
		return parsed, ok, err
	}
	parsed, err = strconv.ParseFloat(strings.ReplaceAll(value, "_", ""), 64)
	if err != nil || math.IsInf(parsed, 0) || math.IsNaN(parsed) {
		return 0, true, document.ErrInvalidScalar
	}
	return parsed, true, nil
}

func canonicalTag(tag string) string {
	switch tag {
	case "tag:yaml.org,2002:str":
		return "!!str"
	case "tag:yaml.org,2002:bool":
		return "!!bool"
	case "tag:yaml.org,2002:int":
		return "!!int"
	case "tag:yaml.org,2002:float":
		return "!!float"
	case "tag:yaml.org,2002:null":
		return "!!null"
	case "tag:yaml.org,2002:seq":
		return "!!seq"
	case "tag:yaml.org,2002:map":
		return "!!map"
	case "tag:yaml.org,2002:merge":
		return "!!merge"
	default:
		return tag
	}
}
