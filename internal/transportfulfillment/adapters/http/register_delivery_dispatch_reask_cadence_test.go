package tfhttp_test

import (
	"context"
	"encoding/json"
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

// 本文件证派送重问间隔登记那一口的线格式（ADR-0230 决定五）。

type cadenceSink struct{ saved bool }

func (sink *cadenceSink) FindByVersion(context.Context, domain.TenantID, domain.DeliveryDispatchReaskCadenceVersion) (ports.DeliveryDispatchReaskCadenceRecord, bool, error) {
	return ports.DeliveryDispatchReaskCadenceRecord{}, false, nil
}

func (sink *cadenceSink) Save(context.Context, ports.DeliveryDispatchReaskCadenceRecord) (bool, error) {
	sink.saved = true
	return true, nil
}

// Covers: AT-TF-287 的线格式那一半（ADR-0230 决定五）——重问间隔登记挂登记册配置写：租户取认证结果，首登 201 带版本、整分钟与登记时刻；
// 间隔不是正的整分钟 200 INPUT_NOT_ACCEPTED、不写；载荷夹带租户 400。
func TestTheReaskCadenceRegistrationFaceTranslatesFieldByField(t *testing.T) {
	sink := &cadenceSink{}
	endpoint := tfhttp.NewRegisterReaskCadenceEndpoint(registryIntake(t), application.NewRegisterReaskCadenceHandler(application.RegisterReaskCadenceDeps{
		Cadences: sink, Clock: tfClock{at: time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)},
	}))
	post := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, registryRequest("t", body))
		return recorder
	}
	if refused := post(`{"version":"SYN-REASK/v1","intervalMinutes":0}`); refused.Code != http.StatusOK ||
		!strings.Contains(refused.Body.String(), `"INPUT_NOT_ACCEPTED"`) || sink.saved {
		t.Fatalf("零分钟答 %d %s、写了=%v", refused.Code, refused.Body.String(), sink.saved)
	}
	if smuggled := post(`{"version":"SYN-REASK/v1","intervalMinutes":60,"tenantId":"SYN-TENANT-02"}`); smuggled.Code != http.StatusBadRequest {
		t.Fatalf("夹带租户答 %d %s", smuggled.Code, smuggled.Body.String())
	}
	registered := post(`{"version":"SYN-REASK/v1","intervalMinutes":360}`)
	var body struct {
		Outcome         string `json:"outcome"`
		Version         string `json:"version"`
		IntervalMinutes int64  `json:"intervalMinutes"`
		RecordedAt      string `json:"recordedAt"`
	}
	if err := json.Unmarshal(registered.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if registered.Code != http.StatusCreated || body.Outcome != "REASK_CADENCE_REGISTERED" || body.Version != "SYN-REASK/v1" ||
		body.IntervalMinutes != 360 || body.RecordedAt != "2026-10-21T08:00:00Z" {
		t.Fatalf("首登答 %d %s", registered.Code, registered.Body.String())
	}
}
