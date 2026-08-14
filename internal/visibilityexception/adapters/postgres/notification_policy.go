package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// NotificationPolicies 实现 ports.NotificationPolicyView：按客户合同与通知策略回答
// 一份披露该走什么渠道、限时多少、按哪条判据算满足义务。
//
// 目录内容属实例半边（`PAR-VIS-07` 待提供），实现不属。查不到行即交回「未配置」，
// 编排停在未决等租户登记——不造占位渠道。门户展示能否满足通知义务同样由它背后的
// 合同判断，本适配器只把答复搬过来，不自行推导。
//
// 租户在装配期固定，理由同本包另外三个视图：DirectNotification 的签名里没有租户，
// 而客户账户引用只在租户内唯一（ADR-0003）。
type NotificationPolicies struct {
	db     *bentopg.DB
	tenant domain.TenantID
}

func NewNotificationPolicies(db *bentopg.DB, tenant domain.TenantID) (*NotificationPolicies, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &NotificationPolicies{db: db, tenant: tenant}, nil
}

var _ ports.NotificationPolicyView = (*NotificationPolicies)(nil)

// DirectNotification 取一份披露决定的通知指令。
//
// 按披露决定所依据的那条披露策略查，而不是按客户或包裹：披露策略引用指名的就是
// 版本化信息披露规则，同一策略换版换的是引用值，新旧两行各自被自己那批披露决定
// 命中，因此历史通知不会因为策略换版而被改判。
//
// 截止时间由库用披露决定时间加相对时限算出。相加放在 SQL 里而不是 Go 里，是因为
// interval 的月与日进位语义归 Postgres——Go 侧自己折算会在跨月上与库存的口径分岔，
// 而合同写的「N 个月内」正是要按库那套算。
func (view *NotificationPolicies) DirectNotification(
	ctx context.Context,
	disclosure domain.DisclosureDecision,
) (ports.NotificationDirective, bool, error) {
	policy := disclosure.Policy()
	if view.tenant.String() == "" || policy.String() == "" || disclosure.DecidedAt().IsZero() {
		return ports.NotificationDirective{}, false, nil
	}

	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return ports.NotificationDirective{}, false, fmt.Errorf("direct notification: %w", err)
	}

	var channelRef, obligationRef string
	var deadline time.Time
	err = querier.QueryRow(ctx,
		`SELECT channel_ref, $3::timestamptz + deadline_after, obligation_ref
		   FROM visibility_exception.notification_policy
		  WHERE tenant_id = $1 AND disclosure_policy_ref = $2`,
		view.tenant.String(), policy.String(), disclosure.DecidedAt(),
	).Scan(&channelRef, &deadline, &obligationRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.NotificationDirective{}, false, nil
	}
	if err != nil {
		return ports.NotificationDirective{}, false, fmt.Errorf("direct notification: %w", err)
	}

	channel, err := domain.NewNotificationChannelReference(channelRef)
	if err != nil {
		return ports.NotificationDirective{}, false, fmt.Errorf("direct notification: %w", err)
	}
	obligation, err := domain.NewDisclosurePolicyReference(obligationRef)
	if err != nil {
		return ports.NotificationDirective{}, false, fmt.Errorf("direct notification: %w", err)
	}
	return ports.NotificationDirective{
		Channel:    channel,
		Deadline:   deadline,
		Obligation: obligation,
	}, true, nil
}
