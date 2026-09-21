"""Read-only, block-pinned LP protection study; no signed transactions."""

import argparse
import collections
import datetime as dt
import gzip
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import time
import urllib.error
import urllib.request


HERE = Path(__file__).resolve().parent
PRIOR = HERE.parent / "newpair_quality_24h_20260921"
sys.path.insert(0, str(PRIOR))
from batch import MULTICALL, aggregate_data, aggregate_result, call_data
from activity import signed, words
from quality import LAUNCH_LOCKER, LAUNCH_TOKEN, matches_reviewed_runtime
from snapshot import STATE_VIEW

MANAGER = "0x7c5f5a4bbd8fd63184577525326123b519429bdc"
POOL_MANAGER = "0x498581ff718922c3f8e6a244956af099b2652b2b"
ZERO = "0x" + "0" * 40


def uint(result, length=66):
    if result.get("success") and len(result["data"]) == length:
        return int(result["data"][2:66], 16)
    return None


def address(result):
    value = uint(result)
    return "0x" + format(value, "040x") if value is not None and value < 1 << 160 else None


def pool_id(fields):
    key = "".join(fields[:5])
    result = subprocess.run(["/tmp/lp-keccak"], input=key + "\n", text=True, capture_output=True, check=True)
    return "0x" + result.stdout.strip()


def bitmap_words(spacing):
    if not 1 <= spacing <= 32767:
        raise ValueError("tick_spacing_invalid")
    limit = 887272 // spacing
    return range((-limit) >> 8, (limit >> 8) + 1)


def bitmap_ticks(word, value, spacing):
    ticks = []
    for bit in range(256):
        if value >> bit & 1:
            tick = ((word << 8) + bit) * spacing
            if not -887272 <= tick <= 887272:
                raise ValueError("bitmap_tick_out_of_range")
            ticks.append(tick)
    return ticks


def reconcile(ticks, positions, active_tick, active_liquidity):
    """Prove coverage including out-of-range positions using every tick's gross."""
    expected_gross, expected_net = collections.Counter(), collections.Counter()
    seen = set()
    for p in positions:
        if p["token_id"] in seen:
            raise ValueError("duplicate_position")
        seen.add(p["token_id"])
        liquidity = int(p["liquidity"])
        if liquidity <= 0 or p["lower"] >= p["upper"]:
            raise ValueError("position_invalid")
        expected_gross[p["lower"]] += liquidity
        expected_gross[p["upper"]] += liquidity
        expected_net[p["lower"]] += liquidity
        expected_net[p["upper"]] -= liquidity
    running, active = 0, 0
    actual_gross, actual_net = {}, {}
    for t in sorted(ticks, key=lambda t: t["tick"]):
        tick, gross, net = t["tick"], int(t["gross"]), int(t["net"])
        if tick in actual_gross or gross <= 0 or abs(net) > gross:
            raise ValueError("tick_state_invalid")
        actual_gross[tick], actual_net[tick] = gross, net
        running += net
        if running < 0:
            raise ValueError("negative_tick_liquidity")
        if tick <= active_tick:
            active = running
    if running != 0 or active != active_liquidity:
        raise ValueError("tick_active_liquidity_mismatch")
    return {"all_gross_covered": bool(positions) and actual_gross == dict(expected_gross),
            "all_net_covered": bool(positions) and actual_net == dict(expected_net),
            "initialized_tick_count": len(ticks),
            "protected_position_count": len(positions),
            "pool_liquidity_gross_sum": str(sum(actual_gross.values())),
            "protected_liquidity_gross_sum": str(sum(expected_gross.values())),
            "active_liquidity_reconciled": True}


def read(path):
    path = Path(path)
    if path.suffix == ".gz":
        with gzip.open(path, "rt") as stream:
            return json.load(stream)
    return json.loads(path.read_text())


def write(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    if path.suffix == ".gz":
        with gzip.open(path, "wt") as stream:
            json.dump(value, stream)
    else:
        path.write_text(json.dumps(value, indent=2) + "\n")


class Unavailable(RuntimeError):
    pass


class Reader:
    def __init__(self, root, endpoint="https://mainnet.base.org"):
        self.root = root
        self.endpoint = endpoint
        self.ledger_path = root / "requests.json"
        self.ledger = read(self.ledger_path) if self.ledger_path.exists() else []
        self.cache_hits = 0

    def rpc(self, method, params, refresh=False):
        payload = {"jsonrpc": "2.0", "id": 1, "method": method, "params": params}
        key = hashlib.sha256(json.dumps([self.endpoint, payload], sort_keys=True).encode()).hexdigest()
        path = self.root / "rpc" / (key + ".json.gz")
        if path.exists() and not refresh:
            self.cache_hits += 1
            saved = read(path)
            if saved["payload"] != payload:
                raise ValueError("cached_payload_mismatch")
            return saved["result"]
        if not refresh and any(r["key"] == key and r["attempt"] == 4 and not r.get("success") for r in self.ledger):
            raise Unavailable("three_retries_already_exhausted")
        for attempt in range(1, 5):
            time.sleep(1.0)
            entry = {"endpoint": self.endpoint, "key": key, "method": method, "attempt": attempt,
                     "started_utc": dt.datetime.now(dt.timezone.utc).isoformat()}
            started = time.monotonic()
            try:
                request = urllib.request.Request(self.endpoint, data=json.dumps(payload).encode(), headers={
                    "Content-Type": "application/json", "User-Agent": "k4k3ru-read-only-census/1"})
                with urllib.request.urlopen(request, timeout=30) as response:
                    raw = response.read(24000001)
                if len(raw) > 24000000:
                    raise Unavailable("response_size_limit")
                result = json.loads(raw)
                if "error" in result:
                    entry["rpc_error"] = result["error"]
                    raise Unavailable("rpc_error")
                if "result" not in result:
                    raise Unavailable("missing_result")
                write(path, {"endpoint": self.endpoint, "payload": payload, "result": result["result"]})
                entry["success"] = True
                return result["result"]
            except urllib.error.HTTPError as exc:
                entry["http_status"] = exc.code
            except (urllib.error.URLError, TimeoutError):
                entry["failure"] = "connection_failed"
            except (Unavailable, ValueError) as exc:
                entry["failure"] = str(exc) if isinstance(exc, Unavailable) else "invalid_json"
            finally:
                entry["seconds"] = round(time.monotonic() - started, 3)
                self.ledger.append(entry)
                write(self.ledger_path, self.ledger)
            if attempt < 4:
                time.sleep(2 ** attempt)
        raise Unavailable("request_failed_after_three_retries")


def batch(reader, calls, block, size=100):
    results = []
    for start in range(0, len(calls), size):
        group = calls[start:start + size]
        try:
            raw = reader.rpc("eth_call", [{"to": MULTICALL, "data": aggregate_data(group)}, hex(block)])
            results.extend(aggregate_result(raw, len(group)))
        except Unavailable as exc:
            results.extend({"success": False, "data": "0x", "failure": str(exc)} for _ in group)
    return results


def inputs():
    root = PRIOR / "evidence"
    summary = read(root / "summary.json")
    census = {p["pool"]: p for p in read(root / "census.json.gz")["pools"]}
    selected = [dict(census[p["pool"]], previous_result=p["result"], previous_reason=p["reason"],
                     previous_history_complete=p.get("activity_replay_complete"))
                for p in summary["pools"] if p["venue"] == "uniswap-v4" and p["result"] in ["unresolved", "pass"]]
    return selected


def state(reader, anchor):
    pools = inputs()
    rows = []
    for start in range(0, len(pools), 50):
        group = pools[start:start + 50]
        calls = [call for p in group for call in [
            (STATE_VIEW, call_data("getSlot0(bytes32)", p["pool"])),
            (STATE_VIEW, call_data("getLiquidity(bytes32)", p["pool"]))]]
        result = batch(reader, calls, anchor["block"])
        for p, slot, liquidity in zip(group, result[::2], result[1::2]):
            row = dict(p, slot_result=slot, liquidity_result=liquidity)
            if slot["success"] and len(slot["data"]) == 258 and uint(liquidity) is not None:
                fields = words(slot["data"])
                row.update(sqrt_price_x96=str(int(fields[0], 16)), tick=signed(fields[1]),
                           active_liquidity=str(uint(liquidity)))
            rows.append(row)
        write(reader.root / "state.json.gz", {"anchor": anchor, "complete": False, "pools": rows})
        print(json.dumps({"state_read": len(rows), "total": len(pools)}), flush=True)
    write(reader.root / "state.json.gz", {"anchor": anchor, "complete": True, "pools": rows})


def discover(reader, anchor):
    pools = inputs()
    # This is a bounded re-evaluation of the custodian discovered in the prior
    # Pool-based study, not a universal locker discovery implementation.
    tokens = sorted({p[k] for p in pools for k in ["token0", "token1"] if p[k] != ZERO})
    results = batch(reader, [(LAUNCH_LOCKER, call_data("tokenIdOf(address)", token)) for token in tokens], anchor["block"])
    registrations = [{"token": token, "token_id": uint(result), "result": result} for token, result in zip(tokens, results)]
    ids = sorted({r["token_id"] for r in registrations if r["token_id"]})
    details = batch(reader, [(MANAGER, call_data("getPoolAndPositionInfo(uint256)", id_)) for id_ in ids], anchor["block"])
    pool_set = {p["pool"] for p in pools}
    found = []
    for id_, result in zip(ids, details):
        if not result["success"] or len(result["data"]) != 386:
            continue
        fields = words(result["data"])
        id_pool = pool_id(fields)
        if id_pool not in pool_set:
            continue
        packed = int(fields[5], 16)
        lower, upper = (packed >> 8) & 0xffffff, (packed >> 32) & 0xffffff
        lower = lower - (1 << 24) if lower >= 1 << 23 else lower
        upper = upper - (1 << 24) if upper >= 1 << 23 else upper
        found.append({"pool": id_pool, "token_id": id_, "lower": lower, "upper": upper, "pool_info_result": result})
    write(reader.root / "discovery.json", {"anchor": anchor, "registrations": registrations, "positions": found,
          "scope": "current tokenIdOf lookup at the previously discovered and source-reviewed LaunchLocker"})
    print(json.dumps({"tokens_checked": len(tokens), "registered_nfts": len(ids), "cohort_positions": len(found)}), flush=True)


def positions(reader, anchor):
    discovery = read(reader.root / "discovery.json")
    rows = []
    calls = [call for p in discovery["positions"] for call in [
        (MANAGER, call_data("ownerOf(uint256)", p["token_id"])),
        (MANAGER, call_data("getApproved(uint256)", p["token_id"])),
        (STATE_VIEW, call_data("getPositionInfo(bytes32,address,int24,int24,bytes32)",
                               p["pool"], MANAGER, p["lower"], p["upper"], p["token_id"]))]]
    result = batch(reader, calls, anchor["block"])
    for p, owner, approved, current in zip(discovery["positions"], result[::3], result[1::3], result[2::3]):
        row = dict(p, owner=address(owner), approved=address(approved), owner_result=owner,
                   approved_result=approved, position_result=current)
        liquidity = uint(current, 194)
        if liquidity is not None:
            row["liquidity"] = str(liquidity)
        rows.append(row)
    sources = {}
    for contract in [LAUNCH_LOCKER, MANAGER]:
        old = read(PRIOR / "evidence" / "sources" / (contract + ".json.gz"))
        current = reader.rpc("eth_getCode", [contract, hex(anchor["block"])])
        sources[contract] = {"code": current, "matches_previously_reviewed_runtime": current == old["code"],
                             "published_runtime_matches": old["published_runtime_matches"]}
    binding = batch(reader, [(LAUNCH_LOCKER, call_data("positionManager()")),
                            (MANAGER, call_data("poolManager()")),
                            (STATE_VIEW, call_data("poolManager()"))], anchor["block"])
    valid = all(r["matches_previously_reviewed_runtime"] and r["published_runtime_matches"] for r in sources.values())
    valid = valid and [address(r) for r in binding] == [MANAGER, POOL_MANAGER, POOL_MANAGER]
    for row in rows:
        row["protected"] = bool(valid and row["owner"] == LAUNCH_LOCKER and row["approved"] == ZERO
                                and int(row.get("liquidity", "0")) > 0)
    write(reader.root / "positions.json", {"anchor": anchor, "positions": rows, "sources": sources,
                                          "bindings": binding, "source_and_bindings_valid": valid})
    print(json.dumps({"positions": len(rows), "protected_positions": sum(p["protected"] for p in rows),
                      "source_and_bindings_valid": valid}), flush=True)


def ticks(reader, anchor):
    positions = read(reader.root / "positions.json")["positions"]
    pool_map = {p["pool"]: p for p in inputs()}
    candidates = sorted({p["pool"] for p in positions if p["protected"]})
    bitmap_keys = [(pool, word) for pool in candidates for word in bitmap_words(pool_map[pool]["tick_spacing"])]
    bitmap_result = batch(reader, [(STATE_VIEW, call_data("getTickBitmap(bytes32,int16)", pool, word))
                                   for pool, word in bitmap_keys], anchor["block"])
    bitmaps, tick_keys, failed = collections.defaultdict(list), [], set()
    for (pool, word), result in zip(bitmap_keys, bitmap_result):
        value = uint(result)
        bitmaps[pool].append({"word": word, "value": str(value) if value is not None else None, "result": result})
        if value is None:
            failed.add(pool)
        else:
            tick_keys.extend((pool, tick) for tick in bitmap_ticks(word, value, pool_map[pool]["tick_spacing"]))
    results = batch(reader, [(STATE_VIEW, call_data("getTickLiquidity(bytes32,int24)", pool, tick))
                             for pool, tick in tick_keys], anchor["block"])
    tick_rows = collections.defaultdict(list)
    for (pool, tick), result in zip(tick_keys, results):
        if result["success"] and len(result["data"]) == 130:
            fields = words(result["data"])
            tick_rows[pool].append({"tick": tick, "gross": str(int(fields[0], 16)),
                                    "net": str(signed(fields[1])), "result": result})
        else:
            failed.add(pool)
    state_map = {p["pool"]: p for p in read(reader.root / "state.json.gz")["pools"]}
    rows = []
    for pool in candidates:
        protected = [p for p in positions if p["pool"] == pool and p["protected"]]
        row = {"pool": pool, "bitmap_words": bitmaps[pool], "ticks": tick_rows[pool], "result": "unresolved"}
        current = state_map[pool]
        if pool not in failed and "active_liquidity" in current:
            proof = reconcile(tick_rows[pool], protected, current["tick"], int(current["active_liquidity"]))
            row["proof"] = proof
            if proof["all_gross_covered"] and proof["all_net_covered"] and pool_map[pool]["hooks"] == ZERO:
                row.update(result="100_percent_permanent_source_reviewed", locked_liquidity_percentage="100")
            else:
                row["reason"] = "remaining_positions_or_hook_semantics_unverified"
        else:
            row["reason"] = "current_tick_or_pool_state_read_failed"
        rows.append(row)
    write(reader.root / "ticks.json", {"anchor": anchor, "pools": rows,
                                       "bitmap_calls": len(bitmap_keys), "tick_calls": len(tick_keys)})
    print(json.dumps({"pools": len(rows), "results": dict(collections.Counter(r["result"] for r in rows)),
                      "bitmap_calls": len(bitmap_keys), "tick_calls": len(tick_keys)}), flush=True)


def historical_control(reader, anchor):
    """Use retained old quantities/owners; fetch only previously unread old ticks."""
    prior_root = PRIOR / "evidence"
    prior_custody = read(prior_root / "custody.json.gz")
    census = read(prior_root / "census.json.gz")
    old_anchor = {"block": census["base_observed_block"], "hash": census["base_observed_hash"],
                  "block_time_utc": census["window_end_exclusive"], "endpoint": reader.endpoint}
    control_root = reader.root / "historical_control"
    control = Reader(control_root, reader.endpoint)
    write(control_root / "anchor.json", old_anchor)
    old_locker = read(prior_root / "sources" / (LAUNCH_LOCKER + ".json.gz"))
    latest_bindings = read(reader.root / "positions.json")
    if (not old_locker["published_runtime_matches"]
            or old_locker["block"] != old_anchor["block"]
            or prior_custody["owner_codes"][LAUNCH_LOCKER] != old_locker["code"]
            or not latest_bindings["source_and_bindings_valid"]):
        raise ValueError("historical_source_or_immutable_binding_unverified")
    positions_old = []
    for row in prior_custody["positions"]:
        if row.get("owner") != LAUNCH_LOCKER:
            continue
        approved = address(row["approved_result"])
        positions_old.append({"pool": row["pool"], "token_id": row["token_id"],
                              "lower": row["position"]["lower"], "upper": row["position"]["upper"],
                              "liquidity": row.get("current_liquidity", "0"),
                              "protected": approved == ZERO and int(row.get("current_liquidity", "0")) > 0})
    write(control_root / "positions.json", {"positions": positions_old, "source": str(prior_root / "custody.json.gz")})
    states_old = []
    for row in read(prior_root / "snapshot.json.gz")["pools"]:
        if "active_liquidity" in row:
            states_old.append(dict(row, tick=signed(words(row["slot_result"]["data"])[1])))
    write(control_root / "state.json.gz", {"pools": states_old, "source": str(prior_root / "snapshot.json.gz")})
    # Source/runtime and immutable binding at the old block were verified by
    # the original study; this control only replaces its history-completeness
    # gate with independent, exhaustive tick-state coverage.
    ticks(control, old_anchor)
    end = control.rpc("eth_getBlockByNumber", [hex(old_anchor["block"]), False], refresh=True)
    if end["hash"] != old_anchor["hash"]:
        raise ValueError("historical_anchor_hash_changed")


def summarize(root):
    states = read(root / "state.json.gz")
    tick_rows = {r["pool"]: r for r in read(root / "ticks.json")["pools"]}
    rows = []
    for row in states["pools"]:
        proof = tick_rows.get(row["pool"])
        output = {k: row[k] for k in ["pool", "previous_result", "previous_reason", "previous_history_complete"]}
        output["current_pool_state_read"] = "active_liquidity" in row
        if proof and proof["result"] == "100_percent_permanent_source_reviewed":
            output.update(lp_result=proof["result"], locked_liquidity_percentage="100", proof=proof["proof"])
        else:
            output.update(lp_result="unresolved", locked_liquidity_percentage=None,
                          reason=proof.get("reason") if proof else "custodian_or_full_share_coverage_outside_completed_analysis")
        rows.append(output)
    cohorts = {
        "prior_history_gap_1575": [r for r in rows if r["previous_history_complete"] is False],
        "prior_unresolved_v4_1875": [r for r in rows if r["previous_result"] == "unresolved"],
        "prior_pass_controls_7": [r for r in rows if r["previous_result"] == "pass"],
    }
    requests = read(root / "requests.json")
    control_path = root / "historical_control" / "ticks.json"
    control = read(control_path) if control_path.exists() else None
    if control:
        requests += read(root / "historical_control" / "requests.json")
    summary = {"anchor": states["anchor"],
               "scope": "LP protection only; no new token-quality, activity, or trade-profitability qualification",
               "cohorts": {name: dict(collections.Counter(r["lp_result"] for r in group)) for name, group in cohorts.items()},
               "pool_state_reads_successful": sum(r["current_pool_state_read"] for r in rows), "pools": rows,
               "http_attempts": len(requests), "rpc_methods": dict(collections.Counter(r["method"] for r in requests)),
               "http_statuses": dict(collections.Counter(
                   str(r.get("http_status", "success" if r.get("success") else "other_failure")) for r in requests)),
               "initial_connectivity_probes_outside_ledger": {"sandbox_connection_failure": 1, "http_403": 1},
               "historical_control": {"anchor": control["anchor"], "results": dict(collections.Counter(
                   r["result"] for r in control["pools"]))} if control else None}
    write(root / "summary.json", summary)
    print(json.dumps({k: v for k, v in summary.items() if k != "pools"}, indent=2))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--stage", default="anchor", choices=[
        "anchor", "state", "discover", "positions", "ticks", "historical_control", "summary"])
    parser.add_argument("--output", type=Path, default=HERE / "evidence")
    parser.add_argument("--endpoint", default="https://mainnet.base.org")
    args = parser.parse_args()
    if args.stage == "summary":
        summarize(args.output)
        return
    reader = Reader(args.output, args.endpoint)
    path = args.output / "anchor.json"
    if path.exists():
        anchor = read(path)
    else:
        b = reader.rpc("eth_getBlockByNumber", ["latest", False])
        anchor = {"block": int(b["number"], 16), "hash": b["hash"],
                  "timestamp": int(b["timestamp"], 16), "endpoint": args.endpoint,
                  "observed_at_utc": dt.datetime.now(dt.timezone.utc).isoformat()}
        anchor["block_time_utc"] = dt.datetime.fromtimestamp(anchor["timestamp"], dt.timezone.utc).isoformat()
        write(path, anchor)
    print(json.dumps(anchor), flush=True)
    if args.stage != "anchor":
        globals()[args.stage](reader, anchor)
        end = reader.rpc("eth_getBlockByNumber", [hex(anchor["block"]), False], refresh=True)
        if end["hash"] != anchor["hash"]:
            raise ValueError("anchor_hash_changed")


if __name__ == "__main__":
    main()
