package application

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// chargeFormer 在来源事实的同一笔事务里形成发生项。缺席时来源事实照样成立。
type chargeFormer interface {
	Form(context.Context, domain.TenantID, ChargeDraft) (ChargeResult, error)
}

// noteCharge 把未配置留在发生项上：单位没登记时不形成，来源事实已经成立。库写失败要让来源事实一起回滚。
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
		pieces += line.Quantity()
		usage, err := handler.zoneUsage(ctx, command.Tenant, command.Warehouse, line.Location())
		if err != nil {
			return err
		}
		chargeLines = append(chargeLines, receiptChargeLine("RECEIPT_LINE", line.Quantity(), line.State().String(), "GOODS", usage))
	}
	for _, carton := range cartons {
		usage, err := handler.zoneUsage(ctx, command.Tenant, command.Warehouse, carton.Location())
		if err != nil {
			return err
		}
		chargeLines = append(chargeLines, receiptChargeLine("RECEIPT_LINE", 1, "GOOD", "CARTON", usage))
	}
	measures := []ports.ChargeMeasure{{Unit: "SHIPMENT", Quantity: "1"}, {Unit: "PIECE", Quantity: strconv.FormatInt(pieces, 10)}}
	if len(cartons) > 0 {
		measures = append(measures, ports.ChargeMeasure{Unit: "CARTON", Quantity: strconv.Itoa(len(cartons))})
	}
	arrival := advice.Arrival().String()
	if arrival == "" {
		arrival = "无"
	}
	return noteCharge(ctx, handler.charges, command.Tenant, ChargeDraft{
		Kind: "INBOUND_HANDLING", Source: command.FactID.String(), Owner: advice.Owner(), Warehouse: command.Warehouse,
		Occurred: command.OccurredAt, Measures: measures, Lines: chargeLines,
		Attributes: []ports.ChargeAttribute{{Name: "ARRIVAL_MODE", Value: arrival}},
	})
}

func (handler *ConfirmReceiptHandler) zoneUsage(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, location domain.LocationCode) (string, error) {
	place, found, err := handler.locations.LoadLatestLocation(ctx, tenant, warehouse, location)
	if err != nil {
		return "", fmt.Errorf("confirm receipt: %w", err)
	}
	if !found {
		return "无", nil
	}
	zone, found, err := handler.zones.LoadLatestZone(ctx, tenant, warehouse, place.Content().ZoneCode())
	if err != nil {
		return "", fmt.Errorf("confirm receipt: %w", err)
	}
	if !found || zone.Content().Usage().String() == "" {
		return "无", nil
	}
	return zone.Content().Usage().String(), nil
}

func (handler *WaveHandler) formOutboundCharge(ctx context.Context, command ConfirmCommand, request domain.OutboundRequest, parcel ports.PackParcel) error {
	if handler.charges == nil {
		return nil
	}
	return noteCharge(ctx, handler.charges, command.Tenant, outboundChargeDraft(
		command.FactID.String(), request, parcel.Quantity, parcel.Packaging, parcel.Weight, parcel.WeightUnit, command.OccurredAt,
	))
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
	draft := outboundChargeDraft(result.ID, request, quantity, "AGENT", result.Weight, result.WeightUnit, result.Reported)
	if !found {
		draft.Measures[1] = ports.ChargeMeasure{Unit: "PIECE", Unmeasured: true}
	}
	return noteCharge(ctx, handler.charges, tenant, draft)
}

func (handler *ReturnHandler) formReturnReceiptCharge(ctx context.Context, command ReceiveReturnCommand, owner domain.OwnerAccountID, occurred time.Time) error {
	if handler.charges == nil {
		return nil
	}
	return noteCharge(ctx, handler.charges, command.Tenant, ChargeDraft{
		Kind: "RETURN_HANDLING", Source: command.FactID.String(), Owner: owner, Warehouse: command.Warehouse, Occurred: occurred,
		Measures:   []ports.ChargeMeasure{{Unit: "SHIPMENT", Quantity: "1"}, {Unit: "PIECE", Quantity: strconv.FormatInt(command.Quantity, 10)}},
		Attributes: []ports.ChargeAttribute{{Name: "ARRIVAL_MODE", Value: "无"}},
		Lines:      []ports.ChargeLine{receiptChargeLine("RETURN_LINE", command.Quantity, command.State.String(), "GOODS", "RETURN_STAGING")},
	})
}

func (handler *ReturnHandler) formDispositionCharge(ctx context.Context, command DisposeReturnCommand, receipt ports.ReturnReceipt, occurred time.Time) error {
	if handler.charges == nil {
		return nil
	}
	owner, err := domain.NewOwnerAccountID(receipt.Owner)
	if err != nil {
		return err
	}
	kind := "RETURN_HANDLING"
	if command.Destination == domain.ReturnDestinationDestroy {
		kind = "DESTRUCTION"
	}
	return noteCharge(ctx, handler.charges, command.Tenant, ChargeDraft{
		Kind: kind, Source: command.Decision.String(), Owner: owner, Warehouse: receipt.Warehouse, Occurred: occurred,
		Measures:   []ports.ChargeMeasure{{Unit: "SHIPMENT", Quantity: "1"}, {Unit: "PIECE", Quantity: strconv.FormatInt(receipt.Quantity, 10)}},
		Attributes: []ports.ChargeAttribute{{Name: "RETURN_DESTINATION", Value: command.Destination.String()}},
	})
}

func (handler *InstructionHandler) formProcessingCharge(ctx context.Context, command ExecuteInstructionCommand, instruction domain.Instruction) error {
	if handler.charges == nil || instruction.Kind() == domain.InstructionRelabel {
		return nil
	}
	return noteCharge(ctx, handler.charges, command.Tenant, ChargeDraft{
		Kind: "PROCESSING", Source: command.FactID.String(), Owner: instruction.Owner(), Warehouse: instruction.Warehouse(),
		Occurred:   command.OccurredAt,
		Measures:   []ports.ChargeMeasure{{Unit: "SHIPMENT", Quantity: "1"}, {Unit: "PIECE", Quantity: strconv.FormatInt(command.DoneQuantity, 10)}},
		Attributes: []ports.ChargeAttribute{{Name: "INSTRUCTION_KIND", Value: instruction.Kind().String()}},
	})
}

func outboundChargeDraft(source string, request domain.OutboundRequest, quantity int64, packaging, weight, weightUnit string, occurred time.Time) ChargeDraft {
	service := request.Service()
	if service == "" {
		service = "无"
	}
	if packaging == "" {
		packaging = "无"
	}
	piece := ports.ChargeMeasure{Unit: "PIECE", Quantity: strconv.FormatInt(quantity, 10)}
	if quantity == 0 {
		piece = ports.ChargeMeasure{Unit: "PIECE", Unmeasured: true}
	}
	return ChargeDraft{
		Kind: "OUTBOUND_HANDLING", Source: source, Owner: request.Owner(), Warehouse: request.Warehouse(), Occurred: occurred,
		Measures: []ports.ChargeMeasure{{Unit: "SHIPMENT", Quantity: "1"}, piece, weightMeasure(weight, weightUnit), {Unit: "CUBIC_METRE", Unmeasured: true}},
		Attributes: []ports.ChargeAttribute{
			{Name: "DISPATCH", Value: request.Dispatch().String()},
			{Name: "SERVICE", Value: service},
			{Name: "PACKAGING", Value: packaging},
			{Name: "STOCK_FORM", Value: "GOODS"},
		},
	}
}

func receiptChargeLine(kind string, quantity int64, state, form, usage string) ports.ChargeLine {
	if state == "" {
		state = "无"
	}
	return ports.ChargeLine{
		Kind: kind,
		Measures: []ports.ChargeMeasure{
			{Unit: "PIECE", Quantity: strconv.FormatInt(quantity, 10)},
			{Unit: "KILOGRAM", Unmeasured: true},
			{Unit: "CUBIC_METRE", Unmeasured: true},
		},
		Attributes: []ports.ChargeAttribute{
			{Name: "STOCK_STATE", Value: state},
			{Name: "STOCK_FORM", Value: form},
			{Name: "ZONE_USAGE", Value: usage},
		},
	}
}

func weightMeasure(quantity, unit string) ports.ChargeMeasure {
	if quantity == "" {
		return ports.ChargeMeasure{Unit: "KILOGRAM", Unmeasured: true}
	}
	return ports.ChargeMeasure{Unit: "KILOGRAM", Quantity: quantity, Declared: quantity, DeclaredUnit: unit}
}
