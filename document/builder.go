package document

type Builder struct {
	draft        *documentDraft
	baseRevision Revision
	changed      bool
}

type documentDraft struct {
	root     NodeID
	nodes    map[NodeID]*docNode
	parents  map[NodeID]ParentRef
	refs     map[NodeID]map[NodeID]struct{}
	nextID   NodeID
	lineage  lineageID
	reserved map[NodeID]struct{}
}

// NewBuilder creates an empty document draft with its own lineage.
func NewBuilder() *Builder {
	return &Builder{
		draft: &documentDraft{
			nodes:    make(map[NodeID]*docNode),
			parents:  make(map[NodeID]ParentRef),
			refs:     make(map[NodeID]map[NodeID]struct{}),
			nextID:   1,
			lineage:  newLineageID(),
			reserved: make(map[NodeID]struct{}),
		},
	}
}

// NewBuilderFrom copies a built immutable document into an independent draft
// while preserving its NodeIDs, indexes, next ID, revision, and lineage.
func NewBuilderFrom(doc *Document) (*Builder, error) {
	if doc == nil || doc.lineage == 0 {
		return nil, ErrInvalidDocument
	}
	return &Builder{
		draft: &documentDraft{
			root:     doc.root,
			nodes:    cloneNodeMap(doc.nodes),
			parents:  cloneParents(doc.parents),
			refs:     cloneReferences(doc.refs),
			nextID:   doc.nextID,
			lineage:  doc.lineage,
			reserved: make(map[NodeID]struct{}),
		},
		baseRevision: doc.revision,
	}, nil
}

// NewScalar normalizes value, allocates a NodeID, and creates a scalar node.
func (b *Builder) NewScalar(value any) (NodeID, error) {
	if !b.ready() {
		return 0, ErrInvalidDocument
	}
	scalar, err := normalizeScalar(value)
	if err != nil {
		return 0, err
	}
	id, next, err := b.availableID()
	if err != nil {
		return 0, err
	}
	b.draft.nodes[id] = &docNode{id: id, kind: NodeScalar, scalar: scalar}
	if err := b.commitPrepared(b.draft.root, b.draft.nodes, b.draft.reserved, next, b.draft.parents, b.draft.refs, true); err != nil {
		return 0, err
	}
	return id, nil
}

// NewSequence creates an ordered sequence that owns each supplied child.
func (b *Builder) NewSequence(items []NodeID) (NodeID, error) {
	if !b.ready() {
		return 0, ErrInvalidDocument
	}
	id, next, err := b.availableID()
	if err != nil {
		return 0, err
	}
	if err := b.validateNewChildren(id, items); err != nil {
		return 0, err
	}
	b.draft.nodes[id] = &docNode{id: id, kind: NodeSequence, sequence: &sequenceNode{items: append([]NodeID(nil), items...)}}
	for index, child := range items {
		b.draft.parents[child] = ParentRef{Parent: id, Role: ParentSequenceItem, Index: index}
	}
	if err := b.commitPrepared(b.draft.root, b.draft.nodes, b.draft.reserved, next, b.draft.parents, b.draft.refs, true); err != nil {
		return 0, err
	}
	return id, nil
}

// NewMapping creates an ordered mapping that owns each entry's key and value.
func (b *Builder) NewMapping(entries []MappingEntry) (NodeID, error) {
	if !b.ready() {
		return 0, ErrInvalidDocument
	}
	id, next, err := b.availableID()
	if err != nil {
		return 0, err
	}
	children := mappingChildren(entries)
	if err := b.validateNewChildren(id, children); err != nil {
		return 0, err
	}
	b.draft.nodes[id] = &docNode{id: id, kind: NodeMapping, mapping: &mappingNode{entries: cloneMappingEntries(entries)}}
	for index, entry := range entries {
		b.draft.parents[entry.Key] = ParentRef{Parent: id, Role: ParentMappingKey, Index: index}
		b.draft.parents[entry.Value] = ParentRef{Parent: id, Role: ParentMappingValue, Index: index}
	}
	if err := b.commitPrepared(b.draft.root, b.draft.nodes, b.draft.reserved, next, b.draft.parents, b.draft.refs, true); err != nil {
		return 0, err
	}
	return id, nil
}

// NewReference creates a reference node to an already defined node.
func (b *Builder) NewReference(target NodeID) (NodeID, error) {
	if !b.ready() {
		return 0, ErrInvalidDocument
	}
	if b.draft.nodes[target] == nil {
		return 0, ErrNodeNotFound
	}
	id, next, err := b.availableID()
	if err != nil {
		return 0, err
	}
	b.draft.nodes[id] = &docNode{id: id, kind: NodeReference, reference: &referenceNode{target: target}}
	addReference(b.draft.refs, target, id)
	if err := b.commitPrepared(b.draft.root, b.draft.nodes, b.draft.reserved, next, b.draft.parents, b.draft.refs, true); err != nil {
		return 0, err
	}
	return id, nil
}

// Reserve allocates an ID for a node that will be defined later. It is useful
// when constructing cyclic references. Every reservation must be defined
// before Build succeeds.
func (b *Builder) Reserve() (NodeID, error) {
	if !b.ready() {
		return 0, ErrInvalidDocument
	}
	id, next, err := b.availableID()
	if err != nil {
		return 0, err
	}
	b.draft.reserved[id] = struct{}{}
	if err := b.commitPrepared(b.draft.root, b.draft.nodes, b.draft.reserved, next, b.draft.parents, b.draft.refs, false); err != nil {
		return 0, err
	}
	return id, nil
}

// DefineScalar defines a previously reserved ID as a scalar node.
func (b *Builder) DefineScalar(id NodeID, value any) error {
	if !b.ready() {
		return ErrInvalidDocument
	}
	if err := b.requireReservation(id); err != nil {
		return err
	}
	scalar, err := normalizeScalar(value)
	if err != nil {
		return err
	}
	b.draft.nodes[id] = &docNode{id: id, kind: NodeScalar, scalar: scalar}
	delete(b.draft.reserved, id)
	return b.commitPrepared(b.draft.root, b.draft.nodes, b.draft.reserved, b.draft.nextID, b.draft.parents, b.draft.refs, true)
}

// DefineSequence defines a reserved ID as a sequence.
func (b *Builder) DefineSequence(id NodeID, items []NodeID) error {
	if !b.ready() {
		return ErrInvalidDocument
	}
	if err := b.requireReservation(id); err != nil {
		return err
	}
	if err := b.validateDefinitionChildren(id, items); err != nil {
		return err
	}
	b.draft.nodes[id] = &docNode{id: id, kind: NodeSequence, sequence: &sequenceNode{items: append([]NodeID(nil), items...)}}
	for index, child := range items {
		b.draft.parents[child] = ParentRef{Parent: id, Role: ParentSequenceItem, Index: index}
	}
	delete(b.draft.reserved, id)
	return b.commitPrepared(b.draft.root, b.draft.nodes, b.draft.reserved, b.draft.nextID, b.draft.parents, b.draft.refs, true)
}

// DefineMapping defines a reserved ID as a mapping.
func (b *Builder) DefineMapping(id NodeID, entries []MappingEntry) error {
	if !b.ready() {
		return ErrInvalidDocument
	}
	if err := b.requireReservation(id); err != nil {
		return err
	}
	if err := b.validateDefinitionChildren(id, mappingChildren(entries)); err != nil {
		return err
	}
	b.draft.nodes[id] = &docNode{id: id, kind: NodeMapping, mapping: &mappingNode{entries: cloneMappingEntries(entries)}}
	for index, entry := range entries {
		b.draft.parents[entry.Key] = ParentRef{Parent: id, Role: ParentMappingKey, Index: index}
		b.draft.parents[entry.Value] = ParentRef{Parent: id, Role: ParentMappingValue, Index: index}
	}
	delete(b.draft.reserved, id)
	return b.commitPrepared(b.draft.root, b.draft.nodes, b.draft.reserved, b.draft.nextID, b.draft.parents, b.draft.refs, true)
}

// DefineReference defines a reserved ID as a reference. target may itself be
// reserved, which permits a group of references to be defined cyclically.
func (b *Builder) DefineReference(id, target NodeID) error {
	if !b.ready() {
		return ErrInvalidDocument
	}
	if err := b.requireReservation(id); err != nil {
		return err
	}
	if b.draft.nodes[target] == nil {
		if _, reserved := b.draft.reserved[target]; !reserved {
			return ErrNodeNotFound
		}
	}
	b.draft.nodes[id] = &docNode{id: id, kind: NodeReference, reference: &referenceNode{target: target}}
	addReference(b.draft.refs, target, id)
	delete(b.draft.reserved, id)
	return b.commitPrepared(b.draft.root, b.draft.nodes, b.draft.reserved, b.draft.nextID, b.draft.parents, b.draft.refs, true)
}

// SetRoot selects the single ownership root. The selected node must exist and
// must not already have an ownership parent.
func (b *Builder) SetRoot(id NodeID) error {
	if !b.ready() {
		return ErrInvalidDocument
	}
	if b.draft.nodes[id] == nil {
		return ErrNodeNotFound
	}
	if parent, owned := b.draft.parents[id]; owned {
		_ = parent
		return ErrNodeAlreadyOwned
	}
	if b.draft.root == id {
		return nil
	}
	return b.commitPrepared(id, b.draft.nodes, b.draft.reserved, b.draft.nextID, b.draft.parents, b.draft.refs, true)
}

// SetScalar changes a scalar's value without changing its ScalarKind.
func (b *Builder) SetScalar(id NodeID, value any) error {
	if !b.ready() {
		return ErrInvalidDocument
	}
	entry := b.draft.nodes[id]
	if entry == nil {
		return ErrNodeNotFound
	}
	if entry.kind != NodeScalar || entry.scalar == nil {
		return ErrInvalidNodeKind
	}
	scalar, err := normalizeScalar(value)
	if err != nil {
		return err
	}
	if scalar.kind != entry.scalar.kind {
		return ErrInvalidScalar
	}
	if equalScalar(entry.scalar, scalar) {
		return nil
	}
	entry.scalar = scalar
	return b.commitPrepared(b.draft.root, b.draft.nodes, b.draft.reserved, b.draft.nextID, b.draft.parents, b.draft.refs, true)
}

// SetSequenceItems replaces a sequence's ordered items. Ownership subtrees of
// removed items are deleted, unless a surviving reference points into them.
func (b *Builder) SetSequenceItems(id NodeID, items []NodeID) error {
	if !b.ready() {
		return ErrInvalidDocument
	}
	entry := b.draft.nodes[id]
	if entry == nil {
		return ErrNodeNotFound
	}
	if entry.kind != NodeSequence || entry.sequence == nil {
		return ErrInvalidNodeKind
	}
	if equalNodeIDs(entry.sequence.items, items) {
		return nil
	}
	if err := b.validateReplacementChildren(id, items); err != nil {
		return err
	}
	removed, err := b.removedSubtrees(entry.sequence.items, items)
	if err != nil {
		return err
	}
	if err := b.checkSurvivingReferences(removed); err != nil {
		return err
	}
	nodes := cloneNodeMap(b.draft.nodes)
	nodes[id].sequence.items = append([]NodeID(nil), items...)
	for nodeID := range removed {
		delete(nodes, nodeID)
	}
	return b.commit(b.draft.root, nodes, cloneReserved(b.draft.reserved), b.draft.nextID, true)
}

// SetMappingEntries replaces a mapping's ordered entries. Ownership subtrees
// of removed keys and values are deleted, unless a surviving reference points
// into them.
func (b *Builder) SetMappingEntries(id NodeID, entries []MappingEntry) error {
	if !b.ready() {
		return ErrInvalidDocument
	}
	entry := b.draft.nodes[id]
	if entry == nil {
		return ErrNodeNotFound
	}
	if entry.kind != NodeMapping || entry.mapping == nil {
		return ErrInvalidNodeKind
	}
	if equalMappingEntries(entry.mapping.entries, entries) {
		return nil
	}
	children := mappingChildren(entries)
	if err := b.validateReplacementChildren(id, children); err != nil {
		return err
	}
	oldChildren := mappingChildrenFromInternal(entry.mapping.entries)
	removed, err := b.removedSubtrees(oldChildren, children)
	if err != nil {
		return err
	}
	if err := b.checkSurvivingReferences(removed); err != nil {
		return err
	}
	nodes := cloneNodeMap(b.draft.nodes)
	nodes[id].mapping.entries = cloneMappingEntries(entries)
	for nodeID := range removed {
		delete(nodes, nodeID)
	}
	return b.commit(b.draft.root, nodes, cloneReserved(b.draft.reserved), b.draft.nextID, true)
}

// SetRestrictions changes the restrictions directly stored on id.
func (b *Builder) SetRestrictions(id NodeID, restrictions NodeRestrictions) error {
	if !b.ready() {
		return ErrInvalidDocument
	}
	entry := b.draft.nodes[id]
	if entry == nil {
		return ErrNodeNotFound
	}
	if restrictions & ^(RestrictionReadOnly|RestrictionKeyReadOnly|RestrictionReferenceOrder) != 0 {
		return ErrInvalidDocument
	}
	if entry.restrictions == restrictions {
		return nil
	}
	entry.restrictions = restrictions
	return b.commitPrepared(b.draft.root, b.draft.nodes, b.draft.reserved, b.draft.nextID, b.draft.parents, b.draft.refs, true)
}

// RestoreContentFrom restores the content of a snapshot from the same lineage.
// The allocator never moves backward, and any outstanding reservations are
// discarded.
func (b *Builder) RestoreContentFrom(doc *Document) error {
	if !b.ready() || doc == nil || doc.lineage == 0 || b.draft.lineage != doc.lineage {
		return ErrInvalidDocument
	}
	parents, refs, err := validateSnapshot(doc)
	if err != nil {
		return err
	}
	next := maxNextID(b.draft.nextID, doc.nextID)
	sameContent := contentEqual(b.draft.root, b.draft.nodes, doc.root, doc.nodes)
	if sameContent && len(b.draft.reserved) == 0 && next == b.draft.nextID {
		return nil
	}
	changed := !sameContent
	nodes := cloneNodeMap(doc.nodes)
	if err := b.commitPrepared(doc.root, nodes, make(map[NodeID]struct{}), next, parents, refs, changed); err != nil {
		return err
	}
	return nil
}

// Build validates the full ownership tree and reference graph and returns an
// immutable snapshot. On success the Builder rebases to that snapshot.
func (b *Builder) Build() (*Document, error) {
	if !b.ready() {
		return nil, ErrInvalidDocument
	}
	parents, refs, err := validateBuild(b.draft)
	if err != nil {
		return nil, err
	}
	revision := b.baseRevision
	if b.changed {
		if revision == Revision(^uint64(0)) {
			return nil, ErrInvalidDocument
		}
		revision++
	}
	doc := &Document{
		root:     b.draft.root,
		nodes:    cloneNodeMap(b.draft.nodes),
		parents:  cloneParents(parents),
		refs:     cloneReferences(refs),
		nextID:   b.draft.nextID,
		revision: revision,
		lineage:  b.draft.lineage,
	}
	b.draft.parents = parents
	b.draft.refs = refs
	b.baseRevision = revision
	b.changed = false
	return doc, nil
}

func (b *Builder) ready() bool {
	return b != nil && b.draft != nil && b.draft.lineage != 0
}

func (b *Builder) availableID() (NodeID, NodeID, error) {
	if b.draft.nextID == 0 {
		return 0, 0, ErrInvalidDocument
	}
	id := b.draft.nextID
	if b.draft.nodes[id] != nil {
		return 0, 0, ErrInvalidDocument
	}
	if _, reserved := b.draft.reserved[id]; reserved {
		return 0, 0, ErrInvalidDocument
	}
	if id == ^NodeID(0) {
		return id, 0, nil
	}
	return id, id + 1, nil
}

func (b *Builder) requireReservation(id NodeID) error {
	if id == 0 {
		return ErrInvalidDocument
	}
	if b.draft.nodes[id] != nil {
		return ErrNodeAlreadyDefined
	}
	if _, ok := b.draft.reserved[id]; !ok {
		return ErrNodeNotFound
	}
	return nil
}

func (b *Builder) validateNewChildren(parent NodeID, children []NodeID) error {
	seen := make(map[NodeID]struct{}, len(children))
	for _, child := range children {
		if child == 0 || b.draft.nodes[child] == nil {
			return ErrNodeNotFound
		}
		if _, duplicate := seen[child]; duplicate {
			return ErrNodeAlreadyOwned
		}
		seen[child] = struct{}{}
		if existing, owned := b.draft.parents[child]; owned && existing.Parent != parent {
			return ErrNodeAlreadyOwned
		}
	}
	return nil
}

// validateDefinitionChildren also rejects a reserved parent that is already
// below one of the proposed children in the current ownership forest.
func (b *Builder) validateDefinitionChildren(parent NodeID, children []NodeID) error {
	if err := b.validateNewChildren(parent, children); err != nil {
		return err
	}
	for _, child := range children {
		for current := parent; current != 0; {
			if current == child {
				return ErrInvalidDocument
			}
			parentRef, ok := b.draft.parents[current]
			if !ok {
				break
			}
			current = parentRef.Parent
		}
	}
	return nil
}

func (b *Builder) validateReplacementChildren(parent NodeID, children []NodeID) error {
	seen := make(map[NodeID]struct{}, len(children))
	for _, child := range children {
		if child == 0 || b.draft.nodes[child] == nil {
			return ErrNodeNotFound
		}
		if _, duplicate := seen[child]; duplicate {
			return ErrNodeAlreadyOwned
		}
		seen[child] = struct{}{}
		if existing, owned := b.draft.parents[child]; owned && existing.Parent != parent {
			return ErrNodeAlreadyOwned
		}
	}
	return nil
}

func (b *Builder) removedSubtrees(oldChildren, newChildren []NodeID) (map[NodeID]struct{}, error) {
	kept := make(map[NodeID]struct{}, len(newChildren))
	for _, id := range newChildren {
		kept[id] = struct{}{}
	}
	removed := make(map[NodeID]struct{})
	stack := make([]NodeID, 0)
	for _, old := range oldChildren {
		if _, retained := kept[old]; !retained {
			stack = append(stack, old)
		}
	}
	for len(stack) > 0 {
		last := len(stack) - 1
		id := stack[last]
		stack = stack[:last]
		if _, visited := removed[id]; visited {
			continue
		}
		entry := b.draft.nodes[id]
		if entry == nil {
			return nil, ErrInvalidDocument
		}
		removed[id] = struct{}{}
		switch entry.kind {
		case NodeSequence:
			stack = append(stack, entry.sequence.items...)
		case NodeMapping:
			stack = append(stack, mappingChildrenFromInternal(entry.mapping.entries)...)
		}
	}
	return removed, nil
}

func (b *Builder) checkSurvivingReferences(removed map[NodeID]struct{}) error {
	for target := range removed {
		for source := range b.draft.refs[target] {
			if _, alsoRemoved := removed[source]; !alsoRemoved {
				return ErrInvalidDocument
			}
		}
	}
	return nil
}

func (b *Builder) commit(root NodeID, nodes map[NodeID]*docNode, reserved map[NodeID]struct{}, nextID NodeID, semanticChange bool) error {
	parents, refs, err := rebuildIndexes(root, nodes, reserved, nextID)
	if err != nil {
		return err
	}
	return b.commitPrepared(root, nodes, reserved, nextID, parents, refs, semanticChange)
}

// commitPrepared installs a draft whose allocator and indexes were validated
// or updated by the caller.
func (b *Builder) commitPrepared(root NodeID, nodes map[NodeID]*docNode, reserved map[NodeID]struct{}, nextID NodeID, parents map[NodeID]ParentRef, refs map[NodeID]map[NodeID]struct{}, semanticChange bool) error {
	if !b.ready() {
		return ErrInvalidDocument
	}
	b.draft = &documentDraft{
		root:     root,
		nodes:    nodes,
		parents:  parents,
		refs:     refs,
		nextID:   nextID,
		lineage:  b.draft.lineage,
		reserved: reserved,
	}
	if semanticChange {
		b.changed = true
	}
	return nil
}

func validateSnapshot(doc *Document) (map[NodeID]ParentRef, map[NodeID]map[NodeID]struct{}, error) {
	if doc == nil || doc.lineage == 0 {
		return nil, nil, ErrInvalidDocument
	}
	parents, refs, err := rebuildIndexes(doc.root, doc.nodes, nil, doc.nextID)
	if err != nil {
		return nil, nil, err
	}
	if err := validateRootedTree(doc.root, doc.nodes, parents); err != nil {
		return nil, nil, err
	}
	if !equalParents(doc.parents, parents) || !equalReferences(doc.refs, refs) {
		return nil, nil, ErrInvalidDocument
	}
	return parents, refs, nil
}

func validateBuild(draft *documentDraft) (map[NodeID]ParentRef, map[NodeID]map[NodeID]struct{}, error) {
	if draft == nil || len(draft.reserved) != 0 {
		return nil, nil, ErrInvalidDocument
	}
	parents, refs, err := rebuildIndexes(draft.root, draft.nodes, draft.reserved, draft.nextID)
	if err != nil {
		return nil, nil, err
	}
	if err := validateRootedTree(draft.root, draft.nodes, parents); err != nil {
		return nil, nil, err
	}
	return parents, refs, nil
}

func validateRootedTree(root NodeID, nodes map[NodeID]*docNode, parents map[NodeID]ParentRef) error {
	if root == 0 || nodes[root] == nil {
		return ErrInvalidDocument
	}
	if _, hasParent := parents[root]; hasParent {
		return ErrInvalidDocument
	}
	visited := make(map[NodeID]struct{}, len(nodes))
	stack := []NodeID{root}
	for len(stack) > 0 {
		last := len(stack) - 1
		id := stack[last]
		stack = stack[:last]
		if _, seen := visited[id]; seen {
			return ErrInvalidDocument
		}
		entry := nodes[id]
		if entry == nil {
			return ErrInvalidDocument
		}
		visited[id] = struct{}{}
		switch entry.kind {
		case NodeSequence:
			stack = append(stack, entry.sequence.items...)
		case NodeMapping:
			stack = append(stack, mappingChildrenFromInternal(entry.mapping.entries)...)
		}
	}
	if len(visited) != len(nodes) {
		return ErrInvalidDocument
	}
	return nil
}

func rebuildIndexes(root NodeID, nodes map[NodeID]*docNode, reserved map[NodeID]struct{}, nextID NodeID) (map[NodeID]ParentRef, map[NodeID]map[NodeID]struct{}, error) {
	if err := validateAllocator(nodes, reserved, nextID); err != nil {
		return nil, nil, err
	}
	parents := make(map[NodeID]ParentRef)
	refs := make(map[NodeID]map[NodeID]struct{})
	addParent := func(child, parent NodeID, role ParentRole, index int) error {
		if child == 0 || nodes[child] == nil {
			return ErrInvalidDocument
		}
		if _, exists := parents[child]; exists {
			return ErrNodeAlreadyOwned
		}
		parents[child] = ParentRef{Parent: parent, Role: role, Index: index}
		return nil
	}
	for id, node := range nodes {
		if id == 0 || node == nil || node.id != id || node.restrictions & ^(RestrictionReadOnly|RestrictionKeyReadOnly|RestrictionReferenceOrder) != 0 {
			return nil, nil, ErrInvalidDocument
		}
		switch node.kind {
		case NodeScalar:
			if node.scalar == nil || node.mapping != nil || node.sequence != nil || node.reference != nil || !validScalar(node.scalar) {
				return nil, nil, ErrInvalidDocument
			}
		case NodeSequence:
			if node.scalar != nil || node.mapping != nil || node.sequence == nil || node.reference != nil {
				return nil, nil, ErrInvalidDocument
			}
			for index, child := range node.sequence.items {
				if err := addParent(child, id, ParentSequenceItem, index); err != nil {
					return nil, nil, err
				}
			}
		case NodeMapping:
			if node.scalar != nil || node.mapping == nil || node.sequence != nil || node.reference != nil {
				return nil, nil, ErrInvalidDocument
			}
			for index, entry := range node.mapping.entries {
				if err := addParent(entry.key, id, ParentMappingKey, index); err != nil {
					return nil, nil, err
				}
				if err := addParent(entry.value, id, ParentMappingValue, index); err != nil {
					return nil, nil, err
				}
			}
		case NodeReference:
			if node.scalar != nil || node.mapping != nil || node.sequence != nil || node.reference == nil || node.reference.target == 0 {
				return nil, nil, ErrInvalidDocument
			}
			if nodes[node.reference.target] == nil {
				if _, pending := reserved[node.reference.target]; !pending {
					return nil, nil, ErrInvalidDocument
				}
			}
			if refs[node.reference.target] == nil {
				refs[node.reference.target] = make(map[NodeID]struct{})
			}
			refs[node.reference.target][id] = struct{}{}
		default:
			return nil, nil, ErrInvalidDocument
		}
	}

	// The parent index is a forest during construction. Reject cycles while
	// allowing temporary unattached nodes and a root that will later be wrapped.
	state := make(map[NodeID]uint8, len(nodes))
	for id := range nodes {
		if state[id] == 2 {
			continue
		}
		path := make([]NodeID, 0)
		current := id
		for current != 0 && state[current] == 0 {
			state[current] = 1
			path = append(path, current)
			parent, hasParent := parents[current]
			if !hasParent {
				current = 0
			} else {
				current = parent.Parent
			}
		}
		if current != 0 && state[current] == 1 {
			return nil, nil, ErrInvalidDocument
		}
		for _, visited := range path {
			state[visited] = 2
		}
	}
	_ = root // root ownership is checked when Build validates the rooted tree.
	return parents, refs, nil
}

func validateAllocator(nodes map[NodeID]*docNode, reserved map[NodeID]struct{}, nextID NodeID) error {
	var greatest NodeID
	for id := range nodes {
		if id == 0 {
			return ErrInvalidDocument
		}
		if id > greatest {
			greatest = id
		}
	}
	for id := range reserved {
		if id == 0 || nodes[id] != nil {
			return ErrInvalidDocument
		}
		if id > greatest {
			greatest = id
		}
	}
	if nextID == 0 {
		if greatest != ^NodeID(0) {
			return ErrInvalidDocument
		}
	} else if nextID <= greatest {
		return ErrInvalidDocument
	}
	return nil
}

func cloneNodeMap(nodes map[NodeID]*docNode) map[NodeID]*docNode {
	cloned := make(map[NodeID]*docNode, len(nodes))
	for id, node := range nodes {
		cloned[id] = cloneDocNode(node)
	}
	return cloned
}

func cloneDocNode(node *docNode) *docNode {
	if node == nil {
		return nil
	}
	cloned := &docNode{id: node.id, kind: node.kind, restrictions: node.restrictions}
	if node.scalar != nil {
		cloned.scalar = cloneScalar(node.scalar)
	}
	if node.mapping != nil {
		cloned.mapping = &mappingNode{entries: append([]mappingEntry(nil), node.mapping.entries...)}
	}
	if node.sequence != nil {
		cloned.sequence = &sequenceNode{items: append([]NodeID(nil), node.sequence.items...)}
	}
	if node.reference != nil {
		cloned.reference = &referenceNode{target: node.reference.target}
	}
	return cloned
}

func cloneReserved(reserved map[NodeID]struct{}) map[NodeID]struct{} {
	cloned := make(map[NodeID]struct{}, len(reserved))
	for id := range reserved {
		cloned[id] = struct{}{}
	}
	return cloned
}

func cloneParents(parents map[NodeID]ParentRef) map[NodeID]ParentRef {
	cloned := make(map[NodeID]ParentRef, len(parents))
	for id, parent := range parents {
		cloned[id] = parent
	}
	return cloned
}

func cloneReferences(refs map[NodeID]map[NodeID]struct{}) map[NodeID]map[NodeID]struct{} {
	cloned := make(map[NodeID]map[NodeID]struct{}, len(refs))
	for target, sources := range refs {
		copySources := make(map[NodeID]struct{}, len(sources))
		for source := range sources {
			copySources[source] = struct{}{}
		}
		cloned[target] = copySources
	}
	return cloned
}

func addReference(refs map[NodeID]map[NodeID]struct{}, target, source NodeID) {
	if refs[target] == nil {
		refs[target] = make(map[NodeID]struct{})
	}
	refs[target][source] = struct{}{}
}

func mappingChildren(entries []MappingEntry) []NodeID {
	children := make([]NodeID, 0, len(entries)*2)
	for _, entry := range entries {
		children = append(children, entry.Key, entry.Value)
	}
	return children
}

func mappingChildrenFromInternal(entries []mappingEntry) []NodeID {
	children := make([]NodeID, 0, len(entries)*2)
	for _, entry := range entries {
		children = append(children, entry.key, entry.value)
	}
	return children
}

func cloneMappingEntries(entries []MappingEntry) []mappingEntry {
	cloned := make([]mappingEntry, len(entries))
	for i, entry := range entries {
		cloned[i] = mappingEntry{key: entry.Key, value: entry.Value}
	}
	return cloned
}

func equalMappingEntries(left []mappingEntry, right []MappingEntry) bool {
	if len(left) != len(right) {
		return false
	}
	for i, entry := range left {
		if entry.key != right[i].Key || entry.value != right[i].Value {
			return false
		}
	}
	return true
}

func equalNodeIDs(left, right []NodeID) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func maxNextID(left, right NodeID) NodeID {
	if left == 0 || right == 0 {
		return 0
	}
	if left > right {
		return left
	}
	return right
}

func contentEqual(leftRoot NodeID, left map[NodeID]*docNode, rightRoot NodeID, right map[NodeID]*docNode) bool {
	if leftRoot != rightRoot || len(left) != len(right) {
		return false
	}
	for id, leftNode := range left {
		rightNode := right[id]
		if rightNode == nil || leftNode == nil || leftNode.kind != rightNode.kind || leftNode.restrictions != rightNode.restrictions {
			return false
		}
		switch leftNode.kind {
		case NodeScalar:
			if !equalScalar(leftNode.scalar, rightNode.scalar) {
				return false
			}
		case NodeSequence:
			if !equalNodeIDs(leftNode.sequence.items, rightNode.sequence.items) {
				return false
			}
		case NodeMapping:
			if len(leftNode.mapping.entries) != len(rightNode.mapping.entries) {
				return false
			}
			for index, entry := range leftNode.mapping.entries {
				other := rightNode.mapping.entries[index]
				if entry != other {
					return false
				}
			}
		case NodeReference:
			if leftNode.reference.target != rightNode.reference.target {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func equalParents(left, right map[NodeID]ParentRef) bool {
	if len(left) != len(right) {
		return false
	}
	for id, parent := range left {
		if other, ok := right[id]; !ok || other != parent {
			return false
		}
	}
	return true
}

func equalReferences(left, right map[NodeID]map[NodeID]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for target, sources := range left {
		other, ok := right[target]
		if !ok || len(other) != len(sources) {
			return false
		}
		for source := range sources {
			if _, exists := other[source]; !exists {
				return false
			}
		}
	}
	return true
}
