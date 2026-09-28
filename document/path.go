package document

import "fmt"

// Path is an immutable ownership location within a Document. It identifies a
// current position, not a stable NodeID.
type Path struct {
	segments []PathSegment
}

// PathSegment is one ownership step in a Path. Its implementations are
// immutable value types provided by this package.
type PathSegment interface {
	pathSegment()
}

type SequenceIndexSegment struct {
	index int
}

type MappingEntrySegment struct {
	entryIndex int
	role       MappingEntryRole
}

type MappingEntryRole uint8

const (
	MappingKeyRole MappingEntryRole = iota
	MappingValueRole
)

func (SequenceIndexSegment) pathSegment() {}
func (MappingEntrySegment) pathSegment()  {}

func (s SequenceIndexSegment) Index() int { return s.index }

func (s MappingEntrySegment) EntryIndex() int { return s.entryIndex }

func (s MappingEntrySegment) Role() MappingEntryRole { return s.role }

// Parent returns the containing path. The root has no parent.
func (p Path) Parent() (Path, bool) {
	if len(p.segments) == 0 {
		return Path{}, false
	}
	segments := append([]PathSegment(nil), p.segments[:len(p.segments)-1]...)
	return Path{segments: segments}, true
}

// AppendSequenceIndex returns a copy of p with one sequence step appended.
func (p Path) AppendSequenceIndex(index int) (Path, error) {
	if index < 0 {
		return Path{}, ErrInvalidPath
	}
	segments := append([]PathSegment(nil), p.segments...)
	segments = append(segments, SequenceIndexSegment{index: index})
	return Path{segments: segments}, nil
}

// AppendMappingEntry returns a copy of p with one mapping step appended.
func (p Path) AppendMappingEntry(entryIndex int, role MappingEntryRole) (Path, error) {
	if entryIndex < 0 || (role != MappingKeyRole && role != MappingValueRole) {
		return Path{}, ErrInvalidPath
	}
	segments := append([]PathSegment(nil), p.segments...)
	segments = append(segments, MappingEntrySegment{entryIndex: entryIndex, role: role})
	return Path{segments: segments}, nil
}

// Segments returns a copy of this path's segments.
func (p Path) Segments() []PathSegment {
	return append([]PathSegment(nil), p.segments...)
}

// String formats a diagnostic path. This representation is not a parser input.
func (p Path) String() string {
	result := "$"
	for _, segment := range p.segments {
		switch typed := segment.(type) {
		case SequenceIndexSegment:
			result += fmt.Sprintf("/s[%d]", typed.index)
		case MappingEntrySegment:
			role := ""
			switch typed.role {
			case MappingKeyRole:
				role = "key"
			case MappingValueRole:
				role = "value"
			default:
				return "<invalid-path>"
			}
			result += fmt.Sprintf("/m[%d].%s", typed.entryIndex, role)
		default:
			return "<invalid-path>"
		}
	}
	return result
}

// Equal reports whether p and other contain the same ownership steps.
func (p Path) Equal(other Path) bool {
	if len(p.segments) != len(other.segments) {
		return false
	}
	for i, segment := range p.segments {
		switch left := segment.(type) {
		case SequenceIndexSegment:
			right, ok := other.segments[i].(SequenceIndexSegment)
			if !ok || left.index != right.index {
				return false
			}
		case MappingEntrySegment:
			right, ok := other.segments[i].(MappingEntrySegment)
			if !ok || left.entryIndex != right.entryIndex || left.role != right.role {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// Path returns id's current ownership location in d.
func (d *Document) Path(id NodeID) (Path, error) {
	if d == nil || id == 0 || d.nodes[id] == nil {
		return Path{}, ErrNodeNotFound
	}
	if d.root == 0 || d.nodes[d.root] == nil {
		return Path{}, ErrInvalidDocument
	}
	reversed := make([]PathSegment, 0)
	seen := make(map[NodeID]struct{})
	for current := id; current != d.root; {
		if _, exists := seen[current]; exists {
			return Path{}, ErrInvalidDocument
		}
		seen[current] = struct{}{}
		parent, ok := d.parents[current]
		if !ok {
			return Path{}, ErrInvalidDocument
		}
		switch parent.Role {
		case ParentSequenceItem:
			reversed = append(reversed, SequenceIndexSegment{index: parent.Index})
		case ParentMappingKey:
			reversed = append(reversed, MappingEntrySegment{entryIndex: parent.Index, role: MappingKeyRole})
		case ParentMappingValue:
			reversed = append(reversed, MappingEntrySegment{entryIndex: parent.Index, role: MappingValueRole})
		default:
			return Path{}, ErrInvalidDocument
		}
		current = parent.Parent
	}
	segments := make([]PathSegment, len(reversed))
	for i := range reversed {
		segments[len(reversed)-1-i] = reversed[i]
	}
	return Path{segments: segments}, nil
}

// LookupPath resolves a path by following ownership edges only.
func (d *Document) LookupPath(path Path) (NodeID, error) {
	if d == nil || d.root == 0 || d.nodes[d.root] == nil {
		return 0, ErrPathNotFound
	}
	current := d.root
	for _, segment := range path.segments {
		entry := d.nodes[current]
		if entry == nil {
			return 0, ErrPathNotFound
		}
		switch typed := segment.(type) {
		case SequenceIndexSegment:
			if typed.index < 0 {
				return 0, ErrInvalidPath
			}
			if entry.kind != NodeSequence || typed.index >= len(entry.sequence.items) {
				return 0, ErrPathNotFound
			}
			current = entry.sequence.items[typed.index]
		case MappingEntrySegment:
			if typed.entryIndex < 0 || (typed.role != MappingKeyRole && typed.role != MappingValueRole) {
				return 0, ErrInvalidPath
			}
			if entry.kind != NodeMapping || typed.entryIndex >= len(entry.mapping.entries) {
				return 0, ErrPathNotFound
			}
			mappingEntry := entry.mapping.entries[typed.entryIndex]
			if typed.role == MappingKeyRole {
				current = mappingEntry.key
			} else {
				current = mappingEntry.value
			}
		default:
			return 0, ErrInvalidPath
		}
	}
	return current, nil
}
