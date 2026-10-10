---
paths:
  - internal/sqlkit/**
  - internal/db/**
  - internal/app/**
---
# SQL 相关的问题，先看开源实现，不要想当然

涉及 SQL 的词法分析和方言差异、catalog 查询、结果的读取与解码、修改回写、只读判定、补全时，动手前先查下面这些项目怎么做，确认没有更好的现成做法再写，并在提交说明里写明参考了哪里。

| 问题 | 先看哪里 |
|---|---|
| 词法分析与高亮 | lazysql `components/sql_lexer.go` |
| 补全上下文：别名、CTE、子查询层级、`schema.table` | lazysql `components/sql_context.go`、`components/sql_completer.go` |
| 只读模式下识别写语句 | lazysql `drivers/validation.go` |
| 修改回写：UPDATE / INSERT / DELETE、复合主键、事务 | lazysql `drivers/utils.go`，以及 `drivers/postgres.go`、`drivers/mysql.go` 中的 `ExecutePendingChanges` |
| 单元格特殊值：NULL / EMPTY / DEFAULT | lazysql `components/set_value_list.go` |
| 查询结果流式读取、行数上限与取消 | lazysql `drivers/query_stream.go`、`drivers/query_stream_cancel_test.go` |
| 行数：估计值与精确值 | lazysql 各 driver 中的 `GetEstimatedRowCount` / `GetExactRowCount` |
| 分页取数 | lazysql 各 driver 中的 `GetRecords` |
| PG catalog：表、列、约束、索引 | pgtui `internal/db/metadata/` |
| 结果 tab：执行中显示占位，完成后替换 | pgtui `internal/ui/components/result_tabs.go` |
| 发现本地 PG 实例：pgpass、unix socket、环境变量 | pgtui `internal/db/discovery/` |
| 把密码存进系统钥匙串 | pgtui `internal/connection_history/password_store.go`（99designs/keyring） |
| 识别与查看 JSON / JSONB 单元格 | pgtui `internal/jsonb/`、`internal/ui/components/jsonb_viewer.go`；lazysql `components/json_viewer.go` |
| 导出 CSV | lazysql `helpers/csv.go`、`components/csv_export_query.go` |
| 语句切分的边界情况（psql 风格） | usql `stmt/` |
| SQL 格式化 | 已定方案：goja + sql-formatter（tech-design §9.5） |
| 模糊匹配 | 已定方案：fzf `src/algo`（tech-design §9.7） |

项目地址：[lazysql](https://github.com/jorgerojas26/lazysql)（MIT）、[pgtui](https://github.com/pgplex/pgtui)（Apache-2.0）、[usql](https://github.com/xo/usql)（MIT）。

参考之前先想清楚它们和本项目的差别，不要整段照搬：

- **UI 框架不同**：lazysql 用 tview / tcell，pgtui 用 Bubble Tea v1 和 lipgloss v1；本项目用 Bubble Tea v2 的立即模式渲染（§7）。UI 代码基本不能直接用，参考的是逻辑。
- **值的处理不同**：它们先解码成 Go 类型再转字符串；本项目统一走文本协议，值用 `db.Val{S, Null}`（§8.1）。
- **连接方式不同**：lazysql 用 `database/sql` 连接池；本项目每个 session 固定两条独占连接（§8.2）。

许可证：借鉴思路不受限制。直接拷贝代码要在文件头注明出处并保留原许可证声明；Apache-2.0 的代码还要遵守它对 NOTICE 文件的要求。
