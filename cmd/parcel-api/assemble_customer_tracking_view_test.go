package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证客户追踪视图查阅口在 parcel-api 装配点的几件（票 demo-chain-gaps/96 完成判据）：首方族未配置时答未配置、一个字节不读；
// 他族令牌答令牌不过、没有这一格授予答未授予；真库上经生产装配的同一组构造函数，合成客户端读本账户包裹的视图答当前视图，别的账户的
// 包裹与不存在的包裹同答；查阅不判准入范围。合成主体、账户与包裹只记 `S`。

const customerTrackingViewPath = "/customer-tracking-view"

// customerTrackingViewEndpoint 按端点表那一行的写法装：Intake 经 customerTrackingViewQueryIntake 取，nil 即字面量未配置。
func customerTrackingViewEndpoint(intake *visibilityhttp.CustomerTrackingViewIntake, reader visibilityhttp.TrackingViewReader) http.Handler {
	return visibilityhttp.NewQueryCustomerTrackingViewEndpoint(customerTrackingViewQueryIntake(intake), reader)
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

// Covers: 票 demo-chain-gaps/96 完成判据「首方族三件不设时仍答 403 ACCESS_CHANNEL_NOT_CONFIGURED」——Intake 缺席（字面量未配置）与
// 三件都没设时装出的首方 Intake 答同一份字节，视图库一次都没读。
func TestCustomerTrackingViewLineAnswersNotConfiguredByteForByteWhenTheCustomerFamilyIsUnset(t *testing.T) {
	verifier, configured, err := buildCustomerClientCredentialVerifier(fakeGetenv(nil))
	if err != nil || configured {
		t.Fatalf("unset parameters: configured=%v err=%v", configured, err)
	}
	const unconfigured = `{"error":{"code":"ACCESS_CHANNEL_NOT_CONFIGURED"}}` + "\n"
	for _, intake := range []*visibilityhttp.CustomerTrackingViewIntake{nil, trackingViewIntakeWith(t, verifier, absentCustomerRegister{})} {
		response := getTrackingView(customerTrackingViewEndpoint(intake, untouchedViews{t}), "any.presented.token", "?parcel=SYN-PARCEL-01")
		if response.Code != http.StatusForbidden || response.Body.String() != unconfigured {
			t.Fatalf("intake %v：%d %q, want 403 %q", intake != nil, response.Code, response.Body.String(), unconfigured)
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
			endpoint := customerTrackingViewEndpoint(trackingViewIntakeWith(t, verifier, testCase.registry), untouchedViews{t})
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
	endpoint := customerTrackingViewEndpoint(trackingViewIntakeWith(t, verifier, registry), views)
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

// registerTrackingViewClient 在真的客户渠道册上登一个绑在（SYN-TENANT-01, SYN-ACCOUNT-01）上的合成客户端，只授查阅那一项。登进去
// 也就证了迁移里那条 CHECK 放开了这个词。
func registerTrackingViewClient(t *testing.T, db *bentopg.DB, registry *aipg.CustomerClientRegistry, subject accessidentity.CustomerClientSubject, grantID string) {
	t.Helper()
	reference, err := accessidentity.NewCredentialReference("SYN-CREDENTIAL-STORE/customer-01")
	if err != nil {
		t.Fatal(err)
	}
	binding, err := accessidentity.NewCustomerClientBinding(subject, "SYN-TENANT-01", syntheticCustomerAccount, reference, false, "SYN-CUSTOMER-CLIENT-BASIS-01")
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
