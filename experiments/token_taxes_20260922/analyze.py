"""Deliberately narrow, runtime-matched research models, NOT a generic auditor.

Models are manually reviewed complete implementations. ABI names or owner()==0
never select a model. Arbitrary source AST recognition is not implemented.
"""

from fractions import Fraction
import hashlib
import time


IMPL = "0x360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc"
ADMIN = "0xb53127684a568b3173ae13b9f8a6016e243e63b6e8ee1178d6a717850b5d6103"


def fraction_string(numerator, denominator):
    value = Fraction(numerator, denominator)
    if not 0 <= value <= 1:
        raise ValueError("failed to normalize rate: out of range")
    den = value.denominator
    for factor in (2, 5):
        while den % factor == 0:
            den //= factor
    if den != 1:
        raise ValueError("failed to normalize rate: nonterminating decimal")
    whole, rem = divmod(value.numerator, value.denominator)
    result = str(whole)
    if rem:
        result += "."
        while rem:
            digit, rem = divmod(rem * 10, value.denominator)
            result += str(digit)
    return result


def word(value):
    if isinstance(value, str):
        value = int(value, 16)
    return f"{value:064x}"


def fingerprint(code):
    return hashlib.sha256(bytes.fromhex(code.removeprefix("0x"))).hexdigest()


class Analyzer:
    def __init__(self, reader, models):
        self.reader = reader
        self.models = models

    def inspect(self, token, pool, venue, block, implementation=None, depth=0):
        try:
            code = self.reader.state("eth_getCode", [implementation or token], block)
        except ValueError:
            return {"taxes": None, "reason": "code_read_failed"}
        model = self.models.get(fingerprint(code))
        if model is None:
            return {"taxes": None, "reason": "unrecognized_runtime"}
        kind = model["kind"]
        values = {"buyRate": None, "sellRate": None,
                  "canChange": None, "hasExemptions": None}
        reasons = []

        def read(signature, *args):
            selector = model["selectors"][signature]
            data = "0x" + selector + "".join(word(x) for x in args)
            try:
                raw = self.reader.state("eth_call", [{"to": token, "data": data}], block)
                if len(raw) != 66:
                    raise ValueError("failed to decode word: invalid length")
                return int(raw, 16)
            except ValueError:
                reasons.append("state_read_failed:" + signature)
                return None

        if kind == "plain":
            values.update(buyRate="0", sellRate="0", canChange=False, hasExemptions=False)
        elif kind == "pool_tax":
            controller = read("controller()")
            values["canChange"] = controller != 0 if controller is not None else None
            values["hasExemptions"] = True
            if venue not in {"uniswap-v3", "aerodrome"} or len(pool) != 42:
                reasons.append("pool_transfer_context_unresolved")
            else:
                enabled = read("taxedPools(address)", pool)
                if enabled == 0:
                    values.update(buyRate="0", sellRate="0")
                elif enabled == 1:
                    for field, signature in (("buyRate", "buyBps()"), ("sellRate", "sellBps()")):
                        amount = read(signature)
                        if amount is not None:
                            try:
                                values[field] = fraction_string(amount, 10000)
                            except ValueError:
                                reasons.append("invalid_rate")
                else:
                    reasons.append("pool_applicability_unresolved")
        elif kind == "complex":
            values.update(canChange=False, hasExemptions=False)
            reasons.append("non_proportional_tax")
        elif kind == "proxy":
            if depth >= 2:
                return {"taxes": None, "reason": "delegation_depth_exceeded"}
            try:
                target = self.reader.state("eth_getStorageAt", [token, IMPL], block)
                admin = self.reader.state("eth_getStorageAt", [token, ADMIN], block)
                if len(target) != 66 or len(admin) != 66 or int(target, 16) >= 2**160 or int(admin, 16) >= 2**160:
                    raise ValueError("failed to decode proxy state: invalid")
                nested = self.inspect(token, pool, venue, block,
                                      "0x" + target[-40:], depth + 1)
                if nested["taxes"] is not None:
                    values.update({k: nested["taxes"][k] for k in values})
                if int(admin, 16) != 0:
                    values["canChange"] = True
                reasons.append("implementation:" + nested["reason"])
            except ValueError:
                reasons.append("proxy_state_read_failed")
        else:
            return {"taxes": None, "reason": "unsupported_model"}
        if all(v is None for v in values.values()):
            return {"taxes": None, "reason": ";".join(reasons)}
        values.update(source="contract_analysis", observedAt=time.time_ns() // 1000,
                      position={"kind": "block", "number": str(int(block["number"], 16)),
                                "id": block["hash"]})
        return {"taxes": values, "reason": ";".join(reasons) or "matched_reviewed_runtime",
                "model": model["name"], "runtimeSHA256": fingerprint(code)}


def fixture_models(compiled):
    # Reviewed model selection is independent of expected scenario outcomes.
    kinds = {"Plain": "plain", "PoolTax": "pool_tax", "RoleTax": "pool_tax",
             "ComplexTax": "complex", "StandardProxy": "proxy"}
    reviewed = {
        "Plain": "27a2e9c2544b878d1e0a879d4454554294bfe65e260ba25584d912ebd4b86411",
        "PoolTax": "3d3f2b687880f8357499cd96c48b48f02e5346bcb6da2f5a7a5b1275e071f192",
        "RoleTax": "568406bbeb76239ea796369e13f4311db8222f89f2e79ca152984e99deb07d11",
        "ComplexTax": "1045a4a0781634f4594d5335e5ec4d2744376e75404f5e2d305897098664f51f",
        "StandardProxy": "6e44939e799e8746ad7e9578810035a6b9d7068b5d256ad43c94488a6293db2b",
    }
    for name, expected in reviewed.items():
        if fingerprint(compiled["contracts"][name]["evm"]["deployedBytecode"]["object"]) != expected:
            raise ValueError("failed to load models: compiled runtime has not been reviewed")
    return {fingerprint(c["evm"]["deployedBytecode"]["object"]): {
        "kind": kinds[name], "name": name, "selectors": c["evm"]["methodIdentifiers"],
    } for name, c in compiled["contracts"].items() if name in kinds}
