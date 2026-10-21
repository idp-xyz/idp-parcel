package domain

import (
	"errors"
	"time"
)

// ErrInvalidDeliveryDispatchReaskCadence 是派送重问间隔登记被拒的理由：租户或版本空白、间隔不是正的整分钟。
var ErrInvalidDeliveryDispatchReaskCadence = errors.New("transport fulfillment: invalid delivery dispatch reask cadence")

// DeliveryDispatchReaskCadenceVersion 是重问间隔的版本，由登记方指名：同一版本重放要认得出来，铸出来的号做不到。
type DeliveryDispatchReaskCadenceVersion struct{ requiredValue }

func NewDeliveryDispatchReaskCadenceVersion(value string) (DeliveryDispatchReaskCadenceVersion, error) {
	required, err := newRequiredValue("delivery dispatch reask cadence version", value)
	return DeliveryDispatchReaskCadenceVersion{required}, err
}

// DeliveryDispatchReaskCadence 是一个租户的派送重问间隔（ADR-0230 决定五）：派送待形成册上的一件，最近一次答复过去多久才算
// 到期、该再问一次所有者。它是租户取值——问得多勤，取决于这个租户的所有者多快补齐、愿意被问多少次——产品不给默认，也不出参考值；
// 没登记就没有到期这回事，到期重问如实答未配置。批量重问「全部」不看它：那是运营人员此刻就要问。
//
// 只收整分钟：答复时刻按登记精度存，间隔比分钟更细没有可执行的意义，按秒收只会让两份本该相同的登记因为零头分开。
type DeliveryDispatchReaskCadence struct {
	tenantID TenantID
	version  DeliveryDispatchReaskCadenceVersion
	interval time.Duration
}

func NewDeliveryDispatchReaskCadence(
	tenant TenantID,
	version DeliveryDispatchReaskCadenceVersion,
	interval time.Duration,
) (DeliveryDispatchReaskCadence, error) {
	if !tenant.valid() || !version.valid() || interval < time.Minute || interval%time.Minute != 0 {
		return DeliveryDispatchReaskCadence{}, ErrInvalidDeliveryDispatchReaskCadence
	}
	return DeliveryDispatchReaskCadence{tenantID: tenant, version: version, interval: interval}, nil
}

func (cadence DeliveryDispatchReaskCadence) TenantID() TenantID { return cadence.tenantID }

func (cadence DeliveryDispatchReaskCadence) Version() DeliveryDispatchReaskCadenceVersion {
	return cadence.version
}

func (cadence DeliveryDispatchReaskCadence) Interval() time.Duration { return cadence.interval }

// DueBy 答此刻 now 之下，最近一次答复不晚于哪一刻的那几件到期。
func (cadence DeliveryDispatchReaskCadence) DueBy(now time.Time) time.Time {
	return now.Add(-cadence.interval)
}

// SameAs 报告两份登记的正文是否相同：同一版本重放要正文相同才算重放，否则是冲突。
func (cadence DeliveryDispatchReaskCadence) SameAs(other DeliveryDispatchReaskCadence) bool {
	return cadence.tenantID == other.tenantID && cadence.version == other.version && cadence.interval == other.interval
}
