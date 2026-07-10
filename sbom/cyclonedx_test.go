package sbom

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/shyim/go-composer"
)

func equal(t *testing.T, want, got any) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("not equal:\nwant: %#v\ngot:  %#v", want, got)
	}
}

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
		})
		if err != nil {
			t.Fatal(err)
		}
		if bom == nil {
			t.Fatal("expected non-nil BOM")
		}

		equal(t, "CycloneDX", bom.BOMFormat)
		equal(t, "1.7", bom.SpecVersion)
		if !strings.HasPrefix(bom.SerialNumber, "urn:uuid:") {
			t.Fatalf("serial number %q should start with urn:uuid:", bom.SerialNumber)
		}
		equal(t, 1, bom.Version)

		if bom.Metadata.Component == nil {
			t.Fatal("expected metadata.component")
		}
		equal(t, "application", bom.Metadata.Component.Type)
		equal(t, "acme/shop", bom.Metadata.Component.Name)
		equal(t, "1.0.0", bom.Metadata.Component.Version)
		equal(t, "app:acme/shop@1.0.0", bom.Metadata.Component.BOMRef)

		if bom.Metadata.Tools == nil || len(bom.Metadata.Tools.Components) != 1 {
			t.Fatalf("expected one tool component, got %#v", bom.Metadata.Tools)
		}
		equal(t, "acme", bom.Metadata.Tools.Components[0].Group)
		equal(t, "my-tool", bom.Metadata.Tools.Components[0].Name)
		equal(t, "test", bom.Metadata.Tools.Components[0].Version)

		if len(bom.Components) != 2 {
			t.Fatalf("expected 2 components, got %d", len(bom.Components))
		}

		var consoleComponent Component
		for _, c := range bom.Components {
			if c.Name == "console" {
				consoleComponent = c
				break
			}
		}

		equal(t, "library", consoleComponent.Type)
		equal(t, "symfony", consoleComponent.Group)
		equal(t, "v6.3.0", consoleComponent.Version)
		equal(t, "pkg:composer/symfony/console@v6.3.0", consoleComponent.PURL)
		equal(t, "pkg:composer/symfony/console@v6.3.0", consoleComponent.BOMRef)
		if len(consoleComponent.Licenses) != 1 {
			t.Fatalf("expected 1 license, got %d", len(consoleComponent.Licenses))
		}
		equal(t, "MIT", consoleComponent.Licenses[0].License.ID)
		equal(t, "", consoleComponent.Licenses[0].License.Name)
		if len(consoleComponent.Hashes) != 1 {
			t.Fatalf("expected 1 hash, got %d", len(consoleComponent.Hashes))
		}
		equal(t, "SHA-1", consoleComponent.Hashes[0].Alg)
		equal(t, "abcdef0123456789", consoleComponent.Hashes[0].Content)

		referenceTypes := make([]string, 0, len(consoleComponent.ExternalReferences))
		for _, ref := range consoleComponent.ExternalReferences {
			referenceTypes = append(referenceTypes, ref.Type)
		}
		if len(referenceTypes) != 3 {
			t.Fatalf("expected 3 external refs, got %v", referenceTypes)
		}
		wantRefs := map[string]bool{"website": true, "vcs": true, "distribution": true}
		for _, rt := range referenceTypes {
			if !wantRefs[rt] {
				t.Fatalf("unexpected external ref type %q", rt)
			}
		}
	})

	t.Run("includes dev dependencies when requested", func(t *testing.T) {
		bom, err := Generate(lock, Options{IncludeDevDependencies: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(bom.Components) != 3 {
			t.Fatalf("expected 3 components, got %d", len(bom.Components))
		}

		found := false
		for _, c := range bom.Components {
			if c.Group+"/"+c.Name == "phpunit/phpunit" {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("expected phpunit/phpunit among components")
		}
	})

	t.Run("dependencies link composer packages and skip platform requirements", func(t *testing.T) {
		bom, err := Generate(lock, Options{ApplicationName: "acme/shop"})
		if err != nil {
			t.Fatal(err)
		}

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

		equal(t, []string{"pkg:composer/symfony/string@v6.3.0"}, consoleDeps.DependsOn)
		if len(rootDeps.DependsOn) != 2 {
			t.Fatalf("expected 2 root deps, got %v", rootDeps.DependsOn)
		}
		wantRoot := map[string]bool{
			"pkg:composer/symfony/console@v6.3.0": true,
			"pkg:composer/symfony/string@v6.3.0":  true,
		}
		for _, d := range rootDeps.DependsOn {
			if !wantRoot[d] {
				t.Fatalf("unexpected root dep %q", d)
			}
		}
	})

	t.Run("omits tools when ToolName empty", func(t *testing.T) {
		bom, err := Generate(lock, Options{ApplicationName: "shop"})
		if err != nil {
			t.Fatal(err)
		}
		if bom.Metadata.Tools != nil {
			t.Fatalf("expected nil tools, got %#v", bom.Metadata.Tools)
		}
	})

	t.Run("nil lock returns error", func(t *testing.T) {
		bom, err := Generate(nil, Options{})
		if err == nil {
			t.Fatal("expected error for nil lock")
		}
		if bom != nil {
			t.Fatalf("expected nil BOM, got %#v", bom)
		}
	})
}

func TestMarshalProducesValidCycloneDXJSON(t *testing.T) {
	bom, err := Generate(&composer.Lock{
		Packages: []composer.LockPackage{
			{Name: "symfony/console", Version: "v6.3.0", License: []string{"MIT"}},
		},
	}, Options{ApplicationName: "shop", ToolName: "shopware-cli", ToolVersion: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}

	data, err := Marshal(bom)
	if err != nil {
		t.Fatal(err)
	}

	roundTrip := map[string]any{}
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatal(err)
	}

	equal(t, "CycloneDX", roundTrip["bomFormat"])
	equal(t, "1.7", roundTrip["specVersion"])

	tools := roundTrip["metadata"].(map[string]any)["tools"].(map[string]any)
	toolComponents := tools["components"].([]any)
	if len(toolComponents) != 1 {
		t.Fatalf("expected 1 tool component, got %d", len(toolComponents))
	}
	equal(t, "shopware-cli", toolComponents[0].(map[string]any)["name"])
}

func TestIsPlatformPackage(t *testing.T) {
	platform := []string{"php", "hhvm", "ext-mbstring", "lib-curl", "composer-runtime-api", "composer-plugin-api", "php-64bit"}
	notPlatform := []string{"symfony/console", "shopware/core", "composer/semver", "composer/installers"}

	for _, name := range platform {
		if !isPlatformPackage(name) {
			t.Fatalf("expected %q to be a platform package", name)
		}
	}
	for _, name := range notPlatform {
		if isPlatformPackage(name) {
			t.Fatalf("expected %q not to be a platform package", name)
		}
	}
}

func TestSplitComposerName(t *testing.T) {
	group, name := splitComposerName("symfony/console")
	equal(t, "symfony", group)
	equal(t, "console", name)

	group, name = splitComposerName("standalone")
	equal(t, "", group)
	equal(t, "standalone", name)
}

func TestLicensesFromPackage(t *testing.T) {
	licenses := licensesFromPackage([]string{"MIT", "proprietary", "  ", "BSD-3-Clause"})
	if len(licenses) != 3 {
		t.Fatalf("expected 3 licenses, got %d", len(licenses))
	}

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

	if !idMatches["MIT"] {
		t.Fatal("MIT should be SPDX id")
	}
	if !idMatches["BSD-3-Clause"] {
		t.Fatal("BSD-3-Clause should be SPDX id")
	}
	if !nameMatches["proprietary"] {
		t.Fatal("proprietary should be free-text name")
	}
}

func TestCyclonedxType(t *testing.T) {
	equal(t, "library", cyclonedxType(""))
	equal(t, "library", cyclonedxType("library"))
	equal(t, "library", cyclonedxType("metapackage"))
	equal(t, "application", cyclonedxType("project"))
	equal(t, "library", cyclonedxType("shopware-platform-plugin"))
}

func TestNewSerialNumberIsUUIDv4Like(t *testing.T) {
	serial, err := newSerialNumber()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(serial, "urn:uuid:") {
		t.Fatalf("serial %q should start with urn:uuid:", serial)
	}

	uuid := strings.TrimPrefix(serial, "urn:uuid:")
	parts := strings.Split(uuid, "-")
	if len(parts) != 5 {
		t.Fatalf("expected 5 UUID parts, got %v", parts)
	}
	if len(parts[0]) != 8 || len(parts[1]) != 4 || len(parts[2]) != 4 || len(parts[3]) != 4 || len(parts[4]) != 12 {
		t.Fatalf("unexpected UUID layout: %v", parts)
	}
	if parts[2][0] != '4' {
		t.Fatalf("version nibble should be 4, got %q", parts[2])
	}
}
