package composer

import (
	"encoding/json"
	"github.com/shyim/go-composer/internal/testassert"
	"testing"
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
	testassert.NoError(t, json.Unmarshal([]byte(raw), &e))

	t.Run("Get and Has by path", func(t *testing.T) {
		v, ok := e.Get("shopware.plugin-class")
		testassert.True(t, ok)
		testassert.Equal(t, "Acme\\Plugin", v)
		testassert.True(t, e.Has("shopware.enabled"))
		testassert.False(t, e.Has("shopware.missing"))
		testassert.False(t, e.Has("missing.deep.path"))
	})

	t.Run("GetString", func(t *testing.T) {
		s, ok := e.GetString("shopware.plugin-class")
		testassert.True(t, ok)
		testassert.Equal(t, "Acme\\Plugin", s)

		_, ok = e.GetString("shopware.enabled") // not a string
		testassert.False(t, ok)

		_, ok = e.GetString("shopware.missing")
		testassert.False(t, ok)
	})

	t.Run("GetBool", func(t *testing.T) {
		b, ok := e.GetBool("shopware.enabled")
		testassert.True(t, ok)
		testassert.True(t, b)

		_, ok = e.GetBool("shopware.plugin-class")
		testassert.False(t, ok)
	})

	t.Run("GetInt accepts JSON whole numbers", func(t *testing.T) {
		n, ok := e.GetInt("shopware.priority")
		testassert.True(t, ok)
		testassert.Equal(t, int64(10), n)

		// 0.5 is not integral.
		_, ok = e.GetInt("shopware.ratio")
		testassert.False(t, ok)
	})

	t.Run("GetFloat", func(t *testing.T) {
		f, ok := e.GetFloat("shopware.ratio")
		testassert.True(t, ok)
		testassert.Equal(t, 0.5, f)

		// Integers are also numbers.
		f, ok = e.GetFloat("shopware.priority")
		testassert.True(t, ok)
		testassert.Equal(t, 10.0, f)
	})

	t.Run("GetMap", func(t *testing.T) {
		m, ok := e.GetMap("shopware.label")
		testassert.True(t, ok)
		testassert.Equal(t, "Hallo", m["de-DE"])

		_, ok = e.GetMap("shopware.bundles") // a slice, not a map
		testassert.False(t, ok)
	})

	t.Run("GetStringSlice", func(t *testing.T) {
		s, ok := e.GetStringSlice("shopware.bundles")
		testassert.True(t, ok)
		testassert.Equal(t, []string{"a", "b"}, s)

		// label is an object, not a slice.
		_, ok = e.GetStringSlice("shopware.label")
		testassert.False(t, ok)
	})

	t.Run("nested string from object path", func(t *testing.T) {
		s, ok := e.GetString("shopware.label.de-DE")
		testassert.True(t, ok)
		testassert.Equal(t, "Hallo", s)
	})

	t.Run("empty path", func(t *testing.T) {
		_, ok := e.Get("")
		testassert.False(t, ok)
	})
}

func TestExtraDataSetUnset(t *testing.T) {
	t.Run("Set creates intermediate objects", func(t *testing.T) {
		e := ExtraData{}
		e.Set("shopware.plugin-class", "Acme\\Plugin")
		s, ok := e.GetString("shopware.plugin-class")
		testassert.True(t, ok)
		testassert.Equal(t, "Acme\\Plugin", s)
	})

	t.Run("Set overwrites a non-object intermediate", func(t *testing.T) {
		e := ExtraData{"shopware": "scalar"}
		e.Set("shopware.foo", "bar")
		s, ok := e.GetString("shopware.foo")
		testassert.True(t, ok)
		testassert.Equal(t, "bar", s)
	})

	t.Run("Unset removes a key", func(t *testing.T) {
		e := ExtraData{}
		e.Set("a.b.c", 1)
		testassert.True(t, e.Has("a.b.c"))
		e.Unset("a.b.c")
		testassert.False(t, e.Has("a.b.c"))
		// Parent object remains.
		testassert.True(t, e.Has("a.b"))
	})

	t.Run("Unset missing path is a no-op", func(t *testing.T) {
		e := ExtraData{}
		e.Unset("does.not.exist")
		testassert.False(t, e.Has("does"))
	})
}

func TestExtraDataRoundTripThroughComposerJson(t *testing.T) {
	input := `{"name":"a/b","extra":{"shopware":{"plugin-class":"Acme\\Plugin"}}}`
	var c Json
	testassert.NoError(t, json.Unmarshal([]byte(input), &c))

	// Typed access works on the parsed Extra.
	s, ok := c.Extra.GetString("shopware.plugin-class")
	testassert.True(t, ok)
	testassert.Equal(t, "Acme\\Plugin", s)

	// Plain map indexing still works (ExtraData is a map underneath).
	sw, ok := c.Extra["shopware"].(map[string]any)
	testassert.True(t, ok)
	testassert.Equal(t, "Acme\\Plugin", sw["plugin-class"])

	// Mutate via Set and confirm it serializes back.
	c.Extra.Set("shopware.enabled", true)
	out, err := json.Marshal(c)
	testassert.NoError(t, err)
	testassert.Contains(t, string(out), `"enabled":true`)
}
