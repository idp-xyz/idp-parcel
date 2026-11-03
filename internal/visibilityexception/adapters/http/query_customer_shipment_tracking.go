package visibilityhttp

import (
	"net/http"
	"net/url"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 按运单号查询的整单四格（ADR-0356 决定二），按客户系统的续办分（ADR-0029）：
//   - `PARCELS_LISTED`：委托已接受（含其后派生的已取消、已完成），带 `parcels` 与 `page`，逐件看。
//   - `AWAITING_ACCEPTANCE`：委托已提交、接受或拒绝未决定，只有 `outcome`，稍后再查。不是「已受理」。
//   - `CLOSED_WITHOUT_SERVICE`：委托已拒绝或已撤回，只有 `outcome`，这个号不会有可追踪的包裹。
//   - `SHIPMENT_NOT_FOUND`：本租户请求方账户名下没有这个号——查无、属别的账户、别的租户、委托没有单据号——同一格同一副字节，核对号。
//
// 四格与单件三格是两套词，各属一口：一个端点一份成功响应形状（ADR-0356 决定一）。
const (
	outcomeParcelsListed        = "PARCELS_LISTED"
	outcomeAwaitingAcceptance   = "AWAITING_ACCEPTANCE"
	outcomeClosedWithoutService = "CLOSED_WITHOUT_SERVICE"
	outcomeShipmentNotFound     = "SHIPMENT_NOT_FOUND"
)

// shipmentTrackingPages 是本口在 ADR-0144 共用翻页件上的声明。只用它的游标：本口不开排序、筛选与 q（ADR-0356 决定三），那几个键在
// Intake 的封闭查询串上就答坏请求，走不到这里。
//
// 游标编进本次的号与末件位置：号是册子选择器，取值进游标摘要，拿这个号的游标去翻别的号答坏请求，这一判不读库；末件位置是
// parcel-shipment 给的位次，排序维取名 position 只住在不透明的游标里，不是答复体的一格。位次一经给出不变，所以按位次续读不跳件，
// 行标识（包裹引用）只因共用件要求每行有决胜键而在。
var shipmentTrackingPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "customer-shipment-tracking",
	Sorts:       []cataloguepage.SortDimension{{Name: "position", Kind: cataloguepage.Integer}},
	DefaultSort: cataloguepage.Sort{Field: "position"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Selectors:   []string{shipmentTrackingNumberParameter},
})

// NewQueryCustomerShipmentTrackingEndpoint 交回 UC-VE-008 按委托的单据号查询的 HTTP 入口（ADR-0356）。首方客户族的口，4xx/5xx 写
// RFC 9457 问题详情（ADR-0140 决定三）；认证与授予各格与按包裹查同形（writeCustomerQueryIntakeProblem）。
//
// 先问整单，列出时本页每一件走与按包裹查同一个单件判据（application.TrackCustomerShipment）。坏请求——含解不开或与本次号不符的
// 游标——在读委托之前答 400；任一读不动整页答 500 NO_ANSWER_FORMED，不出半页（ADR-0356 决定二）。查询不触发派生、披露或通知。
func NewQueryCustomerShipmentTrackingEndpoint(
	intake ShipmentTrackingIntake,
	shipments ports.CustomerShipmentTrackingView,
	views TrackingViewReader,
	attributions ports.ParcelCustomerAccountView,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		query, err := intake.IntakeShipmentTrackingQuery(request.Context(), request)
		if err != nil {
			writeCustomerQueryIntakeProblem(response, request, err)
			return
		}
		page, afterPosition, ok := decodeShipmentTrackingPage(request, query)
		if !ok {
			httpapi.WriteProblem(response, request, http.StatusBadRequest, codeMalformedRequest)
			return
		}

		tracked, err := application.TrackCustomerShipment(request.Context(), shipments, views, attributions, application.CustomerShipmentTrackingQuery{
			Tenant:   query.Tenant,
			Customer: query.Customer,
			Number:   query.Number,
			Page:     ports.ShipmentParcelPage{AfterPosition: afterPosition, Limit: query.PageSize},
		})
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		switch tracked.Standing {
		case ports.ShipmentAwaitingAcceptance:
			writeJSON(response, http.StatusOK, shipmentOutcomeResponse{Outcome: outcomeAwaitingAcceptance})
		case ports.ShipmentClosedWithoutService:
			writeJSON(response, http.StatusOK, shipmentOutcomeResponse{Outcome: outcomeClosedWithoutService})
		case ports.ShipmentNotFound:
			writeJSON(response, http.StatusOK, shipmentOutcomeResponse{Outcome: outcomeShipmentNotFound})
		case ports.ShipmentParcelsListed:
			body, formed := newShipmentParcelsResponse(tracked, page, query.PageSize)
			if !formed {
				httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
				return
			}
			writeJSON(response, http.StatusOK, body)
		default:
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
		}
	})
}

// decodeShipmentTrackingPage 解本次的翻页：号取 Intake 立好的那一个（与查询键同源，游标因此绑在真正被查的号上），after 取查询串
// 原样。游标解不开、与本次号不符、位次不是正整数，都答坏请求；伪造的位次在这里拦下，不带进读。
func decodeShipmentTrackingPage(request *http.Request, query ShipmentTrackingQuery) (cataloguepage.Query, int, bool) {
	parameters, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return cataloguepage.Query{}, 0, false
	}
	values := url.Values{shipmentTrackingNumberParameter: {query.Number.String()}}
	if after, given := parameters[shipmentTrackingAfterParameter]; given {
		values[shipmentTrackingAfterParameter] = after
	}
	page, err := shipmentTrackingPages.Decode(values)
	if err != nil {
		return cataloguepage.Query{}, 0, false
	}
	if page.After == nil {
		return page, 0, true
	}
	position, err := cataloguepage.ParseInteger(page.After.Value)
	if err != nil || position < 1 || int64(int(position)) != position {
		return cataloguepage.Query{}, 0, false
	}
	return page, int(position), true
}

// shipmentOutcomeResponse 是待接受、没有服务、查无此单三格的形状：只有 outcome。
type shipmentOutcomeResponse struct {
	Outcome string `json:"outcome"`
}

// shipmentParcelsResponse 是逐件列出那一格的形状。parcels 恒在场，翻过末件的一页答空数组，不省键；page 照 ADR-0144 决定五，total
// 是这份委托此刻的当前有效包裹件数。
type shipmentParcelsResponse struct {
	Outcome string             `json:"outcome"`
	Parcels []listedParcelBody `json:"parcels"`
	Page    cataloguepage.Page `json:"page"`
}

// listedParcelBody 是列出的一件（ADR-0356 决定三）。parcelId 就是按包裹查用的那个号；customerParcelReference 与首方提交答复同名同值，
// 没声明的件整键缺席；replaces 只在拆分合并形成的后继上出现。outcome 与 view 是嵌进来的 queryResponse，与按包裹查经同一个
// newParcelTrackingBody 写出，两口逐字相同靠的是同一份结构，不是两份对齐的抄本。
type listedParcelBody struct {
	ParcelID                string   `json:"parcelId"`
	CustomerParcelReference string   `json:"customerParcelReference,omitempty"`
	Replaces                []string `json:"replaces,omitempty"`
	queryResponse
}

func newShipmentParcelsResponse(tracked application.CustomerShipmentTracking, page cataloguepage.Query, pageSize int) (shipmentParcelsResponse, bool) {
	parcels := make([]listedParcelBody, 0, len(tracked.Parcels))
	for _, parcel := range tracked.Parcels {
		body, formed := newParcelTrackingBody(parcel.Tracking)
		if !formed {
			return shipmentParcelsResponse{}, false
		}
		listed := listedParcelBody{ParcelID: parcel.Parcel.String(), queryResponse: body}
		if parcel.HasCustomerParcelReference {
			listed.CustomerParcelReference = parcel.CustomerParcelReference.String()
		}
		for _, source := range parcel.Replaces {
			listed.Replaces = append(listed.Replaces, source.String())
		}
		parcels = append(parcels, listed)
	}

	next := ""
	if tracked.More {
		if len(tracked.Parcels) == 0 {
			// 说还有下一页却一件没给：游标没有末件可指，照实答未形成，不编一个从头再来的游标。
			return shipmentParcelsResponse{}, false
		}
		last := tracked.Parcels[len(tracked.Parcels)-1]
		cursor, err := page.CursorAfter(cataloguepage.Position{
			Value:    cataloguepage.FormatInteger(int64(last.Position)),
			Identity: []string{last.Parcel.String()},
		})
		if err != nil {
			return shipmentParcelsResponse{}, false
		}
		next = cursor
	}
	return shipmentParcelsResponse{
		Outcome: outcomeParcelsListed,
		Parcels: parcels,
		Page:    cataloguepage.NewPage(pageSize, next, int64(tracked.Total)),
	}, true
}
