# 同币种计价纠错造出了形成门明禁的形状

Category: bug
Status: resolved

`SupplierExpectedCost` 的两扇门对「同币种两额可不可以不等」给出相反答案。本票只记录
两边的实际行为与它是怎么暴露的，**不提方案**——判哪一扇门对，属领域 owner。

## 两扇门各自做什么

`FormSupplierExpectedCost`（`internal/settlementaccounting/domain/supplier_expected_cost.go`）
在原币与结算币相同时要求两个金额相等，注释写明理由是「同币种两个金额不一致：没有换算
却造出了第二个数」，不满足即 `ErrInvalidSupplierCost`。

`AppendCorrection` 在同一个文件里，只重述结算金额：`corrected.settlementMinor` 取入参，
`originalMinor` 原样留着。它对币种只有一条检查——跨币种时换算步骤必备，对同币种两额
是否仍相等一字未提。

于是这条路径合法而它的产物过不了形成门：

1. `FormSupplierExpectedCost` 造一份 USD 4200 / USD 4200 的首版；
2. 对它 `AppendCorrection(..., settlementMinor: 3900, ...)`；
3. 得到 USD 4200 / USD 3900、无换算步骤的纠错版本。

把这份产物的字段原样喂回 `FormSupplierExpectedCost`，它会被判为不成立。

## 它是怎么暴露的

写迁移 `0008_supplier_expected_cost.sql` 时，`supplier_expected_cost_same_currency_amounts_agree`
这条 CHECK 是照形成门那句不变量抄的。真库用例 `TestACorrectionVersionKeepsItsBackReference`
存纠错版本时当场打红：

```
ERROR: new row for relation "supplier_expected_cost" violates check constraint
"supplier_expected_cost_same_currency_amounts_agree" (SQLSTATE 23514)
```

**这正是「领域不变量在库内再守一遍」那条纪律要的效果。** 两扇门的分歧在领域包里静静
待着，领域用例各测各的门因而都绿；把同一条不变量在库里再写一遍之后，两扇门第一次被
要求同时成立，分歧立刻现形。此处记下作例证。

## 当前代码站在哪里

提交 `91af99c` 的处置是：库内 CHECK 与新的重建门 `RehydrateSupplierExpectedCost` 都把
等值那一条收紧到 `prior_version IS NULL`，即只对首版成立，对纠错版本不生效。重建门也
因此不走 `FormSupplierExpectedCost`——拿形成门去验一份写得好好的纠错版本，读回的会是
「这行不成立」。

这是**适配器忠于当下领域**，不是对分歧的裁定：领域今天能产出什么形状，持久化面就得
存得下、读得回。领域改口径时这两处跟着改。

## 需要 owner 定的

同币种计价纠错之后，原币金额应当是什么。今天它是纠错前的旧值，而结算金额已是新值；
两个数在同币种下不再相等，也没有任何换算依据解释这个差。这条定下来，才谈得上两扇门
该向哪边对齐。

## 影响面

`AppendCorrection` 与 `FormSupplierExpectedCost` 目前都没有应用层调用方——UC-SA-002
的预期成本形成编排尚未落地，两者的生产调用点只有领域用例与
`internal/settlementaccounting/adapters/postgres/supplier_expected_cost.go` 的读写面。
也就是说改口径现在的代价最低：没有已落库的生产数据按旧口径写下过，也没有编排要跟着改。

## Comments

- 2026-08-14 · MCP-5：本票由 MCP-1 在 SA 九口视图那一票的回执里指派创建，要求「写清两个
  门的实际行为差异、不提方案、把真库 CHECK 打红那件写进去当例证」。三条已照办。
- 2026-08-20 MCP-1：triage 完成，派 MCP-3，票名 SUPPLIER-COST-RULING。**本票按裁断票派，不是
  实现票**：走 `/grill-with-docs` → `/ubiquitous-language` → `/domain-modeling`，先回答「需要 owner
  定的」那一问（同币种纠错后原币金额应当是什么），把结论写进 SA 的 `CONTEXT.md` 或另立 ADR，
  并写明 `91af99c` 那处收紧要不要跟改。**改 `supplier_expected_cost.go`、迁移 `0008` 的 CHECK
  与重建门之前停下报 MCP-1 等用户点头**——那是领域不变量，属难逆转取舍。
  现在动代价最低的取证仍成立（两个函数都无应用层调用方，无已落库生产数据）。基线 `11057dc`。
- 2026-08-20 · MCP-3：裁断完成，落 [ADR-0067](../../../docs/adr/0067-cost-correction-restates-the-whole-evaluation-result.md)
  与 SA `CONTEXT.md`（「供应商成本与共享分摊」两条新句 + 「赔付、追偿、税费、币种与法人」两条新句 +
  调整类型表「供应商预期成本计价纠错」一行改写）。经两轮 `/grill-with-docs`，owner 逐问定案：

  1. **同币种两额必须相等对所有版本成立**，纠错版本不例外。
  2. **计价纠错在供应商侧是重述全额的新成本版本**，不是客户侧那种带借/贷方向的差额调整。
  3. **一份版本的计价结果整组出自它引用的那一个评价及其版本清单**；纠错换评价，因此整组重述。
     不动的只有身份四件：集团租户、运输收费发生项、费用项目、合同结算币。
  4. 裁断一次裁全，**实现拆两票**（见本目录 `02`、`03`）。

  据此，票面「需要 owner 定的」那一问的答案是：**同币种计价纠错之后，原币金额是新评价给出的
  原币金额，与新的结算金额相等。** 今天它停在旧评价上不是同币种专有的毛病——`AT-SA-054` 的
  采购规则更正在跨币种下同样会让原币金额停在旧价卡上，只是没有哪条 CHECK 会照出来；同币种
  是它第一次可见的地方。同一个毛病另有三处：采购规则版本、协议引用、发生项版本换评价时也不
  跟着换，其中发生项版本与 `OccurrenceVersion` 自己的注释「发生项有效性更正换版本，预期成本
  据以追加计价纠错」直接冲突。

  **`91af99c` 两处处置：都跟着改，去掉例外。**
  - 迁移 `0008` 的 `supplier_expected_cost_same_currency_amounts_agree` 撤掉 `prior_version IS NOT NULL`
    那一支。**不就地改 0008**——已施加的迁移不可改写（校验和把关），照 `0002_fix_disposition_check.sql`
    与 `0003_fact_and_episode_tenant.sql` 的先例出新文件 `DROP CONSTRAINT` 后重建。
  - `RehydrateSupplierExpectedCost` 的按首版分支去掉条件，对所有版本验。**重建门本身保留**
    （ADR-0028 的两扇门是独立纪律，它剩下的独有职责是校验回指与原因成对且不自指），但它那段
    解释自己为何不走形成门的注释整段作废，必须改写。

  本票交付物到此为止；实现按硬约束停在 MCP-1 与用户点头之前。
- 2026-08-20 · MCP-1：交付物逐项复核通过（ADR-0067 五条决定与 CONTEXT 四句新不变量、调整类型表
  一行改写逐条吻合；迁移不可改写纪律守住——0008 不就地改，出新文件重建），随本提交入库，置
  resolved。实现两票：`02` 等用户点头，`03` 取证部分先行派出。ADR Consequences 里「CustomerCharge
  单币种与 CONTEXT 币种三件组对不上，另记」一条落在本目录 `04`。
