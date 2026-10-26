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
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pgpostgres "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	pgdomain "go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件证补充口经首方渠道在 parcel-api 装配点接通（票 first-party-api/08）。合成主体、账户只记 `S`。

const supplementPath = "/v1/shipment-requests/supplements"

// registerSupplementAdmission 给 SYN-TENANT-01 在补充口上登一行准入范围对照与 authority 的生产权威区间，登法同种子与
// registerCustomerCommandAdmission：补充是在铸造侧判准入的口（ADR-0139 决定八）。
func registerSupplementAdmission(t *testing.T, db *bentopg.DB, authority string) {
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
		FactKind:   accessidentity.CustomerCapabilityShipmentRequestSupplement.String(),
	}
	mapping, err := accessidentity.NewAdmissionScopeMapping("SYN-TENANT-01", "SYN-MAP-CC-SUPPLEMENT", requirement, customerCommandScope, validity, "SYN-BASIS-MAPPING")
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
			ObjectScope: customerCommandScope,
			Capability:  requirement.Capability,
			FactKind:    requirement.FactKind,
			Authority:   authority,
			From:        from,
		})
	})
	if err != nil {
		t.Fatalf("登补充口对照与区间：%v", err)
	}
}

// supplementBody 是对 customerDraftBody 那一单的一次补充：重报整份内容（这里改了收件地址）、带回回执里的三个指名，
// 再加原提交的接入请求标识。extra 追加在对象末尾，供自报字段的用例用。
func supplementBody(accessRequestID, originalID, shipmentRequestID, basis, parcelID, recipientAddress, extra string) string {
	body := `{
  "accessRequestId": "` + accessRequestID + `",
  "originalAccessRequestId": "` + originalID + `",
  "shipmentRequestId": "` + shipmentRequestID + `",
  "basisSubmissionVersionId": "` + basis + `",
  "customerShipmentReference": "SYN-CUSTREF-FPA-0001",
  "requestedServiceProduct": "SYN-PROD-CN-SG-EXPRESS",
  "senderRelation": "SYN-SENDER-01",
  "senderAddress": "合成寄件地址",
  "recipientRelation": "SYN-RECIPIENT-01",
  "recipientAddress": "` + recipientAddress + `",
  "destinationServiceScope": "SYN-DEST/SG",
  "parcels": [{"parcelId": "` + parcelID + `", "customerParcelReference": "SYN-PCLREF-FPA-0001", "declaredWeightValue": "1.50", "declaredWeightUnit": "KG"}]` + extra + `
}`
	return body
}

func receiptOf(t *testing.T, response *httptest.ResponseRecorder) (shipmentRequestID, versionID, parcelID string) {
	t.Helper()
	var body struct {
		ShipmentRequestID   string           `json:"shipmentRequestId"`
		SubmissionVersionID string           `json:"submissionVersionId"`
		Parcels             []answeredParcel `json:"parcels"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Parcels) != 1 {
		t.Fatalf("decode receipt %s: %v", response.Body.Bytes(), err)
	}
	return body.ShipmentRequestID, body.SubmissionVersionID, body.Parcels[0].ParcelID
}

func versionOf(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		SubmissionVersionID string `json:"submissionVersionId"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	return body.SubmissionVersionID
}

// Covers: AT-PS-132、AT-PS-133 与票 first-party-api/08 完成判据——真库上经生产装配的同一组构造函数：演示形态的客户端
// 持提交与补充两项授予，先经首方提交口交一单，再带回执里的指名经 `/v1/shipment-requests/supplements` 补充，答 201
// SUPPLEMENT_RECORDED 且形成不同于基准的新版本；同一补充重发答 200 SUPPLEMENT_ALREADY_HANDLED 同一版本；拿旧基准再补一次答
// SUPPLEMENT_BASIS_STALE；另一个账户的客户端指名这份委托与指名一份不存在的委托同答 404、同字节；只持提交授予的客户端答 403
// CUSTOMER_CLIENT_NOT_GRANTED；载荷里自报账户答 400；成员对不上答 SUPPLEMENT_BOUNDARY_CHANGED；补充之后对原提交重放，回执读回的是
// 补充形成的新版本。
func TestACustomerClientSupplementsThroughTheFirstPartyLineAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	appendIsolatedAuthorityInterval(t, db)
	registerSupplementAdmission(t, db, isolatedSelfAuthority)
	registry, err := aipg.NewCustomerClientRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	both := map[accessidentity.CustomerCapability]string{
		accessidentity.CustomerCapabilityShipmentRequestSubmission: "SYN-GRANT-SU-SUBMISSION-",
		accessidentity.CustomerCapabilityShipmentRequestSupplement: "SYN-GRANT-SU-SUPPLEMENT-",
	}
	grantsFor := func(suffix string, capabilities ...accessidentity.CustomerCapability) map[accessidentity.CustomerCapability]string {
		granted := map[accessidentity.CustomerCapability]string{}
		for _, capability := range capabilities {
			granted[capability] = both[capability] + suffix
		}
		return granted
	}
	submission, supplementCap := accessidentity.CustomerCapabilityShipmentRequestSubmission, accessidentity.CustomerCapabilityShipmentRequestSupplement
	registerPreAdviceClient(t, db, registry, syntheticCustomerSubject, syntheticCustomerAccount, grantsFor("01", submission, supplementCap))
	registerPreAdviceClient(t, db, registry, "SYN-CUSTOMER-CLIENT-02", "SYN-ACCOUNT-02", grantsFor("02", submission, supplementCap))
	registerPreAdviceClient(t, db, registry, "SYN-CUSTOMER-CLIENT-03", "SYN-ACCOUNT-03", grantsFor("03", submission))

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
	supplementIntake, err := buildCustomerSupplementIntake(minter)
	if err != nil {
		t.Fatal(err)
	}
	supplementOrchestration, err := buildCustomerSupplementOrchestration(db)
	if err != nil {
		t.Fatal(err)
	}
	submit := shipmenthttp.NewSubmitShipmentRequestEndpoint(submissionIntake, submissionOrchestration)
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatal(err)
	}
	supplement := shipmenthttp.NewFormNewSubmissionVersionEndpoint(supplementIntake, shipmenthttp.NewCustomerSupplementHandler(requests, supplementOrchestration))
	post := func(endpoint http.Handler, path, subject, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+issuer.token(t, syntheticCustomerAudience, subject))
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, request)
		return response
	}

	const original = "SYN-ACCESS-REQUEST-SU-0001"
	first := post(submit, "/v1/shipment-requests", syntheticCustomerSubject, customerDraftBody(original))
	if first.Code != http.StatusCreated || outcomeOf(t, first) != "SUBMITTED" {
		t.Fatalf("首方提交 = %d %s, want 201 SUBMITTED", first.Code, first.Body.String())
	}
	requestID, basis, parcelID := receiptOf(t, first)

	recorded := post(supplement, supplementPath, syntheticCustomerSubject,
		supplementBody("SYN-ACCESS-REQUEST-SU-0002", original, requestID, basis, parcelID, "合成改后收件地址", ""))
	if recorded.Code != http.StatusCreated || outcomeOf(t, recorded) != "SUPPLEMENT_RECORDED" {
		t.Fatalf("补充 = %d %s, want 201 SUPPLEMENT_RECORDED", recorded.Code, recorded.Body.String())
	}
	formed := versionOf(t, recorded)
	if formed == "" || formed == basis {
		t.Fatalf("补充形成的版本 %q，基准 %q；want 一个不同于基准的新版本", formed, basis)
	}

	// 重发同一补充：同一版本，不形成第二个。
	again := post(supplement, supplementPath, syntheticCustomerSubject,
		supplementBody("SYN-ACCESS-REQUEST-SU-0002", original, requestID, basis, parcelID, "合成改后收件地址", ""))
	if again.Code != http.StatusOK || outcomeOf(t, again) != "SUPPLEMENT_ALREADY_HANDLED" || versionOf(t, again) != formed {
		t.Fatalf("重发补充 = %d %s, want 200 SUPPLEMENT_ALREADY_HANDLED 且版本仍是 %s", again.Code, again.Body.String(), formed)
	}

	// 基准过期：拿换代前的版本再补一次（新的补充请求标识）。
	stale := post(supplement, supplementPath, syntheticCustomerSubject,
		supplementBody("SYN-ACCESS-REQUEST-SU-0003", original, requestID, basis, parcelID, "合成再改收件地址", ""))
	if outcomeOf(t, stale) != "SUPPLEMENT_BASIS_STALE" {
		t.Fatalf("旧基准补充 = %d %s, want SUPPLEMENT_BASIS_STALE", stale.Code, stale.Body.String())
	}

	// 成员对不上：回执之外的包裹号。
	boundary := post(supplement, supplementPath, syntheticCustomerSubject,
		supplementBody("SYN-ACCESS-REQUEST-SU-0004", original, requestID, formed, "DPCL-NOT-A-MEMBER", "合成收件地址", ""))
	if outcomeOf(t, boundary) != "SUPPLEMENT_BOUNDARY_CHANGED" {
		t.Fatalf("成员对不上 = %d %s, want SUPPLEMENT_BOUNDARY_CHANGED", boundary.Code, boundary.Body.String())
	}

	// 他账户：指名这份委托与指名一份不存在的委托同答同字节，不泄露存在。
	foreign := post(supplement, supplementPath, "SYN-CUSTOMER-CLIENT-02",
		supplementBody("SYN-ACCESS-REQUEST-SU-0005", original, requestID, formed, parcelID, "合成收件地址", ""))
	absent := post(supplement, supplementPath, "SYN-CUSTOMER-CLIENT-02",
		supplementBody("SYN-ACCESS-REQUEST-SU-0006", "SYN-ACCESS-REQUEST-NO-SUCH", requestID, formed, parcelID, "合成收件地址", ""))
	if foreign.Code != http.StatusNotFound || absent.Code != http.StatusNotFound ||
		problemCode(t, foreign) != problemCode(t, absent) || withoutRequestID(t, foreign) != withoutRequestID(t, absent) {
		t.Fatalf("他账户指名 = %d %s；不存在 = %d %s；want 同为 404 同字节", foreign.Code, foreign.Body.String(), absent.Code, absent.Body.String())
	}
	// 他账户的失败没有改到委托：账户一的当前版本仍是补充形成的那一版。
	if replayed := post(submit, "/v1/shipment-requests", syntheticCustomerSubject, customerDraftBody(original)); versionOf(t, replayed) != formed {
		t.Fatalf("他账户的失败补充之后，原提交的回执版本 = %q, want %q", versionOf(t, replayed), formed)
	}

	// 委托号与找到的委托不符：按找不到答，与不存在同字节。
	wrongID := post(supplement, supplementPath, syntheticCustomerSubject,
		supplementBody("SYN-ACCESS-REQUEST-SU-0009", original, "SHR-SOMEONE-ELSES", formed, parcelID, "合成收件地址", ""))
	if wrongID.Code != http.StatusNotFound || withoutRequestID(t, wrongID) != withoutRequestID(t, absent) {
		t.Fatalf("委托号不符 = %d %s, want 404 同字节", wrongID.Code, wrongID.Body.String())
	}

	// 同一补充请求标识改指另一份原提交 / 改写成员：来源冲突，不当重放。
	conflictOriginal := post(supplement, supplementPath, syntheticCustomerSubject,
		supplementBody("SYN-ACCESS-REQUEST-SU-0002", "SYN-ACCESS-REQUEST-OTHER", requestID, basis, parcelID, "合成改后收件地址", ""))
	conflictMember := post(supplement, supplementPath, syntheticCustomerSubject,
		supplementBody("SYN-ACCESS-REQUEST-SU-0002", original, requestID, basis, "DPCL-OTHER-MEMBER", "合成改后收件地址", ""))
	for name, response := range map[string]*httptest.ResponseRecorder{"改指原提交": conflictOriginal, "改写成员": conflictMember} {
		if outcomeOf(t, response) != "SUPPLEMENT_SOURCE_CONFLICT" {
			t.Fatalf("%s = %d %s, want SUPPLEMENT_SOURCE_CONFLICT", name, response.Code, response.Body.String())
		}
	}

	// 只持提交授予：未授予。
	notGranted := post(supplement, supplementPath, "SYN-CUSTOMER-CLIENT-03",
		supplementBody("SYN-ACCESS-REQUEST-SU-0007", original, requestID, formed, parcelID, "合成收件地址", ""))
	if notGranted.Code != http.StatusForbidden || problemCode(t, notGranted) != "CUSTOMER_CLIENT_NOT_GRANTED" {
		t.Fatalf("未授予 = %d %s, want 403 CUSTOMER_CLIENT_NOT_GRANTED", notGranted.Code, notGranted.Body.String())
	}

	// 载荷自报账户 / 租户 / 来源：未知字段，整份拒。
	for _, field := range []string{`"customerAccountId": "SYN-ACCOUNT-02"`, `"tenantId": "SYN-TENANT-02"`, `"source": "SYN-SOURCE-X"`} {
		selfReported := post(supplement, supplementPath, syntheticCustomerSubject,
			supplementBody("SYN-ACCESS-REQUEST-SU-0008", original, requestID, formed, parcelID, "合成收件地址", ",\n  "+field))
		if selfReported.Code != http.StatusBadRequest || problemCode(t, selfReported) != "MALFORMED_REQUEST" {
			t.Fatalf("自报 %s = %d %s, want 400 MALFORMED_REQUEST", field, selfReported.Code, selfReported.Body.String())
		}
	}

	// AT-PS-133：补充之后对原提交重放，回执读回的是补充形成的新版本，不是基准。
	replayed := post(submit, "/v1/shipment-requests", syntheticCustomerSubject, customerDraftBody(original))
	if replayed.Code != http.StatusOK || outcomeOf(t, replayed) != "EXISTING_RESULT" || versionOf(t, replayed) != formed {
		t.Fatalf("补充之后重放原提交 = %d %s, want 200 EXISTING_RESULT 带版本 %s", replayed.Code, replayed.Body.String(), formed)
	}
	if _, replayedVersion, _ := receiptOf(t, replayed); replayedVersion == basis {
		t.Fatalf("重放回执仍是补充前的基准版本 %s", basis)
	}
}
