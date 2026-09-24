// reading_completion.go 是形成评价前按价卡绑定补齐输入快照读数的那两步：序列取值（ADR-0099 决定四）与目录读数
// （ADR-0109 决定三、四）。正式评价（EvaluatePricingHandler）与试算（FormEstimateEvaluationsHandler）共用这一份——
// 两处各写一份会在下一次改复核门时分叉，试算与正式评价在同一形成时刻、同一版本清单下就不再给同一结果（ADR-0152
// 决定三）。只读：四口全是解析与读取，没有写口。
package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// readingCompletion 装补齐读数要的时钟与两对解析口。两对各自成对可选：都缺时输入自带读数或缺读数如实落待判断，
// 只给一个是装配错误（halfWired）。
type readingCompletion struct {
	clock            ports.Clock
	inForce          ports.ReferenceSeriesInForceResolver
	seriesVersions   ports.ReferenceSeriesRegister
	catalogueInForce ports.ReferenceCatalogueInForceResolver
	catalogues       ports.ReferenceCatalogueRegister
}

// halfWired 报出只装了一半的那一对；序列那一对先查，与构造期拒法的先后一致。
func (completion readingCompletion) halfWired() error {
	if (completion.inForce == nil) != (completion.seriesVersions == nil) {
		return ErrSeriesResolutionHalfWired
	}
	if (completion.catalogueInForce == nil) != (completion.catalogues == nil) {
		return ErrCatalogueResolutionHalfWired
	}
	return nil
}

// complete 先补序列、再补目录，两步留下的说明按先后并在一起交回。依赖故障返回 error。
func (completion readingCompletion) complete(
	ctx context.Context,
	request domain.EvaluationRequest,
) (domain.EvaluationRequest, []string, error) {
	request, notes, err := completion.completeSeriesReadings(ctx, request)
	if err != nil {
		return domain.EvaluationRequest{}, nil, err
	}
	request, catalogueNotes, err := completion.completeCatalogueReadings(ctx, request)
	if err != nil {
		return domain.EvaluationRequest{}, nil, err
	}
	return request, append(notes, catalogueNotes...), nil
}

// completeSeriesReadings 在形成评价前补齐输入快照里缺席的序列取值（ADR-0099 决定四）：
// 按（租户、种类、序列标识、评价形成时刻）解析在用版本，再按计价基准时点在该版本内解析
// 期次。两个时点分开是要点——形成时刻定版本，基准时点定期次。解析不到不编造：留一条
// 说明进解释，让缺取值照旧在纯函数里落待判断；说明按恢复动作分格，登记侧据以续办。
// 依赖故障返回 error——在用与否未知时形成评价会把一次故障记成一次待判断。
func (completion readingCompletion) completeSeriesReadings(
	ctx context.Context,
	request domain.EvaluationRequest,
) (domain.EvaluationRequest, []string, error) {
	if completion.inForce == nil {
		return request, nil, nil
	}
	missing := missingSeriesBindings(request)
	if len(missing) == 0 {
		return request, nil, nil
	}
	tenant := request.Input().TenantID()
	formedAt := completion.clock.Now()
	basisAt := request.Input().BusinessAt()

	readings := make([]domain.ReferenceSeriesValue, 0, len(missing))
	notes := make([]string, 0)
	for _, binding := range missing {
		reference, outcome, err := completion.inForce.ResolveInForce(ctx, tenant, binding.Kind(), binding.SeriesID(), formedAt)
		if err != nil {
			return domain.EvaluationRequest{}, nil, fmt.Errorf("resolve in-force series version: %w", err)
		}
		switch outcome {
		case ports.SeriesVersionInForce:
			reading, found, err := completion.seriesVersions.ResolveAt(ctx, tenant, reference, basisAt)
			if err != nil {
				return domain.EvaluationRequest{}, nil, fmt.Errorf("resolve series period: %w", err)
			}
			if !found {
				notes = append(notes, fmt.Sprintf("series %s (%s) in-force version %s has no period covering pricing basis time %s",
					binding.SeriesID(), binding.Kind(), reference.Version(), basisAt.UTC().Format(time.RFC3339)))
				// 金额序列的「窗外无期次」是卡声明过的那一格（ADR-0110 Decision 三），不是缺证据：把「查过这一版、
				// 无期次」冻结进输入，纯函数按卡上的窗外行为分流；费率序列没有窗外，缺期次照旧只留说明。
				if binding.Kind() == domain.ReferenceSeriesPublishedAmount {
					absent, err := domain.NewAbsentSeriesReading(binding.Kind(), reference)
					if err != nil {
						return domain.EvaluationRequest{}, nil, fmt.Errorf("freeze absent series reading: %w", err)
					}
					readings = append(readings, absent)
				}
				continue
			}
			readings = append(readings, reading.Value())
		case ports.SeriesHasNoRegisteredVersion:
			notes = append(notes, fmt.Sprintf("series %s (%s) has no registered version", binding.SeriesID(), binding.Kind()))
		case ports.SeriesHasNoApprovedVersion:
			notes = append(notes, fmt.Sprintf("series %s (%s) has registered versions but none approved by review before %s",
				binding.SeriesID(), binding.Kind(), formedAt.UTC().Format(time.RFC3339)))
		case ports.SeriesKindDisagrees:
			notes = append(notes, fmt.Sprintf("series %s is registered under another kind than the plan's %s binding",
				binding.SeriesID(), binding.Kind()))
		default:
			return domain.EvaluationRequest{}, nil, fmt.Errorf("%w: in-force outcome %d", ErrUnexpectedInForceOutcome, outcome)
		}
	}
	completed, err := withSeriesReadings(request, readings)
	if err != nil {
		return domain.EvaluationRequest{}, nil, fmt.Errorf("attach resolved series readings: %w", err)
	}
	return completed.WithSeriesResolutionNotes(notes...), notes, nil
}

// completeCatalogueReadings 在形成评价前补齐输入快照里缺席的目录读数（ADR-0109 Decision 三、四）：按
// （租户、种类、目录标识、评价形成时刻）解析在用版本——复核门照序列那一条（ADR-0099）——再按计价基准
// 时点与输入的邮编路线在该版本内解读数；这一版在基准时点不生效同样是「没查到」。查过没查到的读数照样
// 冻结进输入（值缺席、版本在），纯函数据以落 ZONE_UNRESOLVED / REMOTE_TIER_UNRESOLVED；没有在用版本时留
// 说明，不编造。输入没带邮编路线的请求解不了，留说明交给纯函数按缺分区处置。
func (completion readingCompletion) completeCatalogueReadings(
	ctx context.Context,
	request domain.EvaluationRequest,
) (domain.EvaluationRequest, []string, error) {
	if completion.catalogueInForce == nil {
		return request, nil, nil
	}
	missing := missingCatalogueLinks(request)
	if len(missing) == 0 {
		return request, nil, nil
	}
	tenant := request.Input().TenantID()
	formedAt := completion.clock.Now()
	basisAt := request.Input().BusinessAt()
	route, hasRoute := request.Input().PostalRoute()

	readings := make([]domain.ResolvedCatalogueValue, 0, len(missing))
	notes := make([]string, 0)
	for _, link := range missing {
		if !hasRoute {
			notes = append(notes, fmt.Sprintf("catalogue %s (%s) cannot be consulted: the input carries no postal route", link.CatalogueID(), link.Kind()))
			continue
		}
		reference, outcome, err := completion.catalogueInForce.ResolveInForce(ctx, tenant, link.Kind(), link.CatalogueID(), formedAt)
		if err != nil {
			return domain.EvaluationRequest{}, nil, fmt.Errorf("resolve in-force catalogue version: %w", err)
		}
		switch outcome {
		case ports.CatalogueVersionInForce:
			reading, applicable, err := completion.catalogues.ResolveAt(ctx, tenant, reference, basisAt, route)
			if err != nil {
				return domain.EvaluationRequest{}, nil, fmt.Errorf("resolve catalogue reading: %w", err)
			}
			if !applicable {
				notes = append(notes, fmt.Sprintf("catalogue %s (%s) in-force version %s is not effective at pricing basis time %s",
					link.CatalogueID(), link.Kind(), reference.Version(), basisAt.UTC().Format(time.RFC3339)))
				continue
			}
			readings = append(readings, reading)
		case ports.CatalogueHasNoRegisteredVersion:
			notes = append(notes, fmt.Sprintf("catalogue %s (%s) has no registered version", link.CatalogueID(), link.Kind()))
		case ports.CatalogueHasNoApprovedVersion:
			notes = append(notes, fmt.Sprintf("catalogue %s (%s) has registered versions but none approved by review before %s",
				link.CatalogueID(), link.Kind(), formedAt.UTC().Format(time.RFC3339)))
		case ports.CatalogueKindDisagrees:
			notes = append(notes, fmt.Sprintf("catalogue %s is registered under another kind than the plan's %s link",
				link.CatalogueID(), link.Kind()))
		default:
			return domain.EvaluationRequest{}, nil, fmt.Errorf("%w: catalogue in-force outcome %d", ErrUnexpectedInForceOutcome, outcome)
		}
	}
	completed := request
	if len(readings) > 0 {
		values := append(request.Input().CatalogueReadings(), readings...)
		input, err := request.Input().WithReferenceCatalogues(values...)
		if err != nil {
			return domain.EvaluationRequest{}, nil, fmt.Errorf("attach resolved catalogue readings: %w", err)
		}
		// 与 withSeriesReadings 同一手法：只换输入一格，回指与序列那一步留下的说明都原样保留。
		completed, err = request.WithInput(input)
		if err != nil {
			return domain.EvaluationRequest{}, nil, fmt.Errorf("attach resolved catalogue readings: %w", err)
		}
	}
	return completed.WithSeriesResolutionNotes(notes...), notes, nil
}
