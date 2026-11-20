package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/pricecardtemplate"
	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	pricingdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: 演示租户的完整合成价卡 FULL-03 从真库读回成工作簿，再导入，规范化版本与内容摘要与登记的那一版相同。
// 结构化工作簿与电子表格两种载体都走这一道。表达不了的卡不在这张种子上——它的引用只有三元。
func TestRegisteredFull03ReadsBackAsAWorkbookThatImportsToTheSameDigest(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	cards, err := pppostgres.NewPriceCards(db)
	if err != nil {
		t.Fatal(err)
	}
	drafts, err := pppostgres.NewPriceCardDrafts(db)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := pricingapp.NewReadPriceCardWorkbookHandler(cards, drafts, pricecardtemplate.Reader{})
	if err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join("..", "..", "scripts", "demo-seeds", "data", "pricing", "price-card-cn-sg-full-03.json"))
	if err != nil {
		t.Fatal(err)
	}
	registration, err := pricingdomain.RehydratePriceCardRegistration(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, err := cards.Register(ctx, registration)
		return err
	}); err != nil {
		t.Fatalf("登记演示价卡：%v", err)
	}

	minter, err := buildOperatorMinter(acceptingVerifier{}, operationsReaderRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	intakes, err := buildOperatorRegistryIntakes(minter)
	if err != nil {
		t.Fatal(err)
	}
	serve := func(target string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, target, nil)
		request.Header.Set("Authorization", "Bearer presented.operator.token")
		response := httptest.NewRecorder()
		pricinghttp.NewQueryPriceCardWorkbookEndpoint(intakes.pricingOperationsRead, handler).ServeHTTP(response, request)
		return response
	}

	jsonResponse := serve("/pricing-price-cards/workbook?id=SYN-PLAN-CN-SG-FULL-03&version=v1")
	if jsonResponse.Code != http.StatusOK {
		t.Fatalf("结构化工作簿：%d %s", jsonResponse.Code, jsonResponse.Body.String())
	}
	var found struct {
		Outcome          string          `json:"outcome"`
		Canonicalization string          `json:"canonicalization"`
		ContentDigest    string          `json:"contentDigest"`
		Workbook         json.RawMessage `json:"workbook"`
	}
	if err := json.Unmarshal(jsonResponse.Body.Bytes(), &found); err != nil || found.Outcome != "WORKBOOK" {
		t.Fatalf("outcome %s err %v body %s", found.Outcome, err, jsonResponse.Body.String())
	}
	if found.ContentDigest != registration.Plan().ContentDigest() || found.Canonicalization != registration.Plan().CanonicalizationVersion() {
		t.Fatalf("读口摘要 %s（%s），登记 %s（%s）", found.ContentDigest, found.Canonicalization,
			registration.Plan().ContentDigest(), registration.Plan().CanonicalizationVersion())
	}
	again := pricecardtemplate.Reader{}.ReadPriceCardWorkbook(found.Workbook)
	if again.Content == nil || again.Content.Plan.ContentDigest() != registration.Plan().ContentDigest() ||
		again.Content.Plan.CanonicalizationVersion() != registration.Plan().CanonicalizationVersion() {
		t.Fatalf("再导入摘要 %v，问题 %v", again.Content, again.Problems)
	}

	sheetResponse := serve("/pricing-price-cards/workbook?id=SYN-PLAN-CN-SG-FULL-03&version=v1&carrier=spreadsheet")
	if sheetResponse.Code != http.StatusOK {
		t.Fatalf("电子表格：%d %s", sheetResponse.Code, sheetResponse.Body.String())
	}
	fromSheet := pricecardtemplate.Reader{}.ReadPriceCardTemplate(sheetResponse.Body.Bytes())
	if fromSheet.Content == nil || fromSheet.Content.Plan.ContentDigest() != registration.Plan().ContentDigest() ||
		fromSheet.Content.Plan.CanonicalizationVersion() != registration.Plan().CanonicalizationVersion() {
		t.Fatalf("电子表格再导入摘要 %v，问题 %v", fromSheet.Content, fromSheet.Problems)
	}
}
