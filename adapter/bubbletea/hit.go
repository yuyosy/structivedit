package bubbletea

import "github.com/yuyosy/structivedit/document"

func (model *Model) hitTest(x, y int) (document.NodeID, bool) {
	if model == nil || x < 0 || y < 0 {
		return 0, false
	}
	for _, region := range model.hitRegions {
		if region.line == y {
			return region.nodeID, true
		}
	}
	return 0, false
}
