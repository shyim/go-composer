package repository

import (
	"context"
	"net/http"
	"strconv"
	"strings"
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
				{"advisoryId":"PKSA-1","packageName":"acme/lib","affectedVersions":"<1.0.1","title":"XSS","cve":"CVE-1","severity":"medium","sources":[{"name":"GitHub","remoteId":"GHSA-1"}],"reportedAt":"2024-01-01 00:00:00"}
			]}}`))
		default:
			http.NotFound(w, r)
		}
	}, nil)

	adv, err := repo.GetSecurityAdvisories(context.Background(), []string{"acme/lib"})
	require.NoError(t, err)
	require.Len(t, adv.Package("acme/lib"), 1)
	assert.Equal(t, "CVE-1", adv.Package("acme/lib")[0].CVE)
	assert.Equal(t, "<1.0.1", adv.Package("acme/lib")[0].AffectedVersions)
	assert.Equal(t, "acme/lib", adv.Package("acme/lib")[0].PackageName)
	assert.Equal(t, "medium", adv.Package("acme/lib")[0].Severity)
	require.Len(t, adv.Package("acme/lib")[0].Sources, 1)
	assert.Equal(t, "GHSA-1", adv.Package("acme/lib")[0].Sources[0].RemoteID)
	assert.Equal(t, 1, adv.Len())
}

func TestGetSecurityAdvisoriesMetadataOnly(t *testing.T) {
	repo, _ := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/packages.json":
			_, _ = w.Write([]byte(`{"metadata-url":"/p2/%package%.json","security-advisories":{"metadata":true}}`))
		case "/p2/acme/lib.json":
			_, _ = w.Write([]byte(`{
				"packages":{"acme/lib":[{"name":"acme/lib","version":"1.0.0"}]},
				"security-advisories":[
					{"advisoryId":"PKSA-2","affectedVersions":"<1.0.1"}
				]
			}`))
		default:
			http.NotFound(w, r)
		}
	}, nil)

	adv, err := repo.GetSecurityAdvisories(context.Background(), []string{"acme/lib"})
	require.NoError(t, err)
	require.Len(t, adv.Package("acme/lib"), 1)
	assert.Equal(t, "PKSA-2", adv.Package("acme/lib")[0].AdvisoryID)
	assert.Equal(t, "acme/lib", adv.Package("acme/lib")[0].PackageName)
	assert.Equal(t, "<1.0.1", adv.Package("acme/lib")[0].AffectedVersions)
}

func TestGetSecurityAdvisoriesNoneConfigured(t *testing.T) {
	repo, _ := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"metadata-url":"/p2/%package%.json"}`))
	}, nil)

	adv, err := repo.GetSecurityAdvisories(context.Background(), []string{"acme/lib"})
	require.NoError(t, err)
	assert.Empty(t, adv)
	assert.Equal(t, 0, adv.Len())
}

func TestGetSecurityAdvisoriesRespectsAvailablePackages(t *testing.T) {
	// Package not advertised → never asked of the API.
	apiHits := 0
	repo, _ := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/packages.json":
			_, _ = w.Write([]byte(`{"metadata-url":"/p2/%package%.json","available-packages":["other/pkg"],"security-advisories":{"api-url":"/advisories.json"}}`))
		case "/advisories.json":
			apiHits++
			_, _ = w.Write([]byte(`{"advisories":{}}`))
		default:
			http.NotFound(w, r)
		}
	}, nil)

	adv, err := repo.GetSecurityAdvisories(context.Background(), []string{"acme/lib"})
	require.NoError(t, err)
	assert.Empty(t, adv)
	assert.Equal(t, 0, apiHits)
}

func TestSetGetSecurityAdvisoriesMerges(t *testing.T) {
	a, _ := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/packages.json":
			_, _ = w.Write([]byte(`{"metadata-url":"/p2/%package%.json","security-advisories":{"api-url":"/adv"}}`))
		case "/adv":
			_, _ = w.Write([]byte(`{"advisories":{"acme/lib":[{"advisoryId":"A1","affectedVersions":"<1.0"}]}}`))
		default:
			http.NotFound(w, r)
		}
	}, nil)
	b, _ := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/packages.json":
			_, _ = w.Write([]byte(`{"metadata-url":"/p2/%package%.json","security-advisories":{"api-url":"/adv"}}`))
		case "/adv":
			_, _ = w.Write([]byte(`{"advisories":{"acme/lib":[{"advisoryId":"B1","affectedVersions":"<2.0"}],"acme/other":[{"advisoryId":"B2","affectedVersions":"*"}]}}`))
		default:
			http.NotFound(w, r)
		}
	}, nil)

	set := NewSet(a, b)
	adv, err := set.GetSecurityAdvisories(context.Background(), []string{"acme/lib", "acme/other"})
	require.NoError(t, err)
	require.Len(t, adv.Package("acme/lib"), 2)
	assert.Equal(t, "A1", adv.Package("acme/lib")[0].AdvisoryID)
	assert.Equal(t, "B1", adv.Package("acme/lib")[1].AdvisoryID)
	require.Len(t, adv.Package("acme/other"), 1)
	assert.Equal(t, "B2", adv.Package("acme/other")[0].AdvisoryID)
	assert.Equal(t, 3, adv.Len())
}

// simpleCheck is a tiny numeric "x.y.z" checker good enough for unit tests.
// Supports constraints like "<1.0.1", ">=6.7.0.0,<6.7.8.1" (comma = AND).
func simpleCheck(constraint, ver string) bool {
	parts := strings.Split(constraint, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || p == "*" {
			continue
		}
		op, num := splitOp(p)
		if !compareVersion(ver, op, num) {
			return false
		}
	}
	return true
}

func splitOp(c string) (op, num string) {
	for _, candidate := range []string{">=", "<=", "!=", ">", "<", "="} {
		if strings.HasPrefix(c, candidate) {
			return candidate, strings.TrimSpace(c[len(candidate):])
		}
	}
	return "=", c
}

func parseVer(s string) []int {
	s = strings.TrimPrefix(s, "v")
	fields := strings.Split(s, ".")
	out := make([]int, len(fields))
	for i, f := range fields {
		n, _ := strconv.Atoi(f)
		out[i] = n
	}
	return out
}

func cmpVer(a, b string) int {
	as, bs := parseVer(a), parseVer(b)
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		av, bv := 0, 0
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}

func compareVersion(ver, op, bound string) bool {
	c := cmpVer(ver, bound)
	switch op {
	case ">":
		return c > 0
	case ">=":
		return c >= 0
	case "<":
		return c < 0
	case "<=":
		return c <= 0
	case "!=":
		return c != 0
	default:
		return c == 0
	}
}

func TestSecurityAdvisoryAffects(t *testing.T) {
	a := SecurityAdvisory{
		AffectedVersions: "<6.6.10.15|>=6.7.0.0,<6.7.8.1",
	}

	t.Run("first branch matches", func(t *testing.T) {
		assert.True(t, a.Affects("6.6.10.14", simpleCheck))
	})
	t.Run("second branch matches", func(t *testing.T) {
		assert.True(t, a.Affects("6.7.5.0", simpleCheck))
	})
	t.Run("between branches does not match", func(t *testing.T) {
		assert.False(t, a.Affects("6.6.10.15", simpleCheck))
	})
	t.Run("above all branches does not match", func(t *testing.T) {
		assert.False(t, a.Affects("6.7.8.1", simpleCheck))
	})
	t.Run("version with v prefix", func(t *testing.T) {
		assert.True(t, a.Affects("v6.6.10.14", simpleCheck))
	})
	t.Run("double-pipe separator", func(t *testing.T) {
		b := SecurityAdvisory{AffectedVersions: "<1.0.0||>=2.0.0,<2.1.0"}
		assert.True(t, b.Affects("0.9.0", simpleCheck))
		assert.True(t, b.Affects("2.0.5", simpleCheck))
		assert.False(t, b.Affects("1.5.0", simpleCheck))
	})
	t.Run("nil check is not affected", func(t *testing.T) {
		assert.False(t, a.Affects("6.6.10.14", nil))
	})
	t.Run("empty version is not affected", func(t *testing.T) {
		assert.False(t, a.Affects("", simpleCheck))
	})
	t.Run("empty affected versions not affected", func(t *testing.T) {
		assert.False(t, SecurityAdvisory{}.Affects("1.0.0", simpleCheck))
	})
}

func TestAdvisoriesAffectingPackage(t *testing.T) {
	adv := Advisories{
		"acme/lib": {
			{Title: "A", AffectedVersions: "<6.6.10.15"},
			{Title: "B", AffectedVersions: ">=6.7.0.0,<6.7.8.1"},
			{Title: "C", AffectedVersions: ">=7.0.0.0"},
		},
	}

	matching := adv.AffectingPackage("acme/lib", "6.7.5.0", simpleCheck)
	require.Len(t, matching, 1)
	assert.Equal(t, "B", matching[0].Title)

	assert.Nil(t, adv.AffectingPackage("acme/lib", "6.7.5.0", nil))
	assert.Nil(t, adv.AffectingPackage("missing/pkg", "1.0.0", simpleCheck))
	// case-insensitive package lookup
	assert.Len(t, adv.AffectingPackage("Acme/Lib", "6.7.5.0", simpleCheck), 1)
}

func TestAdvisoriesAffecting(t *testing.T) {
	adv := Advisories{
		"acme/lib": {
			{Title: "XSS", AffectedVersions: "<1.0.1"},
			{Title: "SQLi", AffectedVersions: ">=2.0.0,<2.1.0"},
		},
		"acme/other": {
			{Title: "RCE", AffectedVersions: "<3.0.0"},
		},
	}
	versions := map[string]string{
		"acme/lib":   "1.0.0",
		"acme/other": "3.0.0", // not affected
	}

	out := adv.Affecting(versions, simpleCheck)
	require.Len(t, out, 1)
	require.Len(t, out.Package("acme/lib"), 1)
	assert.Equal(t, "XSS", out.Package("acme/lib")[0].Title)
	assert.Nil(t, out.Package("acme/other"))

	// Case-insensitive package name lookup for the installed map key.
	out = adv.Affecting(map[string]string{"Acme/Lib": "1.0.0"}, simpleCheck)
	require.Len(t, out.Package("Acme/Lib"), 1)

	assert.Empty(t, adv.Affecting(versions, nil))
	assert.Empty(t, Advisories{}.Affecting(versions, simpleCheck))
}
