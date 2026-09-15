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

// ChargeOccurrences 实现 ports.ChargeOccurrenceRegistry：一行一个发生项有效性版本，
// 成员逐对象一行。
//
// 更正不换写原行，而是以新有效性版本落新行——版本在主键里，两代因此天然共存，
// corrects_version 回指前身。用 DO UPDATE 换写会让原发生项消失，而结算是按它追加调整的。
type ChargeOccurrences struct {
	db *bentopg.DB
}

func NewChargeOccurrences(db *bentopg.DB) (*ChargeOccurrences, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &ChargeOccurrences{db: db}, nil
}

var _ ports.ChargeOccurrenceRegistry = (*ChargeOccurrences)(nil)

// 同一结构体满足只读口：它读的是同两张表，只是契约窄——见 ports.ChargeOccurrenceMemberView 头注。
var _ ports.ChargeOccurrenceMemberView = (*ChargeOccurrences)(nil)

// FindByKey 按（租户+发生项+有效性版本）取回整条。读回过重建门复验。
func (repository *ChargeOccurrences) FindByKey(
	ctx context.Context,
	key ports.ChargeOccurrenceKey,
) (ports.ChargeOccurrenceRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("find charge occurrence: %w", err)
	}

	var journey, legalEntity, provider, agreement, reason, factBasis, scope, unit string
	var correctsVersion, revisionKind, revisionBasis *string
	var quantity int64
	var occurredAt, recordedAt time.Time
	var revisedAt *time.Time
	err = querier.QueryRow(ctx,
		`SELECT journey_ref, legal_entity_ref, provider_ref, agreement_ref, reason,
		        fact_basis, scope_ref, quantity, unit_ref, occurred_at,
		        corrects_version, revision_kind, revision_basis, revised_at, recorded_at
		   FROM transport_fulfillment.transport_charge_occurrence
		  WHERE tenant_id = $1 AND occurrence_ref = $2 AND validity_version = $3`,
		key.TenantID.String(), key.Occurrence.String(), key.Validity.String(),
	).Scan(&journey, &legalEntity, &provider, &agreement, &reason,
		&factBasis, &scope, &quantity, &unit, &occurredAt,
		&correctsVersion, &revisionKind, &revisionBasis, &revisedAt, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ChargeOccurrenceRecord{}, false, nil
	}
	if err != nil {
		return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("find charge occurrence: %w", err)
	}

	members, err := repository.loadMembers(ctx, key)
	if err != nil {
		return ports.ChargeOccurrenceRecord{}, false, err
	}

	spec := domain.RehydrateChargeOccurrenceSpec{
		TenantID:   key.TenantID,
		Occurrence: key.Occurrence,
		Validity:   key.Validity,
		Members:    members,
		Quantity:   quantity,
		OccurredAt: occurredAt,
	}
	if spec.Journey, err = domain.NewJourneyReference(journey); err != nil {
		return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("find charge occurrence: %w", err)
	}
	if spec.LegalEntity, err = domain.NewProcurementLegalEntityReference(legalEntity); err != nil {
		return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("find charge occurrence: %w", err)
	}
	if spec.Provider, err = domain.NewServiceProviderReference(provider); err != nil {
		return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("find charge occurrence: %w", err)
	}
	if spec.Agreement, err = domain.NewAgreementSnapshotReference(agreement); err != nil {
		return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("find charge occurrence: %w", err)
	}
	if spec.Reason, err = chargeOccurrenceReasonFrom(reason); err != nil {
		return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("find charge occurrence: %w", err)
	}
	if spec.FactBasis, err = domain.NewOccurrenceBasisReference(factBasis); err != nil {
		return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("find charge occurrence: %w", err)
	}
	if spec.Scope, err = domain.NewOccurrenceScopeReference(scope); err != nil {
		return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("find charge occurrence: %w", err)
	}
	if spec.Unit, err = domain.NewQuantityUnitReference(unit); err != nil {
		return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("find charge occurrence: %w", err)
	}
	if err := applyOccurrenceRevision(&spec, correctsVersion, revisionKind, revisionBasis, revisedAt); err != nil {
		return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("find charge occurrence: %w", err)
	}

	occurrence, err := domain.RehydrateChargeOccurrence(spec)
	if err != nil {
		return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("find charge occurrence: %w", err)
	}
	return ports.ChargeOccurrenceRecord{
		Key:        key,
		Occurrence: occurrence,
		RecordedAt: recordedAt.UTC(),
	}, true, nil
}

func (repository *ChargeOccurrences) loadMembers(
	ctx context.Context,
	key ports.ChargeOccurrenceKey,
) ([]domain.CarriedObjectReference, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("find charge occurrence members: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT object_ref
		   FROM transport_fulfillment.transport_charge_occurrence_member
		  WHERE tenant_id = $1 AND occurrence_ref = $2 AND validity_version = $3
		  ORDER BY object_ref`,
		key.TenantID.String(), key.Occurrence.String(), key.Validity.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("find charge occurrence members: %w", err)
	}
	defer rows.Close()

	var members []domain.CarriedObjectReference
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("find charge occurrence members: %w", err)
		}
		member, err := domain.NewCarriedObjectReference(raw)
		if err != nil {
			return nil, fmt.Errorf("find charge occurrence members: %w", err)
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find charge occurrence members: %w", err)
	}
	return members, nil
}

// LoadMembers 按（租户+发生项+有效性版本）答成员切面。一条 LEFT JOIN 把本体两列与成员逐行
// 一次取回：零行即键不在册（found=false，不退到别的版本）；本体在册而成员列为空是库面不一致
// ——领域构造门拒空成员、Save 又把两表落在同一笔里——这里响亮报错，不把空清单当答案交出去。
// 不经 FindByKey 再投影：那条路要把整条发生项过一遍重建门，读成员的一方不需要也不该为
// 协议、数量与修订三件的合法性买单。
func (repository *ChargeOccurrences) LoadMembers(
	ctx context.Context,
	key ports.ChargeOccurrenceKey,
) (ports.ChargeOccurrenceMembers, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ChargeOccurrenceMembers{}, false, fmt.Errorf("load charge occurrence members: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT occurrence.occurred_at, occurrence.scope_ref, member.object_ref
		   FROM transport_fulfillment.transport_charge_occurrence AS occurrence
		   LEFT JOIN transport_fulfillment.transport_charge_occurrence_member AS member
		     ON member.tenant_id = occurrence.tenant_id
		    AND member.occurrence_ref = occurrence.occurrence_ref
		    AND member.validity_version = occurrence.validity_version
		  WHERE occurrence.tenant_id = $1
		    AND occurrence.occurrence_ref = $2
		    AND occurrence.validity_version = $3
		  ORDER BY member.object_ref`,
		key.TenantID.String(), key.Occurrence.String(), key.Validity.String(),
	)
	if err != nil {
		return ports.ChargeOccurrenceMembers{}, false, fmt.Errorf("load charge occurrence members: %w", err)
	}
	defer rows.Close()

	var members ports.ChargeOccurrenceMembers
	found := false
	for rows.Next() {
		var occurredAt time.Time
		var scope string
		var objectRef *string
		if err := rows.Scan(&occurredAt, &scope, &objectRef); err != nil {
			return ports.ChargeOccurrenceMembers{}, false, fmt.Errorf("load charge occurrence members: %w", err)
		}
		if objectRef == nil {
			return ports.ChargeOccurrenceMembers{}, false, fmt.Errorf(
				"load charge occurrence members: occurrence %s/%s/%s is registered without members",
				key.TenantID.String(), key.Occurrence.String(), key.Validity.String())
		}
		if !found {
			members.OccurredAt = occurredAt.UTC()
			if members.Scope, err = domain.NewOccurrenceScopeReference(scope); err != nil {
				return ports.ChargeOccurrenceMembers{}, false, fmt.Errorf("load charge occurrence members: %w", err)
			}
			found = true
		}
		member, err := domain.NewCarriedObjectReference(*objectRef)
		if err != nil {
			return ports.ChargeOccurrenceMembers{}, false, fmt.Errorf("load charge occurrence members: %w", err)
		}
		members.Members = append(members.Members, member)
	}
	if err := rows.Err(); err != nil {
		return ports.ChargeOccurrenceMembers{}, false, fmt.Errorf("load charge occurrence members: %w", err)
	}
	if !found {
		return ports.ChargeOccurrenceMembers{}, false, nil
	}
	return members, true, nil
}

// applyOccurrenceRevision 把库面四列装回规格。四件的成对判定留给重建门——这里只负责
// 逐列过构造门，不在适配器里复刻那条不变量。
func applyOccurrenceRevision(
	spec *domain.RehydrateChargeOccurrenceSpec,
	correctsVersion, revisionKind, revisionBasis *string,
	revisedAt *time.Time,
) error {
	var err error
	if correctsVersion != nil {
		if spec.Corrects, err = domain.NewOccurrenceValidityVersion(*correctsVersion); err != nil {
			return err
		}
	}
	if revisionKind != nil {
		if spec.RevisionKind, err = occurrenceRevisionKindFrom(*revisionKind); err != nil {
			return err
		}
	}
	if revisionBasis != nil {
		if spec.RevisionBasis, err = domain.NewOccurrenceBasisReference(*revisionBasis); err != nil {
			return err
		}
	}
	if revisedAt != nil {
		spec.RevisedAt = revisedAt.UTC()
	}
	return nil
}

// Save 落一个发生项版本连同其对象范围。撞键答`已登记`（ADR-0031）。
func (repository *ChargeOccurrences) Save(
	ctx context.Context,
	record ports.ChargeOccurrenceRecord,
) (ports.ChargeOccurrenceSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ChargeOccurrenceSaveOutcomeInvalid, fmt.Errorf("save charge occurrence: %w", err)
	}
	if err := assertChargeOccurrenceKeyAgrees(record); err != nil {
		return ports.ChargeOccurrenceSaveOutcomeInvalid, fmt.Errorf("save charge occurrence: %w", err)
	}

	occurrence := record.Occurrence
	quantity, unit := occurrence.Quantity()

	var correctsVersion *string
	if corrects, has := occurrence.Corrects(); has {
		value := corrects.String()
		correctsVersion = &value
	}
	var revisionKind, revisionBasis *string
	var revisedAt *time.Time
	if kind, basis, at, revised := occurrence.Revision(); revised {
		kindName, basisValue, moment := kind.String(), basis.String(), at.UTC()
		revisionKind, revisionBasis, revisedAt = &kindName, &basisValue, &moment
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.transport_charge_occurrence
		     (tenant_id, occurrence_ref, validity_version,
		      journey_ref, legal_entity_ref, provider_ref, agreement_ref, reason,
		      fact_basis, scope_ref, quantity, unit_ref, occurred_at,
		      corrects_version, revision_kind, revision_basis, revised_at, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Occurrence.String(),
		record.Key.Validity.String(),
		occurrence.Journey().String(),
		occurrence.LegalEntity().String(),
		occurrence.Provider().String(),
		occurrence.Agreement().String(),
		occurrence.Reason().String(),
		occurrence.FactBasis().String(),
		occurrence.Scope().String(),
		quantity,
		unit.String(),
		occurrence.OccurredAt().UTC(),
		correctsVersion,
		revisionKind,
		revisionBasis,
		revisedAt,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.ChargeOccurrenceSaveOutcomeInvalid, fmt.Errorf("save charge occurrence: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ChargeOccurrenceAlreadyRegistered, nil
	}

	for _, member := range occurrence.Members() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO transport_fulfillment.transport_charge_occurrence_member
			     (tenant_id, occurrence_ref, validity_version, object_ref)
			 VALUES ($1, $2, $3, $4)`,
			record.Key.TenantID.String(),
			record.Key.Occurrence.String(),
			record.Key.Validity.String(),
			member.String(),
		); err != nil {
			return ports.ChargeOccurrenceSaveOutcomeInvalid, fmt.Errorf("save charge occurrence: %w", err)
		}
	}
	return ports.ChargeOccurrenceSaved, nil
}

func chargeOccurrenceReasonFrom(raw string) (domain.ChargeOccurrenceReason, error) {
	switch raw {
	case domain.BookingOccurrence.String():
		return domain.BookingOccurrence, nil
	case domain.CancellationOccurrence.String():
		return domain.CancellationOccurrence, nil
	case domain.FailedAttemptOccurrence.String():
		return domain.FailedAttemptOccurrence, nil
	case domain.ActualFulfillmentOccurrence.String():
		return domain.ActualFulfillmentOccurrence, nil
	default:
		return 0, fmt.Errorf("unknown charge occurrence reason %q", raw)
	}
}

func occurrenceRevisionKindFrom(raw string) (domain.OccurrenceRevisionKind, error) {
	switch raw {
	case domain.OccurrenceInvalidated.String():
		return domain.OccurrenceInvalidated, nil
	case domain.OccurrenceSuperseded.String():
		return domain.OccurrenceSuperseded, nil
	default:
		return 0, fmt.Errorf("unknown occurrence revision kind %q", raw)
	}
}

func assertChargeOccurrenceKeyAgrees(record ports.ChargeOccurrenceRecord) error {
	if record.Key.TenantID != record.Occurrence.TenantID() ||
		record.Key.Occurrence != record.Occurrence.Occurrence() ||
		record.Key.Validity != record.Occurrence.Validity() {
		return errors.New("record key disagrees with the occurrence it carries")
	}
	return nil
}
