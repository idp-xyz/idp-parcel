# ADR 草案：对客报价进入产品范围，报的是一套价而不是逐票价格承诺——报价对象归 `party-commercial`，价格内容是 `parcel-pricing` 草稿册里的一份销售价卡草稿；客户接受之后经价格生效申请进入现有的批准发布流程，报价自己不发布任何东西；逐票报价与锁价仍不做

Status: 草案，未编号，未经评审（2026-11-11，Cursor 会话起草；票 [commercial-four-module-requirements/08](./issues/08-adr-draft-quotation-review-numbering-and-acceptance.md) 负责评审、编号与接受。放在 `.scratch` 里是因为它还不是规则：下面「对已接受记录与现行文档的作用」列的每一处都没有改。起草能力边界与越权风险点见文末。）
Date: 2026-11-11

## Context

**现行规则说不做报价。** 取证钉 `origin/main` 的 `11bfa1ffb`：

- `docs/product/PILOT-SCOPE.md`：「首发不对客户形成价格承诺：报价有效期、锁价和报价接受不进入首发。」
- `docs/domain/parcel-pricing/CONTEXT.md`：「**正式报价接受、锁价及 Quote 生命周期仍不进入首发**，它们要求承诺一个计价参考序列期次并处理跨期差价归属，须待 `SELL` 侧业务责任落实后另行确认。」同一份文档「计算目的」词条末尾还有一句「首发不做对客报价」。
- `docs/application/README.md`：「它不单独建立报价、合同或账务用例。后续只有在确认独立报价接受、锁价或外部计价服务责任后，才新增相应 `UC-PP-*` 用例。」
- [ADR-0012](../../docs/adr/0012-parcel-pricing-context-within-idp-parcel.md) Consequences：「正式 Quote 生命周期、完整多币种指数体系和任意脚本扩展延后到有业务证据时再决策。」
- [ADR-0152](../../docs/adr/0152-operator-estimate-is-a-pricing-use-case-computed-not-recorded-and-never-handed-to-settlement.md) 把运营试算定为只算不存，并写明：「需要留痕的是报价——将来立报价对象时由它自己留，不是把试算改成入册。」

**新的业务依据。** 2026-11-11 用户在会话里给出一张四个模块的要求清单，并说明它是客户或招标的要求。Quotation 一行原文是「报价创建、审批、发出、客户接受与价格生效申请」。这张表在仓库与历史会话里查不到出处，本草案只凭这一行字。

**这一行字指向哪一种报价。** 报价有两种形态，差得很远：

- 逐票报价：对一票货算出一个金额，带有效期，客户接受后按这个金额或按某个时点重算来计费。
- 报一套价：给一位客户报一套销售价格，客户接受后这套价成为他的合同价。

要求里写的是「客户接受与价格生效申请」。接受之后要申请的是「价格生效」，不是下单或锁价，这是第二种的流程。`PILOT-SCOPE.md` 同一节也写着「客户仍按其合同价结算」。

**行业头部软件也把这两种分成两个对象。** 没有可以去确认的对方，2026-11-11 会话查了公开文档，逐条记在[行业做法笔记](./industry-practice-notes.md)：SAP TM 里，针对一次运输的报价被接受后生成订单，针对一段期间费率的协议报价被接受并签约后变成协议；Salesforce 与 Conga 的报价签约后生成这位客户的合同价；易仓 TMS 的客户协议价审核后才生效，它按运单的「运费报价申请」是事后改价。要求里的措辞对上的是协议级这一种。

**上面那两个难题只属于第一种。** 计价 `CONTEXT.md` 点名的「承诺一个计价参考序列期次」与「跨期差价归属」，都因为要对某一票的金额作承诺才出现：报价时点与计费时点不同，燃油与汇率的期次会变，申报重量与实测重量也会不同。报一套价不承诺任何一票的金额，生效之后照今天的规则逐票计费，这两问不出现。

**代码里已经有什么。**（同一取证点）

- `parcel-pricing` 有价卡草稿册：`PriceCardDraft` 四格状态，`已校验`的草稿带着过了构造门的方案与内容摘要；批准（`Approve`，按 `PriceCardApprovalDutyRule`）之后经 `PublishPriceCardDraftHandler` 交既有的价卡登记。
- `party-commercial` 有待批准发布：`SubmitPublicationDraft`、`Approve`（按 `ApprovalDutyRule`）、发布；批准与发布绑定调用方看到的内容摘要（[ADR-0314](../../docs/adr/0314-pending-publication-drafts-are-readable-back-by-the-operator-and-approval-and-publication-are-bound-to-the-content-digest-the-caller-saw.md)）。客户合同与商业价格政策都是这条流程发布的对象。
- 一位客户用哪张销售价卡，今天由两件事定：他的结算账户登记的计价范围，与这个范围上在用的 `SELL` 价卡（[ADR-0190](../../docs/adr/0190-a-parcel-billing-moment-requests-a-sell-evaluation-that-forms-the-estimated-customer-charge.md)、[ADR-0280](../../docs/adr/0280-a-settlement-account-registers-one-sell-pricing-scope-across-the-estimate-and-billing-moment-adoptions.md)）。评价按范围选卡，不读合同或价格政策的绑定（[ADR-0260](../../docs/adr/0260-an-evaluation-selects-its-price-card-by-scope-and-reads-its-pricing-terms-only-from-the-card.md)）。
- 没有任何报价对象；在 `internal`、`cmd`、`apps/admin-web/src` 的非测试代码里检索 `quotation` 没有业务命中。
- 已登记价卡与价卡草稿都读不回成工作簿，也就导不出一份对客的价格表（[ADR-0349](../../docs/adr/0349-the-price-card-import-template-advances-to-ppt-2-and-its-structured-workbook-is-a-second-carrier-read-through-the-same-path.md) 决定八「已登记价卡反推成模板」一条）。

## Decision

**一、报价进入产品范围，报的是一套价，不是逐票价格承诺。**（产品策略）

- 报价是运营企业的责任法人向一个明确相对方提出的一套销售价格，加有效期与商务条款。相对方是业务参与方，可以还没有货主客户账户。
- 报价不承诺任何一票的金额。生效之后的计费与今天完全一样：按范围选卡，按计费时点取参考序列期次。
- 逐票报价、锁价、把报价金额当作费用或接受前控制金额，照旧不做。计价 `CONTEXT.md` 点名的那两问留给将来的逐票报价，本记录不答。

**二、报价对象归 `party-commercial`，价格内容归 `parcel-pricing`。**（机制）

- 报价带相对方、责任法人、有效期、条款与接受，是商业关系上的事实，归 `party-commercial`。
- 价格内容是 `parcel-pricing` 草稿册里一份`已校验`的 `SELL` 价卡草稿。报价按方案标识、方案版本、规范化版本与内容摘要引用它，不复制价格。
- `party-commercial` 经消费侧适配器读 `parcel-pricing` 的草稿读口（[ADR-0025](../../docs/adr/0025-cross-context-adapters-live-on-the-consumer-side.md)）。`parcel-pricing` 不知道报价。
- 报价不是商业版本：任何解析读不到它。它的生命周期是提出、接受或过期，不是版本演进，照 [ADR-0093](../../docs/adr/0093-channel-account-use-authorization-is-not-a-commercial-version.md) 的判据不进商业版本那个封闭集合。

**三、状态封闭，过期读时派生。**（机制）

- 状态：`待批准`、`已批准`、`已发出`、`已接受`、`已拒绝`、`已撤回`。录入即`待批准`，与待批准发布同一做法。
- `已过期`不是存下来的状态。它是读时按有效期终点派生的结论，与「在用序列版本是派生的结论，不是一个可维护的指针」同一纪律。
- 内容固定在批准那一刻：所引价卡草稿的内容摘要、相对方、责任法人、有效期、条款。批准前可按同一报价版本重新录入替换；批准后换内容另起报价版本，旧版本保留。
- 所引价卡草稿在 `parcel-pricing` 那边被替换、摘要变了，报价的批准、发出、接受与价格生效申请一律答「所引价格已变更」，不放行。
- 同一报价至多一个版本被接受。接受必须指名报价版本与调用方看到的内容摘要。

**四、批准与录入分立，按租户的报价审批职责规则裁。**（机制；规则的取值是租户取值）

- 录入者与批准者的身份都从操作者信封来，都记且可比。两人须否不同、批准者须持哪一格授予，由租户登记的规则定；没登记不放行，不写死双人，也不写死单人。
- 这条规则与商业版本、价卡各自的审批职责规则不共用：租户对三者的要求可以不同。
- 按折扣或毛利分级的批准不在本记录。

**五、发出是一条登记的事实，首个方式是导出对客价格表。**（产品策略）

- 发出记下：谁、何时、对哪个相对方、发出的是哪一版的内容摘要、经什么方式。它不是送达，更不是接受。
- 首个内置方式是导出对客价格表：由所引价卡草稿读回成工作簿，再写成文件，文件在系统外交给客户。它依赖价卡读回口（票 [01](./issues/01-registered-price-card-reads-back-as-an-editable-workbook.md)）。
- 推送类方式等对客通知的出向连接器有实现之后另加（[ADR-0142](../../docs/adr/0142-first-party-webhooks-are-a-product-owned-outbound-channel.md)），不在本记录。

**六、接受是相对方的决定，入口两个。**（机制）

- 运营代录：必须带接受证据引用，即客户书面确认在受限证据库里的索引。登录操作人记为代录人，不是决定方。
- 首方客户渠道的命令：只对已经是货主客户账户的相对方开，账户取自凭据，不取自载荷（[ADR-0139](../../docs/adr/0139-first-party-customer-api-channel-is-a-product-owned-access-channel-family.md)）。
- 接受的门：报价版本是`已发出`；接受时刻不晚于有效期终点；内容摘要与调用方看到的一致。过了有效期的不收，答`已过期`；要继续就发新版本。
- 拒绝同样登记，原因可缺。

**七、价格生效申请不自己发布任何东西，也不立第二套批准。**（机制）

- 接受之后由运营发起。申请把这份报价要生效所需的各件送进各自现有的流程，并回指报价：
  - 所引价卡草稿，进 `parcel-pricing` 的批准与发布。价卡审批职责规则与新规范化版本的部署开关（[ADR-0352](../../docs/adr/0352-registering-a-price-card-in-a-canonicalization-newer-than-the-base-one-needs-an-explicit-deployment-switch-that-is-off-by-default.md)）照旧适用。
  - 客户合同版本与销售方向的商业价格政策版本，定价方案绑定指向这份方案，进 `party-commercial` 的待批准发布。
  - 相对方还没有货主客户账户、结算账户，或结算账户还没登记销售计价范围时，这些是既有登记口上的租户取值。申请只列出缺哪几件，不代登，不给默认。
- 「价格已生效」是读时派生的结论：上面各件都已发布并生效，登记也齐了。任何一件没到，如实答停在哪一件。
- 同一报价版本至多一份申请，申请可重放。

**八、不改按范围选卡。**（机制）

- 评价照旧按范围选卡，不读报价，也不读绑定（ADR-0260 不动）。
- 客户专属价的形态不变：这位客户的结算账户登记的计价范围，加这个范围上的 `SELL` 卡，一户一范围（ADR-0280）。报价所引草稿的计价范围因此必须是给这位客户用的那个范围。
- 申请时这个范围上已有同时适用的在册版本的，如实提示（票 [02](./issues/02-price-card-preview-and-publication-show-overlap-with-registered-versions.md)）；收尾旧版照 [ADR-0234](../../docs/adr/0234-an-older-price-card-version-leaves-new-selections-only-through-an-explicit-closure-for-a-named-successor.md) 由人做，申请不代做。

**九、不做的，逐条写明。**

- 逐票报价、锁价、报价金额成为费用或接受前控制金额。
- 按运单的事后改价。它改的是一笔已经形成的费用，归费用调整，不归报价。
- 试算留痕，以及把一次试算直接转成报价。ADR-0152 一个字不改。
- 运营企业对外挂出的公布标准价。计价 `CONTEXT.md`「计算目的」词条已经认出这个用途，另议。
- 销售线索、商机与跟进记录。
- 按折扣或毛利分级的批准。接单与择优不设毛利门槛是 [ADR-0366](../../docs/adr/0366-margin-is-neither-an-acceptance-nor-a-selection-gate-and-transit-time-is-an-eligibility-constraint-not-a-ranking-dimension.md) 的立场，报价批准要不要看毛利另议。
- 采购一侧向供应商询价、比价与授予。`PILOT-SCOPE.md` 已写不建设。

**三类。** 报价对象、状态、各道门与读时派生是**机制**。报的是一套价、发出的首个内置方式、过期的判法是**产品策略**。报价审批职责规则、谁可以代录接受、每份报价的价格、有效期与条款是**租户取值**，没登记答未配置。演示租户用 `SYN-` 相对方与 `SYN-` 报价走通，证据只记 `S`。

## 对已接受记录与现行文档的作用

本草案接受时同一笔改下面各处。草案阶段一处都没改。

- `docs/product/PILOT-SCOPE.md`：「首发不对客户形成价格承诺：报价有效期、锁价和报价接受不进入首发。」一句，改为只排除逐票价格承诺与锁价，并写明对客报一套价的报价、接受与价格生效申请归本记录。这一句进的是首发试点范围，还是试点范围之外的产品能力，由用户定（越权风险点 1）。
- `docs/domain/parcel-pricing/CONTEXT.md`：Rules 里「**正式报价接受、锁价及 Quote 生命周期仍不进入首发**」一句收窄到逐票报价，并补一句：对客报一套价的报价对象归 `party-commercial`，本上下文只提供被引用的价卡草稿，评价照旧不自动形成客户承诺。「计算目的」词条末尾「首发不做对客报价」改为「首发不做逐票对客报价」。「评价对象」词条的「避免使用：报价单、锁价结果」不动：评价不是报价。
- `docs/application/README.md`：「它不单独建立报价、合同或账务用例。」一句不动，`parcel-pricing` 仍不立报价用例；补一句报价用例归 `party-commercial`，并登新用例。
- `docs/domain/party-commercial/CONTEXT.md`：Language 加「报价」「报价版本」「价格生效申请」；Rules 加决定三至七各条；Lifecycles 加报价一节；Boundaries 写明拥有报价，不拥有所引价卡草稿。
- `docs/domain/CONTEXT-MAP.md`：`party-commercial` 的「拥有」加报价；关系约束加一条 `parcel-pricing → party-commercial`（价卡草稿读口），关系图加一条边。
- `docs/domain/GLOSSARY.md`：加「报价」。「价格评价」词条的「避免使用：……报价承诺」不动。
- `docs/product/PILOT-PARAMETER-REGISTER.md`：加租户取值「报价审批职责规则」一行，号在落文时取。
- ADR-0012：本记录回答它 Consequences「正式 Quote 生命周期……延后到有业务证据时再决策」一句，只答报一套价这一种形态，无条款停用。照 `docs/adr/README.md` 顶部的写法，在它 `Status` 行的索引注记里加一条来向指针。
- ADR-0152：无条款停用。它写的「将来立报价对象时由它自己留」，本记录就是那个报价对象。
- `.scratch/launch-line-freeze-2026-10-27.md`：按「新发现怎么记」，在「记录」追加一行，写明用户点名与本规格的位置。

## Consequences

- `party-commercial` 多一个对象、一本册与一组运营命令面；管理台多报价的列表、详情与录入。
- `party-commercial` 第一次读 `parcel-pricing`：多一只消费侧适配器，读草稿的身份、状态与内容摘要。方向与今天的 `party-commercial → parcel-pricing` 相反，两条都只读。
- 报价的批准之外，价卡发布与合同发布各自还有一道批准。租户想简化，改的是各自的审批职责规则，不是让报价绕过它们。
- 对客的价格表从价卡草稿导出，所以它与将来生效的那张价卡是同一份内容，摘要对得上。
- 首方客户渠道多一个命令面；相对方还不是货主客户账户时只能由运营代录。
- 逐票报价仍然没有。客户问某一票多少钱，运营照旧用计价试算回答，试算不留痕、不是承诺。

## Alternatives considered

- **逐票报价，即参考设计里的 Quote。** 要先回答计价 `CONTEXT.md` 点名的那两问，还要处理报价金额与正式费用的差额；而「试算对象的评价不得进入结算」是现行规则。要求里写的是价格生效申请，不是锁价。头部运输管理软件里它是另一个对象，终点是一张订单（[行业做法笔记](./industry-practice-notes.md) R1、R4）。不取，留给将来。
- **报价对象放在 `parcel-pricing`。** `docs/application/README.md` 预设过这个去处。但报价带相对方、有效期与接受，而计价的规则是评价不自动形成客户承诺，也不拥有合同与商业适用性。不取。
- **新立一个限界上下文。** 参考设计是这样切的。报价的产物全部落在 `party-commercial` 与 `parcel-pricing` 现有的流程里，自己的生命周期很短；ADR-0012 已经否决过照搬参考设计的上下文切法。不取。
- **把报价做成商业版本的一类。** 它的生命周期不是版本演进，进那个集合就要为它扭曲发布、生效、收尾各道门。不取。
- **接受之后一键生效，由报价直接发布价卡与合同。** 这是第二套批准与发布口径，价卡审批职责规则与部署开关都会被绕过。不取。
- **不立报价对象，只做导出价格表。** 覆盖得了「发出」，覆盖不了要求里的批准、接受与生效申请的留痕。不取。

## 起草能力边界与越权风险点

**读过**：计价与参与方与商业两份 `CONTEXT.md`、`CONTEXT-MAP.md` 的全文（读的是工作树上钉 `871f3cdd4` 的旧版），加它们到 `11bfa1ffb` 的改动行（每行只读了开头一段）；`PILOT-SCOPE.md`、`docs/application/README.md` 里上面引的句子（取自 `11bfa1ffb`）；ADR-0012 全文、ADR-0152 全文、ADR-0260 的 Decision（这三篇在两个取证点之间没有改动）、ADR-0349 决定八那一条；ADR-0190、0234、0280、0352、0361、0362、0364、0366 的标题与各项决定的首句；代码里 `party-commercial` 的待批准发布领域与客户合同正文、`parcel-pricing` 的价卡草稿、批准、发布与登记、`settlement-accounting` 的销售评价请求规格（文件读的是旧版，引到的符号都在 `11bfa1ffb` 上逐个核过还在）。

另读过[行业做法笔记](./industry-practice-notes.md)所列的公开文档，各条的取法与状态写在那里。

**没读**：首方客户渠道的实现与 ADR-0139 全文；ADR-0142 全文；ADR-0126 与 ADR-0314 全文（只读了它们在代码里的落点）；结算账户销售计价范围采用册的实现；参数登记册全文；仓配履约上下文；`11bfa1ffb` 上 `party-commercial` 相对旧版新增的往来单位资料与识别信息各册。

**越权风险点**：

0. 形态是从要求清单的一行措辞读出来的。没有可以去确认的对方；用户要求参考行业头部软件，[行业做法笔记](./industry-practice-notes.md)支持这一读法，但它仍然没有经过任何一位真实客户确认。日后有客户要的是逐票报价，决定一就不成立，整篇要重写。
1. 进不进首发试点范围是产品范围取舍，归用户。
2. 报价归 `party-commercial`、并新增一条它读 `parcel-pricing` 的依赖，是上下文边界取舍，归两个上下文的 owner。
3. 运营代录接受：证据要求与「代录人不是决定方」借的是 [ADR-0116](../../docs/adr/0116-source-data-amendment-is-an-authorized-action-and-contract-delegation-resolves-the-actual-decider.md) 的分法，但那篇管的是资料修订授权，类比不等于适用。谁可以代录，是一条租户取值，还是授权规则里的一个新动作，没有裁。
4. 决定七让价格生效申请不设自己的批准。客户要求里的「申请」若本身要一道独立审批，与决定七相抵。
5. 过期读时派生：有效期终点按哪个业务时区、两个系统的时钟不一致时怎么判，没有裁。
6. 相对方可以还不是货主客户账户，首方渠道的接受因此只覆盖一部分相对方。
7. 报价审批职责规则另立一条，它的登记入口与所需的授予面没有裁（参见 [ADR-0362](../../docs/adr/0362-the-commercial-publication-approval-duty-rule-is-registered-through-the-controlled-commercial-cli.md)、[ADR-0363](../../docs/adr/0363-operators-and-grants-are-administered-online-through-an-operator-access-administration-face.md)）。
8. 决定二把价格内容钉在一份价卡草稿上。草稿册今天一版一行、批准前可被替换；一份草稿被多份报价引用、或被直接批准发布而绕过报价时怎么办，没有裁。

## Links

- [规格](./spec.md)、[逐条对照](./compliance-matrix.md)、[行业做法笔记](./industry-practice-notes.md)
- [小包计价上下文](../../docs/domain/parcel-pricing/CONTEXT.md)、[参与方与商业上下文](../../docs/domain/party-commercial/CONTEXT.md)、[领域上下文地图](../../docs/domain/CONTEXT-MAP.md)
- [首发试点范围](../../docs/product/PILOT-SCOPE.md)、[应用用例 README](../../docs/application/README.md)
- ADR-0012、0025、0093、0116、0139、0142、0152、0190、0234、0260、0280、0314、0349、0352、0362、0363、0366（链接见正文）
