# ADR-0078: 隔离环境运营查阅按装配注入放行——合成租户显式入参、缺省朝拦，写路径与客户查阅面维持未配置即拒

Status: Accepted（2026-08-26，用户经 IDP 队列通道 1 委托本会话在取证简报三路选项间裁断；取证材料为[读面准入简报](../../.scratch/product-story-and-demo/read-admission-brief.md)，其四部分材料是本记录的事实底座，本文不复述取证细节）  
Date: 2026-08-26

## Context

演示动线票（product-story-and-demo/04）v1 的前提是「全链业务事实由登记 CLI 灌成套合成 S 种子，页面负责如实展示」。勘察（journey-draft）发现该前提在当前装配下不可达：`assembleBusinessEndpoints` 的每一行都以字面量 `UnconfiguredIntake{}` 写死，读路径与写路径同样停在 403——种子灌得进库，页面看不见。种子包（`scripts/demo-seeds/`，合成租户 `SYN-TENANT-01`）、真库读适配器、查询端点与管理台页面各自都已就位，缺的只有一件：隔离环境里读面的准入。

三件既有文本顶在这里：

1. [ADR-0077](./0077-master-data-catalogue-read-follows-the-operations-read-pattern.md) Consequences 声称「隔离合成 S 环境里，种子经登记 CLI 灌入后页面可见合成数据，证据层级记 S」——简报取证确认这句在代码里没有任何机制支撑，其自己的 Decision 三裁未配置即拒。这句声称要么兑现、要么改写，不能悬着。
2. [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md) Decision 五的两项未决（载荷规范化摘要、准入范围装配 `PAR-GOV-03..07`）拦着「真渠道 Intake」。简报盘点措辞：两项机制各自的文本语境都在写侧；**没有任何一句原文把它们点名到查阅路径上，也没有任何一句原文豁免查阅路径**——豁免与否是一次待作的显式裁决，本记录就是那次裁决。
3. 本仓迄今没有按环境切换进程装配的先例（简报第三部分逐项取证）；环境差异的既有处理是 pgtest 的「跳过/失败」，不是「换装配」。开这个先例本身是难逆转取舍，须 ADR 级记录并把先例钉窄。

## Decision

**一、隔离读面准入成立，覆盖面只有运营查阅端点。** 入格判据三条同时满足：消费所属上下文的存储读面且查阅不触发判断、派生或披露（`assembleBusinessEndpoints` 注释与 ADR-0076/0077 的同一分界句）；零持久化——不铸来源信封、不写任何行、不构造命令；作用域是租户级运营作用域、无客户维（ADR-0076 Decision 二、ADR-0077 Decision 二的形状）。按此判据，当前装配点上入格的是这八行：`/shipment-request-views`、`/tracking-projections`、`/pricing-price-cards`、`/pricing-reference-series`、`/network-catalog`、`/customs-compliance-rules`、`/commercial-service-products`、`/commercial-policies`。**`/customer-tracking-view` 明确排除**：其查询键带货主客户账户维，UC-VE-008 要求请求方身份、账户与对象授权整组同时核对——那正是 `PAR-INT-01` 拥有的实例半边语义，给它注入合成客户身份就是在参数登记册拥有的位置上放替身。命令面端点全部不在本记录，维持未配置即拒。

**二、放行形态是装配注入，不是认证。** 每个入格上下文在生产代码里自立一个隔离运营查阅 Intake 类型（与 `UnconfiguredIntake` 同层同款、每上下文自立不共享），其作用域全部维度与页大小由装配注入合成值给定；实现不读请求的任何部分，延续 `UnconfiguredIntake` 的匿名参数纪律——签名不给「读一眼再决定」留位置。该类型只实现所属上下文的查阅 Intake 接口、不实现任何命令 Intake：放行装不进命令端点由编译期保证，不靠纪律。它不是 ADR-0055/0077 禁的「开发用」采信实现——采信要有自报被信，这里没有任何自报被读取；ADR-0055 否决采信头部 Intake 的理由「事后没有任何东西能把这些信封与真实认证结果区分开」在零持久化的查阅面上没有落点——运营查阅不铸信封、查询作用域不落库，事后无物可混。

**三、激活是显式装配输入，缺省朝拦，合成标识由启动门禁钉死。** `cmd/parcel-api` 新增环境变量 `IDP_PARCEL_ISOLATED_READ_TENANT`：未设时装配与本记录之前逐字节同形，全部端点未配置即拒——缺省方向朝拦。设置时仅 Decision 一枚举的查阅行换注入式 Intake，命令行与 `/customer-tracking-view` 不动。值必须带合成标识前缀 `SYN-`（种子包既有纪律的同一前缀）：不带前缀的值使进程**启动即拒、报错退出**，不静默回落——静默回落会让配置错误与「刻意拦着」两态可观察签名相同，那是「默认值不出声」病。放行生效时启动日志必须写明隔离读面准入已启用与所用合成租户——放行必须出声，事后可查。这道前缀门禁使真实租户标识结构上进不了这个开关：`SYN-` 是证据层级 S 在代码里的锚。作用域引用与页大小的具体合成常量属实现票，不属本记录。

**四、先例钉窄：环境切换只此一处、只此一维。** 按环境选择的只有装配点上查阅行的 Intake 一件事；不得据此再添 demo/mode 类全局开关或第二个 main；pgtest「无 DSN 诚实跳过、CI 缺 DSN 失败」的环境处理通例不变。真渠道 Intake 就位时替换点照旧在装配点逐端点换（ADR-0055 预留、ADR-0072 维持）；隔离读面准入不因真渠道出现自动退场——它属于没有租户的隔离环境，生产部署不设本变量，真实租户标识已被前缀门禁挡死。

**五、与既有记录的界。** ADR-0077 Consequences 那句「页面可见合成数据」由本记录兑现——本记录是那句声称的机制落点，0077 正文不改写。ADR-0022「禁止在两项未决前落地任何默认实现」原文有效：注入式放行不是那句禁的默认实现（不采信自报、不铸信封，且缺省指向拦不指向放）；载荷规范化摘要与准入范围装配两项未决照旧拦着真渠道 Intake 与一切命令面（ADR-0055 Decision 五原文有效）。本记录作出的是那次悬置的显式豁免：对零持久化、租户级、无客户维的运营查阅面，两项写侧未决不构成拦截物——豁免以本记录为据，不是绕开原文。

## Consequences

- 六个上下文（parcel-shipment、visibility-exception、parcel-pricing、network-routing、customs-compliance、party-commercial）各得一个隔离运营查阅 Intake 类型与配套传输层测试（作用域来自注入、不读请求、只实现查阅接口）；`assembleBusinessEndpoints` 增一个隔离读面输入并只切查阅行；`cmd/parcel-api` main 增环境变量解析、前缀门禁与启动日志；装配测试覆盖未设/设置两态。
- 票 04 v1 的前提恢复：隔离环境里种子灌入后，管理台读页可见合成 S 数据；PRODUCT-STORY「今天能演示什么」的页面全链可看一条可兑现。页面所见一切数据来自 `SYN-` 前缀合成种子，S 只记 S。
- 演示深度上限不因此改变：写动作从页面发起仍等票 04 v2 的前件（两项未决闭合与那次渠道裁决）；本记录不缩短那条路，也不为它立任何先例。
- 隔离读面准入启用的进程对外表面：查阅端点如实答数据，命令端点照旧 403——两类端点答复不同是本记录的刻意结果，不是缺陷。

## Alternatives considered

- **在隔离环境按 S 级登记合成接入渠道（简报路径 a）。** 否决：登记册表结构与凭据形态在 `PAR-INT-01` 最低证据前不立是 ADR-0072 Decision 二维持的原判，合成渠道自拟凭据形状正落在「替租户拟 `PAR-INT-01` 的样子」的否决覆盖面里；且它把一个看着像实现的替身放在真渠道能力的位置上——事后要把合成渠道与真渠道在登记册里区分开，恰是 ADR-0055 那句否决理由在写侧的原样复发。
- **数据态不走页面、库内读回取证（简报路径 c）。** 否决，理由是产品的而不是工程的：本产品当前零租户，可演示性是产品就绪判据的主要承载；此路使管理台在真实渠道登记前永远只能演示未配置态，票 04「页面负责如实展示」、PRODUCT-STORY 页面全链可看与 ADR-0077 Consequences 声称三处产品口径同时缩水，等于把一句已接受的声称改成长期不成立。工程上零风险的选项在产品上最贵。
- **前端 mock / 等 `PAR-INT-01` 一起做。** 均已在 ADR-0077 Alternatives 明文否决，理由照旧成立，此处只引不复述。
- **独立 main（如 cmd/parcel-api-demo）或构建标记代替环境变量。** 否决：独立 main 复制整个装配点，两份装配的漂移是新的假绿面；构建标记在运行时不可观察，与「放行必须出声」相逆。环境变量与 `cmd/` 既有三个环境变量同一形态，且能承载合成租户值本身。

## Links

- [读面准入简报](../../.scratch/product-story-and-demo/read-admission-brief.md)：四部分取证材料与三路选项的来源（ADR 约束盘点、现状取证、先例普查、路径代价）
- [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)：未配置格机制与替换缝；其 Decision 五两项未决的适用界由本记录 Decision 五显式裁出
- [ADR-0072](./0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)：登记册形状等真渠道证据的原判，本记录不触碰并在 Alternatives 里维持
- [ADR-0076](./0076-operations-tracking-read-is-a-separate-endpoint-on-the-projection-store.md)、[ADR-0077](./0077-master-data-catalogue-read-follows-the-operations-read-pattern.md)：运营查阅作用域形状与目录读通例；0077 Consequences 的隔离环境声称由本记录兑现
- [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)：「禁止默认实现」原句的出处，划界见 Decision 五
- [ADR-0003](./0003-group-tenant-legal-entity-customer-account.md)：租户隔离边界——注入合成租户不读自报，不触碰该边界
- [票 04 演示动线](../../.scratch/product-story-and-demo/issues/04-demo-journey.md)：v1/v2 分界与本记录的消费方
- [参数登记册](../product/PILOT-PARAMETER-REGISTER.md)：`PAR-INT-01` 与 `PAR-GOV-03..07` 的登记处
