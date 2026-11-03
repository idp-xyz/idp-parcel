# ADR-0358：供应商预期成本记下所依协议的供应商，匹配门核账单的供应商与之相同

Status: Accepted（2026-11-02，Claude Code 会话代 SA owner 决（代决）：用户 2026-11-02 经会话在「预期成本记下供应商」「匹配时经往来单位与协议方核对」「暂不补」三案里裁定「预期成本记下供应商」——形成预期成本时从协议取出供应商一并记下、之后不变，登记表单按所选供应商筛，匹配时在结算上下文内直接比对、不对就拒；票 document-numbering/14。取舍由作者落笔，越权风险点待 SA owner 复核。部分停用 [ADR-0340](./0340-supplier-bill-reception-and-audit-are-two-operator-decisions-and-each-records-the-operator.md) 决定一一句、[ADR-0200](./0200-buy-evaluation-forms-the-supplier-expected-cost-through-its-request.md) 决定四一句与决定五一句，回答 ADR-0340 越权风险点四里点名别家预期成本那一种误选。）<!-- 索引注记 -->｜**入口**：预期成本只记它依的那一版协议，没有供应商；登记表单不按供应商筛，匹配门不核供应商，误选一次就把账单对到别家的预期成本。**取法**：形成首版与纠错版本时由结算经消费侧适配器问 party-commercial「这一版协议的供应商是谁」、记在这一版上，失效纠错版本照搬；匹配门对点名了预期成本的四种分类先核账单的供应商与之相同，不同答 `EXPECTED_COST_SUPPLIER_MISMATCH`、没记答 `EXPECTED_COST_SUPPLIER_UNRECORDED`，都不落账单；迁移只加可空一列、不跨上下文回填，旧行经补记口按协议补；读口带出供应商，管理台按所选供应商筛。
Date: 2026-11-02

## Context

取证于票 document-numbering/13 分支 tip `4bcdc59f7`（已含票 11、票 13 与 main `3b4720283`），只记 `S`。

- 预期成本的领域类型 `SupplierExpectedCost`、表 `settlement_accounting.supplier_expected_cost`（迁移 0008）与读口行 `ports.SupplierExpectedCostCatalogueRow` 上都没有供应商，只有它依的那一版供应商协议（`agreement_ref`，两段式指称「对象/版本」）。
- 协议是哪家供应商的，只在 party-commercial 一处：`party_commercial.supplier_agreement` 每一版一行，`supplier_party_id` 非空；PC CONTEXT「发布后正文不可覆盖；变化形成新版本」。同一协议对象的后继版本另有正文，PC 没有规则禁止它声明另一家供应商。
- 预期成本只由派发进程形成：BUY 评价已记录 → 按回指读回 BUY 请求 → `FormSupplierExpectedCostHandler` 以请求上的发生项、费用项目与协议形成首版或纠错版本（ADR-0200、ADR-0232 决定八）。请求上的协议取自运输收费发生项（ADR-0208 决定二）；发起请求时结算已经经消费侧读口问过 PC 那一版协议的适用范围（ADR-0208 决定四，`SupplierAgreementScopeView`）。没有任何外部载荷形成预期成本。
- 运输收费发生项另有「服务提供方」（TF `ServiceProviderReference`：伙伴或自营法人），说的是谁提供运输，不是协议的对方。
- 匹配门 `MatchBillLine` 核分类与金额、币种，不核供应商；接收编排先判点名的版本是不是当前（`EXPECTED_COST_NOT_CURRENT`，ADR-0232 决定七）。ADR-0340 越权风险点四：「逐行裁决由录入的人给。」——匹配分类与点名的预期成本由人判定后写进载荷，产品不替人做匹配。票 11 的管理台登记表单因此只按当前版与币种筛预期成本，并提醒录入的人自己核协议（UC-SA-004「给开发的交接」那一行缺口）。
- 账单的供应商（`SupplierPartyReference`）与 PC 协议的供应商是同一种标识：往来单位的标识（演示里 `SYN-PARTY-CARRIER-TRUNK-01` 既是协议 `SYN-SUPPLIER-TRUNK-CN-SG-01` 的供应商，也是接收载荷的 `supplier`）。
- 本仓迁移按上下文分 schema，没有一份迁移读别的上下文的表。

## Decision

**一、供应商在形成时问协议的所有者，记在这一版预期成本上。**

- 形成首版与纠错版本时，结算按这一版所依的那一版协议，经消费侧适配器问 party-commercial「这一版协议的供应商是谁」，把答到的往来单位标识记在这一版成本上（`SupplierExpectedCost.Supplier`）。party-commercial 新开一个只交供应商的窄读口（`ports.SupplierAgreementSupplierView`），与 ADR-0208 决定四只交范围的那一口同形：按两段式指称比对，不另写分隔符。
- 失效纠错版本照搬被纠正那一版记下的供应商：它的协议原样带过去，计价结果标明不再成立（ADR-0232 决定七），没有新的协议可问。
- 不由形成载荷带入。形成没有外部载荷，命令由评价请求组成，请求上只有协议引用；让请求或发生项另带一份供应商，就是同一事实的第二处，还得再问一次 PC 才知道它对不对。问一次、记问到的，就只有一处权威。
- 不拿发生项上的服务提供方顶替：它可以是自营法人，说的是谁运，不是谁按协议开账单。
- 记下后不改。已发布协议版本的正文不可覆盖，同一版协议此后问几次都是同一个答案；协议后来终止、收尾或有了后继，都不改写这一版成本，判据同 ADR-0208 决定四「不核协议此刻是否生效」。
- 协议换供应商：成本的每一版记的是它自己所依那一版协议的供应商。纠错版本按新评价整组重述协议引用（ADR-0067 决定二），供应商跟着协议引用重新问、重新记；被纠正那一版有了后继，旧供应商点名它答不是当前版（ADR-0232 决定七），新供应商对纠错版本照常匹配。供应商不是计价结果，不在 ADR-0067 决定二那一组里；也不在决定三「纠错不动的只有身份四件」里——它跟着协议引用走。与 ADR-0067 无条款停用。
- 读口的三格同 ADR-0208 决定四：读不动答未决（原因 `SUPPLIER_AGREEMENT_VIEW_UNAVAILABLE`），整笔回滚等重投；PC 不认这一版协议、指称比中不止一版、答出的值译不过来，都是确定性的两侧矛盾，上抛为发布失败，不重投，不替它挑一家。形成的次序：评价采用、发生项锁下的成本状态（失效答失效）都判过之后、构造成本之前才问——已失效的那一版用不着供应商。
- 供应商不进预期成本的内容比对（同一版本身份的重放 / 冲突判据）：它由协议定，协议已在比对里；放进去，本记录之前形成、没记供应商的那一版与重投新形成的同一版就会被判成冲突。

**二、匹配门核供应商，不同就不接收。**

- `MatchBillLine` 在点名了预期成本的四种分类上（已匹配、量差、价差、重复计费）先核供应商：这一版成本记下的供应商与账单的供应商相同，才往下核币种与金额。没有对应发生项不点名成本，不核。
- 不同：不落账单、不交意图，答 `EXPECTED_COST_SUPPLIER_MISMATCH`（`200`），带出这一行、点名的版本与那一版记下的供应商。续办是改选这家供应商的预期成本，或改判为没有对应发生项。
- 那一版没记供应商（决定三的旧行）：同样不落，答 `EXPECTED_COST_SUPPLIER_UNRECORDED`（`200`），带出这一行与点名的版本。续办是先补记（决定三），不是改单——两格的恢复动作不同，分开立（ADR-0029）。
- 次序：点名的版本先判在不在册（不在答未受理）、是不是当前版（`EXPECTED_COST_NOT_CURRENT`），再判供应商，再判币种。供应商排在币种之前：别家的成本谈不上换算，先答「换算依据未配置」会把一次录错说成租户没登参数。
- 已接收的账单不重判：同一主张同一版本重放照旧答原结果（AT-SA-092）；本记录之前接收的匹配不改写，审核照 ADR-0357 走。
- 与 ADR-0357 的关系：匹配不过就不落账单，也就到不了审核；已匹配金额的定义、按范围审核、越权上限与取号都不变。

**三、旧行读作「未记供应商」，经补记口按协议补。**

- 迁移 `settlement_accounting/0055` 只给 `supplier_expected_cost` 加一列可空的 `supplier_party_ref`（非空时不得是空白），不改任何已有行，在有数据的库上可施加。
- 不在迁移里回填：回填要读 `party_commercial.supplier_agreement`，那是另一个上下文的表；限界上下文不等于库表共享许可（AGENTS 红线「所有权清晰」）。
- 既有行读作「未记供应商」：读口这一格缺席，匹配门答 `EXPECTED_COST_SUPPLIER_UNRECORDED`，不照旧放过——放过就是把这次要补的口子留给旧行，而匹配门对新旧两种行该说同一句话。
- 补记口：`parcel-settlement-register expected-cost-supplier-backfill`，载荷点名租户。它找出这个租户没记供应商的各版成本，按各自的协议经同一只消费侧适配器问 PC，只在这一格仍为空时写下；同一版协议只问一次。PC 读不动答未决（退出码 3），整笔回滚；PC 不认、指称歧义或译不过来的那几份留空并逐份列出，其余照写——它们要人去看两侧为什么对不上，不能为它们挡住其余的补记。只补仍为空的，所以重跑是幂等的；已记的一格不改。
- 补记是准的：已发布协议版本的正文不可覆盖，此刻问到的就是形成时会问到的那一家。它不是租户取值，也没有默认值：问不到就留空、照实答未记。
- 不在接收口里临时去问 PC：那是用户没选的「匹配时经协议方核对」，接收口要多一个跨上下文依赖与一种读不动。

**四、读口与管理台。**

- 预期成本册（`GET /settlement-charges?registry=supplier-expected-cost`）每一行带 `supplier`，未记时整键不出现，不填空串或「未知」。
- 管理台登记表单按所选供应商只列这家的当前预期成本；没选供应商不列；未记供应商的那几份不列，表单说一句有几份未记、补记后才能选。换了供应商，已选的别家成本随之清掉。表单筛选只省人的事，挡人是匹配门的事：接口直调与旧页面一样由匹配门拒。

三类：以上都是机制。供应商是协议上的事实，不是租户取值，产品不预设、不默认。

## Alternatives considered

- **匹配时经往来单位与协议方核对（用户没选）。** 不记供应商，每次接收逐行问 PC 这一版协议是哪家的。接收口多一个跨上下文依赖与一种读不动；表单要按供应商筛也得逐份问或另开 PC 的列表读口；同一事实每匹配一次问一次，而它在协议发布那一刻就定了。否决（用户裁定）。
- **暂不补（用户没选）。** 匹配门不核、表单只提醒，误选就对到别家，ADR-0340 越权风险点四一直开着。否决（用户裁定）。
- **供应商由形成载荷带入（请求或发生项另带一份）。** 同一事实两处，带进来的那份还得再问 PC 才可信；不如直接记问到的。否决。
- **用发生项上的服务提供方。** 那是谁运，可以是自营法人，不是协议的对方，也不是开账单的一方。否决。
- **在迁移里按协议回填。** 要在结算的迁移里读 `party_commercial` 的表，跨上下文读表。否决，改为经适配器的补记口。
- **旧行照旧不核、只在答复与读面上标出。** 口子留给旧行，同一个匹配门对新旧行说两句话。否决，改为答未记、先补记。
- **未记与不符并成一格。** 前者等补记，后者改录入，恢复动作不同（ADR-0029）。否决。
- **供应商进内容比对。** 由协议定、无新信息；旧行重投会被判成冲突。否决。
- **把供应商列进纠错不动的身份（后继协议换供应商就拒，同换结算币）。** 纠错版本的协议本就整组重述，供应商是协议的属性；拒了就要人处置一件当前版判据已经挡住的事——旧供应商点名被纠正那一版答不是当前版。否决。
- **只在管理台上筛。** 页面挡不住接口直调，也挡不住筛选之后又改了供应商。否决，匹配门才是挡人的那一道。
- **发起 BUY 请求时就问供应商、记在请求上。** 问 PC 的时机提早一拍，但请求册要多一列、旧请求同样没有，形成时还得补问；形成编排本就逐版构造成本，在那里问一处就够。否决。

## Consequences

- party-commercial：只加读口 `ports.SupplierAgreementSupplierView` 与它的 PostgreSQL 实现，与协议范围那一口共用同一段按两段式指称比对的读法；领域规则与协议生命周期不动。
- SA 领域：`SupplierExpectedCostSpec`、`CostCorrectionSpec` 必带供应商，重建门可缺（旧行）；`SupplierExpectedCost.Supplier` 交回供应商与是否已记；`MatchBillLine` 多两个哨兵 `ErrBillSupplierMismatch`、`ErrExpectedCostSupplierUnrecorded`。
- SA 编排与端口：形成编排多一个依赖 `SupplierAgreementSupplierView`（消费侧适配器在 `adapters/partycommercial`），未决原因多 `SUPPLIER_AGREEMENT_VIEW_UNAVAILABLE`；接收编排多两种答复；预期成本库多两问（列出未记供应商的版本、补记一版）；补记编排 `BackfillExpectedCostSupplierHandler`。
- 迁移 `settlement_accounting/0055`；派发进程的形成装配接 PC 读口；`parcel-settlement-register` 多一条命令。
- 读口预期成本行多 `supplier`；接收答复多 `expectedCostSupplier` 一格（行、版本，不符时带那一版的供应商）。
- 管理台登记表单按供应商筛，去掉「这里不按供应商筛」的提示。

## 越权风险点

1. **已接收的跨供应商匹配不重判。** 本记录之前录进去、点名了别家成本的账单行照旧在册，审核照走；要找出它们得直查库（账单的供应商与所点名那一版成本的供应商不同）。
2. **补记靠人跑。** 迁移之后、补记之前，旧行对不上账（答未记）；补记口不自动跑。
3. **PC 不认的旧行永远答未记。** 补记留空并列出，要人去看两侧为什么对不上；本记录不给它们别的出路。
4. **协议后继换供应商时，成本跟着换。** 纠错版本按后继协议记新供应商；旧供应商对这份成本只剩「不是当前版」一句。PC 是否该允许同一协议对象换供应商，是 PC owner 的事，本记录不判。
5. **形成多一个跨上下文依赖。** PC 读不动时形成停住重投；此前形成只读结算自己的册与计价的评价。
6. **逐行裁决仍由人给。** ADR-0340 越权风险点四只回答了「点名别家的预期成本」那一种误选；同一家供应商名下点错哪一份、分类给错，仍由人判、由金额与当前版判据兜一部分。
7. **表单只列读口那一页。** 预期成本读口按上限截页（既有行为），某家供应商的成本落在页外时表单列不出；本记录不改截页。

## Links

- [ADR-0340](./0340-supplier-bill-reception-and-audit-are-two-operator-decisions-and-each-records-the-operator.md)：部分停用其决定一「重放、版本冲突、未受理、点名的预期成本不是当前版，都是答案（`200`）。」一句（此前已由 ADR-0357 扩过一次，加账单号冲突；本记录再扩两格：供应商不符、供应商未记）；回答其越权风险点四「逐行裁决由录入的人给。」里点名别家预期成本那一种误选，分类与点名哪一份仍由人给
- [ADR-0200](./0200-buy-evaluation-forms-the-supplier-expected-cost-through-its-request.md)：部分停用其决定四「下列各格上抛错误，落发布失败：」一句的列举（加一格：PC 不认这一版协议、指称歧义或答出译不过来的供应商）与决定五「BUY 请求册、评价读口、预期成本库答不出时，整笔回滚等重投。」一句（加协议供应商读口）
- [ADR-0208](./0208-buy-evaluation-is-requested-from-the-charge-occurrence-formed-envelope.md)：决定四（协议范围读口的形与三格，本记录照搬）
- [ADR-0232](./0232-a-segment-commission-adoption-is-withdrawn-by-decision-only-before-occurrences-form-and-occurrence-revision-drives-the-supplier-cost-correction.md)：决定七（当前版判据、失效纠错版本）、决定八（替代版本形成纠错版本）
- [ADR-0067](./0067-cost-correction-restates-the-whole-evaluation-result.md)：决定二、三（不变；供应商跟着协议引用走）
- [ADR-0357](./0357-a-supplier-bill-number-is-an-external-identifier-of-the-claim-and-an-audit-passes-disputes-or-rejects-amount-ranges-of-a-line.md)：范围审核与已匹配金额（不变；匹配不过到不了审核）
- [ADR-0025](./0025-cross-context-adapters-live-on-the-consumer-side.md)：消费侧适配器；[ADR-0029](./0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)：答复按恢复动作分格；[ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：三类
- [SA CONTEXT「供应商成本与共享分摊」「供应商主张与审核应付」](../domain/settlement-accounting/CONTEXT.md#供应商主张与审核应付)、[GLOSSARY「供应商预期成本」](../domain/GLOSSARY.md#供应商预期成本)
- [UC-SA-004](../application/settlement-accounting/UC-SA-004-RECEIVE-MATCH-AND-AUDIT-SUPPLIER-BILL.md)：`AT-SA-236`、`AT-SA-237`；[UC-SA-002](../application/settlement-accounting/UC-SA-002-CALCULATE-CONFIRM-AND-ADJUST-OPERATIONAL-CHARGES.md) 结果契约「供应商预期成本已形成」
- 票 [document-numbering/11](../../.scratch/document-numbering/issues/11-supplier-bill-and-audited-payable-admin-pages.md)（缺口出处）、[document-numbering/14](../../.scratch/document-numbering/issues/14-expected-cost-records-supplier.md)
