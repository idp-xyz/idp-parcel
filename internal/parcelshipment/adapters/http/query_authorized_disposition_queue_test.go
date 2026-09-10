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

type dispositionQueueReaderDouble struct {
	rows    []ports.AuthorizedDispositionQueueRecord
	listErr error
	scope   domain.AuthorizedQueryScope
	limit   int
}

func (double *dispositionQueueReaderDouble) ListAwaitingAuthorizedDisposition(
	_ context.Context,
	scope domain.AuthorizedQueryScope,
	limit int,
) ([]ports.AuthorizedDispositionQueueRecord, error) {
	double.scope, double.limit = scope, limit
	if double.listErr != nil {
		return nil, double.listErr
	}
	return double.rows, nil
}

func getDispositionQueue(
	t *testing.T,
	intake shipmenthttp.ShipmentRequestViewsIntake,
	reader shipmenthttp.AuthorizedDispositionQueueReader,
	method, target string,
) *httptest.ResponseRecorder {
	t.Helper()
	endpoint := shipmenthttp.NewQueryAuthorizedDispositionQueueEndpoint(intake, reader)
	request := httptest.NewRequest(method, target, nil)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

func dispositionQueueRecordFixture(t *testing.T) ports.AuthorizedDispositionQueueRecord {
	t.Helper()
	return ports.AuthorizedDispositionQueueRecord{
		ShipmentRequestSummaryRecord: viewSummaryRecord(t, "REQ-1"),
		HasAttempt:                   true,
		LastAttemptReason:            "COMMERCIAL_BASIS_UNAVAILABLE",
		LastAttemptContinuation:      "CONT-1",
		LastAttemptedAt:              viewSubmittedAt.Add(time.Hour),
		ControlResultID:              mustValue(t, domain.NewFinancialControlResultID, "CTRL-1"),
		RestrictedItems: []ports.RestrictedControlItemRecord{{
			Kind:               domain.CreditCheckControlItem,
			Order:              2,
			Basis:              mustValue(t, domain.NewControlBasisReference, "AVAILABLE_CREDIT_INSUFFICIENT"),
			FailureDisposition: domain.AuthorizedDispositionOnControlFailure,
			Responsibility:     mustValue(t, domain.NewControlResponsibilityReference, "CONTRACT-CLAUSE-7"),
		}},
	}
}

// Covers: ADR-0132 决定二的队列读面——逐行透出概要、最近处理记录、控制结果标识与受限项（种类、顺序、受限原因、
// 正文登记的失败处置与责任引用）；作用域与页大小原样交给读口；空队列是 `[]` 不是 null。
func TestDispositionQueueListsRestrictedItemsWithTheirAdoptedReferences(t *testing.T) {
	intake := &viewsIntakeDouble{scope: viewsScope(t)}
	reader := &dispositionQueueReaderDouble{rows: []ports.AuthorizedDispositionQueueRecord{dispositionQueueRecordFixture(t)}}
	response := getDispositionQueue(t, intake, reader, http.MethodGet, "/authorized-disposition-queue")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if reader.scope.TenantID().String() != "TENANT-1" || reader.limit < 1 {
		t.Fatalf("读口拿到的 scope/limit = %v/%d，want the intake's", reader.scope, reader.limit)
	}
	var body struct {
		Outcome string `json:"outcome"`
		Entries []struct {
			ShipmentRequestID       string `json:"shipmentRequestId"`
			State                   string `json:"state"`
			LastAttemptReason       string `json:"lastAttemptReason"`
			LastAttemptContinuation string `json:"lastAttemptContinuation"`
			ControlResultID         string `json:"controlResultId"`
			RestrictedItems         []struct {
				Kind               string `json:"kind"`
				Order              uint32 `json:"order"`
				Basis              string `json:"basis"`
				FailureDisposition string `json:"failureDisposition"`
				Responsibility     string `json:"responsibility"`
			} `json:"restrictedItems"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Outcome != "DISPOSITION_QUEUE_LISTED" || len(body.Entries) != 1 {
		t.Fatalf("body = %+v", body)
	}
	entry := body.Entries[0]
	if entry.ShipmentRequestID != "REQ-1" || entry.State != "SUBMITTED" ||
		entry.LastAttemptReason != "COMMERCIAL_BASIS_UNAVAILABLE" || entry.LastAttemptContinuation != "CONT-1" ||
		entry.ControlResultID != "CTRL-1" {
		t.Fatalf("entry 变形：%+v", entry)
	}
	if len(entry.RestrictedItems) != 1 ||
		entry.RestrictedItems[0].Kind != "CREDIT_CHECK" || entry.RestrictedItems[0].Order != 2 ||
		entry.RestrictedItems[0].Basis != "AVAILABLE_CREDIT_INSUFFICIENT" ||
		entry.RestrictedItems[0].FailureDisposition != "AUTHORIZED_DISPOSITION" ||
		entry.RestrictedItems[0].Responsibility != "CONTRACT-CLAUSE-7" {
		t.Fatalf("受限项变形：%+v", entry.RestrictedItems)
	}

	empty := getDispositionQueue(t, &viewsIntakeDouble{scope: viewsScope(t)},
		&dispositionQueueReaderDouble{}, http.MethodGet, "/authorized-disposition-queue")
	var emptyBody map[string]json.RawMessage
	if err := json.Unmarshal(empty.Body.Bytes(), &emptyBody); err != nil {
		t.Fatalf("decode empty: %v", err)
	}
	if string(emptyBody["entries"]) != "[]" {
		t.Fatalf(`entries = %s, want []（空队列不是 null）`, emptyBody["entries"])
	}
}

// Covers: 方法门、未配置渠道与读口答不出三格各落各的（ADR-0022 / ADR-0055）：405 带 Allow；403 不读内容；
// 读不回是 5xx NO_ANSWER_FORMED，不是空队列。
func TestDispositionQueueRefusalsLandOnTheirOwnCodes(t *testing.T) {
	notAllowed := getDispositionQueue(t, &viewsIntakeDouble{scope: viewsScope(t)},
		&dispositionQueueReaderDouble{}, http.MethodPost, "/authorized-disposition-queue")
	if notAllowed.Code != http.StatusMethodNotAllowed || notAllowed.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status = %d allow = %q, want 405 GET", notAllowed.Code, notAllowed.Header().Get("Allow"))
	}

	unconfigured := getDispositionQueue(t, shipmenthttp.UnconfiguredIntake{},
		&dispositionQueueReaderDouble{}, http.MethodGet, "/authorized-disposition-queue")
	if unconfigured.Code != http.StatusForbidden || problemCode(t, unconfigured) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("status = %d code = %q, want 403 ACCESS_CHANNEL_NOT_CONFIGURED", unconfigured.Code, problemCode(t, unconfigured))
	}

	unavailable := getDispositionQueue(t, &viewsIntakeDouble{scope: viewsScope(t)},
		&dispositionQueueReaderDouble{listErr: errors.New("db down")}, http.MethodGet, "/authorized-disposition-queue")
	if unavailable.Code != http.StatusInternalServerError || problemCode(t, unavailable) != "NO_ANSWER_FORMED" {
		t.Fatalf("status = %d code = %q, want 500 NO_ANSWER_FORMED——读不回不是空队列", unavailable.Code, problemCode(t, unavailable))
	}
}
