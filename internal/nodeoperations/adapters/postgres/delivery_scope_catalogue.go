package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
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

// catalogueFilterArgs 把筛选译成可空的四个参数：零值传 NULL，语句里「参数为空即不设这一界」。
func catalogueFilterArgs(filter ports.DeliveryScopeCatalogueFilter) (node *string, from, until *time.Time, documentNumber *string) {
	if value := filter.Node.String(); value != "" {
		node = &value
	}
	if !filter.From.IsZero() {
		from = &filter.From
	}
	if !filter.Until.IsZero() {
		until = &filter.Until
	}
	if value := filter.DocumentNumber.String(); value != "" {
		documentNumber = &value
	}
	return node, from, until, documentNumber
}

const preAdviceRegister = "node_operations.delivery_pre_advice"

// preAdvicePages 把 ports.DeliveryPreAdvicePages 落到预报册的列上；q 在预报标识、单号、客户账户与节点上做字面包含（没有单号的行只靠
// 另几列命中）。
var preAdvicePages = ports.DeliveryPreAdvicePages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"expectedArrivalAt": "expected_arrival_at"},
	Identity: []string{"pre_advice_id"},
	Keyword:  []string{"pre_advice_id", "document_number", "customer_account_id", "node_ref"},
})

// listedPreAdvices 是预报册在选择器筛选下的定义：四个参数由 catalogueFilterArgs 给出，为空即不设那一界。
const listedPreAdvices = `tenant_id = $1
	AND ($2::text IS NULL OR node_ref = $2)
	AND ($3::timestamptz IS NULL OR expected_arrival_at >= $3)
	AND ($4::timestamptz IS NULL OR expected_arrival_at < $4)
	AND ($5::text IS NULL OR document_number = $5)`

// listedPreAdvice 是取页时的一行：预计送达与标识另从排序列上扫回，游标位置用它们而不用还原后领域对象里的副本。
type listedPreAdvice struct {
	state domain.DeliveryPreAdviceState
	at    time.Time
	id    string
}

// ListDeliveryPreAdvices 列预报的一页，缺省按预计送达倒序、同刻按标识倒序；再按这一页的标识取各自的撤回。
func (catalogue *DeliveryScopeCatalogue) ListDeliveryPreAdvices(
	ctx context.Context,
	tenant domain.TenantID,
	filter ports.DeliveryScopeCatalogueFilter,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[domain.DeliveryPreAdviceState], error) {
	fail := func(err error) (cataloguepage.Result[domain.DeliveryPreAdviceState], error) {
		return cataloguepage.Result[domain.DeliveryPreAdviceState]{}, fmt.Errorf("list delivery pre-advices: %w", err)
	}
	if limit <= 0 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	node, from, until, documentNumber := catalogueFilterArgs(filter)
	plan, err := preAdvicePages.Plan(query, listedPreAdvices,
		[]any{tenant.String(), node, from, until, documentNumber}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(preAdviceRegister), plan.Args...).Scan(&total); err != nil {
		return fail(fmt.Errorf("count: %w", err))
	}
	rows, err := querier.Query(ctx,
		plan.PageSQL(preAdviceColumns+`, expected_arrival_at, pre_advice_id`, preAdviceRegister), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	fetched := make([]listedPreAdvice, 0, limit+1)
	for rows.Next() {
		var row listedPreAdvice
		preAdvice, _, err := scanPreAdvice(trailingRow{row: rows, tail: []any{&row.at, &row.id}}, tenant)
		if err != nil {
			rows.Close()
			return fail(err)
		}
		row.state = domain.DeliveryPreAdviceState{PreAdvice: preAdvice}
		fetched = append(fetched, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	page, err := cataloguepage.Finish(query, fetched, limit, total,
		func(row listedPreAdvice, _ string) cataloguepage.Position {
			return cataloguepage.Position{Value: cataloguepage.FormatInstant(row.at), Identity: []string{row.id}}
		})
	if err != nil {
		return fail(err)
	}
	listed := cataloguepage.Result[domain.DeliveryPreAdviceState]{
		Rows: make([]domain.DeliveryPreAdviceState, len(page.Rows)), Next: page.Next, Total: page.Total,
	}
	if len(page.Rows) == 0 {
		return listed, nil
	}
	ids := make([]string, len(page.Rows))
	for index, row := range page.Rows {
		listed.Rows[index] = row.state
		ids[index] = row.state.PreAdvice.ID().String()
	}

	withdrawals, err := querier.Query(ctx,
		`SELECT pre_advice_id, customer_account_id, source_channel, source_request_key, content_digest, withdrawn_at
		   FROM node_operations.delivery_pre_advice_withdrawal
		  WHERE tenant_id = $1 AND pre_advice_id = ANY($2)`,
		tenant.String(), ids,
	)
	if err != nil {
		return fail(fmt.Errorf("withdrawals: %w", err))
	}
	defer withdrawals.Close()
	byPreAdvice := make(map[string]domain.DeliveryPreAdviceWithdrawal, len(ids))
	for withdrawals.Next() {
		withdrawal, _, err := scanWithdrawal(withdrawals, tenant)
		if err != nil {
			return fail(fmt.Errorf("withdrawals: %w", err))
		}
		byPreAdvice[withdrawal.PreAdvice().String()] = withdrawal
	}
	if err := withdrawals.Err(); err != nil {
		return fail(fmt.Errorf("withdrawals: %w", err))
	}
	for index := range listed.Rows {
		listed.Rows[index].Withdrawal = byPreAdvice[ids[index]]
	}
	return listed, nil
}

// FindDeliveryPreAdvice 就是预报册自己的按标识读。
func (catalogue *DeliveryScopeCatalogue) FindDeliveryPreAdvice(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.DeliveryPreAdviceID,
) (domain.DeliveryPreAdviceState, bool, error) {
	return catalogue.preAdvices.FindDeliveryPreAdvice(ctx, tenant, id)
}

const handoverNoteRegister = "node_operations.delivery_handover_note AS note"

// handoverNotePages 把 ports.DeliveryHandoverNotePages 落到交接单册（别名 note）的列上；q 在单据标识、单号、节点与交付方上做字面
// 包含（没有单号、未登交付方的行只靠另几列命中）。
var handoverNotePages = ports.DeliveryHandoverNotePages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"openedAt": "note.opened_at"},
	Identity: []string{"note.note_id"},
	Keyword:  []string{"note.note_id", "note.document_number", "note.node_ref", "note.delivering_party"},
})

// listedHandoverNotes 是交接单册在选择器筛选下的定义，参数同 listedPreAdvices。
const listedHandoverNotes = `note.tenant_id = $1
	AND ($2::text IS NULL OR note.node_ref = $2)
	AND ($3::timestamptz IS NULL OR note.opened_at >= $3)
	AND ($4::timestamptz IS NULL OR note.opened_at < $4)
	AND ($5::text IS NULL OR note.document_number = $5)`

// listedHandoverNote 是取页时的一行：开立时刻与标识另从排序列上扫回，理由同 listedPreAdvice。
type listedHandoverNote struct {
	row ports.DeliveryHandoverNoteCatalogueRow
	at  time.Time
	id  string
}

// ListDeliveryHandoverNotes 列交接单的一页，缺省按开立倒序、同刻按标识倒序，每行另带名下成员数；再按这一页的标识取各自的关闭与结论。
func (catalogue *DeliveryScopeCatalogue) ListDeliveryHandoverNotes(
	ctx context.Context,
	tenant domain.TenantID,
	filter ports.DeliveryScopeCatalogueFilter,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.DeliveryHandoverNoteCatalogueRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.DeliveryHandoverNoteCatalogueRow], error) {
		return cataloguepage.Result[ports.DeliveryHandoverNoteCatalogueRow]{}, fmt.Errorf("list delivery handover notes: %w", err)
	}
	if limit <= 0 {
		return fail(fmt.Errorf("limit must be positive, got %d", limit))
	}
	node, from, until, documentNumber := catalogueFilterArgs(filter)
	plan, err := handoverNotePages.Plan(query, listedHandoverNotes,
		[]any{tenant.String(), node, from, until, documentNumber}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(handoverNoteRegister), plan.Args...).Scan(&total); err != nil {
		return fail(fmt.Errorf("count: %w", err))
	}
	rows, err := querier.Query(ctx, plan.PageSQL(
		`note.note_id, note.node_ref, note.delivering_party, note.claimed_account_ids, note.opened_at,
		 note.document_number, note.document_number_absence,
		 (SELECT count(*) FROM node_operations.delivery_handover_member AS member
		   WHERE member.tenant_id = note.tenant_id AND member.note_id = note.note_id),
		 note.opened_at, note.note_id`,
		handoverNoteRegister), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	fetched := make([]listedHandoverNote, 0, limit+1)
	for rows.Next() {
		var (
			listed listedHandoverNote
			count  int64
		)
		note, _, err := scanHandoverNote(trailingRow{row: rows, tail: []any{&count, &listed.at, &listed.id}}, tenant)
		if err != nil {
			rows.Close()
			return fail(err)
		}
		listed.row = ports.DeliveryHandoverNoteCatalogueRow{State: domain.DeliveryHandoverNoteState{Note: note}, MemberCount: count}
		fetched = append(fetched, listed)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	page, err := cataloguepage.Finish(query, fetched, limit, total,
		func(listed listedHandoverNote, _ string) cataloguepage.Position {
			return cataloguepage.Position{Value: cataloguepage.FormatInstant(listed.at), Identity: []string{listed.id}}
		})
	if err != nil {
		return fail(err)
	}
	result := cataloguepage.Result[ports.DeliveryHandoverNoteCatalogueRow]{
		Rows: make([]ports.DeliveryHandoverNoteCatalogueRow, len(page.Rows)), Next: page.Next, Total: page.Total,
	}
	if len(page.Rows) == 0 {
		return result, nil
	}
	ids := make([]string, len(page.Rows))
	for index, listed := range page.Rows {
		result.Rows[index] = listed.row
		ids[index] = listed.row.State.Note.ID().String()
	}

	closures, bindings, err := catalogue.noteEvidence(ctx, querier, tenant, ids)
	if err != nil {
		return fail(err)
	}
	for index := range result.Rows {
		result.Rows[index].State.Closure = closures[ids[index]]
		result.Rows[index].State.Bindings = bindings[ids[index]]
	}
	return result, nil
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
