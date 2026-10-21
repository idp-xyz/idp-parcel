# 40 PC、PP 三处 Go 注释用计数引别处的文档，其中一处已经数错

Category: chore
Status: ready-for-agent——R3（随手可改）：只改注释，不改行为。读注释的人照「四种」去数 PP CONTEXT「计算方法」会少一种，另两处今天数得对，但被数的那一侧增减一项时它们无声地变错（AGENTS.md「改文档」：Go 注释里的跨文件引用同受此约束，计数与行号同构）。2026-10-21 文档自洽 07 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc07。
Blocked by: 无
出处：[audit/pc-pp.md](../audit/pc-pp.md) L6（审计钉 `6b0074d0`；2026-10-21 按 main `73a2c4a79` 复核，三处注释原样在）。

## 现象

三处注释用「几种」「几项之一」「只留几格」指称另一个文件里的清单。

## 证据

- `internal/parcelpricing/domain/value_objects.go` 的 `ChargeMethod` 注释：「CONTEXT 把取值集合闭合为四种」。PP CONTEXT「计算方法」词条列的是定额、查表、按基数百分比、取当期序列定额，以及其中两者取较大值；代码常量含 `ChargeMethodSeriesAmount`。已经数错。
- `internal/partycommercial/domain/credit_limit.go`：「「入账余额」是 SA CONTEXT 运营结算余额五项之一的原词」。今天数得对，被数的是 SA CONTEXT。
- `internal/partycommercial/adapters/postgres/commercial_resolution.go` 的 `settlementPolicyDocument.policy` 注释：「ADR-0044 只留两格」。今天数得对，被数的是 ADR-0044 的方式集合。

## 三类

机制：注释写法。产品策略、租户取值：无。

## 规模

S：三处注释。

## 最小的诚实修法

三处都改为引词条名或原词，不写数：`ChargeMethod` 注释改为「取值集合由 CONTEXT「计算方法」词条闭合」；`credit_limit.go` 改为「「入账余额」是 SA CONTEXT「运营结算余额」的原词」；`commercial_resolution.go` 改为「ADR-0044 的方式只有预付与账期」——数本身不是论点，不必锚 SHA。

## 要裁的

无。
