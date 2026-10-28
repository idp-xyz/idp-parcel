# 06 「税费付款」门禁规则行没有生产登记面

Category: enhancement
Status: resolved——2026-10-28 进 main（`7f2256f90`；全量 160 ok / 0 FAIL，种子退 0），见文末「进 main 记录」。此前 in-progress——2026-10-28 作者完工，待隔离评审与进 main，分支 claude-lw-duty（代码最后一笔 a0bbe2be9）；此前：ready-for-agent——2026-10-28 会话 tops-20（idp-parcel-91）立：launch-wiring/03 作者报告；等 01–03 进 main 后开工（同在 customs-compliance）
Blocked by: [01–03](./03-release-gate-verification-production-face.md)（同地盘，先进 main）

## 现象

放行门禁核对与同读法的动作准入，在租户登「税费付款」规则行之前都答 `DUTY_PAYMENT_GATE_RULE_NOT_CONFIGURED`。应用编排与 postgres 写口有，但 `cmd/parcel-customs-register`、registrationjson 译装、HTTP 登记口都没有，演示种子也没种——租户没有办法登它。

## 做什么

照 `cmd/parcel-customs-register` 现有登记册的写法补这一册的登记 CLI 与译装（必要时 HTTP 登记口，照同类册是否有 HTTP 口决定），给演示租户一行 `S` 级参考配置种子；规则取值是租户取值，生产不配默认。

## 完成判据

- 演示租户上登规则后，放行门禁核对答出结论并落册（真库用例或种子灌后实打）。
- 未登时仍答 `DUTY_PAYMENT_GATE_RULE_NOT_CONFIGURED`。

## 完成记录

- 起点 SHA：`df7ecc710`；代码提交 `a0bbe2be9`；分支 `claude-lw-duty`。
- 改了什么：`registrationjson.DutyPaymentGateRuleFromJSON`（三维键 + notAPrecondition 或三接受集合，`decodeStrict`，字面同付款核对词表）；`parcel-customs-register` 加 `duty-payment-gate-rule` 命令调 `RegisterDutyPaymentGateRule`；种子 `data/customs/32-duty-payment-gate-rule-not-a-precondition.json` 在 250 节紧随 gate-finding 登记；486 节注释改成现行。无 HTTP 口、无迁移。
- 代决：D1 不开 HTTP 登记口（评审最小补法；同类 gate-catalog 有在线口，规则行暂只走受控批量口）。D2 演示值取「不构成前置条件」，种子注释写明是演示采用、非产品默认。D3 种子编号取 32（origin/main 与各 claude- 分支核过无占用）。无新 AT、无 UC 文档句需改（UC-CC-009 无「无登记面」类现状句）。
- 越权风险点：无；演示规则与 SYN-SCOPE-01/CROSS_CUSTOMS_MOVEMENT/SYN-PROC-CN-EXPORT 门禁同键，不影响其他键。
- 判据对用例：登前答 DUTY_PAYMENT_GATE_RULE_NOT_CONFIGURED、登后 RECORDED + MET 落册、重放 EXISTING、换规则 CONTENT_CONFLICT、两形混写不登——均由 cmd/parcel-customs-register 的 TestDemoDutyPaymentGateRuleSeedLetsTheReleaseGateBeVerified 读种子文件本身在真库证明（用真 VerifyReleaseGateHandler 与真适配器，下游交接用丢弃替身；未经 HTTP 口，HTTP 层已由 launch-wiring/03 的 parcel-api 真库用例覆盖）。
- 自验（含 PG 55432）：go build ./...、go vet、gofmt 无输出；go test -p 1 ./cmd/parcel-customs-register ./internal/customscompliance/... ./internal/architecture/... ./migrations/... 全绿；一次性库 seed.sh 全程零报错（duty-payment-gate-rule: REGISTERED），库已删。
- 留给协调方：无阻上线的新发现。AT 盘点/机制清点/ADR 索引生成物进 main 时重生成（本票未加 AT、未动 ADR）。

## 评审记录（2026-10-28，隔离子代理，钉 `bd43637a4`）

两轴无阻断。载荷与 `RegisterDutyPaymentGateRuleCommand` 一一对上，两形互斥由应用层与领域把门；`decodeStrict` 拒未知字段；CLI 分支与 `gate-finding` 同形；种子只给演示租户并写明演示采用；真库用例读种子文件本身，未登答未配置、登后 `MET` 落册、重放、冲突、两形混写被拒。只走受控批量口、不开 HTTP 登记口：与税费付款核对等只走批量口的册同类，规则取值属租户上线时的实施，批量口足够。非阻断：测试里手写后缀比对可换 `strings.HasSuffix`。

## 进 main 记录（2026-10-28，推送方 tops-20 / idp-parcel-91）

- 分支 `claude-lw-duty` tip `bd43637a4` 作出处；第一次在 main `df7ecc710` 上验过后 main 进了 psb/22 第 2 笔（含代码），改在 main `463d3af1d` 上重放重验。
- 重生成生成物、两项 `-check` 退 0；全量 160 ok / 0 FAIL；种子一次性库退 0；推确切 SHA `7f2256f90`。

