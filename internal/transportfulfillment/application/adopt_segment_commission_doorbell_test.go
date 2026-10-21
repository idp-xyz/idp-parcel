package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// adoptionDoorbellDouble 记下交出的门铃；err 非空时入不了队。
type adoptionDoorbellDouble struct {
	intents []ports.SegmentCommissionAdoptedIntent
	err     error
}

func (double *adoptionDoorbellDouble) HandOffSegmentCommissionAdoption(_ context.Context, intent ports.SegmentCommissionAdoptedIntent) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

// Covers: AT-TF-300 的采用一半（ADR-0231 决定一）——采用成立时同一笔交一封门铃，带的就是落下的那一份连线；同段原样再交答已有采用、不再交；
// 门铃入不了队答未决，交给装配整笔回滚。
func TestAnAdoptionRingsTheCatchUpDoorbellOnlyWhenItIsEstablished(t *testing.T) {
	fixture := newCommissionAdoptionFixture(t)
	result, err := fixture.adopter.Adopt(context.Background(), adoptCommand("SEG-1", "commission-1", "partner-1"))
	if err != nil || result.Outcome() != application.CommissionAdopted {
		t.Fatalf("采用 = %q %v", result.Outcome(), err)
	}
	adopted, _ := result.Adoption()
	if len(fixture.doorbell.intents) != 1 || fixture.doorbell.intents[0].Record.Key != adopted.Key ||
		fixture.doorbell.intents[0].Record.Adoption.Commission().String() != "commission-1" {
		t.Fatalf("门铃 = %+v", fixture.doorbell.intents)
	}
	again, err := fixture.adopter.Adopt(context.Background(), adoptCommand("SEG-1", "commission-1", "partner-1"))
	if err != nil || again.Outcome() != application.CommissionAdoptionExisting || len(fixture.doorbell.intents) != 1 {
		t.Fatalf("原样再交 = %q %v，门铃 %d 封", again.Outcome(), err, len(fixture.doorbell.intents))
	}

	jammed := newCommissionAdoptionFixture(t)
	jammed.doorbell.err = errors.New("outbox down")
	stuck, err := jammed.adopter.Adopt(context.Background(), adoptCommand("SEG-2", "commission-1", "partner-1"))
	if err != nil || stuck.Outcome() != application.CommissionAdoptionUndecided ||
		stuck.UndecidedReason() != application.CommissionAdoptionDoorbellUnavailable || stuck.ContinuationReference() == "" {
		t.Fatalf("门铃入不了队 = %q/%q %v", stuck.Outcome(), stuck.UndecidedReason(), err)
	}
}
