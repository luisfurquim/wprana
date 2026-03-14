//go:build js && wasm

// Package localstorage provides typed access to browser localStorage
// with pluggable serialization.
package localstorage

import (
	"errors"
	"syscall/js"

	"github.com/luisfurquim/wprana"
)

// ErrKeyNotFound é retornado por LS.Get quando a chave não existe no localStorage.
var ErrKeyNotFound = errors.New("localstorage: key not found")

var storage = wprana.JSGlobal().Get("localStorage")

// Encoder codifica um valor Go para string (para gravar no localStorage).
type Encoder interface {
	Encode(inpval any) string
}

// Decoder decodifica uma string do localStorage para um valor Go.
// outval deve ser um ponteiro para o tipo destino.
type Decoder interface {
	Decode(buf string, outval any) error
}

// LS encapsula o acesso ao localStorage com serialização configurável.
type LS struct {
	enc Encoder
	dec Decoder
	st  js.Value
}

// New cria um wrapper de localStorage com o encoder/decoder fornecidos.
func New(enc Encoder, dec Decoder) *LS {
	return &LS{
		enc: enc,
		dec: dec,
		st:  storage,
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
		return ErrKeyNotFound
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
