# 02 收寄后处置管理台页

Category: enhancement
Status: in-progress——2026-10-28 tops-20（idp-parcel-91）立并认领，分支 claude-demo-pid
Blocked by: 无

## 现象

审计：「取消与收寄后处置」页（`cancel-parcel`）没有收寄后处置入口；后端口都在（launch-wiring/05，ADR-0341）：`GET /post-intake-disposition-requests`、`POST /post-intake-disposition-decisions`、`GET /post-intake-disposition-manual-items`（`?parcel=`）、`POST /post-intake-disposition-manual-item-results`。冻结清单记为 R2，演示「异常处理」要用。

## 做什么

只改 `apps/admin-web`：一页（或在合适的现有页加签）——待决请求列表 → 对一件包裹做处置决定（去向：继续、拦截、改送、退运、服务终止）→ 待人工处理清单（带 `supersededByLaterDecision` 标志）→ 逐项登记已处理 / 无法处理 + 说明 → 按包裹读回各项结果。写入口按授予收起（照 ADR-0315 的自查读口与现有页面写法）。`cancel-parcel` 页上客户渠道的取消签会 401（first-party-api/05，冻结），本票不动它，但页名或说明不要让人以为收寄后处置在那里。

## 完成判据

- 组件单测覆盖列表、决定、登记、读回与写入口收起；`tsc` 与全部单测通过；在 `page-registry.tsx` 的 `liveIds` 登记。
