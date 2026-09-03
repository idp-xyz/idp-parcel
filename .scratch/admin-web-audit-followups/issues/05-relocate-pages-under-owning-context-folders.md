# 05 四张页面归位到所属上下文目录

Category: enhancement
Status: ready-for-agent
Blocked by: 无

## 缺什么

`apps/admin-web/src/pages/governance/` 今天装着五张页，`navigation.ts` 的 `moduleInfoById` 给
它们的主责上下文却分属三处：

| 页 | `moduleInfoById.owner` | 现目录 | 应属目录 |
|---|---|---|---|
| `ReconciliationPage` | 结算与经营核算 | governance | `settlement/` |
| `SettlementApplicationPage` | 结算与经营核算 | governance | `settlement/` |
| `ExceptionTriagePage` | 全程追踪与异常 | governance | `visibility/` |
| `AcceptanceReviewPage` | 小包托运 | governance | `shipment-request/` |
| `StageAdmissionPage` | 试点治理 | governance | 留在 `governance/` |

目录是 README 里「治理与复核类页面」那一批多会话并行时的历史分组，与现在按上下文分目录的
其余页面不一致。读代码的人按上下文找页会找不到。

## 做什么

移动文件、改各桶导出 `index.ts` 与 `page-registry.tsx` 的 import 路径；页面内容一字不改。
`page-registry.tsx` 是共享接线文件（第三类），只改 import 行、不动登记表与 `liveIds`。

`governance/api.ts` 只服务 `StageAdmissionPage`，留原地。

## 完成判据

`tsc --noEmit` 无输出；`pnpm test` 仍绿；`git log --follow` 能追到移动前历史（用 `git mv`）。

## Comments
