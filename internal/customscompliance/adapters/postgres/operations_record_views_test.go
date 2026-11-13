package postgres_test

import (
	"net/url"
	"strings"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
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

	cases, err := rowsOf(views.ListCustomsCases(t.Context(), recordTenant(t, "SYN-TENANT-01"), 10, firstPage(t, ports.CustomsCasePages)))
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

	one, err := rowsOf(views.ListCustomsCases(t.Context(), recordTenant(t, "SYN-TENANT-01"), 10,
		pageQuery(t, ports.CustomsCasePages, url.Values{"case": {"CASE-OTHER"}})))
	if err != nil || len(one) != 0 {
		t.Fatalf("按他租户案件号单份读回必须答空：%v %+v", err, one)
	}
}

// Covers: demo-2026-10-30/01 — 提交版本连同尝试、发送事实、技术回执逐层上列，不折叠。
func TestOperationsViewsListSubmissionVersionsWithSendFactAndReceipts(t *testing.T) {
	views, fixture := newOperationsRecordViews(t)
	seedDemoShapedChain(t, fixture)

	versions, err := rowsOf(views.ListSubmissionVersions(t.Context(), recordTenant(t, "SYN-TENANT-01"), 10,
		firstPage(t, ports.SubmissionVersionPages)))
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
	if other, err := rowsOf(views.ListSubmissionVersions(t.Context(), recordTenant(t, "SYN-TENANT-02"), 10,
		firstPage(t, ports.SubmissionVersionPages))); err != nil || len(other) != 0 {
		t.Fatalf("他租户不得看见提交版本：%v %+v", err, other)
	}
}

// Covers: UC-CC-006 — 放行层结果带出放行事实；归属不上的响应照样上列且不带版本（留存不猜）。
func TestOperationsViewsListExternalResultsWithReleaseAndUnattributable(t *testing.T) {
	views, fixture := newOperationsRecordViews(t)
	seedDemoShapedChain(t, fixture)

	results, err := rowsOf(views.ListExternalResults(t.Context(), recordTenant(t, "SYN-TENANT-01"), "", 10,
		firstPage(t, ports.ExternalResultPages)))
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
	scoped, err := rowsOf(views.ListExternalResults(t.Context(), recordTenant(t, "SYN-TENANT-01"), "SUBV-1", 10,
		pageQuery(t, ports.ExternalResultPages, url.Values{"version": {"SUBV-1"}})))
	if err != nil || len(scoped) != 1 {
		t.Fatalf("按版本过滤走样：%v %+v", err, scoped)
	}
}

// Covers: UC-CC-013 — 判断册逐动作答复与依据按判断当时原样上列，按对象过滤，他租户不可见。
func TestOperationsViewsListActionAdmissionJudgements(t *testing.T) {
	views, fixture := newOperationsRecordViews(t)
	seedDemoShapedChain(t, fixture)

	judgements, err := rowsOf(views.ListActionAdmissionJudgements(t.Context(), recordTenant(t, "SYN-TENANT-01"), 10,
		pageQuery(t, ports.AdmissionJudgementPages, url.Values{"object": {"PCL-1"}})))
	if err != nil || len(judgements) != 1 {
		t.Fatalf("判断册上列走样：%v %+v", err, judgements)
	}
	judgement := judgements[0]
	if judgement.Answer != "ADMITTED" || judgement.OnwardNode != "NODE-B" || len(judgement.Findings) != 1 ||
		judgement.Findings[0].Bases[0].Sources[0] != "RES-REL" || !judgement.Zones[0].Registered {
		t.Fatalf("判断记录走样：%+v", judgement)
	}
	if none, err := rowsOf(views.ListActionAdmissionJudgements(t.Context(), recordTenant(t, "SYN-TENANT-01"), 10,
		pageQuery(t, ports.AdmissionJudgementPages, url.Values{"object": {"PCL-NONE"}}))); err != nil || len(none) != 0 {
		t.Fatalf("无判断的对象必须答有答案的空：%v %+v", err, none)
	}
	if other, err := rowsOf(views.ListActionAdmissionJudgements(t.Context(), recordTenant(t, "SYN-TENANT-02"), 10,
		firstPage(t, ports.AdmissionJudgementPages))); err != nil || len(other) != 0 {
		t.Fatalf("他租户不得看见判断册：%v %+v", err, other)
	}
}

// Covers: ADR-0077 Decision 四 — 空册答空列表，不是错误；limit 非正是编程错误。
func TestOperationsViewsEmptyRegistersAnswerEmptyAndRejectNonPositiveLimit(t *testing.T) {
	views, _ := newOperationsRecordViews(t)
	tenant := recordTenant(t, "SYN-TENANT-01")
	if cases, err := rowsOf(views.ListCustomsCases(t.Context(), tenant, 10, firstPage(t, ports.CustomsCasePages))); err != nil || len(cases) != 0 {
		t.Fatalf("空库案件：%v %+v", err, cases)
	}
	if _, err := views.ListSubmissionVersions(t.Context(), tenant, 0, firstPage(t, ports.SubmissionVersionPages)); err == nil {
		t.Fatal("limit 为 0 必须拒")
	}
}

// rejudge 把一份判断的判断时刻挪到 at：同一请求晚些时候重判的那一份。
func rejudge(t *testing.T, fixture *viewFixture, tenant, id string, at time.Time) {
	t.Helper()
	fixture.seed(t,
		`UPDATE customs_compliance.action_admission_judgement SET judged_at = $3 WHERE tenant_id = $1 AND judgement_id = $2`,
		tenant, id, at)
}

// seedJudgement 播一份出发的动作准入判断；answer 是整次答复，逐动作答复取同一格，依据只为让 jsonb 成形。
func seedJudgement(t *testing.T, fixture *viewFixture, tenant, id, object string, actionAt time.Time, answer string) {
	t.Helper()
	seedIntentJudgement(t, fixture, tenant, id, object, "DEPARTURE", actionAt, answer)
}

func seedIntentJudgement(t *testing.T, fixture *viewFixture, tenant, id, object, intent string, actionAt time.Time, answer string) {
	t.Helper()
	fixture.seed(t,
		`INSERT INTO customs_compliance.action_admission_judgement
			(tenant_id, judgement_id, object_ref, intent_kind, node_ref, onward_node_ref, action_at,
			 cross_zone, answer, zones, findings, judged_at)
		 VALUES ($1, $2, $3, $6, 'NODE-A', 'NODE-B', $4, 'CROSSING', $5, '[]',
		         jsonb_build_array(jsonb_build_object('action', 'LOADING_DEPARTURE', 'answer', $5::text, 'bases', '[]'::jsonb)),
		         $4)`,
		tenant, id, object, actionAt, answer, intent)
}

// Covers: AT-CC-456 — 每个对象只看动作时点最晚的那一刻，那一刻上每一次请求取判断时刻最晚的当前答复：有一份阻断、未决或未配置
// 就算卡关，只计一次，放行、不受门禁不算；先被挡后放行的不算，先放行后被挡的算；同一刻交出被挡、出发放行的算（被挡那份交回）；
// 同一请求同一刻先挡后重判放行的不算；同一刻阻断与未决并存交回阻断；晚报的早先动作不盖过之后那次；卡得最久的在前；件数不受页
// 大小截断；他租户的判断不进来。
func TestUnadmittedObjectsAreThoseWhoseLatestJudgementDoesNotAdmit(t *testing.T) {
	views, fixture := newOperationsRecordViews(t)
	tenant := "SYN-TENANT-01"
	at := func(hours int) time.Time { return viewBaseAt.Add(time.Duration(hours) * time.Hour) }
	seedJudgement(t, fixture, tenant, "J-A1", "PCL-A", at(1), "BLOCKED")
	seedJudgement(t, fixture, tenant, "J-A2", "PCL-A", at(5), "ADMITTED")
	seedJudgement(t, fixture, tenant, "J-B1", "PCL-B", at(1), "ADMITTED")
	seedJudgement(t, fixture, tenant, "J-B2", "PCL-B", at(4), "BLOCKED")
	seedJudgement(t, fixture, tenant, "J-C1", "PCL-C", at(2), "UNDECIDED")
	seedJudgement(t, fixture, tenant, "J-D1", "PCL-D", at(3), "NOT_CONFIGURED")
	seedJudgement(t, fixture, tenant, "J-E1", "PCL-E", at(3), "NOT_GUARDED")
	// PCL-F 的阻断是执行方晚报的一次早先动作：动作时点早于那次放行，判断时刻虽晚也不算最近。
	seedJudgement(t, fixture, tenant, "J-F1", "PCL-F", at(6), "ADMITTED")
	fixture.seed(t,
		`UPDATE customs_compliance.action_admission_judgement SET judged_at = $2 WHERE judgement_id = 'J-F1' AND tenant_id = $1`,
		tenant, at(6))
	seedJudgement(t, fixture, tenant, "J-F0", "PCL-F", at(2), "BLOCKED")
	fixture.seed(t,
		`UPDATE customs_compliance.action_admission_judgement SET judged_at = $2 WHERE judgement_id = 'J-F0' AND tenant_id = $1`,
		tenant, at(9))
	// PCL-G 同一动作时点问了交出又问了出发：交出被挡、出发放行。那一刻有一件事没做成，同刻的放行不把它盖掉。
	seedIntentJudgement(t, fixture, tenant, "J-G1", "PCL-G", "HANDOVER", at(7), "BLOCKED")
	seedIntentJudgement(t, fixture, tenant, "J-G2", "PCL-G", "DEPARTURE", at(7), "ADMITTED")
	// PCL-H 同一刻两份都不过门（阻断、未决），只计一次，交回最重的阻断——标识与判断时刻都排在未决后面也一样。
	seedIntentJudgement(t, fixture, tenant, "J-H2", "PCL-H", "DEPARTURE", at(8), "BLOCKED")
	seedIntentJudgement(t, fixture, tenant, "J-H9", "PCL-H", "HANDOVER", at(8), "UNDECIDED")
	rejudge(t, fixture, tenant, "J-H9", at(9))
	// PCL-T 同一请求在同一动作时点先判阻断、三源变了重判放行（另记一行）：当前答复是放行，不算卡关。
	seedIntentJudgement(t, fixture, tenant, "J-T1", "PCL-T", "DEPARTURE", at(9), "BLOCKED")
	seedIntentJudgement(t, fixture, tenant, "J-T2", "PCL-T", "DEPARTURE", at(9), "ADMITTED")
	rejudge(t, fixture, tenant, "J-T2", at(10))
	seedJudgement(t, fixture, "SYN-TENANT-02", "J-X1", "PCL-X", at(1), "BLOCKED")

	listed, err := views.ListUnadmittedObjects(t.Context(), recordTenant(t, tenant), 10, firstPage(t, ports.UnadmittedObjectPages))
	if err != nil {
		t.Fatalf("上列卡关对象：%v", err)
	}
	var objects []string
	for _, judgement := range listed.Rows {
		objects = append(objects, judgement.Object+"/"+judgement.Judgement)
	}
	want := []string{"PCL-C/J-C1", "PCL-D/J-D1", "PCL-B/J-B2", "PCL-G/J-G1", "PCL-H/J-H2"}
	if listed.Count != len(want) || len(objects) != len(want) {
		t.Fatalf("件数或清单走样：count=%d rows=%v，要 %v", listed.Count, objects, want)
	}
	for index := range want {
		if objects[index] != want[index] {
			t.Fatalf("清单走样（卡得最久的在前）：%v，要 %v", objects, want)
		}
	}
	if len(listed.Rows[0].Findings) != 1 || listed.Rows[0].OnwardNode != "NODE-B" {
		t.Fatalf("判断行形与判断册不同：%+v", listed.Rows[0])
	}

	page, err := views.ListUnadmittedObjects(t.Context(), recordTenant(t, tenant), 1, firstPage(t, ports.UnadmittedObjectPages))
	if err != nil || page.Count != len(want) || len(page.Rows) != 1 || page.Rows[0].Object != "PCL-C" {
		t.Fatalf("件数必须是全部而不是这一页：err=%v %+v", err, page)
	}
	none, err := views.ListUnadmittedObjects(t.Context(), recordTenant(t, "SYN-TENANT-03"), 10, firstPage(t, ports.UnadmittedObjectPages))
	if err != nil || none.Count != 0 || len(none.Rows) != 0 {
		t.Fatalf("没有判断的租户答 0 件：err=%v %+v", err, none)
	}
	if _, err := views.ListUnadmittedObjects(t.Context(), recordTenant(t, tenant), 0, firstPage(t, ports.UnadmittedObjectPages)); err == nil {
		t.Fatal("limit 为 0 必须拒")
	}
}

// Covers: 人工录入的外部结果与人工递交的发送事实在读面上带出留痕（谁、凭哪份原件）；接口来的整格缺席，两种分得开。
func TestOperationsViewsTellManualEntriesFromInterfaceDeliveredOnes(t *testing.T) {
	views, fixture := newOperationsRecordViews(t)
	seedDemoShapedChain(t, fixture)
	tenant := recordTenant(t, "SYN-TENANT-01")

	interfaceOnly, err := rowsOf(views.ListSubmissionVersions(t.Context(), tenant, 10, firstPage(t, ports.SubmissionVersionPages)))
	if err != nil || interfaceOnly[0].Attempts[0].SendFact == nil || interfaceOnly[0].Attempts[0].SendFact.ManualLodgement != nil {
		t.Fatalf("发送口发出去的发送事实不得带人工递交留痕：%v %+v", err, interfaceOnly)
	}

	fixture.seed(t,
		`UPDATE customs_compliance.external_result SET entered_by = 'SYN-OP-7', original_ref = '海关放行通知书.pdf'
		  WHERE tenant_id = 'SYN-TENANT-01' AND source_id = 'RES-REL'`)
	fixture.seed(t,
		`INSERT INTO customs_compliance.submission_manual_lodgement
			(tenant_id, version_id, sequence, lodged_by, original_ref, reference, recorded_at)
		 VALUES ('SYN-TENANT-01', 'SUBV-1', 1, 'SYN-OP-7', '单一窗口回执截图.png', 'ACK-778', $1)`, viewBaseAt.Add(time.Hour))

	results, err := rowsOf(views.ListExternalResults(t.Context(), tenant, "", 10, firstPage(t, ports.ExternalResultPages)))
	if err != nil {
		t.Fatalf("上列外部结果：%v", err)
	}
	for _, result := range results {
		switch result.Source {
		case "RES-REL":
			if result.EnteredBy != "SYN-OP-7" || result.Original != "海关放行通知书.pdf" {
				t.Fatalf("人工录入的结果没带出留痕：%+v", result)
			}
		default:
			if result.EnteredBy != "" || result.Original != "" {
				t.Fatalf("接口送来的结果不得带留痕：%+v", result)
			}
		}
	}

	versions, err := rowsOf(views.ListSubmissionVersions(t.Context(), tenant, 10, firstPage(t, ports.SubmissionVersionPages)))
	if err != nil {
		t.Fatalf("上列提交版本：%v", err)
	}
	lodgement := versions[0].Attempts[0].SendFact.ManualLodgement
	if lodgement == nil || lodgement.LodgedBy != "SYN-OP-7" || lodgement.Original != "单一窗口回执截图.png" ||
		lodgement.Reference != "ACK-778" || !lodgement.RecordedAt.Equal(viewBaseAt.Add(time.Hour)) {
		t.Fatalf("人工递交留痕走样：%+v", lodgement)
	}
}

// Covers: 舱单引用查阅——一份舱单一行、新近接受的在前、他租户读不到、q 按字面收窄；留痕只认「在册这一版的这一次录入」：
// 人工录入的在册版带出谁与原件；人录过之后承运方系统把它推到新版的不带；同版号但接受时刻对不上的旧留痕不算数。
func TestOperationsViewsListManifestReferencesAndTellManualVersions(t *testing.T) {
	views, fixture := newOperationsRecordViews(t)
	tenant := recordTenant(t, "SYN-TENANT-01")
	reference := func(tenantID, manifest, version string, acceptedAt time.Time, prior any) {
		fixture.seed(t,
			`INSERT INTO customs_compliance.external_manifest_reference
				(tenant_id, manifest_id, version_id, carrier_ref, procedure_ref, direction, scope_ref, source_fact, accepted_at, prior_version)
			 VALUES ($1, $2, $3, 'SYN-CARRIER-01', 'SYN-PROCEDURE-01', 'EXPORT', 'SYN-SCOPE-'||$2, 'SYN-FACT-'||$2, $4, $5)`,
			tenantID, manifest, version, acceptedAt, prior)
	}
	entry := func(manifest, version string, enteredAt time.Time) {
		fixture.seed(t,
			`INSERT INTO customs_compliance.external_manifest_manual_entry
				(tenant_id, carrier_ref, manifest_id, version_id, entered_by, original_ref, entered_at)
			 VALUES ('SYN-TENANT-01', 'SYN-CARRIER-01', $1, $2, 'SYN-OP-4', '舱单确认函 '||$1, $3)`,
			manifest, version, enteredAt)
	}
	// MF-MANUAL：在册版就是人录的那一次。
	reference("SYN-TENANT-01", "MF-MANUAL", "v1", viewBaseAt.Add(3*time.Hour), nil)
	entry("MF-MANUAL", "v1", viewBaseAt.Add(3*time.Hour))
	// MF-MOVED：人录过 v1，之后承运方系统推到 v2——在册的 v2 没有留痕。
	reference("SYN-TENANT-01", "MF-MOVED", "v2", viewBaseAt.Add(2*time.Hour), "v1")
	entry("MF-MOVED", "v1", viewBaseAt.Add(time.Hour))
	// MF-RESTATED：人录过 v1，推到别的版本后承运方系统又声明回 v1——版本号相同，但不是那一次录入。
	reference("SYN-TENANT-01", "MF-RESTATED", "v1", viewBaseAt.Add(time.Hour), "v2")
	entry("MF-RESTATED", "v1", viewBaseAt)
	reference("SYN-TENANT-02", "MF-OTHER", "v1", viewBaseAt, nil)

	rows, err := rowsOf(views.ListExternalManifestReferences(t.Context(), tenant, 10, firstPage(t, ports.ExternalManifestReferencePages)))
	if err != nil {
		t.Fatalf("上列舱单引用：%v", err)
	}
	if len(rows) != 3 || rows[0].Manifest != "MF-MANUAL" || rows[1].Manifest != "MF-MOVED" || rows[2].Manifest != "MF-RESTATED" {
		t.Fatalf("应按接受时刻新近在前列出本租户三份：%+v", rows)
	}
	manual := rows[0]
	if manual.EnteredBy != "SYN-OP-4" || manual.Original != "舱单确认函 MF-MANUAL" || manual.Carrier != "SYN-CARRIER-01" ||
		manual.Version != "v1" || manual.Direction != "EXPORT" || manual.Scope != "SYN-SCOPE-MF-MANUAL" ||
		manual.SourceFact != "SYN-FACT-MF-MANUAL" || !manual.AcceptedAt.Equal(viewBaseAt.Add(3*time.Hour)) || manual.PriorVersion != "" {
		t.Fatalf("人工录入的在册版走样：%+v", manual)
	}
	for _, declared := range rows[1:] {
		if declared.EnteredBy != "" || declared.Original != "" {
			t.Fatalf("在册版不是那一次人工录入的，不得带留痕：%+v", declared)
		}
	}
	if rows[1].PriorVersion != "v1" {
		t.Fatalf("前版没透出：%+v", rows[1])
	}
	narrowed, err := rowsOf(views.ListExternalManifestReferences(t.Context(), tenant, 10,
		pageQuery(t, ports.ExternalManifestReferencePages, url.Values{"q": {"mf-moved"}})))
	if err != nil || len(narrowed) != 1 || narrowed[0].Manifest != "MF-MOVED" {
		t.Fatalf("q 收窄走样：%v %+v", err, narrowed)
	}
	if empty, err := rowsOf(views.ListExternalManifestReferences(t.Context(), recordTenant(t, "SYN-TENANT-03"), 10,
		firstPage(t, ports.ExternalManifestReferencePages))); err != nil || len(empty) != 0 {
		t.Fatalf("空册应答空列表：%v %+v", err, empty)
	}
	// 游标翻页：行标识是两列（承运人、舱单号），页源带左连接——逐页取完三份，不重不漏。
	var walked []string
	after := url.Values{}
	for range 4 {
		page, err := views.ListExternalManifestReferences(t.Context(), tenant, 1, pageQuery(t, ports.ExternalManifestReferencePages, after))
		if err != nil {
			t.Fatalf("翻页：%v", err)
		}
		for _, row := range page.Rows {
			walked = append(walked, row.Manifest)
		}
		if page.Next == "" {
			break
		}
		after = url.Values{"after": {page.Next}}
	}
	if strings.Join(walked, ",") != "MF-MANUAL,MF-MOVED,MF-RESTATED" {
		t.Fatalf("逐页走出来的次序 = %v", walked)
	}
	if _, err := views.ListExternalManifestReferences(t.Context(), tenant, 0, firstPage(t, ports.ExternalManifestReferencePages)); err == nil {
		t.Fatal("页大小 0 没被拒")
	}
}
