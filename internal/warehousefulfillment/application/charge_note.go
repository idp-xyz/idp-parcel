package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// chargeFormer 在来源事实的同一笔事务里形成发生项。缺席时来源事实照样成立。
type chargeFormer interface {
	Form(context.Context, domain.TenantID, ChargeDraft) (ChargeResult, error)
	VoidSource(context.Context, domain.TenantID, string, string) error
	FormValueAdded(context.Context, domain.TenantID, string, ChargeDraft) (ChargeResult, error)
	CurrentUnits(context.Context, domain.TenantID, domain.WarehouseCode) (string, string, bool, error)
	LineMass(context.Context, domain.TenantID, domain.OwnerAccountID, domain.WarehouseCode, domain.GoodsCode, domain.GoodsUnitName, domain.CartonNumber, int64, time.Time) (ports.ChargeMeasure, ports.ChargeMeasure, bool, error)
}

// noteCharge 把未配置留在发生项上：要带的单位没登记时不形成，来源事实已经成立。库写失败要让来源事实一起回滚。
func noteCharge(ctx context.Context, former chargeFormer, tenant domain.TenantID, draft ChargeDraft) error {
	if former == nil {
		return nil
	}
	result, err := former.Form(ctx, tenant, draft)
	if err != nil {
		return err
	}
	switch result.Outcome() {
	case ChargeFormed, ChargeExisting, ChargeNotConfigured:
		return nil
	default:
		return fmt.Errorf("form charge occurrence: %s", result.Outcome())
	}
}

func noteValueAdded(ctx context.Context, former chargeFormer, tenant domain.TenantID, name string, draft ChargeDraft) error {
	if former == nil || name == "" {
		return nil
	}
	result, err := former.FormValueAdded(ctx, tenant, name, draft)
	if err != nil {
		return err
	}
	switch result.Outcome() {
	case ChargeFormed, ChargeExisting, ChargeNotConfigured:
		return nil
	default:
		return fmt.Errorf("form charge occurrence: %s", result.Outcome())
	}
}

func (handler *ConfirmReceiptHandler) WithCharges(charges *ChargeHandler) *ConfirmReceiptHandler {
	handler.charges = charges
	return handler
}

func (handler *WaveHandler) WithCharges(charges *ChargeHandler) *WaveHandler {
	handler.charges = charges
	return handler
}

func (handler *AgentHandler) WithCharges(charges *ChargeHandler) *AgentHandler {
	handler.charges = charges
	return handler
}

func (handler *ReturnHandler) WithCharges(charges *ChargeHandler) *ReturnHandler {
	handler.charges = charges
	return handler
}

func (handler *InstructionHandler) WithCharges(charges *ChargeHandler) *InstructionHandler {
	handler.charges = charges
	return handler
}

func (handler *ConfirmReceiptHandler) formInboundCharge(
	ctx context.Context,
	command ConfirmReceiptCommand,
	advice domain.PreAdvice,
	lines []domain.ReceiptLine,
	cartons []domain.CartonReceiptLine,
) error {
	if handler.charges == nil {
		return nil
	}
	pieces := int64(0)
	chargeLines := make([]ports.ChargeLine, 0, len(lines)+len(cartons))
	for _, line := range lines {
		revision, found, err := handler.goods.LoadGoodsAt(ctx, command.Tenant, advice.Owner(), line.Goods(), command.OccurredAt)
		if err != nil {
			return err
		}
		counted := line.Quantity()
		if found {
			counted = baseQuantity(revision, line.Unit(), line.Quantity())
		}
		pieces += counted
		usage, err := handler.zoneUsage(ctx, command.Tenant, command.Warehouse, line.Location())
		if err != nil {
			return err
		}
		built, err := handler.receiptLine(ctx, command, advice.Owner(), "RECEIPT_LINE", line.Goods(), line.Unit(), domain.CartonNumber{}, line.Quantity(), line.State().String(), "GOODS", usage)
		if err != nil {
			return err
		}
		chargeLines = append(chargeLines, built)
	}
	for _, carton := range cartons {
		usage, err := handler.zoneUsage(ctx, command.Tenant, command.Warehouse, carton.Location())
		if err != nil {
			return err
		}
		built, err := handler.receiptLine(ctx, command, advice.Owner(), "RECEIPT_LINE", domain.GoodsCode{}, domain.GoodsUnitName{}, carton.Number(), 1, chargeNone, "CARTON", usage)
		if err != nil {
			return err
		}
		chargeLines = append(chargeLines, built)
	}
	measures := []ports.ChargeMeasure{countMeasure("SHIPMENT", 1), countMeasure("PIECE", pieces)}
	if len(cartons) > 0 {
		measures = append(measures, countMeasure("CARTON", int64(len(cartons))))
	}
	return noteCharge(ctx, handler.charges, command.Tenant, ChargeDraft{
		Kind: "INBOUND_HANDLING", Source: command.FactID.String(), Owner: advice.Owner(), Warehouse: command.Warehouse,
		Occurred: command.OccurredAt, Measures: measures, Lines: chargeLines,
		Attributes: []ports.ChargeAttribute{attribute("ARRIVAL_MODE", advice.Arrival().String())},
	})
}

func (handler *ConfirmReceiptHandler) receiptLine(
	ctx context.Context,
	command ConfirmReceiptCommand,
	owner domain.OwnerAccountID,
	kind string,
	goods domain.GoodsCode,
	level domain.GoodsUnitName,
	carton domain.CartonNumber,
	quantity int64,
	state, form, usage string,
) (ports.ChargeLine, error) {
	unit := "PIECE"
	if form == "CARTON" {
		unit = "CARTON"
	}
	measures := []ports.ChargeMeasure{countMeasure(unit, quantity)}
	weight, volume, configured, err := handler.charges.LineMass(ctx, command.Tenant, owner, command.Warehouse, goods, level, carton, quantity, command.OccurredAt)
	if err != nil {
		return ports.ChargeLine{}, err
	}
	if configured {
		measures = append(measures, weight, volume)
	}
	return ports.ChargeLine{
		Kind:     kind,
		Measures: measures,
		Attributes: []ports.ChargeAttribute{
			attribute("STOCK_STATE", state),
			attribute("STOCK_FORM", form),
			attribute("ZONE_USAGE", usage),
		},
	}, nil
}

func (handler *ConfirmReceiptHandler) zoneUsage(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, location domain.LocationCode) (string, error) {
	place, found, err := handler.locations.LoadLatestLocation(ctx, tenant, warehouse, location)
	if err != nil {
		return "", fmt.Errorf("confirm receipt: %w", err)
	}
	if !found {
		return chargeNone, nil
	}
	zone, found, err := handler.zones.LoadLatestZone(ctx, tenant, warehouse, place.Content().ZoneCode())
	if err != nil {
		return "", fmt.Errorf("confirm receipt: %w", err)
	}
	if !found || zone.Content().Usage().String() == "" {
		return chargeNone, nil
	}
	return zone.Content().Usage().String(), nil
}

func (handler *WaveHandler) formOutboundCharge(ctx context.Context, command ConfirmCommand, request domain.OutboundRequest, parcel ports.PackParcel) error {
	if handler.charges == nil {
		return nil
	}
	draft, err := outboundChargeDraft(
		ctx, handler.charges, command.FactID.String(), request, parcel.Quantity, false, parcel.Packaging,
		parcel.Weight, parcel.WeightUnit, parcel.Length, parcel.Width, parcel.Height, parcel.Dimension, command.OccurredAt,
	)
	if err != nil {
		return err
	}
	return noteCharge(ctx, handler.charges, command.Tenant, draft)
}

func (handler *AgentHandler) formReportedCharge(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequest, result ports.AgentResult) error {
	if handler.charges == nil {
		return nil
	}
	_, _, quantity, found, err := handler.agents.FirstLine(ctx, tenant, request.ID())
	if err != nil {
		return fmt.Errorf("record agent result: %w", err)
	}
	if !found {
		quantity = 0
	}
	draft, err := outboundChargeDraft(ctx, handler.charges, result.ID, request, quantity, false, chargeNone, result.Weight, result.WeightUnit, result.Length, result.Width, result.Height, result.Dimension, result.Reported)
	if err != nil {
		return err
	}
	if !found {
		draft.Measures = replacePiece(draft.Measures, ports.ChargeMeasure{Unit: "PIECE", Unmeasured: true})
	}
	return noteCharge(ctx, handler.charges, tenant, draft)
}

func (handler *ReturnHandler) formReturnReceiptCharge(ctx context.Context, command ReceiveReturnCommand, owner domain.OwnerAccountID, occurred time.Time) error {
	if handler.charges == nil {
		return nil
	}
	usage := "RETURN_STAGING"
	if looked, err := handler.returnZoneUsage(ctx, command.Tenant, command.Warehouse, command.Location); err != nil {
		return err
	} else if looked != "" {
		usage = looked
	}
	line := ports.ChargeLine{
		Kind: "RETURN_LINE",
		Measures: []ports.ChargeMeasure{
			countMeasure("PIECE", command.Quantity),
		},
		Attributes: []ports.ChargeAttribute{
			attribute("STOCK_STATE", command.State.String()),
			attribute("STOCK_FORM", "GOODS"),
			attribute("ZONE_USAGE", usage),
		},
	}
	weight, volume, configured, err := handler.charges.LineMass(ctx, command.Tenant, owner, command.Warehouse, command.Goods, command.Unit, domain.CartonNumber{}, command.Quantity, occurred)
	if err != nil {
		return err
	}
	if configured {
		line.Measures = append(line.Measures, weight, volume)
	}
	return noteCharge(ctx, handler.charges, command.Tenant, ChargeDraft{
		Kind: "RETURN_HANDLING", Source: command.FactID.String(), Owner: owner, Warehouse: command.Warehouse, Occurred: occurred,
		Measures:   []ports.ChargeMeasure{countMeasure("SHIPMENT", 1), countMeasure("PIECE", command.Quantity)},
		Attributes: []ports.ChargeAttribute{attribute("ARRIVAL_MODE", chargeNone)},
		Lines:      []ports.ChargeLine{line},
	})
}

func (handler *ReturnHandler) returnZoneUsage(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, location domain.LocationCode) (string, error) {
	place, found, err := handler.locations.LoadLatestLocation(ctx, tenant, warehouse, location)
	if err != nil || !found {
		return "", err
	}
	zone, found, err := handler.zones.LoadLatestZone(ctx, tenant, warehouse, place.Content().ZoneCode())
	if err != nil || !found {
		return "", err
	}
	return zone.Content().Usage().String(), nil
}

func (handler *ReturnHandler) formDispositionCharge(ctx context.Context, command DisposeReturnCommand, receipt ports.ReturnReceipt, occurred time.Time) error {
	if handler.charges == nil {
		return nil
	}
	// 选销毁不形成退货处理。良品上架与转次品要等移出退货暂存的那笔流水，不在处置作出时形成。
	if command.Destination != domain.ReturnDestinationReturnToOwner {
		return nil
	}
	owner, err := domain.NewOwnerAccountID(receipt.Owner)
	if err != nil {
		return err
	}
	return noteCharge(ctx, handler.charges, command.Tenant, dispositionDraft(command.Decision.String(), owner, receipt.Warehouse, occurred, receipt.Quantity, command.Destination.String()))
}

func dispositionDraft(source string, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, occurred time.Time, pieces int64, destination string) ChargeDraft {
	return ChargeDraft{
		Kind: "RETURN_HANDLING", Source: source, Owner: owner, Warehouse: warehouse, Occurred: occurred,
		Measures:   []ports.ChargeMeasure{countMeasure("SHIPMENT", 1), countMeasure("PIECE", pieces)},
		Attributes: []ports.ChargeAttribute{attribute("RETURN_DESTINATION", destination)},
	}
}

func (handler *InstructionHandler) formProcessingCharge(ctx context.Context, command ExecuteInstructionCommand, instruction domain.Instruction) error {
	if handler.charges == nil {
		return nil
	}
	if instruction.Kind() == domain.InstructionRelabel {
		return noteValueAdded(ctx, handler.charges, command.Tenant, relabelProject(instruction.FromAlias().String(), instruction.ToAlias().String()), ChargeDraft{
			Source: command.FactID.String(), Owner: instruction.Owner(), Warehouse: instruction.Warehouse(), Occurred: command.OccurredAt,
			Measures: valueAddedMeasures(command.DoneQuantity, false, 0),
		})
	}
	pieces := int64(0)
	lines := make([]ports.ChargeLine, 0, len(command.Lines))
	for _, line := range command.Lines {
		kind := "INPUT_LINE"
		if line.Direction == domain.StockIn {
			kind = "OUTPUT_LINE"
			counted := line.Quantity
			revision, found, err := handler.goods.LoadGoodsAt(ctx, command.Tenant, instruction.Owner(), line.Goods, command.OccurredAt)
			if err != nil {
				return err
			}
			if found {
				counted = baseQuantity(revision, line.Unit, line.Quantity)
			}
			pieces += counted
		}
		lines = append(lines, ports.ChargeLine{
			Kind:     kind,
			Measures: []ports.ChargeMeasure{countMeasure("PIECE", line.Quantity)},
			Attributes: []ports.ChargeAttribute{
				attribute("STOCK_STATE", line.State.String()),
				attribute("STOCK_FORM", "GOODS"),
			},
		})
	}
	if pieces == 0 {
		pieces = command.DoneQuantity
	}
	return noteCharge(ctx, handler.charges, command.Tenant, ChargeDraft{
		Kind: "PROCESSING", Source: command.FactID.String(), Owner: instruction.Owner(), Warehouse: instruction.Warehouse(),
		Occurred:   command.OccurredAt,
		Measures:   []ports.ChargeMeasure{countMeasure("SHIPMENT", 1), countMeasure("PIECE", pieces)},
		Attributes: []ports.ChargeAttribute{attribute("STOCK_FORM", "GOODS")},
		Lines:      lines,
	})
}

func relabelProject(from, to string) string {
	switch {
	case from == "" && to != "":
		return "贴标"
	case from != "" && to == "":
		return "去掉"
	case from != "" && to != "" && from != to:
		return "换成另一个别名"
	default:
		return ""
	}
}

func valueAddedMeasures(pieces int64, unbox bool, locations int) []ports.ChargeMeasure {
	measures := []ports.ChargeMeasure{countMeasure("SHIPMENT", 1), countMeasure("PIECE", pieces)}
	if unbox {
		measures = append(measures, countMeasure("CARTON", 1))
	}
	if locations > 0 {
		measures = append(measures, countMeasure("LOCATION", int64(locations)))
	}
	return measures
}

func outboundChargeDraft(
	ctx context.Context,
	former chargeFormer,
	source string,
	request domain.OutboundRequest,
	quantity int64,
	fullCarton bool,
	packaging, weight, weightUnit, length, width, height, dimension string,
	occurred time.Time,
) (ChargeDraft, error) {
	piece := countMeasure("PIECE", quantity)
	if quantity == 0 {
		piece = ports.ChargeMeasure{Unit: "PIECE", Unmeasured: true}
	}
	measures := []ports.ChargeMeasure{countMeasure("SHIPMENT", 1), piece}
	if fullCarton {
		measures = append(measures, countMeasure("CARTON", 1))
	}
	form := "GOODS"
	if fullCarton {
		form = "CARTON"
	}
	if former != nil {
		registeredWeight, registeredVolume, found, err := former.CurrentUnits(ctx, request.Tenant(), request.Warehouse())
		if err != nil {
			return ChargeDraft{}, err
		}
		if found {
			measures = append(measures,
				observedWeight(registeredWeight, weight, weightUnit),
				observedVolume(registeredVolume, length, width, height, dimension),
			)
		}
	}
	return ChargeDraft{
		Kind: "OUTBOUND_HANDLING", Source: source, Owner: request.Owner(), Warehouse: request.Warehouse(), Occurred: occurred,
		Measures: measures,
		Attributes: []ports.ChargeAttribute{
			attribute("DISPATCH", request.Dispatch().String()),
			attribute("SERVICE", request.Service()),
			attribute("PACKAGING", packaging),
			attribute("STOCK_FORM", form),
		},
	}, nil
}

func replacePiece(measures []ports.ChargeMeasure, piece ports.ChargeMeasure) []ports.ChargeMeasure {
	replaced := make([]ports.ChargeMeasure, len(measures))
	copy(replaced, measures)
	for index := range replaced {
		if replaced[index].Unit == "PIECE" {
			replaced[index] = piece
		}
	}
	return replaced
}

func (handler *WaveHandler) voidOutboundCharge(ctx context.Context, tenant domain.TenantID, fact string) error {
	if handler.charges == nil {
		return nil
	}
	return handler.charges.VoidSource(ctx, tenant, "OUTBOUND_HANDLING", fact)
}

// 出库确认被判不成立时作废那份出库作业。回补用的调增不从这里走，也不形成入库作业。
func (handler *WaveHandler) noteConfirmationVoid(ctx context.Context, tenant domain.TenantID, fact string, invalidate bool) error {
	if !invalidate {
		return nil
	}
	return handler.voidOutboundCharge(ctx, tenant, fact)
}
