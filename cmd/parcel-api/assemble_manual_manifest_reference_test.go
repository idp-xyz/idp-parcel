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

// Covers: dox/decisions.md「产品形态」——外部舱单引用的手工首登与版本推进在真库上经生产装配走一遍：持「录入外部舱单
// 引用」授予的操作员照承运商的材料首登一版（没有候选对象，保持待关联入册）、再推进一版；两版各有一条留痕；原样重录答
// 已在册、不多一条留痕；同一版换了范围答内容冲突；对不在册的舱单推进答不在册；持别的授予的人答未授予。只记 `S`。
func TestManuallyEnteredManifestReferencesRunThroughTheProductionAssembly(t *testing.T) {
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
	const clerk, outsider = "SYN-MANIFEST-CLERK", "SYN-MANIFEST-OUTSIDER"
	registerOperatorIn(t, db, operators, customsDecisionTenant, clerk, interval, accessidentity.DecisionExternalManifestEntry)
	registerOperatorIn(t, db, operators, customsDecisionTenant, outsider, interval, accessidentity.DecisionCustomsCaseClosure)
	minter, err := buildOperatorMinter(tokenSubjectVerifier{}, operators, admitAll{})
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	const receiveFace, reviseFace = "/customs-external-manifest-reference-manual-registrations", "/customs-external-manifest-reference-revision-manual-registrations"
	endpoints := map[string]http.Handler{}
	for _, feature := range endpointRegistry {
		if feature.wiringKey != any((*manualExternalFactsWiring)(nil)) {
			continue
		}
		mounted, err := feature.production(endpointSources{db: db}, endpointChannels{operatorDecisions: decisions})
		if err != nil {
			t.Fatalf("装配：%v", err)
		}
		for _, candidate := range mounted {
			endpoints[candidate.Pattern] = candidate.Handler
		}
	}
	if endpoints[receiveFace] == nil || endpoints[reviseFace] == nil {
		t.Fatal("手工录入两口没按登记处挂上")
	}
	post := func(face, operator, body string) (int, map[string]any) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, face, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+operator)
		response := httptest.NewRecorder()
		endpoints[face].ServeHTTP(response, request)
		var answer map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
			t.Fatalf("答复不是 JSON：%s", response.Body.String())
		}
		return response.Code, answer
	}
	first := `{"manifestId":"SYN-MANIFEST-MANUAL-1","version":"v1","procedure":"SYN-PROC-CN-EXPORT","direction":"EXPORT","scope":"SYN-MOVEMENT-1","carrier":"SYN-CARRIER-X","original":"SYN-CARRIER-MANIFEST-MAIL-1"}`

	if status, answer := post(receiveFace, outsider, first); status != http.StatusForbidden {
		t.Fatalf("持别的授予的人录入 = %d %v，想要 403", status, answer)
	}
	status, answer := post(receiveFace, clerk, first)
	if status != http.StatusCreated || answer["outcome"] != "PENDING_ASSOCIATION" {
		t.Fatalf("人工首登 = %d %v", status, answer)
	}
	if status, answer = post(receiveFace, clerk, first); status != http.StatusOK || answer["outcome"] != "EXISTING" {
		t.Fatalf("原样重录 = %d %v", status, answer)
	}
	if status, answer = post(receiveFace, clerk, strings.Replace(first, "SYN-MOVEMENT-1", "SYN-MOVEMENT-OTHER", 1)); status != http.StatusOK || answer["outcome"] != "CONTENT_CONFLICT" {
		t.Fatalf("同一版换了范围 = %d %v", status, answer)
	}
	if status, answer = post(reviseFace, clerk, `{"manifestId":"SYN-MANIFEST-NOWHERE","version":"v2","scope":"SYN-MOVEMENT-2","carrier":"SYN-CARRIER-X","original":"SYN-CARRIER-MANIFEST-MAIL-2"}`); status != http.StatusOK ||
		answer["outcome"] != "NOT_FOUND" {
		t.Fatalf("对不在册的舱单推进 = %d %v", status, answer)
	}
	if status, answer = post(reviseFace, clerk, `{"manifestId":"SYN-MANIFEST-MANUAL-1","version":"v2","scope":"SYN-MOVEMENT-2","carrier":"SYN-CARRIER-X","original":"SYN-CARRIER-MANIFEST-MAIL-2"}`); status != http.StatusCreated ||
		answer["outcome"] != "REVISED" {
		t.Fatalf("人工版本推进 = %d %v", status, answer)
	}

	// 同一版再推进而范围不同：内容冲突，不答已在册。推回先前录过的版本号是又一次录入，不撞留痕的键。
	if status, answer = post(reviseFace, clerk, `{"manifestId":"SYN-MANIFEST-MANUAL-1","version":"v2","scope":"SYN-MOVEMENT-OTHER","carrier":"SYN-CARRIER-X","original":"SYN-CARRIER-MANIFEST-MAIL-2"}`); status != http.StatusOK ||
		answer["outcome"] != "CONTENT_CONFLICT" {
		t.Fatalf("同一版换了范围再推进 = %d %v", status, answer)
	}
	if status, answer = post(reviseFace, clerk, `{"manifestId":"SYN-MANIFEST-MANUAL-1","version":"v1","scope":"SYN-MOVEMENT-1","carrier":" SYN-CARRIER-X ","original":"SYN-CARRIER-MANIFEST-MAIL-3"}`); status != http.StatusCreated ||
		answer["outcome"] != "REVISED" {
		t.Fatalf("推回先前的版本号 = %d %v", status, answer)
	}
	if status, answer = post(reviseFace, clerk, `{"manifestId":"SYN-MANIFEST-MANUAL-1","version":"v2","scope":"SYN-MOVEMENT-2","carrier":"SYN-CARRIER-X","original":"SYN-CARRIER-MANIFEST-MAIL-2"}`); status != http.StatusCreated ||
		answer["outcome"] != "REVISED" {
		t.Fatalf("再推进到 v2 = %d %v", status, answer)
	}

	rows, err := pool.Query(t.Context(),
		`SELECT version_id || '|' || entered_by || '|' || original_ref FROM customs_compliance.external_manifest_manual_entry
		  WHERE tenant_id = $1 AND carrier_ref = 'SYN-CARRIER-X' AND manifest_id = 'SYN-MANIFEST-MANUAL-1' ORDER BY entered_at`, customsDecisionTenant)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var trail []string
	for rows.Next() {
		var row string
		if err := rows.Scan(&row); err != nil {
			t.Fatal(err)
		}
		trail = append(trail, row)
	}
	who := "OPERATOR/" + customsDecisionIssuer + "#" + clerk
	if len(trail) != 4 || trail[0] != "v1|"+who+"|SYN-CARRIER-MANIFEST-MAIL-1" || trail[1] != "v2|"+who+"|SYN-CARRIER-MANIFEST-MAIL-2" ||
		trail[2] != "v1|"+who+"|SYN-CARRIER-MANIFEST-MAIL-3" || trail[3] != "v2|"+who+"|SYN-CARRIER-MANIFEST-MAIL-2" {
		t.Fatalf("留痕 = %v", trail)
	}
	var references int
	var current string
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*), min(version_id) FROM customs_compliance.external_manifest_reference WHERE tenant_id = $1 AND manifest_id = 'SYN-MANIFEST-MANUAL-1'`,
		customsDecisionTenant).Scan(&references, &current); err != nil {
		t.Fatal(err)
	}
	if references != 1 || current != "v2" {
		t.Fatalf("引用 %d 条、当前版本 %q，想要 1 条 v2", references, current)
	}
}
