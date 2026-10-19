package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// DeliveryScopeCatalogue 实现 ports.DeliveryScopeCatalogueRead：送站预报册与送站交接单册的运营查阅读（迁移 0013；票
// ps-external-mark-relations/13）。
//
// 每一行都经两本册写侧同一组还原门读回（scanPreAdvice、scanHandoverNote 一族），不另立一份照行转写：范围成立是领域按结论派生的，
// 读面拿到的必须是领域对象；按标识读一份就是两本册自己的读口，不抄第二份语句。列表的撤回、关闭与结论按本页那几个标识再取一次，
// 不逐行回查。
//
// 所有语句显式携带租户条件（ADR-0003）；读走 ReadExecutor。
type DeliveryScopeCatalogue struct {
	db         *bentopg.DB
	preAdvices *DeliveryPreAdvices
	notes      *DeliveryHandoverNotes
}

var _ ports.DeliveryScopeCatalogueRead = (*DeliveryScopeCatalogue)(nil)

func NewDeliveryScopeCatalogue(db *bentopg.DB) (*DeliveryScopeCatalogue, error) {
	preAdvices, err := NewDeliveryPreAdvices(db)
	if err != nil {
		return nil, err
	}
	notes, err := NewDeliveryHandoverNotes(db)
	if err != nil {
		return nil, err
	}
	return &DeliveryScopeCatalogue{db: db, preAdvices: preAdvices, notes: notes}, nil
}

// catalogueFilterArgs 把筛选译成可空的三个参数：零值传 NULL，语句里「参数为空即不设这一界」。
func catalogueFilterArgs(filter ports.DeliveryScopeCatalogueFilter) (node *string, from, until *time.Time) {
	if value := filter.Node.String(); value != "" {
		node = &value
	}
	if !filter.From.IsZero() {
		from = &filter.From
	}
	if !filter.Until.IsZero() {
		until = &filter.Until
	}
	return node, from, until
}

// ListDeliveryPreAdvices 按预计送达倒序列预报，同刻按标识；再按这一页的标识取各自的撤回。
func (catalogue *DeliveryScopeCatalogue) ListDeliveryPreAdvices(
	ctx context.Context,
	tenant domain.TenantID,
	filter ports.DeliveryScopeCatalogueFilter,
	limit int,
) ([]domain.DeliveryPreAdviceState, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list delivery pre-advices: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list delivery pre-advices: %w", err)
	}
	node, from, until := catalogueFilterArgs(filter)
	rows, err := querier.Query(ctx,
		`SELECT `+preAdviceColumns+`
		   FROM node_operations.delivery_pre_advice
		  WHERE tenant_id = $1
		    AND ($2::text IS NULL OR node_ref = $2)
		    AND ($3::timestamptz IS NULL OR expected_arrival_at >= $3)
		    AND ($4::timestamptz IS NULL OR expected_arrival_at < $4)
		  ORDER BY expected_arrival_at DESC, pre_advice_id
		  LIMIT $5`,
		tenant.String(), node, from, until, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list delivery pre-advices: %w", err)
	}
	var (
		states []domain.DeliveryPreAdviceState
		ids    []string
	)
	for rows.Next() {
		preAdvice, _, err := scanPreAdvice(rows, tenant)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("list delivery pre-advices: %w", err)
		}
		states = append(states, domain.DeliveryPreAdviceState{PreAdvice: preAdvice})
		ids = append(ids, preAdvice.ID().String())
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list delivery pre-advices: %w", err)
	}
	if len(ids) == 0 {
		return states, nil
	}

	withdrawals, err := querier.Query(ctx,
		`SELECT pre_advice_id, customer_account_id, source_channel, source_request_key, content_digest, withdrawn_at
		   FROM node_operations.delivery_pre_advice_withdrawal
		  WHERE tenant_id = $1 AND pre_advice_id = ANY($2)`,
		tenant.String(), ids,
	)
	if err != nil {
		return nil, fmt.Errorf("list delivery pre-advices: withdrawals: %w", err)
	}
	defer withdrawals.Close()
	byPreAdvice := make(map[string]domain.DeliveryPreAdviceWithdrawal, len(ids))
	for withdrawals.Next() {
		withdrawal, _, err := scanWithdrawal(withdrawals, tenant)
		if err != nil {
			return nil, fmt.Errorf("list delivery pre-advices: withdrawals: %w", err)
		}
		byPreAdvice[withdrawal.PreAdvice().String()] = withdrawal
	}
	if err := withdrawals.Err(); err != nil {
		return nil, fmt.Errorf("list delivery pre-advices: withdrawals: %w", err)
	}
	for index := range states {
		states[index].Withdrawal = byPreAdvice[states[index].PreAdvice.ID().String()]
	}
	return states, nil
}

// FindDeliveryPreAdvice 就是预报册自己的按标识读。
func (catalogue *DeliveryScopeCatalogue) FindDeliveryPreAdvice(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.DeliveryPreAdviceID,
) (domain.DeliveryPreAdviceState, bool, error) {
	return catalogue.preAdvices.FindDeliveryPreAdvice(ctx, tenant, id)
}

// ListDeliveryHandoverNotes 按开立倒序列交接单，同刻按标识，每行另带名下成员数；再按这一页的标识取各自的关闭与结论。
func (catalogue *DeliveryScopeCatalogue) ListDeliveryHandoverNotes(
	ctx context.Context,
	tenant domain.TenantID,
	filter ports.DeliveryScopeCatalogueFilter,
	limit int,
) ([]ports.DeliveryHandoverNoteCatalogueRow, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("list delivery handover notes: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list delivery handover notes: %w", err)
	}
	node, from, until := catalogueFilterArgs(filter)
	rows, err := querier.Query(ctx,
		`SELECT note.note_id, note.node_ref, note.delivering_party, note.claimed_account_ids, note.opened_at,
		        (SELECT count(*) FROM node_operations.delivery_handover_member AS member
		          WHERE member.tenant_id = note.tenant_id AND member.note_id = note.note_id)
		   FROM node_operations.delivery_handover_note AS note
		  WHERE note.tenant_id = $1
		    AND ($2::text IS NULL OR note.node_ref = $2)
		    AND ($3::timestamptz IS NULL OR note.opened_at >= $3)
		    AND ($4::timestamptz IS NULL OR note.opened_at < $4)
		  ORDER BY note.opened_at DESC, note.note_id
		  LIMIT $5`,
		tenant.String(), node, from, until, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list delivery handover notes: %w", err)
	}
	var (
		list []ports.DeliveryHandoverNoteCatalogueRow
		ids  []string
	)
	for rows.Next() {
		var count int64
		note, _, err := scanHandoverNote(trailingRow{row: rows, tail: []any{&count}}, tenant)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("list delivery handover notes: %w", err)
		}
		list = append(list, ports.DeliveryHandoverNoteCatalogueRow{State: domain.DeliveryHandoverNoteState{Note: note}, MemberCount: count})
		ids = append(ids, note.ID().String())
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list delivery handover notes: %w", err)
	}
	if len(ids) == 0 {
		return list, nil
	}

	closures, bindings, err := catalogue.noteEvidence(ctx, querier, tenant, ids)
	if err != nil {
		return nil, fmt.Errorf("list delivery handover notes: %w", err)
	}
	for index := range list {
		id := list[index].State.Note.ID().String()
		list[index].State.Closure = closures[id]
		list[index].State.Bindings = bindings[id]
	}
	return list, nil
}

// noteEvidence 按一组交接单标识取各自的关闭与结论，经两本册写侧同一组还原门读回。
func (catalogue *DeliveryScopeCatalogue) noteEvidence(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	ids []string,
) (map[string]domain.DeliveryHandoverClosure, map[string][]domain.DeliveryHandoverBinding, error) {
	closureRows, err := querier.Query(ctx,
		`SELECT note_id, fact_id, closed_at FROM node_operations.delivery_handover_closure
		  WHERE tenant_id = $1 AND note_id = ANY($2)`,
		tenant.String(), ids,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("closures: %w", err)
	}
	closures := make(map[string]domain.DeliveryHandoverClosure, len(ids))
	for closureRows.Next() {
		closure, _, err := scanHandoverClosure(closureRows, tenant)
		if err != nil {
			closureRows.Close()
			return nil, nil, fmt.Errorf("closures: %w", err)
		}
		closures[closure.Note().String()] = closure
	}
	closureRows.Close()
	if err := closureRows.Err(); err != nil {
		return nil, nil, fmt.Errorf("closures: %w", err)
	}

	bindingRows, err := querier.Query(ctx,
		`SELECT `+bindingColumns+` FROM node_operations.delivery_handover_binding
		  WHERE tenant_id = $1 AND note_id = ANY($2)
		  ORDER BY note_id, customer_account_id`,
		tenant.String(), ids,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("bindings: %w", err)
	}
	defer bindingRows.Close()
	bindings := make(map[string][]domain.DeliveryHandoverBinding, len(ids))
	for bindingRows.Next() {
		binding, _, err := scanHandoverBinding(bindingRows, tenant)
		if err != nil {
			return nil, nil, fmt.Errorf("bindings: %w", err)
		}
		bindings[binding.Note().String()] = append(bindings[binding.Note().String()], binding)
	}
	if err := bindingRows.Err(); err != nil {
		return nil, nil, fmt.Errorf("bindings: %w", err)
	}
	return closures, bindings, nil
}

// FindDeliveryHandoverNoteDetail 是交接单册自己的按标识读，加名下成员。
func (catalogue *DeliveryScopeCatalogue) FindDeliveryHandoverNoteDetail(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.DeliveryHandoverNoteID,
) (ports.DeliveryHandoverNoteDetail, bool, error) {
	state, found, err := catalogue.notes.FindDeliveryHandoverNote(ctx, tenant, id)
	if err != nil || !found {
		return ports.DeliveryHandoverNoteDetail{}, found, err
	}
	members, err := catalogue.notes.ListDeliveryHandoverMembers(ctx, tenant, id)
	if err != nil {
		return ports.DeliveryHandoverNoteDetail{}, false, err
	}
	return ports.DeliveryHandoverNoteDetail{State: state, Members: members}, true, nil
}

// trailingRow 把一行末尾多出的几列接到 tail 上，前面几列仍交给只认那几列的还原门：列表多带一格计数，也不另抄一份还原。
type trailingRow struct {
	row  pgx.Row
	tail []any
}

func (row trailingRow) Scan(dest ...any) error {
	return row.row.Scan(append(dest, row.tail...)...)
}

// unitScopeBasisFrom 还原复核目录待识别行的两样依据（ADR-0212 决定三、七）：来源文档里的依据两键与这件实物最后归入的交接单。两键都空
// 即来源没带依据；只有一键、或种类认不出，让整页答不出——判法同来源文档的读回门（domain 里来源文档的还原），一份坏文档不悄悄显示成
// 「没有依据」。
func unitScopeBasisFrom(kind, identifier, lastAssigned string) (domain.UnitDeliveryScopeBasis, error) {
	var basis domain.UnitDeliveryScopeBasis
	if kind != "" || identifier != "" {
		parsed, err := domain.ParseDeliveryScopeBasisKind(kind)
		if err != nil {
			return domain.UnitDeliveryScopeBasis{}, err
		}
		if basis.Source, err = domain.NewDeliveryScopeBasisReference(parsed, identifier); err != nil {
			return domain.UnitDeliveryScopeBasis{}, err
		}
	}
	if lastAssigned != "" {
		note, err := domain.NewDeliveryHandoverNoteID(lastAssigned)
		if err != nil {
			return domain.UnitDeliveryScopeBasis{}, err
		}
		basis.LastAssignedNote = note
	}
	return basis, nil
}
