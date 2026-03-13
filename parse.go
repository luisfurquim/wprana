//go:build js && wasm

package wprana

import (
	"fmt"
	"strconv"
)

// ── Parser de texto de template ───────────────────────────────────────────────

// parseText divide uma string de template em segmentos literais e referências.
// Referências são delimitadas por {{ e }}. Cada referência é imediatamente
// tokenizada e parseada em árvore RefNode.
func parseText(s string) ([]TextSegment, error) {
	var segs []TextSegment
	i, start := 0, 0
	inRef := false

	for i < len(s) {
		if !inRef {
			if i+1 < len(s) && s[i] == '{' && s[i+1] == '{' {
				if i > start {
					segs = append(segs, TextSegment{Lit: s[start:i]})
				}
				i += 2
				start = i
				inRef = true
				continue
			}
		} else {
			if i+1 < len(s) && s[i] == '}' && s[i+1] == '}' {
				expr := s[start:i]
				toks := tokenize(expr)
				ref, err := parseReference(&toks)
				if err != nil {
					return nil, fmt.Errorf("parseText: %w", err)
				}
				segs = append(segs, TextSegment{IsRef: true, Ref: ref})
				i += 2
				start = i
				inRef = false
				continue
			}
		}
		i++
	}

	if start < len(s) {
		segs = append(segs, TextSegment{Lit: s[start:]})
	}

	return segs, nil
}

// hasRef retorna true se algum segmento for uma referência.
func hasRef(segs []TextSegment) bool {
	for i := range segs {
		if segs[i].IsRef {
			return true
		}
	}
	return false
}

// isPureTextSegs retorna true se todos os segmentos forem literais (nenhuma referência).
func isPureTextSegs(segs []TextSegment) bool {
	return !hasRef(segs)
}

// ── Tokenizador de expressões de referência ───────────────────────────────────

// preToken é usado internamente por splitStrings.
type preToken struct {
	isStr bool
	val   string
}

// splitStrings separa strings literais entre aspas do restante do código.
// Equivale ao parseString do JS original.
func splitStrings(s string) []preToken {
	var result []preToken
	i, start := 0, 0
	inStr := false
	var delim byte

	for i < len(s) {
		if !inStr {
			if s[i] == '\'' || s[i] == '"' {
				if i > start {
					result = append(result, preToken{val: s[start:i]})
				}
				delim = s[i]
				inStr = true
				start = i + 1
			}
		} else if s[i] == delim {
			result = append(result, preToken{isStr: true, val: s[start:i]})
			inStr = false
			start = i + 1
		}
		i++
	}

	if start < len(s) {
		result = append(result, preToken{val: s[start:]})
	}

	return result
}

// splitSymbols tokeniza um fragmento de código sem strings literais.
// Reconhece: `.`, `[`, `]`, números, identificadores.
func splitSymbols(s string) []RefNode {
	var toks []RefNode
	i := 0
	n := len(s)

	for i < n {
		c := s[i]

		// Whitespace
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}

		switch c {
		case '.':
			toks = append(toks, RefNode{Type: TokDot})
			i++
			continue
		case '[':
			toks = append(toks, RefNode{Type: TokOpen})
			i++
			continue
		case ']':
			toks = append(toks, RefNode{Type: TokClose})
			i++
			continue
		}

		// Número com sinal opcional
		isSign := (c == '+' || c == '-') && i+1 < n && s[i+1] >= '0' && s[i+1] <= '9'
		isDigit := c >= '0' && c <= '9'

		if isSign || isDigit {
			j := i
			if isSign {
				j++
			}
			for j < n && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			// parte decimal (armazenamos apenas a parte inteira via parseInt)
			if j < n && s[j] == '.' {
				j++
				for j < n && s[j] >= '0' && s[j] <= '9' {
					j++
				}
			}
			// parseInt replica o comportamento do JS (trunca fração)
			intStr := s[i:j]
			if idx := indexByte(intStr, '.'); idx >= 0 {
				intStr = intStr[:idx]
			}
			val, _ := strconv.Atoi(intStr)
			toks = append(toks, RefNode{Type: TokNum, IntVal: val})
			i = j
			continue
		}

		// Identificador: [a-zA-Z_$][0-9a-zA-Z_$]*
		if c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			j := i + 1
			for j < n {
				d := s[j]
				if d == '_' || d == '$' || (d >= '0' && d <= '9') || (d >= 'a' && d <= 'z') || (d >= 'A' && d <= 'Z') {
					j++
				} else {
					break
				}
			}
			toks = append(toks, RefNode{Type: TokIdent, StrVal: s[i:j]})
			i = j
			continue
		}

		// Caractere desconhecido: ignora
		i++
	}

	return toks
}

// indexByte encontra o primeiro índice de b em s, ou -1.
func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// tokenize converte uma expressão de referência em lista de RefNodes.
// Equivale ao tokenize() do JS: primeiro extrai strings literais,
// depois tokeniza os demais fragmentos.
func tokenize(s string) []RefNode {
	var toks []RefNode

	preToks := splitStrings(s)
	for _, pt := range preToks {
		if pt.isStr {
			toks = append(toks, RefNode{Type: TokStr, StrVal: pt.val})
		} else {
			toks = append(toks, splitSymbols(pt.val)...)
		}
	}

	return toks
}

// ── Parser de referência ──────────────────────────────────────────────────────

// parseReference constrói a árvore de referência a partir de uma lista de tokens.
// Consome tokens do início da fatia apontada por toks (equivale ao splice do JS).
// Equivale ao parseReference() do JS original.
func parseReference(toks *[]RefNode) ([]RefNode, error) {
	if len(*toks) == 0 {
		return nil, fmt.Errorf("parseReference: referência vazia")
	}

	token := popRef(toks)

	// Literal isolado (num ou str): retorna imediatamente
	if token.Type == TokNum || token.Type == TokStr {
		if len(*toks) == 0 || (*toks)[0].Type == TokClose {
			if len(*toks) > 0 {
				popRef(toks) // consome o ']'
			}
			return []RefNode{token}, nil
		}
	}

	if token.Type != TokIdent {
		return nil, fmt.Errorf("parseReference: esperado identificador, encontrado tipo=%d val=%q", token.Type, token.StrVal)
	}

	tree := []RefNode{token}

	// Estado: stWSep = aguarda separador (. ou [); stRef = aguarda ident/str
	const stWSep, stRef = 0, 1
	stat := stWSep

	for len(*toks) > 0 && (*toks)[0].Type != TokClose {
		token = popRef(toks)

		if stat == stWSep {
			if token.Type == TokOpen {
				sub, err := parseReference(toks)
				if err != nil {
					return nil, err
				}
				tree = append(tree, RefNode{Type: TokExpr, Sub: sub})
				continue
			} else if token.Type != TokDot {
				return nil, fmt.Errorf("parseReference: esperado '.' ou '[', encontrado tipo=%d", token.Type)
			}
			stat = stRef
		} else { // stRef
			if token.Type == TokIdent || token.Type == TokStr {
				tree = append(tree, token)
				stat = stWSep
				continue
			}
			return nil, fmt.Errorf("parseReference: esperado identificador, encontrado tipo=%d", token.Type)
		}
	}

	// Consome ']' de fechamento se presente
	if len(*toks) > 0 && (*toks)[0].Type == TokClose {
		popRef(toks)
	}

	return tree, nil
}

// popRef remove e retorna o primeiro token da fatia.
func popRef(toks *[]RefNode) RefNode {
	t := (*toks)[0]
	*toks = (*toks)[1:]
	return t
}
