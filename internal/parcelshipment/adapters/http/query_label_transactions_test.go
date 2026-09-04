package shipmenthttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

var labelEstablishedAtFixture = time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)

// labelTransactionIntakeDouble 只交出租户与页大小——本端点的 Intake 接口就只有这些。
// 它不能用 viewsIntakeDouble 顶替，而那正是本次接线要的：委托查阅那个 Intake 交的是带
// 账户维的作用域，把它接进面单交易查阅在编译期就不通过（ADR-0084 决定七：本册没有账户维）。
type labelTransactionIntakeDouble struct {
	tenant domain.TenantID
	limit  int
	err    error
}

func (double *labelTransactionIntakeDouble) IntakeLabelTransactionQuery(
	_ context.Context,
	_ *http.Request,
) (shipmenthttp.LabelTransactionQuery, error) {
	if double.err != nil {
		return shipmenthttp.LabelTransactionQuery{}, double.err
	}
	return shipmenthttp.LabelTransactionQuery{Tenant: double.tenant, Limit: double.limit}, nil
}

func labelTransactionIntake(t *testing.T) *labelTransactionIntakeDouble {
	t.Helper()
	return &labelTransactionIntakeDouble{
		tenant: mustValue(t, domain.NewTenantID, "TENANT-1"),
		limit:  50,
	}
}

type labelTransactionsReaderDouble struct {
	records []ports.LabelTransactionRecord
	err     error
	tenant  domain.TenantID
	limit   int
}

func (double *labelTransactionsReaderDouble) ListLabelTransactions(
	_ context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.LabelTransactionRecord, error) {
	double.tenant = tenant
	double.limit = limit
	if double.err != nil {
		return nil, double.err
	}
	return double.records, nil
}

// labelTransactionRecordFixture 是一笔「部分成功」的交易：一件受理带渠道号并被作废，
// 一件未受理带原因——两层结果与后续动作都在，摊开之后要逐行对得上。两件的继续尝试判断
// 同为开放，但一件关过又重开、一件没有人作过决定：读面交出的这一对布尔要原样过传输层。
func labelTransactionRecordFixture(t *testing.T) ports.LabelTransactionRecord {
	t.Helper()
	return ports.LabelTransactionRecord{
		TransactionID:          mustValue(t, domain.NewLabelTransactionID, "label-txn-1"),
		State:                  domain.LabelTransactionPartiallySucceeded,
		Finalized:              true,
		ChannelAccount:         "channel-account-1",
		AccountHolder:          "party-holder-1",
		ServiceProvider:        "party-channel-1",
		SettlementCounterparty: "party-settlement-1",
		Contract:               "contract-1",
		Rate:                   "rate-1",
		ResponsibilityBasis:    "basis-snapshot-1",
		EstablishedAt:          labelEstablishedAtFixture,
		SubmittedAt:            labelEstablishedAtFixture.Add(time.Minute),
		ResultObservedAt:       labelEstablishedAtFixture.Add(2 * time.Minute),
		PriorTransactionID:     "label-txn-0",
		PriorLinkKind:          domain.LabelTransactionRetry,
		Parcels: []ports.LabelTransactionParcelRow{
			{
				Parcel:                  mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
				HasResult:               true,
				Accepted:                true,
				Identifier:              "channel-parcel-1",
				FollowUpKinds:           []domain.FollowUpActionKind{domain.ChannelVoidAction},
				ContinuedAttemptOpen:    true,
				ContinuedAttemptDecided: true,
			},
			{
				Parcel:               mustValue(t, domain.NewDeclaredParcelID, "parcel-2"),
				HasResult:            true,
				Reason:               "ADDRESS_UNSUPPORTED",
				ContinuedAttemptOpen: true,
			},
		},
	}
}

func getLabelTransactions(
	t *testing.T,
	intake shipmenthttp.LabelTransactionQueryIntake,
	reader shipmenthttp.LabelTransactionsReader,
	method string,
) *httptest.ResponseRecorder {
	t.Helper()
	endpoint := shipmenthttp.NewQueryLabelTransactionsEndpoint(intake, reader)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, httptest.NewRequest(method, "/label-transactions", nil))
	return response
}

type labelTransactionsBody struct {
	Outcome               string `json:"outcome"`
	ContinuedAttemptBasis string `json:"continuedAttemptBasis"`
	Rows                  []struct {
		TransactionID           string   `json:"transactionId"`
		ParcelID                string   `json:"parcelId"`
		AccountHolder           string   `json:"accountHolder"`
		ChannelServicer         string   `json:"channelServicer"`
		SettlementCounterparty  string   `json:"settlementCounterparty"`
		TransactionResult       string   `json:"transactionResult"`
		Finalized               bool     `json:"finalized"`
		HasParcelResult         bool     `json:"hasParcelResult"`
		ParcelAccepted          bool     `json:"parcelAccepted"`
		ParcelIdentifier        string   `json:"parcelIdentifier"`
		ParcelResultReason      string   `json:"parcelResultReason"`
		FollowUpKinds           []string `json:"followUpKinds"`
		ContinuedAttemptOpen    bool     `json:"continuedAttemptOpen"`
		ContinuedAttemptDecided bool     `json:"continuedAttemptDecided"`
		EstablishedAt           string   `json:"establishedAt"`
		PriorTransactionID      string   `json:"priorTransactionId"`
		PriorLinkKind           string   `json:"priorLinkKind"`
	} `json:"rows"`
}

func decodeLabelTransactions(t *testing.T, response *httptest.ResponseRecorder) labelTransactionsBody {
	t.Helper()
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	var body labelTransactionsBody
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response %s: %v", response.Body.Bytes(), err)
	}
	return body
}

// Covers: ADR-0084 决定七——行粒度是交易 × 包裹，摊开在读侧完成；两层结果各自透出、
// 互不折算；后续动作按其范围落到该落的行上；重试出处随行带出。读口只收租户维。
func TestLabelTransactionsListExpandsOneRowPerCoveredParcel(t *testing.T) {
	reader := &labelTransactionsReaderDouble{
		records: []ports.LabelTransactionRecord{labelTransactionRecordFixture(t)},
	}
	response := getLabelTransactions(t, labelTransactionIntake(t), reader, http.MethodGet)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200；body = %s", response.Code, response.Body)
	}
	body := decodeLabelTransactions(t, response)
	if body.Outcome != "LABEL_TRANSACTIONS_LISTED" {
		t.Fatalf("outcome = %q", body.Outcome)
	}
	if len(body.Rows) != 2 {
		t.Fatalf("rows = %d, want 2（一笔交易覆盖两件包裹）", len(body.Rows))
	}

	accepted := body.Rows[0]
	if accepted.TransactionID != "label-txn-1" || accepted.ParcelID != "parcel-1" {
		t.Fatalf("第一行的键不对：%+v", accepted)
	}
	if accepted.TransactionResult != "PARTIALLY_SUCCEEDED" || !accepted.Finalized {
		t.Fatalf("交易级结果透出不对：%+v", accepted)
	}
	if !accepted.HasParcelResult || !accepted.ParcelAccepted || accepted.ParcelIdentifier != "channel-parcel-1" {
		t.Fatalf("受理行透出不对：%+v", accepted)
	}
	if len(accepted.FollowUpKinds) != 1 || accepted.FollowUpKinds[0] != "CHANNEL_VOID" {
		t.Fatalf("后续动作没落到指名的那一行：%+v", accepted.FollowUpKinds)
	}
	if accepted.PriorTransactionID != "label-txn-0" || accepted.PriorLinkKind != "RETRY" {
		t.Fatalf("重试出处没带出：%+v", accepted)
	}
	if accepted.EstablishedAt != labelEstablishedAtFixture.Format(time.RFC3339Nano) {
		t.Fatalf("业务时间透出不对：%q", accepted.EstablishedAt)
	}
	// 两件同为开放，一件有决定历史一件没有：这一对布尔各自过传输层，不折成一个值。
	if !accepted.ContinuedAttemptOpen || !accepted.ContinuedAttemptDecided {
		t.Fatalf("关过又重开的行应透出开放且有决定历史：%+v", accepted)
	}

	refused := body.Rows[1]
	if !refused.HasParcelResult || refused.ParcelAccepted || refused.ParcelResultReason != "ADDRESS_UNSUPPORTED" {
		t.Fatalf("未受理行透出不对：%+v", refused)
	}
	if len(refused.FollowUpKinds) != 0 {
		t.Fatalf("指名范围的后续动作落到了范围外的行上：%+v", refused.FollowUpKinds)
	}
	if !refused.ContinuedAttemptOpen || refused.ContinuedAttemptDecided {
		t.Fatalf("没有人作过决定的行应透出开放且无决定历史：%+v", refused)
	}
	if body.ContinuedAttemptBasis != "DERIVED_FROM_DECISION_REGISTER_AND_CURRENT_FINAL" {
		t.Fatalf("继续尝试判断的派生依据不对：%q", body.ContinuedAttemptBasis)
	}

	if reader.tenant.String() != "TENANT-1" || reader.limit != 50 {
		t.Fatalf("读口收到的作用域不对：tenant = %s, limit = %d", reader.tenant, reader.limit)
	}
}

// Covers: 渠道墙未降前登记零行是设计——空册是**答案**，200 + 空数组，并带出继续尝试
// 判断的派生依据。它与「接入渠道未配置」（403）必须在答复上分得开：后者要人去配渠道，
// 前者说的是读取入口已配置、册上确实没有行。
func TestAnEmptyLabelTransactionRegisterIsAnAnswerNotAnError(t *testing.T) {
	response := getLabelTransactions(t, labelTransactionIntake(t),
		&labelTransactionsReaderDouble{}, http.MethodGet)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	body := decodeLabelTransactions(t, response)
	if body.Outcome != "LABEL_TRANSACTIONS_LISTED" || len(body.Rows) != 0 {
		t.Fatalf("空册答案不对：%+v", body)
	}
	if body.ContinuedAttemptBasis != "DERIVED_FROM_DECISION_REGISTER_AND_CURRENT_FINAL" {
		t.Fatalf("继续尝试判断的派生依据没带出：%q", body.ContinuedAttemptBasis)
	}
	// rows 是空数组不是 null：调用方判「册上有没有行」不该先判「有没有这个字段」。
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(fields["rows"]) != "[]" {
		t.Fatalf("rows = %s, want []", fields["rows"])
	}
}

// Covers: ADR-0055 未配置即拒与 ADR-0029 的分格——渠道未配置答 403，读口答不出答 5xx，
// 两者的续办动作不同（一个要人去配渠道，一个要查依赖）；非 GET 拒在方法门上。
func TestLabelTransactionsAnswersSplitByRecoveryAction(t *testing.T) {
	unconfigured := getLabelTransactions(t, shipmenthttp.UnconfiguredIntake{},
		&labelTransactionsReaderDouble{}, http.MethodGet)
	if unconfigured.Code != http.StatusForbidden {
		t.Fatalf("未配置渠道 status = %d, want 403", unconfigured.Code)
	}
	if got := problemCode(t, unconfigured); got != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("未配置渠道 code = %q", got)
	}

	failing := getLabelTransactions(t, labelTransactionIntake(t),
		&labelTransactionsReaderDouble{err: errors.New("库连不上")}, http.MethodGet)
	if failing.Code != http.StatusInternalServerError {
		t.Fatalf("读口失败 status = %d, want 500", failing.Code)
	}
	if got := problemCode(t, failing); got != "NO_ANSWER_FORMED" {
		t.Fatalf("读口失败 code = %q", got)
	}
	assertNoOutcomeField(t, failing)

	posted := getLabelTransactions(t, labelTransactionIntake(t),
		&labelTransactionsReaderDouble{}, http.MethodPost)
	if posted.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", posted.Code)
	}
	if got := posted.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("Allow = %q, want GET；本端点只有读", got)
	}
}
