import argparse
import copy
import json
import statistics
from pathlib import Path


def semantic(result):
    value = copy.deepcopy(result)
    value.pop("ElapsedNS")
    counts = value.pop("Counts")
    value["FilesystemChanges"] = {
        name: counts[name] for name in ("Creates", "Rewrites", "Deletes")
    }
    for file in value["Files"].values():
        file.pop("Modified")
    return value


def compare(before, after):
    if len(before) != len(after):
        raise AssertionError("场景数量不一致")
    rows = []
    for old, new in zip(before, after):
        if semantic(old) != semantic(new):
            raise AssertionError(f"语义不一致: {old['Scenario']}")
        rows.append({
            "scenario": old["Scenario"],
            "semantic_equal": True,
            "baseline_ns": old["ElapsedNS"],
            "after_ns": new["ElapsedNS"],
            "baseline_counts": old["Counts"],
            "after_counts": new["Counts"],
            "final_counts": {name: new[name] for name in ("Total", "Processed", "Success", "Failed", "Added", "Updated", "Skipped", "Conflicts", "SkippedSources", "ExportedFiles")},
        })
    return rows


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("directory", type=Path)
    args = parser.parse_args()
    output = args.directory
    read = lambda name: json.loads((output / name).read_text())
    rows = compare(read("baseline.json"), read("after.json"))
    rows += compare(read("baseline.json.revalidation.json"), read("after.json.revalidation.json"))
    baseline_runs = [json.loads(line) for line in (output / "baseline-benchmark.jsonl").read_text().splitlines()]
    after_runs = [json.loads(line) for line in (output / "after-benchmark.jsonl").read_text().splitlines()]
    if len(baseline_runs) != len(after_runs) or not baseline_runs:
        raise AssertionError("基准重复轮数不一致或无结果")
    for old, new in zip(baseline_runs, after_runs):
        compare(old, new)
    timings = []
    for index, scenario in enumerate(baseline_runs[0]):
        old = [result[index]["ElapsedNS"] for result in baseline_runs]
        new = [result[index]["ElapsedNS"] for result in after_runs]
        timings.append({"scenario": scenario["Scenario"], "baseline_ns": old, "after_ns": new,
                        "baseline_median_ns": statistics.median(old), "after_median_ns": statistics.median(new)})
    (output / "comparison.json").write_text(json.dumps({"scenarios": rows, "benchmark": timings}, ensure_ascii=False, indent=2) + "\n")
    lines = ["# 阶段 0/1 本地测量对比", "", "日期：2026-10-08；维护者：Codex。所有场景的最终任务计数、错误、文件内容、净磁盘变化、状态与 stale 集合自动比较一致。", "", "| 场景 | 内存读 前→后 | 模拟 SQL 读/写 前→后 | 进度/元数据持久化 前→后 | 导出耗时中位数 ms 前→后 |", "|---|---|---|---|---|"]
    medians = {result["scenario"]: result for result in timings}
    for result in rows:
        old, new = result["baseline_counts"], result["after_counts"]
        timing = medians.get(result["scenario"])
        duration = f"{timing['baseline_median_ns']/1e6:.3f}→{timing['after_median_ns']/1e6:.3f}" if timing else "仅回归，不计入基准"
        lines.append(f"| {result['scenario']} | {old['MemoryReads']}→{new['MemoryReads']} | {old['SQLReads']}/{old['SQLWrites']}→{new['SQLReads']}/{new['SQLWrites']} | {old['ProgressWrites']}/{old['MetadataWrites']}→{new['ProgressWrites']}/{new['MetadataWrites']} | {duration} |")
    lines += ["", "限制：大部分来源与播放映射使用内存 store；sql_seed/sql_unchanged 调用真实 ShareRecordDAO 的 sqlmock SQL。SQL 计数来自实际驱动调用（含锁查询/释放，不含事务控制语句），任务使用本地 miniredis，文件使用 TMPDIR 下真实磁盘；不是生产 PostgreSQL/Redis 延迟。磁盘计数来自前后内容、mtime、inode 的观察，不是业务计数推断。跨运行绝对路径、mtime 和包含临时路径的 content fingerprint 不参与相等判断；内容、mapping fingerprint 与状态参与比较。当前重启是全量重跑，无 checkpoint/resume。未更改 SQL、schema、迁移、查询范围、命名、归属检查或删除策略。", "", "三轮耗时取中位数，部分场景未加速或波动较大，不据此承诺生产性能。after-benchmark-concurrent-race.* 保留与 race 同时运行的首次测量；主表使用随后单独执行的相同三轮基准，避免竞态测试额外负载。"]
    (output / "summary.md").write_text("\n".join(lines) + "\n")
    print(f"PASS: {len(rows)} 个场景；{len(baseline_runs)} 轮同夹具基准语义一致")


if __name__ == "__main__":
    main()
