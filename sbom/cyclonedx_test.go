package sbom

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/shyim/go-composer"
	"github.com/shyim/go-composer/internal/testassert"
)

func sampleLock() *composer.Lock {
	return &composer.Lock{
		Packages: []composer.LockPackage{
			{
				Name:        "symfony/console",
				Version:     "v6.3.0",
				Type:        "library",
				Description: "Eases the creation of beautiful and testable command line interfaces",
				Homepage:    "https://symfony.com",
				License:     []string{"MIT"},
				Require: map[string]string{
					"php":            ">=8.1",
					"symfony/string": "^6.3",
				},
				Dist: composer.LockPackageDist{
					Type:   "zip",
					URL:    "https://api.github.com/repos/symfony/console/zipball/abc",
					Shasum: "abcdef0123456789",
				},
				Source: composer.LockPackageSource{
					Type: "git",
					URL:  "https://github.com/symfony/console.git",
				},
			},
			{
				Name:    "symfony/string",
				Version: "v6.3.0",
				Type:    "library",
				License: []string{"MIT"},
				Require: map[string]string{"php": ">=8.1"},
			},
		},
		PackagesDev: []composer.LockPackage{
			{Name: "phpunit/phpunit", Version: "10.0.0", License: []string{"BSD-3-Clause"}},
		},
	}
}

func TestGenerate(t *testing.T) {
	lock := sampleLock()

	t.Run("excludes dev dependencies by default", func(t *testing.T) {
		bom, err := Generate(lock, Options{
			ApplicationName:    "acme/shop",
			ApplicationVersion: "1.0.0",
			ToolName:           "my-tool",
			ToolGroup:          "acme",
			ToolVersion:        "test",
			// Treat only MIT as an SPDX id for this test.
			IsSPDXLicenseID: func(id string) bool { return id == "MIT" },
		})
		testassert.RequireNoError(t, err)
		testassert.RequireNotNil(t, bom)

		testassert.Equal(t, "CycloneDX", bom.BOMFormat)
		testassert.Equal(t, "1.7", bom.SpecVersion)
		testassert.True(t, strings.HasPrefix(bom.SerialNumber, "urn:uuid:"))
		testassert.Equal(t, 1, bom.Version)

		testassert.RequireNotNil(t, bom.Metadata.Component)
		testassert.Equal(t, "application", bom.Metadata.Component.Type)
		testassert.Equal(t, "acme/shop", bom.Metadata.Component.Name)
		testassert.Equal(t, "1.0.0", bom.Metadata.Component.Version)
		testassert.Equal(t, "app:acme/shop@1.0.0", bom.Metadata.Component.BOMRef)

		testassert.RequireNotNil(t, bom.Metadata.Tools)
		testassert.RequireLen(t, bom.Metadata.Tools.Components, 1)
		testassert.Equal(t, "acme", bom.Metadata.Tools.Components[0].Group)
		testassert.Equal(t, "my-tool", bom.Metadata.Tools.Components[0].Name)
		testassert.Equal(t, "test", bom.Metadata.Tools.Components[0].Version)

		testassert.Len(t, bom.Components, 2)

		var consoleComponent Component
		for _, c := range bom.Components {
			if c.Name == "console" {
				consoleComponent = c
				break
			}
		}

		testassert.Equal(t, "library", consoleComponent.Type)
		testassert.Equal(t, "symfony", consoleComponent.Group)
		testassert.Equal(t, "v6.3.0", consoleComponent.Version)
		testassert.Equal(t, "pkg:composer/symfony/console@v6.3.0", consoleComponent.PURL)
		testassert.Equal(t, "pkg:composer/symfony/console@v6.3.0", consoleComponent.BOMRef)
		testassert.RequireLen(t, consoleComponent.Licenses, 1)
		testassert.Equal(t, "MIT", consoleComponent.Licenses[0].License.ID)
		testassert.Equal(t, "", consoleComponent.Licenses[0].License.Name)
		testassert.RequireLen(t, consoleComponent.Hashes, 1)
		testassert.Equal(t, "SHA-1", consoleComponent.Hashes[0].Alg)
		testassert.Equal(t, "abcdef0123456789", consoleComponent.Hashes[0].Content)

		referenceTypes := make([]string, 0, len(consoleComponent.ExternalReferences))
		for _, ref := range consoleComponent.ExternalReferences {
			referenceTypes = append(referenceTypes, ref.Type)
		}
		testassert.ElementsMatch(t, []string{"website", "vcs", "distribution"}, referenceTypes)
	})

	t.Run("includes dev dependencies when requested", func(t *testing.T) {
		bom, err := Generate(lock, Options{IncludeDevDependencies: true})
		testassert.RequireNoError(t, err)
		testassert.Len(t, bom.Components, 3)

		names := make([]string, 0, len(bom.Components))
		for _, c := range bom.Components {
			names = append(names, c.Group+"/"+c.Name)
		}
		testassert.Contains(t, names, "phpunit/phpunit")
	})

	t.Run("dependencies link composer packages and skip platform requirements", func(t *testing.T) {
		bom, err := Generate(lock, Options{ApplicationName: "acme/shop"})
		testassert.RequireNoError(t, err)

		var consoleDeps Dependency
		var rootDeps Dependency
		for _, dep := range bom.Dependencies {
			if dep.Ref == "pkg:composer/symfony/console@v6.3.0" {
				consoleDeps = dep
			}
			if dep.Ref == "app:acme/shop" {
				rootDeps = dep
			}
		}

		testassert.Equal(t, []string{"pkg:composer/symfony/string@v6.3.0"}, consoleDeps.DependsOn)
		testassert.ElementsMatch(t,
			[]string{"pkg:composer/symfony/console@v6.3.0", "pkg:composer/symfony/string@v6.3.0"},
			rootDeps.DependsOn,
		)
	})

	t.Run("omits tools when ToolName empty", func(t *testing.T) {
		bom, err := Generate(lock, Options{ApplicationName: "shop"})
		testassert.RequireNoError(t, err)
		testassert.Nil(t, bom.Metadata.Tools)
	})

	t.Run("nil lock returns error", func(t *testing.T) {
		bom, err := Generate(nil, Options{})
		testassert.Error(t, err)
		testassert.Nil(t, bom)
	})
}

func TestMarshalProducesValidCycloneDXJSON(t *testing.T) {
	bom, err := Generate(&composer.Lock{
		Packages: []composer.LockPackage{
			{Name: "symfony/console", Version: "v6.3.0", License: []string{"MIT"}},
		},
	}, Options{ApplicationName: "shop", ToolName: "shopware-cli", ToolVersion: "1.0.0"})
	testassert.RequireNoError(t, err)

	data, err := Marshal(bom)
	testassert.RequireNoError(t, err)

	roundTrip := map[string]any{}
	testassert.RequireNoError(t, json.Unmarshal(data, &roundTrip))

	testassert.Equal(t, "CycloneDX", roundTrip["bomFormat"])
	testassert.Equal(t, "1.7", roundTrip["specVersion"])

	tools := roundTrip["metadata"].(map[string]any)["tools"].(map[string]any)
	toolComponents := tools["components"].([]any)
	testassert.RequireLen(t, toolComponents, 1)
	testassert.Equal(t, "shopware-cli", toolComponents[0].(map[string]any)["name"])
}

func TestIsPlatformPackage(t *testing.T) {
	platform := []string{"php", "hhvm", "ext-mbstring", "lib-curl", "composer-runtime-api", "composer-plugin-api", "php-64bit"}
	notPlatform := []string{"symfony/console", "shopware/core", "composer/semver", "composer/installers"}

	for _, name := range platform {
		testassert.True(t, isPlatformPackage(name), fmt.Sprintf("expected %q to be a platform package", name))
	}
	for _, name := range notPlatform {
		testassert.False(t, isPlatformPackage(name), fmt.Sprintf("expected %q not to be a platform package", name))
	}
}

func TestSplitComposerName(t *testing.T) {
	group, name := splitComposerName("symfony/console")
	testassert.Equal(t, "symfony", group)
	testassert.Equal(t, "console", name)

	group, name = splitComposerName("standalone")
	testassert.Equal(t, "", group)
	testassert.Equal(t, "standalone", name)
}

func TestLicensesFromPackage(t *testing.T) {
	t.Run("without SPDX classifier emits name", func(t *testing.T) {
		licenses := licensesFromPackage([]string{"MIT", "proprietary", "  ", "BSD-3-Clause"}, nil)
		testassert.Len(t, licenses, 3)
		for _, l := range licenses {
			testassert.Equal(t, "", l.License.ID)
			testassert.NotEmpty(t, l.License.Name)
		}
	})

	t.Run("with SPDX classifier maps ids and free text", func(t *testing.T) {
		isSPDX := func(id string) bool {
			return id == "MIT" || id == "BSD-3-Clause"
		}
		licenses := licensesFromPackage([]string{"MIT", "proprietary", "  ", "BSD-3-Clause"}, isSPDX)
		testassert.Len(t, licenses, 3)

		idMatches := map[string]bool{}
		nameMatches := map[string]bool{}
		for _, l := range licenses {
			if l.License.ID != "" {
				idMatches[l.License.ID] = true
			}
			if l.License.Name != "" {
				nameMatches[l.License.Name] = true
			}
		}

		testassert.True(t, idMatches["MIT"], "MIT should be SPDX id")
		testassert.True(t, idMatches["BSD-3-Clause"], "BSD-3-Clause should be SPDX id")
		testassert.True(t, nameMatches["proprietary"], "proprietary should be free-text name")
	})
}

func TestCyclonedxType(t *testing.T) {
	testassert.Equal(t, "library", cyclonedxType(""))
	testassert.Equal(t, "library", cyclonedxType("library"))
	testassert.Equal(t, "library", cyclonedxType("metapackage"))
	testassert.Equal(t, "application", cyclonedxType("project"))
	testassert.Equal(t, "library", cyclonedxType("shopware-platform-plugin"))
}

func TestNewSerialNumberIsUUIDv4Like(t *testing.T) {
	serial, err := newSerialNumber()
	testassert.RequireNoError(t, err)
	testassert.True(t, strings.HasPrefix(serial, "urn:uuid:"))

	uuid := strings.TrimPrefix(serial, "urn:uuid:")
	parts := strings.Split(uuid, "-")
	testassert.Len(t, parts, 5)
	testassert.Len(t, parts[0], 8)
	testassert.Len(t, parts[1], 4)
	testassert.Len(t, parts[2], 4)
	testassert.Len(t, parts[3], 4)
	testassert.Len(t, parts[4], 12)
	testassert.Equal(t, byte('4'), parts[2][0], "version nibble should be 4")
}
