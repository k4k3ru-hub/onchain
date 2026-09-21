"""Replay cohort liquidity and swap logs, preserving retrieval gaps."""
import argparse
import collections
import json
from pathlib import Path

from census import END_BLOCK, END_HASH, FACTORIES, Reader, RPCFailure, header, topic, write


def words(data):
    if not data.startswith("0x") or (len(data) - 2) % 64:
        raise ValueError("invalid_abi_words")
    return [data[i:i + 64] for i in range(2, len(data), 64)]


def signed(word):
    value = int(word, 16)
    return value - (1 << 256) if value >= (1 << 255) else value


def run(reader, census):
    topics = {name: topic(signature) for name, signature in {
        "modify": "ModifyLiquidity(bytes32,address,int24,int24,int256,bytes32)",
        "v4swap": "Swap(bytes32,address,int128,int128,uint160,uint128,int24,uint24)",
        "mint": "Mint(address,address,int24,int24,uint128,uint256,uint256)",
        "burn": "Burn(address,int24,int24,uint128,uint256,uint256)",
        "swap": "Swap(address,address,int256,int256,uint160,uint128,int24)",
    }.items()}
    evm = [p for p in census["pools"] if p["chain"] == "base"]
    rows = {p["pool"]: {"venue": p["venue"], "pool": p["pool"], "swap_count": 0,
        "liquidity_event_count": 0, "positions": {}, "first_mint_transactions": []} for p in evm}
    gaps = []
    groups = [
        {"name": "v4", "address": FACTORIES["uniswap-v4"], "topics": [topics["modify"], topics["v4swap"]],
         "pool_ids": [p["pool"] for p in evm if p["venue"] == "uniswap-v4"]},
        {"name": "v3_slipstream", "address": [p["pool"] for p in evm if p["venue"] != "uniswap-v4"],
         "topics": [topics["mint"], topics["burn"], topics["swap"]]},
    ]
    if header(reader, END_BLOCK, refresh=True)["hash"] != END_HASH:
        raise ValueError("initial_hash_mismatch")
    for group in groups:
        for start in range(census["base_start_block"], END_BLOCK + 1, 2000):
            stop = min(start + 1999, END_BLOCK)
            query = {"address": group["address"], "topics": [group["topics"]], "fromBlock": hex(start), "toBlock": hex(stop)}
            if "pool_ids" in group:
                query["topics"].append(group["pool_ids"])
            try:
                logs = reader.rpc("eth_getLogs", [query])
            except RPCFailure as exc:
                gaps.append({"group": group["name"], "from_block": start, "to_block": stop, "reason": str(exc)})
                continue
            for event in sorted(logs, key=lambda e: (int(e["blockNumber"], 16), int(e["logIndex"], 16))):
                if event.get("removed") or not start <= int(event["blockNumber"], 16) <= stop:
                    raise ValueError("activity_event_invalid")
                pool = event["topics"][1] if group["name"] == "v4" else event["address"].lower()
                row = rows[pool]
                kind = event["topics"][0]
                if kind in (topics["swap"], topics["v4swap"]):
                    row["swap_count"] += 1
                    row["last_swap"] = event
                    continue
                row["liquidity_event_count"] += 1
                data = words(event["data"])
                owner = "0x" + event["topics"][2 if group["name"] == "v4" else 1][-40:]
                if group["name"] == "v4":
                    lower, upper, delta, salt = signed(data[0]), signed(data[1]), signed(data[2]), "0x" + data[3]
                else:
                    lower, upper, salt = signed(event["topics"][2][2:]), signed(event["topics"][3][2:]), None
                    delta = int(data[1 if kind == topics["mint"] else 0], 16)
                    if kind == topics["burn"]:
                        delta = -delta
                    if kind == topics["mint"] and event["transactionHash"] not in row["first_mint_transactions"]:
                        row["first_mint_transactions"].append(event["transactionHash"])
                key = "/".join(map(str, [owner, lower, upper, salt]))
                pos = row["positions"].setdefault(key, {"owner": owner, "lower": lower, "upper": upper, "salt": salt, "liquidity": 0})
                pos["liquidity"] += delta
                if pos["liquidity"] < 0:
                    row["negative_delta_after_history_gap"] = True
                    if not any(g["group"] == group["name"] for g in gaps):
                        raise ValueError("negative_replayed_liquidity_without_gap")
            print(json.dumps({"group": group["name"], "through_block": stop, "events": len(logs), "gaps": len(gaps)}), flush=True)
    if header(reader, END_BLOCK, refresh=True)["hash"] != END_HASH:
        raise ValueError("final_hash_mismatch")
    for row in rows.values():
        created = next(int(p["creation_event"]["blockNumber"], 16) for p in evm if p["pool"] == row["pool"])
        group_name = "v4" if row["venue"] == "uniswap-v4" else "v3_slipstream"
        row["replay_complete"] = not any(g["group"] == group_name and g["to_block"] >= created for g in gaps)
        row["positions"] = [dict(p, liquidity=str(p["liquidity"])) for p in row["positions"].values() if p["liquidity"] > 0]
        row["has_positive_liquidity"] = bool(row["positions"])
    return {"end_block": END_BLOCK, "end_hash_anchor": END_HASH, "gaps": gaps, "pools": list(rows.values()),
            "rpc_http_attempts": len(reader.requests), "cache_hits": reader.cache_hits}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    result = run(Reader(args.output), json.loads((args.output / "census.json").read_text()))
    write(args.output / "activity.json", result)
    counts = collections.defaultdict(collections.Counter)
    for row in result["pools"]:
        counts[row["venue"]]["created"] += 1
        counts[row["venue"]]["has_swap"] += row["swap_count"] > 0
        counts[row["venue"]]["has_liquidity"] += row["has_positive_liquidity"]
        counts[row["venue"]]["has_both"] += row["has_positive_liquidity"] and row["swap_count"] > 0
    print(json.dumps({"counts": counts, "gaps": result["gaps"], "rpc_http_attempts": result["rpc_http_attempts"]}, indent=2))


if __name__ == "__main__":
    main()
