# 03 — 逐格问题说中文

**Parent:** [spec](../spec.md)

**What to build:** 发布表单点「预览」被拒时，服务端按格交回 `{field, problem}`，表单把 `problem` 原样挂在格下（`PublicationFormFields` 的 `Problems`）。`problem` 是领域错误或 Go 解析错误的原话，配置员读不懂。空表单点预览实测：

- `party commercial: blank value: commercial object ID`
- `须为 RFC 3339 时刻：parsing time "" as "2006-01-02T15:04:05.999999999Z07:00": cannot parse "" as "2006"`
- `额度须恰一格在场：limitMinor 或 limitRatioBasisPoints`

前端先行的过渡：按原话的形状译成中文，认不出的给一句中文概括；原话不丢，收进可展开的「服务端原话」。根治是服务端改回问题码（spec「后续」第三段），到那天本票的译表退役。

要译的形状（以服务端 `publication_draft_payload` 与 PC 领域错误为准，作者读码核对）：

- 留空：`party commercial: blank value…` → 「必填，不能留空」。
- 时刻：`须为 RFC 3339 时刻：…` → 「时间格式不对：请填完整时刻，例如 2026-10-01T00:00:00+08:00」。
- 只填一项：`…须恰一格在场：a 或 b` → 「…只能填其中一项」，不带字段名。
- 不在集合：`集合外的X "v"` → 「X 不在可选范围内」。
- 其余 `party commercial: …` → 「填写的内容不合要求」。

**Blocked by:** None — can start immediately

**Category:** enhancement

**Status:** resolved

- [x] 上面几类原话在格下显示为中文；原话可展开看到，不丢。
- [x] 认不出的原话也先给一句中文，再给原话。
- [x] 译表是纯函数，带 node:test 用例，每种形状至少一例，另有一例认不出的。
- [x] 未认领问题区（对不上任何一格的问题）同样译。
- [x] 三道门退 0。

## 完成记录（2026-10-26，tops-54，前端切片，worktree `idp-parcel-pcux`）

- **落点：** 本笔提交。
- **改了什么：**
  - 新纯函数 `field-problem-text.ts`：`fieldProblemText(raw)` 按原话形状给一句中文，交回 `{text, original}`；`itemProblemText(relativePath, raw)` 给清单逐项的问题，把 `methods[0]`、`materials[2]` 说成「第 1 项」「第 3 项」，原话连同路径收起。
  - 形状取自 `publication_draft_payload` 与 PC 领域错误的现行写法：留空（含时刻格解析空串）→「必填，不能留空」；时刻解析失败 →「时间格式不对：请填完整时刻，例如 …」；「X须恰一格在场」→「X只能填其中一项」（英文键名不进中文）；「集合外的X」→「X不在可选范围内」；「X须在场」→「X必须填写」；额度基数、天数、判断顺序、计不计费、空清单五句专门的说法；其余 `party commercial:` 前缀与 Go 解析错误给通用一句「填写的内容不合要求」。
  - **认不出的照原样显**：同一个显示件也显前端本地校验的中文问题，那些可能夹着「P7D」一类例子，只有看得出是服务端或解析器原话时才换通用句。
  - 显示件 `ProblemText`（`PublicationFormFields`）：先中文，原话收进可展开的「原话」；原话本就是中文白话时不显第二遍。`Problems` 逐条走它；`PathProblems` 不再在每条前挂 JSON 路径，两个键答同一句时只显一条；发布流程的未认领问题区照旧带路径（那里路径是唯一的定位），后面的话同样译；客户服务规则、接受前财务控制策略、交付条件三处直接显原话的行与逐项也改走它。
- **判据：**
  - ✅ 演示环境空的信用政策表单点预览：格下为「必填，不能留空」「时间格式不对…」（非空时）「额度只能填其中一项」，每条后有「▶ 原话」，展开见 `party commercial: blank value: commercial object ID` 原文。
  - ✅ 认不出的先给中文：通用句「填写的内容不合要求」+ 原话；本就是中文的照原样。
  - ✅ 纯函数用例 `field-problem-text.test.ts` 11 条：每种形状至少一例、认不出的两例（领域错误、JSON 解析错误）、本地中文夹英文照原样一例、逐项两例。
  - ✅ 未认领问题区同样走 `ProblemText`。
  - ✅ 三道门：`tsc -b --noEmit` 退 0，`run-tests` 618/618（含同批 06 已写好的纯函数用例），`vite build` 退 0。
- **未验：** 「集合外的X」「须在场」几类要构造畸形请求才触发，只有单测覆盖，没在浏览器里触发。
- **说明：** 这是前端先行的过渡。服务端改写某句原话时这里认不出、退回通用句，不会显错话；根治（服务端回问题码加参数）记在 spec 后续第三段。
- **评审：** `ProblemText` 放在 `pages/party/PublicationFormFields`，不是 `templates/`、`shell/`、`components/` 共享面；改动全在 `pages/**`，自查判据。
