# 14 parcel-pricing：邮编前缀匹配形态与公开标准的参考配置

Category: enhancement
Status: needs-triage——2026-09-24 通道 4 随票 02 立（以「实例半边」为由暂缓的重新定性，PP 一张）
Blocked by: 第 2、3 项的参考配置等 03
地盘：parcel-pricing 计价参考目录（分区 / 偏远档位）的形状与计价输入的单位对表；参考配置存放按 03。
出处：[票 02](./02-split-parameter-register-and-retriage-deferrals.md) 暂缓清单——[ADR-0109](../../../docs/adr/0109-zip-classification-facts-are-owned-by-parcel-pricing-as-a-versioned-reference-catalogue.md) 决定二「邮编前缀匹配的粒度（3 位 / 5 位 / 区间）随首份真实分区表定，本记录不预拟」；[`pp-pricing-input-seams/03`](../../pp-pricing-input-seams/issues/03-ps-origin-destination-postal-route-read-port.md)「邮编格式校验、前缀粒度属实例半边」；[ADR-0048](../../../docs/adr/0048-declared-measurements-are-verbatim-member-profiles.md)「真实单位目录……属实例半边」。按 [ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定二重新定性：匹配粒度是形态，归产品；公开标准的实例数据归参考配置。

## 做什么

1. **邮编前缀匹配形态**：3 位、5 位、区间等作为内置形态，一版目录声明自己用哪种；目录的前缀与值仍是租户取值（承运商分区表）。
2. **各国邮编格式**（公开标准）：按国家出参考配置，供校验与规范化；租户显式采用才生效，没采用照旧只存客户给的串。
3. **单位对表**（公开标准）：重量、尺寸单位到计价所用单位的换算出参考配置；接入侧给的单位串原样保留。与 `pp-pricing-input-seams` spec「不在本目录」预告的 PP 消费侧适配器票同题（那里写「单位对表……是那张票的事」），那张票立起时并过去，不做两遍。

首发随附哪些国家的格式，归 PP owner 与用户定（与 ADR-0146 越权风险点 5 同一口径）。

## 不做

- 不填任何承运商的分区表、偏远档位或价卡；不替租户选匹配形态。

## 完成判据

- 目录能声明匹配形态并按它匹配（带测试）；邮编格式与单位对表各有一份可显式采用的参考配置样板，未采用的租户行为不变。
