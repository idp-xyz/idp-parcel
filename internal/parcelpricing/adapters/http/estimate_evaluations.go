package pricinghttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// EstimateIntake 把一次已认证的接入请求翻译成试算命令（UC-PP-001，ADR-0152 决定六）。
//
// 它是接口，理由与回放口同源：租户只从 ADR-0100 的 `OperatorEnvelope` 来，后端不采信自报身份；载荷形状由产品定义，
// 已在 DecodeEstimatePayload。这一口等的是操作者信封接线，与计价回放同批（operator-channel/06），不是渠道契约。
type EstimateIntake interface {
	IntakeEstimate(ctx context.Context, request *http.Request) (application.FormEstimateEvaluationsCommand, error)
}

// EstimateFormer 是本端点转交的试算编排。它不写任何册，没有事务边界可给。
type EstimateFormer interface {
	Handle(ctx context.Context, command application.FormEstimateEvaluationsCommand) (application.FormEstimateEvaluationsResult, error)
}

// NewEstimateEndpoint 交回试算的 HTTP 入口（POST /pricing-estimates；ADR-0152）。
//
// 路径叫 `-estimates`：本上下文这一格的动词是试算，答案代数说的也是试算。用 `POST` 是因为结构化声明放请求体更合适——
// 它不写任何册，能力面按语义归「主数据与运营查阅读」，不按方法归（ADR-0152 决定六与 owner 复核记录）。
func NewEstimateEndpoint(intake EstimateIntake, former EstimateFormer) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		command, err := intake.IntakeEstimate(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}

		result, err := former.Handle(request.Context(), command)
		if err != nil {
			// 结构上到不了的答案才是 error；试算形成与否未知时编排答未决，不走这里（ADR-0022）。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		outcome := result.Outcome.String()
		if outcome == "" {
			writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		// 形成了的答案一律 200：试算不新增任何一行，没有 201 可取；状态码不替 outcome 说话（ADR-0022）。
		writeJSON(response, http.StatusOK, estimateResponseOf(result))
	})
}

// estimateResponse 是试算端点的封闭响应形状。`outcome` 只说编排（ADR-0152 决定七）；逐卡评价自己的状态五格在
// `evaluation.status` 上原样透出，面上不再折一遍。
type estimateResponse struct {
	Outcome    string                  `json:"outcome"`
	Reason     string                  `json:"reason,omitempty"`
	Candidates []estimateCandidateBody `json:"candidates"`
	Conflict   []planReferenceBody     `json:"conflict"`
}

type estimateCandidateBody struct {
	Plan       planReferenceBody       `json:"plan"`
	Answer     string                  `json:"answer"`
	Missing    []string                `json:"missing,omitempty"`
	Evaluation *estimateEvaluationBody `json:"evaluation,omitempty"`
}

type planReferenceBody struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

// estimateEvaluationBody 透出一份试算评价的全部结论：试算不入册、没有事后可去的详情读面，金额、费用行、版本清单与
// 解释只能随这一次响应交回（回放口不透金额，是因为回放评价入册、详情归评价册读面）。
type estimateEvaluationBody struct {
	EvaluationID   string               `json:"evaluationId"`
	Status         string               `json:"status"`
	Evidence       string               `json:"evidence"`
	Direction      string               `json:"direction"`
	Purpose        string               `json:"purpose"`
	SemanticDigest string               `json:"semanticDigest"`
	Total          *moneyBody           `json:"total,omitempty"`
	ChargeLines    []estimateChargeBody `json:"chargeLines"`
	Issues         []estimateIssueBody  `json:"issues"`
	Explanation    []string             `json:"explanation"`
	Manifest       []manifestEntryBody  `json:"manifest"`
}

type moneyBody struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type estimateChargeBody struct {
	Code        string    `json:"code"`
	Description string    `json:"description"`
	Amount      moneyBody `json:"amount"`
}

type estimateIssueBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type manifestEntryBody struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Version string `json:"version"`
}

func estimateResponseOf(result application.FormEstimateEvaluationsResult) estimateResponse {
	// 空集合交回空数组而不是 null：调用方判「没有」不该先判「有没有字段」。
	body := estimateResponse{
		Outcome:    result.Outcome.String(),
		Reason:     result.Reason.String(),
		Candidates: make([]estimateCandidateBody, 0, len(result.Candidates)),
		Conflict:   make([]planReferenceBody, 0, len(result.Conflict)),
	}
	for _, candidate := range result.Candidates {
		item := estimateCandidateBody{
			Plan:    planReferenceBodyOf(candidate.Plan),
			Answer:  candidate.Answer.String(),
			Missing: append([]string(nil), candidate.Missing...),
		}
		if candidate.Answer == application.EstimateCandidateEvaluated {
			evaluation := estimateEvaluationBodyOf(candidate.Evaluation)
			item.Evaluation = &evaluation
		}
		body.Candidates = append(body.Candidates, item)
	}
	for _, reference := range result.Conflict {
		body.Conflict = append(body.Conflict, planReferenceBodyOf(reference))
	}
	return body
}

func planReferenceBodyOf(reference domain.VersionReference) planReferenceBody {
	return planReferenceBody{ID: reference.ID(), Version: reference.Version()}
}

func estimateEvaluationBodyOf(evaluation domain.PricingEvaluation) estimateEvaluationBody {
	body := estimateEvaluationBody{
		EvaluationID:   evaluation.ID().String(),
		Status:         string(evaluation.Status()),
		Evidence:       string(evaluation.Evidence()),
		Direction:      evaluation.Direction().String(),
		Purpose:        evaluation.Purpose().String(),
		SemanticDigest: evaluation.SemanticDigest(),
		ChargeLines:    make([]estimateChargeBody, 0, len(evaluation.ChargeLines())),
		Issues:         make([]estimateIssueBody, 0, len(evaluation.Issues())),
		Explanation:    append([]string{}, evaluation.Explanation()...),
		Manifest:       make([]manifestEntryBody, 0),
	}
	// 合计只在评价完成时有；待判断、不可计价、冲突与失败都没有合计，面上缺席而不是零（CONTEXT：不得以零金额表达不可计价）。
	if total, completed := evaluation.Total(); completed {
		body.Total = &moneyBody{Amount: total.Amount().String(), Currency: total.Currency().String()}
	}
	for _, line := range evaluation.ChargeLines() {
		body.ChargeLines = append(body.ChargeLines, estimateChargeBody{
			Code:        line.Code().String(),
			Description: line.Description(),
			Amount:      moneyBody{Amount: line.Amount().Amount().String(), Currency: line.Amount().Currency().String()},
		})
	}
	for _, issue := range evaluation.Issues() {
		body.Issues = append(body.Issues, estimateIssueBody{Code: issue.Code(), Message: issue.Message()})
	}
	for _, reference := range evaluation.Manifest().References() {
		body.Manifest = append(body.Manifest, manifestEntryBody{
			Kind: string(reference.Kind()), ID: reference.ID(), Version: reference.Version(),
		})
	}
	return body
}

// EstimatePayload 是运营操作者面的试算载荷线格式（UC-PP-001「试算声明」；逐字段表单，ADR-0101 决定八自裁）。各项由发起方
// 声明、一格不给默认；可缺的四项（尺寸、分区、邮编路线、结算币种）缺席即未声明。
//
// **载荷里只有内容，没有身份。** 租户从 `OperatorEnvelope` 来、由 Intake 作为入参交进来；载荷里出现 tenant 之类的键
// 按未知键拒——严格解码不是挑剔，是不让自报身份有地方落。
type EstimatePayload struct {
	Scope              string              `json:"scope"`
	Direction          string              `json:"direction"`
	BasisAt            string              `json:"basisAt"`
	Weight             *estimateWeight     `json:"weight"`
	Dimensions         *estimateDimensions `json:"dimensions"`
	Zone               string              `json:"zone"`
	PostalRoute        *estimateRoute      `json:"postalRoute"`
	SettlementCurrency string              `json:"settlementCurrency"`
}

type estimateWeight struct {
	Value string `json:"value"`
	Unit  string `json:"unit"`
}

type estimateDimensions struct {
	Length string `json:"length"`
	Width  string `json:"width"`
	Height string `json:"height"`
	Unit   string `json:"unit"`
}

type estimateRoute struct {
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
}

// DecodeEstimatePayload 只做结构解码：JSON 合法、键都认识。字段值对不对留给 Command 里的领域构造器。
func DecodeEstimatePayload(body io.Reader) (EstimatePayload, error) {
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var payload EstimatePayload
	if err := decoder.Decode(&payload); err != nil {
		return EstimatePayload{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	return payload, nil
}

// Command 把载荷译成试算命令，租户由调用方（Intake，从信封）给。每一格都过领域构造器；任何一格不成形都是畸形请求
// （400），不是业务答案——编排的受理门再兜一次必需项。
func (payload EstimatePayload) Command(tenant domain.TenantID) (application.FormEstimateEvaluationsCommand, error) {
	malformed := func(field string, err error) (application.FormEstimateEvaluationsCommand, error) {
		return application.FormEstimateEvaluationsCommand{}, fmt.Errorf("%w: %s: %v", ErrMalformedRequest, field, err)
	}
	scope, err := domain.NewPricingScopeID(payload.Scope)
	if err != nil {
		return malformed("scope", err)
	}
	direction, err := domain.NewPricingDirection(payload.Direction)
	if err != nil {
		return malformed("direction", err)
	}
	basisAt, err := time.Parse(time.RFC3339Nano, payload.BasisAt)
	if err != nil {
		return malformed("basisAt", err)
	}
	if payload.Weight == nil {
		return malformed("weight", fmt.Errorf("absent"))
	}
	weightUnit, err := domain.NewWeightUnit(payload.Weight.Unit)
	if err != nil {
		return malformed("weight.unit", err)
	}
	weight, err := domain.NewWeightFromString(payload.Weight.Value, weightUnit)
	if err != nil {
		return malformed("weight.value", err)
	}
	command := application.FormEstimateEvaluationsCommand{
		Tenant:    tenant,
		Scope:     scope,
		Direction: direction,
		BasisAt:   basisAt,
		Weight:    weight,
		Zone:      payload.Zone,
	}
	if payload.Dimensions != nil {
		dimensions, err := payload.Dimensions.domain()
		if err != nil {
			return malformed("dimensions", err)
		}
		command.Dimensions = &dimensions
	}
	if payload.PostalRoute != nil {
		route, err := domain.NewPostalRoute(payload.PostalRoute.Origin, payload.PostalRoute.Destination)
		if err != nil {
			return malformed("postalRoute", err)
		}
		command.Route = &route
	}
	if payload.SettlementCurrency != "" {
		currency, err := domain.NewCurrency(payload.SettlementCurrency)
		if err != nil {
			return malformed("settlementCurrency", err)
		}
		command.Settlement = &currency
	}
	return command, nil
}

func (dimensions estimateDimensions) domain() (domain.Dimensions, error) {
	unit, err := domain.NewLengthUnit(dimensions.Unit)
	if err != nil {
		return domain.Dimensions{}, err
	}
	length, err := domain.ParseDecimal(dimensions.Length)
	if err != nil {
		return domain.Dimensions{}, err
	}
	width, err := domain.ParseDecimal(dimensions.Width)
	if err != nil {
		return domain.Dimensions{}, err
	}
	height, err := domain.ParseDecimal(dimensions.Height)
	if err != nil {
		return domain.Dimensions{}, err
	}
	return domain.NewDimensions(length, width, height, unit)
}
