# 01 附表：ADR Status 行合并前后对照

票 [01](./01-adr-suspension-chains.md) 的完成记录附表。逐份列出本票改过 `Status` 行或头部停用行的 ADR，前一版取自 main `311adf934`（本票 rebase 的基点），后一版是本票分支上的现状；由只读脚本按 git 生成，处理类别由脚本按特征判出、作者逐份核过。Links 节与正文不在此列（正文一字未动，核法见完成记录）。

共 127 份。按处理类别计（一份可落几类）：头部停用行并入注记 6 份；补指针 37 份；来向两写合并 52 份；措辞对齐（接受出处、去向的模糊重说，或引文改为逐字） 21 份；接受出处两写合并 59 份。

### ADR-0012: 在 idp-parcel 内建立小包计价上下文

处理：头部停用行并入注记

合并前（main `311adf934`）：

```text
Status: Accepted  
Amended by: [ADR-0013](./0013-pricing-owns-versioned-external-reference-series.md)（限定下文 Decision 中「它只消费其他上下文提供的事实快照」一句；本记录正文不改写，边界划分不变）
```

合并后：

```text
Status: Accepted<!-- 索引注记 -->｜**部分停用**：其 Decision 中「它只消费其他上下文提供的事实快照」一句的适用范围已由 [ADR-0013](./0013-pricing-owns-versioned-external-reference-series.md) 限定——燃油费率与汇率没有产出它们的兄弟上下文，由计价拥有版本化的外部数值序列并在评价内完成币种换算。**适用场景**：仅限没有兄弟上下文产出的外部数值序列；边界划分不变，其余各条不变，本记录正文不改写。
```

### ADR-0014: 为内容摘要的规范化形状引入版本号

处理：头部停用行并入注记

合并前（main `311adf934`）：

```text
Status: Accepted<!-- 索引注记 -->｜**部分停用**：其 Consequences「每次变化都要新增一个规范化版本并长期支持它」一句已由 [ADR-0177](./0177-customer-contract-names-its-customer-account-and-the-closure-narrows-by-it.md) 决定三停用。**适用场景**：仅限 PC 客户合同一册加 `customerAccount` 一键那一次，`PCC-1` 不换号——理由是该册没有需要回放的已保存摘要，与本记录「代价在此刻支付最低」同一前提；同一句又由 [ADR-0201](./0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md) 文末补记「迁移 0027」一条停用，**适用场景**：仅限 TF 容量池一册加维度一键那一次，`TFC-1` 不换号——迁移 0027 在池表有行时拒绝施加，库里没有需要回放的池摘要，与上一次同一前提；同一句又由 [ADR-0202](./0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md) 文末补记「摘要形状」一条停用，**适用场景**：仅限 TF 运输委托一册改收协议对象与版本标签、订舱一册加班次一键那两次，`TFC-1` 不换号——迁移 0025、0029 在表里有行时拒绝施加，两册在接线前没有写入方，与上两次同一前提；此后任何一册再改规范化形状照本记录换号，其余各条不变；另由 [ADR-0213](./0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md) Consequences「部分停用 ADR-0014」一条停用决定里「规范化版本不同不是冲突，也不得据此判定重放失败」一句。**适用场景**：仅限 TF 交接首登一口，在册存的是 `TFC-1` 或无版本形、本次命令声明了节点时答冲突——停用理由的前提是旧形之下节点没有写法、按旧形重算会把声明丢掉；本次命令没声明节点时旧形行照旧按旧形比。其余各条不变  
Partially suspended by: [ADR-0177](./0177-customer-contract-names-its-customer-account-and-the-closure-narrows-by-it.md) 决定三（Consequences「每次变化都要新增一个规范化版本并长期支持它」一句，仅限 PC 客户合同一册加 `customerAccount` 一键那一次，`PCC-1` 不换号；其余各条不变，本记录正文不改写）；[ADR-0201](./0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md) 文末补记「迁移 0027」一条（同一句，仅限 TF 容量池一册加维度一键那一次，`TFC-1` 不换号；其余各条不变，本记录正文不改写）；[ADR-0202](./0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md) 文末补记「摘要形状」一条（同一句，仅限 TF 运输委托一册改收协议对象与版本标签、订舱一册加班次一键那两次，`TFC-1` 不换号；其余各条不变，本记录正文不改写）；[ADR-0213](./0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md) Consequences「部分停用 ADR-0014」一条（决定里「规范化版本不同不是冲突，也不得据此判定重放失败」一句，仅限 TF 交接首登一口：在册存的是 `TFC-1` 或无版本形、本次命令声明了节点时答冲突——旧形之下节点没有写法，按旧形重算会把声明丢掉；其余各条不变，本记录正文不改写）  
Amended by: [开发主线的产品就绪里程碑与能力范围判据](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)（限定下文 Alternatives considered 中否决「按行业形态先把三族与分段进位的结构定死」时所用的「在结构层写死未确认内容」这一条理由：该理由对自营系统成立，对面向市场的产品不成立。该替代仍不能取代本记录的 Decision——版本号与建哪些族是两个独立问题。Decision 与 Consequences 不变，本记录正文不改写）
```

合并后：

```text
Status: Accepted<!-- 索引注记 -->｜**部分停用**：其 Consequences「每次变化都要新增一个规范化版本并长期支持它」一句已由 [ADR-0177](./0177-customer-contract-names-its-customer-account-and-the-closure-narrows-by-it.md) 决定三停用。**适用场景**：仅限 PC 客户合同一册加 `customerAccount` 一键那一次，`PCC-1` 不换号——理由是该册没有需要回放的已保存摘要，与本记录「代价在此刻支付最低」同一前提；同一句又由 [ADR-0201](./0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md) 文末补记「迁移 0027」一条停用，**适用场景**：仅限 TF 容量池一册加维度一键那一次，`TFC-1` 不换号——迁移 0027 在池表有行时拒绝施加，库里没有需要回放的池摘要，与上一次同一前提；同一句又由 [ADR-0202](./0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md) 文末补记「摘要形状」一条停用，**适用场景**：仅限 TF 运输委托一册改收协议对象与版本标签、订舱一册加班次一键那两次，`TFC-1` 不换号——迁移 0025、0029 在表里有行时拒绝施加，两册在接线前没有写入方，与上两次同一前提；此后任何一册再改规范化形状照本记录换号，其余各条不变；另由 [ADR-0213](./0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md) Consequences「部分停用 ADR-0014」一条停用决定里「规范化版本不同不是冲突，也不得据此判定重放失败」一句。**适用场景**：仅限 TF 交接首登一口，在册存的是 `TFC-1` 或无版本形、本次命令声明了节点时答冲突——停用理由的前提是旧形之下节点没有写法、按旧形重算会把声明丢掉；本次命令没声明节点时旧形行照旧按旧形比。其余各条不变，本记录正文不改写。**理由限定**：Alternatives considered 中否决「按行业形态先把三族与分段进位的结构定死」时所用的「在结构层写死未确认内容」这一条理由，已由[开发主线的产品就绪里程碑与能力范围判据](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)限定：该理由对自营系统成立，对面向市场的产品不成立；该替代仍不能取代本记录的 Decision——版本号与建哪些族是两个独立问题。Decision 与 Consequences 不变，本记录正文不改写
```

### ADR-0015: 以封闭文法与开放词汇实现多承运商通用计价

处理：头部停用行并入注记

合并前（main `311adf934`）：

```text
Status: Accepted  
Amended by: [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md) 的 Decision（限定下文 Decision 中「词汇按证据增长」一段末尾「并按 ADR-0014 形成新的规范化版本」这半句：是否形成新版本由**规范化文档的形状是否改变**决定，不由词汇是否扩充决定。本记录正文不改写，其余内容不变，见下文「与 ADR-0014 的分工」）
```

合并后：

```text
Status: Accepted<!-- 索引注记 -->｜**读法限定**（不是停用）：Decision「词汇按证据增长」一段末尾「并按 [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md) 形成新的规范化版本」这半句按本记录「与 ADR-0014 的分工」一节读——是否形成新版本由**规范化文档的形状是否改变**决定，不由词汇是否扩充决定，判据在 ADR-0014 的 Decision；本记录正文不改写，其余内容不变。
```

### ADR-0016: 产品交付与租户试点作为两条并行验收轨道

处理：补指针

合并前（main `311adf934`）：

```text
Status: Accepted
```

合并后：

```text
Status: Accepted<!-- 索引注记 -->｜**部分停用**（指针 2026-10-21 补）：其 Decision「机制是规则、判断、版本化、门禁与记录形态，属产品交付；实例是某租户的真实客户、线路、伙伴、参数与阶段决定，属租户上线」一句的两分，与产品交付轨道「判据三项：骨架完整、受控案例可复算、参数显式未配置」里第三条，已由 [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 部分取代——机制与租户取值之间另立第三类产品策略（判断方法：文法、内置策略形态、连接器形态，以及随产品发布的参考配置），机制与产品策略同属产品交付轨道、开发方现在就做；第三条判据改为「租户取值显式未配置，产品策略有执行器」，另加第四条「演示动线走通」（ADR-0146 决定一、五，运行定义以开发主线为准）。两条轨道、「两条轨道不互相替代」、证据红线与其余各条不变，本记录正文不改写
```

### ADR-0017: 实现准入闸门按阻断理由分别裁决

处理：补指针

合并前（main `311adf934`）：

```text
Status: Accepted
```

合并后：

```text
Status: Accepted<!-- 索引注记 -->｜**部分停用**（指针 2026-10-21 补）：其「**Bento 持久化技术闸门原样保留。**」一段里「PostgreSQL Repository 实现、迁移、事务与 Outbox 仍然阻断」一句已由 [ADR-0026](./0026-persistence-written-for-consumer-proof-precedes-gate-passage.md) 收窄——为产出消费者证明而写的持久化实现在闸门通过前即可写（针对精确 SemVer 候选编写 PostgreSQL Repository、迁移、事务与 Outbox 实现及其合同测试，用途限于产出 `PBC-01` 至 `PBC-09` 的证明），把实现登记为发布基线仍在闸门后。**适用场景**：只限为产出消费者证明而写的实现；同段「仍不得建立本地框架替身」与其余各条不变。闸门此后的状态以开发主线与[首个消费者切片简报](../design/parcel-go-first-consumer-slice-decision-brief.md)为准
```

### ADR-0022: HTTP 状态码只回答「有没有形成答案」，业务判别一律进响应体

处理：来向两写合并；补指针

合并前（main `311adf934`）：

```text
Status: Accepted（**部分停用**：三类表 4xx 行「调用方的错，重发不会改变结果｜出队，交给人，不自动重发」对 `429` 的适用已由 [ADR-0140](./0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md) Decision 三停用，**适用场景**仅限首方与集成两族的限流答复（`429` + `Retry-After`，按 `Retry-After` 重发即正当）；其余 4xx 照旧。**部分停用**：Decision 中「两项未决期间，业务端点可以存在并由测试替身驱动，但不进 `cmd/parcel-api` 的装配」的末段「但不进 `cmd/parcel-api` 的装配」已由 [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md) 停用——端点改为带「未配置即拒」Intake 进装配；该句要防的「默认实现铸造信封」由未配置格保留并加强，同段「禁止在它们决定前落地任何默认实现」原文有效，其余各条不变）<!-- 索引注记 -->｜**部分停用**（2026-10-19）：三类表 4xx 行「调用方的错，重发不会改变结果｜出队，交给人，不自动重发」对 `429` 的适用已由 [ADR-0140](./0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md) Decision 三 停用，**适用场景**仅限首方与集成两族的限流答复；其余各条不变。**部分停用**：其 Decision 中「两项未决期间，业务端点可以存在并由测试替身驱动，但不进 `cmd/parcel-api` 的装配」的末段「但不进 `cmd/parcel-api` 的装配」已由 [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md) 停用——业务端点改为带「未配置即拒」Intake 进装配，未配置自成一格如实作答。该句前段（端点可存在并由测试替身驱动）与同段「禁止在两项未决前落地任何默认实现」不变，其余各条不变。
```

合并后：

```text
Status: Accepted<!-- 索引注记 -->｜**部分停用**（2026-10-19）：三类表 4xx 行「调用方的错，重发不会改变结果｜出队，交给人，不自动重发」对 `429` 的适用已由 [ADR-0140](./0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md) Decision 三停用，**适用场景**仅限首方与集成两族的限流答复（`429` + `Retry-After`，按 `Retry-After` 重发即正当）；其余 4xx 照旧，其余各条不变。**部分停用**：其 Decision 中「两项未决期间，业务端点可以存在并由测试替身驱动，但不进 `cmd/parcel-api` 的装配」的末段「但不进 `cmd/parcel-api` 的装配」已由 [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md) 停用——业务端点改为带「未配置即拒」Intake 进装配，未配置自成一格如实作答；该句要防的「默认实现铸造信封」由未配置格保留并加强。该句前段（端点可存在并由测试替身驱动）与同段「并禁止在它们决定前落地任何默认实现」不变，其余各条不变。｜**两项未决已答**（指针 2026-10-21 补，话题通读所得）：Decision 末段「**接入认证与规范化摘要是两项显式未决，本记录不决定，并禁止在它们决定前落地任何默认实现。**」所说两项已逐族逐口作答——接入认证由 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)（操作者族）、[ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)（作业事实与集成客户端两族）、[ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md)（首方客户族）各定，规范化摘要由 [ADR-0072](./0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md) Decision 三判为机制、[ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定四按口定形状版本；没有作答的族与口照旧不得落地默认实现，「并禁止在它们决定前落地任何默认实现」与其余各条不变
```

### ADR-0025: 跨上下文调用的适配器落在消费侧，翻译职责由它独占

处理：来向两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（Decision 中「适配器为此需要的实例半边协作者，其接口定义在适配器包内，**不进消费方 `ports`**」一句已由 [ADR-0027](./0027-multi-step-cross-context-protocol-state-held-by-the-provider.md) **在跨上下文多步协议场景**停用，其余各条不变）<!-- 索引注记 -->｜**部分停用**：其 Decision 中「适配器为此需要的实例半边协作者，其接口定义在适配器包内，不进消费方 `ports`」一句已在跨上下文多步协议场景由 [ADR-0027](./0027-multi-step-cross-context-protocol-state-held-by-the-provider.md) 停用——该协作者已由提供方以第二阶段用例提供，适配器直接调用，不另定接口。其余各条不变。
```

合并后：

```text
Status: Accepted<!-- 索引注记 -->｜**部分停用**：其 Decision 中「适配器为此需要的实例半边协作者，其接口定义在适配器包内，**不进消费方 `ports`**」一句已由 [ADR-0027](./0027-multi-step-cross-context-protocol-state-held-by-the-provider.md) 停用——该协作者已由提供方以第二阶段用例提供，适配器直接调用，不另定接口。**适用场景**：仅限跨上下文多步协议；其余各条不变。
```

### ADR-0026: 为产出消费者证明而写的持久化实现先于闸门通过，发布基线登记仍在闸门后

处理：补指针

合并前（main `311adf934`）：

```text
Status: Accepted
```

合并后：

```text
Status: Accepted（另：「即刻解锁的范围」收窄 [ADR-0017](./0017-admission-gates-judged-by-blocking-cause.md)「PostgreSQL Repository 实现、迁移、事务与 Outbox 仍然阻断」一句，被停原句与适用场景见 ADR-0017 的 Status 行（指针 2026-10-21 补））
```

### ADR-0027: 跨上下文多步协议的中间状态由提供方按解析标识保留，消费方端口按协议阶段分方法

处理：来向两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（Decision 中「不一致按`输入未受理`处理」一句所指定的取值已由 [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md) 改指为`依据未解析`，仅限「按标识取回原状态失败」这一步；该句要防的「解析标识不得成为一张能力凭证」保留且被加强，本记录其余各条不变）（Context 原按行号引用的那些坐标已在本记录被实现时移位，现已逐条改写为符号名与引文——**只改坐标，未动任何决定文字**。Context 描述的仍是决定当时的状态，其中若干形状正是被本记录改掉的，读时按此理解）<!-- 索引注记 -->｜**部分停用**：其 Decision 中「不一致按`输入未受理`处理」一句所指定的取值已由 [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md) 改指为`依据未解析`——该句要防的「解析标识不得成为一张能力凭证」不但保留且被加强，原先只挡读取，现在连回答也不泄。**适用场景**：仅限「按标识取回原状态失败」这一步；该句其余部分与本记录各条不变。
```

合并后：

```text
Status: Accepted（Context 原按行号引用的那些坐标已在本记录被实现时移位，现已逐条改写为符号名与引文——**只改坐标，未动任何决定文字**。Context 描述的仍是决定当时的状态，其中若干形状正是被本记录改掉的，读时按此理解）<!-- 索引注记 -->｜**部分停用**：其 Decision 中「不一致按`输入未受理`处理」一句所指定的取值已由 [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md) 改指为`依据未解析`——该句要防的「解析标识不得成为一张能力凭证」不但保留且被加强，原先只挡读取，现在连回答也不泄。**适用场景**：仅限「按标识取回原状态失败」这一步；该句其余部分与本记录各条不变。
```

### ADR-0044: 结算政策依据经政策采用，结果带回预付/账期方式与适用范围

处理：补指针

合并前（main `311adf934`）：

```text
Status: Accepted
```

合并后：

```text
Status: Accepted<!-- 索引注记 -->｜**部分停用**（指针 2026-10-21 补）：其决定二「部分给出即`输入未受理`」一句在闭包解析键上已由 [ADR-0080](./0080-commercial-closure-resolves-the-contract-first-and-keys-settlement-by-it.md) 决定一停用——闭包键上的结算选择器只带相对方、费用范围、币种三维，合同维必须缺席、带上即整份键不受理，合同维由闭包先解出（ADR-0080 决定二）。**适用场景**：只限闭包解析键 `ClosureResolutionKey`；单依据键 `ResolutionKey` 仍要四维、判据不变，其余各条不变
```

### ADR-0047: 账期方式的接受前控制形成信用暴露，不冒充资金冻结

处理：补指针

合并前（main `311adf934`）：

```text
Status: Accepted
```

合并后：

```text
Status: Accepted<!-- 索引注记 -->｜**部分停用**（指针 2026-10-21 补）：其决定二「账期分支形成信用暴露，走自己的账本」与 Consequences「SA 施加编排按方式分支」两处把结算方式当作选路判据，以及 Consequences 里同一委托两种控制「作用在**不同控制范围**上」一句，已由 [ADR-0122](./0122-pre-acceptance-control-executes-the-policy-content-items-and-parcel-shipment-folds-by-the-joint-pass-condition.md) 决定一、二停用——SA 的`要求`答复长成控制项集合，编排按判断顺序逐项执行，控制种类决定走哪本账，结算方式只作保存；同一范围里可以组合多项控制。决定一要求带方式与采用政策、决定二「两本账互不借用」、决定三的 `CREDIT_EXPOSED` 词汇（ADR-0125 降为投影而保留）与决定四两本账都要认领不变。**部分停用**（指针 2026-10-21 补）：其决定二「预付读运营余额，账期读 `CreditStanding`」那半句括注里当前额度的来源已由 [ADR-0127](./0127-credit-basis-is-handed-over-by-the-commercial-closure-resolution.md) 决定四停用——授信额度取闭包交出的信用依据（`CreditStanding.WithAuthorizedLimit`），已占用暴露与逾期仍取本上下文的账本与状况登记；其余各条不变
```

### ADR-0048: 声明测量是成员级保真画像，不换算不推导

处理：头部停用行并入注记

合并前（main `311adf934`）：

```text
Status: Accepted<!-- 索引注记 -->｜**部分停用**：决定三第三增量的落点（金额缝装配器放在 PS 消费侧）已由 [ADR-0179](./0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md) 决定一停用，受理前控制金额改由结算形成估价；决定一、二不变  
Partially suspended by: [ADR-0179](./0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md) 决定一（决定三第三增量的落点：金额缝装配器不再放在 PS 消费侧，受理前控制金额改由结算形成估价；决定一、二与决定三前两个增量不变，本记录正文不改写）
```

合并后：

```text
Status: Accepted<!-- 索引注记 -->｜**部分停用**：决定三第三增量「金额缝装配器经 `DeclaredMeasurementSource` 读画像译 PP 评价输入」的落点已由 [ADR-0179](./0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md) 决定一停用——金额缝装配器不再放在 PS 消费侧，受理前控制金额改由结算形成估价；决定一、二与决定三前两个增量不变，其余各条不变，本记录正文不改写
```

### ADR-0052: 网络证据端口增设「未配置」格，网络定义登记册的模式属机制半边

处理：来向两写合并；补指针

合并前（main `311adf934`）：

```text
Status: Accepted（**部分停用**：Decision 四的第一句「它要能承载……全部事实族」已由 [ADR-0053](./0053-network-fact-families-are-derived-not-registrable.md) 停用——九族是每次判断的推导结果，不可登记；其余各条不变）<!-- 索引注记 -->｜**部分停用**：其 Decision 四中「它要能承载 `NetworkEvidence` 与 `InitialRouteEvidence` 两个结构今天已声明的全部事实族」一句已由 [ADR-0053](./0053-network-fact-families-are-derived-not-registrable.md) 停用——九族以每次判断生成的 `CandidateID` 为轴、且判断键里没有目的地维，它们是推导结果不是可登记的目录行。其余各条（三格端口、专格未决原因、修订由登记册派生、空册与空集合可分辨）不变。
```

合并后：

```text
Status: Accepted<!-- 索引注记 -->｜**部分停用**：其 Decision 四中「它要能承载 `NetworkEvidence` 与 `InitialRouteEvidence` 两个结构今天已声明的全部事实族」一句已由 [ADR-0053](./0053-network-fact-families-are-derived-not-registrable.md) 停用——九族以每次判断生成的 `CandidateID` 为轴、且判断键里没有目的地维，它们是每次判断的推导结果，不是可登记的目录行。**部分停用**（指针 2026-10-21 补）：其 Decision 一中「`configured=false` 即**网络定义登记册对这个判断范围未配置**」的判法已由 [ADR-0148](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定六停用（自 routing-first-cut/07 落地起）——`未配置`改由目录修订锚与判断时点适用于该判断范围的路由策略版本答，定义登记册不再被读；同理 Decision 四「修订标识由登记册派生，不由调用方传入」里修订改取目录修订锚，「不由调用方传入」不变。**部分停用**（指针 2026-10-21 补）：其 Decision 五「逐候选事实的生成、过滤与排序规则仍属 `PAR-NET-14`」一句的归类已由 [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定七改判——候选生成、硬约束过滤、排序策略族属产品策略，开发方现在就做（首个形态见 ADR-0148）。**适用场景**：只限决定七所列产品策略那几项；网络目录内容、阈值、日历与截单、选用哪种排序策略仍是租户取值，同条「本记录一条都不定」与「适配器只按范围取行、不评估、不排序、不补默认」不变。其余各条（三格端口、专格未决原因、空册与空集合可分辨）不变。
```

### ADR-0053: 网络事实族是推导结果不可登记；登记册只登定义存在，无解析层时一律答未配置

处理：来向两写合并；补指针

合并前（main `311adf934`）：

```text
Status: Accepted（**部分停用**：Decision 三的「现在不建表」半句已由 [ADR-0068](./0068-versioned-network-catalog-structure-precedes-rule-content.md) 停用——目录结构由 CONTEXT 硬句定死、不依赖 `PAR-NET-*` 取值，先行落表；Decision 四「查无登记 → `未配置`」一格已由 [ADR-0148](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定六停用，自票 routing-first-cut/07 落地的那一笔起——`未配置`改由目录修订锚与判断时点适用于该服务目的的路由策略版本答，定义登记册（`0007`）不再被读，表按迁移不可改的纪律留着；同条第二格「查到登记但本进程没有解析层 → 响亮报错」（今天读作「登记了而解不出」）与第三格「禁止：查到登记就答 `configured=true` 带空事实族」不变。「不从事实族倒推」「内容形态不替租户拟」与其余各条不变）<!-- 索引注记 -->｜**部分停用**：其 Decision 三中「现在不建表」半句已由 [ADR-0068](./0068-versioned-network-catalog-structure-precedes-rule-content.md) 停用——目录**结构**由 CONTEXT 硬句定死不依赖取值，先行落表；「不从事实族倒推」「内容形态不替租户拟」与其余各条不变。其 Decision 四「查无登记 → `未配置`」一格已由 [ADR-0148](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定六停用（自 routing-first-cut/07 落地起）——`未配置`改由目录修订锚与适用于该服务目的的路由策略版本答，定义登记册不再被读；同条「登记了而解不出 → 响亮报错」与「禁止带空事实族答已配置」两格不变，其余各条不变。
```

合并后：

```text
Status: Accepted<!-- 索引注记 -->｜**部分停用**：其 Decision 三中「现在不建表」半句已由 [ADR-0068](./0068-versioned-network-catalog-structure-precedes-rule-content.md) 停用——目录**结构**由 CONTEXT 硬句定死、不依赖 `PAR-NET-*` 取值，先行落表；Consequences「不要从事实族倒推」与 Decision 三的理由「列出来的字段是替租户拟的」（ADR-0068 称作「不从事实族倒推」「内容形态不替租户拟」两条护栏）与其余各条不变。**部分停用**：其 Decision 四「查无登记 → `未配置`」一格已由 [ADR-0148](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定六停用，自票 routing-first-cut/07 落地的那一笔起——`未配置`改由目录修订锚与判断时点适用于该服务目的的路由策略版本答，定义登记册（`0007`）不再被读，表按迁移不可改的纪律留着；同条第二格「查到登记但本进程没有解析层 → 响亮报错」（今天读作「登记了而解不出」）与第三格「禁止：查到登记就答 `configured=true` 带空事实族」不变，其余各条不变。**部分停用**（指针 2026-10-21 补）：其 Decision 二「事实族由解析层从原语派生，解析层属 `PAR-NET-14`」一句里解析层的归类已由 [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定七改判——候选生成属产品策略，开发方现在就做（首个形态见 ADR-0148）。**适用场景**：只限决定七所列产品策略那几项；网络目录内容、阈值、日历与截单、选用哪种排序策略仍是租户取值，「事实族由解析层从原语派生」与「可登记的是定义原语，不是事实族」不变。
```

### ADR-0054: 接受前财务控制策略视图增设「未配置」格

处理：措辞对齐（接受出处、去向的模糊重说，或引文改为逐字）

合并前（main `311adf934`）：

```text
Status: Accepted
```

合并后：

```text
Status: Accepted<!-- 索引注记 -->｜**引用更正**（2026-10-21）：Decision 二里指向 ADR-0029 的链接文件名写错（`./0029-recovery-action-is-the-error-algebra.md`，本目录没有这个文件），所指是 [ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)；正文不改，Links 节已改正
```

### ADR-0055: 业务端点 Intake 增设「未配置」格，端点装配属机制半边

处理：补指针

合并前（main `311adf934`）：

```text
Status: Accepted（**数目勘误**：正文所说「七个端点/七个处理器」沿承开发主线的算术口径错——逐项枚举五组（PS 提交/撤回、NO 收寄登记、TF 交付登记双端点、VE 视图查询/索赔受理、CC 外部结果接收）实为**八**个端点、八个 `New*Endpoint` 构造，TF 双端点此前被计作一项。以逐项枚举与 `cmd/parcel-api` 的装配测试为准，各条决定不受影响；实现 `93165cd` 已按八个装配）
```

合并后：

```text
Status: Accepted（**数目勘误**：正文所说「七个端点/七个处理器」沿承开发主线的算术口径错——逐项枚举五组（PS 提交/撤回、NO 收寄登记、TF 交付登记双端点、VE 视图查询/索赔受理、CC 外部结果接收）实为**八**个端点、八个 `New*Endpoint` 构造，TF 双端点此前被计作一项。以逐项枚举与 `cmd/parcel-api` 的装配测试为准，各条决定不受影响；实现 `93165cd` 已按八个装配）<!-- 索引注记 -->｜**部分停用**（指针 2026-10-21 补）：其决定五「本记录不解锁 Intake 的另两项未决。」所说「载荷规范化摘要与准入范围装配（`PAR-GOV-03..07`）仍然拦着真渠道 Intake」已由 [ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定四按口逐一解——每个命令口由所属上下文定一版带形状版本的规范化形状、未定形状的口不开真渠道；生产形态下铸信封前按准入范围读 pilot-governance 当前的生产权威区间，不覆盖即答「不在准入范围」；客户面各口由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) 决定八照办。**适用场景**：作业事实、外部结果与外部资金事实两族的口（ADR-0149）与首方客户族各口（ADR-0139）；区间内容（`PAR-GOV-03..07`）仍是租户取值，「真渠道落地时它们必须先行或同批」与其余各条不变｜**部分停用**（指针 2026-10-21 补，话题通读所得）：其决定三「状态码取 `403`，不取 404、401 或任何 5xx」里对 `401` 的否决（「401 邀请调用方换凭证重试，而此刻不存在任何凭证方案」）随前件在各真渠道族上解除——操作者族由 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定四、作业事实与集成客户端两族由 [ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定二、三沿用同一答复代数、首方客户族由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) 决定四，令牌缺失或校验不过答 `401`。**适用场景**：只限已立真渠道的族；发行方未配置照旧答 `403` `ACCESS_CHANNEL_NOT_CONFIGURED`，其余各条不变
```

### ADR-0064: 初始路由适用性闭包标识从已接受解析标识回指

处理：来向两写合并

合并前（main `311adf934`）：

```text
Status: Accepted。部分停用：后果里「可达性那条链的 `ReachabilityClosureIdentity` 不变」一句，以及 Context 里「`ReachabilityClosureIdentity` 仍属实例半边（可达性判断键同样没有解析标识，且本记录不管 UC-NR-002）」一句，已由 [ADR-0156](./0156-reachability-closure-identity-comes-from-the-adopted-resolution.md) 停用。该符号已不存在。决定一至四与初始路由那条回指不变。正文不改写。<!-- 索引注记 -->｜**部分停用**：后果里「可达性那条链的 `ReachabilityClosureIdentity` 不变」一句，以及 Context 里「`ReachabilityClosureIdentity` 仍属实例半边（可达性判断键同样没有解析标识，且本记录不管 UC-NR-002）」一句，已由 [ADR-0156](./0156-reachability-closure-identity-comes-from-the-adopted-resolution.md) 停用。该符号已不存在。决定一至四与初始路由那条回指不变。正文不改写
```

合并后：

```text
Status: Accepted<!-- 索引注记 -->｜**部分停用**：后果里「可达性那条链的 `ReachabilityClosureIdentity` 不变」一句，以及 Context 里「`ReachabilityClosureIdentity` 仍属实例半边（可达性判断键同样没有解析标识，且本记录不管 UC-NR-002）」一句，已由 [ADR-0156](./0156-reachability-closure-identity-comes-from-the-adopted-resolution.md) 停用。该符号已不存在。决定一至四与初始路由那条回指不变。正文不改写
```

### ADR-0068: 版本化网络目录结构先行——七表由 CONTEXT 硬句推导，内容列与折叠规则等 PAR-NET-14

处理：来向两写合并；补指针

合并前（main `311adf934`）：

```text
Status: Accepted（**部分停用**：决定六「三个证据视图仍不读本目录」一句与其半句「目录修订与视图修订的合流属解析层设计，届时再裁」已由 [ADR-0148](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定六停用，自票 routing-first-cut/07 落地的那一笔起——可达性证据视图从本目录折出事实，`未配置`改由目录修订锚与判断时点适用于该服务目的的路由策略版本答，视图修订就是目录修订锚，0007 的定义登记册不再被读；初始路由证据视图同一条判法答`未配置`，已配置时在 routing-first-cut/09 之前照旧响亮上抛。护栏两处中迁移 `0008` 头注按 checksum 固定不改，改由迁移 `0011` 头注与 `NetworkCatalog` 类型注释承载。其余各条不变）<!-- 索引注记 -->｜**部分停用**：其决定六「三个证据视图仍不读本目录」一句（连同「目录修订与视图修订的合流属解析层设计，届时再裁」半句）已由 [ADR-0148](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定六停用（自 routing-first-cut/07 落地起）——可达性证据视图从本目录折出事实，视图修订就是目录修订锚。其余各条不变
```

合并后：

```text
Status: Accepted<!-- 索引注记 -->｜**部分停用**：其决定六「三个证据视图仍不读本目录」一句与其半句「目录修订与视图修订的合流属解析层设计，届时再裁」已由 [ADR-0148](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定六停用，自票 routing-first-cut/07 落地的那一笔起——可达性证据视图从本目录折出事实，`未配置`改由目录修订锚与判断时点适用于该服务目的的路由策略版本答，视图修订就是目录修订锚，0007 的定义登记册不再被读；初始路由证据视图同一条判法答`未配置`，已配置时在 routing-first-cut/09 之前照旧响亮上抛。护栏两处中迁移 `0008` 头注按 checksum 固定不改，改由迁移 `0011` 头注与 `NetworkCatalog` 类型注释承载。其余各条不变。**部分停用**（指针 2026-10-21 补）：其决定三中「路由策略的规则正文（过滤、排序、冻结边界、改善阈值、改路条件与权限）」「全属 `PAR-NET-14`」与决定六中「把目录折成逐候选事实的解析层属 `PAR-NET-14`」两处的归类已由 [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定七改判——候选生成、硬约束过滤、排序策略族、冻结边界与已执行前缀的判定、自动 / 人工改路的判断结构属产品策略，开发方现在就做（首个形态见 ADR-0148，排序形态列由迁移 `0010` 扩出）。**适用场景**：只限决定七所列产品策略那几项；改善阈值、权限分派、服务区域地理覆盖与服务日历内容仍是租户取值，决定三「等形态定了以**新迁移扩列**，不改已施加迁移」不变，其余各条不变
```

### ADR-0072：接入渠道与凭据验证归共享接入身份能力，登记册形状等真渠道证据

处理：来向两写合并

合并前（main `311adf934`）：

```text
Status: 已接受（**部分停用**，2026-10-19：Decision 2「登记册表结构与凭据形态在 `PAR-INT-01` 最低证据到位前不立」的适用场景再由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一收窄为**租户既有渠道的适配**；首方客户族的册与凭据形态由 ADR-0139 Decision 二定。2026-08-21，MCP-3 受用户委托裁断。**部分停用**，2026-09-03：Decision 2 中「登记册表结构与凭据形态在 `PAR-INT-01` 最低证据（该租户渠道的现行流程）到位前不立」一句已由 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 收窄——**适用场景**限客户接入渠道的登记行与凭据；管理台运营操作者渠道的登记册结构与凭据形态由产品定义、现在就立。Decision 1、3 与其余各条不变）<!-- 索引注记 -->｜**部分停用**（2026-10-19）：Decision 2「登记册表结构与凭据形态在 `PAR-INT-01` 最低证据到位前不立」已由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一 停用，**适用场景**再收窄到租户既有渠道的适配；其余各条不变。**部分停用**：其 Decision 2「登记册表结构与凭据形态在 `PAR-INT-01` 最低证据（该租户渠道的现行流程）到位前不立」一句已由 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 收窄——**适用场景**限客户接入渠道的登记行与凭据；管理台运营操作者渠道的登记册结构与凭据形态由产品定义、现在就立。Decision 1（能力归 `internal/accessidentity`）、Decision 3 与其余各条不变
```

合并后：

```text
Status: 已接受（2026-08-21，MCP-3 受用户委托裁断）<!-- 索引注记 -->｜**部分停用**（2026-10-19）：其 Decision 2「登记册表结构与凭据形态在 `PAR-INT-01` 最低证据（该租户渠道的现行流程）到位前不立」已由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一停用，**适用场景**再收窄为**租户既有渠道的适配**；首方客户族的册与凭据形态由 ADR-0139 Decision 二定；其余各条不变。**部分停用**（2026-09-03）：其 Decision 2「登记册表结构与凭据形态在 `PAR-INT-01` 最低证据（该租户渠道的现行流程）到位前不立」一句已由 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 收窄——**适用场景**限客户接入渠道的登记行与凭据；管理台运营操作者渠道的登记册结构与凭据形态由产品定义、现在就立。Decision 1（能力归 `internal/accessidentity`）、Decision 3 与其余各条不变
```

### ADR-0075: 客户地址随判断请求在请求期携带过界，network-routing 不建读取端口

处理：来向两写合并

合并前（main `311adf934`）：

```text
Status: Accepted。部分停用：[ADR-0215](./0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md) 决定二（被停条款：决定一「`network-routing` 不建立读取 `parcel-shipment` 地址的端口」；适用场景只限受理之后、不由小包托运信封触发的路由复核，NR 经消费侧适配器读受理基线那一版提交的地理投影、只到地理维；适用场景与停用前提见 adr/README 该行，其余各条不变，本记录正文不改写）<!-- 索引注记 -->｜**部分停用**（2026-10-19）：决定一「`network-routing` 不建立读取 `parcel-shipment` 地址的端口」已由 [ADR-0215](./0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md) 决定二停用。**适用场景**：仅限受理之后、不由小包托运信封触发的路由复核（今天只有权威运输交接进入节点这一种），NR 经消费侧适配器按（租户，包裹）取受理基线那一版提交的地理投影，只取用地理维，地址本体不取用、不落——停用理由的前提是受理基线一经固定不再改写，读回的那一版与判断键上的基线同版，本记录防的异版窗口在这里不存在；可达性判断、初始路由与收寄采用触发的复核照旧随请求携带。决定二、决定三与决定一其余部分不变，其余各条不变
```

合并后：

```text
Status: Accepted<!-- 索引注记 -->｜**部分停用**（2026-10-19）：决定一「`network-routing` 不建立读取 `parcel-shipment` 地址的端口」已由 [ADR-0215](./0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md) 决定二停用。**适用场景**：仅限受理之后、不由小包托运信封触发的路由复核（今天只有权威运输交接进入节点这一种），NR 经消费侧适配器按（租户，包裹）取受理基线那一版提交的地理投影，只取用地理维，地址本体不取用、不落——停用理由的前提是受理基线一经固定不再改写，读回的那一版与判断键上的基线同版，本记录防的异版窗口在这里不存在；可达性判断、初始路由与收寄采用触发的复核照旧随请求携带。决定二、决定三与决定一其余部分不变，其余各条不变，本记录正文不改写
```

### ADR-0078: 隔离环境运营查阅按装配注入放行——合成租户显式入参、缺省朝拦，写路径与客户查阅面维持未配置即拒

处理：来向两写合并；补指针

合并前（main `311adf934`）：

```text
Status: Accepted（2026-08-26，用户经 IDP 队列通道 1 委托本会话在取证简报三路选项间裁断；取证材料为[读面准入简报](../../.scratch/product-story-and-demo/read-admission-brief.md)，其四部分材料是本记录的事实底座，本文不复述取证细节。**勘误**，2026-08-26 实现取证：Decision 一第三判据原写「无客户维」、Decision 二原写「不读请求的任何部分」，均比代码实况写宽——`/shipment-request-views` 的 `AuthorizedQueryScope` 带可见客户账户维（授权结果的可见集，非调用方身份主张），其详情分支按既有分工在 Intake 内解析定位标识 `shipmentRequestId`。两句已按此更正；放行面枚举、排除面与各条界线不变。**部分停用**，2026-09-02：Decision 四中「按环境选择的只有装配点上查阅行的 Intake 一件事」一句已由 [ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) 停用，适用面改由入格判据裁定；同条另两句与其余各条不变）<!-- 索引注记 -->｜**部分停用**：其 Decision 四中「按环境选择的只有装配点上查阅行的 Intake 一件事」一句已由 [ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) 停用——适用面由枚举改为入格判据（注入值全为 `SYN-` 合成、不采信自报身份、生产装配无此路径），写路径的命令面 Intake 与生产归属范围目录据此入格。同条另两句（不得据此再添 demo/mode 类全局开关或第二个 main；pgtest 环境处理通例不变）与其余各条不变。**适用场景**：仅限满足那三条判据的缝；`/customer-tracking-view` 的排除判据（客户维是调用方自己的身份主张）不因判据化而松动
```

合并后：

```text
Status: Accepted（2026-08-26，用户经 IDP 队列通道 1 委托本会话在取证简报三路选项间裁断；取证材料为[读面准入简报](../../.scratch/product-story-and-demo/read-admission-brief.md)，其四部分材料是本记录的事实底座，本文不复述取证细节。**勘误**，2026-08-26 实现取证：Decision 一第三判据原写「无客户维」、Decision 二原写「不读请求的任何部分」，均比代码实况写宽——`/shipment-request-views` 的 `AuthorizedQueryScope` 带可见客户账户维（授权结果的可见集，非调用方身份主张），其详情分支按既有分工在 Intake 内解析定位标识 `shipmentRequestId`。两句已按此更正；放行面枚举、排除面与各条界线不变。）<!-- 索引注记 -->｜**部分停用**（2026-09-02）：其 Decision 四中「按环境选择的只有装配点上查阅行的 Intake 一件事」一句已由 [ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) 停用——适用面由枚举改为入格判据（注入值全为 `SYN-` 合成、不采信自报身份、生产装配无此路径），写路径的命令面 Intake 与生产归属范围目录据此入格。同条另两句（不得据此再添 demo/mode 类全局开关或第二个 main；pgtest 环境处理通例不变）与其余各条不变。**适用场景**：仅限满足那三条判据的缝；`/customer-tracking-view` 的排除判据（客户维是调用方自己的身份主张）不因判据化而松动。**部分停用**（指针 2026-10-21 补）：其 Decision 四「隔离读面准入不因真渠道出现自动退场」一句已由 [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定三停用——隔离形态是真渠道缺位时的过渡，某一口在装配点换上真渠道 Intake 的同一笔撤下该口的隔离放行（ADR-0150 越权风险点 4 预留的那一格；它所说「ADR-0078 与 ADR-0091 其余各条……一字不变」指过渡期规则，不含这一句）。**适用场景**：只限已换上真渠道 Intake 的口；Decision 一、三枚举的查阅行因此只在尚未换真渠道的口上仍是放行面，缺省朝拦、`SYN-` 前缀门禁与 `/customer-tracking-view` 的排除判据不变。**部分停用**（指针 2026-10-21 补）：其 Decision 五「载荷规范化摘要与准入范围装配两项未决照旧拦着真渠道 Intake 与一切命令面」一句里「一切命令面」已由 [ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) 停用（**适用场景**：隔离写开关逐口放行的命令面），「两项未决照旧拦着」已由 [ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定四按口逐一解；其余各条不变
```

### ADR-0079: 接受前控制策略视图凭商业解析回指提问——消费方只回显标识，提供方从已固定闭包取合同

处理：补指针

合并前（main `311adf934`）：

```text
Status: Accepted
```

合并后：

```text
Status: Accepted<!-- 索引注记 -->｜**表述更正**（指针 2026-10-21 补）：决定五「释放命令不带回指：释放不问控制策略，它按原请求身份在冻结账本上认领」里「在冻结账本上认领」一语成文时就写窄了，以 [ADR-0047](./0047-terms-control-forms-credit-exposure-not-a-freeze.md) 决定四「释放按原关联双账本认领」与 [ADR-0122](./0122-pre-acceptance-control-executes-the-policy-content-items-and-parcel-shipment-folds-by-the-joint-pass-condition.md) 决定三「释放两本账各认领一次」为准；该句的论点「释放不问控制策略」、不用回指与其余各条不变
```

### ADR-0081: 接受判断由信封驱动——提交落库即交出「委托已提交」，推进落在派发一拍；提交事务随之改两段边界

处理：来向两写合并；补指针

合并前（main `311adf934`）：

```text
Status: Accepted（部分停用：决定六里「结算账户目录留 nil」这一格由 [ADR-0158](./0158-settlement-account-register-is-an-immutable-tuple.md) 停用——生产装配接结算账户登记册。决定六其余各格不由该记录停用。）<!-- 索引注记 -->｜**部分停用**：决定六「结算账户目录留 nil」由 [ADR-0158](./0158-settlement-account-register-is-an-immutable-tuple.md) 停用；决定六其余各格仍有效
```

合并后：

```text
Status: Accepted<!-- 索引注记 -->｜**部分停用**：决定六「时点取值源、可达性闭包标识、结算账户目录、控制金额源全部留 nil」四格已逐格接上——结算账户目录一格由 [ADR-0158](./0158-settlement-account-register-is-an-immutable-tuple.md) 停用，生产装配接结算账户登记册；可达性闭包标识一格由 [ADR-0156](./0156-reachability-closure-identity-comes-from-the-adopted-resolution.md) 停用（指针 2026-10-21 补），生产装配不再传入 nil 身份映射；控制金额源一格由 [ADR-0179](./0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md) 决定一停用（指针 2026-10-21 补），受理前控制金额改由结算形成估价；时点取值源一格按 [ADR-0157](./0157-built-in-as-of-forms-are-adopted-as-reference-citations.md) 的内置时点形态「提交接收」接上（指针 2026-10-21 补，装配见 `pspartycommercial.NewSubmissionReceiptAsOf`）。决定六其余（解析键登记面接真、空册按「显式未配置」答`解析未决`）与其余各条不变
```

### ADR-0085: 登记册配置写面进端点表带未配置格——写表单属产品能力，CLI 保留为受控批量口；写准入不另立形，与其余命令面同等真渠道证据

处理：来向两写合并；补指针

合并前（main `311adf934`）：

```text
Status: Accepted（**部分停用**，2026-10-19：Decision 二同日补记「两族各自要过 `PAR-INT-01` 的真证据门，不得互相顶替」的客户半由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一收窄为**租户既有渠道的适配**（首方族不过该门）；「不得互相顶替」照旧。2026-08-31，用户经 IDP 队列通道 3 授权本会话「作为业务和系统专家直接开干」，据此对 [admin-write-faces/01](../../.scratch/admin-write-faces/issues/01-registry-configuration-has-no-admin-write-face.md) 的三项待裁作出裁决。裁决能力边界：读过 ADR-0055/0072/0077/0078 全文、`assembleBusinessEndpoints` 装配表、`cmd/parcel-pricing-register` 与 parcel-pricing 登记用例先例、closure-batch spec 与产品基线「产品就绪与试点就绪」「首发范围约束」两节；未逐一重读各上下文 `CONTEXT.md`——本记录因此只裁结构（写面走哪条准入、落在哪一层），不裁任何单册的领域不变量，各册表单形状与答案代数由实施票按其登记用例照抄。**部分停用**，2026-09-03：Decision 二补记中「两族各自要过 `PAR-INT-01` 的真证据门」一句的前半，与 Decision 二「本记录新增的登记端点也在被拦之列」一句，已由 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 停用——**适用场景**限管理台操作者族与登记册配置写面；「不得互相顶替」后半保留，客户业务面照旧过 `PAR-INT-01` 并被 ADR-0055 Decision 五两项拦着。另，票 admin-write-faces/01 评论对 Decision 三「翻译属渠道接入契约」的引申已由 [ADR-0101](./0101-operator-facing-registration-payload-shape-is-product-defined.md) 收窄为客户渠道载荷，Decision 三原句不变。其余各条不变）<!-- 索引注记 -->｜**部分停用**（2026-10-19）：Decision 二补记「两族各自要过 `PAR-INT-01` 的真证据门」的客户半已由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一 停用，**适用场景**收窄到租户既有渠道的适配；其余各条不变。**部分停用**：其 Decision 二补记「两族各自要过 `PAR-INT-01` 的真证据门」一句的前半，与 Decision 二「本记录新增的登记端点也在被拦之列」一句，已由 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 停用——**适用场景**限管理台操作者族与登记册配置写面；「不得互相顶替」后半保留，客户业务面照旧过 `PAR-INT-01` 并被 ADR-0055 Decision 五两项拦着。Decision 三原句不变，票 01 评论对它「翻译属渠道接入契约」的引申由 [ADR-0101](./0101-operator-facing-registration-payload-shape-is-product-defined.md) 收窄为客户渠道载荷。其余各条不变
```

合并后：

```text
Status: Accepted（2026-08-31，用户经 IDP 队列通道 3 授权本会话「作为业务和系统专家直接开干」，据此对 [admin-write-faces/01](../../.scratch/admin-write-faces/issues/01-registry-configuration-has-no-admin-write-face.md) 的三项待裁作出裁决。裁决能力边界：读过 ADR-0055/0072/0077/0078 全文、`assembleBusinessEndpoints` 装配表、`cmd/parcel-pricing-register` 与 parcel-pricing 登记用例先例、closure-batch spec 与产品基线「产品就绪与试点就绪」「首发范围约束」两节；未逐一重读各上下文 `CONTEXT.md`——本记录因此只裁结构（写面走哪条准入、落在哪一层），不裁任何单册的领域不变量，各册表单形状与答案代数由实施票按其登记用例照抄。）<!-- 索引注记 -->｜**部分停用**（2026-10-19）：Decision 二同日补记「两族各自要过 `PAR-INT-01` 的真证据门，不得互相顶替」的客户半已由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一停用，**适用场景**收窄为**租户既有渠道的适配**（首方族不过该门）；「不得互相顶替」照旧，其余各条不变。**部分停用**（2026-09-03）：其 Decision 二补记「两族各自要过 `PAR-INT-01` 的真证据门」一句的前半，与 Decision 二「本记录新增的登记端点也在被拦之列」一句，已由 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 停用——**适用场景**限管理台操作者族与登记册配置写面；「不得互相顶替」后半保留，客户业务面照旧过 `PAR-INT-01` 并被 ADR-0055 Decision 五两项拦着。Decision 三原句不变，票 admin-write-faces/01 评论对它「翻译属渠道接入契约」的引申由 [ADR-0101](./0101-operator-facing-registration-payload-shape-is-product-defined.md) 收窄为客户渠道载荷。其余各条不变｜**部分停用**（指针 2026-10-21 补，话题通读所得）：其决定二「ADR-0055 Decision 五两项未决照拦一切命令面」与同日补记「且 ADR-0055 Decision 五两项未决照拦，本补记不预裁也不缩短」两处所说的两项未决，已由 [ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定四按口逐一解，客户面各口由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) 决定八照办。**适用场景**：作业事实、外部结果与外部资金事实两族的口与首方客户族各口；区间内容（`PAR-GOV-03..07`）仍是租户取值，「替换缝就是 ADR-0055 预留的那条」与其余各条不变
```

### ADR-0086: 等待人工复核是入账暂停——暂停与等待态同事务落库，续办由「复核已完成」信封另行驱动

处理：来向两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（部分停用：Context 把`等待受控补充`判在「回滚重投是对的」一侧的那一半，已由 [ADR-0106](./0106-customer-supplement-wait-is-a-committed-pause-resumed-by-the-new-submission-version-envelope.md) 停用——该等待态改为提交入账、由「新提交版本已形成」信封驱动续办；`等待内部续办`的分侧与 Decision 一至四不变）<!-- 索引注记 -->｜**部分停用**：其 Context 把`等待受控补充`判在「回滚重投是对的」那一侧的分侧——原句「等待受控补充 / 等待内部续办：重投重跑同一轮，等的依赖（客户新提交版本、抖动的内部依赖）会自己回来」中关于**客户新提交版本**的那一半——已由 [ADR-0106](./0106-customer-supplement-wait-is-a-committed-pause-resumed-by-the-new-submission-version-envelope.md) 停用，理由是代码证实新版本不会自己回来、回来了也不是在途那封信封受益。**适用场景**：只停`等待受控补充`那一半；`等待内部续办`的分侧与 Decision 一至四不变，其余各条不变
```

合并后：

```text
Status: Accepted<!-- 索引注记 -->｜**部分停用**：其 Context 把`等待受控补充`判在「回滚重投是对的」那一侧的分侧——原句「等待受控补充 / 等待内部续办：重投重跑同一轮，等的依赖（客户新提交版本、抖动的内部依赖）会自己回来」中关于**客户新提交版本**的那一半——已由 [ADR-0106](./0106-customer-supplement-wait-is-a-committed-pause-resumed-by-the-new-submission-version-envelope.md) 停用，该等待态改为提交入账、由「新提交版本已形成」信封驱动续办，理由是代码证实新版本不会自己回来、回来了也不是在途那封信封受益。**适用场景**：只停`等待受控补充`那一半；`等待内部续办`的分侧与 Decision 一至四不变，其余各条不变
```

### ADR-0091：隔离形态从查阅面扩到写路径，按分级开关放行——入格判据取代枚举；写面有持久化，可分辨物由 `SYN-` 前缀承担

处理：来向两写合并；补指针

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-02，用户经 IDP 队列通道 5 在四路范围选项间裁定「一份 ADR 同时裁两格」；本记录据此把写面 Intake 与生产归属范围目录合并裁决）；**部分停用**：决定六「隔离形态不因真渠道出现而自动退场」一句已由 [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 停用——隔离形态是真渠道缺位时的过渡，某一口真渠道 Intake 落地即在同一笔撤下该口的隔离放行；同句「也不因它出现而获得任何真实租户」与其余各条不变<!-- 索引注记 -->｜**部分停用**：其决定六「隔离形态不因真渠道出现而自动退场」一句已由 [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 停用——隔离形态是真渠道缺位时的过渡，某一口真渠道 Intake 落地即在同一笔撤下该口的隔离放行。同句「也不因它出现而获得任何真实租户」与其余各条不变
```

合并后：

```text
Status: Accepted（2026-09-02，用户经 IDP 队列通道 5 在四路范围选项间裁定「一份 ADR 同时裁两格」；本记录据此把写面 Intake 与生产归属范围目录合并裁决。另：决定一至四逐口放行隔离写面，使 [ADR-0078](./0078-isolated-environment-operations-reads-admit-by-assembly-injection.md) 决定五「两项未决照旧拦着真渠道 Intake 与一切命令面」里「一切命令面」不再成立，适用场景是隔离写开关逐口放行的命令面（指针 2026-10-21 补））<!-- 索引注记 -->｜**部分停用**：其决定六「隔离形态不因真渠道出现而自动退场」一句已由 [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 停用——隔离形态是真渠道缺位时的过渡，某一口真渠道 Intake 落地即在同一笔撤下该口的隔离放行。同句「也不因它出现而获得任何真实租户」与其余各条不变
```

### ADR-0094: 未决重投与否由领域的 `ResumePath` 决定，并增设「等运营登记」第四格

处理：来向两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（部分停用：Decision 三对 `ResumeByCustomerSupplement` 的「维持回滚重投」一格，已由 [ADR-0106](./0106-customer-supplement-wait-is-a-committed-pause-resumed-by-the-new-submission-version-envelope.md) 改为提交入账——那一格写明「另立取证票，不在本记录裁」，取证票 first-tenant-runway/09 的三问已从代码答出；Decision 一、二、四、五与其余三格不变）<!-- 索引注记 -->｜**分格判据**：看恢复动作，不看缺了什么——重试、客户补件、人工复核都推不动、只有一次登记动作推得动的，是新的第四格。**适用场景**：接受判断链的未决处置；新增未决原因时先答它的恢复动作，消费门不再持有原因字面量｜**部分停用**：其 Decision 三对 `ResumeByCustomerSupplement` 的「维持回滚重投」一格——原句「`ResumeByCustomerSupplement` → **维持回滚重投**。ADR-0086 的 Context 判过这一格……本记录**不改它**」——已由 [ADR-0106](./0106-customer-supplement-wait-is-a-committed-pause-resumed-by-the-new-submission-version-envelope.md) 停用；该格当时写明「另立取证票，不在本记录裁」，取证票即 first-tenant-runway/09，证据齐后改为提交入账。**适用场景**：只停那一格的处置；Decision 一、二、四、五与其余三格不变，其余各条不变
```

合并后：

```text
Status: Accepted<!-- 索引注记 -->｜**分格判据**：看恢复动作，不看缺了什么——重试、客户补件、人工复核都推不动、只有一次登记动作推得动的，是新的第四格。**适用场景**：接受判断链的未决处置；新增未决原因时先答它的恢复动作，消费门不再持有原因字面量｜**部分停用**：其 Decision 三对 `ResumeByCustomerSupplement` 的「维持回滚重投」一格——原句「`ResumeByCustomerSupplement` → **维持回滚重投**。ADR-0086 的 Context 判过这一格……本记录**不改它**」——已由 [ADR-0106](./0106-customer-supplement-wait-is-a-committed-pause-resumed-by-the-new-submission-version-envelope.md) 停用；该格当时写明「另立取证票，不在本记录裁」，取证票即 first-tenant-runway/09，其三问已从代码答出，改为提交入账。**适用场景**：只停那一格的处置；Decision 一、二、四、五与其余三格不变，其余各条不变
```

### ADR-0098: 失败尝试费发生项不在揽收编排里形成，采购上下文显式给出而不推导

处理：来向两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（**部分停用**：决定四「**今天不建**：建它等于在没有任何租户实证的情况下先定一种采购组织方式（揽收是挂在委托下、还是各自独立由第三方关系相连）」已由 [ADR-0204](./0204-a-fulfillment-segment-adopts-a-transport-commission-by-declaration-and-a-started-commission-cannot-be-cancelled.md) 决定五停用，适用场景只限实际履约段与运输委托之间的连线；揽收任务与尝试 ↔ 委托仍不建，其余各条不变，本记录正文不改写。）<!-- 索引注记 -->｜**位置判据**：CONTEXT 禁「任一对象的成功、失败或取消被压缩成其他对象的状态」，而把派生塞进一个必须照常成功的登记编排，会造出静默不发生且无人重试的一步（同 ADR-0094 那一类）。**适用场景**：一个派生事实该在哪个编排里形成；「不适用」与「未决」「报错」三格恢复动作不同必须分开；揽收↔委托的连线显式留空待租户实证｜**部分停用**：其决定四「**今天不建**：建它等于在没有任何租户实证的情况下先定一种采购组织方式（揽收是挂在委托下、还是各自独立由第三方关系相连）」已由 [ADR-0204](./0204-a-fulfillment-segment-adopts-a-transport-commission-by-declaration-and-a-started-commission-cannot-be-cancelled.md) 决定五停用。**适用场景**：仅限实际履约段与运输委托之间的连线——停用理由的前提是「先定一种采购组织方式」，而那条连线由登记方逐段声明、产品不替租户定组织方式；揽收任务与尝试 ↔ 委托仍不建（失败尝试不成段，段级连线够不着它）。其余各条不变
```

合并后：

```text
Status: Accepted<!-- 索引注记 -->｜**位置判据**：CONTEXT 禁「任一对象的成功、失败或取消被压缩成其他对象的状态」，而把派生塞进一个必须照常成功的登记编排，会造出静默不发生且无人重试的一步（同 ADR-0094 那一类）。**适用场景**：一个派生事实该在哪个编排里形成；「不适用」与「未决」「报错」三格恢复动作不同必须分开；揽收↔委托的连线显式留空待租户实证｜**部分停用**：其决定四「**今天不建**：建它等于在没有任何租户实证的情况下先定一种采购组织方式（揽收是挂在委托下、还是各自独立由第三方关系相连）」已由 [ADR-0204](./0204-a-fulfillment-segment-adopts-a-transport-commission-by-declaration-and-a-started-commission-cannot-be-cancelled.md) 决定五停用。**适用场景**：仅限实际履约段与运输委托之间的连线——停用理由的前提是「先定一种采购组织方式」，而那条连线由登记方逐段声明、产品不替租户定组织方式；揽收任务与尝试 ↔ 委托仍不建（失败尝试不成段，段级连线够不着它）。其余各条不变，本记录正文不改写
```

### ADR-0100：管理台运营操作者身份是产品自有的接入渠道族，属机制半边——信任锚、校验方式与操作者—租户授权模型由产品定义，不等 `PAR-INT-01`；登记册配置写面的真 Intake 据此现在就立，ADR-0055 Decision 五两项未决对该写面不适用

处理：来向两写合并；补指针

合并前（main `311adf934`）：

```text
Status: Accepted（**部分停用**，2026-10-19：决定四末段「客户业务命令面（委托提交与撤回、作业事实登记、外部结果接收）照旧被两项未决拦着」余下的客户委托一半由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一停用，**适用场景**：首方族的客户面各口，ADR-0055 Decision 五两项按 ADR-0139 Decision 八按口解。2026-09-03，用户经 IDP 队列通道 4 先质疑「作为一个独立的软件产品……不可能都通过 cli」，再授权本会话「作为业务和系统专家自决，先按建议出 ADR」，据此裁决。裁决能力边界：读过 [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)、[ADR-0072](./0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)、[ADR-0085](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)、[ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) 全文，`internal/accessidentity` 全部生产文件，[参数登记册](../product/PILOT-PARAMETER-REGISTER.md)的 `PAR-INT-01` 与 `PAR-GOV-03..07` 行，`cmd/parcel-api` 的 `assembleBusinessEndpoints` 装配表，parcel-pricing 的登记端点、登记 CLI 与管理台价卡页；未重读 ADR-0003/0078 全文与各上下文 `CONTEXT.md`，ADR-0019/0020/0021 谱系只核了「跨租户运维后台不属于产品」那一句在现行记录里仍成立——本记录因此只裁**操作者渠道的归属与准入结构**，不裁任何登记册的领域不变量，也不裁客户业务面的准入）；**部分停用**：决定四末段与决定五第一条对「作业事实登记、外部结果接收」的适用已由 [ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 停用（适用场景：作业事实登记面与外部结果、外部资金事实接收面），其余各条不变；决定五第二条中「隔离形态（ADR-0091）继续靠 `SYN-` 写开关服务演示，它与操作者渠道是装配点上的两行，互不替代、不合并；ADR-0091 Decision 六……照旧」一段已由 [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 停用——过渡期内两行并存，某一口真渠道落地即在同一笔撤下该口的隔离行；同条「不做绕过 OIDC 的『开发用』本地账号」不变<!-- 索引注记 -->｜**部分停用**（2026-10-19）：决定四末段「客户业务命令面……照旧被两项未决拦着」余下的客户委托一半已由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一 停用，**适用场景**首方族的客户面各口；其余各条不变。**归族判据**：一族身份的证据由谁出，看它的三件（信任锚、校验方式、授权模型）取决于谁——取决于产品的就是机制半边；`PAR-INT-01` 最低证据列里的每一件都是客户渠道的属性，容不下操作者。**适用场景**：管理台登记册配置写面与目录查阅面的真 Intake，答复按恢复动作分三格（发行方未配置 / 令牌无效 / 无授予）；客户业务命令面照旧过 `PAR-INT-01` 并被 ADR-0055 Decision 五两项拦着，隔离形态的 `SYN-` 写开关与之是装配点上的两行｜**部分停用**：决定四末段「客户业务命令面（委托提交与撤回、作业事实登记、外部结果接收）照旧被两项未决拦着」与决定五第一条「不给客户业务命令面开操作者渠道」对作业事实登记与外部结果接收的适用，已由 [ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 停用；**适用场景**：作业事实登记面与外部结果、外部资金事实接收面；客户委托那一半与其余各条不变。决定五第二条中隔离形态与操作者渠道「互不替代、不合并」一段已由 [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 停用——过渡期内两行并存，某一口真渠道落地即撤下该口的隔离行；同条「不做『开发用』本地账号」不变
```

合并后：

```text
Status: Accepted（2026-09-03，用户经 IDP 队列通道 4 先质疑「作为一个独立的软件产品……不可能都通过 cli」，再授权本会话「作为业务和系统专家自决，先按建议出 ADR」，据此裁决。裁决能力边界：读过 [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)、[ADR-0072](./0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)、[ADR-0085](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)、[ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) 全文，`internal/accessidentity` 全部生产文件，[参数登记册](../product/PILOT-PARAMETER-REGISTER.md)的 `PAR-INT-01` 与 `PAR-GOV-03..07` 行，`cmd/parcel-api` 的 `assembleBusinessEndpoints` 装配表，parcel-pricing 的登记端点、登记 CLI 与管理台价卡页；未重读 ADR-0003/0078 全文与各上下文 `CONTEXT.md`，ADR-0019/0020/0021 谱系只核了「跨租户运维后台不属于产品」那一句在现行记录里仍成立——本记录因此只裁**操作者渠道的归属与准入结构**，不裁任何登记册的领域不变量，也不裁客户业务面的准入）<!-- 索引注记 -->｜**部分停用**（2026-10-19）：决定四末段「客户业务命令面……照旧被两项未决拦着」余下的客户委托一半已由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一停用，**适用场景**首方族的客户面各口，ADR-0055 Decision 五两项按 ADR-0139 Decision 八按口解；其余各条不变。**归族判据**：一族身份的证据由谁出，看它的三件（信任锚、校验方式、授权模型）取决于谁——取决于产品的就是机制半边；`PAR-INT-01` 最低证据列里的每一件都是客户渠道的属性，容不下操作者。**适用场景**：管理台登记册配置写面与目录查阅面的真 Intake，答复按恢复动作分三格（发行方未配置 / 令牌无效 / 无授予）；客户业务命令面照旧过 `PAR-INT-01` 并被 ADR-0055 Decision 五两项拦着，隔离形态的 `SYN-` 写开关与之是装配点上的两行｜**部分停用**：决定四末段「客户业务命令面（委托提交与撤回、作业事实登记、外部结果接收）照旧被两项未决拦着」与决定五第一条「不给客户业务命令面开操作者渠道」对作业事实登记与外部结果接收的适用，已由 [ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 停用；**适用场景**：作业事实登记面与外部结果、外部资金事实接收面；客户委托那一半与其余各条不变。决定五第二条中「隔离形态（ADR-0091）继续靠 `SYN-` 写开关服务演示，它与操作者渠道是装配点上的两行，互不替代、不合并；ADR-0091 Decision 六……照旧」一段已由 [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 停用——过渡期内两行并存，某一口真渠道落地即在同一笔撤下该口的隔离行；同条「不做绕过 OIDC 的「开发用」本地账号」不变｜**已答**（指针 2026-10-21 补，话题通读所得）：决定五末条「不裁 `PAR-GOV-03..07` 对客户业务面还拦不拦：那是另一个问题，超出本记录能力边界」由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) 决定八作答——ADR-0055 Decision 五两项照 ADR-0149 决定四按口解、明文延到客户面各口；无条款停用，其余各条不变
```

### ADR-0101：运营操作者面的登记载荷形状由产品定义，属机制半边——「渠道原始载荷 → 登记快照」的翻译只在客户渠道上属渠道契约；价卡首例：产品发布版本化导入模板，导入形成持久化草稿，校验与发布共用一份规范化摘要，批准是独立的操作者动作，审批职责规则缺省朝拦

处理：来向两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（**部分停用**，2026-10-19：决定一「适用场景收窄为客户渠道载荷」由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一**再收窄为客户既有渠道的载荷**；首方 API 的载荷形状是产品契约（ADR-0140）。2026-09-03，用户经 IDP 队列通道 4 授权本会话「作为业务和系统专家自决，先按建议出 ADR」，与 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 同一次授权、同一批裁决。裁决能力边界：读过 [ADR-0085](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md) 全文与票 [admin-write-faces/01](../../.scratch/admin-write-faces/issues/01-registry-configuration-has-no-admin-write-face.md) 文末「不逐字段建」那条评论、parcel-pricing `CONTEXT.md`「定价方案与价表版本」生命周期与「不采纳的工件形态」两节、参考蓝图 V1.2 第 38 节、`internal/parcelpricing/domain` 的 `PriceCardRegistration` 与方案快照、管理台价卡页与其 `api.ts`、`cmd/parcel-pricing-register`、`scripts/demo-seeds/seedgen`；未重读 parcel-pricing `CONTEXT.md` 其余各节、[计价规则模型最终设计](../design/pp-pricing-rule-model-final-design.md)全文与其他登记册的 `CONTEXT.md`——本记录因此只裁**载荷形状归谁定**与**价卡生命周期中草稿、已校验、已批准、已发布各态的持久化载体**，不改任何一条领域不变量，不裁其他登记册的具体形态）<!-- 索引注记 -->｜**部分停用**（2026-10-19）：决定一「适用场景收窄为客户渠道载荷」已由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一 停用，**适用场景**再收窄到客户既有渠道的载荷；其余各条不变。**形状归属判据**：载荷形状归它唯一的来源——客户渠道的报文是客户系统的既成事实，操作者面的载荷只有产品这一个来源，产品不定义就没人定义。**适用场景**：管理台各登记签的形态由实施票按「登记频次 × 操作者角色 × 载荷结构」逐册裁；价卡首例走模板导入 → 草稿 → 批准 → 交既有 `RegisterPriceCard` 发布，CONTEXT 生命周期的草稿、已校验、已批准、已发布由此各有载体；蓝图「价卡导入与治理」节其余门禁按计价规则模型最终设计的范围判据另裁
```

合并后：

```text
Status: Accepted（2026-09-03，用户经 IDP 队列通道 4 授权本会话「作为业务和系统专家自决，先按建议出 ADR」，与 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 同一次授权、同一批裁决。裁决能力边界：读过 [ADR-0085](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md) 全文与票 [admin-write-faces/01](../../.scratch/admin-write-faces/issues/01-registry-configuration-has-no-admin-write-face.md) 文末「不逐字段建」那条评论、parcel-pricing `CONTEXT.md`「定价方案与价表版本」生命周期与「不采纳的工件形态」两节、参考蓝图 V1.2 第 38 节、`internal/parcelpricing/domain` 的 `PriceCardRegistration` 与方案快照、管理台价卡页与其 `api.ts`、`cmd/parcel-pricing-register`、`scripts/demo-seeds/seedgen`；未重读 parcel-pricing `CONTEXT.md` 其余各节、[计价规则模型最终设计](../design/pp-pricing-rule-model-final-design.md)全文与其他登记册的 `CONTEXT.md`——本记录因此只裁**载荷形状归谁定**与**价卡生命周期中草稿、已校验、已批准、已发布各态的持久化载体**，不改任何一条领域不变量，不裁其他登记册的具体形态）<!-- 索引注记 -->｜**部分停用**（2026-10-19）：决定一「适用场景收窄为客户渠道载荷」已由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一停用，**适用场景**再收窄为**客户既有渠道的载荷**；首方 API 的载荷形状是产品契约（ADR-0140）；其余各条不变。**形状归属判据**：载荷形状归它唯一的来源——客户渠道的报文是客户系统的既成事实，操作者面的载荷只有产品这一个来源，产品不定义就没人定义。**适用场景**：管理台各登记签的形态由实施票按「登记频次 × 操作者角色 × 载荷结构」逐册裁；价卡首例走模板导入 → 草稿 → 批准 → 交既有 `RegisterPriceCard` 发布，CONTEXT 生命周期的草稿、已校验、已批准、已发布由此各有载体；蓝图「价卡导入与治理」节其余门禁按计价规则模型最终设计的范围判据另裁
```

### ADR-0104：客户服务规则版本的正文归 `party-commercial`，首发只进「索赔期限」与「最低材料」两项且子行至少一项；VE 既有的通知策略册与索赔资格声明册是 VE 自己的判断参数，不迁移、不改键；VE 对规则的采用只冻版本引用，解析走既有闭包加一个点读口

处理：来向两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-03，用户经 IDP 队列通道 3 授权本会话「有全部权限」自决并向各会话派工。裁决能力边界：读过票 [party-commercial-context-gaps/05](../../.scratch/party-commercial-context-gaps/issues/05-customer-service-rule-version-has-no-consuming-seam-into-visibility-exception.md) 全文（含 MCP-2 于 `877444a` 的取证与「待 owner 裁的三处」）、`party-commercial` `CONTEXT.md` 中「客户服务规则版本」词条、Rules 两句与 Boundaries 那一句、`visibility-exception` `CONTEXT.md` 索赔一节中期限与材料的硬句、`internal/partycommercial/domain/customer_service_rule.go`、`internal/visibilityexception/adapters/postgres` 的 `claim_eligibility.go` 与 `notification_policy.go` 头注、`migrations/party_commercial/0014` 头注；未重读 VE `CONTEXT.md` 披露与通知各节全文、`UC-VE-007` 与 `UC-VE-008` 全文、VE 编排代码——本记录因此只裁**正文归谁、首发进几项、VE 怎么采用**三件，不改任何一条 VE 不变量、不动 VE 的表与编排）｜**部分停用**：Alternatives「允许显式空版本」条目中「客户服务规则没有对应的回退物——产品没有默认期限」的前提已由 [ADR-0176](./0176-customer-service-rule-contract-tier-selection-and-inheritance.md) 消解，见本文件 Links 节前向指针；其余各条不变<!-- 索引注记 -->｜**看键判归属**：一本册是不是某类正文，看它的键是不是那类正文的键——VE 两册的键是披露策略引用与客户合同，不是（产品/合同版本 + 时点 → 规则版本），所以它们是 VE 判断参数不是错放的规则正文。**适用场景**：客户服务规则六项里首发只做两侧都无承载、且 VE 已因之停在未决的两项；另四项进时放宽 CHECK 即可，要并 VE 通知策略册进 PC 才须新 ADR｜**部分停用**：其 Alternatives「允许显式空版本」条目中「客户服务规则没有对应的回退物——产品没有默认期限」这一句的前提已由 [ADR-0176](./0176-customer-service-rule-contract-tier-selection-and-inheritance.md) 消解——回退物＝同范围挂服务产品的版本的那一行；该条目其余否决理由与 Decision 三「子行至少一项、不允许显式空版本」对版本级继续成立，其余各条不变
```

合并后：

```text
Status: Accepted（2026-09-03，用户经 IDP 队列通道 3 授权本会话「有全部权限」自决并向各会话派工。裁决能力边界：读过票 [party-commercial-context-gaps/05](../../.scratch/party-commercial-context-gaps/issues/05-customer-service-rule-version-has-no-consuming-seam-into-visibility-exception.md) 全文（含 MCP-2 于 `877444a` 的取证与「待 owner 裁的三处」）、`party-commercial` `CONTEXT.md` 中「客户服务规则版本」词条、Rules 两句与 Boundaries 那一句、`visibility-exception` `CONTEXT.md` 索赔一节中期限与材料的硬句、`internal/partycommercial/domain/customer_service_rule.go`、`internal/visibilityexception/adapters/postgres` 的 `claim_eligibility.go` 与 `notification_policy.go` 头注、`migrations/party_commercial/0014` 头注；未重读 VE `CONTEXT.md` 披露与通知各节全文、`UC-VE-007` 与 `UC-VE-008` 全文、VE 编排代码——本记录因此只裁**正文归谁、首发进几项、VE 怎么采用**三件，不改任何一条 VE 不变量、不动 VE 的表与编排）<!-- 索引注记 -->｜**看键判归属**：一本册是不是某类正文，看它的键是不是那类正文的键——VE 两册的键是披露策略引用与客户合同，不是（产品/合同版本 + 时点 → 规则版本），所以它们是 VE 判断参数不是错放的规则正文。**适用场景**：客户服务规则六项里首发只做两侧都无承载、且 VE 已因之停在未决的两项；另四项进时放宽 CHECK 即可，要并 VE 通知策略册进 PC 才须新 ADR｜**部分停用**：其 Alternatives「允许显式空版本」条目中「客户服务规则没有对应的回退物——产品没有默认期限」这一句的前提已由 [ADR-0176](./0176-customer-service-rule-contract-tier-selection-and-inheritance.md) 消解——回退物＝同范围挂服务产品的版本的那一行；该条目其余否决理由与 Decision 三「子行至少一项、不允许显式空版本」对版本级继续成立，其余各条不变；承接线见 ADR-0176 Consequences 与本文件 Links 节前向指针
```

### ADR-0106: 等待受控补充是入账暂停——与人工复核、运营登记同组提交入账，续办由「新提交版本已形成」信封驱动；停用 ADR-0086 Context 对这一格的分侧

处理：来向两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（**部分停用**，2026-10-19：决定四「客户渠道的采信身份属 `PAR-INT-01` 接入契约，是实例半边」的适用场景由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一收窄为**租户既有渠道的适配**；首方族的采信身份由 ADR-0139 Decision 二定。）<!-- 索引注记 -->｜**部分停用**（2026-10-19）：决定四「客户渠道的采信身份属 `PAR-INT-01` 接入契约，是实例半边」已由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一 停用，**适用场景**收窄到租户既有渠道的适配；其余各条不变。**分格判据**：仍是 ADR-0029 / ADR-0094 的「看恢复动作」——客户补件不是本进程重试推得动的，且代码证实受控补充编排不铸信封、无生产调用方、等待态从不落库，旧信封重投即便新版本到达也推不动链。**适用场景**：`ResumePath` 四格自此只剩`等待内部续办`一格回滚重投；处置翻转与它的续办信封（新提交版本）必须同笔落地，入口机制半边随之接进装配、实例半边（`PAR-INT-01`）留空
```

合并后：

```text
Status: Accepted<!-- 索引注记 -->｜**部分停用**（2026-10-19）：决定四「客户渠道的采信身份属 `PAR-INT-01` 接入契约，是实例半边」已由 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 一停用，**适用场景**收窄为**租户既有渠道的适配**；首方族的采信身份由 ADR-0139 Decision 二定；其余各条不变。**分格判据**：仍是 ADR-0029 / ADR-0094 的「看恢复动作」——客户补件不是本进程重试推得动的，且代码证实受控补充编排不铸信封、无生产调用方、等待态从不落库，旧信封重投即便新版本到达也推不动链。**适用场景**：`ResumePath` 四格自此只剩`等待内部续办`一格回滚重投；处置翻转与它的续办信封（新提交版本）必须同笔落地，入口机制半边随之接进装配、实例半边（`PAR-INT-01`）留空
```

### ADR-0114: 末端派送任务由「对象凭`已交接`进入登记方声明为末端派送的段」触发——段服务动作在段成立时由控制事实登记方声明、不推导；触发是异步一拍的跨上下文内部执行器，派送要求三条输入按 ADR-0025 消费侧拉引用与时间窗、不拉地址本体、缺席即待形成

处理：来向两写合并

合并前（main `311adf934`）：

```text
Status: Accepted。决定二末句「执行器的生产入口（谁按拍调它）随第一条派送要求缝的实施票立」已由 [ADR-0216](./0216-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope-per-object.md) 回答（2026-10-19）：派发进程随交接信封逐对象调本执行器，不是租户节拍；本记录各条不变。决定三「接了线而所有者答「没有」时任务保持待形成」落在哪里由 [ADR-0230](./0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md) 补上（2026-10-21）：执行器答要求缺失同笔落派送待形成册；本记录各条不变。<!-- 索引注记 -->｜**触发判据**：班次到达经装载分配推不出对象到达（装载分配是意图不是事实，ADR-0004），对象级「到了」只有凭`已交接`进段一种表达；「尾程」由登记方声明为段服务动作（ADR-0096 同判据），未声明是答案。**适用场景**：段成立时声明三格封闭（场外揽收 / 节点间运输 / 末端派送）、之后不改、矛盾声明即拒；执行器异步一拍、一对象一任务、任务引用确定性铸出、调既有 `OpenDispatchTask`；三条派送要求端口各立票（tf/12–14），选拉不选携带因为触发源在 TF 内部没有请求可带，拉引用不拉地址本体守住 ADR-0075 那一半；执行器生产入口随第一条缝立｜**生产入口已答**（2026-10-19）：决定二末句「执行器的生产入口（谁按拍调它）随第一条派送要求缝的实施票立」由 [ADR-0216](./0216-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope-per-object.md) 回答——派发进程消费交接登记信封、逐对象发起，不是租户节拍；无条款停用，其余各条不变｜**待形成落册已补**（2026-10-21）：决定三「接了线而所有者答「没有」时任务保持待形成（`REQUIREMENT_MISSING`），也不填默认」由 [ADR-0230](./0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md) 补上落点——执行器答要求缺失时与这一拍同笔落「派送待形成」册（段、对象、入场依据、缺件、答复时刻），任务形成时了结，运营按册重问；仍不填默认。决定二的铸法不改，查在册沿替代链认前版任务；无条款停用，其余各条不变
```

合并后：

```text
Status: Accepted<!-- 索引注记 -->｜**触发判据**：班次到达经装载分配推不出对象到达（装载分配是意图不是事实，ADR-0004），对象级「到了」只有凭`已交接`进段一种表达；「尾程」由登记方声明为段服务动作（ADR-0096 同判据），未声明是答案。**适用场景**：段成立时声明三格封闭（场外揽收 / 节点间运输 / 末端派送）、之后不改、矛盾声明即拒；执行器异步一拍、一对象一任务、任务引用确定性铸出、调既有 `OpenDispatchTask`；三条派送要求端口各立票（tf/12–14），选拉不选携带因为触发源在 TF 内部没有请求可带，拉引用不拉地址本体守住 ADR-0075 那一半；执行器生产入口随第一条缝立｜**生产入口已答**（2026-10-19）：决定二末句「执行器的生产入口（谁按拍调它）随第一条派送要求缝的实施票立」由 [ADR-0216](./0216-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope-per-object.md) 回答——派发进程消费交接登记信封、逐对象发起，不是租户节拍；无条款停用，其余各条不变｜**待形成落册已补**（2026-10-21）：决定三「接了线而所有者答「没有」时任务保持待形成（`REQUIREMENT_MISSING`），也不填默认」由 [ADR-0230](./0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md) 补上落点——执行器答要求缺失时与这一拍同笔落「派送待形成」册（段、对象、入场依据、缺件、答复时刻），任务形成时了结，运营按册重问；仍不填默认。决定二的铸法不改，查在册沿替代链认前版任务；无条款停用，其余各条不变
```

### ADR-0122：接受前财务控制按策略正文的控制项逐项执行——`settlement-accounting` 的控制策略答复长成「控制项集合 + 共同通过条件 + 采用的控制策略版本」，控制种类选路、判断顺序定序、结算方式只作保存；费用范围取自已采用结算政策的适用范围；释放两本账各认领；多项结果怎么合起来看由 `parcel-shipment` 按共同通过条件折

处理：补指针

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-07，用户经 IDP 队列通道 1 授权「owner 授权自决」口径由本会话代裁票 [sa-preacceptance-policy-view/02](../../.scratch/sa-preacceptance-policy-view/issues/02-load-control-policy-reads-policy-content-items.md) 五问并实施。裁决能力边界：读过该票全文、票 01、[ADR-0115](./0115-pre-acceptance-financial-control-policy-content-is-a-row-per-control-and-no-control-stays-with-the-contract.md) 全文、`settlement-accounting` `CONTEXT.md` 词条「接受前财务控制结果」与 Rules 中关于接受前控制的四句、`internal/settlementaccounting/adapters/partycommercial/pre_acceptance_control_policy.go`、`application/apply_pre_acceptance_control.go`、`application/release_pre_acceptance_control.go`、`domain/pre_acceptance_control.go`、`domain/funds_freeze.go` 的 `Freeze` / `FindByRequest`、`internal/parcelshipment/adapters/settlementaccounting/pre_acceptance_control.go`、PC 侧 `domain/pre_acceptance_financial_control_policy.go`、`domain/reference_closure.go` 的 `ResolveCommercialClosure` / `AdoptedBasis`、`domain/settlement_policy.go` 的 `SettlementApplicability` / `SettlementSelector`、`cmd/parcel-dispatch/assemble.go` 的 SA 装配段；**未重读** `UC-SA-001` 全文、PS 接受编排读取控制结果的那一段与 SA 的 HTTP 读面。因此本记录只裁 SA 结果形状、执行顺序、费用范围来源、释放认领与 PS 消费适配器怎么折五件；PS 域对逐项结果的表达不在此，另立票）<!-- 索引注记 -->｜**分岔判据**：`pn-02-w03` 禁从结算方式推控制方式，故`要求`格的内容只能来自 ADR-0115 的正文点读口，结算方式退为结果上要保存的一格；CONTEXT 把「按共同通过条件形成接受判断」判给 PS，故 SA 逐项交回、折叠在 PS 消费适配器。**适用场景**：`0007` 说要求而正文未登记停 `CONTROL_POLICY_NOT_CONFIGURED` 不留派生过渡；「全部通过」之下第一处`业务限制`即停后续项、已执行项不回滚（账本对请求身份幂等）；失败处置与信用政策版本都不随 SA 答复走；PS 域逐项结果的表达归 sa-preacceptance-policy-view/03
```

合并后：

```text
Status: Accepted（2026-09-07，用户经 IDP 队列通道 1 授权「owner 授权自决」口径由本会话代裁票 [sa-preacceptance-policy-view/02](../../.scratch/sa-preacceptance-policy-view/issues/02-load-control-policy-reads-policy-content-items.md) 五问并实施。裁决能力边界：读过该票全文、票 01、[ADR-0115](./0115-pre-acceptance-financial-control-policy-content-is-a-row-per-control-and-no-control-stays-with-the-contract.md) 全文、`settlement-accounting` `CONTEXT.md` 词条「接受前财务控制结果」与 Rules 中关于接受前控制的四句、`internal/settlementaccounting/adapters/partycommercial/pre_acceptance_control_policy.go`、`application/apply_pre_acceptance_control.go`、`application/release_pre_acceptance_control.go`、`domain/pre_acceptance_control.go`、`domain/funds_freeze.go` 的 `Freeze` / `FindByRequest`、`internal/parcelshipment/adapters/settlementaccounting/pre_acceptance_control.go`、PC 侧 `domain/pre_acceptance_financial_control_policy.go`、`domain/reference_closure.go` 的 `ResolveCommercialClosure` / `AdoptedBasis`、`domain/settlement_policy.go` 的 `SettlementApplicability` / `SettlementSelector`、`cmd/parcel-dispatch/assemble.go` 的 SA 装配段；**未重读** `UC-SA-001` 全文、PS 接受编排读取控制结果的那一段与 SA 的 HTTP 读面。因此本记录只裁 SA 结果形状、执行顺序、费用范围来源、释放认领与 PS 消费适配器怎么折五件；PS 域对逐项结果的表达不在此，另立票）<!-- 索引注记 -->｜**分岔判据**：`pn-02-w03` 禁从结算方式推控制方式，故`要求`格的内容只能来自 ADR-0115 的正文点读口，结算方式退为结果上要保存的一格；CONTEXT 把「按共同通过条件形成接受判断」判给 PS，故 SA 逐项交回、折叠在 PS 消费适配器。**适用场景**：`0007` 说要求而正文未登记停 `CONTROL_POLICY_NOT_CONFIGURED` 不留派生过渡；「全部通过」之下第一处`业务限制`即停后续项、已执行项不回滚（账本对请求身份幂等）；失败处置与信用政策版本都不随 SA 答复走；PS 域逐项结果的表达归 sa-preacceptance-policy-view/03｜**部分停用**（指针 2026-10-21 补）：其决定四「所以 SA 只把每项结果与条件原样交出，折叠在 PS 的 `adapters/settlementaccounting`」里折叠的位置已由 [ADR-0125](./0125-parcel-shipment-financial-control-result-carries-per-item-results-and-folds-in-the-domain.md) 决定三停用——折叠从 PS→SA 适配器搬进 PS 领域，位置变、归属不变，仍是 PS 按共同通过条件折；本记录文末 owner 复核 2026-09-09 记录已写「字面不改，以本条与 ADR-0125 为准」，其余各条不变
```

### ADR-0125：`parcel-shipment` 的接受前财务控制采用结果长成「逐项控制项结果 + 共同通过条件 + 推导出的接受侧结论」——按共同通过条件折叠从消费适配器搬进领域，结论由构造期推导而不由适配器交入；释放按「有没有成立的项」发，不再按结论；失败处置与责任引用留给授权处置流程，读口与流程同票

处理：补指针

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-07，用户经 IDP 队列通道 1 授权「owner 自决」口径，由通道 3 代裁票 [sa-preacceptance-policy-view/03](../../.scratch/sa-preacceptance-policy-view/issues/03-parcel-shipment-expresses-per-item-control-results.md) 三问并实施。裁决能力边界：读过该票全文、[ADR-0122](./0122-pre-acceptance-control-executes-the-policy-content-items-and-parcel-shipment-folds-by-the-joint-pass-condition.md) 全文、[ADR-0047](./0047-terms-control-forms-credit-exposure-not-a-freeze.md)、PS CONTEXT 关于接受前财务控制的那一句、SA CONTEXT 关于组合控制的两句、`psdomain.FinancialControlResult` 与 `FinancialControlCheckFor`、`AdvanceFinancialControlJudgmentHandler`、`FormAcceptanceDecisionHandler` 的 `assembleChecks` / `releaseIfRejected`、`RejectShipmentRequestHandler` 与 `WithdrawShipmentRequestHandler` 的 `releaseFreeze`、PS→SA 适配器 `pre_acceptance_control.go` 全文与测试、`AcceptanceJudgments` 读写与迁移 `0005` / `0018`、SA 侧 `ApplyPreAcceptanceControlResult` 与 `ReleasePreAcceptanceControlHandler`（只读）、PC `PreAcceptanceFinancialControlPolicy` 与 `PreAcceptanceFinancialControlPolicyContentView`、SA→PC 适配器、UC-PS-001 校验组行与 `AT-PS-035`、BD-PS-003 简报、HTTP 读面 `query_acceptance_review_queue.go`；**未读** admin-web 前端。因此本记录只裁 PS 域结果的形状、折叠的位置、释放的触发、失败处置的去处与「未执行」分不分五件；授权处置流程本身另立票）<!-- 索引注记 -->｜**归属判据**：哪个组合算通过是 PS 的接受语言，与 `FinancialControlCheckFor` 同住领域层，适配器只翻译（ADR-0025）；ADR-0122 留的缺口正是「PS 域里装不下按哪个条件判」，并列新对象填不上它。**适用场景**：`ControlItemResult` 与 `JointPassCondition` 作 PS 自有封闭集镜像、集外不吸收；`HELD` / `CREDIT_EXPOSED` 降为「成立」的投影而不改词，重建门只校验所记结论与逐项一致（ADR-0028）；`OccupationFormed` 修掉 `CREDIT_EXPOSED` 从不释放与 `RESTRICTED` 前成立项不释放两处孤儿；`RESTRICTED` 一律拒绝是票 04 落地前的过渡状态，不是产品口径；「未执行」不记，由已采用解析可算
```

合并后：

```text
Status: Accepted（2026-09-07，用户经 IDP 队列通道 1 授权「owner 自决」口径，由通道 3 代裁票 [sa-preacceptance-policy-view/03](../../.scratch/sa-preacceptance-policy-view/issues/03-parcel-shipment-expresses-per-item-control-results.md) 三问并实施。裁决能力边界：读过该票全文、[ADR-0122](./0122-pre-acceptance-control-executes-the-policy-content-items-and-parcel-shipment-folds-by-the-joint-pass-condition.md) 全文、[ADR-0047](./0047-terms-control-forms-credit-exposure-not-a-freeze.md)、PS CONTEXT 关于接受前财务控制的那一句、SA CONTEXT 关于组合控制的两句、`psdomain.FinancialControlResult` 与 `FinancialControlCheckFor`、`AdvanceFinancialControlJudgmentHandler`、`FormAcceptanceDecisionHandler` 的 `assembleChecks` / `releaseIfRejected`、`RejectShipmentRequestHandler` 与 `WithdrawShipmentRequestHandler` 的 `releaseFreeze`、PS→SA 适配器 `pre_acceptance_control.go` 全文与测试、`AcceptanceJudgments` 读写与迁移 `0005` / `0018`、SA 侧 `ApplyPreAcceptanceControlResult` 与 `ReleasePreAcceptanceControlHandler`（只读）、PC `PreAcceptanceFinancialControlPolicy` 与 `PreAcceptanceFinancialControlPolicyContentView`、SA→PC 适配器、UC-PS-001 校验组行与 `AT-PS-035`、BD-PS-003 简报、HTTP 读面 `query_acceptance_review_queue.go`；**未读** admin-web 前端。因此本记录只裁 PS 域结果的形状、折叠的位置、释放的触发、失败处置的去处与「未执行」分不分五件；授权处置流程本身另立票）<!-- 索引注记 -->｜**归属判据**：哪个组合算通过是 PS 的接受语言，与 `FinancialControlCheckFor` 同住领域层，适配器只翻译（ADR-0025）；ADR-0122 留的缺口正是「PS 域里装不下按哪个条件判」，并列新对象填不上它。**适用场景**：`ControlItemResult` 与 `JointPassCondition` 作 PS 自有封闭集镜像、集外不吸收；`HELD` / `CREDIT_EXPOSED` 降为「成立」的投影而不改词，重建门只校验所记结论与逐项一致（ADR-0028）；`OccupationFormed` 修掉 `CREDIT_EXPOSED` 从不释放与 `RESTRICTED` 前成立项不释放两处孤儿；`RESTRICTED` 一律拒绝是票 04 落地前的过渡状态，不是产品口径；「未执行」不记，由已采用解析可算｜**部分停用**（指针 2026-10-21 补）：其 Consequences「**过渡状态明记**：票 04 落地前 `RESTRICTED` 仍译确定性不通过 → 拒绝」与决定一受限项只带自己原因的形，已由 [ADR-0132](./0132-authorized-disposition-decides-the-destination-of-a-restricted-request-as-its-own-wait-state-and-never-passes.md) 停用（2026-09-10，票 sa-preacceptance-policy-view/04 落地）——`RESTRICTED` 不再一律拒绝：任一 `REJECT` → 未通过照旧，全部 `AUTHORIZED_DISPOSITION` → 无法判定 + `等待授权处置`；受限的控制项结果扩两格采用引用（失败处置与责任引用），以 ADR-0132 决定三为准。上面「`RESTRICTED` 一律拒绝是票 04 落地前的过渡状态」一句自此只是历史；本记录文末「过渡态解除 2026-09-10」一条已记，其余各条不变
```

### ADR-0126：商业发布的内容摘要由服务端按册规范化算出（规范化版本 `PCC-1`，摘要串自带版本）；受控批文仍声明摘要但必须与算出的相等，不等即`未受理`带两串；批准是操作者动作，经一版一行的待批准载体推进，审批职责规则未登记即批准门答未配置；预览与发布走同一份规范化与构造门

处理：头部停用行并入注记

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-08，用户经 IDP 队列通道 1 派单 `task-cc7313e8` 授权 owner「自决口径：硬句一字不改、每个决定写理由、拿不准或跨上下文归属的点单列越权风险点」，据此对票 [admin-write-faces/08](../../.scratch/admin-write-faces/issues/08-publication-form-path-common-half-server-side-digest-and-envelope-approval.md)「要裁的」三问一次答完。裁决能力边界：读过伞票 [07](../../.scratch/admin-write-faces/issues/07-commercial-publication-operator-main-paths-per-register.md) 硬句与票 08 全文、[ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)、[ADR-0101](./0101-operator-facing-registration-payload-shape-is-product-defined.md)、[ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)、[ADR-0031](./0031-owned-repository-write-outcome-is-a-closed-algebra-not-an-error.md)、[ADR-0085](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md) 全文，PC `CONTEXT.md`「商业版本」词条与「商业规则与政策版本」生命周期，`internal/partycommercial/domain` 的 `CommercialVersionSpec` / `NewCommercialContentDigest` / `CommercialObjectKind` / `NewCreditPolicy` / `CreditLimit`，`application/publish_commercial_authority.go` 全文，`cmd/parcel-commercial/translate.go` 的 `publicationItemDocument`，`adapters/http/register_commercial_publication.go`，parcel-pricing 的 `fingerprint.go`（`PPC-5`、`hashCanonical`）与 `preview_reference_series.go` / `reference_series_payload.go`，parcel-shipment 的 `payload_canonicalization.go`（`PSC-1:<sha256>` 摘要串形）；**未重读**十类正文各自的领域构造门（只读了信用政策那一类）、`scripts/demo-seeds` 各批文正文、`internal/accessidentity` 全部——本记录因此只裁**摘要由谁算与摘要串的形、受控批文那一半的对账门、待批准载体的形与批准门、预览口的路径**，不改任何一册正文的形状，不裁其余九册各自的规范化文档（那是各子票逐册接时的事，同一版本号下加册不换号，见决定一））<!-- 索引注记 -->｜**摘要归属判据**：能算出对的那一个的只有持有正文构造门的一侧，调用方声明的串没有任何东西能对——摘要只在领域一处算，串里带版本（先例 `PSC-1:<sha256>`）不给版本册加列。**适用场景**：首例只接信用政策，其余九册各子票接进同一个 `PCC-1` 不换号；`NOT_ACCEPTED`（输入内部不自洽，改批文）与 `CONTENT_CONFLICT`（与册上那一版不同，换版本号）分格；载体一版一行、`待批准`期间修订就地替换是本上下文唯一一处就地更新（权威记录仍只插不改）；预览口 `-previews` 不做发布口的 dry-run 参数｜**部分停用**：其决定二「没接的册与正文缺席的版本没有可比对象，按今天的样子登记声明的串」一句在客户合同一册已由 [ADR-0178](./0178-customer-contract-versions-carry-their-body-and-contract-shells-bind-service-rule-bodies.md) 决定一停用。**适用场景**：只限客户合同这一册——货主客户账户只在正文里，只有壳的合同说不出属于哪一户；其余各册的壳照旧可以单独发布，其余各条不变
Partially suspended by: [ADR-0178](./0178-customer-contract-versions-carry-their-body-and-contract-shells-bind-service-rule-bodies.md) 决定一（决定二「没接的册与正文缺席的版本没有可比对象，按今天的样子登记声明的串」一句，仅限客户合同一册：该册不再收只有壳的版本；其余各册与其余各条不变，本记录正文不改写）
```

合并后：

```text
Status: Accepted（2026-09-08，用户经 IDP 队列通道 1 派单 `task-cc7313e8` 授权 owner「自决口径：硬句一字不改、每个决定写理由、拿不准或跨上下文归属的点单列越权风险点」，据此对票 [admin-write-faces/08](../../.scratch/admin-write-faces/issues/08-publication-form-path-common-half-server-side-digest-and-envelope-approval.md)「要裁的」三问一次答完。裁决能力边界：读过伞票 [07](../../.scratch/admin-write-faces/issues/07-commercial-publication-operator-main-paths-per-register.md) 硬句与票 08 全文、[ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)、[ADR-0101](./0101-operator-facing-registration-payload-shape-is-product-defined.md)、[ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)、[ADR-0031](./0031-owned-repository-write-outcome-is-a-closed-algebra-not-an-error.md)、[ADR-0085](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md) 全文，PC `CONTEXT.md`「商业版本」词条与「商业规则与政策版本」生命周期，`internal/partycommercial/domain` 的 `CommercialVersionSpec` / `NewCommercialContentDigest` / `CommercialObjectKind` / `NewCreditPolicy` / `CreditLimit`，`application/publish_commercial_authority.go` 全文，`cmd/parcel-commercial/translate.go` 的 `publicationItemDocument`，`adapters/http/register_commercial_publication.go`，parcel-pricing 的 `fingerprint.go`（`PPC-5`、`hashCanonical`）与 `preview_reference_series.go` / `reference_series_payload.go`，parcel-shipment 的 `payload_canonicalization.go`（`PSC-1:<sha256>` 摘要串形）；**未重读**十类正文各自的领域构造门（只读了信用政策那一类）、`scripts/demo-seeds` 各批文正文、`internal/accessidentity` 全部——本记录因此只裁**摘要由谁算与摘要串的形、受控批文那一半的对账门、待批准载体的形与批准门、预览口的路径**，不改任何一册正文的形状，不裁其余九册各自的规范化文档（那是各子票逐册接时的事，同一版本号下加册不换号，见决定一））<!-- 索引注记 -->｜**摘要归属判据**：能算出对的那一个的只有持有正文构造门的一侧，调用方声明的串没有任何东西能对——摘要只在领域一处算，串里带版本（先例 `PSC-1:<sha256>`）不给版本册加列。**适用场景**：首例只接信用政策，其余九册各子票接进同一个 `PCC-1` 不换号；`NOT_ACCEPTED`（输入内部不自洽，改批文）与 `CONTENT_CONFLICT`（与册上那一版不同，换版本号）分格；载体一版一行、`待批准`期间修订就地替换是本上下文唯一一处就地更新（权威记录仍只插不改）；预览口 `-previews` 不做发布口的 dry-run 参数｜**部分停用**：其决定二「没接的册与正文缺席的版本没有可比对象，按今天的样子登记声明的串」一句在客户合同一册已由 [ADR-0178](./0178-customer-contract-versions-carry-their-body-and-contract-shells-bind-service-rule-bodies.md) 决定一停用。**适用场景**：只限客户合同这一册——货主客户账户只在正文里，只有壳的合同说不出属于哪一户，该册不再收只有壳的版本；其余各册的壳照旧可以单独发布，其余各条不变，本记录正文不改写
```

### ADR-0127：信用依据随商业闭包解析交出——闭包解析加 `resolveCreditPolicyBasis` 一步（镜像 `resolveSettlementPolicyBasis`），`ResolveCreditPolicy` 作（法人 × 权限等级 × 费用类型 × 时点）四维选择器，`CreditBasis` 随 `Resolution` / 闭包交出并入快照；键上新增 `CreditSelector`（等级 × 费用类型）；`settlement-accounting` 账期分支的授信额度从闭包交出的信用依据取，不从登记状况事实取

处理：补指针

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-08，MCP-1 代裁**甲**——owner 授权自决口径，裁决原文在票 [wiring-baseline-remainder/03](../../.scratch/wiring-baseline-remainder/issues/03-pc-credit-basis-is-never-asked-for-the-pc-to-sa-seam-does-not-exist.md) Comments「11:0x · MCP-1 代裁」一条；本记录由 MCP-3 落文并实施。落文时读过：该票全文（含 MCP-6 02:3x 取证与两条路）、ADR-0044 / ADR-0080 / ADR-0115 / ADR-0025 / ADR-0028、`party-commercial` `CONTEXT.md` 信用政策那句、`settlement-accounting` `CONTEXT.md` 授信额度与接受前控制那几句、`internal/partycommercial/domain` 的 `commercial_resolution.go` / `reference_closure.go` / `credit_policy.go` / `credit_limit.go` / `commercial_registry.go` / `resolution_rehydration.go`、`adapters/postgres` 的 `commercial_publication.go` / `commercial_resolution.go` / `credit_policy.go`、`migrations/party_commercial/0020_credit_policy.sql`、`internal/settlementaccounting` 的 `application/apply_pre_acceptance_control.go` / `domain/credit_exposure.go` / `adapters/postgres/operational_position.go` / `adapters/partycommercial/pre_acceptance_control_policy.go`、`internal/parcelshipment/adapters/partycommercial/commercial_resolution_keys.go`（只读，PS 地盘）。未重读 UC-SA-002 全文与 PS 接受编排；本记录因此只裁**信用政策在哪一层选、选出的依据怎么交出、SA 账期分支从哪里取额度**三件，不改 PS 任何一格、不动 `0020`）<!-- 索引注记 -->｜**选择层判据**：一版一行之下多候选只能来自同一范围内多个信用政策对象，与结算政策同形，同一闭包里三个政策类对象用同一种类别专属选择器解，乙（版本壳 + `covers` 校验）会让两套解析语义并存、逼租户把法人 × 等级拆成范围。**适用场景**：请求信用依据的键必带 `CreditSelector`、其余必缺；`CreditBasis` 整份进闭包快照不回册重读；SA 新端口 `CreditBasisView` 三格照 ADR-0054，比例额度基数未裁停在 `CREDIT_RATIO_BASE_UNDECIDED`；`Deps.CreditBasis` 为 nil 沿旧路是三步法 expand 段，contract 随 PS 夹具补替身那笔收；PS 登记面不承载信用二维那格归 PS 另立票
```

合并后：

```text
Status: Accepted（2026-09-08，MCP-1 代裁**甲**——owner 授权自决口径，裁决原文在票 [wiring-baseline-remainder/03](../../.scratch/wiring-baseline-remainder/issues/03-pc-credit-basis-is-never-asked-for-the-pc-to-sa-seam-does-not-exist.md) Comments「11:0x · MCP-1 代裁」一条；本记录由 MCP-3 落文并实施。落文时读过：该票全文（含 MCP-6 02:3x 取证与两条路）、ADR-0044 / ADR-0080 / ADR-0115 / ADR-0025 / ADR-0028、`party-commercial` `CONTEXT.md` 信用政策那句、`settlement-accounting` `CONTEXT.md` 授信额度与接受前控制那几句、`internal/partycommercial/domain` 的 `commercial_resolution.go` / `reference_closure.go` / `credit_policy.go` / `credit_limit.go` / `commercial_registry.go` / `resolution_rehydration.go`、`adapters/postgres` 的 `commercial_publication.go` / `commercial_resolution.go` / `credit_policy.go`、`migrations/party_commercial/0020_credit_policy.sql`、`internal/settlementaccounting` 的 `application/apply_pre_acceptance_control.go` / `domain/credit_exposure.go` / `adapters/postgres/operational_position.go` / `adapters/partycommercial/pre_acceptance_control_policy.go`、`internal/parcelshipment/adapters/partycommercial/commercial_resolution_keys.go`（只读，PS 地盘）。未重读 UC-SA-002 全文与 PS 接受编排；本记录因此只裁**信用政策在哪一层选、选出的依据怎么交出、SA 账期分支从哪里取额度**三件，不改 PS 任何一格、不动 `0020`。另：决定四使 [ADR-0047](./0047-terms-control-forms-credit-exposure-not-a-freeze.md) 决定二括注里「当前额度」改取闭包交出的信用依据，被停原句见 ADR-0047 的 Status 行（指针 2026-10-21 补））<!-- 索引注记 -->｜**选择层判据**：一版一行之下多候选只能来自同一范围内多个信用政策对象，与结算政策同形，同一闭包里三个政策类对象用同一种类别专属选择器解，乙（版本壳 + `covers` 校验）会让两套解析语义并存、逼租户把法人 × 等级拆成范围。**适用场景**：请求信用依据的键必带 `CreditSelector`、其余必缺；`CreditBasis` 整份进闭包快照不回册重读；SA 新端口 `CreditBasisView` 三格照 ADR-0054，比例额度基数未裁停在 `CREDIT_RATIO_BASE_UNDECIDED`；`Deps.CreditBasis` 为 nil 沿旧路是三步法 expand 段，contract 随 PS 夹具补替身那笔收；PS 登记面不承载信用二维那格归 PS 另立票｜**部分停用**（指针 2026-10-21 补）：其决定四「遇到比例额度停在新增的`待判断`格 `CREDIT_RATIO_BASE_UNDECIDED`，不折成金额、不默认」一句已由 [ADR-0129](./0129-credit-ratio-base-is-declared-on-the-credit-policy-content.md) 决定三、四收窄——比例额度的基数由信用政策正文声明（封闭集 `CreditRatioBase`），SA 按声明的基数取自己的数折成金额，基数尚无事实时停在 `CREDIT_RATIO_BASE_NOT_ESTABLISHED`；`CREDIT_RATIO_BASE_UNDECIDED` 只留给重建门读回的、未声明基数的存量。上面「比例额度基数未裁停在 `CREDIT_RATIO_BASE_UNDECIDED`」一句自此只对那批存量成立，其余各条不变
```

### ADR-0139：产品自有的客户接入渠道（首方 API）是产品自有的接入渠道族，属机制半边——信任锚、凭据形态、渠道登记册结构与来源信封铸造由产品定义，不等 `PAR-INT-01`；`PAR-INT-01` 收窄为「某租户的客户实际启用哪条渠道」与「租户既有渠道的适配」；ADR-0072 Decision 二余下那一半与 ADR-0085 补记「两族各自要过」的客户半据此收窄

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19 **修订后接受**：用户在 Claude Code 会话里答「接受」，按会话复核建议修订。修订要点：凭据机制与校验顺序照集成客户端族（[ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定三），首方客户族、操作者族、集成客户端族三族分立（Decision 二、三）；首方族自带三件部署参数；客户渠道册行形照集成客户端册，生效区间与撤销挂在授予上；答复扩为按恢复动作分的各格并命名错误码（Decision 四）；ADR-0055 Decision 五两项照 ADR-0149 决定四延到客户面各口（新增 Decision 八）；隔离形态照 [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 退场（Decision 六、七）；「既有渠道适配」按 [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 收窄为按某客户自有契约的适配；部分停用清单增 ADR-0100、ADR-0101、ADR-0106；越权风险点逐条按复核建议处置。本记录凡言「机制半边」，按 ADR-0146 读作产品交付（机制与产品策略）；「实例半边」读作租户取值。此前 Proposed——**草案**，2026-09-16；**草案修订**同日，用户经 IDP 队列通道 1 令「按建议处置」，据通道 1 的复核修订——改写 Decision 二凭据形态一条的校验顺序（签名与声明校验前置于 `Minter`、不读册，`Minter` 内只核绑定；原句「既有顺序不变」与 Decision 四的三格互斥，会让登记册成员被未认证调用方枚举），Context 补候选租户既有 API 渠道证据（E-02）的归族，不再停用 ADR-0055 Alternatives 那一条、ADR-0085 客户半改为收窄，`/claims` 改归命令面，Decision 五补「接入请求标识」不进摘要，Alternatives 补 API Key 一条的市场取舍，风险点去掉预判并补齐；修订时另读过 `.scratch/syn-wall-door-audit/issues/01` 的「重启范围」节、`.scratch/tenant-implementation-01/instance-register-draft.md` 的 `PAR-INT-01` 行与 `implementation-week-decisions-and-customer-pack.md` 的 D-08 行、`internal/parcelshipment/adapters/http/cancel_parcel.go` 与 `internal/visibilityexception/adapters/http/receive_claim.go` 的端点构造函数注释。用户经 IDP 队列通道 5 先质疑「我们这是一个我们公司的软件产品，有没有客户都需要 internal/public api 的吧，原来的规范老是等真实的客户，不适合一个软件公司开发产品的吧」，再令「起草」；本记录据此起草，**接受与否归用户**，接受前不是依据、不停用任何记录的任何条款。裁决能力边界：读过 [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)、[ADR-0072](./0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)、[ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)、[ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md) 全文，[ADR-0085](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md) 的 Status 行与 Decision 二同日补记，[ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) Decision 一、三、四、六，[ADR-0018](./0018-product-clients-share-release-boundary-under-apps.md) / [ADR-0019](./0019-product-ships-tenant-facing-operator-clients.md) 谱系中关于 `PAR-INT-01` 的句子，`internal/accessidentity` 全部生产文件，`cmd/parcel-api` 的 `assembleBusinessEndpoints` 与 `internal/platform/httpapi` 路由，[参数登记册](../product/PILOT-PARAMETER-REGISTER.md)的 `PAR-INT-01` 至 `PAR-INT-07` 行，[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)对机制半边的定义句，[parcel-shipment CONTEXT](../domain/parcel-shipment/CONTEXT.md) 邻接上下文里关于共享身份认证与授权技术能力的那句，[UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md) 输入语义契约的来源信封行，以及 [`docs/api` 的 Go + chi + Scalar 方案稿](../api/Go_Chi_Scalar_API_Documentation_Final_V1.0.md)全文；未重读 [ADR-0003](./0003-group-tenant-legal-entity-customer-account.md) 全文、`pilotgovernance` 上下文与 `PAR-GOV-03..07` 相关记录及代码、UC-PS-001 其余各节——本记录因此只裁**首方客户渠道的归属与准入结构**，不裁契约正文、不裁 `PAR-GOV-03..07` 与 ADR-0055 Decision 五两项对客户业务面还拦不拦、不裁任何登记册的领域不变量）<!-- 索引注记 -->｜**已接受**（2026-10-19，用户在 Claude Code 会话里答「接受」，按会话复核建议修订后接受）；接受时部分停用 ADR-0072、0085、0100、0101、0106 各一句，适用场景收窄到租户既有渠道的适配。**归族判据**：同 ADR-0100——一族身份的证据由谁出，看它的三件（信任锚、凭据形态、绑定模型）取决于谁；`PAR-INT-01` 最低证据列写的是客户当前主用渠道的「现行流程」，那是**租户既有渠道**的属性，「三形各异、替它拟表就是替租户拟样子」只对既有渠道的适配成立，产品自己的渠道只有一形且这一形是产品自己选的。**校验顺序判据**（草案修订 2026-09-16 加）：令牌的签名与声明校验前置于 `Minter`、不读册，`Minter` 内只核绑定——JWT 校验不依赖行上秘密，`mint.go` 「顺序不能反」的前提对它不成立；反过来做会让 `401`/`403` 两格把登记册成员泄给未认证的调用方。**适用场景**：客户业务命令面（提交、撤回、取消、补充、资料修订、索赔提交 `/claims`）与客户查阅面（`/customer-tracking-view`）的真 Intake，答复三格同 ADR-0100（发行方未配置 / 令牌无效 / 不在册、已撤销、绑定不符），二三格错误码归实施票不可留白；候选租户既有 API 渠道（apikey 头，证据 E-02）属**既有渠道适配族**，与首方族并列、先后归 owner，票 `syn-wall-door-audit/01` S1 接受时改指；租户既有渠道（标准文件、门户、客户自有契约的 API）的适配照旧等 `PAR-INT-01`；`PAR-GOV-03..07` 与 ADR-0055 Decision 五两项对首方 Intake 怎么算不在本记录内；契约形状照 ADR-0022 一字不改，「接入请求标识」是信封元数据不进摘要，契约生产方式、URL 版本与内外拆分另立记录。接受时收窄 ADR-0072 Decision 二余下那一半与 ADR-0085 补记「两族各自要过」的客户半（ADR-0055 不动），「不得互相顶替」由受众与信封分型变成结构；越权风险点（含受控引用形态、第二种凭据形态、1:N 绑定、与既有渠道适配的先后）归 owner 复核
```

合并后：

```text
Status: Accepted（2026-10-19 **修订后接受**：用户在 Claude Code 会话里答「接受」，按会话复核建议修订。修订要点：凭据机制与校验顺序照集成客户端族（[ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定三），首方客户族、操作者族、集成客户端族三族分立（Decision 二、三）；首方族自带三件部署参数；客户渠道册行形照集成客户端册，生效区间与撤销挂在授予上；答复扩为按恢复动作分的各格并命名错误码（Decision 四）；ADR-0055 Decision 五两项照 ADR-0149 决定四延到客户面各口（新增 Decision 八）；隔离形态照 [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 退场（Decision 六、七）；「既有渠道适配」按 [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 收窄为按某客户自有契约的适配；部分停用清单增 ADR-0100、ADR-0101、ADR-0106；越权风险点逐条按复核建议处置。本记录凡言「机制半边」，按 ADR-0146 读作产品交付（机制与产品策略）；「实例半边」读作租户取值。此前 Proposed——**草案**，2026-09-16；**草案修订**同日，用户经 IDP 队列通道 1 令「按建议处置」，据通道 1 的复核修订——改写 Decision 二凭据形态一条的校验顺序（签名与声明校验前置于 `Minter`、不读册，`Minter` 内只核绑定；原句「既有顺序不变」与 Decision 四的三格互斥，会让登记册成员被未认证调用方枚举），Context 补候选租户既有 API 渠道证据（E-02）的归族，不再停用 ADR-0055 Alternatives 那一条、ADR-0085 客户半改为收窄，`/claims` 改归命令面，Decision 五补「接入请求标识」不进摘要，Alternatives 补 API Key 一条的市场取舍，风险点去掉预判并补齐；修订时另读过 `.scratch/syn-wall-door-audit/issues/01` 的「重启范围」节、`.scratch/tenant-implementation-01/instance-register-draft.md` 的 `PAR-INT-01` 行与 `implementation-week-decisions-and-customer-pack.md` 的 D-08 行、`internal/parcelshipment/adapters/http/cancel_parcel.go` 与 `internal/visibilityexception/adapters/http/receive_claim.go` 的端点构造函数注释。用户经 IDP 队列通道 5 先质疑「我们这是一个我们公司的软件产品，有没有客户都需要 internal/public api 的吧，原来的规范老是等真实的客户，不适合一个软件公司开发产品的吧」，再令「起草」；本记录据此起草，**接受与否归用户**，接受前不是依据、不停用任何记录的任何条款。裁决能力边界：读过 [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)、[ADR-0072](./0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md)、[ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)、[ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md) 全文，[ADR-0085](./0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md) 的 Status 行与 Decision 二同日补记，[ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) Decision 一、三、四、六，[ADR-0018](./0018-product-clients-share-release-boundary-under-apps.md) / [ADR-0019](./0019-product-ships-tenant-facing-operator-clients.md) 谱系中关于 `PAR-INT-01` 的句子，`internal/accessidentity` 全部生产文件，`cmd/parcel-api` 的 `assembleBusinessEndpoints` 与 `internal/platform/httpapi` 路由，[参数登记册](../product/PILOT-PARAMETER-REGISTER.md)的 `PAR-INT-01` 至 `PAR-INT-07` 行，[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)对机制半边的定义句，[parcel-shipment CONTEXT](../domain/parcel-shipment/CONTEXT.md) 邻接上下文里关于共享身份认证与授权技术能力的那句，[UC-PS-001](../application/parcel-shipment/UC-PS-001-SUBMIT-SHIPMENT-REQUEST.md) 输入语义契约的来源信封行，以及 [`docs/api` 的 Go + chi + Scalar 方案稿](../api/Go_Chi_Scalar_API_Documentation_Final_V1.0.md)全文；未重读 [ADR-0003](./0003-group-tenant-legal-entity-customer-account.md) 全文、`pilotgovernance` 上下文与 `PAR-GOV-03..07` 相关记录及代码、UC-PS-001 其余各节——本记录因此只裁**首方客户渠道的归属与准入结构**，不裁契约正文、不裁 `PAR-GOV-03..07` 与 ADR-0055 Decision 五两项对客户业务面还拦不拦、不裁任何登记册的领域不变量。接受时部分停用 ADR-0072、0085、0100、0101、0106 各一句，适用场景收窄到租户既有渠道的适配：收窄 ADR-0072 Decision 二余下那一半与 ADR-0085 补记「两族各自要过」的客户半（ADR-0055 不动），「不得互相顶替」由受众与信封分型变成结构。）<!-- 索引注记 -->｜**归族判据**：同 ADR-0100——一族身份的证据由谁出，看它的三件（信任锚、凭据形态、绑定模型）取决于谁；`PAR-INT-01` 最低证据列写的是客户当前主用渠道的「现行流程」，那是**租户既有渠道**的属性，「三形各异、替它拟表就是替租户拟样子」只对既有渠道的适配成立，产品自己的渠道只有一形且这一形是产品自己选的。**校验顺序判据**（草案修订 2026-09-16 加）：令牌的签名与声明校验前置于 `Minter`、不读册，`Minter` 内只核绑定——JWT 校验不依赖行上秘密，`mint.go` 「顺序不能反」的前提对它不成立；反过来做会让 `401`/`403` 两格把登记册成员泄给未认证的调用方。**适用场景**：客户业务命令面（提交、撤回、取消、补充、资料修订、索赔提交 `/claims`）与客户查阅面（`/customer-tracking-view`）的真 Intake，答复三格同 ADR-0100（发行方未配置 / 令牌无效 / 不在册、已撤销、绑定不符），二三格错误码归实施票不可留白；候选租户既有 API 渠道（apikey 头，证据 E-02）属**既有渠道适配族**，与首方族并列、先后归 owner，票 `syn-wall-door-audit/01` S1 接受时改指；租户既有渠道（标准文件、门户、客户自有契约的 API）的适配照旧等 `PAR-INT-01`；`PAR-GOV-03..07` 与 ADR-0055 Decision 五两项对首方 Intake 怎么算不在本记录内；契约形状照 ADR-0022 一字不改，「接入请求标识」是信封元数据不进摘要，契约生产方式、URL 版本与内外拆分另立记录。越权风险点（含受控引用形态、第二种凭据形态、1:N 绑定、与既有渠道适配的先后）归 owner 复核
```

### ADR-0140：首方 API 的机器可读契约从端点描述符生成、生成物入库并由 CI 校验一致——描述符随处理器住在各上下文 `adapters/http`，OpenAPI 文档是发布形态不是事实源；首方与集成 API 带 URL 主版本 `/v1`，操作者 API 不带；4xx/5xx 一律 RFC 9457 问题详情承载稳定错误码；兼容性以已发布的不可变契约为基线由破坏性变更检查把门；弃用走 `Deprecation` / `Sunset` 头，窗口长度是产品策略

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19 **修订后接受**：用户在 Claude Code 会话里答「接受」，按会话复核建议修订。修订要点：契约按端点表上静态声明的族分拣成首方、集成、操作者三份（Decision 一）；`/v1` 延到集成族（Decision 二）；`detail` 只随「改请求才会好」的 4xx 在场，新增 `429` 一格并据此部分停用 [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md) 三类表 4xx 行（限首方与集成两族的限流答复，Decision 三）；破坏判据以锁定版本的工具规则为准（Decision 五）；弃用窗口归产品策略（Decision 六）；文档匿名可读、另有集成契约入口（Decision 七）；不另开监听端口（Decision 八）；越权风险点逐条按复核建议处置。此前 Proposed——**草案**，2026-09-16。用户经 IDP 队列通道 1 先问「这个方式是我们内外 API 的科学、专业、先进的解决方案吗」，通道 1 复核后答「身份与准入层成立；作为完整方案缺六件，第一件是契约生产与版本策略」，用户令「按建议继续」；本记录据此起草，**接受与否归用户**，接受前不是依据。它填的是 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 五显式留白的那一格——「契约怎么生产、URL 主版本、public 与 internal 契约要不要拆、文档门户放哪个入口，本记录不定」。裁决能力边界：读过 [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)、[ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)、[ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)、ADR-0139 全文，[`docs/api` 的 Go + chi + Scalar 方案稿](../api/Go_Chi_Scalar_API_Documentation_Final_V1.0.md)全文，`internal/platform/httpapi/router.go`，`internal/parcelshipment/adapters/http/submit_shipment_request.go` 与 `cmd/parcel-api` 的 `assembleBusinessEndpoints`，[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)对机制半边清点生成物的 CI 纪律那一句，[docs/README](../README.md) 对该方案稿的登记；未逐一重读其余上下文的 `adapters/http`、未核对 OpenAPI 3.1 在本仓将锁定的工具版本上的实际支持——本记录因此只裁**契约的事实源、生成与校验纪律、版本与弃用的机制、错误响应的媒体类型**，不裁任何端点的字段、不裁文档的阅读策略、不裁弃用窗口的长度） **补充**（2026-10-19）：首方提交答复逐件带回包裹号（`parcels`）、首方契约对客户追踪视图承诺的查询标识是这个号，由 [ADR-0223](./0223-the-first-party-submission-answer-names-each-parcel-and-the-tracking-view-locates-by-it.md) 定——Decision 八「不在本记录定任何端点的字段」留给各上下文 owner 的一处落点；本记录各条不变。<!-- 索引注记 -->｜**已接受**（2026-10-19，用户在 Claude Code 会话里答「接受」，按会话复核建议修订后接受）；Decision 三部分停用 ADR-0022 三类表 4xx 行对 `429` 的适用（限首方与集成两族的限流答复）。填 ADR-0139 Decision 五留白的四问。**事实源判据**：装配表上的处理器全是手写的、已按 ADR-0022 各自持有 `outcome` 词表、响应形状与 405 自守并有测试钉住，契约先行 + `oapi-codegen` 胶水会让同一 operation 迁移期有两个事实源、且把 405 挪到路由层——所以描述符跟处理器住、文档从描述符生成、CI 重生成比对（同机制半边清点的纪律）。**族分拣判据**：端点在装配点挂哪一族 Intake 决定它进首方还是操作者契约，不靠 `x-audience` 标签。**适用场景**：首方 API `/v1` 前缀在装配点按族加、操作者 API 不带（与客户端同版本发布）；问题体换 `application/problem+json`（`type`/`title`/`status`/`code`/`requestId`，不带 `detail`——`UC-PS-001` 泄露约束），写问题体的辅助收进 `httpapi`、错误码仍归各处理器包；不加契约校验中间件（ADR-0055 Decision 一「第二处权威」）；`outcome` 新增取值不算破坏，条件是文档写明未识别 `outcome` 交人；弃用 `Deprecation`（RFC 9745）+ `Sunset`（RFC 8594），窗口长度登记册增行、未登记不弃用；文档门户自托管 Scalar 由 `parcel-api` 提供，阅读策略归 owner；OpenAPI 3.1 工具链核实、描述符 Go 形状、SDK、文档端口、各 owner 逐端点确认描述符列为风险点
```

合并后：

```text
Status: Accepted（2026-10-19 **修订后接受**：用户在 Claude Code 会话里答「接受」，按会话复核建议修订。修订要点：契约按端点表上静态声明的族分拣成首方、集成、操作者三份（Decision 一）；`/v1` 延到集成族（Decision 二）；`detail` 只随「改请求才会好」的 4xx 在场，新增 `429` 一格并据此部分停用 [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md) 三类表 4xx 行（限首方与集成两族的限流答复，Decision 三）；破坏判据以锁定版本的工具规则为准（Decision 五）；弃用窗口归产品策略（Decision 六）；文档匿名可读、另有集成契约入口（Decision 七）；不另开监听端口（Decision 八）；越权风险点逐条按复核建议处置。此前 Proposed——**草案**，2026-09-16。用户经 IDP 队列通道 1 先问「这个方式是我们内外 API 的科学、专业、先进的解决方案吗」，通道 1 复核后答「身份与准入层成立；作为完整方案缺六件，第一件是契约生产与版本策略」，用户令「按建议继续」；本记录据此起草，**接受与否归用户**，接受前不是依据。它填的是 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) Decision 五显式留白的那一格——「契约怎么生产、URL 主版本、public 与 internal 契约要不要拆、文档门户放哪个入口，本记录不定」。裁决能力边界：读过 [ADR-0022](./0022-http-status-carries-answer-formed-not-business-verdict.md)、[ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)、[ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)、ADR-0139 全文，[`docs/api` 的 Go + chi + Scalar 方案稿](../api/Go_Chi_Scalar_API_Documentation_Final_V1.0.md)全文，`internal/platform/httpapi/router.go`，`internal/parcelshipment/adapters/http/submit_shipment_request.go` 与 `cmd/parcel-api` 的 `assembleBusinessEndpoints`，[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)对机制半边清点生成物的 CI 纪律那一句，[docs/README](../README.md) 对该方案稿的登记；未逐一重读其余上下文的 `adapters/http`、未核对 OpenAPI 3.1 在本仓将锁定的工具版本上的实际支持——本记录因此只裁**契约的事实源、生成与校验纪律、版本与弃用的机制、错误响应的媒体类型**，不裁任何端点的字段、不裁文档的阅读策略、不裁弃用窗口的长度）<!-- 索引注记 -->｜**事实源判据**：装配表上的处理器全是手写的、已按 ADR-0022 各自持有 `outcome` 词表、响应形状与 405 自守并有测试钉住，契约先行 + `oapi-codegen` 胶水会让同一 operation 迁移期有两个事实源、且把 405 挪到路由层——所以描述符跟处理器住、文档从描述符生成、CI 重生成比对（同机制半边清点的纪律）。**族分拣判据**：端点在装配点挂哪一族 Intake 决定它进首方还是操作者契约，不靠 `x-audience` 标签。**适用场景**：首方 API `/v1` 前缀在装配点按族加、操作者 API 不带（与客户端同版本发布）；问题体换 `application/problem+json`（`type`/`title`/`status`/`code`/`requestId`，不带 `detail`——`UC-PS-001` 泄露约束），写问题体的辅助收进 `httpapi`、错误码仍归各处理器包；不加契约校验中间件（ADR-0055 Decision 一「第二处权威」）；`outcome` 新增取值不算破坏，条件是文档写明未识别 `outcome` 交人；弃用 `Deprecation`（RFC 9745）+ `Sunset`（RFC 8594），窗口长度登记册增行、未登记不弃用；文档门户自托管 Scalar 由 `parcel-api` 提供，阅读策略归 owner；OpenAPI 3.1 工具链核实、描述符 Go 形状、SDK、文档端口、各 owner 逐端点确认描述符列为风险点｜**补充**（2026-10-19）：首方提交答复逐件带回包裹号（`parcels`）、首方契约对客户追踪视图承诺的查询标识是这个号，由 [ADR-0223](./0223-the-first-party-submission-answer-names-each-parcel-and-the-tracking-view-locates-by-it.md) 定——Decision 八「不在本记录定任何端点的字段」留给各上下文 owner 的一处落点；本记录各条不变。
```

### ADR-0141：集成方沙箱是首方 API 的一个部署形态，不是代码路径——同一二进制、独立数据库、独立受众、全 `SYN-` 合成数据，走真首方渠道 Intake 与真令牌校验；开发方运营的产品沙箱属机制半边、随产品就绪存在，某租户的沙箱属实例半边；沙箱证据一律记 `S`，沙箱只灌 `SYN-` 合成数据、不加代码前缀门，隔离开关不进沙箱

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19 **修订后接受**：用户在 Claude Code 会话里答「接受」，按会话复核建议修订。修订要点：删去原 Decision 二「只接受合成标识」的登记前缀门，因为它与 [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定一「生产代码不以 `SYN-` 前缀决定控制流」相冲；隔离形态照 ADR-0150 逐口退场，沙箱不设两个开关；外部事实经真渠道由合成主体登记；沙箱运营取值归开发方部署配置；越权风险点逐条按复核建议处置。此前 Proposed——**草案**，2026-09-16。用户经 IDP 队列通道 1 令「按建议继续」，本记录是通道 1 复核所列六件缺项的第二件——「[ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) 正确地否决了『开发用』API Key，但没回答『客户的开发者拿什么测』」。**接受与否归用户**，接受前不是依据。裁决能力边界：读过 ADR-0139、[ADR-0078](./0078-isolated-environment-operations-reads-admit-by-assembly-injection.md)、[ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md)、[ADR-0016](./0016-product-delivery-and-tenant-pilot-as-parallel-tracks.md) 的标题与两轨制前提、[ADR-0140](./0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md) 全文，[`docs/api` 方案稿](../api/Go_Chi_Scalar_API_Documentation_Final_V1.0.md)的沙箱相关各节，`cmd/parcel-api` 的 `assembleBusinessEndpoints` 与隔离开关的装配，[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)证据层级与产品就绪判据两节；未重读 `scripts/demo-seeds/` 的种子内容、未核对发行方（Ory Hydra 形）对多受众或多项目的实际配置方式——本记录因此只裁**沙箱的性质、隔离物与准入路径**，不裁沙箱的数据内容、重置节奏与配额）<!-- 索引注记 -->｜**已接受**（2026-10-19，用户在 Claude Code 会话里答「接受」，按会话复核建议修订后接受）；删去与 ADR-0150 相冲的登记前缀门。接住 ADR-0139 否决「开发用」API Key 之后没答的「集成方拿什么测」。**部署形态判据**：集成方要测的是真认证、真三格、真判重、真 `outcome`，恰等于「与生产同一二进制」，差别只在数据库、受众、域名三个已有部署参数；ADR-0078 Decision 四「不得再添 demo/mode 类全局开关」原文有效，沙箱不是开关。**隔离物判据**：受众不同让沙箱令牌在生产 `401`、反向亦然（结构保证）；沙箱部署以一个只朝拦的参数「只接受合成标识」让所有登记写面拒绝非 `SYN-` 前缀的租户与客户账户——`SYN-` 前缀门禁从守开关值改为守登记。**适用场景**：沙箱不设 `IDP_PARCEL_ISOLATED_*` 开关，走真首方 Intake，与隔离形态是装配点上并列的两行；编排前件用合成登记满足、不许假目录（ADR-0091 Decision 二）；沙箱证据一律 `S`、不进任何 PN-08 判据；沙箱 client 由发行方签发、在沙箱渠道册登记（同生产机制），不做匿名自助开户；不用 OpenAPI mock 服务器充当沙箱；发行方形态、谁能拿沙箱 client、运营参数、沙箱里外部事实的注入面、契约集成测试的 CI 形态、操作者册的合成标记列为风险点
```

合并后：

```text
Status: Accepted（2026-10-19 **修订后接受**：用户在 Claude Code 会话里答「接受」，按会话复核建议修订。修订要点：删去原 Decision 二「只接受合成标识」的登记前缀门，因为它与 [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定一「生产代码不以 `SYN-` 前缀决定控制流」相冲；隔离形态照 ADR-0150 逐口退场，沙箱不设两个开关；外部事实经真渠道由合成主体登记；沙箱运营取值归开发方部署配置；越权风险点逐条按复核建议处置。此前 Proposed——**草案**，2026-09-16。用户经 IDP 队列通道 1 令「按建议继续」，本记录是通道 1 复核所列六件缺项的第二件——「[ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md) 正确地否决了『开发用』API Key，但没回答『客户的开发者拿什么测』」。**接受与否归用户**，接受前不是依据。裁决能力边界：读过 ADR-0139、[ADR-0078](./0078-isolated-environment-operations-reads-admit-by-assembly-injection.md)、[ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md)、[ADR-0016](./0016-product-delivery-and-tenant-pilot-as-parallel-tracks.md) 的标题与两轨制前提、[ADR-0140](./0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md) 全文，[`docs/api` 方案稿](../api/Go_Chi_Scalar_API_Documentation_Final_V1.0.md)的沙箱相关各节，`cmd/parcel-api` 的 `assembleBusinessEndpoints` 与隔离开关的装配，[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)证据层级与产品就绪判据两节；未重读 `scripts/demo-seeds/` 的种子内容、未核对发行方（Ory Hydra 形）对多受众或多项目的实际配置方式——本记录因此只裁**沙箱的性质、隔离物与准入路径**，不裁沙箱的数据内容、重置节奏与配额）<!-- 索引注记 -->｜**部署形态判据**：集成方要测的是真认证、真三格、真判重、真 `outcome`，恰等于「与生产同一二进制」，差别只在数据库、受众、域名三个已有部署参数；ADR-0078 Decision 四「不得再添 demo/mode 类全局开关」原文有效，沙箱不是开关。**隔离物判据**：受众不同让沙箱令牌在生产 `401`、反向亦然（结构保证）；草案里另有的「只接受合成标识」登记前缀门已在接受时删去（与 ADR-0150 决定一相冲，见 Status 前段），沙箱只灌 `SYN-` 合成数据、不加代码前缀门。**适用场景**：沙箱不设 `IDP_PARCEL_ISOLATED_*` 开关，走真首方 Intake，与隔离形态是装配点上并列的两行；编排前件用合成登记满足、不许假目录（ADR-0091 Decision 二）；沙箱证据一律 `S`、不进任何 PN-08 判据；沙箱 client 由发行方签发、在沙箱渠道册登记（同生产机制），不做匿名自助开户；不用 OpenAPI mock 服务器充当沙箱；发行方形态、谁能拿沙箱 client、运营参数、沙箱里外部事实的注入面、契约集成测试的 CI 形态、操作者册的合成标记列为风险点
```

### ADR-0142：首方 webhook 是产品自有的出向渠道，属机制半边——订阅绑定在客户渠道册的行上、载荷是契约拥有的投影而不是内部信封、投递由 Outbox → 派发 → 出向渠道消费者承担、至少一次、按事件 ID 幂等、按客户密钥签名、失败预算耗尽即暂停订阅并出声；它是 `NotificationChannelGateway` 在机器接收方上的第一份生产实现，结果层只诚实记到「渠道接受」；`PAR-INT-06` 收窄为「某客户实际启用哪条通知渠道」与既有消息渠道的适配

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19 **修订后接受**：用户在 Claude Code 会话里答「接受」，按会话复核建议修订。修订要点：`PAR-INT-06` 已按 ADR-0146 拆分，本记录是那条注所指的第一种出向连接器形态（Decision 一）；对外事件写成「事件描述符」由 ADR-0140 的生成器收进 `webhooks` 节（Decision 四）；只为声明对外的信封类型登出向路由（Decision 五）；时间窗由接收方定、签名构造与入向签名推送共用一套（Decision 六）；重试节奏随产品发布参考节奏（Decision 七）；VE 通知只有一条入账路（Decision 八）；越权风险点逐条按复核建议处置。此前 Proposed——**草案**，2026-09-16。用户经 IDP 队列通道 1 令「按建议继续」，本记录是通道 1 复核所列六件缺项的第三件——「客户 API 今天只有请求-响应；轨迹更新、异常通知、受理结果都该能推；本仓已有 Outbox 与信封机制，缺的只是一份 ADR」。**接受与否归用户**，接受前不是依据。裁决能力边界：读过 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md)、[ADR-0140](./0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md)、[ADR-0049](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md) 全文，[ADR-0043](./0043-publish-intent-claimed-by-result-identity.md) 与 [ADR-0072](./0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md) 的 Decision，`internal/platform/dispatch/direct_publisher.go` 全文，`internal/visibilityexception/ports/ports.go` 里 `CustomerViewHandoff`、`NotificationChannelGateway`、`NotificationHandoff` 三个口与其注释，[UC-VE-006](../application/visibility-exception/UC-VE-006-FORM-CUSTOMER-DISCLOSURE-AND-NOTIFICATION.md) 的结果层与 `AT-VE-103..105`，[参数登记册](../product/PILOT-PARAMETER-REGISTER.md)的 `PAR-INT-06` 行，[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)机制半边现状里 PN-06 那句「通知网关等真实渠道凭证」；未重读 VE CONTEXT 全文、未重读各上下文 `Outbox*Handoff` 的信封内容、未核对 `eventing.Envelope` 的全部字段——本记录因此只裁**出向渠道的归属、订阅模型、投递语义与结果层的诚实映射**，不裁 v1 事件目录的内容、不裁任何事件载荷的字段、不裁重试节奏的取值）<!-- 索引注记 -->｜**已接受**（2026-10-19，用户在 Claude Code 会话里答「接受」，按会话复核建议修订后接受）。**归族判据**：同 ADR-0139 对 `PAR-INT-01` 的拆法——`PAR-INT-06` 最低证据列（消息渠道能力、送达回执、失败恢复）是客户既有消息渠道的属性，产品自己带的机器回调协议、签名密钥形态、订阅绑定模型三件都取决于产品；「默认人工确认、仅批准范围可自动发布」管披露内容，归 `UC-VE-006`，一字不碰。**落点判据**：ADR-0072 的镜像——带（租户，客户账户）隔离语义不进 `platform`，不判业务不是限界上下文，落 `internal/outboundchannel/` 与 `accessidentity` 对称。**诚实判据**：webhook 只观察得到提交 / 接收方 2xx / 失败，映到 `UC-VE-006` 的提交、渠道接受、失败三层，送达与客户确认两层不造推定（`AT-VE-103..105`）。**适用场景**：订阅继承渠道行的（租户，客户账户），自助口 `/v1/webhook-subscriptions` 与管理台写面两口同一用例；载荷由消费侧翻译器从内部信封投影、未声明的内部事件默认不对外；`outboundchannel` 作为派发路由表的一个 `Consumer`，消费门只落投递账、发送在自己的循环里；事件 ID 从信封 ID 派生（ADR-0043）、接收方按它幂等且不假定有序；HMAC-SHA256 签名照 Standard Webhooks 头名与原文构造、密钥轮换并存窗口；失败预算耗尽进「已暂停」、不静默丢、可恢复补发、必须出声；v1 事件目录、接受结果回写 VE、投递循环进程归属、节奏取值、mTLS / 出向 IP、1:N 下的订阅、沙箱事实来源、客户确认回写用例列为风险点
```

合并后：

```text
Status: Accepted（2026-10-19 **修订后接受**：用户在 Claude Code 会话里答「接受」，按会话复核建议修订。修订要点：`PAR-INT-06` 已按 ADR-0146 拆分，本记录是那条注所指的第一种出向连接器形态（Decision 一）；对外事件写成「事件描述符」由 ADR-0140 的生成器收进 `webhooks` 节（Decision 四）；只为声明对外的信封类型登出向路由（Decision 五）；时间窗由接收方定、签名构造与入向签名推送共用一套（Decision 六）；重试节奏随产品发布参考节奏（Decision 七）；VE 通知只有一条入账路（Decision 八）；越权风险点逐条按复核建议处置。此前 Proposed——**草案**，2026-09-16。用户经 IDP 队列通道 1 令「按建议继续」，本记录是通道 1 复核所列六件缺项的第三件——「客户 API 今天只有请求-响应；轨迹更新、异常通知、受理结果都该能推；本仓已有 Outbox 与信封机制，缺的只是一份 ADR」。**接受与否归用户**，接受前不是依据。裁决能力边界：读过 [ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md)、[ADR-0140](./0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md)、[ADR-0049](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md) 全文，[ADR-0043](./0043-publish-intent-claimed-by-result-identity.md) 与 [ADR-0072](./0072-access-channel-capability-is-shared-and-registry-shape-awaits-channel-evidence.md) 的 Decision，`internal/platform/dispatch/direct_publisher.go` 全文，`internal/visibilityexception/ports/ports.go` 里 `CustomerViewHandoff`、`NotificationChannelGateway`、`NotificationHandoff` 三个口与其注释，[UC-VE-006](../application/visibility-exception/UC-VE-006-FORM-CUSTOMER-DISCLOSURE-AND-NOTIFICATION.md) 的结果层与 `AT-VE-103..105`，[参数登记册](../product/PILOT-PARAMETER-REGISTER.md)的 `PAR-INT-06` 行，[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)机制半边现状里 PN-06 那句「通知网关等真实渠道凭证」；未重读 VE CONTEXT 全文、未重读各上下文 `Outbox*Handoff` 的信封内容、未核对 `eventing.Envelope` 的全部字段——本记录因此只裁**出向渠道的归属、订阅模型、投递语义与结果层的诚实映射**，不裁 v1 事件目录的内容、不裁任何事件载荷的字段、不裁重试节奏的取值）<!-- 索引注记 -->｜**归族判据**：同 ADR-0139 对 `PAR-INT-01` 的拆法——`PAR-INT-06` 最低证据列（消息渠道能力、送达回执、失败恢复）是客户既有消息渠道的属性，产品自己带的机器回调协议、签名密钥形态、订阅绑定模型三件都取决于产品；「默认人工确认、仅批准范围可自动发布」管披露内容，归 `UC-VE-006`，一字不碰。**落点判据**：ADR-0072 的镜像——带（租户，客户账户）隔离语义不进 `platform`，不判业务不是限界上下文，落 `internal/outboundchannel/` 与 `accessidentity` 对称。**诚实判据**：webhook 只观察得到提交 / 接收方 2xx / 失败，映到 `UC-VE-006` 的提交、渠道接受、失败三层，送达与客户确认两层不造推定（`AT-VE-103..105`）。**适用场景**：订阅继承渠道行的（租户，客户账户），自助口 `/v1/webhook-subscriptions` 与管理台写面两口同一用例；载荷由消费侧翻译器从内部信封投影、未声明的内部事件默认不对外；`outboundchannel` 作为派发路由表的一个 `Consumer`，消费门只落投递账、发送在自己的循环里；事件 ID 从信封 ID 派生（ADR-0043）、接收方按它幂等且不假定有序；HMAC-SHA256 签名照 Standard Webhooks 头名与原文构造、密钥轮换并存窗口；失败预算耗尽进「已暂停」、不静默丢、可恢复补发、必须出声；v1 事件目录、接受结果回写 VE、投递循环进程归属、节奏取值、mTLS / 出向 IP、1:N 下的订阅、沙箱事实来源、客户确认回写用例列为风险点
```

### ADR-0144：目录读口的分页、排序与筛选下推一处定义——游标分页、每册点名的封闭集排序与筛选维加统一关键词 `q`、页大小仍由 Intake 按渠道契约定、答复带 `page`；契约一次定，读口逐册迁

处理：措辞对齐（接受出处、去向的模糊重说，或引文改为逐字）

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-24，通道 3 经用户在 IDP 队列授权自决——「你能帮我自决吗，你是业务和系统专家」——裁决；票 [admin-web-group-legal-entities/04](../../.scratch/admin-web-group-legal-entities/issues/04-catalogue-read-pagination-sort-filter-contract.md) 的产物，五问的裁决原文在票面「裁决」节，本记录把它写成契约并补齐细处，两处措辞有出入时以本记录为准。裁决能力边界：读过 [ADR-0077](./0077-master-data-catalogue-read-follows-the-operations-read-pattern.md) Decision 全节；`internal/partycommercial/adapters/http/catalogue_intake.go` 全文（`CatalogueQuery`、`CommercialCatalogueIntake`、`problemDetail` 与 `writeProblemWithDetail`）；同包 `query_party_identities.go` 的三种答复体与 outcome 常量、`query_commercial_policies.go` 对 `?kind=` 的封闭集分派、`isolated_read_intake.go` 的 `IntakeCatalogueQuery`；`cmd/parcel-api/assemble_isolated_read.go` 的 `isolatedReadLimit` 常量注与 `buildIsolatedReadIntakes` 开头；`internal/parcelpricing/adapters/postgres/operations_catalogue.go` 的 `catalogueLimit`；party-commercial postgres 适配器各 `ORDER BY` 子句（`git grep`）；`apps/admin-web/README.md`「列表页上列通则」；`apps/admin-web/src/templates/ListPageTemplate.tsx` 的 `ListPaginationProps`；`pages/party` 三份列表模型的头注。没读：其余上下文的目录处理器与 postgres 读口全文、[ADR-0071](./0071-catalogue-views-carry-tenant-in-the-method-signature.md)、ADR-0076 与 ADR-0078 全文、各登记表的索引现状、vendor `Pagination` 源码。拿不准的列在文末「越权风险点」。）<!-- 索引注记 -->｜**契约判据**：登记册只增不改、列表新登在前，偏移分页在翻页期间会重行、漏行；页大小不采信调用方自报、查询参数一律封闭集，沿既有纪律。2026-09-24 用户授权通道 3 自决接受，越权风险点待各上下文 owner 复核
```

合并后：

```text
Status: Accepted（2026-09-24，通道 3 经用户在 IDP 队列授权自决——「你能帮我自决吗，你是业务和系统专家」——裁决；票 [admin-web-group-legal-entities/04](../../.scratch/admin-web-group-legal-entities/issues/04-catalogue-read-pagination-sort-filter-contract.md) 的产物，五问的裁决原文在票面「裁决」节，本记录把它写成契约并补齐细处，两处措辞有出入时以本记录为准。裁决能力边界：读过 [ADR-0077](./0077-master-data-catalogue-read-follows-the-operations-read-pattern.md) Decision 全节；`internal/partycommercial/adapters/http/catalogue_intake.go` 全文（`CatalogueQuery`、`CommercialCatalogueIntake`、`problemDetail` 与 `writeProblemWithDetail`）；同包 `query_party_identities.go` 的三种答复体与 outcome 常量、`query_commercial_policies.go` 对 `?kind=` 的封闭集分派、`isolated_read_intake.go` 的 `IntakeCatalogueQuery`；`cmd/parcel-api/assemble_isolated_read.go` 的 `isolatedReadLimit` 常量注与 `buildIsolatedReadIntakes` 开头；`internal/parcelpricing/adapters/postgres/operations_catalogue.go` 的 `catalogueLimit`；party-commercial postgres 适配器各 `ORDER BY` 子句（`git grep`）；`apps/admin-web/README.md`「列表页上列通则」；`apps/admin-web/src/templates/ListPageTemplate.tsx` 的 `ListPaginationProps`；`pages/party` 三份列表模型的头注。没读：其余上下文的目录处理器与 postgres 读口全文、[ADR-0071](./0071-catalogue-views-carry-tenant-in-the-method-signature.md)、ADR-0076 与 ADR-0078 全文、各登记表的索引现状、vendor `Pagination` 源码。拿不准的列在文末「越权风险点」，待各上下文 owner 复核。）<!-- 索引注记 -->｜**契约判据**：登记册只增不改、列表新登在前，偏移分页在翻页期间会重行、漏行；页大小不采信调用方自报、查询参数一律封闭集，沿既有纪律
```

### ADR-0145：责任法人的业务属性分身份与资料两层——注册国家 / 地区与终身注册号随身份登记、不作变更（录错走更正修订）；注册地址、税务登记号、开票资料与联系人归按生效时点修订的「法人资料」，对外单据开立时固定其修订引用；币种不上法人；登记只拦身份层，开票资料缺失由开立方拒开

处理：措辞对齐（接受出处、去向的模糊重说，或引文改为逐字）

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-24，通道 3 经用户在 IDP 队列授权自决——「你能帮我自决吗，你是业务和系统专家」——裁决；票 [admin-web-group-legal-entities/05](../../.scratch/admin-web-group-legal-entities/issues/05-legal-entity-business-attributes.md) 的产物。票面「裁决」节是同日早些时候的口径，本记录读过 CONTEXT 之后细化两处，以本记录为准：一、录错不走「停用 + 重登」，走既有的内容更正修订；二、结算币种不进法人资料。裁决能力边界：读过 party-commercial [`CONTEXT.md`](../domain/party-commercial/CONTEXT.md) 的 Language「运营集团租户」「责任法人」「业务参与方」「货主客户账户」「参与方关系」「商业版本」词条，Rules 中参与方身份登记、结算政策与预付账期解析诸句，Lifecycles「参与方身份」一节，Boundaries 全节；[GLOSSARY](../domain/GLOSSARY.md)「货主客户账户」词条；[ADR-0003](./0003-group-tenant-legal-entity-customer-account.md) Decision；[ADR-0101](./0101-operator-facing-registration-payload-shape-is-product-defined.md) Decision 八；`internal/partycommercial/adapters/http/query_party_identities.go` 的 `groupLegalEntityBody`；外来方案 [`IDP_Parcel_Commercial_Party_UX_V1.0.md`](../reference/IDP_Parcel_Commercial_Party_UX_V1.0.md) 的产品模型一节与目录。没读：settlement-accounting 与 customs-compliance 的 `CONTEXT.md`（对账单开立与申报主体资料的需求）、[ADR-0143](./0143-customer-statement-numbering-uses-platform-assignment.md)（Proposed）、party-commercial 身份登记的迁移与领域代码、外来方案 Tax & Registration 诸节正文。拿不准的列在文末「越权风险点」。）<!-- 索引注记 -->｜**切分判据**：注册国家与终身注册号定义「是哪一个法人」，随身份；地址、税号、开票资料与联系人按自己的节奏变，归按生效时点修订的法人资料；币种已由结算政策拥有，不上法人。2026-09-24 用户授权通道 3 自决接受，越权风险点待 PC owner 复核
```

合并后：

```text
Status: Accepted（2026-09-24，通道 3 经用户在 IDP 队列授权自决——「你能帮我自决吗，你是业务和系统专家」——裁决；票 [admin-web-group-legal-entities/05](../../.scratch/admin-web-group-legal-entities/issues/05-legal-entity-business-attributes.md) 的产物。票面「裁决」节是同日早些时候的口径，本记录读过 CONTEXT 之后细化两处，以本记录为准：一、录错不走「停用 + 重登」，走既有的内容更正修订；二、结算币种不进法人资料。裁决能力边界：读过 party-commercial [`CONTEXT.md`](../domain/party-commercial/CONTEXT.md) 的 Language「运营集团租户」「责任法人」「业务参与方」「货主客户账户」「参与方关系」「商业版本」词条，Rules 中参与方身份登记、结算政策与预付账期解析诸句，Lifecycles「参与方身份」一节，Boundaries 全节；[GLOSSARY](../domain/GLOSSARY.md)「货主客户账户」词条；[ADR-0003](./0003-group-tenant-legal-entity-customer-account.md) Decision；[ADR-0101](./0101-operator-facing-registration-payload-shape-is-product-defined.md) Decision 八；`internal/partycommercial/adapters/http/query_party_identities.go` 的 `groupLegalEntityBody`；外来方案 [`IDP_Parcel_Commercial_Party_UX_V1.0.md`](../reference/IDP_Parcel_Commercial_Party_UX_V1.0.md) 的产品模型一节与目录。没读：settlement-accounting 与 customs-compliance 的 `CONTEXT.md`（对账单开立与申报主体资料的需求）、[ADR-0143](./0143-customer-statement-numbering-uses-platform-assignment.md)（Proposed）、party-commercial 身份登记的迁移与领域代码、外来方案 Tax & Registration 诸节正文。拿不准的列在文末「越权风险点」，待 PC owner 复核。）<!-- 索引注记 -->｜**切分判据**：注册国家与终身注册号定义「是哪一个法人」，随身份；地址、税号、开票资料与联系人按自己的节奏变，归按生效时点修订的法人资料；币种已由结算政策拥有，不上法人
```

### ADR-0146：产品策略是机制与实例之间的第三类——判断方法、内置策略与随产品发布的参考配置归产品交付，实例半边只剩某租户的具体取值；参考配置须显式采用才生效；产品就绪加判「演示动线走通」

处理：补指针

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-24，通道 4 经用户在 IDP 队列授权自决——「你是系统和业务专家，请你自决」——裁决；起因是用户同日一句「我们是软件公司，不可能等到租户就绪、业务就绪再去补系统，这是我们的产品，原来的决策有问题」。本记录**部分取代** [ADR-0016](./0016-product-delivery-and-tenant-pilot-as-parallel-tracks.md) 的机制半边定义与第三条判据，0016 的两条轨道与其余决定不变、正文不改写。裁决能力边界：读过[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「产品就绪与试点就绪」「切片的机制半边与实例半边」「机制半边现状」三节，[ADR-0015](./0015-closed-grammar-open-vocabulary-for-multi-carrier-rating.md) Decision，ADR-0016 Decision 与 Alternatives，[ADR-0017](./0017-admission-gates-judged-by-blocking-cause.md) 全文，[ADR-0145](./0145-legal-entity-attributes-split-into-identity-layer-and-dated-profile.md) 越权风险点，[参数登记册](../product/PILOT-PARAMETER-REGISTER.md)头注与 `PAR-NET-*`、`PAR-INT-01..03`、`PAR-COM-13..16` 诸行，票 `nr-route-evidence-views/01`，[合成演示动线](../design/synthetic-demo-journey-script.md)的前置与「三堵墙」，2026-09-22 [完成情况快照](../overview/completion-assessment-2026-09-22.md)。没读：各 `CONTEXT.md` 全文、PN-02..08 各交接文档、`PILOT-SCOPE` 与 `PILOT-ACCEPTANCE-MATRIX` 正文、登记册其余各行、以「实例半边」为由暂缓的各 ADR 正文。拿不准的列在文末「越权风险点」。）<!-- 索引注记 -->｜**分界检验**：不看任何一个租户的客户、网络、合同就能回答的判断方法归产品策略，「这个租户具体是多少、是谁、是哪一条」归租户取值；部分取代 ADR-0016 的机制半边定义与第三条判据，按新判据产品就绪暂不成立。2026-09-24 用户授权通道 4 自决接受，越权风险点待评审复核
```

合并后：

```text
Status: Accepted（2026-09-24，通道 4 经用户在 IDP 队列授权自决——「你是系统和业务专家，请你自决」——裁决；起因是用户同日一句「我们是软件公司，不可能等到租户就绪、业务就绪再去补系统，这是我们的产品，原来的决策有问题」。本记录**部分取代** [ADR-0016](./0016-product-delivery-and-tenant-pilot-as-parallel-tracks.md) 的机制半边定义与第三条判据，0016 的两条轨道与其余决定不变、正文不改写。裁决能力边界：读过[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「产品就绪与试点就绪」「切片的机制半边与实例半边」「机制半边现状」三节，[ADR-0015](./0015-closed-grammar-open-vocabulary-for-multi-carrier-rating.md) Decision，ADR-0016 Decision 与 Alternatives，[ADR-0017](./0017-admission-gates-judged-by-blocking-cause.md) 全文，[ADR-0145](./0145-legal-entity-attributes-split-into-identity-layer-and-dated-profile.md) 越权风险点，[参数登记册](../product/PILOT-PARAMETER-REGISTER.md)头注与 `PAR-NET-*`、`PAR-INT-01..03`、`PAR-COM-13..16` 诸行，票 `nr-route-evidence-views/01`，[合成演示动线](../design/synthetic-demo-journey-script.md)的前置与「三堵墙」，2026-09-22 [完成情况快照](../overview/completion-assessment-2026-09-22.md)。没读：各 `CONTEXT.md` 全文、PN-02..08 各交接文档、`PILOT-SCOPE` 与 `PILOT-ACCEPTANCE-MATRIX` 正文、登记册其余各行、以「实例半边」为由暂缓的各 ADR 正文。拿不准的列在文末「越权风险点」，待评审复核。另：决定七把候选生成、硬约束过滤、排序策略族等改判为产品策略，部分停用 [ADR-0052](./0052-network-evidence-catalogue-has-an-unconfigured-grade.md) 决定五、[ADR-0053](./0053-network-fact-families-are-derived-not-registrable.md) 决定二、[ADR-0068](./0068-versioned-network-catalog-structure-precedes-rule-content.md) 决定三与决定六里把它们归 `PAR-NET-14` 的句子，被停原句与适用场景见三份的 Status 行（指针 2026-10-21 补））<!-- 索引注记 -->｜**分界检验**：不看任何一个租户的客户、网络、合同就能回答的判断方法归产品策略，「这个租户具体是多少、是谁、是哪一条」归租户取值；按新判据产品就绪暂不成立
```

### ADR-0148：路由证据按来源分三路——目录折叠、随请求携带、经端口取；路由候选成本由逐段成本依据（BUY 评价或内部价格政策评价）按比较币种换算后合成，任一段缺依据即缺成本依据；首版候选生成以判断时点适用的一条完整线路为一个候选；`未配置`改由有无适用的路由策略版本答，视图修订取目录修订锚

处理：补指针

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-24，通道 5 起草，票 [routing-first-cut/02](../../.scratch/routing-first-cut/issues/02-route-evidence-sourcing-and-candidate-cost-adr.md) 的产物。同日按用户「按你的建议吧」修订决定四与越权风险点一、二：候选成本改按比较币种换算后合成，自营段经内部价格政策计成本。随后用户在 IDP 队列授权通道 5 自决接受——原话「你作为业务，系统专家，参考头部软件的做法，自决吧」；通道 5 对照头部运输管理系统的做法复核后接受，并在决定二、五补记后续形态。越权风险点待各上下文 owner 复核。决定六对 ADR-0068、ADR-0053 的部分停用自 routing-first-cut/07 落地的那一笔起生效，在那之前两份记录照旧；07 已落这一笔，两份被停记录的 Status 行已加前向指针。决定六「改 `0008` 迁移头注」一格因已施加迁移按 checksum 固定（ADR-0068 Consequences）不改，改由迁移 `0011` 头注与 `NetworkCatalog` 类型注释承载，先例是 ADR-0127 对 `0020` 头注的同一处置。裁决能力边界：读过 network-routing [`CONTEXT.md`](../domain/network-routing/CONTEXT.md) 全文，[CONTEXT-MAP](../domain/CONTEXT-MAP.md) 中 network-routing 的各条边，[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 全文，[ADR-0075](./0075-customer-address-is-carried-with-the-routing-request.md) Decision，[ADR-0068](./0068-versioned-network-catalog-structure-precedes-rule-content.md) 决定六，[ADR-0053](./0053-network-fact-families-are-derived-not-registrable.md) Decision，[ADR-0145](./0145-legal-entity-attributes-split-into-identity-layer-and-dated-profile.md) 决定四，票 `first-tenant-runway/03` 的 Answer、`nr-route-evidence-views/01`、`label-channel-service-first-release/01` 的裁决与 `/13` 的票头；代码读过 network-routing 的证据视图端口与初始路由判断管线、迁移 `0008` 的头注与表结构、parcel-shipment 的地址要素类型与渠道择优取成本的适配器；customs-compliance 与 parcel-pricing 的 `CONTEXT.md` 只按「口岸 / 申报路径 / 关务区域」「汇率 / 换算」检索读了相关句（含 PP 的「外部数值序列」「换算」词条与汇率口径不变量），party-commercial `CONTEXT.md` 读了「价格政策」词条。没读：CC、PP、PS 三份 `CONTEXT.md` 全文，settlement-accounting `CONTEXT.md`，`pp-pricing-input-seams` 的 spec，ADR-0109；ADR-0147 在起草中未读。拿不准的列在文末「越权风险点」。）<!-- 索引注记 -->｜**来源判据**：判断对象那一版的内容随请求携带（ADR-0075 的同版性），邻接上下文拥有的判断经端口取，其余从网络目录折出；关务适用性判断在 CC 侧缺执行器，另立票。**成本口径**：比较币种是租户取值、汇率口径由价格政策声明、换算归 PP，自营段与外包段经同一评价机制同口径比。2026-09-24 用户授权通道 5 自决接受，越权风险点待各上下文 owner 复核；决定六对 ADR-0068、0053 的部分停用已随 routing-first-cut/07 落地生效
```

合并后：

```text
Status: Accepted（2026-09-24，通道 5 起草，票 [routing-first-cut/02](../../.scratch/routing-first-cut/issues/02-route-evidence-sourcing-and-candidate-cost-adr.md) 的产物。同日按用户「按你的建议吧」修订决定四与越权风险点一、二：候选成本改按比较币种换算后合成，自营段经内部价格政策计成本。随后用户在 IDP 队列授权通道 5 自决接受——原话「你作为业务，系统专家，参考头部软件的做法，自决吧」；通道 5 对照头部运输管理系统的做法复核后接受，并在决定二、五补记后续形态。越权风险点待各上下文 owner 复核。决定六对 ADR-0068、0053 的部分停用已随 routing-first-cut/07 落地生效。决定六对 ADR-0068、ADR-0053 的部分停用自 routing-first-cut/07 落地的那一笔起生效，在那之前两份记录照旧；07 已落这一笔，两份被停记录的 Status 行已加前向指针。决定六「改 `0008` 迁移头注」一格因已施加迁移按 checksum 固定（ADR-0068 Consequences）不改，改由迁移 `0011` 头注与 `NetworkCatalog` 类型注释承载，先例是 ADR-0127 对 `0020` 头注的同一处置。裁决能力边界：读过 network-routing [`CONTEXT.md`](../domain/network-routing/CONTEXT.md) 全文，[CONTEXT-MAP](../domain/CONTEXT-MAP.md) 中 network-routing 的各条边，[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 全文，[ADR-0075](./0075-customer-address-is-carried-with-the-routing-request.md) Decision，[ADR-0068](./0068-versioned-network-catalog-structure-precedes-rule-content.md) 决定六，[ADR-0053](./0053-network-fact-families-are-derived-not-registrable.md) Decision，[ADR-0145](./0145-legal-entity-attributes-split-into-identity-layer-and-dated-profile.md) 决定四，票 `first-tenant-runway/03` 的 Answer、`nr-route-evidence-views/01`、`label-channel-service-first-release/01` 的裁决与 `/13` 的票头；代码读过 network-routing 的证据视图端口与初始路由判断管线、迁移 `0008` 的头注与表结构、parcel-shipment 的地址要素类型与渠道择优取成本的适配器；customs-compliance 与 parcel-pricing 的 `CONTEXT.md` 只按「口岸 / 申报路径 / 关务区域」「汇率 / 换算」检索读了相关句（含 PP 的「外部数值序列」「换算」词条与汇率口径不变量），party-commercial `CONTEXT.md` 读了「价格政策」词条。没读：CC、PP、PS 三份 `CONTEXT.md` 全文，settlement-accounting `CONTEXT.md`，`pp-pricing-input-seams` 的 spec，ADR-0109；ADR-0147 在起草中未读。拿不准的列在文末「越权风险点」。另：决定六的判法同时使 [ADR-0052](./0052-network-evidence-catalogue-has-an-unconfigured-grade.md) 决定一「`configured=false` 即**网络定义登记册对这个判断范围未配置**」不再成立，见 ADR-0052 的 Status 行（指针 2026-10-21 补））<!-- 索引注记 -->｜**来源判据**：判断对象那一版的内容随请求携带（ADR-0075 的同版性），邻接上下文拥有的判断经端口取，其余从网络目录折出；关务适用性判断在 CC 侧缺执行器，另立票。**成本口径**：比较币种是租户取值、汇率口径由价格政策声明、换算归 PP，自营段与外包段经同一评价机制同口径比
```

### ADR-0149：主链业务命令面按提交者分两族——一线作业事实走操作者渠道族新增的「作业事实登记」能力面（人 + 已登记设备），外部结果与资金事实走集成客户端族（客户端凭据，推送源经签名 webhook）；ADR-0055 决定五两项按口逐一解

处理：来向两写合并；补指针

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-24，用户经 IDP 队列授权通道 4「参考专业头部软件的做法，你来帮我自决吧」，承票 operator-channel/09。裁决能力边界：读过 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定一至六与 Consequences、[ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md) 决定五、[ADR-0023](./0023-work-fact-identity-and-time-are-minted-by-the-device.md) 所定的纪律、[ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) 逐口放行在 `cmd/parcel-api/endpoints.go` 头注里的装配形态、`internal/accessidentity` 的全部导出面。没读：ADR-0139 至 0142 全文（客户渠道，Proposed）、各命令口载荷的现有形状——后者逐口在实施票里核。）；**部分停用**：决定一列举里「派送任务」「段关闭」「有效时间判断」三项对管理台手工建派送任务、关段与有效时间判断三口（`/transport-fulfillment-dispatch-task-registrations`、`/transport-fulfillment-segment-closures`、`/transport-fulfillment-effective-time-judgments`）的适用已由 [ADR-0151](./0151-unassigned-command-faces-get-their-families.md) 停用——三口是管理台上的运营决定与判断，归操作者渠道的「运营决定」能力面；派送发起口与其余各条不变；**部分停用**：决定一「外部结果与外部资金事实（关务外部结果、监管凭证、承运商轨迹与首次有效收寄证据、外部资金事实）的提交者是外部系统」一句与决定三对外部资金事实、监管凭证两类的独占已由 [ADR-0236](./0236-operators-adopt-external-funds-facts-and-regulatory-credentials-through-operator-decision-faces.md) 决定六停用，适用场景只限租户人员经三个手工口手工采用这一格；外部系统送来的那一格与其余各条不变<!-- 索引注记 -->｜**按提交者分族**：作业事实的提交者是运营企业的一线人员，外部结果与资金事实的提交者是外部系统，都不是货主客户；载荷规范化形状逐口定，生产写入须落在治理的生产权威区间内。部分停用 ADR-0100 决定四末段与决定五第一条的一半。2026-09-24 用户授权通道 4「参考专业头部软件的做法」自决接受，越权风险点待评审复核｜**部分停用**：决定一列举里「派送任务」「段关闭」「有效时间判断」三项对管理台手工建派送任务、关段与有效时间判断三口的适用已由 [ADR-0151](./0151-unassigned-command-faces-get-their-families.md) 停用；**适用场景**：`/transport-fulfillment-dispatch-task-registrations`、`/transport-fulfillment-segment-closures`、`/transport-fulfillment-effective-time-judgments`；派送发起口与其余各条不变｜**部分停用**：决定一「外部结果与外部资金事实（……）的提交者是外部系统」一句与决定三「外部结果与资金事实走集成客户端族」对外部资金事实、监管凭证两类的独占已由 [ADR-0236](./0236-operators-adopt-external-funds-facts-and-regulatory-credentials-through-operator-decision-faces.md) 决定六停用——租户的财务、关务人员手工采用另走操作者渠道「运营决定」能力面。**适用场景**：只限 `/settlement-external-funds-fact-manual-registrations`、`/settlement-external-funds-fact-correction-manual-registrations`、`/customs-regulatory-credential-manual-registrations` 三口，理由是这一格的提交者是租户自己的人；外部系统送来的那一格照旧归集成客户端族，其余各条不变
```

合并后：

```text
Status: Accepted（2026-09-24，用户经 IDP 队列授权通道 4「参考专业头部软件的做法，你来帮我自决吧」，承票 operator-channel/09。裁决能力边界：读过 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定一至六与 Consequences、[ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md) 决定五、[ADR-0023](./0023-work-fact-identity-and-time-are-minted-by-the-device.md) 所定的纪律、[ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) 逐口放行在 `cmd/parcel-api/endpoints.go` 头注里的装配形态、`internal/accessidentity` 的全部导出面。没读：ADR-0139 至 0142 全文（客户渠道，Proposed）、各命令口载荷的现有形状——后者逐口在实施票里核。越权风险点待评审复核。另：决定四按口解两项未决，使 [ADR-0078](./0078-isolated-environment-operations-reads-admit-by-assembly-injection.md) 决定五「两项未决照旧拦着」不再成立，见 ADR-0078 的 Status 行（指针 2026-10-21 补））<!-- 索引注记 -->｜**按提交者分族**：作业事实的提交者是运营企业的一线人员，外部结果与资金事实的提交者是外部系统，都不是货主客户；载荷规范化形状逐口定，生产写入须落在治理的生产权威区间内。部分停用 ADR-0100 决定四末段与决定五第一条的一半｜**部分停用**：决定一列举里「派送任务」「段关闭」「有效时间判断」三项对管理台手工建派送任务、关段与有效时间判断三口的适用已由 [ADR-0151](./0151-unassigned-command-faces-get-their-families.md) 停用；**适用场景**：`/transport-fulfillment-dispatch-task-registrations`、`/transport-fulfillment-segment-closures`、`/transport-fulfillment-effective-time-judgments`；派送发起口（其后由 ADR-0230 改挂，见下）与其余各条不变｜**部分停用**：决定一「外部结果与外部资金事实（……）的提交者是外部系统」一句与决定三「外部结果与资金事实走集成客户端族」对外部资金事实、监管凭证两类的独占已由 [ADR-0236](./0236-operators-adopt-external-funds-facts-and-regulatory-credentials-through-operator-decision-faces.md) 决定六停用——租户的财务、关务人员手工采用另走操作者渠道「运营决定」能力面。**适用场景**：只限 `/settlement-external-funds-fact-manual-registrations`、`/settlement-external-funds-fact-correction-manual-registrations`、`/customs-regulatory-credential-manual-registrations` 三口，理由是这一格的提交者是租户自己的人；外部系统送来的那一格照旧归集成客户端族，其余各条不变｜**部分停用**（指针 2026-10-21 补）：决定一「作业事实登记（节点收寄、场外揽收与尝试、交接、移动、派送任务、交付、段关闭、有效时间判断等）的提交者是运营企业的一线人员」里「派送任务」一项对派送发起补拍口的适用已由 [ADR-0230](./0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md) 决定七停用——`/transport-fulfillment-delivery-dispatch-triggers` 改挂操作者渠道的「运营决定」能力面，再发起的人是管理台上看着待形成清单的调度员。**适用场景**：只限这一口；线格式与答复不变，其余各条不变｜**部分停用**（指针 2026-10-21 补，话题通读所得）：其决定五「隔离形态照旧。」一条里「operator-channel/08 按 ADR-0091 逐口放行的合成写不因本记录退场（ADR-0091 决定六）；两者是装配点上的两行，互不替代」已由 [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定三停用——它所依的 ADR-0091 决定六已被同一条停用，隔离形态改为真渠道缺位时的过渡，某一口换上真渠道 Intake 的同一笔撤下该口的隔离放行。**适用场景**：只限已换上真渠道的口；尚未换的口在过渡期内照旧两行并存，其余各条不变
```

### ADR-0150：开发与演示中合成租户按真实租户对待——代码路径不区分合成与真实租户，合成租户经真渠道进出；隔离形态是真渠道缺位时的过渡，某一口的真渠道 Intake 落地即在同一笔撤下该口的隔离放行；两条红线不变

处理：补指针

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-24，通道 4 据用户在 IDP 队列同日两句裁定成文——「只有合成与真实租户数据这道墙在开发的时候也无需考虑，当成真实的就行了，简化开发」，以及对本记录提案的「同意」。本记录**部分停用** [ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) 决定六的一句与 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定五第二条（见决定三），两记录其余各条不变。裁决能力边界：读过 ADR-0091 全文、ADR-0100 全文、[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 全文、[ADR-0078](./0078-isolated-environment-operations-reads-admit-by-assembly-injection.md) 的 Decision 标题与 [adr/README](./README.md) 对它的部分停用说明、[ADR-0141](./0141-integrator-sandbox-is-a-deployment-form-of-the-first-party-api-on-synthetic-data.md) 的索引摘要、`cmd/parcel-api/endpoints.go` 的隔离放行段与 `assemble_isolated_write.go` 的放行名单、operator-channel 04 / 05 / 10 / 11 的票面；没读：ADR-0078 正文的 Context 与 Alternatives、ADR-0083、ADR-0141 全文、各上下文 `CONTEXT.md`、演示种子脚本全文。拿不准的列在文末「越权风险点」。）<!-- 索引注记 -->｜**不分支判据**：隔离形态守的两件事（合成值不成为生产默认、合成证据不冒充生产证据）由红线「租户取值留给租户」「证据层级诚实」守，不靠准入路径不同；合成与真实租户的数据隔离由租户边界给出。**适用场景**：演示租户 `SYN-TENANT-01` 以合成操作者、设备、集成客户端经真渠道进出；某一口换真渠道 Intake 的同一笔撤下该口的隔离放行，最后一口换完删两个隔离开关；真渠道落地前新口仍可照 ADR-0091 逐口放行。部分停用 ADR-0091 决定六一句与 ADR-0100 决定五第二条一段。2026-09-24 用户在 IDP 队列裁定「当成真实的就行了，简化开发」并同意本记录，越权风险点待各册 owner 复核
```

合并后：

```text
Status: Accepted（2026-09-24，通道 4 据用户在 IDP 队列同日两句裁定成文——「只有合成与真实租户数据这道墙在开发的时候也无需考虑，当成真实的就行了，简化开发」，以及对本记录提案的「同意」。本记录**部分停用** [ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) 决定六的一句与 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定五第二条（见决定三），两记录其余各条不变。裁决能力边界：读过 ADR-0091 全文、ADR-0100 全文、[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 全文、[ADR-0078](./0078-isolated-environment-operations-reads-admit-by-assembly-injection.md) 的 Decision 标题与 [adr/README](./README.md) 对它的部分停用说明、[ADR-0141](./0141-integrator-sandbox-is-a-deployment-form-of-the-first-party-api-on-synthetic-data.md) 的索引摘要、`cmd/parcel-api/endpoints.go` 的隔离放行段与 `assemble_isolated_write.go` 的放行名单、operator-channel 04 / 05 / 10 / 11 的票面；没读：ADR-0078 正文的 Context 与 Alternatives、ADR-0083、ADR-0141 全文、各上下文 `CONTEXT.md`、演示种子脚本全文。拿不准的列在文末「越权风险点」，待各册 owner 复核。另：决定三逐口退场同时部分停用 [ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定五「两者是装配点上的两行，互不替代」，见 ADR-0149 的 Status 行，并部分停用 [ADR-0078](./0078-isolated-environment-operations-reads-admit-by-assembly-injection.md) 决定四「隔离读面准入不因真渠道出现自动退场」一句（越权风险点 4 预留的那一格），见 ADR-0078 的 Status 行（指针 2026-10-21 补））<!-- 索引注记 -->｜**不分支判据**：隔离形态守的两件事（合成值不成为生产默认、合成证据不冒充生产证据）由红线「租户取值留给租户」「证据层级诚实」守，不靠准入路径不同；合成与真实租户的数据隔离由租户边界给出。**适用场景**：演示租户 `SYN-TENANT-01` 以合成操作者、设备、集成客户端经真渠道进出；某一口换真渠道 Intake 的同一笔撤下该口的隔离放行，最后一口换完删两个隔离开关；真渠道落地前新口仍可照 ADR-0091 逐口放行。
```

### ADR-0151：未归族的命令口定族——委托侧五个运营决定口与 TF 在管理台上作决定或判断的各口归操作者渠道新增的「运营决定」能力面，主链事实的更正口随被更正事实的族，发布词表读口随商业发布那一族

处理：来向两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-24，用户经 IDP 队列答通道 4 的立票提议「同意你的决定，开干，开干前，全面审查，确保确实如此」；通道 4 起草，2026-09-25 通道 2 按「全面审查」逐条对代码与文档复核后改定（通道 1 独立复核补了 TF 两个判断口），复核记录见 [operator-channel/15](../../.scratch/operator-channel/issues/15-operation-decision-faces-take-operator-intake.md) Comments。承 [operator-channel/08](../../.scratch/operator-channel/issues/08-isolated-release-of-main-chain-command-faces.md) 逐口归类表里「未归」「同族未列」「族归属待定」三行，以及未入那张表的发布词表读口。裁决能力边界：读过 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)、[ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)、[ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 全文，[ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md) 决定五，[ADR-0132](./0132-authorized-disposition-decides-the-destination-of-a-restricted-request-as-its-own-wait-state-and-never-passes.md) 决定一；PS CONTEXT 受控关闭的定义，PC CONTEXT 合同委派与授权动作两段；委托侧五口的 HTTP 适配器契约、应用命令（`FormControlledClosureCommand`、`FormReopeningCommand`、`CompleteManualReviewCommand`、`RejectShipmentRequestCommand`、`DisposeShipmentRequestCommand`）与 `cmd/parcel-api` 复核装配的注释；TF 四个管理台写面与两个判断口的 Intake 契约、票 tf-segment-lifecycle-closure/07 的定性表；票 operator-channel 03 / 05 / 06 / 08 / 10 / 13 / 14 与 product-strategy-boundary/07。没读：ADR-0126、相关 `UC-*` 与 TF CONTEXT 全文。拿不准的列在文末「越权风险点」。）；**部分停用** ADR-0149 决定一列举里「派送任务」「段关闭」「有效时间判断」三项对管理台三口的适用，见决定六；**部分停用**：决定二受控关闭与重开一项「不进命令的任何一格」半句已由 [ADR-0187](./0187-controlled-closure-and-reopening-record-request-and-operation-evidence-on-the-decision.md) 决定七停用，适用场景只限受控关闭口与重开口；**部分停用**：决定六末句里派送发起口「与 ADR-0149 其余各条不变」那一项已由 [ADR-0230](./0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md) 决定七停用，补拍口改挂运营决定，适用场景只限那一口（被停原句见索引注记）；**部分停用**：决定四「外部资金事实更正随原事实归集成客户端族（operator-channel/11）」一句已由 [ADR-0236](./0236-operators-adopt-external-funds-facts-and-regulatory-credentials-through-operator-decision-faces.md) 决定六停用，适用场景只限手工更正口<!-- 索引注记 -->｜**定族判据**：看提交方是谁、授予维度是什么——运营决定由管理台运营人员作出、授予按决定种类，既不是登记册配置写（授予按登记册）也不是作业事实（要设备）；更正与首登提交方相同，认证与授予同一份。**适用场景**：委托侧复核完成、主动拒绝、授权处置、受控关闭、受控重开五口与 TF 关段、建派送任务、装载分配、终止参与、有效时间判断、承运商首次有效收寄判断六口换操作者 Intake（operator-channel/15）；身份格只从信封与授权答复来——复核人、拒绝决定人、处置人取提交操作者，受控关闭与重开的决定方由授权答复给出、提交操作者只作操作证据，委托来源身份由服务端按租户与委托查出；运营决定形成生产事实，ADR-0055 决定五两项照 ADR-0149 决定四适用；交接、揽收、POD 更正归作业事实登记能力面（10），外部资金事实更正归集成客户端族（11）；发布词表读口归 06。客户侧各口仍归 ADR-0139。部分停用 ADR-0149 决定一「派送任务」「段关闭」「有效时间判断」三项对管理台三口的适用。2026-09-24 用户「同意你的决定，开干」，2026-09-25 按用户「全面审查」复核改定，越权风险点待 PS / PC / TF owner 复核｜**部分停用**：决定二受控关闭与重开一项「提交操作者只作操作证据，不进命令的任何一格」中「不进命令的任何一格」半句已由 [ADR-0187](./0187-controlled-closure-and-reopening-record-request-and-operation-evidence-on-the-decision.md) 决定七停用——提交操作者进且只进操作证据一格。**适用场景**：只限受控关闭口 `/shipment-requests/continued-attempt-closures` 与重开口 `/shipment-requests/continued-attempt-reopenings`，理由是 PS CONTEXT 把操作证据写进这两种决定的定义；「只作操作证据」与同项其余各句不变，TF 各口要不要记提交操作者仍归 TF owner，其余各条不变｜**部分停用**（2026-10-21）：决定六末句「派送发起口 `/transport-fulfillment-delivery-dispatch-triggers`（内部触发执行器的按拍入口，与手工建任务是两件事）与 ADR-0149 其余各条不变」里派送发起口那一项已由 [ADR-0230](./0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md) 决定七停用——它改挂「运营决定」能力面，决定种类「派送重问」与按派送待形成册批量重问共用；越权风险点 7 的前提「调用方是调度拍」随 ADR-0216 改成进程内那一拍，再发起的人是管理台上的调度员。**适用场景**：只限这一口；「与手工建任务是两件事」与 ADR-0149 其余各条不变，其余各条不变｜**部分停用**：决定四「外部资金事实更正随原事实归集成客户端族（operator-channel/11）」一句已由 [ADR-0236](./0236-operators-adopt-external-funds-facts-and-regulatory-credentials-through-operator-decision-faces.md) 决定六停用——外部资金事实的更正也可由持「外部资金事实采用」授予的操作者经手工更正口提交，且不绑原采用族。**适用场景**：只限 `/settlement-external-funds-fact-correction-manual-registrations` 一口；交接、揽收、POD 更正与决定四其余各句不变，其余各条不变
```

合并后：

```text
Status: Accepted（2026-09-24，用户经 IDP 队列答通道 4 的立票提议「同意你的决定，开干，开干前，全面审查，确保确实如此」；通道 4 起草，2026-09-25 通道 2 按「全面审查」逐条对代码与文档复核后改定（通道 1 独立复核补了 TF 两个判断口），复核记录见 [operator-channel/15](../../.scratch/operator-channel/issues/15-operation-decision-faces-take-operator-intake.md) Comments。承 [operator-channel/08](../../.scratch/operator-channel/issues/08-isolated-release-of-main-chain-command-faces.md) 逐口归类表里「未归」「同族未列」「族归属待定」三行，以及未入那张表的发布词表读口。裁决能力边界：读过 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)、[ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)、[ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 全文，[ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md) 决定五，[ADR-0132](./0132-authorized-disposition-decides-the-destination-of-a-restricted-request-as-its-own-wait-state-and-never-passes.md) 决定一；PS CONTEXT 受控关闭的定义，PC CONTEXT 合同委派与授权动作两段；委托侧五口的 HTTP 适配器契约、应用命令（`FormControlledClosureCommand`、`FormReopeningCommand`、`CompleteManualReviewCommand`、`RejectShipmentRequestCommand`、`DisposeShipmentRequestCommand`）与 `cmd/parcel-api` 复核装配的注释；TF 四个管理台写面与两个判断口的 Intake 契约、票 tf-segment-lifecycle-closure/07 的定性表；票 operator-channel 03 / 05 / 06 / 08 / 10 / 13 / 14 与 product-strategy-boundary/07。没读：ADR-0126、相关 `UC-*` 与 TF CONTEXT 全文。拿不准的列在文末「越权风险点」，待 PS / PC / TF owner 复核。）；**部分停用** ADR-0149 决定一列举里「派送任务」「段关闭」「有效时间判断」三项对管理台三口的适用，见决定六）<!-- 索引注记 -->｜**定族判据**：看提交方是谁、授予维度是什么——运营决定由管理台运营人员作出、授予按决定种类，既不是登记册配置写（授予按登记册）也不是作业事实（要设备）；更正与首登提交方相同，认证与授予同一份。**适用场景**：委托侧复核完成、主动拒绝、授权处置、受控关闭、受控重开五口与 TF 关段、建派送任务、装载分配、终止参与、有效时间判断、承运商首次有效收寄判断六口换操作者 Intake（operator-channel/15）；身份格只从信封与授权答复来——复核人、拒绝决定人、处置人取提交操作者，受控关闭与重开的决定方由授权答复给出、提交操作者只作操作证据，委托来源身份由服务端按租户与委托查出；运营决定形成生产事实，ADR-0055 决定五两项照 ADR-0149 决定四适用；交接、揽收、POD 更正归作业事实登记能力面（10），外部资金事实更正归集成客户端族（11）；发布词表读口归 06。客户侧各口仍归 ADR-0139｜**部分停用**：决定二受控关闭与重开一项「提交操作者只作操作证据，不进命令的任何一格」中「不进命令的任何一格」半句已由 [ADR-0187](./0187-controlled-closure-and-reopening-record-request-and-operation-evidence-on-the-decision.md) 决定七停用——提交操作者进且只进操作证据一格。**适用场景**：只限受控关闭口 `/shipment-requests/continued-attempt-closures` 与重开口 `/shipment-requests/continued-attempt-reopenings`，理由是 PS CONTEXT 把操作证据写进这两种决定的定义；「只作操作证据」与同项其余各句不变，TF 各口要不要记提交操作者仍归 TF owner，其余各条不变｜**部分停用**（2026-10-21）：其决定六末句「派送发起口 `/transport-fulfillment-delivery-dispatch-triggers`（内部触发执行器的按拍入口，与手工建任务是两件事）与 ADR-0149 其余各条不变」里派送发起口那一项已由 [ADR-0230](./0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md) 决定七停用——它改挂「运营决定」能力面，决定种类「派送重问」与按派送待形成册批量重问共用；越权风险点 7 的前提「调用方是调度拍」随 ADR-0216 改成进程内那一拍，再发起的人是管理台上的调度员。**适用场景**：只限这一口；「与手工建任务是两件事」与 ADR-0149 其余各条不变，其余各条不变｜**部分停用**：决定四「外部资金事实更正随原事实归集成客户端族（operator-channel/11）」一句已由 [ADR-0236](./0236-operators-adopt-external-funds-facts-and-regulatory-credentials-through-operator-decision-faces.md) 决定六停用——外部资金事实的更正也可由持「外部资金事实采用」授予的操作者经手工更正口提交，且不绑原采用族。**适用场景**：只限 `/settlement-external-funds-fact-correction-manual-registrations` 一口；交接、揽收、POD 更正与决定四其余各句不变，其余各条不变
```

### ADR-0154：接管记录的写侧尚未裁——先定谁可以写、写之前要有什么；不填 PAR-GOV-05..07 的租户值

处理：措辞对齐（接受出处、去向的模糊重说，或引文改为逐字）

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-29 用户在本通道对四项都答「你自决」。通道 2 按已有的 `TakeOver` 选定，不新造前置，不填 `PAR-GOV-05..07`。写入口的实现不在本记录里。）<!-- 索引注记 -->｜**先开 CLI、HTTP 另票**：调用现成 `TakeOver`，不加暂停或既有区间前置，不与暂停、恢复、阶段评审互引；种子不造行。2026-09-29 用户授权自决。实现不在本记录。
```

合并后：

```text
Status: Accepted（2026-09-29 用户在本通道对四项都答「你自决」。通道 2 按已有的 `TakeOver` 选定，不新造前置，不填 `PAR-GOV-05..07`。写入口的实现不在本记录里。）<!-- 索引注记 -->｜**先开 CLI、HTTP 另票**：调用现成 `TakeOver`，不加暂停或既有区间前置，不与暂停、恢复、阶段评审互引；种子不造行。
```

### ADR-0155：对象级接管的查阅口开在既有治理登记册上——空册如实空；不造种子行；HTTP 写仍另票

处理：措辞对齐（接受出处、去向的模糊重说，或引文改为逐字）

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-29 用户在本通道对后续工作答「继续」。通道 2 按已开的登记命令把查阅口补上，不新开写面，不填 `PAR-GOV-05..07`。）<!-- 索引注记 -->｜**查阅口**：`register=takeover` 上列标量，盘点 jsonb 不上列；空册空数组。部分停用 ADR-0083 决定四的接管半句。2026-09-29 用户授权继续。
```

合并后：

```text
Status: Accepted（2026-09-29 用户在本通道对后续工作答「继续」。通道 2 按已开的登记命令把查阅口补上，不新开写面，不填 `PAR-GOV-05..07`。）<!-- 索引注记 -->｜**查阅口**：`register=takeover` 上列标量，盘点 jsonb 不上列；空册空数组。部分停用 ADR-0083 决定四的接管半句。
```

### ADR-0156：可达性资格的闭包标识从本轮已采用的商业解析回指

处理：补指针

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-30。本项是机制。[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定一：「机制与产品策略同属产品交付轨道，开发方现在就做。」通道 2 按 ADR-0064 的同一形补可达性这一条，不造解析号，不填租户截点。用户在本通道对「下一刀做 psb/16 第 1 项」的答复是检查队列，不是本记录的接受依据。）<!-- 索引注记 -->｜**回指**：解析标识随可达性命令带过，不进判断键，不另造映射。空引用仍是未形成。部分停用 ADR-0064 后果里可达性那一句与 Context 里「`ReachabilityClosureIdentity` 仍属实例半边」那一句。2026-09-30。本项是机制，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」，不是通道里的「继续」。
```

合并后：

```text
Status: Accepted（2026-09-30。本项是机制。[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定一：「机制与产品策略同属产品交付轨道，开发方现在就做。」通道 2 按 ADR-0064 的同一形补可达性这一条，不造解析号，不填租户截点。用户在本通道对「下一刀做 psb/16 第 1 项」的答复是检查队列，不是本记录的接受依据。另：决定一使 [ADR-0081](./0081-acceptance-judgment-is-envelope-driven.md) 决定六「可达性闭包标识」一格不再留 nil，见 ADR-0081 的 Status 行（指针 2026-10-21 补））<!-- 索引注记 -->｜**回指**：解析标识随可达性命令带过，不进判断键，不另造映射。空引用仍是未形成。部分停用 ADR-0064 后果里可达性那一句与 Context 里「`ReachabilityClosureIdentity` 仍属实例半边」那一句。
```

### ADR-0157：内置时点形态以参考配置发布，租户在时点语义格用引用选用

处理：补指针

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-30 用户授权通道 1 自决：「你是业务和系统专家，你能帮我来做决策吗」。通道 2 起草。裁决原文在票 `product-strategy-boundary/06` Comments「裁决 ← 通道 1」。）<!-- 索引注记 -->｜**形态选择格**：不是 ADR-0147 的依据格；带 `REFCFG-1:` 的值登记时须已发布，否则拒。2026-09-30 用户授权通道 1 自决。
```

合并后：

```text
Status: Accepted（2026-09-30 用户授权通道 1 自决：「你是业务和系统专家，你能帮我来做决策吗」。通道 2 起草。裁决原文在票 `product-strategy-boundary/06` Comments「裁决 ← 通道 1」。另：内置时点形态「提交接收」使 [ADR-0081](./0081-acceptance-judgment-is-envelope-driven.md) 决定六「时点取值源」一格不再留 nil，见 ADR-0081 的 Status 行（指针 2026-10-21 补））<!-- 索引注记 -->｜**形态选择格**：不是 ADR-0147 的依据格；带 `REFCFG-1:` 的值登记时须已发布，否则拒。
```

### ADR-0158：结算账户登记册是一行固定属性，接受前控制只查应收

处理：措辞对齐（接受出处、去向的模糊重说，或引文改为逐字）

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-30。本项是机制，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」。）<!-- 索引注记 -->｜**登记册**：五格固定、绑结算政策对象不绑某一版、不设修订；部分停用 [ADR-0081](./0081-acceptance-judgment-is-envelope-driven.md) 决定六的结算账户目录留 nil。空册与没有相符应收行都停在 `CONTROL_SCOPE_NOT_CONFIGURED`。2026-09-30。本项是机制，接受依据是 ADR-0146 决定一。
```

合并后：

```text
Status: Accepted（2026-09-30。本项是机制，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」。）<!-- 索引注记 -->｜**登记册**：五格固定、绑结算政策对象不绑某一版、不设修订；部分停用 [ADR-0081](./0081-acceptance-judgment-is-envelope-driven.md) 决定六的结算账户目录留 nil。空册与没有相符应收行都停在 `CONTROL_SCOPE_NOT_CONFIGURED`。
```

### ADR-0159：面单择优的接受时解析回指从接受决定上读

处理：措辞对齐（接受出处、去向的模糊重说，或引文改为逐字）

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-30。本项是机制。[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定一：「机制与产品策略同属产品交付轨道，开发方现在就做。」通道 2 补面单择优这一条，不造解析号，不填租户截点。）<!-- 索引注记 -->｜**回指**：择优查询带来源身份与声明包裹，解析标识从该成员的接受决定读，不另造映射。空身份或未接受仍是未形成。2026-09-30。本项是机制，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」。
```

合并后：

```text
Status: Accepted（2026-09-30。本项是机制。[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定一：「机制与产品策略同属产品交付轨道，开发方现在就做。」通道 2 补面单择优这一条，不造解析号，不填租户截点。）<!-- 索引注记 -->｜**回指**：择优查询带来源身份与声明包裹，解析标识从该成员的接受决定读，不另造映射。空身份或未接受仍是未形成。
```

### ADR-0160：四本结算读口各有登记册，空册仍答未配置

处理：措辞对齐（接受出处、去向的模糊重说，或引文改为逐字）

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-30。本项是机制，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」。）<!-- 索引注记 -->｜**登记册**：审核授权、供应商应付账户查问、确认事实、金额规则版本各一本，不设修订。空册仍按各口已有语义停住。2026-09-30。本项是机制，接受依据是 ADR-0146 决定一。
```

合并后：

```text
Status: Accepted（2026-09-30。本项是机制，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」。）<!-- 索引注记 -->｜**登记册**：审核授权、供应商应付账户查问、确认事实、金额规则版本各一本，不设修订。空册仍按各口已有语义停住。
```

### ADR-0161：赔付、退款与代垫回收用同一套金额文法

处理：措辞对齐（接受出处、去向的模糊重说，或引文改为逐字）

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」与决定二的分界检验：不看任何一个租户的合同，就能回答限额、比例、免赔怎样组成一个金额。）<!-- 索引注记 -->｜**文法**：先免赔，再按万分比向下取整，再以限额封顶。三项数值另册登记，空册不形成金额。2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一、决定二。
```

合并后：

```text
Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」与决定二的分界检验：不看任何一个租户的合同，就能回答限额、比例、免赔怎样组成一个金额。）<!-- 索引注记 -->｜**文法**：先免赔，再按万分比向下取整，再以限额封顶。三项数值另册登记，空册不形成金额。
```

### ADR-0162：成本分摊的内置分法是按重、按件、按收入，尾差按最大余数

处理：措辞对齐（接受出处、去向的模糊重说，或引文改为逐字）

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」与决定二的分界检验：不看任何一个租户的合同，就能回答按重、按件、按收入怎样把一笔来源金额分成份额。）<!-- 索引注记 -->｜**分法**：三套共用最大余数，余数相同按目标标识升序。选用另册登记，空册不算出份额。2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一、决定二。
```

合并后：

```text
Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」与决定二的分界检验：不看任何一个租户的合同，就能回答按重、按件、按收入怎样把一笔来源金额分成份额。）<!-- 索引注记 -->｜**分法**：三套共用最大余数，余数相同按目标标识升序。选用另册登记，空册不算出份额。
```

### ADR-0163：供应商审核越权升级只比已匹配金额与已登记上限

处理：措辞对齐（接受出处、去向的模糊重说，或引文改为逐字）

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」与决定二的分界检验：不看任何一个租户的组织，就能回答一笔已匹配金额相对权限上限该不该升级。）<!-- 索引注记 -->｜**升级**：小于或等于上限在权限内，大于上限必须升级。上限另册登记，空册不形成应付。2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一、决定二。
```

合并后：

```text
Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」与决定二的分界检验：不看任何一个租户的组织，就能回答一笔已匹配金额相对权限上限该不该升级。）<!-- 索引注记 -->｜**升级**：小于或等于上限在权限内，大于上限必须升级。上限另册登记，空册不形成应付。
```

### ADR-0164：BUY 评价请求的触发面是发生项形成，原因没登记就不发起

处理：来向两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」与决定二的分界检验：不看任何一个租户的合同，就能回答发生项形成时怎样决定要不要发起一次 BUY 评价请求。**部分停用**：决定一末句「不从运输履约登记册推导三件来源引用」已由 [ADR-0208](./0208-buy-evaluation-is-requested-from-the-charge-occurrence-formed-envelope.md) 决定二在发生项已形成信封那一路停用，其余各条不变。）<!-- 索引注记 -->｜**触发面**：只在已登记的发生项原因上发起。空册不发起请求。2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一、决定二。｜**部分停用**：其决定一末句「不从运输履约登记册推导三件来源引用」已由 [ADR-0208](./0208-buy-evaluation-is-requested-from-the-charge-occurrence-formed-envelope.md) 决定二停用。**适用场景**：仅限发生项已形成信封那一路——停用的理由是按引用回读的是那一份发生项自己固定的事实（协议是发生项形成时固定下来的来源事实），不是按当前伙伴、线路或报表分组去配；由调用方交齐三件引用的触发面照旧不读运输履约。其余各条不变
```

合并后：

```text
Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」与决定二的分界检验：不看任何一个租户的合同，就能回答发生项形成时怎样决定要不要发起一次 BUY 评价请求。）<!-- 索引注记 -->｜**触发面**：只在已登记的发生项原因上发起。空册不发起请求｜**部分停用**：其决定一末句「不从运输履约登记册推导三件来源引用」已由 [ADR-0208](./0208-buy-evaluation-is-requested-from-the-charge-occurrence-formed-envelope.md) 决定二停用。**适用场景**：仅限发生项已形成信封那一路——停用的理由是按引用回读的是那一份发生项自己固定的事实（协议是发生项形成时固定下来的来源事实），不是按当前伙伴、线路或报表分组去配；由调用方交齐三件引用的触发面照旧不读运输履约。其余各条不变
```

### ADR-0165：周期费用的计算形态是最低消费、保底量、阶梯返利

处理：措辞对齐（接受出处、去向的模糊重说，或引文改为逐字）

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」与决定二的分界检验：不看任何一个租户的合同，就能回答最低消费、保底量、阶梯返利怎样从周期实绩收成一笔金额。）<!-- 索引注记 -->｜**形态**：补差、不足数量乘单价、分档返利。数值另册登记，空册不形成周期费用。2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一、决定二。
```

合并后：

```text
Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」与决定二的分界检验：不看任何一个租户的合同，就能回答最低消费、保底量、阶梯返利怎样从周期实绩收成一笔金额。）<!-- 索引注记 -->｜**形态**：补差、不足数量乘单价、分档返利。数值另册登记，空册不形成周期费用。
```

### ADR-0166：账单与财务交换的内置连接器形态是规范文书，没登记就不交换

处理：措辞对齐（接受出处、去向的模糊重说，或引文改为逐字）

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」与决定二的分界检验：不看任何一个租户的供应商或财务系统，就能回答外部账单和资金事实以什么形态进产品。）<!-- 索引注记 -->｜**连接器**：只认已经是产品主张或事实的载荷。空册答未配置。2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一、决定二。
```

合并后：

```text
Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」与决定二的分界检验：不看任何一个租户的供应商或财务系统，就能回答外部账单和资金事实以什么形态进产品。）<!-- 索引注记 -->｜**连接器**：只认已经是产品主张或事实的载荷。空册答未配置。
```

### ADR-0168：费用归属日按已登记形态判定，没登记就不形成

处理：措辞对齐（接受出处、去向的模糊重说，或引文改为逐字）

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」与决定二的分界检验：不看任何一个租户的合同，就能回答一个已点名的时点怎样落成归属日。）<!-- 索引注记 -->｜**归属日**：来源发生或费用确认，达到截单时刻归到下一日。空册不形成归属日。2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一、决定二。
```

合并后：

```text
Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」与决定二的分界检验：不看任何一个租户的合同，就能回答一个已点名的时点怎样落成归属日。）<!-- 索引注记 -->｜**归属日**：来源发生或费用确认，达到截单时刻归到下一日。空册不形成归属日。
```

### ADR-0169：SELL 评价形成客户费用，何时发起另册登记

处理：来向两写合并；补指针

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-30。消费门与形成编排是机制，触发时点是产品策略。接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」。部分停用：决定四「信封上还没有费用项目时，消费门指名缺这一引用」在包裹计费时点那一路由 [ADR-0198](./0198-sell-request-conclusions-on-the-parcel-billing-moment-path.md) 决定三停用。）<!-- 索引注记 -->｜**部分被 ADR-0190 取代**（「不进派发」那几句）；**部分停用**：决定四「信封上还没有费用项目时，消费门指名缺这一引用」已由 [ADR-0198](./0198-sell-request-conclusions-on-the-parcel-billing-moment-path.md) 决定三停用，**适用场景**仅限包裹计费时点那一路（回指 SELL 请求的评价），其余各条不变。**消费门**：只认 SELL·客户费用。触发空册不发起。不使用 BUY 请求身份。2026-09-30。本项是机制加产品策略，接受依据是 ADR-0146 决定一。
```

合并后：

```text
Status: Accepted（2026-09-30。消费门与形成编排是机制，触发时点是产品策略。接受依据是 ADR-0146 决定一「机制与产品策略同属产品交付轨道，开发方现在就做」。）<!-- 索引注记 -->｜**部分停用**：其 Consequences「形成编排不进 `cmd/parcel-dispatch`」与决定四「也不在本记录里把形成编排接进进程」两句已由 [ADR-0190](./0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 决定六停用——SELL 时点发起与形成两只编排都住在 `cmd/parcel-dispatch`、由派发器驱动；本记录的消费门、形成编排与发生项原因触发册不动，与 ADR-0190 的包裹时点册并存，其余各条不变。**部分停用**（指针 2026-10-21 补）：其决定三「内置时点只有发生项形成」一句已由 [ADR-0190](./0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 决定二扩成——另内置两种包裹计费时点，经包裹时点册发起。**适用场景**：本记录的发生项原因触发册那一路照旧只有发生项形成一种时点；两路是否互斥、会不会对同一件包裹形成两笔预估客户费用，由 ADR-0190 越权风险点 4 留给后继记录裁，其余各条不变。**部分停用**：决定四「信封上还没有费用项目时，消费门指名缺这一引用」已由 [ADR-0198](./0198-sell-request-conclusions-on-the-parcel-billing-moment-path.md) 决定三停用，**适用场景**仅限包裹计费时点那一路（回指 SELL 请求的评价），其余各条不变。**消费门**：只认 SELL·客户费用。触发空册不发起。不使用 BUY 请求身份。
```

### ADR-0170：结算编排从 parcel-api 进入；确认与截单没登记就不做

处理：措辞对齐（接受出处、去向的模糊重说，或引文改为逐字）

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-30。本项里装配与入口是机制，确认与截单的触发是产品策略。接受依据是 ADR-0146 决定一。）<!-- 索引注记 -->｜**部分被 ADR-0190 取代**（「不进派发」那几句）。**入口**：六个构造接在 parcel-api。审核在接收编排上。确认与截单空册不调用。2026-09-30。装配是机制，触发是产品策略，接受依据是 ADR-0146 决定一。
```

合并后：

```text
Status: Accepted（2026-09-30。本项里装配与入口是机制，确认与截单的触发是产品策略。接受依据是 ADR-0146 决定一。）<!-- 索引注记 -->｜**部分停用**：其 Context「SELL 评价形成客户费用的编排不进 `cmd/parcel-dispatch`」一句与 Alternatives 里否决「把 SELL 形成编排接进派发」那一条已由 [ADR-0190](./0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 决定六停用——SELL 时点发起与形成两只编排都住在 `cmd/parcel-dispatch`、由派发器驱动；其余各条不变。**入口**：六个构造接在 parcel-api。审核在接收编排上。确认与截单空册不调用。
```

### ADR-0171：计价输入在没有实测读口时用申报，并标明来源

处理：措辞对齐（接受出处、去向的模糊重说，或引文改为逐字）

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-30。本项是机制。接受依据是 ADR-0146 决定一。）<!-- 索引注记 -->｜**输入**：发生项成员、申报测量、地址要素三口接上。没有实测时用申报。集运单元不可得。2026-09-30。本项是机制，接受依据是 ADR-0146 决定一。
```

合并后：

```text
Status: Accepted（2026-09-30。本项是机制。接受依据是 ADR-0146 决定一。）<!-- 索引注记 -->｜**输入**：发生项成员、申报测量、地址要素三口接上。没有实测时用申报。集运单元不可得。
```

### ADR-0172：冻结边界的首版形态是剩余计划段数，没声明就不是未冻结

处理：措辞对齐（接受出处、去向的模糊重说，或引文改为逐字）

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定二、决定七：判断形态归产品，租户选形态、填取值。）<!-- 索引注记 -->｜**冻结**：剩余段数不超过登记值即越过边界。未声明答未配置。2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定二、决定七。
```

合并后：

```text
Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定二、决定七：判断形态归产品，租户选形态、填取值。）<!-- 索引注记 -->｜**冻结**：剩余段数不超过登记值即越过边界。未声明答未配置。
```

### ADR-0173：自动改路由策略版本折出，事实目录不再是判断权威

处理：措辞对齐（接受出处、去向的模糊重说，或引文改为逐字）

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定二、决定七：判断形态归产品，租户选形态、填取值。）<!-- 索引注记 -->｜**自动改路**：成本改善严格大于已登记阈值才自动切换。未声明只形成建议。2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定二、决定七。
```

合并后：

```text
Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定二、决定七：判断形态归产品，租户选形态、填取值。）<!-- 索引注记 -->｜**自动改路**：成本改善严格大于已登记阈值才自动切换。未声明只形成建议。
```

### ADR-0174：改路与装载并发按业务时间裁决，同一时刻和因果不明都不并进前两格

处理：措辞对齐（接受出处、去向的模糊重说，或引文改为逐字）

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定二、决定七：判断方法归产品。）<!-- 索引注记 -->｜**并发**：只比权威业务时间与生效边界。同一时刻与因果不明都停住。2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定二、决定七。
```

合并后：

```text
Status: Accepted（2026-09-30。本项是产品策略，接受依据是 ADR-0146 决定二、决定七：判断方法归产品。）<!-- 索引注记 -->｜**并发**：只比权威业务时间与生效边界。同一时刻与因果不明都停住。
```

### ADR-0175：服务日历的截单、处理时长与衔接缓冲按绝对时刻折进时间投影

处理：来向两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-09-30。本项是机制。接受依据是 ADR-0146 决定一、决定七。）部分停用：[ADR-0217](./0217-route-plan-carries-a-delivery-leg-after-the-delivery-node.md)（被停条款：决定一「三格都是分钟，都可空」里的「三格」，节点日历多一格派送时长；决定四「线路的连接序列是候选段链」，交付节点登了派送时长时连接序列之后接一段交付段；适用场景与原句见 adr/README 该行，其余各条不变，本记录正文不改写）<!-- 索引注记 -->｜**日历**：三格都是分钟，空着不补。跨节点用绝对时刻。2026-09-30。本项是机制，接受依据是 ADR-0146 决定一、决定七。｜**部分停用**：决定一「三格都是分钟，都可空」里的「三格」与决定四「线路的连接序列是候选段链」已由 [ADR-0217](./0217-route-plan-carries-a-delivery-leg-after-the-delivery-node.md) 停用，**适用场景**：前一句只限节点日历多出的派送时长一格（交付节点登了它，计划在交付节点之后带交付段），截单、处理时长、衔接缓冲的读法不变；后一句只限交付节点日历登了派送时长、且候选有交付范围时，连接序列之后接一段交付段，连接序列本身照旧是节点间段；自票 demo-chain-gaps/64 进 main 起生效；其余各条不变
```

合并后：

```text
Status: Accepted（2026-09-30。本项是机制。接受依据是 ADR-0146 决定一、决定七。）<!-- 索引注记 -->｜**日历**：三格都是分钟，空着不补。跨节点用绝对时刻｜**部分停用**：其决定一「三格都是分钟，都可空」里的「三格」与决定四「线路的连接序列是候选段链」已由 [ADR-0217](./0217-route-plan-carries-a-delivery-leg-after-the-delivery-node.md) 停用，**适用场景**：前一句只限节点日历多出的派送时长一格（交付节点登了它，计划在交付节点之后带交付段），截单、处理时长、衔接缓冲的读法不变；后一句只限交付节点日历登了派送时长、且候选有交付范围时，连接序列之后接一段交付段，连接序列本身照旧是节点间段；自票 demo-chain-gaps/64 进 main 起生效；其余各条不变，本记录正文不改写
```

### ADR-0177：客户合同正文指名它的货主客户账户，闭包按解析键上的客户账户收窄客户合同候选

处理：措辞对齐（接受出处、去向的模糊重说，或引文改为逐字）

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-12，用户经 IDP 队列对票 customer-service-rule-contract-applicability/07「分诊」节的推荐答「同意」；通道 1 起草）<!-- 索引注记 -->｜**归属**：合同正文必填客户账户，闭包只让指名键上那一户的合同参选；同一户同范围两份仍是冲突。规范化文档加必填键、PCC-1 摘要随之变，显式偏离 ADR-0176 决定五的 omitempty 先例（今天无生产数据）。2026-10-12。本项是机制，兑现 PC `CONTEXT`「合同版本必须明确关联货主客户账户」。
```

合并后：

```text
Status: Accepted（2026-10-12，用户经 IDP 队列对票 customer-service-rule-contract-applicability/07「分诊」节的推荐答「同意」；通道 1 起草。本项是机制，兑现 PC `CONTEXT`「合同版本必须明确关联货主客户账户」）<!-- 索引注记 -->｜**归属**：合同正文必填客户账户，闭包只让指名键上那一户的合同参选；同一户同范围两份仍是冲突。规范化文档加必填键、PCC-1 摘要随之变，显式偏离 ADR-0176 决定五的 omitempty 先例（今天无生产数据）。
```

### ADR-0178：客户合同这一册不收只有壳的版本；客户服务规则的壳指名客户合同时正文必须挂同一份合同；合同正文的货主客户账户是开放引用、不核在册

处理：措辞对齐（接受出处、去向的模糊重说，或引文改为逐字）

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-12，用户经 IDP 队列通道 2 授权「请你帮我自决」；通道 2 起草，裁定依据见票 contract-tier-binding-followups/01「分诊」）<!-- 索引注记 -->｜**归属**：货主客户账户只在合同正文里，只有壳的客户合同说不出属于哪一户，发布答`未受理`（改批文，与对账门同格）；选法不改，0037 之前的旧行随重灌消失。壳指名合同即合同层，正文挂产品的规则版本发布前与点读后都拒。合同正文的账户与其余参与方引用同一纪律、不核在册，悬空即对任何键不参选、不串户。部分停用 ADR-0126 决定二在客户合同一册的那一句。2026-10-12。本项是机制，兑现 PC `CONTEXT`「合同版本必须明确关联…货主客户账户」。
```

合并后：

```text
Status: Accepted（2026-10-12，用户经 IDP 队列通道 2 授权「请你帮我自决」；通道 2 起草，裁定依据见票 contract-tier-binding-followups/01「分诊」。本项是机制，兑现 PC `CONTEXT`「合同版本必须明确关联…货主客户账户」）<!-- 索引注记 -->｜**归属**：货主客户账户只在合同正文里，只有壳的客户合同说不出属于哪一户，发布答`未受理`（改批文，与对账门同格）；选法不改，0037 之前的旧行随重灌消失。壳指名合同即合同层，正文挂产品的规则版本发布前与点读后都拒。合同正文的账户与其余参与方引用同一纪律、不核在册，悬空即对任何键不参选、不串户。部分停用 ADR-0126 决定二在客户合同一册的那一句。
```

### ADR-0179：受理前控制金额由结算按委托当前提交版本的逐件 SELL 试算合计形成——估价归结算、试算归计价，选卡与正式 SELL 评价同口径，计价范围随采用登记给出，没登记答未配置

处理：接受出处两写合并；补指针

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-12 20:4x，用户经 IDP 队列授权通道 1 代裁接受，原话「你是业务和系统专家，你来做这个评估吧」；接受时改了决定二、决定六各一处，补了 ADR-0048、ADR 索引与 PP `CONTEXT` 的指针，接受方的读取边界与逐条理由见文末「接受记录」。起草：2026-10-12，通道 3 随票 `product-strategy-boundary/06` 第 2 项分诊，派单 `task-85d7350d` ← 通道 1。起草方裁决能力边界：读过 [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)、[ADR-0152](./0152-operator-estimate-is-a-pricing-use-case-computed-not-recorded-and-never-handed-to-settlement.md)、[ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md) 全文，ADR-0048、[ADR-0034](./0034-pricing-closure-adopts-via-price-policy.md)、[ADR-0169](./0169-sell-evaluation-forms-a-customer-charge.md) 的 Decision，[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「切片的机制半边与实例半边」与 PN-02 行及其补记，[参数登记册](../product/PILOT-PARAMETER-REGISTER.md) `PAR-COM-15` 行，PS、SA `CONTEXT` 里接受前财务控制与估价的词条和规则，PP `CONTEXT` 试算对象那条规则，[UC-SA-002](../application/settlement-accounting/UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md) 的输入表、主流程与接受前财务控制一节；代码读过 PS 的 `EstimationAmountSource`、`PreAcceptanceControlAdapter`、`AdvanceFinancialControlJudgmentHandler`，PP 的 `FormEstimateEvaluationsHandler`、`readingCompletion`、`FormEvaluationFromRequestHandler`、`PriceCardInForceResolver`，PS 的申报测量与地址要素读口头注，`cmd/parcel-dispatch` 的 `acceptanceFinancialControl`。没读：SA `ApplyPreAcceptanceControlHandler` 冻结与信用两支的金额用法全文、SA 冻结与信用暴露两张表的列、PC 价格政策发布翻译、PP `CONTEXT` 全文。拿不准的列在文末「越权风险点」。）<!-- 索引注记 -->｜**已接受**（2026-10-12，用户授权通道 1 代裁；接受时补了「合计币种对不上也答未形成」与留痕带版本清单两处，PP `CONTEXT` 补接受前估价例外句）；部分停用 ADR-0048 决定三第三增量的落点。**归属判据**：SA `CONTEXT` 拥有接受前估价、PS `CONTEXT` 不拥有金额；选卡、选区域、逐件合计一旦是方法就不能留在只翻译的适配器里。**适用场景**：首版形态「逐件 SELL 试算合计」，选卡与正式 SELL·客户费用评价同口径，分区按价卡分区目录与申报邮编，币种取控制作用域；采用登记键（租户、结算账户），空册停在金额未配置，登记发续办信封；估价随控制结果留痕。2026-10-12，票 product-strategy-boundary/06 第 2 项。
```

合并后：

```text
Status: Accepted（2026-10-12 20:4x，用户经 IDP 队列授权通道 1 代裁接受，原话「你是业务和系统专家，你来做这个评估吧」；接受时改了决定二、决定六各一处（索引注记原记作：补了「合计币种对不上也答未形成」与留痕带版本清单两处），补了 ADR-0048、ADR 索引与 PP `CONTEXT` 的指针（PP `CONTEXT` 补接受前估价例外句），接受方的读取边界与逐条理由见文末「接受记录」。起草：2026-10-12，通道 3 随票 `product-strategy-boundary/06` 第 2 项分诊，派单 `task-85d7350d` ← 通道 1。起草方裁决能力边界：读过 [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)、[ADR-0152](./0152-operator-estimate-is-a-pricing-use-case-computed-not-recorded-and-never-handed-to-settlement.md)、[ADR-0094](./0094-undecided-retry-is-decided-by-resume-path-with-a-fourth-grade-for-operator-registration.md) 全文，ADR-0048、[ADR-0034](./0034-pricing-closure-adopts-via-price-policy.md)、[ADR-0169](./0169-sell-evaluation-forms-a-customer-charge.md) 的 Decision，[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「切片的机制半边与实例半边」与 PN-02 行及其补记，[参数登记册](../product/PILOT-PARAMETER-REGISTER.md) `PAR-COM-15` 行，PS、SA `CONTEXT` 里接受前财务控制与估价的词条和规则，PP `CONTEXT` 试算对象那条规则，[UC-SA-002](../application/settlement-accounting/UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md) 的输入表、主流程与接受前财务控制一节；代码读过 PS 的 `EstimationAmountSource`、`PreAcceptanceControlAdapter`、`AdvanceFinancialControlJudgmentHandler`，PP 的 `FormEstimateEvaluationsHandler`、`readingCompletion`、`FormEvaluationFromRequestHandler`、`PriceCardInForceResolver`，PS 的申报测量与地址要素读口头注，`cmd/parcel-dispatch` 的 `acceptanceFinancialControl`。没读：SA `ApplyPreAcceptanceControlHandler` 冻结与信用两支的金额用法全文、SA 冻结与信用暴露两张表的列、PC 价格政策发布翻译、PP `CONTEXT` 全文。拿不准的列在文末「越权风险点」。部分停用 ADR-0048 决定三第三增量的落点；另，决定一使 [ADR-0081](./0081-acceptance-judgment-is-envelope-driven.md) 决定六的控制金额源一格不再留 nil（指针 2026-10-21 补）。）<!-- 索引注记 -->｜**归属判据**：SA `CONTEXT` 拥有接受前估价、PS `CONTEXT` 不拥有金额；选卡、选区域、逐件合计一旦是方法就不能留在只翻译的适配器里。**适用场景**：首版形态「逐件 SELL 试算合计」，选卡与正式 SELL·客户费用评价同口径，分区按价卡分区目录与申报邮编，币种取控制作用域；采用登记键（租户、结算账户），空册停在金额未配置，登记发续办信封；估价随控制结果留痕。
```

### ADR-0180：准入范围的租户对照是接入身份的一本册——按（租户、能力、事实类型）对至多一个对象范围，带生效区间与依据；本产品在治理登记册里的权威串是 `parcel-api` 的一件部署形态参数，准入范围桥与 parcel-shipment 生产归属同读这一处；登记册上的能力与事实类型词表归产品策略

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-13 22:5x，用户经 IDP 队列授权通道 3 自决，原话「你是系统和业务专家，你来自决吧」；接受时不改决定原文，接受方的读取边界与逐条理由见文末「接受记录」。此前 Proposed——**草案**，2026-10-13 起草：通道 2 随票 [operator-channel/19](../../.scratch/operator-channel/issues/19-admission-scope-bridge-for-operator-and-integration-client-minters.md) 分诊，派单 `task-f2fa6dc4` ← 通道 1。**接受与否归用户**，接受前不是依据；草案期间 ADR-0083、ADR-0149 一字不动。起草方裁决能力边界：读过[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「产品就绪与试点就绪」「切片的机制半边与实例半边」两节，[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)、[ADR-0083](./0083-pilot-governance-read-face-carries-registry-dimensions-only.md)、[ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)、[ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 全文，[ADR-0151](./0151-unassigned-command-faces-get-their-families.md) 的 Context 后半与决定一至五，[ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定二信任锚那一条与 Context 里信任锚那一句（按关键词取句，非全文）；票 operator-channel/14、19 全文，10 的做什么、完成判据与完成记录判断项，15 判断项 3 与进展记录里改挡点的那两句；[参数登记册](../product/PILOT-PARAMETER-REGISTER.md) `PAR-GOV-03` 整行，`PAR-INT-08..10` 与 `PAR-GOV-04..07` 各行的开头一段（截取，非整行）；`migrations/pilot_governance/0001` 抬头注，`migrations/access_identity/0001` 抬头注与授予、撤销两表的生效与撤销列（按列名取）、`0002` 的能力面约束、`0003` 抬头注与键。代码读过 `internal/accessidentity` 的 `AdmissionScope` 一族、`WorkFactMinter`、包注释、操作者能力面常量与 `DecisionKind`、`ExternalFactType` 两组词；`internal/pilotgovernance` 的 `AuthorityCoverage` 与领域 `AuthorityInterval`；parcel-shipment `adapters/pilotgovernance` 生产归属适配器的端口、依赖与 `governanceScope`；`cmd/parcel-api` 的 `buildOperatorMinter`、`buildIntegrationClientMinter`、`buildSubmissionOrchestration`、`buildProductionOwnership`、`buildIsolatedWriteAdmission` 与隔离形态的治理常量；各认证方取 `AdmissionRequirement` 的那一句；`internal/architecture` 的跨模块门禁；演示种子 `data/governance/` 各文件的对象范围、能力、事实类型与权威串（区间 01、02、03、07 读过全文，其余按键取），`seed.sh` 治理一段（按关键词）。2026-10-13 22:4x 通道 3 接手（通道 2 会话中断、用户转派）逐条复核引证，改正处记在票 19 分诊节末「接手核对」。没读：ADR-0025、ADR-0091、ADR-0100 全文，ADR-0078，`OperatorMinter` 与 `IntegrationClientMinter` 全文，`access_identity` 0002、0004 迁移，`cmd/parcel-access-register` 各子命令实现，`cmd/parcel-api` 各装配测试正文，`probe-operator-channel.sh`，CONTEXT-MAP 与 GLOSSARY 全文，`PILOT-SCOPE`，登记册其余各行。拿不准的列在文末「越权风险点」。）<!-- 索引注记 -->｜**已接受**（2026-10-13，用户授权通道 3 自决；接受时不改决定原文）；ADR-0083、ADR-0149 不需部分停用。**落点**：对照住 `access_identity`、经 `parcel-access-register` 登记，治理登记册照旧无租户维；同键任一时刻至多一个对象范围，一键多范围会越权放行。**参数**：权威串未设即与今天逐字节相同；PS 只跟参数来源，隔离分支不动。**词表**：`OPERATION_DECISION` + 决定种类、`EXTERNAL_FACT` + 外部事实类型是产品词，不带 `SYN-`。2026-10-13，票 operator-channel/19 分诊。
```

合并后：

```text
Status: Accepted（2026-10-13 22:5x，用户经 IDP 队列授权通道 3 自决，原话「你是系统和业务专家，你来自决吧」；接受时不改决定原文，接受方的读取边界与逐条理由见文末「接受记录」。此前 Proposed——**草案**，2026-10-13 起草：通道 2 随票 [operator-channel/19](../../.scratch/operator-channel/issues/19-admission-scope-bridge-for-operator-and-integration-client-minters.md) 分诊，派单 `task-f2fa6dc4` ← 通道 1。**接受与否归用户**，接受前不是依据；草案期间 ADR-0083、ADR-0149 一字不动。起草方裁决能力边界：读过[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「产品就绪与试点就绪」「切片的机制半边与实例半边」两节，[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)、[ADR-0083](./0083-pilot-governance-read-face-carries-registry-dimensions-only.md)、[ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)、[ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 全文，[ADR-0151](./0151-unassigned-command-faces-get-their-families.md) 的 Context 后半与决定一至五，[ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定二信任锚那一条与 Context 里信任锚那一句（按关键词取句，非全文）；票 operator-channel/14、19 全文，10 的做什么、完成判据与完成记录判断项，15 判断项 3 与进展记录里改挡点的那两句；[参数登记册](../product/PILOT-PARAMETER-REGISTER.md) `PAR-GOV-03` 整行，`PAR-INT-08..10` 与 `PAR-GOV-04..07` 各行的开头一段（截取，非整行）；`migrations/pilot_governance/0001` 抬头注，`migrations/access_identity/0001` 抬头注与授予、撤销两表的生效与撤销列（按列名取）、`0002` 的能力面约束、`0003` 抬头注与键。代码读过 `internal/accessidentity` 的 `AdmissionScope` 一族、`WorkFactMinter`、包注释、操作者能力面常量与 `DecisionKind`、`ExternalFactType` 两组词；`internal/pilotgovernance` 的 `AuthorityCoverage` 与领域 `AuthorityInterval`；parcel-shipment `adapters/pilotgovernance` 生产归属适配器的端口、依赖与 `governanceScope`；`cmd/parcel-api` 的 `buildOperatorMinter`、`buildIntegrationClientMinter`、`buildSubmissionOrchestration`、`buildProductionOwnership`、`buildIsolatedWriteAdmission` 与隔离形态的治理常量；各认证方取 `AdmissionRequirement` 的那一句；`internal/architecture` 的跨模块门禁；演示种子 `data/governance/` 各文件的对象范围、能力、事实类型与权威串（区间 01、02、03、07 读过全文，其余按键取），`seed.sh` 治理一段（按关键词）。2026-10-13 22:4x 通道 3 接手（通道 2 会话中断、用户转派）逐条复核引证，改正处记在票 19 分诊节末「接手核对」。没读：ADR-0025、ADR-0091、ADR-0100 全文，ADR-0078，`OperatorMinter` 与 `IntegrationClientMinter` 全文，`access_identity` 0002、0004 迁移，`cmd/parcel-access-register` 各子命令实现，`cmd/parcel-api` 各装配测试正文，`probe-operator-channel.sh`，CONTEXT-MAP 与 GLOSSARY 全文，`PILOT-SCOPE`，登记册其余各行。拿不准的列在文末「越权风险点」。接受后 ADR-0083、ADR-0149 不需部分停用。）<!-- 索引注记 -->｜**落点**：对照住 `access_identity`、经 `parcel-access-register` 登记，治理登记册照旧无租户维；同键任一时刻至多一个对象范围，一键多范围会越权放行。**参数**：权威串未设即与今天逐字节相同；PS 只跟参数来源，隔离分支不动。**词表**：`OPERATION_DECISION` + 决定种类、`EXTERNAL_FACT` + 外部事实类型是产品词，不带 `SYN-`。
```

### ADR-0181：入账余额在形成它的那次收付映射事务里按本上下文的账重算

处理：补指针

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-14，用户经 IDP 队列授权通道 1 自决，原话「你作为业务和系统专家，0181可以接受吗，你自决」。接受方的读取边界与逐条理由见文末「接受记录」。起草：2026-10-14，通道 2 随票 [product-strategy-boundary/29](../../.scratch/product-strategy-boundary/issues/29-operational-balance-and-credit-standing-have-no-production-writer.md) 分诊，派单 `task-4aaee735-bb59-4042-8ec2-06755e8087cd` ← 通道 1。起草方裁决能力边界：读过[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「切片的机制半边与实例半边」至 2026-10-13 补记、[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定一与决定二、[ADR-0007](./0007-separate-operational-settlement-from-statutory-finance.md) 的 Decision、[ADR-0047](./0047-terms-control-forms-credit-exposure-not-a-freeze.md) 的 Decision、[ADR-0127](./0127-credit-basis-is-handed-over-by-the-commercial-closure-resolution.md) 决定四、决定五与「登记额度与政策额度比对」那条否决、[ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定一、SA `CONTEXT`「运营余额、收付款与坏账」、[UC-SA-005](../application/settlement-accounting/UC-SA-005-MAP-EXTERNAL-FUNDS-AND-APPLY-SETTLEMENT.md) 的目的、步骤 5 与结果「运营余额已更新」、`migrations/settlement_accounting/0010_operational_position.sql` 头注、`BalancePosting` 头注与 `RecordBalance` 只准往前、`OperationalBalance.Available` 与 `Freeze` 用可用余额比较、`freezeFunds` / `exposeCredit`（额度取信用依据，无信用状况行即不可用）、演示种子 `SYN-FIN-CONTROL-01` 的两项控制与 `ALL_CONTROLS_PASS`。没读：UC-SA-002 全文、UC-SA-005 验收表全文、ADR-0129 全文、`MapExternalFundsHandler` 与确认费用编排的实现、通道 4 的 `mcp5-psb26`、合成演示动线全文、`bento-gate-reeval/01`。到期日只在 CONTEXT、GLOSSARY、SCENARIOS 与 UC-SA-005 的词里出现；`internal/settlementaccounting`、`internal/partycommercial` 与 `migrations/` 没有 `due_date` 字段。<!-- 索引注记 -->｜**求法**：入账余额是该作用域已映射客户方向收付净额减去已核销分配；未分配就是这个差，核销同时等额离开已确认未结。**写入**：与映射、核销、撤销同一事务重算，不投影、不采用外部余额快照；登记行上的授信额度列不承载额度。**首切**：只挂 UC-SA-005 的客户收款映射及撤销；信用状况行在到期日有处可读之前不写。2026-10-14 接受，票 product-strategy-boundary/29。
```

合并后：

```text
Status: Accepted（2026-10-14，用户经 IDP 队列授权通道 1 自决，原话「你作为业务和系统专家，0181可以接受吗，你自决」。接受方的读取边界与逐条理由见文末「接受记录」。起草：2026-10-14，通道 2 随票 [product-strategy-boundary/29](../../.scratch/product-strategy-boundary/issues/29-operational-balance-and-credit-standing-have-no-production-writer.md) 分诊，派单 `task-4aaee735-bb59-4042-8ec2-06755e8087cd` ← 通道 1。起草方裁决能力边界：读过[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「切片的机制半边与实例半边」至 2026-10-13 补记、[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定一与决定二、[ADR-0007](./0007-separate-operational-settlement-from-statutory-finance.md) 的 Decision、[ADR-0047](./0047-terms-control-forms-credit-exposure-not-a-freeze.md) 的 Decision、[ADR-0127](./0127-credit-basis-is-handed-over-by-the-commercial-closure-resolution.md) 决定四、决定五与「登记额度与政策额度比对」那条否决、[ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md) 决定一、SA `CONTEXT`「运营余额、收付款与坏账」、[UC-SA-005](../application/settlement-accounting/UC-SA-005-MAP-EXTERNAL-FUNDS-AND-APPLY-SETTLEMENT.md) 的目的、步骤 5 与结果「运营余额已更新」、`migrations/settlement_accounting/0010_operational_position.sql` 头注、`BalancePosting` 头注与 `RecordBalance` 只准往前、`OperationalBalance.Available` 与 `Freeze` 用可用余额比较、`freezeFunds` / `exposeCredit`（额度取信用依据，无信用状况行即不可用）、演示种子 `SYN-FIN-CONTROL-01` 的两项控制与 `ALL_CONTROLS_PASS`。没读：UC-SA-002 全文、UC-SA-005 验收表全文、ADR-0129 全文、`MapExternalFundsHandler` 与确认费用编排的实现、通道 4 的 `mcp5-psb26`、合成演示动线全文、`bento-gate-reeval/01`。到期日只在 CONTEXT、GLOSSARY、SCENARIOS 与 UC-SA-005 的词里出现；`internal/settlementaccounting`、`internal/partycommercial` 与 `migrations/` 没有 `due_date` 字段。<!-- 索引注记 -->｜**求法**：入账余额是该作用域已映射客户方向收付净额减去已核销分配；未分配就是这个差，核销同时等额离开已确认未结。**写入**：与映射、核销、撤销同一事务重算，不投影、不采用外部余额快照；登记行上的授信额度列不承载额度。**首切**：只挂 UC-SA-005 的客户收款映射及撤销；信用状况行在到期日有处可读之前不写｜**部分停用**（指针 2026-10-21 补）：其决定五「信用状况行在算得出逾期之前不写」与决定六「首切只挂决定一在客户收款映射及其撤销上的那一支」两句已由 [ADR-0182](./0182-receivable-account-is-a-funds-mapping-target.md) 收窄（它引作「到期日有处可读之前不写信用状况」「首切只挂对账单映射」）。**适用场景**：在 ADR-0182 决定一至三的条件下——应收结算账户成为第四种映射目标 `ACCOUNT`，第一张对账单之前的预收映射到账户；同一次账户级收款映射里重算信用状况，该作用域没有已确认费用则逾期为假并写行，已有已确认费用却缺到期日则这次不写。决定二的求法与决定五「无行仍是信用状况不可用，不读成未逾期」不变，其余各条不变
```

### ADR-0182：应收结算账户是客户收款的映射目标，账户级映射同时重算信用状况

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-14，用户原话「请你帮我彻底解决吧」。通道 1 据此裁定下面六条，接受方不改决定原文。派单 `task-0e5c89bc-35cc-43c3-8b97-418d2651370f` ← 通道 1，落在票 [product-strategy-boundary/26](../../.scratch/product-strategy-boundary/issues/26-demo-tenant-adopts-the-pre-acceptance-estimate-form.md) 的格 5。）<!-- 索引注记 -->｜**已接受**（2026-10-14，用户原话「请你帮我彻底解决吧」，通道 1 裁定六条决定）。**目标**：应收结算账户成为第四种映射目标；第一张对账单之前的预收映射到账户，不发空对账单，不直写余额。**信用状况**：没有已确认费用则逾期为假并写行，登记额度列插入 0 且不覆盖；缺到期日则这次不写。部分收窄 ADR-0181 决定五、决定六的首切边界。2026-10-14，票 product-strategy-boundary/26。
```

合并后：

```text
Status: Accepted（2026-10-14，用户原话「请你帮我彻底解决吧」。通道 1 据此裁定下面六条，接受方不改决定原文。派单 `task-0e5c89bc-35cc-43c3-8b97-418d2651370f` ← 通道 1，落在票 [product-strategy-boundary/26](../../.scratch/product-strategy-boundary/issues/26-demo-tenant-adopts-the-pre-acceptance-estimate-form.md) 的格 5。部分收窄 [ADR-0181](./0181-posted-balance-is-recomputed-in-the-funds-mapping-transaction.md) 决定五、决定六的首切边界。）<!-- 索引注记 -->｜**目标**：应收结算账户成为第四种映射目标；第一张对账单之前的预收映射到账户，不发空对账单，不直写余额。**信用状况**：没有已确认费用则逾期为假并写行，登记额度列插入 0 且不覆盖；缺到期日则这次不写。
```

### ADR-0183：操作者渠道上受控关闭与重开按包裹寻址，请求方与货主账户取包裹所属已接受委托的客户账户，不从载荷收

处理：来向两写合并；接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-16，用户原话「你作为业务和系统超级专家，你自己全部独立完成吧」。Claude Code 会话据此代 PS owner 裁定下面六条，落在票 [operator-channel/15](../../.scratch/operator-channel/issues/15-operation-decision-faces-take-operator-intake.md) 做什么第 3 条那一格。裁决能力边界：读过 [ADR-0151](./0151-unassigned-command-faces-get-their-families.md) 决定二、七与越权风险点 4，PS CONTEXT「面单继续尝试决定」的定义、Rules 里受控关闭与重开各条与「面单继续尝试」生命周期，`FormContinuedAttemptDecisionHandler` 两条命令与重开那道货主重授权门，`ports.ContinuedAttemptDecisionAuthorizationQuery` 头注，`adapters/partycommercial` 的关闭 / 重开授权适配器头注（pc-gaps/13 裁决 ③），`ports.CurrentAcceptedParcelTargetView`，PS 已换口三口的操作者 Intake。没读：pc-gaps/13 票面原文、label-channel/30 票面原文、UC-PS 各篇。拿不准的列在文末「越权风险点」。）；**部分停用**：决定五首句「两条命令没有提交操作者的格；本记录不给它们加格」已由 [ADR-0187](./0187-controlled-closure-and-reopening-record-request-and-operation-evidence-on-the-decision.md) 决定七停用，适用场景只限受控关闭口与重开口<!-- 索引注记 -->｜**已接受**（2026-10-16，用户原话「你作为业务和系统超级专家，你自己全部独立完成吧」，会话代 PS owner 裁定六条）；补上 ADR-0151 决定二留给 PS owner 的一格，不停用任何记录。**寻址**：载荷只指名包裹，服务端反查当前已接受委托取来源身份。**请求方**：只可能是那份委托的客户账户，写作 `CUSTOMER-ACCOUNT/<标识>`；关闭由责任来源推出（货主指令才有），重开随货主重授权证据出现，货主账户恒取委托客户。**授权**：照旧停在未决 · 授权口不可用。票 operator-channel/15。｜**部分停用**：决定五首句「两条命令没有提交操作者的格；本记录不给它们加格」已由 [ADR-0187](./0187-controlled-closure-and-reopening-record-request-and-operation-evidence-on-the-decision.md) 决定七停用。**适用场景**：只限受控关闭口与重开口；其余各条不变。「复核记录二」的比较格不变，由 ADR-0187 决定四补明请求证据与操作证据不在比较格内
```

合并后：

```text
Status: Accepted（2026-10-16，用户原话「你作为业务和系统超级专家，你自己全部独立完成吧」。Claude Code 会话据此代 PS owner 裁定下面六条，落在票 [operator-channel/15](../../.scratch/operator-channel/issues/15-operation-decision-faces-take-operator-intake.md) 做什么第 3 条那一格。裁决能力边界：读过 [ADR-0151](./0151-unassigned-command-faces-get-their-families.md) 决定二、七与越权风险点 4，PS CONTEXT「面单继续尝试决定」的定义、Rules 里受控关闭与重开各条与「面单继续尝试」生命周期，`FormContinuedAttemptDecisionHandler` 两条命令与重开那道货主重授权门，`ports.ContinuedAttemptDecisionAuthorizationQuery` 头注，`adapters/partycommercial` 的关闭 / 重开授权适配器头注（pc-gaps/13 裁决 ③），`ports.CurrentAcceptedParcelTargetView`，PS 已换口三口的操作者 Intake。没读：pc-gaps/13 票面原文、label-channel/30 票面原文、UC-PS 各篇。拿不准的列在文末「越权风险点」。补上 ADR-0151 决定二留给 PS owner 的一格，不停用任何记录。）<!-- 索引注记 -->｜**寻址**：载荷只指名包裹，服务端反查当前已接受委托取来源身份。**请求方**：只可能是那份委托的客户账户，写作 `CUSTOMER-ACCOUNT/<标识>`；关闭由责任来源推出（货主指令才有），重开随货主重授权证据出现，货主账户恒取委托客户。**授权**：照旧停在未决 · 授权口不可用｜**部分停用**：其决定五首句「两条命令没有提交操作者的格；本记录不给它们加格」已由 [ADR-0187](./0187-controlled-closure-and-reopening-record-request-and-operation-evidence-on-the-decision.md) 决定七停用。**适用场景**：只限受控关闭口与重开口；其余各条不变。「复核记录二」的比较格不变，由 ADR-0187 决定四补明请求证据与操作证据不在比较格内
```

### ADR-0184：外部标识关联自立一本只追加的登记册，随来源事实同笔形成——客户包裹件号随委托接受、渠道包裹件号随面单交易包裹结果受理；节点按号码逐字核对，只拿回此刻适用关联所标识的包裹

处理：头部停用行并入注记；接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-16，用户原话「这些需要确定的事情……你帮我做决定吧」与「开始」。Claude Code 会话据此代 PS owner 裁定下面七条，落在票 [ps-external-mark-relations/01](../../.scratch/ps-external-mark-relations/issues/01-external-mark-relations-have-no-model-in-parcel-shipment.md) 那一格。裁决能力边界：读过 PS CONTEXT「外部标识」「面单交易包裹结果」的定义、Rules 里外部标识两条与面单交易各条、`委托` 与 `包裹身份与服务` 两节生命周期，GLOSSARY「外部标识」「包裹内部标识」，CONTEXT-MAP `parcel-shipment → node-operations` 一条，UC-NO-002 的流程表、控制与身份一节和验收表；代码读过 `IsolatedSubmissionIntake` 的译装、`SubmitShipmentRequestHandler`、`ShipmentRequests.Save` 与 `FindCurrentAcceptedByParcel`、`LabelTransactionParcelResult`、NO 的 `ParcelIdentityView` 与两处 `unconfiguredParcelIdentityView`。**没读** UC-PS-001 全文与面单交易各编排的全部分支。）<!-- 索引注记 -->｜**已接受**（2026-10-16，用户原话「你帮我做决定吧」「开始」，Claude Code 会话代 PS owner 裁定七条）。**册**：键（租户，标识类型，分配方，号码，被标识对象），只插不改，适用变化另起一行、终态一次。**形成**：客户包裹件号随委托接受（已接受委托保存时同笔投影，按键幂等），渠道包裹件号随面单交易包裹结果受理；不开人工登记口。**核对**：按（租户，号码）逐字比对，只交回此刻适用、标识包裹的关联所指的包裹，多件即多候选。**越权风险点**已经 owner 复核（2026-10-19 代裁，见该记录文末）。｜**部分停用**：决定一「键（租户，标识类型，分配方，号码，被标识对象）」已由 [ADR-0185](./0185-external-identifier-relation-identity-includes-its-formation-basis.md) 停用——键加入形成依据；决定五「只交回包裹，不交分配方、客户账户或委托」里的「只交回包裹」已由 [ADR-0195](./0195-a-submitted-declaration-of-the-same-number-turns-a-unique-hit-into-an-identity-conflict.md) 停用——答复另带歧义信号，「不交分配方、客户账户或委托」照旧；决定五「NO 的三分支据此答身份冲突，不按形成时间、来源或分配方挑一件」与决定六「现场观察只带一个号码，不带范围」已由 [ADR-0212](./0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md) 停用，**适用场景**：前一句只限这件实物有可验证交付范围时（NO 按范围相容的候选认），后一句因收寄来源可带交付范围依据引用，决定六结论「核对时不按范围收窄」维持；自票 ps-external-mark-relations/11 进 main 起生效；其余各条不变
Partially suspended by: [ADR-0185](./0185-external-identifier-relation-identity-includes-its-formation-basis.md)（被停条款：决定一「键（租户，标识类型，分配方，号码，被标识对象）」——键加入形成依据；其余各条不变，本记录正文不改写）；[ADR-0195](./0195-a-submitted-declaration-of-the-same-number-turns-a-unique-hit-into-an-identity-conflict.md)（被停条款：决定五「只交回包裹，不交分配方、客户账户或委托」里的「只交回包裹」——答复另带歧义信号；「不交分配方、客户账户或委托」与其余各条不变，本记录正文不改写）；[ADR-0212](./0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md)（被停条款：决定五「NO 的三分支据此答身份冲突，不按形成时间、来源或分配方挑一件」，适用场景是这件实物有可验证交付范围时——NO 按范围相容的候选认；决定六「现场观察只带一个号码，不带范围」——收寄来源可带交付范围依据引用，结论「核对时不按范围收窄」维持；其余各条不变，本记录正文不改写）
```

合并后：

```text
Status: Accepted（2026-10-16，用户原话「这些需要确定的事情……你帮我做决定吧」与「开始」。Claude Code 会话据此代 PS owner 裁定下面七条，落在票 [ps-external-mark-relations/01](../../.scratch/ps-external-mark-relations/issues/01-external-mark-relations-have-no-model-in-parcel-shipment.md) 那一格。裁决能力边界：读过 PS CONTEXT「外部标识」「面单交易包裹结果」的定义、Rules 里外部标识两条与面单交易各条、`委托` 与 `包裹身份与服务` 两节生命周期，GLOSSARY「外部标识」「包裹内部标识」，CONTEXT-MAP `parcel-shipment → node-operations` 一条，UC-NO-002 的流程表、控制与身份一节和验收表；代码读过 `IsolatedSubmissionIntake` 的译装、`SubmitShipmentRequestHandler`、`ShipmentRequests.Save` 与 `FindCurrentAcceptedByParcel`、`LabelTransactionParcelResult`、NO 的 `ParcelIdentityView` 与两处 `unconfiguredParcelIdentityView`。**没读** UC-PS-001 全文与面单交易各编排的全部分支。）<!-- 索引注记 -->｜**册**：键（租户，标识类型，分配方，号码，被标识对象），只插不改，适用变化另起一行、终态一次。**形成**：客户包裹件号随委托接受（已接受委托保存时同笔投影，按键幂等），渠道包裹件号随面单交易包裹结果受理；不开人工登记口。**核对**：按（租户，号码）逐字比对，只交回此刻适用、标识包裹的关联所指的包裹，多件即多候选。**越权风险点**已经 owner 复核（2026-10-19 代裁，见该记录文末）。｜**部分停用**：决定一「键（租户，标识类型，分配方，号码，被标识对象）」已由 [ADR-0185](./0185-external-identifier-relation-identity-includes-its-formation-basis.md) 停用——键加入形成依据；决定五「只交回包裹，不交分配方、客户账户或委托」里的「只交回包裹」已由 [ADR-0195](./0195-a-submitted-declaration-of-the-same-number-turns-a-unique-hit-into-an-identity-conflict.md) 停用——答复另带歧义信号，「不交分配方、客户账户或委托」照旧；决定五「NO 的三分支据此答身份冲突，不按形成时间、来源或分配方挑一件」与决定六「现场观察只带一个号码，不带范围」已由 [ADR-0212](./0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md) 停用，**适用场景**：前一句只限这件实物有可验证交付范围时（NO 按范围相容的候选认），后一句因收寄来源可带交付范围依据引用，决定六结论「核对时不按范围收窄」维持；自票 ps-external-mark-relations/11 进 main 起生效；其余各条不变，本记录正文不改写
```

### ADR-0185：外部标识关联的身份含形成依据——同一分配方对同一对象再签同一个号是新的一条关联

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-16，部分停用 [ADR-0184](./0184-external-identifier-relations-are-an-append-only-register-formed-with-their-source-facts.md) 决定一的键，其余各条不变。由票 [ps-external-mark-relations/03](../../.scratch/ps-external-mark-relations/issues/03-channel-parcel-numbers-from-label-results-void-and-replacement.md) 的两轴评审揭出——Standards 轴报阻断、Spec 轴报非阻断，评审者是 `/code-review` 的隔离子代理（用户 2026-10-16 改裁允许子代理）。Claude Code 会话在用户「你帮我做决定吧」「开始」的同一授权下代 PS owner 裁定。）<!-- 索引注记 -->｜**已接受**（2026-10-16，部分停用 ADR-0184 决定一的键，由票 ps-external-mark-relations/03 两轴评审揭出）。**键**：（租户，标识类型，分配方，号码，被标识对象，形成依据）。换单沿用原号、作废后再签同号都是新关联；替代后继不得是被替代者自己。**越权风险点**已经 owner 复核（2026-10-19 代裁，见该记录文末）。
```

合并后：

```text
Status: Accepted（2026-10-16，部分停用 [ADR-0184](./0184-external-identifier-relations-are-an-append-only-register-formed-with-their-source-facts.md) 决定一的键，其余各条不变。由票 [ps-external-mark-relations/03](../../.scratch/ps-external-mark-relations/issues/03-channel-parcel-numbers-from-label-results-void-and-replacement.md) 的两轴评审揭出——Standards 轴报阻断、Spec 轴报非阻断，评审者是 `/code-review` 的隔离子代理（用户 2026-10-16 改裁允许子代理）。Claude Code 会话在用户「你帮我做决定吧」「开始」的同一授权下代 PS owner 裁定。）<!-- 索引注记 -->｜**键**：（租户，标识类型，分配方，号码，被标识对象，形成依据）。换单沿用原号、作废后再签同号都是新关联；替代后继不得是被替代者自己。**越权风险点**已经 owner 复核（2026-10-19 代裁，见该记录文末）。
```

### ADR-0186：节点收寄遇跨上下文读不动——服务结果读不动照常形成收寄并记「读不动」，身份核对读不动与本方落册失败不形成答案

处理：来向两写合并；接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-18。用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；该会话经隔离子代理研判、三份对抗核验（权威一致性、实现与回归、业务后果）与合成，代 NO owner 裁定，裁定原文落在票 [operator-channel/29](../../.scratch/operator-channel/issues/29-node-reception-records-physical-facts-when-cross-context-reads-fail.md)「裁定」一节，本记录落地后取代那一节。实施与成文由该会话起的实施子代理完成。裁决能力边界：读过 UC-NO-002 的结果语义契约与应用流程表、票 [operator-channel/25](../../.scratch/operator-channel/issues/25-node-reception-carries-parcel-shipment-service-result-markers.md) 的裁定与非作者评审、票 29 全文；代码读过 `ReceiveDeliveredUnitHandler`、NO 的 HTTP 收寄端点、前线导入的收寄子命令、NO 的收寄库 postgres 适配器、两处生产装配与 settlementaccounting 适配器的 SAVEPOINT 先例。**没读** NO `CONTEXT.md` 全文、ADR-0022/0023/0089 全文与 bento `WithinTransaction` 的源码；后者的行为以真库用例实测为准。）部分停用：决定二「身份核对读不动：不形成答案」与决定七「身份读口不隔离」两处由 [ADR-0199](./0199-node-reception-preserves-the-delivery-source-and-rechecks-pending-identities.md) 决定五停用，随票 no-reception-source/05 进 main 生效；被停的原句见本记录文末「复核记录（2026-10-19，ADR-0199 决定五部分停用两处）」与 adr/README 该行。<!-- 索引注记 -->｜**已接受**（2026-10-18，用户授权会话 idp-parcel-9b 代决，落在票 operator-channel/29；取代票 operator-channel/25 裁定里「读不动答`收寄待确认`、带续办引用、不落册」一句）。**服务结果读不动**：照常`节点收寄已形成`，记录带核对态「读不动」、标记为空，不补查、不发续办引用。**身份核对读不动与 NO 落册失败**：不形成答案（5xx / 前线导入记未决）；输入缺陷答 400 / 记被拒；构造期拒缺依赖。**隔离**：服务结果读口经 savepoint，身份读口不包。**标记**：只在 ports 一处合成，程序经读口现问、不解析串｜**部分停用**：决定二「身份核对读不动：不形成答案」里「编排交回 error；HTTP 答 500 `NO_ANSWER_FORMED`、不带 `outcome`，前线导入记未决。不落册」与决定七「身份读口不隔离」（连同理由「身份读口读不动本就不形成答案，整笔回滚正是要的结果」）已由 [ADR-0199](./0199-node-reception-preserves-the-delivery-source-and-rechecks-pending-identities.md) 决定五停用，自票 no-reception-source/05 进 main 起生效：身份核对读不动落`待识别`加身份核对态「未核对」、同笔自投重核门铃，身份读口经 savepoint；决定二「不拿空候选顶替」照旧，由核对态兑现；Consequences 里随决定二的「身份核对读不动现在答 5xx，设备留队重投」一句同停；其余各条不变
```

合并后：

```text
Status: Accepted（2026-10-18。用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；该会话经隔离子代理研判、三份对抗核验（权威一致性、实现与回归、业务后果）与合成，代 NO owner 裁定，裁定原文落在票 [operator-channel/29](../../.scratch/operator-channel/issues/29-node-reception-records-physical-facts-when-cross-context-reads-fail.md)「裁定」一节，本记录落地后取代那一节。实施与成文由该会话起的实施子代理完成。裁决能力边界：读过 UC-NO-002 的结果语义契约与应用流程表、票 [operator-channel/25](../../.scratch/operator-channel/issues/25-node-reception-carries-parcel-shipment-service-result-markers.md) 的裁定与非作者评审、票 29 全文；代码读过 `ReceiveDeliveredUnitHandler`、NO 的 HTTP 收寄端点、前线导入的收寄子命令、NO 的收寄库 postgres 适配器、两处生产装配与 settlementaccounting 适配器的 SAVEPOINT 先例。**没读** NO `CONTEXT.md` 全文、ADR-0022/0023/0089 全文与 bento `WithinTransaction` 的源码；后者的行为以真库用例实测为准。本记录取代票 operator-channel/25 裁定里「读不动答`收寄待确认`、带续办引用、不落册」一句。）<!-- 索引注记 -->｜**服务结果读不动**：照常`节点收寄已形成`，记录带核对态「读不动」、标记为空，不补查、不发续办引用。**身份核对读不动与 NO 落册失败**：不形成答案（5xx / 前线导入记未决）；输入缺陷答 400 / 记被拒；构造期拒缺依赖。**隔离**：服务结果读口经 savepoint，身份读口不包。**标记**：只在 ports 一处合成，程序经读口现问、不解析串｜**部分停用**：其决定二「身份核对读不动：不形成答案」里「编排交回 error；HTTP 答 500 `NO_ANSWER_FORMED`、不带 `outcome`，前线导入记未决。不落册」与决定七「身份读口不隔离」（连同理由「身份读口读不动本就不形成答案，整笔回滚正是要的结果」）已由 [ADR-0199](./0199-node-reception-preserves-the-delivery-source-and-rechecks-pending-identities.md) 决定五停用，自票 no-reception-source/05 进 main 起生效：身份核对读不动落`待识别`加身份核对态「未核对」、同笔自投重核门铃，身份读口经 savepoint；决定二「不拿空候选顶替」照旧，由核对态兑现；Consequences 里随决定二的「身份核对读不动现在答 5xx，设备留队重投」一句同停；其余各条不变；裁定经过见本记录文末「复核记录（2026-10-19，ADR-0199 决定五部分停用两处）」
```

### ADR-0187：受控关闭与重开把请求证据与操作证据落在决定上——请求证据与请求方同在同缺，提交操作者只进操作证据一格；部分停用 ADR-0183 决定五首句与 ADR-0151 决定二受控关闭与重开一项的半句

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19。用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；该会话经隔离子代理研判、对抗核验与合成（研判轮 review26-followups-decide，簇 C5 终裁），代 PS owner 裁定，裁定原文落在票 [operator-channel/38](../../.scratch/operator-channel/issues/38-controlled-closure-and-reopening-record-request-and-operation-evidence.md)「裁定」一节；实施与成文由该会话起的实施子代理完成。裁决能力边界：读过 ADR-0183 全文（含两份复核记录与 owner 复核行）、ADR-0151 全文、ADR-0028 全文、ADR-0146 的 Status 与决定一至三、adr/README 的部分停用机制；PS CONTEXT「面单继续尝试决定」的定义、Rules 里受控关闭与重开各条、Boundaries 相关句，GLOSSARY 同名词条，SCENARIOS 面单场景；pc-gaps/13 的「裁决」与「要裁的」，label-channel/30 的「裁决」与两次评审；受控关闭与重开的领域、编排、HTTP 适配器与快照适配器全文。没读：UC-PS 各篇正文，ADR-0029、0030、0100、0139 正文，PILOT-PARAMETER-REGISTER 与开发主线。拿不准的列在文末「越权风险点」。）；**部分停用** ADR-0183 决定五首句与 ADR-0151 决定二受控关闭与重开一项「不进命令的任何一格」半句，适用场景见决定七<!-- 索引注记 -->｜**已接受**（2026-10-19，用户授权会话 idp-parcel-9b 代 PS owner 裁定，落在票 operator-channel/38）。**请求证据**：请求方所凭依据的引用落在决定上，去两端空白逐字保存、不判真伪；形成时与请求方同在同缺（编排在问授权之前核），重建门只拒有证据无请求方，照收规则生效前的旧历史，不加守卫迁移。**操作证据**：提交操作者写作 `OPERATOR/<发行方>#<sub>`，只进审计，任何门与比对都不读它。**重放**：两份证据不进比对，回执带回册上的 `requestEvidence`。**载荷**：货主指令关闭必带 `shipperInstructionEvidence`，另两种来源带了即 400。部分停用 ADR-0183 决定五首句与 ADR-0151 决定二受控关闭与重开一项半句，只限受控关闭口与重开口
```

合并后：

```text
Status: Accepted（2026-10-19。用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；该会话经隔离子代理研判、对抗核验与合成（研判轮 review26-followups-decide，簇 C5 终裁），代 PS owner 裁定，裁定原文落在票 [operator-channel/38](../../.scratch/operator-channel/issues/38-controlled-closure-and-reopening-record-request-and-operation-evidence.md)「裁定」一节；实施与成文由该会话起的实施子代理完成。裁决能力边界：读过 ADR-0183 全文（含两份复核记录与 owner 复核行）、ADR-0151 全文、ADR-0028 全文、ADR-0146 的 Status 与决定一至三、adr/README 的部分停用机制；PS CONTEXT「面单继续尝试决定」的定义、Rules 里受控关闭与重开各条、Boundaries 相关句，GLOSSARY 同名词条，SCENARIOS 面单场景；pc-gaps/13 的「裁决」与「要裁的」，label-channel/30 的「裁决」与两次评审；受控关闭与重开的领域、编排、HTTP 适配器与快照适配器全文。没读：UC-PS 各篇正文，ADR-0029、0030、0100、0139 正文，PILOT-PARAMETER-REGISTER 与开发主线。拿不准的列在文末「越权风险点」。）；**部分停用** ADR-0183 决定五首句与 ADR-0151 决定二受控关闭与重开一项「不进命令的任何一格」半句，适用场景见决定七（只限受控关闭口与重开口）<!-- 索引注记 -->｜**请求证据**：请求方所凭依据的引用落在决定上，去两端空白逐字保存、不判真伪；形成时与请求方同在同缺（编排在问授权之前核），重建门只拒有证据无请求方，照收规则生效前的旧历史，不加守卫迁移。**操作证据**：提交操作者写作 `OPERATOR/<发行方>#<sub>`，只进审计，任何门与比对都不读它。**重放**：两份证据不进比对，回执带回册上的 `requestEvidence`。**载荷**：货主指令关闭必带 `shipperInstructionEvidence`，另两种来源带了即 400
```

### ADR-0188: 已拒绝委托的重建门按快照表达能力开门

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-18，票 [operator-channel/33](../../.scratch/operator-channel/issues/33-rejected-shipment-requests-persist-and-read-back.md)。会话 `idp-parcel-9b` 在用户 2026-10-16「你作为业务和系统专家你来自决」的授权下代 PS owner 裁定。）<!-- 索引注记 -->｜**已接受**（2026-10-18，票 operator-channel/33，会话 `idp-parcel-9b` 代 PS owner 裁定）。**推进** [ADR-0061](./0061-accepted-shipment-request-rehydration-by-snapshot-expressiveness.md) 决定一里`已拒绝`那一半：快照决定补上主动拒绝留痕、依据对拒绝决定可缺席之后，重建门开到`已拒绝`；正文有三条来路的对账表与坏数据判据（拒绝决定恰带失败校验项与主动拒绝留痕之一，补表达前的旧形状行按坏数据拒绝、不推回）。`已撤回`仍不开
```

合并后：

```text
Status: Accepted（2026-10-18，票 [operator-channel/33](../../.scratch/operator-channel/issues/33-rejected-shipment-requests-persist-and-read-back.md)。会话 `idp-parcel-9b` 在用户 2026-10-16「你作为业务和系统专家你来自决」的授权下代 PS owner 裁定。）<!-- 索引注记 -->｜**推进** [ADR-0061](./0061-accepted-shipment-request-rehydration-by-snapshot-expressiveness.md) 决定一里`已拒绝`那一半：快照决定补上主动拒绝留痕、依据对拒绝决定可缺席之后，重建门开到`已拒绝`；正文有三条来路的对账表与坏数据判据（拒绝决定恰带失败校验项与主动拒绝留痕之一，补表达前的旧形状行按坏数据拒绝、不推回）。`已撤回`仍不开
```

### ADR-0189：逐段计价区域按卡给——绑了分区目录的卡由包裹邮编路线经目录读出，未绑的卡取租户按线路段登记的段计价区域

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-18，Claude Code 会话代决：用户原话「你作为业务和系统专家自决。解决所有的问题」；票 [routing-first-cut/18](../../.scratch/routing-first-cut/issues/18-leg-endpoints-folded-into-price-card-zones.md) 的产物。补 [ADR-0148](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定四第 7 条留给 PP 计价输入缝的那一格，越权风险点 5 由本记录接手。越权风险点见文末，待 PP、NR 与 SA owner 复核。） 决定四第二步（绑目录那一路）落地时的补充裁定见文末「补记：票 routing-first-cut/20 落绑目录那一路时的补充裁定」（2026-10-19）。<!-- 索引注记 -->｜**已接受**（2026-10-18，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 PP、NR owner 裁定；票 routing-first-cut/18）。**按卡给**：绑目录的卡走包裹邮编路线与 PP 读数补全（票 routing-first-cut/20），未绑的卡取段计价区域，都没有即待判断。**段计价区域**：NR 一本只增不改的册，键（租户，线路，版本，段号），只挂在有成本依据的段上，不随线路版本内容走。只适用于整段区域恒定的卡，按目的邮编分区的卡要绑目录。不采节点邮编。**越权风险点**待 PP、NR、SA owner 复核。
```

合并后：

```text
Status: Accepted（2026-10-18，Claude Code 会话代 PP、NR owner 决：用户原话「你作为业务和系统专家自决。解决所有的问题」；票 [routing-first-cut/18](../../.scratch/routing-first-cut/issues/18-leg-endpoints-folded-into-price-card-zones.md) 的产物。补 [ADR-0148](./0148-route-evidence-sourcing-candidate-cost-and-first-candidate-generation-form.md) 决定四第 7 条留给 PP 计价输入缝的那一格，越权风险点 5 由本记录接手。越权风险点见文末，待 PP、NR 与 SA owner 复核。） 决定四第二步（绑目录那一路）落地时的补充裁定见文末「补记：票 routing-first-cut/20 落绑目录那一路时的补充裁定」（2026-10-19）。<!-- 索引注记 -->｜**按卡给**：绑目录的卡走包裹邮编路线与 PP 读数补全（票 routing-first-cut/20），未绑的卡取段计价区域，都没有即待判断。**段计价区域**：NR 一本只增不改的册，键（租户，线路，版本，段号），只挂在有成本依据的段上，不随线路版本内容走。只适用于整段区域恒定的卡，按目的邮编分区的卡要绑目录。不采节点邮编。
```

### ADR-0190：包裹计费时点发起 SELL 评价，形成预估客户费用——独立的 SELL 请求身份，内置「有效网络收寄已采用」「有效交付已终局」两种时点，按结算账户登记采用

处理：接受出处两写合并；补指针

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」，其后又令「继续」，按会话里报告过的次序先做客户费用。取代 [ADR-0169](./0169-sell-evaluation-forms-a-customer-charge.md) Consequences「形成编排不进 `cmd/parcel-dispatch`」与决定四「也不在本记录里把形成编排接进进程」，以及 [ADR-0170](./0170-settlement-orchestrations-enter-through-parcel-api.md) Context「SELL 评价形成客户费用的编排不进 `cmd/parcel-dispatch`」与「候选与反方」里否决「把 SELL 形成编排接进派发」那一条；为 [ADR-0171](./0171-pricing-input-resolver-uses-declaration-until-a-measurement-port-exists.md) 决定五在 SELL·客户费用预估这一格作答。越权风险点见文末，待 SA、PP owner 复核。）<!-- 索引注记 -->｜**已接受**（2026-10-19，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 SA、PP owner 裁定）。**独立 SELL 请求身份**，不动 BUY。**内置两种计费时点**：有效网络收寄已采用、有效交付已终局，结算账户登记采用哪一种与计价范围，没登记答未配置。**选卡同受理前试算**，预估阶段没有实测时用申报并标明来源。**一份评价映一个费用项目**才形成预估客户费用。编排住 `cmd/parcel-dispatch`，取代 ADR-0169、0170 里「不进派发」的句子。**越权风险点**待 SA、PP owner 复核。｜**部分停用**（2026-10-21）：决定一「自然键（租户、包裹、计费时点）只守幂等」与决定五「计价侧没形成评价（未配置、适用冲突、输入不可得）时不发评价已记录信封，请求停在「已提交」」两句已由 [ADR-0229](./0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 停用——自然键加代次唯一，补登参数之后另起新一代请求续办；计价答不出评价时交「评价未形成」，请求记`未形成`连同停因。**适用场景**：决定一那句只停「唯一」的口径，同一件包裹在所采时点只发起一次照旧；越权风险点 3「请求卡死」由 ADR-0229 作答。其余各条不变
```

合并后：

```text
Status: Accepted（2026-10-19，Claude Code 会话代 SA、PP owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」，其后又令「继续」，按会话里报告过的次序先做客户费用。取代 [ADR-0169](./0169-sell-evaluation-forms-a-customer-charge.md) Consequences「形成编排不进 `cmd/parcel-dispatch`」与决定四「也不在本记录里把形成编排接进进程」，以及 [ADR-0170](./0170-settlement-orchestrations-enter-through-parcel-api.md) Context「SELL 评价形成客户费用的编排不进 `cmd/parcel-dispatch`」与「候选与反方」里否决「把 SELL 形成编排接进派发」那一条；为 [ADR-0171](./0171-pricing-input-resolver-uses-declaration-until-a-measurement-port-exists.md) 决定五在 SELL·客户费用预估这一格作答。另，决定二把 ADR-0169 决定三「内置时点只有发生项形成」扩成两路（指针 2026-10-21 补）。越权风险点见文末，待 SA、PP owner 复核。）<!-- 索引注记 -->｜**独立 SELL 请求身份**，不动 BUY。**内置两种计费时点**：有效网络收寄已采用、有效交付已终局，结算账户登记采用哪一种与计价范围，没登记答未配置。**选卡同受理前试算**，预估阶段没有实测时用申报并标明来源。**一份评价映一个费用项目**才形成预估客户费用。编排住 `cmd/parcel-dispatch`｜**部分停用**（2026-10-21）：决定一「自然键（租户、包裹、计费时点）只守幂等」与决定五「计价侧没形成评价（未配置、适用冲突、输入不可得）时不发评价已记录信封，请求停在「已提交」」两句已由 [ADR-0229](./0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 停用——自然键加代次唯一，补登参数之后另起新一代请求续办；计价答不出评价时交「评价未形成」，请求记`未形成`连同停因。**适用场景**：决定一那句只停「唯一」的口径，同一件包裹在所采时点只发起一次照旧；越权风险点 3「请求卡死」由 ADR-0229 作答。其余各条不变
```

### ADR-0195：唯一命中要没有歧义信号——号码同时被一份`已提交`委托声明时，节点答身份冲突；声明不进候选

处理：来向两写合并；接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19。用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；该会话代 PS owner 与 NO owner 裁定，裁定原文落在票 [ps-external-mark-relations/05](../../.scratch/ps-external-mark-relations/issues/05-a-unique-hit-in-someone-elses-number-space-is-not-enough.md)「裁定（2026-10-18，会话 idp-parcel-9b 代决）」一节。本记录照它写成，补上实施时定下的形状；成文由该会话起的作者子代理完成。部分停用 [ADR-0184](./0184-external-identifier-relations-are-an-append-only-register-formed-with-their-source-facts.md) 决定五里「只交回包裹」一句，其余各条不变。本记录自身决定四一句由 [ADR-0211](./0211-an-ambiguity-only-identity-conflict-releases-by-recheck-once-the-declaration-is-gone.md) 停用，只限冲突来由为只因歧义信号的身份冲突，随票 no-reception-identity/07 进 main 生效；原句见本记录文末「复核记录二（2026-10-19，ADR-0211 部分停用一句」与 adr/README 该行。裁决能力边界：读过 PS `CONTEXT.md`「外部标识」「外部标识关联」的定义、外部标识的各条 Rules 与`委托`生命周期，NO `CONTEXT.md`「实物身份与事实完整性」与「待识别实物与身份关联」，CONTEXT-MAP `parcel-shipment → node-operations` 各句，UC-NO-002 的流程表、「权限、隔离与审计」与验收表，ADR-0184 及其复核记录、ADR-0185、ADR-0196 及其复核记录、ADR-0199；代码读过 PS 关联册读面 `FindParcelsByExternalIdentifier`、委托仓储的快照文档与 `Save`、委托状态 `ShipmentRequestState`，NO 收寄编排 `ReceiveDeliveredUnitHandler` 的明确接收支、身份读口 `ParcelIdentityView` 与防腐适配器。**没读** PS 采用节点收寄的适配器全文与 VE 追踪投影的消费者。）部分停用：[ADR-0212](./0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md)（被停条款：决定一「不按客户账户、分配方或交付方收窄」，适用场景是有可验证交付范围时——信号只认范围内客户账户的声明；决定三起句「NO 只认「恰一个候选、且没有信号」为唯一命中」与它下面的分格，有可验证交付范围时由 ADR-0212 决定四的表取代，没有时只「恰一个候选、无信号」一格收窄为候选范围相容才算；其余各条不变，本记录正文不改写）<!-- 索引注记 -->｜**已接受**（2026-10-19，用户授权会话 idp-parcel-9b 代 PS、NO owner 裁定，落在票 ps-external-mark-relations/05）。**信号**：PS 身份核对随候选答出号码是否同时被本租户一份`已提交`委托声明为客户包裹件号，只是一个是否值，声明不进候选；已拒绝、已撤回不算，已接受的号已在候选里。候选与信号同一条语句读。**判法**：恰一个候选且无信号才是唯一命中；恰一个候选带信号与多候选同格，答待识别、记身份冲突，不进 ADR-0196 那一支；零候选带信号仍是待识别、不记冲突。重核（ADR-0199 决定三）经同一份判断继承。PS 迁移 0026 加部分 GIN 索引。部分停用 ADR-0184 决定五「只交回包裹」一句，决定六维持。越权风险点含改 PS CONTEXT 读法、合法重复用号与持号一方的实物也答冲突｜**部分停用**：决定四「信号消失不交信封。……冲突行不经重核解除……」里「信号消失不交信封」与「冲突行不经重核解除」两处，连同越权风险点「信号消失后，已记的冲突不自动解除」，已由 [ADR-0211](./0211-an-ambiguity-only-identity-conflict-releases-by-recheck-once-the-declaration-is-gone.md) 停用，**适用场景**：只限冲突来由为只因歧义信号的身份冲突——PS 在声明它的委托被拒绝或撤回时交声明撤除门铃，这类冲突经重核解开；同句「零候选的待识别又本就不看信号」照旧，自票 no-reception-identity/07 进 main 起生效；决定一「不按客户账户、分配方或交付方收窄」与决定三起句「NO 只认「恰一个候选、且没有信号」为唯一命中」及其分格已由 [ADR-0212](./0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md) 停用，**适用场景**：决定一只限有可验证交付范围时（信号只认范围内客户账户的声明）；决定三在有可验证交付范围时由 ADR-0212 决定四的表取代，没有时只「恰一个候选、无信号」一格收窄为候选范围相容才算；自票 ps-external-mark-relations/11 进 main 起生效；其余各条不变
```

合并后：

```text
Status: Accepted（2026-10-19。用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；该会话代 PS owner 与 NO owner 裁定，裁定原文落在票 [ps-external-mark-relations/05](../../.scratch/ps-external-mark-relations/issues/05-a-unique-hit-in-someone-elses-number-space-is-not-enough.md)「裁定（2026-10-18，会话 idp-parcel-9b 代决）」一节。本记录照它写成，补上实施时定下的形状；成文由该会话起的作者子代理完成。部分停用 [ADR-0184](./0184-external-identifier-relations-are-an-append-only-register-formed-with-their-source-facts.md) 决定五里「只交回包裹」一句，决定六维持，其余各条不变。裁决能力边界：读过 PS `CONTEXT.md`「外部标识」「外部标识关联」的定义、外部标识的各条 Rules 与`委托`生命周期，NO `CONTEXT.md`「实物身份与事实完整性」与「待识别实物与身份关联」，CONTEXT-MAP `parcel-shipment → node-operations` 各句，UC-NO-002 的流程表、「权限、隔离与审计」与验收表，ADR-0184 及其复核记录、ADR-0185、ADR-0196 及其复核记录、ADR-0199；代码读过 PS 关联册读面 `FindParcelsByExternalIdentifier`、委托仓储的快照文档与 `Save`、委托状态 `ShipmentRequestState`，NO 收寄编排 `ReceiveDeliveredUnitHandler` 的明确接收支、身份读口 `ParcelIdentityView` 与防腐适配器。**没读** PS 采用节点收寄的适配器全文与 VE 追踪投影的消费者。）<!-- 索引注记 -->｜**信号**：PS 身份核对随候选答出号码是否同时被本租户一份`已提交`委托声明为客户包裹件号，只是一个是否值，声明不进候选；已拒绝、已撤回不算，已接受的号已在候选里。候选与信号同一条语句读。**判法**：恰一个候选且无信号才是唯一命中；恰一个候选带信号与多候选同格，答待识别、记身份冲突，不进 ADR-0196 那一支；零候选带信号仍是待识别、不记冲突。重核（ADR-0199 决定三）经同一份判断继承。PS 迁移 0026 加部分 GIN 索引。越权风险点含改 PS CONTEXT 读法、合法重复用号与持号一方的实物也答冲突｜**部分停用**：其决定四「信号消失不交信封。……冲突行不经重核解除……」里「信号消失不交信封」与「冲突行不经重核解除」两处，连同越权风险点「信号消失后，已记的冲突不自动解除」，已由 [ADR-0211](./0211-an-ambiguity-only-identity-conflict-releases-by-recheck-once-the-declaration-is-gone.md) 停用，**适用场景**：只限冲突来由为只因歧义信号的身份冲突——PS 在声明它的委托被拒绝或撤回时交声明撤除门铃，这类冲突经重核解开；同句「零候选的待识别又本就不看信号」照旧，自票 no-reception-identity/07 进 main 起生效，裁定经过见本记录文末「复核记录二」；决定一「不按客户账户、分配方或交付方收窄」与决定三起句「NO 只认「恰一个候选、且没有信号」为唯一命中」及其分格已由 [ADR-0212](./0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md) 停用，**适用场景**：决定一只限有可验证交付范围时（信号只认范围内客户账户的声明）；决定三在有可验证交付范围时由 ADR-0212 决定四的表取代，没有时只「恰一个候选、无信号」一格收窄为候选范围相容才算；自票 ps-external-mark-relations/11 进 main 起生效；裁定经过见本记录文末「复核记录三」；其余各条不变，本记录正文不改写
```

### ADR-0196：一码两物按关联到的包裹认——后到那件答身份冲突，先到那件只追加一版身份冲突判断

处理：来向两写合并；接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19。用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；该会话代 NO owner 裁定，裁定原文落在票 [no-reception-identity/01](../../.scratch/no-reception-identity/issues/01-one-barcode-on-two-units-forms-an-identity-conflict.md)「裁定（2026-10-18，会话 idp-parcel-9b 代决）」一节，本记录照它写成并补上实施时定下的形状。）部分停用：几句由 [ADR-0210](./0210-identity-conflicts-are-disposed-by-an-operation-decision-and-the-identity-basis-reads-chain-heads.md) 停用，随票 no-reception-identity/05、06 进 main 各自生效；被停的原句见本记录文末「复核记录二」与 adr/README 该行。另有两句由 [ADR-0199](./0199-node-reception-preserves-the-delivery-source-and-rechecks-pending-identities.md) 决定四停用，只限同一节点上有当前收寄的情形，随票 no-reception-source/04 进 main 生效；原句见本记录文末「复核记录三」与 adr/README 该行。<!-- 索引注记 -->｜**已接受**（2026-10-19，用户授权会话 idp-parcel-9b 代 NO owner 裁定，落在票 no-reception-identity/01）。号码唯一指向的包裹已关联到同租户另一件作业实物：后到那件答`待识别`、记身份冲突、候选记那件包裹；先到那件原判断照留，另追加一版待识别收寄记身份冲突；两件在方向依据的身份一格里都落成身份冲突。同一实物重投仍是重放。PS 已采用的那次不回滚，交 VE。越权风险点含按包裹而非按号认、派生来源标识、并发未串行｜**部分停用**：决定四「先到那件有原判断的关联行加追加的冲突行，落成缺少·身份冲突」与同条「方向依据代码不改」已由 [ADR-0210](./0210-identity-conflicts-are-disposed-by-an-operation-decision-and-the-identity-basis-reads-chain-heads.md) 决定五停用，**适用场景**：前一句只限指名了前版的冲突版，先到那件改落缺少·{待识别, 身份冲突}，自票 no-reception-identity/05 进 main 起生效；决定六「按租户与包裹关联读别的作业实物的收寄，只认收寄本身带的关联」对被身份冲突处置版接续过的关联停用（ADR-0210 决定八），自票 no-reception-identity/06 进 main 起生效；决定一末句「同一件实物换个来源再收一次照常形成收寄」与「能力边界」里冲突形成之后先到那件换来源再收「照常答`节点收寄已形成`、交发布意图」一句已由 [ADR-0199](./0199-node-reception-preserves-the-delivery-source-and-rechecks-pending-identities.md) 决定四停用，**适用场景**：只限同一节点上有当前收寄（当前有效版本是形成或待识别、控制没有转出），这时再来的交付并入那条收寄、答已有结果、不交意图，自票 no-reception-source/04 进 main 起生效；其余各条不变
```

合并后：

```text
Status: Accepted（2026-10-19。用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；该会话代 NO owner 裁定，裁定原文落在票 [no-reception-identity/01](../../.scratch/no-reception-identity/issues/01-one-barcode-on-two-units-forms-an-identity-conflict.md)「裁定（2026-10-18，会话 idp-parcel-9b 代决）」一节，本记录照它写成并补上实施时定下的形状。）<!-- 索引注记 -->｜号码唯一指向的包裹已关联到同租户另一件作业实物：后到那件答`待识别`、记身份冲突、候选记那件包裹；先到那件原判断照留，另追加一版待识别收寄记身份冲突；两件在方向依据的身份一格里都落成身份冲突。同一实物重投仍是重放。PS 已采用的那次不回滚，交 VE。越权风险点含按包裹而非按号认、派生来源标识、并发未串行｜**部分停用**：其决定四「先到那件有原判断的关联行加追加的冲突行，落成缺少·身份冲突」与同条「方向依据代码不改」已由 [ADR-0210](./0210-identity-conflicts-are-disposed-by-an-operation-decision-and-the-identity-basis-reads-chain-heads.md) 决定五停用，**适用场景**：前一句只限指名了前版的冲突版，先到那件改落缺少·{待识别, 身份冲突}，自票 no-reception-identity/05 进 main 起生效；决定六「按租户与包裹关联读别的作业实物的收寄，只认收寄本身带的关联」对被身份冲突处置版接续过的关联停用（ADR-0210 决定八），自票 no-reception-identity/06 进 main 起生效，裁定经过见本记录文末「复核记录二」；其决定一末句「同一件实物换个来源再收一次照常形成收寄」与「能力边界」里冲突形成之后先到那件换来源再收「照常答`节点收寄已形成`、交发布意图」一句已由 [ADR-0199](./0199-node-reception-preserves-the-delivery-source-and-rechecks-pending-identities.md) 决定四停用，**适用场景**：只限同一节点上有当前收寄（当前有效版本是形成或待识别、控制没有转出），这时再来的交付并入那条收寄、答已有结果、不交意图，自票 no-reception-source/04 进 main 起生效；裁定经过见本记录文末「复核记录三」；其余各条不变
```

### ADR-0197：关务外部结果按层进全程追踪投影——事实类型词逐层分开、放行再按全部/部分/附条件分开，沿结果范围指名的申报单元逐成员展开，同层冲突的那份不进投影；参考映射只把全部放行映成清关

处理：来向两写合并；接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19。用户授权「你作为业务和系统专家自决，解决所有的问题」；Claude Code 会话代 VE、CC owner 裁定，落在票 [demo-chain-gaps/11](../../.scratch/demo-chain-gaps/issues/11-external-result-received-has-no-subscriber.md)。越权风险点待 VE、CC owner 复核。）越权风险点 5 后半「CC 的层内一致性核对把同层、同版本、同范围下原始语义不同的两份一律判冲突」已由 [ADR-0226](./0226-successive-same-layer-customs-results-are-successors-unless-they-contradict.md) 回答（2026-10-21）：同层先分后续与矛盾——放行按种类偏序随业务时间只进不退，监管过程决定按业务时间成序列，只有矛盾才标冲突；决定五「同层冲突的那份入账不派生」与本记录各条不变。<!-- 索引注记 -->｜**已接受**（2026-10-19，用户授权「你作为业务和系统专家自决」，Claude Code 会话代 VE、CC owner 裁定，落在票 demo-chain-gaps/11）。**消费方**：VE 投影订阅 `customs-compliance.external-result.received`，只投 VE；CC 放行门禁核对按动作请求驱动、不订阅。**类型词**：按层分开，放行层再按全部/部分/附条件分开，不设跨层词、没有「清关成功」；里程碑归映射目录，未登记即未归类。**展开**：结果范围→申报单元→组成逐成员，引用含成员与来源标识，版本取提交版本。**不派生**：同层冲突的那份入账不派生；归属不上、放行三件与层次不配为不变量已破。**参考映射**：演示租户只把全部放行映成 `CUSTOMS_CLEARED`。越权风险点待 VE、CC owner 复核｜**同层后续已答**（2026-10-21）：越权风险点 5 后半「CC 的层内一致性核对……一律判冲突」由 [ADR-0226](./0226-successive-same-layer-customs-results-are-successors-unless-they-contradict.md) 回答——放行按种类偏序随业务时间只进不退、监管过程决定按业务时间成序列，同一时刻两种结论与放行回退仍是冲突；先部分后全部时成员归入 `CUSTOMS_CLEARED`；VE 冲突信号另票；无条款停用，其余各条不变
```

合并后：

```text
Status: Accepted（2026-10-19。用户授权「你作为业务和系统专家自决，解决所有的问题」；Claude Code 会话代 VE、CC owner 裁定，落在票 [demo-chain-gaps/11](../../.scratch/demo-chain-gaps/issues/11-external-result-received-has-no-subscriber.md)。越权风险点待 VE、CC owner 复核。）<!-- 索引注记 -->｜**消费方**：VE 投影订阅 `customs-compliance.external-result.received`，只投 VE；CC 放行门禁核对按动作请求驱动、不订阅。**类型词**：按层分开，放行层再按全部/部分/附条件分开，不设跨层词、没有「清关成功」；里程碑归映射目录，未登记即未归类。**展开**：结果范围→申报单元→组成逐成员，引用含成员与来源标识，版本取提交版本。**不派生**：同层冲突的那份入账不派生；归属不上、放行三件与层次不配为不变量已破。**参考映射**：演示租户只把全部放行映成 `CUSTOMS_CLEARED`｜**同层后续已答**（2026-10-21）：越权风险点 5 后半「CC 的层内一致性核对……一律判冲突」由 [ADR-0226](./0226-successive-same-layer-customs-results-are-successors-unless-they-contradict.md) 回答——放行按种类偏序随业务时间只进不退、监管过程决定按业务时间成序列，同一时刻两种结论与放行回退仍是冲突；先部分后全部时成员归入 `CUSTOMS_CLEARED`；VE 冲突信号另票；无条款停用，决定五「记了同层冲突的那一份：入账不派生」与其余各条不变
```

### ADR-0198：包裹计费时点那一路，SELL 请求怎样下结论，矛盾怎样上抛

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 sell-customer-charge/04 落地与评审时遇到的、[ADR-0190](./0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 没裁的几格。部分停用 [ADR-0169](./0169-sell-evaluation-forms-a-customer-charge.md) 决定四「信封上还没有费用项目时，消费门指名缺这一引用」，**适用场景**仅限包裹计费时点那一路，即回指一份 SELL 请求的 SELL·客户费用评价。越权风险点见文末，待 SA owner 复核。）<!-- 索引注记 -->｜**已接受**（2026-10-19，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 SA owner 裁定）。评价失败停在`已提交`；卡没声明金额取整、映射不全记`待判断`，缺口不落库；回指不在册、结算币种不符、另有结论、已完成却未受理一律上抛；没有回指零写入。部分停用 ADR-0169 决定四一句，限本路。越权风险点 5 条待 SA owner 复核。｜**部分停用**（2026-10-21）：决定一「评价失败，请求停在`已提交`，入账不重投」中「请求停在`已提交`」已由 [ADR-0229](./0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 停用——改记`未形成`、停因评价失败，补登参数之后另起新一代请求续办；入账不重投照旧。其余各条不变
```

合并后：

```text
Status: Accepted（2026-10-19，Claude Code 会话代 SA owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 sell-customer-charge/04 落地与评审时遇到的、[ADR-0190](./0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 没裁的几格。部分停用 [ADR-0169](./0169-sell-evaluation-forms-a-customer-charge.md) 决定四「信封上还没有费用项目时，消费门指名缺这一引用」，**适用场景**仅限包裹计费时点那一路，即回指一份 SELL 请求的 SELL·客户费用评价。越权风险点见文末，待 SA owner 复核。）<!-- 索引注记 -->｜评价失败停在`已提交`；卡没声明金额取整、映射不全记`待判断`，缺口不落库；回指不在册、结算币种不符、另有结论、已完成却未受理一律上抛；没有回指零写入｜**部分停用**（2026-10-21）：决定一「评价失败，请求停在`已提交`，入账不重投」中「请求停在`已提交`」已由 [ADR-0229](./0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 停用——改记`未形成`、停因评价失败，补登参数之后另起新一代请求续办；入账不重投照旧。其余各条不变
```

### ADR-0199：节点收寄保全交付来源——待识别实物按存下的号重核，同一作业实物在同一节点不另起控制起点，身份核对读不动在前提齐备后翻成待识别加未核对

处理：来向两写合并；接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19。用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；该会话代 NO owner 裁定，裁定原文落在票 [no-reception-source/01](../../.scratch/no-reception-source/issues/01-node-reception-preserves-the-delivery-source.md)「裁定（2026-10-18，会话 idp-parcel-9b 代决）」一节，本记录照它写成，补上各期难逆转的形状。成文由该会话起的作者子代理完成。裁决能力边界：读过 NO `CONTEXT.md`、UC-NO-002、CONTEXT-MAP 与 GLOSSARY 的相关条目、ADR-0186、ADR-0196 及其复核记录、票 operator-channel/29、43、45 与 no-reception-identity/01、03；代码读过收寄编排 `ReceiveDeliveredUnitHandler`、ports 的收寄记录与收寄库端口、收寄库 PG 适配器、`ParcelContainmentView`、载荷规范化 `CanonicalizeDeliveryPayload`、方向依据的 `JudgeIdentityBasis`、复核目录读端口与 HTTP 收寄答复。PS 外部标识关联册只读过读面 `FindParcelsByExternalIdentifier` 与写入函数 `insertExternalIdentifierRelations`，它的调用方（委托仓储、面单交易仓储）的事务与 Outbox 装配没有逐行读。）部分停用：几句由 [ADR-0210](./0210-identity-conflicts-are-disposed-by-an-operation-decision-and-the-identity-basis-reads-chain-heads.md) 停用，随票 no-reception-identity/05、06 进 main 各自生效；被停的原句见本记录文末「复核记录（2026-10-19，ADR-0210 部分停用几句」与 adr/README 该行。另有决定三两句由 [ADR-0211](./0211-an-ambiguity-only-identity-conflict-releases-by-recheck-once-the-declaration-is-gone.md) 停用，只限冲突来由为只因歧义信号的身份冲突，随票 no-reception-identity/07 进 main 生效；原句见本记录文末「复核记录（2026-10-19，ADR-0211 部分停用两句」与 adr/README 该行。另有决定一、决定三各有句子由 [ADR-0212](./0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md) 停用，随票 ps-external-mark-relations/11、12 进 main 各自生效；原句见本记录「复核记录（2026-10-19，ADR-0212 部分停用几句」与 adr/README 该行。<!-- 索引注记 -->｜**已接受**（2026-10-19，用户授权会话 idp-parcel-9b 代 NO owner 裁定，落在票 no-reception-source/01）。**来源**：收寄记录带本次交付的来源文档，三种接收观察都存、外部标识观察原样，指纹口径不变，既有行不回填；每版判断有版本，追加的一版指名前版，已有结果带回原判断种类与当前有效版本。**重核**：只由 PS 关联册新形成关联时交出的（租户，号码）信封与复核页人工发起触发，不扫表；结果唯一即追加关联版，业务时间取原收寄时刻、不另起控制起点、意图一封。**同一实物**：按作业实物标识认，同一节点已有当前收寄时并入、答已有结果；仅扫描待确认由后来的观察续办。**翻转**：身份核对读不动落`待识别`加身份核对态「未核对」（信息格，不进 outcome），四个前提齐备才翻，落地次序一、二、四、三；翻转与挡法落地时分别部分停用 ADR-0186 决定二、决定七各一句与 ADR-0196 决定一末句。越权风险点含触发只有信封与人工、指纹不含证据｜**部分停用**：决定三「重核对象」一项里「册上已有的冲突行在 `JudgeIdentityBasis` 里一直判身份冲突，重核解不开」一句与决定五末项里「方向依据代码不改」「照现有折法」已由 [ADR-0210](./0210-identity-conflicts-are-disposed-by-an-operation-decision-and-the-identity-basis-reads-chain-heads.md) 决定五停用（结论「记身份冲突的不重核」维持，理由改为冲突只经人工处置解除），自票 no-reception-identity/05 进 main 起生效；决定三分格里「唯一，且包裹没有关联到别的作业实物」与「唯一，但包裹已关联到别的作业实物」两支已由 ADR-0210 决定九停用，**适用场景**：只限被重核那一版是「不是其中任何一件」的身份冲突处置版、唯一命中的包裹在它排除之列，自票 no-reception-identity/06 进 main 起生效；其余各条不变｜**部分停用**：决定三「触发只有三处，不做后台扫表」里「只有三处」，与「重核对象」一项里「记身份冲突的（多候选、一码两物）不重核」，已由 [ADR-0211](./0211-an-ambiguity-only-identity-conflict-releases-by-recheck-once-the-declaration-is-gone.md) 停用，**适用场景**：只限冲突来由为只因歧义信号的身份冲突（ADR-0195 决定四把带信号的一个候选落进这一句）——触发多一处 PS 声明撤除门铃，这类冲突是重核对象；「不做后台扫表」与多候选、一码两物、来由缺席的冲突不重核照旧，自票 no-reception-identity/07 进 main 起生效；决定一「内容：作业实物、外部标识观察、接收观察、证据、拒收原因、交付方、节点、发生时间」与「证据、拒收原因、交付方只进来源文档、不进指纹」两句的列举、决定三「触发只有三处」里「只有三处」已由 [ADR-0212](./0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md) 停用，**适用场景**：前两句各多一格交付范围依据引用（指纹口径不变），自票 ps-external-mark-relations/11 进 main 起生效；后一句在 ADR-0211 的声明撤除门铃之外再多一处「交付范围成立」的自投门铃（不做后台扫表照旧），自票 ps-external-mark-relations/12 进 main 起生效；其余各条不变
```

合并后：

```text
Status: Accepted（2026-10-19。用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；该会话代 NO owner 裁定，裁定原文落在票 [no-reception-source/01](../../.scratch/no-reception-source/issues/01-node-reception-preserves-the-delivery-source.md)「裁定（2026-10-18，会话 idp-parcel-9b 代决）」一节，本记录照它写成，补上各期难逆转的形状。成文由该会话起的作者子代理完成。裁决能力边界：读过 NO `CONTEXT.md`、UC-NO-002、CONTEXT-MAP 与 GLOSSARY 的相关条目、ADR-0186、ADR-0196 及其复核记录、票 operator-channel/29、43、45 与 no-reception-identity/01、03；代码读过收寄编排 `ReceiveDeliveredUnitHandler`、ports 的收寄记录与收寄库端口、收寄库 PG 适配器、`ParcelContainmentView`、载荷规范化 `CanonicalizeDeliveryPayload`、方向依据的 `JudgeIdentityBasis`、复核目录读端口与 HTTP 收寄答复。PS 外部标识关联册只读过读面 `FindParcelsByExternalIdentifier` 与写入函数 `insertExternalIdentifierRelations`，它的调用方（委托仓储、面单交易仓储）的事务与 Outbox 装配没有逐行读。）<!-- 索引注记 -->｜**来源**：收寄记录带本次交付的来源文档，三种接收观察都存、外部标识观察原样，指纹口径不变，既有行不回填；每版判断有版本，追加的一版指名前版，已有结果带回原判断种类与当前有效版本。**重核**：只由 PS 关联册新形成关联时交出的（租户，号码）信封与复核页人工发起触发，不扫表；结果唯一即追加关联版，业务时间取原收寄时刻、不另起控制起点、意图一封。**同一实物**：按作业实物标识认，同一节点已有当前收寄时并入、答已有结果；仅扫描待确认由后来的观察续办。**翻转**：身份核对读不动落`待识别`加身份核对态「未核对」（信息格，不进 outcome），四个前提齐备才翻，落地次序一、二、四、三；翻转与挡法落地时分别部分停用 ADR-0186 决定二、决定七各一句与 ADR-0196 决定一末句。越权风险点含触发只有信封与人工、指纹不含证据｜**部分停用**：决定三「重核对象」一项里「册上已有的冲突行在 `JudgeIdentityBasis` 里一直判身份冲突，重核解不开」一句与决定五末项里「方向依据代码不改」「照现有折法」已由 [ADR-0210](./0210-identity-conflicts-are-disposed-by-an-operation-decision-and-the-identity-basis-reads-chain-heads.md) 决定五停用（结论「记身份冲突的（多候选、一码两物）不重核」维持，理由改为冲突只经人工处置解除），自票 no-reception-identity/05 进 main 起生效；决定三分格里「唯一，且包裹没有关联到别的作业实物」与「唯一，但包裹已关联到别的作业实物」两支已由 ADR-0210 决定九停用，**适用场景**：只限被重核那一版是「不是其中任何一件」的身份冲突处置版、唯一命中的包裹在它排除之列，自票 no-reception-identity/06 进 main 起生效，裁定经过见本记录文末「复核记录（2026-10-19，ADR-0210 部分停用几句；决定原文不动）」；其余各条不变｜**部分停用**：决定三「触发只有三处，不做后台扫表」里「只有三处」，与「重核对象」一项里「记身份冲突的（多候选、一码两物）不重核」，已由 [ADR-0211](./0211-an-ambiguity-only-identity-conflict-releases-by-recheck-once-the-declaration-is-gone.md) 停用，**适用场景**：只限冲突来由为只因歧义信号的身份冲突（ADR-0195 决定四把带信号的一个候选落进这一句）——触发多一处 PS 声明撤除门铃，这类冲突是重核对象；「不做后台扫表」与多候选、一码两物、来由缺席的冲突不重核照旧，自票 no-reception-identity/07 进 main 起生效，裁定经过见本记录文末「复核记录（2026-10-19，ADR-0211 部分停用两句；决定原文不动）」；决定一「内容：作业实物、外部标识观察、接收观察、证据、拒收原因、交付方、节点、发生时间」与「证据、拒收原因、交付方只进来源文档、不进指纹」两句的列举、决定三「触发只有三处」里「只有三处」已由 [ADR-0212](./0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md) 停用，**适用场景**：前两句各多一格交付范围依据引用（指纹口径不变），自票 ps-external-mark-relations/11 进 main 起生效；后一句在 ADR-0211 的声明撤除门铃之外再多一处「交付范围成立」的自投门铃（不做后台扫表照旧），自票 ps-external-mark-relations/12 进 main 起生效；裁定经过见本记录文末「复核记录（2026-10-19，ADR-0212 部分停用几句；决定原文不动）」；其余各条不变
```

### ADR-0200：BUY 评价按回指的请求形成供应商预期成本——版本标识由请求派生，评价停下的各格入账

处理：来向两写合并；接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/08。越权风险点见文末，待 SA owner 复核。） 决定三「同一发生项、费用项目与规则版本已有另一个标识的首版时，同样入账」一句已由 [ADR-0232](./0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) **部分停用**（2026-10-21）：这个费用项目上的链头依的是另一版发生项时，评价形成纠错版本；越权风险点 4 由它回答（被停原句与适用场景见索引注记），其余各条不变。<!-- 索引注记 -->｜**已接受**（2026-10-19，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 SA owner 裁定；票 demo-chain-gaps/08）。BUY·供应商成本评价按回指读回 BUY 请求，取请求上的合格来源引用形成首版，版本标识 `BUYSC-<请求标识>`；不可计价、评价冲突、待判断、评价失败、缺换算步骤、卡没声明金额取整一律入账不重投、不改记任何册；没有回指零写入；回指不在册、派生版本已有另一份内容、已完成却未受理上抛。去向与 ADR-0198 同构。越权风险点待 SA owner 复核。｜**部分停用**（2026-10-21）：决定三「评价停下的各格入账，不重投，不改记任何册」中「不改记任何册」已由 [ADR-0229](./0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 停用——采用不了的各格在 BUY 请求册上记停因，补登参数之后另起新一代请求续办；入账不重投照旧，越权风险点 1、3 由 ADR-0229 作答。其余各条不变｜**发生项新版本驱动纠错**（2026-10-21）：越权风险点 4「同三维已有首版即止」由 [ADR-0232](./0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) 回答——形成之前在发生项锁下读成本状态：依已失效那一版的评价不形成成本（`OCCURRENCE_INVALIDATED`、入账），这个费用项目上已有链、链头依的是另一版发生项时按新评价重述全额、接在链头后面（原因 `OCCURRENCE_SUPERSEDED`），金额同也追加。**被停条款**：决定三「同一发生项、费用项目与规则版本已有另一个标识的首版时，同样入账」。**适用场景**：只限链头依的是另一版发生项；同一版发生项上的第二份请求撞上首版照旧答已有首版、入账
```

合并后：

```text
Status: Accepted（2026-10-19，Claude Code 会话代 SA owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/08。越权风险点见文末，待 SA owner 复核。）<!-- 索引注记 -->｜BUY·供应商成本评价按回指读回 BUY 请求，取请求上的合格来源引用形成首版，版本标识 `BUYSC-<请求标识>`；不可计价、评价冲突、待判断、评价失败、缺换算步骤、卡没声明金额取整一律入账不重投、不改记任何册；没有回指零写入；回指不在册、派生版本已有另一份内容、已完成却未受理上抛。去向与 ADR-0198 同构｜**部分停用**（2026-10-21）：其决定三「评价停下的各格入账，不重投，不改记任何册」中「不改记任何册」已由 [ADR-0229](./0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 停用——采用不了的各格在 BUY 请求册上记停因，补登参数之后另起新一代请求续办；入账不重投照旧，越权风险点 1、3 由 ADR-0229 作答。其余各条不变｜**部分停用**（2026-10-21，发生项新版本驱动纠错）：越权风险点 4「同三维已有首版即止」由 [ADR-0232](./0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) 回答——形成之前在发生项锁下读成本状态：依已失效那一版的评价不形成成本（`OCCURRENCE_INVALIDATED`、入账），这个费用项目上已有链、链头依的是另一版发生项时按新评价重述全额、接在链头后面（原因 `OCCURRENCE_SUPERSEDED`），金额同也追加。**被停条款**：其决定三「同一发生项、费用项目与规则版本已有另一个标识的首版时，同样入账」。**适用场景**：只限链头依的是另一版发生项；同一版发生项上的第二份请求撞上首版照旧答已有首版、入账
```

### ADR-0201：运输机会——周期安排按日期物化成班次，初始路由形成后逐段准备，空册答未配置

处理：来向两写合并；接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 [demo-chain-gaps/16](../../.scratch/demo-chain-gaps/issues/16-transport-schedule-templates-and-concrete-schedules-are-registered.md) 起草并落决定一至五，决定三的容量声明与决定一的容量池由票 17 落，决定六至八由票 18 落。越权风险点见文末，待 TF owner 复核，决定二与决定七待 NR owner 会签。）部分停用：[ADR-0217](./0217-route-plan-carries-a-delivery-leg-after-the-delivery-node.md)（被停条款：决定二末句「与计划履约段的比对……对的是 `PlannedLeg.From/To` 与窗口依据里的连接代码」只对节点间段成立，计划末尾的交付段不比对、不准备；适用场景见 adr/README 该行，其余各条不变，本记录正文不改写）<!-- 索引注记 -->｜**已接受**（2026-10-19，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 TF owner 裁定，决定二、七待 NR owner 会签；票 demo-chain-gaps/16 起草，17、18 落其余条款）。TF 自有周期安排册，周期安排、班次与容量池走登记册配置写、CLI 与端点同一用例；方向身份是 NR 连接代码加起讫节点，自由串方向退役；周期文法只内置按周一种，到达用历时；同键同内容答已存在、异内容成下一版；班次身份由（周期安排代码、运行日期）派生，物化只在准备评估候选与显式登记两处，另可登临时班次；该连接空册答未配置并入账、不建占位班次；准备由初始路由形成触发、结果唯一、不发信封；需求量取申报测量。越权风险点待 TF owner 复核。｜**部分停用**：决定二末句「与计划履约段的比对只在 TF 的 `adapters/networkrouting` 里做，对的是 `PlannedLeg.From/To` 与窗口依据里的连接代码（票 18）」已由 [ADR-0217](./0217-route-plan-carries-a-delivery-leg-after-the-delivery-node.md) 停用，**适用场景**：只限计划末尾的交付段——它没有终点节点、窗口依据不指连接，不比对、不形成准备结果；节点间段照原句；自票 demo-chain-gaps/64 进 main 起生效；其余各条不变
```

合并后：

```text
Status: Accepted（2026-10-19，Claude Code 会话代 TF owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 [demo-chain-gaps/16](../../.scratch/demo-chain-gaps/issues/16-transport-schedule-templates-and-concrete-schedules-are-registered.md) 起草并落决定一至五，决定三的容量声明与决定一的容量池由票 17 落，决定六至八由票 18 落。越权风险点见文末，待 TF owner 复核，决定二与决定七待 NR owner 会签。）<!-- 索引注记 -->｜TF 自有周期安排册，周期安排、班次与容量池走登记册配置写、CLI 与端点同一用例；方向身份是 NR 连接代码加起讫节点，自由串方向退役；周期文法只内置按周一种，到达用历时；同键同内容答已存在、异内容成下一版；班次身份由（周期安排代码、运行日期）派生，物化只在准备评估候选与显式登记两处，另可登临时班次；该连接空册答未配置并入账、不建占位班次；准备由初始路由形成触发、结果唯一、不发信封；需求量取申报测量｜**部分停用**：决定二末句「与计划履约段的比对只在 TF 的 `adapters/networkrouting` 里做，对的是 `PlannedLeg.From/To` 与窗口依据里的连接代码（票 18）」已由 [ADR-0217](./0217-route-plan-carries-a-delivery-leg-after-the-delivery-node.md) 停用，**适用场景**：只限计划末尾的交付段——它没有终点节点、窗口依据不指连接，不比对、不形成准备结果；节点间段照原句；自票 demo-chain-gaps/64 进 main 起生效；其余各条不变，本记录正文不改写
```

### ADR-0202：运输委托、订舱与承运答复的入口族与协议核对——委托与订舱各口每口一种运营决定，承运答复走集成客户端，委托在提交时点核供应商协议并从协议快照采购责任法人

处理：来向两写合并；接受出处两写合并；补指针

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 [demo-chain-gaps/23](../../.scratch/demo-chain-gaps/issues/23-transport-commission-submission-face.md) 起草并落决定一（委托提交那一种）、三、四、六、七，决定二与决定五由票 24 落，决定一其余各种由票 24、25 落。越权风险点见文末，待 TF、access-identity owner 复核，PC owner 知会。票 24 落地时的补充裁定见文末补记。）越权风险点 14 已由 [ADR-0239](./0239-a-carrier-answer-from-a-non-provider-is-answered-booking-not-found-and-the-basis-is-kept-on-a-bounce-register.md) 回答（2026-10-21）：不是本服务方的答复方改答订舱不在册，挡回依据记进承运答复挡回册；由此**部分停用**文末补记「承运答复的次序与格」一格（被停原句见索引注记），其余各条不变。<!-- 索引注记 -->｜**已接受**（2026-10-19，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 TF、access-identity owner 裁定，PC owner 知会；票 demo-chain-gaps/23 起草，24、25 落其余条款）。提交委托、提交订舱、取消委托、取消订舱、段采用委托各是一种运营决定；承运答复走集成客户端，新外部事实类型 `CARRIER_BOOKING_ANSWER`，客户端来源身份须等于服务提供方（补 ADR-0149 决定三的列举，其各条不变）；委托在登记方给的提交时点按「在册 → 支持采购 → 供应商即服务方」核协议，四个否定格答 200、PC 读不动答未决，快照与采购责任法人取自核过的那一版、不收登记方给的；条件、角色、责任仍是声明的引用；重放判在核协议之前；订舱引在册班次、答复加候补；委托信封入队不登路由（ADR-0066），信封 ID 取指纹。协议核对适配器里的检查次序与服务方比较裁为翻译、不是 ADR-0025 的例外；区间更正（ADR-0038）未折进点读，记为已知缺口。越权风险点待 TF、access-identity、PC owner 复核。｜**非服务方答复并入订舱不在册**（2026-10-21）：越权风险点 14 由 [ADR-0239](./0239-a-carrier-answer-from-a-non-provider-is-answered-booking-not-found-and-the-basis-is-kept-on-a-bounce-register.md) 回答——答复方的来源身份不是订舱所属委托的服务提供方时答 `BOOKING_NOT_FOUND`，与订舱不在册同一格、响应体逐字相同；挡回依据（`BOOKING_NOT_REGISTERED`、`SOURCE_NOT_SERVICE_PROVIDER`）与那一次答复同笔记进承运答复挡回册；订舱连同服务方一条语句读回，两条挡回路读写不动答同一个未决原因。**被停条款**：文末补记「承运答复的次序与格」里「答复方的来源身份等于订舱所属委托的服务提供方，否则 `SOURCE_NOT_SERVICE_PROVIDER`」的「否则 `SOURCE_NOT_SERVICE_PROVIDER`」那一格。**适用场景**：只限承运答复口 `/transport-fulfillment/booking-answers`；来源取自绑定、来源核对排在重放之前、字面比、不回显在册答复、委托读不回为数据违约与同条其余各句不变，决定二「不等答不受理」不变，其余各条不变
```

合并后：

```text
Status: Accepted（2026-10-19，Claude Code 会话代 TF、access-identity owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 [demo-chain-gaps/23](../../.scratch/demo-chain-gaps/issues/23-transport-commission-submission-face.md) 起草并落决定一（委托提交那一种）、三、四、六、七，决定二与决定五由票 24 落，决定一其余各种由票 24、25 落。越权风险点见文末，待 TF、access-identity owner 复核，PC owner 知会，标 PC owner 的那几点待 PC owner 复核。票 24 落地时的补充裁定见文末补记。）<!-- 索引注记 -->｜提交委托、提交订舱、取消委托、取消订舱、段采用委托各是一种运营决定；承运答复走集成客户端，新外部事实类型 `CARRIER_BOOKING_ANSWER`，客户端来源身份须等于服务提供方（补 ADR-0149 决定三的列举，其各条不变）；委托在登记方给的提交时点按「在册 → 支持采购 → 供应商即服务方」核协议，四个否定格答 200、PC 读不动答未决，快照与采购责任法人取自核过的那一版、不收登记方给的；条件、角色、责任仍是声明的引用；重放判在核协议之前；订舱引在册班次、答复加候补；委托信封入队不登路由（ADR-0066），信封 ID 取指纹。协议核对适配器里的检查次序与服务方比较裁为翻译、不是 ADR-0025 的例外；区间更正（ADR-0038）未折进点读，记为已知缺口｜**非服务方答复并入订舱不在册**（2026-10-21）：越权风险点 14 由 [ADR-0239](./0239-a-carrier-answer-from-a-non-provider-is-answered-booking-not-found-and-the-basis-is-kept-on-a-bounce-register.md) 回答——答复方的来源身份不是订舱所属委托的服务提供方时答 `BOOKING_NOT_FOUND`，与订舱不在册同一格、响应体逐字相同；挡回依据（`BOOKING_NOT_REGISTERED`、`SOURCE_NOT_SERVICE_PROVIDER`）与那一次答复同笔记进承运答复挡回册；订舱连同服务方一条语句读回，两条挡回路读写不动答同一个未决原因。**被停条款**：文末补记「承运答复的次序与格」里「答复方的来源身份等于订舱所属委托的服务提供方，否则 `SOURCE_NOT_SERVICE_PROVIDER`」的「否则 `SOURCE_NOT_SERVICE_PROVIDER`」那一格。**适用场景**：只限承运答复口 `/transport-fulfillment/booking-answers`；来源取自绑定、来源核对排在重放之前、字面比、不回显在册答复、委托读不回为数据违约与同条其余各句不变，决定二「不等答不受理」不变，其余各条不变｜**越权风险点 5 已答**（指针 2026-10-21 补）：越权风险点 5「区间更正不折进核对」由 [ADR-0233](./0233-an-older-commercial-version-is-closed-for-a-named-successor-by-a-cut-or-a-same-start-retirement.md) 决定五回答——供应商协议的版本点读口与正文点读口都把壳上最新一条区间更正的终点折进正文的判用区间，运输履约的核对按既有分格把截止点之后读作未生效、把已退役读作已终止（越权风险点 7 的对法）；无条款停用，其余各条不变
```

### ADR-0203：容量保障——班次挂了容量池，装载分配与确认订舱就要对每个池引用有效预占并声明分配量；预占与释放是两种运营决定，声明需求量与来源，硬上限按预占整段判、带余量拒绝、不可审批越过

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 [demo-chain-gaps/20](../../.scratch/demo-chain-gaps/issues/20-capacity-reservation-and-release-decision-faces.md) 起草并落决定三至六，决定一、二由票 21 落装载分配那一半、票 37 落确认订舱那一半。越权风险点见文末，待 TF owner 复核，access-identity owner 知会。）<!-- 索引注记 -->｜**已接受**（2026-10-19，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 TF owner 裁定，access-identity owner 知会；票 demo-chain-gaps/20 起草并落决定三至六，21、37 落决定一、二）。保障判据只看班次挂没挂池：挂了就逐池引用当前有效预占并声明分配量，没挂不要求；分配关系守恒「已分配 ≤ 预占 − 释放 − 消耗」、撤回归还、不调消耗、池班次须等于分配班次。容量预占、容量释放各是一种运营决定，都声明数量、单位与来源（运营声明版本或准备结果的需求快照两种），单位须等于池的单位、不换算；预占从判断时点占到有效期满，硬上限按这一整段最紧那一刻判，超出答 `CAPACITY_EXCEEDED` 带余量、没有越过的格；读池行锁，并发预占串行判；预占与释放各有池内唯一的引用，同内容答已有、异内容答冲突；池不在册、预占不在册、单位不符各成一格。越权风险点待 TF owner 复核。
```

合并后：

```text
Status: Accepted（2026-10-19，Claude Code 会话代 TF owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 [demo-chain-gaps/20](../../.scratch/demo-chain-gaps/issues/20-capacity-reservation-and-release-decision-faces.md) 起草并落决定三至六，决定一、二由票 21 落装载分配那一半、票 37 落确认订舱那一半。越权风险点见文末，待 TF owner 复核，access-identity owner 知会。）<!-- 索引注记 -->｜保障判据只看班次挂没挂池：挂了就逐池引用当前有效预占并声明分配量，没挂不要求；分配关系守恒「已分配 ≤ 预占 − 释放 − 消耗」、撤回归还、不调消耗、池班次须等于分配班次。容量预占、容量释放各是一种运营决定，都声明数量、单位与来源（运营声明版本或准备结果的需求快照两种），单位须等于池的单位、不换算；预占从判断时点占到有效期满，硬上限按这一整段最紧那一刻判，超出答 `CAPACITY_EXCEEDED` 带余量、没有越过的格；读池行锁，并发预占串行判；预占与释放各有池内唯一的引用，同内容答已有、异内容答冲突；池不在册、预占不在册、单位不符各成一格。
```

### ADR-0204：履约段采用运输委托——声明式连线，采用即以段的首个参与标委托开始，开始之后不可取消

处理：来向两写合并；接受出处两写合并；补指针

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 [demo-chain-gaps/25](../../.scratch/demo-chain-gaps/issues/25-segment-adopts-transport-commission-and-cancellation-faces.md) 起草并落全部条款。部分停用 [ADR-0098](./0098-a-failed-attempt-charge-occurrence-is-not-formed-inside-the-pickup-orchestration.md) 决定四，适用场景只限实际履约段。越权风险点见文末，待 TF owner 复核，access-identity owner 知会。）**部分停用**（2026-10-21）：决定一「服务方、协议快照与采购责任法人的权威仍是委托那一行」一句与决定二末条「采用不入队」已由 [ADR-0231](./0231-actual-fulfillment-left-unformed-is-caught-up-by-doorbells-and-a-commission-agreement-revision.md) 停用（被停原句与适用场景见索引注记），其余各条不变。 决定一末两句与决定二「只前移、不后移」已由 [ADR-0232](./0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) **部分停用**（2026-10-21）：解除采用立为一种运营决定，委托的开始随之撤回或按其余已采用段重新派生；越权风险点 2 由它回答（被停原句与适用场景见索引注记），其余各条不变。<!-- 索引注记 -->｜**已接受**（2026-10-19，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 TF owner 裁定，access-identity owner 知会；票 demo-chain-gaps/25）。履约段采用运输委托是一种运营决定，声明（段、委托、服务方）：段在册、委托在册、服务方等于委托的服务提供方、委托未取消（取消不早于段的首个参与另答取消晚于开始）、段有当前参与且首个参与不早于委托提交，不满足的各答一格；一段至多采用一份委托，同段原样再交答已有、换委托答冲突，采用之后不改不解除，一份委托可被多段采用。采用即以段的首个当前参与（替代与失效版本不算）标委托开始，委托的开始取已采用各段首个参与里最早的那一条、只前移；连线先落、委托后写，同一笔，采用成立以外整笔回滚。取消委托、取消订舱两面就此开放：已开始答 `TRANSPORT_ALREADY_STARTED`，订舱以申请不晚于委托开始判已开始，重放答 `EXISTING_CANCELLATION`；采用与取消的先后按业务时间判，判出取消晚于开始只挂起待更正、不自动撤回（对 UC-TF-004 一条规则的有意收窄）。连线是声明不是推导，没有连线的段不读作自营。部分停用 ADR-0098 决定四，限实际履约段，揽收任务与尝试 ↔ 委托仍不建。越权风险点待 TF owner 复核。｜**部分停用**（2026-10-21）：决定一「服务方、协议快照与采购责任法人的权威仍是委托那一行，连线上另存的那一份是声明原样」一句、决定二末条「采用不入队：今天没有它的消费者（计划「跨票通则」X3）」已由 [ADR-0231](./0231-actual-fulfillment-left-unformed-is-caught-up-by-doorbells-and-a-commission-agreement-revision.md) 停用——委托协议换版让委托出新一版，事实按业务时间取委托那一刻所凭的那一版；采用成立的那一笔交实际履约补形成门铃。**适用场景**：前一句只限协议快照与采购责任法人两格，权威仍是委托、换过版之后是委托上业务时间所凭的那一版，服务方与「连线上另存的那一份是声明原样」不变；后一条全部停用，「运输收费发生项登记按段回读连线」不变；其余各条不变｜**解除采用**（2026-10-21）：越权风险点 2「没有反向解除采用的面」由 [ADR-0232](./0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) 回答——运营决定「解除履约段采用」只在段上还没有按那份委托形成仍有效的发生项时允许，解除另记一行、采用那一行不改，同段可以再采用（下一代）；委托再无别的已采用段时撤回到已提交，还有时按其余段采用上记的开始取最早的那一条。**被停条款**：决定一「采用之后不改、不解除：册上只有插入。没有反向解除采用的面」里的「不解除」与「没有反向解除采用的面」；决定二「只前移、不后移」与「委托始终是已开始」。**适用场景**：只限解除采用——「采用之后不改」「册上只有插入」照旧成立，采用那一路照旧只前移；越权风险点 1、8 未答的部分拆到票 demo-chain-gaps/162–164
```

合并后：

```text
Status: Accepted（2026-10-19，Claude Code 会话代 TF owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 [demo-chain-gaps/25](../../.scratch/demo-chain-gaps/issues/25-segment-adopts-transport-commission-and-cancellation-faces.md) 起草并落全部条款。部分停用 [ADR-0098](./0098-a-failed-attempt-charge-occurrence-is-not-formed-inside-the-pickup-orchestration.md) 决定四，适用场景只限实际履约段，揽收任务与尝试 ↔ 委托仍不建。越权风险点见文末，待 TF owner 复核，access-identity owner 知会。）<!-- 索引注记 -->｜履约段采用运输委托是一种运营决定，声明（段、委托、服务方）：段在册、委托在册、服务方等于委托的服务提供方、委托未取消（取消不早于段的首个参与另答取消晚于开始）、段有当前参与且首个参与不早于委托提交，不满足的各答一格；一段至多采用一份委托，同段原样再交答已有、换委托答冲突，采用之后不改不解除，一份委托可被多段采用。采用即以段的首个当前参与（替代与失效版本不算）标委托开始，委托的开始取已采用各段首个参与里最早的那一条、只前移；连线先落、委托后写，同一笔，采用成立以外整笔回滚。取消委托、取消订舱两面就此开放：已开始答 `TRANSPORT_ALREADY_STARTED`，订舱以申请不晚于委托开始判已开始，重放答 `EXISTING_CANCELLATION`；采用与取消的先后按业务时间判，判出取消晚于开始只挂起待更正、不自动撤回（对 UC-TF-004 一条规则的有意收窄）。连线是声明不是推导，没有连线的段不读作自营｜**部分停用**（2026-10-21）：其决定一「服务方、协议快照与采购责任法人的权威仍是委托那一行，连线上另存的那一份是声明原样」一句、决定二末条「采用不入队：今天没有它的消费者（计划「跨票通则」X3）」已由 [ADR-0231](./0231-actual-fulfillment-left-unformed-is-caught-up-by-doorbells-and-a-commission-agreement-revision.md) 停用——委托协议换版让委托出新一版，事实按业务时间取委托那一刻所凭的那一版；采用成立的那一笔交实际履约补形成门铃。**适用场景**：前一句只限协议快照与采购责任法人两格，权威仍是委托、换过版之后是委托上业务时间所凭的那一版，服务方与「连线上另存的那一份是声明原样」不变；后一条全部停用，「运输收费发生项登记按段回读连线」不变；其余各条不变｜**部分停用**（2026-10-21，解除采用）：越权风险点 2「没有反向解除采用的面」由 [ADR-0232](./0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) 回答——运营决定「解除履约段采用」只在段上还没有按那份委托形成仍有效的发生项时允许，解除另记一行、采用那一行不改，同段可以再采用（下一代）；委托再无别的已采用段时撤回到已提交，还有时按其余段采用上记的开始取最早的那一条。**被停条款**：其决定一「采用之后不改、不解除：册上只有插入。没有反向解除采用的面」里的「不解除」与「没有反向解除采用的面」；决定二「只前移、不后移」与「委托始终是已开始」。**适用场景**：只限解除采用——「采用之后不改」「册上只有插入」照旧成立，采用那一路照旧只前移；越权风险点 1、8 未答的部分拆到票 demo-chain-gaps/162–164｜**读法注**（指针 2026-10-21 补）：「自营只能显式声明（ADR-0207 的读法不变）」一句只指不由有没有委托推断自营；[ADR-0207](./0207-transport-charge-occurrences-are-registered-by-an-operation-decision-and-handed-off-in-the-same-transaction.md) 决定二另有「那一段当前的实际承运商判断是自营法人」一格认自营，照它读（其文末补记把那一格收窄到有采用连线的段）；无条款停用，其余各条不变
```

### ADR-0205：出发门禁是拉取的读数——与移动事实同笔落册，不拒收；移动事实带声明的段

处理：来向两写合并；接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/32，统一代决见 `.scratch/demo-chain-gaps/tf-chain-plan.md`「05 移动事实与出发门禁」D05-1、D05-2、D05-6 至 D05-10 与「需要的 ADR」ADR-0205。越权风险点见文末，待各条括注的 owner 复核。）越权风险点 1「挡下要靠设备在动作之前问；那个读口……等 peer 收工后另立票」已由 [ADR-0219](./0219-a-device-previews-the-departure-gate-before-departing-and-reads-what-the-departure-will-record.md) 回答（2026-10-19）：作业事实渠道开只读问询，交回与随后同参出发同一套读数，运输履约不落记录；出发照本记录在它自己那一刻重问并落册，本记录各条不变。越权风险点 14「段日后才登记，那一条事实不回补投影」已由 [ADR-0224](./0224-tracking-backfills-movement-facts-recorded-before-their-segment-when-a-control-fact-puts-the-object-in-it.md) 回答（2026-10-20）：对象凭`已交接`或对象级场外揽收进段时，全程追踪按段列出声明了这一段的在册移动事实、以同一判据同一幂等键补派生，只补投影；本记录各条不变。越权风险点 5「对象只取段里的参与，不展开集运单元」已由 [ADR-0225](./0225-the-departure-gate-reading-asks-through-consolidation-unit-members-at-the-moment-of-departure.md) 回答（2026-10-20）：段里在控的集运单元按出发那一刻的直接成员逐件问关务，单元本身不问，它那一格取成员里最重的一格；由此**部分停用**决定一、决定二各一句，仅限单元那一格（被停原句见索引注记），其余各条不变。 越权风险点 13「执行方看得到同段每件的那一格」已由 [ADR-0237](./0237-gate-readings-show-the-executor-only-the-objects-that-did-not-pass.md) 回答（2026-10-21）：回给报事实一方与问询方的逐对象那一格只列没得出放行的对象（阻断、读不动、未决），集运单元只指名同样列出的成员，落册照旧记全；同一记录决定五让出发意图的起讫与声明的段绑定——段里各件进段交接声明了的交出、去向节点与班次起讫对不上就停在本侧、不问关务，说不出（凭收寄进段、没声明全）照问关务、落册记全，只是回给设备的不列对象；由此**部分停用**决定一一句与决定二一句（被停原句见索引注记），起讫仍取班次方向，其余各条不变。<!-- 索引注记 -->｜**已接受**（2026-10-19，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 TF owner 裁定；票 demo-chain-gaps/32）。**出发一律落册**：门禁是同一笔保存的封闭五格读数（放行 / 阻断 / 不受门禁 / 未决 / 读不动），退役「门禁未放行、不落库」那一格（出自票 tf-unwired-seven/06，不是 ADR）。**拉取**：写事务之前经消费侧端口逐对象问关务动作准入判断，起讫取班次方向、时点取出发业务时间，只翻译；关务未配置落未决；本侧缺段、缺对象、缺班次停下不问；整格取最重（次序只在决定二）；调用失败与译不回来分记依据种类。回给设备的答复只带各格，关务判断标识与依据原文只落册。**设备字段**：`gateRequired` 不读，`gateClearance` 只作证据。**段**：登记方声明、不加外键、不在册照记，对象取出发那一刻段里在控的参与，不经装载分配推；不核班次在册。读数不进内容摘要，重放回显首次读数、不重问关务；存量出发回填为「登记于门禁读数之前」。信封与 VE 消费者同一变更上线（票 33）。越权风险点待 TF owner 复核，部分同时待 CC、NO、VE owner。｜**出发前读口已答**（2026-10-19）：越权风险点 1「挡下要靠设备在动作之前问；那个读口……等 peer 收工后另立票」由 [ADR-0219](./0219-a-device-previews-the-departure-gate-before-departing-and-reads-what-the-departure-will-record.md) 回答——作业事实渠道开只读问询，交回与随后同参出发同一套读数，运输履约不落记录，出发照旧在它自己那一刻重问并落册；无条款停用，其余各条不变｜**段后登的补派生已答**（2026-10-20）：越权风险点 14「段日后才登记，那一条事实不回补投影」由 [ADR-0224](./0224-tracking-backfills-movement-facts-recorded-before-their-segment-when-a-control-fact-puts-the-object-in-it.md) 回答——对象凭`已交接`或对象级场外揽收进段时全程追踪经运输履约按段列出的只读口找回声明了这一段的在册移动事实，在控判据与幂等键同正常路径，只补投影、不补信号、不重问关务；揽收执行口与承运商段另票；无条款停用，其余各条不变｜**部分停用**（2026-10-20）：越权风险点 5「对象只取段里的参与，不展开集运单元」由 [ADR-0225](./0225-the-departure-gate-reading-asks-through-consolidation-unit-members-at-the-moment-of-departure.md) 回答——段里在控的集运单元按出发那一刻的直接成员逐件问关务，单元那一格取成员里最重的一格，成员各自一格进逐对象明细，出发前问询同受。**被停条款**：决定一「回给报事实一方的答复只带整格、起讫、停点与逐对象那一格（对象与它的格）」——单元那一格另带它的成员；决定二「逐对象问，每件一格：关务的答复译成本上下文的五格」——单元那一格不问关务、由成员推出。**适用场景**：仅限段里在控或展开出来的对象是集运单元时那一件；包裹照旧逐件问关务、答复照旧只有对象与格。其余各条不变｜**部分停用**（2026-10-21）：越权风险点 13「执行方看得到同段每件的那一格」由 [ADR-0237](./0237-gate-readings-show-the-executor-only-the-objects-that-did-not-pass.md) 回答——作业事实渠道上回给设备的门禁读数，逐对象那一格只列没得出放行的对象（阻断、读不动、未决），集运单元只指名同样列出的成员，判法只在领域 `ExecutorGateCells`；落册、信封、VE 与运营查阅不收窄；重放回显首次读数，回给设备时同样只列没得出放行的。**被停条款**：决定一「回给报事实一方的答复只带整格、起讫、停点与逐对象那一格（对象与它的格）……执行方要知道车上被挡的是哪一件，所以逐对象那一格照给」里逐对象那一格照给每件那一半——只给没得出放行的那几件。**适用场景**：只限回给报事实一方或问询方的答复；落册的那一份照旧每件一格，整格、起讫、停点照给。**另停一句**：决定二「本上下文这一侧找不齐问关务要的东西就停下、不问关务，停点只属一格」之下的停点名单——多「班次不服务该段」`SCHEDULE_NOT_SERVING_SEGMENT` 一种未决与「交接册读不动」`HANDOVER_REGISTRY_UNAVAILABLE` 一种读不动（ADR-0237 决定五：段里出发那一刻在控各件进段那次`已交接`声明了的交出、去向节点与班次起讫对不上就不问；说不出的照问，读数标 `entryUnbound`）；适用场景是出发读数（出发登记与出发前问询）。其余各条不变
```

合并后：

```text
Status: Accepted（2026-10-19，Claude Code 会话代 TF owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/32，统一代决见 `.scratch/demo-chain-gaps/tf-chain-plan.md`「05 移动事实与出发门禁」D05-1、D05-2、D05-6 至 D05-10 与「需要的 ADR」ADR-0205。越权风险点见文末，待各条括注的 owner 复核（TF owner，部分同时待 CC、NO、VE owner）。）<!-- 索引注记 -->｜**出发一律落册**：门禁是同一笔保存的封闭五格读数（放行 / 阻断 / 不受门禁 / 未决 / 读不动），退役「门禁未放行、不落库」那一格（出自票 tf-unwired-seven/06，不是 ADR）。**拉取**：写事务之前经消费侧端口逐对象问关务动作准入判断，起讫取班次方向、时点取出发业务时间，只翻译；关务未配置落未决；本侧缺段、缺对象、缺班次停下不问；整格取最重（次序只在决定二）；调用失败与译不回来分记依据种类。回给设备的答复只带各格，关务判断标识与依据原文只落册。**设备字段**：`gateRequired` 不读，`gateClearance` 只作证据。**段**：登记方声明、不加外键、不在册照记，对象取出发那一刻段里在控的参与，不经装载分配推；不核班次在册。读数不进内容摘要，重放回显首次读数、不重问关务；存量出发回填为「登记于门禁读数之前」。信封与 VE 消费者同一变更上线（票 33）｜**出发前读口已答**（2026-10-19）：越权风险点 1「挡下要靠设备在动作之前问；那个读口……等 peer 收工后另立票」由 [ADR-0219](./0219-a-device-previews-the-departure-gate-before-departing-and-reads-what-the-departure-will-record.md) 回答——作业事实渠道开只读问询，交回与随后同参出发同一套读数，运输履约不落记录，出发照旧在它自己那一刻重问并落册；无条款停用，其余各条不变｜**段后登的补派生已答**（2026-10-20）：越权风险点 14「段日后才登记，那一条事实不回补投影」由 [ADR-0224](./0224-tracking-backfills-movement-facts-recorded-before-their-segment-when-a-control-fact-puts-the-object-in-it.md) 回答——对象凭`已交接`或对象级场外揽收进段时全程追踪经运输履约按段列出的只读口找回声明了这一段的在册移动事实，在控判据与幂等键同正常路径，只补投影、不补信号、不重问关务；揽收执行口与承运商段另票；无条款停用，其余各条不变｜**部分停用**（2026-10-20）：越权风险点 5「对象只取段里的参与，不展开集运单元」由 [ADR-0225](./0225-the-departure-gate-reading-asks-through-consolidation-unit-members-at-the-moment-of-departure.md) 回答——段里在控的集运单元按出发那一刻的直接成员逐件问关务，单元那一格取成员里最重的一格，成员各自一格进逐对象明细，出发前问询同受。**被停条款**：决定一「回给报事实一方的答复只带整格、起讫、停点与逐对象那一格（对象与它的格）」——单元那一格另带它的成员；决定二「逐对象问，每件一格：关务的答复译成本上下文的五格」——单元那一格不问关务、由成员推出。**适用场景**：仅限段里在控或展开出来的对象是集运单元时那一件；包裹照旧逐件问关务、答复照旧只有对象与格。其余各条不变｜**部分停用**（2026-10-21）：越权风险点 13「执行方看得到同段每件的那一格」由 [ADR-0237](./0237-gate-readings-show-the-executor-only-the-objects-that-did-not-pass.md) 回答——作业事实渠道上回给设备的门禁读数，逐对象那一格只列没得出放行的对象（阻断、读不动、未决），集运单元只指名同样列出的成员，判法只在领域 `ExecutorGateCells`；落册、信封、VE 与运营查阅不收窄；重放回显首次读数，回给设备时同样只列没得出放行的。**被停条款**：决定一「回给报事实一方的答复只带整格、起讫、停点与逐对象那一格（对象与它的格）……执行方要知道车上被挡的是哪一件，所以逐对象那一格照给」里逐对象那一格照给每件那一半——只给没得出放行的那几件。**适用场景**：只限回给报事实一方或问询方的答复；落册的那一份照旧每件一格，整格、起讫、停点照给，起讫仍取班次方向。**另停一句**：决定二「本上下文这一侧找不齐问关务要的东西就停下、不问关务，停点只属一格」之下的停点名单——多「班次不服务该段」`SCHEDULE_NOT_SERVING_SEGMENT` 一种未决与「交接册读不动」`HANDOVER_REGISTRY_UNAVAILABLE` 一种读不动（ADR-0237 决定五：段里出发那一刻在控各件进段那次`已交接`声明了的交出、去向节点与班次起讫对不上就不问；说不出的照问，读数标 `entryUnbound`）；适用场景是出发读数（出发登记与出发前问询）。其余各条不变
```

### ADR-0206：关务动作准入判断——一口折叠限制、门禁核对、外部放行三源，适用哪些受门禁动作由关务按节点关务区域判

处理：来向两写合并；接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/31，统一代决见 `.scratch/demo-chain-gaps/tf-chain-plan.md`「05 移动事实与出发门禁」D05-2 至 D05-5 与「需要的 ADR」ADR-0206。越权风险点见文末，待各条括注的 owner 复核。）Consequences「外包干线交接……与有效交付……的门禁按交出、交付意图读本判断，挂法等 peer」一句与越权风险点 9 已由 [ADR-0213](./0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md) 回答（2026-10-19）：交接声明交出、去向节点，交出意图取它们；交付意图的 at 取进入派送段那次交接的交出节点。无条款停用，本记录各条不变。决定六「消费方把答复随自己的事实同笔保存」说的是带着事实来问的消费方；没有事实可随的出发前门禁问询由 [ADR-0219](./0219-a-device-previews-the-departure-gate-before-departing-and-reads-what-the-departure-will-record.md) 定（2026-10-19）：问询方只翻译交回、不保存，本上下文照决定六记判断，随后同参出发在同样三源上答`已有判断`。无条款停用，本记录各条不变。越权风险点 8「对象只认包裹」的出发那一半已由 [ADR-0225](./0225-the-departure-gate-reading-asks-through-consolidation-unit-members-at-the-moment-of-departure.md) 回答（2026-10-20）：出发门禁读数按出发那一刻的成员展开集运单元，单元不再交给本判断；交出那一半仍按交接对象问。无条款停用，本记录各条不变。<!-- 索引注记 -->｜**已接受**（2026-10-19，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 CC owner 裁定；票 demo-chain-gaps/31）。**请求驱动**：消费方交对象、动作意图（出发 / 交出 / 交付）与动作时点，CC 定适用动作——出发对应装载出发，起讫节点关务区域不同另加跨关务区域移动；节点未登记而对象带跨区门时答未配置，否则不列。**新册**节点关务区域，租户取值（`PAR-CUS-08`）。**范围**由 CC 按自己的申报单元与案件册解析；不在册的对象跨关务区域移动答未决、其余动作答不受门禁。**三源折五格**（放行 / 阻断 / 不受门禁 / 未决 / 未配置）：限制一律阻断，门禁满足不答放行，只有部分或附条件放行答未决；门禁读法与放行门禁核对共用。**判断记录**按内容派生标识、可重放、不入队；`VerifyReleaseGate` 不变。越权风险点待 CC owner 复核。｜**挂法已答**（2026-10-19）：Consequences「外包干线交接……与有效交付……的门禁按交出、交付意图读本判断，挂法等 peer」与越权风险点 9 由 [ADR-0213](./0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md) 回答——交接声明交出、去向节点，交出意图取它们；交付意图的 at 取进入派送段那次交接的交出节点；无条款停用，其余各条不变｜**展开成员（出发）已答**（2026-10-20）：越权风险点 8「对象只认包裹」的出发那一半由 [ADR-0225](./0225-the-departure-gate-reading-asks-through-consolidation-unit-members-at-the-moment-of-departure.md) 回答——消费方按出发那一刻的直接成员逐件问、单元本身不交给本判断；交出读数仍按交接对象问，另票；无条款停用，其余各条不变
```

合并后：

```text
Status: Accepted（2026-10-19，Claude Code 会话代 CC owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/31，统一代决见 `.scratch/demo-chain-gaps/tf-chain-plan.md`「05 移动事实与出发门禁」D05-2 至 D05-5 与「需要的 ADR」ADR-0206。越权风险点见文末，待 CC owner 与各条括注的 owner 复核。）<!-- 索引注记 -->｜**请求驱动**：消费方交对象、动作意图（出发 / 交出 / 交付）与动作时点，CC 定适用动作——出发对应装载出发，起讫节点关务区域不同另加跨关务区域移动；节点未登记而对象带跨区门时答未配置，否则不列。**新册**节点关务区域，租户取值（`PAR-CUS-08`）。**范围**由 CC 按自己的申报单元与案件册解析；不在册的对象跨关务区域移动答未决、其余动作答不受门禁。**三源折五格**（放行 / 阻断 / 不受门禁 / 未决 / 未配置）：限制一律阻断，门禁满足不答放行，只有部分或附条件放行答未决；门禁读法与放行门禁核对共用。**判断记录**按内容派生标识、可重放、不入队；`VerifyReleaseGate` 不变｜**挂法已答**（2026-10-19）：Consequences「外包干线交接……与有效交付……的门禁按交出、交付意图读本判断，挂法等 peer」与越权风险点 9 由 [ADR-0213](./0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md) 回答——交接声明交出、去向节点，交出意图取它们；交付意图的 at 取进入派送段那次交接的交出节点；无条款停用，其余各条不变｜**出发前问询的保存已答**（2026-10-19）：决定六「消费方把答复随自己的事实同笔保存」说的是带着事实来问的消费方；没有事实可随的出发前门禁问询由 [ADR-0219](./0219-a-device-previews-the-departure-gate-before-departing-and-reads-what-the-departure-will-record.md) 定——问询方只翻译交回、不保存，本上下文照决定六记判断，随后同参出发在同样三源上答`已有判断`；无条款停用，其余各条不变｜**展开成员（出发）已答**（2026-10-20）：越权风险点 8「对象只认包裹」的出发那一半由 [ADR-0225](./0225-the-departure-gate-reading-asks-through-consolidation-unit-members-at-the-moment-of-departure.md) 回答——消费方按出发那一刻的直接成员逐件问、单元本身不交给本判断；交出读数仍按交接对象问，另票；无条款停用，其余各条不变
```

### ADR-0207：运输收费发生项由运营决定显式登记——事实取自事实本身，采购上下文由登记方给出、经协议核对，发生条件先由登记者断言，已形成信封与发生项同笔交出

处理：来向两写合并；接受出处两写合并；补指针

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/27，代决出自 `.scratch/demo-chain-gaps/tf-chain-plan.md` 的「07 运输收费发生项」一节与「需要的 ADR」里 ADR-0207 一条。落在 [ADR-0098](./0098-a-failed-attempt-charge-occurrence-is-not-formed-inside-the-pickup-orchestration.md) 决定二之内，其决定一、三不变。越权风险点见文末，待 TF owner 复核，第 1、4 点同时待 PC owner、第 3 点同时待 SA owner。**部分停用**：决定三「登记方交一句原话断言……缺了答未受理 `CONDITION_NOT_ASSERTED`」与「数量与单位同由登记方声明」两句、决定四「查与随后的落册在同一事务里按（租户、原因、事实依据）串行化」一句的键，已由 [ADR-0209](./0209-supplier-agreements-declare-charge-occurrence-conditions-and-registration-judges-by-them.md) 停用，适用场景见该记录「对已接受记录的作用」；其余各条不变，本记录正文不改写。票 demo-chain-gaps/28 落地订舱与取消两种原因与决定二末条时的补充裁定见文末补记，其中一条收窄决定二「不适用只有两格」的第二格（适用场景只限有采用连线的段），一条让决定一里 `REASON_NOT_REGISTRABLE` 一格退场。）**部分停用**（2026-10-19）：决定七后半句——标题里「也暂不自动形成」与正文「运输履约自消费交接与有效交付信封来自动形成，等 ADR-0209 与票 demo-chain-gaps/25 落地后另定」——与越权风险点 3「不自动形成」已由 [ADR-0218](./0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md) 停用，**适用场景**只限有采用连线的段；决定四末句「演示约定 `SYN-JRN-<包裹号>`」同由 ADR-0218 停用，演示租户改用 `JRN/<包裹号>`。决定七前半句、决定四其余各句不变，本记录正文不改写。文末补记「委托记下指名的那一版协议」一条里「发生项按它回问 PC」一句已由 [ADR-0231](./0231-actual-fulfillment-left-unformed-is-caught-up-by-doorbells-and-a-commission-agreement-revision.md) 部分停用（2026-10-21），适用场景见索引注记。<!-- 索引注记 -->｜**已接受**（2026-10-19，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 TF owner 裁定；票 demo-chain-gaps/27）。操作者渠道「运营决定」能力面加决定种类「运输收费发生项登记」；一个用例覆盖各原因，依据、范围与业务时间取自事实——失败尝试取尝试结果（派送经现有读口），实际履约取段内已结束的参与、业务时间取最晚终点，订舱与取消随票 28；采购上下文由登记方给出、按业务时间经 PC 核对，法人与「对象/版本」快照取自协议，四个否定格各成一格，缺了答未受理绝不答不适用，不适用只认显式声明自营与实际承运商判为自营；发生条件在 ADR-0209 之前由登记者断言并留痕；身份由（租户、原因、依据、成员集合）派生、首版 V1，同一事实上成员与另一项仍有效的发生项有交集答冲突；`transport-fulfillment.charge-occurrence.formed` 引用式三键、指纹 ID、与发生项同笔；不内联进物理编排、暂不自动形成。落在 ADR-0098 决定二之内，决定一、三不变。越权风险点含断言期、旅程无人核、不自动形成、数量单位由登记方声明｜**部分停用**：决定三「登记方交一句原话断言『采用协议的发生条件已满足』……缺了答未受理 `CONDITION_NOT_ASSERTED`」与「数量与单位同由登记方声明」两句已由 [ADR-0209](./0209-supplier-agreements-declare-charge-occurrence-conditions-and-registration-judges-by-them.md) 决定四停用，**适用场景**：本记录之后新形成的发生项——停用的理由是条件与数量形已由采用的协议声明，这一前提只对新登记成立，断言期已形成的项照旧按断言读；决定四「查与随后的落册在同一事务里按（租户、原因、事实依据）串行化」一句的键改为（租户、原因、主要业务范围），**适用场景**：全部登记——按范围计的一项要挡住同一范围上别的事实依据。其余各条不变｜**文末补记**（票 demo-chain-gaps/28）：订舱的依据是已接受的承运答复、取消的依据是尚未开始范围的订舱取消（只认订舱一级，按业务时间与 ADR-0204 决定三同一边界）；订舱与取消、以及有采用连线的实际履约段，采购上下文取委托快照，登记方可省略、给了而不一致答冲突；委托记下指名的协议版本（迁移 TF 0038）；`REASON_NOT_REGISTRABLE` 退场。**收窄**决定二「不适用只有两格」的第二格「那一段当前的实际承运商判断是自营法人」，**适用场景**：只限有采用连线的段——段的采购组织已由登记方声明，成本来源跟着采购走；没有连线的段照旧按两格判｜**部分停用**（[ADR-0218](./0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md)）：决定七后半句——「也暂不自动形成」与「运输履约自消费交接与有效交付信封来自动形成，等 ADR-0209 与票 demo-chain-gaps/25 落地后另定」——与越权风险点 3「不自动形成」，**适用场景**：只限有采用连线的段——停用的理由带前提「采购上下文已由委托给出、条件已由协议声明」，没有连线的段仍由人工登记；决定四末句「演示约定 `SYN-JRN-<包裹号>`」，**适用场景**：演示租户自 ADR-0218 起改用 `JRN/<包裹号>`。决定七前半句「不在任何物理编排里内联」与其余各条不变｜**部分停用**（2026-10-21）：文末补记「委托记下指名的那一版协议」一条里「发生项按它回问 PC」一句已由 [ADR-0231](./0231-actual-fulfillment-left-unformed-is-caught-up-by-doorbells-and-a-commission-agreement-revision.md) 决定四停用——委托换过版之后，回问的是起用不晚于事实业务时间的那一次换版的后继。**适用场景**：只限换过版、且有换版起用不晚于业务时间的委托；没换过版或业务时间早于各次换版起用的，照旧是提交时记下的那一版；补记其余各句与决定七前半句不变
```

合并后：

```text
Status: Accepted（2026-10-19，Claude Code 会话代 TF owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/27，代决出自 `.scratch/demo-chain-gaps/tf-chain-plan.md` 的「07 运输收费发生项」一节与「需要的 ADR」里 ADR-0207 一条。落在 [ADR-0098](./0098-a-failed-attempt-charge-occurrence-is-not-formed-inside-the-pickup-orchestration.md) 决定二之内，其决定一、三不变。越权风险点见文末，待 TF owner 复核，第 1、4 点同时待 PC owner、第 3 点同时待 SA owner。）<!-- 索引注记 -->｜操作者渠道「运营决定」能力面加决定种类「运输收费发生项登记」；一个用例覆盖各原因，依据、范围与业务时间取自事实——失败尝试取尝试结果（派送经现有读口），实际履约取段内已结束的参与、业务时间取最晚终点，订舱与取消随票 28；采购上下文由登记方给出、按业务时间经 PC 核对，法人与「对象/版本」快照取自协议，四个否定格各成一格，缺了答未受理绝不答不适用，不适用只认显式声明自营与实际承运商判为自营；发生条件在 ADR-0209 之前由登记者断言并留痕；身份由（租户、原因、依据、成员集合）派生、首版 V1，同一事实上成员与另一项仍有效的发生项有交集答冲突；`transport-fulfillment.charge-occurrence.formed` 引用式三键、指纹 ID、与发生项同笔；不内联进物理编排、暂不自动形成。越权风险点含断言期、旅程无人核、不自动形成、数量单位由登记方声明｜**部分停用**：其决定三「登记方交一句原话断言「采用协议的发生条件已满足」……缺了答未受理 `CONDITION_NOT_ASSERTED`」与「数量与单位同由登记方声明」两句已由 [ADR-0209](./0209-supplier-agreements-declare-charge-occurrence-conditions-and-registration-judges-by-them.md) 决定四停用，**适用场景**：本记录之后新形成的发生项——停用的理由是条件与数量形已由采用的协议声明，这一前提只对新登记成立，断言期已形成的项照旧按断言读；决定四「查与随后的落册在同一事务里按（租户、原因、事实依据）串行化」一句的键改为（租户、原因、主要业务范围），**适用场景**：全部登记——按范围计的一项要挡住同一范围上别的事实依据。适用场景另见 ADR-0209「对已接受记录的作用」；其余各条不变，本记录正文不改写｜**文末补记**（票 demo-chain-gaps/28）：订舱的依据是已接受的承运答复、取消的依据是尚未开始范围的订舱取消（只认订舱一级，按业务时间与 ADR-0204 决定三同一边界）；订舱与取消、以及有采用连线的实际履约段，采购上下文取委托快照，登记方可省略、给了而不一致答冲突；委托记下指名的协议版本（迁移 TF 0038）；`REASON_NOT_REGISTRABLE` 退场。**收窄**决定二「不适用只有两格」的第二格「那一段当前的实际承运商判断是自营法人」，**适用场景**：只限有采用连线的段——段的采购组织已由登记方声明，成本来源跟着采购走；没有连线的段照旧按两格判｜**部分停用**（2026-10-19）：其决定七后半句——「也暂不自动形成」与「运输履约自消费交接与有效交付信封来自动形成，等 ADR-0209 与票 demo-chain-gaps/25 落地后另定」——与越权风险点 3「不自动形成」已由 [ADR-0218](./0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md) 停用，**适用场景**：只限有采用连线的段——停用的理由带前提「采购上下文已由委托给出、条件已由协议声明」，没有连线的段仍由人工登记；决定四末句「演示约定 `SYN-JRN-<包裹号>`」，**适用场景**：演示租户自 ADR-0218 起改用 `JRN/<包裹号>`。决定七前半句「不在任何物理编排里内联」、决定四其余各句与其余各条不变，本记录正文不改写｜**部分停用**（2026-10-21）：其文末补记「委托记下指名的那一版协议」一条里「发生项按它回问 PC」一句已由 [ADR-0231](./0231-actual-fulfillment-left-unformed-is-caught-up-by-doorbells-and-a-commission-agreement-revision.md) 决定四停用——委托换过版之后，回问的是起用不晚于事实业务时间的那一次换版的后继。**适用场景**：只限换过版、且有换版起用不晚于业务时间的委托；没换过版或业务时间早于各次换版起用的，照旧是提交时记下的那一版；补记其余各句与决定七前半句不变｜**部分停用**（指针 2026-10-21 补）：其决定四「旅程由登记方声明，本上下文没有它的权威来源，不核」一句已由 [ADR-0218](./0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md) 决定四停用——自动形成那一路取参与入场时已开始、开始最晚的那一趟替代 / 退运旅程，没有就按对象的正式包裹身份确定性派生 `JRN/<包裹>`。**适用场景**：只限实际履约发生项自动形成那一路；人工登记照旧由登记方声明（ADR-0218「「旅程由登记方声明」对人工登记不变」），其余各条不变
```

### ADR-0208：BUY 评价由发生项已形成信封发起——发生项与协议取自发生项，费用项目取触发册，主要范围取协议的商业范围

处理：来向两写合并；接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/26，代决出自 `.scratch/demo-chain-gaps/tf-chain-plan.md` 的「BUY 发起（接 08，收 10）」一节。越权风险点见文末，待 SA owner 复核，第 2、8 点同时待 PC owner、第 5、7 点同时待 TF owner。） 决定五「修订版不发起」一句已由 [ADR-0232](./0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) **部分停用**（2026-10-21）：失效之后长出的替代版本照常发起，失效版本照旧不发起、撤下它的成本另有一路（被停原句与适用场景见索引注记），其余各条不变。<!-- 索引注记 -->｜**已接受**（2026-10-19，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 SA owner 裁定；票 demo-chain-gaps/26）。结算在派发里消费 `transport-fulfillment.charge-occurrence.formed`（消费者先登路由），按引用回读发生项；发生项、原因、版本、业务时间与供应商协议取自发生项，部分停用 ADR-0164 决定一末句（限本路径）；费用项目取 BUY 触发册新格，缺行或缺费用项目答未配置入账；主要范围取协议的商业范围（PC 新开只交范围的窄读口），读不动未决、查无是矛盾；修订版不发起。越权风险点待 SA owner 复核，部分同时待 PC、TF owner｜**部分停用**（2026-10-21）：决定三「补登之后，已入账的那一份不回补」与「册不设修订：同原因换一个费用项目或给没登的行补登，都答冲突」两句已由 [ADR-0229](./0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 停用——答`未配置`的那一拍记进发起停办册、补登后经「评价参数已登记」续办；触发册纳入修订形，没登费用项目的那一版可追加一版带费用项目的，旧版留历史。**适用场景**：同原因换一个已登的费用项目仍答冲突；越权风险点 4 由 ADR-0229 作答。其余各条不变｜**替代版本照常发起**（2026-10-21）：发生项有效性修订驱动纠错由 [ADR-0232](./0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) 定（票 demo-chain-gaps/15）。**被停条款**：决定五「修订版指有效性修订产生的那一版（失效或替代）：本路不为它发起」。**适用场景**：只限替代版本——失效之后凭同一件事实重新成立的那一版照常发起，评价形成时接在链头后面重述全额；失效版本照旧不发起，派发另有一路在发生项锁下撤下依被失效那一版形成的成本
```

合并后：

```text
Status: Accepted（2026-10-19，Claude Code 会话代 SA owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/26，代决出自 `.scratch/demo-chain-gaps/tf-chain-plan.md` 的「BUY 发起（接 08，收 10）」一节。部分停用 [ADR-0164](./0164-buy-evaluation-request-trigger-is-occurrence-formed.md) 决定一末句，限本路径。越权风险点见文末，待 SA owner 复核，第 2、8 点同时待 PC owner、第 5、7 点同时待 TF owner。）<!-- 索引注记 -->｜结算在派发里消费 `transport-fulfillment.charge-occurrence.formed`（消费者先登路由），按引用回读发生项；发生项、原因、版本、业务时间与供应商协议取自发生项；费用项目取 BUY 触发册新格，缺行或缺费用项目答未配置入账；主要范围取协议的商业范围（PC 新开只交范围的窄读口），读不动未决、查无是矛盾；修订版不发起｜**部分停用**（2026-10-21）：其决定三「补登之后，已入账的那一份不回补」与「册不设修订：同原因换一个费用项目或给没登的行补登，都答冲突」两句已由 [ADR-0229](./0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 停用——答`未配置`的那一拍记进发起停办册、补登后经「评价参数已登记」续办；触发册纳入修订形，没登费用项目的那一版可追加一版带费用项目的，旧版留历史。**适用场景**：同原因换一个已登的费用项目仍答冲突；越权风险点 4 由 ADR-0229 作答。其余各条不变｜**部分停用**（2026-10-21，替代版本照常发起）：发生项有效性修订驱动纠错由 [ADR-0232](./0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) 定（票 demo-chain-gaps/15）。**被停条款**：其决定五「修订版指有效性修订产生的那一版（失效或替代）：本路不为它发起」。**适用场景**：只限替代版本——失效之后凭同一件事实重新成立的那一版照常发起，评价形成时接在链头后面重述全额；失效版本照旧不发起，派发另有一路在发生项锁下撤下依被失效那一版形成的成本
```

### ADR-0209：供应商协议声明收费发生条件——逐原因计不计费、计费带数量形，发生项登记按采用的那一版判，断言期退场

处理：来向两写合并；接受出处两写合并；补指针

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/29，代决出自 `.scratch/demo-chain-gaps/tf-chain-plan.md` 的「07 运输收费发生项」D07-1 与「需要的 ADR」里 ADR-0209 一条。部分停用 [ADR-0207](./0207-transport-charge-occurrences-are-registered-by-an-operation-decision-and-handed-off-in-the-same-transaction.md) 决定三的断言期判法与决定四串行化那一句的键，适用场景见决定四、六与文末「对已接受记录的作用」。越权风险点见文末，待 PC owner 与 TF owner 复核。）**部分停用**（2026-10-19）：决定五「登记者取自操作者信封的主体」一句已由 [ADR-0218](./0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md) 停用，**适用场景**只限实际履约发生项自动形成那一路（没有操作者信封，登记者记本上下文的系统身份）；运营决定那一路与其余各条不变，本记录正文不改写。<!-- 索引注记 -->｜**已接受**（2026-10-19，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 PC 与 TF owner 裁定；票 demo-chain-gaps/29）。供应商协议正文加可缺的「收费发生条件」一节：四个原因各至多一条，计费带数量形（按段 / 按件 / 按尝试 / 按订舱，按原因配），不计费是一句声明；带这一节的正文以 `PCC-2` 规范化、不带的仍是 `PCC-1` 逐字节不变，本构建两版都认；发生项登记按采用的那一版判——没声明答 `NOT_CONFIGURED`、声明不计费答 `NOT_CHARGED`（不是不适用）、计费按数量形数出数量与单位，依据记 `AGREEMENT_DECLARED`；断言期的断言、数量、单位三键退场（按未知键拒）；按范围计的一项盖住整个范围（`SCOPE_ALREADY_CHARGED`；同一范围连同依据出处认，揽收与派送同串的尝试互不挡），串行化改按主要业务范围；每一项记登记者。部分停用 ADR-0207 决定三的断言期判法与决定四的串行化键。演示干线协议以 v2 重发。越权风险点含文法偏简、取消的配形、v1 与 v2 并存无收尾入口、断言项不迁｜**部分停用**（[ADR-0218](./0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md)）：决定五「登记者取自操作者信封的主体」，**适用场景**：只限实际履约发生项自动形成那一路——那一路没有操作者信封，登记者记本上下文的系统身份；运营决定那一路与其余各条不变
```

合并后：

```text
Status: Accepted（2026-10-19，Claude Code 会话代 PC 与 TF owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/29，代决出自 `.scratch/demo-chain-gaps/tf-chain-plan.md` 的「07 运输收费发生项」D07-1 与「需要的 ADR」里 ADR-0209 一条。部分停用 [ADR-0207](./0207-transport-charge-occurrences-are-registered-by-an-operation-decision-and-handed-off-in-the-same-transaction.md) 决定三的断言期判法与决定四串行化那一句的键，适用场景见决定四、六与文末「对已接受记录的作用」。越权风险点见文末，待 PC owner 与 TF owner 复核。）<!-- 索引注记 -->｜供应商协议正文加可缺的「收费发生条件」一节：四个原因各至多一条，计费带数量形（按段 / 按件 / 按尝试 / 按订舱，按原因配），不计费是一句声明；带这一节的正文以 `PCC-2` 规范化、不带的仍是 `PCC-1` 逐字节不变，本构建两版都认；发生项登记按采用的那一版判——没声明答 `NOT_CONFIGURED`、声明不计费答 `NOT_CHARGED`（不是不适用）、计费按数量形数出数量与单位，依据记 `AGREEMENT_DECLARED`；断言期的断言、数量、单位三键退场（按未知键拒）；按范围计的一项盖住整个范围（`SCOPE_ALREADY_CHARGED`；同一范围连同依据出处认，揽收与派送同串的尝试互不挡），串行化改按主要业务范围；每一项记登记者。演示干线协议以 v2 重发。越权风险点含文法偏简、取消的配形、v1 与 v2 并存无收尾入口、断言项不迁｜**部分停用**（2026-10-19）：其决定五「登记者取自操作者信封的主体」一句已由 [ADR-0218](./0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md) 停用，**适用场景**：只限实际履约发生项自动形成那一路——那一路没有操作者信封，登记者记本上下文的系统身份；运营决定那一路与其余各条不变，本记录正文不改写｜**越权风险点 5 已答**（指针 2026-10-21 补）：越权风险点 5「v1 与 v2 并存在册」由 [ADR-0233](./0233-an-older-commercial-version-is-closed-for-a-named-successor-by-a-cut-or-a-same-start-retirement.md) 回答——同一对象换版时旧版只经显式收尾（按后继生效时点截止，或同起点重发按决定引用退役）离开新的选择；演示租户由种子把干线协议 `v1`、`v2` 依次退役，演示动线指名 `v3`（证据 `S`），决定六「演示租户以 v2 重发干线协议」所说的那一版已经让位；无条款停用，其余各条不变
```

### ADR-0210：身份冲突经运营决定处置——在冲突候选里指定一件或确认都不是，逐条判断链追加处置版；方向依据的身份一格改读每条判断链的当前有效版本

处理：来向两写合并；接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19。用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；该会话代 NO owner 裁定，裁定原文落在票 [no-reception-identity/04](../../.scratch/no-reception-identity/issues/04-identity-conflicts-have-a-disposition-and-release.md)「裁定（2026-10-19，会话 idp-parcel-9b 代决）」一节。本记录照它写成，补上处置的形状，并定下裁定交给本记录定的事：指定的那件已关联到别的作业实物时怎么办，处置命令口归哪份 UC。成文由该会话起的作者子代理完成。部分停用 [ADR-0196](./0196-one-mark-on-two-units-is-an-identity-conflict-judged-by-the-associated-parcel.md)、[ADR-0199](./0199-node-reception-preserves-the-delivery-source-and-rechecks-pending-identities.md) 各几句，逐句见「与已接受决定的关系」。裁决能力边界：读过 NO `CONTEXT.md` 全文、UC-NO-002、GLOSSARY「作业实物」「待识别实物」、CONTEXT-MAP 里 `node-operations` 的拥有项与 `parcel-shipment → node-operations` 两条，ADR-0151 及其复核记录、ADR-0183「复核记录二」、ADR-0186、ADR-0195、ADR-0196 及其复核记录、ADR-0199，票 no-directional-basis/01「偏离标记半边：设计代决」、operator-channel/15 两段补记、no-reception-source/02 完成记录与评审、no-reception-source/03、04、05；代码读过 `domain.JudgeIdentityBasis`、`IntakeIdentities.FindIntakeIdentities`、`Receptions.FindOtherUnitIntakes` 与 `Receptions.CurrentVersion`、`ParcelContainmentView.LoadParcelContainment`、`ReviewCatalogue.ListUnidentifiedItems`、`ReceiveDeliveredUnitHandler.appendIdentityConflicts`、`accessidentity.DecisionKind` 的词表与演示种子的授予文件。没读：PS 采用编排 `AdoptNetworkIntakeHandler` 全文（只读了各函数头注）、VE 侧消费者、并行在途的重核实现。）部分停用：决定一两句与「能力边界」一项由 [ADR-0211](./0211-an-ambiguity-only-identity-conflict-releases-by-recheck-once-the-declaration-is-gone.md) 停用，只限冲突来由为只因歧义信号的身份冲突，随票 no-reception-identity/07 进 main 生效；被停的原句见本记录文末「复核记录（2026-10-19，ADR-0211 部分停用几句）」与 adr/README 该行。<!-- 索引注记 -->｜**已接受**（2026-10-19，用户授权会话 idp-parcel-9b 代 NO owner 裁定，落在票 no-reception-identity/04）。**处置**：操作者渠道「运营决定」能力面加决定种类「身份冲突处置」，谁持有授予是租户取值，不问 PC；不做自动解除，重核与再次收寄都不解除冲突。**两种结果**：在处置候选（各链当前有效版本上的候选与关联）里指定一件，或确认都不是；逐条判断链各追加一版，指名前版，收寄事实与控制照前版、不另起控制起点，原判断与冲突版永久保留。**连带指明**：指定的包裹此刻关联在别的实物上时，同一次处置须把那些实物列为连带实物、各追加「不是它」，不取「先处置那一件」。**读链头**：方向依据身份一格只读没有后继的版本，先到那件第一期之后落缺少·{待识别, 身份冲突}；`FindOtherUnitIntakes` 与 `ParcelContainmentView` 改认「处置截断」、不读链头。**命令口**：重放身份是指名的版本，同内容答已有结果、异内容答已处置并回显，坏载荷 400。**意图**：指定一件且此前没就它交过时交一封，已交出的不撤回。**重核**：「都不是」那一版照常重核，唯一命中被排除的包裹时不追加。另立 UC-NO-004。部分停用 ADR-0196 决定四、决定六与 ADR-0199 决定三、决定五各几句。越权风险点含改读链头、「正式身份由 PS 确认」的读法、处置截断超出裁定字面、「处置」一词同字｜**部分停用**：决定一「**不做自动解除。** 歧义信号消失之后自动解开……本记录不做、不立票。」与同条「**重核与再次收寄都不解除冲突。**」里重核那半句，以及「能力边界」首项里「不做自动解除」，已由 [ADR-0211](./0211-an-ambiguity-only-identity-conflict-releases-by-recheck-once-the-declaration-is-gone.md) 停用，**适用场景**：只限冲突来由为只因歧义信号的身份冲突——声明它的委托被拒绝或撤回后，PS 交声明撤除门铃，重核答唯一命中即追加关联版、冲突随链头解开；再次收寄不解除冲突、多候选与一码两物只经处置照旧，自票 no-reception-identity/07 进 main 起生效；其余各条不变
```

合并后：

```text
Status: Accepted（2026-10-19。用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；该会话代 NO owner 裁定，裁定原文落在票 [no-reception-identity/04](../../.scratch/no-reception-identity/issues/04-identity-conflicts-have-a-disposition-and-release.md)「裁定（2026-10-19，会话 idp-parcel-9b 代决）」一节。本记录照它写成，补上处置的形状，并定下裁定交给本记录定的事：指定的那件已关联到别的作业实物时怎么办，处置命令口归哪份 UC。成文由该会话起的作者子代理完成。部分停用 [ADR-0196](./0196-one-mark-on-two-units-is-an-identity-conflict-judged-by-the-associated-parcel.md)、[ADR-0199](./0199-node-reception-preserves-the-delivery-source-and-rechecks-pending-identities.md) 各几句（ADR-0196 决定四、决定六，ADR-0199 决定三、决定五），逐句见「与已接受决定的关系」。裁决能力边界：读过 NO `CONTEXT.md` 全文、UC-NO-002、GLOSSARY「作业实物」「待识别实物」、CONTEXT-MAP 里 `node-operations` 的拥有项与 `parcel-shipment → node-operations` 两条，ADR-0151 及其复核记录、ADR-0183「复核记录二」、ADR-0186、ADR-0195、ADR-0196 及其复核记录、ADR-0199，票 no-directional-basis/01「偏离标记半边：设计代决」、operator-channel/15 两段补记、no-reception-source/02 完成记录与评审、no-reception-source/03、04、05；代码读过 `domain.JudgeIdentityBasis`、`IntakeIdentities.FindIntakeIdentities`、`Receptions.FindOtherUnitIntakes` 与 `Receptions.CurrentVersion`、`ParcelContainmentView.LoadParcelContainment`、`ReviewCatalogue.ListUnidentifiedItems`、`ReceiveDeliveredUnitHandler.appendIdentityConflicts`、`accessidentity.DecisionKind` 的词表与演示种子的授予文件。没读：PS 采用编排 `AdoptNetworkIntakeHandler` 全文（只读了各函数头注）、VE 侧消费者、并行在途的重核实现。）<!-- 索引注记 -->｜**处置**：操作者渠道「运营决定」能力面加决定种类「身份冲突处置」，谁持有授予是租户取值，不问 PC；不做自动解除，重核与再次收寄都不解除冲突。**两种结果**：在处置候选（各链当前有效版本上的候选与关联）里指定一件，或确认都不是；逐条判断链各追加一版，指名前版，收寄事实与控制照前版、不另起控制起点，原判断与冲突版永久保留。**连带指明**：指定的包裹此刻关联在别的实物上时，同一次处置须把那些实物列为连带实物、各追加「不是它」，不取「先处置那一件」。**读链头**：方向依据身份一格只读没有后继的版本，先到那件第一期之后落缺少·{待识别, 身份冲突}；`FindOtherUnitIntakes` 与 `ParcelContainmentView` 改认「处置截断」、不读链头。**命令口**：重放身份是指名的版本，同内容答已有结果、异内容答已处置并回显，坏载荷 400。**意图**：指定一件且此前没就它交过时交一封，已交出的不撤回。**重核**：「都不是」那一版照常重核，唯一命中被排除的包裹时不追加。另立 UC-NO-004。越权风险点含改读链头、「正式身份由 PS 确认」的读法、处置截断超出裁定字面、「处置」一词同字｜**部分停用**：其决定一「**不做自动解除。** 歧义信号消失之后自动解开……本记录不做、不立票。」与同条「**重核与再次收寄都不解除冲突。**」里重核那半句，以及「能力边界」首项里「不做自动解除」，已由 [ADR-0211](./0211-an-ambiguity-only-identity-conflict-releases-by-recheck-once-the-declaration-is-gone.md) 停用，**适用场景**：只限冲突来由为只因歧义信号的身份冲突——声明它的委托被拒绝或撤回后，PS 交声明撤除门铃，重核答唯一命中即追加关联版、冲突随链头解开；再次收寄不解除冲突、多候选与一码两物只经处置照旧，自票 no-reception-identity/07 进 main 起生效；裁定经过见本记录文末「复核记录（2026-10-19，ADR-0211 部分停用几句；决定原文不动）」；其余各条不变
```

### ADR-0211：只因歧义信号落下的身份冲突，在那份声明随委托被拒绝或撤回之后经重核自动解开——冲突记来由，PS 交声明撤除门铃

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19。代决：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」，2026-10-19 就本票答「需要」；该会话代 NO owner 与 PS owner 裁定，裁定原文落在票 [no-reception-identity/07](../../.scratch/no-reception-identity/issues/07-an-ambiguity-only-conflict-releases-when-the-declaration-is-gone.md)「裁定（2026-10-19，会话 idp-parcel-9b 代决）」一节。本记录照它写成，补上实施时定下的形状；成文由该会话起的作者子代理完成。部分停用 [ADR-0210](./0210-identity-conflicts-are-disposed-by-an-operation-decision-and-the-identity-basis-reads-chain-heads.md)、[ADR-0199](./0199-node-reception-preserves-the-delivery-source-and-rechecks-pending-identities.md)、[ADR-0195](./0195-a-submitted-declaration-of-the-same-number-turns-a-unique-hit-into-an-identity-conflict.md) 各几句，逐句见「与已接受决定的关系」；ADR-0195 那一句超出裁定字面，理由见越权风险点「停用 ADR-0195 一句」。裁决能力边界：读过 NO `CONTEXT.md`「实物身份与事实完整性」与「待识别实物与身份关联」，PS `CONTEXT.md`「歧义信号」、外部标识的各条 Rules 与`委托`生命周期，CONTEXT-MAP `parcel-shipment → node-operations` 各句，GLOSSARY「待识别实物」「身份冲突处置」，UC-NO-002 的「控制、身份与后续作业」与验收表、UC-NO-004 的范围，ADR-0184 越权风险点与复核记录、ADR-0195、ADR-0196、ADR-0199、ADR-0210 及各自复核记录；代码读过 NO 重核编排 `RecheckPendingIdentityHandler`、共用身份判断 `identityJudge`、收寄编排的明确接收支与 `appendIdentityConflicts`、处置编排追加处置版那一段、收寄库 PG 适配器与按号找待重核那一问、两扇重核消费门与派发装配，PS 委托仓储 `ShipmentRequests.Save`、关联已形成门铃 `ExternalIdentifierDoorbell`、委托的拒绝（`Decide`、`RejectByAuthority`、授权处置的拒绝）与撤回（`WithdrawByCustomer`）各条转移及它们落库的编排。**没读** VE 侧消费者与管理台复核页的重核按钮实现以外的前端代码。）<!-- 索引注记 -->｜**已接受**（2026-10-19，用户授权会话 idp-parcel-9b 代 NO、PS owner 裁定，落在票 no-reception-identity/07）。**来由**：身份冲突落下时记封闭三值——只因歧义信号、多候选（压过信号）、一码两物（后到那件与先到那件的冲突版都记）；处置版不记冲突也不带来由；既有冲突行来由缺席、不回填，只经处置解开；NO 迁移 0012 加列并重建按号找待重核的部分索引。**重核**：重核对象放宽到当前有效版本记冲突、来由是只因歧义信号的待识别；走同一份身份判断，唯一且无信号追加关联版（业务时间原收寄时刻、不另起控制起点、意图一封），信号仍在不追加、答新的一格 `STILL_AMBIGUOUS`，多候选或一码两物追加冲突版、来由随之更新，零候选判断不变。**触发**：委托由`已提交`转为`已拒绝`或`已撤回`时，委托仓储保存那一笔按声明的每个号交一封只带（租户，号码）的门铃，ID 由（租户，委托，号码）认领、分区与关联已形成门铃同队；NO 同一个重核消费者多一扇门按号重核；接受不另起触发；人工重核口同样放开。部分停用 ADR-0210 决定一两句与能力边界一项、ADR-0199 决定三两句、ADR-0195 决定四一句。越权风险点含撤单即自动解开、PS 多交一种信封、来由次序与不回填、信号仍在与零候选不追加
```

合并后：

```text
Status: Accepted（2026-10-19。代决：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」，2026-10-19 就本票答「需要」；该会话代 NO owner 与 PS owner 裁定，裁定原文落在票 [no-reception-identity/07](../../.scratch/no-reception-identity/issues/07-an-ambiguity-only-conflict-releases-when-the-declaration-is-gone.md)「裁定（2026-10-19，会话 idp-parcel-9b 代决）」一节。本记录照它写成，补上实施时定下的形状；成文由该会话起的作者子代理完成。部分停用 [ADR-0210](./0210-identity-conflicts-are-disposed-by-an-operation-decision-and-the-identity-basis-reads-chain-heads.md)、[ADR-0199](./0199-node-reception-preserves-the-delivery-source-and-rechecks-pending-identities.md)、[ADR-0195](./0195-a-submitted-declaration-of-the-same-number-turns-a-unique-hit-into-an-identity-conflict.md) 各几句（ADR-0210 决定一两句与「能力边界」一项、ADR-0199 决定三两句、ADR-0195 决定四一句），逐句见「与已接受决定的关系」；ADR-0195 那一句超出裁定字面，理由见越权风险点「停用 ADR-0195 一句」。裁决能力边界：读过 NO `CONTEXT.md`「实物身份与事实完整性」与「待识别实物与身份关联」，PS `CONTEXT.md`「歧义信号」、外部标识的各条 Rules 与`委托`生命周期，CONTEXT-MAP `parcel-shipment → node-operations` 各句，GLOSSARY「待识别实物」「身份冲突处置」，UC-NO-002 的「控制、身份与后续作业」与验收表、UC-NO-004 的范围，ADR-0184 越权风险点与复核记录、ADR-0195、ADR-0196、ADR-0199、ADR-0210 及各自复核记录；代码读过 NO 重核编排 `RecheckPendingIdentityHandler`、共用身份判断 `identityJudge`、收寄编排的明确接收支与 `appendIdentityConflicts`、处置编排追加处置版那一段、收寄库 PG 适配器与按号找待重核那一问、两扇重核消费门与派发装配，PS 委托仓储 `ShipmentRequests.Save`、关联已形成门铃 `ExternalIdentifierDoorbell`、委托的拒绝（`Decide`、`RejectByAuthority`、授权处置的拒绝）与撤回（`WithdrawByCustomer`）各条转移及它们落库的编排。**没读** VE 侧消费者与管理台复核页的重核按钮实现以外的前端代码。）<!-- 索引注记 -->｜**来由**：身份冲突落下时记封闭三值——只因歧义信号、多候选（压过信号）、一码两物（后到那件与先到那件的冲突版都记）；处置版不记冲突也不带来由；既有冲突行来由缺席、不回填，只经处置解开；NO 迁移 0012 加列并重建按号找待重核的部分索引。**重核**：重核对象放宽到当前有效版本记冲突、来由是只因歧义信号的待识别；走同一份身份判断，唯一且无信号追加关联版（业务时间原收寄时刻、不另起控制起点、意图一封），信号仍在不追加、答新的一格 `STILL_AMBIGUOUS`，多候选或一码两物追加冲突版、来由随之更新，零候选判断不变。**触发**：委托由`已提交`转为`已拒绝`或`已撤回`时，委托仓储保存那一笔按声明的每个号交一封只带（租户，号码）的门铃，ID 由（租户，委托，号码）认领、分区与关联已形成门铃同队；NO 同一个重核消费者多一扇门按号重核；接受不另起触发；人工重核口同样放开。越权风险点含撤单即自动解开、PS 多交一种信封、来由次序与不回填、信号仍在与零候选不追加
```

### ADR-0212：客户编号空间里的唯一命中要有可验证交付范围——送站预报与送站交接单归节点作业，范围只消歧、不收窄，范围未证答待识别、不记冲突

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19。代决：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」，2026-10-19 答「需要」；该会话代 PS owner 与 NO owner 裁定方向五条，原文落在票 [ps-external-mark-relations/07](../../.scratch/ps-external-mark-relations/issues/07-verifiable-delivery-scope-for-customer-numbers.md)「方向裁定」一节。方向裁定把细节交给本记录；本记录照它定下，成文由该会话起的作者子代理完成，同样属代决。部分停用 [ADR-0184](./0184-external-identifier-relations-are-an-append-only-register-formed-with-their-source-facts.md)、[ADR-0195](./0195-a-submitted-declaration-of-the-same-number-turns-a-unique-hit-into-an-identity-conflict.md)、[ADR-0199](./0199-node-reception-preserves-the-delivery-source-and-rechecks-pending-identities.md) 里与之相抵的句子，被停原句与适用场景见「与已接受决定的关系」，其余各条不变。裁决能力边界：读过 PS `CONTEXT.md`「外部标识」「外部标识关联」「歧义信号」的定义与外部标识各条 Rules，NO `CONTEXT.md` 全文，PC `CONTEXT.md`「参与方关系」，CONTEXT-MAP `parcel-shipment → node-operations` 各句与 NO、PC 两节，GLOSSARY「节点作业」一节，UC-NO-002 与 UC-NO-004 全文；ADR-0184 全文及复核记录，ADR-0195 全文及复核记录，ADR-0199 决定一、决定三及复核记录，ADR-0139 的 Decision 一至八与 Consequences，ADR-0149 起句，ADR-0151 决定一，ADR-0210 与 ADR-0196 在 adr/README 的摘要行，rebase 时读过 ADR-0211 的 Decision、「与已接受决定的关系」与「能力边界」；票 ps-external-mark-relations/05、06，no-reception-identity/06、07，first-party-api 的 spec、票 00（含完成记录）与票 01；参数登记册的登记规则与 `PAR-NET-13`、`PAR-INT-08`、`PAR-INT-09`、`PAR-INT-11`、`PAR-INT-13` 行。代码读过 NO 在线收寄口的 `receptionDocument` 与 `receptionCommand`、`DeliveringPartyReference`、来源文档 `DeliverySource` 与 `CanonicalizeDeliverySource`、端口 `ParcelIdentityAnswer` 与 `Unique` 的签名、PS 端口 `ExternalIdentifierParcels` 的签名，PC 的 `PartyRelationshipSpec` 与 `PartyRole`。**没读** ADR-0196、ADR-0210 正文全文，ADR-0140 至 0142，PS 身份核对读口的 SQL，重核编排与 `identityJudge` 全文，受控批量口的收寄模板，首方 Intake 的在途代码。）<!-- 索引注记 -->｜**已接受**（2026-10-19，用户授权会话 idp-parcel-9b 代 PS、NO owner 裁定方向，落在票 ps-external-mark-relations/07；细节由该会话起的作者子代理代决成文）。**归属**：可验证交付范围、送站预报与送站交接单归 NO；授权交付关系归 PC，是参与方关系的一种。**预报**：客户账户经首方客户渠道族提交，提交即自证；NO 签发标识；范围落到所列号码；有效时窗是租户取值，空册答未配置。**交接单**：节点当场开立，可归入待识别实物；绑定逐客户账户确认，客户渠道（只对已关闭的）或运营决定「送站交接确认」（依授权交付关系），全部确认才成立范围。**核对**：PS 读口带上范围的客户账户，候选照旧全部取，逐候选答范围相容，信号带范围时收到范围内；没有范围时只经客户包裹件号的唯一命中答待识别、不记冲突（范围未证）；有范围时范围相容的候选恰一个且没有范围内信号即唯一命中，可消歧多候选。**交付范围核对态**是信息格：已证、未证附原因、未配置。重核重算范围，多一处「交付范围成立」门铃；来源文档多一格依据引用，不进指纹。部分停用 ADR-0184 决定五、决定六，ADR-0195 决定一、决定三，ADR-0199 决定一、决定三中的句子。越权风险点含预报归 NO、号码级、翻转与无预报散客送站、范围相容的读法、交接单全有或全无，待 PS、NO、PC owner 复核
```

合并后：

```text
Status: Accepted（2026-10-19。代决：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」，2026-10-19 答「需要」；该会话代 PS owner 与 NO owner 裁定方向五条，原文落在票 [ps-external-mark-relations/07](../../.scratch/ps-external-mark-relations/issues/07-verifiable-delivery-scope-for-customer-numbers.md)「方向裁定」一节。方向裁定把细节交给本记录；本记录照它定下，成文由该会话起的作者子代理完成，同样属代决。部分停用 [ADR-0184](./0184-external-identifier-relations-are-an-append-only-register-formed-with-their-source-facts.md)、[ADR-0195](./0195-a-submitted-declaration-of-the-same-number-turns-a-unique-hit-into-an-identity-conflict.md)、[ADR-0199](./0199-node-reception-preserves-the-delivery-source-and-rechecks-pending-identities.md) 里与之相抵的句子（ADR-0184 决定五、决定六，ADR-0195 决定一、决定三，ADR-0199 决定一、决定三），被停原句与适用场景见「与已接受决定的关系」，其余各条不变。裁决能力边界：读过 PS `CONTEXT.md`「外部标识」「外部标识关联」「歧义信号」的定义与外部标识各条 Rules，NO `CONTEXT.md` 全文，PC `CONTEXT.md`「参与方关系」，CONTEXT-MAP `parcel-shipment → node-operations` 各句与 NO、PC 两节，GLOSSARY「节点作业」一节，UC-NO-002 与 UC-NO-004 全文；ADR-0184 全文及复核记录，ADR-0195 全文及复核记录，ADR-0199 决定一、决定三及复核记录，ADR-0139 的 Decision 一至八与 Consequences，ADR-0149 起句，ADR-0151 决定一，ADR-0210 与 ADR-0196 在 adr/README 的摘要行，rebase 时读过 ADR-0211 的 Decision、「与已接受决定的关系」与「能力边界」；票 ps-external-mark-relations/05、06，no-reception-identity/06、07，first-party-api 的 spec、票 00（含完成记录）与票 01；参数登记册的登记规则与 `PAR-NET-13`、`PAR-INT-08`、`PAR-INT-09`、`PAR-INT-11`、`PAR-INT-13` 行。代码读过 NO 在线收寄口的 `receptionDocument` 与 `receptionCommand`、`DeliveringPartyReference`、来源文档 `DeliverySource` 与 `CanonicalizeDeliverySource`、端口 `ParcelIdentityAnswer` 与 `Unique` 的签名、PS 端口 `ExternalIdentifierParcels` 的签名，PC 的 `PartyRelationshipSpec` 与 `PartyRole`。**没读** ADR-0196、ADR-0210 正文全文，ADR-0140 至 0142，PS 身份核对读口的 SQL，重核编排与 `identityJudge` 全文，受控批量口的收寄模板，首方 Intake 的在途代码。）<!-- 索引注记 -->｜**归属**：可验证交付范围、送站预报与送站交接单归 NO；授权交付关系归 PC，是参与方关系的一种。**预报**：客户账户经首方客户渠道族提交，提交即自证；NO 签发标识；范围落到所列号码；有效时窗是租户取值，空册答未配置。**交接单**：节点当场开立，可归入待识别实物；绑定逐客户账户确认，客户渠道（只对已关闭的）或运营决定「送站交接确认」（依授权交付关系），全部确认才成立范围。**核对**：PS 读口带上范围的客户账户，候选照旧全部取，逐候选答范围相容，信号带范围时收到范围内；没有范围时只经客户包裹件号的唯一命中答待识别、不记冲突（范围未证）；有范围时范围相容的候选恰一个且没有范围内信号即唯一命中，可消歧多候选。**交付范围核对态**是信息格：已证、未证附原因、未配置。重核重算范围，多一处「交付范围成立」门铃；来源文档多一格依据引用，不进指纹。越权风险点含预报归 NO、号码级、翻转与无预报散客送站、范围相容的读法、交接单全有或全无，待 PS、NO、PC owner 复核
```

### ADR-0213：权威运输交接声明交出、接收、去向节点——节点控制转移、路由复核与交出 / 交付门禁的位置都取这份声明

处理：来向两写合并；接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 [demo-chain-gaps/59](../../.scratch/demo-chain-gaps/issues/59-handover-declares-releasing-receiving-and-onward-nodes.md) 起草并落决定一至三，决定四由票 [65](../../.scratch/demo-chain-gaps/issues/65-handover-records-an-outbound-release-gate-reading.md) 落，决定五由票 [66](../../.scratch/demo-chain-gaps/issues/66-effective-delivery-records-a-final-delivery-gate-reading.md) 落；统一代决见 `.scratch/demo-chain-gaps/handover-dispatch-plan.md`「代决」C1、C13、C14。回答 [ADR-0206](./0206-customs-action-admission-judgement-folds-three-sources-and-judges-cross-zone-by-node-customs-zones.md) Consequences「挂法等 peer」一格与其越权风险点 9。部分停用 [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md) 决定里的一句，适用场景只限 TF 交接首登上的节点补报，见 Consequences。越权风险点见文末，待 TF owner 复核，NO、NR、CC owner 会签。） 决定四落地时的补充裁定见文末「补记：票 65 落决定四时的补充裁定」（2026-10-19）。决定五落地时的补充裁定见文末「补记：票 66 落决定五时的补充裁定」（2026-10-19）。 文末票 65、66 两则补记「答复与查阅」里「那件对象的那一格」由 [ADR-0237](./0237-gate-readings-show-the-executor-only-the-objects-that-did-not-pass.md) 收窄（2026-10-21）：那件对象得出放行时答复不列它，与出发读数同一处判法；由此**部分停用**两则补记各一句（被停原句见索引注记），查阅与落册不变，其余各条不变。 文末票 66 补记越权风险点 20「读数挡不住交付」由 [ADR-0238](./0238-a-courier-previews-the-delivery-gate-before-dispatching-and-reads-what-the-delivery-will-record.md) 回答（2026-10-21）：作业事实渠道开派送出发前的交付门禁问询，交回与随后同参有效交付同一套读数，运输履约不落记录；交付照决定五在它自己那一刻重问并落册，无条款停用，本记录各条不变。<!-- 索引注记 -->｜**已接受**（2026-10-19，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 TF owner 裁定，待 NO、NR、CC owner 会签；票 demo-chain-gaps/59 落决定一至三，票 65、66 落决定四、五）。**三格声明**：交接登记加交出节点、接收节点、去向节点三个可选声明，存 NR 节点代码、不透明，不核目录与设备作业范围，不从交出方、接收方或范围解析；裁决不限制它们；缺席是一种答案，不由本上下文补。**内容**：至少声明一格时首登摘要写 `TFC-2`（没声明的写 null），一格都不声明仍写 `TFC-1`、字节不变，旧行照原内容重放答已有版本；同键换节点或撤节点答冲突，旧形行上补报节点也答冲突；更正承继前版，更正载荷带节点键答 400；答复与交接册回显，交接信封不变。**门禁挂法**：交出门禁对`已交接`逐对象按（交出节点，去向节点）问交出意图，没声明交出节点记未决不问；交付门禁的 at 取进入派送段那次交接的交出节点，取不到记未决。NO 控制转移与 NR 复核怎样取这三格由 ADR-0214、ADR-0215 定。回答 ADR-0206 Consequences「挂法等 peer」与越权风险点 9；部分停用 ADR-0014 决定一句，限 TF 交接首登上的节点补报。越权风险点含声明错了没人核、节点与参与方两套指称、存量行补不上节点、没有改节点的面，待 TF owner 复核｜**部分停用**（2026-10-21）：[ADR-0237](./0237-gate-readings-show-the-executor-only-the-objects-that-did-not-pass.md) 让作业事实渠道上回给设备的门禁读数逐对象那一格只列没得出放行的对象（阻断、读不动、未决），交出与交付读数与出发读数同一处判法。**被停条款**：票 65 补记「答复与查阅」里「交接答复带读数的整格、at / onward、停点与那件对象的那一格」与票 66 补记「答复与查阅」里「交付答复带读数的整格、at、停点与那件对象的那一格」——那件对象得出放行（放行、不受门禁）时不列。**适用场景**：只限回给登记方的答复；查阅口、落册与其余各条不变｜**派送出发前读口已答**（2026-10-21）：票 66 补记越权风险点 20「读数挡不住交付」由 [ADR-0238](./0238-a-courier-previews-the-delivery-gate-before-dispatching-and-reads-what-the-delivery-will-record.md) 回答——作业事实渠道开 `POST /transport-fulfillment/delivery-gate-previews`，按对象与拟交付时点问，交回与随后同参有效交付同一套读数（同一条读数路径，at 取法、停点只在决定五与票 66 补记），运输履约不落记录，关务照记判断，授予同有效交付口；交付照旧在它自己那一刻重问并落册；无条款停用，其余各条不变
```

合并后：

```text
Status: Accepted（2026-10-19，Claude Code 会话代 TF owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 [demo-chain-gaps/59](../../.scratch/demo-chain-gaps/issues/59-handover-declares-releasing-receiving-and-onward-nodes.md) 起草并落决定一至三，决定四由票 [65](../../.scratch/demo-chain-gaps/issues/65-handover-records-an-outbound-release-gate-reading.md) 落，决定五由票 [66](../../.scratch/demo-chain-gaps/issues/66-effective-delivery-records-a-final-delivery-gate-reading.md) 落；统一代决见 `.scratch/demo-chain-gaps/handover-dispatch-plan.md`「代决」C1、C13、C14。回答 [ADR-0206](./0206-customs-action-admission-judgement-folds-three-sources-and-judges-cross-zone-by-node-customs-zones.md) Consequences「挂法等 peer」一格与其越权风险点 9。部分停用 [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md) 决定里的一句，适用场景只限 TF 交接首登上的节点补报，见 Consequences。越权风险点见文末，待 TF owner 复核，NO、NR、CC owner 会签。） 决定四落地时的补充裁定见文末「补记：票 65 落决定四时的补充裁定」（2026-10-19）。决定五落地时的补充裁定见文末「补记：票 66 落决定五时的补充裁定」（2026-10-19）。<!-- 索引注记 -->｜**三格声明**：交接登记加交出节点、接收节点、去向节点三个可选声明，存 NR 节点代码、不透明，不核目录与设备作业范围，不从交出方、接收方或范围解析；裁决不限制它们；缺席是一种答案，不由本上下文补。**内容**：至少声明一格时首登摘要写 `TFC-2`（没声明的写 null），一格都不声明仍写 `TFC-1`、字节不变，旧行照原内容重放答已有版本；同键换节点或撤节点答冲突，旧形行上补报节点也答冲突；更正承继前版，更正载荷带节点键答 400；答复与交接册回显，交接信封不变。**门禁挂法**：交出门禁对`已交接`逐对象按（交出节点，去向节点）问交出意图，没声明交出节点记未决不问；交付门禁的 at 取进入派送段那次交接的交出节点，取不到记未决。NO 控制转移与 NR 复核怎样取这三格由 ADR-0214、ADR-0215 定。越权风险点含声明错了没人核、节点与参与方两套指称、存量行补不上节点、没有改节点的面｜**部分停用**（2026-10-21）：[ADR-0237](./0237-gate-readings-show-the-executor-only-the-objects-that-did-not-pass.md) 让作业事实渠道上回给设备的门禁读数逐对象那一格只列没得出放行的对象（阻断、读不动、未决），交出与交付读数与出发读数同一处判法。**被停条款**：票 65 补记「答复与查阅」里「交接答复带读数的整格、at / onward、停点与那件对象的那一格」与票 66 补记「答复与查阅」里「交付答复带读数的整格、at、停点与那件对象的那一格」——那件对象得出放行（放行、不受门禁）时不列。**适用场景**：只限回给登记方的答复；查阅口、落册与其余各条不变｜**派送出发前读口已答**（2026-10-21）：票 66 补记越权风险点 20「读数挡不住交付」由 [ADR-0238](./0238-a-courier-previews-the-delivery-gate-before-dispatching-and-reads-what-the-delivery-will-record.md) 回答——作业事实渠道开 `POST /transport-fulfillment/delivery-gate-previews`，按对象与拟交付时点问，交回与随后同参有效交付同一套读数（同一条读数路径，at 取法、停点只在决定五与票 66 补记），运输履约不落记录，关务照记判断，授予同有效交付口；交付照旧在它自己那一刻重问并落册；无条款停用，其余各条不变
```

### ADR-0214：节点控制按权威运输交接转出与转入——交接对象按正式包裹身份经节点自己的当前关联对上作业实物，转移记在只追加的节点控制转移册上、读控制时折叠，`已交接`转出此刻唯一在身的控制，撤回控制的更正入账待重算（UC-NO-005）

处理：来向两写合并；接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/61，统一代决见 `.scratch/demo-chain-gaps/handover-dispatch-plan.md`「代决」C2–C7 与「需要的 ADR」ADR-0214。决定七（转入）由票 demo-chain-gaps/62 实现。越权风险点见文末，待 NO owner 复核，TF owner 会签。） 决定七与决定四末条落地时的补充裁定见文末「补记：票 62 落决定七时的补充裁定」（2026-10-19）。 **部分停用**（2026-10-19）：决定五「同链认法：同一（对象，范围）的各版。」一句已由 [ADR-0220](./0220-node-control-reads-the-handover-correction-chain-by-the-transport-fulfillment-back-pointer.md) 停用——更正链只按运输履约的更正回指认，正文与文末补记凡说「同链」的各格都按它读；其余各条不变。<!-- 索引注记 -->｜**已接受**（2026-10-19，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 NO owner 裁定、TF owner 会签；票 demo-chain-gaps/61，决定七转入由票 demo-chain-gaps/62 实现）。**对象**：先认本租户集运单元（同串按单元，入账待单元控制规则），否则按正式包裹身份读各条判断链当前有效版本上的关联，恰一件推进，零件、多件入账。**转移册**：只追加，键是（租户，交接键，方向），收寄行不改写，读控制时按（租户，作业实物，控制依据）折进转出；一份控制至多转出一次；与收寄共用实物锁。**转出**：`已交接`转出此刻唯一在身的控制（收寄或转入成立），时刻取交接业务时间；无、不止一份在身控制或交接早于控制成立入账；拒收待确认零写入。**更正**：后继仍已交接零写入，撤回控制入账「控制来源链待重算」、不回退，被接续的前版晚到零写入。**处置**：读不动或读不回登本路未决哨兵重投，其余入账，坏键与越出封闭集响亮失败。不停用别的记录的条款。越权风险点含集运单元交接暂不推进、对象只读链头、不止一份在身控制不推进、同链按（对象，范围）认、待重算没有处置面、收寄晚于交接到达时入账格永久留着，待 NO owner 复核｜**部分停用**（2026-10-19）：决定五「同链认法：同一（对象，范围）的各版。」一句已由 [ADR-0220](./0220-node-control-reads-the-handover-correction-chain-by-the-transport-fulfillment-back-pointer.md) 停用——更正链只按运输履约的更正回指认，同一（对象，范围）下另起的首登是另一次交接。**适用场景**：节点控制转出与转入两半凡认「同链」之处（同链前版已推进、撤回控制的更正、一次交接不转出它自己立起的控制）；转移册的主键与「同一版同一方向只处理一次」不受影响。其余各条不变
```

合并后：

```text
Status: Accepted（2026-10-19，Claude Code 会话代 NO owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/61，统一代决见 `.scratch/demo-chain-gaps/handover-dispatch-plan.md`「代决」C2–C7 与「需要的 ADR」ADR-0214。决定七（转入）由票 demo-chain-gaps/62 实现。越权风险点见文末，待 NO owner 复核，TF owner 会签。） 决定七与决定四末条落地时的补充裁定见文末「补记：票 62 落决定七时的补充裁定」（2026-10-19）。<!-- 索引注记 -->｜**对象**：先认本租户集运单元（同串按单元，入账待单元控制规则），否则按正式包裹身份读各条判断链当前有效版本上的关联，恰一件推进，零件、多件入账。**转移册**：只追加，键是（租户，交接键，方向），收寄行不改写，读控制时按（租户，作业实物，控制依据）折进转出；一份控制至多转出一次；与收寄共用实物锁。**转出**：`已交接`转出此刻唯一在身的控制（收寄或转入成立），时刻取交接业务时间；无、不止一份在身控制或交接早于控制成立入账；拒收待确认零写入。**更正**：后继仍已交接零写入，撤回控制入账「控制来源链待重算」、不回退，被接续的前版晚到零写入。**处置**：读不动或读不回登本路未决哨兵重投，其余入账，坏键与越出封闭集响亮失败。不停用别的记录的条款。越权风险点含集运单元交接暂不推进、对象只读链头、不止一份在身控制不推进、同链按（对象，范围）认、待重算没有处置面、收寄晚于交接到达时入账格永久留着｜**部分停用**（2026-10-19）：其决定五「同链认法：同一（对象，范围）的各版。」一句已由 [ADR-0220](./0220-node-control-reads-the-handover-correction-chain-by-the-transport-fulfillment-back-pointer.md) 停用——更正链只按运输履约的更正回指认，同一（对象，范围）下另起的首登是另一次交接，正文与文末补记凡说「同链」的各格都按它读。**适用场景**：节点控制转出与转入两半凡认「同链」之处（同链前版已推进、撤回控制的更正、一次交接不转出它自己立起的控制）；转移册的主键与「同一版在同一方向上只处理一次」不受影响。其余各条不变
```

### ADR-0215：交接进入节点时路由复核——只对`已交接`且声明了接收节点的交接复核、位置取接收节点；受理后不由小包托运信封触发的复核，经消费侧适配器读受理基线那一版提交的地理投影

处理：来向两写合并；接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 [demo-chain-gaps/63](../../.scratch/demo-chain-gaps/issues/63-network-routing-reassesses-on-a-handover-into-a-node.md)，统一代决见 `.scratch/demo-chain-gaps/handover-dispatch-plan.md`「代决」C8、C9 与「需要的 ADR」ADR-0215。回答 [ADR-0213](./0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md) 决定六「NR 交接触发的路由复核怎样取实际位置」。部分停用 [ADR-0075](./0075-customer-address-is-carried-with-the-routing-request.md) 决定一，适用场景只限受理之后、不由小包托运信封触发的复核，见 Consequences。越权风险点见文末，待 NR owner 复核，PS owner 会签。）越权风险点 9「没有路由历史的已接受包裹会重投到预算耗尽」、10「本路未决会推迟同一对象后续交接的追踪投影与派送发起」与 11「两种触发分属不同分区，复核不按业务时间排序」已由 [ADR-0235](./0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md) 回答（2026-10-21）：没有路由历史、证据未配置、冻结形态未配置三格入账未判，原因落 NR 的未判触发登记，不再重投；业务时间早于路由历史里最近一次已判控制触发的触发入账为「触发早于当前位置」，不判。由此**部分停用**决定四「重投」一条的未决范围，以及决定三「更正之后仍是`已交接`」一句在更正一次早于当前位置的交接时的适用（被停原句见索引注记），其余各条不变。<!-- 索引注记 -->｜**已接受**（2026-10-19，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 NR owner 裁定，待 PS owner 会签；票 demo-chain-gaps/63）。**触发**：派发进程把权威交接登记信封另投 NR 一路；按键读回那一版交接，`已交接`、声明了接收节点、对象是当前已接受包裹三者同时成立才复核——控制依据权威运输交接，位置取接收节点，来源版本与业务时间取交接；拒收、待确认、没声明接收节点、对象不是当前已接受包裹（含集运单元号）入账不复核。**投影**：经消费侧适配器按（租户，包裹）反查当前已接受委托、读委托聚合，取受理基线那一版提交的地理投影，与两封 PS 信封所携同一份；基线引用与初始路由、收寄复核同源。**幂等**：一版交接一个关联（四维长度前缀），同版再投答已有结果，更正仍`已交接`另成一次复核，更正成拒收或待确认入账、不回退已有复核。**停与走**：交接读不回、PS 读不动、复核未决登本路未决哨兵重投；交接与键不符、委托自相矛盾、反查歧义响亮失败。回答 ADR-0213 决定六；部分停用 ADR-0075 决定一，限受理后、不由 PS 信封触发的复核。越权风险点含 NR 首次从 PS 委托聚合取地理维、外包段内部异常不触发、集运单元不逐成员、接收节点错了按错误位置复核、受理后地址修订不进复核、复核册看不出触发位置、本路未决占住交接分区头而推迟同一对象后续交接的追踪投影与派送发起、两种触发分属不同分区而复核不按业务时间排序，待 NR owner 复核｜**部分停用**：其决定四「重投」一条里「复核停在未决（复核登记册或路由历史读不回、证据未配置、适用性看不清、取号依赖故障）」一句的未决范围已由 [ADR-0235](./0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md) 停用——证据未配置，以及随之同归未决的没有路由历史、冻结形态未配置，改为入账未判、原因落未判触发登记。**适用场景**：NR 路由复核的收寄采用与交接两路；交接读不回、小包托运读不动与其余未决照旧重投，其余各条不变｜**部分停用**：其决定三「更正之后仍是`已交接`：新版本承继接收节点，另成一次复核，原复核不被覆盖。」一句已由 [ADR-0235](./0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md) 停用。**适用场景**：更正的那次交接业务时间（承继原交接的判断时刻）早于路由历史里最近一次已判控制触发，这时更正入账为「触发早于当前位置」、不判；更正当前位置那一次交接照判、另成一次复核，更正成拒收或待确认照旧入账不复核，其余各条不变
```

合并后：

```text
Status: Accepted（2026-10-19，Claude Code 会话代 NR owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 [demo-chain-gaps/63](../../.scratch/demo-chain-gaps/issues/63-network-routing-reassesses-on-a-handover-into-a-node.md)，统一代决见 `.scratch/demo-chain-gaps/handover-dispatch-plan.md`「代决」C8、C9 与「需要的 ADR」ADR-0215。回答 [ADR-0213](./0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md) 决定六「NR 交接触发的路由复核怎样取实际位置」。部分停用 [ADR-0075](./0075-customer-address-is-carried-with-the-routing-request.md) 决定一，适用场景只限受理之后、不由小包托运信封触发的复核，见 Consequences。越权风险点见文末，待 NR owner 复核，PS owner 会签。）<!-- 索引注记 -->｜**触发**：派发进程把权威交接登记信封另投 NR 一路；按键读回那一版交接，`已交接`、声明了接收节点、对象是当前已接受包裹三者同时成立才复核——控制依据权威运输交接，位置取接收节点，来源版本与业务时间取交接；拒收、待确认、没声明接收节点、对象不是当前已接受包裹（含集运单元号）入账不复核。**投影**：经消费侧适配器按（租户，包裹）反查当前已接受委托、读委托聚合，取受理基线那一版提交的地理投影，与两封 PS 信封所携同一份；基线引用与初始路由、收寄复核同源。**幂等**：一版交接一个关联（四维长度前缀），同版再投答已有结果，更正仍`已交接`另成一次复核，更正成拒收或待确认入账、不回退已有复核。**停与走**：交接读不回、PS 读不动、复核未决登本路未决哨兵重投；交接与键不符、委托自相矛盾、反查歧义响亮失败。越权风险点含 NR 首次从 PS 委托聚合取地理维、外包段内部异常不触发、集运单元不逐成员、接收节点错了按错误位置复核、受理后地址修订不进复核、复核册看不出触发位置、本路未决占住交接分区头而推迟同一对象后续交接的追踪投影与派送发起、两种触发分属不同分区而复核不按业务时间排序｜**越权风险点 9、10、11 已答**（2026-10-21）：越权风险点 9「没有路由历史的已接受包裹会重投到预算耗尽」、10「本路未决会推迟同一对象后续交接的追踪投影与派送发起」与 11「两种触发分属不同分区，复核不按业务时间排序」由 [ADR-0235](./0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md) 回答——没有路由历史、证据未配置、冻结形态未配置三格入账未判，原因落 NR 的未判触发登记，不再重投；业务时间早于路由历史里最近一次已判控制触发的触发入账为「触发早于当前位置」，不判；由此部分停用下面两句｜**部分停用**（2026-10-21）：其决定四「重投」一条里「复核停在未决（复核登记册或路由历史读不回、证据未配置、适用性看不清、取号依赖故障）」一句的未决范围已由 [ADR-0235](./0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md) 停用——证据未配置，以及随之同归未决的没有路由历史、冻结形态未配置，改为入账未判、原因落未判触发登记。**适用场景**：NR 路由复核的收寄采用与交接两路；交接读不回、小包托运读不动与其余未决照旧重投，其余各条不变｜**部分停用**（2026-10-21）：其决定三「更正之后仍是`已交接`：新版本承继接收节点，另成一次复核，原复核不被覆盖。」一句已由 [ADR-0235](./0235-route-reassessment-settles-what-a-retry-cannot-change-and-does-not-judge-triggers-older-than-the-current-position.md) 停用。**适用场景**：更正的那次交接业务时间（承继原交接的判断时刻）早于路由历史里最近一次已判控制触发，这时更正入账为「触发早于当前位置」、不判；更正当前位置那一次交接照判、另成一次复核，更正成拒收或待确认照旧入账不复核，其余各条不变
```

### ADR-0216：派送发起随交接信封在进程内逐对象发起——TF 自消费交接登记信封，按对象此刻在场的段调既有触发执行器，任务成立时间取派发时钟；要求缺失入账不重投，补拍口保留

处理：来向两写合并；接受出处两写合并；补指针

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/60，统一代决见 `.scratch/demo-chain-gaps/handover-dispatch-plan.md`「代决」C10 与「需要的 ADR」ADR-0216。回答 [ADR-0114](./0114-delivery-dispatch-is-triggered-by-entering-a-declared-delivery-segment-and-pulls-requirements-by-reference.md) 决定二末句，参数登记册 `PAR-NET-09` 的拍频格随之退役。越权风险点见文末，待 TF owner 复核。）越权风险点 1「要求缺失不会自愈」、7「交接更正会自动另立一项任务」与 2 的第一种情形已由 [ADR-0230](./0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md) 回答（2026-10-21）：要求缺失同笔落派送待形成册、运营按册重问，随交接的一拍核期望交接、同段替代参与不另立任务；由此**部分停用**决定四一句（被停原句见索引注记），其余各条不变。<!-- 索引注记 -->｜**已接受**（2026-10-19，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 TF owner 裁定；票 demo-chain-gaps/60）。**事件驱动**：派发进程把 `transport-fulfillment.transport-handover.registered` 扇出给 VE 投影与 TF 派送发起两路（VE 在前、各记各的 inbox 账）；派送发起读回那一版交接，不是`已交接`入账，是则按对象找此刻在场的段交既有执行器，零个入账、多个响亮失败；派送段、在场、入场依据与查在册仍由执行器判。**时刻**：任务成立时间取派发处理时钟，这一拍就是 CONTEXT 的「下一拍」，`PAR-NET-09` 拍频格退役、不出按拍批量形态。**处置**：形成、已在册、不是触发、引用被占、要求缺失入账不重投；读不动登本路未决哨兵重投；自家键不受理、执行器不受理或越出封闭集、缝没接线、对象多段在场响亮失败；三格由应用层 `Disposition` 判。**补拍**：作业事实口保留，同（段，对象，入场依据）答已在册。回答 ADR-0114 决定二末句，无条款停用。越权风险点含要求缺失不会自愈、段取此刻在场的段、交接更正自动另立任务，待 TF owner 复核｜**派送待形成留痕与补拍改族**（2026-10-21）：越权风险点 1「要求缺失不会自愈」、7「交接更正会自动另立一项任务」由 [ADR-0230](./0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md) 回答——执行器答`要求缺失`与这一拍同笔落「派送待形成」册、形成了结、离场作罢，运营按册批量重问走同一个执行器，到期重问凭租户登记的间隔、未登记答未配置；随交接信封的一拍把那一版交接交给执行器核链尾（`ENTRY_NOT_THIS_HANDOVER`），查在册沿替代链认前版任务。越权风险点 2 的第一种情形（信封晚到）随之答了，第二种情形（待确认更正为`已交接`）在随交接那一路挡住、根在票 130。**被停条款**：决定四「补拍口的渠道、Intake 与答复形状都不变」里的渠道与 Intake——补拍口改挂运营决定「派送重问」。**适用场景**：只限 `/transport-fulfillment-delivery-dispatch-triggers` 这一口；答复形状与决定四其余各句不变，其余各条不变
```

合并后：

```text
Status: Accepted（2026-10-19，Claude Code 会话代 TF owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/60，统一代决见 `.scratch/demo-chain-gaps/handover-dispatch-plan.md`「代决」C10 与「需要的 ADR」ADR-0216。回答 [ADR-0114](./0114-delivery-dispatch-is-triggered-by-entering-a-declared-delivery-segment-and-pulls-requirements-by-reference.md) 决定二末句，参数登记册 `PAR-NET-09` 的拍频格随之退役。越权风险点见文末，待 TF owner 复核。）<!-- 索引注记 -->｜**事件驱动**：派发进程把 `transport-fulfillment.transport-handover.registered` 扇出给 VE 投影与 TF 派送发起两路（VE 在前、各记各的 inbox 账）；派送发起读回那一版交接，不是`已交接`入账，是则按对象找此刻在场的段交既有执行器，零个入账、多个响亮失败；派送段、在场、入场依据与查在册仍由执行器判。**时刻**：任务成立时间取派发处理时钟，这一拍就是 CONTEXT 的「下一拍」，`PAR-NET-09` 拍频格退役、不出按拍批量形态。**处置**：形成、已在册、不是触发、引用被占、要求缺失入账不重投；读不动登本路未决哨兵重投；自家键不受理、执行器不受理或越出封闭集、缝没接线、对象多段在场响亮失败；三格由应用层 `Disposition` 判。**补拍**：作业事实口保留，同（段，对象，入场依据）答已在册。越权风险点含要求缺失不会自愈、段取此刻在场的段、交接更正自动另立任务｜**派送待形成留痕与补拍改族**（2026-10-21）：越权风险点 1「要求缺失不会自愈」、7「交接更正会自动另立一项任务」由 [ADR-0230](./0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md) 回答——执行器答`要求缺失`与这一拍同笔落「派送待形成」册、形成了结、离场作罢，运营按册批量重问走同一个执行器，到期重问凭租户登记的间隔、未登记答未配置；随交接信封的一拍把那一版交接交给执行器核链尾（`ENTRY_NOT_THIS_HANDOVER`），查在册沿替代链认前版任务。越权风险点 2 的第一种情形（信封晚到）随之答了，第二种情形（待确认更正为`已交接`）在随交接那一路挡住、根在票 130。**被停条款**：其决定四「补拍口的渠道、Intake 与答复形状都不变」里的渠道与 Intake——补拍口改挂运营决定「派送重问」。**适用场景**：只限 `/transport-fulfillment-delivery-dispatch-triggers` 这一口；答复形状与决定四其余各句不变，其余各条不变｜**部分停用**（指针 2026-10-21 补）：其决定一「信封晚到时，对象可能已经被下一次交接带走。这时此刻在场的段才是该派送的那一段」一句已由 [ADR-0230](./0230-delivery-dispatch-left-unformed-is-kept-on-a-pending-register-and-reasked-through-the-same-executor.md) 决定六停用——随交接信封发起的一拍把那一版交接交给执行器核期望交接，链尾不是凭它入场即答`不是触发事实`（`ENTRY_NOT_THIS_HANDOVER`）、信封入账，晚到的信封不替后一段形成派送任务。**适用场景**：只限随交接信封发起的那一路；补拍与按册重问不传期望交接，照旧取此刻在场的段，其余各条不变
```

### ADR-0217：路由计划在交付节点之后带一段交付段——终点是交付范围，窗口是离开交付节点加上派送时长；段带种类，已执行前缀与运输准备只取节点间段

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；方案 [handover-dispatch-plan](../../.scratch/demo-chain-gaps/handover-dispatch-plan.md)「代决」C11，票 [demo-chain-gaps/64](../../.scratch/demo-chain-gaps/issues/64-route-plan-carries-a-delivery-leg.md)。越权风险点见文末，待 NR owner 复核，TF owner 会签。在 [ADR-0131](./0131-planned-leg-is-referenced-by-plan-version-and-ordinal-and-the-delivery-window-seam-answers-content-not-applicability.md) 决定一的引用形之内；部分停用 [ADR-0175](./0175-service-calendar-content-folds-absolute-time.md) 决定一、决定四各一句与 [ADR-0201](./0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md) 决定二末句，被停原句与适用场景见 adr/README 各该行，其余各条不变。）<!-- 索引注记 -->｜**已接受**（2026-10-19，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 NR owner 裁定、TF owner 会签；票 demo-chain-gaps/64）。**交付段**：至多一段、只在末尾，起于交付节点，止于候选的服务区域解析覆盖收件地址所用的服务区域版本，不是节点；责任暂指同一区域版本。**窗口**：离开交付节点的时刻加派送时长；派送时长是交付节点日历上的新格（可空、不补默认，NR 迁移 0020），没登不成段、计划止于交付节点；成段时候选投影延到交付段终点。**种类**：节点间 / 交付，不是段服务动作，派送段仍由登记方声明；旧计划行无种类即节点间段。**只取节点间段**：计划节点、已执行前缀与冻结剩余段数、适用性复核、TF 运输准备、候选比价。**读面**：段引用按整条段链数，窗口缝不按种类分支；路由计划查阅口成计划行重建后逐段列出。部分停用 ADR-0175 决定一、决定四各一句与 ADR-0201 决定二末句。越权风险点含总窗口变长、责任暂指区域、时长挂节点不挂区域、交付段不进比价、查阅口重建而复核册暂不列段、管理台夹具缺 `legs`，待 NR、TF owner 复核
```

合并后：

```text
Status: Accepted（2026-10-19，Claude Code 会话代 NR owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」；方案 [handover-dispatch-plan](../../.scratch/demo-chain-gaps/handover-dispatch-plan.md)「代决」C11，票 [demo-chain-gaps/64](../../.scratch/demo-chain-gaps/issues/64-route-plan-carries-a-delivery-leg.md)。越权风险点见文末，待 NR owner 复核，TF owner 会签。在 [ADR-0131](./0131-planned-leg-is-referenced-by-plan-version-and-ordinal-and-the-delivery-window-seam-answers-content-not-applicability.md) 决定一的引用形之内；部分停用 [ADR-0175](./0175-service-calendar-content-folds-absolute-time.md) 决定一、决定四各一句与 [ADR-0201](./0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md) 决定二末句，被停原句与适用场景见 ADR-0175、ADR-0201 各自的 Status 行，其余各条不变。）<!-- 索引注记 -->｜**交付段**：至多一段、只在末尾，起于交付节点，止于候选的服务区域解析覆盖收件地址所用的服务区域版本，不是节点；责任暂指同一区域版本。**窗口**：离开交付节点的时刻加派送时长；派送时长是交付节点日历上的新格（可空、不补默认，NR 迁移 0020），没登不成段、计划止于交付节点；成段时候选投影延到交付段终点。**种类**：节点间 / 交付，不是段服务动作，派送段仍由登记方声明；旧计划行无种类即节点间段。**只取节点间段**：计划节点、已执行前缀与冻结剩余段数、适用性复核、TF 运输准备、候选比价。**读面**：段引用按整条段链数，窗口缝不按种类分支；路由计划查阅口成计划行重建后逐段列出。越权风险点含总窗口变长、责任暂指区域、时长挂节点不挂区域、交付段不进比价、查阅口重建而复核册暂不列段、管理台夹具缺 `legs`
```

### ADR-0218：实际履约发生项由交接与有效交付信封自动形成——只对已采用委托的段，找出被这件事实结束的参与，经既有登记用例形成；旅程取替代 / 退运旅程，否则按包裹派生

处理：来向两写合并；接受出处两写合并；补指针

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/67，统一代决见 `.scratch/demo-chain-gaps/handover-dispatch-plan.md`「代决」C15 与「需要的 ADR」ADR-0218。**部分停用** [ADR-0207](./0207-transport-charge-occurrences-are-registered-by-an-operation-decision-and-handed-off-in-the-same-transaction.md) 决定七后半句、越权风险点 3 与决定四末句的演示约定，以及 [ADR-0209](./0209-supplier-agreements-declare-charge-occurrence-conditions-and-registration-judges-by-them.md) 决定五「登记者取自操作者信封的主体」一句，被停条款与适用场景见「对已接受记录的作用」。越权风险点见文末，待 TF owner 复核，第 2、3 点同时待 SA owner、第 5 点同时待 PC owner。） **部分停用**（2026-10-19）：决定一「是`已交接`的，终点依据取交接自己交出的转出依据」与「多于一条：库面不一致，响亮失败，不挑一段去计费」两句、决定六「响亮失败」下「同一件事实在多个段里都结束了参与」一条，已由 [ADR-0221](./0221-a-handover-participation-basis-is-minted-from-the-handover-key.md) 停用——终点依据改由交接键铸，存量按版本加时刻认；后两处只停在旧拼法存量那一格（对上多于一条答`结束的参与认不出`、入账），逐字对上多于一条仍响亮失败；其余各条不变。越权风险点 1「自动形成没赶上的参与不补」、6「入账的业务格不会自愈」已由 [ADR-0231](./0231-actual-fulfillment-left-unformed-is-caught-up-by-doorbells-and-a-commission-agreement-revision.md) 回答（2026-10-21），由此**部分停用**决定七末段两项（被停原句见索引注记），其余各条不变。<!-- 索引注记 -->｜**已接受**（2026-10-19，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 TF owner 裁定，待 SA、PC owner 会签各自那几条越权风险点；票 demo-chain-gaps/67）。**自消费**：派发进程给交接登记与有效交付登记两种信封各多扇出一路，排在既有各路之后、消费者名各自分家；交接取`已交接`那一版的转出依据，交付取信封所指那一版结果，在对象有过参与的各段里找终点恰是这件事实的那一条，零条入账、多条响亮失败。**只对已采用委托的段**：没有采用连线入账零写入，不交登记用例、不推断自营，照旧人工登记。**走既有登记用例**：命令只带原因、段、这一个对象、旅程与登记者，依据、业务时间、采购上下文、协议、条件、数量与覆盖都由登记用例一处判，身份与人工登记同一个、登记者不进内容比对。**旅程**：取参与入场时已开始、开始最晚的那一趟替代 / 退运旅程，同刻两趟响亮失败；没有就派生 `JRN/<包裹>`，演示约定随之改拼写。**登记者**：本上下文的系统身份。**处置**：已登记、已存在、冲突、协议否定格、未配置、不计费、缺采购上下文入账；读不动与登记未决登本路哨兵重投；自家键不受理、多段结束、旅程同刻、登记答不适用或集外格响亮失败。部分停用 ADR-0207 决定七后半句、越权风险点 3 与决定四末句的演示约定，ADR-0209 决定五一句。越权风险点含没赶上的参与不补（票 demo-chain-gaps/88）、更正撤回控制不回退、系统身份登记者、旅程取法、按段计成员只有第一件｜**部分停用**：决定一「是`已交接`的，终点依据取交接自己交出的转出依据」与「多于一条：库面不一致，响亮失败，不挑一段去计费」两句、决定六「响亮失败」下「同一件事实在多个段里都结束了参与」一条，已由 [ADR-0221](./0221-a-handover-participation-basis-is-minted-from-the-handover-key.md) 停用。**适用场景**：第一句全部停用——终点依据改由交接键铸，存量按版本加终点时刻认；后两处只停在 ADR-0221 之前只带版本的存量终点依据那一格，对上多于一条改答`结束的参与认不出`（`ENDED_PARTICIPATION_AMBIGUOUS`）、入账，逐字对上多于一条仍是库面不一致、响亮失败；其余各条不变｜**补形成**（2026-10-21）：越权风险点 1、6 由 [ADR-0231](./0231-actual-fulfillment-left-unformed-is-caught-up-by-doorbells-and-a-commission-agreement-revision.md) 回答——采用成立与委托协议换版各在自己那一笔交门铃，派发进程把段上以下一次交接或有效交付结束的参与交回同一个形成后半；答未配置、协议未生效、已终止、缺采购上下文的留在实际履约未形成册上、查阅页列出。**被停条款**：决定七末段「下面几种情形要靠它」里「采用晚于派发处理那封信的段；派发处理时协议答未配置、后来才另发一版补上条件的段」两项。**适用场景**：这两种情形改由门铃补形成；没有采用连线的段、迁移 0038 之前提交的委托、明确终止结束的参与照旧靠人工口，其余各条不变
```

合并后：

```text
Status: Accepted（2026-10-19，Claude Code 会话代 TF owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/67，统一代决见 `.scratch/demo-chain-gaps/handover-dispatch-plan.md`「代决」C15 与「需要的 ADR」ADR-0218。**部分停用** [ADR-0207](./0207-transport-charge-occurrences-are-registered-by-an-operation-decision-and-handed-off-in-the-same-transaction.md) 决定七后半句、越权风险点 3 与决定四末句的演示约定，以及 [ADR-0209](./0209-supplier-agreements-declare-charge-occurrence-conditions-and-registration-judges-by-them.md) 决定五「登记者取自操作者信封的主体」一句，被停条款与适用场景见「对已接受记录的作用」。另：决定四的旅程取法使 ADR-0207 决定四「旅程由登记方声明，本上下文没有它的权威来源，不核」一句在自动形成那一路不再成立，见 ADR-0207 的 Status 行（指针 2026-10-21 补）。越权风险点见文末，待 TF owner 复核，第 2、3 点同时待 SA owner、第 5 点同时待 PC owner。）<!-- 索引注记 -->｜**自消费**：派发进程给交接登记与有效交付登记两种信封各多扇出一路，排在既有各路之后、消费者名各自分家；交接取`已交接`那一版的转出依据，交付取信封所指那一版结果，在对象有过参与的各段里找终点恰是这件事实的那一条，零条入账、多条响亮失败。**只对已采用委托的段**：没有采用连线入账零写入，不交登记用例、不推断自营，照旧人工登记。**走既有登记用例**：命令只带原因、段、这一个对象、旅程与登记者，依据、业务时间、采购上下文、协议、条件、数量与覆盖都由登记用例一处判，身份与人工登记同一个、登记者不进内容比对。**旅程**：取参与入场时已开始、开始最晚的那一趟替代 / 退运旅程，同刻两趟响亮失败；没有就派生 `JRN/<包裹>`，演示约定随之改拼写。**登记者**：本上下文的系统身份。**处置**：已登记、已存在、冲突、协议否定格、未配置、不计费、缺采购上下文入账；读不动与登记未决登本路哨兵重投；自家键不受理、多段结束、旅程同刻、登记答不适用或集外格响亮失败。越权风险点含没赶上的参与不补（票 demo-chain-gaps/88）、更正撤回控制不回退、系统身份登记者、旅程取法、按段计成员只有第一件｜**部分停用**（2026-10-19）：其决定一「是`已交接`的，终点依据取交接自己交出的转出依据」与「多于一条：库面不一致，响亮失败，不挑一段去计费」两句、决定六「响亮失败」下「同一件事实在多个段里都结束了参与」一条，已由 [ADR-0221](./0221-a-handover-participation-basis-is-minted-from-the-handover-key.md) 停用。**适用场景**：第一句全部停用——终点依据改由交接键铸，存量按版本加终点时刻认；后两处只停在 ADR-0221 之前只带版本的存量终点依据那一格，对上多于一条改答`结束的参与认不出`（`ENDED_PARTICIPATION_AMBIGUOUS`）、入账，逐字对上多于一条仍是库面不一致、响亮失败；其余各条不变｜**补形成**（2026-10-21）：越权风险点 1、6 由 [ADR-0231](./0231-actual-fulfillment-left-unformed-is-caught-up-by-doorbells-and-a-commission-agreement-revision.md) 回答——采用成立与委托协议换版各在自己那一笔交门铃，派发进程把段上以下一次交接或有效交付结束的参与交回同一个形成后半；答未配置、协议未生效、已终止、缺采购上下文的留在实际履约未形成册上、查阅页列出。**被停条款**：决定七末段「下面几种情形要靠它」里「采用晚于派发处理那封信的段；派发处理时协议答未配置、后来才另发一版补上条件的段」两项。**适用场景**：这两种情形改由门铃补形成；没有采用连线的段、迁移 0038 之前提交的委托、明确终止结束的参与照旧靠人工口，其余各条不变
```

### ADR-0219：设备出发前门禁问询——作业事实渠道上的只读一问，交回与随后同参出发同一套读数；运输履约不落记录，关务照记判断

处理：来向两写合并；接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/69，统一代决见 `.scratch/demo-chain-gaps/handover-dispatch-plan.md`「代决」C17 与「需要的 ADR」ADR-0219。回答 [ADR-0205](./0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md) 越权风险点 1「挡下要靠设备在动作之前问；那个读口……等 peer 收工后另立票」，无条款停用。越权风险点见文末，待各条括注的 owner 复核。）决定二「读数还取决于两问之间的段册、班次册与关务三源」由 [ADR-0225](./0225-the-departure-gate-reading-asks-through-consolidation-unit-members-at-the-moment-of-departure.md) 补两源（2026-10-20）：出发读数展开集运单元，读数另取决于节点作业的集运成员关系册与收寄册；同参同读数仍靠只有一条路，问询与出发同样展开。无条款停用，本记录各条不变。 越权风险点 5「段里有什么可以零成本探知」由 [ADR-0237](./0237-gate-readings-show-the-executor-only-the-objects-that-did-not-pass.md) 回答（2026-10-21）：问询与出发两口回给设备的逐对象那一格一起收窄成只列没得出放行的对象（阻断、读不动、未决），落册照旧记全；同一记录决定五让两口都核班次起讫与段里各件进段交接声明的交出、去向节点，对不上就停在本侧、不问关务，说不出照问、回给设备的不列对象，读数另取决于交接登记册；决定一「同一个线格式」与决定二「同参同读数」照旧成立，无条款停用，本记录各条不变。<!-- 索引注记 -->｜**已接受**（2026-10-19，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 TF、CC owner 裁定；票 demo-chain-gaps/69）。**问询口**：`POST /transport-fulfillment/departure-gate-previews`，载荷只收班次、可缺的段与拟出发时点（封闭解码，时点缺席答未受理、不补服务端时钟），答复一律 200（`DEPARTURE_GATE_PREVIEWED` 带 `gateReading`，或 `INPUT_NOT_ACCEPTED`），`gateReading` 与移动事实口同一线格式。**同参同读数**：两口受理后收成同一组（租户、班次、段、时点）走同一条读数路径，受理与否也一致；同格只在两问之间册子与三源不变时成立。**不落记录**：运输履约不写册、不入队、不记谁问过，问询读数不交给随后的出发；关务照 ADR-0206 决定六记判断，同参出发命中同一判断。**授予**：与移动事实口同一只作业事实 Intake，不新增能力种类。**不挡出发**：挡不挡归一线客户端。回答 ADR-0205 越权风险点 1，无条款停用。越权风险点含读到放行不等于出发那一刻仍放行、用写授予开读口、客户端处置未定、关务判断册随轮询增长、段里有什么可零成本探知、不记谁问过，待 TF、CC owner 复核｜**读数多两源**（2026-10-20）：[ADR-0225](./0225-the-departure-gate-reading-asks-through-consolidation-unit-members-at-the-moment-of-departure.md) 让出发读数按成员展开集运单元，决定二「读数还取决于两问之间的段册、班次册与关务三源」之外另取决于节点作业的集运成员关系册与收寄册；问询走同一条路、同样展开；无条款停用，其余各条不变｜**回给设备的那一格收窄**（2026-10-21）：越权风险点 5「段里有什么可以零成本探知」由 [ADR-0237](./0237-gate-readings-show-the-executor-only-the-objects-that-did-not-pass.md) 回答——问询与出发两口回给设备的逐对象那一格一起只列没得出放行的对象（阻断、读不动、未决），集运单元只指名同样列出的成员，判法只在领域 `ExecutorGateCells`，落册照旧记全；两口经同一判法，逐字节仍相同；剩余暴露（任一场外设备仍问得出任一段没得出放行的那几件、不留痕）记在 ADR-0237；同一记录决定五让两口都核班次起讫与段里各件进段交接声明的交出、去向节点，对不上停在本侧、说不出照问而只回整格，决定二「读数还取决于两问之间的段册、班次册与关务三源」之外另取决于交接登记册；无条款停用，其余各条不变
```

合并后：

```text
Status: Accepted（2026-10-19，Claude Code 会话代 TF、CC owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/69，统一代决见 `.scratch/demo-chain-gaps/handover-dispatch-plan.md`「代决」C17 与「需要的 ADR」ADR-0219。回答 [ADR-0205](./0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md) 越权风险点 1「挡下要靠设备在动作之前问；那个读口……等 peer 收工后另立票」，无条款停用。越权风险点见文末，待各条括注的 owner（TF、CC owner）复核。）<!-- 索引注记 -->｜**问询口**：`POST /transport-fulfillment/departure-gate-previews`，载荷只收班次、可缺的段与拟出发时点（封闭解码，时点缺席答未受理、不补服务端时钟），答复一律 200（`DEPARTURE_GATE_PREVIEWED` 带 `gateReading`，或 `INPUT_NOT_ACCEPTED`），`gateReading` 与移动事实口同一线格式。**同参同读数**：两口受理后收成同一组（租户、班次、段、时点）走同一条读数路径，受理与否也一致；同格只在两问之间册子与三源不变时成立。**不落记录**：运输履约不写册、不入队、不记谁问过，问询读数不交给随后的出发；关务照 ADR-0206 决定六记判断，同参出发命中同一判断。**授予**：与移动事实口同一只作业事实 Intake，不新增能力种类。**不挡出发**：挡不挡归一线客户端。越权风险点含读到放行不等于出发那一刻仍放行、用写授予开读口、客户端处置未定、关务判断册随轮询增长、段里有什么可零成本探知、不记谁问过｜**读数多两源**（2026-10-20）：[ADR-0225](./0225-the-departure-gate-reading-asks-through-consolidation-unit-members-at-the-moment-of-departure.md) 让出发读数按成员展开集运单元，决定二「读数还取决于两问之间的段册、班次册与关务三源」之外另取决于节点作业的集运成员关系册与收寄册；问询走同一条路、同样展开；无条款停用，其余各条不变｜**回给设备的那一格收窄**（2026-10-21）：越权风险点 5「段里有什么可以零成本探知」由 [ADR-0237](./0237-gate-readings-show-the-executor-only-the-objects-that-did-not-pass.md) 回答——问询与出发两口回给设备的逐对象那一格一起只列没得出放行的对象（阻断、读不动、未决），集运单元只指名同样列出的成员，判法只在领域 `ExecutorGateCells`，落册照旧记全；两口经同一判法，逐字节仍相同；剩余暴露（任一场外设备仍问得出任一段没得出放行的那几件、不留痕）记在 ADR-0237；同一记录决定五让两口都核班次起讫与段里各件进段交接声明的交出、去向节点，对不上停在本侧、说不出照问而只回整格，决定二「读数还取决于两问之间的段册、班次册与关务三源」之外另取决于交接登记册；决定一「同一个线格式」与决定二「同参同读数」照旧成立，无条款停用，其余各条不变
```

### ADR-0220：节点控制认交接更正链只按运输履约的更正回指——同一（对象，范围）下另起的首登是另一次交接，不是前一次的更正

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/95，出自 `.scratch/product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md`「重走（钉 `8197fa2f`，2026-10-19）」停点一。部分停用 [ADR-0214](./0214-node-control-transfers-out-and-in-on-an-authoritative-transport-handover.md) 决定五「同链认法」一句，回答它的越权风险点「同链按（对象，范围）认」与文末补记的越权风险点「一次交接不转出它自己立起的控制，靠的仍是同链按（对象，范围）认」。越权风险点见文末，待 NO owner 复核，TF owner 会签。）<!-- 索引注记 -->｜**已接受**（2026-10-19，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 NO owner 裁定、TF owner 会签；票 demo-chain-gaps/95）。**认链**：一版交接的更正链是它与它沿 TF 更正回指上溯的各前版；同一（对象，范围）只是链上各版共有的性质，不是判据，同一范围下另起的首登在两个方向上都按它自己判。**上溯**：消费侧适配器读回那一版后沿回指逐版按键读回前版，不加 TF 读口；前版读不动登本路既有未决哨兵重投，读不回、与键不符、回指成环响亮失败；编排立链再核一次形状。**转移册**：「链上推进过没有」只取链上各版，同一范围下可有多行推进，不加迁移。**TF 首登口不改**：不拒同一（对象，范围）下另起的首登；「范围每次交接各起一个」是用法约定。**存量**不改写、不重判，处置归票 demo-chain-gaps/81。部分停用 ADR-0214 决定五一句。越权风险点含 TF 范围汇总对同一对象两次首登算两次、存量认错的行留在册上、补拍旧信封答复会变、每封信多读 TF、前版读不回按缺陷、更正换范围时立链会拒，待 NO owner 复核、TF owner 会签
```

合并后：

```text
Status: Accepted（2026-10-19，Claude Code 会话代 NO owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/95，出自 `.scratch/product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md`「重走（钉 `8197fa2f`，2026-10-19）」停点一。部分停用 [ADR-0214](./0214-node-control-transfers-out-and-in-on-an-authoritative-transport-handover.md) 决定五「同链认法」一句，回答它的越权风险点「同链按（对象，范围）认」与文末补记的越权风险点「一次交接不转出它自己立起的控制，靠的仍是同链按（对象，范围）认」。越权风险点见文末，待 NO owner 复核，TF owner 会签。）<!-- 索引注记 -->｜**认链**：一版交接的更正链是它与它沿 TF 更正回指上溯的各前版；同一（对象，范围）只是链上各版共有的性质，不是判据，同一范围下另起的首登在两个方向上都按它自己判。**上溯**：消费侧适配器读回那一版后沿回指逐版按键读回前版，不加 TF 读口；前版读不动登本路既有未决哨兵重投，读不回、与键不符、回指成环响亮失败；编排立链再核一次形状。**转移册**：「链上推进过没有」只取链上各版，同一范围下可有多行推进，不加迁移。**TF 首登口不改**：不拒同一（对象，范围）下另起的首登；「范围每次交接各起一个」是用法约定。**存量**不改写、不重判，处置归票 demo-chain-gaps/81。越权风险点含 TF 范围汇总对同一对象两次首登算两次、存量认错的行留在册上、补拍旧信封答复会变、每封信多读 TF、前版读不回按缺陷、更正换范围时立链会拒
```

### ADR-0221：权威交接作参与依据时由交接键铸——对象、范围、版本三节长度前缀，另起一个起头；只带版本的存量照读、按版本加时刻认，NO 的转出引用不随之换

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/111，出自 `.scratch/product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md`「重走（钉 `76686b7b`，2026-10-19）」停点一与文末「另记一处观察」。**部分停用** [ADR-0218](./0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md) 决定一两句与决定六「响亮失败」下一条，被停条款与适用场景见「对已接受记录的作用」。越权风险点见文末，待 TF owner 复核，第 3 点同时待 NO owner 会签。）<!-- 索引注记 -->｜**已接受**（2026-10-19，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 TF owner 裁定；票 demo-chain-gaps/111）。**铸法**：`TRANSPORT-HANDOVER-KEY/` 后接对象、范围、版本各一节「字节长度:原串」，用于进段、结束与重派生长出的各版；只有`已交接`给得出；另起起头，新旧串集合不相交。**存量**照读不改写：逐字对上的一定是那一次，旧拼法按版本加按它立下的时刻认。**自动形成**：逐字对上的优先，多于一条仍响亮失败；旧拼法对上多于一条答`结束的参与认不出`、入账。**交付门禁与重派生**同一套认法，`ENTRY_HANDOVER_AMBIGUOUS` 自此只出自存量。**NO 的转出引用不变**，换不换归票 112。别的控制事实（场外揽收、实际承运商首次有效收寄、有效交付）的版本由本上下文按全局序列铸、本身唯一，依据只带版本不在此列。部分停用 ADR-0218 决定一两句与决定六一条。越权风险点含存量歧义仍在、旧拼法恰一条也要时刻对上、同一次交接两串、派送任务引用变长，待 TF owner 复核，NO owner 会签第 3 点
```

合并后：

```text
Status: Accepted（2026-10-19，Claude Code 会话代 TF owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/111，出自 `.scratch/product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md`「重走（钉 `76686b7b`，2026-10-19）」停点一与文末「另记一处观察」。**部分停用** [ADR-0218](./0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md) 决定一两句与决定六「响亮失败」下一条，被停条款与适用场景见「对已接受记录的作用」。越权风险点见文末，待 TF owner 复核，第 3 点同时待 NO owner 会签。）<!-- 索引注记 -->｜**铸法**：`TRANSPORT-HANDOVER-KEY/` 后接对象、范围、版本各一节「字节长度:原串」，用于进段、结束与重派生长出的各版；只有`已交接`给得出；另起起头，新旧串集合不相交。**存量**照读不改写：逐字对上的一定是那一次，旧拼法按版本加按它立下的时刻认。**自动形成**：逐字对上的优先，多于一条仍响亮失败；旧拼法对上多于一条答`结束的参与认不出`、入账。**交付门禁与重派生**同一套认法，`ENTRY_HANDOVER_AMBIGUOUS` 自此只出自存量。**NO 的转出引用不变**，换不换归票 112。别的控制事实（场外揽收、实际承运商首次有效收寄、有效交付）的版本由本上下文按全局序列铸、本身唯一，依据只带版本不在此列。越权风险点含存量歧义仍在、旧拼法恰一条也要时刻对上、同一次交接两串、派送任务引用变长
```

### ADR-0222：组合根按特性文件自登记——parcel-api 的端点行与 parcel-dispatch 的路由各由特性文件在 init 里登进本包登记处，生产与装配测试取同一份

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」，并于 2026-10-19 同意先做两张基础设施票（全量提速、热点清单去冲突）；票 infra-speed/02。越权风险点见文末，待 owner 复核。）<!-- 索引注记 -->｜**已接受**（2026-10-19，用户原话「你作为业务和系统专家自决」，Claude Code 会话代决；票 infra-speed/02）。**登记处**：每个命令一张包级登记表，特性文件在 `init` 里只追加构造函数，构造在开池之后由装配点逐项调用，任一失败原样交回、进程非零退出。**parcel-api**：`endpoints_<特性>.go` 登记各行的方法、接入面与探测目标，`build` 装编排、`mount` 出 `[]httpapi.BusinessEndpoint` 字面量，未接线占位由同名测试文件登记，生产与装配测试走同一个 `mount`；探针表与各换口清单由登记处导出，另立闭合用例核声明的接入面。**parcel-dispatch**：`routes_<特性>.go` 登记各路的事件类型、扇出名次与未决哨兵（或显式不翻译），`wireDispatcher` 按事件类型归组、按名次扇出；同名次、哨兵漏写与同一路径登两次装配即拒；既有哨兵名单留在原处。**门禁与清点**：端点字面量照旧，清点工具的路由一栏改数登记的不同事件类型，数不变。不停用任何条款。越权风险点含探测目标写在生产文件、委托查阅读适配器多建一只、构造顺序随文件名、清点措辞改变、第一次用 `init`、自建 Intake 的行与客户面能力格闭合用例核不到
```

合并后：

```text
Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」，并于 2026-10-19 同意先做两张基础设施票（全量提速、热点清单去冲突）；票 infra-speed/02。越权风险点见文末，待 owner 复核。）<!-- 索引注记 -->｜**登记处**：每个命令一张包级登记表，特性文件在 `init` 里只追加构造函数，构造在开池之后由装配点逐项调用，任一失败原样交回、进程非零退出。**parcel-api**：`endpoints_<特性>.go` 登记各行的方法、接入面与探测目标，`build` 装编排、`mount` 出 `[]httpapi.BusinessEndpoint` 字面量，未接线占位由同名测试文件登记，生产与装配测试走同一个 `mount`；探针表与各换口清单由登记处导出，另立闭合用例核声明的接入面。**parcel-dispatch**：`routes_<特性>.go` 登记各路的事件类型、扇出名次与未决哨兵（或显式不翻译），`wireDispatcher` 按事件类型归组、按名次扇出；同名次、哨兵漏写与同一路径登两次装配即拒；既有哨兵名单留在原处。**门禁与清点**：端点字面量照旧，清点工具的路由一栏改数登记的不同事件类型，数不变。不停用任何条款。越权风险点含探测目标写在生产文件、委托查阅读适配器多建一只、构造顺序随文件名、清点措辞改变、第一次用 `init`、自建 Intake 的行与客户面能力格闭合用例核不到
```

### ADR-0223：首方提交答复逐件带回包裹号——建成委托的答复带 `parcels`，每件配客户包裹参考；号从委托读，重放读回已签的、不重签；首方契约对客户追踪视图承诺的查询标识就是这个号

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-19，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/101，协调方代决先走「答复带回号」这条路。补 [ADR-0140](./0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md) 的首方契约：ADR-0140 Decision 八把端点字段留给各上下文 owner 与其用例，本记录定提交答复的一格与客户追踪视图对外承诺的查询标识。ADR-0140 正文不改，只在其 Status 行与 Links 加前向指针；不停用任何已接受记录的条款。越权风险点见文末，待 PS owner 复核；决定三（连同 UC-VE-008 查询标识一行的改动）与第 3、6、7 点同时待 VE owner 会签。）<!-- 索引注记 -->｜**已接受**（2026-10-19，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 PS owner 裁定、VE owner 会签决定三；票 demo-chain-gaps/101）。**答复**：`SUBMITTED` 与指名了原委托的 `EXISTING_RESULT` 带 `parcels`，每件 `parcelId` 加客户为它声明的 `customerParcelReference`（没声明整键缺席），次序同首个提交版本，件号回显委托上去过两端空白的声明值；没建成委托的答复整键不出现。**号从委托读**：建成、重放、并发建单撞已存在都读委托，接单入口重放时现签的号不出现、不重签。**查询标识**：首方契约对 `/customer-tracking-view` 承诺的是这个号；标识只定位不是凭证，VE 答复代数不变。不另开按委托号读回逐件号的读面（号在答复形成时已在）；按客户参考号直接定位推后到票 119。补 ADR-0140 的首方契约，不停用任何条款。越权风险点含加键是契约变更、次序是附带承诺、接受前查视图与不存在同答、号在接受前交给客户、非首方口无件号的件只能按次序对、内部标识成了客户可见的契约键且拆分合并后成历史身份、决定三替 VE 许诺，待 PS owner 复核，VE owner 会签决定三与第 3、6、7 点
```

合并后：

```text
Status: Accepted（2026-10-19，Claude Code 会话代 PS owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/101，协调方代决先走「答复带回号」这条路。补 [ADR-0140](./0140-first-party-api-contract-is-generated-from-endpoint-descriptors-with-url-major-version-and-problem-details.md) 的首方契约：ADR-0140 Decision 八把端点字段留给各上下文 owner 与其用例，本记录定提交答复的一格与客户追踪视图对外承诺的查询标识。ADR-0140 正文不改，只在其 Status 行与 Links 加前向指针；不停用任何已接受记录的条款。越权风险点见文末，待 PS owner 复核；决定三（连同 UC-VE-008 查询标识一行的改动）与第 3、6、7 点同时待 VE owner 会签。）<!-- 索引注记 -->｜**答复**：`SUBMITTED` 与指名了原委托的 `EXISTING_RESULT` 带 `parcels`，每件 `parcelId` 加客户为它声明的 `customerParcelReference`（没声明整键缺席），次序同首个提交版本，件号回显委托上去过两端空白的声明值；没建成委托的答复整键不出现。**号从委托读**：建成、重放、并发建单撞已存在都读委托，接单入口重放时现签的号不出现、不重签。**查询标识**：首方契约对 `/customer-tracking-view` 承诺的是这个号；标识只定位不是凭证，VE 答复代数不变。不另开按委托号读回逐件号的读面（号在答复形成时已在）；按客户参考号直接定位推后到票 119。越权风险点含加键是契约变更、次序是附带承诺、接受前查视图与不存在同答、号在接受前交给客户、非首方口无件号的件只能按次序对、内部标识成了客户可见的契约键且拆分合并后成历史身份、决定三替 VE 许诺
```

### ADR-0224：先于段登记的移动事实由全程追踪在对象凭交接或场外揽收进段时补派生——按段列出在册事实，同一判据、同一幂等键，只补投影

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-20，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」，并于 2026-10-19 同意只做分诊 A 组；票 demo-chain-gaps/53，协调方代决先走「VE 在交接立段时经 TF 只读口按段补派生」这条路，评审后补上场外揽收那一路。回答 [ADR-0205](./0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md) 文末补记越权风险点 14；ADR-0205 正文不改，只在其 Status 行与 Links 加前向指针，不停用任何已接受记录的条款。越权风险点见文末，待 VE owner 复核，决定二、决定三的领域门与第 1、2 点同时待 TF owner 会签。）<!-- 索引注记 -->｜**已接受**（2026-10-20，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 VE owner 裁定、TF owner 会签决定二与决定三的领域门；票 demo-chain-gaps/53）。**谁补**：全程追踪在消费侧补，运输履约不为此重交信封、不造「段已成立」信封。**读口**：TF ports `MovementFactsOfSegment.FindBySegment` 按（租户，段）交回声明了这一段的全部在册版本，排序契约是先按事实、再按登记时刻、再按版本串，不问段在不在册，一行装不回整次报错；迁移 0044 加部分索引；适配器在 VE 消费侧。**唤醒**：`transport-handover.registered` 与 `offsite-pickup.registered` 的扇出各另起一路、另记一本账，排在同一封信上的投影之后；交接只认转出控制的那一版；只看这一次控制事实让对象进的那一段（领域门 `EnteredByHandover`、新加的 `EnteredByPickup`），只补这一件。**同判据同键**：在控取 `ObjectsUnderControlAt`（事实发生时刻、按消费那一刻段册），派生命令与移动事实那一路是同一段代码，两路谁先谁后、重投几次都只留一份；更正链逐版补。**只补投影**：不补信号、不重问关务，补出的出发没有该件的门禁读数，补问归 ADR-0205 越权风险点 2。揽收执行口（尝试级信封）与承运商首次有效收寄立的段不在本记录（票 120）；上线前已丢的出发不自愈。不停用任何条款。越权风险点含进段欠账时补不上、补出的出发不带门禁读数、执行口与承运商段不补、每次控制事实多读段与事实、停住时挡同一分区且坏行跨对象波及，待 VE owner 复核，TF owner 会签决定二、决定三的领域门与第 1、2 点
```

合并后：

```text
Status: Accepted（2026-10-20，Claude Code 会话代 VE owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」，并于 2026-10-19 同意只做分诊 A 组；票 demo-chain-gaps/53，协调方代决先走「VE 在交接立段时经 TF 只读口按段补派生」这条路，评审后补上场外揽收那一路。回答 [ADR-0205](./0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md) 文末补记越权风险点 14；ADR-0205 正文不改，只在其 Status 行与 Links 加前向指针，不停用任何已接受记录的条款。越权风险点见文末，待 VE owner 复核，决定二、决定三的领域门与第 1、2 点同时待 TF owner 会签。）<!-- 索引注记 -->｜**谁补**：全程追踪在消费侧补，运输履约不为此重交信封、不造「段已成立」信封。**读口**：TF ports `MovementFactsOfSegment.FindBySegment` 按（租户，段）交回声明了这一段的全部在册版本，排序契约是先按事实、再按登记时刻、再按版本串，不问段在不在册，一行装不回整次报错；迁移 0044 加部分索引；适配器在 VE 消费侧。**唤醒**：`transport-handover.registered` 与 `offsite-pickup.registered` 的扇出各另起一路、另记一本账，排在同一封信上的投影之后；交接只认转出控制的那一版；只看这一次控制事实让对象进的那一段（领域门 `EnteredByHandover`、新加的 `EnteredByPickup`），只补这一件。**同判据同键**：在控取 `ObjectsUnderControlAt`（事实发生时刻、按消费那一刻段册），派生命令与移动事实那一路是同一段代码，两路谁先谁后、重投几次都只留一份；更正链逐版补。**只补投影**：不补信号、不重问关务，补出的出发没有该件的门禁读数，补问归 ADR-0205 越权风险点 2。揽收执行口（尝试级信封）与承运商首次有效收寄立的段不在本记录（票 120）；上线前已丢的出发不自愈。越权风险点含进段欠账时补不上、补出的出发不带门禁读数、执行口与承运商段不补、每次控制事实多读段与事实、停住时挡同一分区且坏行跨对象波及
```

### ADR-0225：出发门禁读数按出发那一刻的成员展开集运单元——单元不问关务，它那一格取成员里最重的一格

处理：来向两写合并；接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-20，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」，并于 2026-10-19 同意只做分诊 A 组；票 demo-chain-gaps/42。回答 [ADR-0205](./0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md) 越权风险点 5，以及 [ADR-0206](./0206-customs-action-admission-judgement-folds-three-sources-and-judges-cross-zone-by-node-customs-zones.md) 越权风险点 8 的出发那一半；部分停用 ADR-0205 决定一、决定二各一句（被停原句与适用场景见 ADR-0205 的 Status 行），ADR-0205 与 ADR-0206、[ADR-0219](./0219-a-device-previews-the-departure-gate-before-departing-and-reads-what-the-departure-will-record.md) 正文不改，只在其 Status 行与 Links 加前向指针。越权风险点见文末，待 TF owner 复核，第 1、2 点同时待 NO owner 会签读口形状，第 9 点待 VE owner。） 决定六「列出它在同一份读数里各有一格的直接成员」由 [ADR-0237](./0237-gate-readings-show-the-executor-only-the-objects-that-did-not-pass.md) 收窄（2026-10-21）：回给执行方的单元那一格只指名同样列出的成员，放行的成员不随袋带出，越权风险点 8 随之回答；由此**部分停用**决定六一句（被停原句见索引注记），库面 `members` 照旧列全，其余各条不变。<!-- 索引注记 -->｜**已接受**（2026-10-20，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 TF owner 裁定、NO owner 会签读口形状；票 demo-chain-gaps/42）。**判种类**：段里在控的每一件先经 TF 新开的消费侧端口问节点作业是不是本租户的集运单元（判据同 ADR-0214 决定二），不是按包裹问关务；说不出（读不动、译不回）那一件记读不动、依据指名节点作业那一口，不按包裹问。**取成员**：出发那一刻的直接成员，按移入、移出事实的业务发生时间取，不取封装快照、不取此刻的容纳索引；关闭之后无成员；身份照旧由节点作业收寄册与 `JudgeIdentityBasis` 认。NO ports 新开 `ConsolidationMembershipAtView` 与只读查询（新文件，不改既有）。**单元那一格**：不问关务，取成员里最重的一格、指名成员，成员各自一格进逐对象明细；身份不成立、无成员答未决，身份读不动、成环答读不动，这几种缘由记进单元那一格的依据、不按同格筛。**展开**：逐层到包裹、不设层数上限，沿路径防环，每件只问一次、只一格，定序可重放；出发前问询同一条路。**存取与线格式**：逐对象那一格加可缺的 `members`，存量照读不迁移；交出与交付读数不收单元那一格。**信号**：阻断信号只落在问过关务的那几格，单元那一格不另形成。部分停用 ADR-0205 决定一「对象与它的格」与决定二「关务的答复译成本上下文的五格」，仅限单元那一格。交出读数不展开（票 122）、全程追踪不按成员投（票 123）、单元交接不推进成员控制（票 80）不在本记录。越权风险点含 NO 读口形状与判据第二处、成员按业务时间取受时钟偏差、身份取此刻、每件多读两口、确定性坏数据与读不动同记、依据不按同格筛、执行方看得到袋的成员、交出读数与追踪仍不展开、空单元答未决，待 TF owner 复核，NO、VE owner 会签各自那几点｜**部分停用**（2026-10-21）：越权风险点 8「执行方看得到袋的成员」由 [ADR-0237](./0237-gate-readings-show-the-executor-only-the-objects-that-did-not-pass.md) 回答——回给执行方的逐对象那一格只列没得出放行的对象，单元那一格只指名同样列出的直接成员；单元因本侧问不到的成员或空袋没得出放行时在列、`members` 为空数组。**被停条款**：决定六「回给报事实一方的逐对象那一格，在单元上带 `members`，列出它在同一份读数里各有一格的直接成员」——只列同样回给执行方的那几件。**适用场景**：只限回给报事实一方或问询方的答复；库面单元那一格的 `members` 照旧列全（决定五），其余各条不变
```

合并后：

```text
Status: Accepted（2026-10-20，Claude Code 会话代 TF owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」，并于 2026-10-19 同意只做分诊 A 组；票 demo-chain-gaps/42。回答 [ADR-0205](./0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md) 越权风险点 5，以及 [ADR-0206](./0206-customs-action-admission-judgement-folds-three-sources-and-judges-cross-zone-by-node-customs-zones.md) 越权风险点 8 的出发那一半；部分停用 ADR-0205 决定一、决定二各一句，仅限单元那一格（被停原句与适用场景见 ADR-0205 的 Status 行），ADR-0205 与 ADR-0206、[ADR-0219](./0219-a-device-previews-the-departure-gate-before-departing-and-reads-what-the-departure-will-record.md) 正文不改，只在其 Status 行与 Links 加前向指针。越权风险点见文末，待 TF owner 复核，第 1、2 点同时待 NO owner 会签读口形状，第 9 点待 VE owner。）<!-- 索引注记 -->｜**判种类**：段里在控的每一件先经 TF 新开的消费侧端口问节点作业是不是本租户的集运单元（判据同 ADR-0214 决定二），不是按包裹问关务；说不出（读不动、译不回）那一件记读不动、依据指名节点作业那一口，不按包裹问。**取成员**：出发那一刻的直接成员，按移入、移出事实的业务发生时间取，不取封装快照、不取此刻的容纳索引；关闭之后无成员；身份照旧由节点作业收寄册与 `JudgeIdentityBasis` 认。NO ports 新开 `ConsolidationMembershipAtView` 与只读查询（新文件，不改既有）。**单元那一格**：不问关务，取成员里最重的一格、指名成员，成员各自一格进逐对象明细；身份不成立、无成员答未决，身份读不动、成环答读不动，这几种缘由记进单元那一格的依据、不按同格筛。**展开**：逐层到包裹、不设层数上限，沿路径防环，每件只问一次、只一格，定序可重放；出发前问询同一条路。**存取与线格式**：逐对象那一格加可缺的 `members`，存量照读不迁移；交出与交付读数不收单元那一格。**信号**：阻断信号只落在问过关务的那几格，单元那一格不另形成。交出读数不展开（票 122）、全程追踪不按成员投（票 123）、单元交接不推进成员控制（票 80）不在本记录。越权风险点含 NO 读口形状与判据第二处、成员按业务时间取受时钟偏差、身份取此刻、每件多读两口、确定性坏数据与读不动同记、依据不按同格筛、执行方看得到袋的成员、交出读数与追踪仍不展开、空单元答未决｜**部分停用**（2026-10-21）：越权风险点 8「执行方看得到袋的成员」由 [ADR-0237](./0237-gate-readings-show-the-executor-only-the-objects-that-did-not-pass.md) 回答——回给执行方的逐对象那一格只列没得出放行的对象，单元那一格只指名同样列出的直接成员；单元因本侧问不到的成员或空袋没得出放行时在列、`members` 为空数组。**被停条款**：决定六「回给报事实一方的逐对象那一格，在单元上带 `members`，列出它在同一份读数里各有一格的直接成员」——只列同样回给执行方的那几件。**适用场景**：只限回给报事实一方或问询方的答复；库面单元那一格的 `members` 照旧列全（决定五），其余各条不变
```

### ADR-0226：同层先后到达的关务外部结果先分后续与矛盾——放行按种类偏序随业务时间只进不退，监管过程决定按业务时间成序列，其余四层一版提交一个结论；只有矛盾才标冲突，标在后到那份，存量标记不自愈

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-21，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/12，代 CC owner 裁定。回答 [ADR-0197](./0197-customs-external-results-enter-tracking-projection-per-layer.md) 越权风险点 5 的后半；ADR-0197 正文不改，只在其 Status 行与 Links 加前向指针，不停用任何已接受记录的条款。越权风险点见文末，待 CC owner 复核，第 8 点同时待 VE owner。）<!-- 索引注记 -->｜**已接受**（2026-10-21，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 CC owner 裁定；票 demo-chain-gaps/12）。**判据**：只比同一提交版本、同一范围的同层事实。放行层按种类偏序随业务时间只进不退：全部不窄于任何一种，部分与附条件各只不窄于自己；同一时刻种类不同、全部之后回退、部分与附条件之间改换是矛盾，原始语义与条件说明不进判断。监管过程决定按业务时间成序列，同一时刻异义才是矛盾。监管接收、业务受理、核定税费、处置决定一版提交一个结论，异义即矛盾。**对称与串行**：判断不看到达先后，两两不矛盾的一组按任何顺序到达都不记冲突；矛盾不传递，真矛盾时标哪一份取决于到达先后；比对含已记冲突的在册事实；比对读与落册按（租户，提交版本）以事务级咨询锁串行，并发到达也恰有一份记冲突。**当前判断**：后续事件按业务时间进入；矛盾时冲突标在后到那份，判断以它到达时在册的事实为准——CONTEXT「不得……按最后到达直接改变当前判断」只管矛盾。**范围**：今天只有申报单元一级，包含关系退化为相等。**读面**：同层一致性读回放行事实；在册的行装不回领域对象、或放行事实与层次配不上时整笔报错，不答未决。**存量**：已记的冲突标记不重判、不自愈。VE 冲突信号与 CC 冲突关系另票。越权风险点待 CC owner 复核
```

合并后：

```text
Status: Accepted（2026-10-21，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/12，代 CC owner 裁定。回答 [ADR-0197](./0197-customs-external-results-enter-tracking-projection-per-layer.md) 越权风险点 5 的后半；ADR-0197 正文不改，只在其 Status 行与 Links 加前向指针，不停用任何已接受记录的条款。越权风险点见文末，待 CC owner 复核，第 8 点同时待 VE owner。）<!-- 索引注记 -->｜**判据**：只比同一提交版本、同一范围的同层事实。放行层按种类偏序随业务时间只进不退：全部不窄于任何一种，部分与附条件各只不窄于自己；同一时刻种类不同、全部之后回退、部分与附条件之间改换是矛盾，原始语义与条件说明不进判断。监管过程决定按业务时间成序列，同一时刻异义才是矛盾。监管接收、业务受理、核定税费、处置决定一版提交一个结论，异义即矛盾。**对称与串行**：判断不看到达先后，两两不矛盾的一组按任何顺序到达都不记冲突；矛盾不传递，真矛盾时标哪一份取决于到达先后；比对含已记冲突的在册事实；比对读与落册按（租户，提交版本）以事务级咨询锁串行，并发到达也恰有一份记冲突。**当前判断**：后续事件按业务时间进入；矛盾时冲突标在后到那份，判断以它到达时在册的事实为准——CONTEXT「不得……按最后到达直接改变当前判断」只管矛盾。**范围**：今天只有申报单元一级，包含关系退化为相等。**读面**：同层一致性读回放行事实；在册的行装不回领域对象、或放行事实与层次配不上时整笔报错，不答未决。**存量**：已记的冲突标记不重判、不自愈。VE 冲突信号与 CC 冲突关系另票。
```

### ADR-0227：关务立案与申报提交经操作者渠道的运营决定口进入生产——建案、提交授权确认、申报提交执行各一种决定，首次尝试记「已登记待发送」，生产进程没有自动提交路径

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-21，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 product-strategy-boundary/18，代 CC owner 分诊（裁定见票面「分诊裁定」）并实施。不停用任何已接受记录的条款。越权风险点见文末，待 CC owner 复核；决定一的三种决定种类与授予粒度待 PC owner 在角色模型（票 product-strategy-boundary/07）里会签。）<!-- 索引注记 -->｜**已接受**（2026-10-21，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 CC owner 裁定；票 product-strategy-boundary/18）。**入口**：`POST /customs-cases`、`/customs-submission-authorizations`、`/customs-declaration-submissions` 三口挂操作者渠道「运营决定」能力面，各一种决定 `CUSTOMS_CASE_ESTABLISHMENT`、`CUSTOMS_SUBMISSION_AUTHORIZATION`、`CUSTOMS_DECLARATION_SUBMISSION`（access_identity 0018），批准与实际提交分开授予，谁持哪一种是租户取值；合成租户与真实租户同一条命令面，不加隔离放行。**建案**：包裹由请求指名，客户归属与来源资料引用由服务端按 parcel-shipment 当前已接受委托取，未接受答 `PARCEL_NOT_ACCEPTED`；要不要建仍只由建案要求规则册答，未登记答未决并指名未配置。**授权**：确认人取信封操作者，单元就绪须当前有效，授权依据写成「所凭决定种类@确认人」随提交版本快照；一单元一份，已有不顶替。**提交**：就绪与授权同时有效才固定提交版本；首次尝试记「已登记待发送」、无发送时间（customs_compliance 0025），不收调用方给的尝试结果与发送时间；越过发送边界归票 10。**没有自动提交路径**：接受扇出不建案不提交，建案、授权与提交编排在 `cmd/` 下只由运营决定口的装配文件构造。内置立案触发归票 35，授权的拒绝、撤销、再次申请与目标绑定归票 36。首发对就绪、授权「未登记」答未决并指名未配置，替代 AT-CC-105、110 在那一格的结果名。越权风险点含运维登记授权一路仍被提交执行认、授权只绑单元、组成不核案件关联、提交执行人不留痕、授权依据不带授予号、待发送只形成一半（无授权使用、无发送意图）与连接器的分工、建案四维与角色照请求收、授权使用未建模故更正补充在票 36 前不得开生产口、确认后授予撤销不连带授权失效，待 CC owner 复核，PC owner 会签决定种类
```

合并后：

```text
Status: Accepted（2026-10-21，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」；票 product-strategy-boundary/18，代 CC owner 分诊（裁定见票面「分诊裁定」）并实施。不停用任何已接受记录的条款。越权风险点见文末，待 CC owner 复核；决定一的三种决定种类与授予粒度待 PC owner 在角色模型（票 product-strategy-boundary/07）里会签。）<!-- 索引注记 -->｜**入口**：`POST /customs-cases`、`/customs-submission-authorizations`、`/customs-declaration-submissions` 三口挂操作者渠道「运营决定」能力面，各一种决定 `CUSTOMS_CASE_ESTABLISHMENT`、`CUSTOMS_SUBMISSION_AUTHORIZATION`、`CUSTOMS_DECLARATION_SUBMISSION`（access_identity 0018），批准与实际提交分开授予，谁持哪一种是租户取值；合成租户与真实租户同一条命令面，不加隔离放行。**建案**：包裹由请求指名，客户归属与来源资料引用由服务端按 parcel-shipment 当前已接受委托取，未接受答 `PARCEL_NOT_ACCEPTED`；要不要建仍只由建案要求规则册答，未登记答未决并指名未配置。**授权**：确认人取信封操作者，单元就绪须当前有效，授权依据写成「所凭决定种类@确认人」随提交版本快照；一单元一份，已有不顶替。**提交**：就绪与授权同时有效才固定提交版本；首次尝试记「已登记待发送」、无发送时间（customs_compliance 0025），不收调用方给的尝试结果与发送时间；越过发送边界归票 10。**没有自动提交路径**：接受扇出不建案不提交，建案、授权与提交编排在 `cmd/` 下只由运营决定口的装配文件构造。内置立案触发归票 35，授权的拒绝、撤销、再次申请与目标绑定归票 36。首发对就绪、授权「未登记」答未决并指名未配置，替代 AT-CC-105、110 在那一格的结果名。越权风险点含运维登记授权一路仍被提交执行认、授权只绑单元、组成不核案件关联、提交执行人不留痕、授权依据不带授予号、待发送只形成一半（无授权使用、无发送意图）与连接器的分工、建案四维与角色照请求收、授权使用未建模故更正补充在票 36 前不得开生产口、确认后授予撤销不连带授权失效
```

### ADR-0229：停下的评价请求按停因续办——请求册加停因与代次，参数登记同笔交「参数已登记」，续办另起新一代请求、回指前一代，另设运营续办口兜底

处理：来向两写合并；接受出处两写合并；补指针

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-21，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」，2026-10-20 上线就绪评估把票 demo-chain-gaps/14 归 R1 并扩大范围，裁定见 `.scratch/demo-chain-gaps/triage-2026-10-19.md`「上线就绪评估（2026-10-20）」；另一协调会话按用户 2026-10-20 原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」把 M15 并进本票、代决取第一条。部分停用 [ADR-0190](./0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 决定一、决定五各一句并回答其越权风险点 3，部分停用 [ADR-0198](./0198-sell-request-conclusions-on-the-parcel-billing-moment-path.md) 决定一一句，部分停用 [ADR-0200](./0200-buy-evaluation-forms-the-supplier-expected-cost-through-its-request.md) 决定三一句，部分停用 [ADR-0208](./0208-buy-evaluation-is-requested-from-the-charge-occurrence-formed-envelope.md) 决定三两句；被停各句原文见正文「对已接受记录的作用」，各记录正文不改。越权风险点见文末，待 SA、PP owner 复核，第 13 点同时待 PC owner。） 决定四「不记的」里「修订版不发起」一格已由 [ADR-0232](./0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) **收窄**（2026-10-21）：替代版本照常发起，答未配置时照常记进发起停办册；失效版本照旧不发起、不记（被停原句与适用场景见索引注记），其余各条不变。<!-- 索引注记 -->｜**已接受**（2026-10-21，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 SA、PP owner 裁定；票 demo-chain-gaps/14，并进 M15）。**代次**：两本评价请求册按（自然键、代次）唯一，续办出来的一代回指前一代、带续办依据，一份至多一个下一代，旧请求与旧评价不改写。**停因**：封闭集分发起之前、计价、采用三段，评价待判断按缺的是什么再分目录、序列、价表段、包裹事实与笼统五格；SELL 册加`未形成`；计价答不出评价的三格（没有在用价卡、适用冲突、输入不可得）交「评价未形成」由结算记停因、入账，移出未决名单。**发起停办**：发起之前答`未配置`与包裹无商业解析记进两本发起停办册，续办时按记下的输入重跑；SELL 停在计费时点没采用的记下那一户，重跑答`采用了别的时点`落终止。**参数已登记**：结算三本评价参数册与结算账户、计价价卡登记与序列目录复核通过同笔交信封，只续办这一类登记解得开的停因，并按信封带的收窄格取件——计价三类按受影响的计价范围与方向（序列与目录按绑了它的卡各交一面），BUY 触发册按发生项原因，SELL 计费时点采用按户；同一依据对同一条代次链至多续办一次。**运营续办口**：`parcel-settlement-register evaluation-resume` 按包裹或发生项点名。**BUY 触发册修订形**：没登费用项目的那一版可追加一版带费用项目的。部分停用 ADR-0190、0198、0200、0208 各句（含 ADR-0208「册不设修订」），原句与适用场景见正文「对已接受记录的作用」。越权风险点含停因与登记种类的对照、收窄到哪一层、要换一版卡才解得开的几格等票 150、结算政策没采用没有续办路、竞态、迁移前卡住的请求、运营续办没有查阅面与管理台入口，待 SA、PP owner 复核，结算政策那一点同时待 PC owner｜**替代版本照常发起**（2026-10-21）：发生项有效性修订驱动纠错由 [ADR-0232](./0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) 定（票 demo-chain-gaps/15）。**被停条款**：决定四「不记的」中「修订版不发起」。**适用场景**：只限替代版本——失效之后凭同一件事实重新成立的那一版照常发起，答未配置照常记停办、补登后续办；失效版本照旧不发起、不记
```

合并后：

```text
Status: Accepted（2026-10-21，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」，2026-10-20 上线就绪评估把票 demo-chain-gaps/14 归 R1 并扩大范围，裁定见 `.scratch/demo-chain-gaps/triage-2026-10-19.md`「上线就绪评估（2026-10-20）」；另一协调会话按用户 2026-10-20 原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」把 M15 并进本票、代决取第一条。部分停用 [ADR-0190](./0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md) 决定一、决定五各一句并回答其越权风险点 3，部分停用 [ADR-0198](./0198-sell-request-conclusions-on-the-parcel-billing-moment-path.md) 决定一一句，部分停用 [ADR-0200](./0200-buy-evaluation-forms-the-supplier-expected-cost-through-its-request.md) 决定三一句，部分停用 [ADR-0208](./0208-buy-evaluation-is-requested-from-the-charge-occurrence-formed-envelope.md) 决定三两句（含「册不设修订」）；被停各句原文见正文「对已接受记录的作用」，各记录正文不改。越权风险点见文末，待 SA、PP owner 复核，第 13 点同时待 PC owner。）<!-- 索引注记 -->｜**代次**：两本评价请求册按（自然键、代次）唯一，续办出来的一代回指前一代、带续办依据，一份至多一个下一代，旧请求与旧评价不改写。**停因**：封闭集分发起之前、计价、采用三段，评价待判断按缺的是什么再分目录、序列、价表段、包裹事实与笼统五格；SELL 册加`未形成`；计价答不出评价的三格（没有在用价卡、适用冲突、输入不可得）交「评价未形成」由结算记停因、入账，移出未决名单。**发起停办**：发起之前答`未配置`与包裹无商业解析记进两本发起停办册，续办时按记下的输入重跑；SELL 停在计费时点没采用的记下那一户，重跑答`采用了别的时点`落终止。**参数已登记**：结算三本评价参数册与结算账户、计价价卡登记与序列目录复核通过同笔交信封，只续办这一类登记解得开的停因，并按信封带的收窄格取件——计价三类按受影响的计价范围与方向（序列与目录按绑了它的卡各交一面），BUY 触发册按发生项原因，SELL 计费时点采用按户；同一依据对同一条代次链至多续办一次。**运营续办口**：`parcel-settlement-register evaluation-resume` 按包裹或发生项点名。**BUY 触发册修订形**：没登费用项目的那一版可追加一版带费用项目的。越权风险点含停因与登记种类的对照、收窄到哪一层、要换一版卡才解得开的几格等票 150、结算政策没采用没有续办路、竞态、迁移前卡住的请求、运营续办没有查阅面与管理台入口｜**部分停用**（2026-10-21，替代版本照常发起）：发生项有效性修订驱动纠错由 [ADR-0232](./0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) 定（票 demo-chain-gaps/15）。**被停条款**：其决定四「不记的」中「修订版不发起」。**适用场景**：只限替代版本——失效之后凭同一件事实重新成立的那一版照常发起，答未配置照常记停办、补登后续办；失效版本照旧不发起、不记｜**越权风险点 6 已答**（指针 2026-10-21 补）：越权风险点 6「要换一版卡才解得开的几格，今天续办不出评价」由 [ADR-0234](./0234-an-older-price-card-version-leaves-new-selections-only-through-an-explicit-closure-for-a-named-successor.md) 决定四回答——登同一方案的新一版、再收尾旧版，收尾那一封「参数已登记」续办出评价；无条款停用，其余各条不变
```

### ADR-0230：派送待形成留在册上，运营按册重问走同一个执行器——执行器答要求缺失同笔落册、形成了结、离场作罢；到期重问凭租户登记的间隔、未登记答未配置；补拍口改挂运营决定；随交接发起的一拍核期望交接，同段替代参与不另立任务

处理：接受出处两写合并；补指针

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-21，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」，2026-10-20 上线就绪评估把票 demo-chain-gaps/70 归 R1，裁定见 `.scratch/demo-chain-gaps/triage-2026-10-19.md`「上线就绪评估（2026-10-20）」关于 70 的那一条；代决复核（2026-10-20，用户原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」）把 M2 并进 70、把 M7 第一件与 M8 立为票 demo-chain-gaps/129 同批做。回答 [ADR-0216](./0216-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope-per-object.md) 越权风险点 1、7，以及越权风险点 2 的第一种情形；补 [ADR-0114](./0114-delivery-dispatch-is-triggered-by-entering-a-declared-delivery-segment-and-pulls-requirements-by-reference.md) 决定三「任务保持待形成」；部分停用 ADR-0216 决定四一句与 [ADR-0151](./0151-unassigned-command-faces-get-their-families.md) 决定六末句一句（被停原句与适用场景见两份的 Status 行）。三份正文不改，只在其 Status 行与 Links 加前向指针。越权风险点见文末，待 TF owner 复核，第 7、8 点同时待接入身份 owner。）<!-- 索引注记 -->｜**已接受**（2026-10-21，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 TF owner 裁定；票 demo-chain-gaps/70、129）。**册**：TF 立「派送待形成」册（迁移 0045），（租户，段，对象）一行，记链尾入场依据、缺哪几件、首次与最近一次答复时刻；只由派送发起执行器写，答`要求缺失`立或刷新、`已形成`与`已在册`了结、对象不在段里或已离场作罢，与那一拍其余写入同笔（随交接信封就是 inbox 那一笔），写不动整拍不答；同段同对象的替代参与是同一条。**查阅**：TF 查阅页 `registry=delivery-dispatch-pending` 只列本租户待形成的，与重问读同一个队列。**重问**：内置运营按册批量重问（运营决定新种类「派送重问」），全部或只问到期的，逐件交回同一个执行器、不带期望交接，所以幂等、不另立任务；一次最多 200 件。**间隔**：到期按租户登记的重问间隔算（最近一次问之后多久；登记册配置写，带登记依据，迁移 0046，最近登记的那一版生效），产品不给默认，未登记答`未配置`、一件也不问；自动驱动另票。**补拍口改族**：`/transport-fulfillment-delivery-dispatch-triggers` 从作业事实改挂运营决定「派送重问」，线格式与答复不变。**期望交接**：随交接信封的那一拍把那一版交接交给执行器，链尾不是凭它入场答`不是触发事实`（`ENTRY_NOT_THIS_HANDOVER`）；查在册沿参与替代链回看，前版已形成的任务答`已在册`。同一件的两拍由事务级形成锁排成一前一后；按册重问按最近一次问排队，答复带还剩几件；册写口的确定性拒绝归缺陷、不进未决哨兵。所有者门铃留作后继。部分停用 ADR-0216 决定四「补拍口的渠道、Intake 与答复形状都不变」的渠道与 Intake 两件、ADR-0151 决定六末句里派送发起口「与 ADR-0149 其余各条不变」那一项。越权风险点含册是台账不留答复历史、时刻按写入先后、作罢只在重问那一拍、批量上限是产品常量、补拍与重问共用一种决定、间隔语义与自动驱动、间隔没有生效区间、待确认更正为已交接在票 130 之前仍可经补拍形成任务、确定性拒绝只认三种、形成锁只排执行器那几路，待 TF owner 复核
```

合并后：

```text
Status: Accepted（2026-10-21，Claude Code 会话代 TF owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」，2026-10-20 上线就绪评估把票 demo-chain-gaps/70 归 R1，裁定见 `.scratch/demo-chain-gaps/triage-2026-10-19.md`「上线就绪评估（2026-10-20）」关于 70 的那一条；代决复核（2026-10-20，用户原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」）把 M2 并进 70、把 M7 第一件与 M8 立为票 demo-chain-gaps/129 同批做。回答 [ADR-0216](./0216-delivery-dispatch-is-triggered-in-process-from-the-handover-envelope-per-object.md) 越权风险点 1、7，以及越权风险点 2 的第一种情形；补 [ADR-0114](./0114-delivery-dispatch-is-triggered-by-entering-a-declared-delivery-segment-and-pulls-requirements-by-reference.md) 决定三「任务保持待形成」；部分停用 ADR-0216 决定四一句与 [ADR-0151](./0151-unassigned-command-faces-get-their-families.md) 决定六末句一句（被停原句与适用场景见两份的 Status 行）。三份正文不改，只在其 Status 行与 Links 加前向指针。另，决定七使 [ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定一列举里「派送任务」一项对补拍口不再适用，决定六使 ADR-0216 决定一「信封晚到时……这时此刻在场的段才是该派送的那一段」一句只剩补拍与按册重问那一路（两处指针 2026-10-21 补）。越权风险点见文末，待 TF owner 复核，第 7、8 点同时待接入身份 owner。）<!-- 索引注记 -->｜**册**：TF 立「派送待形成」册（迁移 0045），（租户，段，对象）一行，记链尾入场依据、缺哪几件、首次与最近一次答复时刻；只由派送发起执行器写，答`要求缺失`立或刷新、`已形成`与`已在册`了结、对象不在段里或已离场作罢，与那一拍其余写入同笔（随交接信封就是 inbox 那一笔），写不动整拍不答；同段同对象的替代参与是同一条。**查阅**：TF 查阅页 `registry=delivery-dispatch-pending` 只列本租户待形成的，与重问读同一个队列。**重问**：内置运营按册批量重问（运营决定新种类「派送重问」），全部或只问到期的，逐件交回同一个执行器、不带期望交接，所以幂等、不另立任务；一次最多 200 件。**间隔**：到期按租户登记的重问间隔算（最近一次问之后多久；登记册配置写，带登记依据，迁移 0046，最近登记的那一版生效），产品不给默认，未登记答`未配置`、一件也不问；自动驱动另票。**补拍口改族**：`/transport-fulfillment-delivery-dispatch-triggers` 从作业事实改挂运营决定「派送重问」，线格式与答复不变。**期望交接**：随交接信封的那一拍把那一版交接交给执行器，链尾不是凭它入场答`不是触发事实`（`ENTRY_NOT_THIS_HANDOVER`）；查在册沿参与替代链回看，前版已形成的任务答`已在册`。同一件的两拍由事务级形成锁排成一前一后；按册重问按最近一次问排队，答复带还剩几件；册写口的确定性拒绝归缺陷、不进未决哨兵。所有者门铃留作后继。越权风险点含册是台账不留答复历史、时刻按写入先后、作罢只在重问那一拍、批量上限是产品常量、补拍与重问共用一种决定、间隔语义与自动驱动、间隔没有生效区间、待确认更正为已交接在票 130 之前仍可经补拍形成任务、确定性拒绝只认三种、形成锁只排执行器那几路
```

### ADR-0231：自动形成没赶上的实际履约由门铃补形成——采用成立与委托协议换版各在自己那一笔交一封门铃，派发进程把已结束的参与交回同一个形成后半；委托协议换版是一种运营决定，委托出新一版、事实按业务时间取所凭的那一版；没形成的留痕在册

处理：来向两写合并；接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-21，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」，2026-10-20 又答「要我裁定的部分，你作为业务和系统专家，请你自决吧」；票 demo-chain-gaps/88，裁定的单一权威是 `.scratch/demo-chain-gaps/triage-2026-10-19.md`「上线就绪评估（2026-10-20）」关于 88 的那一条，票 50 评审（2026-10-21）追加的「重跑与查阅面要覆盖收尾造成的已终止、未生效两格」并进本记录。回答 [ADR-0218](./0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md) 越权风险点 1、6 与 [ADR-0233](./0233-an-older-commercial-version-is-closed-for-a-named-successor-by-a-cut-or-a-same-start-retirement.md) 越权风险点 1；**部分停用** [ADR-0204](./0204-a-fulfillment-segment-adopts-a-transport-commission-by-declaration-and-a-started-commission-cannot-be-cancelled.md) 决定一一句与决定二末条、[ADR-0207](./0207-transport-charge-occurrences-are-registered-by-an-operation-decision-and-handed-off-in-the-same-transaction.md) 文末补记一句、ADR-0218 决定七末段两项、ADR-0233 决定一的运营约束（被停原句与适用场景见「对已接受记录的作用」，四份正文不改，只在其 Status 行与 Links 加前向指针）。越权风险点见文末，待 TF owner 复核，第 1、3、15 点同时待 PC owner，第 5、8、9 点同时待 SA owner。） 决定六「状态三格」与「按身份读只读首版」两句已由 [ADR-0232](./0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) **部分停用**（2026-10-21）：未形成册多一格运营决定失效，「盖住」改读链头；越权风险点 10 由它回答（被停原句与适用场景见索引注记），其余各条不变。<!-- 索引注记 -->｜**已接受**（2026-10-21，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 TF owner 裁定；票 demo-chain-gaps/88）。**采用晚到**：采用成立的那一笔交一封自家门铃（不内联形成），派发进程把那一段上以下一次交接或有效交付结束的参与逐件交回自动形成的同一个形成后半，消费前复核采用仍在册，明确终止结束的不补；同一身份，重投与重放答已存在；一件数据缺陷（库里两份记录之间的不一致）记进未形成册（`FORMATION_DEFECT`，类别码连同完整错误）并打 error 级日志、整封照常入账，编程缺陷整封响亮失败、不进册。**委托协议换版**：运营决定新种类 `TRANSPORT_COMMISSION_AGREEMENT_REVISION`，口 `/transport-fulfillment-transport-commission-agreement-revisions`，指名同一协议对象的后继版本、起用时刻与依据；先取换版锁再读委托，后继在起用时刻按委托的服务方核，委托出新一版（第 2 版起）、旧版留历史，同一笔交换版门铃；不随 PC 新版自动改指。**取版**：事实按业务时间取委托那一刻所凭的那一版——起用不晚于业务时间的换版里最后登记的那一版，没有就是提交那一版；实际履约、订舱、取消同一判法。**换版门铃**：消费那一刻按委托列出采用它的各段逐段补形成。**实际履约未形成册**（迁移 0048）：登记答未配置、协议未生效、已终止、缺采购上下文时立或刷新一条（先问是否已有一项盖住），形成或认出已盖住了结，不计费只了结已在册的；只在形成后半一处写、与 inbox 同笔；查阅页 `registry=actual-fulfillment-unformed` 只列本租户未形成的。部分停用 ADR-0204 决定一一句与决定二末条、ADR-0207 文末补记一句、ADR-0218 决定七末段两项、ADR-0233 决定一运营约束。越权风险点含动了委托快照语义、起用时刻由人给、换版只认同一协议对象、后登记覆盖、已形成的不按新版重判、一封门铃一笔事务、不补明确终止、没有采用连线与不计费不进册、盖住按首版认、补形成不重跑订舱与取消、解除采用不作罢、收尾不再被挡、换版门铃上一段读不回整封失败｜**运营决定失效**（2026-10-21）：发生项失效入口由 [ADR-0232](./0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) 立。**被停条款**：决定六「状态三格：未形成、已形成（记盖住它的那一项发生项）、不计费（记作答的那一版快照）。」与「按身份读只读首版，与登记用例的重放同一个读法。」**适用场景**：前一句多一格 `INVALIDATED_BY_DECISION`（链头是运营决定失效的那一版，记失效版本），其余三格照旧；后一句改为按身份读链头、链头已失效不算盖住
```

合并后：

```text
Status: Accepted（2026-10-21，Claude Code 会话代 TF owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」，2026-10-20 又答「要我裁定的部分，你作为业务和系统专家，请你自决吧」；票 demo-chain-gaps/88，裁定的单一权威是 `.scratch/demo-chain-gaps/triage-2026-10-19.md`「上线就绪评估（2026-10-20）」关于 88 的那一条，票 50 评审（2026-10-21）追加的「重跑与查阅面要覆盖收尾造成的已终止、未生效两格」并进本记录。回答 [ADR-0218](./0218-actual-fulfillment-occurrences-form-from-handover-and-delivery-envelopes.md) 越权风险点 1、6 与 [ADR-0233](./0233-an-older-commercial-version-is-closed-for-a-named-successor-by-a-cut-or-a-same-start-retirement.md) 越权风险点 1；**部分停用** [ADR-0204](./0204-a-fulfillment-segment-adopts-a-transport-commission-by-declaration-and-a-started-commission-cannot-be-cancelled.md) 决定一一句与决定二末条、[ADR-0207](./0207-transport-charge-occurrences-are-registered-by-an-operation-decision-and-handed-off-in-the-same-transaction.md) 文末补记一句、ADR-0218 决定七末段两项、ADR-0233 决定一的运营约束（被停原句与适用场景见「对已接受记录的作用」，四份正文不改，只在其 Status 行与 Links 加前向指针）。越权风险点见文末，待 TF owner 复核，第 1、3、15 点同时待 PC owner，第 5、8、9 点同时待 SA owner。）<!-- 索引注记 -->｜**采用晚到**：采用成立的那一笔交一封自家门铃（不内联形成），派发进程把那一段上以下一次交接或有效交付结束的参与逐件交回自动形成的同一个形成后半，消费前复核采用仍在册，明确终止结束的不补；同一身份，重投与重放答已存在；一件数据缺陷（库里两份记录之间的不一致）记进未形成册（`FORMATION_DEFECT`，类别码连同完整错误）并打 error 级日志、整封照常入账，编程缺陷整封响亮失败、不进册。**委托协议换版**：运营决定新种类 `TRANSPORT_COMMISSION_AGREEMENT_REVISION`，口 `/transport-fulfillment-transport-commission-agreement-revisions`，指名同一协议对象的后继版本、起用时刻与依据；先取换版锁再读委托，后继在起用时刻按委托的服务方核，委托出新一版（第 2 版起）、旧版留历史，同一笔交换版门铃；不随 PC 新版自动改指。**取版**：事实按业务时间取委托那一刻所凭的那一版——起用不晚于业务时间的换版里最后登记的那一版，没有就是提交那一版；实际履约、订舱、取消同一判法。**换版门铃**：消费那一刻按委托列出采用它的各段逐段补形成。**实际履约未形成册**（迁移 0048）：登记答未配置、协议未生效、已终止、缺采购上下文时立或刷新一条（先问是否已有一项盖住），形成或认出已盖住了结，不计费只了结已在册的；只在形成后半一处写、与 inbox 同笔；查阅页 `registry=actual-fulfillment-unformed` 只列本租户未形成的。越权风险点含动了委托快照语义、起用时刻由人给、换版只认同一协议对象、后登记覆盖、已形成的不按新版重判、一封门铃一笔事务、不补明确终止、没有采用连线与不计费不进册、盖住按首版认、补形成不重跑订舱与取消、解除采用不作罢、收尾不再被挡、换版门铃上一段读不回整封失败｜**部分停用**（2026-10-21，运营决定失效）：发生项失效入口由 [ADR-0232](./0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md) 立，越权风险点 10 由它回答。**被停条款**：其决定六「状态三格：未形成、已形成（记盖住它的那一项发生项）、不计费（记作答的那一版快照）。」与「按身份读只读首版，与登记用例的重放同一个读法。」**适用场景**：前一句多一格 `INVALIDATED_BY_DECISION`（链头是运营决定失效的那一版，记失效版本），其余三格照旧；后一句改为按身份读链头、链头已失效不算盖住
```

### ADR-0232：解除履约段采用是一种运营决定，只在段上还没有按那份委托形成仍有效的发生项时允许，委托的开始随之撤回或按其余段重新派生；发生项失效经运营决定进生产，失效之后同一件事实再登从失效版本长出替代版本；结算按失效撤下、按替代版本的新评价重述供应商预期成本

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-21，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」，2026-10-20 又答「要我裁定的部分，你作为业务和系统专家，请你自决吧」；票 demo-chain-gaps/57、15。裁定的单一权威是 `.scratch/demo-chain-gaps/triage-2026-10-19.md`「上线就绪评估（2026-10-20）」关于 57 的那一条与末节「代决复核追加的上线前必修（2026-10-20）」，以及代决复核报告 `.scratch/owner-review-20261020/report.md` 的 R10 与 M9。部分停用 [ADR-0204](./0204-a-fulfillment-segment-adopts-a-transport-commission-by-declaration-and-a-started-commission-cannot-be-cancelled.md) 决定一末两句与决定二「只前移、不后移」、[ADR-0208](./0208-buy-evaluation-is-requested-from-the-charge-occurrence-formed-envelope.md) 决定五「修订版不发起」一句、[ADR-0200](./0200-buy-evaluation-forms-the-supplier-expected-cost-through-its-request.md) 决定三「同一发生项、费用项目与规则版本已有另一个标识的首版时，同样入账」一句，部分停用 [ADR-0231](./0231-actual-fulfillment-left-unformed-is-caught-up-by-doorbells-and-a-commission-agreement-revision.md) 决定六两句（未形成册的状态集与「盖住」只读首版），并随 ADR-0208 那一句收窄 [ADR-0229](./0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 决定四「不记的」里「修订版不发起」一格；被停原句与适用场景见「对已接受记录的作用」，五份正文不改，只在其 Status 行与 Links 加前向指针。回答 ADR-0204 越权风险点 2、[ADR-0207](./0207-transport-charge-occurrences-are-registered-by-an-operation-decision-and-handed-off-in-the-same-transaction.md) Consequences「已知限制」与文末补记越权风险点 11、15、ADR-0200 越权风险点 4。越权风险点见文末，待 TF owner 复核，第 10 至 13 点与第 15 点同时待 SA owner，第 14 点 access-identity owner 知会。）<!-- 索引注记 -->｜**已接受**（2026-10-21，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 TF、SA owner 裁定；票 demo-chain-gaps/57、15）。**解除采用**：运营决定「解除履约段采用」，载荷（段、委托、代次），段上还有按这份委托形成（服务方同、协议快照是委托某一版的）、仍有效的实际履约发生项答 `OCCURRENCES_FORMED` 并回显、续办是先失效；解除另记一行，采用那一行不改，同段可以再采用（下一代，请求点名接在哪一代之后）；解除与再采用都按代次认重放。**开始**：委托再无别的已采用段时撤回到已提交（又取消得了），还有时按其余段采用上记的开始取最早的那一条（可以后移），不是开始所依的段时不动（同刻并列按依据与时刻认开始所依的那一条）；之前、变法与之后随解除留下。**发生项失效**：运营决定「运输收费发生项失效」，点名链头那一版与一条依据，长出 V(n+1) 失效版本、记修订者，已形成信封同笔。**再登**：登记用例改读链头，链头已失效时凭这一次的判断从失效版本长出替代版本；自动形成（含补形成）遇已失效链头答已存在、不复活（未形成册了结为运营决定失效），除非这一段的当前采用晚于那一次失效。**并发**：解除与登记共用发生项范围锁、登记在锁下重读采用；采用与解除共用委托的开始锁。**结算**：失效版本的信封另走一路，在发生项锁下记下那一版已失效、给依它形成的链头各长出一版失效纠错版本（计价结果原样、标不再成立）；替代版本照常发起 BUY 评价，评价形成时只接在失效纠错版本后面重述全额（失效还没记到结算时答未决、等撤下那一路），纠错由发生项新版本触发、金额同不同都追加；晚于失效到的、依已失效那一版的评价不形成成本；账单一行点名已撤下或已有后继的版本答 `EXPECTED_COST_NOT_CURRENT`、不进匹配。部分停用 ADR-0204 决定一末两句与决定二「只前移」、ADR-0208 决定五一句、ADR-0200 决定三一句、ADR-0231 决定六两句，收窄 ADR-0229 决定四一格。其余三问拆成票 demo-chain-gaps/162–164。越权风险点含首个参与更正不重新派生、已答出的「运输已开始」随撤回过时、解除与再采用按代次认重放的线格式代价、失效不可撤回、失效纠错版本只有列表与账单匹配认它、替代评价换结算币响亮失败、替代评价等撤下那一路、与票 14 代次的对齐
```

合并后：

```text
Status: Accepted（2026-10-21，Claude Code 会话代 TF、SA owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」，2026-10-20 又答「要我裁定的部分，你作为业务和系统专家，请你自决吧」；票 demo-chain-gaps/57、15。裁定的单一权威是 `.scratch/demo-chain-gaps/triage-2026-10-19.md`「上线就绪评估（2026-10-20）」关于 57 的那一条与末节「代决复核追加的上线前必修（2026-10-20）」，以及代决复核报告 `.scratch/owner-review-20261020/report.md` 的 R10 与 M9。部分停用 [ADR-0204](./0204-a-fulfillment-segment-adopts-a-transport-commission-by-declaration-and-a-started-commission-cannot-be-cancelled.md) 决定一末两句与决定二「只前移、不后移」、[ADR-0208](./0208-buy-evaluation-is-requested-from-the-charge-occurrence-formed-envelope.md) 决定五「修订版不发起」一句、[ADR-0200](./0200-buy-evaluation-forms-the-supplier-expected-cost-through-its-request.md) 决定三「同一发生项、费用项目与规则版本已有另一个标识的首版时，同样入账」一句，部分停用 [ADR-0231](./0231-actual-fulfillment-left-unformed-is-caught-up-by-doorbells-and-a-commission-agreement-revision.md) 决定六两句（未形成册的状态集与「盖住」只读首版），并随 ADR-0208 那一句收窄 [ADR-0229](./0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 决定四「不记的」里「修订版不发起」一格；被停原句与适用场景见「对已接受记录的作用」，五份正文不改，只在其 Status 行与 Links 加前向指针。回答 ADR-0204 越权风险点 2、[ADR-0207](./0207-transport-charge-occurrences-are-registered-by-an-operation-decision-and-handed-off-in-the-same-transaction.md) Consequences「已知限制」与文末补记越权风险点 11、15、ADR-0200 越权风险点 4。越权风险点见文末，待 TF owner 复核，第 10 至 13 点与第 15 点同时待 SA owner，第 14 点 access-identity owner 知会。）<!-- 索引注记 -->｜**解除采用**：运营决定「解除履约段采用」，载荷（段、委托、代次），段上还有按这份委托形成（服务方同、协议快照是委托某一版的）、仍有效的实际履约发生项答 `OCCURRENCES_FORMED` 并回显、续办是先失效；解除另记一行，采用那一行不改，同段可以再采用（下一代，请求点名接在哪一代之后）；解除与再采用都按代次认重放。**开始**：委托再无别的已采用段时撤回到已提交（又取消得了），还有时按其余段采用上记的开始取最早的那一条（可以后移），不是开始所依的段时不动（同刻并列按依据与时刻认开始所依的那一条）；之前、变法与之后随解除留下。**发生项失效**：运营决定「运输收费发生项失效」，点名链头那一版与一条依据，长出 V(n+1) 失效版本、记修订者，已形成信封同笔。**再登**：登记用例改读链头，链头已失效时凭这一次的判断从失效版本长出替代版本；自动形成（含补形成）遇已失效链头答已存在、不复活（未形成册了结为运营决定失效），除非这一段的当前采用晚于那一次失效。**并发**：解除与登记共用发生项范围锁、登记在锁下重读采用；采用与解除共用委托的开始锁。**结算**：失效版本的信封另走一路，在发生项锁下记下那一版已失效、给依它形成的链头各长出一版失效纠错版本（计价结果原样、标不再成立）；替代版本照常发起 BUY 评价，评价形成时只接在失效纠错版本后面重述全额（失效还没记到结算时答未决、等撤下那一路），纠错由发生项新版本触发、金额同不同都追加；晚于失效到的、依已失效那一版的评价不形成成本；账单一行点名已撤下或已有后继的版本答 `EXPECTED_COST_NOT_CURRENT`、不进匹配。其余三问拆成票 demo-chain-gaps/162–164。越权风险点含首个参与更正不重新派生、已答出的「运输已开始」随撤回过时、解除与再采用按代次认重放的线格式代价、失效不可撤回、失效纠错版本只有列表与账单匹配认它、替代评价换结算币响亮失败、替代评价等撤下那一路、与票 14 代次的对齐
```

### ADR-0233：同一对象换版时旧版只经显式收尾离开新的选择——后继晚于旧版起点按后继生效时点截止，同起点重发按决定引用退役；两种都指名同一对象的后继，新版发布本身不收尾旧版

处理：来向两写合并；接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-21，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」，2026-10-20 又答「要我裁定的部分，你作为业务和系统专家，请你自决吧」；票 demo-chain-gaps/50。出处是 [ADR-0209](./0209-supplier-agreements-declare-charge-occurrence-conditions-and-registration-judges-by-them.md) 越权风险点 5 与 [ADR-0202](./0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md) 越权风险点 5；上线就绪评估（`.scratch/demo-chain-gaps/triage-2026-10-19.md`「上线就绪评估（2026-10-20）」）把 50 归上线前必修、只做截止，代决复核（main 本地提交 `d6c7902f` 的 `.scratch/owner-review-20261020/report.md` 的 M16、M17）把点读口折入更正与同起点退役并进本票，退役写口所需的本记录随之提前。不停用任何已接受记录的条款。越权风险点见文末，待 PC owner 复核，决定五与第 1、2 点同时待 TF owner 会签。）决定一的运营约束「票 88 落地前，不得截止或退役仍被在途运输委托指名的供应商协议版本」已由 [ADR-0231](./0231-actual-fulfillment-left-unformed-is-caught-up-by-doorbells-and-a-commission-agreement-revision.md) 停用（2026-10-21），越权风险点 1 随之回答，适用场景见索引注记。<!-- 索引注记 -->｜**已接受**（2026-10-21，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 PC owner 裁定、TF owner 会签决定五；票 demo-chain-gaps/50）。**收尾两种**：后继晚于旧版起点的，旧版截止在后继此刻的选用起点（一条区间更正，ADR-0038、0056）；同起点重发的，旧版按决定引用退役（另册，版本快照不改写，退役时刻取登记时刻）。**共同门**：同一对象、同一范围、两版都已生效、后继接得住旧版此后的那一段（壳与自带区间的政策正文）；先锁旧版行再读整册判。**入口**：登记册配置写两口 `/commercial-version-successor-cuts`、`/commercial-version-retirements`，受控 CLI `register-version-closures`。**读法**：已退役不参选；价格、结算、信用政策正文随壳的退役落选、被壳上最新一条更正的终点收窄；供应商协议版本点读交回已退役的壳，正文点读与版本点读把最新更正的终点折进判用区间；内部成本政策点读折进退役；管理台目录认退役。完整替代写口与不指名后继的一般退役归后继票。**运营约束**：票 88 落地前，不得截止或退役仍被在途运输委托指名的供应商协议版本（越权风险点 1）。｜**运营约束解除**（2026-10-21）：决定一「票 88 落地前，不得截止或退役仍被在途运输委托指名的供应商协议版本——那会让这些委托此后的成本静默不形成（越权风险点 1）」已由 [ADR-0231](./0231-actual-fulfillment-left-unformed-is-caught-up-by-doorbells-and-a-commission-agreement-revision.md) 停用——收尾造成的已终止、未生效两格此后进实际履约未形成册、查阅页列出，承接口是委托协议换版与换版门铃的补形成。**适用场景**：全部；收尾入口仍不反查在途委托，承接是事后的；其余各条不变
```

合并后：

```text
Status: Accepted（2026-10-21，Claude Code 会话代 PC owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」，2026-10-20 又答「要我裁定的部分，你作为业务和系统专家，请你自决吧」；票 demo-chain-gaps/50。出处是 [ADR-0209](./0209-supplier-agreements-declare-charge-occurrence-conditions-and-registration-judges-by-them.md) 越权风险点 5 与 [ADR-0202](./0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md) 越权风险点 5；上线就绪评估（`.scratch/demo-chain-gaps/triage-2026-10-19.md`「上线就绪评估（2026-10-20）」）把 50 归上线前必修、只做截止，代决复核（main 本地提交 `d6c7902f` 的 `.scratch/owner-review-20261020/report.md` 的 M16、M17）把点读口折入更正与同起点退役并进本票，退役写口所需的本记录随之提前。不停用任何已接受记录的条款。越权风险点见文末，待 PC owner 复核，决定五与第 1、2 点同时待 TF owner 会签。）<!-- 索引注记 -->｜**收尾两种**：后继晚于旧版起点的，旧版截止在后继此刻的选用起点（一条区间更正，ADR-0038、0056）；同起点重发的，旧版按决定引用退役（另册，版本快照不改写，退役时刻取登记时刻）。**共同门**：同一对象、同一范围、两版都已生效、后继接得住旧版此后的那一段（壳与自带区间的政策正文）；先锁旧版行再读整册判。**入口**：登记册配置写两口 `/commercial-version-successor-cuts`、`/commercial-version-retirements`，受控 CLI `register-version-closures`。**读法**：已退役不参选；价格、结算、信用政策正文随壳的退役落选、被壳上最新一条更正的终点收窄；供应商协议版本点读交回已退役的壳，正文点读与版本点读把最新更正的终点折进判用区间；内部成本政策点读折进退役；管理台目录认退役。完整替代写口与不指名后继的一般退役归后继票。**运营约束**：票 88 落地前，不得截止或退役仍被在途运输委托指名的供应商协议版本（越权风险点 1）。｜**部分停用**（2026-10-21，运营约束解除）：其决定一「票 88 落地前，不得截止或退役仍被在途运输委托指名的供应商协议版本——那会让这些委托此后的成本静默不形成（越权风险点 1）」已由 [ADR-0231](./0231-actual-fulfillment-left-unformed-is-caught-up-by-doorbells-and-a-commission-agreement-revision.md) 停用——收尾造成的已终止、未生效两格此后进实际履约未形成册、查阅页列出，承接口是委托协议换版与换版门铃的补形成，越权风险点 1 随之回答。**适用场景**：全部；收尾入口仍不反查在途委托，承接是事后的；其余各条不变
```

### ADR-0234：价卡换版时旧版只经显式收尾离开新的选择——后继晚于旧版起点按后继起点截止，后继不晚于旧版起点整段让位退役；两种都指名同一定价方案的后继，收尾同笔交「参数已登记」，新版登记本身不收尾旧版

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-21，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」，2026-10-20 又答「要我裁定的部分，你作为业务和系统专家，请你自决吧」；票 demo-chain-gaps/150，协调方 2026-10-21 分诊升为上线前必修，见 `.scratch/demo-chain-gaps/triage-2026-10-19.md`「票 14 落地时追加的上线前必修（2026-10-21）」。出处是 [ADR-0229](./0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 越权风险点 6，形照 [ADR-0233](./0233-an-older-commercial-version-is-closed-for-a-named-successor-by-a-cut-or-a-same-start-retirement.md)。票面「要裁的」两问由本记录代 PP owner 裁（决定一、决定六）。不停用任何已接受记录的条款。越权风险点见文末，待 PP owner 复核，第 3、4 点同时待 SA owner，第 6 点同时待 network-routing owner。）<!-- 索引注记 -->｜**已接受**（2026-10-21，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 PP owner 裁定；票 demo-chain-gaps/150）。**收尾两种**：后继晚于旧版起点的，旧版此刻的选用截止在后继起点（截止点服务端取）；后继不晚于旧版起点的，旧版整段让位退役（退役时刻取登记时刻）；两版必须同一定价方案、同一计价范围、方向与目的，后继接得住旧版此后的那一段；先锁旧版行再读两版判。截止与退役各另册只增，版本行一字不动。**入口**：登记册配置写两口 `/pricing-price-card-successor-cuts`、`/pricing-price-card-retirements`，受控登记口 `parcel-pricing-register -kind price-card-cut|price-card-retirement`。**读法**：按范围选卡两口（在用解析、适用装载）同一口径折进收尾，重放点读不折，绑定读口不算已退役的版本，管理台价卡目录回显截止点、退役时刻与后继。**续办**：收尾同笔交价卡种类的「参数已登记」，标识与两版各自登记那一封分开——后继登记那一封续办出来、在收尾之前被问到的那一代停在适用冲突，能续办到后继上的是收尾这一封；ADR-0229 越权风险点 5 的竞态对收尾那一封同样适用。**已在旧版下形成的评价**不标注、不改写。让位给另一定价方案与不指名后继的退役归票 168，管理台表单归票 169。**运营约束**：收尾一版卡之后，指名它的线路版本要另发一版改指后继，直到票 173 落地（越权风险点 6）
```

合并后：

```text
Status: Accepted（2026-10-21，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」，2026-10-20 又答「要我裁定的部分，你作为业务和系统专家，请你自决吧」；票 demo-chain-gaps/150，协调方 2026-10-21 分诊升为上线前必修，见 `.scratch/demo-chain-gaps/triage-2026-10-19.md`「票 14 落地时追加的上线前必修（2026-10-21）」。出处是 [ADR-0229](./0229-a-stalled-evaluation-request-resumes-as-a-new-generation-on-parameter-registration.md) 越权风险点 6，形照 [ADR-0233](./0233-an-older-commercial-version-is-closed-for-a-named-successor-by-a-cut-or-a-same-start-retirement.md)。票面「要裁的」两问由本记录代 PP owner 裁（决定一、决定六）。不停用任何已接受记录的条款。越权风险点见文末，待 PP owner 复核，第 3、4 点同时待 SA owner，第 6 点同时待 network-routing owner。）<!-- 索引注记 -->｜**收尾两种**：后继晚于旧版起点的，旧版此刻的选用截止在后继起点（截止点服务端取）；后继不晚于旧版起点的，旧版整段让位退役（退役时刻取登记时刻）；两版必须同一定价方案、同一计价范围、方向与目的，后继接得住旧版此后的那一段；先锁旧版行再读两版判。截止与退役各另册只增，版本行一字不动。**入口**：登记册配置写两口 `/pricing-price-card-successor-cuts`、`/pricing-price-card-retirements`，受控登记口 `parcel-pricing-register -kind price-card-cut|price-card-retirement`。**读法**：按范围选卡两口（在用解析、适用装载）同一口径折进收尾，重放点读不折，绑定读口不算已退役的版本，管理台价卡目录回显截止点、退役时刻与后继。**续办**：收尾同笔交价卡种类的「参数已登记」，标识与两版各自登记那一封分开——后继登记那一封续办出来、在收尾之前被问到的那一代停在适用冲突，能续办到后继上的是收尾这一封；ADR-0229 越权风险点 5 的竞态对收尾那一封同样适用。**已在旧版下形成的评价**不标注、不改写。让位给另一定价方案与不指名后继的退役归票 168，管理台表单归票 169。**运营约束**：收尾一版卡之后，指名它的线路版本要另发一版改指后继，直到票 173 落地（越权风险点 6）
```

### ADR-0235：路由复核按恢复动作与业务时间分格——没有路由历史、证据未配置、冻结形态未配置三格入账未判、留下原因，不再重投；业务时间早于当前位置的触发入账为「触发早于当前位置」，不判

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-21，Claude Code 会话代决：用户原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」与「你作为业务和系统专家自决……解决所有的问题」；票 [demo-chain-gaps/131](../../.scratch/demo-chain-gaps/issues/131-network-reassessment-undecided-is-split-by-reason.md)、[demo-chain-gaps/132](../../.scratch/demo-chain-gaps/issues/132-network-reassessment-ignores-triggers-older-than-the-current-position.md)，单一权威为代决复核报告 [owner-review-20261020](../../.scratch/owner-review-20261020/report.md) 的 M10、M11。回答 [ADR-0215](./0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md) 越权风险点 9、10、11，并部分停用其决定四「重投」一条里的未决范围与决定三「更正之后仍是`已交接`」一句（限更正一次业务时间早于当前位置的交接），被停原句与适用场景见 Consequences；ADR-0215 正文不改，只在其 Status 行与 Links 加前向指针。越权风险点见文末，待 NR owner 复核，更正那一格（越权风险点 11）同时待 TF owner 会签。）<!-- 索引注记 -->｜**已接受**（2026-10-21，用户原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」，Claude Code 会话代 NR owner 裁定；票 demo-chain-gaps/131、132）。**分格**：复核未决只留重投推得动的（复核登记册或路由历史读不回、证据读不动、适用性看不清、取号依赖故障、未判写不进），照旧回滚重投；没有路由历史、证据未配置、冻结形态未配置答`未判`、入账，判据只在复核编排一处，收寄采用与交接两路同改。**留原因**：NR 未判触发登记（network_routing 0021）一行记关联、判断键、原因、触发业务时间，原因没落库不得入账；不是复核结论。**另一封信封**：携带同一触发的另一封信封再来时重新判，前提齐了就判；同一封信封入账后由消费门跳过，今天没有续办手段。**业务时间**：复核记下所判触发的业务时间（0022），读最近一次已判控制触发按判断键取咨询锁串行；触发严格早于它时入账为「触发早于当前位置」、记比较所用的时刻，不判；同一时刻照判；交接更正承继原判断时刻，更正当前位置那一次照判、更正更早的那一次入账不判；已判触发由另一封信封再带来时读回原结果。冻结形态未配置只收真的未声明，冻结事实不成立与前缀立不起来响亮报错。续办触发不在本记录（票 154，与票 14 同族），初始路由一路同病另立票 155。部分停用 ADR-0215 决定四「复核停在未决（复核登记册或路由历史读不回、证据未配置、适用性看不清、取号依赖故障）」一句里证据未配置与随之同归未决的没有路由历史、冻结形态未配置三格，**适用场景**：NR 路由复核的收寄采用与交接两路；另部分停用 ADR-0215 决定三「更正之后仍是`已交接`：新版本承继接收节点，另成一次复核，原复核不被覆盖」一句，**适用场景**：更正的那次交接业务时间早于路由历史里最近一次已判控制触发，这时更正入账为「触发早于当前位置」、不判，更正当前位置那一次照判；其余各条不变。越权风险点含未配置三格入账后没有续办手段、没有路由历史可能自愈、原因落登记不落日志、未判只留第一次的原因、当前位置只认已判复核、同一时刻照判、迁移前的复核不参与比较、较早那次不进复核册、判断键串行的代价、业务时间取自来源、更正一次早于当前位置的交接不再复核、装配缺陷响亮失败照样占着分区，待 NR owner 复核，TF owner 会签更正那一格
```

合并后：

```text
Status: Accepted（2026-10-21，Claude Code 会话代决：用户原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」与「你作为业务和系统专家自决……解决所有的问题」；票 [demo-chain-gaps/131](../../.scratch/demo-chain-gaps/issues/131-network-reassessment-undecided-is-split-by-reason.md)、[demo-chain-gaps/132](../../.scratch/demo-chain-gaps/issues/132-network-reassessment-ignores-triggers-older-than-the-current-position.md)，单一权威为代决复核报告 [owner-review-20261020](../../.scratch/owner-review-20261020/report.md) 的 M10、M11。回答 [ADR-0215](./0215-a-handover-into-a-node-triggers-route-reassessment-and-reads-the-baseline-geo-projection.md) 越权风险点 9、10、11，并部分停用其决定四「重投」一条里的未决范围与决定三「更正之后仍是`已交接`」一句（限更正一次业务时间早于当前位置的交接），被停原句与适用场景见 Consequences；ADR-0215 正文不改，只在其 Status 行与 Links 加前向指针。越权风险点见文末，待 NR owner 复核，更正那一格（越权风险点 11）同时待 TF owner 会签。）<!-- 索引注记 -->｜**分格**：复核未决只留重投推得动的（复核登记册或路由历史读不回、证据读不动、适用性看不清、取号依赖故障、未判写不进），照旧回滚重投；没有路由历史、证据未配置、冻结形态未配置答`未判`、入账，判据只在复核编排一处，收寄采用与交接两路同改。**留原因**：NR 未判触发登记（network_routing 0021）一行记关联、判断键、原因、触发业务时间，原因没落库不得入账；不是复核结论。**另一封信封**：携带同一触发的另一封信封再来时重新判，前提齐了就判；同一封信封入账后由消费门跳过，今天没有续办手段。**业务时间**：复核记下所判触发的业务时间（0022），读最近一次已判控制触发按判断键取咨询锁串行；触发严格早于它时入账为「触发早于当前位置」、记比较所用的时刻，不判；同一时刻照判；交接更正承继原判断时刻，更正当前位置那一次照判、更正更早的那一次入账不判；已判触发由另一封信封再带来时读回原结果。冻结形态未配置只收真的未声明，冻结事实不成立与前缀立不起来响亮报错。续办触发不在本记录（票 154，与票 14 同族），初始路由一路同病另立票 155。越权风险点含未配置三格入账后没有续办手段、没有路由历史可能自愈、原因落登记不落日志、未判只留第一次的原因、当前位置只认已判复核、同一时刻照判、迁移前的复核不参与比较、较早那次不进复核册、判断键串行的代价、业务时间取自来源、更正一次早于当前位置的交接不再复核、装配缺陷响亮失败照样占着分区
```

### ADR-0236：租户财务、关务人员在管理台手工采用外部资金事实与监管凭证——操作者渠道「运营决定」能力面另开三个手工口，两类各一种决定、提交人记成操作证据，与集成客户端族并存

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-21，Claude Code 会话代决：用户原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」与「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/140，落地代决复核报告 R1 那一格的裁定「是」。**部分停用** ADR-0149 决定一、决定三各一句与 ADR-0151 决定四一句，见决定六。越权风险点见文末，待 owner 复核；两种决定种类与授予粒度待 PC owner 在角色模型（票 product-strategy-boundary/07）里会签。）<!-- 索引注记 -->｜**已接受**（2026-10-21，用户原话「你作为业务和系统专家自决」，Claude Code 会话代决；票 demo-chain-gaps/140，承代决复核 R1）。**入口**：`POST /settlement-external-funds-fact-manual-registrations`、`/settlement-external-funds-fact-correction-manual-registrations`、`/customs-regulatory-credential-manual-registrations` 三口挂操作者渠道「运营决定」能力面，两种决定 `EXTERNAL_FUNDS_FACT_ADOPTION`（首版与更正同一种）、`REGULATORY_CREDENTIAL_REGISTRATION`（access_identity 0020），谁持哪一种是租户取值，未授予答 `OPERATOR_NOT_GRANTED`；铸信封前判准入（能力 `OPERATION_DECISION`、事实类型取决定种类）。**编排与载荷**与集成客户端族那三口同一份，两族只差 Intake；集成客户端三口不改、并存。**操作证据**：提交人记成 `OPERATOR/<发行方>#<sub>` 落在采用的那一版资金事实（settlement_accounting 0041）与凭证行（customs_compliance 0026）上，只进审计、不进内容比对与任何门，重放不顶替；集成客户端与受控 CLI 两路记 NULL；两本册的读面透出。**更正不绑原采用族**：两族在同一事实键上顺序到达答 `FUNDS_FACT_CONFLICT` 或`未受理`，外部系统一侧转人工；并发那一格的缺陷归票 demo-chain-gaps/125。管理台收付款核销页两张、关务案件页一张登记签改走手工口。部分停用 ADR-0149 决定一「外部结果与外部资金事实……的提交者是外部系统」与决定三对外部资金事实、监管凭证两类的独占，ADR-0151 决定四「外部资金事实更正随原事实归集成客户端族」，适用场景只限三手工口。越权风险点含 R1 本身（owner 答否即撤三张签与三口）、首版与更正同一种授予、更正不绑族且职责分离不强制（持采用授予的人能更正银行接口送来的金额，UC-SA-005「结算人员不能更改银行事实」，去向票 product-strategy-boundary/07）、重放不留第二人、集成客户端一路不记提交方、管理台仍以 JSON 录入、凭证挂运营决定而非登记写面、两族准入是两套坐标，待 owner 复核
```

合并后：

```text
Status: Accepted（2026-10-21，Claude Code 会话代决：用户原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」与「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/140，落地代决复核报告 R1 那一格的裁定「是」。**部分停用** ADR-0149 决定一、决定三各一句与 ADR-0151 决定四一句，适用场景只限三个手工口，见决定六。越权风险点见文末，待 owner 复核；两种决定种类与授予粒度待 PC owner 在角色模型（票 product-strategy-boundary/07）里会签。）<!-- 索引注记 -->｜**入口**：`POST /settlement-external-funds-fact-manual-registrations`、`/settlement-external-funds-fact-correction-manual-registrations`、`/customs-regulatory-credential-manual-registrations` 三口挂操作者渠道「运营决定」能力面，两种决定 `EXTERNAL_FUNDS_FACT_ADOPTION`（首版与更正同一种）、`REGULATORY_CREDENTIAL_REGISTRATION`（access_identity 0020），谁持哪一种是租户取值，未授予答 `OPERATOR_NOT_GRANTED`；铸信封前判准入（能力 `OPERATION_DECISION`、事实类型取决定种类）。**编排与载荷**与集成客户端族那三口同一份，两族只差 Intake；集成客户端三口不改、并存。**操作证据**：提交人记成 `OPERATOR/<发行方>#<sub>` 落在采用的那一版资金事实（settlement_accounting 0041）与凭证行（customs_compliance 0026）上，只进审计、不进内容比对与任何门，重放不顶替；集成客户端与受控 CLI 两路记 NULL；两本册的读面透出。**更正不绑原采用族**：两族在同一事实键上顺序到达答 `FUNDS_FACT_CONFLICT` 或`未受理`，外部系统一侧转人工；并发那一格的缺陷归票 demo-chain-gaps/125。管理台收付款核销页两张、关务案件页一张登记签改走手工口。越权风险点含 R1 本身（owner 答否即撤三张签与三口）、首版与更正同一种授予、更正不绑族且职责分离不强制（持采用授予的人能更正银行接口送来的金额，UC-SA-005「结算人员不能更改银行事实」，去向票 product-strategy-boundary/07）、重放不留第二人、集成客户端一路不记提交方、管理台仍以 JSON 录入、凭证挂运营决定而非登记写面、两族准入是两套坐标
```

### ADR-0237：门禁读数回给执行方的逐对象那一格只列没得出放行的对象——各口共用一处判法，落册照旧记全；出发意图的起讫与声明的段绑定

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-21，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」，2026-10-20 又答「要我裁定的部分，你作为业务和系统专家，请你自决吧」；票 demo-chain-gaps/146，裁定见代决复核报告 `.scratch/owner-review-20261020/report.md` 的 R11。回答 [ADR-0219](./0219-a-device-previews-the-departure-gate-before-departing-and-reads-what-the-departure-will-record.md) 越权风险点 5 与 [ADR-0205](./0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md) 越权风险点 13，连带 [ADR-0225](./0225-the-departure-gate-reading-asks-through-consolidation-unit-members-at-the-moment-of-departure.md) 越权风险点 8；部分停用 ADR-0205 决定一一句与决定二一句、ADR-0225 决定六一句，以及 [ADR-0213](./0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md) 文末两则补记「答复与查阅」各一句（被停原句与适用场景见三份的 Status 行）。四份正文不改，只在其 Status 行与 Links 加前向指针。决定五（出发意图的起讫与声明的段绑定）出自隔离评审对 R11 的真库实证，同日补入，二轮评审后定为「对不上不问、说不出照问而只回整格」。越权风险点见文末，待 TF owner 复核。）<!-- 索引注记 -->｜**已接受**（2026-10-21，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 TF owner 裁定；票 demo-chain-gaps/146）。**收窄**：作业事实渠道上回给设备的门禁读数（出发登记、出发前问询、交接登记与更正、有效交付登记与 POD 更正，以及此后同形的问询口），逐对象那一格只列没得出放行的对象——阻断、读不动、未决；放行与不受门禁的不列，全都得出放行时没有逐对象那一格；整格、问关务时的意图与本侧停点照实给全。读不动与未决同列（R11 原文「被阻断或未决」，读不动由本记录代决并入）。**集运单元**：单元那一格是成员里最重的一格，成员没得出放行单元也没有，列出的成员沿单元一路找得到段里在控的那一件；单元只指名同样列出的成员，放行的成员不随袋带出；因本侧问不到的成员或空袋没得出放行时单元在列、`members` 为空数组。**一处判法**：领域 `ExecutorGateCells` 是唯一判法，各口线格式只经它取（出发读数经 `DepartureGateReading.ExecutorCells`，未绑定时一件不给）；它是投影，不带关务判断标识与依据原文。**落册照旧**：读数本身、库面明细、信封、VE 信号与运营查阅都不收窄；重放回显的是首次读数，回给设备时同样只列没得出放行的；问询与同参出发经同一判法，逐字节仍相同（ADR-0219 决定二不变）。**起讫与段绑定**：出发登记与出发前问询都核段里出发那一刻在控各件进段那次`已交接`声明的交出、去向节点与班次的起讫，判法在领域 `BindDepartureEntries`（已绑定、未绑定、对不上）。对不上（声明了的那一格与班次不同）停在「班次不服务该段」、交接册读不动停在读不动，都不问关务、不列对象——堵住设备给一段境内段配一班跨区班次、让段里不在案卷的每件都答未决、都回给设备那一格。说不出（凭收寄进段——揽收事实没有节点、进段交接没声明全、交接在册上认不出）是未绑定：照班次方向问关务、读数记全、VE 照旧派生信号，读数标 `entryUnbound`，回给设备的只有整格、起讫与那一位，一件不列。只核节点、不核班次身份。部分停用 ADR-0205 决定一「回给报事实一方的答复只带……逐对象那一格（对象与它的格）……执行方要知道车上被挡的是哪一件，所以逐对象那一格照给」里「逐对象那一格照给每件」那一半、ADR-0225 决定六「列出它在同一份读数里各有一格的直接成员」、ADR-0213 两则补记「答复与查阅」里「那件对象的那一格」，都只限回给登记方或问询方的答复；另部分停用 ADR-0205 决定二「本上下文这一侧找不齐问关务要的东西就停下……停点只属一格」那份停点名单，多两种停点（`SCHEDULE_NOT_SERVING_SEGMENT`、`HANDOVER_REGISTRY_UNAVAILABLE`）、读数另取决于交接登记册。越权风险点含读不动也列、剩余暴露（任一场外设备仍可问出任一段没得出放行的那几件、不留痕，跨区段与票 147 之后起讫有未登区域节点的段即全列，票 171）、故障期收窄失效、设备无从对账段里问了哪几件、空数组含义变了、单件读数也收窄、凭收寄进段与交接没声明全的段上设备只见整格、只核节点、交接声明错了或段内多站时诚实出发停在本侧且不形成信号、每件多读一次交接册、存量读数不回补、核不上的段上配假班次换得到一位汇总（照这么登出发还会落一份按错误意图问来的读数并派生信号）、有交接登记授予的一方塞一笔不声明节点的交接能把段降成只回整格，待 TF owner 复核
```

合并后：

```text
Status: Accepted（2026-10-21，Claude Code 会话代 TF owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」，2026-10-20 又答「要我裁定的部分，你作为业务和系统专家，请你自决吧」；票 demo-chain-gaps/146，裁定见代决复核报告 `.scratch/owner-review-20261020/report.md` 的 R11。回答 [ADR-0219](./0219-a-device-previews-the-departure-gate-before-departing-and-reads-what-the-departure-will-record.md) 越权风险点 5 与 [ADR-0205](./0205-departure-gate-is-a-pulled-reading-recorded-with-the-movement-fact-not-a-refusal.md) 越权风险点 13，连带 [ADR-0225](./0225-the-departure-gate-reading-asks-through-consolidation-unit-members-at-the-moment-of-departure.md) 越权风险点 8；部分停用 ADR-0205 决定一一句与决定二一句、ADR-0225 决定六一句，以及 [ADR-0213](./0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md) 文末两则补记「答复与查阅」各一句（被停原句与适用场景见三份的 Status 行）。四份正文不改，只在其 Status 行与 Links 加前向指针。决定五（出发意图的起讫与声明的段绑定）出自隔离评审对 R11 的真库实证，同日补入，二轮评审后定为「对不上不问、说不出照问而只回整格」。越权风险点见文末，待 TF owner 复核。）<!-- 索引注记 -->｜**收窄**：作业事实渠道上回给设备的门禁读数（出发登记、出发前问询、交接登记与更正、有效交付登记与 POD 更正，以及此后同形的问询口），逐对象那一格只列没得出放行的对象——阻断、读不动、未决；放行与不受门禁的不列，全都得出放行时没有逐对象那一格；整格、问关务时的意图与本侧停点照实给全。读不动与未决同列（R11 原文「被阻断或未决」，读不动由本记录代决并入）。**集运单元**：单元那一格是成员里最重的一格，成员没得出放行单元也没有，列出的成员沿单元一路找得到段里在控的那一件；单元只指名同样列出的成员，放行的成员不随袋带出；因本侧问不到的成员或空袋没得出放行时单元在列、`members` 为空数组。**一处判法**：领域 `ExecutorGateCells` 是唯一判法，各口线格式只经它取（出发读数经 `DepartureGateReading.ExecutorCells`，未绑定时一件不给）；它是投影，不带关务判断标识与依据原文。**落册照旧**：读数本身、库面明细、信封、VE 信号与运营查阅都不收窄；重放回显的是首次读数，回给设备时同样只列没得出放行的；问询与同参出发经同一判法，逐字节仍相同（ADR-0219 决定二不变）。**起讫与段绑定**：出发登记与出发前问询都核段里出发那一刻在控各件进段那次`已交接`声明的交出、去向节点与班次的起讫，判法在领域 `BindDepartureEntries`（已绑定、未绑定、对不上）。对不上（声明了的那一格与班次不同）停在「班次不服务该段」、交接册读不动停在读不动，都不问关务、不列对象——堵住设备给一段境内段配一班跨区班次、让段里不在案卷的每件都答未决、都回给设备那一格。说不出（凭收寄进段——揽收事实没有节点、进段交接没声明全、交接在册上认不出）是未绑定：照班次方向问关务、读数记全、VE 照旧派生信号，读数标 `entryUnbound`，回给设备的只有整格、起讫与那一位，一件不列。只核节点、不核班次身份。越权风险点含读不动也列、剩余暴露（任一场外设备仍可问出任一段没得出放行的那几件、不留痕，跨区段与票 147 之后起讫有未登区域节点的段即全列，票 171）、故障期收窄失效、设备无从对账段里问了哪几件、空数组含义变了、单件读数也收窄、凭收寄进段与交接没声明全的段上设备只见整格、只核节点、交接声明错了或段内多站时诚实出发停在本侧且不形成信号、每件多读一次交接册、存量读数不回补、核不上的段上配假班次换得到一位汇总（照这么登出发还会落一份按错误意图问来的读数并派生信号）、有交接登记授予的一方塞一笔不声明节点的交接能把段降成只回整格
```

### ADR-0238：派送出发前的交付门禁问询——作业事实渠道上按对象与拟交付时点只读一问，交回与随后同参有效交付同一套读数；运输履约不落记录，关务照记判断

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-21，Claude Code 会话代决：用户原话「你作为业务和系统专家自决……解决所有的问题」，2026-10-20 又答「要我裁定的部分，你作为业务和系统专家，请你自决吧」；票 demo-chain-gaps/94，代决复核报告 `.scratch/owner-review-20261020/report.md` 的 R14 把它列为上线前必修。回答 [ADR-0213](./0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md) 文末票 66 补记越权风险点 20「读数挡不住交付」，照 [ADR-0219](./0219-a-device-previews-the-departure-gate-before-departing-and-reads-what-the-departure-will-record.md) 同形；回给设备的逐对象那一格照 [ADR-0237](./0237-gate-readings-show-the-executor-only-the-objects-that-did-not-pass.md)。不停用任何已接受记录的条款，只在 ADR-0213 的 Status 行与 Links、ADR-0206 的 Links 加前向指针。越权风险点见文末，待 TF owner 复核，第 4 点同时待 CC owner。）<!-- 索引注记 -->｜**已接受**（2026-10-21，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 TF owner 裁定；票 demo-chain-gaps/94）。**问询口**：`POST /transport-fulfillment/delivery-gate-previews`，载荷只收对象与拟交付时点 `deliversAt`（封闭解码，时点缺席答未受理、不补服务端时钟），答复一律 200（`DELIVERY_GATE_PREVIEWED` 带 `gateReading`，或 `INPUT_NOT_ACCEPTED`），`gateReading` 与有效交付口同一线格式，逐对象那一格照 ADR-0237 只列没得出放行的那一件。按件问。**同参同读数**：问询与有效交付登记受理后收成同一组（租户、对象、时点）走同一条读数路径（at 取法、停点、问关务只在 ADR-0213 决定五与票 66 补记），对象的受理判据同一处；问询不读派送尝试，派送尝试读面那一格只属登记；同格只在两问之间段册、交接册与关务三源不变、且拟交付时点就是随后那件对象结果的发生时刻时成立。**不落记录**：运输履约不写册、不入队、不结束参与、不记谁问过，问询读数不交给随后的交付；关务照 ADR-0206 决定六记判断，同参交付命中同一判断。**授予**：与有效交付口同一只作业事实 Intake，不新增能力种类。**不挡交付**：派不派、交不交归一线客户端与现场；挂在设备派送出发之前，不挂在派送任务形成那一拍。回答 ADR-0213 票 66 补记越权风险点 20，无条款停用。越权风险点含读到放行不等于交付那一刻仍放行、用写授予开读口、客户端处置未定、关务判断册随问询增长（票 172）、按件问、任一场外设备可零成本探知任一件的关务状态与 at 且不留痕（票 171）、拟交付时点是预计、段按此刻取、CC 文档请求时机一句未补（票 165），待 TF owner 复核
```

合并后：

```text
Status: Accepted（2026-10-21，Claude Code 会话代 TF owner 决：用户原话「你作为业务和系统专家自决……解决所有的问题」，2026-10-20 又答「要我裁定的部分，你作为业务和系统专家，请你自决吧」；票 demo-chain-gaps/94，代决复核报告 `.scratch/owner-review-20261020/report.md` 的 R14 把它列为上线前必修。回答 [ADR-0213](./0213-a-transport-handover-declares-its-releasing-receiving-and-onward-nodes.md) 文末票 66 补记越权风险点 20「读数挡不住交付」，照 [ADR-0219](./0219-a-device-previews-the-departure-gate-before-departing-and-reads-what-the-departure-will-record.md) 同形；回给设备的逐对象那一格照 [ADR-0237](./0237-gate-readings-show-the-executor-only-the-objects-that-did-not-pass.md)。不停用任何已接受记录的条款，只在 ADR-0213 的 Status 行与 Links、ADR-0206 的 Links 加前向指针。越权风险点见文末，待 TF owner 复核，第 4 点同时待 CC owner。）<!-- 索引注记 -->｜**问询口**：`POST /transport-fulfillment/delivery-gate-previews`，载荷只收对象与拟交付时点 `deliversAt`（封闭解码，时点缺席答未受理、不补服务端时钟），答复一律 200（`DELIVERY_GATE_PREVIEWED` 带 `gateReading`，或 `INPUT_NOT_ACCEPTED`），`gateReading` 与有效交付口同一线格式，逐对象那一格照 ADR-0237 只列没得出放行的那一件。按件问。**同参同读数**：问询与有效交付登记受理后收成同一组（租户、对象、时点）走同一条读数路径（at 取法、停点、问关务只在 ADR-0213 决定五与票 66 补记），对象的受理判据同一处；问询不读派送尝试，派送尝试读面那一格只属登记；同格只在两问之间段册、交接册与关务三源不变、且拟交付时点就是随后那件对象结果的发生时刻时成立。**不落记录**：运输履约不写册、不入队、不结束参与、不记谁问过，问询读数不交给随后的交付；关务照 ADR-0206 决定六记判断，同参交付命中同一判断。**授予**：与有效交付口同一只作业事实 Intake，不新增能力种类。**不挡交付**：派不派、交不交归一线客户端与现场；挂在设备派送出发之前，不挂在派送任务形成那一拍。回答 ADR-0213 票 66 补记越权风险点 20，无条款停用。越权风险点含读到放行不等于交付那一刻仍放行、用写授予开读口、客户端处置未定、关务判断册随问询增长（票 172）、按件问、任一场外设备可零成本探知任一件的关务状态与 at 且不留痕（票 171）、拟交付时点是预计、段按此刻取、CC 文档请求时机一句未补（票 165）
```

### ADR-0239：不是本服务方的承运答复答订舱不在册——对外与订舱不在册同一格同一副响应，挡回依据与答复同笔记进承运答复挡回册，两条挡回路做一样多的事

处理：接受出处两写合并

合并前（main `311adf934`）：

```text
Status: Accepted（2026-10-21，Claude Code 会话代决：用户 2026-10-20 原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」，另有常设指令「你作为业务和系统专家自决……解决所有的问题」；代决复核报告 `.scratch/owner-review-20261020/report.md` 的 R9 裁「否」，票 demo-chain-gaps/145。回答 [ADR-0202](./0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md) 越权风险点 14；部分停用 ADR-0202 文末补记「承运答复的次序与格」一条里 `SOURCE_NOT_SERVICE_PROVIDER` 那一格（被停原句与适用场景见 ADR-0202 的 Status 行）。ADR-0202 正文不改，只在其 Status 行与 Links 加前向指针。越权风险点见文末，待 TF owner 复核，第 1 点知会接入身份 owner。）<!-- 索引注记 -->｜**已接受**（2026-10-21，用户原话「你作为业务和系统专家自决」，Claude Code 会话代 TF owner 裁定；票 demo-chain-gaps/145）。**对外一格**：承运答复口对答复方不是订舱所属委托服务提供方的答复答 `BOOKING_NOT_FOUND`（200），与订舱在本租户下不在册同一格、响应体逐字相同；`SOURCE_NOT_SERVICE_PROVIDER` 退出线格式，应用层结果里也不留这一格；续办同是找租户核对；来源核对仍在重放之前。**对内分清**：TF 立承运答复挡回册（迁移 0047），（租户，答复方来源身份，订舱，答复标识，依据）一行，依据封闭两值（订舱不在册、不是服务提供方）存成定宽码，记首末挡回时刻与次数，同键再来只加次数；与那一次答复同笔，写不动答未决、整笔回滚，不答订舱不在册；台账不形成答复、不妨碍订舱录入之后的正当答复、不存报文内容；不叫「拒收」「不受理」，两词在 TF 已有别义，TF CONTEXT 立「承运答复挡回」词条。**一样多的事**：订舱连同委托的服务提供方一条语句读回，两条挡回路都是一读一写、写的键等长；订舱标识与答复标识设 512 字节界，超界在查库之前答`未受理`，两条路一样，迁移 0047 遇已有超界行拒绝施加；TF 命令载荷限读 1 MiB、超限答坏报文；读不动同答 `BOOKING_STORE_UNAVAILABLE`、册写不动同答 `ANSWER_STORE_UNAVAILABLE`，答复口不再单读委托，剩下的差别只有时差。部分停用 ADR-0202 补记那一格。越权风险点含挡回册没有查阅面（票 167）、剩余时差、数据违约 500 仍暴露存在、册无保留期与聚合（票 170）、未决借答复册的名、不存报文内容、字节界是产品常量、限读作用于 TF 全部命令口、键长余量靠租户与来源身份短，待 TF owner 复核
```

合并后：

```text
Status: Accepted（2026-10-21，Claude Code 会话代 TF owner 决：用户 2026-10-20 原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」，另有常设指令「你作为业务和系统专家自决……解决所有的问题」；代决复核报告 `.scratch/owner-review-20261020/report.md` 的 R9 裁「否」，票 demo-chain-gaps/145。回答 [ADR-0202](./0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md) 越权风险点 14；部分停用 ADR-0202 文末补记「承运答复的次序与格」一条里 `SOURCE_NOT_SERVICE_PROVIDER` 那一格（被停原句与适用场景见 ADR-0202 的 Status 行）。ADR-0202 正文不改，只在其 Status 行与 Links 加前向指针。越权风险点见文末，待 TF owner 复核，第 1 点知会接入身份 owner。）<!-- 索引注记 -->｜**对外一格**：承运答复口对答复方不是订舱所属委托服务提供方的答复答 `BOOKING_NOT_FOUND`（200），与订舱在本租户下不在册同一格、响应体逐字相同；`SOURCE_NOT_SERVICE_PROVIDER` 退出线格式，应用层结果里也不留这一格；续办同是找租户核对；来源核对仍在重放之前。**对内分清**：TF 立承运答复挡回册（迁移 0047），（租户，答复方来源身份，订舱，答复标识，依据）一行，依据封闭两值（订舱不在册、不是服务提供方）存成定宽码，记首末挡回时刻与次数，同键再来只加次数；与那一次答复同笔，写不动答未决、整笔回滚，不答订舱不在册；台账不形成答复、不妨碍订舱录入之后的正当答复、不存报文内容；不叫「拒收」「不受理」，两词在 TF 已有别义，TF CONTEXT 立「承运答复挡回」词条。**一样多的事**：订舱连同委托的服务提供方一条语句读回，两条挡回路都是一读一写、写的键等长；订舱标识与答复标识设 512 字节界，超界在查库之前答`未受理`，两条路一样，迁移 0047 遇已有超界行拒绝施加；TF 命令载荷限读 1 MiB、超限答坏报文；读不动同答 `BOOKING_STORE_UNAVAILABLE`、册写不动同答 `ANSWER_STORE_UNAVAILABLE`，答复口不再单读委托，剩下的差别只有时差。越权风险点含挡回册没有查阅面（票 167）、剩余时差、数据违约 500 仍暴露存在、册无保留期与聚合（票 170）、未决借答复册的名、不存报文内容、字节界是产品常量、限读作用于 TF 全部命令口、键长余量靠租户与来源身份短
```
