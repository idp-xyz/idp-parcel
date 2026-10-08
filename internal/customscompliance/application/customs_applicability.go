package application

import (
	"context"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 关务适用性判断服务（票 routing-first-cut/12）——CC 对路由候选作答的判断口。只判、不
// 记：首版即时作答不落判断史库（分诊裁定四），出处由判断标识与目录版本引用随答案交回，
// 由消费方随路由判断留痕（ADR-0148 决定一）。CC「合规判断」词条那条强制保存只针对明
// 确申报范围的判断链，不延伸到这里；将来要争「当时为什么这么判」，另立版本化判断史沁展票。
//
// 目录读不到时**折成状态未知而不是错误上抛**：分诊裁定三把「依赖读不到」收进答案代数，
// 上抛会把它从答案里赶出去，而那一格本为它立。错误只留给调用方坏输入这类编程故障。

type CustomsApplicabilityDeps struct {
	Catalog ports.PortsPathsSnapshotView
}

type CustomsApplicabilityHandler struct {
	deps CustomsApplicabilityDeps
}

func NewCustomsApplicabilityHandler(
	deps CustomsApplicabilityDeps,
) *CustomsApplicabilityHandler {
	return &CustomsApplicabilityHandler{deps: deps}
}

// Handle 逐候选作答。Candidate 引用空值与查询三格缺件（租户、时点）是调用方编程错误，响亮
// 上抛——折成一格答案会把坏输入藏成一份像样的作答。目录读不到那一类依赖故障不在这里抛。
func (handler *CustomsApplicabilityHandler) Handle(
	ctx context.Context,
	query ports.CustomsApplicabilityQuery,
) ([]domain.CustomsApplicabilityJudgment, error) {
	if blankTenant(query.Tenant) || query.AsOf.IsZero() {
		return nil, fmt.Errorf("customs compliance: judge customs applicability: query carries no tenant or instant")
	}
	for _, candidate := range query.Candidates {
		if strings.TrimSpace(candidate.Candidate.String()) == "" {
			return nil, fmt.Errorf("customs compliance: judge customs applicability: a candidate reference is blank")
		}
	}

	judgments := make([]domain.CustomsApplicabilityJudgment, 0, len(query.Candidates))
	snapshot, err := handler.deps.Catalog.LoadPortsPathsSnapshot(ctx, query.Tenant)
	if err != nil {
		// 依赖调不通是答案里的一格（状态未知·目录读不到），不是服务失败。
		for _, candidate := range query.Candidates {
			judgment, foldErr := domain.FoldCustomsApplicabilityUnreadable(
				query.Tenant, candidate.Candidate, query.AsOf)
			if foldErr != nil {
				return nil, fmt.Errorf("judge customs applicability: %w", foldErr)
			}
			judgments = append(judgments, judgment)
		}
		return judgments, nil
	}

	entries := domain.CustomsApplicabilityEntries{
		Ports: make([]domain.CustomsPortRecord, 0, len(snapshot.Ports)),
		Paths: make([]domain.CustomsPathRecord, 0, len(snapshot.Paths)),
	}
	for _, row := range snapshot.Ports {
		entries.Ports = append(entries.Ports, domain.CustomsPortRecord{
			Port: row.Port, AppliesFrom: row.AppliesFrom, AppliesUntil: row.AppliesUntil,
		})
	}
	for _, row := range snapshot.Paths {
		entries.Paths = append(entries.Paths, domain.CustomsPathRecord{
			Path: row.Path, Route: row.Route, AppliesFrom: row.AppliesFrom, AppliesUntil: row.AppliesUntil,
		})
	}

	for _, candidate := range query.Candidates {
		judgment, err := domain.FoldCustomsApplicability(
			query.Tenant,
			candidate.Candidate,
			candidate.Origin, candidate.HasOrigin,
			candidate.Destination, candidate.HasDestination,
			query.AsOf,
			entries,
		)
		if err != nil {
			return nil, fmt.Errorf("judge customs applicability: %w", err)
		}
		judgments = append(judgments, judgment)
	}
	return judgments, nil
}
