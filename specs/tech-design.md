# DB TUI 技术方案

> 状态：草案，待评审 · 2026-09-23（第七版）
> 对应 [PRD - DB TUI v0.0.2](https://claude.ai/design/p/e6118404-bf7a-4420-abfd-e31198df2ba8?file=PRD+-+DB+TUI+v0.0.2.dc.html) 与 [设计稿 DB TUI v0.0.2](https://claude.ai/design/p/e6118404-bf7a-4420-abfd-e31198df2ba8?file=DB+TUI+v0.0.2.dc.html)
> 产品名：**sqlmux**。GitHub 上只有一个 2014 年后就没再更新的同名 Erlang 仓库，Homebrew 里没有同名软件。
> 文中的需求编号（S-01、G-03、F-05 等）指 PRD 条目。对 PRD 的改动建议汇总在 §18。

## 0. 结论速览

| 项 | 决定 |
|---|---|
| 语言 / 框架 | Go 1.26 + Bubble Tea v2 + Ultraviolet（已确认）。Lip Gloss v2 原本在列，但最终只用到解析颜色这一处，M0 修剪时去掉了 |
| 渲染 | 立即模式：每帧把画面画进一块 cell 缓冲区，同时生成鼠标命中表 |
| 输入 | 键盘、鼠标、命令面板统一转成 Action。默认键位只用所有终端都能区分的键，不依赖 kitty 键盘协议，也不假设用户在用 tmux |
| 键位模型 | 不做 tmux 式的全局前缀键。NORMAL 模式下用空格（`SPC`）作 leader，按下后弹出可点击的键位提示（已确认） |
| 数据库 | pgx/pgconn、go-sql-driver/mysql；每个连接两条（Meta / Main），用到时才连；所有值按文本处理 |
| SQL 处理 | 自研词法扫描器，负责分句、高亮、读写判定、自动 LIMIT |
| SQL 格式化 | 通过 goja 在进程内运行 sql-formatter（JS 库），已做可行性验证 |
| SQL 编辑器 | 自研 vim 编辑器，覆盖 nvim 的基础操作：文本对象、操作符加移动、大小写转换、注释、撤销与重做、`:s` 替换。不做宏和 `.` 重复。键位映射（如 J → 5j）可以按 pane 类型分别配置。行为用 nvim 做差分测试来校验 |
| 补全与模糊匹配 | console 和 WHERE 输入时提供 fzf 式的模糊补全。全项目的模糊匹配都直接使用 fzf 的匹配算法 |
| 单元格编辑 | 按列属性提供选项：时间分段调整、NULL、DEFAULT、布尔、枚举 |
| WHERE | 不做简写语法，直接输入完整的 SQL 条件表达式 |
| 结果展示 | 每个 window 底部一个全宽结果区，内部用 tab 区分：日志、各 console 的结果、已固定的结果（已确认） |
| 范围 | PostgreSQL + MySQL。Redis 暂缓，只在 session 层留出位置 |

## 1. 范围

- 覆盖 PRD 第 5 章全部功能需求（Redis 相关除外），以及第 6、7、8 章的键位、鼠标和视觉规范。键位的调整见 §6.8 和 §18。
- 在 PRD 之外新增：console 内的 SQL 模糊补全（§9.7）。PRD 原本只要求 WHERE 输入框补全字段名。
- 不做：PRD 1.4 列出的非目标；Redis 的 key 浏览（布局未定）；表格多选和批量编辑（PRD 已标为后续版本）。

## 2. 技术选型

### 2.1 选型记录：Go + Bubble Tea v2（已确认）

选型依据：

- **团队熟悉 Go。**
- **框架能力足够。** Bubble Tea v2 与 Ultraviolet（Lip Gloss v2 的底层）原生支持：
  - 按 cell 读写的缓冲区；
  - 带 z 序的图层和点击命中测试；
  - 带坐标的鼠标事件，包括悬停；
  - 约束布局（`ultraviolet/layout`，与 ratatui 的 Layout 同构）。

  版本线为 bubbletea v2.0.9。lipgloss 最初也在依赖里，M0 修剪时去掉了，颜色改用 `ansi.XParseColor` 解析。
- **性能不是瓶颈。** 数据库 TUI 只在有输入或数据回来时才重绘，画一帧 200×60 的界面远不到 1ms。真正影响体验的是数据量，靠两点解决：表格只渲染可见行（§7.6），结果集设行数上限（§11）。
  - **M8 F8.20 的实测修正**（用户 2026-10-08：在表格上拖动指针时卡顿、悬停行的底色跟不上，活动监视器里 sqlmux 44%、tmux 40%、kitty 33%）：悬停靠 1003 模式的全量移动上报，指针一动就是一个事件、一次 Update + View，事件率可以到每秒上百次；一帧的开销乘上这个频率才是真实负担。所以要有 profile 的开关和一个可重复的测量（§15「性能」），按测量结果优化到：160×45 的表格上每秒 100 个移动事件，sqlmux 占不到一个核的 10%，悬停底色不落后于指针。

Go 生态相对 Rust 缺三样东西，对策如下：

| 缺口 | 对策 |
|---|---|
| vim 编辑器组件 | 自研，覆盖 nvim 的基础操作（§11）。用 nvim headless 生成期望结果，做差分测试（§15） |
| SQL 词法分析 | 自研扫描器，约 300 行，配表驱动测试（§9.1） |
| SQL 格式化 | 在进程内运行成熟的 JS 库 sql-formatter，已验证可行（§9.5） |

### 2.2 limoni 评估：不采用

[thebanri/limoni](https://github.com/thebanri/limoni) 主打零分配渲染、3D、图表，以及可被 AI 驱动的语义树，和本项目的需求重合不多。关键问题是还不成熟：

- 仓库创建于 2026-08-06，写本文时约 7 周，55 个 star，205 次提交里 194 次来自同一位作者，版本 v0.8.1。
- 未关闭或刚修复的问题，恰好落在本项目依赖的交互上：
  - #48：指针移动被当成 `MousePressMsg`，悬停无法实现；
  - #49：`MouseWheelMsg` 不带坐标；
  - #19：没有可拖动分隔线的 SplitPane；
  - #18：没有 StatusBar；
  - #47：点击事件不会路由到控件，2026-09-21 才修复；
  - #53：输入事件循环还没在真实 PTY 上测试过。

## 3. 总体架构

```
 KeyPressMsg ─► keymap 解析 ──┐
 Mouse*Msg ──► 命中表查找 ────┼─► Action ─► Update（单线程）─► 状态树
 命令面板选中 ────────────────┘                   │   ▲
                                                 ▼   │ resultMsg{tab, seq}
                                             tea.Cmd ─► db.Worker（每条连接一个，互斥锁串行）

 View(): 状态树 ─► Frame（cell 缓冲区 + 命中表）─► Bubble Tea 差分输出
```

四条原则：

1. **只有一棵状态树，只在 Update 里修改。** IO 放在 `tea.Cmd` 里执行，结果以 Msg 的形式回到 Update。
2. **所有操作都是 Action。** 键位、鼠标点击、命令面板最终都落到同一个 Action 上，满足 PRD 第 2、7 章「鼠标和键盘不存在两套逻辑」的要求。
3. **模式由状态推导，不单独存储。** 编辑单元格或输入框获得焦点即为 INSERT；console 的模式就是编辑器自身的模式；命令面板打开即为 COMMAND（B-02）。
4. **视图是状态的纯函数。** 命中表是渲染的副产品，每帧重建。

## 4. 目录结构

```
sqlmux/
├─ cmd/sqlmux/main.go       参数解析、子命令（keys）、启动
├─ internal/app/            Model/Update/View、Action 注册表、工作现场模型、布局树
├─ internal/ui/             Frame、Block、主题、命中表、模糊匹配（fzf 算法的封装）、各组件绘制（grid/tree/tabs/statusline/palette/popup/cell editor/补全列表）
├─ internal/keymap/         键位记法解析、作用域 trie、次数前缀、用户映射（按 pane 类型）、冲突检测、提示查询、default.toml（embed）
├─ internal/editor/         vim 编辑器的纯逻辑：缓冲区、移动/文本对象/操作符、撤销、搜索与 : 命令行；不 import ui 和 bubbletea，console 的绘制在 ui/console.go；testdata/ 存放 nvim 差分用例
├─ internal/sqlkit/         词法扫描、分句、读写判定、自动 LIMIT、WHERE 拼接校验、补全上下文判断、格式化（内嵌 sql-formatter）
├─ internal/db/             Conn/Engine 接口、Worker、postgres/、mysql/、catalog 查询
├─ internal/config/         config / connections / state / 布局文件的读写与 XDG 路径
├─ internal/i18n/           界面文案的翻译：T(中文, 参数…)，en 表（M6 F6.4）
├─ docker-compose.yml       集成测试用的 postgres 与 mysql
├─ justfile                 常规操作的入口（M7 F7.2）
├─ .golangci.yml            golangci-lint v2 的规则（M7 F7.1）
├─ .github/workflows/ci.yml GitHub Actions：lint、vet、单测、集成测试（M7 F7.1）
└─ README.md、LICENSE、assets/  GitHub 上的说明、许可证与截图
```

依赖：

- 界面：`charm.land/bubbletea/v2`、`github.com/charmbracelet/ultraviolet`，以及 `github.com/charmbracelet/x/ansi`（宽度计算、颜色解析）。不用 bubbles 的 textinput：它按 rune 删字，退格会把 é、👍🏽 这类字素簇拆开（M1 核对时实测）。单行输入框在 F0.4 命令行的输入处理上扩展。
- 数据库驱动：`github.com/jackc/pgx/v5`、`github.com/go-sql-driver/mysql`
- 格式化：`github.com/dop251/goja`
- 模糊匹配：`github.com/junegunn/fzf/src/algo`
- 其他：`github.com/BurntSushi/toml`
- 命令行参数：用标准库 `flag`。目前只有 `keys` 一个子命令，加上 `--debug`、`--format`、`--check` 几个参数，不需要引入 kong 这类 CLI 库；等子命令多起来、或者需要 shell 补全时再换。

## 5. 核心数据模型

对应 PRD 第 3 章的 session / window / pane / tab 四级结构。

```go
type Workspace struct {
    Sessions []*Session
    Active   int
}

type Session struct {           // M5 起：session 是一个工作区，不再绑定一个连接（用户 2026-10-02 定）
    Name      string            // 默认取启动时的连接名；之后新建的按 tmux 习惯叫 1、2……，可重命名
    Conns     []*Conn           // connections.toml 里的全部连接，按文件顺序；用到时才连
    Windows   []*Window
    ActiveWin int
    RunSeq    int               // result 标题里的执行序号，如 #42
}

type Conn struct {              // 一个数据源，同 DataGrip 的 data source；每个 session 各有一份
    Spec      ConnSpec          // 来自 connections.toml，含 read_only
    State     ConnState         // 未连接 | 连接中 | 已连接 | 失败（记着错误）
    Meta      *db.Worker        // 树、列属性、计数、DDL 预览、快速 SQL、表格取数
    Main      *db.Worker        // 这个连接上所有 console 的执行、表格的保存；持有事务状态
    Manual    bool              // 事务模式：false 自动、true 手动，按连接（F5.3），这个连接上的 console 共用；新连上的都是自动，不记进 state
    Schema    string            // 树里这个连接当前所在的 schema；新 console 和快速 SQL 的默认（§8.6）
    Catalog   *Catalog          // 表、列属性、主键、行数估计的缓存
}

type Window struct {
    Name     string
    TreeOpen bool               // ⟨0⟩ schema 树是固定侧栏，不参与分割（D-04）
    Root     *Node              // 其余区域的分割树
    Focus    PaneID
    Zoom     PaneID             // 0 = 未缩放（P-03）
}

type Node struct {              // 二叉分割树
    Split Dir                   // None 表示叶子
    Ratio float64               // A 所占比例
    A, B  *Node
    Pane  *Pane
}

type Pane struct {
    ID        PaneID            // 内部稳定 ID；显示的 ⟨n⟩ 按树的遍历顺序计算
    Kind      PaneKind          // data | console | result
    Tabs      []Tab
    Cur, Prev int               // tab 栏中的 * 和 -（T-01）
}
```

Tab 的种类：

- `DataTab`：所属的 `*Conn`（M5 起）、表引用、`Query{Where, Order, Limit, Page}`、隐藏列集合、`GridState{Row, Col, Scroll, Transpose}`、当前页数据、`Edits map[CellKey]Edit`（修改后的值为文本、NULL 或 DEFAULT）、总行数（-1 表示未知）、请求序号 `seq`。
- `ConsoleTab`：文件路径、`*editor.Buffer`、所选的连接 `*Conn`（M5 起）和 schema（仅 PG，§8.6）、它在结果区里的结果 tab。执行走所选连接的 `Main`，同一个连接上的 console 共用它（用户 2026-10-02 定：每个 console 一条连接更复杂、更容易出错，这一版先复用）。
- `ResultTab`：来源 console、执行序号、`*db.Result`、`GridState`、是否固定。

**表格和 console 可以放在同一个 pane 的不同 tab 里**（M3 F3.4 起，用户要求，与 DataGrip 一致）：

- 除结果区外，pane 不再分 data / console 类型。pane 的标题图标、右侧提示（`▶ run`、schema 下拉）、按键作用域都跟着当前 tab 的类型走；tab 栏每个 tab 前面画类型图标。`Pane.Kind` 只剩「普通 / 结果区」之分，M0 修剪时说的那几张按 PaneKind 下标的并行表随之删掉。
- 结果区照旧是独立的 pane，只放结果 tab；console 的执行结果不论 console 在哪个 pane，都进底部结果区（§11）。
- 打开表时当前 tab 是 console：`↵` 不替换它，改为在这个 pane 新开一个 tab。console 里是用户写的 SQL，不能被一张表顶掉。
- 焦点不在普通 pane 上（比如在树上）时，目标 pane 取最近聚焦过的普通 pane，console 所在的 pane 也算（F3.37 用户定，取代 F3.7 的「跳过当前 tab 是 console 的 pane」：焦点在 console 时打开的表就开在这个 pane）。
- 快速 SQL 的 `C-e`（在 console 中打开）在目标 pane 新开一个 console tab。
- 目录树的工作区里，pane 节点不再标类型，tab 节点按各自的类型显示图标。
- **引导页**：没有 tab 的 pane、点 `+` 新开的 tab、分割出来的新 pane，都显示同一个引导页，不再是空白。目标 pane 的当前 tab 是引导 tab 时，打开表（`↵`、`t`、`C-t`、中键都一样）和新建 console（`c`、点击、面板都一样）都替换它，不另开 tab：引导 tab 只是「新 tab」的占位。例外（F3.37）：这个 pane 里已经开着这张表时，切到那个 tab 并关掉引导 tab。内容区中间两个按钮：「打开表」打开命令面板的表范围，选中的表开在这里；「新建 console」直接在这里开一个 console。按钮可点击，旁边的键位文字从 keymap 读。它取代 M1 里「`+` 聚焦树的过滤框」的做法：面板的表范围能跨 schema 模糊搜，更适合挑表。

**一个 session 多个连接**（M5，用户 2026-09-29 提出、2026-10-02 定，参考 DataGrip 的多个数据源）：

- **连接列表**：每个 session 都能看到 connections.toml 里的全部连接。目录树的根就是这些连接（引擎图标、名字、只读时加锁的标记），展开或第一次用到时才建 `Meta` / `Main`；连不上时节点标成失败、错误进 toast，可以再展开重试。连接名不能重复，重复时启动报错。`sqlmux play` 启动时只连 play 并展开它；不带参数时连第一个（M6 起恢复 `last` 布局）。启动时的连接连不上，照 §14 打印错误退出；其他连接失败不影响启动。
- **多个 session 保留**，作工作区用，像 tmux 的多个 session：各有自己的 window、布局，以及自己的一套连接对象（互不共享事务、search_path）。
- **console 复用所属连接的 `Main`**（用户 2026-10-02 定，先不做每个 console 一条连接）：同一个连接上的 console 共用 `Main` 的事务；search_path 照 §8.6 在每次执行前按这个 console 选的 schema 设好；两个 console 同时执行时排队（Worker 的锁）。M3「在 console 里 begin 不提交，表格的保存也会进这个事务」的已知上限照旧，由事务模式和状态栏的「事务中」提示兜住。
- **console 标题**：`<连接> · <库>.<schema> ▾`，连接和 schema 各是一个下拉框。换连接只改这个 console 下次执行用哪个连接；它当前所在的连接有未提交的事务时不让换（提示先提交或回滚）。console 的文件不再按连接名分目录（§11），记着它最后用的连接和 schema。
- **事务模式按连接**（F5.3）：每个 console 标题上 `Tx: 自动 ▾` 显示的是它所在连接的模式，在任一个 console 里切换，同一个连接上的 console 都跟着变；手动时旁边是提交 ✓、回滚 ↺，作用于这个连接的 `Main`。手动模式下表格的保存也进这个事务、不自动提交，表格取数改走 `Main` 才看得到自己没提交的修改（§8.2、§10.3 原来的写法）。有未提交的事务时，退出、关闭 session、关掉这个连接上的最后一个 console 都要确认，确认后先回滚（§10.5）；从手动切回自动被拦下，提示先提交或回滚。
- **只读按连接**：connections.toml 里写 `read_only = true`。这个连接的 `Meta`、`Main` 都设成只读（§13 两层拦截照旧），树和 console 标题上画锁的标记，表格不能进入编辑。
- **当前连接**：表 tab、console tab 各属于一个连接，结果 tab 记着来源的连接。「当前连接」是焦点 tab 所属的连接（结果 tab 取它的来源连接），焦点在树上时是树光标所在的连接（光标在还没连上的连接节点上时沿用上一次的，所以当前连接一定是连上的），引导页、空 pane 沿用上一次的。快速 SQL、新 console、`C-e`、`tree.refresh`、`C-c` 取消和 busy 提示、事务命令都按当前连接；命令面板的表范围列出所有已连上的连接的表，当前连接的排前面（§12）。状态栏的地址块显示当前连接的名字和 `<用户>@<host>:<port>`（§7.8）。
- 开发清单见 [`m5-workspace/task.md`](m5-workspace/task.md) 的 F5.10、F5.11、F5.1–F5.4（2026-10-02 定）；§7.8、§8、§10、§11–§14 已跟着改。

默认 window 的布局：

```
Window
├─ 侧栏 ⟨0⟩ schema（可折叠）
└─ Node(纵向, 0.6)              ← 第一次执行 SQL 时，把原来的根节点包进一个新的纵向节点
   ├─ Node(横向, 5:4)          ← data 与 console 的宽度比，取自设计稿（§7.8）
   │  ├─ Pane ⟨1⟩ data
   │  └─ Pane ⟨2⟩ console
   └─ Pane ⟨3⟩ result           底部全宽结果区（§11）
```

布局树的操作写在 `internal/app/layout.go` 里，都是纯函数，可以单测：

- **分割**：把叶子替换成 `Node{A: 原叶子, B: 新叶子}`。新 pane 是与原 pane 同类型的空 pane（没有 tab，样式见 §7.8），焦点移到新 pane。M3 起新 pane 显示引导页（见上文「表格和 console 可以放在同一个 pane」）。
- **关闭**：用兄弟节点替换父节点。window 里唯一的普通 pane 关不掉（`SPC x` 不做事），结果区不算在内：否则布局里只剩结果区，什么表都打不开了。
- **调整大小**：修改该方向上最近的祖先分割节点的 `Ratio`。
  - 每按一次移动 5%，乘以次数前缀（`3 SPC L` 移动 15%）；比例限制在 10%–90%。
  - 方向与 tmux 的 resize-pane 相同：`H` 把分割线往左移，与当前 pane 在分割线的哪一侧无关。
- **缩放**：pane 占满状态栏以上的整个 window，侧栏的位置也被占用（P-03「缩放至整个 window」）。分割、关闭、按编号跳转都会退出缩放。从命令面板聚焦 pane、或者把表打开到某个 pane 时也一样：目标 pane 被缩放遮住时，先退出缩放再聚焦；目标就是正在缩放的 pane 时，保持缩放。
- **按方向切焦点**：用上一帧的 pane 矩形，在该方向上找相邻、且有重叠的 pane。
  - 有多个候选时，选最近获得过焦点的那个；都没获得过焦点时，选遍历顺序靠前的（上 / 左）。这与 tmux 的 `window_pane_choose_best`（取 `active_point` 最大者）一致。
  - 选它而不选「重叠最长」，是因为这样 C-l 之后再 C-h 总能回到原处。更重要的是默认布局：从底部全宽的结果区按 C-k，会回到刚才执行查询的 console，而不是更宽的 data。
  - 与 tmux 不同的一点：到了边上不绕到另一侧。侧栏固定在最左边，vim 的 `<C-w>h` 也不绕回。
- **按编号跳转**（`SPC q`）：编号一直显示到下一个按键，不自动消失（相当于 tmux 的 `display-panes -d 0`）。按数字跳到对应的 pane；按其他键只关闭编号，这个键不再执行别的操作。
  - 鼠标：编号也是键位提示，按 §7.4 可以点击。点击某个 pane（含侧栏 ⟨0⟩），等于按下它的编号；点击 pane 以外的地方（状态栏、间隔）只关闭编号。

默认 window 名为 `data`，之后新建的叫 `data-2`、`data-3`……；`window.new` 新建的 window 只有一个空 pane（引导页），不带 console，同 tmux 的 `new-window`（M8 F8.18）；启动时的默认布局（左树、引导页、console）不变。（同一 session 里最小的没用过的序号；M8 F8.11，用户：三个 `data` 分不清）；同一 session 里 window 名不能重复，`window.rename` 改成已有的名字时 toast 拒绝。各个 pane 随里程碑逐步出现：M1 只有侧栏和 data，data 占满其余宽度；console 在 M3 加入默认布局（data : console 原为 5 : 9；M8 F8.9 起 1 : 1，用户看到两块不等宽以为是 bug，拆分出来的一对本来就是均分的；代价是 160 列下 ① 从 70 列变成 61 列，查询条按 §7.8 的让位顺序让掉转置、自动刷新、`+ −` 三组，保存 / 刷新组改成排在 LIMIT 之后让位所以还在；接受），结果区在第一次执行 SQL 时出现。

所有持久化字段都是普通值类型，运行时句柄（连接、编辑器实例）不进入序列化结构。所以工作现场恢复（PRD 10.2 #6，M6 F6.5 布局文件）只需要补上读写逻辑。

## 6. 输入系统

### 6.1 Action

```go
type Action struct {                // 注册表是 map[id]Action，id 如 "pane.split.right"
    Title string                    // which-key 和命令面板显示的名字；只在浮层、cell、input 里用的 Action 没有标题（§12）
    Run   func(*App, Args) tea.Cmd  // Args 含参数与次数 Count；还没实现的 Action 只有标题，没有 Run
    On    func(*App) bool           // 开关类命令的当前状态（K-04 的 ON/OFF）
}
```

命令面板列出所有带 Title 的 Action，所以没有绑定键位的命令也能执行（PRD 6.3）。Action 可以带参数，配置里写作 `"window.select 3"`。

### 6.2 终端兼容：默认键位只用处处可用的键

不假设用户使用 tmux 或 kitty。默认键位只用下面这些键，它们在 macOS Terminal.app、iTerm2、Linux 各种终端、SSH、tmux/screen 里都能区分：

- **可以用的**：
  - 可打印字符、Esc、Tab、S-Tab、↵、Backspace、方向键。
  - Ctrl+字母，但排除 C-i、C-m、C-[：在传统编码下它们分别等同于 Tab、↵、Esc。
  - C-h 在少数终端里等同于 Backspace，所以只在 NORMAL 模式下使用，那里 Backspace 没有用途。
- **不用的**：
  - Alt/Option 组合：macOS 的终端默认不把 Option 当作 Meta。
  - Ctrl+Shift 组合。
  - C-↵ 等带修饰键的回车。
  - Ctrl+数字：传统编码无法发送。
  - Home、End、PgUp、PgDn：macOS 的 Terminal.app 默认拿它们来滚动回滚缓冲区，按键不会传给程序；而且笔记本键盘上没有这几个键。
  - F1–F12：经常被操作系统占用，在 Mac 笔记本上还要同时按住 fn。
- **kitty 键盘协议只顺带开启**：终端支持时会打开（Bubble Tea v2 里只是一个字段），用户可以在配置里自己绑定 C-↵ 这类键。终端不支持时，这类绑定不生效，对应的提示也不显示。
  - 默认键位不依赖这个协议。有一条单测保证 `default.toml` 里不出现只能在这个协议下使用的键。
- **终端翻译过来的键**（M5 F5.17，用户要用 Cmd、Ctrl+Shift 组合）：这类键用传统编码送不进来，用户可以让终端把它翻成 CSI u 编码的键（如 `<C-S-w>`）再送进来，做法同用户 kitty + tmux 里给 nvim 的那套：kitty 把键翻成 tmux 前缀加一个键，tmux 看 pane 里跑的是什么再决定发什么。这类键绑在 `[keys.global]` 里，在任何模式下都生效，不会当成文字输进去；终端或 tmux 没有协商 kitty 协议时也要能解析。默认键位照旧不用它们。
  - **送法**（F5.17 在 tmux 3.7b 上实测）：tmux 的分支用 `send-keys -H` 写原始字节，格式 `ESC [ <码点十进制> ; <1+修饰> u`（shift 1、alt 2、ctrl 4、super 8），如 `<C-S-w>` 是 `send-keys -H 1b 5b 31 31 39 3b 36 75`。Bubble Tea 不管协商过没有都按 CSI u 解析；在 console 的 NORMAL / INSERT、表格、WHERE 的 NORMAL / INSERT、面板打开时都执行了绑定的命令，没有字符漏进输入框。tmux 自己的 `send-keys C-S-w` 不行：不论 extended-keys 怎么设，只有请求了 modifyOtherKeys 的 pane 才收到带 shift 的编码，Bubble Tea 只请求 kitty 协议，收到的是普通的 C-w。
  - **Super（Cmd）键**：带 Super、不带 Ctrl / Alt 的键记作 `<D-x>`（vim 的记法），能绑在 `[keys.global]` 里，没绑时也不会当成字符输入（F5.17 实测时发现：kitty 开着协议、Cmd+x 没被 kitty 截住时，原来会当成 `x` 删掉一个字符）。
- **兜底**：键盘不方便的操作，都能用鼠标（§7.4）或命令面板完成。

### 6.3 键位记法

配置里用 vim 记法：`<C-p>`、`<Space>`、`<Esc>`、`<S-Tab>`、`<CR>`；字符序列直接写，如 `gt`、`gq`、`%`。

界面显示成 PRD 的风格：`C-p`、`SPC`、`esc`、`↵`。

### 6.4 作用域与解析

| 作用域 | 何时生效 |
|---|---|
| `palette` / `where` / `cols` / `dropdown` / `sessions` / `form` / `complete` / `options` / `segments` / `confirm` / `keyhelp` | 对应的浮层打开时，取最上层的一个。`options` 是单元格的选项浮层，`segments` 是时间列的分段浮层，都只在展开时生效（§10.2）；`keyhelp` 是 `?` 的键位帮助（§6.5）；`sessions` 是 session 列表，`form` 是新建连接的表单（M5 F5.1，§7.8）。which-key 浮层不是作用域（§6.5） |
| `wherenormal` | WHERE 输入框在 NORMAL 下（F3.39）：`↵` 执行、`/` 历史 / 收藏、`esc` 回表格，其余交给编辑器；`normal` 照常在它下面 |
| `newrow` | 编辑还没保存的新行的格时生效（F3.33），叠在 `options` / `segments` 之上：浮层层级里它在最上，没绑的键再落到下面那层浮层。默认只有 `Tab` / `S-Tab`（`cell.field.next` / `cell.field.prev`） |
| `cell` | 正在编辑单元格（INSERT） |
| `input` | 任意单行输入框获得焦点（INSERT） |
| `result` | result pane 获得焦点时生效，优先级在 `grid` 之上（如 `P`、`q`）；其余按键落到 `grid` |
| `grid` / `tree` / `console` / `landing` | 对应控件获得焦点：按 pane 当前 tab 的类型，表格 → grid，console → console，引导页和没有 tab 的 pane → landing（M3 F3.7）。`console` 只含应用层的键（如 ↵ 执行），其余按键交给 vim 引擎；`gq` 是编辑器的操作符（§9.5） |
| `normal` | 所有 NORMAL 上下文共用：pane 焦点、leader、gt/gT。`:`、`;` M3 起在 grid / tree / landing 里（§6.8） |
| `global` | 始终生效，只有少数 Ctrl 组合：C-p、C-s、C-c |

解析顺序：最上层浮层 → 用户映射（仅在 NORMAL 上下文，§6.6）→ 焦点控件 → `normal` → `global`，先匹配到的生效。

- 实现上，把当前上下文涉及的各作用域 trie 按上述优先级合并，合并结果缓存起来复用。
- INSERT 下没有匹配到的可打印字符交给输入控件。
- console 中没有匹配到的按键序列（包括已经缓冲的前缀，如 `g` 之后的 `g`）整体交给 vim 引擎。
- **编辑器在等后续按键时，按键不经过 keymap**（M3 定）：有待输入的操作符、字符参数或文本对象前缀时（`d`、`c`、`y`、`f`、`t`、`r`、`i` / `a` 等），按键直接交给引擎，用户映射也不展开，与 nvim 在 operator-pending 下不用 normal 映射一致；只有 `C-c` 仍等同 esc。否则 `f<Space>x` 会被当成 leader 序列 `SPC x`，把 pane 关掉。

### 6.5 按键序列、leader 与 which-key

**不做 tmux 式的全局前缀键**：

- 实现上它并不复杂：前缀键和 `gt`、`gg`、`dd` 走的是同一套按键序列机制。
- 真正的问题有两个：全局 Ctrl 前缀会占掉输入框里常用的 Ctrl 键（C-a 行首、C-w 删词）；而且会和 tmux 用户自己设置的前缀冲突。

**也不能只用 Ctrl 组合键**：

- 传统终端里能用的 Ctrl+字母只有二十个左右，大半已被 vim/readline 的习惯占用；Ctrl+数字又无法发送。
- PRD 里约 30 个工作现场操作（切 window 0–9、分割、缩放、调整大小……）装不下。

**方案（已确认）**：

- NORMAL 模式下用 `SPC` 作 leader，后面接 PRD 原来跟在 C-a 后面的那些键：`SPC s` 打开 session 列表、`SPC %` 左右分割，以此类推。与 PRD 基本是机械替换关系，默认只保留常用的几个（§6.8）。
- 最常用的 pane 焦点切换另配直达键：C-h/j/k/l。
- leader 可以配置。配成 Ctrl 组合（如 `<C-a>`）时，它在所有模式下生效，也就恢复了 tmux 式前缀。

**which-key**：按键序列停在「只是前缀」的位置超过 400ms，就弹出提示浮层，列出下一步能按的键和对应操作名。每一项都可以点击。这解决了 PRD 10.2 #4。

**键位帮助 `?`**（M2/M3 验收时用户要求，同 nvim which-key 的 `:WhichKey`）：在表格、树、引导页、结果区按 `?`，打开当前上下文的全部键位，也就是按 §6.4 的顺序合并后生效的键，含用户映射，每项带操作名；前缀显示成一组，画法和 which-key 相同（`g → …`）；每一层的键按来源的配置表分组，组标题是表名（`[keys.grid]`、`[map.grid.normal]`、`[keys.normal]` 等），顺序同上面的解析顺序，只列生效的绑定，让人看出这个键属于哪个作用域、改它该改哪一节（用户验收时要求，which-key 浮层同样分组）；按下前缀往下一层、`<BS>` 回上一层，按到绑定就关掉帮助并照原来的上下文执行；`esc` 关闭。console 里只列 keymap 的绑定，vim 本身的键不列。console 的 NORMAL 下 `?` 也打开它（M3 F3.25 用户定：不用 vim 的反向搜索，`/` 照旧；绑在 `[keys.normal]`，所以 console 的 VISUAL 下 `?` 仍是 vim 的），`<leader>?` 在哪里都能用。这个浮层是作用域 `keyhelp`。按 leader 等前缀停住时弹出的 which-key 本来就按层级列出，比如 `SPC w` 之后列出 `SPC w…` 下的键。

which-key 浮层只是把待输入序列画出来，本身不是作用域：浮层打开时，按键仍按原来的上下文解析，效果与没有浮层时相同；esc 清空待输入序列，浮层随之关闭。所以配置里没有 `[keys.whichkey]` 这张表。

**序列规则**：

- 只是前缀的节点（如 `SPC`、`g`）一直等待，直到按下一个键或 Esc。
- 既是完整绑定、又是更长序列前缀的节点，等待 `timeoutlen`（默认 1000ms）后执行已有的绑定；下一个键接不上任何更长的序列时，立刻执行已有的绑定，再把这个键当成新序列的开头（nvim 的做法，M8 F8.15）。默认键位不会产生这种歧义节点，用户的会：引导页的 `t` 加自己的 `tc`。
- 状态栏显示正在等待的序列（B-03），例如 `SPC`、`g`、`5`。

### 6.6 次数前缀与用户映射

**次数前缀**：NORMAL 下按数字即输入次数。只有在尚未输入次数时，单独按 `0` 才是「移到行首 / 首列」。次数通过 `Args.Count` 传给 Action，表格、树、编辑器的移动都支持，比如 `5j`。

**用户映射**：定义「键 → 键序列」，语义同 vim 的 `noremap`，右侧不会再次展开。映射可以对所有 pane 通用，也可以按 pane 类型单独配置：

- `[map.<模式>]`：所有上下文通用，模式为 `normal` 或 `visual`。
- `[map.<上下文>.<模式>]`：只对某类 pane 生效。上下文有三种：`console`、`grid`（data pane 与 result pane 的表格共用）、`tree`。

```toml
[map.normal]              # 所有 pane 通用
J = "5j"
K = "5k"

[map.console.normal]      # 只在 console：左右各移动 5 个字符
L = "5l"
H = "5h"

[map.grid.normal]         # 只在表格：L 跳到右侧第 5 列，H 回到首列
L = "5l"
H = "0"
```

- 优先级：pane 类型的映射 > 通用映射 > 默认键位，以用户意图为准。
- 右侧按所在 pane 的含义执行：`l` 在 console 里是移动一个字符，在表格里是移动一列。
- 表格和树的默认键位没有占用 H、J、K、L。console 里这几个键本来有 vim 的含义（如 `J` 合并行），用户映射会覆盖它们，与 nvim 的 `nnoremap` 一致。
- 如果要把键绑定到某个操作，而不是一串按键，用 `[keys.<作用域>]`（§6.7）。

### 6.7 配置、冲突、提示、导出（PRD 6.3）

- **加载**：先加载内置的 `default.toml`，再用用户配置覆盖。值写成空字符串表示解绑。
- **冲突**：用户重新绑定默认键属于正常改键，不报冲突。只有一种情况会报（M8 F8.15 起；原来「单键和以它开头的序列同时存在」也警告，F8.15 之后那是 vim 的正常用法，`t` 和 `tc` 并存只有单独按 `t` 才等超时，不再报、`keys --check` 也不算）：
  - 同一作用域内，两个键规范化后相同（如 `<C-p>` 和 `<c-p>`），或者同一个键绑定了两个 Action。

  启动时用浮层列出冲突；`sqlmux keys --check` 发现冲突时以非零状态码退出。
- **标题**：每个有默认键的 Action 都要有标题（含输入框、浮层里的，如 `where.history`、`keyhelp.close`），键位帮助、面板、导出都用它；单测检查 `default.toml` 里每个绑定的 Action 都有标题（M3 F3.17 reviewer 发现）。
- **提示**：`keymap.Hint(actionID, scope)` 返回当前生效的第一个键位的显示形式。界面上所有的键位文字都通过它读取，不写死；未绑定的 Action 不显示提示。
- **导出**：`sqlmux keys` 输出当前生效的键位表（markdown），加 `--format toml` 输出可以分享的配置片段。
  - toml 输出就是一份完整的 `keymaps.toml`（M5 F5.9 起键位和映射单独放在 `~/.config/sqlmux/keymaps.toml`，M1 用户反馈要完整的键位参考）：按作用域分节，每节前一行注释写明这个作用域什么时候生效（§6.4）；**和默认相同的绑定都写成注释行**，值就是默认键，后面注释 Action 的标题，用户去掉 `#` 再改右边才生效——这样新版本改了默认键，不会被一份旧导出盖住（M3 验收期间两次出现）；用户在 keymaps.toml 里改过的（改键、解绑、leader、`[map.*]`）照旧写成生效的行，放在对应默认行的位置，所以升级后重新导出一次，既拿到新版本的完整参考，又不丢自己的改动（用户自己写的注释会丢）；没有默认键的 Action 写成 `# "" = "<action>"  # <标题>`，作用域按 id 前缀对应（`grid.` → grid，`tree.` → tree，`console.` → console，`result.` → result，其余 → normal）；`leader` 和 `[map.<上下文>.<模式>]` 的示例同样注释着放在里面（§6.6）。
  - 同一个键可以在不同作用域各绑一个 Action，按焦点所在的 pane 类型生效，例如 `R` 在表格里是刷新数据、在树里是刷新表列表（§6.4 的解析顺序本来就支持）。

### 6.8 默认键位

| 范围 | 默认键 | 操作 | PRD 原键 |
|---|---|---|---|
| 全局 | `C-p` | 命令面板 | C-k / C-p |
| 全局 | `C-s` | 保存：表格中提交修改，console 中写入文件 | C-s |
| 全局 | `C-c` | 有查询在执行时，取消查询。空闲时连按两次退出：第一次弹出 toast「再按一次 C-c 退出」，显示 2 秒；toast 还在时再按一次即退出，消失后再按，重新算第一次（与 Claude Code、node REPL 的习惯一致）。`:qa` 也可以退出。处在命令行、输入框、浮层、单元格编辑、编辑器的 INSERT / VISUAL，或者编辑器还在等后续按键（`d`、`f` 这类）时，`C-c` 的作用等同 esc（与 vim 一致），不取消查询，也不算作连按两次退出中的一次；要取消查询，在 NORMAL 下按 | 新增 |
| NORMAL | `C-h` `C-j` `C-k` `C-l` | 切换 pane 焦点 | C-a hjkl |
| NORMAL | `SPC s` | session 列表 | C-a s |
| NORMAL | `SPC c` · `SPC n` · `SPC p` · `SPC l` | 新建 · 下一个 · 上一个 · 上次用的 window | C-a c / n / p / l |
| NORMAL | `SPC %` · `SPC "` · `SPC z` · `SPC x` | 左右分割 · 上下分割 · 缩放 · 关闭 pane | C-a % / " / z / x |
| NORMAL | `SPC q` | 按编号跳转 pane | C-a q |
| NORMAL | `SPC b` | 折叠 / 展开 schema 树；展开时焦点到树上、光标落在当前 tab 对应的节点（F3.29） | C-a b |
| NORMAL | `SPC r` | 显示 / 隐藏结果区（`result.toggle`，F3.28） | 新增 |
| NORMAL | `gt` · `gT` | 下一个 · 上一个 tab | 同 |
| 表格 / 树 / 引导页 | `:` | 打开命令面板的「所有」范围（M5 F5.8 起，同 `C-p`；原来进命令范围，想要原来的就把 `:` 绑到 `palette.command`）；`:q`、`:qa`、`:w` 照常可用（§12）。M3 起绑在 `[keys.grid]`、`[keys.tree]`、`[keys.landing]`，不在 normal：console 里的 `:` 要交给编辑器自己的命令行（§11） | 同 |
| 表格 / 树 / 引导页 | `;` | 打开命令面板并直接进入 SQL 范围（§12）。绑法同 `:`，console 里 `;` 是 vim 的「重复 f/t」 | 新增（M1 用户反馈） |
| 引导页（`landing`） | `t` · `c` | 打开表（`tab.table`）· 新建 console（`console.new`） | 新增（M3） |
| 表格 | `hjkl` `gg` `G` `0` `$` | 移动，支持次数前缀；`{N}G` / `{N}gg` 跳到第 N 行（F3.36） | hjkl |
| 表格 | `↵` · `i` | 编辑单元格 | 同 |
| 表格 | `R` · `T` · `x` · `Tab` / `S-Tab` | 刷新 · 转置 · 关闭 tab（有未保存修改时需确认）· 右移 / 左移一格（M8 F8.6，所有行；新增行的编辑中 `Tab` 照旧提交并右移） | 同 |
| 表格 | `/` | 聚焦 WHERE 输入框 | 未定义 |
| 表格 | `go` `gl` `gp` `gc` | 打开 ORDER / LIMIT / PAGE / COLS 下拉 | 未定义（Q-03） |
| 表格 | `]` · `[` | 下一页 · 上一页 | 未定义 |
| 表格 | `yy` · `yl` · `p` · `yi` | 复制整行 · 复制单元格 · 把复制的行粘贴成新行 · 把行复制为 INSERT 语句（F3.32 起 `yy` 从单元格改为整行） | y / yi |
| 表格 | `r` · `o` · `dd` | 撤回这一格的修改 · 新增一行 · 标记 / 取消删除这一行（§10.1、§10.6） | 新增（M2/M3 验收） |
| NORMAL | `?` | 键位帮助（§6.5），所有 pane 通用，绑在 `[keys.normal]`（F3.25 起；原来在 grid / tree / landing 各绑一个） | 新增（M2/M3 验收） |
| NORMAL | `<leader>?` | 键位帮助，哪里都能用 | 新增（M2/M3 验收） |
| result | `P` · `q` | 固定当前结果 tab · 关闭当前结果 tab | P / q（原为固定 / 关闭 pane） |
| schema 树 | `j` `k` `gg` `G` | 移动，支持次数前缀，与表格一致 | j/k |
| schema 树 | `↵` · `t` · `/` · `R` · `l` · `h` | 打开（表）或展开 / 折叠（其他节点）· 打开（F3.37 起和 `↵` 相同）· 过滤 · 刷新 · 展开 · 折叠或到父节点（§7.8） | ↵ / C-↵ / / ；R、l、h 为新增 |
| console | `↵`（NORMAL / VISUAL） | 执行光标所在语句 · 执行选区 | ⌥↵ |
| console | `gq{移动}` · `gqq` | 格式化移动碰到的语句 · 当前语句；VISUAL 下格式化选区。`gq` 是编辑器的操作符，不在 `[keys.console]` 里 | 同 |
| console | `gs` | 打开 schema 下拉框（仅 PG，§8.6） | 新增 |
| 命令面板 | `C-t` | 打开表（F3.37 起和 `↵` 相同）；快速 SQL 的结果送到 result pane | C-↵ |
| 快速 SQL | `C-y` · `C-e` | 复制为 CSV · 在 console 中打开（写语句也走这条路） | C-y / C-e；取消 C-S-↵ |
| 单元格编辑 | 见 §10.2 | 时间分段调整、选项选择 | 新增 |
| 各浮层 | 见下表 | 各浮层内的移动与选择 | 同 PRD |

**默认键位只放常用的**（M0 用户反馈）。其余操作都能在命令面板里搜到并执行；想要快捷键的，在 `keymaps.toml` 里自己绑定（M5 F5.9 起，原来在 `config.toml`），例如：

```toml
[keys.normal]
"<Leader>h" = "pane.focus.left"   # 用 SPC h 切焦点
"<Leader>L" = "pane.resize.right"  # 用 SPC L 调整大小
"<Leader>," = "window.rename"
```

不提供「切换到第 n 个 window」这类按编号的命令：window 用 `SPC n` / `SPC p` / `SPC l` 切换，或者在命令面板的 window 范围里搜，也可以点击状态栏上的 window 名。

M5 新增的命令都不带默认键，在命令面板里执行：`conn.new`「新建连接…」、`console.conn`「切换连接…」、`tx.mode`「手动提交事务」（开关类，手动时为 ON）、`tx.commit`「提交」、`tx.rollback`「回滚」。

M5 验收后加的命令同样不带默认键（F5.16，用户在 keymaps.toml 里自己绑）：

- `pane.focus.next`「下一个 pane」：按屏幕上的位置顺时针切到下一个 pane（用户定）：以整个界面（含状态栏）的中心为圆心，各 pane（含树和结果区）按自己中心点的角度、从 12 点方向起顺时针排，从当前焦点所在的那个往下一个，到最后一个再回到第一个；角度完全相同时按 tmux 的编号顺序（从上到下、从左到右）。树收起时、结果区隐藏或还没出现时不算在内。要满足的例子：默认布局「树 · data · console，下面结果区」是 树 → data → console → 结果区 → 树；一排满高的 `[树|1|2|3]` 是 树 → 1 → 2 → 3 → 树（F5.16 审查时定：圆心含状态栏，满高的 pane 中心比圆心高半行，不再同角度）。
- `console.format`「格式化整个 console」：对整个 console 做格式化，规则同 `gq`（§9.5），算一次修改、`u` 一次撤销；光标留在原来的行号和列号（超出时夹到末尾；M8 F8.3，原来移到语句开头，用户不要光标动）。VISUAL 下格式化选区照旧用 `gq`。
- M8 F8.11–F8.13 新增、都不带默认键：`session.rename`「重命名 session…」、`console.rename`「重命名 console…」（状态栏单行输入，同 `window.rename`；三种名字各自不能重复）、`session.close`「关闭 session」（同 session 列表里的 `x`）、`palette.table`「命令面板：表」（打开面板并输入 `@`）、`conn.manage`「管理连接…」（§7.8「连接管理」，F8.16）。
- `tab.quit`「关闭 tab，空了连 pane / window 一起关」：先照 `tab.close` 关掉当前 tab（有未保存的修改照样确认）；pane 里没有 tab 了、window 里还有别的 pane，就关掉这个 pane；window 里只剩这一个空 pane（结果区不算），就关掉这个 window，结果区和里面的结果 tab（含固定的）一起关、不另外确认，同 `window.close`；是 session 里最后一个 window 时不关，留下引导页，和 `tab.close` 一样。
- `tab.reopen`「重开关掉的 tab」：每个 session 在内存里记最近关掉的 20 个表和 console tab（引导页不记，重启后不留），每次重开最近的一个，再按往前一个。开回原来的 pane，那个 pane 已经没了就开在当前 pane；表恢复关掉时的 WHERE、ORDER、LIMIT，console 打开原来的文件、用原来的连接和 schema；这个文件已经在别处开着时切过去，在别的 session 里就 attach 那个 session 再切过去（F5.16 审查时定）。这张表在那个 pane 里已经开着时（F3.37 一张表在一个 pane 里只有一个 tab），只切过去。关掉时放弃了的未保存修改不恢复。
- `session.new`「新建 session」：和 session 列表里的 `n` 相同，在哪里都能执行。
- `palette.window`、`palette.tab`（F5.19）：打开面板的「窗口」/「Tab」范围（§12）。

console 里的 `x` 是 vim 的删除字符，所以 console 的 tab 用 `:q` 关闭。

各浮层内的键位如下，全部取自 PRD。等到相应的浮层实现时，再加进 `default.toml`。

| 浮层 | 键 | 操作 | PRD |
|---|---|---|---|
| WHERE 历史 / 收藏下拉（`where`） | `C-r` · `C-n` / `Tab` / `↓` · `C-p` / `S-Tab` / `↑` · `↵` · `C-f` · `esc` | 打开 · 下一项 · 上一项（到头绕回）· 应用 · 收藏或取消收藏 · 关闭 | Q-02 |
| COLS 下拉（`cols`） | `/` · `j` / `k` · `space` · `a` · `A` · `esc` | 聚焦过滤框 · 移动 · 勾选 · 全选 · 全不选 · 先清空过滤，再按一次关闭。打开时焦点在列表上，PRD 写的「打开时焦点在过滤框」与这些键冲突，按这里做（§18） | Q-04 |
| session 列表（`sessions`） | `j` / `k` · `l` / `h` · `↵` · `n` · `x` · `$` · `esc` | 移动 · 展开 / 收起（session 下面是它的 window）· attach（停在 window 上时还切到这个 window）· 新建 session · 关闭 · 重命名 · 关闭列表。`n` 原为新建连接：M5 起 session 是工作区，新建连接改为命令 `conn.new`（§18） | S-03 |
| 命令面板（`palette`） | `↑` / `↓` 或 `C-n` / `C-p` · `Tab` / `S-Tab` · `↵` · `C-t` · `C-y` · `C-e` · `esc` | 移动 · 切换范围 · 执行 · 打开表（同 `↵`）/ 把结果送到结果区 · 复制 CSV · 在 console 中打开 · 关闭 | K-01~K-04、F-04 |
| 通用下拉（`dropdown`）：schema、ORDER、LIMIT | `C-n` / `C-p` / `↑` / `↓` · `↵` · `esc` | 移动 · 选中 · 关闭。输入直接进过滤框，所以不用 j/k。Action 为 `dropdown.up/down/select/close` | §8.6 |
| 补全列表（`complete`） | `Tab` / `C-n` / `↓` · `S-Tab` / `C-p` / `↑` | 下一项 · 上一项，到头绕回；弹出时第一项已选中。`↵`（接受，文字不变时照常执行）和 `esc`（第一次关列表）由输入框处理，不在这个作用域里 | §9.7 |
| 确认框（`confirm`） | `y` / `↵` · `n` / `esc` | 确认 · 取消；`C-c` 等同 `esc`；两个按钮都能点击 | §10.5 |
| 新建连接表单（`form`） | `Tab` / `S-Tab` · `↵` / `C-s` · `esc` | 下一个 · 上一个字段（到头绕回）· 保存 · 取消；`C-c` 等同 `esc`。开关字段（engine、read_only）上的 `space` 由表单自己处理、不进作用域，文字字段里照常输入空格 | 新增（M5 F5.1） |

### 6.9 非 vim 模式（M6 F6.6）

用户只用 vim 模式，但要照顾不用 vim 的人（2026-09-30）。`config.toml` 里 `vim = true | false`，默认 `true`。关掉之后改的只有两处，其余界面、命令、鼠标都不变：

**1. console 和 WHERE 是没有模式的普通编辑器**（F6.6a，`internal/editor` 里的另一层，和 vim 层共用缓冲区、撤销栈和绘制）：

- 一直可以输入；方向键移动；`S-←/→/↑/↓` 和鼠标拖动选择（console 和 WHERE 都能拖），选区用 `visual` 底；再输入就替换选区，输入自动配对的开括号或引号时把选区包起来（同 VS Code 的 autoSurround、DataGrip，F6.6 审查时定，§7.9）。`C-a` 行首、`C-e` 行尾（console 里 `C-e` 是执行，行尾没有默认键，想要的自己绑），`C-w` 删到词首（同 §7.9），`Backspace` / `Delete` 删一个字素或删掉选区，行首缩进里 `Backspace` 一次退一个制表位；`Tab` 补到下一个制表位（`tab_width` 一档），有选区时 `Tab` / `S-Tab` 是缩进 / 反缩进这几行（走编辑器的 `>` / `<`）。`C-z` 撤销、`C-y` 重做，撤销步在按方向键、`↵`、粘贴、格式化处断开，连续打字算一步；重做后光标放在重做内容的末尾、不恢复选区（同 VS Code / DataGrip；vim 模式照 vim，光标回这一步开始的地方），撤销后的光标两边一致。`C-c` 取消选区、关补全列表，没有可取消的时照旧算退出的第一次。Home / End、Alt+方向键这类终端分不清的键不作默认（§6.2），用 `C-a` / `C-e`。
- console 里 `↵` 换行，执行用 `C-e`（`console.run`，F6.6b 的键位层改绑；标题上的 `▶ run` 按钮照旧可以点）；有选区时执行选区，否则执行光标所在语句。`:` 只是字符：关 tab 用 tab 栏的 `×` 或面板的「关闭 tab」；格式化用面板的「格式化整个 console」，有选区时格式化选区。
- WHERE 里 `↵` 执行，`C-r` 历史 / 收藏下拉照旧；`esc` 回表格。
- 补全：弹出、`Tab` / `S-Tab` 移动、`↵` 接受都和 vim 模式相同；`esc` 只关列表，没有模式可退。
- 复制、粘贴不另配键：终端自己的选择和 `Cmd-V` 就是非 vim 用户的习惯；寄存器只在 vim 模式下有。
- 编辑器里没有 NORMAL 上下文：leader、`gt` / `gT`、`?`、`[map.*]` 在编辑器里都不生效（`SPC` 是要打的字）。能「到」编辑器的只有两类绑定：Ctrl 组合，和 `[keys.global]`；`Tab`、退格、`↵`、方向键、`esc`、`F5` 这类非 Ctrl 的绑定在这里不生效，免得编辑键被 NORMAL 的默认绑定截走。用户绑的 Ctrl 键优先于编辑器自己的 `C-a` / `C-w` / `C-z`。`C-h` 例外：§6.2 说它在一些终端里就是退格，plain 编辑器面向的正是这类用户，所以在编辑器里当退格、不切焦点，`C-j` / `C-k` / `C-l` 照旧。表格、树、引导页上这些照常。状态栏不显示模式块。界面上的键位提示在 `vim = false` 时优先取 novim 层的键（`↓`、`PgDn`、`Del`），plain console 的 tab 栏不画 `gt` / `gT`。
- 编辑器行为不再对照 nvim，nvim 差分测试不适用，用普通单测。

**2. 一套叠加的非 vim 默认键位**（F6.6b，`default-novim.toml`，`vim = false` 时叠在 `default.toml` 上，同一套解析、冲突检查和导出；`sqlmux keys` 按配置导出叠加后的结果）：

| 作用域 | 加的键 | 操作 |
|---|---|---|
| grid / tree | `←` `→` `↑` `↓` | 移动（hjkl 照旧也在：表格和树里字母不会被当成输入，留着不碍事） |
| grid | `PgUp` `PgDn` | 上一页 / 下一页（`[` `]` 照旧；Terminal.app 吃掉这两个键的人用 `[` `]`） |
| grid | `Delete` | 标记 / 取消删除这一行（`dd` 照旧） |
| console | `<CR>` 解绑、`<C-e>` | 换行交给编辑器；`C-e` 执行（所以 plain console 里 `C-e` 不是行尾） |

这一层允许用 PgUp / PgDn / Delete：它们正是非 vim 用户要的键，§6.2 的「默认键位不用」只管 `default.toml`；检查「只用处处可用的键」的单测对这一层放开这三个键，其余限制照旧。

两个键的对应 `yy` / `dd` / `gg` / `go` 这些照旧可用：它们在表格和树里不会打进输入框，which-key 也会提示。所有有标题的 Action 本来就都在命令面板里，不用再为「只有 vim 键能触发的操作」另加命令；编辑器里的 `gq`、`"+y` 对应 `console.format` 和终端自己的复制。

## 7. 渲染与鼠标

### 7.1 Frame

```go
type Frame struct {
    Buf    uv.ScreenBuffer       // Ultraviolet 的 cell 缓冲区（lipgloss.Canvas 只是在它外面包了一层，还不暴露 FillArea，所以直接用它）
    Hits   []Hit                 // 按绘制顺序追加；查找时倒序，后画的在上层
    Mouse  uv.Position           // 当前指针位置，用于悬停样式
    Theme  *Theme
    Cursor *uv.Position          // 输入框获得焦点时，终端光标的位置（§12）
}
```

- **键位提示**：Frame 不持有 keymap。键位文字由 app 通过 `keymap.Hint` 取好，再传给各个组件。
- **组件接口**：每个组件实现 `Draw(f *Frame, area uv.Rectangle)`。区域由 `ultraviolet/layout` 切分，文字用 `uv.NewStyledString(s).Draw(buf, rect)` 画进去。
- **View()**：依次画 pane、状态栏、浮层，把 `Hits` 存进 model 的缓存指针，然后返回：

  ```go
  tea.View{Content: buf.Render(), AltScreen: true, MouseMode: tea.MouseModeAllMotion, ...}
  ```
- **宽度**：Frame 和 Bubble Tea 的渲染器必须用同一种宽度算法。两者不一致时，渲染器会按自己的算法重排这一行，行尾的边框会被挤掉。统一按字素簇计算（`ansi.GraphemeWidth`）：
  - 实测 tmux 3.7b 按字素簇显示：👍🏽、❤️、👨‍👩‍👧、1️⃣、☺️、⚠️ 都占 2 列；按 wcwidth 算，分别是 4、1、6、1、1、1 列。
  - Bubble Tea v2 默认用 wcwidth，只有终端回报支持 mode 2027 时才改用字素簇。tmux 不回报，Terminal.app 连查询都不发，所以要在启动时主动切换。
  - Bubble Tea v2.0.9 没有公开的设置项。做法是在 `Init` 里返回一个 `tea.ModeReportMsg{Mode: ansi.ModeUnicodeCore, Value: ansi.ModeSet}`，它走的是终端回报 2027 时的同一条路径（`tea.go` 的 eventLoop）。这借用的是未公开的行为，所以配一个单测：终端不回报时，启动后渲染器已经切换（它会输出 `ansi.SetModeUnicodeCore`）。升级 Bubble Tea 后这条路径变了，测试就会失败；升级时也顺便看看是否有了正式的设置项。
  - 代价：只按 wcwidth 显示的老终端，遇到这类字符会错位。有人遇到时再加配置项。
  - 已知上限：East Asian Width 为 Ambiguous 的字符一律按 1 列计算，与 tmux 默认一致。这类字符包括 Nerd Font 图标所在的私用区、`·`、截断用的 `…`、带圈数字 ①–⑳。终端把 Ambiguous 字符设成双宽时（iTerm2 等有这个选项，一些 CJK 用户会打开），它们会占 2 列，边框随之错位；⓪ 属于 Neutral，不受影响，所以还会和 ①–⑳ 表现不一致。有人遇到时，再加 `ambiguous_width = 2` 之类的配置项。

### 7.2 Block

`ui.Block` 画单线边框：

- 上边框左侧嵌入 `⟨n⟩ 图标 对象名`（ascii 图标下为 `⟨n⟩ 图标 类型 · 对象名`，§7.7），右侧嵌入操作提示（如 `SPC b`、`▶ run ↵`）。
- 获得焦点时，边框和标题用 `focus` 色；未聚焦时，边框用 `border` 色，标题用 `dim` 色。
- tab 栏是内容区的最后一行，不画在边框上，与设计稿一致。

### 7.3 主题

颜色从设计稿提取，定义为语义 token。内置主题 `tokyonight-storm`，也可以自己写主题文件（PRD 第 8 章）：

- `config.toml` 里写 `theme = "<名字>"` 选择主题：先找 `~/.config/sqlmux/themes/<名字>.toml`，没有再找内置主题。
- 主题文件只需写要改的 token，没写的沿用 `tokyonight-storm`。颜色写成 `#rrggbb`。token 名或颜色写错、或者找不到这个主题时，启动报错并指出是哪一项。
- 主题文件里还可以用 `[icon]` 表自定义图标（§7.7）。

```toml
# ~/.config/sqlmux/themes/ristretto.toml
pane_bg = "#393333"
row     = "#6c6a6d"   # 当前行
cursor  = "#81817e"   # 当前单元格
number  = "#ab9df2"
string  = "#ffd866"
time    = "#fc9867"
```

终端不支持真彩时，Bubble Tea 的渲染器会按终端能力（colorprofile）自动降级。

| token | 值 | 用途 |
|---|---|---|
| `bg` / `pane_bg` | #1f2335 / #24283b | 背景 |
| `fg` / `fg_muted` / `dim` | #c0caf5 / #a9b1d6 / #565f89 | 文字 |
| `border` | #3b4261 | 未聚焦的边框 |
| `focus` | #9ece6a | 焦点、NORMAL、执行 |
| `warn` | #e0af68 | 待提交修改、键位、INSERT |
| `match` | #ff9e64 | 模糊匹配到的字符：只改前景色，不加底色，行的底色照旧（M1 用户反馈：黄底太重，参考 fzf / telescope 的效果）。补全、命令面板、树的过滤、COLS、WHERE 历史都用它 |
| `cursor` / `cursor_blur` | #3d59a1 / #2f3549 | 单元格光标（pane 聚焦 / 失焦） |
| `select` / `row` | #364a82 / #292e42 | 选中项 / 当前行 |
| `visual` | #2d3f76 | console 的 VISUAL / V-LINE / V-BLOCK 选区，以及单元格编辑、输入框的「全选」（M2/M3 验收时从 `select` 拆出，用户觉得选区不明显） |
| `error_bg` | #3b2230 | 错误栏的底色（§7.8「错误栏」） |
| `row_alt` | #1f2335 | 表格斑马纹的偶数行（比 pane 底色深） |
| `edited_bg` | #2d2a24 | 已修改单元格的底色 |
| `keyword` | #bb9af7 | SQL 关键字、VISUAL |
| `sql_string` / `comment` | #9ece6a / 同 `dim` | console 里 SQL 的字符串字面量 / 注释（M3）。`string` 是表格里字符串值的颜色，默认不着色，所以另设一个 |
| `sql_table` / `sql_column` / `sql_operator` | #2ac3de / #73daca / #89ddff | console 和 WHERE 输入框里的表名 / 列名 / 运算符（F3.27）。按词法加 catalog 缓存判断，别名、CTE 名不着色。`sql_operator` 也用于标点 `;` `,` `(` `)`（M5 F5.14，用户验收时要求 `*`、`;` 这类符号同一个颜色）；`.` 不着色，`schema.table` 里的点着了色反而碎 |
| `yank` | #e0af68 底、`bg` 字 | 复制时闪一下被复制的范围，150ms，同 nvim 的 `vim.hl.on_yank`（F3.32） |
| `info` | #7dcfff | 连接信息、COMMAND |
| `error` | #f7768e | 错误、Redis 标识 |
| `number` / `pk` / `func` | #ff9e64 / #73daca / #7aa2f7 | 数值 / 主键与 schema 值 / 函数名 |
| `string` / `time` / `bool` / `json` | 同 `fg` | 表格里对应类型的值（§7.6）。默认不着色，主题可以改 |
| `bar` | #292e42 | 状态栏和 toast 的底色。原来与 `row` 共用一个值，拆开后主题改当前行的颜色不会连带改状态栏 |
| `sep` | #2f3549 | 分隔线：侧栏内的分隔线、tab 之间的 `│`、连接地址块的底色 |

### 7.4 命中表与鼠标（PRD 第 7 章）

```go
type Hit struct {
    Rect   uv.Rectangle
    Target Target   // {Kind, Pane, I, Action}。M0 的 Kind：pane、title、tab、hint、button、item、number、border、backdrop、row、treeedge；cell、rowno、chip、segment 等随后面的功能加入
}
```

**键位提示即按钮**：界面上画出的每一条键位提示都登记为命中区，点击就执行对应的 Action。覆盖的位置包括 pane 标题右侧、tab 栏右侧、浮层底栏、which-key 浮层、命令面板右列、状态栏。因此键盘不方便的操作，总能在界面上找到可以点的地方。

| 鼠标操作 | 实现 |
|---|---|
| 单击 | 查到目标后转成 Action。例如点击单元格，等于依次执行 `pane.focus` 和 `grid.goto r c`，与键盘走同一条路径 |
| 双击 | 400ms 内第二次点中同一目标，触发它的双击操作：单元格进入编辑，pane 标题切换缩放 |
| 悬停 | 绘制时用 `f.Region(rect, target)` 判断是否悬停并据此选择样式。命令面板里悬停即触发 `palette.select i`（K-03） |
| 滚轮 | 作用于指针所在的 pane，而不是焦点 pane。纵向每格滚动 3 行，与 nvim 的默认值（`mousescroll=ver:3`）一致，表格、树、console 都一样。表格和树最多滚到最后一行贴着底边，内容一屏放得下时滚轮不起作用；console 的滚动范围与 nvim 一致（M3）。横向滚动支持 Shift+滚轮，也支持触控板的横向滑动（终端上报为滚轮按钮 6/7） |
| 拖动 | 在 pane 边界上按下即开始拖动，实时修改对应节点的 `Ratio`。拖动柄：左右相邻的 pane 之间是那 1 列间隔；上下相邻的 pane 之间没有间隔，用上面那个 pane 的下边框这一行，因为下面那个 pane 的上边框放着标题和提示，点击、双击另有用途。在 console 里拖选文本会进入 VISUAL |
| 点击浮层外部 | 浮层先登记一个全屏的 `backdrop` 命中区，点中它就关闭浮层。正在编辑的单元格失焦时，保留修改 |

终端兼容：

- 悬停依赖终端支持全量移动上报（1003 模式）。点击、滚轮、拖动在主流终端上都能用。
- 在 tmux 里使用鼠标，需要 tmux 开启 `mouse` 选项。这是 tmux 自身的行为，只写进 README。

### 7.5 半透明遮罩

终端没有 alpha 通道。遮罩的做法是：画命令面板之前，遍历缓冲区里已经画好的所有 cell，把前景色和背景色都向 `bg` 混合 60%，然后再画面板。toast 画在面板之后，不被压暗：面板开着时也会出现 toast（如快速 SQL 的「查询已取消」），压暗后几乎看不出来。状态栏照旧被压暗。

### 7.6 表格

- **虚拟滚动**：只渲染视口内的行；横向按列偏移滚动。
- **列宽**：
  1. 期望宽度取表头宽度与本页最宽的值中的较大值，上限 40（M8 F8.5 起；原来是 p90，为的是少数长值不撑宽列，现在宽列一律按比例收，p90 反而把只有一行非空的时间列压到表头宽、显示成 `2026-09-0…`）。
  2. 总宽度超出时按比例收缩，但不小于表头宽度（G-06）。M8 F8.5 起（用户：`<null>`、`pending` 被缩成 `<nu…` 看不出是什么）：期望宽度不超过 `[grid] full_width`（默认 22，放得下 `2026-09-01 00:01:00+00`）的列不收缩，只在比它宽的列之间按比例收，收到 `full_width` 为止，还放不下就横向滚动。转置时不收缩，直接横向滚动（见下文「转置时列宽不按比例收缩」）。
  3. 还放不下就横向滚动。
- **宽度计算**：按字素簇计算（§7.1「宽度」），CJK 字符和 emoji 都能正确处理。
- **单元格显示**：
  - 按列的类型分类着色（§7.3），分类按 catalog 的 `format_type`：
    - `number`（右对齐；转置视图里一律左对齐，因为一列里混着各种类型，M5 F5.7 用户要求）：smallint、integer、bigint、numeric(…)、real、double precision、oid；
    - `time`：date、time…、timestamp…（含 with time zone）、interval；
    - `bool`：boolean；
    - `json`：json、jsonb；
    - `string`：text、character varying(…)、character(…)、"char"、name、citext；
    - 其余用 `fg`：枚举（不算字符串类型）、uuid、数组、bytea 等。
  - NULL 显示为 `dim` 色的 `<null>`；超长内容用 `…` 截断；主键列的表头带钥匙图标。
  - 显示前清理控制字符：换行显示为 `dim` 色的 `↵`，Tab 显示为一个空格，其余 C0、DEL、C1 控制字符直接去掉，避免把终端控制序列画到屏幕上。替换之后再按字素簇截断。完整的值在单元格编辑（M2）里看。已知上限：数据里本来就有的 `↵` 字符也会画成 dim 色，不作区分。
  - 空串显示为空白。
  - **数据库报错**（权限不足、WHERE 写错等）：显示在 pane 底部的错误栏里（§7.8「错误栏」），表格照旧画着上一次的数据；第一次打开就出错时表格为空。出错之后，ORDER、LIMIT、PAGE 退回到画着的这份数据用的值，chip 上的操作（方向箭头、下拉框的当前项、`]` / `[`）都按它来；失败的 WHERE 留在输入框和请求里，改完再 `↵`，或直接 `R` 重试（M3 F3.20 reviewer 实测：原来 chip 显示 `id ↑`，点箭头发出去的却是失败的那列）。M3 验收前是画在内容区第一行、不画表格。
  - **取数中**：pane 里保留上一次的数据，不画「加载中」占位；忙碌提示和取消见 §8.3。
  - **点击单元格**：命中区 Kind 为 `cell`，点击等于依次执行 `pane.focus` 和 `grid.goto r c`（§7.4），M1 F1.3 就做。
  - 已修改的单元格用 `warn` 色文字、`edited_bg` 底色、点状下划线（SGR 4:4）。终端不支持点状下划线时，退化为普通下划线。
- **转置（G-05）只影响渲染**：`GridState` 始终保存数据坐标（游标 Row / Col 和视图偏移 Top / Left 都是），按键时把屏幕方向换算成数据方向，所以光标位置、视图和修改标记在两种视图之间自然保持。转置后：表头行是记录的行号（当前记录的行号用绿色），第一列是字段名（`func` 色，主键带钥匙图标），每一行按字段的类型着色；`j` / `k` 在字段间移动，`h` / `l` 在记录间移动，`0` / `$` 到第一条 / 最后一条记录，`gg` / `G` 到第一个 / 最后一个字段。
- **转置时列宽不按比例收缩**：一页 100 条记录横排时，收缩下去每列只剩行号那么宽，什么都看不见；所以转置视图各列取期望宽度，放不下就横向滚动。
- **悬停行**（G-06）：指针所在的行用 `row` 底，不移动光标，与树一致。**点击行号**（G-04）：光标移到那一行，列不变，等于 `grid.goto r <当前列>`；命中区 Kind 为 `rowno`。
- **视图跟随光标**：光标移出视图时视图跟着滚，保证光标所在的列完整可见；这个计算在 `ui.Grid` 里，app 每次移动后调用并把结果存下来，这样上移时视图不会跳回顶部（与 vim 相同）。滚轮只滚视图：纵向每格 3 行，最多滚到最后一行贴底；Shift + 滚轮和横向滚轮每格 1 列；两种滚动都把光标夹回视图内（与树一致）。
- **数据结构**：`Pane.Tabs` 是 `[]Tab`，`Tab{Name; Data *dataTab}`，M3 再加 `Console *consoleTab`，不用接口。`ui.Grid` 的行直接用 `[][]db.Val`，`ui` 只为这个类型 import `db`，`db` 不 import `ui`，不成环；这样每帧不用拷贝整页数据。
- **网格样式**：按用户要求，行和列都要有清晰的分隔（参考 DataGrip 的表格）。
  - **列**：
    - 相邻两列之间用 `│`（`sep` 色）分隔；
    - 行号列和第一列数据之间也用 `│` 分隔；
    - 每个单元格左右各留 1 列空白。
  - **表头**：
    - 表头下面画一条横线，与竖线交叉的位置用 `┼`；
    - 列名用 `func` 色，主键列带钥匙图标；
    - 横向滚动时，表头始终固定在最上方。
  - **行**：
    - 用斑马纹区分：偶数行使用 `row_alt` 底色，比 pane 底色更深一些；
    - 当前行使用 `row` 底色（比 pane 底色更亮），当前单元格使用 `cursor` 底色；
    - 行与行之间不画横线，因为终端里每条横线都要占掉一整行，一屏能显示的行数会减半。
- 转置视图用同样的样式：竖线分隔的是各条记录，横线画在第一行（行号行）的下方。

### 7.7 图标

默认使用 Nerd Font 字形。设置 `icons = "ascii"` 后改用 ASCII 替代字符，用于没有安装 Nerd Font 的环境。console 用带方框的终端图标 `nf-oct-terminal`（U+F489）。

**有图标就不再配文字**（M0 用户反馈）：Nerd Font 图标本身就能说明含义，所以图标旁边不再重复写说明文字，包括 pane 标题里的类型名（`data`、`console`）、状态栏里命令面板入口的 `C-p`、侧栏过滤行的 `/`。ASCII 字符表达不了含义，所以 `icons = "ascii"` 时这些文字照常显示。命令面板里 pane 那一行的名称仍然带类型名，因为那是用来搜索的文字。

主题文件里可以逐个覆盖图标（M0 用户反馈，写法参考 yazi 的 `theme.toml`）：

```toml
[icon]
console = { text = "\uf489", fg = "#78dce8" }   # 换字形，也换颜色
table   = { fg = "#a9dc76" }                     # 只换颜色
```

- `text` 换字形，`fg` 换颜色，两者都可以只写一个。没写的沿用 `icons` 选的那一套（nerd / ascii），`icons = "ascii"` 时覆盖照样生效。
- 写了 `fg` 的图标在任何位置都用这个颜色；没写时跟随所在位置的颜色，比如标题聚焦时是 `focus` 色。
- **图标后面留 1 个空格再接文字**（M1 用户反馈）：树、面板的搜索行与候选、pane 标题、侧栏标题、状态栏、查询条都一样。Nerd Font 图标按 1 列计算（§7.1），但不少终端和字体把它画得比 1 列宽，溢出到后一格，紧挨着文字时看起来贴在一起，悬停底色也只盖住半个。留一格空白正好接住溢出的部分。
- 默认颜色：`save`、`refresh`、`transpose` 用 `info` 色，`sort_asc` / `sort_desc` 和 WHERE 历史下拉里的 `star` 用 `warn` 色（收藏和历史只靠图标区分，黄星一眼能认出）；都可以在 `[icon]` 里用 `fg` 覆盖。
- 可以覆盖的图标：`schema`、`table`、`data`、`console`、`filter`、`search`、`keys`、`conn`、`key`、`postgres`，以及命令面板用的 `command`（nf-fa-bolt，U+F0E7，ascii 为 `:`）和 `window`（nf-fa-window_restore，U+F2D2，ascii 为 `[]`）；查询条的 `save`（U+F0C7，ascii `[S]`）、`refresh`（U+F021，ascii `[R]`）、`transpose`（U+F0EC，ascii `[T]`）、`sort_asc`（nf-fa-sort_amount_asc，U+F160，ascii `↑`）、`sort_desc`（nf-fa-sort_amount_desc，U+F161，ascii `↓`）；树的 `view`（nf-fa-eye，U+F06E，ascii `v`）、`column`（nf-cod-symbol_field，U+EB5F，ascii `-`）、只读连接的 `lock`（nf-fa-lock，U+F023，ascii `RO`，M5 F5.3）；查询条新增的 `row_add`（nf-fa-plus，U+F067，ascii `+`）、`row_delete`（nf-fa-minus，U+F068，ascii `-`）、`auto_refresh`（nf-fa-clock_o，U+F017，ascii `@`）、`stop`（nf-fa-stop，U+F04D，ascii `#`）；结果区的 `result`（nf-fa-list_alt，U+F022，ascii `=`）、`pin`（nf-oct-pin，U+F435，ascii `*`）、`export`（nf-fa-download，U+F019，ascii `>`）、`close`（nf-fa-times，U+F00D，ascii `x`）、日志 tab 的 `log`（nf-fa-file_text_o，U+F0F6，ascii `L`）；WHERE 历史下拉的 `star`（nf-fa-star，U+F005，ascii `*`）、`history`（nf-fa-history，U+F1DA，ascii `h`）；树的工作区里 pane 节点的 `pane`（nf-fa-columns，U+F0DB，ascii `|`，M5 F5.15）；新建连接表单的 `tag`（U+F02B，ascii `#`）、`host`（U+F233，ascii `@`）、`port`（U+F1E6，ascii `:`）、`user`（U+F007，ascii `~`）、`database`（U+F1C0，ascii `=`）、`advanced`（U+F013，ascii `+`）、`link`（U+F0C1，ascii `&`）（M7 F7.4；ascii 都是一个字符，不和 key / pin / star 的 `*` 撞）。M3 起去掉 `data`：表 tab 在 pane 标题、tab 栏、树的工作区、面板的 pane 行里一律用 `table` 图标，console tab 用 `console`，引导 tab 不画图标。`mysql`（nf-dev-mysql，U+E704，ascii `my`，M5 F5.4）。以后新增的图标也按名字加入。名字写错时启动报错。

### 7.8 默认尺寸与样式（取自设计稿）

设计稿按 13px 等宽字体绘制，一列约 7.8px。下面的数值由此折算成终端的行和列；设计稿里没有标注的，由实现自行取整。

- **整体布局**：
  - 最外层不留边距。
  - 横向相邻的 pane 之间留 1 列空白，纵向相邻的不留。
  - data 与 console 的宽度比为 1 : 1（M8 F8.9 起；设计稿是 5 : 4）。
- **schema 侧栏**：
  - 宽度默认 32 列（含边框；设计稿为 250px），config.toml 的 `[tree] width` 可改（M8 F8.22，用户要 300px 左右，即 38 列；范围 16 到窗口宽度的一半，超出夹住；布局文件里记的 `sidebar_width` 优先，拖过的宽度照旧记在 window 上）。窗口宽度小于 100 列时，缩到 24 列。
  - 可以用鼠标拖动侧栏和右边 pane 之间的那 1 列间隔来调宽度，最窄 16 列，最宽为窗口宽度的一半；拖过的宽度记在 window 上。折叠时不能拖。
  - 标题为 `⟨0⟩ <conn 图标> <session 名>`，例如 `⓪  seed`，右侧提示仍是 `SPC b`；放不下时先去掉 `SPC b`，再截短 session 名。点击标题只聚焦树。schema 不再放在标题上、也不再用下拉框切换，改成树里的节点（M1 用户反馈：用下拉框切 schema，看不出还有别的 schema；参考 sqmeow.nvim 的层级树）。
  - 折叠后是 3 列宽的细栏（设计稿为 24px），左右各 1 列边框、中间 1 列内容：顶部显示 `»`，下面竖排 `schema · SPC b`，每行一个字符。
  - 侧栏内部从上到下依次是：
    - 第一行：过滤图标（`info` 色）、`160 tables`（`dim` 色，已连上的各个连接所有 schema 的表和视图总数）；ascii 图标下，图标后面还有 `/`（`fg` 色）（§7.7）。按 `/` 后这一行变成输入框（INSERT），照 nvim-tree 的 live filter：边输入边过滤，只匹配表和视图的名字，匹配项的上级节点全部展开显示，其余节点隐藏（包括工作区整块，以及已展开的表下面的列），顺序不变，匹配到的字符高亮；清空过滤后回到原来的展开状态；`↵` 退出输入框、保留过滤、光标放到第一个匹配上；`esc`（及等同 esc 的 `C-c`）清空过滤并退出。有过滤条件时这一行显示 `<图标> <过滤文字>`，右侧 dim 色显示 `3/14`；
    - 一条分隔线（`sep` 色）；
    - 层级树（M1 F1.12）：

      ```
      ▾ <postgres> play
        ▾ <schema> public
          ▾ <table> Tables (8)
            ▸ <table> t_order                6.0k
            ▾ <table> t_user                   50
                <key> id                     int8
                <column> name                text
          ▸ <view> Views (1)
        ▸ <schema> agentable
      ▸ <postgres> seed <lock>
      ▾ <window> 工作区
        ▾ <window> data
          ▾ ① <data>
              <table> t_order  status = 'done'
              <table> t_order
      ```

      - 每层缩进 2 列。有子节点的节点前面是 `▸`（折叠）/ `▾`（展开），`dim` 色；叶子节点在同一位置留空。
      - **连接节点**（M5 F5.10 起，原来是 session 节点）：connections.toml 里的每个连接一个，按文件顺序；引擎图标加连接名，只读连接（F5.3）名字后面加 `lock` 图标。四种状态：未连接时折叠、没有子节点；连接中名字后面跟 `dim` 色的 `…`；失败时名字用 `error` 色，错误进 toast（`<连接名>：<驱动的错误>`）；已连接时下面是 schema。在未连接或失败的节点上展开（`l`、`↵`、单击、`R`）就去连，失败的就是重试，连接中再展开不做事；两个连接上开着同名表时，tab 名和 pane 标题照旧只写表名，焦点所在的连接看状态栏的地址块（已知上限，要区分时再给 tab 加 `[play]` 这样的后缀）；`password_cmd` 也在 Cmd 里跑；连上后加载 catalog，并照「默认展开」展开它。
      - **schema 节点**：非系统 schema，按名字排。
      - **分组节点**：`Tables (n)`、`Views (n)`，n 为这个 schema 下的个数；n 为 0 时照样显示，没有 `▸`。Tables 含普通表、分区表的父表、外部表；Views 含视图和物化视图。函数、序列、角色等不列：目前对它们没有可做的操作，有用处时再加。
      - **表 / 视图节点**：图标（`func` 色）+ 名字 + 右对齐的行数量级（`border` 色）；普通视图没有行数，不显示。
      - **列节点**：展开表的时候才从 `Meta` 取列，与打开表共用列缓存（§8.4），取回之前不显示子节点，也不显示「加载中」。每行是图标 + 列名 + 右侧 `dim` 色的类型（`format_type`）；主键列用 `key` 图标（`pk` 色），其余用 `column` 图标。
      - **右键菜单**（M8 F8.23，用户要在树里删 console 和表）：在树的节点上右键（SGR 按钮 2）弹出一个小菜单，贴着指针、`select` 底高亮、可点击，作用域 `menu`（`j` / `k` / `↑` / `↓` 移动，`↵` 执行，`esc` 或点别处关闭），键位文字从 keymap 读。菜单项按节点：console 文件 → 打开、重命名…、删除…；表 / 视图 → 打开、在新 tab 打开、删除表…（视图是「删除视图…」）；其他节点不弹。「删除表…」走危险操作的确认框（§13，同没带 WHERE 的 delete：红色标题，正文是要执行的 `DROP TABLE "public"."t_log"`，`y` / `↵` 确认），在那个连接的 `Main` 上执行，成功后关掉这张表的全部 tab（所有 session）、重载 catalog、toast；`confirm_dangerous = false` 也照样问（删表没有不问的理由）。手动事务模式下拒绝删表（toast「手动事务模式下不能删表：先提交或回滚，或切回自动」：PG 里 DROP 进了事务树和 tab 都不知道该怎么办，MySQL 隐式提交会连用户手上的改动一起提交；要在事务里删表去 console 写）。按对象类型用 `DROP TABLE` / `DROP VIEW` / `DROP MATERIALIZED VIEW` / `DROP FOREIGN TABLE`，不带 CASCADE。键盘上同样的事：表 / 视图节点上 `dd`（`tree.delete`，F8.21 的同一个 Action，按节点类型决定删文件还是删表）。只读连接上拒绝、toast。
      - **Consoles 节点**（M8 F8.21，同 DataGrip 的「文件」列表）：工作区节点下面再一个根节点，列出 `consoles/*.sql` 的全部文件加上开着但还没落盘的 console（没打过字的没有文件但占着编号；按名字，`console_<n>` 按 n 排，改过名的按字母排在后面；目录列表缓存在 App 里、在新建 / 关闭 / 改名 / 删除 / 第一次落盘时重读，不每帧读目录），每项 `console` 图标 + 名字，右端 `dim` 色的连接名（state.json 里记的最后一次连接，没有就空）；正在某个 session 里开着的名字用 `focus` 色。`↵` / 单击：已经开着就聚焦过去（别的 session 开着就 attach 过去，同 `tab.reopen` 的规则），没开着就在当前 data pane 里新开一个 tab，连接按 §11（记下的连接，不在了用当前连接）。`dd`（`tree.delete`「删除 console 文件」，只对这个节点下的项有效）：确认框「删除 console_3？」后删文件和 state.json 里的记录；正在开着的拒绝、toast「先关掉它的 tab」。新建、关闭、改名、删除后列表跟着变。
      - **工作区节点**：当前 session 的 window → pane → tab，最小粒度是 tab（M1 用户反馈）。pane 节点显示为 `<pane 图标> pane-<n>`（M5 F5.15 起画图标 `pane`，用户验收时觉得前面空一格很怪；原来不画、留空对齐），只列普通 pane 和结果区，空 pane 没有子节点；tab 节点比 pane 节点多缩进一级，结果区的「日志」tab 画图标 `log`（F3.30）；在 pane 节点上 `↵` 或单击是展开 / 折叠，不跳转；在 tab 节点上才聚焦过去；tab 节点显示表名加 `dim` 色的条件摘要（WHERE、非默认的 ORDER）。打开、关闭、切换 tab 后跟着变。
      - **默认展开**：启动时的连接、它 `current_schema()` 所在的 schema 和那个 schema 的 Tables、工作区及其下所有节点；其余折叠，其他连接也折叠着、不连。之后连上的连接同样展开它的 `current_schema()` 和 Tables。展开状态记在 session 上（节点按连接区分），只在内存里。
      - 行数量级：n < 1000 原样显示；1000 以上用 k / m / b 后缀，一律向下取整，首位只有一位数字时保留一位小数（`8.1k`、`2.1m`），否则取整（`48k`、`410k`）；`reltuples < 0` 显示 `?`。
      - 右侧注释（行数、列的类型）放不下时整个不显示，先保名字；侧栏只有 24 列时列节点的名字只剩十来列，可以拖宽侧栏。
      - 名字放不下时按字素簇截短并加 `…`，与 pane 标题、面板、表格一致；直接截掉会让 `mv_order_by_stat` 看起来像另一张表的全名（tester 在 F1.3 提出）。
      - 光标行和「当前打开的表」分开画：光标行在树聚焦时 `select` 底，失焦时 `row` 底，与表格的 cursor / cursor_blur 同理；悬停行 `row` 底，不移动光标。「当前打开的表」指 `↵` 会打开到的那个 data pane（§12 的规则）当前 tab 里的表，它的表节点图标和名字用 `focus` 色；工作区里，同一个 pane 的当前 tab 节点也用 `focus` 色。两处高亮说的是同一件事，焦点在树上时也有提示。
      - 光标移出可视区域时列表跟着滚；滚轮只滚视图，光标被夹回视图内（与 nvim 相同）。
    - 一条分隔线；
    - 提示行，复用 tab 栏 / 面板底栏的 `hintRow`：`j/k move · ↵ open`（F3.37 起去掉 `t tab`），全部 `dim` 色，` · ` 分隔，每一项都能点击（§7.4），放不下的项整项不显示。
  - **按键**：`j` / `k` / `gg` / `G` 在所有可见节点间移动，支持次数前缀。`l` 展开，已展开时移到第一个子节点；`h` 折叠，已折叠或是叶子时移到父节点（nvim-tree、neo-tree 的习惯）。`↵`：表 / 视图节点打开表（§7.8「打开已有的表」）；列节点打开所属的表，并把光标移到这一列；工作区的 tab 节点切过去并聚焦（在别的 window 里时先切 window），pane 节点展开或折叠，window 节点切到这个 window（F5.2；`l` / `h` 照常展开、折叠）；连接节点见上；其余节点展开或折叠。列节点打开已有的 tab 时，光标移到这一列；这一列被 COLS 隐藏了的话，光标不动，也不取消隐藏。叶子节点上按 `l` 不做事。`t`：表 / 视图节点上和 `↵` 相同（F3.37 起），其他节点上无效；Action `tree.open.tab`、`palette.open.tab` 的标题都叫「打开」。`/` 过滤，`R` 刷新。树里不再用 `gs`。
  - **鼠标**：单击 `▸` / `▾` 或分组、schema、连接节点，展开或折叠（未连接的连接节点同时去连）；单击表 / 视图 / 列节点与 `↵` 相同，中键与 `t` 相同；单击工作区的 tab 节点切过去，单击 window 节点切到这个 window（F5.2），单击 pane 节点展开或折叠；和点树上别的节点一样，焦点到树上，不会跳到那个 pane。
  - **每个连接的 Schema**（M5 起，原来是「树当前的 schema」；快速 SQL 的 search_path、新 console 的默认 schema、面板表范围的排序、快速 SQL 补全的表名，都用当前连接的）：光标停在某个连接的 schema 或它下面的节点上时，这个连接的 Schema 跟着变成那个 schema；光标在连接节点或工作区里时不变；初始为 `current_schema()`，为 NULL 时取第一个 schema。
  - **从树里打开表**（`↵` / `t` / 单击 / 中键）后，焦点移到 data pane，与命令面板一致（§12「打开表就是为了接着看数据」），也与 nvim-tree 的 `<CR>` 一致。要连续浏览几张表时按 `C-h` 回到树。
  - **连接节点下是空的直到 catalog 加载完成**，不显示「加载中」；加载出错用 toast 显示数据库的错误。`R` 重新加载当前连接（焦点在树上时是光标所在的连接）的 schema 和表列表，并清掉它缓存的列信息（Action `tree.refresh`）；展开状态保留。
- **pane 标题**：
  - 格式为 `⟨n⟩ <图标> 对象名`，ascii 图标下为 `⟨n⟩ <图标> 类型 · 对象名`（§7.7）。从左上角往右 1 列开始，两侧各留 1 个空格。
  - 编号 `⟨n⟩` 在 Nerd Font 图标下显示为带圈数字 `⓪ ① ② … ⑳`，只占 1 列（M0 用户反馈）；超过 20 时仍写成 `⟨21⟩`。ascii 图标下照旧是 `⟨n⟩`。标题、侧栏标题、命令面板里 pane 那一行都照此显示。`SPC q` 在 pane 中央显示的编号仍是普通数字，因为那是要按的键。
  - 带圈数字按 1 列计算，已知上限见 §7.1「宽度」。
  - 右侧提示离右上角 1 列。
  - console 标题的右侧从左到右是：事务控件 `Tx: 自动 ▾`（F5.3，手动时后面跟着提交 ✓、回滚 ↺）、连接和 schema 的下拉框 `play · sqlmux_play.public ▾`（F5.11 起加上连接；原来只有 `doraemon.public ▾`）、`▶ run`（`focus` 底、`bg` 色字、粗体）加上 `↵`（`dim` 色）。
    - 连接名和 `<库>.<schema>` 各是一个下拉框，点哪一段开哪一个：连接名用 `info` 色（同状态栏的地址块），`<库>.<schema> ▾` 照旧 `pk` 色，中间的 ` · ` 不能点，悬停只亮那一段；两个下拉框是同一个组件，位置、按键同 §8.6；`gs` 照旧打开 schema 下拉，命令「切换连接…」（`console.conn`）打开连接下拉。连接下拉列出全部连接（名字；每项的图标——只读的 `lock`、MySQL 的引擎图标——等 F5.3 / F5.4 给下拉框加上每项一个图标时一起画），当前的用 `pk` 色标出；选了未连接的就关掉下拉去连，连上之后才换过去（schema 取那个连接的 Schema），连接中又选了别的以最后一次为准，连不上 toast `<连接名>：<错误>`、留在原来的连接。只读连接在标题的连接名后面也画 `lock`，和连接名同一个命中区（`play <lock> · db.public ▾`）。所在连接有未提交的事务时不能换（§5）；这个 console 的执行还没回来时也不能换，toast「正在执行，结束后再换连接」（执行里可能有 begin，回来后原连接停在事务里、上面却没有 console 了，reviewer 在 F5.11 复现）。MySQL 的第二段是 `<database> ▾`（§8.6）。
    - `Tx: 自动 ▾` / `Tx: 手动 ▾` 点开是自动、手动两项的下拉，显示的是所在连接的模式（§5）。✓ 为 `focus` 色、↺ 为 `info` 色，悬停 `select` 底；所在连接不在事务里时两个都是 `dim` 色、不能点（同 DataGrip，也同查询条的停止按钮）。`Tx: …` 用 `fg` 色，下拉里当前那项 `pk` 色；✓ ↺ ▾ 不是 Nerd Font 字形，ascii 图标下照样用。
    - 提交、回滚（✓ ↺、`tx.commit` / `tx.rollback`）在当前连接的 `Main` 上用不可取消、带超时的 ctx 执行（同 §10.3 的 COMMIT），不在事务里时不做事。成功不弹 toast：状态栏的「事务中」消失、✓ ↺ 变暗就是反馈；失败 toast 错误。在出错的事务里提交，PG 实际做的是回滚，toast「事务已出错，提交改为了回滚」，免得以为已经提交了。提交、回滚之后（包括确认框里的回滚）重新取这个连接上打开着的表格，计数也重算：手动模式下表格读的是 `Main`，不重取的话屏幕上还是已经撤掉的值；同时清掉列缓存、重新加载这个连接的 catalog：手动模式下 DDL 执行时还没提交，在 `Meta` 上重载看不到它。
  - **空间不够时，提示优先于标题的对象名**，因为提示都是可点击的按钮（§7.4）。按下面的顺序分配空间：
    1. 先给最短的标题 `⟨n⟩ <图标>`（ascii 下是 `⟨n⟩ <图标> 类型`）留出位置。
    2. 按优先级从高到低依次摆放右侧的提示：放得下就保留；放不下就跳过这一个，继续试下一个。每个 pane 的提示按重要性排序，console 的顺序是 `▶ run` > 连接和 schema 的下拉框（两段算一项）> 事务控件（`Tx ▾` 和 ✓ ↺ 算一项）> 键位文字 `↵`。所以宽度较窄时，可能显示 `▶ run ↵`，但没有下拉框。
       键位文字依附于它所属的按钮：按钮没显示，它的键位文字也不显示。例如 `↵` 只能跟着 `▶ run` 一起出现，不会单独出现。
    3. 剩下的空间再给标题里的对象名；放不下的部分截掉，末尾加 `…`，如 `⟨2⟩ console · cons…`。
    4. 连最短的标题都放不下时，只保留 `⟨n⟩`。
  - **空 pane**（没有 tab）：标题栏只显示 `⟨n⟩`（M3 F3.7 起，ascii 下也一样），右侧不显示任何提示，内容区显示引导页。这些提示都作用于当前 tab：`▶ run` 执行 tab 里的内容，schema 下拉框是 tab 的属性（§8.6）；没有 tab，它们就没有作用对象。新建 tab 用 tab 栏里的 `+`。
- **查询条**（Q-01～Q-06，data pane 的表格上方，两行；设计稿只给了大致布局，细节在 M1 F1.4 定）：
  - 第一行：`keyword` 色的 `WHERE`，后面是输入框，占满整行。输入框为空且没有聚焦时只显示 `WHERE`，不加占位提示。点击这一行就开始输入（Q-01 的鼠标一栏）。行的右端是历史 / 收藏入口 `▾`（F1.5）：点击时若还没开始输入，先进入 WHERE 输入，再打开下拉。
  - 第二行从左到右：
    - 四个 chip：` ORDER id <方向图标> `、` LIMIT 100 `、` PAGE 1/60 `、` COLS 10/10 `。标签 `dim` 色、值 `fg` 色、`sep` 底，悬停 `select` 底，点击等于按对应的键（`go` `gl` `gp` `gc`）。ORDER 的方向用 `sort_asc` / `sort_desc` 图标（§7.7），它本身是一个按钮，见下面的 ORDER；
    - 图标按钮（M2/M3 验收时重新设计，见下面「工具按钮」；这一段是原来的三个按钮）：保存（M2 之前只占位，点击不做事）、刷新（`grid.refresh`）、转置（`grid.transpose`），图标见 §7.7。每个按钮画成 ` <图标> `，左右各留 1 列，按钮之间隔 1 列；悬停时整个按钮 `select` 底，命中区覆盖整个按钮（M1 用户反馈：图标紧贴、悬停只亮半个，见 §7.7「图标后面留空格」）。保存在 M2 之前没有命中区，悬停不亮，免得让人以为能点；M2 起有命中区，点击执行 `save`，有修改时画成 ` <save 图标> 3 `，3 是这个 tab 所有页的修改格数（按格计），没有修改时只有图标（Q-05）；
    - 最右边：`auto · 6000 行 · 12ms`。「行」是计数结果而不是本页行数（本页固定 100 行没有信息量）：计数完成前 `…`，超时 `?`，用估计值时 `~1.2m`（量级格式同侧栏）。
  - 查询条和表格之间不画分隔线。
  - **工具按钮**（M2/M3 验收时定，取代上面的三个按钮；F3.26 重新分组：用户嫌每个图标一块底色难看）：分四组 `+ −` ｜ 保存 刷新 ｜ 自动刷新 停止 ｜ 转置。M5 F5.5 起（用户嫌一组一块底色有割裂感，参考 DataGrip 的工具栏）：整排按钮共用一条 `sep` 底，组之间用 `border` 色的 `│` 分隔，转置也在这条底里；按钮各占 ` <图标> `；悬停时只把那一个按钮画成 `select` 底。下面的按钮说明按原来的三组写，颜色与行为不变：
    - 数据：新增行 `row_add`（`focus` 色，同 `o`）、删除行 `row_delete`（`error` 色，同 `dd`）、保存 `save`（`info` 色，有修改时带数字）；
    - 查询：刷新 `refresh`（`info`）、自动刷新 `auto_refresh`（`info`，开着时 `warn` 色并带间隔，如 ` <图标> 5s `）、停止 `stop`（这个 tab 有请求在跑时 `error` 色、可点，否则 `dim`、不可点，同 `C-c`）；
    - 视图：转置 `transpose`（`info`）。
    - 颜色都可以在主题的 `[icon]` 里用 `fg` 覆盖（§7.7）；有状态的按钮，覆盖的是「亮着」时的颜色（停止有请求在跑、自动刷新开着），停止空闲时照旧 `dim`，自动刷新关着时用 `info`。放不下时整组舍去，顺序为转置 → 自动刷新 / 停止 → `+ −` → 保存 / 刷新（F3.26）。
    - 「有请求在跑」指这个 tab 的取数、计数或保存还没回来。
  - **自动刷新**：点按钮打开下拉框（关 / 2s / 5s / 10s / 30s / 60s；Action `grid.refresh.auto`，没有默认键；按钮让位了、从面板打开时，下拉框同 schema 下拉框锚在这个 pane 查询条的右端，右边对齐 pane 的右边框），按间隔重取当前页和计数，不清掉保存结果那行提示。只在这个 tab 是某个可见 pane 的当前 tab、没有未保存的修改、没有在编辑单元格、没有请求在跑时才触发，不满足就跳过这一轮；有修改时自动暂停，免得修改被刷掉。间隔按 tab 记，不持久化。
  - **第二行放不下时谁让位**（M3 默认布局里 ① 只有 70 列左右）：行数统计 `auto · 6000 行 · 12ms` 最先让位，放不下就不显示；然后依次是转置、自动刷新 / 停止、`+ −` 三组（F3.26），再是 COLS、PAGE、LIMIT，然后才是保存 / 刷新那一组（M8 F8.9 审查时改：它带着未保存的修改数，比 LIMIT 100 要紧；原来排在 COLS 之前，1:1 的默认布局在 160 列下会把四组按钮全让掉），最后 ORDER。保存结果的优先级高于 chip 和按钮，放不下时照上面的顺序隐藏，直到放得下，再放不下按 §10.3 中间截短。保存结果那行文字消失后，被隐藏的 chip 和按钮照常回来。
  - **ORDER**（`go`）：通用下拉框列出所有列，第一项「默认」即按行标识列排。在当前排序列上按 `↵` 翻转方向，在别的列上按 `↵` 按 `[grid] order` 的方向（默认 asc；M8 F8.8，用户常倒序看最新数据，配成 desc 后新打开的表和新选的列都先降序；布局文件里记的方向优先）；只支持单列（Q-03）。默认排序时 chip 上显示的那一列（行标识列；复合键时是它的第一列）就算当前排序列：chip 显示 `id ↑` 时选 `id` 得到 `id ↓`，chip 显示 `occurred_at,id ↑` 时选 `occurred_at` 得到 `occurred_at ↓`，不能只是从「默认」变成显式的 `id ↑`，否则用户看着 chip 会以为按了没反应（tester 在 F1.4 提出）。SQL 为 `ORDER BY <列> <方向>, <行标识列>`，行标识列作 tiebreaker 保证分页稳定，选的就是行标识列时不重复。换了排序后回到第 1 页（和 LIMIT 一样）。chip 默认显示行标识列 `id` 加升序图标，没有行标识列时显示 `—`、不画方向图标。点击方向图标切换升降序（Action `grid.order.toggle`，标题「切换排序方向」，没有默认键），规则同上：默认排序时点击得到行标识列降序；行标识列是复合键时（如 `occurred_at, id`），得到按第一列降序，其余键列照旧作 tiebreaker，与下拉框里选第一列的规则一致；点击 chip 的其余部分仍打开下拉框（设计稿如此，M1 用户反馈）。
  - **LIMIT**（`gl`）：同一个下拉框，100 / 500 / 1000，换了之后回到第 1 页。过滤框里可以直接输入正整数作为自定义的每页行数（M1 用户反馈）：输入是正整数时，候选第一项就是这个数，`↵` 应用，后面照旧列出匹配到的 100 / 500 / 1000；超过 10000 按 10000，因为一页数据全在内存里，再大会卡。`0`、负数、小数不当数字，照常过滤。
  - **PAGE**（`gp`）：chip 原地变成页码输入框 `PAGE [3_]/60`（INSERT），`↵` 跳页（超出范围夹到边界），`esc` 放弃。总页数在计数未知时是 `?`，估计值加 `~`。`]` / `[` 翻页：已是最后一页（没有下一页）时 `]` 不起作用，第 1 页时 `[` 不起作用。翻页后光标的行、列保留，夹在新页范围内。
  - **COLS**（`gc`）：打开时焦点在列表上：`j` / `k` 移动、`space` 勾选、`a` / `A` 全选 / 全不选、`esc` 先清空过滤再关闭；`/` 进过滤框（模式仍是 COMMAND），`↵` 或 `esc` 回列表，过滤文字保留。每行 `[x] 列名  类型` 加主键标记，顶部显示匹配数。隐藏的列按 tab 记住；隐藏的正好是光标所在列时，光标挪到最近的可见列。chip 显示 `可见列数/总列数`。键位在 `[keys.cols]`。
  - **WHERE**（F3.39 起是一行的 vim 编辑器，同 console：INSERT 下 `esc` 到 NORMAL，NORMAL 下能用 `dd`、`ciw` 这类操作，`/` 打开历史 / 收藏下拉；NORMAL 下再 `esc` 才回表格。F6.6 关掉 vim 模式时是普通输入框）：`↵` 应用输入、回到第 1 页、重新取数和计数、焦点回表格；回表格时输入框恢复成当前生效的那条，所以输入框显示的总是正在生效的条件；执行出错时例外，失败的条件留在输入框里。报错显示在 pane 底部的错误栏（§7.8「错误栏」），查询条仍在上面，方便改了再执行。
  - **`R`**（`grid.refresh`）：按当前 WHERE / ORDER / LIMIT / PAGE 重新取当前页并重新计数。
- **tab 栏**：
  - 位于 pane 内容区的最后一行，底色为 `bg`（比 pane 底色深），不画分隔线。
  - 每个 tab 显示为 ` 序号:名称标记 `，tab 之间用 `│`（`sep` 色）分隔。
  - 当前 tab 用 `pane_bg` 底色、`focus` 色字；其他 tab 用 `dim` 色字。
  - tab 之后是可点击的 `+`；最右端是 `dim` 色的键位提示，如 `hjkl · ↵ edit · T 转置 · gt/gT`，文字从 keymap 读取。
  - **切换**（M1 F1.6）：`gt` / `gT` 照 vim，下一个 / 上一个，到头绕回；`{N}gt` 跳到第 N 个（超出不动），`{N}gT` 往回 N 个。作用于焦点 pane，树和空 pane 上无效。点击 tab 切过去并聚焦该 pane。切换时原来的当前 tab 成为「上一个」（`-`）；旧 tab 正在编辑 WHERE / PAGE 时先退出输入，输入框恢复成生效的条件。中键关闭、拖动排序不做。
  - **打开已有的表**（F3.37 用户定，取代 M1 起的「在整个 window 里找、多个时列出选择」）：同一个 pane 里一张表只开一个 tab，不同的 pane 可以各开一个，用来对比同一张表的不同条件。从树或命令面板打开一张表时，只在目标 pane（§12「表」）里找这张表的 tab（按 schema + 表名比较）：
    - 有：切过去，焦点移到这个 pane，不重新取数，状态保留；它就是当前 tab 时只移焦点；是从这个 pane 的引导 tab 发起的（引导页的 `t`、`+` 之后从树或面板选），引导 tab 一起关掉，不留空 tab；
    - 没有：在目标 pane 新开一个 tab，不替换当前的表 tab 或 console tab；只有引导 tab 会被替换，它只是占位。别的 pane 里开着这张表不影响。
    - `C-t`、树里的 `t`、`+` 的标记同样遵守这条规则，目标 pane 里已有就切过去。「选择 tab」列表不再使用。
  - **`+`**：命中区执行 Action `tab.new`（标题「新建 tab」，没有默认键，面板能搜到）：在 `+` 所在的 pane 新开一个引导 tab 并聚焦它（M3 F3.7 起，取代 M1 的「展开树、进过滤框、记下下一次打开进新 tab 的标记」，那个标记已经没有了）。
- **状态栏**：
  - 底色为 `bar`（#292e42）。各段是扁平色块，不用 powerline 箭头，每段左右各留 1 列内边距。
  - 左侧依次为：
    - ` doraemon ▾ `（session 名）：`focus` 底、`bg` 色字、粗体。M5 起 session 不再对应一个连接，不画引擎图标；点击打开 session 列表（F5.1）；
    - window 列表，如 ` 0: data* `：当前 window 用 #3b4261 底、`fg` 色字，其余 window 用 `dim` 色字、无底色。
  - 右侧右对齐，依次为：
    - 模式附加信息：`dim` 色，过长时用省略号。内容取自设计稿，随当前模式变化：
      - NORMAL：不显示；
      - INSERT：正在编辑的对象，如 `-- editing WHERE --`；
      - VISUAL：选区大小和执行键，如 `4 lines · ↵ run`，键位文字从 keymap 读取；
      - COMMAND：不显示，匹配的命令在命令面板里。
    - ` <搜索图标> `：`info` 色，点击打开命令面板；ascii 图标下显示为 ` ~ C-p `（§7.7）；
    - ` <键盘图标> 待输入序列 `：序列用 `warn` 色粗体。没有待输入的键时，显示 `dim` 色的 `·`；这一块始终占着位置，序列部分至少 3 列宽（放得下 `SPC`），内容靠左，这样按键时右侧各块不会左右跳动，序列超过 3 列时才变宽；
    - ` 行,列 `：`fg_muted` 色。只在焦点所在的 data pane 或结果区有已加载的表时显示（M3 起结果区也算）：行号是绝对行号（页偏移 + 1），列号从 1 起；树聚焦、空 pane、空表都不显示这一块（M0 的 `1,1` 占位就此去掉）；
    - ` <图标> play · sqlmux@localhost:5432 `：`info` 色，`sep` 底。内容是当前连接（§5）的 `<连接名> · <用户>@<host>:<port>`（M5 起加上连接名：两个连接常常只差库名，比如 play 和 seed），`@` 前面放数据库用户名；unix socket 时 host 就是目录，照样显示，如 `ctw@/tmp:5432`。设计稿里的 `pg@` 按此理解；
    - ` 事务中 `（`warn` 色）/ ` 事务出错 `（`error` 色）：当前连接的 `Main` 在事务里 / 处在出错的事务里时显示（M5 F5.3），自动模式下用户自己 `begin` 的也算；在 busy 提示（§8.3）的左边，和它一样不让位；
    - ` NORMAL `：模式色底、`bg` 色字、粗体。
  - 窗口太窄、放不下时，按下面的顺序依次省略：
    1. 模式附加信息；
    2. 连接地址；
    3. 不是当前的 window，从右往左去掉；
    4. 光标位置；
    5. 截短 session 名。

    始终保留的是：session 块（名称可以被截短）、当前 window、`C-p` 入口、待输入序列、模式块。
- **COMMAND 模式**：命令面板打开时，模式块显示 COMMAND；接管按键的浮层（schema / ORDER / LIMIT 下拉、COLS 列表、WHERE 历史 / 收藏下拉）打开时也是 COMMAND，不论焦点在列表还是过滤框，因为它们和面板一样把 normal 作用域的键（leader、`C-hjkl`）挡在浮层外，不另设模式名；keymap 里 COMMAND 和 INSERT 一样算「在打字」，没绑定的可打印字符照样进过滤框。which-key 不算：它不是作用域（§6.5），弹出时序列还没按完，剩下的键仍在 normal 里解析，所以照旧显示 NORMAL。补全列表也不算：它不接管输入，只是挂在输入框下面，显示 INSERT。状态栏里不再有命令行：M0 用户体验后改由命令面板取代（§12），`:` 打开面板（F5.8 起是「所有」范围）。
- **错误栏**（M2/M3 验收时定，参考 DataGrip 在编辑区下方的报错区）：数据库的报错显示在出错那个 pane 的内容区底部、tab 栏上方，不再挤在查询条右侧或者占掉表格。
  - 第一行是 `error` 色的 `[SQLSTATE] <Message>`，下面依次是 DETAIL、HINT、位置（PG 返回时才有），一项一行，DETAIL 自带的换行照拆，`error_bg` 底；每行放不下时右边截短加 `…`，不折行；最多 6 行，超出时第 6 行画成 `…`；右端是可以点击的 `×`。不是数据库返回的错误（如「WHERE 里不能有 ;」、保存时行数据已变化）没有 `[SQLSTATE]`，只有文字。
  - 位置：console 的报错换算成 console 文件里的「位置：第 3 行第 12 列」；表格取数的 SQL 是拼出来的，不显示位置。
  - 错误栏占内容区最下面的几行，表格 / console 的可见高度相应变矮，不盖住内容。一个 tab 同时只有一条，后来的替换先前的；错误栏属于 tab，切走不显示，切回来还在。
  - 关闭：点 `×`，或者在这个 pane 的 NORMAL 下按 `esc`（Action `pane.error.close`）。同一个 tab 下一次同类操作成功时自动消失：取数成功清掉取数的报错，保存成功清掉保存的报错，console 每次开始执行就清掉上一次的；编辑单元格不清。`esc` 绑在 `[keys.normal]`，console 的 VISUAL 下和编辑器有待定键时照旧交给 vim。
  - 用在：表格的取数与 WHERE 报错、保存失败（「id = 42：…，已回滚」这一类，§10.3）、console 的执行报错（日志 tab 照旧记，出错语句的 ▶ 照旧变红；结果区 `R` 重跑出错时显示在来源 console 的 tab 上，来源已关就只记日志）。保存被取消的「已取消，已回滚」不是错误，仍在查询条右侧。
- **toast**：显示在状态栏上方一行的右侧，默认 3 秒后消失；「再按一次 C-c 退出」这一条显示 2 秒，正好是连按的窗口（§6.8）。样式为 `warn` 色字、#292e42 底、左右各留 1 列。toast 只有一行：文字里的换行换成「；」、Tab 换成空格，在显示 toast 的那一处统一做（驱动的错误常是多行的，比如 localhost 解析出两个地址各报一行；F5.10 审查时发现）。设计稿里没有 toast，这个样式是后定的。加底色是因为那一行正好是 pane 的下边框，不加底色，文字会和边框混在一起。data pane 中保存 / 刷新的结果按 Q-06 的要求显示在查询条的右侧，不通过 toast 显示。
- **session 列表**（M5 F5.1，S-02、S-03；`SPC s` 或点击状态栏的 session 块）：框和行的样式、位置、宽度照命令面板，标题「session」。每行是 session 名和 `dim` 色的 window 数（如 `3 windows`），当前 session 用 `focus` 色；`l` 展开后下面一级是它的 window（`0: data`），当前 window 同样标出。`$` 把这一行原地变成输入框（INSERT，`↵` 确定、`esc` 放弃）；`x` 关闭 session：有未保存的修改或未提交的事务时先确认（§10.5），关的是最后一个 session 等于退出。单击一行等于移过去再按 `↵`。每个 session 下面都标出它自己的当前 window（同 tmux）。键位见 §6.8 的浮层表。列表和新建连接表单打开时按 `C-p`，面板盖在上面并接管按键（同 COLS 列表）；表单里的 `C-s` 是保存表单，不去保存背后表格的修改。切换 session（包括 `n` 建连回来后自动 attach）时关掉所有浮层，确认框也关。关掉有查询在执行的 session 不另确认，查询随连接关掉，也不弹「查询已取消」。
- **新建 session**（`n`）：名字照 tmux 取最小的没用过的数字（`1`、`2`……）；一个 window、一个空 pane（引导页），不建 console（M8 F8.18，用户：新建 window / session 时总是两个 pane；原来是默认布局带一个新 console_n）；连上当前连接并照「默认展开」展开它，其余连接不连。新 session 有自己的一套连接，按 `n` 后先在后台给它连上当前连接（toast「正在连接 <名字>…」），连上之后才建出 session 并 attach 过去，连不上就 toast `<名字>：<错误>`、不建，所以「当前连接一定是连上的」照样成立。
- **新建连接表单**（M5 F5.1，命令 `conn.new`「新建连接…」；M7 F7.4 按用户要求重做成分字段、居中、好看）：居中的框，宽 60 列（窗口不够时取窗口宽 − 4），单线 `focus` 色边框，标题「新建连接」。字段从上到下：name、engine（分段开关 `postgres │ mysql`，选中段 `select` 底，`space` / `←` `→` 切换，单击选中点到的那一段、点已选中的不变，同单选按钮）、host、port（engine 切换时没改过的 port 跟着换默认 5432 / 3306）、user、password（输入画 `*`，F7.3）、database、read_only（`[x]` 开关）、最下面一行「高级」折叠：dsn 和 password_cmd（展开后直接填 dsn 时上面的 host / port / user / database 变灰、不参与）。每行是「图标 + 字段名 + 输入框」（用户定）：图标用 Nerd Font，后面留 1 格（§7.7），字段名右对齐、`dim` 色，输入框占满右侧、`sep` 底；当前字段的输入框换 `select` 底、图标和字段名 `fg` 色。图标：name `tag`（nf-fa-tag，U+F02B）、engine 用现有的 `postgres` / `mysql`（跟着选中的段变）、host `host`（nf-fa-server，U+F233）、port `port`（nf-fa-plug，U+F1E6）、user `user`（nf-fa-user，U+F007）、password 用现有的 `key`、database `database`（nf-fa-database，U+F1C0）、read_only 用现有的 `lock`（树里只读连接也是它）、「高级」`advanced`（nf-fa-cog，U+F013）、dsn `link`（nf-fa-link，U+F0C1）、password_cmd 用现有的 `console`；校验不通过的字段名 `error` 色，原因写在底栏上方一行。底部一行两个按钮「保存」「取消」（`focus` 色边、可点击）加键位提示（`Tab 下一项 · ↵ 保存 · esc 取消`，从 keymap 读）。单行输入框是 `ui.Input`（同面板，不是 vim）。`↵` 保存：校验同启动时读 connections.toml（name 必填、名字的字符规则、不能重名；host / user / database 必填，port 是数字，或者填了 dsn），不通过就焦点移到出错的字段；通过就拼成 `dsn`（`postgres://user@host:port/db?sslmode=prefer` / `mysql://user@host:port/db`，密码不进 dsn、按 F7.3 存钥匙串）追加到 connections.toml（§14，文件的字段不变，仍是 name / engine / dsn / password_* / read_only），所有 session 的树里多出这个连接，状态是未连接。**从「没有连接」的启动状态保存第一个连接时**，保存后直接连上并展开，并像正常启动那样在 ② 建 console_1 替掉引导页，和 `sqlmux <连接名>` 启动的结果一致；零连接期间 ①② 都是引导页、不建 console 文件（F7.4）。「高级」里 dsn 非空时 host / port / user / database 变灰、Tab 跳过、不校验，password 不灰（dsn 里不带密码是常态，密码照存钥匙串）；host 不预填（必填），port 预填 5432 / 3306（用户改过之前跟着 engine 换），其余空；dsn 用 `net/url` 拼，user / database 转义，IPv6 用 `JoinHostPort`；不预填占位提示字。
- **连接管理**（M8 F8.16，用户要能改、能删，参考 DataGrip 的数据源面板）：命令 `conn.manage`「管理连接…」打开居中浮层，左边是 connections.toml 顺序的连接列表（图标 + 名字，只读的带 `lock`，选中项 `select` 底，最后一项 `＋ 新建`），右边就是上面这张表单，填着选中连接的值（`dsn` 写法的连接把 host 等从 dsn 拆出来只读显示，改的是 `dsn` 那一栏——不做改字段写回 dsn 的对应段，`ponytail:` 标上限；`${VAR}`、`password_cmd` 原样显示在高级里）；`j` / `k` / 单击切换，`Tab` 进表单，`↵` / `C-s` 保存，`esc` 关闭，列表上 `x` 或表单下的「删除」按钮删除（确认框）。按钮和键位提示放不下同一行时提示挪到按钮下面一行、右对齐（80 列下右半边只有 45 列），列表固定 26 列。`conn.new` 和侧栏的 `＋` 都打开它并选中 `＋ 新建`，不再另有一个单独的表单。文件改写规则见 §14「修改和删除连接」；改了已连上的连接不重连，toast「下次连接时生效」；有 session 在用的连接不能删（toast「先关掉用它的 tab」）；删除时钥匙串里的密码一并删。
- **新建连接的入口**（F7.4）：侧栏第一行（过滤图标和 `N tables` 那一行）右端一个 `＋` 按钮（图标 `row_add`，`info` 色，悬停 `select` 底，可在 `[icon]` 里覆盖），单击打开表单；命令面板的「新建连接…」照旧。没有任何连接时，树的正文只有一行可点击的「还没有连接，按 ＋ 新建」（`dim` 色），单击等于按 `＋`。

### 7.9 文本输入：删词与自动配对（M2/M3 验收时定）

- **删词**：所有单行输入框（面板、树的过滤框、COLS、PAGE、单元格编辑、编辑器的 `:` / `/` 命令行；WHERE 在 F3.39 起同 console 的 INSERT，`C-u` / `C-w` 照 nvim 先删到这次 INSERT 的起点，再按一次才删到行首）都支持 `C-w` 删掉光标前的一个词、`C-u` 删到行首；词的划分同 vim INSERT 下的 `C-w`（先跳过空白，再删一串关键字字符或一串其他非空白字符）。console 的 INSERT 本来就有。
- **`M-BS`（Option / Alt + 退格）当作 `C-w`**：在上面这些地方和 console 的 INSERT 里都一样，用户在 nvim 里就是这样映射的，也是 macOS 文本框的习惯。它是编辑文字的内建行为，不写进 `default.toml`，所以「默认键位不用 Alt」（§6.2）的检查不受影响；终端不发这个键时，`C-w` 照样能用。console 的 NORMAL / VISUAL 下 `M-BS` 不做事。
- **自动配对**：在 console 的 INSERT、WHERE 输入框、快速 SQL 里，输入 `(`、`[`、`{`、`'`、`"`、`` ` `` 时补上另一半，光标停在中间，如 `order = '█'`。
  - 只在光标后面是行尾、空白或右括号时才配对；引号还要求光标前面不是字母数字或同一个引号，免得 `don't`、`''` 这类被弄乱。
  - 光标后面正好是同一个右括号或引号时，输入它只是跳过去。接受补全也一样：插入的内容以引号结尾、光标后面正好是同一个引号，就一起替换掉，WHERE 里 `status = '█'` 接受 `'done'` 得到 `'done'█`。
  - 光标在一对空的配对中间时按退格，两边一起删。
  - 粘贴原样插入，不配对，也不跳过光标后面的同一个字符（同 VS Code、nvim-autopairs）。
  - 在 console 里这些都算这次 INSERT 的一部分，一起撤销。单元格编辑、树的过滤框、COLS、面板的其他范围不配对。
  - 配置 `autopairs = true | false`，默认开。编辑器的 nvim 差分测试里关掉，nvim 本体没有这个行为。
- **有选区时输入开括号或引号**（非 vim 模式才有「选着再输入」，F6.6）：把选区包起来，不替换，同 VS Code 的 autoSurround。

## 8. 数据访问

### 8.1 接口

```go
type Conn interface {
    Exec(ctx context.Context, sql string, maxRows int) ([]Result, error)  // 多语句，走简单协议
    Query(ctx context.Context, sql string, args ...Val) (Result, error)   // 单语句，参数按文本或 NULL 传
    Close() error
}

type Val struct{ S string; Null bool }

type Result struct {
    Cols      []Col             // Name, Type（数据库类型名）, Numeric
    Rows      [][]Val
    Tag       string            // 例如 "UPDATE 3"
    Affected  int64             // 影响行数；保存、新增、删除按它检查（PG 上和按 Tag 判断的结果一致）
    Truncated bool
    Took      time.Duration
}
```

- `postgres`、`mysql` 各实现一份 `Conn`，外加各自的 catalog 查询和方言函数（`QuoteIdent`、`Placeholder(n)`）。M1 只有 `postgres` 一个实现，接口仍然要有：`db` 里的 Worker 不能 import `db/postgres`，而 `db/postgres` 要用 `db` 的 `Val` / `Result`，接口是断开这个环的办法，M5 的 mysql 是第二个实现。
- **取消走 context，不单设 Cancel 方法**：取消当前请求的 ctx，由 pgconn 的 `CancelRequestContextWatcherHandler` 发 CancelRequest（§8.3）。直接调 `PgConn.CancelRequest` 有竞态：请求刚结束时发出的取消会打到下一条查询上，pgconn 源码 `HandleCancel` 的注释写明了这一点。
- `Col.Type` 在 PG 下用 pgtype 内置的 OID 表转成类型名（`int8`、`text`、`jsonb` …），认不出的 OID（枚举等自定义类型）留空；按列着色（F1.3）改用 catalog 的 `format_type`，快速 SQL 里的自定义类型按字符串处理。
- `Session` 只依赖这些接口。以后接入非 SQL 引擎（如 Redis）时，由引擎决定 window 里能创建哪些 pane。

**值一律按文本处理**：客户端只负责显示和回写，按文本处理可以绕开各类类型的解码问题（枚举、数组、range、geometry 等）。

- **PG**：
  - `pgconn.Exec`（简单协议）本身就返回文本；`ExecParams` 把结果格式指定为文本，参数 OID 传 0，由服务端推断类型。
  - 列类型取自 FieldDescription 里的 OID。
  - 建连时通过 RuntimeParams（startup 消息）设置 `DateStyle = ISO, YMD`，保证时间文本的格式可以解析（§10.2）。每条连接都会带上，不会漏设，也不需要额外的往返。
  - 连接参数交给 `pgconn.ParseConfig(dsn)` 解析，环境变量（`PGHOST` 等）和 `~/.pgpass` 都由它处理，不自己实现。没写 `application_name` 时补成 `sqlmux`，没写连接超时时补成 10s。
- **MySQL**：DSN 加上 `interpolateParams=true`，全程走文本协议；列类型从 `ColumnTypes()` 获取。go-sql-driver 钉在 v1.7.1（M5 F5.4 审查时定）：v1.8 起文本协议下也把整数、浮点数解析成 Go 数值，database/sql 再转回字符串，和服务端原文不一样（double 1500000 变成 `1.5e+06`，float(10,2) 的 1.50 变成 `1.5`，zerofill 的 007 变成 `7`），显示不对、保存永远报「行数据已变化」，v1.10 也没有关掉的开关。驱动加了开关或者改用原始字节读时再升级，go.mod 旁用 `ponytail:` 注释写明。
- **已知上限**：MySQL 中非 UTF-8 的 blob 显示为 `<binary n bytes>`；PG 的 bytea 按服务端返回的 `\x…` 文本显示。MySQL 的 BIT 列文本协议返回原始字节（bit(1) 的 0 / 1 是 `\x00` / `\x01`），按大端字节解码成数字显示，不能编辑（F5.4）。

### 8.2 连接模型

每个 session 的每个连接（`Conn`，§5「一个 session 多个连接」）用到时建两条：`Meta` / `Main`，这个连接上的 console 共用 `Main`（M5 起；原来是每个 session 两条，分工不变）。每条连接由一个 `db.Worker` 独占，用互斥锁保证同一时刻只跑一个请求，因为连接对象不能并发使用。`tea.Cmd` 本来就各在自己的 goroutine 里执行，所以 Worker 不再单开 goroutine 和 channel；请求的先后由 tab 的 `seq` 兜底（§8.3）。`Worker.Cancel()` 取消当前请求的 ctx。

**启动**：`main` 里同步给启动时的连接（`sqlmux <名字>` 指定的，不带参数时是第一个）建连，先 `Main` 后 `Meta`，两条都连上才进入界面，期间不显示提示（每条最多 10s）。任何一步失败都按 §14「启动时找不到连接」处理。其他连接在用到时（树里展开、console 选它、新 session）在 Cmd 里连，失败只标在树上、进 toast（§7.8）。

| 连接 | 用途 |
|---|---|
| `Main` | console 执行、保存修改、manual 事务模式或在事务中时的表格读取；持有事务状态 |
| `Meta` | schema 树、列属性、DDL 预览、计数、快速 SQL；auto 事务模式下的表格读取 |

- 拆成两条的原因：console 里跑长查询时，树、命令面板和表格浏览不会跟着卡住。
- manual 事务模式下，或者 `Main` 正在事务中时（用户在 console 里自己 begin 了），表格读取改走 `Main`，这样能看到自己还没提交的修改（F5.3）。代价：
  - 表格取数要排在这个连接 console 的执行后面，「表格浏览不会跟着卡住」在这时不成立；
  - 在事务中的取数包在 `SAVEPOINT sqlmux_page` 里，WHERE 写错或取消只撤到 savepoint，不让整个事务进入出错状态（F5.3 worker 加、审查认可）；不在事务里时不发 SAVEPOINT；
  - WHERE 跑在可写的 `Main` 上，调用了写数据的函数（如 `nextval`）就真的写了；只读本来就不是权限边界（§13）。计数仍在 `Meta` 上，看不到没提交的增删（ponytail）。
- **MySQL 的事务状态**（F5.4 审查时定，MySQL 8.4 上实测）：MySQL 拿不到 TxStatus，按执行过的语句记。begin / start transaction 记为在事务中；commit、rollback（不含 rollback to），以及手册「Statements That Cause an Implicit Commit」列出的语句（TEMPORARY 以外的 DDL、账号管理、LOCK / UNLOCK TABLES、ANALYZE / OPTIMIZE / REPAIR / FLUSH、RESET、再一次 BEGIN / START 等）执行后记为不在事务中；死锁（1213）时 MySQL 回滚整个事务，也记为不在事务中，并 toast「死锁，整个事务已回滚」。否则状态记错之后，手动模式不再补 BEGIN、取数和保存的 savepoint 报不存在，已经提交的写入还被报成「已回滚」。其余记不准的情况（如在存储过程里提交）是已知上限，代码里用 `ponytail:` 标出。不另用 `SET autocommit = 0`：隐式提交之后状态照样要自己记。
- **断线重连**（M8 F8.19，用户 2026-10-08：经 `tsh` 代理连公司的库，代理经常断开又自动恢复，sqlmux 要跟着重连；原来断开的连接不自动重连，MySQL `wait_timeout` 8 小时空闲断开后也要重启 sqlmux）：
  - **判定**：`Meta` / `Main` 上的一次请求以连接级错误失败——PG 是 `pgconn` 报的 `conn closed`、`broken pipe`、`EOF`、`connection reset`，或 `PgConn().IsClosed()`；MySQL 是 `driver.ErrBadConn`、`invalid connection`、同样的网络错误——就把这条连接标成断开。SQL 错误（语法、权限、约束）不算。
  - **重连**：Worker 在同一个 goroutine 里立刻重拨，用**当前**配置的 dsn 和密码来源（F8.19 审查时定：连接管理里改了名或 dsn 之后，旧 Spec 的 password_cmd 指着已删的钥匙串项、旧地址，会永远拨不通），engine 和 read_only 沿用建连时的（界面上的只读状态记在 Conn 上，§7.8 连接管理）；`password_cmd` / `${VAR}` 再取一次密码，超时同建连，成功后重放会话状态：建连参数本来就带 DateStyle 和只读（RuntimeParams，§8.2），console 选过的 schema / search_path 要再 `set` 一次（§8.6「Schema」记在 console 上，不是记在连接上，重放的是这条连接上最后一次设置的值）；正在进行的事务（手动模式的，或用户自己 `begin` 的）已经随连接丢了：事务状态回到空闲，**模式不变**（手动还是手动——改回自动的话用户的下一条 update 会被悄悄自动提交，更危险；F8.19 时定），toast「<连接名>：连接断开，事务已丢失」。**事务丢失时可能写的请求一律不跑**（F8.19 审查时定）：失败的那条 console 写语句、表格保存不重跑，返回「连接断开，事务已丢失，这条没有执行」；`commit` 没发出去时报「事务已丢失，没有提交」，**已发出、回复前断开的报「连接断开，提交结果未知：请先查一下数据」**（服务端可能已经提交，这时说「已丢失」会让用户整个重做一遍、变成双写）；`rollback` 两种结果都等于回滚，不区分；console 里手敲的 `commit`（含 `commit work` 等写法）和 `rollback` 同样按这两条处理，不走「这条没有执行」的固定文字（F8.19 复审时定）；新连接上 commit 只会得到一个 WARNING，界面上看起来像提交成功了，其实是半个事务，所以不跑。只读请求照常。事务丢失时同时重载目录和表格（同死锁的处理，手动模式下表格还显示着丢掉的未提交数据）。整个事务由用户重做。
  - **判定分三档**（F8.19 时定）：不是连接错误；连接丢了但请求没发出去（PG `pgconn.SafeToRetry`、MySQL `driver.ErrBadConn`）——这档写语句也能重跑；实测只有代理 RST 断开才落在这档，空闲连接被 `pg_terminate_backend`、`KILL`、`wait_timeout` / `idle_session_timeout`、代理 FIN 断开后，下一次请求拿到的都是「已发出」，所以空闲断开后的第一条 update 会提示「请重新执行」，不自动重跑（要做到得每次请求前多一次探测往返，不值）；连接丢了、不知道服务端执行了没有（网络错误、`conn closed`、PG 的 57P01 / 57P02 / 57P03 和 08 类 PgError）——这档只有只读才重跑。请求前先问一下连接是否已关（PG `IsClosed`），已关就先重拨，取消 5 秒后断开的连接不让用户先吃一个错误。
  - **重试**：重连成功后，失败的那次请求只在**安全**时自动重跑一次：`Meta` 上的全部请求（catalog、取数、计数、补全的查询）和快速 SQL（只读事务，§9.6）都安全；console 的语句（读写都一样）只在「没发出去」时重跑：IsRead 看不出 `select nextval(…)`、`select setval(…)`、`select get_lock(…)`、用户自己写的会 insert 的函数这些副作用，Main 又不是只读连接，「已发出」的 select 再跑一遍就是双写（审查时在真库上实测）；表格保存按三档：没发出去的重跑（自动提交模式下的保存是一个完整事务，整体重跑是安全的），发出去了不知道执行没有的不重跑，错误栏显示原来的错误并 toast「<连接名>：已重连，请重新执行」，保存照 §10.3 报错、用户再按一次 `C-s`；事务丢失时按上一条一律不跑。一次执行里多条语句时，「没发出去」只对第一条成立：前面的语句已经自动提交，整个请求不能重跑。重跑再失败就按普通错误报，不再重连（每次请求最多重连一次）。
  - **重连失败**：树上的连接节点转成失败状态（`error` 色，§7.8），toast `<连接名>：<错误>`，tab 都留着；下一次请求再试一次拨号，所以 tsh 恢复之后用户随便操作一下就回来了。不做后台定时探活、不做指数退避：没有请求时断不断开没有影响。
  - 原来 §8.3 里「取消 5 秒后断开的连接不自动重连」和 MySQL 的空闲断开都由这条覆盖。
- `Meta` 建连后设为只读：PG 设 `default_transaction_read_only = on`（和 DateStyle 一样放在 RuntimeParams 里，效果等同 `SET SESSION CHARACTERISTICS AS TRANSACTION READ ONLY`），MySQL 执行 `SET SESSION TRANSACTION READ ONLY`。`Meta` 上只有读操作，写入只走 `Main`，所以 WHERE 里就算调用了会写数据的函数，也改不了数据。和 §13 一样，这是为了防误操作，不是权限边界。只读连接（`read_only = true`，§13）的 `Main` 也放同样的 RuntimeParams（M5 F5.3），`RESET` / `DISCARD ALL` 之后仍是只读。

### 8.3 取消、超时、过期响应

- **取消**：当前连接（§5，M5 起；原来是本 session）忙碌时（`Main` 上的 console 执行，或 `Meta` 上的表格取数），状态栏在模式块左边显示 ` busy · C-c 取消 `（`warn` 色，键位文字从 keymap 读），点击执行 Action `cancel`。`C-c` 在当前连接有请求执行中时取消它，空闲时照旧是连按两次退出（§6.8）。取消后 toast 显示「查询已取消」，tab 保留之前的数据，第一次打开就是空表。取消按连接算，不按 tab：手动模式下表格取数排在 console 的长查询后面时，在表格上按 `C-c` 取消的是 console 的语句，事务因此进入出错状态，只能回滚（同 psql 在事务里按 Ctrl-C；表格自己的取数在 savepoint 里，取消它不影响事务，§8.2）。按 tab 单独取消要给 Worker 的每个请求一个 ctx，暂不做。
  - PG：取消请求的 ctx。pgconn 默认的 `DeadlineContextWatcherHandler` 会在 context 取消时给连接设 deadline，连接随之断开。每个连接只有两条长连接，所以改用 `CancelRequestContextWatcherHandler`，由它发 CancelRequest，取消之后连接还能继续用。`DeadlineDelay` 取 5s：取消发出 5 秒后服务端还没停，就断开连接；断开后由「断线重连」（§8.2，M8 F8.19）在下一次请求时重连。
  - MySQL（M5）：`Query` 自己监听 ctx，取消时另开一条临时连接执行 `KILL QUERY <connection_id>`，id 在建连时记录。只取消 context 的话，驱动会关掉连接，但服务端上的查询会继续跑。已知上限（F5.4）：临时连接连不上时（库被删、连接数满、网络断）没有兜底，Worker 一直锁到那条语句自己结束；PG 那边 5 秒后断开连接，以后和它对齐，代码里用 `ponytail:` 标出。
- **超时**：建连 10s。计数查询 3s，用 ctx 超时实现：取消由 `CancelRequestContextWatcherHandler` 发出，返回 `context.DeadlineExceeded` 后显示 `?`（用户按 `C-c` 取消返回的是 `context.Canceled`）。不用 `SET LOCAL statement_timeout`：那要 begin / set / count / rollback 四次往返，且必须在 Worker 同一把锁里执行，否则别的请求会插进事务，等于要给 Worker 加 Tx；ctx 的做法引擎无关，MySQL 也能用。计数用 `tea.Sequence` 排在取数之后，3s 从页面数据回来后才开始算；计数不算 busy，不显示忙碌提示。
- **过期响应**：每个 tab 维护一个递增的 `seq`，请求时带上。结果回来时 `seq` 已经不是最新的就丢弃，避免快速翻页时旧结果覆盖新结果。

### 8.4 元数据

| 内容 | PostgreSQL | MySQL |
|---|---|---|
| 表列表、行数量级（D-01） | `pg_class.reltuples` + `pg_namespace` | `information_schema.TABLES.TABLE_ROWS` |
| 列属性：类型、可空、默认值、主键 | `pg_attribute`（`format_type`、`attnotnull`、`pg_get_expr(adbin, adrelid)`）+ `pg_constraint` | `information_schema.COLUMNS` + `KEY_COLUMN_USAGE` |
| 所有列都不可空的唯一索引（用于定位行，见 §10.1） | `pg_index`，条件见表下 | `information_schema.STATISTICS` 中 `NON_UNIQUE = 0`，并要求每列的 `IS_NULLABLE = 'NO'` |
| 枚举值（§10.2） | `pg_type.typtype = 'e'` 时查 `pg_enum` | 解析 `COLUMN_TYPE`，如 `enum('a','b')` |
| DDL 预览（K-05） | 用 `format_type`、`pg_get_constraintdef`、`pg_get_indexdef` 拼出，不要求与 pg_dump 一致 | `SHOW CREATE TABLE` |

PG 的几处细节（M1 核对时对照 lazysql、pgtui 确认，两者各有问题）：

- **表列表**：`relkind IN ('r','p','v','m','f')` 且 `NOT relispartition`，分区的子表不列出，只列父表。系统 schema 用 `nspname !~ '^pg_'` 排除；`NOT LIKE 'pg_%'` 里的 `_` 是通配符，会误伤 `pgx_data` 这类 schema。
- **行数量级**：`reltuples < 0`（从没 analyze 过，或者是分区表的父表）时显示为 `?`。
- **列属性**：不用 `information_schema.columns`，它会漏掉没有权限的列，数组和枚举的类型也只显示成 `ARRAY`、`USER-DEFINED`。
- **主键**：列按 `unnest(conkey) WITH ORDINALITY` 的顺序排列。
- **可用于定位行的唯一索引**：`indisunique AND indisvalid AND indpred IS NULL AND 0 <> ALL(indkey)`；只看前 `indnkeyatts` 个键列（INCLUDE 列不算），并要求这些列都是 `attnotnull`。
- **枚举值**：`array_agg(enumlabel ORDER BY enumsortorder)`，按列的 `atttypid` 关联。

catalog 按连接缓存（每个 session 的每个 `Conn` 一份，M5 起；原来按 session）。console 执行 DDL 后（由 §9.3 的判定得知）或用户执行刷新命令时，重新加载。

**加载时机**：Init 时在 `Meta` 上异步查两样，各一条查询：非系统 schema 的列表，以及这些 schema 里所有表的名字和行数估计。列属性、主键、唯一索引、枚举按表查，第一次打开这张表时才查，查过就缓存，刷新时清掉。这样大库启动时不用把整张 `pg_attribute` 读回来。

### 8.5 分页与计数

**取数**：`SELECT * FROM t WHERE … ORDER BY … LIMIT n OFFSET m`。

- 每页默认 100 行，LIMIT chip 可选 100 / 500 / 1000。100 与 PRD 快速 SQL 的 limit 100 一致。
- 每次多取一行（`LIMIT n+1`），用来判断还有没有下一页。这样计数还没回来或者超时的时候，翻页也能正确停在最后一页。

- 默认按「行标识列」排序（§10.1：主键，没有主键时用所有列都不可空的唯一索引），保证分页结果稳定。
  - 两者都没有时，不加默认排序，用户可以通过 ORDER 指定排序列。
- 总是取全部列，列的显示与隐藏只在客户端控制，所以切换 COLS 不需要重新查询。已知上限：列很多或含大 JSON 字段的表会多传数据，以后可以改成只查可见列。

**计数**：在 `Meta` 上异步执行 `count(*)`。

- 超时则显示 `1/?`。
- 没有 WHERE 条件、且估计行数超过 100 万时，不做精确计数，用估计值显示 `~n`。

**已知上限**：OFFSET 翻到很深的页会变慢，以后可以改成 keyset 分页。

### 8.6 PG 的 schema 与 search_path

**背景**：

- PG 的一个数据库里可以有多个 schema，例如 `doraemon.public`、`doraemon.agentable`。
- SQL 里不带 schema 前缀的表名，会按连接的 `search_path` 设置依次查找，默认是 `"$user", public`。
- 所以，如果树里正在看的是 `agentable`，而 console 里写的是 `select * from tbl_xxx`，PG 默认只会在 `public` 里找这张表，结果报「表不存在」；更糟的情况是 `public` 里恰好有同名表，查到的就是另一张表。

**方案**：与 DataGrip 一致，每个 console 可以单独选择 schema。

- **入口**：console pane 标题的右侧显示 `<连接> · <库名>.<schema> ▾`（M5 F5.11 起加上连接；库名取连接的 database，记在 `Conn` 上），位于 `▶ run ↵` 旁边；连接那一段见 §7.8「console 标题」。
  - 点击它，或在 console 的 NORMAL 模式下按 `gs`，打开 schema 下拉框。
  - 下拉框列出当前库的 schema，系统 schema（`pg_catalog`、`information_schema` 等）不列出。顶部有过滤输入框，支持模糊匹配（§9.7）。
  - 下拉框照命令面板的做法：打开后输入直接进过滤框（模式为 COMMAND，§7.8），`C-n` / `C-p` / `↑` / `↓` 移动，`↵` 选中，`esc` 关闭；`j` / `k` 会被当成输入，所以不用。鼠标：点选，点浮层外部关闭，悬停 `row` 底。
  - 位置在入口（console 标题的 schema 按钮、查询条的 chip）下方、与入口左对齐，宽度取 max(最长 schema 名 + 边距, 入口所在栏的右边界 − 入口起点 x)，这样右边框和所在栏的右边框对齐；仍放不下时向左移。当前所在的 schema 用 `pk` 色标出（§7.3 里 pk 也用于 schema 值）。键位在 `[keys.dropdown]`，Action 为 `dropdown.up` / `dropdown.down` / `dropdown.select` / `dropdown.close`，ORDER、LIMIT 下拉框共用（F1.2 时叫 `schema.*`，F1.4 改名），只在浮层里用，不带标题。
  - 组件放在 `ui` 里，M1 用于 ORDER、LIMIT，M3 给 console 选 schema。树不再用它：M1 F1.12 起 schema 是树里的节点（§7.8）。
  - 命令面板里也有对应的「Switch schema…」命令。
- **默认值**：新建的 console（`c`、引导页按钮、面板命令、`C-e`）一律用当前连接和这个连接的 Schema（§5、§7.8），不管这个 console_n 文件在 state 里记着什么，并把它写回 state；之后两者互不影响。state 里记下的连接和 schema 只在启动时恢复默认布局里的 console_1 时用（M6 F6.5 恢复布局时也用，§11）。
- **执行方式**：同一个连接上的 console 共用 `Main`，所以每次执行前比较一下（`Conn` 记下 `Main` 当前的 search_path，初始为建连时读到的值；一次执行里有 SET / RESET / DISCARD 语句时把记下的值作废，下次一定重新 SET。代价是用户在 console 里自己 `set search_path` 只持续到这次执行结束，以下拉框为准）：如果连接当前的 `search_path` 与这个 console 选择的 schema 不一致，先执行 `SET search_path TO <所选 schema>, <建连时的原始 search_path>`。
  - 原始路径在建连时用 `SHOW search_path` 读取，接在后面，这样装在 `public` 等 schema 里的扩展函数仍然能找到。设置时用 `set_config('search_path', $1, false)` 带参数的写法，不把原始值拼进 SQL：服务器上设了 `search_path = ''`，或者 DSN 里写了 `$user,public` 时，拼接会报语法错（快速 SQL 已经这样写）。
  - **只在事务外记住 search_path**（M3 审查时在真 PG 上复现）：
    - `Main` 处在出错的事务里时（pgconn 的 TxStatus 为 `E`），跳过 SET 直接执行，并清掉记下的值。否则 SET 本身报 25P02，按「SET 失败时后面的语句不执行」，用户的 `rollback` 永远执行不到，session 卡死到重启。
    - 做 SET 的那一刻或者执行结束时，只要有一个时刻连接处在事务里（TxStatus 不是 `I`），就不记这次的 SET，下次一定重新 SET。否则事务里做的 SET 被用户的 `rollback` 撤掉之后，记下的值还当它在，下一次悄悄按别的 schema 查表。只看结束时的状态不够：SET 做在事务里、这次执行又以 `rollback` 结束时，结束时已经在事务外，SET 却已被撤掉（M3 复审时在真 PG 上复现）。
  - **schema 被删掉之后**：PG 接受不存在的 schema，SET 照样成功，只是找表时跳过它。catalog 重新加载后，选中的 schema 已经不存在的 console 退回这个连接的 Schema，不弹提示，同树退回 `current_schema()` 的做法。
  - 已知上限：用 `select set_config('search_path', …)` 改 search_path 不会让记下的值作废（首词是 select），代码里用 `ponytail:` 标出。
- **影响范围**：console 的补全以它自己选择的 schema 为准。表格查询始终带 schema 前缀，不受影响。

MySQL 的 schema 就是 database（M5 F5.4 定，同 DataGrip）：树里 MySQL 连接下面一层列 database，相当于 PG 的 schema 层，不列 `information_schema`、`mysql`、`performance_schema`、`sys`；console 标题是 `<连接> · <database> ▾`，执行前和连接当前的 database 不同就先 `USE`。`USE` 是会话级的、不受事务影响，用不着上面「只在事务外记住」那一套；一次执行里有 `USE` 语句时把记下的值作废，下次重新 `USE`。PRD 原来写切换 database 会新建 session，M5 起不再这样（§18）。

## 9. SQL 处理（`internal/sqlkit`）

### 9.1 词法扫描器

扫描一遍即完成，不构建 AST，按方言处理：

- **PG**：`'…'`（`''` 转义）、`E'…'`、`$tag$…$tag$`、`"标识符"`、`--` 注释、可嵌套的 `/* */`。0x80 以上的字节算标识符字符，同 scan.l 的 `ident_start` / `ident_cont`（M5 F5.3 审查时定：否则 `✓$a$` 这类写法分句和 PG 不一致，只读判定会被绕过）。
- **MySQL**：`'…'` 和 `"…"`（反斜杠转义）、`` `标识符` ``、`#` 注释、`-- `（后面必须带空格）、不可嵌套的 `/* */`。
- **输出**：token 的类型（关键字、标识符、字符串、数字、运算符、标点、注释、空白）、字节偏移、行号。

风险控制：

- 只做词法分析，代码量小，已知的边界情况都用表驱动测试覆盖。
- 执行前，console 会高亮即将执行的语句范围（§9.2），用户能看到实际要跑的是哪一段。
- 读写判定用于提示和应用层拦截，兜底的保护在数据库层（§13）。

### 9.2 分句、可执行标记与执行前高亮（C-01、C-02）

- **分句**：在括号深度为 0 的 `;` 处切分，得到每条语句的范围。已知上限（F5.4）：MySQL 存储过程、触发器、事件的复合语句体会在体内的 `;` 处被切开，`DELIMITER`（mysql 命令行和 DataGrip 都支持的客户端命令）也不认，原样发给服务端报错，所以这些在 console 里建不出来；`DELIMITER` 留作以后的 feature。
- **光标所在语句**（按行算，M3 审查时定）：
  - 光标所在的行上有某条语句的起点时，取这一条，不管光标是不是在行首的缩进里；
  - 紧贴在语句上方、中间没有空行的注释行，算作下面那条语句的一部分，因为这种注释通常是在说明下一条；
  - 否则取范围包含光标的那条；光标停在两条语句之间的空行上时，取前一条。
- **显示**：
  - 每条语句的起始行，在 gutter 显示 ▶ 标记，可以点击执行（C-02）。
  - console 处于 NORMAL 时，光标所在语句的范围用 `row` 底色标出，紧贴在上方、算作这条语句的注释行也一起标出（执行的文字仍从第一行代码开始）；处于 VISUAL 时显示选区。这就是按 ↵ 会执行的内容。

### 9.3 读写判定

1. 第一个有效关键字是 SELECT、SHOW、EXPLAIN、TABLE、VALUES、DESC、DESCRIBE 之一时，判为读。
2. 以 WITH 开头的，继续往后扫描，见第 6 条。
3. `EXPLAIN ANALYZE` 后面跟写语句时判为写，因为它会真正执行；`EXPLAIN (ANALYZE, BUFFERS) …` 这种括号写法里的 ANALYZE 也算。
4. PG 的 `SELECT … INTO 新表` 会建表，判为写。
5. 带行锁子句的 SELECT（`FOR UPDATE` / `FOR SHARE` / `FOR NO KEY UPDATE` / `FOR KEY SHARE`，以及 MySQL 旧写法的 `LOCK IN SHARE MODE`）判为写，不论在不在 WITH 里：它会加锁，PG 的只读事务也拒绝它。
6. WITH 之后在任何括号深度出现 INSERT、UPDATE、DELETE、MERGE 都判为写：CTE 里的写语句在括号里，只看顶层会漏掉。EXPLAIN 括号写法里的 `ANALYZE false` / `off` / `0` 不算 ANALYZE。
7. 其余一律判为写。

这个判定用于提示和应用层的拦截（快速 SQL 的写语句，§12；只读连接，§13）；判为读却会写数据的（比如调用了写数据的函数），靠数据库层的只读兜住（§13）。

### 9.4 自动 LIMIT（F-05）

- 对 SELECT、WITH…SELECT、TABLE、VALUES 这类读语句：顶层没有 LIMIT 或 FETCH 时，先去掉末尾的 `;`，再追加 `\nLIMIT n`（console 为 `max_rows + 1`，快速 SQL 为 101）。前面加换行，防止语句末尾的 `--` 注释把 LIMIT 注释掉。多取一行是为了判断是否还有更多，界面上显示 `100+`。
- 不用子查询包一层来加 LIMIT，因为 MySQL 可能丢掉派生表里的 ORDER BY。
- 其他语句由 `Exec` 的 `maxRows` 截断。

### 9.5 格式化（C-03 gq）

**做法**：用 goja（纯 Go 实现的 JS 引擎）在进程内运行 [sql-formatter](https://github.com/sql-formatter-org/sql-formatter) 的独立打包文件。

- sql-formatter 使用 MIT 许可，支持 postgresql、mysql 等方言，在 JS 生态中广泛使用。
- 打包文件 `sql-formatter.min.js` 固定版本，放在 `internal/sqlkit/` 下，用 `go:embed` 嵌入。

**可行性验证**（2026-09-23 spike，版本 sql-formatter 15.8.2，打包文件 312KB，darwin/arm64）：

| 项 | 结果 |
|---|---|
| 初始化（解析打包文件） | 113ms |
| 单次格式化 | 首次 55ms，之后 13–21ms |
| 内存 | 进程 RSS 峰值约 30MB |
| 二进制体积 | spike 增加约 8MB（空程序 1.6MB → 9.6MB）。M3 实测（F3.9，审查时）：不 strip 13.8MB → 27.4MB，strip 后 9.7MB → 19.4MB。增量比 spike 大，是因为 goja 用反射调方法，链接器因此给整个二进制保留所有导出方法（pgx 的代码段从 718 涨到 5040 个符号），空程序看不出这一点 |
| 正确性 | PG 的 `$$…$$`、`E'…'`、`::` 转换、`filter (where …)`、`$1` 参数、注释，MySQL 的反引号、`#` 注释、`limit 5, 10` 均输出正确 |

**兼容处理**：goja 的正则引擎不认识长 Unicode 属性名。加载前把 `\p{Alphabetic}`、`\p{Mark}`、`\p{Decimal_Number}` 替换为 `\p{L}`、`\p{M}`、`\p{Nd}`。sql-formatter 15.9.0 里共 5 处：`\p{Alphabetic}` 3 处（15.9.0 的数字正则里多了一个前瞻 `(?![\w\p{Alphabetic}])`，用了两次），`\p{Mark}`、`\p{Decimal_Number}` 各 1 处；测试按名字逐个断言次数，升级后次数一变就失败；对 SQL 标识符而言，二者的差别可以忽略。

**运行方式**：

- **`gq` 是操作符**（M3 审查时定）：`gq{移动}` 格式化移动碰到的那几条语句（从第一条到最后一条），`gqq` 和 `gqgq` 是当前语句，VISUAL 下按选区。否则 vim 用户习惯的 `gqap`、`gqip` 会把 `a`、`i` 当成进入 INSERT。编辑器识别这个操作符，产生「格式化这个范围」的效果，由 app 执行。
- 第一次格式化时才初始化 VM，之后一直复用。goja 不是线程安全的，所以用一把互斥锁保护。
- 语言按 console 所在连接的引擎选择；`tab_width` 原样传给 sql-formatter；`keyword_case` 同时设 `keywordCase` 和 `dataTypeCase`（PG 的类型名本身就是关键字，15.x 把它分成了单独的选项，默认 preserve），`functionCase` 保持 preserve。
- **5s 超时**：goja 跑 sql-formatter 时耗时增长比输入快得多，100 行的 insert（2KB）要 3s，200 行要 6.9s，17KB 要 122s，同样的包在 node 里只要 28–146ms，时间都花在 goja 的 Unicode 正则上（M3 审查时实测）。所以和 `formatprg` 一样 5s 超时，用 `vm.Interrupt` 打断后 `ClearInterrupt`，同一个 VM 还能继续用；toast「格式化超时（5s）」，缓冲区不变。已知上限：一两百行以上的语句用内置格式化会超时，这时配 `formatprg`（如 `pg_format -`）。
- 格式化失败时给出 toast 提示，缓冲区保持不变。输出为空也按失败处理（toast「格式化失败：没有输出」），不能把用户的 SQL 删成只剩 `;`。
- 配置了 `formatprg`（例如 `"pg_format -"`）时，改为调用外部命令：从 stdin 输入，读取 stdout 输出，与 vim 的 formatprg 一致。

### 9.6 WHERE 条件（Q-01）

不做简写语法。输入框里直接写完整的 SQL 条件表达式，例如 `deleted_at is null and status <> 'cancelled'`。

- **拼接方式**：输入原样放进 `SELECT * FROM t WHERE (\n<输入>\n) ORDER BY … LIMIT … OFFSET …`；计数查询用同一个表达式。
  - 外面加一层括号，是为了防止表达式里的 `OR` 和后面拼上的子句发生优先级错乱。
  - 括号内侧各加一个换行，这样输入末尾的 `--` 注释不会把右括号和后面的 ORDER / LIMIT 一起注释掉（lazysql 有这个问题）。输入为空时不加 WHERE。
- **只允许一条语句**：
  - PG 的表格查询走扩展协议，协议本身就拒绝多条语句；MySQL 不开启 `multiStatements`。
  - 发送之前，扫描器如果在顶层发现 `;`，直接提示错误，不发送：在 pane 底部的错误栏显示「WHERE 里不能有 ;」（F3.20 起，此前在内容区第一行）（M3 F3.5 起；在此之前由 PG 的扩展协议报错）。这样可以防止类似 `1=1; drop table t` 的输入连带执行其他语句。
- **历史 / 收藏**：保存输入原文。
- **补全**：见 §9.7。

### 9.7 模糊匹配与补全（console、WHERE）

**匹配算法直接用 fzf 的**：`github.com/junegunn/fzf/src/algo`（MIT 许可），用的就是 fzf 的 `FuzzyMatchV2`，得分和高亮位置都与 fzf 一致。

- **统一使用**：项目里所有的模糊匹配（命令面板、COLS 过滤、树过滤、WHERE 历史、补全）都用它，不再引入 sahilm/fuzzy。
- **已验证**（spike）：
  - 单独引入这个包，只会带进 fzf 的 `util` 以及 uniseg、go-isatty、go-shellwords、x/sys，测试程序只有 1.7MB。
  - 输入 `tord` 时，`t_order` 排在第一位。
- **封装**：这个包并不是 fzf 承诺稳定的库 API。所以固定版本，并封装在 `internal/ui/match.go` 一个文件里，以后升级只需要改这一处。
- **大小写**：smartcase，与 fzf 相同。
- **扩展语法**：列表类的过滤支持 fzf 扩展语法的常用部分：空格分隔多个词（AND）、`'精确`、`^前缀`、`后缀$`、`!排除`。

**补全的上下文判断**：根据光标前的 token 决定给哪类候选，只做词法判断，不做语法分析（`sqlkit.CompletionContext`）。

| 光标所在位置 | 候选 |
|---|---|
| FROM / JOIN / UPDATE / INTO / TABLE 之后 | 表名（含视图）；输入 `schema.` 后，只列该 schema 下的表 |
| `INSERT INTO 表 (` 之后，以及这个括号里的逗号之后 | 这张表还没写进括号的列，按表里的列顺序；第一项是「全部列」（M5 F5.13，见下面「INSERT 的列」） |
| `别名.` 或 `表名.` 之后 | 这张表的列。别名从当前语句的 FROM / JOIN 子句中解析，例如 `from t_order o` 中 `o` 指 `t_order` |
| console 的其他位置 | 依次为：当前语句引用到的表的列、其余表名、SQL 关键字。关键字包括查询、DML、DDL 的开头和子句（`insert into`、`update`、`set`、`delete from`、`values`、`returning`、`create table` 等；M5 F5.12 补齐，原来只有查询用的，用户验收时发现 insert / update / delete 补不出来），清单参照 lazysql 的 `builtinKeywords`，MySQL 另有 `replace into`、`on duplicate key update` |
| WHERE 输入框 | 当前表的列名（附带类型）和常用关键字（`and` `or` `is null` `in` `like` `between` 等）。在 `列 =`、`列 <>`、`列 in (` 之后，枚举列和布尔列给出可选值 |

- **排序**：先按上表中候选组的顺序，再按 fzf 得分。
- **数据来源**：候选全部来自 session 的 catalog 缓存，在本地匹配，不发请求。某张表的列第一次用到时，才从 `Meta` 连接获取。

**交互**（与 fzf 以及常见编辑器的补全一致）：

- 输入标识符字符时自动弹出候选列表，随输入实时重排，匹配到的字符用 `match` 色的前景标出，不加底色（与命令面板相同，§7.3）。
- 每一项右侧标注类别：表、视图、列的类型、关键字。列还会标出所属的表。
- **输入的第一个字符必须落在候选的词首**（M2/M3 验收时用户要求 `evt` 能匹配 `mt_event`，取代 F1.14 的「首字符相同」）：词首是候选的开头，或 `_`、`.`、`-`、`$` 之后的字符，以及小写到大写的切换处；空格、字母后面的数字不算，所以 `nu` 不出 `is null`。fzf 给出的对齐首字符不在词首时，从词首重新对齐一次再判断。其余字符照 fzf 模糊匹配。不分大小写；取值位置去掉引号后比较，前缀为空时不过滤。`evt` → `mt_event`、`tord` → `t_order` 都能匹配，`x` 仍不会弹出 `max`、`exists`。VS Code 的补全也是按词首过滤的。补全整串都不分大小写，不用 fzf 的 smartcase：SQL 的关键字和不加引号的标识符本来就不分大小写，用大写写 SQL 的人输入 `STA`、`T_OR` 也要有补全（tester 在 F1.14 实测）。命令面板、树的过滤、COLS 这些列表不受影响，照旧是完整的 fzf 匹配和 smartcase。
- 弹出时第一项就是选中的，用 `select` 底（M1 用户反馈）。`Tab` / `C-n` / `↓` 移到下一项，`S-Tab` / `C-p` / `↑` 移到上一项，到头绕回：最后一项再往下回到第一项，第一项再往上到最后一项。
- **`↵` 只接受**（M8 F8.1，取代 M1–M5 的「文字不变就执行」和 F5.12 的「只有补空格算接受」：那两条留下了 `id↵` 换行、WHERE 里直接执行的洞）：列表开着时 `↵` 一律接受选中项，不执行、不换行，就算文字一个字都没变；想换行或执行再按一次 `↵`。列表没弹出时 `↵` 照常执行。已知的代价：输入的是某个候选的前缀，比如别名 `as ma`，`↵` 会补成 `max`，要执行得先 `esc`；`insert into t (` 之后「全部列」选中，`↵` 一定插入全部列，想换行要先 `esc`（F5.13）。
- **接受后补空格**（F5.12 起关键字补，M8 F8.1 起按「后面自然接什么」）：关键字、表名、列名后面接的是下一个词，补一个空格，光标落在空格后面；光标后面已经是空白时只把光标移过去；**光标后面紧跟 `)` `,` `;` `.` 时也不补**（F8.1 审查时定：要接的东西已经在那里了，开着自动配对在括号里补列名每次都会多出 `status )`）。值（`null`、`true`、`false`、`is null`、`is not null`）、函数（接 `(`）、枚举值（接右引号）、INSERT 列清单里的列（接 `,` / `)`）不补。WHERE 执行和记历史前去掉首尾空白（`paid↵↵` 不记成 `paid `）；console 里留下的行尾空格不管。
- **列表只在输入文字时弹出**（§9.7 开头的「输入时自动弹出」，F8.1 审查时重申）：进 INSERT 的 `i` / `a` / `A` / `o`、方向键这些不改文字的键不弹；原来也弹，只是那时 `↵` 文字不变就执行所以无害，现在会把 `A↵` 变成只补一个空格、`i↵` 变成 `where id id = 1`。`C-n` 手动唤起时列出该位置的全部候选（`select ` 还没有 `from` 时是表和关键字，`where ` 后面列在前、关键字也在），不因为 F8.4 的「只给列」少掉东西。
- **和输入的词完全相同的候选排第一**（不分大小写，F5.12 审查时定）：排在分组顺序之前，否则打完 `set`、`to` 这类短关键字按 `↵`，会被模糊匹配到的表名（`schema_migrations`、`t_order`）换掉。
- 快速 SQL 的面板里，`Tab` / `S-Tab` 在列表开着时移动候选，关着时照常切换范围。
- `esc`：快速 SQL（没有 vim 模式的单行输入框）里，第一次关闭候选列表，第二次才退出输入（WHERE 在 F3.39 起同 console：一次 `esc` 关列表并回 NORMAL），与 COLS 下拉「先清空，再关闭」的规则一致。console 里照 nvim-cmp 的做法，`esc` 关掉列表的同时退出 INSERT：console 里几乎每敲一个标识符都会弹出列表，「打完一个词按 esc」是 vim 用户最常见的动作，分两步会让后面的 `k`、`0` 都被当成文字打进去（M3 F3.10 实测）。
- 鼠标：悬停即移动选择，点击即接受。

**WHERE 补全的细节**（M1 F1.5；上下文判断放在 `sqlkit.WhereContext(text, pos)`。M3 起它和 `CompletionContext` 建在同一个扫描器上，但仍是两个函数：WHERE 的「取值模式」是 WHERE 独有的，合并后重复的只剩几行）：

- 先跳过字符串、带引号的标识符和注释，再看光标前面：光标前是标识符的一部分，就是列名 / 关键字模式；光标前是 `列 =`、`列 <>`、`列 in (`（含 in 列表里逗号之后），后面可以跟半截值（`'d`、`tr`），就是取值模式，只针对枚举列和布尔列。写之前先看 lazysql 的 `sql_lexer.go` / `sql_context.go`。
- **弹出时机**：列名 / 关键字模式在输入标识符字符时弹出，前缀为空不弹（刚敲了空格）；取值模式到了取值位置就弹，前缀可以为空，所以输入 `status = ` 就直接列出枚举值。
- **候选**：列名（附类型）在前；关键字在后：and、or、not、is null、is not null、in、like、ilike、between、true、false、null；取值模式下枚举值带引号（`'done'`）、按 enumsortorder 排，布尔值为 true / false。先按组，组内按 fzf 分数。
- **接受**：用候选替换光标前的前缀，取值模式下替换半截值（`'d` → `'done'`）。**M8 F8.1 起**（用户：打完 `id` 按 `↵` 还换了行 / WHERE 里直接执行了）：列表开着时 `↵` 一律只接受，不执行、不换行，就算文字没变也一样，上面那条「跨过右引号算文字没变、照常执行」的例外作废；补空格按「后面自然接什么」：关键字、表名、列名补一个空格（光标后已是空白就只移过去），值、函数、枚举值、INSERT 列清单里的列不补。
- **空前缀的列表不预选**（M8 F8.17，用户：接受 `where` 补了空格之后还得再敲一个空格才看到列名）：前缀为空时自动弹出的列表（F8.4 的列名、`别名.` 后的列名；`from ` 后的表名不自动弹，只有 `C-n`）**不预选任何一项**，`↵` 照常换行 / 执行，`Tab` / `C-n` / `↓` 才选中第一项，打了字母过滤后第一项自动选中、`↵` 接受（同 nvim 的 `noselect`）；所以接受 `where` / `select` / `and` 等词补出空格后列表立即弹出，不再区分「空格是打的还是补的」。例外：`insert into t (` 弹出的「全部列」仍预选（F5.13，用户要的就是 `↵` 一下全加上）；WHERE 取值位置（`status = `、`paid = `）的枚举 / 布尔值列表也仍预选第一项（F8.17 审查时定：缺值的条件执行一定报错，`↵` 在这里没有别的有意义的结果，F1.5 / F1.14 的验收不变）。
- **空前缀的列名补全**（M8 F8.4）：光标前一个词是 `select`、`where`、`and`、`or`、`on`、`set`、`having`、`by`，或者在 select / set / order by 列表的 `,` 后面，且语句里找得到表（`from` / `update` / `delete from` / `insert into` / `join`，含别名）时，打了空格就弹出这些表的列（光标紧跟空白才弹，`a,` 不弹；F8.17 起接受关键字补出的空格也弹，列表不预选所以 `where↵↵` 仍是换行），前缀为空、不含「全部列」项，多表的排法同打了字母之后、不另加 `表.列` 候选；找不到表不弹；触发词还有 `distinct`（`select distinct `、`count(distinct `），`by` / `on` / `set` 只看前一个词。快速 SQL 和 console 共用补全，行为一致。WHERE 输入框在 `and ` / `or ` 后面时同样，空输入框不弹（删空再 `↵` 是清除过滤）。已知上限：`between 1 and ` 也弹（要的是值，打数字就关了），MySQL 的 `on duplicate key update ` 和 `update a = 1, ` 不弹。lazysql 的 completer 前缀为空直接返回 nil，没有可借的。
- **列表组件** `ui.Complete`：画在输入框下方、光标所在列，最多 8 行；每行是候选（匹配字符高亮）加右侧 dim 注释（列的类型、「关键字」、「值」）。不复用下拉框，因为补全没有过滤框。键位在 `[keys.complete]`：`Tab` / `C-n` / `↓` 下一项，`S-Tab` / `C-p` / `↑` 上一项。`↵` 和 `esc` 不绑，交给输入框按上面的规则处理：绑在 complete 作用域里的键会被这个作用域吃掉，没法在「没选过」时交回输入框去执行。鼠标：悬停即选中，点击即接受。

**INSERT 的列**（M5 F5.13，用户验收时要求：`insert into t_log(` 之后要能看到有哪些列，也能一下全加上）：

- **位置**：`insert into <表> (` 的括号里，光标在 `(` 之后或逗号之后（可以带半截列名）；表名可以带 schema、带引号，PG 和 MySQL 都算。到了这个位置就弹出，前缀可以为空，和 WHERE 的取值模式一样；所以敲 `(`（自动配对成 `(|)`）或 `, ` 就直接列出来。
- **候选**：这张表里还没写进括号的列，按表里的列顺序，右侧注释照常是类型；不是 `generated always` 的才列（`generated always as identity` 和生成列插不进去，写了会报错）。前缀为空时第一项是「全部列」：显示成剩下的列用 `, ` 连起来，放不下的用省略号，右侧注释「全部 N 列」；有前缀后这一项不出现，其余照 fzf 匹配。
- **接受**：单个列插入列名，不补空格（后面接 `,` 或 `)`）；「全部列」把剩下的列用 `, ` 连起来一次插进去，光标停在最后一列后面，正好在 `)` 前。列名照其他补全的规则原样插入（现在不加引号，代码里有 `ponytail:` 注释标着这个上限，要加时补全各处一起加）。

**WHERE 的 `C-r` 历史 / 收藏下拉**：也按当前输入模糊过滤，体验与 shell 里 fzf 的 CTRL-R 相同。

- **打开**：在 WHERE 输入框的 INSERT 下按 `C-r`（Action `where.history`，绑在 `[keys.input]`，只在 WHERE 输入框里有效果）、NORMAL 下按 `/`（F3.39，`[keys.wherenormal]`），或点击 WHERE 行右端的 `▾`（§7.8）。
- **过滤**：不另设过滤框，直接用 WHERE 输入框里的文字做 fzf 过滤，边输入边过滤。
- **列表**：历史在前、收藏在后（M8 F8.14，用户要历史在上；原来收藏在前），两组都非空时中间一行 `sep` 色的横线，组内按时间从新到旧；每项前面画图标 `star` / `history`，不画组标题（F3.31）；底部一行提示（移动 · 应用 · 收藏 / 取消收藏 · 关闭，键位从 keymap 读）；WHERE 行右端的 `▾` 前面写打开下拉的键（`history` 图标 + `where.history` 的键，`dim` 色，这一段也能点开下拉；输入框留得下 16 列才显示，放不下只留 `▾`，F8.14 审查时定）；`icons = "ascii"` 时也不画，`*` / `h` 和右侧的说明已经能区分。打开时列出全部，不按输入框里已有的文字过滤，开始输入后才过滤。右侧说明：收藏项显示附带的 ORDER / LIMIT，跟默认值不同时才显示（`status ↓ · 500`）；历史项显示时间（`09-24 14:05`）。
- **按键**（`[keys.where]`，Action `where.up/down/apply/star/close`）：`C-n` / `Tab` / `↓` 下一项，`C-p` / `S-Tab` / `↑` 上一项，到头绕回（F3.31）；`↵` 应用，把 WHERE、ORDER、LIMIT 都换成这一项的，从第 1 页开始执行；`C-f` 收藏 / 取消收藏；`esc` 关闭，输入框里的文字保留。
- **历史的规则**：每次 `↵` 执行一条非空的 WHERE，就连同当时的 ORDER / LIMIT 记一条；内容相同的去重并挪到最前面；每张表最多 100 条（F3.31，原为 50），满了丢最旧的。快速 SQL 的历史仍是 50 条。收藏不限数量。

## 10. 单元格编辑与提交（G-02、G-03）

### 10.1 进入与记录

- **行标识列**：保存修改时，需要用它唯一定位到要改的那一行。依次取：
  1. 主键；
  2. 没有主键时，取一个所有列都不可空的唯一索引，效果与主键相同；
  3. 两者都没有，这张表只读，在 toast 中说明原因。
  - MySQL 另有两种表只读（F5.4 审查时定）：行标识列里有 binary / varbinary / blob / bit 列的（如 binary(16) 存的 UUID，读到的是 `<binary 16 bytes>`，存不回去）；引擎不支持事务的（MyISAM 等，catalog 读 `tables.engine`），因为保存做不到原子，第 2 行出错时第 1 行已经写进去了。
- **进入编辑**：表有行标识列、且所属连接不是只读（M5 F5.3）时才能进入编辑，否则用 toast 说明原因。只读连接上编辑、新增行、删除行、粘贴都拦下，toast「<连接> 是只读连接」。result pane 里的表格只读，因为无法可靠地确定来源表和主键。
- **没有行标识列时**：`↵` / `i` / 双击 / 粘贴都不进入编辑，toast 说明原因，如「t_log 没有主键，也没有全部列都非空的唯一索引，只读」。视图、物化视图、外部表没有主键，也走这条。只读连接的拦截（S-04）见上。
- **粘贴**：在表格 NORMAL 下粘贴（bracketed paste），会以粘贴的内容开始编辑当前单元格。
- **行内输入框**：就画在单元格上，至少和单元格一样宽；文字更长时向右延伸，最多到 pane 右边框，盖住右边的格子；再长就在框内横向滚动，跟着光标走。转置视图下编辑光标所在的格。
  - 进入时内容全选（`select` 底）：第一个可打印字符替换全部，退格清空，`←` / `→` 取消全选、光标落到对应的一端。起始文字：原值是 NULL 或 DEFAULT 时为空，已有修改时取修改后的值。
  - 含换行、Tab 的值：和表格一样把换行画成 dim 的 `↵`、Tab 画成空格，原字符不变；`↵` 是提交，所以不能新输入换行。多行编辑等 console 的编辑器有了再说。
- **提交**：`esc`、`↵`、点击别处都提交并退出编辑，回到 NORMAL，光标不动；点击别的格子是先提交、再把光标移过去，不进入编辑。编辑中按全局键（`C-p`、`C-s` 等）、滚动滚轮，也是先提交再执行，免得输入框停在已经滚走的格子上。
  - **没改就不算修改**：提交时文字和进入编辑时的起始文字相同，就什么都不记，已有的修改（包括 DEFAULT）原样保留。否则原值是 NULL 的格进去再出来，就变成了空字符串。
  - 编辑中状态栏的模式附加信息显示 `-- editing <列名> --`，与 `-- editing WHERE --` 对应（§7.8）。
  - 输入框没有边框，底色用 `cursor`，终端光标放在输入位置。
- **粘贴**（M2 F2.1 起）：所有单行输入框（单元格、WHERE、面板、树的过滤框、PAGE 等）都把粘贴的文字插到光标处，换行换成空格；在单元格编辑中也一样。之前这些输入框都不处理粘贴，粘贴没有反应。
- **修改的存储**：`Edits` 以（原行标识值, 列）为 key，修改后的值可以是文本、NULL 或 DEFAULT（§10.2）。
  - 翻页后修改仍然保留，保存按钮上显示所有修改的总数。
  - 改回原值时，这条修改被删除，标记自动消失。文本和原值逐字比较；NULL 与原值 NULL 相同。
  - 改 WHERE / ORDER / LIMIT 也保留修改，这些行重新出现在页里时照样标记。
  - 有修改的行，行号用 `warn` 色（M2/M3 验收时定）；保存失败的那一行照旧是 `error` 色，优先。
  - **`r` 撤回光标所在格的修改**（Action `grid.revert`「撤回这一格的修改」，M2/M3 验收时定），等同选项里的「↺ 原值」；在新增的行上撤回整行，在标了删除的行上取消删除（§10.6）。`R` 照旧是刷新。

### 10.2 按列属性提供选项

编辑时，如果该列有可用的选项，单元格下方会出现一个选项浮层。浮层不抢输入框的焦点：键盘照常输入文字，也可以直接用鼠标点选。

| 列属性（来自 catalog） | 选项 |
|---|---|
| 可空 | `∅ NULL` |
| 有默认值（`column_default` 不为空） | `DEFAULT`：写入时使用 SQL 关键字 `DEFAULT`，而不是字符串。做法参考 lazysql 的 `set_value_list` |
| 本格有待提交的修改 | `↺ 原值` |
| 布尔（PG boolean；MySQL tinyint(1)） | `true` / `false`（MySQL 写入 1 / 0） |
| 枚举（PG enum；MySQL ENUM） | 全部枚举值，随输入内容模糊过滤 |
| 时间（date / time / timestamp / timestamptz / datetime） | 分段调整，加上 `◷ 现在` |

时间列的浮层（对应 DataGrip 的日期选择器，改成适合终端的分段形式）：

```
 2026-09-20 02:49:23.305808+08 ▾          行内输入框（INSERT）
┌──────────────────────────────────┐
│  ▴      ▴     ▴     ▴    ▴    ▴  │
│ 2026 - [09] - 20    02 : 49 : 23 │      分段调整，当前段高亮
│  ▾      ▾     ▾     ▾    ▾    ▾  │
│ ◷ 现在    ∅ NULL    ↺ 原值        │      选项
└──────────────────────────────────┘
```

**键盘**（`cell` 作用域）：

- 直接输入：编辑文字。
- 时间浮层打开时（浮层作用域 `segments`）：`Tab` / `S-Tab` 依次走过各段、再走到选项行的每一项，到头绕回（F3.34）；在段上 `↑` / `↓` 加一 / 减一，在选项上 `↵` 应用；`C-n` / `C-p` 在选项行里移动。
- 在还没保存的新行里编辑时（F3.33），`Tab` / `S-Tab` 改为提交这一格并到下一个 / 上一个字段直接编辑，浮层里不用 `Tab`：选项用 `↑` / `↓` / `C-n` / `C-p`，时间段用 `↑` / `↓` 加减、点击切段，选项行用 `C-n` / `C-p`。
- 其他列的选项浮层打开时（浮层作用域 `options`）：`Tab` / `C-n` / `↓` 下一项，`S-Tab` / `C-p` / `↑` 上一项，到头绕回；还没选中时，第一次 `Tab` 选中第一项、第一次 `S-Tab` 选中最后一项（M2/M3 验收时用户要求能用 Tab 选 true / false / NULL）。
- 这两个浮层作用域只在浮层展开、而且有选项时生效；浮层没打开或已收起时，`Tab` / `↑` / `↓` 在单元格里不做事，`C-p` 照 §10.1 当作全局键：先提交编辑，再打开命令面板（tester 在 F2.1 发现：绑在 `cell` 里会把 `C-p` 吃掉，而 F2.1 还没有选项可移）。
- 弹出时不预先选中任何一项（与补全不同）：`↵` 默认是提交文字，NULL / DEFAULT 被 `↵` 误用的代价太大。
- `↵`：有选中的选项时应用该选项，否则提交文字。
- 选项的顺序：布尔的 `true` / `false` 或枚举值 → `∅ NULL`（可空列）→ `DEFAULT`（有默认值）→ `↺ 原值`（本格有修改）。没有任何选项的列不弹浮层。枚举值和布尔的 `true` / `false` 用 `ui.Filter` 按输入过滤（完整 fzf，这里不是补全），还在全选状态、没开始输入时列出全部；NULL / DEFAULT / 原值不参与过滤，始终在后面；过滤后一项都不剩时不画浮层。文字每变一次，选中复位为「没有选中」。
- 浮层复用补全列表的画法；输入框下方放不下全部选项、而上方更宽裕时，开在上方。这条放在补全列表和选项浮层共用的位置计算里，补全列表也照此。
- 鼠标悬停只高亮、不改选中项，免得指针划过之后按 `↵` 就写进了 NULL；这里故意和补全列表不同。悬停用 `row` 底，与树、表格行、下拉框的悬停一致，和选中项的 `select` 底分得开，一眼能看出 `↵` 会应用哪一项。点击照常立即应用。
- 应用 NULL / DEFAULT / 枚举值 / 布尔值：写入修改并退出编辑；应用「↺ 原值」：删掉这条修改并退出。
- `esc`：提交并退出编辑，保留修改（G-02）。

**鼠标**：

- 点击 ▴ / ▾，或在某一段上滚动滚轮：该段加减。
- 点击某一段：选中该段。
- 点击选项：立即应用。
- 点击浮层外部：提交并退出编辑。
- 点击输入框右端的 `▾`：收起或展开浮层。

**文字与分段的同步**：

- 文字按列的类型格式解析：PG 在 ISO DateStyle 下为 `YYYY-MM-DD HH:MM:SS[.ffffff][±TZ]`，MySQL 格式相同但没有时区。
- 文字解析失败时，分段区变暗，但仍然可以直接编辑文字。
- 调整某一段时，只改写文字中对应的那部分，小数秒和时区后缀保持不变。
- 各段在自身取值范围内循环，不向上一级进位（与 macOS、DataGrip 的行为一致）。改动年或月后，日会被限制在该月的天数之内。

**分段**：进入编辑时选中第一段，`Tab` / `S-Tab` 在段之间移动，到头绕回；当前段用 `select` 底标出（设计稿草图里的方括号只是表示高亮）。全选状态下直接调整某一段，会取消全选、光标放到末尾。date 为年月日；time / timetz 为时分秒；timestamp / timestamptz 为年月日时分秒。小数秒、时区后缀、`BC` 保持原样不动；interval 不算时间列。`infinity` 等解析不了的值，分段区变暗，`↑` / `↓` 和 ▴▾ 不起作用。time / timetz 的 `24:00:00` 也按解析不了处理：PG 允许这个值，但在它上面加减分或秒会得到 PG 拒收的 `24:01:00`（reviewer 在 F2.4 的 PG 17 上实测）。

**「现在」**：把当前时间填进输入框，但不结束编辑，方便接着微调某一段，`↵` 才提交（NULL / DEFAULT / 原值照旧是应用即结束）。取客户端本地时间，精确到秒、不带小数秒，按列的类型格式化；timestamptz / timetz 带本地偏移，写法和 PG 的 ISO 输出一致（`+08`、`+05:30`）。

**NULL 与 DEFAULT**：

- 只能通过对应的选项设置。直接输入文字 `<null>` 或 `DEFAULT`，得到的仍然是字符串。
- 修改后的值因此有三种：文本、NULL、DEFAULT。表格里分别显示为原文、`<null>`、`<default>`，都带「已修改」的样式。
- 不可空的列不显示 NULL 选项；没有默认值的列不显示 DEFAULT 选项。
- 命令面板另有「设为 NULL」（`cell.null`）「设为 DEFAULT」（`cell.default`）两条命令，默认不绑定键位。NORMAL 下作用于光标所在的格，编辑中作用于正在编辑的格；列不可空、没有默认值或表没有行标识列时不做事。

### 10.3 保存（C-s / `:w`）

按行分组，每行生成一条 UPDATE，WHERE 中带上被修改列的旧值，作为乐观校验：

```sql
UPDATE t SET c1 = $1, c2 = DEFAULT
WHERE pk = $2 AND format('%s', c1) = $3 AND c2 IS NULL
-- pk 为行标识列（§10.1），有多列时逐列比较；旧值是 NULL 的列写 IS NULL，其余用 format('%s', 列) = 旧值；MySQL 用 cast(convert(列 using utf8mb4) as binary) = 旧值；DEFAULT 直接写成关键字，不作为参数
```

- **旧值按输出函数的文本比较**：旧值不是 NULL 时写 `format('%s', 列) = $n`，是 NULL 时写 `列 IS NULL`（生成语句时旧值是已知的）。我们读到的是服务端输出函数给出的文本，`format('%s', …)` 走的也是输出函数，`Main` 和 `Meta` 的会话参数相同（DateStyle、TimeZone 等来自同样的建连参数），所以两边一致。行标识列照旧写 `pk = $n`：主键和唯一索引的类型一定有 btree 相等运算，也用得上索引。只校验被修改的列。
  - **MySQL**（F5.4 逐类型实测）：旧值不是 NULL 时写 `cast(convert(列 using utf8mb4) as binary) = ?`，是 NULL 时写 `列 IS NULL`。先 convert 到 utf8mb4 是为了 latin1 / gbk / big5 等字符集的列（审查时发现：`cast(列 as binary)` 拿到的是列自身字符集的字节，和我们传的 UTF-8 文本永远不等）；假定连接字符集是 utf8mb4（驱动默认），用 `ponytail:` 标出。MySQL 走文本协议，每个值都以文本发回，CAST AS BINARY 对所有类型都还原出这段文本，逐字节比较。否掉 `<=>` 和 `=`：它们先把文本转成列的类型再比，json 列和字符串永远不相等，float 差在精度上；又按排序规则比，`Abc` 被别人改成 `abc` 查不出来。实测覆盖 float、double、decimal、datetime(3)、timestamp(6)、date、time、year、bit、binary、varbinary、enum、set、char(n)、json、boolean、bigint unsigned、text 和 NULL。
  - 否掉 `列 IS NOT DISTINCT FROM $n`：json、point 等类型没有相等运算符，直接报错（M2 核对时在 PG 17 上实测），seed 里就有 json 列。
  - 否掉 `列::text IS NOT DISTINCT FROM $n`（F2.2 起初的写法）：不少类型到 text 的 cast 不走输出函数，结果和读到的不一样。boolean 输出 `f`、`::text` 却是 `false`，布尔列的修改因此永远保存不了（tester 在 M2 节点 2 实测）；inet 的 `::text` 带掩码；char(n) 的 `::text` 去掉尾部空格。
  - 集成测试要逐个类型实测往返：boolean、inet、char(n)、json、jsonb、numeric、real / double、timestamptz、interval、bytea、数组、枚举。在自建库里建一张临时表测，不改 seed。
- 标识符用 `pgx.Identifier{schema, table}.Sanitize()`；参数按文本传，OID 为 0，由服务端从列推出类型。
- **执行**：在 `Main` 的 Worker 同一把锁里 begin → 每行一条 UPDATE、按影响行数（`Result.Affected`）检查恰好 1 行 → commit，任何一步出错就 rollback。参照 lazysql 的 `queriesInTransaction`，但它不检查影响行数、也不带旧值校验，这两点按本节做。
- **顺序**：按行标识值排序后逐行执行，每次顺序一样（也减少和别的会话互相等锁）；一行里的 SET 按列在表里的顺序。
- **触发**：`C-s`、`:w`、点击保存按钮，都只保存焦点所在 data pane 的当前 tab；焦点在别处时不做事（console 的 `C-s` 在 M3）。没有修改时不做事、不提示；保存进行中再按忽略。
- **取消**：保存进行中照常算 busy，`C-c` 取消它（`cancel` 同时取消 `Meta` 和 `Main` 上的请求）：回滚，修改保留，查询条右侧显示「已取消，已回滚」。COMMIT 和 ROLLBACK 本身用不可取消的 ctx（带超时）执行：COMMIT 发出之后再取消，客户端可能收到错误、而服务端其实已经提交，界面就会报「已回滚」而数据已经改了。
- **保存期间又改了格子**：成功后只清掉这次送出去、之后没再改过的修改，保存期间新改的留着。
- **重新加载**连计数一起重取，UPDATE 可能把行改出 WHERE 的范围。
- **结果显示在查询条右侧**（Q-06）：
  - 成功：清掉这个 tab 的全部修改，重新加载当前页，显示「已保存 N 行 · 12ms」（N 按行计）。这行文字一直显示到下一次由用户发起的取数或编辑，保存后自动的那次重新加载不清掉它。
  - 失败：整体回滚，修改标记保留，原因显示在 pane 底部的错误栏里（§7.8「错误栏」，M2/M3 验收时从查询条挪过去，查询条右侧只留成功的提示），并用行标识值指明是哪一行（它可能不在当前页；多列时用逗号连起来，如 `order_id = 3, line_no = 1`，值照读到的文本写、不加引号）：影响行数不是 1 时为「id = 42 的行数据已变化或行不存在，已回滚」；数据库报错时为「id = 42：<数据库的错误>，已回滚」。数据库的错误只取 PG 错误的 Message，去掉 `ERROR:` 前缀和 `(SQLSTATE …)` 后缀；放不下时截短中间的错误原文并加 `…`，开头的「id = 42：」和末尾的「，已回滚」始终完整显示（tester 在 F2.2 发现：原来放不下就整条不显示，PG 的报错大多超过 60 列）。那一行在当前页上时，行号画成 `error` 色。

- **auto 事务模式**：所有 UPDATE 放在同一个事务里，每条都必须恰好影响 1 行。否则整体回滚，保留修改标记，并在出问题的行上提示「数据已变化或行不存在」，以免悄悄覆盖别人的修改。
- **manual 事务模式**（M5 F5.3），以及 auto 模式下 `Main` 已经在事务里时（用户在 console 里自己 begin 了；在保存的同一次 Run 里读 TxStatus 判断，原来的 BEGIN…COMMIT 会把用户的事务一起提交，M3 的已知上限就此去掉）：不在事务里就先 BEGIN；整次保存包在 `SAVEPOINT sqlmux_save` … `RELEASE SAVEPOINT sqlmux_save` 里，任何一步出错就 `ROLLBACK TO SAVEPOINT sqlmux_save`，只撤掉这一次保存，用户在这个事务里之前做的修改都还在；不 COMMIT，成功时查询条显示「已保存 N 行（未提交）· 12ms」；失败的提示同 auto 模式，其中的「已回滚」指撤掉了这次保存。提交、回滚用 console 标题上的 ✓ ↺ 或 `tx.commit` / `tx.rollback`。
- **保存成功后**：重新加载当前页，拿到数据库规整过的值，例如数值格式、触发器写入的字段。

### 10.4 刷新（R）

丢弃全部修改后重新取数。有未保存的修改时先弹确认框（§10.5），与关闭 tab、退出一致，免得误按一个键就丢掉一片修改（M2 定，PRD 10.2 #1 按此处理）。

### 10.5 未保存修改的确认框

通用的确认浮层 `ui.Confirm`：在整个窗口里居中的小框，不压暗背景，点框外等于取消；一句话加两个按钮，按钮的键位文字从 keymap 读（`confirm.yes` / `confirm.no`）。文字按场合写，N 是修改的格数：退出「有 N 处修改未保存，退出会丢弃。」`y 退出`；关 tab「t_order 有 N 处修改未保存，关闭会丢弃。」`y 关闭`；关 pane「这个 pane 里有 N 处修改未保存，关闭会丢弃。」`y 关闭`；R「有 N 处修改未保存，刷新会丢弃。」`y 刷新`；都配 `n 取消`。M5 F5.3 起，有未提交的事务时同样确认：「<连接> 有未提交的事务，<动作>会回滚。」，有几个连接时用「、」连起来并去重；同时有未保存的修改时，一个框里两句都写。关 session、关 window 时修改那一句带上主语：「<session 名> 里有 N 处修改未保存，关闭会丢弃。」「这个 window 里有 N 处修改未保存，关闭会丢弃。」（同关 pane 的「这个 pane 里」）。按 `y` 之后：关 tab / pane / window 先关，再在 Cmd 里对因此不再有 console 的连接发 ROLLBACK，之后同 §7.8 重取表格；退出、关 session 直接关连接，由服务端回滚（同 F5.1）。只在手动模式下保存过表格、没开过 console 的连接，关表格时不问，退出、关 session 照样问。统计范围：关 tab 只算这个 tab，关 pane 算它的所有 tab，退出算所有 window 的所有 tab。作用域 `confirm`，`y` / `↵` 确认，`n` / `esc` 取消，`C-c` 在浮层里等同 `esc`；按钮可以点击。

触发的地方：

- `:qa` 和第二次 `C-c` 退出：统计所有 window、所有 tab 的修改；
- `x` / `:q` 关闭有修改的 tab；
- `SPC x` 关闭的 pane 里有带修改的 tab；
- 有修改时按 `R`（§10.4）；
- 关闭 window（F5.2）、关闭 session（F5.1）：统计其中所有 tab 的修改；
- 有未提交的事务时（F5.3）：退出、关闭 session，以及让这个连接上不再有 console 的关闭（关 tab、关 pane、关 window）。

从树或面板按 `↵` 打开表、而当前 tab 有修改时，不弹确认，改为新开 tab（§12）。

### 10.6 新增与删除行（M2/M3 验收时定）

- **新增**：`o` 或查询条的 `+`，在光标所在行的下面插入一个新行，光标移到它的第一列。新行的行号显示为 `warn` 色的 `+`，每一格先显示 `<default>`（写入时用 DEFAULT），照常编辑。新行只在当前页里，翻页之后仍保留，回到这一页时在原处：记的是「第几页、页里第几行后面」，改 WHERE / ORDER / LIMIT 或刷新后照样放回去，页或行不够时挂到最后一页末尾。没改过的格用 `dim` 色画 `<default>`。
- **复制为 INSERT**（`yi`，M8 F8.7 才真正实现；F3.32 只留了占位）：当前行拼成一条 `insert into <schema>.<表> (<全部列，去掉 generated>) values (…);` 写进剪贴板，标识符走 `Engine.Quote`，关键字跟 `keyword_case`，数字不加引号、其余单引号（PG `'` 加倍，MySQL 另转义 `\`），值取显示的（含未保存的修改）；结果 tab 上 toast「只有表里的行能复制为 INSERT」；行里有显示成 `<binary N bytes>` 的格（MySQL 非 UTF-8 的二进制，原始字节不在内存里）时 `yi` 和 `p` 都不做、toast「有二进制列，复制不了」；新增行每一格都是 DEFAULT 时写成和保存一样的 `insert into t default values;`（MySQL `insert into t () values ();`）。
- **复制成新行**（F3.32）：`yy` 复制整行，`p` 在光标下面粘贴成一个新行，各格取复制的值；行标识列里有默认值的（serial / identity）填 `<default>`，其余照抄。
- **删除**：`dd` 或查询条的 `−`，把光标所在行标为删除：整行 `dim` 色加删除线，行号显示为 `error` 色的 `−`。删除标记按行标识记，翻页、刷新后照样标着。标了删除的行不能进入编辑，同只读表格不提示，要改先按 `r` 取消删除。光标在新行上时，状态栏的「行,列」里的行显示行号栏画的标记（`+`），和表格上看到的一致。再按一次 `dd`，或 `r`，取消删除。在还没保存的新行上 `dd` 直接去掉它。
- **保存**（与 §10.3 同一个事务）：先 DELETE，再 UPDATE，最后 INSERT，每条都必须恰好影响 1 行，否则整体回滚。
  - DELETE：`DELETE FROM t WHERE <行标识列> = $n`，同 UPDATE 用行标识值定位；不校验其他列。
  - INSERT：只写改过的列，其余用 DEFAULT；一格都没改的新行写 `INSERT INTO t DEFAULT VALUES`。
  - 已经标了删除的行上的修改，保存时丢掉；取消删除后恢复。
  - 失败时的文字：INSERT 没有行标识，写成「新增的第 2 行：<错误>，已回滚」（按新行的显示顺序数），影响行数不是 1（如 BEFORE 触发器返回 NULL）时写成「新增的第 2 行没有插入，已回滚」；DELETE 影响行数不是 1 时写成「id = 42 的行不存在，已回滚」，数据库报错同 UPDATE。
- 保存按钮和确认框里的「N 处修改」，把新增的行、删除的行各算一处，加上修改的格数。
- 没有行标识列的表照旧只读，新增和删除都不行。
- 参考 lazysql 的 `ExecutePendingChanges`（DELETE / INSERT / UPDATE 的顺序与事务）。

### 10.7 字段的前置校验（M2/M3 验收时定）

编辑单元格时，按列的类型检查输入的文字，不对就当场提示，不必等到保存时由数据库报错。这只是提示，最终以数据库为准。

| 类型 | 允许的写法 | 提示 |
|---|---|---|
| smallint / integer / bigint | 可带正负号的整数，在该类型的范围内；照 PG 17 也认前后空格、`0x1F`、`1_000` | 「不是有效的整数」「超出 int4 的范围」 |
| numeric / real / double precision | 小数、科学计数法、`NaN`、`Infinity`、`-Infinity`；照 PG 17 也认 `inf`、`1_000.5`、`0x10`（real / double 也认）。带精度的 numeric(p,s)：按 s 位四舍五入之后整数部分不超过 p−s 位，不接受 Infinity；小数位多了 PG 会自动舍入，不算错。real / double 检查各自的范围 | 「不是有效的数字」「超出 numeric(10,2) 的范围」「超出 float8 的范围」「超出 float4 的范围」 |
| boolean | PG 认的写法：`t f true false yes no on off 1 0` 及其唯一前缀（`tr`、`ye`、`of`），不分大小写，可带前后空格 | 「不是有效的布尔值」 |
| date / time / timestamp 系列 | ISO 写法（比 §10.2 分段宽，分段仍只认 PG 的输出格式）：日期 `YYYY-M-D`，可带 ` BC`；时间 `H:MM[:SS[.f]]`，允许 `24:00:00`；timestamp / timestamptz 是日期，后面可以跟空格或 `T` 加时间，再可以跟时区 `±HH[:MM]` 或 `Z`（timestamp 带时区也放行，PG 会忽略它）。或者该类型认的特殊词：time 只有 `now` `allballs`；date / timestamp 有 `now` `today` `tomorrow` `yesterday` `infinity` `-infinity` `epoch`。PG 还认的其他写法（`2026/09/20`、月份名）会被挡住，是已知上限 | 「不是有效的日期 / 时间」 |
| uuid | 32 个十六进制字符，可带连字符（任意每 4 位一个）或成对的花括号 | 「不是有效的 UUID」 |
| json / jsonb | 合法的 JSON | 「不是有效的 JSON」 |
| 其他 | 不检查 | |

- 不合法时：输入框的文字加 `error` 色的波浪下划线（SGR 4:3，终端不支持时退化为普通下划线），下方弹出一个小框显示提示；有选项浮层时上下叠放，提示框紧贴输入框，选项浮层接在它外侧。各类型的写法以 PG 17 的输入函数为准，集成测试逐条拿 `select '<写法>'::<类型>` 核对。
- 不合法时 `↵`、点击别处、滚轮、`C-p`、`C-s`、换焦点的键都不提交，留在编辑里；`esc`（或 `C-c`）放弃这一次输入，回到进入编辑之前的值，这一格原来就改过的回到改过的值。这是 §10.1「esc 提交」的例外：带着一个明知不合法的值退出，保存时只会整批回滚。
- 文字和进入编辑时一样（没改过）就不检查：本来就不算修改，也免得 PG 自己输出的值被拦住。改过的文字照常检查，包括清空：把非空的数字格清空，PG 会报 22P02，所以提示「不是有效的数字」，要写 NULL 请用选项。NULL / DEFAULT 通过选项设置，不受影响（M3 F3.21 reviewer 实测）。

## 11. Console 与 result

**编辑器**（`internal/editor`）：覆盖 nvim 的基础编辑操作，行为以 nvim 为准。不做宏和 `.` 重复。预计 2000–3000 行加测试，其中块选择约 300–500 行；这是整个项目里最大的一块自研代码。

**支持的操作**（都支持次数前缀）：

| 类别 | 内容 |
|---|---|
| 模式 | NORMAL、INSERT、REPLACE（`R`）、VISUAL（`v`）、VISUAL LINE（`V`）、VISUAL BLOCK（`C-v`） |
| 移动 | `h j k l`、`w b e ge`、`W B E gE`、`0 ^ $`、`gg G {n}G`、`f F t T ; ,`、`%`、`{ }`、`H M L`、`C-d C-u C-f C-b`、`zz zt zb`、`* #` |
| 搜索 | `/ ?` 回车，`n N` 跳转。正则用 Go 的 RE2 语法（接近 vim 的 `\v`），smartcase |
| 进入 INSERT | `i a I A o O`、`s S` |
| 操作符 | `d c y`、`> <`、`gu gU g~`（大小写）、`gc`（切换 `--` 注释，同 nvim 0.10 起内置的行为）。操作符可以接任意移动或文本对象，例如 `dw`、`c$`、`y2j`、`gUiw` |
| 简写 | `dd cc yy`、`D C Y`（`Y` 等同 `y$`，与 nvim 默认一致）、`x X`、`r{字符}`、`~`、`J`、`p P`、`>> <<`、`gcc` |
| 文本对象 | `iw aw`、`iW aW`、`i( a(`（`ib ab`）、`i[ a[`、`i{ a{`（`iB aB`）、`i' a'`、`i" a"`、`` i` a` ``、`ip ap` |
| VISUAL | 可以用移动和文本对象扩展选区（如 `viw`、`vi(`）；`o` 切换选区的两端，`gv` 重新选中上次的选区；选区上可用 `d c y x > < u U ~ J gc` |
| VISUAL BLOCK | 按显示列选出一个矩形；`$` 把选区延伸到每行行尾；`o` / `O` 切换对角。可用的操作：`I` / `A` 在每行的块首 / 块尾插入（输入先显示在第一行，按 esc 后复制到其余各行；`A` 会给较短的行补空格）；`c` 修改整块；`d` / `x` 删除整块；`y` 复制整块，寄存器记为块类型；`p` / `P` 按块粘贴；`r{字符}`；`~ u U`；`> <`；`gc` |
| 撤销 | `u` 撤销、`C-r` 重做、`U` 撤销当前行上的全部修改，与 nvim 一致。想把 `U` 当成重做（Helix 的习惯），映射一行 `U = "<C-r>"` 即可 |
| 命令行 | `:{n}` 跳到第 n 行；`:s` 与 `:%s` 替换，支持选区范围 `'<,'>` 和标志 `g i`，替换串中的 `\1`、`&` 按 vim 的写法；`:w`、`:q`。`/`、`?`、`:` 的输入行画在 console 内容区的最后一行（tab 栏上方），模式显示为 COMMAND；VISUAL 下按 `:` 预填 `'<,'>`。`{n}` 和 `s` 由编辑器执行，其余交给 app 按面板的 ex 别名（q、qa、w）执行，都不认识时 toast「不支持的命令：xxx」。搜索找不到时 toast「找不到：<pat>」（对应 nvim 的 E486），正则编译失败时 toast「正则有误：<err>」，错误里不带内部加的 `(?i)` 前缀；没有上一个模式时（`n`、`/<CR>`、`:s//x/`）toast「没有上一个模式」（E35）；从没进过 VISUAL 就用 `'<,'>` 时 toast「没有选区」（E20）；`:s` 的范围超出文本（行号 < 1、大于最后一行，或 `'<` / `'>` 指向的行已被删掉）时 toast「范围无效」（E16），一行都不改，不夹到文本以内。只有行号的 `:{n}` 超过最后一行时夹到最后一行、不报错，是负数时报「范围无效」、光标不动。细节照 nvim（M3 审查时定）：`/x/` 里没转义的 `/` 结束模式，后面的 search offset 忽略、不支持（SQL 里的除号 `/a/b` 也按此处理）；`3:` 这类带次数的冒号换成 `:.,.+2`；替换文字支持 `&`、`\1`、`\t`（Tab）、`\r`（断行），单独的 `~`（上一次的替换文字）不支持 |
| INSERT 下 | Backspace、`C-w`、`C-u`（照 nvim 的默认映射 `<C-G>u<C-W>` / `<C-G>u<C-U>`，先断开撤销步）、方向键；`↵` 换行并保持上一行的缩进（上一行以 `;` 结尾时缩进为 0，`o` / `O` 同；M8 F8.2）；Tab 按 `tab_width` 插入空格。模糊补全见 §9.7：输入时自动弹出，也可以用 `C-n` 手动唤起（`C-p` 在列表没开时是全局的命令面板） |
| 寄存器 | 无名寄存器（带字符 / 行 / 块类型）只留在程序里；F3.38 起只有 `"+` / `"*` 读写系统剪贴板（同 vim 默认的 `clipboard=`，原来每次 yank、删除都写系统剪贴板）：读写的思路同 nvim 的 clipboard provider，先本地工具、再 OSC 52（用户问「为什么 nvim 里可以复制」后定；顺序不完全同 nvim，两个工具都装了才有差别）：依次找 `pbcopy` / `pbpaste`（macOS）、`wl-copy` / `wl-paste`、`xclip`、`xsel`，找到就直接调它，不经过终端，所以 tmux 的 `set-clipboard` 设成什么都不影响；都没有时（比如 SSH 到没有图形界面的机器）才用 OSC 52 写、OSC 52 查询读，终端不回应就什么都不做。工具找到了但执行失败时也退回 OSC 52，不提示。`wl-*` 要 `WAYLAND_DISPLAY`、`xclip` / `xsel` 要 `DISPLAY` 有值；参数照 nvim，但去掉 `-quiet` / `--nodetach`（一次性调用，工具自己 fork 常驻进程拿着 selection）。程序里所有写读系统剪贴板的地方（表格 `yy` / `yl`、快速 SQL 的 `C-y`、`"+` 寄存器）都走这一处。`"+P`、VISUAL 下的 `"+y` / `"+d` / `"+x` 照 vim 的规则；VISUAL 下的 `"+p` 不做，因为 VISUAL 的 `p` 本身就不做（见下面「不做」）。其他具名寄存器（如 `"ayy`）整条命令不执行。表格没有寄存器，`yy` / `yl` 照旧直接写系统剪贴板 |

**寄存器**：除了无名寄存器，只做系统剪贴板 `"+` / `"*`（F3.38，用户要复制到系统剪贴板），读写方式和表格的复制相同；复制时被复制的范围闪一下 `yank` 色（F3.32）。

**不做**：宏（`q` `@`）、`.` 重复、具名寄存器（`"a`–`"z`）、标记与跳转列表、折叠、句子与标签类文本对象（`is` `as` `it` `at`）、`:g` 等其他 ex 命令；也不做 `gJ`、`{count}%`、VISUAL 下的 `p`、`i<` / `a<`、hlsearch / incsearch，以及 nvim 0.10 起的默认映射 `v*` / `v#`（在 VISUAL 下搜索选中的文字）。不折行（nowrap），横向跟着光标滚动，`H M L`、`C-d` 等按可见行数计算，scrolloff 为 0。

**块选择的补充说明**：

- 块的边界落在 Tab 或宽字符中间时，按 nvim 的规则处理，由差分测试兜住（§15）。
- `C-v` 在 macOS 和 Linux 的终端里都能收到。Windows Terminal 默认把它当作粘贴，需要在终端设置里释放这个键，或者把块选择改绑到 `<C-q>`（与 Windows 版 vim 的做法相同）。

**实现结构**：按 vim 的语法组织，即「[次数] 操作符 [次数] 移动或文本对象」，由一个小状态机解析等待中的按键。

- 移动和文本对象都是「位置 → 范围」的函数，范围分为按字符（含端点或不含端点）和按行两种。
- 操作符作用在范围上。
- 因此每新增一个移动或文本对象，都能自动和所有操作符组合，不用逐个组合去写。

**其他要求**：

- **撤销的粒度**：一条 NORMAL 命令，或一次完整的 INSERT，算一个撤销步骤。实现上保存整份快照；缓冲区是 `[]string`，几 MB 以上的文件会变慢，SQL 文件一般遇不到。
- **粘贴**：支持 bracketed paste，粘贴内容一律作为文本插入。否则在 NORMAL 下粘贴，会把文本当作命令执行，可能误删内容。位置照 nvim 的 `vim.paste`：NORMAL 下贴在光标后面（同 `p`），INSERT 下插在光标处，VISUAL 下替换选区。
- **键位映射**：按 §6.6 配置。`[map.console.normal]` 和 `[map.console.visual]` 只对 console 生效，例如 `L = "5l"`。用户映射会覆盖编辑器内置的同名 vim 键（如 `J` 合并行），与 nvim 的 `nnoremap` 行为一致。

**完整 vim 的出口**：命令面板中的「Edit in $EDITOR」命令，通过 `tea.ExecProcess` 暂停 TUI，用外部编辑器打开当前文件，退出后重新载入。默认不绑定键位。

**文件**：

- 每个 console tab 对应 `$XDG_DATA_HOME/sqlmux/consoles/<名字>.sql`，默认名 `console_<n>`（M5 F5.11 起不再按连接名分目录；M8 F8.11 起可以用 `console.rename` 改名，改名就是改文件名，名字规则同连接名，目标已存在时拒绝；state.json 的 `consoles` 和布局文件都按名字记，布局文件的 `n` 改成 `name`，旧的 `n` 读成 `console_<n>`）。新建时 n 取**没有文件、也没在任何 session 里打开**的最小值（M8 F8.21，用户 2026-10-08：关掉 console_3 再新建又是 console_3、带着旧内容，像 DataGrip 那样应该是 console_4，关掉的文件留在列表里随时能再开；原来只看「没被打开」，所以关掉就被复用；两个 session 不能打开同一个文件，自动保存会互相覆盖）；关 tab 不删文件，在树的「Consoles」节点里删掉文件后这个编号才会被新建的用上。启动时的 console_1 文件已经存在就载入，所以每次启动都带着上次写的 SQL。state.json 记着每个文件最后用的连接和 schema（§14），只在启动时恢复默认布局里的 console_1（以后 M6 恢复布局）时用；新建的 console 一律用当前连接（§8.6「默认值」）；记下的连接已经不在 connections.toml 里时用当前连接。旧版本 `consoles/<连接名>/` 下的文件不写迁移代码。
- 最后一次修改后 1s 自动保存（去抖），写临时文件再 rename，目录 0700、文件 0600（§13）；`:w`、`C-s`、关闭 tab、退出时立即写；没改过就不写。写入失败用 toast 提示。文件存在但读不出来时（是目录、权限为 000 等），toast「读取失败：<err>」，这个 console tab 不打开（pane 显示引导页），程序照常启动；不能用空内容打开，否则自动保存会把那个读不出的文件覆盖掉。`:q` 关闭 tab 不弹确认，内容已经写盘。`:wq` 是先 `:w` 再 `:q`：console 上等同 `:q`；表 tab 上先保存修改（§10.3），保存成功后才关闭，失败时 tab 留着、查询条显示原因，不弹丢弃确认。console 文件写不进去时，`:q`、关 pane、退出都不执行，toast「保存失败：<err>」。

**执行（C-02）**：

- 在 NORMAL 或 VISUAL 下按 `↵`、点击 gutter 的 ▶、点击标题栏的 `▶ run` 都可以执行。有选区时执行选区（块选区执行它覆盖到的那几整行，同 V-LINE），执行后退出 VISUAL、光标不动，同 vim 的操作符；要再跑一次用 `gv` 重新选中，或点结果区的重跑。否则执行光标所在的语句。点 gutter 的 ▶ 执行那一行开始的语句，不移动光标。
- 每次最多取 `console.max_rows` 行（默认 1000），读语句用自动 LIMIT（§9.4），其余靠 `Exec` 的 `maxRows` 截断；截断时显示 `1000+ 行`。
- 在所选连接的 `Main` 的一次 `Worker.Run` 里逐条执行，每条是单独的简单协议请求：自动事务模式下各自自动提交；手动模式下（F5.3）每条语句前 `Main` 不在事务里就先 BEGIN（照 psql 在 AUTOCOMMIT off 时的 SendQuery：一次执行里 commit 之后的语句也在新事务里），psql `command_no_begin` 列出的语句不补 BEGIN（事务控制、VACUUM、CREATE / DROP DATABASE、带 CONCURRENTLY 的、ALTER SYSTEM、DISCARD ALL 等，按 psql 源码定），出错后事务停在出错状态，由用户回滚。console 有执行在跑时不能关（关 tab、pane、window），toast「正在执行，结束后再关闭」，同换连接的规则：否则执行回来后连接停在事务里，上面却没有 console 了。遇到错误就停，后面的不执行。不一次发出整段：PG 简单协议里一次发送的多条语句属于同一个隐式事务，一条出错全部回滚，也没法知道错在哪一条、把哪个 ▶ 标红。
- 首个关键字是 create / alter / drop / rename（MySQL 的 `RENAME TABLE`，F5.4；PG 的改名是 `ALTER … RENAME`）的语句成功后，重新加载这个 console 所在连接的 catalog、清空列缓存（同 `tree.refresh`）。
- 用户在自动模式下自己 `begin`：`Main` 进了事务，这个连接上表格的保存也进这个事务、不自动提交（§10.3）；状态栏显示「事务中」（§7.8），提交、回滚同手动模式（M5 F5.3 起，原来是已知上限）。

**结果区（C-04 至 C-07，已确认）**

参考 DataGrip、DBeaver 的做法：所有结果集中显示在底部一个全宽的区域里，内部用 tab 区分。这样做有两个原因：

- 表格最需要的是宽度。PRD 原方案是在 console 旁边分割，结果只能占到屏幕的一半，向右分时甚至只有四分之一。
- 结果多了只是 tab 变多，pane 数量不变，布局不会越用越乱，这正是 C-05 想要解决的问题。

```
┌⟨0⟩ schema─┐┌⟨1⟩ data · t_order ─────────┐┌⟨2⟩ console · console_1 ─── ▶ run ↵┐
│ t_order   ││ WHERE deleted_at is null    ││ select * from mt_task              │
│ t_user    ││ …                           ││ where status = 'running';          │
│ …         │└─────────────────────────────┘└────────────────────────────────────┘
│           │┌⟨3⟩ result · console_1 #42 ─── 3 rows · 8ms  [重跑][转置][固定][导出][×]┐
│           ││ id    biz_type   status    created_at                                 │
│           ││ 689   goal       running   09-21 10:02                                │
│           ││ …                                                                     │
│           ││ 1:日志  2:console_1 #42*  3:console_1 #40（已固定）                    │
└───────────┘└───────────────────────────────────────────────────────────────────────┘
```

**出现与位置**：

- 某个 window 里第一次执行 SQL 时，在主区域（schema 树右侧）的底部新建一个全宽的 result pane，默认占 40% 的高度。
- 拖动边界或用 `SPC J` / `SPC K` 调整高度后，按 window 记住。
- 它是一个普通 pane，可以缩放（`SPC z`）、关闭（`SPC x`）；关闭后，下次执行时会重新出现。
- 每个 window 只有一个结果区，这个 window 里的所有 console 共用。

**tab 的组织**：

- **日志 tab**：第 1 个 tab 固定为「日志」，不能关闭。
  - 每执行一条语句就追加一行，内容是：时间、来源 console、语句的第一行、结果（行数、影响行数或错误）、耗时。
  - 执行出错时，自动切到日志 tab，显示完整的错误信息；同时，console 里出错语句的 ▶ 标记变成红色。
- **console 的结果 tab**：每个 console tab 在结果区里有自己的结果 tab，标题形如 `console_1 #42`。
  - 同一个 console 再次执行时，替换它原有的结果 tab，序号加一。C-05 的复用从 pane 级别改到了 tab 级别。
  - 一次执行多条语句时，每个结果集各占一个 tab（`#42`、`#42·2`……），下次执行时整组替换。
  - 非查询语句（如 `UPDATE 3`）只写进日志，不单独开 tab。一次执行完全没有结果集时，保留上一次的结果 tab，结果区切到日志。
- **执行中**：结果 tab 先显示占位内容（执行中、已用时、`C-c` 取消），完成后再替换成结果。这一做法参考了 pgtui 的 `result_tabs`。
- **重跑**：用结果 tab 记下的连接和 schema 执行（M5 F5.11），来源 console 已经关掉或换了连接都照样；结果进它原来那一组；这时可能出现两组同名的结果（比如新开的 console_1 也有一组），靠序号区分。
- **出错标记**：同一个 console 每次执行开始时先清掉旧的红 ▶，这次有出错的再标上；缓冲区一有改动也清掉。
- **固定与关闭**：
  - `P` 固定当前结果 tab：它不会再被替换，下次执行另开新 tab（C-06）。固定的 tab 画 `pin` 图标，不写「（已固定）」（有图标就不配文字）。`R` 重跑（`result.rerun`）。
  - `q` 关闭当前结果 tab。
  - 快速 SQL 里按 `C-t`，结果会作为一个已固定的 tab 放进结果区。
- **切换**：`gt` / `gT`，或者直接点击 tab（T-02）。
- **上限**（M5 F5.18，用户验收时要求）：日志 tab 之外最多 10 个结果 tab（160 列宽下大约放得下这么多）。新出一个 tab 超过上限时，关掉最久没更新过的那个（按最后一次执行或重跑完成、结果到达的时间，固定的也算），不问；正在执行的不关。一次执行多个结果集时，这一组本身超过上限的，只留最后 10 个。

**焦点**：执行之后，焦点留在 console，方便接着写；结果区只切换到对应的 tab。按 `C-j` 进入结果区。

**工具行**：

- pane 标题的右侧显示来源与序号、行数、耗时（C-07），以及可以点击的按钮：重跑、转置、固定、导出 CSV、关闭 tab，画成 ` <图标> `。放不下时依次舍去，保留的优先级：关闭 > 重跑 > 固定 > 转置 > 导出 > 统计文字。严格按优先级从低往高丢，低优先级的项不会因为更窄就留下来（与 §7.8 pane 标题的贪心摆放不同：ascii 下 `[T]` 比导出的 `>` 宽，贪心会先丢转置、留下导出）。
- 导出 CSV 写文件：当前目录下的 `console_1-42.csv`，完成后 toast 显示路径。不走剪贴板：OSC 52 能复制的大小有限，1000 行可能超过终端的上限。
- 表格的交互与 data pane 相同（G-01、G-05、G-06），但只读。

**不做 `result_split = console`**（M3 定）：每个 console 旁边各有一个结果 pane，与「每个 window 一个结果区」是两套布局规则，工作量不小，也没人要过。有需要时再加。

## 12. 命令面板与快速 SQL（K、F）

命令面板是执行命令的主要入口，取代了 M0 最初的 `:` 命令行（M0 用户反馈）。M0 先做面板本身，包括命令、表、pane、window 四个范围（F0.13、F0.14）；表在 M0 里用侧栏的假数据，M1 换成 catalog。session、SQL 范围和 DDL 预览在后面的里程碑。

- **打开**：`C-p`（`palette.open`）打开，范围为「所有」；表格、树、引导页里的 `:` 也绑 `palette.open`（F5.8 起；原来绑 `palette.command`，打开并直接进入命令范围，相当于输入了 `>`；它保留、不带默认键）。
- **ex 别名**：命令可以带别名，如 `q`（关闭 tab）、`qa`（退出）、`w`（保存）。在命令范围和「所有」范围里，输入与某个别名完全相同时，这条命令排第一，所以 `:q↵`、`:qa↵` 的用法不变（F5.8 起 `:` 打开「所有」）。
- **数据来源**：session、window、pane 取自工作现场；表取自 catalog；命令取自 Action 注册表（带标题的 Action）；SQL 取自快速查询的历史。
  - 只在浮层或输入状态里用的 Action 不作为候选：离开那个上下文，执行它们没有意义，标题单独看也看不懂。包括：
    - 浮层作用域里的，比如面板自己的 `palette.up` / `palette.down` / `palette.run` / `palette.close`，以及以后的 `where.*`、`cols.*`；
    - `cell`、`input` 作用域里的，比如 `cell.accept`「确定」、`cell.up`「加一」。
  - 判断方法是「只在这些作用域里有绑定」。
- **范围与前缀**：按 PRD K-02，范围标签为 所有 / 会话 / 窗口·Pane / 窗口 / Tab / 表 / 命令 / SQL，M0 先有 所有、窗口·Pane、表、命令，M5 F5.1 加上会话，F5.19 加上窗口和 Tab。`Tab` / `S-Tab` 切换范围标签；输入前缀直接限定范围：`>` 命令、`@` 表、`%` 窗口·Pane、`&` 窗口、`#` Tab、`$` 会话、`;` SQL。
  - **窗口 `&`** 和 **Tab `#`**（M5 F5.19，用户要照 nvim 的习惯用 `SPC j` 切 window、`SPC k` 在最近用过的 tab 之间切）：「窗口」只列当前 session 的 window，按编号排；「Tab」列当前 session 所有 window 里打开着的表、console、结果 tab，按最近聚焦的时间从新到旧，当前 tab 排最后，这样打开就 `↵` 等于回到上一个 tab（同 telescope `buffers` 的 `sort_mru` + `ignore_current_buffer`）。tab 的「最近聚焦」是它最后一次成为焦点 pane 的当前 tab 的时刻，记在 session 上、不持久化。`palette.window`「命令面板：窗口」、`palette.tab`「命令面板：Tab」分别预填 `&`、`#`，同 `palette.command`，都不带默认键。「窗口·Pane」照旧，三者混在一起、按编号。前缀不用 `^`、`!`：fzf 扩展语法里它们是前缀锚点和排除。
  - 范围只由输入里的前缀决定，不另存状态：`Tab` / `S-Tab` 就是改写前缀（「命令」→ `>`、「表」→ `@`、「窗口·Pane」→ `%`，「所有」去掉前缀），输入的其余内容保留；范围标签按当前前缀高亮。`palette.command` 相当于输入了 `>`，也是同一套机制；F5.8 起 `:` 默认绑 `palette.open`，打开「所有」。
- **每一行**（K-03）：图标、名称、所在位置（`dim` 色）、右侧的键位或 ON / OFF、类型标签（命令 / 表 / Pane / 窗口）。图标都放在最左列对齐。
  - pane 的名称是 `⟨1⟩ data · t_order`，所在位置是它所在的 window，如 `0: data`（M4 F4.3 起写全层级 `play › 0: data`）；
  - window 的名称是 `0: data`，所在位置是 session 名。
  - session（M5 F5.1）：名称是 session 名，所在位置是 window 数，类型标签「会话」。window、pane、tab 只列当前 session 的。
  - tab（M4 F4.3 起）：每个打开着的表、console、结果 tab 都是一项，名称是 tab 名，所在位置是完整层级 `play › 0: data › pane-1`，和树的工作区一致，`↵` 切过去。
  - 命令的名称是中文标题，所在位置显示它的 action id，如 `左右分割  pane.split.right`。匹配的对象是「名称 + 空格 + 所在位置」这一整串，两部分都能高亮，所以输入 `split` 也能找到；action id 也正是在 config.toml 里绑键时要写的名字。
  - 表的所在位置是 `<连接>.<schema>`，如 `play.public`（M5 起；原来是 `session.schema`）。
- **布局**（M0 用户体验后调整，参照设计稿）：宽度 min(100, 窗口宽 − 4)，水平居中；上边缘固定在状态栏以上区域高度的 1/6 处，即 (H−1)/6，列表变长变短时输入行不动。比最初的 1/4 更靠上，是为了给 SQL 范围的结果留出下半部分。单线边框、`focus` 色，标题「命令面板」。从上到下：
  - 范围标签，每个标签带上它的前缀，如 `窗口·Pane %`、`表 @`、`命令 >`、`SQL ;`，「所有」没有前缀；
  - 输入行：前面是搜索图标，`>` 等前缀照常显示在输入里；
  - 分隔线、列表（最多 12 行，放不下时滚动）、分隔线；
  - SQL 范围执行之后，列表下方是结果区（见下文「快速 SQL」）；
  - 底栏：左边是移动和关闭的键位提示（从 keymap 读取），右边是 `↵ <按回车的效果>`。
- **列对齐**：每一行的「所在位置」都从同一列开始。名称列的宽度取当前候选里最宽的名称，上限为面板宽度的 40%，超过上限的名称截短，末尾加 `…`。键位和类型标签照旧靠右。没有「所在位置」列的列表（SQL 历史）不受 40% 限制，名称占满整行，放不下再截短。
- **光标**：用终端自己的光标，放在输入位置上，输入法的候选框也会跟着它。
- **开关类命令**（M0）：`tree.toggle`（侧栏展开时为 ON）、`pane.zoom`（缩放中为 ON）、`result.toggle`（结果区显示着为 ON，F3.28）。
- **匹配**：使用 fzf 的算法，并支持它的扩展语法（§9.7），匹配到的字符用 `match` 色的前景标出，不加底色（§7.3）。
  - 输入为空时，先列最近用过的（不分种类）；其余按范围标签的顺序：window、pane（按 ⟨n⟩）→ 表 → 命令（按 action id）。
  - 表范围列出所有已连上的连接里所有非系统 schema 的表（M5 F5.10 起；没连上的连接没有 catalog，不列），所在位置显示 `<连接>.<schema>`，这样树停在 `public` 时也能搜到 `agentable.agent`。顺序：当前连接的 Schema 的表在前（按树的顺序），然后是当前连接的其余 schema，再是其他连接，各按 schema 名、表名排。打开的表属于它所在的连接。
  - 输入不为空时，各种类混在一起按 fzf 的分数排；分数相同时按输入为空时的顺序排，不按长度（M5 F5.4 测试时定：同名的表在几个 schema / 库里同分，原来较短的排前面，`↵` 会打开、改到别的库里的表；表的顺序见下一条，当前连接的 Schema 排最前）。
  - 最近使用 M0 只记在内存里，M1 有了 state.json 之后持久化；M5 起表的 id 带连接名（§14）。
- **执行**：按 K-04。底栏右侧显示当前项按回车会做什么。
  - 命令：执行；开关类命令只切换状态，不关闭面板。
  - 表：打开到焦点所在的普通 pane（表格和 console 所在的 pane 都算，F3.37 起不再跳过 console 的 pane）；焦点不在普通 pane 上时（比如在树上），用这个 window 里最近聚焦过的普通 pane，与按方向切焦点选最近用过的 pane 同理（§5）；从没聚焦过 data pane 时用第一个；一个都没有时什么都不做。M1 F1.12 起如此：原来固定用第一个 data pane，从 ② 按 `C-h` 回到树再打开表，表会开到 ①，对比时很别扭。目标 pane 里已经开着这张表时，按 §7.8「打开已有的表」切过去。打开后焦点移到那个 data pane，因为打开表就是为了接着看数据。`↵`：目标 pane 里这张表已经开着就切过去，没开就在那个 pane 新开一个 tab，不替换当前 tab；`C-t` 同样（F3.37 起同一个 pane 里一张表只开一个）；底栏显示 `↵ 打开`（F3.37 起 `C-t` 和 `↵` 打开表的效果相同，不再单列）。选中的不是表时，`C-t` 不起作用。
  - pane：聚焦。
  - window：切换（M5 F5.2；之前只列出，`↵` 只关闭面板）。
  - tab：切过去；在别的 window 里时先切 window，再选这个 tab、聚焦它的 pane（F5.2 补上 F4.3 留下的这一半）。
  - session：attach（F5.1）。
- **预览（K-05）**：光标在某张表上停留 150ms 后，从这张表所在连接的 `Meta` 获取 DDL，获取后缓存。窗口高度不够时先压缩预览区：列表和预览各自最多 12 行，放不下时先把预览压到 3 行，还放不下再把列表压到 3 行，还放不下就不显示预览；预览被压到不足 3 行时不显示（DDL 本身只有 1–2 行的照它自己的行数显示），底部提示始终显示（M4 F4.2 审查时定：80×24 下原来的「预览只拿剩下的」让「所有」范围一直看不到预览，或只剩一个 `…`）。
- **快速 SQL**（`;` 前缀或「SQL」标签）：NORMAL 下按 `;` 直接打开面板的 SQL 范围，输入框预填 `;`，与 `palette.command` 预填 `>` 同理（M1 用户反馈）。核心部分在 M1 F1.7 实现，用户体验 M0 时要求提前；依赖 console 的部分随 M3 补上。
  - **结果区**：执行后，结果显示在面板的下半部分，用和 table pane 相同的表格组件和网格样式（§7.6）；结果区的标题行显示行数、耗时和「只读」。SQL 范围下面板向下扩展，结果区至少 8 行。
  - **补全**：M1 用 catalog 里的表名、列名和 SQL 关键字做模糊补全，复用 F1.5 的补全列表；能看懂别名、CTE、子查询的补全要等 M3 F3.7。关键字取 lazysql `builtinKeywords` 里常用的约 40 个（小写）；表名取当前连接的 Schema 的；列名按 §9.7「第一次用到时才获取」：输入里出现和某张表同名的标识符时，取那张表的列并缓存（与打开表共用列缓存），不预拉全库的列。顺序为列 → 表 → 关键字，组内按 fzf 分数；右侧注释是列的类型和所属表、「表」、「关键字」。补全状态从 data tab 挪出来，面板和 WHERE 共用。
  - **执行**：在当前连接（§5）的 `Meta` 上执行，PG 用 `BEGIN READ ONLY`，MySQL 用 `START TRANSACTION READ ONLY`，执行完一律 ROLLBACK。MySQL（F5.4）没有下面游标的做法：判为读的加 §9.4 的自动 LIMIT（101 行），已经带 LIMIT 或包在括号里的照样算完（ponytail）；database 用 `USE` 设，代替 search_path。同一事务里先 `SET LOCAL search_path TO <当前连接的 Schema>, <建连时的原始 search_path>`（§8.6 的写法），这样树停在 `agentable` 时 `;select * from agent` 也能找到表，ROLLBACK 后自动恢复。最多显示 100 行，更多时显示 `100+`。
    - **不让服务端算完整个结果集**（M1 F1.7 定，参考 PG16 psql 的 `FETCH_COUNT`：common.c 的 `is_select_command` 跳过空白、注释和左括号后只认 `select` / `values`；PG17 起 psql 改用 chunked rows mode，删掉了这个函数）：跳过开头的空白、注释和左括号后，第一个词是 `select` / `values` / `table` / `with` 的（`table` 和 `with` 是这里另加的，大表上的 WITH … SELECT 很常见，限行更要紧），用 `DECLARE <游标> NO SCROLL CURSOR FOR <语句>` + `FETCH FORWARD 101`，服务端只算到第 101 行；其余语句（SHOW、EXPLAIN、写语句等）直接执行，写语句由只读事务拒绝，显示数据库原文。两条路径都走扩展协议，所以一次只能执行一条语句。
    - 否掉的做法：lazysql 读到上限后停，但关 rows 时 pgx 会把剩下的读完，服务端照样算完；usql 不限制；扩展协议 Execute 带行数上限（pgjdbc 的做法）最通用，但 pgconn 没有暴露，要绕过 pgconn 自己收发协议消息、自己处理取消，与 §8.1「取消走 ctx」冲突。
    - 已知上限：`explain analyze` 这类非 SELECT 语句仍会算完；`with … delete` 这类 WITH 后面接写语句的，走 DECLARE 报的是 `syntax error at or near "delete"`，而不是只读事务的错误，M3 由读写判定（§9.3）解决；首词判断是临时的，代码里用 `ponytail:` 标出。M4 起改为：先用 §9.3 的读写判定，判为写的不执行（见下面「写语句」）；判为读、且首词是 select / with / table / values 的仍走游标 + `FETCH FORWARD 101`，不改用 §9.4 的自动 LIMIT：游标让服务端只算到第 101 行，而自动 LIMIT 对已经带 LIMIT、或者包在括号里的查询不起作用，碰到这些照样算完（M4 起草时定）。
    - 执行中 `C-c` 取消查询、面板不关，取消后保留上次结果，toast「查询已取消」（§8.3）；空闲时 `C-c` 照旧等同 esc。执行中再按 `↵` 忽略。
  - **面板布局**（SQL 范围）：输入为空时列表区列历史（新的在前，每行只有 SQL 文本，不带类型标签，因为整个列表都是同一种），`C-n` / `C-p` 选，`↵` 把选中的填进输入并执行；输入不为空时列表区为 0 行。结果区执行过才出现，出现后面板向下扩展到状态栏上方、留 1 行空隙；窗口太矮放不下 8 行时，按能放下的显示。关掉面板结果就丢掉，历史里有。
  - **结果区**：标题行 `100+ 行 · 12ms · 只读`，右侧是可点击的 `C-y CSV`（键位从 keymap 读）；执行中行数处显示 `…`，保留上次结果；没有结果集的语句显示命令标签（M4 起 `SET` 这类判为写、被拦下，PG 上的读语句都有结果集，这一条只是兜底）。报错显示在结果区第一行（`error` 色），同 data pane。表格只显示、不带光标，滚轮纵向滚动、Shift + 滚轮横向，不加键盘滚动（焦点在输入框）。
  - **历史**：执行过的都记，不论成败；去重后挪到最前，最多 50 条，按连接存在 state.json。「所有」范围不列 SQL 历史，M1 只在 SQL 范围列。已修改的判断按全文比较（去掉 `;` 前缀）。
  - **`C-y`**：CSV 为表头加显示的行（最多 100 行），NULL 写空串（同 lazysql `helpers/csv.go`），写系统剪贴板走和 `"+` 寄存器同一处（§11「寄存器」：先 pbcopy 等本地工具，找不到才 OSC 52）；没有结果时不做事，复制后不加 toast。
  - **写语句（F-05）**：判为写的语句不执行（只拦首词是 SQL 关键字、且判为写的；首词不是关键字的，比如打错的 `selec 1`，照旧发给数据库，在只读事务里报语法错误，M4 全量 e2e 发现。关键字表要包含 PG 全部语句的首词，因为只读事务兜不住其中几条：reviewer 在 PG 17 上实测 `copy … to '<文件>'` / `to program`、`reindex`、`cluster`、`load`、`checkpoint` 在 `begin read only` 里都能执行），结果区显示 `warn` 色的「写语句不在这里执行 · C-e 在 console 中打开」，键位文字从 keymap 读。在 console 里由用户自己按下执行，这一步就是确认，不再需要 C-S-↵。M1–M3 还没接读写判定，写语句由只读事务拒绝，显示数据库返回的错误；M4 起改为这里的做法。
  - **`C-t` 送到结果区**（M4）：把结果作为固定的结果 tab 放进结果区，tab 名 `quick #n`（n 取 session 的执行序号），日志记一行 `quick  <首行>  N 行`；在这个 tab 上重跑（`R`），在 tab 记下的连接的 `Meta` 的只读事务里重新执行，search_path 用执行时记下的 schema（M5 起；原来用树当前的），结果原地替换、`#n` 和导出名不变（不同于 console 的固定 tab 另开新 tab：quick 没有要续的那一组）；导出文件名 `quick-42.csv`；面板不关。
  - **`C-e` 在 console 中打开**（M4）：在打开表的目标 pane（§12 的规则）里按 `console.new` 的规则新开 console（编号照 §11，当前是引导 tab 就原地替换）；新 console 一定是空的（F8.21 起编号不复用关掉的文件），SQL 就是全部内容，光标落在第一行；关掉面板、聚焦这个 console。新 console 的连接和 schema 取当前连接和它的 Schema，与快速 SQL 执行时用的一致。
  - **错误**：语法错误显示红色提示。
  - **已修改提示（F-03）**：当前输入与上次执行的语句不同时，提示「已修改，↵ 重新执行」。
  - **后续操作（F-04）**：`C-t` 把结果送到一个新的、已固定的 result pane；`C-y` 用 `encoding/csv` 生成 CSV，再写进系统剪贴板（同 §11「寄存器」）；`C-e` 在 console 中打开。结果区标题栏上的按钮都可以点击。M1 先做 `C-y`，另外两个要等 M3 有了 console 和结果区。
  - **历史**：保留最近 50 条，去重。

## 13. 安全

- **危险语句先确认**（M6 F6.7，用户 2026-10-05 要求：没带条件的 delete / update、删表这类操作一定要有闸门）。参照 DataGrip 执行前的「DELETE 语句没有 WHERE」提示和 MySQL 的 `sql_safe_updates`。
  - **哪些算危险**（只做词法判断，`sqlkit`，和 §9.3 的读写判定建在同一个扫描器上）：没有 `WHERE` 的 `DELETE` 和 `UPDATE`：主语句的看顶层（子查询、CTE 里的 `where` 不算，`delete from t using …` 照样看顶层有没有 where；MySQL 的 `delete … limit` 没有 where 也算）；括号里的也判，紧跟在 `(` 后面的 DELETE / UPDATE 各按自己那一层有没有 WHERE 算一次（PG 的 `with d as (delete from t returning *) …`、`copy (delete from t returning *) to stdout` 实测照样删光整表，F6.7 审查时定）；`TRUNCATE`；`DROP`（表、库、schema、索引、视图都算，`drop … if exists`、MySQL 的 `drop temporary table` 也算，不细分）。`ALTER TABLE` 里的 `DROP` 按位置和方言判（审查时在 PG 17 / MySQL 8.4 实测后定）：紧跟在 `ALTER [COLUMN] <列名>` 后面的 DROP（default / not null / expression / identity）是改列属性，不算；子句开头的 DROP，PG 只有 `drop constraint` 不算，MySQL 的 `drop constraint / index / key / primary / foreign / check` 不算（这几个在 MySQL 是保留字，在 PG 里 `drop key` 删的是叫 key 的列）；其余一律按删列算。MySQL 的 `alter table … drop partition` 和 `truncate partition` 会删数据，算。`EXPLAIN ANALYZE` 后面跟着危险语句的，PG 上算（它真的会执行），MySQL 上不算（实测只给计划、不执行）。认不出的：`merge … when matched then delete`、`prepare` 之后的 `execute`，和存储过程一样属于已知上限。主语句词的取法：开头是 `with` 时取第一个括号深度 0 的语句词（CTE 体都在括号里），同 §9.3。只读连接上这些本来就执行不了，不会走到确认。
  - **什么时候问**：console 执行（`↵` / `C-e` / `▶ run`，含选区执行）、结果区的重跑 `R`，在真正发出前先扫这次要执行的全部语句；有危险语句就弹确认框（§10.5 的样式）：标题「这次执行有 N 条危险语句」，每条把连续空白（含换行、tab）压成一个空格后截到 60 列（多行语句只列第一行会只剩 `with old as (`），按钮「执行」/「取消」，`y` / `↵` 执行、`n` / `esc` 取消整次（不是跳过那几条）。一次执行只问一次。选区执行被取消时选区不恢复（文字和光标不变，已退出 VISUAL，和 vim 执行命令后一样）。手动事务模式下也问：MySQL 的 DDL 会隐式提交，回滚救不回来。快速 SQL 在只读事务里跑，写语句本来就被拦下（§12），不用问。
  - **表格的保存**：保存里含删除行时也先确认：「将删除 N 行（另有 M 处修改），确定保存？」（M 为 0 时不带括号），`y` / `↵` 保存、`n` / `esc` 回到表格、标记都还在。只改格子、新增行的保存不问：每格都带旧值检查，影响的只是看得见的那几格。
  - **配置**：`config.toml` 的 `confirm_dangerous = true`，默认开；关掉后上面两种确认都不弹。不按语句类型细分开关。
  - 这是防误操作的闸门，不是权限边界：拼接在存储过程、函数里的写操作认不出来，真正要限制权限用只读账号或只读连接。
- **只读连接（S-04；M5 起按连接，connections.toml 的 `read_only = true`）**：
  - **应用层**：
    - console 执行前检查这一次的全部语句：判为写的、会解除只读的，有一条就整次都不发，日志记原因并切到日志 tab；表格的编辑、新增、删除、粘贴都拦下（§10.1）。
    - SET 语句中，下面四类整类拦截（简单；里面有些写法实测并不解除只读，如 `SET TRANSACTION READ ONLY`、`SET TRANSACTION ISOLATION LEVEL …`，要设隔离级别用 `BEGIN ISOLATION LEVEL …` 或 `SET default_transaction_isolation`；作用域词连写几个也照样认出）：`SET TRANSACTION`、`SET SESSION CHARACTERISTICS`、`default_transaction_read_only`、`transaction_read_only`；还有带 `READ WRITE` 的 `BEGIN` / `START TRANSACTION`（PG 里它盖过 `default_transaction_read_only`）。最终清单以在 PG 上逐条实测的集成测试为准（F5.3）。MySQL 的写法：`SET [GLOBAL | SESSION | PERSIST …] TRANSACTION`、`@@[scope.]transaction_read_only`、`START TRANSACTION … READ WRITE`，同样在 MySQL 上逐条实测（F5.4）。MySQL 上另外：`reset` 是管理命令，算写；SET 里带 `GLOBAL` / `PERSIST` / `PERSIST_ONLY` 作用域的项和 `@@global.x` 都拦（改的是整个服务端，同 PG 的 ALTER SYSTEM）；MySQL 8.4 的语句首词（kill、flush、purge、install / uninstall、shutdown、restart、clone、xa、handler、stop、optimize、repair 等）补进关键字表，照 §9.3 第 7 条算写。已知上限：`SET sql_mode = 'NO_BACKSLASH_ESCAPES'` 之后服务端的词法和我们的扫描器不一致，精心构造的 SET 能解开只读，同 PG 的 `U&`。
    - 其他不影响只读的 SET（如 `search_path`、`statement_timeout`）放行；事务控制（begin / start / commit / end / rollback / abort / savepoint / release）和 reset / discard 也放行（F5.3 在 PG 上实测：`RESET`、`RESET ALL`、`DISCARD ALL`、`SET … TO DEFAULT` 之后仍是只读，`SET SESSION AUTHORIZATION DEFAULT` 解除不了）。其余首词是关键字的照 §9.3 第 7 条算写、拦下（vacuum、call、do、copy、prepare、declare、notify 等）；首词不是关键字的照旧发给数据库，由它报错。
    - 拦下时整次都不执行，不算一次执行（不加执行序号、不出占位），日志记一行 `<连接> 是只读连接，整次都没有执行`（`error` 色）并切到日志 tab，不弹 toast、▶ 不变红、不出错误栏；结果区的重跑也检查。
  - **数据库层**：`Meta`、`Main` 两条都设只读：PG 在 RuntimeParams 里设 `default_transaction_read_only = on`（同 §8.2 的 `Meta`，`RESET` / `DISCARD ALL` 之后仍是 on），MySQL 执行 `SET SESSION TRANSACTION READ ONLY`，兜住判定漏掉的情况（例如会写数据的函数）。已知上限：`select set_config('default_transaction_read_only', 'off', false)` 判为读，两层都挡不住，代码里用 `ponytail:` 标出；`set U&"default_transaction_read_only" = off` 这类 Unicode 转义的标识符，判定也认不出，同样是已知上限。
  - 只读是为了防止误操作，不是权限边界。真正需要限制权限时，应使用只读的数据库账号。这一点要写进用户文档。
- **快速 SQL**：只读由数据库事务保证，即使一条会写数据的 select 被判为读，也写不进去。但只读事务挡不住 `copy … to` 文件 / program、`reindex`、`cluster`、`load`、`checkpoint` 这类语句（M4 审查时实测），所以这些首词在应用层判为写、拦下（§12「写语句」）。
- **凭据**：
  - `connections.toml` 支持 `password_cmd`（例如 macOS 的 `security` 命令、`pass`）、`password_env`；PG 还会被 pgconn 自动读取 `~/.pgpass`。
  - 也允许直接写明文 `password`，以及 DSN 里带密码（`postgres://u:p@…`）。**明文是用户自己负责的易用性兜底**（用户 2026-10-07 定：推广时易用性比安全性重要）：程序不检查权限、不改用户手写文件的权限、不弹警告（M7 F7.5 起；原来文件对其他用户可读时每次启动 toast「建议 chmod 600」，去掉）。程序自己新建 connections.toml 时仍用 0600，那是看不见的；表单保存的密码默认进钥匙串，所以不手写的用户拿到的都是加密的。README 的配置一节写明：明文能用，安全由用户自己负责，想安全用表单、`password_cmd` 或环境变量。
  - 密码来源的优先级：`password_cmd` > `password_env` > `password` > DSN 里写的 / `~/.pgpass`（后两者交给 pgconn）。写了多个只取优先级最高的，不报错。
  - **环境变量展开**（M7 F7.3，用户 2026-10-05 问「要不要支持环境变量配连接」）：`dsn` 和 `password` 里的 `${VAR}` 在建连前（`dial` 里取密码之前）用正则 `\$\{([^}]+)\}` 加 `os.LookupEnv` 展开（只认 `${VAR}` 这一种写法，`$VAR` 不展开，免得 DSN 里的 `$` 误伤；不用 `os.Expand`，它连 `$VAR` 一起展开）；变量没设置时报错「`${PGURL}`：这个环境变量没有设置」，和 `password_env` 一致。这样 `dsn = "${DATABASE_URL}"` 就能整条走环境变量（同 dbmate、Atlas 的 `env("DATABASE_URL")`、sqlc 的 `${}`），`password_env` 保留、不删（已经有人用）。变量名由每条连接自己写，多个连接各用各的（`${PROD_URL}`、`${DEV_PW}`），也可以只把密码写成变量、host 等照常写在 dsn 里；没有「按连接名自动找 `SQLMUX_<NAME>_PASSWORD`」这类约定，写明的比猜的清楚。
  - **表单里的密码**（M7 F7.3）：新建连接表单加一个 `password` 字段，输入时画成 `*`（同 DataGrip 和所有登录框），`dsn` 字段照旧明文（它本来就不该含密码）。保存时密码不写进 connections.toml：macOS 上用 `security add-generic-password -U -a sqlmux -s sqlmux-<name> -w <密码>` 存进钥匙串，文件里写 `password_cmd = "security find-generic-password -a sqlmux -s sqlmux-<name> -w"`；密码里有 0x20–0x7e 以外的字节（中文、`ä`、tab）时，`security -w` 会把整串按十六进制输出（SecurityTool 的 keychain_find.c），所以这时 password_cmd 末尾接 `| xxd -r -p`（macOS 自带），读回原值（F7.3 审查时发现）；Linux 上有 `secret-tool` 就用它（`secret-tool store --label sqlmux-<name> sqlmux <name>` / `secret-tool lookup sqlmux <name>`）；两者都没有时写明文 `password`，不改已有文件的权限（F7.5 起；新建的文件照旧 0600），toast「没有钥匙串工具，密码以明文存进 connections.toml」。不引入 keyring 库：`password_cmd` 已经是通用机制，钥匙串只是替用户把命令写好（pgtui 用 99designs/keyring 直接读写钥匙串）。平台覆盖：macOS 钥匙串；Linux 桌面（GNOME Keyring、KDE Wallet 都走 libsecret 的 `secret-tool`）；没有桌面的 Linux 服务器没有钥匙串，走明文兜底，或者用户自己写 `password_cmd`（`pass`、1Password 的 `op`、Bitwarden 的 `bw` 都行）；Windows 本来就不在支持范围（§14 只按 Unix 的 XDG 路径，e2e 靠 tmux），要支持时用 Credential Manager 或换 keyring 库，到时再定。
  - **界面上不显示密码**：状态栏地址块、树、面板、日志里都只有用户名和 host，DSN 的密码照 §13「日志」脱敏；表单的 `password` 字段是唯一出现密码的地方，画 `*`。
  - `password_cmd` 用 `sh -c` 执行，去掉末尾的换行；结果在进程内按连接名缓存（一次运行里 connections.toml 只读一次，同名就是同一条；测试里换密码来源时先清缓存），`Main`、`Meta` 和别的 session 建同一个连接时都用它（免得钥匙串一再弹窗），建连失败时丢掉缓存，下次重新执行。非 0 退出时报错，错误里带上它的 stderr；`password_env` 指的变量没设置或为空时报错。这些错误对启动时的连接是打印后退出（§14），对其他连接是标成失败、进 toast（§7.8）。
- **注入**：值一律参数化，标识符一律按方言加引号。
- **日志与文件**：
  - 只有带 `--debug` 启动时才写日志，DSN 中的密码会被脱敏。
  - 历史记录里可能含有敏感的字面量，所以 state 文件和 console 文件的权限都设为 0600。

## 14. 配置与持久化

所有 Unix 平台都遵循 XDG 规范：优先读 `$XDG_CONFIG_HOME`、`$XDG_STATE_HOME`、`$XDG_DATA_HOME`，未设置时分别用 `~/.config`、`~/.local/state`、`~/.local/share`。macOS 上也用 `~/.config`，不用 `~/Library/Application Support`，这是终端工具的惯例。

**文件格式**：

- **配置文件用 TOML**。配置是用户手写的，需要能写注释；键位的分层表（如 `[keys.grid]`、`[map.console.normal]`）写成 TOML 的表最自然。lazysql、Helix、Alacritty 用的也是 TOML。
  - 不选 YAML：缩进容易写错，而且有隐式类型转换，比如 `no` 会被当成 false。
  - 不选 JSON：不能写注释。
- **state 文件用 JSON**。它只由程序读写，不需要用户手工编辑。

| 文件 | 写入方 | 内容 |
|---|---|---|
| `~/.config/sqlmux/config.toml` | 只有用户写 | 常规选项、主题 |
| `~/.config/sqlmux/keymaps.toml` | 只有用户写 | 键位 `[keys.*]`、leader、映射 `[map.*]`（M5 F5.9 起从 config.toml 挪出来；`sqlmux keys --format toml` 生成一份完整的参考：和默认相同的写成注释行，用户改过的写成生效的行，没有改动时全是注释）。config.toml 里还写着 `[keys]` / `[map]` 时启动报错，提示挪到 keymaps.toml |
| `~/.config/sqlmux/connections.toml` | 用户手写，也由应用追加（M5 F5.1 新建连接） | 连接定义 |
| `~/.config/sqlmux/themes/*.toml` | 只有用户写 | 自定义主题 |
| `~/.local/state/sqlmux/state.json` | 由应用写入 | 最近使用、每张表的 WHERE 历史与收藏、快速 SQL 历史、console 文件最后用的连接和 schema（M5） |
| `~/.local/share/sqlmux/consoles/` | 由应用写入 | console 的 SQL 文件 `console_<n>.sql`（M5 起不再按连接分目录） |
| `~/.config/sqlmux/layouts/*.toml` | 用户手写，或「保存布局为…」写入 | 命名布局（M6 F6.5，格式见下面「布局文件」） |
| `~/.local/state/sqlmux/last.toml` | 由应用写入 | 退出时的工作现场，`sqlmux` 不带参数时恢复（M6 F6.5） |

- 连接定义单独放一个文件，是因为应用改写 TOML 时会丢掉注释，所以不能去改用户手写的 config.toml。
- `name`、`engine`、`dsn` 必填，`name` 不能重复，`read_only` 可选（默认 false，M5 F5.3）。`name` 是 state.json 里的键（M5 起不再用作 console 文件的目录名，§11），规则照旧：不允许含 `/`、`\`，也不能以 `.` 开头，否则启动报错。`engine` 在 M1 只接受 `postgres`，其他值报错「目前只支持 postgres」。
- **启动时找不到连接**：`sqlmux <名字>` 找不到这个名字时，在终端打印错误就退出（退出码 1），不进入界面，错误里写明配置文件的路径并列出已有的连接名。**没有 `connections.toml` 或文件里没有连接时**（M7 F7.4，用户 2026-10-05 定；原来也是报错退出），照常进入界面：一个 session、默认布局、树是空的（只有「还没有连接，按 ＋ 新建」那一行），并直接打开新建连接表单；这时 `last.toml` 存在也当没有，直接进零连接状态，不报错、不改名；需要连接的命令（开表、新 console、快速 SQL、新建 session、切换连接、打开布局）在零连接时 toast「还没有连接，先新建一个」，一个都不能 panic（有测试把每条命令都跑一遍）；保存第一个连接后连上并展开（§7.8「新建连接表单」）。`esc` 关掉表单也能用：空的工作区可以看引导页、改配置，再从 `＋` 或面板新建。启动时的连接连不上（比如密码错误）也一样，打印驱动返回的错误后退出；pgconn 逐个地址尝试时会打出多行（标题一行，每个地址一行），照样输出，不压成一行。其他连接连不上不影响启动（§8.2）。
- **新建连接**（M5 F5.1，§7.8「新建连接表单」）：在 connections.toml 末尾追加一段 `[[connection]]` 文本，不重新编码整个文件，用户的注释都保留；文件不存在就新建，权限 0600。
- **修改和删除连接**（M8 F8.16，§7.8「连接管理」）：同样不重新编码整个文件。修改只改写这一个 `[[connection]]` 块里的 `key = value` 行（改值、在块末加新键、删掉清空的键），删除去掉这一块和它上面紧挨着的注释行；其余字节不动。块从表头到它最后一个键行（加后面的空行）为止，后面单独成段的注释不属于它；写完把这一块读回来和要写的连接比对，不一致就拒绝并提示手工改（多行值这类写法）。写盘走临时文件 + rename，保留原权限和符号链接。顺序：预检文件 → 存钥匙串 → 写文件 → 才删旧钥匙串项；改名时我们模板的 `password_cmd` 跟着搬到新名字。已连上的连接的 `read_only` 等下次连接再变。有 session 正在用的连接不能删。改了名字时 state.json 里按连接名记的键跟着改，布局文件里的 `connection = "旧名"` 不改（恢复时按「没有这个连接」跳过）；删除时只删我们自己模板的 `password_cmd` 对应的钥匙串项，用户自己写的命令不碰。
- **界面语言**（M6，用户要求）：`language = "en" | "zh"`，默认 `en`（M7 F7.6，用户 2026-10-07 定：开源面向英语用户；M6 时默认 `zh`），写错启动报错。
  - 界面上所有文案都走一处翻译：toast、提示、确认框、查询条与结果区的文字、状态栏的模式附加信息、面板的范围标签与底栏、Action 的标题、引导页、日志、错误说明（数据库返回的原文不翻译）。
  - 做法照 gettext：中文原文就是 key，`en` 是一张「中文 → 英文」的表，找不到就显示原文。这样现有代码只需要把字符串包一层，不用先给每条文案起名字。带参数的文案用占位符（如「已保存 %d 行」）。
  - 命令面板按当前语言的标题搜索，action id 照旧可以搜。
  - 键位文字不属于文案，照旧从 keymap 读（§6.7）。
  - 读到 config.toml 之前显示的文字一律是英文（F7.6）：`sqlmux` 命令行的用法说明、flag 说明和错误、解析 config.toml / keymaps.toml 的错误（语言就是正在读的内容之一），直接用英文写，不走 `T`；`i18n` 没 `Set` 之前就是英文，所以主题文件的错误照旧走 `T`，启动时读到的也是英文，运行中 `theme.switch` 读坏文件时跟着语言；config 读完之后的启动错误、`sqlmux keys` 的输出（含 `--format toml` 里的注释）跟着语言（F6.4 定）。
  - key 仍是中文原文（F7.6 时定）：改成英文 key 要动 300 多处文案且不改任何行为，不值；代价是读代码的人看到的是中文 key，`en` 表就是对照。
  - 测试：单测扫一遍所有包，找出没走翻译的中文字面量；`en` 表里缺的条目也由单测报出来。单测和 golden 照旧按 `zh` 断言（各包的 TestMain 里 `Set("zh")`），`en` 的几张 golden 照旧，默认是英文这件事由 i18n / config / cmd 的单测直接盖；e2e 的 `lib.sh` 在每个脚本的 config.toml 里补 `language = "zh"`（脚本自己写了就不补，同 `confirm_dangerous`），已有的中文断言不动，默认英文和 `language = "zh"` 各由一个脚本验证。
- **布局文件**（M6 F6.5，用户 2026-09-30 提出，细节 2026-10-05 按 worker 的提议定）：
  - **格式**：TOML，`version = 1`；每个 `[[session]]` 记 `name`、`connection`（启动连接）、`active`；每个 `[[session.window]]` 记 `name`、`layout`（分割树，TOML 表：`split = "h" | "v"`、`ratio`、`a`、`b`，叶子是 `pane = <id>`；程序写成子表 `[session.window.layout.a]`，手写成内联表也能读）、`focus`、`zoom`、`sidebar_open`、`sidebar_width`、`result_height`；每个 `[[session.window.pane]]` 记 `id` 和它的 tab：表 tab 是 `kind = "table"`、`connection`、`schema`、`table`、`where`、`order`、`desc`、`limit`、`page`、`cols`（隐藏的列）、`transpose`；console tab 是 `kind = "console"`、`n`、`connection`、`schema`；再记当前 tab。不存：结果集、正在跑的查询、未保存的单元格修改（退出前本来就要确认）、引导页。
  - **恢复**：第一个 session 的启动连接照现在连（连不上就是启动错误）；其余被 tab 用到的连接和其他 session 的启动连接都在启动时后台连（Cmd）；恢复出来的表 tab 先挂起不取数，等那个连接的 catalog 到了，表在 catalog 里的才取数、不在的关掉并 toast；连接失败就关掉它的全部挂起 tab，每个连接一条 toast（启动连接也走这一条）。console tab 先建在所在 session 的启动连接上，布局里的连接和 schema 作为「想要的」，连上后像标题下拉切换那样挪过去。catalog 里没有的表、读不了的 console 文件（读出错；文件不存在就当空 console 打开，没打过字的 console 本来就没有文件，§11）、不存在的连接：跳过这一项、toast 一条，其余照建。布局里的 console 编号已经被另一个正在跑的 session 用着时，换下一个空号，打开新编号自己的文件，不复制、不覆盖任何文件（原文件本来就在占着它的 session 里开着）。手写的布局缺字段时给缺省值：console 不写 `connection` 用这个 session 的启动连接，表不写 `schema` 用那个连接的 home schema，和 `sqlmux <连接名>` 启动时一致。同一个 pane 里同一张表写了两次只留第一个、不 toast（F3.37）；`[[pane]]` 里有、分割树里没有的 pane，它的 tab 一概不建。
  - **保存**：`last.toml` 只在正常退出时写（`:qa`、两次 `C-c`、关掉最后一个 session），不是每次改动都写；进程被杀掉就丢掉这次的布局改动（`ponytail:` 标出）。不管这次是怎么启动的，退出都覆盖 `last`，同 DataGrip。
  - **命令**：`layout.save`「保存布局为…」在状态栏的单行输入里要名字（同 `window.rename`，提示和输入框只占左边、右边照常画模式块，M8 F8.11 起三处统一；面板的输入行是搜索框兼快速 SQL，不再给它加一种模式），`↵` 写 `layouts/<名字>.toml`，同名覆盖，`esc` 放弃；名字含 `/`、以 `.` 开头、或是保留名 `last` 时 toast 拒绝（`last` 指 last.toml，F6.5 审查时定）。`layout.open`「打开布局」是下拉框（同 `theme.switch`），列出 `layouts/*.toml` 和 `last`，选中后替换正在跑的全部 session，有未保存的修改或没提交的事务先按 §10.5 确认（动词「打开」）；确认后在 Update 里同步读布局、建好 session，所有启动连接都在后台连，没有「拨号期间旧 session 还能操作」的窗口（审查时发现那时打的字会丢、还会把旧内容写进新 console 的文件）；第一个 session 的连接连不上也是 toast、丢掉它的 tab，和别的连接一样；`--layout` 和它共用读布局、建 session 的代码，但启动时连不上照上面「恢复」的规则，是启动错误、在终端里报出来退出，和 `sqlmux <连接名>` 一致（F6.5 复审时定）。
  - **启动**：`sqlmux` 不带参数时有 `last.toml` 就恢复，没有照默认布局；`sqlmux <连接名>` 用这个连接开默认布局、不恢复 `last`；`sqlmux --layout <名字或路径>` 打开这个布局；连接名和 `--layout` 同时给时报错退出「连接名和 --layout 只能给一个」。`last.toml` 读不了：toast、默认布局，坏文件改名为 `last.toml.broken`。
- **state.json**（M1 F1.5 起）：路径 `$XDG_STATE_HOME/sqlmux/state.json`，读写都在 `config` 包里。
  - 内容：`recent` 是面板的最近使用，记成 `{"kind": "table", "id": "play/public.t_order"}`，kind 为 window / pane / table / command，M5 起表的 id 带连接名，旧的不带连接名的条目读到时丢掉；`consoles` 记每个 console 文件最后用的连接和 schema，如 `{"console_1": {"conn": "play", "schema": "public"}}`，换连接或 schema 时写入（M5 F5.11）；`tables` 的键是 `"<连接名>/<schema>.<表>"`（不同连接可能有同名表），每张表下存 history 和 favorites。
  - 启动时读一次；每次改动（执行 WHERE、收藏、面板运行）都写一次。写入：在 Update 里序列化，在 Cmd 里写临时文件再 rename，权限 0600；用互斥锁和版本号，丢掉比已写入版本更旧的快照，避免两个 Cmd 并发时旧内容覆盖新内容。
  - 读取失败（文件损坏）：先把坏文件改名为 `state.json.broken`，再用 toast 报错，当作空状态继续，不退出。改名是为了下次写入不会把用户的历史直接覆盖掉。

```toml
# config.toml
theme        = "tokyonight-storm"
icons        = "nerd"            # nerd | ascii
vim          = true              # false 时 console / WHERE 是普通编辑器，表格、树多一层方向键（§6.9，M6）
language     = "en"              # en | zh，界面文案的语言（M6；默认 en，M7 F7.6）
timeoutlen   = 1000
result_height = 0.4              # 底部结果区默认所占的高度比例
formatprg    = ""                # 例如 "pg_format -"；为空时使用内置的 sql-formatter
keyword_case = "lower"
tab_width    = 2                 # console 缩进与格式化共用
autopairs    = true              # 输入括号、引号时补上另一半（§7.9）
confirm_dangerous = true         # 执行没带 WHERE 的 delete / update、drop、truncate，以及保存里有删除行时先确认（§13，M6）
border       = "single"          # single | rounded | double | bold | hidden，pane 和浮层的边框（M8 F8.10，取值同 lazygit；Unicode 没有粗圆角，所以粗和圆角二选一）

[tree]                           # M8 F8.22
width        = 32                # 侧栏宽度（列，含边框）

[grid]                           # M8 F8.5 / F8.8
full_width   = 22                # 期望宽度不超过它的列不收缩（22 放得下带整点时区的 timestamptz）
order        = "asc"             # asc | desc，新打开的表默认排序的方向
transpose    = false             # 新打开的表是否转置（关掉再重开的 tab 保留原来的状态，同 WHERE / ORDER / LIMIT）

[console]
max_rows = 1000
```

```toml
# keymaps.toml（M5 F5.9 起；sqlmux keys --format toml 生成完整版，默认键位是注释行，这里只列改动过的几行）
[keys]
leader = "<Space>"               # 改成 "<C-a>" 即为 tmux 式的全局前缀

[keys.normal]
"<C-w>v" = "pane.split.right"    # 想要 vim 风格的窗口键，可以自己添加
"<C-w>s" = "pane.split.below"

[keys.grid]
"x" = ""                         # 解绑

[map.normal]                     # 键 → 键序列，语义同 vim 的 noremap，所有 pane 通用
J = "5j"
K = "5k"

[map.console.normal]             # 只在 console
L = "5l"
H = "5h"

[map.grid.normal]                # 只在表格（data / result）
L = "5l"
H = "0"
```

```toml
# connections.toml
[[connection]]
name         = "doraemon"
engine       = "postgres"        # postgres | mysql
dsn          = "postgres://ctw@localhost:5432/doraemon?sslmode=disable"
password_cmd = "security find-generic-password -s sqlmux-doraemon -w"
read_only    = false
```

## 15. 测试

- **性能**（M8 F8.20）：`SQLMUX_PPROF=<addr>` 启动时开 `net/http/pprof`（如 `localhost:6060`），`just profile` 起一个连着 seed 库的实例并打印 `go tool pprof` 的用法；不开时零开销。可重复的测量放在 `internal/app` 的 benchmark 里：默认布局、160×45、t_order 一页 100 行，往 Update 里喂指针在表格上来回扫过的移动事件（每个事件一次 View），报告每事件的耗时和分配；目标是每事件 < 1ms（100 事件 / 秒不到一个核的 10%）。优化只按 profile 的热点做（AGENTS.md：不做没有测量依据的性能优化），做完的数字写在这里。
  - **优化前**（worker 2026-10-08，M3 Pro，`BenchmarkPointerMotion`）：1.33–1.41 ms / 事件，1.35 MB、7557 次分配 / 事件；真实终端每秒约 90 个移动事件时 sqlmux 占约 0.2 个核（2.3 ms CPU / 事件，含 GC 和渲染器 flush；macOS 的 `ps %cpu` 瞬时值不可信，以 pprof 为准）。热点：`Frame.Render` 每帧把 7200 个单元格全部重新序列化 44%（指针移动只变 2 行）；`Grid.Draw` 31%（其中每帧重算所有可见单元格的列宽 15%）；每帧新建整块 ScreenBuffer 约 0.6 MB，GC 占 12–17%。tea 的渲染器已按 60 fps 合并 flush、相同内容跳过，不是瓶颈。
  - **改法**（已定）：A 行缓存——渲染时和上一帧逐行比较单元格，没变的行复用上一帧的字符串；B 悬停没变就不重画——渲染时记下查过指针的矩形，移动事件到来时新旧坐标对每个矩形的包含关系都一样（且没有拖拽、没有列表的悬停选中）就沿用上一次的 View；C 列宽按（页内容版本、列集合）缓存，A、B 做完已经达标就不做。**优化后**（同一台机器、load 14–16、取最小值）：跨行 1.27 → **0.54 ms**，1.35 → 0.35 MB，7559 → 2090 allocs；行内 **1.2 µs**、516 B、6 allocs；真实终端每秒约 90 个事件时约 21% → 约 10.5% 核，剩下的大头是 tea 渲染器的 flush（35%）和写终端（15%），按 60 fps 合并，不归我们。做了 A、B 和 C 的一半（Grid.Draw 列宽只算一次），外加 ASCII 宽度的快路径；`Frame.Mouse` 改成只能经 `Over` 读，B 才算得准。

- **静态检查**（M7 F7.1，用户 2026-10-05 要求）：golangci-lint v2，规则在仓库根的 `.golangci.yml`，每个 commit 零报告，reviewer 审查前先跑。规则以 golangci-lint 的默认集（errcheck、govet、ineffassign、staticcheck、unused）为底，加上能抓到真 bug 和误用的：errorlint、nilerr、exhaustive（`default-signifies-exhaustive`，`keymap.Key` 不算枚举）、copyloopvar、intrange、unconvert、unparam、wastedassign、perfsprint、usestdlibvars、misspell、gocritic（diagnostic + performance 组，关掉 evalOrder、filepathJoin 两个误报项和 hugeParam、rangeValCopy）、revive（默认规则）、nolintlint（要写明 linter 和理由）、govet 的 nilness；staticcheck 去掉 QF*（按口味的重构建议）；formatters 用 gofmt、goimports（import 分标准库 / 第三方 / sqlmux 三组）；`build-tags: [integration]`，集成测试也检查。排除 preset `comments`（revive 要求每个导出名写「X is …」，和「注释写为什么」相反）和 `std-error-handling`。不开只为速度、没有测量依据的（prealloc、perfsprint 的 concat-loop、hugeParam、rangeValCopy，同 AGENTS.md「不做没有测量依据的性能优化」），不开和 gofmt 打架或只讲风格的（wsl、nlreturn、lll、godot、varnamelen、exhaustruct 等），也不开 depguard、gochecknoglobals 这类要为项目逐个配的。每个开启的 linter 在配置里一行注释写为什么；屏蔽用带理由的 `//nolint`。版本钉在 v2.14.0，本地和 CI 一致（reviewer 2026-10-05 起草，398899a 上 131 条报告：生产代码 52、测试 79）。
- **CI**（M7 F7.1）：GitHub Actions，push 和 PR 到 main 时跑 lint、vet、单测、集成测试（PG 17 和 MySQL 8.4 用 services 起，seed 同 docker-compose）；e2e 不进 CI（要 tmux 和 40 分钟以上，本地由 tester 跑）。
- **常规操作走 `just`**（M7 F7.2）：`justfile` 里有 build、run、test、test-integration、e2e（只编译一次）、lint、fmt、tidy、check（commit 前的全部检查）、golden 更新、db-up / db-down、keys 导出、clean；AGENTS.md 里写命令的地方都改成 just 的入口。

- **单元测试**（表驱动）：
  - `sqlkit`：扫描、分句、读写判定、自动 LIMIT、WHERE 拼接与多语句拦截，以及补全上下文判断（光标位置 → 候选类别、表的别名、CTE 和子查询的层级）。用例可以参考 lazysql 的 `sql_context_test.go`。
  - `keymap`：解析、trie 合并、次数前缀、映射展开（含按 pane 类型的优先级）、冲突检测、提示。
  - 模糊匹配：对 fzf 算法的封装，以及扩展语法的解析。
  - 布局树：分割、关闭、调整大小、按方向切焦点。
  - 保存时生成的 UPDATE 语句。
  - 时间分段的解析与调整。
- **编辑器与 nvim 的差分测试**：
  - `internal/editor/testdata/cases.txt` 中每条用例包含：初始文本、光标位置、按键序列。
  - `go generate` 调用 `nvim --headless --clean`，逐条用 `silent! call feedkeys(keys, 'xt')` 喂按键，记录结果的文本、光标位置、无名寄存器和 topline，作为 golden 文件提交。不用 `normal!`：一次 `:normal` 的全部按键只算一个撤销步，`xxu` 会把两个 `x` 一起撤掉，和手按不一致（M3 核对时在 nvim 0.12.4 上实测）；`t` 让按键按真实输入处理。
  - 生成器的选项：`--clean --cmd 'set noloadplugins'` 加 `expandtab tabstop=2 shiftwidth=2 nowrap ignorecase smartcase commentstring=--\ %s formatoptions-=j`，窗口固定 24×80，Go 那边用同一个高度；golden 文件头记下 nvim 版本。
  - 正则：RE2 和 vim 的语法不同，搜索和 `:s` 的差分用例只用两边含义相同的写法（字面量、`.`、`^`、`$`、`\d`、`[…]`、`*`），其余另写单测。POSIX 字符类（`[[:upper:]]` 等）不算通用写法：ignorecase 下 RE2 的 `(?i)` 会让它也忽略大小写，nvim 不会，不进差分用例。
  - 基准是不加载插件的 nvim 本体（M3 审查时定）：`--cmd 'set noloadplugins'` 关掉 matchit 这类自带插件（nvim 0.12.4 上 `--clean` 会把 `loadplugins` 设回 1，`--noplugin` 不起作用，worker 实测），否则 `%` 在 operator-pending 下是 matchit 的行为；nvim 本体的默认映射照做（如 INSERT 下 `C-w` / `C-u` 先断开撤销步）；`formatoptions` 去掉 `j`，因为 `J` 去掉注释前导符只对 nvim 默认 comments 里的 `#`、`//` 等生效，`--` 本来就不在里面，对 SQL 收益小，写进已知上限。
  - `go test` 把自研编辑器的结果和 golden 文件逐条比对。CI 上不需要安装 nvim；只有新增用例或升级 nvim 版本时，才需要重新生成。
  - 已验证可行：在 nvim 0.12.4 上跑了 `ciw`、`daw`、`di(`、`gUiw`、`D`、`J`、`caw`、`dd`、`C`、`gcc`，以及块选择的 `I`、`$A`、`d`、`c`、`y` + `p` 等用例，都能拿到结果文本、光标位置和寄存器类型。
  - 生成期望结果时，用 `silent!` 执行按键，避免 nvim 的提示消息混进输出。
- **默认键位兼容性测试**：断言 `default.toml` 中没有只能在 kitty 协议下使用的键，也没有 Alt 组合键。
- **格式化 golden 测试**：几条典型的 PG / MySQL 语句，格式化结果与 golden 文件比对。升级 sql-formatter 时，输出如有变化，这个测试会失败。
- **渲染 golden 测试**：在固定尺寸 160×45 下渲染 Frame，与 golden 文件比对（`charmbracelet/x/exp/golden` 已是 bubbletea 的依赖）。场景对应设计稿的几种状态：WHERE 下拉、COLS 下拉、命令面板的 SQL 模式、转置、result 在右侧或下方、时间选项浮层、which-key。
- **e2e（tmux 黑盒）**：只测真实终端才能验证的行为：终端模式、SGR 鼠标、resize、字素宽度、时序（连按、超时、取消）、配置加载，以及真实数据库上的端到端流程。布局、颜色、位置由上面的渲染 golden 覆盖，UI 改动时改 golden，不去重写几十条 e2e 断言（M0 复盘）。
- **集成测试**：用 `docker compose` 启动 postgres:17 和 mysql:8.4，运行 `go test -tags integration ./internal/db/...`。覆盖以下内容：catalog、文本值、取消、只读拦截、保存时的乐观校验、枚举和可空属性、用唯一索引作为行标识、无主键表只读、按 console 切换 `search_path`。
- **手工测试矩阵**：Terminal.app、iTerm2、Ghostty、WezTerm、Linux 下的 GNOME Terminal。每个都分别在 tmux 内外测试，重点检查默认键位和鼠标。

## 16. 里程碑

| 阶段 | 内容 | PRD 条目 |
|---|---|---|
| M0 骨架 | Bubble Tea 程序、Frame、Block、主题与主题文件、状态栏；keymap（序列、leader、which-key、次数、映射、提示）；Action 注册表；命中表与键位提示按钮；命令面板（命令、表、pane、window 范围） | B-01~03、K-01~04 的面板部分、第 6 章、第 7 章基础 |
| M1 浏览 | PG 连接、schema 树、data pane 只读（WHERE 条件及其补全、历史/收藏、ORDER/LIMIT/PAGE/COLS、转置）、滚轮；命令面板的快速 SQL（只读，结果显示在面板里） | D-01~04、Q-01~06、G-01、G-04~06、T-01~03、F-01~F-03 |
| M2 编辑 | 单元格编辑与按列属性的选项、待提交标记、保存与刷新 | G-02、G-03 |
| M3 console | vim 编辑器（用 nvim 差分测试校验）、SQL 模糊补全、高亮、分句与执行前高亮、执行与取消、sql-formatter 格式化、底部结果区（日志 tab、结果 tab 的复用与固定）。编辑器（含块选择）是工作量最大的一项，可以从 M0 起并行开发 | C-01~07 |
| M4 命令面板 | 面板的 session 范围、DDL 预览、快速 SQL 的写语句提示与送到结果区（面板本身已在 M0 完成，快速 SQL 的核心在 M1） | K-02、K-05、F-04、F-05 |
| M5 工作现场 | 多 session 与多 window、pane 的分割/缩放/关闭/拖拽、session 列表、只读、事务模式、MySQL | S-01~04、W-01~02、P-01~04 |
| M6 配置 | 键位覆盖/冲突/导出、持久化（主题文件已在 M0 完成） | 第 6 章 |

M1 完成后，就有一个能日常查数据的只读版本，可以尽早给用户试用。

## 17. 待定问题

已确认：

- 技术栈 Go；leader 用 `SPC`；产品名 sqlmux。
- 表格的 chip 与翻页键：`go` `gl` `gp` `gc`、`]` `[`、`/`。
- WHERE 输入完整的条件表达式，不做简写。
- 编辑器支持块选择（`C-v`）。
- 结果展示采用底部全宽结果区（§11）。
- PG 的 schema 选择与 DataGrip 一致：console 标题右侧有 schema 下拉框，每个 console 单独选择（§8.6）。
- 行标识列依次取主键、所有列都不可空的唯一索引；两者都没有的表只读，也不加默认排序，不做额外提示（§10.1、§8.5）。
- 边界情况暂不处理，出现了再说。

暂不讨论，按文中做法实现：

- 「按行提交」：一个事务，每行一条 UPDATE，全部成功才提交（§10.3）。

目前没有待定问题。实现过程中出现的新问题，由 worker 或 tester 提给决策者，决策后更新本文。

PRD 10.2 中影响实现的几条：

- #1 刷新时是否确认：两种实现都支持。
- #2 分页后行号是否连续：只是一个参数。
- #4 which-key：已采纳，见 §6.5。
- #6 工作现场是否恢复：要做（2026-09-30 用户定），M6 F6.5「布局文件」：退出时自动保存、启动时恢复、多个命名布局、`--layout`。数据模型已经可以序列化。

## 18. 需要同步到 PRD / 设计稿的改动

设计稿不再同步（2026-10-04 用户定）：界面按现有实现的风格做，新加的界面元素照已有的同类元素。下表只作为实现与 PRD 的差异记录。

| # | 改动 | 涉及 |
|---|---|---|
| 1 | 前缀 `C-a` 改为 leader `SPC`，只在 NORMAL 模式下生效；pane 焦点新增直达键 `C-h/j/k/l` | 第 2 章「tmux 心智模型」、5.1–5.5、6.2 |
| 2 | 命令面板只用 `C-p` 打开，`C-k` 改为 pane 焦点向上 | K-01、6.2 |
| 3 | console 执行由 `⌥↵` 改为 NORMAL / VISUAL 下的 `↵` | C-02、6.2 |
| 4 | `C-↵` 改为：schema 树中用 `t`，命令面板中用 `C-t` | D-03、K-04、F-04 |
| 5 | 快速 SQL 中写语句的确认，由 `C-S-↵` 改为 `C-e` 在 console 打开后执行 | F-05 |
| 6 | 单元格编辑增加按列属性的选项：时间分段、NULL、DEFAULT、布尔、枚举、原值 | G-02、10.2 #5 相关 |
| 7 | 采纳 which-key 提示层，且各项可点击 | 10.2 #4 |
| 8 | 补充 chip 键位 `go`/`gl`/`gp`/`gc`，翻页 `]`/`[`，WHERE 聚焦 `/`（已确认） | Q-03 |
| 9 | 补充规则：界面上的每条键位提示都是可点击的按钮 | 第 7 章 |
| 10 | console 的 tab 用 `:q` 关闭，因为 `x` 是 vim 的删除字符 | T-02 |
| 11 | 默认键位只使用任何终端都能区分的键 | 6.3 |
| 12 | 新增 console 的 SQL 模糊补全；WHERE 的字段补全和 `C-r` 历史改为 fzf 式模糊匹配 | C-01、Q-01、Q-02 |
| 13 | vim 映射可以按 pane 类型分别配置 | 6.3 |
| 14 | WHERE 不做简写语法，直接输入完整的 SQL 条件表达式 | Q-01 |
| 15 | 结果改为在底部全宽结果区中显示：第 1 个 tab 为日志，每个 console 复用自己的结果 tab，固定的结果单独保留。原来在 console 旁边分割的方式保留为可选配置 | C-04~C-07、第 4 章 |
| 16 | console 编辑器支持块选择 `C-v` | C-01、6.1 |
| 17 | console 标题右侧增加 schema 下拉框（仅 PG，与 DataGrip 一致），每个 console 单独选择 schema | 5.7、3 章 Session 行 |
| 18 | 取消 `SPC 0-9`（原 C-a 0-9）按编号切换 window，改为 `SPC n` / `SPC p` / `SPC l` 和命令面板的 window 范围；默认键位只保留常用的，其余在命令面板里执行，或者自己绑定 | W-02、6.2 |
| 19 | `:` 命令行由命令面板取代：`:` 打开面板（F5.8 起是「所有」范围），`:q` 等用法不变 | B-02、K-01 |
| 20 | 侧栏标题显示当前 schema，可以点击切换；侧栏宽度可以拖动 | D-01~D-04、第 4 章 |
| 21 | 主题可以写成 `~/.config/sqlmux/themes/` 下的文件；表格按列的类型着色 | 第 8 章 |
| 22 | COLS 下拉框打开时焦点在列表上，`/` 才进过滤框；PRD 的「打开时焦点在过滤框」与 `j/k/space/a/A` 冲突 | Q-04 |
| 23 | 「返回行数」显示为计数结果而不是本页行数；G-04「点击行号选中该行」在只读阶段理解为把光标移到该行 | Q-06、G-04 |
| 24 | schema 树改为层级树：session → schema → Tables / Views → 表 → 列，外加工作区（window → pane → tab）；schema 不再用下拉框切换 | D-01~D-04 |
| 25 | NORMAL 下 `;` 直接打开命令面板的 SQL 范围 | F-01、K-01 |
| 26 | 补全列表弹出时默认选中第一项，`Tab` / `S-Tab` 为下一项 / 上一项，`↵` 接受 | C-06、Q-01 |
| 27 | session 改为工作区，不再绑定一个连接：每个 session 都能看到 connections.toml 里的全部连接，目录树的根是这些连接，用到时才连；状态栏的 session 块不画引擎图标，地址块加上连接名 | 第 3 章 Session 行、S-01~S-04、D-01、B-01 |
| 28 | session 列表的 `n` 改为新建 session；新建连接改为命令面板里的「新建连接…」表单 | S-03 |
| 29 | console 标题加上连接下拉和事务控件 `Tx: 自动 ▾`、提交 ✓、回滚 ↺（照 DataGrip）；只读和事务模式都按连接 | 5.7、S-04 |
| 30 | MySQL 切换 database 不再新建 session：database 同 PG 的 schema，树里列出，console 标题上选（执行前 `USE`） | 第 9 章 |

设计稿里写死的键位文字（如 `C-a b`、`run ⌥↵`、`C-↵ 送到 result pane`）会按新的默认键位显示；实现中这些文字都从 keymap 读取，不写死。
