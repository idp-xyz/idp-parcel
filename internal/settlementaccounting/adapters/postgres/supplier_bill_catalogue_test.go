package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 供应商账单与应付页两本册的真库用例（票 document-numbering/11）。播种照本包 catalogueFixture 的约定走显式 SQL，不借道写适配器。

// seedSupplierBill 播一份三行的账单：L1 已匹配、L2 价差、L3 没有对应发生项；received 是到达的业务时刻，evidence 为空即不记
// 提交方。
func seedSupplierBill(t *testing.T, fixture *catalogueFixture, tenant, claim, version, supplier, evidence string, recordedAt time.Time) {
	t.Helper()
	var operationEvidence any
	if evidence != "" {
		operationEvidence = evidence
	}
	fixture.seed(t,
		`INSERT INTO settlement_accounting.supplier_bill_reception
			(tenant_id, claim_id, claim_version, supplier_ref, legal_entity, period_ref,
			 currency, content_digest, claim, matches, audit_authority_configured, recorded_at, operation_evidence)
		 VALUES ($1, $2, $3, $4, 'SYN-LE-01', 'SYN-PER-2608', 'CNY', 'SYN-DIGEST',
		         '{"lines":[{"line":"L1","fee_item":"SYN-FEE-01","claimed_minor":1000},
		                    {"line":"L2","fee_item":"SYN-FEE-02","claimed_minor":2000},
		                    {"line":"L3","fee_item":"SYN-FEE-03","claimed_minor":300}],
		           "supplements":"SYN-CLM-ORIGIN",
		           "received_at":"2026-08-29T23:30:00Z"}'::jsonb,
		         '[{"line":"L1","classification":"MATCHED","expected":"SYN-EXP-1","claimed_minor":1000,
		            "expected_minor":1000,"matched_at":"2026-08-30T09:00:00Z"},
		           {"line":"L2","classification":"PRICE_VARIANCE","expected":"SYN-EXP-2","claimed_minor":2000,
		            "expected_minor":1800,"basis":"SYN-BASIS-PRICE","matched_at":"2026-08-30T09:00:00Z"},
		           {"line":"L3","classification":"NO_MATCHING_OCCURRENCE","claimed_minor":300,
		            "expected_minor":0,"basis":"SYN-BASIS-SEARCH","matched_at":"2026-08-30T09:00:00Z"}]'::jsonb,
		         true, $5, $6)`,
		tenant, claim, version, supplier, recordedAt, operationEvidence)
}

// seedAuditedPayable 播一笔应付。number 与 absence 照库上两列的三种形状给：有号、时区未配置、两列都空（上线前开立）。
func seedAuditedPayable(t *testing.T, fixture *catalogueFixture, tenant, payable, claim, line string, amount int64, number, absence any, auditedAt time.Time) {
	t.Helper()
	fixture.seed(t,
		`INSERT INTO settlement_accounting.audited_payable
			(tenant_id, payable_id, claim_id, line_ref, expected_version, legal_entity, account_id, currency,
			 amount_minor, auditor_ref, audited_at, content_digest, recorded_at, operation_evidence,
			 document_number, document_number_absence)
		 VALUES ($1, $2, $3, $4, 'SYN-EXP-1', 'SYN-LE-01', 'SYN-ACC-PAYABLE', 'CNY',
		         $5, 'SYN-AUDITOR-ROLE', $6, 'SYN-DIGEST-P', $6, 'OPERATOR/syn#auditor', $7, $8)`,
		tenant, payable, claim, line, amount, auditedAt, number, absence)
}

// Covers: AT-SA-232 — 账单册目录行的汇总只是求和与计数：主张金额之和、按逐行匹配分类原名计的行数、这一版各行上已形成的应付
// 行数与金额、已匹配而还没有应付的行数；到达时刻、被追加的原主张与接收操作者照实转写，不记提交方的读作空。
func TestSupplierBillReceptionCatalogueSummarisesLinesAndPayables(t *testing.T) {
	fixture := newCatalogueFixture(t)
	seedSupplierBill(t, fixture, catalogueTenant, "SYN-SUP-01/INV-1", "1", "SYN-SUP-01", "OPERATOR/syn#clerk", catalogueBaseAt)
	seedSupplierBill(t, fixture, catalogueTenant, "SYN-SUP-01/INV-2", "1", "SYN-SUP-01", "", catalogueBaseAt)
	seedAuditedPayable(t, fixture, catalogueTenant, "SYN-PAYABLE-1", "SYN-SUP-01/INV-1", "L1", 1000, "AP-202608-00001", nil, catalogueBaseAt.Add(time.Hour))
	// 另一个租户同一主张同一行的应付不算进本租户这一份。
	seedAuditedPayable(t, fixture, "tenant-b", "SYN-PAYABLE-1", "SYN-SUP-01/INV-2", "L1", 1000, "AP-202608-00001", nil, catalogueBaseAt)

	rows, err := fixture.statement.ListSupplierBillReceptions(t.Context(), catalogueTenantID(t, catalogueTenant), 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("上列账单走样：err=%v rows=%+v", err, rows)
	}
	audited, pending := rows[0], rows[1]
	if audited.Claim != "SYN-SUP-01/INV-1" || pending.Claim != "SYN-SUP-01/INV-2" {
		t.Fatalf("按主张身份排序走样：%+v", rows)
	}
	if audited.TotalClaimedMinor != 3300 || audited.LineCount != 3 || audited.MatchCount != 3 {
		t.Fatalf("主张金额之和或行数走样（要 3300、3、3）：%+v", audited)
	}
	if len(audited.ClassificationCounts) != 3 || audited.ClassificationCounts["MATCHED"] != 1 ||
		audited.ClassificationCounts["PRICE_VARIANCE"] != 1 || audited.ClassificationCounts["NO_MATCHING_OCCURRENCE"] != 1 {
		t.Fatalf("分类计数要按原名各计一行、不归并：%+v", audited.ClassificationCounts)
	}
	if audited.PayableLineCount != 1 || audited.PayableMinor != 1000 || audited.MatchedWithoutPayableCount != 0 {
		t.Fatalf("已审的那份应付计数走样：%+v", audited)
	}
	if pending.PayableLineCount != 0 || pending.PayableMinor != 0 || pending.MatchedWithoutPayableCount != 1 {
		t.Fatalf("别的租户的应付算进了本租户，或已匹配没应付的行没数到：%+v", pending)
	}
	if !audited.ReceivedAt.Equal(time.Date(2026, 8, 29, 23, 30, 0, 0, time.UTC)) || audited.Supplements != "SYN-CLM-ORIGIN" ||
		audited.OperationEvidence != "OPERATOR/syn#clerk" || pending.OperationEvidence != "" {
		t.Fatalf("到达时刻、被追加的主张或接收操作者转写走样：%+v / %+v", audited, pending)
	}
}

// Covers: AT-SA-232 — 一份账单的各行按主张原序交回，每行带它的匹配分类、所引预期成本、依据与挂在这一行上的审核应付；应付
// 单号与无号原因恰出其一；别的租户同一主张身份读不到；不在册答 false。
func TestSupplierBillLinesCarryMatchAndPayablePerLine(t *testing.T) {
	fixture := newCatalogueFixture(t)
	seedSupplierBill(t, fixture, catalogueTenant, "SYN-SUP-01/INV-1", "1", "SYN-SUP-01", "OPERATOR/syn#clerk", catalogueBaseAt)
	seedSupplierBill(t, fixture, "tenant-b", "SYN-SUP-01/INV-9", "1", "SYN-SUP-01", "", catalogueBaseAt)
	seedAuditedPayable(t, fixture, catalogueTenant, "SYN-PAYABLE-1", "SYN-SUP-01/INV-1", "L1", 1000, nil, "NUMBERING_TIME_ZONE_NOT_CONFIGURED", catalogueBaseAt.Add(time.Hour))

	tenant := catalogueTenantID(t, catalogueTenant)
	found, has, err := fixture.statement.FindSupplierBillLines(t.Context(), tenant, "SYN-SUP-01/INV-1", "1")
	if err != nil || !has {
		t.Fatalf("读不到账单各行：has=%v err=%v", has, err)
	}
	if found.Supplier != "SYN-SUP-01" || found.Currency != "CNY" || found.Supplements != "SYN-CLM-ORIGIN" ||
		!found.AuditAuthorityConfigured || found.OperationEvidence != "OPERATOR/syn#clerk" || len(found.Lines) != 3 {
		t.Fatalf("账单面走样：%+v", found)
	}
	matched, variance, missing := found.Lines[0], found.Lines[1], found.Lines[2]
	if matched.Line != "L1" || matched.Classification != "MATCHED" || matched.ExpectedVersion != "SYN-EXP-1" || matched.Basis != "" {
		t.Fatalf("已匹配那一行走样：%+v", matched)
	}
	if matched.Payable == nil || matched.Payable.Payable != "SYN-PAYABLE-1" || matched.Payable.DocumentNumber != "" ||
		matched.Payable.DocumentNumberAbsence != "NUMBERING_TIME_ZONE_NOT_CONFIGURED" || matched.Payable.AmountMinor != 1000 ||
		matched.Payable.AuditorRef != "SYN-AUDITOR-ROLE" || matched.Payable.OperationEvidence != "OPERATOR/syn#auditor" {
		t.Fatalf("挂在已匹配行上的应付走样：%+v", matched.Payable)
	}
	if variance.Classification != "PRICE_VARIANCE" || variance.ExpectedMinor != 1800 || variance.Basis != "SYN-BASIS-PRICE" || variance.Payable != nil {
		t.Fatalf("价差那一行走样：%+v", variance)
	}
	if missing.Classification != "NO_MATCHING_OCCURRENCE" || missing.ExpectedVersion != "" || missing.MatchedAt == nil {
		t.Fatalf("没有对应发生项那一行走样：%+v", missing)
	}
	if _, has, err := fixture.statement.FindSupplierBillLines(t.Context(), tenant, "SYN-SUP-01/INV-9", "1"); err != nil || has {
		t.Fatalf("别的租户的账单读到了本租户：has=%v err=%v", has, err)
	}
	if _, has, err := fixture.statement.FindSupplierBillLines(t.Context(), tenant, "SYN-SUP-01/INV-1", "2"); err != nil || has {
		t.Fatalf("不在册的版本要答 false：has=%v err=%v", has, err)
	}
}

// Covers: AT-SA-232、AT-SA-231 — 审核应付册新形成的在前，带应付单号或无号原因（有号、时区未配置、上线前开立三种分开）、
// 所属主张与行、供应商与账期（取接收记录）、审核授权方与点审核的人；按应付单号、主张、供应商精确检索，几维同给取交集；
// 只列本租户。
func TestAuditedPayableCatalogueListsNumbersAndFilters(t *testing.T) {
	fixture := newCatalogueFixture(t)
	seedSupplierBill(t, fixture, catalogueTenant, "SYN-SUP-01/INV-1", "1", "SYN-SUP-01", "", catalogueBaseAt)
	seedSupplierBill(t, fixture, catalogueTenant, "SYN-SUP-02/INV-1", "1", "SYN-SUP-02", "", catalogueBaseAt)
	seedSupplierBill(t, fixture, "tenant-b", "SYN-SUP-01/INV-1", "1", "SYN-SUP-01", "", catalogueBaseAt)
	seedAuditedPayable(t, fixture, catalogueTenant, "SYN-PAYABLE-OLD", "SYN-SUP-01/INV-1", "L1", 1000, nil, nil, catalogueBaseAt)
	seedAuditedPayable(t, fixture, catalogueTenant, "SYN-PAYABLE-NEW", "SYN-SUP-02/INV-1", "L1", 1000, "AP-202608-00002", nil, catalogueBaseAt.Add(2*time.Hour))
	seedAuditedPayable(t, fixture, catalogueTenant, "SYN-PAYABLE-ZONE", "SYN-SUP-01/INV-1", "L2", 2000, nil, "NUMBERING_TIME_ZONE_NOT_CONFIGURED", catalogueBaseAt.Add(time.Hour))
	seedAuditedPayable(t, fixture, "tenant-b", "SYN-PAYABLE-B", "SYN-SUP-01/INV-1", "L1", 1000, "AP-202608-00002", nil, catalogueBaseAt.Add(3*time.Hour))

	tenant := catalogueTenantID(t, catalogueTenant)
	rows, err := fixture.statement.ListAuditedPayables(t.Context(), tenant, ports.AuditedPayableFilter{}, 10)
	if err != nil || len(rows) != 3 {
		t.Fatalf("上列应付走样（只列本租户三笔）：err=%v rows=%+v", err, rows)
	}
	if rows[0].Payable != "SYN-PAYABLE-NEW" || rows[1].Payable != "SYN-PAYABLE-ZONE" || rows[2].Payable != "SYN-PAYABLE-OLD" {
		t.Fatalf("要新形成的在前：%+v", rows)
	}
	numbered, zone, before := rows[0], rows[1], rows[2]
	if numbered.DocumentNumber != "AP-202608-00002" || numbered.DocumentNumberAbsence != "" || numbered.Supplier != "SYN-SUP-02" ||
		numbered.Period != "SYN-PER-2608" || numbered.Claim != "SYN-SUP-02/INV-1" || numbered.Line != "L1" ||
		numbered.AmountMinor != 1000 || numbered.Account != "SYN-ACC-PAYABLE" || numbered.AuditorRef != "SYN-AUDITOR-ROLE" ||
		numbered.OperationEvidence != "OPERATOR/syn#auditor" {
		t.Fatalf("有号那一笔转写走样：%+v", numbered)
	}
	if zone.DocumentNumber != "" || zone.DocumentNumberAbsence != "NUMBERING_TIME_ZONE_NOT_CONFIGURED" {
		t.Fatalf("时区未配置那一笔要带原因：%+v", zone)
	}
	if before.DocumentNumber != "" || before.DocumentNumberAbsence != "ISSUED_BEFORE_NUMBERING" {
		t.Fatalf("两列都空要读作上线前开立：%+v", before)
	}

	for _, probe := range []struct {
		name   string
		filter ports.AuditedPayableFilter
		want   []string
	}{
		{"按应付单号", ports.AuditedPayableFilter{DocumentNumber: "AP-202608-00002"}, []string{"SYN-PAYABLE-NEW"}},
		{"按主张", ports.AuditedPayableFilter{Claim: "SYN-SUP-01/INV-1"}, []string{"SYN-PAYABLE-ZONE", "SYN-PAYABLE-OLD"}},
		{"按供应商", ports.AuditedPayableFilter{Supplier: "SYN-SUP-02"}, []string{"SYN-PAYABLE-NEW"}},
		{"几维同给取交集", ports.AuditedPayableFilter{Supplier: "SYN-SUP-01", DocumentNumber: "AP-202608-00002"}, nil},
	} {
		t.Run(probe.name, func(t *testing.T) {
			got, err := fixture.statement.ListAuditedPayables(t.Context(), tenant, probe.filter, 10)
			if err != nil || len(got) != len(probe.want) {
				t.Fatalf("err=%v got=%+v want=%v", err, got, probe.want)
			}
			for index, payable := range probe.want {
				if got[index].Payable != payable {
					t.Fatalf("got=%+v want=%v", got, probe.want)
				}
			}
		})
	}

	limited, err := fixture.statement.ListAuditedPayables(t.Context(), tenant, ports.AuditedPayableFilter{}, 1)
	if err != nil || len(limited) != 1 || limited[0].Payable != "SYN-PAYABLE-NEW" {
		t.Fatalf("页大小要截在最新那一笔：err=%v rows=%+v", err, limited)
	}
	if _, err := fixture.statement.ListAuditedPayables(t.Context(), tenant, ports.AuditedPayableFilter{}, 0); err == nil {
		t.Fatal("页大小为 0 要拒")
	}
}

// Covers: AT-SA-232 — 一笔应付找不到它所属主张的接收记录是册上矛盾，答错误（端点答 NO_ANSWER_FORMED），不把它当成没有供应商
// 上列，也不从列表里悄悄漏掉。
func TestAnAuditedPayableWithoutItsReceptionIsAnError(t *testing.T) {
	fixture := newCatalogueFixture(t)
	seedAuditedPayable(t, fixture, catalogueTenant, "SYN-PAYABLE-ORPHAN", "SYN-SUP-01/INV-404", "L1", 1000, "AP-202608-00009", nil, catalogueBaseAt)
	if _, err := fixture.statement.ListAuditedPayables(t.Context(), catalogueTenantID(t, catalogueTenant), ports.AuditedPayableFilter{}, 10); err == nil {
		t.Fatal("没有接收记录的应付要答错误")
	}
}
