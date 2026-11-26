package warehousehttp

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// IntakeSort 先认证再读分拣的载荷。分拣挂拣货这一种作业事实：它是合并拣货的后一半——货合着拿回来，分进各张单的箱
// 才算拣到了单上——能拣货的人就能分拣，租户不必为它另登一种准入。
func (intake *PickIntake) IntakeSort(ctx context.Context, request *http.Request) (application.SortCommand, error) {
	tenant, reference, err := admitWarehouseWork(ctx, request, intake.authenticator)
	if err != nil {
		return application.SortCommand{}, err
	}
	command, err := registrationjson.SortCommand(requestBody(request))
	if err != nil {
		return application.SortCommand{}, asMalformed(err)
	}
	if reference != command.Warehouse.String() {
		return application.SortCommand{}, &malformedRequest{reason: httpapi.HeaderWorkScopeReference + " must equal warehouse"}
	}
	command.Tenant = tenant
	return command, nil
}

func (UnconfiguredPickIntake) IntakeSort(context.Context, *http.Request) (application.SortCommand, error) {
	return application.SortCommand{}, ErrAccessChannelNotConfigured
}

// NewRecordSortEndpoint 交回分拣投放的入口（POST /warehouse-fulfillment-sort-puts）。
func NewRecordSortEndpoint(intake interface {
	IntakeSort(context.Context, *http.Request) (application.SortCommand, error)
}, waves WaveService) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeSort(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := waves.Sort(request.Context(), command)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeWaveAnswer(response, request, result)
	})
}

// SortPlanQueries 读一个波次的分拣情况。
type SortPlanQueries interface {
	LoadSortPlan(ctx context.Context, tenant domain.TenantID, wave string) (ports.SortPlan, bool, error)
}

// NewQuerySortPlanEndpoint 交回按波次号读分拣情况的入口（GET /warehouse-fulfillment-wave-sort?waveId=...）。
// 没有这个波次是一种答案，不是出错：答 WAVE_NOT_FOUND。波次不用分拣（逐单拣或单品单件）答 SORT_NOT_REQUIRED，不带请求。
func NewQuerySortPlanEndpoint(intake CatalogueIntake, queries SortPlanQueries) http.Handler {
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
		plan, found, err := queries.LoadSortPlan(request.Context(), query.Tenant, waveID)
		if err != nil {
			slog.ErrorContext(request.Context(), "sort plan read did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		if !found {
			writeJSON(response, http.StatusOK, sortPlanResponse{Outcome: "WAVE_NOT_FOUND"})
			return
		}
		body := sortPlanResponse{
			Outcome: "SORT_PLAN_FOUND", WaveID: plan.Wave, WarehouseCode: plan.Warehouse, Status: plan.Status,
			Requests: make([]sortPlanRequestBody, 0, len(plan.Requests)),
		}
		if !plan.Sorts {
			body.Outcome = "SORT_NOT_REQUIRED"
		}
		for _, item := range plan.Requests {
			lines := make([]sortPlanLineBody, 0, len(item.Lines))
			for _, line := range item.Lines {
				lines = append(lines, sortPlanLineBody{GoodsCode: line.Goods, Unit: line.Unit, Picked: line.Picked, Sorted: line.Sorted, Open: line.Open})
			}
			totes := item.Totes
			if totes == nil {
				totes = []string{}
			}
			body.Requests = append(body.Requests, sortPlanRequestBody{
				RequestID: item.Request, DocumentNumber: item.DocumentNumber, OwnerAccount: item.Owner, Status: item.Status,
				Totes: totes, Lines: lines,
			})
		}
		writeJSON(response, http.StatusOK, body)
	})
}

type sortPlanResponse struct {
	Outcome       string                `json:"outcome"`
	WaveID        string                `json:"waveId,omitempty"`
	WarehouseCode string                `json:"warehouseCode,omitempty"`
	Status        string                `json:"status,omitempty"`
	Requests      []sortPlanRequestBody `json:"requests,omitempty"`
}

type sortPlanRequestBody struct {
	RequestID      string `json:"requestId"`
	DocumentNumber string `json:"documentNumber,omitempty"`
	OwnerAccount   string `json:"ownerAccount"`
	Status         string `json:"status"`
	// Totes 是这张出库请求此刻绑着的周转箱；还没有箱是空数组。
	Totes []string           `json:"totes"`
	Lines []sortPlanLineBody `json:"lines"`
}

type sortPlanLineBody struct {
	GoodsCode string `json:"goodsCode"`
	Unit      string `json:"unit"`
	// Picked 是拣出来的件数，Sorted 是已经投放的件数，Open 是拣货的人还没拿回来的件数。都按基本单位。
	Picked int64 `json:"picked"`
	Sorted int64 `json:"sorted"`
	Open   int64 `json:"open"`
}
