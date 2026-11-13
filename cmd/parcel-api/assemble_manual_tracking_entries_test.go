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
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// Covers: dox/decisions.md「产品形态」——承运商轨迹的手工录入口在真库上经生产装配走一遍。先把承运商运单号登记成外部
// 承运凭证；一批三条：两条凭证认得出的认领成事实（没有登记有效时间规则，停在待判断）、一条凭证不认识的如实留痕；
// 每一版事实各有一条人工录入留痕（录入人与原件）；整批重投全部认成重复，不落第二版、不记第二条留痕；持别的授予的人
// 答未授予。合成租户与标识只记 `S`。
func TestManuallyEnteredTrackingIsAdoptedThroughTheProductionAssemblyAndKeepsItsEntryTrail(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	const tenant = customsDecisionTenant
	registrar, err := buildExternalCarrierCredentialRegistration(db)
	if err != nil {
		t.Fatal(err)
	}
	if registered, err := registrar.Register(t.Context(), tfapp.RegisterExternalCarrierCredentialCommand{
		TenantID: mustValue(t, tfdomain.NewTenantID, tenant), Credential: "SYN-CARRIER-X/1Z-000001", Version: "SYN-ECV-000000000001",
		Assigner: "SYN-CARRIER-X", IdentifiedKind: "CARRIED_OBJECT", IdentifiedRef: "SYN-PARCEL-TRK-1",
		EffectiveFrom: time.Now().UTC().Add(-24 * time.Hour),
	}); err != nil || registered.Outcome() != tfapp.CredentialRegistered {
		t.Fatalf("登记外部承运凭证：err=%v outcome=%v", err, registered.Outcome())
	}

	operators, err := aipg.NewOperatorRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	const clerk, outsider = "SYN-TRACKING-CLERK", "SYN-TRACKING-OUTSIDER"
	registerOperatorIn(t, db, operators, tenant, clerk, interval, accessidentity.DecisionExternalTrackingEntry)
	registerOperatorIn(t, db, operators, tenant, outsider, interval, accessidentity.DecisionEffectiveTimeJudgment)
	minter, err := buildOperatorMinter(tokenSubjectVerifier{}, operators, admitAll{})
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	const face = "/transport-fulfillment-external-tracking-manual-entries"
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
	type answer struct {
		Outcome string
		Counts  map[string]int
		Entries []struct {
			Outcome, Reason, Fact, Version, Object, EffectiveBasis, EventID string
		}
	}
	post := func(operator, body string) (int, answer) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, face, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+operator)
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, request)
		var got answer
		if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
			t.Fatalf("答复不是 JSON：%s", response.Body.String())
		}
		return response.Code, got
	}
	occurred := time.Now().UTC().Add(-3 * time.Hour).Format(time.RFC3339)
	body := `{"entries":[` +
		`{"source":"SYN-CARRIER-X","credential":"SYN-CARRIER-X/1Z-000001","eventId":"EVT-1","occurredAt":"` + occurred + `","status":"IN_TRANSIT","original":"SYN-CARRIER-MAIL-1"},` +
		`{"source":"SYN-CARRIER-X","credential":"SYN-CARRIER-X/1Z-000001","occurredAt":"` + occurred + `","status":"ARRIVED_AT_HUB","original":"SYN-CARRIER-MAIL-1"},` +
		`{"source":"SYN-CARRIER-X","credential":"SYN-CARRIER-X/1Z-UNKNOWN","occurredAt":"` + occurred + `","status":"IN_TRANSIT","original":"SYN-CARRIER-MAIL-1"}]}`

	if status, _ := post(outsider, body); status != http.StatusForbidden {
		t.Fatalf("持别的授予的人录入 = %d，想要 403", status)
	}
	status, got := post(clerk, body)
	if status != http.StatusCreated || got.Outcome != "ENTRIES_PROCESSED" || got.Counts["TRACKING_FACT_ADOPTED_PENDING_JUDGMENT"] != 2 || got.Counts["MATERIAL_LEFT_UNADOPTED"] != 1 {
		t.Fatalf("首次导入 = %d %+v", status, got)
	}
	if first := got.Entries[0]; first.Object != "SYN-PARCEL-TRK-1" || first.EventID != "TRKMANUAL/EVT-1" || first.Fact == "" || first.EffectiveBasis != "PENDING" {
		t.Fatalf("第一条 = %+v", first)
	}
	if third := got.Entries[2]; third.Reason != "CREDENTIAL_UNKNOWN" || third.Fact != "" {
		t.Fatalf("凭证不认识的那一条 = %+v", third)
	}
	count := func(query string) (n int) {
		t.Helper()
		if err := pool.QueryRow(t.Context(), query, tenant).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	const facts = `SELECT count(*) FROM transport_fulfillment.external_carrier_tracking_fact WHERE tenant_id = $1`
	const trail = `SELECT count(*) FROM transport_fulfillment.external_tracking_manual_entry
	                WHERE tenant_id = $1 AND original_ref = 'SYN-CARRIER-MAIL-1' AND entered_by = 'OPERATOR/` + customsDecisionIssuer + `#` + clerk + `'`
	if count(facts) != 2 || count(trail) != 2 {
		t.Fatalf("事实 %d 版、人工录入留痕 %d 条，想要各 2", count(facts), count(trail))
	}

	status, got = post(clerk, body)
	if status != http.StatusOK || got.Counts["DUPLICATE_DELIVERY"] != 2 || count(facts) != 2 || count(trail) != 2 {
		t.Fatalf("整批重投 = %d %+v；事实 %d 版、留痕 %d 条", status, got.Counts, count(facts), count(trail))
	}
}
