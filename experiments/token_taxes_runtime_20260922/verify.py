"""Check recorded compiler and isolation outcomes without network calls."""

import json
from pathlib import Path
import statistics

from probe import CACHE, ROOT, validate


def main():
    evidence = ROOT / "evidence"
    rows = json.loads((evidence / "matrix.json").read_text())
    checks = {r["check"]: r for r in json.loads((evidence / "controls.json").read_text())}
    assert len(rows) == 24, "incomplete comparison matrix"
    groups = {}
    for row in rows:
        assert row.get("success") and row.get("artifactMatchesPreviousCompile"), row
        assert row["compilerErrors"] == 0 and row["sourceASTCount"] > 0, row
        groups.setdefault((row["mode"], row["case"]), []).append(row)
    summary = []
    for (mode, case), runs in sorted(groups.items()):
        assert len(runs) == 3, (mode, case)
        assert len({r["runtimeSHA256"] for r in runs}) == 1, (mode, case)
        summary.append({"mode": mode, "case": case, "runs": len(runs),
                        "medianMs": round(statistics.median(r["elapsedMs"] for r in runs), 3),
                        "maxRssMiB": round(max(r["maxRssKiB"] for r in runs)/1024, 3)})
    assert len(summary) == 8
    assert checks["wall_timeout"]["deadlineExceeded"] and not checks["wall_timeout"]["success"]
    assert checks["explicit_cancel"]["canceled"] and not checks["explicit_cancel"]["success"]
    assert checks["cpu_limit"]["signal"] == "killed" and not checks["cpu_limit"]["deadlineExceeded"]
    assert checks["memory_limit"]["compilerErrors"] > 0
    assert not checks["memory_limit"].get("artifactMatchesPreviousCompile", False)
    assert checks["output_limit"]["outputLimitExceeded"]
    assert checks["output_limit"]["stdoutBytes"] <= 1024
    assert checks["invalid_source"]["compilerErrors"] > 0
    assert checks["healthy_after_failures"]["artifactMatchesPreviousCompile"]
    imports = json.loads((evidence / "imports.json").read_text())
    assert len(imports) == 3
    for result in imports[:2]:
        assert result["artifactMatchesPreviousCompile"] and result["compilerErrors"] == 0
    assert imports[2]["check"] == "file_url_disabled" and imports[2]["compilerErrors"] > 0
    for label in ("wall_timeout", "explicit_cancel", "output_limit"):
        assert checks[label]["elapsedMs"] < 2000, label

    cases = json.loads((evidence / "cases.json").read_text())
    expected = json.loads((ROOT.parent / "token_taxes_20260922/evidence/live-compiled.json").read_text())
    images = []
    for name, filename in (("ShinyLIMPET", "image-lmpt.json"), ("TAOT", "image-taot.json")):
        case = next(c for c in cases if c["name"] == name)
        result = json.loads((CACHE / filename).read_text())
        validate(result, case, expected)
        assert result["success"] and result["compilerErrors"] == 0 and result["artifactMatchesPreviousCompile"], result
        result["case"] = name
        images.append(result)
    (evidence / "image-smoke.json").write_text(json.dumps(images, indent=2) + "\n")
    for source in ROOT.glob("*.go"):
        assert source.read_text().startswith("//go:build ignore"), source
    (evidence / "summary.json").write_text(json.dumps({"matrix": summary, "controlsPassed": len(checks),
                                                     "importChecksPassed": len(imports),
                                                     "imageSmokePassed": len(images), "matrixRunsPassed": len(rows)}, indent=2) + "\n")
    print(json.dumps({"matrixRunsPassed": len(rows), "controlsPassed": len(checks), "importChecksPassed": len(imports), "imageSmokePassed": len(images)}))
    for entry in summary:
        print(json.dumps(entry))


if __name__ == "__main__":
    main()
