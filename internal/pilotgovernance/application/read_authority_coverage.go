package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
)

// ErrAuthorityCoverageUnavailable 表示生产权威区间读不动。它不折成「不覆盖」：读不动要运维去救，
// 折成不覆盖会让人去登一个其实早已登好的区间（票 operator-channel/14，分格判据同 ADR-0029）。
var ErrAuthorityCoverageUnavailable = errors.New("pilot governance: authority intervals cannot be read")

// AuthorityCoordinates 是一次生产写在治理登记册里的坐标：权威区间按（对象范围 × 能力 × 事实类型）
// 三维定位。登记册没有租户维（ADR-0083 决定三），租户到对象范围的对照是租户随试点登记的实例
// 半边，由调用方换好坐标再来问。
type AuthorityCoordinates struct {
	ObjectScope string
	Capability  string
	FactKind    string
}

func (coordinates AuthorityCoordinates) complete() bool {
	return strings.TrimSpace(coordinates.ObjectScope) != "" &&
		strings.TrimSpace(coordinates.Capability) != "" &&
		strings.TrimSpace(coordinates.FactKind) != ""
}

// AuthorityCoverage 是准入范围的只读口（ADR-0149 决定四第二条）：答某一时刻某组坐标是否落在
// 本产品持有的生产权威区间内。
type AuthorityCoverage struct {
	store         ports.AuthorityIntervalStore
	selfAuthority string
}

// NewAuthorityCoverage 的 selfAuthority 是登记册里代表本产品的那个权威串，全册一个约定。留空即
// 未配置：无从判断命中的区间是不是自己的，一律答不覆盖而不是认领它（同 parcel-shipment 生产归属
// 适配器的 SelfAuthority）。
func NewAuthorityCoverage(store ports.AuthorityIntervalStore, selfAuthority string) (*AuthorityCoverage, error) {
	if store == nil {
		return nil, errors.New("pilot governance: authority coverage needs an interval store")
	}
	return &AuthorityCoverage{store: store, selfAuthority: strings.TrimSpace(selfAuthority)}, nil
}

// Covers 答 at 那一刻坐标是否落在本产品的生产权威区间内：区间含起点、不含终点，终点为零值即开放。
// 登记册一个区间也没有时答不覆盖——首个租户登记区间之前，任何生产写都不在准入范围，这是设计
// （ADR-0149 越权风险点 3）。
func (coverage *AuthorityCoverage) Covers(ctx context.Context, coordinates AuthorityCoordinates, at time.Time) (bool, error) {
	if coverage.selfAuthority == "" || !coordinates.complete() {
		return false, nil
	}
	intervals, err := coverage.store.ListCurrent(ctx)
	if err != nil {
		return false, fmt.Errorf("%w: %w", ErrAuthorityCoverageUnavailable, err)
	}
	for _, interval := range intervals {
		if interval.ObjectScope != coordinates.ObjectScope ||
			interval.Capability != coordinates.Capability ||
			interval.FactKind != coordinates.FactKind ||
			interval.Authority != coverage.selfAuthority {
			continue
		}
		if !at.Before(interval.From) && (interval.To.IsZero() || at.Before(interval.To)) {
			return true, nil
		}
	}
	return false, nil
}
