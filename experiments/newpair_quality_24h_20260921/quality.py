"""Bounded source-reviewed qualification. Unknown families never pass by default."""
import argparse
import collections
from fractions import Fraction
import hashlib
import json
from pathlib import Path

from activity import words, signed
from batch import batch, call_data
from census import END_BLOCK, END_HASH, END_TIME, Reader, RPCFailure, header, write
from custody import MANAGERS

LAUNCH_LOCKER = "0xcd1680d26922fcd9cabfbb8a56ba40c333fd842a"
LAUNCH_TOKEN = "0x6eede15fb4c8001fdadad730c5d97b47f3185da6"
TRUSTED = {"0x0000000000000000000000000000000000000000": (18, "ETH"),
    "0x4200000000000000000000000000000000000006": (18, "ETH"),
    "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913": (6, "USDC")}


def sqrt_at_tick(tick):
    # Uniswap TickMath Q128 constants; round up when converting to Q96.
    constants = [int(s, 16) for s in [
        "fffcb933bd6fad37aa2d162d1a594001", "fff97272373d413259a46990580e213a", "fff2e50f5f656932ef12357cf3c7fdcc", "ffe5caca7e10e4e61c3624eaa0941cd0",
        "ffcb9843d60f6159c9db58835c926644", "ff973b41fa98c081472e6896dfb254c0", "ff2ea16466c96a3843ec78b326b52861", "fe5dee046a99a2a811c461f1969c3053",
        "fcbe86c7900a88aedcffc83b479aa3a4", "f987a7253ac413176f2b074cf7815e54", "f3392b0822b70005940c7a398e4b70f3", "e7159475a2c29b7443b29c7fa6e889d9",
        "d097f3bdfd2022b8845ad8f792aa5825", "a9f746462d870fdf8a65dc1f90e061e5", "70d869a156d2a1b890bb3df62baf32f7", "31be135f97d08fd981231505542fcfa6",
        "9aa508b5b7a84e1c677de54f3e99bc9", "5d6af8dedb81196699c329225ee604", "2216e584f5fa1ea926041bedfe98", "48a170391f7dc42444e8fa2"]]
    if abs(tick) > 887272:
        raise ValueError("tick_out_of_range")
    price = 1 << 128
    for bit, factor in enumerate(constants):
        if abs(tick) & (1 << bit):
            price = price * factor >> 128
    if tick > 0:
        price = ((1 << 256) - 1) // price
    return (price + (1 << 32) - 1) >> 32


def principal(positions, sqrt):
    amount0, amount1 = 0, 0
    for p in positions:
        lower, upper, liquidity = sqrt_at_tick(p["lower"]), sqrt_at_tick(p["upper"]), int(p["liquidity"])
        current = min(max(sqrt, lower), upper)
        amount0 += liquidity * (upper - current) * (1 << 96) // (upper * current)
        amount1 += liquidity * (current - lower) // (1 << 96)
    return amount0, amount1


def matches_reviewed_runtime(source_record, code):
    source = source_record["source"]
    if not source_record["published_runtime_matches"] or source_record["provider"] != "sourcify":
        return False
    contract = source["compilation"]["fullyQualifiedName"]
    file, name = contract.rsplit(":", 1)
    deployed = source["stdJsonOutput"]["contracts"][file][name]["evm"]["deployedBytecode"]
    reference, candidate = bytes.fromhex(source_record["code"][2:]), bytes.fromhex(code[2:])
    if len(reference) != len(candidate) or deployed.get("linkReferences"):
        return False
    masked = set()
    for sites in deployed.get("immutableReferences", {}).values():
        values = set()
        for site in sites:
            start, size = site["start"], site["length"]
            masked.update(range(start, start + size))
            values.add(candidate[start:start + size])
        if len(values) != 1:
            return False
    return all(a == b or i in masked for i, (a, b) in enumerate(zip(reference, candidate)))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--state", type=Path, required=True)
    parser.add_argument("--custody", type=Path, required=True)
    parser.add_argument("--sources", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    reader = Reader(args.output)
    census = {r["pool"]: r for r in json.loads((args.input / "census.json").read_text())["pools"]}
    activity = {r["pool"]: r for r in json.loads((args.input / "activity.json").read_text())["pools"]}
    snapshots = {r["pool"]: r for r in json.loads((args.state / "snapshot.json").read_text())["pools"]}
    custody = json.loads((args.custody / "custody.json").read_text())
    positions = collections.defaultdict(list)
    for p in custody["positions"]:
        positions[p["pool"]].append(p)
    source = json.loads((args.sources / (LAUNCH_TOKEN + ".json")).read_text())
    locker = json.loads((args.sources / (LAUNCH_LOCKER + ".json")).read_text())
    if not locker["published_runtime_matches"] or custody["owner_codes"][LAUNCH_LOCKER] != locker["code"]:
        raise ValueError("reviewed_locker_runtime_mismatch")
    if header(reader, END_BLOCK, refresh=True)["hash"] != END_HASH:
        raise ValueError("initial_hash_mismatch")
    binding = batch(reader, [(LAUNCH_LOCKER, call_data("positionManager()"))])[0]
    if not binding["success"] or "0x" + binding["data"][-40:] != MANAGERS["uniswap-v4"]:
        raise ValueError("locker_manager_mismatch")
    feed = "0x71041dddad3595f9ced3dccfbe3d1f4b0a16bb70"
    price_data = batch(reader, [(feed, call_data("decimals()")), (feed, call_data("latestRoundData()"))])
    eth_price = None
    if all(r["success"] for r in price_data) and len(price_data[0]["data"]) == 66:
        decimals = int(price_data[0]["data"], 16)
        values = words(price_data[1]["data"])
        if len(values) == 5 and 0 < signed(values[1]) and int(values[4], 16) >= int(values[0], 16) and 0 <= END_TIME - int(values[3], 16) <= 3600:
            eth_price = Fraction(int(values[1], 16), 10 ** decimals)
    rows = []
    for pool, pos in positions.items():
        if not any(p.get("owner") == LAUNCH_LOCKER for p in pos):
            continue
        a, c, snapshot = activity[pool], census[pool], snapshots[pool]
        row = {"pool": pool, "venue": c["venue"], "strict_result": "unresolved", "reasons": [],
            "lp_protection": "unresolved", "token_assessments": []}
        rows.append(row)
        if not a["replay_complete"]:
            row["reasons"].append("liquidity_history_gap")
            continue
        actual = {(p["manager"], p["position"]["lower"], p["position"]["upper"], hex(p["token_id"])): p for p in pos}
        complete = len(actual) == len(a["positions"])
        for p in a["positions"]:
            nft = actual.get((p["owner"], p["lower"], p["upper"], hex(int(p["salt"], 16))))
            complete = complete and nft is not None and nft.get("owner") == LAUNCH_LOCKER and nft.get("current_liquidity") == p["liquidity"] and nft["approved_result"]["success"] and int(nft["approved_result"]["data"], 16) == 0
        if not complete or c["hooks"] != "0x" + "0" * 40:
            row["reasons"].append("all_shares_not_verified_in_reviewed_locker")
            continue
        tick = signed(words(snapshot["slot_result"]["data"])[1])
        active = sum(int(p["liquidity"]) for p in a["positions"] if p["lower"] <= tick < p["upper"])
        if active != int(snapshot["active_liquidity"]):
            raise ValueError("active_liquidity_reconciliation_mismatch")
        amounts = principal(a["positions"], int(snapshot["sqrt_price_x96"]))
        if not any(amounts):
            row["reasons"].append("principal_zero")
            continue
        row.update(lp_protection="100_percent_permanent_source_reviewed", principal_token0=str(amounts[0]), principal_token1=str(amounts[1]))
        token_results = []
        for token in [c["token0"], c["token1"]]:
            if token in TRUSTED:
                token_results.append({"token": token, "result": "trusted_chain_definition"})
                continue
            try:
                code = reader.rpc("eth_getCode", [token, hex(END_BLOCK)])
                matched = matches_reviewed_runtime(source, code)
            except RPCFailure:
                matched = False
            token_results.append({"token": token, "result": "reviewed_launch_token" if matched else "unresolved"})
        row["token_assessments"] = token_results
        q = next((i for i, token in enumerate([c["token0"], c["token1"]]) if token in TRUSTED and TRUSTED[token][1] == "ETH"), None)
        if q is not None and eth_price is not None:
            ratio = Fraction(int(snapshot["sqrt_price_x96"]) ** 2, 1 << 192)
            quote = amounts[0] + Fraction(amounts[1], ratio) if q == 0 else amounts[1] + amounts[0] * ratio
            usd = quote * eth_price / (10 ** 18)
            row["liquidity_usd_fraction"] = {"numerator": str(usd.numerator), "denominator": str(usd.denominator)}
            row["liquidity_usd_display"] = f"{float(usd):.6f}"
            row["quote_principal_usd_display"] = f"{float(Fraction(amounts[q], 10 ** 18) * eth_price):.6f}"
            if usd < 1000:
                row.update(strict_result="excluded", reasons=["liquidity_below_existing_1000_usd_threshold"])
                continue
        else:
            row["reasons"].append("usd_reference_unverified")
        if any(t["result"] == "unresolved" for t in token_results):
            row["reasons"].append("token_semantics_unverified")
        if not row["reasons"]:
            row.update(strict_result="pass", residual_1h=True, residual_24h=True, residual_7d=True)
    # The current MarketHub baseline also requires token metadata. Do not label
    # a source-matched token with an empty symbol as eligible.
    tokens = sorted({t["token"] for row in rows if row["strict_result"] == "pass" for t in row["token_assessments"] if t["token"] not in TRUSTED})
    metadata_results = batch(reader, [(token, call_data(signature)) for token in tokens for signature in ["decimals()", "symbol()"]])
    metadata = {}
    for token, decimal_result, symbol_result in zip(tokens, metadata_results[::2], metadata_results[1::2]):
        valid = decimal_result["success"] and decimal_result["data"] == "0x" + format(18, "064x") and symbol_result["success"]
        symbol = ""
        if valid:
            raw = bytes.fromhex(symbol_result["data"][2:])
            if len(raw) >= 64 and int.from_bytes(raw[:32], "big") == 32:
                size = int.from_bytes(raw[32:64], "big")
                if size <= len(raw) - 64:
                    symbol = raw[64:64 + size].decode("utf-8", errors="replace")
        metadata[token] = {"valid": bool(valid and symbol.strip()), "symbol": symbol, "decimals_result": decimal_result, "symbol_result": symbol_result}
    for row in rows:
        if row["strict_result"] == "pass" and any(t["token"] in metadata and not metadata[t["token"]]["valid"] for t in row["token_assessments"]):
            row.update(strict_result="unresolved", reasons=["token_metadata_unverified"], residual_1h=None, residual_24h=None, residual_7d=None)
    if header(reader, END_BLOCK, refresh=True)["hash"] != END_HASH:
        raise ValueError("final_hash_mismatch")
    result = {"block": END_BLOCK, "block_hash": END_HASH, "eth_usd_oracle": price_data,
        "scope": "non-proxy LaunchLocker and LaunchToken family discovered through pool positions; other source families remain unqualified",
        "pools": rows, "metadata": metadata, "counts": dict(collections.Counter(r["strict_result"] for r in rows))}
    write(args.output / "quality.json", result)
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
