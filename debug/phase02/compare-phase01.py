"""更新日期：2026-10-08；维护者：Codex。复用一期原始语义比较器，不修改断言。"""
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("baseline", type=Path)
    parser.add_argument("current", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    original = Path(__file__).parents[1] / "phase01" / "compare.py"
    spec = importlib.util.spec_from_file_location("phase01_compare", original)
    comparison = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(comparison)
    all_rows = []
    evidence = []
    for name in ("baseline", "after"):
        rows = []
        for suffix in ("", ".revalidation.json"):
            previous = args.baseline / (name + ".json" + suffix)
            current = Path(str(args.current) + suffix)
            before = json.loads(previous.read_text())
            after = json.loads(current.read_text())
            rows.extend(comparison.compare(before, after))
            evidence.append({"path": str(previous), "sha256": hashlib.sha256(previous.read_bytes()).hexdigest(), "count": len(before)})
        if len(rows) != 32 or not all(row["semantic_equal"] for row in rows):
            raise AssertionError(f"{name}: 必须是原始 32 场景且全部语义相同")
        all_rows.append({"against": name, "count": len(rows), "semantic_equal": True, "scenarios": rows})
    args.output.write_text(json.dumps({"original_assertions": str(original), "original_assertions_sha256": hashlib.sha256(original.read_bytes()).hexdigest(), "evidence": evidence, "comparisons": all_rows}, ensure_ascii=False, indent=2) + "\n")
    print("PASS: 原始 baseline 32/32；一期 after 32/32；复用未修改的 compare()")


if __name__ == "__main__":
    main()
