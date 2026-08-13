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

// ErrCurrentViewChanged 表示乐观锁失手：首发时已有别人占住当前，或替代时当前版本
// 已不是新版本所指的前版。两个并发派生只有一个赢——输家重读当前再走幂等或替代路，
// 覆盖会把赢家的版本链拆断。
var ErrCurrentViewChanged = errors.New("visibility exception postgres: current view changed")

// CustomerViews 实现 ports.CustomerViewStore。
type CustomerViews struct {
	db *bentopg.DB
}

func NewCustomerViews(db *bentopg.DB) (*CustomerViews, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &CustomerViews{db: db}, nil
}

// FindCurrent 按（租户+账户+包裹）取回当前视图版本。
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

	var (
		version, basedOn                                     string
		priorVersion                                         *string
		publishedAt                                          time.Time
		milestonesState, etaState, finalState, noteState     string
		milestonesContent, etaContent, finalContent, noteRaw *string
	)
	err = querier.QueryRow(ctx,
		`SELECT version, based_on_projection, prior_version, published_at,
		        milestones_state, milestones_content,
		        eta_state, eta_content,
		        final_state, final_content,
		        note_state, note_content
		   FROM visibility_exception.customer_view
		  WHERE tenant_id = $1
		    AND customer_account_id = $2
		    AND parcel_id = $3`,
		tenant.String(),
		customer.String(),
		parcel.String(),
	).Scan(&version, &basedOn, &priorVersion, &publishedAt,
		&milestonesState, &milestonesContent,
		&etaState, &etaContent,
		&finalState, &finalContent,
		&noteState, &noteRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CustomerTrackingView{}, false, nil
	}
	if err != nil {
		return domain.CustomerTrackingView{}, false, fmt.Errorf("find current customer view: %w", err)
	}

	dimensions, err := rebuildDimensions(
		dimensionColumns{milestonesState, milestonesContent},
		dimensionColumns{etaState, etaContent},
		dimensionColumns{finalState, finalContent},
		dimensionColumns{noteState, noteRaw},
	)
	if err != nil {
		return domain.CustomerTrackingView{}, false, fmt.Errorf("find current customer view: %w", err)
	}
	snapshot, err := snapshotOf(tenant, customer, parcel, version, basedOn, priorVersion, publishedAt, dimensions)
	if err != nil {
		return domain.CustomerTrackingView{}, false, fmt.Errorf("find current customer view: %w", err)
	}
	view, err := domain.RehydrateCustomerView(snapshot)
	if err != nil {
		return domain.CustomerTrackingView{}, false, fmt.Errorf("find current customer view: %w", err)
	}
	return view, true, nil
}

// Save 提交视图版本：首版只在无当前行时成立（ON CONFLICT DO NOTHING），替代只在当前
// 版本恰为新版本所指前版时成立（UPDATE ... WHERE version = prior 的乐观锁）。零行
// 命中都是 ErrCurrentViewChanged——并发输家的恢复动作是重读，不是覆盖。
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
	if !superseding {
		tag, err := executor.Exec(ctx,
			`INSERT INTO visibility_exception.customer_view
				(tenant_id, customer_account_id, parcel_id,
				 version, based_on_projection, prior_version, published_at,
				 milestones_state, milestones_content,
				 eta_state, eta_content,
				 final_state, final_content,
				 note_state, note_content)
			 VALUES ($1, $2, $3, $4, $5, NULL, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			 ON CONFLICT DO NOTHING`,
			tenant.String(),
			view.Customer().String(),
			view.Parcel().String(),
			view.Version().String(),
			view.BasedOn().String(),
			view.PublishedAt().UTC(),
			milestonesState, milestonesContent,
			etaState, etaContent,
			finalState, finalContent,
			noteState, noteContent,
		)
		if err != nil {
			return fmt.Errorf("save customer view: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("%w", ErrCurrentViewChanged)
		}
		return nil
	}

	tag, err := executor.Exec(ctx,
		`UPDATE visibility_exception.customer_view
		    SET version = $4,
		        based_on_projection = $5,
		        prior_version = $6,
		        published_at = $7,
		        milestones_state = $8, milestones_content = $9,
		        eta_state = $10, eta_content = $11,
		        final_state = $12, final_content = $13,
		        note_state = $14, note_content = $15
		  WHERE tenant_id = $1
		    AND customer_account_id = $2
		    AND parcel_id = $3
		    AND version = $16`,
		tenant.String(),
		view.Customer().String(),
		view.Parcel().String(),
		view.Version().String(),
		view.BasedOn().String(),
		prior.String(),
		view.PublishedAt().UTC(),
		milestonesState, milestonesContent,
		etaState, etaContent,
		finalState, finalContent,
		noteState, noteContent,
		prior.String(),
	)
	if err != nil {
		return fmt.Errorf("save customer view: %w", err)
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

func snapshotOf(
	tenant domain.TenantID,
	customer domain.CustomerAccountReference,
	parcel domain.TrackedParcelReference,
	version, basedOn string,
	priorVersion *string,
	publishedAt time.Time,
	dimensions domain.CustomerViewDimensions,
) (domain.CustomerViewSnapshot, error) {
	_ = tenant // 租户只在键上；视图对象按账户隔离，租户不进领域字段。
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
