package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 外部承运轨迹事实的收编侧端口（label-channel/16，落文 ADR-0102）。素材由 TrackingSource
// 交来（tracking_source.go），这里是它被认领之后的登记册、留痕册、身份签发，以及收编时要问的
// 两件实例参数——凭证指向哪个对象、该源有没有有效时间规则。

// ExternalTrackingFactKey 是一条外部承运轨迹事实某一版的幂等键。版本在键里：源声明更正与
// 本上下文的有效时间判断都形成新版本，原版本不被改写。
type ExternalTrackingFactKey struct {
	TenantID domain.TenantID
	Fact     domain.ExternalTrackingFactReference
	Version  domain.ExternalTrackingFactVersion
}

// ExternalTrackingFactRecord 是一个版本越过提交边界留下的东西。RecordedAt 是认领落库的时刻，
// 与事实上的 ReceivedAt（接到素材的时刻）是两个时间，不合并。
type ExternalTrackingFactRecord struct {
	Key        ExternalTrackingFactKey
	Fact       domain.ExternalCarrierTrackingFact
	RecordedAt time.Time
}

// ExternalTrackingFactSaveOutcome 是登记一个版本的结果。撞键是业务答案不是错误（ADR-0031）。
type ExternalTrackingFactSaveOutcome uint8

const (
	ExternalTrackingFactSaveOutcomeInvalid ExternalTrackingFactSaveOutcome = iota
	ExternalTrackingFactSaved
	ExternalTrackingFactAlreadyRegistered
)

func (outcome ExternalTrackingFactSaveOutcome) String() string {
	switch outcome {
	case ExternalTrackingFactSaved:
		return "SAVED"
	case ExternalTrackingFactAlreadyRegistered:
		return "ALREADY_REGISTERED"
	default:
		return ""
	}
}

// ExternalTrackingFactRegistry 按键找回并登记外部承运轨迹事实。只插不改。
//
// FindBySourceEvent 是接收形态的幂等锚（源引用 + 源事件标识，ADR-0090 决定四）与更正解析的
// 共用入口：同一锚再次到达即重复投递；源声明「更正了 X」时也按它找 X 落在哪条事实上。
// 源未给事件标识的素材从不经过它——那类素材不判重。
//
// FindCurrent 交回一条事实此刻未被任何版本回指的那一版。更正要回指的是当前版而不是被更正
// 事件所在的那一版：中间可能已有一次判断换过版本。
type ExternalTrackingFactRegistry interface {
	FindByKey(ctx context.Context, key ExternalTrackingFactKey) (ExternalTrackingFactRecord, bool, error)
	FindBySourceEvent(
		ctx context.Context,
		tenant domain.TenantID,
		source domain.TrackingSourceReference,
		event domain.SourceEventReference,
	) (ExternalTrackingFactRecord, bool, error)
	FindCurrent(
		ctx context.Context,
		tenant domain.TenantID,
		fact domain.ExternalTrackingFactReference,
	) (ExternalTrackingFactRecord, bool, error)
	Save(ctx context.Context, record ExternalTrackingFactRecord) (ExternalTrackingFactSaveOutcome, error)
}

// ExternalTrackingIdentityFactory 签发本上下文自己的事实身份与版本。源事件标识由源给、本仓
// 不代铸；本仓自己的身份另铸，两者分开保存（ADR-0102 决定四）。
type ExternalTrackingIdentityFactory interface {
	NextExternalTrackingFactReference(ctx context.Context) (domain.ExternalTrackingFactReference, error)
	NextExternalTrackingFactVersion(ctx context.Context) (domain.ExternalTrackingFactVersion, error)
}

// UnadoptedReason 说一条素材为什么没有成为事实。两格的续办不同：源未给发生时间只能等源改
// 报文；凭证不认识要去查凭证登记。
type UnadoptedReason uint8

const (
	UnadoptedReasonInvalid UnadoptedReason = iota
	UnadoptedOccurredAtNotGiven
	UnadoptedCredentialUnknown
)

func (reason UnadoptedReason) String() string {
	switch reason {
	case UnadoptedOccurredAtNotGiven:
		return "OCCURRED_AT_NOT_GIVEN_BY_SOURCE"
	case UnadoptedCredentialUnknown:
		return "CREDENTIAL_UNKNOWN"
	default:
		return ""
	}
}

// UnadoptedTrackingMaterial 是一条没有成为事实的素材的留痕。它如实记源给了什么、没给什么，
// 不进事实库、不进投影；本体不存，只留摘要——「我们确实收到过这条」的全部证据。SourceEvent
// 允许为空：源未给事件标识的素材同样如实留痕，不按内容判重。
type UnadoptedTrackingMaterial struct {
	TenantID      domain.TenantID
	Source        TrackingSourceReference
	SourceEvent   string
	Credential    string
	Status        string
	ReceivedAt    time.Time
	PayloadDigest string
	Reason        UnadoptedReason
	RecordedAt    time.Time
}

// UnadoptedTrackingMaterialLedger 只追加。留痕不是拒收：素材没有丢，所有者随时能看见
// 「这家源给了多少条没有时间的状态」（ADR-0102 Consequences）。
type UnadoptedTrackingMaterialLedger interface {
	RecordUnadopted(ctx context.Context, entry UnadoptedTrackingMaterial) error
}

// CredentialResolution 是「这份外部承运凭证指向哪个载运对象」的答案格。`未配置`单列：没有凭证
// 登记册时任何素材都认领不了，那是本上下文自己的缺口，不是源的缺陷，因此不留痕、只报未决。
//
// `未知`是「登记册此刻解析不到一个载运对象」而不只是「没登记过」：凭证从未登记、已作废/失效/
// 替代、或它标识的是运输委托、订舱、班次、实际履约段而非载运对象（CONTEXT「不能全部解释为包裹
// 的当前运单号」），都落这一格——四种情形的续办是同一个动作，去查凭证登记；把它们分开会让收编
// 执行器替登记册解释凭证。登记册的实现见 adapters/postgres 的 ExternalCarrierCredentials。
type CredentialResolution uint8

const (
	CredentialResolutionInvalid CredentialResolution = iota
	CredentialResolved
	CredentialUnknown
	CredentialRegistryUnconfigured
)

func (resolution CredentialResolution) String() string {
	switch resolution {
	case CredentialResolved:
		return "RESOLVED"
	case CredentialUnknown:
		return "UNKNOWN"
	case CredentialRegistryUnconfigured:
		return "REGISTRY_UNCONFIGURED"
	default:
		return ""
	}
}

// ExternalCarrierCredentialResolver 把凭证引用解析到它真实标识的载运对象。凭证的登记属
// CONTEXT「外部承运凭证」（分配方、真实标识对象、适用范围、版本），本口只问「此刻指向谁」。
// 对象引用只在 CredentialResolved 时有意义。
type ExternalCarrierCredentialResolver interface {
	ResolveCredential(
		ctx context.Context,
		tenant domain.TenantID,
		credential domain.ExternalCarrierCredentialReference,
	) (domain.CarriedObjectReference, CredentialResolution, error)
}

// EffectiveTimeRuleOutcome 说该源此刻有没有可用的有效时间规则。`无规则`是如实的一格而不是
// 失败：事实照样成立，只是保持「有效时间待判断」、不提供给投影。
type EffectiveTimeRuleOutcome uint8

const (
	EffectiveTimeRuleOutcomeInvalid EffectiveTimeRuleOutcome = iota
	EffectiveTimeRuleAbsent
	EffectiveTimeRuleApplied
)

func (outcome EffectiveTimeRuleOutcome) String() string {
	switch outcome {
	case EffectiveTimeRuleAbsent:
		return "RULE_ABSENT"
	case EffectiveTimeRuleApplied:
		return "RULE_APPLIED"
	default:
		return ""
	}
}

// EffectiveTimeRuleInput 是规则能看到的全部：这条素材自己的几件事。规则的形状与取值是实例
// 参数（`PAR-INT-02`），本口不预设它用哪几件。
type EffectiveTimeRuleInput struct {
	TenantID   domain.TenantID
	Source     domain.TrackingSourceReference
	Status     domain.RawStatusReference
	OccurredAt time.Time
	ReceivedAt time.Time
}

// EffectiveTimeRuling 是规则的答复。Rule 与 EffectiveAt 只在 EffectiveTimeRuleApplied 时有
// 意义；规则必须带版本，事实上要记下按哪一版判的。
type EffectiveTimeRuling struct {
	Outcome     EffectiveTimeRuleOutcome
	Rule        domain.EffectiveTimeRuleReference
	EffectiveAt time.Time
}

// EffectiveTimeRules 是「按该源已登记并带版本的规则形成有效时间」这一路的入口。**没有规则时
// 必须答 EffectiveTimeRuleAbsent，不得默认「等于发生时间」**——那是 ADR-0102 点名否决的最像
// 无害的默认值。
type EffectiveTimeRules interface {
	JudgeEffectiveTime(ctx context.Context, input EffectiveTimeRuleInput) (EffectiveTimeRuling, error)
}

// ExternalTrackingFactHandoffIntent 把一个**有效时间已判断**的版本交给 visibility-exception
// 作运输履约来源事实。待判断的版本没有可交的东西，不产生意图。
type ExternalTrackingFactHandoffIntent struct {
	Record ExternalTrackingFactRecord
}

// ExternalTrackingFactHandoff 由 OutboxExternalTrackingFactHandoff 实现：意图与登记同一事务
// 入队。
type ExternalTrackingFactHandoff interface {
	HandOffExternalTrackingFact(ctx context.Context, intent ExternalTrackingFactHandoffIntent) error
}
