package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	pgpostgres "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	pgdomain "go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件证送站预报两口在 parcel-api 装配点的几件（票 ps-external-mark-relations/09 完成判据「HTTP 与铸造用例」「真库用例」）：首方
// 族未配置时两口答未配置、一个字节不读；他族令牌一律拒；本产品的权威串没设时答不在准入范围；真库上经生产装配的同一组构造函数，
// 合成客户端取令牌交预报、撤回，册上各一行，撤销授予之后再交答未授予。合成主体、账户与号码只记 `S`。

const (
	preAdvicePath           = "/node-operations/delivery-pre-advices"
	preAdviceWithdrawalPath = "/node-operations/delivery-pre-advice-withdrawals"
	customerCommandScope    = "SYN-PILOT-SCOPE/customer-commands@v1"
)

func preAdviceBody(accessRequestID string, numbers ...string) string {
	encoded, err := json.Marshal(map[string]any{
		"accessRequestId":       accessRequestID,
		"node":                  "SYN-NODE-SHA-HUB",
		"expectedArrivalAt":     "2026-10-20T02:00:00Z",
		"customerParcelNumbers": numbers,
	})
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func preAdviceWithdrawalBody(accessRequestID, preAdviceID string) string {
	return `{"accessRequestId": "` + accessRequestID + `", "preAdviceId": "` + preAdviceID + `"}`
}

// preAdviceIntakeWith 经装配点那只构造函数装预报两口的首方 Intake。
func preAdviceIntakeWith(
	t *testing.T,
	verifier accessidentity.CustomerClientCredentialVerifier,
	registry accessidentity.CustomerClientRegistry,
	admission accessidentity.AdmissionScope,
) *nodeopshttp.CustomerPreAdviceIntake {
	t.Helper()
	minter, err := buildCustomerClientMinter(verifier, registry, admission)
	if err != nil {
		t.Fatal(err)
	}
	intake, err := buildCustomerPreAdviceIntake(minter)
	if err != nil {
		t.Fatal(err)
	}
	return intake
}

// preAdviceEndpoints 按端点表那两行的写法装两口：Intake 经 customerPreAdviceIntakes 取，nil 即字面量未配置。
func preAdviceEndpoints(intake *nodeopshttp.CustomerPreAdviceIntake, handler nodeopshttp.DeliveryPreAdviceHandler) map[string]http.Handler {
	submission, withdrawal := customerPreAdviceIntakes(intake)
	return map[string]http.Handler{
		preAdvicePath:           nodeopshttp.NewSubmitDeliveryPreAdviceEndpoint(submission, handler),
		preAdviceWithdrawalPath: nodeopshttp.NewWithdrawDeliveryPreAdviceEndpoint(withdrawal, handler),
	}
}

func postPreAdvice(endpoint http.Handler, path, token, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

// Covers: 票 ps-external-mark-relations/09 完成判据「发行方未配置 403」与 ADR-0055——首方族三件参数都没设时两口答未配置，与字面量
// 未配置逐字节同，一个字节的载荷都不读。
func TestPreAdviceLinesAnswerNotConfiguredByteForByteWhenTheCustomerFamilyIsUnset(t *testing.T) {
	verifier, configured, err := buildCustomerClientCredentialVerifier(fakeGetenv(nil))
	if err != nil || configured {
		t.Fatalf("unset parameters: configured=%v err=%v", configured, err)
	}
	const unconfigured = `{"error":{"code":"ACCESS_CHANNEL_NOT_CONFIGURED"}}` + "\n"
	for _, intake := range []*nodeopshttp.CustomerPreAdviceIntake{nil, preAdviceIntakeWith(t, verifier, absentCustomerRegister{}, admissionScopeUnset())} {
		for path, endpoint := range preAdviceEndpoints(intake, unwiredDeliveryPreAdvices{}) {
			probe := &readProbe{}
			request := httptest.NewRequest(http.MethodPost, path, probe)
			request.Header.Set("Authorization", "Bearer any.presented.token")
			response := httptest.NewRecorder()
			endpoint.ServeHTTP(response, request)
			if response.Code != http.StatusForbidden || response.Body.String() != unconfigured {
				t.Fatalf("%s（intake %v）：%d %q, want 403 %q", path, intake != nil, response.Code, response.Body.String(), unconfigured)
			}
			if probe.read {
				t.Fatalf("%s：首方族未配置却读了载荷", path)
			}
		}
	}
}

// preAdviceGrantRegister 是客户渠道册的替身：唯一的主体绑在（SYN-TENANT-01, SYN-ACCOUNT-01）上，持预报两口的授予。
type preAdviceGrantRegister struct{}

func (preAdviceGrantRegister) FindCustomerClient(_ context.Context, subject accessidentity.CustomerClientSubject) (accessidentity.CustomerClientStanding, bool, error) {
	reference, err := accessidentity.NewCredentialReference("SYN-CREDENTIAL-REF/customer-01")
	if err != nil {
		return accessidentity.CustomerClientStanding{}, false, err
	}
	binding, err := accessidentity.NewCustomerClientBinding(subject, "SYN-TENANT-01", syntheticCustomerAccount, reference, false, "SYN-BASIS")
	if err != nil {
		return accessidentity.CustomerClientStanding{}, false, err
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		return accessidentity.CustomerClientStanding{}, false, err
	}
	var grants []accessidentity.RecordedCustomerClientGrant
	for _, capability := range []accessidentity.CustomerCapability{
		accessidentity.CustomerCapabilityDeliveryPreAdviceSubmission,
		accessidentity.CustomerCapabilityDeliveryPreAdviceWithdrawal,
	} {
		grant, err := accessidentity.NewCustomerClientGrant("SYN-TENANT-01", "SYN-CUSTOMER-GRANT-"+capability.String(), subject, capability, interval, "SYN-BASIS-GRANT")
		if err != nil {
			return accessidentity.CustomerClientStanding{}, false, err
		}
		recorded, err := accessidentity.NewRecordedCustomerClientGrant(grant, nil)
		if err != nil {
			return accessidentity.CustomerClientStanding{}, false, err
		}
		grants = append(grants, recorded)
	}
	standing, err := accessidentity.NewCustomerClientStanding(binding, grants)
	return standing, err == nil, err
}

// Covers: 票 ps-external-mark-relations/09 完成判据「别族令牌（操作者、集成客户端）一律拒」「不在准入范围 403」——同一个发行方签的
// 操作者令牌与集成客户端令牌在两口答令牌不过；首方受众的令牌、主体不在册答未授予；在册且持两格授予、本产品的权威串没设时答不在
// 准入范围——预报是生产写，在铸造侧判准入（ADR-0139 决定八）。各格都不带 outcome。
func TestPreAdviceLinesRefuseOtherFamiliesAndJudgeAdmission(t *testing.T) {
	issuer := newCustomerIssuerDouble(t)
	verifier, configured, err := buildCustomerClientCredentialVerifier(fakeGetenv(issuer.environment()))
	if err != nil || !configured {
		t.Fatalf("complete parameters: configured=%v err=%v", configured, err)
	}
	for name, testCase := range map[string]struct {
		audience string
		registry accessidentity.CustomerClientRegistry
		status   int
		code     string
	}{
		"operator token":               {syntheticOperatorAudience, preAdviceGrantRegister{}, http.StatusUnauthorized, "CUSTOMER_CLIENT_CREDENTIAL_REJECTED"},
		"integration client token":     {"idp-parcel-integration", preAdviceGrantRegister{}, http.StatusUnauthorized, "CUSTOMER_CLIENT_CREDENTIAL_REJECTED"},
		"customer token, unregistered": {syntheticCustomerAudience, absentCustomerRegister{}, http.StatusForbidden, "CUSTOMER_CLIENT_NOT_GRANTED"},
		"customer token, shipment only": {syntheticCustomerAudience, customerStandingRegister{start: time.Now().Add(-time.Hour)},
			http.StatusForbidden, "CUSTOMER_CLIENT_NOT_GRANTED"},
		"granted, no self authority": {syntheticCustomerAudience, preAdviceGrantRegister{}, http.StatusForbidden, "OUTSIDE_ADMISSION_SCOPE"},
	} {
		for path, endpoint := range preAdviceEndpoints(preAdviceIntakeWith(t, verifier, testCase.registry, admissionScopeUnset()), unwiredDeliveryPreAdvices{}) {
			t.Run(name+path, func(t *testing.T) {
				body := preAdviceBody("SYN-ACCESS-REQUEST-PA-0001", "SYN-PCL-PA-0001")
				if path == preAdviceWithdrawalPath {
					body = preAdviceWithdrawalBody("SYN-ACCESS-REQUEST-PA-0001", "PREADV-SYN-0001")
				}
				response := postPreAdvice(endpoint, path, issuer.token(t, testCase.audience, syntheticCustomerSubject), body)
				if response.Code != testCase.status || problemCode(t, response) != testCase.code {
					t.Fatalf("answer = %d %q, want %d %q", response.Code, problemCode(t, response), testCase.status, testCase.code)
				}
				assertNoOutcome(t, response, path)
			})
		}
	}
}

// registerCustomerCommandAdmission 给 SYN-TENANT-01 在预报两口上各登一行准入范围对照（能力 CUSTOMER_COMMAND，事实类型取能力词），
// 并在对照指向的对象范围上各登一条 authority 的生产权威区间——与演示种子同一种登法。
func registerCustomerCommandAdmission(t *testing.T, db *bentopg.DB, authority string) {
	t.Helper()
	mappings, err := aipg.NewAdmissionScopeMappings(db)
	if err != nil {
		t.Fatal(err)
	}
	intervals, err := pgpostgres.NewAuthorityIntervals(db)
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	validity, err := accessidentity.NewEffectiveInterval(from, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, capability := range []accessidentity.CustomerCapability{
		accessidentity.CustomerCapabilityDeliveryPreAdviceSubmission,
		accessidentity.CustomerCapabilityDeliveryPreAdviceWithdrawal,
	} {
		requirement := accessidentity.AdmissionRequirement{Capability: accessidentity.CustomerCommandAdmissionCapability, FactKind: capability.String()}
		mapping, err := accessidentity.NewAdmissionScopeMapping("SYN-TENANT-01", "SYN-MAP-CC-"+capability.String(), requirement, customerCommandScope, validity, "SYN-BASIS-MAPPING")
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			outcome, err := mappings.RegisterAdmissionScopeMapping(txCtx, mapping)
			if err != nil {
				return err
			}
			if outcome != accessidentity.AdmissionScopeMappingRecorded {
				return errors.New("admission scope mapping: " + string(outcome))
			}
			return intervals.Append(txCtx, pgdomain.AuthorityInterval{
				ObjectScope: customerCommandScope,
				Capability:  requirement.Capability,
				FactKind:    requirement.FactKind,
				Authority:   authority,
				From:        from,
			})
		}); err != nil {
			t.Fatalf("登对照与区间 %s：%v", capability, err)
		}
	}
}

// registerPreAdviceClient 在客户渠道册上登一个绑在 account 上的合成客户端，并按 grants（能力 → 授予号）逐项授予。
func registerPreAdviceClient(
	t *testing.T,
	db *bentopg.DB,
	registry *aipg.CustomerClientRegistry,
	sub, account string,
	grants map[accessidentity.CustomerCapability]string,
) {
	t.Helper()
	subject, err := accessidentity.NewCustomerClientSubject(syntheticCustomerIssuer, sub)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := accessidentity.NewCredentialReference("SYN-CREDENTIAL-STORE/" + sub)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := accessidentity.NewCustomerClientBinding(subject, "SYN-TENANT-01", account, reference, false, "SYN-CUSTOMER-CLIENT-BASIS-PA")
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	err = db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		if outcome, err := registry.RegisterCustomerClient(ctx, binding); err != nil || outcome != accessidentity.CustomerClientRegistrationRecorded {
			return errors.Join(err, errors.New("register customer client: "+outcome.String()))
		}
		for capability, grantID := range grants {
			grant, err := accessidentity.NewCustomerClientGrant("SYN-TENANT-01", grantID, subject, capability, interval, "SYN-CUSTOMER-GRANT-BASIS-PA")
			if err != nil {
				return err
			}
			if outcome, err := registry.RegisterCustomerClientGrant(ctx, grant); err != nil || outcome != accessidentity.CustomerClientRegistrationRecorded {
				return errors.Join(err, errors.New("register customer client grant: "+outcome.String()))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("登记合成首方客户端 %s：%v", sub, err)
	}
}

func preAdviceIDOf(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		PreAdvice struct {
			PreAdviceID string `json:"preAdviceId"`
		} `json:"preAdvice"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	return body.PreAdvice.PreAdviceID
}

// Covers: AT-NO-069、AT-NO-070（撤回半边）与票 ps-external-mark-relations/09 完成判据「真库用例」——真库上：客户渠道册登一个合成客户端
// 并授预报两格，准入范围对照与生产权威区间照种子的登法登好，发行方替身签首方受众的令牌；经装配点同一组构造函数装出的首方 Intake
// 与预报编排交一份预报答 201 并签发标识，册上一行、来源是（发行方 # 主体，接入请求标识）；同一请求原样重投答已有结果、换内容答
// 来源冲突；另一个客户账户的客户端撤这份预报与撤一份不存在的预报同答 404；本客户端撤回答 201，册上一行撤回；撤销提交授予之后再交
// 答未授予。
func TestACustomerClientSubmitsAndWithdrawsAPreAdviceAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	const authority = "SYN-AUTH/idp-parcel-pilot"
	registerCustomerCommandAdmission(t, db, authority)
	registry, err := aipg.NewCustomerClientRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	registerPreAdviceClient(t, db, registry, syntheticCustomerSubject, syntheticCustomerAccount, map[accessidentity.CustomerCapability]string{
		accessidentity.CustomerCapabilityDeliveryPreAdviceSubmission: "SYN-CUSTOMER-GRANT-PA-SUBMISSION-01",
		accessidentity.CustomerCapabilityDeliveryPreAdviceWithdrawal: "SYN-CUSTOMER-GRANT-PA-WITHDRAWAL-01",
	})
	registerPreAdviceClient(t, db, registry, "SYN-CUSTOMER-CLIENT-02", "SYN-ACCOUNT-02", map[accessidentity.CustomerCapability]string{
		accessidentity.CustomerCapabilityDeliveryPreAdviceWithdrawal: "SYN-CUSTOMER-GRANT-PA-WITHDRAWAL-02",
	})

	issuer := newCustomerIssuerDouble(t)
	verifier, _, err := buildCustomerClientCredentialVerifier(fakeGetenv(issuer.environment()))
	if err != nil {
		t.Fatal(err)
	}
	admission, err := buildAdmissionScope(db, authority)
	if err != nil {
		t.Fatal(err)
	}
	orchestration, err := buildDeliveryPreAdviceOrchestration(db)
	if err != nil {
		t.Fatal(err)
	}
	endpoints := preAdviceEndpoints(preAdviceIntakeWith(t, verifier, registry, admission), orchestration)
	token := issuer.token(t, syntheticCustomerAudience, syntheticCustomerSubject)
	submit := func(body string) *httptest.ResponseRecorder {
		return postPreAdvice(endpoints[preAdvicePath], preAdvicePath, token, body)
	}

	first := submit(preAdviceBody("SYN-ACCESS-REQUEST-PA-0001", "SYN-PCL-PA-0001", "SYN-PCL-PA-0002"))
	if first.Code != http.StatusCreated || outcomeOf(t, first) != "DELIVERY_PRE_ADVICE_REGISTERED" {
		t.Fatalf("交预报 = %d %s, want 201 DELIVERY_PRE_ADVICE_REGISTERED", first.Code, first.Body.String())
	}
	id := preAdviceIDOf(t, first)
	preAdvices, err := nopostgres.NewDeliveryPreAdvices(db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := mustValue(t, nodomain.NewTenantID, "SYN-TENANT-01")
	state, found, err := preAdvices.FindDeliveryPreAdvice(t.Context(), tenant, mustValue(t, nodomain.NewDeliveryPreAdviceID, id))
	if err != nil || !found || state.Withdrawn() {
		t.Fatalf("册上的预报 %q：found %v, withdrawn %v, err %v", id, found, state.Withdrawn(), err)
	}
	source := state.PreAdvice.Source()
	if state.PreAdvice.CustomerAccount().String() != syntheticCustomerAccount ||
		source.Channel != syntheticCustomerIssuer+"#"+syntheticCustomerSubject || source.RequestKey != "SYN-ACCESS-REQUEST-PA-0001" {
		t.Fatalf("册上的预报 = 账户 %q，来源 %+v", state.PreAdvice.CustomerAccount(), source)
	}

	if replay := submit(preAdviceBody("SYN-ACCESS-REQUEST-PA-0001", "SYN-PCL-PA-0002", "SYN-PCL-PA-0001")); replay.Code != http.StatusOK ||
		outcomeOf(t, replay) != "EXISTING_RESULT" || preAdviceIDOf(t, replay) != id {
		t.Fatalf("原样重投 = %d %s, want 200 EXISTING_RESULT %s", replay.Code, replay.Body.String(), id)
	}
	if conflict := submit(preAdviceBody("SYN-ACCESS-REQUEST-PA-0001", "SYN-PCL-PA-0003")); conflict.Code != http.StatusOK || outcomeOf(t, conflict) != "SOURCE_CONFLICT" {
		t.Fatalf("换内容 = %d %s, want 200 SOURCE_CONFLICT", conflict.Code, conflict.Body.String())
	}

	withdraw := endpoints[preAdviceWithdrawalPath]
	otherToken := issuer.token(t, syntheticCustomerAudience, "SYN-CUSTOMER-CLIENT-02")
	foreign := postPreAdvice(withdraw, preAdviceWithdrawalPath, otherToken, preAdviceWithdrawalBody("SYN-ACCESS-REQUEST-PA-0101", id))
	absent := postPreAdvice(withdraw, preAdviceWithdrawalPath, otherToken, preAdviceWithdrawalBody("SYN-ACCESS-REQUEST-PA-0102", "PREADV-SYN-ABSENT"))
	if foreign.Code != http.StatusNotFound || foreign.Body.String() != absent.Body.String() || absent.Code != http.StatusNotFound {
		t.Fatalf("撤别的账户的 = %d %s；撤不存在的 = %d %s；want 同答 404", foreign.Code, foreign.Body.String(), absent.Code, absent.Body.String())
	}

	withdrawn := postPreAdvice(withdraw, preAdviceWithdrawalPath, token, preAdviceWithdrawalBody("SYN-ACCESS-REQUEST-PA-0002", id))
	if withdrawn.Code != http.StatusCreated || outcomeOf(t, withdrawn) != "DELIVERY_PRE_ADVICE_WITHDRAWN" {
		t.Fatalf("撤回 = %d %s, want 201 DELIVERY_PRE_ADVICE_WITHDRAWN", withdrawn.Code, withdrawn.Body.String())
	}
	if replay := postPreAdvice(withdraw, preAdviceWithdrawalPath, token, preAdviceWithdrawalBody("SYN-ACCESS-REQUEST-PA-0002", id)); replay.Code != http.StatusOK || outcomeOf(t, replay) != "EXISTING_RESULT" {
		t.Fatalf("撤回原样重投 = %d %s, want 200 EXISTING_RESULT", replay.Code, replay.Body.String())
	}
	if state, _, err := preAdvices.FindDeliveryPreAdvice(t.Context(), tenant, mustValue(t, nodomain.NewDeliveryPreAdviceID, id)); err != nil || !state.Withdrawn() {
		t.Fatalf("撤回之后：withdrawn %v, err %v", state.Withdrawn(), err)
	}
	for table, want := range map[string]int{"delivery_pre_advice": 1, "delivery_pre_advice_withdrawal": 1} {
		var rows int
		if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM node_operations.`+table).Scan(&rows); err != nil || rows != want {
			t.Fatalf("%s 有 %d 行（err %v），want %d", table, rows, err, want)
		}
	}

	revocation, err := accessidentity.NewGrantRevocation("SYN-TENANT-01", "SYN-CUSTOMER-GRANT-PA-SUBMISSION-01", time.Now().Add(-time.Second), "SYN-CUSTOMER-REVOKE-BASIS-PA")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, err := registry.RegisterCustomerClientRevocation(ctx, revocation)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if refused := submit(preAdviceBody("SYN-ACCESS-REQUEST-PA-0003", "SYN-PCL-PA-0004")); refused.Code != http.StatusForbidden || problemCode(t, refused) != "CUSTOMER_CLIENT_NOT_GRANTED" {
		t.Fatalf("撤销授予之后 = %d %s, want 403 CUSTOMER_CLIENT_NOT_GRANTED", refused.Code, refused.Body.String())
	}
}

// unwiredDeliveryPreAdvices 是预报两口编排的占位：装配测试用它证端点表两行都接到了处理器、且未配置 Intake 拒在编排之前。
type unwiredDeliveryPreAdvices struct{}

func (unwiredDeliveryPreAdvices) Submit(context.Context, nodeopsapp.SubmitDeliveryPreAdviceCommand) (nodeopsapp.DeliveryPreAdviceSubmissionResult, error) {
	return nodeopsapp.DeliveryPreAdviceSubmissionResult{}, errOrchestrationNotWired
}

func (unwiredDeliveryPreAdvices) Withdraw(context.Context, nodeopsapp.WithdrawDeliveryPreAdviceCommand) (nodeopsapp.DeliveryPreAdviceWithdrawalResult, error) {
	return nodeopsapp.DeliveryPreAdviceWithdrawalResult{}, errOrchestrationNotWired
}
