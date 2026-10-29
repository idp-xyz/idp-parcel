package pricinghttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件对价卡内容查阅端点（GET /pricing-price-cards/content）证传输面：方法门、接入拒绝先于一切、租户只来自信封、
// 查询串封闭、答出的文档与读口交来的逐字节相同、查无与「本构建读不了」各答其格、读失败不伪装成查无。

type contentReaderDouble struct {
	tenant  domain.TenantID
	content ports.PriceCardContent
	err     error

	calls       int
	askedTenant domain.TenantID
	asked       domain.VersionReference
}

func (double *contentReaderDouble) ReadPriceCardContent(
	_ context.Context,
	tenant domain.TenantID,
	reference domain.VersionReference,
) (ports.PriceCardContent, bool, error) {
	double.calls++
	double.askedTenant, double.asked = tenant, reference
	if double.err != nil {
		return ports.PriceCardContent{}, false, double.err
	}
	// 读口自己按租户隔离：别的租户问同一个引用得到的是「没有」，与真不存在不可分辨。
	if tenant != double.tenant || reference.ID() != double.content.PlanID || reference.Version() != double.content.PlanVersion {
		return ports.PriceCardContent{}, false, nil
	}
	return double.content, true, nil
}

// 文档故意写成非规整排版（多余空白、非字母序的键）：端点若重排或重新编码，逐字节比对就会掉。
var storedPlanDocument = json.RawMessage(`{"canonicalization":"PPC-6",  "reference":{"kind":"pricing-plan","id":"plan-1","version":"v1"},
	"direction":"SELL","totalChargeBounds":{"minimum":{"amount":{"coefficient":"12099","scale":2},"currency":"CNY"}}}`)

func newContentReader(t *testing.T) *contentReaderDouble {
	t.Helper()
	return &contentReaderDouble{
		tenant: catalogueQuery(t).Scope.Tenant(),
		content: ports.PriceCardContent{
			PlanID:              "plan-1",
			PlanVersion:         "v1",
			Canonicalization:    "PPC-6",
			ContentDigest:       "digest-1",
			PublicationApprover: "SYN-PRC-GOVERNANCE",
			RegisteredAt:        pricingBaseAt,
			Document:            storedPlanDocument,
		},
	}
}

func getContent(endpoint http.Handler, target string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

func TestPriceCardContentEndpointOnlyAcceptsGet(t *testing.T) {
	reader := newContentReader(t)
	endpoint := pricinghttp.NewQueryPriceCardContentEndpoint(intakeDouble{query: catalogueQuery(t)}, reader)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pricing-price-cards/content?id=plan-1&version=v1", nil))

	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status = %d, Allow = %q", recorder.Code, recorder.Header().Get("Allow"))
	}
	if reader.calls != 0 {
		t.Fatal("方法不对的请求越过了方法门")
	}
}

// Covers: 接入拒绝先于读口与查询串——未配置 403、凭据被拒 401、未授予 403、身份依赖不可用 503，读口一次都不被调到；
// 请求即便缺参数，也先答接入拒绝，不泄露本端点认哪些键。
func TestPriceCardContentEndpointRefusesBeforeReadingAnything(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"channel not configured", pricinghttp.ErrAccessChannelNotConfigured, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		{"credential rejected", pricinghttp.ErrOperatorCredentialRejected, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		{"operations read not granted", pricinghttp.ErrOperatorNotGranted, http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		{"identity dependency unavailable", pricinghttp.ErrIdentityDependencyUnavailable, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
	}
	for _, testCase := range cases {
		reader := newContentReader(t)
		endpoint := pricinghttp.NewQueryPriceCardContentEndpoint(intakeDouble{err: testCase.err}, reader)
		recorder := getContent(endpoint, "/pricing-price-cards/content")
		if recorder.Code != testCase.status || errorCode(t, recorder) != testCase.code {
			t.Fatalf("%s: answer = %d %q, want %d %q", testCase.name, recorder.Code, errorCode(t, recorder), testCase.status, testCase.code)
		}
		if reader.calls != 0 {
			t.Fatalf("%s: 接入被拒的请求调到了读口", testCase.name)
		}
	}

	unconfigured := pricinghttp.NewQueryPriceCardContentEndpoint(pricinghttp.UnconfiguredIntake{}, newContentReader(t))
	recorder := getContent(unconfigured, "/pricing-price-cards/content?id=plan-1&version=v1")
	if recorder.Code != http.StatusForbidden || errorCode(t, recorder) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("未配置即拒的 Intake 应答 403：%d %s", recorder.Code, recorder.Body.String())
	}
}

// Covers: 租户只取认证信封、不收租户参数——任何租户样的键、未知键、重复键、缺键、空值与带首尾空白的值都是坏请求 400，
// 读口一次都不被调到（带一个 tenant 来就是以为自己能指定租户，装没看见比拒更坏）。
func TestPriceCardContentEndpointDecodesTheQueryClosedAndNeverTakesATenant(t *testing.T) {
	for _, target := range []string{
		"/pricing-price-cards/content?id=plan-1&version=v1&tenant=tenant-2",
		"/pricing-price-cards/content?id=plan-1&version=v1&tenantId=tenant-2",
		"/pricing-price-cards/content?id=plan-1&version=v1&limit=5",
		"/pricing-price-cards/content?id=plan-1",
		"/pricing-price-cards/content?version=v1",
		"/pricing-price-cards/content",
		"/pricing-price-cards/content?id=plan-1&id=plan-2&version=v1",
		"/pricing-price-cards/content?id=plan-1&version=v1&version=v2",
		"/pricing-price-cards/content?id=&version=v1",
		"/pricing-price-cards/content?id=plan-1&version=",
		"/pricing-price-cards/content?id=%20plan-1&version=v1",
		"/pricing-price-cards/content?id=plan-1;version=v1",
	} {
		reader := newContentReader(t)
		endpoint := pricinghttp.NewQueryPriceCardContentEndpoint(intakeDouble{query: catalogueQuery(t)}, reader)
		recorder := getContent(endpoint, target)
		if recorder.Code != http.StatusBadRequest || errorCode(t, recorder) != "MALFORMED_REQUEST" {
			t.Fatalf("%s: answer = %d %s, want 400 MALFORMED_REQUEST", target, recorder.Code, recorder.Body.String())
		}
		if reader.calls != 0 {
			t.Fatalf("%s: 坏请求调到了读口", target)
		}
	}
}

// Covers: 租户只来自信封；读口收到的租户是认证交回的那一个，引用是查询串里指名的那一版。
func TestPriceCardContentEndpointReadsForTheEnvelopeTenantOnly(t *testing.T) {
	query := catalogueQuery(t)
	reader := newContentReader(t)
	endpoint := pricinghttp.NewQueryPriceCardContentEndpoint(intakeDouble{query: query}, reader)
	if recorder := getContent(endpoint, "/pricing-price-cards/content?id=plan-1&version=v1"); recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if reader.askedTenant != query.Scope.Tenant() {
		t.Fatalf("读口收到租户 %q，想要信封里的 %q", reader.askedTenant, query.Scope.Tenant())
	}
	if reader.asked.Kind() != domain.ArtifactPricingPlan || reader.asked.ID() != "plan-1" || reader.asked.Version() != "v1" {
		t.Fatalf("读口收到的引用 = %s/%s/%s", reader.asked.Kind(), reader.asked.ID(), reader.asked.Version())
	}
}

// Covers: 答出文档逐字等于登记的文档——端点不重排、不重新编码；元数据逐字段转写。
func TestPriceCardContentEndpointAnswersTheRegisteredDocumentVerbatim(t *testing.T) {
	reader := newContentReader(t)
	endpoint := pricinghttp.NewQueryPriceCardContentEndpoint(intakeDouble{query: catalogueQuery(t)}, reader)
	recorder := getContent(endpoint, "/pricing-price-cards/content?id=plan-1&version=v1")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	var envelope struct {
		Outcome             string          `json:"outcome"`
		PlanID              string          `json:"planId"`
		PlanVersion         string          `json:"planVersion"`
		Canonicalization    string          `json:"canonicalization"`
		ContentDigest       string          `json:"contentDigest"`
		PublicationApprover string          `json:"publicationApprover"`
		RegisteredAt        string          `json:"registeredAt"`
		Document            json.RawMessage `json:"document"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Outcome != "PRICE_CARD_CONTENT_FOUND" || envelope.PlanID != "plan-1" || envelope.PlanVersion != "v1" ||
		envelope.Canonicalization != "PPC-6" || envelope.ContentDigest != "digest-1" ||
		envelope.PublicationApprover != "SYN-PRC-GOVERNANCE" || envelope.RegisteredAt != "2026-08-25T09:00:00Z" {
		t.Fatalf("元数据 = %+v", envelope)
	}
	// 编码器写回 RawMessage 时会压掉空白，所以比对用压缩后的字节；键序与取值逐字节不变。
	var compact bytes.Buffer
	if err := json.Compact(&compact, storedPlanDocument); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(envelope.Document, compact.Bytes()) {
		t.Fatalf("document 与登记的不同：\n got %s\nwant %s", envelope.Document, compact.Bytes())
	}
}

// Covers: 查无答 200 + NOT_FOUND（ADR-0022），含别的租户的卡与未登过的版本；两者同答，回显所问。
func TestPriceCardContentEndpointAnswersNotFoundAsABusinessOutcome(t *testing.T) {
	query := catalogueQuery(t)
	otherTenantReader := newContentReader(t)
	otherTenantReader.tenant = pricingValue(t, domain.NewTenantID, "tenant-2")
	for name, probe := range map[string]struct {
		reader *contentReaderDouble
		target string
	}{
		"unregistered version":  {newContentReader(t), "/pricing-price-cards/content?id=plan-1&version=v9"},
		"unregistered plan":     {newContentReader(t), "/pricing-price-cards/content?id=plan-9&version=v1"},
		"another tenant's card": {otherTenantReader, "/pricing-price-cards/content?id=plan-1&version=v1"},
	} {
		endpoint := pricinghttp.NewQueryPriceCardContentEndpoint(intakeDouble{query: query}, probe.reader)
		recorder := getContent(endpoint, probe.target)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, 查无是形成了的业务答案", name, recorder.Code)
		}
		body := decodeBody(t, recorder)
		if body["outcome"] != "NOT_FOUND" {
			t.Fatalf("%s: outcome = %v", name, body["outcome"])
		}
		if _, has := body["document"]; has {
			t.Fatalf("%s: 查无的答复里长出了 document", name)
		}
	}
}

// Covers: 册上有、本构建重建不了（另一套规范化版本）自成一格，不折成查无——续办是等支持该形状的构建，不是去登记。
func TestPriceCardContentEndpointSeparatesAnUnreadableShapeFromAbsence(t *testing.T) {
	reader := newContentReader(t)
	reader.err = fmt.Errorf("plan-1/v1：%w", domain.ErrCanonicalizationVersionUnsupported)
	endpoint := pricinghttp.NewQueryPriceCardContentEndpoint(intakeDouble{query: catalogueQuery(t)}, reader)
	recorder := getContent(endpoint, "/pricing-price-cards/content?id=plan-1&version=v1")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	body := decodeBody(t, recorder)
	if body["outcome"] != "CANONICALIZATION_UNSUPPORTED" || body["planId"] != "plan-1" || body["planVersion"] != "v1" {
		t.Fatalf("body = %v", body)
	}
}

// Covers: 读不回（含重建门拒了这一行）是答案未形成 500，不是查无——后者会让一次该重试的故障长得像「没登过这张卡」。
func TestPriceCardContentEndpointDoesNotDisguiseAReadFailureAsAbsence(t *testing.T) {
	reader := newContentReader(t)
	reader.err = errors.New("snapshot rejected by the rehydration gate")
	endpoint := pricinghttp.NewQueryPriceCardContentEndpoint(intakeDouble{query: catalogueQuery(t)}, reader)
	recorder := getContent(endpoint, "/pricing-price-cards/content?id=plan-1&version=v1")
	if recorder.Code != http.StatusInternalServerError || errorCode(t, recorder) != "NO_ANSWER_FORMED" {
		t.Fatalf("answer = %d %s", recorder.Code, recorder.Body.String())
	}
}
