package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证取消口的生命周期闸门与账户核（票 docs-consistency/code/101）：委托经领域真的越过各个生命周期
// 状态再经生产委托仓储落库，取消编排走生产装配（授权换成已登记的答复，唯一的实例半边）。测试输入是隔离合成，只记 `S`。

const (
	requestStateSubmitted = "submitted"
	requestStateAccepted  = "accepted"
	requestStateWithdrawn = "withdrawn"
	requestStateRejected  = "rejected"
)

// seedAcceptedShipmentRequest 落一份`已接受`委托：包裹取消只对它开放。
func seedAcceptedShipmentRequest(t *testing.T, db *bentopg.DB, identity domain.SourceIdentity) {
	t.Helper()
	seedShipmentRequestInState(t, db, identity, requestStateAccepted)
}

// seedShipmentRequestInState 先落一份`已提交`委托，再经领域转移推到目标状态并经生产仓储保存。假状态挡不住
// 闸门读的是真终态，也证明不了编排认的是领域的生命周期。
func seedShipmentRequestInState(t *testing.T, db *bentopg.DB, identity domain.SourceIdentity, state string) {
	t.Helper()
	seedSubmittedShipmentRequest(t, db, identity)
	if state == requestStateSubmitted {
		return
	}

	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	request, found, err := requests.FindBySourceIdentity(t.Context(), identity)
	if err != nil || !found {
		t.Fatalf("读回委托：err=%v found=%v", err, found)
	}
	decidedAt := time.Date(2026, 8, 21, 10, 30, 0, 0, time.UTC)
	switch state {
	case requestStateAccepted:
		acceptAndSave(t, db, requests, shipmentapp.SubmitShipmentRequestCommand{
			Identity:          identity,
			DeclaredParcelIDs: []domain.DeclaredParcelID{mustValue(t, domain.NewDeclaredParcelID, "SYN-PARCEL-1")},
		}, request)
		return
	case requestStateWithdrawn:
		request, err = request.WithdrawByCustomer(domain.WithdrawalSpec{
			DecisionID: mustValue(t, domain.NewAcceptanceDecisionID, "SYN-WITHDRAWAL-1"),
			Authority:  mustValue(t, domain.NewWithdrawalAuthorityReference, "SYN-WITHDRAW-ROLE-1"),
			Requester:  mustValue(t, domain.NewWithdrawalRequesterReference, "SYN-CUSTOMER-1"),
			Reason:     mustValue(t, domain.NewWithdrawalReasonReference, "SYN-NO-LONGER-REQUIRED"),
			DecidedAt:  decidedAt,
		})
	case requestStateRejected:
		request, err = request.RejectByAuthority(domain.ActiveRejectionSpec{
			DecisionID: mustValue(t, domain.NewAcceptanceDecisionID, "SYN-REJECTION-1"),
			Authority:  mustValue(t, domain.NewRejectionAuthorityReference, "SYN-REJECT-ROLE-1"),
			Decider:    mustValue(t, domain.NewDeciderReference, "SYN-OPERATOR-1"),
			Reason:     mustValue(t, domain.NewRejectionReasonReference, "SYN-NOT-UNDERTAKEN"),
			Evidence:   mustValue(t, domain.NewRejectionEvidenceReference, "SYN-EVIDENCE-1"),
			DecidedAt:  decidedAt,
		})
	default:
		t.Fatalf("未知的委托状态 %q", state)
	}
	if err != nil {
		t.Fatalf("领域转移到 %s：%v", state, err)
	}
	err = db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		saved, saveErr := requests.Save(txCtx, identity, request)
		if saveErr != nil {
			return saveErr
		}
		if saved != ports.ShipmentRequestSaved {
			return fmt.Errorf("save outcome = %v", saved)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("落 %s 委托：%v", state, err)
	}
}

func newCancellationOverState(t *testing.T, state string) (*bentopg.DB, shipmenthttp.CancellationHandler, domain.SourceIdentity, func(string) int) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	cancellation, err := cancellationOrchestrationWith(db, grantingCancellationAuthority{})
	if err != nil {
		t.Fatalf("装配取消编排：%v", err)
	}
	identity := cancellationSeedIdentity(t, "SYN-SUBMIT-KEY-1")
	seedShipmentRequestInState(t, db, identity, state)
	rows := func(query string) int { return countRows(t, pool, query) }
	return db, cancellation, identity, rows
}

const (
	cancellationRows       = `SELECT count(*) FROM parcel_shipment.parcel_cancellation`
	cancellationHandoffs   = `SELECT count(*) FROM ` + migrate.SchemaBento + `.outbox WHERE event_type = 'parcel-shipment.parcel-cancellation.recorded'`
	cancellationEndpointAt = "/shipment-requests/parcel-cancellations"
)

func serveCancellation(
	cancellation shipmenthttp.CancellationHandler,
	command shipmentapp.CancelParcelCommand,
) *httptest.ResponseRecorder {
	endpoint := shipmenthttp.NewCancelParcelEndpoint(fixedCancellationIntake{command: command}, cancellation)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, cancellationEndpointAt, nil))
	return response
}

// Covers: PS CONTEXT「委托」生命周期与 UC-PS-006 结果语义「委托已终结」「委托待决定」经生产装配对真库——只有`已接受`的委托
// 才能取消其包裹。已撤回、已拒绝答 200 `REQUEST_ENDED`，只是已提交的答 200 `REQUEST_AWAITING_DECISION`，授权已登记也不放行；
// 被拦时取消库与发布意图一行都不落。已接受的委托照常取消（201、一行取消、一份发布意图）。
func TestTheWiredCancellationIsGatedByTheRequestLifecycleAgainstARealDatabase(t *testing.T) {
	cases := []struct {
		state   string
		status  int
		outcome string
		rows    int
	}{
		{requestStateWithdrawn, http.StatusOK, "REQUEST_ENDED", 0},
		{requestStateRejected, http.StatusOK, "REQUEST_ENDED", 0},
		{requestStateSubmitted, http.StatusOK, "REQUEST_AWAITING_DECISION", 0},
		{requestStateAccepted, http.StatusCreated, "PARCEL_CANCELLED", 1},
	}
	for _, tc := range cases {
		t.Run(tc.state, func(t *testing.T) {
			_, cancellation, identity, rows := newCancellationOverState(t, tc.state)

			response := serveCancellation(cancellation, cancelParcelCommand(t, identity))
			if response.Code != tc.status {
				t.Fatalf("status = %d body = %s, want %d", response.Code, response.Body, tc.status)
			}
			var body struct {
				Outcome string `json:"outcome"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode %s: %v", response.Body, err)
			}
			if body.Outcome != tc.outcome {
				t.Fatalf("outcome = %q, want %q", body.Outcome, tc.outcome)
			}
			if n := rows(cancellationRows); n != tc.rows {
				t.Fatalf("取消记录 %d 行, want %d", n, tc.rows)
			}
			if n := rows(cancellationHandoffs); n != tc.rows {
				t.Fatalf("发布意图 %d 份, want %d", n, tc.rows)
			}
		})
	}
}

// Covers: `AT-PS-090` 的隔离面落到应用层、经生产装配对真库——委托在库里、编号与成员都对，只是不属命令里的信封账户：
// 与指名一份根本不存在的委托逐字同答（404 `SHIPMENT_REQUEST_NOT_VISIBLE`，同一份响应体），一行取消都不落；别人的已撤回
// 委托也只答这个，不露自己的生命周期状态。账户字段缺席答 200 `REQUEST_NOT_ACCEPTED`，不是跳过账户核放行。
func TestTheWiredCancellationChecksTheTargetBelongsToTheEnvelopeAccountAgainstARealDatabase(t *testing.T) {
	foreign := func(t *testing.T, identity domain.SourceIdentity) shipmentapp.CancelParcelCommand {
		t.Helper()
		command := cancelParcelCommand(t, identity)
		command.CustomerAccount = mustValue(t, domain.NewCustomerAccountID, "SYN-CUSTOMER-2")
		return command
	}

	for _, state := range []string{requestStateAccepted, requestStateWithdrawn} {
		t.Run("another account's "+state+" target", func(t *testing.T) {
			_, cancellation, identity, rows := newCancellationOverState(t, state)

			absent := serveCancellation(cancellation, cancelParcelCommand(t, cancellationSeedIdentity(t, "SYN-SUBMIT-KEY-ABSENT")))
			crossed := serveCancellation(cancellation, foreign(t, identity))
			if crossed.Code != http.StatusNotFound || problemCode(t, crossed) != "SHIPMENT_REQUEST_NOT_VISIBLE" {
				t.Fatalf("status = %d body = %s, want 404 SHIPMENT_REQUEST_NOT_VISIBLE", crossed.Code, crossed.Body)
			}
			if absent.Code != crossed.Code || absent.Body.String() != crossed.Body.String() {
				t.Fatalf("跨账户 %d %s 与查无 %d %s 必须逐字同答", crossed.Code, crossed.Body, absent.Code, absent.Body)
			}
			if n := rows(cancellationRows); n != 0 {
				t.Fatalf("看不见的委托上落了 %d 行取消", n)
			}
		})
	}

	t.Run("a missing account is not accepted", func(t *testing.T) {
		_, cancellation, identity, rows := newCancellationOverState(t, requestStateAccepted)
		command := cancelParcelCommand(t, identity)
		command.CustomerAccount = domain.CustomerAccountID{}

		response := serveCancellation(cancellation, command)
		var body struct {
			Outcome string `json:"outcome"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode %s: %v", response.Body, err)
		}
		if response.Code != http.StatusOK || body.Outcome != "REQUEST_NOT_ACCEPTED" {
			t.Fatalf("status = %d body = %s, want 200 REQUEST_NOT_ACCEPTED", response.Code, response.Body)
		}
		if n := rows(cancellationRows); n != 0 {
			t.Fatalf("账户缺席却落了 %d 行取消", n)
		}
	})
}
