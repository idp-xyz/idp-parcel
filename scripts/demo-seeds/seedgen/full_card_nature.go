package main

// 第二张完整售价卡 SYN-PLAN-CN-SG-FULL-02（范围 SYN-SCOPE-03）：FULL-01 复制而来，改两处，都是引擎补上能力之后才写得出的
// （ADR-0347）：
//
//   - 超重费按计价重量（CHARGEABLE_WEIGHT）超过 20 千克触发，而不是实重：实重 5 千克、尺寸折出 24 千克体积重的轻泡件，
//     计价重量 24 千克，收超重费。真实承运商的超重费常按这个口径。计价重量是包裹自身的、进位之后的值，不含条件最低
//     计价重量的抬高（本卡也没有这样的条款）。
//   - 多一条住宅派送费：收件地址性质为住宅则加收 6 CNY。它与偏远档位是两个独立维度，独立计收，同一票既是住宅又落在偏远
//     档位时两笔都收——这正是地址性质另占一个特征槽（ADDRESS_NATURE）而不挤进地址类型那一槽（偏远档位）的原因。
//
// 读了地址性质的卡，评价要发起方声明地址性质：试算页不选就点名缺项，不套默认住宅或商业。正式评价路径上，包裹受理一侧今天
// 没有结构化的地址性质（ADR-0347 决定四），这张卡的正式评价会如实待判断，演示动线不用它。
//
// 与 FULL-01 共用两本目录与燃油序列（绑的是标识不是版本），价表、重量策略与费率行另起标识，不与 FULL-01 共享。

// fullNatureSpec 是 SYN-PLAN-CN-SG-FULL-02 的几格。
var fullNatureSpec = fullCardSpec{
	planID:                 "SYN-PLAN-CN-SG-FULL-02",
	tableID:                "SYN-TABLE-CN-SG-FULL-02",
	weightPolicyID:         "SYN-WEIGHT-CN-SG-FULL-02",
	rateIDPrefix:           "SYN-RATE-CN-SG-FULL2",
	scope:                  fullNatureScopeID,
	sourceFileName:         "SYN-CARD-CN-SG-FULL-02.xlsx",
	overweightOnChargeable: true,
	residentialDelivery:    true,
}

func fullNatureSellCardSnapshot() []byte {
	return buildFullSellCard(fullNatureSpec)
}
