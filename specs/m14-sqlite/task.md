# M14 SQLite 与历史 · 任务清单

- **状态**：**todo**（节点 1 F14.6、F14.2 passed 2026-10-12，646750f，全量 170 个脚本 3548 过、4 败是 f11.2 的 docker 环境问题；2026-10-12 决策者按 worker 的草稿定清单；2026-10-11 用户规划时定方向）。
- **目标**：SQLite（无 cgo）和命令行直接给 DSN / 文件，查询历史，树上的索引 / 键 / 函数 / 序列，结构修改的模板 SQL，表格的 JSON 视图，MySQL 取消的兜底。
- **范围**：tech-design §4（`internal/db/sqlite`、modernc）、§5、§7.6（JSON 视图）、§7.8（树）、§8（SQLite 的 catalog、行标识 rowid；MySQL 取消的截止时间）、§9（SQLite 方言）、§11（历史）、§14（`history.jsonl`、`[query] history_max`、命令行 DSN）、§15。
- **依赖**：M13 passed（用户 2026-10-12 定：不等验收，直接开工；验收意见进后续迭代）。
- **完成标准**：F14.1–F14.8 passed；tester 在最后一个 commit 上跑完整回归；用户验收通过后打 tag `m14`。
- **审查节点**（小的、不依赖 SQLite 的先做，worker 提议、决策者同意）：
  1. F14.2 之后：F14.6、F14.2 一批（MySQL 取消兜底、查询历史）；
  2. F14.1 之后：F14.1 单独一批（SQLite）；
  3. F14.8 之后：F14.3–F14.5、F14.7、F14.8 一批（树上的对象、结构模板、JSON 视图、主题未知键降级为警告、gqq 后的视口）。

**驱动已定**（2026-10-12，worker 量数：Go 1.26.3 darwin/arm64，CGO_ENABLED=0 -trimpath -s -w；现在 22.9 MB、冷编译 6.6s）：`modernc.org/sqlite` v1.60.1，二进制 +3.8 MB、冷编译 8.0s、7 个它自己的依赖；`ncruces/go-sqlite3` v0.35.6 是 +5.6 MB、9.5s、wasm 运行时，速度没有优势（20 万行插入 370ms 对 330ms，读 130ms 对 120ms）。F14.1 里要验证四个 goreleaser 目标交叉编译都过。

任务文件的格式和状态约定见 [`../plan.md`](../plan.md)。

---

## F14.6 MySQL 取消的兜底 · 状态：passed（646750f；e2e 01e0c93）

M13 F13.6 审查时发现：MySQL 的 Query 用 WithoutCancel 发语句，取消只能另开连接 KILL QUERY，服务端或网络不回应时 KILL 发不出去，Worker 一直锁到语句自己结束，快速 SQL 取消后 25 秒还占着 Meta。

- **依赖**：F13.6
- **涉及**：`internal/db/mysql`、`internal/db`（Worker）

**开发**（§8.3）
- [x] 给驱动一个带截止时间的 ctx：KILL 发出 5 秒后取消，驱动关掉连接，Worker 按 §8.2 断线重连；§8.3 的已知上限随之去掉。

**验收**
- [x] 集成：KILL 被拒的连接，取消后 5–7 秒内 Worker 释放、下一个请求重拨（run 和 stream 各一；reviewer 的探针只有 PG 版，MySQL 没有，worker 开工时改）。

## F14.2 查询历史 · 状态：passed（646750f；e2e 01e0c93）

- **依赖**：F8.21（query 文件）、F1.x（WHERE 历史下拉）
- **涉及**：`internal/config`（history.jsonl）、`internal/app`（query、下拉）

**开发**（§11、§14）
- [x] 每次执行（成功和失败都记；表格取数的 SQL 不记）追加到 `$XDG_DATA_HOME/sqlmux/history.jsonl`：一行 JSON：时间、连接名、schema、语句、摘要（行数或受影响行数、耗时、错误第一行）；`O_APPEND` 一次 write、不加锁（多进程安全，行小于 PIPE_BUF 的靠 O_APPEND，超长语句截到 64 KB 并记 `truncated = true`，决策者定）。
- [x] `[query] history_max = 1000`：启动时超过就截掉最旧的，临时文件加 rename 一次重写；不按天数。
- [x] 打开：INSERT 下 `C-r`（`query.history`「查询历史…」，新作用域 `[keys.queryinsert]`，§6.4；vim NORMAL 的 `C-r` 仍是 redo，不让给历史，决策者定），命令面板里也有；和 WHERE 历史同一个下拉，fzf 模糊匹配，每行语句一行化加 `dim` 的时间和摘要，`↵` 插到光标处；只列当前连接的，不做「全部连接」（决策者定，先这样）。
- [x] 细节（已定）：`vim = false` 时 `C-r` 同样是历史。

**验收**
- [x] 单测：追加一行的格式；截尾到 `history_max`；两个进程并发追加都在；超长截断带 `truncated`。
- [x] e2e：执行成功 / 失败各一条后 `C-r` 能模糊找到、`↵` 插到光标处；另一个连接的不列。

## F14.1 SQLite 与命令行 DSN · 状态：todo

- **依赖**：F14.2 之后做（节点顺序）
- **涉及**：`internal/db/sqlite`（新包，§4）、`internal/db`、`internal/sqlkit`（Dialect）、`internal/config`、`internal/app`（树、表单、启动参数）、`cmd/sqlmux`、`.goreleaser.yaml`

**开发**（§4、§8、§9、§14）
- [ ] `internal/db/sqlite`：`Engine`（同 postgres / mysql 的形状，`workspace.go` 的 `engines` 加一项）、`Conn`（Exec / Query / Stream）、catalog（`sqlite_schema`、`pragma table_info`）；行标识用主键，没有就 rowid，WITHOUT ROWID 的表必有主键；取消把 ctx 传给驱动，没有服务端可 KILL。
- [ ] `config`：engine 允许 `sqlite`，DSN 是文件路径（`~` 展开，相对路径按启动目录；`?mode=ro` 等查询参数照传给驱动）；路径不存在时报错、不新建空文件（决策者定，§8.1）；连接表单的 engine 段加 sqlite，只剩 name / group / engine / file / read_only，测试用 `sqlite_version()`。
- [ ] 树：文件 → Tables / Views → 列，没有 schema 层。
- [ ] 方言：`sqlkit.Dialect` 加 SQLite，标识符引号 `"`，格式化用 sql-formatter 的 `sqlite`，补全的关键字表共用 PG 的（决策者定）。
- [ ] 类型：SQLite 的类型是动态的，`Col.Type` 用声明类型（可能为空）；值的文本格式以集成测试为准，blob 走 §8.1 的 `<binary N bytes>`。只读文件（权限、`mode=ro`）上编辑的报错走现有错误栏，不另加提示。
- [ ] 命令行：位置参数是 DSN 或文件路径（`sqlmux ./x.db`、`sqlmux postgres://…`、`sqlmux mysql://…`），开一个临时连接：名字用文件名（不带扩展名）或 `<库>@<主机>`（连接名里不能有 `/`，worker 开工时定），不写 connections.toml、密码不存钥匙串、退出不存布局；`DATABASE_URL` 只在没有配置的连接、也没给位置参数和 `--layout` 时才认（决策者定，§14）；现有 flag 和 `sqlmux <连接名>` 照旧（连接名和路径 / URL 靠「含 `/` 或 `.`、且不是已有连接名」区分）。
- [ ] 构建：goreleaser 四个目标 CGO_ENABLED=0 都编过；二进制和编译时间的增量写进 commit 说明。
- [ ] 审查时定（§8.1、§9、§14）：CREATE TRIGGER 的 BEGIN … END 不切；手动模式下 pragma / attach / detach 不补 BEGIN；DATE / DATETIME / TIMESTAMP 列按 SQLite 标准文本显示、julianday 比较；REAL 最短往返显示、按数值比；blob 行标识整表只读；临时连接同名保留扩展名、不合规则用引擎名、管理器不列、session 用新编号的 query。

**验收**
- [ ] 集成（测试里建临时文件，不起容器，seed 用 `testdata/seed/sqlite.sql`）：连接、浏览表 / 视图 / 列、翻页、排序、WHERE、编辑（有主键 / 无主键用 rowid / WITHOUT ROWID）、事务回滚、取消长查询、导出。
- [ ] 单测：DSN 路径的 `~` 展开；路径不存在时报错；`DATABASE_URL` 只在零连接且无参数时生效；临时连接的名字（`shop@db.example`、文件名去扩展名、同名保留 `.db`、空名用引擎名）；rowid 表多一列只读 rowid；trigger 不被切开；DATETIME 列保存不报「行已变」；REAL `1/7` 保存不报「行已变」。
- [ ] e2e：`sqlmux x.db` 起来能看表、改一格存回、退出后 connections.toml 没有它；`sqlmux postgres://…` 同样。
- [ ] 四个目标交叉编译通过（`goreleaser build --snapshot` 或等价）。

## F14.3 树上的对象 · 状态：todo

- **依赖**：F14.1（三种引擎都要有）
- **涉及**：`internal/db`（catalog）、`internal/db/postgres`、`internal/db/mysql`、`internal/db/sqlite`、`internal/app`（树）、`testdata/seed`

**开发**（§7.8、§8.4）
- [ ] 表下面两个子节点：`Indexes`（叶子，`name (a, b) UNIQUE`，`↵` 开 DDL 浮窗显示那条 `create index`）和 `Keys`（叶子：主键 `PRIMARY (id)`、外键 `fk_name (a) → schema.table (b)`，外键 `↵` 跳到那张表：目标在另一个 schema 时展开那个 schema 并打开，决策者定）。
- [ ] schema 下面 `Functions`（名加参数签名，`↵` 把定义开进当前连接的新 query：PG `pg_get_functiondef`，MySQL `SHOW CREATE FUNCTION`）和 `Sequences`（叶子，`↵` 无事）。只做函数，聚合、过程、触发器不做（决策者定）；MySQL 没有 Sequences，SQLite 没有 Functions、Sequences，这些节点不画。
- [ ] catalog 查询：PG `pg_indexes` / `pg_constraint` / `pg_proc` / `information_schema.sequences`；MySQL `information_schema.statistics` / `key_column_usage` / `routines`；SQLite `pragma index_list` / `index_info` / `foreign_key_list`。按 §8.4 的方式惰性取、`R` 刷新。
- [ ] seed：三种 seed 里加索引、外键、函数、序列（PG）；改完通知 reviewer 和 tester（AGENTS.md）。
- [ ] 细节（worker 2026-10-12 开工时定，决策者同意，写在 §7.8「对象节点」和 §8.4）：分组默认收起、和列一起取；树过滤时不显示；Indexes 含主键那条（SQLite 的 autoindex 不含）；SQLite 外键没名字写 `FOREIGN KEY (a) → table (b)`；只做函数，PG 排除扩展自带的和系统 schema；引擎接口五个方法、不支持返回 nil。

**验收**
- [ ] 集成（PG、MySQL、SQLite）：每种对象的列表和定义。
- [ ] golden：展开 Indexes / Keys / Functions 的树。
- [ ] e2e：外键 `↵` 跳到目标表；Functions 里 `↵` 开出带定义的 query。

## F14.4 结构修改的模板 · 状态：todo

- **依赖**：F14.3
- **涉及**：`internal/app`（右键菜单、命令面板、模板）

**开发**（§7.8、§12）
- [ ] 表 / 视图节点的右键菜单和命令面板加「加列…」「加索引…」「改名…」「删除列…」「删除索引…」：每个把一条按引擎拼好的模板 SQL 开进当前连接的新 query（PG `alter table "s"."t" add column <name> <type>`，MySQL 反引号，SQLite 不支持的（老版本的 drop column、改类型）不列）；占位符是字面的 `<name>` / `<type>`，光标停在第一个占位符上并选中（VISUAL，按 `c` 直接替换；没有 snippet 机制，决策者定）；删除类模板直接写、不写成注释，`C-e` 走现有的 Dangerous 确认（决策者定）。不做表单（用户 2026-10-11 定）。

**验收**
- [ ] 单测：每个引擎、每个模板的文本和光标位置。
- [ ] e2e：右键菜单和命令面板各走一条，开出的 query 内容正确、光标在占位符上。

## F14.5 表格的 JSON 视图 · 状态：todo

用户 2026-10-11 验收 M12 时提：参考 DataGrip 的「查看方式」，只要 表 和 JSON 两种，不做树；JSON 是只读视图。

- **依赖**：F12.2（值编辑器的只读规则）
- **涉及**：`internal/app`（grid、result）、`internal/ui`

**开发**（§7.6）
- [ ] `grid.view.json`「以 JSON 查看」在表 tab 和结果 tab 上切换：内容区换成只读的编辑器（同值编辑器的只读规则，JSON 着色、vim 移动、`/` 搜索、`y` 复制），文字是当前页的 JSON 数组，每个对象同 `Y`（键按显示顺序、值按类型），不截断（和 `Y` 同源，决策者定）；查询条照旧，chip 行末多一段 `JSON`；再执行一次或 `esc` 回表格，光标行不同步（决策者定）；翻页、刷新、改 WHERE / ORDER 在 JSON 视图里照常，内容跟着换；转置和 JSON 视图互斥；视图按 tab 记、不进布局文件；右键菜单和命令面板里都有。M16 Redis 的「以 JSON 查看」复用它。

**验收**
- [ ] 单测：JSON 文字与 `Y` 一致；转置时切 JSON 关掉转置。
- [ ] golden：JSON 视图。
- [ ] e2e：`grid.view.json` 后内容区是 JSON、`]` 翻页内容变、`esc` 回表格。

## F14.7 主题文件的未知键降级为警告 · 状态：todo

用户 2026-10-12：brew 装的 v0.2.1 读到新版才有的 `selected` 色键直接退出（`There is no token called selected`），配置文件带着新键就不能用旧版本。config.toml、connections.toml 的未知键 M13 F13.5 已经是 toast 警告，主题文件还是启动错误，不一致。

- **依赖**：F13.5
- **涉及**：`internal/ui`（theme）、`internal/config`

**开发**（§7.7、§14）
- [ ] 主题文件里不认识的色键和 `[icon]` 名字同 F13.5：忽略它、启动后 toast「<主题文件>: 不认识的键 selected」，不阻止启动；颜色值格式错仍是启动错误。
- [ ] 细节（已定）：错误文案里不再出现 BurntSushi 的 `There is no token called`，用 F13.5 的那句。

**验收**
- [ ] 单测：主题多一个未知键能加载、警告列出键名；坏的颜色值仍报错。
- [ ] e2e：带未知键的主题启动后有 toast。

## F14.8 `gqq` 之后视口横向偏了 2 格 · 状态：todo

tester 2026-10-12 在 F14.1 e2e 时发现，6472fcd 上就有、引擎无关：query 里 `gqq` 之后左上角视口停在横向滚动了 2 格的位置，行首被盖住 2 个字符，光标移一下就恢复。

- **涉及**：`internal/app`（query）或 `internal/editor`

**开发**
- [ ] 找根因（格式化替换缓冲区后没按新光标重算横向偏移），改在共用的那一处；不加「格式化后把视口归零」的特判。

**验收**
- [ ] 单测：`gqq` 后视口左边是 0。
- [ ] e2e：`gqq` 后第一行行首完整。
