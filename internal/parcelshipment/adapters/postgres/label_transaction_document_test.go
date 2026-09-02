package postgres_test

import (
	"context"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件对真实 PostgreSQL 证渠道载荷记录的快照往返与读面（ADR-0092）。
//
// 载荷不另建表：裁决是只落**引用**，而引用是小值，走聚合快照与本聚合其余追加式清单同形。
// 因此本票没有新迁移——那不是漏做，是「本体入库」被否决之后随之取消的一步；证据改由这一组
// 真库往返承担，它证的正是同一件事：写下去的与读回来的是同一份。

func mustSaveLabelTransaction(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.LabelTransactions,
	transaction domain.LabelTransaction,
) {
	t.Helper()

	var outcome ports.LabelTransactionSaveOutcome
	mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Save(txCtx, transaction)
		return err
	})
	if outcome != ports.LabelTransactionSaved {
		t.Fatalf("保存结果 = %s，want SAVED", outcome)
	}
}

func labelDocumentSpec(
	t *testing.T,
	granularity domain.LabelDocumentGranularity,
	digest string,
	observedAt time.Time,
	parcels ...string,
) domain.RecordLabelDocumentSpec {
	t.Helper()

	covered := make([]domain.DeclaredParcelID, 0, len(parcels))
	for _, parcel := range parcels {
		covered = append(covered, mustBuild(t, domain.NewDeclaredParcelID, parcel))
	}
	return domain.RecordLabelDocumentSpec{
		Role:           mustBuild(t, domain.NewLabelDocumentRole, "shipping-label"),
		Format:         mustBuild(t, domain.NewLabelDocumentFormat, "channel-declared-format"),
		Granularity:    granularity,
		CoveredParcels: covered,
		Digest:         mustBuild(t, domain.NewLabelDocumentDigest, digest),
		ObservedAt:     observedAt,
	}
}

// 一份批粒度件覆盖两件包裹，且本体无存放处——这正是文件组件就位之前唯一走得到的形态。
// 往返之后粒度、覆盖范围与摘要都还在，而本体仍如实报告未存放。
func TestABatchDocumentSurvivesTheSnapshotRoundTripWithoutABody(t *testing.T) {
	repository, views, transactor := newLabelTransactions(t)
	ctx := t.Context()

	submittedAt := labelEstablishedAt.Add(time.Minute)
	observedAt := labelEstablishedAt.Add(2 * time.Minute)

	transaction := establishedLabelTransactionFixture(t, "tenant-doc-a", "label-doc-1", "parcel-1", "parcel-2")
	submitted, err := transaction.SubmitToChannel(submittedAt)
	if err != nil {
		t.Fatalf("提交渠道：%v", err)
	}
	withDocument, err := submitted.AppendLabelDocument(
		labelDocumentSpec(t, domain.LabelDocumentPerBatch, "digest-batch", observedAt, "parcel-1", "parcel-2"),
	)
	if err != nil {
		t.Fatalf("追加批粒度载荷：%v", err)
	}
	mustInsertLabelTransaction(t, transactor, ctx, repository, withDocument)

	readBack, found, err := repository.FindByID(ctx,
		mustBuild(t, domain.NewTenantID, "tenant-doc-a"),
		mustBuild(t, domain.NewLabelTransactionID, "label-doc-1"),
	)
	if err != nil {
		t.Fatalf("读回交易：%v", err)
	}
	if !found {
		t.Fatal("交易读不回来")
	}

	documents := readBack.LabelDocuments()
	if len(documents) != 1 {
		t.Fatalf("应读回一条载荷记录，实得 %d 条", len(documents))
	}
	record := documents[0]
	if got := record.Granularity(); got != domain.LabelDocumentPerBatch {
		t.Errorf("粒度 = %q，想要 PER_BATCH", got)
	}
	if got := len(record.CoveredParcels()); got != 2 {
		t.Errorf("覆盖范围应有两件，实得 %d 件", got)
	}
	if record.Digest().String() != "digest-batch" {
		t.Errorf("摘要 = %q，想要 digest-batch", record.Digest().String())
	}
	if record.BodyStored() {
		t.Error("本体无存放处时不该报告已存放")
	}

	records, err := views.ListLabelTransactions(ctx, mustBuild(t, domain.NewTenantID, "tenant-doc-a"), 10)
	if err != nil {
		t.Fatalf("读面查阅：%v", err)
	}
	if len(records) != 1 {
		t.Fatalf("读面应交回一笔，实得 %d 笔", len(records))
	}
	rows := records[0].Documents
	if len(rows) != 1 {
		t.Fatalf("读面应交回一条载荷行，实得 %d 条", len(rows))
	}
	// 批粒度件在读面上仍是**一行**、自带覆盖范围，不按包裹复制成两行——复制之后没有任何
	// 东西说得出这两行其实是同一张纸。
	if got := len(rows[0].CoveredParcels); got != 2 {
		t.Errorf("读面那一行应自带两件覆盖范围，实得 %d 件", got)
	}
	if rows[0].BodyStored {
		t.Error("读面不该报告本体已存放")
	}
	if got := len(records[0].Parcels); got != 2 {
		t.Errorf("包裹行仍逐件一行，实得 %d 行", got)
	}
}

// 追加第二条之后原条仍在，且顺序即追加顺序。读面据此才挑得出「客户手上那张纸是哪一版」；
// 顶替式写法在这里会只剩一条。
func TestASecondDocumentIsAppendedInsteadOfReplacingTheFirst(t *testing.T) {
	repository, views, transactor := newLabelTransactions(t)
	ctx := t.Context()

	submittedAt := labelEstablishedAt.Add(time.Minute)
	transaction := establishedLabelTransactionFixture(t, "tenant-doc-b", "label-doc-2", "parcel-1")
	submitted, err := transaction.SubmitToChannel(submittedAt)
	if err != nil {
		t.Fatalf("提交渠道：%v", err)
	}
	first, err := submitted.AppendLabelDocument(
		labelDocumentSpec(t, domain.LabelDocumentPerParcel, "digest-first", labelEstablishedAt.Add(2*time.Minute), "parcel-1"),
	)
	if err != nil {
		t.Fatalf("追加第一条：%v", err)
	}
	mustInsertLabelTransaction(t, transactor, ctx, repository, first)

	// 先读回再追加：写入后的版本由库给出，内存里那份仍停在未持久化的零版本。这一步不是绕路，
	// 它就是真实的推进路径——Save 的预期版本必须来自库。
	persisted, found, err := repository.FindByID(ctx,
		mustBuild(t, domain.NewTenantID, "tenant-doc-b"),
		mustBuild(t, domain.NewLabelTransactionID, "label-doc-2"),
	)
	if err != nil || !found {
		t.Fatalf("读回首次写入：err=%v found=%v", err, found)
	}

	reprint := labelDocumentSpec(t, domain.LabelDocumentPerParcel, "digest-reprint", labelEstablishedAt.Add(3*time.Minute), "parcel-1")
	reprint.Locator = mustBuild(t, domain.NewLabelDocumentLocator, "store://label/reprint")
	second, err := persisted.AppendLabelDocument(reprint)
	if err != nil {
		t.Fatalf("追加第二条：%v", err)
	}
	mustSaveLabelTransaction(t, transactor, ctx, repository, second)

	readBack, found, err := repository.FindByID(ctx,
		mustBuild(t, domain.NewTenantID, "tenant-doc-b"),
		mustBuild(t, domain.NewLabelTransactionID, "label-doc-2"),
	)
	if err != nil {
		t.Fatalf("读回交易：%v", err)
	}
	if !found {
		t.Fatal("交易读不回来")
	}

	documents := readBack.LabelDocuments()
	if len(documents) != 2 {
		t.Fatalf("重打之后应有两条载荷记录，实得 %d 条", len(documents))
	}
	if documents[0].Digest().String() != "digest-first" || documents[1].Digest().String() != "digest-reprint" {
		t.Errorf("顺序应即追加顺序，实得 %q、%q",
			documents[0].Digest().String(), documents[1].Digest().String())
	}
	if documents[0].BodyStored() {
		t.Error("第一条本体无存放处")
	}
	if !documents[1].BodyStored() {
		t.Error("第二条带定位符，本体应报告已存放")
	}

	records, err := views.ListLabelTransactions(ctx, mustBuild(t, domain.NewTenantID, "tenant-doc-b"), 10)
	if err != nil {
		t.Fatalf("读面查阅：%v", err)
	}
	if got := len(records[0].Documents); got != 2 {
		t.Errorf("读面应交回两条载荷行，实得 %d 条", got)
	}
}

// `0010` 之后写下的旧快照没有载荷字段。读回来应当是一笔没有载荷的交易——那正是它当时的真相，
// 不必迁移改写历史行，也不该因为少一个字段就读不回来。
func TestASnapshotWrittenBeforeDocumentsExistedStillReadsBack(t *testing.T) {
	repository, _, transactor := newLabelTransactions(t)
	ctx := t.Context()

	transaction := establishedLabelTransactionFixture(t, "tenant-doc-c", "label-doc-3", "parcel-1")
	mustInsertLabelTransaction(t, transactor, ctx, repository, transaction)

	readBack, found, err := repository.FindByID(ctx,
		mustBuild(t, domain.NewTenantID, "tenant-doc-c"),
		mustBuild(t, domain.NewLabelTransactionID, "label-doc-3"),
	)
	if err != nil {
		t.Fatalf("读回交易：%v", err)
	}
	if !found {
		t.Fatal("交易读不回来")
	}
	if got := len(readBack.LabelDocuments()); got != 0 {
		t.Errorf("无载荷的一行应读回零条，实得 %d 条", got)
	}
}
