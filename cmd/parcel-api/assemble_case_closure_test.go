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
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

const (
	closureCloser   = "SYN-OPERATOR-LW02-CLOSER"
	closureOutsider = "SYN-OPERATOR-LW02-OUTSIDER"
)

// Covers: UC-CC-010 关闭入口的生产装配半边（真库，票 launch-wiring/02）——持「关务案件关闭」授予的运营人员经 `/customs-case-closures` 关案：
// 义务目录没登记答未决并指名`OBLIGATION_INVENTORY_UNCONFIGURED`、不落关闭记录；有未解决项与冲突项（code/03）答被阻并列出各项、案件仍未关；
// 逐项终结或被有效承接之后答已关（201），关闭记录带操作者、关闭意图同笔入了 Outbox；重放答已关闭且不多入意图；未授予答 403，建案授予换不了关案。
func TestACustomsCaseIsClosedOrBlockedThroughTheProductionFace(t *testing.T) {
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
	registerOperator(t, db, operators, closureCloser, interval, accessidentity.DecisionCustomsCaseClosure)
	registerOperator(t, db, operators, closureOutsider, interval, accessidentity.DecisionCustomsCaseEstablishment)
	minter, err := buildOperatorMinter(tokenSubjectVerifier{}, operators, admitAll{})
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	closer, err := buildCaseClosure(db)
	if err != nil {
		t.Fatalf("装配：%v", err)
	}
	var endpoint http.Handler
	for _, feature := range endpointRegistry {
		if feature.wiringKey != any((*caseClosureWiring)(nil)) {
			continue
		}
		for _, mounted := range feature.mountWith(endpointChannels{operatorDecisions: decisions}, caseClosureWiring{closer: closer}) {
			endpoint = mounted.Handler
		}
	}
	if endpoint == nil {
		t.Fatal("关闭口没按登记处挂上")
	}
	post := func(operator, body string) (int, map[string]any) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/customs-case-closures", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+operator)
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, request)
		var answer map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
			t.Fatalf("答复不是 JSON：%s", response.Body.String())
		}
		return response.Code, answer
	}
	body := func(caseID string) string {
		return `{"caseId":"` + caseID + `","cutoffAt":"2026-10-26T00:00:00Z"}`
	}
	tenant := mustValue(t, ccdomain.NewTenantID, customsDecisionTenant)
	registrations, err := ccpostgres.NewObligationInventoryRegistrations(db)
	if err != nil {
		t.Fatal(err)
	}
	register := func(caseID string, items ...ccdomain.ClosureObligationItem) {
		t.Helper()
		caseRef := mustValue(t, ccdomain.NewCustomsCaseID, caseID)
		if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			if _, err := registrations.RegisterObligationCatalog(txCtx, tenant, caseRef, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)); err != nil {
				return err
			}
			for _, item := range items {
				if _, err := registrations.RegisterObligationItem(txCtx, tenant, caseRef, ccports.ObligationRegistration{
					Item: item, AppliesFrom: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
				}); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			t.Fatalf("登记义务盘点 %s：%v", caseID, err)
		}
	}

	// 授予：未授予与建案授予都换不了关案。
	if status, answer := post(closureOutsider, body("SYN-CASE-LW02-A")); status != http.StatusForbidden {
		t.Fatalf("只持建案授予关案 = %d %v，want 403", status, answer)
	}
	// 目录没登记：未决，不当成「没有义务所以可关」，也不落关闭记录。
	status, answer := post(closureCloser, body("SYN-CASE-LW02-A"))
	if status != http.StatusOK || answer["outcome"] != "UNDECIDED" || answer["undecidedReason"] != "OBLIGATION_INVENTORY_UNCONFIGURED" {
		t.Fatalf("目录未登记 = %d %v", status, answer)
	}
	// 一项未解决、一项冲突：被阻并列出各项。
	register("SYN-CASE-LW02-A",
		ccdomain.ClosureObligationItem{Obligation: "SYN-OBL-DUTY", Scope: "SYN-SCOPE-1", State: ccdomain.ObligationUnresolved, Basis: "SYN-BASIS-1"},
		ccdomain.ClosureObligationItem{Obligation: "SYN-OBL-DECL", Scope: "SYN-SCOPE-1", State: ccdomain.ObligationConflict, Basis: "SYN-BASIS-2",
			ConflictsWith: []string{"SYN-FACT-A", "SYN-FACT-B"}},
		ccdomain.ClosureObligationItem{Obligation: "SYN-OBL-DONE", Scope: "SYN-SCOPE-1", State: ccdomain.ObligationConcluded, Basis: "SYN-BASIS-3"},
	)
	status, answer = post(closureCloser, body("SYN-CASE-LW02-A"))
	blocking, _ := answer["blockingItems"].([]any)
	if status != http.StatusOK || answer["outcome"] != "BLOCKED" || len(blocking) != 2 {
		t.Fatalf("有未解决与冲突项 = %d %v，want 200 BLOCKED 列两项", status, answer)
	}
	if !strings.Contains(mustJSON(t, blocking), "SYN-FACT-A") {
		t.Fatalf("冲突项没带冲突依据：%v", blocking)
	}
	var closures int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM customs_compliance.case_closure WHERE case_ref = 'SYN-CASE-LW02-A'`).Scan(&closures); err != nil || closures != 0 {
		t.Fatalf("被阻的案件落了关闭记录：%d（%v）", closures, err)
	}

	// 全部终结或有效承接：已关；重放答已关闭，不多入意图。
	register("SYN-CASE-LW02-B",
		ccdomain.ClosureObligationItem{Obligation: "SYN-OBL-1", Scope: "SYN-SCOPE-2", State: ccdomain.ObligationConcluded, Basis: "SYN-BASIS-4"},
		ccdomain.ClosureObligationItem{Obligation: "SYN-OBL-2", Scope: "SYN-SCOPE-2", State: ccdomain.ObligationHandedOver, Basis: "SYN-BASIS-5", HandedTo: "SYN-RECEIVER-1"},
	)
	status, answer = post(closureCloser, body("SYN-CASE-LW02-B"))
	if status != http.StatusCreated || answer["outcome"] != "CLOSED" || answer["caseId"] != "SYN-CASE-LW02-B" || answer["decidedBy"] != customsDecisionIssuer+"#"+closureCloser {
		t.Fatalf("关案 = %d %v", status, answer)
	}
	if status, answer = post(closureCloser, body("SYN-CASE-LW02-B")); status != http.StatusOK || answer["outcome"] != "ALREADY_CLOSED" {
		t.Fatalf("重放 = %d %v，want 200 ALREADY_CLOSED", status, answer)
	}
	var intents int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type LIKE 'customs-compliance.case-closure%'`).Scan(&intents); err != nil || intents != 1 {
		t.Fatalf("关闭意图 = %d（%v），want 恰一份（重放不多入）", intents, err)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
