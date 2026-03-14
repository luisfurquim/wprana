//go:build js && wasm

// Package location provides helpers for reading browser location URLs.
package location

import (
	"net/url"

	"github.com/luisfurquim/wprana"
)

var jsGlobal = wprana.JSGlobal()

// Get retorna window.location.href como *url.URL.
func Get() (*url.URL, error) {
	href := jsGlobal.Get("location").Get("href").String()
	return url.Parse(href)
}

// GetTop retorna top.location.href como *url.URL.
// Útil para detectar a URL real quando dentro de um iframe.
func GetTop() (*url.URL, error) {
	href := jsGlobal.Get("top").Get("location").Get("href").String()
	return url.Parse(href)
}
