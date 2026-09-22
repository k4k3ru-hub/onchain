"""Recompiled code matching for three manually reviewed, direct ERC-20s.

Only compiler-reported immutable ranges are substituted. Metadata relaxation is
limited to the reviewed WETH9 trailing CBOR, preceded by INVALID. The resulting
recognizer always uses the FULL observed runtime hash, never a masked hash.
"""

import hashlib
import json
from pathlib import Path

from acquire import save
from analyze import fingerprint


REVIEWED_SOURCES = {
    "c995cf1598c809dc511723a4625146266e477761cf347959297a1e8cbe03552a": "WETH9",
    "70dde694aab27e4761f2d70b9959e49ae70eaf432c83fd678581561db13473e7": "ShinyLIMPET",
    "d6453972fa826b639c988c621610b848575baa8ce87fc6745f188e7b2fcac83d": "TAOT",
}


def match_runtime(artifact, actual_hex, allow_metadata=False):
    code = artifact["evm"]["deployedBytecode"]
    compiled = bytearray.fromhex(code["object"])
    actual = bytes.fromhex(actual_hex.removeprefix("0x"))
    if code.get("linkReferences") or len(compiled) != len(actual):
        raise ValueError("failed to match runtime: length or library links differ")
    substitutions = []
    for identifier, ranges in code.get("immutableReferences", {}).items():
        for item in ranges:
            start, length = item["start"], item["length"]
            if start < 0 or length != 32 or start + length > len(compiled):
                raise ValueError("failed to match runtime: immutable range invalid")
            compiled[start:start+length] = actual[start:start+length]
            substitutions.append({"kind": "compiler_immutable", "id": identifier, **item})
    if allow_metadata:
        clen = int.from_bytes(compiled[-2:], "big") + 2
        alen = int.from_bytes(actual[-2:], "big") + 2
        if clen != alen or clen >= len(compiled) or compiled[-clen-1] != 0xFE or actual[-alen-1] != 0xFE:
            raise ValueError("failed to match runtime: metadata boundary differs")
        compiled[-clen:] = actual[-alen:]
        substitutions.append({"kind": "reviewed_trailing_metadata", "start": len(compiled)-clen, "length": clen})
    if bytes(compiled) != actual:
        raise ValueError("failed to match runtime: executable code differs")
    return substitutions


def verified_models(live, compiled):
    models = {}
    evidence = []
    for address, token in live["tokens"].items():
        if address not in compiled:
            evidence.append({"address": address, "matched": False, "reason": "proxy_implementation_not_analyzed"})
            continue
        source = token["source"]
        bundle = {k: v["content"] for k, v in source["sources"].items()}
        digest = hashlib.sha256(json.dumps(bundle, sort_keys=True).encode()).hexdigest()
        name = REVIEWED_SOURCES.get(digest)
        if name is None:
            raise ValueError("failed to verify source: bundle has not been reviewed")
        c = compiled[address]
        substitutions = match_runtime(c["artifact"], token["code"], name == "WETH9")
        code_hash = fingerprint(token["code"])
        models[code_hash] = {"kind": "plain", "name": name, "selectors": {}}
        evidence.append({"address": address, "matched": True, "model": name,
                         "sourceBundleSHA256": digest, "runtimeSHA256": code_hash,
                         "compiler": c["compiler"], "compilerSHA256": c["compilerSHA256"],
                         "substitutions": substitutions})
    return models, evidence


if __name__ == "__main__":
    live = json.loads(Path("evidence/live.json").read_text())
    compiled = json.loads(Path("evidence/live-compiled.json").read_text())
    models, evidence = verified_models(live, compiled)
    save("evidence/live-code-matches.json", evidence)
    print(json.dumps(evidence, indent=2))
