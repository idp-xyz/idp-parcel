package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 出库确认的两种作业事实。自提单独一种，其余发运方式走出库确认。
const (
	ConfirmKindOutbound = "OUTBOUND_CONFIRMATION"
	ConfirmKindPickup   = "PICKUP_HANDOVER"
)

// 放行条件缺的格。面单本体没有存放，在线的本产品网络因此答未配置。
const (
	gapAccepted     = "ACCEPTED"
	gapNotCancelled = "NOT_CANCELLED"
	gapCorrection   = "CORRECTION_ADOPTED"
	gapLabelBody    = "LABEL_BODY"
	gapLabelCount   = "LABEL_COUNT"
	gapInterval     = "INTERVAL"
	gapCredential   = "CREDENTIAL"
)

// ConfirmCommand 是一次出库确认或自提交付。在线核放行条件；离线照记并标出缺的格。
type ConfirmCommand struct {
	Tenant           domain.TenantID
	FactID           domain.PickFactID
	Request          domain.OutboundRequestID
	Sequence         int
	Warehouse        domain.WarehouseCode
	Kind             string
	Offline          bool
	OccurredAt       time.Time
	ValidFrom        time.Time
	ValidUntil       time.Time
	ExternalNumber   string
	PickupParty      string
	PickupCredential string
}

// RegisterOwnerLabelCommand 登记一个包裹的货主面单。面单数要和包裹数对上才出库确认。
type RegisterOwnerLabelCommand struct {
	Tenant         domain.TenantID
	Request        domain.OutboundRequestID
	Sequence       int
	ExternalNumber string
}

type outboundConfirmationHandoff interface {
	HandOffOutboundConfirmation(ctx context.Context, tenant, factID, parcelID string, occurredAt time.Time, version, supersedes string) error
}

func (handler *WaveHandler) RegisterOwnerLabel(ctx context.Context, command RegisterOwnerLabelCommand) (WaveResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return WaveResult{}, fmt.Errorf("register owner label: %w", err)
	}
	if !configured {
		return WaveResult{outcome: WaveNotConfigured}, nil
	}
	number := strings.TrimSpace(command.ExternalNumber)
	if command.Request.String() == "" || command.Sequence < 1 || number == "" {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	current, found, err := handler.waves.LoadOwnerLabel(ctx, command.Tenant, command.Request, command.Sequence)
	if err != nil {
		return WaveResult{}, fmt.Errorf("register owner label: %w", err)
	}
	if found {
		if current == number {
			return WaveResult{outcome: WaveExisting}, nil
		}
		return WaveResult{outcome: WaveConflict}, nil
	}
	if err := handler.waves.SaveOwnerLabel(ctx, command.Tenant, command.Request, command.Sequence, number); err != nil {
		return WaveResult{}, fmt.Errorf("register owner label: %w", err)
	}
	return WaveResult{outcome: WaveStrategyRegistered}, nil
}

func (handler *WaveHandler) Confirm(ctx context.Context, command ConfirmCommand) (WaveResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return WaveResult{}, fmt.Errorf("confirm outbound: %w", err)
	}
	if !configured {
		return WaveResult{outcome: WaveNotConfigured}, nil
	}
	if command.FactID.String() == "" || command.Request.String() == "" || command.Sequence < 1 || command.OccurredAt.IsZero() ||
		(command.Kind != ConfirmKindOutbound && command.Kind != ConfirmKindPickup) {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	stored, found, err := handler.waves.LoadConfirmation(ctx, command.Tenant, command.FactID.String())
	if err != nil {
		return WaveResult{}, fmt.Errorf("confirm outbound: %w", err)
	}
	request, requestFound, err := handler.requests.LoadByID(ctx, command.Tenant, command.Request)
	if err != nil {
		return WaveResult{}, fmt.Errorf("confirm outbound: %w", err)
	}
	if !requestFound {
		return WaveResult{outcome: WaveNotFound}, nil
	}
	if command.Warehouse.String() != "" && command.Warehouse != request.Warehouse() {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	if command.Kind == ConfirmKindPickup && request.Dispatch() != domain.DispatchPickup {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	if command.Kind == ConfirmKindOutbound && request.Dispatch() == domain.DispatchPickup {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	parcels, err := handler.waves.ParcelsOf(ctx, command.Tenant, command.Request)
	if err != nil {
		return WaveResult{}, fmt.Errorf("confirm outbound: %w", err)
	}
	var parcel ports.PackParcel
	seen := false
	for _, item := range parcels {
		if item.Sequence == command.Sequence {
			parcel = item
			seen = true
		}
	}
	if !seen {
		return WaveResult{outcome: WaveStatusMismatch}, nil
	}
	// 自提凭证写在这一笔交付上：写了这一格就齐。放行区间没带就不核，带了才要求发生时刻落在里面。
	missing, err := handler.releaseGaps(ctx, command, request, parcel, parcels)
	if err != nil {
		return WaveResult{}, fmt.Errorf("confirm outbound: %w", err)
	}
	missingText := strings.Join(missing, ",")
	if found {
		if confirmationMatches(stored, command, missingText) {
			return WaveResult{outcome: WaveExisting}, nil
		}
		return WaveResult{outcome: WaveConflict}, nil
	}
	if request.Status() != domain.OutboundPacked {
		return WaveResult{outcome: WaveStatusMismatch}, nil
	}
	if parcelConfirmed, err := handler.waves.ParcelConfirmed(ctx, command.Tenant, command.Request, command.Sequence); err != nil {
		return WaveResult{}, fmt.Errorf("confirm outbound: %w", err)
	} else if parcelConfirmed {
		return WaveResult{outcome: WaveStatusMismatch}, nil
	}
	if !command.Offline {
		switch {
		case onlyGap(missing, gapLabelBody) || onlyGap(missing, gapCredential):
			return WaveResult{outcome: WaveNotConfigured}, nil
		case len(missing) > 0:
			return WaveResult{outcome: WaveWithheld}, nil
		}
	}
	if err := handler.consumeStaged(ctx, request, parcel, command); err != nil {
		return WaveResult{}, err
	}
	state := "CONFIRMED"
	outcome := WaveConfirmed
	if len(missing) > 0 {
		state = "RECORDED"
		outcome = WaveRecorded
	}
	if err := handler.waves.SaveConfirmation(ctx, command.Tenant, ports.OutboundConfirmation{
		FactID: command.FactID.String(), Request: command.Request, Sequence: command.Sequence, Kind: command.Kind,
		Offline: command.Offline, Missing: missingText, ExternalNumber: strings.TrimSpace(command.ExternalNumber),
		PickupParty: strings.TrimSpace(command.PickupParty), PickupCredential: strings.TrimSpace(command.PickupCredential),
		OccurredAt: command.OccurredAt.UTC(), State: state,
	}); err != nil {
		return WaveResult{}, fmt.Errorf("confirm outbound: %w", err)
	}
	if handler.confirmation != nil && request.Dispatch() == domain.DispatchProductNetwork && parcel.DeclaredParcelID != "" {
		if err := handler.confirmation.HandOffOutboundConfirmation(ctx, command.Tenant.String(), command.FactID.String(), parcel.DeclaredParcelID, command.OccurredAt.UTC(), "1", ""); err != nil {
			return WaveResult{}, fmt.Errorf("confirm outbound: %w", err)
		}
	}
	if err := handler.formOutboundCharge(ctx, command, request, parcel); err != nil {
		return WaveResult{}, err
	}
	return WaveResult{outcome: outcome}, nil
}

func confirmationMatches(stored ports.OutboundConfirmation, command ConfirmCommand, missing string) bool {
	return stored.Request == command.Request && stored.Sequence == command.Sequence && stored.Kind == command.Kind &&
		stored.Offline == command.Offline && stored.Missing == missing &&
		stored.ExternalNumber == strings.TrimSpace(command.ExternalNumber) &&
		stored.PickupParty == strings.TrimSpace(command.PickupParty) &&
		stored.PickupCredential == strings.TrimSpace(command.PickupCredential)
}

func onlyGap(missing []string, gap string) bool {
	return len(missing) == 1 && missing[0] == gap
}

func (handler *WaveHandler) releaseGaps(ctx context.Context, command ConfirmCommand, request domain.OutboundRequest, parcel ports.PackParcel, parcels []ports.PackParcel) ([]string, error) {
	var missing []string
	occurred := command.OccurredAt.UTC()
	windowGiven := !command.ValidFrom.IsZero() || !command.ValidUntil.IsZero()
	if windowGiven && (command.ValidFrom.IsZero() || command.ValidUntil.IsZero() || occurred.Before(command.ValidFrom.UTC()) || !occurred.Before(command.ValidUntil.UTC())) {
		missing = append(missing, gapInterval)
	}
	switch request.Dispatch() {
	case domain.DispatchProductNetwork:
		switch parcel.CommissionState {
		case "CANCELLED":
			missing = append(missing, gapNotCancelled)
		case "ACCEPTED":
		default:
			missing = append(missing, gapAccepted)
		}
		if parcel.CorrectionState == "AWAITING_REVIEW" {
			missing = append(missing, gapCorrection)
		}
		missing = append(missing, gapLabelBody)
	case domain.DispatchOwnerLabel:
		labels, err := handler.waves.OwnerLabelsOf(ctx, command.Tenant, command.Request)
		if err != nil {
			return nil, err
		}
		if len(labels) != len(parcels) || labels[command.Sequence] == "" {
			missing = append(missing, gapLabelCount)
		}
	case domain.DispatchPickup:
		if strings.TrimSpace(command.PickupCredential) == "" {
			missing = append(missing, gapCredential)
		}
	}
	return missing, nil
}

func (handler *WaveHandler) consumeStaged(ctx context.Context, request domain.OutboundRequest, parcel ports.PackParcel, command ConfirmCommand) error {
	if err := handler.ledger.SerializePosting(ctx, command.Tenant, request.Owner(), request.Warehouse()); err != nil {
		return fmt.Errorf("confirm outbound: %w", err)
	}
	tasks, err := handler.waves.TasksOf(ctx, command.Tenant, command.Request)
	if err != nil {
		return fmt.Errorf("confirm outbound: %w", err)
	}
	staging := map[string]struct{}{}
	for _, task := range countedTasks(tasks) {
		if task.Staging.String() != "" && task.Goods == parcel.Goods {
			staging[task.Staging.String()] = struct{}{}
		}
	}
	rows, err := handler.ledger.ListOnHandCells(ctx, command.Tenant, request.Owner(), request.Warehouse())
	if err != nil {
		return fmt.Errorf("confirm outbound: %w", err)
	}
	remaining := parcel.Quantity
	var lines []domain.StockLineRequest
	for _, row := range rows {
		if remaining == 0 {
			break
		}
		if _, staged := staging[row.LocationCode]; !staged || row.GoodsCode != parcel.Goods.String() || row.Quantity <= 0 {
			continue
		}
		cell, err := cellFromBalance(row)
		if err != nil {
			return fmt.Errorf("confirm outbound: %w", err)
		}
		take := remaining
		if take > row.Quantity {
			take = row.Quantity
		}
		line, err := stockLine(domain.StockOut, take, cell)
		if err != nil {
			return fmt.Errorf("confirm outbound: %w", err)
		}
		lines = append(lines, line)
		remaining -= take
	}
	if remaining != 0 || len(lines) == 0 {
		return fmt.Errorf("confirm outbound: staged goods do not cover parcel %d", parcel.Sequence)
	}
	reference, err := domain.NewSourceFactReference(command.FactID.String())
	if err != nil {
		return fmt.Errorf("confirm outbound: %w", err)
	}
	posted, err := handler.poster.Post(ctx, PostStockMovementCommand{
		Tenant:  command.Tenant,
		Request: domain.NewStockMovementRequest(request.Owner(), request.Warehouse(), domain.StockReasonOutboundConfirmation, reference, command.OccurredAt.UTC(), lines),
	})
	if err != nil {
		return fmt.Errorf("confirm outbound: %w", err)
	}
	if posted.Outcome() != PostStockPosted && posted.Outcome() != PostStockAlreadyPosted {
		return fmt.Errorf("confirm outbound: stock was not posted")
	}
	return nil
}
