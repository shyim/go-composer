package repository

import (
	"bytes"
	"encoding/json"
	"sort"
)

// minifiedComposer2 is the value of a Metadata document's "minified" key that
// marks its version arrays as delta-encoded.
const minifiedComposer2 = "composer/2.0"

// metadataUnset is the sentinel value Composer's "composer/2.0" minified
// metadata format uses to signal that a key present in a previous version
// object must be removed from the current one.
const metadataUnset = "__unset"

// DecodePackageVersions decodes a packages[<name>] value into version objects.
// It accepts both shapes Composer uses:
//
//   - Array form (V2 metadata / partial packages):
//     [{"name":"…","version":"1.0.0"}, …]
//   - Map form (legacy packages.json catalogs, Satis, packages.shopware.com):
//     {"1.0.0":{"name":"…","version":"1.0.0"}, …}
//
// When minified is true the array form is expanded first ("composer/2.0"
// delta encoding). Minification is not defined for the map form.
func DecodePackageVersions(raw json.RawMessage, minified bool) ([]Version, error) {
	raw = json.RawMessage(bytes.TrimSpace(raw))
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	// Prefer the array form used by V2 metadata.
	if raw[0] == '[' {
		if !minified {
			var versions []Version
			if err := json.Unmarshal(raw, &versions); err == nil {
				return versions, nil
			}
		}
		var rawVersions []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &rawVersions); err != nil {
			return nil, err
		}
		if minified {
			rawVersions = expandMetadata(rawVersions)
		}
		return decodeVersionMaps(rawVersions)
	}

	// Map form: version-string → package object (Composer V1 packages.json).
	if raw[0] == '{' {
		var byVersion map[string]map[string]json.RawMessage
		if err := json.Unmarshal(raw, &byVersion); err != nil {
			return nil, err
		}
		keys := make([]string, 0, len(byVersion))
		for k := range byVersion {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		rawVersions := make([]map[string]json.RawMessage, 0, len(byVersion))
		for _, ver := range keys {
			obj := byVersion[ver]
			if obj == nil {
				obj = map[string]json.RawMessage{}
			}
			// Ensure a "version" field even if the repo keyed it only as the map key.
			if _, ok := obj["version"]; !ok {
				b, err := json.Marshal(ver)
				if err != nil {
					return nil, err
				}
				obj["version"] = b
			}
			rawVersions = append(rawVersions, obj)
		}
		return decodeVersionMaps(rawVersions)
	}

	return nil, json.Unmarshal(raw, &struct{}{}) // force a useful error for other JSON kinds
}

func decodeVersionMaps(rawVersions []map[string]json.RawMessage) ([]Version, error) {
	if len(rawVersions) == 0 {
		return nil, nil
	}
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, rv := range rawVersions {
		if i > 0 {
			buf.WriteByte(',')
		}
		b, err := json.Marshal(rv)
		if err != nil {
			return nil, err
		}
		buf.Write(b)
	}
	buf.WriteByte(']')

	var versions []Version
	if err := json.Unmarshal(buf.Bytes(), &versions); err != nil {
		return nil, err
	}
	return versions, nil
}

// EncodePackageVersions marshals versions into the JSON array placed under
// Metadata.Packages[name]. When minify is true the array is delta-encoded with
// the "composer/2.0" format (set Metadata.Minified accordingly); the matching
// decoder is DecodePackageVersions.
func EncodePackageVersions(versions []Version, minify bool) (json.RawMessage, error) {
	if versions == nil {
		versions = []Version{}
	}
	if !minify {
		return json.Marshal(versions)
	}

	raws := make([]map[string]json.RawMessage, len(versions))
	for i, v := range versions {
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		raws[i] = m
	}
	return json.Marshal(minifyRaw(raws))
}

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
			acc = cloneRawMap(delta)
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

// minifyRaw is the inverse of expandMetadata: it delta-encodes full version
// objects so that each entry after the first carries only the keys that
// changed relative to the previous one, with removed keys marked "__unset".
// This mirrors composer/metadata-minifier's minify(). The first entry is
// emitted verbatim as the baseline.
func minifyRaw(versions []map[string]json.RawMessage) []map[string]json.RawMessage {
	minified := make([]map[string]json.RawMessage, 0, len(versions))

	// last holds the accumulated state a decoder would have after the previous
	// emitted entry, so deltas are computed against it.
	var last map[string]json.RawMessage

	for _, v := range versions {
		if last == nil {
			last = cloneRawMap(v)
			minified = append(minified, v)
			continue
		}

		delta := map[string]json.RawMessage{}

		// Changed or newly-added keys.
		for key, val := range v {
			if prev, ok := last[key]; !ok || !bytes.Equal(prev, val) {
				delta[key] = val
				last[key] = val
			}
		}

		// Keys that disappeared since the previous version.
		for key := range last {
			if _, ok := v[key]; !ok {
				delta[key] = json.RawMessage(`"` + metadataUnset + `"`)
				delete(last, key)
			}
		}

		minified = append(minified, delta)
	}

	return minified
}

// isUnsetSentinel reports whether a raw JSON value is the string "__unset".
func isUnsetSentinel(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte(`"`+metadataUnset+`"`))
}

func cloneRawMap(m map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
