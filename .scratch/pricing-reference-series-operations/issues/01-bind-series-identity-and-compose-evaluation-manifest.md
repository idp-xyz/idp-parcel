# 价卡绑序列标识；评价清单组合解析到的序列版本；PPC-4；复核与在用选择的领域件

Category: enhancement
Status: resolved——`7042a38`（MCP-3，2026-09-03）
Blocked by: 无

## 要改成什么

按 [ADR-0099](../../../docs/adr/0099-price-card-binds-series-identity-and-in-force-version-is-derived-from-review.md) 决定一、二、三、四在 `internal/parcelpricing/domain` 内落地，不出领域包：

1. **`ReferenceSeriesBinding` 改为（种类 + 序列标识）。** 序列标识是租户内的序列 ID 字符串（与登记册 `series_id` 同一取值）。方案清单不再并入序列版本引用；`PricingPlanStructures.valid()` 对绑定的排序与去重改按（种类、序列标识）。
2. **规范化形状 `PPC-3 → PPC-4`。** `plan_snapshot.go` 的 `referenceSeriesBindingSnapshot` 去掉 `reference`、加 `seriesId`；`fingerprint.go` 的 `canonicalReferenceSeriesDocument` 同步；常量注释按既有写法记这次拓宽改了什么。`PPC-3` 快照在重建门被拒（既有纪律，不加兼容读法）。
3. **`resolveSeries` 改比序列标识。** 输入快照携带的取值其 `Reference().ID()` 必须等于绑定的序列标识；不等仍是 `REFERENCE_SERIES_VERSION_MISMATCH`（对象从「版本不同」变成「不是这条序列」，码不改、文字改）。
4. **评价清单 = 方案清单 + 解析到的序列版本引用。** `EvaluatePricing` 在形成评价时把每条取值的 `Reference()` 并入 `evaluation.manifest`；`ReplayEvaluationRequest` 的 `expectedManifest` 与新组合清单比对。`manifestContains(planReference)` 不变。
5. **`SeriesReview` 值对象**：租户、序列版本引用（`ArtifactReferenceSeries`）、复核责任方、复核时刻、结论（`通过` / `退回`，封闭集）、依据（非空）。构造门：复核责任方 ≠ 登记责任方——这需要构造时拿到被复核版本的登记责任方，做成 `NewSeriesReview(registration ReferenceSeriesRegistration, ...)` 让门在结构上关得住。
6. **在用选择纯函数** `SelectInForceSeriesVersion(candidates []ReviewedSeriesVersion, at time.Time) (VersionReference, bool)`：只看 `通过` 且复核时刻 ≤ at 的候选；按复核时刻 → 登记时刻 → 版本号字典序取最新；空则 false。`ReviewedSeriesVersion` 是（版本引用、登记时刻、复核时刻、结论）的最小载体，由端口交回。

## 红线

- 领域包不依赖 HTTP / `pgx`；解析在用不进 `EvaluatePricing`。
- 不改 `reference_series_register.go` 的登记语义与 `PRS-1` 形状。
- 调用点：`scripts/demo-seeds/seedgen/main.go` 用 `NewReferenceSeriesBinding(kind, versionReference)`——签名改动会拆它，属「会让旧调用点对不上」一类：**在隔离 worktree 改完验绿一次性应用**，并同笔更新该调用点与重跑产物（`scripts/demo-seeds/data/pricing/*.json`）。开工前在频道报窗口，做不成也报「窗没开」。

## 验证

- `internal/parcelpricing/domain` 单测：绑定改形、`PPC-4` 摘要、清单组合、重放清单比对、`SeriesReview` 四眼门、在用选择的排序与边界（同刻、退回、未来复核）。
- 全仓 `go build` / `go vet` / `go test -count=1 ./...`；seedgen 产物重生成后 `cmd/parcel-pricing-register` 单测仍绿。

## Comments

- 2026-09-03 MCP-3：落地 `7042a38`（在隔离 worktree `mcp3-rsops` 上做完验绿，cherry-pick 到 main；
  父提交 `29085fe`）。六项全部按票面落：绑定改形、`PPC-4`、`resolveSeries` 比标识、评价清单组合
  （`composeEvaluationManifest`，重放比对改比评价清单）、`SeriesReview`（四眼门以登记为入参）、
  `SelectInForceSeriesVersion`。棘轮基线加一行（调用方是票 02 的真库读口；`New*` 构造按门禁判据
  不入名单）。seedgen 调用点同笔跟上，两张 SYN 价卡快照升 `PPC-4`；两条序列快照 PRS-1 内容不变。
  **验证**：在 `7042a38` 的 detached 检出上 gofmt 零输出、`go build` / `go vet` / `go test -count=1 ./...`
  全绿；**未设 PG DSN，PG 用例跳过**（本票无 .sql、无适配器改动）；domain 新增与改动的用例 `-v` 下
  逐条 PASS。
  一处与票面不同：票面写「评价原因文字区分无版本 / 未复核 / 期次缺口」属票 03（编排层才知道
  是哪一格），本票纯函数只在解释里带 `序列@版本`。
  **`go test -race` 未跑**（本机 Windows 侧无 cgo，见 workflow.md）；本票无并发代码。
