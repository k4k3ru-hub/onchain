"""Reproduce compiler inputs/artifacts using archived public source (offline)."""

import argparse
import gzip
import hashlib
import json
from pathlib import Path

HERE = Path(__file__).resolve().parent
COMPILER = "0.8.26+commit.8a97fa7a"
COMPILER_SHA256 = "d5f23436f443edb85d8e76906d12f0a86ce0490e7663a9e608efeb7a93f149ef"


def sources():
    factory = json.loads(gzip.decompress((HERE / "factory-source.json.gz").read_bytes()))
    prior = HERE.parent / "newpair_quality_24h_20260921" / "evidence" / "sources"
    manager = json.loads(gzip.decompress((prior / "0x7c5f5a4bbd8fd63184577525326123b519429bdc.json.gz").read_bytes()))["source"]
    return {"factory": factory, "manager": manager}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["prepare", "verify"])
    parser.add_argument("directory", type=Path)
    args = parser.parse_args()
    group = {"factory": ["LaunchFactory", "LaunchLocker"], "manager": ["PositionManager"]}
    provenance = {"compiler": COMPILER, "compilerSHA256": COMPILER_SHA256, "contracts": {}}
    for label, source in sources().items():
        if source["compilation"]["compilerVersion"] != COMPILER:
            raise ValueError("compiler version mismatch")
        if args.action == "prepare":
            data = source["stdJsonInput"]
            data["settings"]["outputSelection"] = {"src/" + name + ".sol": {name: ["abi", "evm.bytecode.object", "evm.deployedBytecode.object", "evm.deployedBytecode.immutableReferences"]} for name in group[label]}
            (args.directory / (label + "-input.json")).write_text(json.dumps(data))
            continue
        output = json.loads((args.directory / (label + "-output.json")).read_text())
        if any(e["severity"] == "error" for e in output.get("errors", [])):
            raise ValueError("compiler error")
        for name in group[label]:
            contract = output["contracts"]["src/" + name + ".sol"][name]
            prior = source["stdJsonOutput"]["contracts"].get("src/" + name + ".sol", {}).get(name)
            if prior is None:
                path = HERE.parent / "newpair_quality_24h_20260921" / "evidence" / "sources" / "0xcd1680d26922fcd9cabfbb8a56ba40c333fd842a.json.gz"
                prior = json.loads(gzip.decompress(path.read_bytes()))["source"]["stdJsonOutput"]["contracts"]["src/LaunchLocker.sol"][name]
            for kind in ["bytecode", "deployedBytecode"]:
                if contract["evm"][kind]["object"] != prior["evm"][kind]["object"]:
                    raise ValueError("independent compilation mismatch")
            contract["evm"] = {kind: {k: v for k, v in contract["evm"][kind].items() if k in ["object", "immutableReferences", "linkReferences"]} for kind in ["bytecode", "deployedBytecode"]}
            content = json.dumps({"compiler": COMPILER, "contract": contract}, indent=2) + "\n"
            (HERE / (name + "-compiled.json")).write_text(content)
            provenance["contracts"][name] = {"artifactSHA256": hashlib.sha256(content.encode()).hexdigest(), "creationBytes": len(contract["evm"]["bytecode"]["object"]) // 2, "runtimeBytes": len(contract["evm"]["deployedBytecode"]["object"]) // 2, "independentCompilationMatches": True}
    if args.action == "verify":
        provenance["factorySourceGzipSHA256"] = hashlib.sha256((HERE / "factory-source.json.gz").read_bytes()).hexdigest()
        (HERE / "compilation-proof.json").write_text(json.dumps(provenance, indent=2) + "\n")


if __name__ == "__main__":
    main()
