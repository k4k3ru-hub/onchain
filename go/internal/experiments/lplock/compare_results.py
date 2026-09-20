"""Compare a completed independent extraction with a separate reference run."""

import argparse
import hashlib
import json
from pathlib import Path
from fractions import Fraction


def compare(independent, reference):
    for field in ["pool", "block_number", "block_hash", "custodian", "token_id"]:
        if independent[field] != reference[field]:
            raise ValueError("comparison_identity_mismatch:" + field)
    if len(independent["time_fields"]) != 1 or len(independent["mutable_gates"]) != 1:
        raise ValueError("comparison_shape_unsupported")
    gate = independent["mutable_gates"][0]
    matches = {
        "record_id": independent["record_key_discovered"] == reference["lock_id"],
        "time_value": independent["time_fields"][0]["value"] == reference["unlock_date"],
        "record_owner": independent["record_owner_addresses"] == [reference["lock_owner"]],
        "migration_address": gate["address"] == reference["migrator"],
        "writer_role_address": gate["writer_role_addresses"] == [reference["admin_address"]],
    }
    if "pool_principal" in independent:
        for token in ["token0", "token1"]:
            matches["principal_" + token] = independent["pool_principal"][token] == reference["principal_" + token]
    return {
        "pool": independent["pool"], "block_number": independent["block_number"],
        "field_matches": matches,
        "independent_locked_liquidity_percentage": independent["locked_liquidity_percentage"],
        "reference_percentage": reference["percentage"],
        "percentage_comparison": "not_reached" if independent["locked_liquidity_percentage"] is None else (
            "matched" if Fraction(independent["locked_liquidity_percentage"]) == Fraction(reference["percentage"]) else "mismatched"),
        "reference_does_not_fill_independent_unknowns": True,
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    parser.add_argument("--independent", default="independent-result.json")
    args = parser.parse_args()
    raw = (args.directory / args.independent).read_bytes()
    reference = json.loads((args.directory / "reference-result.json").read_text())
    result = compare(json.loads(raw), reference)
    result["independent_result_sha256"] = hashlib.sha256(raw).hexdigest()
    (args.directory / "comparison.json").write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps(result, indent=2))
    if not all(result["field_matches"].values()) or result["percentage_comparison"] == "mismatched":
        raise SystemExit(1)


if __name__ == "__main__":
    main()
