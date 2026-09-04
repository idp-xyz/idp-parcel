package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// NotifyCustomerOutcome 是按披露决定生成并提交客户通知的应用处理结果。提交成功与提交
// 失败分成两格：失败是要分别记录的过程节点（通知本身已成立、按策略重试），不是未决。
type NotifyCustomerOutcome uint8

const (
	NotifyCustomerOutcomeInvalid NotifyCustomerOutcome = iota
	NotificationSubmitted
	NotificationSubmissionFailed
	NotificationExistingResult
	NotifyUndecided
	NotifyNotAccepted
)

func (outcome NotifyCustomerOutcome) String() string {
	switch outcome {
	case NotificationSubmitted:
		return "SUBMITTED"
	case NotificationSubmissionFailed:
		return "SUBMISSION_FAILED"
	case NotificationExistingResult:
		return "EXISTING_RESULT"
	case NotifyUndecided:
		return "UNDECIDED"
	case NotifyNotAccepted:
		return "NOT_ACCEPTED"
	default:
		return ""
	}
}

// NotifyCustomerUndecidedReason 指名本轮停在哪一步。`策略未配置`与`策略答不出`分开：
// 一个等租户把渠道与时限目录登记上，一个重试依赖，混起来会对着一个没配置的租户参数
// 无休止重试。
type NotifyCustomerUndecidedReason uint8

const (
	NotifyCustomerUndecidedReasonNone NotifyCustomerUndecidedReason = iota
	NotificationStoreUnavailable
	NotificationPolicyUnavailable
	NotificationPolicyNotConfigured
	NotificationIdentityUnavailable
	NotificationDecisionStoreUnavailable
)

func (reason NotifyCustomerUndecidedReason) String() string {
	switch reason {
	case NotificationStoreUnavailable:
		return "NOTIFICATION_STORE_UNAVAILABLE"
	case NotificationPolicyUnavailable:
		return "NOTIFICATION_POLICY_UNAVAILABLE"
	case NotificationPolicyNotConfigured:
		return "NOTIFICATION_POLICY_NOT_CONFIGURED"
	case NotificationIdentityUnavailable:
		return "NOTIFICATION_IDENTITY_UNAVAILABLE"
	case NotificationDecisionStoreUnavailable:
		return "NOTIFICATION_DECISION_STORE_UNAVAILABLE"
	default:
		return ""
	}
}

// NotifyCustomerCommand 指名要通知的是哪一份已登记的披露决定：（发作期 + 客户账户）是
// 决定登记册的当前键。命令不裸带一份决定——决定由 DecideDisclosureHandler 形成并入册，
// 这里只引用产物；裸带会让任何调用方都能凭空造一份「已披露」的决定送进通知，而披露
// 决定是本上下文拥有的判断，不是输入。通知的对象、内容与目标客户全部取自登记的那份
// 决定——编排不自造披露内容。租户显式随命令到达（ADR-0003）：客户账户引用只在租户内
// 唯一。
type NotifyCustomerCommand struct {
	TenantID domain.TenantID
	Episode  domain.EpisodeID
	Customer domain.CustomerAccountReference
}

type NotifyCustomerResult struct {
	outcome      NotifyCustomerOutcome
	notification *domain.CustomerNotification
	reason       NotifyCustomerUndecidedReason
	handoffRef   string
}

func (result NotifyCustomerResult) Outcome() NotifyCustomerOutcome {
	return result.outcome
}

// Notification 只在通知成立（本轮或此前）时给出。
func (result NotifyCustomerResult) Notification() (*domain.CustomerNotification, bool) {
	return result.notification, result.notification != nil
}

func (result NotifyCustomerResult) UndecidedReason() NotifyCustomerUndecidedReason {
	return result.reason
}

// HandoffReference 非空说明通知已成立但意图还没交出去，重放会重发同一份。
func (result NotifyCustomerResult) HandoffReference() string {
	return result.handoffRef
}

type NotifyCustomerDeps struct {
	Decisions     ports.DisclosureDecisionStore
	Notifications ports.CustomerNotificationStore
	Policy        ports.NotificationPolicyView
	Identities    ports.NotificationIdentityFactory
	Channel       ports.NotificationChannelGateway
	Downstream    ports.NotificationHandoff
	Clock         ports.Clock
}

type NotifyCustomerHandler struct {
	deps NotifyCustomerDeps
}

func NewNotifyCustomerHandler(deps NotifyCustomerDeps) *NotifyCustomerHandler {
	return &NotifyCustomerHandler{deps: deps}
}

// Handle 把一份已登记的`披露`结论决定推进到客户通知：受理（发作期与客户缺一即未受理）
// → 取回当前决定（没有决定就没有可通知的东西；非披露结论构不成通知——领域构造器已钉，
// 编排在门口就答未受理而不是撞领域错）→ 幂等按披露决定（已提交过渠道的不重发，只重发
// 同一份意图；上次提交失败的按策略重试，新节点接在失败后面）→ 策略取渠道与时限（未配置
// 即未决，不造渠道）→ 生成、提交渠道、分别记录节点 → 意图。查询与门户展示不经这里——
// 那是不同结果，本编排只做主动通知这一种。
func (handler *NotifyCustomerHandler) Handle(
	ctx context.Context,
	command NotifyCustomerCommand,
) (NotifyCustomerResult, error) {
	if command.TenantID.String() == "" ||
		command.Episode.String() == "" ||
		command.Customer.String() == "" {
		return NotifyCustomerResult{outcome: NotifyNotAccepted}, nil
	}

	disclosure, found, err := handler.deps.Decisions.FindCurrent(ctx, command.TenantID, command.Episode, command.Customer)
	if err != nil {
		return NotifyCustomerResult{outcome: NotifyUndecided, reason: NotificationDecisionStoreUnavailable}, nil
	}
	// 没有决定、或决定不是披露结论，都构不成通知：暂不披露与待授权都没有可通知的内容。
	// 这是门口的业务答案，不是等 GenerateNotification 报错——撞出来的错分不清是路由错了
	// 还是内容缺了。
	if !found || disclosure.Conclusion() != domain.DiscloseToCustomer {
		return NotifyCustomerResult{outcome: NotifyNotAccepted}, nil
	}

	existing, found, err := handler.deps.Notifications.FindByDisclosure(ctx, command.TenantID, disclosure)
	if err != nil {
		return NotifyCustomerResult{outcome: NotifyUndecided, reason: NotificationStoreUnavailable}, nil
	}
	if found {
		if existing.ObligationMetBy(domain.NotificationSubmittedToChannel) {
			// 同一披露已经提交过渠道：不重发通知，只把同一份意图再交一次（ADR-0043，
			// 重放重发同一份——只答已有结果就收工，一份首次发布失败的通知决定会永远
			// 停在「已成立、下游不知道」）。
			return NotifyCustomerResult{
				outcome:      NotificationExistingResult,
				notification: existing,
				handoffRef:   handler.handOffNotification(ctx, command.TenantID, existing),
			}, nil
		}
		// 上次提交失败：按策略重试。新节点接在后面，前面的失败保留——重试不是改写。
		return handler.submit(ctx, command.TenantID, existing)
	}

	directive, configured, err := handler.deps.Policy.DirectNotification(ctx, disclosure)
	if err != nil {
		return NotifyCustomerResult{outcome: NotifyUndecided, reason: NotificationPolicyUnavailable}, nil
	}
	if !configured {
		// 渠道与时限目录属待登记实例参数。没有渠道的通知不存在如实的空白格——造占位
		// 渠道是虚构，所以停在未决等租户登记，与依赖故障分开。
		return NotifyCustomerResult{outcome: NotifyUndecided, reason: NotificationPolicyNotConfigured}, nil
	}

	notificationID, err := handler.deps.Identities.NextNotificationID(ctx)
	if err != nil {
		return NotifyCustomerResult{outcome: NotifyUndecided, reason: NotificationIdentityUnavailable}, nil
	}

	notification, err := domain.GenerateNotification(
		notificationID,
		disclosure,
		directive.Deadline,
		directive.Channel,
		directive.Obligation,
		handler.deps.Clock.Now(),
	)
	if err != nil {
		// 披露结论在门口已经验过，走到这里还构不成通知，只剩策略答复缺渠道、时限或
		// 义务判据——那是端口坏答复，上抛而不吞。
		return NotifyCustomerResult{}, fmt.Errorf("generate customer notification: %w", err)
	}
	// 先落`已生成`再提交渠道：渠道那一次一旦发出就收不回来，通知记录必须先于它存在，
	// 否则一次落库失败会让「客户可能已收到」查无出处。
	saved, err := handler.deps.Notifications.Save(ctx, command.TenantID, notification)
	if err != nil {
		return NotifyCustomerResult{outcome: NotifyUndecided, reason: NotificationStoreUnavailable}, nil
	}
	switch saved {
	case ports.NotificationSaved:
		return handler.submit(ctx, command.TenantID, notification)
	case ports.NotificationAlreadyRecorded:
		existing, found, err := handler.deps.Notifications.FindByDisclosure(ctx, command.TenantID, disclosure)
		if err != nil || !found {
			return NotifyCustomerResult{outcome: NotifyUndecided, reason: NotificationStoreUnavailable}, nil
		}
		if existing.ObligationMetBy(domain.NotificationSubmittedToChannel) {
			return NotifyCustomerResult{
				outcome:      NotificationExistingResult,
				notification: existing,
				handoffRef:   handler.handOffNotification(ctx, command.TenantID, existing),
			}, nil
		}
		return handler.submit(ctx, command.TenantID, existing)
	default:
		return NotifyCustomerResult{}, fmt.Errorf("notify customer: unexpected save outcome %d", saved)
	}
}

// submit 执行一次渠道提交尝试并分别记录结果节点：成功记`已提交消息渠道`，失败记
// `失败`——两者都是过程事实，历史节点全保留。渠道提交与节点落库天然不原子（渠道
// 那一次已经发生），落库失败停在未决，重试会再提交一次；渠道侧去重属实例半边。
func (handler *NotifyCustomerHandler) submit(
	ctx context.Context,
	tenant domain.TenantID,
	notification *domain.CustomerNotification,
) (NotifyCustomerResult, error) {
	milestone := domain.NotificationSubmittedToChannel
	outcome := NotificationSubmitted
	if err := handler.deps.Channel.SubmitToChannel(ctx, notification); err != nil {
		milestone = domain.NotificationFailed
		outcome = NotificationSubmissionFailed
	}
	if err := notification.RecordMilestone(milestone, handler.deps.Clock.Now()); err != nil {
		return NotifyCustomerResult{}, fmt.Errorf("record notification milestone: %w", err)
	}
	saved, err := handler.deps.Notifications.Save(ctx, tenant, notification)
	if err != nil {
		return NotifyCustomerResult{outcome: NotifyUndecided, reason: NotificationStoreUnavailable}, nil
	}
	if saved != ports.NotificationSaved {
		return NotifyCustomerResult{}, fmt.Errorf("notify customer: unexpected save outcome %d", saved)
	}
	return NotifyCustomerResult{
		outcome:      outcome,
		notification: notification,
		handoffRef:   handler.handOffNotification(ctx, tenant, notification),
	}, nil
}

// handOffNotification 把通知决定交给适用下游，交不出去时交回发布续办引用。失败不改写
// 通知，也不算进未决——通知已经成立，要续办的是发布（ADR-0043）。
func (handler *NotifyCustomerHandler) handOffNotification(
	ctx context.Context,
	tenant domain.TenantID,
	notification *domain.CustomerNotification,
) string {
	if err := handler.deps.Downstream.HandOffNotification(ctx, ports.NotificationHandoffIntent{
		TenantID:     tenant,
		Notification: notification,
	}); err != nil {
		return "CONT-" + shortDigest("NOTIFICATION_HANDOFF", notification.ID().String())
	}
	return ""
}
