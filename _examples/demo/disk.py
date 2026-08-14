#!/usr/bin/env python3
"""A mani tool, in eight lines. JSON in on stdin, JSON out on stdout."""
import json, shutil, sys

args = json.load(sys.stdin)
path = args.get("path", "/")
total, used, free = shutil.disk_usage(path)
print(json.dumps({"path": path, "free_gb": round(free / 2**30, 1),
                  "used_pct": round(100 * used / total)}))
