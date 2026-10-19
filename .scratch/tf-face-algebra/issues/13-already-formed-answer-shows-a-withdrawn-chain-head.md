# 13 装载分配：链头是撤回版时，「分配已形成」的答复看不出分配已撤回

Category: bug
Status: ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 立（tf-face-algebra/02 实施中发现，票外未改）
Blocked by: 无
父票：[operator-channel/26](../../operator-channel/issues/26-non-author-review-backlog-for-claude-code-landings.md)
出处：tf-face-algebra/02 实施子代理的「本票外的发现」，经其非作者评审确认。
地盘：
- internal/transportfulfillment/adapters/http/form_load_assignment.go 及其测试
- internal/transportfulfillment/application/form_load_assignment.go（只在需要从结果取撤回标记时）

## 现象

tf-face-algebra/02 之后，分配已有版本时答 `LOAD_ASSIGNMENT_ALREADY_FORMED` 并回显链头。链头若是撤回版，答复里没有撤回标记，调用方会把一个已撤回的分配读成仍在生效。今天撤回入口没接，所以潜伏。

## 应当

回显链头时带上它是否已撤回（与撤回时刻），调用方据此决定另起分配。

## 完成判据

- [ ] HTTP 用例：链头为撤回版时答复带撤回标记；非撤回链头答复形状不变。
- [ ] Go 门禁照常。
