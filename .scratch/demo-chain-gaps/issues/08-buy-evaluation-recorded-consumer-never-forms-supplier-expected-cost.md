# 08 BUY 评价已记录的消费门对每一封都答来源引用未登记，供应商预期成本编排没装配

Category: enhancement
Status: needs-triage——2026-10-18 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自同日对演示动线余下各段的只读勘察（隔离子代理，证据 `S`）；下列现象与证据取自勘察结果，本票立时没有逐条复核，接票时先核。
Blocked by: 无
出处：[product-strategy-boundary/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 2026-10-18 那次重走。

## 现象

SA 的 BUY 评价已记录消费门对每一封都答 ErrSourceReferencesUnrecorded，供应商预期成本形成编排没装配

## 证据（读码）

internal/settlementaccounting/adapters/parcelpricing/form_on_buy_evaluation_recorded.go 的 HandleRecordedBuyEvaluation 末行恒返回 ErrSourceReferencesUnrecorded。cmd/parcel-dispatch/assemble.go 的 formSupplierExpectedCostOnBuyEvaluationConsumer 注释写「这里不装 NewFormSupplierExpectedCostHandler」「两头没接上」，已过时：PP 侧回指已在 PricingEvaluation.RequestReference，由 FormEvaluationFromRequestHandler 经 WithRequestReference 设置，sa-cc/11 已 resolved。saports.BuyEvaluationAdoption 没有请求回指字段；SA 没有 SupplierCostVersionID 铸造口（adapters/identity 只有 MintEvaluationRequestID）。

## 三类

机制。规模 M。

## 最小的诚实修法

BuyEvaluationAdoption 补带 RequestReference，由 BuyEvaluationAdapter 从 evaluation.RequestReference() 取。消费门按回指读 sapostgres.EvaluationRequests.FindByID，从记录里的 Sources 组成 FormSupplierExpectedCostCommand，铸一个版本 ID：新增铸造口，或由请求 ID 确定性派生，要定。在 dispatch 装 NewFormSupplierExpectedCostHandler 和 sapostgres.NewSupplierExpectedCosts，从未决名单里删掉 ErrSourceReferencesUnrecorded，改登 ExpectedCostUndecided 那几格。

## 要裁的

版本 ID 铸造还是由请求 ID 派生（SA owner）；PP 内部或重放形成、没有回指的 BUY 评价是答 nil 还是指名未决。

## 既有票

none（票 05 格 18 去处 sa-cc/11 已 resolved，票 05 重走一节已记「去处与代码对不上」）
