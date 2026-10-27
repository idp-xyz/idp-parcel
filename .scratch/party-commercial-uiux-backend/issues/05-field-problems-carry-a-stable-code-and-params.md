# 05 逐格问题改回问题码加参数，管理台按码查中文

Category: enhancement
Status: in-progress——作者完工，分支 claude-pcub5，待隔离评审与进 main
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
- [ ] 全量 `scripts/test-full.sh`（待推送方）。

## 三类

问题码与参数是机制；话术是呈现。

## 规模

M

## 完成记录

- 起点：分支 `claude-pcub5` 自 main `8dea17a21`（含 03）起。
- 后端：新文件 `publication_problem_codes.go`（词表常量与参数键注释、`codedProblem`、`notInSet` / `requiredCell` / `exactlyOne` / `timeFormat` 构造、`problemCodeOf`）；`PayloadProblem` 与 `payloadProblemAnswer` 各多 `Code`、`Params`；各册载荷里对应这几类的构造点改用带码的原因。
- AT：`AT-PC-070`（UC-PC-001，紧接 069），盘点总览已补行。
- 管理台：`PayloadProblemRecord` 多 `code?`、`params?`；`field-problem-text.ts` 的 `codeTexts` 与 `codedProblemLine`；`problemsByField` 经它出行。问题在页面里是字符串行，带码的在归组处译好并把原话用私有分隔符接在后面，`fieldProblemText` 显示时拆开——为了不改各个只认字符串的表单组件，ADR-0316 第三点与风险点 4 已记为权宜。
- 代决与越权风险点：见 ADR-0316（四点），其中点 1（码是公开契约）与点 3（`NOT_IN_SET` 的 `what` 是写死的中文格名）需 PC owner 复核。
- 已知：多数领域错误仍落在 `INVALID`，管理台对它们仍按原话译；受控批文口与 JSON 镜像登记口的问题不带码。

## 评审记录

（待填）
