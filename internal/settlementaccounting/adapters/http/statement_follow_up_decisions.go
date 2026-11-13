package settlementhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件是对账单发布之后的四个结算运营决定口（UC-SA-003 步 6–7）：对一笔已确认费用登记调整、依据作废整张对账单、把
// 后到的已确认费用纳入后续账期、把一笔既有调整纳入后续账期。进的是 parcel-api 已装配的调整编排与截单编排，各包一笔
// 事务。它们不过结算时点那一道门：门管确认与截单什么时候能动，这四种动的都是门放行之后才会存在的对象。
//
// 四口的时刻都由服务端给：载荷里没有调整形成时刻、作废时刻、纳入时刻这几格。

const (
	// OperatorDecisionChargeAdjustmentRegistration 是对一笔已确认费用登记调整（计价纠错或商业让利）。
	OperatorDecisionChargeAdjustmentRegistration OperatorDecision = "CHARGE_ADJUSTMENT_REGISTRATION"
	// OperatorDecisionStatementVoid 是依据作废整张对账单。
	OperatorDecisionStatementVoid OperatorDecision = "STATEMENT_VOID"
	// OperatorDecisionLateChargeInclusion 是把后到的已确认费用纳入后续账期。
	OperatorDecisionLateChargeInclusion OperatorDecision = "LATE_CHARGE_INCLUSION"
	// OperatorDecisionAdjustmentInclusion 是把一笔既有调整纳入后续账期。
	OperatorDecisionAdjustmentInclusion OperatorDecision = "ADJUSTMENT_INCLUSION"
)

// ChargeAdjuster 是调整登记口转交的那一口（settlementOrchestrations.RecordAdjustment）。
type ChargeAdjuster interface {
	RecordAdjustment(ctx context.Context, command application.RecordChargeAdjustmentCommand) (application.RecordChargeAdjustmentResult, error)
}

// StatementFollowUp 是作废口与两个纳入口转交的那三口（settlementOrchestrations 上包事务的同名方法）。
type StatementFollowUp interface {
	VoidStatement(ctx context.Context, command application.VoidStatementCommand) (application.StatementResult, error)
	IncludeLateCharge(ctx context.Context, command application.IncludeLateChargeCommand) (application.StatementResult, error)
	IncludeAdjustment(ctx context.Context, command application.IncludeAdjustmentCommand) (application.StatementResult, error)
}

// StatementFollowUpIntake 是四口的 Intake。
type StatementFollowUpIntake interface {
	IntakeChargeAdjustment(ctx context.Context, request *http.Request) (application.RecordChargeAdjustmentCommand, error)
	IntakeStatementVoid(ctx context.Context, request *http.Request) (application.VoidStatementCommand, error)
	IntakeLateChargeInclusion(ctx context.Context, request *http.Request) (application.IncludeLateChargeCommand, error)
	IntakeAdjustmentInclusion(ctx context.Context, request *http.Request) (application.IncludeAdjustmentCommand, error)
}

var _ StatementFollowUpIntake = (*OperatorDecisionIntake)(nil)

// ChargeAdjustmentPayload 是调整登记口的线格式。adjustment 是调用方给这笔调整起的标识，同一标识重投交回原调整。kind
// 取 PRICING_CORRECTION（必带 evaluation：纠错依据的那一次新评价）或 COMMERCIAL_CONCESSION（必带 authorization：商业
// 授权），有此无彼由领域的形成门判。金额按最小货币单位的整数串发；结算金额与原币不同币种时带 conversion。
type ChargeAdjustmentPayload struct {
	Adjustment         string `json:"adjustment"`
	Charge             string `json:"charge"`
	Kind               string `json:"kind"`
	Direction          string `json:"direction"`
	Evaluation         string `json:"evaluation"`
	Authorization      string `json:"authorization"`
	OriginalCurrency   string `json:"originalCurrency"`
	OriginalMinor      string `json:"originalMinor"`
	SettlementCurrency string `json:"settlementCurrency"`
	SettlementMinor    string `json:"settlementMinor"`
	Conversion         string `json:"conversion"`
}

// StatementVoidPayload 是作废口的线格式：哪张单、凭什么作废。
type StatementVoidPayload struct {
	Statement string `json:"statement"`
	Basis     string `json:"basis"`
}

// LateChargeInclusionPayload 是后到费用纳入口的线格式。inclusion 是调用方给这次纳入起的标识；statement 是原账单；
// subsequentPeriod 是纳入的后续账期——纳回原账期即回填已发布的快照，不受理。
type LateChargeInclusionPayload struct {
	Inclusion        string `json:"inclusion"`
	Statement        string `json:"statement"`
	Charge           string `json:"charge"`
	SubsequentPeriod string `json:"subsequentPeriod"`
}

// AdjustmentInclusionPayload 是调整纳入口的线格式。只指名调整：金额、方向与原因都在调整本体上。
type AdjustmentInclusionPayload struct {
	Inclusion        string `json:"inclusion"`
	Statement        string `json:"statement"`
	Adjustment       string `json:"adjustment"`
	SubsequentPeriod string `json:"subsequentPeriod"`
}

// followUpPayload 先认证、后读载荷；载荷封闭，多一个键即拒。
func (intake *OperatorDecisionIntake) followUpPayload(ctx context.Context, request *http.Request, decision OperatorDecision, payload any) (OperatorIdentity, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), decision)
	if err != nil {
		return OperatorIdentity{}, err
	}
	raw, err := readSinglePayload(io.LimitReader(request.Body, maxManualRegistrationBytes+1))
	if err != nil {
		return OperatorIdentity{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(payload); err != nil {
		return OperatorIdentity{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	return operator, nil
}

func (intake *OperatorDecisionIntake) IntakeChargeAdjustment(ctx context.Context, request *http.Request) (application.RecordChargeAdjustmentCommand, error) {
	none := application.RecordChargeAdjustmentCommand{}
	var payload ChargeAdjustmentPayload
	operator, err := intake.followUpPayload(ctx, request, OperatorDecisionChargeAdjustmentRegistration, &payload)
	if err != nil {
		return none, err
	}
	kind, err := adjustmentKindFromWord(payload.Kind)
	if err != nil {
		return none, err
	}
	direction, err := adjustmentDirectionFromWord(payload.Direction)
	if err != nil {
		return none, err
	}
	original, err := minorFromWord("originalMinor", payload.OriginalMinor)
	if err != nil {
		return none, err
	}
	settlement, err := minorFromWord("settlementMinor", payload.SettlementMinor)
	if err != nil {
		return none, err
	}
	return application.RecordChargeAdjustmentCommand{
		TenantID: operator.Tenant, AdjustmentID: payload.Adjustment, ChargeID: payload.Charge, Kind: kind, Direction: direction,
		Evaluation: payload.Evaluation, Authorization: payload.Authorization,
		OriginalCurrency: payload.OriginalCurrency, OriginalMinor: original,
		SettlementCurrency: payload.SettlementCurrency, SettlementMinor: settlement, Conversion: payload.Conversion,
	}, nil
}

func (intake *OperatorDecisionIntake) IntakeStatementVoid(ctx context.Context, request *http.Request) (application.VoidStatementCommand, error) {
	var payload StatementVoidPayload
	operator, err := intake.followUpPayload(ctx, request, OperatorDecisionStatementVoid, &payload)
	if err != nil {
		return application.VoidStatementCommand{}, err
	}
	return application.VoidStatementCommand{TenantID: operator.Tenant, Number: payload.Statement, Basis: payload.Basis}, nil
}

func (intake *OperatorDecisionIntake) IntakeLateChargeInclusion(ctx context.Context, request *http.Request) (application.IncludeLateChargeCommand, error) {
	var payload LateChargeInclusionPayload
	operator, err := intake.followUpPayload(ctx, request, OperatorDecisionLateChargeInclusion, &payload)
	if err != nil {
		return application.IncludeLateChargeCommand{}, err
	}
	return application.IncludeLateChargeCommand{
		TenantID: operator.Tenant, Inclusion: payload.Inclusion, Number: payload.Statement, ChargeID: payload.Charge, SubsequentPeriod: payload.SubsequentPeriod,
	}, nil
}

func (intake *OperatorDecisionIntake) IntakeAdjustmentInclusion(ctx context.Context, request *http.Request) (application.IncludeAdjustmentCommand, error) {
	var payload AdjustmentInclusionPayload
	operator, err := intake.followUpPayload(ctx, request, OperatorDecisionAdjustmentInclusion, &payload)
	if err != nil {
		return application.IncludeAdjustmentCommand{}, err
	}
	return application.IncludeAdjustmentCommand{
		TenantID: operator.Tenant, Inclusion: payload.Inclusion, Number: payload.Statement, AdjustmentID: payload.Adjustment, SubsequentPeriod: payload.SubsequentPeriod,
	}, nil
}

func adjustmentKindFromWord(raw string) (domain.AdjustmentKind, error) {
	for _, kind := range []domain.AdjustmentKind{domain.PricingCorrection, domain.CommercialConcession} {
		if kind.String() == raw {
			return kind, nil
		}
	}
	return domain.AdjustmentKindInvalid, fmt.Errorf("%w: kind=%q is not PRICING_CORRECTION or COMMERCIAL_CONCESSION", ErrMalformedRequest, raw)
}

func adjustmentDirectionFromWord(raw string) (domain.AdjustmentDirection, error) {
	for _, direction := range []domain.AdjustmentDirection{domain.AdjustmentDebit, domain.AdjustmentCredit} {
		if direction.String() == raw {
			return direction, nil
		}
	}
	return domain.AdjustmentDirectionInvalid, fmt.Errorf("%w: direction=%q is not DEBIT or CREDIT", ErrMalformedRequest, raw)
}

// minorFromWord 把最小货币单位的整数串译成整数。金额按串发：JSON 数字过了 2^53 会在途中丢精度。
func minorFromWord(field, raw string) (int64, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %s must be an integer string of minor units", ErrMalformedRequest, field)
	}
	return value, nil
}

func followUpEndpoint[C, R any](
	intake func(context.Context, *http.Request) (C, error),
	handle func(context.Context, C) (R, error),
	write func(http.ResponseWriter, R),
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}
		result, err := handle(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		write(response, result)
	})
}

// NewRecordChargeAdjustmentEndpoint 交回调整登记口（POST /settlement-charge-adjustments）。
func NewRecordChargeAdjustmentEndpoint(intake StatementFollowUpIntake, adjuster ChargeAdjuster) http.Handler {
	return followUpEndpoint(intake.IntakeChargeAdjustment, adjuster.RecordAdjustment, writeAdjustmentAnswer)
}

// NewVoidStatementEndpoint 交回作废口（POST /settlement-statement-voids）。
func NewVoidStatementEndpoint(intake StatementFollowUpIntake, followUp StatementFollowUp) http.Handler {
	return followUpEndpoint(intake.IntakeStatementVoid, followUp.VoidStatement, func(response http.ResponseWriter, result application.StatementResult) {
		writeFollowUpAnswer(response, result, application.StatementVoidedOutcome)
	})
}

// NewIncludeLateChargeEndpoint 交回后到费用纳入口（POST /settlement-late-charge-inclusions）。
func NewIncludeLateChargeEndpoint(intake StatementFollowUpIntake, followUp StatementFollowUp) http.Handler {
	return followUpEndpoint(intake.IntakeLateChargeInclusion, followUp.IncludeLateCharge, func(response http.ResponseWriter, result application.StatementResult) {
		writeFollowUpAnswer(response, result, application.LateChargeIncluded)
	})
}

// NewIncludeAdjustmentEndpoint 交回调整纳入口（POST /settlement-adjustment-inclusions）。
func NewIncludeAdjustmentEndpoint(intake StatementFollowUpIntake, followUp StatementFollowUp) http.Handler {
	return followUpEndpoint(intake.IntakeAdjustmentInclusion, followUp.IncludeAdjustment, func(response http.ResponseWriter, result application.StatementResult) {
		writeFollowUpAnswer(response, result, application.AdjustmentIncluded)
	})
}

// adjustmentResponse 透出答复连同那一笔调整此刻的样子：重放带上库里那一份，操作者看得到同一标识下登的是什么。
type adjustmentResponse struct {
	Outcome            string `json:"outcome"`
	Adjustment         string `json:"adjustment,omitempty"`
	Charge             string `json:"charge,omitempty"`
	Kind               string `json:"kind,omitempty"`
	Direction          string `json:"direction,omitempty"`
	Evaluation         string `json:"evaluation,omitempty"`
	Authorization      string `json:"authorization,omitempty"`
	OriginalCurrency   string `json:"originalCurrency,omitempty"`
	OriginalMinor      string `json:"originalMinor,omitempty"`
	SettlementCurrency string `json:"settlementCurrency,omitempty"`
	SettlementMinor    string `json:"settlementMinor,omitempty"`
	Conversion         string `json:"conversion,omitempty"`
	FormedAt           string `json:"formedAt,omitempty"`
}

// writeAdjustmentAnswer 与本包其余写口同一条判据：状态码只答有没有形成答案。这次落了新调整答 201；重放、费用未确认、
// 未受理都是答案（200）；册读不动是没形成答案。
func writeAdjustmentAnswer(response http.ResponseWriter, result application.RecordChargeAdjustmentResult) {
	name := result.Outcome().String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	if result.Outcome() == application.AdjustmentUndecided {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	body := adjustmentResponse{Outcome: name}
	if adjustment, has := result.Adjustment(); has {
		body.Adjustment = adjustment.ID().String()
		body.Charge = adjustment.Charge().String()
		body.Kind = adjustment.Kind().String()
		body.Direction = adjustment.Direction().String()
		if evaluation, present := adjustment.Evaluation(); present {
			body.Evaluation = evaluation.String()
		}
		if authorization, present := adjustment.Authorization(); present {
			body.Authorization = authorization.String()
		}
		originalCurrency, originalMinor := adjustment.OriginalAmount()
		body.OriginalCurrency, body.OriginalMinor = originalCurrency.String(), minorAmount(originalMinor)
		settlementCurrency, settlementMinor := adjustment.SettlementAmount()
		body.SettlementCurrency, body.SettlementMinor = settlementCurrency.String(), minorAmount(settlementMinor)
		if conversion, present := adjustment.Conversion(); present {
			body.Conversion = conversion.String()
		}
		body.FormedAt = rfc3339(adjustment.FormedAt())
	}
	status := http.StatusOK
	if result.Outcome() == application.ChargeAdjustmentRecordedOutcome {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}

// followUpResponse 是作废口与两个纳入口的响应：对账单各格只在对账单在场时交回，纳入各格只在纳入在场时交回。
type followUpResponse struct {
	Outcome          string `json:"outcome"`
	Statement        string `json:"statement,omitempty"`
	VoidBasis        string `json:"voidBasis,omitempty"`
	VoidedAt         string `json:"voidedAt,omitempty"`
	Inclusion        string `json:"inclusion,omitempty"`
	InclusionKind    string `json:"inclusionKind,omitempty"`
	Charge           string `json:"charge,omitempty"`
	Adjustment       string `json:"adjustment,omitempty"`
	OriginalPeriod   string `json:"originalPeriod,omitempty"`
	SubsequentPeriod string `json:"subsequentPeriod,omitempty"`
	IncludedAt       string `json:"includedAt,omitempty"`
}

// writeFollowUpAnswer 的判据同 writeDisputeAnswer：formed 是这一口「这次落了新东西」的那一格，答 201；其余具名答复
// 答 200；册读不动、作废或纳入的意图没交出去是没形成答案。
func writeFollowUpAnswer(response http.ResponseWriter, result application.StatementResult, formed application.StatementOutcome) {
	name := result.Outcome().String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	if result.Outcome() == application.StatementUndecided {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	if result.StatementHandoffReference() != "" {
		writeProblem(response, http.StatusInternalServerError, codeHandoffNotSent)
		return
	}
	body := followUpResponse{Outcome: name}
	if record, has := result.Statement(); has {
		body.Statement = record.Statement.Number().String()
		if basis, voidedAt, voided := record.Statement.Voided(); voided {
			body.VoidBasis, body.VoidedAt = basis.String(), rfc3339(voidedAt)
		}
	}
	if record, has := result.Inclusion(); has {
		inclusion := record.Inclusion
		body.Inclusion = inclusion.Inclusion().String()
		body.InclusionKind = inclusion.Kind().String()
		body.Statement = inclusion.Statement().String()
		body.Charge = inclusion.Charge().String()
		if adjustment, present := inclusion.Adjustment(); present {
			body.Adjustment = adjustment.String()
		}
		body.OriginalPeriod = inclusion.OriginalPeriod().String()
		body.SubsequentPeriod = inclusion.SubsequentPeriod().String()
		body.IncludedAt = rfc3339(inclusion.IncludedAt())
	}
	status := http.StatusOK
	if result.Outcome() == formed {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
