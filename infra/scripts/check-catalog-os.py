#!/usr/bin/env python3
import json
import urllib.request

r = urllib.request.urlopen("http://127.0.0.1:8080/api/v1/catalog/os")
data = json.load(r)
templates = data.get("os_templates", [])
print("active OS count:", len(templates))
for t in templates:
    if "win" in t["id"].lower() or "windows" in t["name"].lower():
        print(" ", t["id"], t["name"], t["version"])
