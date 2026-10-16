# M17 体验回合与外部执行器 · 任务清单

- **状态**：**draft**（2026-10-16 决策者起草，等用户定清单；定了改 todo，设计章节随之改）。
- **目标**：两件事。一是把 v0.5.0 体验期和做 README / 演示时攒下的意见一次收掉（tab 栏溢出、`;` 跑 Redis 命令、命令行 `redis://`、几处小 bug、发布元数据、面板预览区）；二是「外部执行器」引擎：SQL 和 Redis 命令交给一个外部命令去跑，让本地连不上的库（公司 prod 的 PG 走 pgAdmin、Redis 走 metaknow，都已由 `doraemonctl` 打通）也能在 sqlmux 里浏览。
- **范围**：§7.8「tab 栏」（滚动）；§12「快速 SQL」（Redis）和「预览」（滚轮、焦点）；§14「启动」（`redis://`）；§4（新包 `internal/db/exec`）、§5「连接缝」（exec 的 `db.Conn`）、§8 新一节「外部执行器」、§14 的 `engine = "exec"`。设计在用户定清单后写进各章，再开工。
- **依赖**：M16 passed（`db.Driver` / `db.Conn` 的接口缝、Redis 的回复摆表和判定）。
- **完成标准**：节点全过、完整回归绿；用户用 `doraemonctl` 浏览 prod 的一张表和一个键。
- **审查节点**：1 = F17.1–F17.6（体验回合，都小，一批审）；2 = F17.7 + F17.8（外部执行器）。
- **不做（留 M18 候选）**：Redis 第二轮（Pub/Sub、MONITOR、Cluster、格子里的 JSON 按格高亮、string 键是 JSON 时直接开值编辑器）；exec 引擎的写回和流式导出。

## F17.1 tab 栏横向滚动 · 状态：draft

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

## F17.2 快速 SQL 在 Redis 连接上跑命令 · 状态：draft

- **依赖**：F16.4
- **涉及**：`internal/app`（palette 的 SQL 范围、quicksql）
- 用户 2026-10-16：`;` 后直接打一条 Redis 命令。M16 定的「入口隐藏」改掉。

**开发**（§12「快速 SQL」、§9.8、§13）
- [ ] a. 当前连接是 Redis 时 `;` 范围的标签仍是 `;`，输入一行命令（词法同控制台 §9.8，只一条），`↵` 在 Main 上跑，回复照 §11「Redis 控制台」摆成表放进面板结果区；`C-t` 开成结果 tab、`C-e` 开成控制台 tab 照旧。
- [ ] b. 只读 / 危险判定同控制台（§13）；补全和签名提示同控制台（命令名、子命令、键名、token）。
- [ ] c. 历史记进 history.jsonl（`db` 字段）。

**验收**
- [ ] 单测：一条命令 → 面板结果；写命令在只读连接被挡；mutation。
- [ ] e2e：`;` `HGETALL hash:user` 两列；`;` `FLUSHDB` 弹确认；`C-e` 开出的控制台里是这一行。

## F17.3 命令行直接给 `redis://` · 状态：draft

- **依赖**：F16.1
- **涉及**：`cmd/sqlmux`、`internal/app`（F14.1 的临时连接）

**开发**（§14「启动」）
- [ ] a. 位置参数是 `redis://` / `rediss://` 时开一个临时 Redis 连接，名字 `<db>@<host>`（没有 db 用 `db0`），规则同 F14.1 的 URL：不写配置、不存钥匙串、不恢复 last。

**验收**
- [ ] 单测：URL → 临时连接的名字和 DSN；mutation。
- [ ] e2e：`sqlmux redis://127.0.0.1:<port>/2` 开出树。

## F17.4 三处小 bug · 状态：draft

- **依赖**：无
- **涉及**：`internal/app`、`internal/i18n`、`cmd/sqlmux`（keys）

**开发**
- [ ] a. 新建 session 时右下角的「Connecting to <conn>…」toast 在连上后不消失，切回原 session 还在（video-producer 2026-10-16 抽帧看到；决策者没复现，先复现）：连接完成就清掉，和首个 session 一致。
- [ ] b. 命令面板的 session 行「1 windows」：用 `i18n.N` 的单复数。
- [ ] c. `sqlmux keys --format toml` 的注释去掉 F8.23、M5、§6.6 这类内部编号（default.toml 的注释是给用户看的，编号留在 specs 里）；`docs/keys.py` 的剥除随之去掉（oss 改）。

**验收**
- [ ] 单测：keys 输出里 `grep -E '\((F|M)[0-9]|§'` 为空（加进现有的 keys 测试）；复数表驱动。
- [ ] e2e：新建 session 后 2s 内 toast 消失。

## F17.5 发布元数据 · 状态：draft

- **依赖**：无
- **涉及**：`.github/scripts/tap-formula.sh`、`npm/sqlmux/package.json`、`cmd/sqlmux/main.go`

**开发**
- [ ] a. formula 和 npm 的描述改成四种引擎（和 README 第一句一致）。
- [ ] b. `go install …@latest` 的 `--version`：没有 ldflags 时用 `debug.ReadBuildInfo` 的模块版本（`v0.5.0`）加 `vcs.revision` / `vcs.time`（有就用），都没有时只打版本不打 `(none, unknown)`。便宜就做，不便宜只做 a。

**验收**
- [ ] 单测：versionLine 对三种 BuildInfo 的输出表驱动。
- [ ] 推送后 oss 的安装验证看 `brew info` 的描述。

## F17.6 命令面板的预览区：滚轮和焦点 · 状态：draft

- **依赖**：F9.7
- **涉及**：`internal/app`（palette）、`internal/ui`（TextView）
- 用户 2026-10-16：预览区要能滚轮滚；要能像切 pane 一样把焦点切进预览区，当只读视图用 nvim 模式移动、选择、复制。

**开发**（§12「预览」）
- [ ] a. 滚轮在预览区上滚预览（命中区按行），在列表上滚列表。
- [ ] b. `palette.preview.focus`（默认键决策者定，候选 `C-o`；用户的 `D-1` 是他自己的 pane 键，不占）：焦点进预览区，预览换成只读编辑器（值编辑器 / JSON 视图那套：vim 移动、`/` 搜索、`v` 选择、`y` 复制），`esc` 回列表（搜索高亮亮着先清）；列表换了候选时预览重载、焦点回列表。

**验收**
- [ ] 单测：焦点在预览区时 `j` 动的是编辑器、`esc` 回列表；mutation。
- [ ] golden：焦点在预览区一屏。
- [ ] e2e：`C-o` 进预览，`yy` 复制一行到假 pbcopy。

## F17.7 外部执行器引擎（PG 方言） · 状态：draft

- **依赖**：F16.0（`db.Driver` / `db.Conn`）
- **涉及**：`internal/db/exec`（新包）、`internal/db/postgres`（`Connect` 可换）、`internal/config`、`internal/app`（连接表单的 engine 多一项）
- 用户 2026-10-16：prod 的 PG 只有 `doraemonctl exec db` 这条路（pgAdmin 面板、只读、每条 ≤ 1000 行、`-o json`）。决策者定：加引擎 `exec`，不内置 pgAdmin 的接口。

**开发**（§4、§5、§8「外部执行器」、§14）
- [ ] a. 配置：`engine = "exec"`、`dialect = "postgres"`、`command = ["doraemonctl", "exec", "db", "-o", "json", "--no-probe", "{sql}"]`（argv 模板，`{sql}` 被整条语句替换；多条语句多次调用）；`read_only` 强制为 true。
- [ ] b. `exec.Conn` 实现 `db.Conn`：`Query` 把 `$n` 参数内联成字面量（字符串按 PG 规则加引号、数字和布尔原样、NULL）后调用命令；`Exec` 按语句拆开逐条调用；`Close` 无事。输出的 JSON 约定写在 §8：顶层一条语句一项，`columns`（有序）、`rows`（值是 JSON 标量或嵌套，嵌套转文本）、`n`、`err`；doraemonctl 的 `-o json` 是第一个实现，字段对不上的部分由 doraemonctl 补（用户自己的工具）。
- [ ] c. `postgres.Engine` 的 catalog、分页、`Quote` / `Arg` / `Same` / `Eq` 照用，只换 `Connect`；`Info` 用 `select version()`；`Broken` / `ErrorOf` 按命令的退出码和 stderr。
- [ ] d. 取消：杀掉子进程；断线重连不适用（每次都是新进程）；表单的「测试」跑 `select 1`。
- [ ] e. 错误：命令的 stderr 原文进错误栏（面板没开、trans_id 失效这些 doraemonctl 自己会说）。

**验收**
- [ ] 单测：用一个假命令（脚本回固定 JSON）跑 Schemas / Tables / 分页 / Count；参数内联表驱动（引号、NULL、数字）；退出码非 0 → 错误；mutation。
- [ ] e2e：假命令当 `command`，树、表、翻页、query 都通；写语句被只读挡。
- [ ] 用户在自己机器上用 doraemonctl 浏览 prod 的一张表（这是完成标准，不进 e2e）。

## F17.8 外部执行器引擎（Redis 方言） · 状态：draft

- **依赖**：F17.7、F16.3
- **涉及**：`internal/db/exec`、`internal/db/redis`（键 tab 的数据源能跑在 exec 的 Conn 上）

**开发**（§8「外部执行器」）
- [ ] a. `dialect = "redis"`，`command = ["doraemonctl", "exec", "redis", "-o", "json", "--", "{cmd}", "{args...}"]`；`Query(cmd, args)` 一一对应，回复 JSON 转成控制台已有的回复形状。
- [ ] b. 树（SCAN / TYPE）、键 tab 的读（HGETALL / LRANGE / …）、控制台都走它；写回、WATCH、阻塞命令不做（只读）；`COMMAND` 拿不到时按 §13 全按写处理。

**验收**
- [ ] 单测：假命令回 SCAN / TYPE / HGETALL 的 JSON，树和键 tab 能摆出来；mutation。
- [ ] e2e：假命令，树和一个 hash 键 tab。

任务文件的格式和状态约定见 [`../plan.md`](../plan.md)。
