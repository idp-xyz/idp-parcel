# 56 受控关闭与重开的注释与 Covers 仍引已删的合同委派例外句；渠道方叫停形成的关闭今天可以直接重开

Category: chore
Status: in-progress——S 半边（注释与 Covers）随 code/24 在分支 claude-code24 完工待评审与进 main，见文末完成记录；M 半边（解除门）照旧等来源可读。原状态行：ready-for-agent——R3（不急）：注释一半只是说法，不改行为，但会让读者以为「首发不开、日后改这道拒绝即可开」；解除那一半是已知缺口，今天渠道方叫停靠运营角色转成关闭，重开同样在运营一侧，没有自动化的绕过路。2026-10-21 文档自洽 10（10a 半边）评审轮立（用户「你作为业务和系统专家自决」授权），分支 claude-dc10a。
Blocked by: 无（解除那一半要先有硬限制解除的来源可读，见「要裁的」）
出处：文档自洽 10a 隔离评审 B3 与建议 1；裁定 [ADR-0275](../../../docs/adr/0275-controlled-closure-and-reopening-are-decided-only-by-operator-roles-and-a-channel-stop-is-an-external-restriction.md)。

## 现象

- ADR-0275 决定一：受控关闭与重开只由运营侧授权角色形成，合同委派不参与，这一例外是规则上不存在，不是「首发不开」。parcel-shipment CONTEXT 里「除非客户合同或明确授权把其纳入可直接形成决定的授权角色范围」那半句已删。
- 代码里三处注释与 Covers 仍逐字引那半句，并按 pc-gaps/13 裁决 ③ 写「例外支首发不开、留位」「日后开例外支改的是这一道拒绝」。
- ADR-0275 决定三：渠道服务方或代理商叫停记为外部限制，解除只恢复可以申请并授权重开的资格。重开编排今天不为硬限制的解除加门，渠道方叫停形成的关闭，授权角色可以直接重开。

## 证据

- `internal/partycommercial/domain/authority_grant.go`：`ErrCustomerAccountCannotDecide` 的注释引 PS CONTEXT 那半句，写「例外支首发不开、留位」与「日后开例外支改的是这一道拒绝与一条新的委派方向」。
- `internal/partycommercial/domain/authority_grant_test.go`：`TestACustomerAccountMayOnlyRequestClosureAndReopening` 的 Covers 引同一半句与「例外支首发不开、留位」。
- `internal/parcelshipment/adapters/partycommercial/continued_attempt_decision_authorization.go`：`ContinuedAttemptDecisionRequestCoordinates` 的结构注释「pc-gaps/13 裁决 ③：例外支首发不开」。
- `internal/parcelshipment/application/form_continued_attempt_decision.go`：`ClosureResponsibilitySourceKind` 头注说硬限制那一格的解除今天没有来源可读，本编排不为它加门。

## 三类

机制。产品策略已由 ADR-0275 定；租户取值：无。

## 规模

S（注释与 Covers）：三处改为指向 ADR-0275 与 party-commercial CONTEXT「两者都是运营侧凭授权规则形成的决定，合同委派不参与。」，去掉首发不开、日后改这道拒绝即可开的说法；`ErrCustomerAccountCannotDecide` 的行为与测试断言不变。

M（解除门，另起时）：重开编排对来源为外部限制的原关闭，读该限制来源的有效解除再放行；要先有可读的来源。

## 最小的诚实修法

先做注释与 Covers 那一半。解除门在来源可读之前不加——加一道读不到来源的门等于把重开一律拒掉，与 `ClosureResponsibilitySourceKind` 头注的理由相同；在那之前，运营角色重开前自己核渠道方已解除，ADR-0275 越权风险点 3 已写明。

## 要裁的

- 外部限制的解除从哪里读：渠道账号撤销、到期等已有来源的直接读，渠道方口头恢复那一类是否要运营先登一条解除记录。归 PS owner。

## 完成记录（2026-10-21，分支 claude-code24，S 半边）

与 code/24 同一片代码，一并做；用户授权原话「你作为业务和系统专家自决……解决所有的问题」。

### 做了什么

- `internal/partycommercial/domain/authority_grant.go`：`ErrCustomerAccountCannotDecide` 的注释改为引 party-commercial CONTEXT「两者都是运营侧凭授权规则形成的决定，合同委派不参与。」与 ADR-0275 决定一。去掉「例外支首发不开、留位」与「日后开例外支改的是这一道拒绝」，改写为：要开这条路须另立记录改授权规则，不是改这一道拒绝。错误值、行为与断言不变。
- `internal/partycommercial/domain/authority_grant_test.go`：`TestACustomerAccountMayOnlyRequestClosureAndReopening` 的 Covers 同样改指那句与 ADR-0275 决定一。
- `internal/parcelshipment/adapters/partycommercial/continued_attempt_decision_authorization.go`：`ContinuedAttemptDecisionRequestCoordinates` 的结构注释删去「pc-gaps/13 裁决 ③：例外支首发不开」，改指 ADR-0275 决定一、二与同一句 CONTEXT。
- `internal/parcelshipment/application/form_continued_attempt_decision.go`：`ClosureResponsibilitySourceKind` 头注补一句：渠道服务方或代理商叫停形成的关闭也落在外部限制那一格（ADR-0275 决定三），在解除可读之前由授权角色重开前自己核来源方已解除（ADR-0275 越权风险点 3）。不加门，理由照旧。

### 代决

1. `ClosureResponsibilitySourceKind` 头注多补一句指向 ADR-0275 决定三与越权风险点 3。票面只列了这一处作证据，没要求改；补上是为了不让读者以为渠道方叫停另有一条路。

### 越权风险点

无新增。解除门（M 半边）未动，「要裁的」照旧归 PS owner。

### 验证

同 code/24 完成记录的验证一节（同一分支、同一跑）。

