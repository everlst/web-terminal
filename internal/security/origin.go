package security

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
)

func ValidOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if !strings.EqualFold(parsed.Scheme, requestScheme(r)) {
		return false
	}
	for _, host := range requestHosts(r) {
		if strings.EqualFold(parsed.Host, host) {
			return true
		}
	}
	return false
}

func IsHTTPS(r *http.Request) bool {
	return requestScheme(r) == "https"
}

func requestScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if value := firstHeaderValue(r.Header.Get("X-Forwarded-Proto")); value == "http" || value == "https" {
		return value
	}
	var visitor struct {
		Scheme string `json:"scheme"`
	}
	if json.Unmarshal([]byte(r.Header.Get("CF-Visitor")), &visitor) == nil {
		if visitor.Scheme == "http" || visitor.Scheme == "https" {
			return visitor.Scheme
		}
	}
	return "http"
}

func requestHosts(r *http.Request) []string {
	hosts := []string{strings.TrimSpace(r.Host)}
	if forwarded := firstHeaderValue(r.Header.Get("X-Forwarded-Host")); forwarded != "" {
		hosts = append(hosts, forwarded)
	}
	return hosts
}

func firstHeaderValue(value string) string {
	if index := strings.IndexByte(value, ','); index >= 0 {
		value = value[:index]
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func ClientIP(r *http.Request, trustCloudflare bool) string {
	if trustCloudflare {
		if value := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); net.ParseIP(value) != nil {
			return value
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
