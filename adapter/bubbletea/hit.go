package bubbletea

func (model *Model) hitTest(x, y int) (hitRegion, bool) {
	if model == nil || x < 0 || y < 0 {
		return hitRegion{}, false
	}
	for _, region := range model.hitRegions {
		if region.line == y {
			return region, true
		}
	}
	return hitRegion{}, false
}
