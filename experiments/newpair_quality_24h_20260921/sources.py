"""Fetch public verified source for contracts discovered from pool evidence."""
import argparse
import json
from pathlib import Path
import time
import urllib.error
import urllib.request

from census import END_BLOCK, Reader, write


def get(output, url):
    ledger_file = output / "get-requests.json"
    ledger = json.loads(ledger_file.read_text()) if ledger_file.exists() else []
    for attempt in range(4):
        entry = {"url": url, "attempt": attempt + 1}
        try:
            with urllib.request.urlopen(urllib.request.Request(url, headers={"User-Agent": "k4k3ru-read-only-census/1"}), timeout=40) as response:
                data = json.load(response)
            entry["success"] = True
            return data
        except urllib.error.HTTPError as exc:
            entry["http_status"] = exc.code
        except (urllib.error.URLError, TimeoutError, ValueError):
            entry["failure"] = "source_request_failed"
        finally:
            ledger.append(entry)
            write(ledger_file, ledger)
        if attempt < 3:
            time.sleep(2 ** (attempt + 1))
    return None


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("addresses", nargs="+")
    args = parser.parse_args()
    reader = Reader(args.output)
    for address in args.addresses:
        path = args.output / (address + ".json")
        if path.exists():
            continue
        code = reader.rpc("eth_getCode", [address, hex(END_BLOCK)])
        source = get(args.output, "https://sourcify.dev/server/v2/contract/8453/" + address + "?fields=all")
        provider = "sourcify"
        if not source or not source.get("sources"):
            source = get(args.output, "https://base.blockscout.com/api/v2/smart-contracts/" + address)
            provider = "blockscout"
        row = {"address": address, "block": END_BLOCK, "code": code, "provider": provider, "source": source}
        if source and provider == "sourcify":
            row["published_runtime_matches"] = source["runtimeBytecode"]["onchainBytecode"].lower() == code.lower()
            row["contract_name"] = source["compilation"]["fullyQualifiedName"]
        elif source:
            row["published_runtime_matches"] = bool(source.get("is_verified") and source.get("source_code") and source.get("deployed_bytecode", "").lower() == code.lower())
            row["contract_name"] = source.get("name")
        write(path, row)
        print(json.dumps({k: v for k, v in row.items() if k not in ["code", "source"]}), flush=True)


if __name__ == "__main__":
    main()
