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
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件证索赔口在 parcel-api 装配点的几件（票 first-party-api/09、demo-chain-gaps/102 完成判据）：首方族未配置时答未配置、一个字节
// 不读；真库上经登记处里这一特性的生产 build 与 mount，合成客户端交索赔答受理，重放答已有项，他账户的目标与合同同答找不到，没有这一格
// 授予答未授予，不在准入范围答 OUTSIDE_ADMISSION_SCOPE，载荷自报账户答 400。合成主体、账户与包裹只记 `S`。

const (
	customerClaimPath  = "/v1/claims"
	claimOtherSubject  = "SYN-CUSTOMER-CLIENT-02"
	claimOtherAccount  = "SYN-ACCOUNT-02"
	claimParcel        = "SYN-PARCEL-FPA09-01"
	claimOwnContract   = "SYN-CONTRACT-FPA09-01"
	claimOtherContract = "SYN-CONTRACT-FPA09-02"
)

func claimBody(accessRequestID, item, contract, target string) string {
	encoded, err := json.Marshal(map[string]string{
		"accessRequestId": accessRequestID, "batch": "SYN-BATCH-FPA09-01", "item": item,
		"applicant": "SYN-APPLICANT-01", "contract": contract, "target": target, "kind": "LOSS",
	})
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func postClaim(endpoint http.Handler, token, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, customerClaimPath, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

// productionClaimLine 从端点登记处取索赔那一特性，经它自己的生产 build 与 mount 装出 /v1/claims 那一行。
func productionClaimLine(t *testing.T, sources endpointSources) http.Handler {
	t.Helper()
	for _, feature := range endpointRegistry {
		if feature.wiringKey != any((*claimsWiring)(nil)) {
			continue
		}
		mounted, err := feature.production(sources, endpointChannels{})
		if err != nil {
			t.Fatalf("装配：%v", err)
		}
		for _, line := range mounted {
			if line.Pattern == customerClaimPath {
				return line.Handler
			}
		}
	}
	t.Fatalf("%s 没按登记处挂上", customerClaimPath)
	return nil
}

// Covers: AT-VE-211 与票 demo-chain-gaps/102 完成判据「首方族三件不设时仍答 403 ACCESS_CHANNEL_NOT_CONFIGURED」——Intake 缺席（字面量
// 未配置）与三件都没设时的行答同一份字节，载荷一个字节没读。
func TestClaimLineAnswersNotConfiguredByteForByteWhenTheCustomerFamilyIsUnset(t *testing.T) {
	probe := &readProbe{}
	request := httptest.NewRequest(http.MethodPost, customerClaimPath, probe)
	request.Header.Set("Authorization", "Bearer any.presented.token")
	response := httptest.NewRecorder()
	visibilityhttp.NewReceiveClaimEndpoint(customerClaimIntake(nil), unwiredClaims{}).ServeHTTP(response, request)
	if !answersFirstPartyUnconfigured(t, response) {
		t.Fatalf("%d %q, want 403 %s", response.Code, response.Body.String(), firstPartyUnconfigured)
	}
	if probe.read {
		t.Fatal("首方族未配置却读了载荷")
	}
}

// Covers: AT-VE-208、AT-VE-209、AT-VE-210、AT-VE-211 与票 first-party-api/09 完成判据——真库上：客户渠道册登两个绑在不同账户上的合成客户端并
// 授索赔一格，准入范围对照与生产权威区间照种子的登法登好，两个账户各登一份合同责任范围声明，账户一有一件已接受委托的包裹。账户一的
// 客户端交索赔答 201 受理；同一接入请求重投答 200 已有结果、同一份项；账户二的客户端指名账户一的包裹、账户一的合同与指名不存在的包裹、
// 不存在的合同同答 404 同字节；载荷自报账户答 400；撤销授予之后答未授予；不在准入范围答 403。
func TestACustomerClientSubmitsAClaimThroughTheFirstPartyLineAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	const authority = "SYN-AUTH/idp-parcel-pilot"
	registerCustomerCommandAdmissionFor(t, db, authority, accessidentity.CustomerCapabilityClaimSubmission)
	registry, err := aipg.NewCustomerClientRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	registerPreAdviceClient(t, db, registry, syntheticCustomerSubject, syntheticCustomerAccount, map[accessidentity.CustomerCapability]string{
		accessidentity.CustomerCapabilityClaimSubmission: "SYN-CUSTOMER-GRANT-CLAIM-01",
	})
	registerPreAdviceClient(t, db, registry, claimOtherSubject, claimOtherAccount, map[accessidentity.CustomerCapability]string{
		accessidentity.CustomerCapabilityClaimSubmission: "SYN-CUSTOMER-GRANT-CLAIM-02",
	})

	// 账户一的包裹经真装配提交并接受；账户二没有任何包裹。
	command := trackingViewSubmissionCommand(t, claimParcel)
	submitOnRealAssembly(t, db, command)
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatal(err)
	}
	request, found, err := requests.FindBySourceIdentity(t.Context(), command.Identity)
	if err != nil || !found {
		t.Fatalf("读回委托：err=%v found=%v", err, found)
	}
	acceptAndSave(t, db, requests, command, request)

	registrar, err := vepostgres.NewCatalogRegistrar(db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := mustValue(t, vedomain.NewTenantID, "SYN-TENANT-01")
	for contract, account := range map[string]string{claimOwnContract: syntheticCustomerAccount, claimOtherContract: claimOtherAccount} {
		if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			outcome, err := registrar.RegisterClaimEligibility(txCtx, tenant, veports.ClaimEligibilityRegistration{
				Header:       veports.CatalogApprovalHeader{Version: "SYN-CLAIM-RULES-1", ApprovedBy: "SYN-OPERATOR-1"},
				Contract:     mustValue(t, vedomain.NewContractScopeReference, contract),
				Customer:     mustValue(t, vedomain.NewCustomerAccountReference, account),
				CoveredKinds: []vedomain.ClaimKindReference{mustValue(t, vedomain.NewClaimKindReference, "LOSS")},
			})
			if err == nil && outcome != veports.CatalogVersionRegistered {
				t.Errorf("登记索赔声明 %s = %s", contract, outcome)
			}
			return err
		}); err != nil {
			t.Fatalf("登记索赔声明 %s：%v", contract, err)
		}
	}

	issuer := newCustomerIssuerDouble(t)
	verifier, _, err := buildCustomerClientCredentialVerifier(fakeGetenv(issuer.environment()))
	if err != nil {
		t.Fatal(err)
	}
	admission, err := buildAdmissionScope(db, authority)
	if err != nil {
		t.Fatal(err)
	}
	minter, err := buildCustomerClientMinter(verifier, registry, admission)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := productionClaimLine(t, endpointSources{db: db, customerClientMinter: minter})
	owner := issuer.token(t, syntheticCustomerAudience, syntheticCustomerSubject)
	other := issuer.token(t, syntheticCustomerAudience, claimOtherSubject)

	countClaimItems := func() int {
		t.Helper()
		var rows int
		if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM visibility_exception.claim_item`).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		return rows
	}

	first := postClaim(endpoint, owner, claimBody("SYN-ACCESS-REQUEST-CL-0001", "SYN-ITEM-FPA09-01", claimOwnContract, claimParcel))
	if first.Code != http.StatusCreated || outcomeOf(t, first) != "CLAIM_RECEIVED" {
		t.Fatalf("交索赔 = %d %s, want 201 CLAIM_RECEIVED", first.Code, first.Body.String())
	}
	var rows int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM visibility_exception.claim_item
		  WHERE tenant_id = 'SYN-TENANT-01' AND customer_ref = $1 AND batch_ref = 'SYN-BATCH-FPA09-01' AND item_id = 'SYN-ITEM-FPA09-01'
		    AND applicant_ref = 'SYN-APPLICANT-01' AND contract_ref = $2 AND target_ref = $3`,
		syntheticCustomerAccount, claimOwnContract, claimParcel).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("册上的索赔项 %d 行（err %v），want 1 行、账户取自信封", rows, err)
	}

	replay := postClaim(endpoint, owner, claimBody("SYN-ACCESS-REQUEST-CL-0001", "SYN-ITEM-FPA09-01", claimOwnContract, claimParcel))
	if replay.Code != http.StatusOK || outcomeOf(t, replay) != "CLAIM_EXISTING_RESULT" {
		t.Fatalf("原样重投 = %d %s, want 200 CLAIM_EXISTING_RESULT", replay.Code, replay.Body.String())
	}
	var firstBody, replayBody struct {
		Claim struct{ Batch, Item, SubmittedAt string } `json:"claim"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstBody); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(replay.Body.Bytes(), &replayBody); err != nil || firstBody != replayBody || firstBody.Claim.Item == "" {
		t.Fatalf("重放读回的不是同一项：%s 对 %s", first.Body.String(), replay.Body.String())
	}
	if rows := countClaimItems(); rows != 1 {
		t.Fatalf("重放之后有 %d 项, want 1", rows)
	}

	// 账户二的客户端：账户一的包裹、账户一的合同，与不存在的包裹、不存在的合同，同答同字节。
	foreignTarget := postClaim(endpoint, other, claimBody("SYN-ACCESS-REQUEST-CL-0101", "SYN-ITEM-FPA09-02", claimOtherContract, claimParcel))
	absentTarget := postClaim(endpoint, other, claimBody("SYN-ACCESS-REQUEST-CL-0101", "SYN-ITEM-FPA09-02", claimOtherContract, "SYN-PARCEL-FPA09-NOWHERE"))
	foreignContract := postClaim(endpoint, other, claimBody("SYN-ACCESS-REQUEST-CL-0101", "SYN-ITEM-FPA09-02", claimOwnContract, claimParcel))
	absentContract := postClaim(endpoint, other, claimBody("SYN-ACCESS-REQUEST-CL-0101", "SYN-ITEM-FPA09-02", "SYN-CONTRACT-FPA09-NOWHERE", claimParcel))
	for name, response := range map[string]*httptest.ResponseRecorder{
		"他账户的目标": foreignTarget, "不存在的目标": absentTarget, "他账户的合同": foreignContract, "不存在的合同": absentContract,
	} {
		if response.Code != http.StatusNotFound || problemCode(t, response) != "CLAIM_REFERENCE_NOT_FOUND" {
			t.Fatalf("%s = %d %s, want 404 CLAIM_REFERENCE_NOT_FOUND", name, response.Code, response.Body.String())
		}
		assertNoOutcome(t, response, name)
		if withoutRequestID(t, response) != withoutRequestID(t, foreignTarget) {
			t.Fatalf("%s 与他账户的目标可分辨：%s 对 %s", name, response.Body.String(), foreignTarget.Body.String())
		}
	}
	if rows := countClaimItems(); rows != 1 {
		t.Fatalf("被拒的提交落了库：%d 项", rows)
	}

	// 载荷自报账户、租户：400。
	selfReported := strings.Replace(claimBody("SYN-ACCESS-REQUEST-CL-0201", "SYN-ITEM-FPA09-03", claimOwnContract, claimParcel),
		`"kind":"LOSS"`, `"kind":"LOSS","customer":"`+claimOtherAccount+`"`, 1)
	if declared := postClaim(endpoint, owner, selfReported); declared.Code != http.StatusBadRequest || problemCode(t, declared) != "MALFORMED_REQUEST" {
		t.Fatalf("载荷自报账户 = %d %s, want 400 MALFORMED_REQUEST", declared.Code, declared.Body.String())
	}

	revocation, err := accessidentity.NewGrantRevocation("SYN-TENANT-01", "SYN-CUSTOMER-GRANT-CLAIM-01", time.Now().Add(-time.Second), "SYN-CUSTOMER-REVOKE-BASIS-CL")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, err := registry.RegisterCustomerClientRevocation(ctx, revocation)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if refused := postClaim(endpoint, owner, claimBody("SYN-ACCESS-REQUEST-CL-0301", "SYN-ITEM-FPA09-04", claimOwnContract, claimParcel)); refused.Code != http.StatusForbidden || problemCode(t, refused) != "CUSTOMER_CLIENT_NOT_GRANTED" {
		t.Fatalf("撤销授予之后 = %d %s, want 403 CUSTOMER_CLIENT_NOT_GRANTED", refused.Code, refused.Body.String())
	}
}

// Covers: AT-VE-211——本产品的权威串没设时，持有授予的客户端在索赔口答不在准入范围（索赔提交在铸造侧判准入，ADR-0139 决定八）；
// 别族令牌答令牌不过；没有索赔授予答未授予。
func TestClaimLineRefusesOtherFamiliesAndJudgesAdmission(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registry, err := aipg.NewCustomerClientRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	registerPreAdviceClient(t, db, registry, syntheticCustomerSubject, syntheticCustomerAccount, map[accessidentity.CustomerCapability]string{
		accessidentity.CustomerCapabilityClaimSubmission: "SYN-CUSTOMER-GRANT-CLAIM-01",
	})
	registerPreAdviceClient(t, db, registry, claimOtherSubject, claimOtherAccount, map[accessidentity.CustomerCapability]string{
		accessidentity.CustomerCapabilityTrackingViewQuery: "SYN-CUSTOMER-GRANT-CLAIM-NOCLAIM",
	})
	issuer := newCustomerIssuerDouble(t)
	verifier, _, err := buildCustomerClientCredentialVerifier(fakeGetenv(issuer.environment()))
	if err != nil {
		t.Fatal(err)
	}
	minter, err := buildCustomerClientMinter(verifier, registry, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	endpoint := productionClaimLine(t, endpointSources{db: db, customerClientMinter: minter})
	body := claimBody("SYN-ACCESS-REQUEST-CL-0001", "SYN-ITEM-FPA09-01", claimOwnContract, claimParcel)
	for name, testCase := range map[string]struct {
		token  string
		status int
		code   string
	}{
		"操作者令牌":   {issuer.token(t, syntheticOperatorAudience, syntheticCustomerSubject), http.StatusUnauthorized, "CUSTOMER_CLIENT_CREDENTIAL_REJECTED"},
		"集成客户端令牌": {issuer.token(t, "idp-parcel-integration", syntheticCustomerSubject), http.StatusUnauthorized, "CUSTOMER_CLIENT_CREDENTIAL_REJECTED"},
		"没有索赔授予":  {issuer.token(t, syntheticCustomerAudience, claimOtherSubject), http.StatusForbidden, "CUSTOMER_CLIENT_NOT_GRANTED"},
		"权威串没设":   {issuer.token(t, syntheticCustomerAudience, syntheticCustomerSubject), http.StatusForbidden, "OUTSIDE_ADMISSION_SCOPE"},
		"没有令牌":    {"", http.StatusUnauthorized, "CUSTOMER_CLIENT_CREDENTIAL_REJECTED"},
	} {
		response := postClaim(endpoint, testCase.token, body)
		if response.Code != testCase.status || problemCode(t, response) != testCase.code {
			t.Fatalf("%s = %d %q, want %d %q", name, response.Code, problemCode(t, response), testCase.status, testCase.code)
		}
		assertNoOutcome(t, response, name)
	}
}
