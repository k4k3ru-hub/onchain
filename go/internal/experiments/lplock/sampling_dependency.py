"""Retrieve one observed code dependency at the sample block for source review."""
import argparse
import hashlib
import json
from pathlib import Path
import sys


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--legacy-dir', type=Path, required=True)
    parser.add_argument('--address', required=True)
    parser.add_argument('--block', type=int, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    sys.path.insert(0, str(args.legacy_dir.resolve()))
    from probe import Reader
    reader = Reader(args.output)
    result = {'address': args.address, 'block_number': args.block}
    try:
        code = reader.rpc('https://mainnet.base.org', 'eth_getCode', [args.address, hex(args.block)])
        source = reader.request('https://sourcify.dev/server/v2/contract/8453/' + args.address + '?fields=all')
        result['runtime_matches_rpc'] = source['runtimeBytecode']['onchainBytecode'].lower() == code.lower()
        result['compilation'] = source['compilation']
        reader.save('dependency-source-' + args.address, source)
        result['source_file_sha256'] = hashlib.sha256((args.output / ('dependency-source-' + args.address + '.json')).read_bytes()).hexdigest()
    except RuntimeError as exc:
        result['reason'] = str(exc)
    result['requests'] = reader.requests
    reader.save('dependency-' + args.address, result)
    print(json.dumps(result))


if __name__ == '__main__':
    main()
