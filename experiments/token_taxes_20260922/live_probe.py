"""Read-only Base replay and cold/warm/new-block metering of verified models."""

import json
from pathlib import Path
import time

from acquire import Reader, save
from analyze import Analyzer
from verify_live import verified_models


def main():
    live = json.loads(Path("evidence/live.json").read_text())
    compiled = json.loads(Path("evidence/live-compiled.json").read_text())
    inputs = json.loads(Path("evidence/inputs.json").read_text())
    models, matches = verified_models(live, compiled)
    save("evidence/live-code-matches.json", matches)
    reader = Reader("https://mainnet.base.org", budget=40)
    analyzer = Analyzer(reader, models)
    cases = []
    assert reader.rpc("eth_chainId", []) == hex(8453)

    def evaluate(label, number):
        start = time.monotonic()
        before = reader.stats()
        block = reader.pin(number)
        pairs = []
        for pair in inputs["pairs"]:
            taxes = {}
            details = {}
            for side in ("token0", "token1"):
                result = analyzer.inspect(pair[side]["id"], pair["poolId"], pair["venue"], block)
                taxes[side] = result["taxes"]
                details[side] = result
            pairs.append({"venue": pair["venue"], "poolId": pair["poolId"],
                          "tokenTaxes": taxes, "details": details})
        if reader.pin(block["number"])["hash"] != block["hash"]:
            raise ValueError("failed to confirm observation: block changed")
        after = reader.stats()
        cases.append({"case": label, "block": {k: block[k] for k in ("number", "hash", "timestamp")},
                      "pairs": pairs, "elapsedMs": round((time.monotonic()-start)*1000, 3),
                      "rpc": {k: after[k]-before[k] for k in ("logical", "httpAttempts", "cacheHits", "failedAttempts")}})

    try:
        evaluate("cold", live["block"]["number"])
        evaluate("warm_same_block", live["block"]["number"])
        evaluate("new_block", "latest")
    finally:
        save("evidence/live-results.json", {"cases": cases, "stats": reader.stats(), "ledger": reader.ledger})
    print(json.dumps({"cases": [{"case": c["case"], "rpc": c["rpc"], "elapsedMs": c["elapsedMs"]} for c in cases],
                      "stats": reader.stats()}, indent=2))


if __name__ == "__main__":
    main()
