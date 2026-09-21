"""Follow live NFT liquidity positions to their actual owners at the fixed block."""
import argparse
import collections
import json
from pathlib import Path

from activity import words, signed
from batch import batch, call_data
from census import END_BLOCK, END_HASH, Reader, RPCFailure, header, topic, write
from snapshot import STATE_VIEW

MANAGERS = {"uniswap-v3": "0x03a520b32c04bf3beef7beb72e919cf822ed34f1",
    "aerodrome": "0xe1f8cd9ac4e4a65f54f38a5cdafca44f6dd68b53", "uniswap-v4": "0x7c5f5a4bbd8fd63184577525326123b519429bdc"}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    reader = Reader(args.output)
    census = {r["pool"]: r for r in json.loads((args.input / "census.json").read_text())["pools"]}
    rows = [r for r in json.loads((args.input / "activity.json").read_text())["pools"] if r["swap_count"] and r["positions"]]
    if header(reader, END_BLOCK, refresh=True)["hash"] != END_HASH:
        raise ValueError("initial_hash_mismatch")
    observations = []
    transfer_topic = topic("IncreaseLiquidity(uint256,uint128,uint256,uint256)")
    for row in rows:
        if row["venue"] == "uniswap-v4":
            for pos in row["positions"]:
                if pos["owner"] == MANAGERS["uniswap-v4"]:
                    observations.append({"pool": row["pool"], "venue": row["venue"], "manager": pos["owner"], "token_id": int(pos["salt"], 16), "position": pos})
            continue
        ids = set()
        # One currently withdrawable share is enough to disprove 100% protection.
        # This bound does not claim to discover every NFT or certify a full ratio.
        for tx in row["first_mint_transactions"][:8]:
            try:
                receipt = reader.rpc("eth_getTransactionReceipt", [tx])
                if not receipt or receipt["status"] != "0x1":
                    continue
                for event in receipt["logs"]:
                    if event["address"].lower() == MANAGERS[row["venue"]] and event["topics"][0] == transfer_topic:
                        ids.add(int(event["topics"][1], 16))
            except RPCFailure:
                continue
        if ids:
            results = batch(reader, [(MANAGERS[row["venue"]], call_data("positions(uint256)", id_)) for id_ in sorted(ids)])
            pool = census[row["pool"]]
            for id_, result in zip(sorted(ids), results):
                if not result["success"]:
                    continue
                fields = words(result["data"])
                if len(fields) != 12:
                    continue
                if "0x" + fields[2][-40:] != pool["token0"] or "0x" + fields[3][-40:] != pool["token1"] or int(fields[4], 16) != int(pool["creation_event"]["topics"][3], 16):
                    continue
                if int(fields[7], 16) > 0:
                    observations.append({"pool": row["pool"], "venue": row["venue"], "manager": MANAGERS[row["venue"]], "token_id": id_,
                        "position": {"lower": signed(fields[5]), "upper": signed(fields[6]), "liquidity": str(int(fields[7], 16))}, "position_result": result})
        print(json.dumps({"v3_pool": row["pool"], "discovered_ids": len(ids)}), flush=True)
    calls = []
    for row in observations:
        calls.extend([(row["manager"], call_data("ownerOf(uint256)", row["token_id"])), (row["manager"], call_data("getApproved(uint256)", row["token_id"]))])
    results = batch(reader, calls)
    for row, owner, approved in zip(observations, results[::2], results[1::2]):
        row.update(owner_result=owner, approved_result=approved)
        if owner["success"] and len(owner["data"]) == 66:
            row["owner"] = "0x" + owner["data"][-40:]
    v4 = [r for r in observations if r["venue"] == "uniswap-v4"]
    results = batch(reader, [(STATE_VIEW, call_data("getPositionInfo(bytes32,address,int24,int24,bytes32)", r["pool"], r["manager"], r["position"]["lower"], r["position"]["upper"], r["position"]["salt"])) for r in v4])
    for row, result in zip(v4, results):
        row["position_result"] = result
        if result["success"] and len(result["data"]) == 194:
            row["current_liquidity"] = str(int(result["data"][2:66], 16))
    codes = {}
    for owner in sorted({r["owner"] for r in observations if "owner" in r}):
        try:
            codes[owner] = reader.rpc("eth_getCode", [owner, hex(END_BLOCK)])
        except RPCFailure:
            codes[owner] = None
    if header(reader, END_BLOCK, refresh=True)["hash"] != END_HASH:
        raise ValueError("final_hash_mismatch")
    result = {"block": END_BLOCK, "block_hash": END_HASH, "positions": observations, "owner_codes": codes,
        "scope": "positive replayed positions of pools with observed swap; v3 uses at most eight mint receipts per pool"}
    write(args.output / "custody.json", result)
    print(json.dumps({"positions": len(observations), "owners": collections.Counter(r.get("owner", "unavailable") for r in observations)}, indent=2))


if __name__ == "__main__":
    main()
