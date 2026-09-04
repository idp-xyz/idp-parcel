# SA 四条：步骤在 UC 里、执行器不在编排里——接线

Category: enhancement
Status: ready-for-agent
Blocked by: 无（第三组先定 BUY 入向缝的形状，见下）

由[票 03](./03-fourteen-that-only-tests-ever-call.md) `## Answer` 立出，按 [spec「处置裁决」](../spec.md) 第 1 条「按上下文各一张、票内逐条列」。举证在票 03，此处不复述；改动对象与完成判据在此。

## 三组，每组一个编排文件，可各成一笔

**SA-a `UC-SA-004` 步 5–6** — `internal/settlementaccounting/application/receive_supplier_bill.go` 今天到 `MatchBillLine` 就停。往后接：
- `FormAuditedPayable`：只对`已匹配`行、由授权审核责任方形成审核应付；量差、价差、无匹配、重复计费走争议，不在这里被顺手接受（函数注释与 `AT-SA-089`）。
- `FormSupplierCreditNote`：接收供应商更正/贷项/追加/迟到主张，形成供应商费用贷项并关联原应付。
- 步 7 的发布（审核应付与贷项引用分别发布）随之补上，供 `UC-SA-005`/`006` 消费。

**SA-b `UC-SA-003` 步 7 调整半边** — `cut_off_publish_statement.go` 已接 `IncludeLateChargeInSubsequentPeriod`（费用半边），补 `IncludeAdjustmentInSubsequentPeriod`：既有调整挂在原账单内的费用上、归入后续账期、关联原账单；纳入原周期构造期拒绝。

**SA-c `UC-SA-002` 步 5 BUY 侧 + 入向缝** — `FormSupplierExpectedCost` 缺的不只是编排，还缺 BUY `PricingEvaluation` 到 SA 的入向缝（`ports/` 与 `application/` 今天零处提到 BUY）。先定缝的形状（事件还是查询、谁发、`asOf` 怎么带），再写编排；与 [`label-channel/13`](../../label-channel-service-first-release/issues/13-buy-evaluation-to-cost-score-bridge.md) 对一下边界——那张是 BUY 评价到成本分，不是到预期成本。原币与合同结算币不同而评价未带换算步骤时保持待判断，不自行补算（`AT-SA-177`）。

## 完成判据

- 三组各自：对应 UC 步骤在编排里有一条真实路径走到该领域函数；`git grep -w <符号> -- 'internal/*.go' ':(exclude)*_test.go'` 在 application 层至少命中一处调用。
- 每组带真库用例（`pgtest`）与应用层测试；先写 red 再读既有编排，不照着现有代码长（见 `parallel-sessions.md`「镜像测试」）。
- `production_wiring_baseline.txt` 名单按接线结果剪，**在干净检出上重数并带 SHA**，不加宽棘轮。
- 无任何金额、费率、阈值取值进代码；审核责任方、争议规则、账期政策一律走已登记的实例。
- `gofmt -l` 空、`go build`/`go vet` 退 0、`go test -count=1 ./...` 绿（含真库，`-v` 下看 `PASS` 不是 `SKIP`）。

## 地盘

`internal/settlementaccounting/{application,ports,adapters/postgres}`；SA-c 若要新迁移，走 `migrations/settlement_accounting/` 新号并按「同笔提交」纪律带 `migrations.go` 接线（先占号）。不碰 `parcel-pricing`——缝的另一头若要改，先在频道问 parcelpricing 地盘持有者。
