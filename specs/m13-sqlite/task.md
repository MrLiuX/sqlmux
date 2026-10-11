# M13 SQLite 与历史 · 任务清单

- **状态**：**draft**（2026-10-11 用户规划时定方向；开发清单在 M12 开工后由决策者补成 todo）。
- **目标**：SQLite（无 cgo）和命令行直接给 DSN / 文件，查询历史，树上的索引 / 键 / 函数 / 序列，结构修改的模板 SQL，query 的上下文提示与值补全。
- **范围**：tech-design §4（`internal/db/sqlite`）、§5、§7.8（树）、§8（SQLite 的 catalog、行标识 rowid）、§11（历史）、§14（`history.jsonl`、`[query] history_max`）、§15。
- **依赖**：M12 passed。
- **审查节点**：
  1. F13.1 之后：F13.1 单独一批（SQLite）；
  2. F13.5 之后：F13.2–F13.5 一批（F13.5 可以按量拆到 M14）。

已定的方向（2026-10-11，用户同意）：

- **F13.1 SQLite 与命令行 DSN**：驱动 `modernc.org/sqlite`（纯 Go，`CGO_ENABLED=0`），备选 `ncruces/go-sqlite3`，开工前 worker 量二进制大小和编译时间再定。`sqlmux ./x.db`、`sqlmux postgres://…` / `mysql://…` 开一个临时连接（不写 connections.toml、退出不存布局）；认 `DATABASE_URL`。树：文件 → Tables / Views → 列，没有 schema 层；行标识用主键，没有就 rowid；编辑、query、快速 SQL 走现有路径。
- **F13.2 查询历史**：每次执行（成功和失败都记）追加到 `~/.local/share/sqlmux/history.jsonl`：时间、连接、schema、语句、结果摘要；query 里 `C-r` 打开，和 WHERE 历史同一个下拉，fzf 模糊匹配，`↵` 插到光标处；`[query] history_max = 1000`，启动时截掉最旧的，不按天数。
- **F13.3 树上的对象**：表下面 `Indexes`（名、列、UNIQUE，`↵` 开 DDL 浮窗）和 `Keys`（主键、外键，外键显示指向的表，`↵` 跳到那张表）；schema 下面 `Functions`（名加参数签名，`↵` 把定义开进 query）和 `Sequences`。运算符、类型、扩展不做。
- **F13.5 query 的编辑体验：上下文提示与值补全**（用户 2026-10-11：INSERT 的 VALUES 里不知道第几个值对应哪个字段、什么类型，DataGrip 有字段名 hint；同类的编辑体验优化都归到这一条，往后排）。做法定为「签名提示加值补全」，不做行内虚拟文本（列号映射的代价，以后单独评估）：
  - **签名提示**：光标在某个「列上下文」里时，光标所在行的上一行（第一行时是下一行）画一条浮层，列出相关字段和类型，当前字段高亮、两边变暗，放不下只显示当前字段前后几个、两头 `…`；光标移动跟着换，出了上下文就消失，不用唤起也不用关；`[query] hints = false` 关掉。上下文：`INSERT … VALUES (…)`（按语句里的列清单，没写列清单按 catalog 的列顺序；第几个值按括号深度 1 的逗号数，字符串和注释里的不算，多行 VALUES 每个括号重新计数；`INSERT … SELECT` 不提示）；`UPDATE … SET col = `（这一列的名字和类型）；`WHERE` / `ON` / `HAVING` 里 `col = ` / `IN (` 的右边（这一列）；用户定义的函数的调用括号里（名字加参数签名，来自 catalog，和 F13.3 的 Functions 节点同一份数据；内置函数不做，没有 catalog）。
  - **值补全**：在上面这些位置弹补全：枚举列列出枚举值（带引号）、布尔列 `true` / `false`、可空列 `NULL`、有默认值的列 `DEFAULT`（只在 INSERT / UPDATE 里）、时间列 `now()` / `CURRENT_TIMESTAMP`，和单元格编辑的选项浮层用同一份 catalog 数据；其余列照旧不弹。
  - **同类小项**（一起做）：`INSERT INTO t ` 后补全时提供 `(全部列) VALUES ()` 的骨架（光标停在括号里）；`UPDATE t SET` 后的列补全只列这张表的列；补全插入的标识符需要引号时加引号（去掉 §12「已知上限」里的那一条）。
  - 候选但没定（用户补充后加）：`SELECT` 列表里 `*` 展开成全部列名；悬停或 `K` 看光标下标识符的类型和注释。

- **F13.4 结构修改的模板**：右键菜单和命令面板加「加列…」「加索引…」「改名…」「删除列 / 索引…」，每个把一条模板 SQL 开进当前连接的新 query，光标停在要填的位置，`C-e` 执行；不做表单（用户 2026-10-11 定：编辑表单先不做）。

任务文件的格式和状态约定见 [`../plan.md`](../plan.md)。
