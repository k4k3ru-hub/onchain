"""Exercise source-derived layouts with unrelated synthetic contract names."""

import json
import os
from pathlib import Path
import subprocess
import unittest

from analyze_source import Analyzer
from compare_results import compare


SOURCE = """
pragma solidity 0.8.19;
interface IAsset {
    function safeTransferFrom(address from, address to, uint256 id) external;
    function approve(address operator, uint256 id) external;
}
contract Example {
    struct Parcel { IAsset item; uint256 serial; address holder; uint48 ready; }
    mapping(uint256 => Parcel) private parcels;
    address private destination;
    address private administrator;
    function releaseParcel(uint256 index, address receiver) external {
        Parcel memory parcel = parcels[index];
        require(parcel.ready < block.timestamp);
        require(parcel.holder == msg.sender);
        parcel.item.safeTransferFrom(address(this), receiver, parcel.serial);
    }
    function changeRoute(uint256 index) external {
        require(destination != address(0));
        Parcel memory parcel = parcels[index];
        parcel.item.approve(destination, parcel.serial);
    }
    function configureRoute(address newDestination) external {
        require(administrator == msg.sender);
        destination = newDestination;
    }
}
"""


def compile_example(source):
    module = os.environ.get("ONCHAIN_SOLC_MODULE")
    if not module:
        raise unittest.SkipTest("set ONCHAIN_SOLC_MODULE to an isolated solc module")
    program = "const s=require(process.argv[1]);let d='';process.stdin.on('data',x=>d+=x);process.stdin.on('end',()=>process.stdout.write(s.compile(d)));"
    payload = {"language": "Solidity", "sources": {"example.sol": {"content": source}}, "settings": {"outputSelection": {"*": {"": ["ast"], "*": ["abi", "storageLayout"]}}}}
    result = subprocess.run(["node", "-e", program, module], input=json.dumps(payload), capture_output=True, text=True, check=True)
    compiled = json.loads(result.stdout)
    if any(e["severity"] == "error" for e in compiled.get("errors", [])):
        raise AssertionError("synthetic source failed compilation")
    return Analyzer(compiled, {"compilation": {"fullyQualifiedName": "example.sol:Example"}}).build()


class SourceAnalysisTest(unittest.TestCase):
    def test_unrelated_names_and_packed_time_field(self):
        plan = compile_example(SOURCE)
        self.assertEqual(len(plan["records"]), 1)
        record = plan["records"][0]
        self.assertEqual(record["time_fields"][0]["bytes"], 6)
        self.assertEqual(record["time_fields"][0]["offset"], 20)
        self.assertEqual(record["manager_field"]["slot"], "0")
        self.assertEqual(record["nft_field"]["slot"], "1")
        self.assertEqual(len(plan["mutable_gates"]), 1)
        self.assertEqual(len(plan["mutable_gates"][0]["writers"][0]["role_storage"]), 1)
        self.assertIsNone(plan["locked_liquidity_percentage"])

    def test_custom_identifier_renaming_preserves_semantic_fields(self):
        changed = SOURCE
        for before, after in [("Parcel", "Bundle"), ("parcels", "records"), ("parcel", "bundle"), ("ready", "moment"), ("holder", "beneficiary"), ("destination", "delegate"), ("administrator", "controller")]:
            changed = changed.replace(before, after)
        first, second = compile_example(SOURCE), compile_example(changed)
        for plan in [first, second]:
            for record in plan["records"]:
                record.pop("entry_selector")
        self.assertEqual(first["records"], second["records"])

    def test_conditional_guard_cannot_certify_lock(self):
        changed = SOURCE.replace("require(parcel.ready < block.timestamp);", "if (receiver != msg.sender) { require(parcel.ready < block.timestamp); }")
        plan = compile_example(changed)
        self.assertEqual(len(plan["records"]), 1)
        self.assertFalse(plan["records"][0]["guard_dominance_proven"])
        self.assertIsNone(plan["locked_liquidity_percentage"])

    def test_absent_time_condition_produces_no_time_lock_candidate(self):
        plan = compile_example(SOURCE.replace("require(parcel.ready < block.timestamp);", ""))
        self.assertEqual(plan["records"], [])
        self.assertIsNone(plan["locked_liquidity_percentage"])

    def test_independent_sources_do_not_reference_registry_or_reference_values(self):
        root = Path(__file__).parent
        for name in ["analyze_source.py", "independent_discovery_test.go", "independent_storage_test.go"]:
            source = (root / name).read_text()
            for forbidden in ["reviewedRuntime", "reviewedSource", "getLock(", "lockSignature", "discoverLock(", "verifySource(", "reference-result", "1791648509", "5951125", "1158"]:
                self.assertNotIn(forbidden, source, (name, forbidden))

    def test_reference_does_not_convert_unknown_percentage_to_confirmed(self):
        independent = {
            "pool": "p", "block_number": 10, "block_hash": "h", "custodian": "c", "token_id": "9",
            "record_key_discovered": "7", "time_fields": [{"value": "99"}],
            "record_owner_addresses": ["a"], "mutable_gates": [{"address": "zero", "writer_role_addresses": ["b"]}],
            "locked_liquidity_percentage": None,
        }
        reference = {**{k: independent[k] for k in ["pool", "block_number", "block_hash", "custodian", "token_id"]},
                     "lock_id": "7", "unlock_date": "99", "lock_owner": "a", "migrator": "zero", "admin_address": "b", "percentage": "100"}
        result = compare(independent, reference)
        self.assertTrue(all(result["field_matches"].values()))
        self.assertIsNone(result["independent_locked_liquidity_percentage"])
        self.assertEqual(result["percentage_comparison"], "not_reached")
        independent["locked_liquidity_percentage"] = "100"
        self.assertEqual(compare(independent, reference)["percentage_comparison"], "matched")
        independent["locked_liquidity_percentage"] = "99"
        self.assertEqual(compare(independent, reference)["percentage_comparison"], "mismatched")
        reference["block_number"] = 11
        with self.assertRaisesRegex(ValueError, "identity_mismatch"):
            compare(independent, reference)


if __name__ == "__main__":
    unittest.main()
