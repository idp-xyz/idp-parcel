package domain

import (
	"errors"
	"fmt"
	"time"
)

// 本文件是供应商账单的审核决定（ADR-0357 决定二至四）：授权审核人对一行主张金额上的一段范围给出通过、争议或拒绝。
// 范围守恒、通过不超已匹配金额、再审只落在原争议之内、通过与拒绝终局，都在 DecideBillLineAudit 这一道门上判；
// 编排只负责把同一行已有的决定按序排进来、按错误名作答。

var (
	ErrInvalidAuditDecision = errors.New("settlement accounting: invalid bill line audit decision")
	// ErrAuditRangeOutsideLine：范围出了这一行的主张金额，或再审出了它所解的那段争议。是提交矛盾，改请求。
	ErrAuditRangeOutsideLine = errors.New("settlement accounting: audit range lies outside the line or the disputed range")
	// ErrAuditRangeTaken：范围与同一行上已有的决定重叠（再审则是与同一争议的另一份再审重叠）。通过与拒绝终局，
	// 已有结果的那一段不再审；并发审同一行时后到者也落这一格。
	ErrAuditRangeTaken = errors.New("settlement accounting: audit range overlaps a decision already given")
	// ErrDisputeNotFound：再审点名的不是这一行上的一份争议。
	ErrDisputeNotFound = errors.New("settlement accounting: the resolved decision is not a dispute on this line")
	// ErrUnknownAuditVerdict 说明载荷里的审核结果不是产品认得的那三种。
	ErrUnknownAuditVerdict = errors.New("settlement accounting: unknown audit verdict")
)

// AuditVerdict 是一段范围的审核结果，封闭三值。通过形成审核应付；争议等再审；拒绝在这份主张上不再审。
type AuditVerdict uint8

const (
	AuditVerdictInvalid AuditVerdict = iota
	AuditPassed
	AuditDisputed
	AuditRejected
)

func (verdict AuditVerdict) String() string {
	switch verdict {
	case AuditPassed:
		return "PASSED"
	case AuditDisputed:
		return "DISPUTED"
	case AuditRejected:
		return "REJECTED"
	default:
		return ""
	}
}

func (verdict AuditVerdict) valid() bool {
	return verdict >= AuditPassed && verdict <= AuditRejected
}

// AuditVerdictFromName 按字面取值认审核结果，给审核口的载荷与读回用。
func AuditVerdictFromName(name string) (AuditVerdict, error) {
	for verdict := AuditPassed; verdict <= AuditRejected; verdict++ {
		if verdict.String() == name {
			return verdict, nil
		}
	}
	return AuditVerdictInvalid, fmt.Errorf("%w: %q", ErrUnknownAuditVerdict, name)
}

// AmountRange 是一行主张金额上的一段 [From, To)，最小币单位，左闭右开。记位置而不只记长度：「不重叠」与「再审只落在原
// 争议之内」由此在结构上成立，库上的排他约束也才兜得住。
type AmountRange struct {
	from, to int64
}

// NewAmountRange 要 0 ≤ from < to。
func NewAmountRange(from, to int64) (AmountRange, error) {
	if from < 0 || to <= from {
		return AmountRange{}, fmt.Errorf("%w: amount range [%d, %d)", ErrInvalidAuditDecision, from, to)
	}
	return AmountRange{from: from, to: to}, nil
}

func (span AmountRange) From() int64   { return span.from }
func (span AmountRange) To() int64     { return span.to }
func (span AmountRange) Length() int64 { return span.to - span.from }

func (span AmountRange) valid() bool {
	return span.from >= 0 && span.to > span.from
}

// Overlaps 说两段是否共有至少一个最小币单位。首尾相接不算重叠。
func (span AmountRange) Overlaps(other AmountRange) bool {
	return span.from < other.to && other.from < span.to
}

// Within 说本段是否整段落在 outer 之内。
func (span AmountRange) Within(outer AmountRange) bool {
	return span.from >= outer.from && span.to <= outer.to
}

// AuditDecisionID 是一份审核决定的稳定身份，由调用方生成做幂等键；通过时它就是所形成审核应付的身份（ADR-0357 决定三、
// ADR-0355 决定十）。
type AuditDecisionID struct{ requiredValue }

func NewAuditDecisionID(value string) (AuditDecisionID, error) {
	required, err := newRequiredValue("audit decision ID", value)
	return AuditDecisionID{required}, err
}

// AuditBasisReference 指名争议、拒绝或再审的依据：理由与证据的引用。
type AuditBasisReference struct{ requiredValue }

func NewAuditBasisReference(value string) (AuditBasisReference, error) {
	required, err := newRequiredValue("audit basis reference", value)
	return AuditBasisReference{required}, err
}

// BillLineAuditDecisionSpec 是给出一份审核决定的输入。Basis 在争议、拒绝与再审时必备，直接通过可缺；Resolves 只在再审时
// 给出，点名所解的那份争议。
type BillLineAuditDecisionSpec struct {
	Decision  AuditDecisionID
	Version   BillClaimVersion
	Verdict   AuditVerdict
	Span      AmountRange
	Basis     AuditBasisReference
	Resolves  AuditDecisionID
	Auditor   AuditorReference
	AuditedAt time.Time
}

// BillLineAuditDecision 是授权审核人对一行主张金额上一段范围的裁定。决定按主张身份与行归属、不分版本——审核应付本就按
// 主张与行回指；version 记的是审的时候点名的那一版，迁移回填的旧应付没有它。
type BillLineAuditDecision struct {
	decision  AuditDecisionID
	claim     BillClaimID
	version   BillClaimVersion
	line      BillLineReference
	verdict   AuditVerdict
	span      AmountRange
	basis     AuditBasisReference
	resolves  AuditDecisionID
	auditor   AuditorReference
	auditedAt time.Time
}

func (decision BillLineAuditDecision) Decision() AuditDecisionID { return decision.decision }
func (decision BillLineAuditDecision) Claim() BillClaimID        { return decision.claim }
func (decision BillLineAuditDecision) Line() BillLineReference   { return decision.line }
func (decision BillLineAuditDecision) Verdict() AuditVerdict     { return decision.verdict }
func (decision BillLineAuditDecision) Span() AmountRange         { return decision.span }
func (decision BillLineAuditDecision) Auditor() AuditorReference { return decision.auditor }
func (decision BillLineAuditDecision) AuditedAt() time.Time      { return decision.auditedAt }

// Version 交回审的时候点名的那一版；迁移回填的旧应付不知道是按哪一版审的，第二个值为 false。
func (decision BillLineAuditDecision) Version() (BillClaimVersion, bool) {
	return decision.version, decision.version.valid()
}

// Basis 交回依据；直接通过可以没有。
func (decision BillLineAuditDecision) Basis() (AuditBasisReference, bool) {
	return decision.basis, decision.basis.valid()
}

// Resolves 交回这份再审所解的争议；直接给的决定没有。
func (decision BillLineAuditDecision) Resolves() (AuditDecisionID, bool) {
	return decision.resolves, decision.resolves.valid()
}

// Repeats 说这份决定是不是正是「这一段、这种结果、这条依据、解这份争议」：同一行上换个身份再提交同一个业务变化，编排读回
// 先到的那份，不形成第二份。审核方与点审核的人不比——它们不是请求说的事。
func (decision BillLineAuditDecision) Repeats(
	verdict AuditVerdict,
	span AmountRange,
	basis AuditBasisReference,
	resolves AuditDecisionID,
) bool {
	return decision.verdict == verdict && decision.span == span && decision.basis == basis && decision.resolves == resolves
}

// shapeHolds 守每份决定都要自证的那几件：身份、结果、范围、审核方与时点齐备；争议与拒绝带依据；再审只能通过或拒绝、
// 必须带依据、不能解自己。
func (decision BillLineAuditDecision) shapeHolds() bool {
	if !decision.decision.valid() || !decision.claim.valid() || !decision.line.valid() || !decision.verdict.valid() ||
		!decision.span.valid() || !decision.auditor.valid() || decision.auditedAt.IsZero() {
		return false
	}
	if decision.verdict != AuditPassed && !decision.basis.valid() {
		return false
	}
	if decision.resolves.valid() {
		if decision.verdict == AuditDisputed || !decision.basis.valid() || decision.resolves == decision.decision {
			return false
		}
	}
	return true
}

// DecideBillLineAudit 对一行给出一份审核决定（ADR-0357 决定二至四）。prior 是同一主张同一行上已有的全部决定（任一版本、
// 含回填的旧应付），由编排在排过队之后读来——这道门只在这一行的完整历史上判：
//   - 范围在这一行的主张金额之内（取 match 那一版），否则 ErrAuditRangeOutsideLine；
//   - 直接给的决定与同一行上直接给的各段不重叠；再审落在所解争议之内、与同一争议的其余再审不重叠；否则 ErrAuditRangeTaken。
//     通过与拒绝因此终局：它们的那一段已经被占，再审又只能落在争议里；
//   - 再审点名的必须是这一行上的一份争议，否则 ErrDisputeNotFound；
//   - 通过的各段之和（含这一段）不超过 match.MatchedMinor()，否则 ErrNotAuditable。
func DecideBillLineAudit(
	match BillLineMatch,
	prior []BillLineAuditDecision,
	spec BillLineAuditDecisionSpec,
) (BillLineAuditDecision, error) {
	decision := BillLineAuditDecision{
		decision:  spec.Decision,
		claim:     match.claim,
		version:   spec.Version,
		line:      match.line,
		verdict:   spec.Verdict,
		span:      spec.Span,
		basis:     spec.Basis,
		resolves:  spec.Resolves,
		auditor:   spec.Auditor,
		auditedAt: spec.AuditedAt.UTC(),
	}
	if !match.claim.valid() || !spec.Version.valid() || !decision.shapeHolds() || decision.auditedAt.Before(match.matchedAt) {
		return BillLineAuditDecision{}, ErrInvalidAuditDecision
	}
	if decision.span.to > match.claimedMinor {
		return BillLineAuditDecision{}, ErrAuditRangeOutsideLine
	}
	passed := int64(0)
	var dispute BillLineAuditDecision
	found := false
	for _, earlier := range prior {
		if earlier.claim != match.claim || earlier.line != match.line {
			return BillLineAuditDecision{}, fmt.Errorf("%w: prior decision %s is on another line", ErrInvalidAuditDecision, earlier.decision)
		}
		if earlier.verdict == AuditPassed {
			passed += earlier.span.Length()
		}
		if decision.resolves.valid() && earlier.decision == decision.resolves {
			dispute, found = earlier, true
		}
	}
	if decision.resolves.valid() {
		if !found || dispute.verdict != AuditDisputed || dispute.resolves.valid() {
			return BillLineAuditDecision{}, ErrDisputeNotFound
		}
		if !decision.span.Within(dispute.span) {
			return BillLineAuditDecision{}, ErrAuditRangeOutsideLine
		}
	}
	for _, earlier := range prior {
		if earlier.resolves != decision.resolves {
			continue
		}
		if earlier.span.Overlaps(decision.span) {
			return BillLineAuditDecision{}, ErrAuditRangeTaken
		}
	}
	if decision.verdict == AuditPassed && passed+decision.span.Length() > match.MatchedMinor() {
		return BillLineAuditDecision{}, ErrNotAuditable
	}
	return decision, nil
}

// RehydrateBillLineAuditDecisionSpec 是审核决定在库里的样子。Version 零值表示迁移回填的旧应付（不知道按哪一版审）。
type RehydrateBillLineAuditDecisionSpec struct {
	Decision  AuditDecisionID
	Claim     BillClaimID
	Version   BillClaimVersion
	Line      BillLineReference
	Verdict   AuditVerdict
	Span      AmountRange
	Basis     AuditBasisReference
	Resolves  AuditDecisionID
	Auditor   AuditorReference
	AuditedAt time.Time
}

// RehydrateBillLineAuditDecision 是审核决定的重建门（ADR-0030 的纪律）：复验每份决定自证的那几件；与同一行其余决定的
// 守恒是写入时在 DecideBillLineAudit 与库上的排他约束判过的，单行重建不重判。
func RehydrateBillLineAuditDecision(spec RehydrateBillLineAuditDecisionSpec) (BillLineAuditDecision, error) {
	decision := BillLineAuditDecision{
		decision:  spec.Decision,
		claim:     spec.Claim,
		version:   spec.Version,
		line:      spec.Line,
		verdict:   spec.Verdict,
		span:      spec.Span,
		basis:     spec.Basis,
		resolves:  spec.Resolves,
		auditor:   spec.Auditor,
		auditedAt: spec.AuditedAt.UTC(),
	}
	if !decision.shapeHolds() {
		return BillLineAuditDecision{}, ErrInvalidAuditDecision
	}
	return decision, nil
}

// LineAuditTally 是一行主张金额按审核结果分开的四个和：已通过、已拒绝、仍在争议中、还没有任何结果。
type LineAuditTally struct {
	PassedMinor      int64
	RejectedMinor    int64
	OpenDisputeMinor int64
	UndecidedMinor   int64
}

// TallyLineDecisions 从一行的全部决定求四个和：审核答复带出已通过金额，账单各行的读面带出四个和。再审只落在争议之内、同一争议
// 的再审互不重叠，所以仍在争议中的就是直接给的争议之和减去全部再审之和；还没有结果的是主张金额减去直接给的各段之和（各版本
// 主张金额不同时可能为负，按零计）。
func TallyLineDecisions(claimedMinor int64, decisions []BillLineAuditDecision) LineAuditTally {
	tally := LineAuditTally{}
	decided, resolved := int64(0), int64(0)
	for _, decision := range decisions {
		length, resolving := decision.span.Length(), decision.resolves.valid()
		switch decision.verdict {
		case AuditPassed:
			tally.PassedMinor += length
		case AuditRejected:
			tally.RejectedMinor += length
		case AuditDisputed:
			if !resolving {
				tally.OpenDisputeMinor += length
			}
		}
		if resolving {
			resolved += length
		} else {
			decided += length
		}
	}
	tally.OpenDisputeMinor = max(tally.OpenDisputeMinor-resolved, 0)
	tally.UndecidedMinor = max(claimedMinor-decided, 0)
	return tally
}
