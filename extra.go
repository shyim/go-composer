package composer

import (
	"encoding/json"
	"math"
	"strings"
)

// ExtraData is the parsed "extra" section of a composer.json. It is a plain
// map[string]any, so existing map indexing (e.g. data["key"]) keeps working,
// but it also provides dotted-path accessors for ergonomically reading and
// manipulating nested values, for example GetString("shopware.plugin-class").
type ExtraData map[string]any

// splitPath splits a dotted path such as "a.b.c" into its segments. An empty
// path yields no segments.
func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	return strings.Split(path, ".")
}

// Get returns the value at the given dotted path and whether it was found.
// Intermediate segments must resolve to objects; a non-object encountered
// before the final segment yields (nil, false).
func (e ExtraData) Get(path string) (any, bool) {
	segments := splitPath(path)
	if len(segments) == 0 {
		return nil, false
	}

	var current any = map[string]any(e)
	for _, segment := range segments {
		asMap, ok := toStringMap(current)
		if !ok {
			return nil, false
		}
		current, ok = asMap[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

// Has reports whether a value exists at the given dotted path.
func (e ExtraData) Has(path string) bool {
	_, ok := e.Get(path)
	return ok
}

// GetString returns the string at the given dotted path and whether a string
// was found there.
func (e ExtraData) GetString(path string) (string, bool) {
	v, ok := e.Get(path)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// GetBool returns the bool at the given dotted path and whether a bool was
// found there.
func (e ExtraData) GetBool(path string) (bool, bool) {
	v, ok := e.Get(path)
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

// GetInt returns the integer at the given dotted path and whether an integer
// was found there. JSON numbers decode as float64, so whole-number float64
// values are accepted; non-integral floats are rejected.
func (e ExtraData) GetInt(path string) (int64, bool) {
	v, ok := e.Get(path)
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		if n >= -(1<<53) && n <= (1<<53) && n == math.Trunc(n) {
			return int64(n), true
		}
		return 0, false
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return i, true
		}
		if f, err := n.Float64(); err == nil {
			if f >= -(1<<53) && f <= (1<<53) && f == math.Trunc(f) {
				return int64(f), true
			}
		}
		return 0, false
	default:
		return 0, false
	}
}

// GetFloat returns the float at the given dotted path and whether a number was
// found there.
func (e ExtraData) GetFloat(path string) (float64, bool) {
	v, ok := e.Get(path)
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		if f, err := n.Float64(); err == nil {
			return f, true
		}
		return 0, false
	default:
		return 0, false
	}
}

// GetMap returns the nested object at the given dotted path as a map and
// whether an object was found there.
func (e ExtraData) GetMap(path string) (map[string]any, bool) {
	v, ok := e.Get(path)
	if !ok {
		return nil, false
	}
	return toStringMap(v)
}

// GetSlice returns the array at the given dotted path and whether an array was
// found there.
func (e ExtraData) GetSlice(path string) ([]any, bool) {
	v, ok := e.Get(path)
	if !ok {
		return nil, false
	}
	s, ok := v.([]any)
	return s, ok
}

// GetStringSlice returns the array at the given dotted path as a []string. It
// reports false if the path is absent, is not an array, or contains a
// non-string element.
func (e ExtraData) GetStringSlice(path string) ([]string, bool) {
	raw, ok := e.GetSlice(path)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		s, ok := item.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

// Set stores value at the given dotted path, creating intermediate objects as
// needed. An intermediate segment that exists but is not an object is replaced
// with a new object. Setting an empty path is a no-op.
func (e ExtraData) Set(path string, value any) {
	if e == nil {
		return
	}
	segments := splitPath(path)
	if len(segments) == 0 {
		return
	}

	current := map[string]any(e)
	for _, segment := range segments[:len(segments)-1] {
		next, ok := toStringMap(current[segment])
		if !ok {
			next = map[string]any{}
			current[segment] = next
		}
		current = next
	}
	current[segments[len(segments)-1]] = value
}

// Unset removes the value at the given dotted path. It is a no-op if the path
// or any intermediate object does not exist.
func (e ExtraData) Unset(path string) {
	segments := splitPath(path)
	if len(segments) == 0 {
		return
	}

	current := map[string]any(e)
	for _, segment := range segments[:len(segments)-1] {
		next, ok := toStringMap(current[segment])
		if !ok {
			return
		}
		current = next
	}
	delete(current, segments[len(segments)-1])
}

// toStringMap coerces a value to map[string]any. It handles both the native
// map[string]any (from json.Unmarshal and in-memory construction) and the
// ExtraData alias.
func toStringMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case ExtraData:
		return map[string]any(m), true
	default:
		return nil, false
	}
}
