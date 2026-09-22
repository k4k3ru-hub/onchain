"""Bounded research I/O. Remote RPC is read-only; successful pinned reads cache.

No production imports, credentials, private keys, or third-party Python modules.
The ledger records attempts as well as logical requests and cache hits.
"""

from collections import Counter
import json
from pathlib import Path
import time
import urllib.error
import urllib.request


READ_METHODS = {"eth_chainId", "eth_getBlockByNumber", "eth_getCode",
                "eth_getStorageAt", "eth_call"}


class Reader:
    def __init__(self, url, budget=100, delay=0.35, sleep=time.sleep, transport=None):
        self.url = url
        self.budget = budget
        self.delay = delay
        self.sleep = sleep
        self.transport = transport or self._http
        self.cache = {}
        self.ledger = []
        self.logical = 0
        self.hits = 0

    def _http(self, method, params):
        request = urllib.request.Request(self.url, json.dumps({
            "jsonrpc": "2.0", "id": 1, "method": method, "params": params,
        }).encode(), {"Content-Type": "application/json", "User-Agent": "K4K3RU-read-only-research/1.0"})
        with urllib.request.urlopen(request, timeout=20) as response:
            data = json.loads(response.read(8 * 1024 * 1024))
        if "error" in data or data.get("result") is None:
            raise ValueError("failed to read rpc response: error or missing result")
        return data["result"]

    def rpc(self, method, params, cache=False):
        if method not in READ_METHODS:
            raise ValueError("failed to read rpc: method not allowed")
        self.logical += 1
        key = json.dumps([method, params], sort_keys=True)
        if cache and key in self.cache:
            self.hits += 1
            return self.cache[key]
        for attempt in range(4):
            if len(self.ledger) >= self.budget:
                raise ValueError("failed to read rpc: experiment budget exhausted")
            self.sleep(self.delay)
            start = time.monotonic()
            record = {"method": method, "params": params, "attempt": attempt + 1}
            try:
                result = self.transport(method, params)
                record["result"] = result
                if cache:
                    self.cache[key] = result
                return result
            except (OSError, ValueError) as exc:
                record["errorType"] = type(exc).__name__
                if isinstance(exc, urllib.error.HTTPError):
                    record["status"] = exc.code
                if attempt == 3:
                    raise ValueError("failed to read rpc: retries exhausted") from exc
            finally:
                record["elapsedMs"] = round((time.monotonic() - start) * 1000, 3)
                self.ledger.append(record)
            self.sleep(2 ** attempt)

    def pin(self, number):
        # Always reconfirm canonical number->hash mapping, including warm runs.
        return self.rpc("eth_getBlockByNumber", [number, False])

    def state(self, method, args, block):
        # EIP-1898 prevents combining state from different versions of a block.
        ref = {"blockHash": block["hash"], "requireCanonical": True}
        return self.rpc(method, [*args, ref], cache=True)

    def stats(self):
        return {"logical": self.logical, "httpAttempts": len(self.ledger),
                "cacheHits": self.hits,
                "methods": dict(Counter(x["method"] for x in self.ledger)),
                "failedAttempts": sum("errorType" in x for x in self.ledger)}


def fetch_source(address, ledger, sleep=time.sleep):
    url = f"https://sourcify.dev/server/v2/contract/8453/{address}?fields=all"
    for attempt in range(4):
        record = {"url": url, "attempt": attempt + 1}
        try:
            request = urllib.request.Request(url, headers={"User-Agent": "K4K3RU-read-only-research/1.0"})
            with urllib.request.urlopen(request, timeout=20) as response:
                record["status"] = response.status
                return json.loads(response.read(16 * 1024 * 1024))
        except (OSError, ValueError) as exc:
            record["errorType"] = type(exc).__name__
            if isinstance(exc, urllib.error.HTTPError):
                record["status"] = exc.code
            if attempt == 3:
                return None
        finally:
            ledger.append(record)
        sleep(2 ** attempt)


def save(path, value):
    Path(path).write_text(json.dumps(value, indent=2) + "\n")


if __name__ == "__main__":
    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument("--event", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    out = Path(args.output)
    out.mkdir(parents=True, exist_ok=True)
    event = json.loads(Path(args.event).read_text())
    # Deterministic small sample: first V3 pair and first Slipstream pair.
    pairs = []
    for venue in ("uniswap-v3", "aerodrome"):
        pairs.append(next(p for p in event["pairs"] if p["venue"] == venue))
    save(out / "inputs.json", {"eventTimestamp": event["timestamp"], "pairs": pairs})
    reader = Reader("https://mainnet.base.org", budget=50)
    source_ledger = []
    tokens = {}
    try:
        assert reader.rpc("eth_chainId", []) == hex(8453)
        number = hex(max(int(p["lpStatePosition"]["number"]) for p in pairs))
        block = reader.pin(number)
        for pair in pairs:
            for side in ("token0", "token1"):
                address = pair[side]["id"].lower()
                if address in tokens:
                    continue
                code = reader.state("eth_getCode", [address], block)
                source = fetch_source(address, source_ledger)
                tokens[address] = {"code": code, "source": source}
                print(address, "codeBytes", (len(code)-2)//2,
                      "source", source is not None, flush=True)
        assert reader.pin(number)["hash"] == block["hash"]
        save(out / "live.json", {"block": block, "tokens": tokens})
    finally:
        save(out / "live-rpc.json", {"stats": reader.stats(), "ledger": reader.ledger})
        save(out / "source-http.json", source_ledger)
