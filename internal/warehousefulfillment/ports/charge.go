package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// ChargeMeasure 是发生项某一层的一个计量。未测量不给数量。没有金额。
type ChargeMeasure struct {
	Unit         string `json:"unit"`
	Quantity     string `json:"quantity,omitempty"`
	Unmeasured   bool   `json:"unmeasured,omitempty"`
	Declared     string `json:"declared,omitempty"`
	DeclaredUnit string `json:"declaredUnit,omitempty"`
}

// ChargeAttribute 是作业属性。没有的取值是「无」。
type ChargeAttribute struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ChargeLine 是一行明细。行上的计量与属性不带金额。
type ChargeLine struct {
	Kind       string            `json:"kind"`
	Measures   []ChargeMeasure   `json:"measures"`
	Attributes []ChargeAttribute `json:"attributes"`
}

// ChargeVersion 是发生项的一版。作废不带计量。
type ChargeVersion struct {
	ID         string
	Version    int
	Supersedes int
	Correction string
	Kind       string
	Owner      string
	Warehouse  string
	Source     string
	Occurred   time.Time
	Measures   []ChargeMeasure
	Attributes []ChargeAttribute
	Lines      []ChargeLine
}

// ChargeStore 记下发生项与更正版本。写入在没有环境事务时拒绝。
type ChargeStore interface {
	LoadBySource(ctx context.Context, tenant domain.TenantID, kind, source string) (ChargeVersion, bool, error)
	LoadVersion(ctx context.Context, tenant domain.TenantID, id string, version int) (ChargeVersion, bool, error)
	Successor(ctx context.Context, tenant domain.TenantID, id string, version int) (ChargeVersion, bool, error)
	SaveOriginal(ctx context.Context, tenant domain.TenantID, version ChargeVersion) error
	SaveCorrection(ctx context.Context, tenant domain.TenantID, version ChargeVersion) error
	LoadUnits(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode) (weight, volume string, found bool, err error)
	SaveUnits(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, weight, volume string, at time.Time) error
	LoadAging(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode) (basis string, at time.Time, found bool, err error)
	SaveAging(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, basis string, at time.Time) error
	LoadProject(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, name string) (bool, error)
	SaveProject(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, name string, at time.Time) error
}

// ChargeHandoff 在形成或更正的同一事务交出信封，只带租户、发生项与版本。
type ChargeHandoff interface {
	HandOffChargeOccurrence(ctx context.Context, tenant, occurrenceID string, version int) error
}

// ChargeIdentities 签发发生项标识。
type ChargeIdentities interface {
	NextChargeOccurrenceID() (string, error)
}
