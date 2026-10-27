package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidExternalResult = errors.New("customs compliance: invalid external result")
	ErrLayerConflict         = errors.New("customs compliance: conflicting facts on the same layer")
	// ErrCorrectionOutsideSubmission：更正所指的那份不是同一提交版本的在册事实。更正只在同一层次与业务事项
	// 上成立（UC-CC-006「同一层次和业务事项的后续明确更正」），跨提交版本的「更正」是矛盾输入。
	ErrCorrectionOutsideSubmission = errors.New("customs compliance: the corrected result is not a booked fact of the same submission version")
	// ErrCorrectionAcrossLayers：更正所指的那份在另一层。跨层「更正」等于让一层的结果改写另一层的事实，
	// 正是 CONTEXT「任何前一层成功都不能自动生成后一层结果」禁的那种推导。
	ErrCorrectionAcrossLayers = errors.New("customs compliance: a correction names a result on another layer")
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

// SourceAuthorityRole 指名来源在该结果层的权威角色。CONTEXT Rules「外部结果与规则时效」一节：「技术中介或报关服务商的响应只有
// 在来源语义明确代表监管机构时，才能形成相应监管事实」——角色引用正是那个判断的落点。
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
	// Corrects 是来源明确声明本结果所更正的那一份结果的来源标识，没有更正语义时为空。它是来源内容
	// （UC-CC-006 来源结果内容「来源更正关系（如有）」），不由本上下文从业务时间或语义差别推出。
	Corrects string
}

// ExternalResult 是已接收并解释的一项外部监管事实。各必备件缺一立不起；CONTEXT Rules「外部结果与规则时效」一节：「查询请求、
// 查询尝试、查询执行状态和查询技术结果不得直接改变原提交的业务判断」——那些不是外部结果，构造不出这个类型。
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
	corrects     string
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
		spec.ReceivedAt.IsZero() ||
		spec.Corrects == spec.SourceID ||
		(spec.Corrects != "" && strings.TrimSpace(spec.Corrects) == "") {
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
		corrects:     spec.Corrects,
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

// Corrects 交回来源声明本结果所更正的那一份的来源标识；没有更正语义时第二个返回值为 false。
func (result ExternalResult) Corrects() (string, bool) {
	return result.corrects, result.corrects != ""
}

// LayerFact 是同层一致性核对读的一份事实：分层事实本体，放行层另带同一次接收拆出的放行
// 事实。放行层的先后要按放行种类判，而种类不在分层事实上，所以两面一起交进来。
//
// CorrectionInForce 说这份到达时，它对所更正那份的更正关系在同一范围上生效了：自这份更正的
// 业务发生时间起，所更正那份不再算数——当前判断按时点取，此后到达的结果只在业务发生时间不早于
// 更正时不再与它比；原记录留作历史。它是接收那一刻的判断，与冲突标记一样只建立不改写。
type LayerFact struct {
	Result            ExternalResult
	Release           *CustomsReleaseOutcome
	CorrectionInForce bool
}

// CheckLayerConsistency 把新到结果与同一提交版本的在册事实逐一比对，只在新到那份与某一份
// 同层、同范围的在册事实相互矛盾时交出 ErrLayerConflict——独立哨兵，调用方保留双方事实，
// 不选边。什么算矛盾、什么只是同层后续事件，判法在 CONTEXT「同层后续事件不是同层冲突」
// 那一条，取舍见 ADR-0226。不同层或不同范围的事实各归各位。
//
// 在册事实里那些已经记了冲突的也照样比：冲突关系在两份事实之间，被标的那份仍是留存的
// 来源事实。这样未记冲突的事实两两都比过、两两不矛盾，按业务时间排成的序列自洽（两两都比过
// 要靠调用方把比对读与落册串行，见 ports.ExternalResultStore）。矛盾不传递：部分、附条件、
// 全部三份依次发生时，全部与另两份都不矛盾，部分与附条件却矛盾，被标的是两者中后到的那份
// ——真矛盾时标哪一份仍取决于到达先后。
//
// 新到那份带着来源的更正指向时，先认更正关系再判矛盾，判法在 CONTEXT「来源明确的层内更正不是
// 同层冲突」那一条：
//
//   - 所指那份必须是同一提交版本、同一层的在册事实，否则交出 ErrCorrectionOutsideSubmission
//     或 ErrCorrectionAcrossLayers，那是矛盾输入，不是冲突。
//   - 只许更正链尾。所指那份已被另一份生效的更正取代时，两份更正都声称取代它，按到达先后或
//     业务时间择一都是替来源判断谁更正了谁，所以新到那份记冲突。
//   - 更正关系只解释新到那份与所指那份之间的差别，不进它俩之间的矛盾判断；与其余在册事实照常
//     比——没有更正语义的矛盾结果保持冲突（AT-CC-156）。
//   - 已被生效更正取代的在册事实只对业务发生时间不早于那份更正的新到结果退出比对：自那一刻起
//     来源收回了它的说法。在它仍算数的那段时间里（它自己到更正之间），与它矛盾、又不带更正指向
//     的新到结果照判冲突——否则这样一份结果不记冲突就进了那段时间的当前判断，正是 ADR-0226
//     决定二要挡的。
//
// 第一个返回值说更正关系是否在同一范围上生效：范围今天只到申报单元一级（ADR-0226），新旧范围
// 的交集与差集退化为相等与不等——相等时所指那份自更正的业务发生时间起整份不再算数（当前判断按时点取，见 ReleaseReading）；不等时交集为空，所指那份在
// 它的范围上保持原判断，新到那份在自己的范围上照同层判法独立判断（AT-CC-156）。记了冲突的更正
// 不生效：冲突的一方不得改变当前判断（CONTEXT「不得……按最后到达直接改变当前判断」）。
func CheckLayerConsistency(existing []LayerFact, incoming LayerFact) (bool, error) {
	if err := incoming.wellFormed(); err != nil {
		return false, err
	}
	if incoming.CorrectionInForce {
		// 生效与否是本判断的输出，新到那份自带一个，等于让来源自己宣布取代了谁。
		return false, ErrInvalidExternalResult
	}
	if incoming.Result.layer == ReleaseResultLayer && incoming.Release == nil {
		return false, ErrInvalidExternalResult
	}
	// supersededAt 记被生效更正取代的事实与取代时点（那份更正的业务发生时间）。只许更正链尾只看
	// 有没有后继；比对时的排除按时点。
	supersededAt := make(map[string]time.Time, len(existing))
	for _, fact := range existing {
		if err := fact.wellFormed(); err != nil {
			return false, err
		}
		if corrected, has := fact.Result.Corrects(); has && fact.CorrectionInForce {
			supersededAt[corrected] = fact.Result.occurredAt
		}
	}

	corrected, correcting := incoming.Result.Corrects()
	var target *LayerFact
	if correcting {
		for index := range existing {
			fact := &existing[index]
			if fact.Result.sourceID == corrected && fact.Result.version == incoming.Result.version {
				target = fact
				break
			}
		}
		if target == nil {
			return false, ErrCorrectionOutsideSubmission
		}
		if target.Result.layer != incoming.Result.layer {
			return false, ErrCorrectionAcrossLayers
		}
		if _, already := supersededAt[corrected]; already {
			return false, ErrLayerConflict
		}
	}

	for _, fact := range existing {
		if fact.Result.layer != incoming.Result.layer ||
			fact.Result.version != incoming.Result.version ||
			fact.Result.scope != incoming.Result.scope ||
			(correcting && fact.Result.sourceID == corrected) {
			continue
		}
		if at, superseded := supersededAt[fact.Result.sourceID]; superseded && !incoming.Result.occurredAt.Before(at) {
			continue
		}
		if contradicts(fact, incoming) {
			return false, ErrLayerConflict
		}
	}
	return correcting && target.Result.scope == incoming.Result.scope, nil
}

// wellFormed 守住放行事实与层次的配对：只有放行层带放行事实，且放行范围就是结果范围
// （接收编排两头都这么落）。放行层缺放行事实只放过在册那一侧——放行事实有自己的落点
// 之前，放行层只以分层事实落库，那批存量说不出种类；新到的放行层在核对入口另行要求带着它。
// 更正生效必有所更正的那一份。
func (fact LayerFact) wellFormed() error {
	if !fact.Result.layer.valid() {
		return ErrInvalidExternalResult
	}
	if _, has := fact.Result.Corrects(); fact.CorrectionInForce && !has {
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
		// 要么是矛盾、要么是更正，而更正得由来源指明，不能从业务时间猜——来源指明了的更正走不到
		// 这里，见 CheckLayerConsistency。
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
