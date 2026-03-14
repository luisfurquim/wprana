//go:build js && wasm

package wprana

import (
	"errors"
	"syscall/js"

	"github.com/luisfurquim/goose"
)

// ErrLSKeyNotFound é retornado por LS.Get quando a chave não existe no localStorage.
var ErrLSKeyNotFound = errors.New("wprana: localStorage key not found")

// G é o logger global do pacote. Níveis recomendados: 1=somente erros, 2=geral, 3=detalhe,
// 4=debug leve, 5=debug verboso, 6=debug sensível.
var G goose.Alert = goose.Alert(2)

// ── Constantes de tipo de token ──────────────────────────────────────────────

// TokenType representa o tipo de um token no parser de templates.
type TokenType int8

const (
	TokTxt   TokenType = 0  // texto literal
	TokRef   TokenType = 1  // referência {{ }}
	TokStr   TokenType = 2  // string literal entre aspas
	TokDot   TokenType = 3  // operador .
	TokOpen  TokenType = 4  // operador [
	TokClose TokenType = 5  // operador ]
	TokNum   TokenType = 6  // número inteiro
	TokIdent TokenType = 7  // identificador
	TokWSep  TokenType = 8  // estado interno: aguardando separador
	TokExpr  TokenType = 9  // sub-expressão (acesso por índice dinâmico)
	TokAttr  TokenType = 10 // nó de atributo (tipo de DOMRefNode)
)

// ── Estruturas de parse de templates ─────────────────────────────────────────

// RefNode é um nó na árvore de referência parseada.
// Para TokExpr, Sub contém a sub-expressão (e.g. o índice em arr[expr]).
type RefNode struct {
	Type   TokenType
	StrVal string
	IntVal int
	Sub    []RefNode // preenchido apenas quando Type == TokExpr
}

// TextSegment é um segmento de texto de template: literal ou referência.
type TextSegment struct {
	IsRef bool
	Lit   string    // se !IsRef: texto literal
	Ref   []RefNode // se IsRef: árvore de referência parseada
}

// AttrBinding armazena os bindings de um único atributo.
type AttrBinding struct {
	Segs      []TextSegment
	ForceSync bool      // atributo com prefixo &
	PureRef   []RefNode // non-nil se o binding é uma referência pura (two-way binding)
}

// DOMRefNode descreve os bindings de template para um nó DOM.
type DOMRefNode struct {
	Type     TokenType               // TokTxt = nó de texto; TokAttr = elemento
	TextSegs []TextSegment           // segmentos de nó de texto
	Attrs    map[string]*AttrBinding // bindings de atributos
	Children map[int]*DOMRefNode     // bindings de filhos (por índice)
	ArrayVar string                  // variável de controle de iteração (vazio se nenhum)
	ArrayIdx string                  // variável de índice de iteração
	NoSpan   bool                    // prefixo **: modelo é o próprio parent
	Cond     string                  // expressão condicional (vazio se nenhum)
	CondTree []RefNode               // árvore parseada da condição
}

// ── Estado reativo ────────────────────────────────────────────────────────────

// Change descreve uma mutação de dados para sync otimizado.
type Change struct {
	Delete *DeleteInfo
}

// DeleteInfo descreve a remoção de um elemento de array.
type DeleteInfo struct {
	Target []any // slice alvo (referência, não cópia)
	Index  int
}

// Ctx é uma pilha de contextos de dados usada na resolução de referências.
type Ctx []any

// PranaState mantém o estado reativo de um componente vinculado.
type PranaState struct {
	Data      *ReactiveData
	Refs      *DOMRefNode
	ForceSync bool
	MaySync   bool
	dom       js.Value // SPAN container na shadow root
	model     js.Value // raiz do conteúdo HTML template
	lastEpoch uint64   // época do último sync (para prevenção de ciclos)
}

// ReactiveData encapsula o mapa de dados com notificação de mudança.
// Mutations via Set/Delete/Append/DeleteAt disparam sync automático.
type ReactiveData struct {
	M     map[string]any
	state *PranaState
}

// TwoWayBinding mantém o estado de um binding bidirecional (input/select/textarea).
type TwoWayBinding struct {
	Ref     []RefNode
	CtxPtr  *Ctx    // atualizado a cada sync; closure do handler aponta para cá
	Handler js.Func // handler JS; deve ser Released quando o elemento for removido
}

// NodeState armazena o estado Go-side para nós DOM gerenciados pelo prana.
// É indexado pelo campo _pranaId do nó JS.
type NodeState struct {
	// Para nós plug de iteração de array
	Model  js.Value
	ACtrl  string
	AIndex string
	Tree   []RefNode

	// Para nós condicionais (quando substituídos por comentário)
	CondModel js.Value // o elemento original (guardado enquanto há comentário)
	CondDaddy js.Value // parent para restauração de conditional

	// Para raízes de componente
	State     *PranaState
	PRoot     js.Value
	EHandlers map[string]string

	// Para bindings bidirecionais (indexado por nome de atributo)
	TwoWay map[string]*TwoWayBinding
}

// ── Interface pública do módulo ───────────────────────────────────────────────

// PranaObj é passado ao método Render do módulo.
type PranaObj struct {
	This    *ReactiveData
	Dom     js.Value // SPAN na shadow root
	Element js.Value // o custom element em si
	Trigger func(eventName string, args ...any)
}

// PranaMod é a interface que todo web component Go deve implementar.
//   - InitData() retorna o mapa de dados inicial (equivale ao "return {...}" do JS).
//   - Render(obj) é chamado após conexão ao DOM (equivale ao ready.then(...) do JS).
type PranaMod interface {
	InitData() map[string]any
	Render(obj *PranaObj)
}

// TriggerHandler é o tipo de função usada como handler de eventos @.
// Use o valor nil literal (TriggerHandler(nil)) como placeholder no InitData;
// defina o corpo real no Render, onde obj está disponível.
type TriggerHandler func(...any)

// ModFactory cria uma nova instância de PranaMod.
type ModFactory func() PranaMod

// modDef é a definição interna de um módulo registrado.
type modDef struct {
	factory  ModFactory
	html     string
	css      string
	observed []string // atributos observados pelo attributeChangedCallback
}

// ── Registros globais ─────────────────────────────────────────────────────────

var (
	// moduleRegistry armazena os módulos registrados via Register().
	moduleRegistry = map[string]*modDef{}

	// nodeRegistry armazena o estado Go-side de nós DOM, indexado por _pranaId.
	nodeRegistry = map[int64]*NodeState{}

	// nextNodeID é o próximo ID a ser atribuído.
	nextNodeID int64 = 1

	// jsSVGNS é o namespace SVG.
	jsSVGNS = "http://www.w3.org/2000/svg"

	// syncEpoch é o contador global de épocas de propagação.
	// Cada cadeia de propagação (Set, Delete, etc.) incrementa a época.
	// Componentes já sincronizados na época corrente são ignorados,
	// quebrando ciclos de propagação circular.
	// Inicia em 1 para que lastEpoch=0 (default de PranaState) seja sempre < syncEpoch.
	syncEpoch uint64 = 1

	// syncDepth conta o nível de aninhamento de sync (0 = nenhum sync em curso).
	// Usado para distinguir elementAttrChanged disparado internamente (por
	// setAttribute durante sync) de mudanças externas (JavaScript do usuário).
	syncDepth int
)

// jsVars são inicializadas em init() para evitar chamadas repetidas.
var (
	jsGlobal js.Value
	jsDoc    js.Value
)

func init() {
	jsGlobal = js.Global()
	jsDoc = jsGlobal.Get("document")
}

// assignNodeID atribui um _pranaId único ao nó e retorna o ID.
func assignNodeID(node js.Value) int64 {
	id := nextNodeID
	nextNodeID++
	node.Set("_pranaId", id)
	return id
}

// getNodeID lê o _pranaId de um nó JS. Retorna (0, false) se não tiver.
func getNodeID(node js.Value) (int64, bool) {
	v := node.Get("_pranaId")
	if v.IsUndefined() || v.IsNull() {
		return 0, false
	}
	return int64(v.Int()), true
}

// getOrCreateState retorna (ou cria) o NodeState associado a um nó DOM.
func getOrCreateState(node js.Value) (int64, *NodeState) {
	id, ok := getNodeID(node)
	if ok {
		if st, found := nodeRegistry[id]; found {
			return id, st
		}
	}
	id = assignNodeID(node)
	st := &NodeState{}
	nodeRegistry[id] = st
	return id, st
}

// getState retorna o NodeState de um nó DOM, ou nil se não existir.
func getState(node js.Value) *NodeState {
	id, ok := getNodeID(node)
	if !ok {
		return nil
	}
	return nodeRegistry[id]
}
