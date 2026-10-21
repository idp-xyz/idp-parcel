# ADR-0236：租户财务、关务人员在管理台手工采用外部资金事实与监管凭证——操作者渠道「运营决定」能力面另开三个手工口，两类各一种决定、提交人记成操作证据，与集成客户端族并存

Status: Accepted（2026-10-21，Claude Code 会话代决：用户原话「要我裁定的部分，你作为业务和系统专家，请你自决吧」与「你作为业务和系统专家自决……解决所有的问题」；票 demo-chain-gaps/140，落地代决复核报告 R1 那一格的裁定「是」。**部分停用** ADR-0149 决定一、决定三各一句与 ADR-0151 决定四一句，见决定六。越权风险点见文末，待 owner 复核；两种决定种类与授予粒度待 PC owner 在角色模型（票 product-strategy-boundary/07）里会签。）<!-- 索引注记 -->｜**已接受**（2026-10-21，用户原话「你作为业务和系统专家自决」，Claude Code 会话代决；票 demo-chain-gaps/140，承代决复核 R1）。**入口**：`POST /settlement-external-funds-fact-manual-registrations`、`/settlement-external-funds-fact-correction-manual-registrations`、`/customs-regulatory-credential-manual-registrations` 三口挂操作者渠道「运营决定」能力面，两种决定 `EXTERNAL_FUNDS_FACT_ADOPTION`（首版与更正同一种）、`REGULATORY_CREDENTIAL_REGISTRATION`（access_identity 0020），谁持哪一种是租户取值，未授予答 `OPERATOR_NOT_GRANTED`；铸信封前判准入（能力 `OPERATION_DECISION`、事实类型取决定种类）。**编排与载荷**与集成客户端族那三口同一份，两族只差 Intake；集成客户端三口不改、并存。**操作证据**：提交人记成 `OPERATOR/<发行方>#<sub>` 落在采用的那一版资金事实（settlement_accounting 0041）与凭证行（customs_compliance 0026）上，只进审计、不进内容比对与任何门，重放不顶替；集成客户端与受控 CLI 两路记 NULL；两本册的读面透出。**更正不绑原采用族**：两族在同一事实键上顺序到达答 `FUNDS_FACT_CONFLICT` 或`未受理`，外部系统一侧转人工；并发那一格的缺陷归票 demo-chain-gaps/125。管理台收付款核销页两张、关务案件页一张登记签改走手工口。部分停用 ADR-0149 决定一「外部结果与外部资金事实……的提交者是外部系统」与决定三对外部资金事实、监管凭证两类的独占，ADR-0151 决定四「外部资金事实更正随原事实归集成客户端族」，适用场景只限三手工口。越权风险点含 R1 本身（owner 答否即撤三张签与三口）、首版与更正同一种授予、更正不绑族且职责分离不强制（持采用授予的人能更正银行接口送来的金额，UC-SA-005「结算人员不能更改银行事实」，去向票 product-strategy-boundary/07）、重放不留第二人、集成客户端一路不记提交方、管理台仍以 JSON 录入、凭证挂运营决定而非登记写面、两族准入是两套坐标，待 owner 复核
Date: 2026-10-21

## Context

- 现象（读码钉 `cb087e5a`，只记 `S`）：外部资金事实的采用与更正两口（`/settlement-external-funds-fact-registrations`、`/settlement-external-funds-fact-correction-registrations`）与监管凭证登记口（`/customs-regulatory-credential-registrations`）都挂集成客户端族（ADR-0149 决定三；ADR-0151 决定四把资金事实更正随原事实归同一族）。管理台收付款核销页的「采用资金事实」签（首版、更正两册）与关务案件页的凭证登记签发的是操作者令牌，到了这三口只会被集成客户端认证拒掉——三张签今天实际用不了。演示里资金事实与凭证只经受控登记 CLI 登入。
- 代决复核报告 R1 那一格裁「是」：生产上允许租户的财务、关务人员在管理台手工采用外部资金事实与监管凭证；操作者渠道另开入口，按决定种类单独授予、记提交操作者，与集成客户端族并存。理由：多数租户没有银行或报关行接口，照单人工录入是常态；单独授予加留痕把资金控制风险收在可审计范围。
- 已有的规则：SA CONTEXT 写明外部资金事实进产品只有本上下文的采用这一口，人工核实与更正也先在本上下文采用、再传出（ADR-0137 决定四）；UC-SA-005 要求外部事实采用、手工分配、核销撤销各自授权，审计至少记录操作主体。CC CONTEXT 要求监管凭证有稳定身份与不可变版本，附件不替代凭证身份。PS 已把受控关闭与重开的提交操作者记成操作证据，只进审计（ADR-0187）；TF 的收费发生项登记把登记者落成 `RegisteredBy`。
- 运营决定能力面的先例：ADR-0227（关务三口）、ADR-0204、ADR-0212——新决定种类追加进 `accessidentity` 与迁移、端点行挂 `decisionFace`、Intake 经 `operatorDecisions` 共享渠道、准入照 ADR-0149 决定四判。合成租户按真实租户对待，写开关换不了运营决定口（ADR-0150 决定三）。

## Decision

**一、两类各一种运营决定。** 操作者渠道「运营决定」能力面加两种决定：`EXTERNAL_FUNDS_FACT_ADOPTION`（外部资金事实采用，首版与更正同一种）与 `REGULATORY_CREDENTIAL_REGISTRATION`（监管凭证登记）。两种分开授予——管钱的与管关务的通常不是一个人，持其一不因此能用另一类的口，也不因持关务建案、授权或提交那几种而能登凭证。更正与首版同一种：更正是采用的一种，走同一个用例，判据同 ADR-0151 决定四「更正与首登认证与授予同一份」。有哪些种类是产品策略；谁持哪一种是租户取值，没有授予答 `OPERATOR_NOT_GRANTED`。迁移 `access_identity/0020` 重建决定种类那条 CHECK。

**二、三个手工口与集成客户端三口并存。** `POST /settlement-external-funds-fact-manual-registrations`、`/settlement-external-funds-fact-correction-manual-registrations` 按第一种认证，`/customs-regulatory-credential-manual-registrations` 按第二种认证；铸信封前读生产权威区间（能力 `OPERATION_DECISION`、事实类型取决定种类），不覆盖答 `OUTSIDE_ADMISSION_SCOPE`。载荷形状、编排、幂等与答案代数与集成客户端那三口同一份：载荷仍由各自的 `registrationjson` 译装，租户取信封，载荷自报租户即拒；采用编排、凭证登记编排与事务壳在装配点由同一套构造给出，两族只差 Intake。集成客户端三口一行不改：有接口的外部系统照旧推，没接口的由人照单录。路径带 `manual` 让两族在端点表上一眼分得开。合成租户与真实租户同一条命令面，不加隔离放行。

**三、提交人记成操作证据，只进审计。** 手工口的 Intake 把认证出的操作者记成命令上的操作证据 `OPERATOR/<发行方>#<sub>`（与 PS 操作证据同一写法：渠道前缀加发行方与 sub 成对），编排把它落在采用的那一版资金事实上（`settlement_accounting/0041`，版本行一列）与那一版凭证上（`customs_compliance/0026`，凭证行一列）。待遇照 ADR-0187：不进内容摘要、不进重放比对、不进任何门——凭证适用性判断读不到它；同一份由另一位持同种授予的人重放答已存在，留下的仍是首次那一位。首版与每次更正可能出自不同的人，每一版各记各的。集成客户端族与受控登记 CLI 两路不记提交方，列为 NULL，读作「这一版不记提交方」，不读作「无人提交」。资金事实册读面（`/settlement-funds-applications`，链头那一版）与凭证册读面（`/customs-credentials`）把它原样透出，缺席即不记。

**四、更正不绑原采用族。** 财务人员可以手工更正一条经集成客户端采用的事实，外部系统也可以更正一条手工采用的事实：更正编排只认「回指当前链头」，不看链头是哪一族采用的。理由：外部系统送错金额时，续办正是财务照银行回单更正；而集成客户端一路今天不记提交方，「按原采用族绑定」本来也没有可读的依据。

UC-SA-005 的业务规则写明「结算人员不能更改银行事实，也不能以资金映射权限创建费用、赔付或追偿金额」。手工更正不改银行事实：它采用的是同一事实的一个新来源版本，回指原版本、原版本留着，依据是银行回单、对账单这类外部记录，本上下文仍只持引用；采用与映射、核销分开授予，映射与核销今天只有受控 CLI，不与采用授予相通。但职责分离今天不强制：持采用授予的人确实能把银行接口送来的金额更正成另一个数，产品只记下是谁、不拦——这是越权风险点 3，去向是角色模型里的职责分离（票 product-strategy-boundary/07）。

两族在同一事实键（租户、`factRef`）上**顺序**到达时，答复与续办如下，外部系统那一侧都要转人工处理，不是可自动重试的答案：

- 手工先录、录错了，银行随后推来真值：同一版本字面异内容，或换了一个首版字面，都答 `FUNDS_FACT_CONFLICT`，原行不被顶替。续办是财务核对回单，以手工更正把链头改到真值。
- 手工先更正，银行随后推来回指原版本的更正：它回指的已不是链头，答 `SOURCE_NOT_ACCEPTED`（`未受理`）。续办是财务核对两份更正，需要时再以手工更正对齐。

**并发**到达时后到那份今天答已存在、被静默丢弃，那是缺陷，记在票 demo-chain-gaps/125 的 SA 同形追记，修好之后与顺序到达同答。

**五、管理台三张登记签改走手工口。** 收付款核销页「采用资金事实」签的首版、更正两册与关务案件页的凭证登记签发向三个手工口，登记输入的形状提示去掉 `tenantId`；两本册的读签多一栏「采用人」「登记人」，缺席答「未记」并点出缘由。

**六、部分停用。**

- ADR-0149 决定一「外部结果与外部资金事实（关务外部结果、监管凭证、承运商轨迹与首次有效收寄证据、外部资金事实）的提交者是外部系统」一句，对外部资金事实与监管凭证两类的适用；与决定三「外部结果与资金事实走集成客户端族」对这两类的独占。**适用场景**：只限租户人员经三个手工口手工采用这一格——理由是这一格的提交者是租户自己的人，不是外部系统；外部系统送来的那一格照旧归集成客户端族，关务外部结果、承运商轨迹与首次有效收寄证据不受影响，其余各条不变。
- ADR-0151 决定四「外部资金事实更正随原事实归集成客户端族（operator-channel/11）」一句。**适用场景**：只限手工更正口——更正也可以由持外部资金事实采用授予的操作者提交，且不绑原采用族（决定四）；交接、揽收、POD 更正与该决定其余各句不变。

**七、不裁的。** 集成客户端一路记下提交的客户端（让两族留痕对称），另立票 demo-chain-gaps/157；管理台把 JSON 录入换成逐字段表单，另立票 demo-chain-gaps/156。映射、核销与核销撤销仍只有受控 CLI 一路，本记录不给它们开在线口。职责分离（采用人不得是核对或核销人）不由本记录强制，归角色模型（票 product-strategy-boundary/07）。

## 候选与反方

**甲 · 管理台后端作为一台集成客户端代发。** 反方：客户端凭据要落在管理台一侧，所有手工采用都记成同一台客户端，人不留痕；共享凭据正是 ADR-0149 否决丙的那种形态。否决。

**乙 · 既有三口改成双族（同一路径既认客户端凭据又认操作者令牌）。** 反方：端点表一行一族（ADR-0222 决定三的装配门禁按行声明的族核对），一行两族就要破例；两族的答复格（`INTEGRATION_CLIENT_*` 与 `OPERATOR_*`）与授予坐标（事实类型与决定种类）在一口里混答，持令牌的人分不清该去哪本册登授予。否决。

**丙 · 挂登记册配置写面（按登记册授予）。** 反方：采用一版资金事实形成生产事实——它随即交出采用信封，CC 的税费付款核对就等它；登记写面不判准入范围，授予按登记册，能配价卡的人会因此能采用收款（ADR-0151 否决甲的同一理由）。凭证虽在 CC 叫「登记」，它是外部监管事实的一版，不是租户怎么配置。否决（越权风险点 7）。

**丁 · 挂作业事实登记面。** 反方：要求设备在册、事实身份由设备签发，管理台上的财务、关务人员没有设备，这也不是现场作业事实。否决。

**戊 · 只开资金事实，凭证仍只走集成客户端。** 反方：多数租户的报关行同样没有接口，凭证原件在关务人员手里；R1 两类都裁「是」。否决。

**己 · 资金事实首版与更正分两种授予。** 反方：更正是采用的一种、走同一个用例，分开授予就会出现「能登不能改」；改金额的风险若要四眼，那是角色模型的一格，不是另一种决定。否决（越权风险点 2）。

## Consequences

- 生产进程有了租户人员手工采用外部资金事实与登记监管凭证的路：持对应授予的操作者经三口采用、更正、登记，采用信封照旧交给 CC，版本行与凭证行记下提交人。`cmd/parcel-api` 的真库用例经生产装配与真操作者册证三口的答复格、操作证据、两种授予互不相用与集成客户端三口照旧可用；2026-10-21 在一次性库上按 `seed.sh` 灌入、起 `parcel-api`，用配置员的 Dex 令牌走通三口与重放，查阅员三口都答 `OPERATOR_NOT_GRANTED`，合成银行与合成报关服务商的客户端令牌经原两口照旧采用（实测，只记 `S`）。
- 迁移 `access_identity/0020`（决定种类 CHECK）、`settlement_accounting/0041`（资金事实版本的操作证据列）、`customs_compliance/0026`（凭证行的操作证据列）。
- SA CONTEXT「外部资金事实进入本产品只有本上下文的采用这一口」那条补人工采用与操作证据；CC CONTEXT「监管凭证、限制与处置」加一条两路登记与操作证据；UC-SA-005 加 `AT-SA-200`、`201`，UC-CC-003 加 `AT-CC-446`、`447`。
- 演示种子：配置员加两种授予，准入对照与权威区间各两行（步 `470`）。
- 立票 demo-chain-gaps/156（逐字段表单）、157（集成客户端一路记提交方）。

## 越权风险点

1. **R1 本身**（owner）。若 owner 答否：撤掉管理台三张登记签（改回只读、登记走受控 CLI），撤三个手工口与两种决定种类的授予；两列操作证据可留着，全为 NULL。
2. **首版与更正同一种授予**（PC owner，票 07）。改金额比首次采用风险高，若要求更正单独授予或四眼复核，加一种决定、更正口改认它即可。
3. **更正不绑原采用族，职责分离不强制**（SA owner、PC owner）。UC-SA-005「结算人员不能更改银行事实」：手工更正形成的是新来源版本、不改写原版本，但持采用授予的人确实能把银行接口送来的金额更正成另一个数，产品只留痕、不拦。去向：职责分离归角色模型（票 product-strategy-boundary/07），例如更正一条经集成客户端采用的事实须第二人复核，或手工更正口只认手工采用的链头；若 owner 认为外部系统送来的事实只能由外部系统更正，要在更正编排里判链头的提交族——先要集成客户端一路记下提交方（票 157），否则无从判起。两族顺序到达时的答复与续办见决定四，外部系统那一侧都转人工。
4. **重放不留第二人**（SA、CC owner）。同一份由另一人重放答已存在，第二人只在接入日志里；若审计要记每一次尝试的提交人，另立尝试册，不改采用行。
5. **集成客户端一路不记提交方**（SA、CC owner）。两族留痕不对称：手工那一路记人，外部系统那一路记 NULL，能从册上看出「这一版是不是人工录的」，看不出是哪台客户端送的。票 157。
6. **管理台仍以 JSON 录入**（产品）。财务、关务人员要照键名填 JSON；上线前若试点租户的录入量不小，票 156 要提前。
7. **凭证挂运营决定而不挂登记写面**（CC owner）。若 owner 认为凭证属租户的主数据登记，改挂登记写面只换端点行的族与 Intake，代价是准入范围那一格对它不再起作用（登记写面不判准入）。
8. **两族准入是两套坐标**（PG owner）。手工口判 `OPERATION_DECISION` + 决定种类，集成客户端口判 `EXTERNAL_FACT` + 事实类型，阶段治理要为同一类事实登两份区间；若 owner 要按事实类型统一坐标，改认证方递的那一格即可。
9. **读面原样透出发行方与 sub**（PC owner）。操作证据是 `OPERATOR/<发行方>#<sub>`，不译姓名——产品不存姓名（敏感实例外置）；读的人要认出是谁，回操作者册按主体对。

## Links

- [ADR-0149](./0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md)：本记录部分停用其决定一一句与决定三对外部资金事实、监管凭证两类的独占（决定六）；复核记录 R1 指向本记录所承的票
- [ADR-0151](./0151-unassigned-command-faces-get-their-families.md)：运营决定能力面（决定一、三）；本记录部分停用其决定四「外部资金事实更正随原事实归集成客户端族」一句（决定六）
- [ADR-0150](./0150-synthetic-tenant-is-treated-as-a-real-tenant-and-isolated-form-retires-per-face.md)：合成租户按真实租户、写开关换不了运营决定口（决定三）
- [ADR-0187](./0187-controlled-closure-and-reopening-record-request-and-operation-evidence-on-the-decision.md)：操作证据只进审计的待遇（决定三沿用）
- [ADR-0137](./0137-customs-gate-judgments-are-registered-facts-driven-by-assessment-requests-payment-gate-rule-is-registered-and-funds-facts-are-minted-only-in-settlement-accounting.md)：外部资金事实只经 SA 采用（决定四），本记录不改
- [ADR-0227](./0227-customs-case-and-declaration-submission-enter-production-through-operator-decision-faces.md)、[ADR-0204](./0204-a-fulfillment-segment-adopts-a-transport-commission-by-declaration-and-a-started-commission-cannot-be-cancelled.md)、[ADR-0212](./0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md)：新运营决定口的先例
- [ADR-0222](./0222-composition-roots-register-endpoints-and-routes-per-feature-file.md)：三口登在 `cmd/parcel-api/endpoints_manual_external_facts.go`
- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：机制、产品策略与租户取值三分
- [UC-SA-005](../application/settlement-accounting/UC-SA-005-MAP-EXTERNAL-FUNDS-AND-APPLY-SETTLEMENT.md)：`AT-SA-200`、`201`；[UC-CC-003](../application/customs-compliance/UC-CC-003-ASSESS-DECLARATION-READINESS.md)：`AT-CC-446`、`447`
- 票 [demo-chain-gaps/140](../../.scratch/demo-chain-gaps/issues/140-operators-adopt-external-funds-facts-and-regulatory-credentials.md)；代决复核报告 [owner-review-20261020](../../.scratch/owner-review-20261020/report.md) R1；后继票 [156](../../.scratch/demo-chain-gaps/issues/156-admin-web-manual-external-fact-forms-replace-json-entry.md)、[157](../../.scratch/demo-chain-gaps/issues/157-integration-client-adoptions-record-the-submitting-client.md)
