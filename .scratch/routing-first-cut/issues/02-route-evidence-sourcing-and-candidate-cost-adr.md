# 02 路由证据由谁供哪一族事实、路由候选成本怎么来——先裁后做

Category: enhancement
Status: resolved——2026-09-24 通道 5 完成：ADR-0148 经用户授权自决接受，CONTEXT 与 CONTEXT-MAP 引用句随接受之笔落下，完成记录见文末。此前：in-progress——2026-09-24 通道 5 认领（单 task-af4117c9-5176-468b-953f-fef79b86ccad），共享树 main 上做、pathspec 提交、不推；预留 ADR-0148，起草为 Proposed（接受归用户或其明确授权）。此前：ready-for-agent
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

- [x] ADR Accepted，上列各问各有决定；越权风险点写明碰到的邻接上下文（PS、PP、CC）由谁复核。
- [x] network-routing CONTEXT 与 CONTEXT-MAP 只加引用句，不复述决定。

## Comments

- 2026-09-24 通道 5：草案 [ADR-0148](../../../docs/adr/0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 已落（Proposed），上列各问各有决定，越权风险点点名 PS、PP、CC、SA owner 与用户各复核哪一条；索引补了一行。本票保持 in-progress：判据要 Accepted，接受归用户或其明确授权（用户授权自决的是 psb04 的拆法，不含本 ADR）。CONTEXT 与 CONTEXT-MAP 的引用句照草案先例留到接受那一笔再加，届时本票转 resolved。关务一侧确认缺执行器（CC 没有按路由候选作答的关务适用性判断口），已登 [psb/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 格 6 并报通道 1 另立票。
- 2026-09-24 通道 5：用户问「专业、科学的解决方案」，通道 5 建议后用户答「按你的建议吧」，据此修订 ADR-0148 决定四与越权风险点一、二——候选成本按比较币种合成（比较币种是路由策略版本上的租户取值，汇率口径由所引价格政策声明，换算归 PP、逐段留痕，先合计后取整一次）；每段成本依据是 BUY 评价或内部价格政策评价，自营段走后者；公共段剔除列为允许的优化。修订后仍为 Proposed：用户这句是让按建议改草案，接受仍待用户明言或明确授权。

## 完成记录（2026-09-24，通道 5，共享树 main）

**接受**：用户在 IDP 队列授权通道 5 自决（原话「你作为业务，系统专家，参考头部软件的做法，自决吧」）。通道 5 对照头部运输管理系统的做法逐条复核后接受 ADR-0148：事实按来源分三路、成本按比较币种换算、自营段以标准成本与外包同口径、关务未过筛即不放行，四条与主流做法一致；另在决定二补记邮编区间与行政区域两种后续覆盖形态，在决定五补记拼接按有界搜索生成（换乘次数上限与可换乘节点是租户取值）。首版范围不变。

**落点**：`f9d70b16` 草案；`b4591db6` 按用户建议修订决定四；接受之笔（本票转 resolved 的这一笔）——ADR-0148 Status → Accepted、索引行改接受口径、network-routing `CONTEXT.md`「路由形成与选择」一节末尾加一条引用、CONTEXT-MAP 的「parcel-shipment → network-routing」「parcel-pricing → network-routing」「customs-compliance ↔ network-routing」各加一句引用。

**各问决定**（全文在 ADR-0148）：一、事实族分目录折叠、随请求携带、经端口取三路；二、地理投影是 PS 地址要素的寄件段与收件段，首版覆盖文法为整国家 / 地区与国家加邮编前缀；三、关务适用性经端口向 CC 取，CC 侧缺执行器已另立 12；四、候选成本按比较币种合成，成本依据逐段为 BUY 评价或内部价格政策评价；五、首版一条完整线路即一个候选；六、`未配置`改由有无适用的路由策略版本答、视图修订取目录修订锚，部分停用 ADR-0068 决定六与 ADR-0053 决定四一格，自 07 落地起生效。

**解除的阻塞**：02 这条边对 07、08、10、12 都解除了。07 由此可开工；08 还等 07，10 还等 03、09；12 还等 CC owner 分诊。

**门**：纯 md，推送方自审。改动文件无 CR、无 BOM；新增链接逐个解析到实存文件；提交前核过这几份相对 HEAD 只有本笔改动。
