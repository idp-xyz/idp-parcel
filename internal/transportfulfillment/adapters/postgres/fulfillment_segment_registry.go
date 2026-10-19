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
	var serviceAction *string
	err = querier.QueryRow(ctx,
		`SELECT closed, closed_at, recorded_at, service_action
		   FROM transport_fulfillment.actual_fulfillment_segment
		  WHERE tenant_id = $1 AND segment_ref = $2`,
		key.TenantID.String(), key.Segment.String(),
	).Scan(&closed, &closedAt, &recordedAt, &serviceAction)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.FulfillmentSegmentRecord{}, false, nil
	}
	if err != nil {
		return ports.FulfillmentSegmentRecord{}, false, fmt.Errorf("find fulfillment segment: %w", err)
	}

	// 成员按入场时刻再按对象、入场依据排序：顺序不进任何判断，但让读回稳定，比对夹具时不必先排一遍。
	rows, err := querier.Query(ctx,
		`SELECT object_ref, planned_ref, entry_kind, entry_basis, entered_at,
		        end_kind, end_basis, ended_at, supersedes_entry_basis, voided
		   FROM transport_fulfillment.fulfillment_participation
		  WHERE tenant_id = $1 AND segment_ref = $2
		  ORDER BY entered_at, object_ref, entry_basis`,
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
	// NULL 就是未声明（ADR-0114 决定一）；非空的词经封闭集合认回，词不在集合内是坏行，在这里响亮。
	if serviceAction != nil {
		if spec.ServiceAction, err = domain.ParseSegmentServiceAction(*serviceAction); err != nil {
			return ports.FulfillmentSegmentRecord{}, false, fmt.Errorf("find fulfillment segment: %w", err)
		}
	}
	for rows.Next() {
		var objectRef, entryKind, entryBasis string
		var plannedRef, endKind, endBasis, supersedes *string
		var enteredAt time.Time
		var endedAt *time.Time
		var voided bool
		if err := rows.Scan(&objectRef, &plannedRef, &entryKind, &entryBasis, &enteredAt,
			&endKind, &endBasis, &endedAt, &supersedes, &voided); err != nil {
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
			supersedes: supersedes,
			voided:     voided,
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
	// 段服务动作只在首登这一行写入（ADR-0114 决定一）：未声明落 NULL，不给默认。
	var serviceAction *string
	if action, declared := record.Segment.ServiceAction(); declared {
		word := action.String()
		serviceAction = &word
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO transport_fulfillment.actual_fulfillment_segment
		     (tenant_id, segment_ref, closed, closed_at, recorded_at, service_action)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Segment.String(),
		closed,
		nullableTime(closedAt, closed),
		record.RecordedAt.UTC(),
		serviceAction,
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
			ctx, executor, record.Key, participation, endColumnsOf(participation), record.RecordedAt, false,
		); err != nil {
			return ports.SegmentSaveOutcomeInvalid, fmt.Errorf("save fulfillment segment: %w", err)
		}
	}
	return ports.SegmentSaved, nil
}

// Join 把一个对象插进既有段（ADR-0097 第一个窄口）。**只插不改**：撞键即「该对象已在
// 段内」，是业务答案不是错误——首次入场撞的是「每对象一个首登」那道部分唯一索引。
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
	if _, chained := participation.Supersedes(); chained {
		return ports.SegmentJoinOutcomeInvalid,
			errors.New("join fulfillment segment: a superseding participation enters through Supersede")
	}
	tag, err := insertParticipationRow(ctx, executor, key, participation, endColumnsOf(participation), recordedAt, true)
	if err != nil {
		return ports.SegmentJoinOutcomeInvalid, fmt.Errorf("join fulfillment segment: %w", err)
	}
	if tag == 0 {
		return ports.ObjectAlreadyParticipating, nil
	}
	return ports.ObjectJoined, nil
}

// Supersede 把一条替代参与版本插进段（ADR-0112 决定一）。**只插不改**，与 Join 同一段 INSERT：
// 撞主键是同一更正的重放，撞「一版至多被替代一次」那道索引是另一方先替代了同一前版——两者都答
// `已替代`，编排读回链尾作答。自引用外键让回指一个不存在的前版在这里就失败。
//
// 失效版本（ADR-0112 决定四，迁移 0019）也从这一口进：它同样回指前版，只多 voided 列为真；「首登不能
// 失效、只出自交接更正」由 0019 的 CHECK 与领域重建门守，本口不另判。
//
// 不校验段是否已关闭：段已关闭照样长替代版本，那是 CONTEXT 封存例外格，领域门也不拒。
//
// **落笔前先锁住被替代的链尾，新版本的离场三件取锁住之后读到的那一份**（票 tf-face-algebra/05）。调用方的派生
// 基于一次不加锁的读：读完到落笔之间，终止的窄口写可能已在被替代链尾上提交，或正挂着未提交——按旧读法落一版
// 没有终点的新链尾，终点就留在已被替代的版本上，参与重新在场。锁让替代排在那次写之后；离场三件是参与行上唯一会被
// 改写的列，也是领域重派生从被替代版本继承的全部（ActualFulfillmentSegment.rederive），所以只在这三列上按锁后的
// 在册值重派生，其余各格仍是调用方交来的。
func (repository *FulfillmentSegments) Supersede(
	ctx context.Context,
	key ports.FulfillmentSegmentKey,
	participation domain.FulfillmentParticipation,
	recordedAt time.Time,
) (ports.ParticipationSupersedeOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ParticipationSupersedeOutcomeInvalid, fmt.Errorf("supersede participation: %w", err)
	}
	replaced, chained := participation.Supersedes()
	if !chained {
		return ports.ParticipationSupersedeOutcomeInvalid,
			errors.New("supersede participation: the participation supersedes nothing")
	}
	end, err := lockedInheritedEnd(ctx, executor, key, participation, replaced)
	if err != nil {
		return ports.ParticipationSupersedeOutcomeInvalid, fmt.Errorf("supersede participation: %w", err)
	}
	tag, err := insertParticipationRow(ctx, executor, key, participation, end, recordedAt, true)
	if err != nil {
		return ports.ParticipationSupersedeOutcomeInvalid, fmt.Errorf("supersede participation: %w", err)
	}
	if tag == 0 {
		return ports.ParticipationAlreadySuperseded, nil
	}
	return ports.ParticipationSuperseded, nil
}

// lockedInheritedEnd 以 FOR UPDATE 锁住被替代的那一行，交回新版本应带的离场三列：被替代版本在册的那一份。
//
// 被替代的行不在册时交回调用方自带的那一份，不在这里报错——自引用外键会在插入时拒它，「回指一个不存在的前版」
// 的口径留在那一处。
//
// 继承的终点早于新版本的起点即先结束再进入，领域在新鲜读法上会拒（ErrCorrectedStartAfterInheritedEnd）。这里
// 答不出那一格，只交错误让编排记欠账；不放给 0006 的 CHECK 去撞，CHECK 失败会让调用方整笔事务作废，连同一笔
// 里的来源更正一起丢掉。
func lockedInheritedEnd(
	ctx context.Context,
	executor bentopg.Executor,
	key ports.FulfillmentSegmentKey,
	participation domain.FulfillmentParticipation,
	replaced domain.ParticipationBasisReference,
) (participationEndColumns, error) {
	carried := endColumnsOf(participation)
	var locked participationEndColumns
	err := executor.QueryRow(ctx,
		`SELECT end_kind, end_basis, ended_at
		   FROM transport_fulfillment.fulfillment_participation
		  WHERE tenant_id = $1 AND segment_ref = $2 AND object_ref = $3 AND entry_basis = $4
		    FOR UPDATE`,
		key.TenantID.String(), key.Segment.String(), participation.Object().String(), replaced.String(),
	).Scan(&locked.kind, &locked.basis, &locked.at)
	if errors.Is(err, pgx.ErrNoRows) {
		return carried, nil
	}
	if err != nil {
		return participationEndColumns{}, err
	}
	// 终点只增不改（在场谓词），被替代版本在册无终点而新版本带着一份，只能是调用方交错了参与。
	if locked.at == nil && carried.at != nil {
		return participationEndColumns{}, errors.New("the superseding participation carries an end its superseded version does not have")
	}
	if locked.at != nil && participation.EnteredAt().After(*locked.at) {
		return participationEndColumns{}, errors.New("the corrected start falls after the end inherited from the superseded version")
	}
	return locked, nil
}

// currentParticipationPredicate 是「这一行是该对象在段内的当前参与（链尾）」在 SQL 上的写法：没有任何
// 一版回指它。「被替代」不落列（ADR-0112 决定一），读写两侧对它的定义因此只有这一处。调用方要给外层表起
// 别名 `participation`。
const currentParticipationPredicate = `NOT EXISTS (
		    SELECT 1
		      FROM transport_fulfillment.fulfillment_participation successor
		     WHERE successor.tenant_id = participation.tenant_id
		       AND successor.segment_ref = participation.segment_ref
		       AND successor.object_ref = participation.object_ref
		       AND successor.supersedes_entry_basis = participation.entry_basis)`

// activeParticipationPredicate 是「在场」在 SQL 上的写法：未离场、未失效、且是链尾——与领域 Active() 的三条
// 同一（票 tf-segment-lifecycle-closure/11 裁决 4）。FindActiveSegments 与 EndParticipation 都用它，读写两侧对
// 「在场」的定义因此只有这一处：被替代的版本不再是当前控制，失效的版本没有当前控制，两者都不算在场也不会被结束。
const activeParticipationPredicate = `participation.ended_at IS NULL
		    AND NOT participation.voided
		    AND ` + currentParticipationPredicate

// FindActiveSegments 按（租户 + 对象）找回该对象仍在场的全部段键。按入场时刻排序只为读回稳定，不进任何判断。
func (repository *FulfillmentSegments) FindActiveSegments(
	ctx context.Context,
	tenant domain.TenantID,
	object domain.CarriedObjectReference,
) ([]ports.FulfillmentSegmentKey, error) {
	return repository.findSegmentKeys(ctx, "find active segments",
		`SELECT participation.segment_ref
		   FROM transport_fulfillment.fulfillment_participation participation
		  WHERE participation.tenant_id = $1 AND participation.object_ref = $2
		    AND `+activeParticipationPredicate+`
		  ORDER BY participation.entered_at, participation.segment_ref`,
		tenant, object)
}

// FindSegmentsForObject 按（租户 + 对象）找回该对象有参与的全部段键，在场或已离场都算（ADR-0112 决定二）。
// 一段只报一次：链上几版都在同一段里。
func (repository *FulfillmentSegments) FindSegmentsForObject(
	ctx context.Context,
	tenant domain.TenantID,
	object domain.CarriedObjectReference,
) ([]ports.FulfillmentSegmentKey, error) {
	return repository.findSegmentKeys(ctx, "find segments for object",
		`SELECT participation.segment_ref
		   FROM transport_fulfillment.fulfillment_participation participation
		  WHERE participation.tenant_id = $1 AND participation.object_ref = $2
		  GROUP BY participation.segment_ref
		  ORDER BY min(participation.entered_at), participation.segment_ref`,
		tenant, object)
}

func (repository *FulfillmentSegments) findSegmentKeys(
	ctx context.Context,
	operation string,
	query string,
	tenant domain.TenantID,
	object domain.CarriedObjectReference,
) ([]ports.FulfillmentSegmentKey, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := querier.Query(ctx, query, tenant.String(), object.String())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	defer rows.Close()

	var keys []ports.FulfillmentSegmentKey
	for rows.Next() {
		var segmentRef string
		if err := rows.Scan(&segmentRef); err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		segment, err := domain.NewFulfillmentSegmentReference(segmentRef)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}
		keys = append(keys, ports.FulfillmentSegmentKey{TenantID: tenant, Segment: segment})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return keys, nil
}

// EndParticipation 填离场三列（ADR-0097 第二个窄口）。
//
// 「在场」谓词是这个口的全部要害：**它让"改写一条已离场的参与"、"结束一条已被替代的参与"与"结束一条已失效的
// 参与"都表达不出来**。行按入场依据指名——那是参与在链上的身份，调用方交来的就是领域判定的链尾。
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

	// 先单独锁住这一行，再用一条新语句判在场并写（票 tf-face-algebra/05）。READ COMMITTED 下 UPDATE 的链尾判据
	// 按语句开始时的快照求值；它若排在替代口锁住这一行的事务后面，对方提交时这一行本身没改，UPDATE 不重判就照写，
	// 终点落到刚被替代的版本上。锁先拿到，后面的 UPDATE 才看得见对方刚落的新链尾、写中 0 行，交编排读回重走。
	if _, err := executor.Exec(ctx,
		`SELECT 1
		   FROM transport_fulfillment.fulfillment_participation
		  WHERE tenant_id = $1 AND segment_ref = $2 AND object_ref = $3 AND entry_basis = $4
		    FOR UPDATE`,
		key.TenantID.String(), key.Segment.String(), participation.Object().String(),
		participation.EntryBasis().String(),
	); err != nil {
		return ports.ParticipationEndOutcomeInvalid, fmt.Errorf("end participation: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`UPDATE transport_fulfillment.fulfillment_participation participation
		    SET end_kind = $1, end_basis = $2, ended_at = $3
		  WHERE participation.tenant_id = $4 AND participation.segment_ref = $5
		    AND participation.object_ref = $6 AND participation.entry_basis = $7
		    AND `+activeParticipationPredicate,
		kind.String(), basis.String(), endedAt.UTC(),
		key.TenantID.String(), key.Segment.String(), participation.Object().String(),
		participation.EntryBasis().String(),
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

// insertParticipationRow 落一条参与关系。`ignoreConflict` 区分调用方：首登时撞键说明
// 记录本身有问题（同一段里同一对象两条不成链），应当报错；`Join` 与 `Supersede` 时撞键是业务答案。
//
// 各调用方共用同一段 INSERT 而不是各写一遍：列一多，几份就会在下一次加列时分叉，而分叉
// 处正是「首登写得对、加入写漏一列」这种不可能靠测试穷尽的错。
func insertParticipationRow(
	ctx context.Context,
	executor bentopg.Executor,
	key ports.FulfillmentSegmentKey,
	participation domain.FulfillmentParticipation,
	end participationEndColumns,
	recordedAt time.Time,
	ignoreConflict bool,
) (int64, error) {
	var plannedRef *string
	if planned, has := participation.PlannedSegment(); has {
		value := planned.String()
		plannedRef = &value
	}

	var supersedes *string
	if replaced, chained := participation.Supersedes(); chained {
		value := replaced.String()
		supersedes = &value
	}

	statement := `INSERT INTO transport_fulfillment.fulfillment_participation
		     (tenant_id, segment_ref, object_ref, planned_ref, entry_kind, entry_basis,
		      entered_at, end_kind, end_basis, ended_at, recorded_at, supersedes_entry_basis, voided)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`
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
		end.kind,
		end.basis,
		end.at,
		recordedAt.UTC(),
		supersedes,
		participation.Voided(),
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// participationEndColumns 是离场三列在库面的样子，三者同在或同缺（0006 的 CHECK）。离场三列与参与的其余各格
// 分开传，是因为替代口要按锁后的在册值落这三列，其余各格仍取调用方交来的参与（lockedInheritedEnd）。
type participationEndColumns struct {
	kind  *string
	basis *string
	at    *time.Time
}

func endColumnsOf(participation domain.FulfillmentParticipation) participationEndColumns {
	kind, basis, at, ended := participation.End()
	if !ended {
		return participationEndColumns{}
	}
	kindName, basisValue, endMoment := kind.String(), basis.String(), at.UTC()
	return participationEndColumns{kind: &kindName, basis: &basisValue, at: &endMoment}
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
	supersedes *string
	voided     bool
}

// participationSpecFrom 逐列走各自的构造门装回，不按列直接拼结构体——构造门是坏行的
// 第一道拦截，绕过它等于把库当成可信来源。
func participationSpecFrom(row participationRow) (domain.RehydrateParticipationSpec, error) {
	spec := domain.RehydrateParticipationSpec{EnteredAt: row.enteredAt.UTC(), Voided: row.voided}

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
	if row.supersedes != nil {
		if spec.Supersedes, err = domain.NewParticipationBasisReference(*row.supersedes); err != nil {
			return domain.RehydrateParticipationSpec{}, err
		}
	}
	return spec, nil
}

// participationEntryKindFrom 要与领域 ParticipationEntryKind 的每个词逐一对上：迁移的 CHECK 收下的词这里不认，
// 那一名成员落得进、读不回，整段的终止、交接与进段都随之读不回段而停住。
func participationEntryKindFrom(raw string) (domain.ParticipationEntryKind, error) {
	switch raw {
	case domain.EnteredByOffsitePickup.String():
		return domain.EnteredByOffsitePickup, nil
	case domain.EnteredByTransportHandover.String():
		return domain.EnteredByTransportHandover, nil
	case domain.EnteredByCarrierFirstEffectivePickup.String():
		return domain.EnteredByCarrierFirstEffectivePickup, nil
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
