// Package sbom generates Software Bill of Materials (SBOM) documents in the
// CycloneDX JSON format from a Composer lock file.
package sbom

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shyim/go-composer"
)

const (
	cyclonedxBOMFormat   = "CycloneDX"
	cyclonedxSpecVersion = "1.7"
)

// BOM is the root document of a CycloneDX SBOM.
type BOM struct {
	BOMFormat    string       `json:"bomFormat"`
	SpecVersion  string       `json:"specVersion"`
	SerialNumber string       `json:"serialNumber"`
	Version      int          `json:"version"`
	Metadata     Metadata     `json:"metadata"`
	Components   []Component  `json:"components"`
	Dependencies []Dependency `json:"dependencies,omitempty"`
}

// Metadata is the CycloneDX metadata object.
type Metadata struct {
	Timestamp string     `json:"timestamp"`
	Tools     *Tools     `json:"tools,omitempty"`
	Component *Component `json:"component,omitempty"`
}

// Tools is the CycloneDX 1.6+ structured tools container. The legacy flat
// array form was deprecated in 1.6 and is no longer emitted.
type Tools struct {
	Components []Component `json:"components,omitempty"`
}

// Component is a CycloneDX component entry.
type Component struct {
	Type               string              `json:"type"`
	BOMRef             string              `json:"bom-ref,omitempty"`
	Group              string              `json:"group,omitempty"`
	Name               string              `json:"name"`
	Version            string              `json:"version,omitempty"`
	Description        string              `json:"description,omitempty"`
	PURL               string              `json:"purl,omitempty"`
	Licenses           []LicenseChoice     `json:"licenses,omitempty"`
	Hashes             []Hash              `json:"hashes,omitempty"`
	ExternalReferences []ExternalReference `json:"externalReferences,omitempty"`
}

// LicenseChoice wraps a single license expression inside a CycloneDX licenses array.
type LicenseChoice struct {
	License License `json:"license"`
}

// License is either an SPDX identifier (ID) or free-text (Name).
type License struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// Hash is a CycloneDX hash entry.
type Hash struct {
	Alg     string `json:"alg"`
	Content string `json:"content"`
}

// ExternalReference is a CycloneDX external reference (website, vcs, distribution, …).
type ExternalReference struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// Dependency describes one node’s outgoing edges in the dependency graph.
type Dependency struct {
	Ref       string   `json:"ref"`
	DependsOn []string `json:"dependsOn,omitempty"`
}

// Options configures BOM generation.
type Options struct {
	// ApplicationName is the name of the root application component (e.g. the
	// project's composer name). Falls back to "application" when empty.
	ApplicationName string
	// ApplicationVersion is the version of the root application component.
	ApplicationVersion string

	// ToolName, when non-empty, is reported as a tools.components entry.
	// ToolGroup and ToolVersion are optional companions (Group defaults empty).
	ToolName    string
	ToolGroup   string
	ToolVersion string
	// ToolType is the CycloneDX component type used for the tool entry.
	// Defaults to "application" when ToolName is set and ToolType is empty.
	ToolType string

	// IncludeDevDependencies includes packages from the composer.lock
	// "packages-dev" section.
	IncludeDevDependencies bool

	// IsSPDXLicenseID optionally classifies a license string as a valid SPDX
	// identifier. When it returns true the value is emitted as license.id;
	// otherwise as license.name. When nil, every non-empty license is emitted
	// as license.name (always valid CycloneDX, no SPDX database required).
	//
	// Callers that want full SPDX validation can wire in github.com/shyim/go-spdx:
	//
	//	s, _ := spdx.NewSpdxLicenses()
	//	opts.IsSPDXLicenseID = func(id string) bool {
	//	    ok, _ := s.Validate(id)
	//	    return ok
	//	}
	IsSPDXLicenseID func(string) bool
}

// Generate builds a CycloneDX BOM from the given Composer lock.
func Generate(lock *composer.Lock, opts Options) (*BOM, error) {
	if lock == nil {
		return nil, fmt.Errorf("composer lock is nil")
	}

	serial, err := newSerialNumber()
	if err != nil {
		return nil, err
	}

	appName := opts.ApplicationName
	if appName == "" {
		appName = "application"
	}

	rootRef := "app:" + appName
	if opts.ApplicationVersion != "" {
		rootRef = rootRef + "@" + opts.ApplicationVersion
	}

	bom := &BOM{
		BOMFormat:    cyclonedxBOMFormat,
		SpecVersion:  cyclonedxSpecVersion,
		SerialNumber: serial,
		Version:      1,
		Metadata: Metadata{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Component: &Component{
				Type:    "application",
				BOMRef:  rootRef,
				Name:    appName,
				Version: opts.ApplicationVersion,
			},
		},
	}

	if opts.ToolName != "" {
		toolType := opts.ToolType
		if toolType == "" {
			toolType = "application"
		}
		bom.Metadata.Tools = &Tools{
			Components: []Component{
				{
					Type:    toolType,
					Group:   opts.ToolGroup,
					Name:    opts.ToolName,
					Version: opts.ToolVersion,
				},
			},
		}
	}

	packages := lock.Packages
	if opts.IncludeDevDependencies {
		packages = append(packages, lock.PackagesDev...)
	}

	refByName := make(map[string]string, len(packages))
	bom.Components = make([]Component, 0, len(packages))

	for _, pkg := range packages {
		component := componentFromPackage(pkg, opts.IsSPDXLicenseID)
		refByName[pkg.Name] = component.BOMRef
		bom.Components = append(bom.Components, component)
	}

	bom.Dependencies = buildDependencies(packages, refByName, rootRef)

	return bom, nil
}

// Marshal returns the BOM rendered as indented JSON.
func Marshal(bom *BOM) ([]byte, error) {
	return json.MarshalIndent(bom, "", "  ")
}

func componentFromPackage(pkg composer.LockPackage, isSPDX func(string) bool) Component {
	purl := buildPURL(pkg)
	group, name := splitComposerName(pkg.Name)

	component := Component{
		Type:        cyclonedxType(pkg.Type),
		BOMRef:      purl,
		Group:       group,
		Name:        name,
		Version:     pkg.Version,
		Description: pkg.Description,
		PURL:        purl,
		Licenses:    licensesFromPackage(pkg.License, isSPDX),
	}

	if pkg.Dist.Shasum != "" {
		// Composer lock dist.shasum is a SHA-1 hex digest.
		component.Hashes = append(component.Hashes, Hash{Alg: "SHA-1", Content: pkg.Dist.Shasum})
	}

	if pkg.Homepage != "" {
		component.ExternalReferences = append(component.ExternalReferences, ExternalReference{Type: "website", URL: pkg.Homepage})
	}
	if pkg.Source.URL != "" {
		component.ExternalReferences = append(component.ExternalReferences, ExternalReference{Type: "vcs", URL: pkg.Source.URL})
	}
	if pkg.Dist.URL != "" {
		component.ExternalReferences = append(component.ExternalReferences, ExternalReference{Type: "distribution", URL: pkg.Dist.URL})
	}

	return component
}

func buildDependencies(packages []composer.LockPackage, refByName map[string]string, rootRef string) []Dependency {
	deps := make([]Dependency, 0, len(packages)+1)

	rootDeps := make([]string, 0, len(packages))
	for _, pkg := range packages {
		if ref, ok := refByName[pkg.Name]; ok {
			rootDeps = append(rootDeps, ref)
		}
	}
	sort.Strings(rootDeps)
	deps = append(deps, Dependency{Ref: rootRef, DependsOn: rootDeps})

	for _, pkg := range packages {
		ref, ok := refByName[pkg.Name]
		if !ok {
			continue
		}

		dependsOn := make([]string, 0, len(pkg.Require))
		for required := range pkg.Require {
			if isPlatformPackage(required) {
				continue
			}
			if depRef, ok := refByName[required]; ok {
				dependsOn = append(dependsOn, depRef)
			}
		}

		sort.Strings(dependsOn)
		deps = append(deps, Dependency{Ref: ref, DependsOn: dependsOn})
	}

	return deps
}

// isPlatformPackage reports whether the given composer package name refers to a
// platform requirement (php, ext-*, lib-*, composer-plugin-api, …) that should
// not appear as an SBOM dependency edge.
func isPlatformPackage(name string) bool {
	if name == "php" || name == "hhvm" {
		return true
	}
	for _, prefix := range []string{"php-", "ext-", "lib-", "composer-"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func buildPURL(pkg composer.LockPackage) string {
	return "pkg:composer/" + pkg.Name + "@" + pkg.Version
}

func splitComposerName(name string) (group, pkgName string) {
	if idx := strings.Index(name, "/"); idx > 0 {
		return name[:idx], name[idx+1:]
	}
	return "", name
}

func licensesFromPackage(licenses []string, isSPDX func(string) bool) []LicenseChoice {
	if len(licenses) == 0 {
		return nil
	}

	out := make([]LicenseChoice, 0, len(licenses))
	for _, license := range licenses {
		license = strings.TrimSpace(license)
		if license == "" {
			continue
		}
		if isSPDX != nil && isSPDX(license) {
			out = append(out, LicenseChoice{License: License{ID: license}})
		} else {
			out = append(out, LicenseChoice{License: License{Name: license}})
		}
	}
	return out
}

// cyclonedxType maps a composer package type to a CycloneDX component type.
func cyclonedxType(composerType string) string {
	switch composerType {
	case "project":
		return "application"
	default:
		// library, metapackage, and anything else fall back to "library".
		return "library"
	}
}

// newSerialNumber returns a CycloneDX serial number of the form
// "urn:uuid:xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx" (UUID v4).
func newSerialNumber() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate serial: %w", err)
	}

	// UUID v4 per RFC 4122.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("urn:uuid:%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
