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

**Manifest**:
Il file YAML che *dichiara* un agente. È il prodotto: la tesi è "agenti come configurazione".
Ha **8 blocchi di primo livello**, ognuno dei quali risponde a **una sola domanda** (vedi
§ Grammatica del manifest). Si carica in `RuntimeSpec` con `KnownFields(true)`: una chiave
sconosciuta è un **errore**, mai un silenzio. _Avoid_: Config (è la config globale in
`config/`, un'altra cosa), Spec (è il tipo Go, non il documento), Definition.

**Block**:
Una delle 8 sezioni di primo livello del Manifest. Un blocco = una domanda; se una chiave ne
risponde a due va spezzata, se a nessuna non è una chiave di manifest.
_Avoid_: Section, Group, Namespace.

**Policy**:
Il blocco che risponde a "cosa gli è permesso", su **tre grane**: `tools` (se un tool è
utilizzabile: allow/ask/deny), `rules` (se la **singola invocazione** passa, guardando
l'input), `redact` (mascheratura dell'**output**). Meccanismi interni diversi — la prima è
un gate, le altre due sono hook — ma un'unica domanda, quindi un unico blocco.
_Avoid_: Permissions (è solo la prima grana), Guardrails (era il nome delle altre due).

**Limits**:
Il blocco dei **tetti numerici** su un run: token, tool call, durata, iterazioni, timeout per
tool, profondità dei subagent. Prima erano sparsi in quattro posti.
_Avoid_: Budget (era il nome parziale), Quota, Constraints.

**Capability**:
Un'abilità dell'agente dichiarata in `capabilities`: un Tool (built-in, subprocess o MCP), un
Subagent, il workspace. `planning` e `delegate` **sono tool**, non flag: si dichiarano come
gli altri. Chiave di manifest e `tool.Name()` a runtime devono coincidere — se divergono, la
`Policy` non blocca e i Subagent non risolvono, entrambi in silenzio.
_Avoid_: Feature (era il cassetto senza criterio), Plugin, Skill.

**Trace**:
La sequenza strutturata di step di un run (llm call, tool call/result, con `run_id` di
correlazione), emessa come log `slog` a livelli (error/warn/info/debug). Non è nel `core`:
è un osservatore trasversale costruito **sugli hook** esistenti (`RegisterTracing` in `app`).
Il `run_id` viaggia nel `context`. _Avoid_: Log (è il mezzo), Span (non è OpenTelemetry).

**MCP**:
Model Context Protocol: standard per esporre tool come processi/server esterni. mani è un
**MCP client** (`tool/mcp`): collega un server (stdio o HTTP/SSE) via l'SDK ufficiale, elenca
i suoi tool e li adatta a `tool.Tool`. Rende i tool scrivibili in qualsiasi linguaggio. È un
adapter sorgente-di-tool, come `tool/fs`. _Avoid_: Plugin, Extension.

**Trigger**:
Una sorgente di eventi (`every` a intervallo, `daily` a orario, `webhook` HTTP) che, allo
scatto, accoda un `Task`. È un **driving adapter** (il `Daemon` in `app`) che guida il
`Runtime` da eventi invece che dall'umano. Ha un'identità stabile (`name`, o un hash di
tipo+schedule+prompt) che permette il **catch-up** degli scatti persi mentre il processo era
spento. Non essendoci umano, i prompt di permesso seguono una **Policy** (deny di default,
allow opt-in). _Avoid_: Hook (è middleware del loop), Cron (è solo uno dei tipi).

**Task**:
Una singola esecuzione dell'agente generata dallo scatto di un Trigger: porta il prompt, il
trigger d'origine, la modalità di memoria e lo stato dei tentativi (`attempt`, `next_at`).
È **durevole**: sopravvive al riavvio, quindi tutto ciò che serve a rieseguirlo sta nel Task
stesso. Non esiste (per ora) un modo esterno di accodarne.
_Avoid_: Job, Run (il Run è l'esecuzione, il Task è la richiesta).

**Task Queue**:
La porta che accoda, consegna e archivia i `Task`. Due adapter: in memoria (default) e su
filesystem in stile **maildir** — lo stato di un task *è la directory in cui si trova*
(`pending/`, `running/`, `done/`, `failed/`) e la transizione è un `os.Rename` atomico. Si
ispeziona con `ls`, si ripara con `mv`. Scelta opposta al `Journal` e per un motivo preciso:
il journal è un record storico (append-only), la coda è **stato mutabile**.
_Avoid_: Broker, Bus, Mailbox.

**Scheduler**:
Il blocco `run.scheduler` del manifest e i worker che ne discendono: governa **come** i Task
vengono eseguiti (quanti in parallelo, quanti accumularne, quante volte ritentare, se
sopravvivono al riavvio). **Non contiene task**: quelli nascono a runtime dai Trigger. I suoi
worker partono da soli — non si dichiara nulla per consumarli.
_Avoid_: Queue (come nome del blocco: descrive il contenitore, non il comportamento), Pool.

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
derivata dall'`input` (es. `old_content`/`new_content` di `edit`) **prima** di eseguire
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

**Subagent**:
Un `core.Agent` figlio spawnato dal tool `delegate` per un sotto-task: memoria fresca,
stessi tool del padre (incluso `delegate`), gate permesso ereditato, output silenzioso
(`nopEmitter`). Ritorna al padre solo la risposta finale (un `tool_result`) → isola il
contesto. La profondità di annidamento viaggia nel `context` ed è limitata da un depth-cap.
Non è un tipo nuovo: è composizione del `core.Agent` esistente, orchestrata in `app`.
_Avoid_: Worker, Child agent (in codice "child" ok), Actor.

**Plan**:
La todo list del task corrente: una sequenza di `PlanStep` (`description` + `status`:
pending/in_progress/done). È **model-owned** — il modello la scrive/aggiorna via il tool
`planning` — e **advisory**: il loop non la impone, guida soltanto. Vive nella `Session`
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

---

## Grammatica del manifest

**La regola:** ogni blocco di primo livello risponde a **esattamente una domanda**.

| Blocco | Domanda |
|---|---|
| `identity` | chi PENSA? |
| `capabilities` | cosa SA FARE? |
| `context` | cosa VEDE e RICORDA? |
| `output` | cosa RESTITUISCE? |
| `policy` | cosa GLI È PERMESSO? |
| `limits` | QUANTO può consumare? |
| `run` | QUANDO parte e COME viene eseguito? |
| `observability` | cosa LASCIA DIETRO DI SÉ? |

**Dove va una chiave nuova.** Si pongono queste domande *in quest'ordine*; la prima che
risponde "sì" vince. L'ordine va dal meno invasivo (osserva soltanto) al più fondante
(definisce l'agente), così la decisione è deterministica.

```
1. Registra soltanto, senza cambiare il comportamento?   → observability
2. Decide QUANDO parte un run, o QUANTI ne girano?       → run
3. È un tetto numerico su un consumo?                    → limits
4. Può BLOCCARE o MODIFICARE un'azione?                  → policy
5. Vincola la FORMA della risposta finale?               → output
6. Cambia cosa finisce nel CONTESTO del modello?         → context
7. Aggiunge un'ABILITÀ all'agente?                       → capabilities
8. Cambia CHI o COSA ragiona?                            → identity
```

L'albero decide **dove** va una cosa; il filtro-feature decide **se** deve esistere
(manifest expressiveness / safe autonomy / operability, altrimenti è terreno LangChain).
Sono due controlli distinti, entrambi necessari.

**Nomi ritirati** (fase 31, rottura netta — `KnownFields(true)` li rifiuta esplicitamente):
`features` · `permissions` · `guardrails` · `budget` · `queue` · `mcpservers` ·
`system_prompt` · `output_schema` · `context_window` · `max_iterations` · `triggers` ·
`tools` e `provider`/`model` di primo livello. Il tool `todo_write` è diventato `planning`;
i tool `read_file`/`edit_file`/`write_file` sono diventati `read`/`edit`/`write` per far
coincidere chiave di manifest e nome a runtime.
