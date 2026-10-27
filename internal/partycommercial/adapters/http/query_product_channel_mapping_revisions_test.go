package commercialhttp_test

import (
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

// 本文件对产品—渠道映射修订历史端点（票 party-commercial-uiux-backend/02）证传输面：整条修订链逐笔转写、顶层回显映射标识、
// 不在册答 200 加空数组、方法门、路径参数缺失答 400、读失败答 5xx。装配后的真路由由 cmd/parcel-api 的隔离读用例再证。

func mappingRevisionsRequest(mapping string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/commercial-product-channel-mappings/"+mapping+"/revisions", nil)
	request.SetPathValue("mappingId", mapping)
	return request
}

// Covers: AT-PC-067——一个映射的整条修订链按给的次序逐笔透出（绑定从哪几个渠道换到哪几个、区间改过几次），每一笔与目录行同形；
// 顶层回显映射标识；显式“未配置”的那一笔 channels 是空数组而非 null。
func TestProductChannelMappingRevisionsEndpointTranscribesTheWholeChain(t *testing.T) {
	reader := &productChannelMappingReaderDouble{
		tenant: catValue(t, domain.NewTenantID, "tenant-1"),
		revisions: map[string][]ports.ProductChannelMappingRow{
			"map-1": {
				{TenantID: "tenant-1", MappingID: "map-1", Revision: 1, ProductObjectID: "product-1", ProductVersionLabel: "v1",
					Channels: []string{"CH-SG-POST"}, Basis: "basis-1", EffectiveStartsAt: mappingListedAt, RegisteredAt: mappingListedAt},
				{TenantID: "tenant-1", MappingID: "map-1", Revision: 2, ProductObjectID: "product-1", ProductVersionLabel: "v1",
					Channels: nil, Basis: "basis-2", EffectiveStartsAt: mappingListedAt, EffectiveEndsAt: mappingListedAt.AddDate(1, 0, 0),
					HasEffectiveEnd: true, RegisteredAt: mappingListedAt.Add(time.Hour)},
			},
		},
	}
	endpoint := commercialhttp.NewQueryProductChannelMappingRevisionsEndpoint(intakeDouble{query: catalogueQuery(t)}, reader)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, mappingRevisionsRequest("map-1"))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	var body struct {
		Outcome   string `json:"outcome"`
		MappingID string `json:"mappingId"`
		Revisions []struct {
			Revision          int       `json:"revision"`
			Channels          *[]string `json:"channels"`
			Basis             string    `json:"basis"`
			EffectiveEndsAt   string    `json:"effectiveEndsAt"`
			RegisteredAt      string    `json:"registeredAt"`
			EffectiveStartsAt string    `json:"effectiveStartsAt"`
		} `json:"revisions"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Outcome != "PRODUCT_CHANNEL_MAPPING_REVISIONS_LISTED" || body.MappingID != "map-1" || len(body.Revisions) != 2 {
		t.Fatalf("body = %+v", body)
	}
	first, second := body.Revisions[0], body.Revisions[1]
	if first.Revision != 1 || first.Channels == nil || len(*first.Channels) != 1 || first.Basis != "basis-1" || first.EffectiveEndsAt != "" {
		t.Fatalf("第一笔 = %+v", first)
	}
	if second.Revision != 2 || second.Channels == nil || len(*second.Channels) != 0 || second.EffectiveEndsAt == "" {
		t.Fatalf("第二笔（显式未配置、区间有终点）= %+v", second)
	}
}

// Covers: 不在册的映射与跨租户都答 200 加空数组，不答 404、不泄露存在性。
func TestProductChannelMappingRevisionsEndpointAnswersAnUnknownMappingWithAnEmptyChain(t *testing.T) {
	reader := &productChannelMappingReaderDouble{tenant: catValue(t, domain.NewTenantID, "tenant-1")}
	endpoint := commercialhttp.NewQueryProductChannelMappingRevisionsEndpoint(intakeDouble{query: catalogueQuery(t)}, reader)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, mappingRevisionsRequest("map-nobody"))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(body["revisions"]) != "[]" || string(body["mappingId"]) != `"map-nobody"` {
		t.Fatalf("不在册的映射 = %s", recorder.Body)
	}
}

// Covers: 方法门、路径参数缺失、读失败各自的答复。
func TestProductChannelMappingRevisionsEndpointGuardsMethodPathAndReadFailure(t *testing.T) {
	endpoint := commercialhttp.NewQueryProductChannelMappingRevisionsEndpoint(
		intakeDouble{query: catalogueQuery(t)},
		&productChannelMappingReaderDouble{tenant: catValue(t, domain.NewTenantID, "tenant-1")})

	post := httptest.NewRecorder()
	postRequest := httptest.NewRequest(http.MethodPost, "/commercial-product-channel-mappings/map-1/revisions", nil)
	postRequest.SetPathValue("mappingId", "map-1")
	endpoint.ServeHTTP(post, postRequest)
	if post.Code != http.StatusMethodNotAllowed || post.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST = %d allow=%q", post.Code, post.Header().Get("Allow"))
	}

	noPath := httptest.NewRecorder()
	endpoint.ServeHTTP(noPath, httptest.NewRequest(http.MethodGet, "/commercial-product-channel-mappings//revisions", nil))
	if noPath.Code != http.StatusBadRequest {
		t.Fatalf("没有路径参数 = %d, want 400", noPath.Code)
	}

	failing := commercialhttp.NewQueryProductChannelMappingRevisionsEndpoint(
		intakeDouble{query: catalogueQuery(t)},
		&productChannelMappingReaderDouble{tenant: catValue(t, domain.NewTenantID, "tenant-1"), loadErr: errors.New("boom")})
	failed := httptest.NewRecorder()
	failing.ServeHTTP(failed, mappingRevisionsRequest("map-1"))
	if failed.Code != http.StatusInternalServerError {
		t.Fatalf("读失败 = %d, want 500（答案未形成，不能伪装成空历史）", failed.Code)
	}
}
