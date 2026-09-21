"""Compare discovered custodians to locally compiled source and read its getters.

The source is selected by code investigation, not a locker allowlist. Matching
code/getters are evidence for manual review, never an automatic lock verdict.
"""
import argparse
import hashlib
import json
from pathlib import Path
import sys
import time


def match_runtime(compiled, code):
    local = bytes.fromhex(compiled["object"])
    observed = bytes.fromhex(code.removeprefix("0x"))
    if len(local) != len(observed) or compiled.get("linkReferences"):
        return None
    allowed, values = set(), {}
    for identifier, sites in compiled.get("immutableReferences", {}).items():
        repeated = set()
        for site in sites:
            start, length = site["start"], site["length"]
            if start < 0 or length <= 0 or start + length > len(local):
                return None
            repeated.add(observed[start:start + length].hex())
            allowed.update(range(start, start + length))
        if len(repeated) != 1:
            return None
        values[identifier] = repeated.pop()
    if any(a != b and i not in allowed for i, (a, b) in enumerate(zip(local, observed))):
        return None
    return values


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--legacy-dir", type=Path, required=True)
    parser.add_argument("--samples", type=Path, required=True)
    parser.add_argument("--source", type=Path, required=True)
    parser.add_argument("--compiled", type=Path, required=True)
    parser.add_argument("--getter", action="append", required=True)
    parser.add_argument("--clones", action="store_true", help="compare EIP-1167 implementation code and call getters on the discovered clone")
    parser.add_argument("--resume", action="store_true", help="reuse completed observations at this exact block and source")
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    sys.path.insert(0, str(args.legacy_dir.resolve()))
    from probe import Reader, keccak
    from sampling_custody import code_structure
    source, compiled = json.loads(args.source.read_text()), json.loads(args.compiled.read_text())
    file, name = source["compilation"]["fullyQualifiedName"].rsplit(":", 1)
    contract = compiled["contracts"][file][name]
    getters = {a["name"]: a for a in contract["abi"] if a["type"] == "function" and not a["inputs"] and a["stateMutability"] == "view"}
    if any(name not in getters for name in args.getter):
        raise ValueError("getter_not_in_compiled_abi")
    samples = json.loads(args.samples.read_text())
    reader = Reader(args.output)
    result = {"block_number": samples["block_number"], "block_hash": samples["block_hash"], "source_sha256": hashlib.sha256(args.source.read_bytes()).hexdigest(), "compiled_sha256": hashlib.sha256(args.compiled.read_bytes()).hexdigest(), "custodians": [], "automatic_lock_verdict": False}
    previous_requests = []
    if args.resume:
        previous = json.loads((args.output / "runtime-review.json").read_text())
        if any(previous[key] != result[key] for key in ["block_number", "block_hash", "source_sha256", "compiled_sha256"]):
            raise ValueError("resume_evidence_mismatch")
        result["custodians"] = previous["custodians"]
        previous_requests = previous["requests"]

    def rpc(method, params):
        for attempt in range(4):
            time.sleep(1.5 if attempt == 0 else 2 ** attempt)
            try:
                return reader.rpc("https://mainnet.base.org", method, params)
            except RuntimeError:
                if attempt == 3:
                    raise

    block = hex(samples["block_number"])
    def check_block():
        if rpc("eth_getBlockByNumber", [block, False])["hash"] != samples["block_hash"]:
            raise RuntimeError("block_hash_mismatch")

    check_block()
    owners = {}
    for row in samples["samples"]:
        for position in row["positions"]:
            if (args.clones and position["owner_code_bytes"] >= 45 and position["withdrawal_assessment"] == "unresolved") or (not args.clones and position["owner_code_bytes"] == len(contract["evm"]["deployedBytecode"]["object"]) // 2):
                owners[position["owner"]] = position["owner_code_hash"]
    implementation_codes = {}
    for owner, expected_hash in owners.items():
        if any(r["address"] == owner and all(name in r["getters"] for name in args.getter) for r in result["custodians"]):
            continue
        observation = {"address": owner, "getters": {}}
        try:
            code = rpc("eth_getCode", [owner, block])
            reader.save("runtime-" + owner, {"code": code, "block_number": samples["block_number"]})
            if "0x" + keccak(bytes.fromhex(code[2:])) != expected_hash:
                raise RuntimeError("observed_runtime_hash_mismatch")
            if args.clones:
                structure = code_structure(code)
                if structure["kind"] != "eip1167":
                    raise RuntimeError("clone_structure_unsupported")
                target = structure["target"]
                observation["implementation"] = target
                if target not in implementation_codes:
                    implementation_codes[target] = rpc("eth_getCode", [target, block])
                    reader.save("runtime-" + target, {"code": implementation_codes[target], "block_number": samples["block_number"]})
                code = implementation_codes[target]
            values = match_runtime(contract["evm"]["deployedBytecode"], code)
            observation["compiled_runtime_matches"] = values is not None
            if values is None:
                raise RuntimeError("compiled_runtime_mismatch")
            observation["immutable_values_by_ast_id"] = values
            for name in args.getter:
                data = "0x" + keccak((name + "()").encode())[:8]
                observation["getters"][name] = rpc("eth_call", [{"to": owner, "data": data}, block])
        except RuntimeError as exc:
            observation["reason"] = str(exc)
        result["custodians"].append(observation)
        result["requests"] = previous_requests + reader.requests
        reader.save("runtime-review", result)
        print(json.dumps({"address": owner, "matched": observation.get("compiled_runtime_matches"), "getters": observation["getters"]}), flush=True)
    check_block()
    result["final_block_hash_verified"] = True
    result["requests"] = previous_requests + reader.requests
    reader.save("runtime-review", result)


if __name__ == "__main__":
    main()
