package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ControlReleaseRefusalHandlings 实现 ports.ControlReleaseRefusalHandlings：释放未受理册的处理留痕（ADR-0293）。只追加，主键是册行的
// 主键，一笔占用至多被处理一次。
type ControlReleaseRefusalHandlings struct {
	db *bentopg.DB
}

func NewControlReleaseRefusalHandlings(db *bentopg.DB) (*ControlReleaseRefusalHandlings, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &ControlReleaseRefusalHandlings{db: db}, nil
}

var _ ports.ControlReleaseRefusalHandlings = (*ControlReleaseRefusalHandlings)(nil)

// HandleControlReleaseRefusal 追加一次处理。要环境事务：处理留痕与续办信封同笔落地。
//
// 一句 INSERT … SELECT 连册行一并核对：租户条件和册行存在性在同一句里，册上没有这一行（或属于另一个租户）时一行都插不进去，
// 与「已被处理」在 0 行受影响上相同，再读一次留痕分开两者。两人同时处理同一笔时，后到的那句在主键上等先到的事务结束：先到的提交，
// 后到的 DO NOTHING 并读回赢家；先到的回滚，后到的接着插。
func (handlings *ControlReleaseRefusalHandlings) HandleControlReleaseRefusal(
	ctx context.Context,
	handling ports.ControlReleaseRefusalHandling,
) (ports.ControlReleaseRefusalHandlingSaveOutcome, ports.ControlReleaseRefusalHandling, error) {
	executor, err := handlings.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ControlReleaseRefusalHandlingSaveOutcomeInvalid, ports.ControlReleaseRefusalHandling{},
			fmt.Errorf("handle control release refusal: %w", err)
	}
	if handling.Kind.String() == "" {
		return ports.ControlReleaseRefusalHandlingSaveOutcomeInvalid, ports.ControlReleaseRefusalHandling{},
			fmt.Errorf("handle control release refusal: handling kind is not a kind")
	}
	key := handling.Key
	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_shipment.control_release_refusal_handling
			(tenant_id, shipment_request_id, submission_version, control_result_id,
			 handling_kind, handled_by, handling_basis, handled_at)
		 SELECT r.tenant_id, r.shipment_request_id, r.submission_version, r.control_result_id,
		        $5, $6, $7, $8
		   FROM parcel_shipment.control_release_refusal r
		  WHERE r.tenant_id = $1 AND r.shipment_request_id = $2
		    AND r.submission_version = $3 AND r.control_result_id = $4
		 ON CONFLICT DO NOTHING`,
		key.TenantID.String(), key.ShipmentRequestID.String(), key.SubmissionVersion.String(), key.ControlResultID.String(),
		handling.Kind.String(), handling.Handler.String(), handling.Basis.String(), handling.HandledAt.UTC(),
	)
	if err != nil {
		return ports.ControlReleaseRefusalHandlingSaveOutcomeInvalid, ports.ControlReleaseRefusalHandling{},
			fmt.Errorf("handle control release refusal: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.ControlReleaseRefusalHandlingRecorded, handling, nil
	}
	winner, found, err := handlings.FindControlReleaseRefusalHandling(ctx, key)
	if err != nil {
		return ports.ControlReleaseRefusalHandlingSaveOutcomeInvalid, ports.ControlReleaseRefusalHandling{}, err
	}
	if !found {
		return ports.ControlReleaseRefusalNotRegistered, ports.ControlReleaseRefusalHandling{}, nil
	}
	return ports.ControlReleaseRefusalAlreadyHandled, winner, nil
}

// FindControlReleaseRefusalHandling 读一笔占用的处理留痕。租户条件进语句，否定结果不区分「没处理」与「属于另一个租户」。
func (handlings *ControlReleaseRefusalHandlings) FindControlReleaseRefusalHandling(
	ctx context.Context,
	key ports.ControlReleaseRefusalKey,
) (ports.ControlReleaseRefusalHandling, bool, error) {
	querier, err := handlings.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ControlReleaseRefusalHandling{}, false, fmt.Errorf("find control release refusal handling: %w", err)
	}
	var (
		kind, handler, basis string
		handledAt            time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT handling_kind, handled_by, handling_basis, handled_at
		   FROM parcel_shipment.control_release_refusal_handling
		  WHERE tenant_id = $1 AND shipment_request_id = $2
		    AND submission_version = $3 AND control_result_id = $4`,
		key.TenantID.String(), key.ShipmentRequestID.String(), key.SubmissionVersion.String(), key.ControlResultID.String(),
	).Scan(&kind, &handler, &basis, &handledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ControlReleaseRefusalHandling{}, false, nil
	}
	if err != nil {
		return ports.ControlReleaseRefusalHandling{}, false, fmt.Errorf("find control release refusal handling: %w", err)
	}
	handling, err := controlReleaseRefusalHandling(key, kind, handler, basis, handledAt)
	if err != nil {
		return ports.ControlReleaseRefusalHandling{}, false, fmt.Errorf("find control release refusal handling: %w", err)
	}
	return handling, true, nil
}

func controlReleaseRefusalHandling(
	key ports.ControlReleaseRefusalKey,
	kindRaw, handlerRaw, basisRaw string,
	handledAt time.Time,
) (ports.ControlReleaseRefusalHandling, error) {
	var kind ports.ControlReleaseRefusalHandlingKind
	switch kindRaw {
	case ports.ControlReleaseRefusalReleasedAtSettlement.String():
		kind = ports.ControlReleaseRefusalReleasedAtSettlement
	case ports.ControlReleaseRefusalMarkedHandled.String():
		kind = ports.ControlReleaseRefusalMarkedHandled
	default:
		return ports.ControlReleaseRefusalHandling{}, fmt.Errorf("handling kind %q is not a kind", kindRaw)
	}
	handler, err := domain.NewControlReleaseRefusalHandlerReference(handlerRaw)
	if err != nil {
		return ports.ControlReleaseRefusalHandling{}, err
	}
	basis, err := domain.NewControlReleaseRefusalHandlingBasis(basisRaw)
	if err != nil {
		return ports.ControlReleaseRefusalHandling{}, err
	}
	return ports.ControlReleaseRefusalHandling{
		Key:       key,
		Kind:      kind,
		Handler:   handler,
		Basis:     basis,
		HandledAt: handledAt.UTC(),
	}, nil
}
