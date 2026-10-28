package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

const (
	collaborationAcceptancesPath = "/node-operations/collaboration-acceptances"
	collaborationExecutionsPath  = "/node-operations/collaboration-executions"
)

// collaborationDecisions 在一份库上登记节点监管协作承接的准入对照，交回挂着两位合成操作者的运营决定 Intake（节点作业那一格）。
func collaborationDecisions(t *testing.T, db *bentopg.DB) *nodeopshttp.OperatorDecisionIntake {
	t.Helper()
	kinds := []accessidentity.DecisionKind{accessidentity.DecisionNodeCollaborationAcceptance}
	registerOperationDecisionAdmission(t, db, isolatedSelfAuthority, kinds...)
	admission, err := buildAdmissionScope(db, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	minter, err := buildOperatorMinter(twoOperatorVerifier{}, decisionRegister{kinds: kinds}, admission)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	return decisions.nodeOperations
}

func executeAtDevice(endpoint http.Handler, device, body string) (int, string) {
	request := httptest.NewRequest(http.MethodPost, collaborationExecutionsPath, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	request.Header.Set("Parcel-Device-Id", device)
	request.Header.Set("Parcel-Work-Scope-Kind", "NODE")
	request.Header.Set("Parcel-Work-Scope-Reference", "SYN-NODE-SHA-HUB")
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response.Code, response.Body.String()
}

func countCollaborationRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatalf("读 %q：%v", query, err)
	}
	return n
}

func collaborationAcceptanceBody(item, node, decision, rest string) string {
	return `{"item":"` + item + `","node":"` + node + `","decision":"` + decision + `","authority":"SYN-NODE-AUTHORITY-04",` +
		`"decidedAt":"2026-10-27T08:00:00Z"` + rest + `}`
}

// Covers: `AT-NO-133`（真库）——节点监管协作承接口与执行事实口经生产装配走到真编排：先答待补充（201，一轮，带缺口），事项上登记执行事实被拒
// （`ITEM_PENDING_SUPPLEMENT`、不落事实）；补齐后同一事项的承接决定是下一轮（201，回指待补充，库里两轮并存）；同一承接原样再投答
// `EXISTING_DECISION`、不多入意图；改了范围再投答 `DECISION_CONFLICT`；承接范围内的执行事实首投 201、重放 `EXISTING_FACT`、同键异证据
// `FACT_CONFLICT`、越出动作范围 `OUTSIDE_ACCEPTED_SCOPE`；拒接的事项上登记 `ITEM_DECLINED`；两份意图各按轮次与键入队一次。
func TestTheNodeCollaborationFacesAcceptSupplementAndRecordExecutionAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	handler, err := buildNodeCollaboration(db)
	if err != nil {
		t.Fatalf("装配协作编排：%v", err)
	}
	acceptEndpoint := nodeopshttp.NewAcceptCollaborationEndpoint(collaborationDecisions(t, db), handler)
	intake, err := buildNodeCollaborationExecutionIntake(acceptingVerifier{}, workFactOperators{}, workFactDevices{}, admitAll{})
	if err != nil {
		t.Fatal(err)
	}
	executeEndpoint := nodeopshttp.NewRecordCollaborationExecutionEndpoint(intake, handler)

	const item = "SYN-CC-ITEM-NO04-01"
	const node = "SYN-NODE-SHA-HUB"
	executeBody := func(item, action, evidence string) string {
		return `{"item":"` + item + `","unit":"SYN-UNIT-NO04-1","action":"` + action + `","evidence":"` + evidence + `","performedAt":"2026-10-27T09:00:00Z"}`
	}

	// 待补充：缺授权依据，记一轮、不带承接范围。
	pending := collaborationAcceptanceBody(item, node, "PENDING_SUPPLEMENT", `,"gaps":["AUTHORITY"]`)
	status, answer := postWithToken(t, acceptEndpoint, collaborationAcceptancesPath, firstOperatorToken, pending)
	if status != http.StatusCreated || answer["outcome"] != "PENDING_SUPPLEMENT" || answer["round"] != float64(1) {
		t.Fatalf("待补充 = %d %v，want 201 PENDING_SUPPLEMENT 第 1 轮", status, answer)
	}
	if code, body := executeAtDevice(executeEndpoint, "SYN-DEVICE-NODE", executeBody(item, "UNSEAL", "SYN-EVIDENCE-1")); code != http.StatusOK || !strings.Contains(body, `"outcome":"ITEM_PENDING_SUPPLEMENT"`) {
		t.Fatalf("待补充事项上登记事实 = %d %s，want 200 ITEM_PENDING_SUPPLEMENT", code, body)
	}
	if n := countCollaborationRows(t, pool, `SELECT count(*) FROM node_operations.execution_fact WHERE tenant_id = 'SYN-TENANT-01'`); n != 0 {
		t.Fatalf("待补充事项上落了 %d 条执行事实，want 0", n)
	}

	// 补齐后的决定是下一轮。
	accepted := collaborationAcceptanceBody(item, node, "ACCEPTED", `,"acceptedUnits":["SYN-UNIT-NO04-1"],"acceptedActions":["UNSEAL","PRESENT"]`)
	status, answer = postWithToken(t, acceptEndpoint, collaborationAcceptancesPath, firstOperatorToken, accepted)
	if status != http.StatusCreated || answer["outcome"] != "COLLABORATION_DECIDED" || answer["round"] != float64(2) {
		t.Fatalf("补充后承接 = %d %v，want 201 COLLABORATION_DECIDED 第 2 轮", status, answer)
	}
	if n := countCollaborationRows(t, pool, `SELECT count(*) FROM node_operations.collaboration_acceptance WHERE tenant_id = 'SYN-TENANT-01' AND item_ref = $1`, item); n != 2 {
		t.Fatalf("承接册 = %d 轮，want 2（待补充不改写）", n)
	}
	if status, answer = postWithToken(t, acceptEndpoint, collaborationAcceptancesPath, secondOperatorToken, accepted); status != http.StatusOK || answer["outcome"] != "EXISTING_DECISION" {
		t.Fatalf("原样再投 = %d %v，want 200 EXISTING_DECISION", status, answer)
	}
	narrowed := strings.Replace(accepted, `"acceptedActions":["UNSEAL","PRESENT"]`, `"acceptedActions":["UNSEAL"]`, 1)
	if status, answer = postWithToken(t, acceptEndpoint, collaborationAcceptancesPath, firstOperatorToken, narrowed); status != http.StatusOK || answer["outcome"] != "DECISION_CONFLICT" {
		t.Fatalf("改范围再投 = %d %v，want 200 DECISION_CONFLICT", status, answer)
	}

	// 执行事实。
	code, body := executeAtDevice(executeEndpoint, "SYN-DEVICE-NODE", executeBody(item, "UNSEAL", "SYN-EVIDENCE-1"))
	if code != http.StatusCreated || !strings.Contains(body, `"outcome":"EXECUTION_RECORDED"`) {
		t.Fatalf("首登 = %d %s，want 201 EXECUTION_RECORDED", code, body)
	}
	for _, step := range []struct {
		name, action, evidence, outcome string
	}{
		{"重放", "UNSEAL", "SYN-EVIDENCE-1", "EXISTING_FACT"},
		{"同键异证据", "UNSEAL", "SYN-EVIDENCE-2", "FACT_CONFLICT"},
		{"越出动作范围", "ISOLATE", "SYN-EVIDENCE-1", "OUTSIDE_ACCEPTED_SCOPE"},
	} {
		if code, body := executeAtDevice(executeEndpoint, "SYN-DEVICE-NODE", executeBody(item, step.action, step.evidence)); code != http.StatusOK || !strings.Contains(body, `"outcome":"`+step.outcome+`"`) {
			t.Fatalf("%s = %d %s，want 200 %s", step.name, code, body, step.outcome)
		}
	}

	// 拒接的事项没有可执行的东西。
	declined := collaborationAcceptanceBody("SYN-CC-ITEM-NO04-02", node, "DECLINED", `,"basis":"SYN-NO-CAPABILITY-LIMIT-04"`)
	if status, answer = postWithToken(t, acceptEndpoint, collaborationAcceptancesPath, firstOperatorToken, declined); status != http.StatusCreated || answer["outcome"] != "COLLABORATION_DECIDED" {
		t.Fatalf("拒接 = %d %v", status, answer)
	}
	if code, body := executeAtDevice(executeEndpoint, "SYN-DEVICE-NODE", executeBody("SYN-CC-ITEM-NO04-02", "UNSEAL", "SYN-EVIDENCE-1")); code != http.StatusOK || !strings.Contains(body, `"outcome":"ITEM_DECLINED"`) {
		t.Fatalf("拒接事项上登记事实 = %d %s，want 200 ITEM_DECLINED", code, body)
	}

	if n := countCollaborationRows(t, pool, `SELECT count(*) FROM node_operations.execution_fact WHERE tenant_id = 'SYN-TENANT-01'`); n != 1 {
		t.Fatalf("执行事实 = %d 条，want 1（只有承接范围内的首登落事实）", n)
	}
	countEvents := func(eventType string) int {
		return countCollaborationRows(t, pool, `SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1`, eventType)
	}
	// 操作者作为操作证据落列：承接是首投的那一位（换人重放不改），事实是登记的操作者；回执里没有操作者。
	if got := recordedOperator(t, pool, `SELECT decided_by FROM node_operations.collaboration_acceptance
		 WHERE tenant_id = 'SYN-TENANT-01' AND item_ref = $1 AND round = 2`, item); got != firstOperatorRecorded {
		t.Fatalf("承接决定人 = %q，want %q", got, firstOperatorRecorded)
	}
	if got := recordedOperator(t, pool, `SELECT recorded_by FROM node_operations.execution_fact WHERE tenant_id = 'SYN-TENANT-01'`); got != "https://id.syn.example/dex#SYN-OPERATOR-01" {
		t.Fatalf("事实登记人 = %q", got)
	}
	if got := countEvents("node-operations.collaboration-acceptance.decided"); got != 3 {
		t.Fatalf("承接意图 = %d，want 3（待补充一轮、承接一轮、拒接一条；重放不多入）", got)
	}
	if got := countEvents("node-operations.execution-fact.recorded"); got != 1 {
		t.Fatalf("执行事实意图 = %d，want 1", got)
	}
}

// Covers: `AT-NO-134`（真库）——别的节点登记的执行事实不落：事项承接在 SYN-NODE-OTHER，作业范围是 SYN-NODE-SHA-HUB 的设备登记时答
// `SOURCE_NOT_ACCEPTED`、不落事实、不入意图；授予齐备而准入范围没登的设备答 403 `OUTSIDE_ADMISSION_SCOPE`。
func TestAnExecutionFactFromAnotherNodesDeviceIsNotRecordedAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	handler, err := buildNodeCollaboration(db)
	if err != nil {
		t.Fatal(err)
	}
	acceptEndpoint := nodeopshttp.NewAcceptCollaborationEndpoint(collaborationDecisions(t, db), handler)
	build := func(admission accessidentity.AdmissionScope) http.Handler {
		intake, err := buildNodeCollaborationExecutionIntake(acceptingVerifier{}, workFactOperators{}, workFactDevices{}, admission)
		if err != nil {
			t.Fatal(err)
		}
		return nodeopshttp.NewRecordCollaborationExecutionEndpoint(intake, handler)
	}
	accepted := collaborationAcceptanceBody("SYN-CC-ITEM-NO04-03", "SYN-NODE-OTHER", "ACCEPTED", `,"acceptedUnits":["SYN-UNIT-NO04-1"],"acceptedActions":["UNSEAL"]`)
	if status, answer := postWithToken(t, acceptEndpoint, collaborationAcceptancesPath, firstOperatorToken, accepted); status != http.StatusCreated {
		t.Fatalf("承接 = %d %v", status, answer)
	}
	body := `{"item":"SYN-CC-ITEM-NO04-03","unit":"SYN-UNIT-NO04-1","action":"UNSEAL","evidence":"SYN-EVIDENCE-1","performedAt":"2026-10-27T09:00:00Z"}`
	if code, got := executeAtDevice(build(admitAll{}), "SYN-DEVICE-NODE", body); code != http.StatusOK || !strings.Contains(got, `"outcome":"SOURCE_NOT_ACCEPTED"`) {
		t.Fatalf("别的节点的设备登记 = %d %s，want 200 SOURCE_NOT_ACCEPTED", code, got)
	}
	if n := countCollaborationRows(t, pool, `SELECT count(*) FROM node_operations.execution_fact WHERE tenant_id = 'SYN-TENANT-01'`); n != 0 {
		t.Fatalf("别的节点的设备落了 %d 条事实，want 0", n)
	}
	if code, got := executeAtDevice(build(admissionScopeUnset()), "SYN-DEVICE-NODE", body); code != http.StatusForbidden || !strings.Contains(got, "OUTSIDE_ADMISSION_SCOPE") {
		t.Fatalf("准入范围没登 = %d %s，want 403 OUTSIDE_ADMISSION_SCOPE", code, got)
	}
}
