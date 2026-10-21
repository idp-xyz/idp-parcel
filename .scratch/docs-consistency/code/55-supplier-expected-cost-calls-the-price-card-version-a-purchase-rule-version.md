# 55 SA 把评价命中的价卡版本叫「采购规则版本」，注释还说它是 GLOSSARY「价格规则版本」的 PP 半边

Category: chore
Status: ready-for-agent——R3（不急）：只是命名与注释，不改行为；读供应商预期成本的作者会把这一格当成 party-commercial 的采购价格政策版本去溯源，实际存的是 parcel-pricing 的定价方案版本。2026-10-21 文档自洽 10（10a 半边）立（用户「你作为业务和系统专家自决」授权），分支 claude-dc10a。
Blocked by: 无
出处：[audit/cross.md](../audit/cross.md) GL·G-02（归 A，附带代码命名与注释的跟改；修订票不改代码，故立本票）。

## 现象

- GLOSSARY「价格规则版本」（文档自洽 10a 之后）只指 `party-commercial` 拥有的商业价格政策版本，评价命中的那一版价卡是 GLOSSARY「可执行价卡版本」；settlement-accounting CONTEXT 同名词条改为指针，并写明本上下文不把评价命中的价卡版本也叫价格规则版本。
- 代码里供应商预期成本的「规则版本」一格存的是 PP 的定价方案版本，类型名与注释却按「采购规则版本」与「价格规则版本的 PP 半边」叫。

## 证据

- `internal/settlementaccounting/domain/supplier_expected_cost.go`：`PurchaseRuleVersionReference`，注释「指名采购规则版本」。
- `internal/settlementaccounting/adapters/parcelpricing/buy_evaluation.go`：这一格填 `evaluation.PlanReference().ID() + "@" + evaluation.PlanReference().Version()`，即定价方案版本。
- `internal/settlementaccounting/ports/ports.go`：`BuyEvaluationAdoption` 头注「RuleVersion 是评价命中的可执行价卡版本（GLOSSARY「价格规则版本」的 PP 半边）」——GLOSSARY 里已经没有这个半边。

## 三类

机制（命名）。产品策略、租户取值：无。

## 规模

S：类型与构造函数改名为可执行价卡版本一类的词（例如 `PriceCardVersionReference`），注释改指 GLOSSARY「可执行价卡版本」；存储列名不改（列名不是领域语言的出处，改名要迁移，不值）。

## 最小的诚实修法

只改 Go 标识符与注释，行为与库表不动；幂等三维的注释照改，把规则版本那一维写成价卡版本。
