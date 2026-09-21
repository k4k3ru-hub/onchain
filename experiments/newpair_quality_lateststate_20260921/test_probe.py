"""Offline checks of full-range coverage and conservative missing-data handling."""

import tempfile
from pathlib import Path
import unittest
from unittest.mock import patch
import urllib.error

from probe import Reader, Unavailable, bitmap_ticks, bitmap_words, reconcile


class CoverageTests(unittest.TestCase):
    def test_full_position_including_out_of_range_is_covered(self):
        positions = [{"token_id": 1, "lower": -200, "upper": 200, "liquidity": "10"},
                     {"token_id": 2, "lower": 400, "upper": 600, "liquidity": "20"}]
        ticks = [{"tick": -200, "gross": "10", "net": "10"},
                 {"tick": 200, "gross": "10", "net": "-10"},
                 {"tick": 400, "gross": "20", "net": "20"},
                 {"tick": 600, "gross": "20", "net": "-20"}]
        proof = reconcile(ticks, positions, 0, 10)
        self.assertTrue(proof["all_gross_covered"])
        self.assertTrue(proof["all_net_covered"])
        # Matching active liquidity alone must not conceal another position.
        proof = reconcile(ticks, positions[:1], 0, 10)
        self.assertFalse(proof["all_gross_covered"])

    def test_same_boundaries_with_additional_owner_are_not_covered(self):
        positions = [{"token_id": 1, "lower": -200, "upper": 200, "liquidity": "10"}]
        ticks = [{"tick": -200, "gross": "15", "net": "15"},
                 {"tick": 200, "gross": "15", "net": "-15"}]
        self.assertFalse(reconcile(ticks, positions, 0, 15)["all_gross_covered"])

    def test_empty_pool_is_not_100_percent(self):
        self.assertFalse(reconcile([], [], 0, 0)["all_gross_covered"])

    def test_inconsistent_state_and_duplicate_positions_are_rejected(self):
        position = {"token_id": 1, "lower": -200, "upper": 200, "liquidity": "10"}
        with self.assertRaisesRegex(ValueError, "duplicate_position"):
            reconcile([], [position, position], 0, 0)
        with self.assertRaisesRegex(ValueError, "tick_active_liquidity_mismatch"):
            reconcile([{"tick": -200, "gross": "10", "net": "10"}], [position], 0, 10)

    def test_bitmap_scan_includes_negative_and_extreme_ticks(self):
        words = list(bitmap_words(200))
        self.assertEqual((words[0], words[-1], len(words)), (-18, 17, 36))
        self.assertEqual(bitmap_ticks(-1, 1 << 255, 200), [-200])
        self.assertEqual(bitmap_ticks(-18, 1 << 172, 200), [-887200])
        with self.assertRaisesRegex(ValueError, "bitmap_tick_out_of_range"):
            bitmap_ticks(-18, 1, 200)

    def test_failure_retries_three_times_then_preserves_failure(self):
        with tempfile.TemporaryDirectory() as temp:
            reader = Reader(Path(temp))
            error = urllib.error.HTTPError(reader.endpoint, 500, "server_error", {}, None)
            with patch("probe.urllib.request.urlopen", side_effect=error) as call, patch("probe.time.sleep"):
                with self.assertRaises(Unavailable):
                    reader.rpc("eth_call", [{}, "0x1"])
                self.assertEqual(call.call_count, 4)
                with self.assertRaises(Unavailable):
                    reader.rpc("eth_call", [{}, "0x1"])
                self.assertEqual(call.call_count, 4)


if __name__ == "__main__":
    unittest.main()
