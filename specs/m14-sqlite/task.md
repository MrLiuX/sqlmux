# M14 SQLite 与历史 · 任务清单

- **状态**：**draft**（2026-10-11 用户规划时定方向；开发清单在 M13 开工后由决策者补成 todo）。
- **目标**：SQLite（无 cgo）和命令行直接给 DSN / 文件，查询历史，树上的索引 / 键 / 函数 / 序列，结构修改的模板 SQL。
- **范围**：tech-design §4（`internal/db/sqlite`）、§5、§7.8（树）、§8（SQLite 的 catalog、行标识 rowid）、§11（历史）、§14（`history.jsonl`、`[query] history_max`）、§15。
- **依赖**：M13 passed。
- **审查节点**：
  1. F14.1 之后：F14.1 单独一批（SQLite）；
  2. F14.4 之后：F14.2–F14.4 一批。

已定的方向（2026-10-11，用户同意）：

- **F14.1 SQLite 与命令行 DSN**：驱动 `modernc.org/sqlite`（纯 Go，`CGO_ENABLED=0`），备选 `ncruces/go-sqlite3`，开工前 worker 量二进制大小和编译时间再定。`sqlmux ./x.db`、`sqlmux postgres://…` / `mysql://…` 开一个临时连接（不写 connections.toml、退出不存布局）；认 `DATABASE_URL`。树：文件 → Tables / Views → 列，没有 schema 层；行标识用主键，没有就 rowid；编辑、query、快速 SQL 走现有路径。
- **F14.2 查询历史**：每次执行（成功和失败都记）追加到 `~/.local/share/sqlmux/history.jsonl`：时间、连接、schema、语句、结果摘要；query 里 `C-r` 打开，和 WHERE 历史同一个下拉，fzf 模糊匹配，`↵` 插到光标处；`[query] history_max = 1000`，启动时截掉最旧的，不按天数。
- **F14.3 树上的对象**：表下面 `Indexes`（名、列、UNIQUE，`↵` 开 DDL 浮窗）和 `Keys`（主键、外键，外键显示指向的表，`↵` 跳到那张表）；schema 下面 `Functions`（名加参数签名，`↵` 把定义开进 query）和 `Sequences`。运算符、类型、扩展不做。
- **F14.5 表格的 JSON 视图**（用户 2026-10-11 验收 M12 时提：参考 DataGrip 的「查看方式」，只要 表 和 JSON 两种，不做树；JSON 是只读视图）：`grid.view.json`「以 JSON 查看」在表 tab 和结果 tab 上切换：内容区换成只读的编辑器（同值编辑器的只读规则，JSON 着色、vim 移动、`/` 搜索、`y` 复制），文字是当前页的 JSON 数组，每个对象同 `Y`（键按显示顺序、值按类型）；查询条照旧，chip 行末多一段 `JSON`；再执行一次或 `esc` 回表格；翻页、刷新、改 WHERE / ORDER 在 JSON 视图里照常，内容跟着换；转置和 JSON 视图互斥（开一个关另一个）；视图按 tab 记、不进布局文件；右键菜单和命令面板里都有。M16 Redis 的「以 JSON 查看」复用它。
- **F14.4 结构修改的模板**：右键菜单和命令面板加「加列…」「加索引…」「改名…」「删除列 / 索引…」，每个把一条模板 SQL 开进当前连接的新 query，光标停在要填的位置，`C-e` 执行；不做表单（用户 2026-10-11 定：编辑表单先不做）。

任务文件的格式和状态约定见 [`../plan.md`](../plan.md)。
