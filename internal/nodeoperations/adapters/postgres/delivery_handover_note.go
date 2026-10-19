package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// DeliveryHandoverNotes 实现 ports.DeliveryHandoverNoteRegister（迁移 0013；ADR-0212 决定三）。各写口都只插入：交接单、成员、关闭、
// 绑定结论各一张表，撞键读回回显，没有 UPDATE 或 DELETE。
//
// 关闭与加成员在交接单那一行上串行：加成员先取共享锁再看关闭，关闭取排他锁。不锁，两笔并发时加成员那一笔看不到还没提交的关闭，
// 就会在关闭之后落进一个成员，而「关闭冻结成员」正是要挡这一格。
type DeliveryHandoverNotes struct {
	db *bentopg.DB
}

func NewDeliveryHandoverNotes(db *bentopg.DB) (*DeliveryHandoverNotes, error) {
	if db == nil {
		return nil, fmt.Errorf("node operations postgres: db is nil")
	}
	return &DeliveryHandoverNotes{db: db}, nil
}

var _ ports.DeliveryHandoverNoteRegister = (*DeliveryHandoverNotes)(nil)

const handoverNoteColumns = `note_id, node_ref, delivering_party, claimed_account_ids, opened_at`

// OpenDeliveryHandoverNote 落一张交接单；同一标识已在册时回显册上那一张。
func (register *DeliveryHandoverNotes) OpenDeliveryHandoverNote(
	ctx context.Context,
	note domain.DeliveryHandoverNote,
) (domain.DeliveryHandoverNote, ports.DeliveryScopeAppendOutcome, error) {
	none := domain.DeliveryHandoverNote{}
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("open delivery handover note: %w", err)
	}
	claimed := make([]string, 0, len(note.ClaimedAccounts()))
	for _, account := range note.ClaimedAccounts() {
		claimed = append(claimed, account.String())
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO node_operations.delivery_handover_note
			(tenant_id, `+handoverNoteColumns+`)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT DO NOTHING`,
		note.TenantID().String(), note.ID().String(), note.Node().String(), note.DeliveringParty().String(), claimed, note.OpenedAt(),
	)
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("open delivery handover note: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return note, ports.DeliveryScopeAppended, nil
	}
	recorded, found, err := scanHandoverNote(executor.QueryRow(ctx,
		`SELECT `+handoverNoteColumns+` FROM node_operations.delivery_handover_note WHERE tenant_id = $1 AND note_id = $2`,
		note.TenantID().String(), note.ID().String(),
	), note.TenantID())
	if err != nil || !found {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("open delivery handover note: read back: found=%v: %w", found, err)
	}
	return recorded, ports.DeliveryScopeAlreadyRecorded, nil
}

const memberColumns = `note_id, unit_id, membership_kind, fact_id, formed_at`

// AppendDeliveryHandoverMember 落一个成员。次序：锁交接单 → 同一事实已在册就回显（关闭之后的重放照旧答已在册，那一份事实发生在关闭
// 之前） → 已关闭就拒 → 插入。
func (register *DeliveryHandoverNotes) AppendDeliveryHandoverMember(
	ctx context.Context,
	member domain.DeliveryHandoverMember,
) (domain.DeliveryHandoverMember, ports.DeliveryScopeAppendOutcome, error) {
	none := domain.DeliveryHandoverMember{}
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("append delivery handover member: %w", err)
	}
	tenant := member.TenantID()
	var closed bool
	err = executor.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM node_operations.delivery_handover_closure AS closure
		                 WHERE closure.tenant_id = note.tenant_id AND closure.note_id = note.note_id)
		   FROM node_operations.delivery_handover_note AS note
		  WHERE note.tenant_id = $1 AND note.note_id = $2
		    FOR SHARE OF note`,
		tenant.String(), member.Note().String(),
	).Scan(&closed)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, ports.DeliveryScopeTargetNotFound, nil
	}
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("append delivery handover member: lock note: %w", err)
	}
	readBack := func() (domain.DeliveryHandoverMember, bool, error) {
		return scanHandoverMember(executor.QueryRow(ctx,
			`SELECT `+memberColumns+` FROM node_operations.delivery_handover_member
			  WHERE tenant_id = $1 AND membership_kind = $2 AND fact_id = $3`,
			tenant.String(), member.Kind().String(), member.FactID(),
		), tenant)
	}
	recorded, found, err := readBack()
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("append delivery handover member: read back: %w", err)
	}
	if found {
		return recorded, ports.DeliveryScopeAlreadyRecorded, nil
	}
	if closed {
		return none, ports.DeliveryScopeNoteClosed, nil
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO node_operations.delivery_handover_member
			(tenant_id, `+memberColumns+`)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT DO NOTHING`,
		tenant.String(), member.Note().String(), member.Unit().String(), member.Kind().String(), member.FactID(), member.FormedAt(),
	)
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("append delivery handover member: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return member, ports.DeliveryScopeAppended, nil
	}
	// 并发的同一事实先一步落了：读回它。
	recorded, found, err = readBack()
	if err != nil || !found {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("append delivery handover member: read back: found=%v: %w", found, err)
	}
	return recorded, ports.DeliveryScopeAlreadyRecorded, nil
}

// CloseDeliveryHandoverNote 落一份关闭。先取交接单的排他锁，与正在加成员的事务串行。
func (register *DeliveryHandoverNotes) CloseDeliveryHandoverNote(
	ctx context.Context,
	closure domain.DeliveryHandoverClosure,
) (domain.DeliveryHandoverClosure, ports.DeliveryScopeAppendOutcome, error) {
	none := domain.DeliveryHandoverClosure{}
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("close delivery handover note: %w", err)
	}
	tenant := closure.TenantID()
	var locked string
	err = executor.QueryRow(ctx,
		`SELECT note_id FROM node_operations.delivery_handover_note WHERE tenant_id = $1 AND note_id = $2 FOR UPDATE`,
		tenant.String(), closure.Note().String(),
	).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, ports.DeliveryScopeTargetNotFound, nil
	}
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("close delivery handover note: lock note: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO node_operations.delivery_handover_closure (tenant_id, note_id, fact_id, closed_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		tenant.String(), closure.Note().String(), closure.FactID(), closure.ClosedAt(),
	)
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("close delivery handover note: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return closure, ports.DeliveryScopeAppended, nil
	}
	recorded, found, err := scanHandoverClosure(executor.QueryRow(ctx,
		`SELECT note_id, fact_id, closed_at FROM node_operations.delivery_handover_closure WHERE tenant_id = $1 AND note_id = $2`,
		tenant.String(), closure.Note().String(),
	), tenant)
	if err != nil || !found {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("close delivery handover note: read back: found=%v: %w", found, err)
	}
	return recorded, ports.DeliveryScopeAlreadyRecorded, nil
}

const bindingColumns = `note_id, customer_account_id, conclusion, route, concluder, delivering_participant, basis_ref, concluded_at`

// RecordDeliveryHandoverBinding 落一份绑定结论。插入只从「交接单在册、客户账户在声称之列」那一行选出；撞主键（这个账户已有结论）读
// 回回显，不覆盖——异结论撞键就落在这里。
func (register *DeliveryHandoverNotes) RecordDeliveryHandoverBinding(
	ctx context.Context,
	binding domain.DeliveryHandoverBinding,
) (domain.DeliveryHandoverBinding, ports.DeliveryScopeAppendOutcome, error) {
	none := domain.DeliveryHandoverBinding{}
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("record delivery handover binding: %w", err)
	}
	tenant := binding.TenantID()
	participant, basis := binding.OperationDecisionEvidence()
	tag, err := executor.Exec(ctx,
		`INSERT INTO node_operations.delivery_handover_binding
			(tenant_id, `+bindingColumns+`)
		 SELECT note.tenant_id, note.note_id, $3, $4, $5, $6, NULLIF($7, ''), NULLIF($8, ''), $9
		   FROM node_operations.delivery_handover_note AS note
		  WHERE note.tenant_id = $1 AND note.note_id = $2 AND $3 = ANY (note.claimed_account_ids)
		 ON CONFLICT DO NOTHING`,
		tenant.String(), binding.Note().String(), binding.Account().String(), binding.Conclusion().String(), binding.Route().String(),
		binding.Concluder(), participant, basis, binding.ConcludedAt(),
	)
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("record delivery handover binding: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return binding, ports.DeliveryScopeAppended, nil
	}
	recorded, found, err := scanHandoverBinding(executor.QueryRow(ctx,
		`SELECT `+bindingColumns+` FROM node_operations.delivery_handover_binding
		  WHERE tenant_id = $1 AND note_id = $2 AND customer_account_id = $3`,
		tenant.String(), binding.Note().String(), binding.Account().String(),
	), tenant)
	if err != nil {
		return none, ports.DeliveryScopeAppendInvalid, fmt.Errorf("record delivery handover binding: read back: %w", err)
	}
	if !found {
		return none, ports.DeliveryScopeTargetNotFound, nil
	}
	return recorded, ports.DeliveryScopeAlreadyRecorded, nil
}

// FindDeliveryHandoverNote 读一张交接单连同关闭与各结论。走 ReadExecutor：在事务里调时读得到本事务刚落的结论。
func (register *DeliveryHandoverNotes) FindDeliveryHandoverNote(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.DeliveryHandoverNoteID,
) (domain.DeliveryHandoverNoteState, bool, error) {
	querier, err := register.db.ReadExecutor(ctx)
	if err != nil {
		return domain.DeliveryHandoverNoteState{}, false, fmt.Errorf("find delivery handover note: %w", err)
	}
	note, found, err := scanHandoverNote(querier.QueryRow(ctx,
		`SELECT `+handoverNoteColumns+` FROM node_operations.delivery_handover_note WHERE tenant_id = $1 AND note_id = $2`,
		tenant.String(), id.String(),
	), tenant)
	if err != nil {
		return domain.DeliveryHandoverNoteState{}, false, fmt.Errorf("find delivery handover note: %w", err)
	}
	if !found {
		return domain.DeliveryHandoverNoteState{}, false, nil
	}
	state := domain.DeliveryHandoverNoteState{Note: note}
	if state.Closure, _, err = scanHandoverClosure(querier.QueryRow(ctx,
		`SELECT note_id, fact_id, closed_at FROM node_operations.delivery_handover_closure WHERE tenant_id = $1 AND note_id = $2`,
		tenant.String(), id.String(),
	), tenant); err != nil {
		return domain.DeliveryHandoverNoteState{}, false, fmt.Errorf("find delivery handover note: closure: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT `+bindingColumns+` FROM node_operations.delivery_handover_binding
		  WHERE tenant_id = $1 AND note_id = $2
		  ORDER BY customer_account_id`,
		tenant.String(), id.String(),
	)
	if err != nil {
		return domain.DeliveryHandoverNoteState{}, false, fmt.Errorf("find delivery handover note: bindings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		binding, _, err := scanHandoverBinding(rows, tenant)
		if err != nil {
			return domain.DeliveryHandoverNoteState{}, false, fmt.Errorf("find delivery handover note: bindings: %w", err)
		}
		state.Bindings = append(state.Bindings, binding)
	}
	if err := rows.Err(); err != nil {
		return domain.DeliveryHandoverNoteState{}, false, fmt.Errorf("find delivery handover note: bindings: %w", err)
	}
	return state, true, nil
}

// ListDeliveryHandoverMembers 按落册先后列一张交接单的成员。
func (register *DeliveryHandoverNotes) ListDeliveryHandoverMembers(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.DeliveryHandoverNoteID,
) ([]domain.DeliveryHandoverMember, error) {
	querier, err := register.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list delivery handover members: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT `+memberColumns+` FROM node_operations.delivery_handover_member
		  WHERE tenant_id = $1 AND note_id = $2
		  ORDER BY member_sequence`,
		tenant.String(), id.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("list delivery handover members: %w", err)
	}
	defer rows.Close()
	var members []domain.DeliveryHandoverMember
	for rows.Next() {
		member, _, err := scanHandoverMember(rows, tenant)
		if err != nil {
			return nil, fmt.Errorf("list delivery handover members: %w", err)
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list delivery handover members: %w", err)
	}
	return members, nil
}

// FindLastAssignedHandoverNote 走部分索引 delivery_handover_member_last_assignment（迁移 0013）。按业务时间而不按落册先后取「最后」：
// 离线补传的那次归入可能后落册、却发生得更早，实物最后落在哪张交接单名下看的是现场先后。
func (register *DeliveryHandoverNotes) FindLastAssignedHandoverNote(
	ctx context.Context,
	tenant domain.TenantID,
	unit domain.HandlingUnitID,
) (domain.DeliveryHandoverNoteID, bool, error) {
	querier, err := register.db.ReadExecutor(ctx)
	if err != nil {
		return domain.DeliveryHandoverNoteID{}, false, fmt.Errorf("find last assigned handover note: %w", err)
	}
	var raw string
	err = querier.QueryRow(ctx,
		`SELECT note_id FROM node_operations.delivery_handover_member
		  WHERE tenant_id = $1 AND unit_id = $2 AND membership_kind = 'ASSIGNMENT'
		  ORDER BY formed_at DESC, member_sequence DESC
		  LIMIT 1`,
		tenant.String(), unit.String(),
	).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DeliveryHandoverNoteID{}, false, nil
	}
	if err != nil {
		return domain.DeliveryHandoverNoteID{}, false, fmt.Errorf("find last assigned handover note: %w", err)
	}
	id, err := domain.NewDeliveryHandoverNoteID(raw)
	if err != nil {
		return domain.DeliveryHandoverNoteID{}, false, fmt.Errorf("find last assigned handover note: %w", err)
	}
	return id, true, nil
}

// 以下各 scan 读一行并经各自的形成门还原，读回的也过一遍不变量。

func scanHandoverNote(row pgx.Row, tenant domain.TenantID) (domain.DeliveryHandoverNote, bool, error) {
	var (
		id, node, party string
		claimed         []string
		openedAt        time.Time
	)
	err := row.Scan(&id, &node, &party, &claimed, &openedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DeliveryHandoverNote{}, false, nil
	}
	if err != nil {
		return domain.DeliveryHandoverNote{}, false, err
	}
	spec := domain.DeliveryHandoverNoteSpec{TenantID: tenant, OpenedAt: openedAt}
	if spec.ID, err = domain.NewDeliveryHandoverNoteID(id); err != nil {
		return domain.DeliveryHandoverNote{}, false, err
	}
	if spec.Node, err = domain.NewNodeReference(node); err != nil {
		return domain.DeliveryHandoverNote{}, false, err
	}
	if spec.DeliveringParty, err = domain.NewDeliveringPartyReference(party); err != nil {
		return domain.DeliveryHandoverNote{}, false, err
	}
	for _, raw := range claimed {
		account, err := domain.NewCustomerAccountReference(raw)
		if err != nil {
			return domain.DeliveryHandoverNote{}, false, err
		}
		spec.ClaimedAccounts = append(spec.ClaimedAccounts, account)
	}
	note, err := domain.OpenDeliveryHandoverNote(spec)
	if err != nil {
		return domain.DeliveryHandoverNote{}, false, err
	}
	return note, true, nil
}

func scanHandoverMember(row pgx.Row, tenant domain.TenantID) (domain.DeliveryHandoverMember, bool, error) {
	var (
		note, unit, kind, factID string
		formedAt                 time.Time
	)
	err := row.Scan(&note, &unit, &kind, &factID, &formedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DeliveryHandoverMember{}, false, nil
	}
	if err != nil {
		return domain.DeliveryHandoverMember{}, false, err
	}
	spec := domain.DeliveryHandoverMemberSpec{TenantID: tenant, FactID: factID, FormedAt: formedAt}
	if spec.Note, err = domain.NewDeliveryHandoverNoteID(note); err != nil {
		return domain.DeliveryHandoverMember{}, false, err
	}
	if spec.Unit, err = domain.NewHandlingUnitID(unit); err != nil {
		return domain.DeliveryHandoverMember{}, false, err
	}
	if spec.Kind, err = domain.ParseHandoverMembershipKind(kind); err != nil {
		return domain.DeliveryHandoverMember{}, false, err
	}
	member, err := domain.FormDeliveryHandoverMember(spec)
	if err != nil {
		return domain.DeliveryHandoverMember{}, false, err
	}
	return member, true, nil
}

func scanHandoverClosure(row pgx.Row, tenant domain.TenantID) (domain.DeliveryHandoverClosure, bool, error) {
	var (
		note, factID string
		closedAt     time.Time
	)
	err := row.Scan(&note, &factID, &closedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DeliveryHandoverClosure{}, false, nil
	}
	if err != nil {
		return domain.DeliveryHandoverClosure{}, false, err
	}
	id, err := domain.NewDeliveryHandoverNoteID(note)
	if err != nil {
		return domain.DeliveryHandoverClosure{}, false, err
	}
	closure, err := domain.FormDeliveryHandoverClosure(domain.DeliveryHandoverClosureSpec{TenantID: tenant, Note: id, FactID: factID, ClosedAt: closedAt})
	if err != nil {
		return domain.DeliveryHandoverClosure{}, false, err
	}
	return closure, true, nil
}

func scanHandoverBinding(row pgx.Row, tenant domain.TenantID) (domain.DeliveryHandoverBinding, bool, error) {
	var (
		note, account, conclusion, route, concluder string
		participant, basis                          *string
		concludedAt                                 time.Time
	)
	err := row.Scan(&note, &account, &conclusion, &route, &concluder, &participant, &basis, &concludedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DeliveryHandoverBinding{}, false, nil
	}
	if err != nil {
		return domain.DeliveryHandoverBinding{}, false, err
	}
	spec := domain.DeliveryHandoverBindingSpec{TenantID: tenant, Concluder: concluder, ConcludedAt: concludedAt}
	if participant != nil {
		spec.DeliveringParticipant = *participant
	}
	if basis != nil {
		spec.Basis = *basis
	}
	if spec.Note, err = domain.NewDeliveryHandoverNoteID(note); err != nil {
		return domain.DeliveryHandoverBinding{}, false, err
	}
	if spec.Account, err = domain.NewCustomerAccountReference(account); err != nil {
		return domain.DeliveryHandoverBinding{}, false, err
	}
	if spec.Conclusion, err = domain.ParseHandoverBindingConclusion(conclusion); err != nil {
		return domain.DeliveryHandoverBinding{}, false, err
	}
	if spec.Route, err = domain.ParseHandoverBindingRoute(route); err != nil {
		return domain.DeliveryHandoverBinding{}, false, err
	}
	binding, err := domain.FormDeliveryHandoverBinding(spec)
	if err != nil {
		return domain.DeliveryHandoverBinding{}, false, err
	}
	return binding, true, nil
}
