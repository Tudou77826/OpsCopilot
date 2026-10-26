//go:build !windows

package updater

import (
	"net/http"
	"net/url"
)

func systemProxyFunc(req *http.Request) (*url.URL, error) {
	return http.ProxyFromEnvironment(req)
}
