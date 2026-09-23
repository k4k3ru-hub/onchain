"""Read-only, bounded public Base RPC evidence collection; never sends transactions."""

import argparse
import json
from pathlib import Path
import time
import urllib.error
import urllib.request


ALLOWED = {"eth_chainId", "eth_getBlockByNumber", "eth_getTransactionByHash",
           "eth_getTransactionReceipt", "eth_getCode", "eth_getLogs", "eth_call",
           "eth_getTransactionCount"}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("plan", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    plan = json.loads(args.plan.read_text())
    evidence = json.loads(args.output.read_text()) if args.output.exists() else {"attempts": [], "results": {}}
    started = time.monotonic()
    last = 0.0
    for item in plan:
        name, method, params = item["name"], item["method"], item["params"]
        if method not in ALLOWED:
            raise ValueError("method is not read-only allowlisted")
        prior = evidence["results"].get(name)
        if prior:
            if prior["method"] != method or prior["params"] != params:
                raise ValueError("cached identity mismatch")
            continue
        previous = [v for v in evidence["attempts"] if v["name"] == name]
        for attempt in range(len(previous), 4):
            if len(evidence["attempts"]) >= 128 or time.monotonic() - started >= 100:
                raise RuntimeError("acquisition budget exceeded")
            delay = max(0.0, 2.5 - (time.monotonic() - last))
            if attempt:
                delay = max(delay, min(2 ** attempt, 8))
            if time.monotonic() - started + delay + 20 > 120:
                raise RuntimeError("acquisition time budget exceeded")
            time.sleep(delay)
            last = time.monotonic()
            request = urllib.request.Request("https://mainnet.base.org", data=json.dumps({"jsonrpc": "2.0", "id": len(evidence["attempts"])+1, "method": method, "params": params}).encode(), headers={"Content-Type": "application/json", "User-Agent": "k4k3ru-lp-evidence-probe/1"})
            record = {"name": name, "method": method, "params": params, "attempt": attempt + 1}
            try:
                with urllib.request.urlopen(request, timeout=20) as response:
                    raw = response.read(4 * 1024 * 1024 + 1)
                if len(raw) > 4 * 1024 * 1024:
                    raise ValueError("response too long")
                value = json.loads(raw)
                record["response_bytes"] = len(raw)
                if "error" in value:
                    record["rpc_error"] = value["error"]
                elif value.get("result") is None:
                    record["error"] = "null_result"
                else:
                    evidence["results"][name] = {"method": method, "params": params, "result": value["result"]}
                    record["success"] = True
            except urllib.error.HTTPError as exc:
                record["http_status"] = exc.code
            except (urllib.error.URLError, TimeoutError, ValueError) as exc:
                record["error"] = type(exc).__name__
            record["elapsed_seconds"] = round(time.monotonic() - last, 3)
            evidence["attempts"].append(record)
            args.output.write_text(json.dumps(evidence, indent=2) + "\n")
            print(json.dumps({k: v for k, v in record.items() if k != "params"}), flush=True)
            if record.get("success"):
                break
        if name not in evidence["results"]:
            raise RuntimeError(f"acquisition incomplete after retries: {name}")
    print(json.dumps({"actual_attempts": len(evidence["attempts"]), "cached_results": len(evidence["results"]), "run_seconds": round(time.monotonic()-started, 3)}))


if __name__ == "__main__":
    main()
