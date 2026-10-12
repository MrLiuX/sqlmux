# M12 表格日常 · 任务清单

- **状态**：**完成**（节点 3 F12.7–F12.12 passed 2026-10-12，6472fcd，和 M13 一起的完整回归 170 个脚本 3455 过、4 败全是 f11.2 拉不到 docker 镜像的环境问题；等用户验收；用户 2026-10-11 验收 M12 时提了 4 条加 3 条，追加为 F12.7–F12.12，节点 3；节点 2 F12.3–F12.6 passed 2026-10-11，9b6179f，全量 155 个脚本 3387 项 0 败；等用户验收；节点 1 F12.1–F12.2 passed 2026-10-11，a2f6aa8，全量 3242 项 0 败；已知上限：1MB 级的 JSON 值在值编辑器里 `j` 约 32ms 一步、打字 50ms，几百行的正常，不设门槛。原文：2026-10-11 用户规划 M12–M14 时定；M11 F11.4 passed 之后开工）。
- **目标**：多列排序、值编辑器（长文本和 JSON）、按单元值过滤和按列排序加表格的右键菜单、导出 CSV / JSON；连接连不上不退出、表单的测试按钮、tab 移到相邻 pane；补全列表的排序。用户 2026-10-11 对照 DataGrip 提出，决策者分析后拆成 M12（表格日常）、M13（SQLite 与历史）、M14（PG 数据库层级）三个里程碑，一个 sprint 不超过两个审查节点。
- **范围**：tech-design §5（`Query.Order`、启动连接）、§6.4（`value` 作用域、`menu`）、§6.8（键位表）、§7.6（ORDER、表头、按值过滤、按列排序、右键菜单、导出、工具按钮）、§10（值编辑器）、§11（结果导出）、§14（布局文件的 `order`、启动时连不上）、§7.8（表单的测试、tab 移动）、§9.7（补全排序）。
- **依赖**：M11 F11.4 passed。
- **完成标准**：F12.1–F12.12 passed；tester 在最后一个 commit 上跑完整回归；用户验收通过后打 tag `m12`。
- **审查节点**：
  1. F12.2 之后：F12.1–F12.2 一批（多列排序、值编辑器）；
  2. F12.6 之后：F12.3–F12.6 一批（过滤 / 排序 / 右键菜单、导出、连接与 tab 的小项、补全排序）；
  3. F12.12 之后：F12.7–F12.12 一批（用户验收意见：表头排序箭头、值编辑器的位置和提示、ORDER 浮层、面板里的 query、`O` / `fv` / `fc` / `fe`、选择的颜色和 `V` 取消选择）。排在 M13 前面做。

任务文件的格式和状态约定见 [`../plan.md`](../plan.md)。

---

## F12.1 多列排序 · 状态：passed（a2f6aa8；e2e 1f1dfc9）

用户 2026-10-11：ORDER 只支持一个字段一个方向是严重问题，应该多个字段、各自指定方向，像 DataGrip 的 `ORDER BY source DESC, id asc`。

- **依赖**：F1.4（ORDER chip）、F6.5（布局文件）
- **涉及**：`internal/app`（grid、query 拼装、布局文件）、`internal/ui`（下拉框、chip）

**开发**（§5、§7.6「ORDER」、§14）
- [x] `Query.Order` 从一列改成有序的 `[]OrderCol{Col, Desc}`，空是默认排序；SQL `ORDER BY <列1> <方向1>, …, <行标识列>`，行标识列作 tiebreaker，列表里已有的不重复。
- [x] 下拉框：已在排序里的列前面标 `1↓` `2↑`；`↵` 保持原来单列的语义（唯一排序列上翻转，别的列上换成只按这一列）；`space`（`dropdown.toggle`，过滤框为空时）把光标所在列循环：追加（方向按 `[grid] order`）→ 翻转 → 去掉，下拉框不关；过滤框非空时 `space` 是字符。
- [x] chip 显示整个列表 `source ↓, id ↑`，末尾画第一列方向的图标按钮（点它翻转第一列）；放不下只显示前两列加 `…`（审查时定）。
- [x] 布局文件 `order` 改成 `[{col, desc}]` 数组，旧的 `order` 字符串加 `desc` 照读成一列，写回新格式。换了排序回到第 1 页，同原来。
- [x] 细节（已定）：默认排序的 chip 显示和「选行标识列得到降序」的规则不变（§7.6）。

**验收**
- [x] 单测：`space` 两列后 SQL 是 `ORDER BY source DESC, id ASC`（id 是行标识列时不再追加）；`↵` 在别的列上清掉列表只剩这一列；`space` 第三次去掉；旧布局文件读成一列、写回是数组。
- [x] golden：两列的 ORDER chip。
- [x] e2e：`go`、移到 source、`space`、移到 id、`space` `space`、`esc` 后第一行是 source 最大、同 source 里 id 最大的记录（PG 和 MySQL 各一次）。

## F12.2 值编辑器 · 状态：passed（a2f6aa8；e2e 1f1dfc9）

用户 2026-10-11：长文本和 JSON 看不见，要像 DataGrip 的值编辑器：弹窗里支持 vim、修改、格式化，JSON 自动格式化并高亮；和 query 的编辑器差不多。记录视图不另做，转置视图 `T` 就是。

- **依赖**：F3.1（编辑器）、F9.8（DDL 浮窗）
- **涉及**：`internal/app`（grid、浮窗）、`internal/editor`（只读模式、JSON 着色）、`internal/ui`、`internal/keymap/default.toml`

**开发**（§10「值编辑器」、§6.4 `value`）
- [x] `e`（`grid.value`「在值编辑器中打开」，grid 和 result）打开居中浮窗，宽高同表 DDL 浮窗，标题 `<列名> · <类型>`，内容是 `internal/editor` 的编辑器，作用域 `value`：`C-s` 写回并关闭，其余交给 vim；`[map.normal]` / `[map.visual]` 照常。模式块显示编辑器的模式。
- [x] JSON：列类型 json / jsonb / json，或文字以 `{` / `[` 开头且 `json.Valid`，打开时格式化成 2 空格缩进；着色用一个小的 JSON 词法（放 `internal/sqlkit/json.go`，和 SQL 词法同包，不新增目录；键名 `sql_column`、字符串 `string`、数字 `number`、true / false / null `keyword`、标点 `sql_operator`）；`gq` 仍是操作符（`gqq` / `gq{移动}`），不论范围都格式化整个值，不合法 toast「不是 JSON」。
- [x] 写回：`:w` 记成这一格的修改（规则同行内输入框：没改不记；空是空字符串），不关；`C-s` / `:wq` 写回并关；`esc`（NORMAL）/ `:q` 不写，有改动先确认「放弃修改？」。JSON 列写回原文含缩进。
- [x] 只读：进不了编辑的格和结果 tab 也能开，照 nvim `nomodifiable`：改动的键统一拦在一处、toast「只读」；VISUAL 下被拒的操作回 NORMAL、光标到选区起点（审查时定，加 `:set noma` 生成的 nvim 用例）；NORMAL 下不移光标、`U` 也 toast 是有意的差异，用例里注明。鼠标同 query 编辑器（点击、拖选、滚轮）；`vim = false` 时 `C-c` / `esc` 先取消选区再关。
- [x] 细节（已定）：不做 DataGrip 的「设置高亮显示语言」，只认 JSON；不做记录视图。

**验收**
- [x] 单测：json 列打开后文字是格式化的 JSON、着色的 token 类型正确；`:w` 后格的修改是编辑后的文字；没改 `:q` 不留修改；只读格里 `x` 不改文字且 toast；`gq` 对非 JSON 不动。
- [x] golden：JSON 值的浮窗。
- [x] e2e：表格上 `e`，`gqq`，`:wq`，`C-s` 后库里的 json 列是编辑后的值（PG 的 jsonb 会规范化，比对解析后的值）。

## F12.3 按值过滤、按列排序、表格右键菜单 · 状态：passed（9b6179f；e2e e0c1838）

用户 2026-10-11：DataGrip 列上右键按单元值过滤要做；每列的漏斗和上下按钮直接按列排序要评估。决策者：漏斗不做（右键菜单加 WHERE 就够），表头点击排序和右键菜单做。

- **依赖**：F12.1、F8.23（树的右键菜单）、F10.3（COLS）
- **涉及**：`internal/app`（grid、where、菜单）、`internal/ui`（表头命中、菜单）、`internal/keymap/default.toml`

**开发**（§7.6「表头」「按单元值过滤」「按列排序」「右键菜单」、§6.8）
- [x] `*`（`grid.filter.cell`「按这个值过滤」）：`col = <字面量>`（按列类型加引号和转义，NULL 是 `IS NULL`）追加进 WHERE（已有条件 ` AND ` 接），相同条件不重复，重新取数，进 WHERE 历史。结果 tab 不做。
- [x] 表头命中目标 `head`：单击只按这一列 → 翻转 → 默认；`S-click` 同 ORDER 下拉的 `space`；转置下不响应。键：`gs`（`grid.order.cycle`「按这一列排序」）、`gS`（`grid.order.add`「加入排序」）。
- [x] 右键菜单（复用树的 `menu` 作用域和画法）：格上是 值编辑器 / 按这个值过滤 / 排序 ↑ / 排序 ↓ / 加入排序 / 隐藏这一列 / 复制这一格 / 复制行为 JSON / 导出…；表头上是 排序 ↑ / ↓ / 加入排序 / 隐藏这一列；结果 tab 的格只有 值编辑器 / 复制 / 导出。右键先把光标移到那一格。
- [x] 细节（已定）：不做 DataGrip 的本地筛选（不发 SQL 的过滤）；`S-click` 终端不传 shift 位时就没有这个功能，键照样能用。

**验收**
- [x] 单测：字符串、数字、NULL 三种格的 `*` 拼出的 WHERE；已有条件时的 ` AND `；重复不加；表头单击三次回默认；`gS` 两列。
- [x] golden：表格右键菜单。
- [x] e2e：格上 `*` 后行数变成那个值的行数；表头单击后第一行是最小值；右键菜单「隐藏这一列」后表头少一列。

## F12.4 导出 CSV / JSON · 状态：passed（9b6179f；e2e e0c1838）

用户 2026-10-11：表顶部要一个导出按钮，先支持 CSV 和 JSON（DataGrip 的导出对话框有提取程序和输出文件）。决策者：下拉选格式和范围，文件写当前目录，不做剪贴板。

- **依赖**：F3.x（结果导出 CSV）、F9.5（`Y` 的 JSON 规则）
- **涉及**：`internal/app`（导出、流式取数）、`internal/db`（不带 LIMIT 的流式读）、`internal/ui`（按钮）

**开发**（§7.6「导出」「工具按钮」、§11）
- [x] 查询条加导出按钮（`export` 图标，转置那一组），`ge`（`grid.export`「导出…」）和右键菜单都开同一个下拉：`CSV · 当前页`、`CSV · 全部`、`JSON · 当前页`、`JSON · 全部`；结果区的导出按钮和 `result.export`（标题改「导出…」）也开它。
- [x] 当前页写内存里这一页；「全部」用同一条取数语句不带 LIMIT / OFFSET 流式读写文件（列同显示的列和顺序），`C-c` 取消并删掉半截文件；结果 tab 的「全部」去掉自动 LIMIT 重跑，自己写了 LIMIT 的和快速 SQL 的就是当前页。
- [x] 文件名 `<表名>.csv` / `.json`、结果 `query_1-42.csv`（第 2 页起 `-p2`），已存在加 `-2`；完成 toast 路径和行数。CSV 同 §11，JSON 是对象数组、每个对象同 `Y`。
- [x] 细节（已定）：不做剪贴板、TSV、INSERT、XML；不做导出对话框。

**验收**
- [x] 单测：四个选项的文件内容（CSV 表头、NULL 为空；JSON 的类型和键顺序）；已存在时 `-2`；隐藏的列不导出。
- [x] 集成（PG 和 MySQL）：1203 行的表「全部」导出 1203 行，和 `R` 看到的顺序一致。
- [x] e2e：`ge` 选 `JSON · 全部`，toast 路径，文件能 `python3 -m json.tool`。

## F12.5 连接连不上不退出、测试按钮、tab 移到相邻 pane · 状态：passed（9b6179f；e2e e0c1838）

用户 2026-10-11：`sqlmux` 启动时 play 连接超时直接报错退出（`failed to connect … context deadline exceeded`），应该进界面再 toast，让用户能在树里建新连接；新建连接要有 DataGrip 那样的测试按钮；要 `SPC wn` / `SPC wp` 把当前 tab 移到下一个 / 上一个 pane；dsn 的格式怕新用户看不懂。

- **依赖**：F6.5（布局恢复）、F8.16（连接管理）、F5.2（pane 顺序）
- **涉及**：`cmd/sqlmux`、`internal/app`（启动、form、tab）、`internal/db`（测试连接）、`internal/keymap/default.toml`

**开发**（§5、§14「恢复」、§7.8「新建连接表单」「把 tab 移到相邻 pane」）
- [x] 启动连接连不上（`sqlmux`、`sqlmux <连接名>`、`--layout` 三条路都是）不再是启动错误（`app.Open` 永远建出 Session，不再返回 error；启动连接改走和其余连接同一条后台拨号的路：先进界面、节点连接中、tab 挂起，连不上节点标失败、toast；catalog 读失败照旧关挂起的表 tab）：进界面，节点标失败、toast 驱动的错误；用它的挂起 tab 不关（用户 2026-10-11 补：DataGrip 那样留着显示报错），内容区居中画 `error` 色的错误页（连接名加驱动的错误原文，折行），`R`、展开节点、连上都重试，连上后照常取数；query tab 本来就留着。启动错误只剩配置本身的问题。
- [x] 表单加「测试」按钮（`form.test`，`C-t`）：校验通过后用当前值真连一次（10s 超时），`测试中…`，结果写在底栏上方一行：`已连接 · <引擎 版本> · <耗时>` 或驱动的错误；不保存、测完断开。连接管理里同样。
- [x] dsn 字段空时显示 `dim` 色的示例（按 engine）；别的字段照旧不预填。
- [x] `tab.move.next` / `tab.move.prev`（默认 `<Leader>wn` / `<Leader>wp`）：当前 tab 挪到顺时针下一个 / 上一个普通 pane 的末尾并成为当前 tab，焦点跟过去；来源空了留引导页；只有一个普通 pane 或引导 tab 时无事。
- [x] 细节（已定）：测试不把连接留在树里；MySQL 的 dsn 两种写法（`mysql://` URL 和驱动的 `user:pass@tcp(host:port)/db`）本来就都认（§8），示例只写 URL 那种。

**验收**
- [x] 单测：启动连接拨号失败时 App 照常建出 session、节点状态是失败、toast 有错误、表 tab 还在且内容是错误页，连接恢复后 `R` 取到数据；`form.test` 成功和失败两种结果行；`tab.move.next` 后两个 pane 的 tab 列表和焦点；单 pane 无事。
- [x] golden：带「测试」按钮和结果行的表单。
- [x] e2e：把 play 的端口改成没人听的，`sqlmux` 能进界面、树里 play 变红、表 tab 居中显示错误、`conn.manage` 能建新连接；表单填一个错端口 `C-t` 结果行是错误、改对后是「已连接」；`SPC wn` 后 tab 在右边的 pane。

## F12.6 补全列表的排序 · 状态：passed（9b6179f；e2e e0c1838）

用户 2026-10-11：打 `sele` 时 `pap_secretary_problem_event` 排在 `select` 前面、打 `inse` 时四个 `oa_workflow_instance*` 排在 `insert into` 前面，匹配度明显是关键字高；关键字排前面。原因是 §9.7 原来先按候选组（列 → 表 → 关键字）再按得分。

- **依赖**：F5.12
- **涉及**：`internal/app`（补全的排序）、`internal/sqlkit`（语句开头的判断）

**开发**（§9.7「排序」）
- [x] 先按匹配层排：完全相同 → 候选开头的前缀 → 候选某个词首开始的前缀 → 其余模糊；层内按候选组顺序，组内按 fzf 得分。
- [x] 候选组顺序在语句开头处（光标前这条语句里没有别的 token）改成 关键字 → 表 → 列；别处不变。
- [x] 细节（已定）：WHERE 输入框、快速 SQL 同一套排序；「和输入完全相同的排第一」不变。

**验收**
- [x] 单测：`sele` 的第一项是 `select`、`inse` 的第一项是 `insert into`、`sec` 时 `pap_secretary_problem_event` 在词首前缀层；`select ` 之后打 `t` 时表仍在关键字前（不在语句开头）；`from t_order where sta` 列在前。
- [x] e2e：query 里打 `sele` 列表第一项是 `select`。

## F12.7 表头上的排序标记 · 状态：passed（6472fcd；e2e 17f3ccc）

用户 2026-10-11 验收：某列应用了排序只能在 chip 和树里看到，不直观，表头上给排序中的列加箭头。

- **依赖**：F12.1
- **涉及**：`internal/ui`（grid 表头）

**开发**（§7.6「按列排序」）
- [x] 在排序列表里的列，表头列名后面画方向图标（`sort_asc` / `sort_desc`，`warn` 色，同 chip）；排序列表多于一列时图标前加序号 `1` `2`（同下拉里的标法）；默认排序（只按行标识列）不画，不然每张表的 id 都带箭头。列宽不够放时表头照常截断、图标仍画在末尾（图标优先于列名的尾巴）；转置视图下不画。
- [x] 细节（已定）：不加新的图标名，`[icon]` 里改 `sort_asc` / `sort_desc` 两处都跟着变。

**验收**
- [x] golden：两列排序时的表头（`1↓` `2↑`）；默认排序时没有箭头。
- [x] e2e：表头单击后那一列的表头末尾出现箭头，第三次单击回默认后消失。

## F12.8 值编辑器的位置、大小和提示行 · 状态：passed（6472fcd；e2e 17f3ccc）

用户 2026-10-11 验收：居中的浮窗太宽，改成在单元格下方、窄一半、竖长；边框太素，进来不知道怎么编辑，底部加提示（`esc` 退出、`C-s` 保存关闭）；JSON 要有格式化。格式化本来有（`gqq`、JSON 列打开即格式化），问题是不可见，提示行里写上。

- **依赖**：F12.2
- **涉及**：`internal/app`（value）、`internal/ui`

**开发**（§10「值编辑器」）
- [x] 位置同列的定义浮窗：左边对齐光标所在格，从格的下一行开始；高度到 pane 内容区底部（最小 8 行，下方放不下 8 行就翻到格的上方、底边贴格的上一行；上下都不够时放在行多的一边）；宽度取 pane 内容区的一半（最小 40，再不够取 pane 宽）。转置视图下同样按光标所在格。
- [x] 底边框上一行提示（同表单和 COLS 的提示行，键位从 keymap 读）：`esc 关闭 · C-s 写回并关闭 · :w 写回 · gqq 格式化 JSON`；只读时只有 `esc 关闭 · y 复制`；`vim = false` 时 `esc 关闭 · C-s 写回并关闭`，只读时只有 `esc 关闭`。标题行不变（`<列名> · <类型>`），边框 `focus` 色不变。
- [x] 细节（已定）：不加 `=` 之类的新格式化键，`gqq` 就是 vim 的键；提示行放不下时按顺序丢末尾的项。

**验收**
- [x] golden：JSON 格的值编辑器（在格下方、半宽、带提示行）；靠底部的格翻到上方；只读的提示行。
- [x] 单测：位置算法（下方够 / 不够）。
- [x] e2e：`e` 打开后浮窗左边和光标所在格对齐、底部有 `C-s` 字样。

## F12.9 ORDER 浮层改成 COLS 式的列表 · 状态：passed（6472fcd；e2e 17f3ccc）

用户 2026-10-11 验收：ORDER 下拉打开就是输入匹配，不如 COLS 那样 `j` / `k` 选、`/` 搜；`space` 只切方向不再兼做去掉；`a` 移出一列、`A` 清空全部；提示行写上这些键。

- **依赖**：F12.1
- **涉及**：`internal/app`（dropdown → order 浮层）、`internal/keymap`（`[keys.order]`）、`internal/ui`

**开发**（§7.6「ORDER」、§6.4、§6.8）
- [x] ORDER 不再用通用下拉框，改用 COLS 那个列表浮层（同一个组件，另一份行文字和键）：打开时焦点在列表，`j` / `k` 移动，`/` 进过滤框（`↵` / `esc` 回列表，过滤保留），`esc` 先清过滤再关闭；每行 `1↓ 列名  类型`（不在排序里的前面空着）加主键标记；没有「默认」这一项，清空用 `A`。
- [x] `space`（`order.toggle`）：不在排序里 → 追加到末尾、方向按 `[grid] order`；在 → 翻转；不再去掉。`a`（`order.remove`）把光标所在列移出排序；`A`（`order.clear`）清空、回到默认排序；`↵`（`order.only`）只按这一列并关闭（在唯一的排序列上是翻转，同原来的 `↵`）。每次改动立刻重取（同原来 `space` 不关下拉）。
- [x] 提示行：`j/k 移动 · space 方向 · a 移出 · A 清空 · ↵ 只按这一列 · / 搜索`，键位从 keymap 读。作用域 `order`，键在 `[keys.order]`；`dropdown.toggle` 去掉（ORDER 是它唯一的用处）。
- [x] 细节（已定）：表头 `S-click` 和 `gS` 改成和新 `space` 一样（追加或翻转，不去掉）；右键菜单项不变；chip 不变。

**验收**
- [x] 单测：`space` 两次是翻转不是去掉；`a` 移出；`A` 后 SQL 回默认 ORDER；`/` 后打字进过滤、`esc` 回列表。
- [x] golden：ORDER 浮层（两列排序、提示行）。
- [x] e2e：`go` 后直接 `j` 移动（不是输入字符）；`space` `A` `esc` 后 chip 回默认。
- [x] 用户的 keymaps.toml 没有 ORDER 下拉的映射，不用改（决策者核对）。

## F12.10 命令面板的「表」范围列出 query · 状态：passed（6472fcd；e2e 17f3ccc）

用户 2026-10-11 验收：树里 Queries 的搜索和打开并进命令面板的「表」范围就行，不另开一个范围。

- **依赖**：F8.21（Queries 节点）、F4.x（面板的表范围）
- **涉及**：`internal/app`（palette）

**开发**（§12）
- [x] `@` 范围（和「所有」）在表之后列出 Queries 节点下的全部 query（同树的列表：文件加开着未落盘的）：`query` 图标 + 名字，中间列是记下的连接名（没有就空），右端 `Query`；`↵` 同树上的 `↵`（开着就聚焦过去，没开着在当前 data pane 新开 tab）；预览区显示文件的前若干行（没有文件就空），着色同 query。
- [x] 细节（已定）：不加新前缀；按分数混排、同分时表在 query 前面（审查时定）；没有「在新 tab 打开」。

**验收**
- [x] 单测：`@que` 匹配到 query 项，同分的表排在它前面、分低的表排在后面；`↵` 打开它。
- [x] e2e：面板 `@` 列表里出现 `query_1`，`↵` 后焦点 pane 的当前 tab 是它。

## F12.11 表格的 `O`、`fv`、`fc`、`fe` · 状态：passed（6472fcd；e2e 17f3ccc）

用户 2026-10-11 验收：`O` 也打开值编辑器；`fv` 按这个值过滤、`fc` 隐藏这一列、`fe` 导出，和 `fh` / `fH` 一组。

- **依赖**：F12.2、F12.3、F12.4
- **涉及**：`internal/keymap`（default.toml）、`internal/app`（`grid.col.hide`）

**开发**（§6.8、§7.6）
- [x] `[keys.grid]` 加 `O` = `grid.value`（`e` 保留）、`fv` = `grid.filter.cell`（`*` 保留）、`fe` = `grid.export`（`ge` 保留）。
- [x] 新 Action `grid.col.hide`「隐藏这一列」（`fc`）：同 COLS 里取消勾选光标所在列（光标挪到最近的可见列，chip 的 `COLS n/m` 跟着变）；右键菜单的「隐藏这一列」改成就是它；转置视图下作用于光标所在字段；结果 tab 没有 COLS、不做事（同 `fv`，审查时定）。
- [x] 细节（已定）：which-key 的 `f` 组里多出三项；不动 `fh` / `fH`。

**验收**
- [x] 单测：四个键各触发对应的 Action；`fc` 后列不可见、光标在相邻列。
- [x] e2e：`fc` 后表头少一列，`gc` 里那一列未勾选。
- [x] 用户的 keymaps.toml 里没有 `O` / `fv` / `fc` / `fe`（决策者 2026-10-11 核对过），不用改。

## F12.12 选择的颜色和 `V` 取消选择 · 状态：passed（6472fcd；e2e 17f3ccc）

用户 2026-10-11 验收：选中的样式像 yazi：正在选的（VISUAL 范围）蓝色，`esc` 确认后的黄色，现在全是一种灰色看不清；`v` / `V` 都是选择，想取消一段没办法，`V` 改成 yazi 的 unset 模式。

- **依赖**：F10.7
- **涉及**：`internal/app`（gridselect）、`internal/ui`（主题色 `selected`）

**开发**（§7.6「选择」、§7.7）
- [x] 主题加 `selected`：已确认选中的记录的序号格底色，默认取黄色系（和 `warn` 同色相、压暗到能当底色，默认主题和 ristretto 各给一个值）；VISUAL 里正在选的范围仍用 `visual`。主题里没写 `selected` 时退回 `visual`（旧主题文件不报错）。chip 行的 `3 selected` 文字改用 `warn` 色（用户 2026-10-11 补：和选中的黄色对应）。
- [x] `V`（`grid.select.unset`「取消选择（范围）」）：进 VISUAL 的 unset 变体，范围的记录在 `esc` 或再按 `V` 时从选中集里去掉；模式块显示 `UNSET`；范围的底色用 `visual`；范围内本来没选中的不受影响。`v` 不变。
- [x] 细节（已定）：`m` 不变；`d` / `y` 在 UNSET 里按去掉之后的选中集算（同 yazi：先结束模式再执行）；VISUAL 里按 `V`、UNSET 里按 `v` 只结束当前模式、不换模式（审查时定）。

**验收**
- [x] golden：两条已确认（`selected` 底）加一段正在选（`visual` 底）、chip 行的 `2 selected` 是 `warn` 色；UNSET 模式块。
- [x] 单测：`v` `j` `esc` 后两条选中；`V` `k` `esc` 后剩一条；主题缺 `selected` 时用 `visual`。
- [x] e2e：`m` 两行、`V` 覆盖其中一行、`esc` 后 chip 是 `1 selected`。
- [x] 用户的 ristretto.toml 由决策者加 `selected`（备份后缀按当天），`visual` 要不要从灰改蓝由用户定。
