"""Read block-pinned current liquidity and price for every EVM cohort pool."""
import argparse
import collections
import json
from pathlib import Path

from batch import batch, call_data
from census import END_BLOCK, END_HASH, Reader, RPCFailure, header, write

STATE_VIEW = "0xa3c0c9b65bad0b08107aa264b0f3db444b867a71"


def run(reader, pools):
    if header(reader, END_BLOCK, refresh=True)["hash"] != END_HASH:
        raise ValueError("initial_hash_mismatch")
    rows = []
    evm = [p for p in pools if p["chain"] == "base"]
    for offset in range(0, len(evm), 50):
        group = evm[offset:offset + 50]
        calls = []
        for p in group:
            if p["venue"] == "uniswap-v4":
                calls.extend([(STATE_VIEW, call_data("getSlot0(bytes32)", p["pool"])), (STATE_VIEW, call_data("getLiquidity(bytes32)", p["pool"]))])
            else:
                calls.extend([(p["pool"], call_data("slot0()")), (p["pool"], call_data("liquidity()"))])
        try:
            results = batch(reader, calls, size=100)
        except RPCFailure as exc:
            rows.extend({"pool": p["pool"], "venue": p["venue"], "reason": str(exc)} for p in group)
            continue
        for p, slot, liquidity in zip(group, results[::2], results[1::2]):
            row = {"pool": p["pool"], "venue": p["venue"], "slot_result": slot, "liquidity_result": liquidity}
            if slot["success"] and len(slot["data"]) >= 130 and liquidity["success"] and len(liquidity["data"]) == 66:
                row.update(sqrt_price_x96=str(int(slot["data"][2:66], 16)), active_liquidity=str(int(liquidity["data"], 16)))
            else:
                row["reason"] = "state_read_failed"
            rows.append(row)
        write(reader.output / "snapshot.json", {"block": END_BLOCK, "block_hash": END_HASH, "pools": rows, "complete": False})
        print(json.dumps({"snapshot_pools": len(rows), "total": len(evm)}), flush=True)
    if header(reader, END_BLOCK, refresh=True)["hash"] != END_HASH:
        raise ValueError("final_hash_mismatch")
    return {"block": END_BLOCK, "block_hash": END_HASH, "pools": rows, "complete": True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--census", type=Path, required=True)
    args = parser.parse_args()
    result = run(Reader(args.output), json.loads(args.census.read_text())["pools"])
    write(args.output / "snapshot.json", result)
    stats = collections.defaultdict(collections.Counter)
    for row in result["pools"]:
        stats[row["venue"]]["read_failed" if "reason" in row else "active_positive" if int(row["active_liquidity"]) else "active_zero"] += 1
    print(json.dumps(stats, indent=2))


if __name__ == "__main__":
    main()
