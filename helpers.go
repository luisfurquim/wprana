//go:build js && wasm

package wprana

import "syscall/js"

// JSGlobal retorna js.Global() para uso nos pacotes de módulo.
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

// SetTimeout agenda fn para ser executado após delay milissegundos.
// Retorna um canal que fecha quando fn completar.
func SetTimeout(fn func(), delay int) <-chan struct{} {
	done := make(chan struct{})
	jsGlobal.Call("setTimeout", JSFuncOnce(func() {
		fn()
		close(done)
	}), delay)
	return done
}

// SetInterval agenda fn para ser executado a cada interval milissegundos.
// Retorna uma função cancel() que interrompe o intervalo.
func SetInterval(fn func(), interval int) (cancel func()) {
	var id js.Value
	f := js.FuncOf(func(this js.Value, args []js.Value) any {
		fn()
		return nil
	})
	id = jsGlobal.Call("setInterval", f, interval)
	return func() {
		jsGlobal.Call("clearInterval", id)
		f.Release()
	}
}
