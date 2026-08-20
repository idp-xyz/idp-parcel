package visibilityhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var claimHTTPSubmittedAt = time.Date(2026, 8, 13, 11, 0, 0, 0, time.UTC)

type claimIntakeDouble struct {
	command application.ReceiveClaimCommand
	err     error
}

func (double claimIntakeDouble) IntakeClaim(
	_ context.Context,
	_ *http.Request,
) (application.ReceiveClaimCommand, error) {
	if double.err != nil {
		return application.ReceiveClaimCommand{}, double.err
	}
	return double.command, nil
}

// httpClaimStore 是给真编排充当索赔库的替身——适配器测试消费真 HandleClaimHandler，
// 与 PS 参照同款：结果类型不可外部拼装，替身只替端口。
type httpClaimStoreKey struct {
	tenant domain.TenantID
	batch  domain.ClaimBatchReference
	item   domain.ClaimItemID
}

type httpClaimStore struct {
	claims  map[httpClaimStoreKey]*domain.ClaimItem
	findErr error
}

func newHTTPClaimStore() *httpClaimStore {
	return &httpClaimStore{claims: map[httpClaimStoreKey]*domain.ClaimItem{}}
}

func (store *httpClaimStore) FindByBatchItem(
	_ context.Context,
	tenant domain.TenantID,
	batch domain.ClaimBatchReference,
	item domain.ClaimItemID,
) (*domain.ClaimItem, bool, error) {
	if store.findErr != nil {
		return nil, false, store.findErr
	}
	claim, found := store.claims[httpClaimStoreKey{tenant: tenant, batch: batch, item: item}]
	return claim, found, nil
}

func (store *httpClaimStore) Save(
	_ context.Context,
	tenant domain.TenantID,
	claim *domain.ClaimItem,
) (ports.ClaimSaveOutcome, error) {
	store.claims[httpClaimStoreKey{tenant: tenant, batch: claim.Batch(), item: claim.ID()}] = claim
	return ports.ClaimSaved, nil
}

type inertEligibility struct{}

func (inertEligibility) ScreenClaim(
	_ context.Context,
	_ ports.EligibilityQuery,
) (ports.EligibilityAnswer, bool, error) {
	return ports.EligibilityAnswer{}, false, nil
}

type inertRecoveries struct{}

func (inertRecoveries) FindByID(_ context.Context, _ domain.TenantID, _ domain.RecoveryMatterID) (domain.RecoveryMatter, bool, error) {
	return domain.RecoveryMatter{}, false, nil
}

func (inertRecoveries) FindCurrent(
	_ context.Context,
	_ domain.TenantID,
	_ domain.CaseID,
	_ domain.CounterpartyReference,
	_ domain.RequestScopeReference,
) (domain.RecoveryMatter, bool, error) {
	return domain.RecoveryMatter{}, false, nil
}

func (inertRecoveries) Save(_ context.Context, _ domain.TenantID, _ domain.RecoveryMatter) (ports.RecoverySaveOutcome, error) {
	return ports.RecoverySaved, nil
}

func (inertRecoveries) CountActions(
	_ context.Context,
	_ domain.TenantID,
	_ domain.RecoveryMatterID,
	_ domain.RecoveryActionKind,
) (int, error) {
	return 0, nil
}

func (inertRecoveries) AppendAction(_ context.Context, _ domain.TenantID, _ domain.RecoveryAction) error {
	return nil
}

type inertRecoveryIdentities struct{}

func (inertRecoveryIdentities) NextRecoveryMatterID(_ context.Context) (domain.RecoveryMatterID, error) {
	return domain.NewRecoveryMatterID("unused")
}

type inertSettlement struct{}

func (inertSettlement) HandOffLiability(_ context.Context, _ ports.LiabilityHandoffIntent) error {
	return nil
}

type httpClock struct{ at time.Time }

func (clock httpClock) Now() time.Time { return clock.at }

type erroringReceiver struct{ err error }

func (double erroringReceiver) ReceiveClaim(
	_ context.Context,
	_ application.ReceiveClaimCommand,
) (application.HandleClaimResult, error) {
	return application.HandleClaimResult{}, double.err
}

func newClaimReceiver(store *httpClaimStore) *application.HandleClaimHandler {
	return application.NewHandleClaimHandler(application.HandleClaimDeps{
		Claims:      store,
		Eligibility: inertEligibility{},
		Recoveries:  inertRecoveries{},
		Identities:  inertRecoveryIdentities{},
		Settlement:  inertSettlement{},
		Clock:       httpClock{at: claimHTTPSubmittedAt.Add(time.Minute)},
	})
}

func claimCommandOf(t *testing.T) application.ReceiveClaimCommand {
	t.Helper()
	return application.ReceiveClaimCommand{
		TenantID:    value(t, domain.NewTenantID, "tenant-9"),
		Batch:       value(t, domain.NewClaimBatchReference, "claim-batch-9"),
		Item:        value(t, domain.NewClaimItemID, "item-9"),
		Customer:    value(t, domain.NewCustomerAccountReference, "customer-1"),
		Contract:    value(t, domain.NewContractScopeReference, "contract-scope/v1"),
		Target:      value(t, domain.NewRequestScopeReference, "parcel-1/loss"),
		Kind:        value(t, domain.NewClaimKindReference, "LOSS"),
		SubmittedAt: claimHTTPSubmittedAt,
	}
}

func serveClaim(t *testing.T, intake visibilityhttp.ClaimIntake, receiver visibilityhttp.ClaimReceiver, method string) *httptest.ResponseRecorder {
	t.Helper()
	endpoint := visibilityhttp.NewReceiveClaimEndpoint(intake, receiver)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(method, "/claims", nil))
	return recorder
}

// Covers: UC-VE-007「先保留原始提交事实」经 HTTP 面——受理成立 201 带回执（批次、项、
// 提交时间），重放同一提交 200 返已有；**响应体结构上没有资格与结论字段**（三判分步，
// 受理端点不暴露审核进度）。
func TestAClaimSubmissionIsReceivedOnceWithItsReceipt(t *testing.T) {
	store := newHTTPClaimStore()
	receiver := newClaimReceiver(store)
	intake := claimIntakeDouble{command: claimCommandOf(t)}

	first := serveClaim(t, intake, receiver, http.MethodPost)
	if first.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", first.Code)
	}
	var body struct {
		Outcome string `json:"outcome"`
		Claim   struct {
			Batch       string `json:"batch"`
			Item        string `json:"item"`
			SubmittedAt string `json:"submittedAt"`
		} `json:"claim"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Outcome != "CLAIM_RECEIVED" || body.Claim.Batch != "claim-batch-9" || body.Claim.Item != "item-9" {
		t.Fatalf("body = %+v, want a CLAIM_RECEIVED receipt", body)
	}
	if body.Claim.SubmittedAt == "" {
		t.Fatal("the receipt lost the original submission time")
	}
	lowered := strings.ToLower(first.Body.String())
	if strings.Contains(lowered, "screen") || strings.Contains(lowered, "conclusion") || strings.Contains(lowered, "liability") {
		t.Fatalf("body = %s; 受理回执结构上不得携带资格或结论字段", first.Body.String())
	}

	replay := serveClaim(t, intake, receiver, http.MethodPost)
	if replay.Code != http.StatusOK {
		t.Fatalf("replay status = %d, want 200", replay.Code)
	}
	var replayed struct {
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(replay.Body.Bytes(), &replayed); err != nil {
		t.Fatalf("decode replay: %v", err)
	}
	if replayed.Outcome != "CLAIM_EXISTING_RESULT" {
		t.Fatalf("outcome = %q, want CLAIM_EXISTING_RESULT", replayed.Outcome)
	}
}

// Covers: ADR-0022 的未决半边——库读不回时编排答 UNDECIDED（带稳定原因），那是已形成
// 的业务答案：200 带 outcome，不是 5xx，也不伪装成未受理。
func TestAnUndecidedReceiptionIsAFormedAnswer(t *testing.T) {
	store := newHTTPClaimStore()
	store.findErr = errors.New("claim store down")
	receiver := newClaimReceiver(store)

	recorder := serveClaim(t, claimIntakeDouble{command: claimCommandOf(t)}, receiver, http.MethodPost)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200（UNDECIDED 是已形成的答案）", recorder.Code)
	}
	var body struct {
		Outcome         string `json:"outcome"`
		UndecidedReason string `json:"undecidedReason"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Outcome != "UNDECIDED" || body.UndecidedReason != "CLAIM_STORE_UNAVAILABLE" {
		t.Fatalf("body = %+v, want UNDECIDED/CLAIM_STORE_UNAVAILABLE", body)
	}
}

// Covers: 传输纪律同首票——翻译不出命令 400、编排 Go 错误 500 无 outcome、方法不对
// 405；都不带业务答案也不带自由文本。
func TestClaimTransportGridsStayClosed(t *testing.T) {
	malformed := serveClaim(t, claimIntakeDouble{err: visibilityhttp.ErrMalformedClaim}, erroringReceiver{}, http.MethodPost)
	if malformed.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", malformed.Code)
	}

	broken := serveClaim(t, claimIntakeDouble{command: claimCommandOf(t)}, erroringReceiver{err: errors.New("boom")}, http.MethodPost)
	if broken.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", broken.Code)
	}
	var body struct {
		Outcome string `json:"outcome"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(broken.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Outcome != "" || body.Error.Code != "NO_ANSWER_FORMED" {
		t.Fatalf("body = %+v; 编排错误不得伪装成业务答案", body)
	}

	wrongMethod := serveClaim(t, claimIntakeDouble{command: claimCommandOf(t)}, erroringReceiver{}, http.MethodGet)
	if wrongMethod.Code != http.StatusMethodNotAllowed || wrongMethod.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("status = %d allow = %q, want 405/POST", wrongMethod.Code, wrongMethod.Header().Get("Allow"))
	}
}
