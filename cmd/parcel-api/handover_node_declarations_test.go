package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件在生产装配的交接编排、作业事实 Intake 与真库上证交接三格节点声明（ADR-0213；票 demo-chain-gaps/59）。
// 只有查阅口的操作者认证换成一个固定租户的替身——那一口不是本票的对象。输入都是合成串，证据只记 `S`。

const handoverNodesTenant = "SYN-TENANT-01"

func handoverNodesBody(t *testing.T, object string, extra map[string]string) string {
	t.Helper()
	fields := map[string]string{
		"object":            object,
		"scope":             "SYN-HANDOVER-SCOPE-DCG59",
		"releasedBy":        "SYN-PARTY-HUB-SHA",
		"receivedBy":        "SYN-PARTY-CARRIER-TRUNK-01",
		"verdict":           "HANDED_OVER",
		"releasingEvidence": "SYN-EVIDENCE-RELEASE-DCG59",
		"receivingEvidence": "SYN-EVIDENCE-RECEIVE-DCG59",
		"rule":              "SYN-HANDOVER-RULE/v1",
		"version":           "v1",
		"judgedAt":          "2026-10-21T20:00:00+08:00",
	}
	for key, value := range extra {
		if value == "" {
			delete(fields, key)
			continue
		}
		fields[key] = value
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	return string(encoded)
}

func postWorkFact(t *testing.T, endpoint http.Handler, path, body string) (int, map[string]string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	stampWorkFact(request, "OFFSITE", "SYN-OFFSITE-01", "SYN-DEVICE-OFFSITE")
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	var fields map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode %s：%v", response.Body.String(), err)
	}
	return response.Code, fields
}

// handoverNodesCatalogueIntake 是查阅口认证已就位的样子：租户固定为演示租户。
type handoverNodesCatalogueIntake struct{}

func (handoverNodesCatalogueIntake) IntakeCatalogueQuery(context.Context, *http.Request) (tfhttp.CatalogueQuery, error) {
	reference, err := tfdomain.NewOperationsScopeReference("SYN-OPS-SCOPE-01")
	if err != nil {
		return tfhttp.CatalogueQuery{}, err
	}
	tenant, err := tfdomain.NewTenantID(handoverNodesTenant)
	if err != nil {
		return tfhttp.CatalogueQuery{}, err
	}
	scope, err := tfdomain.NewOperationsQueryScope(reference, tenant)
	if err != nil {
		return tfhttp.CatalogueQuery{}, err
	}
	return tfhttp.CatalogueQuery{Scope: scope, Limit: 25}, nil
}

// Covers: AT-TF-217、AT-TF-218、AT-TF-219、AT-TF-220、AT-TF-221 真库半边，票 demo-chain-gaps/59 完成判据的演示一条——带
// 交出节点首登答 201 `HANDOVER_REGISTERED` 并回显；原样重发答 200 `EXISTING_VERSION`；改接收节点重发答 200
// `SOURCE_CONFLICT`、在册那一版不动；更正不带节点、新版本照前版承继；`registry=transport-handover` 列出两版的节点格。
func TestTheWiredHandoverRegistersRepliesAndListsDeclaredNodesAgainstARealDatabase(t *testing.T) {
	orchestrations, pool := wiredHandoverOrchestrations(t)
	intake := workFactTransportForTest(t)
	register := tfhttp.NewRegisterTransportHandoverEndpoint(intake, orchestrations.handover)
	correct := tfhttp.NewCorrectTransportHandoverEndpoint(intake, orchestrations.handover)
	declared := handoverNodesBody(t, "SYN-PARCEL-DCG59-01", map[string]string{"releasingNode": "SYN-NODE-SHA-HUB"})

	status, answer := postWorkFact(t, register, "/transport-fulfillment/handovers", declared)
	if status != http.StatusCreated || answer["outcome"] != "HANDOVER_REGISTERED" || answer["releasingNode"] != "SYN-NODE-SHA-HUB" {
		t.Fatalf("首登 HTTP %d %v", status, answer)
	}
	status, answer = postWorkFact(t, register, "/transport-fulfillment/handovers", declared)
	if status != http.StatusOK || answer["outcome"] != "EXISTING_VERSION" || answer["releasingNode"] != "SYN-NODE-SHA-HUB" {
		t.Fatalf("原样重发 HTTP %d %v", status, answer)
	}
	moved := handoverNodesBody(t, "SYN-PARCEL-DCG59-01", map[string]string{
		"releasingNode": "SYN-NODE-SHA-HUB", "receivingNode": "SYN-NODE-SZX-GATE",
	})
	status, answer = postWorkFact(t, register, "/transport-fulfillment/handovers", moved)
	if status != http.StatusOK || answer["outcome"] != "SOURCE_CONFLICT" {
		t.Fatalf("改接收节点重发 HTTP %d %v", status, answer)
	}

	var digest string
	var receiving *string
	if err := pool.QueryRow(t.Context(),
		`SELECT content_digest, receiving_node FROM transport_fulfillment.transport_handover
		  WHERE tenant_id = $1 AND object_ref = 'SYN-PARCEL-DCG59-01' AND handover_version = 'v1'`,
		handoverNodesTenant).Scan(&digest, &receiving); err != nil {
		t.Fatalf("读回 v1：%v", err)
	}
	if !strings.HasPrefix(digest, "TFC-2:") || receiving != nil {
		t.Fatalf("在册 v1 摘要 = %s 接收节点 = %v，want TFC-2 且冲突不顶替", digest, receiving)
	}

	correction := `{"object":"SYN-PARCEL-DCG59-01","scope":"SYN-HANDOVER-SCOPE-DCG59","predecessorVersion":"v1",` +
		`"verdict":"REFUSED","basis":"SYN-BASIS-DCG59-RECOUNT","newVersion":"v2","correctedAt":"2026-10-21T22:00:00+08:00"}`
	status, answer = postWorkFact(t, correct, "/transport-fulfillment/handover-corrections", correction)
	if status != http.StatusCreated || answer["outcome"] != "HANDOVER_CORRECTED" || answer["releasingNode"] != "SYN-NODE-SHA-HUB" {
		t.Fatalf("更正 HTTP %d %v，want 201 且承继交出节点", status, answer)
	}

	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	records, err := tfpostgres.NewReviewCatalogue(db)
	if err != nil {
		t.Fatalf("查阅目录：%v", err)
	}
	list := httptest.NewRecorder()
	tfhttp.NewQueryTransportFulfillmentRecordsEndpoint(handoverNodesCatalogueIntake{}, records).
		ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/transport-fulfillment-records?registry=transport-handover", nil))
	var listed struct {
		Outcome   string              `json:"outcome"`
		Handovers []map[string]string `json:"handovers"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil || list.Code != http.StatusOK {
		t.Fatalf("交接册 HTTP %d %s：%v", list.Code, list.Body.String(), err)
	}
	versions := map[string]bool{}
	for _, row := range listed.Handovers {
		if row["object"] != "SYN-PARCEL-DCG59-01" {
			continue
		}
		versions[row["version"]] = true
		if row["releasingNode"] != "SYN-NODE-SHA-HUB" {
			t.Fatalf("册面 %s 交出节点 = %q", row["version"], row["releasingNode"])
		}
		if _, present := row["receivingNode"]; present {
			t.Fatalf("册面 %s 出现没声明的接收节点：%v", row["version"], row)
		}
	}
	if !versions["v1"] || !versions["v2"] {
		t.Fatalf("交接册没列全两版：%s", list.Body.String())
	}
}

// Covers: AT-TF-216、AT-TF-219 真库半边——不声明节点的首登答复没有节点键、摘要仍是 TFC-1；同键原样重发答
// `EXISTING_VERSION`，同键补报节点答 `SOURCE_CONFLICT`，补报的声明不被当成重放吞掉。
func TestAWiredHandoverWithoutNodesKeepsItsShapeAndRefusesALateNodeAgainstARealDatabase(t *testing.T) {
	orchestrations, pool := wiredHandoverOrchestrations(t)
	register := tfhttp.NewRegisterTransportHandoverEndpoint(workFactTransportForTest(t), orchestrations.handover)
	plain := handoverNodesBody(t, "SYN-PARCEL-DCG59-02", nil)

	status, answer := postWorkFact(t, register, "/transport-fulfillment/handovers", plain)
	if status != http.StatusCreated || answer["outcome"] != "HANDOVER_REGISTERED" {
		t.Fatalf("首登 HTTP %d %v", status, answer)
	}
	for _, key := range []string{"releasingNode", "receivingNode", "onwardNode"} {
		if _, present := answer[key]; present {
			t.Fatalf("不声明节点的首登答复里出现 %s：%v", key, answer)
		}
	}
	var digest string
	if err := pool.QueryRow(t.Context(),
		`SELECT content_digest FROM transport_fulfillment.transport_handover
		  WHERE tenant_id = $1 AND object_ref = 'SYN-PARCEL-DCG59-02'`, handoverNodesTenant).Scan(&digest); err != nil {
		t.Fatalf("读回：%v", err)
	}
	if !strings.HasPrefix(digest, "TFC-1:") {
		t.Fatalf("不声明节点的首登摘要 = %s，want TFC-1", digest)
	}
	if status, answer = postWorkFact(t, register, "/transport-fulfillment/handovers", plain); status != http.StatusOK || answer["outcome"] != "EXISTING_VERSION" {
		t.Fatalf("原样重发 HTTP %d %v", status, answer)
	}
	late := handoverNodesBody(t, "SYN-PARCEL-DCG59-02", map[string]string{"onwardNode": "SYN-NODE-SIN-HUB"})
	if status, answer = postWorkFact(t, register, "/transport-fulfillment/handovers", late); status != http.StatusOK || answer["outcome"] != "SOURCE_CONFLICT" {
		t.Fatalf("补报节点 HTTP %d %v，want SOURCE_CONFLICT", status, answer)
	}
}
