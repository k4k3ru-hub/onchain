"""Deploy fixtures and compare analysis against real local EVM transfers."""

import json
from pathlib import Path
import time
import urllib.request

from acquire import Reader, save
from analyze import Analyzer, fixture_models, word


URL = "http://127.0.0.1:19545"


class Local:
    def __init__(self):
        self.ledger = []

    def rpc(self, method, params):
        with urllib.request.urlopen(urllib.request.Request(URL, json.dumps({
            "jsonrpc": "2.0", "id": 1, "method": method, "params": params,
        }).encode(), {"Content-Type": "application/json"}), timeout=15) as response:
            result = json.load(response)
        self.ledger.append({"method": method, "params": params, "response": result})
        if "error" in result:
            raise ValueError("failed to execute local rpc: " + json.dumps(result["error"]))
        return result["result"]

    def tx(self, sender, data, target=None):
        tx = {"from": sender, "data": data, "gas": hex(8_000_000)}
        if target:
            tx["to"] = target
        txid = self.rpc("eth_sendTransaction", [tx])
        for _ in range(30):
            receipt = self.rpc("eth_getTransactionReceipt", [txid])
            if receipt is not None:
                assert receipt["status"] == "0x1", receipt
                return receipt
            time.sleep(0.1)
        raise ValueError("failed to confirm local transaction: timed out")


def main():
    out = Path("evidence")
    compiled = json.loads((out / "compiled.json").read_text())
    contracts = compiled["contracts"]
    local = Local()
    admin, pool, user, exempt, other_pool = local.rpc("eth_accounts", [])[:5]

    def calldata(name, signature, *args):
        return "0x" + contracts[name]["evm"]["methodIdentifiers"][signature] + "".join(word(a) for a in args)

    addresses = {}
    for name in ("Plain", "PoolTax", "RoleTax", "FakeZero", "ComplexTax", "StandardProxy", "UnknownProxy"):
        bytecode = contracts[name]["evm"]["bytecode"]["object"]
        if "Proxy" in name:
            bytecode += word(addresses["Plain"])
        addresses[name] = local.tx(admin, "0x" + bytecode)["contractAddress"]
    for name in ("PoolTax", "RoleTax"):
        local.tx(admin, calldata(name, "setPool(address,bool)", pool, 1), addresses[name])
    local.tx(admin, calldata("PoolTax", "setExempt(address,bool)", exempt, 1), addresses["PoolTax"])

    cases = []
    models = fixture_models(compiled)
    for name in addresses:
        reader = Reader(URL, budget=50, delay=0)
        block = reader.pin("latest")
        result = Analyzer(reader, models).inspect(addresses[name], pool, "uniswap-v3", block)
        assert reader.pin(block["number"])["hash"] == block["hash"]
        cases.append({"case": name, "address": addresses[name], "analysis": result,
                      "rpc": reader.stats(), "ledger": reader.ledger})

    # Compare direct transfers, not a synthetic 'tax getter' expectation.
    transfers = []
    for name in ("Plain", "PoolTax", "RoleTax", "FakeZero", "ComplexTax"):
        target = addresses[name]

        def balance(account):
            return int(local.rpc("eth_call", [{"to": target, "data": calldata(name, "balanceOf(address)", account)}, "latest"]), 16)

        local.tx(admin, calldata(name, "transfer(address,uint256)", pool, 1_000_000), target)
        local.tx(admin, calldata(name, "transfer(address,uint256)", user, 100_000), target)
        for label, sender, recipient in (("buy", pool, user), ("sell", user, pool)):
            before = balance(recipient)
            local.tx(sender, calldata(name, "transfer(address,uint256)", recipient, 10000), target)
            received = balance(recipient) - before
            transfers.append({"case": name, "direction": label, "sent": 10000, "received": received,
                              "deducted": 10000 - received})
            local.tx(sender, calldata(name, "approve(address,uint256)", admin, 10000), target)
            before = balance(recipient)
            local.tx(admin, calldata(name, "transferFrom(address,address,uint256)", sender, recipient, 10000), target)
            received = balance(recipient) - before
            transfers.append({"case": name, "direction": label, "method": "transferFrom", "sent": 10000,
                              "received": received, "deducted": 10000 - received})
        if name == "PoolTax":
            before = balance(exempt)
            local.tx(pool, calldata(name, "transfer(address,uint256)", exempt, 10000), target)
            transfers.append({"case": name, "direction": "exempt_buy", "sent": 10000,
                              "received": balance(exempt)-before, "deducted": 10000-(balance(exempt)-before)})
        if name == "ComplexTax":
            before = balance(user)
            local.tx(pool, calldata(name, "transfer(address,uint256)", user, 100), target)
            transfers.append({"case": name, "direction": "small_buy", "sent": 100,
                              "received": balance(user)-before, "deducted": 100-(balance(user)-before)})

    # Same token, same block, new block, other pool, V4 ID: meter each separately.
    reader = Reader(URL, budget=80, delay=0)
    analyzer = Analyzer(reader, models)
    costs = []

    def evaluate(label, block_number, venue="uniswap-v3", pool_id=pool):
        before = reader.stats()
        block = reader.pin(block_number)
        result = analyzer.inspect(addresses["PoolTax"], pool_id, venue, block)
        assert reader.pin(block["number"])["hash"] == block["hash"]
        after = reader.stats()
        costs.append({"case": label, "analysis": result,
                      "rpc": {k: after[k]-before[k] for k in ("logical", "httpAttempts", "cacheHits", "failedAttempts")}})
        return block

    first = evaluate("cold", "latest")
    evaluate("warm", first["number"])
    evaluate("other_pool", first["number"], pool_id=other_pool)
    evaluate("v4_pool_id", first["number"], venue="uniswap-v4", pool_id="0x" + "ab" * 32)
    local.tx(admin, calldata("PoolTax", "setRates(uint256,uint256)", 0, 0), addresses["PoolTax"])
    evaluate("new_block_current_zero", "latest")

    # Independently observe misleading getters and reachable change paths.
    def getter(name, signature, *args):
        return local.rpc("eth_call", [{"to": addresses[name], "data": calldata(name, signature, *args)}, "latest"])

    controls = {"fakeBuyTax": getter("FakeZero", "buyTax()"),
                "fakeSellTax": getter("FakeZero", "sellTax()"),
                "roleOwner": getter("RoleTax", "owner()"),
                "roleController": getter("RoleTax", "controller()"),
                "roleEmptyExemption": getter("RoleTax", "exempt(address)", user)}
    local.tx(admin, calldata("RoleTax", "setRates(uint256,uint256)", 300, 700), addresses["RoleTax"])
    controls["roleChangedBuyBps"] = getter("RoleTax", "buyBps()")
    controls["roleChangedSellBps"] = getter("RoleTax", "sellBps()")
    local.tx(admin, calldata("StandardProxy", "upgradeTo(address)", addresses["FakeZero"]), addresses["StandardProxy"])
    from analyze import IMPL
    controls["proxyImplementationAfterUpgrade"] = local.rpc("eth_getStorageAt", [addresses["StandardProxy"], IMPL, "latest"])
    controls["expectedProxyImplementation"] = addresses["FakeZero"]

    results = {"cases": cases, "transfers": transfers, "costs": costs, "costLedger": reader.ledger,
               "controls": controls}
    save(out / "local-results.json", results)
    save(out / "local-transactions.json", local.ledger)
    for case in cases:
        print(case["case"], json.dumps(case["analysis"]), "RPC", case["rpc"]["httpAttempts"])
    print("transfers", transfers)
    print("costs", [(c["case"], c["rpc"]) for c in costs])


if __name__ == "__main__":
    main()
