package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	pricingdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: 价卡内容读口（GET /pricing-price-cards/content）经操作者渠道走到真库——生产铸造器、定价认证适配器、操作者
// Intake、价卡仓储连起来，只有令牌校验与操作者册是替身（那位合成操作者绑 SYN-TENANT-01）。证：持「主数据与运营查阅读」
// 授予的操作者读回演示种子卡登记的文档；只持登记写授予的操作者答未授予；无令牌答凭据被拒；租户只取信封，带租户参数是
// 坏请求；别的租户登的卡对这位操作者答查无。
func TestPriceCardContentEndpointReadsFromTheRealRegisterThroughTheOperatorChannel(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	cards, err := pppostgres.NewPriceCards(db)
	if err != nil {
		t.Fatal(err)
	}

	seeds := filepath.Join("..", "..", "scripts", "demo-seeds", "data", "pricing")
	read := func(name string) []byte {
		raw, err := os.ReadFile(filepath.Join(seeds, name))
		if err != nil {
			t.Fatalf("读演示种子 %s：%v", name, err)
		}
		return raw
	}
	own := read("price-card-cn-sg-full-03.json")
	ownRegistration, err := pricingdomain.RehydratePriceCardRegistration(own)
	if err != nil {
		t.Fatal(err)
	}
	// 别的租户登的卡：同一份种子去掉 tenant 键、按另一个租户重建——快照里不带租户这一格，租户由调用方给。
	var asDocument map[string]json.RawMessage
	if err := json.Unmarshal(read("price-card-cn-sg-cost.json"), &asDocument); err != nil {
		t.Fatal(err)
	}
	delete(asDocument, "tenant")
	withoutTenant, err := json.Marshal(asDocument)
	if err != nil {
		t.Fatal(err)
	}
	otherRegistration, err := pricingdomain.RehydratePriceCardRegistrationForTenant(withoutTenant, mustValue(t, pricingdomain.NewTenantID, "SYN-TENANT-OTHER"))
	if err != nil {
		t.Fatal(err)
	}
	for _, registration := range []pricingdomain.PriceCardRegistration{ownRegistration, otherRegistration} {
		if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
			_, err := cards.Register(ctx, registration)
			return err
		}); err != nil {
			t.Fatalf("登记演示价卡：%v", err)
		}
	}

	minterFor := func(register accessidentity.OperatorRegistry) operatorRegistryIntakes {
		t.Helper()
		minter, err := buildOperatorMinter(acceptingVerifier{}, register, admissionScopeUnset())
		if err != nil {
			t.Fatal(err)
		}
		intakes, err := buildOperatorRegistryIntakes(minter)
		if err != nil {
			t.Fatal(err)
		}
		return intakes
	}
	serve := func(intakes operatorRegistryIntakes, target, token string) *httptest.ResponseRecorder {
		t.Helper()
		var handler http.Handler = pricinghttp.NewQueryPriceCardContentEndpoint(pricinghttp.UnconfiguredIntake{}, cards)
		if intakes.pricingOperationsRead != nil {
			handler = pricinghttp.NewQueryPriceCardContentEndpoint(intakes.pricingOperationsRead, cards)
		}
		request := httptest.NewRequest(http.MethodGet, target, nil)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	granted := minterFor(operationsReaderRegister{})
	ownTarget := "/pricing-price-cards/content?id=SYN-PLAN-CN-SG-FULL-03&version=v1"

	response := serve(granted, ownTarget, "presented.operator.token")
	if response.Code != http.StatusOK {
		t.Fatalf("授予齐备的操作者：status = %d body=%s", response.Code, response.Body.String())
	}
	var found struct {
		Outcome  string          `json:"outcome"`
		Document json.RawMessage `json:"document"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &found); err != nil || found.Outcome != "PRICE_CARD_CONTENT_FOUND" {
		t.Fatalf("outcome = %q err=%v body=%s", found.Outcome, err, response.Body.String())
	}
	var seedPlan struct {
		Plan any `json:"plan"`
	}
	if err := json.Unmarshal(own, &seedPlan); err != nil {
		t.Fatal(err)
	}
	var served any
	if err := json.Unmarshal(found.Document, &served); err != nil || !reflect.DeepEqual(served, seedPlan.Plan) {
		t.Fatalf("读回的文档与种子登记的不同：%s", found.Document)
	}

	// 别的租户登的成本卡：对这位操作者是「没有」，不是「无权」，也不是它的内容。
	response = serve(granted, "/pricing-price-cards/content?id=SYN-PLAN-CN-SG-COST-01&version=v1", "presented.operator.token")
	if response.Code != http.StatusOK || decodeOutcome(t, response) != "NOT_FOUND" {
		t.Fatalf("别的租户的卡：%d %s", response.Code, response.Body.String())
	}

	// 租户只取信封：带租户参数是坏请求，哪怕指的就是操作者自己的租户。
	response = serve(granted, ownTarget+"&tenant=SYN-TENANT-01", "presented.operator.token")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("带租户参数：status = %d", response.Code)
	}

	response = serve(granted, ownTarget, "")
	if response.Code != http.StatusUnauthorized || problemCode(t, response) != "OPERATOR_CREDENTIAL_REJECTED" {
		t.Fatalf("无令牌：%d %s", response.Code, response.Body.String())
	}
	response = serve(minterFor(registryWriterRegister{}), ownTarget, "presented.operator.token")
	if response.Code != http.StatusForbidden || problemCode(t, response) != "OPERATOR_NOT_GRANTED" {
		t.Fatalf("只持登记写授予：%d %s", response.Code, response.Body.String())
	}
	response = serve(unconfiguredOperatorRegistries(), ownTarget, "presented.operator.token")
	if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("渠道未配置：%d %s", response.Code, response.Body.String())
	}
}

func decodeOutcome(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON：%v\n%s", err, response.Body.String())
	}
	return body.Outcome
}
