import hashlib
import io
from pathlib import Path
import tarfile
import tempfile
import unittest

from verify_release import ARCHES, verify


class ReleaseTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)

    def packages(self, fault=None):
        checksums = []
        for component in ("server", "agent"):
            for arch, (elf_class, machine) in ARCHES.items():
                binary = f"fenghuo-{component}"
                path = self.root / f"{binary}_linux_{arch}.tar.gz"
                header = bytearray(20)
                header[:6] = b"\x7fELF" + bytes((elf_class, 1))
                header[18:20] = machine.to_bytes(2, "little")
                files = {binary: bytes(header), f"{binary}.service": b"unit", "LICENSE": b"license",
                         "docs/installation.md": b"guide", "docs/installation.zh-CN.md": b"guide"}
                if component == "server":
                    files.update({"server.env.example": b"config", "Caddyfile.example": b"proxy"})
                if fault:
                    fault(files, binary)
                with tarfile.open(path, "w:gz") as archive:
                    for name, content in files.items():
                        entry = tarfile.TarInfo(name)
                        entry.mode = 0o755 if name == binary else 0o644
                        entry.size = len(content)
                        archive.addfile(entry, io.BytesIO(content))
                checksums.append(f"{hashlib.sha256(path.read_bytes()).hexdigest()}  {path.name}\n")
        (self.root / "checksums.txt").write_text("".join(checksums), encoding="utf-8")

    def test_valid_archives(self):
        self.packages()
        verify(self.root)

    def test_missing_required_file(self):
        self.packages(lambda files, binary: files.pop(f"{binary}.service"))
        with self.assertRaisesRegex(ValueError, "missing"):
            verify(self.root)

    def test_wrong_architecture(self):
        self.packages(lambda files, binary: files.update({binary: b"not an ELF"}))
        with self.assertRaisesRegex(ValueError, "architecture"):
            verify(self.root)

    def test_path_traversal(self):
        self.packages(lambda files, _: files.update({"../outside": b"unsafe"}))
        with self.assertRaisesRegex(ValueError, "unsafe"):
            verify(self.root)

    def test_tampered_download(self):
        self.packages()
        next(self.root.glob("*.tar.gz")).write_bytes(b"broken")
        with self.assertRaisesRegex(ValueError, "checksum mismatch"):
            verify(self.root)

    def test_duplicate_checksum(self):
        self.packages()
        checksum = self.root / "checksums.txt"
        text = checksum.read_text(encoding="utf-8")
        checksum.write_text(text + text.splitlines()[0] + "\n", encoding="utf-8")
        with self.assertRaisesRegex(ValueError, "duplicate"):
            verify(self.root)

    def test_missing_architecture(self):
        self.packages()
        next(self.root.glob("*.tar.gz")).unlink()
        with self.assertRaisesRegex(ValueError, "missing installation archive"):
            verify(self.root)


if __name__ == "__main__":
    unittest.main()
