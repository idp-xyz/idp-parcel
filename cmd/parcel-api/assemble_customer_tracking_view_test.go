package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件证客户追踪视图查阅口在 parcel-api 装配点的几件（票 demo-chain-gaps/96 完成判据）：首方族未配置时答未配置、一个字节不读；
// 他族令牌答令牌不过、没有这一格授予答未授予；真库上经生产装配的同一组构造函数，合成客户端读本账户包裹的视图答当前视图，别的账户的
// 包裹与不存在的包裹同答；查阅不判准入范围。合成主体、账户与包裹只记 `S`。

const customerTrackingViewPath = "/v1/customer-tracking-view"

// customerTrackingViewEndpoint 按端点表那一行的写法装：Intake 经 customerTrackingViewQueryIntake 取，nil 即字面量未配置。
func customerTrackingViewEndpoint(
	intake *visibilityhttp.CustomerTrackingViewIntake,
	reader visibilityhttp.TrackingViewReader,
	attributions veports.ParcelCustomerAccountView,
) http.Handler {
	return visibilityhttp.NewQueryCustomerTrackingViewEndpoint(customerTrackingViewQueryIntake(intake), reader, attributions)
}

func trackingViewIntakeWith(
	t *testing.T,
	verifier accessidentity.CustomerClientCredentialVerifier,
	registry accessidentity.CustomerClientRegistry,
) *visibilityhttp.CustomerTrackingViewIntake {
	t.Helper()
	minter, err := buildCustomerClientMinter(verifier, registry, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	intake, err := buildCustomerTrackingViewIntake(minter)
	if err != nil {
		t.Fatal(err)
	}
	return intake
}

func getTrackingView(endpoint http.Handler, token, query string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, customerTrackingViewPath+query, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

// untouchedViews 是视图读口的替身：被读一次就判失败——Intake 没过的请求不该走到读库这一步。
type untouchedViews struct{ t *testing.T }

func (views untouchedViews) FindCurrent(context.Context, vedomain.TenantID, vedomain.CustomerAccountReference, vedomain.TrackedParcelReference) (vedomain.CustomerTrackingView, bool, error) {
	views.t.Error("Intake 没过却读了视图库")
	return vedomain.CustomerTrackingView{}, false, nil
}

// untouchedAttributions 是归属反查口的替身：被问一次就判失败，理由同 untouchedViews。
type untouchedAttributions struct{ t *testing.T }

func (attributions untouchedAttributions) FindCustomerAccount(context.Context, vedomain.TenantID, vedomain.TrackedParcelReference) (vedomain.CustomerAccountReference, bool, error) {
	attributions.t.Error("Intake 没过却问了客户归属")
	return vedomain.CustomerAccountReference{}, false, nil
}

// Covers: 票 demo-chain-gaps/96 完成判据「首方族三件不设时仍答 403 ACCESS_CHANNEL_NOT_CONFIGURED」——Intake 缺席（字面量未配置）与
// 三件都没设时装出的首方 Intake 答同一份字节，视图库一次都没读。
func TestCustomerTrackingViewLineAnswersNotConfiguredByteForByteWhenTheCustomerFamilyIsUnset(t *testing.T) {
	verifier, configured, err := buildCustomerClientCredentialVerifier(fakeGetenv(nil))
	if err != nil || configured {
		t.Fatalf("unset parameters: configured=%v err=%v", configured, err)
	}
	for _, intake := range []*visibilityhttp.CustomerTrackingViewIntake{nil, trackingViewIntakeWith(t, verifier, absentCustomerRegister{})} {
		response := getTrackingView(customerTrackingViewEndpoint(intake, untouchedViews{t}, untouchedAttributions{t}), "any.presented.token", "?parcel=SYN-PARCEL-01")
		if !answersFirstPartyUnconfigured(t, response) {
			t.Fatalf("intake %v：%d %q, want 403 %s", intake != nil, response.Code, response.Body.String(), firstPartyUnconfigured)
		}
	}
}

// trackingViewGrantRegister 是客户渠道册的替身：唯一的主体绑在（SYN-TENANT-01, SYN-ACCOUNT-01）上，持给定的几项授予。
type trackingViewGrantRegister struct {
	capabilities []accessidentity.CustomerCapability
}

func (register trackingViewGrantRegister) FindCustomerClient(_ context.Context, subject accessidentity.CustomerClientSubject) (accessidentity.CustomerClientStanding, bool, error) {
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
	for _, capability := range register.capabilities {
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

// Covers: 票 demo-chain-gaps/96 完成判据「操作者令牌答 401 CUSTOMER_CLIENT_CREDENTIAL_REJECTED；没有这一格授予答 403
// CUSTOMER_CLIENT_NOT_GRANTED」与 AT-VE-172——同一个发行方签的操作者令牌、集成客户端令牌答令牌不过；首方受众的令牌、主体不在册答
// 未授予；只持委托提交与送站预报授予的客户端也答未授予。各格都不带 outcome，视图库一次都没读。
// 也证 AT-VE-153「只有外部运单号但没有客户账户授权→不受理查询，标识本身不作为访问凭证」：不带令牌、或只持别口授予的请求，带着
// 包裹定位也答认证那一格（401、403），不是坏请求，也不读视图。
func TestCustomerTrackingViewLineRefusesOtherFamiliesAndClientsWithoutTheGrant(t *testing.T) {
	issuer := newCustomerIssuerDouble(t)
	verifier, configured, err := buildCustomerClientCredentialVerifier(fakeGetenv(issuer.environment()))
	if err != nil || !configured {
		t.Fatalf("complete parameters: configured=%v err=%v", configured, err)
	}
	otherGrants := trackingViewGrantRegister{capabilities: []accessidentity.CustomerCapability{
		accessidentity.CustomerCapabilityShipmentRequestSubmission,
		accessidentity.CustomerCapabilityDeliveryPreAdviceSubmission,
		accessidentity.CustomerCapabilityDeliveryPreAdviceWithdrawal,
	}}
	for name, testCase := range map[string]struct {
		registry accessidentity.CustomerClientRegistry
		audience string
		status   int
		code     string
	}{
		"operator token":               {otherGrants, syntheticOperatorAudience, http.StatusUnauthorized, "CUSTOMER_CLIENT_CREDENTIAL_REJECTED"},
		"integration client token":     {otherGrants, "idp-parcel-integration", http.StatusUnauthorized, "CUSTOMER_CLIENT_CREDENTIAL_REJECTED"},
		"customer token, unregistered": {absentCustomerRegister{}, syntheticCustomerAudience, http.StatusForbidden, "CUSTOMER_CLIENT_NOT_GRANTED"},
		"customer token, other grants": {otherGrants, syntheticCustomerAudience, http.StatusForbidden, "CUSTOMER_CLIENT_NOT_GRANTED"},
		"no token at all":              {otherGrants, "", http.StatusUnauthorized, "CUSTOMER_CLIENT_CREDENTIAL_REJECTED"},
	} {
		t.Run(name, func(t *testing.T) {
			token := ""
			if testCase.audience != "" {
				token = issuer.token(t, testCase.audience, syntheticCustomerSubject)
			}
			endpoint := customerTrackingViewEndpoint(trackingViewIntakeWith(t, verifier, testCase.registry), untouchedViews{t}, untouchedAttributions{t})
			response := getTrackingView(endpoint, token, "?parcel=SYN-PARCEL-01")
			if response.Code != testCase.status || problemCode(t, response) != testCase.code {
				t.Fatalf("answer = %d %s, want %d %s", response.Code, response.Body.String(), testCase.status, testCase.code)
			}
			assertNoOutcome(t, response, customerTrackingViewPath)
		})
	}
}

// Covers: 票 demo-chain-gaps/96 完成判据「首方客户令牌读自己账户名下包裹的视图答 200；别的账户的包裹与不存在的包裹同答」、
// AT-VE-150、AT-VE-151、AT-VE-171、AT-VE-173——真库上：客户渠道册登一个合成客户端并授查阅能力，视图库里本账户与另一账户各有一件
// 包裹的当前视图；发行方替身签首方受众的令牌，经装配点同一组构造函数装出的首方 Intake 与生产用的视图读口查：本账户那件答
// CURRENT_VIEW 并带版本；另一账户那件与根本没有的那件答同一份字节 VIEW_NOT_FOUND；查询串自报另一账户答 400。准入范围是本产品
// 权威串没设的那一只（对写口答不在范围），查阅照答——查阅口不判准入。撤销授予之后答未授予。
func TestACustomerClientReadsItsOwnTrackingViewAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registry, err := aipg.NewCustomerClientRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	subject, err := accessidentity.NewCustomerClientSubject(syntheticCustomerIssuer, syntheticCustomerSubject)
	if err != nil {
		t.Fatal(err)
	}
	registerTrackingViewClient(t, db, registry, subject, "SYN-CUSTOMER-GRANT-TRACKING-VIEW-01")

	views, err := vepostgres.NewCustomerViews(db)
	if err != nil {
		t.Fatal(err)
	}
	saveCustomerView(t, db, views, syntheticCustomerAccount, "SYN-PARCEL-DCG96-OWN", "SYN-VIEW-DCG96-OWN")
	saveCustomerView(t, db, views, "SYN-ACCOUNT-02", "SYN-PARCEL-DCG96-OTHER", "SYN-VIEW-DCG96-OTHER")

	issuer := newCustomerIssuerDouble(t)
	verifier, _, err := buildCustomerClientCredentialVerifier(fakeGetenv(issuer.environment()))
	if err != nil {
		t.Fatal(err)
	}
	attributions, err := buildCustomerTrackingViewAttributions(db)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := customerTrackingViewEndpoint(trackingViewIntakeWith(t, verifier, registry), views, attributions)
	token := issuer.token(t, syntheticCustomerAudience, syntheticCustomerSubject)

	own := getTrackingView(endpoint, token, "?parcel=SYN-PARCEL-DCG96-OWN")
	var body struct {
		Outcome string `json:"outcome"`
		View    struct {
			Version string `json:"version"`
			Parcel  string `json:"parcel"`
		} `json:"view"`
	}
	if err := json.Unmarshal(own.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", own.Body.Bytes(), err)
	}
	if own.Code != http.StatusOK || body.Outcome != "CURRENT_VIEW" || body.View.Version != "SYN-VIEW-DCG96-OWN" || body.View.Parcel != "SYN-PARCEL-DCG96-OWN" {
		t.Fatalf("本账户包裹 = %d %s, want 200 CURRENT_VIEW SYN-VIEW-DCG96-OWN", own.Code, own.Body.String())
	}

	other := getTrackingView(endpoint, token, "?parcel=SYN-PARCEL-DCG96-OTHER")
	missing := getTrackingView(endpoint, token, "?parcel=SYN-PARCEL-DCG96-NOWHERE")
	if other.Code != http.StatusOK || outcomeOf(t, other) != "VIEW_NOT_FOUND" {
		t.Fatalf("别的账户的包裹 = %d %s, want 200 VIEW_NOT_FOUND", other.Code, other.Body.String())
	}
	if other.Code != missing.Code || other.Body.String() != missing.Body.String() {
		t.Fatalf("别的账户的包裹 %d %q 与不存在的包裹 %d %q 可分辨", other.Code, other.Body.String(), missing.Code, missing.Body.String())
	}

	if declared := getTrackingView(endpoint, token, "?parcel=SYN-PARCEL-DCG96-OTHER&customerAccount=SYN-ACCOUNT-02"); declared.Code != http.StatusBadRequest || problemCode(t, declared) != "MALFORMED_REQUEST" {
		t.Fatalf("查询串自报账户 = %d %s, want 400 MALFORMED_REQUEST", declared.Code, declared.Body.String())
	}

	revocation, err := accessidentity.NewGrantRevocation("SYN-TENANT-01", "SYN-CUSTOMER-GRANT-TRACKING-VIEW-01", time.Now().Add(-time.Second), "SYN-CUSTOMER-REVOKE-BASIS-01")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, err := registry.RegisterCustomerClientRevocation(ctx, revocation)
		return err
	}); err != nil {
		t.Fatalf("撤销授予：%v", err)
	}
	if refused := getTrackingView(endpoint, token, "?parcel=SYN-PARCEL-DCG96-OWN"); refused.Code != http.StatusForbidden || problemCode(t, refused) != "CUSTOMER_CLIENT_NOT_GRANTED" {
		t.Fatalf("撤销授予之后 = %d %s, want 403 CUSTOMER_CLIENT_NOT_GRANTED", refused.Code, refused.Body.String())
	}
}

// Covers: UC-VE-008 `AT-VE-188`、`AT-VE-151`——真库上，经登记处里这一特性的生产 build 与 mount（端点表实挂的那一行、生产用的视图读口
// 与归属反查口），一件包裹沿委托与视图的时间线走过三格（ADR-0246 决定一、二、五）：委托`已提交`、还没接受时，归属方查它与查一件不存在
// 的包裹同答 VIEW_NOT_FOUND；委托`已接受`、视图未形成时，归属方答 NO_VISIBLE_RESULT；视图形成之后归属方答 CURRENT_VIEW。三个阶段里，
// 另一个账户的客户端查同一个号，都与它查不存在的号答同一份字节。合成主体、账户与包裹只记 `S`。
func TestTheProductionTrackingViewLineWalksAParcelThroughItsThreeGradesAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registry, err := aipg.NewCustomerClientRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	const otherSubject, otherAccount = "SYN-CUSTOMER-CLIENT-02", "SYN-ACCOUNT-02"
	for _, client := range []struct{ subject, account, grant string }{
		{syntheticCustomerSubject, syntheticCustomerAccount, "SYN-CUSTOMER-GRANT-C31-01"},
		{otherSubject, otherAccount, "SYN-CUSTOMER-GRANT-C31-02"},
	} {
		subject, err := accessidentity.NewCustomerClientSubject(syntheticCustomerIssuer, client.subject)
		if err != nil {
			t.Fatal(err)
		}
		registerTrackingViewClientFor(t, db, registry, subject, client.account, client.grant)
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
	endpoint := productionTrackingViewLine(t, endpointSources{db: db, customerClientMinter: minter})
	owner := issuer.token(t, syntheticCustomerAudience, syntheticCustomerSubject)
	other := issuer.token(t, syntheticCustomerAudience, otherSubject)

	const parcel = "SYN-PARCEL-C31-01"
	command := trackingViewSubmissionCommand(t, parcel)
	answersOf := func(stage string) (ownerAnswer *httptest.ResponseRecorder) {
		t.Helper()
		ownerAnswer = getTrackingView(endpoint, owner, "?parcel="+parcel)
		probe := getTrackingView(endpoint, other, "?parcel="+parcel)
		missing := getTrackingView(endpoint, other, "?parcel=SYN-PARCEL-C31-NOWHERE")
		if missing.Code != http.StatusOK || outcomeOf(t, missing) != "VIEW_NOT_FOUND" {
			t.Fatalf("%s：另一账户查不存在的号 = %d %s, want 200 VIEW_NOT_FOUND", stage, missing.Code, missing.Body.String())
		}
		if probe.Code != missing.Code || probe.Body.String() != missing.Body.String() || !reflect.DeepEqual(probe.Header(), missing.Header()) {
			t.Fatalf("%s：另一账户查这件 %d %v %q 与查不存在的号 %d %v %q 可分辨", stage,
				probe.Code, probe.Header(), probe.Body.String(), missing.Code, missing.Header(), missing.Body.String())
		}
		return ownerAnswer
	}

	submitOnRealAssembly(t, db, command)
	beforeAcceptance := answersOf("委托已提交")
	ownerMissing := getTrackingView(endpoint, owner, "?parcel=SYN-PARCEL-C31-NOWHERE")
	if beforeAcceptance.Code != ownerMissing.Code || beforeAcceptance.Body.String() != ownerMissing.Body.String() || outcomeOf(t, beforeAcceptance) != "VIEW_NOT_FOUND" {
		t.Fatalf("委托接受之前，归属方查它 %d %q，want 与查不存在的号 %d %q 同答 VIEW_NOT_FOUND",
			beforeAcceptance.Code, beforeAcceptance.Body.String(), ownerMissing.Code, ownerMissing.Body.String())
	}

	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatal(err)
	}
	request, found, err := requests.FindBySourceIdentity(t.Context(), command.Identity)
	if err != nil || !found {
		t.Fatalf("读回委托：err=%v found=%v", err, found)
	}
	acceptAndSave(t, db, requests, command, request)
	if accepted := answersOf("委托已接受、视图未形成"); accepted.Code != http.StatusOK || strings.TrimSpace(accepted.Body.String()) != `{"outcome":"NO_VISIBLE_RESULT"}` {
		t.Fatalf("委托已接受、视图未形成，归属方查它 = %d %s, want 200 {\"outcome\":\"NO_VISIBLE_RESULT\"}", accepted.Code, accepted.Body.String())
	}

	views, err := vepostgres.NewCustomerViews(db)
	if err != nil {
		t.Fatal(err)
	}
	saveCustomerView(t, db, views, syntheticCustomerAccount, parcel, "SYN-VIEW-C31-01")
	formed := answersOf("视图已形成")
	var body struct {
		Outcome string `json:"outcome"`
		View    struct {
			Version string `json:"version"`
		} `json:"view"`
	}
	if err := json.Unmarshal(formed.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", formed.Body.Bytes(), err)
	}
	if formed.Code != http.StatusOK || body.Outcome != "CURRENT_VIEW" || body.View.Version != "SYN-VIEW-C31-01" {
		t.Fatalf("视图形成之后，归属方查它 = %d %s, want 200 CURRENT_VIEW SYN-VIEW-C31-01", formed.Code, formed.Body.String())
	}
}

// productionTrackingViewLine 从端点登记处取客户追踪视图那一特性，经它自己的生产 build 与 mount 装出查阅口那一行。
func productionTrackingViewLine(t *testing.T, sources endpointSources) http.Handler {
	t.Helper()
	for _, feature := range endpointRegistry {
		if feature.wiringKey != any((*customerTrackingViewWiring)(nil)) {
			continue
		}
		mounted, err := feature.production(sources, endpointChannels{})
		if err != nil {
			t.Fatalf("装配：%v", err)
		}
		for _, line := range mounted {
			if line.Pattern == customerTrackingViewPath {
				return line.Handler
			}
		}
	}
	t.Fatalf("%s 没按登记处挂上", customerTrackingViewPath)
	return nil
}

// trackingViewSubmissionCommand 是 submissionCommand 换到查阅口客户端所绑的（SYN-TENANT-01, SYN-ACCOUNT-01）上、只声明一件包裹的形。
func trackingViewSubmissionCommand(t *testing.T, parcel string) shipmentapp.SubmitShipmentRequestCommand {
	t.Helper()
	command := submissionCommand(t)
	identity, err := psdomain.NewSourceIdentity(
		mustValue(t, psdomain.NewTenantID, "SYN-TENANT-01"),
		mustValue(t, psdomain.NewCustomerAccountID, syntheticCustomerAccount),
		mustValue(t, psdomain.NewSource, "SYN-SOURCE-C31"),
		mustValue(t, psdomain.NewSourceRequestKey, "SYN-KEY-C31-01"),
	)
	if err != nil {
		t.Fatal(err)
	}
	command.Identity = identity
	command.ShipmentRequestID = mustValue(t, psdomain.NewShipmentRequestID, "SYN-REQUEST-C31-01")
	command.DeclaredParcelIDs = []psdomain.DeclaredParcelID{mustValue(t, psdomain.NewDeclaredParcelID, parcel)}
	return command
}

// registerTrackingViewClient 在真的客户渠道册上登一个绑在（SYN-TENANT-01, SYN-ACCOUNT-01）上的合成客户端，只授查阅那一项。登进去
// 也就证了迁移里那条 CHECK 放开了这个词。
func registerTrackingViewClient(t *testing.T, db *bentopg.DB, registry *aipg.CustomerClientRegistry, subject accessidentity.CustomerClientSubject, grantID string) {
	t.Helper()
	registerTrackingViewClientFor(t, db, registry, subject, syntheticCustomerAccount, grantID)
}

// registerTrackingViewClientFor 是 registerTrackingViewClient 指定账户的形：同一租户下另一个货主客户账户的客户端照同一配方登。
func registerTrackingViewClientFor(t *testing.T, db *bentopg.DB, registry *aipg.CustomerClientRegistry, subject accessidentity.CustomerClientSubject, account, grantID string) {
	t.Helper()
	reference, err := accessidentity.NewCredentialReference("SYN-CREDENTIAL-STORE/" + account)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := accessidentity.NewCustomerClientBinding(subject, "SYN-TENANT-01", account, reference, false, "SYN-CUSTOMER-CLIENT-BASIS-01")
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := accessidentity.NewCustomerClientGrant("SYN-TENANT-01", grantID, subject,
		accessidentity.CustomerCapabilityTrackingViewQuery, interval, "SYN-CUSTOMER-GRANT-BASIS-01")
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

// saveCustomerView 在视图库里给（SYN-TENANT-01, account, parcel）提交一版首发视图：里程碑展示，其余三维待确认。视图怎么派生不是本
// 文件要证的（UC-VE-008 的内部半边另有用例），这里只要库里有一行。
func saveCustomerView(t *testing.T, db *bentopg.DB, views *vepostgres.CustomerViews, account, parcel, version string) {
	t.Helper()
	milestones, err := vedomain.ShowDimension(mustValue(t, vedomain.NewViewContentReference, "SYN-MILESTONES/"+parcel))
	if err != nil {
		t.Fatal(err)
	}
	view, err := vedomain.PublishCustomerView(
		mustValue(t, vedomain.NewCustomerViewVersionID, version),
		mustValue(t, vedomain.NewCustomerAccountReference, account),
		mustValue(t, vedomain.NewTrackedParcelReference, parcel),
		mustValue(t, vedomain.NewProjectionVersionID, "SYN-PROJECTION/"+parcel),
		vedomain.CustomerViewDimensions{
			Milestones: milestones,
			ETA:        vedomain.PendDimension(),
			Final:      vedomain.PendDimension(),
			Note:       vedomain.PendDimension(),
		},
		time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	tenant := mustValue(t, vedomain.NewTenantID, "SYN-TENANT-01")
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		return views.Save(ctx, tenant, view)
	}); err != nil {
		t.Fatalf("提交视图 %s：%v", parcel, err)
	}
}
