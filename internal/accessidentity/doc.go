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
// # 租户既有渠道的适配那一半仍等 PAR-INT-01；首方渠道那一半已按 ADR-0139 立；操作者与集成客户端两族已立
//
// 首方客户族（ADR-0139 决定二、三：货主客户的系统经本产品自定的 API 交客户命令）不受下面那道否决：ADR-0139 决定一把
// ADR-0072 决定二再收窄到租户既有渠道的适配。它有客户渠道册（CustomerClientRegistry，客户端主体绑定唯一的租户与客户
// 账户，授予按客户面能力登记）、复用集成客户端族凭据机制的令牌校验（adapters/oidc 的 CustomerClientVerifier）与按命令
// 分型的 SourceEnvelope 的铸造（CustomerClientMinter，信封仍只经 seal 产生）。
//
// 操作者渠道不受下面那道否决：ADR-0100 决定一把 ADR-0072 决定二的适用场景收窄为客户接入渠道。
// 操作者那一半已有操作者册（migrations/access_identity/，OperatorRegistry）、OIDC 令牌校验
// （adapters/oidc，OperatorCredentialVerifier）与 OperatorEnvelope 的铸造（OperatorMinter）。
//
// 集成客户端族（ADR-0149 决定三：外部结果与资金事实的提交方是外部系统）同样不受那道否决——外部系统不是客户
// 渠道。它有集成客户端册（IntegrationClientRegistry）、客户端凭据令牌校验（adapters/oidc 的 ClientVerifier，
// 含 RFC 8705 证书绑定）与 IntegrationClientEnvelope 的铸造（IntegrationClientMinter）。
//
// 一线作业事实（ADR-0149 决定二：提交方是运营企业的一线人员）走操作者族的「作业事实登记」能力面：授予按作业范围
// 登在操作者册里，设备单列一本作业设备册（DeviceRegistry），WorkFactEnvelope 由 WorkFactMinter 在令牌、授予、设备
// 三道都过之后铸出。同一本册另有按租户列出已登记设备的查阅口（DeviceCatalogue）：行上只有设备标识、作业范围、
// 登记还是停用，停用了的才带停用时刻。
// 本节以下只说租户既有渠道（客户自有的 API、文件格式与门户）的适配那一半。
//
// ADR-0072 二维持了 ADR-0055 的否决，原句挡的是两件：「登记册**表结构**与**凭据形态**
// 在 PAR-INT-01 最低证据到位前不立」。所以：
//
//   - 表：本包只有装载口 ChannelRegistry 这个接口，仓内没有它的生产实现；
//     migrations/access_identity/ 已随操作者册、集成客户端册与首方族的客户渠道册立起，但其中没有既有渠道适配的表。
//     空册可读是正常态而不是故障——FindChannel 用
//     found=false 而不是 error 表达它（ADR-0052 的分界句：读一个空登记册并如实答未配置
//     不是默认实现，恰恰是它想保护的东西）。
//   - 凭据形态：本包**不知道凭据长什么样**。ChannelCredentialProof 只有一个方法，只交出
//     登记行的查找键；核验交给 CredentialVerifier 的实现方，本包不构造也不拆凭证。写成
//     `{公开键, 秘密}` 那种结构就已经替三种渠道拟了同一种形态——标准文件投递与门户登录
//     都没有「秘密」这一栏。
//
// 本包在既有渠道适配一侧做的因此是「已核验的渠道 → 四要素信封」这一段，不是凭据验证本身。
//
// # 两条门槛不是一条
//
// ADR-0072 的重启门槛是括号里那一件（该租户渠道的现行流程），**已到**；参数登记册
// PAR-INT-01 的最低证据是四件，只到两件半。本轮不立表的理由是后者——S0 缺的那两件决定
// 判重语义，也就决定登记册有哪几列——**不是 ADR 还锁着**。把理由记错的代价在以后：
// S0 齐了那天，接手的人会以为还得先出一份新 ADR 才能立表。首方族不过这两条门槛（ADR-0139 决定一）。
//
// 注：ADR-0072 Consequences 写的顺序是「按渠道形状立册 → 凭据验证与信封铸造 → 逐端点
// 替换」，本包把它倒了过来。倒序的理由与代价记在 .scratch/syn-wall-door-audit 的票 01
// （2026-09-01 那条裁定）：那句顺序在 Consequences 里是推论不是裁决。代价就是此刻装载口
// 只有接口没有实现。
package accessidentity
