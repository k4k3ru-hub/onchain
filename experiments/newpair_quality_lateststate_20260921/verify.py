"""Recheck retained state coverage and hash the evidence without network access."""

import hashlib
from pathlib import Path

from probe import HERE, PRIOR, bitmap_ticks, bitmap_words, read, reconcile, signed, words, write


def verify(root):
    census = {p["pool"]: p for p in read(PRIOR / "evidence" / "census.json.gz")["pools"]}
    for directory in [root, root / "historical_control"]:
        data = read(directory / "ticks.json")
        positions = read(directory / "positions.json")["positions"]
        states = {p["pool"]: p for p in read(directory / "state.json.gz")["pools"]}
        for row in data["pools"]:
            pool = row["pool"]
            spacing = census[pool]["tick_spacing"]
            assert [r["word"] for r in row["bitmap_words"]] == list(bitmap_words(spacing))
            initialized = []
            for bitmap in row["bitmap_words"]:
                assert bitmap["result"]["success"]
                assert int(bitmap["result"]["data"], 16) == int(bitmap["value"])
                initialized.extend(bitmap_ticks(bitmap["word"], int(bitmap["value"]), spacing))
            assert initialized == [t["tick"] for t in row["ticks"]]
            for tick in row["ticks"]:
                assert tick["result"]["success"]
                raw = words(tick["result"]["data"])
                assert int(raw[0], 16) == int(tick["gross"])
                assert signed(raw[1]) == int(tick["net"])
            current = states[pool]
            proof = reconcile(row["ticks"], [p for p in positions if p["pool"] == pool and p["protected"]],
                              current["tick"], int(current["active_liquidity"]))
            assert proof == row["proof"]
            assert proof["all_gross_covered"] and proof["all_net_covered"]
            assert row["locked_liquidity_percentage"] == "100"
        for path in (directory / "rpc").glob("*.json.gz"):
            record = read(path)
            payload = record["payload"]
            assert payload["method"] in ["eth_call", "eth_getCode", "eth_getBlockByNumber"]
            if payload["method"] in ["eth_call", "eth_getCode"]:
                assert payload["params"][-1] == hex(data["anchor"]["block"])
        print(directory.name, "coverage_reproduced", len(data["pools"]))
    artifacts = []
    for path in sorted(root.rglob("*")):
        if not path.is_file() or path.name == "artifact-index.json":
            continue
        content = path.read_bytes()
        artifacts.append({"path": str(path.relative_to(root)), "bytes": len(content),
                          "sha256": hashlib.sha256(content).hexdigest()})
    write(root / "artifact-index.json", {"files": artifacts})
    print("artifacts_hashed", len(artifacts), "bytes", sum(r["bytes"] for r in artifacts))


if __name__ == "__main__":
    verify(HERE / "evidence")
