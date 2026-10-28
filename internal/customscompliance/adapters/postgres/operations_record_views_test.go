package postgres_test

import (
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

func newOperationsRecordViews(t *testing.T) (*adapter.OperationsRecordViews, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	views, err := adapter.NewOperationsRecordViews(fixture.db)
	if err != nil {
		t.Fatalf("构造关务记录读面：%v", err)
	}
	return views, fixture
}

func recordTenant(t *testing.T, raw string) domain.TenantID {
	return viewValue(t, domain.NewTenantID, raw)
}

// seedDemoShapedChain 播一条演示租户形状的链：案件 → 申报单元 → 提交版本 → 发送事实与技术回执 → 外部结果
// （业务受理、放行）→ 关案记录 → 动作准入判断；另一租户播同形状的案件，验证租户隔离。
func seedDemoShapedChain(t *testing.T, fixture *viewFixture) {
	t.Helper()
	fixture.seed(t,
		`INSERT INTO customs_compliance.customs_case
			(tenant_id, jurisdiction_ref, direction, procedure_ref, obligation_ref, case_id, parcels, roles, established_at)
		 VALUES
			('SYN-TENANT-01', 'CN', 'EXPORT', 'proc-1', 'oblig-1', 'CASE-1',
			 '[{"parcel":"PCL-1","customer":"CUST-1","sourceRef":"src-1"}]',
			 '[{"role":"DECLARANT","party":"PARTY-1","authority":"auth-1"}]', $1),
			('SYN-TENANT-02', 'CN', 'EXPORT', 'proc-1', 'oblig-1', 'CASE-OTHER',
			 '[{"parcel":"PCL-9","customer":"CUST-9","sourceRef":"src-9"}]', '[]', $1)`, viewBaseAt)
	fixture.seed(t,
		`INSERT INTO customs_compliance.declaration_unit
			(tenant_id, unit_id, case_id, procedure_ref, members, formed_at)
		 VALUES ('SYN-TENANT-01', 'UNIT-1', 'CASE-1', 'proc-1', '["PCL-1"]', $1)`, viewBaseAt)
	fixture.seed(t,
		`INSERT INTO customs_compliance.case_closure
			(tenant_id, case_ref, cutoff_at, verified_at, decided_by, closed_at, items)
		 VALUES ('SYN-TENANT-01', 'CASE-1', $1, $1, 'ROLE-CLOSER', $1, '[{"obligation":"o"}]')`, viewBaseAt)
	fixture.seed(t,
		`INSERT INTO customs_compliance.declaration_submission
			(tenant_id, unit_id, procedure_ref, version_id, content_digest, members, dossier_ref, roles_ref,
			 readiness_basis, authority_ref, fixed_at, recorded_at, is_current)
		 VALUES ('SYN-TENANT-01', 'UNIT-1', 'proc-1', 'SUBV-1', 'dg', '["PCL-1"]', 'dossier-1', 'roles-1',
		         'basis-1', 'auth-1', $1, $1, true)`, viewBaseAt)
	fixture.seed(t,
		`INSERT INTO customs_compliance.submission_attempt
			(tenant_id, version_id, sequence, target, result, sent_at)
		 VALUES ('SYN-TENANT-01', 'SUBV-1', 1, 'SYN-BROKER', 'ACKNOWLEDGED', $1)`, viewBaseAt)
	fixture.seed(t,
		`INSERT INTO customs_compliance.submission_send_fact
			(tenant_id, version_id, sequence, sent_at, outcome, channel_message, recorded_at)
		 VALUES ('SYN-TENANT-01', 'SUBV-1', 1, $1, 'ACKNOWLEDGED', 'msg-1', $1)`, viewBaseAt)
	fixture.seed(t,
		`INSERT INTO customs_compliance.technical_receipt
			(tenant_id, source_id, version_id, sequence, outcome, occurred_at, received_at, content_digest, recorded_at)
		 VALUES ('SYN-TENANT-01', 'rcpt-1', 'SUBV-1', 1, 'ACKNOWLEDGED', $1, $1, 'dg', $1)`, viewBaseAt)
	fixture.seed(t,
		`INSERT INTO customs_compliance.external_result
			(tenant_id, source_id, content_digest, unattributable, layer_conflict, result, submission_version, recorded_at)
		 VALUES
			('SYN-TENANT-01', 'RES-REL', 'dg', false, false,
			 '{"layer":"RELEASE_RESULT","sourceId":"RES-REL","role":"CUSTOMS","rawSemantics":"放行","rule":"r/v1","version":"SUBV-1","attempt":1,"scope":"declaration/full","occurredAt":"2026-08-14T09:30:00Z","receivedAt":"2026-08-14T09:31:00Z"}',
			 'SUBV-1', $1),
			('SYN-TENANT-01', 'RES-LOST', 'dg2', true, false, NULL, NULL, $1)`, viewBaseAt)
	fixture.seed(t,
		`INSERT INTO customs_compliance.release_outcome
			(tenant_id, source_id, kind, authority_ref, scope_ref, received_at)
		 VALUES ('SYN-TENANT-01', 'RES-REL', 'FULL', 'CUSTOMS', 'declaration/full', $1)`, viewBaseAt)
	fixture.seed(t,
		`INSERT INTO customs_compliance.action_admission_judgement
			(tenant_id, judgement_id, object_ref, intent_kind, node_ref, onward_node_ref, action_at,
			 cross_zone, answer, zones, findings, judged_at)
		 VALUES ('SYN-TENANT-01', 'J-1', 'PCL-1', 'HANDOVER', 'NODE-A', 'NODE-B', $1, 'CROSSING', 'ADMITTED',
		         '[{"node":"NODE-A","zone":"Z1","registered":true}]',
		         '[{"action":"CROSS_CUSTOMS_MOVEMENT","answer":"ADMITTED","bases":[{"cause":"RELEASE_OUTCOME","scope":"declaration/full","sources":["RES-REL"]}]}]', $1)`,
		viewBaseAt)
}

// Covers: demo-2026-10-30/01 完成判据 — 演示租户形状数据上案件读口答出案件、单元与关案记录，他租户不可见。
func TestOperationsViewsListCasesWithUnitsAndClosureIsolatedByTenant(t *testing.T) {
	views, fixture := newOperationsRecordViews(t)
	seedDemoShapedChain(t, fixture)

	cases, err := views.ListCustomsCases(t.Context(), recordTenant(t, "SYN-TENANT-01"), "", 10)
	if err != nil {
		t.Fatalf("上列案件：%v", err)
	}
	if len(cases) != 1 || cases[0].Case != "CASE-1" {
		t.Fatalf("案件上列走样（他租户案件不得出现）：%+v", cases)
	}
	got := cases[0]
	if len(got.Parcels) != 1 || got.Parcels[0].Parcel != "PCL-1" || len(got.Roles) != 1 ||
		len(got.Units) != 1 || got.Units[0].Unit != "UNIT-1" || got.Units[0].Members[0] != "PCL-1" {
		t.Fatalf("包裹、角色或单元走样：%+v", got)
	}
	if got.Closure == nil || got.Closure.DecidedBy != "ROLE-CLOSER" || got.Closure.ItemCount != 1 {
		t.Fatalf("关案记录走样：%+v", got.Closure)
	}

	one, err := views.ListCustomsCases(t.Context(), recordTenant(t, "SYN-TENANT-01"), "CASE-OTHER", 10)
	if err != nil || len(one) != 0 {
		t.Fatalf("按他租户案件号单份读回必须答空：%v %+v", err, one)
	}
}

// Covers: demo-2026-10-30/01 — 提交版本连同尝试、发送事实、技术回执逐层上列，不折叠。
func TestOperationsViewsListSubmissionVersionsWithSendFactAndReceipts(t *testing.T) {
	views, fixture := newOperationsRecordViews(t)
	seedDemoShapedChain(t, fixture)

	versions, err := views.ListSubmissionVersions(t.Context(), recordTenant(t, "SYN-TENANT-01"), "", 10)
	if err != nil {
		t.Fatalf("上列提交版本：%v", err)
	}
	if len(versions) != 1 || versions[0].Version != "SUBV-1" || !versions[0].IsCurrent || len(versions[0].Attempts) != 1 {
		t.Fatalf("提交版本走样：%+v", versions)
	}
	attempt := versions[0].Attempts[0]
	if attempt.Result != "ACKNOWLEDGED" || attempt.SendFact == nil || attempt.SendFact.ChannelMessage != "msg-1" ||
		len(attempt.Receipts) != 1 || attempt.Receipts[0].Source != "rcpt-1" {
		t.Fatalf("尝试、发送事实或回执走样：%+v", attempt)
	}
	if other, err := views.ListSubmissionVersions(t.Context(), recordTenant(t, "SYN-TENANT-02"), "", 10); err != nil || len(other) != 0 {
		t.Fatalf("他租户不得看见提交版本：%v %+v", err, other)
	}
}

// Covers: UC-CC-006 — 放行层结果带出放行事实；归属不上的响应照样上列且不带版本（留存不猜）。
func TestOperationsViewsListExternalResultsWithReleaseAndUnattributable(t *testing.T) {
	views, fixture := newOperationsRecordViews(t)
	seedDemoShapedChain(t, fixture)

	results, err := views.ListExternalResults(t.Context(), recordTenant(t, "SYN-TENANT-01"), "", 10)
	if err != nil {
		t.Fatalf("上列外部结果：%v", err)
	}
	if len(results) != 2 {
		t.Fatalf("应有两份外部结果：%+v", results)
	}
	bySource := map[string]int{}
	for index, result := range results {
		bySource[result.Source] = index
	}
	release := results[bySource["RES-REL"]]
	if release.Layer != "RELEASE_RESULT" || release.SubmissionVersion != "SUBV-1" ||
		release.Release == nil || release.Release.Kind != "FULL" || release.OccurredAt.IsZero() {
		t.Fatalf("放行层结果走样：%+v", release)
	}
	lost := results[bySource["RES-LOST"]]
	if !lost.Unattributable || lost.SubmissionVersion != "" || lost.Release != nil {
		t.Fatalf("归属不上的响应不得被猜出版本或放行：%+v", lost)
	}
	scoped, err := views.ListExternalResults(t.Context(), recordTenant(t, "SYN-TENANT-01"), "SUBV-1", 10)
	if err != nil || len(scoped) != 1 {
		t.Fatalf("按版本过滤走样：%v %+v", err, scoped)
	}
}

// Covers: UC-CC-013 — 判断册逐动作答复与依据按判断当时原样上列，按对象过滤，他租户不可见。
func TestOperationsViewsListActionAdmissionJudgements(t *testing.T) {
	views, fixture := newOperationsRecordViews(t)
	seedDemoShapedChain(t, fixture)

	judgements, err := views.ListActionAdmissionJudgements(t.Context(), recordTenant(t, "SYN-TENANT-01"), "PCL-1", 10)
	if err != nil || len(judgements) != 1 {
		t.Fatalf("判断册上列走样：%v %+v", err, judgements)
	}
	judgement := judgements[0]
	if judgement.Answer != "ADMITTED" || judgement.OnwardNode != "NODE-B" || len(judgement.Findings) != 1 ||
		judgement.Findings[0].Bases[0].Sources[0] != "RES-REL" || !judgement.Zones[0].Registered {
		t.Fatalf("判断记录走样：%+v", judgement)
	}
	if none, err := views.ListActionAdmissionJudgements(t.Context(), recordTenant(t, "SYN-TENANT-01"), "PCL-NONE", 10); err != nil || len(none) != 0 {
		t.Fatalf("无判断的对象必须答有答案的空：%v %+v", err, none)
	}
	if other, err := views.ListActionAdmissionJudgements(t.Context(), recordTenant(t, "SYN-TENANT-02"), "", 10); err != nil || len(other) != 0 {
		t.Fatalf("他租户不得看见判断册：%v %+v", err, other)
	}
}

// Covers: ADR-0077 Decision 四 — 空册答空列表，不是错误；limit 非正是编程错误。
func TestOperationsViewsEmptyRegistersAnswerEmptyAndRejectNonPositiveLimit(t *testing.T) {
	views, _ := newOperationsRecordViews(t)
	tenant := recordTenant(t, "SYN-TENANT-01")
	if cases, err := views.ListCustomsCases(t.Context(), tenant, "", 10); err != nil || len(cases) != 0 {
		t.Fatalf("空库案件：%v %+v", err, cases)
	}
	if _, err := views.ListSubmissionVersions(t.Context(), tenant, "", 0); err == nil {
		t.Fatal("limit 为 0 必须拒")
	}
}
