package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 委托协议换版的读写（ADR-0231 决定三、四；迁移 0047）。写口挂在 TransportCommissions 上：换版是委托上的一串版本，委托按键读回时
// 连同它们一起装回，读与写同一个适配器。

var _ ports.CommissionAgreementRevisionStore = (*TransportCommissions)(nil)

// HoldRevisions 取这一份委托的换版锁（事务级咨询锁，双 int 形式：第一键是锁空间名散列，第二键是租户与委托以 chr(31) 拼接的散列）。
// 第二键撞上只让两份不相干的委托的换版多等一下，不错判。锁随事务释放，不在事务里调用由 RequireExecutor 拒——无锁的「已有几版」
// 答不了此刻。
func (repository *TransportCommissions) HoldRevisions(ctx context.Context, key ports.TransportCommissionKey) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("hold commission agreement revisions: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('transport_fulfillment.transport_commission.agreement_revision'), hashtext($1 || chr(31) || $2))`,
		key.TenantID.String(), key.Commission.String(),
	); err != nil {
		return fmt.Errorf("hold commission agreement revisions: %w", err)
	}
	return nil
}

// SaveRevision 落一行换版，只插不改。同一份委托上撞号或撞依据都答已登记，不覆盖先到者。
func (repository *TransportCommissions) SaveRevision(
	ctx context.Context,
	record ports.CommissionAgreementRevisionRecord,
) (ports.CommissionAgreementRevisionSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CommissionAgreementRevisionSaveOutcomeInvalid, fmt.Errorf("save commission agreement revision: %w", err)
	}
	revision := record.Revision
	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.transport_commission_agreement_revision
			(tenant_id, commission_id, revision, successor_object, successor_version, agreement_ref,
			 procurement_legal_entity_ref, applies_from, basis, decided_by, registered_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Commission.String(),
		revision.Number(),
		revision.Successor().Object(),
		revision.Successor().Version(),
		revision.Snapshot().String(),
		revision.LegalEntity().String(),
		revision.AppliesFrom().UTC(),
		revision.Basis().String(),
		revision.DecidedBy().String(),
		revision.RegisteredAt().UTC(),
	)
	if err != nil {
		return ports.CommissionAgreementRevisionSaveOutcomeInvalid, fmt.Errorf("save commission agreement revision: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CommissionAgreementRevisionAlreadyRegistered, nil
	}
	return ports.CommissionAgreementRevisionSaved, nil
}

// agreementRevisionsOf 读一份委托的各次换版，按号从小到大。一行装不回就整次报错，不跳过它：跳过一版，事实就会按别的版判。
func agreementRevisionsOf(
	ctx context.Context,
	querier bentopg.Querier,
	key ports.TransportCommissionKey,
) ([]domain.CommissionAgreementRevision, error) {
	rows, err := querier.Query(ctx,
		`SELECT revision, successor_object, successor_version, agreement_ref, procurement_legal_entity_ref,
		        applies_from, basis, decided_by, registered_at
		   FROM transport_fulfillment.transport_commission_agreement_revision
		  WHERE tenant_id = $1
		    AND commission_id = $2
		  ORDER BY revision`,
		key.TenantID.String(), key.Commission.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("read commission agreement revisions: %w", err)
	}
	defer rows.Close()
	var revisions []domain.CommissionAgreementRevision
	for rows.Next() {
		var (
			number                                                    int
			successorObject, successorVersion, snapshot, legal, basis string
			decidedBy                                                 string
			appliesFrom, registeredAt                                 time.Time
		)
		if err := rows.Scan(&number, &successorObject, &successorVersion, &snapshot, &legal,
			&appliesFrom, &basis, &decidedBy, &registeredAt); err != nil {
			return nil, fmt.Errorf("read commission agreement revision: %w", err)
		}
		revision, err := rebuildAgreementRevision(number, successorObject, successorVersion, snapshot, legal, appliesFrom, basis, decidedBy, registeredAt)
		if err != nil {
			return nil, fmt.Errorf("read commission agreement revision %d of %s: %w", number, key.Commission, err)
		}
		revisions = append(revisions, revision)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read commission agreement revisions: %w", err)
	}
	return revisions, nil
}

func rebuildAgreementRevision(
	number int,
	successorObject, successorVersion, snapshot, legal string,
	appliesFrom time.Time,
	basis, decidedBy string,
	registeredAt time.Time,
) (domain.CommissionAgreementRevision, error) {
	spec := domain.RehydrateCommissionAgreementRevisionSpec{Number: number, AppliesFrom: appliesFrom, RegisteredAt: registeredAt}
	var err error
	if spec.Successor, err = domain.NewSupplierAgreementVersion(successorObject, successorVersion); err != nil {
		return domain.CommissionAgreementRevision{}, err
	}
	if spec.Snapshot, err = domain.NewAgreementSnapshotReference(snapshot); err != nil {
		return domain.CommissionAgreementRevision{}, err
	}
	if spec.LegalEntity, err = domain.NewProcurementLegalEntityReference(legal); err != nil {
		return domain.CommissionAgreementRevision{}, err
	}
	if spec.Basis, err = domain.NewCommissionAgreementRevisionBasisReference(basis); err != nil {
		return domain.CommissionAgreementRevision{}, err
	}
	if spec.DecidedBy, err = domain.NewRegistrantReference(decidedBy); err != nil {
		return domain.CommissionAgreementRevision{}, err
	}
	return domain.RehydrateCommissionAgreementRevision(spec)
}
