import base64
import hashlib
import io
from pathlib import Path
import tempfile
import unittest
from unittest.mock import Mock, patch

from botocore.exceptions import ClientError

import mirror_to_r2 as mirror


class FakeStore:
    def __init__(self):
        self.objects = {}
        self.writes = []
        self.race_content = None

    def digest(self, key):
        value = self.objects.get(key)
        return None if value is None else hashlib.sha256(value).hexdigest()

    def put_if_absent(self, key, path):
        self.writes.append(key)
        self.objects.setdefault(key, path.read_bytes() if self.race_content is None else self.race_content)


class MirrorTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)
        self.tag = "macos-v0.1.0"
        self.prefix = f"macos/releases/{self.tag}/"
        self.zip_name = "DetectRadarBar-v0.1.0-macos-universal.zip"
        self.write_release()

    def write_release(self):
        lines = []
        for name in sorted(mirror.expected_assets(self.tag) - {mirror.CHECKSUMS}):
            data = f"asset: {name}\n".encode()
            (self.directory / name).write_bytes(data)
            lines.append(f"{hashlib.sha256(data).hexdigest()}  {name}\n")
        (self.directory / mirror.CHECKSUMS).write_text("".join(lines))

    def test_complete_mirror_preserves_three_names_and_writes_manifest_last(self):
        store = FakeStore()
        hashes = mirror.mirror(self.directory, self.tag, store)
        self.assertEqual(len(hashes), 3)
        self.assertEqual(set(store.objects), {self.prefix + name for name in hashes})
        self.assertEqual(store.writes[-1], self.prefix + mirror.CHECKSUMS)
        self.assertFalse(any("/latest/" in key for key in store.objects))

    def test_identical_rerun_does_not_upload_again(self):
        store = FakeStore()
        mirror.mirror(self.directory, self.tag, store)
        store.writes.clear()
        mirror.mirror(self.directory, self.tag, store)
        self.assertEqual(store.writes, [])

    def test_partial_upload_resumes_without_replacing_existing_files(self):
        store = FakeStore()
        key = self.prefix + "LICENSE"
        store.objects[key] = (self.directory / "LICENSE").read_bytes()
        mirror.mirror(self.directory, self.tag, store)
        self.assertEqual(len(store.writes), 2)
        self.assertNotIn(key, store.writes)
        self.assertEqual(len(store.objects), 3)

    def test_conflicting_remote_object_blocks_all_new_writes(self):
        store = FakeStore()
        store.objects[self.prefix + self.zip_name] = b"another build of the same version"
        with self.assertRaisesRegex(ValueError, "Refusing to replace"):
            mirror.mirror(self.directory, self.tag, store)
        self.assertEqual(store.writes, [])

    def test_concurrent_conflicting_creation_is_never_overwritten(self):
        store = FakeStore()
        store.race_content = b"different concurrent writer"
        with self.assertRaisesRegex(ValueError, "not overwritten"):
            mirror.mirror(self.directory, self.tag, store)
        self.assertEqual(list(store.objects.values()), [store.race_content])
        self.assertNotIn(self.prefix + mirror.CHECKSUMS, store.objects)

    def test_tampered_local_asset_is_rejected_before_upload(self):
        (self.directory / "LICENSE").write_text("tampered")
        store = FakeStore()
        with self.assertRaisesRegex(ValueError, "Checksum mismatch"):
            mirror.mirror(self.directory, self.tag, store)
        self.assertEqual(store.writes, [])

    def test_checksum_manifest_requires_exact_coverage_and_unique_safe_names(self):
        path = self.directory / mirror.CHECKSUMS
        original = path.read_text()
        for changed in [
            "\n".join(original.splitlines()[1:]) + "\n",
            original + original.splitlines()[0] + "\n",
            original.replace("  LICENSE", "  ../LICENSE"),
            original + f"{'0' * 64}  extra.txt\n",
        ]:
            with self.subTest(manifest=changed):
                path.write_text(changed)
                with self.assertRaises(ValueError):
                    mirror.verify_assets(self.directory, self.tag)
        path.write_text(original)

    def test_missing_extra_and_symlink_assets_are_rejected(self):
        license_path = self.directory / "LICENSE"
        license_data = license_path.read_bytes()
        license_path.unlink()
        with self.assertRaises(ValueError):
            mirror.verify_assets(self.directory, self.tag)
        license_path.write_bytes(license_data)
        (self.directory / "extra").write_text("unexpected")
        with self.assertRaises(ValueError):
            mirror.verify_assets(self.directory, self.tag)
        (self.directory / "extra").unlink()
        license_path.unlink()
        license_path.symlink_to(self.directory / mirror.CHECKSUMS)
        with self.assertRaises(ValueError):
            mirror.verify_assets(self.directory, self.tag)

    def test_published_release_metadata_must_match_exact_asset_set(self):
        release = {
            "tag_name": self.tag,
            "draft": False,
            "assets": [{"name": name} for name in mirror.expected_assets(self.tag)],
        }
        mirror.validate_release(release, self.tag)
        for change in [
            dict(draft=True),
            dict(tag_name="macos-v0.1.1"),
            dict(assets=release["assets"][:-1]),
            dict(assets=release["assets"] + [release["assets"][0]]),
        ]:
            with self.subTest(change=change), self.assertRaises(ValueError):
                mirror.validate_release(release | change, self.tag)

    def test_tag_and_public_origin_validation(self):
        for tag in ["macos-v0.1.0", "macos-v1.20.3-rc.2"]:
            self.assertEqual(mirror.validate_tag(tag), tag)
        for tag in ["v1.0.0", "dataset-latest", "macos-v0.1.0/../../x", "macos-v0.1.0\n", "macos-v0.01.0"]:
            with self.assertRaises(ValueError):
                mirror.validate_tag(tag)
        self.assertEqual(mirror.public_base("https://dl.detectradar.com/"), "https://dl.detectradar.com")
        for base in [
            "http://dl.detectradar.com",
            "https://user:pass@dl.detectradar.com",
            "https://dl.detectradar.com/path",
            "https://dl.detectradar.com/?key=value",
        ]:
            with self.assertRaises(ValueError):
                mirror.public_base(base)

    def test_public_downloads_are_hashed_and_urls_have_one_separator(self):
        hashes = mirror.verify_assets(self.directory, self.tag)

        def open_asset(request, timeout):
            self.assertEqual(request.get_header("User-agent"), mirror.PUBLIC_USER_AGENT)
            url = request.full_url
            self.assertTrue(url.startswith("https://dl.detectradar.com/macos/releases/macos-v0.1.0/"))
            self.assertEqual(timeout, 120)
            return io.BytesIO((self.directory / url.rsplit("/", 1)[1]).read_bytes())

        with patch.object(mirror, "urlopen", side_effect=open_asset) as request:
            url = mirror.verify_public("https://dl.detectradar.com/", self.tag, hashes)
        self.assertEqual(request.call_count, 3)
        self.assertEqual(url, "https://dl.detectradar.com/macos/releases/macos-v0.1.0/")
        with patch.object(mirror, "urlopen", return_value=io.BytesIO(b"cached wrong content")):
            with self.assertRaisesRegex(ValueError, "Public download checksum mismatch"):
                mirror.verify_public("https://dl.detectradar.com", self.tag, hashes)

    def test_r2_put_is_conditional_and_supplies_content_md5(self):
        client = Mock()
        store = mirror.R2Store(client, "detectradar-releases")
        zip_path = self.directory / self.zip_name
        store.put_if_absent(f"{self.prefix}{self.zip_name}", zip_path)
        args = client.put_object.call_args.kwargs
        self.assertEqual(args["IfNoneMatch"], "*")
        self.assertEqual(args["ContentMD5"], base64.b64encode(hashlib.md5(zip_path.read_bytes()).digest()).decode())
        self.assertEqual(args["ContentType"], "application/zip")
        self.assertEqual(args["CacheControl"], mirror.CACHE_CONTROL)
        self.assertEqual(args["ContentDisposition"], f'attachment; filename="{self.zip_name}"')

    def test_r2_precondition_race_is_rechecked_but_permission_errors_fail(self):
        client = Mock()
        store = mirror.R2Store(client, "detectradar-releases")
        for code in ["PreconditionFailed", "412"]:
            client.put_object.side_effect = ClientError({"Error": {"Code": code}}, "PutObject")
            store.put_if_absent("key", self.directory / "LICENSE")
        client.put_object.side_effect = ClientError({"Error": {"Code": "AccessDenied"}}, "PutObject")
        with self.assertRaises(ClientError):
            store.put_if_absent("key", self.directory / "LICENSE")

    def test_r2_hash_reads_content_and_does_not_treat_auth_failure_as_missing(self):
        client = Mock()
        store = mirror.R2Store(client, "detectradar-releases")
        client.get_object.return_value = {"Body": io.BytesIO(b"remote file")}
        self.assertEqual(store.digest("key"), hashlib.sha256(b"remote file").hexdigest())
        client.get_object.side_effect = ClientError({"Error": {"Code": "NoSuchKey"}}, "GetObject")
        self.assertIsNone(store.digest("key"))
        client.get_object.side_effect = ClientError({"Error": {"Code": "AccessDenied"}}, "GetObject")
        with self.assertRaises(ClientError):
            store.digest("key")


if __name__ == "__main__":
    unittest.main()
