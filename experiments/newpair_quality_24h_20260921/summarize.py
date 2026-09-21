"""Reproduce the fixed 24-hour classification from retained evidence, offline."""
import argparse
import collections
import csv
import gzip
import json
from pathlib import Path


def read(root, name):
    path = root / name
    if path.exists():
        return json.loads(path.read_text())
    with gzip.open(str(path) + ".gz", "rt") as file:
        return json.load(file)


def summarize(root):
    census = read(root, "census.json")
    activity = {r["pool"]: r for r in read(root, "activity.json")["pools"]}
    quality = read(root, "quality.json")
    detailed = {r["pool"]: r for r in quality["pools"]}
    probes = read(root, "withdrawals.json")
    assert probes["complete"]
    withdrawable = {r["pool"] for r in probes["probes"] if r["withdrawal"] == "positive_share_withdrawal_succeeded"}
    prior = {(r["venue"], r["pool"].lower()): r for r in read(root, "previous-summary.json")["pools"]}
    rows = []
    for p in census["pools"]:
        a = activity.get(p["pool"])
        old = prior.get((p["venue"], p["pool"].lower()))
        if old and p["chain"] == "base":
            assert old["observed_block"] == census["base_observed_block"]
        q = detailed.get(p["pool"])
        row = {"venue": p["venue"], "pool": p["pool"], "created_at": p["created_at"], "result": "unresolved", "reason": "token_lp_or_usd_conditions_unverified"}
        if a:
            row.update(observed_swap_count=a["swap_count"], activity_replay_complete=a["replay_complete"])
        if a and a["replay_complete"] and not a["swap_count"]:
            row.update(result="excluded", reason="no_swap_in_complete_creation_to_snapshot_history")
        elif a and a["replay_complete"] and not a["positions"]:
            row.update(result="excluded", reason="no_remaining_lp_principal")
        elif old and old.get("locked_liquidity_percentage") == "0" and p["chain"] == "base":
            row.update(result="excluded", reason="previous_same_block_liquidity_unlocked")
        elif old and old.get("admin_change_risk") is True and p["chain"] == "base":
            row.update(result="excluded", reason="previous_same_block_mutable_lock_migration_authority")
        elif p["pool"] in withdrawable:
            row.update(result="excluded", reason="positive_lp_share_withdrawal_succeeded")
        elif q and q["strict_result"] == "excluded":
            row.update(result="excluded", reason=";".join(q["reasons"]))
        elif q and q["strict_result"] == "pass":
            assert a["replay_complete"] and a["swap_count"] > 0 and q["lp_protection"] == "100_percent_permanent_source_reviewed"
            row.update(result="pass", reason="reviewed_token_and_permanent_lp_conditions", lp_protection=q["lp_protection"],
                       residual_1h=q["residual_1h"], residual_24h=q["residual_24h"], residual_7d=q["residual_7d"],
                       liquidity_usd=q["liquidity_usd_display"], quote_principal_usd=q["quote_principal_usd_display"],
                       dex_swap_fee_percentage=str(p["fee"] / 10000),
                       symbol=quality["metadata"][p["token1"]]["symbol"])
        elif q:
            row["reason"] = ";".join(q["reasons"])
        elif a and not a["replay_complete"]:
            row["reason"] = "activity_gap_and_other_conditions_unverified"
        rows.append(row)
    assert len(rows) == sum(census["counts"].values())
    assert len(rows) == len({(r["venue"], r["pool"]) for r in rows})
    counts = {v: dict(collections.Counter(r["result"] for r in rows if r["venue"] == v)) for v in census["counts"]}
    ledger = []
    for stage in ["census-activity", "state", "custody", "sources", "quality", "withdrawals"]:
        attempts = read(root, "ledgers/" + stage + ".json")
        ledger.append({"stage": stage, "chain_http_attempts": len(attempts), "methods": dict(collections.Counter(r["method"] for r in attempts)),
            "http_429": sum(r.get("http_status") == 429 for r in attempts), "http_500": sum(r.get("http_status") == 500 for r in attempts),
            "connection_failed": sum(r.get("failure") == "connection_failed" for r in attempts)})
    confirmed = [r for r in rows if r["result"] == "pass"]
    return {"window_start_inclusive": census["window_start_inclusive"], "window_end_exclusive": census["window_end_exclusive"],
        "creation_counts": census["counts"], "counts": counts, "pools": rows, "confirmed": confirmed,
        "lp_residual_comparison": {label: sum(bool(r.get(key)) for r in confirmed) for label, key in [("1h", "residual_1h"), ("24h", "residual_24h"), ("7d", "residual_7d")]},
        "quote_principal_comparison_within_confirmed_only": {str(n): sum(float(r["quote_principal_usd"]) >= n for r in confirmed) for n in [1, 10, 100, 1000]},
        "ledger": ledger, "chain_http_attempts": sum(r["chain_http_attempts"] for r in ledger),
        "public_source_get_attempts": len(read(root, "ledgers/source-get.json")),
        "gateway_requests": read(root, "agent-list.json")["gateway_requests"]}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--data", type=Path, default=Path(__file__).parent / "evidence")
    args = parser.parse_args()
    result = summarize(args.data)
    (args.data / "summary.json").write_text(json.dumps(result, indent=2) + "\n")
    with (args.data / "pools.csv").open("w", newline="") as file:
        writer = csv.DictWriter(file, fieldnames=["venue", "pool", "created_at", "result", "reason", "observed_swap_count", "activity_replay_complete", "symbol", "lp_protection", "liquidity_usd", "quote_principal_usd", "dex_swap_fee_percentage"], extrasaction="ignore")
        writer.writeheader()
        writer.writerows(result["pools"])
    print(json.dumps({k: v for k, v in result.items() if k not in ["pools", "confirmed", "ledger"]}, indent=2))


if __name__ == "__main__":
    main()
