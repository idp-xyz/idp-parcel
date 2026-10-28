package main

import (
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
	restrictionMaker    = "SYN-OPERATOR-LW01-MAKER"
	restrictionDecider  = "SYN-OPERATOR-LW01-DECIDER"
	restrictionOutsider = "SYN-OPERATOR-LW01-OUTSIDER"
	restrictionScope    = "SYN-PARCEL-LW01"
)

const restrictionEstablishBody = `{"restrictionId":"SYN-IR-LW01-01","source":"SYN-SOURCE-TRADE-COMPLIANCE-01","basis":"SYN-RULE-VERSION-LW01",` +
	`"condition":"SYN-RELEASE-CONDITION-LW01","startedAt":"2026-10-20T00:00:00Z",` +
	`"coverage":[{"scope":"SYN-PARCEL-LW01","action":"OUTBOUND_RELEASE","standing":"IN_FORCE"},` +
	`{"scope":"SYN-PARCEL-LW01","action":"FINAL_DELIVERY","standing":"UNCONFIRMED"}]}`

const restrictionReleaseBody = `{"restrictionId":"SYN-IR-LW01-01","decisionId":"SYN-IRD-LW01-01","kind":"RELEASE",` +
	`"source":"SYN-SOURCE-TRADE-COMPLIANCE-01","covers":[{"scope":"SYN-PARCEL-LW01","action":"OUTBOUND_RELEASE"}],` +
	`"evidence":"SYN-EVIDENCE-CONDITION-MET-LW01","authority":"SYN-DELEGATION-LW01","decidedAt":"2026-10-25T00:00:00Z"}`

// Covers: UC-CC-011 的生产装配半边（真库，票 launch-wiring/01）——持「内部合规限制建立」授予的运营人员经 `/customs-internal-restrictions`
// 建出一份限制（201，重放答已有、库里仍是一份，租户与决定方取自认证信封），动作准入读到的适用性随之是`有效限制`；持「内部合规限制决定」
// 授予的人经 `/customs-internal-restriction-decisions` 由原责任来源解除其中一个覆盖项（201），解除之后的动作时点适用性放开、没解除的覆盖项
// 仍约束；来源不是责任来源答 NOT_RESPONSIBLE_SOURCE、不落决定；两种授予互不相通，未授予答 403；载荷自报租户答 400。
func TestInternalRestrictionsAreEstablishedAndReleasedThroughTheProductionFaces(t *testing.T) {
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
	registerOperator(t, db, operators, restrictionMaker, interval, accessidentity.DecisionInternalRestrictionEstablishment)
	registerOperator(t, db, operators, restrictionDecider, interval, accessidentity.DecisionInternalRestrictionDecision)
	registerOperator(t, db, operators, restrictionOutsider, interval, accessidentity.DecisionCustomsCaseEstablishment)
	minter, err := buildOperatorMinter(tokenSubjectVerifier{}, operators, admitAll{})
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	faces, err := buildInternalRestrictionFaces(db)
	if err != nil {
		t.Fatalf("装配：%v", err)
	}
	endpoints := map[string]http.Handler{}
	for _, feature := range endpointRegistry {
		if feature.wiringKey != any((*internalRestrictionsWiring)(nil)) {
			continue
		}
		for _, endpoint := range feature.mountWith(endpointChannels{operatorDecisions: decisions},
			internalRestrictionsWiring{establisher: faces, decider: faces}) {
			endpoints[endpoint.Pattern] = endpoint.Handler
		}
	}
	if len(endpoints) != 2 {
		t.Fatalf("内部合规限制两口没按登记处挂上：%v", endpoints)
	}
	post := func(pattern, operator, body string) (int, map[string]any) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, pattern, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+operator)
		response := httptest.NewRecorder()
		endpoints[pattern].ServeHTTP(response, request)
		var answer map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
			t.Fatalf("%s 答复不是 JSON：%s", pattern, response.Body.String())
		}
		return response.Code, answer
	}
	const establish, decide = "/customs-internal-restrictions", "/customs-internal-restriction-decisions"

	// 未授予、授予不相通、载荷自报租户。
	if status, answer := post(establish, restrictionOutsider, restrictionEstablishBody); status != http.StatusForbidden {
		t.Fatalf("未持建立授予 = %d %v，want 403", status, answer)
	}
	if status, answer := post(establish, restrictionDecider, restrictionEstablishBody); status != http.StatusForbidden {
		t.Fatalf("只持决定授予去建立 = %d %v，want 403", status, answer)
	}
	if status, answer := post(decide, restrictionMaker, restrictionReleaseBody); status != http.StatusForbidden {
		t.Fatalf("只持建立授予去解除 = %d %v，want 403", status, answer)
	}
	if status, _ := post(establish, restrictionMaker, strings.Replace(restrictionEstablishBody, `{"restrictionId"`, `{"tenantId":"SYN-TENANT-X","restrictionId"`, 1)); status != http.StatusBadRequest {
		t.Fatalf("载荷自报租户 = %d，want 400", status)
	}

	// 建立与重放。
	status, answer := post(establish, restrictionMaker, restrictionEstablishBody)
	if status != http.StatusCreated || answer["outcome"] != "ESTABLISHED" || answer["restrictionId"] != "SYN-IR-LW01-01" {
		t.Fatalf("建立 = %d %v", status, answer)
	}
	if status, answer := post(establish, restrictionMaker, restrictionEstablishBody); status != http.StatusOK || answer["outcome"] != "EXISTING" {
		t.Fatalf("重放建立 = %d %v，want 200 EXISTING", status, answer)
	}
	changed := strings.Replace(restrictionEstablishBody, "SYN-RULE-VERSION-LW01", "SYN-RULE-VERSION-OTHER", 1)
	if status, answer := post(establish, restrictionMaker, changed); status != http.StatusOK || answer["outcome"] != "CONTENT_CONFLICT" {
		t.Fatalf("同标识异内容 = %d %v，want 200 CONTENT_CONFLICT", status, answer)
	}
	var heads int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM customs_compliance.internal_compliance_restriction WHERE restriction_id = 'SYN-IR-LW01-01'`).Scan(&heads); err != nil || heads != 1 {
		t.Fatalf("限制头 = %d（%v），want 恰一份", heads, err)
	}

	store, err := ccpostgres.NewInternalRestrictions(db)
	if err != nil {
		t.Fatal(err)
	}
	applicability := func(action ccdomain.GuardedAction, at string) ccdomain.RestrictionApplicability {
		t.Helper()
		listed, err := store.ListInternalByScope(t.Context(),
			mustValue(t, ccdomain.NewTenantID, customsDecisionTenant), mustValue(t, ccdomain.NewDecisionScopeReference, restrictionScope))
		if err != nil {
			t.Fatal(err)
		}
		moment, err := time.Parse(time.RFC3339, at)
		if err != nil {
			t.Fatal(err)
		}
		judged, err := ccdomain.JudgeInternalApplicability(action, mustValue(t, ccdomain.NewDecisionScopeReference, restrictionScope), listed, moment)
		if err != nil {
			t.Fatal(err)
		}
		return judged.Applicability()
	}
	if got := applicability(ccdomain.OutboundRelease, "2026-10-26T00:00:00Z"); got != ccdomain.RestrictionInForce {
		t.Fatalf("建立之后交出的适用性 = %v，want 有效限制", got)
	}

	// 解除：来源不是责任来源不落；责任来源解除其中一个覆盖项。
	wrongSource := strings.Replace(restrictionReleaseBody, "SYN-SOURCE-TRADE-COMPLIANCE-01", "SYN-SOURCE-OTHER", 1)
	if status, answer := post(decide, restrictionDecider, wrongSource); status != http.StatusOK || answer["outcome"] != "NOT_RESPONSIBLE_SOURCE" {
		t.Fatalf("非责任来源解除 = %d %v，want 200 NOT_RESPONSIBLE_SOURCE", status, answer)
	}
	status, answer = post(decide, restrictionDecider, restrictionReleaseBody)
	if status != http.StatusCreated || answer["outcome"] != "DECIDED" {
		t.Fatalf("解除 = %d %v", status, answer)
	}
	if status, answer := post(decide, restrictionDecider, restrictionReleaseBody); status != http.StatusOK || answer["outcome"] != "DECISION_EXISTING" {
		t.Fatalf("重放解除 = %d %v，want 200 DECISION_EXISTING", status, answer)
	}
	noEvidence := strings.Replace(strings.Replace(restrictionReleaseBody, "SYN-EVIDENCE-CONDITION-MET-LW01", "", 1), "SYN-IRD-LW01-01", "SYN-IRD-LW01-02", 1)
	if status, answer := post(decide, restrictionDecider, noEvidence); status != http.StatusOK || answer["outcome"] != "DECISION_REJECTED" {
		t.Fatalf("没有证据的解除 = %d %v，want 200 DECISION_REJECTED", status, answer)
	}
	var decided int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM customs_compliance.internal_restriction_decision WHERE restriction_id = 'SYN-IR-LW01-01'`).Scan(&decided); err != nil || decided != 1 {
		t.Fatalf("决定 = %d（%v），want 恰一条（被拒与重放都不落）", decided, err)
	}
	if got := applicability(ccdomain.OutboundRelease, "2026-10-26T00:00:00Z"); got != ccdomain.RestrictionNoneRecorded {
		t.Fatalf("解除之后交出的适用性 = %v，want 册上无记录（放开）", got)
	}
	if got := applicability(ccdomain.OutboundRelease, "2026-10-22T00:00:00Z"); got != ccdomain.RestrictionInForce {
		t.Fatalf("解除之前的动作时点 = %v，want 仍读到有效限制（不回溯）", got)
	}
	if got := applicability(ccdomain.FinalDelivery, "2026-10-26T00:00:00Z"); got != ccdomain.RestrictionUnconfirmed {
		t.Fatalf("没解除的覆盖项 = %v，want 仍待确认", got)
	}
}
