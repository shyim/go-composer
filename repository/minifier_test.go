package repository

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rawMap(t *testing.T, jsonStr string) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &m))
	return m
}

func TestExpandMetadata(t *testing.T) {
	input := []map[string]json.RawMessage{
		rawMap(t, `{"name":"a/b","version":"2.0.0","require":{"php":">=8"},"type":"library"}`),
		// only changed keys; type carried over from baseline
		rawMap(t, `{"version":"1.1.0","require":{"php":">=7"}}`),
		// remove the require key entirely, change version
		rawMap(t, `{"version":"1.0.0","require":"__unset"}`),
	}

	expanded := expandMetadata(input)
	require.Len(t, expanded, 3)

	// Baseline preserved verbatim.
	assert.JSONEq(t, `"2.0.0"`, string(expanded[0]["version"]))
	assert.JSONEq(t, `{"php":">=8"}`, string(expanded[0]["require"]))
	assert.JSONEq(t, `"library"`, string(expanded[0]["type"]))

	// Second entry: version + require updated, type inherited.
	assert.JSONEq(t, `"1.1.0"`, string(expanded[1]["version"]))
	assert.JSONEq(t, `{"php":">=7"}`, string(expanded[1]["require"]))
	assert.JSONEq(t, `"library"`, string(expanded[1]["type"]))

	// Third entry: require removed via __unset, type still inherited.
	assert.JSONEq(t, `"1.0.0"`, string(expanded[2]["version"]))
	_, hasRequire := expanded[2]["require"]
	assert.False(t, hasRequire, "require should be unset")
	assert.JSONEq(t, `"library"`, string(expanded[2]["type"]))

	// Earlier snapshots must not be mutated by later deltas.
	assert.JSONEq(t, `{"php":">=8"}`, string(expanded[0]["require"]))
}

func TestIsUnsetSentinel(t *testing.T) {
	assert.True(t, isUnsetSentinel(json.RawMessage(`"__unset"`)))
	assert.False(t, isUnsetSentinel(json.RawMessage(`"value"`)))
	assert.False(t, isUnsetSentinel(json.RawMessage(`{"a":1}`)))
	assert.False(t, isUnsetSentinel(json.RawMessage(`123`)))
}

func TestMinifyExpandRoundTrip(t *testing.T) {
	versions := []map[string]json.RawMessage{
		rawMap(t, `{"name":"a/b","version":"2.0.0","type":"library","require":{"php":">=8"}}`),
		rawMap(t, `{"name":"a/b","version":"1.1.0","type":"library","require":{"php":">=7"}}`),
		rawMap(t, `{"name":"a/b","version":"1.0.0","type":"library"}`), // require disappears
	}

	minified := minifyRaw(versions)
	require.Len(t, minified, 3)

	// The delta for v2 should not repeat unchanged keys (name, type).
	_, hasName := minified[1]["name"]
	assert.False(t, hasName, "unchanged name should be omitted from delta")
	assert.JSONEq(t, `{"php":">=7"}`, string(minified[1]["require"]))

	// v3 drops require -> __unset marker.
	assert.JSONEq(t, `"__unset"`, string(minified[2]["require"]))

	// Expanding the minified form reproduces the originals.
	expanded := expandMetadata(minified)
	require.Len(t, expanded, 3)
	for i := range versions {
		for k, want := range versions[i] {
			assert.JSONEq(t, string(want), string(expanded[i][k]), "version %d key %q", i, k)
		}
		assert.Len(t, expanded[i], len(versions[i]), "version %d key count", i)
	}
}

func TestEncodeDecodePackageVersionsRoundTrip(t *testing.T) {
	versions := []Version{
		{Name: "a/b", Version: "2.0.0", VersionNormalized: "2.0.0.0", Type: "library", Require: map[string]string{"php": ">=8"}},
		{Name: "a/b", Version: "1.0.0", VersionNormalized: "1.0.0.0", Type: "library"},
	}

	for _, minify := range []bool{false, true} {
		raw, err := EncodePackageVersions(versions, minify)
		require.NoError(t, err)

		got, err := DecodePackageVersions(raw, minify)
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, "2.0.0", got[0].Version)
		assert.Equal(t, map[string]string{"php": ">=8"}, got[0].Require)
		assert.Equal(t, "1.0.0", got[1].Version)
		assert.Nil(t, got[1].Require, "minify=%v: require should round-trip as absent", minify)
	}
}

func TestEncodePackageVersionsEmpty(t *testing.T) {
	raw, err := EncodePackageVersions(nil, true)
	require.NoError(t, err)
	assert.JSONEq(t, `[]`, string(raw))
}
