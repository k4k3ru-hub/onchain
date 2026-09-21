"""Inspect frozen Meteora candidates whose creation history exceeded the budget.

This supplements account observations without replacing the cohort or claiming
that the creation transaction or the whole-pool lock ratio was verified.
"""
import argparse
import base64
import json
from pathlib import Path
import sys
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--legacy-dir", type=Path, required=True)
    parser.add_argument("--samples", type=Path, required=True)
    parser.add_argument("--idl", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    sys.path.insert(0, str(args.legacy_dir.resolve()))
    from probe import Reader, b58, idl_fields
    from pool_first_non_evm import DLMM, SOLANA, POOL_TAG, POSITION_TAG
    reader = Reader(args.output)
    original = json.loads(args.samples.read_text())
    idl = json.loads(args.idl.read_text())
    result = {"samples": [], "creation_verification": "unchanged", "read_only": True}

    def rpc(method, params):
        for attempt in range(4):
            time.sleep(1.5 if attempt == 0 else 2 ** attempt)
            try:
                value = reader.rpc(SOLANA, method, params)
                reader.save("observation-" + str(len(reader.requests)), {"method": method, "params": params, "result": value})
                return value
            except RuntimeError:
                if attempt == 3:
                    raise

    for previous in original["samples"]:
        if previous.get("evidence"):
            continue
        pool = previous["pool"]
        row = {"pool": pool, "locked_liquidity_percentage": None, "positions": []}
        try:
            enumeration = rpc("getProgramAccounts", [DLMM, {"encoding": "base64", "commitment": "finalized", "withContext": True, "dataSlice": {"offset": 0, "length": 72}, "filters": [{"memcmp": {"offset": 0, "bytes": b58(POSITION_TAG)}}, {"memcmp": {"offset": 8, "bytes": pool}}]}])
            row["enumeration_slot"] = enumeration["context"]["slot"]
            if len(enumeration["value"]) > 98:
                raise RuntimeError("account_read_budget_exceeded")
            keys = [pool] + [a["pubkey"] for a in enumeration["value"]]
            snapshot = rpc("getMultipleAccounts", [keys, {"encoding": "base64", "commitment": "finalized", "minContextSlot": row["enumeration_slot"]}])
            row["snapshot_slot"] = snapshot["context"]["slot"]
            account = snapshot["value"][0]
            raw = base64.b64decode(account["data"][0])
            if account["owner"] != DLMM or raw[:8] != POOL_TAG:
                raise RuntimeError("unexpected_pool_account")
            fields = idl_fields(idl, "LbPair", raw)
            row["pool_keys"] = {key: fields[key] for key in ["token_x_mint", "token_y_mint", "bin_step", "activation_type"]}
            for key, account in zip(keys[1:], snapshot["value"][1:]):
                if account is None:
                    row["positions"].append({"position": key, "reason": "account_disappeared"})
                    continue
                raw = base64.b64decode(account["data"][0])
                if account["owner"] != DLMM or raw[:8] != POSITION_TAG:
                    raise RuntimeError("unexpected_position_account")
                position = idl_fields(idl, "PositionV2", raw)
                if position["lb_pair"] != pool:
                    raise RuntimeError("position_pool_mismatch")
                row["positions"].append({"position": key, **{name: str(position[name]) for name in ["owner", "operator", "lock_release_point", "version", "lower_bin_id", "upper_bin_id"]}, "nonzero_shares_in_legacy_70_bins": sum(v > 0 for v in position["liquidity_shares"]), "raw_data_bytes": len(raw)})
            row["reason"] = "creation_history_and_withdrawal_semantics_and_all_bin_principal_not_verified"
        except RuntimeError as exc:
            row["reason"] = str(exc)
        result["samples"].append(row)
        result["requests"] = reader.requests
        reader.save("accounts-supplement", result)
        print(json.dumps({"pool": pool, "positions": len(row["positions"]), "reason": row["reason"]}), flush=True)


if __name__ == "__main__":
    main()
