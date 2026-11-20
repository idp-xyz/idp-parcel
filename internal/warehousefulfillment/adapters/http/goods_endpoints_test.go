package warehousehttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

const goodsOnBehalfPath = "/warehouse-fulfillment-goods-on-behalf"

type fixedGoodsAuthenticator struct {
	identity warehousehttp.OperatorIdentity
}

func (authenticator fixedGoodsAuthenticator) AuthenticateGoodsOnBehalf(context.Context, string) (warehousehttp.OperatorIdentity, error) {
	return authenticator.identity, nil
}

type countingGoodsRegistrar struct {
	calls int
	err   error
}

func (registrar *countingGoodsRegistrar) Register(context.Context, application.RegisterGoodsCommand) (application.RegisterGoodsResult, error) {
	registrar.calls++
	return application.RegisterGoodsResult{}, registrar.err
}

func postGoods(handler http.Handler, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, goodsOnBehalfPath, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func goodsEndpoint(t *testing.T, registrar *countingGoodsRegistrar) http.Handler {
	t.Helper()
	intake, err := warehousehttp.NewOperatorGoodsIntake(fixedGoodsAuthenticator{identity: operatorIdentity(t, tenantA, operatorOne)})
	if err != nil {
		t.Fatal(err)
	}
	return warehousehttp.NewRegisterGoodsOnBehalfEndpoint(intake, registrar)
}

const goodsBodyMissingEvidence = `{
	"ownerAccount": "SYN-OWNER-01",
	"decisionId": "SYN-DECISION-GOODS-01",
	"goodsCode": "SYN-GOODS-01",
	"name": "合成商品",
	"baseUnit": "件"
}`

// Covers: 代录缺依据引用是坏载荷，登记编排不被调用。货主客户账户在本租户查不到答 404，不带 outcome。
// 读依赖失败答 500，正文不带回错的原文。
func TestGoodsOnBehalfKeepsAMissingEvidenceOffTheRegisterAndAnswersAnUnknownOwnerAsNotVisible(t *testing.T) {
	registrar := &countingGoodsRegistrar{}
	missing := postGoods(goodsEndpoint(t, registrar), goodsBodyMissingEvidence)
	detail := assertProblem(t, "missing evidence", missing, http.StatusBadRequest, "MALFORMED_REQUEST")
	if registrar.calls != 0 {
		t.Fatalf("缺依据引用仍进了登记编排 %d 次", registrar.calls)
	}
	if reason := stringMember(t, detail, "detail"); !strings.Contains(reason, "evidenceReference") {
		t.Fatalf("detail = %q，want 点名 evidenceReference", reason)
	}

	registrar.err = application.ErrOwnerAccountNotInTenant
	complete := strings.TrimRight(strings.TrimSpace(goodsBodyMissingEvidence), "}") + `, "evidenceReference": "SYN-EVIDENCE-01"}`
	unknown := postGoods(goodsEndpoint(t, registrar), complete)
	assertProblem(t, "owner not visible", unknown, http.StatusNotFound, "OWNER_ACCOUNT_NOT_VISIBLE")
	if registrar.calls != 1 {
		t.Fatalf("货主不可见时登记编排被调用 %d 次，want 1", registrar.calls)
	}

	registrar.err = errors.New("party commercial down: SYN-EVIDENCE-01")
	failed := postGoods(goodsEndpoint(t, registrar), complete)
	assertProblem(t, "lookup down", failed, http.StatusInternalServerError, "NO_ANSWER_FORMED")
	if strings.Contains(failed.Body.String(), "SYN-EVIDENCE-01") || strings.Contains(failed.Body.String(), "party commercial") {
		t.Fatalf("500 带回了内部原文：%s", failed.Body.String())
	}
}

type silentGoodsReader struct{}

func (silentGoodsReader) ListGoods(context.Context, domain.TenantID, domain.OwnerAccountID, int, cataloguepage.Query) (cataloguepage.Result[ports.GoodsRow], error) {
	return cataloguepage.Result[ports.GoodsRow]{}, nil
}

func (silentGoodsReader) CurrentGoods(context.Context, domain.TenantID, domain.OwnerAccountID, domain.GoodsCode) (ports.GoodsRow, bool, error) {
	return ports.GoodsRow{}, false, nil
}

func (silentGoodsReader) GoodsRevisions(context.Context, domain.TenantID, domain.OwnerAccountID, domain.GoodsCode) ([]ports.GoodsRow, error) {
	return nil, nil
}

func (silentGoodsReader) FindGoods(context.Context, domain.TenantID, domain.OwnerAccountID, domain.IdentifierCode) (ports.GoodsRow, bool, error) {
	return ports.GoodsRow{}, false, nil
}

// Covers: 运营查阅找不到时不答 404：当前修订 found 为假，修订史是空数组，按码找 found 为假，列表是空数组。
func TestGoodsReadsAnswerNotFoundAsAnEmptyResult(t *testing.T) {
	intake, err := warehousehttp.NewOperatorOperationsReadIntake(&fakeReadAuthenticator{tenant: valueOf(t, domain.NewTenantID, tenantA)})
	if err != nil {
		t.Fatal(err)
	}
	reader := silentGoodsReader{}
	cases := []struct {
		name    string
		handler http.Handler
		target  string
		outcome string
		keys    []string
	}{
		{"list", warehousehttp.NewQueryGoodsEndpoint(intake, reader), "/warehouse-fulfillment-goods?ownerAccount=SYN-OWNER-01", "GOODS_LISTED", []string{"outcome", "ownerAccount", "goods", "page"}},
		{"current", warehousehttp.NewQueryCurrentGoodsEndpoint(intake, reader), "/warehouse-fulfillment-goods-current?ownerAccount=SYN-OWNER-01&goodsCode=SYN-GOODS-01", "GOODS_CURRENT", []string{"outcome", "ownerAccount", "goodsCode", "found"}},
		{"revisions", warehousehttp.NewQueryGoodsRevisionsEndpoint(intake, reader), "/warehouse-fulfillment-goods-revisions?ownerAccount=SYN-OWNER-01&goodsCode=SYN-GOODS-01", "GOODS_REVISIONS_LISTED", []string{"outcome", "ownerAccount", "goodsCode", "revisions"}},
		{"by code", warehousehttp.NewFindGoodsEndpoint(intake, reader), "/warehouse-fulfillment-goods-by-code?ownerAccount=SYN-OWNER-01&code=SYN-GOODS-01", "GOODS_FOUND", []string{"outcome", "ownerAccount", "code", "found"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			response := get(testCase.handler, testCase.target)
			if response.Code != http.StatusOK {
				t.Fatalf("状态 %d %s，want 200", response.Code, response.Body.String())
			}
			object := decodeObject(t, response)
			assertKeys(t, testCase.name, object, testCase.keys...)
			if stringMember(t, object, "outcome") != testCase.outcome {
				t.Fatalf("outcome = %s，want %s", object["outcome"], testCase.outcome)
			}
			switch testCase.name {
			case "current", "by code":
				if string(object["found"]) != "false" {
					t.Fatalf("found = %s，want false", object["found"])
				}
			case "revisions":
				if string(object["revisions"]) != "[]" {
					t.Fatalf("revisions = %s，want []", object["revisions"])
				}
			case "list":
				if string(object["goods"]) != "[]" {
					t.Fatalf("goods = %s，want []", object["goods"])
				}
			}
		})
	}
}
