package warehousehttp

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// WorkspaceQueries 是仓配工作台、规则总览与各模块明细/台账读口。
type WorkspaceQueries interface {
	SettingsOverview(ctx context.Context, tenant domain.TenantID) (ports.SettingsOverview, error)
	OutboundRequestDetail(ctx context.Context, tenant domain.TenantID, requestID string) (ports.OutboundRequestDetail, bool, error)
	ListFreezes(ctx context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[ports.StockFreezeListRow], error)
	ListCounts(ctx context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[ports.StockCountRow], error)
	CountDetail(ctx context.Context, tenant domain.TenantID, countID string) (ports.StockCountDetail, bool, error)
	ListAdjustments(ctx context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[ports.StockAdjustmentRow], error)
	ListJobDiscrepancies(ctx context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[ports.JobDiscrepancyRow], error)
	ListInstructions(ctx context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[ports.InstructionRow], error)
	ReturnsOverview(ctx context.Context, tenant domain.TenantID) (ports.ReturnsOverview, error)
	ReadReturns(ctx context.Context, tenant domain.TenantID, query ports.ReturnsQuery) (ports.ReturnsPage, error)
	AgentOverview(ctx context.Context, tenant domain.TenantID, now time.Time) (ports.AgentOverview, error)
}

func optRFC3339(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return rfc3339(*t)
}

// NewQuerySettingsOverviewEndpoint 返回租户全部仓配规则的当前生效配置（GET /warehouse-fulfillment-settings-overview）。
func NewQuerySettingsOverviewEndpoint(intake CatalogueIntake, queries WorkspaceQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		overview, err := queries.SettingsOverview(request.Context(), query.Tenant)
		if err != nil {
			slog.ErrorContext(request.Context(), "warehouse settings overview did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		type allocBody struct {
			Configured   bool   `json:"configured"`
			Revision     int    `json:"revision,omitempty"`
			Operator     string `json:"operator,omitempty"`
			RegisteredAt string `json:"registeredAt,omitempty"`
		}
		type shortageBody struct {
			OwnerAccount string `json:"ownerAccount"`
			Revision     int    `json:"revision"`
			Kind         string `json:"kind,omitempty"`
			// PartialShipment 是拣不齐时怎么发的默认；没登记就不带这一格。
			PartialShipment string `json:"partialShipment,omitempty"`
			Operator        string `json:"operator"`
			RegisteredAt    string `json:"registeredAt"`
		}
		type dutyBody struct {
			Configured       bool   `json:"configured"`
			Revision         int    `json:"revision,omitempty"`
			DistinctSubjects bool   `json:"distinctSubjects,omitempty"`
			Operator         string `json:"operator,omitempty"`
			RegisteredAt     string `json:"registeredAt,omitempty"`
		}
		type disposalBody struct {
			WarehouseCode      string   `json:"warehouseCode"`
			Revision           int      `json:"revision"`
			ClaimWindowSeconds int64    `json:"claimWindowSeconds"`
			DisposalMethods    []string `json:"disposalMethods"`
			Registrant         string   `json:"registrant"`
			RegisteredAt       string   `json:"registeredAt"`
		}
		type unitsBody struct {
			WarehouseCode string `json:"warehouseCode"`
			Revision      int    `json:"revision"`
			WeightUnit    string `json:"weightUnit"`
			VolumeUnit    string `json:"volumeUnit"`
			RegisteredAt  string `json:"registeredAt"`
		}
		type agingBody struct {
			WarehouseCode string `json:"warehouseCode"`
			Basis         string `json:"basis"`
			RegisteredAt  string `json:"registeredAt"`
		}
		type projectBody struct {
			WarehouseCode string `json:"warehouseCode"`
			ProjectName   string `json:"projectName"`
			RegisteredAt  string `json:"registeredAt"`
		}
		type connectorBody struct {
			WarehouseCode string `json:"warehouseCode"`
			RegisteredAt  string `json:"registeredAt"`
		}

		alloc := allocBody{}
		if overview.AllocationStrategy != nil {
			alloc = allocBody{
				Configured:   true,
				Revision:     overview.AllocationStrategy.Revision,
				Operator:     overview.AllocationStrategy.Operator,
				RegisteredAt: rfc3339(overview.AllocationStrategy.RegisteredAt),
			}
		}
		duty := dutyBody{}
		if overview.ApprovalDuty != nil {
			duty = dutyBody{
				Configured:       true,
				Revision:         overview.ApprovalDuty.Revision,
				DistinctSubjects: overview.ApprovalDuty.DistinctSubjects,
				Operator:         overview.ApprovalDuty.Operator,
				RegisteredAt:     rfc3339(overview.ApprovalDuty.RegisteredAt),
			}
		}
		shortages := make([]shortageBody, 0, len(overview.ShortagePolicies))
		for _, item := range overview.ShortagePolicies {
			shortages = append(shortages, shortageBody{
				OwnerAccount: item.OwnerAccount, Revision: item.Revision, Kind: item.Kind, PartialShipment: item.PartialShipment,
				Operator: item.Operator, RegisteredAt: rfc3339(item.RegisteredAt),
			})
		}
		disposals := make([]disposalBody, 0, len(overview.DisposalRules))
		for _, item := range overview.DisposalRules {
			methods := item.DisposalMethods
			if methods == nil {
				methods = []string{}
			}
			disposals = append(disposals, disposalBody{
				WarehouseCode: item.WarehouseCode, Revision: item.Revision,
				ClaimWindowSeconds: item.ClaimWindowSeconds, DisposalMethods: methods,
				Registrant: item.Registrant, RegisteredAt: rfc3339(item.RegisteredAt),
			})
		}
		units := make([]unitsBody, 0, len(overview.ChargeUnits))
		for _, item := range overview.ChargeUnits {
			units = append(units, unitsBody{
				WarehouseCode: item.WarehouseCode, Revision: item.Revision,
				WeightUnit: item.WeightUnit, VolumeUnit: item.VolumeUnit, RegisteredAt: rfc3339(item.RegisteredAt),
			})
		}
		agings := make([]agingBody, 0, len(overview.ChargeAging))
		for _, item := range overview.ChargeAging {
			agings = append(agings, agingBody{
				WarehouseCode: item.WarehouseCode, Basis: item.Basis, RegisteredAt: rfc3339(item.RegisteredAt),
			})
		}
		projects := make([]projectBody, 0, len(overview.ChargeProjects))
		for _, item := range overview.ChargeProjects {
			projects = append(projects, projectBody{
				WarehouseCode: item.WarehouseCode, ProjectName: item.ProjectName, RegisteredAt: rfc3339(item.RegisteredAt),
			})
		}
		connectors := make([]connectorBody, 0, len(overview.AgentConnectors))
		for _, item := range overview.AgentConnectors {
			connectors = append(connectors, connectorBody{
				WarehouseCode: item.WarehouseCode, RegisteredAt: rfc3339(item.RegisteredAt),
			})
		}
		// 没登记这条规则的仓不在这一格里：那是未配置，不是「不要求」。
		toteRequirements := make([]map[string]any, 0, len(overview.ToteRequirements))
		for _, item := range overview.ToteRequirements {
			toteRequirements = append(toteRequirements, map[string]any{
				"warehouseCode": item.WarehouseCode, "revision": item.Revision, "required": item.Required,
				"operator": item.Operator, "registeredAt": rfc3339(item.RegisteredAt),
			})
		}
		// 停用的规则也列：它在册，只是不能再按它成波。货主、发运方式没限的不带那一格。
		// 带了 page 或 pageSize 才按页交回；都不带则整份交回，规则设置页的说明要用全部规则。
		rules := overview.WaveRules
		booked, book, reason := waveRuleBook(request, &rules)
		if reason != "" {
			writeMalformed(response, reason)
			return
		}
		waveRules := make([]map[string]any, 0, len(rules))
		for _, item := range rules {
			rule := map[string]any{
				"warehouseCode": item.WarehouseCode, "ruleCode": item.RuleCode, "revision": item.Revision, "name": item.Name,
				"profile": item.Profile, "maxRequests": item.MaxRequests, "batchPick": item.BatchPick, "deactivated": item.Deactivated,
				"operator": item.Operator, "registeredAt": rfc3339(item.RegisteredAt),
			}
			if item.OwnerAccount != "" {
				rule["ownerAccount"] = item.OwnerAccount
			}
			if item.Dispatch != "" {
				rule["dispatch"] = item.Dispatch
			}
			if item.MaxLines > 0 {
				rule["maxLines"] = item.MaxLines
			}
			if item.MaxUnits > 0 {
				rule["maxUnits"] = item.MaxUnits
			}
			if len(item.ReleaseTimes) > 0 {
				rule["releaseTimes"] = item.ReleaseTimes
			}
			waveRules = append(waveRules, rule)
		}
		body := map[string]any{
			"waveRules":          waveRules,
			"toteRequirements":   toteRequirements,
			"outcome":            "SETTINGS_OVERVIEW_LOADED",
			"allocationStrategy": alloc,
			"shortagePolicies":   shortages,
			"approvalDuty":       duty,
			"disposalRules":      disposals,
			"chargeUnits":        units,
			"chargeAging":        agings,
			"chargeProjects":     projects,
			"agentConnectors":    connectors,
		}
		if booked {
			body["waveRuleBook"] = stockBookBody{Page: book.Page, PageSize: book.PageSize, Total: book.Total}
		}
		writeJSON(response, http.StatusOK, body)
	})
}

// NewQueryOutboundRequestDetailEndpoint 返回单张出库请求的 360° 详情（GET /warehouse-fulfillment-outbound-request?requestId=...）。
func NewQueryOutboundRequestDetailEndpoint(intake CatalogueIntake, queries WorkspaceQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		requestID := strings.TrimSpace(request.URL.Query().Get("requestId"))
		if requestID == "" {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		detail, found, err := queries.OutboundRequestDetail(request.Context(), query.Tenant, requestID)
		if err != nil {
			slog.ErrorContext(request.Context(), "outbound request detail did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		if !found {
			writeJSON(response, http.StatusOK, map[string]any{"outcome": "OUTBOUND_NOT_FOUND"})
			return
		}
		lines := make([]map[string]any, 0, len(detail.Lines))
		for _, l := range detail.Lines {
			lines = append(lines, map[string]any{
				"lineIndex": l.LineIndex, "goodsCode": l.GoodsCode, "unit": l.Unit,
				"stockState": l.StockState, "labelAlias": l.LabelAlias,
				"batchNumber": l.BatchNumber, "expiryDate": l.ExpiryDate, "quantity": l.Quantity,
			})
		}
		reservations := make([]map[string]any, 0, len(detail.Reservations))
		for _, r := range detail.Reservations {
			reservations = append(reservations, map[string]any{
				"lineIndex": r.LineIndex, "goodsCode": r.GoodsCode, "stockState": r.StockState,
				"labelAlias": r.LabelAlias, "batchNumber": r.BatchNumber, "expiryDate": r.ExpiryDate,
				"quantity": r.Quantity, "fromUnlabeled": r.FromUnlabeled, "wanted": r.Wanted,
			})
		}
		tasks := make([]map[string]any, 0, len(detail.PickTasks))
		for _, t := range detail.PickTasks {
			tasks = append(tasks, map[string]any{
				"taskId": t.TaskID, "waveId": t.WaveID, "locationCode": t.LocationCode,
				"stagingLocation": t.StagingLocation, "goodsCode": t.GoodsCode, "unit": t.Unit,
				"stockState": t.StockState, "inboundBatchId": t.InboundBatchID,
				"quantity": t.Quantity, "picked": t.Picked, "status": t.Status, "factId": t.FactID,
			})
		}
		checks := make([]map[string]any, 0, len(detail.Checks))
		for _, c := range detail.Checks {
			checks = append(checks, map[string]any{
				"factId": c.FactID, "observed": c.Observed, "matched": c.Matched, "checkedAt": rfc3339(c.CheckedAt),
			})
		}
		parcels := make([]map[string]any, 0, len(detail.Parcels))
		for _, p := range detail.Parcels {
			pLines := make([]map[string]any, 0, len(p.Lines))
			for _, pl := range p.Lines {
				pLines = append(pLines, map[string]any{
					"goodsCode": pl.GoodsCode, "unit": pl.Unit, "quantity": pl.Quantity,
				})
			}
			parcels = append(parcels, map[string]any{
				"parcelSequence": p.ParcelSequence, "packagingMaterial": p.PackagingMaterial,
				"weightValue": p.WeightValue, "weightUnit": p.WeightUnit,
				"lengthValue": p.LengthValue, "widthValue": p.WidthValue, "heightValue": p.HeightValue,
				"dimensionUnit": p.DimensionUnit, "shipmentRequestId": p.ShipmentRequestID,
				"confirmationState": p.ConfirmationState, "cutoffState": p.CutoffState,
				"ownerLabel": p.OwnerLabel, "lines": pLines, "serials": p.Serials,
			})
		}
		confirmations := make([]map[string]any, 0, len(detail.Confirmations))
		for _, c := range detail.Confirmations {
			confirmations = append(confirmations, map[string]any{
				"factId": c.FactID, "parcelSequence": c.ParcelSequence, "kind": c.Kind,
				"offline": c.Offline, "missing": c.Missing, "externalNumber": c.ExternalNumber,
				"pickupParty": c.PickupParty, "pickupCredential": c.PickupCredential,
				"occurredAt": rfc3339(c.OccurredAt), "correctionKind": c.CorrectionKind,
				"correctionStatus": c.CorrectionStatus,
			})
		}
		returnTasks := make([]map[string]any, 0, len(detail.ReturnTasks))
		for _, rt := range detail.ReturnTasks {
			returnTasks = append(returnTasks, map[string]any{
				"returnId": rt.ReturnID, "parcelSequence": rt.ParcelSequence,
				"locationCode": rt.LocationCode, "goodsCode": rt.GoodsCode, "unit": rt.Unit,
				"stockState": rt.StockState, "inboundBatchId": rt.InboundBatchID,
				"quantity": rt.Quantity, "status": rt.Status, "factId": rt.FactID,
			})
		}
		waves := detail.Waves
		if waves == nil {
			waves = []string{}
		}
		req := detail.Request
		writeJSON(response, http.StatusOK, map[string]any{
			"outcome": "OUTBOUND_REQUEST_FOUND",
			"request": map[string]any{
				"requestId": req.RequestID, "documentNumber": req.DocumentNumber, "numberingAbsence": req.NumberingAbsence,
				"ownerAccount": req.OwnerAccount, "warehouseCode": req.WarehouseCode, "origin": req.Origin,
				"requestKey": req.RequestKey, "dispatch": req.Dispatch, "service": req.Service,
				"recipient": req.Recipient, "address": req.Address, "basis": req.Basis, "operator": req.Operator,
				"status": req.Status, "parentRequestId": req.ParentRequestID, "platformStocking": req.PlatformStocking,
				"partialShipment":    req.PartialShipment,
				"prefetchShipmentId": req.PrefetchShipmentID, "prefetchDeclaredParcelId": req.PrefetchDeclaredParcelID,
				"receivedAt": rfc3339(req.ReceivedAt), "reservedQuantity": req.ReservedQuantity, "wantedQuantity": req.WantedQuantity,
			},
			// 只在这张请求仍是已预占、最近一次放波次时分配不出被跳过时有，否则是 null。
			"allocationException": allocationExceptionBodyOf(req.AllocationException),
			"partialShipment":     partialShipmentBodyOf(detail.PartialShipment),
			// 没办过拣不齐的处置时这一格是 null。
			"shortPickDisposal": shortPickDisposalBodyOf(detail.ShortPickDisposal),
			"totes":             outboundTotesBodyOf(detail.Totes),
			"lines":             lines,
			"reservations":      reservations,
			"waves":             waves,
			"pickTasks":         tasks,
			"checks":            checks,
			"parcels":           parcels,
			"confirmations":     confirmations,
			"returnTasks":       returnTasks,
		})
	})
}

// NewQueryFreezesEndpoint 分页列出库存冻结单（GET /warehouse-fulfillment-freezes）。
func NewQueryFreezesEndpoint(intake CatalogueIntake, queries WorkspaceQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		pageQuery, err := ports.StockFreezePages.DecodeRaw(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		if pageQuery.PageSize > 0 {
			query.Limit = pageQuery.PageSize
		}
		page, err := queries.ListFreezes(request.Context(), query.Tenant, query.Limit, pageQuery)
		if err != nil {
			slog.ErrorContext(request.Context(), "freeze list did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		items := make([]map[string]any, 0, len(page.Rows))
		for _, row := range page.Rows {
			items = append(items, map[string]any{
				"freezeId": row.FreezeID, "decisionId": row.DecisionID, "ownerAccount": row.OwnerAccount,
				"warehouseCode": row.WarehouseCode, "target": row.Target, "locationCode": row.LocationCode,
				"goodsCode": row.GoodsCode, "unit": row.Unit, "stockState": row.StockState,
				"labelAlias": row.LabelAlias, "inboundBatchId": row.InboundBatchID, "batchNumber": row.BatchNumber,
				"expiryDate": row.ExpiryDate, "cartonNumber": row.CartonNumber, "quantity": row.Quantity,
				"reason": row.Reason, "operator": row.Operator, "frozenAt": rfc3339(row.FrozenAt),
				"status": row.Status, "unfreezeOperator": row.UnfreezeOperator,
				"unfrozenAt": optRFC3339(row.UnfrozenAt), "unfreezeNote": row.UnfreezeNote,
			})
		}
		writeJSON(response, http.StatusOK, map[string]any{
			"outcome": "FREEZES_LISTED",
			"freezes": items,
			"page":    page.Page(query.Limit),
		})
	})
}

// NewQueryCountsEndpoint 分页列出盘点任务（GET /warehouse-fulfillment-counts）。
func NewQueryCountsEndpoint(intake CatalogueIntake, queries WorkspaceQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		pageQuery, err := ports.StockCountPages.DecodeRaw(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		if pageQuery.PageSize > 0 {
			query.Limit = pageQuery.PageSize
		}
		page, err := queries.ListCounts(request.Context(), query.Tenant, query.Limit, pageQuery)
		if err != nil {
			slog.ErrorContext(request.Context(), "count list did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		items := make([]map[string]any, 0, len(page.Rows))
		for _, row := range page.Rows {
			items = append(items, countRowMap(row))
		}
		writeJSON(response, http.StatusOK, map[string]any{
			"outcome": "COUNTS_LISTED",
			"counts":  items,
			"page":    page.Page(query.Limit),
		})
	})
}

func countRowMap(row ports.StockCountRow) map[string]any {
	locs := row.Locations
	if locs == nil {
		locs = []string{}
	}
	goods := row.Goods
	if goods == nil {
		goods = []string{}
	}
	return map[string]any{
		"countId": row.CountID, "decisionId": row.DecisionID, "warehouseCode": row.WarehouseCode,
		"ownerAccount": row.OwnerAccount, "note": row.Note, "operator": row.Operator,
		"createdAt": rfc3339(row.CreatedAt), "status": row.Status,
		"startedBy": row.StartedBy, "startedAt": optRFC3339(row.StartedAt),
		"completedBy": row.CompletedBy, "completedAt": optRFC3339(row.CompletedAt),
		"voidedBy": row.VoidedBy, "voidedAt": optRFC3339(row.VoidedAt), "voidReason": row.VoidReason,
		"locations": locs, "goods": goods,
		"entryCount": row.EntryCount, "differenceCount": row.DifferenceCount, "pendingDifferences": row.PendingDifferences,
	}
}

// NewQueryCountDetailEndpoint 返回单张盘点任务完整详情（GET /warehouse-fulfillment-count?countId=...）。
func NewQueryCountDetailEndpoint(intake CatalogueIntake, queries WorkspaceQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		countID := strings.TrimSpace(request.URL.Query().Get("countId"))
		if countID == "" {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		detail, found, err := queries.CountDetail(request.Context(), query.Tenant, countID)
		if err != nil {
			slog.ErrorContext(request.Context(), "count detail did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		if !found {
			writeJSON(response, http.StatusOK, map[string]any{"outcome": "COUNT_NOT_FOUND"})
			return
		}
		entries := make([]map[string]any, 0, len(detail.Entries))
		for _, e := range detail.Entries {
			lines := make([]map[string]any, 0, len(e.Lines))
			for _, l := range e.Lines {
				lines = append(lines, map[string]any{
					"ownerAccount": l.OwnerAccount, "goodsCode": l.GoodsCode, "unit": l.Unit,
					"stockState": l.StockState, "labelAlias": l.LabelAlias, "batchNumber": l.BatchNumber,
					"expiryDate": l.ExpiryDate, "quantity": l.Quantity, "observed": l.Observed,
				})
			}
			entries = append(entries, map[string]any{
				"factId": e.FactID, "locationCode": e.LocationCode, "occurredAt": rfc3339(e.OccurredAt),
				"deviceSequence": e.DeviceSequence, "deviceId": e.DeviceID, "operatorId": e.OperatorID,
				"receivedAt": rfc3339(e.ReceivedAt), "lines": lines,
			})
		}
		diffs := make([]map[string]any, 0, len(detail.Differences))
		for _, d := range detail.Differences {
			diffs = append(diffs, map[string]any{
				"differenceId": d.DifferenceID, "countId": d.CountID, "ownerAccount": d.OwnerAccount,
				"warehouseCode": d.WarehouseCode, "locationCode": d.LocationCode, "goodsCode": d.GoodsCode,
				"unit": d.Unit, "stockState": d.StockState, "labelAlias": d.LabelAlias,
				"batchNumber": d.BatchNumber, "expiryDate": d.ExpiryDate,
				"bookQuantity": d.BookQuantity, "countedQuantity": d.CountedQuantity, "status": d.Status,
				"approvedBy": d.ApprovedBy, "approvedAt": optRFC3339(d.ApprovedAt), "approvalNote": d.ApprovalNote,
				"rejectedBy": d.RejectedBy, "rejectedAt": optRFC3339(d.RejectedAt), "rejectReason": d.RejectReason,
			})
		}
		writeJSON(response, http.StatusOK, map[string]any{
			"outcome":     "COUNT_FOUND",
			"count":       countRowMap(detail.Count),
			"entries":     entries,
			"differences": diffs,
		})
	})
}

// NewQueryAdjustmentsEndpoint 分页列出库存调整申请（GET /warehouse-fulfillment-adjustments）。
func NewQueryAdjustmentsEndpoint(intake CatalogueIntake, queries WorkspaceQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		pageQuery, err := ports.StockAdjustmentPages.DecodeRaw(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		if pageQuery.PageSize > 0 {
			query.Limit = pageQuery.PageSize
		}
		page, err := queries.ListAdjustments(request.Context(), query.Tenant, query.Limit, pageQuery)
		if err != nil {
			slog.ErrorContext(request.Context(), "adjustment list did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		items := make([]map[string]any, 0, len(page.Rows))
		for _, row := range page.Rows {
			lines := make([]map[string]any, 0, len(row.Lines))
			for _, l := range row.Lines {
				lines = append(lines, map[string]any{
					"lineIndex": l.LineIndex, "kind": l.Kind, "locationCode": l.LocationCode,
					"goodsCode": l.GoodsCode, "unit": l.Unit, "stockState": l.StockState,
					"labelAlias": l.LabelAlias, "batchNumber": l.BatchNumber, "expiryDate": l.ExpiryDate,
					"inboundBatchId": l.InboundBatchID, "quantity": l.Quantity, "basis": l.Basis,
				})
			}
			items = append(items, map[string]any{
				"adjustmentId": row.AdjustmentID, "ownerAccount": row.OwnerAccount,
				"warehouseCode": row.WarehouseCode, "reason": row.Reason, "operator": row.Operator,
				"submittedAt": rfc3339(row.SubmittedAt), "status": row.Status,
				"approvedBy": row.ApprovedBy, "approvedAt": optRFC3339(row.ApprovedAt), "approvalNote": row.ApprovalNote,
				"rejectedBy": row.RejectedBy, "rejectedAt": optRFC3339(row.RejectedAt), "rejectReason": row.RejectReason,
				"lines": lines,
			})
		}
		writeJSON(response, http.StatusOK, map[string]any{
			"outcome":     "ADJUSTMENTS_LISTED",
			"adjustments": items,
			"page":        page.Page(query.Limit),
		})
	})
}

// NewQueryJobDiscrepanciesEndpoint 分页列出库内作业差异（GET /warehouse-fulfillment-job-discrepancies）。
func NewQueryJobDiscrepanciesEndpoint(intake CatalogueIntake, queries WorkspaceQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		pageQuery, err := ports.JobDiscrepancyPages.DecodeRaw(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		if pageQuery.PageSize > 0 {
			query.Limit = pageQuery.PageSize
		}
		page, err := queries.ListJobDiscrepancies(request.Context(), query.Tenant, query.Limit, pageQuery)
		if err != nil {
			slog.ErrorContext(request.Context(), "job discrepancy list did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		items := make([]map[string]any, 0, len(page.Rows))
		for _, row := range page.Rows {
			items = append(items, map[string]any{
				"discrepancyId": row.DiscrepancyID, "factId": row.FactID, "linePosition": row.LinePosition,
				"ownerAccount": row.OwnerAccount, "warehouseCode": row.WarehouseCode,
				"sourceLocation": row.SourceLocation, "locationCode": row.LocationCode,
				"goodsCode": row.GoodsCode, "unit": row.Unit, "stockState": row.StockState,
				"labelAlias": row.LabelAlias, "batchNumber": row.BatchNumber, "expiryDate": row.ExpiryDate,
				"inboundBatchId": row.InboundBatchID, "cartonNumber": row.CartonNumber, "note": row.Note,
				"quantity": row.Quantity, "shortOnBook": row.ShortOnBook, "heldQuantity": row.HeldQuantity,
				"cartonMissing": row.CartonMissing, "status": row.Status,
				"approvedBy": row.ApprovedBy, "approvedAt": optRFC3339(row.ApprovedAt), "approvalNote": row.ApprovalNote,
				"rejectedBy": row.RejectedBy, "rejectedAt": optRFC3339(row.RejectedAt), "rejectReason": row.RejectReason,
			})
		}
		writeJSON(response, http.StatusOK, map[string]any{
			"outcome":       "JOB_DISCREPANCIES_LISTED",
			"discrepancies": items,
			"page":          page.Page(query.Limit),
		})
	})
}

// NewQueryInstructionsEndpoint 分页列出加工与换标指令（GET /warehouse-fulfillment-instructions）。
func NewQueryInstructionsEndpoint(intake CatalogueIntake, queries WorkspaceQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		pageQuery, err := ports.InstructionPages.DecodeRaw(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		if pageQuery.PageSize > 0 {
			query.Limit = pageQuery.PageSize
		}
		page, err := queries.ListInstructions(request.Context(), query.Tenant, query.Limit, pageQuery)
		if err != nil {
			slog.ErrorContext(request.Context(), "instruction list did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		items := make([]map[string]any, 0, len(page.Rows))
		for _, row := range page.Rows {
			comps := make([]map[string]any, 0, len(row.Components))
			for _, c := range row.Components {
				comps = append(comps, map[string]any{"goodsCode": c.GoodsCode, "quantity": c.Quantity})
			}
			execs := make([]map[string]any, 0, len(row.Executions))
			for _, ex := range row.Executions {
				execs = append(execs, map[string]any{
					"factId": ex.FactID, "deviceId": ex.DeviceID, "operatorId": ex.OperatorID,
					"occurredAt": rfc3339(ex.OccurredAt), "deviceSequence": ex.DeviceSequence,
					"doneQuantity": ex.DoneQuantity, "movementId": ex.MovementID,
				})
			}
			items = append(items, map[string]any{
				"instructionId": row.InstructionID, "ownerAccount": row.OwnerAccount,
				"warehouseCode": row.WarehouseCode, "kind": row.Kind, "status": row.Status, "origin": row.Origin,
				"goodsCode": row.GoodsCode, "unit": row.Unit, "quantity": row.Quantity, "doneQuantity": row.DoneQuantity,
				"fromAlias": row.FromAlias, "toAlias": row.ToAlias, "batchNumber": row.BatchNumber,
				"expiryDate": row.ExpiryDate, "reason": row.Reason, "basis": row.Basis, "operator": row.Operator,
				"fixedRevision": row.FixedRevision, "voidReason": row.VoidReason,
				"voidedAt": optRFC3339(row.VoidedAt), "receivedAt": rfc3339(row.ReceivedAt),
				"components": comps, "executions": execs,
			})
		}
		writeJSON(response, http.StatusOK, map[string]any{
			"outcome":      "INSTRUCTIONS_LISTED",
			"instructions": items,
			"page":         page.Page(query.Limit),
		})
	})
}

// NewQueryReturnsOverviewEndpoint 返回退货预报与退件到仓/处置总览（GET /warehouse-fulfillment-returns-overview）。
func NewQueryReturnsOverviewEndpoint(intake CatalogueIntake, queries WorkspaceQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		asked, err := ports.ParseReturnsQuery(request.URL.Query())
		if err != nil {
			writeMalformed(response, err.Error())
			return
		}
		if asked.Paged() || asked.Keyed() {
			page, err := queries.ReadReturns(request.Context(), query.Tenant, asked)
			if err != nil {
				slog.ErrorContext(request.Context(), "returns page did not form an answer", "err", err)
				writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
				return
			}
			writeReturnsPage(response, page)
			return
		}
		overview, err := queries.ReturnsOverview(request.Context(), query.Tenant)
		if err != nil {
			slog.ErrorContext(request.Context(), "returns overview did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		advices := make([]map[string]any, 0, len(overview.PreAdvices))
		for _, a := range overview.PreAdvices {
			lines := make([]map[string]any, 0, len(a.Lines))
			for _, l := range a.Lines {
				lines = append(lines, map[string]any{"goodsCode": l.GoodsCode, "unit": l.Unit, "quantity": l.Quantity})
			}
			advices = append(advices, map[string]any{
				"adviceId": a.AdviceID, "documentNumber": a.DocumentNumber, "numberingAbsence": a.NumberingAbsence,
				"ownerAccount": a.OwnerAccount, "warehouseCode": a.WarehouseCode, "origin": a.Origin,
				"trackingNumber": a.TrackingNumber, "basis": a.Basis, "operator": a.Operator,
				"receivedAt": rfc3339(a.ReceivedAt), "lines": lines,
			})
		}
		receipts := make([]map[string]any, 0, len(overview.Receipts))
		for _, r := range overview.Receipts {
			item := map[string]any{
				"factId": r.FactID, "warehouseCode": r.WarehouseCode, "locationCode": r.LocationCode,
				"trackingNumber": r.TrackingNumber, "ownerAccount": r.OwnerAccount, "adviceId": r.AdviceID,
				"arrivalId": r.ArrivalID, "goodsCode": r.GoodsCode, "unit": r.Unit, "stockState": r.StockState,
				"quantity": r.Quantity, "outcome": r.Outcome, "occurredAt": rfc3339(r.OccurredAt),
			}
			if r.Disposition != nil {
				item["disposition"] = map[string]any{
					"decisionId": r.Disposition.DecisionID, "destination": r.Disposition.Destination,
					"basis": r.Disposition.Basis, "operator": r.Disposition.Operator,
					"origin": r.Disposition.Origin, "decidedAt": rfc3339(r.Disposition.DecidedAt),
				}
			}
			receipts = append(receipts, item)
		}
		writeJSON(response, http.StatusOK, map[string]any{
			"outcome":    "RETURNS_OVERVIEW_LOADED",
			"preAdvices": advices,
			"receipts":   receipts,
		})
	})
}

func writeReturnsPage(response http.ResponseWriter, page ports.ReturnsPage) {
	advices := make([]map[string]any, 0, len(page.PreAdvices))
	for _, advice := range page.PreAdvices {
		item := adviceJSON(advice)
		if advice.Status != "" {
			item["status"] = advice.Status
		}
		advices = append(advices, item)
	}
	receipts := make([]map[string]any, 0, len(page.Receipts))
	for _, receipt := range page.Receipts {
		receipts = append(receipts, receiptJSON(receipt))
	}
	body := map[string]any{
		"outcome":    "RETURNS_PAGE_LOADED",
		"view":       page.View,
		"preAdvices": advices,
		"receipts":   receipts,
	}
	if page.View == ports.ReturnsViewAdvices || page.View == ports.ReturnsViewReceipts {
		body["page"] = map[string]any{"page": page.Page, "pageSize": page.PageSize, "total": page.Total}
		body["counts"] = page.Counts
	}
	writeJSON(response, http.StatusOK, body)
}

func adviceJSON(advice ports.ReturnPreAdviceRow) map[string]any {
	lines := make([]map[string]any, 0, len(advice.Lines))
	for _, line := range advice.Lines {
		lines = append(lines, map[string]any{"goodsCode": line.GoodsCode, "unit": line.Unit, "quantity": line.Quantity})
	}
	return map[string]any{
		"adviceId": advice.AdviceID, "documentNumber": advice.DocumentNumber, "numberingAbsence": advice.NumberingAbsence,
		"ownerAccount": advice.OwnerAccount, "warehouseCode": advice.WarehouseCode, "origin": advice.Origin,
		"trackingNumber": advice.TrackingNumber, "basis": advice.Basis, "operator": advice.Operator,
		"receivedAt": rfc3339(advice.ReceivedAt), "lines": lines,
	}
}

func receiptJSON(receipt ports.ReturnReceiptRow) map[string]any {
	item := map[string]any{
		"factId": receipt.FactID, "warehouseCode": receipt.WarehouseCode, "locationCode": receipt.LocationCode,
		"trackingNumber": receipt.TrackingNumber, "ownerAccount": receipt.OwnerAccount, "adviceId": receipt.AdviceID,
		"arrivalId": receipt.ArrivalID, "goodsCode": receipt.GoodsCode, "unit": receipt.Unit, "stockState": receipt.StockState,
		"quantity": receipt.Quantity, "outcome": receipt.Outcome, "occurredAt": rfc3339(receipt.OccurredAt),
	}
	if receipt.Disposition != nil {
		item["disposition"] = map[string]any{
			"decisionId": receipt.Disposition.DecisionID, "destination": receipt.Disposition.Destination,
			"basis": receipt.Disposition.Basis, "operator": receipt.Disposition.Operator,
			"origin": receipt.Disposition.Origin, "decidedAt": rfc3339(receipt.Disposition.DecidedAt),
		}
	}
	return item
}

// waveRuleBook 在带了 page 或 pageSize 时按仓筛过再切页。warehouse 单独出现时只筛、不切页。理由不回显取值。
func waveRuleBook(request *http.Request, rules *[]ports.WaveRuleOverview) (bool, ports.Book, string) {
	values, reason := overviewValues(request)
	if reason != "" {
		return false, ports.Book{}, reason
	}
	if warehouse := values.Get("warehouse"); warehouse != "" {
		kept := make([]ports.WaveRuleOverview, 0, len(*rules))
		for _, rule := range *rules {
			if rule.WarehouseCode == warehouse {
				kept = append(kept, rule)
			}
		}
		*rules = kept
	}
	page, size, reason, on := oneBook(values, "page")
	if reason != "" || !on {
		return false, ports.Book{}, reason
	}
	window, book := ports.CutBook(*rules, page, size)
	*rules = window
	return true, book, ""
}

// NewQueryAgentOverviewEndpoint 返回代理仓连接器、库存声明与回执记录（GET /warehouse-fulfillment-agent-overview）。
// 带了 pageSize 或某一张表的页码才按页交回；都不带则三张表整份交回。
func NewQueryAgentOverviewEndpoint(intake CatalogueIntake, queries WorkspaceQueries) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}
		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		overview, err := queries.AgentOverview(request.Context(), query.Tenant, time.Now().UTC())
		if err != nil {
			slog.ErrorContext(request.Context(), "agent overview did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		pages, reason := agentBooks(request, &overview)
		if reason != "" {
			writeMalformed(response, reason)
			return
		}
		connectors := make([]map[string]any, 0, len(overview.Connectors))
		for _, c := range overview.Connectors {
			connectors = append(connectors, map[string]any{
				"warehouseCode": c.WarehouseCode, "registeredAt": rfc3339(c.RegisteredAt),
			})
		}
		statements := make([]map[string]any, 0, len(overview.Statements))
		for _, s := range overview.Statements {
			statements = append(statements, map[string]any{
				"sourceId": s.SourceID, "statedAt": rfc3339(s.StatedAt), "validUntil": rfc3339(s.ValidUntil),
				"warehouseCode": s.WarehouseCode, "ownerAccount": s.OwnerAccount, "goodsCode": s.GoodsCode,
				"unit": s.Unit, "quantity": s.Quantity, "freshness": s.Freshness,
			})
		}
		results := make([]map[string]any, 0, len(overview.Results))
		for _, r := range overview.Results {
			results = append(results, map[string]any{
				"resultId": r.ResultID, "requestId": r.RequestID, "warehouseCode": r.WarehouseCode,
				"kind": r.Kind, "weightValue": r.WeightValue, "weightUnit": r.WeightUnit,
				"lengthValue": r.LengthValue, "widthValue": r.WidthValue, "heightValue": r.HeightValue,
				"dimensionUnit": r.DimensionUnit, "reportedAt": rfc3339(r.ReportedAt),
			})
		}
		body := map[string]any{
			"outcome":    "AGENT_OVERVIEW_LOADED",
			"connectors": connectors,
			"statements": statements,
			"results":    results,
		}
		if pages != nil {
			body["pages"] = pages
		}
		writeJSON(response, http.StatusOK, body)
	})
}

func agentBooks(request *http.Request, overview *ports.AgentOverview) (map[string]stockBookBody, string) {
	values, reason := overviewValues(request)
	if reason != "" {
		return nil, reason
	}
	size, sized, reason := bookSize(values)
	if reason != "" {
		return nil, reason
	}
	connectorsPage, connectorsOn, reason := namedPage(values, "connectorsPage")
	if reason != "" {
		return nil, reason
	}
	statementsPage, statementsOn, reason := namedPage(values, "statementsPage")
	if reason != "" {
		return nil, reason
	}
	resultsPage, resultsOn, reason := namedPage(values, "resultsPage")
	if reason != "" {
		return nil, reason
	}
	if !sized && !connectorsOn && !statementsOn && !resultsOn {
		return nil, ""
	}
	if !sized {
		size = cataloguepage.DefaultPageSize
	}
	if !connectorsOn {
		connectorsPage = 1
	}
	if !statementsOn {
		statementsPage = 1
	}
	if !resultsOn {
		resultsPage = 1
	}
	connectors, connectorsBook := ports.CutBook(overview.Connectors, connectorsPage, size)
	statements, statementsBook := ports.CutBook(overview.Statements, statementsPage, size)
	results, resultsBook := ports.CutBook(overview.Results, resultsPage, size)
	overview.Connectors, overview.Statements, overview.Results = connectors, statements, results
	return map[string]stockBookBody{
		"connectors": {Page: connectorsBook.Page, PageSize: connectorsBook.PageSize, Total: connectorsBook.Total},
		"statements": {Page: statementsBook.Page, PageSize: statementsBook.PageSize, Total: statementsBook.Total},
		"results":    {Page: resultsBook.Page, PageSize: resultsBook.PageSize, Total: resultsBook.Total},
	}, ""
}

func overviewValues(request *http.Request) (url.Values, string) {
	if request.URL.RawQuery == "" {
		return url.Values{}, ""
	}
	values, err := cataloguepage.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return nil, "查询串里有解不开的一段"
	}
	return values, ""
}

func oneBook(values url.Values, pageKey string) (page int, size int, reason string, on bool) {
	size, sized, reason := bookSize(values)
	if reason != "" {
		return 0, 0, reason, false
	}
	page, paged, reason := namedPage(values, pageKey)
	if reason != "" {
		return 0, 0, reason, false
	}
	if !sized && !paged {
		return 0, 0, "", false
	}
	if !sized {
		size = cataloguepage.DefaultPageSize
	}
	if !paged {
		page = 1
	}
	return page, size, "", true
}

func bookSize(values url.Values) (int, bool, string) {
	given, present := values["pageSize"]
	if !present {
		return 0, false, ""
	}
	if len(given) != 1 {
		return 0, true, "pageSize 只能给一次"
	}
	size, err := strconv.Atoi(given[0])
	if err != nil || !stockPageSizeAllowed(size) {
		return 0, true, "pageSize 只接受 20、50、100 或 200"
	}
	return size, true, ""
}

func namedPage(values url.Values, key string) (int, bool, string) {
	given, present := values[key]
	if !present {
		return 0, false, ""
	}
	if len(given) != 1 {
		return 0, true, key + " 只能给一次"
	}
	page, err := strconv.Atoi(given[0])
	if err != nil || page < 1 || page > 1_000_000 {
		return 0, true, key + " 须是从 1 起的整数"
	}
	return page, true, ""
}

// outboundTotesBodyOf 答这张请求绑过的周转箱；还绑着的不带 releasedAt。没绑过是空数组。
func outboundTotesBodyOf(totes []ports.OutboundToteDetail) []map[string]any {
	body := make([]map[string]any, 0, len(totes))
	for _, tote := range totes {
		item := map[string]any{"toteCode": tote.ToteCode, "boundAt": rfc3339(tote.BoundAt)}
		if tote.ReleasedAt != nil {
			item["releasedAt"] = rfc3339(*tote.ReleasedAt)
		}
		body = append(body, item)
	}
	return body
}

// shortPickDisposalBodyOf 答这张请求拣不齐之后办过的那一次处置：照的规则与出处、要发与实发、谁在何时办的。
func shortPickDisposalBodyOf(disposal *ports.ShortPickDisposalDetail) any {
	if disposal == nil {
		return nil
	}
	body := map[string]any{
		"rule": disposal.Rule, "source": disposal.Source, "wanted": disposal.Wanted, "shipped": disposal.Shipped,
		"operator": disposal.Operator, "recordedAt": rfc3339(disposal.RecordedAt),
	}
	if disposal.RemainderRequest != "" {
		body["remainderRequestId"] = disposal.RemainderRequest
	}
	return body
}

// partialShipmentBodyOf 答一张请求此刻适用的部分发货规则。configured 为假是三层都没写明：
// 不填默认，规则与出处两格都不带。各层自己写的照实带出，没写的不带。
func partialShipmentBodyOf(detail ports.PartialShipmentDetail) map[string]any {
	body := map[string]any{"configured": detail.Rule != ""}
	if detail.Rule != "" {
		body["rule"] = detail.Rule
		body["source"] = detail.Source
	}
	if detail.Order != "" {
		body["order"] = detail.Order
	}
	if detail.Owner != "" {
		body["owner"] = detail.Owner
	}
	if detail.Consent != "" {
		body["consent"] = map[string]any{
			"rule": detail.Consent, "basis": detail.ConsentBasis, "operator": detail.ConsentOperator,
			"recordedAt": optRFC3339(detail.ConsentAt),
		}
	}
	return body
}
