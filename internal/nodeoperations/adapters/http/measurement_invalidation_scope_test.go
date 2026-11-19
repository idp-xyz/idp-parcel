package nodeopshttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件证作废口怎样表达「限定在哪个节点」：作业端的口把认证出的节点交进命令，手工的口交「不限节点」；到了编排，别的节点的
// 作废答 TARGET_AT_ANOTHER_NODE，状态与 TARGET_MISSING 同样处理。夹具全为合成（S 级）。

const (
	deviceVoidBody = `{"sourceId":"SYN-VOID-1","targetSourceId":"SYN-WEIGH-1","reason":"称错件","performedBy":"SYN-SCALE-01",` +
		`"evidenceRef":"WEIGH/2","occurredAt":"2026-11-17T11:00:00+08:00"}`
	manualVoidBody = `{"sourceId":"SYN-VOID-1","targetSourceId":"SYN-WEIGH-1","reason":"称错件","performedBy":"SYN-SCALE-01",` +
		`"evidenceRef":"WEIGH/2","occurredAt":"2026-11-17T11:00:00+08:00","originalDocument":"SYN-DOC-1"}`
)

// Covers: 作业端设备的作废把认证出的节点交进命令，而不是请求头出示的作业范围引用：范围是「限定在这个节点」，不是不限节点。
func TestTheDeviceInvalidationIsLimitedToTheAuthenticatedNode(t *testing.T) {
	intake := receptionIntakeForTest(t)
	request := receptionRequest(deviceVoidBody)

	command, err := intake.IntakeMeasurementInvalidation(context.Background(), request)
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	node, limited := command.Scope.AtNode()
	if !limited || node.String() != workFactNode {
		t.Fatalf("Scope 限定的节点 = %q（限定 %v），want %q（认证结果，不是请求头出示的 %q）", node.String(), limited, workFactNode, presentedScopeReference)
	}
	if command.Scope.AcrossTenantNodes() {
		t.Fatal("作业端的作废被交成了不限节点")
	}
}

// Covers: 手工作废（运营决定面）不限节点：命令的范围是「本租户任何节点」，没有被限定的节点。
func TestTheManualInvalidationIsNotLimitedToANode(t *testing.T) {
	intake, err := nodeopshttp.NewOperatorDecisionIntake(measurementDecisionAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(manualVoidBody))

	command, err := intake.IntakeManualMeasurementInvalidation(request.Context(), request)
	if err != nil {
		t.Fatalf("手工作废：%v", err)
	}
	if !command.Scope.AcrossTenantNodes() {
		t.Fatal("手工作废的范围不是本租户任何节点")
	}
	if node, limited := command.Scope.AtNode(); limited {
		t.Fatalf("手工作废被限定在节点 %q", node.String())
	}
}

// scopeTargetStore 是只有一条测量在册的登记册替身：那条测量登记在 node 上。写口记下有没有被碰到。
type scopeTargetStore struct {
	ports.ActualMeasurementStore
	target ports.ActualMeasurementRecord
	saved  bool
}

func (*scopeTargetStore) SerializeSourceIdentity(context.Context, ports.ActualMeasurementKey) error {
	return nil
}

func (store *scopeTargetStore) FindMeasurement(_ context.Context, key ports.ActualMeasurementKey) (ports.ActualMeasurementRecord, bool, error) {
	if key.SourceID == store.target.Key.SourceID {
		return store.target, true, nil
	}
	return ports.ActualMeasurementRecord{}, false, nil
}

func (*scopeTargetStore) FindInvalidation(context.Context, ports.ActualMeasurementKey) (ports.MeasurementInvalidationRecord, bool, error) {
	return ports.MeasurementInvalidationRecord{}, false, nil
}

func (store *scopeTargetStore) SaveInvalidation(context.Context, ports.MeasurementInvalidationRecord) (ports.MeasurementInvalidationSaveOutcome, error) {
	store.saved = true
	return ports.MeasurementInvalidationSaved, nil
}

type scopeQuietNotice struct{}

func (scopeQuietNotice) NoticeMeasurementChanged(context.Context, ports.MeasurementChanged) error {
	return nil
}

type scopeClock struct{}

func (scopeClock) Now() time.Time { return time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC) }

func newScopeTargetStore(t *testing.T, node string) *scopeTargetStore {
	t.Helper()
	party, err := domain.NewPerformingPartyReference("SYN-SCALE-01")
	if err != nil {
		t.Fatal(err)
	}
	proof, err := domain.NewExecutionEvidenceReference("WEIGH/1")
	if err != nil {
		t.Fatal(err)
	}
	source, err := domain.NewWorkFactSource("SYN-WEIGH-1", party, proof, time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	registeredAt, err := domain.NewNodeReference(node)
	if err != nil {
		t.Fatal(err)
	}
	location, err := domain.NewWorkLocationReference("SYN-BENCH-01")
	if err != nil {
		t.Fatal(err)
	}
	basis, err := domain.NewWorkBasisReference("SYN-PARCEL-1")
	if err != nil {
		t.Fatal(err)
	}
	gross, err := domain.NewGrossWeight("2.10", "KG")
	if err != nil {
		t.Fatal(err)
	}
	outer, err := domain.NewOuterDimensions("30", "25", "20", "CM")
	if err != nil {
		t.Fatal(err)
	}
	measurement, err := domain.NewActualMeasurement(domain.ActualMeasurementSpec{
		Source: source, ObjectKind: domain.HandlingUnitObject, ObjectID: "SYN-HU-1",
		Node: registeredAt, Location: location, Basis: basis,
		Weight: gross, Dimensions: outer,
	})
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := domain.NewTenantID(workFactTenant)
	if err != nil {
		t.Fatal(err)
	}
	return &scopeTargetStore{target: ports.ActualMeasurementRecord{
		Key:           ports.ActualMeasurementKey{TenantID: tenant, SourceID: "SYN-WEIGH-1"},
		Measurement:   measurement,
		ContentDigest: measurement.ContentDigest(),
		RecordedAt:    time.Date(2026, 11, 17, 3, 0, 0, 0, time.UTC),
	}}
}

func invalidationOutcome(t *testing.T, endpoint http.Handler, request *http.Request) (int, string) {
	t.Helper()
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	var body struct {
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("解应答 %s：%v", response.Body.String(), err)
	}
	return response.Code, body.Outcome
}

// Covers: 作业端设备作废别的节点登记的测量：口上答 200 与 TARGET_AT_ANOTHER_NODE，状态与目标不在册的 TARGET_MISSING 同样；写口一次
// 也没碰。手工作废同一条（运营决定面不限节点）照常记下，答 201。
func TestAnInvalidationAtAnotherNodeAnswersLikeAMissingTargetAndAManualOneGoesThrough(t *testing.T) {
	store := newScopeTargetStore(t, "SYN-NODE-SHA-HUB")
	handler, err := application.NewRecordActualMeasurementHandler(store, scopeClock{}, scopeQuietNotice{})
	if err != nil {
		t.Fatal(err)
	}
	// 认证出的节点是 workFactNode（SYN-NODE-SZX-01），那条测量登记在 SYN-NODE-SHA-HUB 上。
	device := nodeopshttp.NewInvalidateMeasurementEndpoint(receptionIntakeForTest(t), handler)

	status, outcome := invalidationOutcome(t, device, receptionRequest(deviceVoidBody))
	if status != http.StatusOK || outcome != "TARGET_AT_ANOTHER_NODE" {
		t.Fatalf("别的节点的设备作废 = %d %s，want 200 TARGET_AT_ANOTHER_NODE", status, outcome)
	}
	if store.saved {
		t.Fatal("别的节点的作废碰了写口")
	}

	missing := strings.Replace(deviceVoidBody, `"targetSourceId":"SYN-WEIGH-1"`, `"targetSourceId":"SYN-WEIGH-NOWHERE"`, 1)
	missingStatus, missingOutcome := invalidationOutcome(t, device, receptionRequest(missing))
	if missingStatus != status || missingOutcome != "TARGET_MISSING" {
		t.Fatalf("目标不在册 = %d %s，want %d TARGET_MISSING（与别的节点同样的状态）", missingStatus, missingOutcome, status)
	}

	manualIntake, err := nodeopshttp.NewOperatorDecisionIntake(measurementDecisionAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	manual := nodeopshttp.NewInvalidateManualMeasurementEndpoint(manualIntake, handler)
	status, outcome = invalidationOutcome(t, manual, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(manualVoidBody)))
	if status != http.StatusCreated || outcome != "INVALIDATION_RECORDED" || !store.saved {
		t.Fatalf("手工作废别的节点登记的测量 = %d %s（写口 %v），want 201 INVALIDATION_RECORDED", status, outcome, store.saved)
	}
}
