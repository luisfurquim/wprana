//go:build js && wasm

package wprana

import (
	"strings"
	"syscall/js"
)

// hashFragment holds the current value of window.location.hash (without the leading '#').
// Available in templates as {{#}}.
var hashFragment string

// initHash reads the initial hash from window.location.href and installs
// a "hashchange" listener on window to keep hashFragment in sync.
// Called once from the package init in prana.go.
func initHash() {
	// Read initial hash
	hashFragment = readHash()
	G.Logf(3, "initHash: fragment=%q\n", hashFragment)

	// Listen for hash changes
	jsGlobal.Call("addEventListener", "hashchange", js.FuncOf(func(this js.Value, args []js.Value) any {
		hashFragment = readHash()
		G.Logf(3, "hashchange: fragment=%q\n", hashFragment)
		syncAllInstances()
		return nil
	}))
}

// readHash extracts the fragment from window.location.hash, stripping the leading '#'.
func readHash() string {
	h := jsGlobal.Get("location").Get("hash").String()
	return strings.TrimPrefix(h, "#")
}

// GoTo sets window.location.hash to the given fragment.
// This triggers the "hashchange" event, which in turn updates hashFragment
// and syncs all component instances.
func GoTo(frag string) {
	jsGlobal.Get("location").Set("hash", frag)
}

// syncAllInstances triggers a sync on every live component instance,
// so that {{#}} references are updated everywhere.
func syncAllInstances() {
	syncEpoch++
	syncDepth++
	for _, instances := range instanceRegistry {
		for _, self := range instances {
			nodeID, ok := getNodeID(self)
			if !ok {
				continue
			}
			st, found := nodeRegistry[nodeID]
			if !found || st.State == nil {
				continue
			}
			st.State.syncLocal(nil)
		}
	}
	syncDepth--
}
