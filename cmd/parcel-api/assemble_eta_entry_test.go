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

// Covers: dox/decisions.md「产品形态」——预计到达的手工录入口在真库上经生产装配走一遍（ETA 形成的第一个生产入口）：
// 操作员录入承运商告知的预计到达，形成一版并交出意图，另有一条留痕；同一份内容换一种时间写法重录答已有结果、不多
// 一版；改了范围换版并指回前版；运营自己估的是另一条线，各有各的当前版；持别的授予的人答未授予。只记 `S`。
func TestAManuallyEnteredETARunsThroughTheProductionAssembly(t *testing.T) {
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
	const clerk, outsider = "SYN-ETA-CLERK", "SYN-ETA-OUTSIDER"
	registerOperatorIn(t, db, operators, customsDecisionTenant, clerk, interval, accessidentity.DecisionETAEntry)
	registerOperatorIn(t, db, operators, customsDecisionTenant, outsider, interval, accessidentity.DecisionEvidenceSubmission)
	minter, err := buildOperatorMinter(tokenSubjectVerifier{}, operators, admitAll{})
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	const face = "/eta-prediction-manual-entries"
	var endpoint http.Handler
	for _, feature := range endpointRegistry {
		if feature.wiringKey != any((*etaEntryWiring)(nil)) {
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
	entry := func(source, from, to, original string) string {
		return `{"parcel":"SYN-PARCEL-ETA-1","milestone":"DELIVERED","source":"` + source + `","rangeFrom":"` + from + `","rangeTo":"` + to +
			`","confidence":"HIGH","original":"` + original + `"}`
	}
	count := func(query string) (n int) {
		t.Helper()
		if err := pool.QueryRow(t.Context(), query, customsDecisionTenant).Scan(&n); err != nil {
			t.Fatalf("%s：%v", query, err)
		}
		return n
	}
	const versions = `SELECT count(*) FROM visibility_exception.eta_prediction_version WHERE tenant_id = $1`
	const trail = `SELECT count(*) FROM visibility_exception.eta_manual_entry WHERE tenant_id = $1 AND entered_by = 'OPERATOR/` + customsDecisionIssuer + `#` + clerk + `'`

	if status, answer := post(outsider, entry("CARRIER_PROVIDED", "2026-11-20T08:00:00Z", "2026-11-20T18:00:00Z", "SYN-CARRIER-ETA-MAIL-1")); status != http.StatusForbidden {
		t.Fatalf("持别的授予的人录入 = %d %v，想要 403", status, answer)
	}
	status, answer := post(clerk, entry("CARRIER_PROVIDED", "2026-11-20T08:00:00Z", "2026-11-20T18:00:00Z", "SYN-CARRIER-ETA-MAIL-1"))
	first, _ := answer["version"].(string)
	if status != http.StatusCreated || answer["outcome"] != "ETA_FORMED" || first == "" || answer["source"] != "CARRIER_PROVIDED" || count(versions) != 1 || count(trail) != 1 {
		t.Fatalf("首次录入 = %d %v；预测 %d 版、留痕 %d 条", status, answer, count(versions), count(trail))
	}
	if status, answer = post(clerk, entry("CARRIER_PROVIDED", "2026-11-20T16:00:00+08:00", "2026-11-21T02:00:00+08:00", "SYN-CARRIER-ETA-MAIL-1")); status != http.StatusOK ||
		answer["outcome"] != "ETA_EXISTING_RESULT" || answer["version"] != first || count(versions) != 1 || count(trail) != 1 {
		t.Fatalf("换一种时间写法重录 = %d %v；预测 %d 版", status, answer, count(versions))
	}
	status, answer = post(clerk, entry("CARRIER_PROVIDED", "2026-11-21T08:00:00Z", "2026-11-21T18:00:00Z", "SYN-CARRIER-ETA-MAIL-2"))
	if status != http.StatusCreated || answer["outcome"] != "ETA_REFRESHED" || answer["priorVersion"] != first || count(versions) != 2 || count(trail) != 2 {
		t.Fatalf("改了范围 = %d %v；预测 %d 版、留痕 %d 条", status, answer, count(versions), count(trail))
	}
	second, _ := answer["version"].(string)
	// 把内容改回第一份：不多一版、不移当前版，如实说那一版已被接替，并交回此刻的当前版。
	if status, answer = post(clerk, entry("CARRIER_PROVIDED", "2026-11-20T08:00:00Z", "2026-11-20T18:00:00Z", "SYN-CARRIER-ETA-MAIL-1")); status != http.StatusOK ||
		answer["outcome"] != "ETA_EXISTING_SUPERSEDED" || answer["version"] != second || count(versions) != 2 {
		t.Fatalf("改回第一份内容 = %d %v；预测 %d 版", status, answer, count(versions))
	}
	// 只有一个时间点：坏报文，不是 500。
	if status, answer = post(clerk, entry("CARRIER_PROVIDED", "2026-11-23T08:00:00Z", "2026-11-23T08:00:00Z", "SYN-CARRIER-ETA-MAIL-3")); status != http.StatusBadRequest {
		t.Fatalf("只有一个时间点 = %d %v，想要 400", status, answer)
	}
	if status, answer = post(clerk, entry("OPERATOR_DERIVED", "2026-11-22T08:00:00Z", "2026-11-22T18:00:00Z", "SYN-OPS-ESTIMATE-1")); status != http.StatusCreated ||
		answer["outcome"] != "ETA_FORMED" || count(versions) != 3 {
		t.Fatalf("运营自己估的另一条线 = %d %v；预测 %d 版", status, answer, count(versions))
	}
	var handoffs int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = 'visibility-exception.eta-prediction.formed'`).Scan(&handoffs); err != nil {
		t.Fatalf("数意图：%v", err)
	}
	if handoffs != 3 {
		t.Fatalf("交给客户视图链的意图 %d 份，想要每一版一份共 3", handoffs)
	}
}
