"""Prepare compiler-only research inputs and checksum-verified native binaries."""

import argparse
import hashlib
import json
from pathlib import Path
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parent
VERSIONS = ("0.5.17", "0.8.34", "0.8.37")
BASE = "https://raw.githubusercontent.com/argotorg/solc-bin/gh-pages"


def save(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2) + "\n")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--cache", type=Path, required=True)
    parser.add_argument("--fetch-native", action="store_true")
    args = parser.parse_args()
    args.cache.mkdir(parents=True, exist_ok=True)
    previous = ROOT.parent / "token_taxes_20260922" / "evidence"
    live = json.loads((previous / "live.json").read_text())
    cases = []
    for address, token in live["tokens"].items():
        source = token.get("source")
        if not source or source["compilation"]["name"] == "FiatTokenProxy":
            continue
        compilation = source["compilation"]
        inp = source["stdJsonInput"]
        inp["settings"]["outputSelection"] = {
            "*": {"*": ["evm.deployedBytecode", "evm.methodIdentifiers"], "": ["ast"]}
        }
        name = compilation["name"]
        data = json.dumps(inp).encode()
        (args.cache / (name + ".json")).write_bytes(data)
        cases.append({"name": name, "address": address,
                      "compiler": compilation["compilerVersion"],
                      "version": compilation["compilerVersion"].split("+")[0],
                      "qualifiedName": compilation["fullyQualifiedName"],
                      "inputBytes": len(data),
                      "inputSHA256": hashlib.sha256(data).hexdigest()})
    save(ROOT / "evidence" / "cases.json", cases)
    if not args.fetch_native:
        print(json.dumps({"prepared": len(cases), "cache": str(args.cache)}))
        return

    ledger = []

    def fetch(url, path):
        if path.exists():
            return path.read_bytes()
        for attempt in range(4):
            record = {"url": url, "attempt": attempt + 1}
            try:
                request = urllib.request.Request(url, headers={"User-Agent": "K4K3RU-compiler-research/1.0"})
                with urllib.request.urlopen(request, timeout=30) as response:
                    data = response.read(64 * 1024 * 1024 + 1)
                    if len(data) > 64 * 1024 * 1024:
                        raise ValueError("download exceeds research size limit")
                    record.update(status=response.status, bytes=len(data))
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(data)
                return data
            except OSError as err:
                record["errorType"] = type(err).__name__
                if isinstance(err, urllib.error.HTTPError):
                    record["status"] = err.code
                if attempt == 3:
                    raise
            finally:
                ledger.append(record)
                save(ROOT / "evidence" / "downloads.json", ledger)
            time.sleep(2 ** attempt)

    builds = []
    for platform in ("linux-arm64", "linux-amd64"):
        manifest = json.loads(fetch(BASE + "/" + platform + "/list.json",
                                    args.cache / platform / "list.json"))
        for version in VERSIONS:
            filename = manifest["releases"].get(version)
            if filename is None:
                builds.append({"platform": platform, "version": version, "available": False})
                continue
            build = next(b for b in manifest["builds"] if b["path"] == filename)
            binary = args.cache / platform / filename
            data = fetch(BASE + "/" + platform + "/" + filename, binary)
            digest = "0x" + hashlib.sha256(data).hexdigest()
            if digest != build["sha256"]:
                raise ValueError("compiler checksum mismatch")
            binary.chmod(0o755)
            builds.append({"platform": platform, "version": version, "available": True,
                           "path": filename, "sha256": digest, "bytes": len(data)})
            save(ROOT / "evidence" / "native-builds.json", builds)
            print(json.dumps(builds[-1]), flush=True)
    save(ROOT / "evidence" / "native-builds.json", builds)


if __name__ == "__main__":
    main()
