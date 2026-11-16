# 开发环境

依赖版本固定在 `apps/api/go.mod`、`.nvmrc`、`apps/web/package.json` 和 lockfile。需要 Go、Node 26/npm 11、Python 3.11+、FFmpeg/ffprobe、Docker Compose。运行 Redis 集成测试还需本机 `redis-server`。

## 准备和启动

```bash
# macOS 可选独立 VM，不切换其他项目的默认 Docker context：
colima start --profile gcfeed --cpu 4 --memory 6 --disk 40 --activate=false
./scripts/setup.sh
./scripts/start.sh
```

`setup.sh` 下载依赖并生成 `apps/.env`，权限 0600；重复运行保留已有文件。仅标记为 `generated-by-setup` 的字段生成随机密钥，其他配置保留示例默认值。

`start.sh` 启动依赖容器、构建 API/Worker，等 API 完成迁移后启动 Worker 和 Vite。前端支持 HMR；Go 改动后运行 `./scripts/stop.sh`，再运行 `./scripts/start.sh`。

脚本优先使用 `GCFEED_DOCKER_CONTEXT`，未指定且存在 `colima-gcfeed` 时使用它，否则使用当前 Docker context：

```bash
GCFEED_DOCKER_CONTEXT=default ./scripts/start.sh
```

## 地址和配置

| 服务 | 本地地址 |
| --- | --- |
| Web | http://127.0.0.1:15173 |
| API | http://127.0.0.1:18080 |
| API ready / metrics | `/ready` / `/metrics` |
| Worker ready / metrics | http://127.0.0.1:19091/ready · http://127.0.0.1:19091/metrics |
| MySQL | 127.0.0.1:13307，数据库 GCFeed，用户 gcfeed |
| Redis | 127.0.0.1:16379 |
| RabbitMQ | AMQP 127.0.0.1:15673；管理台 http://127.0.0.1:15674 |

密钥读取 `apps/.env`；YAML 中不提供可用的默认凭据。`GCFEED_*` 环境变量覆盖配置，映射见 `apps/api/internal/infra/config/config.go`。`start.sh` 自动设置本地依赖地址。

## 停止与容器部署

`Ctrl+C` 或 `./scripts/stop.sh` 停止本地 API/Worker/Vite，保留数据容器。停止依赖用 `./scripts/compose.sh stop`；不要用会删除数据卷的 `down -v` 代替日常停止。

完整容器模式需要先停止本地应用，避免端口冲突：

```bash
./scripts/compose.sh up -d --build --wait
./scripts/compose.sh logs -f api worker web
```

Web/API 地址不变；Prometheus 在 http://127.0.0.1:19090，Grafana 在 http://127.0.0.1:13000，用户 admin，密码见 `GCFEED_GRAFANA_PASSWORD`。本地模式不自动运行这套监控。

切回本地运行前执行 `./scripts/compose.sh stop api worker web prometheus grafana`。本地上传目录是 `apps/api/uploads`，容器使用命名卷，二者不会自动同步。

## 完整检查

```bash
source apps/.env
export GCFEED_TEST_MYSQL_ADMIN_DSN="root:${GCFEED_DB_ROOT_PASSWORD}@tcp(127.0.0.1:13307)/?parseTime=true"
export GCFEED_TEST_RABBITMQ_URL="amqp://gcfeed:${GCFEED_MQ_PASSWORD}@127.0.0.1:15673/"
GCFEED_REQUIRE_INTEGRATION=1 ./scripts/check.sh
npm --prefix apps/web audit --audit-level=high
python3 scripts/smoke.py
python3 scripts/regression.py
unset GCFEED_TEST_MYSQL_ADMIN_DSN GCFEED_TEST_RABBITMQ_URL
```

Go 集成测试创建并删除自己的随机数据库、队列和独立 Redis 进程；未提供环境变量时会明确跳过。CI 强制运行集成检查，另外校验告警规则。

`smoke.py` 覆盖真实上传、发布、Range 播放及互动；`regression.py` 覆盖鉴权、资产归属、关注变化、幂等请求和缓存删除可见性。两者保留测试账号及上传文件，只软删除自己的视频。连续运行会触发正常注册限频，等待 `Retry-After` 后重试即可。

## 账号与上传约束

登录按客户端 IP 每分钟最多 10 次，注册最多 5 次。Redis 共享固定窗口，故障返回 503；未配置 Redis 的单进程测试环境使用有界内存计数。超限返回 429 和 `Retry-After`。

默认忽略转发 IP 头。使用反向代理时将 `GCFEED_TRUSTED_PROXIES` 设为实际代理 IP/CIDR（逗号分隔），入口代理应覆盖客户端转发头。共享出口用户会共享配额，部署阈值需结合实际流量调整。

账号状态和角色每次鉴权重新读取。新发布的 video/cover 和修改的 avatar 必须是本人已登记的对应类型资产；其他用户、错误类型及任意外链统一拒绝。文件通过 `/uploads` 公开读取，归属校验不等于私有文件权限控制。

## 运行与诊断

API `/ready` 检查 MySQL/Redis，Worker 另检查 MQ。Worker 连接 MQ 失败后退出；Compose 自动重启，本地 `start.sh` 会停止整组进程，需要重新启动。

`gcfeed_db_*` 暴露连接池状态；`gcfeed_outbox_*` 暴露积压和采样健康。事件回放、备份和告警见[运维说明](operations.md)，混合负载实验见[性能基线](performance-baseline.md)。
