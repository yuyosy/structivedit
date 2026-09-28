package selector

import "github.com/yuyosy/structivedit/document"

type concreteKind uint8

const (
	concreteIndex concreteKind = iota
	concreteStringKey
	concreteOtherKey
)

type concreteSegment struct {
	kind  concreteKind
	index int
	key   string
}

// Match returns matching NodeIDs in ownership preorder. Mapping keys are not
// candidates, and Reference edges are never followed.
func Match(doc *document.Document, pattern Pattern) []document.NodeID {
	if doc == nil || doc.Root() == 0 {
		return nil
	}
	matches := make([]document.NodeID, 0)
	path := make([]concreteSegment, 0)
	var walk func(document.NodeID)
	walk = func(id document.NodeID) {
		if matchesConcretePath(pattern, path) {
			matches = append(matches, id)
		}
		node, ok := doc.Node(id)
		if !ok {
			return
		}
		switch node.Kind() {
		case document.NodeSequence:
			items, _ := doc.SequenceItems(id)
			for index, child := range items {
				path = append(path, concreteSegment{kind: concreteIndex, index: index})
				walk(child)
				path = path[:len(path)-1]
			}
		case document.NodeMapping:
			entries, _ := doc.MappingEntries(id)
			for _, entry := range entries {
				path = append(path, mappingKeyStep(doc, entry.Key))
				walk(entry.Value)
				path = path[:len(path)-1]
			}
		}
	}
	walk(doc.Root())
	return matches
}

// MatchesNode reports whether id is selected by pattern.
func MatchesNode(doc *document.Document, pattern Pattern, id document.NodeID) bool {
	if doc == nil || id == 0 {
		return false
	}
	if id == doc.Root() {
		_, ok := doc.Node(id)
		return ok && matchesConcretePath(pattern, nil)
	}
	path, err := doc.Path(id)
	if err != nil {
		return false
	}
	segments := path.Segments()
	current := doc.Root()
	concrete := make([]concreteSegment, 0, len(segments))
	for _, segment := range segments {
		switch typed := segment.(type) {
		case document.SequenceIndexSegment:
			items, ok := doc.SequenceItems(current)
			if !ok || typed.Index() < 0 || typed.Index() >= len(items) {
				return false
			}
			concrete = append(concrete, concreteSegment{kind: concreteIndex, index: typed.Index()})
			current = items[typed.Index()]
		case document.MappingEntrySegment:
			if typed.Role() != document.MappingValueRole {
				return false
			}
			entries, ok := doc.MappingEntries(current)
			if !ok || typed.EntryIndex() < 0 || typed.EntryIndex() >= len(entries) {
				return false
			}
			entry := entries[typed.EntryIndex()]
			concrete = append(concrete, mappingKeyStep(doc, entry.Key))
			current = entry.Value
		default:
			return false
		}
	}
	return current == id && matchesConcretePath(pattern, concrete)
}

func mappingKeyStep(doc *document.Document, keyID document.NodeID) concreteSegment {
	keyNode, ok := doc.Node(keyID)
	if !ok {
		return concreteSegment{kind: concreteOtherKey}
	}
	kind, value, scalar := keyNode.Scalar()
	if !scalar || kind != document.ScalarString {
		return concreteSegment{kind: concreteOtherKey}
	}
	key, ok := value.(string)
	if !ok {
		return concreteSegment{kind: concreteOtherKey}
	}
	return concreteSegment{kind: concreteStringKey, key: key}
}

func matchesConcretePath(pattern Pattern, path []concreteSegment) bool {
	// Memoization bounds recursive wildcard matching to O(pattern * path).
	type state struct{ pattern, path int }
	capacity := (len(pattern.segments) + 1) * (len(path) + 1)
	memo := make(map[state]bool, capacity)
	known := make(map[state]bool, capacity)
	var match func(int, int) bool
	match = func(patternIndex, pathIndex int) bool {
		key := state{pattern: patternIndex, path: pathIndex}
		if known[key] {
			return memo[key]
		}
		known[key] = true
		var result bool
		if patternIndex == len(pattern.segments) {
			result = pathIndex == len(path)
		} else if _, recursive := pattern.segments[patternIndex].(recursiveSegment); recursive {
			result = match(patternIndex+1, pathIndex)
			if !result && pathIndex < len(path) {
				result = match(patternIndex, pathIndex+1)
			}
		} else if pathIndex < len(path) && segmentMatches(pattern.segments[patternIndex], path[pathIndex]) {
			result = match(patternIndex+1, pathIndex+1)
		}
		memo[key] = result
		return result
	}
	return match(0, 0)
}

func segmentMatches(pattern segment, path concreteSegment) bool {
	switch typed := pattern.(type) {
	case keySegment:
		return path.kind == concreteStringKey && path.key == typed.key
	case indexSegment:
		return path.kind == concreteIndex && path.index == typed.index
	case anyKeySegment:
		return path.kind == concreteStringKey || path.kind == concreteOtherKey
	case anyIndexSegment:
		return path.kind == concreteIndex
	default:
		return false
	}
}
