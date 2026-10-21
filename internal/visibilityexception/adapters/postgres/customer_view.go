// Package postgres 是 visibility-exception 自有语义端口的 PostgreSQL 适配器。
//
// 显式 SQL、行模型与乐观锁翻译都留在这里，不进领域对象。所有语句显式携带租户与
// 客户账户条件：作用域不是过滤器而是身份的一部分（ADR-0003），缺了它另一个租户或
// 另一个客户的同名包裹就会共用一份视图。
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// ErrCurrentViewChanged 表示乐观锁失手：首发时已有别人占住当前，替代时当前版本已不是
// 新版本所指的前版，或重新确认时当前版已换代或已跟上别的投影。两个并发派生只有一个
// 赢——输家重读当前再走幂等、重新确认或替代路，覆盖会把赢家的版本链拆断。
var ErrCurrentViewChanged = errors.New("visibility exception postgres: current view changed")

// CustomerViews 实现 ports.CustomerViewStore。版本行只增不改写：Save 追加版本行并把
// customer_view_current 的显式标记指向它，已发布的每一版连同四维与发布时间留存、按版本
// 读得回；读当前版走标记直取，不扫描历史。标记是唯一的可变处。
type CustomerViews struct {
	db *bentopg.DB
}

func NewCustomerViews(db *bentopg.DB) (*CustomerViews, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &CustomerViews{db: db}, nil
}

// customerViewColumns 是版本行读回领域对象所需的列，FindCurrent 与 FindVersion 共用一份
// 次序，scanCustomerView 按它扫描。所依据的投影一格由调用方各自选：当前读标记上的，按
// 版本读版本行上的。
const customerViewColumns = `v.version, v.prior_version, v.published_at,
		        v.milestones_state, v.milestones_content,
		        v.eta_state, v.eta_content,
		        v.final_state, v.final_content,
		        v.note_state, v.note_content`

// FindCurrent 按（租户+账户+包裹）取回当前视图版本。所依据的投影取标记上的那一格：当前版
// 经 Reaffirm 跟上过较新的投影时，答的是跟上之后的那一份。
//
// 走 ReadExecutor：事务内读得到本事务刚写的行，事务外用显式注入的连接池——查询
// 端点的读面正是后者。否定结果只回 false，不区分「不存在」「属于另一个客户」与
// 「属于另一个租户」：区分它们等于把对象存在性泄给跨账户探针（ADR-0029，HTTP 面
// 逐字节同答的持久化半边）。读回的一切经领域构造函数重建。
func (repository *CustomerViews) FindCurrent(
	ctx context.Context,
	tenant domain.TenantID,
	customer domain.CustomerAccountReference,
	parcel domain.TrackedParcelReference,
) (domain.CustomerTrackingView, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.CustomerTrackingView{}, false, fmt.Errorf("find current customer view: %w", err)
	}
	view, found, err := scanCustomerView(customer, parcel, querier.QueryRow(ctx,
		`SELECT c.based_on_projection, `+customerViewColumns+`
		   FROM visibility_exception.customer_view_current AS c
		   JOIN visibility_exception.customer_view_version AS v
		     ON v.tenant_id = c.tenant_id
		    AND v.customer_account_id = c.customer_account_id
		    AND v.parcel_id = c.parcel_id
		    AND v.version = c.version
		  WHERE c.tenant_id = $1
		    AND c.customer_account_id = $2
		    AND c.parcel_id = $3`,
		tenant.String(),
		customer.String(),
		parcel.String(),
	))
	if err != nil {
		return domain.CustomerTrackingView{}, false, fmt.Errorf("find current customer view: %w", err)
	}
	return view, found, nil
}

// FindVersion 按版本读回该账户该包裹名下留存的任一版——当前版或已被替代的历史版皆可。
// 所依据的投影取版本行上的那一格，即那一版形成时所依据的投影。否定结果与 FindCurrent
// 同理不分成因。
func (repository *CustomerViews) FindVersion(
	ctx context.Context,
	tenant domain.TenantID,
	customer domain.CustomerAccountReference,
	parcel domain.TrackedParcelReference,
	version domain.CustomerViewVersionID,
) (domain.CustomerTrackingView, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.CustomerTrackingView{}, false, fmt.Errorf("find customer view by version: %w", err)
	}
	view, found, err := scanCustomerView(customer, parcel, querier.QueryRow(ctx,
		`SELECT v.based_on_projection, `+customerViewColumns+`
		   FROM visibility_exception.customer_view_version AS v
		  WHERE v.tenant_id = $1
		    AND v.customer_account_id = $2
		    AND v.parcel_id = $3
		    AND v.version = $4`,
		tenant.String(),
		customer.String(),
		parcel.String(),
		version.String(),
	))
	if err != nil {
		return domain.CustomerTrackingView{}, false, fmt.Errorf("find customer view by version: %w", err)
	}
	return view, found, nil
}

func scanCustomerView(
	customer domain.CustomerAccountReference,
	parcel domain.TrackedParcelReference,
	row pgx.Row,
) (domain.CustomerTrackingView, bool, error) {
	var (
		basedOn, version                                     string
		priorVersion                                         *string
		publishedAt                                          time.Time
		milestonesState, etaState, finalState, noteState     string
		milestonesContent, etaContent, finalContent, noteRaw *string
	)
	err := row.Scan(&basedOn, &version, &priorVersion, &publishedAt,
		&milestonesState, &milestonesContent,
		&etaState, &etaContent,
		&finalState, &finalContent,
		&noteState, &noteRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CustomerTrackingView{}, false, nil
	}
	if err != nil {
		return domain.CustomerTrackingView{}, false, err
	}

	dimensions, err := rebuildDimensions(
		dimensionColumns{milestonesState, milestonesContent},
		dimensionColumns{etaState, etaContent},
		dimensionColumns{finalState, finalContent},
		dimensionColumns{noteState, noteRaw},
	)
	if err != nil {
		return domain.CustomerTrackingView{}, false, err
	}
	snapshot, err := snapshotOf(customer, parcel, version, basedOn, priorVersion, publishedAt, dimensions)
	if err != nil {
		return domain.CustomerTrackingView{}, false, err
	}
	view, err := domain.RehydrateCustomerView(snapshot)
	if err != nil {
		return domain.CustomerTrackingView{}, false, err
	}
	return view, true, nil
}

// Save 追加视图版本并移当前标记：首版只在无当前标记时立标记（ON CONFLICT DO NOTHING），
// 替代只在标记恰指新版本所指前版时移标记（UPDATE ... WHERE version = prior 的乐观锁）。
// 零行命中都是 ErrCurrentViewChanged——并发输家的恢复动作是重读，不是覆盖；它已写下的
// 版本行随同一事务回滚，不留一版从未成为当前的「已发布」视图。
//
// 版本行不带 ON CONFLICT：同版本号重写以主键冲突报错暴露，不静默覆盖已发布的那一版。
//
// 走 RequireExecutor：视图提交与将来同一步的意图发布必须同生共死。
func (repository *CustomerViews) Save(
	ctx context.Context,
	tenant domain.TenantID,
	view domain.CustomerTrackingView,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save customer view: %w", err)
	}

	dimensions := view.Dimensions()
	milestonesState, milestonesContent := dimensionColumnsOf(dimensions.Milestones)
	etaState, etaContent := dimensionColumnsOf(dimensions.ETA)
	finalState, finalContent := dimensionColumnsOf(dimensions.Final)
	noteState, noteContent := dimensionColumnsOf(dimensions.Note)

	prior, superseding := view.PriorVersion()
	var priorColumn *string
	if superseding {
		priorColumn = stringPointer(prior.String())
	}

	if _, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.customer_view_version
			(tenant_id, customer_account_id, parcel_id,
			 version, based_on_projection, prior_version, published_at,
			 milestones_state, milestones_content,
			 eta_state, eta_content,
			 final_state, final_content,
			 note_state, note_content)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		tenant.String(),
		view.Customer().String(),
		view.Parcel().String(),
		view.Version().String(),
		view.BasedOn().String(),
		priorColumn,
		view.PublishedAt().UTC(),
		milestonesState, milestonesContent,
		etaState, etaContent,
		finalState, finalContent,
		noteState, noteContent,
	); err != nil {
		return fmt.Errorf("save customer view version: %w", err)
	}

	if !superseding {
		tag, err := executor.Exec(ctx,
			`INSERT INTO visibility_exception.customer_view_current
				(tenant_id, customer_account_id, parcel_id, version, based_on_projection)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT DO NOTHING`,
			tenant.String(),
			view.Customer().String(),
			view.Parcel().String(),
			view.Version().String(),
			view.BasedOn().String(),
		)
		if err != nil {
			return fmt.Errorf("save customer view current marker: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("%w", ErrCurrentViewChanged)
		}
		return nil
	}

	tag, err := executor.Exec(ctx,
		`UPDATE visibility_exception.customer_view_current
		    SET version = $4,
		        based_on_projection = $5
		  WHERE tenant_id = $1
		    AND customer_account_id = $2
		    AND parcel_id = $3
		    AND version = $6`,
		tenant.String(),
		view.Customer().String(),
		view.Parcel().String(),
		view.Version().String(),
		view.BasedOn().String(),
		prior.String(),
	)
	if err != nil {
		return fmt.Errorf("save customer view current marker: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w", ErrCurrentViewChanged)
	}
	return nil
}

// Reaffirm 只改当前标记上所依据的投影那一格，不写版本行：客户已读到的那一版一列不动。
// 乐观锁同时看版本与原依据——当前版已换代，或已被另一次派生跟上别的投影，都零行命中，
// 输家重读后按新的当前版重走。
func (repository *CustomerViews) Reaffirm(
	ctx context.Context,
	tenant domain.TenantID,
	view domain.CustomerTrackingView,
	replacing domain.ProjectionVersionID,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("reaffirm customer view: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`UPDATE visibility_exception.customer_view_current
		    SET based_on_projection = $5
		  WHERE tenant_id = $1
		    AND customer_account_id = $2
		    AND parcel_id = $3
		    AND version = $4
		    AND based_on_projection = $6`,
		tenant.String(),
		view.Customer().String(),
		view.Parcel().String(),
		view.Version().String(),
		view.BasedOn().String(),
		replacing.String(),
	)
	if err != nil {
		return fmt.Errorf("reaffirm customer view: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w", ErrCurrentViewChanged)
	}
	return nil
}

type dimensionColumns struct {
	state   string
	content *string
}

func dimensionColumnsOf(dimension domain.ViewDimension) (string, *string) {
	if content, shown := dimension.Content(); shown {
		value := content.String()
		return dimension.State().String(), &value
	}
	return dimension.State().String(), nil
}

func rebuildDimensions(milestones, eta, final, note dimensionColumns) (domain.CustomerViewDimensions, error) {
	milestonesDim, err := rebuildDimension(milestones)
	if err != nil {
		return domain.CustomerViewDimensions{}, fmt.Errorf("milestones dimension: %w", err)
	}
	etaDim, err := rebuildDimension(eta)
	if err != nil {
		return domain.CustomerViewDimensions{}, fmt.Errorf("eta dimension: %w", err)
	}
	finalDim, err := rebuildDimension(final)
	if err != nil {
		return domain.CustomerViewDimensions{}, fmt.Errorf("final dimension: %w", err)
	}
	noteDim, err := rebuildDimension(note)
	if err != nil {
		return domain.CustomerViewDimensions{}, fmt.Errorf("note dimension: %w", err)
	}
	return domain.CustomerViewDimensions{
		Milestones: milestonesDim,
		ETA:        etaDim,
		Final:      finalDim,
		Note:       noteDim,
	}, nil
}

func rebuildDimension(columns dimensionColumns) (domain.ViewDimension, error) {
	switch columns.state {
	case domain.DimensionShown.String():
		if columns.content == nil {
			return domain.ViewDimension{}, fmt.Errorf("shown dimension without content")
		}
		content, err := domain.NewViewContentReference(*columns.content)
		if err != nil {
			return domain.ViewDimension{}, err
		}
		return domain.ShowDimension(content)
	case domain.DimensionPendingConfirmation.String():
		return domain.PendDimension(), nil
	case domain.DimensionNotDisclosed.String():
		return domain.WithholdDimension(), nil
	default:
		return domain.ViewDimension{}, fmt.Errorf("unknown dimension state %q", columns.state)
	}
}

// snapshotOf 不收租户：租户只在键上，视图对象按账户隔离，租户不进领域字段。
func snapshotOf(
	customer domain.CustomerAccountReference,
	parcel domain.TrackedParcelReference,
	version, basedOn string,
	priorVersion *string,
	publishedAt time.Time,
	dimensions domain.CustomerViewDimensions,
) (domain.CustomerViewSnapshot, error) {
	versionID, err := domain.NewCustomerViewVersionID(version)
	if err != nil {
		return domain.CustomerViewSnapshot{}, err
	}
	basedOnID, err := domain.NewProjectionVersionID(basedOn)
	if err != nil {
		return domain.CustomerViewSnapshot{}, err
	}
	snapshot := domain.CustomerViewSnapshot{
		Version:     versionID,
		Customer:    customer,
		Parcel:      parcel,
		BasedOn:     basedOnID,
		Dimensions:  dimensions,
		PublishedAt: publishedAt.UTC(),
	}
	if priorVersion != nil {
		priorID, err := domain.NewCustomerViewVersionID(*priorVersion)
		if err != nil {
			return domain.CustomerViewSnapshot{}, err
		}
		snapshot.PriorVersion = priorID
	}
	return snapshot, nil
}
