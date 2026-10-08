"""更新日期：2026-10-08；维护者：Codex。只收集本地证据，明确排除 .backup 和配置凭据。"""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess


def git(repo, *arguments):
    return subprocess.check_output(["git", "-c", "core.quotepath=false", *arguments], cwd=repo)


def function_text(source, name):
    start = source.index("func " + name + "(")
    end = source.find("\nfunc ", start + 1)
    return source[start:] if end < 0 else source[start:end]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    repo = Path(__file__).resolve().parents[2]
    output = args.output.resolve()
    scopes = ["easy-strm", "docs", "debug/phase02", ".gitignore"]
    patch = git(repo, "diff", "--", *scopes)
    statistics = git(repo, "diff", "--stat", "--", *scopes)
    extra_paths = git(repo, "ls-files", "--others", "--exclude-standard", "-z", "--", *scopes).decode().split("\0")
    files = []
    for name in filter(None, extra_paths):
        if ".backup" in Path(name).parts or name.endswith(".env"):
            raise AssertionError("forbidden evidence input")
        result = subprocess.run(["git", "-c", "core.quotepath=false", "diff", "--no-index", "--", "/dev/null", name], cwd=repo, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if result.returncode not in (0, 1):
            raise RuntimeError(result.stderr.decode())
        patch += result.stdout
        result = subprocess.run(["git", "-c", "core.quotepath=false", "diff", "--no-index", "--stat", "--", "/dev/null", name], cwd=repo, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if result.returncode not in (0, 1):
            raise RuntimeError(result.stderr.decode())
        statistics += result.stdout
        files.append({"path": name, "sha256": hashlib.sha256((repo / name).read_bytes()).hexdigest()})
    (output / "resume-final.diff").write_bytes(patch)
    (output / "resume-final.diffstat").write_bytes(statistics)
    checks = []
    for name in ["resume-full-test", "resume-race", "resume-phase01", "resume-sql-generation-final", "resume-phase01-comparison", "resume-phase01-corrected-comparison", "resume-migration-freeze", "resume-diff-check"]:
        exit_code = int((output / (name + ".exit")).read_text())
        if exit_code != 0:
            raise AssertionError(name + " is not green")
        log = output / (name + ".log")
        checks.append({"name": name, "exit_code": exit_code, "raw_log": str(log), "sha256": hashlib.sha256(log.read_bytes()).hexdigest()})
    scenario_source = "easy-strm/internal/service/share_strm_phase01_test.go"
    original = git(repo, "show", "HEAD:" + scenario_source).decode()
    current = (repo / scenario_source).read_text()
    preserved = {}
    for name in ("phaseScenarios", "TestShareStrmPhase01Scenarios", "TestShareStrmPhase01FallbackAndLockedRevalidation"):
        preserved[name] = function_text(original, name) == function_text(current, name)
        if not preserved[name]:
            raise AssertionError("original scenario/assertion changed: " + name)
    for name in ("debug/phase01/compare.py", "easy-strm/internal/service/share_strm_progress_test.go"):
        preserved[name] = git(repo, "show", "HEAD:" + name) == (repo / name).read_bytes()
        if not preserved[name]:
            raise AssertionError("original regression changed: " + name)
    comparisons = []
    for name in ("resume-phase01-comparison.json", "resume-phase01-corrected-comparison.json"):
        value = json.loads((output / name).read_text())
        comparisons.append({"evidence": name, "counts": [{"against": row["against"], "count": row["count"], "semantic_equal": row["semantic_equal"]} for row in value["comparisons"]]})
    document = {
        "branch": git(repo, "branch", "--show-current").decode().strip(),
        "head": git(repo, "rev-parse", "HEAD").decode().strip(),
        "committed": False,
        "database_connected": False,
        "dba_checks_executed": False,
        "backup_directory_inspected": False,
        "tests": checks,
        "phase01_comparisons": comparisons,
        "original_assertions_preserved": preserved,
        "new_files": files,
        "diff_sha256": hashlib.sha256(patch).hexdigest(),
        "migration_fingerprints": (repo / "debug/phase02/migrations.sha256").read_text().splitlines(),
    }
    (output / "resume-evidence.json").write_text(json.dumps(document, ensure_ascii=False, indent=2) + "\n")
    print(f"PASS: {len(checks)} green checks; original assertions preserved; {len(files)} new files included, .backup excluded")


if __name__ == "__main__":
    main()
