"""Tally step: every classified review on stdin, one record on stdout.

Reads the records of the classify step, one JSON object per line:
  {"id": "r02", "product": "kettle", "result": {"sentiment": "negative", "problem": "..."}, "run": {...}}
and prints a single record whose task is an object: the next agent gets it as JSON.

Also runs alone, which is the point of a code step:
  echo '{"id":"r02","product":"kettle","result":{"sentiment":"negative","problem":"lid"}}' | python3 tally.py
"""

import json
import sys
from collections import Counter

counts = Counter()
negative = []

for line in sys.stdin:
    if not line.strip():
        continue
    rec = json.loads(line)
    result = rec["result"]
    counts[result["sentiment"]] += 1
    if result["sentiment"] == "negative":
        negative.append({"review": rec["id"], "product": rec.get("product"), "problem": result["problem"]})

# Progress goes to stderr: mani passes it through, stdout is records only.
print(f"tally: {sum(counts.values())} reviews, {len(negative)} negative", file=sys.stderr)

print(json.dumps({
    "id": "weekly",
    "task": {"total": sum(counts.values()), "by_sentiment": dict(counts), "negative": negative},
}))
