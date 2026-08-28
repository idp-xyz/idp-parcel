package commercialhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对渠道产品目录端点（票 admin-remainder-mechanism-batch/02）证传输面：方法门、
// 行体逐字段转写（含显式“未配置”绑定答空数组）、空目录答空数组、读失败答 5xx。

type productChannelMappingReaderDouble struct {
	tenant  domain.TenantID
	rows    []ports.ProductChannelMappingRow
	loadErr error
}

func (double *productChannelMappingReaderDouble) ListProductChannelMappings(
	_ context.Context, tenant domain.TenantID, _ int,
) ([]ports.ProductChannelMappingRow, error) {
	if double.loadErr != nil {
		return nil, double.loadErr
	}
	if tenant != double.tenant {
		return nil, nil
	}
	return double.rows, nil
}

var mappingListedAt = time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)

// Covers: 行体逐字段转写——已配置绑定原样上列引用，显式“未配置”答空数组而非 null
// （绑定格是本册存在的理由，空与缺席不可混）；行上没有状态字段。
func TestProductChannelMappingsEndpointTranscribesRows(t *testing.T) {
	reader := &productChannelMappingReaderDouble{
		tenant: catValue(t, domain.NewTenantID, "tenant-1"),
		rows: []ports.ProductChannelMappingRow{
			{
				TenantID:            "tenant-1",
				MappingID:           "map-1",
				Revision:            2,
				ProductObjectID:     "product-1",
				ProductVersionLabel: "v1",
				Channels:            []string{"CH-SG-POST", "CH-AGG"},
				Basis:               "basis-map-1",
				EffectiveStartsAt:   mappingListedAt,
				EffectiveEndsAt:     mappingListedAt.AddDate(1, 0, 0),
				HasEffectiveEnd:     true,
				RegisteredAt:        mappingListedAt,
			},
			{
				TenantID:            "tenant-1",
				MappingID:           "map-2",
				Revision:            1,
				ProductObjectID:     "product-2",
				ProductVersionLabel: "v1",
				Channels:            nil, // 读口交回 nil 也必须转写成空数组
				Basis:               "basis-map-2",
				EffectiveStartsAt:   mappingListedAt,
				RegisteredAt:        mappingListedAt,
			},
		},
	}
	endpoint := commercialhttp.NewQueryProductChannelMappingsEndpoint(
		intakeDouble{query: catalogueQuery(t)}, reader)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/commercial-product-channel-mappings", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}

	var body struct {
		Outcome  string `json:"outcome"`
		Mappings []struct {
			MappingID           string    `json:"mappingId"`
			Revision            int       `json:"revision"`
			ProductObjectID     string    `json:"productObjectId"`
			ProductVersionLabel string    `json:"productVersionLabel"`
			Channels            *[]string `json:"channels"`
			Basis               string    `json:"basis"`
			EffectiveStartsAt   string    `json:"effectiveStartsAt"`
			EffectiveEndsAt     string    `json:"effectiveEndsAt"`
			RegisteredAt        string    `json:"registeredAt"`
		} `json:"mappings"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Outcome != "PRODUCT_CHANNEL_MAPPINGS_LISTED" || len(body.Mappings) != 2 {
		t.Fatalf("body = %+v", body)
	}
	configured := body.Mappings[0]
	if configured.MappingID != "map-1" || configured.Revision != 2 ||
		configured.ProductObjectID != "product-1" || configured.ProductVersionLabel != "v1" ||
		configured.Basis != "basis-map-1" ||
		configured.EffectiveStartsAt != "2026-08-28T09:00:00Z" ||
		configured.EffectiveEndsAt != "2027-08-28T09:00:00Z" ||
		configured.RegisteredAt != "2026-08-28T09:00:00Z" {
		t.Fatalf("configured mapping = %+v", configured)
	}
	if configured.Channels == nil || len(*configured.Channels) != 2 ||
		(*configured.Channels)[0] != "CH-SG-POST" {
		t.Fatalf("configured channels = %+v", configured.Channels)
	}
	unconfigured := body.Mappings[1]
	if unconfigured.MappingID != "map-2" || unconfigured.EffectiveEndsAt != "" {
		t.Fatalf("unconfigured mapping = %+v", unconfigured)
	}
	if unconfigured.Channels == nil || len(*unconfigured.Channels) != 0 {
		t.Fatalf("未配置绑定的 channels 必须是空数组而非 null：%+v", unconfigured.Channels)
	}
}

// Covers: 空目录答空数组（ADR-0077 Decision 四——空册是内容，不是 null）。
func TestProductChannelMappingsEndpointAnswersEmptyCataloguesAsContent(t *testing.T) {
	reader := &productChannelMappingReaderDouble{
		tenant: catValue(t, domain.NewTenantID, "tenant-1"),
	}
	endpoint := commercialhttp.NewQueryProductChannelMappingsEndpoint(
		intakeDouble{query: catalogueQuery(t)}, reader)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	raw, has := body["mappings"]
	if !has || string(raw) == "null" {
		t.Fatalf("mappings = %s，空册要答空数组", raw)
	}
}

// Covers: 方法门答 405；读失败答 5xx——读不回是答案未形成，不是空目录；未配置
// Intake 的拒绝原样转写（判据同其他目录端点）。
func TestProductChannelMappingsEndpointGuardsMethodIntakeAndReadFailures(t *testing.T) {
	failing := &productChannelMappingReaderDouble{loadErr: errors.New("db down")}
	endpoint := commercialhttp.NewQueryProductChannelMappingsEndpoint(
		intakeDouble{query: catalogueQuery(t)}, failing)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("read failure status = %d（读不回不是空册）", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	commercialhttp.NewQueryProductChannelMappingsEndpoint(
		intakeDouble{err: commercialhttp.ErrAccessChannelNotConfigured}, failing,
	).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("intake 未配置 status = %d, want 403", recorder.Code)
	}
}
