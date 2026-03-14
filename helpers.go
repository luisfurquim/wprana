//go:build js && wasm

package wprana

import "syscall/js"

// JSGlobal retorna js.Global() para uso nos subpacotes e módulos.
func JSGlobal() js.Value {
	return jsGlobal
}

// JSFuncOnce cria um js.Func que se auto-libera após ser chamado uma vez.
// Útil para callbacks de setTimeout/setInterval sem vazamento de memória.
func JSFuncOnce(fn func()) js.Func {
	var f js.Func
	f = js.FuncOf(func(this js.Value, args []js.Value) any {
		fn()
		f.Release()
		return nil
	})
	return f
}

// JSFunc cria um js.Func que permanece ativo até ser liberado manualmente.
// O chamador é responsável por chamar Release() quando não precisar mais.
func JSFunc(fn func(this js.Value, args []js.Value) any) js.Func {
	return js.FuncOf(fn)
}
