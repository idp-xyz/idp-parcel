package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// DisclosurePolicies 实现 ports.DisclosurePolicyView：按客户合同与信息披露规则回答
// 普通查询视图四维各自获准展示什么。
//
// 目录内容属实例半边（`PAR-VIS-03` / `PAR-VIS-09` 待提供），实现不属。查无规则时
// 如实交回「未配置」——那是空白，不是「不展示」；编排据此把四维全部落成待确认。
// 依赖调不通才作为 error 返回，由应用层形成未决。两条续办路不能压成一格。
//
// 租户在装配期固定，理由同本包另外几个视图：AssessDisclosure 的签名里没有租户，
// 而客户账户引用只在租户内唯一（ADR-0003）。
type DisclosurePolicies struct {
	db     *bentopg.DB
	tenant domain.TenantID
}

func NewDisclosurePolicies(db *bentopg.DB, tenant domain.TenantID) (*DisclosurePolicies, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &DisclosurePolicies{db: db, tenant: tenant}, nil
}

var _ ports.DisclosurePolicyView = (*DisclosurePolicies)(nil)

// AssessDisclosure 对这个货主客户账户与这份投影，逐维答复获准展示什么。
//
// 适用版本按投影的**派生时点**选，不按查询当下：规则换版只作用于生效后的新判断，
// 一份在旧规则下派生的投影不应被事后换版改判可见性。两个版本同时适用时报
// ErrAmbiguousCatalog——挑一个就是替商业责任方作了它没作的决定。
//
// 目录已发布但这个客户没有条目，仍交回未配置：没有默认披露值可发明，不能把「还没
// 写到这个账户」读成「这个账户什么都不给看」。
//
// 四维各自独立翻译。一维不展示不株连其余三维；展示无内容在构造期立不起来，上抛
// 而不吞成待确认——库的 CHECK 已经守过一遍，这里再过领域门，挡的是 CHECK 被后续
// 迁移放宽而 Go 侧没跟上。
func (view *DisclosurePolicies) AssessDisclosure(
	ctx context.Context,
	customer domain.CustomerAccountReference,
	projection domain.TrackingProjection,
) (ports.DisclosureAnswer, bool, error) {
	if view.tenant.String() == "" ||
		customer.String() == "" ||
		projection.Version().String() == "" ||
		projection.Parcel().String() == "" ||
		projection.DerivedAt().IsZero() {
		return ports.DisclosureAnswer{}, false, nil
	}

	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return ports.DisclosureAnswer{}, false, fmt.Errorf("assess disclosure: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT policy_version
		   FROM visibility_exception.disclosure_policy_version
		  WHERE tenant_id = $1
		    AND effective_from <= $2
		    AND (effective_to IS NULL OR effective_to > $2)
		  LIMIT 2`,
		view.tenant.String(), projection.DerivedAt(),
	)
	if err != nil {
		return ports.DisclosureAnswer{}, false, fmt.Errorf("assess disclosure: %w", err)
	}
	versions, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return ports.DisclosureAnswer{}, false, fmt.Errorf("assess disclosure: %w", err)
	}
	switch len(versions) {
	case 0:
		return ports.DisclosureAnswer{}, false, nil
	case 1:
	default:
		return ports.DisclosureAnswer{}, false, fmt.Errorf(
			"%w: 披露策略 tenant=%s at=%s", ErrAmbiguousCatalog, view.tenant, projection.DerivedAt())
	}

	var (
		milestonesState, etaState, finalState, noteState         string
		milestonesContent, etaContent, finalContent, noteContent *string
	)
	err = querier.QueryRow(ctx,
		`SELECT milestones_state, eta_state, final_state, note_state,
		        milestones_content, eta_content, final_content, note_content
		   FROM visibility_exception.disclosure_policy_entry
		  WHERE tenant_id = $1
		    AND policy_version = $2
		    AND customer_account_ref = $3`,
		view.tenant.String(), versions[0], customer.String(),
	).Scan(
		&milestonesState, &etaState, &finalState, &noteState,
		&milestonesContent, &etaContent, &finalContent, &noteContent,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.DisclosureAnswer{}, false, nil
	}
	if err != nil {
		return ports.DisclosureAnswer{}, false, fmt.Errorf("assess disclosure: %w", err)
	}

	milestones, err := dimensionDisclosureFrom(milestonesState, milestonesContent)
	if err != nil {
		return ports.DisclosureAnswer{}, false, fmt.Errorf("assess disclosure: milestones: %w", err)
	}
	eta, err := dimensionDisclosureFrom(etaState, etaContent)
	if err != nil {
		return ports.DisclosureAnswer{}, false, fmt.Errorf("assess disclosure: eta: %w", err)
	}
	final, err := dimensionDisclosureFrom(finalState, finalContent)
	if err != nil {
		return ports.DisclosureAnswer{}, false, fmt.Errorf("assess disclosure: final: %w", err)
	}
	note, err := dimensionDisclosureFrom(noteState, noteContent)
	if err != nil {
		return ports.DisclosureAnswer{}, false, fmt.Errorf("assess disclosure: note: %w", err)
	}
	return ports.DisclosureAnswer{
		Milestones: milestones,
		ETA:        eta,
		Final:      final,
		Note:       note,
	}, true, nil
}

// dimensionDisclosureFrom 把一行的一维译回端口形状。展示必带内容来处，其余两态必
// 不带——与 ShowDimension / PendDimension / WithholdDimension 同一道门。
func dimensionDisclosureFrom(stateRaw string, contentRaw *string) (ports.DimensionDisclosure, error) {
	state, err := dimensionStateFrom(stateRaw)
	if err != nil {
		return ports.DimensionDisclosure{}, err
	}
	switch state {
	case domain.DimensionShown:
		if contentRaw == nil {
			return ports.DimensionDisclosure{}, fmt.Errorf("展示维缺少内容来处")
		}
		content, err := domain.NewViewContentReference(*contentRaw)
		if err != nil {
			return ports.DimensionDisclosure{}, err
		}
		return ports.DimensionDisclosure{State: state, Content: content}, nil
	case domain.DimensionPendingConfirmation, domain.DimensionNotDisclosed:
		if contentRaw != nil {
			return ports.DimensionDisclosure{}, fmt.Errorf("%s 维携带了内容来处", state)
		}
		return ports.DimensionDisclosure{State: state}, nil
	default:
		return ports.DimensionDisclosure{}, fmt.Errorf("未知展示维状态 %q", stateRaw)
	}
}

func dimensionStateFrom(raw string) (domain.DimensionState, error) {
	switch raw {
	case domain.DimensionShown.String():
		return domain.DimensionShown, nil
	case domain.DimensionPendingConfirmation.String():
		return domain.DimensionPendingConfirmation, nil
	case domain.DimensionNotDisclosed.String():
		return domain.DimensionNotDisclosed, nil
	default:
		return domain.DimensionStateInvalid, fmt.Errorf("未知展示维状态 %q", raw)
	}
}
