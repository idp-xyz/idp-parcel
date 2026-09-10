package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// PlannedLegWindows 实现 ports.PlannedLegWindowRead：按计划履约段引用交回那一段的计划时间窗口
// （ADR-0131 决定一 / 二）。
//
// 它不另存任何东西，读的是两张判断库里已有的计划本体：初始路由判断行的 plan 列，与复核判断行的
// new_plan 列（改路成的新版本与复核时首次形成的计划都住在那里，见 route_reassessment.go 自注「新计划
// 归 new_plan 列独家拥有」）。两处都按各判断口现有的装载纪律——rebuildPlan 经 FormInitialRoutePlan
// 重建整份计划——之后再按序位取段：段链连续、被选候选合格在读回处复验，一次坏写入在这里暴露，而不是
// 变成一个看起来合法的窗口。本口**不从 jsonb 里抠字段冒充窗口**（RoutePlanCatalogueRead 同一句纪律）；
// 复核行那条语句里的 `new_plan->>'version'` 只用来**定位行**——新计划的版本标识没有平铺列，它是那一
// 行唯一的键——定位到的内容仍整份过构造门，键名即 planRow 的 json 标签，重建后再核一次版本、并由
// 读改路新计划的用例钉住，标签一改这里当场红而不是静默答未找到。
type PlannedLegWindows struct {
	db *bentopg.DB
}

func NewPlannedLegWindows(db *bentopg.DB) (*PlannedLegWindows, error) {
	if db == nil {
		return nil, fmt.Errorf("network routing postgres: db is nil")
	}
	return &PlannedLegWindows{db: db}, nil
}

var _ ports.PlannedLegWindowRead = (*PlannedLegWindows)(nil)

// LoadPlannedLegWindow 先在本租户的初始路由判断行按 plan_version 找，找不到再在本租户的复核判断行
// 的 new_plan 列里找同版本的计划；两处都零行、或版本对上而序位越出段链，答 found=false 且 err=nil
// ——「没找到」是业务答案不是错误。租户条件进每条语句（ADR-0003）：他租户的同名版本对本租户零行，
// 不区分「不存在」与「属于另一个租户」。取的是引用所钉的那一版，不看适用性（ADR-0131 决定二）。
func (repository *PlannedLegWindows) LoadPlannedLegWindow(
	ctx context.Context,
	tenant domain.TenantID,
	reference domain.PlannedLegReference,
) (domain.PlannedTimeWindow, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.PlannedTimeWindow{}, false, fmt.Errorf("load planned leg window: %w", err)
	}

	plan, found, err := planByVersion(ctx, querier, tenant, reference.Version(), initialPlanByVersionSQL, "initial route")
	if err != nil {
		return domain.PlannedTimeWindow{}, false, fmt.Errorf("load planned leg window: %w", err)
	}
	if !found {
		plan, found, err = planByVersion(ctx, querier, tenant, reference.Version(), reassessedPlanByVersionSQL, "route reassessment")
		if err != nil {
			return domain.PlannedTimeWindow{}, false, fmt.Errorf("load planned leg window: %w", err)
		}
	}
	if !found {
		return domain.PlannedTimeWindow{}, false, nil
	}

	leg, found := plan.LegAt(reference.Ordinal())
	if !found {
		return domain.PlannedTimeWindow{}, false, nil
	}
	return leg.Window(), true, nil
}

// 两条语句交回同一列面（判断键五列 + 计划 jsonb），差别只在计划住哪一列、按哪个键定位。
const (
	initialPlanByVersionSQL = `
		SELECT customer_account_id, shipment_request_id,
		       acceptance_baseline, declared_parcel_id, service_purpose,
		       plan
		  FROM network_routing.initial_route
		 WHERE tenant_id = $1
		   AND plan_version = $2`

	reassessedPlanByVersionSQL = `
		SELECT customer_account_id, shipment_request_id,
		       acceptance_baseline, declared_parcel_id, service_purpose,
		       new_plan
		  FROM network_routing.route_reassessment
		 WHERE tenant_id = $1
		   AND new_plan IS NOT NULL
		   AND new_plan->>'version' = $2`
)

// planByVersion 在一张判断库里按版本定位到恰一行，重建判断键与整份计划。多于一行是签发方唯一性
// （RouteIdentityFactory 走序列）被破坏，响亮报错不挑一行——挑了就是把两份计划的内容当一份交出去。
// 重建后的计划版本与定位用的版本再核一次：行里 jsonb 的版本与定位键说的不是同一版是坏数据，同样不吸收。
func planByVersion(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	version domain.RoutePlanVersionID,
	sql string,
	register string,
) (domain.InitialRoutePlan, bool, error) {
	rows, err := querier.Query(ctx, sql, tenant.String(), version.String())
	if err != nil {
		return domain.InitialRoutePlan{}, false, fmt.Errorf("%s by plan version: %w", register, err)
	}
	defer rows.Close()

	var (
		columns  routeKeyColumns
		planJSON []byte
		matched  int
	)
	for rows.Next() {
		matched++
		if matched > 1 {
			return domain.InitialRoutePlan{}, false, fmt.Errorf(
				"%s by plan version: version %q matches more than one row in this tenant", register, version)
		}
		if err := rows.Scan(
			&columns.customerAccount, &columns.shipmentRequest,
			&columns.acceptanceBaseline, &columns.declaredParcel, &columns.servicePurpose,
			&planJSON,
		); err != nil {
			return domain.InitialRoutePlan{}, false, fmt.Errorf("%s by plan version: %w", register, err)
		}
	}
	if err := rows.Err(); err != nil {
		return domain.InitialRoutePlan{}, false, fmt.Errorf("%s by plan version: %w", register, err)
	}
	if matched == 0 {
		return domain.InitialRoutePlan{}, false, nil
	}

	key, err := rebuildRouteKey(tenant, columns)
	if err != nil {
		return domain.InitialRoutePlan{}, false, fmt.Errorf("%s by plan version: %w", register, err)
	}
	plan, err := rebuildPlan(key, planJSON)
	if err != nil {
		return domain.InitialRoutePlan{}, false, fmt.Errorf("%s by plan version: %w", register, err)
	}
	if plan.Version() != version {
		return domain.InitialRoutePlan{}, false, fmt.Errorf(
			"%s by plan version: row located as %q carries plan version %q", register, version, plan.Version())
	}
	return plan, true, nil
}
