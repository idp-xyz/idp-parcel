package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件证首方客户族在 parcel-api 装配点的几件（票 first-party-api/00 完成判据「装配测试」与「单元与真库用例」）：三件参数
// 的三态与受众两两不同、提交口在首方族未配置时逐字节答未配置、各答复格、他族令牌一律拒，以及在真库上经首方口提交一单
// 形成 SUBMITTED。合成主体只记 `S`。

const (
	syntheticCustomerIssuer   = "https://id.syn.example/dex"
	syntheticCustomerAudience = "idp-parcel-customer-api"
	syntheticCustomerSubject  = "SYN-CUSTOMER-CLIENT-01"
	syntheticCustomerAccount  = "SYN-ACCOUNT-01"
)

// customerDraftBody 是首方提交口的一份载荷：管理台草案形状加接入请求标识。
func customerDraftBody(accessRequestID string) string {
	return `{
  "accessRequestId": "` + accessRequestID + `",
  "customerShipmentReference": "SYN-CUSTREF-FPA-0001",
  "requestedServiceProduct": "SYN-PROD-CN-SG-EXPRESS",
  "senderRelation": "SYN-SENDER-01",
  "senderAddress": "合成寄件地址",
  "recipientRelation": "SYN-RECIPIENT-01",
  "recipientAddress": "合成收件地址",
  "destinationServiceScope": "SYN-DEST/SG",
  "parcels": [{"customerParcelReference": "SYN-PCLREF-FPA-0001", "declaredWeightValue": "1.50", "declaredWeightUnit": "KG"}]
}`
}

type absentCustomerRegister struct{}

func (absentCustomerRegister) FindCustomerClient(context.Context, accessidentity.CustomerClientSubject) (accessidentity.CustomerClientStanding, bool, error) {
	return accessidentity.CustomerClientStanding{}, false, nil
}

// unconfiguredCustomerSubmission 是首方族发行方参数未设时装配点交出的那只 Intake。
func unconfiguredCustomerSubmission(t *testing.T) shipmenthttp.SubmissionIntake {
	t.Helper()
	verifier, configured, err := buildCustomerClientCredentialVerifier(fakeGetenv(nil))
	if err != nil || configured {
		t.Fatalf("unset parameters: configured=%v err=%v", configured, err)
	}
	return customerSubmissionWith(t, verifier, absentCustomerRegister{})
}

func customerSubmissionWith(t *testing.T, verifier accessidentity.CustomerClientCredentialVerifier, registry accessidentity.CustomerClientRegistry) shipmenthttp.SubmissionIntake {
	t.Helper()
	minter, err := buildCustomerClientMinter(verifier, registry, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	intake, err := buildCustomerSubmissionIntake(minter, permittingOwnership{anchor: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	return intake
}

func TestCustomerChannelWithoutIssuerParametersAnswersNotConfigured(t *testing.T) {
	verifier, configured, err := buildCustomerClientCredentialVerifier(fakeGetenv(nil))
	if err != nil {
		t.Fatalf("unset parameters must not refuse to start: %v", err)
	}
	if configured {
		t.Fatal("unset parameters reported as a configured customer channel")
	}
	_, err = verifier.VerifyCustomerClientCredential(context.Background(), accessidentity.NewIntegrationClientCredential("any.presented.token", nil))
	if !errors.Is(err, accessidentity.ErrAccessChannelNotConfigured) {
		t.Fatalf("err = %v, want ErrAccessChannelNotConfigured", err)
	}
}

// Covers: 票 first-party-api/00 完成判据「三件参数只设一部分启动拒」与做什么「部署参数」那一项「受众必须与操作者族、集成客户端族
// 都不同」。
func TestCustomerChannelParametersRefuseToStartWhenPartialOrSharingAnAudience(t *testing.T) {
	complete := map[string]string{
		customerAPIIssuerEnv:   syntheticCustomerIssuer,
		customerAPIJWKSURLEnv:  "https://id.syn.example/dex/keys",
		customerAPIAudienceEnv: syntheticCustomerAudience,
	}
	with := func(extra map[string]string) map[string]string {
		values := map[string]string{}
		for key, value := range complete {
			values[key] = value
		}
		for key, value := range extra {
			values[key] = value
		}
		return values
	}
	cases := map[string]map[string]string{
		"issuer only":                 {customerAPIIssuerEnv: syntheticCustomerIssuer},
		"audience only":               {customerAPIAudienceEnv: syntheticCustomerAudience},
		"issuer without JWKS":         {customerAPIIssuerEnv: syntheticCustomerIssuer, customerAPIAudienceEnv: syntheticCustomerAudience},
		"JWKS and aud no issuer":      {customerAPIJWKSURLEnv: complete[customerAPIJWKSURLEnv], customerAPIAudienceEnv: syntheticCustomerAudience},
		"issuer is not a URL":         with(map[string]string{customerAPIIssuerEnv: "dex"}),
		"operator audience reused":    with(map[string]string{operatorAudienceEnv: syntheticCustomerAudience}),
		"integration audience reused": with(map[string]string{integrationClientAudienceEnv: " " + syntheticCustomerAudience + " "}),
	}
	for name, values := range cases {
		t.Run(name, func(t *testing.T) {
			if verifier, _, err := buildCustomerClientCredentialVerifier(fakeGetenv(values)); err == nil {
				t.Fatalf("parameters %v started with verifier %T; want refusal", values, verifier)
			}
		})
	}
	distinct := with(map[string]string{operatorAudienceEnv: syntheticOperatorAudience, integrationClientAudienceEnv: "idp-parcel-integration"})
	if _, configured, err := buildCustomerClientCredentialVerifier(fakeGetenv(distinct)); err != nil || !configured {
		t.Fatalf("操作者族、集成客户端族、首方客户族受众各不相同：configured=%v err=%v", configured, err)
	}
}

// Covers: 票 first-party-api/00 完成判据「首方族未配置时提交口答 ACCESS_CHANNEL_NOT_CONFIGURED」——与字面量未配置即拒那一份
// 字节相同，且一个字节的载荷都没读。
func TestSubmissionLineAnswersNotConfiguredByteForByteWhenTheCustomerFamilyIsUnset(t *testing.T) {
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnwiredBusinessEndpointsWith(nil, unconfiguredCustomerSubmission(t)))
	const unconfigured = `{"error":{"code":"ACCESS_CHANNEL_NOT_CONFIGURED"}}` + "\n"
	probe := &readProbe{}
	request := httptest.NewRequest(http.MethodPost, "/shipment-requests", probe)
	request.Header.Set("Authorization", "Bearer any.presented.token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden || response.Body.String() != unconfigured ||
		response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("/shipment-requests: %d %q %q, want 403 %q", response.Code, response.Header().Get("Content-Type"), response.Body.String(), unconfigured)
	}
	if probe.read {
		t.Fatal("首方族未配置却读了载荷")
	}
}

type readProbe struct{ read bool }

func (probe *readProbe) Read([]byte) (int, error) {
	probe.read = true
	return 0, errors.New("must not be read")
}

type fixedCustomerVerifier struct {
	bound bool
	err   error
}

func (verifier fixedCustomerVerifier) VerifyCustomerClientCredential(
	_ context.Context,
	credential accessidentity.IntegrationClientCredential,
) (accessidentity.VerifiedCustomerClientToken, error) {
	if verifier.err != nil {
		return accessidentity.VerifiedCustomerClientToken{}, verifier.err
	}
	if credential.Token() == "" {
		return accessidentity.VerifiedCustomerClientToken{}, accessidentity.ErrCredentialRejected
	}
	subject, err := accessidentity.NewCustomerClientSubject(syntheticCustomerIssuer, syntheticCustomerSubject)
	if err != nil {
		return accessidentity.VerifiedCustomerClientToken{}, err
	}
	return accessidentity.NewVerifiedCustomerClientToken(subject, verifier.bound), nil
}

type customerStandingRegister struct {
	certificateBound bool
	start, end       time.Time
	err              error
}

func (register customerStandingRegister) FindCustomerClient(_ context.Context, subject accessidentity.CustomerClientSubject) (accessidentity.CustomerClientStanding, bool, error) {
	if register.err != nil {
		return accessidentity.CustomerClientStanding{}, false, register.err
	}
	reference, err := accessidentity.NewCredentialReference("SYN-CREDENTIAL-REF/customer-01")
	if err != nil {
		return accessidentity.CustomerClientStanding{}, false, err
	}
	binding, err := accessidentity.NewCustomerClientBinding(subject, "SYN-TENANT-01", syntheticCustomerAccount, reference, register.certificateBound, "SYN-BASIS")
	if err != nil {
		return accessidentity.CustomerClientStanding{}, false, err
	}
	interval, err := accessidentity.NewEffectiveInterval(register.start, register.end)
	if err != nil {
		return accessidentity.CustomerClientStanding{}, false, err
	}
	grant, err := accessidentity.NewCustomerClientGrant("SYN-TENANT-01", "SYN-CUSTOMER-GRANT-01", subject,
		accessidentity.CustomerCapabilityShipmentRequestSubmission, interval, "SYN-BASIS-GRANT")
	if err != nil {
		return accessidentity.CustomerClientStanding{}, false, err
	}
	recorded, err := accessidentity.NewRecordedCustomerClientGrant(grant, nil)
	if err != nil {
		return accessidentity.CustomerClientStanding{}, false, err
	}
	standing, err := accessidentity.NewCustomerClientStanding(binding, []accessidentity.RecordedCustomerClientGrant{recorded})
	return standing, err == nil, err
}

// Covers: 票 first-party-api/00 完成判据「令牌无效 / 证书绑定不符 401；不在册或无生效授予 403；依赖读不动 503」——各格经真
// 端点答各自的码，没有 outcome；令牌与授予都过了的那一笔越过 Intake 走到编排（这里接的是未接线编排，答 NO_ANSWER_FORMED）。
func TestSubmissionLineAnswersTheCustomerChannelGrades(t *testing.T) {
	now := time.Now()
	open := customerStandingRegister{start: now.Add(-time.Hour), end: now.Add(time.Hour)}
	expired := customerStandingRegister{start: now.Add(-2 * time.Hour), end: now.Add(-time.Hour)}
	bindingRequired := open
	bindingRequired.certificateBound = true

	for name, testCase := range map[string]struct {
		verifier accessidentity.CustomerClientCredentialVerifier
		registry accessidentity.CustomerClientRegistry
		token    string
		status   int
		code     string
	}{
		"no bearer token":              {fixedCustomerVerifier{}, open, "", http.StatusUnauthorized, "CUSTOMER_CLIENT_CREDENTIAL_REJECTED"},
		"certificate binding required": {fixedCustomerVerifier{}, bindingRequired, "t", http.StatusUnauthorized, "CUSTOMER_CLIENT_CREDENTIAL_REJECTED"},
		"client not registered":        {fixedCustomerVerifier{}, absentCustomerRegister{}, "t", http.StatusForbidden, "CUSTOMER_CLIENT_NOT_GRANTED"},
		"grant interval has ended":     {fixedCustomerVerifier{}, expired, "t", http.StatusForbidden, "CUSTOMER_CLIENT_NOT_GRANTED"},
		"register unreadable":          {fixedCustomerVerifier{}, customerStandingRegister{err: errors.New("connection refused")}, "t", http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
		"key set unreachable":          {fixedCustomerVerifier{err: accessidentity.ErrCredentialVerifierUnavailable}, open, "t", http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
		"certificate binding honoured": {fixedCustomerVerifier{bound: true}, bindingRequired, "t", http.StatusInternalServerError, "NO_ANSWER_FORMED"},
		"granted":                      {fixedCustomerVerifier{}, open, "t", http.StatusInternalServerError, "NO_ANSWER_FORMED"},
	} {
		t.Run(name, func(t *testing.T) {
			router := httpapi.NewWithEndpoints(buildinfo.Info{},
				assembleUnwiredBusinessEndpointsWith(nil, customerSubmissionWith(t, testCase.verifier, testCase.registry)))
			request := httptest.NewRequest(http.MethodPost, "/shipment-requests", strings.NewReader(customerDraftBody("SYN-ACCESS-REQUEST-0001")))
			if testCase.token != "" {
				request.Header.Set("Authorization", "Bearer "+testCase.token)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != testCase.status || problemCode(t, response) != testCase.code {
				t.Fatalf("answer = %d %q, want %d %q", response.Code, problemCode(t, response), testCase.status, testCase.code)
			}
			assertNoOutcome(t, response, "/shipment-requests")
		})
	}
}

// customerIssuerDouble 是同一个发行方：操作者族、集成客户端族、首方客户族的令牌都由它签，parcel-api 的首方族按它公布的公钥
// 集验——各族落不落在同一个发行方实例上是部署决定（ADR-0139 决定二），落在同一个上时一枚令牌过得了哪一族只看受众。
type customerIssuerDouble struct {
	key  *rsa.PrivateKey
	jwks *httptest.Server
}

func newCustomerIssuerDouble(t *testing.T) customerIssuerDouble {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{"keys": []map[string]any{{
			"kty": "RSA", "kid": "syn-customer-1", "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	t.Cleanup(jwks.Close)
	return customerIssuerDouble{key: key, jwks: jwks}
}

func (issuer customerIssuerDouble) environment() map[string]string {
	return map[string]string{
		customerAPIIssuerEnv:         syntheticCustomerIssuer,
		customerAPIJWKSURLEnv:        issuer.jwks.URL,
		customerAPIAudienceEnv:       syntheticCustomerAudience,
		operatorAudienceEnv:          syntheticOperatorAudience,
		integrationClientAudienceEnv: "idp-parcel-integration",
	}
}

func (issuer customerIssuerDouble) token(t *testing.T, audience, subject string) string {
	t.Helper()
	return signOperatorToken(t, issuer.key, "syn-customer-1", map[string]any{
		"iss": syntheticCustomerIssuer,
		"aud": audience,
		"sub": subject,
		"exp": time.Now().Add(10 * time.Minute).Unix(),
	})
}

// Covers: 票 first-party-api/00 完成判据「他族令牌（操作者、集成客户端）在首方口一律拒」——同一个发行方签出的操作者令牌与集成
// 客户端令牌，受众不是首方族那一个，在提交口答令牌不过；首方受众的令牌过了校验期，不在客户渠道册里答未授予。
func TestOtherFamiliesTokensAreRefusedAtTheSubmissionLine(t *testing.T) {
	issuer := newCustomerIssuerDouble(t)
	verifier, configured, err := buildCustomerClientCredentialVerifier(fakeGetenv(issuer.environment()))
	if err != nil || !configured {
		t.Fatalf("complete parameters: configured=%v err=%v", configured, err)
	}
	router := httpapi.NewWithEndpoints(buildinfo.Info{},
		assembleUnwiredBusinessEndpointsWith(nil, customerSubmissionWith(t, verifier, absentCustomerRegister{})))

	for name, testCase := range map[string]struct {
		audience string
		status   int
		code     string
	}{
		"operator token":               {syntheticOperatorAudience, http.StatusUnauthorized, "CUSTOMER_CLIENT_CREDENTIAL_REJECTED"},
		"integration client token":     {"idp-parcel-integration", http.StatusUnauthorized, "CUSTOMER_CLIENT_CREDENTIAL_REJECTED"},
		"customer token, unregistered": {syntheticCustomerAudience, http.StatusForbidden, "CUSTOMER_CLIENT_NOT_GRANTED"},
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/shipment-requests", strings.NewReader(customerDraftBody("SYN-ACCESS-REQUEST-0001")))
			request.Header.Set("Authorization", "Bearer "+issuer.token(t, testCase.audience, syntheticCustomerSubject))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != testCase.status || problemCode(t, response) != testCase.code {
				t.Fatalf("answer = %d %q, want %d %q", response.Code, problemCode(t, response), testCase.status, testCase.code)
			}
		})
	}
}

// Covers: 票 first-party-api/00 完成判据「正例铸出首方信封、提交形成 SUBMITTED」——真库上：客户渠道册登一个合成客户端并授提交
// 能力，发行方替身签首方受众的令牌，经装配点同一组构造函数装出的首方 Intake 与提交编排提交一单，答 201 SUBMITTED；委托的来源
// 身份是册上的绑定加客户端主体与接入请求标识。同一请求重发答 EXISTING_RESULT；撤销授予之后同一个客户端答未授予。归属坐标取
// 写开关那一格（墙二）与种子同款的合成区间：生产目录未立，生产形态下归属答`权威未确定`（assemble_submission_test.go）。
func TestACustomerClientSubmitsThroughTheFirstPartyLineAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	appendIsolatedAuthorityInterval(t, db)
	registry, err := aipg.NewCustomerClientRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	subject, err := accessidentity.NewCustomerClientSubject(syntheticCustomerIssuer, syntheticCustomerSubject)
	if err != nil {
		t.Fatal(err)
	}
	registerCustomerClient(t, db, registry, subject, "SYN-TENANT-01", syntheticCustomerAccount, "SYN-CUSTOMER-GRANT-SUBMISSION-01")

	issuer := newCustomerIssuerDouble(t)
	verifier, _, err := buildCustomerClientCredentialVerifier(fakeGetenv(issuer.environment()))
	if err != nil {
		t.Fatal(err)
	}
	minter, err := buildCustomerClientMinter(verifier, registry, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	isolated := isolatedWriteAdmissionForTest(t)
	ownership, err := buildProductionOwnership(db, isolated, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	intake, err := buildCustomerSubmissionIntake(minter, ownership)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := buildSubmissionOrchestration(db, isolated, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := shipmenthttp.NewSubmitShipmentRequestEndpoint(intake, submission)
	token := issuer.token(t, syntheticCustomerAudience, syntheticCustomerSubject)
	post := func(body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/shipment-requests", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, request)
		return response
	}

	first := post(customerDraftBody("SYN-ACCESS-REQUEST-FPA-0001"))
	if first.Code != http.StatusCreated || outcomeOf(t, first) != "SUBMITTED" {
		t.Fatalf("首方提交 = %d %s, want 201 SUBMITTED", first.Code, first.Body.String())
	}
	identity, err := domain.NewSourceIdentity(
		mustValue(t, domain.NewTenantID, "SYN-TENANT-01"),
		mustValue(t, domain.NewCustomerAccountID, syntheticCustomerAccount),
		mustValue(t, domain.NewSource, syntheticCustomerIssuer+"#"+syntheticCustomerSubject),
		mustValue(t, domain.NewSourceRequestKey, "SYN-ACCESS-REQUEST-FPA-0001"),
	)
	if err != nil {
		t.Fatal(err)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := requests.FindBySourceIdentity(t.Context(), identity); err != nil || !found {
		t.Fatalf("按首方信封的来源身份找委托：found %v, err %v", found, err)
	}

	if replay := post(customerDraftBody("SYN-ACCESS-REQUEST-FPA-0001")); replay.Code != http.StatusOK || outcomeOf(t, replay) != "EXISTING_RESULT" {
		t.Fatalf("同一请求重发 = %d %s, want 200 EXISTING_RESULT", replay.Code, replay.Body.String())
	}

	revokeCustomerGrant(t, db, registry)
	if refused := post(customerDraftBody("SYN-ACCESS-REQUEST-FPA-0002")); refused.Code != http.StatusForbidden || problemCode(t, refused) != "CUSTOMER_CLIENT_NOT_GRANTED" {
		t.Fatalf("撤销授予之后 = %d %s, want 403 CUSTOMER_CLIENT_NOT_GRANTED", refused.Code, refused.Body.String())
	}
}

// Covers: ADR-0091 决定六「也不因它出现而获得任何真实租户」——写开关开着、登记册里有匹配的合成权威区间时经首方口提交：
// 开关租户的客户端答 201 SUBMITTED；别的租户的客户端（另一个合成租户、一个不带合成前缀的租户）答 OWNERSHIP_UNRESOLVED，
// 归属`权威未确定`、原因是规则不可用，修订里没有隔离形态的权威串——与生产形态同一个答法。提交口挂的是真渠道，哪个租户
// 的客户端都走得到归属这一步，挡住它们的只有隔离目录认租户这一格。
func TestTheWriteSwitchLendsItsCoordinatesOnlyToItsOwnTenant(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	appendIsolatedAuthorityInterval(t, db)
	registry, err := aipg.NewCustomerClientRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	issuer := newCustomerIssuerDouble(t)
	verifier, _, err := buildCustomerClientCredentialVerifier(fakeGetenv(issuer.environment()))
	if err != nil {
		t.Fatal(err)
	}
	minter, err := buildCustomerClientMinter(verifier, registry, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	isolated := isolatedWriteAdmissionForTest(t)
	ownership, err := buildProductionOwnership(db, isolated, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	intake, err := buildCustomerSubmissionIntake(minter, ownership)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := buildSubmissionOrchestration(db, isolated, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := shipmenthttp.NewSubmitShipmentRequestEndpoint(intake, submission)

	for _, testCase := range []struct {
		name, subject, tenant, account string
		status                         int
		outcome                        string
	}{
		{"switch tenant", "SYN-CUSTOMER-CLIENT-01", isolated.tenant, syntheticCustomerAccount, http.StatusCreated, "SUBMITTED"},
		{"another synthetic tenant", "SYN-CUSTOMER-CLIENT-02", "SYN-TENANT-02", syntheticCustomerAccount, http.StatusOK, "OWNERSHIP_UNRESOLVED"},
		{"tenant without the synthetic prefix", "CUSTOMER-CLIENT-PROD-1", "TENANT-PROD-1", "ACCOUNT-PROD-1", http.StatusOK, "OWNERSHIP_UNRESOLVED"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			subject, err := accessidentity.NewCustomerClientSubject(syntheticCustomerIssuer, testCase.subject)
			if err != nil {
				t.Fatal(err)
			}
			registerCustomerClient(t, db, registry, subject, testCase.tenant, testCase.account, "SYN-CUSTOMER-GRANT-"+testCase.subject)

			request := httptest.NewRequest(http.MethodPost, "/shipment-requests", strings.NewReader(customerDraftBody("SYN-ACCESS-REQUEST-"+testCase.subject)))
			request.Header.Set("Authorization", "Bearer "+issuer.token(t, syntheticCustomerAudience, testCase.subject))
			response := httptest.NewRecorder()
			endpoint.ServeHTTP(response, request)

			var body struct {
				Outcome             string `json:"outcome"`
				ProductionOwnership struct {
					Authority        string `json:"authority"`
					Revision         string `json:"revision"`
					UnresolvedReason string `json:"unresolvedReason"`
				} `json:"productionOwnership"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
			}
			if response.Code != testCase.status || body.Outcome != testCase.outcome {
				t.Fatalf("提交 = %d %s, want %d %s", response.Code, response.Body.String(), testCase.status, testCase.outcome)
			}
			if testCase.outcome != "OWNERSHIP_UNRESOLVED" {
				return
			}
			if body.ProductionOwnership.Authority != "UNRESOLVED" || body.ProductionOwnership.UnresolvedReason != "RULE_UNAVAILABLE" {
				t.Fatalf("归属 = %+v, want UNRESOLVED / RULE_UNAVAILABLE——与生产形态目录缺席同一个答法", body.ProductionOwnership)
			}
			if strings.Contains(body.ProductionOwnership.Revision, isolatedSelfAuthority) {
				t.Fatalf("修订 %q 带着隔离形态的权威串——非开关租户拿到了合成坐标", body.ProductionOwnership.Revision)
			}
		})
	}
}

func registerCustomerClient(
	t *testing.T,
	db *bentopg.DB,
	registry *aipg.CustomerClientRegistry,
	subject accessidentity.CustomerClientSubject,
	tenant, account, grantID string,
) {
	t.Helper()
	reference, err := accessidentity.NewCredentialReference("SYN-CREDENTIAL-STORE/customer-01")
	if err != nil {
		t.Fatal(err)
	}
	binding, err := accessidentity.NewCustomerClientBinding(subject, tenant, account, reference, false, "SYN-CUSTOMER-CLIENT-BASIS-01")
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := accessidentity.NewCustomerClientGrant(tenant, grantID, subject,
		accessidentity.CustomerCapabilityShipmentRequestSubmission, interval, "SYN-CUSTOMER-GRANT-BASIS-01")
	if err != nil {
		t.Fatal(err)
	}
	err = db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		if outcome, err := registry.RegisterCustomerClient(ctx, binding); err != nil || outcome != accessidentity.CustomerClientRegistrationRecorded {
			return errors.Join(err, errors.New("register customer client: "+outcome.String()))
		}
		if outcome, err := registry.RegisterCustomerClientGrant(ctx, grant); err != nil || outcome != accessidentity.CustomerClientRegistrationRecorded {
			return errors.Join(err, errors.New("register customer client grant: "+outcome.String()))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("登记合成首方客户端：%v", err)
	}
}

func revokeCustomerGrant(t *testing.T, db *bentopg.DB, registry *aipg.CustomerClientRegistry) {
	t.Helper()
	revocation, err := accessidentity.NewGrantRevocation("SYN-TENANT-01", "SYN-CUSTOMER-GRANT-SUBMISSION-01", time.Now().Add(-time.Second), "SYN-CUSTOMER-REVOKE-BASIS-01")
	if err != nil {
		t.Fatal(err)
	}
	err = db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, err := registry.RegisterCustomerClientRevocation(ctx, revocation)
		return err
	})
	if err != nil {
		t.Fatalf("撤销授予：%v", err)
	}
}

func outcomeOf(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	return body.Outcome
}
