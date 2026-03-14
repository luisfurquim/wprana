//go:build js && wasm

package wprana

import (
	"net/url"
	"syscall/js"
)

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

// ── Location helpers ────────────────────────────────────────────────────────

// GetLocation retorna window.location.href como *url.URL.
func GetLocation() (*url.URL, error) {
	href := jsGlobal.Get("location").Get("href").String()
	return url.Parse(href)
}

// GetTopLocation retorna top.location.href como *url.URL.
func GetTopLocation() (*url.URL, error) {
	href := jsGlobal.Get("top").Get("location").Get("href").String()
	return url.Parse(href)
}

// ── Event helpers ────────────────────────────────────────────────────────────

// eventEntry guarda os dados necessários para remover um event listener.
type eventEntry struct {
	target    js.Value
	eventName string
	fn        js.Func
}

// eventRegistry mapeia IDs de handlers registrados via AddEvent.
var (
	eventRegistry = map[int64]*eventEntry{}
	nextEventID   int64 = 1
)

// AddEvent registra um event listener no elemento dom para o evento eventName.
// Se preventDefault for true, chama event.preventDefault() antes do handler.
// Se stopPropagation for true, chama event.stopPropagation() antes do handler.
// Retorna um ID que pode ser passado a RmEvent para remover o listener.
func AddEvent(dom js.Value, eventName string, handler func(this js.Value, args []js.Value) any, preventDefault, stopPropagation bool) int64 {
	wrapped := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 {
			if preventDefault {
				args[0].Call("preventDefault")
			}
			if stopPropagation {
				args[0].Call("stopPropagation")
			}
		}
		return handler(this, args)
	})

	dom.Call("addEventListener", eventName, wrapped)

	id := nextEventID
	nextEventID++
	eventRegistry[id] = &eventEntry{
		target:    dom,
		eventName: eventName,
		fn:        wrapped,
	}
	return id
}

// RmEvent remove o event listener registrado com o ID retornado por AddEvent.
func RmEvent(id int64) {
	entry, ok := eventRegistry[id]
	if !ok {
		return
	}
	entry.target.Call("removeEventListener", entry.eventName, entry.fn)
	entry.fn.Release()
	delete(eventRegistry, id)
}

// ── Query helper ─────────────────────────────────────────────────────────────

// Query executa querySelectorAll no elemento dom e retorna os resultados
// como um slice de js.Value.
func Query(dom js.Value, selector string) []js.Value {
	nodeList := dom.Call("querySelectorAll", selector)
	n := nodeList.Get("length").Int()
	result := make([]js.Value, n)
	for i := 0; i < n; i++ {
		result[i] = nodeList.Index(i)
	}
	return result
}

// ── Timer helpers ────────────────────────────────────────────────────────────

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

// Sleep bloqueia a goroutine atual por ms milissegundos, cedendo o controle
// ao event loop JavaScript enquanto aguarda.
func Sleep(ms int) {
	done := make(chan struct{})
	jsGlobal.Call("setTimeout", JSFuncOnce(func() {
		close(done)
	}), ms)
	<-done
}

type Ticker struct{
	id js.Value
	fn js.Func
	Tick chan struct{}
}


// NewTicker retorna um channel que recebe um struct{}{} a cada ms milissegundos
// e uma função stop() para interromper o ticker e liberar recursos.
func NewTicker(ms int) *Ticker {
	var tk Ticker

	tk.Tick = make(chan struct{}, 1)

	tk.fn = js.FuncOf(func(this js.Value, args []js.Value) any {
		select {
		case tk.Tick <- struct{}{}:
		default:
		}
		return nil
	})

	tk.id = jsGlobal.Call(
		"setInterval",
		tk.fn,
		ms,
	)
	return &tk
}

func (tk *Ticker) Stop() {
	jsGlobal.Call("clearInterval", tk.id)
	tk.fn.Release()
	close(tk.Tick)
}

// ── LocalStorage helpers ────────────────────────────────────────────────────

// LSEncoder codifica um valor Go para string (para gravar no localStorage).
type LSEncoder interface {
	Encode(inpval any) string
}

// LSDecoder decodifica uma string do localStorage para um valor Go.
// outval deve ser um ponteiro para o tipo destino.
type LSDecoder interface {
	Decode(buf string, outval any) error
}

// LS encapsula o acesso ao localStorage com serialização configurável.
type LS struct {
	enc LSEncoder
	dec LSDecoder
	st  js.Value
}

// NewLS cria um wrapper de localStorage com o encoder/decoder fornecidos.
func NewLS(enc LSEncoder, dec LSDecoder) *LS {
	return &LS{
		enc: enc,
		dec: dec,
		st:  jsGlobal.Get("localStorage"),
	}
}

// Set grava key no localStorage usando o encoder configurado.
func (ls *LS) Set(key string, val any) {
	ls.st.Call("setItem", key, ls.enc.Encode(val))
}

// Get lê key do localStorage e decodifica em outval.
// outval deve ser ponteiro para o tipo destino.
// Retorna erro se a chave não existir ou se o decode falhar.
func (ls *LS) Get(key string, outval any) error {
	v := ls.st.Call("getItem", key)
	if v.IsNull() || v.IsUndefined() {
		return ErrLSKeyNotFound
	}
	return ls.dec.Decode(v.String(), outval)
}

// Del remove key do localStorage.
func (ls *LS) Del(key string) {
	ls.st.Call("removeItem", key)
}

// Clear remove todas as chaves do localStorage.
func (ls *LS) Clear() {
	ls.st.Call("clear")
}

// Key retorna o nome da chave no índice index.
// Retorna ("", false) se o índice estiver fora do intervalo.
func (ls *LS) Key(index int) (string, bool) {
	v := ls.st.Call("key", index)
	if v.IsNull() || v.IsUndefined() {
		return "", false
	}
	return v.String(), true
}

// Len retorna o número de chaves no localStorage.
func (ls *LS) Len() int {
	return ls.st.Get("length").Int()
}


