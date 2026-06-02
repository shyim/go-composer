package packagist

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComposerJsonRepositoriesHasRepository(t *testing.T) {
	repos := ComposerJsonRepositories{
		{
			Type: "vcs",
			URL:  "https://github.com/shopware/platform",
		},
		{
			Type: "composer",
			URL:  "https://packages.example.org",
		},
	}

	assert.True(t, repos.HasRepository("https://github.com/shopware/platform"))
	assert.True(t, repos.HasRepository("https://packages.example.org"))
	assert.False(t, repos.HasRepository("https://github.com/shopware/core"))
	assert.False(t, repos.HasRepository(""))
}

func TestComposerJsonHasPackage(t *testing.T) {
	composer := &ComposerJson{
		Require: ComposerPackageLink{
			"symfony/console": "^5.0",
			"php":             "^7.4 || ^8.0",
		},
		RequireDev: ComposerPackageLink{
			"phpunit/phpunit": "^9.5",
		},
	}

	assert.True(t, composer.HasPackage("symfony/console"))
	assert.True(t, composer.HasPackage("php"))
	assert.False(t, composer.HasPackage("phpunit/phpunit"))
	assert.False(t, composer.HasPackage("not-exists"))
}

func TestComposerJsonHasPackageDev(t *testing.T) {
	composer := &ComposerJson{
		Require: ComposerPackageLink{
			"symfony/console": "^5.0",
		},
		RequireDev: ComposerPackageLink{
			"phpunit/phpunit": "^9.5",
			"mockery/mockery": "^1.4",
		},
	}

	assert.True(t, composer.HasPackageDev("phpunit/phpunit"))
	assert.True(t, composer.HasPackageDev("mockery/mockery"))
	assert.False(t, composer.HasPackageDev("symfony/console"))
	assert.False(t, composer.HasPackageDev("not-exists"))
}

func TestComposerJsonSave(t *testing.T) {
	tempDir := t.TempDir()
	composerFile := filepath.Join(tempDir, "composer.json")

	composer := &ComposerJson{
		path:        composerFile,
		Name:        "shopware/cli",
		Description: "Shopware CLI tool",
		Version:     "1.0.0",
		Type:        "library",
		License:     NewString("MIT"),
		Authors: []ComposerJsonAuthor{
			{
				Name:  "Shopware AG",
				Email: "info@shopware.com",
			},
		},
		Require: ComposerPackageLink{
			"php":             "^7.4 || ^8.0",
			"symfony/console": "^5.0",
		},
		RequireDev: ComposerPackageLink{
			"phpunit/phpunit": "^9.5",
		},
		Repositories: ComposerJsonRepositories{
			{
				Type: "vcs",
				URL:  "https://github.com/shopware/platform",
			},
		},
	}

	err := composer.Save()
	assert.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(composerFile)
	assert.NoError(t, err)

	// Read and verify content
	content, err := os.ReadFile(composerFile)
	assert.NoError(t, err)

	var savedComposer ComposerJson
	err = json.Unmarshal(content, &savedComposer)
	assert.NoError(t, err)

	assert.Equal(t, composer.Name, savedComposer.Name)
	assert.Equal(t, composer.Description, savedComposer.Description)
	assert.Equal(t, composer.Version, savedComposer.Version)
	assert.Equal(t, composer.Type, savedComposer.Type)
	assert.Equal(t, composer.License, savedComposer.License)
	assert.Equal(t, composer.Authors[0].Name, savedComposer.Authors[0].Name)
	assert.Equal(t, composer.Authors[0].Email, savedComposer.Authors[0].Email)
	assert.Equal(t, composer.Require["php"], savedComposer.Require["php"])
	assert.Equal(t, composer.Require["symfony/console"], savedComposer.Require["symfony/console"])
	assert.Equal(t, composer.RequireDev["phpunit/phpunit"], savedComposer.RequireDev["phpunit/phpunit"])
	assert.Equal(t, composer.Repositories[0].Type, savedComposer.Repositories[0].Type)
	assert.Equal(t, composer.Repositories[0].URL, savedComposer.Repositories[0].URL)
}

func TestReadComposerJson(t *testing.T) {
	// Test with valid file
	t.Run("valid file", func(t *testing.T) {
		tempDir := t.TempDir()
		composerFile := filepath.Join(tempDir, "composer.json")

		testComposer := ComposerJson{
			Name:        "shopware/cli",
			Description: "Shopware CLI tool",
			Version:     "1.0.0",
			Require: ComposerPackageLink{
				"php": "^7.4 || ^8.0",
			},
			Repositories: ComposerJsonRepositories{
				{
					Type: "vcs",
					URL:  "https://github.com/shopware/platform",
				},
			},
		}

		content, err := json.MarshalIndent(testComposer, "", "  ")
		assert.NoError(t, err)
		err = os.WriteFile(composerFile, content, 0o644)
		assert.NoError(t, err)

		composer, err := ReadComposerJson(composerFile)
		assert.NoError(t, err)
		assert.Equal(t, composerFile, composer.path)
		assert.Equal(t, "shopware/cli", composer.Name)
		assert.Equal(t, "Shopware CLI tool", composer.Description)
		assert.Equal(t, "1.0.0", composer.Version)
		assert.Equal(t, "^7.4 || ^8.0", composer.Require["php"])
		assert.Equal(t, "vcs", composer.Repositories[0].Type)
		assert.Equal(t, "https://github.com/shopware/platform", composer.Repositories[0].URL)
	})

	// Test with non-existing file
	t.Run("non-existing file", func(t *testing.T) {
		tempDir := t.TempDir()
		composerFile := filepath.Join(tempDir, "nonexistent.json")

		composer, err := ReadComposerJson(composerFile)
		assert.Error(t, err)
		assert.Nil(t, composer)
	})

	// Test with invalid JSON
	t.Run("invalid JSON", func(t *testing.T) {
		tempDir := t.TempDir()
		composerFile := filepath.Join(tempDir, "invalid.json")

		err := os.WriteFile(composerFile, []byte("{invalid json}"), 0o644)
		assert.NoError(t, err)

		composer, err := ReadComposerJson(composerFile)
		assert.Error(t, err)
		assert.Nil(t, composer)
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
		assert.NoError(t, err)

		composer, err := ReadComposerJson(composerFile)
		assert.NoError(t, err)
		assert.Equal(t, composerFile, composer.path)

		expectedRepos := []ComposerJsonRepository{
			{Type: "vcs", URL: "https://github.com/shopware/platform"},
			{Type: "path", URL: "custom/plugins"},
		}
		assert.ElementsMatch(t, expectedRepos, composer.Repositories)
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
		assert.NoError(t, err)

		composer, err := ReadComposerJson(composerFile)
		assert.NoError(t, err)
		assert.Equal(t, composerFile, composer.path)

		expectedRepos := []ComposerJsonRepository{
			{Type: "vcs", URL: "https://github.com/shopware/platform"},
			{Type: "path", URL: "custom/plugins"},
		}
		assert.ElementsMatch(t, expectedRepos, composer.Repositories)
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

		var composer ComposerJson
		err := json.Unmarshal([]byte(input), &composer)
		assert.NoError(t, err)

		// Known fields decode normally.
		assert.Equal(t, "vendor/pkg", composer.Name)
		assert.Equal(t, "^8.2", composer.Require["php"])

		// Unknown keys are captured.
		assert.Contains(t, composer.AdditionalFields, "future-key")
		assert.Contains(t, composer.AdditionalFields, "some-flag")
		assert.NotContains(t, composer.AdditionalFields, "name")
		assert.NotContains(t, composer.AdditionalFields, "require")

		// Marshalling re-emits the unknown keys unchanged alongside the known
		// fields. (Byte-for-byte equality is not asserted: the base struct
		// always emits empty autoload/autoload-dev objects.)
		out, err := json.Marshal(composer)
		assert.NoError(t, err)

		var got map[string]json.RawMessage
		assert.NoError(t, json.Unmarshal(out, &got))
		assert.JSONEq(t, `{"nested": "value"}`, string(got["future-key"]))
		assert.JSONEq(t, `true`, string(got["some-flag"]))
		assert.JSONEq(t, `"vendor/pkg"`, string(got["name"]))
		assert.JSONEq(t, `{"php": "^8.2"}`, string(got["require"]))
	})

	t.Run("modeled extra section is not treated as unknown", func(t *testing.T) {
		input := `{"name": "vendor/pkg", "extra": {"branch-alias": {"dev-main": "1.x-dev"}}}`

		var composer ComposerJson
		err := json.Unmarshal([]byte(input), &composer)
		assert.NoError(t, err)

		// "extra" maps to the modeled Extra field, not AdditionalFields.
		assert.Nil(t, composer.AdditionalFields)
		assert.Contains(t, composer.Extra, "branch-alias")
	})

	t.Run("no unknown keys leaves AdditionalFields nil", func(t *testing.T) {
		var composer ComposerJson
		err := json.Unmarshal([]byte(`{"name": "vendor/pkg"}`), &composer)
		assert.NoError(t, err)
		assert.Nil(t, composer.AdditionalFields)
	})

	t.Run("unknown keys survive read/save to disk", func(t *testing.T) {
		tempDir := t.TempDir()
		composerFile := filepath.Join(tempDir, "composer.json")

		input := `{"name":"vendor/pkg","future-key":{"nested":"value"}}`
		err := os.WriteFile(composerFile, []byte(input), 0o644)
		assert.NoError(t, err)

		composer, err := ReadComposerJson(composerFile)
		assert.NoError(t, err)
		assert.Contains(t, composer.AdditionalFields, "future-key")

		err = composer.Save()
		assert.NoError(t, err)

		written, err := os.ReadFile(composerFile)
		assert.NoError(t, err)

		var roundTripped ComposerJson
		err = json.Unmarshal(written, &roundTripped)
		assert.NoError(t, err)
		assert.Equal(t, "vendor/pkg", roundTripped.Name)
		assert.Contains(t, roundTripped.AdditionalFields, "future-key")
		assert.JSONEq(t, `{"nested":"value"}`, string(roundTripped.AdditionalFields["future-key"]))
	})
}

func TestStringOrSliceRoundTrip(t *testing.T) {
	t.Run("license as single string stays scalar", func(t *testing.T) {
		var c ComposerJson
		err := json.Unmarshal([]byte(`{"name":"a/b","license":"MIT"}`), &c)
		assert.NoError(t, err)
		assert.Equal(t, []string{"MIT"}, c.License.Strings())
		assert.Equal(t, "MIT", c.License.First())

		out, err := json.Marshal(c)
		assert.NoError(t, err)
		assert.Contains(t, string(out), `"license":"MIT"`)
	})

	t.Run("license as array stays array", func(t *testing.T) {
		var c ComposerJson
		err := json.Unmarshal([]byte(`{"name":"a/b","license":["MIT","Apache-2.0"]}`), &c)
		assert.NoError(t, err)
		assert.Equal(t, []string{"MIT", "Apache-2.0"}, c.License.Strings())

		out, err := json.Marshal(c)
		assert.NoError(t, err)
		assert.Contains(t, string(out), `"license":["MIT","Apache-2.0"]`)
	})

	t.Run("bin accepts single string", func(t *testing.T) {
		var c ComposerJson
		err := json.Unmarshal([]byte(`{"name":"a/b","bin":"bin/foo"}`), &c)
		assert.NoError(t, err)
		assert.Equal(t, []string{"bin/foo"}, c.Bin.Strings())

		out, err := json.Marshal(c)
		assert.NoError(t, err)
		assert.Contains(t, string(out), `"bin":"bin/foo"`)
	})

	t.Run("bin accepts array", func(t *testing.T) {
		var c ComposerJson
		err := json.Unmarshal([]byte(`{"name":"a/b","bin":["bin/a","bin/b"]}`), &c)
		assert.NoError(t, err)
		assert.Equal(t, []string{"bin/a", "bin/b"}, c.Bin.Strings())
	})

	t.Run("constructors set shape", func(t *testing.T) {
		single, err := json.Marshal(NewString("MIT"))
		assert.NoError(t, err)
		assert.JSONEq(t, `"MIT"`, string(single))

		multi, err := json.Marshal(NewStrings("MIT", "GPL-3.0"))
		assert.NoError(t, err)
		assert.JSONEq(t, `["MIT","GPL-3.0"]`, string(multi))
	})
}

func TestBoolOrStringRoundTrip(t *testing.T) {
	t.Run("abandoned true", func(t *testing.T) {
		var c ComposerJson
		err := json.Unmarshal([]byte(`{"name":"a/b","abandoned":true}`), &c)
		assert.NoError(t, err)
		assert.True(t, c.Abandoned.IsAbandoned())
		assert.Equal(t, "", c.Abandoned.Replacement())

		out, err := json.Marshal(c)
		assert.NoError(t, err)
		assert.Contains(t, string(out), `"abandoned":true`)
	})

	t.Run("abandoned with replacement", func(t *testing.T) {
		var c ComposerJson
		err := json.Unmarshal([]byte(`{"name":"a/b","abandoned":"vendor/new"}`), &c)
		assert.NoError(t, err)
		assert.True(t, c.Abandoned.IsAbandoned())
		assert.Equal(t, "vendor/new", c.Abandoned.Replacement())

		out, err := json.Marshal(c)
		assert.NoError(t, err)
		assert.Contains(t, string(out), `"abandoned":"vendor/new"`)
	})

	t.Run("nil receiver is not abandoned", func(t *testing.T) {
		var c ComposerJson
		assert.False(t, c.Abandoned.IsAbandoned())
		assert.Equal(t, "", c.Abandoned.Replacement())
	})

	t.Run("constructors", func(t *testing.T) {
		b, err := json.Marshal(NewAbandonedBool(true))
		assert.NoError(t, err)
		assert.JSONEq(t, `true`, string(b))

		s, err := json.Marshal(NewAbandonedReplacement("vendor/new"))
		assert.NoError(t, err)
		assert.JSONEq(t, `"vendor/new"`, string(s))
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

	var c ComposerJson
	err := json.Unmarshal([]byte(input), &c)
	assert.NoError(t, err)

	assert.Equal(t, "git", c.Source.Type)
	assert.Equal(t, "https://example.com/a.git", c.Source.URL)
	assert.Equal(t, "deadbeef", c.Dist.Shasum)
	assert.Equal(t, "a-archive", c.Archive.Name)
	assert.Equal(t, []string{"/tests"}, c.Archive.Exclude)
	assert.Equal(t, []string{"lib/"}, c.IncludePath)
	assert.Equal(t, "Acme/Foo", c.TargetDir)
	assert.NotNil(t, c.DefaultBranch)
	assert.True(t, *c.DefaultBranch)
	assert.Equal(t, "ext-foo", c.PHPExt["extension-name"])
	assert.Equal(t, "Runs tests", c.ScriptsDescriptions["test"])
	assert.Equal(t, []string{"phpunit"}, c.ScriptsAliases["test"])

	// None of these typed keys should leak into AdditionalFields.
	assert.Nil(t, c.AdditionalFields)
}

func TestComposerJsonManipulationVerbs(t *testing.T) {
	c := &ComposerJson{}

	c.AddPackage("symfony/console", "^6.0")
	assert.True(t, c.HasPackage("symfony/console"))
	c.RemovePackage("symfony/console")
	assert.False(t, c.HasPackage("symfony/console"))

	c.AddPackageDev("phpunit/phpunit", "^10.0")
	assert.True(t, c.HasPackageDev("phpunit/phpunit"))
	c.RemovePackageDev("phpunit/phpunit")
	assert.False(t, c.HasPackageDev("phpunit/phpunit"))

	repo := ComposerJsonRepository{Type: "vcs", URL: "https://example.com/r.git"}
	c.AddRepository(repo)
	assert.True(t, c.Repositories.HasRepository("https://example.com/r.git"))
	// Adding the same URL again is a no-op.
	c.AddRepository(repo)
	assert.Len(t, c.Repositories, 1)
	c.RemoveRepository("https://example.com/r.git")
	assert.False(t, c.Repositories.HasRepository("https://example.com/r.git"))

	c.SetConfig("sort-packages", true)
	assert.True(t, c.HasConfig("sort-packages"))
	c.RemoveConfig("sort-packages")
	assert.False(t, c.HasConfig("sort-packages"))
}

func TestComposerJsonPreservesKeyOrder(t *testing.T) {
	t.Run("top-level order preserved across round-trip", func(t *testing.T) {
		input := `{"type":"library","name":"a/b","require":{"php":"^8.2"},"description":"x"}`
		var c ComposerJson
		err := json.Unmarshal([]byte(input), &c)
		assert.NoError(t, err)

		out, err := json.Marshal(c)
		assert.NoError(t, err)
		assert.Equal(t, input, string(out))
	})

	t.Run("no empty autoload emitted when absent", func(t *testing.T) {
		var c ComposerJson
		err := json.Unmarshal([]byte(`{"name":"a/b"}`), &c)
		assert.NoError(t, err)
		out, err := json.Marshal(c)
		assert.NoError(t, err)
		assert.NotContains(t, string(out), "autoload")
	})

	t.Run("newly added keys appended deterministically", func(t *testing.T) {
		input := `{"name":"a/b"}`
		var c ComposerJson
		err := json.Unmarshal([]byte(input), &c)
		assert.NoError(t, err)
		c.AddPackage("php", "^8.2")
		c.Description = "added"

		out, err := json.Marshal(c)
		assert.NoError(t, err)
		// name stays first (original order); new keys appended in sorted order.
		assert.True(t, len(string(out)) > len(input))
		assert.Contains(t, string(out), `"name":"a/b"`)
		assert.Contains(t, string(out), `"description":"added"`)
		assert.Contains(t, string(out), `"require":{"php":"^8.2"}`)
	})

	t.Run("order survives save to disk", func(t *testing.T) {
		dir := t.TempDir()
		file := dir + "/composer.json"
		input := "{\n  \"type\": \"library\",\n  \"name\": \"a/b\"\n}"
		err := os.WriteFile(file, []byte(input), 0o644)
		assert.NoError(t, err)

		c, err := ReadComposerJson(file)
		assert.NoError(t, err)
		err = c.Save()
		assert.NoError(t, err)

		written, err := os.ReadFile(file)
		assert.NoError(t, err)
		// "type" must still precede "name".
		typeIdx := indexOf(string(written), `"type"`)
		nameIdx := indexOf(string(written), `"name"`)
		assert.True(t, typeIdx >= 0 && nameIdx >= 0 && typeIdx < nameIdx, "type should precede name; got: %s", written)
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
		var c ComposerJson
		err := json.Unmarshal([]byte(`{"name":"a/b","prefer-stable":false}`), &c)
		assert.NoError(t, err)
		assert.NotNil(t, c.PreferStable)
		assert.False(t, *c.PreferStable)

		out, err := json.Marshal(c)
		assert.NoError(t, err)
		assert.Contains(t, string(out), `"prefer-stable":false`)
	})

	t.Run("default-branch false survives", func(t *testing.T) {
		var c ComposerJson
		err := json.Unmarshal([]byte(`{"name":"a/b","default-branch":false}`), &c)
		assert.NoError(t, err)
		assert.NotNil(t, c.DefaultBranch)
		assert.False(t, *c.DefaultBranch)

		out, err := json.Marshal(c)
		assert.NoError(t, err)
		assert.Contains(t, string(out), `"default-branch":false`)
	})

	t.Run("absent stays absent", func(t *testing.T) {
		var c ComposerJson
		err := json.Unmarshal([]byte(`{"name":"a/b"}`), &c)
		assert.NoError(t, err)
		assert.Nil(t, c.PreferStable)
		assert.Nil(t, c.DefaultBranch)

		out, err := json.Marshal(c)
		assert.NoError(t, err)
		assert.NotContains(t, string(out), "prefer-stable")
		assert.NotContains(t, string(out), "default-branch")
	})

	t.Run("true survives via Bool helper", func(t *testing.T) {
		c := ComposerJson{Name: "a/b", PreferStable: Bool(true)}
		out, err := json.Marshal(c)
		assert.NoError(t, err)
		assert.Contains(t, string(out), `"prefer-stable":true`)
	})
}
