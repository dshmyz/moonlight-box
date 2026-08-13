# 运维脚本

| 脚本 | 用途 | 示例 |
|------|------|------|
| `start.sh` | 启动/停止/重启服务 | `./ops/start.sh --daemon` |
| `health.sh` | 健康检查 | `./ops/health.sh http://localhost:8080` |
| `db_maintain.sh` | 数据库维护（压缩/检查/统计） | `./ops/db_maintain.sh --all` |
| `log_cleanup.sh` | 清理应用日志文件 | `./ops/log_cleanup.sh --dry-run` |
| `storage_cleanup.sh` | 清理孤立 blob 文件 | `./ops/storage_cleanup.sh --dry-run` |

## 快速上手

```bash
# 启动服务（后台）
./ops/start.sh --daemon

# 查看状态
./ops/start.sh --status

# 停止
./ops/start.sh --stop

# 健康检查
./ops/health.sh

# 数据库维护（先检查再压缩）
./ops/db_maintain.sh --integrity
./ops/db_maintain.sh --stats

# 日志清理（先预览）
./ops/log_cleanup.sh --dry-run
./ops/log_cleanup.sh --days 7

# 存储清理（先预览）
./ops/storage_cleanup.sh --dry-run
```
