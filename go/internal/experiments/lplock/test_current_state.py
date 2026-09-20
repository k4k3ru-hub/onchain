"""Check current-state classification, rather than merely extracted fields."""

import copy
import json
import os
import subprocess
import unittest
from pathlib import Path

from current_state import CurrentState, finish, percentage
from test_analyze_source import SOURCE


def analyze(source, expires="99", gate="0x0"):
    module = os.environ.get("ONCHAIN_SOLC_MODULE")
    if not module:
        raise unittest.SkipTest("set ONCHAIN_SOLC_MODULE")
    payload = {"language": "Solidity", "sources": {"example.sol": {"content": source}},
               "settings": {"outputSelection": {"*": {"": ["ast"], "*": ["abi", "storageLayout"]}}}}
    program = "const s=require(process.argv[1]);let d='';process.stdin.on('data',x=>d+=x);process.stdin.on('end',()=>process.stdout.write(s.compile(d)));"
    result = subprocess.run(["node", "-e", program, module], input=json.dumps(payload), capture_output=True, text=True, check=True)
    compiled = json.loads(result.stdout)
    if any(e["severity"] == "error" for e in compiled.get("errors", [])):
        raise AssertionError("synthetic source failed compilation")
    discovery = {"pool": "p", "block_number": 1, "block_hash": "h", "block_timestamp": 50,
                 "custodian": "0x02", "manager": "0x01", "token_id": "9",
                 "complete_single_position_coverage": True, "principal_token0": "100000000000000000000000000000001", "principal_token1": "17"}
    observed = {**{k: discovery[k] for k in ["pool", "block_number", "block_hash", "custodian", "token_id"]},
                "time_fields": [{"value": expires}], "mutable_gates": [{"address": gate}],
                "locked_liquidity_percentage": None}
    analyzer = CurrentState(compiled, {"compilation": {"fullyQualifiedName": "example.sol:Example"}}, discovery, observed)
    return discovery, observed, analyzer.compile_plan()


class CurrentStateTest(unittest.TestCase):
    def test_time_and_zero_gate_block_exits(self):
        _, _, plan = analyze(SOURCE)
        self.assertEqual(plan["unresolved"], [])
        self.assertEqual(len(plan["routes"]), 2)
        self.assertTrue(all(r["assessment"] == "blocked_by_current_state" for r in plan["routes"]))

    def test_conditional_time_check_is_not_sufficient(self):
        altered = SOURCE.replace("require(parcel.ready < block.timestamp);", "if (receiver != msg.sender) { require(parcel.ready < block.timestamp); }")
        self.assertTrue(analyze(altered)[2]["unresolved"])

    def test_expired_lock_and_enabled_gate_are_unresolved(self):
        self.assertTrue(analyze(SOURCE, expires="49")[2]["unresolved"])
        self.assertTrue(analyze(SOURCE, gate="0x1234")[2]["unresolved"])

    def test_another_record_cannot_supply_the_time_check(self):
        altered = SOURCE.replace("require(parcel.ready < block.timestamp);", "require(parcels[index + 1].ready < block.timestamp);")
        self.assertTrue(analyze(altered)[2]["unresolved"])

    def test_extra_unconditional_exit_is_not_ignored(self):
        extra = "function escape(uint256 index, address receiver) external { Parcel memory parcel = parcels[index]; parcel.item.safeTransferFrom(address(this), receiver, parcel.serial); }"
        altered = SOURCE[:SOURCE.rfind("}")] + extra + "}"
        self.assertTrue(analyze(altered)[2]["unresolved"])

    def test_state_mutation_before_guard_is_not_frozen(self):
        altered = SOURCE.replace("require(parcel.ready < block.timestamp);", "parcel.ready = 0; require(parcel.ready < block.timestamp);")
        self.assertTrue(analyze(altered)[2]["unresolved"])

    def test_identifier_renaming_preserves_verdict(self):
        changed = SOURCE
        for before, after in [("Parcel", "Bundle"), ("parcels", "records"), ("parcel", "bundle"), ("ready", "moment"), ("holder", "beneficiary"), ("destination", "delegate"), ("administrator", "controller"), ("releaseParcel", "dispatch"), ("changeRoute", "reroute")]:
            changed = changed.replace(before, after)
        plan = analyze(changed)[2]
        self.assertEqual(plan["unresolved"], [])
        self.assertEqual(len(plan["routes"]), 2)

    def test_finish_needs_coverage_identity_and_all_probes(self):
        discovery, observed, plan = analyze(SOURCE)
        probes = {**{k: discovery[k] for k in ["pool", "block_number", "block_hash", "custodian", "token_id"]},
                  "exit_checks": [{"selector": p["selector"], "reverted": True} for p in plan["probe_entries"]],
                  "runtime_unchanged": True, "custody_unchanged": True, "incoming_existing_nft_reverted": True,
                  "direct_decrease_reverted": True, "erc20_selector_reverted": True}
        result = finish(discovery, observed, plan, probes)
        self.assertEqual(result["locked_liquidity_percentage"], "100")
        self.assertEqual(result["locked_principal"]["token0"], discovery["principal_token0"])
        self.assertFalse(result["arbitrary_contract_certification"])
        for key, value in [("block_hash", "different"), ("exit_checks", []), ("erc20_selector_reverted", False)]:
            changed = copy.deepcopy(probes)
            changed[key] = value
            with self.assertRaises(ValueError):
                finish(discovery, observed, plan, changed)
        with self.assertRaises(ValueError):
            finish({**discovery, "complete_single_position_coverage": False}, observed, plan, probes)

    def test_exact_ratio_and_invalid_denominator(self):
        self.assertEqual(percentage("1", "3"), "100/3")
        self.assertEqual(percentage("0", "10"), "0")
        with self.assertRaises(ValueError):
            percentage("1", "0")

    def test_no_locker_registry_or_reference_inputs(self):
        for name in ["current_state.py", "current_state_test.go"]:
            content = (Path(__file__).parent / name).read_text()
            for forbidden in ["reviewedRuntime", "reviewedSource", "getLock(", "lockSignature", "discoverLock(", "verifySource(", "reference-result", "1791648509", "5951125", "1158"]:
                self.assertNotIn(forbidden, content, (name, forbidden))


if __name__ == "__main__":
    unittest.main()
