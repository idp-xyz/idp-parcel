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

// FulfillmentSegments 实现 ports.ActualFulfillmentSegmentRegistry：段一行，成员逐对象一行。
//
// 两张表同笔落。半个段（有段无成员）是领域产不出的东西——`RehydrateActualFulfillmentSegment`
// 对空成员集直接拒——所以写口要求环境事务，宁可拒绝写入也不留下一个读不回来的段。
type FulfillmentSegments struct {
	db *bentopg.DB
}

func NewFulfillmentSegments(db *bentopg.DB) (*FulfillmentSegments, error) {
	if db == nil {
		return nil, fmt.Errorf("transport fulfillment postgres: db is nil")
	}
	return &FulfillmentSegments{db: db}, nil
}

var _ ports.ActualFulfillmentSegmentRegistry = (*FulfillmentSegments)(nil)

// FindByKey 按（租户+段）取回整图。否定结果只回 false。
//
// 读回过重建门，逐格完备性与成对关系在这里复验一遍——坏行在这里暴露，而不是流到判断里。
func (repository *FulfillmentSegments) FindByKey(
	ctx context.Context,
	key ports.FulfillmentSegmentKey,
) (ports.FulfillmentSegmentRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.FulfillmentSegmentRecord{}, false, fmt.Errorf("find fulfillment segment: %w", err)
	}

	var closed bool
	var closedAt *time.Time
	var recordedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT closed, closed_at, recorded_at
		   FROM transport_fulfillment.actual_fulfillment_segment
		  WHERE tenant_id = $1 AND segment_ref = $2`,
		key.TenantID.String(), key.Segment.String(),
	).Scan(&closed, &closedAt, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.FulfillmentSegmentRecord{}, false, nil
	}
	if err != nil {
		return ports.FulfillmentSegmentRecord{}, false, fmt.Errorf("find fulfillment segment: %w", err)
	}

	// 成员按入场时刻再按对象排序：顺序不进任何判断，但让读回稳定，比对夹具时不必先排一遍。
	rows, err := querier.Query(ctx,
		`SELECT object_ref, planned_ref, entry_kind, entry_basis, entered_at,
		        end_kind, end_basis, ended_at
		   FROM transport_fulfillment.fulfillment_participation
		  WHERE tenant_id = $1 AND segment_ref = $2
		  ORDER BY entered_at, object_ref`,
		key.TenantID.String(), key.Segment.String(),
	)
	if err != nil {
		return ports.FulfillmentSegmentRecord{}, false, fmt.Errorf("find fulfillment segment: %w", err)
	}
	defer rows.Close()

	spec := domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID: key.TenantID,
		Segment:  key.Segment,
		Closed:   closed,
	}
	if closedAt != nil {
		spec.ClosedAt = closedAt.UTC()
	}
	for rows.Next() {
		var objectRef, entryKind, entryBasis string
		var plannedRef, endKind, endBasis *string
		var enteredAt time.Time
		var endedAt *time.Time
		if err := rows.Scan(&objectRef, &plannedRef, &entryKind, &entryBasis, &enteredAt,
			&endKind, &endBasis, &endedAt); err != nil {
			return ports.FulfillmentSegmentRecord{}, false, fmt.Errorf("find fulfillment segment: %w", err)
		}
		member, err := participationSpecFrom(participationRow{
			objectRef:  objectRef,
			plannedRef: plannedRef,
			entryKind:  entryKind,
			entryBasis: entryBasis,
			enteredAt:  enteredAt,
			endKind:    endKind,
			endBasis:   endBasis,
			endedAt:    endedAt,
		})
		if err != nil {
			return ports.FulfillmentSegmentRecord{}, false, fmt.Errorf("find fulfillment segment: %w", err)
		}
		spec.Participations = append(spec.Participations, member)
	}
	if err := rows.Err(); err != nil {
		return ports.FulfillmentSegmentRecord{}, false, fmt.Errorf("find fulfillment segment: %w", err)
	}

	segment, err := domain.RehydrateActualFulfillmentSegment(spec)
	if err != nil {
		return ports.FulfillmentSegmentRecord{}, false, fmt.Errorf("find fulfillment segment: %w", err)
	}
	return ports.FulfillmentSegmentRecord{
		Key:        key,
		Segment:    segment,
		RecordedAt: recordedAt.UTC(),
	}, true, nil
}

// Save 首登一个段连同它全部成员。撞键答`已登记`（ADR-0031），编排据此读回赢家。
func (repository *FulfillmentSegments) Save(
	ctx context.Context,
	record ports.FulfillmentSegmentRecord,
) (ports.SegmentSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.SegmentSaveOutcomeInvalid, fmt.Errorf("save fulfillment segment: %w", err)
	}
	if err := assertSegmentKeyAgrees(record); err != nil {
		return ports.SegmentSaveOutcomeInvalid, fmt.Errorf("save fulfillment segment: %w", err)
	}

	closedAt, closed := record.Segment.ClosedAt()
	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.actual_fulfillment_segment
		     (tenant_id, segment_ref, closed, closed_at, recorded_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Segment.String(),
		closed,
		nullableTime(closedAt, closed),
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.SegmentSaveOutcomeInvalid, fmt.Errorf("save fulfillment segment: %w", err)
	}
	// 撞键只看段那一行：成员挂在它下面，段已在册就说明这一整图已经有人写过。
	if tag.RowsAffected() == 0 {
		return ports.SegmentAlreadyRegistered, nil
	}

	for _, participation := range record.Segment.Participations() {
		if _, err := insertParticipationRow(
			ctx, executor, record.Key, participation, record.RecordedAt, false,
		); err != nil {
			return ports.SegmentSaveOutcomeInvalid, fmt.Errorf("save fulfillment segment: %w", err)
		}
	}
	return ports.SegmentSaved, nil
}

// Join 把一个对象插进既有段（ADR-0097 第一个窄口）。**只插不改**：撞主键即「该对象已在
// 段内」，是业务答案不是错误。
//
// 不校验段是否已关闭——那由外键之外的领域判断负责（`join` 在段已关闭时拒）。本口若替它判，
// 就成了第二个口径。
func (repository *FulfillmentSegments) Join(
	ctx context.Context,
	key ports.FulfillmentSegmentKey,
	participation domain.FulfillmentParticipation,
	recordedAt time.Time,
) (ports.SegmentJoinOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.SegmentJoinOutcomeInvalid, fmt.Errorf("join fulfillment segment: %w", err)
	}
	tag, err := insertParticipationRow(ctx, executor, key, participation, recordedAt, true)
	if err != nil {
		return ports.SegmentJoinOutcomeInvalid, fmt.Errorf("join fulfillment segment: %w", err)
	}
	if tag == 0 {
		return ports.ObjectAlreadyParticipating, nil
	}
	return ports.ObjectJoined, nil
}

// EndParticipation 填离场三列（ADR-0097 第二个窄口）。
//
// `WHERE ended_at IS NULL` 是这个口的全部要害：**它让"改写一条已离场的参与"表达不出来**。
// 没有匹配行时答`已离场`，而不是报错——已结束的参与不重复结束也不改写，那是业务答案。
func (repository *FulfillmentSegments) EndParticipation(
	ctx context.Context,
	key ports.FulfillmentSegmentKey,
	participation domain.FulfillmentParticipation,
) (ports.ParticipationEndOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ParticipationEndOutcomeInvalid, fmt.Errorf("end participation: %w", err)
	}
	kind, basis, endedAt, ended := participation.End()
	if !ended {
		return ports.ParticipationEndOutcomeInvalid,
			errors.New("end participation: the participation carries no end")
	}

	tag, err := executor.Exec(ctx,
		`UPDATE transport_fulfillment.fulfillment_participation
		    SET end_kind = $1, end_basis = $2, ended_at = $3
		  WHERE tenant_id = $4 AND segment_ref = $5 AND object_ref = $6
		    AND ended_at IS NULL`,
		kind.String(), basis.String(), endedAt.UTC(),
		key.TenantID.String(), key.Segment.String(), participation.Object().String(),
	)
	if err != nil {
		return ports.ParticipationEndOutcomeInvalid, fmt.Errorf("end participation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ParticipationAlreadyEnded, nil
	}
	return ports.ParticipationEnded, nil
}

// CloseSegment 填关闭两列（ADR-0097 第三个窄口）。`WHERE closed = false` 让重复关段与
// 「把已关闭的段改回未关闭」都表达不出来。
//
// **它不校验是否仍有在场参与**——那是跨行条件，本口表达不了，留在领域。编排必须先读回整段、
// 走 `CloseSegment` 转换门再调本口。
func (repository *FulfillmentSegments) CloseSegment(
	ctx context.Context,
	key ports.FulfillmentSegmentKey,
	closedAt time.Time,
) (ports.SegmentCloseOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.SegmentCloseOutcomeInvalid, fmt.Errorf("close fulfillment segment: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE transport_fulfillment.actual_fulfillment_segment
		    SET closed = true, closed_at = $1
		  WHERE tenant_id = $2 AND segment_ref = $3 AND closed = false`,
		closedAt.UTC(), key.TenantID.String(), key.Segment.String(),
	)
	if err != nil {
		return ports.SegmentCloseOutcomeInvalid, fmt.Errorf("close fulfillment segment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.SegmentAlreadyClosed, nil
	}
	return ports.SegmentClosed, nil
}

// insertParticipationRow 落一条参与关系。`ignoreConflict` 区分两个调用方：首登时撞键说明
// 记录本身有问题（同一段里同一对象两条），应当报错；`Join` 时撞键是业务答案。
//
// 两个调用方共用同一段 INSERT 而不是各写一遍：列一多，两份就会在下一次加列时分叉，而分叉
// 处正是「首登写得对、加入写漏一列」这种不可能靠测试穷尽的错。
func insertParticipationRow(
	ctx context.Context,
	executor bentopg.Executor,
	key ports.FulfillmentSegmentKey,
	participation domain.FulfillmentParticipation,
	recordedAt time.Time,
	ignoreConflict bool,
) (int64, error) {
	var plannedRef *string
	if planned, has := participation.PlannedSegment(); has {
		value := planned.String()
		plannedRef = &value
	}

	var endKind, endBasis *string
	var endedAt *time.Time
	if kind, basis, at, ended := participation.End(); ended {
		kindName, basisValue, endMoment := kind.String(), basis.String(), at.UTC()
		endKind, endBasis, endedAt = &kindName, &basisValue, &endMoment
	}

	statement := `INSERT INTO transport_fulfillment.fulfillment_participation
		     (tenant_id, segment_ref, object_ref, planned_ref, entry_kind, entry_basis,
		      entered_at, end_kind, end_basis, ended_at, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`
	if ignoreConflict {
		statement += ` ON CONFLICT DO NOTHING`
	}

	tag, err := executor.Exec(ctx, statement,
		key.TenantID.String(),
		key.Segment.String(),
		participation.Object().String(),
		plannedRef,
		participation.EntryKind().String(),
		participation.EntryBasis().String(),
		participation.EnteredAt().UTC(),
		endKind,
		endBasis,
		endedAt,
		recordedAt.UTC(),
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// participationRow 是参与关系在库面的一行。
type participationRow struct {
	objectRef  string
	plannedRef *string
	entryKind  string
	entryBasis string
	enteredAt  time.Time
	endKind    *string
	endBasis   *string
	endedAt    *time.Time
}

// participationSpecFrom 逐列走各自的构造门装回，不按列直接拼结构体——构造门是坏行的
// 第一道拦截，绕过它等于把库当成可信来源。
func participationSpecFrom(row participationRow) (domain.RehydrateParticipationSpec, error) {
	spec := domain.RehydrateParticipationSpec{EnteredAt: row.enteredAt.UTC()}

	var err error
	if spec.Object, err = domain.NewCarriedObjectReference(row.objectRef); err != nil {
		return domain.RehydrateParticipationSpec{}, err
	}
	if row.plannedRef != nil {
		if spec.Planned, err = domain.NewPlannedSegmentReference(*row.plannedRef); err != nil {
			return domain.RehydrateParticipationSpec{}, err
		}
	}
	if spec.EntryKind, err = participationEntryKindFrom(row.entryKind); err != nil {
		return domain.RehydrateParticipationSpec{}, err
	}
	if spec.EntryBasis, err = domain.NewParticipationBasisReference(row.entryBasis); err != nil {
		return domain.RehydrateParticipationSpec{}, err
	}

	if row.endedAt != nil {
		spec.EndedAt = row.endedAt.UTC()
	}
	if row.endKind != nil {
		if spec.EndKind, err = participationEndKindFrom(*row.endKind); err != nil {
			return domain.RehydrateParticipationSpec{}, err
		}
	}
	if row.endBasis != nil {
		if spec.EndBasis, err = domain.NewParticipationBasisReference(*row.endBasis); err != nil {
			return domain.RehydrateParticipationSpec{}, err
		}
	}
	return spec, nil
}

func participationEntryKindFrom(raw string) (domain.ParticipationEntryKind, error) {
	switch raw {
	case domain.EnteredByOffsitePickup.String():
		return domain.EnteredByOffsitePickup, nil
	case domain.EnteredByTransportHandover.String():
		return domain.EnteredByTransportHandover, nil
	default:
		return 0, fmt.Errorf("unknown participation entry kind %q", raw)
	}
}

func participationEndKindFrom(raw string) (domain.ParticipationEndKind, error) {
	switch raw {
	case domain.EndedByEffectiveDelivery.String():
		return domain.EndedByEffectiveDelivery, nil
	case domain.EndedByNextHandover.String():
		return domain.EndedByNextHandover, nil
	case domain.EndedByControlTermination.String():
		return domain.EndedByControlTermination, nil
	default:
		return 0, fmt.Errorf("unknown participation end kind %q", raw)
	}
}

// assertSegmentKeyAgrees 挡住「键与本体说的不是同一个段」——那种记录一旦落库，按键取回的
// 东西与它自称的身份对不上，而两边都看不出错。
func assertSegmentKeyAgrees(record ports.FulfillmentSegmentRecord) error {
	if record.Key.TenantID != record.Segment.TenantID() ||
		record.Key.Segment != record.Segment.Segment() {
		return errors.New("record key disagrees with the segment it carries")
	}
	if !record.Segment.Established() {
		return errors.New("a segment without participations was never established")
	}
	return nil
}

func nullableTime(value time.Time, present bool) *time.Time {
	if !present {
		return nil
	}
	moment := value.UTC()
	return &moment
}
