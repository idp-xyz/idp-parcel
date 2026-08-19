// Package postgres 是 transport-fulfillment 自有语义端口的 PostgreSQL 适配器。
//
// 显式 SQL、行模型与冲突翻译都留在这里，不进领域对象。所有语句显式携带租户条件：
// 作用域不是过滤器而是身份的一部分（与其他上下文的适配器同一条纪律）。
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

// EffectiveDeliveries 实现 ports.EffectiveDeliveryStore：一行一版本的交付登记。
//
// 「当前版」由 is_current 部分唯一索引承担：FindByKey 只读当前行；Save 插首登行，
// 撞当前唯一即译已登记（ON CONFLICT 代数，事务保持可用）；Supersede 在同一事务里
// 翻旧行再插新行——更正是新键新行指回前版，历史行只增不删。
//
// 历史行只增不删这一条正是 FindByKeyAndVersion 成立的前提：要哪一代给哪一代，问「谁
// 是当前版」与问「v1 长什么样」是两个问题，读口因而分两个。
type EffectiveDeliveries struct {
	db *bentopg.DB
}

func NewEffectiveDeliveries(db *bentopg.DB) (*EffectiveDeliveries, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &EffectiveDeliveries{db: db}, nil
}

// FindByKey 按（租户+对象+尝试）取回当前版。否定结果只回 false，不区分「不存在」
// 与「属于另一个租户」。
func (repository *EffectiveDeliveries) FindByKey(
	ctx context.Context,
	key ports.EffectiveDeliveryKey,
) (ports.EffectiveDeliveryRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.EffectiveDeliveryRecord{}, false, fmt.Errorf("find effective delivery: %w", err)
	}

	var version, place, method, recipient, proof, digest string
	var corrects *string
	var correctedAt *time.Time
	var occurredAt, recordedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT delivery_version, place_ref, method_ref, recipient_ref, proof_ref,
		        corrects_version, corrected_at, occurred_at, content_digest, recorded_at
		   FROM transport_fulfillment.effective_delivery
		  WHERE tenant_id = $1
		    AND object_ref = $2
		    AND attempt_ref = $3
		    AND is_current`,
		key.TenantID.String(),
		key.Object.String(),
		key.Attempt.String(),
	).Scan(&version, &place, &method, &recipient, &proof,
		&corrects, &correctedAt, &occurredAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.EffectiveDeliveryRecord{}, false, nil
	}
	if err != nil {
		return ports.EffectiveDeliveryRecord{}, false, fmt.Errorf("find effective delivery: %w", err)
	}

	delivery, err := rehydrateDelivery(key, version, place, method, recipient, proof, corrects, correctedAt, occurredAt)
	if err != nil {
		return ports.EffectiveDeliveryRecord{}, false, fmt.Errorf("find effective delivery: %w", err)
	}
	return ports.EffectiveDeliveryRecord{
		Key:           key,
		ContentDigest: digest,
		Delivery:      delivery,
		RecordedAt:    recordedAt,
	}, true, nil
}

// FindByKeyAndVersion 按（租户+对象+尝试+结果版本）取回指名的那一代。它与 FindByKey
// 的差别只在最后一维：这里不问 is_current，因此被更正翻成历史的旧行照样读得回。
func (repository *EffectiveDeliveries) FindByKeyAndVersion(
	ctx context.Context,
	key ports.EffectiveDeliveryKey,
	version domain.DeliveryResultVersion,
) (ports.EffectiveDeliveryRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.EffectiveDeliveryRecord{}, false, fmt.Errorf("find effective delivery version: %w", err)
	}

	var place, method, recipient, proof, digest string
	var corrects *string
	var correctedAt *time.Time
	var occurredAt, recordedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT place_ref, method_ref, recipient_ref, proof_ref,
		        corrects_version, corrected_at, occurred_at, content_digest, recorded_at
		   FROM transport_fulfillment.effective_delivery
		  WHERE tenant_id = $1
		    AND object_ref = $2
		    AND attempt_ref = $3
		    AND delivery_version = $4`,
		key.TenantID.String(),
		key.Object.String(),
		key.Attempt.String(),
		version.String(),
	).Scan(&place, &method, &recipient, &proof,
		&corrects, &correctedAt, &occurredAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.EffectiveDeliveryRecord{}, false, nil
	}
	if err != nil {
		return ports.EffectiveDeliveryRecord{}, false, fmt.Errorf("find effective delivery version: %w", err)
	}

	delivery, err := rehydrateDelivery(
		key, version.String(), place, method, recipient, proof, corrects, correctedAt, occurredAt)
	if err != nil {
		return ports.EffectiveDeliveryRecord{}, false, fmt.Errorf("find effective delivery version: %w", err)
	}
	return ports.EffectiveDeliveryRecord{
		Key:           key,
		ContentDigest: digest,
		Delivery:      delivery,
		RecordedAt:    recordedAt,
	}, true, nil
}

// Save 落首登行。撞「当前版唯一」译已登记——业务答案不是 error（ADR-0031），编排
// 拿到它还要在同一个事务里读回原版本作答，所以用 ON CONFLICT DO NOTHING 保事务可用。
func (repository *EffectiveDeliveries) Save(
	ctx context.Context,
	record ports.EffectiveDeliveryRecord,
) (ports.DeliverySaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.DeliverySaveOutcomeInvalid, fmt.Errorf("save effective delivery: %w", err)
	}

	tag, err := executor.Exec(ctx,
		insertDeliverySQL+` ON CONFLICT DO NOTHING`,
		deliveryArguments(record)...,
	)
	if err != nil {
		return ports.DeliverySaveOutcomeInvalid, fmt.Errorf("save effective delivery: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.DeliveryAlreadyRegistered, nil
	}
	return ports.DeliverySaved, nil
}

// Supersede 落更正版本：同一事务里把当前行翻成历史，再插指回前版的新行。found=false
// 表示没有可更正的登记——更正不出无中生有的交付。
func (repository *EffectiveDeliveries) Supersede(
	ctx context.Context,
	record ports.EffectiveDeliveryRecord,
) (bool, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("supersede effective delivery: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`UPDATE transport_fulfillment.effective_delivery
		    SET is_current = false
		  WHERE tenant_id = $1
		    AND object_ref = $2
		    AND attempt_ref = $3
		    AND is_current`,
		record.Key.TenantID.String(),
		record.Key.Object.String(),
		record.Key.Attempt.String(),
	)
	if err != nil {
		return false, fmt.Errorf("supersede effective delivery: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}

	if _, err := executor.Exec(ctx, insertDeliverySQL, deliveryArguments(record)...); err != nil {
		return false, fmt.Errorf("supersede effective delivery: %w", err)
	}
	return true, nil
}

// insertDeliverySQL 是首登与更正共用的插入语句：两条路都插「一行一版本」的新行，
// 差别只在更正行带前版引用（由记录本体携带）。
const insertDeliverySQL = `INSERT INTO transport_fulfillment.effective_delivery
	(tenant_id, object_ref, attempt_ref, delivery_version,
	 place_ref, method_ref, recipient_ref, proof_ref,
	 corrects_version, corrected_at, occurred_at, content_digest, is_current)
 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, true)`

func deliveryArguments(record ports.EffectiveDeliveryRecord) []any {
	delivery := record.Delivery
	var corrects *string
	var correctedAt *time.Time
	if predecessor, corrected := delivery.Corrects(); corrected {
		value := predecessor.String()
		corrects = &value
		if at, present := delivery.CorrectedAt(); present {
			utc := at.UTC()
			correctedAt = &utc
		}
	}
	return []any{
		record.Key.TenantID.String(),
		record.Key.Object.String(),
		record.Key.Attempt.String(),
		delivery.Version().String(),
		delivery.Place().String(),
		delivery.Method().String(),
		delivery.Recipient().String(),
		delivery.Proof().String(),
		corrects,
		correctedAt,
		delivery.OccurredAt().UTC(),
		record.ContentDigest,
	}
}

func rehydrateDelivery(
	key ports.EffectiveDeliveryKey,
	version, place, method, recipient, proof string,
	corrects *string,
	correctedAt *time.Time,
	occurredAt time.Time,
) (domain.EffectiveDelivery, error) {
	spec := domain.RehydrateEffectiveDeliverySpec{
		TenantID:   key.TenantID,
		Object:     key.Object,
		Attempt:    key.Attempt,
		OccurredAt: occurredAt,
	}
	var err error
	if spec.Place, err = domain.NewAttemptPlaceReference(place); err != nil {
		return domain.EffectiveDelivery{}, err
	}
	if spec.Method, err = domain.NewDeliveryMethodReference(method); err != nil {
		return domain.EffectiveDelivery{}, err
	}
	if spec.Recipient, err = domain.NewReceivingPartyReference(recipient); err != nil {
		return domain.EffectiveDelivery{}, err
	}
	if spec.Proof, err = domain.NewDeliveryProofReference(proof); err != nil {
		return domain.EffectiveDelivery{}, err
	}
	if spec.Version, err = domain.NewDeliveryResultVersion(version); err != nil {
		return domain.EffectiveDelivery{}, err
	}
	if corrects != nil {
		if spec.Corrects, err = domain.NewDeliveryResultVersion(*corrects); err != nil {
			return domain.EffectiveDelivery{}, err
		}
	}
	if correctedAt != nil {
		spec.CorrectedAt = *correctedAt
	}
	return domain.RehydrateEffectiveDelivery(spec)
}
