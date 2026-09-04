// Package postgres 是 customs-compliance 自有语义端口的 PostgreSQL 适配器。
//
// 显式 SQL、行模型与写入代数翻译都留在这里，不进领域对象。所有语句显式携带租户
// 条件：按 ADR-0003 运营集团租户是最高数据隔离边界，缺了它另一个租户的同名来源
// 响应就会被当成同一次接收。
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// ExternalResults 实现 ports.ExternalResultStore（写入代数同 ADR-0031）。
type ExternalResults struct {
	db *bentopg.DB
}

func NewExternalResults(db *bentopg.DB) (*ExternalResults, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &ExternalResults{db: db}, nil
}

// resultRow 是 result jsonb 列的行模型，只在本包存在；领域对象经 InterpretExternalResult
// 重建，行模型不外泄。
type resultRow struct {
	Layer        string    `json:"layer"`
	SourceID     string    `json:"sourceId"`
	Role         string    `json:"role"`
	RawSemantics string    `json:"rawSemantics"`
	Rule         string    `json:"rule"`
	Version      string    `json:"version"`
	Attempt      int       `json:"attempt"`
	Scope        string    `json:"scope"`
	OccurredAt   time.Time `json:"occurredAt"`
	ReceivedAt   time.Time `json:"receivedAt"`
}

// FindByKey 按（租户+来源标识）取回接收记录。否定结果只回 false，不区分「不存在」
// 与「属于另一个租户」。读回的可归属事实经领域构造函数重建；归属不上的记录如实无
// 事实——留存不猜在读回处同样成立。
func (repository *ExternalResults) FindByKey(
	ctx context.Context,
	key ports.ExternalResultKey,
) (ports.ExternalResultRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ExternalResultRecord{}, false, fmt.Errorf("find external result: %w", err)
	}

	var (
		digest, rawSemantics, claimedVersion string
		unattributable, layerConflict        bool
		resultJSON                           []byte
		recordedAt                           time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT content_digest, unattributable, raw_semantics, claimed_version,
		        layer_conflict, result, recorded_at
		   FROM customs_compliance.external_result
		  WHERE tenant_id = $1
		    AND source_id = $2`,
		key.TenantID.String(),
		key.SourceID,
	).Scan(&digest, &unattributable, &rawSemantics, &claimedVersion,
		&layerConflict, &resultJSON, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ExternalResultRecord{}, false, nil
	}
	if err != nil {
		return ports.ExternalResultRecord{}, false, fmt.Errorf("find external result: %w", err)
	}

	record := ports.ExternalResultRecord{
		Key:            key,
		ContentDigest:  digest,
		Unattributable: unattributable,
		RawSemantics:   rawSemantics,
		ClaimedVersion: claimedVersion,
		LayerConflict:  layerConflict,
		RecordedAt:     recordedAt.UTC(),
	}
	if len(resultJSON) > 0 {
		result, err := rebuildResult(resultJSON)
		if err != nil {
			return ports.ExternalResultRecord{}, false, fmt.Errorf("find external result: %w", err)
		}
		record.Result = result
		if result.Layer() == domain.ReleaseResultLayer {
			release, err := loadReleaseOutcome(ctx, querier, key)
			if err != nil {
				return ports.ExternalResultRecord{}, false, fmt.Errorf("find external result: release outcome: %w", err)
			}
			record.Release = release
		}
	}
	return record, true, nil
}

// Save 写下一次接收记录。同（租户+来源标识）已有记录时答`已有记录`——业务答案不是
// 错误（ADR-0031）；用 ON CONFLICT DO NOTHING 而不是捕 23505 译码：撞键的 INSERT 会
// 把整个事务打进中止态，而编排拿到`已有记录`还要在同一个事务里读回原记录作答。
//
// 走 RequireExecutor：接收记录落库与将来同一步的意图发布必须同生共死。
func (repository *ExternalResults) Save(
	ctx context.Context,
	record ports.ExternalResultRecord,
) (ports.ExternalResultSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ExternalResultSaveOutcomeInvalid, fmt.Errorf("save external result: %w", err)
	}

	var resultJSON []byte
	var submissionVersion *string
	if !record.Unattributable {
		row := rowOf(record.Result)
		resultJSON, err = json.Marshal(row)
		if err != nil {
			return ports.ExternalResultSaveOutcomeInvalid, fmt.Errorf("save external result: %w", err)
		}
		version := record.Result.Version().String()
		submissionVersion = &version
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.external_result
			(tenant_id, source_id, content_digest, unattributable, raw_semantics,
			 claimed_version, layer_conflict, result, submission_version, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.SourceID,
		record.ContentDigest,
		record.Unattributable,
		record.RawSemantics,
		record.ClaimedVersion,
		record.LayerConflict,
		resultJSON,
		submissionVersion,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.ExternalResultSaveOutcomeInvalid, fmt.Errorf("save external result: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ExternalResultAlreadyRecorded, nil
	}
	if record.Release != nil {
		// 放行事实与分层事实同键同事务落地（0015）。只在分层事实这一行真落了之后才写——
		// 撞键的重放连分层事实都没重写，放行行更不该被后来者顶掉；两行的先后由外键钉住。
		if err := insertReleaseOutcome(ctx, executor, record); err != nil {
			return ports.ExternalResultSaveOutcomeInvalid, fmt.Errorf("save external result: %w", err)
		}
	}
	return ports.ExternalResultSaved, nil
}

func insertReleaseOutcome(ctx context.Context, executor bentopg.Executor, record ports.ExternalResultRecord) error {
	release := record.Release
	kind := release.Kind().String()
	if kind == "" {
		return fmt.Errorf("release outcome: unknown release kind %d", release.Kind())
	}
	condition, _ := release.Condition()
	_, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.release_outcome
			(tenant_id, source_id, kind, authority_ref, scope_ref, condition, received_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		record.Key.TenantID.String(),
		record.Key.SourceID,
		kind,
		release.Authority().String(),
		release.Scope().String(),
		condition,
		release.ReceivedAt().UTC(),
	)
	return err
}

// loadReleaseOutcome 读回同键的放行事实；没有即 nil——非放行层的记录本就没有这一面。
func loadReleaseOutcome(
	ctx context.Context,
	querier bentopg.Querier,
	key ports.ExternalResultKey,
) (*domain.CustomsReleaseOutcome, error) {
	var (
		kind, authority, scope, condition string
		receivedAt                        time.Time
	)
	err := querier.QueryRow(ctx,
		`SELECT kind, authority_ref, scope_ref, condition, received_at
		   FROM customs_compliance.release_outcome
		  WHERE tenant_id = $1 AND source_id = $2`,
		key.TenantID.String(), key.SourceID,
	).Scan(&kind, &authority, &scope, &condition, &receivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	releaseKind, err := releaseKindFrom(kind)
	if err != nil {
		return nil, err
	}
	authorityRef, err := domain.NewRegulatoryAuthorityReference(authority)
	if err != nil {
		return nil, err
	}
	scopeRef, err := domain.NewDecisionScopeReference(scope)
	if err != nil {
		return nil, err
	}
	release, err := domain.ReceiveReleaseOutcome(releaseKind, authorityRef, scopeRef, condition, receivedAt)
	if err != nil {
		return nil, err
	}
	return &release, nil
}

func releaseKindFrom(raw string) (domain.ReleaseKind, error) {
	for _, kind := range []domain.ReleaseKind{
		domain.FullRelease,
		domain.PartialRelease,
		domain.ConditionalRelease,
	} {
		if kind.String() == raw {
			return kind, nil
		}
	}
	return 0, fmt.Errorf("unknown release kind %q", raw)
}

// LoadForSubmission 按（租户+提交版本）读回同一提交已保存的各层事实，供同层一致性
// 比对。只回可归属事实——归属不上的留存记录没有监管事实可比。顺序按入库序稳定。
func (repository *ExternalResults) LoadForSubmission(
	ctx context.Context,
	tenant domain.TenantID,
	version domain.SubmissionVersionID,
) ([]domain.ExternalResult, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("load external results: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT result
		   FROM customs_compliance.external_result
		  WHERE tenant_id = $1
		    AND submission_version = $2
		  ORDER BY inserted_at, source_id`,
		tenant.String(),
		version.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("load external results: %w", err)
	}
	defer rows.Close()

	var results []domain.ExternalResult
	for rows.Next() {
		var resultJSON []byte
		if err := rows.Scan(&resultJSON); err != nil {
			return nil, fmt.Errorf("load external results: %w", err)
		}
		result, err := rebuildResult(resultJSON)
		if err != nil {
			return nil, fmt.Errorf("load external results: %w", err)
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load external results: %w", err)
	}
	return results, nil
}

func rowOf(result domain.ExternalResult) resultRow {
	return resultRow{
		Layer:        result.Layer().String(),
		SourceID:     result.SourceID(),
		Role:         result.Role().String(),
		RawSemantics: result.RawSemantics(),
		Rule:         result.Rule().String(),
		Version:      result.Version().String(),
		Attempt:      result.Attempt(),
		Scope:        result.Scope().String(),
		OccurredAt:   result.OccurredAt().UTC(),
		ReceivedAt:   result.ReceivedAt().UTC(),
	}
}

func rebuildResult(raw []byte) (domain.ExternalResult, error) {
	var row resultRow
	if err := json.Unmarshal(raw, &row); err != nil {
		return domain.ExternalResult{}, err
	}
	layer, err := resultLayerFrom(row.Layer)
	if err != nil {
		return domain.ExternalResult{}, err
	}
	role, err := domain.NewSourceAuthorityRole(row.Role)
	if err != nil {
		return domain.ExternalResult{}, err
	}
	rule, err := domain.NewInterpretationRuleReference(row.Rule)
	if err != nil {
		return domain.ExternalResult{}, err
	}
	version, err := domain.NewSubmissionVersionID(row.Version)
	if err != nil {
		return domain.ExternalResult{}, err
	}
	scope, err := domain.NewDecisionScopeReference(row.Scope)
	if err != nil {
		return domain.ExternalResult{}, err
	}
	return domain.InterpretExternalResult(domain.ExternalResultSpec{
		Layer:        layer,
		SourceID:     row.SourceID,
		Role:         role,
		RawSemantics: row.RawSemantics,
		Rule:         rule,
		Version:      version,
		Attempt:      row.Attempt,
		Scope:        scope,
		OccurredAt:   row.OccurredAt,
		ReceivedAt:   row.ReceivedAt,
	})
}

func resultLayerFrom(raw string) (domain.ResultLayer, error) {
	for _, layer := range []domain.ResultLayer{
		domain.RegulatoryReceiptLayer,
		domain.BusinessAcceptanceLayer,
		domain.ProcessDecisionLayer,
		domain.AssessedDutyLayer,
		domain.ReleaseResultLayer,
		domain.DispositionDecisionLayer,
	} {
		if layer.String() == raw {
			return layer, nil
		}
	}
	return 0, fmt.Errorf("unknown result layer %q", raw)
}
