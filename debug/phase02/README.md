# 分享 STRM phase02 本地与 DBA 验证

- 更新日期：2026-10-08；维护者：Codex。
- 开发者只运行 `local-tests.sh`、Python AST/`sh -n`。**不得运行任何 DBA SQL 或 `run.sh --synthetic-test-only --execute`。**
- DBA 脚本默认不连接；显式执行需要外部 `PGHOST/PGPORT/PGUSER/PGDATABASE`、目标确认、授权和绝对证据目录。不读取仓库或 output 中的 `.env`/连接文件，不包含密码。数据库固定为 DBA 已创建的 `easy_strm_test/public`，审批版本 PG17.6。
- 最终执行清单、冻结指纹、完整新旧 SQL：`/opt/data/output/easy-strm-phase02/dba-execution-handoff.md`。本地评审：同目录 `resume-codex-review.md`。脚本只是交付，**没有执行真实迁移、回滚、seed、EXPLAIN 或真实数据库集成测试**。

## 本地

```sh
sh debug/phase02/local-tests.sh "$PHASE02_LOCAL_OUTPUT"
python3 debug/phase02/compare-phase01.py "$PHASE01_ORIGINAL_OUTPUT" "$PHASE02_LOCAL_OUTPUT/resume-phase01.json" "$PHASE02_LOCAL_OUTPUT/resume-phase01-comparison.json"
```

调用方保持已安装 Go 在 `PATH`，保留 `TMPDIR`。脚本显式 unset 六个真实测试入口和 `GOFLAGS`，检查 Go 的有效 flags 为空；不使用 `-tags=live`。检查点及增量测试统一用 `TestShareExport`/`TestShareStrm` 前缀，纳入指定 race 命令。比较器复用未修改的一期 `compare()`，同时比较原始 baseline 和一期 after 的 32 场景。

## DBA 文件清单

| 文件 | 作用 |
|---|---|
| `preflight.sql` | 目标、版本、合成数据边界及真实索引/FK catalog |
| `prebuild-indexes.sql` | 两条路径索引 DBA 事务外 CONCURRENTLY 预建及有效性证据 |
| `seed.sql` | 可在 v42 下灌入 20k 媒体、100k 文件、100k 输出、20k 历史及旧清单；有 hot work/多季集/失效/失败来源 |
| `seed-checkpoints.sql` | 约 100k 合成检查点、20k dirty；仅 SQL fixture，不是实际 full 凭证 |
| `idempotency-state.sql` / `capture-state.sql` | ready/revision/pending/指纹哨兵与完整行稳定哈希，强制 cmp 验证迁移不重置 |
| `assertions.sql` / `rebind-orders.sql` | 白名单生产、诊断字段不投递、级联/SET NULL、配置、pending 范围、破坏保护、CAS、两种重绑消费顺序；事务回滚 |
| `dao-explain.sql` | 从实际 DAO 常量/旧构建器生成，完整 PREPARE/EXECUTE，新旧点查、冲突读、零 dirty、路径三分支、缺索引、100 work 原始计划 |
| `fanout.sql` / `bulk-dml.sql` | 20k work fanout、v43 下 100k 真实 DML 的锁/WAL/耗时；全部回滚 |
| `concurrency.py` | 持久 psql 会话 + 实际锁等待 barrier；晚提交、CAS 等待 commit/rollback、配置、排序及 40P01；K2/K3 SQL + 临时文件证据 |
| `run.sh` / `common.sh` | 备份、正向两次、回滚两次、再正向，逐命令原始日志/退出码；默认仅计划 |
| `fresh-required.sql` | 再安装必须 required/无凭证/空 checkpoint，零业务 FK |
| `migrations.sha256` | 通过最终本地测试后冻结的正反 SQL；任何变更须 DBA 重审 |

## 证据边界

- `concurrency.py` 是实际 SQL 协议和临时文件的演练，**不是运行 Go 导出器**。K2/K3 文件保留在 DBA `TMPDIR` 供核验，业务恢复只标 stale 不删文件。需要应用进程 kill/restart 的端到端演练应由 DBA/owner 在隔离环境另行授权，不在开发者本轮执行。
- SQL fixture 的 ready/ack 是合成测试状态，不能当作应用完成基线或生产凭证；应用必须明确运行既有完整导出成功，再显式重建。
- 全量事务 fanout 仍可能超锁/WAL预算；跨多语句反向锁序仍可能产生 40P01。排序只降低死锁，不宣称消除；应用传播错误、回滚且保留 pending，下轮重试。
- 每个待消费 work 重算全部有效来源，作品内 ID 分页不是变更水位。增量不调用 full/FinishSnapshot，不删除磁盘 stale，不注册新 cron/API。
- 若测试 catalog 本来没有 status 单列索引，两组计划不得声称“删索引前后比较”。路径旧索引对照和 status 对照仅在事务内临时 DROP 并 ROLLBACK，禁止在生产使用此脚本。新路径索引不能覆盖首次 legacy 查询的成本；是否还需旧清单路径索引由 DBA 另审，v43 没擅自新增。
