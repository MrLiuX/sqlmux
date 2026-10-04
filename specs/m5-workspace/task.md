# M5 工作现场与 MySQL · 任务清单

- **状态**：**验收中，做用户的验收意见 F5.12–F5.17**（2026-10-04 定，用户说这几点做完再接着验收），之后验收通过再打 tag `m5`。F5.1–F5.11 全部 passed，完整回归在 91fc2c1 上全过（70 个脚本 1874 项，e2e a5539b2；集成测试在 PG 和 MySQL 上都过）。按 F5.5–F5.9 → F5.10 → F5.11 → F5.1 → F5.2 → F5.3 → F5.4 的顺序做，四个审查节点 2026-10-02 / 10-03 依次通过。
- **目标**：
  - 一个 session 多个连接：目录树的根是 connections.toml 里的全部连接，用到时才连；
  - 支持多个 session、多个 window；
  - 支持只读连接和两种事务模式；
  - 支持 MySQL。
- **范围**：
  - PRD：S-01~S-04、W-01~W-02、第 9 章中的 MySQL
  - tech-design：§5、§7.8（树、console 标题、状态栏、session 列表、新建连接表单）、§8、§10.1、§10.3、§10.5、§11–§14
- **依赖**：M1–M4 全部 passed。
- **设计变更**（M2/M3 验收时用户提出，2026-10-02 定，已写进 §5「一个 session 多个连接」）：
  - session 变成工作区，每个 session 能看到 connections.toml 里的全部连接，目录树的根是这些连接，用到时才连；多个 session 照样保留。
  - console 复用所属连接的 `Main`（用户先选了每个 console 一条连接，随后改为复用：更简单、不容易出错）；console 标题上可以选连接和 schema，所在连接有未提交的事务时不能换连接。
  - 事务模式按连接：每个 console 标题上 `Tx: 自动 ▾`，手动时有提交 ✓、回滚 ↺（照 DataGrip，用户给的截图）；同一个连接上的 console 和表格保存共用这个事务。
  - 只读按连接：connections.toml 的 `read_only = true`。
- **完成标准**：
  - F5.1–F5.17 全部 passed；
  - 前面各阶段的集成测试在 MySQL 上也全部通过；
  - 用户验收通过后，打 tag `m5`。
- **审查节点**：
  0. F5.9 之后：F5.5–F5.9，M4 验收意见，先做；
  1. F5.11 之后：F5.10、F5.11，连接模型。几乎所有用到 `a.sess` 的地方都要改，后面几个 feature 都建在它上面，先单独审；
  2. F5.3 之后：F5.1–F5.3，多 session、多 window、只读与事务模式；
  3. F5.4 之后：MySQL 单独一批；
  4. F5.17 之后：F5.12–F5.17，用户的 M5 验收意见，一批。

任务文件的格式和状态约定见 [`../plan.md`](../plan.md)。

---

## F5.10 连接模型与连接树 · 状态：passed（1155504；e2e e9c906c）

- **依赖**：F5.5–F5.9
- **涉及**：`internal/app`（Session / Conn、树、状态栏、面板、快速 SQL）、`internal/config`（connections、state）、`internal/ui`（树、状态栏）

**开发**
- [x] 拆 `app.Session`（§5）：
  - `Session`（工作区）保留 Name、Windows、Active、RunSeq，加上 `Conns`；
  - 新增 `Conn`，放 Spec、State、Meta、Main、Schema、Schemas、Tables、home，列 / 存在性 / DDL 三个缓存，DB、Addr，以及 search_path 的记账 startedPath、mainPath；
  - 树的展开状态仍记在 Session 上，节点 ID 加上连接名前缀。
- [x] `config.LoadConnections` 读出全部连接，名字重复时启动报错。`sqlmux [名字]` 选启动时连哪一个（不带参数时连第一个），照旧同步建连，失败照 §14 退出；其他连接都不连。
- [x] 树的根是各个连接（§7.8「连接节点」）：未连接、连接中、失败、已连接四种状态。在未连接或失败的根上展开、`↵` 或单击时，在 Cmd 里建连（`password_cmd` 也在 Cmd 里跑），连上后加载 catalog，照「默认展开」展开它的 `current_schema()` 和 Tables。
- [x] 表 tab 属于一个 Conn：tab 的 id 带上连接名，两个连接上的同名表是不同的 tab、各有各的缓存。state.json 的表键本来就是 `<连接>/<schema>.<表>`，不用改；`recent` 里表的 id 加上连接名，旧的不带连接名的条目读到时丢掉（§14）。
- [x] 「当前连接」（§5）：焦点 tab 的连接，结果 tab 取它记下的来源连接；焦点在树上时取光标所在的连接；引导页、空 pane 沿用上一次的。用到它的有：快速 SQL（`Meta` 加它的 Schema，历史照旧按连接记）、`C-e`、新 console、`tree.refresh`、`C-c` 和 busy 提示。DDL 预览取表所属连接的 `Meta`。`quick #n` 的结果 tab 记下连接和 schema，`R` 用它们重跑。
- [x] 命令面板的表范围列出所有已连上的连接的表，所在位置是 `<连接>.<schema>`；当前连接的 Schema 排最前，然后是当前连接的其余 schema，再是其他连接（§12）。
- [x] 状态栏（§7.8）：session 块不再画引擎图标；地址块显示当前连接的 `<连接名> · <用户>@<host>:<port>`。侧栏第一行的表数是已连上的各个连接的合计。
- [x] ponytail：连接断了不自动重连（和现在一样）。
- [x] 细节（worker 提议，已定）：
  - 树光标停在还没连上的连接节点上（未连接、连接中、失败）时，当前连接沿用上一次的，所以当前连接一定是连上的，不写「连上之后接着执行」的续接逻辑；
  - 未连接或失败的连接节点上按 `R` 等于展开（去连或重试），已连上的照旧刷新；连接中再展开或 `↵` 不做事；失败的 toast 写 `<连接名>：<驱动的错误>`；
  - `password_cmd` 的进程内缓存（§13）在这里做，建连失败时丢掉；
  - ponytail：两个连接上开着同名表时，tab 名和 pane 标题照旧只写表名；
  - console 在这里就带上连接：新 console 取当前连接、在它的 `Main` 上执行，默认布局里的 console_1 用启动时的连接；标题、切换连接、文件路径、state 记录留给 F5.11。

**验收**
- [x] 单测：当前连接的规则；两个连接上的同名表是两个 tab，缓存分开。
- [x] 集成测试：坏 DSN 的连接标成失败，不影响别的连接。
- [x] e2e：connections.toml 里写两个连接（同一个库起两个名字就行）。启动时另一个连接是折叠的、没有连上；展开它出现 schema；从它打开表；状态栏的地址块跟着焦点变。
- [x] golden：连接节点的四种状态；状态栏的 session 块和地址块。

## F5.11 console 的连接与文件 · 状态：passed（1155504；e2e e9c906c）

- **依赖**：F5.10
- **涉及**：`internal/app`（console、结果区）、`internal/db`（postgres.Conn 暴露 TxStatus）、`internal/config`（state 的 consoles）、`internal/ui`（console 标题、下拉框）

**开发**
- [x] console 标题改成 `<连接> · <库>.<schema> ▾`，连接和 schema 各是一个下拉框（§7.8「console 标题」）：
  - 连接下拉列出全部连接。选到未连接的就去连，连上之后才换过去；连不上就 toast 报错，留在原来的连接。命令「切换连接…」（`console.conn`）打开它，不绑默认键；
  - schema 下拉照旧（`gs`），列的是这个 console 所在连接的 schema；
  - 让位顺序：`▶ run` > 连接和 schema 的下拉框（两段算一项）> `↵`。
- [x] 执行走所选连接的 `Main`，search_path 的记账（mainPath / startedPath）挪到 Conn 上（§8.6）。
- [x] 所在连接的 `Main` 在事务里（TxStatus 不是 `I`）时不让换连接，toast「当前连接有未提交的事务，先提交或回滚」。在 postgres.Conn 上暴露 TxStatus，经 Worker 取，F5.3 也用它。
- [x] 文件（§11、§14）：
  - 路径改成 `consoles/console_<n>.sql`；
  - n 取**所有 session** 里都没打开的最小值：文件不再按连接分目录之后，两个 session 同时打开同一个 console_1，自动保存会互相覆盖；
  - state.json 加 `consoles: {"console_1": {"conn", "schema"}}`，换连接或 schema 时写入，打开文件时恢复；记下的连接已经不在 connections.toml 里时，用当前连接；
  - 新 console 的连接和 schema 取当前连接和它的 Schema；
  - 旧的 `consoles/<连接名>/` 下的文件不写迁移代码，通过后由决策者手工帮用户挪过去（同 F5.9 的键位）。
- [x] 补全、执行 DDL 之后刷新 catalog，都用这个 console 的连接。结果 tab 记下连接和 schema，重跑用它们，不管 console 之后关了还是换了连接。
- [x] 细节（worker 提议，已定）：
  - 标题两段分开点：连接名 `info` 色、`<库>.<schema> ▾` 照旧 `pk` 色，` · ` 不能点，悬停只亮那一段；连接下拉复用 dropdown 组件，行为、位置同 schema 下拉；
  - 选到未连接的：下拉关掉、去连，console 先留在原来的连接，连上之后才换过去（schema 取那个连接的 Schema）；连接中又选了别的，以最后一次为准；连不上 toast `<连接名>：<错误>`，console 不动；
  - 事务状态记在 `Conn` 上：每次 `Main` 的请求结束前在 Worker.Run 里读一次 TxStatus、随应答带回，换连接时看它（Main 在跑长查询时界面也不用等锁），F5.3 也用它；
  - state 的 `consoles` 以文件名为键；打开文件时没有记录、或记下的连接已不在 connections.toml 里，用当前连接和它的 Schema；记下的连接还没连上（比如上次 console_1 用的是 seed），照上面先挂在当前连接、后台去连、连上再换过去；
  - n 先在当前 session 里算，F5.1 有了多 session 后改成遍历所有 session；
  - 重跑显式传入连接和 schema：console 执行传它自己的，结果区重跑传结果 tab 记下的；
  - config 里 name 的字符限制保留，注释的理由改成 state 的键和树的节点 ID。
- [x] 审查时定（reviewer 提问，2026-10-02）：
  - 新建的 console（`c`、引导页按钮、面板命令、`C-e`）一律用当前连接和它的 Schema，并写回 state；state 里的记录只在启动时恢复默认布局的 console_1 时用（§8.6、§11）；
  - console 自己的执行还没回来时也不能换连接，toast「正在执行，结束后再换连接」；
  - 连接下拉暂时不画每项的图标，F5.3 加 `lock`、F5.4 加 MySQL 图标时给 Dropdown 加每项一个图标；
  - `password_cmd` 照 §13 按连接名缓存，测试里换密码来源时先清缓存；
  - toast 里的换行换成「；」、Tab 换成空格，在显示 toast 的那一处统一做（§7.8）。

**验收**
- [x] 单测：有事务时换连接被拦下；文件路径；n 的取法；从 state 恢复连接和 schema。
- [x] 集成测试：同一个连接上的两个 console 共用事务（一个里 begin，另一个也在事务里）。
- [x] e2e：用下拉框换连接后，在新连接上执行；重启后连接和 schema 都恢复。
- [x] golden：几种宽度下的 console 标题（连接和 schema 的下拉框让位）。

## F5.1 多 session 与新建连接 · 状态：passed（fce61c1；e2e 95c3338）

- **依赖**：F5.11
- **涉及**：`internal/app`（多个 session、session 列表、面板的会话范围、表单）、`internal/ui`（session 列表、表单）、`internal/config`（追加连接）、`internal/keymap/default.toml`（`[keys.sessions]`、`[keys.form]`）

**开发**
- [x] App 改成持有多个 session，`a.sess` 指当前那个；切换 session 时关掉浮层。
- [x] session 列表浮层（§7.8「session 列表」，键位在 `[keys.sessions]`，§6.8）：
  - `j` / `k` 移动；`l` / `h` 展开、收起（session 下面是它的 window）；`↵` attach，停在 window 上时还切到这个 window；
  - `n` 新建 session（PRD S-03 原为新建连接，§18）；
  - `x` 关闭：有未保存的修改或未提交的事务时先确认（§10.5）；关的是最后一个 session 时等于退出；
  - `$` 原地重命名；`esc` 关闭列表；
  - 框和行的样式照命令面板，每行是名字和 window 数，当前 session 用 `focus` 色标出；单击一行等于移过去再按 `↵`。
- [x] 新建 session（§7.8）：名字按 tmux 的习惯取最小的没用过的数字（`1`、`2`……）；默认布局，带一个新的 console_n；连上当前连接并展开它，其余连接不连。
- [x] 点击状态栏的 session 块打开列表（`SPC s` 已经有了）。面板的 `$` 范围列出 session，`↵` attach；「所有」范围也列 session（§12）。
- [x] 新建连接：命令 `conn.new`「新建连接…」，不绑默认键，打开表单（§7.8「新建连接表单」，作用域 `[keys.form]`）：
  - 字段：name、engine（F5.4 之前只有 postgres）、dsn、password_cmd、read_only；
  - 校验同 LoadConnections（必填、名字规则、不能重名）；
  - 写入：在 connections.toml 末尾追加一段 `[[connection]]` 文本，不重新编码，保留用户的注释；文件不存在就新建，权限 0600；
  - 写入后所有 session 的树里都多出这个连接，状态是未连接；保存时不试连，展开时才连。
- [x] 细节（worker 提议，已定）：
  - 新建 session：先在后台给新 session 的那份当前连接建连，toast「正在连接 <名字>…」，连上之后才建出 session 并 attach；连不上 toast `<名字>：<错误>`，不建 session（保住 F5.10「当前连接一定是连上的」）；
  - session 列表没有过滤框，`j` / `k` 是移动键；行不画图标，session 行是名字加 `dim` 色的 `N windows`，window 行缩进一级写 `0: data`；打开时光标在当前 session 上；`n` 建好后直接 attach、关掉列表；
  - `x` 关的是当前 session 时，attach 到它的上一个（它是第一个就下一个），列表不关，光标留在原来的行号；关之前先把它的 console 文件写盘，再在 Cmd 里关它的连接，断开时服务端自动回滚未提交的事务，确认之后不另发 ROLLBACK；确认框把这个 session 里所有表格的未保存修改数加起来，它的连接里有一个在事务中就加上「有未提交的事务，关闭会回滚」；
  - `$` 重命名：空名字等于放弃，和已有 session 重名时 toast「已有同名 session」、名字不变；window 行上 `x` / `$` 不做事（F5.2 走面板和状态栏）；
  - 面板 `$` 范围：名称是 session 名，所在位置 `N windows`，类型标签「会话」，图标 `conn`；当前 session 上 `↵` 只关面板；session 项不记进「最近用过」（ponytail，同 F4.3 的 tab 项）；
  - 应答可能属于后台的 session：按应答里带的 conn / tab 指针处理，焦点、布局类的副作用只对当前 session 做；console_n 的 n 改成遍历所有 session；
  - 表单：config 加 `ReadOnly`（F5.1 只写进文件，F5.3 才生效）；`Connection.Check(names)` 返回出错的字段和原因，LoadConnections 也用它；追加时用 BurntSushi 的 encoder 只编码这一条，原文件末尾没有换行先补一个，没填的可选字段不写；键位 `[keys.form]`：`form.next` / `form.prev`（Tab / S-Tab）、`form.save`（↵）、`form.cancel`（esc）。

- [x] 审查时定（reviewer 提问，2026-10-03）：
  - 切换 session（包括 `n` 建连回来后自动 attach）时关掉所有浮层，确认框、改名框、表单、面板都关；
  - 列表和表单打开时按 `C-p`，面板盖在上面并接管按键（同 COLS 列表）；表单里 `C-s` 绑 `form.save`，不去保存背后表格的修改；
  - 关掉有查询在执行的 session 不另确认，也不弹「查询已取消」；
  - 列表里每个 session 都标出它自己的当前 window（同 tmux）；
  - 关 session 的确认框写「<session 名> 里有 N 处修改未保存，关闭会丢弃。」
**验收**
- [x] e2e：
  - 新建连接后，`connections.toml` 中出现这条连接，且不会破坏文件中已有的内容（含注释）；
  - 在两个 session 之间切换后，各自的界面状态保持不变；
  - 新建 session 的名字与布局；
  - 关掉最后一个 session 等于退出。
- [x] golden：session 列表（含展开的 window）；新建连接表单（含校验报错）。

## F5.2 多 window · 状态：passed（fce61c1；e2e 95c3338）

- **依赖**：F5.1
- **涉及**：`internal/app`（window、面板、树的工作区）、`internal/ui`（状态栏）

**开发**
- [x] **按键**：`SPC n` / `SPC p` / `SPC l` 切到下一个 / 上一个 / 上次用的 window，`SPC c` 新建；重命名、关闭默认不绑键，在命令面板里执行（§6.8）。不提供按编号切换 window。
- [x] **鼠标**：点击状态栏上的 window 列表可切换。
- [x] **独立性**：每个 window 有自己的布局和结果区。
- [x] 面板里 window 项 `↵` 切过去；别的 window 的 tab 项 `↵` 先切 window、再选 tab、聚焦它的 pane（F4.3 留下的那一半）；树的工作区里 window 节点 `↵` / 单击也切过去（§7.8、§12）。
- [x] 重命名：状态栏上这个 window 的位置原地变成输入框（INSERT，`↵` 确定、`esc` 放弃）。
- [x] 关闭：有未保存的修改时先确认（§10.5）；关的是最后一个 window 时不做事，同「最后一个普通 pane 关不掉」（§5）。
- [x] 细节（worker 提议，已定）：
  - `SPC c` 新建的 window：照 §5 默认布局，侧栏、一个空 pane，旁边放一个新的 console_n，比例同新 session；名字 `data`，重名不拦（同 tmux）；放在最后并切过去；
  - `SPC n` / `SPC p` 到头绕回（同 tmux）；`SPC l` 回到上一个用过的 window，那个 window 已关掉就不做事；
  - 关闭：先把这个 window 的 console 写盘，有未保存的修改先确认；关的是当前 window 时切到上一个用过的，没有就切到它前面那个（它是第一个就后面那个）。未提交事务的确认在 F5.3 加（让某个连接上不再有 console 时）；
  - 重命名：状态栏上当前 window 那一段原地变成输入框（INSERT，没绑定的键当文字，`↵` 确定、`esc` 放弃），空名字等于放弃；
  - 状态栏 window 列表的每一段可以点击，执行不带标题的 `window.select <i>`（同 `palette.scope`）；
  - Window 加一个稳定 ID（同 Pane 的做法，session 里递增），树的展开状态和 `SPC l` 的「上一个用过的」都按 ID 记，不按下标：下标在关掉 window 后会前移，展开状态会错到后一个 window 上，`SPC l` 也会指错（决策者定，没有采用 worker 提的「按下标、用 ponytail 标出」）。

- [x] 审查时定（reviewer 提问，2026-10-03）：
  - 改名时点改名框以外的任何地方都等于 `esc`；
  - 关 window 的确认框写「这个 window 里有 N 处修改未保存，关闭会丢弃。」（同关 pane）。
**验收**
- [x] e2e：
  - 新建、切换、重命名、关闭 window 都正确；
  - 各 window 的布局互不影响。
- [x] 补回 M0 里因 F1.1 去掉第二个 window 而删掉的 3 项 e2e（tester 在 F1.1 结论里列出，2026-09-24）：f0.5 其余 window 为 dim 字、无底色；窄宽度下省略非当前 window（含省略顺序单调性检查里的这一环）；f0.14 window 范围 `↵`（`%report`）。

## F5.3 只读连接与事务模式 · 状态：passed（fce61c1；e2e 95c3338）

- **依赖**：F5.2
- **涉及**：`internal/db`（RuntimeParams、TxStatus）、`internal/sqlkit`（解除只读的语句）、`internal/app`（console 执行、保存、确认、状态栏）、`internal/ui`（console 标题、图标）

**开发**
- [x] **只读，数据库层**（§8.2、§13）：`read_only` 打开时，`Main` 也在 RuntimeParams 里设 `default_transaction_read_only = on`（现在只有 `Meta` 设），`RESET` / `DISCARD ALL` 之后仍是 on。
- [x] **只读，应用层**（§13）：console 执行前检查这一次的全部语句，判为写的、以及会解除只读的语句，有一条就整次都不发，日志记原因并切到日志 tab。
  - 会解除只读的：§13 那四种 SET，加上 `BEGIN READ WRITE` / `START TRANSACTION READ WRITE`（PG 里它会盖过 `default_transaction_read_only`）；
  - 照 AGENTS.md 先写集成测试，在 PG 上逐条实测再定清单；其他 SET 放行；
  - ponytail：`select set_config('default_transaction_read_only', 'off', false)` 判为读，两层都挡不住（§13：只读是防误操作，不是权限边界）。
- [x] **只读，界面**：表格的编辑、新增行、删除行、粘贴都拦下，toast「<连接> 是只读连接」（§10.1）。树的连接节点、console 标题的连接下拉里画 `lock` 图标（nf-fa-lock U+F023，ascii `RO`，§7.7）。
- [x] **事务模式**记在 `Conn.Tx`：
  - console 标题上 `Tx: 自动 ▾`（自动 / 手动），手动时旁边是 ✓ ↺，不在事务里时 `dim`、不能点（§7.8「console 标题」）；让位顺序 `▶ run` > 连接和 schema 的下拉框 > 事务控件（`Tx ▾` 和 ✓ ↺ 算一项）> `↵`；
  - 命令 `tx.mode`「手动提交事务」（开关类，手动时为 ON）、`tx.commit`「提交」、`tx.rollback`「回滚」，都作用于当前连接，都不绑默认键；
  - 手动模式下 console 执行：`Main` 的 TxStatus 是 `I` 时，先在同一个 Worker.Run 里 BEGIN。出错后事务停在 `E` 状态，由用户回滚；
  - 手动模式下保存（§10.3）：不在事务里就先 BEGIN；整次保存包在 `SAVEPOINT sqlmux_save … RELEASE` 里，失败时 `ROLLBACK TO SAVEPOINT`，否则一次保存失败会把用户在这个事务里之前做的修改全部作废；不 COMMIT，提示「已保存 N 行（未提交）」；
  - 手动模式下表格取数走 `Main`。ponytail：计数仍在 `Meta` 上，看不到未提交的增删；
  - 有未提交的事务时从手动切回自动被拦下，toast 提示先提交或回滚，同换连接的规则。
- [x] **状态栏**（§7.8）：当前连接的 `Main` 在事务里时显示 `warn` 色的「事务中」，自动模式下用户自己 begin 的也算；处在出错的事务里（`E`）显示 `error` 色的「事务出错」。查询条不显示事务模式（原清单的「查询条显示事务模式」去掉）。
- [x] **确认**（§10.5）：有未提交的事务时，退出、关闭 session、让这个连接上不再有 console 的关闭（关 tab、关 pane、关 window）都弹确认框「<连接> 有未提交的事务，<动作>会回滚。」，按 `y` 之后回滚（关 tab / pane / window 时先关再发 ROLLBACK，退出、关 session 时靠断开连接，§10.5）。
- [x] 细节（worker 提议，已定）：
  - 只读判定 `sqlkit.BreaksReadOnly`：IsWrite 判为写的拦下，事务控制和会话设置放行（begin / start / commit / end / rollback / abort / savepoint / release / set / reset / discard），再拦会解除只读的 SET（含加引号的参数名）和带 `read write` 的 begin / start；集成测试按 PG 实测的那张表写，每条后面跟一句 `create temp table`，断言实际结果和判定一致（§13）；
  - 拦下时不算一次执行，日志记一行 `error` 色的原因、切到日志 tab，不弹 toast、▶ 不变红、不出错误栏；重跑也检查；
  - 表格：编辑、粘贴、`o` / `+`、`dd` / `−`、`p` 都 toast「<连接> 是只读连接」，和「没有主键」走同一个入口、先判只读；设 NULL / DEFAULT 的键静默不做事；查询条按钮照常画；
  - lock：树的连接节点在名字后面画 `dim` 色的 `lock`（和「连接中」的 `…` 都有时是 `seed <lock> …`）；连接下拉放在已有的右侧注释列；console 标题的连接名后面也画，和连接名同一个命中区（§5「树和 console 标题上画锁」）；
  - `Conn.Manual bool`，新连上的都是自动，不记进 state；
  - 标题上 `Tx: 自动 ▾` 用 `fg` 色，点开的下拉里当前那项 `pk` 色；点击走不带标题的 `console.tx`，面板里用 `tx.mode`；✓ ↺ 各是一个提示，不在事务里时 `dim`、不注册命中区，三者算同一组；
  - 提交、回滚用不可取消、带超时的 ctx，应答带回 TxStatus；不在事务里时不做事；成功不弹 toast，失败 toast 错误；出错的事务里提交 toast「事务已出错，提交改为了回滚」；
  - 提交、回滚之后（包括确认框里的回滚）重取这个连接上打开着的表格、重算计数；
  - 手动模式下执行：先按 §8.6 设 search_path（在 I 状态下设、会记住），再 BEGIN，再执行；BEGIN 不写进日志；
  - 保存：手动模式，或者 auto 模式下 Run 里读到的 TxStatus 不是 I（用户自己 begin 了），都走 SAVEPOINT、不 COMMIT，提示「（未提交）」；gotSave 用读到的真实 TxStatus（§10.3）；
  - 手动模式下 Page 走 `Main`，列信息和计数仍走 `Meta`；有事务时从手动切回自动被拦下，toast 同换连接那句；
  - 状态栏：T「事务中」`warn`、E「事务出错」`error`，在 busy 左边，不能点、不让位；
  - 确认框合成一个函数：修改一句、事务一句，几个连接用「、」连起来并去重；关 tab / pane / window 时算出关掉之后不再有 console、又在事务中的连接，`y` 之后先关、再在 Cmd 里对它们发 ROLLBACK、再重取表格；退出、关 session 不另发 ROLLBACK，关 session 那句改成带连接名的统一写法；只在手动模式下保存过表格、没开过 console 的连接，关表格不问，退出、关 session 照样问。

- [x] 审查时定（reviewer 提问，2026-10-03）：
  - §13 的四类 SET 整类拦下（其中实测不解除只读的也拦），§13 已写明；只读解除的集成测试加一列，把「拦下但无害」的也纳入实测；
  - 作用域词连写（`SET SESSION SESSION CHARACTERISTICS …`）也要认出；扫描器照 scan.l 把 0x80 以上的字节当标识符字符（§9.1）；`U&"…"` 记成 ponytail 上限；
  - 手动模式下每条语句前检查 BEGIN，照 psql 的 SendQuery，psql `command_no_begin` 列出的语句不补（先查 psql 源码，§11）；
  - console 有执行在跑时不能关（tab、pane、window），toast「正在执行，结束后再关闭」；
  - 表格取数走 `Main` 的条件改成「手动模式或正在事务中」（§8.2）；
  - 提交、回滚之后也清列缓存、重新加载 catalog（§7.8）；
  - `C-c` 照旧按连接取消，后果写进 §8.3；手动模式下表格取数等 `Main`、WHERE 能写，写进 §8.2；
  - 在事务中的表格取数包在 `SAVEPOINT sqlmux_page` 里（worker 在清单外加的，认可，§8.2）；
  - 计数仍在 `Meta` 上看不到未提交的增删，代码里要有 `ponytail:` 注释。
**验收**
- [x] 集成测试：
  - 只读连接中，写语句被拦截；
  - 解除只读的 SET、`BEGIN READ WRITE` 被拦截，`SET search_path` 等可以执行；
  - 手动模式下保存失败后，之前的修改还在。
- [x] e2e：manual 模式下保存后不提交；执行 Rollback 后修改被撤销。
- [x] golden：console 标题的事务控件（自动、手动、在事务里）；状态栏的「事务中」「事务出错」；只读连接的锁。

## F5.4 MySQL · 状态：passed（d99727f；e2e a5539b2）

- **依赖**：F5.3
- **涉及**：`internal/db/mysql`、`internal/sqlkit`、`testdata/seed/mysql.sql`

**开发**
- [x] 开工前在主工作区起 MySQL 容器：`docker compose up -d mysql`（AGENTS.md「集成测试环境」）。
- [x] **驱动**：对应 §8.1–§8.3 中 MySQL 的部分：
  - 走文本协议，开启 `interpolateParams`；
  - 取消执行用 `KILL QUERY`。
- [x] **catalog**：取 §8.4 表格中 MySQL 那一列的数据：
  - `TABLE_ROWS`、`STATISTICS`；
  - 从 `COLUMN_TYPE` 解析出 enum 的可选值；
  - DDL 用 `SHOW CREATE TABLE` 获取。
- [x] **方言**：
  - 标识符用反引号；
  - 保存时的旧值比较用 `cast(列 as binary) = ?`，旧值是 NULL 时 `IS NULL`（177d054 逐类型实测，原定的 `<=>` 否掉，§10.3）；
  - 布尔值写入 1 / 0。
- [x] **种子数据**：`testdata/seed/mysql.sql`，覆盖的情况与 PG 的种子数据相同。
- [x] **扫描器**：MySQL 会执行的注释 `/*! … */`（如 `SELECT 5 /*! +1 */` 返回 6，dump 开头的 `/*!40101 SET NAMES utf8 */;`）M3 的扫描器当作注释，分句和读写判定会错；M5 接 MySQL 时处理（M3 审查时发现）。
- [x] **database 能选**（开工前定，同 PG 的 schema，§8.6）：树里 MySQL 连接下面一层列 database（不列系统库），console 标题是 `<连接> · <database> ▾`，执行前不同就先 `USE`，一次执行里有 `USE` 时作废记下的值。
- [x] **事务状态**：MySQL 拿不到 TxStatus，按执行过的 begin / start transaction / commit / rollback 来记（ponytail：DDL 的隐式提交这类情况记不准）。
- [x] 新建连接表单的 engine 可以选 mysql；树的连接节点和 console 的连接下拉里，MySQL 连接画 `mysql` 图标、PG 连接画 `postgres` 图标（§7.7）。
- [x] 细节（worker 提议，已定）：
  - 按层分 4 个 commit，各自全绿，审查节点仍是 F5.4 一个：① 驱动和浏览（compose 挂载 seed、mysql.sql、引擎抽象、mysql.Conn、catalog、Page / Count、树和表格）② 编辑和保存 ③ console（扫描器 `/*! */`、执行、只读判定、快速 SQL、DDL 预览、格式化、补全）④ 表单的 engine 和图标；
  - `db.Engine` 接口（两个实现），Conn 上存一个；app 里直接调 `postgres.X` 的地方都改成走它；共用的类型从 postgres 挪到 db；
  - 驱动 go-sql-driver/mysql：`sql.DB` + `SetMaxOpenConns(1)` + `db.Conn(ctx)` 固定一条，由 Worker 独占；DSN 加 `interpolateParams=true`、`clientFoundRows=true`（否则改成同样的值时影响 0 行，被误报成「行数据已变化」），不开 multiStatements；值读成文本；connections.toml 的 dsn 写 `mysql://user:pass@host:port/db?…`，用 net/url 转成驱动的 Config，不加 dburl，不是 `mysql://` 开头的按驱动原生格式解析；
  - 事务状态按执行过的语句记：begin / start transaction → T，commit / rollback（不含 rollback to）→ I；MySQL 出错不中止事务，没有 E；ponytail：DDL 的隐式提交等记不准；
  - 只读：`Meta` 和只读连接的 `Main` 建连后 `SET SESSION TRANSACTION READ ONLY`；BreaksReadOnly 补上 MySQL 的写法，在 MySQL 上逐条实测（§13）；
  - 列和编辑：列信息照 §8.4；tinyint(1) 当布尔，选项写 1 / 0；时间分段认 date / datetime / timestamp / time；§10.7 的前置校验只对 PG 做（ponytail）。保存时的旧值比较先逐类型实测（float / double、decimal、datetime、json、bit、binary、enum / set、char 尾空格），`<=>` 带文本参数会按类型转换，很可能改成 `CAST(列 AS CHAR) = ?`、旧值是 NULL 时 `IS NULL`；结论写进提交说明，并告诉决策者改 §10.3；
  - 取消照 §8.3：另起 goroutine 盯着 ctx，取消时用同一份配置开临时连接 `KILL QUERY <id>`（id 建连时 `select connection_id()`），错误 1317 换成 ctx 的错误；
  - 快速 SQL：判为读的加自动 LIMIT，在只读的 `Meta` 上执行（§12）；
  - 集成测试：mysql 包对照 postgres 包写一套（catalog、page / count、逐类型保存、快速 SQL、KILL QUERY、只读解除表），环境变量 `SQLMUX_TEST_MYSQL`；app 层和引擎无关的（连接、console 共用 Main、手动事务、只读）两个引擎各跑一遍；PG 特有的（search_path、DateStyle、E 状态、format()）不搬；
  - 图标 `mysql`（nf-dev-mysql U+E704，ascii `my`）；表单的 engine 在 postgres / mysql 之间用空格切换。

- [x] 测试时定（tester 提问，2026-10-03）：面板里输入不为空时，分数相同的按输入为空时的顺序排，不按长度：同名的表当前连接的 Schema 里那张排第一（§12）。MySQL 能看到服务器上所有的库，同名表很常见，原来 `↵` 会打开别的库的表。
- [x] 审查时定（reviewer 提问，2026-10-03，MySQL 8.4 上都已复现）：
  - 事务状态照手册「Statements That Cause an Implicit Commit」记：这些语句执行后、以及死锁（1213）后记为不在事务中，死锁时 toast「死锁，整个事务已回滚」；MySQL 下这些语句和不带作用域的 `SET TRANSACTION` 都不补 BEGIN；其余记不准的写成 ponytail（§8.2）；
  - 只读：MySQL 下 `reset` 算写；SET 里带 GLOBAL / PERSIST / PERSIST_ONLY 的项和 `@@global.x` 都拦；MySQL 8.4 的语句首词补进关键字表；`NO_BACKSLASH_ESCAPES` 记成 §13 的已知上限；
  - go-sql-driver 钉在 v1.7.1，加 `ponytail:` 注释写明原因和升级条件（§8.1）；
  - 旧值比较改成 `cast(convert(列 using utf8mb4) as binary) = ?`（§10.3）；
  - 行标识列含 binary / varbinary / blob / bit 的表、非事务引擎的表只读（§10.1）；
  - BIT 列按数字显示、不能编辑（§8.1）；
  - 复合语句体被切开、不支持 `DELIMITER`、`KILL QUERY` 没有兜底、`wait_timeout` 断开后不重连，都写成已知上限，代码里用 `ponytail:` 标出（§9.2、§8.3、§8.2）。
**验收**
- [x] M1–M4 的集成测试在 MySQL 上全部通过。
- [x] 用 `KILL QUERY` 取消长查询有效。

---

以下为 M2–M4 用户验收的意见（2026-10-02），在 F5.10 之前做，审查节点 0。

## F5.5 工具按钮共用一条底色、竖线分隔 · 状态：passed（4179aca；e2e 0e2db7a）

- **依赖**：F3.26
- **涉及**：`internal/ui`（querybar）

**开发**
- [x] 按 §7.8「工具按钮」：整排按钮共用一条 `sep` 底，组之间用 `border` 色的 `│` 分隔（参考 DataGrip 的工具栏，用户嫌一组一块底色有割裂感），转置也在这条底里；悬停只亮那一个按钮；让位顺序不变。
- [x] 细节（worker 提议，已定）：`│` 占 1 列、前后不另留空，组间距从 2 列变 1 列；`│` 是 `sep` 底、没有命中区；某组让位后 `│` 只画在仍显示的组之间；`ui.ButtonGroup.Bare` 没人用了，删掉。

**验收**
- [x] golden：几种宽度下的按钮条，含悬停。

## F5.6 tab 上悬停的 `×` 底色画满 · 状态：passed（4179aca；e2e 0e2db7a）

- **依赖**：F3.35
- **涉及**：`internal/ui`（tab 栏）

**开发**
- [x] 悬停时 `×` 的 `select` 底只盖住了一半（用户截图）：nerd 图标在终端里会画出格子外，按 §7.7「图标后面留空格」，`×` 后面跟一个空格、一起画底色，命中区同样覆盖这两格；悬停时 tab 栏仍不移位。原因和做法由 worker 确认后在提交说明里写明。
- [x] 细节（worker 提议，已定）：根因是 U+F00D 按字素宽度算 1 格，终端却画进右边一格。有标记（`*` / `-`）的 tab，`×` 替换标记，和后面的空格两格一起亮、一起做命中区；没有标记的 tab，悬停时把右边的 `│` 画成亮着的空格并算进命中区，结果区最后一个 tab 后面直接画一个亮着的空格。平时和悬停时布局都不变，不给每个 tab 预留标记位。

**验收**
- [x] golden：悬停时 `×` 和它后面的空格都是 `select` 底。

## F5.7 转置视图左对齐 · 状态：passed（4179aca；e2e 0e2db7a）

- **依赖**：F1.3
- **涉及**：`internal/ui`（grid）

**开发**
- [x] 转置视图里所有值一律左对齐，数字也是（§7.6：一列里混着各种类型，右对齐的数字看着乱）；不转置时照旧。

**验收**
- [x] golden：转置后的 t_order，`id`、`amount` 左对齐。

## F5.8 `:` 打开「所有」范围 · 状态：passed（4179aca；e2e 0e2db7a）

- **依赖**：F0.13
- **涉及**：`internal/keymap/default.toml`、`internal/app`（palette 的排序）

**开发**
- [x] `[keys.grid]`、`[keys.tree]`、`[keys.landing]` 里的 `:` 改绑 `palette.open`（「所有」范围，同 `C-p`）；在「所有」范围里，输入和某个 ex 别名完全相同时这条命令也排第一，`:q↵`、`:qa↵`、`:w↵` 照旧（§12「ex 别名」）。想要原来的命令范围，把 `:` 绑到 `palette.command`（不另加配置项）。
- [x] 细节（worker 提议，已定）：`palette.command` 保留标题、不再有默认键，导出时是 normal 节里的 `# "" = "palette.command"`；ex 别名在「所有」和 `>` 范围都生效，去掉首尾空白后完全相同才算，排在最近用过的前面。

**验收**
- [x] e2e：表格里按 `:` 打开的是「所有」；`:q↵` 关掉 tab、`:w↵` 保存。

## F5.9 键位挪到 keymaps.toml · 状态：passed（4179aca；e2e 0e2db7a）

- **依赖**：F1.13
- **涉及**：`internal/config`、`internal/keymap`、`cmd/sqlmux`（`keys --format toml`）

**开发**
- [x] `[keys]`（含 leader）、`[keys.*]`、`[map.*]` 从 config.toml 挪到 `~/.config/sqlmux/keymaps.toml`（§14）；config.toml 里还写着它们时启动报错，提示挪到 keymaps.toml。
- [x] `sqlmux keys --format toml` 输出一份完整的 keymaps.toml（§6.7）：各作用域都有，和默认相同的绑定写成注释行、值是默认键，后面注释标题；没有默认键的写 `# "" = …`；`leader` 和各层 `[map.*]` 的示例也注释着放进去。用户去掉 `#` 才生效，新版本改了默认键不会被旧文件盖住。用户改过的（改键、解绑、leader、映射）照旧写成生效的行（tester 提问后定，保留 F1.13「导出的是当前生效的键位」的往返）。`--check` 读 keymaps.toml。
- [x] 细节（worker 提议，已定）：config.toml 里有顶层 `keys` / `map` 时报 `<路径>/config.toml: [keys] / [map] 要写在 keymaps.toml 里（sqlmux keys --format toml 生成一份完整的参考）`，走现有的启动报错、退出码 1，`keys --check` 同样报；keymaps.toml 里写了别的顶层键时报 `<路径>/keymaps.toml: <键>：这里只能写 [keys] 和 [map.*]`，文件不存在就当没有改动；config 包加 `ParseKeys`，`default.toml` 也走它，不加新类型。导出：`[keys]` 和各作用域的表头不注释，leader 是默认值时写 `# leader = …`；每个作用域按 default.toml 的顺序，相同的写注释行、改了的和解绑的在原位置写生效的行，然后是用户新加的绑定，最后是没绑定的 `# "" = …`；`[map.*]` 只出用户写了的表，末尾照旧附注释掉的示例；空作用域不出，文件头不加说明。
- [x] 通过后决策者把用户的 config.toml 里的键位挪出去：生成 keymaps.toml，用户的 `Y` 映射保留为生效的行，config.toml 里只留主题等常规选项（各留备份）。2026-10-03 做完：用户选了精简写法，keymaps.toml 只有两行 `Y`（同 lazygit / yazi，只写改过的），备份 `config.toml.bak-2026-10-03`。

**验收**
- [x] 单测：没有用户改动时，导出的全是注释，原样加载后和默认完全一样；有改动时导出再加载，生效的键位不变（往返）；去掉一行的 `#` 改了键后生效；config.toml 里写 `[keys]` 时报错。
- [x] e2e：只有 keymaps.toml、里面改了一个键时，界面上的键位提示跟着变。

## F5.12 补全：接受关键字补空格，补齐 DML / DDL 关键字 · 状态：todo

- **依赖**：F1.14、F3.39
- **涉及**：`internal/sqlkit`（`keywords.go` 的 `Common`、补全上下文）、`internal/app`（console、WHERE、快速 SQL 的接受）

**开发**
- [ ] 接受关键字时补一个空格，光标落在空格后；值（`null`、`true`、`false`、`is null`、`is not null`）、函数、表名、列名、枚举值不补；光标后已经是空白时不补、光标移过去（§9.7「接受关键字时补一个空格」）。console、WHERE、快速 SQL 三处一样，改在它们共用的那一处。
- [ ] 关键字候选补齐 DML / DDL 的开头和子句，清单参照 lazysql 的 `builtinKeywords`，MySQL 另有 `replace into`、`on duplicate key update`（§9.7 候选表）。具体清单开工前发给决策者。

**验收**
- [ ] 单测：`selec` 接受后是 `select `、光标在空格后；`an` 接受后是 `and `；`nul` 接受后是 `null`、不带空格；光标后已有空格时不多补。
- [ ] e2e：WHERE 里打完整个 `and`、列表里是 `and` 时按 `↵`，文字变成 `… and `、不执行查询、没有报错（用户验收时的复现）；打完 `null` 按 `↵` 照常执行。
- [ ] e2e：console 里输入 `ins`、`upd`、`del` 都能补出来。

## F5.13 INSERT 的列补全 · 状态：todo

- **依赖**：F5.12
- **涉及**：`internal/sqlkit`（补全上下文）、`internal/app`（console、快速 SQL）

**开发**
- [ ] `insert into <表> (` 的括号里，`(` 或逗号之后弹出这张表还没写进去的列，第一项「全部列」，接受后一次插入剩下的列（§9.7「INSERT 的列」）。PG、MySQL 都做。

**验收**
- [ ] 单测：上下文识别（带 schema、带引号、MySQL 反引号、逗号之后、已经写了几列）；`generated always` 的列不出现。
- [ ] e2e：console 里输入 `insert into t_log(`，不再敲别的就弹出列表，第一项是全部列，`↵` 后括号里是全部列、光标在 `)` 前；再试一次只选一列、敲 `, ` 后列表里少了这一列。

## F5.14 标点用运算符的颜色 · 状态：todo

- **依赖**：F3.27
- **涉及**：`internal/ui`（SQL 着色）

**开发**
- [ ] `;` `,` `(` `)` 用 `sql_operator` 色，`.` 不着色（§7.3）。console、WHERE 输入框、快速 SQL 一样。
- [ ] 通过后决策者把用户主题 `ristretto.toml` 的 `sql_operator` 改成用户要的 `#ee9089`。

**验收**
- [ ] golden：`select * from t_log;` 里 `*` 和 `;` 是 `sql_operator` 色；`play.t_log` 的点不着色。

## F5.15 树的 pane 节点加图标 · 状态：todo

- **依赖**：F1.12
- **涉及**：`internal/ui`（图标表、树）

**开发**
- [ ] 工作区的 pane 节点画图标 `pane`（nf-fa-columns，U+F0DB，ascii `|`），可以在主题的 `[icon]` 里覆盖（§7.7、§7.8 工作区节点）。

**验收**
- [ ] golden：工作区里的 `pane-1`、`pane-2` 前面有图标，图标后留一格。

## F5.16 新命令：循环切焦点、关 tab 连带关 pane / window、重开关掉的 tab、新建 session · 状态：todo

- **依赖**：F5.1、F5.2
- **涉及**：`internal/app`（焦点、tab / pane / window 的关闭、session）、`internal/keymap`（标题）

**开发**
- [ ] `pane.focus.next`、`tab.quit`、`tab.reopen`、`session.new` 四个命令，规则见 §6.8「M5 验收后加的命令」；都不带默认键，都有标题，命令面板里能搜到。

**验收**
- [ ] 单测：`pane.focus.next` 的循环顺序（树收起、结果区隐藏时跳过它们）；`tab.quit` 的三种情况（pane 里还有 tab、pane 空了关 pane、window 空了关 window）和最后一个 window 不关；`tab.reopen` 恢复 WHERE / ORDER / LIMIT，原 pane 没了时开在当前 pane，连按往前翻。
- [ ] e2e：keymaps.toml 里给四个命令各绑一个键，在真实终端里各按一遍，界面变化符合上面的规则。

## F5.17 终端翻译过来的修饰键（先实测） · 状态：todo

- **依赖**：F5.9
- **涉及**：`internal/keymap`（键的解析与记法）、`e2e/`（tester）

**开发**
- [ ] 先实测，再决定改不改代码（§6.2「终端翻译过来的键」）：在 tmux 3.7b 里，用 `send-keys` 把 CSI u 编码的键（如 `<C-S-w>`、`<C-S-[>`、`<C-S-t>`）送进跑着 sqlmux 的 pane，试原始字节（`send-keys -H`）和 tmux 自己的 extended-keys 两种送法；看 `[keys.global]` 里绑的命令在表格、树、console 的 NORMAL 和 INSERT、WHERE 的 INSERT 下是否都生效、是否有字符漏进输入框。不通的地方改解析或记法，通的就只加 e2e。
- [ ] 结果和推荐的送法（tmux 那一侧的命令）发给决策者，决策者写进 §6.2，并据此配用户的 kitty / tmux。

**验收**
- [ ] e2e：用推荐的送法送一个绑在 `[keys.global]` 的键，在表格和 console 的 INSERT 下都执行了对应命令，输入框里没有多出字符。

**通过后决策者给用户配键**（用户 2026-10-04 要的一组，写进 `~/.config/sqlmux/keymaps.toml`，只写改动；Cmd / Ctrl+Shift 那几个照用户给 nvim 的做法，改 kitty.conf 和 .tmux.conf.local 的分支，先备份）：`s` 表格里选排序列（`grid.order`）；`Cmd+Shift+[` / `]` 同一个 pane 里左右切 tab；`Cmd+Shift+T` 重开关掉的 tab；`Ctrl+Shift+[` / `]` 左右切 window；`tc` 新建 window；`tC` 新建 session；`tt` 上一个 window（`window.last`）；`Cmd+1` 循环切焦点；`Cmd+W` 关 tab、空了连 window 一起关（`tab.quit`）；`SPC q` 收起结果区（`result.toggle`）；`SPC ll` 格式化当前语句（console 里映射成 `gqq`，VISUAL 下 `gq`）。为了不和 `t…`、`SPC l…` 冲突，解绑树和引导页的 `t`、默认的 `SPC l`。
