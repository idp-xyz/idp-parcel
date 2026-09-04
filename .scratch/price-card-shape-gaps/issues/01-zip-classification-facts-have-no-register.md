# 邮编分类事实（分区表、偏远档位表）在仓内没有登记载体，计价的类别特征只有消费侧

Category: enhancement
Status: in-progress——MCP-3 接手实施（2026-09-04，隔离分支 `mcp3-pp-shapegaps`，基线 main `ae7b4c8a`，task-de0159af 换号批续作四票之一）；此前 MCP-6 领了未开工（2026-09-04，隔离分支 `mcp6-pp-renumber`，基线 main `4cc1bc34`，task-2b9edfe4 换号批五票之一；交接点见文末 Comments）；两件已裁（2026-09-04，通道 6，owner 授权）：归属取 1（`parcel-pricing` 自有「计价参考目录」），形状照票面第二问并落文 [ADR-0109](../../../docs/adr/0109-zip-classification-facts-are-owned-by-parcel-pricing-as-a-versioned-reference-catalogue.md)，CONTEXT 已补词条「计价参考目录」并改口「地址分类」；本票转实施票，范围见「裁决」节末段
Blocked by: 无

## 为什么立

[E1 形状核对](../report.md) 第 15 项。parcel-pricing CONTEXT「地址分类」词条原句：「由邮编分类事实提供的偏远档位」；「特征」词条把分区、地址类型列进闭合的可判定量。代码侧对应：`domain.FeatureZone` / `domain.FeatureAddressType` 只能与 `ComparisonEquals` 配对；`PricingInputSnapshot` 的 `zone` 是构造时传入的字符串，地址类型随「地址与商业维度」进 `PackageFeatures`。**即：计价只消费分区与档位，不产生它们。**

而全仓（`internal/` 与 `docs/domain/`，核于 `a17bfac`）搜「邮编 / zip / postal / 分区表 / zone chart / DAS」零命中——没有任何上下文拥有「邮编 → 分区」「邮编 → 偏远档位」这两张表。候选租户的需求材料里这类数据有一万三千余行（登记册草案「13,092 行商派邮编覆盖」），每家末端渠道各有自己的分区表与偏远邮编表，且按承运商年度更新。

**没有载体的后果**：真实价卡登进来之后，评价输入里的 `zone` 与 `addressType` 无处可查——要么由调用方（PS 接受链 / 择优编排）手填，要么在 E2 转换工具里把邮编档位焊进每张卡。两条都是在没人声明过的地方发明规则。

## 要裁的两件

**一、归属。** 三条互斥：

1. **parcel-pricing 自有的「计价参考目录」**：与计价参考序列同族（版本化、按时点选版、评价清单冻结引用），只是取值是类别不是数值。理由：分区表是承运商价卡的一部分（FedEx/UPS 的 zone chart 随资费一起公布），且只被计价读。代价：PP 要多一类目录与登记面；「参考序列」词条要扩或另立词条。
2. **network-routing 的服务区域**：NR CONTEXT 已有「服务区域」与节点覆盖版本，DAS 档位读起来像「派送区域的可达性分类」。代价：NR 的服务区域是运营企业自己网络的语言，承运商分区表是别人的商业分类，两者混进一个词条会污染统一语言；且 NR 那一侧被 `PAR-NET-14` 硬阻断。
3. **party-commercial 的渠道产品附属数据**：分区表随「产品—渠道映射」登记。代价：PC 不拥有计价语言，评价时还得跨界读。

倾向（不是裁决）：1。判据是「谁读它谁拥有它」与 ADR-0013 把外部数值序列判给计价的同一条理由；分区表是承运商公布的外部分类事实，形状上与燃油率序列只差「取值是类别」。

**二、形状。** 若取 1：一张目录版本 =（承运商/渠道标识、始发邮编前缀集或始发分区、目的邮编前缀 → 分区 或 → 偏远档位、生效区间）；查询按计价基准时点选版；`PricingInputSnapshot` 的 `zone` 从「调用方给」改为「由目录解析、并把目录版本引用冻进评价清单」（`VersionManifest` 已能装第二类引用）。邮编前缀匹配的粒度（3 位 / 5 位 / 区间）随首份真实分区表定，不预拟。

## 红线

- 不写任何真实分区表或邮编行进仓；SYN 夹具只记 `S`。
- 裁定前 E2 转换工具**不转**邮编分类数据，缺口项如实列「未转换：等本票」。
- 不在评价里给未登记的邮编一个默认分区或默认档位——查不到即评价原因「分区未解析」，与 `REFERENCE_SERIES_UNRESOLVED` 同形。

## 裁决（2026-09-04，通道 6，task-f530ad56 裁决批口径：owner 授权自决，写明能力边界）

**归属取 1，形状照第二问，落文 [ADR-0109](../../../docs/adr/0109-zip-classification-facts-are-owned-by-parcel-pricing-as-a-versioned-reference-catalogue.md)。** 判据是「谁读它谁拥有它」加 ADR-0013 把外部数值序列判给计价的同一条理由：分区表是承运商公布的外部分类事实，只被计价读，与燃油率序列只差取值是类别。2 否决因 NR 服务区域是运营企业自己网络的语言且被 `PAR-NET-14` 硬阻断，3 否决因 PC 不拥有计价语言、评价还得跨界读。ADR 五条：归 PP；以「计价参考目录」登记（来源、种类封闭两种、始发维度、目的邮编前缀映射、生效区间；版本化、复核、在用照 ADR-0099；前缀粒度随首份真实表来）；价卡以目录绑定声明来源、绑标识不绑版本、目录版本引用冻进评价清单；绑定了的卡由目录解析、查不到即待判断（分区 / 档位两格分开）、不给默认，未绑定的卡保留调用方给值路径且两者在摘要里分得开；E2 与调用方都不替目录干活。CONTEXT 同笔加词条「计价参考目录」、改口「地址分类」（AGENTS「改文档」）。

**能力边界**：读了 PP CONTEXT 的「地址分类」「特征」「计价参考序列」「计价输入快照」词条、ADR-0013 / 0099、`report.md` 第 15 项与本票取证（全仓零命中一条本轮未重跑，沿用 `a17bfac` 的取证）；**未读** NR CONTEXT「服务区域」词条原文（只据票面转述）与 `PricingInputSnapshot` 构造函数的完整参数面；未打开任何客户材料。

### 本票转实施票，范围

1. 领域：目录值对象（来源、种类、始发维度、映射、生效区间）、目录版本与登记 spec、`CatalogueBinding`（形照 `ReferenceSeriesBinding`）进 `PricingPlanVersion` 内容与规范化文档（换号，与 ADR-0107 / 0108 同期合并为一次）；评价解析分区与档位并把目录版本引用冻进 `VersionManifest`（ADR-0108 三元）；两格待判断原因；未绑定路径保留。
2. 持久化：目录登记册 + 按时点解析口 + 复核记录（照序列那套，`0a67406` / ADR-0099 实施为样板）；迁移号开工时重取。
3. 写面：按 ADR-0101 决定八自裁——万行级映射是模板导入而不是逐字段表单；受控批量口照 `parcel-pricing-register` 家族。
4. E2：把客户分区表与 DAS 表转成目录登记快照，走目录登记用例；裁定落地前如实列「未转换：等本票」。
5. 领域封闭集与快照改动走三步法或单独 worktree，频道占号。

## 验证

裁决记进 CONTEXT（归属改动按 AGENTS「改文档」），必要时 ADR 编号落进上面某一条，再按所选拆实施票；本票 `resolved` 的判据是那条引用在。

## Comments

- 2026-09-04 · MCP-1：立票。起因是 E1 核对到第 15 项时发现触发侧齐、提供侧空。**只写票面，未动代码。**
- 2026-09-04 · 通道 6：换号批里领了本票，**一行未写**。交接点：规范化号已在 `b8dfc9a8` 换成 PPC-5（`fingerprint.go` 的 `canonicalizationVersion` 注释预告本票的目录绑定落这一号，**不再换号**，规范化文档新增字段照 `amount_rounding` 那样 `omitempty` 即可）；版本引用是三元 + 可选指纹（`NewVersionReferenceIdentity`），目录版本引用冻进 `VersionManifest` 用它；`CatalogueBinding` 进 `PricingPlanStructures` 可照 `WithAmountRounding` 的 builder 写法（可缺的另一个轴，不改 `NewPricingPlanVersion` 签名）；迁移取 `parcel_pricing/0006`（本批占号广播已点名）；解析口与复核记录照 `0a67406` / ADR-0099 那套。
