package tfhttp

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 运输收费发生项登记的在线写面（票 demo-chain-gaps/27，ADR-0207）。它是一次运营决定（ADR-0151 定族判据：操作者做出
// 的生产决定），挂操作者渠道「运营决定」能力面的「运输收费发生项登记」一种，不挂作业事实——发生项不是现场发生的物理
// 事实，是对已发生事实的一次采购判断。载荷只有内容没有身份：租户与登记者（断言人）都从信封来。
//
// 面上不判任何业务结果：事实是否满足、协议是否在册、是不是自营，都由编排作答，这里只转交与映射。

// ChargeOccurrenceRegistrationIntake 把已认证的运营决定请求翻译成登记命令。
type ChargeOccurrenceRegistrationIntake interface {
	IntakeChargeOccurrenceRegistration(ctx context.Context, request *http.Request) (application.RegisterChargeOccurrenceCommand, error)
}

var _ ChargeOccurrenceRegistrationIntake = (*OperatorDecisionIntake)(nil)

// ChargeOccurrenceRegistrar 是本适配器转交的应用编排。
type ChargeOccurrenceRegistrar interface {
	Register(ctx context.Context, command application.RegisterChargeOccurrenceCommand) (application.RegisterChargeOccurrenceResult, error)
}

// ChargeOccurrenceRegistrationPayload 是登记口的线格式。
//
// reason 取发生原因的四个字面量之一。事实按原因各给一组：失败尝试给 pickupAttemptSource 或 deliveryAttempt 之一、
// objects 恰一个；实际履约给 segment 与 objects。selfOperated 为真是一种声明，此时 provider / agreement /
// agreementVersion 留空；否则三者必给——缺了是未受理，不是不适用。conditionAssertion 是登记者对「采用协议的发生
// 条件已满足」的原话断言。依据、范围、业务时间、发生项引用与版本都不收：它们取自事实或由事实派生。
type ChargeOccurrenceRegistrationPayload struct {
	Reason              string   `json:"reason"`
	PickupAttemptSource string   `json:"pickupAttemptSource,omitempty"`
	DeliveryAttempt     string   `json:"deliveryAttempt,omitempty"`
	Segment             string   `json:"segment,omitempty"`
	Objects             []string `json:"objects"`
	Journey             string   `json:"journey"`
	SelfOperated        bool     `json:"selfOperated,omitempty"`
	Provider            string   `json:"provider,omitempty"`
	Agreement           string   `json:"agreement,omitempty"`
	AgreementVersion    string   `json:"agreementVersion,omitempty"`
	Quantity            int64    `json:"quantity"`
	Unit                string   `json:"unit"`
	ConditionAssertion  string   `json:"conditionAssertion"`
}

// Command 把载荷连同信封给的租户与操作者翻成登记命令。原因词不在封闭集内是坏报文（400）；其余成形与否留给编排答
// 未受理（200，形成了的业务答案）——两者的续办不同，不折成一格。
func (payload ChargeOccurrenceRegistrationPayload) Command(operator OperatorIdentity) (application.RegisterChargeOccurrenceCommand, error) {
	if operator.Tenant.String() == "" || strings.TrimSpace(operator.Operator) == "" {
		return application.RegisterChargeOccurrenceCommand{}, ErrOperatorIdentityMissing
	}
	reason, err := domain.ParseChargeOccurrenceReason(strings.TrimSpace(payload.Reason))
	if err != nil {
		return application.RegisterChargeOccurrenceCommand{}, fmt.Errorf("%w: reason is not a charge occurrence reason word", ErrMalformedRequest)
	}
	return application.RegisterChargeOccurrenceCommand{
		TenantID:            operator.Tenant,
		Registrant:          operator.Operator,
		Reason:              reason,
		PickupAttemptSource: payload.PickupAttemptSource,
		DeliveryAttempt:     payload.DeliveryAttempt,
		Segment:             payload.Segment,
		Objects:             append([]string(nil), payload.Objects...),
		Journey:             payload.Journey,
		SelfOperated:        payload.SelfOperated,
		Provider:            payload.Provider,
		AgreementObject:     payload.Agreement,
		AgreementVersion:    payload.AgreementVersion,
		Quantity:            payload.Quantity,
		Unit:                payload.Unit,
		ConditionAssertion:  payload.ConditionAssertion,
	}, nil
}

// NewRegisterChargeOccurrenceEndpoint 交回登记口的 HTTP 入口。
func NewRegisterChargeOccurrenceEndpoint(intake ChargeOccurrenceRegistrationIntake, registrar ChargeOccurrenceRegistrar) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.RegisterChargeOccurrenceResult, error, bool) {
		command, err := intake.IntakeChargeOccurrenceRegistration(request.Context(), request)
		if err != nil {
			return application.RegisterChargeOccurrenceResult{}, err, false
		}
		result, err := registrar.Register(request.Context(), command)
		return result, err, true
	}, writeChargeOccurrenceOutcome)
}

// chargeOccurrenceResponse 是封闭响应形状。**没有任何金额或币种键**：发生项不是价格、预期成本、账单或应付（CONTEXT），
// 金额由计价与结算另行形成。detail 指名不适用、未决、未受理落在哪一格；agreedSupplier 只在服务方不符时给出。
type chargeOccurrenceResponse struct {
	Outcome               string                `json:"outcome"`
	Detail                string                `json:"detail,omitempty"`
	ContinuationReference string                `json:"continuationReference,omitempty"`
	AgreedSupplier        string                `json:"agreedSupplier,omitempty"`
	Occurrence            *chargeOccurrenceBody `json:"occurrence,omitempty"`
}

// chargeOccurrenceBody 逐字段透出一个发生项版本。occurredAt 是业务时间，recordedAt 是登记时刻，两者归属不同不合并。
type chargeOccurrenceBody struct {
	Occurrence          string   `json:"occurrence"`
	Version             string   `json:"version"`
	Reason              string   `json:"reason"`
	Journey             string   `json:"journey"`
	LegalEntity         string   `json:"legalEntity"`
	Provider            string   `json:"provider"`
	Agreement           string   `json:"agreement"`
	FactBasis           string   `json:"factBasis"`
	Scope               string   `json:"scope"`
	Members             []string `json:"members"`
	Quantity            int64    `json:"quantity"`
	Unit                string   `json:"unit"`
	OccurredAt          string   `json:"occurredAt"`
	ConditionBasis      string   `json:"conditionBasis"`
	ConditionAssertedBy string   `json:"conditionAssertedBy,omitempty"`
	ConditionAssertion  string   `json:"conditionAssertion,omitempty"`
	RecordedAt          string   `json:"recordedAt"`
}

func chargeOccurrenceBodyOf(record ports.ChargeOccurrenceRecord) chargeOccurrenceBody {
	occurrence := record.Occurrence
	quantity, unit := occurrence.Quantity()
	body := chargeOccurrenceBody{
		Occurrence:     occurrence.Occurrence().String(),
		Version:        occurrence.Validity().String(),
		Reason:         occurrence.Reason().String(),
		Journey:        occurrence.Journey().String(),
		LegalEntity:    occurrence.LegalEntity().String(),
		Provider:       occurrence.Provider().String(),
		Agreement:      occurrence.Agreement().String(),
		FactBasis:      occurrence.FactBasis().String(),
		Scope:          occurrence.Scope().String(),
		Members:        make([]string, 0, len(occurrence.Members())),
		Quantity:       quantity,
		Unit:           unit.String(),
		OccurredAt:     rfc3339(occurrence.OccurredAt()),
		ConditionBasis: occurrence.Condition().Basis().String(),
		RecordedAt:     rfc3339(record.RecordedAt),
	}
	for _, member := range occurrence.Members() {
		body.Members = append(body.Members, member.String())
	}
	if assertion, asserted := occurrence.Condition().Assertion(); asserted {
		body.ConditionAssertion = assertion
	}
	if registrant, asserted := occurrence.Condition().AssertedBy(); asserted {
		body.ConditionAssertedBy = registrant.String()
	}
	return body
}

func writeChargeOccurrenceOutcome(response http.ResponseWriter, result application.RegisterChargeOccurrenceResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := chargeOccurrenceResponse{
		Outcome:               outcome,
		Detail:                result.Detail().String(),
		ContinuationReference: result.Continuation(),
		AgreedSupplier:        result.AgreedSupplier(),
	}
	if record, present := result.Record(); present {
		occurrence := chargeOccurrenceBodyOf(record)
		body.Occurrence = &occurrence
	}
	// ADR-0022：只有新落了一个发生项版本用 201；已存在、冲突、不适用、协议否定格、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.ChargeOccurrenceRegistered {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
