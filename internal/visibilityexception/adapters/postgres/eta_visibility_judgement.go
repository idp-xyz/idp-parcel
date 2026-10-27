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

// ErrCurrentJudgementChanged 表示乐观锁失手：首发时已有别人占住当前，或替代时当前版已不是新判断所指的
// 前版。两个并发判断只有一个赢——输家重读当前再走重放或替代，覆盖会把赢家的判断链拆断。
var ErrCurrentJudgementChanged = errors.New("visibility exception postgres: current eta visibility judgement changed")

// ETAVisibilityJudgements 实现 ports.ETAVisibilityJudgementStore。版本行只增不改写：Save 追加版本行并把
// eta_visibility_judgement_current 的显式标记指向它，被替代的判断连同依据留存、按版本读得回；读当前版走
// 标记直取，不扫描历史。标记是唯一的可变处。
type ETAVisibilityJudgements struct {
	db *bentopg.DB
}

func NewETAVisibilityJudgements(db *bentopg.DB) (*ETAVisibilityJudgements, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &ETAVisibilityJudgements{db: db}, nil
}

var _ ports.ETAVisibilityJudgementStore = (*ETAVisibilityJudgements)(nil)

const etaJudgementColumns = `v.version, v.milestone_ref, v.candidate_eta_version, v.candidate_source,
		        v.conclusion, v.reason, v.gate_version, v.service_rule_version,
		        v.expression, v.stop_boundary_ref, v.judged_at, v.prior_version`

// FindCurrent 按（租户+账户+包裹）取回当前判断。否定结果只回 false，不区分「不存在」「属于另一个账户」与
// 「属于另一个租户」（ADR-0029）。读回的一切经领域重建口过一遍结论与字段成对的不变量。
func (repository *ETAVisibilityJudgements) FindCurrent(
	ctx context.Context,
	tenant domain.TenantID,
	customer domain.CustomerAccountReference,
	parcel domain.TrackedParcelReference,
) (domain.ETAVisibilityJudgement, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ETAVisibilityJudgement{}, false, fmt.Errorf("find current eta visibility judgement: %w", err)
	}
	judgement, found, err := scanETAVisibilityJudgement(customer, parcel, querier.QueryRow(ctx,
		`SELECT `+etaJudgementColumns+`
		   FROM visibility_exception.eta_visibility_judgement_current AS c
		   JOIN visibility_exception.eta_visibility_judgement_version AS v
		     ON v.tenant_id = c.tenant_id
		    AND v.customer_account_id = c.customer_account_id
		    AND v.parcel_id = c.parcel_id
		    AND v.version = c.version
		  WHERE c.tenant_id = $1
		    AND c.customer_account_id = $2
		    AND c.parcel_id = $3`,
		tenant.String(), customer.String(), parcel.String(),
	))
	if err != nil {
		return domain.ETAVisibilityJudgement{}, false, fmt.Errorf("find current eta visibility judgement: %w", err)
	}
	return judgement, found, nil
}

// FindVersion 按版本读回该账户该包裹名下留存的任一版，当前版或已被替代的皆可。
func (repository *ETAVisibilityJudgements) FindVersion(
	ctx context.Context,
	tenant domain.TenantID,
	customer domain.CustomerAccountReference,
	parcel domain.TrackedParcelReference,
	version domain.ETAVisibilityJudgementVersionID,
) (domain.ETAVisibilityJudgement, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ETAVisibilityJudgement{}, false, fmt.Errorf("find eta visibility judgement by version: %w", err)
	}
	judgement, found, err := scanETAVisibilityJudgement(customer, parcel, querier.QueryRow(ctx,
		`SELECT `+etaJudgementColumns+`
		   FROM visibility_exception.eta_visibility_judgement_version AS v
		  WHERE v.tenant_id = $1
		    AND v.customer_account_id = $2
		    AND v.parcel_id = $3
		    AND v.version = $4`,
		tenant.String(), customer.String(), parcel.String(), version.String(),
	))
	if err != nil {
		return domain.ETAVisibilityJudgement{}, false, fmt.Errorf("find eta visibility judgement by version: %w", err)
	}
	return judgement, found, nil
}

func scanETAVisibilityJudgement(
	customer domain.CustomerAccountReference,
	parcel domain.TrackedParcelReference,
	row pgx.Row,
) (domain.ETAVisibilityJudgement, bool, error) {
	var (
		version, milestone, candidate, source, conclusion, reason string
		gate, service, expression, boundary, prior                *string
		judgedAt                                                  time.Time
	)
	err := row.Scan(&version, &milestone, &candidate, &source, &conclusion, &reason,
		&gate, &service, &expression, &boundary, &judgedAt, &prior)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ETAVisibilityJudgement{}, false, nil
	}
	if err != nil {
		return domain.ETAVisibilityJudgement{}, false, err
	}

	finding := domain.ETAVisibilityFinding{Parcel: parcel}
	snapshot := domain.ETAVisibilityJudgementSnapshot{Customer: customer, JudgedAt: judgedAt}
	if snapshot.Version, err = domain.NewETAVisibilityJudgementVersionID(version); err != nil {
		return domain.ETAVisibilityJudgement{}, false, err
	}
	if finding.Milestone, err = domain.NewMilestoneReference(milestone); err != nil {
		return domain.ETAVisibilityJudgement{}, false, err
	}
	if finding.CandidateVersion, err = domain.NewETAVersionID(candidate); err != nil {
		return domain.ETAVisibilityJudgement{}, false, err
	}
	if finding.CandidateSource, err = etaSourceFrom(source); err != nil {
		return domain.ETAVisibilityJudgement{}, false, err
	}
	if finding.Conclusion, err = etaConclusionFrom(conclusion); err != nil {
		return domain.ETAVisibilityJudgement{}, false, err
	}
	if finding.Reason, err = etaReasonFrom(reason); err != nil {
		return domain.ETAVisibilityJudgement{}, false, err
	}
	if gate != nil {
		if finding.GateVersion, err = domain.NewETAQualityGateVersion(*gate); err != nil {
			return domain.ETAVisibilityJudgement{}, false, err
		}
	}
	if service != nil {
		if finding.ServiceRule, err = domain.NewCustomerServiceRuleVersion(*service); err != nil {
			return domain.ETAVisibilityJudgement{}, false, err
		}
	}
	if expression != nil {
		if finding.Expression, err = etaExpressionFrom(*expression); err != nil {
			return domain.ETAVisibilityJudgement{}, false, err
		}
	}
	if boundary != nil {
		if finding.StopBoundary, err = domain.NewETAStopShowingBoundary(*boundary); err != nil {
			return domain.ETAVisibilityJudgement{}, false, err
		}
	}
	if prior != nil {
		if snapshot.Prior, err = domain.NewETAVisibilityJudgementVersionID(*prior); err != nil {
			return domain.ETAVisibilityJudgement{}, false, err
		}
	}
	snapshot.Finding = finding

	judgement, err := domain.RehydrateETAVisibilityJudgement(snapshot)
	if err != nil {
		return domain.ETAVisibilityJudgement{}, false, err
	}
	return judgement, true, nil
}

// Save 追加判断版本并移当前标记：首版只在无当前标记时立标记（ON CONFLICT DO NOTHING），替代只在标记恰指
// 新版本所指前版时移标记（UPDATE ... WHERE version = prior 的乐观锁）。零行命中都是
// ErrCurrentJudgementChanged；输家已写下的版本行随同一事务回滚，不留一版从未成为当前的判断。
//
// 走 RequireExecutor：判断提交与同一步的客户视图重派生必须同生共死。
func (repository *ETAVisibilityJudgements) Save(
	ctx context.Context,
	tenant domain.TenantID,
	judgement domain.ETAVisibilityJudgement,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save eta visibility judgement: %w", err)
	}

	finding := judgement.Finding()
	var gate, service, expression, boundary *string
	if finding.GateVersion.String() != "" {
		gate = stringPointer(finding.GateVersion.String())
	}
	if finding.ServiceRule.String() != "" {
		service = stringPointer(finding.ServiceRule.String())
	}
	if finding.Expression != domain.ETAExpressionInvalid {
		expression = stringPointer(finding.Expression.String())
	}
	if finding.StopBoundary.String() != "" {
		boundary = stringPointer(finding.StopBoundary.String())
	}
	prior, superseding := judgement.PriorVersion()
	var priorColumn *string
	if superseding {
		priorColumn = stringPointer(prior.String())
	}

	if _, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.eta_visibility_judgement_version
			(tenant_id, customer_account_id, parcel_id, version,
			 milestone_ref, candidate_eta_version, candidate_source,
			 conclusion, reason, gate_version, service_rule_version, expression, stop_boundary_ref,
			 judged_at, prior_version)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		tenant.String(), judgement.Customer().String(), judgement.Parcel().String(), judgement.Version().String(),
		finding.Milestone.String(), finding.CandidateVersion.String(), finding.CandidateSource.String(),
		finding.Conclusion.String(), finding.Reason.String(), gate, service, expression, boundary,
		judgement.JudgedAt().UTC(), priorColumn,
	); err != nil {
		return fmt.Errorf("save eta visibility judgement version: %w", err)
	}

	if !superseding {
		tag, err := executor.Exec(ctx,
			`INSERT INTO visibility_exception.eta_visibility_judgement_current
				(tenant_id, customer_account_id, parcel_id, version)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT DO NOTHING`,
			tenant.String(), judgement.Customer().String(), judgement.Parcel().String(), judgement.Version().String(),
		)
		if err != nil {
			return fmt.Errorf("save eta visibility judgement current marker: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("%w", ErrCurrentJudgementChanged)
		}
		return nil
	}

	tag, err := executor.Exec(ctx,
		`UPDATE visibility_exception.eta_visibility_judgement_current
		    SET version = $4
		  WHERE tenant_id = $1
		    AND customer_account_id = $2
		    AND parcel_id = $3
		    AND version = $5`,
		tenant.String(), judgement.Customer().String(), judgement.Parcel().String(),
		judgement.Version().String(), prior.String(),
	)
	if err != nil {
		return fmt.Errorf("save eta visibility judgement current marker: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w", ErrCurrentJudgementChanged)
	}
	return nil
}

func etaConclusionFrom(value string) (domain.ETAVisibilityConclusion, error) {
	switch value {
	case "CUSTOMER_VISIBLE_ETA":
		return domain.CustomerVisibleETA, nil
	case "ETA_NOT_VISIBLE":
		return domain.ETANotVisible, nil
	case "ETA_VISIBILITY_PENDING":
		return domain.ETAVisibilityPendingJudgement, nil
	default:
		return domain.ETAVisibilityConclusionInvalid,
			fmt.Errorf("eta visibility judgement: unknown conclusion %q", value)
	}
}

func etaReasonFrom(value string) (domain.ETAVisibilityReason, error) {
	switch value {
	case "QUALITY_MET_GATE":
		return domain.QualityMetTheGate, nil
	case "QUALITY_BELOW_GATE":
		return domain.QualityBelowTheGate, nil
	case "RULE_NOT_CONFIGURED":
		return domain.CustomerRuleNotConfigured, nil
	case "RULE_CONFLICT":
		return domain.CustomerRuleInConflict, nil
	default:
		return domain.ETAVisibilityReasonInvalid,
			fmt.Errorf("eta visibility judgement: unknown reason %q", value)
	}
}

func etaExpressionFrom(value string) (domain.ETAExpression, error) {
	switch value {
	case "POINT_VALUE":
		return domain.ETAShownAsPointValue, nil
	case "TIME_RANGE":
		return domain.ETAShownAsTimeRange, nil
	default:
		return domain.ETAExpressionInvalid,
			fmt.Errorf("eta visibility judgement: unknown expression %q", value)
	}
}
