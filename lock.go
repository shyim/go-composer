package packagist

import (
	"encoding/json"
	"fmt"
	"os"
)

type ComposerLockPackageDist struct {
	Type      string `json:"type,omitempty"`
	URL       string `json:"url,omitempty"`
	Reference string `json:"reference,omitempty"`
	Shasum    string `json:"shasum,omitempty"`
}

type ComposerLockPackageSource struct {
	Type      string `json:"type,omitempty"`
	URL       string `json:"url,omitempty"`
	Reference string `json:"reference,omitempty"`
}

type ComposerLockPackage struct {
	Name        string                    `json:"name"`
	Version     string                    `json:"version"`
	Type        string                    `json:"type,omitempty"`
	Require     map[string]string         `json:"require"`
	License     []string                  `json:"license,omitempty"`
	Description string                    `json:"description,omitempty"`
	Homepage    string                    `json:"homepage,omitempty"`
	Time        string                    `json:"time,omitempty"`
	Dist        ComposerLockPackageDist   `json:"dist,omitempty"`
	Source      ComposerLockPackageSource `json:"source,omitempty"`
}

type ComposerLock struct {
	Packages    []ComposerLockPackage `json:"packages"`
	PackagesDev []ComposerLockPackage `json:"packages-dev"`
}

func (c *ComposerLock) GetPackage(name string) *ComposerLockPackage {
	for _, pkg := range c.Packages {
		if pkg.Name == name {
			return &pkg
		}
	}

	return nil
}

// PHPConstraint returns the require.php constraint declared by the first of the
// given package names that is present in the lock file and declares a php
// requirement. Returns nil when none match or none declare a php requirement.
func (c *ComposerLock) PHPConstraint(packageNames ...string) *PHPConstraint {
	for _, name := range packageNames {
		pkg := c.GetPackage(name)
		if pkg == nil {
			continue
		}
		if php, ok := pkg.Require["php"]; ok && php != "" {
			return NewPHPConstraint(php)
		}
	}
	return nil
}

func ReadComposerLock(pathToFile string) (*ComposerLock, error) {
	content, err := os.ReadFile(pathToFile)
	if err != nil {
		return nil, err
	}

	var lock ComposerLock
	if err := json.Unmarshal(content, &lock); err != nil {
		return nil, fmt.Errorf("could not parse composer.lock: %w", err)
	}

	return &lock, nil
}
