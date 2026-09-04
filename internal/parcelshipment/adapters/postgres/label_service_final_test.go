package postgres_test

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证面单渠道服务终局（票 label-channel/11）的库面两件：按包裹取全部
// 相关面单交易的只读口整册交回、不筛、按建立时间序，且租户隔离；final_outcome 收下面单渠道两种
// 责任结果种类并按当前有效终局读回（委托完成派生与取消核验读的就是这一口）。

// Covers: CONTEXT「任何已经实际形成且归属该包裹的面单结果都必须参与终局判断」的读口面——
// 覆盖 parcel-1 的三笔（含多包裹交易）整册回来、按建立时间序；只覆盖别的包裹的那笔不来；
// 另一个租户按同一包裹号读到空册。
func TestLabelTransactionsCoveringAParcelComeBackAsAWholeRegister(t *testing.T) {
	repository, _, transactor := newLabelTransactions(t)
	ctx := t.Context()

	first := establishedLabelTransactionFixture(t, "tenant-1", "label-txn-1", "parcel-1")
	second := establishedLabelTransactionFixture(t, "tenant-1", "label-txn-2", "parcel-2", "parcel-1")
	unrelated := establishedLabelTransactionFixture(t, "tenant-1", "label-txn-3", "parcel-9")
	third := recordedLabelTransactionFixture(t, "tenant-1", "label-txn-4")
	for _, transaction := range []domain.LabelTransaction{first, second, unrelated, third} {
		mustInsertLabelTransaction(t, transactor, ctx, repository, transaction)
	}

	var view ports.LabelTransactionsByParcelView = repository
	covering, err := view.ListByCoveredParcel(ctx,
		mustBuild(t, domain.NewTenantID, "tenant-1"), mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"))
	if err != nil {
		t.Fatalf("按包裹取交易：%v", err)
	}
	// 计数锚定本夹具：三笔覆盖 parcel-1（其中一笔同时覆盖 parcel-2），一笔只覆盖 parcel-9。
	if len(covering) != 3 {
		t.Fatalf("covering = %d, want 3", len(covering))
	}
	for _, transaction := range covering {
		if transaction.ID().String() == "label-txn-3" {
			t.Fatal("只覆盖别的包裹的交易被算进了本包裹")
		}
	}
	// 记过结果与作废的那一笔要整份回来——判断读的是它的定案、包裹结果与后续动作。
	var recorded domain.LabelTransaction
	for _, transaction := range covering {
		if transaction.ID().String() == "label-txn-4" {
			recorded = transaction
		}
	}
	if !recorded.Finalized() || len(recorded.FollowUpActions()) != 1 {
		t.Fatalf("已定案带作废的交易没有整份回来：%+v", recorded)
	}

	other, err := view.ListByCoveredParcel(ctx,
		mustBuild(t, domain.NewTenantID, "tenant-b"), mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"))
	if err != nil || len(other) != 0 {
		t.Fatalf("他租户按同一包裹号读到了 %d 笔（err=%v）", len(other), err)
	}
}

// Covers: 库表封闭集与领域 ResponsibilityOutcomeKind 是同一个集合的两份（迁移 0014）——面单渠道
// 服务两种来源的终局能落 final_outcome 并作为当前有效终局读回；集合外的种类仍被 CHECK 拒。
func TestALabelServiceFinalIsStoredAsTheCurrentFinal(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	finals, err := adapter.NewFinalOutcomes(db)
	if err != nil {
		t.Fatalf("构造终局库：%v", err)
	}
	ctx := t.Context()
	tenant := mustBuild(t, domain.NewTenantID, "tenant-1")
	parcel := mustBuild(t, domain.NewDeclaredParcelID, "parcel-1")

	source, err := domain.NewResponsibilityOutcome(domain.ResponsibilityOutcomeSpec{
		Kind:       domain.LabelServiceOutcome,
		Parcel:     parcel,
		Decision:   mustBuild(t, domain.NewResponsibilityDecisionReference, "LABEL-SERVICE-FINAL/parcel-1/FINAL_BY_FIRST_PICKUP/TF-TRACK-FACT-7@v1"),
		Execution:  mustBuild(t, domain.NewExecutionEvidenceReference, "TF-TRACK-FACT-7@v1"),
		Version:    mustBuild(t, domain.NewResponsibilityOutcomeVersion, "v1"),
		OccurredAt: time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("责任结果：%v", err)
	}
	final, err := domain.FormParcelFinalOutcome(domain.ParcelFinalOutcomeSpec{
		Version:     mustBuild(t, domain.NewFinalOutcomeVersionID, "final-1"),
		Parcel:      parcel,
		Kind:        mustBuild(t, domain.NewFinalKindReference, "LABEL_SERVICE_FINAL"),
		Source:      source,
		RuleVersion: mustBuild(t, domain.NewFinalRuleVersionReference, "PAR-COM-17/LABEL_SERVICE_FINAL"),
	})
	if err != nil {
		t.Fatalf("形成终局：%v", err)
	}
	record := ports.FinalOutcomeRecord{
		Key: ports.FinalAdoptionKey{
			TenantID: tenant, Parcel: parcel, Kind: source.Kind(), Version: source.Version(),
		},
		ContentDigest: "digest-label-1",
		Finalized:     true,
		Final:         final,
		AdoptedAt:     time.Date(2026, 9, 12, 10, 5, 0, 0, time.UTC),
	}
	var saved ports.FinalOutcomeSaveOutcome
	mustWithinTransaction(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		var err error
		saved, err = finals.Save(txCtx, record)
		return err
	})
	if saved != ports.FinalOutcomeSaved {
		t.Fatalf("save outcome = %d", saved)
	}

	current, found, err := finals.FindCurrentFinal(ctx, tenant, parcel)
	if err != nil || !found || !current.Finalized {
		t.Fatalf("当前有效终局：found=%v finalized=%v err=%v", found, current.Finalized, err)
	}
	if current.Final.Source().Kind() != domain.LabelServiceOutcome ||
		current.Final.Source().Execution().String() != "TF-TRACK-FACT-7@v1" {
		t.Fatalf("面单渠道来源没有原样读回：%+v", current.Final.Source())
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_shipment.final_outcome
			(tenant_id, parcel_id, outcome_kind, outcome_version, content_digest, finalized, is_current,
			 refusal_basis, adopted_at)
		 VALUES ('tenant-x', 'parcel-x', 'SHIFT_COMPLETED', 'v1', 'digest-x', false, false, 'basis', now())`); err == nil {
		t.Fatal("集合外的责任结果种类进了终局库——封闭集在库面没守住")
	}
}
