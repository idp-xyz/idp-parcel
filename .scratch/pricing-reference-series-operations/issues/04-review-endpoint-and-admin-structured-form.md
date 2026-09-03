# 复核端点（未配置格）；管理台复核动作、逐字段登记表单、更正动作

Category: enhancement
Status: in-progress——MCP-1（切片 04a 后端已落 `9035df7`，04b 前端未做且已释号，谁都可以接）
Blocked by: 无（03 的代码已是主线祖先 `f62d619`；票 03 卡的只是 `seed.sh` 真库冒烟那一格，
不拦本票——那一格真正缺的是 WSL 里的 Go 工具链而不是票面旧写的「WSL 够不到 PG」，MCP-5 实测）

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

## 切片

**04a 后端（已落）／04b 前端（未做）**，分开是因为**前端半必须晚于后端半落地**：`2a9a76a`
那道门禁断言前端路径 ⊆ parcel-api 端点表，先推一个发向 `/pricing-reference-series-reviews`
的前端动作而端点表还没那一行，会把全仓染红在 `internal/architecture/`——而那种红看起来
与自己的改动无关，别人得先停下来查归属。

## Comments

- 2026-09-03 · MCP-1：**切片 04a 落地：复核端点进端点表，生产编排接真。**

  `POST /pricing-reference-series-reviews`。`adapters/http` 增 `ReferenceSeriesReviewIntake`
  与 `ReferenceSeriesReviewer` 两个接口 + 端点，`UnconfiguredIntake` 补第三个方法；装配行
  进 `endpoints.go` 挂字面量 `UnconfiguredIntake{}`（写准入不另立形，ADR-0085 决定二），
  `main.go` 接真编排，`unwired_orchestration.go` 加占位，探针表补一格。

  **七格答案逐字透出，其中三格是治理答案不是调用方错误**：`需换人复核`（换个人来）、
  `版本不在册`（先去登记）、`冲突`（改主意另追加一条）。折成 4xx 会让调用方以为自己请求
  写错了，而那三格的恢复动作各不相同。`已追加`取 201，其余取 200。

  **这一口比两个登记口更不能松，写在了三处代码注释里**：复核责任方是四眼门的一半（领域
  拒绝复核责任方等于登记责任方），任何采信自报身份的 Intake 都等于把那道门拆了。因此
  `TestIsolatedReadIntakeCannotServeRegistration` 补了第三条断言——隔离读那个注入的合成身份
  装不进复核口，编译期成立。

  **事务包装不与两个登记包装合并**（`transactionalReferenceSeriesReview` 独立成型）：登记与
  复核是两种命令两套代数，合成之后装配点可以把登记编排接到复核端点上而编译仍绿。生产侧
  读口与写口是两只适配器落在两张表上（`ReferenceSeriesVersions` 读回登记供四眼门，
  `ReferenceSeriesReviews` 追加复核），分设不是为了对称。

  **未做（04b，前端那半）**：状态列、复核动作、逐字段登记表单、更正动作、JSON 粘贴口折成
  「高级」。端点已在表上，那道门禁不再拦着它，谁接都行。**接的人注意票面里那条**：口径要从
  商业价格政策目录读口选带口径的版本，而那个读口今天可能没有——票面自己给了退路（先做成
  手填两格并在文案里说明），**但请先取证它到底有没有，别照票面那句直接实现**。

  **验证**：`gofmt -l` 为空、`go build ./...` 退 0、`go vet ./cmd/parcel-api/
  ./internal/parcelpricing/...` 退 0、`go test -count=1 ./cmd/parcel-api/
  ./internal/parcelpricing/... ./internal/architecture/` 全 ok（含真库；本切片对
  `adapters/postgres` 零改动，那一层的绿是既有用例给的）。`-race` 未跑。
