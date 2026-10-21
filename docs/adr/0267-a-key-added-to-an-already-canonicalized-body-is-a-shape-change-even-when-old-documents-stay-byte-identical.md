# ADR-0267：在已接规范化的册正文里加键（含可缺键）是形状变化，照 ADR-0014 换号；旧文档逐字节不变不是不换号的理由，同号改形只在册上没有要回放的已存摘要时成立

Status: Accepted（2026-10-21，Claude Code 会话代 PC owner 决：用户 2026-10-21 原话「需要修改文档，确保自洽，正确」，常设授权「你作为业务和系统专家自决」；票 docs-consistency/11，出处审计 [audit/cross.md](../../.scratch/docs-consistency/audit/cross.md) B2·B2-04。部分停用 [ADR-0129](./0129-credit-ratio-base-is-declared-on-the-credit-policy-content.md) 决定一括注一句与 [ADR-0176](./0176-customer-service-rule-contract-tier-selection-and-inheritance.md) 决定五括注一句，被停原句与适用场景见「对已接受记录的作用」；两份正文不改，只在其 Status 行与 Links 加前向指针。[ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)、[ADR-0126](./0126-commercial-publication-digest-is-computed-server-side-per-register-and-approval-comes-through-a-pending-carrier.md)、[ADR-0209](./0209-supplier-agreements-declare-charge-occurrence-conditions-and-registration-judges-by-them.md) 无条款停用。代码已是本记录所述的样子，不立代码票。越权风险点见文末，待 PC owner 复核，PP、TF owner 知会。）<!-- 索引注记 -->｜**判据**：一册已经接进某个规范化版本之后，它的正文文档多一个键、少一个键或改一个键，都是 ADR-0014 所说的规范化结构变化，照换号；可缺键让旧文档逐字节不变，可旧构建读到带新键的文档会把它当未知键丢掉、算出对不上的摘要而报不出形状变了（ADR-0209 候选与反方），所以旧文档不变不构成不换号的理由。**加册**：一册从没接到接上不是形状变化（ADR-0126 决定一），照旧不换号。**例外**：只在那一册没有需要回放的已存摘要时可以同号改形，须在记录里写明前提并部分停用 ADR-0014 Consequences「每次变化都要新增一个规范化版本并长期支持它」一句，与 ADR-0177、0201、0202 同形。**既往**：ADR-0129 给信用政策加 `ratioBase` 不换号，认作同一例外（前提是金额正文不变、比例正文没有存量），不回头换号；它括注里援引的「加册不换号」与 ADR-0176 决定五括注里的「加 omitempty 键不换号的先例」都不再作先例

Date: 2026-10-21

## Context

- [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md) 决定「规范化版本的变化必须来自规范化结构的变化」，Consequences 写「每次变化都要新增一个规范化版本并长期支持它」。它的索引注记记了三次同号改形（ADR-0177、0201、0202），每次都以「没有需要回放的已保存摘要」为前提，并写明「此后任何一册再改规范化形状照本记录换号」。
- [ADR-0126](./0126-commercial-publication-digest-is-computed-server-side-per-register-and-approval-comes-through-a-pending-carrier.md) 决定一：「加册不换号：换号只在既有册的文档形状变化时发生（ADR-0014），一册从「没接」到「接了」不改任何已算出的字节。」
- [ADR-0129](./0129-credit-ratio-base-is-declared-on-the-credit-policy-content.md) 决定一在已接的信用政策一册里加键不换号：「PCC-1 不换号：`canonicalCreditPolicyBody` 加键 `ratioBase,omitempty`（ADR-0126 决定一「各册接进同一个 `PCC-1` 不换号」同法）；金额正文的规范化文档因此一字不变，既有摘要不动。」它援引的是加册那一条，情形不同；它也没有按停用机制在 ADR-0014 上挂指针。
- [ADR-0176](./0176-customer-service-rule-contract-tier-selection-and-inheritance.md) 决定五反把它当先例：「PCC-1 一字不改（若要改则按加 omitempty 键不换号的先例另核）」。[ADR-0177](./0177-customer-contract-names-its-customer-account-and-the-closure-narrows-by-it.md) 决定三已对客户合同一册显式偏离这一先例：「这是对 ADR-0176 决定五所记先例（「若要改则按加 omitempty 键不换号的先例另核」）的一次显式偏离」；ADR-0176 上没有这条指针。
- [ADR-0209](./0209-supplier-agreements-declare-charge-occurrence-conditions-and-registration-judges-by-them.md) 决定三遇到同一种情形（供应商协议一节加可缺的收费发生条件，不带条件的旧文档逐字节不变）却换号：「改的是供应商协议一节的形状，照 ADR-0014 换号。」它的候选与反方写了理由：「只认 `PCC-1` 的构建读到带条件的文档时会把条件当未知键丢掉，算出的摘要与列上对不上却报不出是形状变了。」
- 于是仓里有两条换号规则：ADR-0014、0126、0209 一条，ADR-0129、0176 一条，没有记录裁过谁先谁后。下一位给 PCC、PPC 或 TFC 加可缺键的作者，照哪条做都能找到一份已接受记录撑腰。
- 代码（取证于 main `b18f662bc`）：`internal/partycommercial/domain/publication_canonicalization.go` 里信用政策正文的 `RatioBase string json:"ratioBase,omitempty"` 在 `PCC-1` 之下；同一文件另有供应商协议用的 `PCC-2`（`supplierChargeConditionCanonicalizationVersion`），按内容取能表达它的最低一版。折回存下的文档走 `RehydratePublicationContent`，用 `json.Unmarshal` 解码、不拒未知键——ADR-0209 说的「当未知键丢掉」在这一处成立。

## Decision

**一、已接规范化的册，正文文档的键集变化就是形状变化，照 ADR-0014 换号。**（机制）

- 加键、删键、改键名或改键的取值形状都算，可缺键（`omitempty`）也算。
- 不带新内容的旧文档逐字节不变，不构成不换号的理由。旧文档不变只保证已存的摘要照旧对得上；读到新文档的旧构建会把新键当未知键丢掉，再算出一个对不上的摘要，却说不出是形状变了。换号让旧构建对新文档答「不支持」，而不是答错。
- 换号之后按内容取能表达它的最低一版（ADR-0209 决定三的做法）：不带新内容的文档仍写旧版、逐字节不变，带新内容的写新版。
- 适用于各上下文的规范化版本（PCC、PPC、TFC、PSC 等），不只 party-commercial。

**二、加册不是形状变化，照旧不换号。**（机制）

一册从没接到接上（ADR-0126 决定一）：旧构建本来就没有那一册的规范化，册上不会有它写下的那一册的摘要可比，接上不改任何已算出的字节。这一条与决定一不相抵。

**三、同号改形只有一种例外：那一册没有需要回放的已存摘要。**（机制）

前提要在记录里写明，并照 ADR-0177、0201、0202 的形部分停用 ADR-0014 Consequences「每次变化都要新增一个规范化版本并长期支持它」一句，适用场景限那一次改形。前提失效的那一刻（册上有了第一条要回放的摘要）起，此后的改形照决定一换号。

**四、ADR-0129 那一次认作决定三的例外，不回头换号。**（机制）

ADR-0129 给信用政策加 `ratioBase`，前提是它同句写的「金额正文的规范化文档因此一字不变，既有摘要不动」，加上比例额度的存量正文理论上为零（ADR-0129 决定三「无租户故理论上为零」）。这与决定三同一前提，只是当时没有按停用机制记。今天不回头给带基数的比例正文换号：它们的摘要已经在 `PCC-1` 之下，换号会让册上已存的串对不上，正是 ADR-0209 决定三「不按构建取最新一版」要避的。它括注里援引的 ADR-0126 决定一说的是加册，不作在册加键的先例。

## 候选与反方

- **旧文档字节不变就不换号（ADR-0129、0176 那一条）。** 否决：它只顾了已存摘要，没顾旧构建读新文档；本仓的折回口不拒未知键，丢键是静默的。滚动发布、回滚或多个进程版本并存时，这一格就会出现。
- **一律换号，连没有已存摘要的那一册也换。** 否决：ADR-0014 的代价论证（「代价在此刻支付最低」）正是为没有存量的时刻留的；没有存量时换号只多一个要长期支持的版本，换不来任何可比性。三次既有例外都按此成立，本记录不推翻。
- **改折回口拒未知键，就可以不换号。** 不采纳为替代：拒未知键让旧构建响亮失败，可它报的是「文档坏了」，不是形状变了、该换构建；换号后旧构建答的是本构建不认这一版，恢复动作明确。两者可以并存，拒未知键另作加固，不在本记录。
- **把 ADR-0129 的 `ratioBase` 改写成新一版。** 否决：理由见决定四。

## 对已接受记录的作用

- **部分停用** ADR-0129 决定一括注「（ADR-0126 决定一「各册接进同一个 `PCC-1` 不换号」同法）」一句作为不换号理由的效力。**适用场景**：只停这一层理由——加册与在册加键不是同一情形；决定一「PCC-1 不换号」的结论不变，成立的理由改为同句「金额正文的规范化文档因此一字不变，既有摘要不动」与比例正文没有存量（决定四）。ADR-0129 其余各条不变。
- ADR-0129 决定一「PCC-1 不换号」本身是对 ADR-0014 Consequences「每次变化都要新增一个规范化版本并长期支持它」一句的部分停用，ADR-0129 正文没写；本记录认定，指针补在 ADR-0129 的 Status 行前段（去向）与 ADR-0014 的索引注记（来向），适用场景仅限信用政策一册加 `ratioBase` 一键那一次。
- **部分停用** ADR-0176 决定五括注「若要改则按加 omitempty 键不换号的先例另核」一句。**适用场景**：此后任何一册的规范化文档要加键（含可缺键），照决定一换号或照决定三写明例外，不再按这一先例另核；决定五「PCC-1 一字不改」本身照旧成立（ADR-0176 那一次确实没加键）。ADR-0177 决定三对客户合同一册那一次的显式偏离，此前没在 ADR-0176 上挂指针，随本记录一并补在 ADR-0176 的索引注记与 Links。ADR-0176 其余各条不变。
- ADR-0014：无条款停用。决定一读的正是它 Decision「规范化版本的变化必须来自规范化结构的变化」；它的索引注记加一条读法，指向本记录。
- ADR-0126：无条款停用。决定二照用它决定一的「加册不换号」。
- ADR-0209：无条款停用。决定一照用它决定三与候选与反方的理由。

## Consequences

- 代码不改：`PCC-1` 之下的 `ratioBase` 是决定四认下的例外，`PCC-2` 是决定一的做法。`publication_canonicalization.go` 里信用政策正文那段头注（「ADR-0129，PCC-1 加键不换号——金额正文的文档一字不变，既有摘要不动」）与本记录一致，不改。
- ADR-0014、0129、0176、0177 的 Status 行与 Links 各补指针（见上节）。
- 此后给任何一册的规范化正文加可缺键的作者，按决定一换号；要同号改形，先查那一册有没有已存摘要，按决定三写明前提。
- 不改 CONTEXT：规范化版本是摘要的技术合同，不进领域语言。

## 越权风险点

1. **代 PC owner 定了全仓的换号判据**（PC owner，PP、TF owner 知会）。决定一对各上下文的规范化版本一体适用；某个上下文若要可缺键不换号，要另立记录说清它的折回口怎样认出形状变了。
2. **ADR-0129 那一次没有按今天的口径做**（PC owner）。带基数的比例正文与不带基数的存量正文同在 `PCC-1` 之下；只认无基数形的构建读到带基数的文档，会把基数丢掉、按「未声明基数」读回。本仓今天没有滚动发布与多版本并存，这一格到不了；有了之后，要么换号，要么让折回口拒未知键。
3. **决定三的前提由作者自证**（PC、PP、TF owner）。「册上没有要回放的已存摘要」靠迁移在有行时拒绝施加（ADR-0201、0202 的做法）或靠作者核库；没有这道迁移闸的同号改形，前提只是一句陈述。
4. **折回口不拒未知键**（PC owner）。决定一的理由建立在这一事实上；它本身是一处可加固的地方，本记录不改，留给 PC owner 判要不要另立票。

## Links

- [ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)：Decision「规范化版本的变化必须来自规范化结构的变化」与 Consequences「每次变化都要新增一个规范化版本并长期支持它」；本记录写明读法，并认定 ADR-0129 那一次部分停用后一句
- [ADR-0126](./0126-commercial-publication-digest-is-computed-server-side-per-register-and-approval-comes-through-a-pending-carrier.md)：决定一「加册不换号」，本记录照用
- [ADR-0129](./0129-credit-ratio-base-is-declared-on-the-credit-policy-content.md)：部分停用决定一括注一句；认定决定一「PCC-1 不换号」是对 ADR-0014 的部分停用
- [ADR-0176](./0176-customer-service-rule-contract-tier-selection-and-inheritance.md)：部分停用决定五括注「若要改则按加 omitempty 键不换号的先例另核」一句
- [ADR-0177](./0177-customer-contract-names-its-customer-account-and-the-closure-narrows-by-it.md)：决定三对 ADR-0176 决定五先例的显式偏离与对 ADR-0014 的部分停用，决定三的例外照它的形
- [ADR-0201](./0201-schedule-templates-materialize-concrete-schedules-by-date-and-preparation-answers-not-configured-on-an-empty-register.md)、[ADR-0202](./0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md)：同号改形的前提与迁移闸
- [ADR-0209](./0209-supplier-agreements-declare-charge-occurrence-conditions-and-registration-judges-by-them.md)：决定三换号与按内容取最低一版；候选与反方里旧构建丢键的理由
- 票 `.scratch/docs-consistency/issues/11-unclaimed-cross-adr-contradictions.md`
