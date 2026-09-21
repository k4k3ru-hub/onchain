"""Offline checks for financial arithmetic and incomplete-observation handling."""
import tempfile
import unittest
from pathlib import Path
from unittest import mock
import urllib.error

from batch import aggregate_data, aggregate_result, word
from census import Reader, RPCFailure
from quality import principal, sqrt_at_tick


class QualityTests(unittest.TestCase):
    def test_tick_math_protocol_boundary_vectors(self):
        self.assertEqual(sqrt_at_tick(-887272), 4295128739)
        self.assertEqual(sqrt_at_tick(0), 2 ** 96)
        self.assertEqual(sqrt_at_tick(887272), 1461446703485210103287273052203988822378723970342)
        with self.assertRaises(ValueError):
            sqrt_at_tick(887273)

    def test_principal_outside_range_is_one_sided(self):
        positions = [{"lower": 100, "upper": 200, "liquidity": "1000000000000000000"}]
        amount0, amount1 = principal(positions, sqrt_at_tick(0))
        self.assertGreater(amount0, 0)
        self.assertEqual(amount1, 0)
        amount0, amount1 = principal(positions, sqrt_at_tick(300))
        self.assertEqual(amount0, 0)
        self.assertGreater(amount1, 0)

    def test_zero_rounding_is_not_positive_principal(self):
        self.assertEqual(principal([{"lower": -1, "upper": 1, "liquidity": "1"}], 2 ** 96), (0, 0))

    def test_aggregate_rejects_bad_address_and_truncated_response(self):
        with self.assertRaises(ValueError):
            aggregate_data([("0x1234", "0x")])
        with self.assertRaises(ValueError):
            aggregate_result("0x00", 1)
        # ABI (bool, bytes)[] containing one successful empty return.
        fixture = b"".join(word(v) for v in [32, 1, 32, 1, 64, 0])
        self.assertEqual(aggregate_result("0x" + fixture.hex(), 1), [{"success": True, "data": "0x"}])
        with self.assertRaises(ValueError):
            aggregate_result("0x" + fixture.hex(), 2)

    def test_failed_history_stops_after_three_retries_and_stays_missing(self):
        with tempfile.TemporaryDirectory() as directory:
            reader = Reader(Path(directory))
            with mock.patch("census.urllib.request.urlopen", side_effect=urllib.error.URLError("unavailable")) as transport, mock.patch("census.time.sleep"):
                with self.assertRaises(RPCFailure):
                    reader.rpc("eth_getLogs", [{"fromBlock": "0x1", "toBlock": "0x2"}])
                self.assertEqual(transport.call_count, 4)
                with self.assertRaises(RPCFailure):
                    reader.rpc("eth_getLogs", [{"fromBlock": "0x1", "toBlock": "0x2"}])
                self.assertEqual(transport.call_count, 4)
            self.assertEqual(len(reader.requests), 4)


if __name__ == "__main__":
    unittest.main()
