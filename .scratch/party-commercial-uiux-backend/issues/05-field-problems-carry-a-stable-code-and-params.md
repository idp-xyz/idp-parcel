# 05 逐格问题改回问题码加参数，管理台按码查中文

Category: enhancement
Status: resolved——2026-10-27 进 main `914473999`（见文末「进 main 记录」）。此前：作者完工，分支 claude-pcub5
Blocked by: 无（ADR-0316 由本票会话代决，待 PC owner 复核）
地盘：`internal/partycommercial/adapters/http` 的发布载荷收集与预览、录入答复（`publication_draft_payload*.go`、`preview_commercial_publication.go`、新文件 `publication_problem_codes.go`）；管理台 `pages/party` 的逐格问题话术（前端跟进，同票）。
出处：[party-commercial-uiux 票 03](../../party-commercial-uiux/issues/03-field-problems-in-chinese.md)（前端按原话形状译的过渡）与 spec 第三段「预览与发布的逐格问题改回问题码加参数，前端按码查中文」。

## 现象（读码，钉 main `8dea17a21`）

- 预览与录入的 `NOT_ACCEPTED` 答复里，逐格问题是 `{field, problem}`，`problem` 是 `err.Error()` 原话：领域错误、解析错误与适配器里手写的中文混在一起。
- 管理台 `field-problem-text.ts` 按原话的形状（正则）译中文，文件头自己写着是过渡。服务端改措辞时前端静默退回通用的一句；原话里的主语与取值要靠正则抠。

## 要做的

- 每条逐格问题除原话外带 `code`（恒在场）与 `params`（字符串键值，缺省不出现）。词表小而稳：`REQUIRED`、`NOT_IN_SET`、`TIME_FORMAT`、`EXACTLY_ONE`、`RATIO_BASE_NOT_ALLOWED`、`RATIO_BASE_REQUIRED`、`DAYS_NOT_POSITIVE`、`ORDER_NOT_POSITIVE`、`CHARGED_UNDECLARED`、`LIST_EMPTY_OR_DUPLICATE`，兜底 `INVALID`。贴码在问题产生的地方做，原话字节不变。
- 管理台：认得的码按码查话术（参数里的主语、格名直接用），码缺席 / `INVALID` / 不认得时退回原有的按原话译，原话仍收进可展开的一格。
- 稳定性承诺写进 ADR-0316：码只增不改，参数键只增不改，原话不在承诺内。
- 不在本票：受控批文口与各 JSON 镜像登记口的问题答复；领域错误的全面编码；话术多语种。

## 完成判据

- [x] HTTP 用例：典型载荷的逐格问题各带对应的码与参数（集合外带格名与收到的值、空值、时刻格式与时刻没填分开、额度两种互斥与缺基数、各册的天数 / 顺序 / 计不计费 / 空清单）；没有更具体码的答 `INVALID` 且无参数；预览答复带 `code`，无参数时没有 `params` 键。既有用例不动（原话字节不变）。
- [x] 管理台用例：每个码的话术、缺参数的说法、码缺席 / 兜底 / 不认得退回原话、清单逐项前缀、归组处按码译。
- [x] 管理台三道门、`internal/architecture` 通过。
- [x] 全量 `scripts/test-full.sh`。

## 三类

问题码与参数是机制；话术是呈现。

## 规模

M

## 完成记录

- 起点：分支 `claude-pcub5` 自 main `8dea17a21`（含 03）起，进 main 前 rebase 到 `b1cdd22e2`。
- 后端：新文件 `publication_problem_codes.go`（词表常量与参数键注释、`codedProblem`、`notInSet` / `requiredCell` / `exactlyOne` / `timeFormat` 构造、`problemCodeOf`）；`PayloadProblem` 与 `payloadProblemAnswer` 各多 `Code`、`Params`；各册载荷里对应这几类的构造点改用带码的原因。
- AT：`AT-PC-070`（UC-PC-001，紧接 069），盘点总览已补行。
- 管理台：`PayloadProblemRecord` 多 `code?`、`params?`；`field-problem-text.ts` 的 `codeTexts` 与 `codedProblemLine`；`problemsByField` 经它出行。问题在页面里是字符串行，带码的在归组处译好并把原话用私有分隔符接在后面，`fieldProblemText` 显示时拆开——为了不改各个只认字符串的表单组件，ADR-0316 第三点与风险点 4 已记为权宜。
- 代决与越权风险点：见 ADR-0316（四点），其中点 1（码是公开契约）与点 3（`NOT_IN_SET` 的 `what` 是写死的中文格名）需 PC owner 复核。
- 已知：多数领域错误仍落在 `INVALID`，管理台对它们仍按原话译；受控批文口与 JSON 镜像登记口的问题不带码。

## 评审记录

评审 ← 隔离子代理(sonnet) · 钉 `e4af0195d` / `014103b76`（rebase 前）：阻断无，总评可进 main；七条非阻断。

- **已修**：词表读口里漏贴码的「集合外的商业对象类别」改用 `notInSet`，补断言；`timeFormat` 只对空串答 `REQUIRED`，只有空白的时刻格答 `TIME_FORMAT`，补用例；答复处把空码兜成 `INVALID`，让「code 恒在场」不依赖调用方自觉，补录入口 400 体的断言；`field-problem-text.ts` 的注释去掉计数与变更叙事，分隔符的说法改准；`unclaimedProblems` 的注释说明交回的是行；ADR-0316 日期与同晚两份对齐并补「code 恒在场」「时刻格留空」一句。
- 核对无问题：改前每个格式串在改后逐字相同（用脚本比对整份 diff）；`add` 是问题进入 Problems 的唯一路径；管理台所有渲染问题行的地方都经 `fieldProblemText` / `itemProblemText`。

## 进 main 记录（2026-10-27，作者兼推送方）

- **main**：`914473999`（快进，起点 `b1cdd22e2`）。出处：分支 `claude-pcub5` 的代码笔、票面笔、评审修订笔与票面补笔；推送方在 tip 上另补 AT 盘点总览计数、ADR 索引与机制清点三笔生成物。
- **验**：全量 `scripts/test-full.sh` 退 0（158 ok / 0 FAIL）；`adr-index -check` 与 `at-coverage-counts -check` 退 0；管理台三道门：tsc 退 0、node:test 803/803、vite build 退 0；一次性库灌种子退 0、用后删库；`internal/architecture` 通过。
- **号**：`AT-PC-070`；`ADR-0316`。
- **评审**：见上「评审记录」。
