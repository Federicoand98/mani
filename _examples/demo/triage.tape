# Act 3 — the agent is a typed function.
#
# The hero demo: an agent that ends up inside a unix pipeline.
# Run from the repo root:  vhs _examples/demo/triage.tape

# Il path va QUOTATO: vhs rifiuta un percorso non quotato che inizia con "_"
# (legge l'underscore come inizio di comando e fallisce col parser).
Output "_examples/demo/triage.gif"

Require mani
Require jq

Set Shell "bash"
Set FontSize 18
Set Width 1200
Set Height 700
Set Padding 20
Set Theme "Catppuccin Mocha"
Set TypingSpeed 35ms

# Setup non registrato: prompt neutro e schermo pulito.
Hide
Type "export PS1='$ '" Enter
Type "cd _examples/demo" Enter
Type "clear" Enter
Show

Sleep 1s

# 1. Solo il blocco che conta: la forma della risposta, dichiarata nel manifest.
#    Si mostra un estratto e non l'intero file perche' a 700px / font 18 ci
#    stanno ~31 righe: oltre quelle il terminale scorre e l'inizio sparisce.
#    Il range si ferma su `required:` (ultima riga dello schema) e non su
#    `^limits:`, che verrebbe stampato anche lui.
Type "sed -n '/^output:/,/^ *required:/p' ../demo-triage.yaml" Enter
Sleep 5s

# Da qui NIENTE clear: le schermate si accumulano, cosi' l'ultimo fotogramma —
# quello che finisce nel README — racconta tutta la storia invece di mostrare
# due righe in cima a uno schermo vuoto.

# 2. Un campo estratto, pronto per un `if` in uno script.
#    Si estrae needs_paging e non severity per una ragione di marcatore: lo
#    schema qui sopra elenca i valori della severity (enum), quindi "high" e'
#    gia' a schermo e il Wait scatterebbe subito. Lo schema mostra solo
#    `type: boolean`, quindi true/false compaiono solo nell'output.
Type `mani run --config ../demo-triage.yaml --task "$(cat incident.log)" | jq -r .needs_paging` Enter
Wait+Screen@180s /true|false/
Sleep 3s

# 3. E l'oggetto completo: e' JSON valido, validato contro lo schema.
#    Marcatore: "}" — compare solo nell'output di jq (lo YAML qui sopra e' in
#    stile a blocchi e non ha graffe).
Type `mani run --config ../demo-triage.yaml --task "$(cat incident.log)" | jq .` Enter
Wait+Screen@180s /}/
Sleep 4s
