package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// PostIntakeManualItems 实现 ports.PostIntakeManualItems：待人工处理项及其处理结果（ADR-0341）。项只追加，结果每项至多一份、不改不删；租户条件进每条语句。
type PostIntakeManualItems struct {
	db *bentopg.DB
}

func NewPostIntakeManualItems(db *bentopg.DB) (*PostIntakeManualItems, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel shipment postgres: db is nil")
	}
	return &PostIntakeManualItems{db: db}, nil
}

var _ ports.PostIntakeManualItems = (*PostIntakeManualItems)(nil)

// RecordPostIntakeManualItems 落这份决定的待人工处理项。ON CONFLICT DO NOTHING 是重放兜底：决定本身在包裹取消边界锁里只会追加一次，项随它同笔落，
// 撞主键说明同一份决定被重放，已有的就是要落的。
func (items *PostIntakeManualItems) RecordPostIntakeManualItems(ctx context.Context, disposition domain.PostIntakeDisposition) error {
	manual := disposition.Direction().ManualOwnerRequests()
	if len(manual) == 0 {
		return nil
	}
	executor, err := items.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("record post-intake manual items: %w", err)
	}
	for _, request := range manual {
		if _, err := executor.Exec(ctx,
			`INSERT INTO parcel_shipment.post_intake_disposition_manual_item
				(tenant_id, parcel_id, decision_version, owner, action, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT DO NOTHING`,
			disposition.Tenant().String(), disposition.Parcel().String(), disposition.Version(),
			request.Owner.String(), request.Action.String(), disposition.DecidedAt().UTC(),
		); err != nil {
			return fmt.Errorf("record post-intake manual items: %w", err)
		}
	}
	return nil
}

// postIntakeManualItemSelect 取项连同所属决定的去向与目的地、包裹决定链此刻的版本、以及结果（没有结果时结果列为空）。选择列与来源
// 分开写，是因为待登记清单按 ADR-0144 取页，片段要分别接在两处。
const (
	postIntakeManualItemColumns = `i.tenant_id, i.parcel_id, i.decision_version, i.owner, i.action,
	       d.direction, d.destination_ref, d.decided_at,
	       (SELECT max(c.version) FROM parcel_shipment.post_intake_disposition_decision c
	         WHERE c.tenant_id = i.tenant_id AND c.parcel_id = i.parcel_id),
	       r.outcome, r.note, r.handled_by, r.handled_at`
	postIntakeManualItemSource = `parcel_shipment.post_intake_disposition_manual_item i
	  JOIN parcel_shipment.post_intake_disposition_decision d
	    ON d.tenant_id = i.tenant_id AND d.parcel_id = i.parcel_id AND d.version = i.decision_version
	  LEFT JOIN parcel_shipment.post_intake_disposition_manual_result r
	    ON r.tenant_id = i.tenant_id AND r.parcel_id = i.parcel_id AND r.decision_version = i.decision_version
	   AND r.owner = i.owner AND r.action = i.action`
	postIntakeManualItemSelect = `
	SELECT ` + postIntakeManualItemColumns + `
	  FROM ` + postIntakeManualItemSource
	// pendingPostIntakeManualItem 是「待登记」：项还没有结果。
	pendingPostIntakeManualItem = "i.tenant_id = $1 AND r.outcome IS NULL"
)

// postIntakeManualItemPages 把 ports.PostIntakeManualItemPages 落到列上。决定时刻落在项上的 created_at：写入时取自所属决定，与
// 答复里的 decidedAt 同值，而索引在项上；q 在包裹上做字面包含。
var postIntakeManualItemPages = ports.PostIntakeManualItemPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"decidedAt": "i.created_at"},
	Identity: []string{"i.parcel_id", "i.decision_version", "i.owner", "i.action"},
	Filters:  map[string]string{"owner": "i.owner", "action": "i.action"},
	Keyword:  []string{"i.parcel_id"},
})

// withTrailing 在一行既有的扫描目标之后多接几格：取页那一句在共用的选择列后面多取了游标要的列，扫描函数照旧只认共用那几格。
type withTrailing struct {
	row   dispositionScanner
	extra []any
}

func (scanner withTrailing) Scan(targets ...any) error {
	return scanner.row.Scan(append(targets, scanner.extra...)...)
}

// scannedPostIntakeManualItem 带着取页读回的 created_at：游标位置从它取，与 ORDER BY 用的是同一份。
type scannedPostIntakeManualItem struct {
	item      ports.PostIntakeManualItem
	createdAt time.Time
}

func (items *PostIntakeManualItems) FindPostIntakeManualItem(ctx context.Context, key domain.ManualItemKey) (ports.PostIntakeManualItem, bool, error) {
	querier, err := items.db.ReadExecutor(ctx)
	if err != nil {
		return ports.PostIntakeManualItem{}, false, fmt.Errorf("find post-intake manual item: %w", err)
	}
	item, err := scanPostIntakeManualItem(querier.QueryRow(ctx,
		postIntakeManualItemSelect+`
		 WHERE i.tenant_id = $1 AND i.parcel_id = $2 AND i.decision_version = $3 AND i.owner = $4 AND i.action = $5`,
		key.Tenant.String(), key.Parcel.String(), key.DecisionVersion, key.Owner.String(), key.Action.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.PostIntakeManualItem{}, false, nil
	}
	if err != nil {
		return ports.PostIntakeManualItem{}, false, fmt.Errorf("find post-intake manual item: %w", err)
	}
	return item, true, nil
}

// RegisterPostIntakeManualItemResult 登记结果，要环境事务。先核项在册，再 INSERT ... ON CONFLICT DO NOTHING：撞主键说明这项已有结果，读回那一份交调用方比内容。
func (items *PostIntakeManualItems) RegisterPostIntakeManualItemResult(
	ctx context.Context,
	key domain.ManualItemKey,
	result domain.ManualItemResult,
) (ports.PostIntakeManualRegistrationOutcome, ports.PostIntakeManualItem, bool, error) {
	executor, err := items.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PostIntakeManualRegistrationOutcomeInvalid, ports.PostIntakeManualItem{}, false, fmt.Errorf("register post-intake manual item result: %w", err)
	}
	if _, found, err := items.FindPostIntakeManualItem(ctx, key); err != nil || !found {
		return ports.PostIntakeManualRegistrationOutcomeInvalid, ports.PostIntakeManualItem{}, false, err
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_shipment.post_intake_disposition_manual_result
			(tenant_id, parcel_id, decision_version, owner, action, outcome, note, handled_by, handled_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT DO NOTHING`,
		key.Tenant.String(), key.Parcel.String(), key.DecisionVersion, key.Owner.String(), key.Action.String(),
		result.Outcome.String(), result.Note.String(), result.Handler.String(), result.HandledAt.UTC())
	if err != nil {
		return ports.PostIntakeManualRegistrationOutcomeInvalid, ports.PostIntakeManualItem{}, false, fmt.Errorf("register post-intake manual item result: %w", err)
	}
	item, found, err := items.FindPostIntakeManualItem(ctx, key)
	if err != nil || !found {
		return ports.PostIntakeManualRegistrationOutcomeInvalid, ports.PostIntakeManualItem{}, false, err
	}
	if tag.RowsAffected() == 0 {
		return ports.PostIntakeManualAlreadyRegistered, item, true, nil
	}
	return ports.PostIntakeManualRegistered, item, true, nil
}

// ListPendingPostIntakeManualItems 上列本租户还没有登记结果的项的一页：缺省最早的在前、同刻按（包裹、决定版本、所有者、动作）收尾。
// Count 是全部待登记的项数，不受页与条件截断。几句不在同一快照里：计数与清单是此刻的大概，登记的成败由登记口自己判。
func (items *PostIntakeManualItems) ListPendingPostIntakeManualItems(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (ports.PendingPostIntakeManualItems, error) {
	fail := func(err error) (ports.PendingPostIntakeManualItems, error) {
		return ports.PendingPostIntakeManualItems{}, fmt.Errorf("list pending post-intake manual items: %w", err)
	}
	if limit <= 0 {
		return fail(fmt.Errorf("limit %d is not a positive page size", limit))
	}
	plan, err := postIntakeManualItemPages.Plan(query, pendingPostIntakeManualItem, []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := items.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	var total int64
	if err := querier.QueryRow(ctx, plan.CountSQL(postIntakeManualItemSource), plan.Args...).Scan(&total); err != nil {
		return fail(fmt.Errorf("count: %w", err))
	}
	backlog, err := queueBacklog(ctx, querier, query, total,
		"SELECT count(*) FROM "+postIntakeManualItemSource+" WHERE "+pendingPostIntakeManualItem, tenant)
	if err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, plan.PageSQL(postIntakeManualItemColumns+", i.created_at", postIntakeManualItemSource), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()
	var fetched []scannedPostIntakeManualItem
	for rows.Next() {
		var row scannedPostIntakeManualItem
		if row.item, err = scanPostIntakeManualItem(withTrailing{row: rows, extra: []any{&row.createdAt}}); err != nil {
			return fail(err)
		}
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	page, err := cataloguepage.Finish(query, fetched, limit, total,
		func(row scannedPostIntakeManualItem, _ string) cataloguepage.Position {
			key := row.item.Key
			return cataloguepage.Position{
				Value: cataloguepage.FormatInstant(row.createdAt),
				Identity: []string{
					key.Parcel.String(), cataloguepage.FormatInteger(int64(key.DecisionVersion)),
					key.Owner.String(), key.Action.String(),
				},
			}
		})
	if err != nil {
		return fail(err)
	}
	listed := ports.PendingPostIntakeManualItems{
		Items: make([]ports.PostIntakeManualItem, 0, len(page.Rows)),
		Next:  page.Next, Total: page.Total, Count: backlog,
	}
	for _, row := range page.Rows {
		listed.Items = append(listed.Items, row.item)
	}
	return listed, nil
}

func (items *PostIntakeManualItems) ListPostIntakeManualItemsOfParcel(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
) ([]ports.PostIntakeManualItem, error) {
	querier, err := items.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list post-intake manual items of parcel: %w", err)
	}
	rows, err := querier.Query(ctx,
		postIntakeManualItemSelect+`
		 WHERE i.tenant_id = $1 AND i.parcel_id = $2
		 ORDER BY i.decision_version, i.owner, i.action`, tenant.String(), parcel.String())
	if err != nil {
		return nil, fmt.Errorf("list post-intake manual items of parcel: %w", err)
	}
	defer rows.Close()
	listed := []ports.PostIntakeManualItem{}
	for rows.Next() {
		item, err := scanPostIntakeManualItem(rows)
		if err != nil {
			return nil, fmt.Errorf("list post-intake manual items of parcel: %w", err)
		}
		listed = append(listed, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list post-intake manual items of parcel: %w", err)
	}
	return listed, nil
}

func scanPostIntakeManualItem(row dispositionScanner) (ports.PostIntakeManualItem, error) {
	var (
		tenantRaw, parcelRaw, ownerRaw, actionRaw, directionRaw string
		destinationRaw                                          *string
		decisionVersion, currentVersion                         int
		decidedAt                                               time.Time
		outcomeRaw, noteRaw, handledByRaw                       *string
		handledAt                                               *time.Time
	)
	if err := row.Scan(&tenantRaw, &parcelRaw, &decisionVersion, &ownerRaw, &actionRaw,
		&directionRaw, &destinationRaw, &decidedAt, &currentVersion,
		&outcomeRaw, &noteRaw, &handledByRaw, &handledAt); err != nil {
		return ports.PostIntakeManualItem{}, err
	}
	tenant, err := domain.NewTenantID(tenantRaw)
	if err != nil {
		return ports.PostIntakeManualItem{}, err
	}
	parcel, err := domain.NewDeclaredParcelID(parcelRaw)
	if err != nil {
		return ports.PostIntakeManualItem{}, err
	}
	owner, err := domain.ParseDispositionOwner(ownerRaw)
	if err != nil {
		return ports.PostIntakeManualItem{}, err
	}
	action, err := domain.ParseDispositionOwnerAction(actionRaw)
	if err != nil {
		return ports.PostIntakeManualItem{}, err
	}
	direction, err := domain.ParsePostIntakeDispositionDirection(directionRaw)
	if err != nil {
		return ports.PostIntakeManualItem{}, err
	}
	item := ports.PostIntakeManualItem{
		Key:            domain.ManualItemKey{Tenant: tenant, Parcel: parcel, DecisionVersion: decisionVersion, Owner: owner, Action: action},
		Direction:      direction,
		DecidedAt:      decidedAt.UTC(),
		CurrentVersion: currentVersion,
	}
	if destinationRaw != nil {
		if item.Destination, err = domain.NewPostIntakeDispositionDestination(*destinationRaw); err != nil {
			return ports.PostIntakeManualItem{}, err
		}
	}
	if outcomeRaw != nil {
		if noteRaw == nil || handledByRaw == nil || handledAt == nil {
			return ports.PostIntakeManualItem{}, errors.New("a manual item result arrived without its note, handler or time")
		}
		outcome, err := domain.ParseManualItemOutcome(*outcomeRaw)
		if err != nil {
			return ports.PostIntakeManualItem{}, err
		}
		note, err := domain.NewManualItemNote(*noteRaw)
		if err != nil {
			return ports.PostIntakeManualItem{}, err
		}
		handler, err := domain.NewManualItemHandlerReference(*handledByRaw)
		if err != nil {
			return ports.PostIntakeManualItem{}, err
		}
		if item.Result, err = domain.FormManualItemResult(outcome, note, handler, *handledAt); err != nil {
			return ports.PostIntakeManualItem{}, err
		}
		item.HasResult = true
	}
	return item, nil
}
