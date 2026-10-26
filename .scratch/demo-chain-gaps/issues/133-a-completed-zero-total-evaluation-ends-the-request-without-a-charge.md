# 133 已完成但合计为零的评价被当成矛盾上抛，请求永远停在已提交

Category: bug
Status: in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 `claude-dcg133` tip `22849dd6e`（其后一笔只写本完成记录）。此前 in-progress——2026-10-26 会话 `tops-20`（Claude Code）认领（code/12+46 已进 main），分支 `claude-dcg133`（worktree `idp-parcel-dcg133`）；新 ADR 取 0306。此前 ready-for-agent——2026-10-20 代决复核立（上线后尽快），见 [owner-review-20261020](../../owner-review-20261020/report.md)「应修订 → 去向」与「须你裁定 → 已代决」
Blocked by: 无
出处：用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」；本会话代决复核报告 [owner-review-20261020](../../owner-review-20261020/report.md) 的 M12；ADR-0198 决定四与越权风险点 4（BUY 路 ADR-0200 同形）

## 现象与修法

现象、证据与修法的完整写法以复核报告 M12 一条为准（单一权威），此处只记要点：

把「已完成而合计为零」移出矛盾清单，请求册加「负向结束（零额）」：入账、不重投、不建费用行；补 UC-SA-002 一行 AT；BUY 路同形。须新 ADR。

## 完成记录（2026-10-26，作者：tops-20 派出的 Claude Code 子代理）

起点：`origin/main` `1a8a51edc`（含认领）。单一权威：复核报告 M12，票面与报告无相抵；报告给的「至少做到入账不重投、在请求册上可辨认」那一档没有取，做全。

### 提交

| SHA | 内容 |
|---|---|
| `c601eaed6` | 代码、迁移与用例：形成门零额一格、两本请求册的`负向结束（零额）`、BUY 零额写口与装配、迁移 `settlement_accounting/0048` |
| `22849dd6e` | 文档：ADR-0306，ADR-0198 / ADR-0200 前向指针，SA `CONTEXT` 两句，UC-SA-002，AT 盘点 |
| 本笔 | 本完成记录 |

### 改了什么

- **领域**（SA）：`ErrZeroEvaluationTotal`——`FormCustomerCharge`、`FormSupplierExpectedCost`、`SupplierExpectedCost.AppendCorrection` 对结算币合计为零另答这一格，不成行；币种、跨币种换算步骤、同币种两额相等先判，负数与原币为零而结算币不为零照旧未受理（`zeroTotalGate`）。SELL 请求册状态 `SellRequestZeroTotal`（`ZERO_TOTAL`）、`NewZeroTotalSellEvaluationRequestConclusion`、`SellEvaluationRequestState.CarriesStopCause`。
- **应用**（SA）：`SellChargeZeroTotal` → `FormEstimatedCustomerChargeHandler` 记`负向结束（零额）`；`ExpectedCostZeroTotal` → `FormRequestedSupplierExpectedCostHandler` 经新依赖 `ZeroTotals` 记零额，册上已有停因上抛 `ErrBuyEvaluationRequestStoppedBeforeZeroTotal`，写口答不出为未决；外层 `RequestedSupplierExpectedCostZeroTotal`。
- **端口 / 适配器**：`ports.EvaluationRequestZeroTotalRecorder`、`EvaluationRequestRecord.ZeroTotal`；`postgres.EvaluationRequests.RecordZeroTotal`，`RecordStop` 不覆盖零额，`Save` 拒带零额的登记；SELL 册重建按 `CarriesStopCause` 复核；`sapricing.FormOnBuyEvaluationRecordedAdapter` 对零额入账。
- **装配**：`cmd/parcel-dispatch/assemble.go` 的 `formSupplierExpectedCostOnBuyEvaluationConsumer` 给外层接 `ZeroTotals: requests`。别的 `cmd/` 没有装这只外层。
- **迁移**：`migrations/settlement_accounting/0048_evaluation_request_zero_total_end.sql`（取 0048、空着 0047 给在途的 code/94）：SELL 状态格与停因配对两条 CHECK 收 `ZERO_TOTAL`；BUY 请求册加 `negative_end`，只收 `ZERO_TOTAL`、与停因互斥。
- **计价**（PP）：代码不改；`internal/parcelpricing/domain/evaluation_test.go` 补一条用例钉住扣减恰到零落成已完成。
- **文档**：ADR-0306（新）；ADR-0198、ADR-0200 只改 Status 行索引注记与 Links；SA `CONTEXT` 两句（「计价、组合与精度」SELL 请求结论那一条、「供应商成本与共享分摊」BUY 首版那一条）；UC-SA-002 元数据「负向结束」、主流程步 4、`AT-SA-184`、新 `AT-SA-215`；`.scratch/at-coverage-inventory.md` 补 `AT-SA-215` 入①，`AT-SA-184` 那一行（②）随场景改写。ADR 索引、AT 总览计数与机制清点未重生成（推送方的事）。

### 代决与越权风险点

1. **零额按合同结算币判**：合计取评价在结算币上的那个数（跨币种时是换算后），原币不为零而换算取整为零也落零额。读了 PP `evaluation.go` 合计与换算那一段、SA 两只读口的金额换写；没读计价的取整策略全集。owner 不同意改 `zeroTotalGate` 一处（ADR-0306 越权风险点 4）。
2. **零额判在映射之后**（SELL）：映射不全照记`待判断`，补登映射续办出的下一代再答零额。理由是零额只由形成门一处判、外层不碰金额。owner 要零额先说话，改 `FormEstimatedCustomerChargeHandler` 的次序（越权风险点 2）。
3. **零额不续办**：不带停因，参数登记与运营续办都不推；卡登错要改价，本票不开路。owner 不同意改续办队列「等着」的口径与运营续办口（越权风险点 1）。
4. **BUY 只加一列 `negative_end`**，不补完整状态格；与停因互斥，撞上作矛盾上抛。owner 要完整状态格另立迁移（ADR-0306「候选与反方」）。
5. **替代版本零额不接纠错版本**：链头停在失效纠错版本，当前有效预期成本为空；判在发生项失效与「失效未记到结算」等待之后。owner 要「重述为零」的版本，改 `AppendCorrection` 的零额一格与库上 `0008` 的金额 CHECK（越权风险点 3）。
6. **迁移前停在`已提交`的零额请求不回补**（越权风险点 5）；上线前没有租户。
7. **合计不为零的「已完成却未受理」仍上抛**：ADR-0306 只停用两份记录里「例如合计为零」那一句，其余矛盾格不动。
8. **SA `CONTEXT` 改了两条里各一句**：派单说「只改那一句」，BUY 路同形一并修，BUY 的现行规则在「供应商成本与共享分摊」那一条，不改它 `CONTEXT` 就落后于代码；两处都离「运营余额、收付款与坏账」很远，没碰资金映射、收付款入账与 `CustomerReceiptPostings`。
9. **AT 号**：`AT-SA-215`（全仓与在途分支 grep 不撞，UC-SA-002 原最大号 214）；`AT-SA-184` 改记、号不变。

### 完成判据 / 修法 → 用例

| 复核报告 M12 修法 | 落点 |
|---|---|
| 把「已完成而合计为零」移出决定四的矛盾清单（新 ADR、部分停用、指针、`CONTEXT` 同笔改） | ADR-0306「对已接受记录的作用」；ADR-0198 / ADR-0200 Status 行与 Links；SA `CONTEXT` 两句 |
| 请求册加一格「负向结束（零额）」：入账、不重投、不建费用行 | `AT-SA-215`：`TestACompletedEvaluationTotallingZeroEndsTheRequestWithoutACharge`、`TestTheSellGateEndsTheRequestWithoutAChargeForAZeroTotal`（真算的计价评价）、`TestASellRequestEndsAtZeroTotalOnceWithoutChargeOrStopCause`（真库） |
| 补 UC-SA-002 一行 AT | `AT-SA-215`；元数据「负向结束」、主流程步 4 补指针 |
| BUY 路同形一并修（ADR-0200、AT-SA-184） | `TestACompletedBuyEvaluationTotallingZeroEndsTheRequestWithoutACost`、`TestASupersedingEvaluationTotallingZeroAppendsNoCorrection`、`TestABuyRequestEndsAtZeroTotalExclusiveOfAStopCause`（真库）、`TestABuyEvaluationTotallingZeroEndsTheRequestThroughTheGate`（真库消费门、inbox 记账）；`AT-SA-184` 改记，`TestContradictionsAreErrorsNotUndecided` 改钉合计不为零的未受理 |
| 零额不吞矛盾、不进续办队列 | `TestACustomerChargeTotallingZeroIsAZeroTotalNotAnInvalidCharge`、`TestASupplierCostTotallingZeroIsAZeroTotalForTheFirstVersionAndACorrection`、`TestTheRegistersRefuseAZeroTotalPairedWithAStopCause`（库上 CHECK）、`TestABuyRequestEndsAtZeroTotalExclusiveOfAStopCause`（队列取不到） |
| PP 只在合计 < 0 时答失败 | `TestEvaluatePricingCompletesWithAZeroTotalWhenADeductionExactlyEqualsTheSubtotal` |

### 自验（含 PG，DSN `127.0.0.1:55432`）

- `go build ./...`、`go vet ./...` 通过；改过与新写的 `.go` 已 `gofmt -w`，`gofmt -l internal cmd` 为空。
- `go test -p 1 -count=1`（带 DSN）：`./internal/settlementaccounting/...`、`./internal/parcelpricing/...`、反查出依赖二者（含测试依赖）的 `./cmd/parcel-api/...`、`./cmd/parcel-dispatch/...`、`./cmd/parcel-pricing-feed/...`、`./cmd/parcel-pricing-register/...`、`./cmd/parcel-pricing-template/...`、`./cmd/parcel-settlement-register/...`，`./internal/architecture/...`、`./migrations/...`——33 个包全部 ok，无 FAIL。新写的真库用例另以 `-v` 确认 PASS 而非 SKIP；库上 CHECK 那一条临时打印过报错，三条各撞对应约束。
- `go run ./tools/adr-index -check`、`go run ./tools/at-coverage-counts -check` 只报生成物过期（退 1），不报缺行或格式错。
- 全量 `scripts/test-full.sh` 与种子灌库未跑（推送方的事）。

