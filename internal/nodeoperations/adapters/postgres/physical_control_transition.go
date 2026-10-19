package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件是节点控制转移册（ADR-0214 决定三，迁移 0017）的适配器：读写转移、答一件作业实物此刻在身的各份控制、认交接对象。

// foldedControl 是读控制时的折叠（ADR-0214 决定三）：收寄行的 control 列不改写，控制是否已转出要把转移册上的转出折进去读。折叠由
// 迁移 0017 的 SQL 函数做，凡读收寄控制的语句都经这一格读，收寄按键读回、认当前收寄与查阅目录才答得一致。
const foldedControl = `node_operations.control_with_transfer_out(tenant_id, control)`

// PhysicalControlTransitions 实现 ports.PhysicalControlRegister 与 ports.HandoverObjectResolver：两者都读收寄册与转移册，生产上是
// 同一只适配器。
type PhysicalControlTransitions struct {
	db *bentopg.DB
}

var (
	_ ports.PhysicalControlRegister = (*PhysicalControlTransitions)(nil)
	_ ports.HandoverObjectResolver  = (*PhysicalControlTransitions)(nil)
)

func NewPhysicalControlTransitions(db *bentopg.DB) (*PhysicalControlTransitions, error) {
	if db == nil {
		return nil, fmt.Errorf("node operations postgres: db is nil")
	}
	return &PhysicalControlTransitions{db: db}, nil
}

// transitionColumns 与 scanTransition 的扫描顺序是同一份约定，两处一起改。
const transitionColumns = `handover_object, handover_scope, handover_version, direction, outcome, reason,
		        unit, node, control_kind, control_basis, control_established_at,
		        transfer_reference, transferred_at, occurred_at, recorded_at`

// FindTransition 按主键取一行。走 ReadExecutor：事务内读得到本事务刚落的那一行。
func (register *PhysicalControlTransitions) FindTransition(
	ctx context.Context,
	tenant domain.TenantID,
	handover domain.AuthoritativeHandoverReference,
	direction domain.ControlTransitionDirection,
) (ports.ControlTransitionRecord, bool, error) {
	querier, err := register.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ControlTransitionRecord{}, false, fmt.Errorf("find control transition: %w", err)
	}
	record, err := scanTransition(querier.QueryRow(ctx,
		`SELECT `+transitionColumns+`
		   FROM node_operations.physical_control_transition
		  WHERE tenant_id = $1
		    AND handover_object = $2
		    AND handover_scope = $3
		    AND handover_version = $4
		    AND direction = $5`,
		tenant.String(), handover.Object(), handover.Scope(), handover.Version(), direction.String(),
	), tenant)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ControlTransitionRecord{}, false, nil
	}
	if err != nil {
		return ports.ControlTransitionRecord{}, false, fmt.Errorf("find control transition: %w", err)
	}
	return record, true, nil
}

// FindAdvancedInHandoverChain 按（租户，对象，范围，方向）取推进了的那一行，任一版，走 physical_control_transition_advanced_by_chain。
// 读到两行报错：一条更正链上推进过两次，是编排的锁或链检查被绕过了，任取一行会把另一次藏起来。
func (register *PhysicalControlTransitions) FindAdvancedInHandoverChain(
	ctx context.Context,
	tenant domain.TenantID,
	handover domain.AuthoritativeHandoverReference,
	direction domain.ControlTransitionDirection,
) (ports.ControlTransitionRecord, bool, error) {
	querier, err := register.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ControlTransitionRecord{}, false, fmt.Errorf("find advanced transition in handover chain: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT `+transitionColumns+`
		   FROM node_operations.physical_control_transition
		  WHERE tenant_id = $1
		    AND handover_object = $2
		    AND handover_scope = $3
		    AND direction = $4
		    AND outcome = 'ADVANCED'
		  LIMIT 2`,
		tenant.String(), handover.Object(), handover.Scope(), direction.String(),
	)
	if err != nil {
		return ports.ControlTransitionRecord{}, false, fmt.Errorf("find advanced transition in handover chain: %w", err)
	}
	defer rows.Close()
	var found []ports.ControlTransitionRecord
	for rows.Next() {
		record, err := scanTransition(rows, tenant)
		if err != nil {
			return ports.ControlTransitionRecord{}, false, fmt.Errorf("find advanced transition in handover chain: %w", err)
		}
		found = append(found, record)
	}
	if err := rows.Err(); err != nil {
		return ports.ControlTransitionRecord{}, false, fmt.Errorf("find advanced transition in handover chain: %w", err)
	}
	switch len(found) {
	case 0:
		return ports.ControlTransitionRecord{}, false, nil
	case 1:
		return found[0], true, nil
	default:
		return ports.ControlTransitionRecord{}, false, fmt.Errorf(
			"find advanced transition in handover chain: object %q scope %q advanced more than once", handover.Object(), handover.Scope())
	}
}

// FindActiveControls 先取实物锁，再读这件实物此刻在身的各份控制。
//
// 锁空间名与键口径必须与 Receptions.FindCurrentJudgmentsAtNode 逐字相同：转出与同一件实物的收寄要串在同一把锁上，否则一次送站
// 在转出提交之前读到仍在身的控制，就会并进一条正被转出的收寄。走 RequireExecutor，理由同那一问：锁随事务释放，没有事务时锁在
// 取锁那条语句结束就放掉。
//
// 两处来源都经同一个折叠读：收寄行上的控制（各条判断链照抄同一份，按控制依据去重），与转移册上转入成立的控制（票
// demo-chain-gaps/62 起才有行）。折叠之后只留在身的。
func (register *PhysicalControlTransitions) FindActiveControls(
	ctx context.Context,
	tenant domain.TenantID,
	unit domain.HandlingUnitID,
) ([]domain.PhysicalControl, error) {
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("find active controls: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('node_operations.reception.handling_unit'), hashtext($1 || chr(31) || $2))`,
		tenant.String(), unit.String(),
	); err != nil {
		return nil, fmt.Errorf("find active controls: 取实物锁：%w", err)
	}
	rows, err := executor.Query(ctx,
		`SELECT `+foldedControl+`
		   FROM node_operations.reception
		  WHERE tenant_id = $1
		    AND intake IS NOT NULL
		    AND (intake ->> 'unit') = $2
		 UNION ALL
		 SELECT node_operations.control_with_transfer_out(t.tenant_id, jsonb_build_object(
		            'unit', t.unit, 'node', t.node, 'kind', t.control_kind, 'basis', t.control_basis,
		            'establishedAt', t.control_established_at))
		   FROM node_operations.physical_control_transition AS t
		  WHERE t.tenant_id = $1
		    AND t.unit = $2
		    AND t.direction = 'IN'
		    AND t.outcome = 'ADVANCED'`,
		tenant.String(), unit.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("find active controls: %w", err)
	}
	defer rows.Close()
	active := []domain.PhysicalControl{}
	seen := map[string]bool{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("find active controls: %w", err)
		}
		control, err := rebuildControl(tenant, raw)
		if err != nil {
			return nil, fmt.Errorf("find active controls: %w", err)
		}
		if !control.Active() || seen[control.Basis().String()] {
			continue
		}
		seen[control.Basis().String()] = true
		active = append(active, control)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find active controls: %w", err)
	}
	return active, nil
}

// Save 落一行。撞主键答已在册（ON CONFLICT 只认主键，理由同 Receptions.Save：撞键的 INSERT 会把整个事务打进中止态，而编排拿到
// 已在册还要在同一个事务里读回原行作答）。撞「一份控制至多转出一次」那本部分唯一索引照常报错、不形成答案：同一份控制被另一版交接
// 转出了，读回的那一行不是这一版的答案。走 RequireExecutor：转出与消费门的已处理同笔提交。
func (register *PhysicalControlTransitions) Save(
	ctx context.Context,
	record ports.ControlTransitionRecord,
) (ports.ControlTransitionSaveOutcome, error) {
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ControlTransitionSaveOutcomeInvalid, fmt.Errorf("save control transition: %w", err)
	}
	transition := record.Transition
	handover := transition.Handover()
	var (
		reason, unit, node, kind, basis, reference *string
		establishedAt, transferredAt               *time.Time
	)
	reason = nameColumn(transition.Reason().String())
	if handlingUnit, present := transition.Unit(); present {
		unit = nameColumn(handlingUnit.String())
	}
	if control, present := transition.Control(); present {
		node = nameColumn(control.Node().String())
		kind = nameColumn(control.EstablishmentKind().String())
		basis = nameColumn(control.Basis().String())
		established := control.EstablishedAt().UTC()
		establishedAt = &established
		if releasedBy, releasedAt, released := control.Release(); released {
			reference = nameColumn(releasedBy.String())
			releasedUTC := releasedAt.UTC()
			transferredAt = &releasedUTC
		}
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO node_operations.physical_control_transition
			(tenant_id, handover_object, handover_scope, handover_version, direction, outcome, reason,
			 unit, node, control_kind, control_basis, control_established_at,
			 transfer_reference, transferred_at, occurred_at, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		 ON CONFLICT (tenant_id, handover_object, handover_scope, handover_version, direction) DO NOTHING`,
		transition.TenantID().String(), handover.Object(), handover.Scope(), handover.Version(),
		transition.Direction().String(), transition.Outcome().String(), reason,
		unit, node, kind, basis, establishedAt,
		reference, transferredAt, transition.OccurredAt().UTC(), record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.ControlTransitionSaveOutcomeInvalid, fmt.Errorf("save control transition: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ControlTransitionAlreadyRecorded, nil
	}
	return ports.ControlTransitionSaved, nil
}

// ResolveHandoverObject 先问对象是不是本租户的集运单元，不是再按正式包裹身份认（ADR-0214 决定二）：两个号同串时按集运单元判。
//
// 按包裹认只读各条判断链的当前有效版本——「没有一行指名它为前版」，走 reception_predecessor_unique——上的关联，走
// reception_intake_association_idx。被重核、冲突或处置接续过的那一版上的关联不再算：一码两物给先到那件追加的冲突版不带关联，这件
// 实物于是对不上，转出停在入账，不拿一件身份正在冲突的实物替那件包裹交走。走 ReadExecutor，不取包裹锁：收寄的锁序是先实物后包裹，
// 这里先取包裹锁再取实物锁会与收寄互等；认出之后编排取实物锁再读控制。
func (register *PhysicalControlTransitions) ResolveHandoverObject(
	ctx context.Context,
	tenant domain.TenantID,
	handover domain.AuthoritativeHandoverReference,
) (ports.HandoverObjectResolution, error) {
	querier, err := register.db.ReadExecutor(ctx)
	if err != nil {
		return ports.HandoverObjectResolution{}, fmt.Errorf("resolve handover object: %w", err)
	}
	var consolidation bool
	if err := querier.QueryRow(ctx,
		`SELECT EXISTS (
		     SELECT 1 FROM node_operations.consolidation_unit WHERE tenant_id = $1 AND unit_id = $2)`,
		tenant.String(), handover.Object(),
	).Scan(&consolidation); err != nil {
		return ports.HandoverObjectResolution{}, fmt.Errorf("resolve handover object: %w", err)
	}
	if consolidation {
		return ports.HandoverObjectResolution{Kind: ports.HandoverObjectConsolidationUnit}, nil
	}
	rows, err := querier.Query(ctx,
		`SELECT DISTINCT r.intake ->> 'unit'
		   FROM node_operations.reception AS r
		  WHERE r.tenant_id = $1
		    AND r.intake IS NOT NULL
		    AND (r.intake ->> 'association') = $2
		    AND NOT EXISTS (
		        SELECT 1
		          FROM node_operations.reception AS successor
		         WHERE successor.tenant_id = r.tenant_id
		           AND successor.predecessor_version = r.judgment_version)
		  ORDER BY 1`,
		tenant.String(), handover.Object(),
	)
	if err != nil {
		return ports.HandoverObjectResolution{}, fmt.Errorf("resolve handover object: %w", err)
	}
	defer rows.Close()
	resolution := ports.HandoverObjectResolution{Kind: ports.HandoverObjectParcel, Units: []domain.HandlingUnitID{}}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return ports.HandoverObjectResolution{}, fmt.Errorf("resolve handover object: %w", err)
		}
		unit, err := domain.NewHandlingUnitID(raw)
		if err != nil {
			return ports.HandoverObjectResolution{}, fmt.Errorf("resolve handover object: %w", err)
		}
		resolution.Units = append(resolution.Units, unit)
	}
	if err := rows.Err(); err != nil {
		return ports.HandoverObjectResolution{}, fmt.Errorf("resolve handover object: %w", err)
	}
	return resolution, nil
}

// scanTransition 把一行读回成记录：控制经领域构造函数重建、转出经 TransferOut 重放，整行再过 RehydrateControlTransition 的形状门。
func scanTransition(row pgx.Row, tenant domain.TenantID) (ports.ControlTransitionRecord, error) {
	var (
		object, scope, version, directionName, outcomeName  string
		reasonName, unitName, nodeName, kindName, basisName *string
		establishedAt, transferredAt                        *time.Time
		referenceName                                       *string
		occurredAt, recordedAt                              time.Time
	)
	if err := row.Scan(&object, &scope, &version, &directionName, &outcomeName, &reasonName,
		&unitName, &nodeName, &kindName, &basisName, &establishedAt,
		&referenceName, &transferredAt, &occurredAt, &recordedAt); err != nil {
		return ports.ControlTransitionRecord{}, err
	}
	handover, err := domain.NewAuthoritativeHandoverReference(object, scope, version)
	if err != nil {
		return ports.ControlTransitionRecord{}, err
	}
	spec := domain.ControlTransitionSpec{TenantID: tenant, Handover: handover, OccurredAt: occurredAt}
	if spec.Direction, err = domain.ParseControlTransitionDirection(directionName); err != nil {
		return ports.ControlTransitionRecord{}, err
	}
	if spec.Outcome, err = domain.ParseControlTransitionOutcome(outcomeName); err != nil {
		return ports.ControlTransitionRecord{}, err
	}
	if reasonName != nil {
		if spec.Reason, err = domain.ParseControlNotAdvancedReason(*reasonName); err != nil {
			return ports.ControlTransitionRecord{}, err
		}
	}
	if unitName != nil {
		if spec.Unit, err = domain.NewHandlingUnitID(*unitName); err != nil {
			return ports.ControlTransitionRecord{}, err
		}
	}
	if nodeName != nil {
		row := controlRow{Unit: spec.Unit.String(), Node: *nodeName}
		if kindName != nil {
			row.Kind = *kindName
		}
		if basisName != nil {
			row.Basis = *basisName
		}
		if establishedAt != nil {
			row.EstablishedAt = establishedAt.UTC()
		}
		if referenceName != nil {
			row.ReleasedBy = *referenceName
			row.ReleasedAt = transferredAt
		}
		// 经收寄控制同一份行模型与重建读回：转移册上的控制与收寄行上的控制是同一个东西，重建与转出重放只有一处。
		raw, err := json.Marshal(row)
		if err != nil {
			return ports.ControlTransitionRecord{}, err
		}
		if spec.Control, err = rebuildControl(tenant, raw); err != nil {
			return ports.ControlTransitionRecord{}, err
		}
	}
	transition, err := domain.RehydrateControlTransition(spec)
	if err != nil {
		return ports.ControlTransitionRecord{}, err
	}
	return ports.ControlTransitionRecord{Transition: transition, RecordedAt: recordedAt.UTC()}, nil
}
