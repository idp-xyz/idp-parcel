package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// Covers: ADR-0202 决定三与 UC-TF-004 步 2「固定供应商协议、履约条件和成本来源快照」——指名的那一版协议核不过时逐格答
// 业务结果（未在册、未生效、已终止、服务方不符），各有各的续办，都不落委托、不交意图；服务方不符另带协议约定的供应商。
// 协议核对口答不出是未决（指名等的是协议），同样零写入。
func TestACommissionWhoseAgreementDoesNotHoldIsAnsweredPerCell(t *testing.T) {
	agreedSupplier, err := domain.NewServiceProviderReference("partner-trunk")
	if err != nil {
		t.Fatal(err)
	}
	cells := map[string]struct {
		check ports.SupplierAgreementCheck
		want  application.CommissionOutcome
		label string
	}{
		"not registered": {ports.SupplierAgreementCheck{Standing: ports.SupplierAgreementNotRegistered}, application.CommissionAgreementNotRegistered, "AGREEMENT_NOT_REGISTERED"},
		"not effective":  {ports.SupplierAgreementCheck{Standing: ports.SupplierAgreementNotEffective}, application.CommissionAgreementNotEffective, "AGREEMENT_NOT_EFFECTIVE"},
		"terminated":     {ports.SupplierAgreementCheck{Standing: ports.SupplierAgreementTerminated}, application.CommissionAgreementTerminated, "AGREEMENT_TERMINATED"},
		"provider mismatch": {
			ports.SupplierAgreementCheck{Standing: ports.SupplierAgreementProviderMismatch, Supplier: agreedSupplier},
			application.CommissionProviderMismatch, "PROVIDER_MISMATCH",
		},
	}
	for name, cell := range cells {
		t.Run(name, func(t *testing.T) {
			fixture := newCommissionFixture(t)
			fixture.agreements.check = cell.check
			result, err := fixture.handler.SubmitCommission(context.Background(), submitCommissionCommand(t))
			if err != nil {
				t.Fatalf("submit: %v", err)
			}
			if result.Outcome() != cell.want || result.Outcome().String() != cell.label {
				t.Fatalf("outcome = %q, want %s", result.Outcome(), cell.label)
			}
			if _, has := result.Commission(); has || fixture.commissions.saves != 0 || len(fixture.handoff.intents) != 0 {
				t.Fatalf("协议核不过却落了委托或交了意图：saves=%d intents=%d", fixture.commissions.saves, len(fixture.handoff.intents))
			}
			supplier, named := result.AgreementSupplier()
			if named != (cell.want == application.CommissionProviderMismatch) {
				t.Fatalf("协议供应商一格只在服务方不符时出现：named=%v", named)
			}
			if named && supplier != agreedSupplier {
				t.Fatalf("服务方不符时说出的协议供应商 = %q", supplier)
			}
		})
	}

	t.Run("an unreadable agreement is undecided and writes nothing", func(t *testing.T) {
		fixture := newCommissionFixture(t)
		fixture.agreements.err = errors.New("party commercial down")
		result, err := fixture.handler.SubmitCommission(context.Background(), submitCommissionCommand(t))
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
		if result.Outcome() != application.CommissionUndecided || result.UndecidedReason() != application.SupplierAgreementUnavailable {
			t.Fatalf("outcome = %q reason = %q, want COMMISSION_UNDECIDED / SUPPLIER_AGREEMENT_UNAVAILABLE", result.Outcome(), result.UndecidedReason())
		}
		if result.ContinuationReference() == "" || fixture.commissions.saves != 0 {
			t.Fatalf("未决要带续办引用且零写入：continuation=%q saves=%d", result.ContinuationReference(), fixture.commissions.saves)
		}
	})

	t.Run("a declaration that cannot draft never asks party commercial", func(t *testing.T) {
		fixture := newCommissionFixture(t)
		command := submitCommissionCommand(t)
		command.AgreementVersion = " "
		result, err := fixture.handler.SubmitCommission(context.Background(), command)
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
		if result.Outcome() != application.CommissionNotAccepted || result.Outcome().String() != "INPUT_NOT_ACCEPTED" {
			t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", result.Outcome())
		}
		if len(fixture.agreements.asked) != 0 {
			t.Fatalf("声明不合法也去问了协议：%d 次", len(fixture.agreements.asked))
		}
	})

	t.Run("a registered check without snapshot or legal entity is a port breach, not an answer", func(t *testing.T) {
		fixture := newCommissionFixture(t)
		fixture.agreements.check = ports.SupplierAgreementCheck{Standing: ports.SupplierAgreementRegistered}
		if _, err := fixture.handler.SubmitCommission(context.Background(), submitCommissionCommand(t)); !errors.Is(err, application.ErrUnexpectedAgreementCheck) {
			t.Fatalf("err = %v, want ErrUnexpectedAgreementCheck", err)
		}
		fixture.agreements.check = ports.SupplierAgreementCheck{Standing: ports.SupplierAgreementStanding(99)}
		if _, err := fixture.handler.SubmitCommission(context.Background(), submitCommissionCommand(t)); !errors.Is(err, application.ErrUnexpectedAgreementCheck) {
			t.Fatalf("err = %v, want ErrUnexpectedAgreementCheck", err)
		}
		if fixture.commissions.saves != 0 {
			t.Fatalf("核对口违约仍落了委托：saves=%d", fixture.commissions.saves)
		}
	})
}

// racingCommissionStore 在 Save 那一刻让另一份同标识的委托先到：Save 答已登记，读回的是先到者。
type racingCommissionStore struct {
	*commissionStoreDouble
	winner ports.TransportCommissionRecord
}

func (store *racingCommissionStore) Save(
	_ context.Context,
	record ports.TransportCommissionRecord,
) (ports.CommissionSaveOutcome, error) {
	store.saves++
	store.records[commissionStoreKey(record.Key)] = store.winner
	return ports.CommissionAlreadyRegistered, nil
}

// Covers: UC-TF-004「同一委托/订舱/外部结果版本只能形成一个采用结果；相同身份内容不同形成冲突」的并发面——首交撞上
// 同标识的先到者时，按先到者的摘要分格：内容相同答已有委托，内容不同答冲突，不把冲突吞成「已有」。
func TestAConcurrentFirstSubmissionLoserIsJudgedByContent(t *testing.T) {
	for name, testCase := range map[string]struct {
		winnerVersion string
		want          application.CommissionOutcome
	}{
		"same content":      {"v1", application.CommissionExistingResult},
		"different content": {"v2", application.CommissionConflict},
	} {
		t.Run(name, func(t *testing.T) {
			// 先用一份独立的编排把先到者的样子做出来，再让它在本次 Save 那一刻占位。
			seed := newCommissionFixture(t)
			winnerCommand := submitCommissionCommand(t)
			winnerCommand.AgreementVersion = testCase.winnerVersion
			if _, err := seed.handler.SubmitCommission(context.Background(), winnerCommand); err != nil {
				t.Fatalf("seed: %v", err)
			}
			var winner ports.TransportCommissionRecord
			for _, record := range seed.commissions.records {
				winner = record
			}

			fixture := newCommissionFixture(t)
			racing := &racingCommissionStore{commissionStoreDouble: newCommissionStore(), winner: winner}
			handler := application.NewCommissionTransportHandler(application.CommissionTransportDeps{
				Commissions: racing,
				Bookings:    fixture.bookings,
				Answers:     fixture.answers,
				Agreements:  fixture.agreements,
				Downstream:  fixture.handoff,
				Clock:       commissionClock{at: commissionNowAt},
			})
			result, err := handler.SubmitCommission(context.Background(), submitCommissionCommand(t))
			if err != nil {
				t.Fatalf("submit: %v", err)
			}
			if result.Outcome() != testCase.want {
				t.Fatalf("outcome = %q, want %q", result.Outcome(), testCase.want)
			}
		})
	}
}
