"""Offline checks for bounded research acquisition; no live service calls."""

import contextlib
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import urllib.error

import probe


class AcquisitionTests(unittest.TestCase):
    def test_three_retries_survive_restart_and_count_error_bytes(self):
        plan = [{"name": "chain", "method": "eth_chainId", "params": []}]
        failures = [urllib.error.HTTPError("https://mainnet.base.org", 500, "failure", {}, io.BytesIO(b"failure")) for _ in range(4)]
        with tempfile.TemporaryDirectory() as temp, patch.object(probe.time, "sleep"), patch.object(probe.urllib.request, "urlopen", side_effect=failures) as request, contextlib.redirect_stdout(io.StringIO()):
            output = Path(temp) / "ledger.json"
            with self.assertRaises(RuntimeError):
                probe.collect(plan, output)
            data = json.loads(output.read_text())
            self.assertEqual(request.call_count, 4)
            self.assertEqual(sum(x["response_bytes"] for x in data["attempts"]), 28)
            with self.assertRaises(RuntimeError):
                probe.collect(plan, output)
            self.assertEqual(request.call_count, 4)

    def test_cached_identity_must_match(self):
        with tempfile.TemporaryDirectory() as temp, patch.object(probe.urllib.request, "urlopen") as request:
            output = Path(temp) / "ledger.json"
            output.write_text(json.dumps({"results": {"state": {"method": "eth_getCode", "params": ["address", "0x1"], "result": "0x"}}, "attempts": [], "runs": []}))
            with self.assertRaises(ValueError):
                probe.collect([{"name": "state", "method": "eth_getCode", "params": ["address", "0x2"]}], output)
            request.assert_not_called()

    def test_budget_and_read_only_boundary(self):
        with tempfile.TemporaryDirectory() as temp, patch.object(probe.urllib.request, "urlopen") as request:
            output = Path(temp) / "ledger.json"
            with self.assertRaises(ValueError):
                probe.collect([{"name": "send", "method": "eth_sendTransaction", "params": []}], output)
            output.write_text(json.dumps({"attempts": [{"key": "prior", "success": True}] * 128, "results": {}, "runs": []}))
            with self.assertRaises(RuntimeError):
                probe.collect([{"name": "chain", "method": "eth_chainId", "params": []}], output)
            request.assert_not_called()


if __name__ == "__main__":
    unittest.main()
