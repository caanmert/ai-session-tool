import hashlib
from pathlib import Path
import tempfile
import unittest

from homebrew import render


class HomebrewTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.dist = Path(self.tmp.name)
        self.names = []
        rows = []
        for platform in ("darwin", "linux"):
            for arch in ("arm64", "amd64"):
                name = f"ais_1.2.3_{platform}_{arch}.tar.gz"
                self.names.append(name)
                payload = name.encode()
                (self.dist / name).write_bytes(payload)
                rows.append(f"{hashlib.sha256(payload).hexdigest()}  {name}")
        (self.dist / "checksums.txt").write_text("\n".join(rows) + "\n")

    def test_all_platforms_and_completions(self):
        formula = render("v1.2.3", self.dist)
        for name in self.names:
            self.assertIn(f"/releases/download/v1.2.3/{name}", formula)
        self.assertEqual(formula.count("sha256 "), 4)
        self.assertIn('zsh_completion.install "completions/ais.zsh" => "_ais"', formula)
        self.assertNotIn("depends_on", formula)

    def test_corrupt_archive_is_rejected(self):
        (self.dist / self.names[0]).write_bytes(b"corrupt")
        with self.assertRaisesRegex(ValueError, "checksum mismatch"):
            render("v1.2.3", self.dist)

    def test_missing_platform_is_rejected(self):
        rows = (self.dist / "checksums.txt").read_text().splitlines()
        (self.dist / "checksums.txt").write_text("\n".join(rows[:-1]))
        with self.assertRaisesRegex(ValueError, "missing checksum"):
            render("v1.2.3", self.dist)

    def test_duplicate_is_rejected(self):
        path = self.dist / "checksums.txt"
        path.write_text(path.read_text() * 2)
        with self.assertRaisesRegex(ValueError, "duplicate checksum"):
            render("v1.2.3", self.dist)

    def test_unsafe_tag_is_rejected(self):
        for tag in ('v1.2.3"', "../../bad", "main", "v1.2.3\n"):
            with self.assertRaises(ValueError):
                render(tag, self.dist)


if __name__ == "__main__":
    unittest.main()
