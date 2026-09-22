"""Offline regression tests: recorded EVM outcomes and injected failure paths."""

import copy
import json
from pathlib import Path
import unittest

from acquire import Reader
from analyze import Analyzer, fixture_models, fraction_string
from verify_live import match_runtime, verified_models


OUT = Path(__file__).parent / "evidence"


class RecordedTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.results = json.loads((OUT / "local-results.json").read_text())
        cls.cases = {x["case"]: x["analysis"]["taxes"] for x in cls.results["cases"]}

    def test_plain_erc20_positive_and_actual_transfers(self):
        self.assertEqual(self.cases["Plain"]["buyRate"], "0")
        self.assertEqual(self.cases["Plain"]["sellRate"], "0")
        self.assertIs(self.cases["Plain"]["canChange"], False)
        self.assertIs(self.cases["Plain"]["hasExemptions"], False)
        for x in self.results["transfers"]:
            if x["case"] == "Plain":
                self.assertEqual(x["sent"], x["received"])

    def test_proportional_tax_and_actual_transfers(self):
        for name in ("PoolTax", "RoleTax"):
            self.assertEqual(self.cases[name]["buyRate"], "0.02")
            self.assertEqual(self.cases[name]["sellRate"], "0.05")
            self.assertIs(self.cases[name]["canChange"], True)
            self.assertIs(self.cases[name]["hasExemptions"], True)
        expected = {"buy": 9800, "sell": 9500, "exempt_buy": 10000}
        for x in self.results["transfers"]:
            if x["case"] in ("PoolTax", "RoleTax"):
                self.assertEqual(x["received"], expected[x["direction"]])

    def test_fake_zero_is_not_accepted(self):
        self.assertIsNone(self.cases["FakeZero"])
        observed = [x for x in self.results["transfers"] if x["case"] == "FakeZero"]
        self.assertEqual([x["deducted"] for x in observed], [900, 900, 900, 900])
        self.assertEqual(int(self.results["controls"]["fakeBuyTax"], 16), 0)
        self.assertEqual(int(self.results["controls"]["fakeSellTax"], 16), 0)

    def test_complex_tax_not_misrepresented_as_single_rate(self):
        self.assertIsNone(self.cases["ComplexTax"]["buyRate"])
        self.assertIsNone(self.cases["ComplexTax"]["sellRate"])
        small = next(x for x in self.results["transfers"] if x["direction"] == "small_buy")
        self.assertEqual(small["deducted"], 2)

    def test_owner_zero_still_allows_tax_change(self):
        c = self.results["controls"]
        self.assertEqual(int(c["roleOwner"], 16), 0)
        self.assertNotEqual(int(c["roleController"], 16), 0)
        self.assertEqual(int(c["roleEmptyExemption"], 16), 0)
        self.assertEqual(int(c["roleChangedBuyBps"], 16), 300)
        self.assertEqual(int(c["roleChangedSellBps"], 16), 700)

    def test_proxy_upgrade_independent_of_current_zero_rate(self):
        self.assertEqual(self.cases["StandardProxy"]["buyRate"], "0")
        self.assertIs(self.cases["StandardProxy"]["canChange"], True)
        self.assertIsNone(self.cases["UnknownProxy"])
        c = self.results["controls"]
        self.assertEqual(int(c["proxyImplementationAfterUpgrade"], 16), int(c["expectedProxyImplementation"], 16))

    def test_costs_warm_state_and_other_pool(self):
        cases = {x["case"]: x for x in self.results["costs"]}
        self.assertEqual(cases["warm"]["rpc"]["cacheHits"], 5)
        self.assertEqual(cases["warm"]["rpc"]["httpAttempts"], 2)
        self.assertEqual(cases["other_pool"]["analysis"]["taxes"]["buyRate"], "0")
        self.assertEqual(cases["other_pool"]["rpc"]["httpAttempts"], 3)
        self.assertEqual(cases["new_block_current_zero"]["analysis"]["taxes"]["buyRate"], "0")
        self.assertIs(cases["new_block_current_zero"]["analysis"]["taxes"]["canChange"], True)
        self.assertIsNone(cases["v4_pool_id"]["analysis"]["taxes"]["buyRate"])


class FailureTests(unittest.TestCase):
    def setUp(self):
        self.compiled = json.loads((OUT / "compiled.json").read_text())
        self.models = fixture_models(self.compiled)
        self.case = json.loads((OUT / "local-results.json").read_text())["cases"][1]
        self.lookup = {json.dumps([x["method"], x["params"]], sort_keys=True): x["result"]
                       for x in self.case["ledger"]}
        self.block = self.case["ledger"][0]["result"]
        self.token = self.case["address"]
        # Pool comes from the actual mapping read, independent of expected output.
        call = next(x for x in self.case["ledger"] if x["method"] == "eth_call" and len(x["params"][0]["data"]) == 74)
        self.pool = "0x" + call["params"][0]["data"][-40:]

    def test_initial_plus_three_retries_and_success_reuse(self):
        sleep = []
        fail_selector = self.compiled["contracts"]["PoolTax"]["evm"]["methodIdentifiers"]["sellBps()"]

        def transport(method, params):
            if method == "eth_call" and params[0]["data"] == "0x" + fail_selector:
                raise OSError("injected failure")
            return self.lookup[json.dumps([method, params], sort_keys=True)]

        reader = Reader("injected", delay=0, sleep=sleep.append, transport=transport)
        analyzer = Analyzer(reader, self.models)
        result = analyzer.inspect(self.token, self.pool, "uniswap-v3", self.block)["taxes"]
        self.assertEqual(result["buyRate"], "0.02")
        self.assertIsNone(result["sellRate"])
        self.assertIs(result["hasExemptions"], True)
        self.assertEqual(reader.stats()["failedAttempts"], 4)
        self.assertEqual([v for v in sleep if v], [1, 2, 4])
        analyzer.inspect(self.token, self.pool, "uniswap-v3", self.block)
        self.assertEqual(reader.stats()["cacheHits"], 4)
        self.assertEqual(reader.stats()["failedAttempts"], 8)

    def test_same_height_different_hash_does_not_reuse_state(self):
        reader = Reader("injected", delay=0, transport=lambda method, params: "0x01")
        reader.state("eth_getCode", [self.token], self.block)
        other = {**self.block, "hash": "0x" + "ab" * 32}
        reader.state("eth_getCode", [self.token], other)
        self.assertEqual(reader.stats()["httpAttempts"], 2)
        self.assertEqual(reader.stats()["cacheHits"], 0)

    def test_unsupported_source_cannot_register_plain_name(self):
        tampered = copy.deepcopy(self.compiled)
        tampered["contracts"]["Plain"]["evm"]["deployedBytecode"]["object"] = "00"
        with self.assertRaises(ValueError):
            fixture_models(tampered)

    def test_no_remote_write_methods(self):
        reader = Reader("injected", transport=lambda *args: self.fail("transport must not run"))
        with self.assertRaises(ValueError):
            reader.rpc("eth_sendTransaction", [{}])

    def test_code_read_failure_returns_null(self):
        def unavailable(*args):
            raise OSError("injected failure")

        reader = Reader("injected", delay=0, sleep=lambda _: None, transport=unavailable)
        result = Analyzer(reader, self.models).inspect(self.token, self.pool, "uniswap-v3", self.block)
        self.assertIsNone(result["taxes"])
        self.assertEqual(reader.stats()["httpAttempts"], 4)

    def test_exact_fraction_not_ppm_rounding(self):
        self.assertEqual(fraction_string(1, 10**9), "0.000000001")
        self.assertEqual(fraction_string(1, 8), "0.125")
        for n, d in ((1, 3), (2, 1), (-1, 100)):
            with self.assertRaises(ValueError):
                fraction_string(n, d)


class LiveCodeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.live = json.loads((OUT / "live.json").read_text())
        cls.compiled = json.loads((OUT / "live-compiled.json").read_text())

    def test_three_recompiled_models_match_and_usdc_stays_unresolved(self):
        models, evidence = verified_models(self.live, self.compiled)
        self.assertEqual(len(models), 3)
        self.assertEqual(sum(x["matched"] for x in evidence), 3)
        self.assertEqual(evidence[-1]["reason"], "proxy_implementation_not_analyzed")

    def test_executable_change_is_rejected(self):
        address = "0x4200000000000000000000000000000000000006"
        raw = bytearray.fromhex(self.live["tokens"][address]["code"][2:])
        raw[10] ^= 1
        with self.assertRaises(ValueError):
            match_runtime(self.compiled[address]["artifact"], raw.hex(), allow_metadata=True)

    def test_unreviewed_source_bundle_is_rejected(self):
        tampered = copy.deepcopy(self.live)
        address = "0x4200000000000000000000000000000000000006"
        tampered["tokens"][address]["source"]["sources"]["WETH9.sol"]["content"] += " changed"
        with self.assertRaises(ValueError):
            verified_models(tampered, self.compiled)

    def test_live_results_and_cache(self):
        results = json.loads((OUT / "live-results.json").read_text())
        cases = results["cases"]
        self.assertEqual(len(cases), 3)
        self.assertEqual(cases[1]["rpc"]["httpAttempts"], 2)
        self.assertEqual(cases[1]["rpc"]["cacheHits"], 4)
        for case in cases:
            taxes = [v for p in case["pairs"] for v in p["tokenTaxes"].values()]
            self.assertEqual(sum(t is not None for t in taxes), 3)
            for tax in filter(None, taxes):
                self.assertEqual(tax["buyRate"], "0")
                self.assertEqual(tax["sellRate"], "0")
                self.assertIs(tax["canChange"], False)


if __name__ == "__main__":
    unittest.main()
