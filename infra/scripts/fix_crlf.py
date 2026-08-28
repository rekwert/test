import glob
files = [
    "/opt/testVPStrade/infra/scripts/solusvm-nl-bootstrap-from-back.sh",
    "/opt/testVPStrade/infra/scripts/solusvm-nl-host-prep.sh",
    "/opt/testVPStrade/infra/scripts/solusvm-nl-install.sh",
    "/opt/testVPStrade/infra/scripts/solusvm-bootstrap-api.py",
    "/opt/testVPStrade/infra/scripts/solusvm-nl-finish-setup.sh",
    "/opt/testVPStrade/infra/scripts/solusvm-nl-network-recover.sh",
]
for p in files:
    data = open(p, "rb").read()
    data = data.replace(b"\r\n", b"\n").replace(b"\r", b"\n")
    open(p, "wb").write(data)
    print("fixed", p, "first_line", open(p, "rb").readline())
