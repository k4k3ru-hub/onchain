"""Inspect code discovered from frozen Pool samples; no locker registry.

ERC-7702 and ERC-1167 patterns are protocol standards. Source names and fields
are observations only and never authorize a positive locked percentage.
"""
import argparse
import json
from pathlib import Path
import sys
import time


def code_structure(code):
    raw = bytes.fromhex(code.removeprefix("0x"))
    if not raw:
        return {"kind": "empty"}
    if len(raw) == 23 and raw[:3] == bytes.fromhex("ef0100"):
        return {"kind": "eip7702", "target": "0x" + raw[3:].hex()}
    if len(raw) >= 45 and raw[:10] == bytes.fromhex("363d3d373d3d3d363d73") and raw[30:45] == bytes.fromhex("5af43d82803e903d91602b57fd5bf3"):
        return {"kind": "eip1167", "target": "0x" + raw[10:30].hex(), "extra_data_bytes": len(raw) - 45}
    return {"kind": "unrecognized_runtime", "bytes": len(raw)}


def can_report_unlocked(row, positions):
    """Require complete, nonempty principal and every withdrawal route verified."""
    return bool(row.get("complete_position_coverage") and positions
                and (int(row.get("principal_token0", "0")) > 0 or int(row.get("principal_token1", "0")) > 0)
                and all(p["withdrawal_assessment"] in ["owner_full_decrease_succeeded", "eip7702_owner_full_decrease_succeeded"] for p in positions))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--legacy-dir", type=Path, required=True)
    parser.add_argument("--samples", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    sys.path.insert(0, str(args.legacy_dir.resolve()))
    from probe import Reader, keccak
    reader = Reader(args.output)
    original = json.loads(args.samples.read_text())
    result = {"venue": original["venue"], "block_number": original["block_number"], "samples": [], "custodians": []}
    block = hex(original["block_number"])
    endpoint = "https://mainnet.base.org"

    def rpc(method, params):
        for attempt in range(4):
            time.sleep(1.5)
            try:
                return reader.rpc(endpoint, method, params)
            except RuntimeError:
                if reader.requests[-1].get("http_status") != 429 or attempt == 3:
                    raise
                time.sleep(3)

    header = rpc("eth_getBlockByNumber", [block, False])
    if header["hash"] != original["block_hash"]:
        raise RuntimeError("sample_block_hash_mismatch")
    custodians = {}
    sources = {}
    for row in original["samples"]:
        evaluated = {"pool": row["pool"], "locked_liquidity_percentage": row["locked_liquidity_percentage"], "positions": [], "reasons": row["reasons"]}
        for position in row["positions"]:
            if position["owner_code_bytes"] == 0:
                evaluated["positions"].append(position)
                continue
            owner = position["owner"]
            if owner not in custodians:
                code = rpc("eth_getCode", [owner, block])
                if "0x" + keccak(bytes.fromhex(code[2:])) != position["owner_code_hash"]:
                    raise RuntimeError("owner_runtime_hash_mismatch")
                custodian = {"address": owner, "code": code, "structure": code_structure(code)}
                custodians[owner] = custodian
                result["custodians"].append(custodian)
                address = custodian["structure"].get("target", owner)
                # EIP-7702 permits owner-originated outgoing transactions directly;
                # delegate source is not needed for this route.
                if custodian["structure"]["kind"] != "eip7702":
                    if address not in sources:
                        try:
                            source = reader.request("https://sourcify.dev/server/v2/contract/8453/" + address + "?fields=all")
                            reader.save(original["venue"] + "-source-" + address, source)
                            source_runtime = rpc("eth_getCode", [address, block]) if address != owner else code
                            sources[address] = {"address": address, "runtime_matches_rpc": source["runtimeBytecode"]["onchainBytecode"].lower() == source_runtime.lower(), "compilation": source.get("compilation"), "abi_functions": [{"name": a["name"], "inputs": a.get("inputs"), "stateMutability": a.get("stateMutability")} for a in source.get("abi", []) if a["type"] == "function"]}
                        except RuntimeError as exc:
                            sources[address] = {"address": address, "reason": str(exc), "runtime_matches_rpc": False}
                    custodian["source"] = sources[address]
            observation = dict(position)
            observation["structure"] = custodians[owner]["structure"]
            if observation["structure"]["kind"] == "eip7702":
                data = "0x" + keccak(b"decreaseLiquidity((uint256,uint128,uint256,uint256,uint256))")[:8]
                for value in [int(position["token_id"]), int(position["liquidity"]), 0, 0, int(header["timestamp"], 16) + 3600]:
                    data += value.to_bytes(32, "big").hex()
                try:
                    returned = rpc("eth_call", [{"from": owner, "to": position["manager"], "data": data}, block])
                    if len(bytes.fromhex(returned[2:])) != 64:
                        raise RuntimeError("unexpected_decrease_result")
                    observation["withdrawal_assessment"] = "eip7702_owner_full_decrease_succeeded"
                    observation["probe_result"] = returned
                except RuntimeError as exc:
                    observation["probe_error"] = str(exc)
            evaluated["positions"].append(observation)
        if can_report_unlocked(row, evaluated["positions"]):
            evaluated["locked_liquidity_percentage"] = "0"
            evaluated["reasons"] = []
        result["samples"].append(evaluated)
    after = rpc("eth_getBlockByNumber", [block, False])
    if after["hash"] != original["block_hash"]:
        raise RuntimeError("sample_block_hash_changed")
    result["requests"] = reader.requests
    reader.save(original["venue"] + "-custody", result)
    print(json.dumps({"venue": original["venue"], "custodians": len(custodians), "calculated": sum(r["locked_liquidity_percentage"] is not None for r in result["samples"]), "http_attempts": len(reader.requests)}))


if __name__ == "__main__":
    main()
