package repository

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSecurityAdvisoriesAPI(t *testing.T) {
	repo, _ := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/packages.json":
			_, _ = w.Write([]byte(`{"metadata-url":"/p2/%package%.json","security-advisories":{"api-url":"/advisories.json"}}`))
		case "/advisories.json":
			require.Equal(t, http.MethodPost, r.Method)
			require.NoError(t, r.ParseForm())
			assert.Contains(t, r.PostForm["packages[]"], "acme/lib")
			_, _ = w.Write([]byte(`{"advisories":{"acme/lib":[
				{"advisoryId":"PKSA-1","packageName":"acme/lib","affectedVersions":"<1.0.1","title":"XSS","cve":"CVE-1"}
			]}}`))
		default:
			http.NotFound(w, r)
		}
	}, nil)

	adv, err := repo.GetSecurityAdvisories(context.Background(), []string{"acme/lib"})
	require.NoError(t, err)
	require.Len(t, adv["acme/lib"], 1)
	assert.Equal(t, "CVE-1", adv["acme/lib"][0].CVE)
	assert.Equal(t, "<1.0.1", adv["acme/lib"][0].AffectedVersions)
}

func TestGetSecurityAdvisoriesNoneConfigured(t *testing.T) {
	repo, _ := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"metadata-url":"/p2/%package%.json"}`))
	}, nil)

	adv, err := repo.GetSecurityAdvisories(context.Background(), []string{"acme/lib"})
	require.NoError(t, err)
	assert.Empty(t, adv)
}
