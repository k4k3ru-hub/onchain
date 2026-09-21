"""Read-only, fixed-window NewPair census; no automatic safety verdicts.

Run from the repository root. Successful block-pinned responses are cached;
provider failures receive three retries and remain explicit gaps thereafter.
"""

import argparse
import collections
import datetime as dt
import hashlib
import functools
import json
from pathlib import Path
import subprocess
import time
import urllib.error
import urllib.request


END_BLOCK = 51596896
END_TIME = 1789983139
END_HASH = "0xacc268320c2198258d4b13b4fb4f90bb810d626b9329fe58ac60e52c55796ba2"
START_TIME = END_TIME - 86400
RPC = "https://mainnet.base.org"
FACTORIES = {
    "uniswap-v3": "0x33128a8fc17869897dce68ed026d694621f6fdfd",
    "aerodrome": "0xf8f2eb4940cfe7d13603dddd87f123820fc061ef",
    "uniswap-v4": "0x498581ff718922c3f8e6a244956af099b2652b2b",
}
SIGNATURES = {
    "uniswap-v3": "PoolCreated(address,address,uint24,int24,address)",
    "aerodrome": "PoolCreated(address,address,int24,address)",
    "uniswap-v4": "Initialize(bytes32,address,address,uint24,int24,address,uint160,int24)",
}


def utc(timestamp):
    return dt.datetime.fromtimestamp(timestamp, dt.timezone.utc).isoformat()


@functools.lru_cache(maxsize=128)
def topic(signature):
    result = subprocess.run(["/tmp/lp-keccak"], input=signature.encode().hex() + "\n",
                            text=True, capture_output=True, check=True)
    return "0x" + result.stdout.strip()


def write(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2) + "\n")


class RPCFailure(RuntimeError):
    pass


class Reader:
    def __init__(self, output):
        self.output = output
        self.ledger_path = output / "requests.json"
        self.requests = json.loads(self.ledger_path.read_text()) if self.ledger_path.exists() else []
        self.cache_hits = 0

    def rpc(self, method, params, refresh=False):
        payload = {"jsonrpc": "2.0", "id": 1, "method": method, "params": params}
        key = hashlib.sha256(json.dumps(payload, sort_keys=True).encode()).hexdigest()
        path = self.output / "rpc" / (key + ".json")
        if path.exists() and not refresh:
            self.cache_hits += 1
            saved = json.loads(path.read_text())
            if saved["payload"] != payload:
                raise ValueError("cache_payload_mismatch")
            if "error" in saved["response"]:
                raise RPCFailure("execution_reverted")
            return saved["response"]["result"]
        if not refresh and any(r["request_key"] == key and r["attempt"] == 4 and not r.get("success") for r in self.requests):
            raise RPCFailure("previous_request_exhausted_three_retries")
        for attempt in range(4):
            time.sleep(0.3)
            entry = {"method": method, "request_key": key, "attempt": attempt + 1}
            started = time.monotonic()
            reverted = False
            try:
                req = urllib.request.Request(RPC, data=json.dumps(payload).encode(), headers={
                    "Content-Type": "application/json", "User-Agent": "k4k3ru-read-only-census/1"})
                with urllib.request.urlopen(req, timeout=40) as response:
                    raw = response.read(24000001)
                if len(raw) > 24000000:
                    raise RPCFailure("response_size_budget")
                data = json.loads(raw)
                entry["response_bytes"] = len(raw)
                if "error" in data:
                    entry["rpc_error_code"] = data["error"].get("code")
                    reverted = data["error"].get("code") == 3
                    if reverted:
                        write(path, {"payload": payload, "response": {
                            "error": {"code": 3, "data": data["error"].get("data")}}})
                    raise RPCFailure("execution_reverted" if reverted else "provider_rejected_request")
                if "result" not in data:
                    raise RPCFailure("missing_result")
                write(path, {"payload": payload, "response": data})
                entry["success"] = True
                return data["result"]
            except urllib.error.HTTPError as exc:
                entry["http_status"] = exc.code
            except (urllib.error.URLError, TimeoutError):
                entry["failure"] = "connection_failed"
            except (RPCFailure, ValueError) as exc:
                entry["failure"] = str(exc) if isinstance(exc, RPCFailure) else "invalid_json"
            finally:
                entry["seconds"] = round(time.monotonic() - started, 3)
                self.requests.append(entry)
                write(self.ledger_path, self.requests)
            if reverted:
                raise RPCFailure("execution_reverted")
            if attempt < 3:
                time.sleep(2 ** (attempt + 1))
        raise RPCFailure("request_failed_after_three_retries")


def header(reader, number, refresh=False):
    value = reader.rpc("eth_getBlockByNumber", [hex(number), False], refresh=refresh)
    if not value or int(value["number"], 16) != number:
        raise ValueError("header_number_mismatch")
    return value


def decode(event, venue, topics):
    if event.get("removed") or event["address"].lower() != FACTORIES[venue] or event["topics"][0] != topics[venue]:
        raise ValueError("creation_event_mismatch")
    if len(event["topics"]) != 4:
        raise ValueError("creation_topics_mismatch")
    words = [event["data"][i:i + 64] for i in range(2, len(event["data"]), 64)]
    row = {"venue": venue, "chain": "base", "network": "mainnet", "creation_event": event,
           "created_at": utc(int(event["blockTimestamp"], 16))}
    if venue == "uniswap-v4":
        if len(words) != 5:
            raise ValueError("v4_creation_layout_mismatch")
        row.update(pool=event["topics"][1], token0="0x" + event["topics"][2][-40:],
                   token1="0x" + event["topics"][3][-40:], fee=int(words[0], 16),
                   tick_spacing=int(words[1], 16), hooks="0x" + words[2][-40:])
    else:
        row.update(pool="0x" + words[-1][-40:], token0="0x" + event["topics"][1][-40:],
                   token1="0x" + event["topics"][2][-40:])
    return row


def census(reader, old):
    end = header(reader, END_BLOCK, refresh=True)
    if end["hash"] != END_HASH or int(end["timestamp"], 16) != END_TIME:
        raise ValueError("frozen_end_hash_mismatch")
    # Verify the expected two-second Base boundary; do not infer time from height.
    start_block = END_BLOCK - 43200
    first, before = header(reader, start_block), header(reader, start_block - 1)
    if not int(before["timestamp"], 16) < START_TIME <= int(first["timestamp"], 16):
        raise ValueError("start_boundary_unverified")
    topics = {venue: topic(signature) for venue, signature in SIGNATURES.items()}
    venues_by_address = {address: venue for venue, address in FACTORIES.items()}
    rows, gaps = [], []
    for start in range(start_block, END_BLOCK, 2000):
        stop = min(start + 1999, END_BLOCK - 1)
        try:
            events = reader.rpc("eth_getLogs", [{"address": list(FACTORIES.values()),
                "topics": [list(topics.values())], "fromBlock": hex(start), "toBlock": hex(stop)}])
        except RPCFailure as exc:
            gaps.append({"from_block": start, "to_block": stop, "reason": str(exc)})
            continue
        for event in events:
            venue = venues_by_address[event["address"].lower()]
            if not start <= int(event["blockNumber"], 16) <= stop:
                raise ValueError("creation_block_out_of_range")
            if "blockTimestamp" not in event:
                event["blockTimestamp"] = header(reader, int(event["blockNumber"], 16))["timestamp"]
            if not START_TIME <= int(event["blockTimestamp"], 16) < END_TIME:
                raise ValueError("creation_time_out_of_range")
            rows.append(decode(event, venue, topics))
        print(json.dumps({"through_block": stop, "counts": dict(collections.Counter(r["venue"] for r in rows)), "gaps": len(gaps)}), flush=True)
    cetus = json.loads((old / "cetus/cetus-clmm-manifest.json").read_text())
    dates = [dt.datetime.fromisoformat(e["timestamp"].replace("Z", "+00:00")).timestamp() for e in cetus["events"]]
    # Last 30 chain events, queried after the window, already reach before its start.
    if min(dates) >= START_TIME or dt.datetime.fromisoformat(cetus["checkpoint"]["timestamp"].replace("Z", "+00:00")).timestamp() < END_TIME:
        raise ValueError("cetus_cached_window_incomplete")
    for event, created in zip(cetus["events"], dates):
        if START_TIME <= created < END_TIME:
            fields = event["contents"]["json"]
            rows.append({"venue": "cetus-clmm", "chain": "sui", "network": "mainnet", "pool": fields["pool_id"],
                "token0": fields["coin_type_a"], "token1": fields["coin_type_b"], "created_at": event["timestamp"], "creation_event": event})
    keys = [(r["venue"], r["pool"]) for r in rows]
    if len(keys) != len(set(keys)):
        raise ValueError("duplicate_pool")
    final = header(reader, END_BLOCK, refresh=True)
    if final["hash"] != END_HASH:
        raise ValueError("final_hash_mismatch")
    return {"window_start_inclusive": utc(START_TIME), "window_end_exclusive": utc(END_TIME),
        "base_start_block": start_block, "base_observed_block": END_BLOCK, "base_observed_hash": END_HASH,
        "cetus_cached_creation_manifest": str(old / "cetus/cetus-clmm-manifest.json"),
        "cetus_cached_observation": cetus["checkpoint"], "counts": dict(collections.Counter(r["venue"] for r in rows)),
        "creation_gaps": gaps, "pools": rows, "read_only": True, "cache_hits": reader.cache_hits,
        "rpc_http_attempts": len(reader.requests)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--previous", type=Path, default=Path("go/internal/experiments/lplock/testdata/venue_samples_20260921_30"))
    args = parser.parse_args()
    reader = Reader(args.output)
    result = census(reader, args.previous)
    write(args.output / "census.json", result)
    print(json.dumps({key: value for key, value in result.items() if key != "pools"}, indent=2))


if __name__ == "__main__":
    main()
