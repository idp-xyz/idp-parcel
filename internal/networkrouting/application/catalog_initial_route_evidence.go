package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// ErrInitialRouteEvidenceUnresolvable 说明这个范围的目录已配置（修订锚在、有适用的路由策略版本），而初始路由的
// 事实族还没从目录折出：时间投影、候选段链与成本归 routing-first-cut/09、10。它必须响亮上抛，不能退成`未配置`
// 也不能退成空证据（ADR-0053 决定四第三格、ADR-0148 决定六）：退成`未配置`会让人去催租户登记一份已经登记过的
// 网络；退成空证据则让领域评出`无当前有效路由`，把「这个构建还没折这几族」讲成「这个网络里没有路」。
var ErrInitialRouteEvidenceUnresolvable = errors.New(
	"network routing: the network catalog is configured but initial route facts are not folded from it yet")

// CatalogInitialRouteEvidence 是初始路由与复核所用证据视图的目录实现。在 routing-first-cut/09 之前它只答`未配置`
// 这一格——与可达性一侧同一条判法（catalogConfiguredFor），不再读定义登记册（迁移 0007）；目录已配置时照旧上抛
// ErrInitialRouteEvidenceUnresolvable。
//
// 初始路由判断键没有 asOf：路由判断时点归本上下文（UC-NR-001「网络判断基线」一行），取时钟。
type CatalogInitialRouteEvidence struct {
	catalog ports.NetworkCatalogRead
	clock   ports.Clock
}

var _ ports.InitialRouteEvidenceView = (*CatalogInitialRouteEvidence)(nil)

func NewCatalogInitialRouteEvidence(
	catalog ports.NetworkCatalogRead,
	clock ports.Clock,
) (*CatalogInitialRouteEvidence, error) {
	if catalog == nil {
		return nil, fmt.Errorf("network routing application: network catalog read is required")
	}
	if clock == nil {
		return nil, fmt.Errorf("network routing application: clock is required")
	}
	return &CatalogInitialRouteEvidence{catalog: catalog, clock: clock}, nil
}

// LoadInitialRouteEvidence 三格见 ports.InitialRouteEvidenceView。
func (view *CatalogInitialRouteEvidence) LoadInitialRouteEvidence(
	ctx context.Context,
	key domain.InitialRouteJudgmentKey,
) (ports.InitialRouteEvidence, bool, error) {
	snapshot, configured, err := view.catalog.LoadDefinitionsAt(ctx, key.TenantID, view.clock.Now())
	if err != nil {
		return ports.InitialRouteEvidence{}, false, fmt.Errorf("load initial route evidence: %w", err)
	}
	if !catalogConfiguredFor(snapshot, configured, key.ServicePurpose) {
		return ports.InitialRouteEvidence{}, false, nil
	}
	return ports.InitialRouteEvidence{}, false, ErrInitialRouteEvidenceUnresolvable
}
