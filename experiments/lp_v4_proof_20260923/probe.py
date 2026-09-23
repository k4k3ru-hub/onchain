"""Isolated, bounded, read-only Base RPC evidence collection for V4 LP research."""

import argparse
import hashlib
import json
from pathlib import Path
import time
import urllib.error
import urllib.request


ALLOWED = {"eth_chainId", "eth_getBlockByNumber", "eth_getTransactionByHash",
           "eth_getTransactionReceipt", "eth_getCode", "eth_getTransactionCount", "eth_call"}


def collect(plan, output):
    evidence = json.loads(output.read_text()) if output.exists() else {"attempts": [], "results": {}, "runs": []}
    started = time.monotonic()
    run = {"attempts_before": len(evidence["attempts"]), "cache_hits": 0}
    evidence["runs"].append(run)

    def save():
        run["elapsed_seconds"] = round(time.monotonic() - started, 3)
        run["attempts_after"] = len(evidence["attempts"])
        output.write_text(json.dumps(evidence, indent=2) + "\n")

    last = 0.0
    try:
        for item in plan:
            name, method, params = item["name"], item["method"], item["params"]
            if method not in ALLOWED:
                raise ValueError("method is not read-only allowlisted")
            digest = hashlib.sha256(json.dumps([method, params], sort_keys=True).encode()).hexdigest()
            prior = evidence["results"].get(name)
            if prior:
                if prior["method"] != method or prior["params"] != params:
                    raise ValueError("cached identity mismatch")
                run["cache_hits"] += 1
                continue
            count = sum(r["key"] == digest for r in evidence["attempts"] if not r.get("success"))
            for attempt in range(count, 4):
                used = sum(r.get("response_bytes", 0) for r in evidence["attempts"])
                if len(evidence["attempts"]) >= 128 or used >= 2 << 20:
                    raise RuntimeError("acquisition count or response budget exceeded")
                delay = max(0.0, 2.5 - (time.monotonic() - last))
                if attempt:
                    delay = max(delay, 2 ** attempt)
                if time.monotonic() - started + delay + 15 > 120:
                    raise RuntimeError("acquisition time budget exceeded")
                time.sleep(delay)
                last = time.monotonic()
                request = urllib.request.Request(
                    "https://mainnet.base.org",
                    data=json.dumps({"jsonrpc": "2.0", "id": len(evidence["attempts"]) + 1,
                                     "method": method, "params": params}).encode(),
                    headers={"Content-Type": "application/json", "User-Agent": "k4k3ru-v4-lp-proof/1"})
                record = {"name": name, "key": digest, "method": method, "attempt": attempt + 1}
                environment_error = None
                try:
                    with urllib.request.urlopen(request, timeout=15) as response:
                        raw = response.read((2 << 20) - used + 1)
                    record["response_bytes"] = len(raw)
                    if len(raw) + used > 2 << 20:
                        raise ValueError("cumulative response too long")
                    value = json.loads(raw)
                    if "error" in value:
                        record["rpc_error"] = value["error"]
                    elif value.get("result") is None:
                        record["error"] = "null_result"
                    else:
                        evidence["results"][name] = {"method": method, "params": params, "result": value["result"]}
                        record["success"] = True
                except urllib.error.HTTPError as exc:
                    record["http_status"] = exc.code
                    # Error bodies consume the same acquisition budget as successful responses.
                    record["response_bytes"] = len(exc.read((2 << 20) - used + 1))
                except urllib.error.URLError as exc:
                    record["error"] = "transport_unavailable"
                    environment_error = exc
                except (TimeoutError, ValueError) as exc:
                    record["error"] = type(exc).__name__
                record["elapsed_seconds"] = round(time.monotonic() - last, 3)
                evidence["attempts"].append(record)
                save()
                print(json.dumps(record), flush=True)
                if environment_error:
                    raise RuntimeError("transport unavailable; preserve ledger before environment retry") from environment_error
                if record.get("success"):
                    break
            if name not in evidence["results"]:
                raise RuntimeError("acquisition incomplete after retries: " + name)
        run["complete"] = True
    finally:
        save()
    print(json.dumps(run), flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("plan", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    collect(json.loads(args.plan.read_text()), args.output)


if __name__ == "__main__":
    main()
