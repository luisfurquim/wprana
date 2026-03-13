//go:build js && wasm

// Package mywidget demonstra como implementar um web component Go usando wprana.
//
// Estrutura de um módulo wprana:
//   - mywidget.html   → template HTML com bindings {{expr}}, *arr, ?cond, &attr
//   - mywidget.css    → estilos do componente
//   - mywidget.go     → lógica Go: init() registra, MyWidget implementa PranaMod
package mywidget

import (
	_ "embed"

	"github.com/luisfurquim/goose"
	"github.com/luisfurquim/wprana"
)

// G é o logger deste módulo.
var G goose.Alert

//go:embed mywidget.html
var htmlContent string

//go:embed mywidget.css
var cssContent string

// Item é um elemento da lista.
type Item struct {
	Label string
}

// MyWidget implementa PranaMod para o custom element <my-widget>.
type MyWidget struct{}

// init registra o módulo. Equivale ao function main(ready) do JS original:
// executa uma vez no carregamento do pacote.
func init() {
	G.Set(4)
	wprana.Register(
		"my-widget",
		htmlContent,
		cssContent,
		func() wprana.PranaMod { return &MyWidget{} },
		// atributos observados (equivalente a observedAttributes no JS):
		"title",
	)
	G.Printf(2, "mywidget: módulo registrado\n")
}

// InitData retorna o estado inicial do componente.
// Equivale ao "return {...}" do function main(ready) no JS original.
func (w *MyWidget) InitData() map[string]any {
	return map[string]any{
		"title":     "Meu Widget",
		"count":     0,
		"items":     []any{},
		"showExtra": false,
		"extra":     "",
		"inputVal":  "",
	}
}

// Render é chamado após o componente ser conectado ao DOM e os dados
// iniciais estarem disponíveis. Equivale ao ready.then(function(obj){...})
// do JS original.
//
// obj.This   → *wprana.ReactiveData  (use Set/Get/Delete/Append/DeleteAt)
// obj.Dom    → js.Value (SPAN na shadow root)
// obj.Element → js.Value (o custom element em si)
// obj.Trigger → func(eventName string, args ...any)
func (w *MyWidget) Render(obj *wprana.PranaObj) {
	G.Printf(2, "mywidget: Render chamado\n")

	// Inicializa com alguns itens
	obj.This.Set("items", []any{
		map[string]any{"label": "Item Alpha"},
		map[string]any{"label": "Item Beta"},
		map[string]any{"label": "Item Gamma"},
	})

	// Incrementa contador a cada 2 segundos como demonstração
	go func() {
		for {
			done := make(chan struct{})
			wprana.JSGlobal().Call("setTimeout", wprana.JSFuncOnce(func() {
				count := obj.This.Get("count")
				if n, ok := count.(int); ok {
					obj.This.Set("count", n+1)
					G.Printf(5, "mywidget: count=%d\n", n+1)
				}
				close(done)
			}), 2000)
			<-done
		}
	}()

	// Lê o valor do input à medida que muda (two-way binding já cuida disso,
	// mas aqui mostramos como reagir a mudanças via polling do modelo)
	G.Printf(3, "mywidget: Render concluído\n")
}
