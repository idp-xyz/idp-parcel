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

	var journey, legalEntity, provider, agreement, reason, factBasis, scope, unit, conditionBasis, registeredBy string
	var conditionAssertedBy, conditionAssertion, correctsVersion, revisionKind, revisionBasis *string
	var quantity int64
	var occurredAt, recordedAt time.Time
	var revisedAt *time.Time
	err = querier.QueryRow(ctx,
		`SELECT journey_ref, legal_entity_ref, provider_ref, agreement_ref, reason,
		        fact_basis, scope_ref, quantity, unit_ref, occurred_at,
		        condition_basis, condition_asserted_by, condition_assertion, registered_by,
		        corrects_version, revision_kind, revision_basis, revised_at, recorded_at
		   FROM transport_fulfillment.transport_charge_occurrence
		  WHERE tenant_id = $1 AND occurrence_ref = $2 AND validity_version = $3`,
		key.TenantID.String(), key.Occurrence.String(), key.Validity.String(),
	).Scan(&journey, &legalEntity, &provider, &agreement, &reason,
		&factBasis, &scope, &quantity, &unit, &occurredAt,
		&conditionBasis, &conditionAssertedBy, &conditionAssertion, &registeredBy,
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
	if spec.Condition, err = occurrenceConditionFrom(conditionBasis, registeredBy, conditionAssertedBy, conditionAssertion); err != nil {
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

// FindCovering 先取（租户、原因、主要业务范围）这一组的事务级咨询锁，再找另一项仍有效、盖住了查询的发生项（「盖住」
// 的三种读法见 ports.ChargeOccurrenceCoverageQuery）。
//
// 锁按范围不按事实依据（ADR-0209 决定四）：按范围计的一项要挡住同一范围上任何事实依据的另一笔，按依据上锁，两笔
// 各握各的锁、各自看不见对方。事实依据定得出范围，同一依据上的两笔照旧排在一把锁上。锁键只取范围串、不取出处：
// 揽收与派送同串的两次到场因此共用一把锁，只多等，不错判。
//
// 同一范围要同一出处：存下的依据串取第一道斜杠之前那一词，与查询依据的 Source 比（读法见
// domain.OccurrenceBasisReference.Source）。成员交集那一支本就要求依据相同，出处一条对它不多挡什么。
//
// 「仍有效」读每一项的当前版：没有别的版本以 corrects_version 回指它的那一版，且它不是失效版——被替代的旧版已由
// 新版接替，失效的那一项说的是「那个范围不成立」，其成员可以另登。多于一项盖住时回显最早登记的那一项：登记方
// 要去改的是先到的那一份。锁键各维以 chr(31) 分隔后取 hashtext，口径同 node_operations 收寄那把锁；哈希撞上只
// 多等一会儿，不错判。锁随事务释放，不在事务里调用由 RequireExecutor 拒——无锁的查答不了「此刻没有」。
func (repository *ChargeOccurrences) FindCovering(
	ctx context.Context,
	query ports.ChargeOccurrenceCoverageQuery,
) (ports.ChargeOccurrenceRecord, bool, error) {
	if query.TenantID.String() == "" || query.Reason.String() == "" || query.Scope.String() == "" ||
		query.FactBasis.String() == "" || query.Except.String() == "" || len(query.Members) == 0 {
		return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("find covering charge occurrence: query is incomplete")
	}
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("find covering charge occurrence: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('transport_fulfillment.charge_occurrence.scope'), hashtext($1 || chr(31) || $2 || chr(31) || $3))`,
		query.TenantID.String(), query.Reason.String(), query.Scope.String(),
	); err != nil {
		return ports.ChargeOccurrenceRecord{}, false, fmt.Errorf("find covering charge occurrence: 取范围锁：%w", err)
	}
	members := make([]string, 0, len(query.Members))
	for _, member := range query.Members {
		members = append(members, member.String())
	}
	scopeUnits := make([]string, 0, len(query.ScopeUnits))
	for _, unit := range query.ScopeUnits {
		scopeUnits = append(scopeUnits, unit.String())
	}
	var occurrenceRef, version string
	err = executor.QueryRow(ctx,
		`SELECT head.occurrence_ref, head.validity_version
		   FROM transport_fulfillment.transport_charge_occurrence AS head
		  WHERE head.tenant_id = $1
		    AND head.reason = $2
		    AND head.scope_ref = $3
		    AND split_part(head.fact_basis, '/', 1) = $10
		    AND head.occurrence_ref <> $4
		    AND head.revision_kind IS DISTINCT FROM $5
		    AND NOT EXISTS (
		        SELECT 1
		          FROM transport_fulfillment.transport_charge_occurrence AS successor
		         WHERE successor.tenant_id = head.tenant_id
		           AND successor.occurrence_ref = head.occurrence_ref
		           AND successor.corrects_version = head.validity_version)
		    AND ($6
		         OR head.unit_ref = ANY($7)
		         OR (head.fact_basis = $8
		             AND EXISTS (
		                 SELECT 1
		                   FROM transport_fulfillment.transport_charge_occurrence_member AS member
		                  WHERE member.tenant_id = head.tenant_id
		                    AND member.occurrence_ref = head.occurrence_ref
		                    AND member.validity_version = head.validity_version
		                    AND member.object_ref = ANY($9))))
		  ORDER BY head.recorded_at, head.occurrence_ref
		  LIMIT 1`,
		query.TenantID.String(), query.Reason.String(), query.Scope.String(), query.Except.String(),
		domain.OccurrenceInvalidated.String(), query.WholeScope, scopeUnits, query.FactBasis.String(), members,
		query.FactBasis.Source(),
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
	registeredBy := condition.RegisteredBy().String()
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
		      condition_basis, condition_asserted_by, condition_assertion, registered_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
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
		registeredBy,
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

// occurrenceConditionFrom 把库面四列装回发生条件的依据与登记者。依据一格是封闭集合，库面多出一格是坏行，不在这里
// 替它挑一个近似的读法。断言那一格的断言人与登记者是同一个人（迁移 0034 的 CHECK 守着），两列不一致即坏行；按协议
// 声明判的那一格不该带断言人与原话。
func occurrenceConditionFrom(basis, registeredBy string, assertedBy, assertion *string) (domain.OccurrenceCondition, error) {
	registrant, err := domain.NewRegistrantReference(registeredBy)
	if err != nil {
		return domain.OccurrenceCondition{}, err
	}
	switch basis {
	case domain.ConditionAssertedByRegistrant.String():
		if assertedBy == nil || assertion == nil || *assertedBy != registeredBy {
			return domain.OccurrenceCondition{}, fmt.Errorf("occurrence condition %q carries no assertion or another asserter", basis)
		}
		return domain.NewAssertedOccurrenceCondition(registrant, *assertion)
	case domain.ConditionDeclaredByAgreement.String():
		if assertedBy != nil || assertion != nil {
			return domain.OccurrenceCondition{}, fmt.Errorf("occurrence condition %q carries an assertion", basis)
		}
		return domain.NewAgreementDeclaredOccurrenceCondition(registrant)
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
