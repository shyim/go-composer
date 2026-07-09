package composer

import (
	"bytes"
	"encoding/json"
	"os"
	"sort"
)

type Author struct {
	Name     string `json:"name,omitempty"`
	Email    string `json:"email,omitempty"`
	Homepage string `json:"homepage,omitempty"`
	Role     string `json:"role,omitempty"`
}

type Support struct {
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

type Funding struct {
	Type string `json:"type,omitempty"`
	URL  string `json:"url,omitempty"`
}

type PackageLink map[string]string

// StringOrSlice decodes a composer.json value that may be either a single JSON
// string or an array of strings, always exposing it as a slice. It remembers
// whether the source was scalar so it can re-emit the same shape on marshal.
type StringOrSlice struct {
	Values []string
	single bool
}

// NewString returns a StringOrSlice that marshals back as a single JSON string.
func NewString(s string) *StringOrSlice {
	return &StringOrSlice{Values: []string{s}, single: true}
}

// NewStrings returns a StringOrSlice that marshals back as a JSON array.
func NewStrings(s ...string) *StringOrSlice {
	return &StringOrSlice{Values: append([]string(nil), s...), single: false}
}

// Strings returns the contained values.
func (s *StringOrSlice) Strings() []string {
	if s == nil {
		return nil
	}
	return s.Values
}

// First returns the first value, or "" when empty.
func (s *StringOrSlice) First() string {
	if s == nil || len(s.Values) == 0 {
		return ""
	}
	return s.Values[0]
}

func (s *StringOrSlice) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		s.Values = []string{single}
		s.single = true
		return nil
	}

	var arr []string
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	s.Values = arr
	s.single = false
	return nil
}

func (s StringOrSlice) MarshalJSON() ([]byte, error) {
	if s.single && len(s.Values) == 1 {
		return json.Marshal(s.Values[0])
	}
	return json.Marshal(s.Values)
}

// BoolOrString decodes a composer.json "abandoned" value, which may be either a
// boolean or a string naming the recommended replacement package.
type BoolOrString struct {
	Bool     bool
	String   string
	isString bool
}

// NewAbandonedBool marks a package abandoned (or not) without a replacement.
func NewAbandonedBool(b bool) *BoolOrString {
	return &BoolOrString{Bool: b, isString: false}
}

// NewAbandonedReplacement marks a package abandoned in favor of replacement.
func NewAbandonedReplacement(replacement string) *BoolOrString {
	return &BoolOrString{String: replacement, isString: true}
}

// IsAbandoned reports whether the package is abandoned.
func (b *BoolOrString) IsAbandoned() bool {
	if b == nil {
		return false
	}
	if b.isString {
		return b.String != ""
	}
	return b.Bool
}

// Replacement returns the recommended replacement package, or "".
func (b *BoolOrString) Replacement() string {
	if b == nil || !b.isString {
		return ""
	}
	return b.String
}

func (b *BoolOrString) UnmarshalJSON(data []byte) error {
	var asBool bool
	if err := json.Unmarshal(data, &asBool); err == nil {
		b.Bool = asBool
		b.isString = false
		return nil
	}

	var asString string
	if err := json.Unmarshal(data, &asString); err != nil {
		return err
	}
	b.String = asString
	b.isString = true
	return nil
}

func (b BoolOrString) MarshalJSON() ([]byte, error) {
	if b.isString {
		return json.Marshal(b.String)
	}
	return json.Marshal(b.Bool)
}

type Autoload struct {
	Psr0     map[string]string `json:"psr-0,omitempty"`
	Psr4     map[string]string `json:"psr-4,omitempty"`
	Classmap []string          `json:"classmap,omitempty"`
	Files    []string          `json:"files,omitempty"`
	Exclude  []string          `json:"exclude-from-classmap,omitempty"`
}

// NewAutoload returns a pointer to the given autoload configuration, suitable
// for the Autoload / AutoloadDev fields.
func NewAutoload(a Autoload) *Autoload { return &a }

// Bool returns a pointer to b, for setting the optional tri-state boolean fields
// (PreferStable, DefaultBranch) where an explicit false must be distinguished
// from an absent key.
func Bool(b bool) *bool { return &b }

type Repository struct {
	Type    string         `json:"type,omitempty"`
	URL     string         `json:"url,omitempty"`
	Options map[string]any `json:"options,omitempty"`
}

type Repositories []Repository

// UnmarshalJSON decodes a composer.json "repositories" value, which may be
// either a JSON object keyed by repository name or a plain JSON array.
func (e *Repositories) UnmarshalJSON(data []byte) error {
	var asMap map[string]Repository

	if err := json.Unmarshal(data, &asMap); err == nil {
		*e = Repositories{}

		for _, v := range asMap {
			*e = append(*e, v)
		}

		return nil
	}

	var asArray []Repository
	err := json.Unmarshal(data, &asArray)

	if err != nil {
		return err
	}

	*e = asArray

	return nil
}

// HasRepository reports whether a repository with the given URL is configured.
func (r *Repositories) HasRepository(url string) bool {
	for _, repository := range *r {
		if repository.URL == url {
			return true
		}
	}

	return false
}

// Source models the root-level "source" object of a composer.json.
type Source struct {
	Type      string           `json:"type,omitempty"`
	URL       string           `json:"url,omitempty"`
	Reference string           `json:"reference,omitempty"`
	Mirrors   []map[string]any `json:"mirrors,omitempty"`
}

// Dist models the root-level "dist" object of a composer.json.
type Dist struct {
	Type      string           `json:"type,omitempty"`
	URL       string           `json:"url,omitempty"`
	Reference string           `json:"reference,omitempty"`
	Shasum    string           `json:"shasum,omitempty"`
	Mirrors   []map[string]any `json:"mirrors,omitempty"`
}

// Archive models the "archive" object of a composer.json.
type Archive struct {
	Name    string   `json:"name,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

// Json represents the contents of a composer.json file.
//
// Any top-level keys that are not modeled by an explicit field are preserved
// in AdditionalFields and written back unchanged, so that keys added by future
// Composer versions survive a read/modify/write round-trip.
type Json struct {
	path               string            `json:"-"`
	keyOrder           []string          `json:"-"`
	Name               string            `json:"name"`
	Abandoned          *BoolOrString     `json:"abandoned,omitempty"`
	Bin                *StringOrSlice    `json:"bin,omitempty"`
	Description        string            `json:"description,omitempty"`
	Version            string            `json:"version,omitempty"`
	Type               string            `json:"type,omitempty"`
	Keywords           []string          `json:"keywords,omitempty"`
	Homepage           string            `json:"homepage,omitempty"`
	Readme             string            `json:"readme,omitempty"`
	Time               string            `json:"time,omitempty"`
	License            *StringOrSlice    `json:"license,omitempty"`
	MinimumStability   string            `json:"minimum-stability,omitempty"`
	PreferStable       *bool             `json:"prefer-stable,omitempty"`
	Authors            []Author          `json:"authors,omitempty"`
	Support            *Support          `json:"support,omitempty"`
	Funding            []Funding         `json:"funding,omitempty"`
	Require            PackageLink       `json:"require,omitempty"`
	RequireDev         PackageLink       `json:"require-dev,omitempty"`
	Conflict           PackageLink       `json:"conflict,omitempty"`
	Replace            PackageLink       `json:"replace,omitempty"`
	Provide            PackageLink       `json:"provide,omitempty"`
	Autoload           *Autoload         `json:"autoload,omitempty"`
	AutoloadDev        *Autoload         `json:"autoload-dev,omitempty"`
	Repositories       Repositories      `json:"repositories,omitempty"`
	Config             map[string]any    `json:"config,omitempty"`
	Scripts            map[string]any    `json:"scripts,omitempty"`
	Extra              ExtraData         `json:"extra,omitempty"`
	Suggest            map[string]string `json:"suggest,omitempty"`
	NonFeatureBranches []string          `json:"non-feature-branches,omitempty"`

	Source              *Source             `json:"source,omitempty"`
	Dist                *Dist               `json:"dist,omitempty"`
	Archive             *Archive            `json:"archive,omitempty"`
	IncludePath         []string            `json:"include-path,omitempty"`
	TargetDir           string              `json:"target-dir,omitempty"`
	DefaultBranch       *bool               `json:"default-branch,omitempty"`
	PHPExt              map[string]any      `json:"php-ext,omitempty"`
	ScriptsDescriptions map[string]string   `json:"scripts-descriptions,omitempty"`
	ScriptsAliases      map[string][]string `json:"scripts-aliases,omitempty"`

	// AdditionalFields holds any top-level composer.json keys not covered by the
	// fields above. It is populated on read and merged back in on write,
	// preserving unknown or future Composer settings verbatim. Note this is
	// distinct from the modeled "extra" section, which is exposed via Extra.
	AdditionalFields map[string]json.RawMessage `json:"-"`
}

// composerJsonAlias mirrors Json without the custom (un)marshal methods,
// so the encoding/json default behavior can be reused without recursion.
type composerJsonAlias Json

// knownComposerJsonKeys lists the composer.json keys mapped to explicit fields.
// Keys not in this set are routed to Json.AdditionalFields on unmarshal.
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
	"source":               {},
	"dist":                 {},
	"archive":              {},
	"include-path":         {},
	"target-dir":           {},
	"default-branch":       {},
	"php-ext":              {},
	"scripts-descriptions": {},
	"scripts-aliases":      {},
}

// topLevelKeyOrder returns the top-level object keys of a JSON document in the
// order they appear. It returns nil if data is not a JSON object.
func topLevelKeyOrder(data []byte) []string {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil
	}
	var keys []string
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return keys
		}
		key, ok := keyTok.(string)
		if !ok {
			return keys
		}
		keys = append(keys, key)
		valTok, err := dec.Token()
		if err != nil {
			return keys
		}
		if d, ok := valTok.(json.Delim); ok && (d == '{' || d == '[') {
			depth := 1
			for depth > 0 {
				t, err := dec.Token()
				if err != nil {
					return keys
				}
				if dd, ok := t.(json.Delim); ok {
					if dd == '{' || dd == '[' {
						depth++
					} else {
						depth--
					}
				}
			}
		}
	}
	return keys
}

// UnmarshalJSON decodes the known composer.json fields and captures every
// remaining top-level key in AdditionalFields so it can be re-emitted unchanged
// on marshal.
func (c *Json) UnmarshalJSON(data []byte) error {
	var alias composerJsonAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	// Preserve fields set before unmarshalling (e.g. path) that are not part of
	// the JSON payload.
	path := c.path
	*c = Json(alias)
	c.path = path
	c.keyOrder = topLevelKeyOrder(data)

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

// MarshalJSON serializes the known composer.json fields, merges any unknown keys
// held in AdditionalFields, and emits top-level keys in their original document
// order when known (falling back to a deterministic sorted order for new keys
// and freshly-created documents).
func (c Json) MarshalJSON() ([]byte, error) {
	encoded, err := json.Marshal(composerJsonAlias(c))
	if err != nil {
		return nil, err
	}

	merged := map[string]json.RawMessage{}
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

	// Emit keys in original order first, then any remaining keys (newly added or
	// from a freshly created doc with no recorded order) in sorted order.
	ordered := make([]string, 0, len(merged))
	seen := map[string]struct{}{}
	for _, key := range c.keyOrder {
		if _, ok := merged[key]; ok {
			if _, dup := seen[key]; !dup {
				ordered = append(ordered, key)
				seen[key] = struct{}{}
			}
		}
	}
	rest := make([]string, 0, len(merged))
	for key := range merged {
		if _, ok := seen[key]; !ok {
			rest = append(rest, key)
		}
	}
	sort.Strings(rest)
	ordered = append(ordered, rest...)

	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range ordered {
		if i > 0 {
			buf.WriteByte(',')
		}
		keyBytes, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		buf.Write(keyBytes)
		buf.WriteByte(':')
		buf.Write(merged[key])
	}
	buf.WriteByte('}')

	return buf.Bytes(), nil
}

// HasPackage reports whether the given package is listed in "require".
func (c *Json) HasPackage(name string) bool {
	_, ok := c.Require[name]
	return ok
}

// HasPackageDev reports whether the given package is listed in "require-dev".
func (c *Json) HasPackageDev(name string) bool {
	_, ok := c.RequireDev[name]
	return ok
}

// HasConfig reports whether the given key is present in the "config" section.
func (c *Json) HasConfig(key string) bool {
	_, ok := c.Config[key]
	return ok
}

// EnableComposerPlugin marks the given plugin as allowed under
// config.allow-plugins.
func (c *Json) EnableComposerPlugin(name string) {
	allowedPlugins, ok := c.Config["allow-plugins"].(map[string]any)

	if !ok {
		allowedPlugins = map[string]any{}
	}

	allowedPlugins[name] = true

	c.Config["allow-plugins"] = allowedPlugins
}

// RemoveComposerPlugin removes the given plugin from config.allow-plugins.
func (c *Json) RemoveComposerPlugin(name string) {
	allowedPlugins, ok := c.Config["allow-plugins"].(map[string]any)

	if !ok {
		return
	}

	delete(allowedPlugins, name)

	c.Config["allow-plugins"] = allowedPlugins
}

// AddPackage adds or updates a package constraint in the "require" section.
func (c *Json) AddPackage(name, constraint string) {
	if c.Require == nil {
		c.Require = PackageLink{}
	}
	c.Require[name] = constraint
}

// EnsurePackage adds name=constraint to "require" only when the package is not
// already present in either "require" or "require-dev". Returns true when the
// document was modified.
func (c *Json) EnsurePackage(name, constraint string) bool {
	if c.HasPackage(name) || c.HasPackageDev(name) {
		return false
	}
	c.AddPackage(name, constraint)
	return true
}

// RemovePackage removes a package from the "require" section.
func (c *Json) RemovePackage(name string) {
	delete(c.Require, name)
}

// AddPackageDev adds or updates a package constraint in the "require-dev" section.
func (c *Json) AddPackageDev(name, constraint string) {
	if c.RequireDev == nil {
		c.RequireDev = PackageLink{}
	}
	c.RequireDev[name] = constraint
}

// EnsurePackageDev adds name=constraint to "require-dev" only when the package
// is not already present in either "require" or "require-dev". Returns true when
// the document was modified.
func (c *Json) EnsurePackageDev(name, constraint string) bool {
	if c.HasPackage(name) || c.HasPackageDev(name) {
		return false
	}
	c.AddPackageDev(name, constraint)
	return true
}

// RemovePackageDev removes a package from the "require-dev" section.
func (c *Json) RemovePackageDev(name string) {
	delete(c.RequireDev, name)
}

// AddRepository appends a repository to "repositories" unless one with the same
// URL is already configured.
func (c *Json) AddRepository(repo Repository) {
	if c.Repositories.HasRepository(repo.URL) {
		return
	}
	c.Repositories = append(c.Repositories, repo)
}

// RemoveRepository removes every repository whose URL matches the given value.
func (c *Json) RemoveRepository(url string) {
	filtered := c.Repositories[:0]
	for _, repo := range c.Repositories {
		if repo.URL != url {
			filtered = append(filtered, repo)
		}
	}
	if len(filtered) == 0 {
		c.Repositories = nil
	} else {
		c.Repositories = filtered
	}
}

// SetConfig sets a key in the "config" section, creating the section if needed.
func (c *Json) SetConfig(key string, value any) {
	if c.Config == nil {
		c.Config = map[string]any{}
	}
	c.Config[key] = value
}

// RemoveConfig removes a key from the "config" section.
func (c *Json) RemoveConfig(key string) {
	delete(c.Config, key)
}

// Save writes the composer.json back to the path it was read from.
func (c *Json) Save() error {
	content, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.path, content, 0o644)
}

// ReadJson reads and parses a composer.json file from the given path.
func ReadJson(composerPath string) (*Json, error) {
	var composerJson Json
	composerJson.path = composerPath

	content, err := os.ReadFile(composerPath)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(content, &composerJson); err != nil {
		return nil, err
	}

	if composerJson.Extra == nil {
		composerJson.Extra = ExtraData{}
	}

	return &composerJson, nil
}
