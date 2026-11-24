#!/usr/bin/env python3
"""Mirror verified macOS release assets to immutable, versioned R2 keys."""

import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
from urllib.parse import quote, urlsplit
from urllib.request import Request, urlopen

import boto3
from botocore.config import Config
from botocore.exceptions import ClientError

REPOSITORY = "harrisonwang/detect-radar"
APP_NAME = "DetectRadarBar"
CHECKSUMS = "SHA256SUMS.txt"
PREFIX = "macos/releases"
CACHE_CONTROL = "public, max-age=31536000, immutable"
PUBLIC_USER_AGENT = "DetectRadar-Release-Verifier/1.0 (+https://github.com/harrisonwang/detect-radar)"

TAG_RE = re.compile(
    r"macos-v((?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?)"
)


def validate_tag(tag):
    if not TAG_RE.fullmatch(tag):
        raise ValueError("Expected a macos-vX.Y.Z release tag, optionally with a prerelease suffix")
    return tag


def version_from_tag(tag):
    match = TAG_RE.fullmatch(tag)
    if not match:
        raise ValueError("Expected a macos-vX.Y.Z release tag, optionally with a prerelease suffix")
    return match.group(1)


def expected_assets(tag):
    version = version_from_tag(tag)
    return {
        f"{APP_NAME}-v{version}-macos-universal.zip",
        "LICENSE",
        CHECKSUMS,
    }


def hash_stream(stream, algorithm="sha256"):
    digest = hashlib.new(algorithm)
    for chunk in iter(lambda: stream.read(1024 * 1024), b""):
        digest.update(chunk)
    return digest.digest()


def digest_file(path):
    with path.open("rb") as stream:
        return hash_stream(stream).hex()


def verify_assets(directory, tag):
    expected = expected_assets(tag)
    actual = {path.name for path in directory.iterdir()}
    if actual != expected:
        raise ValueError(f"Unexpected or missing release assets: {sorted(actual ^ expected)}")
    for name in expected:
        path = directory / name
        if path.is_symlink() or not path.is_file() or path.stat().st_size == 0:
            raise ValueError(f"Release asset must be a nonempty regular file: {name}")
    hashes = {}
    for line in (directory / CHECKSUMS).read_text(encoding="utf-8").splitlines():
        match = re.fullmatch(r"([0-9a-f]{64})  ([A-Za-z0-9_.-]+)", line)
        if not match or match[2] in hashes:
            raise ValueError("Malformed or duplicate SHA256SUMS entry")
        hashes[match[2]] = match[1]
    payloads = expected - {CHECKSUMS}
    if hashes.keys() != payloads:
        raise ValueError(f"SHA256SUMS must cover exactly all {len(payloads)} release payloads")
    for name, expected_hash in hashes.items():
        if digest_file(directory / name) != expected_hash:
            raise ValueError(f"Checksum mismatch: {name}")
    hashes[CHECKSUMS] = digest_file(directory / CHECKSUMS)
    return hashes


def validate_release(release, tag):
    if release["tag_name"] != tag or release["draft"]:
        raise ValueError("Only the requested, published release can be mirrored")
    names = [asset["name"] for asset in release["assets"]]
    expected = expected_assets(tag)
    if len(names) != len(set(names)) or set(names) != expected:
        raise ValueError(f"Release must contain exactly the {len(expected)} expected macOS assets")


def download_release(tag, directory):
    raw = subprocess.check_output(
        ["gh", "api", f"repos/{REPOSITORY}/releases/tags/{quote(tag, safe='')}"], text=True
    )
    validate_release(json.loads(raw), tag)
    subprocess.run(
        ["gh", "release", "download", tag, "--repo", REPOSITORY, "--dir", str(directory)],
        check=True,
    )


class R2Store:
    def __init__(self, client, bucket):
        self.client = client
        self.bucket = bucket

    def digest(self, key):
        try:
            response = self.client.get_object(Bucket=self.bucket, Key=key)
        except ClientError as error:
            if error.response["Error"]["Code"] in {"NoSuchKey", "404"}:
                return None
            raise
        with response["Body"] as stream:
            return hash_stream(stream).hex()

    def put_if_absent(self, key, path):
        # R2 evaluates If-None-Match atomically. Never retry with an unconditional PUT.
        with path.open("rb") as stream:
            md5 = base64.b64encode(hash_stream(stream, "md5")).decode("ascii")
            stream.seek(0)
            if path.suffix == ".txt" or path.name == "LICENSE":
                content_type = "text/plain; charset=utf-8"
            elif path.suffix == ".zip":
                content_type = "application/zip"
            else:
                content_type = "application/octet-stream"
            try:
                self.client.put_object(
                    Bucket=self.bucket,
                    Key=key,
                    Body=stream,
                    IfNoneMatch="*",
                    ContentLength=path.stat().st_size,
                    ContentMD5=md5,
                    ContentType=content_type,
                    CacheControl=CACHE_CONTROL,
                    ContentDisposition=f'attachment; filename="{path.name}"',
                )
            except ClientError as error:
                if error.response["Error"]["Code"] not in {"PreconditionFailed", "412"}:
                    raise
                # A concurrent upload won. The caller verifies its full content.


def mirror(directory, tag, store):
    hashes = verify_assets(directory, tag)
    prefix = f"{PREFIX}/{tag}/"
    missing = []
    # Check the complete existing version before uploading anything new.
    for name, expected_hash in sorted(hashes.items()):
        remote_hash = store.digest(prefix + name)
        if remote_hash is None:
            missing.append(name)
        elif remote_hash != expected_hash:
            raise ValueError(f"Refusing to replace changed R2 object: {prefix}{name}")
    # Publish the checksum manifest last, after every payload has been verified.
    for name in sorted(missing, key=lambda item: (item == CHECKSUMS, item)):
        store.put_if_absent(prefix + name, directory / name)
        if store.digest(prefix + name) != hashes[name]:
            raise ValueError(f"R2 upload verification failed; object was not overwritten: {prefix}{name}")
    return hashes


def public_base(value):
    parts = urlsplit(value)
    if (
        parts.scheme != "https"
        or not parts.hostname
        or parts.username
        or parts.password
        or parts.query
        or parts.fragment
        or parts.path not in {"", "/"}
    ):
        raise ValueError("R2_PUBLIC_BASE_URL must be an HTTPS origin without credentials, a path or query")
    return value.rstrip("/")


def verify_public(base, tag, hashes):
    base = public_base(base)
    for name, expected_hash in sorted(hashes.items()):
        url = f"{base}/{PREFIX}/{tag}/{name}"
        # Identify this verifier explicitly; Cloudflare rejects generic Python-urllib clients.
        request = Request(url, headers={"User-Agent": PUBLIC_USER_AGENT})
        with urlopen(request, timeout=120) as response:
            if hash_stream(response).hex() != expected_hash:
                raise ValueError(f"Public download checksum mismatch: {name}")
    return f"{base}/{PREFIX}/{tag}/"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tag", required=True)
    args = parser.parse_args()
    tag = validate_tag(args.tag)
    required = ("R2_ACCOUNT_ID", "R2_BUCKET", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "R2_PUBLIC_BASE_URL")
    missing = [name for name in required if not os.environ.get(name)]
    if missing:
        raise ValueError(f"Missing mirror configuration: {', '.join(missing)}")
    account = os.environ["R2_ACCOUNT_ID"]
    if not re.fullmatch(r"[0-9a-f]{32}", account):
        raise ValueError("Invalid R2 account ID")
    base = public_base(os.environ["R2_PUBLIC_BASE_URL"])
    client = boto3.client(
        "s3",
        region_name="auto",
        endpoint_url=f"https://{account}.r2.cloudflarestorage.com",
        config=Config(
            signature_version="s3v4",
            retries={"mode": "standard", "max_attempts": 5},
            request_checksum_calculation="when_required",
            response_checksum_validation="when_required",
        ),
    )
    with tempfile.TemporaryDirectory(prefix="detectradar-mirror-") as temp:
        directory = Path(temp)
        download_release(tag, directory)
        hashes = mirror(directory, tag, R2Store(client, os.environ["R2_BUCKET"]))
        url = verify_public(base, tag, hashes)
    message = f"Verified all {len(hashes)} immutable macOS assets at {url}"
    print(message)
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(os.environ["GITHUB_STEP_SUMMARY"], "a", encoding="utf-8") as summary:
            summary.write(
                f"{message}\n\nSHA-256 verified against the published GitHub release. No latest alias was changed.\n"
            )


if __name__ == "__main__":
    main()
