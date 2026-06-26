# mani

Framework micro-agent in Go per costruire agenti LLM. Architettura esagonale:
il dominio (`core`) non sa nulla di provider, trasporto, filesystem o UI.

## Language

**Agent**:
Il ciclo di dominio che, data una memoria e un input, chiama l'LLM ed esegue tool
finché il modello non chiude il turno. Vive in `core`, non conosce adapter concreti.

**Runtime**:
La radice di composizione della CLI: cabla Agent, provider, tool e permessi, ed espone
l'esecuzione come stream di `Event`. È un adapter applicativo, non dominio. Un utente
libreria lo salta e cabla `core` da sé.
_Avoid_: Engine, Orchestrator, App.

**Emitter**:
La porta (in `core`) attraverso cui l'Agent comunica verso l'esterno ciò che produce
mentre gira: token, reasoning, chiamate ed esiti dei tool. Parla solo stringhe e
`map[string]any` — non sa di canali, eventi o UI. L'adapter a canale vive in `app`.
_Avoid_: Handler, Listener, Sink, Callback.

**Tool**:
Una capacità che l'Agent può invocare (leggere file, eseguire bash). Dichiara nome,
schema e un `Risk Level`. Definito nel package `tool`, consumato dagli adapter.

**Risk Level**:
La pericolosità dichiarata da un Tool: `none`, `write`, `execute`. Determina se serve
un permesso prima dell'esecuzione. Vive in `core`.
_Avoid_: Danger, Severity, Permission level.

**Hook**:
Un middleware registrato sull'Agent, invocato a punti precisi del ciclo di vita
(pre/post tool, pre/post chiamata LLM nel core; session start/end lato orchestratore).
Riceve un `HookEvent` (`Type` + payload a puntatore) e può osservare, **mutare** i dati
in place, o abortire ritornando `error`. Uniforme: ogni hook riceve tutti gli eventi e
filtra sul `Type`. Il payload è valido solo per la durata della chiamata.
_Avoid_: Filter, Interceptor, Listener.

**HookEvent**:
Ciò che un Hook riceve: un `Type` (stringa aperta — il core dichiara gli eventi di loop,
l'orchestratore può aggiungere i suoi, es. session) e un `Payload` a puntatore tipizzato
mutabile in place.
_Avoid_: Signal, Message.

**Compaction**:
La riduzione della storia dei messaggi quando la stima dei token supera una soglia della
finestra di contesto. Non è incorporata nell'Agent: è una *strategia* implementata da un
hook `ContextFull` (che muta i messaggi in place). L'Agent si limita a stimare i token e
sparare l'evento.
_Avoid_: Truncation, Summarization (sono strategie specifiche di compaction).

**Permission Manager**:
Il gate che, prima di eseguire un tool, traduce un `Risk Level` in una richiesta
all'utente e ne attende la `Decision`. **Non è un Hook generico**: è un meccanismo a sé,
invocato dall'Agent *dopo* gli hook `PreToolUse` (che possono aver mutato l'input), così
decide sull'input finale (niente TOCTOU). Tiene lo stato di sessione di ciò che è
"sempre permesso". Vive in `app`, non è dominio.

**Decision**:
La risposta dell'utente a una richiesta di permesso: `Deny`, `AllowOnce`, `AllowAlways`.
Concetto applicativo (`app`), mai esposto al `core`.
_Avoid_: Permission, Choice, Answer.

**Diff Preview**:
La rappresentazione `+`/`-` di una modifica che un tool di scrittura sta per applicare,
derivata dall'`input` (es. `old_content`/`new_content` di edit_file) **prima** di eseguire
il tool. Viaggia nel campo `Preview` della richiesta di permesso e la TUI la colora, così
l'utente approva/rifiuta vedendo il cambiamento. Non è un meccanismo a sé: è un
arricchimento del gate permesso. _Avoid_: Patch, Hunk.

**Tool Output Truncation**:
Il taglio dell'output di un tool oltre un limite in byte (testa+coda con marker) prima che
finisca in `Memory`, per non saturare il contesto. Non è nel `core`: è un hook `PostToolUse`
di default registrato da `app` (muta `Result` in place). _Avoid_: Trim, Clip (Trim è già la
strategia di Compaction).

**Event**:
Un'unità del flusso asincrono `Runtime → UI`: token, reasoning, chiamata/esito tool,
richiesta di permesso, fine, errore. Concetto applicativo. La UI lo consuma per
renderizzare. Distinto dall'`Emitter`, che è la porta lato dominio.

**Memory**:
La sequenza di messaggi del turno corrente passata all'LLM. Porta in `core`;
l'implementazione di default è in-memory.
_Avoid_: History, Context, Conversation.

**Session**:
Una conversazione distinta, con la sua `Memory`, un `Plan` (todo) e dei metadati (id,
titolo, timestamp, modello), switchabile durante un'esecuzione e ripristinabile da disco.
Concetto di orchestrazione: vive nel package `session/`, il `core` non la conosce.
_Avoid_: Conversation, Chat, Thread.

**Plan**:
La todo list del task corrente: una sequenza di `PlanStep` (`description` + `status`:
pending/in_progress/done). È **model-owned** — il modello la scrive/aggiorna via il tool
`todo_write` — e **advisory**: il loop non la impone, guida soltanto. Vive nella `Session`
(persiste), viene re-iniettata come reminder a ogni chiamata LLM. Il `core` non la conosce:
è orchestrazione (tool + hook in `app`). _Avoid_: Tasks, Steps (sono le voci), Workflow.

**Session Store**:
La porta che salva, carica, elenca ed elimina le `Session`. Vive in `session/`;
ha un adapter in memoria e uno su file (un JSON per sessione). Il `core` non lo
conosce: la serializzazione dei `Message` è interamente nell'adapter (via DTO).
_Avoid_: Repository, Persistence, DAO.

**Provider**:
Un servizio LLM concreto (ollama, openai, anthropic, copilot, openrouter, o un endpoint
OpenAI-compatible custom). È una *scelta di configurazione*, non un tipo: il `provider`
attivo nella config seleziona quale adapter cablare. La sua config (`base_url` + `model`)
vive nella mappa `providers`, quindi **ogni Provider ricorda il proprio modello**; il
modello attivo è quello del Provider attivo. Più Provider possono condividere lo stesso
`Wire Format`. _Avoid_: Backend, Vendor, Engine.

**Wire Format**:
Il protocollo concreto con cui un adapter parla all'LLM (OpenAI Chat Completions vs
Anthropic Messages). Determina mappatura di messaggi/tool e parsing dello streaming.
Copilot e Openrouter usano il Wire Format di OpenAI con endpoint/auth diversi.
_Avoid_: Protocol, API style.

**Credential**:
Il segreto per autenticarsi a un Provider: una API key (tipo `api`) o un token OAuth
con refresh ed expiry (tipo `oauth`, es. Copilot). Vive **solo** in `auth.json`
(`$XDG_DATA_HOME/mani/auth.json`, 0600), mai in `config.json`. Gestita nel package
`config/`; `auth.json` è autoritativo.
_Avoid_: Secret, Token, Key (sono casi specifici).

**Command**:
Un comando slash della TUI (`/model`, `/clear`, `/login`, …): parsa gli argomenti,
agisce sul `Runtime` e ritorna un `Result` — output sincrono, oppure un `Action` che
chiede alla TUI di entrare in una modalità (picker, login). Vive in `tui/command`,
consumato **solo** dalla TUI (il REPL è stato deprecato). _Avoid_: Action (è un campo
del Result), Handler, Verb.

**Model Lister**:
Capability *opzionale* di un adapter: elencare i modelli disponibili per il Provider.
Interfaccia separata in `core` (`ModelLister`), non parte del port `LLMClient`. Il
comando `/model` fa type-assert: se l'adapter la implementa mostra il picker, altrimenti
degrada a testo libero.
_Avoid_: ModelRegistry, Catalog.
