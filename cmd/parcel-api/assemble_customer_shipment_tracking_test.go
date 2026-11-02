package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件证按运单号查询那一口在 parcel-api 装配点的几件（票 document-numbering/12 完成判据「cmd/parcel-api 真库测试」；ADR-0356）：
// 新口挂在首方客户族、带 /v1、要自己那个词的授予；首方族未配置时答未配置、一个字节不读；他族令牌与只持按包裹查授予的客户端各答
// 认证那一格；真库上经登记处这一特性的生产 build 与 mount，一份委托从已提交走到已接受、视图形成。合成主体、账户、包裹与号只记 `S`。

const customerShipmentTrackingPath = "/v1/customer-shipment-tracking"

// untouchedShipments 是整单读口的替身：被问一次就判失败——Intake 没过的请求不该走到读委托这一步。
type untouchedShipments struct{ t *testing.T }

func (shipments untouchedShipments) LocateCustomerShipment(
	context.Context, vedomain.TenantID, vedomain.CustomerAccountReference, vedomain.ShipmentDocumentNumberReference, veports.ShipmentParcelPage,
) (veports.LocatedCustomerShipment, error) {
	shipments.t.Error("Intake 没过却读了委托")
	return veports.LocatedCustomerShipment{}, nil
}

func shipmentTrackingIntakeWith(
	t *testing.T,
	verifier accessidentity.CustomerClientCredentialVerifier,
	registry accessidentity.CustomerClientRegistry,
) *visibilityhttp.CustomerShipmentTrackingIntake {
	t.Helper()
	minter, err := buildCustomerClientMinter(verifier, registry, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	intake, err := buildCustomerShipmentTrackingIntake(minter)
	if err != nil {
		t.Fatal(err)
	}
	return intake
}

func shipmentTrackingEndpointWith(t *testing.T, intake *visibilityhttp.CustomerShipmentTrackingIntake) http.Handler {
	t.Helper()
	return visibilityhttp.NewQueryCustomerShipmentTrackingEndpoint(customerShipmentTrackingQueryIntake(intake), untouchedShipments{t}, untouchedViews{t}, untouchedAttributions{t})
}

func getCustomerLine(endpoint http.Handler, path, token, query string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path+query, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

// Covers: ADR-0356 决定一与 ADR-0140 决定二——新口在登记处是首方客户族的一行、带 /v1、授予是 CUSTOMER_SHIPMENT_TRACKING_QUERY，不是按
// 包裹查那一个词。
func TestTheShipmentTrackingLineIsAFirstPartyLineOnItsOwnWord(t *testing.T) {
	line, declared := declaredEndpointLines()[customerShipmentTrackingPath]
	if !declared {
		t.Fatalf("%s 不在登记处", customerShipmentTrackingPath)
	}
	if line.method != http.MethodGet || line.face.family != faceCustomerClient ||
		line.face.capability != accessidentity.CustomerCapabilityShipmentTrackingQuery || !line.ownIntake {
		t.Fatalf("%s 的登记 = %+v", customerShipmentTrackingPath, line)
	}
}

// Covers: 首方族三件不设时答 403 ACCESS_CHANNEL_NOT_CONFIGURED——Intake 缺席与三件都没设时装出的首方 Intake 答同一份字节，委托一次都
// 没读。
func TestShipmentTrackingLineAnswersNotConfiguredByteForByteWhenTheCustomerFamilyIsUnset(t *testing.T) {
	verifier, configured, err := buildCustomerClientCredentialVerifier(fakeGetenv(nil))
	if err != nil || configured {
		t.Fatalf("unset parameters: configured=%v err=%v", configured, err)
	}
	for _, intake := range []*visibilityhttp.CustomerShipmentTrackingIntake{nil, shipmentTrackingIntakeWith(t, verifier, absentCustomerRegister{})} {
		response := getCustomerLine(shipmentTrackingEndpointWith(t, intake), customerShipmentTrackingPath, "any.presented.token", "?documentNumber=P20261102000001")
		if !answersFirstPartyUnconfigured(t, response) {
			t.Fatalf("intake %v：%d %q, want 403 %s", intake != nil, response.Code, response.Body.String(), firstPartyUnconfigured)
		}
	}
}

// Covers: 票 document-numbering/12 完成判据「认证、授予各格的形状与按包裹查一致，词不同」——操作者令牌、集成客户端令牌、不带令牌答
// 401；主体不在册、只持按包裹查授予（CUSTOMER_TRACKING_VIEW_QUERY）、只持委托提交授予答 403 CUSTOMER_CLIENT_NOT_GRANTED；同一份出示在
// 按包裹查那一口答同一个状态码与码（只持按包裹查授予那一格除外：它在那一口是放行的）。各格都不带 outcome，委托一次都没读。
func TestShipmentTrackingLineRefusesOtherFamiliesAndClientsWithoutItsWord(t *testing.T) {
	issuer := newCustomerIssuerDouble(t)
	verifier, configured, err := buildCustomerClientCredentialVerifier(fakeGetenv(issuer.environment()))
	if err != nil || !configured {
		t.Fatalf("complete parameters: configured=%v err=%v", configured, err)
	}
	byParcelOnly := trackingViewGrantRegister{capabilities: []accessidentity.CustomerCapability{accessidentity.CustomerCapabilityTrackingViewQuery}}
	submissionOnly := trackingViewGrantRegister{capabilities: []accessidentity.CustomerCapability{accessidentity.CustomerCapabilityShipmentRequestSubmission}}
	for name, testCase := range map[string]struct {
		registry accessidentity.CustomerClientRegistry
		audience string
		status   int
		code     string
		// sameOnTheByParcelLine 为真：同一份出示在按包裹查那一口答同一格。
		sameOnTheByParcelLine bool
	}{
		"operator token":               {submissionOnly, syntheticOperatorAudience, http.StatusUnauthorized, "CUSTOMER_CLIENT_CREDENTIAL_REJECTED", true},
		"integration client token":     {submissionOnly, "idp-parcel-integration", http.StatusUnauthorized, "CUSTOMER_CLIENT_CREDENTIAL_REJECTED", true},
		"no token at all":              {submissionOnly, "", http.StatusUnauthorized, "CUSTOMER_CLIENT_CREDENTIAL_REJECTED", true},
		"customer token, unregistered": {absentCustomerRegister{}, syntheticCustomerAudience, http.StatusForbidden, "CUSTOMER_CLIENT_NOT_GRANTED", true},
		"customer token, submission":   {submissionOnly, syntheticCustomerAudience, http.StatusForbidden, "CUSTOMER_CLIENT_NOT_GRANTED", true},
		"customer token, by-parcel":    {byParcelOnly, syntheticCustomerAudience, http.StatusForbidden, "CUSTOMER_CLIENT_NOT_GRANTED", false},
	} {
		t.Run(name, func(t *testing.T) {
			token := ""
			if testCase.audience != "" {
				token = issuer.token(t, testCase.audience, syntheticCustomerSubject)
			}
			response := getCustomerLine(shipmentTrackingEndpointWith(t, shipmentTrackingIntakeWith(t, verifier, testCase.registry)),
				customerShipmentTrackingPath, token, "?documentNumber=P20261102000001")
			if response.Code != testCase.status || problemCode(t, response) != testCase.code {
				t.Fatalf("answer = %d %s, want %d %s", response.Code, response.Body.String(), testCase.status, testCase.code)
			}
			assertNoOutcome(t, response, customerShipmentTrackingPath)
			if testCase.sameOnTheByParcelLine {
				byParcel := getTrackingView(customerTrackingViewEndpoint(trackingViewIntakeWith(t, verifier, testCase.registry), untouchedViews{t}, untouchedAttributions{t}),
					token, "?parcel=SYN-PARCEL-01")
				if byParcel.Code != response.Code || problemCode(t, byParcel) != problemCode(t, response) {
					t.Fatalf("按包裹查答 %d %s，按运单号答 %d %s", byParcel.Code, byParcel.Body.String(), response.Code, response.Body.String())
				}
			}
		})
	}
}

// productionCustomerTrackingLines 从端点登记处取客户追踪那一特性，经它自己的生产 build 与 mount 装出各行。
func productionCustomerTrackingLines(t *testing.T, sources endpointSources) map[string]http.Handler {
	t.Helper()
	lines := map[string]http.Handler{}
	for _, feature := range endpointRegistry {
		if feature.wiringKey != any((*customerTrackingViewWiring)(nil)) {
			continue
		}
		mounted, err := feature.production(sources, endpointChannels{})
		if err != nil {
			t.Fatalf("装配：%v", err)
		}
		for _, line := range mounted {
			lines[line.Pattern] = line.Handler
		}
	}
	for _, pattern := range []string{customerTrackingViewPath, customerShipmentTrackingPath} {
		if lines[pattern] == nil {
			t.Fatalf("%s 没按登记处挂上", pattern)
		}
	}
	return lines
}

// registerCustomerClientHolding 在真的客户渠道册上登一个绑在（SYN-TENANT-01, account）上的合成客户端，授给定的几项能力。登进去也就
// 证了迁移里那条 CHECK 放开了这些词。
func registerCustomerClientHolding(t *testing.T, db *bentopg.DB, registry *aipg.CustomerClientRegistry, subjectName, account string, capabilities ...accessidentity.CustomerCapability) {
	t.Helper()
	subject, err := accessidentity.NewCustomerClientSubject(syntheticCustomerIssuer, subjectName)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := accessidentity.NewCredentialReference("SYN-CREDENTIAL-STORE/" + subjectName)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := accessidentity.NewCustomerClientBinding(subject, "SYN-TENANT-01", account, reference, false, "SYN-CUSTOMER-CLIENT-BASIS-DN12")
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
		for _, capability := range capabilities {
			grant, err := accessidentity.NewCustomerClientGrant("SYN-TENANT-01", "SYN-GRANT-DN12-"+subjectName+"-"+capability.String(), subject,
				capability, interval, "SYN-CUSTOMER-GRANT-BASIS-DN12")
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
		t.Fatalf("登记合成首方客户端 %s：%v", subjectName, err)
	}
}

// shipmentTrackingCommand 是（SYN-TENANT-01, account）名下一份两件委托：第一件声明了客户包裹件号，第二件没有。
func shipmentTrackingCommand(t *testing.T, account, key string) shipmentapp.SubmitShipmentRequestCommand {
	t.Helper()
	command := submissionCommand(t)
	identity, err := psdomain.NewSourceIdentity(
		mustValue(t, psdomain.NewTenantID, "SYN-TENANT-01"),
		mustValue(t, psdomain.NewCustomerAccountID, account),
		mustValue(t, psdomain.NewSource, "SYN-SOURCE-DN12"),
		mustValue(t, psdomain.NewSourceRequestKey, key),
	)
	if err != nil {
		t.Fatal(err)
	}
	command.Identity = identity
	command.ShipmentRequestID = mustValue(t, psdomain.NewShipmentRequestID, "SYN-REQUEST-"+key)
	first := mustValue(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-"+key+"-1")
	command.DeclaredParcelIDs = []psdomain.DeclaredParcelID{first, mustValue(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-"+key+"-2")}
	declaration, err := psdomain.DeclareCustomerParcelNumber(first, mustValue(t, psdomain.NewExternalIdentifierNumber, "SYN-PCLREF-"+key+"-1"))
	if err != nil {
		t.Fatal(err)
	}
	command.CustomerParcelNumbers = []psdomain.CustomerParcelNumberDeclaration{declaration}
	return command
}

// Covers: `AT-VE-231`（列出与逐字相同的真库半边）、`AT-VE-232`（真库半边）、`AT-VE-233`（已提交与接受后再查）、`AT-VE-234`（页大小）——
// 真库上，经登记处这一特性的生产 build 与 mount（生产用的 PS 委托查阅读面、视图读口与归属反查口）：C1 提交一份两件委托、开立运单号；
// 接受之前 C1 按号查答 AWAITING_ACCEPTANCE；C2 拿 C1 的号、C1 拿本租户没有的号、C1 拿 C2 自己委托的号、C1 拿无号委托的委托标识，答同一
// 份字节 SHIPMENT_NOT_FOUND。接受之后 C1 按号查答 PARCELS_LISTED：两件按提交答复的次序、带 parcelId 与 C1 声明的件号（没声明的那件
// 整键缺席），页大小 50、total 2、没有下一页；视图没形成时两件都答 NO_VISIBLE_RESULT，第一件视图形成之后答 CURRENT_VIEW，每一件与同一
// 时刻按包裹查那一件的答复逐字相同。
func TestACustomerTracksItsShipmentByNumberAgainstARealDatabase(t *testing.T) {
	fixture := newNumberingFixture(t, nil)
	fixture.registerTimeZone(t, "SYN-TENANT-01")
	db := fixture.db
	registry, err := aipg.NewCustomerClientRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	const otherSubject, otherAccount = "SYN-CUSTOMER-CLIENT-DN12-02", "SYN-ACCOUNT-02"
	registerCustomerClientHolding(t, db, registry, syntheticCustomerSubject, syntheticCustomerAccount,
		accessidentity.CustomerCapabilityShipmentTrackingQuery, accessidentity.CustomerCapabilityTrackingViewQuery)
	registerCustomerClientHolding(t, db, registry, otherSubject, otherAccount, accessidentity.CustomerCapabilityShipmentTrackingQuery)

	issuer := newCustomerIssuerDouble(t)
	verifier, _, err := buildCustomerClientCredentialVerifier(fakeGetenv(issuer.environment()))
	if err != nil {
		t.Fatal(err)
	}
	minter, err := buildCustomerClientMinter(verifier, registry, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	lines := productionCustomerTrackingLines(t, endpointSources{db: db, customerClientMinter: minter})
	byNumber, byParcel := lines[customerShipmentTrackingPath], lines[customerTrackingViewPath]
	owner := issuer.token(t, syntheticCustomerAudience, syntheticCustomerSubject)
	other := issuer.token(t, syntheticCustomerAudience, otherSubject)
	query := func(token, number string) *httptest.ResponseRecorder {
		return getCustomerLine(byNumber, customerShipmentTrackingPath, token, "?documentNumber="+url.QueryEscape(number))
	}

	command := shipmentTrackingCommand(t, syntheticCustomerAccount, "DN12-OWN")
	submitted, err := fixture.handler.Handle(t.Context(), command)
	if err != nil || submitted.Outcome() != shipmentapp.OutcomeSubmitted {
		t.Fatalf("提交：%v %v", submitted.Outcome(), err)
	}
	number, numbered := submitted.DocumentNumbering().Number()
	if !numbered {
		t.Fatalf("委托没开立运单号：%+v", submitted.DocumentNumbering())
	}
	otherSubmitted, err := fixture.handler.Handle(t.Context(), shipmentTrackingCommand(t, otherAccount, "DN12-OTHER"))
	if err != nil {
		t.Fatal(err)
	}
	otherNumber, _ := otherSubmitted.DocumentNumbering().Number()

	if awaiting := query(owner, number.String()); awaiting.Code != http.StatusOK || strings.TrimSpace(awaiting.Body.String()) != `{"outcome":"AWAITING_ACCEPTANCE"}` {
		t.Fatalf("接受之前 C1 按号查 = %d %s", awaiting.Code, awaiting.Body.String())
	}

	missing := query(owner, "P20991231999999")
	if missing.Code != http.StatusOK || strings.TrimSpace(missing.Body.String()) != `{"outcome":"SHIPMENT_NOT_FOUND"}` {
		t.Fatalf("本租户没有的号 = %d %s", missing.Code, missing.Body.String())
	}
	for name, probe := range map[string]*httptest.ResponseRecorder{
		"C2 拿 C1 的号":  query(other, number.String()),
		"C1 拿 C2 的号":  query(owner, otherNumber.String()),
		"C1 拿委托标识当号":  query(owner, "SYN-REQUEST-DN12-OWN"),
		"C1 拿大小写折过的号": query(owner, strings.ToLower(number.String())),
	} {
		if probe.Code != missing.Code || probe.Body.String() != missing.Body.String() || !reflect.DeepEqual(probe.Header(), missing.Header()) {
			t.Errorf("%s = %d %v %q，与查无此单 %d %v %q 可分辨", name, probe.Code, probe.Header(), probe.Body.String(), missing.Code, missing.Header(), missing.Body.String())
		}
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
	assertListedLikeByParcel(t, query(owner, number.String()), byParcel, owner,
		[]string{"SYN-PARCEL-DN12-OWN-1", "SYN-PARCEL-DN12-OWN-2"}, []string{"NO_VISIBLE_RESULT", "NO_VISIBLE_RESULT"})

	views, err := vepostgres.NewCustomerViews(db)
	if err != nil {
		t.Fatal(err)
	}
	saveCustomerView(t, db, views, syntheticCustomerAccount, "SYN-PARCEL-DN12-OWN-1", "SYN-VIEW-DN12-01")
	assertListedLikeByParcel(t, query(owner, number.String()), byParcel, owner,
		[]string{"SYN-PARCEL-DN12-OWN-1", "SYN-PARCEL-DN12-OWN-2"}, []string{"CURRENT_VIEW", "NO_VISIBLE_RESULT"})

	if probe := query(other, number.String()); probe.Body.String() != missing.Body.String() {
		t.Fatalf("接受之后 C2 拿 C1 的号 = %s，want 与查无此单同答", probe.Body.String())
	}
}

// assertListedLikeByParcel 断言一份逐件列出：件与次序、页（50、total、没有下一页）、第一件带件号而第二件没有，每一件去掉 parcelId 与
// 件号之后的原文就是按包裹查那一件整个答复去掉开头花括号的那一段。
func assertListedLikeByParcel(t *testing.T, listed *httptest.ResponseRecorder, byParcel http.Handler, token string, parcels, outcomes []string) {
	t.Helper()
	var answer struct {
		Outcome string            `json:"outcome"`
		Parcels []json.RawMessage `json:"parcels"`
		Page    struct {
			Size  int     `json:"size"`
			Next  *string `json:"next"`
			Total int     `json:"total"`
		} `json:"page"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &answer); err != nil {
		t.Fatalf("decode %s: %v", listed.Body.Bytes(), err)
	}
	if listed.Code != http.StatusOK || answer.Outcome != "PARCELS_LISTED" || len(answer.Parcels) != len(parcels) ||
		answer.Page.Size != 50 || answer.Page.Next != nil || answer.Page.Total != len(parcels) {
		t.Fatalf("按号查 = %d %s", listed.Code, listed.Body.String())
	}
	for index, raw := range answer.Parcels {
		var item struct {
			ParcelID                string  `json:"parcelId"`
			CustomerParcelReference *string `json:"customerParcelReference"`
			Outcome                 string  `json:"outcome"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			t.Fatal(err)
		}
		if item.ParcelID != parcels[index] || item.Outcome != outcomes[index] {
			t.Fatalf("第 %d 件 = %s，want %s %s", index, raw, parcels[index], outcomes[index])
		}
		if declared := item.CustomerParcelReference != nil; declared != (index == 0) ||
			(declared && *item.CustomerParcelReference != "SYN-PCLREF-DN12-OWN-1") {
			t.Fatalf("第 %d 件的件号 = %s", index, raw)
		}
		single := getTrackingView(byParcel, token, "?parcel="+parcels[index])
		if single.Code != http.StatusOK || !strings.HasSuffix(string(raw), strings.TrimPrefix(strings.TrimSpace(single.Body.String()), "{")) {
			t.Fatalf("第 %d 件 %s 与按包裹查 %d %s 不逐字相同", index, raw, single.Code, single.Body.String())
		}
	}
}
