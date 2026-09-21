import unittest

from sampling_custody import can_report_unlocked, code_structure
from sampling_plan_probe import matches_expected
from sampling_runtime_review import match_runtime


class SamplingSafetyTests(unittest.TestCase):
    def test_runtime_comparison_allows_only_consistent_compiler_immutables(self):
        compiled = {'object': '60000000550000', 'immutableReferences': {'7': [{'start': 2, 'length': 2}, {'start': 5, 'length': 2}]}, 'linkReferences': {}}
        self.assertEqual(match_runtime(compiled, '0x60001234551234'), {'7': '1234'})
        self.assertIsNone(match_runtime(compiled, '0x60001234555678'))
        self.assertIsNone(match_runtime(compiled, '0x60001234561234'))
        self.assertIsNone(match_runtime(compiled, '0x6000123455123400'))
        self.assertIsNone(match_runtime({**compiled, 'linkReferences': {'library': {}}}, '0x60001234551234'))

    def test_provider_errors_are_not_withdrawal_reverts(self):
        expected = {'revert_data': '0x12345678'}
        self.assertTrue(matches_expected(expected, {'error': {'code': 3, 'data': '0x12345678'}}))
        self.assertFalse(matches_expected(expected, {'error': {'code': -32000, 'data': '0x12345678'}}))
        self.assertFalse(matches_expected(expected, {'error': {'code': 3, 'data': '0x99999999'}}))
        self.assertFalse(matches_expected({'result': '0x00'}, {'result': '0x00', 'error': {'code': 3}}))

    def test_delegation_requires_exact_indicator(self):
        target = '12' * 20
        self.assertEqual(code_structure('0xef0100' + target), {'kind': 'eip7702', 'target': '0x' + target})
        for code in ['0xef0101' + target, '0xef0100' + target[:-2], '0xef0100' + target + '00']:
            self.assertEqual(code_structure(code)['kind'], 'unrecognized_runtime')

    def test_clone_requires_executable_suffix(self):
        code = '363d3d373d3d3d363d73' + '34' * 20 + '5af43d82803e903d91602b57fd5bf3'
        self.assertEqual(code_structure('0x' + code)['target'], '0x' + '34' * 20)
        self.assertEqual(code_structure('0x' + code[:-2] + '00')['kind'], 'unrecognized_runtime')

    def test_unknown_custody_cannot_become_zero(self):
        row = {'complete_position_coverage': True, 'principal_token0': '100', 'principal_token1': '0'}
        positions = [{'withdrawal_assessment': 'owner_full_decrease_succeeded'}]
        self.assertTrue(can_report_unlocked(row, positions))
        self.assertFalse(can_report_unlocked(row, positions + [{'withdrawal_assessment': 'unresolved'}]))
        self.assertFalse(can_report_unlocked(row, [{'withdrawal_assessment': 'owner_decrease_reverted'}]))

    def test_empty_or_incomplete_principal_cannot_become_zero(self):
        positions = [{'withdrawal_assessment': 'eip7702_owner_full_decrease_succeeded'}]
        self.assertFalse(can_report_unlocked({'complete_position_coverage': False, 'principal_token0': '100'}, positions))
        self.assertFalse(can_report_unlocked({'complete_position_coverage': True, 'principal_token0': '0', 'principal_token1': '0'}, positions))
        self.assertFalse(can_report_unlocked({'complete_position_coverage': True, 'principal_token0': '100'}, []))


if __name__ == '__main__':
    unittest.main()
