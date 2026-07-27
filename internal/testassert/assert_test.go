package testassert

import (
	"reflect"
	"testing"
)

func TestIsPointerKind(t *testing.T) {
	if isPointerKind(reflect.ValueOf(123).Kind()) {
		t.Errorf("int should not be pointer kind")
	}
	if !isPointerKind(reflect.ValueOf(&struct{}{}).Kind()) {
		t.Errorf("pointer should be pointer kind")
	}
}

func TestContainsSafeNoPanic(t *testing.T) {
	m := map[string]int{"a": 1}

	// Ensure no panic when checking nil item or slice item
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("contains panicked: %v", r)
		}
	}()

	_ = contains(m, nil)
	_ = contains(m, []int{1, 2})
}

func TestIsEmptyCyclic(t *testing.T) {
	type cyclic struct {
		Self *cyclic
	}
	c := &cyclic{}
	c.Self = c

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("isEmpty panicked on cyclic struct: %v", r)
		}
	}()

	_ = isEmpty(c)
}
