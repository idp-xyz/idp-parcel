# 参考设计（外部交付，非权威）

**这个目录里没有任何一条现行规则。** 它是外部提供给本仓的一套参考设计交付物，定性见 [ADR-0012](../../adr/0012-parcel-pricing-context-within-idp-parcel.md)：外部参考输入，不是规格、不是待实现清单、不是验收依据。读它可以了解本仓当初从何处取得语义，不能据它判断本仓应该做什么。

**若某条规则只能靠本目录才读得懂，那是缺陷。** 处置办法是把内容补进本仓权威文档，而不是保留对本目录的引用。

## 权威位置在哪

| 你想找的 | 去这里 |
|---|---|
| 小包计价的领域语言、不变量、源完整性门禁 | [`docs/domain/parcel-pricing/CONTEXT.md`](../../domain/parcel-pricing/CONTEXT.md) |
| 136 个治理案例的本仓副本（逐字转录，已核验保真） | [`docs/reference/golden-cases/`](../golden-cases/) |
| 转录说明、证据层级与阻断状态 | [`计价治理案例转录本`](../../design/pp-golden-case-transcript.md) |
| 本目录各交付物在本仓的落点与吸收结论 | [`参考设计吸收覆盖对照`](../../design/pp-reference-design-absorption-coverage.md) |
| 源价卡抽取的原样归档（仅追溯，未经核实） | [`docs/archive/reference-design-rate-card-extraction-v1.0.1.json`](../../archive/reference-design-rate-card-extraction-v1.0.1.json) |

## 内容与版本

28 个文件，含《国际小包计费与结算平台最终解决方案》V1.0／V1.1／V1.2 三版并存、领域模型 V1.0.1、Rating Runtime 计算语义规范与技术设计 V1.0.1、Rating API 契约 V1.0.1、治理案例集 V1.0.1、MVP 研发实施设计与任务拆解 V1.0.1，以及配套的 schema、manifest 与校验脚本。

三版《最终解决方案》并存不代表哪一版有效。案例集自述的基线只有三份，见吸收对照第 29 行起的说明。

## 为什么留着

案例语料与价卡抽取都已落进本仓，删除本目录在内容上不会丢失任何被引用的东西——这一点在删除前已用逐例比对核实过（136 例全部一致、0 处差异）。保留是产品决定：留一份可回查的一手交付物，代价是必须靠本文件这类抬头挡住误读。

原路径为仓库根目录的 `foo/`，2026-08-10 移入此处并改名。历史文档中的「参考树」一词指的就是本目录。已被取代的 [ADR-0011](../../adr/0011-parcel-pricing-context-within-idp-parcel.md) 正文里三条指向 `../../foo/` 的链接不作修改——红线禁止改写已接受记录，它们保持断链状态。
