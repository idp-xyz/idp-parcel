# 复核端点（未配置格）；管理台复核动作、逐字段登记表单、更正动作

Category: enhancement
Status: ready-for-agent
Blocked by: 03

## 要建什么

按 ADR-0085 与 ADR-0099：

1. **`POST /pricing-reference-series-reviews`**：`adapters/http` 增 Intake 接口 + 处理器接口 + 封闭响应形状（ADR-0022 状态码语义），装配以 `UnconfiguredIntake{}` 起步；装配行进 `cmd/parcel-api/endpoints.go`（共享接线文件，先在频道占号）。
2. **管理台「计价参考序列」页**（`apps/admin-web/src/pages/pricing/ReferenceSeriesPage.tsx`）：
   - 目录列增「状态」列：已登记 / 在用 / 已退回 / 已替代（由读口给，见 05 或本票内最小扩展 `ReferenceSeriesListResponseBody`）。
   - 行动作「复核」：结论 + 依据；复核责任方取当前操作员标识（未配置时端点如实 403，文案说「接入渠道未配置」不说「尚未实现」）。
   - 「登记序列」签改为**逐字段表单**：序列（选已有 / 新建）、种类、来源标识、口径（汇率必填；从商业价格政策目录读口选带口径的版本——若该读口今天没有，先做成手填两格并在文案里说明）、期次表格（起 / 止 / 值 / 凭证引用）、登记责任方取当前操作员。提交前预览：证据等级、内容摘要、与上一版逐期差异。表单在前端组快照后仍走 `POST /pricing-reference-series-registrations`——摘要由后端领域构造函数算，前端只预览。
   - 行动作「更正此版本」：预填该版本全部期次，强制填更正依据，自动带 `PriorVersion`；**页面上不出现「编辑」**。
   - JSON 粘贴口保留为「高级」折叠，供 API 集成方。

## 红线

- 写准入不另立形（ADR-0085 决定二）；隔离 demo 里这些动作如实答未配置。
- 前端不算摘要、不裁证据等级，只呈现后端答复。
- 前端门禁只有 `tsc --noEmit`（本机 `pnpm build` 坏在环境，见 workflow.md），别去跑 `pnpm install`。

## 验证

http 单测：只收 POST、未配置 403、三态响应。`node node_modules/typescript/bin/tsc --noEmit` 绿。
