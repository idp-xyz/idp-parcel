// Package accessidentity 是共享接入身份技术能力的落点：接入渠道登记、凭据验证与来源
// 信封铸造。五个业务上下文只消费已铸造的来源信封，自己不做凭据验证（ADR-0072 一，
// 落实 parcel-shipment CONTEXT 已点名的那个「共享身份认证与授权技术能力」）。
//
// # 它不是限界上下文
//
// 本包不进 CONTEXT-MAP 业务地图，不拥有业务领域语言，因此**没有 domain 子包**。四要素
// 在这里是四个字符串，业务含义由消费方各自的领域类型承载——把 parcelshipment 的
// SourceIdentity 搬进来就等于让一个技术能力拥有了业务语言。
//
// # 本轮没有登记册的表
//
// ADR-0072 二维持了 ADR-0055 对运行时渠道登记表的否决：表结构与凭据形态在 PAR-INT-01
// 最低证据到位前不立。所以本包只有装载口 ChannelRegistry 这个接口，仓内没有它的生产
// 实现，也没有 migrations/access_identity/。空册可读是正常态而不是故障——FindChannel
// 用 found=false 而不是 error 表达它（ADR-0052 的分界句：读一个空登记册并如实答未配置
// 不是默认实现，恰恰是它想保护的东西）。
//
// 注：ADR-0072 Consequences 写的顺序是「按渠道形状立册 → 凭据验证与信封铸造 → 逐端点
// 替换」，本包把前两步倒了过来。倒序的理由与代价记在 .scratch/syn-wall-door-audit 的
// 票 01（2026-09-01 那条裁定）：Decision 二只挡立册不挡铸造，那句顺序在 Consequences
// 里是推论不是裁决。代价就是此刻装载口只有接口没有实现。
package accessidentity
