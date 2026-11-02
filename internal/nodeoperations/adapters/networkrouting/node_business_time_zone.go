package networkrouting

import (
	"context"
	"errors"
	"time"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// NodeBusinessTimezoneSource 是 NR 按节点身份答业务时区的窄读口，按 NR 的 ports.NodeBusinessTimezoneRead 原形取。只声明这一个方法：
// 节点作业只问网点按哪个时区过日子，不拿目录的登记口与判断口。
type NodeBusinessTimezoneSource interface {
	FindNodeBusinessTimezoneAt(ctx context.Context, tenant nrdomain.TenantID, node string, at time.Time) (string, bool, error)
}

// NodeBusinessTimeZones 实现 ports.IssuingNodeTimeZones：本上下文的网点引用就是 NR 的节点身份（节点代码），逐字交过去。
type NodeBusinessTimeZones struct {
	source NodeBusinessTimezoneSource
}

var _ ports.IssuingNodeTimeZones = (*NodeBusinessTimeZones)(nil)

func NewNodeBusinessTimeZones(source NodeBusinessTimezoneSource) (*NodeBusinessTimeZones, error) {
	if source == nil {
		return nil, errors.New("node operations networkrouting adapter: node business timezone source is nil")
	}
	return &NodeBusinessTimeZones{source: source}, nil
}

func (zones *NodeBusinessTimeZones) NodeBusinessTimeZoneAt(
	ctx context.Context,
	tenant domain.TenantID,
	node domain.NodeReference,
	at time.Time,
) (string, bool, error) {
	nrTenant, err := nrdomain.NewTenantID(tenant.String())
	if err != nil {
		return "", false, err
	}
	return zones.source.FindNodeBusinessTimezoneAt(ctx, nrTenant, node.String(), at)
}
