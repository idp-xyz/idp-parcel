# SA 四条：步骤在 UC 里、执行器不在编排里——接线

Category: enhancement
Status: resolved——MCP-3（2026-09-04，基线 `3fff246`；提交 `b3d3343` / `3f4a428` / `1ea2597` + `047d4a2`，见文末 `## Answer` 与 Comments）
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

## Answer

2026-09-04，MCP-3，基线 `3fff246`。三组各成一笔，四条领域函数在 `application/` 都有了真实调用路径；
两份棘轮基线的 settlement-accounting 组/段随之清空。

| 组 | 提交 | 生产调用方 | 留下的形状 |
|---|---|---|---|
| SA-b `UC-SA-003` 步 7 调整半边 | `b3d3343` | `cut_off_publish_statement.go` 的 `CutOffPublishStatementHandler.IncludeAdjustment` → `IncludeAdjustmentInSubsequentPeriod` | 截单编排只拿到只读口 `ports.ChargeAdjustmentView`（没有 `Save`）——读写在接口名上分开，是 `TestOnlyTheOwningUseCaseReachesTheChargeAdjustmentRegister` 要的形状，不是绕它；同一处理器两种纳入的提交纪律收成一份 `commitInclusion` |
| SA-a `UC-SA-004` 步 5–7 | `3f4a428` | `audit_supplier_bill.go` 的 `ReceiveSupplierBillHandler.Audit` → `FormAuditedPayable`；`.Credit` → `FormSupplierCreditNote` | 审核人与供应商应付结算账户从已登记实例取（`SupplierAuditAuthorityView` / 新读口 `SupplierPayableAccountView`，未配置停在未决）；命令不带金额；一行只成立一份应付（库内唯一约束）；贷项账户/主张/币种取自原应付；步 7 两份引用各自成封、与接收同分区 租户/账单主张（`supplier_bill_handoff.go`，`SupplierBillHandoffIntent` 三格恰填一格）；新迁移 `0016_audited_payable_and_credit_note.sql`；新重建门 `RehydrateAuditedPayable` |
| SA-c `UC-SA-002` 步 5 BUY 侧 | `1ea2597` + `047d4a2`（基线补剪） | `form_supplier_expected_cost.go` 的 `FormSupplierExpectedCostHandler.Handle` → `FormSupplierExpectedCost` | 见下「缝的形状」；登记面 `ports.ExpectedCostRegistry`（`Save` + 按幂等三维 `LoadFirstVersion`），落点代数搬进 `ports.ExpectedCostSaveOutcome` |

### SA-c：BUY `PricingEvaluation` → SA 入向缝的形状（本票裁定）

票面三问逐答：

- **事件还是查询**——两者各半：**触发**用提供方已发的 `parcel-pricing.evaluation.recorded`（指针载荷
  `{tenantId, evaluationId}`，`internal/parcelpricing/adapters/postgres/evaluation_handoff.go`，其 `ports.EvaluationHandoffIntent`
  注释本就写着「费用采用在 settlement-accounting」）；**内容**按引用查——SA 侧端口 `ports.BuyEvaluationView.LoadBuyEvaluation`
  交回 `ports.BuyEvaluationAdoption`：评价引用、结果五格 `BuyEvaluationOutcome`（已完成 + 提供方四种非完成结果逐格保留，
  不合并、不折零）、命中的规则版本、原币/结算币金额（最小币单位）、换算步骤引用。
- **谁发**——提供方发触发；SA 侧的 inbox 消费者是接线的下一层，**不在本票**：那封信封只带评价引用，而形成预期成本还要
  发生项（TF）、费用项目与供应商协议（PC/TF）——它们只在 SA 请求评价那一步（`UC-SA-002` 步 2「结算提交主要范围、计算目的
  和合格来源引用」）才聚在一处，而那条编排今天同样不存在。本票的编排以命令承接这几件（`FormSupplierExpectedCostCommand`），
  金额、币种、换算步骤与规则版本整组出自评价，命令带不了。
- **asOf 怎么带**——不带。计价基准时点由评价固定在评价内（提供方 `Input().BusinessAt()`），SA 以评价引用回指，不复制第二份
  （单一权威）。

**消费侧适配器 `internal/settlementaccounting/adapters/parcelpricing/` 本票留空**，不是忘了：SA 领域存 `int64` 最小币单位，
提供方的 `Money` 是十进制（coefficient + scale）。向 parcelpricing 地盘（MCP-5）问过并得到实测答复：提供方今天**没有任何金额
取整步骤**——合计裸累加、换算裸相乘，`Total().Amount()` 的 scale 随价表写法与汇率位数漂，`RoundToIncrement` 全仓只用在重量上；
而 `label-channel/13` 已裁「没有币种小数位表，编一张就是把未确认参数写死成生产默认」。因此从十进制折到最小币单位这一步今天
**没有任何依据**，适配器里做任何取整都是在评价之外造一个评价没算过的数。提供方已立票记这个机制缺口：
[`pricing-amount-precision/01`](../../pricing-amount-precision/issues/01-evaluation-amount-scale-has-no-declared-source.md)
（金额取整槽落在哪、是否进 ADR，待 owner 裁）。那一格落地那天，适配器按 `BuyEvaluationView` 的形状落，读提供方
`ports.EvaluationStore.FindByID` 翻译：方向/目的不是 BUY·SUPPLIER_COST 拒，四种非完成结果逐格译（与 `label-channel/13` 缝二
`cost_bridge.go` 同一立场，那张是 BUY 评价到成本分，本票是到预期成本，两者相邻不同缝）。

与 `label-channel/13` 的边界对过：13 的桥答的是「候选择优时这个数多少」，不落库、不进 SA；本票答的是「结算采用哪个数」，
落成预期成本版本。两条缝各自读同一份评价，互不依赖。

### 完成判据逐条

- 三组在 `application/` 各有真实路径走到领域函数：`git grep -w <符号> -- 'internal/*.go' ':(exclude)*_test.go'` 在
  `047d4a2` 上对四个符号各命中 application 层调用（SA-c 那条是 `form_supplier_expected_cost.go`）。
- 每组带真库用例与应用层测试；三组都先写 red 再读既有编排（SA-a 的第一片 red 是 `audit_supplier_bill_test.go`，SA-c 的是
  `form_supplier_expected_cost_test.go`）。
- 基线按接线结果剪，每一剪都在父提交的干净内容上两法重数并带 SHA（记在两份基线各自的流水账里）；不加宽棘轮。
- 无金额、费率、阈值取值进代码；审核责任方、结算账户一律走已登记实例，未配置停在未决。
- 验证：见文末「验证」。

### 不在本票（各有去处）

- `cmd/parcel-api` 尚无 `UC-SA-004` / `UC-SA-002` BUY 侧的装配；`ReceiveSupplierBillDeps` 新增 `Accounts/Payables/CreditNotes`
  没有调用点要跟。装配随各自的在线登记面/入向消费者那笔一起落，届时先占 `cmd/parcel-api`。
- `SupplierPayableAccountView` 与 `SupplierAuditAuthorityView` 都没有 postgres 适配器（实例登记面，`PAR-SET-01`），编排对它们的
  「未配置」已按未决处理；登记面本身归 PN-07 登记面那批票。
- 预期成本形成**不发**发布意图：消费方（`UC-SA-004` 逐行匹配、`UC-SA-006` 经营口径）按版本读它，今天没有信封驱动的消费者，
  不预开空口（`FormSupplierExpectedCostDeps` 注释写明）。
- MCP-4 在做 CC-c 时问过：SA 的外部资金事实采用（`map_external_funds.go`）**不发信封**，唯一交出去的是核销/撤销两型。SA→CC 的
  「事实采用发信封」若要，另立票归 SA；CC 侧已按「应用层入口为缝」落。
- 机制清点 `docs/product/MECHANISM-INVENTORY.md` 本票三笔新增了文件与一份迁移，需在 tip 上重生成——推的人（MCP-1）在推前跑。

### 一次撞车的记录（给下一个改共享基线的人）

两份棘轮基线在本票期间被三个会话先后各改一次。我占号期间他人的 hunk 仍两次落到共享工作副本上；提交前的
`git diff HEAD -- <file>` 逐块核与 `git diff --cached --name-only` 都要**贴着 commit 跑**——后者当场拦下过一次会卷走他人七件的
索引提交。另一次我自己判错了：用带 `^` 锚的 `Select-String` 在 `git show` 管道上查「条目还在不在」得零命中，据此断言别人的提交
带走了我的剪（`1ea2597` 提交信里那句），实际没有——workflow.md「本机环境」点名的 GBK 并行坑，零命中恰是想要的答案时最危险。
`047d4a2` 补剪并在提交信里更正。

### 验证

- 每笔都在 detached worktree 里按 SHA 验（不含任何人的在途改动）：`3f4a428` 与 `1ea2597` 上 `gofmt -l` 空、`go build ./...` /
  `go vet ./...` 退 0、`go test -count=1 ./internal/settlementaccounting/...` 全绿且含真库（DSN 已设，`-v` 抽验
  `TestTheSameCostTripletFormsOnlyOneFirstVersion` 为 `PASS` 非 `SKIP`）；`047d4a2` 上 `internal/architecture` 与 `./migrations` ok。
- `1ea2597` 单独检出时两道 `HasNoStaleEntry` 对 SA 两条报红，是 `047d4a2` 之前的预期中间态；`616646d`（CC-c）本身干净。
- 全仓 `go test -count=1 -p 1 ./...`（含 DSN）在 `047d4a2` 的隔离检出上跑过，结果见 Comments 末条。

## Comments

- 2026-09-04 MCP-3：`047d4a2` 的 detached 检出上全仓 `go test -count=1 -p 1 ./...`（DSN 已设）退 0、零 `FAIL`；
  `internal/settlementaccounting/adapters/postgres` 那一包耗时数十秒且同一检出上 `-v` 抽验为 `PASS`，真库确实跑了
  （秒表本身不作判据，只配起疑）。四笔均未推，推送归 MCP-1；验过的 SHA 是 `047d4a2`，`616646d` 与 `1ea2597`
  两个中间态不单验（成因见 `## Answer`「一次撞车的记录」）。票面本条随 resolved 一笔提交。
