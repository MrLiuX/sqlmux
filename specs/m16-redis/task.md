# M16 Redis 初版 · 任务清单

- **状态**：**todo**（2026-10-11 用户定方向；2026-10-14 用户验收 M15 后定接着做；清单 worker 当天起草、决策者定，设计在各章）。
- **目标**：Redis 支持的第一版，做到能日常用：连接、按命名空间分的树、六种类型加 RedisJSON 的键 tab、一行一条命令的控制台、来自服务器的补全和签名提示。用户体验过初版之后再定第二轮（Pub/Sub、MONITOR、SLOWLOG、Cluster / Sentinel、内存分析，候选 M17）。
- **范围**（设计 2026-10-14 已写进各章，以它们为准，下面的「已定的方向」是当时的记录）：tech-design §4（`internal/db/redis`、go-redis v9）、§5（tab 的新类型 `key`；`db.Source` 接口缝）、§7.7（类型图标）、§7.8（「Redis 的树」）、§8.7（连接、SCAN、键 tab 的数据源表）、§9.8（控制台的词法、高亮、补全）、§11（「Redis 控制台」的回复摆法）、§13（只读与危险命令来自 `COMMAND INFO`）、§14（`redis://` DSN、`delimiter`、`[redis]` 段、layout 的 `key` tab）、§15（redis-stack 容器、`SQLMUX_TEST_REDIS`）。
- **依赖**：M15 passed（F15.3 的签名提示浮层在这里和 Redis 共用；M14 F14.2 的历史）。
- **审查节点**（worker 2026-10-14 起草、决策者定）：1 = F16.6 + F16.0（F16.6 是一行键位、先做；F16.0 接口缝单独成批：不改任何行为，全量 e2e 一条不改就是验收）；2 = F16.1 + F16.2；3 = F16.3；4 = F16.4 + F16.5。

已定的方向（2026-10-11，用户同意；决策时参考了 RedisInsight 的 Browse 和 DataGrip 的 Redis 支持，目标是比它们展示更清楚、JSON 更友好、键盘能走完）：

- **复用**：session / window / pane / tab、树和侧栏过滤、命令面板、键位、主题、i18n、布局文件、连接表单和测试按钮、状态栏、断线重连（F8.19）、toast、错误页、`ui.Grid`（多选、复制为 JSON、COLS、转置、导出、编辑标记和 `C-s` 保存的交互）、值编辑器（F12.2）、query tab 的编辑器 / 结果区 / 结果翻页 / 历史 / 补全框、DDL 查看。
- **不复用**：`sqlkit`（SQL 词法、分句、WHERE、只读判定、补全上下文）和 `db.Engine`（SQL 形状的接口：Quote、Arg、Tables、TableColumns、Quick）；Redis 不塞进 `db.Engine`。
- **接口缝**（这是 M16 最大的工程量，先写 §5 / §8 再动代码）：表格 tab 只依赖一组操作——翻一页、过滤、排序、把一批修改写回、键 / 表的信息；SQL 表和 Redis 键各自实现，`app/edit.go` 的保存不再直接拼 SQL。SQLite（M14）不需要这道缝，Redis 必须有，所以排在 M14 之后。
- **这道缝就是唯一的抽象层**（用户 2026-10-11 问要不要为以后的 MongoDB / Elasticsearch 提前抽一层；定：不提前抽，缝由 SQL 和 Redis 两个真实现定形，第三种引擎来了再用它验证，不做引擎注册表、能力矩阵、插件接口这类「以后可能用到」的东西）。为了让它不长成 Redis 专用的，接口按表格 tab 的需要写、不按引擎写，三处保持引擎中性：过滤条件是一段不透明的文本，由引擎解释并提供输入框的占位提示（SQL 是 WHERE，Redis 是 MATCH 模式，以后 Mongo 是 filter JSON）；行的身份是引擎给的不透明值（SQL 是主键，Redis 是 field / member / index / ID，以后是 `_id`）；嵌套值一律是格子里的 JSON 文本，由值编辑器处理。翻页有偏移和游标两种，Redis 自己就两种都有，不另加。Mongo（库 → 集合 → 文档）和 ES（索引 → 文档）都能照这组操作映射，树、键 tab、控制台三件套不变；它们的实现到要做时再说。

决策者 2026-10-14 定的（worker 起草清单时问）：Redis 的 db 用 F15.1 的子连接（每 db 一对 Meta / Main，URL 路径 `/N`），不用 `SELECT`（worker 第一版提的，第二版撤回想改成每次请求 pipeline 里先 SELECT；决策者定子连接：F15.1 的机制现成、没有连接态要管）；F16.0 单独一个 feature、一个审查节点；query tab 的语言缝做（`lang` 接口，不另开控制台 tab）；命令面板的「表」范围这一版在 Redis 连接上不列键；测试只起 redis-stack（7.x），Redis 6 的退化用假连接单测；`Source` 的签名按 §5（绑定连接和表 / 键，`Count` 用 -1，有 `Each`，没有 `Describe`）。

## F16.6 表格和树的方向键 · 状态：todo

用户 2026-10-14：vim 模式下 `←↓↑→` 不动表格光标（非 vim 层有，§6.9）。nvim 的 NORMAL 本来就认方向键，两种模式都给。

- **涉及**：`internal/keymap/default.toml`、`default-novim.toml`、README 的键位表

**开发**（§6.9）
- [ ] `[keys.grid]` 的 `<Left>` `<Down>` `<Up>` `<Right>` 和 `[keys.tree]` 的四个从 `default-novim.toml` 挪进 `default.toml`；`PgUp` / `PgDn` / `Delete` 留在非 vim 层；`sqlmux keys` 和 README 的表跟着。

**验收**
- [ ] 单测：两种模式下 grid / tree 作用域都解析出方向键，默认键位兼容性测试仍过。
- [ ] e2e：vim 模式下表格里 `→` `↓` 动光标、树里 `→` 展开。

## F16.0 接口缝（SQL 侧重构，行为不变）· 状态：todo

- **依赖**：M15
- **涉及**：`internal/db`（`source.go`）、`internal/app`

**开发**（§5「接口缝」「连接缝」「语言缝」）
- [ ] a. `db/source.go`：`Source`、`PageQuery`、`Page`、`Attr`；`db.SQLTable` 包现有 `Page` / `Count` / `Select` / `Each` / `Save`，fetch 里选 Meta / Main 和 `Guarded` 的逻辑搬进去。
- [ ] b. `dataTab` 持 `src db.Source`：fetch / count / sendSave / export 改走它；request 加游标表；`Columns` 加 `Sortable`、`NoInsert`；`conn.cols` 缓存仍只给 SQL。
- [ ] c. `db.Driver` 抽出；`Conn.drv`；`eng == nil` 的入口挡住；三处拨号（reconnect、workspace、connform）走 Driver；`counted.Unwrap`。
- [ ] d. app 内 `lang` 接口 + `sqlLang`，query / result / resultpage / hint / format / template 改走它；非 SQL 连接上 `gq`、快速 SQL、对象 DDL 等入口隐藏。

**验收**
- [ ] 全量 e2e 和现有 golden 一条不改全过；`just check` 绿。
- [ ] 单测：`SQLTable` 对 PG / MySQL / SQLite 的 Page / Count / Each / Save 与改前逐字相同（现有 page_test / save_test 套到 Source 上）；假 Source（内存）驱动 `dataTab`：游标翻页（只有下一页、页号对应游标）、`Count` -1 显示 `?`、`Sortable` 为空时表头排序 toast、`NoInsert` 时新增行被拒。

## F16.1 驱动与连接 · 状态：todo

- **依赖**：F16.0
- **涉及**：`internal/db/redis`、`internal/config`、`internal/app`（表单、workspace）、docker-compose、ci.yml、`testdata/seed`

**开发**（§8.7「连接」、§14、§15）
- [ ] a. `internal/db/redis`：go-redis v9，`redis.Conn` 实现 `db.Conn`（Exec = 控制台一行、Query = 一条命令）；`Driver`：Connect 用 `ParseURL`（redis:// rediss://，用户名密码走 ACL）、`HELLO 3`，Info 来自 `INFO server`（Redis < 6 报「需要 6.0+」）；Broken 认 `io.EOF` / net.Error / `LOADING` 之类；ErrorOf 把 `WRONGTYPE …` 的第一个词当 Code。每个 db 一对连接（F15.1 的子 Conn，URL 路径 `/N`）。
- [ ] b. config：`engine = "redis"`（Check 放行）；`[redis] delimiter` / `scan_keys` / `load_all` 和连接里的 `delimiter` 覆盖（进未知键白名单；非 redis 连接写了 delimiter 时 Check 提示）；密码走现有 password_cmd / env / 钥匙串；`ssh` 对 Redis 同样生效（`Options.Dialer` 接 db 的拨号，F15.2 的假 ssh 能测）。
- [ ] c. 表单：engine 选项加 redis，DSN 示例，`db` 一栏（只是 URL 路径 `/N` 的另一种写法，不另存）；测试按钮发 `PING`。
- [ ] d. 取消 / 阻塞命令：先写 spike 测试（真 Redis 上 `BLPOP` 不带超时、取消 ctx，看 go-redis 能否中断；预期不能）；不能就取消时关连接，让 Worker 走 F8.19 重拨。
- [ ] e. 集成环境：docker-compose 和 ci.yml 加 redis-stack-server（带 RedisJSON）；`testdata/seed/redis.txt`（每种类型、一个大 hash > 5000、一个大 keyspace > 10000、二进制值、带 TTL 的键）用 `redis-cli --pipe` 导入；`SQLMUX_TEST_REDIS`。改 seed 要通知 reviewer 和 tester。

**验收**
- [ ] 集成：PING；INFO 解析；ACL 用户名密码；rediss 只测 URL 解析；取消 BLPOP 在 2 s 内返回且连接可再用；ssh 经假 ssh 通；db 3 的子连接 `CLIENT INFO` 里 db=3。
- [ ] 单测：DSN → 表单字段往返；connections.toml 往返和旧文件兼容；Check 对 engine 和 delimiter 的提示文字。
- [ ] e2e：新建 Redis 连接（表单、测试按钮）、状态栏地址块、断线（`CLIENT KILL`）后自动重拨。

## F16.2 树 · 状态：todo

- **依赖**：F16.1
- **涉及**：`internal/app`（tree）、`internal/db/redis`、`internal/ui`（图标）

**开发**（§7.8「Redis 的树」、§8.7「树」、§7.7）
- [ ] a. 连接 → db → 命名空间 → 键，节点 id `/db:N/ns:a:b/key:name`；db 只列 `INFO keyspace` 里的加 DSN 指定的；展开 db 就是连它的子 Conn。
- [ ] b. SCAN 预算：展开 db 时在 Meta 上 `SCAN 0 COUNT 1000` 循环到 `[redis] scan_keys`，每批 pipeline `TYPE` 取类型；db 节点尾注 `DBSIZE`，超预算时「已扫描 10000 / 1.2M，用 / 过滤」；命名空间尾注 = 下面扫到的键数；永远不用 KEYS。
- [ ] c. 侧栏 `/` 过滤在 Redis 连接上走服务端 `SCAN MATCH`（不含 `*` 两头补 `*`），同预算；不走本地过滤。
- [ ] d. 图标名 `redis` 和 `str` / `hash` / `list` / `set` / `zset` / `stream` / `json` 进 `[icon]` 的名字表（名字写错启动报错那张），颜色用 §7.7 指定的现有主题键。
- [ ] e. 操作：键 删除、改名（RENAME，目标存在时确认覆盖）、设 TTL；命名空间节点删除 = 不设预算扫全、确认「删除 N 个键？」后每批 500 个 `UNLINK`（走 confirm_dangerous），`C-c` 能停、toast 已删数量；`tree.refresh` 重扫光标所在 db。
- [ ] f. layout / state 的树展开状态带 db 和命名空间；命令面板的「表」范围在 Redis 连接上不列键。

**验收**
- [ ] 集成：> 预算的 keyspace 只扫预算内、尾注文字对；MATCH 过滤结果对；UNLINK 分批删光一个命名空间、取消后已删数量对；RENAME 覆盖确认。
- [ ] 单测：键名按分隔符切成树（连续分隔符、首尾分隔符、键名等于命名空间名）；连接级 delimiter 覆盖全局。
- [ ] golden：db → 命名空间 → 键展开、超预算尾注、七种图标。
- [ ] e2e：展开 db、点进命名空间、`/` 过滤、删命名空间。

## F16.3 键 tab · 状态：todo

- **依赖**：F16.0、F16.2
- **涉及**：`internal/db/redis`（`Key` 实现 Source）、`internal/app`（key tab、layout）、`internal/ui`（信息行）、`internal/config`（layout 的 key 字段）

**开发**（§5 `KeyTab`、§8.7「键 tab」、§14）
- [ ] a. `redis.Key` 实现 Source：`Columns` 按 TYPE 出各类型的列和 `Sortable`（≤ load_all 的 hash / set / zset 全列，zset 的 score；HSCAN / SSCAN 和 list / stream 为空）、`NoInsert`（stream、string、json）；Page（hash / set / zset ≤ load_all 整取、本地翻页 / 排序 / 过滤，超过走 HSCAN / SSCAN / ZRANGE 偏移；list LRANGE；stream XRANGE 按 ID 游标）；Count（HLEN / LLEN / SCARD / ZCARD / XLEN；MATCH 过滤且走游标的类型 -1）；Each（导出，按 `scan_keys` 预算截断并提示）；Save（只读连接直接拒；WATCH + MULTI / EXEC，按 §8.7 的表；string `SET … KEEPTTL`；list 的删除 = LSET 哨兵 + LREM）；Info = 类型、TTL、编码、长度，内存（MEMORY USAGE）只在 `R` 刷新时查；`Rebuild` 给 DDL 查看。
- [ ] b. tab：`kind = "key"`（layout 记 connection、db、key），复用 dataTab 的网格 / 编辑 / 标记 / `C-s`；标题 = 键名；信息行画在标题下；TTL（EXPIRE / PERSIST）、改名、删除在信息行上操作，TTL 人话化（`12m03s`）、-1 =「不过期」、`R` 刷新；键在别处被删或换了类型，下一次取数报「键已不存在 / 类型已变」并回到信息行。
- [ ] c. 查询条的 WHERE 位置放 MATCH 模式，占位提示由 Source 给；WHERE 补全、`*`、复制为 INSERT 在 Redis 上 toast 不支持。
- [ ] d. RedisJSON：值编辑器打开、`JSON.SET $` 写回；TYPE 给的 `ReJSON-RL` 识别为 json。
- [ ] e. 「以 JSON 查看」（`key.json`）和 DDL 查看（`Rebuild` 的命令：HSET / SET … EX / RPUSH …）复用 jsonview / ddlview；二进制值走 §8.1 占位符。

**验收**
- [ ] 集成（真 Redis 上七种类型逐个）：Page 内容与 `redis-cli` 一致；各类型的写回（改值 / 增 / 删 / 改 Field = 改名）；只读连接的 Save 被拒、没发 MULTI；并发改同一键 → ErrStale（WATCH 失效）；EXEC 前断线 → ErrCommitUnknown；> load_all 的 hash 走 HSCAN，只有下一页，MATCH 过滤对；zset 两种模式；stream 只能 XDEL；list 删除用哨兵不误删相同值的别的行；二进制值不被写回；导出超预算截断并提示。
- [ ] 单测：Changes → 命令序列（表驱动，每类型每种变更）；TTL 文本；Sortable / NoInsert 随类型；layout 往返。
- [ ] golden：每种类型一屏（带信息行）、游标翻页的页脚、JSON 查看。
- [ ] e2e：打开各类型键、改一个格子 `C-s` 后 redis-cli 里对、设 / 去 TTL、改名、删除、`R` 刷新、重启后 layout 恢复到键 tab。

## F16.4 控制台 · 状态：todo

- **依赖**：F16.0、F16.1
- **涉及**：`internal/db/redis`、`internal/app`（`redisLang`、result 内存翻页）

**开发**（§9.8、§11「Redis 控制台」、§13）
- [ ] a. `redisLang`：一行一条命令，`#` 开头是注释，引号照 redis-cli（`"…"` 带转义、`'…'`）；`↵` 跑光标行，选区和全部同 query；标题的 db 段下拉换子连接。
- [ ] b. 高亮：命令名来自连接时在 Meta 上取一次的 `COMMAND` 缓存，字符串和注释沿用现有色键。
- [ ] c. 判定：只读连接拒绝 `write` 标记的命令，`@dangerous` 走 confirm_dangerous，两份名单都来自 `COMMAND INFO`；子命令（`CONFIG SET`、`CLIENT KILL`）按 `command|sub` 查；`COMMAND` 被 ACL 禁掉时全按写处理、危险名单空、toast 一次。
- [ ] d. 回复成表：标量 `value`；数组一列；map 两列 field / value；等长二维数组按列；更深嵌套是格子里的 JSON；nil 是 NULL；错误进错误栏。整份回复在内存，按 500 行翻页，「全部」不重跑（resultpage 新增「内存页」路径，SQL 仍是 AutoLimit 重跑）。
- [ ] e. 阻塞命令（BLPOP / SUBSCRIBE 不带超时）取消 = F16.1d 的关连接重拨；历史走现有 history.jsonl；控制台里 `SELECT n` 只改这条 Main 的 db，标题和树不跟着变（`ponytail:` 标出）。

**验收**
- [ ] 单测：分词（引号、转义、空格、注释、未闭合引号）表驱动；回复 → 表（标量 / 数组 / map / 等长二维 / 嵌套 / nil / 错误）；mutation 逐条验。
- [ ] 集成：只读连接拒写（用 COMMAND INFO，不手写名单）；FLUSHALL 走确认；5000+ 元素的回复分页、「全部」不重发命令（用 `INCR` 计数验证只跑一次）。
- [ ] golden：每种回复形状一屏。
- [ ] e2e：跑 `SET` / `GET` / `ZRANGE … WITHSCORES` / `HGETALL` / `LRANGE 0 -1`、写命令被只读挡、BLPOP 取消。

## F16.5 补全与签名提示 · 状态：todo

- **依赖**：F16.4、F15.3
- **涉及**：`internal/db/redis`、`internal/app`、`internal/ui`（复用提示浮层）

**开发**（§9.8「补全」）
- [ ] a. `COMMAND DOCS`（Redis 7+）：命令名、子命令、参数签名、一句说明，连接时取一次缓存在 Meta 上；没有 DOCS（`ERR unknown subcommand`）就只补命令名（`COMMAND` 里来）。
- [ ] b. 补全：命令名 → 子命令 → 键名；候选排序照 §9.7；签名提示用 F15.3 的浮层，当前参数高亮（可变 / 可选参数、子命令的签名从 DOCS 的 arguments 树展开）。
- [ ] c. 键名补全：打字时 Meta 上 `SCAN MATCH <前缀>* COUNT 100`，去抖（只一轮，不循环到底）；字段名：HGET / HSET / HDEL 的键已知且长度 ≤ load_all 时 `HKEYS`。

**验收**
- [ ] 单测：DOCS 的 arguments 树 → 签名文本 / 当前参数下标（可选、可变、oneof、block、子命令）表驱动；mutation；假连接没有 DOCS 时退化为只补命令名。
- [ ] 集成：7.x 的 SET / ZADD / XADD 签名与 `COMMAND DOCS` 一致；键名补全对 10000+ 键的库不超时。
- [ ] golden：签名提示行（含当前参数高亮）、键名候选框。
- [ ] e2e：打 `HSET key f` 时提示行高亮第二个参数；`GET us` 补出键名。

不做（第二轮候选）：Pub/Sub tab、MONITOR、SLOWLOG、CLIENT LIST、内存分析、Cluster / Sentinel、按 score 范围过滤 zset、键的批量设 TTL、命令面板里列键。

任务文件的格式和状态约定见 [`../plan.md`](../plan.md)。
