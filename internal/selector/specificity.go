package selector

type score struct {
	exact     int
	recursive int
	any       int
	segments  int
}

// CompareSpecificity returns 1 if a is more specific, -1 if b is more
// specific, and 0 when their selector specificity is equal. Declaration order
// is intentionally handled by the caller.
func CompareSpecificity(a, b Pattern) int {
	left := specificity(a)
	right := specificity(b)
	switch {
	case left.exact != right.exact:
		if left.exact > right.exact {
			return 1
		}
		return -1
	case left.recursive != right.recursive:
		if left.recursive < right.recursive {
			return 1
		}
		return -1
	case left.any != right.any:
		if left.any < right.any {
			return 1
		}
		return -1
	case left.segments != right.segments:
		if left.segments > right.segments {
			return 1
		}
		return -1
	default:
		return 0
	}
}

func specificity(pattern Pattern) score {
	result := score{segments: len(pattern.segments)}
	for _, item := range pattern.segments {
		switch item.(type) {
		case keySegment, indexSegment:
			result.exact++
		case recursiveSegment:
			result.recursive++
		case anyKeySegment, anyIndexSegment:
			result.any++
		}
	}
	return result
}
