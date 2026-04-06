/**
 * prana_helper.js
 *
 * Helper JS mínimo para interop com o wprana Go/WASM.
 * Deve ser carregado ANTES do wasm_exec.js e do binário .wasm.
 *
 * Define window._pranaDef(tagName, ctor, connected, attrChanged, disconnected, observed)
 * que é chamado pelo Go via DefineAll() / defineCustomElement().
 */

(function() {
   "use strict";

   /**
    * _pranaDef registra um custom element cujos callbacks de ciclo de vida
    * são implementados em Go/WASM.
    *
    * @param {string}   tagName      - nome do custom element (e.g. "my-widget")
    * @param {Function} ctor         - callback Go para constructor(self)
    * @param {Function} connected    - callback Go para connectedCallback(self)
    * @param {Function} attrChanged  - callback Go para attributeChangedCallback(self,name,old,new)
    * @param {Function} disconnected - callback Go para disconnectedCallback(self)
    * @param {string[]} observed     - lista de atributos observados
    */
   window._pranaDef = function(tagName, ctor, connected, attrChanged, disconnected, observed) {
      customElements.define(
         tagName,
         class extends HTMLElement {
            static get observedAttributes() {
               return observed || [];
            }

            constructor() {
               super();
               // Encaminha para o Go
               ctor(this);
            }

            connectedCallback() {
               connected(this);
            }

            attributeChangedCallback(name, oldValue, newValue) {
               attrChanged(this, name, oldValue ?? "", newValue ?? "");
            }

            disconnectedCallback() {
               disconnected(this);
            }
         }
      );
   };
})();
