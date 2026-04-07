package origin

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Normalize returns the normalized origin string (scheme://host[:port]) or false if invalid.
func Normalize(origin string) (string, bool) {
	trimmed := strings.TrimSpace(origin)
	if trimmed == "" {
		return "", false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", false
	}

	scheme := strings.ToLower(parsed.Scheme)
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return "", false
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	port := parsed.Port()
	if port != "" {
		if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
			port = ""
		}
	}
	if port != "" {
		host = host + ":" + port
	}

	return scheme + "://" + host, true
}

// BuildAllowlist builds the websocket origin allowlist from the provided slice.
// It returns (allowAll, allowedSet, errors).  When the only non-empty entry is
// "*" it returns (true, nil, nil) indicating all origins are permitted.
// Errors are collected for invalid entries but valid entries are still kept.
func BuildAllowlist(origins []string) (bool, map[string]struct{}, []error) {
	allowed := make(map[string]struct{})
	var errs []error
	if len(origins) == 0 {
		return false, allowed, nil
	}

	trimmed := make([]string, 0, len(origins))
	for _, origin := range origins {
		value := strings.TrimSpace(origin)
		if value == "" {
			continue
		}
		trimmed = append(trimmed, value)
	}

	if len(trimmed) == 0 {
		return false, allowed, nil
	}

	if len(trimmed) == 1 && trimmed[0] == "*" {
		return true, nil, nil
	}

	for _, origin := range trimmed {
		if origin == "*" {
			errs = append(errs, errors.New("ws-origins '*' must be the only entry"))
			continue
		}
		normalized, ok := Normalize(origin)
		if !ok {
			errs = append(errs, fmt.Errorf("invalid ws-origin %q", origin))
			continue
		}
		if _, exists := allowed[normalized]; exists {
			continue
		}
		allowed[normalized] = struct{}{}
	}

	return false, allowed, errs
}

// IsAllowed reports whether the given origin header value is permitted.
// An empty origin is always allowed (non-browser clients).  If allowAll is
// true every origin is accepted.  Otherwise the origin is normalised and
// looked up in allowlist.
func IsAllowed(origin string, allowAll bool, allowlist map[string]struct{}) bool {
	if origin == "" {
		return true
	}
	if allowAll {
		return true
	}
	if len(allowlist) == 0 {
		return false
	}

	normalized, ok := Normalize(origin)
	if !ok {
		return false
	}

	_, ok = allowlist[normalized]
	return ok
}
