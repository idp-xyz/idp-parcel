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

// AcceptedFacts 实现 ports.AcceptedFactStore（写入代数同 ADR-0031）。事实只增不删：
// 来源更正是新版本新键，适配器没有 UPDATE 与 DELETE 语句。
type AcceptedFacts struct {
	db *bentopg.DB
}

func NewAcceptedFacts(db *bentopg.DB) (*AcceptedFacts, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &AcceptedFacts{db: db}, nil
}

// FindByKey 按幂等键（来源上下文+事实引用+来源版本）取回事实。同键异内容是来源冲突
// 不是重放——冲突判定在应用层拿内容指纹比，这里只如实交回。
func (repository *AcceptedFacts) FindByKey(
	ctx context.Context,
	key ports.FactKey,
) (ports.FactRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.FactRecord{}, false, fmt.Errorf("find accepted fact: %w", err)
	}

	var (
		parcel, digest                      string
		occurredAt, effectiveAt, receivedAt time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT parcel_ref, content_digest, occurred_at, effective_at, received_at
		   FROM visibility_exception.accepted_fact
		  WHERE source_context = $1 AND fact_ref = $2 AND fact_version = $3`,
		key.Source.String(), key.Fact.String(), key.Version.String(),
	).Scan(&parcel, &digest, &occurredAt, &effectiveAt, &receivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.FactRecord{}, false, nil
	}
	if err != nil {
		return ports.FactRecord{}, false, fmt.Errorf("find accepted fact: %w", err)
	}

	record, err := factRecordFromRow(
		key.Source.String(), key.Fact.String(), key.Version.String(),
		parcel, digest, occurredAt, effectiveAt, receivedAt)
	if err != nil {
		return ports.FactRecord{}, false, err
	}
	return record, true, nil
}

// FindByParcel 交回该包裹全部已接受事实——投影派生的输入。接收序为主排序：投影
// 消费的是「知道了什么」，同刻到达再按键序保证读回次序确定。
func (repository *AcceptedFacts) FindByParcel(
	ctx context.Context,
	parcel domain.TrackedParcelReference,
) ([]ports.FactRecord, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("find facts by parcel: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT source_context, fact_ref, fact_version, parcel_ref, content_digest,
		        occurred_at, effective_at, received_at
		   FROM visibility_exception.accepted_fact
		  WHERE parcel_ref = $1
		  ORDER BY received_at, source_context, fact_ref, fact_version`,
		parcel.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("find facts by parcel: %w", err)
	}
	defer rows.Close()

	var records []ports.FactRecord
	for rows.Next() {
		var (
			source, factRef, factVersion, parcelRef, digest string
			occurredAt, effectiveAt, receivedAt             time.Time
		)
		if err := rows.Scan(&source, &factRef, &factVersion, &parcelRef, &digest,
			&occurredAt, &effectiveAt, &receivedAt); err != nil {
			return nil, fmt.Errorf("find facts by parcel: %w", err)
		}
		record, err := factRecordFromRow(
			source, factRef, factVersion, parcelRef, digest, occurredAt, effectiveAt, receivedAt)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find facts by parcel: %w", err)
	}
	return records, nil
}

// Save 写下一份已接受事实。同键已有记录时答`已有记录`——业务答案不是错误
// （ADR-0031）；ON CONFLICT DO NOTHING 保事务可用。
func (repository *AcceptedFacts) Save(
	ctx context.Context,
	record ports.FactRecord,
) (ports.FactSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.FactSaveOutcomeInvalid, fmt.Errorf("save accepted fact: %w", err)
	}

	fact := record.Fact
	if record.Key.Source != fact.Source() ||
		record.Key.Fact != fact.Fact() ||
		record.Key.Version != fact.Version() {
		return ports.FactSaveOutcomeInvalid,
			fmt.Errorf("save accepted fact: key disagrees with the fact it claims to index")
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.accepted_fact
			(source_context, fact_ref, fact_version, parcel_ref, content_digest,
			 occurred_at, effective_at, received_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT DO NOTHING`,
		fact.Source().String(),
		fact.Fact().String(),
		fact.Version().String(),
		fact.Parcel().String(),
		record.ContentDigest,
		fact.OccurredAt(),
		fact.EffectiveAt(),
		fact.ReceivedAt(),
	)
	if err != nil {
		return ports.FactSaveOutcomeInvalid, fmt.Errorf("save accepted fact: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.FactAlreadyRecorded, nil
	}
	return ports.FactSaved, nil
}

// sourceContextFrom 把列值译回封闭五值。迁移 CHECK 已拦住集合外取值，这里的 default
// 只在库被绕过迁移改写时才会走到。
func sourceContextFrom(value string) (domain.SourceContext, error) {
	switch value {
	case "PARCEL_SHIPMENT":
		return domain.SourceParcelShipment, nil
	case "NETWORK_ROUTING":
		return domain.SourceNetworkRouting, nil
	case "NODE_OPERATIONS":
		return domain.SourceNodeOperations, nil
	case "TRANSPORT_FULFILLMENT":
		return domain.SourceTransportFulfillment, nil
	case "CUSTOMS_COMPLIANCE":
		return domain.SourceCustomsCompliance, nil
	default:
		return domain.SourceContextInvalid,
			fmt.Errorf("visibility exception postgres: unknown source context %q", value)
	}
}

// factRecordFromRow 把一行译回端口记录，事实本体经领域构造函数重建重验。
func factRecordFromRow(
	source, factRef, factVersion, parcelRef, digest string,
	occurredAt, effectiveAt, receivedAt time.Time,
) (ports.FactRecord, error) {
	sourceContext, err := sourceContextFrom(source)
	if err != nil {
		return ports.FactRecord{}, err
	}
	fact, err := domain.NewSourceFactReference(factRef)
	if err != nil {
		return ports.FactRecord{}, fmt.Errorf("rebuild accepted fact: %w", err)
	}
	version, err := domain.NewSourceFactVersion(factVersion)
	if err != nil {
		return ports.FactRecord{}, fmt.Errorf("rebuild accepted fact: %w", err)
	}
	parcel, err := domain.NewTrackedParcelReference(parcelRef)
	if err != nil {
		return ports.FactRecord{}, fmt.Errorf("rebuild accepted fact: %w", err)
	}
	rebuilt, err := domain.NewAcceptedSourceFact(domain.AcceptedSourceFactSpec{
		Source:      sourceContext,
		Parcel:      parcel,
		Fact:        fact,
		Version:     version,
		OccurredAt:  occurredAt,
		EffectiveAt: effectiveAt,
		ReceivedAt:  receivedAt,
	})
	if err != nil {
		return ports.FactRecord{}, fmt.Errorf("rebuild accepted fact: %w", err)
	}
	return ports.FactRecord{
		Key:           ports.FactKey{Source: sourceContext, Fact: fact, Version: version},
		ContentDigest: digest,
		Fact:          rebuilt,
	}, nil
}
