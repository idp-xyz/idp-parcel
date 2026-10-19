package registrationjson

import (
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// 节点关务区域登记的译装（ADR-0206）。另立文件是因为它属一本新册，判据与本包其余各册同：未知字段拒收、构造门在
// 这里就拒、生效起点无默认。

type nodeCustomsZoneDocument struct {
	TenantID    string    `json:"tenantId"`
	NodeRef     string    `json:"nodeRef"`
	ZoneRef     string    `json:"zoneRef"`
	AppliesFrom time.Time `json:"appliesFrom"`
}

// NodeCustomsZoneFromJSON 译装一次节点关务区域版本登记。
func NodeCustomsZoneFromJSON(raw []byte) (application.RegisterNodeCustomsZoneCommand, error) {
	return nodeCustomsZoneFromJSON(raw, tenantFromDocument)
}

// NodeCustomsZoneFromJSONForTenant 是在线口那一路：租户取操作者信封给的，批文带 tenantId 即拒。
func NodeCustomsZoneFromJSONForTenant(raw []byte, tenant domain.TenantID) (application.RegisterNodeCustomsZoneCommand, error) {
	if err := refuseSelfReportedTenant(raw); err != nil {
		return application.RegisterNodeCustomsZoneCommand{}, err
	}
	return nodeCustomsZoneFromJSON(raw, injectedTenant(tenant))
}

func nodeCustomsZoneFromJSON(raw []byte, tenantSource tenantOf) (application.RegisterNodeCustomsZoneCommand, error) {
	none := application.RegisterNodeCustomsZoneCommand{}
	var document nodeCustomsZoneDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("节点关务区域登记输入不是本入口的形状：%w", err)
	}
	tenant, err := tenantSource(document.TenantID)
	if err != nil {
		return none, err
	}
	node, err := domain.NewNetworkNodeReference(document.NodeRef)
	if err != nil {
		return none, err
	}
	zone, err := domain.NewCustomsZoneReference(document.ZoneRef)
	if err != nil {
		return none, err
	}
	// 生效起点在键上且无默认可言；终点不是输入——换区域走登记一个更晚起点的新版本。
	if document.AppliesFrom.IsZero() {
		return none, fmt.Errorf("节点关务区域登记缺 appliesFrom——生效起点没有默认值")
	}
	return application.RegisterNodeCustomsZoneCommand{
		TenantID:    tenant,
		Node:        node,
		Zone:        zone,
		AppliesFrom: document.AppliesFrom,
	}, nil
}
