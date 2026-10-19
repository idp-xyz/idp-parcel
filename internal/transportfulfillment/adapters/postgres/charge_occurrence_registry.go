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

	var journey, legalEntity, provider, agreement, reason, factBasis, scope, unit, conditionBasis string
	var conditionAssertedBy, conditionAssertion, correctsVersion, revisionKind, revisionBasis *string
	var quantity int64
	var occurredAt, recordedAt time.Time
	var revisedAt *time.Time
	err = querier.QueryRow(ctx,
		`SELECT journey_ref, legal_entity_ref, provider_ref, agreement_ref, reason,
		        fact_basis, scope_ref, quantity, unit_ref, occurred_at,
		        condition_basis, condition_asserted_by, condition_assertion,
		        corrects_version, revision_kind, revision_basis, revised_at, recorded_at
		   FROM transport_fulfillment.transport_charge_occurrence
		  WHERE tenant_id = $1 AND occurrence_ref = $2 AND validity_version = $3`,
		key.TenantID.String(), key.Occurrence.String(), key.Validity.String(),
	).Scan(&journey, &legalEntity, &provider, &agreement, &reason,
		&factBasis, &scope, &quantity, &unit, &occurredAt,
		&conditionBasis, &conditionAssertedBy, &conditionAssertion,
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
	if spec.Condition, err = occurrenceConditionFrom(conditionBasis, conditionAssertedBy, conditionAssertion); err != nil {
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

// FindCovering 先取（租户、原因、事实依据）这一组的事务级咨询锁，再找另一项仍有效、成员与查询有交集的发生项。
//
// 「仍有效」读每一项的当前版：没有别的版本以 corrects_version 回指它的那一版，且它不是失效版——被替代的旧版已由
// 新版接替，失效的那一项说的是「那个范围不成立」，其成员可以另登。多于一项有交集时回显最早登记的那一项：登记方
// 要去改的是先到的那一份。锁键各维以 chr(31) 分隔后取 hashtext，口径同 node_operations 收寄那把锁；哈希撞上只
// 多等一会儿，不错判。锁随事务释放，不在事务里调用由 RequireExecutor 拒——无锁的查答不了「此刻没有」。
func (repository *ChargeOccurrences) FindCovering(
	ctx context.Context,
	query ports.ChargeOccurrenceCoverageQuery,
) (ports.ChargeOccurrenceRecord, bool, error) {
	if query.TenantID.String() == "" || query.Reason.String() == "" || query.FactBasis.String() == "" ||
		query.Except.String() == "" || len(query.Members) == 0 {
		return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("find covering charge occurrence: query is incomplete")
	}
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("find covering charge occurrence: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('transport_fulfillment.charge_occurrence.fact_basis'), hashtext($1 || chr(31) || $2 || chr(31) || $3))`,
		query.TenantID.String(), query.Reason.String(), query.FactBasis.String(),
	); err != nil {
		return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("find covering charge occurrence: 取事实依据锁：%w", err)
	}
	members := make([]string, 0, len(query.Members))
	for _, member := range query.Members {
		members = append(members, member.String())
	}
	var occurrenceRef, version string
	err = executor.QueryRow(ctx,
		`SELECT head.occurrence_ref, head.validity_version
		   FROM transport_fulfillment.transport_charge_occurrence AS head
		  WHERE head.tenant_id = $1
		    AND head.reason = $2
		    AND head.fact_basis = $3
		    AND head.occurrence_ref <> $4
		    AND head.revision_kind IS DISTINCT FROM $5
		    AND NOT EXISTS (
		        SELECT 1
		          FROM transport_fulfillment.transport_charge_occurrence AS successor
		         WHERE successor.tenant_id = head.tenant_id
		           AND successor.occurrence_ref = head.occurrence_ref
		           AND successor.corrects_version = head.validity_version)
		    AND EXISTS (
		        SELECT 1
		          FROM transport_fulfillment.transport_charge_occurrence_member AS member
		         WHERE member.tenant_id = head.tenant_id
		           AND member.occurrence_ref = head.occurrence_ref
		           AND member.validity_version = head.validity_version
		           AND member.object_ref = ANY($6))
		  ORDER BY head.recorded_at, head.occurrence_ref
		  LIMIT 1`,
		query.TenantID.String(), query.Reason.String(), query.FactBasis.String(), query.Except.String(),
		domain.OccurrenceInvalidated.String(), members,
	).Scan(&occurrenceRef, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ChargeOccurrenceRecord{}, false, nil
	}
	if err != nil {
		return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("find covering charge occurrence: %w", err)
	}
	key := ports.ChargeOccurrenceKey{TenantID: query.TenantID}
	if key.Occurrence, err = domain.NewChargeOccurrenceReference(occurrenceRef); err != nil {
		return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("find covering charge occurrence: %w", err)
	}
	if key.Validity, err = domain.NewOccurrenceValidityVersion(version); err != nil {
		return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("find covering charge occurrence: %w", err)
	}
	record, found, err := repository.FindByKey(ctx, key)
	if err != nil {
		return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("find covering charge occurrence: %w", err)
	}
	if !found {
		return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf(
			"find covering charge occurrence: %s/%s/%s vanished within the same transaction",
			key.TenantID.String(), key.Occurrence.String(), key.Validity.String())
	}
	return record, true, nil
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
	condition := occurrence.Condition()
	var conditionAssertedBy, conditionAssertion *string
	if assertion, asserted := condition.Assertion(); asserted {
		conditionAssertion = &assertion
	}
	if registrant, asserted := condition.AssertedBy(); asserted {
		value := registrant.String()
		conditionAssertedBy = &value
	}

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
		      corrects_version, revision_kind, revision_basis, revised_at, recorded_at,
		      condition_basis, condition_asserted_by, condition_assertion)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
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
		condition.Basis().String(),
		conditionAssertedBy,
		conditionAssertion,
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

// occurrenceConditionFrom 把库面三列装回发生条件的依据。依据一格是封闭集合，库面多出一格是坏行，不在这里替它
// 挑一个近似的读法。
func occurrenceConditionFrom(basis string, assertedBy, assertion *string) (domain.OccurrenceCondition, error) {
	switch basis {
	case domain.ConditionAssertedByRegistrant.String():
		if assertedBy == nil || assertion == nil {
			return domain.OccurrenceCondition{}, fmt.Errorf("occurrence condition %q carries no registrant or assertion", basis)
		}
		registrant, err := domain.NewRegistrantReference(*assertedBy)
		if err != nil {
			return domain.OccurrenceCondition{}, err
		}
		return domain.NewAssertedOccurrenceCondition(registrant, *assertion)
	default:
		return domain.OccurrenceCondition{}, fmt.Errorf("unknown occurrence condition basis %q", basis)
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
