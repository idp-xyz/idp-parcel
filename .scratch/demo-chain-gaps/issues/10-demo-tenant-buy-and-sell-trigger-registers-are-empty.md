# 10 演示租户 BUY 与 SELL 触发册为空

Category: enhancement
Status: needs-triage——2026-10-18 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。出自同日对演示动线余下各段的只读勘察（隔离子代理，证据 `S`）；下列现象与证据取自勘察结果，本票立时没有逐条复核，接票时先核。
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
