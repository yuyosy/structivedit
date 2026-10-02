package selector

import (
	"github.com/yuyosy/structivedit/document"
	"testing"
)

func FuzzParseMatch(f *testing.F) {
	for _, seed := range []string{"$", "$[0]", "$.**", "$[*]", "$[\"key\"]"} {
		f.Add(seed)
	}
	b := document.NewBuilder()
	value, _ := b.NewScalar("value")
	root, _ := b.NewSequence([]document.NodeID{value})
	b.SetRoot(root)
	doc, err := b.Build()
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 512 {
			return
		}
		pattern, err := Parse(input)
		if err != nil {
			return
		}
		selected := Match(doc, pattern)
		for _, id := range []document.NodeID{root, value} {
			found := false
			for _, match := range selected {
				if match == id {
					found = true
				}
			}
			if MatchesNode(doc, pattern, id) != found {
				t.Fatalf("inconsistent match: %q", input)
			}
		}
	})
}
