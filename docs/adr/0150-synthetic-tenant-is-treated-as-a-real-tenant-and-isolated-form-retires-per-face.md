# ADR-0150：开发与演示中合成租户按真实租户对待——代码路径不区分合成与真实租户，合成租户经真渠道进出；隔离形态是真渠道缺位时的过渡，某一口的真渠道 Intake 落地即在同一笔撤下该口的隔离放行；两条红线不变

Status: Accepted（2026-09-24，通道 4 据用户在 IDP 队列同日两句裁定成文——「只有合成与真实租户数据这道墙在开发的时候也无需考虑，当成真实的就行了，简化开发」，以及对本记录提案的「同意」。本记录**部分停用** [ADR-0091](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) 决定六的一句与 [ADR-0100](./0100-operator-identity-is-a-product-owned-access-channel-family.md) 决定五第二条（见决定三），两记录其余各条不变。裁决能力边界：读过 ADR-0091 全文、ADR-0100 全文、[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 全文、[ADR-0078](./0078-isolated-environment-operations-reads-admit-by-assembly-injection.md) 的 Decision 标题与 [adr/README](./README.md) 对它的部分停用说明、[ADR-0141](./0141-integrator-sandbox-is-a-deployment-form-of-the-first-party-api-on-synthetic-data.md) 的索引摘要、`cmd/parcel-api/endpoints.go` 的隔离放行段与 `assemble_isolated_write.go` 的放行名单、operator-channel 04 / 05 / 10 / 11 的票面；没读：ADR-0078 正文的 Context 与 Alternatives、ADR-0083、ADR-0141 全文、各上下文 `CONTEXT.md`、演示种子脚本全文。拿不准的列在文末「越权风险点」。）
Date: 2026-09-24

## Context

**合成租户今天有两条进出路径。** 一条是隔离形态：装配点注入 `SYN-` 合成租户，读面按 ADR-0078、写面按 ADR-0091 逐口放行；另一条是真渠道：操作者渠道（ADR-0100、ADR-0149）、首方客户渠道（ADR-0139，Proposed）。真渠道今天只落了操作者册（operator-channel/01），凭据校验、信封与各口换真 Intake 都还在票上，所以演示动线实际走的全是隔离形态。

**两条路径被写成了永久并存。** ADR-0091 决定六：「隔离形态不因真渠道出现而自动退场」；ADR-0100 决定五第二条：隔离形态与操作者渠道「是装配点上的两行，互不替代、不合并」。照这两句，每个命令口在真渠道落地之后仍要长期维护两份准入——隔离 Intake 上的方法、放行名单、启动日志与两套测试——而演示动线证明的始终是一条租户生产上并不存在的路径。

**隔离形态守的两件事，都不靠它守。** 一是合成值不能变成生产默认——红线「租户取值留给租户」与 ADR-0146 决定三（参考配置显式采用才生效）已经守着，与数据从哪条路进来无关。二是合成证据不能冒充生产证据——红线「证据层级诚实」守着，靠的是合成标识本身可辨认，不是准入路径不同。合成租户与真实租户之间的数据隔离，由租户边界（[ADR-0003](./0003-group-tenant-legal-entity-customer-account.md)）本来就给了。

**用户的裁定是简化**：开发中把合成租户当成真实租户对待，不为它另走一条路。

## 候选与反方

**甲 · 维持现状，隔离形态与真渠道永久并存。** 反方：每口两份准入与两套测试；演示证明的是生产上不存在的路径；与用户裁定相反。否决。

**乙 · 现在就删掉隔离形态。** 反方：真渠道还没建成，删了演示动线的命令口全部退回 `403`——以简化之名把今天能走的路堵死。否决。

**丙 · 逐口过渡：某一口真渠道落地的同一笔撤下该口的隔离放行，合成租户以合成主体经真渠道进出。** 即本记录。

**丁 · 在真渠道里给合成租户开例外（如免令牌、免授予）。** 反方：ADR-0100 否决「开发用」本地账号的理由原样成立——事后分不开真实认证结果与例外放行；也正是本记录要去掉的那种「按合成与否分支」。否决。

## Decision

**一、代码路径不区分合成租户与真实租户。** 演示租户（`scripts/demo-seeds` 灌的 `SYN-TENANT-01`）在代码眼里就是一个租户：同一套 Intake、编排、登记册、授予与准入。生产代码不以租户是否带 `SYN-` 前缀决定控制流；`SYN-` 是命名约定，供证据层级与数据审计辨认。决定三所说过渡期内既有的隔离形态门禁是唯一的例外，随隔离形态一起退场。

**二、合成租户经真渠道进出。** 管理台操作用合成操作者主体（在操作者册登记，令牌由演示环境的 OIDC 发行方签发，选型归 operator-channel/02）；一线作业事实用合成设备（operator-channel/10）；外部结果与资金事实用合成集成客户端（operator-channel/11）；客户面在 ADR-0139 接受后用合成客户渠道行。这些合成主体都走普通登记，不另造旁路（同 ADR-0146 决定三「采用就是一次普通登记」的取法）。

**三、隔离形态是真渠道缺位时的过渡，逐口退场。** 某一口在装配点换上真渠道 Intake 的同一笔，撤下该口的隔离放行：隔离 Intake 类型上对应的方法、放行名单（`isolatedWriteAdmittedCommandLines` 与隔离读放行表）里对应的行，以及该口的隔离放行用例——改写为真渠道的答复格用例。最后一口换完的那一笔，删除 `IDP_PARCEL_ISOLATED_READ_TENANT` 与 `IDP_PARCEL_ISOLATED_WRITE_TENANT` 两个开关。真渠道落地之前，新开的口仍可照 ADR-0091 逐口放行，否则演示动线会在真渠道之前断掉；每加一口，就是日后要随真渠道撤下的一行。据此部分停用：

- ADR-0091 决定六「隔离形态不因真渠道出现而自动退场」一句——改为按口随真渠道退场。同句「也不因它出现而获得任何真实租户」与决定六其余各句不变。
- ADR-0100 决定五第二条中「隔离形态（ADR-0091）继续靠 `SYN-` 写开关服务演示，它与操作者渠道是装配点上的两行，互不替代、不合并；ADR-0091 Decision 六『隔离形态不因真渠道出现而自动退场』照旧」——改为过渡期内两行并存、真渠道落地即替代该口的隔离行。同条「不做绕过 OIDC 的『开发用』本地账号」不变。

过渡期内隔离形态的现行规则——缺省朝拦、`SYN-` 前缀门禁、读写两个开关、放行出声（ADR-0078 与 ADR-0091 其余各条）——一字不变。

**四、两条红线不变，它们本来就不靠隔离形态。** 租户取值留给租户：不写死为生产默认，参考配置显式采用才生效。证据层级诚实：合成租户上形成的证据一律记 `S`，改走真渠道也不升级。本记录不放宽任何一条。

**五、演示环境与租户生产环境的区别只在部署。** 演示与开发部署灌合成租户、配演示发行方；租户的生产部署不灌合成租户。这是部署时灌不灌数据，不是代码分支——与 ADR-0141「沙箱是部署形态，不是代码路径」同一取法。

## Consequences

- operator-channel 04、05、10、11 票面各加一句：已换各口若在隔离放行名单上，同一笔撤下该口的隔离放行；05 原完成判据「隔离读放行的既有用例不改一字仍绿」据本记录改写，原句在票面留痕。
- 演示动线脚本（[合成演示动线](../design/synthetic-demo-journey-script.md)）按口改走合成主体登录；过渡期内隔离开关与真渠道混用，全部退场时脚本删去两个开关的设置。
- AGENTS.md「演示租户」那句补一句「代码按真实租户对待它」并指向本记录。
- ADR-0091、ADR-0100 的 `Status` 行与 Links 节加前向指针；[adr/README](./README.md) 两行补部分停用说明、新增本记录一行。
- ADR-0141（Proposed）「隔离写开关与沙箱是两条路不合并」在隔离形态退场后失去对象；它的接受与否仍归用户，本记录不改它。
- 票 product-strategy-boundary/19 新开的 `/transport-fulfillment/delivery-attempts` 按决定三的过渡条款经写开关放行，随 operator-channel/10 撤下。

## 越权风险点

1. **合成主体与真实主体同册。** 操作者册、设备册、集成客户端册里合成主体靠 `SYN-` 命名辨认，本记录不加「是否合成」列；若审计或演示库重置需要显式列，归各册 owner 另裁（ADR-0141 风险点也提过操作者册的合成标记）。
2. **演示多一个运行依赖。** 撤下隔离放行的口要靠演示发行方签令牌；发行方不可用时这些口在演示里停在 `401` / `403`，比今天多一处可坏的地方。发行方的选型与运维归 operator-channel/02。
3. **过渡期可能很长。** 客户面的口要等 ADR-0139 接受才有真渠道，两个开关在那之前删不掉；「最后一口」何时到来不由本记录决定。
4. **只读了 ADR-0078 的 Decision 标题与索引说明。** 若其正文 Context 或 Alternatives 有依赖「隔离形态永久存在」的论证，需 owner 复核是否一并停用。
5. **ADR-0089 的受控批量导入不在本记录范围。** 那条路径带自己的 `FTI/` 来源标记与拆除期限，与隔离形态是两回事。

## Links

- [ADR-0091：隔离形态从查阅面扩到写路径，按分级开关放行](./0091-isolated-form-extends-to-the-write-path-by-graded-switches.md)：本记录部分停用其决定六「隔离形态不因真渠道出现而自动退场」一句；过渡期内其余各条原样有效
- [ADR-0100：管理台运营操作者身份是产品自有的接入渠道族](./0100-operator-identity-is-a-product-owned-access-channel-family.md)：本记录部分停用其决定五第二条中隔离形态与操作者渠道「互不替代、不合并」的一段；同条「不做『开发用』本地账号」不变
- [ADR-0078：隔离环境运营查阅按装配注入放行](./0078-isolated-environment-operations-reads-admit-by-assembly-injection.md)：隔离读面的出处，过渡期内原样有效，随各查阅口的真渠道退场
- [ADR-0146：产品策略是机制与实例之间的第三类](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：「采用就是一次普通登记」与参考配置显式采用才生效的出处，决定二、四沿用
- [ADR-0149：主链业务命令面按提交者分两族](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)：合成设备与合成集成客户端所经的两族真渠道
- [ADR-0139（Proposed）](./0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md)、[ADR-0141（Proposed）](./0141-integrator-sandbox-is-a-deployment-form-of-the-first-party-api-on-synthetic-data.md)：客户面真渠道与沙箱部署形态，接受与否归用户
- [ADR-0003：集团租户、法人责任与货主客户账户三级边界](./0003-group-tenant-legal-entity-customer-account.md)：合成租户与真实租户的数据隔离由租户边界给出
- [AGENTS.md](../../AGENTS.md)：「演示租户就是 `SYN-TENANT-01`，开发与演示共用这一份」的运行规则落点
