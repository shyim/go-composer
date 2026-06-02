package packagist

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
