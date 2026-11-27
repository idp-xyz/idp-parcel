package warehousehttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// OutboundRequestQueries 是运营查阅出库请求列表的读口。
type OutboundRequestQueries interface {
	List(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[ports.OutboundRequestRow], error)
}

// WaveQueries 是运营查阅波次的读口：分页列表，与按波次号读一张。
type WaveQueries interface {
	ListWaves(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[ports.WaveRow], error)
	LoadWaveRow(ctx context.Context, tenant domain.TenantID, waveID string) (ports.WaveRow, bool, error)
	ListPickDifferences(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[ports.PickDifferenceRow], error)
}

// ChargeOccurrenceQueries 是运营查阅仓储作业收费发生项列表的读口。
type ChargeOccurrenceQueries interface {
	ListOccurrences(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[ports.ChargeOccurrenceRow], error)
}

// WarehouseAdoptionReadRow 是运营查阅仓储计费采用列表的一行（由结算上下文回读）。
type WarehouseAdoptionReadRow struct {
	OwnerAccount  string
	WarehouseCode string
	Revision      int
	EffectiveAt   time.Time
	Account       string
	PricingScope  string
	Basis         string
	Registrant    string
}

// WarehouseAdoptionQueries 由装配点桥接到结算的采用册读口；rawQuery 交由结算自己的分页声明解码。
type WarehouseAdoptionQueries interface {
	ListAdoptions(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		rawQuery string,
	) (cataloguepage.Result[WarehouseAdoptionReadRow], error)
}

// WarehouseChargeRequestReadRow 是运营查阅仓储 SELL 计费请求列表的一行（由结算上下文回读）。
type WarehouseChargeRequestReadRow struct {
	OccurrenceID       string
	Version            int
	Status             string
	ChargeID           string
	EvaluationID       string
	RequestID          string
	Account            string
	SettlementCurrency string
	PricingScope       string
	PricingBasisAt     time.Time
}

// WarehouseChargeRequestQueries 由装配点桥接到结算的仓储计费请求读口。
type WarehouseChargeRequestQueries interface {
	ListRequests(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		rawQuery string,
	) (cataloguepage.Result[WarehouseChargeRequestReadRow], error)
}

// NewQueryOutboundRequestsEndpoint 交回运营分页查阅出库请求的入口（GET /warehouse-fulfillment-outbound-request-list）。
func NewQueryOutboundRequestsEndpoint(intake CatalogueIntake, queries OutboundRequestQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		pageQuery, err := ports.OutboundRequestPages.DecodeRaw(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		if pageQuery.PageSize > 0 {
			query.Limit = pageQuery.PageSize
		}
		page, err := queries.List(request.Context(), query.Tenant, query.Limit, pageQuery)
		if err != nil {
			slog.ErrorContext(request.Context(), "outbound request list did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		items := make([]outboundRequestRowBody, 0, len(page.Rows))
		for _, row := range page.Rows {
			lines := make([]outboundRequestLineBody, 0, len(row.Lines))
			for _, line := range row.Lines {
				lines = append(lines, outboundRequestLineBody{
					GoodsCode: line.GoodsCode, Unit: line.Unit, StockState: line.StockState, Quantity: line.Quantity,
				})
			}
			items = append(items, outboundRequestRowBody{
				RequestID:                row.RequestID,
				DocumentNumber:           row.DocumentNumber,
				NumberingAbsence:         row.NumberingAbsence,
				OwnerAccount:             row.OwnerAccount,
				WarehouseCode:            row.WarehouseCode,
				Origin:                   row.Origin,
				RequestKey:               row.RequestKey,
				Dispatch:                 row.Dispatch,
				Service:                  row.Service,
				Recipient:                row.Recipient,
				Address:                  row.Address,
				Basis:                    row.Basis,
				Operator:                 row.Operator,
				Status:                   row.Status,
				ParentRequestID:          row.ParentRequestID,
				PlatformStocking:         row.PlatformStocking,
				PartialShipment:          row.PartialShipment,
				PrefetchShipmentID:       row.PrefetchShipmentID,
				PrefetchDeclaredParcelID: row.PrefetchDeclaredParcelID,
				ReceivedAt:               rfc3339(row.ReceivedAt),
				Lines:                    lines,
				ReservedQuantity:         row.ReservedQuantity,
				WantedQuantity:           row.WantedQuantity,
				AllocationException:      allocationExceptionBodyOf(row.AllocationException),
			})
		}
		writeJSON(response, http.StatusOK, outboundRequestListResponse{
			Outcome:  "OUTBOUND_REQUESTS_LISTED",
			Requests: items,
			Page:     page.Page(query.Limit),
		})
	})
}

// NewQueryWavesEndpoint 交回运营分页查阅波次的入口（GET /warehouse-fulfillment-wave-list）。
func NewQueryWavesEndpoint(intake CatalogueIntake, queries WaveQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		pageQuery, err := ports.WavePages.DecodeRaw(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		if pageQuery.PageSize > 0 {
			query.Limit = pageQuery.PageSize
		}
		page, err := queries.ListWaves(request.Context(), query.Tenant, query.Limit, pageQuery)
		if err != nil {
			slog.ErrorContext(request.Context(), "wave list did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		items := make([]waveRowBody, 0, len(page.Rows))
		for _, row := range page.Rows {
			items = append(items, waveRowBodyOf(row))
		}
		writeJSON(response, http.StatusOK, waveListResponse{
			Outcome: "WAVES_LISTED",
			Waves:   items,
			Page:    page.Page(query.Limit),
		})
	})
}

// NewQueryWaveEndpoint 交回运营按波次号读一张的入口（GET /warehouse-fulfillment-wave?waveId=...）。
// 没有这个波次是一种答案，不是出错：答 WAVE_NOT_FOUND，不带 wave。
func NewQueryWaveEndpoint(intake CatalogueIntake, queries WaveQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		waveID := strings.TrimSpace(request.URL.Query().Get("waveId"))
		if waveID == "" {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		row, found, err := queries.LoadWaveRow(request.Context(), query.Tenant, waveID)
		if err != nil {
			slog.ErrorContext(request.Context(), "wave read did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		if !found {
			writeJSON(response, http.StatusOK, waveResponse{Outcome: "WAVE_NOT_FOUND"})
			return
		}
		body := waveRowBodyOf(row)
		writeJSON(response, http.StatusOK, waveResponse{Outcome: "WAVE_FOUND", Wave: &body})
	})
}

// SingleUnitMatching 替复核打包台扫到的一件货配一张单品单件的出库请求。
type SingleUnitMatching interface {
	Match(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, code domain.IdentifierCode) (application.SingleUnitMatch, bool, error)
}

// NewMatchSingleUnitOrderEndpoint 交回扫货配单的入口（GET /warehouse-fulfillment-single-unit-match?warehouseCode=...&code=...）。
// 没有等复核的单要这种货是一种答案，不是出错：答 SINGLE_UNIT_NOT_MATCHED。这是读：配上的那张单并没有被占住，
// 两个人同时扫同一种货会配到同一张，后复核的那一个由复核口答状态不符，再扫一次配到下一张。
func NewMatchSingleUnitOrderEndpoint(intake CatalogueIntake, matcher SingleUnitMatching) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		warehouse, warehouseErr := domain.NewWarehouseCode(strings.TrimSpace(request.URL.Query().Get("warehouseCode")))
		code, codeErr := domain.NewIdentifierCode(strings.TrimSpace(request.URL.Query().Get("code")))
		if warehouseErr != nil || codeErr != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		match, found, err := matcher.Match(request.Context(), query.Tenant, warehouse, code)
		if err != nil {
			slog.ErrorContext(request.Context(), "single unit match did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		if !found {
			writeJSON(response, http.StatusOK, singleUnitMatchResponse{Outcome: "SINGLE_UNIT_NOT_MATCHED"})
			return
		}
		writeJSON(response, http.StatusOK, singleUnitMatchResponse{
			Outcome: "SINGLE_UNIT_MATCHED", RequestID: match.Request.String(), DocumentNumber: match.DocumentNumber,
			OwnerAccount: match.Owner.String(), GoodsCode: match.Goods.String(), Waiting: match.Waiting,
		})
	})
}

type singleUnitMatchResponse struct {
	Outcome        string `json:"outcome"`
	RequestID      string `json:"requestId,omitempty"`
	DocumentNumber string `json:"documentNumber,omitempty"`
	OwnerAccount   string `json:"ownerAccount,omitempty"`
	GoodsCode      string `json:"goodsCode,omitempty"`
	// Waiting 是这位货主的这种货此刻还有几张单等着复核，含配上的这一张。
	Waiting int `json:"waiting,omitempty"`
}

// NewQueryPickDifferencesEndpoint 交回运营分页查阅拣货差异的入口（GET /warehouse-fulfillment-pick-differences）。
func NewQueryPickDifferencesEndpoint(intake CatalogueIntake, queries WaveQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		pageQuery, err := ports.PickDifferencePages.DecodeRaw(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		if pageQuery.PageSize > 0 {
			query.Limit = pageQuery.PageSize
		}
		page, err := queries.ListPickDifferences(request.Context(), query.Tenant, query.Limit, pageQuery)
		if err != nil {
			slog.ErrorContext(request.Context(), "pick difference list did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		items := make([]pickDifferenceBody, 0, len(page.Rows))
		for _, row := range page.Rows {
			items = append(items, pickDifferenceBody{
				DifferenceID: row.DifferenceID, Status: row.Status, Quantity: row.Quantity, TaskID: row.TaskID, WaveID: row.WaveID,
				RequestID: row.RequestID, OwnerAccount: row.OwnerAccount, WarehouseCode: row.WarehouseCode, LocationCode: row.LocationCode,
				GoodsCode: row.GoodsCode, Unit: row.Unit, StockState: row.StockState, TaskQuantity: row.TaskQuantity, Picked: row.Picked,
				RecordedAt: optRFC3339(row.RecordedAt), ClosedAt: optRFC3339(row.ClosedAt), ClosedByCount: row.ClosedByCount,
			})
		}
		writeJSON(response, http.StatusOK, pickDifferenceListResponse{
			Outcome:     "PICK_DIFFERENCES_LISTED",
			Differences: items,
			Page:        page.Page(query.Limit),
		})
	})
}

type pickDifferenceListResponse struct {
	Outcome     string               `json:"outcome"`
	Differences []pickDifferenceBody `json:"differences"`
	Page        cataloguepage.Page   `json:"page"`
}

type pickDifferenceBody struct {
	DifferenceID  string `json:"differenceId"`
	Status        string `json:"status"`
	Quantity      int64  `json:"quantity"`
	TaskID        string `json:"taskId"`
	WaveID        string `json:"waveId"`
	RequestID     string `json:"requestId"`
	OwnerAccount  string `json:"ownerAccount"`
	WarehouseCode string `json:"warehouseCode"`
	LocationCode  string `json:"locationCode"`
	GoodsCode     string `json:"goodsCode"`
	Unit          string `json:"unit"`
	StockState    string `json:"stockState"`
	TaskQuantity  int64  `json:"taskQuantity"`
	Picked        int64  `json:"picked"`
	RecordedAt    string `json:"recordedAt,omitempty"`
	ClosedAt      string `json:"closedAt,omitempty"`
	ClosedByCount string `json:"closedByCount,omitempty"`
}

func waveRowBodyOf(row ports.WaveRow) waveRowBody {
	tasks := make([]waveTaskBody, 0, len(row.Tasks))
	for _, task := range row.Tasks {
		tasks = append(tasks, waveTaskBody{
			TaskID: task.TaskID, RequestID: task.RequestID, OwnerAccount: task.OwnerAccount, LocationCode: task.LocationCode,
			ZoneCode: task.ZoneCode, StagingLocation: task.StagingLocation, GoodsCode: task.GoodsCode, Unit: task.Unit,
			StockState: task.StockState, Quantity: task.Quantity, Picked: task.Picked, Status: task.Status,
		})
	}
	requests := row.Requests
	if requests == nil {
		requests = []string{}
	}
	withdrawn := row.Withdrawn
	if withdrawn == nil {
		withdrawn = []string{}
	}
	var zones []waveZoneAssigneeBody
	for _, zone := range row.ZoneAssignees {
		zones = append(zones, waveZoneAssigneeBody{ZoneCode: zone.ZoneCode, Assignee: zone.Assignee})
	}
	return waveRowBody{
		WaveID: row.WaveID, WarehouseCode: row.WarehouseCode, Operator: row.Operator,
		Prefetch: row.Prefetch, Status: row.Status, ReleasedAt: rfc3339(row.ReleasedAt), SingleUnit: row.SingleUnit, BatchPick: row.BatchPick, RuleCode: row.RuleCode, Assignee: row.Assignee,
		ZoneAssignees: zones, Requests: requests, WithdrawnRequests: withdrawn, Tasks: tasks,
	}
}

// NewQueryChargeOccurrencesEndpoint 交回运营分页查阅仓储作业收费发生项的入口（GET /warehouse-fulfillment-charge-occurrences）。
func NewQueryChargeOccurrencesEndpoint(intake CatalogueIntake, queries ChargeOccurrenceQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		pageQuery, err := ports.ChargeOccurrencePages.DecodeRaw(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		if pageQuery.PageSize > 0 {
			query.Limit = pageQuery.PageSize
		}
		page, err := queries.ListOccurrences(request.Context(), query.Tenant, query.Limit, pageQuery)
		if err != nil {
			slog.ErrorContext(request.Context(), "charge occurrence list did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		items := make([]chargeOccurrenceRowBody, 0, len(page.Rows))
		for _, row := range page.Rows {
			measures := row.Measures
			if measures == nil {
				measures = []ports.ChargeMeasure{}
			}
			attrs := row.Attributes
			if attrs == nil {
				attrs = []ports.ChargeAttribute{}
			}
			items = append(items, chargeOccurrenceRowBody{
				OccurrenceID: row.OccurrenceID,
				Version:      row.Version,
				Supersedes:   row.Supersedes,
				Correction:   row.Correction,
				Kind:         row.Kind,
				OwnerAccount: row.OwnerAccount,
				Warehouse:    row.Warehouse,
				SourceFact:   row.SourceFact,
				BusinessTime: rfc3339(row.BusinessTime),
				WeightUnit:   row.WeightUnit,
				VolumeUnit:   row.VolumeUnit,
				Measures:     measures,
				Attributes:   attrs,
				LineCount:    row.LineCount,
			})
		}
		writeJSON(response, http.StatusOK, chargeOccurrenceListResponse{
			Outcome:     "CHARGE_OCCURRENCES_LISTED",
			Occurrences: items,
			Page:        page.Page(query.Limit),
		})
	})
}

// NewQueryWarehouseAdoptionsEndpoint 交回运营分页查阅仓储计费采用的入口（GET /warehouse-fulfillment-charge-adoption-list）。
func NewQueryWarehouseAdoptionsEndpoint(intake CatalogueIntake, queries WarehouseAdoptionQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		if chosen, err := cataloguepage.ChosenPageSize(request.URL.RawQuery); err != nil {
			writePageQueryProblem(response, err)
			return
		} else if chosen > 0 {
			query.Limit = chosen
		}
		page, err := queries.ListAdoptions(request.Context(), query.Tenant, query.Limit, request.URL.RawQuery)
		if err != nil {
			var malformed *cataloguepage.MalformedQuery
			if errors.As(err, &malformed) {
				writePageQueryProblem(response, err)
				return
			}
			slog.ErrorContext(request.Context(), "warehouse charge adoption list did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		items := make([]warehouseAdoptionRowBody, 0, len(page.Rows))
		for _, row := range page.Rows {
			items = append(items, warehouseAdoptionRowBody{
				OwnerAccount:  row.OwnerAccount,
				WarehouseCode: row.WarehouseCode,
				Revision:      row.Revision,
				EffectiveAt:   rfc3339(row.EffectiveAt),
				Account:       row.Account,
				PricingScope:  row.PricingScope,
				Basis:         row.Basis,
				Registrant:    row.Registrant,
			})
		}
		writeJSON(response, http.StatusOK, warehouseAdoptionListResponse{
			Outcome:   "WAREHOUSE_ADOPTIONS_LISTED",
			Adoptions: items,
			Page:      page.Page(query.Limit),
		})
	})
}

// NewQueryWarehouseChargeRequestsEndpoint 交回运营分页查阅仓储 SELL 计费请求的入口（GET /warehouse-fulfillment-charge-requests）。
func NewQueryWarehouseChargeRequestsEndpoint(intake CatalogueIntake, queries WarehouseChargeRequestQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		if chosen, err := cataloguepage.ChosenPageSize(request.URL.RawQuery); err != nil {
			writePageQueryProblem(response, err)
			return
		} else if chosen > 0 {
			query.Limit = chosen
		}
		page, err := queries.ListRequests(request.Context(), query.Tenant, query.Limit, request.URL.RawQuery)
		if err != nil {
			var malformed *cataloguepage.MalformedQuery
			if errors.As(err, &malformed) {
				writePageQueryProblem(response, err)
				return
			}
			slog.ErrorContext(request.Context(), "warehouse charge request list did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		items := make([]warehouseChargeRequestRowBody, 0, len(page.Rows))
		for _, row := range page.Rows {
			basis := ""
			if !row.PricingBasisAt.IsZero() {
				basis = rfc3339(row.PricingBasisAt)
			}
			items = append(items, warehouseChargeRequestRowBody{
				OccurrenceID:       row.OccurrenceID,
				Version:            row.Version,
				Status:             row.Status,
				ChargeID:           row.ChargeID,
				EvaluationID:       row.EvaluationID,
				RequestID:          row.RequestID,
				Account:            row.Account,
				SettlementCurrency: row.SettlementCurrency,
				PricingScope:       row.PricingScope,
				PricingBasisAt:     basis,
			})
		}
		writeJSON(response, http.StatusOK, warehouseChargeRequestListResponse{
			Outcome:  "WAREHOUSE_CHARGE_REQUESTS_LISTED",
			Requests: items,
			Page:     page.Page(query.Limit),
		})
	})
}

type outboundRequestListResponse struct {
	Outcome  string                   `json:"outcome"`
	Requests []outboundRequestRowBody `json:"requests"`
	Page     cataloguepage.Page       `json:"page"`
}

type outboundRequestRowBody struct {
	RequestID                string                    `json:"requestId"`
	DocumentNumber           string                    `json:"documentNumber,omitempty"`
	NumberingAbsence         string                    `json:"numberingAbsence,omitempty"`
	OwnerAccount             string                    `json:"ownerAccount"`
	WarehouseCode            string                    `json:"warehouseCode"`
	Origin                   string                    `json:"origin"`
	RequestKey               string                    `json:"requestKey"`
	Dispatch                 string                    `json:"dispatch"`
	Service                  string                    `json:"service,omitempty"`
	Recipient                string                    `json:"recipient"`
	Address                  string                    `json:"address"`
	Basis                    string                    `json:"basis,omitempty"`
	Operator                 string                    `json:"operator,omitempty"`
	Status                   string                    `json:"status"`
	ParentRequestID          string                    `json:"parentRequestId,omitempty"`
	PlatformStocking         bool                      `json:"platformStocking"`
	PartialShipment          string                    `json:"partialShipment,omitempty"`
	PrefetchShipmentID       string                    `json:"prefetchShipmentId,omitempty"`
	PrefetchDeclaredParcelID string                    `json:"prefetchDeclaredParcelId,omitempty"`
	ReceivedAt               string                    `json:"receivedAt"`
	Lines                    []outboundRequestLineBody `json:"lines"`
	ReservedQuantity         int64                     `json:"reservedQuantity"`
	WantedQuantity           int64                     `json:"wantedQuantity"`
	// AllocationException 只在这张请求仍是已预占、最近一次放波次时分配不出被跳过时带。
	AllocationException *allocationExceptionBody `json:"allocationException,omitempty"`
}

type allocationExceptionBody struct {
	// Source 是 WAVE（由人放的波次）或 RULE（按规则成波的一次执行），Reference 是波次号或执行号。
	Source     string `json:"source"`
	Reference  string `json:"reference"`
	RecordedAt string `json:"recordedAt"`
}

func allocationExceptionBodyOf(row *ports.AllocationExceptionRow) *allocationExceptionBody {
	if row == nil {
		return nil
	}
	return &allocationExceptionBody{Source: row.Source, Reference: row.Reference, RecordedAt: rfc3339(row.RecordedAt)}
}

type outboundRequestLineBody struct {
	GoodsCode  string `json:"goodsCode"`
	Unit       string `json:"unit"`
	StockState string `json:"stockState"`
	Quantity   int64  `json:"quantity"`
}

type waveListResponse struct {
	Outcome string             `json:"outcome"`
	Waves   []waveRowBody      `json:"waves"`
	Page    cataloguepage.Page `json:"page"`
}

type waveResponse struct {
	Outcome string       `json:"outcome"`
	Wave    *waveRowBody `json:"wave,omitempty"`
}

type waveRowBody struct {
	WaveID        string `json:"waveId"`
	WarehouseCode string `json:"warehouseCode"`
	Operator      string `json:"operator"`
	Prefetch      bool   `json:"prefetch"`
	Status        string `json:"status"`
	ReleasedAt    string `json:"releasedAt"`
	// SingleUnit 是单品单件波次：每张出库单要拣的都只有一件。
	SingleUnit bool `json:"singleUnit"`
	// BatchPick 是合并拣货的波次：货合在一起拿，拿回来再按出库单分拣。
	BatchPick bool `json:"batchPick"`
	// RuleCode 是放出这个波次的成波规则；由人点单放的波次不带。
	RuleCode string `json:"ruleCode,omitempty"`
	// Assignee 是这个波次派给了谁拣（操作者标识）；没派不带。
	Assignee string `json:"assignee,omitempty"`
	// ZoneAssignees 是单独派了人的库区；没单独派的库区照 assignee 走。一个都没有不带。
	ZoneAssignees []waveZoneAssigneeBody `json:"zoneAssignees,omitempty"`
	Requests      []string               `json:"requests"`
	// WithdrawnRequests 是 requests 里已经撤出的那几张。
	WithdrawnRequests []string       `json:"withdrawnRequests"`
	Tasks             []waveTaskBody `json:"tasks"`
}

type waveZoneAssigneeBody struct {
	ZoneCode string `json:"zoneCode"`
	Assignee string `json:"assignee"`
}

type waveTaskBody struct {
	TaskID    string `json:"taskId"`
	RequestID string `json:"requestId"`
	// OwnerAccount 是这条任务所属出库请求的货主。
	OwnerAccount string `json:"ownerAccount"`
	LocationCode string `json:"locationCode"`
	// ZoneCode 是这条任务所在库位此刻所属的库区；认不出不带。
	ZoneCode        string `json:"zoneCode,omitempty"`
	StagingLocation string `json:"stagingLocation,omitempty"`
	GoodsCode       string `json:"goodsCode"`
	Unit            string `json:"unit"`
	StockState      string `json:"stockState"`
	Quantity        int64  `json:"quantity"`
	Picked          int64  `json:"picked"`
	Status          string `json:"status"`
}

type chargeOccurrenceListResponse struct {
	Outcome     string                    `json:"outcome"`
	Occurrences []chargeOccurrenceRowBody `json:"occurrences"`
	Page        cataloguepage.Page        `json:"page"`
}

type chargeOccurrenceRowBody struct {
	OccurrenceID string                  `json:"occurrenceId"`
	Version      int                     `json:"version"`
	Supersedes   int                     `json:"supersedes,omitempty"`
	Correction   string                  `json:"correction"`
	Kind         string                  `json:"kind"`
	OwnerAccount string                  `json:"ownerAccount"`
	Warehouse    string                  `json:"warehouseCode"`
	SourceFact   string                  `json:"sourceFact"`
	BusinessTime string                  `json:"businessTime"`
	WeightUnit   string                  `json:"weightUnit,omitempty"`
	VolumeUnit   string                  `json:"volumeUnit,omitempty"`
	Measures     []ports.ChargeMeasure   `json:"measures"`
	Attributes   []ports.ChargeAttribute `json:"attributes"`
	LineCount    int                     `json:"lineCount"`
}

type warehouseAdoptionListResponse struct {
	Outcome   string                     `json:"outcome"`
	Adoptions []warehouseAdoptionRowBody `json:"adoptions"`
	Page      cataloguepage.Page         `json:"page"`
}

type warehouseAdoptionRowBody struct {
	OwnerAccount  string `json:"ownerAccount"`
	WarehouseCode string `json:"warehouseCode"`
	Revision      int    `json:"revision"`
	EffectiveAt   string `json:"effectiveAt"`
	Account       string `json:"account"`
	PricingScope  string `json:"pricingScope"`
	Basis         string `json:"basis"`
	Registrant    string `json:"registrant"`
}

type warehouseChargeRequestListResponse struct {
	Outcome  string                          `json:"outcome"`
	Requests []warehouseChargeRequestRowBody `json:"requests"`
	Page     cataloguepage.Page              `json:"page"`
}

type warehouseChargeRequestRowBody struct {
	OccurrenceID       string `json:"occurrenceId"`
	Version            int    `json:"version"`
	Status             string `json:"status"`
	ChargeID           string `json:"chargeId,omitempty"`
	EvaluationID       string `json:"evaluationId,omitempty"`
	RequestID          string `json:"requestId,omitempty"`
	Account            string `json:"account,omitempty"`
	SettlementCurrency string `json:"settlementCurrency,omitempty"`
	PricingScope       string `json:"pricingScope,omitempty"`
	PricingBasisAt     string `json:"pricingBasisAt,omitempty"`
}
