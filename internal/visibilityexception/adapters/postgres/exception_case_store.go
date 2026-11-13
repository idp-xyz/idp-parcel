package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// ExceptionCaseStore 实现 ports.ExceptionCaseStore：按（租户，案件标识）读回案件，并在册上仍是推进所基于的那一格时落
// 推进。它与 ExceptionCases（只答「案件活着吗」的那只视图）分开：那一只在构造期钉了租户，这一只的租户随调用到达。
type ExceptionCaseStore struct {
	db *bentopg.DB
}

func NewExceptionCaseStore(db *bentopg.DB) (*ExceptionCaseStore, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &ExceptionCaseStore{db: db}, nil
}

var _ ports.ExceptionCaseStore = (*ExceptionCaseStore)(nil)

func (store *ExceptionCaseStore) FindCase(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.CaseID,
) (*domain.ExceptionCase, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("find exception case: %w", err)
	}
	var (
		root, scope, team, phase                              string
		establishedAt                                         time.Time
		firstResponse, closedAt                               *time.Time
		conclusion, mergedInto, documentNumber, numberAbsence *string
	)
	err = querier.QueryRow(ctx,
		`SELECT root_parcel, impact_scope, responsible_team, phase, established_at,
		        first_response, closed_at, conclusion, merged_into, document_number, document_number_absence
		   FROM visibility_exception.exception_case
		  WHERE tenant_id = $1 AND case_id = $2`,
		tenant.String(), id.String(),
	).Scan(&root, &scope, &team, &phase, &establishedAt,
		&firstResponse, &closedAt, &conclusion, &mergedInto, &documentNumber, &numberAbsence)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("find exception case: %w", err)
	}
	snapshot := domain.ExceptionCaseSnapshot{ID: id, EstablishedAt: establishedAt.UTC()}
	if snapshot.Root, err = domain.NewTrackedParcelReference(root); err != nil {
		return nil, false, fmt.Errorf("find exception case: root: %w", err)
	}
	if snapshot.Scope, err = domain.NewImpactScopeReference(scope); err != nil {
		return nil, false, fmt.Errorf("find exception case: scope: %w", err)
	}
	if snapshot.Team, err = domain.NewResponsibleTeamReference(team); err != nil {
		return nil, false, fmt.Errorf("find exception case: team: %w", err)
	}
	if snapshot.Phase, err = casePhaseFromWord(phase); err != nil {
		return nil, false, fmt.Errorf("find exception case: %w", err)
	}
	if snapshot.DocumentNumber, err = domain.RehydrateDocumentNumber(textOrEmpty(documentNumber), textOrEmpty(numberAbsence)); err != nil {
		return nil, false, fmt.Errorf("find exception case: document number: %w", err)
	}
	if firstResponse != nil {
		snapshot.FirstResponse = firstResponse.UTC()
	}
	if closedAt != nil {
		snapshot.ClosedAt = closedAt.UTC()
	}
	snapshot.Conclusion = textOrEmpty(conclusion)
	if merged := textOrEmpty(mergedInto); merged != "" {
		if snapshot.MergedInto, err = domain.NewCaseID(merged); err != nil {
			return nil, false, fmt.Errorf("find exception case: merged into: %w", err)
		}
	}
	exceptionCase, err := domain.RehydrateCase(snapshot)
	if err != nil {
		return nil, false, fmt.Errorf("find exception case: %w", err)
	}
	return exceptionCase, true, nil
}

// LockCases 按案件标识升序给指名的行加行锁。顺序固定是为了两笔对向的归并不互相等死；不在册的标识不报错——在不在册
// 由随后的 FindCase 答。
func (store *ExceptionCaseStore) LockCases(ctx context.Context, tenant domain.TenantID, ids ...domain.CaseID) error {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("lock exception cases: %w", err)
	}
	wanted := make([]string, 0, len(ids))
	for _, id := range ids {
		wanted = append(wanted, id.String())
	}
	rows, err := executor.Query(ctx,
		`SELECT case_id
		   FROM visibility_exception.exception_case
		  WHERE tenant_id = $1 AND case_id = ANY($2)
		  ORDER BY case_id
		    FOR UPDATE`,
		tenant.String(), wanted)
	if err != nil {
		return fmt.Errorf("lock exception cases: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("lock exception cases: %w", err)
	}
	return nil
}

// AdvanceCase 落一次接单、关闭或归并。条件进语句：册上仍是 From 那一格才动。接单人只在这次推进形成了首次响应时写，
// 关闭人只在这次推进形成了关闭时写——已有的接单人不被关闭那一笔盖掉。
func (store *ExceptionCaseStore) AdvanceCase(ctx context.Context, advance ports.CaseAdvance) (ports.CaseAdvanceOutcome, error) {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CaseAdvanceOutcomeInvalid, fmt.Errorf("advance exception case: %w", err)
	}
	if advance.Case == nil || strings.TrimSpace(advance.Operator) == "" || advance.From.String() == "" {
		return ports.CaseAdvanceOutcomeInvalid, fmt.Errorf("advance exception case: a case, its prior phase and the deciding operator are required")
	}
	snapshot := advance.Case.Snapshot()
	tag, err := executor.Exec(ctx,
		`UPDATE visibility_exception.exception_case
		    SET phase = $3,
		        first_response = $4,
		        closed_at = $5,
		        conclusion = $6,
		        merged_into = $7,
		        first_response_by = CASE WHEN first_response IS NULL AND $4::timestamptz IS NOT NULL THEN $8 ELSE first_response_by END,
		        closed_by = CASE WHEN $5::timestamptz IS NOT NULL THEN $8 ELSE closed_by END
		  WHERE tenant_id = $1 AND case_id = $2 AND phase = $9`,
		advance.Tenant.String(),
		snapshot.ID.String(),
		snapshot.Phase.String(),
		nullIfZeroTime(snapshot.FirstResponse),
		nullIfZeroTime(snapshot.ClosedAt),
		nullIfBlank(snapshot.Conclusion),
		nullIfBlank(snapshot.MergedInto.String()),
		advance.Operator,
		advance.From.String(),
	)
	if err != nil {
		return ports.CaseAdvanceOutcomeInvalid, fmt.Errorf("advance exception case: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CasePhaseMoved, nil
	}
	return ports.CaseAdvanced, nil
}

func casePhaseFromWord(raw string) (domain.CasePhase, error) {
	for _, phase := range []domain.CasePhase{domain.CaseAwaitingResponse, domain.CaseInProgress, domain.CaseClosed} {
		if phase.String() == raw {
			return phase, nil
		}
	}
	return domain.CasePhaseInvalid, fmt.Errorf("phase %q is not a case phase word", raw)
}

func textOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
