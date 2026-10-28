package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

const (
	gateVerifier = "SYN-OPERATOR-LW03-VERIFIER"
	gateOutsider = "SYN-OPERATOR-LW03-OUTSIDER"
)

// Covers: UC-CC-009 门禁核对的生产装配半边（真库，票 launch-wiring/03，code/34）——持「放行门禁核对」授予的运营人员经 `/customs-release-gate-verifications`
// 请求核对：门禁目录没登记答未决并指名`GATE_CATALOG_NOT_CONFIGURED`、「税费付款」规则没登记答`DUTY_PAYMENT_GATE_RULE_NOT_CONFIGURED`，两者都不落册、不发意图；
// 目录与规则（不构成前置条件）齐了、逐项认定全满足答 201 `RECORDED` 结论 `MET`，核对记录与核对意图同笔入库；重放答 `EXISTING_RESULT` 且不多入意图；
// 动作不在封闭集合答 400；只持建案授予答 403。
func TestAReleaseGateVerificationIsRecordedOrLeftUndecidedThroughTheProductionFace(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	operators, err := aipg.NewOperatorRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	registerOperator(t, db, operators, gateVerifier, interval, accessidentity.DecisionReleaseGateVerification)
	registerOperator(t, db, operators, gateOutsider, interval, accessidentity.DecisionCustomsCaseEstablishment)
	minter, err := buildOperatorMinter(tokenSubjectVerifier{}, operators, admitAll{})
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := buildReleaseGateVerification(db)
	if err != nil {
		t.Fatalf("装配：%v", err)
	}
	var endpoint http.Handler
	for _, feature := range endpointRegistry {
		if feature.wiringKey != any((*releaseGateVerificationWiring)(nil)) {
			continue
		}
		for _, mounted := range feature.mountWith(endpointChannels{operatorDecisions: decisions}, releaseGateVerificationWiring{verifier: verifier}) {
			endpoint = mounted.Handler
		}
	}
	if endpoint == nil {
		t.Fatal("核对口没按登记处挂上")
	}
	post := func(operator, body string) (int, map[string]any) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/customs-release-gate-verifications", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+operator)
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, request)
		var answer map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
			t.Fatalf("答复不是 JSON：%s", response.Body.String())
		}
		return response.Code, answer
	}
	const body = `{"scope":"SYN-SCOPE-LW03","action":"CROSS_CUSTOMS_MOVEMENT","boundary":"SYN-PROC-CN-EXPORT"}`
	count := func(query string) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(t.Context(), query).Scan(&n); err != nil {
			t.Fatalf("%s：%v", query, err)
		}
		return n
	}
	const records = `SELECT count(*) FROM customs_compliance.gate_verification WHERE scope_ref = 'SYN-SCOPE-LW03'`
	const intents = `SELECT count(*) FROM ` + migrate.SchemaBento + `.outbox WHERE event_type = 'customs-compliance.gate-verification.recorded'`
	intentsBefore := count(intents)

	if status, answer := post(gateOutsider, body); status != http.StatusForbidden {
		t.Fatalf("只持建案授予核对门禁 = %d %v，want 403", status, answer)
	}
	if status, _ := post(gateVerifier, strings.Replace(body, "CROSS_CUSTOMS_MOVEMENT", "RECEIVE", 1)); status != http.StatusBadRequest {
		t.Fatalf("动作不在封闭集合 = %d，want 400", status)
	}
	status, answer := post(gateVerifier, body)
	if status != http.StatusOK || answer["outcome"] != "UNDECIDED" || answer["undecidedReason"] != "GATE_CATALOG_NOT_CONFIGURED" {
		t.Fatalf("目录未登记 = %d %v", status, answer)
	}

	// 目录与逐项认定登了，「税费付款」规则还没登：停在规则未配置，不取任何默认折法。
	registrations, err := ccpostgres.NewGateConditionRegistrations(db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := mustValue(t, ccdomain.NewTenantID, customsDecisionTenant)
	scope := mustValue(t, ccdomain.NewDecisionScopeReference, "SYN-SCOPE-LW03")
	boundary := mustValue(t, ccdomain.NewCustomsProcedureReference, "SYN-PROC-CN-EXPORT")
	inTransaction := func(step func(context.Context) error) {
		t.Helper()
		if err := db.Transactor().WithinTransaction(t.Context(), step); err != nil {
			t.Fatal(err)
		}
	}
	inTransaction(func(txCtx context.Context) error {
		if _, err := registrations.RegisterGateCatalog(txCtx, tenant, scope, ccdomain.CrossCustomsMovement, boundary, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)); err != nil {
			return err
		}
		_, err := registrations.RegisterGateFinding(txCtx, tenant, scope, ccdomain.CrossCustomsMovement, boundary, ccdomain.PreconditionFinding{
			Precondition: mustValue(t, ccdomain.NewPreconditionReference, "SYN-PRE-EXPORT-DECL-RELEASED"), State: ccdomain.PreconditionMet,
		})
		return err
	})
	status, answer = post(gateVerifier, body)
	if status != http.StatusOK || answer["outcome"] != "UNDECIDED" || answer["undecidedReason"] != "DUTY_PAYMENT_GATE_RULE_NOT_CONFIGURED" {
		t.Fatalf("规则未登记 = %d %v", status, answer)
	}
	if n := count(records); n != 0 {
		t.Fatalf("未决落了核对记录：%d", n)
	}
	if n := count(intents); n != intentsBefore {
		t.Fatalf("未决发了核对意图：%d → %d", intentsBefore, n)
	}

	// 规则登成「不构成前置条件」：认定全满足，结论 MET，核对与意图同笔落地。
	inTransaction(func(txCtx context.Context) error {
		_, err := registrations.RegisterDutyPaymentGateRule(txCtx, tenant, scope, ccdomain.CrossCustomsMovement, boundary, ccdomain.DutyPaymentNotAPrecondition())
		return err
	})
	status, answer = post(gateVerifier, body)
	if status != http.StatusCreated || answer["outcome"] != "RECORDED" || answer["conclusion"] != "MET" || answer["handoffReference"] != nil {
		t.Fatalf("门禁满足 = %d %v", status, answer)
	}
	if status, answer = post(gateVerifier, body); status != http.StatusOK || answer["outcome"] != "EXISTING_RESULT" || answer["conclusion"] != "MET" {
		t.Fatalf("重放 = %d %v，want 200 EXISTING_RESULT", status, answer)
	}
	if n := count(records); n != 1 {
		t.Fatalf("核对记录 = %d，want 1", n)
	}
	if n := count(intents); n != intentsBefore+1 {
		t.Fatalf("核对意图 %d → %d，want 恰多一份（重放不多入）", intentsBefore, n)
	}
}
