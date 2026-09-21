# 使用日志模型诊断可见性迁移

本维护步骤把历史 `logs.other` 顶层的模型诊断字段移动到
`other.admin_info`，使它们只会由管理员和超级管理员的使用日志接口返回。

涉及字段：

- `is_model_mapped`
- `upstream_model_name`
- `response_model`

应用代码已经会在普通用户的 `/api/log/self` 和 `/api/log/token` 响应中
移除这些历史顶层字段。因此，迁移不是用户侧数据脱敏的前置条件；它用于让历史
记录采用正式存储结构，并让管理员继续在新前端中查看诊断信息。

## 发布顺序

1. 先发布包含本功能的后端和前端版本。
2. 在运行环境中执行 dry-run，确认候选数符合预期。
3. 备份 `logs` 表后执行 apply。
4. 确认命令报告 `remaining=0`，再检查管理员和普通用户视图。

不要在旧版本仍可接收请求时执行 apply：旧写入路径会继续创建顶层诊断字段，
使迁移结果立即失效。应在新版本健康切换完成后，或在写流量已明确停止的维护窗口中
执行。

迁移命令不会在服务启动时自动运行，也不修改任务表的 `properties`。任务的
`upstream_model_name` 由 `/api/task/self` 的用户 DTO 投影隐藏；原生 `/v1`
任务响应不受影响。

## 备份

MySQL 示例（把备份保存在宿主机受控目录，命令不会打印数据库密码）：

```bash
backup_dir=/var/backups/new-api
mkdir -p "$backup_dir"
docker exec mysql sh -c 'mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" --single-transaction --no-tablespaces "$MYSQL_DATABASE" logs' \
  | gzip > "$backup_dir/logs-before-model-visibility-$(date +%Y%m%d%H%M%S).sql.gz"
```

确认生成的压缩文件可读取且大小合理，再继续 apply。生产库有持续写入时，不要用
整表恢复作为常规回滚手段，因为那会覆盖迁移后的新日志。

## 执行

维护二进制继承运行服务的 `SQL_DSN` 和可选 `LOG_SQL_DSN`，只打开日志数据库，
不执行主库 schema 迁移或检查。先运行 dry-run：

```bash
go run ./cmd/migrate-log-model-visibility --batch-size 500
```

输出中的字段含义：

- `scanned`：本次扫描的日志行数。
- `candidates`：仍带有旧顶层诊断字段的日志数。
- `migrated`：实际重写的日志数；dry-run 中始终为零。
- `remaining`：扫描完成后仍含旧字段的日志数。

确认 dry-run 正常后执行：

```bash
go run ./cmd/migrate-log-model-visibility --apply --batch-size 500
```

`--apply` 按批次事务写入。每行会保留所有非诊断字段；若 `admin_info` 中已存在
同名诊断字段，保留该管理员作用域的值并仅删除旧顶层字段。非法 JSON 或非对象的
`admin_info` 会使命令失败，不会静默改写该行；当前批次发生写入错误时会整体回滚。

## 校验

apply 成功必须以命令输出的 `remaining=0` 为准。可重复执行 dry-run 作为额外校验：

```bash
go run ./cmd/migrate-log-model-visibility --batch-size 500
```

预期输出为 `candidates=0` 和 `remaining=0`。随后用普通用户会话确认使用日志没有
模型映射警告、实际模型或响应模型详情；用管理员和超级管理员会话确认相同日志仍在
`admin_info` 下提供这些诊断信息。

## 回滚

该迁移不删除整条日志，但字段冲突时以已有 `admin_info` 为准，因此不能无备份地
逐字节恢复被移除的旧顶层冲突值。若必须回退到旧版本：

1. 停止后续 apply，并保留本次备份。
2. 仅针对迁移命令报告涉及的日志 ID，从备份中构造并审核定向更新；不要恢复整个
   `logs` 表。
3. 完成定向恢复后再次执行 dry-run，确认结果与目标版本的读取逻辑相符。

通常无需为回退旧前端而迁移回顶层结构：先恢复应用版本即可，数据库备份仅用于
需要精确恢复历史字段的异常情况。
