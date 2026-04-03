//go:build js && wasm

package wprana

import "syscall/js"

// JSGlobal returns js.Global() for use in subpackages and modules.
func JSGlobal() js.Value {
	return jsGlobal
}

// JSFuncOnce creates a js.Func that auto-releases after being called once.
// Useful for setTimeout/setInterval callbacks without memory leaks.
func JSFuncOnce(fn func()) js.Func {
	var f js.Func
	f = js.FuncOf(func(this js.Value, args []js.Value) any {
		fn()
		f.Release()
		return nil
	})
	return f
}

// JSFunc creates a js.Func that remains active until manually released.
// The caller is responsible for calling Release() when no longer needed.
func JSFunc(fn func(this js.Value, args []js.Value) any) js.Func {
	return js.FuncOf(fn)
}
