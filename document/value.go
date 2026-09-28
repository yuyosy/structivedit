package document

import (
	"math"
	"reflect"
)

// ScalarKind identifies the normalized type stored in a scalar node.
type ScalarKind uint8

const (
	ScalarString ScalarKind = iota
	ScalarBool
	ScalarInteger
	ScalarFloat
	ScalarNull
)

type scalarNode struct {
	kind  ScalarKind
	value any
}

func normalizeScalar(value any) (*scalarNode, error) {
	if value == nil {
		return &scalarNode{kind: ScalarNull}, nil
	}

	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.String:
		return &scalarNode{kind: ScalarString, value: reflected.String()}, nil
	case reflect.Bool:
		return &scalarNode{kind: ScalarBool, value: reflected.Bool()}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &scalarNode{kind: ScalarInteger, value: reflected.Int()}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		unsigned := reflected.Uint()
		if unsigned > uint64(^uint64(0)>>1) {
			return nil, ErrInvalidScalar
		}
		return &scalarNode{kind: ScalarInteger, value: int64(unsigned)}, nil
	case reflect.Float32, reflect.Float64:
		return &scalarNode{kind: ScalarFloat, value: reflected.Float()}, nil
	default:
		return nil, ErrInvalidScalar
	}
}

func validScalar(value *scalarNode) bool {
	if value == nil {
		return false
	}
	switch value.kind {
	case ScalarString:
		_, ok := value.value.(string)
		return ok
	case ScalarBool:
		_, ok := value.value.(bool)
		return ok
	case ScalarInteger:
		_, ok := value.value.(int64)
		return ok
	case ScalarFloat:
		_, ok := value.value.(float64)
		return ok
	case ScalarNull:
		return value.value == nil
	default:
		return false
	}
}

func cloneScalar(value *scalarNode) *scalarNode {
	if value == nil {
		return nil
	}
	return &scalarNode{kind: value.kind, value: value.value}
}

func equalScalar(left, right *scalarNode) bool {
	if left == nil || right == nil || left.kind != right.kind {
		return left == right
	}
	switch left.kind {
	case ScalarString:
		return left.value.(string) == right.value.(string)
	case ScalarBool:
		return left.value.(bool) == right.value.(bool)
	case ScalarInteger:
		return left.value.(int64) == right.value.(int64)
	case ScalarFloat:
		return math.Float64bits(left.value.(float64)) == math.Float64bits(right.value.(float64))
	case ScalarNull:
		return true
	default:
		return false
	}
}
