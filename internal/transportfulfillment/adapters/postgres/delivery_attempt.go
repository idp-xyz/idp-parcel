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

// DeliveryAttempts 实现 ports.DeliveryAttemptView：按（尝试、对象）取回派送尝试与
// 该对象的结果，供交付生效引用。
//
// 它只读不写。派送尝试由执行侧登记（与揽收侧的 PickupAttempts 对称），交付生效登记
// 引用它已经发生的事实——一个入口同时造尝试和造交付，就没有东西拦得住「先声称到过场
// 再声称交付成功」。
type DeliveryAttempts struct {
	db *bentopg.DB
}

func NewDeliveryAttempts(db *bentopg.DB) (*DeliveryAttempts, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &DeliveryAttempts{db: db}, nil
}

var _ ports.DeliveryAttemptView = (*DeliveryAttempts)(nil)

// LoadDeliveryResult 一次连表取回尝试与对象结果。
//
// 尝试与结果一起取而不分两次：分两次时中间那个瞬间足以让「尝试在、结果不在」变成
// 「两者都在」，而这个 found 正是编排用来分「指错」与「等谁」的——指名的尝试或对象
// 结果不存在是提交矛盾（DeliveryNotAccepted），读不到库才是未决。两者混一格，一次
// 库抖动就会被当成来源写错了单。
func (view *DeliveryAttempts) LoadDeliveryResult(
	ctx context.Context,
	tenant domain.TenantID,
	attempt domain.AttemptReference,
	object domain.CarriedObjectReference,
) (domain.FulfillmentAttempt, domain.DeliveryAttemptResult, bool, error) {
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false,
			fmt.Errorf("load delivery result: %w", err)
	}

	var taskRef, executedBy, placeRef, evidence, outcomeName string
	var rescheduledFrom, basis *string
	var objectsJSON []byte
	var plannedFrom, plannedTo, arrivedAt, occurredAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT a.task_ref, a.executed_by, a.place_ref,
		        a.planned_from, a.planned_to, a.arrived_at, a.evidence_ref,
		        a.rescheduled_from, a.objects,
		        r.outcome, r.basis, r.occurred_at
		   FROM transport_fulfillment.delivery_attempt AS a
		   JOIN transport_fulfillment.delivery_attempt_result AS r
		     ON r.tenant_id = a.tenant_id
		    AND r.attempt_ref = a.attempt_ref
		  WHERE a.tenant_id = $1
		    AND a.attempt_ref = $2
		    AND r.object_ref = $3`,
		tenant.String(),
		attempt.String(),
		object.String(),
	).Scan(&taskRef, &executedBy, &placeRef,
		&plannedFrom, &plannedTo, &arrivedAt, &evidence,
		&rescheduledFrom, &objectsJSON,
		&outcomeName, &basis, &occurredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false, nil
	}
	if err != nil {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false,
			fmt.Errorf("load delivery result: %w", err)
	}

	// 复用揽收侧的尝试重建：同一个 FulfillmentAttempt 领域对象，两处各写一份重建会
	// 让同一格事实长出两个样子。
	fulfillmentAttempt, err := rebuildAttempt(
		tenant, attempt.String(), taskRef, executedBy, placeRef, evidence,
		rescheduledFrom, objectsJSON, plannedFrom, plannedTo, arrivedAt)
	if err != nil {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false,
			fmt.Errorf("load delivery result: %w", err)
	}

	result, err := rebuildDeliveryResult(fulfillmentAttempt, object, outcomeName, basis, occurredAt)
	if err != nil {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false,
			fmt.Errorf("load delivery result: %w", err)
	}
	return fulfillmentAttempt, result, true, nil
}

// rebuildDeliveryResult 经 FormDeliveryAttemptResult 复验：对象在尝试范围内、失败带
// 依据、妥投不带依据、结果不早于到场。库内 CHECK 守得住前后两条，「对象在范围内」与
// 时序跨表守不了，靠这道门。
func rebuildDeliveryResult(
	attempt domain.FulfillmentAttempt,
	object domain.CarriedObjectReference,
	outcomeName string,
	basis *string,
	occurredAt time.Time,
) (domain.DeliveryAttemptResult, error) {
	outcome, err := deliveryOutcomeFrom(outcomeName)
	if err != nil {
		return domain.DeliveryAttemptResult{}, err
	}
	var basisRef domain.AttemptResultBasisReference
	if basis != nil {
		if basisRef, err = domain.NewAttemptResultBasisReference(*basis); err != nil {
			return domain.DeliveryAttemptResult{}, err
		}
	}
	return domain.FormDeliveryAttemptResult(attempt, object, outcome, basisRef, occurredAt)
}

func deliveryOutcomeFrom(raw string) (domain.DeliveryObjectOutcome, error) {
	switch raw {
	case domain.ObjectDelivered.String():
		return domain.ObjectDelivered, nil
	case domain.DeliveryRefused.String():
		return domain.DeliveryRefused, nil
	case domain.NoOneToReceive.String():
		return domain.NoOneToReceive, nil
	case domain.WrongAddress.String():
		return domain.WrongAddress, nil
	default:
		return 0, fmt.Errorf("unknown delivery object outcome %q", raw)
	}
}
