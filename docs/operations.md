# 备份恢复与告警

这些工具面向当前单机开发部署：MySQL 是事实源，Redis 缓存可重建，尚未标记投递成功的 Outbox 记录仍可继续投递。RabbitMQ 队列不在本次备份范围；已经获 Broker 确认但尚未消费的消息，不能仅靠数据库恢复保证其异步副作用完成，因此正式备份前应暂停新写入并等待 Outbox / 消费队列排空。脚本不自动配置外部通知人，也不提供覆盖原库的恢复命令。

## CI

`.github/workflows/ci.yml` 启动独立 MySQL / RabbitMQ 服务，安装 Redis / FFmpeg 后运行 `scripts/check.sh`。检查包含 Go race 测试、真实 MySQL/RabbitMQ 集成测试、vet、模块校验、前端 API/Query 测试、UI 测试、TypeScript/Vite 构建及备份隔离/损坏校验回归；另执行 npm 高危依赖审计和 Prometheus 规则测试。集成环境缺失会失败，不会把跳过集成测试当成完整验收。

本地完整检查需设置 `GCFEED_REQUIRE_INTEGRATION=1`、测试专用 MySQL admin DSN 和 RabbitMQ URL；不要把凭据提交到仓库或粘贴进日志。测试只管理各自随机命名的库/队列。

## 创建备份

需要 Python 3.11+、已启动的项目 MySQL 以及可用的 `scripts/compose.sh`；Python 工具只使用标准库。

```bash
python3 scripts/backup.py create
# 输出 .local/backups/<UTC时间>-<随机ID> 的绝对路径
python3 scripts/backup.py drill .local/backups/<对应备份目录>
```

`create` 通过 `scripts/compose.sh` 使用当前 GCFeed Docker context，容器内读取 MySQL 凭据。默认源库取容器 `MYSQL_DATABASE`，上传文件取本机 `apps/api/uploads`。Docker API 使用的是独立 uploads volume：运行 Docker API 时必须先把其 uploads 内容导出到受保护目录，并通过 `--uploads <目录>` 指定，不能拿本机目录冒充容器文件。`--database` 主要供隔离 fixture 演练使用。

MySQL 使用 `mysqldump --single-transaction --quick` 导出 InnoDB 一致性快照。单行 INSERT 便于直接从同一份 SQL 计算各表行数，无须在导出后查询仍在变化的源库。备份期间不要执行 DDL；工具拒绝非 InnoDB 业务表。数据库与文件系统没有跨资源事务，因此正式备份应先停止业务写入；文件在复制期间发生改变会中止备份。快照时间窗口、文件大小和 SHA256 均写入 manifest。

每个完整备份包含 `database.sql`、`uploads.tar.gz`、`manifest.json`。目录权限为 0700，文件为 0600，且位于 Git 忽略的 `.local` 下。备份含账号及业务数据，应按敏感数据管理；SHA256 用于发现损坏，并非防恶意篡改的签名。仅将自己生成的可信备份交给演练工具。失败时没有完整 manifest 的目录不可作为可恢复备份。

## 恢复演练的验收范围

演练只新建 `gcfeed_restore_test_<随机ID>` 库和独立临时文件目录；SQL 导入该临时库，绝不覆盖原库。完成或异常退出时清理本次创建的资源，并在备份目录保存 `restore-report.json`。进程被强制 SIGKILL 或 MySQL 不可用时无法保证清理成功，报告会保留临时库名供人工核查，不能批量删除前缀相同的其他库。

验收包括：

- SQL / tar 文件摘要、SQL 中的表行数与 manifest 一致。
- 恢复后各表行数与导出快照一致，归档内每个上传文件大小、SHA256 一致。
- 视频作者、视频统计、互动用户/视频、上传所属用户等关键关系无悬空引用，点赞/收藏/评论计数与事实表一致。
- `upload_asset` 已登记的本地文件全部存在且大小符合登记。旧视频可能是历史测试数据，缺失旧 media/cover 路径会单独报告，不能据此声称所有视频文件可恢复。

报告记录 SQL 导入耗时和总耗时。它是本机该数据量下的恢复演练结果，不等同于生产 RTO、异地容灾或自动备份保留策略。已有备份不会自动删除；重要数据应另存加密的异地副本。脚本未备份数据库账户/授权、服务器配置及外部对象存储。

### 本地验收记录

2026-11-17（UTC+8）执行备份 `20261116T173123Z-7417da25` 前，已停止 API / Worker / Web，确认 8 个 RabbitMQ 业务/失败队列的 ready 与 unacked 均为 0。快照窗口为 UTC `2026-11-16 17:31:23.548` 至 `17:31:23.919`，备份耗时 0.756 秒；19 张表共 33,433 行、12 个上传文件恢复一致，2 条登记资产全部验证通过，6 项关系/互动计数检查均无违规。SQL 导入 27.592 秒，总演练 30.511 秒，临时库和文件目录已删除，备份权限检查通过。

历史测试数据中 `/uploads/{bench,regression,review-fixture}.{mp4,png}` 共 6 个路径没有对应文件，已经在报告中明确列出。上述通过结论仅涵盖实际归档文件和登记资产，不涵盖这些历史占位媒体。原始证据保留在 `.local/backups/20261116T173123Z-7417da25/{manifest,restore-report,run-context}.json`，不提交包含敏感业务内容的 SQL / 文件归档。

## 关键告警

`apps/monitoring/alerts.yml` 由 Compose 的 Prometheus 加载。阈值是当前个人项目的初始基线，应在实际负载下调整。

| 告警 | 条件与持续时间 | 首要排查 |
| --- | --- | --- |
| API down | API scrape 失败或目标指标缺失，持续 1 分钟 | API 进程、网络、ready 与启动日志 |
| API 5xx | 5 分钟错误率 > 5%，请求速率至少 12/分钟，持续 2 分钟 | API 日志、依赖健康、最近变更 |
| API P95 | 5 分钟 P95 > 500ms，同样设置最小流量，持续 5 分钟 | 路由耗时、慢查询、缓存命中 |
| Worker down | Worker scrape 失败或目标指标缺失，持续 1 分钟 | Worker 进程、网络、ready 与启动日志 |
| Outbox backlog | 最早待投递消息年龄 > 60 秒，持续 2 分钟 | Broker 健康、投递日志、消费者处理 |
| Outbox failed | 耗尽重试的失败事件 > 0，持续 1 分钟 | 先定位 last_error，修复原因后使用项目失败事件重放工具 |
| Outbox sampling | 采样失败/缺失，或超过 60 秒未更新，持续 1 分钟 | Worker 到 MySQL 连接与采样错误 |

Outbox 是全库统计，多 Worker 抓取的数值使用 `max` 聚合，不能相加。采样告警使用最差实例，避免一个正常实例掩盖另一实例的采样失败。

```bash
# 有本机 promtool 时直接使用；否则使用 Compose 同版本的官方 Docker 镜像。
GCFEED_DOCKER_CONTEXT=colima-gcfeed bash apps/monitoring/check.sh

# 使用官方 Prometheus 可执行文件，开一个仅监听 localhost 的临时实例。
python3 apps/monitoring/drill.py --prometheus /path/to/prometheus
```

规则测试覆盖七类告警的触发/恢复、API 低流量不误报、API/Worker/采样指标缺失。实时演练运行原始告警规则，向独立测试端点注入 `gcfeed_outbox_failed=1`，等待原有 1 分钟持续条件进入 `firing`，再改为 0 验证恢复为 `inactive`；报告保存在 `.local/monitoring/alert-drill.json`。演练不会停止真实 Broker、修改业务指标或发送外部通知。

当前验证边界是规则计算和本机触发/恢复。尚未配置 Alertmanager 接收人，因此不会发送邮件、短信或聊天消息；也没有验证通知送达、升级值班或 Prometheus 自身不可用时的外部探测。

## 迁移、对账与失败事件

以下命令在独立的终端执行，使用本地开发环境地址。先停止旧 API/Worker，再迁移；迁移不需要清空业务数据库。

```bash
set -a
source apps/.env
set +a
export GCFEED_DATABASE_HOST=127.0.0.1 GCFEED_DATABASE_PORT=13307
export GCFEED_DATABASE_PASSWORD="$GCFEED_DB_PASSWORD"
export GCFEED_REDIS_ADDR=127.0.0.1:16379
export GCFEED_RABBITMQ_URL="amqp://gcfeed:${GCFEED_MQ_PASSWORD}@127.0.0.1:15673/"
cd apps/api
go run ./cmd/migrate
go run ./cmd/reconcile -limit 100
go run ./cmd/outbox -limit 50
go run ./cmd/failed -queue action -limit 50
```

`reconcile` 只读比较事实与计数，不自动修复。Outbox 的失败记录用 `cmd/outbox -replay '<event_id>'` 重新排队；消费失败用 `cmd/failed -queue action -replay '<event_id>'`，也可选择 `preheat` 或 `embedding`。先确认失败原因已经修复，工具保留原事件 ID 和 payload；无路由或投递确认失败时不会删除失败副本。旧格式互动事件不受支持，不提供强制执行旧协议的开关。
