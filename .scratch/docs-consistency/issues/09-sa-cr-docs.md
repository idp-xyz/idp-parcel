# 09 SA+CR（settlement-accounting、collection-remittance）：CONTEXT 写出现行规则、UC 与之一致、矛盾裁清

Category: enhancement
Status: in-progress——完工待评审与进 main，分支 claude-dc09。此前 ready-for-agent——2026-10-21 协调方立（文档自洽修订，见 [README](../README.md)「修订执行」）
Blocked by: 无（01 进 main 后开工；票 57+15 已进 main）
出处：[audit/sa-cr.md](../audit/sa-cr.md)（sa-cr.md）；[02 的完成记录](./02-code-defects-from-the-audit.md)里「交修订票」一表中交给本票的条目；[01](./01-adr-suspension-chains.md) 的「交后续」一节里交给本票的条目。

## 要什么

1. 按 main 现状逐条复核审计报告里本组的 A、B 类条目（审计钉 `6b0074d0`；其后进 main 的票与文档自洽 01、02 可能已消解一些——消解的写「已由某票消解」并指 SHA）。
2. **A 类**：改文档。范围 docs/domain/settlement-accounting/、docs/domain/collection-remittance/ 的 CONTEXT.md 与对应 docs/application/，以及本组新立 ADR 与它所停用的 ADR 的 Status 行与 Links。目标照 README「目标」1、2、4、5：CONTEXT 写出今天生效的语言、不变量与生命周期（已被 ADR 部分停用的句子不再以现行口吻留着，改写为现行规则并引用 ADR）；UC 同一场景只有一套结果，结果名与代码一致；不用行号、计数、位置引用。
3. **B 类**：按用户授权（「你作为业务和系统专家自决」）代决，立新 ADR（号段 0269–0273），Status 写法照近期 ADR，写越权风险点，部分停用照 `docs/adr/README.md` 顶部规则（文档自洽 01 改过的「一件事只写一处」）；裁完 CONTEXT 与 UC 随之改。裁定若要求改代码，不在本票改：在 [code/](../code/) 下补一张代码票（编号接 code/ 现有最大号），或在 02 已立的对应代码票里写明裁向并解除其 Blocked by。
4. **交横切**：需要改 GLOSSARY、CONTEXT-MAP、SCENARIOS、`docs/product/`、`docs/agents/`、别组 CONTEXT/UC 的，不在本票改，逐条写进本票「交横切」一节（位置、引文、改成什么、依据），由票 10 统一改。

## 不变式

- 已接受 ADR 正文不改；只动本组文档、本组新 ADR 及其停用对象的 Status 行与 Links。
- 不改代码；不改 GLOSSARY、CONTEXT-MAP 等横切文件。
- `go run ./tools/adr-index -check`（重生成后）退 0。

## 完成判据

- 审计报告本组每一条 A、B 都有去向（已改 / 已由某票消解 / 交横切 / 立代码票）。
- 完成记录里一张对照表：审计条目 → 去向。

## 完成记录（2026-10-21，分支 claude-dc09，rebase 于 main `35d181d1c`）

审计 [audit/sa-cr.md](../audit/sa-cr.md) 26 条逐条按 main 复核（审计钉 `6b0074d0`）。其间进 main、动过本组的有：票 demo-chain-gaps/57+15（`5246a8c30` 代码、`0b338da96`、`381274461` 文档，改 SA CONTEXT 供应商成本各条与 UC-SA-002、UC-SA-004）、文档自洽 01（`bc8283e33`、`caf5d8fe0` 改 0081、0179、0229 等的 Status 行）；07、08 没碰本组文件，只有 `.scratch/at-coverage-inventory.md` 在 rebase 时并入。代码类条目复核时现象全部仍在，相关符号与注释原样在。02「交修订票」交给本票的两张代码票（code/11 待裁 M11、code/14 待裁 H3/H5）已写裁向、解除 Blocked by；01「交后续」没有交给本票的条目。

### 审计条目 → 去向

| 条目 | 归类 | 去向 | 落在哪 |
|---|---|---|---|
| H1 `AT-SA-179`「补登声明之后这一份也不回补」 | A | 已改 | UC-SA-002 `AT-SA-179`：记`待判断`、停因 `AMOUNT_ROUNDING_UNDECLARED`，登新卡并收尾旧版后经续办另起新一代（ADR-0198 决定二，ADR-0229 决定三、五，ADR-0234 决定四） |
| H2 多份 UC「未登记则待判断」 | A | 已改 | UC-SA-001 输入「商业责任」与主流程金额文法一步（`ADVANCE_UNDECIDED` 带 `CONTRACT_UNCONFIGURED` / `RECOVERY_GRAMMAR_UNCONFIGURED`）；UC-SA-002 主流程发起评价一步的前半句删去、按 ADR-0208 写 `BUY_EVALUATION_TRIGGER_UNCONFIGURED`；UC-SA-003 主流程读归属日一步（`CHARGE_ATTRIBUTION_UNCONFIGURED`）；UC-SA-004 主流程审核一步（`AUDIT_UNDECIDED` 带 `AUDIT_ESCALATION_UNCONFIGURED`）；UC-SA-006 主流程分法一步（`ALLOCATION_FORM_UNCONFIGURED`）；UC-SA-007 输入「商业责任」与主流程金额文法一步（`CLAIM_UNDECIDED` 带 `CLAIM_RULE_UNCONFIGURED` / `CLAIM_GRAMMAR_UNCONFIGURED`）；六份 UC 元数据补「未配置结束」一行、结果表合并行补`未配置`。审计附带的待核（未配置有的单立结果、有的收在未决结果下当原因）不裁，留 SA owner：文档照代码逐只写明 |
| H3 同一笔本金多次入账 | B | 代决 ADR-0271；已改；既有代码票写裁向 | CR CONTEXT「资金位置」词条、「分户账与记账」入账与判重各条、「清分、汇付与差异」两条、「代收事实与入账」「分户账资金位置」两节生命周期；[code/14](../code/14-collection-principal-posts-more-than-once-and-shortfall-moves-received-funds.md)「裁向」一节，解除 Blocked by |
| H4 CR CONTEXT 写入账可冲正 | A | 已改 | CR CONTEXT「分户账记账」词条、「分户账与记账」新立「入账不能冲正」一条、「代收事实与入账」来源侧更正一条、「分户账资金位置」错记一条（ADR-0082 Consequences） |
| H5 `短款`既是位置又是差额 | B | 代决 ADR-0271（与 H3 同一份）；已改；code/14 | CR CONTEXT「资金位置」`短款`、`溢款`两格释义，「差异事项」词条与生命周期，「清分、汇付与差异」差异一条；`待清分` / `应付客户` → `短款` 一条改为只从`渠道在途`落入，补 `待清分` → `溢款` |
| M1 价卡声明计价基准时点 | A | 已改；GLOSSARY 交横切 | SA CONTEXT 词条「计价基准时点」与「计价、组合与精度」里计价基准时点那一条；票 demo-chain-gaps/141 追记一节；交横切 X1 |
| M2 冻结由预估费用形成 | A | 已改 | SA CONTEXT 词条「当前有效客户预估费用」、「运营余额、收付款与坏账」冻结一条、「费用明细」预估一条、「资金冻结与核销」冻结一条；UC-SA-002「接受前财务控制」首条、结果表「客户费用预估/暂估已形成」一行（ADR-0179 决定一、二、五） |
| M3 BUY「结论不记在请求上」 | A | 已改 | UC-SA-002 主流程校验映射一步（ADR-0200、ADR-0229 决定三） |
| M4 CONTEXT-MAP `settlement-accounting → parcel-pricing` 两句 | A | 交横切 | X2 |
| M5 发生项原因另册登记写成 SELL 现行路 | A | 已改 | SA CONTEXT「计价、组合与精度」SELL 一条；UC-SA-002 主流程发起评价一步（ADR-0169 决定三与越权风险点 2、ADR-0190 越权风险点 4） |
| M6 UC-SA-003「结算账户」行混进时区、周期、付款条件 | A | 已改 | UC-SA-003 输入语义契约拆成结算账户、对账安排、付款条件各一行 |
| M7 截单不核归属日 | C | 既有代码票写裁向；文档加缺口注 | [code/11](../code/11-statement-cut-does-not-check-the-settlement-attribution-date.md)「裁向」一节（随 ADR-0269），解除 Blocked by；UC-SA-003、UC-SA-002「给开发的交接」 |
| M8 发生项有效性修订不驱动纠错 | C | 已由票 demo-chain-gaps/15 消解（`5246a8c30` 代码，`0b338da96`、`381274461` 文档） | SA CONTEXT 失效纠错与替代版本一条、UC-SA-002 `AT-SA-186`、`AT-SA-206`–`210` 已随 ADR-0232 改；`AT-SA-164` 在盘点 ① 由撤下测试钉住 |
| M9 资金映射缺赔付义务、追偿应收 | C | 既有代码票承接；文档加缺口注 | [code/12](../code/12-funds-mapping-has-no-compensation-or-recovery-targets.md)；UC-SA-005、UC-SA-007「给开发的交接」 |
| M10 GLOSSARY「客户费用退款」 | A | 交横切 | X3 |
| M11 时区与截单时刻两个所有者 | B | 代决 ADR-0269；已改；code/11 写裁向 | ADR-0168 挂部分停用与越权风险点 3 的回答；SA CONTEXT 词条「截单快照」「对账安排」与「结算账户、对账与争议」里对账单纳入、对账安排、归属日各条；UC-SA-003 目标与边界、输入、元数据、主流程读归属日与形成纳入快照的步骤，新立 `AT-SA-212`、`AT-SA-213`；UC-SA-002 主流程确认费用一步 |
| M12 ADR-0236 称核销有受控 CLI | A | 已改；入口缺口立代码票 | ADR-0236 Status 行注记「表述更正」与 Links；入口缺口立 [code/46](../code/46-settlement-application-and-reversal-have-no-process-entry.md)；UC-SA-005 状态行与「给开发的交接」 |
| L1 风险点被回答而无前向指针 | A | 0229 ← 0234 已由文档自洽 01 消解（`caf5d8fe0`）；ADR-0190、ADR-0198 那几处已改 | ADR-0190 注记补越权风险点 8 由 ADR-0229 决定四回答；ADR-0198 注记补越权风险点 1、2 由 ADR-0229 部分回答 |
| L2 0158 引文非原句、0179 停 0081 决定六无链 | A | 已由文档自洽 01 消解（`bc8283e33`） | 0081 注记逐格引原句并挂 0158、0156、0179、0157；0179 前段已写去向。0158 正文那句是转述、不带书名号，正文不改 |
| L3 不可计价「不重试」与续办 | A | 已改；CONTEXT-MAP、GLOSSARY 交横切 | UC-SA-002 结果表「计价明确排除已接收」一行、`AT-SA-175`、`AT-SA-183`；交横切 X4 |
| L4 阶梯返利漏写逐档取整 | A | 已改 | SA CONTEXT 周期费用一条 |
| L5 渠道两层停在`待清分` | A | 已改（随 ADR-0271） | CR CONTEXT「清分、汇付与差异」里未实际收到的代收款那一条 |
| L6 CR 关联原记录、结论格、归集截点 | C | 既有代码票承接；文档加缺口注 | [code/15](../code/15-collection-records-cannot-point-back-to-what-they-revise.md)；CR CONTEXT 文末「给开发的交接」 |
| L7 对账单替代关系不存在 | C | 既有代码票承接；文档加缺口注 | [code/13](../code/13-voided-statement-has-no-replacement-relation.md)；UC-SA-003「给开发的交接」 |
| L8 计费时点形成的费用标预估 | B | 代决 ADR-0270；已改；不改代码 | ADR-0190 注记挂越权风险点 7 的回答；SA CONTEXT「费用明细」预估与暂估一条；UC-SA-002「生命周期与控制分支」费用明细一条 |
| L9 代码注释计数不符 | A（注释） | 立代码票 | [code/45](../code/45-sa-cr-comments-carry-counts-that-no-longer-hold.md) |

去向计数（按主去向，合计 26）：已改 17（A 13：H1、H2、H4、M1、M2、M3、M5、M6、M12、L1、L3、L4、L5，其中 L1 的 0229 一处已由 01 消解；B 4：H3、H5、M11、L8）；已由某票消解 2（M8 由票 15，L2 由 01）；交横切 2（M4、M10，另有 M1、L3 的共享文件一半）；既有代码票承接 4（C 类 M7、M9、L6、L7 → code/11、12、15、13）；立代码票 1（L9 → code/45）。新立代码票 2：code/45（L9）、code/46（M12 的入口缺口，审计没有单列为 C）。02 已立的 code/11、code/14 写了裁向并解除 Blocked by。

### 代决与越权风险点

用户授权「你作为业务和系统专家自决」，ADR-0269、0270、0271 都是会话代决，Status 写明待谁复核，越权风险点见各记录文末。

- **[ADR-0269](../../../docs/adr/0269-business-time-zone-and-cutoff-belong-to-the-statement-arrangement-of-the-settlement-account-and-the-cut-admits-by-attribution-date.md)（M11，兼 M7 裁向，代 SA owner）**：业务时区与截单时刻只登在按结算账户的对账安排上，归属日判定与截单同读这一份；费用项目一册只登形态；归属日随费用确认形成、此后不变，确认时判不出的截单时当场判；截单命令点名本期最后一个归属日，按它纳入并逐件指名。部分停用 ADR-0168 决定二一句里「时区与截单时刻」那一半与决定三一句里「截单编排仍只读已经形成的归属日」那一半，回答其越权风险点 3。越权风险点：一个结算账户一个时区；对账安排换版不回改已形成的归属日；截单当场判用截单那一刻的版本；截单时刻 0 会把每个时点归到次日（ADR-0168 的判法，不改）；本期最后一个归属日由命令点名。
- **[ADR-0270](../../../docs/adr/0270-a-charge-stage-is-given-by-the-judgement-that-forms-it-and-an-estimate-does-not-become-provisional-with-time.md)（L8，代 SA owner）**：预估与暂估是形成那一步的结论，预估不随时点自动转为暂估；包裹计费时点那一路形成预估、停在预估直到确认；首发没有形成暂估的路。回答 ADR-0190 越权风险点 7，无条款停用，不改代码。越权风险点：服务已完成的包裹费用仍标预估；首发没有暂估。
- **[ADR-0271](../../../docs/adr/0271-collection-principal-enters-the-ledger-only-on-the-channel-collection-report-and-shortfall-comes-only-from-in-transit.md)（H3、H5，代 CR owner）**：只有渠道代收报告自外部来源入`渠道在途`；真实到账只移位到`待清分`、不足停在余额不足；收件人付款与渠道回款通知是旁证、不记账；凭代收事实的记账按依据判重，换标识再记是内容冲突；`短款`只从`渠道在途`落入，报告本身就少报的那部分只记在差异事项上；`溢款`只从`待清分`落入；补回走冲正短款再移位。无条款停用（ADR-0082 的依据门都是必要条件，本记录加门）。演示种子已是这一形。越权风险点：没有逐指令代收报告的渠道要由连接器补登报告；收件人付款不进账；到账多于报告整笔停住；换标识重记答冲突；短款补回要等 code/15 的更正关系。

作者在 A 类上另作的判断（不另立 ADR，评审可挑）：

1. **CR 没有 UC，缺口写在 CR CONTEXT 文末「给开发的交接」**。CR CONTEXT 头部因此不能照抄 NR/NO/VE/PS 那一句「规则与实现的差距见各 UC「给开发的交接」。」，写的是「本上下文还没有 UC，规则与实现的差距见文末「给开发的交接」。」；评审轮协调方裁定由票 10 立 `UC-CR-001` 并把这一节搬过去（交横切 X5）；本票先把那张表的「规则」一列改为只指 CR CONTEXT 的小节名。SA CONTEXT 头部照抄那一句。
2. **「当前有效客户预估费用」词条与「费用明细」生命周期的预估一条去掉「报价」**，不只是审计点的「冻结」：运营试算是计价用例、只算不存、不交结算（ADR-0152），报价不引结算的预估费用。UC-SA-002 结果表那一行同改。
3. **UC-SA-001、002、003、004、006、007 元数据补「未配置结束」一行**（H2 的连带）：原「待判断结束」只写依据不足，租户取值没登记的那一格在元数据上没有落处；UC-SA-003「待判断结束」去掉「时区」。UC-SA-002「未形成结束」补一句与 SELL 请求`未形成`结论的分别（后者是评价失败或计价没形成评价，不是技术未完成）。
4. **`AT-SA-050`（规则或币种版本缺失、原写保持待判断）改写**：包裹计费时点那一路与 BUY 那一路上没有在用价卡记的是`未形成`与停因（`AT-SA-198`），同一场景不能两套结果。
5. **ADR-0236 的「表述更正」、ADR-0190 / 0198 的风险点回答由本票挂**：审计列给本组、01 的附表没覆盖；照 06 的先例按「本组文档」处理，只动 Status 行与 Links，正文去掉 Status 行与 Links 后与 main 逐字节相同。
6. **X14（07 交来）在本票改了**：UC-SA-002 `PAR-SET-11` 一行补「取值时点规则与加点规则只作登记，评价的期次按计价基准时点取、不加点」（ADR-0260 决定三）；ADR-0260 已随 07 进 main，链接可解析。票 10 收 07 的 X14 时不必再改。

### 交横切（交 docs-consistency/10）

- **X1 GLOSSARY「计价基准时点」**（M1）。引文：「商业价格政策和可执行价卡明确规定的、用于选择适用版本和输入依据的业务时点。」与所有者一条「商业政策归 `party-commercial`，可执行规则归 `parcel-pricing`，实际金额采用时点归 `settlement-accounting`。」改成什么：定义改为产品内置、由发起评价的那一路与评价目的给出的业务时点，商业价格政策与可执行价卡不另声明；所有者一条去掉商业政策与可执行规则两半，或改为内置时点归产品、实际采用的时点由 `settlement-accounting` 保存。依据：ADR-0190 决定二与复核记录 R2、SA CONTEXT 同名词条、票 demo-chain-gaps/141（本票已在票面追记 SA 半边已改）。
- **X2 CONTEXT-MAP `settlement-accounting → parcel-pricing`**（M4）。引文：「合格来源引用指不到包裹主体、分区或计费重量时，评价停在显式未决`输入不可得`并点名缺哪一只只读口」与「评价请求的身份是铸造标识，自然键（范围、目的、引用集合摘要）只守幂等」。改成什么：前一句改为计价不形成评价，经「评价未形成」交回、结算在请求上记停因`PRICING_INPUT_UNAVAILABLE`，不按未决重投；后一句改为按（自然键、代次）唯一，续办出来的一代回指前一代。依据：ADR-0229 决定一、三；同文件紧邻的「`parcel-pricing → settlement-accounting`（评价未形成与参数已登记）」一条；ADR-0190 注记里同一措辞已列为被 ADR-0229 停用。
- **X3 GLOSSARY「客户费用退款」**（M10）。引文：「依据合同、渠道结果或其他有效责任，对已经形成的客户费用追加的退款或贷记金额。」改成什么：由计价纠错、商业让利（`UC-SA-002`）或客户索赔责任（`UC-SA-007`）对已形成的客户费用追加的退款或贷记金额；渠道退款结果不是它的依据。依据：SA CONTEXT 同名词条、调整类型表「非索赔客户费用退款」一行与 Boundaries「不能从渠道退款结果自动推导」；GLOSSARY 自己的「渠道退款结果」词条「它不自动形成客户费用退款」。
- **X4 CONTEXT-MAP 与 GLOSSARY「不可计价」不重试**（L3）。引文：CONTEXT-MAP `parcel-pricing → settlement-accounting` 一条「也不把它当作可补齐的缺口反复重试」；GLOSSARY「不可计价」一条「也不得把它当作可补齐的缺口反复重试」。改成什么：补一句不自动重试、登了新一版价卡之后经续办另起一代（ADR-0229 决定五），排除依据留在评价上、请求册只记停因（ADR-0198 越权风险点 3）。依据：UC-SA-002 `AT-SA-175` 与结果表「计价明确排除已接收」一行。
- **X5 collection-remittance 没有 UC：票 10 立 `UC-CR-001` 并迁交接表**（作者判断 1；协调方 2026-10-21 裁定）。位置：`docs/application/` 下没有 CR 的用例，`docs/application/README.md` 用例总表没有 CR 一行；CR CONTEXT 头部那一句与 NR/NO/VE/PS 的固定句不同，文末暂放「给开发的交接」（「规则」一列只指 CR CONTEXT 的小节名）。改成什么：票 10 立 `UC-CR-001`（登记代收指令、代收事实、分户账入账与移位记账、差异事项与回汇批次，结果码按 `internal/collectionremittance/application` 的 `Outcome`），把 CR CONTEXT 文末那一节搬进它的「给开发的交接」，CR CONTEXT 头部改回固定句「规则与实现的差距见各 UC「给开发的交接」。」，并在 `docs/application/README.md` 补一行。依据：CR 已有完整的编排与结果码（`cmd/parcel-collection-register`）；AGENTS.md「改文档」新增权威文档要在 docs/README 补入口。
- **X6 `docs/application/README.md` UC-SA-003、UC-SA-005 两行**（M7、M12 的连带）。引文：UC-SA-003「截单快照、对账单、金额争议和既有调整的后续账期纳入边界已形成」；UC-SA-005「产品内唯一真实收付采用、运营映射、未分配、部分/完整核销和撤销边界已形成」。改成什么：照两份 UC 的状态行补「按结算归属日纳入与对账安排登记未落」与「核销与核销撤销还没有进程入口、赔付义务与追偿应收的映射目标未落」。依据：UC-SA-003、UC-SA-005 状态行与「给开发的交接」。
- **X7 GLOSSARY「截单快照」**（ADR-0269）。引文：「在结算账户的明确时区和截单时刻，对尚未出账且已经具备结算资格的费用范围形成的不可覆盖集合。」改成什么：对一个结算账户、按截单命令点名的本期最后一个归属日，把尚未出账、已经具备结算资格且归属日不晚于它的费用范围形成的不可覆盖集合；归属日在该账户对账安排的业务时区与截单时刻里判定，截单动作的时刻不参与纳入。依据：ADR-0269 决定四；SA CONTEXT 同名词条。
- **X8 GLOSSARY「资金冻结」**（本票 M2 的连带）。引文：「在面单购买、订单接受或其他授权业务发生前，对运营结算余额中的明确金额作出的临时占用。」改成什么：在委托接受或其他已裁定冻结金额形态的授权业务发生前……；面单交易前的冻结没有裁定过金额从何而来，今天不形成。依据：SA CONTEXT 同名词条与「运营余额、收付款与坏账」里冻结那一条；ADR-0179 决定一、二。
- **X9 GLOSSARY「当前有效客户预估费用」**（作者判断 2）。引文：「它是费用明细的阶段化当前视图，可用于报价、冻结和预估毛利，不是新的可编辑金额对象，也不是客户运营应收或会计收入。」改成什么：去掉「报价、冻结」，写用于预估口径的经营毛利，受理前估价与资金冻结不引用它。依据：ADR-0152（运营试算只算不存、不交结算）、ADR-0179 决定一；SA CONTEXT 同名词条。
- **X10 GLOSSARY「结算政策」与「对账安排」撞义**（ADR-0269）。引文：「商业协议对结算币种、收付方向、预付或账期方式、对账周期、付款条件、信用约束、税务处理依据和允许归集范围作出的版本化约定。」改成什么：「对账周期」从结算政策的列举里拿出，或写明结算政策只约定周期的商业口径、对账周期的日历与业务时区、截单时刻属结算账户的对账安排（settlement-accounting 登记）；GLOSSARY 另立「对账安排」词条，与 SA CONTEXT 同名词条一致。依据：ADR-0269 决定一；ADR-0158 决定二「前三格是对账安排，付款条件是商业结算政策」；SA CONTEXT 词条「对账安排」。

### 检查

- scratchpad 副本（`git archive claude-dc09` 解出）上 `go run ./tools/adr-index` 重生成后 `-check` 退 0；与 main 的 README 比，变的只有 0168、0190、0198、0236 的注记行与新增的 0269–0271 三行。重生成的 README 不随本票提交。
- `go run ./tools/at-coverage-counts -check`：只报计数过期（② 与合计、SA 一行各多 2，即新立的 `AT-SA-212`、`AT-SA-213`），计数由推送方重算。
- 本票改过的 ADR（0168、0190、0198、0236）去掉 `Status` 行与 Links 节后与 main 逐字节相同（脚本比对，0 份不同）；0168 注记所引被停原句「形态、时区与截单时刻按费用项目另册登记。」「截单编排仍只读已经形成的归属日，本记录不改纳入与发布。」「截单编排不调用这次判定。」在 0168 正文逐字存在；0236 注记所引两句在 0236 正文逐字存在；0190、0198 注记所引风险点原句在各自正文逐字存在，0198 注记另引的一句出自 ADR-0229「对已接受记录的作用」。
- 改过与新增的全部 `.md` 相对链接与锚点逐个解析，全部能解析。
- 新写文字里的书名号引文逐个对全仓（main 与本分支）查过：除引自本文件节名、新 CONTEXT 原句与代码票里给出的建议写法之外，全部逐字有出处；ADR-0271 Context 所引 CR CONTEXT 原句是接受前的文字，在 main 上逐字存在，正文已注明。
- 新写文字里没有行号、跨文件计数与「上一条」式位置引用（逐行扫过）。
- `git diff main --stat` 只含：SA、CR 两份 CONTEXT；`docs/application/settlement-accounting/` 下七份 UC；新 ADR 0269–0271；ADR 0168、0190、0198、0236 的 Status 行与 Links，ADR-0082 的 Links（评审轮加）；`.scratch/at-coverage-inventory.md`（② 加两行）；`.scratch/docs-consistency/` 下本票与 code/11、14、45、46；`.scratch/demo-chain-gaps/issues/141-*`（追记一节）。不改代码。

## 评审记录（2026-10-21，隔离评审第 1 轮 → 作者修订）

阻断 2 条、建议合入前修 4 条、顺手 9 条，除下面写明的一处口径之外全部照办。修订后 rebase 到 main `14443ac03`。

| 条 | 评审意见 | 处置 |
|---|---|---|
| B1 阻断 | 「未配置结束」只覆盖改过的那一步，同一 UC 里别的编排的未配置格漏了 | 各行按编排列全、写明码属哪只编排，逐码对过 `String()`：（BUY 一格的编排第 2 轮评审更正为 `TriggerBuyEvaluationOnOccurrenceFormedHandler.Trigger`，ADR-0164 触发面另写半句）UC-SA-002 补确认触发门（`AdmitSettlementMomentHandler` 的 `SETTLEMENT_MOMENT_UNCONFIGURED`，ADR-0170 决定二）、确认（`ConfirmChargeHandler` 的 `CONFIRM_UNDECIDED` 带 `CONDITION_UNCONFIGURED` / `CONFIRMATION_FACTS_UNCONFIGURED`）、接受前控制（`ApplyPreAcceptanceControlHandler` 的 `CONTROL_NOT_FORMED` 带 `CONTROL_POLICY_NOT_CONFIGURED`、`CREDIT_BASIS_NOT_CONFIGURED`、`CONTROL_AMOUNT_NOT_CONFIGURED`、`ESTIMATE_PRICE_CARD_NOT_CONFIGURED`），SELL 一格写明 `SELL_BILLING_MOMENT_DIFFERS` 同答 `SELL_EVALUATION_REQUEST_UNCONFIGURED` 但不记停办（`sellInitiationStopCauseOf`、`AT-SA-191`）；UC-SA-003 补截单触发门、主流程首步加这一格，`CHARGE_ATTRIBUTION_UNCONFIGURED` 写明属 `JudgeChargeAttributionHandler`、截单今天不交，挂在 code/11 那一行上作落地后的格；UC-SA-004 补接收与匹配（`ReceiveSupplierBillHandler.Handle`）的 `BILL_UNDECIDED` 带 `CONVERSION_UNCONFIGURED`，`AT-SA-096` 与 `PAR-SET-04` 一行改为未配置；UC-SA-006 补 `AllocateCostsHandler` 的 `OPERATING_UNDECIDED` 带 `RULE_UNCONFIGURED`，主流程采用分摊规则一步同补 |
| B2 阻断 | UC-SA-002 费用明细首条前后两种口径 | 改为「预估或暂估 → 确认只在该费用类型的条件满足时发生；阶段由形成那一步给出」，去掉「预估 → 暂估」的转换说法 |
| 1 | ADR-0271 把`渠道在途`当成按指令的余额，代码按分户账键判 | 照协调方裁定按指令：决定二写明移位上限是同一指令已入账报告减去已移位、不挪用同账别的指令的在途款，并补候选「按分户账算」的否决；决定四、索引注记写明短款金额按指令；越权风险点 3 改写；CR CONTEXT 两条与资金位置生命周期同步；code/14「移位」一项改准（`SubledgerBalance.Admit` 今天按账判，要另加按指令的上限），「差额来源」与测试两项同步。**一处没照字面**：协调方写的「`溢款` = 到账 − 已入账报告」，在决定二下到账移不过已入账报告，这个差恒不大于零；落进`待清分`、能判溢款的是相对指令多出的那部分，所以写成「该指令已移入`待清分`的到账减去指令金额」。若协调方另有所指，请再裁 |
| 2 | 0168、0190、0198 注记里回答风险点写「无条款停用」，与回答方别的停用自相矛盾 | 0168、0190、0198 注记里新加的回答各处改为「这一回答不另停条款」 |
| 3 | ADR-0270 决定一的去向漏了失效与替代 | 补「预估失效、暂估不再成立或被新版本替代时追加失效或替代关系，原记录保留」，注记同步 |
| 4 | ADR-0270 Context 引的是改写前的 CONTEXT | Context 开头注明「以下所引 settlement-accounting CONTEXT 原文是本记录接受前的文字」 |
| 5 | CR 的 UC | 照裁定：X5 改写为票 10 立 `UC-CR-001` 并迁交接表；CR CONTEXT 交接表「规则」一列改为只指小节名，那一节末尾注明待迁 |
| 6 | 截单快照、UC-SA-003 首段与首步对齐 ADR-0269 决定四 | SA CONTEXT「截单快照」词条改按本期最后一个归属日、`CutOffAt` 只记在快照上；UC-SA-003 目标与边界首段、主流程首步去掉账户时区 |
| 7 | `AT-SA-213` 与「未配置结束」一套结果 | `AT-SA-213`、「未配置结束」与 ADR-0269 决定四（补一条）同写：那一件不入本期并指名，其余合格费用照常纳入、草稿照常形成；一件都入不了本期时不形成草稿，整笔答未配置并指名缺哪一册 |
| 8 | `AT-SA-050` 币种那半 | 补币种或汇率缺失时指向 `AT-SA-177` 的半句 |
| 9 | 冻结一条补现状、「资金冻结」词条列举对齐 | 规则补「面单交易前的冻结没有裁定过金额从何而来，本上下文今天不为面单交易形成冻结」；词条列举改为「委托接受或其他已裁定冻结金额形态的授权业务」（代码核过：SA 应用层没有面单交易的冻结路） |
| 10 | 新概念补词条 | SA Language 立「对账安排」；CR Language 立「入账记账与移位记账」；ADR-0271 去掉「入账层 / 移位层」，旁证在决定一里一句定义 |
| 11 | 序号与位置引用 | code/14、code/11 裁向去掉「第 N 条」「下面」；code/11 用例一项与 code/46 的跨文件步号改为引步骤原文；票面表与票 141 追记的「首条」、跨文件步号改为点名；ADR-0271 决定六、ADR-0269 Context 改为点名 |
| 12 | ADR-0082 Links 指向 0271 | 加一行「在本记录的依据门上另加门」；0082 正文去掉 Status 行与 Links 后与 main 逐字节相同 |
| 13 | 截单时刻 0 | 不改，如实留在 ADR-0269 越权风险点 4 |

修订后的检查（main `14443ac03`）：scratchpad 副本上 `go run ./tools/adr-index` 后 `-check` 退 0；`at-coverage-counts -check` 只报计数过期（② 与合计、SA 一行各多 2）；0082、0168、0190、0198、0236 去掉 Status 行与 Links 后与 main 逐字节相同；改动的 `.md` 链接与锚点全部能解析；新写的书名号引文逐个对过出处（未命中的只有本文件节名与本文件原有文字）；新写的码与符号逐个在 `internal/`、`cmd/` 里确认存在（字符串码对 `String()` 的返回值，符号对定义）。

## 评审记录（第 2 轮，2026-10-21）

阻断 5 条、建议合入前修 3 条、顺手 3 条，全部照办；修订后 rebase 到 main `e8cd3de65`。

| 条 | 评审意见 | 处置 |
|---|---|---|
| 1 阻断 | UC-SA-002 BUY 一格挂错了编排 | 未配置结束一行改挂 `TriggerBuyEvaluationOnOccurrenceFormedHandler.Trigger`（装配在 `cmd/parcel-dispatch` 的 `assemble_evaluation_resumption.go`），带两个原因、记停办；另写半句：ADR-0164 触发面 `TriggerBuyEvaluationHandler` 同答 `BUY_EVALUATION_TRIGGER_UNCONFIGURED`，但不带原因、不记停办。主流程发起评价一步结果列补「未配置」；第 1 轮记录的 B1 行补一句更正 |
| 2 阻断 | ADR-0269 整笔答未配置太宽，全部晚于本期没有格 | 决定四收窄为「一件都入不了本期、且其中有判不出归属日的」整笔答未配置；全部晚于本期另给一格：不形成草稿，逐件指名晚于本期，不答未配置。同一句逐字写进 SA CONTEXT 进入对账单那一条、UC-SA-003 未配置结束一行与 `AT-SA-213`、code/11；`AT-SA-212` 补全部晚于本期一格；ADR-0269 Consequences 与 code/11 写明截单编排要新立整笔未配置的格、归属日判定要补「缺哪一册」的原因格（今天都没有）；UC-SA-003 交接表第一列点名元数据「未配置结束」一行，现状缺口一格同步 |
| 3 阻断 | ADR-0271「已移位」口径 | 照评审写法定稿，移位上限、短款、溢款三式逐字写进 ADR-0271 决定二、决定四、决定五与索引注记，CR CONTEXT「清分、汇付与差异」，code/14「移位」「差额来源」两项；用 grep 核过三式在三份文件里逐字相同 |
| 4 阻断 | 与 SCENARIOS 场景十七相抵 | 照裁定：短款式扣除经 settlement-accounting 认可的协议抵扣，认可的抵扣额暂留在`渠道在途`、可见为未了结、不落短款；ADR-0271 决定四补这一条，补越权风险点 6（抵扣记账形未裁，待 CR owner 定），Links 补场景十七一行（场景不改）；立 [code/47](../code/47-agreed-cod-offset-has-no-posting-form.md)（`ls` 确认未占，建议 R2，Blocked by CR owner 定形）。CR CONTEXT「抵扣只能依据……」一句末尾指向本上下文的「给开发的交接」，交接表补 code/47 一行——规则句上不直接挂票号，照前几轮 CONTEXT 规则句上不挂票号的口径 |
| 5 阻断 | 交横切漏项 | 补 X7 GLOSSARY「截单快照」、X8「资金冻结」、X9「当前有效客户预估费用」的「报价」、X10「结算政策」与「对账安排」撞义，各写位置、引文、改法、依据 |
| 建议 | code/14 差额来源与 ADR 统一 | 写成两者金额都等于差异事项金额，差异事项金额按三式核 |
| 建议 | 停在余额不足的到账时不判短款 | ADR-0271 决定四补一条，并进越权风险点 3；CR CONTEXT 与 code/14 同步 |
| 建议 | 结果列与待判断措辞对齐 | UC-SA-002 确认一步改「或保持原阶段（预估或暂估）」、结果列补未配置，待判断结束去掉「账户」并注明归未配置结束；UC-SA-004 待判断结束去掉「币种」并注明跨币种归未配置结束，匹配一步结果列补未配置（`AT-SA-096`）；UC-SA-003 纳入快照一步结果列补全 |
| 顺手 | `receive_supplier_bill.go` 引 `AT-SA-096` 的注释 | 记进 code/45（`BillUndecidedReason` 头注与 `ErrCrossCurrencyMatch` 分支注释） |
| 顺手 | 序号与计数 | code/14「下面「判重」一项」改为「本节「判重」一项」；票面完成记录与处置表里的跨文件计数改为点名；UC 交接表同文件步号保留 |
| 顺手 | `AT-SA-198` 指针 | UC-SA-002 结果表「待判断/冲突/未受理/未形成」一行补：没有在用价卡不在这一行，走 SELL `未形成`连同停因、BUY 停因（`AT-SA-198`） |

修订后的检查（main `e8cd3de65`）：scratchpad 副本上 `go run ./tools/adr-index` 后 `-check` 退 0；改动的 `.md` 链接与锚点全部能解析；0082、0168、0190、0198、0236 去掉 Status 行与 Links 后与 main 逐字节相同；新写的码与符号逐个在 `internal/`、`cmd/` 里确认存在；三式用 grep 证明在 ADR-0271、CR CONTEXT、code/14 逐字一致。

## 评审记录（第 3 轮，2026-10-21）

阻断 1 条、顺手 4 条，全部照办；修订后 rebase 到 main `a304507b3`（其间进 main 的只有 TF 测试一笔，不碰本组文件）。

| 条 | 评审意见 | 处置 |
|---|---|---|
| B1 | code/47 缺「证据」与「最小的诚实修法」 | 补「证据」：`PostingBasisKind` 没有抵扣依据、`FundPosition` 没有抵扣位置、迁移 `collection_remittance/0001` 的 `subledger_posting` 各列 CHECK 封住同一份词表；补「最小的诚实修法」：形定之前代码不动，认可的抵扣额留在`渠道在途`，汇付与对账时要看得出是未了结的认可抵扣、不当作渠道欠款追收 |
| N2 | ADR-0269 决定四那句没覆盖全是已出账、已出账与晚于本期混着 | 后半句改为「一件都入不了本期而其中没有判不出归属日的，也不形成草稿，逐件指名晚于本期或已出账，不答未配置」，ADR-0269 决定四、SA CONTEXT 进入对账单那一条、UC-SA-003「未配置结束」、`AT-SA-213`、code/11 五处逐字同改（grep 整句各命中一次）；ADR-0269 索引注记的缩写同步；`AT-SA-212` 那一格对齐 |
| N3 | 余额不足之后的出路少了短款冲正 | ADR-0271 决定二补「或短款冲正（决定四）回到`渠道在途`后再移」，CR CONTEXT 同义句补「或短款冲正回`渠道在途`（ADR-0271 决定四）」 |
| N4 | code/47 Blocked by | 改为「14；用户（CR owner）决定抵扣记账形（ADR-0271 越权风险点 6）」 |
| N5 | UC-SA-002 BUY 一格的另一个装配点 | 补 `cmd/parcel-settlement-register` 的 `evaluation_resume.go`（读码确认它构造 `TriggerBuyEvaluationOnOccurrenceFormedHandler`） |

检查：scratchpad 副本上 `go run ./tools/adr-index` 后 `-check` 退 0；改过的 `.md` 链接与锚点全部能解析。

