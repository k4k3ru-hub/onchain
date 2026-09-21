"""Regression tests against recorded fork evidence and read-only transport bounds."""

import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from assess import assess
from fork_probe import MeteredProxy, TOKEN, cacheable
from probe import Capture


EVIDENCE = Path(__file__).parent / "evidence"


class AssessmentTests(unittest.TestCase):
    def setUp(self):
        self.ordinary = json.loads((EVIDENCE / "ordinary.json").read_text())
        self.owner = json.loads((EVIDENCE / "owner.json").read_text())

    def check(self):
        return assess(self.ordinary, self.owner, TOKEN)

    def test_recorded_auto_restriction_and_allowlist_bypass(self):
        result = self.check()
        for field in ("automaticRecipientRestriction", "ownerControlledAllowlistBypass",
                      "buyThenSellRejectedByToken", "ownerRoundTripSucceeded"):
            self.assertEqual(result[field], "observed")
        self.assertEqual(result["safetyVerdict"], "not_provided")

    def test_recorded_dex_pause_is_inconclusive(self):
        self.ordinary = json.loads((EVIDENCE / "dex_stopped.json").read_text())
        self.assertEqual(self.check()["buyThenSellRejectedByToken"], "inconclusive")

    def test_rpc_error_is_not_token_revert(self):
        self.ordinary["sellNextBlock"] = {"error": {"code": -32001, "message": "upstream unavailable"}}
        self.assertEqual(self.check()["buyThenSellRejectedByToken"], "inconclusive")

    def test_insufficient_balance_or_allowance_is_not_restriction(self):
        for field in ("balanceBeforeSellRaw", "allowanceBeforeSellRaw"):
            with self.subTest(field=field):
                previous = self.ordinary[field]
                self.ordinary[field] = "0"
                self.assertEqual(self.check()["buyThenSellRejectedByToken"], "inconclusive")
                self.ordinary[field] = previous

    def test_router_revert_without_token_revert_is_inconclusive(self):
        self.ordinary["sellTraceNextBlock"]["result"]["calls"] = []
        self.assertEqual(self.check()["buyThenSellRejectedByToken"], "inconclusive")

    def test_missing_causal_comparison_does_not_prove_automatic_restriction(self):
        self.ordinary.pop("restrictionSlotCounterfactuals")
        self.assertEqual(self.check()["automaticRecipientRestriction"], "inconclusive")
        self.assertEqual(self.check()["buyThenSellRejectedByToken"], "observed")

    def test_changed_balance_invalidates_flag_comparison(self):
        for row in self.ordinary["restrictionSlotCounterfactuals"]:
            row["balanceUnchanged"] = False
        self.assertEqual(self.check()["automaticRecipientRestriction"], "inconclusive")

    def test_failed_owner_mutation_does_not_prove_allowlist_bypass(self):
        self.ordinary["allowlistOwnerReceipt"]["status"] = "0x0"
        self.assertEqual(self.check()["ownerControlledAllowlistBypass"], "inconclusive")

    def test_missing_data_does_not_mean_safe(self):
        result = assess({}, {}, TOKEN)
        self.assertEqual(result["automaticRecipientRestriction"], "inconclusive")
        self.assertEqual(result["safetyVerdict"], "not_provided")


class ProxyTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.capture = Capture(Path(self.directory.name) / "run")
        self.proxy = MeteredProxy(self.capture)

    def test_public_proxy_rejects_transaction_and_storage_mutation(self):
        with patch.object(self.capture, "rpc") as rpc:
            for method in ("eth_sendTransaction", "eth_sendRawTransaction", "anvil_setStorageAt"):
                response = self.proxy.forward({"id": 1, "method": method, "params": []})
                self.assertEqual(response["error"]["code"], -32601)
            rpc.assert_not_called()

    def test_capture_also_rejects_remote_writes(self):
        with self.assertRaises(ValueError):
            self.capture.rpc("eth_sendRawTransaction", ["0x00"])

    def test_retries_are_bounded_and_abort_later_queries(self):
        def unavailable(method, params):
            self.capture.requests.append({"method": method})
            return None

        with patch.object(self.capture, "rpc", side_effect=unavailable) as rpc, patch("fork_probe.time.sleep"):
            request = {"id": 1, "method": "eth_chainId", "params": []}
            self.assertIn("error", self.proxy.forward(request))
            self.assertIn("error", self.proxy.forward(request))
            self.assertEqual(rpc.call_count, 4)
            self.assertTrue(self.proxy.failed)

    def test_budget_stops_new_upstream_requests(self):
        self.proxy.budget = 0
        with patch.object(self.capture, "rpc") as rpc:
            response = self.proxy.forward({"id": 1, "method": "eth_chainId", "params": []})
            self.assertIn("error", response)
            rpc.assert_not_called()

    def test_pinned_reads_reuse_cache_and_preserve_response_id(self):
        request = {"id": 1, "method": "eth_getCode", "params": [TOKEN, "0x1f02a2"]}
        with patch.object(self.capture, "rpc", return_value={"result": "0x1234"}) as rpc:
            self.assertEqual(self.proxy.forward(request)["result"], "0x1234")
            repeated = {**request, "id": 2}
            self.assertEqual(self.proxy.forward(repeated)["id"], 2)
            self.assertEqual(rpc.call_count, 1)
            self.assertEqual(self.proxy.hits, 1)

    def test_live_head_pending_and_missing_values_are_not_cached(self):
        for method, params, response in (
            ("eth_blockNumber", [], {"result": "0x42"}),
            ("eth_getCode", [TOKEN, "latest"], {"result": "0x1234"}),
            ("eth_getCode", [TOKEN, "pending"], {"result": "0x1234"}),
            ("eth_getTransactionReceipt", ["0x1234"], {"result": None}),
            ("eth_getCode", [TOKEN, "0x42"], {"error": {"code": -32001}}),
        ):
            with self.subTest(method=method, params=params):
                self.assertFalse(cacheable(method, params, response))


if __name__ == "__main__":
    unittest.main()
