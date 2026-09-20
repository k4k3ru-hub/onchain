"""Read fixed-width zero-argument getters discovered from a verified source ABI.

This records observations only; ABI names never establish withdrawal semantics.
"""
import argparse
import json
from pathlib import Path
import sys
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--legacy-dir', type=Path, required=True)
    parser.add_argument('--custody', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    sys.path.insert(0, str(args.legacy_dir.resolve()))
    from probe import Reader, keccak
    reader = Reader(args.output)
    custody = json.loads(args.custody.read_text())
    result = {'venue': custody['venue'], 'block_number': custody['block_number'], 'custodians': []}
    for custodian in custody['custodians']:
        source_info = custodian.get('source', {})
        if not source_info.get('runtime_matches_rpc'):
            continue
        source = json.loads((args.output / (custody['venue'] + '-source-' + source_info['address'] + '.json')).read_text())
        record = {'address': custodian['address'], 'getters': []}
        result['custodians'].append(record)
        for abi in source['abi']:
            if abi['type'] != 'function' or abi.get('inputs') or abi['stateMutability'] not in ['view', 'pure']:
                continue
            outputs = abi.get('outputs', [])
            if len(outputs) != 1 or not outputs[0]['type'].startswith(('address', 'uint', 'int', 'bool', 'bytes32')):
                continue
            signature = abi['name'] + '()'
            item = {'signature': signature, 'type': outputs[0]['type']}
            for attempt in range(4):
                time.sleep(2)
                try:
                    value = reader.rpc('https://mainnet.base.org', 'eth_call', [{'to': custodian['address'], 'data': '0x' + keccak(signature.encode())[:8]}, hex(custody['block_number'])])
                    item['raw'] = value
                    if len(bytes.fromhex(value[2:])) == 32:
                        item['value'] = '0x' + value[-40:] if outputs[0]['type'] == 'address' else str(int(value, 16))
                    break
                except RuntimeError as exc:
                    if reader.requests[-1].get('http_status') == 429 and attempt < 3:
                        time.sleep(3)
                        continue
                    item['reason'] = str(exc)
                    break
            record['getters'].append(item)
            result['requests'] = reader.requests
            reader.save(custody['venue'] + '-getters', result)
    result['requests'] = reader.requests
    reader.save(custody['venue'] + '-getters', result)
    print(json.dumps({'venue': custody['venue'], 'custodians': len(result['custodians']), 'http_attempts': len(reader.requests)}))


if __name__ == '__main__':
    main()
