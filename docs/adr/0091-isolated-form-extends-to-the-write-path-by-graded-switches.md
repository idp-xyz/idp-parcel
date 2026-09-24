# ADR-0091：隔离形态从查阅面扩到写路径，按分级开关放行——入格判据取代枚举；写面有持久化，可分辨物由 `SYN-` 前缀承担

Status: Accepted（2026-09-02，用户经 IDP 队列通道 5 在四路范围选项间裁定「一份 ADR 同时裁两格」；本记录据此把写面 Intake 与生产归属范围目录合并裁决）；**部分停用**：决定六「隔离形态不因真渠道出现而自动退场」一句已由 [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 停用——隔离形态是真渠道缺位时的过渡，某一口真渠道 Intake 落地即在同一笔撤下该口的隔离放行；同句「也不因它出现而获得任何真实租户」与其余各条不变
Date: 2026-09-02

## Context

[ADR-0078](./0078-isolated-environment-operations-reads-admit-by-assembly-injection.md) 给隔离环境的运营查阅面开了一道装配注入的准入，并在 Decision 四把这个先例钉窄：「按环境选择的只有装配点上查阅行的 Intake 一件事」。那句话当时是对的——它防的是「开一次口子就顺手长出 demo/mode 类全局开关」，而当时确实只有查阅面要开。

首租户跑道盘出的三堵墙里，前两堵各自顶在一条缝上，而两条缝都不是「查阅行的 Intake」：

- **墙一**：命令面端点在 `assembleBusinessEndpoints` 里挂的是字面量 `UnconfiguredIntake{}`，答 `403` + `ACCESS_CHANNEL_NOT_CONFIGURED`。
- **墙二**：`buildSubmissionOrchestration` 把 `ProductionOwnershipAdapterDeps` 的 `Directory` 与 `SelfAuthority` 留空，`ProductionOwnershipAdapter.governanceScope` 因此先于读治理登记册就返回未配置，归属如实答`权威未确定`，提交停在 `OWNERSHIP_UNRESOLVED`。

两堵墙问的是同一个问题——**隔离环境里，装配注入合成值算不算 [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md) 与 [ADR-0072](./0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md) 所禁的默认实现**——只是问在了相邻的两条缝上。两条缝各自都要碰 ADR-0078 Decision 四；分两份记录裁就得把同一条句子修订两次，而两次修订之间那份记录说的是什么，取决于读的人碰巧读到哪一份。

`.scratch/first-tenant-runway/issues/02-isolated-governance-scope-directory.md` 把「落点跟随票 01 的形态判定」写进了票面，但票 01 的形态判定按其原范围只覆盖写端点的 Intake，不覆盖归属目录。这个缺口由本记录补上。

## Decision

**一、隔离形态的适用面由枚举改为判据，ADR-0078 Decision 四那句「只此一维」随之停用。** 入格要同时满足三条：注入值全部是带 `SYN-` 前缀的合成值；实现不采信请求里的任何自报身份；生产装配里不存在通往它的代码路径，缺省朝拦。判据取代枚举的理由是枚举当时只够用一次——ADR-0078 立的时候放行面恰好就是那八行，把「当时的清单」写成「先例的边界」使每一条新缝都要重开一次同样的论证。Decision 四的另两句（不得据此再添 demo/mode 类全局开关或第二个 main；pgtest 的环境处理通例不变）**原文有效，本记录不碰**。

**二、写路径的两格入格：命令面 Intake 与生产归属范围目录。** 前者是墙一，后者是墙二。两者都只放行「有没有配置」这道门，不放行它后面的任何一道：过了 Intake 之后提交编排仍要走完真实的来源保全、判重、生产归属与门禁评估；接上合成目录之后归属仍要真的读治理登记册，登记册为空时照旧答`权威未确定`。**不许写一个直接返回「已确定」的假目录**——那会把墙二从「诚实的未配置」换成「撒谎的已配置」，而后者在库里长着与真实放行一模一样的脸。

**三、写面有持久化，ADR-0078 的「零持久化」论证不可照搬，可分辨物改由 `SYN-` 前缀承担。** ADR-0078 Decision 二为放行辩护时用的是「运营查阅不铸信封、查询作用域不落库，事后无物可混」；写路径会落行、会铸来源信封、会入队信封意图，这半个论证在此**不成立**，本记录不假装它成立。取代它的是：隔离形态写下的每一行，其租户维与来源维本身就带 `SYN-` 前缀，因此**事后可分辨靠的是事实自己的来源表达，不靠旁表也不靠时间窗口**——形状同 [ADR-0089](./0089-frontline-transition-controlled-import-with-structural-sunset.md) 细则四的 `FTI/` 标记，理由一字不差：时间窗口不是事实的属性，一次重放就失效。这是本记录相对 ADR-0078 真正新增的风险，以及它的抵消物；两者都记在这一条，不拆开写。

**四、读与写分成两个开关，不复用同一个。** 新增 `IDP_PARCEL_ISOLATED_WRITE_TENANT`，形状照抄 `IDP_PARCEL_ISOLATED_READ_TENANT`：未设即全拦、设了必须带 `SYN-` 前缀否则**进程启动即拒**并带原因退出、启用时启动日志写明。**不复用读开关的理由是缺省朝拦本身**：今天所有设了读开关的隔离环境，若两者合一，会在升级的那一刻静默获得写准入——一个已经存在的配置的含义被后来的代码改宽，正是「缺省朝拦」要防的那件事。两者都设时取值必须相同，否则启动即拒：写下的委托挂在一个读面不过滤的租户上，页面看不见它，而那个症状看起来像缺陷不像配置错。

[ADR-0083](./0083-pilot-governance-read-face-carries-registry-dimensions-only.md) Decision 三写过「不添第二个环境变量、不添第二个开关」，那句**不因本记录失效，也不被本记录停用**：它是 ADR-0083 对自己那一格（治理查阅行）的克制声明——治理读面沿用读开关，本记录一个字都没改它。会读错的地方在于那句话长得像通则，因此在此点明：它约束的是 ADR-0083 自己的改动，新开关服务的是另一条路径。

**五、治理坐标不从租户派生。** 治理登记册无租户维是设计（[ADR-0083](./0083-pilot-governance-read-face-carries-registry-dimensions-only.md)），所以写开关的值在归属目录这一格**只作启用凭据**，合成的对象范围、能力、事实类型与试点范围版本是各自独立的合成常量。这条与 `buildIsolatedReadIntakes` 对治理读面的处置同款：同一个开关决定启用与前缀门禁，但开关值不进治理作用域。

**六、本记录不解锁任何实例半边。** `PAR-INT-01` 的登记状态、`PAR-GOV-03..07` 的「待提供」、[ADR-0072](./0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md) Decision 二「登记册表结构与凭据形态等真渠道证据」全部原样有效。隔离形态不是通往生产的捷径，它是一条生产装配里不存在的路径；真渠道 Intake 就位时替换点照旧在装配点逐端点换，隔离形态不因真渠道出现而自动退场，也不因它出现而获得任何真实租户——`SYN-` 前缀门禁把真实租户标识结构上挡在开关之外。

## Consequences

- `cmd/parcel-api` 增一个环境变量、一道前缀门禁、一条两开关一致性校验与一条启动日志；装配点的提交编排构造多一个「隔离形态注入」入参，未启用时与本记录之前逐字节同形。
- 墙二随本记录的实现落地：隔离形态下提交编排能越过 `OWNERSHIP_UNRESOLVED`，前提是治理登记册里真有一行匹配的合成权威区间；种子包因此多一笔委托受理维的权威区间登记。
- **命令面按端点逐口放行，不是一次全开。** 首批只有 `/shipment-requests`；撤回、取消、复核完成、主动拒绝与其余上下文的命令面仍挂不经任何变量的字面量 `UnconfiguredIntake{}`。逐口换的纪律沿用 ADR-0055 预留的那条缝，理由也一样：一次只换一口，每换一口该口的「未配置即拒」测试改写为放行测试，未换的口答复不变。
- **提交口的 Intake 要多做三件读面不必做的事**，因为提交命令不是一个作用域就能凑齐的：铸来源信封、把草案译成规范化摘要、向归属权威预取期望规则修订。第三件依据 `UC-PS-001` 步骤 3B「准入范围与期望规则修订由试点准入控制装配」；其代价是门禁在隔离形态下只拦得住预取与提交之间登记册发生的变化，拦不住「调用方揣着上周的修订」——隔离形态里没有那样的调用方。
- **生产接线棘轮的基线少一条。** `CanonicalizeSubmissionPayload` 自此有了非测试调用点（隔离提交 Intake），从 `production_wiring_baseline.txt` 出名单；同路径的 `CurrentPayloadCanonicalizationVersion` 仍无调用点，留在名单上。原基线注释预言两者「接上那天一起出名单」，本记录让那个预言不成立，注释已同笔改写。
- 演示深度上限仍不等于生产：墙三（网络解析层）照旧拦着接受判断之后的初始路由与可达性，本记录不缩短那条路。
- 隔离形态启用的进程对外表面从「读能看、写全拒」变为分级可配，三态（都不设 / 只设读 / 读写都设）各自可观察，装配测试逐态覆盖。

## Alternatives considered

- **复用 `IDP_PARCEL_ISOLATED_READ_TENANT` 一个开关。** 否决，理由在 Decision 四：已存在的配置的含义会被后来的代码改宽，设了读的环境在升级那一刻静默获得写准入。省一个变量换来的是一次不出声的放宽，而「默认值不出声」正是 ADR-0078 自己点名的病。
- **两份 ADR 各裁一格，按票 01 / 票 02 的原范围分开。** 否决（用户裁定）：ADR-0078 Decision 四会被修订两次，而两次之间那句话的效力取决于读的人碰巧读到哪一份；两格问的本来就是同一个问题。
- **给生产归属范围目录建运行时登记表，隔离与生产同走登记。** 否决：那张表的形状要说「哪份拟受理范围对应哪个治理坐标」，而那正是 `PAR-GOV-03..07` 拥有的实例半边语义；替它拟表与 ADR-0072 Decision 二否决的「替租户拟 `PAR-INT-01` 的样子」是同一件事。
- **不动 ADR-0078，在票 02 里悄悄复用读开关。** 否决：Decision 四是明文，绕开它而不修订它，会让下一个读 ADR-0078 的人以为「一处一维」仍然成立。难逆转取舍走 ADR，不在实现票里定。
- **合成目录直接返回「本产品即权威、准入开放」，不读登记册。** 否决：它把墙二换成一句谎话，且与真实放行在库里不可分辨；票 02「必须守住的一格」原句即此。

## Links

- [ADR-0078：隔离环境运营查阅按装配注入放行](./0078-isolated-environment-operations-reads-admit-by-assembly-injection.md)：本记录的形状来源；其 Decision 四「按环境选择的只有装配点上查阅行的 Intake 一件事」由本记录停用，同条另两句不变
- [ADR-0055：业务端点 Intake 增设「未配置」格](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)：未配置格与替换缝；其 Decision 五两项未决对生产装配照旧有效
- [ADR-0072：接入渠道能力属共享技术能力，登记册形状等渠道证据](./0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)：本记录不触碰其 Decision 二，并在 Alternatives 里维持
- [ADR-0089：一线作业过渡走受控批量导入并内置结构性拆除期限](./0089-frontline-transition-controlled-import-with-structural-sunset.md)：「来源标记落在事实自己的来源表达上、不另立旁表」的形状与理由出处，Decision 三沿用
- [ADR-0083：试点治理读面只带登记册维度](./0083-pilot-governance-read-face-carries-registry-dimensions-only.md)：治理登记册无租户维的出处，Decision 五据此；其 Decision 三「不添第二个环境变量」是对自己那一格的克制声明，本记录不停用它，治理读面仍沿用读开关
- [ADR-0003：采用集团租户、法人责任与货主客户账户三级边界](./0003-group-tenant-legal-entity-customer-account.md)：注入合成租户不读自报，不触碰该边界
- [ADR-0017：实现准入闸门按阻断理由分别裁决](./0017-admission-gates-judged-by-blocking-cause.md)：机制半边放行与实例半边阻断分别判读的依据
- [参数登记册](../product/PILOT-PARAMETER-REGISTER.md)：`PAR-INT-01` 与 `PAR-GOV-03..07` 的登记处，本记录不改变其状态
- [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)：部分停用本记录决定六「隔离形态不因真渠道出现而自动退场」一句——合成租户经真渠道进出，隔离形态逐口随真渠道退场
- 来源：`.scratch/first-tenant-runway/issues/01-isolated-write-admission.md`（墙一）与 `.scratch/first-tenant-runway/issues/02-isolated-governance-scope-directory.md`（墙二）
