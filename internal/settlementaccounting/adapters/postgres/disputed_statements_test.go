package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// seedStatement 播一张已发布对账单；voided 为真时带作废留痕。
func (fixture *catalogueFixture) seedStatement(t *testing.T, tenant, number string, voided bool) {
	t.Helper()
	var voidBasis, voidedAt any
	if voided {
		voidBasis, voidedAt = "SYN-VOID-"+number, catalogueBaseAt.Add(time.Hour)
	}
	fixture.seed(t,
		`INSERT INTO settlement_accounting.customer_statement
			(tenant_id, statement_number, account_id, period_ref, currency, total_minor,
			 content_digest, lines, adjustment_lines, published_at, void_basis, voided_at, recorded_at)
		 VALUES ($1, $2, 'SYN-ACC-01', 'SYN-PER-01', 'SYN-CUR-01', 1000, 'SYN-DIGEST',
		         '[{"charge":"SYN-CHG-01","amount_minor":1000}]'::jsonb, '[]'::jsonb, $3, $4, $5, $3)`,
		tenant, number, catalogueBaseAt, voidBasis, voidedAt)
}

// seedDispute 播一项挂在 number 上的争议；resolution 空串即未处理。
func (fixture *catalogueFixture) seedDispute(t *testing.T, tenant, number, dispute, resolution string) {
	t.Helper()
	var kind, ref, resolvedAt any
	if resolution != "" {
		kind, ref, resolvedAt = resolution, "SYN-RES-"+dispute, catalogueBaseAt.Add(2*time.Hour)
	}
	fixture.seed(t,
		`INSERT INTO settlement_accounting.statement_dispute
			(tenant_id, dispute_id, statement_number, charge_id, disputed_minor,
			 reason_ref, opened_at, resolution, resolution_ref, resolved_at, content_digest, recorded_at)
		 VALUES ($1, $2, $3, 'SYN-CHG-01', 100, 'SYN-RSN', $4, $5, $6, $7, 'SYN-DIGEST', $4)`,
		tenant, dispute, number, catalogueBaseAt, kind, ref, resolvedAt)
}

// Covers: AT-SA-227 — 待对账的单是至少挂着一项没有终局结果的争议的单：未处理与待复核算，接受、部分接受、
// 拒绝不算；一单多项只算一张；作废的单照算；没有争议的单不算；张数不受页大小截断；他租户的单不进来。
func TestDisputedStatementsAreThoseWithADisputeAwaitingAFinalResolution(t *testing.T) {
	fixture := newCatalogueFixture(t)
	for _, number := range []string{"SYN-STMT-A", "SYN-STMT-B", "SYN-STMT-C", "SYN-STMT-D", "SYN-STMT-E", "SYN-STMT-F"} {
		fixture.seedStatement(t, catalogueTenant, number, number == "SYN-STMT-E")
	}
	fixture.seedDispute(t, catalogueTenant, "SYN-STMT-A", "SYN-DSP-A1", "")
	fixture.seedDispute(t, catalogueTenant, "SYN-STMT-A", "SYN-DSP-A2", "")
	fixture.seedDispute(t, catalogueTenant, "SYN-STMT-B", "SYN-DSP-B1", "PENDING_REVIEW")
	fixture.seedDispute(t, catalogueTenant, "SYN-STMT-C", "SYN-DSP-C1", "ACCEPTED")
	fixture.seedDispute(t, catalogueTenant, "SYN-STMT-C", "SYN-DSP-C2", "REJECTED")
	fixture.seedDispute(t, catalogueTenant, "SYN-STMT-D", "SYN-DSP-D1", "PARTIALLY_ACCEPTED")
	fixture.seedDispute(t, catalogueTenant, "SYN-STMT-D", "SYN-DSP-D2", "")
	fixture.seedDispute(t, catalogueTenant, "SYN-STMT-E", "SYN-DSP-E1", "")
	fixture.seedStatement(t, "tenant-b", "SYN-STMT-A", false)
	fixture.seedDispute(t, "tenant-b", "SYN-STMT-A", "SYN-DSP-X", "")

	disputedPage := firstPage(t, ports.DisputedCustomerStatementPages)
	listed, err := fixture.statement.ListDisputedCustomerStatements(t.Context(), catalogueTenantID(t, catalogueTenant), 10, disputedPage)
	if err != nil {
		t.Fatalf("上列挂着未了结争议的单：%v", err)
	}
	if listed.Next != "" || listed.Total != 4 {
		t.Fatalf("一页装得下时没有下一页、总数是全部：next=%q total=%d", listed.Next, listed.Total)
	}
	var numbers []string
	for _, row := range listed.Statements {
		numbers = append(numbers, row.StatementNumber)
	}
	want := []string{"SYN-STMT-A", "SYN-STMT-B", "SYN-STMT-D", "SYN-STMT-E"}
	if listed.Count != len(want) || len(numbers) != len(want) {
		t.Fatalf("张数或清单走样：count=%d rows=%v，要 %v", listed.Count, numbers, want)
	}
	for index := range want {
		if numbers[index] != want[index] {
			t.Fatalf("清单走样：%v，要 %v", numbers, want)
		}
	}
	if len(listed.Statements[0].Disputes) != 2 || len(listed.Statements[2].Disputes) != 2 {
		t.Fatalf("单上的争议要整份带出（已终局的也在），与对账单册同形：%+v", listed.Statements)
	}

	page, err := fixture.statement.ListDisputedCustomerStatements(t.Context(), catalogueTenantID(t, catalogueTenant), 1, disputedPage)
	if err != nil || page.Count != len(want) || len(page.Statements) != 1 || page.Statements[0].StatementNumber != "SYN-STMT-A" {
		t.Fatalf("张数必须是全部而不是这一页：err=%v %+v", err, page)
	}
	if page.Next == "" || page.Total != int64(len(want)) {
		t.Fatalf("截了页要给下一页的游标、总数仍是全部：next=%q total=%d", page.Next, page.Total)
	}

	none, err := fixture.statement.ListDisputedCustomerStatements(t.Context(), catalogueTenantID(t, "tenant-c"), 10, disputedPage)
	if err != nil || none.Count != 0 || len(none.Statements) != 0 {
		t.Fatalf("没有单的租户答 0 张：err=%v %+v", err, none)
	}
	if _, err := fixture.statement.ListDisputedCustomerStatements(t.Context(), catalogueTenantID(t, catalogueTenant), 0, disputedPage); err == nil {
		t.Fatal("limit 为 0 必须拒")
	}
}

// Covers: AT-SA-228（读口半边）——一张单自己的费用行照单上快照读出，带币种、发布与作废时刻；作废单的行照样读得出（作废前提出的
// 争议还要从它上面选费用）；他租户与不在册的单答没有。
func TestStatementLinesAreReadFromTheStatementItself(t *testing.T) {
	fixture := newCatalogueFixture(t)
	fixture.seedStatement(t, catalogueTenant, "SYN-STMT-L", true)
	found, has, err := fixture.statement.FindStatementLines(t.Context(), catalogueTenantID(t, catalogueTenant), "SYN-STMT-L")
	if err != nil || !has {
		t.Fatalf("读费用行：%v %v", err, has)
	}
	if found.StatementNumber != "SYN-STMT-L" || found.Currency != "SYN-CUR-01" || found.VoidedAt == nil ||
		len(found.Lines) != 1 || found.Lines[0].Charge != "SYN-CHG-01" || found.Lines[0].AmountMinor != 1000 {
		t.Fatalf("费用行走样：%+v", found)
	}
	if _, has, err := fixture.statement.FindStatementLines(t.Context(), catalogueTenantID(t, "tenant-b"), "SYN-STMT-L"); err != nil || has {
		t.Fatalf("他租户不得读到：%v %v", err, has)
	}
	if _, has, err := fixture.statement.FindStatementLines(t.Context(), catalogueTenantID(t, catalogueTenant), "SYN-STMT-NONE"); err != nil || has {
		t.Fatalf("不在册答没有：%v %v", err, has)
	}
}
