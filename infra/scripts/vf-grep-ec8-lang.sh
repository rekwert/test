#!/bin/bash
echo "=== SEARCH Not valid EC in language ==="
grep -r 'Not valid' /opt/virtfusion/app/control/language/ 2>/dev/null | grep -i EC | head -20
grep -r '\[EC ' /opt/virtfusion/app/control/language/ 2>/dev/null | head -30
ls /opt/virtfusion/app/control/language/en/ 2>/dev/null | head -20

echo "=== api.json EC snippets ==="
python3 - <<'PY'
import json,glob
for path in glob.glob('/opt/virtfusion/app/control/language/en/*.json'):
    try:
        data=open(path,encoding='utf-8').read()
        if 'EC 8' in data or '[EC' in data:
            print('FILE', path)
            for i,line in enumerate(data.splitlines()):
                if 'EC' in line and '8' in line:
                    print(line[:200])
    except Exception:
        pass
PY
