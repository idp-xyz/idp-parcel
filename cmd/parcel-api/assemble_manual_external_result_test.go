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
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: dox/decisions.md「产品形态」——海关外部结果的手工录入口在真库上经生产装配走一遍：持「人工录入海关结果」授予的
// 关务人员照回执录入，进的是集成客户端口后面的同一只接收编排（这里用一条归属不上的结果：留存不猜、答 UNATTRIBUTABLE），
// 行上记下录入人与所凭原件；同一份重投答已有记录、不落第二行；持别的授予的人答未授予；库拒绝只有录入人没有原件的行。
// 合成租户与标识只记 `S`。
func TestAManuallyEnteredCustomsResultIsReceivedByTheSameOrchestrationAndKeepsItsProvenance(t *testing.T) {
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
	const clerk, outsider = "SYN-CUSTOMS-CLERK-MANUAL", "SYN-CUSTOMS-OUTSIDER-MANUAL"
	registerOperatorIn(t, db, operators, customsDecisionTenant, clerk, interval, accessidentity.DecisionCustomsExternalResultEntry)
	registerOperatorIn(t, db, operators, customsDecisionTenant, outsider, interval, accessidentity.DecisionCustomsCaseClosure)
	minter, err := buildOperatorMinter(tokenSubjectVerifier{}, operators, admitAll{})
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	const face = "/customs-external-result-manual-registrations"
	var endpoint http.Handler
	for _, feature := range endpointRegistry {
		if feature.wiringKey != any((*manualExternalFactsWiring)(nil)) {
			continue
		}
		mounted, err := feature.production(endpointSources{db: db}, endpointChannels{operatorDecisions: decisions})
		if err != nil {
			t.Fatalf("装配：%v", err)
		}
		for _, candidate := range mounted {
			if candidate.Pattern == face {
				endpoint = candidate.Handler
			}
		}
	}
	if endpoint == nil {
		t.Fatalf("%s 没按登记处挂上", face)
	}
	post := func(operator, body string) (int, map[string]any) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, face, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+operator)
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, request)
		var answer map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
			t.Fatalf("答复不是 JSON：%s", response.Body.String())
		}
		return response.Code, answer
	}
	body := `{"sourceId":"SYN-RESULT-MANUAL-1","layer":"PROCESS_DECISION","role":"SYN-AUTHORITY-1","rawSemantics":"SYN-RAW-RELEASED",` +
		`"claimedVersion":"SYN-CC-VERSION-MISSING","attempt":1,"scope":"SYN-CC-UNIT-1","occurredAt":"` + time.Now().UTC().Add(-time.Hour).Format(time.RFC3339) +
		`","original":"SYN-RECEIPT-NO-20261113-01"}`

	if status, answer := post(outsider, body); status != http.StatusForbidden {
		t.Fatalf("持别的授予的人录入 = %d %v，想要 403", status, answer)
	}
	if status, answer := post(clerk, body); status != http.StatusOK || answer["outcome"] != "UNATTRIBUTABLE" {
		t.Fatalf("手工录入 = %d %v，想要留存不猜", status, answer)
	}
	if status, answer := post(clerk, body); status != http.StatusOK || answer["outcome"] != "EXISTING_RESULT" {
		t.Fatalf("同一份重投 = %d %v", status, answer)
	}
	var rows int
	var enteredBy, original string
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*), min(entered_by), min(original_ref) FROM customs_compliance.external_result WHERE tenant_id = $1 AND source_id = 'SYN-RESULT-MANUAL-1'`,
		customsDecisionTenant).Scan(&rows, &enteredBy, &original); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || enteredBy != "OPERATOR/"+customsDecisionIssuer+"#"+clerk || original != "SYN-RECEIPT-NO-20261113-01" {
		t.Fatalf("结果行 %d 行，录入人 %q，原件 %q", rows, enteredBy, original)
	}
	if _, err := pool.Exec(t.Context(),
		`UPDATE customs_compliance.external_result SET original_ref = NULL WHERE tenant_id = $1 AND source_id = 'SYN-RESULT-MANUAL-1'`,
		customsDecisionTenant); err == nil {
		t.Fatal("库接受了有录入人没有原件的结果行")
	}
}
