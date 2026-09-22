"""Run isolated compiler probes; use saved sources, never an onchain RPC."""

import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import time

ROOT = Path(__file__).resolve().parent
CACHE = Path("/private/tmp/token-taxes-runtime-20260922")
JS = Path("/private/tmp/token-taxes-solc-20260922")
MODULES = Path("/private/tmp/lp-lock-solc-0830/node_modules")
SERVER_IMAGE = "sha256:129d54b986d38751e010fa0277891e3d0516774516c95c6367374f6c7b057fc9"
NODE_IMAGE = "sha256:83f487e0a63425e5b4d146fb5e5be574bcbe1b7b843d3ebafdd95eaf7767a7e5"


def save(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2) + "\n")


def invoke(case, mode, input_data, options=(), memory="512m", compiler_options=()):
    native = mode.startswith("native")
    arch = "amd64" if mode.endswith("amd64") else "arm64"
    # Supervisor remains native arm64; amd64 compiler is explicitly emulated.
    command = ["docker", "run", "--rm", "-i", "--network", "none", "--read-only",
               "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
               "--user", "65532:65532", "--cpus", "1", "--memory", memory,
               "--memory-swap", memory, "--pids-limit", "32",
               "--tmpfs", "/tmp:rw,noexec,nosuid,size=16m",
               "--mount", f"type=bind,src={CACHE},dst=/probe,readonly",
               "--mount", f"type=bind,src={ROOT},dst=/research,readonly"]
    if not native:
        command += ["--mount", f"type=bind,src={MODULES},dst=/deps/node_modules,readonly",
                    "--mount", f"type=bind,src={JS},dst=/compilers,readonly"]
    command += ["--entrypoint", "/probe/supervise-arm64", SERVER_IMAGE if native else NODE_IMAGE]
    command += ["-memory-mib", "512" if native and arch == "arm64" else "0"]
    command += list(options) + ["--"]
    if native:
        builds = json.loads((ROOT / "evidence/native-builds.json").read_text())
        build = next(b for b in builds if b["platform"] == "linux-" + arch and b["version"] == case["version"] and b["available"])
        command += ["/probe/linux-" + arch + "/" + build["path"], "--standard-json"]
        command += list(compiler_options)
    else:
        command += ["/usr/local/bin/node", "/research/compile.cjs", "/deps/node_modules/solc",
                    "/compilers/" + case["version"] + ".json"]
    start = time.monotonic()
    p = subprocess.run(command, input=input_data, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=45)
    elapsed = round((time.monotonic() - start) * 1000, 2)
    if p.returncode:
        return {"mode": mode, "case": case["name"], "dockerExitCode": p.returncode,
                "containerWallMs": elapsed, "dockerError": p.stderr.decode(errors="replace")[:2000]}
    result = json.loads(p.stdout)
    result.update(mode=mode, case=case["name"], containerWallMs=elapsed)
    return result


def validate(result, case, expected):
    output = result.pop("output", {})
    errors = [e for e in output.get("errors", []) if e.get("severity") == "error"]
    result["compilerErrors"] = len(errors)
    if errors:
        result["compilerErrorKinds"] = [e.get("type") for e in errors]
    if "contracts" in output:
        source, name = case["qualifiedName"].rsplit(":", 1)
        artifact = output["contracts"][source][name]
        result["artifactMatchesPreviousCompile"] = artifact == expected[case["address"]]["artifact"]
        result["runtimeSHA256"] = hashlib.sha256(bytes.fromhex(artifact["evm"]["deployedBytecode"]["object"])).hexdigest()
        result["sourceASTCount"] = sum("ast" in s for s in output.get("sources", {}).values())
    return result


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("mode", choices=("matrix", "controls", "imports"))
    args = parser.parse_args()
    cases = json.loads((ROOT / "evidence/cases.json").read_text())
    expected = json.loads((ROOT.parent / "token_taxes_20260922/evidence/live-compiled.json").read_text())
    results = []
    if args.mode == "matrix":
        for mode in ("native-arm64", "native-amd64", "node-arm64"):
            for case in cases:
                if mode == "native-arm64" and case["version"] == "0.5.17":
                    continue
                for repeat in range(3):
                    result = invoke(case, mode, (CACHE / (case["name"] + ".json")).read_bytes())
                    validate(result, case, expected)
                    result["repeat"] = repeat + 1
                    results.append(result)
                    save(ROOT / "evidence/matrix.json", results)
                    print(json.dumps(result), flush=True)
    elif args.mode == "imports":
        for case in cases:
            if case["version"] == "0.5.17":
                continue
            result = invoke(case, "native-arm64", (CACHE / (case["name"] + ".json")).read_bytes(),
                            compiler_options=["--no-import-callback"])
            validate(result, case, expected)
            result["check"] = "bundled_sources_" + case["version"]
            results.append(result)
        source = 'pragma solidity ^0.8.0; contract Importable {}'
        (CACHE / "importable.sol").write_text(source)
        case = next(c for c in cases if c["name"] == "ShinyLIMPET")
        url_input = json.dumps({"language": "Solidity", "sources": {"Importable.sol": {"urls": ["/probe/importable.sol"]}},
                                "settings": {"outputSelection": {"*": {"*": ["evm.deployedBytecode"]}}}}).encode()
        result = invoke(case, "native-arm64", url_input, compiler_options=["--no-import-callback"])
        validate(result, case, expected)
        result["check"] = "file_url_disabled"
        results.append(result)
        save(ROOT / "evidence/imports.json", results)
        for result in results:
            print(json.dumps(result), flush=True)
    else:
        case = next(c for c in cases if c["name"] == "ShinyLIMPET")
        real_input = (CACHE / (case["name"] + ".json")).read_bytes()
        functions = "\n".join(f"function f{i}(uint x) external pure returns(uint) {{return x*{i+1}+{i};}}" for i in range(2000))
        stress = json.dumps({"language": "Solidity", "sources": {"Stress.sol": {"content": "pragma solidity ^0.8.0; contract Stress {" + functions + "}"}},
                             "settings": {"viaIR": True, "optimizer": {"enabled": True, "runs": 200},
                                          "outputSelection": {"*": {"*": ["evm.deployedBytecode"]}}}}).encode()
        checks = [
            ("wall_timeout", stress, ["-timeout", "100ms", "-cpu-seconds", "10"]),
            ("explicit_cancel", stress, ["-cancel-after", "100ms", "-cpu-seconds", "10"]),
            ("cpu_limit", stress, ["-timeout", "10s", "-cpu-seconds", "1"]),
            ("memory_limit", real_input, ["-memory-mib", "24", "-timeout", "5s"]),
            ("output_limit", real_input, ["-output-bytes", "1024"]),
            ("invalid_source", b'{"language":"Solidity","sources":{"Bad.sol":{"content":"invalid solidity"}},"settings":{"outputSelection":{"*":{"*":["evm.bytecode"]}}}}', []),
        ]
        for label, inp, options in checks:
            result = invoke(case, "native-arm64", inp, options)
            validate(result, case, expected)
            result["check"] = label
            results.append(result)
            save(ROOT / "evidence/controls.json", results)
            print(json.dumps(result), flush=True)
        result = invoke(case, "native-arm64", real_input)
        validate(result, case, expected)
        result["check"] = "healthy_after_failures"
        results.append(result)
        save(ROOT / "evidence/controls.json", results)
        print(json.dumps(result), flush=True)


if __name__ == "__main__":
    main()
