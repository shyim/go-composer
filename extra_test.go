package composer

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtraDataGetters(t *testing.T) {
	raw := `{
		"shopware": {
			"plugin-class": "Acme\\Plugin",
			"enabled": true,
			"priority": 10,
			"ratio": 0.5,
			"bundles": ["a", "b"],
			"label": {"de-DE": "Hallo"}
		}
	}`
	var e ExtraData
	assert.NoError(t, json.Unmarshal([]byte(raw), &e))

	t.Run("Get and Has by path", func(t *testing.T) {
		v, ok := e.Get("shopware.plugin-class")
		assert.True(t, ok)
		assert.Equal(t, "Acme\\Plugin", v)
		assert.True(t, e.Has("shopware.enabled"))
		assert.False(t, e.Has("shopware.missing"))
		assert.False(t, e.Has("missing.deep.path"))
	})

	t.Run("GetString", func(t *testing.T) {
		s, ok := e.GetString("shopware.plugin-class")
		assert.True(t, ok)
		assert.Equal(t, "Acme\\Plugin", s)

		_, ok = e.GetString("shopware.enabled") // not a string
		assert.False(t, ok)

		_, ok = e.GetString("shopware.missing")
		assert.False(t, ok)
	})

	t.Run("GetBool", func(t *testing.T) {
		b, ok := e.GetBool("shopware.enabled")
		assert.True(t, ok)
		assert.True(t, b)

		_, ok = e.GetBool("shopware.plugin-class")
		assert.False(t, ok)
	})

	t.Run("GetInt accepts JSON whole numbers", func(t *testing.T) {
		n, ok := e.GetInt("shopware.priority")
		assert.True(t, ok)
		assert.Equal(t, int64(10), n)

		// 0.5 is not integral.
		_, ok = e.GetInt("shopware.ratio")
		assert.False(t, ok)
	})

	t.Run("GetFloat", func(t *testing.T) {
		f, ok := e.GetFloat("shopware.ratio")
		assert.True(t, ok)
		assert.Equal(t, 0.5, f)

		// Integers are also numbers.
		f, ok = e.GetFloat("shopware.priority")
		assert.True(t, ok)
		assert.Equal(t, 10.0, f)
	})

	t.Run("GetMap", func(t *testing.T) {
		m, ok := e.GetMap("shopware.label")
		assert.True(t, ok)
		assert.Equal(t, "Hallo", m["de-DE"])

		_, ok = e.GetMap("shopware.bundles") // a slice, not a map
		assert.False(t, ok)
	})

	t.Run("GetStringSlice", func(t *testing.T) {
		s, ok := e.GetStringSlice("shopware.bundles")
		assert.True(t, ok)
		assert.Equal(t, []string{"a", "b"}, s)

		// label is an object, not a slice.
		_, ok = e.GetStringSlice("shopware.label")
		assert.False(t, ok)
	})

	t.Run("nested string from object path", func(t *testing.T) {
		s, ok := e.GetString("shopware.label.de-DE")
		assert.True(t, ok)
		assert.Equal(t, "Hallo", s)
	})

	t.Run("empty path", func(t *testing.T) {
		_, ok := e.Get("")
		assert.False(t, ok)
	})
}

func TestExtraDataSetUnset(t *testing.T) {
	t.Run("Set creates intermediate objects", func(t *testing.T) {
		e := ExtraData{}
		e.Set("shopware.plugin-class", "Acme\\Plugin")
		s, ok := e.GetString("shopware.plugin-class")
		assert.True(t, ok)
		assert.Equal(t, "Acme\\Plugin", s)
	})

	t.Run("Set overwrites a non-object intermediate", func(t *testing.T) {
		e := ExtraData{"shopware": "scalar"}
		e.Set("shopware.foo", "bar")
		s, ok := e.GetString("shopware.foo")
		assert.True(t, ok)
		assert.Equal(t, "bar", s)
	})

	t.Run("Unset removes a key", func(t *testing.T) {
		e := ExtraData{}
		e.Set("a.b.c", 1)
		assert.True(t, e.Has("a.b.c"))
		e.Unset("a.b.c")
		assert.False(t, e.Has("a.b.c"))
		// Parent object remains.
		assert.True(t, e.Has("a.b"))
	})

	t.Run("Unset missing path is a no-op", func(t *testing.T) {
		e := ExtraData{}
		e.Unset("does.not.exist")
		assert.False(t, e.Has("does"))
	})
}

func TestExtraDataRoundTripThroughComposerJson(t *testing.T) {
	input := `{"name":"a/b","extra":{"shopware":{"plugin-class":"Acme\\Plugin"}}}`
	var c Json
	assert.NoError(t, json.Unmarshal([]byte(input), &c))

	// Typed access works on the parsed Extra.
	s, ok := c.Extra.GetString("shopware.plugin-class")
	assert.True(t, ok)
	assert.Equal(t, "Acme\\Plugin", s)

	// Plain map indexing still works (ExtraData is a map underneath).
	sw, ok := c.Extra["shopware"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "Acme\\Plugin", sw["plugin-class"])

	// Mutate via Set and confirm it serializes back.
	c.Extra.Set("shopware.enabled", true)
	out, err := json.Marshal(c)
	assert.NoError(t, err)
	assert.Contains(t, string(out), `"enabled":true`)
}
