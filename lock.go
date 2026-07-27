package composer

import (
	"encoding/json"
	"fmt"
	"os"
)

type LockPackageDist struct {
	Type      string `json:"type,omitempty"`
	URL       string `json:"url,omitempty"`
	Reference string `json:"reference,omitempty"`
	Shasum    string `json:"shasum,omitempty"`
}

type LockPackageSource struct {
	Type      string `json:"type,omitempty"`
	URL       string `json:"url,omitempty"`
	Reference string `json:"reference,omitempty"`
}

type LockPackage struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Type        string            `json:"type,omitempty"`
	Require     map[string]string `json:"require"`
	License     []string          `json:"license,omitempty"`
	Description string            `json:"description,omitempty"`
	Homepage    string            `json:"homepage,omitempty"`
	Time        string            `json:"time,omitempty"`
	Dist        LockPackageDist   `json:"dist,omitempty"`
	Source      LockPackageSource `json:"source,omitempty"`
}

type Lock struct {
	Packages    []LockPackage `json:"packages"`
	PackagesDev []LockPackage `json:"packages-dev"`
}

func (c *Lock) GetPackage(name string) *LockPackage {
	if c == nil {
		return nil
	}
	for i := range c.Packages {
		if c.Packages[i].Name == name {
			return &c.Packages[i]
		}
	}

	return nil
}

func ReadLock(pathToFile string) (*Lock, error) {
	content, err := os.ReadFile(pathToFile)
	if err != nil {
		return nil, err
	}

	var lock Lock
	if err := json.Unmarshal(content, &lock); err != nil {
		return nil, fmt.Errorf("could not parse composer.lock: %w", err)
	}

	return &lock, nil
}
