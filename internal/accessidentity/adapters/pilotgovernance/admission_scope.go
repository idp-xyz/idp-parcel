// Package pilotgovernance 是共享接入身份能力对试点治理的消费侧适配器：准入范围桥（ADR-0180 决定一至三；票
// operator-channel/19）。它读本能力自己的准入范围对照，把（租户、能力、事实类型）换成治理登记册的坐标，再问
// pilot-governance 的 AuthorityCoverage。对照只换坐标，「谁在写生产」由治理登记册的生产权威区间答（ADR-0149 决定四
// 第二条）。跨上下文翻译落在消费侧的这个位置，见 internal/architecture 的边界门禁。
package pilotgovernance

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	pgapplication "go.idp.xyz/idp-parcel/internal/pilotgovernance/application"
)

// Coverage 是本适配器对治理区间只读口的最小依赖，pgapplication.AuthorityCoverage 满足它。本产品的权威串在它里面：
// 桥不另带一份（ADR-0180 决定三）。
type Coverage interface {
	Covers(ctx context.Context, coordinates pgapplication.AuthorityCoordinates, at time.Time) (bool, error)
}

var _ Coverage = (*pgapplication.AuthorityCoverage)(nil)

// AdmissionScope 实现 accessidentity.AdmissionScope：操作者与集成客户端两只铸造器共用这一只。
type AdmissionScope struct {
	mappings accessidentity.AdmissionScopeMappingReader
	coverage Coverage
}

var _ accessidentity.AdmissionScope = (*AdmissionScope)(nil)

func NewAdmissionScope(mappings accessidentity.AdmissionScopeMappingReader, coverage Coverage) (*AdmissionScope, error) {
	if mappings == nil || coverage == nil {
		return nil, errors.New("access identity pilot governance: admission scope needs a mapping reader and an authority coverage")
	}
	return &AdmissionScope{mappings: mappings, coverage: coverage}, nil
}

// Admits 先取对照、后问区间。对照没登、或此刻没有生效的那一行，答不在准入范围，区间一行都不读（ADR-0149 越权风险点 3）。
// 对照读不动、同键两行同时生效、区间读不动都交回错误：铸造器把它译成准入范围读不动，不折成不在准入范围——折了会让
// 阶段治理去登一个其实早已登好的区间（票 operator-channel/14 的分格）。
func (scope *AdmissionScope) Admits(
	ctx context.Context,
	tenantID string,
	requirement accessidentity.AdmissionRequirement,
	at time.Time,
) (bool, error) {
	mappings, err := scope.mappings.FindAdmissionScopeMappings(ctx, tenantID, requirement)
	if err != nil {
		return false, fmt.Errorf("access identity pilot governance: read admission scope mappings: %w", err)
	}
	objectScope, found, err := accessidentity.ResolveAdmissionObjectScope(mappings, at)
	if err != nil {
		return false, fmt.Errorf("access identity pilot governance: %w", err)
	}
	if !found {
		return false, nil
	}
	covered, err := scope.coverage.Covers(ctx, pgapplication.AuthorityCoordinates{
		ObjectScope: objectScope,
		Capability:  requirement.Capability,
		FactKind:    requirement.FactKind,
	}, at)
	if err != nil {
		return false, fmt.Errorf("access identity pilot governance: %w", err)
	}
	return covered, nil
}
