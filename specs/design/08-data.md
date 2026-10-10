# sqlmux 技术方案 · §8 数据访问

> 这是技术方案的一章，各章的索引和阅读规则在 [`../tech-design.md`](../tech-design.md)；章节编号（§N.M）全仓库通用，不随文件改。

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
  - **判定分三档**（F8.19 时定）：不是连接错误；连接丢了但请求没发出去（PG `pgconn.SafeToRetry` 且不是 `conn closed`——pgconn 读回复出错时会先关掉连接再报 `connLockError "conn closed"`，它的 SafeToRetry 恒为 true，可写语句可能已经在服务端提交、只是回复丢了，当「没发出去」重跑就是双写，F8.23 审查时在真库上复现；MySQL `driver.ErrBadConn`）——这档写语句也能重跑；实测只有代理 RST 断开才落在这档，空闲连接被 `pg_terminate_backend`、`KILL`、`wait_timeout` / `idle_session_timeout`、代理 FIN 断开后，下一次请求拿到的都是「已发出」，所以空闲断开后的第一条 update 会提示「请重新执行」，不自动重跑（要做到得每次请求前多一次探测往返，不值）；连接丢了、不知道服务端执行了没有（网络错误、`conn closed`、PG 的 57P01 / 57P02 / 57P03 和 08 类 PgError）——这档只有只读才重跑。请求前先问一下连接是否已关（PG `IsClosed`），已关就先重拨，取消 5 秒后断开的连接不让用户先吃一个错误。
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
| DDL 预览（K-05） | 用 `format_type`、`pg_get_constraintdef`、`pg_get_indexdef` 拼出，末尾带 `comment on table / column … is '…';`（`obj_description` / `col_description`，M9 F9.8），不要求与 pg_dump 一致 | `SHOW CREATE TABLE` |

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
