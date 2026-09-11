package postgres_test

import (
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

func newObligationInventoryView(t *testing.T) (*adapter.ObligationInventoryView, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	view, err := adapter.NewObligationInventoryView(fixture.db)
	if err != nil {
		t.Fatalf("构造义务盘点读口：%v", err)
	}
	return view, fixture
}

func loadObligations(
	t *testing.T,
	view *adapter.ObligationInventoryView,
	tenant, caseRef string,
	cutoff time.Time,
) ([]domain.ClosureObligationItem, bool) {
	t.Helper()
	items, configured, err := view.LoadObligationItems(t.Context(),
		viewValue(t, domain.NewTenantID, tenant), viewValue(t, domain.NewCustomsCaseID, caseRef), cutoff)
	if err != nil {
		t.Fatalf("盘义务：%v", err)
	}
	return items, configured
}

func seedObligationCatalog(t *testing.T, fixture *viewFixture, tenant, caseRef string) {
	t.Helper()
	fixture.seed(t,
		`INSERT INTO customs_compliance.closure_obligation_catalog (tenant_id, case_ref, registered_at)
		 VALUES ($1, $2, $3)`, tenant, caseRef, viewBaseAt)
}

// 目录未登记 → 未决。绝不是「没有义务所以可关」（CONTEXT「关闭前必须在明确业务截点盘点全部适用」义务）。
func TestObligationInventoryIsUnconfiguredWithoutACatalog(t *testing.T) {
	view, _ := newObligationInventoryView(t)
	items, configured := loadObligations(t, view, "tenant-a", "case-1", viewBaseAt)
	if configured || len(items) != 0 {
		t.Fatalf("没有目录却答出了清单：configured=%v items=%d", configured, len(items))
	}
}

// 目录登记了而本截点无适用项 → configured=true + 空清单。这与上一条是不同的答案，
// 两者的续办动作也不同；分两张表存正是为了让它们分得开。
func TestARegisteredCatalogWithNoApplicableItemIsConfiguredAndEmpty(t *testing.T) {
	view, fixture := newObligationInventoryView(t)
	seedObligationCatalog(t, fixture, "tenant-a", "case-1")

	items, configured := loadObligations(t, view, "tenant-a", "case-1", viewBaseAt)
	if !configured {
		t.Fatal("目录已登记却答未配置")
	}
	if len(items) != 0 {
		t.Fatalf("空目录盘出了 %d 项", len(items))
	}
}

func TestObligationItemsAreReadBackWithStateAndHandover(t *testing.T) {
	view, fixture := newObligationInventoryView(t)
	seedObligationCatalog(t, fixture, "tenant-a", "case-1")
	fixture.seed(t,
		`INSERT INTO customs_compliance.closure_obligation_item
			(tenant_id, case_ref, obligation, scope, item_state, basis, handed_to, applies_from)
		 VALUES
			('tenant-a', 'case-1', 'DUTY-SETTLEMENT', 'parcel-1', 'CONCLUDED', 'duty/paid', NULL, $1),
			('tenant-a', 'case-1', 'RECORD-KEEPING', 'parcel-1', 'HANDED_OVER', 'handover/v1', 'broker-x', $1)`,
		viewBaseAt)

	items, configured := loadObligations(t, view, "tenant-a", "case-1", viewBaseAt.Add(time.Hour))
	if !configured || len(items) != 2 {
		t.Fatalf("清单没读回：configured=%v items=%d", configured, len(items))
	}
	// ORDER BY obligation：DUTY-SETTLEMENT 在前。
	if items[0].State != domain.ObligationConcluded || items[0].HandedTo != "" {
		t.Fatalf("已终结项走样：%+v", items[0])
	}
	if items[1].State != domain.ObligationHandedOver || items[1].HandedTo != "broker-x" {
		t.Fatalf("承接项走样：%+v", items[1])
	}

	// 逐项完整即可进领域盘点；未解决项在场时关闭被挡住，这条链在这里连通。
	verification, err := domain.VerifyClosure(viewValue(t, domain.NewCustomsCaseID, "case-1"),
		viewBaseAt.Add(time.Hour), items, viewBaseAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("读回的清单进不了领域盘点：%v", err)
	}
	if !verification.Closable() {
		t.Fatal("两项都已了结却判不可关")
	}
}

// 截点是盘点的时间边界：还没生效与已经失效的义务都不进本次清单。
func TestObligationInventoryRespectsTheBusinessCutoff(t *testing.T) {
	view, fixture := newObligationInventoryView(t)
	seedObligationCatalog(t, fixture, "tenant-a", "case-1")
	fixture.seed(t,
		`INSERT INTO customs_compliance.closure_obligation_item
			(tenant_id, case_ref, obligation, scope, item_state, basis, applies_from, applies_until)
		 VALUES
			('tenant-a', 'case-1', 'EXPIRED', 'parcel-1', 'UNRESOLVED', 'b', $1, $2),
			('tenant-a', 'case-1', 'FUTURE', 'parcel-1', 'UNRESOLVED', 'b', $3, NULL),
			('tenant-a', 'case-1', 'CURRENT', 'parcel-1', 'UNRESOLVED', 'b', $1, NULL)`,
		viewBaseAt, viewBaseAt.Add(time.Hour), viewBaseAt.Add(4*time.Hour))

	items, configured := loadObligations(t, view, "tenant-a", "case-1", viewBaseAt.Add(2*time.Hour))
	if !configured || len(items) != 1 || items[0].Obligation != "CURRENT" {
		t.Fatalf("截点没生效：configured=%v items=%+v", configured, items)
	}
}

func TestObligationInventoryOfAnotherTenantIsInvisible(t *testing.T) {
	view, fixture := newObligationInventoryView(t)
	seedObligationCatalog(t, fixture, "tenant-a", "case-1")

	if _, configured := loadObligations(t, view, "tenant-b", "case-1", viewBaseAt); configured {
		t.Fatal("跨租户可见")
	}
}

func TestObligationItemChecksPinHandoverAndCatalogPresence(t *testing.T) {
	_, fixture := newObligationInventoryView(t)
	seedObligationCatalog(t, fixture, "tenant-a", "case-1")

	fixture.rejects(t, "承接项没有指名接收责任方",
		`INSERT INTO customs_compliance.closure_obligation_item
			(tenant_id, case_ref, obligation, scope, item_state, basis, applies_from)
		 VALUES ('tenant-a', 'case-1', 'O', 's', 'HANDED_OVER', 'b', now())`)
	fixture.rejects(t, "非承接项却带了接收方",
		`INSERT INTO customs_compliance.closure_obligation_item
			(tenant_id, case_ref, obligation, scope, item_state, basis, handed_to, applies_from)
		 VALUES ('tenant-a', 'case-1', 'O', 's', 'CONCLUDED', 'b', 'broker-x', now())`)
	fixture.rejects(t, "封闭三值之外的状态",
		`INSERT INTO customs_compliance.closure_obligation_item
			(tenant_id, case_ref, obligation, scope, item_state, basis, applies_from)
		 VALUES ('tenant-a', 'case-1', 'O', 's', 'PROBABLY_FINE', 'b', now())`)
	fixture.rejects(t, "明细先于目录存在",
		`INSERT INTO customs_compliance.closure_obligation_item
			(tenant_id, case_ref, obligation, scope, item_state, basis, applies_from)
		 VALUES ('tenant-a', 'case-nope', 'O', 's', 'CONCLUDED', 'b', now())`)
}
