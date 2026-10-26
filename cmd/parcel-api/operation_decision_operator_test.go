package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 证 ADR-0303 的生产装配半边：七个运营决定口经操作者渠道「运营决定」能力面走到真编排，令牌认出的
// 操作者随决定落进库里那一列；同一份载荷换一位操作者再投，答复是各口既有的重放或已有格、库里那一列不变，回执里没有任何
// 一位操作者。与生产装配的差别只在令牌核验方与操作者册是替身。夹具全为合成（S 级）。

const (
	operatorIssuerForEvidence = "https://id.syn.example/dex"
	firstOperatorToken        = "token.of.operator.a"
	secondOperatorToken       = "token.of.operator.b"
	firstOperatorRecorded     = operatorIssuerForEvidence + "#SYN-OPERATOR-A"
)

// twoOperatorVerifier 按出示的令牌认出两位合成操作者，同一发行方；其余令牌一律不过。
type twoOperatorVerifier struct{}

func (twoOperatorVerifier) VerifyOperatorCredential(_ context.Context, credential accessidentity.OperatorCredential) (accessidentity.OperatorSubject, error) {
	switch credential.Token() {
	case firstOperatorToken:
		return accessidentity.NewOperatorSubject(operatorIssuerForEvidence, "SYN-OPERATOR-A")
	case secondOperatorToken:
		return accessidentity.NewOperatorSubject(operatorIssuerForEvidence, "SYN-OPERATOR-B")
	default:
		return accessidentity.OperatorSubject{}, accessidentity.ErrCredentialRejected
	}
}

var operationEvidenceKinds = []accessidentity.DecisionKind{
	accessidentity.DecisionSegmentClosure,
	accessidentity.DecisionParticipationTermination,
	accessidentity.DecisionDispatchTaskRegistration,
	accessidentity.DecisionLoadAssignment,
	accessidentity.DecisionEffectiveTimeJudgment,
	accessidentity.DecisionCarrierFirstEffectivePickupJudgment,
	accessidentity.DecisionTransportCommissionSubmission,
}

// decideTwice 让两位操作者先后投同一份载荷，核两次答复，交回第二次的原文。
func decideTwice(
	t *testing.T,
	endpoint http.Handler,
	path, body string,
	first, second [2]any,
) string {
	t.Helper()
	status, answer := postWithToken(t, endpoint, path, firstOperatorToken, body)
	if status != first[0] || answer["outcome"] != first[1] {
		t.Fatalf("%s 首投 = %d %v，want %v %v", path, status, answer, first[0], first[1])
	}
	status, raw := postWithTokenRaw(endpoint, path, secondOperatorToken, body)
	if status != second[0] || !strings.Contains(raw, `"outcome":"`+second[1].(string)+`"`) {
		t.Fatalf("%s 另一位操作者再投 = %d %s，want %v %v", path, status, raw, second[0], second[1])
	}
	if strings.Contains(raw, "SYN-OPERATOR") {
		t.Fatalf("%s 回执带了操作证据：%s", path, raw)
	}
	return raw
}

func recordedOperator(t *testing.T, pool *pgxpool.Pool, query string, args ...any) string {
	t.Helper()
	var values []string
	rows, err := pool.Query(t.Context(), query, args...)
	if err != nil {
		t.Fatalf("读 %q：%v", query, err)
	}
	defer rows.Close()
	for rows.Next() {
		var value *string
		if err := rows.Scan(&value); err != nil {
			t.Fatal(err)
		}
		if value == nil {
			values = append(values, "<NULL>")
			continue
		}
		values = append(values, *value)
	}
	if rows.Err() != nil || len(values) != 1 {
		t.Fatalf("%q 读到 %v（%v），want 恰一行", query, values, rows.Err())
	}
	return values[0]
}

// Covers: `AT-TF-335`、`AT-TF-336`、`AT-TF-337`、`AT-TF-338`、`AT-TF-339`、`AT-TF-340`、`AT-TF-341` 生产装配半边（真库）——关段、明确终止、建派送任务、装载分配、有效时间判断、承运商首次有效
// 收寄判断与运输委托提交七口：首投的操作者落进各自那一列，另一位操作者原样再投答各口既有的重放或已有格、那一列不变、回执里
// 没有操作者。
func TestEachOperationDecisionRecordsTheOperatorWhoMadeItAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registerOperationDecisionAdmission(t, db, isolatedSelfAuthority, operationEvidenceKinds...)
	admission, err := buildAdmissionScope(db, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	minter, err := buildOperatorMinter(twoOperatorVerifier{}, decisionRegister{kinds: operationEvidenceKinds}, admission)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	operations, err := buildSegmentOperations(db)
	if err != nil {
		t.Fatalf("装配段运营编排：%v", err)
	}

	t.Run("transport commission submission", func(t *testing.T) {
		submitter, err := buildTransportCommissionSubmission(db)
		if err != nil {
			t.Fatalf("装配委托提交：%v", err)
		}
		trunk := publishSeededSupplierAgreements(t, db)["SYN-SUPPLIER-TRUNK-CN-SG-01"]
		body := commissionBody("SYN-COMMISSION-OE", trunk.Declarations.SupplierAgreementBody.Supplier, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z")
		decideTwice(t, tfhttp.NewSubmitTransportCommissionEndpoint(decisions.transport, submitter), commissionSubmissionPath, body,
			[2]any{http.StatusCreated, "COMMISSION_SUBMITTED"}, [2]any{http.StatusOK, "EXISTING_COMMISSION"})
		if got := recordedOperator(t, pool, `SELECT submitted_by FROM transport_fulfillment.transport_commission
			 WHERE tenant_id = 'SYN-TENANT-01' AND commission_id = 'SYN-COMMISSION-OE'`); got != firstOperatorRecorded {
			t.Fatalf("提交人 = %q，want %q", got, firstOperatorRecorded)
		}
	})

	t.Run("dispatch task registration", func(t *testing.T) {
		body := `{"task":"SYN-DISPATCH-OE","kind":"DELIVERY","objects":["SYN-PARCEL-OE-DT"],"place":"SYN-PLACE/consignee-OE",` +
			`"windowFrom":"2026-09-25T09:00:00+08:00","windowTo":"2026-09-25T12:00:00+08:00","conditions":"SYN-CONDITION/signature-required",` +
			`"openedAt":"2026-09-24T20:00:00+08:00"}`
		decideTwice(t, tfhttp.NewOpenDispatchTaskEndpoint(decisions.transport, operations.opener), "/transport-fulfillment-dispatch-task-registrations", body,
			[2]any{http.StatusCreated, "DISPATCH_TASK_OPENED"}, [2]any{http.StatusOK, "DISPATCH_TASK_ALREADY_OPEN"})
		if got := recordedOperator(t, pool, `SELECT opened_by FROM transport_fulfillment.dispatch_task
			 WHERE tenant_id = 'SYN-TENANT-01' AND task_ref = 'SYN-DISPATCH-OE'`); got != firstOperatorRecorded {
			t.Fatalf("建任务人 = %q，want %q", got, firstOperatorRecorded)
		}
	})

	t.Run("load assignment", func(t *testing.T) {
		registerTransportSchedules(t, db, "SYN-TENANT-01", "SYN-SCHEDULE-OE")
		body := `{"assignment":"SYN-LOAD-ASSIGNMENT-OE","schedule":"SYN-SCHEDULE-OE","members":["SYN-PARCEL-OE-LA"],"version":"v1",` +
			`"assignedAt":"2026-10-18T10:00:00Z"}`
		decideTwice(t, tfhttp.NewFormLoadAssignmentEndpoint(decisions.transport, operations.assign), "/transport-fulfillment-load-assignment-registrations", body,
			[2]any{http.StatusCreated, "LOAD_ASSIGNMENT_FORMED"}, [2]any{http.StatusOK, "LOAD_ASSIGNMENT_VERSION_EXISTS"})
		if got := recordedOperator(t, pool, `SELECT decided_by FROM transport_fulfillment.load_assignment
			 WHERE tenant_id = 'SYN-TENANT-01' AND assignment_ref = 'SYN-LOAD-ASSIGNMENT-OE'`); got != firstOperatorRecorded {
			t.Fatalf("分配人 = %q，want %q", got, firstOperatorRecorded)
		}
	})

	t.Run("participation termination and segment closure", func(t *testing.T) {
		controlFacts, err := buildControlFactOrchestrations(db)
		if err != nil {
			t.Fatalf("装配控制事实编排：%v", err)
		}
		handover := registerHandoverCommand(t, "SYN-PARCEL-OE-SEG", "SYN-SEGMENT-OE", "")
		handover.TenantID = mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-01")
		if entered, err := controlFacts.handover.Register(t.Context(), handover); err != nil || entered.Outcome() != tfapp.HandoverRegistered {
			t.Fatalf("立段：outcome=%v err=%v", entered.Outcome(), err)
		}
		termination := `{"segment":"SYN-SEGMENT-OE","object":"SYN-PARCEL-OE-SEG","basis":"SYN-CONTROL-TERMINATION/CASE-OE",` +
			`"endedAt":"` + controlFactJudgedAt.Add(2*time.Hour).Format(time.RFC3339) + `"}`
		decideTwice(t, tfhttp.NewTerminateFulfillmentParticipationEndpoint(decisions.transport, operations.enderOf),
			"/transport-fulfillment-participation-terminations", termination,
			[2]any{http.StatusCreated, "PARTICIPATION_ENDED"}, [2]any{http.StatusOK, "PARTICIPATION_ALREADY_ENDED"})
		if got := recordedOperator(t, pool, `SELECT terminated_by FROM transport_fulfillment.fulfillment_participation
			 WHERE tenant_id = 'SYN-TENANT-01' AND segment_ref = 'SYN-SEGMENT-OE'`); got != firstOperatorRecorded {
			t.Fatalf("终止人 = %q，want %q", got, firstOperatorRecorded)
		}

		closure := `{"segment":"SYN-SEGMENT-OE","closedAt":"` + controlFactJudgedAt.Add(4*time.Hour).Format(time.RFC3339) + `"}`
		decideTwice(t, tfhttp.NewCloseFulfillmentSegmentEndpoint(decisions.transport, operations.closer), "/transport-fulfillment-segment-closures", closure,
			[2]any{http.StatusCreated, "SEGMENT_CLOSED"}, [2]any{http.StatusOK, "SEGMENT_ALREADY_CLOSED"})
		if got := recordedOperator(t, pool, `SELECT closed_by FROM transport_fulfillment.actual_fulfillment_segment
			 WHERE tenant_id = 'SYN-TENANT-01' AND segment_ref = 'SYN-SEGMENT-OE'`); got != firstOperatorRecorded {
			t.Fatalf("关段人 = %q，want %q", got, firstOperatorRecorded)
		}
	})

	t.Run("effective time judgment", func(t *testing.T) {
		judge, err := buildEffectiveTimeJudgment(db)
		if err != nil {
			t.Fatalf("装配有效时间判断：%v", err)
		}
		facts, err := tfpostgres.NewExternalTrackingFacts(db)
		if err != nil {
			t.Fatal(err)
		}
		tenant := mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-01")
		source := mustValue(t, tfdomain.NewTrackingSourceReference, "SYN-CARRIER-Y")
		pending := pendingExternalTrackingFact(t, tenant, source, "SYN-EXTF-OE", "SYN-EXTV-OE")
		if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			_, saveErr := facts.Save(txCtx, pending)
			return saveErr
		}); err != nil {
			t.Fatalf("落待判断事实：%v", err)
		}
		body := `{"fact":"SYN-EXTF-OE","effectiveAt":"` + effectiveTimeJudgmentAnchor.Add(5*time.Minute).Format(time.RFC3339) + `"}`
		decideTwice(t, tfhttp.NewJudgeEffectiveTimeEndpoint(decisions.transport, judge), "/transport-fulfillment-effective-time-judgments", body,
			[2]any{http.StatusCreated, "EFFECTIVE_TIME_JUDGED"}, [2]any{http.StatusOK, "ALREADY_JUDGED_AS_GIVEN"})
		if got := recordedOperator(t, pool, `SELECT effective_judged_by FROM transport_fulfillment.external_carrier_tracking_fact
			 WHERE tenant_id = 'SYN-TENANT-01' AND fact_ref = 'SYN-EXTF-OE' AND version_origin = 'JUDGMENT'`); got != firstOperatorRecorded {
			t.Fatalf("判断人 = %q，want %q", got, firstOperatorRecorded)
		}
		rows, err := facts.ListCurrentExternalTrackingFacts(t.Context(), tenant, source, tfports.EveryCurrentVersion, 10)
		if err != nil || len(rows) != 1 || rows[0].EffectiveJudgedBy != firstOperatorRecorded {
			t.Fatalf("查阅读面：%v %+v", err, rows)
		}
	})

	t.Run("carrier first effective pickup judgment", func(t *testing.T) {
		judge, pickups, err := buildCarrierPickupJudgment(db)
		if err != nil {
			t.Fatalf("装配承运商收寄判断：%v", err)
		}
		// 合成承运方没在 PC 身份册里：判断如实落成待确认；同一依据再来，身份仍未在册，链尾不动。
		body := `{"object":"SYN-PARCEL-OE-CP","source":"CARRIER_PICKUP_SCAN","evidenceReference":"SYN-SCAN-OE","evidenceVersion":"v1",` +
			`"expressesControl":true,"occurredAt":"2026-09-24T11:00:00+08:00","carrierKind":"EXTERNAL_PARTY","carrierReference":"SYN-PARTY/carrier-OE"}`
		decideTwice(t, tfhttp.NewJudgeCarrierFirstEffectivePickupEndpoint(decisions.transport, judge), "/transport-fulfillment-carrier-first-effective-pickup-judgments", body,
			[2]any{http.StatusOK, "PICKUP_PENDING"}, [2]any{http.StatusOK, "PICKUP_PENDING"})
		if got := recordedOperator(t, pool, `SELECT judged_by FROM transport_fulfillment.carrier_first_effective_pickup
			 WHERE tenant_id = 'SYN-TENANT-01' AND object_ref = 'SYN-PARCEL-OE-CP'`); got != firstOperatorRecorded {
			t.Fatalf("判断人 = %q，want %q", got, firstOperatorRecorded)
		}
		chain, err := pickups.ListByObject(t.Context(), mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-01"),
			mustValue(t, tfdomain.NewCarriedObjectReference, "SYN-PARCEL-OE-CP"))
		if err != nil || len(chain) != 1 || chain[0].JudgedBy.String() != firstOperatorRecorded {
			t.Fatalf("收寄链读回：%v %+v", err, chain)
		}
	})
}
