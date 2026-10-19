package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	nodeopsaccess "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfaccess "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/accessidentity"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

// Covers: 票 operator-channel/10 完成判据「隔离放行用例改写为真渠道答复格用例」在节点收寄口上的那一格——收寄口走作业事实渠道。
// 编排走生产装配（buildReceptionOrchestration），中间是生产的端点构造函数。作业范围种类必须是 NODE，节点引用取作业范围的引用。
//
// 明确拒收不经身份核对，真实落库答`未收寄`；明确接收的号码不在 PS 的外部标识关联册里，答`待识别`（ADR-0184：查过了、
// 查无此号，真实建立收寄与控制）——两格都是编排形成的业务答案（ADR-0022：2xx 带 outcome），不是渠道层的 403。带 tenantId 的载荷在 Intake 就拒，编排一次也不该被调到。
// 测试输入是隔离合成，只记 `S`。
func TestNodeOperationsReceptionLineAnswersBusinessOutcomesAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	reception, err := buildReceptionOrchestration(db)
	if err != nil {
		t.Fatalf("装配收寄编排：%v", err)
	}
	endpoint := nodeopshttp.NewReceiveDeliveredUnitEndpoint(workFactReceptionForTest(t), reception)
	post := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/node-operations/receptions", strings.NewReader(body))
		stampWorkFact(request, "NODE", "SYN-NODE-SHA-HUB", "SYN-DEVICE-NODE")
		endpoint.ServeHTTP(recorder, request)
		return recorder
	}

	refused := post(`{"sourceId":"SYN-DEVICE-08/000001","deliveredBy":"SYN-COURIER-08","handlingUnit":"SYN-HU-08-0001",` +
		`"claim":"REFUSED","refusalReason":"SYN 外包装破损拒收","occurredAt":"2026-09-24T09:00:00+08:00"}`)
	if refused.Code != http.StatusOK || registrationOutcome(t, refused) != "INTAKE_NOT_FORMED" {
		t.Fatalf("明确拒收答 %d %s，want 200 INTAKE_NOT_FORMED", refused.Code, refused.Body.String())
	}

	received := post(`{"sourceId":"SYN-DEVICE-08/000002","deliveredBy":"SYN-COURIER-08","handlingUnit":"SYN-HU-08-0002",` +
		`"mark":"SYN-MARK-08-0002","claim":"RECEIVED","evidenceRef":"SYN-EVIDENCE/tally-08-0002","occurredAt":"2026-09-24T09:05:00+08:00"}`)
	if received.Code != http.StatusOK || registrationOutcome(t, received) != "PENDING_IDENTIFICATION" {
		t.Fatalf("明确接收答 %d %s，want 200 PENDING_IDENTIFICATION（号码不在关联册里）", received.Code, received.Body.String())
	}

	selfReported := post(`{"tenantId":"SYN-TENANT-01","sourceId":"SYN-DEVICE-08/000003","deliveredBy":"SYN-COURIER-08",` +
		`"handlingUnit":"SYN-HU-08-0003","claim":"REFUSED","refusalReason":"r","occurredAt":"2026-09-24T09:10:00+08:00"}`)
	if selfReported.Code != http.StatusBadRequest || problemCode(t, selfReported) != "MALFORMED_REQUEST" {
		t.Fatalf("带 tenantId 的载荷答 %d %s，want 400 MALFORMED_REQUEST", selfReported.Code, selfReported.Body.String())
	}
	assertNoOutcome(t, selfReported, "/node-operations/receptions")
}

func stampWorkFact(request *http.Request, kind, reference, device string) {
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	request.Header.Set("Parcel-Work-Scope-Kind", kind)
	request.Header.Set("Parcel-Work-Scope-Reference", reference)
	request.Header.Set("Parcel-Device-Id", device)
}

type workFactVerifier struct{}

func (workFactVerifier) VerifyOperatorCredential(_ context.Context, credential accessidentity.OperatorCredential) (accessidentity.OperatorSubject, error) {
	if credential.Token() == "" {
		return accessidentity.OperatorSubject{}, accessidentity.ErrCredentialRejected
	}
	return accessidentity.NewOperatorSubject("https://id.syn.example/dex", "SYN-OPERATOR-01")
}

type workFactOperators struct{}

func (workFactOperators) FindOperator(_ context.Context, subject accessidentity.OperatorSubject) (accessidentity.OperatorStanding, bool, error) {
	binding, err := accessidentity.NewOperatorBinding(subject, "SYN-TENANT-01", "SYN-BASIS")
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	var grants []accessidentity.RecordedGrant
	for _, scope := range []accessidentity.WorkScope{mustScope("OFFSITE", "SYN-OFFSITE-01"), mustScope("NODE", "SYN-NODE-SHA-HUB")} {
		grant, err := accessidentity.NewWorkFactRegistrationGrant("SYN-TENANT-01", "SYN-GRANT-"+scope.Reference(), subject, scope, interval, "SYN-BASIS")
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

type workFactDevices struct{}

func (workFactDevices) FindDevice(_ context.Context, tenantID, deviceID string) (accessidentity.RecordedDevice, bool, error) {
	var scope accessidentity.WorkScope
	switch deviceID {
	case "SYN-DEVICE-OFFSITE":
		scope = mustScope("OFFSITE", "SYN-OFFSITE-01")
	case "SYN-DEVICE-NODE":
		scope = mustScope("NODE", "SYN-NODE-SHA-HUB")
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

func mustScope(kind, reference string) accessidentity.WorkScope {
	scope, err := accessidentity.NewWorkScope(accessidentity.WorkScopeKind(kind), reference)
	if err != nil {
		panic(err)
	}
	return scope
}

func workFactReceptionForTest(t *testing.T) nodeopshttp.ReceptionIntake {
	t.Helper()
	minter, err := accessidentity.NewWorkFactMinter(workFactVerifier{}, workFactOperators{}, workFactDevices{}, accessidentity.UnconfiguredAdmissionScope{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	authenticator, err := nodeopsaccess.NewWorkFactAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	intake, err := nodeopshttp.NewWorkFactReceptionIntake(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	return intake
}

func workFactTransportForTest(t *testing.T) *tfhttp.WorkFactCommandIntake {
	t.Helper()
	minter, err := accessidentity.NewWorkFactMinter(workFactVerifier{}, workFactOperators{}, workFactDevices{}, accessidentity.UnconfiguredAdmissionScope{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	authenticator, err := tfaccess.NewWorkFactAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	intake, err := tfhttp.NewWorkFactCommandIntake(authenticator, frontlineMovementSource)
	if err != nil {
		t.Fatal(err)
	}
	return intake
}
