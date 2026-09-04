package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// DisclosureDecisions 实现 ports.DisclosureDecisionStore：披露决定登记册（0023）。
//
// 三态结论全部入册——暂不披露与待授权是已作出的决定，不记它们同一发作期就会被反复
// 重判。行身份取决定三维（客户、发作期、决定时刻），与客户通知定位披露决定的三维同一
// 口径；FindCurrent 按（发作期+客户）取决定时刻最晚的那一份。读回经 DecideDisclosure
// 重过一遍构造门：结论与内容成对那条不变量在库上有 CHECK，Go 侧再验一次，一次坏写入
// 不得变成一份看起来合法的决定。
type DisclosureDecisions struct {
	db *bentopg.DB
}

func NewDisclosureDecisions(db *bentopg.DB) (*DisclosureDecisions, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &DisclosureDecisions{db: db}, nil
}

var _ ports.DisclosureDecisionStore = (*DisclosureDecisions)(nil)

func (repository *DisclosureDecisions) FindCurrent(
	ctx context.Context,
	tenant domain.TenantID,
	episode domain.EpisodeID,
	customer domain.CustomerAccountReference,
) (domain.DisclosureDecision, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.DisclosureDecision{}, false, fmt.Errorf("find disclosure decision: %w", err)
	}

	var (
		policyRaw, conclusionRaw string
		contentRaw               *string
		decidedAt                time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT disclosure_policy_ref, conclusion, content_ref, decided_at
		   FROM visibility_exception.disclosure_decision
		  WHERE tenant_id = $1 AND episode_id = $2 AND customer_ref = $3
		  ORDER BY decided_at DESC
		  LIMIT 1`,
		tenant.String(), episode.String(), customer.String(),
	).Scan(&policyRaw, &conclusionRaw, &contentRaw, &decidedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DisclosureDecision{}, false, nil
	}
	if err != nil {
		return domain.DisclosureDecision{}, false, fmt.Errorf("find disclosure decision: %w", err)
	}

	decision, err := rebuildDisclosureDecision(episode, customer, policyRaw, conclusionRaw, contentRaw, decidedAt)
	if err != nil {
		return domain.DisclosureDecision{}, false, fmt.Errorf("rebuild disclosure decision: %w", err)
	}
	return decision, true, nil
}

// Save 落一份决定。版本只增不改写：撞三维主键交回 AlreadyRecorded（ON CONFLICT DO
// NOTHING，事务保持可用），编排读回赢家；没有 UPDATE 路径——待授权转披露是另一行新决定。
func (repository *DisclosureDecisions) Save(
	ctx context.Context,
	tenant domain.TenantID,
	decision domain.DisclosureDecision,
) (ports.DisclosureDecisionSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DisclosureDecisionSaveOutcomeInvalid, fmt.Errorf("save disclosure decision: %w", err)
	}
	if decision.Conclusion().String() == "" {
		return ports.DisclosureDecisionSaveOutcomeInvalid, fmt.Errorf("save disclosure decision: decision is zero")
	}
	// 内容用指针：0023 的成对约束靠 NULL 分辨「不带内容」与「空内容引用」。
	var content *string
	if reference, disclosed := decision.Content(); disclosed {
		content = nullIfBlank(reference.String())
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.disclosure_decision
			(tenant_id, episode_id, customer_ref, decided_at,
			 disclosure_policy_ref, conclusion, content_ref)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT DO NOTHING`,
		tenant.String(),
		decision.Episode().String(),
		decision.Customer().String(),
		decision.DecidedAt(),
		decision.Policy().String(),
		decision.Conclusion().String(),
		content,
	)
	if err != nil {
		return ports.DisclosureDecisionSaveOutcomeInvalid, fmt.Errorf("save disclosure decision: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.DisclosureDecisionAlreadyRecorded, nil
	}
	return ports.DisclosureDecisionSaved, nil
}

// rebuildDisclosureDecision 把一行译回决定。结论逐格译回封闭三值（库上有 CHECK，这里的
// default 兜的是 CHECK 被放宽而 Go 侧没跟上），内容与结论成对由 DecideDisclosure 重验。
func rebuildDisclosureDecision(
	episode domain.EpisodeID,
	customer domain.CustomerAccountReference,
	policyRaw, conclusionRaw string,
	contentRaw *string,
	decidedAt time.Time,
) (domain.DisclosureDecision, error) {
	policy, err := domain.NewDisclosurePolicyReference(policyRaw)
	if err != nil {
		return domain.DisclosureDecision{}, err
	}
	conclusion, err := disclosureConclusionFrom(conclusionRaw)
	if err != nil {
		return domain.DisclosureDecision{}, err
	}
	var content domain.DisclosureContentReference
	if contentRaw != nil {
		if content, err = domain.NewDisclosureContentReference(*contentRaw); err != nil {
			return domain.DisclosureDecision{}, err
		}
	}
	return domain.DecideDisclosure(episode, customer, policy, conclusion, content, decidedAt)
}

func disclosureConclusionFrom(raw string) (domain.DisclosureConclusion, error) {
	for _, candidate := range []domain.DisclosureConclusion{
		domain.DiscloseToCustomer,
		domain.NotYetDisclosable,
		domain.AwaitingAuthorization,
	} {
		if candidate.String() == raw {
			return candidate, nil
		}
	}
	return domain.DisclosureConclusionInvalid, fmt.Errorf("未知披露结论 %q", raw)
}
