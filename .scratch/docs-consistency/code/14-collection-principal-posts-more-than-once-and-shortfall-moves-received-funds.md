# 14 CR 同一笔本金可以多次「自外部来源入账」，`短款`会挪走已到账的本金

Category: bug
Status: ready-for-agent——R1（上线前必修）：碰钱且破坏数据完整性。CR 受托保管的是客户的代收本金：一包裹的同一笔钱经收件人付款、渠道代收报告、渠道回款通知三层各自全额入账，`应付客户`会超过真实到账；短款又从已到账的`待清分`里挪走差额，客户少拿钱。今天记账只走受控 CLI（`cmd/parcel-collection-register`），但守恒不变量一破就无法事后分辨哪一笔是重的，没有可靠绕法。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无——2026-10-21 文档自洽 09 已裁（[ADR-0271](../../../docs/adr/0271-collection-principal-enters-the-ledger-only-on-the-channel-collection-report-and-shortfall-comes-only-from-in-transit.md)），裁向见文末「裁向（文档自洽 09，2026-10-21）」。此前：docs-consistency/09（SA+CR 修订）先裁 [audit/sa-cr.md](../audit/sa-cr.md) H3、H5：四层事实里哪一层负责入账、其余层只做移位或旁证；真实到账遇已由渠道层入账的本金走入账还是走移位；`短款`只记在差异事项上、不占资金位置，还是只允许从「已报告未到账」的`渠道在途`落入。
出处：[audit/sa-cr.md](../audit/sa-cr.md) H3、H5（两条在审计里归 B，建议修法写明「裁定之后另立代码票」；同一上下文、同一根因——记账只核形状不核依据的入账次数与资金来源——合成本票。审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，CR 代码在此期间未改）。

## 现象

1. **重复入账**（H3）：入账只按记账标识判重放；凭同一条代收事实换一个记账标识可以再入一次。三层事实的入账位置都是`渠道在途`，各层各自全额入账，同一笔钱记两三遍。运营企业真实到账既能入账到`待清分`，又能再凭它把`渠道在途`移到`待清分`，那一笔移位的金额不受事实金额约束。
2. **短款挪钱**（H5）：差额那部分钱从没进账，短款记账却不限来源位置；测试里指令 12000、到账 10000，全额入`待清分`后再移 2000 到`短款`，只剩 8000 能进`应付客户`。Lifecycles 也没有任何一条从`短款`出去。

## 证据

- 文档：
  - CR CONTEXT Rules：「资金只能自外部来源入账一次，且入账必须以一笔已接受的代收事实为依据」；「同一笔代收事实不得重复入账」。
  - Language「分配守恒」：「全部非外部资金位置的余额之和恒等于自外部来源入账的金额之和」。
  - Language「资金位置」：「`短款`（实收少于指令的差额已确认落在此处）」；Lifecycles：「`待清分` 或 `应付客户` → `短款`：差异事项已登记且短款结论已确认」。（这两句与守恒相抵，是 H5 要裁的那一处。）
- 代码：
  - `internal/collectionremittance/application/post_subledger.go` 的 `PostSubledger` 按记账标识判重放；`factPostingShapeHolds` 只核去向等于事实的 `IntakePosition()`、金额等于事实金额（外部来源那一支），或只核层次与起讫位置（渠道在途 → 待清分那一支，不核金额上限）；不查这条事实是否已入过账。
  - 同文件 `discrepancyPostingShapeHolds`：去向是事项落点即可，不限来源位置。
  - `migrations/collection_remittance/0001_collection_subledger_and_remittance.sql`：`subledger_posting` 只有主键 `(tenant_id, posting_ref)`，没有按（依据种类，依据引用）的入账唯一约束。
  - `application/handler_test.go` 的 `TestADiscrepancySettlesOnlyThroughItsOwnPosting` 一带即上面那组短款数字。

## 三类

机制：一条事实至多入账一次、移位金额以事实为限、短款的来源位置受限。产品策略：入账层与移位层的划分（待 09 裁）。租户取值：无。

## 规模

M：唯一约束（迁移另取号）与编排判重；移位金额核对；短款记账核来源位置；改写那组短款测试；存量只有演示租户，`--reset` 重灌即可。

## 最小的诚实修法（按 09 的裁定二选一落细节，下列三条不随裁向变）

1. 外部来源 → 任一位置的入账，按（依据种类，依据引用）唯一：第二次换标识入账答`已入账`并回指第一笔，不记账。库上加同一唯一约束，编排判重与约束同一口径。
2. 凭事实的移位，累计金额不得超过该事实金额。
3. 短款记账的来源位置按 09 的裁定限死；不满足的答依据不符，不记账。

## 要裁的

全部在 Blocked by 那一裁里；裁定落在 09 新立的 ADR，本票只照它实现。

## 裁向（文档自洽 09，2026-10-21）

[ADR-0271](../../../docs/adr/0271-collection-principal-enters-the-ledger-only-on-the-channel-collection-report-and-shortfall-comes-only-from-in-transit.md) 裁定，本票照它实现；「最小的诚实修法」第 1 条的答复按下面第 3 条改（换标识再记答内容冲突，不答已入账）：

1. **入账层**：只有渠道代收报告（`CHANNEL_COLLECTION_REPORT`）能自外部来源入账，去向`渠道在途`、金额等于事实金额。`CollectionFact.IntakePosition()` 只对这一层有位置；收件人付款与渠道回款通知凭它记任何一笔都答`NOT_ACCEPTED`。
2. **移位层**：运营企业真实到账（`OPERATOR_BANK_CREDIT`）只收`渠道在途` → `待清分`，金额等于事实金额；不再能自外部来源入账。`渠道在途`不够答`UNDERFUNDED`（余额守卫已有）。
3. **判重**：凭代收事实的记账按（依据种类、依据引用）唯一。同一记账标识重放仍答`EXISTING`；换一个记账标识再凭同一条事实记账答`CONTENT_CONFLICT`，不记账。编排判重先于余额守卫，与库上的唯一约束（迁移另取号）同一口径。
4. **差额来源**：凭短款事项的记账只收`渠道在途` → `短款`；凭溢款事项的落账只收`待清分` → `溢款`（`溢款` → `应付客户` 的归属确认照旧）；不满足答`NOT_ACCEPTED`。领域构造门与迁移 CHECK 各补一份。
5. **测试**：`application/handler_test.go` 的短款用例改成渠道报告入`渠道在途`、到账移位、差额自`渠道在途`落入`短款`；补三层各自入账、同一事实换标识、到账多于报告、溢款自`渠道在途`各一例。演示种子已是这一形，不改。
6. 注释：`IntakePosition`、`factPostingShapeHolds`、`discrepancyPostingShapeHolds` 与 `DiscrepancyItem.SettlementPosition` 的注释随之改；[code/45](./45-sa-cr-comments-carry-counts-that-no-longer-hold.md) 里 `collectionremittance` 那两处可一并改。

建议分级不变（R1）。

