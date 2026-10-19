package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

// 动作准入判断在真库上走通（隔离合成 S）：区域册、对象范围、限制、门禁目录、放行事实、判断册全是 0003–0024 的真表，
// 编排是生产那一只。五格各钉一遍，外加重放与「不入队」。种子走显式 SQL 与写口两路：门禁与放行的写入编排不在
// 本票，绕开它们才看得清准入判断读到的是库里的什么。

var admissionDBAt = time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)

const (
	admissionTenant    = "SYN-TENANT-01"
	admissionParcel    = "SYN-PCL-ADMISSION-01"
	admissionUnitRef   = "SYN-UNIT-CN-EXPORT-ADM-01"
	admissionCaseRef   = "SYN-CASE-CN-EXPORT-ADM-01"
	admissionProcedure = "SYN-PROC-CN-EXPORT"
)

type admissionDB struct {
	t       *testing.T
	fixture *viewFixture
	handler *application.JudgeActionAdmissionHandler
	zones   *adapter.NodeCustomsZoneRegistrations
	clock   *admissionDBClock
}

type admissionDBClock struct{ at time.Time }

func (clock *admissionDBClock) Now() time.Time { return clock.at }

func newAdmissionDB(t *testing.T) *admissionDB {
	t.Helper()
	fixture := newViewFixture(t)
	construct := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("构造适配器：%v", err)
		}
	}
	zoneRegistry, err := adapter.NewNodeCustomsZoneRegistrations(fixture.db)
	construct(err)
	zoneView, err := adapter.NewNodeCustomsZonePointView(fixture.db)
	construct(err)
	scopes, err := adapter.NewObjectCustomsScopeView(fixture.db)
	construct(err)
	restrictions, err := adapter.NewRestrictions(fixture.db)
	construct(err)
	gates, err := adapter.NewGateConditionView(fixture.db)
	construct(err)
	duty, err := adapter.NewDutyPaymentReconciliation(fixture.db)
	construct(err)
	releases, err := adapter.NewReleaseOutcomeView(fixture.db)
	construct(err)
	store, err := adapter.NewActionAdmissionJudgements(fixture.db)
	construct(err)
	clock := &admissionDBClock{at: admissionDBAt.Add(time.Minute)}
	handler, err := application.NewJudgeActionAdmissionHandler(application.JudgeActionAdmissionDeps{
		Zones:             zoneView,
		Scopes:            scopes,
		Restrictions:      restrictions,
		Conditions:        gates,
		DutyRules:         gates,
		DutyVerifications: duty,
		Releases:          releases,
		Store:             store,
		Clock:             clock,
	})
	construct(err)
	return &admissionDB{t: t, fixture: fixture, handler: handler, zones: zoneRegistry, clock: clock}
}

func (db *admissionDB) zone(node, zone string) {
	db.t.Helper()
	outcome, err := register(db.t, db.fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return db.zones.RegisterNodeCustomsZone(ctx,
			viewValue(db.t, domain.NewTenantID, admissionTenant),
			viewValue(db.t, domain.NewNetworkNodeReference, node),
			viewValue(db.t, domain.NewCustomsZoneReference, zone),
			time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	})
	if err != nil || outcome != ports.CaseConfigurationRegistered {
		db.t.Fatalf("登区域 %s：%v / %d", node, err, outcome)
	}
}

// demoZones 登演示租户的四个节点：上海、深圳在 CN，新加坡两个在 SG——与演示种子同形（证据 S）。
func (db *admissionDB) demoZones() {
	db.zone("SYN-NODE-SHA-HUB", "SYN-ZONE-CN")
	db.zone("SYN-NODE-SZX-GATE", "SYN-ZONE-CN")
	db.zone("SYN-NODE-SIN-HUB", "SYN-ZONE-SG")
	db.zone("SYN-NODE-SIN-LM", "SYN-ZONE-SG")
}

// inUnit 立一个出口案件与一个含本包裹的申报单元。
func (db *admissionDB) inUnit() {
	db.fixture.seed(db.t,
		`INSERT INTO customs_compliance.customs_case
			(tenant_id, jurisdiction_ref, direction, procedure_ref, obligation_ref, case_id, parcels, roles, established_at)
		 VALUES ($1, 'SYN-JUR-CN', 'EXPORT', $2, 'SYN-OBLIGATION-01', $3,
		         jsonb_build_array(jsonb_build_object('parcel', $4::text, 'customer', 'SYN-ACCOUNT-01', 'sourceRef', 'SYN-SRC-01')),
		         '[]'::jsonb, $5)`,
		admissionTenant, admissionProcedure, admissionCaseRef, admissionParcel, admissionDBAt.Add(-48*time.Hour))
	db.fixture.seed(db.t,
		`INSERT INTO customs_compliance.declaration_unit
			(tenant_id, unit_id, case_id, procedure_ref, members, formed_at)
		 VALUES ($1, $2, $3, $4, jsonb_build_array($5::text), $6)`,
		admissionTenant, admissionUnitRef, admissionCaseRef, admissionProcedure, admissionParcel, admissionDBAt.Add(-47*time.Hour))
}

// gate 登一本门禁目录：findings 为空即「此动作在此边界不受门禁」；dutyNotAPrecondition 为真时同登「税费付款不构成
// 前置条件」那条规则（没登规则时这一道答规则未配置）。
func (db *admissionDB) gate(action string, dutyNotAPrecondition bool, findings map[string]string) {
	db.t.Helper()
	db.fixture.seed(db.t,
		`INSERT INTO customs_compliance.gate_condition_catalog (tenant_id, scope_ref, action, boundary_ref, registered_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		admissionTenant, admissionUnitRef, action, admissionProcedure, admissionDBAt.Add(-46*time.Hour))
	for precondition, state := range findings {
		db.fixture.seed(db.t,
			`INSERT INTO customs_compliance.gate_condition_finding
				(tenant_id, scope_ref, action, boundary_ref, precondition_ref, finding_state)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			admissionTenant, admissionUnitRef, action, admissionProcedure, precondition, state)
	}
	if dutyNotAPrecondition {
		db.fixture.seed(db.t,
			`INSERT INTO customs_compliance.gate_condition_duty_payment_rule
				(tenant_id, scope_ref, action, boundary_ref, not_a_precondition,
				 accept_coverage, accept_delta, accept_validity, registered_at)
			 VALUES ($1, $2, $3, $4, true, '[]', '[]', '[]', $5)`,
			admissionTenant, admissionUnitRef, action, admissionProcedure, admissionDBAt.Add(-46*time.Hour))
	}
}

// release 落一份放行层接收记录与它的放行事实（0001 + 0015），业务时间取 occurredAt。
func (db *admissionDB) release(source, kind string, occurredAt time.Time, layerConflict bool) {
	db.t.Helper()
	condition := ""
	if kind == "CONDITIONAL" {
		condition = "SYN-COND-BONDED"
	}
	db.fixture.seed(db.t,
		`INSERT INTO customs_compliance.external_result
			(tenant_id, source_id, content_digest, unattributable, layer_conflict, result, submission_version, recorded_at)
		 VALUES ($1, $2, 'syn-digest', false, $3,
		         jsonb_build_object('layer', 'RELEASE', 'sourceId', $2::text, 'scope', $4::text,
		                            'occurredAt', $5::timestamptz, 'receivedAt', $6::timestamptz),
		         'SYN-SUBV-01', $6)`,
		admissionTenant, source, layerConflict, admissionUnitRef, occurredAt.UTC(), occurredAt.Add(time.Minute).UTC())
	db.fixture.seed(db.t,
		`INSERT INTO customs_compliance.release_outcome
			(tenant_id, source_id, kind, authority_ref, scope_ref, condition, received_at)
		 VALUES ($1, $2, $3, 'SYN-AUTHORITY-CN', $4, $5, $6)`,
		admissionTenant, source, kind, admissionUnitRef, condition, occurredAt.Add(time.Minute).UTC())
}

func (db *admissionDB) restrictParcel(id, action string) {
	db.t.Helper()
	db.fixture.seed(db.t,
		`INSERT INTO customs_compliance.regulatory_restriction
			(tenant_id, restriction_id, decision_id, scope_ref, constrains, effective_at)
		 VALUES ($1, $2, 'SYN-DECISION-HOLD', $3, jsonb_build_array($4::text), $5)`,
		admissionTenant, id, admissionParcel, action, admissionDBAt.Add(-3*time.Hour))
}

func (db *admissionDB) judge(from, to string) (application.JudgeActionAdmissionResult, domain.ActionAdmissionJudgement, string) {
	db.t.Helper()
	intent, err := domain.NewDepartureIntent(
		viewValue(db.t, domain.NewNetworkNodeReference, from), viewValue(db.t, domain.NewNetworkNodeReference, to))
	if err != nil {
		db.t.Fatalf("构造出发意图：%v", err)
	}
	command := application.JudgeActionAdmissionCommand{
		TenantID: viewValue(db.t, domain.NewTenantID, admissionTenant),
		Object:   viewValue(db.t, domain.NewDeclaredParcelReference, admissionParcel),
		Intent:   intent,
		At:       admissionDBAt,
	}
	var result application.JudgeActionAdmissionResult
	err = db.fixture.db.Transactor().WithinTransaction(db.t.Context(), func(txCtx context.Context) error {
		var handleErr error
		result, handleErr = db.handler.Handle(txCtx, command)
		return handleErr
	})
	if err != nil {
		db.t.Fatalf("准入判断：%v", err)
	}
	judgement, id, ok := result.Judgement()
	if !ok {
		db.t.Fatalf("判断没形成：%s / %s", result.Outcome(), result.UnreadableReason())
	}
	return result, judgement, id
}

func admissionFinding(t *testing.T, judgement domain.ActionAdmissionJudgement, action domain.GuardedAction) domain.ActionAdmissionFinding {
	t.Helper()
	for _, finding := range judgement.Findings() {
		if finding.Action == action {
			return finding
		}
	}
	t.Fatalf("判断里没有 %s：%+v", action, judgement.Findings())
	return domain.ActionAdmissionFinding{}
}

func expectAdmission(t *testing.T, finding domain.ActionAdmissionFinding, answer domain.AdmissionAnswer, cause domain.AdmissionCause) {
	t.Helper()
	if finding.Answer != answer || len(finding.Bases) == 0 || finding.Bases[0].Cause != cause {
		t.Fatalf("%s = %s %+v，要 %s / %s", finding.Action, finding.Answer, finding.Bases, answer, cause)
	}
}

// 演示判据（证据 S）：SHA→SZX 只适用装载出发；SZX→SIN 另加跨关务区域移动。对象不在任何案件与单元里：同区那一步
// 不受门禁，跨境那一步缺案可依答未决。
func TestAdmissionOnRealPostgresAppliesActionsByNodeZones(t *testing.T) {
	db := newAdmissionDB(t)
	db.demoZones()

	_, domestic, _ := db.judge("SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE")
	if len(domestic.Findings()) != 1 || domestic.CrossZone() != domain.CrossZoneSame {
		t.Fatalf("SHA→SZX 只适用装载出发：%s %+v", domestic.CrossZone(), domestic.Findings())
	}
	expectAdmission(t, admissionFinding(t, domestic, domain.LoadingDeparture), domain.AdmissionNotGuarded, domain.CauseOutsideCustomsBooks)

	_, crossing, _ := db.judge("SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB")
	if len(crossing.Findings()) != 2 || crossing.CrossZone() != domain.CrossZoneCrossing {
		t.Fatalf("SZX→SIN 另加跨区：%s %+v", crossing.CrossZone(), crossing.Findings())
	}
	expectAdmission(t, admissionFinding(t, crossing, domain.CrossCustomsMovement), domain.AdmissionUndecided, domain.CauseCustomsScopeUnresolved)
	if crossing.Answer() != domain.AdmissionUndecided {
		t.Fatalf("SZX→SIN 整次答复 = %s", crossing.Answer())
	}
}

func TestAdmissionOnRealPostgresAdmitsOnlyWithAGateMetAndAFullRelease(t *testing.T) {
	db := newAdmissionDB(t)
	db.demoZones()
	db.inUnit()
	db.gate("LOADING_DEPARTURE", true, nil)
	db.gate("CROSS_CUSTOMS_MOVEMENT", true, map[string]string{"SYN-PRE-EXPORT-DECL-RELEASED": "MET"})

	// 门禁满足、还没有放行：阻断。
	_, waiting, _ := db.judge("SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB")
	expectAdmission(t, admissionFinding(t, waiting, domain.LoadingDeparture), domain.AdmissionNotGuarded, domain.CauseGateNotApplicable)
	expectAdmission(t, admissionFinding(t, waiting, domain.CrossCustomsMovement), domain.AdmissionBlocked, domain.CauseReleaseNotReceived)
	if waiting.Answer() != domain.AdmissionBlocked {
		t.Fatalf("门禁满足不答放行：%s", waiting.Answer())
	}

	// 同层冲突的那份全部放行不算；部分放行说不出本件放没放，答未决。
	db.release("SYN-SRC-CONFLICT", "FULL", admissionDBAt.Add(-2*time.Hour), true)
	db.release("SYN-SRC-PARTIAL", "PARTIAL", admissionDBAt.Add(-2*time.Hour), false)
	_, partial, _ := db.judge("SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB")
	expectAdmission(t, admissionFinding(t, partial, domain.CrossCustomsMovement), domain.AdmissionUndecided, domain.CauseReleasePartialOnly)

	// 动作时点之前生效的全部放行：放行。
	db.release("SYN-SRC-FULL", "FULL", admissionDBAt.Add(-time.Hour), false)
	_, admitted, _ := db.judge("SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB")
	finding := admissionFinding(t, admitted, domain.CrossCustomsMovement)
	expectAdmission(t, finding, domain.AdmissionAdmitted, domain.CauseReleasedInFull)
	if finding.Bases[0].Sources[0] != "SYN-SRC-FULL" {
		t.Fatalf("放行依据要指回那份放行：%v", finding.Bases[0].Sources)
	}
	if admitted.Answer() != domain.AdmissionAdmitted {
		t.Fatalf("整次答复 = %s", admitted.Answer())
	}

	// 外部放行不解内部限制：直接落在包裹上的限制照挡。
	db.restrictParcel("SYN-RESTRICTION-ADM-01", "CROSS_CUSTOMS_MOVEMENT")
	_, blocked, _ := db.judge("SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB")
	expectAdmission(t, admissionFinding(t, blocked, domain.CrossCustomsMovement), domain.AdmissionBlocked, domain.CauseRestrictionInForce)
}

// 未配置两种来路：门禁目录没登；节点没登区域而对象带着跨区的门。
func TestAdmissionOnRealPostgresAnswersNotConfigured(t *testing.T) {
	db := newAdmissionDB(t)
	db.demoZones()
	db.inUnit()
	_, catalogMissing, _ := db.judge("SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE")
	expectAdmission(t, admissionFinding(t, catalogMissing, domain.LoadingDeparture), domain.AdmissionNotConfigured, domain.CauseGateCatalogNotRegistered)

	unzoned := newAdmissionDB(t)
	unzoned.zone("SYN-NODE-SZX-GATE", "SYN-ZONE-CN")
	unzoned.inUnit()
	unzoned.gate("LOADING_DEPARTURE", true, nil)
	unzoned.gate("CROSS_CUSTOMS_MOVEMENT", true, map[string]string{"SYN-PRE-EXPORT-DECL-RELEASED": "MET"})
	_, judgement, _ := unzoned.judge("SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB")
	finding := admissionFinding(t, judgement, domain.CrossCustomsMovement)
	expectAdmission(t, finding, domain.AdmissionNotConfigured, domain.CauseNodeZoneNotRegistered)
	if finding.Bases[0].Sources[0] != "SYN-NODE-SIN-HUB" || judgement.Answer() != domain.AdmissionNotConfigured {
		t.Fatalf("未配置要指名未登记的节点：%+v / %s", finding.Bases, judgement.Answer())
	}
}

// 重放回到同一行、不另记；准入判断不入队。
func TestAdmissionOnRealPostgresReplaysWithoutEnqueueing(t *testing.T) {
	db := newAdmissionDB(t)
	db.demoZones()
	countOutbox := func() int {
		var count int
		if err := db.fixture.pool.QueryRow(t.Context(), `SELECT count(*) FROM `+migrate.SchemaBento+`.outbox`).Scan(&count); err != nil {
			t.Fatalf("数 Outbox：%v", err)
		}
		return count
	}
	before := countOutbox()

	first, judgement, id := db.judge("SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE")
	db.clock.at = db.clock.at.Add(time.Hour)
	replay, replayed, replayID := db.judge("SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE")
	if first.Outcome() != application.ActionAdmissionJudged || replay.Outcome() != application.ActionAdmissionExisting {
		t.Fatalf("首判 / 重放 = %s / %s", first.Outcome(), replay.Outcome())
	}
	if replayID != id || !replayed.JudgedAt().Equal(judgement.JudgedAt()) {
		t.Fatalf("重放要回到首判那一行：%s@%s vs %s@%s", replayID, replayed.JudgedAt(), id, judgement.JudgedAt())
	}

	var rows int
	var answer, findings string
	if err := db.fixture.pool.QueryRow(t.Context(),
		`SELECT count(*), max(answer), max(findings::text) FROM customs_compliance.action_admission_judgement WHERE tenant_id = $1`,
		admissionTenant).Scan(&rows, &answer, &findings); err != nil {
		t.Fatalf("读判断册：%v", err)
	}
	if rows != 1 || answer != "NOT_GUARDED" {
		t.Fatalf("判断册 = %d 行 / %s", rows, answer)
	}
	if countOutbox() != before {
		t.Fatal("准入判断不该入队")
	}
}

// 库上的词表与形状防线：旁路写入也进不来。
func TestActionAdmissionJudgementTableRefusesOutOfVocabularyRows(t *testing.T) {
	fixture := newViewFixture(t)
	insert := `INSERT INTO customs_compliance.action_admission_judgement
		(tenant_id, judgement_id, object_ref, intent_kind, node_ref, onward_node_ref,
		 action_at, cross_zone, answer, zones, findings, judged_at)
		VALUES ('t', 'AADJ-1', 'p', $1, 'n', $2, now(), $3, $4, '[]', $5::jsonb, now())`
	fixture.rejects(t, "意图词表外", insert, "ARRIVAL", "m", "SAME_ZONE", "ADMITTED", `[{}]`)
	fixture.rejects(t, "出发缺终点", insert, "DEPARTURE", nil, "SAME_ZONE", "ADMITTED", `[{}]`)
	fixture.rejects(t, "答复词表外", insert, "DELIVERY", nil, "NOT_ASKED", "RELEASED", `[{}]`)
	fixture.rejects(t, "没有逐动作答复", insert, "DELIVERY", nil, "NOT_ASKED", "ADMITTED", `[]`)
}

// PBC-08 行为面负向证据：本票两只写口在无事务上下文必须被 RequireExecutor 拒绝。判断册写口先核词表、后取执行器，
// 所以喂一份合法的判断让它走到守卫。
func TestActionAdmissionWritesRefuseToRunOutsideATransaction(t *testing.T) {
	fixture := newViewFixture(t)
	ctx := t.Context()

	zones, err := adapter.NewNodeCustomsZoneRegistrations(fixture.db)
	if err != nil {
		t.Fatalf("构造区域写口：%v", err)
	}
	if _, err := zones.RegisterNodeCustomsZone(ctx, domain.TenantID{}, domain.NetworkNodeReference{}, domain.CustomsZoneReference{}, admissionDBAt); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登区域应返回 ErrTransactionRequired，实得：%v", err)
	}

	judgements, err := adapter.NewActionAdmissionJudgements(fixture.db)
	if err != nil {
		t.Fatalf("构造判断册：%v", err)
	}
	intent, err := domain.NewDeliveryIntent(viewValue(t, domain.NewNetworkNodeReference, "SYN-NODE-SIN-LM"))
	if err != nil {
		t.Fatalf("构造交付意图：%v", err)
	}
	judgement, err := domain.JudgeActionAdmission(domain.ActionAdmissionInput{
		Object:   viewValue(t, domain.NewDeclaredParcelReference, admissionParcel),
		Intent:   intent,
		At:       admissionDBAt,
		JudgedAt: admissionDBAt,
	})
	if err != nil {
		t.Fatalf("构造判断：%v", err)
	}
	_, err = judgements.SaveActionAdmissionJudgement(ctx, viewValue(t, domain.NewTenantID, admissionTenant), "AADJ-guard", judgement)
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务记判断应返回 ErrTransactionRequired，实得：%v", err)
	}
	// 缺事务是接线错，不是判断册读写不动：标成拒收，编排才不会把它答成读不动让消费方重投。
	if !errors.Is(err, ports.ErrActionAdmissionJudgementRejected) {
		t.Errorf("无事务记判断应标 ErrActionAdmissionJudgementRejected，实得：%v", err)
	}
}
