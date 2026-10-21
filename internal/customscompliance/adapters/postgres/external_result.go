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
	release, err := rebuildReleaseOutcome(kind, authority, scope, condition, receivedAt)
	if err != nil {
		return nil, err
	}
	return &release, nil
}

// rebuildReleaseOutcome 把 release_outcome 一行的各列经领域构造函数重建成放行事实。
func rebuildReleaseOutcome(kind, authority, scope, condition string, receivedAt time.Time) (domain.CustomsReleaseOutcome, error) {
	releaseKind, err := releaseKindFrom(kind)
	if err != nil {
		return domain.CustomsReleaseOutcome{}, err
	}
	authorityRef, err := domain.NewRegulatoryAuthorityReference(authority)
	if err != nil {
		return domain.CustomsReleaseOutcome{}, err
	}
	scopeRef, err := domain.NewDecisionScopeReference(scope)
	if err != nil {
		return domain.CustomsReleaseOutcome{}, err
	}
	return domain.ReceiveReleaseOutcome(releaseKind, authorityRef, scopeRef, condition, receivedAt)
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
// 比对。只回可归属事实——归属不上的留存记录没有监管事实可比。记了同层冲突的也回：冲突
// 关系在两份事实之间，被标的那份仍要参与比对（判法见 domain.CheckLayerConsistency）。
// 放行层连同放行事实一起读回；放行事实落点建立之前的放行层存量没有那一行，如实不带。
// 顺序按入库序稳定。
//
// 查询之前按（租户、提交版本）取事务级咨询锁，把「读同一提交的在册事实 → 判同层一致性 →
// 落册」串起来：读已提交下两份同层结果在两笔并发事务里到达，各自读不到对方尚未提交的那一行，
// 会双双判不矛盾、都记无冲突。后到的一方等在锁上，先到的提交后才查；这条查询另取快照，读得到
// 刚提交的那一行。锁随事务释放，所以走 RequireExecutor：没有事务时锁在取锁那条语句结束就放掉，
// 读完到落册之间不再串行，静默放过比报错更糟。粒度取提交版本，因为比对本就只在同一提交版本内
// 做；锁键各维以 chr(31) 分隔后取 hashtext，口径同 TF 的 BookingAnswers.FindAnswersForUpdate，
// 哈希撞上只多等一会儿，不错判。
//
// 某一行装不回领域对象即仓储不变量已破，交出 ports.ErrLayerFactInconsistent，与读不动分开。
func (repository *ExternalResults) LoadForSubmission(
	ctx context.Context,
	tenant domain.TenantID,
	version domain.SubmissionVersionID,
) ([]domain.LayerFact, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("load external results: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('customs_compliance.external_result.submission_version'), hashtext($1 || chr(31) || $2))`,
		tenant.String(), version.String(),
	); err != nil {
		return nil, fmt.Errorf("load external results: 取提交版本锁：%w", err)
	}

	rows, err := executor.Query(ctx,
		`SELECT e.source_id, e.result, r.kind, r.authority_ref, r.scope_ref, r.condition, r.received_at
		   FROM customs_compliance.external_result e
		   LEFT JOIN customs_compliance.release_outcome r
		     ON r.tenant_id = e.tenant_id AND r.source_id = e.source_id
		  WHERE e.tenant_id = $1
		    AND e.submission_version = $2
		  ORDER BY e.inserted_at, e.source_id`,
		tenant.String(),
		version.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("load external results: %w", err)
	}
	defer rows.Close()

	var facts []domain.LayerFact
	for rows.Next() {
		var (
			sourceID                          string
			resultJSON                        []byte
			kind, authority, scope, condition *string
			releaseReceivedAt                 *time.Time
		)
		if err := rows.Scan(&sourceID, &resultJSON, &kind, &authority, &scope, &condition, &releaseReceivedAt); err != nil {
			return nil, fmt.Errorf("load external results: %w", err)
		}
		result, err := rebuildResult(resultJSON)
		if err != nil {
			return nil, fmt.Errorf("load external results: %s: %w: %w", sourceID, ports.ErrLayerFactInconsistent, err)
		}
		fact := domain.LayerFact{Result: result}
		if kind != nil {
			if authority == nil || scope == nil || condition == nil || releaseReceivedAt == nil {
				return nil, fmt.Errorf("load external results: %s: %w: release outcome row is incomplete",
					sourceID, ports.ErrLayerFactInconsistent)
			}
			release, err := rebuildReleaseOutcome(*kind, *authority, *scope, *condition, *releaseReceivedAt)
			if err != nil {
				return nil, fmt.Errorf("load external results: %s: %w: release outcome: %w",
					sourceID, ports.ErrLayerFactInconsistent, err)
			}
			fact.Release = &release
		}
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load external results: %w", err)
	}
	return facts, nil
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
