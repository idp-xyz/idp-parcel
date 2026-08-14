package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// TransportCommissions 实现 ports.TransportCommissionStore。Save 登记；Replace 只写
// 取消或开始两列——协议/条件/角色/责任快照与成员只在 INSERT 出现（CONTEXT 243）。
type TransportCommissions struct {
	db *bentopg.DB
}

func NewTransportCommissions(db *bentopg.DB) (*TransportCommissions, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &TransportCommissions{db: db}, nil
}

// FindByKey 按（租户+委托）取回已登记委托。读回经重建门复验开始/取消互斥。
func (repository *TransportCommissions) FindByKey(
	ctx context.Context,
	key ports.TransportCommissionKey,
) (ports.TransportCommissionRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.TransportCommissionRecord{}, false, fmt.Errorf("find transport commission: %w", err)
	}

	var (
		provider, agreement, conditions, role, responsibility, digest string
		membersJSON                                                   []byte
		submittedAt, recordedAt                                       time.Time
		startedAt, cancelledAt                                        *time.Time
		startedBasis                                                  *string
	)
	err = querier.QueryRow(ctx,
		`SELECT provider_ref, agreement_ref, conditions_ref, role_ref, responsibility_ref,
		        members, submitted_at, started_at, started_basis, cancelled_at,
		        content_digest, recorded_at
		   FROM transport_fulfillment.transport_commission
		  WHERE tenant_id = $1
		    AND commission_id = $2`,
		key.TenantID.String(),
		key.Commission.String(),
	).Scan(&provider, &agreement, &conditions, &role, &responsibility,
		&membersJSON, &submittedAt, &startedAt, &startedBasis, &cancelledAt,
		&digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.TransportCommissionRecord{}, false, nil
	}
	if err != nil {
		return ports.TransportCommissionRecord{}, false, fmt.Errorf("find transport commission: %w", err)
	}

	commission, err := rebuildCommission(key, provider, agreement, conditions, role, responsibility, membersJSON, submittedAt, startedAt, startedBasis, cancelledAt)
	if err != nil {
		return ports.TransportCommissionRecord{}, false, fmt.Errorf("find transport commission: %w", err)
	}
	return ports.TransportCommissionRecord{
		Key:           key,
		ContentDigest: digest,
		Commission:    commission,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 登记一份运输委托。同键已有记录时答`已登记`，不覆盖先到者。
func (repository *TransportCommissions) Save(
	ctx context.Context,
	record ports.TransportCommissionRecord,
) (ports.CommissionSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CommissionSaveOutcomeInvalid, fmt.Errorf("save transport commission: %w", err)
	}

	memberIDs := make([]string, 0, len(record.Commission.Members()))
	for _, member := range record.Commission.Members() {
		memberIDs = append(memberIDs, member.String())
	}
	membersJSON, err := json.Marshal(memberIDs)
	if err != nil {
		return ports.CommissionSaveOutcomeInvalid, fmt.Errorf("save transport commission: %w", err)
	}

	startedBasis, startedAt := startedColumns(record.Commission)
	cancelledAt := cancelledColumn(record.Commission)

	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.transport_commission
			(tenant_id, commission_id, provider_ref, agreement_ref, conditions_ref,
			 role_ref, responsibility_ref, members, submitted_at, started_at,
			 started_basis, cancelled_at, content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Commission.String(),
		record.Commission.Provider().String(),
		record.Commission.Agreement().String(),
		record.Commission.Conditions().String(),
		record.Commission.Role().String(),
		record.Commission.Responsibility().String(),
		membersJSON,
		record.Commission.SubmittedAt().UTC(),
		startedAt,
		startedBasis,
		cancelledAt,
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.CommissionSaveOutcomeInvalid, fmt.Errorf("save transport commission: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CommissionAlreadyRegistered, nil
	}
	return ports.CommissionSaved, nil
}

// Replace 只落取消或开始：UPDATE 不含快照与成员列。WHERE 要求两者都还缺席——并发
// 第二转与行不存在同样答 false。拿一份既没取消也没开始的快照来换值是编排缺陷。
func (repository *TransportCommissions) Replace(
	ctx context.Context,
	record ports.TransportCommissionRecord,
) (bool, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return false, fmt.Errorf("replace transport commission: %w", err)
	}

	cancelledAt, cancelled := record.Commission.Cancelled()
	startedBasis, startedAt, started := record.Commission.TransportStarted()
	if cancelled == started {
		return false, fmt.Errorf("replace transport commission: replace requires exactly one of cancel or start")
	}

	if cancelled {
		tag, err := executor.Exec(ctx,
			`UPDATE transport_fulfillment.transport_commission
			    SET cancelled_at = $3, recorded_at = $4
			  WHERE tenant_id = $1
			    AND commission_id = $2
			    AND started_at IS NULL
			    AND cancelled_at IS NULL`,
			record.Key.TenantID.String(),
			record.Key.Commission.String(),
			cancelledAt.UTC(),
			record.RecordedAt.UTC(),
		)
		if err != nil {
			return false, fmt.Errorf("replace transport commission: %w", err)
		}
		return tag.RowsAffected() > 0, nil
	}

	tag, err := executor.Exec(ctx,
		`UPDATE transport_fulfillment.transport_commission
		    SET started_at = $3, started_basis = $4, recorded_at = $5
		  WHERE tenant_id = $1
		    AND commission_id = $2
		    AND started_at IS NULL
		    AND cancelled_at IS NULL`,
		record.Key.TenantID.String(),
		record.Key.Commission.String(),
		startedAt.UTC(),
		startedBasis.String(),
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return false, fmt.Errorf("replace transport commission: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func rebuildCommission(
	key ports.TransportCommissionKey,
	provider, agreement, conditions, role, responsibility string,
	membersJSON []byte,
	submittedAt time.Time,
	startedAt *time.Time,
	startedBasis *string,
	cancelledAt *time.Time,
) (domain.TransportCommission, error) {
	var memberIDs []string
	if err := json.Unmarshal(membersJSON, &memberIDs); err != nil {
		return domain.TransportCommission{}, err
	}
	members := make([]domain.CarriedObjectReference, 0, len(memberIDs))
	for _, raw := range memberIDs {
		member, err := domain.NewCarriedObjectReference(raw)
		if err != nil {
			return domain.TransportCommission{}, err
		}
		members = append(members, member)
	}

	spec := domain.RehydrateTransportCommissionSpec{
		TenantID:    key.TenantID,
		Commission:  key.Commission,
		SubmittedAt: submittedAt,
		Members:     members,
	}
	var err error
	if spec.Provider, err = domain.NewServiceProviderReference(provider); err != nil {
		return domain.TransportCommission{}, err
	}
	if spec.Agreement, err = domain.NewAgreementSnapshotReference(agreement); err != nil {
		return domain.TransportCommission{}, err
	}
	if spec.Conditions, err = domain.NewConditionsSnapshotReference(conditions); err != nil {
		return domain.TransportCommission{}, err
	}
	if spec.Role, err = domain.NewRoleSnapshotReference(role); err != nil {
		return domain.TransportCommission{}, err
	}
	if spec.Responsibility, err = domain.NewResponsibilitySnapshotReference(responsibility); err != nil {
		return domain.TransportCommission{}, err
	}
	if startedAt != nil {
		spec.StartedAt = startedAt.UTC()
	}
	if startedBasis != nil {
		basis, err := domain.NewParticipationBasisReference(*startedBasis)
		if err != nil {
			return domain.TransportCommission{}, err
		}
		spec.StartedBasis = basis
	}
	if cancelledAt != nil {
		spec.CancelledAt = cancelledAt.UTC()
	}
	return domain.RehydrateTransportCommission(spec)
}

func startedColumns(commission domain.TransportCommission) (*string, *time.Time) {
	basis, at, started := commission.TransportStarted()
	if !started {
		return nil, nil
	}
	raw := basis.String()
	utc := at.UTC()
	return &raw, &utc
}

func cancelledColumn(commission domain.TransportCommission) *time.Time {
	at, cancelled := commission.Cancelled()
	if !cancelled {
		return nil
	}
	utc := at.UTC()
	return &utc
}
