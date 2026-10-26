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
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	pspartycommercial "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	pgpostgres "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	pgdomain "go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件证资料修订口经首方渠道在 parcel-api 装配点接通（票 first-party-api/10；ADR-0311）。合成主体、账户只记 `S`。

const amendmentPath = "/v1/shipment-requests/source-data-amendments"

// registerAmendmentAdmission 给 SYN-TENANT-01 在资料修订口上登准入范围对照与权威区间，登法同种子。
func registerAmendmentAdmission(t *testing.T, db *bentopg.DB, authority string) {
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
	requirement := accessidentity.AdmissionRequirement{
		Capability: accessidentity.CustomerCommandAdmissionCapability,
		FactKind:   accessidentity.CustomerCapabilitySourceDataAmendment.String(),
	}
	mapping, err := accessidentity.NewAdmissionScopeMapping("SYN-TENANT-01", "SYN-MAP-CC-AMENDMENT", requirement, customerCommandScope, validity, "SYN-BASIS-MAPPING")
	if err != nil {
		t.Fatal(err)
	}
	err = db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		outcome, err := mappings.RegisterAdmissionScopeMapping(txCtx, mapping)
		if err != nil {
			return err
		}
		if outcome != accessidentity.AdmissionScopeMappingRecorded {
			return errors.New("admission scope mapping: " + string(outcome))
		}
		return intervals.Append(txCtx, pgdomain.AuthorityInterval{
			ObjectScope: customerCommandScope, Capability: requirement.Capability, FactKind: requirement.FactKind,
			Authority: authority, From: from,
		})
	})
	if err != nil {
		t.Fatalf("登修订口对照与区间：%v", err)
	}
}

// amendmentBody 是对 customerDraftBody 那一单的一次收件地址更正：只带邮编。extra 追加在末尾供自报字段用。
func amendmentBody(accessRequestID, originalID, shipmentRequestID, basis, postalCode, extra string) string {
	return `{
  "accessRequestId": "` + accessRequestID + `",
  "originalAccessRequestId": "` + originalID + `",
  "shipmentRequestId": "` + shipmentRequestID + `",
  "basisVersionId": "` + basis + `",
  "dataGroup": "DELIVERY_PLACE",
  "intent": "CORRECTION",
  "reason": "SYN-REASON-ADDRESS-FIX",
  "postalCode": "` + postalCode + `"` + extra + `
}`
}

// rulePackageOf 是 synAdoptedRulePackage 按租户展开：规则包版本属哪个租户由调用方定。
func rulePackageOf(t *testing.T, tenant string) pcdomain.CommercialVersion {
	t.Helper()
	approvedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	approval, err := pcdomain.NewApprovalBasis(
		mustValue(t, pcdomain.NewApprovalReference, "SYN-APPROVAL-RULES-1"),
		mustValue(t, pcdomain.NewCommercialSourceReference, "SYN-SOURCE-RULES-1"), approvedAt)
	if err != nil {
		t.Fatal(err)
	}
	live, err := pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
		TenantID: mustValue(t, pcdomain.NewTenantID, tenant), Kind: pcdomain.AcceptanceRulePackageObject,
		ObjectID: mustValue(t, pcdomain.NewCommercialObjectID, "SYN-RULES-1"), Version: mustValue(t, pcdomain.NewCommercialVersionLabel, "v1"),
		Scope:         mustValue(t, pcdomain.NewCommercialScopeReference, "SYN-PC-SCOPE-1"),
		ContentDigest: mustValue(t, pcdomain.NewCommercialContentDigest, "sha256:SYN-RULES-1"),
		Effective:     interval, Status: pcdomain.CommercialVersionEffective, Approval: approval,
		PublishedAt: approvedAt, EffectiveAt: approvedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	return live
}

// seedClosureFor 是 seedAdoptedClosure 按来源身份展开：接受时固定的解析 SYN-RES-1 回指到规则包版本。
func seedClosureFor(t *testing.T, db *bentopg.DB, identity domain.SourceIdentity, rules pcdomain.CommercialVersion) {
	t.Helper()
	anchor, err := pcdomain.NewSelectionAnchor(time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC), mustValue(t, pcdomain.NewAnchorPolicyVersion, "SYN-ANCHOR-POLICY-1"))
	if err != nil {
		t.Fatal(err)
	}
	closure, err := pcdomain.RehydrateCommercialClosure(pcdomain.RehydrateCommercialClosureSpec{
		Outcome: pcdomain.UniquelyResolved, ResolutionID: mustValue(t, pcdomain.NewResolutionID, "SYN-RES-1"),
		Key: pcdomain.ClosureResolutionKey{
			TenantID:             mustValue(t, pcdomain.NewTenantID, identity.TenantID().String()),
			CustomerAccountID:    mustValue(t, pcdomain.NewCustomerAccountID, identity.CustomerAccountID().String()),
			LegalEntityCandidate: mustValue(t, pcdomain.NewLegalEntityReference, "SYN-LE-01"),
			Scope:                mustValue(t, pcdomain.NewCommercialScopeReference, "SYN-PC-SCOPE-1"),
			Purpose:              pcdomain.AcceptanceControlPurpose, Anchor: anchor,
			RequiredBases: []pcdomain.CommercialObjectKind{pcdomain.AcceptanceRulePackageObject},
		},
		Anchor: anchor, ViewRevision: mustValue(t, pcdomain.NewAuthorityViewRevision, "SYN-VIEW-1"),
		Adopted: []pcdomain.RehydrateAdoptedBasisSpec{{Kind: pcdomain.AcceptanceRulePackageObject, Version: rules}},
	})
	if err != nil {
		t.Fatal(err)
	}
	resolutions, err := pcpostgres.NewCommercialResolutions(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		outcome, err := resolutions.Save(txCtx, closure)
		if err == nil && outcome != pcports.ResolutionSaved {
			err = errors.New("resolution: " + outcome.String())
		}
		return err
	}); err != nil {
		t.Fatalf("落闭包：%v", err)
	}
}

// registerSelfServiceKey 登记 SYN-TENANT-01 / SYN-ACCOUNT-01 的解析键行（与种子 resolution-key-syn-account-01 同值的最小形）：
// 直客坐标映射的法人与范围取自它。
func registerSelfServiceKey(t *testing.T, db *bentopg.DB) {
	t.Helper()
	keys, err := pspostgres.NewCommercialResolutionKeyStore(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		outcome, err := keys.RegisterResolutionKey(txCtx, pspartycommercial.ResolutionKeyRow{
			TenantID: "SYN-TENANT-01", CustomerAccountID: "SYN-ACCOUNT-01", Scope: "SYN-SCOPE-01", LegalEntity: "SYN-LE-01",
			AnchorPolicy: "SYN-ANCHOR-POLICY-01", AnchorAt: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			RequiredBases: []string{"ACCEPTANCE_RULE_PACKAGE"},
		})
		if err == nil && outcome != pspartycommercial.ResolutionKeySaved {
			err = errors.New("resolution key: " + outcome.String())
		}
		return err
	}); err != nil {
		t.Fatalf("登解析键：%v", err)
	}
}

// seedSelfServiceGrant 在授权册里为 SYN-TENANT-01 登一条 SOURCE_DATA_AMENDMENT × CUSTOMER_SELF_SERVICE 的授予，法人与范围对着解析键。
// 生产的登记入口由票 first-party-api/13 立；在它进 main 之前用例经 PC 领域重建门加 SaveGrant 登。
func seedSelfServiceGrant(t *testing.T, db *bentopg.DB) {
	t.Helper()
	approvedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	approval, err := pcdomain.NewApprovalBasis(
		mustValue(t, pcdomain.NewApprovalReference, "SYN-APPROVAL-SELF-SERVICE"),
		mustValue(t, pcdomain.NewCommercialSourceReference, "SYN-SOURCE-SELF-SERVICE"), approvedAt)
	if err != nil {
		t.Fatal(err)
	}
	version, err := pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
		TenantID: mustValue(t, pcdomain.NewTenantID, "SYN-TENANT-01"), Kind: pcdomain.AuthorizationRuleObject,
		ObjectID:      mustValue(t, pcdomain.NewCommercialObjectID, "SYN-AUTH-SELF-SERVICE-AMENDMENT"),
		Version:       mustValue(t, pcdomain.NewCommercialVersionLabel, "v1"),
		Scope:         mustValue(t, pcdomain.NewCommercialScopeReference, "SYN-SCOPE-01"),
		ContentDigest: mustValue(t, pcdomain.NewCommercialContentDigest, "sha256:SYN-AUTH-SELF-SERVICE-AMENDMENT"),
		Effective:     interval, Status: pcdomain.CommercialVersionEffective, Approval: approval,
		PublishedAt: approvedAt, EffectiveAt: approvedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := pcdomain.NewAuthorityGrant(version, pcdomain.SourceDataAmendmentAction,
		mustValue(t, pcdomain.NewLegalEntityReference, "SYN-LE-01"),
		mustValue(t, pcdomain.NewAuthorityLevel, pspartycommercial.CustomerSelfServiceAuthorityLevel),
		mustValue(t, pcdomain.NewCommercialScopeReference, "SYN-SCOPE-01"), interval)
	if err != nil {
		t.Fatal(err)
	}
	grants, err := pcpostgres.NewAuthorityGrants(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		outcome, err := grants.SaveGrant(txCtx, grant)
		if err == nil && outcome != pcports.GrantSaved {
			err = errors.New("grant: " + outcome.String())
		}
		return err
	}); err != nil {
		t.Fatalf("登授予：%v", err)
	}
}

// Covers: AT-PS-143 与票 first-party-api/10 完成判据——真库上经生产装配的同一组构造函数：演示形态客户端经首方提交口交一单、(测试里)接受之后，
// 经 `/v1/shipment-requests/source-data-amendments` 更正收件邮编。解析键没登记答 200 UNDECIDED 停在授权不可用；登了解析键而授权册
// 没有 CUSTOMER_SELF_SERVICE 授予答 UNDECIDED 停在授权规则未配置（租户没放行，如实）；登了授予之后**同一修订请求原样重放**形成版本答 201
// RECORDED（code/44 的接着办），再重放答 200 ALREADY_HANDLED 同一版本；同一请求标识改内容答 SOURCE_CONFLICT；他账户指名这份委托与指名
// 不存在的委托同答 404 同字节；委托号对不上答 404；只持提交授予的客户端答 403 CUSTOMER_CLIENT_NOT_GRANTED；载荷自报账户 400。
func TestACustomerClientAmendsThroughTheFirstPartyLineAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	appendIsolatedAuthorityInterval(t, db)
	registerAmendmentAdmission(t, db, isolatedSelfAuthority)
	registry, err := aipg.NewCustomerClientRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	submission, amendCap := accessidentity.CustomerCapabilityShipmentRequestSubmission, accessidentity.CustomerCapabilitySourceDataAmendment
	registerPreAdviceClient(t, db, registry, syntheticCustomerSubject, syntheticCustomerAccount,
		map[accessidentity.CustomerCapability]string{submission: "SYN-GRANT-AM-SUBMISSION-01", amendCap: "SYN-GRANT-AM-AMENDMENT-01"})
	registerPreAdviceClient(t, db, registry, "SYN-CUSTOMER-CLIENT-02", "SYN-ACCOUNT-02",
		map[accessidentity.CustomerCapability]string{submission: "SYN-GRANT-AM-SUBMISSION-02", amendCap: "SYN-GRANT-AM-AMENDMENT-02"})
	registerPreAdviceClient(t, db, registry, "SYN-CUSTOMER-CLIENT-03", "SYN-ACCOUNT-03",
		map[accessidentity.CustomerCapability]string{submission: "SYN-GRANT-AM-SUBMISSION-03"})

	issuer := newCustomerIssuerDouble(t)
	verifier, _, err := buildCustomerClientCredentialVerifier(fakeGetenv(issuer.environment()))
	if err != nil {
		t.Fatal(err)
	}
	admission, err := buildAdmissionScope(db, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	minter, err := buildCustomerClientMinter(verifier, registry, admission)
	if err != nil {
		t.Fatal(err)
	}
	isolated := isolatedWriteAdmissionForTest(t)
	ownership, err := buildProductionOwnership(db, isolated, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	submissionIntake, err := buildCustomerSubmissionIntake(minter, ownership)
	if err != nil {
		t.Fatal(err)
	}
	submissionOrchestration, err := buildSubmissionOrchestration(db, isolated, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	amendmentIntake, err := buildCustomerAmendmentIntake(minter)
	if err != nil {
		t.Fatal(err)
	}
	amendmentOrchestration, err := buildCustomerAmendmentOrchestration(db)
	if err != nil {
		t.Fatal(err)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatal(err)
	}
	submit := shipmenthttp.NewSubmitShipmentRequestEndpoint(submissionIntake, submissionOrchestration)
	amend := shipmenthttp.NewAmendCustomerSourceDataEndpoint(amendmentIntake, shipmenthttp.NewCustomerAmendmentHandler(requests, amendmentOrchestration))
	post := func(endpoint http.Handler, path, subject, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+issuer.token(t, syntheticCustomerAudience, subject))
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, request)
		return response
	}

	const original = "SYN-ACCESS-REQUEST-AM-0001"
	first := post(submit, "/v1/shipment-requests", syntheticCustomerSubject, customerDraftBody(original))
	if first.Code != http.StatusCreated || outcomeOf(t, first) != "SUBMITTED" {
		t.Fatalf("首方提交 = %d %s, want 201 SUBMITTED", first.Code, first.Body.String())
	}
	requestID, basis, parcelID := receiptOf(t, first)

	// 委托的接受不在首方面内：测试里经领域 Decide 推到已接受，并给它固定一份回指规则包的闭包与允许声明。
	identity, err := domain.NewSourceIdentity(
		mustValue(t, domain.NewTenantID, "SYN-TENANT-01"), mustValue(t, domain.NewCustomerAccountID, syntheticCustomerAccount),
		mustValue(t, domain.NewSource, syntheticCustomerIssuer+"#"+syntheticCustomerSubject), mustValue(t, domain.NewSourceRequestKey, original))
	if err != nil {
		t.Fatal(err)
	}
	stored, found, err := requests.FindBySourceIdentity(t.Context(), identity)
	if err != nil || !found {
		t.Fatalf("读回委托：err=%v found=%v", err, found)
	}
	acceptAndSave(t, db, requests, shipmentapp.SubmitShipmentRequestCommand{
		Identity: identity, DeclaredParcelIDs: []domain.DeclaredParcelID{mustValue(t, domain.NewDeclaredParcelID, parcelID)},
	}, stored)
	rules := rulePackageOf(t, "SYN-TENANT-01")
	seedClosureFor(t, db, identity, rules)
	declareAmendmentAllowance(t, db, rules, false,
		amendmentCell(t, "DELIVERY_PLACE", pcdomain.DeclaredAcceptedNotYetReceived, pcdomain.DeclaredCorrectionIntent, pcdomain.AmendmentAllowed))

	const amendmentID = "SYN-ACCESS-REQUEST-AM-0002"
	body := amendmentBody(amendmentID, original, requestID, basis, "SYN-POSTAL-NEW", "")
	reasonOf := func(response *httptest.ResponseRecorder) string {
		t.Helper()
		var decoded struct {
			PendingReason string `json:"pendingReason"`
		}
		if err := jsonUnmarshal(response, &decoded); err != nil {
			t.Fatal(err)
		}
		return decoded.PendingReason
	}

	// 解析键没登记：坐标折不出来，授权不可用，如实停下。
	unkeyed := post(amend, amendmentPath, syntheticCustomerSubject, body)
	if unkeyed.Code != http.StatusOK || outcomeOf(t, unkeyed) != "UNDECIDED" || reasonOf(unkeyed) != "SOURCE_DATA_AMENDMENT_AUTHORITY_UNAVAILABLE" {
		t.Fatalf("未登记解析键 = %d %s, want 200 UNDECIDED 授权不可用", unkeyed.Code, unkeyed.Body.String())
	}
	// 登了解析键而授权册没有授予：租户没放行，答授权规则未配置。
	registerSelfServiceKey(t, db)
	ungranted := post(amend, amendmentPath, syntheticCustomerSubject, body)
	if outcomeOf(t, ungranted) != "UNDECIDED" || reasonOf(ungranted) != "SOURCE_DATA_AMENDMENT_AUTHORITY_RULES_NOT_CONFIGURED" {
		t.Fatalf("无授予 = %d %s, want UNDECIDED 授权规则未配置", ungranted.Code, ungranted.Body.String())
	}
	// 登了授予：同一请求原样重放接着办，形成版本。
	seedSelfServiceGrant(t, db)
	recorded := post(amend, amendmentPath, syntheticCustomerSubject, body)
	if recorded.Code != http.StatusCreated || outcomeOf(t, recorded) != "RECORDED" {
		t.Fatalf("登授予后重放 = %d %s, want 201 RECORDED", recorded.Code, recorded.Body.String())
	}
	formed := sourceDataVersionOf(t, recorded)
	if formed == "" {
		t.Fatal("RECORDED 没带版本号")
	}
	if got := envelopeCountOfType(t, db, sourceDataVersionEnvelopeType); got != 1 {
		t.Fatalf("「资料版本已形成」意图 = %d 封, want 1", got)
	}
	// 再重放：已有结果，同一版本。
	again := post(amend, amendmentPath, syntheticCustomerSubject, body)
	if again.Code != http.StatusOK || outcomeOf(t, again) != "ALREADY_HANDLED" || sourceDataVersionOf(t, again) != formed {
		t.Fatalf("重放 = %d %s, want 200 ALREADY_HANDLED 版本 %s", again.Code, again.Body.String(), formed)
	}
	// 同一修订请求标识改内容：来源冲突。
	conflict := post(amend, amendmentPath, syntheticCustomerSubject, amendmentBody(amendmentID, original, requestID, basis, "SYN-POSTAL-OTHER", ""))
	if outcomeOf(t, conflict) != "SOURCE_CONFLICT" {
		t.Fatalf("改内容 = %d %s, want SOURCE_CONFLICT", conflict.Code, conflict.Body.String())
	}

	// 他账户：指名这份委托与指名不存在的委托同答 404 同字节。
	foreign := post(amend, amendmentPath, "SYN-CUSTOMER-CLIENT-02", amendmentBody("SYN-ACCESS-REQUEST-AM-0003", original, requestID, basis, "SYN-POSTAL-X", ""))
	absent := post(amend, amendmentPath, "SYN-CUSTOMER-CLIENT-02", amendmentBody("SYN-ACCESS-REQUEST-AM-0004", "SYN-ACCESS-REQUEST-NO-SUCH", requestID, basis, "SYN-POSTAL-X", ""))
	if foreign.Code != http.StatusNotFound || absent.Code != http.StatusNotFound ||
		problemCode(t, foreign) != problemCode(t, absent) || withoutRequestID(t, foreign) != withoutRequestID(t, absent) {
		t.Fatalf("他账户 = %d %s；不存在 = %d %s；want 同为 404 同字节", foreign.Code, foreign.Body.String(), absent.Code, absent.Body.String())
	}
	// 委托号与找到的委托不符：按找不到答。
	wrongID := post(amend, amendmentPath, syntheticCustomerSubject, amendmentBody("SYN-ACCESS-REQUEST-AM-0005", original, "SHR-SOMEONE-ELSES", basis, "SYN-POSTAL-X", ""))
	if wrongID.Code != http.StatusNotFound || withoutRequestID(t, wrongID) != withoutRequestID(t, absent) {
		t.Fatalf("委托号不符 = %d %s, want 404 同字节", wrongID.Code, wrongID.Body.String())
	}
	// 只持提交授予：未授予。
	notGranted := post(amend, amendmentPath, "SYN-CUSTOMER-CLIENT-03", amendmentBody("SYN-ACCESS-REQUEST-AM-0006", original, requestID, basis, "SYN-POSTAL-X", ""))
	if notGranted.Code != http.StatusForbidden || problemCode(t, notGranted) != "CUSTOMER_CLIENT_NOT_GRANTED" {
		t.Fatalf("未授予 = %d %s, want 403 CUSTOMER_CLIENT_NOT_GRANTED", notGranted.Code, notGranted.Body.String())
	}
	// 载荷自报账户 / 租户 / 来源：整份拒。
	for _, field := range []string{`"customerAccountId": "SYN-ACCOUNT-02"`, `"tenantId": "SYN-TENANT-02"`, `"source": "SYN-SOURCE-X"`} {
		selfReported := post(amend, amendmentPath, syntheticCustomerSubject, amendmentBody("SYN-ACCESS-REQUEST-AM-0007", original, requestID, basis, "SYN-POSTAL-X", ",\n  "+field))
		if selfReported.Code != http.StatusBadRequest || problemCode(t, selfReported) != "MALFORMED_REQUEST" {
			t.Fatalf("自报 %s = %d %s, want 400 MALFORMED_REQUEST", field, selfReported.Code, selfReported.Body.String())
		}
	}
	// 他账户与未授予的失败没有形成任何版本。
	if got := envelopeCountOfType(t, db, sourceDataVersionEnvelopeType); got != 1 {
		t.Fatalf("失败请求之后意图 = %d 封, want 仍是 1", got)
	}
}

func jsonUnmarshal(response *httptest.ResponseRecorder, target any) error {
	return json.Unmarshal(response.Body.Bytes(), target)
}

func sourceDataVersionOf(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		SourceDataVersionID string `json:"sourceDataVersionId"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	return body.SourceDataVersionID
}
