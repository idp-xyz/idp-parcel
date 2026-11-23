package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	warehouseapp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	warehousedomain "go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	warehouseports "go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 本特性的未接线占位是开关打开的那一份：占位登记的是装配测试里「别的登记写行、查阅行怎么答，仓配的行就怎么答」的基线，既有的
// 各条换口、请求体检查与目标检查用例因此不改一条断言就把仓配的行也核到。开关关着的那一格（缺省，生产装配的基线）由本文件下面的
// 用例与 TestProductionAssemblyMountsEveryDeclaredLine 钉。

type unwiredWarehouseRegistration struct{}

func (unwiredWarehouseRegistration) Register(
	context.Context,
	warehouseapp.RegisterWarehouseCommand,
) (warehouseapp.RegisterWarehouseResult, error) {
	return warehouseapp.RegisterWarehouseResult{}, errOrchestrationNotWired
}

type unwiredWarehouseCatalogue struct{}

func (unwiredWarehouseCatalogue) ListWarehouses(
	context.Context,
	warehousedomain.TenantID,
	int,
	cataloguepage.Query,
) (cataloguepage.Result[warehouseports.WarehouseRow], error) {
	return cataloguepage.Result[warehouseports.WarehouseRow]{}, errOrchestrationNotWired
}

func (unwiredWarehouseCatalogue) WarehouseRevisions(
	context.Context,
	warehousedomain.TenantID,
	warehousedomain.WarehouseCode,
) ([]warehouseports.WarehouseRow, error) {
	return nil, errOrchestrationNotWired
}

type unwiredZoneRegistration struct{}

func (unwiredZoneRegistration) Register(
	context.Context,
	warehouseapp.RegisterZoneCommand,
) (warehouseapp.RegisterZoneResult, error) {
	return warehouseapp.RegisterZoneResult{}, errOrchestrationNotWired
}

type unwiredZoneCatalogue struct{}

func (unwiredZoneCatalogue) ListZones(
	context.Context,
	warehousedomain.TenantID,
	int,
	cataloguepage.Query,
) (cataloguepage.Result[warehouseports.ZoneRow], error) {
	return cataloguepage.Result[warehouseports.ZoneRow]{}, errOrchestrationNotWired
}

func (unwiredZoneCatalogue) ZoneRevisions(
	context.Context,
	warehousedomain.TenantID,
	warehousedomain.WarehouseCode,
	warehousedomain.ZoneCode,
) ([]warehouseports.ZoneRow, error) {
	return nil, errOrchestrationNotWired
}

type unwiredLocationTypeRegistration struct{}

func (unwiredLocationTypeRegistration) Register(
	context.Context,
	warehouseapp.RegisterLocationTypeCommand,
) (warehouseapp.RegisterLocationTypeResult, error) {
	return warehouseapp.RegisterLocationTypeResult{}, errOrchestrationNotWired
}

type unwiredLocationTypeCatalogue struct{}

func (unwiredLocationTypeCatalogue) ListLocationTypes(
	context.Context,
	warehousedomain.TenantID,
	int,
	cataloguepage.Query,
) (cataloguepage.Result[warehouseports.LocationTypeRow], error) {
	return cataloguepage.Result[warehouseports.LocationTypeRow]{}, errOrchestrationNotWired
}

func (unwiredLocationTypeCatalogue) LocationTypeRevisions(
	context.Context,
	warehousedomain.TenantID,
	warehousedomain.LocationTypeCode,
) ([]warehouseports.LocationTypeRow, error) {
	return nil, errOrchestrationNotWired
}

type unwiredLocationRegistration struct{}

func (unwiredLocationRegistration) Register(
	context.Context,
	warehouseapp.RegisterLocationCommand,
) (warehouseapp.RegisterLocationResult, error) {
	return warehouseapp.RegisterLocationResult{}, errOrchestrationNotWired
}

type unwiredLocationCatalogue struct{}

func (unwiredLocationCatalogue) ListLocations(
	context.Context,
	warehousedomain.TenantID,
	int,
	cataloguepage.Query,
) (cataloguepage.Result[warehouseports.LocationRow], error) {
	return cataloguepage.Result[warehouseports.LocationRow]{}, errOrchestrationNotWired
}

func (unwiredLocationCatalogue) LocationRevisions(
	context.Context,
	warehousedomain.TenantID,
	warehousedomain.WarehouseCode,
	warehousedomain.LocationCode,
) ([]warehouseports.LocationRow, error) {
	return nil, errOrchestrationNotWired
}

type unwiredPackagingMaterialRegistration struct{}

func (unwiredPackagingMaterialRegistration) Register(
	context.Context,
	warehouseapp.RegisterPackagingMaterialCommand,
) (warehouseapp.RegisterPackagingMaterialResult, error) {
	return warehouseapp.RegisterPackagingMaterialResult{}, errOrchestrationNotWired
}

type unwiredPackagingMaterialCatalogue struct{}

func (unwiredPackagingMaterialCatalogue) ListPackagingMaterials(
	context.Context,
	warehousedomain.TenantID,
	int,
	cataloguepage.Query,
) (cataloguepage.Result[warehouseports.PackagingMaterialRow], error) {
	return cataloguepage.Result[warehouseports.PackagingMaterialRow]{}, errOrchestrationNotWired
}

func (unwiredPackagingMaterialCatalogue) PackagingMaterialRevisions(
	context.Context,
	warehousedomain.TenantID,
	warehousedomain.PackagingMaterialCode,
) ([]warehouseports.PackagingMaterialRow, error) {
	return nil, errOrchestrationNotWired
}

func unwiredWarehouseFulfillment(enabled bool) warehouseFulfillmentWiring {
	return warehouseFulfillmentWiring{
		enabled:          enabled,
		registration:     unwiredWarehouseRegistration{},
		catalogue:        unwiredWarehouseCatalogue{},
		revisions:        unwiredWarehouseCatalogue{},
		zoneRegistration: unwiredZoneRegistration{},
		zoneCatalogue:    unwiredZoneCatalogue{},
		zoneRevisions:    unwiredZoneCatalogue{},

		locationTypeRegistration: unwiredLocationTypeRegistration{},
		locationTypeCatalogue:    unwiredLocationTypeCatalogue{},
		locationTypeRevisions:    unwiredLocationTypeCatalogue{},

		locationRegistration: unwiredLocationRegistration{},
		locationCatalogue:    unwiredLocationCatalogue{},
		locationRevisions:    unwiredLocationCatalogue{},

		packagingMaterialRegistration: unwiredPackagingMaterialRegistration{},
		packagingMaterialCatalogue:    unwiredPackagingMaterialCatalogue{},
		packagingMaterialRevisions:    unwiredPackagingMaterialCatalogue{},

		goodsRegistration: unwiredGoodsRegistration{},
		goodsCatalogue:    unwiredGoodsCatalogue{},
		goodsCurrent:      unwiredGoodsCatalogue{},
		goodsRevisions:    unwiredGoodsCatalogue{},
		goodsFinder:       unwiredGoodsCatalogue{},

		preAdviceOnBehalf:        unwiredPreAdvice{},
		preAdviceQueries:         unwiredPreAdviceQueries{},
		preAdviceClose:           unwiredPreAdviceClose{},
		discrepancyConclusion:    unwiredDiscrepancyConclusion{},
		receiptConfirm:           unwiredReceiptConfirmation{},
		measurementRecord:        unwiredWarehouseMeasurement{},
		measurementQueries:       unwiredWarehouseMeasurement{},
		disposalRuleRegistration: unwiredDisposalRuleRegistration{},
		disposalRuleCatalogue:    unwiredDisposalRuleCatalogue{},
		disposalRuleRevisions:    unwiredDisposalRuleCatalogue{},
		unclaimedArrivalRegister: unwiredUnclaimedArrival{},
		unclaimedArrivalManage:   unwiredUnclaimedArrival{},
		unclaimedArrivalQueries:  unwiredUnclaimedArrival{},
		internalJobs:             unwiredInternalJob{},
		conditionRestore:         unwiredInternalJob{},
		internalJobQueries:       unwiredInternalJob{},
		instructions:             unwiredInstruction{},
		freezes:                  unwiredFreeze{},
		stockQueries:             unwiredStockRead{},
		counts:                   unwiredCount{},
		approvals:                unwiredApproval{},
		adjustments:              unwiredAdjustment{},
		jobDiscrepancies:         unwiredJobDiscrepancy{},
		outbounds:                unwiredOutbound{},
		waves:                    unwiredWave{},
	}
}

type unwiredWave struct{}

func (unwiredWave) Release(context.Context, warehouseapp.ReleaseWaveCommand) (warehouseapp.WaveResult, error) {
	return warehouseapp.WaveResult{}, errOrchestrationNotWired
}
func (unwiredWave) Adopt(context.Context, warehouseapp.AdoptStrategyCommand) (warehouseapp.WaveResult, error) {
	return warehouseapp.WaveResult{}, errOrchestrationNotWired
}
func (unwiredWave) Pick(context.Context, warehouseapp.PickCommand) (warehouseapp.WaveResult, error) {
	return warehouseapp.WaveResult{}, errOrchestrationNotWired
}
func (unwiredWave) Check(context.Context, warehouseapp.CheckCommand) (warehouseapp.WaveResult, error) {
	return warehouseapp.WaveResult{}, errOrchestrationNotWired
}
func (unwiredWave) Pack(context.Context, warehouseapp.PackCommand) (warehouseapp.WaveResult, error) {
	return warehouseapp.WaveResult{}, errOrchestrationNotWired
}
func (unwiredWave) Confirm(context.Context, warehouseapp.ConfirmCommand) (warehouseapp.WaveResult, error) {
	return warehouseapp.WaveResult{}, errOrchestrationNotWired
}
func (unwiredWave) RegisterOwnerLabel(context.Context, warehouseapp.RegisterOwnerLabelCommand) (warehouseapp.WaveResult, error) {
	return warehouseapp.WaveResult{}, errOrchestrationNotWired
}
func (unwiredWave) Correct(context.Context, warehouseapp.CorrectConfirmationCommand) (warehouseapp.WaveResult, error) {
	return warehouseapp.WaveResult{}, errOrchestrationNotWired
}
func (unwiredWave) ApproveReplenishment(context.Context, warehouseapp.ApproveReplenishmentCommand) (warehouseapp.WaveResult, error) {
	return warehouseapp.WaveResult{}, errOrchestrationNotWired
}
func (unwiredWave) Cutoff(context.Context, warehouseapp.CutoffCommand) (warehouseapp.CutoffResult, error) {
	return warehouseapp.CutoffResult{}, errOrchestrationNotWired
}
func (unwiredWave) Return(context.Context, warehouseapp.ReturnCommand) (warehouseapp.WaveResult, error) {
	return warehouseapp.WaveResult{}, errOrchestrationNotWired
}

type unwiredOutbound struct{}

func (unwiredOutbound) Accept(context.Context, warehouseapp.AcceptOutboundCommand) (warehouseapp.OutboundResult, error) {
	return warehouseapp.OutboundResult{}, errOrchestrationNotWired
}
func (unwiredOutbound) RegisterPolicy(context.Context, warehouseapp.RegisterShortagePolicyCommand) (warehouseapp.OutboundResult, error) {
	return warehouseapp.OutboundResult{}, errOrchestrationNotWired
}
func (unwiredOutbound) Split(context.Context, warehouseapp.SplitOutboundCommand) (warehouseapp.SplitResult, error) {
	return warehouseapp.SplitResult{}, errOrchestrationNotWired
}

type unwiredJobDiscrepancy struct{}

func (unwiredJobDiscrepancy) Approve(context.Context, warehouseapp.ApproveCountCommand) (warehouseapp.ApprovalResult, error) {
	return warehouseapp.ApprovalResult{}, errOrchestrationNotWired
}
func (unwiredJobDiscrepancy) Reject(context.Context, warehouseapp.RejectCountCommand) (warehouseapp.ApprovalResult, error) {
	return warehouseapp.ApprovalResult{}, errOrchestrationNotWired
}

type unwiredInstruction struct{}

func (unwiredInstruction) Execute(context.Context, warehouseapp.ExecuteInstructionCommand) (warehouseapp.InstructionResult, error) {
	return warehouseapp.InstructionResult{}, errOrchestrationNotWired
}

func (unwiredInstruction) Unbox(context.Context, warehouseapp.UnboxCommand) (warehouseapp.InstructionResult, error) {
	return warehouseapp.InstructionResult{}, errOrchestrationNotWired
}

func (unwiredInstruction) Accept(context.Context, warehouseapp.AcceptInstructionCommand) (warehouseapp.InstructionResult, error) {
	return warehouseapp.InstructionResult{}, errOrchestrationNotWired
}

func (unwiredInstruction) Withdraw(context.Context, warehousedomain.TenantID, warehousedomain.OwnerAccountID, warehousedomain.InstructionID, warehousedomain.RequestKey, []warehousedomain.InstructionKind) (warehouseapp.InstructionResult, error) {
	return warehouseapp.InstructionResult{}, errOrchestrationNotWired
}

func (unwiredInstruction) Void(context.Context, warehousedomain.TenantID, warehousedomain.InstructionID, warehousedomain.Registrant, string, []warehousedomain.InstructionKind) (warehouseapp.InstructionResult, error) {
	return warehouseapp.InstructionResult{}, errOrchestrationNotWired
}

type unwiredAdjustment struct{}

func (unwiredAdjustment) Submit(context.Context, warehouseapp.SubmitAdjustmentCommand) (warehouseapp.AdjustmentResult, error) {
	return warehouseapp.AdjustmentResult{}, errOrchestrationNotWired
}
func (unwiredAdjustment) Approve(context.Context, warehouseapp.ApproveAdjustmentCommand) (warehouseapp.AdjustmentResult, error) {
	return warehouseapp.AdjustmentResult{}, errOrchestrationNotWired
}
func (unwiredAdjustment) Reject(context.Context, warehouseapp.RejectAdjustmentCommand) (warehouseapp.AdjustmentResult, error) {
	return warehouseapp.AdjustmentResult{}, errOrchestrationNotWired
}

type unwiredApproval struct{}

func (unwiredApproval) Approve(context.Context, warehouseapp.ApproveCountCommand) (warehouseapp.ApprovalResult, error) {
	return warehouseapp.ApprovalResult{}, errOrchestrationNotWired
}
func (unwiredApproval) Reject(context.Context, warehouseapp.RejectCountCommand) (warehouseapp.ApprovalResult, error) {
	return warehouseapp.ApprovalResult{}, errOrchestrationNotWired
}
func (unwiredApproval) RegisterDuty(context.Context, warehouseapp.RegisterApprovalDutyCommand) (warehouseapp.ApprovalResult, error) {
	return warehouseapp.ApprovalResult{}, errOrchestrationNotWired
}

type unwiredCount struct{}

func (unwiredCount) Create(context.Context, warehouseapp.CreateCountCommand) (warehouseapp.CountResult, error) {
	return warehouseapp.CountResult{}, errOrchestrationNotWired
}
func (unwiredCount) Start(context.Context, warehouseapp.CountCommand) (warehouseapp.CountResult, error) {
	return warehouseapp.CountResult{}, errOrchestrationNotWired
}
func (unwiredCount) Record(context.Context, warehouseapp.RecordCountCommand) (warehouseapp.CountResult, error) {
	return warehouseapp.CountResult{}, errOrchestrationNotWired
}
func (unwiredCount) Complete(context.Context, warehouseapp.CountCommand) (warehouseapp.CountResult, error) {
	return warehouseapp.CountResult{}, errOrchestrationNotWired
}
func (unwiredCount) Void(context.Context, warehouseapp.CountCommand) (warehouseapp.CountResult, error) {
	return warehouseapp.CountResult{}, errOrchestrationNotWired
}

type unwiredStockRead struct{}

func (unwiredStockRead) Query(context.Context, warehouseapp.StockQuery) (warehouseapp.StockSnapshot, error) {
	return warehouseapp.StockSnapshot{}, errOrchestrationNotWired
}

type unwiredFreeze struct{}

func (unwiredFreeze) Freeze(context.Context, warehouseapp.FreezeCommand) (warehouseapp.FreezeResult, error) {
	return warehouseapp.FreezeResult{}, errOrchestrationNotWired
}

func (unwiredFreeze) Unfreeze(context.Context, warehouseapp.UnfreezeCommand) (warehouseapp.FreezeResult, error) {
	return warehouseapp.FreezeResult{}, errOrchestrationNotWired
}

type unwiredInternalJob struct{}

func (unwiredInternalJob) Register(context.Context, warehouseapp.RegisterInternalJobCommand) (warehouseapp.RegisterInternalJobResult, error) {
	return warehouseapp.RegisterInternalJobResult{}, errOrchestrationNotWired
}

func (unwiredInternalJob) Restore(context.Context, warehouseapp.RestoreConditionCommand) (warehouseapp.RestoreConditionResult, error) {
	return warehouseapp.RestoreConditionResult{}, errOrchestrationNotWired
}

func (unwiredInternalJob) ListInternalJobs(context.Context, warehousedomain.TenantID, int, cataloguepage.Query) (cataloguepage.Result[warehousedomain.RecordedInternalJob], error) {
	return cataloguepage.Result[warehousedomain.RecordedInternalJob]{}, errOrchestrationNotWired
}

func (unwiredInternalJob) GetInternalJob(context.Context, warehousedomain.TenantID, warehousedomain.InternalJobFactID) (warehousedomain.RecordedInternalJob, bool, error) {
	return warehousedomain.RecordedInternalJob{}, false, errOrchestrationNotWired
}

func (unwiredInternalJob) GetConditionRestore(context.Context, warehousedomain.TenantID, warehousedomain.ConditionRestoreID) (warehousedomain.ConditionRestore, bool, error) {
	return warehousedomain.ConditionRestore{}, false, errOrchestrationNotWired
}

type unwiredDisposalRuleRegistration struct{}

func (unwiredDisposalRuleRegistration) Register(
	context.Context,
	warehouseapp.RegisterDisposalRuleCommand,
) (warehouseapp.RegisterDisposalRuleResult, error) {
	return warehouseapp.RegisterDisposalRuleResult{}, errOrchestrationNotWired
}

type unwiredDisposalRuleCatalogue struct{}

func (unwiredDisposalRuleCatalogue) ListDisposalRules(
	context.Context,
	warehousedomain.TenantID,
	int,
	cataloguepage.Query,
) (cataloguepage.Result[warehouseports.DisposalRuleRow], error) {
	return cataloguepage.Result[warehouseports.DisposalRuleRow]{}, errOrchestrationNotWired
}

func (unwiredDisposalRuleCatalogue) DisposalRuleRevisions(
	context.Context,
	warehousedomain.TenantID,
	warehousedomain.WarehouseCode,
) ([]warehouseports.DisposalRuleRow, error) {
	return nil, errOrchestrationNotWired
}

type unwiredUnclaimedArrival struct{}

func (unwiredUnclaimedArrival) Register(
	context.Context,
	warehouseapp.RegisterUnclaimedArrivalCommand,
) (warehouseapp.RegisterUnclaimedArrivalResult, error) {
	return warehouseapp.RegisterUnclaimedArrivalResult{}, errOrchestrationNotWired
}

func (unwiredUnclaimedArrival) Claim(
	context.Context,
	warehouseapp.ClaimUnclaimedArrivalCommand,
) (warehouseapp.ClaimUnclaimedArrivalResult, error) {
	return warehouseapp.ClaimUnclaimedArrivalResult{}, errOrchestrationNotWired
}

func (unwiredUnclaimedArrival) RevokeClaim(
	context.Context,
	warehouseapp.RevokeUnclaimedClaimCommand,
) (warehouseapp.RevokeUnclaimedClaimResult, error) {
	return warehouseapp.RevokeUnclaimedClaimResult{}, errOrchestrationNotWired
}

func (unwiredUnclaimedArrival) Dispose(
	context.Context,
	warehouseapp.DisposeUnclaimedArrivalCommand,
) (warehouseapp.DisposeUnclaimedArrivalResult, error) {
	return warehouseapp.DisposeUnclaimedArrivalResult{}, errOrchestrationNotWired
}

func (unwiredUnclaimedArrival) ListUnclaimedArrivals(
	context.Context,
	warehousedomain.TenantID,
	int,
	cataloguepage.Query,
	time.Time,
	time.Time,
) (cataloguepage.Result[warehousedomain.UnclaimedArrivalDetail], error) {
	return cataloguepage.Result[warehousedomain.UnclaimedArrivalDetail]{}, errOrchestrationNotWired
}

func (unwiredUnclaimedArrival) GetUnclaimedArrival(
	context.Context,
	warehousedomain.TenantID,
	warehousedomain.UnclaimedArrivalID,
) (warehousedomain.UnclaimedArrivalDetail, bool, error) {
	return warehousedomain.UnclaimedArrivalDetail{}, false, errOrchestrationNotWired
}

type unwiredWarehouseMeasurement struct{}

func (unwiredWarehouseMeasurement) Record(
	context.Context,
	warehouseapp.RecordWarehouseMeasurementCommand,
) (warehouseapp.RecordWarehouseMeasurementResult, error) {
	return warehouseapp.RecordWarehouseMeasurementResult{}, errOrchestrationNotWired
}

func (unwiredWarehouseMeasurement) GoodsLevelMeasurements(
	context.Context,
	warehousedomain.TenantID,
	warehousedomain.OwnerAccountID,
	warehousedomain.GoodsCode,
	warehousedomain.GoodsUnitName,
) (warehouseapp.GoodsLevelMeasurementReading, error) {
	return warehouseapp.GoodsLevelMeasurementReading{}, errOrchestrationNotWired
}

func (unwiredWarehouseMeasurement) CartonMeasurements(
	context.Context,
	warehousedomain.TenantID,
	warehousedomain.OwnerAccountID,
	warehousedomain.WarehouseCode,
	warehousedomain.CartonNumber,
) (warehouseapp.CartonMeasurementReading, error) {
	return warehouseapp.CartonMeasurementReading{}, errOrchestrationNotWired
}

type unwiredDiscrepancyConclusion struct{}

func (unwiredDiscrepancyConclusion) Conclude(
	context.Context,
	warehouseapp.ConcludeDiscrepancyCommand,
) (warehouseapp.ConcludeDiscrepancyResult, error) {
	return warehouseapp.ConcludeDiscrepancyResult{}, errOrchestrationNotWired
}

type unwiredReceiptConfirmation struct{}

func (unwiredReceiptConfirmation) Confirm(
	context.Context,
	warehouseapp.ConfirmReceiptCommand,
) (warehouseapp.ConfirmReceiptResult, error) {
	return warehouseapp.ConfirmReceiptResult{}, errOrchestrationNotWired
}

type unwiredPreAdviceClose struct{}

func (unwiredPreAdviceClose) ClosePreAdvice(
	context.Context,
	warehouseapp.ClosePreAdviceCommand,
) (warehouseapp.ClosePreAdviceResult, error) {
	return warehouseapp.ClosePreAdviceResult{}, errOrchestrationNotWired
}

type unwiredPreAdvice struct{}

func (unwiredPreAdvice) Accept(context.Context, warehouseapp.AcceptPreAdviceCommand) (warehouseapp.AcceptPreAdviceResult, error) {
	return warehouseapp.AcceptPreAdviceResult{}, errOrchestrationNotWired
}

func (unwiredPreAdvice) UpdateNotice(context.Context, warehouseapp.UpdatePreAdviceNoticeCommand) (warehouseapp.AcceptPreAdviceResult, error) {
	return warehouseapp.AcceptPreAdviceResult{}, errOrchestrationNotWired
}

func (unwiredPreAdvice) Withdraw(context.Context, warehouseapp.WithdrawPreAdviceCommand) (warehouseapp.AcceptPreAdviceResult, error) {
	return warehouseapp.AcceptPreAdviceResult{}, errOrchestrationNotWired
}

type unwiredPreAdviceQueries struct{}

func (unwiredPreAdviceQueries) ListPreAdvices(
	context.Context,
	warehousedomain.TenantID,
	warehousedomain.OwnerAccountID,
	int,
	cataloguepage.Query,
) (cataloguepage.Result[warehouseapp.PreAdviceReading], error) {
	return cataloguepage.Result[warehouseapp.PreAdviceReading]{}, errOrchestrationNotWired
}

func (unwiredPreAdviceQueries) PreAdviceForOwner(
	context.Context,
	warehousedomain.TenantID,
	warehousedomain.OwnerAccountID,
	warehousedomain.PreAdviceID,
) (warehouseapp.PreAdviceReading, bool, error) {
	return warehouseapp.PreAdviceReading{}, false, errOrchestrationNotWired
}

func (unwiredPreAdviceQueries) PreAdviceByDocumentNumber(
	context.Context,
	warehousedomain.TenantID,
	warehousedomain.DocumentNumber,
) (warehouseapp.PreAdviceReading, bool, error) {
	return warehouseapp.PreAdviceReading{}, false, errOrchestrationNotWired
}

type unwiredGoodsRegistration struct{}

func (unwiredGoodsRegistration) Register(
	context.Context,
	warehouseapp.RegisterGoodsCommand,
) (warehouseapp.RegisterGoodsResult, error) {
	return warehouseapp.RegisterGoodsResult{}, errOrchestrationNotWired
}

type unwiredGoodsCatalogue struct{}

func (unwiredGoodsCatalogue) ListGoods(
	context.Context,
	warehousedomain.TenantID,
	warehousedomain.OwnerAccountID,
	int,
	cataloguepage.Query,
) (cataloguepage.Result[warehouseports.GoodsRow], error) {
	return cataloguepage.Result[warehouseports.GoodsRow]{}, errOrchestrationNotWired
}

func (unwiredGoodsCatalogue) CurrentGoods(
	context.Context,
	warehousedomain.TenantID,
	warehousedomain.OwnerAccountID,
	warehousedomain.GoodsCode,
) (warehouseports.GoodsRow, bool, error) {
	return warehouseports.GoodsRow{}, false, errOrchestrationNotWired
}

func (unwiredGoodsCatalogue) GoodsRevisions(
	context.Context,
	warehousedomain.TenantID,
	warehousedomain.OwnerAccountID,
	warehousedomain.GoodsCode,
) ([]warehouseports.GoodsRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredGoodsCatalogue) FindGoods(
	context.Context,
	warehousedomain.TenantID,
	warehousedomain.OwnerAccountID,
	warehousedomain.IdentifierCode,
) (warehouseports.GoodsRow, bool, error) {
	return warehouseports.GoodsRow{}, false, errOrchestrationNotWired
}

func init() {
	registerUnwired(unwiredWarehouseFulfillment(true))
}

const (
	warehouseRegistrationsLine = "/warehouse-fulfillment-warehouse-registrations"
	warehouseListLine          = "/warehouse-fulfillment-warehouses"
	warehouseRevisionsLine     = "/warehouse-fulfillment-warehouse-revisions"
	zoneRegistrationsLine      = "/warehouse-fulfillment-zone-registrations"
	zoneListLine               = "/warehouse-fulfillment-zones"
	zoneRevisionsLine          = "/warehouse-fulfillment-zone-revisions"

	locationTypeRegistrationsLine = "/warehouse-fulfillment-location-type-registrations"
	locationTypeListLine          = "/warehouse-fulfillment-location-types"
	locationTypeRevisionsLine     = "/warehouse-fulfillment-location-type-revisions"

	locationRegistrationsLine = "/warehouse-fulfillment-location-registrations"
	locationListLine          = "/warehouse-fulfillment-locations"
	locationRevisionsLine     = "/warehouse-fulfillment-location-revisions"

	packagingMaterialRegistrationsLine = "/warehouse-fulfillment-packaging-material-registrations"
	packagingMaterialListLine          = "/warehouse-fulfillment-packaging-materials"
	packagingMaterialRevisionsLine     = "/warehouse-fulfillment-packaging-material-revisions"

	goodsOnBehalfLine  = "/warehouse-fulfillment-goods-on-behalf"
	goodsListLine      = "/warehouse-fulfillment-goods"
	goodsCurrentLine   = "/warehouse-fulfillment-goods-current"
	goodsRevisionsLine = "/warehouse-fulfillment-goods-revisions"
	goodsByCodeLine    = "/warehouse-fulfillment-goods-by-code"

	preAdviceOnBehalfLine   = "/warehouse-fulfillment-inbound-pre-advice-on-behalf"
	preAdviceNoticeLine     = "/warehouse-fulfillment-inbound-pre-advice-notice-on-behalf"
	preAdviceWithdrawalLine = "/warehouse-fulfillment-inbound-pre-advice-withdrawal-on-behalf"
	preAdviceCloseLine      = "/warehouse-fulfillment-inbound-pre-advice-closures"
	preAdviceListLine       = "/warehouse-fulfillment-inbound-pre-advices"
	preAdviceOneLine        = "/warehouse-fulfillment-inbound-pre-advice"
	preAdviceByNumberLine   = "/warehouse-fulfillment-inbound-pre-advice-by-number"

	receiptConfirmationsLine = "/warehouse-fulfillment-receipt-confirmations"

	warehouseMeasurementsLine = "/warehouse-fulfillment-warehouse-measurements"
	goodsMeasurementsLine     = "/warehouse-fulfillment-goods-measurements"
	cartonMeasurementsLine    = "/warehouse-fulfillment-carton-measurements"

	disposalRuleRegistrationsLine = "/warehouse-fulfillment-unclaimed-disposal-rule-registrations"
	disposalRuleListLine          = "/warehouse-fulfillment-unclaimed-disposal-rules"
	disposalRuleRevisionsLine     = "/warehouse-fulfillment-unclaimed-disposal-rule-revisions"
	unclaimedArrivalRegLine       = "/warehouse-fulfillment-unclaimed-arrival-registrations"
	unclaimedArrivalClaimLine     = "/warehouse-fulfillment-unclaimed-arrival-claims"
	unclaimedArrivalRevokeLine    = "/warehouse-fulfillment-unclaimed-arrival-claim-revocations"
	unclaimedArrivalDisposeLine   = "/warehouse-fulfillment-unclaimed-arrival-dispositions"
	unclaimedArrivalListLine      = "/warehouse-fulfillment-unclaimed-arrivals"
	unclaimedArrivalOneLine       = "/warehouse-fulfillment-unclaimed-arrival"

	internalMovesLine           = "/warehouse-fulfillment-internal-moves"
	formConversionsLine         = "/warehouse-fulfillment-form-conversions"
	conditionConversionsLine    = "/warehouse-fulfillment-condition-conversions"
	shortageRegistrationsLine   = "/warehouse-fulfillment-shortage-registrations"
	processingExecutionsLine    = "/warehouse-fulfillment-processing-executions"
	relabelExecutionsLine       = "/warehouse-fulfillment-relabel-executions"
	unboxingsLine               = "/warehouse-fulfillment-unboxings"
	countEntriesLine            = "/warehouse-fulfillment-count-entries"
	picksLine                   = "/warehouse-fulfillment-picks"
	outboundChecksLine          = "/warehouse-fulfillment-outbound-checks"
	packsLine                   = "/warehouse-fulfillment-packs"
	returnsLine                 = "/warehouse-fulfillment-returns"
	returnReceiptsLine          = "/warehouse-fulfillment-return-receipts"
	outboundConfirmationsLine   = "/warehouse-fulfillment-outbound-confirmations"
	pickupHandoversLine         = "/warehouse-fulfillment-pickup-handovers"
	ownerLabelsLine             = "/warehouse-fulfillment-owner-labels"
	confirmationCorrectionsLine = "/warehouse-fulfillment-outbound-confirmation-corrections"
	replenishmentsLine          = "/warehouse-fulfillment-outbound-confirmation-replenishments"
	conditionRestoresLine       = "/warehouse-fulfillment-condition-restores"
	internalJobsLine            = "/warehouse-fulfillment-internal-jobs"
	internalJobLine             = "/warehouse-fulfillment-internal-job"
	conditionRestoreReadLine    = "/warehouse-fulfillment-condition-restore"

	customerGoodsWriteLine         = "/v1/warehouse/goods"
	customerGoodsListLine          = "/v1/warehouse/reads/goods"
	customerGoodsCurrentLine       = "/v1/warehouse/reads/goods-current"
	customerGoodsRevisionsLine     = "/v1/warehouse/reads/goods-revisions"
	customerGoodsByCodeLine        = "/v1/warehouse/reads/goods-by-code"
	customerGoodsMeasurementsLine  = "/v1/warehouse/reads/goods-measurements"
	customerCartonMeasurementsLine = "/v1/warehouse/reads/carton-measurements"

	discrepancyConclusionLine         = "/warehouse-fulfillment-receipt-discrepancy-conclusion-on-behalf"
	customerDiscrepancyConclusionLine = "/v1/warehouse/receipt-discrepancy-conclusions"
)

func warehouseWorkLine(pattern string) bool {
	return pattern == countEntriesLine || pattern == picksLine || pattern == outboundChecksLine || pattern == packsLine || pattern == returnsLine || pattern == returnReceiptsLine || pattern == outboundConfirmationsLine || pattern == pickupHandoversLine
}

// warehouseFulfillmentLines 是登记处里声明了仓配开关的各行，按路径排序。
func warehouseFulfillmentLines() []string {
	return declaredPatterns(func(line endpointLine) bool { return line.warehouseFulfillment })
}

// channelsFor 同时打开操作者的登记写面、查阅读与运营决定：商品代录认的是决定授予，各读口认的是查阅读。
func channelsFor(t *testing.T, register accessidentity.OperatorRegistry) endpointChannels {
	t.Helper()
	minter, err := buildOperatorMinter(acceptingVerifier{}, register, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	registries, err := buildOperatorRegistryIntakes(minter)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	return unwiredChannels(decisions, registries, unconfiguredIntegrationClientIntakes(), nil, nil, nil, nil)
}

// warehouseFulfillmentRouter 只挂仓配特性的各行（经登记处的 mountWith，请求体与目标检查照挂），接入渠道与开关由调用方给。
func warehouseFulfillmentRouter(t *testing.T, channels endpointChannels, wiring warehouseFulfillmentWiring) http.Handler {
	t.Helper()
	key := any((*warehouseFulfillmentWiring)(nil))
	for _, feature := range endpointRegistry {
		if feature.wiringKey == key {
			return httpapi.NewWithEndpoints(buildinfo.Info{}, feature.mountWith(channels, wiring))
		}
	}
	t.Fatal("登记处没有仓配履约这个特性")
	return nil
}

// probeWarehouseFulfillment 打仓配的一行：令牌可缺，请求体取 body（GET 行不带）。
func probeWarehouseFulfillment(router http.Handler, pattern, token, body string) *httptest.ResponseRecorder {
	probe := businessEndpointProbes()[pattern]
	var request *http.Request
	if probe.method == http.MethodGet {
		request = httptest.NewRequest(probe.method, probe.target, nil)
	} else {
		request = httptest.NewRequest(probe.method, probe.target, strings.NewReader(body))
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func leftUnconfigured(t *testing.T, response *httptest.ResponseRecorder) bool {
	t.Helper()
	return response.Code == http.StatusForbidden && problemCode(t, response) == "ACCESS_CHANNEL_NOT_CONFIGURED"
}

// Covers: AT-WF-010、AT-WF-012——登记处里仓配的行恰是声明了开关的那几行，接入面是登记册配置写（登记口）与主数据与运营查阅读
// （各读口）：声明错了，换口与闭合用例会红，这一条先点名。
func TestTheWarehouseFulfillmentLinesDeclareTheirFacesAndTheSwitch(t *testing.T) {
	want := map[string]struct {
		method string
		face   faceFamily
	}{
		warehouseRegistrationsLine: {http.MethodPost, faceRegistryWrite},
		warehouseListLine:          {http.MethodGet, faceOperationsRead},
		warehouseRevisionsLine:     {http.MethodGet, faceOperationsRead},
		zoneRegistrationsLine:      {http.MethodPost, faceRegistryWrite},
		zoneListLine:               {http.MethodGet, faceOperationsRead},
		zoneRevisionsLine:          {http.MethodGet, faceOperationsRead},

		locationTypeRegistrationsLine: {http.MethodPost, faceRegistryWrite},
		locationTypeListLine:          {http.MethodGet, faceOperationsRead},
		locationTypeRevisionsLine:     {http.MethodGet, faceOperationsRead},

		locationRegistrationsLine: {http.MethodPost, faceRegistryWrite},
		locationListLine:          {http.MethodGet, faceOperationsRead},
		locationRevisionsLine:     {http.MethodGet, faceOperationsRead},

		packagingMaterialRegistrationsLine: {http.MethodPost, faceRegistryWrite},
		packagingMaterialListLine:          {http.MethodGet, faceOperationsRead},
		packagingMaterialRevisionsLine:     {http.MethodGet, faceOperationsRead},

		goodsOnBehalfLine:  {http.MethodPost, faceOperationDecision},
		goodsListLine:      {http.MethodGet, faceOperationsRead},
		goodsCurrentLine:   {http.MethodGet, faceOperationsRead},
		goodsRevisionsLine: {http.MethodGet, faceOperationsRead},
		goodsByCodeLine:    {http.MethodGet, faceOperationsRead},

		preAdviceOnBehalfLine:   {http.MethodPost, faceOperationDecision},
		preAdviceNoticeLine:     {http.MethodPost, faceOperationDecision},
		preAdviceWithdrawalLine: {http.MethodPost, faceOperationDecision},
		preAdviceCloseLine:      {http.MethodPost, faceOperationDecision},
		preAdviceListLine:       {http.MethodGet, faceOperationsRead},
		preAdviceOneLine:        {http.MethodGet, faceOperationsRead},
		preAdviceByNumberLine:   {http.MethodGet, faceOperationsRead},

		"/v1/warehouse/inbound-pre-advices":                 {http.MethodPost, faceCustomerClient},
		"/v1/warehouse/inbound-pre-advices/arrival-notices": {http.MethodPost, faceCustomerClient},
		"/v1/warehouse/inbound-pre-advices/withdrawals":     {http.MethodPost, faceCustomerClient},
		"/v1/warehouse/reads/inbound-pre-advices":           {http.MethodGet, faceCustomerClient},
		"/v1/warehouse/reads/inbound-pre-advice":            {http.MethodGet, faceCustomerClient},
		"/v1/warehouse/reads/inbound-pre-advice-by-number":  {http.MethodGet, faceCustomerClient},

		discrepancyConclusionLine:                           {http.MethodPost, faceOperationDecision},
		customerDiscrepancyConclusionLine:                   {http.MethodPost, faceCustomerClient},
		"/v1/warehouse/processing-instructions":             {http.MethodPost, faceCustomerClient},
		"/v1/warehouse/processing-instructions/withdrawals": {http.MethodPost, faceCustomerClient},
		"/v1/warehouse/relabel-instructions":                {http.MethodPost, faceCustomerClient},
		"/v1/warehouse/relabel-instructions/withdrawals":    {http.MethodPost, faceCustomerClient},
		"/v1/warehouse/outbound-requests":                   {http.MethodPost, faceCustomerClient},
		"/v1/warehouse/outbound-requests/splits":            {http.MethodPost, faceCustomerClient},
		"/v1/warehouse/outbound-requests/supplements":       {http.MethodPost, faceCustomerClient},
		"/v1/warehouse/outbound-requests/cancellations":     {http.MethodPost, faceCustomerClient},
		"/v1/warehouse/return-pre-advices":                  {http.MethodPost, faceCustomerClient},
		"/v1/warehouse/return-dispositions":                 {http.MethodPost, faceCustomerClient},

		receiptConfirmationsLine:  {http.MethodPost, faceWorkFact},
		warehouseMeasurementsLine: {http.MethodPost, faceWorkFact},
		goodsMeasurementsLine:     {http.MethodGet, faceOperationsRead},
		cartonMeasurementsLine:    {http.MethodGet, faceOperationsRead},

		disposalRuleRegistrationsLine:                                 {http.MethodPost, faceRegistryWrite},
		disposalRuleListLine:                                          {http.MethodGet, faceOperationsRead},
		disposalRuleRevisionsLine:                                     {http.MethodGet, faceOperationsRead},
		unclaimedArrivalRegLine:                                       {http.MethodPost, faceWorkFact},
		unclaimedArrivalClaimLine:                                     {http.MethodPost, faceOperationDecision},
		unclaimedArrivalRevokeLine:                                    {http.MethodPost, faceOperationDecision},
		unclaimedArrivalDisposeLine:                                   {http.MethodPost, faceOperationDecision},
		unclaimedArrivalListLine:                                      {http.MethodGet, faceOperationsRead},
		unclaimedArrivalOneLine:                                       {http.MethodGet, faceOperationsRead},
		internalMovesLine:                                             {http.MethodPost, faceWorkFact},
		formConversionsLine:                                           {http.MethodPost, faceWorkFact},
		conditionConversionsLine:                                      {http.MethodPost, faceWorkFact},
		shortageRegistrationsLine:                                     {http.MethodPost, faceWorkFact},
		processingExecutionsLine:                                      {http.MethodPost, faceWorkFact},
		relabelExecutionsLine:                                         {http.MethodPost, faceWorkFact},
		unboxingsLine:                                                 {http.MethodPost, faceWorkFact},
		conditionRestoresLine:                                         {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-processing-instructions":              {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-processing-instruction-voids":         {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-relabel-instructions":                 {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-relabel-instruction-voids":            {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-stock-freezes":                        {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-stock-unfreezes":                      {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-stock-counts":                         {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-stock-count-starts":                   {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-stock-count-completions":              {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-stock-count-voids":                    {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-count-entries":                        {http.MethodPost, faceWorkFact},
		"/warehouse-fulfillment-picks":                                {http.MethodPost, faceWorkFact},
		"/warehouse-fulfillment-outbound-checks":                      {http.MethodPost, faceWorkFact},
		"/warehouse-fulfillment-packs":                                {http.MethodPost, faceWorkFact},
		"/warehouse-fulfillment-returns":                              {http.MethodPost, faceWorkFact},
		"/warehouse-fulfillment-return-receipts":                      {http.MethodPost, faceWorkFact},
		"/warehouse-fulfillment-outbound-confirmations":               {http.MethodPost, faceWorkFact},
		"/warehouse-fulfillment-pickup-handovers":                     {http.MethodPost, faceWorkFact},
		"/warehouse-fulfillment-owner-labels":                         {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-outbound-confirmation-corrections":    {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-outbound-confirmation-replenishments": {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-approval-duty-rules":                  {http.MethodPost, faceRegistryWrite},
		"/warehouse-fulfillment-count-approvals":                      {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-count-rejections":                     {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-stock-adjustments":                    {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-stock-adjustment-approvals":           {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-stock-adjustment-rejections":          {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-job-discrepancy-approvals":            {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-job-discrepancy-rejections":           {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-outbound-requests":                    {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-outbound-request-splits":              {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-outbound-cancellations":               {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-return-pre-advices":                   {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-return-dispositions":                  {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-shortage-policies":                    {http.MethodPost, faceRegistryWrite},
		"/warehouse-fulfillment-agent-connectors":                     {http.MethodPost, faceRegistryWrite},
		"/warehouse-fulfillment-agent-stock-statements":               {http.MethodPost, faceRegistryWrite},
		"/warehouse-fulfillment-agent-outbound-results":               {http.MethodPost, faceRegistryWrite},
		"/warehouse-fulfillment-charge-measure-units":                 {http.MethodPost, faceRegistryWrite},
		"/warehouse-fulfillment-charge-adoptions":                     {http.MethodPost, faceRegistryWrite},
		"/warehouse-fulfillment-charge-aging-bases":                   {http.MethodPost, faceRegistryWrite},
		"/warehouse-fulfillment-charge-service-projects":              {http.MethodPost, faceRegistryWrite},
		"/warehouse-fulfillment-storage-day-closes":                   {http.MethodPost, faceRegistryWrite},
		"/warehouse-fulfillment-charge-occurrence-voids":              {http.MethodPost, faceRegistryWrite},
		"/warehouse-fulfillment-charge-occurrence-restates":           {http.MethodPost, faceRegistryWrite},
		"/warehouse-fulfillment-allocation-strategy-adoptions":        {http.MethodPost, faceRegistryWrite},
		"/warehouse-fulfillment-waves":                                {http.MethodPost, faceOperationDecision},
		"/warehouse-fulfillment-stock":                                {http.MethodGet, faceOperationsRead},
		"/v1/warehouse/reads/stock":                                   {http.MethodGet, faceCustomerClient},
		internalJobsLine:                                              {http.MethodGet, faceOperationsRead},
		internalJobLine:                                               {http.MethodGet, faceOperationsRead},
		conditionRestoreReadLine:                                      {http.MethodGet, faceOperationsRead},

		customerGoodsWriteLine:         {http.MethodPost, faceCustomerClient},
		customerGoodsListLine:          {http.MethodGet, faceCustomerClient},
		customerGoodsCurrentLine:       {http.MethodGet, faceCustomerClient},
		customerGoodsRevisionsLine:     {http.MethodGet, faceCustomerClient},
		customerGoodsByCodeLine:        {http.MethodGet, faceCustomerClient},
		customerGoodsMeasurementsLine:  {http.MethodGet, faceCustomerClient},
		customerCartonMeasurementsLine: {http.MethodGet, faceCustomerClient},
	}
	lines := declaredEndpointLines()
	if got := warehouseFulfillmentLines(); len(got) != len(want) {
		t.Fatalf("声明了仓配开关的行 = %v，want 恰是 %d 行：%v", got, len(want), want)
	}
	customerCapabilities := map[string]accessidentity.CustomerCapability{
		"/v1/warehouse/inbound-pre-advices":                 accessidentity.CustomerCapabilityInboundPreAdviceSubmission,
		"/v1/warehouse/inbound-pre-advices/arrival-notices": accessidentity.CustomerCapabilityInboundPreAdviceArrivalNotice,
		"/v1/warehouse/inbound-pre-advices/withdrawals":     accessidentity.CustomerCapabilityInboundPreAdviceWithdrawal,
		"/v1/warehouse/reads/inbound-pre-advices":           accessidentity.CustomerCapabilityWarehouseRead,
		"/v1/warehouse/reads/inbound-pre-advice":            accessidentity.CustomerCapabilityWarehouseRead,
		"/v1/warehouse/reads/inbound-pre-advice-by-number":  accessidentity.CustomerCapabilityWarehouseRead,
		customerDiscrepancyConclusionLine:                   accessidentity.CustomerCapabilityReceiptDiscrepancyConclusion,
		"/v1/warehouse/processing-instructions":             accessidentity.CustomerCapabilityProcessingInstructionSubmission,
		"/v1/warehouse/processing-instructions/withdrawals": accessidentity.CustomerCapabilityProcessingInstructionWithdrawal,
		"/v1/warehouse/relabel-instructions":                accessidentity.CustomerCapabilityRelabelInstructionSubmission,
		"/v1/warehouse/relabel-instructions/withdrawals":    accessidentity.CustomerCapabilityRelabelInstructionWithdrawal,
		"/v1/warehouse/outbound-requests":                   accessidentity.CustomerCapabilityOutboundRequestSubmission,
		"/v1/warehouse/outbound-requests/splits":            accessidentity.CustomerCapabilityOutboundRequestSubmission,
		"/v1/warehouse/outbound-requests/supplements":       accessidentity.CustomerCapabilityOutboundRequestSubmission,
		"/v1/warehouse/outbound-requests/cancellations":     accessidentity.CustomerCapabilityOutboundCancellation,
		"/v1/warehouse/return-pre-advices":                  accessidentity.CustomerCapabilityReturnPreAdviceSubmission,
		"/v1/warehouse/return-dispositions":                 accessidentity.CustomerCapabilityReturnDisposition,
		"/v1/warehouse/reads/stock":                         accessidentity.CustomerCapabilityWarehouseRead,
		customerGoodsWriteLine:                              accessidentity.CustomerCapabilityGoodsMaintenance,
		customerGoodsListLine:                               accessidentity.CustomerCapabilityWarehouseRead,
		customerGoodsCurrentLine:                            accessidentity.CustomerCapabilityWarehouseRead,
		customerGoodsRevisionsLine:                          accessidentity.CustomerCapabilityWarehouseRead,
		customerGoodsByCodeLine:                             accessidentity.CustomerCapabilityWarehouseRead,
		customerGoodsMeasurementsLine:                       accessidentity.CustomerCapabilityWarehouseRead,
		customerCartonMeasurementsLine:                      accessidentity.CustomerCapabilityWarehouseRead,
	}
	for pattern, expected := range want {
		line, declared := lines[pattern]
		capability, customer := customerCapabilities[pattern]
		ownIntake := customer || pattern == receiptConfirmationsLine || pattern == warehouseMeasurementsLine || pattern == unclaimedArrivalRegLine ||
			pattern == internalMovesLine || pattern == formConversionsLine || pattern == conditionConversionsLine || pattern == shortageRegistrationsLine ||
			pattern == processingExecutionsLine || pattern == relabelExecutionsLine || pattern == unboxingsLine || warehouseWorkLine(pattern)
		if !declared || !line.warehouseFulfillment || line.method != expected.method || line.face.family != expected.face || line.ownIntake != ownIntake {
			t.Fatalf("%s 的声明 = %+v，want %s %s、声明仓配开关、自建 Intake=%t", pattern, line, expected.method, expected.face, ownIntake)
		}
		if (pattern == receiptConfirmationsLine || pattern == warehouseMeasurementsLine || pattern == unclaimedArrivalRegLine ||
			pattern == internalMovesLine || pattern == formConversionsLine || pattern == conditionConversionsLine || pattern == shortageRegistrationsLine ||
			pattern == processingExecutionsLine || pattern == relabelExecutionsLine || pattern == unboxingsLine || warehouseWorkLine(pattern)) && line.face.workScope != accessidentity.WorkScopeWarehouse {
			t.Fatalf("%s 的作业范围 = %s，want WAREHOUSE", pattern, line.face.workScope)
		}
		if customer && line.face.capability != capability {
			t.Fatalf("%s 的能力词 = %s，want %s", pattern, line.face.capability, capability)
		}
		decisionLines := map[string]accessidentity.DecisionKind{
			goodsOnBehalfLine:                                             accessidentity.DecisionGoodsRecordOnBehalf,
			preAdviceOnBehalfLine:                                         accessidentity.DecisionInboundPreAdviceRecordOnBehalf,
			preAdviceNoticeLine:                                           accessidentity.DecisionInboundPreAdviceRecordOnBehalf,
			preAdviceWithdrawalLine:                                       accessidentity.DecisionInboundPreAdviceRecordOnBehalf,
			preAdviceCloseLine:                                            accessidentity.DecisionInboundPreAdviceClose,
			discrepancyConclusionLine:                                     accessidentity.DecisionReceiptDiscrepancyRecordOnBehalf,
			unclaimedArrivalClaimLine:                                     accessidentity.DecisionUnclaimedArrivalClaim,
			unclaimedArrivalRevokeLine:                                    accessidentity.DecisionUnclaimedArrivalClaim,
			unclaimedArrivalDisposeLine:                                   accessidentity.DecisionUnclaimedArrivalDisposition,
			conditionRestoresLine:                                         accessidentity.DecisionConditionRestore,
			"/warehouse-fulfillment-processing-instructions":              accessidentity.DecisionProcessingInstruction,
			"/warehouse-fulfillment-processing-instruction-voids":         accessidentity.DecisionProcessingInstruction,
			"/warehouse-fulfillment-relabel-instructions":                 accessidentity.DecisionRelabelInstruction,
			"/warehouse-fulfillment-relabel-instruction-voids":            accessidentity.DecisionRelabelInstruction,
			"/warehouse-fulfillment-stock-freezes":                        accessidentity.DecisionStockFreeze,
			"/warehouse-fulfillment-stock-unfreezes":                      accessidentity.DecisionStockFreeze,
			"/warehouse-fulfillment-stock-counts":                         accessidentity.DecisionStockCount,
			"/warehouse-fulfillment-stock-count-starts":                   accessidentity.DecisionStockCount,
			"/warehouse-fulfillment-stock-count-completions":              accessidentity.DecisionStockCount,
			"/warehouse-fulfillment-stock-count-voids":                    accessidentity.DecisionStockCount,
			"/warehouse-fulfillment-count-approvals":                      accessidentity.DecisionStockAdjustmentApproval,
			"/warehouse-fulfillment-count-rejections":                     accessidentity.DecisionStockAdjustmentApproval,
			"/warehouse-fulfillment-stock-adjustments":                    accessidentity.DecisionStockAdjustmentSubmission,
			"/warehouse-fulfillment-stock-adjustment-approvals":           accessidentity.DecisionStockAdjustmentApproval,
			"/warehouse-fulfillment-stock-adjustment-rejections":          accessidentity.DecisionStockAdjustmentApproval,
			"/warehouse-fulfillment-job-discrepancy-approvals":            accessidentity.DecisionStockAdjustmentApproval,
			"/warehouse-fulfillment-job-discrepancy-rejections":           accessidentity.DecisionStockAdjustmentApproval,
			"/warehouse-fulfillment-outbound-requests":                    accessidentity.DecisionOutboundRequestRecordOnBehalf,
			"/warehouse-fulfillment-outbound-request-splits":              accessidentity.DecisionOutboundRequestRecordOnBehalf,
			"/warehouse-fulfillment-outbound-cancellations":               accessidentity.DecisionOutboundRequestRecordOnBehalf,
			"/warehouse-fulfillment-return-pre-advices":                   accessidentity.DecisionReturnRecordOnBehalf,
			"/warehouse-fulfillment-return-dispositions":                  accessidentity.DecisionReturnRecordOnBehalf,
			"/warehouse-fulfillment-owner-labels":                         accessidentity.DecisionOutboundRequestRecordOnBehalf,
			"/warehouse-fulfillment-outbound-confirmation-corrections":    accessidentity.DecisionOutboundConfirmationCorrection,
			"/warehouse-fulfillment-outbound-confirmation-replenishments": accessidentity.DecisionStockAdjustmentApproval,
			"/warehouse-fulfillment-waves":                                accessidentity.DecisionWaveRelease,
		}
		if kind, writesOnBehalf := decisionLines[pattern]; writesOnBehalf && line.face.decision != kind {
			t.Fatalf("%s 的决定种类 = %s，want %s", pattern, line.face.decision, kind)
		}
	}
}

// Covers: AT-WF-012（端点那一半）、ADR-0222 决定三——开关开着时，闭合用例对仓配的行也核到：渠道都不配，仓配各行答「接入渠道未配置」；
// 只打开操作者渠道，各行的答复离开「接入渠道未配置」；打开别的渠道（隔离读、首方客户、集成客户端），各行仍是「接入渠道未配置」——
// 仓配的行不消费它们。只看离没离开未配置，离开后答哪一格由 AT-WF-010 那一条钉。
func TestWarehouseFulfillmentLinesOpenOnlyOnTheOperatorChannelWhenSwitchedOn(t *testing.T) {
	isolated, err := buildIsolatedReadIntakes(fakeGetenv(map[string]string{isolatedReadTenantEnv: "SYN-TENANT-01"}))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	customer := customerSubmissionWith(t, fixedCustomerVerifier{}, customerStandingRegister{start: now.Add(-time.Hour), end: now.Add(time.Hour)})
	clientMinter, err := buildIntegrationClientMinter(acceptingClientVerifier{}, absentClientRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	clients, err := buildIntegrationClientIntakes(clientMinter)
	if err != nil {
		t.Fatal(err)
	}
	nothing := unwiredChannels(unconfiguredOperatorDecisions(), unconfiguredOperatorRegistries(), unconfiguredIntegrationClientIntakes(), nil, nil, nil, nil)
	cases := []struct {
		name     string
		channels endpointChannels
		opens    bool
	}{
		{"nothing configured", nothing, false},
		{"isolated read", unwiredChannels(unconfiguredOperatorDecisions(), unconfiguredOperatorRegistries(), unconfiguredIntegrationClientIntakes(), isolated, nil, nil, nil), false},
		{"first-party customer", unwiredChannels(unconfiguredOperatorDecisions(), unconfiguredOperatorRegistries(), unconfiguredIntegrationClientIntakes(), nil, customer, nil, nil), false},
		{"integration client", unwiredChannels(unconfiguredOperatorDecisions(), unconfiguredOperatorRegistries(), clients, nil, nil, nil, nil), false},
		{"operator", channelsFor(t, everyOperatorGrant{}), true},
	}
	for _, testCase := range cases {
		router := warehouseFulfillmentRouter(t, testCase.channels, unwiredWarehouseFulfillment(true))
		lines := declaredEndpointLines()
		for _, pattern := range warehouseFulfillmentLines() {
			response := probeWarehouseFulfillment(router, pattern, presentedOperatorToken, `{}`)
			// 货主六口与收货确认的 Intake 由特性自建，不吃这里换上的共享渠道。开关开着、Intake 未接线时它们仍是未配置。
			wantOpen := testCase.opens && !lines[pattern].ownIntake
			if got := leftUnconfigured(t, response); got == wantOpen {
				t.Fatalf("%s 打开、开关开着：%s 答 %d %s（离开了「接入渠道未配置」=%t，want %t）",
					testCase.name, pattern, response.Code, response.Body.String(), !got, wantOpen)
			}
			assertNoOutcome(t, response, pattern)
		}
	}
}

// Covers: AT-WF-010（仓的部分）——操作者渠道打开后，仓配各行按各自声明的面认授予：只持登记册配置写的操作者，登记口越过授予那一格
// （答坏载荷，而不是未授予），各查阅口答 OPERATOR_NOT_GRANTED；只持查阅读的反过来：登记口答未授予，查阅口越过授予走到读口
// （这里接的是未接线读口，答 NO_ANSWER_FORMED）。无令牌答凭据被拒。
func TestWarehouseFulfillmentLinesAskForTheGrantOfTheirOwnFace(t *testing.T) {
	type answer struct {
		status int
		code   string
	}
	same := func(status int, code string) map[string]answer {
		return map[string]answer{
			warehouseRegistrationsLine: {status, code}, warehouseListLine: {status, code}, warehouseRevisionsLine: {status, code},
			zoneRegistrationsLine: {status, code}, zoneListLine: {status, code}, zoneRevisionsLine: {status, code},
			locationTypeRegistrationsLine: {status, code}, locationTypeListLine: {status, code}, locationTypeRevisionsLine: {status, code},
			locationRegistrationsLine: {status, code}, locationListLine: {status, code}, locationRevisionsLine: {status, code},
			packagingMaterialRegistrationsLine: {status, code}, packagingMaterialListLine: {status, code}, packagingMaterialRevisionsLine: {status, code},
			goodsOnBehalfLine: {status, code}, goodsListLine: {status, code}, goodsCurrentLine: {status, code},
			goodsRevisionsLine: {status, code}, goodsByCodeLine: {status, code},
			preAdviceOnBehalfLine: {status, code}, preAdviceNoticeLine: {status, code}, preAdviceWithdrawalLine: {status, code},
			preAdviceCloseLine:        {status, code},
			discrepancyConclusionLine: {status, code},
			preAdviceListLine:         {status, code}, preAdviceOneLine: {status, code}, preAdviceByNumberLine: {status, code},
			receiptConfirmationsLine:                               {status, code},
			warehouseMeasurementsLine:                              {status, code},
			goodsMeasurementsLine:                                  {status, code},
			cartonMeasurementsLine:                                 {status, code},
			disposalRuleRegistrationsLine:                          {status, code},
			disposalRuleListLine:                                   {status, code},
			disposalRuleRevisionsLine:                              {status, code},
			unclaimedArrivalRegLine:                                {status, code},
			unclaimedArrivalClaimLine:                              {status, code},
			unclaimedArrivalRevokeLine:                             {status, code},
			unclaimedArrivalDisposeLine:                            {status, code},
			unclaimedArrivalListLine:                               {status, code},
			unclaimedArrivalOneLine:                                {status, code},
			internalMovesLine:                                      {status, code},
			formConversionsLine:                                    {status, code},
			conditionConversionsLine:                               {status, code},
			shortageRegistrationsLine:                              {status, code},
			processingExecutionsLine:                               {status, code},
			relabelExecutionsLine:                                  {status, code},
			unboxingsLine:                                          {status, code},
			conditionRestoresLine:                                  {status, code},
			"/warehouse-fulfillment-processing-instructions":       {status, code},
			"/warehouse-fulfillment-processing-instruction-voids":  {status, code},
			"/warehouse-fulfillment-relabel-instructions":          {status, code},
			"/warehouse-fulfillment-relabel-instruction-voids":     {status, code},
			"/warehouse-fulfillment-stock-freezes":                 {status, code},
			"/warehouse-fulfillment-stock-unfreezes":               {status, code},
			"/warehouse-fulfillment-stock-counts":                  {status, code},
			"/warehouse-fulfillment-stock-count-starts":            {status, code},
			"/warehouse-fulfillment-stock-count-completions":       {status, code},
			"/warehouse-fulfillment-stock-count-voids":             {status, code},
			countEntriesLine:                                       {status, code},
			picksLine:                                              {status, code},
			outboundChecksLine:                                     {status, code},
			packsLine:                                              {status, code},
			returnsLine:                                            {status, code},
			returnReceiptsLine:                                     {status, code},
			outboundConfirmationsLine:                              {status, code},
			pickupHandoversLine:                                    {status, code},
			ownerLabelsLine:                                        {status, code},
			confirmationCorrectionsLine:                            {status, code},
			replenishmentsLine:                                     {status, code},
			"/warehouse-fulfillment-approval-duty-rules":           {status, code},
			"/warehouse-fulfillment-count-approvals":               {status, code},
			"/warehouse-fulfillment-count-rejections":              {status, code},
			"/warehouse-fulfillment-stock-adjustments":             {status, code},
			"/warehouse-fulfillment-stock-adjustment-approvals":    {status, code},
			"/warehouse-fulfillment-stock-adjustment-rejections":   {status, code},
			"/warehouse-fulfillment-job-discrepancy-approvals":     {status, code},
			"/warehouse-fulfillment-job-discrepancy-rejections":    {status, code},
			"/warehouse-fulfillment-outbound-requests":             {status, code},
			"/warehouse-fulfillment-outbound-request-splits":       {status, code},
			"/warehouse-fulfillment-outbound-cancellations":        {status, code},
			"/warehouse-fulfillment-return-pre-advices":            {status, code},
			"/warehouse-fulfillment-return-dispositions":           {status, code},
			"/warehouse-fulfillment-shortage-policies":             {status, code},
			"/warehouse-fulfillment-agent-connectors":              {status, code},
			"/warehouse-fulfillment-agent-stock-statements":        {status, code},
			"/warehouse-fulfillment-agent-outbound-results":        {status, code},
			"/warehouse-fulfillment-charge-measure-units":          {status, code},
			"/warehouse-fulfillment-charge-adoptions":              {status, code},
			"/warehouse-fulfillment-charge-aging-bases":            {status, code},
			"/warehouse-fulfillment-charge-service-projects":       {status, code},
			"/warehouse-fulfillment-storage-day-closes":            {status, code},
			"/warehouse-fulfillment-charge-occurrence-voids":       {status, code},
			"/warehouse-fulfillment-charge-occurrence-restates":    {status, code},
			"/warehouse-fulfillment-allocation-strategy-adoptions": {status, code},
			"/warehouse-fulfillment-waves":                         {status, code},
			"/warehouse-fulfillment-stock":                         {status, code},
			internalJobsLine:                                       {status, code},
			internalJobLine:                                        {status, code},
			conditionRestoreReadLine:                               {status, code},
		}
	}
	cases := []struct {
		name     string
		register accessidentity.OperatorRegistry
		token    string
		want     map[string]answer
	}{
		{"write grant only", registryWriterRegister{}, presentedOperatorToken, map[string]answer{
			warehouseRegistrationsLine: {http.StatusBadRequest, "MALFORMED_REQUEST"},
			warehouseListLine:          {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			warehouseRevisionsLine:     {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			zoneRegistrationsLine:      {http.StatusBadRequest, "MALFORMED_REQUEST"},
			zoneListLine:               {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			zoneRevisionsLine:          {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},

			locationTypeRegistrationsLine: {http.StatusBadRequest, "MALFORMED_REQUEST"},
			locationTypeListLine:          {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			locationTypeRevisionsLine:     {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},

			locationRegistrationsLine: {http.StatusBadRequest, "MALFORMED_REQUEST"},
			locationListLine:          {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			locationRevisionsLine:     {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},

			packagingMaterialRegistrationsLine: {http.StatusBadRequest, "MALFORMED_REQUEST"},
			packagingMaterialListLine:          {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			packagingMaterialRevisionsLine:     {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},

			goodsOnBehalfLine:  {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			goodsListLine:      {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			goodsCurrentLine:   {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			goodsRevisionsLine: {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			goodsByCodeLine:    {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},

			preAdviceOnBehalfLine:     {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			preAdviceNoticeLine:       {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			preAdviceWithdrawalLine:   {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			preAdviceCloseLine:        {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			discrepancyConclusionLine: {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			preAdviceListLine:         {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			preAdviceOneLine:          {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			preAdviceByNumberLine:     {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},

			receiptConfirmationsLine:                               {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			warehouseMeasurementsLine:                              {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			goodsMeasurementsLine:                                  {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			cartonMeasurementsLine:                                 {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			disposalRuleRegistrationsLine:                          {http.StatusBadRequest, "MALFORMED_REQUEST"},
			disposalRuleListLine:                                   {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			disposalRuleRevisionsLine:                              {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			unclaimedArrivalRegLine:                                {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			unclaimedArrivalClaimLine:                              {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			unclaimedArrivalRevokeLine:                             {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			unclaimedArrivalDisposeLine:                            {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			unclaimedArrivalListLine:                               {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			unclaimedArrivalOneLine:                                {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			internalMovesLine:                                      {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			formConversionsLine:                                    {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			conditionConversionsLine:                               {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			shortageRegistrationsLine:                              {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			processingExecutionsLine:                               {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			relabelExecutionsLine:                                  {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			unboxingsLine:                                          {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			conditionRestoresLine:                                  {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-processing-instructions":       {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-processing-instruction-voids":  {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-relabel-instructions":          {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-relabel-instruction-voids":     {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-stock-freezes":                 {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-stock-unfreezes":               {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-stock-counts":                  {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-stock-count-starts":            {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-stock-count-completions":       {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-stock-count-voids":             {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			countEntriesLine:                                       {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			picksLine:                                              {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			outboundChecksLine:                                     {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			packsLine:                                              {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			returnsLine:                                            {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			returnReceiptsLine:                                     {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			outboundConfirmationsLine:                              {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			pickupHandoversLine:                                    {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			ownerLabelsLine:                                        {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			confirmationCorrectionsLine:                            {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			replenishmentsLine:                                     {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-approval-duty-rules":           {http.StatusBadRequest, "MALFORMED_REQUEST"},
			"/warehouse-fulfillment-shortage-policies":             {http.StatusBadRequest, "MALFORMED_REQUEST"},
			"/warehouse-fulfillment-agent-connectors":              {http.StatusBadRequest, "MALFORMED_REQUEST"},
			"/warehouse-fulfillment-agent-stock-statements":        {http.StatusBadRequest, "MALFORMED_REQUEST"},
			"/warehouse-fulfillment-agent-outbound-results":        {http.StatusBadRequest, "MALFORMED_REQUEST"},
			"/warehouse-fulfillment-charge-measure-units":          {http.StatusBadRequest, "MALFORMED_REQUEST"},
			"/warehouse-fulfillment-charge-adoptions":              {http.StatusBadRequest, "MALFORMED_REQUEST"},
			"/warehouse-fulfillment-charge-aging-bases":            {http.StatusBadRequest, "MALFORMED_REQUEST"},
			"/warehouse-fulfillment-charge-service-projects":       {http.StatusBadRequest, "MALFORMED_REQUEST"},
			"/warehouse-fulfillment-storage-day-closes":            {http.StatusBadRequest, "MALFORMED_REQUEST"},
			"/warehouse-fulfillment-charge-occurrence-voids":       {http.StatusBadRequest, "MALFORMED_REQUEST"},
			"/warehouse-fulfillment-charge-occurrence-restates":    {http.StatusBadRequest, "MALFORMED_REQUEST"},
			"/warehouse-fulfillment-allocation-strategy-adoptions": {http.StatusBadRequest, "MALFORMED_REQUEST"},
			"/warehouse-fulfillment-count-approvals":               {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-count-rejections":              {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-stock-adjustments":             {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-stock-adjustment-approvals":    {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-stock-adjustment-rejections":   {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-job-discrepancy-approvals":     {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-job-discrepancy-rejections":    {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-outbound-requests":             {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-outbound-request-splits":       {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-outbound-cancellations":        {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-return-pre-advices":            {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-return-dispositions":           {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-waves":                         {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-stock":                         {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			internalJobsLine:                                       {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			internalJobLine:                                        {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			conditionRestoreReadLine:                               {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		}},
		{"read grant only", operationsReaderRegister{}, presentedOperatorToken, map[string]answer{
			warehouseRegistrationsLine: {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			warehouseListLine:          {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
			warehouseRevisionsLine:     {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
			zoneRegistrationsLine:      {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			zoneListLine:               {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
			zoneRevisionsLine:          {http.StatusInternalServerError, "NO_ANSWER_FORMED"},

			locationTypeRegistrationsLine: {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			locationTypeListLine:          {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
			locationTypeRevisionsLine:     {http.StatusInternalServerError, "NO_ANSWER_FORMED"},

			locationRegistrationsLine: {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			locationListLine:          {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
			locationRevisionsLine:     {http.StatusInternalServerError, "NO_ANSWER_FORMED"},

			packagingMaterialRegistrationsLine: {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			packagingMaterialListLine:          {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
			packagingMaterialRevisionsLine:     {http.StatusInternalServerError, "NO_ANSWER_FORMED"},

			goodsOnBehalfLine:  {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			goodsListLine:      {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
			goodsCurrentLine:   {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
			goodsRevisionsLine: {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
			goodsByCodeLine:    {http.StatusInternalServerError, "NO_ANSWER_FORMED"},

			preAdviceOnBehalfLine:     {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			preAdviceNoticeLine:       {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			preAdviceWithdrawalLine:   {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			preAdviceCloseLine:        {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			discrepancyConclusionLine: {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			preAdviceListLine:         {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
			preAdviceOneLine:          {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
			preAdviceByNumberLine:     {http.StatusInternalServerError, "NO_ANSWER_FORMED"},

			receiptConfirmationsLine:                               {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			warehouseMeasurementsLine:                              {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			goodsMeasurementsLine:                                  {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
			cartonMeasurementsLine:                                 {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
			disposalRuleRegistrationsLine:                          {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			disposalRuleListLine:                                   {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
			disposalRuleRevisionsLine:                              {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
			unclaimedArrivalRegLine:                                {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			unclaimedArrivalClaimLine:                              {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			unclaimedArrivalRevokeLine:                             {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			unclaimedArrivalDisposeLine:                            {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			unclaimedArrivalListLine:                               {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
			unclaimedArrivalOneLine:                                {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
			internalMovesLine:                                      {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			formConversionsLine:                                    {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			conditionConversionsLine:                               {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			shortageRegistrationsLine:                              {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			processingExecutionsLine:                               {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			relabelExecutionsLine:                                  {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			unboxingsLine:                                          {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			conditionRestoresLine:                                  {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-processing-instructions":       {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-processing-instruction-voids":  {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-relabel-instructions":          {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-relabel-instruction-voids":     {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-stock-freezes":                 {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-stock-unfreezes":               {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-stock-counts":                  {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-stock-count-starts":            {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-stock-count-completions":       {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-stock-count-voids":             {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			countEntriesLine:                                       {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			picksLine:                                              {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			outboundChecksLine:                                     {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			packsLine:                                              {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			returnsLine:                                            {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			returnReceiptsLine:                                     {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			outboundConfirmationsLine:                              {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			pickupHandoversLine:                                    {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			ownerLabelsLine:                                        {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			confirmationCorrectionsLine:                            {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			replenishmentsLine:                                     {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-approval-duty-rules":           {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-count-approvals":               {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-count-rejections":              {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-stock-adjustments":             {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-stock-adjustment-approvals":    {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-stock-adjustment-rejections":   {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-job-discrepancy-approvals":     {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-job-discrepancy-rejections":    {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-outbound-requests":             {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-outbound-request-splits":       {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-outbound-cancellations":        {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-return-pre-advices":            {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-return-dispositions":           {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-shortage-policies":             {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-agent-connectors":              {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-agent-stock-statements":        {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-agent-outbound-results":        {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-charge-measure-units":          {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-charge-adoptions":              {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-charge-aging-bases":            {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-charge-service-projects":       {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-storage-day-closes":            {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-charge-occurrence-voids":       {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-charge-occurrence-restates":    {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-allocation-strategy-adoptions": {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-waves":                         {http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
			"/warehouse-fulfillment-stock":                         {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
			internalJobsLine:                                       {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
			internalJobLine:                                        {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
			conditionRestoreReadLine:                               {http.StatusInternalServerError, "NO_ANSWER_FORMED"},
		}},
		{"no bearer token", everyOperatorGrant{}, "", same(http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED")},
		{"register is down", failingRegister{}, presentedOperatorToken, same(http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE")},
	}
	for _, testCase := range cases {
		channels := channelsFor(t, testCase.register)
		wiring := unwiredWarehouseFulfillment(true)
		intake, err := buildReceiptConfirmationIntake(acceptingVerifier{}, testCase.register, workFactDevices{}, accessidentity.UnconfiguredAdmissionScope{})
		if err != nil {
			t.Fatal(err)
		}
		wiring.receiptIntake = intake
		measurementIntake, err := buildWarehouseMeasurementIntake(acceptingVerifier{}, testCase.register, workFactDevices{}, accessidentity.UnconfiguredAdmissionScope{})
		if err != nil {
			t.Fatal(err)
		}
		wiring.measurementIntake = measurementIntake
		unclaimedIntake, err := buildUnclaimedArrivalIntake(acceptingVerifier{}, testCase.register, workFactDevices{}, accessidentity.UnconfiguredAdmissionScope{})
		if err != nil {
			t.Fatal(err)
		}
		wiring.unclaimedArrivalIntake = unclaimedIntake
		moveIntake, formIntake, conditionIntake, shortageIntake, intakeErr := buildInternalJobIntakes(acceptingVerifier{}, testCase.register, workFactDevices{}, accessidentity.UnconfiguredAdmissionScope{})
		if intakeErr != nil {
			t.Fatal(intakeErr)
		}
		wiring.internalMoveIntake = moveIntake
		wiring.formConversionIntake = formIntake
		wiring.conditionConversionIntake = conditionIntake
		wiring.shortageIntake = shortageIntake
		processingIntake, relabelIntake, unboxIntake, instructionIntakeErr := buildInstructionIntakes(acceptingVerifier{}, testCase.register, workFactDevices{}, accessidentity.UnconfiguredAdmissionScope{})
		if instructionIntakeErr != nil {
			t.Fatal(instructionIntakeErr)
		}
		wiring.processingExecutionIntake = processingIntake
		wiring.relabelExecutionIntake = relabelIntake
		wiring.unboxIntake = unboxIntake
		countEntry, countEntryErr := buildCountEntryIntake(acceptingVerifier{}, testCase.register, workFactDevices{}, accessidentity.UnconfiguredAdmissionScope{})
		if countEntryErr != nil {
			t.Fatal(countEntryErr)
		}
		wiring.countEntryIntake = countEntry
		pickIntake, checkIntake, packIntake, confirmIntake, pickupIntake, workIntakeErr := buildOutboundWorkIntakes(acceptingVerifier{}, testCase.register, workFactDevices{}, accessidentity.UnconfiguredAdmissionScope{})
		if workIntakeErr != nil {
			t.Fatal(workIntakeErr)
		}
		wiring.pickIntake = pickIntake
		wiring.checkIntake = checkIntake
		wiring.packIntake = packIntake
		wiring.confirmIntake = confirmIntake
		wiring.pickupIntake = pickupIntake
		returnIntake, returnErr := buildReturnIntake(acceptingVerifier{}, testCase.register, workFactDevices{}, accessidentity.UnconfiguredAdmissionScope{})
		if returnErr != nil {
			t.Fatal(returnErr)
		}
		wiring.returnIntake = returnIntake
		returnReceiptIntake, receiptErr := buildReturnReceiptIntake(acceptingVerifier{}, testCase.register, workFactDevices{}, accessidentity.UnconfiguredAdmissionScope{})
		if receiptErr != nil {
			t.Fatal(receiptErr)
		}
		wiring.returnReceiptIntake = returnReceiptIntake
		router := warehouseFulfillmentRouter(t, channels, wiring)
		for pattern, want := range testCase.want {
			var response *httptest.ResponseRecorder
			switch pattern {
			case receiptConfirmationsLine:
				response = probeWorkFactPost(router, receiptConfirmationsLine, testCase.token, "SYN-DEVICE-NODE", "WAREHOUSE", "SYN-WH-01", receiptConfirmationBody())
			case warehouseMeasurementsLine:
				response = probeWorkFactPost(router, warehouseMeasurementsLine, testCase.token, "SYN-DEVICE-NODE", "WAREHOUSE", "SYN-WH-01", warehouseMeasurementBody())
			case unclaimedArrivalRegLine:
				response = probeWorkFactPost(router, unclaimedArrivalRegLine, testCase.token, "SYN-DEVICE-NODE", "WAREHOUSE", "SYN-WH-01", unclaimedArrivalBody())
			case internalMovesLine, formConversionsLine, conditionConversionsLine, shortageRegistrationsLine, processingExecutionsLine, relabelExecutionsLine, unboxingsLine, countEntriesLine, picksLine, outboundChecksLine, packsLine, returnsLine, returnReceiptsLine, outboundConfirmationsLine, pickupHandoversLine:
				response = probeWorkFactPost(router, pattern, testCase.token, "SYN-DEVICE-NODE", "WAREHOUSE", "SYN-WH-01", `{}`)
			default:
				response = probeWarehouseFulfillment(router, pattern, testCase.token, `{}`)
			}
			if response.Code != want.status || problemCode(t, response) != want.code {
				t.Fatalf("%s, %s：答 %d %s，want %d %s", testCase.name, pattern, response.Code, response.Body.String(), want.status, want.code)
			}
			assertNoOutcome(t, response, pattern)
		}
	}

	// 登在节点上的设备，拿仓的作业范围去收货、仓测或待认领登记：授予在仓上，设备册上的范围仍是节点，答设备未登记或已停用。
	nodeDevice := unwiredWarehouseFulfillment(true)
	nodeIntake, err := buildReceiptConfirmationIntake(acceptingVerifier{}, warehouseWorkFactRegister{}, workFactDevices{}, accessidentity.UnconfiguredAdmissionScope{})
	if err != nil {
		t.Fatal(err)
	}
	nodeDevice.receiptIntake = nodeIntake
	nodeMeasurementIntake, err := buildWarehouseMeasurementIntake(acceptingVerifier{}, warehouseWorkFactRegister{}, workFactDevices{}, accessidentity.UnconfiguredAdmissionScope{})
	if err != nil {
		t.Fatal(err)
	}
	nodeDevice.measurementIntake = nodeMeasurementIntake
	nodeUnclaimedIntake, err := buildUnclaimedArrivalIntake(acceptingVerifier{}, warehouseWorkFactRegister{}, workFactDevices{}, accessidentity.UnconfiguredAdmissionScope{})
	if err != nil {
		t.Fatal(err)
	}
	nodeDevice.unclaimedArrivalIntake = nodeUnclaimedIntake
	nodeRouter := warehouseFulfillmentRouter(t, channelsFor(t, warehouseWorkFactRegister{}), nodeDevice)
	nodeResponse := probeWorkFactPost(nodeRouter, receiptConfirmationsLine, presentedOperatorToken, "SYN-DEVICE-NODE", "WAREHOUSE", "SYN-WH-01", receiptConfirmationBody())
	if nodeResponse.Code != http.StatusForbidden || problemCode(t, nodeResponse) != "DEVICE_NOT_IN_USE" {
		t.Fatalf("节点上的设备去收货：答 %d %s，want 403 DEVICE_NOT_IN_USE", nodeResponse.Code, nodeResponse.Body.String())
	}
	assertNoOutcome(t, nodeResponse, receiptConfirmationsLine)
	nodeMeasureResp := probeWorkFactPost(nodeRouter, warehouseMeasurementsLine, presentedOperatorToken, "SYN-DEVICE-NODE", "WAREHOUSE", "SYN-WH-01", warehouseMeasurementBody())
	if nodeMeasureResp.Code != http.StatusForbidden || problemCode(t, nodeMeasureResp) != "DEVICE_NOT_IN_USE" {
		t.Fatalf("节点上的设备去仓测：答 %d %s，want 403 DEVICE_NOT_IN_USE", nodeMeasureResp.Code, nodeMeasureResp.Body.String())
	}
	assertNoOutcome(t, nodeMeasureResp, warehouseMeasurementsLine)
	nodeUnclaimedResp := probeWorkFactPost(nodeRouter, unclaimedArrivalRegLine, presentedOperatorToken, "SYN-DEVICE-NODE", "WAREHOUSE", "SYN-WH-01", unclaimedArrivalBody())
	if nodeUnclaimedResp.Code != http.StatusForbidden || problemCode(t, nodeUnclaimedResp) != "DEVICE_NOT_IN_USE" {
		t.Fatalf("节点上的设备去登待认领：答 %d %s，want 403 DEVICE_NOT_IN_USE", nodeUnclaimedResp.Code, nodeUnclaimedResp.Body.String())
	}
	assertNoOutcome(t, nodeUnclaimedResp, unclaimedArrivalRegLine)
}

// warehouseWorkFactRegister 只持 SYN-WH-01 上的作业事实登记授予，没有登记册配置写。
type warehouseWorkFactRegister struct{}

func (warehouseWorkFactRegister) FindOperator(_ context.Context, subject accessidentity.OperatorSubject) (accessidentity.OperatorStanding, bool, error) {
	binding, err := accessidentity.NewOperatorBinding(subject, "SYN-TENANT-01", "SYN-BASIS-BINDING")
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	grant, err := accessidentity.NewWorkFactRegistrationGrant("SYN-TENANT-01", "SYN-GRANT-WH", subject, mustScope("WAREHOUSE", "SYN-WH-01"), interval, "SYN-BASIS-GRANT")
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	recorded, err := accessidentity.NewRecordedGrant(grant, nil)
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	standing, err := accessidentity.NewOperatorStanding(binding, []accessidentity.RecordedGrant{recorded})
	return standing, err == nil, err
}

func receiptConfirmationBody() string {
	return `{"factId":"SYN-FACT-01","occurredAt":"2026-11-20T01:00:00Z","warehouse":"SYN-WH-01","preAdviceId":"SYN-IPA-01","shape":"BY_GOODS","lines":[{"observed":"SYN-GOODS-01","unit":"EA","stockState":"GOOD","quantity":1,"locationCode":"SYN-LOC-01"}]}`
}

func warehouseMeasurementBody() string {
	return `{"factId":"SYN-MEAS-01","occurredAt":"2026-11-20T01:00:00Z","deviceSequence":1,"warehouse":"SYN-WH-01","targetKind":"GOODS_LEVEL","ownerAccount":"SYN-OWNER-01","goodsCode":"SYN-GOODS-01","packagingLevel":"EA","dimensions":{"length":"30","width":"20","height":"10","unit":"CM"},"weight":{"value":"1.5","unit":"KG"}}`
}

func unclaimedArrivalBody() string {
	return `{"factId":"SYN-UCA-FACT-01","occurredAt":"2026-11-20T01:00:00Z","deviceSequence":1,"warehouse":"SYN-WH-01","kind":"INBOUND","observation":"外箱无货主标签，约三箱"}`
}

func probeWorkFactPost(router http.Handler, pattern, token, device, kind, reference, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, pattern, strings.NewReader(body))
	request.Header.Set("Parcel-Work-Scope-Kind", kind)
	request.Header.Set("Parcel-Work-Scope-Reference", reference)
	request.Header.Set("Parcel-Device-Id", device)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

// Covers: AT-WF-012（端点那一半）——开关关着时仓配各行都答 403 WAREHOUSE_FULFILLMENT_NOT_ENABLED，不是 404，也不是「接入渠道未配置」，
// 不带 outcome；与渠道配没配无关（全不配、操作者渠道授予齐备且带令牌，答复逐字相同，requestId 除外）；方法不对仍答 405 并带本行
// 声明的方法（与开关开着时同一份）。
func TestSwitchedOffWarehouseFulfillmentLinesAnswerNotEnabledWhateverTheChannels(t *testing.T) {
	nothing := unwiredChannels(unconfiguredOperatorDecisions(), unconfiguredOperatorRegistries(), unconfiguredIntegrationClientIntakes(), nil, nil, nil, nil)
	everything := unwiredChannels(unconfiguredOperatorDecisions(), registriesFor(t, everyOperatorGrant{}), unconfiguredIntegrationClientIntakes(), nil, nil, nil, nil)
	closed, open := warehouseFulfillmentRouter(t, nothing, unwiredWarehouseFulfillment(false)), warehouseFulfillmentRouter(t, everything, unwiredWarehouseFulfillment(false))

	lines := declaredEndpointLines()
	for _, pattern := range warehouseFulfillmentLines() {
		bare := probeWarehouseFulfillment(closed, pattern, "", "")
		granted := probeWarehouseFulfillment(open, pattern, presentedOperatorToken, `{"tenantId":"SYN-TENANT-02"}`)
		for name, response := range map[string]*httptest.ResponseRecorder{"nothing configured": bare, "operator channel fully open": granted} {
			if response.Code != http.StatusForbidden || problemCode(t, response) != "WAREHOUSE_FULFILLMENT_NOT_ENABLED" {
				t.Fatalf("%s，%s：答 %d %s，want 403 WAREHOUSE_FULFILLMENT_NOT_ENABLED", name, pattern, response.Code, response.Body.String())
			}
			if response.Header().Get("Content-Type") != httpapi.ProblemMediaType {
				t.Fatalf("%s，%s：Content-Type = %q，want %s", name, pattern, response.Header().Get("Content-Type"), httpapi.ProblemMediaType)
			}
			assertNoOutcome(t, response, pattern)
		}
		if withoutRequestID(t, bare) != withoutRequestID(t, granted) {
			t.Fatalf("%s：开关关着的答复随渠道变了：%s 与 %s", pattern, bare.Body.String(), granted.Body.String())
		}

		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
			response := httptest.NewRecorder()
			closed.ServeHTTP(response, httptest.NewRequest(method, pattern, nil))
			switch {
			case response.Code == http.StatusNotFound:
				t.Fatalf("%s %s：开关关着不得答 404", method, pattern)
			case method == lines[pattern].method:
			case response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != lines[pattern].method:
				t.Fatalf("%s %s：答 %d（Allow=%q），want 405 并带 %s", method, pattern, response.Code, response.Header().Get("Allow"), lines[pattern].method)
			}
		}
	}
}

// Covers: AT-WF-012（端点那一半）——关着时同一装配里别的行的答复与开关无关：本特性以外的行，在仓配开关关着与开着的两种装配下答复
// 逐字相同（拿一行不属于仓配的登记写行与一行查阅行比）。
func TestOtherLinesAnswerTheSameWhetherOrNotWarehouseFulfillmentIsSwitchedOn(t *testing.T) {
	channels := unwiredChannels(unconfiguredOperatorDecisions(), unconfiguredOperatorRegistries(), unconfiguredIntegrationClientIntakes(), nil, nil, nil, nil)
	for _, pattern := range []string{"/commercial-registration-number-type-registrations", "/commercial-registration-number-types"} {
		line := declaredEndpointLines()[pattern]
		if line.warehouseFulfillment {
			t.Fatalf("%s 不该声明仓配开关", pattern)
		}
		var answers []string
		for _, enabled := range []bool{false, true} {
			endpoints := assembleUnwiredEndpointsReplacing(channels, unwiredWarehouseFulfillment(enabled))
			response := httptest.NewRecorder()
			request := httptest.NewRequest(line.method, businessEndpointProbes()[pattern].target, strings.NewReader(`{}`))
			httpapi.NewWithEndpoints(buildinfo.Info{}, endpoints).ServeHTTP(response, request)
			answers = append(answers, withoutRequestID(t, response))
			if !leftUnconfigured(t, response) {
				t.Fatalf("%s：答 %d %s，want 起步的「接入渠道未配置」", pattern, response.Code, response.Body.String())
			}
		}
		if answers[0] != answers[1] {
			t.Fatalf("%s：开关关着与开着答复不同：%s 与 %s", pattern, answers[0], answers[1])
		}
	}
}

// assembleUnwiredEndpointsReplacing 同 assembleUnwiredEndpoints，只把仓配特性的占位换成给定的一份。
func assembleUnwiredEndpointsReplacing(channels endpointChannels, wiring warehouseFulfillmentWiring) []httpapi.BusinessEndpoint {
	var endpoints []httpapi.BusinessEndpoint
	replaced := any((*warehouseFulfillmentWiring)(nil))
	for _, feature := range endpointRegistry {
		placeholder := unwiredWirings[feature.wiringKey]
		if feature.wiringKey == replaced {
			placeholder = wiring
		}
		endpoints = append(endpoints, feature.mountWith(channels, placeholder)...)
	}
	return endpoints
}

// Covers: ADR-0375 决定四「关着时不构造任何仓配的适配器与编排」——开关关着时特性的 build 不碰库：db 传 nil 也装得起来，装出的
// 依赖没有任何编排；开着时 build 要构造仓配的适配器，db 为空装不起来（证明上面「装得起来」不是因为 build 什么都不做）。
// 开着时的失败不得是缺分配器：那一判若排在任何适配器之前，build 什么都不做也会红，证不出构造过适配器。
func TestTheSwitchedOffFeatureBuildsNoAdaptersAndTheSwitchedOnOneNeedsTheDatabase(t *testing.T) {
	var feature registeredEndpoints
	for _, registered := range endpointRegistry {
		if registered.wiringKey == any((*warehouseFulfillmentWiring)(nil)) {
			feature = registered
		}
	}
	if feature.production == nil {
		t.Fatal("登记处没有仓配履约这个特性")
	}

	endpoints, err := feature.production(endpointSources{warehouseFulfillment: false}, endpointChannels{})
	if err != nil {
		t.Fatalf("开关关着、db 为空：%v，want 装得起来", err)
	}
	if len(endpoints) != len(warehouseFulfillmentLines()) {
		t.Fatalf("开关关着挂出 %d 个端点，want 与声明的行数一致", len(endpoints))
	}
	_, err = feature.production(endpointSources{warehouseFulfillment: true}, endpointChannels{})
	if err == nil || strings.Contains(err.Error(), "inbound pre-advice numbers need the document number assigner") {
		t.Fatalf("开关开着、db 为空：%v，want 来自构造仓配适配器", err)
	}
}

// Covers: 入库预报取号要单据号分配器。开关开着、库在、分配器没给时，装配必须失败，而且失败就是缺分配器这一句。
func TestTheSwitchedOnAssemblyFailsWithoutTheDocumentNumberAssigner(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	var feature registeredEndpoints
	for _, registered := range endpointRegistry {
		if registered.wiringKey == any((*warehouseFulfillmentWiring)(nil)) {
			feature = registered
		}
	}
	if feature.production == nil {
		t.Fatal("登记处没有仓配履约这个特性")
	}
	_, err = feature.production(endpointSources{db: db, warehouseFulfillment: true}, endpointChannels{})
	if err == nil || !strings.Contains(err.Error(), "inbound pre-advice numbers need the document number assigner") {
		t.Fatalf("缺分配器：%v", err)
	}
}
