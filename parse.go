//go:build js && wasm

package wprana

import (
	"fmt"
	"strconv"
)

// ── Template text parser ────────────────────────────────────────────────────

// parseText splits a template string into literal segments and references.
// References are delimited by {{ and }}. Each reference is immediately
// tokenized and parsed into a RefNode tree.
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

// hasRef returns true if any segment is a reference.
func hasRef(segs []TextSegment) bool {
	for i := range segs {
		if segs[i].IsRef {
			return true
		}
	}
	return false
}

// isPureTextSegs returns true if all segments are literals (no references).
func isPureTextSegs(segs []TextSegment) bool {
	return !hasRef(segs)
}

// ── Reference expression tokenizer ──────────────────────────────────────────

// preToken is used internally by splitStrings.
type preToken struct {
	isStr bool
	val   string
}

// splitStrings separates quoted string literals from the rest of the code.
// Equivalent to the parseString from the original JS.
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

// isWhitespace returns true for space, tab, newline, carriage return.
func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// isIdentStart returns true for characters that can start an identifier.
func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// isIdentChar returns true for characters that can appear inside an identifier.
func isIdentChar(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

// consumeNumber reads a number (with optional sign and decimal part) from s
// starting at position i. Returns the RefNode and the new position.
func consumeNumber(s string, i int) (RefNode, int) {
	n := len(s)
	j := i
	if s[j] == '+' || s[j] == '-' {
		j++
	}
	for j < n && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	// decimal part (we only store the integer part via parseInt)
	if j < n && s[j] == '.' {
		j++
		for j < n && s[j] >= '0' && s[j] <= '9' {
			j++
		}
	}
	// parseInt replicates the JS behavior (truncates fraction)
	intStr := s[i:j]
	if idx := indexByte(intStr, '.'); idx >= 0 {
		intStr = intStr[:idx]
	}
	val, _ := strconv.Atoi(intStr)
	return RefNode{Type: TokNum, IntVal: val}, j
}

// consumeIdent reads an identifier from s starting at position i.
// Returns the RefNode and the new position.
func consumeIdent(s string, i int) (RefNode, int) {
	j := i + 1
	for j < len(s) && isIdentChar(s[j]) {
		j++
	}
	return RefNode{Type: TokIdent, StrVal: s[i:j]}, j
}

// splitSymbols tokenizes a code fragment without string literals.
// Recognizes: `.`, `[`, `]`, `#`, numbers, identifiers.
func splitSymbols(s string) []RefNode {
	var toks []RefNode
	i := 0
	n := len(s)

	for i < n {
		c := s[i]

		if isWhitespace(c) {
			i++
			continue
		}

		switch c {
		case '.':
			toks = append(toks, RefNode{Type: TokDot})
			i++
		case '[':
			toks = append(toks, RefNode{Type: TokOpen})
			i++
		case ']':
			toks = append(toks, RefNode{Type: TokClose})
			i++
		case '#':
			toks = append(toks, RefNode{Type: TokIdent, StrVal: "#"})
			i++
		default:
			isSign := (c == '+' || c == '-') && i+1 < n && s[i+1] >= '0' && s[i+1] <= '9'
			switch {
			case isSign || (c >= '0' && c <= '9'):
				var tok RefNode
				tok, i = consumeNumber(s, i)
				toks = append(toks, tok)
			case isIdentStart(c):
				var tok RefNode
				tok, i = consumeIdent(s, i)
				toks = append(toks, tok)
			default:
				// Unknown character: skip
				i++
			}
		}
	}

	return toks
}

// indexByte finds the first index of b in s, or -1.
func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// tokenize converts a reference expression into a list of RefNodes.
// Equivalent to the tokenize() from the JS: first extracts string literals,
// then tokenizes the remaining fragments.
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

// ── Reference parser ────────────────────────────────────────────────────────

// parseReference builds the reference tree from a list of tokens.
// Consumes tokens from the beginning of the slice pointed to by toks (equivalent to JS splice).
// Equivalent to the parseReference() from the original JS.
func parseReference(toks *[]RefNode) ([]RefNode, error) {
	if len(*toks) == 0 {
		return nil, fmt.Errorf("parseReference: empty reference")
	}

	token := popRef(toks)

	// Standalone literal (num or str): returns immediately
	if token.Type == TokNum || token.Type == TokStr {
		if len(*toks) == 0 || (*toks)[0].Type == TokClose {
			if len(*toks) > 0 {
				popRef(toks) // consumes the ']'
			}
			return []RefNode{token}, nil
		}
	}

	if token.Type != TokIdent {
		return nil, fmt.Errorf("parseReference: expected identifier, found type=%d val=%q", token.Type, token.StrVal)
	}

	tree := []RefNode{token}

	// State: stWSep = waiting for separator (. or [); stRef = waiting for ident/str
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
				return nil, fmt.Errorf("parseReference: expected '.' or '[', found type=%d", token.Type)
			}
			stat = stRef
		} else { // stRef
			if token.Type == TokIdent || token.Type == TokStr {
				tree = append(tree, token)
				stat = stWSep
				continue
			}
			return nil, fmt.Errorf("parseReference: expected identifier, found type=%d", token.Type)
		}
	}

	// Consumes closing ']' if present
	if len(*toks) > 0 && (*toks)[0].Type == TokClose {
		popRef(toks)
	}

	return tree, nil
}

// popRef removes and returns the first token from the slice.
func popRef(toks *[]RefNode) RefNode {
	t := (*toks)[0]
	*toks = (*toks)[1:]
	return t
}
