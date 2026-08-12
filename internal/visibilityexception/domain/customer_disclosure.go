package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidDisclosure   = errors.New("visibility exception: invalid disclosure decision")
	ErrInvalidNotification = errors.New("visibility exception: invalid customer notification")
)

// DisclosurePolicyReference 指名版本化信息披露规则（产品、合同、可信度、影响范围与
// 披露规则共同决定客户可见性——CONTEXT 硬句 159）。
type DisclosurePolicyReference struct{ requiredValue }

func NewDisclosurePolicyReference(value string) (DisclosurePolicyReference, error) {
	required, err := newRequiredValue("disclosure policy reference", value)
	return DisclosurePolicyReference{required}, err
}

// DisclosureContentReference 指名披露内容快照。跨客户内部案件按客户分别形成披露
// 内容——快照按账户隔离。
type DisclosureContentReference struct{ requiredValue }

func NewDisclosureContentReference(value string) (DisclosureContentReference, error) {
	required, err := newRequiredValue("disclosure content reference", value)
	return DisclosureContentReference{required}, err
}

// DisclosureConclusion 是披露决定的封闭三值（CONTEXT 硬句 165：「披露条件不成立或
// 授权不足时分别形成暂不披露或待授权结果」——两个非披露格分开，等条件与等授权是
// 不同的续办路）。
type DisclosureConclusion uint8

const (
	DisclosureConclusionInvalid DisclosureConclusion = iota
	DiscloseToCustomer
	NotYetDisclosable
	AwaitingAuthorization
)

func (conclusion DisclosureConclusion) valid() bool {
	return conclusion >= DiscloseToCustomer && conclusion <= AwaitingAuthorization
}

func (conclusion DisclosureConclusion) String() string {
	switch conclusion {
	case DiscloseToCustomer:
		return "DISCLOSE"
	case NotYetDisclosable:
		return "NOT_YET_DISCLOSABLE"
	case AwaitingAuthorization:
		return "AWAITING_AUTHORIZATION"
	default:
		return ""
	}
}

// DisclosureDecision 是从内部信号或案件形成的客户可见异常版本化决定。内部信号、内部
// 案件和客户可见异常不是同一对象（CONTEXT 159）——这里只引用内部来源；异常案件存在
// 不自动要求披露（165）。披露格必带内容快照，非披露格必不带——无中生有与有中不给
// 两向都在构造期拦下。
type DisclosureDecision struct {
	episode    EpisodeID
	customer   CustomerAccountReference
	policy     DisclosurePolicyReference
	conclusion DisclosureConclusion
	content    DisclosureContentReference
	decidedAt  time.Time
}

func DecideDisclosure(
	episode EpisodeID,
	customer CustomerAccountReference,
	policy DisclosurePolicyReference,
	conclusion DisclosureConclusion,
	content DisclosureContentReference,
	decidedAt time.Time,
) (DisclosureDecision, error) {
	if !episode.valid() || !customer.valid() || !policy.valid() ||
		!conclusion.valid() || decidedAt.IsZero() {
		return DisclosureDecision{}, ErrInvalidDisclosure
	}
	if conclusion == DiscloseToCustomer && !content.valid() {
		return DisclosureDecision{}, ErrInvalidDisclosure
	}
	if conclusion != DiscloseToCustomer && content.valid() {
		return DisclosureDecision{}, ErrInvalidDisclosure
	}
	return DisclosureDecision{
		episode:    episode,
		customer:   customer,
		policy:     policy,
		conclusion: conclusion,
		content:    content,
		decidedAt:  decidedAt.UTC(),
	}, nil
}

func (decision DisclosureDecision) Episode() EpisodeID {
	return decision.episode
}

func (decision DisclosureDecision) Customer() CustomerAccountReference {
	return decision.customer
}

func (decision DisclosureDecision) Policy() DisclosurePolicyReference {
	return decision.policy
}

func (decision DisclosureDecision) Conclusion() DisclosureConclusion {
	return decision.conclusion
}

// Content 只在披露格给出。
func (decision DisclosureDecision) Content() (DisclosureContentReference, bool) {
	return decision.content, decision.conclusion == DiscloseToCustomer
}

// NotificationID 是客户异常通知决定的标识。
type NotificationID struct{ requiredValue }

func NewNotificationID(value string) (NotificationID, error) {
	required, err := newRequiredValue("notification ID", value)
	return NotificationID{required}, err
}

// NotificationChannelReference 指名适用渠道。
type NotificationChannelReference struct{ requiredValue }

func NewNotificationChannelReference(value string) (NotificationChannelReference, error) {
	required, err := newRequiredValue("notification channel reference", value)
	return NotificationChannelReference{required}, err
}

// NotificationMilestone 是通知过程节点的封闭六值（CONTEXT 硬句 163：已生成、已提交
// 消息渠道、渠道已接受、已送达、失败和客户确认分别记录）。
type NotificationMilestone uint8

const (
	NotificationMilestoneInvalid NotificationMilestone = iota
	NotificationGenerated
	NotificationSubmittedToChannel
	NotificationChannelAccepted
	NotificationDelivered
	NotificationFailed
	CustomerConfirmed
)

func (milestone NotificationMilestone) valid() bool {
	return milestone >= NotificationGenerated && milestone <= CustomerConfirmed
}

func (milestone NotificationMilestone) String() string {
	switch milestone {
	case NotificationGenerated:
		return "GENERATED"
	case NotificationSubmittedToChannel:
		return "SUBMITTED_TO_CHANNEL"
	case NotificationChannelAccepted:
		return "CHANNEL_ACCEPTED"
	case NotificationDelivered:
		return "DELIVERED"
	case NotificationFailed:
		return "FAILED"
	case CustomerConfirmed:
		return "CUSTOMER_CONFIRMED"
	default:
		return ""
	}
}

// CustomerNotification 是客户异常通知决定：通知对象、内容快照、披露依据、目标客户、
// 要求时限和适用渠道六件必备（CONTEXT 硬句 162）。类型上没有源事实、案件责任、限制
// 或索赔字段——客户通知、确认或异议都不能直接修改它们（164）；门户展示能否满足通知
// 义务由合同另行判断，这里只带义务判据引用。
type CustomerNotification struct {
	id         NotificationID
	disclosure DisclosureDecision
	customer   CustomerAccountReference
	content    DisclosureContentReference
	deadline   time.Time
	channel    NotificationChannelReference
	obligation DisclosurePolicyReference
	milestones []NotificationMilestone
	recordedAt []time.Time
}

// GenerateNotification 依据一份`披露`结论的决定生成通知。非披露结论生成不了通知——
// 暂不披露与待授权都没有可通知的内容。
func GenerateNotification(
	id NotificationID,
	disclosure DisclosureDecision,
	deadline time.Time,
	channel NotificationChannelReference,
	obligation DisclosurePolicyReference,
	generatedAt time.Time,
) (*CustomerNotification, error) {
	if !id.valid() || !channel.valid() || !obligation.valid() ||
		deadline.IsZero() || generatedAt.IsZero() {
		return nil, ErrInvalidNotification
	}
	content, disclosed := disclosure.Content()
	if !disclosed {
		return nil, ErrInvalidNotification
	}
	return &CustomerNotification{
		id:         id,
		disclosure: disclosure,
		customer:   disclosure.customer,
		content:    content,
		deadline:   deadline.UTC(),
		channel:    channel,
		obligation: obligation,
		milestones: []NotificationMilestone{NotificationGenerated},
		recordedAt: []time.Time{generatedAt.UTC()},
	}, nil
}

func (notification *CustomerNotification) ID() NotificationID {
	return notification.id
}

func (notification *CustomerNotification) Customer() CustomerAccountReference {
	return notification.customer
}

func (notification *CustomerNotification) Content() DisclosureContentReference {
	return notification.content
}

func (notification *CustomerNotification) Deadline() time.Time {
	return notification.deadline
}

func (notification *CustomerNotification) Obligation() DisclosurePolicyReference {
	return notification.obligation
}

// Milestones 给出全部已记录节点（副本，按记录顺序）。
func (notification *CustomerNotification) Milestones() []NotificationMilestone {
	return append([]NotificationMilestone(nil), notification.milestones...)
}

// RecordMilestone 追加一个过程节点：分别记录不覆盖（历史节点全保留——失败后重试产生
// 的新节点接在后面，前面的失败还在）；时间不倒流。
func (notification *CustomerNotification) RecordMilestone(milestone NotificationMilestone, at time.Time) error {
	if !milestone.valid() || milestone == NotificationGenerated {
		// 已生成随构造记录一次；再记一遍是把生成当成了可重复动作。
		return ErrInvalidNotification
	}
	if at.IsZero() || at.Before(notification.recordedAt[len(notification.recordedAt)-1]) {
		return ErrInvalidNotification
	}
	notification.milestones = append(notification.milestones, milestone)
	notification.recordedAt = append(notification.recordedAt, at.UTC())
	return nil
}

// ObligationMetBy 报告按给定的义务节点要求，本通知是否已满足义务——只有相应结果
// 成立才满足（合同要求送达就查送达、要求确认就查确认，CONTEXT 硬句 163）；哪个节点
// 是要求来自义务判据，这里不猜。
func (notification *CustomerNotification) ObligationMetBy(required NotificationMilestone) bool {
	if !required.valid() {
		return false
	}
	for _, milestone := range notification.milestones {
		if milestone == required {
			return true
		}
	}
	return false
}
