//go:build js && wasm

package main

import (
	"github.com/luisfurquim/wprana"

	// Side-effect imports: each init() registers the module via wprana.Register()
	_ "live-demo/mod/mywidget"
)

func main() {
	wprana.Main()
}
