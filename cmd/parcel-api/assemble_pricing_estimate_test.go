package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件钉 `/pricing-estimates` 换上操作者渠道之后装配点那一半（ADR-0152 决定六；票 operator-channel/06）。它的能力面是主数据
// 与运营查阅读，所以不并进 swappedRegistryFaces：TestSwappedRegistryFacesMapTheRefusalAndDependencyGrades 里「只持查阅授予」
// 答未授予的那一格，在这一口恰是授予齐备。

// estimateDeclaration 与 syntheticPriceCardRegistration 那张卡对得上：同范围、同方向、适用期内，分区与重量落在它的费率段里。
const estimateDeclaration = `{"scope":"SYN-PRC-REG-SCOPE","direction":"SELL","basisAt":"2026-08-07T10:00:00Z","weight":{"value":"5","unit":"KG"},"zone":"Z1"}`

// Covers: ADR-0152 决定六——试算口的各格：未带令牌 401，只持登记册配置写 403，操作者册读不动 503，持查阅读而载荷自报租户
// 400；声明立得住时越过 Intake 到编排（未接线占位答 NO_ANSWER_FORMED）。发行方未配置仍答 403 未配置，带不带令牌都一样。
func TestThePricingEstimateAnswersFromTheOperatorReadChannel(t *testing.T) {
	cases := map[string]struct {
		register accessidentity.OperatorRegistry
		token    string
		body     string
		status   int
		code     string
	}{
		"no bearer token":               {readerRegister{}, "", estimateDeclaration, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		"registry write grant only":     {registryWriterRegister{}, "presented.operator.token", estimateDeclaration, http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		"register is down":              {failingRegister{}, "presented.operator.token", estimateDeclaration, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
		"granted, tenant self-reported": {readerRegister{}, "presented.operator.token", `{"tenantId": "SYN-TENANT-02"}`, http.StatusBadRequest, "MALFORMED_REQUEST"},
		"granted":                       {readerRegister{}, "presented.operator.token", estimateDeclaration, http.StatusInternalServerError, "NO_ANSWER_FORMED"},
	}
	for name, testCase := range cases {
		response := serveThroughOperatorRegistries(t, testCase.register, http.MethodPost, "/pricing-estimates", testCase.token, testCase.body)
		if response.Code != testCase.status || problemCode(t, response) != testCase.code {
			t.Fatalf("%s: answer = %d %q, want %d %q", name, response.Code, problemCode(t, response), testCase.status, testCase.code)
		}
		assertNoOutcome(t, response, name)
	}

	router := httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnconfiguredBusinessEndpoints())
	for _, token := range []string{"", "presented.operator.token"} {
		request := httptest.NewRequest(http.MethodPost, "/pricing-estimates", strings.NewReader(estimateDeclaration))
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("issuer unconfigured, token %q: answer = %d %q", token, response.Code, problemCode(t, response))
		}
	}
}

// tenantReader 是操作者册的替身：主体绑在 tenant 上，持有主数据与运营查阅读的授予。readerRegister 同形，只是钉死在
// SYN-TENANT-01 上。
type tenantReader struct{ tenant string }

func (register tenantReader) FindOperator(_ context.Context, subject accessidentity.OperatorSubject) (accessidentity.OperatorStanding, bool, error) {
	binding, err := accessidentity.NewOperatorBinding(subject, register.tenant, "SYN-BASIS-BINDING")
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	grant, err := accessidentity.NewOperatorGrant(register.tenant, "SYN-GRANT-READ", subject, accessidentity.CapabilityMasterDataAndOperationsRead, interval, "SYN-BASIS-GRANT")
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	recorded, err := accessidentity.NewRecordedGrant(grant, nil)
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	standing, err := accessidentity.NewOperatorStanding(binding, []accessidentity.RecordedGrant{recorded})
	return standing, err == nil, err
}

// Covers: ADR-0152 决定六与票 operator-channel/06——真铸造、PP 查阅读认证方、操作者 Intake 与真库价卡册上的试算编排连起来：
// 操作者认证到哪个租户，就按哪个租户的价卡试算。绑夹具价卡所在租户的操作者得到那张卡的一份 `S` 试算评价；绑另一个租户
// （readerRegister）的操作者递同一份声明，答价卡未配置——声明里没有租户的位置。测试输入是隔离合成，只记 `S`。
func TestTheOperatorReadChannelEstimatesForTheEnvelopeTenantAgainstARealDatabase(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	cards, err := buildPriceCardRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配价卡登记编排：%v", err)
	}
	registration := syntheticPriceCardRegistration(t)
	if outcome, err := cards.Handle(t.Context(), pricingapp.RegisterPriceCardCommand{Registration: registration}); err != nil || outcome != pricingapp.PriceCardRecorded {
		t.Fatalf("价卡首登 outcome = %s err = %v", outcome, err)
	}
	estimate, err := buildPricingEstimateOrchestration(db)
	if err != nil {
		t.Fatalf("装配试算编排：%v", err)
	}
	post := func(register accessidentity.OperatorRegistry) *httptest.ResponseRecorder {
		t.Helper()
		minter, err := buildOperatorMinter(acceptingVerifier{}, register)
		if err != nil {
			t.Fatal(err)
		}
		registries, err := buildOperatorRegistryIntakes(minter)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/pricing-estimates", strings.NewReader(estimateDeclaration))
		request.Header.Set("Authorization", "Bearer presented.operator.token")
		response := httptest.NewRecorder()
		pricinghttp.NewEstimateEndpoint(registries.pricingOperationsRead, estimate).ServeHTTP(response, request)
		return response
	}

	var formed struct {
		Outcome    string `json:"outcome"`
		Candidates []struct {
			Plan       struct{ ID, Version string } `json:"plan"`
			Answer     string                       `json:"answer"`
			Evaluation *struct {
				Status   string                             `json:"status"`
				Evidence string                             `json:"evidence"`
				Total    *struct{ Amount, Currency string } `json:"total"`
			} `json:"evaluation"`
		} `json:"candidates"`
	}
	response := post(tenantReader{tenant: registration.Tenant().String()})
	if err := json.Unmarshal(response.Body.Bytes(), &formed); err != nil || response.Code != http.StatusOK ||
		formed.Outcome != "FORMED" || len(formed.Candidates) != 1 {
		t.Fatalf("绑价卡所在租户的操作者 = %d %s", response.Code, response.Body.Bytes())
	}
	candidate := formed.Candidates[0]
	if candidate.Answer != "EVALUATED" || candidate.Plan.ID != "SYN-PRC-REG-PLAN" || candidate.Plan.Version != "v1" ||
		candidate.Evaluation == nil || candidate.Evaluation.Status != "COMPLETED" || candidate.Evaluation.Evidence != "S" ||
		candidate.Evaluation.Total == nil || candidate.Evaluation.Total.Amount != "10" || candidate.Evaluation.Total.Currency != "USD" {
		t.Fatalf("试算那一格 = %s", response.Body.Bytes())
	}

	if other := post(readerRegister{}); other.Code != http.StatusOK || answeredOutcome(t, other) != "PRICE_CARD_NOT_CONFIGURED" {
		t.Fatalf("绑另一个租户的操作者 = %d %s", other.Code, other.Body.Bytes())
	}
}
