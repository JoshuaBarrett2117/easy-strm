# 分享 STRM 指标口径（P1 item1）

- 更新日期：2026-10-11
- 维护者：Codex
- 范围：仅澄清指标单位，保留现有数值、去重策略、计数算法、路径与文件内容；不处理任何其他 P1 项。
- 来源：`/opt/data/output/estrm_followups_20261011.md` 的新问题 2，`easy-strm/internal/service/share_strm_export.go`、`share_strm_incremental.go`、`share_strm_metric_units.go`、`strm_output.go`。

## 不同单位不能直接相加

全量/筛选导出的 `processed_files` 是已遍历的**来源文件记录数**。一个来源文件可以对应多个季集，一个季集也可以存在多个来源。`written` 与 `exported_files` 表示本次实际写入/改写的 **STRM 文件数**；`skipped_dedupe` 是每个来源内去重后的**作品季集淘汰项数**。三者不是互斥的同单位分桶，不能要求 `written + skipped_dedupe = processed_files`。

已记录的 `440,098` 个来源、`155,023` 个 STRM 写入、`287,521` 个去重季集之和相差 `2,446`，不能仅凭这三个总量判定少写、多写或推导某个单一原因；需要来源-季集明细才可核算。无需为满足错误等式而修正任何计数。

回归使用三个来源文件：来源 A 含四个季集，来源 B 含两个相同季集且胜出，来源 C 含两个相同季集且淘汰（重复季集映射仅计一次）。实际结果是 `processed_files=3`、`written=exported_files=4`、`skipped_dedupe=4`，临时目录恰有四个 STRM；`4 + 4` 不等于 `3` 是正确结果。

## 接口单位字段

任务接口仍返回原有 `total_files/processed_files/success_files/failed_files` 及 metadata 数值；新增可选的 metadata `*_unit` 字段和 `metric_count_note`，旧客户端可忽略它们。原有 `written_unit`、`skipped_dedupe_unit` 保持不变。

| 字段 | 单位与边界 |
| --- | --- |
| 全量/筛选任务 `total_files/processed_files/success_files/failed_files` | 来源文件记录；total 是预扫描/已处理数量构成的进度分母，processed 是处理记录数，成功/失败也是来源级，非季集数。 |
| 纯增量或新任务 building 恢复中的上述字段 | 作品待办；仅本次尝试消费的作品，不是全队列大小或来源文件数。由对应 `*_unit` 明示，不能只相信历史字段名。 |
| 当前任务已成功全量后的消费/同任务恢复中的上述字段 | 保留全量来源文件进度及来源单位，不被作品消费覆盖；新任务不继承另一任务的全量历史。 |
| `written/exported_files/consumption_written` | 实际落盘/改写的 STRM 文件；未改写跳过不计入，已落盘后登记失败仍计入，不能当作成功凭证数或目录现存总数。 |
| `skipped_dedupe/consumption_skipped_dedupe` | 每来源内不重复的作品季集淘汰项；不作为错误。两者分别描述保留的全量结果与本次消费，不要重复相加。 |
| `processed_works/consumed_works/round_consumed_works/failed_works` | 尝试/已确认/当前页已确认/失败的作品待办。 |
| `pending_works` | 当前有界探测的作品待办数；满 100 项时 `pending_works_exact=false`，仅为下界。探测失败时 `pending_probe_failed=true`，之前的值不可当作新准确值。 |
| `affected_sources/skipped_sources` | 消费阶段遍历的来源记录 / 全量阶段来源级跳过，均不是季集淘汰数。 |
| `added/updated/skipped/conflicts` | 本次安全输出的文件创建/内容改写/未改写/路径冲突事件；不修正已有累计语义，不与来源级成功失败直接相加。 |

全量来源进度百分比只描述来源遍历，不代表后续作品检查点全部确认；完成边界及并发重新入队语义见 `docs/产品与架构.md` 的 P0 节。单位说明不改变 `phase`、revision、pending 精确标志或任务完成协议。

单位随实际计数类型判定：消费/恢复若没有可保留的 `full_export_completed=true` 与 `written`，作品进度单位优先于历史残留的 `export_query`；重新全量的 preparing 阶段仍使用来源单位。这样旧任务的查询信息不会把当前作品计数误标为来源计数。

## 可拆分提交与验证

P1 的代码与测试独立放在 `share_strm_metric_units.go`、`share_strm_metric_units_test.go`，仅在 `share_strm_scheduled.go` 的 `updateExportMetadata` 两个持久化调用处接入单位说明；与 P0 的 `Run` 改动是可单独选择的 hunk。本文件是独立 P1 文档，P0 文档及 P0 测试无需纳入 P1 提交。

`TestShareStrmMetricUnitsPreserveCountsAndInput` 验证来源/作品/恢复分类、计数与标志不变、非分享任务不扩展、输入元数据不变；`TestShareStrmMetricUnitsMultiEpisodeSourceCountsAreNotAdditive` 验证真实临时文件和非加法口径；`TestShareStrmMetricUnitsScheduledProgressAndFullRetention` 验证单位随实际进度类型变化且不覆盖全量指标。完整离线 build/vet/test、重复回归、race 与前端构建日志保存在 `/opt/data/output/estrm_p0_reviewed_*.log`；全仓既有无关 race 失败单独报告，不伪称全绿。
