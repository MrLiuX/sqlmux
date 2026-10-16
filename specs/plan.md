# sqlmux 开发计划（索引）

范围和设计以 [`tech-design.md`](tech-design.md) 为准。每个里程碑一个目录，目录里的 `task.md` 是这个里程碑的任务清单。协作流程、审查节点怎么走、谁改什么、里程碑怎么结束，全在 [`../AGENTS.md`](../AGENTS.md)，这里不重复。

| 里程碑 | 任务清单 | 目标 | 状态 |
|---|---|---|---|
| M0 骨架 | [m0-skeleton/task.md](m0-skeleton/task.md) | 界面骨架、按键系统、鼠标、命令面板、主题；不连数据库 | 完成（tag `m0`，7c1f353） |
| M1 浏览 | [m1-browse/task.md](m1-browse/task.md) | 连接 PG，浏览 schema 与表数据，命令面板的快速 SQL；只读 | 完成（tag `m1`，fc9cfc6） |
| M2 编辑 | [m2-edit/task.md](m2-edit/task.md) | 编辑单元格、选项浮层、保存与刷新 | 完成（用户 2026-10-02 与 M3、M4 一起验收，tag `m2`） |
| M3 console | [m3-console/task.md](m3-console/task.md) | vim 编辑器、执行、底部结果区、格式化、补全、schema 下拉；表格与 console 混放 tab | 完成（用户 2026-10-02 验收，tag `m3`） |
| M4 命令面板 | [m4-palette/task.md](m4-palette/task.md) | 快速 SQL 接上 M3、DDL 预览、面板里 tab 的层级 | 完成（用户 2026-10-02 验收，tag `m4`）；验收意见排进 M5 的 F5.5–F5.9 |
| M5 工作现场 | [m5-workspace/task.md](m5-workspace/task.md) | 一个 session 多个连接、多 session / window、只读与事务模式、MySQL；先做 M4 验收意见 F5.5–F5.9 | 完成（F5.1–F5.19 passed，完整回归在 d156ec7 上全过，2026-10-05；tag `m5`） |
| M6 配置 | [m6-config/task.md](m6-config/task.md) | 键位配置收尾、主题、持久化、界面中英文切换、布局文件（保存与恢复工作现场）、可以关掉 vim 模式 | 完成（F6.1–F6.7 passed，完整回归在 be5f6f3 上全过，2026-10-06；tag `m6`） |
| M7 工程质量 | [m7-quality/task.md](m7-quality/task.md) | golangci-lint 与 CI、justfile、凭据不落明文、分字段的新建连接表单与无连接启动（用户 2026-10-05 提出） | 完成（F7.1–F7.6 passed，ec4aaad，2026-10-07；tag `m7`；CI 2026-10-10 首次在 GitHub 上跑绿） |
| M8 第一轮使用反馈 | [m8-feedback/task.md](m8-feedback/task.md) | 用户照 WALKTHROUGH 走完 M5–M7 后的 20 条意见：补全和格式化的 bug、列宽、边框样式、默认排序、window / session / console 改名、连接的修改与删除 | 完成（F8.1–F8.23 passed，0c023d2，完整回归 118 个脚本全过，2026-10-10；tag `m8`） |
| M9 第二轮使用反馈 | [m9-feedback2/task.md](m9-feedback2/task.md) | 用户 2026-10-10 在 M8 验收期间提的 10 条：WHERE 历史 / 收藏的顺手操作、表格 `^` / `C-r` / 复制为 JSON、缩放提示、DDL 查看 | 完成（F9.1–F9.10 passed，93e9d8c，完整回归 130 个脚本全过，2026-10-10；tag `m9`，`v0.1.0` 在 ffbcec5；用户 10-10 定：先打 tag，之后自己慢慢验收） |
| M10 第三轮使用反馈 | [m10-feedback3/task.md](m10-feedback3/task.md) | 用户 2026-10-10 在 M9 审查期间提的 9 条：表格多选、flash 式跳转、树去掉 pane 层、连接分组、WHERE 下拉撑满、新建 session 先要名字、console 结果 500 行并翻页、COLS 调列顺序、侧栏去掉键位文字、删开着的 console；最后 console 改名 query（单独一个节点） | 完成（F10.1–F10.12 passed，08d9009，完整回归 2982 项全过，2026-10-11；tag `m10`、`v0.2.0`；用户照 M9 的办法先打 tag 再验收） |
| M11 分发 | [m11-distribution/task.md](m11-distribution/task.md) | GoReleaser 与 release workflow、`--version`、Homebrew tap（源码编译的 formula）、npm / npx（壳包加平台子包） | todo（2026-10-10 用户定；2026-10-11 M10 passed 后开工） |
| M12 表格日常 | [m12-grid/task.md](m12-grid/task.md) | 用户 2026-10-11 对照 DataGrip 规划：多列排序、值编辑器（JSON 格式化和高亮、vim）、按单元值过滤 / 表头排序 / 表格右键菜单、导出 CSV 和 JSON | 完成（F12.1–F12.12 passed，6472fcd，2026-10-12；F12.7–F12.12 是用户 10-11 验收时的意见；tag `m12` 在 6472fcd；用户 10-12 定：照 M9 的办法先打 tag、验收意见进后续迭代） |
| M13 工程质量审计 | [m13-quality/task.md](m13-quality/task.md) | 用户 2026-10-11 让 reviewer 做的全仓审计，20 条：COMMIT 后断线报成已回滚、多进程写同一文件、确认框漏键、配置未知键静默、浮层优先级散在 12 处、扫描器平方、列表绘制和可见性重复、`C-h`、布局文件、依赖链、小拷贝 | 完成（F13.1–F13.19 passed，6472fcd，完整回归 170 个脚本 3455 过、4 败是 f11.2 的 docker 环境问题，2026-10-12；tag `m13` 在 6472fcd，同 M12） |
| M14 SQLite 与历史 | [m14-sqlite/task.md](m14-sqlite/task.md) | SQLite（无 cgo）和命令行 DSN、查询历史、树上的索引 / 键 / 函数 / 序列、结构修改的模板 SQL、表格的 JSON 视图、MySQL 取消的兜底 | 完成（F14.1–F14.8 passed，55f6d85，2026-10-12；完整回归 186 个脚本 3717 过、4 败是 f11.2 的 docker 环境问题；F14.7、F14.8 是开发中用户和 tester 发现的；用户 10-13 验收通过、五条意见进 M15 F15.5–F15.9；tag `m14`、`v0.3.0` 打在 55f6d85，推送等用户；驱动 modernc，二进制 +3.8 MB） |
| M15 PG 数据库层级 | [m15-pg-databases/task.md](m15-pg-databases/task.md) | PG 的树加数据库一层（每库一对连接）、SSH 隧道、query 的签名提示与值补全（INSERT VALUES、UPDATE SET、WHERE 的值；用户后续的编辑体验需求也归这里）、状态栏随机轮播的特性提示；用户 10-13 验收 M14 的五条（F15.5–F15.9：值编辑器和 ORDER 框宽度按内容、flash 无匹配即退出、snippet 式占位符、`gj` 与搜索高亮） | 完成（F15.1–F15.11 passed，3e4318b，2026-10-14；四批：F15.5–F15.9 0b4a268、F15.1 e056ebf、F15.3 / F15.10 / F15.11 1ae0694、F15.2 / F15.4 3e4318b；完整回归 204 个脚本 4060 条、4 败是 f11.2 的 docker 环境问题；用户 10-14 验收通过，tag `m15`（5259415）、`v0.4.0`（51481f4，产品同 3e4318b 加 formula 脚本的 commit / date），10-14 已推送，CI 绿） | |
| M16 Redis 初版 | [m16-redis/task.md](m16-redis/task.md) | 用户 2026-10-11 定：go-redis、按命名空间分的树（类型只做图标）、六种类型加 RedisJSON 的键 tab、一行一条命令的控制台、来自 `COMMAND DOCS` 的补全和签名提示；表格 tab 和数据源之间加接口缝；用户体验初版后再定第二轮（候选 M17：Pub/Sub、MONITOR、Cluster） | 完成（F16.0–F16.6、F16.8–F16.10 passed，ea91858，2026-10-15；五批：节点 1 6eda9bb、节点 2 / 3 c384691、节点 4 82ad643、节点 5 ea91858；完整回归 226 个脚本 4436 过；F16.6 是用户 10-14 提的方向键，F16.7–F16.10 是用户第一轮验收的 4 条，F16.7 光标闪烁是上游渲染器的行为、不做；用户 10-15 验收通过并要求推送，tag `m16`、`v0.5.0` 打在 c219210 之后的 docs commit 上；go-redis 二进制 +7.6 MB；用户的下一步：体验一段、完善 doc 和 README、reddit 发帖推广，再起 M17） |
| M17 体验回合与外部执行器 | [m17-polish-exec/task.md](m17-polish-exec/task.md) | v0.5.0 体验期的意见一次收掉（tab 栏滚动、`;` 跑 Redis 命令、命令行 `redis://`、三处小 bug、发布元数据、面板预览区的滚轮和焦点）；外部执行器引擎 `exec`（SQL / Redis 命令交给外部命令跑，第一个是 `doraemonctl`，只读） | draft（2026-10-16 决策者起草，等用户定；两个节点：F17.1–F17.6、F17.7–F17.8） |

## M17 候选（用户体验 v0.5.0 期间的意见；2026-10-16 已排进 m17-polish-exec/task.md，下面是原始记录）

- 2026-10-16 命令面板的 DDL 预览：鼠标滚轮在预览区上下滚（现在只有 `C-f` / `C-b`，用户自己把 `C-n` / `C-p` 也绑成了滚预览）；像切 pane 一样（用户的 `D-1`）把焦点切进预览区，当成只读视图：nvim 模式下移动、选择、复制（值编辑器 / JSON 视图那套只读编辑器）。
- 2026-10-16 快速 SQL（`;`）在 Redis 连接上跑命令：面板的 SQL 范围现在在 Redis 连接上没有反应（M16 定的「入口隐藏」），用户要 `;` 后直接打一条 Redis 命令、回复照控制台的规则摆成表；只读 / 危险判定和补全同控制台（§9.8、§13）。
- 2026-10-16 录演示时发现的小 bug（video-producer 抽帧看到，决策者没复现第二条）：新建 session 时右下角的「Connecting to sqlite…」toast 不消失，切回原 session 后还在；命令面板的 session 行写「1 windows」（英文复数）。签名提示浮层盖住光标上一行是设计（§9.8 / F15.3），不算。
- 2026-10-16 oss 做首页 README 时发现的（d39d737）：① brew formula（`.github/scripts/tap-formula.sh`）和 npm 包（`npm/sqlmux/package.json`）的描述还是「for PostgreSQL and MySQL」，要列四种引擎；② `go install …@latest` 装的 `--version` 显示 `(none, unknown)`（模块构建没有 vcs 信息），便宜就修；③ `sqlmux keys --format toml` 的注释把 F8.23、M5、§6.6 这类内部编号露给用户了，生成器里去掉（docs/keys.py 现在自己剥）；④ 命令行 `sqlmux redis://host:6379/0` 报 unknown URL，F14.1 只认 postgres:// mysql:// 和文件路径，Redis 用户第一反应就是这么试，加上。
- 2026-10-16 用户：一个 pane 开到七八个 tab 后 tab 栏放不下，后面的整个看不见（截图：第 6 个只剩 `6:list:d`，第 8 个没了）。用户给的选项：自动换行 / 翻页 / 一个按钮展开多余的 tab 列表。决策者倾向 tab 栏横向滚动、当前 tab 始终可见（同浏览器和 tmux 的 window 列表）：放不下时两端画 dim 的 `‹N` / `N›`（N 是看不见的 tab 数，可点击，各滚一屏），`gt` / `gT` / 点击 / `{N}gt` 切过去时自动滚到当前 tab，`+` 和右侧键位提示照旧；不换行（吃内容区一行）、不另开翻页态；命令面板的 `#` 范围本来就列全部 tab。M17 时写进 §7.8「tab 栏」。
- 2026-10-16 用户：公司 prod 的 PG 和 Redis 本地连不上，只有 `doraemonctl`（/Users/ctw/proj/doraemonctl）这条路：`exec db "<sql>" …` 经 opencli 在登录着的 Chrome 里借 pgAdmin 的 Query Tool 面板跑 SQL（只读账号、每条语句最多 1000 行、一次浏览器往返可批多条、`-o json` 回带列序的行和 `n` / `fetched` / `err`），`exec redis -- <cmd> <args>` 经 metaknow 的 `POST /internal/cache` 透传 redis.Do。决策者看过代码后的方案（「外部执行器」）：加引擎 `engine = "exec"`，`dialect = "postgres" | "redis"`，`command = ["doraemonctl", "exec", "db", "-o", "json", "--no-probe", "{sql}"]` 这样的 argv 模板；它实现 `db.Conn`（`Exec` / `Query`），PG 的 `Engine`（catalog、分页、`Quote` / `Arg`）照用、只换 `Connect`——`Engine.Schemas(ctx, c Conn)` 这些本来就只认 `Conn`；`Query` 的 `$n` 参数由 exec 端内联成字面量（只读连接，字面量引号自己写）；回来的 JSON 值转成文本协议的 `Val`，列类型按文本；Redis 侧 `Query(cmd, args)` 一一对应、回复 JSON 转成控制台已有的「回复摆成表」；初版只读（`read_only` 强制为 true，写回和 WATCH 不做）、不做流式导出；错误原文就是 doraemonctl 的 stderr（面板没开、trans_id 失效这些它自己会说）。每次查询一次浏览器往返，几百毫秒到一两秒，接受。不内置 pgAdmin 的 REST：那是它的内部接口，版本一变就坏，维护归 doraemonctl。
- M16 定下的第二轮候选：Pub/Sub、MONITOR、Cluster、格子里的 JSON 按格高亮、string 键是 JSON 时直接开值编辑器（见 m16-redis/task.md 末尾）。

## 发布

2026-10-10 推送：`m5`–`m9` 打在各自最后一个产品 commit 上，`v0.1.0` 打在 ffbcec5（m9 之后只多一个让 keychain 测试在 Linux 上过的 commit；93e9d8c 的首次 CI 是红的，tag 打在绿的那个），`go install` 还装不了：go.mod 的 module 是 `sqlmux` 而不是 `github.com/kackerx/sqlmux`，M10 F10.12 改掉后下一个 tag 才能 `go install github.com/kackerx/sqlmux/cmd/sqlmux@latest`；2026-10-11 M10 passed 后推送 `m10`、`v0.2.0`（de62155；M10 新功能，配置键 console → query 不兼容，所以升 minor），CI run 38096306240 绿，第一个 GitHub Release 发在 v0.2.0 上，只有源码包和说明，`go install …@v0.2.0` 验过；`kackerx/homebrew-tap` 仓库已建；`v0.2.1` 打在 b1cb1c6（M11 节点 1 passed，只推了 tag），release.yml 第一次真跑（run 38104197358）：ci、release（四个压缩包加 checksums）、tap（formula 已是 v0.2.1，Linux docker 里 brew 装出 `sqlmux v0.2.1`）都绿，`go install …@v0.2.1` 的 `--version` 是 v0.2.1；npm 红：publish 的路径不带 `./` 被 npm 当成 GitHub 简写，37206c0 修了，0.2.1 在 npm 上缺席，npm 从 `v0.2.2` 开始；F11.4 passed 后 `v0.2.2` 只在本地打在 bd6ceb1、没推（用户 10-11 定：没必要单独发，和 M12 一起发）；2026-10-12 M12、M13 passed 后 `m12`、`m13`、`v0.2.2` 都打在 6472fcd，推送等用户点头，release.yml 的 npm 修复还没在真 tag 上验过；2026-10-13 M14 验收通过后 `m14`、`v0.3.0` 打在 55f6d85（SQLite 引擎，升 minor）；用户 10-13 定：等 M15 第一批（F15.5–F15.9）通过后 main 和这些 tag 一起推，推之前再问一次；实际 worker 10-13 09:28 收到 M14 批次结论时已经推了 main（62be1a9）和 `m12` `m13` `m14` `v0.2.2`（违反流程，AGENTS.md 加了一句），四个 tag 一起推 GitHub 不触发 workflow，所以 v0.2.2 没出 Release、brew 和 npm 都还是 v0.2.1；`v0.3.0` 10-13 15:23 单独推（用户点头后），release.yml run 38362353458 全绿：Release 四个压缩包加 checksums，tap formula 到 v0.3.0（`brew upgrade` 后 `sqlmux v0.3.0 (none, unknown)`：formula 的 ldflags 只给 version，commit / date 空着，小瑕疵，下次改 .goreleaser 的 brews 模板），npm 第一次真跑成功：`sqlmux@0.3.0` 和四个 `@kackerx/sqlmux-<os>-<arch>@0.3.0`（0.0.0-stage 是 M11 占名时发的）；README 6b61ad4 写了 brew / npm 的安装；2026-10-14 M15 验收通过后 `m15` 打在 5259415、`v0.4.0` 打在 51481f4（3e4318b 加 51481f4：tap-formula.sh 多收 commit、date，brew 装的 `--version` 不再是 `(none, unknown)`），当天推送（main 和 m15 一次、v0.4.0 单独一次），main 的 CI run 38506053018 绿，release run 38506058266 结果由 worker 看。GoReleaser、Homebrew、npm 是 M11。每个里程碑的 tag 在用户验收通过后由 worker 打并推送（M5–M9 由用户 10-10 定为先打 tag 再慢慢验收）。

## 任务文件的格式

每个 `task.md` 开头写清五项：目标、范围（PRD 编号与 tech-design 章节）、依赖、完成标准、**审查节点**。之后每个 feature 一节：

- **标题行**：`## <ID> <名称> · 状态：<状态>`。状态为 passed 时，在后面注明 commit sha 和 e2e sha。
- **依赖**：必须先完成的 feature。
- **涉及**：会改动的包或文件，用来判断改动范围、避免冲突。
- **开发**：worker 按顺序实现的清单。
- **验收**：tester 的测试项，每项都要能用命令、golden 或 e2e 脚本验证。写着「e2e」的验收项里，属于布局、颜色、位置的，可以由渲染 golden 验证（`roles/tester.md`）。

**审查节点**：worker 按 feature 提交，reviewer 不逐个审，只在节点上整批审，通过后整批提测。节点由决策者按工作量定：小的里程碑只在最后一个 feature 之后设一个；大的里程碑在中间合适的位置多设几个，让一批的改动量保持在一次能审完的范围内。用户要求拉长节点、减少来回：一轮改进项或一个中等大小的里程碑只设一两个节点。

## 状态

| 状态 | 含义 |
|---|---|
| `draft` | 决策者起草，开发清单等 worker 补充 |
| `todo` | 决策者已确认，可以开工 |
| `passed（sha；e2e sha）` | 所在批次审查、测试都已通过，由决策者填写 |

不再有 doing、reviewing、testing、failed 这些中间状态：按批审查之后，它们只增加记账，不提供信息。
