package updater

import (
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// systemProxyFunc returns a ProxyFunc that checks Windows Internet Settings
// (the same proxy the browser uses) when no environment variable proxy is set.
func systemProxyFunc(req *http.Request) (*url.URL, error) {
	// First try environment variables (HTTP_PROXY / HTTPS_PROXY).
	envProxy, err := http.ProxyFromEnvironment(req)
	if err == nil && envProxy != nil {
		return envProxy, nil
	}

	// Fall back to Windows Internet Settings registry key.
	return windowsProxy(req.URL)
}

// windowsProxy reads HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings
// and returns the system proxy if enabled.
func windowsProxy(target *url.URL) (*url.URL, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
		registry.QUERY_VALUE)
	if err != nil {
		return nil, nil
	}
	defer key.Close()

	enabled, _, err := key.GetIntegerValue("ProxyEnable")
	if err != nil || enabled == 0 {
		return nil, nil
	}

	server, _, err := key.GetStringValue("ProxyServer")
	if err != nil || server == "" {
		return nil, nil
	}

	// ProxyServer may contain protocol-specific entries like:
	//   http=127.0.0.1:7897;https=127.0.0.1:7897;ftp=...
	// or a single address: 127.0.0.1:7897
	addr := parseProxyServer(server, target.Scheme)
	if addr == "" {
		return nil, nil
	}

	// Ensure the proxy URL has a scheme.
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") && !strings.HasPrefix(addr, "socks") {
		addr = "http://" + addr
	}
	return url.Parse(addr)
}

// parseProxyServer handles both "host:port" and "proto=host:port;..." formats.
func parseProxyServer(server, scheme string) string {
	if !strings.Contains(server, "=") {
		return strings.TrimSpace(server)
	}
	// Parse "http=host:port;https=host:port;..." format.
	for _, part := range strings.Split(server, ";") {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			key := strings.TrimSpace(kv[0])
			val := strings.TrimSpace(kv[1])
			if key == scheme || (scheme == "https" && key == "https") || (scheme == "http" && key == "http") {
				return val
			}
		}
	}
	// Fallback: return the last entry that doesn't have a protocol prefix.
	for i := len(server) - 1; i >= 0; i-- {
		if server[i] == ';' {
			return strings.TrimSpace(server[i+1:])
		}
	}
	return strings.TrimSpace(server)
}
