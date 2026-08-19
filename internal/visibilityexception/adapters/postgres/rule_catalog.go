package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// ErrAmbiguousCatalog 说明目录在同一时点有两个适用版本。
//
// 它是错误而不是一个业务答案：CONTEXT 不许「采用全局来源排名或最后消息覆盖」，
// 两个并存版本下按任一条挑一个都是替商业责任方作了它没作的决定。交回错误，编排
// 形成未决等目录修好——这与「目录未配置」是两条续办路，不能压成同一格。
var ErrAmbiguousCatalog = errors.New("visibility exception postgres: 目录在同一时点有多个适用版本")

// MilestoneMappings 实现 ports.MilestoneMappingView：按版本化映射目录归类已接受事实。
//
// 目录内容属实例半边（`PAR-VIS-01` 待提供），实现不属：空目录时如实交回「未配置」，
// 由编排按 CONTEXT 落未归类，不强行映射为「运输中」之类的宽泛结果。
//
// 租户在装配期固定，理由同 ExceptionCases——ClassifyFact 的签名里没有租户。
type MilestoneMappings struct {
	db     *bentopg.DB
	tenant domain.TenantID
}

func NewMilestoneMappings(db *bentopg.DB, tenant domain.TenantID) (*MilestoneMappings, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &MilestoneMappings{db: db, tenant: tenant}, nil
}

var _ ports.MilestoneMappingView = (*MilestoneMappings)(nil)

// ClassifyFact 归类一份已接受事实。
//
// 两级查找对应消费方要分开的两件事：目录整个没配（found=false，等租户登记）与目录
// 配了但这个事实类型没有可靠映射（found=true 且 Classified=false，带所依据的版本号）。
// 后者是一次已经作出的判断，必须带着版本进投影——投影要能追溯「按哪版判的未归类」。
// 条目按源上下文与事实类型建键，一行覆盖此后同类型事实，不按单条事实引用查目录。
//
// 适用版本按事实的**业务发生时间**选，不按当前时间：一条迟到三天才到达的事实属于
// 它发生那天的映射版本，「新版本默认只作用于生效后的事件」说的是事件不是消息。
func (view *MilestoneMappings) ClassifyFact(
	ctx context.Context,
	fact domain.AcceptedSourceFact,
) (ports.MilestoneAnswer, bool, error) {
	if view.tenant.String() == "" {
		return ports.MilestoneAnswer{}, false, nil
	}

	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return ports.MilestoneAnswer{}, false, fmt.Errorf("classify fact: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT mapping_version
		   FROM visibility_exception.milestone_mapping_version
		  WHERE tenant_id = $1
		    AND effective_from <= $2
		    AND (effective_to IS NULL OR effective_to > $2)
		  LIMIT 2`,
		view.tenant.String(), fact.OccurredAt(),
	)
	if err != nil {
		return ports.MilestoneAnswer{}, false, fmt.Errorf("classify fact: %w", err)
	}
	versions, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return ports.MilestoneAnswer{}, false, fmt.Errorf("classify fact: %w", err)
	}
	switch len(versions) {
	case 0:
		return ports.MilestoneAnswer{}, false, nil
	case 1:
	default:
		return ports.MilestoneAnswer{}, false, fmt.Errorf(
			"%w: 里程碑映射 tenant=%s at=%s", ErrAmbiguousCatalog, view.tenant, fact.OccurredAt())
	}

	mapping, err := domain.NewMappingVersionReference(versions[0])
	if err != nil {
		return ports.MilestoneAnswer{}, false, fmt.Errorf("classify fact: %w", err)
	}

	var milestoneRef string
	err = querier.QueryRow(ctx,
		`SELECT milestone_ref
		   FROM visibility_exception.milestone_mapping_entry
		  WHERE tenant_id = $1
		    AND mapping_version = $2
		    AND source_context = $3
		    AND source_fact_kind = $4`,
		view.tenant.String(), versions[0], fact.Source().String(), fact.Kind().String(),
	).Scan(&milestoneRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.MilestoneAnswer{Classified: false, Mapping: mapping}, true, nil
	}
	if err != nil {
		return ports.MilestoneAnswer{}, false, fmt.Errorf("classify fact: %w", err)
	}

	milestone, err := domain.NewMilestoneReference(milestoneRef)
	if err != nil {
		return ports.MilestoneAnswer{}, false, fmt.Errorf("classify fact: %w", err)
	}
	return ports.MilestoneAnswer{
		Milestone:  milestone,
		Classified: true,
		Mapping:    mapping,
	}, true, nil
}

// TriageRules 实现 ports.TriageRuleView：按版本化分诊规则判信号该走哪一格。
//
// 目录内容属实例半边（`PAR-VIS-05` 待提供），实现不属：空目录时如实交回「未配置」，
// 由编排把信号送进人工复核——不自动建案，也不装作没有信号。
type TriageRules struct {
	db     *bentopg.DB
	tenant domain.TenantID
}

func NewTriageRules(db *bentopg.DB, tenant domain.TenantID) (*TriageRules, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &TriageRules{db: db, tenant: tenant}, nil
}

var _ ports.TriageRuleView = (*TriageRules)(nil)

// TriageSignal 判一个信号的走向。
//
// 适用版本取当前仍然有效的那一份。分诊判的是手上这个活信号，而 TriageQuery 不带
// 判断时点——没有时点就不去猜一个历史版本；库上的部分唯一索引担保未闭区间至多
// 一份，因此「当前」是唯一的。
//
// 目录已配而这一条没有条目时交回`人工复核`，不是 found=false：目录已经回答过了——
// 「高可信、高影响且**命中**版本化分诊规则的信号可以自动建立或关联案件」，没命中
// 就不具备自动建案的条件，而「低可信、资料不足、可能重复或关联不明确的信号必须先
// 进入分诊」正是人工复核那一格。答复带上所依据的规则版本，让分诊结论可追溯到
// 「按哪版判的无命中」。
func (view *TriageRules) TriageSignal(
	ctx context.Context,
	query ports.TriageQuery,
) (ports.TriageAnswer, bool, error) {
	if view.tenant.String() == "" || !triageQueryComplete(query) {
		return ports.TriageAnswer{}, false, nil
	}

	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return ports.TriageAnswer{}, false, fmt.Errorf("triage signal: %w", err)
	}

	var ruleVersion string
	err = querier.QueryRow(ctx,
		`SELECT rule_version
		   FROM visibility_exception.triage_rule_version
		  WHERE tenant_id = $1
		    AND effective_to IS NULL
		    AND effective_from <= now()`,
		view.tenant.String(),
	).Scan(&ruleVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.TriageAnswer{}, false, nil
	}
	if err != nil {
		return ports.TriageAnswer{}, false, fmt.Errorf("triage signal: %w", err)
	}

	rule, err := domain.NewSignalRuleVersionReference(ruleVersion)
	if err != nil {
		return ports.TriageAnswer{}, false, fmt.Errorf("triage signal: %w", err)
	}

	var outcomeRaw string
	err = querier.QueryRow(ctx,
		`SELECT outcome
		   FROM visibility_exception.triage_rule_entry
		  WHERE tenant_id = $1
		    AND rule_version = $2
		    AND signal_kind = $3
		    AND confidence_ref = $4`,
		view.tenant.String(), ruleVersion, query.Kind.String(), query.Confidence.String(),
	).Scan(&outcomeRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.TriageAnswer{Outcome: domain.ManualReviewRequired, Rule: rule}, true, nil
	}
	if err != nil {
		return ports.TriageAnswer{}, false, fmt.Errorf("triage signal: %w", err)
	}

	outcome, err := triageOutcomeFrom(outcomeRaw)
	if err != nil {
		return ports.TriageAnswer{}, false, fmt.Errorf("triage signal: %w", err)
	}
	return ports.TriageAnswer{Outcome: outcome, Rule: rule}, true, nil
}

// triageQueryComplete 挡下缺维的查询。信号必须保存对象、类型、规则版本与可信度
// （CONTEXT 硬句），缺哪一维都不该由本适配器补一个默认值去查目录。
func triageQueryComplete(query ports.TriageQuery) bool {
	return query.Kind.String() != "" &&
		query.Parcel.String() != "" &&
		query.Rule.String() != "" &&
		query.Confidence.String() != ""
}

// triageOutcomeFrom 逐格译回封闭四走向。库上已有 CHECK 守着取值集合，这里的 default
// 兜的是「CHECK 被后续迁移放宽而 Go 侧没跟上」——那时报错，不吸收成某一格。
func triageOutcomeFrom(raw string) (domain.TriageOutcome, error) {
	switch raw {
	case "ATTACH_TO_EXISTING":
		return domain.AttachToExistingCase, nil
	case "AUTO_ESTABLISH":
		return domain.AutoEstablishCase, nil
	case "MANUAL_REVIEW":
		return domain.ManualReviewRequired, nil
	case "NO_CASE":
		return domain.NoCaseNeeded, nil
	default:
		return domain.TriageOutcomeInvalid, fmt.Errorf("未知分诊走向 %q", raw)
	}
}
