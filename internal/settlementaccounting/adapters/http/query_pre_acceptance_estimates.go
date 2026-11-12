package settlementhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// —— 一份委托的受理前估价 ——

// PreAcceptanceEstimateReader 是受理前估价查阅消费的读口。
type PreAcceptanceEstimateReader interface {
	ListPreAcceptanceEstimates(ctx context.Context, tenant domain.TenantID, shipmentRequest string) ([]ports.PreAcceptanceEstimateEntry, error)
}

var _ PreAcceptanceEstimateReader = ports.PreAcceptanceEstimateRead(nil)

// 没有留痕的委托同样是 LISTED、estimates 为空数组：估价没形成（停因在受理任务上）与委托不在册，这一口都答不出，也不该替调用方猜。
const outcomePreAcceptanceEstimatesListed = "PRE_ACCEPTANCE_ESTIMATES_LISTED"

type preAcceptanceEstimatesResponse struct {
	Outcome   string                      `json:"outcome"`
	Estimates []preAcceptanceEstimateBody `json:"estimates"`
}

// preAcceptanceEstimateBody 的金额照本读面的写法：最小币单位的整数串（minorAmount）配币种。费用行是例外，见 estimateChargeLineBody。
type preAcceptanceEstimateBody struct {
	SubmissionVersion string                   `json:"submissionVersion"`
	Form              string                   `json:"form"`
	PricingScope      string                   `json:"pricingScope"`
	PriceCard         estimatePriceCardBody    `json:"priceCard"`
	ServiceProduct    string                   `json:"serviceProduct,omitempty"`
	DetailRecorded    bool                     `json:"detailRecorded"`
	TotalAmount       string                   `json:"totalAmount"`
	Currency          string                   `json:"currency"`
	TrialAt           string                   `json:"trialAt"`
	RecordedAt        string                   `json:"recordedAt"`
	Parcels           []estimateParcelBody     `json:"parcels"`
	Occupations       []estimateOccupationBody `json:"occupations"`
}

type estimatePriceCardBody struct {
	Plan        string `json:"plan"`
	Version     string `json:"version"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// estimateParcelBody 的 pricingWeight 与 chargeLines 缺键即这一件没记（补记之前的行，或那一格没译出来）。
type estimateParcelBody struct {
	Parcel        string                   `json:"parcel"`
	Evaluation    string                   `json:"evaluation"`
	TotalAmount   string                   `json:"totalAmount"`
	Currency      string                   `json:"currency"`
	PricingWeight *estimatePricingWeight   `json:"pricingWeight,omitempty"`
	ChargeLines   []estimateChargeLineBody `json:"chargeLines,omitempty"`
}

type estimatePricingWeight struct {
	Value string `json:"value"`
	Unit  string `json:"unit"`
}

// estimateChargeLineBody 的 amount 是评价的十进制原文、currency 是卡本币：费用行不换算、逐行取整可选，写不成最小币单位
// （domain.PreAcceptanceTrialChargeLine）。
type estimateChargeLineBody struct {
	Code     string `json:"code"`
	Effect   string `json:"effect"`
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type estimateOccupationBody struct {
	Kind            string `json:"kind"`
	ID              string `json:"id"`
	Status          string `json:"status"`
	Amount          string `json:"amount"`
	Currency        string `json:"currency"`
	FormedAt        string `json:"formedAt"`
	ReleasedAt      string `json:"releasedAt,omitempty"`
	ConvertedAmount string `json:"convertedAmount"`
	ConvertedAt     string `json:"convertedAt,omitempty"`
}

// NewQueryPreAcceptanceEstimatesEndpoint 交回一份委托受理前估价查阅的 HTTP 入口（GET /settlement-pre-acceptance-estimates?shipmentRequest=，
// ADR-0379 决定二）：逐提交版本一份估价，连同它的冻结或信用暴露。只对运营；按一份委托取，不按 ADR-0144 翻页。shipmentRequest 缺即
// 坏请求，在准入之后才判（ADR-0055 决定三，同 NewQueryStatementLinesEndpoint）。
//
// 这是受理前估价，不是费用：受理判断的结论与限制原因由 parcel-shipment 在委托详情上交出，本口不带；页面两边各取、自己拼（决定三、四）。
func NewQueryPreAcceptanceEstimatesEndpoint(intake CatalogueQueryIntake, reader PreAcceptanceEstimateReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !guardGet(response, request) {
			return
		}
		query, ok := intakeCatalogueQuery(response, request, intake)
		if !ok {
			return
		}
		shipmentRequest := request.URL.Query().Get("shipmentRequest")
		if shipmentRequest == "" {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		entries, err := reader.ListPreAcceptanceEstimates(request.Context(), query.Scope.Tenant(), shipmentRequest)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		body := preAcceptanceEstimatesResponse{
			Outcome:   outcomePreAcceptanceEstimatesListed,
			Estimates: make([]preAcceptanceEstimateBody, 0, len(entries)),
		}
		for _, entry := range entries {
			body.Estimates = append(body.Estimates, preAcceptanceEstimateBodyOf(entry))
		}
		writeJSON(response, http.StatusOK, body)
	})
}

func preAcceptanceEstimateBodyOf(entry ports.PreAcceptanceEstimateEntry) preAcceptanceEstimateBody {
	body := preAcceptanceEstimateBody{
		SubmissionVersion: entry.SubmissionVersion,
		Form:              entry.Form,
		PricingScope:      entry.PricingScope,
		PriceCard: estimatePriceCardBody{
			Plan: entry.PriceCard.ID, Version: entry.PriceCard.Version, Fingerprint: entry.PriceCard.Fingerprint,
		},
		ServiceProduct: entry.ServiceProduct,
		DetailRecorded: entry.DetailRecorded,
		TotalAmount:    minorAmount(entry.TotalMinor),
		Currency:       entry.Currency,
		TrialAt:        rfc3339(entry.TrialAt),
		RecordedAt:     rfc3339(entry.RecordedAt),
		Parcels:        make([]estimateParcelBody, 0, len(entry.Parcels)),
		// 空数组而不是缺键：受限而没有占用是一个答案，读的人不该先判字段在不在。
		Occupations: make([]estimateOccupationBody, 0, len(entry.Occupations)),
	}
	for _, parcel := range entry.Parcels {
		row := estimateParcelBody{
			Parcel: parcel.Parcel, Evaluation: parcel.Evaluation,
			TotalAmount: minorAmount(parcel.TotalMinor), Currency: parcel.Currency,
		}
		if parcel.PricingWeight != nil {
			row.PricingWeight = &estimatePricingWeight{Value: parcel.PricingWeight.Value, Unit: parcel.PricingWeight.Unit}
		}
		for _, line := range parcel.ChargeLines {
			row.ChargeLines = append(row.ChargeLines, estimateChargeLineBody{
				Code: line.Code, Effect: line.Effect, Amount: line.Amount, Currency: line.Currency,
			})
		}
		body.Parcels = append(body.Parcels, row)
	}
	for _, occupation := range entry.Occupations {
		body.Occupations = append(body.Occupations, estimateOccupationBody{
			Kind:            string(occupation.Kind),
			ID:              occupation.ID,
			Status:          occupation.Status,
			Amount:          minorAmount(occupation.AmountMinor),
			Currency:        occupation.Currency,
			FormedAt:        rfc3339(occupation.FormedAt),
			ReleasedAt:      optionalInstant(occupation.ReleasedAt),
			ConvertedAmount: minorAmount(occupation.ConvertedMinor),
			ConvertedAt:     optionalInstant(occupation.ConvertedAt),
		})
	}
	return body
}
