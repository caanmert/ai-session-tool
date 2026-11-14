import io
import stat
import unittest
import zipfile

from verify_desktop import check_archive_paths, check_info


class DesktopArchiveTests(unittest.TestCase):
    def archive(self, filename, data=b"file", symlink=False):
        stream = io.BytesIO()
        with zipfile.ZipFile(stream, "w") as archive:
            entry = zipfile.ZipInfo(filename)
            if symlink:
                entry.external_attr = (stat.S_IFLNK | 0o777) << 16
            archive.writestr(entry, data)
        stream.seek(0)
        return zipfile.ZipFile(stream)

    def test_paths_and_internal_links(self):
        with self.archive("ais-desktop.app/Contents/Frameworks/current", b"Versions/A", True) as archive:
            check_archive_paths(archive)
        for filename in ["../outside", "/tmp/outside", "ais-desktop.app/../../outside"]:
            with self.archive(filename) as archive, self.assertRaises(ValueError):
                check_archive_paths(archive)

    def test_escaping_links(self):
        for target in [b"/tmp/outside", b"../../../outside"]:
            with self.archive("ais-desktop.app/Contents/link", target, True) as archive, self.assertRaises(ValueError):
                check_archive_paths(archive)

    def test_bundle_identity_and_minimum(self):
        info = {"CFBundleExecutable": "ais-desktop", "CFBundleIdentifier": "io.github.caanmert.ais", "CFBundlePackageType": "APPL", "LSMinimumSystemVersion": "13.0"}
        self.assertEqual(check_info(info), "ais-desktop")
        for key, value in [("CFBundleExecutable", "../outside"), ("LSMinimumSystemVersion", "14.0"), ("CFBundleIdentifier", "other.app")]:
            with self.assertRaises(ValueError):
                check_info({**info, key: value})

    def test_link_chain_cannot_escape_via_a_shallower_parent(self):
        stream = io.BytesIO()
        with zipfile.ZipFile(stream, "w") as archive:
            for filename, target in [("ais-desktop.app/Contents/alias", b".."), ("ais-desktop.app/Contents/alias/link", b"../outside")]:
                entry = zipfile.ZipInfo(filename)
                entry.external_attr = (stat.S_IFLNK | 0o777) << 16
                archive.writestr(entry, target)
        stream.seek(0)
        with zipfile.ZipFile(stream) as archive, self.assertRaises(ValueError):
            check_archive_paths(archive)


if __name__ == "__main__":
    unittest.main()
