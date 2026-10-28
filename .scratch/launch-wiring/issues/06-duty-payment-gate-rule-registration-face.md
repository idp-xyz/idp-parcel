# 06 「税费付款」门禁规则行没有生产登记面

Category: enhancement
Status: ready-for-agent——2026-10-28 会话 tops-20（idp-parcel-91）立：launch-wiring/03 作者报告；等 01–03 进 main 后开工（同在 customs-compliance）
Blocked by: [01–03](./03-release-gate-verification-production-face.md)（同地盘，先进 main）

## 现象

放行门禁核对与同读法的动作准入，在租户登「税费付款」规则行之前都答 `DUTY_PAYMENT_GATE_RULE_NOT_CONFIGURED`。应用编排与 postgres 写口有，但 `cmd/parcel-customs-register`、registrationjson 译装、HTTP 登记口都没有，演示种子也没种——租户没有办法登它。

## 做什么

照 `cmd/parcel-customs-register` 现有登记册的写法补这一册的登记 CLI 与译装（必要时 HTTP 登记口，照同类册是否有 HTTP 口决定），给演示租户一行 `S` 级参考配置种子；规则取值是租户取值，生产不配默认。

## 完成判据

- 演示租户上登规则后，放行门禁核对答出结论并落册（真库用例或种子灌后实打）。
- 未登时仍答 `DUTY_PAYMENT_GATE_RULE_NOT_CONFIGURED`。
