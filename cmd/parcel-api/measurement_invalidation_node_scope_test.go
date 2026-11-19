package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	nodeopsaccess "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	nodeopsdomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件证作业端设备只能作废在自己节点上登记的测量，手工作废不限节点：编排走生产装配（buildActualMeasurementOrchestration），
// 登记册与 Outbox 是真实 PostgreSQL，设备身份走作业事实渠道的真实铸信封与认证。测试输入是隔离合成，只记 `S`。

const (
	nodeScopeNodeA = "SYN-NODE-SHA-HUB"
	nodeScopeNodeB = "SYN-NODE-SZX-01"
)

// twoNodeOperators 给同一个操作者两个节点的作业事实登记授予。
type twoNodeOperators struct{}

func (twoNodeOperators) FindOperator(_ context.Context, subject accessidentity.OperatorSubject) (accessidentity.OperatorStanding, bool, error) {
	binding, err := accessidentity.NewOperatorBinding(subject, "SYN-TENANT-01", "SYN-BASIS")
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	var grants []accessidentity.RecordedGrant
	for _, node := range []string{nodeScopeNodeA, nodeScopeNodeB} {
		scope := mustScope("NODE", node)
		grant, err := accessidentity.NewWorkFactRegistrationGrant("SYN-TENANT-01", "SYN-GRANT-"+node, subject, scope, interval, "SYN-BASIS")
		if err != nil {
			return accessidentity.OperatorStanding{}, false, err
		}
		recorded, err := accessidentity.NewRecordedGrant(grant, nil)
		if err != nil {
			return accessidentity.OperatorStanding{}, false, err
		}
		grants = append(grants, recorded)
	}
	standing, err := accessidentity.NewOperatorStanding(binding, grants)
	return standing, err == nil, err
}

// twoNodeDevices 各节点一台设备，登在各自的节点上。
type twoNodeDevices struct{}

func (twoNodeDevices) FindDevice(_ context.Context, tenantID, deviceID string) (accessidentity.RecordedDevice, bool, error) {
	var scope accessidentity.WorkScope
	switch deviceID {
	case "SYN-DEVICE-NODE-A":
		scope = mustScope("NODE", nodeScopeNodeA)
	case "SYN-DEVICE-NODE-B":
		scope = mustScope("NODE", nodeScopeNodeB)
	default:
		return accessidentity.RecordedDevice{}, false, nil
	}
	device, err := accessidentity.NewDeviceRegistration(tenantID, deviceID, scope, "SYN-BASIS")
	if err != nil {
		return accessidentity.RecordedDevice{}, false, err
	}
	recorded, err := accessidentity.NewRecordedDevice(device, nil)
	if err != nil {
		return accessidentity.RecordedDevice{}, false, err
	}
	return recorded, true, nil
}

// manualMeasurementOperators 认证一次手工录入：租户与操作者都是合成值，不绑任何节点。
type manualMeasurementOperators struct{}

func (manualMeasurementOperators) AuthenticateOperatorDecision(
	_ context.Context, token string, decision nodeopshttp.OperatorDecision,
) (nodeopshttp.OperatorIdentity, error) {
	if token == "" || decision != nodeopshttp.OperatorDecisionManualMeasurementEntry {
		return nodeopshttp.OperatorIdentity{}, nodeopshttp.ErrOperatorCredentialRejected
	}
	tenant, err := nodeopsdomain.NewTenantID(sharedIdentityTenant)
	return nodeopshttp.OperatorIdentity{Tenant: tenant, Operator: "https://id.syn.example#SYN-OPERATOR-01"}, err
}

type nodeScopeFixture struct {
	*sharedIdentityFixture
	record           http.Handler
	invalidate       http.Handler
	manualInvalidate http.Handler
}

func newNodeScopeFixture(t *testing.T) *nodeScopeFixture {
	t.Helper()
	base := newSharedIdentityFixture(t)
	minter, err := accessidentity.NewWorkFactMinter(workFactVerifier{}, twoNodeOperators{}, twoNodeDevices{}, accessidentity.UnconfiguredAdmissionScope{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	authenticator, err := nodeopsaccess.NewWorkFactAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	device, err := nodeopshttp.NewWorkFactReceptionIntake(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	manual, err := nodeopshttp.NewOperatorDecisionIntake(manualMeasurementOperators{})
	if err != nil {
		t.Fatal(err)
	}
	return &nodeScopeFixture{
		sharedIdentityFixture: base,
		record:                nodeopshttp.NewRecordMeasurementEndpoint(device, base.handler),
		invalidate:            nodeopshttp.NewInvalidateMeasurementEndpoint(device, base.handler),
		manualInvalidate:      nodeopshttp.NewInvalidateManualMeasurementEndpoint(manual, base.handler),
	}
}

// postAs 以某个节点上的设备出示请求。
func (fixture *nodeScopeFixture) postAs(t *testing.T, endpoint http.Handler, node, device, path, body string) (int, string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	stampWorkFact(request, "NODE", node, device)
	endpoint.ServeHTTP(recorder, request)
	return recorder.Code, registrationOutcome(t, recorder)
}

func (fixture *nodeScopeFixture) registerAtNodeA(t *testing.T, sourceID, unit string) {
	t.Helper()
	body := `{"sourceId":"` + sourceID + `","objectKind":"HANDLING_UNIT","objectId":"` + unit + `",` +
		`"location":"SYN-BENCH-01","basis":"SYN-PARCEL-11","performedBy":"SYN-SCALE-01","evidenceRef":"WEIGH/1",` +
		`"occurredAt":"2026-11-17T02:00:00Z","weightValue":"2.10","weightUnit":"KG",` +
		`"length":"20","width":"15","height":"10","dimensionUnit":"CM"}`
	if status, outcome := fixture.postAs(t, fixture.record, nodeScopeNodeA, "SYN-DEVICE-NODE-A", "/node-operations/measurements", body); status != http.StatusCreated || outcome != "MEASUREMENT_RECORDED" {
		t.Fatalf("节点 A 的设备登记 %s = %d %s，want 201 MEASUREMENT_RECORDED", sourceID, status, outcome)
	}
}

func (fixture *nodeScopeFixture) registerAtNodeB(t *testing.T, sourceID, unit string) {
	t.Helper()
	body := `{"sourceId":"` + sourceID + `","objectKind":"HANDLING_UNIT","objectId":"` + unit + `",` +
		`"location":"SYN-BENCH-02","basis":"SYN-PARCEL-11","performedBy":"SYN-SCALE-02","evidenceRef":"WEIGH/1",` +
		`"occurredAt":"2026-11-17T02:05:00Z","weightValue":"3.20","weightUnit":"KG",` +
		`"length":"30","width":"25","height":"20","dimensionUnit":"CM"}`
	if status, outcome := fixture.postAs(t, fixture.record, nodeScopeNodeB, "SYN-DEVICE-NODE-B", "/node-operations/measurements", body); status != http.StatusCreated || outcome != "MEASUREMENT_RECORDED" {
		t.Fatalf("节点 B 的设备登记 %s = %d %s，want 201 MEASUREMENT_RECORDED", sourceID, status, outcome)
	}
}

func deviceInvalidationBody(sourceID, target string) string {
	return `{"sourceId":"` + sourceID + `","targetSourceId":"` + target + `","reason":"称错件",` +
		`"performedBy":"SYN-SCALE-01","evidenceRef":"WEIGH/2","occurredAt":"2026-11-17T03:00:00Z"}`
}

// Covers: 节点 A 的设备登记的测量，节点 B 的设备作废不得：口上答 200 TARGET_AT_ANOTHER_NODE（与 TARGET_MISSING 同样的处理），册上
// 没有作废行，仍有效清单里那条还在，不发变化事件；A 的设备自己作废成功之后，B 的设备连重放 A 那条作废也只得 TARGET_AT_ANOTHER_NODE。
func TestADeviceAtAnotherNodeCannotVoidAMeasurementAgainstARealDatabase(t *testing.T) {
	fixture := newNodeScopeFixture(t)
	const unit = "SYN-HU-11-0001"
	fixture.registerAtNodeA(t, "SYN-DEVICE-11/WEIGH-A", unit)
	before := fixture.changeEvents(t)

	status, outcome := fixture.postAs(t, fixture.invalidate, nodeScopeNodeB, "SYN-DEVICE-NODE-B",
		"/node-operations/measurement-invalidations", deviceInvalidationBody("SYN-DEVICE-11/VOID-B", "SYN-DEVICE-11/WEIGH-A"))
	if status != http.StatusOK || outcome != "TARGET_AT_ANOTHER_NODE" {
		t.Fatalf("节点 B 的设备作废节点 A 的测量 = %d %s，want 200 TARGET_AT_ANOTHER_NODE", status, outcome)
	}
	if fixture.invalidationRow(t, "SYN-DEVICE-11/VOID-B") {
		t.Fatal("别的节点的作废落了册")
	}
	if ids := fixture.stillValid(t, unit); !ids["SYN-DEVICE-11/WEIGH-A"] {
		t.Fatalf("仍有效 = %v，want 节点 A 的那条还在", ids)
	}
	if after := fixture.changeEvents(t); after != before {
		t.Fatalf("变化事件 %d 封，want %d（别的节点的作废不发）", after, before)
	}

	if status, outcome := fixture.postAs(t, fixture.invalidate, nodeScopeNodeA, "SYN-DEVICE-NODE-A",
		"/node-operations/measurement-invalidations", deviceInvalidationBody("SYN-DEVICE-11/VOID-A", "SYN-DEVICE-11/WEIGH-A")); status != http.StatusCreated || outcome != "INVALIDATION_RECORDED" {
		t.Fatalf("节点 A 的设备作废自己节点的测量 = %d %s，want 201 INVALIDATION_RECORDED", status, outcome)
	}
	if ids := fixture.stillValid(t, unit); ids["SYN-DEVICE-11/WEIGH-A"] {
		t.Fatalf("A 作废之后仍有效 = %v，want 空", ids)
	}
	if status, outcome := fixture.postAs(t, fixture.invalidate, nodeScopeNodeB, "SYN-DEVICE-NODE-B",
		"/node-operations/measurement-invalidations", deviceInvalidationBody("SYN-DEVICE-11/VOID-A", "SYN-DEVICE-11/WEIGH-A")); status != http.StatusOK || outcome != "TARGET_AT_ANOTHER_NODE" {
		t.Fatalf("节点 B 的设备重放节点 A 的作废 = %d %s，want 200 TARGET_AT_ANOTHER_NODE", status, outcome)
	}
	if status, outcome := fixture.postAs(t, fixture.invalidate, nodeScopeNodeA, "SYN-DEVICE-NODE-A",
		"/node-operations/measurement-invalidations", deviceInvalidationBody("SYN-DEVICE-11/VOID-A", "SYN-DEVICE-11/WEIGH-A")); status != http.StatusOK || outcome != "EXISTING_RESULT" {
		t.Fatalf("节点 A 的设备重放自己的作废 = %d %s，want 200 EXISTING_RESULT", status, outcome)
	}
}

// Covers: 手工作废（运营决定面）不限节点：持授予的操作者可以作废本租户任何节点上登记的测量。
func TestAManualInvalidationReachesAMeasurementAtAnyNodeAgainstARealDatabase(t *testing.T) {
	fixture := newNodeScopeFixture(t)
	const unit = "SYN-HU-11-0002"
	fixture.registerAtNodeB(t, "SYN-DEVICE-11/WEIGH-B", unit)

	body := `{"sourceId":"SYN-DEVICE-11/MANUAL-VOID-B","targetSourceId":"SYN-DEVICE-11/WEIGH-B","reason":"称错件",` +
		`"performedBy":"SYN-OPERATOR-01","evidenceRef":"WEIGH/3","occurredAt":"2026-11-17T04:00:00Z","originalDocument":"SYN-DOC-11"}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/node-operations/manual-measurement-invalidations", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	fixture.manualInvalidate.ServeHTTP(recorder, request)
	if outcome := registrationOutcome(t, recorder); recorder.Code != http.StatusCreated || outcome != "INVALIDATION_RECORDED" {
		t.Fatalf("手工作废别的节点登记的测量 = %d %s，want 201 INVALIDATION_RECORDED", recorder.Code, outcome)
	}
	if ids := fixture.stillValid(t, unit); len(ids) != 0 {
		t.Fatalf("手工作废之后仍有效 = %v，want 空", ids)
	}
}
