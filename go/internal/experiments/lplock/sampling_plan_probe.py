"""Execute a source-review-generated list of read-only calls at a pinned block.

The plan carries its observation-derived selectors, addresses, expected results
and source hashes. This runner does not classify arbitrary contracts.
"""
import argparse
import json
from pathlib import Path
import time
import urllib.error
import urllib.request


def matches_expected(expected, response):
    if 'result' in expected:
        return 'error' not in response and response.get('result') == expected['result']
    error = response.get('error', {})
    return error.get('code') == 3 and error.get('data') == expected['revert_data']


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('plan', type=Path)
    parser.add_argument('output', type=Path)
    args = parser.parse_args()
    plan = json.loads(args.plan.read_text())
    evidence = {'block_number': plan['block_number'], 'block_hash': plan['block_hash'], 'source_sha256': plan['source_sha256'], 'probes': [], 'http_attempts': 0}
    entries = [{'name': 'initial_block_hash', 'method': 'eth_getBlockByNumber', 'params': [hex(plan['block_number']), False], 'call': {}, 'expected': {'block_hash': plan['block_hash']}}]
    entries += plan['calls']
    entries += [{'name': 'final_block_hash', 'method': 'eth_getBlockByNumber', 'params': [hex(plan['block_number']), False], 'call': {}, 'expected': {'block_hash': plan['block_hash']}}]
    for entry in entries:
        record = {'name': entry['name'], 'call': entry['call'], 'expected': entry['expected']}
        for attempt in range(4):
            time.sleep(2)
            payload = {'jsonrpc': '2.0', 'id': 1, 'method': entry.get('method', 'eth_call'), 'params': entry.get('params', [entry['call'], hex(plan['block_number'])])}
            evidence['http_attempts'] += 1
            request = urllib.request.Request('https://mainnet.base.org', data=json.dumps(payload).encode(), headers={'Content-Type': 'application/json', 'User-Agent': 'k4k3ru-read-only-probe/1'})
            try:
                with urllib.request.urlopen(request, timeout=30) as response:
                    result = json.load(response)
                record['result'] = result.get('result')
                error = result.get('error', {})
                record['error_code'] = error.get('code')
                record['error_data'] = error.get('data')
                expected = entry['expected']
                if 'block_hash' in expected:
                    record['matched'] = (record['result'] or {}).get('hash') == expected['block_hash']
                else:
                    record['matched'] = matches_expected(expected, result)
                break
            except urllib.error.HTTPError as exc:
                record['http_status'] = exc.code
                if exc.code != 429 or attempt == 3:
                    record['matched'] = False
                    break
                time.sleep(3)
            except urllib.error.URLError:
                record['failure'] = 'connection_failed'
                record['matched'] = False
                break
        evidence['probes'].append(record)
        args.output.write_text(json.dumps(evidence, indent=2) + '\n')
    print(json.dumps({'probes': len(evidence['probes']), 'matched': sum(p['matched'] for p in evidence['probes']), 'http_attempts': evidence['http_attempts']}))
    return 0 if all(p['matched'] for p in evidence['probes']) else 1


if __name__ == '__main__':
    raise SystemExit(main())
