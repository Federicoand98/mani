# Act 2 — the agent runs itself, and survives being killed.
#
# Run from the repo root:  vhs _examples/demo/unattended.tape
#
# Questa e' la demo che un framework a codice non puo' fare: non "un agente che
# gira", ma un agente che riprende LO STESSO task dopo un SIGKILL, con la policy
# che continua ad applicarsi mentre nessuno guarda.

# Il path va QUOTATO: vhs rifiuta un percorso non quotato che inizia con "_".
Output "_examples/demo/unattended.gif"

Require mani
Require jq
Require timeout

Set Shell "bash"
# 1400 di larghezza a font 16: le righe di log del daemon sono ~127 caratteri e
# a 1200/18 andavano a capo, spezzando proprio gli id che sono la prova.
# 420 di altezza: le due schermate stanno in ~17 righe, e a 620 restava mezzo
# fotogramma vuoto.
Set FontSize 16
Set Width 1400
Set Height 420
Set Padding 20
Set Theme "Catppuccin Mocha"
Set TypingSpeed 30ms

# Setup non registrato: prompt neutro e stato pulito, cosi' ogni registrazione
# parte dalle stesse condizioni.
Hide
Type "export PS1='$ '" Enter
Type "cd _examples/demo" Enter
Type "rm -rf queue runs" Enter
Type "clear" Enter
Show

Sleep 1s

# 1. Le due parti che contano: la regola, e il trigger con la coda su disco.
#    Si mostra un estratto e non il file intero per tenere l'altezza bassa —
#    a schermo pieno il manifest costringerebbe a un fotogramma mezzo vuoto
#    nella parte finale, che e' quella che si guarda piu' a lungo.
Type "sed -n '/^policy:/,/path: .\/queue/p' ../demo-unattended.yaml" Enter
Sleep 8s

Type "clear" Enter
Sleep 500ms

# 2. `mani run` SENZA --task: parte il daemon e lavora da solo. Il crash e'
#    PROGRAMMATO, non digitato: `timeout -s KILL` manda un SIGKILL dopo 9s.
#
#    Perche' non digitare `kill -9`: con un trigger ogni 2s il daemon stampa di
#    continuo, e i caratteri digitati finiscono in mezzo all'output ("kill -time=
#    2026-08-14T18:09..."). Programmando il crash non si digita mai mentre scorre
#    qualcosa.
#
#    Il filtro sulle righe [daemon] toglie venti righe di llm/tool che
#    seppellirebbero il punto. `grep --line-buffered` e' obbligatorio: senza, la
#    pipe bufferizza e non compare nulla finche' il processo non esce.
#
#    Nessun Wait in tutta la tape: le righe di log vanno a capo e un marcatore
#    spezzato dal wrapping non viene matchato — con un laconico "recording
#    failed" come unico indizio. I tempi qui sono noti, non stimati.
Type `timeout -s KILL 9 mani run --verbose --config ../demo-unattended.yaml 2>&1 | grep --line-buffered '\[daemon\]'` Enter
Sleep 12s

# 3. Il task interrotto e' ancora li', sul disco — e dietro c'e' l'arretrato.
#    La coda e' una maildir: lo stato di un task E' la directory in cui si trova,
#    quindi si ispeziona con `ls` e si ripara con `mv`.
Type "ls queue/running/ queue/pending/" Enter
Sleep 6s

Type "clear" Enter
Sleep 500ms

# 4. Si riparte. L'orfano viene recuperato e ripreso: STESSO id.
Type `timeout -s KILL 8 mani run --verbose --config ../demo-unattended.yaml 2>&1 | grep --line-buffered '\[daemon\]'` Enter
Sleep 11s

# 5. La prova, da qui senza clear: la coda ha portato i task a termine, e il
#    journal registra che la regola ha bloccato `rm -rf` a ogni giro — mentre
#    nessuno guardava. E' per questo che la policy sta nel runtime e non nel
#    prompt.
Type "ls queue/done/" Enter
Sleep 4s

Type `cat runs/*.jsonl | jq -c 'select(.kind=="guardrail") | .data'` Enter
Sleep 6s
