# sqlmux 技术方案 · §14 配置与持久化

> 这是技术方案的一章，各章的索引和阅读规则在 [`../tech-design.md`](../tech-design.md)；章节编号（§N.M）全仓库通用，不随文件改。

## 14. 配置与持久化

所有 Unix 平台都遵循 XDG 规范：优先读 `$XDG_CONFIG_HOME`、`$XDG_STATE_HOME`、`$XDG_DATA_HOME`，未设置时分别用 `~/.config`、`~/.local/state`、`~/.local/share`。macOS 上也用 `~/.config`，不用 `~/Library/Application Support`，这是终端工具的惯例。

**文件格式**：

- **配置文件用 TOML**。配置是用户手写的，需要能写注释；键位的分层表（如 `[keys.grid]`、`[map.query.normal]`）写成 TOML 的表最自然。lazysql、Helix、Alacritty 用的也是 TOML。
  - 不选 YAML：缩进容易写错，而且有隐式类型转换，比如 `no` 会被当成 false。
  - 不选 JSON：不能写注释。
- **state 文件用 JSON**。它只由程序读写，不需要用户手工编辑。

| 文件 | 写入方 | 内容 |
|---|---|---|
| `~/.config/sqlmux/config.toml` | 只有用户写 | 常规选项、主题 |
| `~/.config/sqlmux/keymaps.toml` | 只有用户写 | 键位 `[keys.*]`、leader、映射 `[map.*]`（M5 F5.9 起从 config.toml 挪出来；`sqlmux keys --format toml` 生成一份完整的参考：和默认相同的写成注释行，用户改过的写成生效的行，没有改动时全是注释）。config.toml 里还写着 `[keys]` / `[map]` 时启动报错，提示挪到 keymaps.toml |
| `~/.config/sqlmux/connections.toml` | 用户手写，也由应用追加（M5 F5.1 新建连接） | 连接定义 |
| `~/.config/sqlmux/themes/*.toml` | 只有用户写 | 自定义主题 |
| `~/.local/state/sqlmux/state.json` | 由应用写入 | 最近使用、每张表的 WHERE 历史与收藏、快速 SQL 历史、query 文件最后用的连接和 schema（M5） |
| `~/.local/share/sqlmux/queries/` | 由应用写入 | query 的 SQL 文件 `query_<n>.sql`（M5 起不再按连接分目录） |
| `~/.config/sqlmux/layouts/*.toml` | 用户手写，或「保存布局为…」写入 | 命名布局（M6 F6.5，格式见下面「布局文件」） |
| `~/.local/state/sqlmux/last.toml` | 由应用写入 | 退出时的工作现场，`sqlmux` 不带参数时恢复（M6 F6.5） |

- 连接定义单独放一个文件，是因为应用改写 TOML 时会丢掉注释，所以不能去改用户手写的 config.toml。
- `name`、`engine`、`dsn` 必填，`name` 不能重复，`read_only` 可选（默认 false，M5 F5.3）。`name` 是 state.json 里的键（M5 起不再用作 query 文件的目录名，§11），规则照旧：不允许含 `/`、`\`，也不能以 `.` 开头，否则启动报错。`engine` 在 M1 只接受 `postgres`，其他值报错「目前只支持 postgres」。
- **启动时找不到连接**：`sqlmux <名字>` 找不到这个名字时，在终端打印错误就退出（退出码 1），不进入界面，错误里写明配置文件的路径并列出已有的连接名。**没有 `connections.toml` 或文件里没有连接时**（M7 F7.4，用户 2026-10-05 定；原来也是报错退出），照常进入界面：一个 session、默认布局、树是空的（只有「还没有连接，按 ＋ 新建」那一行），并直接打开新建连接表单；这时 `last.toml` 存在也当没有，直接进零连接状态，不报错、不改名；需要连接的命令（开表、新 query、快速 SQL、新建 session、切换连接、打开布局）在零连接时 toast「还没有连接，先新建一个」，一个都不能 panic（有测试把每条命令都跑一遍）；保存第一个连接后连上并展开（§7.8「新建连接表单」）。`esc` 关掉表单也能用：空的工作区可以看引导页、改配置，再从 `＋` 或面板新建。启动时的连接连不上（比如密码错误）也一样，打印驱动返回的错误后退出；pgconn 逐个地址尝试时会打出多行（标题一行，每个地址一行），照样输出，不压成一行。其他连接连不上不影响启动（§8.2）。
- **新建连接**（M5 F5.1，§7.8「新建连接表单」）：在 connections.toml 末尾追加一段 `[[connection]]` 文本，不重新编码整个文件，用户的注释都保留；文件不存在就新建，权限 0600。
- **修改和删除连接**（M8 F8.16，§7.8「连接管理」）：同样不重新编码整个文件。修改只改写这一个 `[[connection]]` 块里的 `key = value` 行（改值、在块末加新键、删掉清空的键），删除去掉这一块和它上面紧挨着的注释行；其余字节不动。块从表头到它最后一个键行（加后面的空行）为止，后面单独成段的注释不属于它；写完把这一块读回来和要写的连接比对，不一致就拒绝并提示手工改（多行值这类写法）。写盘走临时文件 + rename，保留原权限和符号链接。顺序：预检文件 → 存钥匙串 → 写文件 → 才删旧钥匙串项；改名时我们模板的 `password_cmd` 跟着搬到新名字。已连上的连接的 `read_only` 等下次连接再变。有 session 正在用的连接不能删。改了名字时 state.json 里按连接名记的键跟着改，布局文件里的 `connection = "旧名"` 不改（恢复时按「没有这个连接」跳过）；删除时只删我们自己模板的 `password_cmd` 对应的钥匙串项，用户自己写的命令不碰。
- **分组**（M10 F10.6）：`[[connection]]` 里可选的 `group = "prod"`，树和连接管理按它分段（§7.8「分组」）；表单和连接管理按上面的块级改写加 / 删这一行。
- **界面语言**（M6，用户要求）：`language = "en" | "zh"`，默认 `en`（M7 F7.6，用户 2026-10-07 定：开源面向英语用户；M6 时默认 `zh`），写错启动报错。
  - 界面上所有文案都走一处翻译：toast、提示、确认框、查询条与结果区的文字、状态栏的模式附加信息、面板的范围标签与底栏、Action 的标题、引导页、日志、错误说明（数据库返回的原文不翻译）。
  - 做法照 gettext：中文原文就是 key，`en` 是一张「中文 → 英文」的表，找不到就显示原文。这样现有代码只需要把字符串包一层，不用先给每条文案起名字。带参数的文案用占位符（如「已保存 %d 行」）。
  - 命令面板按当前语言的标题搜索，action id 照旧可以搜。
  - 键位文字不属于文案，照旧从 keymap 读（§6.7）。
  - 读到 config.toml 之前显示的文字一律是英文（F7.6）：`sqlmux` 命令行的用法说明、flag 说明和错误、解析 config.toml / keymaps.toml 的错误（语言就是正在读的内容之一），直接用英文写，不走 `T`；`i18n` 没 `Set` 之前就是英文，所以主题文件的错误照旧走 `T`，启动时读到的也是英文，运行中 `theme.switch` 读坏文件时跟着语言；config 读完之后的启动错误、`sqlmux keys` 的输出（含 `--format toml` 里的注释）跟着语言（F6.4 定）。
  - key 仍是中文原文（F7.6 时定）：改成英文 key 要动 300 多处文案且不改任何行为，不值；代价是读代码的人看到的是中文 key，`en` 表就是对照。
  - 测试：单测扫一遍所有包，找出没走翻译的中文字面量；`en` 表里缺的条目也由单测报出来。单测和 golden 照旧按 `zh` 断言（各包的 TestMain 里 `Set("zh")`），`en` 的几张 golden 照旧，默认是英文这件事由 i18n / config / cmd 的单测直接盖；e2e 的 `lib.sh` 在每个脚本的 config.toml 里补 `language = "zh"`（脚本自己写了就不补，同 `confirm_dangerous`），已有的中文断言不动，默认英文和 `language = "zh"` 各由一个脚本验证。
- **布局文件**（M6 F6.5，用户 2026-09-30 提出，细节 2026-10-05 按 worker 的提议定）：
  - **格式**：TOML，`version = 1`；每个 `[[session]]` 记 `name`、`connection`（启动连接）、`active`；每个 `[[session.window]]` 记 `name`、`layout`（分割树，TOML 表：`split = "h" | "v"`、`ratio`、`a`、`b`，叶子是 `pane = <id>`；程序写成子表 `[session.window.layout.a]`，手写成内联表也能读）、`focus`、`zoom`、`sidebar_open`、`sidebar_width`、`result_height`；每个 `[[session.window.pane]]` 记 `id` 和它的 tab：表 tab 是 `kind = "table"`、`connection`、`schema`、`table`、`where`、`order`、`desc`、`limit`、`page`、`cols`（隐藏的列）、`order_cols`（列的显示顺序，M10 F10.3；刷新后新出现的列排最后，已删除的列去掉）、`transpose`；query tab 是 `kind = "query"`、`n`、`connection`、`schema`；再记当前 tab。不存：结果集、正在跑的查询、未保存的单元格修改（退出前本来就要确认）、引导页。
  - **恢复**：第一个 session 的启动连接照现在连（连不上就是启动错误）；其余被 tab 用到的连接和其他 session 的启动连接都在启动时后台连（Cmd）；恢复出来的表 tab 先挂起不取数，等那个连接的 catalog 到了，表在 catalog 里的才取数、不在的关掉并 toast；连接失败就关掉它的全部挂起 tab，每个连接一条 toast（启动连接也走这一条）。query tab 先建在所在 session 的启动连接上，布局里的连接和 schema 作为「想要的」，连上后像标题下拉切换那样挪过去。catalog 里没有的表、读不了的 query 文件（读出错；文件不存在就当空 query 打开，没打过字的 query 本来就没有文件，§11）、不存在的连接：跳过这一项、toast 一条，其余照建。布局里的 query 编号已经被另一个正在跑的 session 用着时，换下一个空号，打开新编号自己的文件，不复制、不覆盖任何文件（原文件本来就在占着它的 session 里开着）。手写的布局缺字段时给缺省值：query 不写 `connection` 用这个 session 的启动连接，表不写 `schema` 用那个连接的 home schema，和 `sqlmux <连接名>` 启动时一致。同一个 pane 里同一张表写了两次只留第一个、不 toast（F3.37）；`[[pane]]` 里有、分割树里没有的 pane，它的 tab 一概不建。
  - **保存**：`last.toml` 只在正常退出时写（`:qa`、两次 `C-c`、关掉最后一个 session），不是每次改动都写；进程被杀掉就丢掉这次的布局改动（`ponytail:` 标出）。不管这次是怎么启动的，退出都覆盖 `last`，同 DataGrip。
  - **命令**：`layout.save`「保存布局为…」在状态栏的单行输入里要名字（同 `window.rename`，提示和输入框只占左边、右边照常画模式块，M8 F8.11 起三处统一；面板的输入行是搜索框兼快速 SQL，不再给它加一种模式），`↵` 写 `layouts/<名字>.toml`，同名覆盖，`esc` 放弃；名字含 `/`、以 `.` 开头、或是保留名 `last` 时 toast 拒绝（`last` 指 last.toml，F6.5 审查时定）。`layout.open`「打开布局」是下拉框（同 `theme.switch`），列出 `layouts/*.toml` 和 `last`，选中后替换正在跑的全部 session，有未保存的修改或没提交的事务先按 §10.5 确认（动词「打开」）；确认后在 Update 里同步读布局、建好 session，所有启动连接都在后台连，没有「拨号期间旧 session 还能操作」的窗口（审查时发现那时打的字会丢、还会把旧内容写进新 query 的文件）；第一个 session 的连接连不上也是 toast、丢掉它的 tab，和别的连接一样；`--layout` 和它共用读布局、建 session 的代码，但启动时连不上照上面「恢复」的规则，是启动错误、在终端里报出来退出，和 `sqlmux <连接名>` 一致（F6.5 复审时定）。
  - **启动**：`sqlmux` 不带参数时有 `last.toml` 就恢复，没有照默认布局；`sqlmux <连接名>` 用这个连接开默认布局、不恢复 `last`；`sqlmux --layout <名字或路径>` 打开这个布局；连接名和 `--layout` 同时给时报错退出「连接名和 --layout 只能给一个」。`last.toml` 读不了：toast、默认布局，坏文件改名为 `last.toml.broken`。
- **state.json**（M1 F1.5 起）：路径 `$XDG_STATE_HOME/sqlmux/state.json`，读写都在 `config` 包里。
  - 内容：`recent` 是面板的最近使用，记成 `{"kind": "table", "id": "play/public.t_order"}`，kind 为 window / pane / table / command，M5 起表的 id 带连接名，旧的不带连接名的条目读到时丢掉；`queries` 记每个 query 文件最后用的连接和 schema，如 `{"query_1": {"conn": "play", "schema": "public"}}`，换连接或 schema 时写入（M5 F5.11）；`tables` 的键是 `"<连接名>/<schema>.<表>"`（不同连接可能有同名表），每张表下存 history 和 favorites。
  - 启动时读一次；每次改动（执行 WHERE、收藏、面板运行）都写一次。写入：在 Update 里序列化，在 Cmd 里写临时文件再 rename，权限 0600；用互斥锁和版本号，丢掉比已写入版本更旧的快照，避免两个 Cmd 并发时旧内容覆盖新内容。
  - 读取失败（文件损坏）：先把坏文件改名为 `state.json.broken`，再用 toast 报错，当作空状态继续，不退出。改名是为了下次写入不会把用户的历史直接覆盖掉。

```toml
# config.toml
theme        = "tokyonight-storm"
icons        = "nerd"            # nerd | ascii
vim          = true              # false 时 query / WHERE 是普通编辑器，表格、树多一层方向键（§6.9，M6）
language     = "en"              # en | zh，界面文案的语言（M6；默认 en，M7 F7.6）
timeoutlen   = 1000
result_height = 0.4              # 底部结果区默认所占的高度比例
formatprg    = ""                # 例如 "pg_format -"；为空时使用内置的 sql-formatter
keyword_case = "lower"
tab_width    = 2                 # query 缩进与格式化共用
autopairs    = true              # 输入括号、引号时补上另一半（§7.9）
confirm_dangerous = true         # 执行没带 WHERE 的 delete / update、drop、truncate，以及保存里有删除行时先确认（§13，M6）
border       = "single"          # single | rounded | double | bold | hidden，pane 和浮层的边框（M8 F8.10，取值同 lazygit；Unicode 没有粗圆角，所以粗和圆角二选一）

[tree]                           # M8 F8.22
width        = 32                # 侧栏宽度（列，含边框）

[grid]                           # M8 F8.5 / F8.8
full_width   = 22                # 期望宽度不超过它的列不收缩（22 放得下带整点时区的 timestamptz）
order        = "asc"             # asc | desc，新打开的表默认排序的方向
transpose    = false             # 新打开的表是否转置（关掉再重开的 tab 保留原来的状态，同 WHERE / ORDER / LIMIT）

[query]
max_rows = 500                   # M10 F10.4
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

[map.query.normal]             # 只在 query
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
group        = "prod"            # optional (M10 F10.6)
```
