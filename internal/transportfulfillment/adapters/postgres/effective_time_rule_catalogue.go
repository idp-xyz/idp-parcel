package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// EffectiveTimeRuleCatalogue 实现 ports.EffectiveTimeRuleRegistry 与 ports.EffectiveTimeRules：
// 规则一个版本一行，只插不改。
//
// 一个类型担两个端口是因为规则口读的就是这本目录——「按该源已登记并带版本的规则形成有效时间」是对
// 登记内容的一次问答，不是另一份数据；装配处仍按端口各自注入。有了这个实现之后，收编执行器拿到的
// `无规则`只剩一种来路：该源在目录里此刻没有任何版本。
type EffectiveTimeRuleCatalogue struct {
	db *bentopg.DB
}

func NewEffectiveTimeRuleCatalogue(db *bentopg.DB) (*EffectiveTimeRuleCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &EffectiveTimeRuleCatalogue{db: db}, nil
}

var (
	_ ports.EffectiveTimeRuleRegistry = (*EffectiveTimeRuleCatalogue)(nil)
	_ ports.EffectiveTimeRules        = (*EffectiveTimeRuleCatalogue)(nil)
)

const effectiveTimeRuleColumns = `tenant_id, source_ref, version, source_time_meaning, anchor, offset_seconds,
		        supersedes_version, recorded_at`

// FindByKey 按（租户+源+版本）取回一个版本。否定结果只回 false。
func (catalogue *EffectiveTimeRuleCatalogue) FindByKey(
	ctx context.Context,
	key ports.EffectiveTimeRuleKey,
) (ports.EffectiveTimeRuleRecord, bool, error) {
	records, err := catalogue.query(ctx, "find effective time rule",
		`SELECT `+effectiveTimeRuleColumns+`
		   FROM transport_fulfillment.effective_time_rule
		  WHERE tenant_id = $1 AND source_ref = $2 AND version = $3`,
		key.TenantID.String(), key.Source.String(), key.Version.String())
	if err != nil || len(records) == 0 {
		return ports.EffectiveTimeRuleRecord{}, false, err
	}
	return records[0], true, nil
}

// FindCurrent 取回某源此刻未被任何版本回指的那一版。「当前」是派生问答不是可变标记：表上没有
// current 列，改一版就得回写的东西这里一个都没有。
func (catalogue *EffectiveTimeRuleCatalogue) FindCurrent(
	ctx context.Context,
	tenant domain.TenantID,
	source domain.TrackingSourceReference,
) (ports.EffectiveTimeRuleRecord, bool, error) {
	records, err := catalogue.query(ctx, "find current effective time rule",
		`SELECT `+effectiveTimeRuleColumns+`
		   FROM transport_fulfillment.effective_time_rule AS current
		  WHERE tenant_id = $1 AND source_ref = $2
		    AND NOT EXISTS (
		        SELECT 1 FROM transport_fulfillment.effective_time_rule AS successor
		         WHERE successor.tenant_id = current.tenant_id
		           AND successor.source_ref = current.source_ref
		           AND successor.supersedes_version = current.version)
		  ORDER BY recorded_at DESC
		  LIMIT 1`,
		tenant.String(), source.String())
	if err != nil || len(records) == 0 {
		return ports.EffectiveTimeRuleRecord{}, false, err
	}
	return records[0], true, nil
}

// ListVersions 按登记先后交回某源的全部版本；同一时刻落的再按版本号排，让顺序可复现。
func (catalogue *EffectiveTimeRuleCatalogue) ListVersions(
	ctx context.Context,
	tenant domain.TenantID,
	source domain.TrackingSourceReference,
) ([]ports.EffectiveTimeRuleRecord, error) {
	return catalogue.query(ctx, "list effective time rule versions",
		`SELECT `+effectiveTimeRuleColumns+`
		   FROM transport_fulfillment.effective_time_rule
		  WHERE tenant_id = $1 AND source_ref = $2
		  ORDER BY recorded_at, version`,
		tenant.String(), source.String())
}

// JudgeEffectiveTime 答该源此刻有没有可用规则、有则按当前版形成有效时间。**没有版本答 RULE_ABSENT**，
// 不拿发生时间顶上（ADR-0102 Alternatives 第二条否决的正是它）；输入里的状态词不参与——规则不解释状态词。
// 目录读不回来是 error 不是`无`：执行器据以答`未决`，而`无`会让事实静默留在待判断。
func (catalogue *EffectiveTimeRuleCatalogue) JudgeEffectiveTime(
	ctx context.Context,
	input ports.EffectiveTimeRuleInput,
) (ports.EffectiveTimeRuling, error) {
	current, found, err := catalogue.FindCurrent(ctx, input.TenantID, input.Source)
	if err != nil {
		return ports.EffectiveTimeRuling{}, fmt.Errorf("judge effective time by rule: %w", err)
	}
	if !found {
		return ports.EffectiveTimeRuling{Outcome: ports.EffectiveTimeRuleAbsent}, nil
	}
	judgment, err := current.Rule.EffectiveTimeFor(input.OccurredAt, input.ReceivedAt)
	if err != nil {
		return ports.EffectiveTimeRuling{}, fmt.Errorf("judge effective time by rule: %w", err)
	}
	rule, _ := judgment.Rule()
	at, _ := judgment.EffectiveAt()
	return ports.EffectiveTimeRuling{Outcome: ports.EffectiveTimeRuleApplied, Rule: rule, EffectiveAt: at}, nil
}

func (catalogue *EffectiveTimeRuleCatalogue) query(
	ctx context.Context,
	verb string,
	sql string,
	args ...any,
) ([]ports.EffectiveTimeRuleRecord, error) {
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", verb, err)
	}
	rows, err := querier.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", verb, err)
	}
	defer rows.Close()

	var records []ports.EffectiveTimeRuleRecord
	for rows.Next() {
		record, err := scanEffectiveTimeRule(rows)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", verb, err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%s: %w", verb, err)
	}
	return records, nil
}

// scanEffectiveTimeRule 把一行装回一个版本。逐列走各自的构造门，再交给重建门核前版——不按列直接
// 拼结构体（ADR-0028）。偏移在库里是整秒：规则的偏移是所有者登记的业务量，没有亚秒的用法。
func scanEffectiveTimeRule(rows pgx.Rows) (ports.EffectiveTimeRuleRecord, error) {
	var tenantID, sourceRef, version, meaningText, anchorText string
	var offsetSeconds int64
	var supersedes *string
	var recordedAt time.Time
	if err := rows.Scan(&tenantID, &sourceRef, &version, &meaningText, &anchorText, &offsetSeconds, &supersedes, &recordedAt); err != nil {
		return ports.EffectiveTimeRuleRecord{}, err
	}

	spec := domain.RehydrateEffectiveTimeRuleSpec{Offset: time.Duration(offsetSeconds) * time.Second}
	var err error
	if spec.TenantID, err = domain.NewTenantID(tenantID); err != nil {
		return ports.EffectiveTimeRuleRecord{}, err
	}
	if spec.Source, err = domain.NewTrackingSourceReference(sourceRef); err != nil {
		return ports.EffectiveTimeRuleRecord{}, err
	}
	if spec.Version, err = domain.NewEffectiveTimeRuleVersion(version); err != nil {
		return ports.EffectiveTimeRuleRecord{}, err
	}
	if spec.SourceTimeMeaning, err = domain.ParseSourceTimeMeaning(meaningText); err != nil {
		return ports.EffectiveTimeRuleRecord{}, err
	}
	if spec.Anchor, err = domain.ParseEffectiveTimeAnchor(anchorText); err != nil {
		return ports.EffectiveTimeRuleRecord{}, err
	}
	if supersedes != nil {
		if spec.Supersedes, err = domain.NewEffectiveTimeRuleVersion(*supersedes); err != nil {
			return ports.EffectiveTimeRuleRecord{}, err
		}
	}

	rule, err := domain.RehydrateEffectiveTimeRule(spec)
	if err != nil {
		return ports.EffectiveTimeRuleRecord{}, err
	}
	return ports.EffectiveTimeRuleRecord{
		Key:        ports.EffectiveTimeRuleKey{TenantID: spec.TenantID, Source: spec.Source, Version: spec.Version},
		Rule:       rule,
		RecordedAt: recordedAt.UTC(),
	}, nil
}

// Save 登记一个版本。撞键答`已登记`（ADR-0031）：撞的可能是主键，也可能是「一源一首版」「一版一后继」
// 两道部分唯一索引之一——三者对编排都是「另一方先落了」，读回赢家即可；读不回自己那一版就是`未决`。
func (catalogue *EffectiveTimeRuleCatalogue) Save(
	ctx context.Context,
	record ports.EffectiveTimeRuleRecord,
) (ports.EffectiveTimeRuleSaveOutcome, error) {
	executor, err := catalogue.db.RequireExecutor(ctx)
	if err != nil {
		return ports.EffectiveTimeRuleSaveOutcomeInvalid, fmt.Errorf("save effective time rule: %w", err)
	}
	if err := assertEffectiveTimeRuleKeyAgrees(record); err != nil {
		return ports.EffectiveTimeRuleSaveOutcomeInvalid, fmt.Errorf("save effective time rule: %w", err)
	}

	rule := record.Rule
	var supersedes *string
	if prior, has := rule.Supersedes(); has {
		text := prior.String()
		supersedes = &text
	}
	content := rule.Content()

	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.effective_time_rule
		     (`+effectiveTimeRuleColumns+`)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Source.String(),
		record.Key.Version.String(),
		content.SourceTimeMeaning.String(),
		content.Anchor.String(),
		int64(content.Offset/time.Second),
		supersedes,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.EffectiveTimeRuleSaveOutcomeInvalid, fmt.Errorf("save effective time rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.EffectiveTimeRuleAlreadyRegistered, nil
	}
	return ports.EffectiveTimeRuleSaved, nil
}

// assertEffectiveTimeRuleKeyAgrees 挡住「键说的是一个版本、聚合说的是另一个」那种写入。
func assertEffectiveTimeRuleKeyAgrees(record ports.EffectiveTimeRuleRecord) error {
	if record.Key.TenantID != record.Rule.TenantID() ||
		record.Key.Source != record.Rule.Source() ||
		record.Key.Version != record.Rule.Version() {
		return fmt.Errorf("effective time rule key disagrees with the aggregate")
	}
	return nil
}
