# ADR-0109: 邮编分类事实（分区表、偏远档位表）归 `parcel-pricing` 拥有，作为版本化的「计价参考目录」登记；价卡以目录绑定声明分区与档位的来源，评价由目录解析、查不到即待判断，不给默认

Status: Accepted
Date: 2026-09-04

## Context

[parcel-pricing CONTEXT](../domain/parcel-pricing/CONTEXT.md)「地址分类」词条原句「由邮编分类事实提供的偏远档位」，「特征」词条把分区与地址类型列进闭合的可判定量。代码侧（核于 `main = 512b419`）：`domain.FeatureZone` / `domain.FeatureAddressType` 只与 `ComparisonEquals` 配对，`PricingInputSnapshot` 的 `zone` 是构造时由调用方传入的字符串——**计价只消费分区与档位，不产生它们**。而全仓 `internal/` 与 `docs/domain/` 搜「邮编 / zip / postal / 分区表 / zone chart / DAS」零命中：没有任何上下文拥有「邮编 → 分区」「邮编 → 偏远档位」这两张表（票 [price-card-shape-gaps/01](../../.scratch/price-card-shape-gaps/issues/01-zip-classification-facts-have-no-register.md)，E1 核对第 15 项）。

这两张表是承运商价卡的一部分：FedEx / UPS 的 zone chart 与 DAS 邮编表随资费一起公布、按年度更新，每家末端渠道各有自己的一份；候选租户的材料里这类数据以万行计。**没有载体的后果**：真实价卡登进来之后，评价输入里的 `zone` 与 `addressType` 无处可查——要么由调用方（接受链、择优编排）手填，要么在 E2 转换工具里把邮编档位焊进每张卡；两条都是在没人声明过的地方发明规则，与 AGENTS 红线「只实现已确认规则」正面相抵。

归属有三个候选：`parcel-pricing` 自有目录、`network-routing` 的服务区域、`party-commercial` 的渠道产品附属数据。

## Decision

**一、邮编分类事实归 `parcel-pricing` 拥有。** 判据是「谁读它谁拥有它」加 [ADR-0013](./0013-pricing-owns-versioned-external-reference-series.md) 把外部数值序列判给计价的同一条理由：分区表与偏远档位表是承运商公布的**外部分类事实**，只被计价读，形状上与燃油率序列只差「取值是类别不是数值」。本上下文拥有它的登记、版本化与治理，不生产它的内容——内容来自承运商公布，计价只登来源、生效区间与映射。

**二、它以「计价参考目录」登记，与计价参考序列同族。** 一版目录声明：来源标识（承运商 / 渠道，与序列的来源同形）、目录种类（首发封闭两种：**分区**、**偏远档位**）、始发维度（始发邮编前缀集或始发分区）、目的邮编前缀 → 值的映射、生效区间。版本化、只追加、按计价基准时点选版，复核与在用的门照 [ADR-0099](./0099-price-card-binds-series-identity-and-in-force-version-is-derived-from-review.md) 给序列立的那一套，不另立形。**邮编前缀匹配的粒度（3 位 / 5 位 / 区间）随首份真实分区表定，本记录不预拟**——那是形状里唯一要等实例的一格，机制给槽。

**三、价卡以「目录绑定」声明分区与档位从哪个目录来，绑标识不绑版本。** 形照 `ReferenceSeriesBinding`：一张卡声明「我的分区来自来源 X 的分区目录」「我的偏远档位来自来源 X 的偏远档位目录」；采用哪一版由评价基准时点的在用版本决定（ADR-0099 决定的那条派生规则）。目录版本引用冻进评价清单——`VersionManifest` 装第二类引用，按 [ADR-0108](./0108-version-reference-identity-is-kind-id-version-and-digest-becomes-an-optional-declared-fingerprint.md) 的三元身份。

**四、绑定了目录的卡，`zone` 与偏远档位由评价从目录解析，不再由调用方给；查不到即待判断，不给默认。** 解析失败的原因与 `REFERENCE_SERIES_UNRESOLVED` 同形，命名由实施定（「分区未解析」/「档位未解析」两格分开——前者影响基础运费查表，后者只影响一类附加费，续办不同）。**没有绑定目录的卡保留今天的调用方给值路径**——这不是默认值，是该卡显式没有声明来源；两条路径在卡的内容摘要里分得开。

**五、E2 转换工具与调用方都不得替目录干活。** 裁定落地前 E2 不转邮编分类数据、不把档位焊进卡，缺口项如实列「未转换：等本票」；落地后 E2 把客户材料里的分区表与 DAS 表转成目录登记快照，走目录自己的登记用例与复核。

## Consequences

- **评价输入的分区与档位第一次有了可审的来源**：查的是哪一版目录、按哪个时点选的，都在评价清单里；争议时按同一版目录复算得到同一个分区。
- **`parcel-pricing` 多一类目录与登记面**（目录登记册、按时点解析口、逐字段或模板导入的写面按 ADR-0101 决定八自裁）；「计价参考序列」词条不扩，另立「计价参考目录」词条——两者同族但取值类型不同，混成一个词会让「序列取值是数值」这句话失真。
- **CONTEXT「地址分类」词条改口**：偏远档位由计价参考目录解析得出，不再泛写「由邮编分类事实提供」。
- 代价：`PricingInputSnapshot` 多一条来源路径（目录解析 vs 调用方给），构造门与快照要能分辨；评价清单多一类引用；规范化形状因绑定进卡内容而换号——与 ADR-0107 / ADR-0108 同期实施合并为一次。
- **`network-routing` 的服务区域与本记录无关**：它是运营企业自己网络的语言，承运商分区表是别人的商业分类；两者日后若要对照（某目的邮编既在某承运商 5 区又在自家某服务区域），那是读面的事，不是归属的事。
- **本记录不填任何取值**：没有任何真实分区表或邮编行进仓，SYN 夹具只记 `S`。

## Alternatives considered

- **`network-routing` 的服务区域。** 否决：NR CONTEXT 的「服务区域」与节点覆盖版本是运营企业自己网络的语言；承运商分区表是别人的商业分类，混进一个词条会污染统一语言。且 NR 那一侧被 `PAR-NET-14` 硬阻断，把计价的输入挂在一个实例半边阻断的上下文上，等于让每一次真实评价都等那条阻断解开。
- **`party-commercial` 的渠道产品附属数据。** 否决：PC 不拥有计价语言（PC CONTEXT 首段：不拥有可执行价卡），评价时还得跨界读；「随产品—渠道映射登记」把一份按承运商年度更新的外部事实绑到了商业关系的生命周期上。
- **焊进每张卡 / 在 E2 工具里绕。** 否决：在没人声明过的地方发明规则；且同一承运商的分区表被复制进它的每一张卡，年度更新时要改 N 张卡而不是一版目录。
- **扩「计价参考序列」词条装类别取值。** 否决：序列词条的核心句是「外部数值序列」「取值」，把类别塞进去要么改掉「数值」，要么让读的人在同一个词下分辨两种取值语义——另立同族词条更便宜。

## Links

- [票 price-card-shape-gaps/01](../../.scratch/price-card-shape-gaps/issues/01-zip-classification-facts-have-no-register.md)：取证与三条候选
- [E1 形状核对报告](../../.scratch/price-card-shape-gaps/report.md)：第 15 项
- [parcel-pricing CONTEXT](../domain/parcel-pricing/CONTEXT.md)：「地址分类」「特征」「计价参考序列」词条；本记录新增「计价参考目录」词条
- [ADR-0013](./0013-pricing-owns-versioned-external-reference-series.md)：计价拥有外部序列的理由——本记录把同一理由用到外部分类事实上
- [ADR-0099](./0099-price-card-binds-series-identity-and-in-force-version-is-derived-from-review.md)：绑标识不绑版本、在用版本从复核派生——目录照抄这一套
- [ADR-0108](./0108-version-reference-identity-is-kind-id-version-and-digest-becomes-an-optional-declared-fingerprint.md)：目录版本引用冻进评价清单时的身份形状
