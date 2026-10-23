"""Verify the installer's release contract using only the Python standard library."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import socket
import subprocess
import tarfile
import tempfile
import time
import urllib.error
import urllib.request


ARCHES = {"amd64": (2, 62), "arm64": (2, 183), "armv7": (1, 40)}


def verify(directory):
    expected = {
        f"fenghuo-{component}_linux_{arch}.tar.gz": (component, arch)
        for component in ("server", "agent") for arch in ARCHES
    }
    checksums = {}
    for line in (directory / "checksums.txt").read_text(encoding="utf-8").splitlines():
        match = re.fullmatch(r"([0-9a-f]{64})  ([A-Za-z0-9_.-]+)", line)
        if not match or match[2] in checksums:
            raise ValueError("invalid or duplicate checksum entry")
        checksums[match[2]] = match[1]
    if checksums.keys() != expected.keys():
        raise ValueError("checksums must cover exactly the six installation archives")
    if {p.name for p in directory.glob("*.tar.gz")} != expected.keys():
        raise ValueError("unexpected or missing installation archive")
    for name, (component, arch) in expected.items():
        path = directory / name
        with path.open("rb") as source:
            digest = hashlib.file_digest(source, "sha256").hexdigest()
        if digest != checksums[name]:
            raise ValueError(f"checksum mismatch: {name}")
        binary = f"fenghuo-{component}"
        required = {binary, f"{binary}.service", "LICENSE",
                    "docs/installation.md", "docs/installation.zh-CN.md"}
        if component == "server":
            required |= {"server.env.example", "Caddyfile.example"}
        with tarfile.open(path, "r:gz") as archive:
            members = archive.getmembers()
            names = [member.name for member in members]
            if len(set(names)) != len(names) or not required.issubset(names):
                raise ValueError(f"missing or duplicate archive entries: {name}")
            for member in members:
                if (member.name.startswith("/") or ".." in member.name.split("/")
                        or "\\" in member.name or not (member.isfile() or member.isdir())):
                    raise ValueError(f"unsafe archive entry: {name}: {member.name}")
                if member.name in required and (not member.isfile() or member.size == 0):
                    raise ValueError(f"empty or invalid required file: {member.name}")
            executable = archive.getmember(binary)
            if not executable.mode & 0o111:
                raise ValueError(f"binary is not executable: {name}")
            with archive.extractfile(executable) as source:
                header = source.read(20)
            elf_class, machine = ARCHES[arch]
            if (len(header) < 20 or header[:4] != b"\x7fELF" or header[4] != elf_class
                    or header[5] != 1 or int.from_bytes(header[18:20], "little") != machine):
                raise ValueError(f"wrong binary architecture: {name}")


def smoke(directory):
    if platform.system() != "Linux" or platform.machine() not in ("x86_64", "amd64"):
        raise ValueError("--smoke requires a Linux amd64 host")
    # Ignore developer machine overrides, proxies and credentials in this isolated test.
    env = {key: value for key, value in os.environ.items() if not key.startswith("FENGHUO_")}
    client = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    with tempfile.TemporaryDirectory(prefix="fenghuo-release-") as temporary:
        root = Path(temporary)
        for component in ("agent", "server"):
            binary = f"fenghuo-{component}"
            with tarfile.open(directory / f"{binary}_linux_amd64.tar.gz", "r:gz") as archive:
                with archive.extractfile(binary) as source:
                    (root / binary).write_bytes(source.read())
            (root / binary).chmod(0o755)
            subprocess.run([str(root / binary), "-version" if component == "agent" else "version"],
                           check=True, env=env, timeout=10)
        # Reserve both ports at once so they cannot accidentally be identical.
        with socket.socket() as public, socket.socket() as admin:
            public.bind(("127.0.0.1", 0))
            admin.bind(("127.0.0.1", 0))
            public_port, admin_port = public.getsockname()[1], admin.getsockname()[1]
        base = f"http://127.0.0.1:{public_port}"
        with (root / "server.log").open("w+") as log:
            proc = subprocess.Popen([
                str(root / "fenghuo-server"), "serve", "-data", str(root / "data"),
                "-listen", f"127.0.0.1:{public_port}",
                "-admin-listen", f"127.0.0.1:{admin_port}",
            ], stdout=log, stderr=log, env=env)
            try:
                for _ in range(100):
                    if proc.poll() is not None:
                        raise ValueError("packaged server exited during startup")
                    try:
                        with client.open(base + "/api/v1/meta", timeout=1) as response:
                            if json.load(response)["admin"] is not False:
                                raise ValueError("admin exposed on public listener")
                        break
                    except (urllib.error.URLError, TimeoutError):
                        time.sleep(0.1)
                else:
                    raise ValueError("packaged server failed to become ready")
                with client.open(base, timeout=5) as response:
                    html = response.read().decode("utf-8")
                assets = re.findall(r'(?:src|href)="(/assets/[^"<>]+\.(?:js|css))"', html)
                if not assets:
                    raise ValueError("packaged server is missing its embedded frontend")
                for asset in assets:
                    with client.open(base + asset, timeout=5) as response:
                        if "text/html" in response.headers.get("Content-Type", "") or not response.read():
                            raise ValueError(f"embedded asset missing: {asset}")
                with client.open(f"http://127.0.0.1:{admin_port}/api/v1/meta", timeout=5) as response:
                    if json.load(response)["admin"] is not True:
                        raise ValueError("admin listener is not available")
            finally:
                proc.terminate()
                try:
                    proc.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    proc.kill()
                    proc.wait()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    parser.add_argument("--smoke", action="store_true")
    args = parser.parse_args()
    verify(args.directory)
    if args.smoke:
        smoke(args.directory)
    print("Release archives verified" + ("; binary smoke tests passed" if args.smoke else ""))
