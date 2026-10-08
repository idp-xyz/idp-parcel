// Package customscompliance 把 network-routing 的候选投影接到 customs-compliance 的
// 关务适用性判断口（票 routing-first-cut/12，ADR-0148 决定三）。位置由 ADR-0025 定死：
// 只有消费侧的 adapters/<provider>/ 可以导入提供方上下文——口岸与申报路径过不过关务边
// 界是提供方说的，这里只把投影翻译过去、把答案翻译回来，不自行判一格。
package customscompliance

import (
	"context"
	"errors"
	"fmt"

	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// errUntranslatable 说提供方交回了本桥认不出的形状（答案缺格、三格与负载对不上、候选
// 对不齐）：逐格分派不留兜底，认不出就上抛，不折成一个像样的硬约束（ADR-0031）。
var errUntranslatable = errors.New("network routing: untranslatable customs applicability answer")

// Judge 是 customs-compliance 判断服务的窄面：按（租户，时点，候选投影）逐条作答，答案
// 词形与出处都在提供方 domain 上。装配时把 CC 的应用处理器直接接进来。
type Judge interface {
	Handle(
		ctx context.Context,
		query ccports.CustomsApplicabilityQuery,
	) ([]ccdomain.CustomsApplicabilityJudgment, error)
}

// CustomsApplicabilityAssessor 实现 nrports.CustomsApplicabilitySource：翻译投影、逐候
// 选译回硬约束事实与出处。
type CustomsApplicabilityAssessor struct {
	judge Judge
}

func NewCustomsApplicabilityAssessor(judge Judge) (*CustomsApplicabilityAssessor, error) {
	if judge == nil {
		return nil, fmt.Errorf("network routing: customs applicability: judge is nil")
	}
	return &CustomsApplicabilityAssessor{judge: judge}, nil
}

var _ nrports.CustomsApplicabilitySource = (*CustomsApplicabilityAssessor)(nil)

// AssessCustomsApplicability 的映射口径（routing-first-cut/12 分诊裁定三轮次四落定）：
// 可用 → 满足；不可用 → 适用限制，限制引用指回判断标识与理由（原因链上指得回来源，
// 「已解除」才核对得了）；状态未知 → 逐类缺口与再次判断条件。出处逐候选带回——判断
// 标识加目录版本引用，随判断记录留痕。
func (assessor *CustomsApplicabilityAssessor) AssessCustomsApplicability(
	ctx context.Context,
	query nrports.CustomsApplicabilityQuery,
) (nrports.CustomsApplicabilityAssessment, error) {
	none := nrports.CustomsApplicabilityAssessment{}
	if len(query.Candidates) == 0 {
		return none, nil
	}

	tenant, err := ccdomain.NewTenantID(query.Tenant.String())
	if err != nil {
		return none, fmt.Errorf("translate customs query: %w", err)
	}
	ccQuery := ccports.CustomsApplicabilityQuery{
		Tenant:     tenant,
		AsOf:       query.AsOf,
		Candidates: make([]ccports.CustomsApplicabilityCandidate, 0, len(query.Candidates)),
	}
	for _, candidate := range query.Candidates {
		reference, err := ccdomain.NewRouteCandidateReference(candidate.Candidate.String())
		if err != nil {
			return none, fmt.Errorf("translate customs query: %w", err)
		}
		ccQuery.Candidates = append(ccQuery.Candidates, ccports.CustomsApplicabilityCandidate{
			Candidate:      reference,
			Origin:         candidate.Origin,
			HasOrigin:      candidate.HasOrigin,
			Destination:    candidate.Destination,
			HasDestination: candidate.HasDestination,
		})
	}

	judgments, err := assessor.judge.Handle(ctx, ccQuery)
	if err != nil {
		return none, fmt.Errorf("assess customs applicability: %w", err)
	}
	if len(judgments) != len(query.Candidates) {
		return none, fmt.Errorf("%w: %d candidates asked, %d judgments returned",
			errUntranslatable, len(query.Candidates), len(judgments))
	}
	byCandidate := make(map[string]ccdomain.CustomsApplicabilityJudgment, len(judgments))
	for _, judgment := range judgments {
		byCandidate[judgment.Candidate().String()] = judgment
	}

	findings := make([]nrdomain.HardConstraintFinding, 0, len(query.Candidates))
	citations := make([]nrdomain.CustomsApplicabilityCitation, 0, len(query.Candidates))
	for _, candidate := range query.Candidates {
		judgment, ok := byCandidate[candidate.Candidate.String()]
		if !ok {
			return none, fmt.Errorf("%w: no answer for candidate %s", errUntranslatable, candidate.Candidate)
		}
		finding, citation, err := translate(candidate.Candidate, judgment)
		if err != nil {
			return none, err
		}
		findings = append(findings, finding)
		citations = append(citations, citation)
	}
	return nrports.CustomsApplicabilityAssessment{Findings: findings, Citations: citations}, nil
}

// translate 把一份 CC 作答译成一条硬约束事实加一条出处。
func translate(
	candidate nrdomain.CandidateID,
	judgment ccdomain.CustomsApplicabilityJudgment,
) (nrdomain.HardConstraintFinding, nrdomain.CustomsApplicabilityCitation, error) {
	cite := func() (nrdomain.CustomsApplicabilityCitation, error) {
		return nrdomain.NewCustomsApplicabilityCitation(nrdomain.CustomsApplicabilityCitationSpec{
			Candidate: candidate,
			Judgment:  judgment.JudgmentID(),
			Versions:  judgment.Versions(),
		})
	}
	var finding nrdomain.HardConstraintFinding
	switch judgment.Outcome() {
	case ccdomain.CustomsAvailable:
		var err error
		finding, err = nrdomain.NewHardConstraintFinding(nrdomain.HardConstraintFindingSpec{
			Candidate: candidate,
			Outcome:   nrdomain.ConstraintSatisfied,
		})
		if err != nil {
			return finding, nrdomain.CustomsApplicabilityCitation{}, err
		}
	case ccdomain.CustomsUnavailable:
		restriction, err := nrdomain.NewRestrictionReference("CUSTOMS_APPLICABILITY/" +
			judgment.JudgmentID() + "/" + judgment.UnavailableReason().String())
		if err != nil {
			return finding, nrdomain.CustomsApplicabilityCitation{}, err
		}
		finding, err = nrdomain.NewHardConstraintFinding(nrdomain.HardConstraintFindingSpec{
			Candidate:   candidate,
			Outcome:     nrdomain.RestrictionApplies,
			Restriction: restriction,
		})
		if err != nil {
			return finding, nrdomain.CustomsApplicabilityCitation{}, err
		}
	case ccdomain.CustomsStatusUnknown:
		missing, reassess, err := unknownGap(judgment)
		if err != nil {
			return finding, nrdomain.CustomsApplicabilityCitation{}, err
		}
		finding, err = nrdomain.NewHardConstraintFinding(nrdomain.HardConstraintFindingSpec{
			Candidate: candidate,
			Outcome:   nrdomain.ConstraintStatusUnknown,
			Missing:   missing,
			Reassess:  reassess,
		})
		if err != nil {
			return finding, nrdomain.CustomsApplicabilityCitation{}, err
		}
	default:
		return finding, nrdomain.CustomsApplicabilityCitation{},
			fmt.Errorf("%w: outcome %d", errUntranslatable, judgment.Outcome())
	}
	citation, err := cite()
	if err != nil {
		return finding, nrdomain.CustomsApplicabilityCitation{}, err
	}
	return finding, citation, nil
}

// unknownGap 把状态未知译成缺口与再次判断条件：同类缺口同一对词，缺国家码再指名哪一
// 端——缺口点名要补的那一格，重判条件点名换了什么再判。
func unknownGap(
	judgment ccdomain.CustomsApplicabilityJudgment,
) (nrdomain.EvidenceGapReference, nrdomain.ReassessmentCondition, error) {
	switch judgment.UnknownReason() {
	case ccdomain.EndpointCountryMissing:
		side, ok := judgment.EndpointSide()
		if !ok {
			return nrdomain.EvidenceGapReference{}, nrdomain.ReassessmentCondition{}, errUntranslatable
		}
		return gapPair(
			"CUSTOMS_ENDPOINT_COUNTRY_MISSING/"+side.String(),
			"CUSTOMS_ENDPOINT_COUNTRY_SUPPLEMENTED",
		)
	case ccdomain.CatalogEmpty:
		return gapPair("CUSTOMS_PORT_PATH_CATALOG_EMPTY", "CUSTOMS_PORT_PATH_CATALOG_REGISTERED")
	case ccdomain.CatalogUnreadable:
		return gapPair("CUSTOMS_PORT_PATH_CATALOG_UNREADABLE", "CUSTOMS_PORT_PATH_CATALOG_RESTORED")
	default:
		return nrdomain.EvidenceGapReference{}, nrdomain.ReassessmentCondition{},
			fmt.Errorf("%w: unknown reason %d", errUntranslatable, judgment.UnknownReason())
	}
}

func gapPair(
	missing, reassess string,
) (nrdomain.EvidenceGapReference, nrdomain.ReassessmentCondition, error) {
	gap, err := nrdomain.NewEvidenceGapReference(missing)
	if err != nil {
		return nrdomain.EvidenceGapReference{}, nrdomain.ReassessmentCondition{}, err
	}
	condition, err := nrdomain.NewReassessmentCondition(reassess)
	if err != nil {
		return nrdomain.EvidenceGapReference{}, nrdomain.ReassessmentCondition{}, err
	}
	return gap, condition, nil
}
