"""Compare alternate compiler outputs; metadata-prefix equality is diagnostic only."""

import hashlib
import json

from probe import CACHE, ROOT, invoke, save


def metadata_prefix(code):
    # Decode the definite CBOR trailer before splitting. This is not a trust rule.
    size = int.from_bytes(code[-2:], "big") + 2
    if size >= len(code) or size <= 2:
        return None
    data = code[-size:-2]
    cursor = 0

    def read(depth=0):
        nonlocal cursor
        if depth > 4 or cursor >= len(data):
            raise ValueError("invalid metadata")
        head = data[cursor]
        cursor += 1
        major, value = head >> 5, head & 31
        if value >= 24:
            width = {24: 1, 25: 2, 26: 4, 27: 8}.get(value)
            if width is None or cursor + width > len(data):
                raise ValueError("invalid metadata size")
            value = int.from_bytes(data[cursor:cursor + width], "big")
            cursor += width
        if major == 0:
            return value
        if major in (2, 3):
            if cursor + value > len(data):
                raise ValueError("invalid metadata bytes")
            result = data[cursor:cursor + value]
            cursor += value
            return result.decode() if major == 3 else result
        if major == 5 and value <= 16:
            result = {}
            for _ in range(value):
                key = read(depth + 1)
                if not isinstance(key, str) or key in result:
                    raise ValueError("invalid metadata key")
                result[key] = read(depth + 1)
            return result
        if major == 7 and value in (20, 21):
            return value == 21
        raise ValueError("unsupported metadata encoding")

    try:
        metadata = read()
    except (ValueError, UnicodeError):
        return None
    if cursor != len(data) or not isinstance(metadata, dict) or not isinstance(metadata.get("solc"), bytes) or len(metadata["solc"]) != 3:
        return None
    return code[:-size]


def main():
    cases = json.loads((ROOT / "evidence/cases.json").read_text())
    previous = ROOT.parent / "token_taxes_20260922/evidence"
    compiled = json.loads((previous / "live-compiled.json").read_text())
    live = json.loads((previous / "live.json").read_text())
    rows = []
    for name, version in (("TAOT", "0.8.37"), ("ShinyLIMPET", "0.8.34"), ("WETH9", "0.8.37")):
        original = next(c for c in cases if c["name"] == name)
        case = dict(original, version=version)
        inp = (CACHE / (name + ".json")).read_bytes()
        result = invoke(case, "native-arm64", inp, compiler_options=["--no-import-callback"])
        output = result.pop("output", {})
        errors = [e for e in output.get("errors", []) if e.get("severity") == "error"]
        result.update(originalCompiler=original["compiler"], attemptedCompiler=version,
                      compilerErrors=len(errors), compilerErrorKinds=[e.get("type") for e in errors],
                      compilerErrorMessages=[e.get("message", "")[:300] for e in errors],
                      adoptedTaxObservation=False)
        if "contracts" in output and not errors:
            file, contract = case["qualifiedName"].rsplit(":", 1)
            code = output["contracts"][file][contract]["evm"]["deployedBytecode"]
            old = compiled[case["address"]]["artifact"]["evm"]["deployedBytecode"]
            new_bytes, old_bytes = bytes.fromhex(code["object"]), bytes.fromhex(old["object"])
            actual = bytes.fromhex(live["tokens"][case["address"]]["code"].removeprefix("0x"))
            result.update(runtimeBytes=len(new_bytes), originalRuntimeBytes=len(old_bytes),
                          matchesOriginalCompiledRuntime=new_bytes == old_bytes,
                          matchesObservedRuntime=new_bytes == actual,
                          runtimeSHA256=hashlib.sha256(new_bytes).hexdigest(),
                          immutableLayoutEqual=code.get("immutableReferences", {}) == old.get("immutableReferences", {}),
                          hasImmutables=bool(code.get("immutableReferences")))
            new_prefix, old_prefix, actual_prefix = (metadata_prefix(b) for b in (new_bytes, old_bytes, actual))
            result["metadataBoundariesLocatedDiagnosticOnly"] = all(p is not None for p in (new_prefix, old_prefix, actual_prefix))
            if new_prefix is not None and old_prefix is not None:
                result["templatePrefixDifferingBytes"] = sum(a != b for a, b in zip(new_prefix, old_prefix)) + abs(len(new_prefix) - len(old_prefix))
                result["templatePrefixBytes"] = len(new_prefix)
            result["templatePrefixEqualDiagnosticOnly"] = new_prefix is not None and new_prefix == old_prefix
            result["observedPrefixEqualDiagnosticOnly"] = new_prefix is not None and new_prefix == actual_prefix
            result["metadataDifferenceOnlyAgainstObserved"] = bool(new_prefix is not None and new_prefix == actual_prefix and new_bytes != actual)
        rows.append(result)
        print(json.dumps(result), flush=True)
    assert len(rows) == 3
    assert rows[0]["compilerErrors"] == 0 and not rows[0]["matchesObservedRuntime"]
    assert rows[2]["compilerErrors"] > 0
    save(ROOT / "evidence/cross-version.json", rows)


if __name__ == "__main__":
    main()
