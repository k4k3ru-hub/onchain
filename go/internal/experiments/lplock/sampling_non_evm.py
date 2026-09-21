"""Sample creation events using the existing read-only discovery experiment.

The explicit --legacy-dir points at the earlier sibling-repository experiment;
this is a research harness, not a production SDK dependency. Sampling does not
promote an observed timestamp or address ownership to a verified lock ratio.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import sys
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("venue", choices=["meteora-dlmm", "cetus-clmm"])
    parser.add_argument("--legacy-dir", type=Path, required=True)
    parser.add_argument("--env-file", type=Path)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--count", type=int, default=10)
    parser.add_argument("--manifest", type=Path, help="reuse the frozen Cetus cohort and checkpoint")
    args = parser.parse_args()
    if not 1 <= args.count <= 30:
        raise ValueError("sample_count_out_of_range")
    sys.path.insert(0, str(args.legacy_dir.resolve()))
    from probe import Reader, CETUS
    from pool_first_non_evm import decode_creations, meteora_event, SOLANA

    class ThrottledReader(Reader):
        def __init__(self, output):
            super().__init__(output)
            self.immutable_cache = {}
            self.cache_hits = 0

        def request(self, url, payload=None):
            key = hashlib.sha256((url + json.dumps(payload, sort_keys=True)).encode()).hexdigest()
            immutable = payload is None or payload.get("method") == "getTransaction"
            if immutable and key in self.immutable_cache:
                self.cache_hits += 1
                return self.immutable_cache[key]
            for attempt in range(4):
                # Public Solana method limits are tighter than general HTTP limits.
                time.sleep(1.5 if payload and payload.get("method") in ["getTransaction", "getProgramAccounts"] else 0.15)
                try:
                    data = super().request(url, payload)
                    if immutable:
                        self.immutable_cache[key] = data
                    self.save("request_observation_" + key, {"payload": payload, "response": data})
                    return data
                except RuntimeError:
                    if attempt == 3:
                        raise
                    time.sleep(2 ** (attempt + 1))

    if args.env_file:
        os.environ["MARKETHUB_ENV_FILE"] = str(args.env_file.resolve())
    reader = ThrottledReader(args.output)
    started = time.monotonic()
    result = {"venue": args.venue, "selection_count": args.count, "samples": [], "selection_failures": [],
              "sampling_policy": "most recent verified pool creation events; frozen before custody inspection", "read_only": True}

    def save():
        result["requests"] = reader.requests
        result["immutable_cache_hits"] = reader.cache_hits
        result["wall_seconds"] = round(time.monotonic() - started, 3)
        reader.save(args.venue + "-samples", result)

    def object_at(address, checkpoint):
        return reader.gql('{object(address:"' + address + '",atCheckpoint:' + str(checkpoint) + '){address version owner{__typename ... on AddressOwner{address{address}} ... on ObjectOwner{address{address}}} asMoveObject{contents{type{repr} json}}}}')["object"]

    try:
        if args.venue == "meteora-dlmm":
            idl = reader.request("https://raw.githubusercontent.com/MeteoraAg/dlmm-sdk/main/idls/dlmm.json")
            reader.save("meteora-sampling-idl", idl)
            result["idl_sha256"] = hashlib.sha256(json.dumps(idl, sort_keys=True).encode()).hexdigest()
            candidates = reader.request("https://dlmm.datapi.meteora.ag/pools?page_size=30&sort_by=pool_created_at:desc")["data"]
            candidates = candidates[:args.count]
            if len(candidates) != args.count or len({c["address"] for c in candidates}) != args.count:
                raise RuntimeError("candidate_count_mismatch")
            reader.save(args.venue + "-candidate-manifest", {"candidates": candidates, "selection": "latest API candidates frozen before creation and custody inspection; failures are retained"})
            manifest = []
            for candidate in candidates:
                pool = candidate["address"]
                try:
                    options = {"limit": 1000, "commitment": "finalized"}
                    for page in range(3):
                        signatures = reader.rpc(SOLANA, "getSignaturesForAddress", [pool, options])
                        if not signatures:
                            raise RuntimeError("creation_history_unavailable")
                        if len(signatures) < 1000:
                            break
                        options["before"] = signatures[-1]["signature"]
                    if len(signatures) == 1000:
                        raise RuntimeError("creation_history_page_budget")
                    signature = signatures[-1]["signature"]
                    transaction = reader.rpc(SOLANA, "getTransaction", [signature, {"encoding": "json", "maxSupportedTransactionVersion": 0, "commitment": "finalized"}])
                    events = [e for e in decode_creations(transaction) if e["pool"] == pool]
                    if len(events) != 1:
                        raise RuntimeError("creation_event_not_matched")
                    manifest.append({"pool": pool, "signature": signature, "creation_slot": transaction["slot"], "creation_time": transaction.get("blockTime"), "event": events[0]})
                except RuntimeError as exc:
                    result["selection_failures"].append({"pool": pool, "reason": str(exc)})
                    manifest.append({"pool": pool, "reason": str(exc), "creation_unverified": True})
            reader.save(args.venue + "-manifest", {"venue": args.venue, "events": manifest, "candidate_source": "official index for discovery only; creation events verified by RPC"})
            for event in manifest:
                row = {"pool": event["pool"], "creation_event": event, "locked_liquidity_percentage": None}
                begin = len(reader.requests)
                try:
                    if event.get("creation_unverified"):
                        raise RuntimeError(event["reason"])
                    evidence = meteora_event(reader, event["signature"], idl)
                    matching = [s for s in evidence["samples"] if s["pool"] == event["pool"]]
                    if len(matching) != 1:
                        raise RuntimeError("replayed_creation_event_mismatch")
                    row["evidence"] = matching[0]
                    row["reason"] = matching[0]["reason"]
                except RuntimeError as exc:
                    row["reason"] = str(exc)
                row["http_attempts"] = len(reader.requests) - begin
                result["samples"].append(row)
                save()
                print(json.dumps({"venue": args.venue, "sample": len(result["samples"]), "pool": row["pool"], "reason": row["reason"]}), flush=True)
        else:
            frozen = json.loads(args.manifest.read_text()) if args.manifest else None
            checkpoint = frozen["checkpoint"] if frozen else reader.gql("{checkpoint{sequenceNumber timestamp}} ")["checkpoint"]
            observed = checkpoint["sequenceNumber"]
            events = frozen["events"] if frozen else reader.gql('{events(last:' + str(args.count) + ',filter:{type:"' + CETUS + '::factory::CreatePoolEvent"}){nodes{timestamp contents{json} transaction{digest effects{checkpoint{sequenceNumber}}}}}}')["events"]["nodes"]
            if len(events) != args.count or len({e["contents"]["json"]["pool_id"] for e in events}) != args.count:
                raise RuntimeError("creation_event_count_mismatch")
            reader.save(args.venue + "-manifest", {"venue": args.venue, "events": events, "checkpoint": checkpoint})
            result["checkpoint"] = checkpoint
            for event in events:
                pool = event["contents"]["json"]["pool_id"]
                row = {"pool": pool, "creation_event": event, "checkpoint": observed, "locked_liquidity_percentage": None, "positions": [], "reasons": []}
                begin = len(reader.requests)
                try:
                    obj = object_at(pool, observed)
                    if not obj:
                        raise RuntimeError("pool_object_unavailable")
                    row["pool_object"] = obj
                    fields = obj["asMoveObject"]["contents"]["json"]
                    handle = fields["position_manager"]["positions"]["id"]
                    positions = {"nodes": [], "pageInfo": {"hasNextPage": True}}
                    cursor = ""
                    for page in range(4):
                        batch = reader.gql('{address(address:"' + handle + '",atCheckpoint:' + str(observed) + '){dynamicFields(first:50' + cursor + '){nodes{name{json} value{... on MoveValue{json}}} pageInfo{hasNextPage endCursor}}}}')["address"]["dynamicFields"]
                        positions["nodes"].extend(batch["nodes"])
                        positions["pageInfo"] = batch["pageInfo"]
                        if not batch["pageInfo"]["hasNextPage"]:
                            break
                        cursor = ',after:' + json.dumps(batch["pageInfo"]["endCursor"])
                    row["position_fields"] = positions
                    if positions["pageInfo"]["hasNextPage"]:
                        row["reasons"].append("position_page_budget_reached")
                    for field in positions["nodes"]:
                        position_id = field["name"]["json"]
                        if isinstance(position_id, dict):
                            position_id = position_id["value"]
                        position = object_at(position_id, observed)
                        if position is None:
                            row["positions"].append({"address": position_id, "reason": "position_object_not_returned"})
                            row["reasons"].append("position_object_not_returned")
                            continue
                        if position["asMoveObject"]["contents"]["json"]["pool"] != pool:
                            raise RuntimeError("position_pool_mismatch")
                        current = position
                        visited = {position_id}
                        for depth in range(4):
                            if current["owner"]["__typename"] != "ObjectOwner":
                                break
                            parent = current["owner"]["address"]["address"]
                            if parent in visited:
                                raise RuntimeError("ownership_cycle")
                            visited.add(parent)
                            current["parent"] = object_at(parent, observed)
                            if current["parent"] is None:
                                row["reasons"].append("parent_object_not_returned")
                                break
                            current = current["parent"]
                        else:
                            if current["owner"]["__typename"] == "ObjectOwner":
                                row["reasons"].append("ownership_depth_budget_reached")
                        row["positions"].append(position)
                    row["reasons"].append("withdrawal_semantics_and_pool_principal_not_verified")
                except RuntimeError as exc:
                    row["reasons"].append(str(exc))
                row["reasons"] = sorted(set(row["reasons"]))
                row["http_attempts"] = len(reader.requests) - begin
                result["samples"].append(row)
                save()
                print(json.dumps({"venue": args.venue, "sample": len(result["samples"]), "pool": pool, "reasons": row["reasons"]}), flush=True)
        result["sampling_complete"] = len(result["samples"]) == args.count
    except Exception as exc:
        result["sampling_complete"] = False
        result["failure_type"] = type(exc).__name__
        if isinstance(exc, RuntimeError):
            result["failure_reason"] = str(exc)
    finally:
        save()
    print(json.dumps({"venue": args.venue, "sampling_complete": result["sampling_complete"], "sample_count": len(result["samples"]), "http_attempts": len(reader.requests)}), flush=True)
    return 0 if result["sampling_complete"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
