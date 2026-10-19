# 10 演示租户 BUY 与 SELL 触发册为空

Category: enhancement
Status: 半收——2026-10-19 SELL 那一半由 [sell-customer-charge/05](../../sell-customer-charge/issues/05-demo-tenant-forms-an-estimated-customer-charge.md) 收：演示账户登了 SELL 计费时点采用（ADR-0190 的包裹时点那一路，不经 SELL 触发册）。余下 BUY 触发册：原因要等 [07](./07-transport-charge-occurrences-have-no-production-path.md) 定下哪种发生项先有生产路径。BUY 那一半由 [26](./26-settlement-consumes-charge-occurrence-formed-to-request-buy-evaluation.md) 收（2026-10-19，代决见 [tf-chain-plan](../tf-chain-plan.md)）。此前 needs-triage——2026-10-18 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自同日对演示动线余下各段的只读勘察（隔离子代理，证据 `S`）；下列现象与证据取自勘察结果，本票立时没有逐条复核，接票时先核。
Blocked by: 无
出处：[product-strategy-boundary/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 2026-10-18 那次重走。

## 现象

演示租户 BUY 触发册为空，Trigger 答 BUY_EVALUATION_TRIGGER_UNCONFIGURED

## 证据（读码）

internal/settlementaccounting/application/trigger_buy_evaluation.go 的 TriggerBuyEvaluationHandler.Trigger 遇 !found 答 BuyEvaluationTriggerUnconfigured；scripts/demo-seeds/seed.sh 的 settlement 段只登 settlement-account、external-funds-fact、funds-mapping、pre-acceptance-estimate-adoption，没有 buy-evaluation-trigger（命令见 cmd/parcel-settlement-register/translate.go 的 commandBuyEvaluationTrigger）。

## 三类

租户取值。规模 S。

## 最小的诚实修法

scripts/demo-seeds/data/settlement 增一份 SYN-TENANT-01 的 buy-evaluation-trigger 输入（原因取停点 1 选定的那一格，时点 OCCURRENCE_FORMED），seed.sh 加一行 parcel-settlement-register buy-evaluation-trigger。

## 要裁的

none（原因随停点 1 的选择定）

## 既有票

none
