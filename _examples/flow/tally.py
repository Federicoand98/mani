"""Tally step: a join of three steps on stdin, one record on stdout.

Each record says where it came from in its "from" field:
  from=classify  {"id": "r02", "product": "kettle", "result": {"sentiment": "negative", "problem": "..."}}
  from=reply     {"id": "r02", "product": "kettle", "result": {"reply": "..."}}
  from=catalog   {"id": "kettle", "task": {"name": "Electric kettle 1.7 l", ...}}

The same id can appear twice — r02 classified, and r02 replied to — and "from"
is what keeps them apart. Prints one record whose task is an object, so the
next agent receives it as JSON.

Also runs alone, which is the point of a code step:
  printf '%s\n' \
    '{"from":"classify","id":"r02","product":"kettle","result":{"sentiment":"negative","problem":"lid"}}' \
    '{"from":"catalog","id":"kettle","task":{"name":"Kettle"}}' | python3 tally.py --team Kitchen
"""

import argparse
import json
import sys
from collections import Counter

args = argparse.ArgumentParser()
args.add_argument("--team", required=True)
args.add_argument("--top", type=int, default=2)
opts = args.parse_args()

names, sentiment, problem, product, replies = {}, {}, {}, {}, {}
for line in sys.stdin:
    if not line.strip():
        continue
    rec = json.loads(line)
    source = rec.get("from")
    if source == "catalog":
        names[rec["id"]] = rec["task"]["name"]
    elif source == "classify":
        sentiment[rec["id"]] = rec["result"]["sentiment"]
        problem[rec["id"]] = rec["result"]["problem"]
        product[rec["id"]] = rec.get("product")
    elif source == "reply":
        replies[rec["id"]] = rec["result"]["reply"]

counts = Counter(sentiment.values())
complaints = Counter(product[r] for r, s in sentiment.items() if s == "negative")
negative = [
    {
        "review": r,
        "product": names.get(product[r], product[r]),
        "problem": problem[r],
        "reply_drafted": r in replies,
    }
    for r, s in sorted(sentiment.items())
    if s == "negative"
]

# Progress goes to stderr: mani passes it through, stdout is records only.
print(f"tally: {len(sentiment)} reviews, {len(negative)} negative, {len(replies)} replies", file=sys.stderr)

print(json.dumps({
    "id": "weekly",
    "task": {
        "team": opts.team,
        "total": len(sentiment),
        "by_sentiment": dict(counts),
        "negative": negative,
        "most_complained_about": [names.get(p, p) for p, _ in complaints.most_common(opts.top)],
    },
}))
