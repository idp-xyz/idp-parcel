package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

// 本文件在生产端点构造与生产准入桥上证票 tf-face-algebra/10：建派送任务口的 201 首答与 `DISPATCH_TASK_ALREADY_OPEN`
// 都回显在册任务的 conditions。票 20 给本口裁「已在册交回原任务、差异调用方自己比」，所凭的就是这份回显——缺了
// conditions，同一任务换 conditions 重投时调用方比不出来。测试输入是隔离合成，只记 `S`。

type dispatchTaskEchoView struct {
	Outcome    string `json:"outcome"`
	Task       string `json:"task"`
	Conditions string `json:"conditions"`
}

// Covers: 以 conditions=A 建任务答 201 并回显 A；同一任务以 conditions=B 重投答 200 `DISPATCH_TASK_ALREADY_OPEN`，回显的仍是
// 在册的 A；库里仍只有那一行、conditions_ref 仍是 A——重投不改写工作范围。
func TestTheDispatchTaskFaceEchoesTheRegisteredConditionsOnAReplay(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registerOperationDecisionAdmission(t, db, isolatedSelfAuthority, accessidentity.DecisionDispatchTaskRegistration)
	admission, err := buildAdmissionScope(db, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	minter, err := buildOperatorMinter(acceptingVerifier{},
		decisionRegister{kinds: []accessidentity.DecisionKind{accessidentity.DecisionDispatchTaskRegistration}}, admission)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	operations, err := buildSegmentOperations(db)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := tfhttp.NewOpenDispatchTaskEndpoint(decisions.transport, operations.opener)

	const task = "SYN-DISPATCH-COND-ECHO"
	post := func(conditions string) (int, dispatchTaskEchoView, string) {
		t.Helper()
		body := fmt.Sprintf(`{"task":%q,"kind":"DELIVERY","objects":["SYN-PARCEL-COND-1"],"place":"SYN-PLACE/consignee-01",`+
			`"windowFrom":"2026-10-19T09:00:00Z","windowTo":"2026-10-19T12:00:00Z","conditions":%q,"openedAt":"2026-10-18T20:00:00Z"}`,
			task, conditions)
		request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment-dispatch-task-registrations", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer presented.operator.token")
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, request)
		var view dispatchTaskEchoView
		_ = json.Unmarshal(response.Body.Bytes(), &view)
		return response.Code, view, response.Body.String()
	}

	code, view, raw := post("SYN-CONDITION/A")
	if code != http.StatusCreated || view.Outcome != "DISPATCH_TASK_OPENED" || view.Conditions != "SYN-CONDITION/A" {
		t.Fatalf("以 A 建任务：%d %s, want 201 DISPATCH_TASK_OPENED 回显 conditions A", code, raw)
	}

	code, view, raw = post("SYN-CONDITION/B")
	if code != http.StatusOK || view.Outcome != "DISPATCH_TASK_ALREADY_OPEN" || view.Task != task || view.Conditions != "SYN-CONDITION/A" {
		t.Fatalf("以 B 重投：%d %s, want 200 DISPATCH_TASK_ALREADY_OPEN 回显在册的 conditions A", code, raw)
	}

	var rows int
	var conditions string
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*), min(conditions_ref) FROM transport_fulfillment.dispatch_task
		  WHERE tenant_id = 'SYN-TENANT-01' AND task_ref = $1`, task).Scan(&rows, &conditions); err != nil {
		t.Fatalf("读任务行：%v", err)
	}
	if rows != 1 || conditions != "SYN-CONDITION/A" {
		t.Fatalf("任务行 %d、conditions_ref %q, want 1 行 SYN-CONDITION/A——重投改写了工作范围", rows, conditions)
	}
}
