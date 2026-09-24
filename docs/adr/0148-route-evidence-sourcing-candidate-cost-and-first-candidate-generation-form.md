# ADR-0148：路由证据按来源分三路——目录折叠、随请求携带、经端口取；路由候选成本由逐段成本依据（BUY 评价或内部价格政策评价）按比较币种换算后合成，任一段缺依据即缺成本依据；首版候选生成以判断时点适用的一条完整线路为一个候选；`未配置`改由有无适用的路由策略版本答，视图修订取目录修订锚

Status: Accepted（2026-09-24，通道 5 起草，票 [routing-first-cut/02](../../.scratch/routing-first-cut/issues/02-route-evidence-sourcing-and-candidate-cost-adr.md) 的产物。同日按用户「按你的建议吧」修订决定四与越权风险点一、二：候选成本改按比较币种换算后合成，自营段经内部价格政策计成本。随后用户在 IDP 队列授权通道 5 自决接受——原话「你作为业务，系统专家，参考头部软件的做法，自决吧」；通道 5 对照头部运输管理系统的做法复核后接受，并在决定二、五补记后续形态。越权风险点待各上下文 owner 复核。决定六对 ADR-0068、ADR-0053 的部分停用自 routing-first-cut/07 落地的那一笔起生效，在那之前两份记录照旧。裁决能力边界：读过 network-routing [`CONTEXT.md`](../domain/network-routing/CONTEXT.md) 全文，[CONTEXT-MAP](../domain/CONTEXT-MAP.md) 中 network-routing 的各条边，[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 全文，[ADR-0075](./0075-customer-address-is-carried-with-the-routing-request.md) Decision，[ADR-0068](./0068-versioned-network-catalog-structure-precedes-rule-content.md) 决定六，[ADR-0053](./0053-network-fact-families-are-derived-not-registrable.md) Decision，[ADR-0145](./0145-legal-entity-attributes-split-into-identity-layer-and-dated-profile.md) 决定四，票 `first-tenant-runway/03` 的 Answer、`nr-route-evidence-views/01`、`label-channel-service-first-release/01` 的裁决与 `/13` 的票头；代码读过 network-routing 的证据视图端口与初始路由判断管线、迁移 `0008` 的头注与表结构、parcel-shipment 的地址要素类型与渠道择优取成本的适配器；customs-compliance 与 parcel-pricing 的 `CONTEXT.md` 只按「口岸 / 申报路径 / 关务区域」「汇率 / 换算」检索读了相关句（含 PP 的「外部数值序列」「换算」词条与汇率口径不变量），party-commercial `CONTEXT.md` 读了「价格政策」词条。没读：CC、PP、PS 三份 `CONTEXT.md` 全文，settlement-accounting `CONTEXT.md`，`pp-pricing-input-seams` 的 spec，ADR-0109；ADR-0147 在起草中未读。拿不准的列在文末「越权风险点」。）
Date: 2026-09-24

## Context

[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定七把路由列为第一刀：候选生成、硬约束过滤、排序策略族等判断方法归产品，网络目录内容、阈值、日历与截单等归租户。实施票族在 `.scratch/routing-first-cut/`；本记录是其中取数侧开工前要先裁的那几件。

**证据视图要一次交出的事实族，只有一部分出自网络目录。** `ports.NetworkEvidence` 与 `ports.InitialRouteEvidence` 列出的事实里，服务区域解析、路径可执行性、时间投影、候选段链、策略引用与视图修订可以从版本化网络目录（[ADR-0068](./0068-versioned-network-catalog-structure-precedes-rule-content.md)）折出；判断对象的地址、服务要求与客户承诺属 `parcel-shipment`，候选的关务适用性属 `customs-compliance`，候选的采购成本评价属 `parcel-pricing`。[CONTEXT-MAP](../domain/CONTEXT-MAP.md) 已为这几条边写了谁提供什么，没写取数侧怎样把它们接进一次取回。

**已有的先例定了大半方向。**

- [ADR-0075](./0075-customer-address-is-carried-with-the-routing-request.md)：地址随判断请求在请求期携带地理投影，NR 不建读 PS 地址的端口；「投影的具体字段形状属机制侧实现，随取数侧实现票定」。
- PS CONTEXT「地址要素」在代码里是寄件与收件资料范围上的一个封闭集——邮编与国家 / 地区码，值原样保全、不规范化；`AddressElementName` 的注释写明「分区表（PP）与服务区域（NR）要的就是这两格」。
- 面单渠道择优（[label-channel-service-first-release/01](../../.scratch/label-channel-service-first-release/issues/01-channel-candidate-tie-break-authority.md) 的裁决、[/13](../../.scratch/label-channel-service-first-release/issues/13-buy-evaluation-to-cost-score-bridge.md)）：PP 批量评价口只答一个候选算不算得出价，出局与并列落在择优一侧的比较器；取价用的计价输入由消费方适配器组装。
- 换算与汇率口径：PP 拥有版本化的汇率序列（PP CONTEXT「外部数值序列」），一次评价的「换算」保留原币金额、所引汇率序列版本与换算后金额；汇率口径（牌价类型、取值时点规则、加点规则）「由商业价格政策版本化声明；不接受未声明口径的裸汇率」。PC 的「价格政策」按方向分销售、采购与内部三种，内部那种由法人间协议拥有。
- [first-tenant-runway/03](../../.scratch/first-tenant-runway/issues/03-network-resolution-layer.md) 的 Answer：视图修订必须改由目录修订锚派生，ADR-0068 决定六随解析层同笔部分停用。它同时主张定义登记册（迁移 `0007`）保留「这个范围登记过」的职责——那时路由策略版本既没有内容，也没有「采用」一说。

**关务一侧没有可接的判断。** CC 有口岸目录与申报路径目录的登记册（`ports.PortsPathsRegistry`），没有「这个路由候选的关务区域、口岸、申报路径是否合规可用」的判断口；CONTEXT-MAP「customs-compliance ↔ network-routing」一条要求的正是后者。

## 候选与反方

**成本 · 甲：整条候选当作一个计价对象取一次评价。** 反方：一个候选的各段由不同履约方按各自的价卡承运，计价重量按各卡自己的体积系数与进位算（参数登记册 `PAR-NET-16` 已确认的那句）；把整条路当一个对象，要么只能挑一张卡去算全程，要么在 PP 里拼出一张不存在的卡。否决。

**成本 · 乙：逐段取评价、在 NR 合成。** 即决定四。

**币种 · 丙：禁止跨币种，只比同一币种。** 反方：首发走廊 CN→SG 的头程与尾程常由不同币种结算的供应商承运，一律停下等于首个排序形态在首发走廊上不可用。否决——只留作比较币种没登记时的退路（决定四）。

**币种 · 丁：取数侧自选一个汇率换算，或拿法人的币种当比较币种。** 反方：NR 与取数侧都不拥有汇率，PP 也不接受未声明口径的裸汇率；法人上没有币种——ADR-0145 决定四已把「法人的记账本位币」留作另议。否决——比较币种由路由策略版本登记，换算照 PP 既有纪律、口径由价格政策声明（决定四）。

**自营 · 戊：自营段一律缺成本依据。** 反方：租户网络有自营头程或尾程时，首个形态排不了这些候选；自营与外包放在一起比（make-or-buy）本来就要求两者同一口径。否决——自营段经内部价格政策计成本（决定四）。

**候选生成 · 己：首版就允许线路拼接（中转）。** 反方：拼接要先定可拼的衔接条件（同一节点、衔接缓冲、跨线路的责任交接），这些正是时间投影与段链两张实施票要落的形状；首版先让一条线路即一个候选，拼接作为后续形态加进候选生成族，不改已登线路的含义。否决（延后）。

**`未配置` · 庚：定义登记册（`0007`）照旧当开关。** 反方：ADR-0146 决定三之后，租户采用一版路由策略就是一次普通登记，而路由策略版本本就「在明确范围和有效期内」生效（CONTEXT Language「路由策略版本」）。再留一个 `0007` 开关，「配没配」就有两处答案，二者不一致时没有一条规则说谁赢。否决。

## Decision

**一、事实族按来源分三路。**

- **目录折叠**：服务区域解析、候选生成、路径可执行性（含临时网络可用性调整）、时间投影、候选段链、策略引用、视图修订。
- **随请求携带**（ADR-0075 同款，判据同是同版性）：地理解析投影（决定二）、服务要求、承诺上界。三者都是判断对象那一版的内容——接受前是委托当前提交版本，接受后是接受基线——由发起方随请求交来；NR 只保存判断对象引用与所携内容的版本化摘要，不回读。
- **经端口取**：候选的关务适用性（`customs-compliance`）与候选各段的成本评价（`parcel-pricing`：BUY 评价或内部价格政策评价，连同换算）。二者不进视图修订——视图修订只标目录那一路（决定六）；端口取回的事实各自带出处（关务判断标识、评价标识与版本清单），与判断一并留痕。

**二、地理解析投影就是 PS 地址要素的寄件段与收件段，每段国家 / 地区码与邮编，原样携带。** 缺席如实：某一侧缺国家 / 地区码，服务区域解析在那一侧答资料不足，不补默认国家。服务区域覆盖文法首版两种形态：整个国家 / 地区；国家 / 地区加一组邮编前缀（按所携邮编串逐字比前缀）。邮编要不要规范化（去空白、大小写）走参考配置——与 parcel-pricing 同一份公开格式（票 [product-strategy-boundary/14](../../.scratch/product-strategy-boundary/issues/14-pp-postal-prefix-granularity-and-public-unit-reference-configuration.md)），采用路径按 ADR-0147（票 [product-strategy-boundary/03](../../.scratch/product-strategy-boundary/issues/03-reference-configuration-adoption-pattern.md) 的产物，待进 main），NR 不另定一套。行政区域不进首版文法：PS 的地址要素封闭集里没有它，要用先回 PS CONTEXT 扩集。后续形态：邮编区间（起止）在各国邮编格式经参考配置可比较之后加入，与主流运输管理系统的运输区域定义同一类；行政区域等 PS 扩集后加入。

**三、关务适用性经端口向 CC 取；CC 侧的判断缺执行器，另立票，不在本票族里补。** 在它补上之前，取数侧对候选的关务一格如实答状态未知，领域照既有规则得出路由判断未决（UC-NR-001「依赖超时、版本无法解析、关务资格未知、候选计算失败或证据冲突只能形成路由判断未决」）；不答满足，也不自行推断候选是否跨关务区域——那是 CC 的判断。

**四、候选成本按比较币种合成；每段的成本依据是 BUY 评价或内部价格政策评价。**

1. **成本依据**：候选段链的每一段带它的成本依据引用——外包段引 BUY 价卡；自营段（含集团内另一法人承运的段）引内部价格政策下的价卡，按租户登记的内部标准成本计价。形状归产品，引哪张卡是租户取值。取数侧逐段向 PP 取评价，PP 只答已出价、待判断或不可计价；自营段与外包段因此走同一套评价、同一口径，放在一起比才成立。
2. **比较币种与换算**：候选成本在一个比较币种下合成。比较币种是路由策略版本上的租户取值；换算所用的汇率口径（牌价类型、取值时点规则、加点规则）由策略版本所引的一份价格政策声明——PP 不接受未声明口径的裸汇率，本记录不另开口子。换算由 PP 按该口径、取判断时点在用的汇率序列完成，逐段保留原币金额、结算币种金额、所引汇率序列版本与比较币种金额；NR 不持有汇率，也不自己换算。
3. **合成与并列**：各段的比较币种金额先按不舍入的精度相加，合计后按比较币种的最小币单位取整一次；两个候选取整后相等即并列，交首个内置形态的并列出口。逐段先取整再相加，舍入误差会随段数累积，改变并列判断。
4. **缺依据与退路**：任一段待判断、不可计价或没挂成本依据，该候选即缺成本依据（CONTEXT-MAP「parcel-pricing → network-routing」：「候选评价为待判断或不可计价时，该候选按缺少成本依据处置，不得以零金额或其他候选的金额顶替」），也不以他段的金额顶替。策略版本没登比较币种时，只在各段与各候选同一币种时合成与比较；出现不同币种即停下不比，不换算、不编汇率。缺依据与停下都是证据未齐，不是网络不可行。
5. **结果格**：缺成本依据与停下各落哪个结果格，由首个内置排序形态定（routing-first-cut/03）。本记录只划一条线：它们不得落`无当前有效路由`——那一格只给「权威输入齐备且所有候选均被确定性淘汰」（UC-NR-001 原句）。
6. **公共段剔除**（允许的优化，首版不必做）：所有候选都含的同一段（同一连接、同一承运方、同一成本依据引用）给每个候选加同一金额，不改变排序，可以不参与比较。实现时留痕要写明比较的是差异段成本，且自动改路的改善阈值只能按比较币种的绝对金额声明——按比例声明会因剔除公共段而变。
7. **计价输入**：逐段的计价输入由 NR 侧的 parcel-pricing 消费方适配器组装：包裹事实按 CONTEXT「预路由使用客户声明快照；首次收寄复核使用当时已经取得的物理事实，到站复核使用当前有效物理实测」取；每段怎样折成价卡的区域词汇随 PP 的计价输入缝定。NR 不自算计价重量，也不自定区域。

**五、首版候选生成：判断时点适用的一条完整线路即一个候选。** 线路首节点须按节点覆盖服务寄件侧的服务区域，末节点须服务收件侧；不拼接线路，也不截取线路中段。线路的连接序列即候选段链，每个连接成一段计划履约段。节点对服务区域的覆盖角色是目录内容：形状归产品，取值归租户。拼接是候选生成族的后续形态，加入时不改已登线路的含义；届时按有界搜索生成候选——换乘次数上限与可换乘节点是租户取值，不做无界枚举，这也是主流路由引擎在多段网络上的通行做法。

**六、`未配置`改由判断时点有无适用的路由策略版本答；视图修订取目录修订锚。**

- 证据视图答`未配置`，当且仅当该租户的目录修订锚不存在，或判断时点没有适用于该判断范围（租户 + 服务目的）的路由策略版本。路由策略版本的适用范围因此按服务目的解释；它在目录上怎样落形，随策略内容载体（routing-first-cut/03）与可达性取数侧（/07）定。
- 视图修订就是目录修订锚的值，与目录事实同一条语句取回（first-tenant-runway/03 Answer 的结论，本记录承接）。
- 定义登记册（迁移 `0007`）不再承载`未配置`，读口不再读它；表按迁移不可改的纪律留着，不删。
- 登记了而解不出，照旧响亮报错，不退成`未配置`，也不退成空事实（ADR-0053 决定四「禁止：查到登记就答 `configured=true` 带空事实族」）。
- 本条部分停用 ADR-0068 决定六「三个证据视图仍不读本目录」与 ADR-0053 决定四「查无登记 → `未配置`」一格，**自 routing-first-cut/07 落地的那一笔起生效**：那一笔同时改 `0008` 迁移头注与目录适配器 `NetworkCatalog` 类型注释两处护栏，并在两份被停记录的 Status 行加前向指针。在那之前，两条照旧。

## Consequences

- 接受之笔已在 network-routing `CONTEXT.md`「路由形成与选择」一节末尾，与 CONTEXT-MAP 的「parcel-shipment → network-routing」「parcel-pricing → network-routing」「customs-compliance ↔ network-routing」各加一句引用本记录，不复述决定。
- 证据视图端口要收随请求携带的三样，属导出签名改动（routing-first-cut/07、/09 开工前报窗口）；PS 随可达性请求与接受交接携带它们（/08）。
- 证据结构为端口取回的两族带出处字段，判断记录随之留痕（/07、/09、/10）。
- CC 侧另立一张票：按路由候选答关务适用性（区域、口岸、申报路径）的判断口；票 product-strategy-boundary/05 的动线取证登这一格。
- 首个内置排序形态（/03）据决定四定缺成本依据与停下的结果格；路由策略版本加「比较币种」与所引价格政策两项，随 /03 的策略载体或 /10 接成本时补，不写死成单一币种。
- PP 的换算从结算币种延伸到比较币种、逐段留痕；PC 的内部价格政策承载自营段的内部标准成本（/10，与 PP、PC owner 对口径）。

## 越权风险点

1. **比较币种换算是 PP 换算的延伸**（PP、PC owner 复核）：PP 今天的换算把一次评价换到它自己的结算币种；决定四要它再按路由策略版本所引价格政策的口径换到比较币种，并逐段留痕。口径仍由 PC 的价格政策声明、PP 按口径取值，不动「不接受裸汇率」那条。这只是内部择优的口径，不改结算——SA 照旧按合同结算币种记账。比较币种没登记时退回同币种比较，CN→SG 头尾程异币种即停下；演示租户可采用一份声明了口径的参考配置，或演示网络先用同一币种的合成价卡。
2. **自营成本经内部价格政策承载**（PC owner 与用户复核）：自营段的内部标准成本登成内部价格政策下的价卡，经 PP 同一评价机制出价。PC CONTEXT 的内部价格政策原本由法人间协议拥有；用它承载同一法人自营段的标准成本是否越出原义，归 PC owner 复核。没登内部价卡的自营段仍缺成本依据。
3. **随请求携带服务要求与承诺上界**（PS owner）：CONTEXT-MAP 已写小包托运提供服务要求；本记录把它与承诺上界定为请求期携带，接受交接的载荷要随之扩。
4. **关务适用性判断缺执行器**（CC owner）：本记录只定来源是 CC 的判断口；口的形状、答到哪一级（区域 / 口岸 / 申报路径）归 CC。
5. **逐段计价输入的区域词汇**（PP owner）：段起讫怎样折成价卡区域，依赖 PP 计价输入缝的进度；那边未定时取数侧对该段答待判断，候选随之缺成本依据。
6. **推翻 first-tenant-runway/03 Answer 的半句**（NR owner）：`0007` 不再承载`未配置`，前提变化是 ADR-0146 给了路由策略版本「采用」的语义。若 owner 认为服务目的这一维不该落在策略适用范围上，决定六的第一格要改回由 `0007` 承载。

## Links

- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：决定七是本记录的出处
- [ADR-0075](./0075-customer-address-is-carried-with-the-routing-request.md)：地址随请求携带；本记录定投影的字段形状并把同一判据扩到服务要求与承诺上界
- [ADR-0068](./0068-versioned-network-catalog-structure-precedes-rule-content.md)：决定六由本记录部分停用（自 routing-first-cut/07 落地起）
- [ADR-0053](./0053-network-fact-families-are-derived-not-registrable.md)：决定四「查无登记 → `未配置`」一格由本记录部分停用（同上）
- [ADR-0145](./0145-legal-entity-attributes-split-into-identity-layer-and-dated-profile.md)：决定四把法人的记账本位币留作另议，比较币种因此不取法人的币种，改由路由策略版本登记
- 票 [routing-first-cut/02](../../.scratch/routing-first-cut/issues/02-route-evidence-sourcing-and-candidate-cost-adr.md)：本记录的派生票；实施票族见 [product-strategy-boundary/04](../../.scratch/product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md) 的切片计划
