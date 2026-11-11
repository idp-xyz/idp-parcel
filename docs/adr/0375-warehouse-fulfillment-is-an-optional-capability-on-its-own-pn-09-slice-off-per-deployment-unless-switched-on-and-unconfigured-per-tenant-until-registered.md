# ADR-0375：仓配履约是可选能力，立为首发之外的 PN-09 切片——部署不显式打开就不装配，租户不登记仓、不授予就各格答未配置，其他上下文不以它为前提，产品就绪的判据不算它

Status: Accepted（2026-11-11，Cursor 会话代决：用户原话「按你的定义开始吧，在我们的系统仓配实现的时候可以是可选的能力吗」——前半句让会话照它上一轮说的顺序开工（立切片、写 UC、先定仓储费的 ADR），后半句是问句，会话答可以，并把做法写成本记录；用户没有逐条确认。决定一、二属产品 owner；决定三至五落到小包托运、派发与 `parcel-api` 的装配、接入身份和仓配履约的细节由会话拟定、代各 owner 决，越权风险点见文末，待产品与各上下文 owner 复核。部分停用 [ADR-0368](./0368-commodity-inventory-and-warehouse-fulfillment-join-the-product-as-a-new-context-and-node-operations-stays-non-wms.md) Consequences 一句里的一半，落实其决定八的「另立 PN 切片」，见「对已接受记录的作用」。票 [warehouse-fulfillment/05](../../.scratch/warehouse-fulfillment/issues/05-stand-up-the-pn-09-slice-and-make-warehouse-fulfillment-an-optional-capability.md)。本记录不动 `internal/**`。起草能力边界：读过开发主线全文，ADR-0368、ADR-0370、ADR-0371、ADR-0345、ADR-0352、ADR-0222 全文，仓配履约 CONTEXT 全文，CONTEXT-MAP 的仓配履约一节与「关系约束」里仓配各条，产品自身收费模型与外部 WMS 协作缝设计备忘全文，以及 `internal/platform/dispatch` 的直投与扇出、`cmd/parcel-dispatch/routes.go`、`cmd/parcel-api/endpoints.go`、`internal/architecture/boundaries_test.go`、`internal/platform/migrate/plan.go`；没读：其余上下文的 CONTEXT 全文、各 UC 全文、`internal/accessidentity` 的授予模型细节、管理台。）<!-- 索引注记 -->｜**判据**：仓配履约是可选能力——不用仓配、接外部 WMS、用本产品的仓配履约三种用法并列，部署没启用仓配或租户没登记仓时，其他上下文各用例的答复与交出的信封和没有仓配代码时相同；切片立为 PN-09，不进首发切片表、不计入产品就绪，自己按同一套产品就绪判据定级；其他上下文的用例不读仓配的读口、不等它的信封（专为消费仓配事实而立的几处除外），仓配挂在别人信封上的每一路对不属于仓配的信封直接入账收工，其他上下文为仓配新增的信封对各来源照交、没启用仓配的部署里走派发登记的收工路；部署参数 `IDP_PARCEL_WAREHOUSE_FULFILLMENT_ENABLED` 只认恰为 `true`、默认关，是长期的部署参数，关着时派发不登记仓配的任何一路、仓配端点不提供业务答复，库表照常施加；租户与货主两级靠登记仓与授予，不另立启用开关；不做按租户的商业授权，不拆成独立进程

Date: 2026-11-11

## Context

取证于 main `4d8809add`，只记 `S`。

- **切片还没立。** [ADR-0368](./0368-commodity-inventory-and-warehouse-fulfillment-join-the-product-as-a-new-context-and-node-operations-stays-non-wms.md) 决定八把仓配留给「另立 PN 切片」，并写明首发范围不变。[开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)的切片表叫「首发纵向开发切片」，产品就绪按它判：「产品就绪 = 八个切片的机制半边与产品策略全部达标」。仓配不在首发里，它的切片进不了那张表，也不该改那句判据。
- **对多数租户它是用不到的能力。** 开发主线「目标客户群」的分层表里，只有「3PL 与集运干线运营商」一层「以仓配或集运为主业，跨境干线是其中一段」；其余各层的常规业务形态里没有仓配。「产品需求假设」的 `PA-WF-01` 已写「某个租户用哪一种是租户取值」，自带 WMS 的租户照[外部 WMS 协作缝设计备忘](../design/external-wms-collaboration-seams-design-note.md)接入。可选是产品事实，不是实现上的偏好。
- **现有机制已经给出的。**
  - 跨上下文调用只经消费侧适配器（[ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)，由 `internal/architecture` 的边界门禁守）；仓配履约是小包托运的消费方，不是反过来。
  - `parcel-api` 与 `parcel-dispatch` 的组合根按特性文件登记端点行与派发路由（[ADR-0222](./0222-composition-roots-register-endpoints-and-routes-per-feature-file.md)）。
  - 每个端点行有接入面，按授予打开（[ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md)、[ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)、[ADR-0139](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md)）。
  - 迁移计划是一份统一的有序历史（`internal/platform/migrate`）；代收与清分首发不启用，它的表也在这份历史里。
  - 部署参数有先例：[ADR-0352](./0352-registering-a-price-card-in-a-canonicalization-newer-than-the-base-one-needs-an-explicit-deployment-switch-that-is-off-by-default.md) 的登记开关、[ADR-0180](./0180-admission-scope-tenant-mapping-is-an-access-identity-register-and-self-authority-is-a-deployment-parameter.md) 的本产品权威串。
- **现有机制没回答的。**
  - **扇出。** [ADR-0371](./0371-warehouse-fulfillment-acts-for-the-owner-on-warehouse-outbound-requests-and-every-decision-path-hands-off-the-acceptance-decision.md) Consequences 写「仓配履约要消费四类小包托运的信封：接受决定已形成、包裹取消已记录、资料版本已形成、委托待受控补充。」其中接受决定已形成是每一份委托都会交出的信封。派发的 `FanOut` 头注写「任一失败都让 Publish 失败，派发器会重试。」仓配那一路停住，那一封信封与同分区排在它后面的信封都等；小包托运的接受决定信封按租户加委托分区。
  - **无订阅者。** `ErrNoSubscriber` 头注写「按 ADR-0049 无订阅者显式失败并入账，因而会阻塞该分区直到失败预算耗尽。」ADR-0371 决定三新增「委托待受控补充」信封，UC-PS-001「给开发的交接」记着「其他来源暂无消费方，投递路由随实现定」；资料版本已形成的信封照 ADR-0371 Context 的取证也「还没有消费方」。仓配不在的部署里，这两类信封没有订阅者。
  - **实施期。** 仓配的实现会分很多笔进 main，而首个租户的生产部署跑的是同一个 main。
- **开工时点。** ADR-0368 Consequences 写「首发不受影响（决定八），但会分走首个租户上线前后的开发注意力；排期宜放在首个租户上线之后。」用户 2026-11-11 点名现在开工。

## Decision

**一、仓配履约是可选能力。**（产品边界）

- 产品不带仓配也是完整的。三种用法并列：不用仓配；接外部 WMS，照外部 WMS 协作缝设计备忘；用本产品的仓配履约。某个租户用哪一种是租户取值，`PA-WF-01` 原句照旧。
- 判据：部署没启用仓配，或租户没登记任何仓时，其他上下文各用例的答复与交出的信封，和没有仓配代码时相同。

**二、立 PN-09「仓配履约」，属首发之外的可选能力切片。**（产品边界）

- 它不进「首发纵向开发切片」表，在开发主线另起「首发之外的可选能力切片」一节。「产品就绪 = 八个切片的机制半边与产品策略全部达标」一句不改：PN-09 达标与否不改变产品就绪的结论，它只回答仓配这项能力可不可以对外承诺。
- PN-09 自己按开发主线「切片的机制半边与实例半边」一节的同一套产品就绪判据定级。它的演示动线是仓配自己的一条，由 PN-09 的开发交接定义，证据记 `S`。
- 标识：用例 `UC-WF-*`，验收场景 `AT-WF-*`，工作包 `PN09-W*`。
- 现在开工。首发范围与[上线线与冻结清单](../../.scratch/launch-line-freeze-2026-10-27.md)不变；冻结清单管的是存量票池，仓配切片的票是用户点名的新能力，不在其内。

**三、其他上下文不以仓配履约为前提。**（机制）

- 依赖方向：仓配履约可以经自己的消费侧适配器调用其他上下文、消费它们的信封。其他上下文的用例不读仓配履约的读口来形成自己的答复，也不等它的信封才继续——专为消费仓配事实而立的几处除外：结算采用仓储作业收费发生项、计价评价它、全程追踪投影出库确认。这几处只在有仓配事实时被触发，是 PN-09 的承接项，不是所在上下文首发用例的一部分。
- 小包托运与参与方与商业为仓配加的词表（来源「仓配出库」、授权动作「仓配代办」与代办等级）照 ADR-0371 是它们自己的规则，部署启不启用仓配都在；没有仓配提交时不被触发。
- 仓配履约挂在其他上下文信封上的每一路消费者：信封不属于仓配（委托的来源不是「仓配出库」）时不读任何依赖、直接入账收工，不得答未决。启用了仓配的部署里，一路停住因此只拖住它自己那份仓配出库委托所在的分区。
- 其他上下文为仓配新增的信封类型——今天是 ADR-0371 决定三的「委托待受控补充」——照各自的规则对各来源照交，发出方不按来源分支。部署没启用仓配时，派发为这类信封登记一条收工路：只入账、不做别的，登记处写明本部署没有它的下游；启用时由仓配的那一路接。判据：没启用仓配的部署里，没有一封信封因仓配缺席而失败。
- 本来就没有订阅者的既有信封不归收工路管。资料版本已形成的信封照 ADR-0371 Context 的取证今天就没有消费方，本记录不改变它：没启用仓配的部署里它照旧，启用时仓配的那一路是它的第一个消费者。

**四、部署不显式打开就不装配。**（机制）

- 部署参数 `IDP_PARCEL_WAREHOUSE_FULFILLMENT_ENABLED`，默认关，取值的读法照 ADR-0352 决定二：「去首尾空白后恰为 `true` 才开；缺省、空、`TRUE`、`1`、`yes` 都按关读。」各进程在启动时读一次、带启动日志，经装配的共享件交给各特性；领域包不读环境。
- 关着时：
  - `parcel-dispatch` 不登记仓配的任何一路，包括它自己各类信封的路与它挂在其他上下文信封上的路。
  - `parcel-api` 的仓配端点不向任何调用方提供业务答复。怎么答归第一张落仓配端点的实施票定：沿用「接入渠道未配置」那一格，还是按恢复动作另立一格。两种做法都碰 ADR-0222 决定三按接入面核声明的闭合用例，由那张票的记录向 [ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md) 与 ADR-0222 挂指针。两种都不得折成 404。
  - 仓配的登记 CLI 与仓内作业事实的导入拒绝执行。
- 库表不随开关变：仓配的迁移进同一份迁移计划，关着开关的部署也施加，表是空的。打开开关因此只是一次重启，不是一次迁移。
- 开关是部署方的断言——这个部署提供仓配——不是租户取值；一个部署里所有租户同开同关。它是长期的部署参数，不是实施期的临时护栏：不用仓配的部署一直关着。
- 开过再关：关之前先让派发把仓配的在途信封投完。关了之后还在途的仓配信封没有订阅者，照 ADR-0049 显式失败并入账，不静默丢。
- 两种状态都要有装配测试。关着：路由表里没有仓配的路，其他上下文的用例全套照旧。开着：仓配各路在，且对不属于仓配的信封入账收工。全量测试与演示种子以关着为基线；仓配自己的用例与 PN-09 的演示动线显式打开。

**五、租户与货主两级靠登记与授予，不另立启用开关。**（租户取值）

- 仓、库区、库位与货主商品都由租户登记。一个仓都没登记的租户，仓配各写口答未配置或查无所指的仓，各读口答空，不建默认仓。
- 进仓配各面要有授予：操作者的仓配决定种类与仓内作业范围，客户系统的仓配客户面能力，货主的仓配代办授予（ADR-0371 决定二）。种类与能力的词表随实施票进 `internal/accessidentity`；没有授予时照各族既有的答法。
- 不立「该租户是否启用仓配」的登记。它与「登记了仓、给了授予」说的是同一件事，另立一格就是两处定义。
- 管理台的仓配入口按授予收起，沿用现有做法。

**六、不做的。**

- 不做按租户的商业授权或计量（这个租户买没买仓配）。那是[产品自身收费模型](../product/PRODUCT-MONETIZATION-MODEL.md)的事，要做照它的规矩单独立票。
- 不拆成独立进程、独立二进制或可热插拔的插件（[ADR-0009](./0009-go-modular-monolith-and-versioned-bento-contracts.md)）。
- 不做只有库存记数的轻量版；ADR-0368 Alternatives 已否决「只做轻库存」。

## 对已接受记录的作用

- ADR-0368 Consequences「首发不受影响（决定八），但会分走首个租户上线前后的开发注意力；排期宜放在首个租户上线之后。」一句里「排期宜放在首个租户上线之后」那一半，部分停用——改为：切片、用例与机制半边自 2026-11-11 起开工（用户点名）。**适用场景**：只限开工时点。「首发不受影响」照旧成立，由决定四的开关守；首发范围、冻结清单与其余各条不变。
- ADR-0368 决定八「仓配履约照 [ADR-0016](./0016-product-delivery-and-tenant-pilot-as-parallel-tracks.md) 的两轨制另立 PN 切片」由决定二落实为 PN-09，无条款停用。
- 不停用、照旧成立：
  - [ADR-0049](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md) Decision 第 3 条「路由表是显式清单，没有订阅者的事件类型显式失败并入账，不静默丢弃，也不得回退成「未知类型即放行」。」决定三的收工路是为指名的信封类型显式登记的一路，入账，不是未知类型放行；关着开关时在途的仓配信封照它显式失败。
  - ADR-0222：仓配的特性文件照样在 `init` 里登记构造函数，开关关着时构造函数交回零路。
  - ADR-0055：仓配端点行照样进端点登记处；关着时怎么答留给实施票（决定四）。
  - [ADR-0078](./0078-isolated-environment-operations-reads-admit-by-assembly-injection.md) 决定四「不得据此再添 demo/mode 类全局开关或第二个 main」：那一句管的是换一种运行模式的开关；本记录的开关不改变任何既有用例的行为，只决定一项能力装不装，与 ADR-0352 的登记开关同类。
  - ADR-0371 决定三、五与小包托运 CONTEXT 的对应规则：信封对各来源照交，发出方不按来源分支；决定三只给没启用仓配的部署在派发里加一条收工路。
  - 开发主线「产品就绪 = 八个切片的机制半边与产品策略全部达标」，与[首发试点范围](../product/PILOT-SCOPE.md)的「首发不建设完整 WMS。」

## 同笔改动的文件

- ADR-0368 的 `Status` 行索引注记与 Links 各加一条前向指针，正文不动。
- 开发主线：新增「首发之外的可选能力切片」一节，登 PN-09。
- [CONTEXT-MAP](../domain/CONTEXT-MAP.md)：仓配履约一节在「首发不包含本上下文的能力。」之后补一句，「关系约束」补一条，写决定一、三。
- [仓配履约 CONTEXT](../domain/warehouse-fulfillment/CONTEXT.md)「Boundaries and ownership」补决定一、五。
- [docs/README](../README.md) 的仓配履约条目、[ABBREVIATIONS](../ABBREVIATIONS.md) 的 `PN-` 一行、[workflow.md](../agents/workflow.md)「记号」表、产品业务全景说明书「PN-01 至 PN-08」一节各补一句指针。
- `.scratch/warehouse-fulfillment/` 下补 `spec.md` 与本票之后的各张文档票。
- 不改小包托运 CONTEXT、UC-PS-001 与 ADR-0371：决定三对它们没有条款停用，实现约束随 PN-09 的实施票进各 UC「给开发的交接」。
- ADR 索引由推送方重生成，不手改。

## Consequences

- 首发与首个租户的生产部署不受在建仓配的影响：开关默认关，关着时派发里没有仓配的路。
- 多一个部署参数与两种装配状态，装配测试两种都要跑。
- 仓配的词表（来源、授权动作、代办等级、决定种类、客户面能力）在不用仓配的部署与租户里也在，只是不被触发。
- 关着开关的部署，库里有仓配的空表。
- 多租户部署打开开关之后，不用仓配的租户靠决定三、五保持无感；产品自己拦不住「没买仓配的租户登记了仓」，那一层在商务侧。
- 仓配切片的每张实施票都多两条验收：开关关着时其他上下文的答复不变；开着时对不属于仓配的信封入账收工。
- 决定三的依赖方向今天没有门禁守：`internal/architecture` 的边界门禁认的是适配器的位置，不认方向。PN-09 的头一个工作包要补一条。

## Alternatives considered

- **不设部署开关，只靠授予与未登记。** 否决：派发的路是进程级的，没有授予可言；仓配的路一登记，每一份委托的接受决定信封都要过它，半成品期间等于把在建代码接进主链。
- **按租户立一本启用登记册**（如 [ADR-0345](./0345-tenant-enablement-of-countries-and-currencies-is-a-tenant-value-registered-in-party-commercial-and-new-write-entries-accept-only-catalogue-codes.md) 的启用范围）。否决：租户登记仓就是启用，另立一本是同一件事的第二处定义。
- **默认开。** 否决：升级一次就把仓配带进每个既有部署，与 ADR-0368 决定八的首发范围不变相抵。
- **把 PN-09 放进首发切片表。** 否决：产品就绪会被绑到一项首发不包含的能力上。
- **拆成独立服务或独立二进制。** 否决：ADR-0009 已对同一类取舍裁过；仓配提交委托、代办与收信封都要跨进程，没有负载或团队证据支撑这份一致性与运维成本。
- **编译期裁剪（build tag）。** 否决：两套二进制、两套测试矩阵，换来的只是包体小一点；运行期开关已经做到关着不装配。
- **「委托待受控补充」只对来源为「仓配出库」的委托交出。** 否决：发出方要因此知道谁在消费它，而且收窄了 ADR-0371 决定三与小包托运 CONTEXT 不分来源的那条规则；派发登记收工路不动这两处，也与启用时仓配的那一路对不属于仓配的信封入账收工是同一个形状。
- **没启用仓配时让「委托待受控补充」照无订阅者失败。** 否决：每一次等待受控补充都会多出一封失败的信封并占住它的分区直到失败预算耗尽，这是仓配的存在给不用仓配的部署带来的变化，与决定一的判据相抵。

## 越权风险点

1. 可选的口径、PN-09 不计入产品就绪、现在开工（决定一、二）。归产品 owner。
2. 来源过滤与收工路（决定三）：仓配的消费者要从信封上认得出委托的来源；没启用仓配的部署里，「委托待受控补充」由派发登记的收工路入账，小包托运的发出规则不动；资料版本已形成的信封照旧没有订阅者。归 PS owner 与派发 owner。
3. 开关默认关、只认 `true`、进程级、长期存在；开过再关要先排空在途的仓配信封；关着时端点的答法未定（决定四）。归运维、`parcel-api` owner 与派发 owner。
4. 库表不随开关变（决定四）：关着的部署也带空表。归运维。
5. 租户内谁能开始用仓配（决定五）：登记册配置写与运营查阅读两族不分上下文，持有它们的操作者就能登记仓；要更细得另开能力面。归接入身份 owner 与仓配履约 owner。
6. 没有商业授权（决定六）：多租户部署开着开关时，任何租户登记了仓就用得上。归产品 owner。
7. 某个租户在生产上启用仓配怎样经 PN-08 的阶段准入，本记录没定。归产品 owner 与试点治理 owner。

第 6、7 两点在任何生产部署打开开关之前要有答案。

## Links

- [ADR-0368](./0368-commodity-inventory-and-warehouse-fulfillment-join-the-product-as-a-new-context-and-node-operations-stays-non-wms.md)：部分停用其 Consequences 里「排期宜放在首个租户上线之后」那一半，落实决定八的「另立 PN 切片」
- [ADR-0370](./0370-warehouse-outbound-dispatches-by-product-network-owner-label-or-owner-pickup-and-the-remaining-warehouse-gaps-are-decided.md)、[ADR-0371](./0371-warehouse-fulfillment-acts-for-the-owner-on-warehouse-outbound-requests-and-every-decision-path-hands-off-the-acceptance-decision.md)：仓配履约的发运方式、代办与要消费的信封
- [ADR-0016](./0016-product-delivery-and-tenant-pilot-as-parallel-tracks.md)、[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：两轨制与机制、产品策略、租户取值的分界
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)、[ADR-0222](./0222-composition-roots-register-endpoints-and-routes-per-feature-file.md)、[ADR-0049](./0049-publish-channel-is-in-process-delivery-until-load-evidence.md)、[ADR-0055](./0055-business-endpoint-intake-has-an-unconfigured-grade.md)：依赖方向、组合根登记、直投与未配置即拒
- [ADR-0352](./0352-registering-a-price-card-in-a-canonicalization-newer-than-the-base-one-needs-an-explicit-deployment-switch-that-is-off-by-default.md)、[ADR-0180](./0180-admission-scope-tenant-mapping-is-an-access-identity-register-and-self-authority-is-a-deployment-parameter.md)：部署参数的先例
- [开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)：切片表、产品就绪判据、目标客户群与 `PA-WF-01`
- [外部 WMS 协作缝设计备忘](../design/external-wms-collaboration-seams-design-note.md)：不用本产品仓配时的另一条路
- [仓配履约 CONTEXT](../domain/warehouse-fulfillment/CONTEXT.md)、[CONTEXT-MAP](../domain/CONTEXT-MAP.md)
