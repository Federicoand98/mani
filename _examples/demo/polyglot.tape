# Act 3 — extend it in any language.
#
# Run from the repo root:  vhs _examples/demo/polyglot.tape
#
# Chi arriva si aspetta un sistema di plugin e trova uno script che legge JSON
# da stdin. Sono le due schermate piu' corte della demo, ed e' voluto: il punto
# e' proprio che non c'e' molto da mostrare.

# Il path va QUOTATO: vhs rifiuta un percorso non quotato che inizia con "_".
Output "_examples/demo/polyglot.gif"

Require mani
Require python3

Set Shell "bash"
Set FontSize 18
Set Width 1200
# 760px ≈ 34 righe: servono per tenere a schermo lo script, la dichiarazione e
# il risultato insieme. A 620 lo script scorreva via e l'argomento si perdeva.
Set Height 760
Set Padding 20
Set Theme "Catppuccin Mocha"
Set TypingSpeed 35ms

Hide
Type "export PS1='$ '" Enter
Type "cd _examples/demo" Enter
Type "clear" Enter
Show

Sleep 1s

# 1. Il tool: uno script. Argomenti JSON su stdin, risultato JSON su stdout.
Type "cat disk.py" Enter
Sleep 6s

# Niente clear: script, dichiarazione e risultato devono stare nello stesso
# fotogramma finale, o l'argomento ("sono la stessa cosa") si perde.

# 2. La dichiarazione nel manifest: nome, schema, livello di rischio.
Type "sed -n '/^capabilities:/,/required:/p' ../demo-polyglot.yaml" Enter
Sleep 6s

# 3. L'agente lo usa come qualunque tool built-in.
#
# Le righe "llm response" sono rumore: si tengono solo tool call/result, che
# sono quelle che dimostrano che lo script Python e' stato invocato davvero.
# grep -v lascia passare anche la risposta finale, che non e' una riga di log.
#
# Niente Wait: le righe di log vanno a capo e un marcatore spezzato dal wrapping
# non viene matchato. Una run misurata sta sui 3s; 12s lasciano margine.
Type `mani run --verbose --config ../demo-polyglot.yaml --task "how much space is left on / ?" 2>&1 | grep -v 'llm '` Enter
Sleep 12s
