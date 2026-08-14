# 同币种计价纠错造出了形成门明禁的形状

Category: bug
Status: needs-triage

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
