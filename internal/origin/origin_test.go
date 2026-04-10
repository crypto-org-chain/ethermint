package origin

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantOut   string
		wantValid bool
	}{
		{name: "empty", input: "", wantValid: false},
		{name: "whitespaceOnly", input: "   ", wantValid: false},
		{name: "noScheme", input: "example.com", wantValid: false},
		{name: "noHost", input: "http://", wantValid: false},
		{name: "simple", input: "http://example.com", wantOut: "http://example.com", wantValid: true},
		{name: "uppercaseSchemeAndHost", input: "HTTP://EXAMPLE.COM", wantOut: "http://example.com", wantValid: true},
		{name: "trailingSlash", input: "http://example.com/", wantOut: "http://example.com", wantValid: true},
		{name: "withPath", input: "http://example.com/some/path", wantOut: "http://example.com", wantValid: true},
		{name: "leadingTrailingSpace", input: "  http://example.com  ", wantOut: "http://example.com", wantValid: true},
		{name: "httpDefaultPort80Stripped", input: "http://example.com:80", wantOut: "http://example.com", wantValid: true},
		{name: "httpsDefaultPort443Stripped", input: "https://example.com:443", wantOut: "https://example.com", wantValid: true},
		{name: "nonDefaultPortKept", input: "http://example.com:8080", wantOut: "http://example.com:8080", wantValid: true},
		{name: "httpsNonDefaultPort", input: "https://example.com:8443", wantOut: "https://example.com:8443", wantValid: true},
		{name: "websocketScheme", input: "ws://example.com", wantOut: "ws://example.com", wantValid: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Normalize(tc.input)
			require.Equal(t, tc.wantValid, ok)
			require.Equal(t, tc.wantOut, got)
		})
	}
}

func TestBuildAllowlist(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		allowAll, origins, errs := BuildAllowlist(nil)
		require.False(t, allowAll)
		require.Len(t, origins, 0)
		require.Empty(t, errs)
	})

	t.Run("allWhitespace", func(t *testing.T) {
		allowAll, origins, errs := BuildAllowlist([]string{"  ", ""})
		require.False(t, allowAll)
		require.Len(t, origins, 0)
		require.Empty(t, errs)
	})

	t.Run("star", func(t *testing.T) {
		allowAll, origins, errs := BuildAllowlist([]string{"*"})
		require.True(t, allowAll)
		require.Nil(t, origins)
		require.Empty(t, errs)
	})

	t.Run("starMixedWithOthers", func(t *testing.T) {
		allowAll, origins, errs := BuildAllowlist([]string{"*", "http://example.com"})
		require.False(t, allowAll)
		require.NotNil(t, origins)
		require.Len(t, origins, 1)
		require.NotEmpty(t, errs)
	})

	t.Run("normalizes", func(t *testing.T) {
		allowAll, origins, errs := BuildAllowlist([]string{" HTTP://Example.COM ", "http://example.com/"})
		require.False(t, allowAll)
		require.Len(t, origins, 1)
		_, ok := origins["http://example.com"]
		require.True(t, ok)
		require.Empty(t, errs)
	})

	t.Run("invalidOrigin", func(t *testing.T) {
		allowAll, origins, errs := BuildAllowlist([]string{"not a url"})
		require.False(t, allowAll)
		require.NotNil(t, origins)
		require.Len(t, origins, 0)
		require.NotEmpty(t, errs)
	})

	t.Run("mixValidAndInvalid", func(t *testing.T) {
		allowAll, origins, errs := BuildAllowlist([]string{"http://valid.example", "not-a-url"})
		require.False(t, allowAll)
		require.Len(t, origins, 1)
		_, ok := origins["http://valid.example"]
		require.True(t, ok)
		require.Len(t, errs, 1)
	})
}

func TestIsAllowed(t *testing.T) {
	t.Run("emptyOriginAlwaysAllowed", func(t *testing.T) {
		require.True(t, IsAllowed("", false, map[string]struct{}{}))
		require.True(t, IsAllowed("", true, nil))
	})

	t.Run("allowAll", func(t *testing.T) {
		require.True(t, IsAllowed("http://any.example", true, nil))
		require.True(t, IsAllowed("http://any.example", true, map[string]struct{}{}))
	})

	t.Run("emptyAllowlistBlocksNonEmpty", func(t *testing.T) {
		require.False(t, IsAllowed("http://example.com", false, map[string]struct{}{}))
	})

	t.Run("allowlistEnforced", func(t *testing.T) {
		allowlist := map[string]struct{}{"http://allowed.example": {}}
		require.True(t, IsAllowed("http://allowed.example", false, allowlist))
		require.True(t, IsAllowed("HTTP://ALLOWED.EXAMPLE:80/", false, allowlist))
		require.False(t, IsAllowed("http://blocked.example", false, allowlist))
	})

	t.Run("invalidOriginBlocked", func(t *testing.T) {
		allowlist := map[string]struct{}{"http://allowed.example": {}}
		require.False(t, IsAllowed("not-a-url", false, allowlist))
	})
}
