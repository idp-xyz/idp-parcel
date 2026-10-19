package tfhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 运输委托提交口（UC-TF-004 步 2、3；ADR-0202 决定一、三）。运营人员向外部服务提供方提出明确的运输范围与条件，是管理台
// 上作出的生产决定，挂操作者渠道的「运营决定」能力面、按自己的决定种类认证；路径取读面册名前缀 `transport-fulfillment-`，
// 理由同关段口——运营写决定与接入方回传事实不混一个前缀。

// TransportCommissionSubmissionIntake 把已认证的运营写请求翻译成委托提交命令。
type TransportCommissionSubmissionIntake interface {
	IntakeTransportCommissionSubmission(ctx context.Context, request *http.Request) (application.SubmitCommissionCommand, error)
}

// TransportCommissionSubmitter 是本适配器转交的应用编排。
type TransportCommissionSubmitter interface {
	SubmitCommission(ctx context.Context, command application.SubmitCommissionCommand) (application.CommissionResult, error)
}

// TransportCommissionSubmissionPayload 是委托提交口的线格式。载荷只有内容、没有身份：租户从信封来，载荷带租户即拒（封闭
// 解码）。协议只收登记方指名的协议对象与版本标签两格——快照引用与采购责任法人由编排核过协议之后取，载荷里没有这两格，
// 也就无从自报（ADR-0202 决定三）。条件、角色、责任是登记方声明的快照引用。提交时刻照 ADR-0023 由提交方给。
type TransportCommissionSubmissionPayload struct {
	Commission       string   `json:"commission"`
	Provider         string   `json:"provider"`
	Agreement        string   `json:"agreement"`
	AgreementVersion string   `json:"agreementVersion"`
	Conditions       string   `json:"conditions"`
	Role             string   `json:"role"`
	Responsibility   string   `json:"responsibility"`
	Members          []string `json:"members"`
	SubmittedAt      string   `json:"submittedAt"`
}

// Command 把载荷连同信封给的租户翻成委托提交命令。时刻解不出是坏报文（400）；各格成不成立、协议核不核得过由编排答。
func (payload TransportCommissionSubmissionPayload) Command(tenant domain.TenantID) (application.SubmitCommissionCommand, error) {
	if tenant.String() == "" {
		return application.SubmitCommissionCommand{}, ErrOperatorIdentityMissing
	}
	submittedAt, err := parseOptionalInstant("submittedAt", payload.SubmittedAt)
	if err != nil {
		return application.SubmitCommissionCommand{}, err
	}
	return application.SubmitCommissionCommand{
		TenantID:         tenant,
		Commission:       payload.Commission,
		Provider:         payload.Provider,
		AgreementObject:  payload.Agreement,
		AgreementVersion: payload.AgreementVersion,
		Conditions:       payload.Conditions,
		Role:             payload.Role,
		Responsibility:   payload.Responsibility,
		Members:          append([]string(nil), payload.Members...),
		SubmittedAt:      submittedAt,
	}, nil
}

// NewSubmitTransportCommissionEndpoint 交回委托提交的 HTTP 入口。
func NewSubmitTransportCommissionEndpoint(intake TransportCommissionSubmissionIntake, submitter TransportCommissionSubmitter) http.Handler {
	return commandEndpoint(func(request *http.Request) (application.CommissionResult, error, bool) {
		command, err := intake.IntakeTransportCommissionSubmission(request.Context(), request)
		if err != nil {
			return application.CommissionResult{}, err, false
		}
		result, err := submitter.SubmitCommission(request.Context(), command)
		return result, err, true
	}, writeTransportCommissionSubmissionOutcome)
}

// transportCommissionSubmissionResponse 是委托提交口的封闭响应形状。协议核对不过的各格各自成格，续办两两不同（去
// party-commercial 发布或补正文 / 换一版此刻生效的 / 取得新协议 / 改服务方），传输层不合并。`agreementSupplier` 只随
// `PROVIDER_MISMATCH` 出现，是那一版协议约定的供应商。`handoffReference` 非空说明委托已提交而意图还没交出去，原样重交
// 会重发同一份。
type transportCommissionSubmissionResponse struct {
	Outcome               string                   `json:"outcome"`
	UndecidedReason       string                   `json:"undecidedReason,omitempty"`
	ContinuationReference string                   `json:"continuationReference,omitempty"`
	HandoffReference      string                   `json:"handoffReference,omitempty"`
	AgreementSupplier     string                   `json:"agreementSupplier,omitempty"`
	Commission            *transportCommissionBody `json:"commission,omitempty"`
}

// transportCommissionBody 逐字段透出在册那一份委托：`agreement` 是 party-commercial 拼出的协议快照指称，`legalEntity`
// 是从那一版协议快照下来的采购责任法人。重放与已有委托回显的是在册那一份，不是这次送来的。时间一律 RFC 3339 UTC。
type transportCommissionBody struct {
	Commission     string   `json:"commission"`
	Provider       string   `json:"provider"`
	Agreement      string   `json:"agreement"`
	LegalEntity    string   `json:"legalEntity"`
	Conditions     string   `json:"conditions"`
	Role           string   `json:"role"`
	Responsibility string   `json:"responsibility"`
	Members        []string `json:"members"`
	SubmittedAt    string   `json:"submittedAt"`
	RecordedAt     string   `json:"recordedAt"`
}

func writeTransportCommissionSubmissionOutcome(response http.ResponseWriter, result application.CommissionResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := transportCommissionSubmissionResponse{
		Outcome:               outcome,
		UndecidedReason:       result.UndecidedReason().String(),
		ContinuationReference: result.ContinuationReference(),
		HandoffReference:      result.CommissionHandoffReference(),
	}
	if supplier, named := result.AgreementSupplier(); named {
		body.AgreementSupplier = supplier.String()
	}
	if record, present := result.Commission(); present {
		commission := record.Commission
		members := make([]string, 0, len(commission.Members()))
		for _, member := range commission.Members() {
			members = append(members, member.String())
		}
		body.Commission = &transportCommissionBody{
			Commission:     commission.Commission().String(),
			Provider:       commission.Provider().String(),
			Agreement:      commission.Agreement().String(),
			LegalEntity:    commission.LegalEntity().String(),
			Conditions:     commission.Conditions().String(),
			Role:           commission.Role().String(),
			Responsibility: commission.Responsibility().String(),
			Members:        members,
			SubmittedAt:    rfc3339(commission.SubmittedAt()),
			RecordedAt:     rfc3339(record.RecordedAt),
		}
	}
	// ADR-0022：只有新提交了一份委托用 201；已有委托、冲突、协议核对不过的各格、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if result.Outcome() == application.CommissionSubmitted {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
