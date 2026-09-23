"""Prepare a fixed historical V4 sample; no network access or production imports."""

import gzip
import json
from pathlib import Path
import subprocess

HERE = Path(__file__).resolve().parent
MANAGER = "0x7c5f5a4bbd8fd63184577525326123b519429bdc"
CORE = "0x498581ff718922c3f8e6a244956af099b2652b2b"
VIEW = "0xa3c0c9b65bad0b08107aa264b0f3db444b867a71"
FACTORY = "0x815542e8b392389a1389e22e588e4b62a67ade72"
LOCKER = "0xcd1680d26922fcd9cabfbb8a56ba40c333fd842a"
POOL = "0x2f20eec5945b32624fb6ccb8ba8716aabea99bc7d8e8590b5a863c283042601a"
TOKEN_ID = 3073314
OBSERVATION = 51604306
MULTICALL = "0xca11bde05977b3631167028862be2a173976ca11"


def read(path):
    if path.suffix == ".gz":
        return json.loads(gzip.decompress(path.read_bytes()))
    return json.loads(path.read_text())


def write(name, data):
    (HERE / name).write_text(json.dumps(data, indent=2) + "\n")


def keccak(data):
    return subprocess.run(["/private/tmp/lp-v4-keccak"], input=data.hex() + "\n",
                          text=True, capture_output=True, check=True).stdout.strip()


def word(value):
    if isinstance(value, str):
        value = int(value, 16)
    return (value % (1 << 256)).to_bytes(32, "big")


def call(signature, *args):
    return "0x" + keccak(signature.encode())[:8] + b"".join(map(word, args)).hex()


def aggregate(calls):
    parts = []
    for to, data in calls:
        data = bytes.fromhex(data[2:])
        parts.append(word(to) + word(1) + word(96) + word(len(data)) + data + bytes(-len(data) % 32))
    offset = 32 * len(parts)
    offsets = []
    for part in parts:
        offsets.append(word(offset))
        offset += len(part)
    return "0x82ad56cb" + (word(32) + word(len(parts)) + b"".join(offsets + parts)).hex()


def main():
    prior = HERE.parent / "newpair_quality_lateststate_20260921" / "evidence"
    state = read(prior / "state.json.gz")
    pool = next(p for p in state["pools"] if p["pool"] == POOL)
    position = next(p for p in read(prior / "positions.json")["positions"] if p["pool"] == POOL)
    ticks = next(p for p in read(prior / "ticks.json")["pools"] if p["pool"] == POOL)
    write("sample.json", {"anchor": state["anchor"], "pool": pool, "position": position, "ticks": ticks})
    plan = read(HERE / "discovery-plan.json")
    birth = read(HERE / "discovery-rpc.json")["results"]["birth_header"]["result"]
    observation = {"blockHash": state["anchor"]["hash"], "requireCanonical": True}
    creation = {"blockHash": birth["hash"], "requireCanonical": True}

    def add(name, method, params):
        plan.append({"name": name, "method": method, "params": params})

    event = pool["creation_event"]
    add("pool_header", "eth_getBlockByNumber", [event["blockNumber"], False])
    add("pool_receipt", "eth_getTransactionReceipt", [event["transactionHash"]])
    for name, address in [("factory", FACTORY), ("locker", LOCKER)]:
        add(name + "_birth_code", "eth_getCode", [address, creation])
        add(name + "_code", "eth_getCode", [address, observation])
    add("manager_code", "eth_getCode", [MANAGER, observation])
    reads = [
        (MANAGER, call("getPoolAndPositionInfo(uint256)", TOKEN_ID)),
        (MANAGER, call("getPositionLiquidity(uint256)", TOKEN_ID)),
        (MANAGER, call("ownerOf(uint256)", TOKEN_ID)),
        (MANAGER, call("getApproved(uint256)", TOKEN_ID)),
        (VIEW, call("getPositionInfo(bytes32,address,int24,int24,bytes32)", POOL, MANAGER,
                    position["lower"], position["upper"], TOKEN_ID)),
        (VIEW, call("getSlot0(bytes32)", POOL)),
        (VIEW, call("getLiquidity(bytes32)", POOL)),
        (LOCKER, call("positionManager()")), (LOCKER, call("factory()")),
        (MANAGER, call("poolManager()")), (VIEW, call("poolManager()")),
    ]
    bitmap = [(VIEW, call("getTickBitmap(bytes32,int16)", POOL, n)) for n in range(-18, 18)]
    tick_reads = [(VIEW, call("getTickLiquidity(bytes32,int24)", POOL, t["tick"])) for t in ticks["ticks"]]
    write("reads.json", {"state": reads, "bitmap": bitmap, "ticks": tick_reads})
    for name, group in [("state", reads), ("bitmap", bitmap), ("ticks", tick_reads)]:
        add(name, "eth_call", [{"to": MULTICALL, "data": aggregate(group)}, observation])
    for name, block in [("birth", birth["number"]), ("pool", event["blockNumber"]), ("observation", hex(OBSERVATION))]:
        add(name + "_header_final", "eth_getBlockByNumber", [block, False])
    write("cold-plan.json", plan)
    # Same observation with immutable creation evidence cached; mutable evidence is fetched again.
    warm_names = {"chain", "observation_header", "factory_code", "locker_code", "manager_code",
                  "state", "bitmap", "ticks", "observation_header_final"}
    write("warm-plan.json", [p for p in plan if p["name"] in warm_names])
    print("cold RPC", len(plan), "warm RPC", len(warm_names), "nested reads", len(reads) + len(bitmap) + len(tick_reads))


if __name__ == "__main__":
    main()
