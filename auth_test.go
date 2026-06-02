package composer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitMaps(t *testing.T) {
	// Empty struct gets its maps initialized.
	auth := &Auth{}
	auth.initMaps()

	assert.NotNil(t, auth.HTTPBasicAuth)
	assert.NotNil(t, auth.BearerAuth)
	assert.NotNil(t, auth.GitlabAuth)
	assert.NotNil(t, auth.GithubOAuth)
	assert.NotNil(t, auth.BitbucketOauth)

	// Existing entries are preserved.
	auth = &Auth{
		HTTPBasicAuth: map[string]BasicAuth{
			"example.org": {Username: "user", Password: "pass"},
		},
	}
	auth.initMaps()

	assert.Equal(t, "user", auth.HTTPBasicAuth["example.org"].Username)
	assert.Equal(t, "pass", auth.HTTPBasicAuth["example.org"].Password)
	assert.NotNil(t, auth.BearerAuth)
}

func TestMergeEnv(t *testing.T) {
	t.Run("merges COMPOSER_AUTH", func(t *testing.T) {
		composerAuth := `{
			"http-basic": {
				"example.com": {
					"username": "user",
					"password": "password"
				}
			},
			"bearer": {
				"example.com": "bearer-token"
			}
		}`
		t.Setenv("COMPOSER_AUTH", composerAuth)

		auth := &Auth{}
		assert.NoError(t, auth.MergeEnv())

		assert.Equal(t, "user", auth.HTTPBasicAuth["example.com"].Username)
		assert.Equal(t, "password", auth.HTTPBasicAuth["example.com"].Password)
		assert.Equal(t, "bearer-token", auth.BearerAuth["example.com"])
	})

	t.Run("env overrides file entry for the same host", func(t *testing.T) {
		t.Setenv("COMPOSER_AUTH", `{"bearer":{"example.com":"from-env"}}`)

		auth := &Auth{BearerAuth: map[string]string{"example.com": "from-file", "other.com": "keep"}}
		assert.NoError(t, auth.MergeEnv())

		assert.Equal(t, "from-env", auth.BearerAuth["example.com"])
		assert.Equal(t, "keep", auth.BearerAuth["other.com"])
	})

	t.Run("invalid COMPOSER_AUTH returns an error", func(t *testing.T) {
		t.Setenv("COMPOSER_AUTH", "invalid-json")

		auth := &Auth{}
		assert.Error(t, auth.MergeEnv())
	})

	t.Run("unset COMPOSER_AUTH is a no-op", func(t *testing.T) {
		t.Setenv("COMPOSER_AUTH", "")

		auth := &Auth{}
		assert.NoError(t, auth.MergeEnv())
		assert.Empty(t, auth.HTTPBasicAuth)
		assert.Empty(t, auth.BearerAuth)
	})
}

func TestReadAuthDoesNotMergeEnv(t *testing.T) {
	// ReadAuth must read only the file; the environment is opt-in via MergeEnv.
	t.Setenv("COMPOSER_AUTH", `{"bearer":{"example.com":"from-env"}}`)

	tempDir := t.TempDir()
	authFile := filepath.Join(tempDir, "auth.json")
	assert.NoError(t, os.WriteFile(authFile, []byte(`{"bearer":{"file.com":"from-file"}}`), 0o644))

	auth, err := ReadAuth(authFile)
	assert.NoError(t, err)
	assert.Equal(t, "from-file", auth.BearerAuth["file.com"])
	assert.NotContains(t, auth.BearerAuth, "example.com")

	// Opting in pulls the environment credentials.
	assert.NoError(t, auth.MergeEnv())
	assert.Equal(t, "from-env", auth.BearerAuth["example.com"])
}

func TestComposerAuthSave(t *testing.T) {
	tempDir := t.TempDir()
	authFile := filepath.Join(tempDir, "auth.json")

	auth := &Auth{
		path: authFile,
		HTTPBasicAuth: map[string]BasicAuth{
			"example.org": {
				Username: "user",
				Password: "pass",
			},
		},
		BearerAuth: map[string]string{
			"api.example.org": "token123",
		},
	}

	err := auth.Save()
	assert.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(authFile)
	assert.NoError(t, err)

	// Read and verify content
	content, err := os.ReadFile(authFile)
	assert.NoError(t, err)

	var savedAuth Auth
	err = json.Unmarshal(content, &savedAuth)
	assert.NoError(t, err)

	assert.Equal(t, auth.HTTPBasicAuth["example.org"].Username, savedAuth.HTTPBasicAuth["example.org"].Username)
	assert.Equal(t, auth.HTTPBasicAuth["example.org"].Password, savedAuth.HTTPBasicAuth["example.org"].Password)
	assert.Equal(t, auth.BearerAuth["api.example.org"], savedAuth.BearerAuth["api.example.org"])
}

func TestComposerAuth_Json(t *testing.T) {
	auth := &Auth{
		HTTPBasicAuth: map[string]BasicAuth{
			"example.org": {
				Username: "user",
				Password: "pass",
			},
		},
		BearerAuth: map[string]string{
			"api.example.org": "token123",
		},
	}

	jsonBytes, err := auth.Json(true)
	assert.NoError(t, err)

	var decodedAuth Auth
	err = json.Unmarshal(jsonBytes, &decodedAuth)
	assert.NoError(t, err)

	assert.Equal(t, auth.HTTPBasicAuth["example.org"].Username, decodedAuth.HTTPBasicAuth["example.org"].Username)
	assert.Equal(t, auth.HTTPBasicAuth["example.org"].Password, decodedAuth.HTTPBasicAuth["example.org"].Password)
	assert.Equal(t, auth.BearerAuth["api.example.org"], decodedAuth.BearerAuth["api.example.org"])
}

func TestReadComposerAuth(t *testing.T) {
	// Test with existing file
	t.Run("existing file", func(t *testing.T) {
		tempDir := t.TempDir()
		authFile := filepath.Join(tempDir, "auth.json")

		testAuth := Auth{
			HTTPBasicAuth: map[string]BasicAuth{
				"example.org": {
					Username: "user",
					Password: "pass",
				},
			},
			BearerAuth: map[string]string{
				"api.example.org": "token123",
			},
		}

		content, err := json.MarshalIndent(testAuth, "", "  ")
		assert.NoError(t, err)
		err = os.WriteFile(authFile, content, 0o644)
		assert.NoError(t, err)

		auth, err := ReadAuth(authFile)
		assert.NoError(t, err)
		assert.Equal(t, authFile, auth.path)
		assert.Equal(t, "user", auth.HTTPBasicAuth["example.org"].Username)
		assert.Equal(t, "pass", auth.HTTPBasicAuth["example.org"].Password)
		assert.Equal(t, "token123", auth.BearerAuth["api.example.org"])
	})

	// Test with non-existing file, with fallback
	t.Run("non-existing file with fallback", func(t *testing.T) {
		tempDir := t.TempDir()
		authFile := filepath.Join(tempDir, "nonexistent.json")

		auth, err := ReadAuth(authFile)
		assert.NoError(t, err)
		assert.Equal(t, authFile, auth.path)
		assert.NotNil(t, auth.HTTPBasicAuth)
		assert.NotNil(t, auth.BearerAuth)
		assert.NotNil(t, auth.GitlabAuth)
		assert.NotNil(t, auth.GithubOAuth)
		assert.NotNil(t, auth.BitbucketOauth)
	})

	// Test with invalid JSON
	t.Run("invalid JSON", func(t *testing.T) {
		tempDir := t.TempDir()
		authFile := filepath.Join(tempDir, "invalid.json")

		err := os.WriteFile(authFile, []byte("{invalid json}"), 0o644)
		assert.NoError(t, err)

		auth, err := ReadAuth(authFile)
		assert.Error(t, err)
		assert.Nil(t, auth)
	})
}

func TestGitlabTokenUnmarshalling(t *testing.T) {
	t.Run("unmarshal string", func(t *testing.T) {
		jsonData := `{"gitlab-token": {"gitlab.com": "my-token"}}`
		var auth Auth
		err := json.Unmarshal([]byte(jsonData), &auth)
		assert.NoError(t, err)
		assert.Equal(t, "my-token", auth.GitlabAuth["gitlab.com"].Token)
		assert.Empty(t, auth.GitlabAuth["gitlab.com"].Username)
	})

	t.Run("unmarshal object", func(t *testing.T) {
		jsonData := `{"gitlab-token": {"gitlab.com": {"username": "my-user", "token": "my-token"}}}`
		var auth Auth
		err := json.Unmarshal([]byte(jsonData), &auth)
		assert.NoError(t, err)
		assert.Equal(t, "my-token", auth.GitlabAuth["gitlab.com"].Token)
		assert.Equal(t, "my-user", auth.GitlabAuth["gitlab.com"].Username)
	})

	t.Run("unmarshal mixed", func(t *testing.T) {
		jsonData := `{"gitlab-token": {"gitlab.com": "my-token", "example.com": {"username": "my-user", "token": "my-token2"}}}`
		var auth Auth
		err := json.Unmarshal([]byte(jsonData), &auth)
		assert.NoError(t, err)
		assert.Equal(t, "my-token", auth.GitlabAuth["gitlab.com"].Token)
		assert.Empty(t, auth.GitlabAuth["gitlab.com"].Username)
		assert.Equal(t, "my-token2", auth.GitlabAuth["example.com"].Token)
		assert.Equal(t, "my-user", auth.GitlabAuth["example.com"].Username)
	})

	t.Run("marshal string", func(t *testing.T) {
		auth := Auth{
			GitlabAuth: map[string]GitlabToken{
				"gitlab.com": {Token: "my-token"},
			},
		}
		jsonData, err := json.Marshal(auth)
		assert.NoError(t, err)
		assert.JSONEq(t, `{"gitlab-token": {"gitlab.com": "my-token"}}`, string(jsonData))
	})

	t.Run("marshal object", func(t *testing.T) {
		auth := Auth{
			GitlabAuth: map[string]GitlabToken{
				"gitlab.com": {Username: "my-user", Token: "my-token"},
			},
		}
		jsonData, err := json.Marshal(auth)
		assert.NoError(t, err)
		assert.JSONEq(t, `{"gitlab-token": {"gitlab.com": {"username": "my-user", "token": "my-token"}}}`, string(jsonData))
	})
}

func TestGitlabOAuthTokenUnmarshalling(t *testing.T) {
	t.Run("unmarshal string", func(t *testing.T) {
		jsonData := `{"gitlab-oauth": {"gitlab.com": "my-token"}}`
		var auth Auth
		err := json.Unmarshal([]byte(jsonData), &auth)
		assert.NoError(t, err)
		assert.Equal(t, "my-token", auth.GitlabOAuth["gitlab.com"].Token)
		assert.Empty(t, auth.GitlabOAuth["gitlab.com"].RefreshToken)
		assert.Zero(t, auth.GitlabOAuth["gitlab.com"].ExpiresAt)
	})

	t.Run("unmarshal object", func(t *testing.T) {
		jsonData := `{"gitlab-oauth": {"gitlab.com": {"token": "my-token", "refresh-token": "my-refresh", "expires-at": 123}}}`
		var auth Auth
		err := json.Unmarshal([]byte(jsonData), &auth)
		assert.NoError(t, err)
		assert.Equal(t, "my-token", auth.GitlabOAuth["gitlab.com"].Token)
		assert.Equal(t, "my-refresh", auth.GitlabOAuth["gitlab.com"].RefreshToken)
		assert.Equal(t, int64(123), auth.GitlabOAuth["gitlab.com"].ExpiresAt)
	})

	t.Run("marshal string", func(t *testing.T) {
		auth := Auth{
			GitlabOAuth: map[string]GitlabOAuthToken{
				"gitlab.com": {Token: "my-token"},
			},
		}
		jsonData, err := json.Marshal(auth)
		assert.NoError(t, err)
		assert.JSONEq(t, `{"gitlab-oauth": {"gitlab.com": "my-token"}}`, string(jsonData))
	})

	t.Run("marshal object", func(t *testing.T) {
		auth := Auth{
			GitlabOAuth: map[string]GitlabOAuthToken{
				"gitlab.com": {Token: "my-token", RefreshToken: "my-refresh", ExpiresAt: 123},
			},
		}
		jsonData, err := json.Marshal(auth)
		assert.NoError(t, err)
		assert.JSONEq(t, `{"gitlab-oauth": {"gitlab.com": {"token": "my-token", "refresh-token": "my-refresh", "expires-at": 123}}}`, string(jsonData))
	})
}

func TestCustomHeadersUnmarshalling(t *testing.T) {
	t.Run("unmarshal", func(t *testing.T) {
		jsonData := `{"custom-headers": {"example.com": ["Header-Name: Header-Value"]}}`
		var auth Auth
		err := json.Unmarshal([]byte(jsonData), &auth)
		assert.NoError(t, err)
		assert.Equal(t, []string{"Header-Name: Header-Value"}, auth.CustomHeaders["example.com"])
	})

	t.Run("marshal", func(t *testing.T) {
		auth := Auth{
			CustomHeaders: map[string][]string{
				"example.com": {"Header-Name: Header-Value"},
			},
		}
		jsonData, err := json.Marshal(auth)
		assert.NoError(t, err)
		assert.JSONEq(t, `{"custom-headers": {"example.com": ["Header-Name: Header-Value"]}}`, string(jsonData))
	})
}

func TestGitlabDomainsUnmarshalling(t *testing.T) {
	t.Run("unmarshal", func(t *testing.T) {
		jsonData := `{"gitlab-domains": ["gitlab.com", "example.com"]}`
		var auth Auth
		err := json.Unmarshal([]byte(jsonData), &auth)
		assert.NoError(t, err)
		assert.Equal(t, []string{"gitlab.com", "example.com"}, auth.GitlabDomains)
	})

	t.Run("marshal", func(t *testing.T) {
		auth := Auth{
			GitlabDomains: []string{"gitlab.com", "example.com"},
		}
		jsonData, err := json.Marshal(auth)
		assert.NoError(t, err)
		assert.JSONEq(t, `{"gitlab-domains": ["gitlab.com", "example.com"]}`, string(jsonData))
	})
}

func TestGithubDomainsUnmarshalling(t *testing.T) {
	t.Run("unmarshal", func(t *testing.T) {
		jsonData := `{"github-domains": ["github.com", "example.com"]}`
		var auth Auth
		err := json.Unmarshal([]byte(jsonData), &auth)
		assert.NoError(t, err)
		assert.Equal(t, []string{"github.com", "example.com"}, auth.GithubDomains)
	})

	t.Run("marshal", func(t *testing.T) {
		auth := Auth{
			GithubDomains: []string{"github.com", "example.com"},
		}
		jsonData, err := json.Marshal(auth)
		assert.NoError(t, err)
		assert.JSONEq(t, `{"github-domains": ["github.com", "example.com"]}`, string(jsonData))
	})
}

func TestComposerAuthPreservesUnknownFields(t *testing.T) {
	t.Run("unknown top-level keys round-trip", func(t *testing.T) {
		input := `{
			"bearer": {"example.com": "token"},
			"future-auth": {"example.com": {"client-id": "abc", "client-secret": "xyz"}},
			"some-flag": true
		}`

		var auth Auth
		err := json.Unmarshal([]byte(input), &auth)
		assert.NoError(t, err)

		// Known field is decoded normally.
		assert.Equal(t, "token", auth.BearerAuth["example.com"])

		// Unknown keys are captured in Extra.
		assert.Contains(t, auth.Extra, "future-auth")
		assert.Contains(t, auth.Extra, "some-flag")
		assert.NotContains(t, auth.Extra, "bearer")

		// Marshalling re-emits the unknown keys unchanged.
		out, err := json.Marshal(auth)
		assert.NoError(t, err)
		assert.JSONEq(t, input, string(out))
	})

	t.Run("no unknown keys leaves Extra nil", func(t *testing.T) {
		var auth Auth
		err := json.Unmarshal([]byte(`{"bearer": {"example.com": "token"}}`), &auth)
		assert.NoError(t, err)
		assert.Nil(t, auth.Extra)

		out, err := json.Marshal(auth)
		assert.NoError(t, err)
		assert.JSONEq(t, `{"bearer": {"example.com": "token"}}`, string(out))
	})

	t.Run("unknown keys survive read/write to disk", func(t *testing.T) {
		tempDir := t.TempDir()
		authFile := filepath.Join(tempDir, "auth.json")

		input := `{"bearer":{"example.com":"token"},"future-auth":{"example.com":"secret"}}`
		err := os.WriteFile(authFile, []byte(input), 0o600)
		assert.NoError(t, err)

		auth, err := ReadAuth(authFile)
		assert.NoError(t, err)
		assert.Contains(t, auth.Extra, "future-auth")

		err = auth.Save()
		assert.NoError(t, err)

		written, err := os.ReadFile(authFile)
		assert.NoError(t, err)

		var roundTripped Auth
		err = json.Unmarshal(written, &roundTripped)
		assert.NoError(t, err)
		assert.Equal(t, "token", roundTripped.BearerAuth["example.com"])
		assert.Contains(t, roundTripped.Extra, "future-auth")
		assert.JSONEq(t, `{"example.com":"secret"}`, string(roundTripped.Extra["future-auth"]))
	})

	t.Run("COMPOSER_AUTH unknown keys merge into Extra", func(t *testing.T) {
		t.Setenv("COMPOSER_AUTH", `{"future-auth":{"example.com":"env-secret"}}`)
		auth := &Auth{}
		assert.NoError(t, auth.MergeEnv())
		assert.Contains(t, auth.Extra, "future-auth")
		assert.JSONEq(t, `{"example.com":"env-secret"}`, string(auth.Extra["future-auth"]))
	})
}
