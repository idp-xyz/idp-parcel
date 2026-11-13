package visibilityhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
)

// 本文件是客户异常通知的运营决定口（UC-VE-006 步 6）：把一期信号上已成立的`披露`决定通知客户。载荷与披露两口相同——
// 指名哪件包裹上哪一类信号的哪一期；客户、内容、渠道与时限都不在载荷里。首次发送与失败后的重发打同一个口。

// CustomerExceptionNotifier 是本口转交的编排（application.NotifyCustomerHandler）。
type CustomerExceptionNotifier interface {
	HandleSignal(ctx context.Context, command application.NotifySignalCommand) (application.NotifyCustomerResult, error)
}

type CustomerExceptionNotificationIntake interface {
	IntakeCustomerExceptionNotification(ctx context.Context, request *http.Request) (application.NotifySignalCommand, error)
}

var _ CustomerExceptionNotificationIntake = (*OperatorDecisionIntake)(nil)

// IntakeCustomerExceptionNotification 按客户异常通知这一种决定认证。
func (intake *OperatorDecisionIntake) IntakeCustomerExceptionNotification(ctx context.Context, request *http.Request) (application.NotifySignalCommand, error) {
	target, err := intake.customerDisclosureTarget(ctx, request, OperatorDecisionCustomerExceptionNotification)
	if err != nil {
		return application.NotifySignalCommand{}, err
	}
	return application.NotifySignalCommand{
		TenantID: target.operator.Tenant, Parcel: target.parcel, Kind: target.kind, Episode: target.episode, Operator: target.operator.Operator,
	}, nil
}

func NewNotifyCustomerExceptionEndpoint(intake CustomerExceptionNotificationIntake, notifier CustomerExceptionNotifier) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeCustomerExceptionNotification(request.Context(), request)
		if err != nil {
			writeCaseDecisionIntakeProblem(response, err)
			return
		}
		result, err := notifier.HandleSignal(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeCustomerExceptionNotification(response, result)
	})
}

type notificationMilestoneResponse struct {
	Milestone  string `json:"milestone"`
	RecordedAt string `json:"recordedAt"`
}

// customerExceptionNotificationResponse 是本口的封闭响应形状。通知各格只在通知在场时交回；reason 只在未决时交回。
type customerExceptionNotificationResponse struct {
	Outcome        string                          `json:"outcome"`
	Reason         string                          `json:"reason,omitempty"`
	NotificationID string                          `json:"notificationId,omitempty"`
	EpisodeID      string                          `json:"episodeId,omitempty"`
	Customer       string                          `json:"customer,omitempty"`
	Channel        string                          `json:"channel,omitempty"`
	Content        string                          `json:"content,omitempty"`
	Deadline       string                          `json:"deadline,omitempty"`
	Milestones     []notificationMilestoneResponse `json:"milestones,omitempty"`
}

// writeCustomerExceptionNotification 逐名转写。未决分两种：通知策略未配置、渠道未登记、凭据变量没有值、客户归属不可
// 确定是业务答案——都等某一项配置或委托，重试不会变——答 200 并带原因；其余未决是某一只依赖读写不动，答 5xx。只有
// 本轮真提交给了渠道才 201；提交失败是记下了`失败`节点的业务答案（200），可以再打同一个口重发。
func writeCustomerExceptionNotification(response http.ResponseWriter, result application.NotifyCustomerResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := customerExceptionNotificationResponse{Outcome: outcome}
	if result.Outcome() == application.NotifyUndecided {
		switch result.UndecidedReason() {
		case application.NotificationPolicyNotConfigured, application.NotificationChannelNotConfigured,
			application.NotificationChannelCredentialMissing, application.NotificationCustomerAccountUndeterminable:
			body.Reason = result.UndecidedReason().String()
		default:
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
	} else if notification, present := result.Notification(); present {
		const instant = "2006-01-02T15:04:05.999999Z07:00"
		body.NotificationID = notification.ID().String()
		body.EpisodeID = notification.Disclosure().Episode().String()
		body.Customer = notification.Customer().String()
		body.Channel = notification.Channel().String()
		body.Content = notification.Content().String()
		body.Deadline = notification.Deadline().UTC().Format(instant)
		recorded := notification.RecordedAt()
		for index, milestone := range notification.Milestones() {
			body.Milestones = append(body.Milestones, notificationMilestoneResponse{
				Milestone: milestone.String(), RecordedAt: recorded[index].UTC().Format(instant),
			})
		}
	}
	status := http.StatusOK
	if result.Outcome() == application.NotificationSubmitted {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
