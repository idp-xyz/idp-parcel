# 02 路由证据由谁供哪一族事实、路由候选成本怎么来——先裁后做

Category: enhancement
Status: draft
Blocked by: 无
父票：[psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md)「接路由证据取数侧」那一步的前置裁决
地盘：一份新 ADR（编号开工时取）；network-routing [`CONTEXT.md`](../../../docs/domain/network-routing/CONTEXT.md) 与 [CONTEXT-MAP](../../../docs/domain/CONTEXT-MAP.md) 相关边只加引用句。不写代码。
出处：[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定二、七；[ADR-0075](../../../docs/adr/0075-customer-address-is-carried-with-the-routing-request.md)（地址随请求携带，「投影的具体字段形状……随取数侧实现票定」）；[`first-tenant-runway/03`](../../first-tenant-runway/issues/03-network-resolution-layer.md) 的 Answer；[`label-channel-service-first-release/13`](../../label-channel-service-first-release/issues/13-buy-evaluation-to-cost-score-bridge.md)（`parcelpricing` 只答算不算得出价，出局归择优侧）。

## 做什么

两个证据视图要一次交出的事实族里，只有一部分出自版本化网络目录。逐族定来源，定路由候选的成本口径，落一份 ADR：

1. **出自目录折叠的**：服务区域解析、候选生成、路径可执行性（含临时网络可用性调整）、时间投影、段链、策略引用、视图修订。
2. **随请求携带的**（ADR-0075 同款）：地理解析投影的字段形状（随服务区域覆盖文法走）；服务要求与承诺上界是否同走请求。
3. **经端口取的**：候选的关务资格（`customs-compliance`）、候选成本（`parcel-pricing`）。关务一侧若今天没有按候选作答的判断方法，按 ADR-0146 登为又一处缺执行器，交 [psb/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 的动线取证并另立票，不在本票族里补。
4. **路由候选成本口径**：一个候选由若干计划履约段组成，成本怎样由各段的 BUY 评价合成（各段按自己的体积系数与进位算计价重量——`PAR-NET-16` 已确认的那句）；币种不齐怎么办；任一段待判断或不可计价时候选怎么处置。PP 只答算不算得出，出局与并列归 NR 的择优（label-channel/13 的分工先例）。
5. **首版候选生成形态**：起止节点间每条适用线路各成一个候选，还是允许线路拼接。这是产品策略（ADR-0146 决定二），租户只填取值。
6. **定义登记册（迁移 `0007`）与目录（迁移 `0008`）的分工**：first-tenant-runway/03 的 Answer「`0007` 与 `0008` 不合流」一节已答「不合流，视图修订改由目录修订锚派生」，本 ADR 承接或推翻并写理由。[ADR-0068](../../../docs/adr/0068-versioned-network-catalog-structure-precedes-rule-content.md) 决定六（证据视图不读本目录）的部分停用写成**随本票族 07 落地的那一笔生效**，不留「ADR 已说可读、代码读不了」的中间态。

## 不做

- 不写实现；不定任何租户的网络内容、阈值、日历或截单。

## 完成判据

- [ ] ADR Accepted，上列各问各有决定；越权风险点写明碰到的邻接上下文（PS、PP、CC）由谁复核。
- [ ] network-routing CONTEXT 与 CONTEXT-MAP 只加引用句，不复述决定。
