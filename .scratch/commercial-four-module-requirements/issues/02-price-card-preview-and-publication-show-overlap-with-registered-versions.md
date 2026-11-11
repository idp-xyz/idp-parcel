# 02 价卡预览、批准与发布都不提示与在册版本的适用期重叠——操作者要等下游的评价请求停住才知道

Category: enhancement
Status: ready-for-agent
父票：[spec](../spec.md)
Blocked by: 无
地盘：`internal/parcelpricing/application`、`internal/parcelpricing/adapters/postgres`、`internal/parcelpricing/adapters/http`、`apps/admin-web/src/pages/pricing`

对应要求：Pricing「发布冲突检测」。

## 现象

取证钉 `11bfa1ffb`。

- 价卡登记查的是内容冲突：同一版本号内容不同答 `CONTENT_CONFLICT`，规范化版本不可比答 `CANONICALIZATION_DIFFERS`（`RegisterPriceCardHandler`）。
- 两版适用期重叠不拦登记。这是 [ADR-0234](../../../docs/adr/0234-an-older-price-card-version-leaves-new-selections-only-through-an-explicit-closure-for-a-named-successor.md) 的决定：新版登记本身不收尾旧版，旧版只经指名后继的显式收尾离开新的选择。
- 重叠到选卡时才露出来：`PriceCards` 的 `ResolveInForce` 答 `PriceCardApplicabilityConflict`，评价请求停住，等人收尾旧版。
- 预览、草稿批准、草稿发布的答复里，没有一格说「这个范围、方向与目的上已有同时适用的在册版本」。在 `internal/parcelpricing/application` 与 `adapters/http` 的非测试代码里检索「重叠、overlap」无命中。

## 要做的

1. **答复多一格**：预览、草稿批准、草稿发布各带「同时适用的在册版本」，列出与这张卡的适用期有交集、且折完收尾之后仍可选的每一版的引用与交集区间。
2. **一个口径**：折截止与退役的读法与 `ResolveInForce` 用同一处，不另写一套判重叠。
3. **只提示，不拦**：ADR-0234 的换版流程要求后继先在册，拦了后继就登不进去。
4. **分两种说**：同一方案的旧版，下一步是收尾，说明该走截止还是退役；另一个方案的卡，要人裁。
5. **管理台**：预览步与草稿面板显示这一格。

## 要裁的

- 客户若把「冲突检测」理解成拦截：可以让租户登记一条「重叠即拒」的规则，但那要先改 ADR-0234 的换版顺序。默认不做，开工前向用户确认一次。

## 三类

提示是机制。「重叠即拒」若做，开不开是租户取值。

## 完成判据

- 同范围同方向同目的上登一张适用期有交集的卡：预览答出在册的那一版与交集区间；发布照旧成功。
- 旧版已被截止到新版起点之前时，不提示。
- 提示里的版本集合与随后 `ResolveInForce` 在交集内任一时点答的候选一致；有真库用例。
- `UC-PP-001`「价卡收尾」一节补一句，AT 号取空号。
