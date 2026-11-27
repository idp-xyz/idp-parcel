package warehousehttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

const (
	preAdviceOnBehalfPath   = "/warehouse-fulfillment-inbound-pre-advice-on-behalf"
	preAdviceNoticePath     = "/warehouse-fulfillment-inbound-pre-advice-notice-on-behalf"
	preAdviceWithdrawalPath = "/warehouse-fulfillment-inbound-pre-advice-withdrawal-on-behalf"
)

type countingPreAdvice struct {
	calls  int
	err    error
	result application.AcceptPreAdviceResult
}

func (preAdvices *countingPreAdvice) Accept(context.Context, application.AcceptPreAdviceCommand) (application.AcceptPreAdviceResult, error) {
	preAdvices.calls++
	return preAdvices.result, preAdvices.err
}

func (preAdvices *countingPreAdvice) UpdateNotice(context.Context, application.UpdatePreAdviceNoticeCommand) (application.AcceptPreAdviceResult, error) {
	preAdvices.calls++
	return preAdvices.result, preAdvices.err
}

func (preAdvices *countingPreAdvice) Withdraw(context.Context, application.WithdrawPreAdviceCommand) (application.AcceptPreAdviceResult, error) {
	preAdvices.calls++
	return preAdvices.result, preAdvices.err
}

type fixedPreAdviceAuthenticator struct {
	identity warehousehttp.OperatorIdentity
}

func (authenticator fixedPreAdviceAuthenticator) AuthenticatePreAdviceOnBehalf(context.Context, string) (warehousehttp.OperatorIdentity, error) {
	return authenticator.identity, nil
}

func preAdviceIntake(t *testing.T) warehousehttp.PreAdviceOnBehalfIntake {
	t.Helper()
	intake, err := warehousehttp.NewOperatorPreAdviceIntake(fixedPreAdviceAuthenticator{identity: operatorIdentity(t, tenantA, operatorOne)})
	if err != nil {
		t.Fatal(err)
	}
	return intake
}

func postPreAdvice(handler http.Handler, target, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

const preAdviceSubmitBody = `{
	"ownerAccount": "SYN-OWNER-01",
	"decisionId": "SYN-DECISION-01",
	"evidenceReference": "SYN-EVIDENCE-01",
	"warehouseCode": "SYN-WH-01",
	"arrivalMode": "PALLET",
	"inboundMode": "BY_GOODS",
	"expectedAt": "2026-11-21T00:00:00Z",
	"goodsLines": [{"goodsCode": "SYN-GOODS-01", "unit": "件", "quantity": 2}]
}`

// Covers: 缺依据引用、不认识的到仓方式或入库方式答 400，登记编排不被调用，理由不回显取值。
// 代录提交指名的货主不在租户答 404，不带 outcome。
func TestPreAdviceOnBehalfRejectsABadPayloadBeforeTheRegisterAndHidesAnUnknownOwner(t *testing.T) {
	preAdvices := &countingPreAdvice{}
	accept := warehousehttp.NewAcceptPreAdviceOnBehalfEndpoint(preAdviceIntake(t), preAdvices)

	missing := strings.Replace(preAdviceSubmitBody, `"evidenceReference": "SYN-EVIDENCE-01",`, "", 1)
	response := postPreAdvice(accept, preAdviceOnBehalfPath, missing)
	detail := assertProblem(t, "missing evidence", response, http.StatusBadRequest, "MALFORMED_REQUEST")
	if preAdvices.calls != 0 {
		t.Fatalf("缺依据引用仍进了登记编排 %d 次", preAdvices.calls)
	}
	if reason := stringMember(t, detail, "detail"); !strings.Contains(reason, "evidenceReference") || strings.Contains(reason, "SYN-EVIDENCE") {
		t.Fatalf("detail = %q", reason)
	}

	unknownArrival := strings.Replace(preAdviceSubmitBody, `"arrivalMode": "PALLET"`, `"arrivalMode": "SYN-NOT-AN-ARRIVAL"`, 1)
	response = postPreAdvice(accept, preAdviceOnBehalfPath, unknownArrival)
	detail = assertProblem(t, "unknown arrival", response, http.StatusBadRequest, "MALFORMED_REQUEST")
	if preAdvices.calls != 0 || strings.Contains(response.Body.String(), "SYN-NOT-AN-ARRIVAL") {
		t.Fatalf("到仓方式：calls=%d body=%s detail=%s", preAdvices.calls, response.Body.String(), detail)
	}

	unknownInbound := strings.Replace(preAdviceSubmitBody, `"inboundMode": "BY_GOODS"`, `"inboundMode": "SYN-NOT-AN-INBOUND"`, 1)
	response = postPreAdvice(accept, preAdviceOnBehalfPath, unknownInbound)
	assertProblem(t, "unknown inbound", response, http.StatusBadRequest, "MALFORMED_REQUEST")
	if preAdvices.calls != 0 || strings.Contains(response.Body.String(), "SYN-NOT-AN-INBOUND") {
		t.Fatalf("入库方式：calls=%d body=%s", preAdvices.calls, response.Body.String())
	}

	preAdvices.err = application.ErrOwnerAccountNotInTenant
	unknown := postPreAdvice(accept, preAdviceOnBehalfPath, preAdviceSubmitBody)
	assertProblem(t, "owner not visible", unknown, http.StatusNotFound, "OWNER_ACCOUNT_NOT_VISIBLE")
	if preAdvices.calls != 1 || strings.Contains(unknown.Body.String(), "outcome") {
		t.Fatalf("货主不可见：calls=%d body=%s", preAdvices.calls, unknown.Body.String())
	}
}

// Covers: 告知更新或撤回的载荷带了 ownerAccount 答 400。撤回再带决定标识也答 400。登记编排不被调用。
// 告知更新不问货主在不在租户：那一错不映成 404。
func TestPreAdviceNoticeAndWithdrawalRefuseAnOwnerCarriedInThePayload(t *testing.T) {
	preAdvices := &countingPreAdvice{}
	intake := preAdviceIntake(t)
	notice := warehousehttp.NewUpdatePreAdviceNoticeOnBehalfEndpoint(intake, preAdvices)
	withdrawal := warehousehttp.NewWithdrawPreAdviceOnBehalfEndpoint(intake, preAdvices)

	noticeBody := `{
		"preAdviceId": "IPA-1",
		"decisionId": "SYN-DECISION-N1",
		"evidenceReference": "SYN-EVIDENCE-01",
		"expectedAt": "2026-11-22T00:00:00Z",
		"ownerAccount": "SYN-OWNER-PROBE"
	}`
	response := postPreAdvice(notice, preAdviceNoticePath, noticeBody)
	assertProblem(t, "notice owner", response, http.StatusBadRequest, "MALFORMED_REQUEST")
	if preAdvices.calls != 0 || strings.Contains(response.Body.String(), "SYN-OWNER-PROBE") {
		t.Fatalf("告知更新：calls=%d body=%s", preAdvices.calls, response.Body.String())
	}

	withdrawalBody := `{
		"preAdviceId": "IPA-1",
		"evidenceReference": "SYN-EVIDENCE-01",
		"ownerAccount": "SYN-OWNER-PROBE"
	}`
	response = postPreAdvice(withdrawal, preAdviceWithdrawalPath, withdrawalBody)
	assertProblem(t, "withdrawal owner", response, http.StatusBadRequest, "MALFORMED_REQUEST")
	if preAdvices.calls != 0 || strings.Contains(response.Body.String(), "SYN-OWNER-PROBE") {
		t.Fatalf("撤回货主：calls=%d body=%s", preAdvices.calls, response.Body.String())
	}

	withDecision := `{
		"preAdviceId": "IPA-1",
		"evidenceReference": "SYN-EVIDENCE-01",
		"decisionId": "SYN-DECISION-PROBE"
	}`
	response = postPreAdvice(withdrawal, preAdviceWithdrawalPath, withDecision)
	assertProblem(t, "withdrawal decision", response, http.StatusBadRequest, "MALFORMED_REQUEST")
	if preAdvices.calls != 0 || strings.Contains(response.Body.String(), "SYN-DECISION-PROBE") {
		t.Fatalf("撤回决定标识：calls=%d body=%s", preAdvices.calls, response.Body.String())
	}

	preAdvices.err = application.ErrOwnerAccountNotInTenant
	plain := `{
		"preAdviceId": "IPA-1",
		"decisionId": "SYN-DECISION-N1",
		"evidenceReference": "SYN-EVIDENCE-01",
		"expectedAt": "2026-11-22T00:00:00Z"
	}`
	response = postPreAdvice(notice, preAdviceNoticePath, plain)
	assertProblem(t, "notice does not ask the owner", response, http.StatusInternalServerError, "NO_ANSWER_FORMED")
	if strings.Contains(response.Body.String(), "OWNER_ACCOUNT_NOT_VISIBLE") || strings.Contains(response.Body.String(), "not registered") {
		t.Fatalf("告知更新把货主不可见答成了探针：%s", response.Body.String())
	}
}

func TestTheUnconfiguredIntakeAnswersPreAdviceLinesWithoutReadingAnything(t *testing.T) {
	accept := warehousehttp.NewAcceptPreAdviceOnBehalfEndpoint(warehousehttp.UnconfiguredIntake{}, nil)
	list := warehousehttp.NewQueryPreAdvicesEndpoint(warehousehttp.UnconfiguredIntake{}, nil)
	one := warehousehttp.NewQueryPreAdviceEndpoint(warehousehttp.UnconfiguredIntake{}, nil)
	response := postPreAdvice(accept, preAdviceOnBehalfPath, preAdviceSubmitBody)
	assertProblem(t, "write", response, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED")
	for name, handler := range map[string]http.Handler{
		"list": list,
		"one":  one,
	} {
		response = get(handler, "/warehouse-fulfillment-inbound-pre-advices?ownerAccount=SYN-OWNER-01")
		assertProblem(t, name, response, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED")
	}
}

type scriptedPreAdviceQueries struct {
	list          []application.PreAdviceReading
	last          cataloguepage.Query
	one           application.PreAdviceReading
	oneFound      bool
	byNumber      application.PreAdviceReading
	byNumberFound bool
	calls         int
}

func readingOf(advice domain.PreAdvice) application.PreAdviceReading {
	seen := map[string]bool{}
	cumulative := make([]application.CumulativeReceived, 0)
	for _, line := range advice.GoodsLines() {
		if seen[line.Goods().String()] {
			continue
		}
		seen[line.Goods().String()] = true
		cumulative = append(cumulative, application.CumulativeReceived{Goods: line.Goods(), Quantity: 0})
	}
	return application.PreAdviceReading{Advice: advice, Receipts: []domain.RecordedReceipt{}, Cumulative: cumulative}
}

func (queries *scriptedPreAdviceQueries) ListPreAdvices(
	_ context.Context, _ domain.TenantID, _ domain.OwnerAccountID, _ int, query cataloguepage.Query,
) (cataloguepage.Result[application.PreAdviceReading], error) {
	queries.calls++
	queries.last = query
	return cataloguepage.Result[application.PreAdviceReading]{Rows: queries.list, Total: int64(len(queries.list))}, nil
}

func (queries *scriptedPreAdviceQueries) PreAdviceForOwner(
	context.Context, domain.TenantID, domain.OwnerAccountID, domain.PreAdviceID,
) (application.PreAdviceReading, bool, error) {
	queries.calls++
	return queries.one, queries.oneFound, nil
}

func (queries *scriptedPreAdviceQueries) PreAdviceByDocumentNumber(
	context.Context, domain.TenantID, domain.DocumentNumber,
) (application.PreAdviceReading, bool, error) {
	queries.calls++
	return queries.byNumber, queries.byNumberFound, nil
}

func TestPreAdviceReadsOmitAnEmptyNumberAndKeepNoticeHistoryOnTheSingleRead(t *testing.T) {
	intake, err := warehousehttp.NewOperatorOperationsReadIntake(&fakeReadAuthenticator{tenant: valueOf(t, domain.NewTenantID, tenantA)})
	if err != nil {
		t.Fatal(err)
	}
	unnumbered := samplePreAdvice(t, "")
	numbered := samplePreAdvice(t, "IB-20261120-00001")
	queries := &scriptedPreAdviceQueries{
		list: []application.PreAdviceReading{readingOf(unnumbered)}, one: readingOf(unnumbered), oneFound: true,
		byNumber: readingOf(numbered), byNumberFound: true,
	}
	list := warehousehttp.NewQueryPreAdvicesEndpoint(intake, queries)
	one := warehousehttp.NewQueryPreAdviceEndpoint(intake, queries)
	byNumber := warehousehttp.NewQueryPreAdviceByNumberEndpoint(intake, queries)

	listed := get(list, "/warehouse-fulfillment-inbound-pre-advices?ownerAccount=SYN-OWNER-01")
	if listed.Code != http.StatusOK {
		t.Fatalf("列表 %d %s", listed.Code, listed.Body.String())
	}
	if strings.Contains(listed.Body.String(), "receivedQuantity") || strings.Contains(listed.Body.String(), `"documentNumber"`) {
		t.Fatalf("列表不该带单据号或另一套实收字段：%s", listed.Body.String())
	}
	if !strings.Contains(listed.Body.String(), `"cumulativeReceived"`) {
		t.Fatalf("列表没带累计实收：%s", listed.Body.String())
	}
	listObject := decodeObject(t, listed)
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(listObject["preAdvices"], &rows); err != nil || len(rows) != 1 {
		t.Fatalf("列表行：%v %s", err, listObject["preAdvices"])
	}
	if _, present := rows[0]["notices"]; present {
		t.Fatal("列表不该带告知的历史版")
	}
	if _, present := rows[0]["documentNumber"]; present {
		t.Fatal("没有单据号不该答空串或键")
	}
	if stringMember(t, rows[0], "numberingAbsence") != "NUMBERING_TIME_ZONE_NOT_CONFIGURED" {
		t.Fatalf("无号原因 = %s", rows[0]["numberingAbsence"])
	}
	var current map[string]json.RawMessage
	if err := json.Unmarshal(rows[0]["currentNotice"], &current); err != nil {
		t.Fatal(err)
	}
	if jsonNumber(t, current, "version") != 2 {
		t.Fatalf("现行告知 = %s", current["version"])
	}

	single := get(one, "/warehouse-fulfillment-inbound-pre-advice?ownerAccount=SYN-OWNER-01&preAdviceId=IPA-1")
	singleObject := decodeObject(t, single)
	if stringMember(t, singleObject, "outcome") != "PRE_ADVICE" || string(singleObject["found"]) != "true" {
		t.Fatalf("单份 = %s", single.Body.String())
	}
	var advice map[string]json.RawMessage
	if err := json.Unmarshal(singleObject["preAdvice"], &advice); err != nil {
		t.Fatal(err)
	}
	var notices []map[string]json.RawMessage
	if err := json.Unmarshal(advice["notices"], &notices); err != nil || len(notices) != 2 {
		t.Fatalf("告知版本 = %s", advice["notices"])
	}
	if jsonNumber(t, notices[0], "version") != 1 || jsonNumber(t, notices[1], "version") != 2 {
		t.Fatalf("版本顺序 = %s %s", notices[0]["version"], notices[1]["version"])
	}

	missed := &scriptedPreAdviceQueries{}
	missing := get(warehousehttp.NewQueryPreAdviceByNumberEndpoint(intake, missed),
		"/warehouse-fulfillment-inbound-pre-advice-by-number?documentNumber=IB-MISSING")
	missingObject := decodeObject(t, missing)
	if string(missingObject["found"]) != "false" {
		t.Fatalf("按号找不到 = %s", missing.Body.String())
	}
	if _, present := missingObject["preAdvice"]; present {
		t.Fatal("找不到不该带预报")
	}

	found := get(byNumber, "/warehouse-fulfillment-inbound-pre-advice-by-number?documentNumber=IB-20261120-00001")
	foundObject := decodeObject(t, found)
	var numberedBody map[string]json.RawMessage
	if err := json.Unmarshal(foundObject["preAdvice"], &numberedBody); err != nil {
		t.Fatal(err)
	}
	if stringMember(t, numberedBody, "documentNumber") != "IB-20261120-00001" {
		t.Fatalf("单据号 = %s", numberedBody["documentNumber"])
	}
	if _, present := numberedBody["numberingAbsence"]; present {
		t.Fatal("有单据号不该再带无号原因")
	}

	queries.calls = 0
	bad := get(list, "/warehouse-fulfillment-inbound-pre-advices?ownerAccount=SYN-OWNER-01&nope=1")
	assertProblem(t, "unknown query key", bad, http.StatusBadRequest, "MALFORMED_REQUEST")
	if queries.calls != 0 {
		t.Fatal("坏查询不该问读口")
	}

	narrowed := get(list, "/warehouse-fulfillment-inbound-pre-advices?ownerAccount=SYN-OWNER-01&status=ADVISED")
	if narrowed.Code != http.StatusOK {
		t.Fatalf("按状态 %d %s", narrowed.Code, narrowed.Body.String())
	}
	if got := queries.last.Filters["status"]; len(got) != 1 || got[0] != "ADVISED" {
		t.Fatalf("状态没交给读口：%v", queries.last.Filters)
	}
	queries.calls = 0
	unknown := get(list, "/warehouse-fulfillment-inbound-pre-advices?ownerAccount=SYN-OWNER-01&status=NOPE")
	assertProblem(t, "unknown status", unknown, http.StatusBadRequest, "MALFORMED_REQUEST")
	if queries.calls != 0 {
		t.Fatal("词表外的状态不该问读口")
	}
}

func jsonNumber(t *testing.T, object map[string]json.RawMessage, key string) int {
	t.Helper()
	var number int
	if err := json.Unmarshal(object[key], &number); err != nil {
		t.Fatalf("%s = %s", key, object[key])
	}
	return number
}

func samplePreAdvice(t *testing.T, documentNumber string) domain.PreAdvice {
	t.Helper()
	registrant := valueOf(t, domain.NewRegistrant, operatorOne)
	submission, err := domain.NewOnBehalfPreAdviceSubmission(
		valueOf(t, domain.NewRequestKey, "SYN-DECISION-01"), valueOf(t, domain.NewEvidenceReference, "SYN-EVIDENCE-01"))
	if err != nil {
		t.Fatal(err)
	}
	line, err := domain.NewGoodsInboundLine(
		valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), valueOf(t, domain.NewGoodsUnitName, "件"),
		2, domain.BatchNumber{}, domain.ExpiryDate{}, domain.CartonCount{})
	if err != nil {
		t.Fatal(err)
	}
	notice, err := domain.NewPreAdviceNotice(1, firstInstant.Add(24*time.Hour), domain.ArrivalClue{}, domain.OwnerReference{}, submission, registrant, firstInstant)
	if err != nil {
		t.Fatal(err)
	}
	var number domain.DocumentNumber
	absence := domain.NumberingTimeZoneNotConfigured
	if documentNumber != "" {
		number = valueOf(t, domain.NewDocumentNumber, documentNumber)
		absence = domain.NumberingAbsenceNone
	}
	advice, err := domain.NewAdvisedPreAdvice(
		valueOf(t, domain.NewTenantID, tenantA), valueOf(t, domain.NewPreAdviceID, "IPA-1"),
		valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"), valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		domain.ArrivalPallet, domain.InboundByGoods, []domain.GoodsInboundLine{line}, nil,
		number, absence, submission, registrant, firstInstant, notice)
	if err != nil {
		t.Fatal(err)
	}
	second, err := domain.NewOnBehalfPreAdviceSubmission(
		valueOf(t, domain.NewRequestKey, "SYN-DECISION-02"), valueOf(t, domain.NewEvidenceReference, "SYN-EVIDENCE-02"))
	if err != nil {
		t.Fatal(err)
	}
	clue, err := domain.NewArrivalClue("SYN-CLUE")
	if err != nil {
		t.Fatal(err)
	}
	later, err := domain.NewPreAdviceNotice(2, firstInstant.Add(48*time.Hour), clue, domain.OwnerReference{}, second, registrant, laterInstant)
	if err != nil {
		t.Fatal(err)
	}
	advice, err = advice.AppendNotice(later)
	if err != nil {
		t.Fatal(err)
	}
	return advice
}
