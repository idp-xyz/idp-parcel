package application

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func (handler *RegisterInternalJobHandler) noteJobCharges(
	ctx context.Context,
	command RegisterInternalJobCommand,
	recorded []domain.RecordedInternalJobLine,
	places map[string]placeInfo,
) error {
	if handler.charges == nil {
		return nil
	}
	type groupKey struct {
		owner string
		kind  string
		label string
	}
	type group struct {
		owner  domain.OwnerAccountID
		pieces int64
		kind   string
		label  string
	}
	grouped := map[groupKey]*group{}
	var order []groupKey
	for _, recordedLine := range recorded {
		line := recordedLine.Input()
		result := recordedLine.Result()
		moved := result.Posted
		if command.Kind == domain.InternalJobForm && result.PostedInbound > 0 {
			moved = result.PostedInbound
		}
		if moved <= 0 {
			continue
		}
		sourceUsage := places[line.Source().String()].usage
		targetUsage := places[line.Target().String()].usage
		kind, label := jobChargeKind(command.Kind, sourceUsage, targetUsage, line)
		if kind == "" {
			continue
		}
		key := groupKey{owner: line.Owner().String(), kind: kind, label: label}
		item := grouped[key]
		if item == nil {
			item = &group{owner: line.Owner(), kind: kind, label: label}
			grouped[key] = item
			order = append(order, key)
		}
		item.pieces += moved
	}
	for _, key := range order {
		item := grouped[key]
		source := command.FactID.String()
		if len(order) > 1 {
			source = command.FactID.String() + "/" + item.kind + "/" + item.label
		}
		var err error
		switch item.kind {
		case "RETURN_HANDLING":
			err = noteCharge(ctx, handler.charges, command.Tenant, dispositionDraft(
				source, item.owner, command.Warehouse, command.OccurredAt, item.pieces, item.label,
			))
		case "VALUE_ADDED":
			err = noteValueAdded(ctx, handler.charges, command.Tenant, item.label, ChargeDraft{
				Source: source, Owner: item.owner, Warehouse: command.Warehouse, Occurred: command.OccurredAt,
				Measures: valueAddedMeasures(item.pieces, false, 0),
			})
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func jobChargeKind(kind domain.InternalJobKind, source, target domain.ZoneUsage, line domain.InternalJobLine) (string, string) {
	switch kind {
	case domain.InternalJobForm:
		return "VALUE_ADDED", "形态转换"
	case domain.InternalJobMove:
		if line.Source().String() == "" || line.Source() == line.Target() {
			return "", ""
		}
		// 从退货暂存出发的良品上架与转次品只形成退货处理，不另形成移库。
		if source == domain.ZoneUsageReturnStaging {
			switch {
			case target == domain.ZoneUsageDefective:
				return "RETURN_HANDLING", domain.ReturnDestinationDefective.String()
			case target == domain.ZoneUsageStorage || target == domain.ZoneUsagePicking:
				return "RETURN_HANDLING", domain.ReturnDestinationGoodPutaway.String()
			default:
				return "", ""
			}
		}
		if source == domain.ZoneUsageReceivingStaging {
			return "", ""
		}
		if source == domain.ZoneUsageStorage && target == domain.ZoneUsagePicking {
			return "VALUE_ADDED", "拣选位补货"
		}
		return "VALUE_ADDED", "移库"
	default:
		return "", ""
	}
}

func (handler *CountHandler) noteCountCharge(ctx context.Context, count domain.Count, entries map[string]ports.CountEntry) error {
	if handler.charges == nil || count.Owner().String() == "" {
		return nil
	}
	pieces := int64(0)
	for _, entry := range entries {
		for _, line := range entry.Lines {
			if line.Observed != "" {
				continue
			}
			pieces += line.Quantity
		}
	}
	return noteValueAdded(ctx, handler.charges, count.Tenant(), "盘点", ChargeDraft{
		Source: count.ID().String(), Owner: count.Owner(), Warehouse: count.Warehouse(), Occurred: handler.clock.Now(),
		Measures: valueAddedMeasures(pieces, false, len(count.Locations())),
	})
}

func (handler *InstructionHandler) noteUnboxCharge(ctx context.Context, command UnboxCommand) error {
	if handler.charges == nil {
		return nil
	}
	pieces := int64(0)
	for _, line := range command.Lines {
		pieces += line.Quantity
	}
	return noteValueAdded(ctx, handler.charges, command.Tenant, "开箱", ChargeDraft{
		Source: command.FactID.String(), Owner: command.Owner, Warehouse: command.Warehouse, Occurred: command.OccurredAt,
		Measures: valueAddedMeasures(pieces, true, 0),
	})
}

func (handler *RegisterInternalJobHandler) noteRestoreCharge(ctx context.Context, command RestoreConditionCommand) error {
	if handler.charges == nil {
		return nil
	}
	return noteValueAdded(ctx, handler.charges, command.Tenant, "次品转良品", ChargeDraft{
		Source: command.ID.String(), Owner: command.Owner, Warehouse: command.Warehouse, Occurred: handler.clock.Now(),
		Measures: valueAddedMeasures(command.Quantity, false, 0),
	})
}

func (handler *AdjustmentHandler) formDestructionCharge(
	ctx context.Context,
	tenant domain.TenantID,
	adjustment ports.StockAdjustment,
	index int,
	line ports.AdjustmentLine,
) error {
	if handler.charges == nil {
		return nil
	}
	usage := chargeNone
	place, found, err := handler.locations.LoadLatestLocation(ctx, tenant, adjustment.Warehouse, line.Location)
	if err != nil {
		return err
	}
	if found {
		zone, zoneFound, err := handler.zones.LoadLatestZone(ctx, tenant, adjustment.Warehouse, place.Content().ZoneCode())
		if err != nil {
			return err
		}
		if zoneFound && zone.Content().Usage().String() != "" {
			usage = zone.Content().Usage().String()
		}
	}
	measures := []ports.ChargeMeasure{countMeasure("SHIPMENT", 1), countMeasure("PIECE", line.Quantity)}
	weight, volume, configured, err := handler.charges.LineMass(
		ctx, tenant, adjustment.Owner, adjustment.Warehouse, line.Goods, line.Unit, domain.CartonNumber{}, line.Quantity, handler.clock.Now(),
	)
	if err != nil {
		return err
	}
	if configured {
		measures = append(measures, weight, volume)
	}
	return noteCharge(ctx, handler.charges, tenant, ChargeDraft{
		Kind: "DESTRUCTION", Source: adjustment.ID.String() + "#" + itoa(index),
		Owner: adjustment.Owner, Warehouse: adjustment.Warehouse, Occurred: handler.clock.Now(),
		Measures: measures,
		Attributes: []ports.ChargeAttribute{
			attribute("STOCK_STATE", line.State.String()),
			attribute("STOCK_FORM", "GOODS"),
			attribute("ZONE_USAGE", usage),
		},
	})
}
