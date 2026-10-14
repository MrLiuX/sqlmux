# sqlmux 技术方案 · §5 核心数据模型

> 这是技术方案的一章，各章的索引和阅读规则在 [`../tech-design.md`](../tech-design.md)；章节编号（§N.M）全仓库通用，不随文件改。

## 5. 核心数据模型

对应 PRD 第 3 章的 session / window / pane / tab 四级结构。

```go
type Workspace struct {
    Sessions []*Session
    Active   int
}

type Session struct {           // M5 起：session 是一个工作区，不再绑定一个连接（用户 2026-10-02 定）
    Name      string            // 默认取启动时的连接名；之后新建的按 tmux 习惯叫 1、2……，可重命名
    Conns     []*Conn           // connections.toml 里的全部连接，按分组排（§7.8「分组」）；用到时才连
    Windows   []*Window
    ActiveWin int
    RunSeq    int               // result 标题里的执行序号，如 #42
}

type Conn struct {              // 一个数据源，同 DataGrip 的 data source；每个 session 各有一份
    Spec      ConnSpec          // 来自 connections.toml，含 read_only
    State     ConnState         // 未连接 | 连接中 | 已连接 | 失败（记着错误）
    Meta      *db.Worker        // 树、列属性、计数、DDL 预览、快速 SQL、表格取数
    Main      *db.Worker        // 这个连接上所有 query 的执行、表格的保存；持有事务状态
    Manual    bool              // 事务模式：false 自动、true 手动，按连接（F5.3），这个连接上的 query 共用；新连上的都是自动，不记进 state
    Schema    string            // 树里这个连接当前所在的 schema；新 query 和快速 SQL 的默认（§8.6）
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
    Kind      PaneKind          // data | query | result
    Tabs      []Tab
    Cur, Prev int               // tab 栏中的 * 和 -（T-01）
}
```

Tab 的种类：

- `DataTab`：所属的 `*Conn`（M5 起）、表引用、`Query{Where, Order, Limit, Page}`（`Order` 是有序的 `[]OrderCol{Col, Desc}`，M12 F12.1 起多列；空表示默认排序）、隐藏列集合、`GridState{Row, Col, Scroll, Transpose}`、当前页数据、`Edits map[CellKey]Edit`（修改后的值为文本、NULL 或 DEFAULT）、总行数（-1 表示未知）、请求序号 `seq`。
- `QueryTab`：文件路径、`*editor.Buffer`、所选的连接 `*Conn`（M5 起）和 schema（仅 PG，§8.6）、它在结果区里的结果 tab。执行走所选连接的 `Main`，同一个连接上的 query 共用它（用户 2026-10-02 定：每个 query 一条连接更复杂、更容易出错，这一版先复用）。
- `ResultTab`：来源 query、执行序号、`*db.Result`、`GridState`、是否固定。
- `KeyTab`（M16 F16.3，`kind = "key"`）：所属的 `*Conn`、Redis 的 db 号、键名；其余和 `DataTab` 一样（`Query`、`GridState`、`Edits`、当前页、总数、`seq`），因为它就是一个 `DataTab`，数据源换成这个键（下面的「接口缝」）。标题是键名，长的按 tab 标题的规则截断；信息行见 §8.7。

**表格 tab 和数据源之间的接口缝**（M16，用户 2026-10-11 定；这是 M16 最大的工程量，先定这里和 §8.7 再动代码）：表格 tab 只依赖一组操作，SQL 表和 Redis 键各自实现，`app/edit.go` 的保存和 `app/grid.go` 的取数、计数不再直接拼 SQL：

```go
package db

type Source interface {     // 一个表格 tab 背后的数据：一张 SQL 表 / 视图，或一个 Redis 键；绑定了它的连接（Worker）和表 / 键（F16.0 审查时按实现回写）
    Columns(ctx) (Columns, error)                    // 列、行标识、不能编辑 / 新增的原因、过滤框的占位提示；R 刷新时再读一次（Redis 的键换了类型列就变）
    Page(ctx, q PageQuery) (Paged, error)
    Count(ctx, filter string) (int64, error)         // -1 表示不知道（HSCAN 的大 hash、视图），tab 显示 ?
    Each(ctx, q PageQuery, head func([]Col) error, row func([]Val) error) error   // 导出：q 留下的全部行，流式；q.Limit > 0 时只读那一页（导出「当前页」）
    Save(ctx, changes Changes, manual bool) (failed int, tx byte, err error)       // 一批修改原子写回：SQL 一个事务，Redis WATCH + MULTI / EXEC；只读连接在这里拒
}
type PageQuery struct {
    Cols          Columns    // 已知的目录信息：SQL 的取数要行标识和 Lead
    Filter        string     // 不透明文本：SQL 的 WHERE、Redis 的 MATCH
    Order         []OrderCol // 「默认顺序反过来」由 app 展开成键列全 desc 的 Order，没有 Desc 字段
    Limit, Offset int
    Cursor        string     // 游标引擎的续页，取自上一页的 Paged.Cursor
    Live          bool       // 手动事务或在事务里时读 Main（§8.2）
}
type Paged struct {         // 不叫 Page：db 包里已有函数 db.Page
    Result                  // 行
    Next    bool
    Cursor  string          // 非空 = 按游标翻页，tab 只给「下一页」，页号对应的游标记在 request 上
    Info    []Attr          // 标题下的信息行 Attr{Name, Value}；SQL 为空，键 tab 用（§8.7）
}
```

- 三处保持引擎中性：过滤条件是一段不透明的文本（SQL 是 WHERE，Redis 是 MATCH 模式，以后 Mongo 是 filter JSON），校验和占位提示由引擎给；行的身份是引擎给的不透明值（SQL 是主键 / 唯一索引的值，Redis 是 field / member / index / ID，以后是 `_id`），`Change` 里按这个值定位；嵌套值一律是格子里的 JSON 文本，由值编辑器处理。翻页有偏移和游标两种：`Page` 返回下一页用的 `Query`（偏移加一页或带上游标），表格不区分，只知道「有没有下一页」和「能不能回上一页」（游标翻页只能向前，§8.7）。
- 这道缝就是唯一的抽象层：不提前为 MongoDB / Elasticsearch 抽第二层，缝由 SQL 和 Redis 两个真实现定形，第三种引擎来了再用它验证；不做引擎注册表、能力矩阵、插件接口。Mongo（库 → 集合 → 文档）和 ES（索引 → 文档）都能照这组操作映射，树、键 tab、控制台三件套不变。
- SQL 的实现 `db.SQLTable{E, Schema, Name, Meta, Main, Known, ReadOnly}`（`Known` 是 conn.cols 的缓存，`ReadOnly` 给 Save 拒写）只是把现有的 `Page` / `Count` / `Select` / `Each` / `Save` 包一层，逻辑不挪，fetch 里「手动事务或在事务里读 `Main`、`Guarded` 的 savepoint」也搬进它；只读连接的写回在 `Save` 里直接拒（「只读连接」错误，不进事务 / MULTI）；`db.Changes` / `Row` / `Change` 原样用于 Redis，行身份（`Key`）是引擎给的不透明值，Redis 把 hash 的 field 当主键列，改它就是改名（`HDEL` 旧 + `HSET` 新）。`Columns` 加 `Sortable []string`（表头点排序和 ORDER 框只认它：nil 是所有列都能排，SQL 不填；空切片是都不能排；Redis 按类型填）、`NoInsert string`（不能新增行的原因，stream）和 `NoUpdate string`（格子不能改的原因，stream 只能删行）；`Each` 超预算返回 `db.ErrTruncated`，导出提示截断；现有的 `ReadOnly` / `Column.ReadOnly` / `Generated` / `NotNull` 够表达其余。DDL 不进缝：SQL 的留在 `Engine.DDL`，Redis 的重建命令在 `redis.Key` 上。不进缝的三处 SQL 专属功能——`*` 按值过滤、复制为 INSERT、PG 的 `cellCheck`——在 Redis 上 toast「不支持」，不加 Source 方法。
- **连接缝**：`db.Driver{Connect, Broken, ErrorOf}` 从 `Engine` 抽出、`Engine` 内嵌它；引擎表改成 Driver 表；`app.Conn` 持 `drv`，`eng` 只对 SQL 连接非 nil，要 `eng` 的入口（query 的 SQL 路径、快速 SQL、WHERE 补全、DDL 预览、对象树、签名提示）按「是不是 SQL 连接」挡住。`Worker` 不动：Redis 的连接也实现 `db.Conn`（§8.7），重拨、取消、`counted` 全沿用，只给 `counted` 加 `Unwrap() Conn` 让 redis 包拿回自己的连接做 SCAN / pipeline；`TxStatus` 恒为空闲，`Guarded` 不发 savepoint。
- **query tab 的语言缝**（worker 2026-10-14 提、决策者定做）：query 的 SQL 管线（分句、高亮、写 / 危险判定、自动 LIMIT 翻页、`UseSchema`、补全、签名提示、`gq` 格式化，散在 result / hint / format / template / resultpage 里）收进 `app` 内部的 `lang` 接口，`sqlLang` / `redisLang` 两个实现，签名在 F16.0 里照 `sqlLang` 的真实调用写；不另开一种控制台 tab 重写编辑器 / 文件 / 运行状态的外壳。非 SQL 连接上 `gq`、快速 SQL、对象 DDL 这些入口隐藏。
- `Conn` 结构不变：Redis 连接的 `Meta` / `Main` 还是两个 `*db.Worker`；Redis 的每个 db 是一个子 `Conn`（同 F15.1 的 PG 数据库，拨号时 URL 路径改成 `/N`，不用 `SELECT`：Main 是 query 和写回共用的，键 tab 在 db 3、控制台在 db 0 时来回 SELECT 会互相踩）；`Catalog` 对 Redis 是扫到的键（§8.7「树」）。
- 已知上限：SQL 表上的过滤仍然是 WHERE 文本，补全、历史、`C-r` 下拉照旧只对 SQL 有；Redis 的过滤框只有 MATCH 模式、没有补全。

**表格和 query 可以放在同一个 pane 的不同 tab 里**（M3 F3.4 起，用户要求，与 DataGrip 一致）：

- 除结果区外，pane 不再分 data / query 类型。pane 的标题图标、右侧提示（`▶ run`、schema 下拉）、按键作用域都跟着当前 tab 的类型走；tab 栏每个 tab 前面画类型图标。`Pane.Kind` 只剩「普通 / 结果区」之分，M0 修剪时说的那几张按 PaneKind 下标的并行表随之删掉。
- 结果区照旧是独立的 pane，只放结果 tab；query 的执行结果不论 query 在哪个 pane，都进底部结果区（§11）。
- 打开表时当前 tab 是 query：`↵` 不替换它，改为在这个 pane 新开一个 tab。query 里是用户写的 SQL，不能被一张表顶掉。
- 焦点不在普通 pane 上（比如在树上）时，目标 pane 取最近聚焦过的普通 pane，query 所在的 pane 也算（F3.37 用户定，取代 F3.7 的「跳过当前 tab 是 query 的 pane」：焦点在 query 时打开的表就开在这个 pane）。
- 快速 SQL 的 `C-e`（在 query 中打开）在目标 pane 新开一个 query tab。
- 目录树的工作区里，pane 节点不再标类型，tab 节点按各自的类型显示图标。
- **引导页**：没有 tab 的 pane、点 `+` 新开的 tab、分割出来的新 pane，都显示同一个引导页，不再是空白。目标 pane 的当前 tab 是引导 tab 时，打开表（`↵`、`t`、`C-t`、中键都一样）和新建 query（`c`、点击、面板都一样）都替换它，不另开 tab：引导 tab 只是「新 tab」的占位。例外（F3.37）：这个 pane 里已经开着这张表时，切到那个 tab 并关掉引导 tab。内容区中间两个按钮：「打开表」打开命令面板的表范围，选中的表开在这里；「新建 query」直接在这里开一个 query。按钮可点击，旁边的键位文字从 keymap 读。它取代 M1 里「`+` 聚焦树的过滤框」的做法：面板的表范围能跨 schema 模糊搜，更适合挑表。

**一个 session 多个连接**（M5，用户 2026-09-29 提出、2026-10-02 定，参考 DataGrip 的多个数据源）：

- **连接列表**：每个 session 都能看到 connections.toml 里的全部连接。目录树的根就是这些连接（引擎图标、名字、只读时加锁的标记），展开或第一次用到时才建 `Meta` / `Main`；连不上时节点标成失败、错误进 toast，可以再展开重试。连接名不能重复，重复时启动报错。`sqlmux play` 启动时只连 play 并展开它；不带参数时连第一个（M6 起恢复 `last` 布局）。启动时的连接连不上，M12 F12.5 起和别的连接一样：节点标失败、错误进 toast、用它的 tab 留着显示错误页（§14「恢复」），程序照常进界面（用户 2026-10-11：连不上也要能进去在树里建新连接；原来照 §14 打印错误退出）。
- **多个 session 保留**，作工作区用，像 tmux 的多个 session：各有自己的 window、布局，以及自己的一套连接对象（互不共享事务、search_path）。
- **query 复用所属连接的 `Main`**（用户 2026-10-02 定，先不做每个 query 一条连接）：同一个连接上的 query 共用 `Main` 的事务；search_path 照 §8.6 在每次执行前按这个 query 选的 schema 设好；两个 query 同时执行时排队（Worker 的锁）。M3「在 query 里 begin 不提交，表格的保存也会进这个事务」的已知上限照旧，由事务模式和状态栏的「事务中」提示兜住。
- **query 标题**：`<连接> · <库>.<schema> ▾`，连接和 schema 各是一个下拉框。换连接只改这个 query 下次执行用哪个连接；它当前所在的连接有未提交的事务时不让换（提示先提交或回滚）。query 的文件不再按连接名分目录（§11），记着它最后用的连接和 schema。
- **事务模式按连接**（F5.3）：每个 query 标题上 `Tx: 自动 ▾` 显示的是它所在连接的模式，在任一个 query 里切换，同一个连接上的 query 都跟着变；手动时旁边是提交 ✓、回滚 ↺，作用于这个连接的 `Main`。手动模式下表格的保存也进这个事务、不自动提交，表格取数改走 `Main` 才看得到自己没提交的修改（§8.2、§10.3 原来的写法）。有未提交的事务时，退出、关闭 session、关掉这个连接上的最后一个 query 都要确认，确认后先回滚（§10.5）；从手动切回自动被拦下，提示先提交或回滚。
- **只读按连接**：connections.toml 里写 `read_only = true`。这个连接的 `Meta`、`Main` 都设成只读（§13 两层拦截照旧），树和 query 标题上画锁的标记，表格不能进入编辑。
- **PG 的数据库层**（M15 F15.1，用户 2026-10-11：同 DataGrip，PG 一个连接只能看一个库）：模型不变，**每个（连接, 数据库）一对 `Meta` / `Main`**：`Conn` 上多一张 数据库名 → 子 `*Conn` 的表，子 Conn 用 spec 的副本加一个不落盘的 `Database` 覆盖字段拨号（postgres 包改写 DSN 里的 dbname），展开那个库时才建、才连；catalog 缓存、事务模式、只读、busy、重拨都原样按子 Conn 走，所以事务模式从「按连接」变成「按库」（事务本来就在物理连接上，worker 起草时提出、决策者同意）。DSN 里的库是默认展开的那个；PG 连接 N 个库就有 N 对 Worker，不做共享。MySQL 的 database 就是 schema 这一层、SQLite 没有，这两个引擎的 `Databases` 返回 nil，树不变。
- **当前连接**：表 tab、query tab 各属于一个连接，结果 tab 记着来源的连接。「当前连接」是焦点 tab 所属的连接（结果 tab 取它的来源连接），焦点在树上时是树光标所在的连接（光标在还没连上或失败的连接节点上时沿用上一次的；M12 F12.5 起启动连接也可能失败，这时当前连接可以是失败的那个，需要连接的命令按 §8「启动」重拨或 toast），引导页、空 pane 沿用上一次的。快速 SQL、新 query、`C-e`、`tree.refresh`、`C-c` 取消和 busy 提示、事务命令都按当前连接；命令面板的表范围列出所有已连上的连接的表（M15 F15.1 起每个已连上的（连接, 库）都算一个连接：库的表在 catalog 里现成、不多拨号；条目右边写 `连接/库.schema`，默认库不写库；条目的 id 带库，选中后回到那个库的 Conn 去开，不按名字回父 Conn 找，审查时定），当前连接的排前面（§12）。状态栏的地址块显示当前连接的名字和 `<用户>@<host>:<port>`（§7.8）。
- 开发清单见 [`m5-workspace/task.md`](m5-workspace/task.md) 的 F5.10、F5.11、F5.1–F5.4（2026-10-02 定）；§7.8、§8、§10、§11–§14 已跟着改。

默认 window 的布局：

```
Window
├─ 侧栏 ⟨0⟩ schema（可折叠）
└─ Node(纵向, 0.6)              ← 第一次执行 SQL 时，把原来的根节点包进一个新的纵向节点
   ├─ Node(横向, 5:4)          ← data 与 query 的宽度比，取自设计稿（§7.8）
   │  ├─ Pane ⟨1⟩ data
   │  └─ Pane ⟨2⟩ query
   └─ Pane ⟨3⟩ result           底部全宽结果区（§11）
```

布局树的操作写在 `internal/app/layout.go` 里，都是纯函数，可以单测：

- **分割**：把叶子替换成 `Node{A: 原叶子, B: 新叶子}`。新 pane 是与原 pane 同类型的空 pane（没有 tab，样式见 §7.8），焦点移到新 pane。M3 起新 pane 显示引导页（见上文「表格和 query 可以放在同一个 pane」）。
- **关闭**：用兄弟节点替换父节点。window 里唯一的普通 pane 关不掉（`SPC x` 不做事），结果区不算在内：否则布局里只剩结果区，什么表都打不开了。
- **调整大小**：修改该方向上最近的祖先分割节点的 `Ratio`。
  - 每按一次移动 5%，乘以次数前缀（`3 SPC L` 移动 15%）；比例限制在 10%–90%。
  - 方向与 tmux 的 resize-pane 相同：`H` 把分割线往左移，与当前 pane 在分割线的哪一侧无关。
- **缩放**：pane 占满状态栏以上的整个 window，侧栏的位置也被占用（P-03「缩放至整个 window」）。缩放中的 pane 要看得出来（M9 F9.6，用户：放大后以为只开了一个 pane）：标题的编号后面画 `zoom` 图标（nf-fa-expand U+F065，ascii `[Z]`），边框用主题的 `zoom` 色（§7.2，缺省 = `warn`），焦点与否都是；只有缩放中的那个 pane 画，退出恢复。同 tmux 在 window 名后标 `Z`。分割、关闭、按编号跳转都会退出缩放。从命令面板聚焦 pane、或者把表打开到某个 pane 时也一样：目标 pane 被缩放遮住时，先退出缩放再聚焦；目标就是正在缩放的 pane 时，保持缩放。
- **按方向切焦点**：用上一帧的 pane 矩形，在该方向上找相邻、且有重叠的 pane。
  - 有多个候选时，选最近获得过焦点的那个；都没获得过焦点时，选遍历顺序靠前的（上 / 左）。这与 tmux 的 `window_pane_choose_best`（取 `active_point` 最大者）一致。
  - 选它而不选「重叠最长」，是因为这样 C-l 之后再 C-h 总能回到原处。更重要的是默认布局：从底部全宽的结果区按 C-k，会回到刚才执行查询的 query，而不是更宽的 data。
  - 与 tmux 不同的一点：到了边上不绕到另一侧。侧栏固定在最左边，vim 的 `<C-w>h` 也不绕回。
- **按编号跳转**（`SPC q`）：编号一直显示到下一个按键，不自动消失（相当于 tmux 的 `display-panes -d 0`）。按数字跳到对应的 pane；按其他键只关闭编号，这个键不再执行别的操作。
  - 鼠标：编号也是键位提示，按 §7.4 可以点击。点击某个 pane（含侧栏 ⟨0⟩），等于按下它的编号；点击 pane 以外的地方（状态栏、间隔）只关闭编号。

默认 window 名为 `data`，之后新建的叫 `data-2`、`data-3`……；`window.new` 新建的 window 只有一个空 pane（引导页），不带 query，同 tmux 的 `new-window`（M8 F8.18）；启动时的默认布局（左树、引导页、query）不变。（同一 session 里最小的没用过的序号；M8 F8.11，用户：三个 `data` 分不清）；同一 session 里 window 名不能重复，`window.rename` 改成已有的名字时 toast 拒绝。各个 pane 随里程碑逐步出现：M1 只有侧栏和 data，data 占满其余宽度；query 在 M3 加入默认布局（data : query 原为 5 : 9；M8 F8.9 起 1 : 1，用户看到两块不等宽以为是 bug，拆分出来的一对本来就是均分的；代价是 160 列下 ① 从 70 列变成 61 列，查询条按 §7.8 的让位顺序让掉转置、自动刷新、`+ −` 三组，保存 / 刷新组改成排在 LIMIT 之后让位所以还在；接受），结果区在第一次执行 SQL 时出现。

所有持久化字段都是普通值类型，运行时句柄（连接、编辑器实例）不进入序列化结构。所以工作现场恢复（PRD 10.2 #6，M6 F6.5 布局文件）只需要补上读写逻辑。
