# 12 演示：FBA 产品缺箱标号停下，受控补充补齐后受理

Category: enhancement
Status: ready-for-agent
Blocked by: 09、10、11

依据：[ADR-0376](../../../docs/adr/0376-fba-submission-conditions-are-judged-as-required-documents-platform-identifiers-may-be-supplied-before-acceptance-and-rule-packages-follow-the-service-product.md) 三项决定；票 08 完成记录「未做」两项。

## 要做的

- 演示租户 `SYN-TENANT-01` 登一个合成的 FBA 头程服务产品与它的接单规则包：规则包指名这个产品、声明「必要资料」组适用、以 `CROSS_FIELD_CONDITION` 引用 `REFCFG-1:parcel-shipment/submission-conditions/FBA-HEADHAUL@1`；产品的价格规则、客户服务规则、授权规则与产品到渠道的映射照小包产品的样子各登一份。
- 演示合同按 ADR-0376 决定三重发一版不在外壳指名接单规则包（随票 11 可缺正文那一格），小包与 FBA 两个产品同一账户各解各的规则包。
- 价卡：演示引擎支持按整票汇总计重的，FBA 产品登一张按整票计费的售价卡（票 08 未做的那一项）；不支持就照实写明。
- 演示段：提交一票缺两箱箱标号的 FBA 货件 → 停在等待受控补充，查阅面列出缺箱标号的那两箱 → 经受控补充补齐 → 已接受；小包产品同时提交照常受理。
- 演示 README 的 FBA 动线补这一段。

## 完成判据

- `seed.sh --reset` 灌进一次性库退 0；演示段跑通，证据记 `S`，参数登记册不动。
