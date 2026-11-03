"""Catalog step: reads nothing, prints one record per product.

A first step is where a flow fetches what it works on. Here it is a local
file; in a real flow it would be an API call or a database query. Runs alone:
  python3 catalog.py
"""

import pathlib
import sys

catalog = pathlib.Path(__file__).with_name("products.jsonl").read_text()
sys.stdout.write(catalog)
print(f"catalog: {len(catalog.splitlines())} products", file=sys.stderr)
