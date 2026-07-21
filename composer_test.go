package composer

import (
	"encoding/json"
	"fmt"
	"github.com/shyim/go-composer/internal/testassert"
	"os"
	"path/filepath"
	"testing"
)

func TestComposerJsonRepositoriesHasRepository(t *testing.T) {
	repos := Repositories{
		{
			Type: "vcs",
			URL:  "https://github.com/shopware/platform",
		},
		{
			Type: "composer",
			URL:  "https://packages.example.org",
		},
	}

	testassert.True(t, repos.HasRepository("https://github.com/shopware/platform"))
	testassert.True(t, repos.HasRepository("https://packages.example.org"))
	testassert.False(t, repos.HasRepository("https://github.com/shopware/core"))
	testassert.False(t, repos.HasRepository(""))
}

func TestComposerJsonHasPackage(t *testing.T) {
	composer := &Json{
		Require: PackageLink{
			"symfony/console": "^5.0",
			"php":             "^7.4 || ^8.0",
		},
		RequireDev: PackageLink{
			"phpunit/phpunit": "^9.5",
		},
	}

	testassert.True(t, composer.HasPackage("symfony/console"))
	testassert.True(t, composer.HasPackage("php"))
	testassert.False(t, composer.HasPackage("phpunit/phpunit"))
	testassert.False(t, composer.HasPackage("not-exists"))
}

func TestComposerJsonHasPackageDev(t *testing.T) {
	composer := &Json{
		Require: PackageLink{
			"symfony/console": "^5.0",
		},
		RequireDev: PackageLink{
			"phpunit/phpunit": "^9.5",
			"mockery/mockery": "^1.4",
		},
	}

	testassert.True(t, composer.HasPackageDev("phpunit/phpunit"))
	testassert.True(t, composer.HasPackageDev("mockery/mockery"))
	testassert.False(t, composer.HasPackageDev("symfony/console"))
	testassert.False(t, composer.HasPackageDev("not-exists"))
}

func TestComposerJsonSave(t *testing.T) {
	tempDir := t.TempDir()
	composerFile := filepath.Join(tempDir, "composer.json")

	composer := &Json{
		path:        composerFile,
		Name:        "shopware/cli",
		Description: "Shopware CLI tool",
		Version:     "1.0.0",
		Type:        "library",
		License:     NewString("MIT"),
		Authors: []Author{
			{
				Name:  "Shopware AG",
				Email: "info@shopware.com",
			},
		},
		Require: PackageLink{
			"php":             "^7.4 || ^8.0",
			"symfony/console": "^5.0",
		},
		RequireDev: PackageLink{
			"phpunit/phpunit": "^9.5",
		},
		Repositories: Repositories{
			{
				Type: "vcs",
				URL:  "https://github.com/shopware/platform",
			},
		},
	}

	err := composer.Save()
	testassert.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(composerFile)
	testassert.NoError(t, err)

	// Read and verify content
	content, err := os.ReadFile(composerFile)
	testassert.NoError(t, err)

	var savedComposer Json
	err = json.Unmarshal(content, &savedComposer)
	testassert.NoError(t, err)

	testassert.Equal(t, composer.Name, savedComposer.Name)
	testassert.Equal(t, composer.Description, savedComposer.Description)
	testassert.Equal(t, composer.Version, savedComposer.Version)
	testassert.Equal(t, composer.Type, savedComposer.Type)
	testassert.Equal(t, composer.License, savedComposer.License)
	testassert.Equal(t, composer.Authors[0].Name, savedComposer.Authors[0].Name)
	testassert.Equal(t, composer.Authors[0].Email, savedComposer.Authors[0].Email)
	testassert.Equal(t, composer.Require["php"], savedComposer.Require["php"])
	testassert.Equal(t, composer.Require["symfony/console"], savedComposer.Require["symfony/console"])
	testassert.Equal(t, composer.RequireDev["phpunit/phpunit"], savedComposer.RequireDev["phpunit/phpunit"])
	testassert.Equal(t, composer.Repositories[0].Type, savedComposer.Repositories[0].Type)
	testassert.Equal(t, composer.Repositories[0].URL, savedComposer.Repositories[0].URL)
}

func TestReadComposerJson(t *testing.T) {
	// Test with valid file
	t.Run("valid file", func(t *testing.T) {
		tempDir := t.TempDir()
		composerFile := filepath.Join(tempDir, "composer.json")

		testComposer := Json{
			Name:        "shopware/cli",
			Description: "Shopware CLI tool",
			Version:     "1.0.0",
			Require: PackageLink{
				"php": "^7.4 || ^8.0",
			},
			Repositories: Repositories{
				{
					Type: "vcs",
					URL:  "https://github.com/shopware/platform",
				},
			},
		}

		content, err := json.MarshalIndent(testComposer, "", "  ")
		testassert.NoError(t, err)
		err = os.WriteFile(composerFile, content, 0o644)
		testassert.NoError(t, err)

		composer, err := ReadJson(composerFile)
		testassert.NoError(t, err)
		testassert.Equal(t, composerFile, composer.path)
		testassert.Equal(t, "shopware/cli", composer.Name)
		testassert.Equal(t, "Shopware CLI tool", composer.Description)
		testassert.Equal(t, "1.0.0", composer.Version)
		testassert.Equal(t, "^7.4 || ^8.0", composer.Require["php"])
		testassert.Equal(t, "vcs", composer.Repositories[0].Type)
		testassert.Equal(t, "https://github.com/shopware/platform", composer.Repositories[0].URL)
	})

	// Test with non-existing file
	t.Run("non-existing file", func(t *testing.T) {
		tempDir := t.TempDir()
		composerFile := filepath.Join(tempDir, "nonexistent.json")

		composer, err := ReadJson(composerFile)
		testassert.Error(t, err)
		testassert.Nil(t, composer)
	})

	// Test with invalid JSON
	t.Run("invalid JSON", func(t *testing.T) {
		tempDir := t.TempDir()
		composerFile := filepath.Join(tempDir, "invalid.json")

		err := os.WriteFile(composerFile, []byte("{invalid json}"), 0o644)
		testassert.NoError(t, err)

		composer, err := ReadJson(composerFile)
		testassert.Error(t, err)
		testassert.Nil(t, composer)
	})
}

func TestReadComposerJsonDifferentRepositoryWritings(t *testing.T) {
	t.Run("repository list", func(t *testing.T) {
		tempDir := t.TempDir()
		composerFile := filepath.Join(tempDir, "composer.json")

		content := `
{
	"repositories": [
		{
			"type": "vcs",
			"url": "https://github.com/shopware/platform"
		},
		{
			"type": "path",
			"url": "custom/plugins"
		}
	]
}
`
		err := os.WriteFile(composerFile, []byte(content), 0o644)
		testassert.NoError(t, err)

		composer, err := ReadJson(composerFile)
		testassert.NoError(t, err)
		testassert.Equal(t, composerFile, composer.path)

		expectedRepos := []Repository{
			{Type: "vcs", URL: "https://github.com/shopware/platform"},
			{Type: "path", URL: "custom/plugins"},
		}
		testassert.ElementsMatch(t, expectedRepos, composer.Repositories)
	})

	t.Run("repository map", func(t *testing.T) {
		tempDir := t.TempDir()
		composerFile := filepath.Join(tempDir, "composer.json")

		content := `
{
	"repositories": {
		"remote": {
			"type": "vcs",
			"url": "https://github.com/shopware/platform"
		},
		"local": {
			"type": "path",
			"url": "custom/plugins"
		}
	}
}
`
		err := os.WriteFile(composerFile, []byte(content), 0o644)
		testassert.NoError(t, err)

		composer, err := ReadJson(composerFile)
		testassert.NoError(t, err)
		testassert.Equal(t, composerFile, composer.path)

		expectedRepos := []Repository{
			{Type: "vcs", URL: "https://github.com/shopware/platform"},
			{Type: "path", URL: "custom/plugins"},
		}
		testassert.ElementsMatch(t, expectedRepos, composer.Repositories)
	})
}

func TestComposerJsonPreservesUnknownFields(t *testing.T) {
	t.Run("unknown top-level keys round-trip", func(t *testing.T) {
		input := `{
			"name": "vendor/pkg",
			"require": {"php": "^8.2"},
			"future-key": {"nested": "value"},
			"some-flag": true
		}`

		var composer Json
		err := json.Unmarshal([]byte(input), &composer)
		testassert.NoError(t, err)

		// Known fields decode normally.
		testassert.Equal(t, "vendor/pkg", composer.Name)
		testassert.Equal(t, "^8.2", composer.Require["php"])

		// Unknown keys are captured.
		testassert.Contains(t, composer.AdditionalFields, "future-key")
		testassert.Contains(t, composer.AdditionalFields, "some-flag")
		testassert.NotContains(t, composer.AdditionalFields, "name")
		testassert.NotContains(t, composer.AdditionalFields, "require")

		// Marshalling re-emits the unknown keys unchanged alongside the known
		// fields. (Byte-for-byte equality is not asserted: the base struct
		// always emits empty autoload/autoload-dev objects.)
		out, err := json.Marshal(composer)
		testassert.NoError(t, err)

		var got map[string]json.RawMessage
		testassert.NoError(t, json.Unmarshal(out, &got))
		testassert.JSONEq(t, `{"nested": "value"}`, string(got["future-key"]))
		testassert.JSONEq(t, `true`, string(got["some-flag"]))
		testassert.JSONEq(t, `"vendor/pkg"`, string(got["name"]))
		testassert.JSONEq(t, `{"php": "^8.2"}`, string(got["require"]))
	})

	t.Run("modeled extra section is not treated as unknown", func(t *testing.T) {
		input := `{"name": "vendor/pkg", "extra": {"branch-alias": {"dev-main": "1.x-dev"}}}`

		var composer Json
		err := json.Unmarshal([]byte(input), &composer)
		testassert.NoError(t, err)

		// "extra" maps to the modeled Extra field, not AdditionalFields.
		testassert.Nil(t, composer.AdditionalFields)
		testassert.Contains(t, composer.Extra, "branch-alias")
	})

	t.Run("no unknown keys leaves AdditionalFields nil", func(t *testing.T) {
		var composer Json
		err := json.Unmarshal([]byte(`{"name": "vendor/pkg"}`), &composer)
		testassert.NoError(t, err)
		testassert.Nil(t, composer.AdditionalFields)
	})

	t.Run("unknown keys survive read/save to disk", func(t *testing.T) {
		tempDir := t.TempDir()
		composerFile := filepath.Join(tempDir, "composer.json")

		input := `{"name":"vendor/pkg","future-key":{"nested":"value"}}`
		err := os.WriteFile(composerFile, []byte(input), 0o644)
		testassert.NoError(t, err)

		composer, err := ReadJson(composerFile)
		testassert.NoError(t, err)
		testassert.Contains(t, composer.AdditionalFields, "future-key")

		err = composer.Save()
		testassert.NoError(t, err)

		written, err := os.ReadFile(composerFile)
		testassert.NoError(t, err)

		var roundTripped Json
		err = json.Unmarshal(written, &roundTripped)
		testassert.NoError(t, err)
		testassert.Equal(t, "vendor/pkg", roundTripped.Name)
		testassert.Contains(t, roundTripped.AdditionalFields, "future-key")
		testassert.JSONEq(t, `{"nested":"value"}`, string(roundTripped.AdditionalFields["future-key"]))
	})
}

func TestStringOrSliceRoundTrip(t *testing.T) {
	t.Run("license as single string stays scalar", func(t *testing.T) {
		var c Json
		err := json.Unmarshal([]byte(`{"name":"a/b","license":"MIT"}`), &c)
		testassert.NoError(t, err)
		testassert.Equal(t, []string{"MIT"}, c.License.Strings())
		testassert.Equal(t, "MIT", c.License.First())

		out, err := json.Marshal(c)
		testassert.NoError(t, err)
		testassert.Contains(t, string(out), `"license":"MIT"`)
	})

	t.Run("license as array stays array", func(t *testing.T) {
		var c Json
		err := json.Unmarshal([]byte(`{"name":"a/b","license":["MIT","Apache-2.0"]}`), &c)
		testassert.NoError(t, err)
		testassert.Equal(t, []string{"MIT", "Apache-2.0"}, c.License.Strings())

		out, err := json.Marshal(c)
		testassert.NoError(t, err)
		testassert.Contains(t, string(out), `"license":["MIT","Apache-2.0"]`)
	})

	t.Run("bin accepts single string", func(t *testing.T) {
		var c Json
		err := json.Unmarshal([]byte(`{"name":"a/b","bin":"bin/foo"}`), &c)
		testassert.NoError(t, err)
		testassert.Equal(t, []string{"bin/foo"}, c.Bin.Strings())

		out, err := json.Marshal(c)
		testassert.NoError(t, err)
		testassert.Contains(t, string(out), `"bin":"bin/foo"`)
	})

	t.Run("bin accepts array", func(t *testing.T) {
		var c Json
		err := json.Unmarshal([]byte(`{"name":"a/b","bin":["bin/a","bin/b"]}`), &c)
		testassert.NoError(t, err)
		testassert.Equal(t, []string{"bin/a", "bin/b"}, c.Bin.Strings())
	})

	t.Run("constructors set shape", func(t *testing.T) {
		single, err := json.Marshal(NewString("MIT"))
		testassert.NoError(t, err)
		testassert.JSONEq(t, `"MIT"`, string(single))

		multi, err := json.Marshal(NewStrings("MIT", "GPL-3.0"))
		testassert.NoError(t, err)
		testassert.JSONEq(t, `["MIT","GPL-3.0"]`, string(multi))
	})
}

func TestBoolOrStringRoundTrip(t *testing.T) {
	t.Run("abandoned true", func(t *testing.T) {
		var c Json
		err := json.Unmarshal([]byte(`{"name":"a/b","abandoned":true}`), &c)
		testassert.NoError(t, err)
		testassert.True(t, c.Abandoned.IsAbandoned())
		testassert.Equal(t, "", c.Abandoned.Replacement())

		out, err := json.Marshal(c)
		testassert.NoError(t, err)
		testassert.Contains(t, string(out), `"abandoned":true`)
	})

	t.Run("abandoned with replacement", func(t *testing.T) {
		var c Json
		err := json.Unmarshal([]byte(`{"name":"a/b","abandoned":"vendor/new"}`), &c)
		testassert.NoError(t, err)
		testassert.True(t, c.Abandoned.IsAbandoned())
		testassert.Equal(t, "vendor/new", c.Abandoned.Replacement())

		out, err := json.Marshal(c)
		testassert.NoError(t, err)
		testassert.Contains(t, string(out), `"abandoned":"vendor/new"`)
	})

	t.Run("nil receiver is not abandoned", func(t *testing.T) {
		var c Json
		testassert.False(t, c.Abandoned.IsAbandoned())
		testassert.Equal(t, "", c.Abandoned.Replacement())
	})

	t.Run("constructors", func(t *testing.T) {
		b, err := json.Marshal(NewAbandonedBool(true))
		testassert.NoError(t, err)
		testassert.JSONEq(t, `true`, string(b))

		s, err := json.Marshal(NewAbandonedReplacement("vendor/new"))
		testassert.NoError(t, err)
		testassert.JSONEq(t, `"vendor/new"`, string(s))
	})
}

func TestComposerJsonNewTypedFields(t *testing.T) {
	input := `{
		"name": "a/b",
		"source": {"type": "git", "url": "https://example.com/a.git", "reference": "abc"},
		"dist": {"type": "zip", "url": "https://example.com/a.zip", "shasum": "deadbeef"},
		"archive": {"name": "a-archive", "exclude": ["/tests"]},
		"include-path": ["lib/"],
		"target-dir": "Acme/Foo",
		"default-branch": true,
		"php-ext": {"extension-name": "ext-foo"},
		"scripts-descriptions": {"test": "Runs tests"},
		"scripts-aliases": {"test": ["phpunit"]}
	}`

	var c Json
	err := json.Unmarshal([]byte(input), &c)
	testassert.NoError(t, err)

	testassert.Equal(t, "git", c.Source.Type)
	testassert.Equal(t, "https://example.com/a.git", c.Source.URL)
	testassert.Equal(t, "deadbeef", c.Dist.Shasum)
	testassert.Equal(t, "a-archive", c.Archive.Name)
	testassert.Equal(t, []string{"/tests"}, c.Archive.Exclude)
	testassert.Equal(t, []string{"lib/"}, c.IncludePath)
	testassert.Equal(t, "Acme/Foo", c.TargetDir)
	testassert.NotNil(t, c.DefaultBranch)
	testassert.True(t, *c.DefaultBranch)
	testassert.Equal(t, "ext-foo", c.PHPExt["extension-name"])
	testassert.Equal(t, "Runs tests", c.ScriptsDescriptions["test"])
	testassert.Equal(t, []string{"phpunit"}, c.ScriptsAliases["test"])

	// None of these typed keys should leak into AdditionalFields.
	testassert.Nil(t, c.AdditionalFields)
}

func TestComposerJsonManipulationVerbs(t *testing.T) {
	c := &Json{}

	c.AddPackage("symfony/console", "^6.0")
	testassert.True(t, c.HasPackage("symfony/console"))
	c.RemovePackage("symfony/console")
	testassert.False(t, c.HasPackage("symfony/console"))

	c.AddPackageDev("phpunit/phpunit", "^10.0")
	testassert.True(t, c.HasPackageDev("phpunit/phpunit"))
	c.RemovePackageDev("phpunit/phpunit")
	testassert.False(t, c.HasPackageDev("phpunit/phpunit"))

	// EnsurePackage adds only when missing from both require and require-dev.
	testassert.True(t, c.EnsurePackage("symfony/http-foundation", "^6.0"))
	testassert.True(t, c.HasPackage("symfony/http-foundation"))
	testassert.Equal(t, "^6.0", c.Require["symfony/http-foundation"])
	// Already in require: no-op, keeps existing constraint.
	testassert.False(t, c.EnsurePackage("symfony/http-foundation", "^7.0"))
	testassert.Equal(t, "^6.0", c.Require["symfony/http-foundation"])
	// Present in require-dev: also a no-op for EnsurePackage.
	c.AddPackageDev("friendsofphp/php-cs-fixer", "^3.0")
	testassert.False(t, c.EnsurePackage("friendsofphp/php-cs-fixer", "^4.0"))
	testassert.False(t, c.HasPackage("friendsofphp/php-cs-fixer"))
	testassert.Equal(t, "^3.0", c.RequireDev["friendsofphp/php-cs-fixer"])

	// EnsurePackageDev adds only when missing from both require and require-dev.
	testassert.True(t, c.EnsurePackageDev("phpunit/phpunit", "^10.0"))
	testassert.True(t, c.HasPackageDev("phpunit/phpunit"))
	testassert.Equal(t, "^10.0", c.RequireDev["phpunit/phpunit"])
	// Already in require-dev: no-op.
	testassert.False(t, c.EnsurePackageDev("phpunit/phpunit", "^11.0"))
	testassert.Equal(t, "^10.0", c.RequireDev["phpunit/phpunit"])
	// Present in require: also a no-op for EnsurePackageDev.
	testassert.False(t, c.EnsurePackageDev("symfony/http-foundation", "^7.0"))
	testassert.False(t, c.HasPackageDev("symfony/http-foundation"))
	testassert.Equal(t, "^6.0", c.Require["symfony/http-foundation"])

	repo := Repository{Type: "vcs", URL: "https://example.com/r.git"}
	c.AddRepository(repo)
	testassert.True(t, c.Repositories.HasRepository("https://example.com/r.git"))
	// Adding the same URL again is a no-op.
	c.AddRepository(repo)
	testassert.Len(t, c.Repositories, 1)
	c.RemoveRepository("https://example.com/r.git")
	testassert.False(t, c.Repositories.HasRepository("https://example.com/r.git"))

	c.SetConfig("sort-packages", true)
	testassert.True(t, c.HasConfig("sort-packages"))
	c.RemoveConfig("sort-packages")
	testassert.False(t, c.HasConfig("sort-packages"))
}

func TestComposerJsonComposerPluginVerbs(t *testing.T) {
	c := &Json{}

	// EnableComposerPlugin creates the allow-plugins map when missing.
	c.EnableComposerPlugin("phpstan/extension-installer")
	allowedPlugins, ok := c.Config["allow-plugins"].(map[string]any)
	testassert.True(t, ok)
	testassert.Equal(t, true, allowedPlugins["phpstan/extension-installer"])

	// EnableComposerPlugin keeps existing entries and appends.
	c.EnableComposerPlugin("dealerdirect/phpstan/plugin")
	allowedPlugins = c.Config["allow-plugins"].(map[string]any)
	testassert.Equal(t, true, allowedPlugins["phpstan/extension-installer"])
	testassert.Equal(t, true, allowedPlugins["dealerdirect/phpstan/plugin"])

	// DisableComposerPlugin marks an enabled plugin as disallowed.
	c.DisableComposerPlugin("phpstan/extension-installer")
	allowedPlugins = c.Config["allow-plugins"].(map[string]any)
	testassert.Equal(t, false, allowedPlugins["phpstan/extension-installer"])
	// Other entries are untouched.
	testassert.Equal(t, true, allowedPlugins["dealerdirect/phpstan/plugin"])

	// DisableComposerPlugin creates the allow-plugins map when missing.
	c2 := &Json{}
	c2.DisableComposerPlugin("phpstan/extension-installer")
	allowedPlugins, ok = c2.Config["allow-plugins"].(map[string]any)
	testassert.True(t, ok)
	testassert.Equal(t, false, allowedPlugins["phpstan/extension-installer"])

	// Re-enabling a disabled plugin flips it back to true.
	c.DisableComposerPlugin("dealerdirect/phpstan/plugin")
	c.EnableComposerPlugin("dealerdirect/phpstan/plugin")
	allowedPlugins = c.Config["allow-plugins"].(map[string]any)
	testassert.Equal(t, true, allowedPlugins["dealerdirect/phpstan/plugin"])

	// RemoveComposerPlugin deletes the entry entirely.
	c.RemoveComposerPlugin("phpstan/extension-installer")
	allowedPlugins = c.Config["allow-plugins"].(map[string]any)
	_, ok = allowedPlugins["phpstan/extension-installer"]
	testassert.False(t, ok)
}

func TestComposerJsonPreservesKeyOrder(t *testing.T) {
	t.Run("top-level order preserved across round-trip", func(t *testing.T) {
		input := `{"type":"library","name":"a/b","require":{"php":"^8.2"},"description":"x"}`
		var c Json
		err := json.Unmarshal([]byte(input), &c)
		testassert.NoError(t, err)

		out, err := json.Marshal(c)
		testassert.NoError(t, err)
		testassert.Equal(t, input, string(out))
	})

	t.Run("no empty autoload emitted when absent", func(t *testing.T) {
		var c Json
		err := json.Unmarshal([]byte(`{"name":"a/b"}`), &c)
		testassert.NoError(t, err)
		out, err := json.Marshal(c)
		testassert.NoError(t, err)
		testassert.NotContains(t, string(out), "autoload")
	})

	t.Run("newly added keys appended deterministically", func(t *testing.T) {
		input := `{"name":"a/b"}`
		var c Json
		err := json.Unmarshal([]byte(input), &c)
		testassert.NoError(t, err)
		c.AddPackage("php", "^8.2")
		c.Description = "added"

		out, err := json.Marshal(c)
		testassert.NoError(t, err)
		// name stays first (original order); new keys appended in sorted order.
		testassert.True(t, len(string(out)) > len(input))
		testassert.Contains(t, string(out), `"name":"a/b"`)
		testassert.Contains(t, string(out), `"description":"added"`)
		testassert.Contains(t, string(out), `"require":{"php":"^8.2"}`)
	})

	t.Run("order survives save to disk", func(t *testing.T) {
		dir := t.TempDir()
		file := dir + "/composer.json"
		input := "{\n  \"type\": \"library\",\n  \"name\": \"a/b\"\n}"
		err := os.WriteFile(file, []byte(input), 0o644)
		testassert.NoError(t, err)

		c, err := ReadJson(file)
		testassert.NoError(t, err)
		err = c.Save()
		testassert.NoError(t, err)

		written, err := os.ReadFile(file)
		testassert.NoError(t, err)
		// "type" must still precede "name".
		typeIdx := indexOf(string(written), `"type"`)
		nameIdx := indexOf(string(written), `"name"`)
		testassert.True(t, typeIdx >= 0 && nameIdx >= 0 && typeIdx < nameIdx, fmt.Sprintf("type should precede name; got: %s", written))
	})
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func TestComposerJsonExplicitFalseBoolsSurvive(t *testing.T) {
	// Regression: prefer-stable/default-branch are tri-state. An explicit false
	// must survive a read/modify/write round-trip rather than being dropped by
	// omitempty (which omits the bool zero value).
	t.Run("prefer-stable false survives", func(t *testing.T) {
		var c Json
		err := json.Unmarshal([]byte(`{"name":"a/b","prefer-stable":false}`), &c)
		testassert.NoError(t, err)
		testassert.NotNil(t, c.PreferStable)
		testassert.False(t, *c.PreferStable)

		out, err := json.Marshal(c)
		testassert.NoError(t, err)
		testassert.Contains(t, string(out), `"prefer-stable":false`)
	})

	t.Run("default-branch false survives", func(t *testing.T) {
		var c Json
		err := json.Unmarshal([]byte(`{"name":"a/b","default-branch":false}`), &c)
		testassert.NoError(t, err)
		testassert.NotNil(t, c.DefaultBranch)
		testassert.False(t, *c.DefaultBranch)

		out, err := json.Marshal(c)
		testassert.NoError(t, err)
		testassert.Contains(t, string(out), `"default-branch":false`)
	})

	t.Run("absent stays absent", func(t *testing.T) {
		var c Json
		err := json.Unmarshal([]byte(`{"name":"a/b"}`), &c)
		testassert.NoError(t, err)
		testassert.Nil(t, c.PreferStable)
		testassert.Nil(t, c.DefaultBranch)

		out, err := json.Marshal(c)
		testassert.NoError(t, err)
		testassert.NotContains(t, string(out), "prefer-stable")
		testassert.NotContains(t, string(out), "default-branch")
	})

	t.Run("true survives via Bool helper", func(t *testing.T) {
		c := Json{Name: "a/b", PreferStable: Bool(true)}
		out, err := json.Marshal(c)
		testassert.NoError(t, err)
		testassert.Contains(t, string(out), `"prefer-stable":true`)
	})
}
