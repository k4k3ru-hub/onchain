"""Reproduce the 2026-09-21 frozen 4 x 30 sample summary without network calls."""
import argparse
import collections
import csv
import datetime as dt
import json
from pathlib import Path


def utc(timestamp):
    return dt.datetime.fromtimestamp(timestamp, dt.timezone.utc).isoformat()


def summarize(root):
    def read(path):
        return json.loads((root / path).read_text())

    def verified_probes(path):
        evidence = read(path)
        assert evidence["probes"][0]["name"] == "initial_block_hash"
        assert evidence["probes"][-1]["name"] == "final_block_hash"
        assert all(p["matched"] for p in evidence["probes"])
        return evidence

    uni_review = read("uniswap-review/runtime-review.json")
    aero_review = read("aerodrome-review/runtime-review.json")
    expired = verified_probes("uniswap-review/expired-release-probes.json")
    controls = verified_probes("aerodrome-review/controls-probes.json")
    verified_probes("aerodrome-review/gauge-probes.json")
    verified_probes("aerodrome-review/gauge-bindings-probes.json")
    assert uni_review["final_block_hash_verified"] and aero_review["final_block_hash_verified"]
    uni_custody = {r["address"]: r for r in uni_review["custodians"]}
    aero_custody = {r["address"]: r for r in aero_review["custodians"] if r["compiled_runtime_matches"]}
    release_pools = {p["name"].split(":")[0].lower() for p in expired["probes"] if p["name"].endswith(":expired_vault_nft_release")}
    control_names = {p["name"] for p in controls["probes"]}
    rows, venues, ledger = [], {}, []

    def add_methods(label, methods):
        ledger.append({"stage": label, "chain_http_attempts": sum(methods.values()), "public_get_attempts": 0, "methods": methods})

    def add_requests(label, requests):
        methods = collections.Counter(r["method"] for r in requests)
        public = methods.pop("GET", 0)
        ledger.append({"stage": label, "chain_http_attempts": sum(methods.values()), "public_get_attempts": public, "methods": dict(methods), "http_429_responses": sum(r.get("http_status") == 429 for r in requests)})

    for venue, directory in [("uniswap-v3", "uniswap"), ("aerodrome", "aerodrome")]:
        data = read(directory + "/" + venue + "-samples.json")
        manifest = read(venue + "-manifest.json")
        assert data["final_block_hash_verified"] and len(data["samples"]) == 30
        assert len({r["pool"].lower() for r in data["samples"]}) == 30
        assert {r["pool"].lower() for r in data["samples"]} == {"0x" + e["data"][-40:].lower() for e in manifest["events"]}
        for review in [uni_review, aero_review, expired, controls]:
            assert review["block_hash"] == data["block_hash"]
        for sample in data["samples"]:
            row = {"venue": venue, "pool": sample["pool"], "created_at": utc(int(sample["creation_event"]["blockTimestamp"], 16)), "observed_block": data["block_number"], "locked_liquidity_percentage": sample["locked_liquidity_percentage"], "basis": "core_rpc_inspection", "reasons": sample["reasons"], "admin_change_risk": None}
            positions = sample["positions"]
            if venue == "uniswap-v3" and row["pool"].lower() in release_pools:
                assert sample["complete_position_coverage"] and positions
                assert int(sample["principal_token0"]) or int(sample["principal_token1"])
                durations = set()
                for pos in positions:
                    observed = uni_custody[pos["owner"]]
                    assert observed["compiled_runtime_matches"]
                    assert int(observed["getters"]["unlockTimestamp"], 16) < manifest["block_timestamp"]
                    # AST id 2310 is lockTimestamp in the pinned, independently compiled source.
                    durations.add(int(observed["getters"]["unlockTimestamp"], 16) - int(observed["immutable_values_by_ast_id"]["2310"], 16))
                assert len(durations) == 1
                row.update(locked_liquidity_percentage="0", basis="compiled_vault_expired_and_nft_release_succeeded", reasons=[], lock_duration_seconds=durations.pop())
            elif venue == "aerodrome" and positions and all(p["owner"] in aero_custody for p in positions):
                assert sample["complete_position_coverage"]
                assert int(sample["principal_token0"]) or int(sample["principal_token1"])
                deadlines = []
                for pos in positions:
                    getter = aero_custody[pos["owner"]]["getters"]
                    assert "0x" + getter["pool"][-40:] == row["pool"].lower()
                    assert "0x" + getter["nfpManager"][-40:] == pos["manager"]
                    assert int(getter["lp"], 16) == int(pos["token_id"])
                    assert int(getter["staked"], 16) == int(getter["gauge"], 16) == 0
                    assert int(pos["approved"], 16) == 0
                    deadline = int(getter["lockedUntil"], 16)
                    assert deadline > manifest["block_timestamp"]
                    deadlines.append(deadline)
                    for suffix in ["factory_registered", "gauge_unavailable", "unlock", "migrate", "stake", "direct_unlock"]:
                        assert pos["owner"] + ":" + suffix in control_names
                row.update(locked_liquidity_percentage="100", basis="manual_review_current_configuration", reasons=[], admin_change_risk=True, earliest_unlock_at=utc(min(deadlines)), protection_immutable=False)
            elif venue == "aerodrome" and row["pool"].lower() == "0xf9bed06362613a556391de629dc6a57d7e31a776":
                assert sample["complete_position_coverage"]
                assert len(positions) == 2
                assert any(p["withdrawal_assessment"] == "owner_full_decrease_succeeded" for p in positions)
                row.update(locked_liquidity_percentage="0", basis="gauge_nft_withdrawal_and_other_position_withdrawal_succeeded", reasons=[])
            rows.append(row)
        timestamps = [int(e["blockTimestamp"], 16) for e in manifest["events"]]
        venues[venue] = {"created_from": utc(min(timestamps)), "created_to": utc(max(timestamps)), "creation_span_seconds": max(timestamps) - min(timestamps), "observed_at": utc(manifest["block_timestamp"]), "block_number": data["block_number"], "core_wall_seconds": data["wall_seconds"], "core_429_retries": data["rate_limit_retries"]}
        add_methods(venue + ":discovery", read(venue + "-discovery-metrics.json")["rpc_http_attempts"])
        add_methods(venue + ":core", data["rpc_http_attempts"])

    for venue, directory in [("meteora-dlmm", "meteora"), ("cetus-clmm", "cetus")]:
        data = read(directory + "/" + venue + "-samples.json")
        assert data["sampling_complete"] and len(data["samples"]) == 30
        assert len({r["pool"] for r in data["samples"]}) == 30
        if venue == "meteora-dlmm":
            candidates = read("meteora/meteora-dlmm-candidate-manifest.json")["candidates"]
            assert {r["pool"] for r in data["samples"]} == {c["address"] for c in candidates}
            timestamps = [c["created_at"] // 1000 for c in candidates]
            assert timestamps == sorted(timestamps, reverse=True)
            supplement = read("meteora-supplement/accounts-supplement.json")
            observed = {r["pool"]: r for r in supplement["samples"]}
            observed.update({r["pool"]: r["evidence"] for r in data["samples"] if r.get("evidence")})
            assert len(observed) == 30
            venues[venue] = {"created_from": utc(min(timestamps)), "created_to": utc(max(timestamps)), "creation_span_seconds": max(timestamps) - min(timestamps), "creation_events_rpc_verified": sum(not r["creation_event"].get("creation_unverified") for r in data["samples"]), "observed_positions": sum(len(r["positions"]) for r in observed.values()), "empty_position_enumerations": sum(not r["positions"] for r in observed.values()), "native_future_lock_observations": sum(int(p.get("lock_release_point", "0")) > 0 for r in observed.values() for p in r["positions"]), "snapshot_slot_from": min(r["snapshot_slot"] for r in observed.values()), "snapshot_slot_to": max(r["snapshot_slot"] for r in observed.values())}
            created = {c["address"]: utc(c["created_at"] // 1000) for c in candidates}
            add_requests(venue + ":account_supplement", supplement["requests"])
        else:
            timestamps = [r["creation_event"]["timestamp"] for r in data["samples"]]
            checkpoint = data["checkpoint"]
            assert all(r["creation_event"]["transaction"]["effects"]["checkpoint"]["sequenceNumber"] <= checkpoint["sequenceNumber"] for r in data["samples"])
            venues[venue] = {"created_from": min(timestamps), "created_to": max(timestamps), "checkpoint": checkpoint, "observed_position_owners": dict(collections.Counter(p.get("owner", {}).get("__typename", "object_not_returned") for r in data["samples"] for p in r["positions"]))}
            created = {r["pool"]: r["creation_event"]["timestamp"] for r in data["samples"]}
        for row in data["samples"]:
            assert row["locked_liquidity_percentage"] is None
            rows.append({"venue": venue, "pool": row["pool"], "created_at": created[row["pool"]], "locked_liquidity_percentage": None, "basis": "whole_pool_principal_or_withdrawal_semantics_unverified", "reasons": row.get("reasons", [row.get("reason")]), "admin_change_risk": None})
        add_requests(venue + ":core", data["requests"])

    for directory in ["uniswap-review", "aerodrome-review"]:
        add_requests(directory, read(directory + "/runtime-review.json")["requests"])
        for path in sorted((root / directory).glob("*-probes.json")):
            evidence = read(str(path.relative_to(root)))
            ledger.append({"stage": str(path.relative_to(root)), "chain_http_attempts": evidence["http_attempts"], "public_get_attempts": 0})
    for path in sorted((root / "source-review").glob("dependency-0x*.json")):
        add_requests(path.name, read(str(path.relative_to(root)))["requests"])
    add_requests("source_fallback", read("source-review/fallback-source-probe.json")["requests"])
    for venue, stats in venues.items():
        cohort = [r for r in rows if r["venue"] == venue]
        stats["sample_count"] = len(cohort)
        stats["current_locked_100_count"] = sum(r["locked_liquidity_percentage"] == "100" for r in cohort)
        stats["unlocked_0_count"] = sum(r["locked_liquidity_percentage"] == "0" for r in cohort)
        stats["unresolved_count"] = sum(r["locked_liquidity_percentage"] is None for r in cohort)
    assert len(rows) == 120
    return {"sample_count": 120, "cohort_kind": "recent_created_pools_not_markethub_listings", "venues": venues, "pools": rows, "rpc_ledger": ledger, "chain_http_attempts": sum(r["chain_http_attempts"] for r in ledger), "public_get_attempts": sum(r["public_get_attempts"] for r in ledger)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--data", type=Path, default=Path(__file__).parent / "testdata/venue_samples_20260921_30")
    args = parser.parse_args()
    result = summarize(args.data)
    (args.data / "summary.json").write_text(json.dumps(result, indent=2) + "\n")
    with (args.data / "pools.csv").open("w", newline="") as output:
        columns = ["venue", "pool", "created_at", "locked_liquidity_percentage", "basis", "admin_change_risk", "earliest_unlock_at", "reasons"]
        writer = csv.DictWriter(output, fieldnames=columns, extrasaction="ignore")
        writer.writeheader()
        writer.writerows(result["pools"])
    print(json.dumps({k: v for k, v in result.items() if k not in ["pools", "rpc_ledger"]}, indent=2))


if __name__ == "__main__":
    main()
