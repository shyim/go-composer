package composer

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
)

type BasicAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type GitlabToken struct {
	Username string
	Token    string
}

func (t *GitlabToken) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		t.Token = s
		return nil
	}

	var obj struct {
		Username string `json:"username"`
		Token    string `json:"token"`
	}

	if err := json.Unmarshal(data, &obj); err == nil {
		t.Username = obj.Username
		t.Token = obj.Token
		return nil
	}

	return fmt.Errorf("cannot unmarshal gitlab-token from %q", string(data))
}

func (t GitlabToken) MarshalJSON() ([]byte, error) {
	if t.Username != "" {
		return json.Marshal(struct {
			Username string `json:"username"`
			Token    string `json:"token"`
		}{
			Username: t.Username,
			Token:    t.Token,
		})
	}

	return json.Marshal(t.Token)
}

type GitlabOAuthToken struct {
	ExpiresAt    int64  `json:"expires-at,omitempty"`
	RefreshToken string `json:"refresh-token,omitempty"`
	Token        string `json:"token"`
}

func (t *GitlabOAuthToken) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		t.Token = s
		return nil
	}

	var obj struct {
		ExpiresAt    int64  `json:"expires-at"`
		RefreshToken string `json:"refresh-token"`
		Token        string `json:"token"`
	}

	if err := json.Unmarshal(data, &obj); err == nil {
		t.ExpiresAt = obj.ExpiresAt
		t.RefreshToken = obj.RefreshToken
		t.Token = obj.Token
		return nil
	}

	return fmt.Errorf("cannot unmarshal gitlab-oauth from %q", string(data))
}

func (t GitlabOAuthToken) MarshalJSON() ([]byte, error) {
	if t.RefreshToken != "" || t.ExpiresAt > 0 {
		return json.Marshal(struct {
			ExpiresAt    int64  `json:"expires-at,omitempty"`
			RefreshToken string `json:"refresh-token,omitempty"`
			Token        string `json:"token"`
		}{
			ExpiresAt:    t.ExpiresAt,
			RefreshToken: t.RefreshToken,
			Token:        t.Token,
		})
	}

	return json.Marshal(t.Token)
}

// Auth represents the contents of a Composer auth.json file.
//
// Any top-level keys that are not modeled by an explicit field are preserved
// in Extra and written back unchanged, so that authentication methods added by
// future Composer versions survive a read/modify/write round-trip.
type Auth struct {
	path           string                       `json:"-"`
	HTTPBasicAuth  map[string]BasicAuth         `json:"http-basic,omitempty"`
	BearerAuth     map[string]string            `json:"bearer,omitempty"`
	GitlabAuth     map[string]GitlabToken       `json:"gitlab-token,omitempty"`
	GitlabOAuth    map[string]GitlabOAuthToken  `json:"gitlab-oauth,omitempty"`
	GithubOAuth    map[string]string            `json:"github-oauth,omitempty"`
	BitbucketOauth map[string]map[string]string `json:"bitbucket-oauth,omitempty"`
	CustomHeaders  map[string][]string          `json:"custom-headers,omitempty"`
	GitlabDomains  []string                     `json:"gitlab-domains,omitempty"`
	GithubDomains  []string                     `json:"github-domains,omitempty"`

	// Extra holds any top-level auth.json keys not covered by the fields above.
	// It is populated on read and merged back in on write, preserving unknown
	// or future Composer authentication settings verbatim.
	Extra map[string]json.RawMessage `json:"-"`
}

// composerAuthAlias mirrors Auth without the custom (un)marshal methods,
// so the encoding/json default behavior can be reused without recursion.
type composerAuthAlias Auth

// knownComposerAuthKeys lists the auth.json keys mapped to explicit fields. Keys
// not in this set are routed to Auth.Extra on unmarshal.
var knownComposerAuthKeys = map[string]struct{}{
	"http-basic":      {},
	"bearer":          {},
	"gitlab-token":    {},
	"gitlab-oauth":    {},
	"github-oauth":    {},
	"bitbucket-oauth": {},
	"custom-headers":  {},
	"gitlab-domains":  {},
	"github-domains":  {},
}

// UnmarshalJSON decodes the known auth.json fields and captures every remaining
// top-level key in Extra so it can be re-emitted unchanged on marshal.
func (a *Auth) UnmarshalJSON(data []byte) error {
	var alias composerAuthAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*a = Auth(alias)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	for key := range raw {
		if _, known := knownComposerAuthKeys[key]; known {
			delete(raw, key)
		}
	}

	if len(raw) > 0 {
		a.Extra = raw
	} else {
		a.Extra = nil
	}

	return nil
}

// MarshalJSON serializes the known auth.json fields and merges any unknown keys
// held in Extra back into the output object.
func (a Auth) MarshalJSON() ([]byte, error) {
	encoded, err := json.Marshal(composerAuthAlias(a))
	if err != nil {
		return nil, err
	}

	if len(a.Extra) == 0 {
		return encoded, nil
	}

	var merged map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &merged); err != nil {
		return nil, err
	}

	for key, value := range a.Extra {
		// Known fields always win over a stale Extra entry of the same name.
		if _, known := knownComposerAuthKeys[key]; known {
			continue
		}
		merged[key] = value
	}

	return json.Marshal(merged)
}

// Save writes the auth configuration back to the file it was read from.
func (a *Auth) Save() error {
	content, err := a.Json(true)
	if err != nil {
		return err
	}

	return os.WriteFile(a.path, content, 0o600)
}

// Json serializes the auth configuration to JSON, optionally indented.
func (a *Auth) Json(formatted bool) ([]byte, error) {
	if !formatted {
		return json.Marshal(a)
	}
	return json.MarshalIndent(a, "", "  ")
}

// initMaps initializes any nil credential maps so callers can write to them
// (e.g. auth.HTTPBasicAuth["host"] = ...) without a nil check.
func (a *Auth) initMaps() {
	if a.BearerAuth == nil {
		a.BearerAuth = map[string]string{}
	}
	if a.HTTPBasicAuth == nil {
		a.HTTPBasicAuth = map[string]BasicAuth{}
	}
	if a.GitlabAuth == nil {
		a.GitlabAuth = map[string]GitlabToken{}
	}
	if a.GitlabOAuth == nil {
		a.GitlabOAuth = map[string]GitlabOAuthToken{}
	}
	if a.CustomHeaders == nil {
		a.CustomHeaders = map[string][]string{}
	}
	if a.GithubOAuth == nil {
		a.GithubOAuth = map[string]string{}
	}
	if a.BitbucketOauth == nil {
		a.BitbucketOauth = map[string]map[string]string{}
	}
}

// MergeEnv merges credentials from the COMPOSER_AUTH environment variable — a
// JSON document in auth.json format — on top of the current configuration, the
// way Composer does. ReadAuth does not call this automatically; opt in with:
//
//	auth, err := composer.ReadAuth("auth.json")
//	// ...
//	if err := auth.MergeEnv(); err != nil { /* ... */ }
//
// For the per-host methods (http-basic, bearer, gitlab-token, gitlab-oauth,
// github-oauth, bitbucket-oauth, custom-headers) an environment entry overrides
// the entry for the same host; a non-empty gitlab-domains or github-domains
// list in the environment replaces the current list entirely. MergeEnv is a
// no-op when COMPOSER_AUTH is unset or empty, and returns an error when it is
// set but not valid JSON.
func (a *Auth) MergeEnv() error {
	composerAuth := os.Getenv("COMPOSER_AUTH")
	if composerAuth == "" {
		return nil
	}

	var envAuth Auth
	if err := json.Unmarshal([]byte(composerAuth), &envAuth); err != nil {
		return fmt.Errorf("parsing COMPOSER_AUTH: %w", err)
	}

	a.initMaps()
	maps.Copy(a.HTTPBasicAuth, envAuth.HTTPBasicAuth)
	maps.Copy(a.BearerAuth, envAuth.BearerAuth)
	maps.Copy(a.GitlabAuth, envAuth.GitlabAuth)
	maps.Copy(a.GitlabOAuth, envAuth.GitlabOAuth)
	maps.Copy(a.GithubOAuth, envAuth.GithubOAuth)
	maps.Copy(a.BitbucketOauth, envAuth.BitbucketOauth)
	maps.Copy(a.CustomHeaders, envAuth.CustomHeaders)
	if len(envAuth.GitlabDomains) > 0 {
		a.GitlabDomains = envAuth.GitlabDomains
	}
	if len(envAuth.GithubDomains) > 0 {
		a.GithubDomains = envAuth.GithubDomains
	}
	if len(envAuth.Extra) > 0 {
		if a.Extra == nil {
			a.Extra = map[string]json.RawMessage{}
		}
		maps.Copy(a.Extra, envAuth.Extra)
	}

	return nil
}

// ReadAuth reads a Composer auth.json file from the given path. If the file
// does not exist, an empty (but usable) auth configuration is returned rather
// than an error.
//
// ReadAuth reads only the file. To also layer in credentials from the
// COMPOSER_AUTH environment variable (as the Composer CLI does), call MergeEnv
// on the result.
func ReadAuth(authFile string) (*Auth, error) {
	content, err := os.ReadFile(authFile)
	if err != nil {
		if os.IsNotExist(err) {
			auth := &Auth{path: authFile}
			auth.initMaps()
			return auth, nil
		}
		return nil, err
	}

	var auth Auth
	if err := json.Unmarshal(content, &auth); err != nil {
		return nil, err
	}

	auth.path = authFile
	auth.initMaps()

	return &auth, nil
}
