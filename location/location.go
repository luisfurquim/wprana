//go:build js && wasm

// Package location provides helpers for reading browser location URLs.
package location

import (
	"net/url"

	"github.com/luisfurquim/wprana"
)

var jsGlobal = wprana.JSGlobal()

// Get returns window.location.href as *url.URL.
func Get() (*url.URL, error) {
	href := jsGlobal.Get("location").Get("href").String()
	return url.Parse(href)
}

// GetTop returns top.location.href as *url.URL.
// Useful for detecting the real URL when inside an iframe.
func GetTop() (*url.URL, error) {
	href := jsGlobal.Get("top").Get("location").Get("href").String()
	return url.Parse(href)
}
