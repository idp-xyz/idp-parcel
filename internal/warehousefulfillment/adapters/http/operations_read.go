package warehousehttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
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

// WaveQueries 是运营查阅波次列表的读口。
type WaveQueries interface {
	ListWaves(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[ports.WaveRow], error)
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
				PrefetchShipmentID:       row.PrefetchShipmentID,
				PrefetchDeclaredParcelID: row.PrefetchDeclaredParcelID,
				ReceivedAt:               rfc3339(row.ReceivedAt),
				Lines:                    lines,
				ReservedQuantity:         row.ReservedQuantity,
				WantedQuantity:           row.WantedQuantity,
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
		page, err := queries.ListWaves(request.Context(), query.Tenant, query.Limit, pageQuery)
		if err != nil {
			slog.ErrorContext(request.Context(), "wave list did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		items := make([]waveRowBody, 0, len(page.Rows))
		for _, row := range page.Rows {
			tasks := make([]waveTaskBody, 0, len(row.Tasks))
			for _, task := range row.Tasks {
				tasks = append(tasks, waveTaskBody{
					TaskID: task.TaskID, RequestID: task.RequestID, LocationCode: task.LocationCode,
					StagingLocation: task.StagingLocation, GoodsCode: task.GoodsCode, Unit: task.Unit,
					StockState: task.StockState, Quantity: task.Quantity, Picked: task.Picked, Status: task.Status,
				})
			}
			requests := row.Requests
			if requests == nil {
				requests = []string{}
			}
			items = append(items, waveRowBody{
				WaveID: row.WaveID, WarehouseCode: row.WarehouseCode, Operator: row.Operator,
				Prefetch: row.Prefetch, ReleasedAt: rfc3339(row.ReleasedAt),
				Requests: requests, Tasks: tasks,
			})
		}
		writeJSON(response, http.StatusOK, waveListResponse{
			Outcome: "WAVES_LISTED",
			Waves:   items,
			Page:    page.Page(query.Limit),
		})
	})
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
	PrefetchShipmentID       string                    `json:"prefetchShipmentId,omitempty"`
	PrefetchDeclaredParcelID string                    `json:"prefetchDeclaredParcelId,omitempty"`
	ReceivedAt               string                    `json:"receivedAt"`
	Lines                    []outboundRequestLineBody `json:"lines"`
	ReservedQuantity         int64                     `json:"reservedQuantity"`
	WantedQuantity           int64                     `json:"wantedQuantity"`
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

type waveRowBody struct {
	WaveID        string         `json:"waveId"`
	WarehouseCode string         `json:"warehouseCode"`
	Operator      string         `json:"operator"`
	Prefetch      bool           `json:"prefetch"`
	ReleasedAt    string         `json:"releasedAt"`
	Requests      []string       `json:"requests"`
	Tasks         []waveTaskBody `json:"tasks"`
}

type waveTaskBody struct {
	TaskID          string `json:"taskId"`
	RequestID       string `json:"requestId"`
	LocationCode    string `json:"locationCode"`
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
