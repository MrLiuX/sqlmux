# M17 体验回合与外部执行器 · 任务清单

- **状态**：**验收中，F17.14 改键待审**（2026-10-19：六个节点 F17.1–F17.14 全 passed，验收版 7d5ea0b；2026-10-18：五个节点 F17.1–F17.13 全 passed，验收版 52847da；2026-10-16 决策者起草；2026-10-17 用户定：加 F17.9 / F17.10 两条安全项，F16.11 做完就开工；设计已写进各章）。
- **目标**：两件事。一是把 v0.5.0 体验期和做 README / 演示时攒下的意见一次收掉（tab 栏溢出、`;` 跑 Redis 命令、命令行 `redis://`、几处小 bug、发布元数据、面板预览区）；二是「外部执行器」引擎：SQL 和 Redis 命令交给一个外部命令去跑，让本地连不上的库（公司 prod 的 PG 走 pgAdmin、Redis 走 metaknow，都已由 `doraemonctl` 打通）也能在 sqlmux 里浏览。
- **范围**：§7.8「tab 栏」（滚动）；§12「快速 SQL」（Redis）和「预览」（滚轮、焦点）；§14「启动」（`redis://`）、`production`、`engine = "exec"`；§13「生产连接」「保存前预览」、§10.3；§4（新包 `internal/db/exec`）、§5「连接缝」、§8.8「外部执行器」。都已写进各章（2026-10-17）。
- **依赖**：M16 passed（`db.Driver` / `db.Conn` 的接口缝、Redis 的回复摆表和判定）。
- **完成标准**：节点全过、完整回归绿；用户用 `doraemonctl` 浏览 prod 的一张表和一个键（2026-10-18 改：键那半等 metaknow 的 `/internal/cache` 改成返回结构化回复，用户定现在不改后端，不挡 M17；§8.8 已知上限）。
- **审查节点**：1 = F17.1–F17.6（体验回合，都小，一批审）；2 = F17.9 + F17.10（安全：生产连接、保存前预览）；3 = F17.7 + F17.8（外部执行器）；4 = F17.11（和 doraemonctl 对口径后的补丁，2026-10-18）；5 = F17.12 + F17.13（用户 2026-10-18 验收 prod 时提：exec 的库层、库 / schema 勾选）；6 = F17.14（用户 2026-10-19 验收时提：手动列宽）。
- **不做（留 M18 候选）**：Redis 第二轮（Pub/Sub、MONITOR、Cluster、格子里的 JSON 按格高亮、string 键是 JSON 时直接开值编辑器）；exec 引擎的写回和流式导出。

## F17.1 tab 栏横向滚动 · 状态：passed（8baeb8c；e2e 37ebc89）

- **依赖**：无
- **涉及**：`internal/app`（tab 栏绘制和命中区）、`internal/ui`
- 用户 2026-10-16：一个 pane 开到七八个 tab 后后面的整个看不见。定：横向滚动，不换行（吃内容区一行）、不另开翻页态。

**开发**（§7.8「tab 栏」）
- [ ] a. 当前 tab 始终可见：放不下时从当前 tab 往两边摆，两端画 dim 的 `‹N` / `N›`（N 是那一边看不见的 tab 数），`+` 和右侧键位提示照旧。
- [ ] b. `gt` / `gT` / `{N}gt` / 点击切换后自动滚到当前 tab；点 `‹N` / `N›` 各滚一屏；滚动位置按 pane 记，不进布局文件。

**验收**
- [ ] 单测：10 个 tab、宽度放得下 4 个时两端的 N 对；切到第 8 个后它可见；mutation。
- [ ] golden：溢出的 tab 栏一屏。
- [ ] e2e：开 8 个 tab，`8gt` 后标题可见，点 `‹N` 滚回。

## F17.2 快速 SQL 在 Redis 连接上跑命令 · 状态：passed（8baeb8c；e2e 37ebc89）

- **依赖**：F16.4
- **涉及**：`internal/app`（palette 的 SQL 范围、quicksql）
- 用户 2026-10-16：`;` 后直接打一条 Redis 命令。M16 定的「入口隐藏」改掉。

**开发**（§12「快速 SQL」、§9.8、§13）
- [ ] a. 当前连接是 Redis 时 `;` 范围的标签仍是 `;`，输入一行命令（词法同控制台 §9.8，只一条），`↵` 在 Main 上跑，回复照 §11「Redis 控制台」摆成表放进面板结果区；`C-t` 开成结果 tab、`C-e` 开成控制台 tab 照旧。
- [ ] b. 只读 / 危险判定同控制台（§13）；补全同控制台（命令名、子命令、键名、token）；不做签名提示（审查时定：面板是单行输入、SQL 的快速 SQL 也没有，不给面板新做浮层）。
- [ ] c. 历史记进 history.jsonl（`db` 字段）。

**验收**
- [ ] 单测：一条命令 → 面板结果；写命令在只读连接被挡；mutation。
- [ ] e2e：`;` `HGETALL hash:user` 两列；`;` `FLUSHDB` 弹确认；`C-e` 开出的控制台里是这一行。

## F17.3 命令行直接给 `redis://` · 状态：passed（8baeb8c；e2e 37ebc89）

- **依赖**：F16.1
- **涉及**：`cmd/sqlmux`、`internal/app`（F14.1 的临时连接）

**开发**（§14「启动」）
- [ ] a. 位置参数是 `redis://` / `rediss://` 时开一个临时 Redis 连接，名字 `<db>@<host>`（没有 db 用 `db0`），规则同 F14.1 的 URL：不写配置、不存钥匙串、不恢复 last。

**验收**
- [ ] 单测：URL → 临时连接的名字和 DSN；mutation。
- [ ] e2e：`sqlmux redis://127.0.0.1:<port>/2` 开出树。

## F17.4 三处小 bug · 状态：passed（8baeb8c；e2e 37ebc89）

- **依赖**：无
- **涉及**：`internal/app`、`internal/i18n`、`cmd/sqlmux`（keys）

**开发**
- [ ] a. 新建 session 时右下角的「Connecting to <conn>…」toast 在连上后不消失，切回原 session 还在（video-producer 2026-10-16 抽帧看到；决策者没复现，先复现）：连接完成就清掉，和首个 session 一致。
- [ ] b. 命令面板的 session 行「1 windows」：两个 i18n 键，`i18n.T("1 个窗口")` 和 `i18n.T("%d 个窗口", n)`（en.go 对应 "1 window" / "%d windows"），不加复数机制、不写只有英文的函数（审查时定：中文界面也要对）。
- [ ] c. `sqlmux keys --format toml` 的注释去掉 F8.23、M5、§6.6 这类内部编号（default.toml 的注释是给用户看的，编号留在 specs 里）；`docs/keys.py` 的剥除随之去掉（oss 改）。

**验收**
- [ ] 单测：keys 输出里 `grep -E '\((F|M)[0-9]|§'` 为空（加进现有的 keys 测试）；复数表驱动。
- [ ] e2e：新建 session 后 2s 内 toast 消失。

## F17.5 发布元数据 · 状态：passed（8baeb8c；e2e 37ebc89）

- **依赖**：无
- **涉及**：`.github/scripts/tap-formula.sh`、`npm/sqlmux/package.json`、`cmd/sqlmux/main.go`

**开发**
- [ ] a. formula 和 npm 的描述改成四种引擎（和 README 第一句一致）。
- [ ] b. `go install …@latest` 的 `--version`：没有 ldflags 时用 `debug.ReadBuildInfo` 的模块版本（`v0.5.0`）加 `vcs.revision` / `vcs.time`（有就用），都没有时只打版本不打 `(none, unknown)`。便宜就做，不便宜只做 a。
- [ ] c. npm 的五个包都带 LICENSE（MIT 要附副本）：`npm/build.mjs` 给每个包 `cpSync` 一份仓库根的 LICENSE，壳包的 `files` 加上它（agmux 的审查 2026-10-17 顺手发现）。
- [ ] d. brew formula 的 `ldflags` 加 `-s -w`：注释写着 `std_go_args` 会加，实际 brew 的 `go build` 不加，Cellar 里的二进制比 GoReleaser 的大约三成（同上来源）。

**验收**
- [ ] 单测：versionLine 对三种 BuildInfo 的输出表驱动。
- [ ] 推送后 oss 的安装验证看 `brew info` 的描述。

## F17.6 命令面板的预览区：滚轮和焦点 · 状态：passed（8baeb8c；e2e 37ebc89）

- **依赖**：F9.7
- **涉及**：`internal/app`（palette）、`internal/ui`（TextView）
- 用户 2026-10-16：预览区要能滚轮滚；要能像切 pane 一样把焦点切进预览区，当只读视图用 nvim 模式移动、选择、复制。

**开发**（§12「预览」）
- [ ] a. 滚轮在预览区上滚预览（命中区按行），在列表上滚列表。
- [ ] b. `palette.preview.focus`（默认键决策者定，候选 `C-o`；用户的 `D-1` 是他自己的 pane 键，不占）：焦点进预览区，预览换成只读编辑器（值编辑器 / JSON 视图那套：vim 移动、`/` 搜索、`v` 选择、`y` 复制），`esc` 回列表（搜索高亮亮着先清）；列表换了候选时预览重载、焦点回列表。

**验收**
- [ ] 单测：焦点在预览区时 `j` 动的是编辑器、`esc` 回列表；mutation。
- [ ] golden：焦点在预览区一屏。
- [ ] e2e：`C-o` 进预览，`"+yy` 复制一行到假 pbcopy（`yy` 只进 vim 寄存器，同 F3.38 和值编辑器 / JSON 视图；用户自己的 `Y` 映射照样可用；tester 2026-10-18 问，决策者定）。

## F17.7 外部执行器引擎（PG 方言） · 状态：passed（8baeb8c；e2e 37ebc89）

- **依赖**：F16.0（`db.Driver` / `db.Conn`）
- **涉及**：`internal/db/exec`（新包）、`internal/db/postgres`（`Connect` 可换）、`internal/config`、`internal/app`（连接表单的 engine 多一项）
- 用户 2026-10-16：prod 的 PG 只有 `doraemonctl exec db` 这条路（pgAdmin 面板、只读、每条 ≤ 1000 行、`-o json`）。决策者定：加引擎 `exec`，不内置 pgAdmin 的接口。

**开发**（§4、§5、§8「外部执行器」、§14）
- [ ] a. 配置：`engine = "exec"`、`dialect = "postgres"`、`command = ["doraemonctl", "exec", "db", "-o", "json", "--no-probe", "{sql}"]`（argv 模板，`{sql}` 被整条语句替换；多条语句多次调用）；`read_only` 强制为 true。
- [ ] b. `exec.Conn` 实现 `db.Conn`：`Query` 把 `$n` 参数内联成字面量（字符串按 PG 规则加引号、数字和布尔原样、NULL）后调用命令；`Exec` 按语句拆开逐条调用；`Close` 无事。输出的 JSON 约定写在 §8：顶层一条语句一项，`columns`（有序）、`rows`（值是 JSON 标量或嵌套，嵌套转文本）、`n`、`err`；doraemonctl 的 `-o json` 是第一个实现，字段对不上的部分由 doraemonctl 补（用户自己的工具）。
- [ ] c. `postgres.Engine` 的 catalog、分页、`Quote` / `Arg` / `Same` / `Eq` 照用，只换 `Connect`；`Info` 用 `select version()`；`Broken` / `ErrorOf` 按命令的退出码和 stderr。
- [ ] d. 取消：杀掉子进程；断线重连不适用（每次都是新进程）；exec 连接只在 connections.toml 手写、连接管理器不列（删、改名也在文件里），连通性在连接时验（`select current_database()` / `PING`），审查时定。
- [ ] e. 错误：命令的 stderr 原文进错误栏（面板没开、trans_id 失效这些 doraemonctl 自己会说）。

**验收**
- [ ] 单测：用一个假命令（脚本回固定 JSON）跑 Schemas / Tables / 分页 / Count；参数内联表驱动（引号、NULL、数字）；退出码非 0 → 错误；mutation。
- [ ] e2e：假命令当 `command`，树、表、翻页、query 都通；写语句被只读挡。
- [ ] 用户在自己机器上用 doraemonctl 浏览 prod 的一张表（这是完成标准，不进 e2e）。

## F17.8 外部执行器引擎（Redis 方言） · 状态：passed（8baeb8c；e2e 37ebc89）

- **依赖**：F17.7、F16.3
- **涉及**：`internal/db/exec`、`internal/db/redis`（键 tab 的数据源能跑在 exec 的 Conn 上）

**开发**（§8「外部执行器」）
- [ ] a. `dialect = "redis"`，`command = ["doraemonctl", "exec", "redis", "-o", "json", "--", "{cmd}", "{args...}"]`；`Query(cmd, args)` 一一对应，回复 JSON 转成控制台已有的回复形状。
- [ ] b. 树（只 SCAN，不对每个键发 TYPE：键节点用通用 `key` 图标，打开键 tab 时再 TYPE；审查时定，一条命令一次子进程）、键 tab 的读（HGETALL / LRANGE / …）、控制台都走它；写回、WATCH、阻塞命令不做（只读）；`COMMAND` 拿不到时按 §13 全按写处理。

**验收**
- [ ] 单测：假命令回 SCAN / TYPE / HGETALL 的 JSON，树和键 tab 能摆出来；mutation。
- [ ] e2e：假命令，树和一个 hash 键 tab。

## F17.9 生产连接 · 状态：passed（8baeb8c；e2e 37ebc89）

- **依赖**：无
- **涉及**：`internal/config`、`internal/app`（确认框、边框和状态栏颜色、写入口）、`internal/ui`（主题色 `production`）
- 用户 2026-10-17：reddit 上有人说数据库界面出一次错就是生产数据；补一道「生产连接」。

**开发**（§13「生产连接」、§14）
- [ ] a. `production = true`：边框、状态栏连接块、tab 标题用 `production` 色（默认同 `error`，主题可覆盖）；连接表单加一个开关。
- [ ] b. 这个连接上每一次写都先确认（不看 `confirm_dangerous`）：表格保存（走 F17.10 的预览框）、query 判为写的语句、Redis 写命令、树上的删表 / 删键 / 改名 / 设 TTL；确认框标题「<连接名> 是生产连接」。
- [ ] c. 没写 `read_only` 的生产连接默认只读；命令行 URL 的临时连接没有标记。

**验收**
- [ ] 单测：生产连接的写入口都经确认（遍历入口表）；默认只读；mutation。
- [ ] golden：生产连接的边框和状态栏一屏。
- [ ] e2e：`production = true` 上保存、`update`、删表都先问，`confirm_dangerous = false` 也问；`n` 什么都不发（抓发出的 SQL）。
- [ ] oss：README 加 Safety 一节（两层只读、危险语句确认、单事务保存加旧值检查、生产连接、保存前预览、测试在真库上跑、「不是权限边界」）。

## F17.10 保存前预览 · 状态：passed（8baeb8c；e2e 37ebc89）

- **依赖**：M13 的共用保存路径（`internal/db` 的 Save）
- **涉及**：`internal/app`（保存流程、确认框）、`internal/db`（把生成的语句和参数交出来而不是直接执行）

**开发**（§13「保存前预览」、§10.3）
- [ ] a. 保存流程拆成「生成语句」和「执行」两步；预览框列参数内联后的语句原文（一条一行，超过 20 条折叠「…另 N 条」）和「改 N 行、增 N 行、删 N 行」，`y` / `↵` 执行、`n` / `esc` 回表格、标记都还在；Redis 列命令。
- [ ] b. `[grid] preview_save = true` 默认开；关掉只剩原来的「含删除行时确认」；生产连接上关不掉。

**验收**
- [ ] 单测：预览文本和真正执行的语句一致（同一份生成结果）；折叠；mutation。
- [ ] golden：预览框一屏（含 UPDATE、INSERT、DELETE 各一条）。
- [ ] e2e：改两格删一行 `C-s` → 预览三行和「改 2 行、删 1 行」，`n` 不发任何写语句，`y` 后库里对；Redis 的 hash 保存预览 `HSET` / `HDEL`。

## F17.11 exec 认 `truncated`、示例 argv 对齐 doraemonctl · 状态：passed（631a299；e2e d4457e1）

- **依赖**：F17.7
- **涉及**：`internal/db/exec`、§8.8 / §14 的示例
- 2026-10-18 和 doraemonctl 的会话对口径（它加 `-o sqlmux`）：退出码「数组出得来就是 0、逐条错在 err 里」和现有实现一致；行可能被命令截断、带 `truncated: true`。

**开发**（§8.8）
- [x] a. 一项带 `truncated: true` 时：exec 的 Conn 把这条语句包成 `select * from (<sql>) as sqlmux_page limit 1000 offset <已收行数>` 接着跑，页长取第一批被截时的行数，页不满或空页才停、一页和第一批一样长就接着翻（后续页不看 `truncated`，doraemonctl 对正好 `--max-rows` 行的页不带它），行拼成一份（上限 100000 行，超过标成已截断）；决策者在 staging 实测 `agentable` 有 4144 张表，不翻的话 `public` 一张都列不出。用户 query 的语句（Exec 那条路）不翻，标已截断照 AutoLimit 提示。多出来的字段忽略（现在就是）。
- [x] b. 配置检查和 i18n 文案里的示例 argv 改成 §14 的（`-o sqlmux`、`--max-rows 1000`，redis `--no-parse`，不带 `--no-probe`）。

**验收**
- [x] 单测：假命令每次最多回 3 行、只有第一批标 `truncated`，`Query` 拼出全部 7 行且发出的语句带 `limit 3 offset 3` / `offset 6`；`Exec` 不翻、标已截断；没有这个字段不变；mutation。
- [x] e2e：假命令每批 1000 行截断，4000 张表的树全部列出；query 里的截断提示出现。
- [x] 决策者用 doraemonctl 在 staging 上看 `public` 的表列齐（真路径，不进 e2e）。

## F17.12 exec 连接的数据库层 · 状态：passed（52847da；e2e 5676885）

- **依赖**：F17.7
- **涉及**：`internal/db/exec`、`internal/app`（树）
- 用户 2026-10-18 验收 prod-pg：PG 的第一级应该是库、第二级才是 schema，exec 连接也要统一。doraemonctl 的 `exec db` 只到 `doraemon` 一个库（探针写死），所以列一个。

**开发**（§7.8「数据库节点」、§8.8）
- [x] a. `exec.Engine.Databases` 返回连接时 `select current_database()` 的那一个名字；树上是一个数据库节点，默认展开，它是默认库、走连接本身的 Conn（F15.1 的「DSN 里的库」那条路），表 tab 标题不带库名。

**验收**
- [x] 单测：假命令回 `current_database()` = `doraemon` → `Databases` 是 `["doraemon"]`；mutation。
- [x] e2e：假命令的 exec 连接树是 连接 → doraemon → schema → Tables，query 标题 `doraemon.public`；Redis 方言不变。

## F17.13 库 / schema 的勾选 · 状态：passed（52847da；e2e 5676885）

- **依赖**：F15.1（数据库层）、F10.3（COLS 浮层）
- **涉及**：`internal/app`（树、右键菜单、面板命令、浮层）、`internal/config`（两个字段、写回）、`internal/db/postgres` / `mysql`（catalog 查询按 schema 过滤）、i18n
- 用户 2026-10-18 验收时提（截图是 DataGrip 数据源的「架构」）：连接和库上要有右键菜单，勾选显示和加载哪些库 / schema，节点尾注 `2/7`。prod 的 `agentable` 有 2428 张表，不勾就不拉。

**开发**（§7.8「库 / schema 的勾选」「右键菜单」、§8.3、§14）
- [x] a. 配置：connections.toml 的 `databases`（PG）、`schemas`（PG 写 `库.schema`，MySQL 写名字）；没写 = 全部；DSN 里的库固定勾选。浮层关闭时有改动就写回（只改这条连接的这两个字段，保留文件里其他内容和注释，同 `AppendConnection` / `RenameConnection` 的写法）。
- [x] b. catalog：`Databases` 列表按 `databases` 过滤后再上树（列表本身仍是全部）；Tables / Views / Functions / Sequences 的查询加 `n.nspname in (…)`，MySQL 同样按 schema 过滤；没勾的库不拨号。
- [x] c. 浮层：复用 COLS 的列表浮层和 `[keys.cols]` 的键，标题「库」/「schema」，每行 `[x] 名字`，schema 行尾注已加载的表数（没加载的空）；`esc` 关闭，有改动才写回并重载这个库的 catalog（连接节点的「选择库…」重载整个连接）。
- [x] d. 入口：树的右键菜单（连接节点：PG「选择库…」、MySQL「选择 schema…」；PG 数据库节点「选择 schema…」）和命令面板的同名命令（作用于光标所在节点）；Action `tree.pick`，默认不绑键。
- [x] e. 尾注：收窄了的连接 / 库节点名后面 `dim` 的 `勾了的/总数`，放不下不画。query 的 schema 下拉、补全、面板 `@` 范围只认加载的。

**验收**
- [x] 单测：配置的读写（往返保留其他字段和注释）；catalog 查询带 `in (…)`（PG 和 MySQL 集成测试各一条：勾一个 schema 只回它的表）；mutation。
- [x] e2e：PG 连接 `D` 上右键「选择库…」只勾 doraemon → 树只剩它、尾注 `1/N`；doraemon 上「选择 schema…」只勾 public → agentable 不列、面板 `@` 搜不到它的表、query 下拉只有 public；connections.toml 里出现两行；重启后还是这样；取消勾选回到全部、尾注消失；MySQL 连接上「选择 schema…」同样；exec 的假命令连接上勾 schema 后 Tables 查询带 `in`。

## F17.14 手动列宽 · 状态：passed（7d5ea0b；e2e 8a867a3；e 的改键待审）

- **依赖**：F10.3（`order_cols` 的存法）
- **涉及**：`internal/ui`（列宽计算）、`internal/app`（表格键、表头拖动、布局文件）、`internal/keymap/default.toml`、i18n
- 用户 2026-10-19 验收时提：一条记录差一点点看不全，终端拖不了列宽；DataGrip 是拖单列。要一个单列加宽 / 收窄的映射，鼠标拖拽更好（不强求）。

**开发**（§7.8「列宽」3、§6.4、§14）
- [x] a. `grid.col.wider` / `grid.col.narrower`（默认 `C-w >` / `C-w <`，次数前缀，一次 1 列；e 改成 `>` / `<`）：光标所在列定为当前显示宽度 ± N，定宽列不参与比例收缩、不随内容变、不受 40 上限；最窄 1 列。`grid.col.fit`（`C-w =`）清掉这个 tab 的全部定宽。
- [x] b. 定宽按列名记在 tab 上，布局文件 `col_widths = {列名 = 宽}`，同 `order_cols` 的读写；COLS 隐藏再恢复不丢；转置视图不用；结果 tab 同样。
- [x] c. 鼠标：表头行上两列之间的分隔符按住左右拖，松开时左边那列定宽为拖到的宽度（复用 pane 边界拖动的按下 / 移动 / 松开），拖到 1 列为止；行号列不能拖。
- [ ] e. 改默认键（用户 2026-10-19 试过后定）：`>` = `grid.col.wider`、`<` = `grid.col.narrower`、`=` = `grid.col.fit`，`C-w` 那三个不留；`default.toml`、键位帮助、docs/keys.md 跟着（docs 归 oss，发版前重生成）。e2e 的 f17.14 / f17.14b 换键。
- [x] d. 修复（tester 2026-10-19 发现）：结果 tab（快速 SQL `C-t` 开的、query 的结果）的表头分隔符拖不动，`internal/ui/grid.go` 只在 `Heads` 为真时画拖动区，而只有表格 tab 设了它。§7.8 写的是「结果 tab 同样」：结果 tab 也要能拖（表头点击排序照旧只在表格 tab）。e2e 补一条：结果 tab 上拖 5 列。

**验收**
- [x] 单测：列宽计算：定宽列原样、其余按原规则收缩；`3 C-w >` 加 3；收到 1 为止；`C-w =` 清空；布局文件往返；mutation。
- [x] e2e：`t_order` 上光标停在一列，`C-w >` 三次该列宽 3 列、别的列重算；`3C-w <` 回去；关掉再开这个 tab 宽度还在，重启也在；`C-w =` 回自动；表头分隔符拖 5 列后该列宽 5 列；转置视图不受影响；COLS 隐藏再恢复宽度还在。

任务文件的格式和状态约定见 [`../plan.md`](../plan.md)。
