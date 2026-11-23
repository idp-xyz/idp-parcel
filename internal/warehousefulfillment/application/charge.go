package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// ChargeOutcome 是形成或更正仓储作业收费发生项的结果。没有金额。
type ChargeOutcome uint8

const (
	ChargeOutcomeInvalid ChargeOutcome = iota
	ChargeFormed
	ChargeExisting
	ChargeRestated
	ChargeVoided
	ChargeConflict
	ChargeNotConfigured
	ChargeNotFound
	ChargeContentInvalid
)

func (outcome ChargeOutcome) String() string {
	switch outcome {
	case ChargeFormed:
		return "FORMED"
	case ChargeExisting:
		return "EXISTING_RESULT"
	case ChargeRestated:
		return "RESTATED"
	case ChargeVoided:
		return "VOIDED"
	case ChargeConflict:
		return "CONTENT_CONFLICT"
	case ChargeNotConfigured:
		return "NOT_CONFIGURED"
	case ChargeNotFound:
		return "NOT_FOUND"
	case ChargeContentInvalid:
		return "CONTENT_INVALID"
	default:
		return ""
	}
}

type ChargeResult struct {
	outcome    ChargeOutcome
	occurrence string
	version    int
}

func (result ChargeResult) Outcome() ChargeOutcome { return result.outcome }
func (result ChargeResult) Occurrence() string     { return result.occurrence }
func (result ChargeResult) Version() int           { return result.version }

// ChargeDraft 是一份要形成的发生项。计量与属性来自作业事实，不带金额。
type ChargeDraft struct {
	Kind       string
	Source     string
	Owner      domain.OwnerAccountID
	Warehouse  domain.WarehouseCode
	Occurred   time.Time
	Measures   []ports.ChargeMeasure
	Attributes []ports.ChargeAttribute
	Lines      []ports.ChargeLine
}

// ChargeHandler 形成发生项并追加更正版本。重量体积单位、库龄口径与业务时区没登记时，要带它们的不形成。
type ChargeHandler struct {
	charges    ports.ChargeStore
	identities ports.ChargeIdentities
	handoff    ports.ChargeHandoff
	warehouses ports.WarehouseLookup
	clock      ports.Clock
	// stock 缺席时仓租仍形成，只是没有在库格。库区用途不在余额行上，行上这一格记「无」。
	stock ports.StockOnHandRead
}

func NewChargeHandler(
	charges ports.ChargeStore,
	identities ports.ChargeIdentities,
	handoff ports.ChargeHandoff,
	warehouses ports.WarehouseLookup,
	clock ports.Clock,
) *ChargeHandler {
	return &ChargeHandler{charges: charges, identities: identities, handoff: handoff, warehouses: warehouses, clock: clock}
}

func (handler *ChargeHandler) WithStock(stock ports.StockOnHandRead) *ChargeHandler {
	handler.stock = stock
	return handler
}

func (handler *ChargeHandler) Form(ctx context.Context, tenant domain.TenantID, draft ChargeDraft) (ChargeResult, error) {
	if draft.Kind == "" || draft.Source == "" || draft.Occurred.IsZero() {
		return ChargeResult{outcome: ChargeContentInvalid}, nil
	}
	if needsMeasureUnits(draft) {
		_, _, found, err := handler.charges.LoadUnits(ctx, tenant, draft.Warehouse)
		if err != nil {
			return ChargeResult{}, fmt.Errorf("form charge occurrence: %w", err)
		}
		if !found {
			return ChargeResult{outcome: ChargeNotConfigured}, nil
		}
	}
	existing, found, err := handler.charges.LoadBySource(ctx, tenant, draft.Kind, draft.Source)
	if err != nil {
		return ChargeResult{}, fmt.Errorf("form charge occurrence: %w", err)
	}
	if found {
		return ChargeResult{outcome: ChargeExisting, occurrence: existing.ID, version: existing.Version}, nil
	}
	id, err := handler.identities.NextChargeOccurrenceID()
	if err != nil {
		return ChargeResult{}, fmt.Errorf("form charge occurrence: %w", err)
	}
	version := ports.ChargeVersion{
		ID: id, Version: 1, Correction: "ORIGINAL", Kind: draft.Kind, Owner: draft.Owner.String(), Warehouse: draft.Warehouse.String(),
		Source: draft.Source, Occurred: draft.Occurred, Measures: draft.Measures, Attributes: blankAttributes(draft.Attributes), Lines: draft.Lines,
	}
	if err := handler.charges.SaveOriginal(ctx, tenant, version); err != nil {
		return ChargeResult{}, fmt.Errorf("form charge occurrence: %w", err)
	}
	if err := handler.handoff.HandOffChargeOccurrence(ctx, tenant.String(), id, 1); err != nil {
		return ChargeResult{}, fmt.Errorf("form charge occurrence: %w", err)
	}
	return ChargeResult{outcome: ChargeFormed, occurrence: id, version: 1}, nil
}

func (handler *ChargeHandler) Void(ctx context.Context, tenant domain.TenantID, id string, version int) (ChargeResult, error) {
	return handler.correct(ctx, tenant, id, version, "VOID", ChargeDraft{})
}

func (handler *ChargeHandler) Restate(ctx context.Context, tenant domain.TenantID, id string, version int, draft ChargeDraft) (ChargeResult, error) {
	return handler.correct(ctx, tenant, id, version, "RESTATE", draft)
}

func (handler *ChargeHandler) Read(ctx context.Context, tenant domain.TenantID, id string, version int) (ports.ChargeVersion, bool, error) {
	return handler.charges.LoadVersion(ctx, tenant, id, version)
}

func (handler *ChargeHandler) RegisterUnits(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, weight, volume string) (ChargeResult, error) {
	if weight == "" || volume == "" {
		return ChargeResult{outcome: ChargeContentInvalid}, nil
	}
	_, _, found, err := handler.charges.LoadUnits(ctx, tenant, warehouse)
	if err != nil {
		return ChargeResult{}, fmt.Errorf("register charge units: %w", err)
	}
	if found {
		return ChargeResult{outcome: ChargeExisting}, nil
	}
	if err := handler.charges.SaveUnits(ctx, tenant, warehouse, weight, volume, handler.clock.Now()); err != nil {
		return ChargeResult{}, fmt.Errorf("register charge units: %w", err)
	}
	return ChargeResult{outcome: ChargeFormed}, nil
}

func (handler *ChargeHandler) RegisterProject(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, name string) (ChargeResult, error) {
	if name == "" {
		return ChargeResult{outcome: ChargeContentInvalid}, nil
	}
	found, err := handler.charges.LoadProject(ctx, tenant, warehouse, name)
	if err != nil {
		return ChargeResult{}, fmt.Errorf("register charge project: %w", err)
	}
	if found {
		return ChargeResult{outcome: ChargeExisting}, nil
	}
	if err := handler.charges.SaveProject(ctx, tenant, warehouse, name, handler.clock.Now()); err != nil {
		return ChargeResult{}, fmt.Errorf("register charge project: %w", err)
	}
	return ChargeResult{outcome: ChargeFormed}, nil
}

// FormValueAdded 只在租户登记过这个项目名时形成。没登记不形成，也不编一个默认项目。
func (handler *ChargeHandler) FormValueAdded(ctx context.Context, tenant domain.TenantID, name string, draft ChargeDraft) (ChargeResult, error) {
	found, err := handler.charges.LoadProject(ctx, tenant, draft.Warehouse, name)
	if err != nil {
		return ChargeResult{}, fmt.Errorf("form value-added charge: %w", err)
	}
	if !found {
		return ChargeResult{outcome: ChargeNotConfigured}, nil
	}
	draft.Kind = "VALUE_ADDED"
	return handler.Form(ctx, tenant, draft)
}

func (handler *ChargeHandler) RegisterAging(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, basis string) (ChargeResult, error) {
	if basis != "ACTUAL_BATCH" && basis != "THEORETICAL_FIFO" {
		return ChargeResult{outcome: ChargeContentInvalid}, nil
	}
	_, _, found, err := handler.charges.LoadAging(ctx, tenant, warehouse)
	if err != nil {
		return ChargeResult{}, fmt.Errorf("register aging basis: %w", err)
	}
	if found {
		return ChargeResult{outcome: ChargeExisting}, nil
	}
	if err := handler.charges.SaveAging(ctx, tenant, warehouse, basis, handler.clock.Now()); err != nil {
		return ChargeResult{}, fmt.Errorf("register aging basis: %w", err)
	}
	return ChargeResult{outcome: ChargeFormed}, nil
}

// CloseStorageDay 在计费日结束后形成一份仓租。时区、库龄口径或体积单位没登记时不形成。没有手工填写口。
func (handler *ChargeHandler) CloseStorageDay(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, day time.Time) (ChargeResult, error) {
	revision, found, err := handler.warehouses.LoadLatestWarehouse(ctx, tenant, warehouse)
	if err != nil {
		return ChargeResult{}, fmt.Errorf("close storage day: %w", err)
	}
	if !found || !revision.Content().BusinessTimeZone().Registered() {
		return ChargeResult{outcome: ChargeNotConfigured}, nil
	}
	basis, ready, agingFound, err := handler.charges.LoadAging(ctx, tenant, warehouse)
	if err != nil {
		return ChargeResult{}, fmt.Errorf("close storage day: %w", err)
	}
	_, _, unitsFound, err := handler.charges.LoadUnits(ctx, tenant, warehouse)
	if err != nil {
		return ChargeResult{}, fmt.Errorf("close storage day: %w", err)
	}
	if !agingFound || !unitsFound {
		return ChargeResult{outcome: ChargeNotConfigured}, nil
	}
	location, err := time.LoadLocation(revision.Content().BusinessTimeZone().String())
	if err != nil {
		return ChargeResult{outcome: ChargeNotConfigured}, nil
	}
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, location)
	if !handler.clock.Now().After(start.Add(24 * time.Hour)) {
		return ChargeResult{outcome: ChargeContentInvalid}, nil
	}
	if !ready.Before(start.Add(24 * time.Hour)) {
		return ChargeResult{outcome: ChargeNotConfigured}, nil
	}
	lines, err := handler.storageLines(ctx, tenant, owner, warehouse)
	if err != nil {
		return ChargeResult{}, err
	}
	return handler.Form(ctx, tenant, ChargeDraft{
		Kind: "STORAGE", Source: "storage:" + owner.String() + "/" + warehouse.String() + "/" + start.Format("2006-01-02"),
		Owner: owner, Warehouse: warehouse, Occurred: start, Lines: lines,
		Attributes: []ports.ChargeAttribute{{Name: "AGING_BASIS", Value: basis}},
	})
}

func (handler *ChargeHandler) storageLines(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode) ([]ports.ChargeLine, error) {
	if handler.stock == nil {
		return nil, nil
	}
	rows, err := handler.stock.ListOnHandCells(ctx, tenant, owner, warehouse)
	if err != nil {
		return nil, fmt.Errorf("close storage day: %w", err)
	}
	lines := make([]ports.ChargeLine, 0, len(rows))
	for _, row := range rows {
		if row.Quantity <= 0 {
			continue
		}
		state := row.State
		if state == "" {
			state = "无"
		}
		lines = append(lines, ports.ChargeLine{
			Kind: "STORAGE_CELL",
			Measures: []ports.ChargeMeasure{
				{Unit: "PIECE", Quantity: fmt.Sprintf("%d", row.Quantity)},
				{Unit: "CUBIC_METRE", Unmeasured: true},
			},
			Attributes: []ports.ChargeAttribute{
				{Name: "STOCK_STATE", Value: state},
				{Name: "STOCK_FORM", Value: "GOODS"},
				{Name: "ZONE_USAGE", Value: "无"},
				{Name: "INBOUND_BATCH", Value: row.InboundBatchID},
			},
		})
	}
	return lines, nil
}

func (handler *ChargeHandler) correct(ctx context.Context, tenant domain.TenantID, id string, version int, correction string, draft ChargeDraft) (ChargeResult, error) {
	target, found, err := handler.charges.LoadVersion(ctx, tenant, id, version)
	if err != nil {
		return ChargeResult{}, fmt.Errorf("correct charge occurrence: %w", err)
	}
	if !found {
		return ChargeResult{outcome: ChargeNotFound}, nil
	}
	successor, found, err := handler.charges.Successor(ctx, tenant, id, version)
	if err != nil {
		return ChargeResult{}, fmt.Errorf("correct charge occurrence: %w", err)
	}
	if found {
		if successor.Correction == correction {
			return ChargeResult{outcome: ChargeExisting, occurrence: id, version: successor.Version}, nil
		}
		return ChargeResult{outcome: ChargeConflict}, nil
	}
	next := ports.ChargeVersion{
		ID: id, Version: version + 1, Supersedes: version, Correction: correction, Kind: target.Kind,
		Owner: target.Owner, Warehouse: target.Warehouse, Source: target.Source, Occurred: target.Occurred,
		Measures: []ports.ChargeMeasure{}, Attributes: []ports.ChargeAttribute{}, Lines: []ports.ChargeLine{},
	}
	if correction == "RESTATE" {
		next.Measures = draft.Measures
		next.Attributes = blankAttributes(draft.Attributes)
		next.Lines = draft.Lines
		if !draft.Occurred.IsZero() {
			next.Occurred = draft.Occurred
		}
	}
	if err := handler.charges.SaveCorrection(ctx, tenant, next); err != nil {
		return ChargeResult{}, fmt.Errorf("correct charge occurrence: %w", err)
	}
	if err := handler.handoff.HandOffChargeOccurrence(ctx, tenant.String(), id, next.Version); err != nil {
		return ChargeResult{}, fmt.Errorf("correct charge occurrence: %w", err)
	}
	if correction == "VOID" {
		return ChargeResult{outcome: ChargeVoided, occurrence: id, version: next.Version}, nil
	}
	return ChargeResult{outcome: ChargeRestated, occurrence: id, version: next.Version}, nil
}

func needsMeasureUnits(draft ChargeDraft) bool {
	switch draft.Kind {
	case "INBOUND_HANDLING", "OUTBOUND_HANDLING", "STORAGE", "PROCESSING", "DESTRUCTION":
		return true
	}
	if measureUsesMass(draft.Measures) {
		return true
	}
	for _, line := range draft.Lines {
		if measureUsesMass(line.Measures) {
			return true
		}
	}
	return false
}

func measureUsesMass(measures []ports.ChargeMeasure) bool {
	for _, measure := range measures {
		if measure.Unit == "KILOGRAM" || measure.Unit == "CUBIC_METRE" {
			return true
		}
	}
	return false
}

// RegisterChargeUnitsCommand 登记一个仓的重量单位与体积单位。不给生产默认。
type RegisterChargeUnitsCommand struct {
	Warehouse domain.WarehouseCode
	Weight    string
	Volume    string
}

// RegisterChargeAgingCommand 登记库龄口径。只认实批或理论先进先出。
type RegisterChargeAgingCommand struct {
	Warehouse domain.WarehouseCode
	Basis     string
}

// RegisterChargeProjectCommand 登记一个增值服务项目名。
type RegisterChargeProjectCommand struct {
	Warehouse domain.WarehouseCode
	Name      string
}

// CloseStorageDayCommand 是一次仓租日结。不收计量，计量由日结自己形成。
type CloseStorageDayCommand struct {
	Owner     domain.OwnerAccountID
	Warehouse domain.WarehouseCode
	Day       time.Time
}

// ChargeCorrectionCommand 点名要更正的那一版。重述带整份内容，作废不带计量。
type ChargeCorrectionCommand struct {
	Occurrence string
	Version    int
	Draft      ChargeDraft
}

func blankAttributes(attributes []ports.ChargeAttribute) []ports.ChargeAttribute {
	if attributes == nil {
		return []ports.ChargeAttribute{}
	}
	return attributes
}
