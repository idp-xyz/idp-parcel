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

// ErrClaimHistoryStale 说明本次要落的补充期限历史落后于库里已记的那份，或与它分叉。
// 它不是「库坏了」：另一个写入方已经把历史推进了，调用方要读回赢家再重放（同 ADR-0031
// 分开「答不出」与「有人先到」的理由）。
var ErrClaimHistoryStale = errors.New("visibility exception postgres: claim supplement deadline history is stale")

// Claims 实现 ports.ClaimStore。索赔是判断历史推进的聚合（受理→过审→结论→复核/
// 撤回），Save 走 UPSERT 按键整行更新——三判形状由迁移 CHECK 与领域重建口两头把门，
// 复核换版走前版列（原结论保留，CONTEXT 253），不翻旧插新。
//
// 补充期限历史是另一回事：它是只增序列，Save 只追加不重写（见 appendSupplementDeadlines）。
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
		revision                                            int64
		customer, contract, target, kind                    string
		submittedAt                                         time.Time
		screen, screenBasis, conclusion, priorConclusion    *string
		concludedAt, reviewBy, withdrawnAt                  *time.Time
		withdrawn                                           bool
		missingMaterials, supplementScope, supplementNotice *string
		supplementDeadline                                  *time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT revision, customer_ref, contract_ref, target_ref, kind_ref, submitted_at,
		        screen, screen_basis, conclusion, concluded_at, review_by,
		        prior_conclusion, withdrawn, withdrawn_at,
		        missing_materials_ref, supplement_scope_ref, supplement_notice_ref,
		        supplement_deadline
		   FROM visibility_exception.claim_item
		  WHERE tenant_id = $1 AND batch_ref = $2 AND item_id = $3`,
		tenant.String(), batch.String(), item.String(),
	).Scan(&revision, &customer, &contract, &target, &kind, &submittedAt,
		&screen, &screenBasis, &conclusion, &concludedAt, &reviewBy,
		&priorConclusion, &withdrawn, &withdrawnAt,
		&missingMaterials, &supplementScope, &supplementNotice, &supplementDeadline)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("find claim item: %w", err)
	}

	snapshot := domain.ClaimItemSnapshot{
		Revision:    revision,
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
	if snapshot.Screen == domain.ClaimAwaitingSupplement {
		if missingMaterials == nil || supplementScope == nil || supplementNotice == nil || supplementDeadline == nil {
			return nil, false, fmt.Errorf("rebuild claim item: awaiting supplement missing required fields")
		}
		missing, err := domain.NewMissingMaterialsReference(*missingMaterials)
		if err != nil {
			return nil, false, fmt.Errorf("rebuild claim item: %w", err)
		}
		scope, err := domain.NewSupplementScopeReference(*supplementScope)
		if err != nil {
			return nil, false, fmt.Errorf("rebuild claim item: %w", err)
		}
		notice, err := domain.NewSupplementNoticeReference(*supplementNotice)
		if err != nil {
			return nil, false, fmt.Errorf("rebuild claim item: %w", err)
		}
		if snapshot.Supplement, err = domain.NewSupplementRequirement(missing, scope, notice, *supplementDeadline); err != nil {
			return nil, false, fmt.Errorf("rebuild claim item: %w", err)
		}
	}
	history, err := loadSupplementDeadlines(ctx, querier, tenant, batch, item)
	if err != nil {
		return nil, false, err
	}
	snapshot.DeadlineHistory = history

	claim, err := domain.RehydrateClaimItem(snapshot)
	if err != nil {
		return nil, false, fmt.Errorf("rebuild claim item: %w", err)
	}
	return claim, true, nil
}

// CountLiveScopeClaims 数同租户下与本项同（客户账户+目标范围+索赔类型）的其他在办
// 索赔，供资格审核的重复关系那一维取事实。
//
// 不带批次：重复是跨批次的事——同一客户把同一包裹的同一类型索赔分两批提交，正是这一
// 维要认出来的情形，按批次收窄就永远数不到。已撤回的不数（`AT-VE-123`：撤回后重新
// 提交同一范围要建立新索赔项并重新检查重复关系，把撤回那项算进来同一范围就再也提不了
// 第二次）；本项自己按项标识排除——重判一项在办索赔不该把它数成自己的重复。
//
// 只回计数不回索赔：这一维要的是「有没有、有几个」，交出整份别的索赔会让调用方顺手
// 读到与本次审核无关的内容。
func (repository *Claims) CountLiveScopeClaims(
	ctx context.Context,
	tenant domain.TenantID,
	customer domain.CustomerAccountReference,
	target domain.RequestScopeReference,
	kind domain.ClaimKindReference,
	excluding domain.ClaimItemID,
) (int, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return 0, fmt.Errorf("count live scope claims: %w", err)
	}

	var count int
	err = querier.QueryRow(ctx,
		`SELECT count(*)
		   FROM visibility_exception.claim_item
		  WHERE tenant_id = $1
		    AND customer_ref = $2
		    AND target_ref = $3
		    AND kind_ref = $4
		    AND item_id <> $5
		    AND NOT withdrawn`,
		tenant.String(), customer.String(), target.String(), kind.String(), excluding.String(),
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count live scope claims: %w", err)
	}
	return count, nil
}

// Save 落索赔的当前判断历史。同键整行更新（受理后每一步判断都经同一入口落库）；
// 键列与提交事实列在更新时同样重写——它们不可变，重写等值是无害的，靠 CHECK 与
// 领域门拦形状而不是在这里分列。
//
// 整行重写要求并发保护：期望修订由索赔项携带，写在 DO UPDATE 自己的 WHERE 上，检查、
// 递增与回报由这一条语句一起完成。不先读后写——先 SELECT 再判等会在读与写之间留一道
// 缝，两个写入方都能读到同一版再双双通过。命中零行即`版本冲突`：另一方已经推进过这项
// 索赔，本方手里的快照不再是当前那一版。
//
// 新版本号取`期望修订 + 1` 而不是常量 1，这样索赔行不存在时那一支（受理首落，期望为
// 零）与更新支共用同一条口径，且修订永不回退——万一有行在别处消失，重建出来的那份也
// 排在旧持有者的期望之后，旧持有者随后照样撞冲突而不是把行盖回去。
func (repository *Claims) Save(
	ctx context.Context,
	tenant domain.TenantID,
	claim *domain.ClaimItem,
) (ports.ClaimSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ClaimSaveOutcomeInvalid, fmt.Errorf("save claim item: %w", err)
	}
	if claim == nil {
		return ports.ClaimSaveOutcomeInvalid, fmt.Errorf("save claim item: claim is nil")
	}
	snapshot := claim.Snapshot()

	var screen, screenBasis, conclusion, prior *string
	if value := snapshot.Screen.String(); value != "" {
		screen = stringPointer(value)
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

	var missingMaterials, supplementScope, supplementNotice *string
	var supplementDeadline *time.Time
	if snapshot.Screen == domain.ClaimAwaitingSupplement {
		missingMaterials = stringPointer(snapshot.Supplement.MissingMaterials.String())
		supplementScope = stringPointer(snapshot.Supplement.Scope.String())
		supplementNotice = stringPointer(snapshot.Supplement.Notice.String())
		deadline := snapshot.Supplement.Deadline
		supplementDeadline = &deadline
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.claim_item
			(tenant_id, batch_ref, item_id, customer_ref, contract_ref, target_ref, kind_ref,
			 submitted_at, screen, screen_basis, conclusion, concluded_at, review_by,
			 prior_conclusion, withdrawn, withdrawn_at,
			 missing_materials_ref, supplement_scope_ref, supplement_notice_ref, supplement_deadline,
			 revision)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
			 $21 + 1)
		 ON CONFLICT (tenant_id, batch_ref, item_id) DO UPDATE SET
			screen = EXCLUDED.screen,
			screen_basis = EXCLUDED.screen_basis,
			conclusion = EXCLUDED.conclusion,
			concluded_at = EXCLUDED.concluded_at,
			review_by = EXCLUDED.review_by,
			prior_conclusion = EXCLUDED.prior_conclusion,
			withdrawn = EXCLUDED.withdrawn,
			withdrawn_at = EXCLUDED.withdrawn_at,
			missing_materials_ref = EXCLUDED.missing_materials_ref,
			supplement_scope_ref = EXCLUDED.supplement_scope_ref,
			supplement_notice_ref = EXCLUDED.supplement_notice_ref,
			supplement_deadline = EXCLUDED.supplement_deadline,
			revision = claim_item.revision + 1
		  WHERE claim_item.revision = $21`,
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
		missingMaterials,
		supplementScope,
		supplementNotice,
		supplementDeadline,
		snapshot.Revision,
	)
	if err != nil {
		return ports.ClaimSaveOutcomeInvalid, fmt.Errorf("save claim item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ClaimRevisionConflict, nil
	}
	if err := appendSupplementDeadlines(ctx, executor, tenant, snapshot); err != nil {
		return ports.ClaimSaveOutcomeInvalid, err
	}
	return ports.ClaimSaved, nil
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

// Save 写下一项追偿事项。事项不可回写，语句只插入。同（案件+相对方+范围）已有事项
// 时 ON CONFLICT DO NOTHING——业务答案是已有，不是错误（ADR-0031）。捕 23505 会
// 把本事务后续语句一并废掉，先查也拦不住并发赢家。零行命中交回 AlreadyRecorded，
// 调用方同事务还能继续读赢家。
func (repository *Recoveries) Save(
	ctx context.Context,
	tenant domain.TenantID,
	matter domain.RecoveryMatter,
) (ports.RecoverySaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.RecoverySaveOutcomeInvalid, fmt.Errorf("save recovery matter: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.recovery_matter
			(tenant_id, matter_id, case_id, counterparty_ref, scope_ref,
			 basis_ref, legal_entity_ref, evidence_ref, deadline, opened_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT (tenant_id, case_id, counterparty_ref, scope_ref) DO NOTHING`,
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
		return ports.RecoverySaveOutcomeInvalid, fmt.Errorf("save recovery matter: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.RecoveryAlreadyRecorded, nil
	}
	return ports.RecoverySaved, nil
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
// 保留（硬句 185）。行无业务唯一约束（同一尝试的多个过程节点各占一行，主键是
// identity seq）——没有 23505 可撞，无需 ON CONFLICT。
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

func loadSupplementDeadlines(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	batch domain.ClaimBatchReference,
	item domain.ClaimItemID,
) ([]domain.SupplementDeadlineVersion, error) {
	rows, err := querier.Query(ctx,
		`SELECT deadline, established_at
		   FROM visibility_exception.claim_supplement_deadline
		  WHERE tenant_id = $1 AND batch_ref = $2 AND item_id = $3
		  ORDER BY version_seq`,
		tenant.String(), batch.String(), item.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("load supplement deadlines: %w", err)
	}
	defer rows.Close()
	var history []domain.SupplementDeadlineVersion
	for rows.Next() {
		var version domain.SupplementDeadlineVersion
		if err := rows.Scan(&version.Deadline, &version.EstablishedAt); err != nil {
			return nil, fmt.Errorf("load supplement deadlines: %w", err)
		}
		history = append(history, version)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load supplement deadlines: %w", err)
	}
	return history, nil
}

// appendSupplementDeadlines 只追加期限版本，一行都不删。
//
// 删光重插会丢东西：两个写入方各持一份旧快照时，后提交的那个按自己的历史重写整段，
// 另一方刚批的延期就此消失。它不是可以按到达顺序覆盖的中间态——期限是对客户作过的
// 承诺，`AT-VE-119` 之后的每一次判定都按它算。
//
// 落后（库里版本更多）与分叉（同一版内容不同）都停下报 ErrClaimHistoryStale，由调用方
// 读回赢家再重放。静默截断与静默改写都会让一次并发写入变成一份没人发现的历史。
//
// 本函数跑在 Save 的事务里、且在索赔行条件更新命中之后：那次写入已经把该行锁住，
// 同键的并发 Save 因此排成序，这里读到的库内历史不会在读与写之间被第三方推进。
//
// 并发那一路由修订守卫先答——落后的写入撞的是`版本冲突`，走不到这里。本函数因此守的
// 是另一格：修订对得上、历史却对不上的快照（手工拼出的重建规格是唯一进得来的路）。
// 两道都留着，因为它们拦的不是同一件事，而这一段护的是对客户作过的承诺。
func appendSupplementDeadlines(
	ctx context.Context,
	executor bentopg.Executor,
	tenant domain.TenantID,
	snapshot domain.ClaimItemSnapshot,
) error {
	stored, err := loadSupplementDeadlines(ctx, executor, tenant, snapshot.Batch, snapshot.ID)
	if err != nil {
		return err
	}
	if len(stored) > len(snapshot.DeadlineHistory) {
		return fmt.Errorf("%w：库中 %d 版，本次快照只有 %d 版",
			ErrClaimHistoryStale, len(stored), len(snapshot.DeadlineHistory))
	}
	for seq, version := range snapshot.DeadlineHistory {
		if seq < len(stored) {
			if !stored[seq].Deadline.Equal(version.Deadline) ||
				!stored[seq].EstablishedAt.Equal(version.EstablishedAt) {
				return fmt.Errorf("%w：第 %d 版与库中已记的那一版不是同一个", ErrClaimHistoryStale, seq+1)
			}
			continue
		}
		if _, err := executor.Exec(ctx,
			`INSERT INTO visibility_exception.claim_supplement_deadline
				(tenant_id, batch_ref, item_id, version_seq, deadline, established_at)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			tenant.String(), snapshot.Batch.String(), snapshot.ID.String(),
			seq+1, version.Deadline, version.EstablishedAt); err != nil {
			return fmt.Errorf("append supplement deadlines: %w", err)
		}
	}
	return nil
}

func eligibilityScreenFrom(value string) (domain.EligibilityScreen, error) {
	switch value {
	case "ELIGIBLE":
		return domain.ClaimEligible, nil
	case "INELIGIBLE":
		return domain.ClaimIneligible, nil
	case "AWAITING_SUPPLEMENT":
		return domain.ClaimAwaitingSupplement, nil
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
