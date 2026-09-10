package tfhttp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证实际承运商首次有效收寄的两个在线口（label-channel/31）的传输层纪律：新落一版收寄 201、待确认等形成了的
// 答案 200 且 `outcome` 区分、载荷严格解码（未知键拒、来源词集外拒）、未配置 Intake 403 不读体、GET 按对象交回整链、
// 对象缺席 400。编排是真处理器接内存登记册替身，不构造结果。

type carrierPickupJudgmentIntakeDouble struct{ tenant string }

func (double carrierPickupJudgmentIntakeDouble) IntakeCarrierPickupJudgment(_ context.Context, request *http.Request) (application.JudgeCarrierFirstEffectivePickupCommand, error) {
	payload, err := tfhttp.DecodeCarrierPickupJudgmentPayload(request.Body)
	if err != nil {
		return application.JudgeCarrierFirstEffectivePickupCommand{}, err
	}
	tenant, err := domain.NewTenantID(double.tenant)
	if err != nil {
		return application.JudgeCarrierFirstEffectivePickupCommand{}, err
	}
	return payload.Command(tenant)
}

type carrierPickupRegistryDouble struct {
	records []ports.CarrierFirstEffectivePickupRecord
}

func (double *carrierPickupRegistryDouble) FindByKey(_ context.Context, key ports.CarrierFirstEffectivePickupKey) (ports.CarrierFirstEffectivePickupRecord, bool, error) {
	for _, record := range double.records {
		if record.Key == key {
			return record, true, nil
		}
	}
	return ports.CarrierFirstEffectivePickupRecord{}, false, nil
}

func (double *carrierPickupRegistryDouble) FindCurrentByObject(_ context.Context, tenant domain.TenantID, object domain.CarriedObjectReference) (ports.CarrierFirstEffectivePickupRecord, bool, error) {
	superseded := map[domain.CarrierFirstEffectivePickupVersion]bool{}
	for _, record := range double.records {
		if prior, has := record.Pickup.Supersedes(); has {
			superseded[prior] = true
		}
	}
	for _, record := range double.records {
		if record.Key.TenantID == tenant && record.Pickup.Object() == object && !superseded[record.Key.Version] {
			return record, true, nil
		}
	}
	return ports.CarrierFirstEffectivePickupRecord{}, false, nil
}

func (double *carrierPickupRegistryDouble) ListByObject(_ context.Context, tenant domain.TenantID, object domain.CarriedObjectReference) ([]ports.CarrierFirstEffectivePickupRecord, error) {
	var records []ports.CarrierFirstEffectivePickupRecord
	for _, record := range double.records {
		if record.Key.TenantID == tenant && record.Pickup.Object() == object {
			records = append(records, record)
		}
	}
	return records, nil
}

func (double *carrierPickupRegistryDouble) Save(_ context.Context, record ports.CarrierFirstEffectivePickupRecord) (ports.CarrierPickupSaveOutcome, error) {
	double.records = append(double.records, record)
	return ports.CarrierPickupSaved, nil
}

type carrierPickupIdentityDouble struct{ n int }

func (double *carrierPickupIdentityDouble) NextCarrierFirstEffectivePickupReference(context.Context) (domain.CarrierFirstEffectivePickupReference, error) {
	double.n++
	return domain.NewCarrierFirstEffectivePickupReference(fmt.Sprintf("CFEP-%d", double.n))
}

func (double *carrierPickupIdentityDouble) NextCarrierFirstEffectivePickupVersion(context.Context) (domain.CarrierFirstEffectivePickupVersion, error) {
	double.n++
	return domain.NewCarrierFirstEffectivePickupVersion(fmt.Sprintf("CFEV-%d", double.n))
}

type carrierPickupHandoffDouble struct{ count int }

func (double *carrierPickupHandoffDouble) HandOffCarrierFirstEffectivePickup(context.Context, ports.CarrierFirstEffectivePickupHandoffIntent) error {
	double.count++
	return nil
}

type carrierDirectoryDouble struct{ registered bool }

func (double carrierDirectoryDouble) IdentityRegistered(context.Context, domain.TenantID, domain.CarrierSubject) (bool, error) {
	return double.registered, nil
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

func newCarrierPickupHTTPFixture(t *testing.T, registered bool) (*carrierPickupRegistryDouble, http.Handler, http.Handler) {
	t.Helper()
	pickups := &carrierPickupRegistryDouble{}
	handler := application.NewJudgeCarrierFirstEffectivePickupHandler(application.JudgeCarrierFirstEffectivePickupDeps{
		Pickups:    pickups,
		Identities: &carrierPickupIdentityDouble{},
		Downstream: &carrierPickupHandoffDouble{},
		Directory:  carrierDirectoryDouble{registered: registered},
		Clock:      fixedClock{at: time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC)},
	})
	judge := tfhttp.NewJudgeCarrierFirstEffectivePickupEndpoint(carrierPickupJudgmentIntakeDouble{tenant: "tenant-1"}, handler)
	query := tfhttp.NewQueryCarrierFirstEffectivePickupsEndpoint(grantedCatalogueIntake{tenant: "tenant-1", limit: 25}, pickups)
	return pickups, judge, query
}

func postCarrierPickupJudgment(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment-carrier-first-effective-pickup-judgments", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

const scanPickupBody = `{"object":"PCL-1","source":"CARRIER_PICKUP_SCAN","evidenceReference":"SCAN-1","evidenceVersion":"v1","expressesControl":true,"occurredAt":"2026-09-10T09:00:00Z","carrierKind":"EXTERNAL_PARTY","carrierReference":"party/carrier-x"}`

// Covers: 新落一版已形成 → 201，`pickup` 带承运主体与业务时间；GET 按对象交回整链。
func TestAFormedPickupAnswers201AndTheChainIsListedByObject(t *testing.T) {
	_, judge, query := newCarrierPickupHTTPFixture(t, true)
	recorder := postCarrierPickupJudgment(t, judge, scanPickupBody)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Outcome string `json:"outcome"`
		Pickup  struct {
			Result, CarrierReference, OccurredAt, Fact, Version string
			Bases                                               []struct{ EvidenceReference, SourceVersion string }
		} `json:"pickup"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("译响应：%v", err)
	}
	if body.Outcome != "PICKUP_FORMED" || body.Pickup.Result != "FORMED" || body.Pickup.CarrierReference != "party/carrier-x" || body.Pickup.OccurredAt != "2026-09-10T09:00:00Z" {
		t.Fatalf("响应形状：%+v", body)
	}
	if len(body.Pickup.Bases) != 1 || body.Pickup.Bases[0].EvidenceReference != "SCAN-1" || body.Pickup.Bases[0].SourceVersion != "v1" {
		t.Fatalf("依据：%+v", body.Pickup.Bases)
	}

	request := httptest.NewRequest(http.MethodGet, "/transport-fulfillment-carrier-first-effective-pickups?object=PCL-1", nil)
	recorder = httptest.NewRecorder()
	query.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var chain struct {
		Outcome string                     `json:"outcome"`
		Pickups []struct{ Version string } `json:"pickups"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &chain); err != nil {
		t.Fatalf("译链：%v", err)
	}
	if chain.Outcome != "CARRIER_FIRST_EFFECTIVE_PICKUP_CHAIN_LISTED" || len(chain.Pickups) != 1 || chain.Pickups[0].Version != body.Pickup.Version {
		t.Fatalf("链：%+v", chain)
	}
	request = httptest.NewRequest(http.MethodGet, "/transport-fulfillment-carrier-first-effective-pickups", nil)
	recorder = httptest.NewRecorder()
	query.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("对象缺席应 400，实得 %d", recorder.Code)
	}
}

// Covers: 待确认是形成了的答案 → 200 且 outcome 区分、`pickup` 带原因与名称素材；不构成 → 200 无 pickup。
func TestPendingAndNotAPickupAnswer200WithDistinctOutcomes(t *testing.T) {
	_, judge, _ := newCarrierPickupHTTPFixture(t, false)
	recorder := postCarrierPickupJudgment(t, judge, scanPickupBody)
	if recorder.Code != http.StatusOK {
		t.Fatalf("待确认 status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Outcome string                                               `json:"outcome"`
		Pickup  *struct{ Result, PendingReason, CarrierName string } `json:"pickup"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("译响应：%v", err)
	}
	if body.Outcome != "PICKUP_PENDING" || body.Pickup == nil || body.Pickup.Result != "PENDING" || body.Pickup.PendingReason != "IDENTITY_NOT_REGISTERED" || body.Pickup.CarrierName != "party/carrier-x" {
		t.Fatalf("待确认响应：%+v", body)
	}
	// 同一依据再来只问身份，所以「不构成」要用一条新证据、一条新链来证。
	_, fresh, _ := newCarrierPickupHTTPFixture(t, true)
	recorder = postCarrierPickupJudgment(t, fresh, strings.Replace(scanPickupBody, `"expressesControl":true`, `"expressesControl":false`, 1))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"outcome":"NOT_A_PICKUP"`) || strings.Contains(recorder.Body.String(), `"pickup"`) {
		t.Fatalf("不构成响应：%d %s", recorder.Code, recorder.Body.String())
	}
}

// Covers: 载荷严格解码——未知键（含自报租户）拒 400、来源词集外拒 400、时刻解不出拒 400；GET 用 POST 405；未配置 Intake 403 不读体。
func TestCarrierPickupJudgmentTransportDiscipline(t *testing.T) {
	_, judge, _ := newCarrierPickupHTTPFixture(t, true)
	for name, body := range map[string]string{
		"自报租户":  `{"tenant":"x","object":"PCL-1","source":"CARRIER_PICKUP_SCAN","evidenceReference":"SCAN-1","evidenceVersion":"v1","expressesControl":true}`,
		"来源词集外": `{"object":"PCL-1","source":"BRAND_ON_LABEL","evidenceReference":"SCAN-1","evidenceVersion":"v1","expressesControl":true}`,
		"时刻解不出": `{"object":"PCL-1","source":"CARRIER_PICKUP_SCAN","evidenceReference":"SCAN-1","evidenceVersion":"v1","expressesControl":true,"occurredAt":"yesterday"}`,
		"分支词集外": `{"object":"PCL-1","source":"CARRIER_PICKUP_SCAN","evidenceReference":"SCAN-1","evidenceVersion":"v1","expressesControl":true,"carrierKind":"BRAND"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if recorder := postCarrierPickupJudgment(t, judge, body); recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
	request := httptest.NewRequest(http.MethodGet, "/transport-fulfillment-carrier-first-effective-pickup-judgments", nil)
	recorder := httptest.NewRecorder()
	judge.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET 应 405，实得 %d", recorder.Code)
	}
	unconfigured := tfhttp.NewJudgeCarrierFirstEffectivePickupEndpoint(tfhttp.UnconfiguredIntake{}, nil)
	if recorder := postCarrierPickupJudgment(t, unconfigured, scanPickupBody); recorder.Code != http.StatusForbidden {
		t.Fatalf("未配置 Intake 应 403，实得 %d", recorder.Code)
	}
}
