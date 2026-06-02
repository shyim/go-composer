package packagist

import (
	"encoding/json"
	"os"
)

type ComposerJsonAuthor struct {
	Name     string `json:"name,omitempty"`
	Email    string `json:"email,omitempty"`
	Homepage string `json:"homepage,omitempty"`
	Role     string `json:"role,omitempty"`
}

type ComposerJsonSupport struct {
	Email    string `json:"email,omitempty"`
	Issues   string `json:"issues,omitempty"`
	Forum    string `json:"forum,omitempty"`
	Wiki     string `json:"wiki,omitempty"`
	IRC      string `json:"irc,omitempty"`
	Source   string `json:"source,omitempty"`
	Docs     string `json:"docs,omitempty"`
	RSS      string `json:"rss,omitempty"`
	Chat     string `json:"chat,omitempty"`
	Security string `json:"security,omitempty"`
}

type ComposerFunding struct {
	Type string `json:"type,omitempty"`
	URL  string `json:"url,omitempty"`
}

type ComposerPackageLink map[string]string

type ComposerJsonAutoload struct {
	Psr0     map[string]string `json:"psr-0,omitempty"`
	Psr4     map[string]string `json:"psr-4,omitempty"`
	Classmap []string          `json:"classmap,omitempty"`
	Files    []string          `json:"files,omitempty"`
	Exclude  []string          `json:"exclude-from-classmap,omitempty"`
}

type ComposerJsonRepository struct {
	Type    string         `json:"type,omitempty"`
	URL     string         `json:"url,omitempty"`
	Options map[string]any `json:"options,omitempty"`
}

type ComposerJsonRepositories []ComposerJsonRepository

// UnmarshalJSON decodes a composer.json "repositories" value, which may be
// either a JSON object keyed by repository name or a plain JSON array.
func (e *ComposerJsonRepositories) UnmarshalJSON(data []byte) error {
	var asMap map[string]ComposerJsonRepository

	if err := json.Unmarshal(data, &asMap); err == nil {
		*e = ComposerJsonRepositories{}

		for _, v := range asMap {
			*e = append(*e, v)
		}

		return nil
	}

	var asArray []ComposerJsonRepository
	err := json.Unmarshal(data, &asArray)

	if err != nil {
		return err
	}

	*e = asArray

	return nil
}

// HasRepository reports whether a repository with the given URL is configured.
func (r *ComposerJsonRepositories) HasRepository(url string) bool {
	for _, repository := range *r {
		if repository.URL == url {
			return true
		}
	}

	return false
}

// ComposerJson represents the contents of a composer.json file.
//
// Any top-level keys that are not modeled by an explicit field are preserved
// in AdditionalFields and written back unchanged, so that keys added by future
// Composer versions survive a read/modify/write round-trip.
type ComposerJson struct {
	path               string                   `json:"-"`
	Name               string                   `json:"name"`
	Abandoned          bool                     `json:"abandoned,omitempty"`
	Bin                []string                 `json:"bin,omitempty"`
	Description        string                   `json:"description,omitempty"`
	Version            string                   `json:"version,omitempty"`
	Type               string                   `json:"type,omitempty"`
	Keywords           []string                 `json:"keywords,omitempty"`
	Homepage           string                   `json:"homepage,omitempty"`
	Readme             string                   `json:"readme,omitempty"`
	Time               string                   `json:"time,omitempty"`
	License            string                   `json:"license,omitempty"`
	MinimumStability   string                   `json:"minimum-stability,omitempty"`
	PreferStable       bool                     `json:"prefer-stable,omitempty"`
	Authors            []ComposerJsonAuthor     `json:"authors,omitempty"`
	Support            *ComposerJsonSupport     `json:"support,omitempty"`
	Funding            []ComposerFunding        `json:"funding,omitempty"`
	Require            ComposerPackageLink      `json:"require,omitempty"`
	RequireDev         ComposerPackageLink      `json:"require-dev,omitempty"`
	Conflict           ComposerPackageLink      `json:"conflict,omitempty"`
	Replace            ComposerPackageLink      `json:"replace,omitempty"`
	Provide            ComposerPackageLink      `json:"provide,omitempty"`
	Autoload           ComposerJsonAutoload     `json:"autoload,omitempty"`
	AutoloadDev        ComposerJsonAutoload     `json:"autoload-dev,omitempty"`
	Repositories       ComposerJsonRepositories `json:"repositories,omitempty"`
	Config             map[string]any           `json:"config,omitempty"`
	Scripts            map[string]any           `json:"scripts,omitempty"`
	Extra              map[string]any           `json:"extra,omitempty"`
	Suggest            map[string]string        `json:"suggest,omitempty"`
	NonFeatureBranches []string                 `json:"non-feature-branches,omitempty"`

	// AdditionalFields holds any top-level composer.json keys not covered by the
	// fields above. It is populated on read and merged back in on write,
	// preserving unknown or future Composer settings verbatim. Note this is
	// distinct from the modeled "extra" section, which is exposed via Extra.
	AdditionalFields map[string]json.RawMessage `json:"-"`
}

// composerJsonAlias mirrors ComposerJson without the custom (un)marshal methods,
// so the encoding/json default behavior can be reused without recursion.
type composerJsonAlias ComposerJson

// knownComposerJsonKeys lists the composer.json keys mapped to explicit fields.
// Keys not in this set are routed to ComposerJson.AdditionalFields on unmarshal.
var knownComposerJsonKeys = map[string]struct{}{
	"name":                 {},
	"abandoned":            {},
	"bin":                  {},
	"description":          {},
	"version":              {},
	"type":                 {},
	"keywords":             {},
	"homepage":             {},
	"readme":               {},
	"time":                 {},
	"license":              {},
	"minimum-stability":    {},
	"prefer-stable":        {},
	"authors":              {},
	"support":              {},
	"funding":              {},
	"require":              {},
	"require-dev":          {},
	"conflict":             {},
	"replace":              {},
	"provide":              {},
	"autoload":             {},
	"autoload-dev":         {},
	"repositories":         {},
	"config":               {},
	"scripts":              {},
	"extra":                {},
	"suggest":              {},
	"non-feature-branches": {},
}

// UnmarshalJSON decodes the known composer.json fields and captures every
// remaining top-level key in AdditionalFields so it can be re-emitted unchanged
// on marshal.
func (c *ComposerJson) UnmarshalJSON(data []byte) error {
	var alias composerJsonAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	// Preserve fields set before unmarshalling (e.g. path) that are not part of
	// the JSON payload.
	path := c.path
	*c = ComposerJson(alias)
	c.path = path

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	for key := range raw {
		if _, known := knownComposerJsonKeys[key]; known {
			delete(raw, key)
		}
	}

	if len(raw) > 0 {
		c.AdditionalFields = raw
	} else {
		c.AdditionalFields = nil
	}

	return nil
}

// MarshalJSON serializes the known composer.json fields and merges any unknown
// keys held in AdditionalFields back into the output object.
func (c ComposerJson) MarshalJSON() ([]byte, error) {
	encoded, err := json.Marshal(composerJsonAlias(c))
	if err != nil {
		return nil, err
	}

	if len(c.AdditionalFields) == 0 {
		return encoded, nil
	}

	var merged map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &merged); err != nil {
		return nil, err
	}

	for key, value := range c.AdditionalFields {
		// Known fields always win over a stale AdditionalFields entry.
		if _, known := knownComposerJsonKeys[key]; known {
			continue
		}
		merged[key] = value
	}

	return json.Marshal(merged)
}

// HasPackage reports whether the given package is listed in "require".
func (c *ComposerJson) HasPackage(name string) bool {
	_, ok := c.Require[name]
	return ok
}

// HasPackageDev reports whether the given package is listed in "require-dev".
func (c *ComposerJson) HasPackageDev(name string) bool {
	_, ok := c.RequireDev[name]
	return ok
}

// HasConfig reports whether the given key is present in the "config" section.
func (c *ComposerJson) HasConfig(key string) bool {
	_, ok := c.Config[key]
	return ok
}

// EnableComposerPlugin marks the given plugin as allowed under
// config.allow-plugins.
func (c *ComposerJson) EnableComposerPlugin(name string) {
	allowedPlugins, ok := c.Config["allow-plugins"].(map[string]any)

	if !ok {
		allowedPlugins = map[string]any{}
	}

	allowedPlugins[name] = true

	c.Config["allow-plugins"] = allowedPlugins
}

// RemoveComposerPlugin removes the given plugin from config.allow-plugins.
func (c *ComposerJson) RemoveComposerPlugin(name string) {
	allowedPlugins, ok := c.Config["allow-plugins"].(map[string]any)

	if !ok {
		return
	}

	delete(allowedPlugins, name)

	c.Config["allow-plugins"] = allowedPlugins
}

// Save writes the composer.json back to the path it was read from.
func (c *ComposerJson) Save() error {
	content, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.path, content, 0o644)
}

// ReadComposerJson reads and parses a composer.json file from the given path.
func ReadComposerJson(composerPath string) (*ComposerJson, error) {
	var composerJson ComposerJson
	composerJson.path = composerPath

	content, err := os.ReadFile(composerPath)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(content, &composerJson); err != nil {
		return nil, err
	}

	if composerJson.Extra == nil {
		composerJson.Extra = map[string]any{}
	}

	return &composerJson, nil
}
