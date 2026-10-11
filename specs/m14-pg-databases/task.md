# M14 PG 数据库层级 · 任务清单

- **状态**：**draft**（2026-10-11 用户规划时定方向，排在 M13 之后）。
- **目标**：PG 的树从 连接 → schema 改成 连接 → 数据库 → schema，同 DataGrip；顺带 SSH 隧道。
- **范围**：tech-design §5（`Conn` 按数据库分 Meta / Main）、§7.8（树）、§8（`pg_database`）、§14（connections.toml 的 ssh 字段）。
- **依赖**：M13 passed。
- **审查节点**：待定。

已定的方向（2026-10-11，用户同意）：

- **F14.1 数据库层级**：PG 一个连接只能看一个库，所以每个数据库一对 `Meta` / `Main`，展开时才连；DSN 里的库默认展开；列表来自 `pg_database`（排除模板）。MySQL 不变（它的 database 就是 schema 这一层），SQLite 没有这一层。这是 M5 以来最大的模型改动，单独一个里程碑。
- **F14.2 SSH 隧道**：connections.toml 的 `ssh = "user@host"`（走本机的 ssh agent 和 `~/.ssh/config`），连接时先开隧道。终端用户多数自己 `ssh -L`，优先级低于 F14.1。

任务文件的格式和状态约定见 [`../plan.md`](../plan.md)。
