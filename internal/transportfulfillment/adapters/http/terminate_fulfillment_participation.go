package tfhttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 票 tf-segment-lifecycle-closure/07 的第四个写面：明确控制终止——结束一条履约参与关系的三来源里唯一
// 走写面的一路（票 03 简报第 5 行）。有效交付与下一次权威交接两路是 TF 自己的派生，内部触发（票 06）。

// ParticipationTermination 是终止口自己的命令形状，**刻意比应用命令窄**：只有段、对象、依据与时刻。
//
// 它存在的全部理由是让「这个口只能铸终止那一路」在类型上成立——Intake 造不出 Source，也造不出
// NextSegment / Scope / Version / Attempt：前者由端点固定为明确终止，后几格属交接与交付两路，从这个口进来
// 就是让接入方替 TF 做 UC-TF-005 步骤 7。终止之后没有「下一段」（carriesControlOnward），所以下一段那格
// 连位置都不留。
type ParticipationTermination struct {
	TenantID domain.TenantID
	Segment  string
	Object   string
	// Basis 指向本上下文之外的处置决定，只能由调用方给——这是终止一路与另两路真正的不对称。
	Basis   string
	EndedAt time.Time
}

// ParticipationTerminationIntake 把已认证的运营写请求翻译成一次明确终止。
type ParticipationTerminationIntake interface {
	IntakeParticipationTermination(ctx context.Context, request *http.Request) (ParticipationTermination, error)
}

// ParticipationEnder 是本适配器转交的应用编排。接口收的是完整的应用命令：编排本来就是三路共用的一条，
// 窄口只窄在 Intake 这一侧。
type ParticipationEnder interface {
	End(
		ctx context.Context,
		command application.EndFulfillmentParticipationCommand,
	) (application.EndFulfillmentParticipationResult, error)
}

// participationTerminationAnswer 把编排结果与要回显的段、对象捆在一起交给映射（结果类型自己不带它们）。
type participationTerminationAnswer struct {
	result  application.EndFulfillmentParticipationResult
	segment string
	object  string
}

// NewTerminateFulfillmentParticipationEndpoint 交回明确终止参与的 HTTP 入口。
func NewTerminateFulfillmentParticipationEndpoint(intake ParticipationTerminationIntake, ender ParticipationEnder) http.Handler {
	return commandEndpoint(func(request *http.Request) (participationTerminationAnswer, error, bool) {
		termination, err := intake.IntakeParticipationTermination(request.Context(), request)
		if err != nil {
			return participationTerminationAnswer{}, err, false
		}
		result, err := ender.End(request.Context(), application.EndFulfillmentParticipationCommand{
			TenantID: termination.TenantID,
			Segment:  termination.Segment,
			Object:   termination.Object,
			Source:   application.ParticipationEndedByTermination,
			Basis:    termination.Basis,
			EndedAt:  termination.EndedAt,
		})
		return participationTerminationAnswer{result: result, segment: termination.Segment, object: termination.Object}, err, true
	}, writeParticipationTerminationOutcome)
}

// participationTerminationResponse 是终止口的封闭响应形状。四个否定格（已离场 / 对象不在段内 / 段不在册 /
// 控制事实不在册）各自成格：续办动作两两不同，传输层不合并。没有 nextSegmentContinuationReference：终止之后
// 没有下一段，那一格在这个口上永远为空，透出一个恒空的格只会让调用方去等一件不会发生的事。
type participationTerminationResponse struct {
	Outcome               string `json:"outcome"`
	Segment               string `json:"segment,omitempty"`
	Object                string `json:"object,omitempty"`
	ContinuationReference string `json:"continuationReference,omitempty"`
}

func writeParticipationTerminationOutcome(response http.ResponseWriter, answer participationTerminationAnswer) {
	outcome := answer.result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := participationTerminationResponse{
		Outcome:               outcome,
		ContinuationReference: answer.result.ContinuationReference(),
	}
	if answer.result.Outcome() != application.ParticipationEndNotAccepted {
		body.Segment, body.Object = answer.segment, answer.object
	}
	// ADR-0022：只有这次真把那一条参与结束了用 201；已离场、不在段内、段不在册、未受理、未决都是形成了的答案（200）。
	status := http.StatusOK
	if answer.result.Outcome() == application.ParticipationEndedNow {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}

// ParticipationTerminationPayload 是明确终止口的线格式（票 operator-channel/15 定；此前本口只挂未配置、没有线格式）。
// 载荷只有内容、没有身份：租户从信封来，载荷带租户即拒。Basis 指向本上下文之外的处置决定，只能由调用方给。
type ParticipationTerminationPayload struct {
	Segment string `json:"segment"`
	Object  string `json:"object"`
	Basis   string `json:"basis"`
	EndedAt string `json:"endedAt"`
}

// Termination 把载荷连同信封给的租户翻成一次明确终止。时刻解不出是坏报文（400）。
func (payload ParticipationTerminationPayload) Termination(tenant domain.TenantID) (ParticipationTermination, error) {
	if tenant.String() == "" {
		return ParticipationTermination{}, ErrOperatorIdentityMissing
	}
	endedAt, err := parseOptionalInstant("endedAt", payload.EndedAt)
	if err != nil {
		return ParticipationTermination{}, err
	}
	return ParticipationTermination{TenantID: tenant, Segment: payload.Segment, Object: payload.Object, Basis: payload.Basis, EndedAt: endedAt}, nil
}
