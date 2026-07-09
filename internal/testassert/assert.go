package testassert

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func formatMsg(msgAndArgs ...any) string {
	if len(msgAndArgs) == 0 {
		return ""
	}
	if s, ok := msgAndArgs[0].(string); ok {
		if len(msgAndArgs) == 1 {
			if s == "" {
				return ""
			}
			return "\n" + s
		}
		return "\n" + fmt.Sprintf(s, msgAndArgs[1:]...)
	}
	return "\n" + fmt.Sprint(msgAndArgs...)
}

func fail(t *testing.T, fatal bool, msg string) {
	t.Helper()
	if fatal {
		t.Fatal(msg)
	}
	t.Error(msg)
}

func Equal(t *testing.T, expected, actual any, msgAndArgs ...any) {
	t.Helper()
	if !reflect.DeepEqual(expected, actual) {
		fail(t, false, fmt.Sprintf("not equal:\nexpected: %#v\nactual:   %#v%s", expected, actual, formatMsg(msgAndArgs...)))
	}
}

func RequireEqual(t *testing.T, expected, actual any, msgAndArgs ...any) {
	t.Helper()
	if !reflect.DeepEqual(expected, actual) {
		fail(t, true, fmt.Sprintf("not equal:\nexpected: %#v\nactual:   %#v%s", expected, actual, formatMsg(msgAndArgs...)))
	}
}

func NotEqual(t *testing.T, expected, actual any, msgAndArgs ...any) {
	t.Helper()
	if reflect.DeepEqual(expected, actual) {
		fail(t, false, fmt.Sprintf("should not be equal: %#v%s", actual, formatMsg(msgAndArgs...)))
	}
}

func True(t *testing.T, value bool, msgAndArgs ...any) {
	t.Helper()
	if !value {
		fail(t, false, "expected true"+formatMsg(msgAndArgs...))
	}
}

func False(t *testing.T, value bool, msgAndArgs ...any) {
	t.Helper()
	if value {
		fail(t, false, "expected false"+formatMsg(msgAndArgs...))
	}
}

func Nil(t *testing.T, object any, msgAndArgs ...any) {
	t.Helper()
	if !isNil(object) {
		fail(t, false, fmt.Sprintf("expected nil, got %#v%s", object, formatMsg(msgAndArgs...)))
	}
}

func RequireNil(t *testing.T, object any, msgAndArgs ...any) {
	t.Helper()
	if !isNil(object) {
		fail(t, true, fmt.Sprintf("expected nil, got %#v%s", object, formatMsg(msgAndArgs...)))
	}
}

func NotNil(t *testing.T, object any, msgAndArgs ...any) {
	t.Helper()
	if isNil(object) {
		fail(t, false, "expected non-nil"+formatMsg(msgAndArgs...))
	}
}

func RequireNotNil(t *testing.T, object any, msgAndArgs ...any) {
	t.Helper()
	if isNil(object) {
		fail(t, true, "expected non-nil"+formatMsg(msgAndArgs...))
	}
}

func isNil(object any) bool {
	if object == nil {
		return true
	}
	v := reflect.ValueOf(object)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func Empty(t *testing.T, object any, msgAndArgs ...any) {
	t.Helper()
	if !isEmpty(object) {
		fail(t, false, fmt.Sprintf("expected empty, got %#v%s", object, formatMsg(msgAndArgs...)))
	}
}

func NotEmpty(t *testing.T, object any, msgAndArgs ...any) {
	t.Helper()
	if isEmpty(object) {
		fail(t, false, "expected non-empty"+formatMsg(msgAndArgs...))
	}
}

func isEmpty(object any) bool {
	if object == nil {
		return true
	}
	v := reflect.ValueOf(object)
	switch v.Kind() {
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Ptr, reflect.Interface:
		if v.IsNil() {
			return true
		}
		return isEmpty(v.Elem().Interface())
	default:
		zero := reflect.Zero(v.Type())
		return reflect.DeepEqual(object, zero.Interface())
	}
}

func Len(t *testing.T, object any, length int, msgAndArgs ...any) {
	t.Helper()
	v := reflect.ValueOf(object)
	switch v.Kind() {
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
		if v.Len() != length {
			fail(t, false, fmt.Sprintf("unexpected length: got %d, want %d%s", v.Len(), length, formatMsg(msgAndArgs...)))
		}
	default:
		fail(t, false, fmt.Sprintf("cannot take len of %T%s", object, formatMsg(msgAndArgs...)))
	}
}

func RequireLen(t *testing.T, object any, length int, msgAndArgs ...any) {
	t.Helper()
	v := reflect.ValueOf(object)
	switch v.Kind() {
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
		if v.Len() != length {
			fail(t, true, fmt.Sprintf("unexpected length: got %d, want %d%s", v.Len(), length, formatMsg(msgAndArgs...)))
		}
	default:
		fail(t, true, fmt.Sprintf("cannot take len of %T%s", object, formatMsg(msgAndArgs...)))
	}
}

func NoError(t *testing.T, err error, msgAndArgs ...any) {
	t.Helper()
	if err != nil {
		fail(t, false, fmt.Sprintf("unexpected error: %v%s", err, formatMsg(msgAndArgs...)))
	}
}

func RequireNoError(t *testing.T, err error, msgAndArgs ...any) {
	t.Helper()
	if err != nil {
		fail(t, true, fmt.Sprintf("unexpected error: %v%s", err, formatMsg(msgAndArgs...)))
	}
}

func Error(t *testing.T, err error, msgAndArgs ...any) {
	t.Helper()
	if err == nil {
		fail(t, false, "expected error"+formatMsg(msgAndArgs...))
	}
}

func ErrorIs(t *testing.T, err, target error, msgAndArgs ...any) {
	t.Helper()
	if !errors.Is(err, target) {
		fail(t, false, fmt.Sprintf("errors.Is failed: got %v, want %v%s", err, target, formatMsg(msgAndArgs...)))
	}
}

func Contains(t *testing.T, container, item any, msgAndArgs ...any) {
	t.Helper()
	if !contains(container, item) {
		fail(t, false, fmt.Sprintf("%#v does not contain %#v%s", container, item, formatMsg(msgAndArgs...)))
	}
}

func NotContains(t *testing.T, container, item any, msgAndArgs ...any) {
	t.Helper()
	if contains(container, item) {
		fail(t, false, fmt.Sprintf("%#v unexpectedly contains %#v%s", container, item, formatMsg(msgAndArgs...)))
	}
}

func contains(container, item any) bool {
	if s, ok := container.(string); ok {
		switch v := item.(type) {
		case string:
			return strings.Contains(s, v)
		case []byte:
			return strings.Contains(s, string(v))
		default:
			return strings.Contains(s, fmt.Sprint(item))
		}
	}
	cv := reflect.ValueOf(container)
	switch cv.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < cv.Len(); i++ {
			if reflect.DeepEqual(cv.Index(i).Interface(), item) {
				return true
			}
		}
		return false
	case reflect.Map:
		return cv.MapIndex(reflect.ValueOf(item)).IsValid()
	default:
		return false
	}
}

func JSONEq(t *testing.T, expected, actual string, msgAndArgs ...any) {
	t.Helper()
	var e, a any
	if err := json.Unmarshal([]byte(expected), &e); err != nil {
		fail(t, true, fmt.Sprintf("expected is not valid JSON: %v\nvalue: %s", err, expected))
		return
	}
	if err := json.Unmarshal([]byte(actual), &a); err != nil {
		fail(t, true, fmt.Sprintf("actual is not valid JSON: %v\nvalue: %s", err, actual))
		return
	}
	if !reflect.DeepEqual(e, a) {
		fail(t, false, fmt.Sprintf("JSON not equal:\nexpected: %s\nactual:   %s%s", expected, actual, formatMsg(msgAndArgs...)))
	}
}

func ElementsMatch(t *testing.T, expected, actual any, msgAndArgs ...any) {
	t.Helper()
	ev := reflect.ValueOf(expected)
	av := reflect.ValueOf(actual)
	if ev.Kind() != reflect.Slice && ev.Kind() != reflect.Array {
		fail(t, false, fmt.Sprintf("expected is not a slice/array: %T%s", expected, formatMsg(msgAndArgs...)))
		return
	}
	if av.Kind() != reflect.Slice && av.Kind() != reflect.Array {
		fail(t, false, fmt.Sprintf("actual is not a slice/array: %T%s", actual, formatMsg(msgAndArgs...)))
		return
	}
	if ev.Len() != av.Len() {
		fail(t, false, fmt.Sprintf("elements length mismatch: expected %d, actual %d%s", ev.Len(), av.Len(), formatMsg(msgAndArgs...)))
		return
	}
	used := make([]bool, av.Len())
	for i := 0; i < ev.Len(); i++ {
		want := ev.Index(i).Interface()
		found := false
		for j := 0; j < av.Len(); j++ {
			if used[j] {
				continue
			}
			if reflect.DeepEqual(want, av.Index(j).Interface()) {
				used[j] = true
				found = true
				break
			}
		}
		if !found {
			fail(t, false, fmt.Sprintf("missing expected element %#v in %#v%s", want, actual, formatMsg(msgAndArgs...)))
			return
		}
	}
}

func Same(t *testing.T, expected, actual any, msgAndArgs ...any) {
	t.Helper()
	if reflect.ValueOf(expected).Pointer() != reflect.ValueOf(actual).Pointer() {
		fail(t, false, fmt.Sprintf("not same pointer:\nexpected: %p\nactual:   %p%s", expected, actual, formatMsg(msgAndArgs...)))
	}
}

func Zero(t *testing.T, value any, msgAndArgs ...any) {
	t.Helper()
	if !isEmpty(value) {
		fail(t, false, fmt.Sprintf("expected zero value, got %#v%s", value, formatMsg(msgAndArgs...)))
	}
}
