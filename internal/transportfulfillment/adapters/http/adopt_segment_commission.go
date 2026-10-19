package tfhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 履约段采用运输委托口（ADR-0204；ADR-0202 决定一）。运营人员声明一个在册的实际履约段在一份已提交的委托下、由哪一家
// 履约，是管理台上作出的生产决定，挂操作者渠道的「运营决定」能力面、按履约段采用运输委托这一种认证；路径取读面册名前缀
// `transport-fulfillment-`，理由同委托提交口。

// SegmentCommissionAdoptionIntake 把已认证的运营写请求翻译成采用命令。
type SegmentCommissionAdoptionIntake interface {
	IntakeSegmentCommissionAdoption(ctx context.Context, request *http.Request) (application.AdoptSegmentCommissionCommand, error)
}

// SegmentCommissionAdopter 是本适配器转交的应用编排。
type SegmentCommissionAdopter interface {
	Adopt(ctx context.Context, command application.AdoptSegmentCommissionCommand) (application.CommissionAdoptionResult, error)
}

// SegmentCommissionAdoptionPayload 是采用口的线格式。载荷只有内容、没有身份：租户从信封来，载荷带租户即拒（封闭解码）。
// 服务方由登记方声明、与委托的服务提供方核对；开始依据与时刻不收——它们取自段的首个参与，载荷里写的只会是自报。
type SegmentCommissionAdoptionPayload struct {
	Segment    string `json:"segment"`
	Commission string `json:"commission"`
	Provider   string `json:"provider"`
}

// Command 把载荷连同信封给的租户翻成采用命令。各格成不成立、段与委托在不在册由编排答。
func (payload SegmentCommissionAdoptionPayload) Command(tenant domain.TenantID) (application.AdoptSegmentCommissionCommand, error) {
	if tenant.String() == "" {
		return application.AdoptSegmentCommissionCommand{}, ErrOperatorIdentityMissing
	}
	return application.AdoptSegmentCommissionCommand{
		TenantID:   tenant,
		Segment:    payload.Segment,
		Commission: payload.Commission,
		Provider:   payload.Provider,
	}, nil
}

// NewAdoptSegmentCommissionEndpoint 交回采用口的 HTTP 入口。
func NewAdoptSegmentCommissionEndpoint(intake SegmentCommissionAdoptionIntake, adopter SegmentCommissionAdopter) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.CommissionAdoptionResult, error, bool) {
		command, err := intake.IntakeSegmentCommissionAdoption(request.Context(), request)
		if err != nil {
			return application.CommissionAdoptionResult{}, err, false
		}
		result, err := adopter.Adopt(request.Context(), command)
		return result, err, true
	}, writeSegmentCommissionAdoptionOutcome)
}

// segmentCommissionAdoptionResponse 是采用口的封闭响应形状。前置条件不满足的各格各自成格，续办两两不同，传输层不合并。
// `commissionProvider` 只随 `PROVIDER_MISMATCH` 出现，是委托的服务提供方；`adoption` 随采用成立、已有采用与采用冲突出现，
// 冲突时是段上在册的那一份；`commission` 只随采用成立出现，回显委托此刻的开始。
type segmentCommissionAdoptionResponse struct {
	Outcome               string                         `json:"outcome"`
	UndecidedReason       string                         `json:"undecidedReason,omitempty"`
	ContinuationReference string                         `json:"continuationReference,omitempty"`
	CommissionProvider    string                         `json:"commissionProvider,omitempty"`
	Adoption              *segmentCommissionAdoptionBody `json:"adoption,omitempty"`
	Commission            *commissionTransitionBody      `json:"commission,omitempty"`
}

// segmentCommissionAdoptionBody 逐字段透出在册那一份采用：`startBasis`、`startedAt` 是这一段在委托下开始的依据与时刻（采用
// 时段的首个参与）。时间一律 RFC 3339 UTC。
type segmentCommissionAdoptionBody struct {
	Segment    string `json:"segment"`
	Commission string `json:"commission"`
	Provider   string `json:"provider"`
	StartBasis string `json:"startBasis"`
	StartedAt  string `json:"startedAt"`
	RecordedAt string `json:"recordedAt"`
}

func writeSegmentCommissionAdoptionOutcome(response http.ResponseWriter, result application.CommissionAdoptionResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := segmentCommissionAdoptionResponse{
		Outcome:               outcome,
		UndecidedReason:       result.UndecidedReason().String(),
		ContinuationReference: result.ContinuationReference(),
	}
	if provider, named := result.CommissionProvider(); named {
		body.CommissionProvider = provider.String()
	}
	if record, present := result.Adoption(); present {
		adoption := record.Adoption
		basis, startedAt := adoption.Started()
		body.Adoption = &segmentCommissionAdoptionBody{
			Segment:    adoption.Segment().String(),
			Commission: adoption.Commission().String(),
			Provider:   adoption.Provider().String(),
			StartBasis: basis.String(),
			StartedAt:  rfc3339(startedAt),
			RecordedAt: rfc3339(record.RecordedAt),
		}
	}
	if record, present := result.Commission(); present {
		body.Commission = commissionTransitionBodyOf(record.Commission, record.RecordedAt)
	}
	// ADR-0022：只有这次真把段挂到委托上用 201；已有采用、冲突、前置条件不满足的各格、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.CommissionAdopted {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
