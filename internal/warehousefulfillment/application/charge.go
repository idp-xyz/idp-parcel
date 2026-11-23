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
	// WeightUnit 与 VolumeUnit 非空时，这一版就用它们，不再读登记册的当前修订。
	WeightUnit string
	VolumeUnit string
}

// ChargeHandler 形成发生项并追加更正版本。重量体积单位、库龄口径与业务时区没登记时，要带它们的不形成。
type ChargeHandler struct {
	charges      ports.ChargeStore
	identities   ports.ChargeIdentities
	handoff      ports.ChargeHandoff
	warehouses   ports.WarehouseLookup
	clock        ports.Clock
	goods        ports.StockGoodsLookup
	measurements ports.WarehouseMeasurementRead
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

func (handler *ChargeHandler) WithReadings(goods ports.StockGoodsLookup, measurements ports.WarehouseMeasurementRead) *ChargeHandler {
	handler.goods = goods
	handler.measurements = measurements
	return handler
}

func (handler *ChargeHandler) Form(ctx context.Context, tenant domain.TenantID, draft ChargeDraft) (ChargeResult, error) {
	if draft.Kind == "" || draft.Source == "" || draft.Occurred.IsZero() {
		return ChargeResult{outcome: ChargeContentInvalid}, nil
	}
	weightUnit, volumeUnit, err := handler.unitsFor(ctx, tenant, draft)
	if err != nil {
		return ChargeResult{}, fmt.Errorf("form charge occurrence: %w", err)
	}
	if weightUnit == "\x00" {
		return ChargeResult{outcome: ChargeNotConfigured}, nil
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
		WeightUnit: weightUnit, VolumeUnit: volumeUnit,
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

// VoidSource 作废某一来源事实的现行版本。没有形成过，或现行版已经是作废，都不再写。
func (handler *ChargeHandler) VoidSource(ctx context.Context, tenant domain.TenantID, kind, source string) error {
	version, found, err := handler.charges.LoadBySource(ctx, tenant, kind, source)
	if err != nil {
		return err
	}
	if !found || version.Correction == "VOID" {
		return nil
	}
	result, err := handler.Void(ctx, tenant, version.ID, version.Version)
	if err != nil {
		return err
	}
	switch result.Outcome() {
	case ChargeVoided, ChargeExisting:
		return nil
	default:
		return fmt.Errorf("void charge occurrence: %s", result.Outcome())
	}
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
	// 同一对单位再登记是重放。换了词就追加一版，只作用于此后形成的发生项。
	currentWeight, currentVolume, found, err := handler.charges.LoadUnits(ctx, tenant, warehouse)
	if err != nil {
		return ChargeResult{}, fmt.Errorf("register charge units: %w", err)
	}
	if found && currentWeight == weight && currentVolume == volume {
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
	draft.Attributes = withAttribute(draft.Attributes, "PROJECT", name)
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
	location, err := time.LoadLocation(revision.Content().BusinessTimeZone().String())
	if err != nil {
		return ChargeResult{outcome: ChargeNotConfigured}, nil
	}
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, location)
	dayEnd := start.Add(24 * time.Hour)
	if !handler.clock.Now().After(dayEnd) {
		return ChargeResult{outcome: ChargeContentInvalid}, nil
	}
	// 理论先进先出只写了「把在库量归到最近的几次入库」，没写按什么切开、一次入库指哪一笔、
	// 重归之后行上的库存状态、库区用途与有无冻结跟哪一格。这一口径不猜，不形成。
	if !agingFound || !ready.Before(dayEnd) || basis == "THEORETICAL_FIFO" {
		return ChargeResult{outcome: ChargeNotConfigured}, nil
	}
	weightUnit, volumeUnit, unitsFound, err := handler.charges.LoadUnitsAt(ctx, tenant, warehouse, dayEnd)
	if err != nil {
		return ChargeResult{}, fmt.Errorf("close storage day: %w", err)
	}
	if !unitsFound || volumeUnit == "" {
		return ChargeResult{outcome: ChargeNotConfigured}, nil
	}
	lines, err := handler.storageLines(ctx, tenant, owner, warehouse, start, dayEnd, location, volumeUnit)
	if err != nil {
		return ChargeResult{}, err
	}
	return handler.Form(ctx, tenant, ChargeDraft{
		Kind: "STORAGE", Source: "storage:" + owner.String() + "/" + warehouse.String() + "/" + start.Format("2006-01-02"),
		Owner: owner, Warehouse: warehouse, Occurred: start, Lines: lines,
		WeightUnit: weightUnit, VolumeUnit: volumeUnit,
		Attributes: []ports.ChargeAttribute{{Name: "AGING_BASIS", Value: basis}},
	})
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
		WeightUnit: target.WeightUnit, VolumeUnit: target.VolumeUnit,
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

// unitsFor 按种类决定要不要重量单位、体积单位。没登记时第一个返回值是 \x00，调用方答未配置。
// 退货入库带明细行，要重量和体积；处置执行那一份不带。加工与增值服务不带。
func (handler *ChargeHandler) unitsFor(ctx context.Context, tenant domain.TenantID, draft ChargeDraft) (string, string, error) {
	needWeight, needVolume := false, false
	switch draft.Kind {
	case "INBOUND_HANDLING", "OUTBOUND_HANDLING", "DESTRUCTION":
		needWeight, needVolume = true, true
	case "STORAGE":
		needVolume = true
	case "RETURN_HANDLING":
		if len(draft.Lines) > 0 {
			needWeight, needVolume = true, true
		}
	}
	weight, volume := draft.WeightUnit, draft.VolumeUnit
	if (needWeight && weight == "") || (needVolume && volume == "") {
		loadedWeight, loadedVolume, found, err := handler.charges.LoadUnits(ctx, tenant, draft.Warehouse)
		if err != nil || !found {
			if !found && err == nil && (needWeight || needVolume) {
				return "\x00", "", nil
			}
			return "", "", err
		}
		if weight == "" {
			weight = loadedWeight
		}
		if volume == "" {
			volume = loadedVolume
		}
	}
	if (needWeight && weight == "") || (needVolume && volume == "") {
		return "\x00", "", nil
	}
	if !needWeight {
		weight = draft.WeightUnit
	}
	if !needVolume {
		volume = draft.VolumeUnit
	}
	return weight, volume, nil
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
