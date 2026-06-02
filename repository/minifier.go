package repository

import "encoding/json"

// metadataUnset is the sentinel value Composer's "composer/2.0" minified
// metadata format uses to signal that a key present in a previous version
// object must be removed from the current one.
const metadataUnset = "__unset"

// expandMetadata reconstructs full version objects from the delta-compressed
// representation used by the "minified": "composer/2.0" metadata format.
//
// The first entry is the baseline. Every subsequent entry carries only the
// keys that changed relative to the accumulated previous version; a value of
// "__unset" deletes that key. This mirrors composer/metadata-minifier's
// expand().
func expandMetadata(versions []map[string]json.RawMessage) []map[string]json.RawMessage {
	expanded := make([]map[string]json.RawMessage, 0, len(versions))

	// acc holds the running, fully-expanded version state.
	var acc map[string]json.RawMessage

	for _, delta := range versions {
		if acc == nil {
			acc = make(map[string]json.RawMessage, len(delta))
			for k, v := range delta {
				acc[k] = v
			}
			expanded = append(expanded, cloneRawMap(acc))
			continue
		}

		for key, val := range delta {
			if isUnsetSentinel(val) {
				delete(acc, key)
				continue
			}
			acc[key] = val
		}

		expanded = append(expanded, cloneRawMap(acc))
	}

	return expanded
}

// isUnsetSentinel reports whether a raw JSON value is the string "__unset".
func isUnsetSentinel(raw json.RawMessage) bool {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return false
	}
	return s == metadataUnset
}

func cloneRawMap(m map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
