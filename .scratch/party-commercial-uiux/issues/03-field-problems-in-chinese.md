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

**Status:** ready-for-agent

- [ ] 上面几类原话在格下显示为中文；原话可展开看到，不丢。
- [ ] 认不出的原话也先给一句中文，再给原话。
- [ ] 译表是纯函数，带 node:test 用例，每种形状至少一例，另有一例认不出的。
- [ ] 未认领问题区（对不上任何一格的问题）同样译。
- [ ] 三道门退 0。
