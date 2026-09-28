package structivedit

// Capabilities reports the operations currently available for one node.
type Capabilities struct {
	Editable    bool
	Addable     bool
	Deletable   bool
	Reorderable bool
}
