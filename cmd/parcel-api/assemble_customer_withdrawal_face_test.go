package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件证撤回口经首方渠道在 parcel-api 装配点接通（票 first-party-api/07）。合成主体、账户只记 `S`。

const withdrawalPath = "/v1/shipment-requests/withdrawals"

// customerWithdrawalBody 是一次撤回载荷：撤回自己的接入请求标识、原提交的接入请求标识、结构化原因；shipmentRequestID 空则不带该字段。
// extra 追加在对象末尾，供自报字段的用例用。
func customerWithdrawalBody(accessRequestID, originalID, shipmentRequestID, reason, extra string) string {
	named := ""
	if shipmentRequestID != "" {
		named = `  "shipmentRequestId": "` + shipmentRequestID + `",` + "\n"
	}
	return `{
  "accessRequestId": "` + accessRequestID + `",
  "originalAccessRequestId": "` + originalID + `",
` + named + `  "reason": "` + reason + `"` + extra + `
}`
}

// Covers: AT-PS-136、AT-PS-137 与票 first-party-api/07 完成判据——真库上经生产装配的同一组构造函数：演示形态的客户端持提交与撤回
// 两项授予，先经首方提交口交两单，再经 `/v1/shipment-requests/withdrawals` 撤回：不带委托号的撤回答 201 FORMED 且委托成为 WITHDRAWN；
// 同一撤回重发答 200 DECISION_ALREADY_FORMED 带同一份撤回决定；撤回之后对原提交重放，提交回执不表达委托状态，仍答 EXISTING_RESULT 与
// 原回执同版本，要读回撤回结果只能重放撤回；另一个账户指名这份委托与指名一份不存在的委托同答 404、同字节，且没改到委托；
// 回执委托号与找到的委托不符同样答 404；同一撤回请求标识改指另一份原提交或改了原因答 SOURCE_CONFLICT；只持提交授予的客户端答
// 403 CUSTOMER_CLIENT_NOT_GRANTED；载荷里自报账户、租户、来源或撤回者答 400；带回执委托号的撤回对另一份委托同样成立。
func TestACustomerClientWithdrawsThroughTheFirstPartyLineAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	appendIsolatedAuthorityInterval(t, db)
	registerCustomerCommandAdmissionFor(t, db, isolatedSelfAuthority, accessidentity.CustomerCapabilityShipmentRequestWithdrawal)
	registry, err := aipg.NewCustomerClientRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	submissionCap, withdrawalCap := accessidentity.CustomerCapabilityShipmentRequestSubmission, accessidentity.CustomerCapabilityShipmentRequestWithdrawal
	grantsFor := func(suffix string, capabilities ...accessidentity.CustomerCapability) map[accessidentity.CustomerCapability]string {
		prefix := map[accessidentity.CustomerCapability]string{
			submissionCap: "SYN-GRANT-WD-SUBMISSION-",
			withdrawalCap: "SYN-GRANT-WD-WITHDRAWAL-",
		}
		granted := map[accessidentity.CustomerCapability]string{}
		for _, capability := range capabilities {
			granted[capability] = prefix[capability] + suffix
		}
		return granted
	}
	registerPreAdviceClient(t, db, registry, syntheticCustomerSubject, syntheticCustomerAccount, grantsFor("01", submissionCap, withdrawalCap))
	registerPreAdviceClient(t, db, registry, "SYN-CUSTOMER-CLIENT-02", "SYN-ACCOUNT-02", grantsFor("02", submissionCap, withdrawalCap))
	registerPreAdviceClient(t, db, registry, "SYN-CUSTOMER-CLIENT-03", "SYN-ACCOUNT-03", grantsFor("03", submissionCap))

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
	withdrawalIntake, err := buildCustomerWithdrawalIntake(minter)
	if err != nil {
		t.Fatal(err)
	}
	withdrawalOrchestration, err := buildCustomerWithdrawalOrchestration(db)
	if err != nil {
		t.Fatal(err)
	}
	submit := shipmenthttp.NewSubmitShipmentRequestEndpoint(submissionIntake, submissionOrchestration)
	withdraw := shipmenthttp.NewWithdrawShipmentRequestEndpoint(withdrawalIntake, withdrawalOrchestration)
	post := func(endpoint http.Handler, path, subject, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+issuer.token(t, syntheticCustomerAudience, subject))
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, request)
		return response
	}
	type withdrawn struct {
		Outcome      string `json:"outcome"`
		RequestState string `json:"requestState"`
		WithdrawalID string `json:"withdrawalId"`
	}
	decode := func(response *httptest.ResponseRecorder) withdrawn {
		var body withdrawn
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
		}
		return body
	}

	const original, second = "SYN-ACCESS-REQUEST-WD-0001", "SYN-ACCESS-REQUEST-WD-0002"
	first := post(submit, "/v1/shipment-requests", syntheticCustomerSubject, customerDraftBody(original))
	if first.Code != http.StatusCreated || outcomeOf(t, first) != "SUBMITTED" {
		t.Fatalf("首方提交 = %d %s, want 201 SUBMITTED", first.Code, first.Body.String())
	}
	requestID, basis, _ := receiptOf(t, first)
	secondSubmission := post(submit, "/v1/shipment-requests", syntheticCustomerSubject, customerDraftBody(second))
	if secondSubmission.Code != http.StatusCreated {
		t.Fatalf("第二单提交 = %d %s", secondSubmission.Code, secondSubmission.Body.String())
	}
	secondID, _, _ := receiptOf(t, secondSubmission)

	// 他账户：指名这份委托与指名一份不存在的委托同答同字节，不泄露存在，也没改到委托。
	foreign := post(withdraw, withdrawalPath, "SYN-CUSTOMER-CLIENT-02", customerWithdrawalBody("SYN-ACCESS-REQUEST-WD-W0", original, "", "SYN-REASON-1", ""))
	absent := post(withdraw, withdrawalPath, "SYN-CUSTOMER-CLIENT-02", customerWithdrawalBody("SYN-ACCESS-REQUEST-WD-W0B", "SYN-ACCESS-REQUEST-NO-SUCH", "", "SYN-REASON-1", ""))
	if foreign.Code != http.StatusNotFound || absent.Code != http.StatusNotFound ||
		problemCode(t, foreign) != problemCode(t, absent) || withoutRequestID(t, foreign) != withoutRequestID(t, absent) {
		t.Fatalf("他账户指名 = %d %s；不存在 = %d %s；want 同为 404 同字节", foreign.Code, foreign.Body.String(), absent.Code, absent.Body.String())
	}

	// 回执委托号与找到的委托不符：按找不到答，与不存在同字节。
	wrongID := post(withdraw, withdrawalPath, syntheticCustomerSubject, customerWithdrawalBody("SYN-ACCESS-REQUEST-WD-W0C", original, secondID, "SYN-REASON-1", ""))
	if wrongID.Code != http.StatusNotFound || withoutRequestID(t, wrongID) != withoutRequestID(t, absent) {
		t.Fatalf("委托号不符 = %d %s, want 404 同字节", wrongID.Code, wrongID.Body.String())
	}

	// 只持提交授予：未授予。
	notGranted := post(withdraw, withdrawalPath, "SYN-CUSTOMER-CLIENT-03", customerWithdrawalBody("SYN-ACCESS-REQUEST-WD-W0D", original, "", "SYN-REASON-1", ""))
	if notGranted.Code != http.StatusForbidden || problemCode(t, notGranted) != "CUSTOMER_CLIENT_NOT_GRANTED" {
		t.Fatalf("未授予 = %d %s, want 403 CUSTOMER_CLIENT_NOT_GRANTED", notGranted.Code, notGranted.Body.String())
	}

	// 载荷自报账户 / 租户 / 来源 / 撤回者：未知字段，整份拒。
	for _, field := range []string{`"customerAccountId": "SYN-ACCOUNT-02"`, `"tenantId": "SYN-TENANT-02"`, `"source": "SYN-SOURCE-X"`, `"requester": "SYN-SOMEONE"`} {
		selfReported := post(withdraw, withdrawalPath, syntheticCustomerSubject,
			customerWithdrawalBody("SYN-ACCESS-REQUEST-WD-W0E", original, "", "SYN-REASON-1", ",\n  "+field))
		if selfReported.Code != http.StatusBadRequest || problemCode(t, selfReported) != "MALFORMED_REQUEST" {
			t.Fatalf("自报 %s = %d %s, want 400 MALFORMED_REQUEST", field, selfReported.Code, selfReported.Body.String())
		}
	}
	// 缺原因：400。
	if noReason := post(withdraw, withdrawalPath, syntheticCustomerSubject, customerWithdrawalBody("SYN-ACCESS-REQUEST-WD-W0F", original, "", "", "")); noReason.Code != http.StatusBadRequest {
		t.Fatalf("缺原因 = %d %s, want 400", noReason.Code, noReason.Body.String())
	}

	// 成功：不带委托号的撤回形成撤回，委托成为 WITHDRAWN。
	formed := post(withdraw, withdrawalPath, syntheticCustomerSubject, customerWithdrawalBody("SYN-ACCESS-REQUEST-WD-W1", original, "", "SYN-REASON-1", ""))
	if formed.Code != http.StatusCreated || decode(formed).Outcome != "FORMED" || decode(formed).RequestState != "WITHDRAWN" || decode(formed).WithdrawalID == "" {
		t.Fatalf("撤回 = %d %s, want 201 FORMED WITHDRAWN 带 withdrawalId", formed.Code, formed.Body.String())
	}

	// 重发同一撤回：已有决定，带同一份撤回。
	again := post(withdraw, withdrawalPath, syntheticCustomerSubject, customerWithdrawalBody("SYN-ACCESS-REQUEST-WD-W1", original, "", "SYN-REASON-1", ""))
	if again.Code != http.StatusOK || decode(again).Outcome != "DECISION_ALREADY_FORMED" || decode(again).RequestState != "WITHDRAWN" ||
		decode(again).WithdrawalID != decode(formed).WithdrawalID {
		t.Fatalf("重发撤回 = %d %s, want 200 DECISION_ALREADY_FORMED 带同一 withdrawalId %s", again.Code, again.Body.String(), decode(formed).WithdrawalID)
	}
	// 换一个撤回请求标识再撤同一份：也读回原撤回，不形成第二次。
	other := post(withdraw, withdrawalPath, syntheticCustomerSubject, customerWithdrawalBody("SYN-ACCESS-REQUEST-WD-W1B", original, requestID, "SYN-REASON-1", ""))
	if other.Code != http.StatusOK || decode(other).Outcome != "DECISION_ALREADY_FORMED" || decode(other).WithdrawalID != decode(formed).WithdrawalID {
		t.Fatalf("另一标识再撤 = %d %s, want 200 DECISION_ALREADY_FORMED 带原 withdrawalId", other.Code, other.Body.String())
	}

	// 同一撤回请求标识改了原因 / 改指另一份原提交：来源冲突，不当重放。
	conflictReason := post(withdraw, withdrawalPath, syntheticCustomerSubject, customerWithdrawalBody("SYN-ACCESS-REQUEST-WD-W1", original, "", "SYN-REASON-2", ""))
	conflictOriginal := post(withdraw, withdrawalPath, syntheticCustomerSubject, customerWithdrawalBody("SYN-ACCESS-REQUEST-WD-W1", second, "", "SYN-REASON-1", ""))
	conflictNamed := post(withdraw, withdrawalPath, syntheticCustomerSubject, customerWithdrawalBody("SYN-ACCESS-REQUEST-WD-W1", original, requestID, "SYN-REASON-1", ""))
	for name, response := range map[string]*httptest.ResponseRecorder{"改原因": conflictReason, "改指原提交": conflictOriginal, "补带委托号": conflictNamed} {
		if decode(response).Outcome != "SOURCE_CONFLICT" {
			t.Fatalf("%s = %d %s, want SOURCE_CONFLICT", name, response.Code, response.Body.String())
		}
	}
	// 冲突没有牵连另一份原提交：它仍可撤，带回执委托号也成立。
	secondWithdrawn := post(withdraw, withdrawalPath, syntheticCustomerSubject, customerWithdrawalBody("SYN-ACCESS-REQUEST-WD-W2", second, secondID, "SYN-REASON-1", ""))
	if secondWithdrawn.Code != http.StatusCreated || decode(secondWithdrawn).Outcome != "FORMED" || decode(secondWithdrawn).WithdrawalID == decode(formed).WithdrawalID {
		t.Fatalf("另一份委托的撤回 = %d %s, want 201 FORMED 且是另一份撤回决定", secondWithdrawn.Code, secondWithdrawn.Body.String())
	}

	// AT-PS-137：撤回之后对原提交重放，提交回执不带委托状态，仍答 EXISTING_RESULT 与原回执同一版本；撤回态只能经撤回的重放读回。
	replayed := post(submit, "/v1/shipment-requests", syntheticCustomerSubject, customerDraftBody(original))
	if replayed.Code != http.StatusOK || outcomeOf(t, replayed) != "EXISTING_RESULT" || versionOf(t, replayed) != basis {
		t.Fatalf("撤回之后重放原提交 = %d %s, want 200 EXISTING_RESULT 带原版本 %s", replayed.Code, replayed.Body.String(), basis)
	}
	if strings.Contains(strings.ToLower(replayed.Body.String()), "withdrawn") {
		t.Fatalf("提交回执表达了委托状态：%s；契约里「提交回执不表达状态」要随之改", replayed.Body.String())
	}
}
