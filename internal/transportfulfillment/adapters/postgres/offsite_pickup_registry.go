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

// OffsitePickupRegistrations 实现 ports.OffsitePickupRegistry：对象级揽收登记，一次
// 到场对一个对象一行。
//
// 它与 pickup_attempt_pickup（尝试提交那条链上的揽收）不是同一张表：那张按来源身份
// 幂等、随整份尝试一起落，这张按（对象+尝试）幂等、由登记入口单独落。两条链各有自己
// 的幂等键，压进一张表就得二选一，另一条的重放判定必然出错。
type OffsitePickupRegistrations struct {
	db *bentopg.DB
}

func NewOffsitePickupRegistrations(db *bentopg.DB) (*OffsitePickupRegistrations, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &OffsitePickupRegistrations{db: db}, nil
}

var _ ports.OffsitePickupRegistry = (*OffsitePickupRegistrations)(nil)

// FindByKey 按（租户+对象+尝试）取回已登揽收。否定结果只回 false，不区分「不存在」
// 与「属于另一个租户」。读回经 FormOffsitePickup 复验——控制依据缺席的一行在这里
// 暴露，而不是变成一份看起来合法的有效收寄。
func (repository *OffsitePickupRegistrations) FindByKey(
	ctx context.Context,
	key ports.OffsitePickupKey,
) (ports.OffsitePickupRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.OffsitePickupRecord{}, false, fmt.Errorf("find offsite pickup: %w", err)
	}

	var task, place, control, executedBy, version, digest string
	var occurredAt, recordedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT task_ref, place_ref, control_ref, executed_by,
		        pickup_version, occurred_at, content_digest, recorded_at
		   FROM transport_fulfillment.offsite_pickup
		  WHERE tenant_id = $1
		    AND object_ref = $2
		    AND attempt_ref = $3`,
		key.TenantID.String(),
		key.Object.String(),
		key.Attempt.String(),
	).Scan(&task, &place, &control, &executedBy, &version, &occurredAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.OffsitePickupRecord{}, false, nil
	}
	if err != nil {
		return ports.OffsitePickupRecord{}, false, fmt.Errorf("find offsite pickup: %w", err)
	}

	pickup, err := rebuildRegisteredPickup(key, task, place, control, executedBy, version, occurredAt)
	if err != nil {
		return ports.OffsitePickupRecord{}, false, fmt.Errorf("find offsite pickup: %w", err)
	}
	return ports.OffsitePickupRecord{
		Key:           key,
		ContentDigest: digest,
		Pickup:        pickup,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 落一份揽收登记。撞键答`已登记`——业务答案不是 error（ADR-0031），编排拿到它
// 还要在同一事务里读回赢家作答，所以用 ON CONFLICT DO NOTHING 保事务可用。
func (repository *OffsitePickupRegistrations) Save(
	ctx context.Context,
	record ports.OffsitePickupRecord,
) (ports.OffsitePickupSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.OffsitePickupSaveOutcomeInvalid, fmt.Errorf("save offsite pickup: %w", err)
	}
	if err := assertPickupKeyAgrees(record); err != nil {
		return ports.OffsitePickupSaveOutcomeInvalid, fmt.Errorf("save offsite pickup: %w", err)
	}

	pickup := record.Pickup
	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.offsite_pickup
			(tenant_id, object_ref, attempt_ref, task_ref, place_ref, control_ref,
			 executed_by, pickup_version, occurred_at, content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Object.String(),
		record.Key.Attempt.String(),
		pickup.Task().String(),
		pickup.Place().String(),
		pickup.Control().String(),
		pickup.ExecutedBy().String(),
		pickup.Version().String(),
		pickup.OccurredAt().UTC(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.OffsitePickupSaveOutcomeInvalid, fmt.Errorf("save offsite pickup: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.OffsitePickupAlreadyRegistered, nil
	}
	return ports.OffsitePickupSaved, nil
}

// assertPickupKeyAgrees 挡住「记录键与揽收本体的键各说各话」这种装配错误：键由平铺列
// 独家拥有，本体的对应字段不再入库，两者不一致时写下去就再也查不出原本该是哪一个。
func assertPickupKeyAgrees(record ports.OffsitePickupRecord) error {
	pickup := record.Pickup
	if pickup.TenantID() != record.Key.TenantID ||
		pickup.Object() != record.Key.Object ||
		pickup.Attempt() != record.Key.Attempt {
		return errors.New("record key disagrees with the pickup's own identity")
	}
	return nil
}

func rebuildRegisteredPickup(
	key ports.OffsitePickupKey,
	task, place, control, executedBy, version string,
	occurredAt time.Time,
) (domain.OffsitePickup, error) {
	spec := domain.OffsitePickupSpec{
		TenantID:   key.TenantID,
		Object:     key.Object,
		Attempt:    key.Attempt,
		OccurredAt: occurredAt,
	}
	var err error
	if spec.Task, err = domain.NewPickupTaskReference(task); err != nil {
		return domain.OffsitePickup{}, err
	}
	if spec.Place, err = domain.NewPickupPlaceReference(place); err != nil {
		return domain.OffsitePickup{}, err
	}
	if spec.Control, err = domain.NewTransportControlReference(control); err != nil {
		return domain.OffsitePickup{}, err
	}
	if spec.ExecutedBy, err = domain.NewExecutingPartyReference(executedBy); err != nil {
		return domain.OffsitePickup{}, err
	}
	if spec.Version, err = domain.NewPickupResultVersion(version); err != nil {
		return domain.OffsitePickup{}, err
	}
	return domain.FormOffsitePickup(spec)
}
