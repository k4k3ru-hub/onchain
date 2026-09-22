"""Download pinned research compilers, checking official manifest SHA-256."""

import hashlib
import json
from pathlib import Path
import sys
import time
import urllib.error
import urllib.request

from acquire import save


def main():
    target = Path(sys.argv[1])
    target.mkdir(parents=True, exist_ok=True)
    ledger = []

    def get(url):
        for attempt in range(4):
            record = {"url": url, "attempt": attempt + 1}
            try:
                req = urllib.request.Request(url, headers={"User-Agent": "K4K3RU-read-only-research/1.0"})
                with urllib.request.urlopen(req, timeout=30) as response:
                    body = response.read(40 * 1024 * 1024)
                    record.update(status=response.status, size=len(body))
                    return body
            except OSError as exc:
                record["errorType"] = type(exc).__name__
                if isinstance(exc, urllib.error.HTTPError):
                    record["status"] = exc.code
                if attempt == 3:
                    raise
            finally:
                ledger.append(record)
                save("evidence/compiler-http.json", ledger)
            time.sleep(2 ** attempt)

    manifest = json.loads(get("https://binaries.soliditylang.org/bin/list.json"))
    for version in ("0.5.17", "0.8.34", "0.8.37"):
        filename = manifest["releases"][version]
        build = next(x for x in manifest["builds"] if x["path"] == filename)
        code = get("https://binaries.soliditylang.org/bin/" + filename)
        digest = hashlib.sha256(code).hexdigest()
        assert "0x" + digest == build["sha256"]
        (target / filename).write_bytes(code)
        save(target / (version + ".json"), build)
        print(version, digest, flush=True)


if __name__ == "__main__":
    main()
