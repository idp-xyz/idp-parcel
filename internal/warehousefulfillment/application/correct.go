package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// CorrectConfirmationCommand 更正一版出库确认。改时点当场成立；判不成立只留下待回补。
type CorrectConfirmationCommand struct {
	Tenant     domain.TenantID
	Operator   domain.Registrant
	FactID     domain.PickFactID
	OccurredAt time.Time
	Invalidate bool
}

// ApproveReplenishmentCommand 批准一次待回补。批准职责规则没登记答未配置。
type ApproveReplenishmentCommand struct {
	Tenant   domain.TenantID
	Operator domain.Registrant
	FactID   domain.PickFactID
}

func (handler *WaveHandler) Correct(ctx context.Context, command CorrectConfirmationCommand) (WaveResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return WaveResult{}, fmt.Errorf("correct outbound confirmation: %w", err)
	}
	if !configured {
		return WaveResult{outcome: WaveNotConfigured}, nil
	}
	if command.FactID.String() == "" || command.Operator.String() == "" || command.Invalidate == !command.OccurredAt.IsZero() {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	confirmation, found, err := handler.waves.LoadConfirmation(ctx, command.Tenant, command.FactID.String())
	if err != nil {
		return WaveResult{}, fmt.Errorf("correct outbound confirmation: %w", err)
	}
	if !found {
		return WaveResult{outcome: WaveNotFound}, nil
	}
	kind := "TIME"
	status := "TIME_CORRECTED"
	outcome := WaveCorrected
	occurred := command.OccurredAt.UTC()
	if command.Invalidate {
		kind = "INVALIDATE"
		status = "PENDING"
		outcome = WavePending
		occurred = time.Time{}
	}
	current, found, err := handler.waves.LoadCorrection(ctx, command.Tenant, command.FactID.String())
	if err != nil {
		return WaveResult{}, fmt.Errorf("correct outbound confirmation: %w", err)
	}
	if found {
		if current.Kind == kind && current.Operator == command.Operator.String() && current.OccurredAt.Equal(occurred) {
			return WaveResult{outcome: WaveExisting}, nil
		}
		return WaveResult{outcome: WaveConflict}, nil
	}
	correction := ports.ConfirmationCorrection{
		FactID: command.FactID.String(), Kind: kind, OccurredAt: occurred, Operator: command.Operator.String(), Status: status,
	}
	if err := handler.waves.SaveCorrection(ctx, command.Tenant, correction); err != nil {
		return WaveResult{}, fmt.Errorf("correct outbound confirmation: %w", err)
	}
	if err := handler.handOffCorrection(ctx, command.Tenant, confirmation, occurred); err != nil {
		return WaveResult{}, err
	}
	return WaveResult{outcome: outcome}, nil
}

func (handler *WaveHandler) ApproveReplenishment(ctx context.Context, command ApproveReplenishmentCommand) (WaveResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return WaveResult{}, fmt.Errorf("approve replenishment: %w", err)
	}
	if !configured || handler.duties == nil {
		return WaveResult{outcome: WaveNotConfigured}, nil
	}
	if command.FactID.String() == "" || command.Operator.String() == "" {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	duty, found, err := handler.duties.LoadDuty(ctx, command.Tenant)
	if err != nil {
		return WaveResult{}, fmt.Errorf("approve replenishment: %w", err)
	}
	if !found {
		return WaveResult{outcome: WaveNotConfigured}, nil
	}
	correction, found, err := handler.waves.LoadCorrection(ctx, command.Tenant, command.FactID.String())
	if err != nil {
		return WaveResult{}, fmt.Errorf("approve replenishment: %w", err)
	}
	if !found || correction.Kind != "INVALIDATE" {
		return WaveResult{outcome: WaveNotFound}, nil
	}
	if correction.Status == "POSTED" {
		return WaveResult{outcome: WaveExisting}, nil
	}
	if correction.Status != "PENDING" {
		return WaveResult{outcome: WaveStatusMismatch}, nil
	}
	if duty.Distinct && correction.Operator == command.Operator.String() {
		return WaveResult{outcome: WaveDutyBlocked}, nil
	}
	reference, err := domain.NewSourceFactReference(command.FactID.String())
	if err != nil {
		return WaveResult{}, fmt.Errorf("approve replenishment: %w", err)
	}
	movement, found, err := handler.ledger.LoadMovementBySource(ctx, command.Tenant, domain.StockReasonOutboundConfirmation, reference)
	if err != nil {
		return WaveResult{}, fmt.Errorf("approve replenishment: %w", err)
	}
	if !found {
		return WaveResult{outcome: WaveStatusMismatch}, nil
	}
	lines := make([]domain.StockLineRequest, 0, len(movement.Lines()))
	for _, line := range movement.Lines() {
		if line.Direction() != domain.StockOut {
			continue
		}
		restored, err := stockLine(domain.StockIn, line.Quantity(), line.Cell())
		if err != nil {
			return WaveResult{}, fmt.Errorf("approve replenishment: %w", err)
		}
		lines = append(lines, restored)
	}
	if len(lines) == 0 {
		return WaveResult{outcome: WaveStatusMismatch}, nil
	}
	replenishRef, err := domain.NewSourceFactReference(command.FactID.String() + ":replenish")
	if err != nil {
		return WaveResult{}, fmt.Errorf("approve replenishment: %w", err)
	}
	posted, err := handler.poster.Post(ctx, PostStockMovementCommand{
		Tenant: command.Tenant,
		Request: domain.NewStockMovementRequest(
			movement.Owner(), movement.Warehouse(), domain.StockReasonAdjustmentIncrease, replenishRef, handler.clock.Now(), lines,
		),
	})
	if err != nil {
		return WaveResult{}, fmt.Errorf("approve replenishment: %w", err)
	}
	if posted.Outcome() != PostStockPosted && posted.Outcome() != PostStockAlreadyPosted {
		return WaveResult{outcome: WaveStatusMismatch}, nil
	}
	if err := handler.waves.PostReplenishment(ctx, command.Tenant, command.FactID.String(), command.Operator, duty.Revision, handler.clock.Now()); err != nil {
		return WaveResult{}, fmt.Errorf("approve replenishment: %w", err)
	}
	return WaveResult{outcome: WavePosted}, nil
}

func (handler *WaveHandler) handOffCorrection(ctx context.Context, tenant domain.TenantID, confirmation ports.OutboundConfirmation, occurred time.Time) error {
	if handler.confirmation == nil {
		return nil
	}
	parcels, err := handler.waves.ParcelsOf(ctx, tenant, confirmation.Request)
	if err != nil {
		return fmt.Errorf("correct outbound confirmation: %w", err)
	}
	var parcelID string
	for _, parcel := range parcels {
		if parcel.Sequence == confirmation.Sequence {
			parcelID = parcel.DeclaredParcelID
		}
	}
	if parcelID == "" {
		return nil
	}
	at := confirmation.OccurredAt
	if !occurred.IsZero() {
		at = occurred
	}
	if err := handler.confirmation.HandOffOutboundConfirmation(ctx, tenant.String(), confirmation.FactID, parcelID, at, "2", "1"); err != nil {
		return fmt.Errorf("correct outbound confirmation: %w", err)
	}
	return nil
}
