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

// seedAuditedPayable 播一笔应付。number 与 absence 照库上两列能有的形状给：有号、时区未配置、两列都空（上线前开立）。
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

// seedAuditDecision 播一份审核决定；basis 与 resolves 为 nil 即不带。
func seedAuditDecision(t *testing.T, fixture *catalogueFixture, tenant, decision, claim, line, verdict string, from, to int64, basis, resolves any, auditedAt time.Time) {
	t.Helper()
	fixture.seed(t,
		`INSERT INTO settlement_accounting.supplier_bill_audit_decision
			(tenant_id, decision_id, claim_id, claim_version, line_ref, verdict, range_from_minor, range_to_minor,
			 basis_ref, resolves_decision, auditor_ref, operation_evidence, audited_at, content_digest, recorded_at)
		 VALUES ($1, $2, $3, '1', $4, $5, $6, $7, $8, $9, 'SYN-AUDITOR-ROLE', 'OPERATOR/syn#auditor', $10, 'SYN-DIGEST-D', $10)`,
		tenant, decision, claim, line, verdict, from, to, basis, resolves, auditedAt)
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

	rows, err := rowsOf(fixture.statement.ListSupplierBillReceptions(t.Context(), catalogueTenantID(t, catalogueTenant), ports.SupplierBillReceptionFilter{}, 10,
		firstPage(t, ports.SupplierBillReceptionPages)))
	if err != nil || len(rows) != 2 {
		t.Fatalf("上列账单走样：err=%v rows=%+v", err, rows)
	}
	// 同一收到时刻按主张身份排，与收到时刻同向倒序（票 catalogue-read-pagination-rollout spec「行标识取登记表主键去掉租户那几段」）。
	pending, audited := rows[0], rows[1]
	if pending.Claim != "SYN-SUP-01/INV-2" || audited.Claim != "SYN-SUP-01/INV-1" {
		t.Fatalf("同一收到时刻要按主张身份倒序：%+v", rows)
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
	seedAuditDecision(t, fixture, catalogueTenant, "SYN-PAYABLE-1", "SYN-SUP-01/INV-1", "L1", "PASSED", 0, 1000, nil, nil, catalogueBaseAt.Add(time.Hour))

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
	if len(matched.Decisions) != 1 || matched.Decisions[0].Payable == nil {
		t.Fatalf("已匹配那一行的决定走样：%+v", matched.Decisions)
	}
	if decision, payable := matched.Decisions[0], matched.Decisions[0].Payable; decision.Verdict != "PASSED" ||
		decision.AuditorRef != "SYN-AUDITOR-ROLE" || decision.OperationEvidence != "OPERATOR/syn#auditor" ||
		payable.Payable != "SYN-PAYABLE-1" || payable.DocumentNumber != "" ||
		payable.DocumentNumberAbsence != "NUMBERING_TIME_ZONE_NOT_CONFIGURED" || payable.AmountMinor != 1000 {
		t.Fatalf("挂在已匹配行上的应付走样：%+v %+v", decision, payable)
	}
	if variance.Classification != "PRICE_VARIANCE" || variance.ExpectedMinor != 1800 || variance.Basis != "SYN-BASIS-PRICE" || len(variance.Decisions) != 0 {
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

// Covers: AT-SA-232、AT-SA-231 — 审核应付册新形成的在前，带应付单号或无号原因（有号、时区未配置、上线前开立各自分开）、
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
	rows, err := rowsOf(fixture.statement.ListAuditedPayables(t.Context(), tenant, ports.AuditedPayableFilter{}, 10, firstPage(t, ports.AuditedPayablePages)))
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
			got, err := rowsOf(fixture.statement.ListAuditedPayables(t.Context(), tenant, probe.filter, 10, firstPage(t, ports.AuditedPayablePages)))
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

	limited, err := rowsOf(fixture.statement.ListAuditedPayables(t.Context(), tenant, ports.AuditedPayableFilter{}, 1, firstPage(t, ports.AuditedPayablePages)))
	if err != nil || len(limited) != 1 || limited[0].Payable != "SYN-PAYABLE-NEW" {
		t.Fatalf("页大小要截在最新那一笔：err=%v rows=%+v", err, limited)
	}
	if _, err := fixture.statement.ListAuditedPayables(t.Context(), tenant, ports.AuditedPayableFilter{}, 0, firstPage(t, ports.AuditedPayablePages)); err == nil {
		t.Fatal("页大小为 0 要拒")
	}
}

// Covers: AT-SA-232 — 一笔应付找不到它所属主张的接收记录是册上矛盾，答错误（端点答 NO_ANSWER_FORMED），不把它当成没有供应商
// 上列，也不从列表里悄悄漏掉；按供应商检索时也答错误，不因为对不上供应商就把它滤掉、只交回其余几笔。
func TestAnAuditedPayableWithoutItsReceptionIsAnError(t *testing.T) {
	fixture := newCatalogueFixture(t)
	seedSupplierBill(t, fixture, catalogueTenant, "SYN-SUP-01/INV-1", "1", "SYN-SUP-01", "", catalogueBaseAt)
	seedAuditedPayable(t, fixture, catalogueTenant, "SYN-PAYABLE-1", "SYN-SUP-01/INV-1", "L1", 1000, "AP-202608-00001", nil, catalogueBaseAt)
	seedAuditedPayable(t, fixture, catalogueTenant, "SYN-PAYABLE-ORPHAN", "SYN-SUP-01/INV-404", "L1", 1000, "AP-202608-00009", nil, catalogueBaseAt)
	tenant := catalogueTenantID(t, catalogueTenant)
	for name, filter := range map[string]ports.AuditedPayableFilter{
		"不检索":       {},
		"按那家供应商检索":  {Supplier: "SYN-SUP-01"},
		"按另一家供应商检索": {Supplier: "SYN-SUP-02"},
	} {
		t.Run(name, func(t *testing.T) {
			if rows, err := rowsOf(fixture.statement.ListAuditedPayables(t.Context(), tenant, filter, 10, firstPage(t, ports.AuditedPayablePages))); err == nil {
				t.Fatalf("没有接收记录的应付要答错误，却交回了 %+v", rows)
			}
		})
	}
	// 按应付单号或主张检索点名的是有接收记录的那一笔，孤儿不在检索范围里，照常答。
	rows, err := rowsOf(fixture.statement.ListAuditedPayables(t.Context(), tenant, ports.AuditedPayableFilter{DocumentNumber: "AP-202608-00001"}, 10,
		firstPage(t, ports.AuditedPayablePages)))
	if err != nil || len(rows) != 1 || rows[0].Payable != "SYN-PAYABLE-1" {
		t.Fatalf("按应付单号检索有接收记录的那一笔要照常答：err=%v rows=%+v", err, rows)
	}
}

// seedReceptionReceivedAt 播一份一行的账单，到达时刻与接收时的审核授权快照由调用方给。
func seedReceptionReceivedAt(t *testing.T, fixture *catalogueFixture, claim, supplier string, receivedAt time.Time, authorityAtReception bool) {
	t.Helper()
	fixture.seed(t,
		`INSERT INTO settlement_accounting.supplier_bill_reception
			(tenant_id, claim_id, claim_version, supplier_ref, legal_entity, period_ref,
			 currency, content_digest, claim, matches, audit_authority_configured, recorded_at)
		 VALUES ($1, $2, '1', $3, 'SYN-LE-01', 'SYN-PER-2608', 'CNY', 'SYN-DIGEST',
		         jsonb_build_object('lines', '[{"line":"L1","fee_item":"SYN-FEE-01","claimed_minor":1000}]'::jsonb,
		                            'received_at', $4::timestamptz),
		         '[{"line":"L1","classification":"MATCHED","expected":"SYN-EXP-1","claimed_minor":1000,
		            "expected_minor":1000,"matched_at":"2026-08-30T09:00:00Z"}]'::jsonb,
		         $5, $6)`,
		catalogueTenant, claim, supplier, receivedAt, authorityAtReception, catalogueBaseAt)
}

// Covers: AT-SA-232 — 账单册按到达的业务时刻倒序上列、按页大小截：截掉的是最早收到的，最新收到的那一份总在页上，与主张身份
// 怎么排无关。
func TestSupplierBillReceptionCatalogueKeepsTheLatestReceivedOnThePage(t *testing.T) {
	fixture := newCatalogueFixture(t)
	// 主张身份排在最前的那一份收到得最早：按主张身份截页，它会挤掉最新收到的那一份。
	seedReceptionReceivedAt(t, fixture, "SYN-SUP-01/A-OLDEST", "SYN-SUP-01", catalogueBaseAt.Add(-48*time.Hour), true)
	seedReceptionReceivedAt(t, fixture, "SYN-SUP-01/Z-LATEST", "SYN-SUP-01", catalogueBaseAt, true)
	seedReceptionReceivedAt(t, fixture, "SYN-SUP-01/M-MIDDLE", "SYN-SUP-01", catalogueBaseAt.Add(-24*time.Hour), true)

	rows, err := rowsOf(fixture.statement.ListSupplierBillReceptions(t.Context(), catalogueTenantID(t, catalogueTenant), ports.SupplierBillReceptionFilter{}, 2,
		firstPage(t, ports.SupplierBillReceptionPages)))
	if err != nil || len(rows) != 2 {
		t.Fatalf("上列走样：err=%v rows=%+v", err, rows)
	}
	if rows[0].Claim != "SYN-SUP-01/Z-LATEST" || rows[1].Claim != "SYN-SUP-01/M-MIDDLE" {
		t.Fatalf("要按收到时间倒序截页、最新收到的在前：%s、%s", rows[0].Claim, rows[1].Claim)
	}
}

// Covers: AT-SA-232 — 审核授权分两格：接收时的快照照实转写，另一格是读这一刻审核授权册上有没有这家供应商与这个责任法人那一组。
// 接收时没登、后来补登了，现状说已配置、快照仍说没配；接收时登了的，快照不被现状改写。
func TestSupplierBillReceptionCatalogueReadsTheAuditAuthorityAsItIsNow(t *testing.T) {
	fixture := newCatalogueFixture(t)
	seedReceptionReceivedAt(t, fixture, "SYN-SUP-01/LATER-REGISTERED", "SYN-SUP-01", catalogueBaseAt, false)
	seedReceptionReceivedAt(t, fixture, "SYN-SUP-02/STILL-MISSING", "SYN-SUP-02", catalogueBaseAt.Add(-time.Hour), false)
	fixture.seed(t,
		`INSERT INTO settlement_accounting.supplier_audit_authority (tenant_id, supplier_id, legal_entity_id, auditor_id, registered_at)
		 VALUES ($1, 'SYN-SUP-01', 'SYN-LE-01', 'SYN-AUDITOR-ROLE', $2),
		        ('tenant-b', 'SYN-SUP-02', 'SYN-LE-01', 'SYN-AUDITOR-ROLE', $2),
		        ($1, 'SYN-SUP-02', 'SYN-LE-OTHER', 'SYN-AUDITOR-ROLE', $2)`,
		catalogueTenant, catalogueBaseAt.Add(time.Hour))

	rows, err := rowsOf(fixture.statement.ListSupplierBillReceptions(t.Context(), catalogueTenantID(t, catalogueTenant), ports.SupplierBillReceptionFilter{}, 10,
		firstPage(t, ports.SupplierBillReceptionPages)))
	if err != nil || len(rows) != 2 {
		t.Fatalf("上列走样：err=%v rows=%+v", err, rows)
	}
	registered, missing := rows[0], rows[1]
	if registered.AuditAuthorityConfigured || !registered.AuditAuthorityConfiguredNow {
		t.Fatalf("后来补登的：快照要仍是 false、现状要是 true：%+v", registered)
	}
	// 别的租户登的、本租户别的责任法人登的，都不是这一组。
	if missing.AuditAuthorityConfigured || missing.AuditAuthorityConfiguredNow {
		t.Fatalf("这一组一直没登，两格都要是 false：%+v", missing)
	}
}

// Covers: AT-SA-233、AT-SA-234、AT-SA-235（读面半边）——账单册、账单各行与应付册带出供应商账单号（取账单号册，不从主张标识拆）与
// 各段审核结果：各行的已匹配金额（价差行到预期成本）、各份决定（结果、范围、依据、所解争议、通过的带应付）与四个和；目录行的
// 已拒绝、仍在争议中、还没有结果三个和与按各行决定求的一致；同一行两份应付只算一行；应付册带范围与所解争议；账单册按主张、
// 供应商与账单号在全册里精确检索，页外的原账单也找得到，账单号逐字比。
func TestSupplierBillBooksCarryRangedDecisionsAndTheBillNumber(t *testing.T) {
	fixture := newCatalogueFixture(t)
	const claim = "SYN-BILL-RANDOM-1"
	seedSupplierBill(t, fixture, catalogueTenant, claim, "1", "SYN-SUP-01", "", catalogueBaseAt)
	fixture.seed(t, `INSERT INTO settlement_accounting.supplier_bill_number (tenant_id, supplier_ref, bill_number, claim_id, claim_version, recorded_at)
		VALUES ($1, 'SYN-SUP-01', 'INV-2608-007', $2, '1', $3)`, catalogueTenant, claim, catalogueBaseAt)
	at := catalogueBaseAt.Add(time.Hour)
	// L1（1000，已匹配）：通过 600、拒绝 400。L2（2000，价差到 1800）：通过 1500、争议 500，争议里再审通过 300、剩 200 仍在争议。
	// L3（300，没有对应发生项）：还没审。
	seedAuditedPayable(t, fixture, catalogueTenant, "D-L1-PASS", claim, "L1", 600, "AP-202608-00011", nil, at)
	seedAuditDecision(t, fixture, catalogueTenant, "D-L1-PASS", claim, "L1", "PASSED", 0, 600, nil, nil, at)
	seedAuditDecision(t, fixture, catalogueTenant, "D-L1-REJECT", claim, "L1", "REJECTED", 600, 1000, "SYN-NOT-ORDERED", nil, at)
	seedAuditedPayable(t, fixture, catalogueTenant, "D-L2-PASS", claim, "L2", 1500, "AP-202608-00012", nil, at)
	seedAuditDecision(t, fixture, catalogueTenant, "D-L2-PASS", claim, "L2", "PASSED", 0, 1500, nil, nil, at)
	seedAuditDecision(t, fixture, catalogueTenant, "D-L2-DISPUTE", claim, "L2", "DISPUTED", 1500, 2000, "SYN-PRICE-PROOF", nil, at)
	seedAuditedPayable(t, fixture, catalogueTenant, "D-L2-REPASS", claim, "L2", 300, "AP-202608-00013", nil, at.Add(time.Hour))
	seedAuditDecision(t, fixture, catalogueTenant, "D-L2-REPASS", claim, "L2", "PASSED", 1500, 1800, "SYN-PRICE-CONFIRMED", "D-L2-DISPUTE", at.Add(time.Hour))

	tenant := catalogueTenantID(t, catalogueTenant)
	found, has, err := fixture.statement.FindSupplierBillLines(t.Context(), tenant, claim, "1")
	if err != nil || !has || found.SupplierBillNumber != "INV-2608-007" || len(found.Lines) != 3 {
		t.Fatalf("账单面走样：has=%v err=%v %+v", has, err, found)
	}
	l1, l2, l3 := found.Lines[0], found.Lines[1], found.Lines[2]
	if l1.MatchedMinor != 1000 || l1.PassedMinor != 600 || l1.RejectedMinor != 400 || l1.UndecidedMinor != 0 || len(l1.Decisions) != 2 {
		t.Fatalf("L1 = %+v", l1)
	}
	if l2.MatchedMinor != 1800 || l2.PassedMinor != 1800 || l2.OpenDisputeMinor != 200 || l2.UndecidedMinor != 0 || len(l2.Decisions) != 3 {
		t.Fatalf("L2 = %+v", l2)
	}
	byID := map[string]ports.SupplierBillLineDecision{}
	for _, decision := range l2.Decisions {
		byID[decision.Decision] = decision
	}
	if repass := byID["D-L2-REPASS"]; repass.ResolvesDispute != "D-L2-DISPUTE" || repass.Basis != "SYN-PRICE-CONFIRMED" ||
		repass.RangeFromMinor != 1500 || repass.RangeToMinor != 1800 || repass.Payable == nil || repass.Payable.DocumentNumber != "AP-202608-00013" {
		t.Fatalf("L2 的再审 = %+v", repass)
	}
	if dispute := byID["D-L2-DISPUTE"]; dispute.Payable != nil || dispute.Verdict != "DISPUTED" || dispute.Basis != "SYN-PRICE-PROOF" {
		t.Fatalf("争议不该挂应付：%+v", dispute)
	}
	if l3.MatchedMinor != 0 || l3.UndecidedMinor != 300 || len(l3.Decisions) != 0 {
		t.Fatalf("L3 = %+v", l3)
	}

	rows, err := rowsOf(fixture.statement.ListSupplierBillReceptions(t.Context(), tenant, ports.SupplierBillReceptionFilter{}, 10,
		firstPage(t, ports.SupplierBillReceptionPages)))
	if err != nil || len(rows) != 1 {
		t.Fatalf("账单册：err=%v rows=%+v", err, rows)
	}
	row := rows[0]
	sum := func(pick func(ports.SupplierBillLineEntry) int64) int64 {
		return pick(l1) + pick(l2) + pick(l3)
	}
	if row.SupplierBillNumber != "INV-2608-007" || row.PayableLineCount != 2 || row.PayableMinor != 2400 ||
		row.RejectedMinor != sum(func(line ports.SupplierBillLineEntry) int64 { return line.RejectedMinor }) ||
		row.OpenDisputeMinor != sum(func(line ports.SupplierBillLineEntry) int64 { return line.OpenDisputeMinor }) ||
		row.UndecidedMinor != sum(func(line ports.SupplierBillLineEntry) int64 { return line.UndecidedMinor }) ||
		row.RejectedMinor != 400 || row.OpenDisputeMinor != 200 || row.UndecidedMinor != 300 {
		t.Fatalf("目录行 = %+v", row)
	}

	// 精确检索在全册里找：另登一份更晚收到的账单、页大小 1，原账单在页外，按账单号（供应商加号）与按主张都找得到；号逐字比。
	seedSupplierBill(t, fixture, catalogueTenant, "SYN-BILL-RANDOM-2", "1", "SYN-SUP-01", "", catalogueBaseAt.Add(time.Hour))
	fixture.seed(t, `UPDATE settlement_accounting.supplier_bill_reception
		SET claim = jsonb_set(claim, '{received_at}', '"2026-08-31T00:00:00Z"') WHERE claim_id = 'SYN-BILL-RANDOM-2'`)
	receptions := func(filter ports.SupplierBillReceptionFilter, limit int) ([]ports.SupplierBillReceptionCatalogueRow, error) {
		return rowsOf(fixture.statement.ListSupplierBillReceptions(t.Context(), tenant, filter, limit, firstPage(t, ports.SupplierBillReceptionPages)))
	}
	if page, err := receptions(ports.SupplierBillReceptionFilter{}, 1); err != nil ||
		len(page) != 1 || page[0].Claim != "SYN-BILL-RANDOM-2" {
		t.Fatalf("页大小 1 只该列最晚收到的那份：err=%v %+v", err, page)
	}
	for name, filter := range map[string]ports.SupplierBillReceptionFilter{
		"by number":   {Supplier: "SYN-SUP-01", SupplierBillNumber: "INV-2608-007"},
		"by claim":    {Claim: claim},
		"number only": {SupplierBillNumber: "INV-2608-007"},
	} {
		found, err := receptions(filter, 1)
		if err != nil || len(found) != 1 || found[0].Claim != claim {
			t.Fatalf("%s：页外的原账单要找得到：err=%v %+v", name, err, found)
		}
	}
	for name, filter := range map[string]ports.SupplierBillReceptionFilter{
		"other case":     {SupplierBillNumber: "inv-2608-007"},
		"other supplier": {Supplier: "SYN-SUP-02", SupplierBillNumber: "INV-2608-007"},
	} {
		if found, err := receptions(filter, 10); err != nil || len(found) != 0 {
			t.Fatalf("%s 不该找到：err=%v %+v", name, err, found)
		}
	}

	// 存量里各版本供应商不一的主张（编排今天会挡，这里直写一版另一家供应商的）：按（另一家，号）检索不命中——供应商比的是号册上的
	// 分配方；只按另一家供应商检索，照接收行的供应商找得到那一版。
	seedSupplierBill(t, fixture, catalogueTenant, claim, "2", "SYN-SUP-02", "", catalogueBaseAt)
	if found, err := receptions(ports.SupplierBillReceptionFilter{Supplier: "SYN-SUP-02", SupplierBillNumber: "INV-2608-007"}, 10); err != nil || len(found) != 0 {
		t.Fatalf("另一家供应商加这个号不该命中：err=%v %+v", err, found)
	}
	if found, err := receptions(ports.SupplierBillReceptionFilter{Supplier: "SYN-SUP-01", SupplierBillNumber: "INV-2608-007"}, 10); err != nil || len(found) != 2 {
		t.Fatalf("分配方加号要命中这份主张的两版：err=%v %+v", err, found)
	}
	if found, err := receptions(ports.SupplierBillReceptionFilter{Supplier: "SYN-SUP-02", Claim: claim}, 10); err != nil || len(found) != 1 || found[0].ClaimVersion != "2" {
		t.Fatalf("不给号时按接收行的供应商比：err=%v %+v", err, found)
	}

	payables, err := rowsOf(fixture.statement.ListAuditedPayables(t.Context(), tenant, ports.AuditedPayableFilter{Claim: claim}, 10, firstPage(t, ports.AuditedPayablePages)))
	if err != nil || len(payables) != 3 {
		t.Fatalf("应付册：err=%v rows=%+v", err, payables)
	}
	newest := payables[0]
	if newest.Payable != "D-L2-REPASS" || newest.RangeFromMinor == nil || *newest.RangeFromMinor != 1500 || *newest.RangeToMinor != 1800 ||
		newest.ResolvesDispute != "D-L2-DISPUTE" || newest.SupplierBillNumber != "INV-2608-007" {
		t.Fatalf("应付册最新那一笔 = %+v", newest)
	}
}
