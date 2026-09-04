package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证面单交易聚合的持久化与查阅（ADR-0084 决定七/八）：全聚合
// 快照往返不丢双层结果与追加式后续动作、租户隔离由 SQL 条件承担、重复建立由主键拦住、
// 并发保存由乐观版本拦住、读面按「交易 × 包裹」摊开且分得清「还没有结果」与「未受理」。

var labelEstablishedAt = time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)

func newLabelTransactions(t *testing.T) (*adapter.LabelTransactions, *adapter.LabelTransactionViews, bentoapp.Transactor) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewLabelTransactions(db)
	if err != nil {
		t.Fatalf("构造面单交易仓储：%v", err)
	}
	views, err := adapter.NewLabelTransactionViews(db)
	if err != nil {
		t.Fatalf("构造面单交易读面：%v", err)
	}
	return repository, views, db.Transactor()
}

// establishedLabelTransactionFixture 造一笔刚建立的交易：租户、交易号与覆盖包裹按参数给，
// 其余七项固定依据取同一批夹具值。
func establishedLabelTransactionFixture(
	t *testing.T,
	tenant, transactionID string,
	parcels ...string,
) domain.LabelTransaction {
	t.Helper()

	covered := make([]domain.DeclaredParcelID, len(parcels))
	for index, parcel := range parcels {
		covered[index] = mustBuild(t, domain.NewDeclaredParcelID, parcel)
	}
	transaction, err := domain.EstablishLabelTransaction(domain.EstablishLabelTransactionSpec{
		Tenant:                 mustBuild(t, domain.NewTenantID, tenant),
		ID:                     mustBuild(t, domain.NewLabelTransactionID, transactionID),
		CoveredParcels:         covered,
		ChannelAccount:         mustBuild(t, domain.NewChannelAccountReference, "channel-account-1"),
		AccountHolder:          mustBuild(t, domain.NewChannelAccountHolderReference, "party-holder-1"),
		ServiceProvider:        mustBuild(t, domain.NewChannelServiceProviderReference, "party-channel-1"),
		SettlementCounterparty: mustBuild(t, domain.NewSettlementCounterpartyReference, "party-settlement-1"),
		Contract:               mustBuild(t, domain.NewChannelContractReference, "contract-1"),
		Rate:                   mustBuild(t, domain.NewChannelRateReference, "rate-1"),
		ResponsibilityBasis:    mustBuild(t, domain.NewResponsibilityBasisSnapshotReference, "basis-snapshot-1"),
		EstablishedAt:          labelEstablishedAt,
	})
	if err != nil {
		t.Fatalf("建立面单交易：%v", err)
	}
	return transaction
}

// recordedLabelTransactionFixture 把交易推到「部分成功 + 一条指名包裹作废」：两层结果与
// 追加式清单都在，往返验的就是这些东西一样不少地回来。
func recordedLabelTransactionFixture(t *testing.T, tenant, transactionID string) domain.LabelTransaction {
	t.Helper()

	submitted, err := establishedLabelTransactionFixture(t, tenant, transactionID, "parcel-1", "parcel-2").
		SubmitToChannel(labelEstablishedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("提交渠道：%v", err)
	}
	recorded, err := submitted.RecordChannelResult(domain.RecordChannelResultSpec{
		Outcome: domain.LabelTransactionPartiallySucceeded,
		ParcelResults: []domain.LabelTransactionParcelResultSpec{
			{
				Parcel:     mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"),
				Accepted:   true,
				Identifier: mustBuild(t, domain.NewChannelParcelIdentifier, "channel-parcel-1"),
			},
			{
				Parcel: mustBuild(t, domain.NewDeclaredParcelID, "parcel-2"),
				Reason: mustBuild(t, domain.NewChannelResultReasonReference, "ADDRESS_UNSUPPORTED"),
			},
		},
		ObservedAt: labelEstablishedAt.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("记录渠道结果：%v", err)
	}
	voided, err := recorded.AppendFollowUpAction(domain.FollowUpActionSpec{
		Kind:       domain.ChannelVoidAction,
		Parcels:    []domain.DeclaredParcelID{mustBuild(t, domain.NewDeclaredParcelID, "parcel-1")},
		Reason:     mustBuild(t, domain.NewChannelResultReasonReference, "CUSTOMER_WITHDREW"),
		OccurredAt: labelEstablishedAt.Add(3 * time.Minute),
	})
	if err != nil {
		t.Fatalf("追加后续动作：%v", err)
	}
	return voided
}

func mustInsertLabelTransaction(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.LabelTransactions,
	transaction domain.LabelTransaction,
) {
	t.Helper()

	var outcome ports.LabelTransactionInsertOutcome
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Insert(txCtx, transaction)
		return err
	})
	if outcome != ports.LabelTransactionInserted {
		t.Fatalf("建立写入结果 = %s，want INSERTED", outcome)
	}
}

// TestALabelTransactionRoundTripsWholly 证双层结果、追加式后续动作与关系出处随快照往返。
// 丢任何一样都不是小事：丢包裹级结果，「不能由整笔交易结果推断」的那一层就没了；丢后续
// 动作，一笔已作废的面单读回来像仍然有效。
func TestALabelTransactionRoundTripsWholly(t *testing.T) {
	repository, _, transactor := newLabelTransactions(t)
	ctx := t.Context()

	original := recordedLabelTransactionFixture(t, "tenant-a", "label-txn-1")
	mustInsertLabelTransaction(t, transactor, ctx, repository, original)

	found, exists, err := repository.FindByID(ctx,
		mustBuild(t, domain.NewTenantID, "tenant-a"),
		mustBuild(t, domain.NewLabelTransactionID, "label-txn-1"))
	if err != nil {
		t.Fatalf("取回面单交易：%v", err)
	}
	if !exists {
		t.Fatal("已建立的面单交易读不回来")
	}
	if found.Revision() != 1 {
		t.Fatalf("revision = %d, want 1（Insert 写 1）", found.Revision())
	}
	if found.State() != domain.LabelTransactionPartiallySucceeded || !found.Finalized() {
		t.Fatalf("交易级结果往返变形：state = %s", found.State())
	}
	if !found.EstablishedAt().Equal(original.EstablishedAt()) ||
		!found.SubmittedAt().Equal(original.SubmittedAt()) ||
		!found.ResultObservedAt().Equal(original.ResultObservedAt()) {
		t.Fatalf("三个业务时间往返变形：%s / %s / %s",
			found.EstablishedAt(), found.SubmittedAt(), found.ResultObservedAt())
	}
	if found.ChannelAccount() != original.ChannelAccount() ||
		found.AccountHolder() != original.AccountHolder() ||
		found.ServiceProvider() != original.ServiceProvider() ||
		found.SettlementCounterparty() != original.SettlementCounterparty() ||
		found.Contract() != original.Contract() ||
		found.Rate() != original.Rate() ||
		found.ResponsibilityBasis() != original.ResponsibilityBasis() {
		t.Fatalf("建立时固定的依据往返变形")
	}
	if len(found.CoveredParcels()) != 2 {
		t.Fatalf("覆盖范围往返变形：%v", found.CoveredParcels())
	}

	accepted, present := found.ParcelResult(mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"))
	if !present || !accepted.Accepted() || accepted.Identifier().String() != "channel-parcel-1" {
		t.Fatalf("受理包裹的结果往返变形：%+v", accepted)
	}
	refused, present := found.ParcelResult(mustBuild(t, domain.NewDeclaredParcelID, "parcel-2"))
	if !present || refused.Accepted() || refused.Reason().String() != "ADDRESS_UNSUPPORTED" {
		t.Fatalf("未受理包裹的结果往返变形：%+v", refused)
	}

	actions := found.FollowUpActions()
	if len(actions) != 1 || actions[0].Kind() != domain.ChannelVoidAction || actions[0].CoversWholeTransaction() {
		t.Fatalf("后续动作往返变形：%+v", actions)
	}
	if got := actions[0].Parcels(); len(got) != 1 || got[0].String() != "parcel-1" {
		t.Fatalf("后续动作的指名范围往返变形：%v", got)
	}
}

// TestALabelTransactionRetryLinkSurvivesTheRoundTrip 证重试出处随快照往返：丢了它，一笔
// 重试交易读回来看起来像首笔，「跨该包裹全部相关交易」的追溯就断在这一步。
func TestALabelTransactionRetryLinkSurvivesTheRoundTrip(t *testing.T) {
	repository, _, transactor := newLabelTransactions(t)
	ctx := t.Context()

	prior := recordedLabelTransactionFixture(t, "tenant-a", "label-txn-1")
	mustInsertLabelTransaction(t, transactor, ctx, repository, prior)

	link, err := domain.EstablishPriorLabelTransactionLink(prior, domain.LabelTransactionRetry)
	if err != nil {
		t.Fatalf("建立重试关系：%v", err)
	}
	retrySpec := domain.EstablishLabelTransactionSpec{
		Tenant:                 mustBuild(t, domain.NewTenantID, "tenant-a"),
		ID:                     mustBuild(t, domain.NewLabelTransactionID, "label-txn-2"),
		CoveredParcels:         []domain.DeclaredParcelID{mustBuild(t, domain.NewDeclaredParcelID, "parcel-2")},
		ChannelAccount:         mustBuild(t, domain.NewChannelAccountReference, "channel-account-1"),
		AccountHolder:          mustBuild(t, domain.NewChannelAccountHolderReference, "party-holder-1"),
		ServiceProvider:        mustBuild(t, domain.NewChannelServiceProviderReference, "party-channel-1"),
		SettlementCounterparty: mustBuild(t, domain.NewSettlementCounterpartyReference, "party-settlement-1"),
		Contract:               mustBuild(t, domain.NewChannelContractReference, "contract-1"),
		Rate:                   mustBuild(t, domain.NewChannelRateReference, "rate-1"),
		ResponsibilityBasis:    mustBuild(t, domain.NewResponsibilityBasisSnapshotReference, "basis-snapshot-1"),
		EstablishedAt:          labelEstablishedAt.Add(time.Hour),
		PriorLink:              link,
	}
	retry, err := domain.EstablishLabelTransaction(retrySpec)
	if err != nil {
		t.Fatalf("建立重试交易：%v", err)
	}
	mustInsertLabelTransaction(t, transactor, ctx, repository, retry)

	found, _, err := repository.FindByID(ctx,
		mustBuild(t, domain.NewTenantID, "tenant-a"),
		mustBuild(t, domain.NewLabelTransactionID, "label-txn-2"))
	if err != nil {
		t.Fatalf("取回重试交易：%v", err)
	}
	borne, established := found.PriorLink()
	if !established || borne.Kind() != domain.LabelTransactionRetry ||
		borne.PriorTransactionID().String() != "label-txn-1" {
		t.Fatalf("重试出处往返变形：%+v, established = %v", borne, established)
	}
}

// TestASecondEstablishOnTheSameKeyAnswersAlreadyExists 证建立只发生一次：同键第二次落在
// `已存在`这个业务答案上，而不是抛错也不是覆盖原交易。
func TestASecondEstablishOnTheSameKeyAnswersAlreadyExists(t *testing.T) {
	repository, _, transactor := newLabelTransactions(t)
	ctx := t.Context()

	first := recordedLabelTransactionFixture(t, "tenant-a", "label-txn-1")
	mustInsertLabelTransaction(t, transactor, ctx, repository, first)

	second := establishedLabelTransactionFixture(t, "tenant-a", "label-txn-1", "parcel-9")
	var outcome ports.LabelTransactionInsertOutcome
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Insert(txCtx, second)
		return err
	})
	if outcome != ports.LabelTransactionAlreadyExists {
		t.Fatalf("第二次建立结果 = %s，want ALREADY_EXISTS", outcome)
	}

	found, _, err := repository.FindByID(ctx,
		mustBuild(t, domain.NewTenantID, "tenant-a"),
		mustBuild(t, domain.NewLabelTransactionID, "label-txn-1"))
	if err != nil {
		t.Fatalf("取回：%v", err)
	}
	if len(found.CoveredParcels()) != 2 || found.State() != domain.LabelTransactionPartiallySucceeded {
		t.Fatalf("第二次建立覆盖了原交易：%+v", found.CoveredParcels())
	}
}

// TestAStaleLabelTransactionSaveAnswersRevisionConflict 证乐观版本：拿一份过期聚合保存，
// 答`版本冲突`让调用方重读再重放，而不是把抢先那一方的写入盖掉。
func TestAStaleLabelTransactionSaveAnswersRevisionConflict(t *testing.T) {
	repository, _, transactor := newLabelTransactions(t)
	ctx := t.Context()

	established := establishedLabelTransactionFixture(t, "tenant-a", "label-txn-1", "parcel-1")
	mustInsertLabelTransaction(t, transactor, ctx, repository, established)

	loaded, _, err := repository.FindByID(ctx,
		mustBuild(t, domain.NewTenantID, "tenant-a"),
		mustBuild(t, domain.NewLabelTransactionID, "label-txn-1"))
	if err != nil {
		t.Fatalf("读回：%v", err)
	}
	submitted, err := loaded.SubmitToChannel(labelEstablishedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("提交渠道：%v", err)
	}
	var first ports.LabelTransactionSaveOutcome
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		first, err = repository.Save(txCtx, submitted)
		return err
	})
	if first != ports.LabelTransactionSaved {
		t.Fatalf("首次保存 = %s，want SAVED", first)
	}

	// loaded 仍停在读出时的版本：同一份再保存一次就是「抢先那一方已经落库」的形态。
	stale, err := loaded.SubmitToChannel(labelEstablishedAt.Add(2 * time.Minute))
	if err != nil {
		t.Fatalf("过期聚合上的转移：%v", err)
	}
	var second ports.LabelTransactionSaveOutcome
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		second, err = repository.Save(txCtx, stale)
		return err
	})
	if second != ports.LabelTransactionRevisionConflict {
		t.Fatalf("过期保存 = %s，want REVISION_CONFLICT", second)
	}

	found, _, err := repository.FindByID(ctx,
		mustBuild(t, domain.NewTenantID, "tenant-a"),
		mustBuild(t, domain.NewLabelTransactionID, "label-txn-1"))
	if err != nil {
		t.Fatalf("取回：%v", err)
	}
	if found.Revision() != 2 || !found.SubmittedAt().Equal(labelEstablishedAt.Add(time.Minute)) {
		t.Fatalf("过期保存把抢先那一方的写入盖掉了：revision = %d, submittedAt = %s",
			found.Revision(), found.SubmittedAt())
	}
}

// TestALabelTransactionIsInvisibleToAnotherTenant 证租户隔离在 SQL 条件上（ADR-0003）：
// 另一个租户读同一个交易号得到否定结果，且否定不区分「不存在」与「属别的租户」。
func TestALabelTransactionIsInvisibleToAnotherTenant(t *testing.T) {
	repository, views, transactor := newLabelTransactions(t)
	ctx := t.Context()

	mustInsertLabelTransaction(t, transactor, ctx, repository,
		recordedLabelTransactionFixture(t, "tenant-a", "label-txn-1"))

	_, exists, err := repository.FindByID(ctx,
		mustBuild(t, domain.NewTenantID, "tenant-b"),
		mustBuild(t, domain.NewLabelTransactionID, "label-txn-1"))
	if err != nil {
		t.Fatalf("按另一租户取回：%v", err)
	}
	if exists {
		t.Fatal("另一个租户读到了不属于它的面单交易")
	}

	records, err := views.ListLabelTransactions(ctx, mustBuild(t, domain.NewTenantID, "tenant-b"), 10)
	if err != nil {
		t.Fatalf("按另一租户列册：%v", err)
	}
	if len(records) != 0 {
		t.Fatalf("另一个租户的册子上出现了 %d 行", len(records))
	}
}

// TestLabelTransactionWritesRefuseToRunOutsideATransaction 证两个写口在无事务上下文被
// RequireExecutor 拒绝：快照与它的键必须同一事务落地，无事务写入会留下半份聚合。
func TestLabelTransactionWritesRefuseToRunOutsideATransaction(t *testing.T) {
	repository, _, _ := newLabelTransactions(t)
	ctx := t.Context()

	transaction := establishedLabelTransactionFixture(t, "tenant-a", "label-txn-1", "parcel-1")
	if _, err := repository.Insert(ctx, transaction); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务建立应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := repository.Save(ctx, transaction); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务保存应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// TestTheLabelTransactionViewExpandsOneRowPerCoveredParcel 证读面按「交易 × 包裹」摊开
// （ADR-0084 决定七），且分得清「还没有结果」与「未受理」——后者是 CONTEXT 禁止的按失败
// 处理在读面这一侧的形态。整笔范围与指名范围的后续动作各自落到该落的行上。
func TestTheLabelTransactionViewExpandsOneRowPerCoveredParcel(t *testing.T) {
	repository, views, transactor := newLabelTransactions(t)
	ctx := t.Context()

	mustInsertLabelTransaction(t, transactor, ctx, repository,
		recordedLabelTransactionFixture(t, "tenant-a", "label-txn-1"))
	// 第二笔停在`已提交渠道`：结果未回的交易同样要上册，且它的两行不能读成「未受理」。
	pending, err := establishedLabelTransactionFixture(t, "tenant-a", "label-txn-0", "parcel-3").
		SubmitToChannel(labelEstablishedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("提交渠道：%v", err)
	}
	mustInsertLabelTransaction(t, transactor, ctx, repository, pending)

	records, err := views.ListLabelTransactions(ctx, mustBuild(t, domain.NewTenantID, "tenant-a"), 10)
	if err != nil {
		t.Fatalf("列册：%v", err)
	}
	if len(records) != 2 {
		t.Fatalf("册上 %d 行，want 2", len(records))
	}
	// 同刻建立时按交易标识倒序：label-txn-1 在前。
	if records[0].TransactionID.String() != "label-txn-1" {
		t.Fatalf("排序不对：%s 在前", records[0].TransactionID)
	}

	recorded := records[0]
	if !recorded.Finalized || recorded.State != domain.LabelTransactionPartiallySucceeded {
		t.Fatalf("定案派生不对：finalized = %v, state = %s", recorded.Finalized, recorded.State)
	}
	if len(recorded.Parcels) != 2 {
		t.Fatalf("摊开后 %d 行，want 2（每件覆盖包裹一行）", len(recorded.Parcels))
	}
	first := recorded.Parcels[0]
	if !first.HasResult || !first.Accepted || first.Identifier != "channel-parcel-1" {
		t.Fatalf("受理行不对：%+v", first)
	}
	if len(first.FollowUpKinds) != 1 || first.FollowUpKinds[0] != domain.ChannelVoidAction {
		t.Fatalf("指名包裹的后续动作没落到该落的行上：%+v", first.FollowUpKinds)
	}
	second := recorded.Parcels[1]
	if !second.HasResult || second.Accepted || second.Reason != "ADDRESS_UNSUPPORTED" {
		t.Fatalf("未受理行不对：%+v", second)
	}
	if len(second.FollowUpKinds) != 0 {
		t.Fatalf("指名范围的后续动作落到了范围外的行上：%+v", second.FollowUpKinds)
	}

	awaiting := records[1]
	if awaiting.Finalized || len(awaiting.Parcels) != 1 {
		t.Fatalf("结果未回的交易摊开不对：finalized = %v, rows = %d", awaiting.Finalized, len(awaiting.Parcels))
	}
	if awaiting.Parcels[0].HasResult {
		t.Fatalf("结果未回的行报告了结果")
	}
	if awaiting.Parcels[0].Accepted {
		t.Fatalf("结果未回的行被读成了受理")
	}
	// 这几件包裹都没开过继续尝试决定册也没有终局：判断派生为开放，且读面要说得出「没有人作过
	// 决定」——开放的另一种来源（关过又重开）在下一条用例里。
	for _, row := range []ports.LabelTransactionParcelRow{awaiting.Parcels[0], recorded.Parcels[0]} {
		if !row.ContinuedAttemptOpen || row.ContinuedAttemptDecided {
			t.Fatalf("没开过册且无终局的包裹应派生开放且无决定历史，实得 %+v", row)
		}
	}
}

// TestTheLabelTransactionViewDerivesContinuedAttemptFromTheRegisterAndTheCurrentFinal 证读面那一格
// 拿的是真输入（票 label-channel/10 第三层）：`包裹级继续尝试判断`按 CONTEXT「只由有效的关闭、重开
// 决定及当前有效终局结果派生」由登记册的 Judge 现算；「没有人作过决定」与「关过又重开」派生出
// 同一格`开放`，读面靠 ContinuedAttemptDecided 把两者交代开；他租户同号包裹上的关闭渗不进来。
func TestTheLabelTransactionViewDerivesContinuedAttemptFromTheRegisterAndTheCurrentFinal(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	transactions, err := adapter.NewLabelTransactions(db)
	if err != nil {
		t.Fatalf("构造面单交易仓储：%v", err)
	}
	views, err := adapter.NewLabelTransactionViews(db)
	if err != nil {
		t.Fatalf("构造面单交易读面：%v", err)
	}
	registers, err := adapter.NewContinuedAttemptRegisters(db)
	if err != nil {
		t.Fatalf("构造继续尝试决定登记册仓储：%v", err)
	}
	finals, err := adapter.NewFinalOutcomes(db)
	if err != nil {
		t.Fatalf("构造终局库：%v", err)
	}
	transactor := db.Transactor()
	ctx := t.Context()

	// 一笔交易覆盖四件包裹，四件各占一格：
	//   parcel-1 没开过册、无终局           → 开放，无决定历史
	//   parcel-2 一条生效关闭               → 受控关闭，有决定历史
	//   parcel-3 关过又重开、无终局         → 开放，有决定历史（与 parcel-1 同格，靠 Decided 分开）
	//   parcel-4 没开过册、当前有效终局在场 → 受控关闭，无决定历史（终局关掉了「允许新尝试」）
	mustInsertLabelTransaction(t, transactor, ctx, transactions,
		establishedLabelTransactionFixture(t, "tenant-a", "label-txn-1", "parcel-1", "parcel-2", "parcel-3", "parcel-4"))
	closed, err := openRegisterFixture(t, "tenant-a", "parcel-2").
		Append(closureDecisionFixture(t, "decision-1", continuedAttemptDecidedAt), false)
	if err != nil {
		t.Fatalf("追加关闭：%v", err)
	}
	mustInsertRegister(t, transactor, ctx, registers, closed)
	mustInsertRegister(t, transactor, ctx, registers, closedThenReopenedFixture(t, "tenant-a", "parcel-3"))
	mustSaveFinal(t, transactor, ctx, finals, firstFinalRecord(t, "tenant-a", "parcel-4", "ORV-1", "digest-4"))
	// 他租户在同号 parcel-1 上的关闭：登记册按（租户 + 包裹）成册（ADR-0003），本租户读不到它。
	foreign, err := openRegisterFixture(t, "tenant-b", "parcel-1").
		Append(closureDecisionFixture(t, "decision-9", continuedAttemptDecidedAt), false)
	if err != nil {
		t.Fatalf("他租户追加关闭：%v", err)
	}
	mustInsertRegister(t, transactor, ctx, registers, foreign)

	records, err := views.ListLabelTransactions(ctx, mustBuild(t, domain.NewTenantID, "tenant-a"), 10)
	if err != nil {
		t.Fatalf("列册：%v", err)
	}
	if len(records) != 1 || len(records[0].Parcels) != 4 {
		t.Fatalf("册上应是一笔交易摊四行，实得 %d 笔", len(records))
	}
	want := []struct {
		parcel  string
		open    bool
		decided bool
	}{
		{"parcel-1", true, false},
		{"parcel-2", false, true},
		{"parcel-3", true, true},
		{"parcel-4", false, false},
	}
	for index, expected := range want {
		row := records[0].Parcels[index]
		if row.Parcel.String() != expected.parcel ||
			row.ContinuedAttemptOpen != expected.open ||
			row.ContinuedAttemptDecided != expected.decided {
			t.Errorf("第 %d 行 want %s open=%v decided=%v，实得 %s open=%v decided=%v",
				index, expected.parcel, expected.open, expected.decided,
				row.Parcel, row.ContinuedAttemptOpen, row.ContinuedAttemptDecided)
		}
	}
}

// TestTheLabelTransactionViewRejectsANonPositiveLimit 证 limit 非正是调用方编程错误：
// 静默答一页会把「忘了传」变成一个没人决定过的页大小。
func TestTheLabelTransactionViewRejectsANonPositiveLimit(t *testing.T) {
	_, views, _ := newLabelTransactions(t)

	if _, err := views.ListLabelTransactions(t.Context(), mustBuild(t, domain.NewTenantID, "tenant-a"), 0); err == nil {
		t.Fatal("limit 为 0 时读面静默答了一页")
	}
}
