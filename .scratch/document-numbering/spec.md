# 单据编号：按规则给单据开立业务编号

Category: enhancement
Status: in-progress——2026-11-02 子票 01–09 全部进 main（ADR-0355）；收尾票 10、补页票 11 已立。此前 in-progress——2026-11-02 协调方起草；用户对草稿答「继续」，「定下的几条」与默认格式照此采纳，决定落在 [ADR-0355](../../docs/adr/0355-a-document-number-is-separate-from-internal-identity-and-is-assigned-in-the-issuing-transaction-under-built-in-rules-a-tenant-may-override.md)（Accepted）；子票 01–09 已立并写明阻塞边
出处：用户 2026-11-01 问「项目中有 document number 的定义吗」，2026-11-02 答「打算加一套单据编号规则」。同日经提问裁定：覆盖下表全部十一类单据；编号能力用公司平台库 `idp-platform-go` 的 `numbering`（先给它发版再接）；规则由产品内置默认、租户可改。

## 为什么要做

今天系统里没有「单据号」这个概念（取证于 main `a14f4f6b3`）：

- 运单、包裹、异常案件、清关案件、送货预报等的号是随机标识（`internal/platform/identity`，形如 `SHR-` 加 26 位），能做主键，但人念不顺、看不出日期与先后，也不按任何租户能定的格式出。
- 订舱、运输委托、审核应付、手工发布的对账单等，编号要操作者手填。
- 自动截单的对账单号从账户与截止日拼出（`STMT/<账户>/<日期>`，ADR-0339 决定三），不是按规则编的流水号。
- ADR-0143（Proposed）已为对账单定下「平台编号能力在发布事务内分配」的路线，但一直卡在平台库没有可锁定的版本。

运营与客户沟通时引用的是「单据号」：报给客户的运单号、发给客户的对账单号、客服登记的争议号、和承运商对的订舱号。这些号要按规则、按租户、不重复、开立后不变。

## 定下的几条

1. **单据号与内部标识分开。** 内部标识照旧做主键与幂等键，一个不动；单据号是另加在单据上的一项，给人看、给相对方引用。例外是对账单：它的「固定单号」本来就是单据号（GLOSSARY「对账单」），沿用 ADR-0143 的路线，新发布的对账单由编号能力分配这个号。
2. **编号能力用平台库。** `go.idp.xyz/idp-platform-go` 的 `numbering` 是公司级技术组件（CONTEXT-MAP：产品只复用有版本与兼容约束的公司级技术组件，技术组件不拥有本产品领域对象）。本仓只经消费侧适配器调它的公开 API；它的序列技术表按 ADR-0143 的方式作为独立平台 schema 接入迁移计划，资产不改写。
3. **单据号归单据所属的上下文。** 每类单据在它自己的开立事务内取号、随单据落库，开立后不可改。同一开立意图重放答原号（号存在单据上，重放读回，不再取号）；开立回滚则号码随之释放（平台库担保：已提交编号不重复、无空洞）。
4. **编号规则 = 单据类型 + 模板，产品内置默认、租户可改。**
   - 每类单据随产品带一个默认模板（产品策略，ADR-0146）。租户没登记自己的规则时就用它，开箱即用。（模板开箱即用；日期所需的时区若开立方没有，要租户先登单据编号时区，见第 11 条）
   - 租户可以为某类单据登记自己的模板（租户取值），带生效时刻；新规则只影响之后开立的单据，已开立的单据号不变。
   - 加扰序列（`{seqx}`，让人猜不出业务量）的密钥属租户取值，不进仓库、无默认；租户模板用了加扰而没配密钥，登记即拒。
5. **规则册是编号技术组件的配置。** 一张册，按（租户，单据类型）存各版模板，只认单据类型代码，不拥有任何单据。单据类型代码及其默认模板由所属上下文声明（类型与默认在同一处定义）。管理台「设置 → 单据编号」查阅、预览下一个号（只渲染不占号）、登记新规则。
6. **序列按租户与单据类型隔离**，按模板推出的周期重置（含日 → 按日，含月 → 按月，只有年 → 按年，无日期 → 不重置）。序列不跨租户，一个租户从自己的号推不出别的租户的业务量——`internal/platform/identity` 选随机数防的正是跨租户推算，与此不冲突。
7. **（租户，单据类型，单据号）唯一。** 租户换格式后渲染出的号若已被用过，取号时跳过、取下一个流水（跳过的号成空号）；连续跳过到上限仍撞，这一次开立不落单、答「编号规则撞号」，页面说人话指到规则册；登记规则时预览会提示与已用号段可能重叠。
8. **上线前已开立的单据不补号。** 页面对没有单据号的单据照旧显示内部标识，技术详情里注明「上线前开立，无单据号」。已有对账单的单号就是它的单据号，不变。
9. **原先手填编号的单据改由系统给号。** 订舱、运输委托、审核应付、手工发布对账单的表单不再让人填编号；需要幂等的地方由前端生成幂等键（同费用争议的做法），单据号由服务端在开立时分配并答回。
10. **对外呈现。** 管理台各页以单据号为主列、可按单据号检索，内部标识进技术详情。首方客户接口的运单提交答复与查询带上运单单据号（新增字段，不破坏既有契约）。
11. **日期按谁的时区。** 模板里的日期部件按开立方已登记的业务时区渲染：网点单据按网点业务时区，对账单按结算账户的业务时区；开立方没有业务时区的，按租户在规则册登记的单据编号时区（租户取值，产品不给默认）；两者都没有时单据照常开立、不带单据号。开立时刻取服务端落单时刻。（2026-11-02 评审修订：初稿写的「没有业务时区按 UTC」等于替租户定了时区，违反红线，改为此。）

## 默认编号格式（产品策略，2026-11-02 采纳，权威在 ADR-0355 决定五）

| 单据 | 上下文 | 开立时点（用例） | 类型代码 | 默认模板 | 样例 | 重置 |
|---|---|---|---|---|---|---|
| 委托（界面词：运单） | parcel-shipment | 委托已提交（UC-PS-001） | `SHIPMENT` | `P{yyyy}{mm}{dd}{seq:6}` | `P20261102000123` | 日 |
| 对账单 | settlement-accounting | 发布（UC-SA-003） | `CUSTOMER_STATEMENT` | `STMT-{yyyy}{mm}-{seq:5}` | `STMT-202611-00007` | 月 |
| 费用争议 | settlement-accounting | 登记（UC-SA-003） | `STATEMENT_DISPUTE` | `DSP-{yyyy}{mm}-{seq:5}` | `DSP-202611-00012` | 月 |
| 审核应付 | settlement-accounting | 审核形成应付（UC-SA-004） | `AUDITED_PAYABLE` | `AP-{yyyy}{mm}-{seq:5}` | `AP-202611-00031` | 月 |
| 异常案件 | visibility-exception | 立案（UC-VE-004） | `EXCEPTION_CASE` | `EX-{yyyy}{mm}{dd}-{seq:4}` | `EX-20261102-0003` | 日 |
| 客户索赔项（索赔） | visibility-exception | 客户提交被记录（UC-VE-007） | `CLAIM` | `CLM-{yyyy}{mm}-{seq:5}` | `CLM-202611-00002` | 月 |
| 关务案件（清关案件） | customs-compliance | 建立（UC-CC-001） | `CUSTOMS_CASE` | `CC-{yyyy}{mm}{dd}-{seq:4}` | `CC-20261102-0011` | 日 |
| 送站预报（送货预报） | node-operations | → 已登记（UC-NO-006） | `DELIVERY_PRE_ADVICE` | `PA-{yyyy}{mm}{dd}-{seq:5}` | `PA-20261102-00045` | 日 |
| 送站交接单 | node-operations | 服务端收下节点开立（UC-NO-006） | `DELIVERY_HANDOVER_NOTE` | `HO-{yyyy}{mm}{dd}-{seq:5}` | `HO-20261102-00018` | 日 |
| 订舱 | transport-fulfillment | 订舱申请登记（UC-TF-004） | `BOOKING` | `BK-{yyyy}{mm}-{seq:5}` | `BK-202611-00009` | 月 |
| 运输委托 | transport-fulfillment | 委托提交（UC-TF-004） | `TRANSPORT_COMMISSION` | `TC-{yyyy}{mm}-{seq:5}` | `TC-202611-00004` | 月 |

取舍：前缀两三个字母、短横分隔、日期在前，人念得出、按字面能排序；量大的（运单、网点单据、案件）按日重置，其余按月。运单不带短横，便于条码与客户系统录入。

## 不做

- 不改任何内部标识、主键或幂等语义；不给外部标识（客户单号、总单号、承运商单号等）编号——那些号由外部方分配（GLOSSARY「外部标识」）。
- 不给上线前已开立的单据补号。
- 不做打印模板、条码生成。
- 包裹不单独编单据号（客户看到的是运单号与客户自己的包裹号）；如需要另立票。

## 闸门

生产接入（子票 02 起）需平台库可被正式依赖：`idp-xyz/idp-platform-go` 合入去本地 `replace` 的改动并打 `v0.1.0-rc.1`（PR idp-xyz/idp-platform-go#1），`go.idp.xyz/idp-platform-go` 的 vanity 映射合入并在 vanity 主机部署（PR idp-xyz/idp-bento-go#5）。这两步需用户审批合并与 root 部署。ADR-0143 列的其余证据（事务合同、迁移接线、重放 / 并发 / 租户隔离 / 周期重置 / 回滚测试、CONTEXT 与 UC 同步）在子票里逐条产出。

## 子票

| 号 | 题 | 形态 | 阻塞边 | 状态 |
|---|---|---|---|---|
| [01](./issues/01-adr-glossary-context-and-use-case-handoffs.md) | ADR-0355、GLOSSARY「单据号」「单据编号规则」、六个上下文 CONTEXT 规则句、八份 UC 交接 | 纯文档（改现行规则，要非作者评审） | — | resolved |
| [02](./issues/02-platform-numbering-adoption-and-rule-register.md) | 接入平台库与编号规则册：依赖、平台 schema 迁移、取号编排、规则册存储 / 读口 / 登记口 / 预览口、十一类类型声明 | 后端 | 01；平台库发版与 vanity 部署（用户动作） | resolved |
| [03](./issues/03-admin-settings-document-numbering-page.md) | 管理台「设置 → 单据编号」 | 前端切片 | 02 | resolved |
| [04](./issues/04-shipment-numbers.md) | 委托的单据号（界面词「运单号」），含首方接口答复与查询 | 后端 + 前端 | 02 | resolved |
| [05](./issues/05-statement-dispute-and-payable-numbers.md) | 对账单、费用争议、审核应付 | 后端 + 前端 | 02 | resolved |
| [06](./issues/06-exception-case-and-claim-numbers.md) | 异常案件、客户索赔项 | 后端 + 前端 | 02 | resolved |
| [07](./issues/07-customs-case-numbers.md) | 关务案件 | 后端 + 前端 | 02 | resolved |
| [08](./issues/08-pre-advice-and-handover-note-numbers.md) | 送站预报、送站交接单 | 后端 + 前端 | 02 | resolved |
| [09](./issues/09-booking-and-transport-commission-numbers.md) | 订舱、运输委托 | 后端 + 前端 | 02 | resolved |
| [10](./issues/10-wrap-up-references-labels-and-tracking-by-shipment-number.md) | 收尾：线路规划引用委托处显示运单号、误标的运单号改回包裹号、壳层页签、客户追踪按运单号定位 | 前端为主 + 少量后端 | — | ready-for-agent |
| [11](./issues/11-supplier-bill-and-audited-payable-admin-pages.md) | 管理台补供应商账单与应付：读口、查阅页、接收与审核表单（用户 2026-11-02 裁定补全套） | 后端 + 前端 | — | ready-for-agent |
| [12](./issues/12-customer-tracking-by-shipment-number.md) | 客户追踪按运单号（委托的单据号）查：PS 按（租户，客户账户，单据号）的读、VE 消费侧适配器与编排、首方新口 `/v1/customer-shipment-tracking`、契约与演示（用户 2026-11-02 裁定，先写 ADR；出处票 10 第 4 条） | 后端 + 文档 | —（ADR-0356 已接受） | ready-for-agent |

04–09 互不阻塞，可并行；各票动到同一份 AT 盘点与同一张端点表时按并行会话的取号规矩办。
