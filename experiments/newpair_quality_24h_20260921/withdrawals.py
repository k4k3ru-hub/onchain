"""Disprove full protection only when a positive owned share can be withdrawn.

All simulations use eth_call; no transaction is signed or broadcast. A revert
or provider failure is inconclusive, never evidence of a lock.
"""
import argparse
import collections
import json
from pathlib import Path

from batch import bytes_value, call_data, word
from census import END_BLOCK, END_HASH, END_TIME, Reader, RPCFailure, header, topic, write
from quality import principal


def v4_withdrawal(row, pool):
    first = b"".join(word(v) for v in [row["token_id"], int(row["current_liquidity"]), 0, 0, 160]) + word(0)
    second = b"".join(word(int(v, 16)) for v in [pool["token0"], pool["token1"], row["owner"]])
    encoded_first, encoded_second = bytes_value(first), bytes_value(second)
    params = word(2) + word(64) + word(64 + len(encoded_first)) + encoded_first + encoded_second
    # v4 Actions: DECREASE_LIQUIDITY=1, TAKE_PAIR=0x11.
    actions = bytes_value(bytes([1, 0x11]))
    unlock = word(64) + word(64 + len(actions)) + actions + params
    return topic("modifyLiquidities(bytes,uint256)")[:10] + (word(64) + word(END_TIME + 3600) + bytes_value(unlock)).hex()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--state", type=Path, required=True)
    parser.add_argument("--custody", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    reader = Reader(args.output)
    census = {r["pool"]: r for r in json.loads((args.input / "census.json").read_text())["pools"]}
    snapshots = {r["pool"]: r for r in json.loads((args.state / "snapshot.json").read_text())["pools"]}
    custody = json.loads((args.custody / "custody.json").read_text())
    if header(reader, END_BLOCK, refresh=True)["hash"] != END_HASH:
        raise ValueError("initial_hash_mismatch")
    rows, proven = [], set()
    for pos in custody["positions"]:
        code = custody["owner_codes"].get(pos.get("owner"))
        if pos["pool"] in proven or code is None or pos.get("owner") == "0x000000000000000000000000000000000000dead":
            continue
        if code != "0x" and not (code.startswith("0xef0100") and len(code) == 48):
            continue
        liquidity = pos.get("current_liquidity") if pos["venue"] == "uniswap-v4" else pos["position"]["liquidity"]
        if not liquidity or int(liquidity) <= 0:
            continue
        amounts = principal([dict(pos["position"], liquidity=liquidity)], int(snapshots[pos["pool"]]["sqrt_price_x96"]))
        if not any(amounts):
            continue
        if pos["venue"] == "uniswap-v4":
            data = v4_withdrawal(pos, census[pos["pool"]])
        else:
            data = call_data("decreaseLiquidity((uint256,uint128,uint256,uint256,uint256))", pos["token_id"], int(liquidity), 0, 0, END_TIME + 3600)
        result = {"pool": pos["pool"], "venue": pos["venue"], "token_id": pos["token_id"], "owner": pos["owner"], "manager": pos["manager"],
            "positive_principal": [str(v) for v in amounts], "withdrawal": "unresolved"}
        try:
            returned = reader.rpc("eth_call", [{"to": pos["manager"], "from": pos["owner"], "data": data}, hex(END_BLOCK)])
            if pos["venue"] == "uniswap-v4" and returned == "0x" or pos["venue"] != "uniswap-v4" and len(returned) == 130 and int(returned[2:], 16) > 0:
                result["withdrawal"] = "positive_share_withdrawal_succeeded"
                proven.add(pos["pool"])
            result["returned"] = returned
        except RPCFailure as exc:
            result["reason"] = str(exc)
        rows.append(result)
        write(args.output / "withdrawals.json", {"block": END_BLOCK, "block_hash": END_HASH, "probes": rows, "complete": False})
    if header(reader, END_BLOCK, refresh=True)["hash"] != END_HASH:
        raise ValueError("final_hash_mismatch")
    write(args.output / "withdrawals.json", {"block": END_BLOCK, "block_hash": END_HASH, "probes": rows, "complete": True})
    print(json.dumps({"successful_pools": len(proven), "probes": len(rows), "by_venue": dict(collections.Counter(r["venue"] for r in rows if r["withdrawal"] == "positive_share_withdrawal_succeeded"))}))


if __name__ == "__main__":
    main()
