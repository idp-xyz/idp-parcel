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
// 到场对一个对象一条版本链——首登一行，此后每次更正一行回指前版（0015），原行不动。
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

// FindByKey 按（租户+对象+尝试）取回这次揽收的**当前版**。否定结果只回 false，不区分「不存在」
// 与「属于另一个租户」。读回经 RehydrateOffsitePickup 复验——控制依据缺席的一行在这里
// 暴露，而不是变成一份看起来合法的有效收寄。
//
// 「当前版」按回指派生：同键下没有任何行回指它的那一版（0015：更正是新行新版本，表上没有
// current 列）。部分唯一索引 offsite_pickup_one_first_registration 与 offsite_pickup_corrects_once
// 让链严格线性，所以这条子查询恰答一行——首登还没被更正时就是首登本身，与 0005 时期的读法同义。
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
	var corrects *string
	var correctedAt *time.Time
	err = querier.QueryRow(ctx,
		`SELECT current.task_ref, current.place_ref, current.control_ref, current.executed_by,
		        current.pickup_version, current.occurred_at, current.content_digest, current.recorded_at,
		        current.corrects_version, current.corrected_at
		   FROM transport_fulfillment.offsite_pickup AS current
		  WHERE current.tenant_id = $1
		    AND current.object_ref = $2
		    AND current.attempt_ref = $3
		    AND NOT EXISTS (
		        SELECT 1
		          FROM transport_fulfillment.offsite_pickup AS successor
		         WHERE successor.tenant_id = current.tenant_id
		           AND successor.object_ref = current.object_ref
		           AND successor.attempt_ref = current.attempt_ref
		           AND successor.corrects_version = current.pickup_version)`,
		key.TenantID.String(),
		key.Object.String(),
		key.Attempt.String(),
	).Scan(&task, &place, &control, &executedBy, &version, &occurredAt, &digest, &recordedAt, &corrects, &correctedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.OffsitePickupRecord{}, false, nil
	}
	if err != nil {
		return ports.OffsitePickupRecord{}, false, fmt.Errorf("find offsite pickup: %w", err)
	}

	pickup, err := rebuildRegisteredPickup(key, task, place, control, executedBy, version, occurredAt, corrects, correctedAt)
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

// FindByKeyAndVersion 按（租户+对象+尝试+结果版本）取回指名的那一代，不问它是不是当前版。
// 更正只加新行、原行不动（0015），所以任一代都读得回；读回同样经 RehydrateOffsitePickup，
// 回指与更正时刻随行——下游沿链判「回指接不接得到已采用版本」时不必再读一次。不存在的
// 版本与他租户都答 false 不报错，与 FindByKey 同纪律。
//
// 它不进 ports.OffsitePickupRegistry：拓宽那个接口会拆掉 TF 自己 http 与 application 两处的
// 替身，而这一口的唯一调用方在 parcel-shipment——消费方每份信封代表一代，按当前版读会让
// 更正之前入队的那一份也读成更正后那一代（票 label-channel/24，ADR-0117 决定四）。与
// EffectiveDeliveries.FindByKeyAndVersion 同形；PS 侧自己的消费方接口声明它，本类型结构满足。
func (repository *OffsitePickupRegistrations) FindByKeyAndVersion(
	ctx context.Context,
	key ports.OffsitePickupKey,
	version domain.PickupResultVersion,
) (ports.OffsitePickupRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.OffsitePickupRecord{}, false, fmt.Errorf("find offsite pickup by version: %w", err)
	}

	var task, place, control, executedBy, digest string
	var occurredAt, recordedAt time.Time
	var corrects *string
	var correctedAt *time.Time
	err = querier.QueryRow(ctx,
		`SELECT task_ref, place_ref, control_ref, executed_by, occurred_at, content_digest, recorded_at,
		        corrects_version, corrected_at
		   FROM transport_fulfillment.offsite_pickup
		  WHERE tenant_id = $1
		    AND object_ref = $2
		    AND attempt_ref = $3
		    AND pickup_version = $4`,
		key.TenantID.String(),
		key.Object.String(),
		key.Attempt.String(),
		version.String(),
	).Scan(&task, &place, &control, &executedBy, &occurredAt, &digest, &recordedAt, &corrects, &correctedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.OffsitePickupRecord{}, false, nil
	}
	if err != nil {
		return ports.OffsitePickupRecord{}, false, fmt.Errorf("find offsite pickup by version: %w", err)
	}

	pickup, err := rebuildRegisteredPickup(key, task, place, control, executedBy, version.String(), occurredAt, corrects, correctedAt)
	if err != nil {
		return ports.OffsitePickupRecord{}, false, fmt.Errorf("find offsite pickup by version: %w", err)
	}
	return ports.OffsitePickupRecord{
		Key:           key,
		ContentDigest: digest,
		Pickup:        pickup,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 落一份揽收登记——首登与更正版本都从这里进，一行一版，只插不改。撞键答`已登记`——业务
// 答案不是 error（ADR-0031），编排拿到它还要在同一事务里读回赢家作答，所以用 ON CONFLICT DO
// NOTHING 保事务可用。撞的可以是主键（同版本重放）、offsite_pickup_one_first_registration（同键
// 第二次首登）或 offsite_pickup_corrects_once（同一前版第二次更正），译法相同：读回当前版就是答案。
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
	var corrects *string
	var correctedAt *time.Time
	if predecessor, corrected := pickup.Corrects(); corrected {
		value := predecessor.String()
		corrects = &value
		at, _ := pickup.CorrectedAt()
		at = at.UTC()
		correctedAt = &at
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.offsite_pickup
			(tenant_id, object_ref, attempt_ref, task_ref, place_ref, control_ref,
			 executed_by, pickup_version, occurred_at, content_digest, recorded_at,
			 corrects_version, corrected_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
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
		corrects,
		correctedAt,
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

// rebuildRegisteredPickup 走重建门而不是构造门：版本链两字段写在领域的未导出字段上，只有
// RehydrateOffsitePickup 收得下——经 FormOffsitePickup 读回的更正版本会退化成首登。
func rebuildRegisteredPickup(
	key ports.OffsitePickupKey,
	task, place, control, executedBy, version string,
	occurredAt time.Time,
	corrects *string,
	correctedAt *time.Time,
) (domain.OffsitePickup, error) {
	spec := domain.RehydrateOffsitePickupSpec{
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
	if corrects != nil {
		if spec.Corrects, err = domain.NewPickupResultVersion(*corrects); err != nil {
			return domain.OffsitePickup{}, err
		}
	}
	if correctedAt != nil {
		spec.CorrectedAt = *correctedAt
	}
	return domain.RehydrateOffsitePickup(spec)
}
