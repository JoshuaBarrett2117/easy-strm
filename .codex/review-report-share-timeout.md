# 分享任务时限审查

日期：2026-09-08；执行者：Codex。结论：通过，95/100。

- 去除固定30分钟限制；配置0使用context.WithCancel，自定义分钟使用context.WithTimeout；继承调用方取消，不改变外部请求自身超时。
- 配置复用系统配置DAO，入口位于分享管理“识别任务设置”，新任务读取一次并在metadata记录timeout_minutes。配置异常明确失败；手动取消设cancelled，达到总时限提示具体分钟数。
- Service验证无deadline且可取消、自定义期限、父取消/过期期限、配置读写及非法值；Controller验证缺字段、null、0、负数、小数与正整数。Go全量通过，浏览器无限制/90分钟保存重载回归通过，前端构建通过。
- 本地后端更新为13992，配置share_identify_timeout_minutes已保存0；前端页面200、未登录配置API401。未自动发起真实批量任务，旧失败任务需重新发起。
- 构建日志：.codex/share-task-timeout-build.log，既有大分块提示。无需数据库迁移，未改分享记录。
