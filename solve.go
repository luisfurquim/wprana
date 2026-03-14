//go:build js && wasm

package wprana

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// ── Acesso a campos e índices ─────────────────────────────────────────────────

// getField retorna o valor do campo key de obj.
// Suporta map[string]any, outros maps com chave string e structs via reflection.
func getField(obj any, key string) any {
	if obj == nil {
		return nil
	}
	switch v := obj.(type) {
	case map[string]any:
		return v[key]
	default:
		rv := reflect.ValueOf(obj)
		if rv.Kind() == reflect.Ptr {
			if rv.IsNil() {
				return nil
			}
			rv = rv.Elem()
		}
		switch rv.Kind() {
		case reflect.Map:
			kv := rv.MapIndex(reflect.ValueOf(key))
			if kv.IsValid() {
				return kv.Interface()
			}
			return nil
		case reflect.Struct:
			fv := rv.FieldByName(key)
			if fv.IsValid() && fv.CanInterface() {
				return fv.Interface()
			}
			return nil
		}
	}
	return nil
}

// setField atribui val ao campo key de obj (apenas map[string]any).
// Retorna true se bem-sucedido.
func setField(obj any, key string, val any) bool {
	if m, ok := obj.(map[string]any); ok {
		m[key] = val
		return true
	}
	return false
}

// getAt retorna o elemento do array obj no índice key (int ou string numérica).
func getAt(obj any, key any) any {
	if obj == nil {
		return nil
	}
	idx := toInt(key)
	switch v := obj.(type) {
	case []any:
		if idx >= 0 && idx < len(v) {
			return v[idx]
		}
		return nil
	default:
		rv := reflect.ValueOf(obj)
		if rv.Kind() == reflect.Ptr {
			if rv.IsNil() {
				return nil
			}
			rv = rv.Elem()
		}
		if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
			if idx >= 0 && idx < rv.Len() {
				return rv.Index(idx).Interface()
			}
		}
	}
	return nil
}

// toInt converte any em int: int, float64, string ou seus equivalentes.
func toInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(x)
		return n
	}
	return 0
}

// toStr converte any em string para exibição no DOM.
func toStr(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		s := strconv.FormatFloat(x, 'f', -1, 64)
		return s
	case []any:
		parts := make([]string, len(x))
		for i, el := range x {
			parts[i] = toStr(el)
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprintf("%v", v)
	}
}

// coerceToType converte a string s para o mesmo tipo de existing.
// Usado em elementAttrChanged para preservar o tipo original do dado
// (HTML attributes são sempre strings, mas o mapa de dados pode ter bool, int, etc.).
func coerceToType(s string, existing any) any {
	if existing == nil {
		return s
	}
	switch existing.(type) {
	case bool:
		return s == "true"
	case int:
		n, err := strconv.Atoi(s)
		if err != nil {
			return s
		}
		return n
	case int64:
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return s
		}
		return n
	case float64:
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return s
		}
		return f
	default:
		return s
	}
}

// ── Resolução de referências ──────────────────────────────────────────────────

// solve percorre a árvore de referência e resolve o valor contra ctx.
// Equivale ao solve() do JS original.
func solve(tree []RefNode, ctx any) any {
	if tree == nil {
		return ctx
	}

	sym := ctx

	for i := range tree {
		switch tree[i].Type {
		case TokStr, TokTxt:
			sym = tree[i].StrVal
		case TokNum:
			sym = tree[i].IntVal
		case TokIdent:
			sym = getField(sym, tree[i].StrVal)
		case TokExpr:
			key := solve(tree[i].Sub, ctx)
			sym = getAt(sym, key)
		}
	}

	return sym
}

// solveAll percorre todos os segmentos, resolve referências e concatena.
// Busca em ctx (pilha de contextos) até encontrar um valor não-nil.
// Equivale ao solveAll() do JS original.
func solveAll(segs []TextSegment, ctx Ctx) string {
	var sb strings.Builder

	for i := range segs {
		if !segs[i].IsRef {
			sb.WriteString(segs[i].Lit)
			continue
		}
		// Busca na pilha de contextos
		var val any
		for j := range ctx {
			val = solve(segs[i].Ref, ctx[j])
			if val != nil {
				break
			}
		}
		sb.WriteString(toStr(val))
	}

	return sb.String()
}

// isPureReference retorna true se a árvore não contém literais (str/num/txt),
// ou seja, é um caminho puro de identifiers/expr (útil para two-way binding).
// Equivale ao isPureReference() do JS original.
func isPureReference(tree []RefNode) bool {
	for i := range tree {
		switch tree[i].Type {
		case TokStr, TokNum, TokTxt:
			return false
		}
	}
	return true
}

// isPureSegs retorna true se há exatamente um segmento IsRef com referência pura.
func isPureSegs(segs []TextSegment) bool {
	if len(segs) != 1 || !segs[0].IsRef {
		return false
	}
	return isPureReference(segs[0].Ref)
}

// ── Resolução para atribuição (two-way binding) ───────────────────────────────

// refOf encontra o par (container, chave) para atribuição via two-way binding.
// Retorna (nil, "") se não encontrado.
// Equivale ao refOf() do JS original.
func refOf(tree []RefNode, ctx Ctx) (container any, key string) {
	if len(tree) == 0 || len(ctx) == 0 {
		return nil, ""
	}

	var sym any
	var nextKey any

	sym = ctx[0]

	for i := range tree {
		if nextKey != nil {
			sym = getField(sym, toStr(nextKey))
			if sym == nil {
				// Tenta outros contextos da pilha
				for j := 1; j < len(ctx); j++ {
					sub, k := refOf(tree[i:], ctx[j:])
					if sub != nil && k != "" {
						return sub, k
					}
				}
				return nil, ""
			}
		}

		switch tree[i].Type {
		case TokIdent:
			nextKey = tree[i].StrVal
		case TokStr:
			nextKey = tree[i].StrVal
		case TokNum:
			nextKey = tree[i].IntVal
		case TokExpr:
			resolved := solve(tree[i].Sub, ctx[0])
			nextKey = resolved
		}
	}

	if nextKey == nil {
		return nil, ""
	}

	return sym, toStr(nextKey)
}
