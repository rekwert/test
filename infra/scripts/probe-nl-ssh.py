#!/usr/bin/env python3
import subprocess
from pathlib import Path

passwords = ["3Px+eEWVcT"]
for p in ["/tmp/nl-finish-safe.sh", "/root/nl-pass.txt"]:
    path = Path(p)
    if not path.exists():
        continue
    text = path.read_text(errors="ignore")
    for line in text.splitlines():
        if "NL_PASS" in line and "'" in line:
            try:
                passwords.append(line.split("'", 2)[1])
            except Exception:
                pass

seen = []
for pwd in passwords:
    if pwd and pwd not in seen:
        seen.append(pwd)

print("trying", len(seen), "passwords")
for pwd in seen:
    r = subprocess.run(
        [
            "sshpass", "-p", pwd,
            "ssh",
            "-o", "StrictHostKeyChecking=no",
            "-o", "ConnectTimeout=10",
            "-o", "PreferredAuthentications=password",
            "-o", "PubkeyAuthentication=no",
            "root@66.248.206.14",
            "echo OK; uname -a; head -3 /etc/os-release; ip -br a | head -15",
        ],
        capture_output=True,
        text=True,
    )
    print("rc", r.returncode, "pwd_prefix", pwd[:4])
    out = (r.stdout or "") + (r.stderr or "")
    print(out[:1000])
    if r.returncode == 0 and "OK" in out:
        Path("/tmp/nl-root-pass.txt").write_text(pwd)
        break
else:
    # try key auth without password
    r = subprocess.run(
        [
            "ssh",
            "-o", "StrictHostKeyChecking=no",
            "-o", "ConnectTimeout=10",
            "root@66.248.206.14",
            "echo OK_KEY; uname -a; head -3 /etc/os-release; ip -br a | head -15",
        ],
        capture_output=True,
        text=True,
    )
    print("key_rc", r.returncode)
    print(((r.stdout or "") + (r.stderr or ""))[:1000])
