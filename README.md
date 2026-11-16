# GoF

一个 Go 短视频 Feed 项目。支持注册登录、上传发布、最新/关注/热门/推荐流、点赞收藏评论、关注关系与站内消息。

基于 [LeoninCS/GCFeed](https://github.com/LeoninCS/GCFeed) 的 `8cf995cfa03985c335594c453ea050df5c0f1b6a` 继续开发。应用名称、Go module 和数据库名称仍使用 GCFeed。

## 实现

- **API**：Gin + GORM。业务事实写入 MySQL；点赞、收藏及视频发布与 Outbox 在同一事务提交。
- **Worker**：投递 Outbox，消费 RabbitMQ 消息，更新通知、热榜、视频卡片和文本特征。重复投递由各投影幂等处理。
- **Redis**：Feed 缓存、排名快照、热榜、账号接口限频。关注关系与互动计数以数据库为准。
- **Web**：React + TypeScript、TanStack Router/Query、Tailwind、shadcn Base UI。上传由 FFmpeg/ffprobe 校验和处理。

推荐使用字符 n-gram 哈希向量与行为特征，不是训练过的推荐模型。项目没有审核后台、内容搜索或运营平台；当前采用单机本地文件存储。

## 启动

准备 Go（版本见 `apps/api/go.mod`）、Node（见 `.nvmrc`）、npm、Python 3.11+、FFmpeg 和 Docker Compose。

```bash
./scripts/setup.sh
./scripts/start.sh
```

打开 http://127.0.0.1:15173 。API 在 http://127.0.0.1:18080 。停止本地进程用 `./scripts/stop.sh`，数据库与文件会保留。

`setup.sh` 自动生成本机密钥到被 Git 忽略的 `apps/.env`。完整容器部署、端口和环境变量见[开发说明](docs/development.md)。

## 检查

```bash
./scripts/check.sh
python3 scripts/smoke.py        # 需要正在运行的本地服务
python3 scripts/regression.py
```

`check.sh` 包含 Go race/vet、前端逻辑与 UI 测试、类型检查和构建。完整的 MySQL/RabbitMQ 集成检查需要[开发说明中的环境变量](docs/development.md#完整检查)；未配置时这些测试会跳过，CI 中强制运行。

## 文档

- [架构与数据一致性](docs/architecture.md)
- [开发环境与检查](docs/development.md)
- [前端模块](docs/frontend-development.md) · [界面行为](docs/uiux.md)
- [备份恢复、失败事件处理与告警](docs/operations.md)
- [混合负载测试及实测限制](docs/performance-baseline.md)

依赖锁文件、SQL 迁移和测试属于源码。密钥、上传文件、数据库备份、压测原始记录、截图及构建产物留在本地，不提交。
