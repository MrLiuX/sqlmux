# M16 Redis 初版 · 任务清单

- **状态**：**draft**（2026-10-11 用户定方向；2026-10-14 用户验收 M15 后定：接着做 M16。worker 先按下面的方向起草每个 feature 的开发 / 验收清单和接口缝的签名发决策者，决策者同时写设计章节，清单定下来改 todo 再开工）。
- **目标**：Redis 支持的第一版，做到能日常用：连接、按命名空间分的树、六种类型加 RedisJSON 的键 tab、一行一条命令的控制台、来自服务器的补全和签名提示。用户体验过初版之后再定第二轮（Pub/Sub、MONITOR、SLOWLOG、Cluster / Sentinel、内存分析，候选 M17）。
- **范围**：tech-design §4（`internal/db/redis`、go-redis v9）、§5（tab 的新类型 `key`；表格 tab 和数据源之间的接口缝）、§7.7（类型图标）、§7.8（树的 db 和命名空间层、键 tab 的信息行）、§8（键值引擎的接口、SCAN 预算、翻页规则）、§9（控制台的词法、分句、补全）、§11（控制台）、§13（只读与危险命令来自 `COMMAND INFO`）、§14（`redis://` DSN、`[redis]` 段）、§15。
- **依赖**：M15 passed（F15.3 的签名提示浮层在这里和 Redis 共用；M14 F14.2 的历史）。
- **审查节点**：待定，倾向三个：F16.1–F16.2（驱动、连接、树）；F16.3（键 tab）；F16.4–F16.5（控制台、补全）。

已定的方向（2026-10-11，用户同意；决策时参考了 RedisInsight 的 Browse 和 DataGrip 的 Redis 支持，目标是比它们展示更清楚、JSON 更友好、键盘能走完）：

- **复用**：session / window / pane / tab、树和侧栏过滤、命令面板、键位、主题、i18n、布局文件、连接表单和测试按钮、状态栏、断线重连（F8.19）、toast、错误页、`ui.Grid`（多选、复制为 JSON、COLS、转置、导出、编辑标记和 `C-s` 保存的交互）、值编辑器（F12.2）、query tab 的编辑器 / 结果区 / 结果翻页 / 历史 / 补全框、DDL 查看。
- **不复用**：`sqlkit`（SQL 词法、分句、WHERE、只读判定、补全上下文）和 `db.Engine`（SQL 形状的接口：Quote、Arg、Tables、TableColumns、Quick）；Redis 不塞进 `db.Engine`。
- **接口缝**（这是 M16 最大的工程量，先写 §5 / §8 再动代码）：表格 tab 只依赖一组操作——翻一页、过滤、排序、把一批修改写回、键 / 表的信息；SQL 表和 Redis 键各自实现，`app/edit.go` 的保存不再直接拼 SQL。SQLite（M14）不需要这道缝，Redis 必须有，所以排在 M14 之后。
- **这道缝就是唯一的抽象层**（用户 2026-10-11 问要不要为以后的 MongoDB / Elasticsearch 提前抽一层；定：不提前抽，缝由 SQL 和 Redis 两个真实现定形，第三种引擎来了再用它验证，不做引擎注册表、能力矩阵、插件接口这类「以后可能用到」的东西）。为了让它不长成 Redis 专用的，接口按表格 tab 的需要写、不按引擎写，三处保持引擎中性：过滤条件是一段不透明的文本，由引擎解释并提供输入框的占位提示（SQL 是 WHERE，Redis 是 MATCH 模式，以后 Mongo 是 filter JSON）；行的身份是引擎给的不透明值（SQL 是主键，Redis 是 field / member / index / ID，以后是 `_id`）；嵌套值一律是格子里的 JSON 文本，由值编辑器处理。翻页有偏移和游标两种，Redis 自己就两种都有，不另加。Mongo（库 → 集合 → 文档）和 ES（索引 → 文档）都能照这组操作映射，树、键 tab、控制台三件套不变；它们的实现到要做时再说。

- **F16.1 驱动与连接**：go-redis v9（BSD-2），`dsn = "redis://user:pass@host:6379/0"` / `rediss://`，用它的 URL 解析，表单加 `db` 一栏；测试按钮发 `PING`；`Info` 的版本来自 `INFO server`（`redis_version`、`redis_mode`）；状态栏地址块照旧。每个 session 仍是 Meta / Main 两条连接：Meta 做 SCAN / TYPE / TTL / `COMMAND DOCS`，Main 跑控制台和写回。阻塞命令（BLPOP、SUBSCRIBE、MONITOR）会卡住 Main，取消时关连接重拨，走 F8.19 的路。`password_cmd` / `password_env` / 钥匙串和 SQL 连接一样。Cluster、Sentinel 不做（go-redis 有，等第二轮）。
- **F16.2 树**：连接 → db（只列 `INFO keyspace` 里非空的，加 DSN 指定的那个；切 db 是 `SELECT`，对应 SQL 的 schema 层）→ 命名空间（按分隔符切键名，默认 `:`，`[redis] delimiter` 可改，连接里 `delimiter` 可覆盖）→ 键。键前面是类型图标（str / hash / list / set / zset / stream / json），和现在表 / 视图的图标一样走 `[icon]`，每种一个默认色；不按类型分文件夹（用户 2026-10-11 定：真实项目的键按前缀组织，类型做标记就够）。**只用 SCAN，永远不用 KEYS**：每个 db 有扫描预算（`[redis] scan_keys`，默认 10000），超出时 db 节点的尾注是「已扫描 10000 / 1.2M，用 / 过滤」；侧栏 `/` 过滤改成服务端 `SCAN MATCH <模式>`，同一预算，模式不含 `*` 时两头补 `*`。命名空间节点的尾注是它下面扫到的键数，db 节点是 `DBSIZE`；不做 RedisInsight 的内存百分比（要全量扫）。命名空间节点上删除：确认「删除 N 个键？」后分批 `UNLINK`（走 `confirm_dangerous`）。单个键：删除、改名（`RENAME`，确认目标存在时覆盖）、设 TTL。
- **F16.3 键 tab**：打开键是一个 tab，`kind = "key"`，布局文件记 `connection`、`db`、`key`；标题是键名，长键名按现有 tab 标题截断。第一行是信息行：类型、TTL（人话化 `12m03s`，`R` 刷新，`-1` 显示「不过期」）、编码（`OBJECT ENCODING`）、内存（`MEMORY USAGE`，只在这里查）、长度；TTL（`EXPIRE` / `PERSIST`）、改名、删除都在这一行操作，`C-s` 写回。表格按类型：

  | 类型 | 列 | 翻页 | 写回 |
  |---|---|---|---|
  | string | 单格；合法 JSON 直接以值编辑器打开 | 无 | `SET` |
  | hash | Field / Value | 长度 ≤ `[redis] load_all`（默认 5000）全取、本地翻页 / 排序 / 过滤；超过用 `HSCAN`，只能「下一页」，过滤是 `MATCH` | `HSET` / `HDEL` |
  | list | Index / Value | `LRANGE` 按偏移 | `LSET`；删除是 `LSET` 哨兵 + `LREM`（Redis 文档的做法） |
  | set | Member | 同 hash（`SSCAN`） | `SADD` / `SREM`；改成员是 `SREM` + `SADD` |
  | zset | Member / Score | `ZRANGE` 按偏移，可按 score 倒序 | `ZADD` / `ZREM` |
  | stream | ID 加展平的字段（并集，缺的格子空） | `XRANGE` / `XREVRANGE` 按偏移 | 只 `XDEL` |
  | RedisJSON | 值编辑器 | 无 | `JSON.SET $` |

  查询条里的 WHERE 位置放 MATCH 模式；本地翻页的类型上多列排序、按值过滤、右键菜单照用。新增行（现有的新增行操作）是 `HSET` / `SADD` / `ZADD` / `RPUSH`。一批修改包进 `MULTI` / `EXEC`，对应 SQL 的每次保存一个事务；没有事务模式切换。值是合法 JSON 的格子行内高亮，`↵` 进值编辑器（写回走上表）。「以 JSON 查看」（`key.json`）：hash 整体变 object、list / set 变数组、zset 变 `[member, score]` 数组、stream 变 `{id, fields}` 数组，在值编辑器里只读看，复制为 JSON 同一份。键的 DDL 查看是重建它的命令（`HSET k f v …`、`SET k v EX ttl`），复用 ddlview。二进制值按 §8.1 的占位符。
- **F16.4 控制台**：Redis 连接上的 query tab 就是控制台：一行一条命令，`↵` 跑光标所在行，选区和全部同 query；引号按 redis-cli 的规则（`"…"` 带转义、`'…'`），`#` 开头的行是注释。高亮：命令名来自 `COMMAND`（连接时在 Meta 上取一次并缓存），字符串和注释沿用现有的色键。只读连接拒绝带 `write` 标记的命令，`@dangerous` 分类（FLUSHALL、FLUSHDB、CONFIG、DEBUG、SHUTDOWN …）走 `confirm_dangerous`，两份名单都来自 `COMMAND INFO`，不手写。回复摆成表：标量一格 `value`；数组一列 `value`；RESP3 的 map 是 Field / Value；等长的二维数组按列摆（`ZRANGE … WITHSCORES` 是 member / score）；更深的嵌套在格子里显示 JSON 并高亮；nil 是 NULL；错误进错误栏。整份回复在内存里，按现有 500 行翻页，「全部」不重跑。历史走 M14 F14.2 的 `history.jsonl`。
- **F16.5 补全与签名提示**：`COMMAND DOCS`（Redis 7 起）给命令名、子命令、参数签名和一句说明，连接时取一次，缓存在 Meta 上；Redis 6 没有 DOCS 时只补全命令名。签名提示用 F15.3 的浮层，当前参数高亮。键名补全：打字时在 Meta 上 `SCAN MATCH <前缀>* COUNT 100`，去抖；字段名：`HGET` / `HSET` / `HDEL` 的键已知且长度 ≤ `load_all` 时 `HKEYS`。补全排序照 §9.7（F12.6）。

不做（第二轮候选）：Pub/Sub tab、MONITOR、SLOWLOG、CLIENT LIST、内存分析、Cluster / Sentinel、按 score 范围过滤 zset、键的批量设 TTL。

任务文件的格式和状态约定见 [`../plan.md`](../plan.md)。
