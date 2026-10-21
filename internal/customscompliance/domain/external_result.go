package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidExternalResult = errors.New("customs compliance: invalid external result")
	ErrLayerConflict         = errors.New("customs compliance: conflicting facts on the same layer")
)

// ResultLayer 是外部监管结果的封闭六层（CONTEXT「提交尝试、技术回执、监管接收、业务受理、监管过程决定、监管核定税费、放行结果和监管处置决定必须分层保存。任何前一层成功都不能自动生成后一层结果，也不能使用一个“清关成功”状态覆盖各层事实」——提交尝试与技术回执在提交对象上，这里是监管侧六层）。类型上没有任何
// 从一层派生另一层的方法：不虚构缺失结果是结构性的。
type ResultLayer uint8

const (
	ResultLayerInvalid ResultLayer = iota
	RegulatoryReceiptLayer
	BusinessAcceptanceLayer
	ProcessDecisionLayer
	AssessedDutyLayer
	ReleaseResultLayer
	DispositionDecisionLayer
)

func (layer ResultLayer) valid() bool {
	return layer >= RegulatoryReceiptLayer && layer <= DispositionDecisionLayer
}

func (layer ResultLayer) String() string {
	switch layer {
	case RegulatoryReceiptLayer:
		return "REGULATORY_RECEIPT"
	case BusinessAcceptanceLayer:
		return "BUSINESS_ACCEPTANCE"
	case ProcessDecisionLayer:
		return "PROCESS_DECISION"
	case AssessedDutyLayer:
		return "ASSESSED_DUTY"
	case ReleaseResultLayer:
		return "RELEASE_RESULT"
	case DispositionDecisionLayer:
		return "DISPOSITION_DECISION"
	default:
		return ""
	}
}

// SourceAuthorityRole 指名来源在该结果层的权威角色。技术中介或报关服务商的响应只有
// 在来源语义明确代表监管机构时才能形成监管事实（186）——角色引用正是那个判断的落点。
type SourceAuthorityRole struct{ requiredValue }

func NewSourceAuthorityRole(value string) (SourceAuthorityRole, error) {
	required, err := newRequiredValue("source authority role", value)
	return SourceAuthorityRole{required}, err
}

// InterpretationRuleReference 指名实际采用的解释规则。
type InterpretationRuleReference struct{ requiredValue }

func NewInterpretationRuleReference(value string) (InterpretationRuleReference, error) {
	required, err := newRequiredValue("interpretation rule reference", value)
	return InterpretationRuleReference{required}, err
}

// ExternalResultSpec 是保存一项外部结果所需的全部输入（CONTEXT「每项外部结果必须保存来源身份……」逐件：来源身份、权威角色、原始语义、业务发生或适用时间、接收时间、解释规则、
// 与提交版本/尝试和明确结果范围的关系）。
type ExternalResultSpec struct {
	Layer        ResultLayer
	SourceID     string
	Role         SourceAuthorityRole
	RawSemantics string
	Rule         InterpretationRuleReference
	Version      SubmissionVersionID
	Attempt      int
	Scope        DecisionScopeReference
	OccurredAt   time.Time
	ReceivedAt   time.Time
}

// ExternalResult 是已接收并解释的一项外部监管事实。八件缺一立不起；查询请求、查询
// 执行状态与查询技术结果改变不了原提交的业务判断（188）——那些不是外部结果，构造
// 不出这个类型。
type ExternalResult struct {
	layer        ResultLayer
	sourceID     string
	role         SourceAuthorityRole
	rawSemantics string
	rule         InterpretationRuleReference
	version      SubmissionVersionID
	attempt      int
	scope        DecisionScopeReference
	occurredAt   time.Time
	receivedAt   time.Time
}

func InterpretExternalResult(spec ExternalResultSpec) (ExternalResult, error) {
	if !spec.Layer.valid() ||
		spec.SourceID == "" ||
		!spec.Role.valid() ||
		spec.RawSemantics == "" ||
		!spec.Rule.valid() ||
		!spec.Version.valid() ||
		spec.Attempt <= 0 ||
		!spec.Scope.valid() ||
		spec.OccurredAt.IsZero() ||
		spec.ReceivedAt.IsZero() {
		return ExternalResult{}, ErrInvalidExternalResult
	}
	return ExternalResult{
		layer:        spec.Layer,
		sourceID:     spec.SourceID,
		role:         spec.Role,
		rawSemantics: spec.RawSemantics,
		rule:         spec.Rule,
		version:      spec.Version,
		attempt:      spec.Attempt,
		scope:        spec.Scope,
		occurredAt:   spec.OccurredAt.UTC(),
		receivedAt:   spec.ReceivedAt.UTC(),
	}, nil
}

func (result ExternalResult) Layer() ResultLayer {
	return result.layer
}

// SourceID、Rule 与 Attempt 是持久化重建的必需读口——八件里这三件没有出口，登记册
// 适配器连原样写回都做不到。
func (result ExternalResult) SourceID() string {
	return result.sourceID
}

func (result ExternalResult) Rule() InterpretationRuleReference {
	return result.rule
}

func (result ExternalResult) Attempt() int {
	return result.attempt
}

func (result ExternalResult) Role() SourceAuthorityRole {
	return result.role
}

func (result ExternalResult) RawSemantics() string {
	return result.rawSemantics
}

func (result ExternalResult) Version() SubmissionVersionID {
	return result.version
}

func (result ExternalResult) Scope() DecisionScopeReference {
	return result.scope
}

func (result ExternalResult) OccurredAt() time.Time {
	return result.occurredAt
}

func (result ExternalResult) ReceivedAt() time.Time {
	return result.receivedAt
}

// LayerFact 是同层一致性核对读的一份事实：分层事实本体，放行层另带同一次接收拆出的放行
// 事实。放行层的先后要按放行种类判，而种类不在分层事实上，所以两面一起交进来。
type LayerFact struct {
	Result  ExternalResult
	Release *CustomsReleaseOutcome
}

// CheckLayerConsistency 把新到结果与同一提交版本的在册事实逐一比对，只在新到那份与某一份
// 同层、同范围的在册事实相互矛盾时交出 ErrLayerConflict——独立哨兵，调用方保留双方事实，
// 不选边。什么算矛盾、什么只是同层后续事件，判法在 CONTEXT「同层后续事件不是同层冲突」
// 那一条，取舍见 ADR-0226。不同层或不同范围的事实各归各位。
//
// 在册事实里那些已经记了冲突的也照样比：冲突关系在两份事实之间，被标的那份仍是留存的
// 来源事实。这样未记冲突的事实两两都比过、两两不矛盾，读当前判断的一方拿到的就是一条
// 自洽的序列，与到达先后无关。
func CheckLayerConsistency(existing []LayerFact, incoming LayerFact) error {
	if err := incoming.wellFormed(); err != nil {
		return err
	}
	if incoming.Result.layer == ReleaseResultLayer && incoming.Release == nil {
		return ErrInvalidExternalResult
	}
	for _, fact := range existing {
		if err := fact.wellFormed(); err != nil {
			return err
		}
		if fact.Result.layer != incoming.Result.layer ||
			fact.Result.version != incoming.Result.version ||
			fact.Result.scope != incoming.Result.scope {
			continue
		}
		if contradicts(fact, incoming) {
			return ErrLayerConflict
		}
	}
	return nil
}

// wellFormed 守住放行事实与层次的配对：只有放行层带放行事实，且放行范围就是结果范围
// （接收编排两头都这么落）。放行层缺放行事实只放过在册那一侧——放行事实有自己的落点
// 之前，放行层只以分层事实落库，那批存量说不出种类；新到的放行层在核对入口另行要求带着它。
func (fact LayerFact) wellFormed() error {
	if !fact.Result.layer.valid() {
		return ErrInvalidExternalResult
	}
	if fact.Release == nil {
		return nil
	}
	if fact.Result.layer != ReleaseResultLayer || fact.Release.scope != fact.Result.scope {
		return ErrInvalidExternalResult
	}
	return nil
}

// contradicts 判同层、同版本、同范围的两份事实是否相互矛盾。对两份事实对称：谁先到达
// 不进判断，先后只按业务发生时间——CONTEXT「不得仅按消息到达顺序覆盖」。
func contradicts(one, other LayerFact) bool {
	switch one.Result.layer {
	case ReleaseResultLayer:
		if one.Release == nil || other.Release == nil {
			// 说不出种类就说不出谁比谁完整，排不进放行的先后，退回按原始语义判。
			return one.Result.rawSemantics != other.Result.rawSemantics
		}
		return releasesContradict(one, other)
	case ProcessDecisionLayer:
		// 过程决定是先后发生的监管事件，后一项不否定前一项；只有同一时刻说两样排不出先后。
		return one.Result.occurredAt.Equal(other.Result.occurredAt) &&
			one.Result.rawSemantics != other.Result.rawSemantics
	default:
		// 监管接收、业务受理、监管核定税费与监管处置决定对一版提交只给一个结论：换一种说法
		// 要么是矛盾、要么是更正，而更正得由来源指明，不能从业务时间猜。
		return one.Result.rawSemantics != other.Result.rawSemantics
	}
}

// releasesContradict 按放行种类判：同一时刻种类不同即矛盾；时刻不同时，后一份的种类不窄于
// 前一份才是推进。种类之外的原始语义与条件说明不进判断——那是同一结论的来源措辞与附注，
// 两次部分放行措辞不同，说的仍是这一范围部分放行。
func releasesContradict(one, other LayerFact) bool {
	if one.Result.occurredAt.Equal(other.Result.occurredAt) {
		return one.Release.kind != other.Release.kind
	}
	earlier, later := one, other
	if later.Result.occurredAt.Before(earlier.Result.occurredAt) {
		earlier, later = later, earlier
	}
	return !later.Release.kind.atLeastAsComplete(earlier.Release.kind)
}

// atLeastAsComplete 是放行种类的偏序：全部放行不窄于任何一种，部分与附条件各只不窄于
// 自己，两者之间不可比——附条件放行说不出它盖的是剩余范围还是已放的那部分，后者等于给
// 已放行的货补上条件。
func (kind ReleaseKind) atLeastAsComplete(other ReleaseKind) bool {
	return kind == other || kind == FullRelease
}
