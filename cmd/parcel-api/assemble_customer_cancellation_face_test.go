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

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件证取消口经首方渠道在 parcel-api 装配点接通（票 first-party-api/11；ADR-0313）。合成主体、账户只记 `S`。

const cancellationPath = "/v1/shipment-requests/parcel-cancellations"

// customerCancellationBody 是一次取消载荷。extra 追加在对象末尾，供自报字段的用例用。
func customerCancellationBody(accessRequestID, originalID, shipmentRequestID, parcelID, reason, requestedAt, extra string) string {
	return `{
  "accessRequestId": "` + accessRequestID + `",
  "originalAccessRequestId": "` + originalID + `",
  "shipmentRequestId": "` + shipmentRequestID + `",
  "parcelId": "` + parcelID + `",
  "reason": "` + reason + `",
  "requestedAt": "` + requestedAt + `"` + extra + `
}`
}

const cancellationRequestedAt = "2026-08-24T11:00:00Z"

// cancellationFace 是取消口首方面对真库的生产装配：两台持提交与取消授予的客户端（账户 01、02）与一台只持提交授予的（账户 03）。
type cancellationFace struct {
	db       *bentopg.DB
	pool     *pgxpool.Pool
	requests *pspostgres.ShipmentRequests
	issuer   customerIssuerDouble
	submit   http.Handler
	cancel   http.Handler
}

func newCancellationFace(t *testing.T) *cancellationFace {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	appendIsolatedAuthorityInterval(t, db)
	registerCustomerCommandAdmissionFor(t, db, isolatedSelfAuthority, accessidentity.CustomerCapabilityParcelCancellation)
	registry, err := aipg.NewCustomerClientRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	submission, cancellation := accessidentity.CustomerCapabilityShipmentRequestSubmission, accessidentity.CustomerCapabilityParcelCancellation
	registerPreAdviceClient(t, db, registry, syntheticCustomerSubject, syntheticCustomerAccount,
		map[accessidentity.CustomerCapability]string{submission: "SYN-GRANT-CX-SUBMISSION-01", cancellation: "SYN-GRANT-CX-CANCELLATION-01"})
	registerPreAdviceClient(t, db, registry, "SYN-CUSTOMER-CLIENT-02", "SYN-ACCOUNT-02",
		map[accessidentity.CustomerCapability]string{submission: "SYN-GRANT-CX-SUBMISSION-02", cancellation: "SYN-GRANT-CX-CANCELLATION-02"})
	registerPreAdviceClient(t, db, registry, "SYN-CUSTOMER-CLIENT-03", "SYN-ACCOUNT-03",
		map[accessidentity.CustomerCapability]string{submission: "SYN-GRANT-CX-SUBMISSION-03"})

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
	cancellationIntake, err := buildCustomerCancellationIntake(minter)
	if err != nil {
		t.Fatal(err)
	}
	cancellationOrchestration, err := buildCancellationOrchestration(db)
	if err != nil {
		t.Fatal(err)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatal(err)
	}
	return &cancellationFace{
		db: db, pool: pool, requests: requests, issuer: issuer,
		submit: shipmenthttp.NewSubmitShipmentRequestEndpoint(submissionIntake, submissionOrchestration),
		cancel: shipmenthttp.NewCancelParcelEndpoint(cancellationIntake, cancellationOrchestration),
	}
}

func (face *cancellationFace) post(t *testing.T, endpoint http.Handler, path, subject, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+face.issuer.token(t, syntheticCustomerAudience, subject))
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

// submitted 经首方提交口交一单，交回它的回执要素与来源身份。
type submittedRequest struct {
	original, requestID, parcelID string
	identity                      domain.SourceIdentity
}

func (face *cancellationFace) submitOne(t *testing.T, original string) submittedRequest {
	t.Helper()
	response := face.post(t, face.submit, "/v1/shipment-requests", syntheticCustomerSubject, customerDraftBody(original))
	if response.Code != http.StatusCreated || outcomeOf(t, response) != "SUBMITTED" {
		t.Fatalf("首方提交 = %d %s, want 201 SUBMITTED", response.Code, response.Body.String())
	}
	requestID, _, parcelID := receiptOf(t, response)
	identity, err := domain.NewSourceIdentity(
		mustValue(t, domain.NewTenantID, "SYN-TENANT-01"), mustValue(t, domain.NewCustomerAccountID, syntheticCustomerAccount),
		mustValue(t, domain.NewSource, syntheticCustomerIssuer+"#"+syntheticCustomerSubject), mustValue(t, domain.NewSourceRequestKey, original))
	if err != nil {
		t.Fatal(err)
	}
	return submittedRequest{original: original, requestID: requestID, parcelID: parcelID, identity: identity}
}

// accept 把委托经领域 Decide 推到已接受（接受不在首方面内，配方见 acceptedOnRealAssembly）。
func (face *cancellationFace) accept(t *testing.T, submitted submittedRequest) {
	t.Helper()
	stored, found, err := face.requests.FindBySourceIdentity(t.Context(), submitted.identity)
	if err != nil || !found {
		t.Fatalf("读回委托：err=%v found=%v", err, found)
	}
	acceptAndSave(t, face.db, face.requests, shipmentapp.SubmitShipmentRequestCommand{
		Identity: submitted.identity, DeclaredParcelIDs: []domain.DeclaredParcelID{mustValue(t, domain.NewDeclaredParcelID, submitted.parcelID)},
	}, stored)
}

func (face *cancellationFace) count(t *testing.T, query string) int {
	t.Helper()
	return countRows(t, face.pool, query)
}

type cancellationAnswer struct {
	Outcome        string `json:"outcome"`
	CancellationID string `json:"cancellationId"`
	RefusalBasis   string `json:"refusalBasis"`
	PendingReason  string `json:"pendingReason"`
	Continuation   string `json:"continuationReference"`
}

func answerOf(t *testing.T, response *httptest.ResponseRecorder) cancellationAnswer {
	t.Helper()
	var body cancellationAnswer
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	return body
}

// registerCancellationRule 经 PC 的发布登记册登一版授权规则及它的取消授权目录（SYN-TENANT-01 / SYN-SCOPE-01，与演示种子里
// SYN-AUTH-CANCEL-01 同形）。parties 是目录里有行的请求方格。
func registerCancellationRule(t *testing.T, db *bentopg.DB, objectID string, parties ...pcdomain.DeclaredCancellationParty) {
	t.Helper()
	approvedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	approval, err := pcdomain.NewApprovalBasis(
		mustValue(t, pcdomain.NewApprovalReference, "SYN-APPROVAL-"+objectID),
		mustValue(t, pcdomain.NewCommercialSourceReference, "SYN-SOURCE-"+objectID), approvedAt)
	if err != nil {
		t.Fatal(err)
	}
	version, err := pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
		TenantID: mustValue(t, pcdomain.NewTenantID, "SYN-TENANT-01"), Kind: pcdomain.AuthorizationRuleObject,
		ObjectID:      mustValue(t, pcdomain.NewCommercialObjectID, objectID),
		Version:       mustValue(t, pcdomain.NewCommercialVersionLabel, "v1"),
		Scope:         mustValue(t, pcdomain.NewCommercialScopeReference, "SYN-SCOPE-01"),
		ContentDigest: mustValue(t, pcdomain.NewCommercialContentDigest, "sha256:"+objectID),
		Effective:     interval, Status: pcdomain.CommercialVersionEffective, Approval: approval,
		PublishedAt: approvedAt, EffectiveAt: approvedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	var rows []pcdomain.CancellationAuthorityDeclaration
	for _, party := range parties {
		rows = append(rows, pcdomain.CancellationAuthorityDeclaration{
			Party: party, Rule: mustValue(t, pcdomain.NewRuleReference, "SYN-RULE-CANCEL-"+party.String()+"-"+objectID)})
	}
	content, err := pcdomain.NewCancellationAuthorityContent(version, rows)
	if err != nil {
		t.Fatal(err)
	}
	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if outcome, err := publications.SaveVersion(txCtx, version); err != nil || outcome != pcports.PublicationSaved {
			return errors.Join(err, errors.New("save version: "+outcome.String()))
		}
		if outcome, err := publications.SaveCancellationAuthority(txCtx, content); err != nil || outcome != pcports.DeclarationSaved {
			return errors.Join(err, errors.New("save cancellation authority: "+outcome.String()))
		}
		return nil
	}); err != nil {
		t.Fatalf("登授权规则 %s：%v", objectID, err)
	}
}

// Covers: AT-PS-144、AT-PS-147 与票 first-party-api/11 完成判据——真库上经生产装配的同一组构造函数：演示形态的客户端持提交与取消两项
// 授予，租户登了「客户可取消」的授权规则（与演示种子 SYN-AUTH-CANCEL-01 同形）、账户登了解析键，先经首方提交口交一单、(测试里)接受之后，
// 经 `/v1/shipment-requests/parcel-cancellations` 在收寄前取消其包裹：答 201 PARCEL_CANCELLED 带取消决定标识，一行取消、一份发布意图；
// 原样重放答 200 EXISTING_RESULT 带同一取消决定标识，换一个取消请求标识同内容重放也读回同一份；同一委托同一包裹改原因答
// REQUEST_CONFLICT，不覆盖；他账户指名这份委托与指名不存在的委托同答 404 同字节；回执委托号对不上答 404；包裹不属这份委托答 404；
// 只持提交授予的客户端答 403 CUSTOMER_CLIENT_NOT_GRANTED；载荷自报账户、租户、来源或取消者，或缺请求时刻，答 400。失败的请求一行取消都不落。
func TestACustomerClientCancelsThroughTheFirstPartyLineAgainstARealDatabase(t *testing.T) {
	face := newCancellationFace(t)
	registerSelfServiceKey(t, face.db)
	registerCancellationRule(t, face.db, "SYN-AUTH-CANCEL-01", pcdomain.DeclaredCustomerCancellation, pcdomain.DeclaredOperationsCancellation)
	target := face.submitOne(t, "SYN-ACCESS-REQUEST-CX-0001")
	face.accept(t, target)
	body := func(accessRequestID string) string {
		return customerCancellationBody(accessRequestID, target.original, target.requestID, target.parcelID, "SYN-REASON-CHANGED-MIND", cancellationRequestedAt, "")
	}
	post := func(subject, payload string) *httptest.ResponseRecorder {
		return face.post(t, face.cancel, cancellationPath, subject, payload)
	}

	// 他账户：指名这份委托与指名不存在的委托同答 404 同字节，不泄露存在。
	foreign := post("SYN-CUSTOMER-CLIENT-02", body("SYN-ACCESS-REQUEST-CX-C0"))
	absent := post("SYN-CUSTOMER-CLIENT-02", customerCancellationBody("SYN-ACCESS-REQUEST-CX-C0B", "SYN-ACCESS-REQUEST-NO-SUCH", target.requestID, target.parcelID, "SYN-REASON-CHANGED-MIND", cancellationRequestedAt, ""))
	if foreign.Code != http.StatusNotFound || absent.Code != http.StatusNotFound || problemCode(t, foreign) != "SHIPMENT_REQUEST_NOT_VISIBLE" ||
		problemCode(t, foreign) != problemCode(t, absent) || withoutRequestID(t, foreign) != withoutRequestID(t, absent) {
		t.Fatalf("他账户 = %d %s；不存在 = %d %s；want 同为 404 SHIPMENT_REQUEST_NOT_VISIBLE 同字节", foreign.Code, foreign.Body.String(), absent.Code, absent.Body.String())
	}
	// 回执委托号对不上、包裹不属这份委托：按找不到答，与不存在同字节。
	wrongID := post(syntheticCustomerSubject, customerCancellationBody("SYN-ACCESS-REQUEST-CX-C1", target.original, "SHR-SOMEONE-ELSES", target.parcelID, "SYN-REASON-CHANGED-MIND", cancellationRequestedAt, ""))
	wrongParcel := post(syntheticCustomerSubject, customerCancellationBody("SYN-ACCESS-REQUEST-CX-C2", target.original, target.requestID, "SYN-PARCEL-NOT-OURS", "SYN-REASON-CHANGED-MIND", cancellationRequestedAt, ""))
	for name, response := range map[string]*httptest.ResponseRecorder{"委托号不符": wrongID, "包裹不属": wrongParcel} {
		if response.Code != http.StatusNotFound || withoutRequestID(t, response) != withoutRequestID(t, absent) {
			t.Fatalf("%s = %d %s, want 404 同字节", name, response.Code, response.Body.String())
		}
	}
	// 只持提交授予：未授予。
	notGranted := post("SYN-CUSTOMER-CLIENT-03", body("SYN-ACCESS-REQUEST-CX-C3"))
	if notGranted.Code != http.StatusForbidden || problemCode(t, notGranted) != "CUSTOMER_CLIENT_NOT_GRANTED" {
		t.Fatalf("未授予 = %d %s, want 403 CUSTOMER_CLIENT_NOT_GRANTED", notGranted.Code, notGranted.Body.String())
	}
	// 载荷自报账户 / 租户 / 来源 / 取消者：未知字段，整份拒。缺请求时刻同样 400。
	for _, field := range []string{`"customerAccountId": "SYN-ACCOUNT-02"`, `"tenantId": "SYN-TENANT-02"`, `"source": "SYN-SOURCE-X"`, `"requester": "SYN-SOMEONE"`} {
		selfReported := post(syntheticCustomerSubject, customerCancellationBody("SYN-ACCESS-REQUEST-CX-C4", target.original, target.requestID, target.parcelID, "SYN-REASON-CHANGED-MIND", cancellationRequestedAt, ",\n  "+field))
		if selfReported.Code != http.StatusBadRequest || problemCode(t, selfReported) != "MALFORMED_REQUEST" {
			t.Fatalf("自报 %s = %d %s, want 400 MALFORMED_REQUEST", field, selfReported.Code, selfReported.Body.String())
		}
	}
	if noTime := post(syntheticCustomerSubject, customerCancellationBody("SYN-ACCESS-REQUEST-CX-C5", target.original, target.requestID, target.parcelID, "SYN-REASON-CHANGED-MIND", "", "")); noTime.Code != http.StatusBadRequest {
		t.Fatalf("缺请求时刻 = %d %s, want 400", noTime.Code, noTime.Body.String())
	}
	if n := face.count(t, cancellationRows); n != 0 {
		t.Fatalf("失败的请求落了 %d 行取消", n)
	}

	// 成功：收寄前取消。
	formed := post(syntheticCustomerSubject, body("SYN-ACCESS-REQUEST-CX-C6"))
	if formed.Code != http.StatusCreated || answerOf(t, formed).Outcome != "PARCEL_CANCELLED" || answerOf(t, formed).CancellationID == "" {
		t.Fatalf("取消 = %d %s, want 201 PARCEL_CANCELLED 带 cancellationId", formed.Code, formed.Body.String())
	}
	if n := face.count(t, cancellationRows); n != 1 {
		t.Fatalf("取消记录 %d 行, want 1", n)
	}
	if n := face.count(t, cancellationHandoffs); n != 1 {
		t.Fatalf("发布意图 %d 份, want 1", n)
	}

	// 原样重放：已有结果，同一份取消。换一个取消请求标识同内容重放也读回同一份（幂等键是委托加包裹，不是取消请求标识）。
	again := post(syntheticCustomerSubject, body("SYN-ACCESS-REQUEST-CX-C6"))
	other := post(syntheticCustomerSubject, body("SYN-ACCESS-REQUEST-CX-C7"))
	for name, response := range map[string]*httptest.ResponseRecorder{"原样重放": again, "换标识重放": other} {
		if response.Code != http.StatusOK || answerOf(t, response).Outcome != "EXISTING_RESULT" || answerOf(t, response).CancellationID != answerOf(t, formed).CancellationID {
			t.Fatalf("%s = %d %s, want 200 EXISTING_RESULT 带同一 cancellationId", name, response.Code, response.Body.String())
		}
	}
	// 同一委托同一包裹改原因（或改请求时刻）：冲突，不覆盖。
	changedReason := post(syntheticCustomerSubject, customerCancellationBody("SYN-ACCESS-REQUEST-CX-C6", target.original, target.requestID, target.parcelID, "SYN-REASON-OTHER", cancellationRequestedAt, ""))
	changedTime := post(syntheticCustomerSubject, customerCancellationBody("SYN-ACCESS-REQUEST-CX-C6", target.original, target.requestID, target.parcelID, "SYN-REASON-CHANGED-MIND", "2026-08-24T11:00:01Z", ""))
	for name, response := range map[string]*httptest.ResponseRecorder{"改原因": changedReason, "改请求时刻": changedTime} {
		if answerOf(t, response).Outcome != "REQUEST_CONFLICT" {
			t.Fatalf("%s = %d %s, want REQUEST_CONFLICT", name, response.Code, response.Body.String())
		}
	}
	if n := face.count(t, cancellationRows); n != 1 {
		t.Fatalf("重放与冲突之后取消记录 %d 行, want 仍是 1", n)
	}
}

// Covers: AT-PS-148 与票 first-party-api/11 完成判据——生命周期闸门经首方取消口：目标委托已撤回、已拒绝答 200 REQUEST_ENDED，只是已提交
// 答 200 REQUEST_AWAITING_DECISION（HTTP 状态与首方族其它口一致，业务答复不译 409）；被拦时取消库与发布意图一行都不落。闸门在授权之前，
// 所以本例不登授权规则也答这两格，不答「授权未配置」。
func TestACustomerCancellationAgainstANonAcceptedRequestIsAnsweredByTheLifecycleGate(t *testing.T) {
	face := newCancellationFace(t)
	submitted := face.submitOne(t, "SYN-ACCESS-REQUEST-CX-1001")
	withdrawn := face.submitOne(t, "SYN-ACCESS-REQUEST-CX-1002")
	rejected := face.submitOne(t, "SYN-ACCESS-REQUEST-CX-1003")

	for _, ended := range []struct {
		target submittedRequest
		move   func(domain.ShipmentRequest) (domain.ShipmentRequest, error)
	}{
		{withdrawn, func(request domain.ShipmentRequest) (domain.ShipmentRequest, error) {
			return request.WithdrawByCustomer(domain.WithdrawalSpec{
				DecisionID: mustValue(t, domain.NewAcceptanceDecisionID, "SYN-WITHDRAWAL-1"),
				Authority:  mustValue(t, domain.NewWithdrawalAuthorityReference, "SYN-WITHDRAW-ROLE-1"),
				Requester:  mustValue(t, domain.NewWithdrawalRequesterReference, "SYN-CUSTOMER-1"),
				Reason:     mustValue(t, domain.NewWithdrawalReasonReference, "SYN-NO-LONGER-REQUIRED"),
				DecidedAt:  time.Date(2026, 8, 21, 10, 30, 0, 0, time.UTC),
			})
		}},
		{rejected, func(request domain.ShipmentRequest) (domain.ShipmentRequest, error) {
			return request.RejectByAuthority(domain.ActiveRejectionSpec{
				DecisionID: mustValue(t, domain.NewAcceptanceDecisionID, "SYN-REJECTION-1"),
				Authority:  mustValue(t, domain.NewRejectionAuthorityReference, "SYN-REJECT-ROLE-1"),
				Decider:    mustValue(t, domain.NewDeciderReference, "SYN-OPERATOR-1"),
				Reason:     mustValue(t, domain.NewRejectionReasonReference, "SYN-NOT-UNDERTAKEN"),
				Evidence:   mustValue(t, domain.NewRejectionEvidenceReference, "SYN-EVIDENCE-1"),
				DecidedAt:  time.Date(2026, 8, 21, 10, 30, 0, 0, time.UTC),
			})
		}},
	} {
		stored, found, err := face.requests.FindBySourceIdentity(t.Context(), ended.target.identity)
		if err != nil || !found {
			t.Fatalf("读回委托：err=%v found=%v", err, found)
		}
		moved, err := ended.move(stored)
		if err != nil {
			t.Fatalf("领域转移：%v", err)
		}
		if err := face.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			saved, err := face.requests.Save(txCtx, ended.target.identity, moved)
			if err == nil && saved != ports.ShipmentRequestSaved {
				err = errors.New("save outcome")
			}
			return err
		}); err != nil {
			t.Fatalf("落委托：%v", err)
		}
	}

	for name, tc := range map[string]struct {
		target  submittedRequest
		outcome string
	}{
		"已撤回":   {withdrawn, "REQUEST_ENDED"},
		"已拒绝":   {rejected, "REQUEST_ENDED"},
		"只是已提交": {submitted, "REQUEST_AWAITING_DECISION"},
	} {
		response := face.post(t, face.cancel, cancellationPath, syntheticCustomerSubject, customerCancellationBody(
			"SYN-ACCESS-REQUEST-CX-X-"+tc.target.original, tc.target.original, tc.target.requestID, tc.target.parcelID, "SYN-REASON-CHANGED-MIND", cancellationRequestedAt, ""))
		if response.Code != http.StatusOK || answerOf(t, response).Outcome != tc.outcome {
			t.Fatalf("%s = %d %s, want 200 %s", name, response.Code, response.Body.String(), tc.outcome)
		}
	}
	if n := face.count(t, cancellationRows); n != 0 {
		t.Fatalf("被闸门拦下的请求落了 %d 行取消", n)
	}
	if n := face.count(t, cancellationHandoffs); n != 0 {
		t.Fatalf("被闸门拦下的请求交了 %d 份发布意图", n)
	}
}

// Covers: AT-PS-149 与票 first-party-api/11 取舍 C——取消授权是租户取值，产品不代拟：账户没登解析键、登了解析键而该范围没有登记取消授权
// 目录的授权规则，都答 200 CANCELLATION_UNDECIDED 停在 AUTHORITY_UNCONFIGURED 带续办引用，且什么都不落（不默认放行，也不默认拒绝）；
// 登了目录而目录里没有客户这一行，是一份真的拒绝（CANCELLATION_REFUSED，依据指名客户请求方）；同一范围同一接受时点登了两版取消授权目录，
// 取消说不出听哪一版，答 CANCELLATION_UNDECIDED 停在 AUTHORITY_UNAVAILABLE（登记册要收尾，不是授权服务要修）。
func TestTheCustomerCancellationIsTheTenantsToConfigureAndSaysSoWhenItIsNot(t *testing.T) {
	face := newCancellationFace(t)
	first := face.submitOne(t, "SYN-ACCESS-REQUEST-CX-2001")
	face.accept(t, first)
	second := face.submitOne(t, "SYN-ACCESS-REQUEST-CX-2002")
	face.accept(t, second)
	cancel := func(target submittedRequest, accessRequestID string) *httptest.ResponseRecorder {
		return face.post(t, face.cancel, cancellationPath, syntheticCustomerSubject, customerCancellationBody(
			accessRequestID, target.original, target.requestID, target.parcelID, "SYN-REASON-CHANGED-MIND", cancellationRequestedAt, ""))
	}
	requireUnconfigured := func(step string, response *httptest.ResponseRecorder) {
		t.Helper()
		answer := answerOf(t, response)
		if response.Code != http.StatusOK || answer.Outcome != "CANCELLATION_UNDECIDED" || answer.PendingReason != "AUTHORITY_UNCONFIGURED" || answer.Continuation == "" {
			t.Fatalf("%s = %d %s, want 200 CANCELLATION_UNDECIDED AUTHORITY_UNCONFIGURED 带续办引用", step, response.Code, response.Body.String())
		}
		if n := face.count(t, cancellationRows); n != 0 {
			t.Fatalf("%s 之后落了 %d 行取消", step, n)
		}
	}

	requireUnconfigured("没登解析键", cancel(first, "SYN-ACCESS-REQUEST-CX-A1"))
	registerSelfServiceKey(t, face.db)
	requireUnconfigured("登了解析键而没有取消授权目录", cancel(first, "SYN-ACCESS-REQUEST-CX-A2"))

	// 目录里只有运营这一行：客户没被放行，是一份真的拒绝。
	registerCancellationRule(t, face.db, "SYN-AUTH-CANCEL-OPS-ONLY", pcdomain.DeclaredOperationsCancellation)
	refused := cancel(first, "SYN-ACCESS-REQUEST-CX-A3")
	if answer := answerOf(t, refused); refused.Code != http.StatusOK || answer.Outcome != "CANCELLATION_REFUSED" || !strings.Contains(answer.RefusalBasis, "CUSTOMER") {
		t.Fatalf("目录无客户行 = %d %s, want 200 CANCELLATION_REFUSED 依据指名客户请求方", refused.Code, refused.Body.String())
	}

	// 同范围又登一版带目录的授权规则：两版并存，取消说不出听哪一版（另一份委托，免得读到上面那份拒绝）。
	registerCancellationRule(t, face.db, "SYN-AUTH-CANCEL-CUSTOMER", pcdomain.DeclaredCustomerCancellation)
	conflicted := cancel(second, "SYN-ACCESS-REQUEST-CX-A4")
	if answer := answerOf(t, conflicted); conflicted.Code != http.StatusOK || answer.Outcome != "CANCELLATION_UNDECIDED" || answer.PendingReason != "AUTHORITY_UNAVAILABLE" {
		t.Fatalf("两版目录并存 = %d %s, want 200 CANCELLATION_UNDECIDED AUTHORITY_UNAVAILABLE", conflicted.Code, conflicted.Body.String())
	}
}
