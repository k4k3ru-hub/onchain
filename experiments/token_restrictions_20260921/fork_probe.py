"""Bounded historical Base fork experiment; every transaction stays on localhost.

This is a research harness, not a general-purpose honeypot classifier. The public
RPC proxy rejects writes and records every upstream attempt, including retries.
"""

from collections import Counter
from http.server import BaseHTTPRequestHandler, HTTPServer
import json
from pathlib import Path
import subprocess
import threading
import time
import urllib.request

from probe import Capture, READ_METHODS, TOKENS


IMAGE = "ghcr.io/foundry-rs/foundry@sha256:043752653d5be351c71709091b3db97c4421c907eb40ea294195e7f532aadf46"
TOKEN = TOKENS["zebra_reported"]
WETH = TOKENS["weth_control"]
ROUTER = "0xfcd3842f85ed87ba2889b4d35893403796e67ff1"
POOL = "0x123bffbacb6fce5e1696bcc1492f2c5e2bee4f09"
OWNER = "0xf01e51504a9f5020f6f46cc0bc76c1d2efb40ec0"
ORDINARY = "0x1000000000000000000000000000000000000001"
RECIPIENT = "0x1000000000000000000000000000000000000002"
LOCAL_URL = "http://127.0.0.1:18545"
PROXY_PORT = 18546
CONTAINER = "k4k3ru-token-restrictions-20260921"


def word(value):
    if isinstance(value, str):
        value = int(value, 16)
    return f"{value:064x}"


def data(selector, *values):
    return selector + "".join(word(value) for value in values)


def cacheable(method, params, response):
    """Cache only successful chain identity or explicitly pinned state reads."""
    if "error" in response or response.get("result") is None:
        return False
    if method in {"eth_chainId", "net_version"}:
        return True
    if method == "eth_getBlockByHash":
        return True
    if method == "eth_getBlockByNumber":
        return bool(params) and isinstance(params[0], str) and params[0].startswith("0x")
    if method in {"eth_getCode", "eth_getStorageAt", "eth_getBalance",
                  "eth_getTransactionCount", "eth_call", "eth_getProof"}:
        return len(params) >= 2 and isinstance(params[-1], str) and params[-1].startswith("0x")
    return False


class MeteredProxy:
    def __init__(self, capture, budget=250):
        self.capture = capture
        self.budget = budget
        self.cache = {}
        self.cache_path = capture.output.parent / "token-restrictions-20260921-rpc-cache.json"
        if self.cache_path.exists():
            self.cache = json.loads(self.cache_path.read_text())
        self.hits = 0
        self.failed = False
        self.last_request = 0

    def forward(self, request):
        method, params = request.get("method"), request.get("params", [])
        if method not in READ_METHODS:
            return {"jsonrpc": "2.0", "id": request.get("id"),
                    "error": {"code": -32601, "message": "read-only proxy"}}
        key = json.dumps([method, params], sort_keys=True)
        if key in self.cache:
            self.hits += 1
            response = dict(self.cache[key])
        elif self.failed:
            response = {"error": {"code": -32001, "message": "upstream unavailable; experiment stopped"}}
        else:
            response = None
            # Initial attempt plus three retries, counting every upstream call.
            for attempt in range(4):
                if len(self.capture.requests) >= self.budget:
                    break
                time.sleep(max(0, 0.35 - (time.monotonic() - self.last_request)))
                self.last_request = time.monotonic()
                response = self.capture.rpc(method, params)
                if response is not None and "result" in response:
                    # Never persist live-head queries or missing receipts.
                    if cacheable(method, params, response):
                        self.cache[key] = dict(response)
                        self.cache_path.write_text(json.dumps(self.cache, indent=2) + "\n")
                    break
                if attempt < 3:
                    time.sleep(2 ** (attempt + 1))
            if response is None or "result" not in response:
                self.failed = True
                response = response or {"error": {"code": -32001, "message": "upstream unavailable"}}
        return {**response, "jsonrpc": "2.0", "id": request.get("id")}


class LocalFork:
    def __init__(self, capture):
        self.capture = capture
        self.calls = []

    def rpc(self, method, params):
        request = {"jsonrpc": "2.0", "id": len(self.calls) + 1, "method": method, "params": params}
        start = time.monotonic()
        with urllib.request.urlopen(urllib.request.Request(
            LOCAL_URL, json.dumps(request).encode(), {"Content-Type": "application/json"}
        ), timeout=180) as response:
            value = json.load(response)
        self.calls.append({"method": method, "params": params, "response": value,
                           "elapsedMs": round((time.monotonic() - start) * 1000, 3)})
        self.capture.save("local_calls.json", self.calls)
        return value

    def result(self, method, params):
        response = self.rpc(method, params)
        if "error" in response:
            raise RuntimeError(f"local {method} failed: {response['error']}")
        return response["result"]

    def call(self, target, calldata, sender=ORDINARY):
        return self.rpc("eth_call", [{"from": sender, "to": target, "data": calldata,
                                      "gas": hex(8_000_000)}, "latest"])

    def transact(self, target, calldata, sender=ORDINARY, value=0):
        tx = {"from": sender, "to": target, "data": calldata, "value": hex(value),
              "gas": hex(8_000_000)}
        tx_hash = self.result("eth_sendTransaction", [tx])
        self.result("evm_mine", [])
        for _ in range(180):
            receipt = self.result("eth_getTransactionReceipt", [tx_hash])
            if receipt is not None:
                return receipt
            time.sleep(0.5)
        raise RuntimeError("local transaction was not mined within 90 seconds")

    def balance(self, token, account):
        return int(self.result("eth_call", [{"to": token,
            "data": data("0x70a08231", account)}, "latest"]), 16)


def swap_data(buy, account, amount, deadline):
    if buy:
        return data("0x7ff36ab5", 0, 128, account, deadline, 2, WETH, TOKEN)
    return data("0x18cbafe5", amount, 0, 160, account, deadline, 2, TOKEN, WETH)


def scenario(fork, account, label, capture):
    print("scenario", label, "starting", flush=True)
    fork.result("anvil_setBalance", [account, hex(10 * 10**18)])
    approved = fork.transact(TOKEN, data("0x095ea7b3", ROUTER, 2**256 - 1), account)
    if approved["status"] != "0x1":
        raise RuntimeError("approval failed locally")
    before = fork.balance(TOKEN, account)
    timestamp = int(fork.result("eth_getBlockByNumber", ["latest", False])["timestamp"], 16)
    deadline = timestamp + 86400
    buy = fork.transact(ROUTER, swap_data(True, account, 0, deadline), account, 10**14)
    amount = fork.balance(TOKEN, account) - before
    result = {"account": account, "buyReceipt": buy, "receivedRaw": str(amount)}
    result["buyTrace"] = fork.rpc("debug_traceTransaction", [buy["transactionHash"], {"tracer": "callTracer"}])
    result["buyStateChanges"] = fork.rpc("debug_traceTransaction", [buy["transactionHash"],
        {"tracer": "prestateTracer", "tracerConfig": {"diffMode": True}}])
    capture.save(label + ".json", result)
    print("scenario", label, "buy", buy["status"], "received", amount, flush=True)
    if buy["status"] != "0x1" or amount <= 0:
        result["status"] = "inconclusive_buy_failed"
        capture.save(label + ".json", result)
        return result
    sell = {"from": account, "to": ROUTER,
            "data": swap_data(False, account, amount, deadline), "gas": hex(8_000_000)}
    result["balanceBeforeSellRaw"] = str(fork.balance(TOKEN, account))
    result["allowanceBeforeSellRaw"] = str(int(fork.result("eth_call", [{"to": TOKEN,
        "data": data("0xdd62ed3e", account, ROUTER)}, "latest"]), 16))
    result["sellSameBlock"] = fork.rpc("eth_call", [sell, "latest"])
    fork.result("anvil_mine", ["0x1"])
    result["sellNextBlock"] = fork.rpc("eth_call", [sell, "latest"])
    # Keep the complete call tree to distinguish token rejection from liquidity,
    # slippage, approval, deadline, or router failures.
    result["sellTraceNextBlock"] = fork.rpc("debug_traceCall", [sell, "latest", {"tracer": "callTracer"}])
    result["directTransferNextBlock"] = fork.call(TOKEN, data("0xa9059cbb", RECIPIENT, amount), account)
    if label == "ordinary" and "error" in result["sellNextBlock"]:
        # Change one observed 0->1 token slot in an isolated counterfactual.
        # Preserve token balances and pool state; never change remote storage.
        changes = result["buyStateChanges"].get("result", {})
        pre = changes.get("pre", {}).get(TOKEN, {}).get("storage", {})
        post = changes.get("post", {}).get(TOKEN, {}).get("storage", {})
        result["restrictionSlotCounterfactuals"] = []
        candidates = [slot for slot, value in post.items()
                      if int(value, 16) == 1 and int(pre.get(slot, "0x0"), 16) == 0]
        for slot in candidates[:4]:
            snapshot = fork.result("evm_snapshot", [])
            fork.result("anvil_setStorageAt", [TOKEN, slot, "0x" + word(0)])
            result["restrictionSlotCounterfactuals"].append({"slot": slot,
                "balanceUnchanged": fork.balance(TOKEN, account) == int(result["balanceBeforeSellRaw"]),
                "sellWithOnlySlotCleared": fork.rpc("eth_call", [sell, "latest"])})
            if not fork.result("evm_revert", [snapshot]):
                raise RuntimeError("counterfactual snapshot restore failed")
        fork.result("anvil_mine", ["0x40"])
        result["sellAfter65Blocks"] = fork.rpc("eth_call", [sell, "latest"])
    result["sellReceipt"] = fork.transact(ROUTER, sell["data"], account)
    result["balanceAfterSellRaw"] = str(fork.balance(TOKEN, account))
    if label == "ordinary" and result["sellReceipt"]["status"] == "0x0":
        result["allowlistOrdinaryCall"] = fork.call(TOKEN, data("0x9dd21928", account, 1), account)
        fork.result("anvil_setBalance", [OWNER, hex(10 * 10**18)])
        result["allowlistOwnerReceipt"] = fork.transact(TOKEN, data("0x9dd21928", account, 1), OWNER)
        result["sellAfterAllowlist"] = fork.rpc("eth_call", [sell, "latest"])
        result["sellAfterAllowlistReceipt"] = fork.transact(ROUTER, sell["data"], account)
        result["balanceAfterAllowlistSellRaw"] = str(fork.balance(TOKEN, account))
    capture.save(label + ".json", result)
    print("scenario", label, "same_block", "error" not in result["sellSameBlock"],
          "next_block", "error" not in result["sellNextBlock"],
          "sell_status", result["sellReceipt"]["status"], flush=True)
    return result


def run(output, block):
    capture = Capture(output)
    proxy = MeteredProxy(capture)
    started = time.monotonic()

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_args):
            pass

        def do_POST(self):
            length = int(self.headers.get("Content-Length", "0"))
            if not 0 < length <= 1_000_000:
                self.send_error(413)
                return
            try:
                request = json.loads(self.rfile.read(length))
                response = [proxy.forward(item) for item in request] if isinstance(request, list) else proxy.forward(request)
                body = json.dumps(response).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
            except (ValueError, TypeError, BrokenPipeError, ConnectionResetError) as error:
                print("proxy error", type(error).__name__, flush=True)

    server = HTTPServer(("127.0.0.1", PROXY_PORT), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    container_started = False
    fork = LocalFork(capture)
    try:
        subprocess.run(["docker", "run", "--detach", "--rm", "--name", CONTAINER,
            "--publish", "127.0.0.1:18545:8545", "--entrypoint", "anvil", IMAGE,
            "--host", "0.0.0.0", "--accounts", "0", "--quiet", "--auto-impersonate",
            "--no-mining",
            "--fork-url", f"http://host.docker.internal:{PROXY_PORT}",
            "--fork-block-number", str(block), "--chain-id", "8453",
            "--retries", "0", "--timeout", "120000", "--no-storage-caching",
            "--disable-default-create2-deployer", "--optimism"], check=True, capture_output=True)
        container_started = True
        for _ in range(90):
            try:
                fork.result("eth_chainId", [])
                break
            except (OSError, ValueError):
                time.sleep(1)
        else:
            raise RuntimeError("local fork did not become ready")
        print("fork ready", block, flush=True)
        snapshot = fork.result("evm_snapshot", [])
        ordinary = scenario(fork, ORDINARY, "ordinary", capture)
        if not fork.result("evm_revert", [snapshot]):
            raise RuntimeError("baseline snapshot restore failed")
        owner = scenario(fork, OWNER, "owner", capture)
        # Benign transfer control at the same historical state. This does not
        # claim a WETH DEX round-trip, only ERC-20 transfer behavior.
        fork.result("anvil_setBalance", [ORDINARY, hex(10**18)])
        deposited = fork.transact(WETH, "0xd0e30db0", value=10**14)
        fork.result("anvil_mine", ["0x1"])
        transferred = fork.transact(WETH, data("0xa9059cbb", RECIPIENT, 10**14))
        capture.save("weth_control.json", {"depositReceipt": deposited, "transferReceipt": transferred})
        from assess import assess
        capture.save("assessment.json", assess(ordinary, owner, TOKEN))
    finally:
        if container_started:
            logs = subprocess.run(["docker", "logs", CONTAINER], capture_output=True, text=True)
            (Path(output) / "anvil.log").write_text(logs.stdout + logs.stderr)
            stopped = subprocess.run(["docker", "stop", CONTAINER], capture_output=True, text=True)
            if stopped.returncode != 0:
                print("container cleanup failed", stopped.stderr, flush=True)
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)
        capture.save("fork_summary.json", {"forkBlock": block, "image": IMAGE,
            "rpcAttempts": len(capture.requests), "cacheHits": proxy.hits,
            "rpcByMethod": dict(Counter(row["identity"]["method"] for row in capture.requests)),
            "rpcErrors": sum("error" in row or "rpcError" in row for row in capture.requests),
            "rpcElapsedMs": round(sum(row["elapsedMs"] for row in capture.requests), 3),
            "wallElapsedMs": round((time.monotonic() - started) * 1000, 3),
            "upstreamStopped": proxy.failed, "localCalls": len(fork.calls)})
