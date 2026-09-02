package postgres_test

import (
	"context"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证节点作业查阅目录的读面：上列经由写侧适配器真实落库
// 的行（测试内插行再读回，票 05），照实转写不重建、租户隔离进 SQL 条件、排序稳定、
// 空册如实交回空列表。读方与写方共用同一个测试库——pgtest.Pool 每次调用都是一个
// 新库，分开建会读到两个世界。

func newReviewCatalogue(t *testing.T) (*adapter.ReviewCatalogue, *adapter.Receptions, *adapter.ConsolidationUnits, bentoapp.Transactor) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	catalogue, err := adapter.NewReviewCatalogue(db)
	if err != nil {
		t.Fatalf("构造查阅目录：%v", err)
	}
	receptions, err := adapter.NewReceptions(db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	consolidations, err := adapter.NewConsolidationUnits(db)
	if err != nil {
		t.Fatalf("构造合箱库：%v", err)
	}
	return catalogue, receptions, consolidations, db.Transactor()
}

func saveReceptions(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.Receptions,
	records ...ports.ReceptionRecord,
) {
	t.Helper()
	inTx(t, transactor, ctx, func(txCtx context.Context) error {
		for _, record := range records {
			if _, err := repository.Save(txCtx, record); err != nil {
				return err
			}
		}
		return nil
	})
}

// TestReviewCatalogueListsReceptionsNewestFirstWithinTenant 证收寄册只列带收寄在场
// 的两格、逐字段照实转写、新近登记在前、租户隔离与页大小都由读口执行。
func TestReviewCatalogueListsReceptionsNewestFirstWithinTenant(t *testing.T) {
	catalogue, receptions, _, transactor := newReviewCatalogue(t)
	ctx := t.Context()

	formed := formedRecord(t, "tenant-a", "scan-1")
	formed.RecordedAt = receivedAt.Add(2 * time.Minute)
	pending := pendingRecord(t, "tenant-a", "scan-2")
	pending.RecordedAt = receivedAt.Add(time.Minute)
	refused := notFormedRecord(t, "tenant-a", "scan-3")
	foreign := formedRecord(t, "tenant-b", "scan-9")
	saveReceptions(t, transactor, ctx, receptions, formed, pending, refused, foreign)

	rows, err := catalogue.ListReceptions(ctx, ref(t, domain.NewTenantID, "tenant-a"), 10)
	if err != nil {
		t.Fatalf("上列收寄册：%v", err)
	}
	if len(rows) != 2 || rows[0].SourceID != "scan-1" || rows[1].SourceID != "scan-2" {
		t.Fatalf("册面行序变形：%+v", rows)
	}

	first := rows[0]
	if first.Kind != "INTAKE_FORMED" ||
		first.Unit != "unit-1" ||
		first.Node != "node-1" ||
		first.DeliveredBy != "courier-1" ||
		!first.ReceivedAt.Equal(receivedAt) ||
		first.ControlKind != "NODE_INTAKE" ||
		!first.ControlEstablishedAt.Equal(receivedAt) ||
		first.ControlReleasedBy != "" ||
		first.ControlReleasedAt != nil ||
		!first.RecordedAt.Equal(formed.RecordedAt) {
		t.Errorf("形成格转写变形：%+v", first)
	}
	if len(first.ServiceMarkers) != 1 || first.ServiceMarkers[0] != "CANCELLED_BEFORE_ARRIVAL" {
		t.Errorf("服务标记转写变形：%v", first.ServiceMarkers)
	}
	if rows[1].Kind != "PENDING_IDENTIFICATION" || rows[1].Unit != "unit-2" {
		t.Errorf("待识别格转写变形：%+v", rows[1])
	}

	limited, err := catalogue.ListReceptions(ctx, ref(t, domain.NewTenantID, "tenant-a"), 1)
	if err != nil || len(limited) != 1 || limited[0].SourceID != "scan-1" {
		t.Errorf("页大小未生效：rows=%+v err=%v", limited, err)
	}
}

// TestReviewCatalogueListsUnidentifiedItemsOnly 证待识别册只列 PENDING_IDENTIFICATION
// 一格：候选与冲突标照登记转写，登记时没有正式关联就是空串，不代填。
func TestReviewCatalogueListsUnidentifiedItemsOnly(t *testing.T) {
	catalogue, receptions, _, transactor := newReviewCatalogue(t)
	ctx := t.Context()

	saveReceptions(t, transactor, ctx, receptions,
		formedRecord(t, "tenant-a", "scan-1"),
		pendingRecord(t, "tenant-a", "scan-2"),
		notFormedRecord(t, "tenant-a", "scan-3"),
	)

	rows, err := catalogue.ListUnidentifiedItems(ctx, ref(t, domain.NewTenantID, "tenant-a"), 10)
	if err != nil {
		t.Fatalf("上列待识别册：%v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("册面行数 = %d，应只有待识别一格", len(rows))
	}
	item := rows[0]
	if item.SourceID != "scan-2" ||
		item.Unit != "unit-2" ||
		item.Node != "node-1" ||
		!item.IdentityConflict ||
		item.Association != "" ||
		!item.ReceivedAt.Equal(receivedAt) {
		t.Errorf("待识别行转写变形：%+v", item)
	}
	if len(item.Candidates) != 2 ||
		item.Candidates[0] != "parcel-7/link-v1" ||
		item.Candidates[1] != "parcel-8/link-v1" {
		t.Errorf("候选转写变形：%v", item.Candidates)
	}
}

// TestReviewCatalogueListsConsolidationUnitsByIdentity 证集运单元册按实例标识稳定
// 上列：三相各自的成员数、最近封签与关闭时刻照行转写，另一个租户的实例不可见。
func TestReviewCatalogueListsConsolidationUnitsByIdentity(t *testing.T) {
	catalogue, _, consolidations, transactor := newReviewCatalogue(t)
	ctx := t.Context()
	tenant := ref(t, domain.NewTenantID, "tenant-a")

	open := openConsolidation(t, "bag-1", "asset-1")
	saveConsolidation(t, transactor, ctx, consolidations, tenant, open)

	sealed := openConsolidation(t, "bag-2", "asset-2")
	saveConsolidation(t, transactor, ctx, consolidations, tenant, sealed)
	if err := sealed.AddMember(ref(t, domain.NewHandlingUnitID, "unit-21")); err != nil {
		t.Fatalf("加入成员：%v", err)
	}
	if err := sealed.AddMember(ref(t, domain.NewHandlingUnitID, "unit-22")); err != nil {
		t.Fatalf("加入成员：%v", err)
	}
	if err := sealed.Seal(
		ref(t, domain.NewSealReference, "seal-1"),
		ref(t, domain.NewWorkBasisReference, "PACK/1"),
		workSource(t, "src-seal-catalogue", consolidationAt),
	); err != nil {
		t.Fatalf("封装：%v", err)
	}
	updateConsolidation(t, transactor, ctx, consolidations, tenant, sealed)

	closed := openConsolidation(t, "bag-3", "asset-3")
	saveConsolidation(t, transactor, ctx, consolidations, tenant, closed)
	if err := closed.AddMember(ref(t, domain.NewHandlingUnitID, "unit-31")); err != nil {
		t.Fatalf("加入成员：%v", err)
	}
	if err := closed.Close(ref(t, domain.NewWorkBasisReference, "DISPOSITION/3"), consolidationAt.Add(time.Hour)); err != nil {
		t.Fatalf("关闭：%v", err)
	}
	updateConsolidation(t, transactor, ctx, consolidations, tenant, closed)

	foreignTenant := ref(t, domain.NewTenantID, "tenant-b")
	foreign := openConsolidation(t, "bag-9", "asset-9")
	saveConsolidation(t, transactor, ctx, consolidations, foreignTenant, foreign)

	rows, err := catalogue.ListConsolidationUnits(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列集运单元册：%v", err)
	}
	if len(rows) != 3 || rows[0].UnitID != "bag-1" || rows[1].UnitID != "bag-2" || rows[2].UnitID != "bag-3" {
		t.Fatalf("册面行序变形：%+v", rows)
	}

	if rows[0].Phase != domain.ConsolidationPhaseOpen ||
		rows[0].Asset != "asset-1" ||
		rows[0].MemberCount != 0 ||
		rows[0].SealCount != 0 ||
		rows[0].LatestSeal != "" ||
		rows[0].LatestSealedAt != nil ||
		rows[0].ClosedAt != nil {
		t.Errorf("开放行转写变形：%+v", rows[0])
	}
	if rows[1].Phase != domain.ConsolidationPhaseSealed ||
		rows[1].MemberCount != 2 ||
		rows[1].SealCount != 1 ||
		rows[1].LatestSeal != "seal-1" ||
		rows[1].LatestSealedAt == nil ||
		!rows[1].LatestSealedAt.Equal(consolidationAt) ||
		rows[1].ClosedAt != nil {
		t.Errorf("封装行转写变形：%+v", rows[1])
	}
	if rows[2].Phase != domain.ConsolidationPhaseClosed ||
		rows[2].MemberCount != 1 ||
		rows[2].ClosedAt == nil ||
		!rows[2].ClosedAt.Equal(consolidationAt.Add(time.Hour)) {
		t.Errorf("关闭行转写变形：%+v", rows[2])
	}
}

// TestReviewCatalogueRejectsNonPositiveLimitAndAnswersEmptyHonestly 证读口只拒绝无
// 意义的页大小；空登记册如实交回空列表——空册是内容，不是错误（ADR-0077 Decision 四）。
func TestReviewCatalogueRejectsNonPositiveLimitAndAnswersEmptyHonestly(t *testing.T) {
	catalogue, _, _, _ := newReviewCatalogue(t)
	ctx := t.Context()
	tenant := ref(t, domain.NewTenantID, "tenant-a")

	if _, err := catalogue.ListReceptions(ctx, tenant, 0); err == nil {
		t.Error("零页大小的收寄上列没有被拒")
	}
	if _, err := catalogue.ListUnidentifiedItems(ctx, tenant, -1); err == nil {
		t.Error("负页大小的待识别上列没有被拒")
	}
	if _, err := catalogue.ListConsolidationUnits(ctx, tenant, 0); err == nil {
		t.Error("零页大小的集运上列没有被拒")
	}

	receptions, err := catalogue.ListReceptions(ctx, tenant, 5)
	if err != nil || len(receptions) != 0 {
		t.Errorf("空收寄册：rows=%+v err=%v", receptions, err)
	}
	items, err := catalogue.ListUnidentifiedItems(ctx, tenant, 5)
	if err != nil || len(items) != 0 {
		t.Errorf("空待识别册：rows=%+v err=%v", items, err)
	}
	units, err := catalogue.ListConsolidationUnits(ctx, tenant, 5)
	if err != nil || len(units) != 0 {
		t.Errorf("空集运册：rows=%+v err=%v", units, err)
	}
}
