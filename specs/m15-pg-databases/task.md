# M15 PG 数据库层级 · 任务清单

- **状态**：**进行中**（第一批 F15.5–F15.9 passed 2026-10-13，0b4a268；第二批 F15.1 passed 2026-10-13，e056ebf，全量 197 个脚本 3848 条、红的只有 f11.2 的 4 条 brew 环境问题，reviewer 退回过一次（面板条目的 id 不带库、在别的库时开错同名表，e25ca90 修）；第三批 F15.3 / F15.10 / F15.11 passed 2026-10-14，1ae0694，全量 199 个脚本 3964 条、红的只有 f11.2 的 brew 和共用 PG 上残留的 sqlmux_e2e_* 库撑长了树；第四批 F15.2 / F15.4 passed 2026-10-14，3e4318b，全量 204 个脚本 4060 条、红的只有 f11.2 的 4 条 brew；四个节点都过，等用户验收；2026-10-11 用户规划时定方向，排在 M14 之后）。
- **目标**：PG 的树从 连接 → schema 改成 连接 → 数据库 → schema，同 DataGrip；SSH 隧道；query 的上下文提示与值补全（用户后续补充的编辑体验需求也归到这里）；状态栏的随机提示；用户 2026-10-13 验收 M14 时的五条（F15.5–F15.9：值编辑器宽度、ORDER 框宽度、flash 无匹配即退出、snippet 式占位符、`gj` 与搜索高亮）。
- **范围**：tech-design §5（`Conn` 按数据库分 Meta / Main）、§7.8（数据库节点、特性提示）、§8.4（`Databases`、SSH 隧道）、§11（签名提示与值补全）、§14（connections.toml 的 ssh 字段、layout / state 的 `db`）。
- **完成标准**：F15.1–F15.11 passed；tester 在最后一个 commit 上跑完整回归；用户验收通过后打 tag `m15`。
- **依赖**：M14 passed。
- **审查节点**（worker 2026-10-13 起草、决策者定）：1 = F15.5–F15.9（已过）；2 = F15.1（a、b 两个 commit，单独一批）；3 = F15.3、F15.10、F15.11（编辑体验）；4 = F15.2、F15.4。

## F15.1 PG 数据库层级 · 状态：passed（e056ebf；e2e 3163b9f）

用户 2026-10-11：同 DataGrip，PG 一个连接只能看一个库，树从 连接 → schema 改成 连接 → 数据库 → schema。做法（worker 2026-10-13 起草、决策者同意，§5）：不改 Worker / Meta / Main 的模型，每个（连接, 数据库）一对 `Meta` / `Main`，事务模式随之按库。

- **涉及**：`internal/db`、`internal/db/postgres`（`Databases`、DSN 改库）、`internal/config`（`Database` 覆盖字段、layout / state / history 的 `db`）、`internal/app`（树、query 的库选择、标题、state 键）

**开发**（§5、§7.8「数据库节点」、§8.4、§14）
- [x] F15.1a 数据层：`Engine.Databases(ctx, c)`（PG：`pg_database` 去模板、去 `datallowconn = false`，再 `has_database_privilege(datname, 'CONNECT')` 过滤；MySQL / SQLite 返回 nil）；`config.Connection` 加不落盘的 `Database` 覆盖字段；`Conn` 加 数据库名 → 子 `*Conn` 的表，展开时才建、才连；layout / last.toml、state.json 的表键、history.jsonl 加 `db` 字段，和 DSN 的库一样时省略，旧文件照读。
- [x] F15.1b 树和用到它的地方：连接 → 数据库 → schema（PG 才有；MySQL、SQLite 不变）；DSN 里的库排第一、默认展开，别的库点开才连；节点 id 加 `/db:<name>`；展开状态、layout 恢复、`tree.refresh`（光标所在库）、右键菜单、表 tab 标题（非默认库才带库名）和 state 键、query 标题的 `DB` 字段、状态栏的地址都带库名；query 切库放进 `query.schema` 的下拉（`db.schema ▾`）第一级，不另开下拉。

**验收**
- [x] 集成（PG）：列表不含 template*、没 CONNECT 权限的库被滤掉；连另一个库后 `current_database()` 对。config 往返和旧文件兼容的单测；MySQL / SQLite 列表为空。
- [x] golden：两个库、一个展开一个没展开的树；MySQL / SQLite 的树 golden 不变。
- [x] 单测：展开才拨号（假引擎计 Connect 次数）；layout 里存了非默认库的 query 重启后回到那个库。
- [x] e2e：树里展开第二个库、打开其中一张表、query 里 `select current_database()` 是它；query 的下拉里切库。

## F15.2 SSH 隧道 · 状态：passed（3e4318b；e2e 5259415）

用户 2026-10-11 定方向；做法（worker 起草、决策者同意，§8.4「SSH 隧道」）：本机 `ssh -W` 子进程当 `net.Conn`，不用 `x/crypto/ssh`，不支持交互式口令（文档写「先 `ssh-add`」）。

- **涉及**：`internal/db`（拨号）、`internal/db/postgres`、`internal/db/mysql`、`internal/config`（`ssh` 字段）、`internal/app`（表单）

**开发**（§8.4、§14）
- [x] connections.toml `ssh = "user@host"`（端口、密钥、ProxyJump 走 `~/.ssh/config`）；每次拨号起 `ssh -W host:port -o BatchMode=yes -o ExitOnForwardFailure=yes user@host`，stdin / stdout 接 pgx 的 `DialFunc` 和 mysql 的 `RegisterDialContext`；连接关了进程退；ssh 失败时 stderr 原文进连接错误。命令行的临时连接不支持；SQLite 忽略。
- [x] 连接表单加 `ssh` 一项（高级里）。

**验收**
- [x] 单测：拨号函数用 PATH 里的假 `ssh` 脚本（回显、断开、退出码非零时的错误原文）。
- [x] 集成：假 ssh 脚本把 stdin / stdout 接到 docker 里的 PG 端口（`nc`），`SELECT 1` 通；连接 spec 往返；表单里能写能读。
- [x] 真实 ssh 只在用户机器上手动验收一条，CI 不测。

## F15.3 签名提示 · 状态：passed（1ae0694；e2e 83bfd24）

用户 2026-10-11：INSERT 的 VALUES 里不知道第几个值对应哪个字段、什么类型。细节在 §11「签名提示与值补全」。F15.3 只做上下文识别和提示浮层；值补全和小项拆成 F15.10、F15.11（worker 起草时拆，编号追加、不动 F15.4）。

- **涉及**：`internal/sqlkit`（`ValueContext`）、`internal/ui`（提示浮层）、`internal/app`（query）、`internal/config`（`[query] hints`）

**开发**（§11）
- [x] `sqlkit.ValueContext(sql, offset)`：表、列清单、第几个、种类（insert / update / where / call），用 `CompletionContext` 同一套扫描器。
- [x] 提示浮层：光标行上一行（第一行时下一行）一行，当前字段高亮、两边 `dim`、放不下两头 `…`；光标移动跟着换，出上下文消失；`[query] hints = false` 关。

**验收**
- [x] sqlkit 表驱动单测：多行 VALUES 每个括号重数、`INSERT … SELECT` 不出、字符串 / 注释里的逗号不算、没列清单用 catalog 顺序、函数调用括号里取签名；mutation 逐条验。
- [x] golden：一个 INSERT 的、一个放不下的。
- [x] e2e：打 INSERT 到第二个值时提示行高亮第二列；`hints = false` 没有。

## F15.4 状态栏的随机提示 · 状态：passed（3e4318b；e2e 5259415）

用户 2026-10-11 定；细节在 §7.8「特性提示」。

- **涉及**：`internal/ui`（状态栏）、`internal/app`（清单、`tips.next` / `tips.show`）、`internal/i18n`、`internal/config`（`[ui] tips`）

**开发**（§7.8、§14）
- [x] 清单在代码里（Action ID + 一句话 i18n，`{key}` 从 keymap 读，解绑的跳过）；启动随机、5 分钟一条（`tea.Tick`）、不重复到轮完、单击换下一条；`tips.next`、`tips.show`（列表浮层复用命令面板的列表，`↵` 执行）；`[ui] tips = false`。

**验收**
- [x] 单测：轮转不重复、轮完重来、解绑跳过、改键跟着变；每条提示的 Action 存在（遍历清单对 Action 表）；i18n 中英文齐全（现有 i18n 测试扫）。
- [x] golden：状态栏带提示、窄到让位。

## F15.10 值补全 · 状态：passed（1ae0694；e2e 83bfd24）

- **依赖**：F15.3
- **涉及**：`internal/app`（补全）、`internal/sqlkit`

**开发**（§11「值补全」）
- [x] 在 F15.3 的列上下文里弹补全：枚举值（带引号）、布尔 `true` / `false`、可空 `NULL` 和有默认值 `DEFAULT`（这两项只 INSERT / UPDATE，条件里不给）、时间 `now()` / `CURRENT_TIMESTAMP`，候选来自单元格编辑选项浮层的同一份 catalog；其余列不弹。

**验收**
- [x] 单测：每类候选和不弹的。
- [x] e2e：枚举列的 VALUES 里弹出枚举值，`Tab` 补全。

## F15.11 补全的同类小项 · 状态：passed（1ae0694；e2e 83bfd24）

- **依赖**：F15.8（snippet）、F15.3
- **涉及**：`internal/app`（补全）、`internal/sqlkit`

**开发**（§11「同类小项」、§9.7）
- [x] `INSERT INTO t ` 后的补全给 `(全部列) VALUES ()` 骨架，VALUES 括号里一个 tabstop，列清单是普通文字。
- [x] `UPDATE t SET` 后的列补全只列 UPDATE 和 SET 之间第 0 层的表（MySQL 多表 UPDATE 都列）。
- [x] 补全插入的标识符需要引号时加引号，去掉 §9.7 的 `ponytail:` 上限；保留字每个方言一份表（§9.7「接受」：PG 不变，MySQL 8.4 `RESERVED = 1`，SQLite 的关键字表）。

**验收**
- [x] 单测三项。
- [x] e2e：INSERT 骨架后打值；大小写混合的列名补全后带引号。

用户 2026-10-13 验收 M14 时提的（决策者当天定细节，已写进设计章节；都不依赖 F15.1，建议排第一批）：

- **F15.5 值编辑器的宽度按内容**（状态：passed（0b4a268；e2e 0b4a268））（§10.3）：浮窗宽取最长一行（JSON 格式化之后）加边框和内边距，最少 pane 内容区的一半（最小 40）、最多 pane 内容区的宽；左对齐光标所在格，右边出界整体左移；原来固定一半，长 JSON 的行被截掉看不全。验收：单测宽度的三档（短文本一半、长行撑开、超长到 pane 宽）；golden 一个长 JSON 的。
- **F15.6 ORDER / COLS 框的宽度按内容**（状态：passed（0b4a268；e2e 0b4a268））（§7.8「ORDER」）：`ui.Dropdown.Box` 不再让提示行把框撑宽（现在 `max(w, hintRowWidth+4)` 把六项提示的 ORDER 撑到整个 pane）；提示行放不下折成两行，再放不下按值编辑器提示行的规则丢末尾的项。看一眼别的用这个框的下拉有没有被同一句撑宽。验收：golden ORDER 框和 COLS 框都按内容、远窄于 pane（ORDER 比 COLS 宽序号标记那几列，不是同宽，tester 试跑时改）、提示行两行；单测提示行的折行和丢项。
- **F15.7 flash 跳转没有匹配就退出**（状态：passed（0b4a268；e2e 0b4a268））（§6.10）：模式打到一个匹配都没有时直接退出 JUMP、光标不动、这个字符丢掉；原来留在全黑的遮罩里，删字符才回来、看不见已打的前缀。验收：单测；e2e 打一个屏幕上没有的字符后模式块回到 NORMAL、遮罩没了。
- **F15.8 snippet 式的占位符**（状态：passed（0b4a268；e2e 0b4a268））（§11「snippet 与 Select 模式」、§7.8「模板」）：编辑器加最小的 snippet 和 Select 模式：插入带 tabstop 的文字时第一个在 Select 模式下选中、打字即替换，`Tab` / `S-Tab` 跳下一个 / 上一个，剩下的 tabstop `visual` 底；结构模板的占位符改走它（原来 VISUAL 选中第一个、要按 `c`）；F15.3 的 `(全部列) VALUES ()` 骨架也用它。照 nvim 的 `vim.snippet` 和 `:help Select-mode`，拿不准的以 nvim 实测为准。验收：单测（第一个选中、打字替换、Tab / S-Tab、最后一个结束、离开范围结束、补全开着时 Tab 归补全）；e2e 右键「加索引…」后直接打字、`Tab` 到 `<column>` 再打字，语句正确。用户顺带提的「改表、改列的表单」不排，以后再说。 已知、不排：R 模式里改写越过下一个 tabstop 的开头时那个 tabstop 缩成一个点，nvim 会保留剩下的部分（审查时说明是改动前就有的）。
- **F15.9 JSON 视图的键和编辑器的搜索高亮**（状态：passed（0b4a268；e2e 0b4a268））（§7.6、§6.4、§11「搜索」、§7.7）：`gJ` 改成 `gj`；JSON 视图的翻页和刷新沿用表格的键（`[keys.grid]` / `[keys.result]` 里绑到 `grid.page.*` / `result.page.*` / `grid.refresh` 的，用户改成 `<` / `>` 也生效），`json.page.*` / `json.refresh` 三个 Action 去掉，提示行的键位跟着读；编辑器（query、值编辑器、JSON 视图）的 `/` `?` 回车后所有匹配 `search` 底、当前匹配 `search_current` 底，`n` / `N` 跟着换，NORMAL 下 `esc` 清掉；回车和 `n` / `N` 后命令行留着显示模式、右端 `[1/2]`、绕回加 `W`，下一个不是 `n` / `N` 的键消失，计数写法照 nvim search.c `update_search_stat`。主题加 `search`、`search_current` 两个键（没写时退回 `visual` / `yank`），`sqlmux keys` 和 default.toml、README 的键位表跟着改。验收：单测高亮区间和计数（含绕回、超过 99）；golden 一个两处匹配的；e2e query 里 `/` 后看见两种底色和 `[1/2]`，JSON 视图里用户的 `<` `>` 翻页。

任务文件的格式和状态约定见 [`../plan.md`](../plan.md)。
