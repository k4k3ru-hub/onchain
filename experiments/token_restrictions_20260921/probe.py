"""Read-only Base evidence capture for the token restriction experiment.

The public RPC allowlist deliberately excludes all transaction submission calls.
No signer, wallet file, or credential is read. Fork mutations are local only.
"""

import argparse
import json
from pathlib import Path
import time
import urllib.error
import urllib.request


RPC_URL = "https://mainnet.base.org"
TOKENS = {
    "zebra_reported": "0xac6b1693f547a6235a40c1559b287eab9ee4e167",
    "base_verified_family": "0xdb72d11dbc54f24c9e61ee420d3635b5bcb712c7",
    "base_verified_family_2": "0x7387856052de6414ef805c0487281d9b2725e844",
    "weth_control": "0x4200000000000000000000000000000000000006",
}
REMOVAL_TX = "0x0e29cf79485da7847b0272092847b7cad1cd3e778ff60f8cd45aa64656161580"
READ_METHODS = {
    "eth_chainId", "eth_blockNumber", "eth_getBlockByNumber", "eth_getBlockByHash",
    "eth_getCode", "eth_getStorageAt", "eth_getBalance", "eth_getTransactionCount",
    "eth_getTransactionReceipt", "eth_getTransactionByHash", "eth_call", "eth_getLogs",
    "eth_getProof", "net_version",
}


class Capture:
    def __init__(self, output):
        self.output = Path(output)
        self.output.mkdir(parents=True, exist_ok=True)
        self.requests = []
        self.save("requests.json", self.requests)

    def request(self, url, payload=None, category="source", identity=None):
        start = time.monotonic()
        row = {"category": category, "identity": identity or url}
        try:
            encoded = None if payload is None else json.dumps(payload).encode()
            request = urllib.request.Request(url, data=encoded, headers={
                "User-Agent": "K4K3RU-read-only-research/1.0",
                "Content-Type": "application/json", "Accept": "application/json,text/html",
            })
            with urllib.request.urlopen(request, timeout=20) as response:
                body = response.read(8 * 1024 * 1024 + 1)
                row["httpStatus"] = response.status
                row["bytes"] = len(body)
            if len(body) > 8 * 1024 * 1024:
                raise ValueError("response exceeds experiment byte limit")
            return body
        except urllib.error.HTTPError as error:
            row["httpStatus"] = error.code
            row["error"] = "http_error"
            return None
        except (urllib.error.URLError, TimeoutError, ValueError) as error:
            row["error"] = type(error).__name__
            return None
        finally:
            row["elapsedMs"] = round((time.monotonic() - start) * 1000, 3)
            self.requests.append(row)
            self.save("requests.json", self.requests)

    def rpc(self, method, params):
        if method not in READ_METHODS:
            raise ValueError("remote rpc method is not read-only")
        payload = {"jsonrpc": "2.0", "id": len(self.requests) + 1, "method": method, "params": params}
        raw = self.request(RPC_URL, payload, "rpc", {"method": method, "params": params})
        if raw is None:
            return None
        response = json.loads(raw)
        if "error" in response:
            self.requests[-1]["rpcError"] = response["error"]
            self.save("requests.json", self.requests)
        return response

    def save(self, name, data):
        (self.output / name).write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n")


def capture(output):
    probe = Capture(output)
    results = {}
    for method, params, name in [
        ("eth_chainId", [], "chain_id"),
        ("eth_getTransactionReceipt", [REMOVAL_TX], "removal_receipt"),
        ("eth_blockNumber", [], "head"),
    ]:
        response = probe.rpc(method, params)
        results[name] = response
        probe.save(name + ".json", response)
        print(name, "available" if response and "result" in response else "unavailable", flush=True)
    for name, token in TOKENS.items():
        response = probe.rpc("eth_getCode", [token, "latest"])
        probe.save(name + "_code.json", response)
        print(name, "code_bytes", (len(response.get("result", "0x")) - 2) // 2 if response else None, flush=True)
    for name in ["base_verified_family", "base_verified_family_2", "zebra_reported"]:
        token = TOKENS[name]
        urls = {
            "basescan": f"https://basescan.org/address/{token}#code",
            "sourcify": f"https://repo.sourcify.dev/contracts/partial_match/8453/{token}/metadata.json",
        }
        for provider, url in urls.items():
            raw = probe.request(url)
            if raw is not None:
                (probe.output / f"{name}_{provider}.txt").write_bytes(raw)
            print(name, provider, "bytes", len(raw) if raw is not None else None, flush=True)
    probe.save("summary.json", {"rpcAttempts": sum(r["category"] == "rpc" for r in probe.requests),
        "sourceAttempts": sum(r["category"] == "source" for r in probe.requests),
        "elapsedMs": sum(r["elapsedMs"] for r in probe.requests)})


def inspect(output):
    probe = Capture(output)
    results = {}
    for name, token, block in [
        ("zebra_historical", TOKENS["zebra_reported"], hex(2032747)),
        ("family_latest", TOKENS["base_verified_family"], "latest"),
        ("family_2_latest", TOKENS["base_verified_family_2"], "latest"),
    ]:
        details = {}
        for field, selector in {"owner": "0x8da5cb5b", "lp": "0xb6fccf8a",
                                "name": "0x06fdde03", "symbol": "0x95d89b41"}.items():
            details[field] = probe.rpc("eth_call", [{"to": token, "data": selector}, block])
        lp = (details.get("lp") or {}).get("result", "0x")
        if len(lp) == 66:
            details["reserves"] = probe.rpc("eth_call", [{"to": "0x" + lp[-40:], "data": "0x0902f1ac"}, block])
        results[name] = details
        probe.save("inspection.json", results)
        print(name, json.dumps(details), flush=True)


def history(output):
    probe = Capture(output)
    response = probe.rpc("eth_getLogs", [{"address": "0x123bffbacb6fce5e1696bcc1492f2c5e2bee4f09",
        "fromBlock": hex(2030747), "toBlock": hex(2032747)}])
    probe.save("pool_logs.json", response)
    if response and "result" in response:
        rows = response["result"]
        print("pool_logs", len(rows), "last_blocks", sorted({int(row['blockNumber'], 16) for row in rows})[-15:])


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", required=True)
    parser.add_argument("--mode", choices=("capture", "inspect", "history", "fork"), default="capture")
    parser.add_argument("--fork-block", type=int, default=2032290)
    args = parser.parse_args()
    if args.mode == "capture":
        capture(args.output)
    elif args.mode == "inspect":
        inspect(args.output)
    elif args.mode == "history":
        history(args.output)
    else:
        from fork_probe import run
        run(args.output, args.fork_block)


if __name__ == "__main__":
    main()
