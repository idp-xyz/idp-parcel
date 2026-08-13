package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// Claims 实现 ports.ClaimStore。索赔是判断历史推进的聚合（受理→过审→结论→复核/
// 撤回），Save 走 UPSERT 按键整行更新——三判形状由迁移 CHECK 与领域重建口两头把门，
// 复核换版走前版列（原结论保留，CONTEXT 253），不翻旧插新。
type Claims struct {
	db *bentopg.DB
}

func NewClaims(db *bentopg.DB) (*Claims, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &Claims{db: db}, nil
}

// FindByBatchItem 按（租户+批次+项）取回索赔。读回经 RehydrateClaimItem 重验三判
// 形状——一次坏写入不得变成一个看起来合法的判断历史。
func (repository *Claims) FindByBatchItem(
	ctx context.Context,
	tenant domain.TenantID,
	batch domain.ClaimBatchReference,
	item domain.ClaimItemID,
) (*domain.ClaimItem, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("find claim item: %w", err)
	}

	var (
		customer, contract, target, kind                 string
		submittedAt                                      time.Time
		screen, screenBasis, conclusion, priorConclusion *string
		concludedAt, reviewBy, withdrawnAt               *time.Time
		withdrawn                                        bool
	)
	err = querier.QueryRow(ctx,
		`SELECT customer_ref, contract_ref, target_ref, kind_ref, submitted_at,
		        screen, screen_basis, conclusion, concluded_at, review_by,
		        prior_conclusion, withdrawn, withdrawn_at
		   FROM visibility_exception.claim_item
		  WHERE tenant_id = $1 AND batch_ref = $2 AND item_id = $3`,
		tenant.String(), batch.String(), item.String(),
	).Scan(&customer, &contract, &target, &kind, &submittedAt,
		&screen, &screenBasis, &conclusion, &concludedAt, &reviewBy,
		&priorConclusion, &withdrawn, &withdrawnAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("find claim item: %w", err)
	}

	snapshot := domain.ClaimItemSnapshot{
		ID:          item,
		Batch:       batch,
		SubmittedAt: submittedAt,
		Withdrawn:   withdrawn,
	}
	if snapshot.Customer, err = domain.NewCustomerAccountReference(customer); err != nil {
		return nil, false, fmt.Errorf("rebuild claim item: %w", err)
	}
	if snapshot.Contract, err = domain.NewContractScopeReference(contract); err != nil {
		return nil, false, fmt.Errorf("rebuild claim item: %w", err)
	}
	if snapshot.Target, err = domain.NewRequestScopeReference(target); err != nil {
		return nil, false, fmt.Errorf("rebuild claim item: %w", err)
	}
	if snapshot.Kind, err = domain.NewClaimKindReference(kind); err != nil {
		return nil, false, fmt.Errorf("rebuild claim item: %w", err)
	}
	if screen != nil {
		if snapshot.Screen, err = eligibilityScreenFrom(*screen); err != nil {
			return nil, false, err
		}
	}
	if screenBasis != nil {
		snapshot.ScreenBasis = *screenBasis
	}
	if conclusion != nil {
		if snapshot.Conclusion, err = liabilityConclusionFrom(*conclusion); err != nil {
			return nil, false, err
		}
	}
	if priorConclusion != nil {
		if snapshot.PriorConclusion, err = liabilityConclusionFrom(*priorConclusion); err != nil {
			return nil, false, err
		}
	}
	if concludedAt != nil {
		snapshot.ConcludedAt = *concludedAt
	}
	if reviewBy != nil {
		snapshot.ReviewBy = *reviewBy
	}
	if withdrawnAt != nil {
		snapshot.WithdrawnAt = *withdrawnAt
	}

	claim, err := domain.RehydrateClaimItem(snapshot)
	if err != nil {
		return nil, false, fmt.Errorf("rebuild claim item: %w", err)
	}
	return claim, true, nil
}

// Save 落索赔的当前判断历史。同键整行更新（受理后每一步判断都经同一入口落库）；
// 键列与提交事实列在更新时同样重写——它们不可变，重写等值是无害的，靠 CHECK 与
// 领域门拦形状而不是在这里分列。
func (repository *Claims) Save(
	ctx context.Context,
	tenant domain.TenantID,
	claim *domain.ClaimItem,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save claim item: %w", err)
	}
	if claim == nil {
		return fmt.Errorf("save claim item: claim is nil")
	}
	snapshot := claim.Snapshot()

	var screen, screenBasis, conclusion, prior *string
	if snapshot.Screen == domain.ClaimEligible {
		screen = stringPointer("ELIGIBLE")
	} else if snapshot.Screen == domain.ClaimIneligible {
		screen = stringPointer("INELIGIBLE")
	}
	if snapshot.ScreenBasis != "" {
		screenBasis = &snapshot.ScreenBasis
	}
	if value := snapshot.Conclusion.String(); value != "" {
		conclusion = &value
	}
	if value := snapshot.PriorConclusion.String(); value != "" {
		prior = &value
	}

	_, err = executor.Exec(ctx,
		`INSERT INTO visibility_exception.claim_item
			(tenant_id, batch_ref, item_id, customer_ref, contract_ref, target_ref, kind_ref,
			 submitted_at, screen, screen_basis, conclusion, concluded_at, review_by,
			 prior_conclusion, withdrawn, withdrawn_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		 ON CONFLICT (tenant_id, batch_ref, item_id) DO UPDATE SET
			screen = EXCLUDED.screen,
			screen_basis = EXCLUDED.screen_basis,
			conclusion = EXCLUDED.conclusion,
			concluded_at = EXCLUDED.concluded_at,
			review_by = EXCLUDED.review_by,
			prior_conclusion = EXCLUDED.prior_conclusion,
			withdrawn = EXCLUDED.withdrawn,
			withdrawn_at = EXCLUDED.withdrawn_at`,
		tenant.String(),
		snapshot.Batch.String(),
		snapshot.ID.String(),
		snapshot.Customer.String(),
		snapshot.Contract.String(),
		snapshot.Target.String(),
		snapshot.Kind.String(),
		snapshot.SubmittedAt,
		screen,
		screenBasis,
		conclusion,
		nullIfZeroTime(snapshot.ConcludedAt),
		nullIfZeroTime(snapshot.ReviewBy),
		prior,
		snapshot.Withdrawn,
		nullIfZeroTime(snapshot.WithdrawnAt),
	)
	if err != nil {
		return fmt.Errorf("save claim item: %w", err)
	}
	return nil
}

// Recoveries 实现 ports.RecoveryStore。事项要件成立即固定（硬句 184）——插入即
// 不可回写，适配器没有事项的 UPDATE 语句；动作只增（重试是新记录不是改写）。
type Recoveries struct {
	db *bentopg.DB
}

func NewRecoveries(db *bentopg.DB) (*Recoveries, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &Recoveries{db: db}, nil
}

func (repository *Recoveries) FindByID(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.RecoveryMatterID,
) (domain.RecoveryMatter, bool, error) {
	return repository.findWhere(ctx,
		`tenant_id = $1 AND matter_id = $2`,
		tenant.String(), id.String())
}

// FindCurrent 按（租户+案件+相对方+范围）取回当前事项——事项幂等的读半边，唯一
// 约束保证至多一行。
func (repository *Recoveries) FindCurrent(
	ctx context.Context,
	tenant domain.TenantID,
	caseID domain.CaseID,
	counterparty domain.CounterpartyReference,
	scope domain.RequestScopeReference,
) (domain.RecoveryMatter, bool, error) {
	return repository.findWhere(ctx,
		`tenant_id = $1 AND case_id = $2 AND counterparty_ref = $3 AND scope_ref = $4`,
		tenant.String(), caseID.String(), counterparty.String(), scope.String())
}

func (repository *Recoveries) findWhere(
	ctx context.Context,
	where string,
	args ...any,
) (domain.RecoveryMatter, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.RecoveryMatter{}, false, fmt.Errorf("find recovery matter: %w", err)
	}

	var (
		matterID, caseID, counterparty, scope, basis, legalEntity, evidence string
		deadline, openedAt                                                  time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT matter_id, case_id, counterparty_ref, scope_ref, basis_ref,
		        legal_entity_ref, evidence_ref, deadline, opened_at
		   FROM visibility_exception.recovery_matter
		  WHERE `+where,
		args...,
	).Scan(&matterID, &caseID, &counterparty, &scope, &basis, &legalEntity, &evidence, &deadline, &openedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RecoveryMatter{}, false, nil
	}
	if err != nil {
		return domain.RecoveryMatter{}, false, fmt.Errorf("find recovery matter: %w", err)
	}

	matter, err := rebuildRecoveryMatter(matterID, caseID, counterparty, scope, basis, legalEntity, evidence, deadline, openedAt)
	if err != nil {
		return domain.RecoveryMatter{}, false, err
	}
	return matter, true, nil
}

// Save 写下一项追偿事项。事项不可回写，语句只插入；（案件+相对方+范围）撞唯一约束
// 如实报错——编排先 FindCurrent 短路，撞上说明并发另一方刚赢，重试会读到它。
func (repository *Recoveries) Save(
	ctx context.Context,
	tenant domain.TenantID,
	matter domain.RecoveryMatter,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save recovery matter: %w", err)
	}

	_, err = executor.Exec(ctx,
		`INSERT INTO visibility_exception.recovery_matter
			(tenant_id, matter_id, case_id, counterparty_ref, scope_ref,
			 basis_ref, legal_entity_ref, evidence_ref, deadline, opened_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		tenant.String(),
		matter.ID().String(),
		matter.Case().String(),
		matter.Counterparty().String(),
		matter.Scope().String(),
		matter.Basis().String(),
		matter.LegalEntity().String(),
		matter.Evidence().String(),
		matter.Deadline(),
		matter.OpenedAt(),
	)
	if err != nil {
		return fmt.Errorf("save recovery matter: %w", err)
	}
	return nil
}

// CountActions 交回该（事项+种类）已用到的最大尝试号——预先通知与正式主张各有各的
// 尝试序列。取 MAX 而不是数行：同一尝试的多个过程节点各占一行，数行会虚增序号。
func (repository *Recoveries) CountActions(
	ctx context.Context,
	tenant domain.TenantID,
	matter domain.RecoveryMatterID,
	kind domain.RecoveryActionKind,
) (int, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("count recovery actions: %w", err)
	}

	var attempts int
	err = querier.QueryRow(ctx,
		`SELECT COALESCE(MAX(attempt), 0)
		   FROM visibility_exception.recovery_action
		  WHERE tenant_id = $1 AND matter_id = $2 AND kind = $3`,
		tenant.String(), matter.String(), kind.String(),
	).Scan(&attempts)
	if err != nil {
		return 0, fmt.Errorf("count recovery actions: %w", err)
	}
	return attempts, nil
}

// AppendAction 追记一条动作节点。只增：没有 UPDATE 与 DELETE，所有尝试与内容版本
// 保留（硬句 185）。
func (repository *Recoveries) AppendAction(
	ctx context.Context,
	tenant domain.TenantID,
	action domain.RecoveryAction,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("append recovery action: %w", err)
	}

	_, err = executor.Exec(ctx,
		`INSERT INTO visibility_exception.recovery_action
			(tenant_id, matter_id, kind, attempt, milestone, content_ref, obligation, occurred_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		tenant.String(),
		action.Matter().String(),
		action.Kind().String(),
		action.Attempt(),
		action.Milestone().String(),
		action.ContentRef(),
		action.Obligation().String(),
		action.OccurredAt(),
	)
	if err != nil {
		return fmt.Errorf("append recovery action: %w", err)
	}
	return nil
}

func rebuildRecoveryMatter(
	matterID, caseID, counterparty, scope, basis, legalEntity, evidence string,
	deadline, openedAt time.Time,
) (domain.RecoveryMatter, error) {
	spec := domain.RecoveryMatterSpec{Deadline: deadline, OpenedAt: openedAt}
	var err error
	if spec.ID, err = domain.NewRecoveryMatterID(matterID); err != nil {
		return domain.RecoveryMatter{}, fmt.Errorf("rebuild recovery matter: %w", err)
	}
	if spec.Case, err = domain.NewCaseID(caseID); err != nil {
		return domain.RecoveryMatter{}, fmt.Errorf("rebuild recovery matter: %w", err)
	}
	if spec.Counterparty, err = domain.NewCounterpartyReference(counterparty); err != nil {
		return domain.RecoveryMatter{}, fmt.Errorf("rebuild recovery matter: %w", err)
	}
	if spec.Scope, err = domain.NewRequestScopeReference(scope); err != nil {
		return domain.RecoveryMatter{}, fmt.Errorf("rebuild recovery matter: %w", err)
	}
	if spec.Basis, err = domain.NewLiabilityBasisReference(basis); err != nil {
		return domain.RecoveryMatter{}, fmt.Errorf("rebuild recovery matter: %w", err)
	}
	if spec.LegalEntity, err = domain.NewLegalEntityReference(legalEntity); err != nil {
		return domain.RecoveryMatter{}, fmt.Errorf("rebuild recovery matter: %w", err)
	}
	if spec.Evidence, err = domain.NewRequestEvidenceReference(evidence); err != nil {
		return domain.RecoveryMatter{}, fmt.Errorf("rebuild recovery matter: %w", err)
	}
	// 事项不可变，重建即重走 OpenRecoveryMatter 的全部不变量（含期限在开启之后）。
	matter, err := domain.OpenRecoveryMatter(spec)
	if err != nil {
		return domain.RecoveryMatter{}, fmt.Errorf("rebuild recovery matter: %w", err)
	}
	return matter, nil
}

func eligibilityScreenFrom(value string) (domain.EligibilityScreen, error) {
	switch value {
	case "ELIGIBLE":
		return domain.ClaimEligible, nil
	case "INELIGIBLE":
		return domain.ClaimIneligible, nil
	default:
		return domain.EligibilityScreenInvalid,
			fmt.Errorf("visibility exception postgres: unknown eligibility screen %q", value)
	}
}

func liabilityConclusionFrom(value string) (domain.LiabilityConclusion, error) {
	switch value {
	case "FULLY_ESTABLISHED":
		return domain.LiabilityFullyEstablished, nil
	case "PARTIALLY_ESTABLISHED":
		return domain.LiabilityPartiallyEstablished, nil
	case "NOT_ESTABLISHED":
		return domain.LiabilityNotEstablished, nil
	case "UNDETERMINABLE":
		return domain.LiabilityUndeterminable, nil
	default:
		return domain.LiabilityConclusionInvalid,
			fmt.Errorf("visibility exception postgres: unknown liability conclusion %q", value)
	}
}

func stringPointer(value string) *string {
	return &value
}
