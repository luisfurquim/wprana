//go:build js && wasm

// Package main é o ponto de entrada do binário WASM.
// Importa todos os módulos (side-effect imports para acionar seus init())
// e então chama wprana.Main() que define os custom elements e bloqueia.
package main

import (
	"github.com/luisfurquim/wprana"

	// Side-effect imports: cada init() registra o módulo via wprana.Register()
	_ "github.com/luisfurquim/wprana/example/mywidget"
)

func main() {
	// Define todos os custom elements registrados e mantém o WASM ativo.
	wprana.Main()
}
