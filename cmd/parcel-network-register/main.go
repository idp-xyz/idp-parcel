// parcel-network-register 是版本化网络目录的受控登记口（票 04 件②）：运营方操作员
// 在数据库网络内手跑，不监听任何端口——网络定义登记是治理动作，不是在线请求面，
// 走独立进程而不进 parcel-api 的端点表（先例：parcel-pricing-register，「是登记口
// 不是后台 CRUD」）。
//
// 输入是一份登记行 JSON（-kind 指族，-file 指路径），未知字段一律拒绝——打错的键
// 静默丢弃会让操作员以为登进去的比实际多。译装后交给登记用例过受理门，缺件在入库前
// 拒绝并指名差哪格。本工具不携带任何默认取值——目录内容全属实例半边（PAR-NET-01..15
// 待提供），机制先行；服务区域地理覆盖、日历内容、策略规则正文的列还不存在
// （PAR-NET-14），本口今天登的就是版本骨架，不多不少。
//
// 登记的是 0008 的七张版本表；0007 的（租户+服务目的）网络定义登记册**不在本口**——
// 该行如何随目录登记形成属两表合流口径，判给解析层设计（ADR-0068 Decision 六），
// 在那之前三个证据视图照旧答`未配置`，本口登多少都不改变这一点。
//
// 本工具假设业务 schema 已由迁移作业施加，不自行迁移。
//
// 退出码：0 = 已登记；1 = 用法或输入不合法（含受理门指名拒绝）；3 = 未决（依赖故障
// 或撞上库上防线，登记与否未知）。没有退出码 2：本口今天没有治理格——重复版本号由
// 主键挡（ADR-0068 Consequences），落在未决的错误文本里由人按约束名续办（换号或核对
// 内容），工具不替登记方决定哪个版本号是对的。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

const (
	exitRegistered = 0
	exitUsage      = 1
	exitUndecided  = 3
)

const (
	kindNode                   = "node"
	kindConnection             = "connection"
	kindLine                   = "line"
	kindServiceArea            = "service-area"
	kindServiceCalendar        = "service-calendar"
	kindAvailabilityAdjustment = "availability-adjustment"
	kindRouteStrategy          = "route-strategy"
)

func main() {
	kind := flag.String("kind", "", "登记族：node / connection / line / service-area / service-calendar / availability-adjustment / route-strategy")
	file := flag.String("file", "", "登记行 JSON 路径")
	flag.Parse()

	dsn := os.Getenv("IDP_PARCEL_POSTGRES_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "IDP_PARCEL_POSTGRES_DSN 未设置——登记口不猜连接串")
		os.Exit(exitUsage)
	}
	if *file == "" {
		fmt.Fprintln(os.Stderr, "-file 未指定")
		os.Exit(exitUsage)
	}
	raw, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取登记行：%v\n", err)
		os.Exit(exitUsage)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接数据库：%v\n", err)
		os.Exit(exitUndecided)
	}
	defer pool.Close()
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		fmt.Fprintf(os.Stderr, "构造框架 DB：%v\n", err)
		os.Exit(exitUndecided)
	}
	catalog, err := adapter.NewNetworkCatalog(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "构造网络目录：%v\n", err)
		os.Exit(exitUndecided)
	}
	registration, err := application.NewNetworkCatalogRegistration(catalog)
	if err != nil {
		fmt.Fprintf(os.Stderr, "构造登记用例：%v\n", err)
		os.Exit(exitUndecided)
	}

	message, code := execute(ctx, *kind, raw, registration, db.Transactor())
	fmt.Println(message)
	os.Exit(code)
}

// execute 把一行登记推进到应用答案：按族译装（未知字段拒绝、封闭枚举逐格译），再在
// 事务内交给登记用例（版本行与目录修订同一事务推进，正是「环境事务由进程级入口给出」
// 的那一半），最后把应用答案译成退出码。收具体用例类型而不另立窄接口：本口的测试
// 替身立在 ports 写入口上，受理门与译装走的都是真路径。
func execute(
	ctx context.Context,
	kind string,
	raw []byte,
	registrar *application.NetworkCatalogRegistration,
	transactor bentoapp.Transactor,
) (string, int) {
	dispatch, err := commandFor(kind, raw)
	if err != nil {
		return fmt.Sprintf("%s: 译装被拒：%v", kind, err), exitUsage
	}

	var result application.RegisterCatalogResult
	err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		outcome, err := dispatch(txCtx, registrar)
		result = outcome
		return err
	})
	if err != nil {
		return fmt.Sprintf("%s: 未决：%v", kind, err), exitUndecided
	}
	switch result.Outcome() {
	case application.CatalogRegistered:
		return kind + ": " + result.Outcome().String(), exitRegistered
	case application.CatalogRegistrationRefused:
		return fmt.Sprintf("%s: %s(%s)", kind, result.Outcome(), result.RefusalReason()), exitUsage
	default:
		// 用例交回一个它自己都不认识的格是实现坏了，不是业务答案。
		return fmt.Sprintf("%s: 未知应用结果 %d", kind, result.Outcome()), exitUndecided
	}
}

// commandFor 按族译装登记行，交回一个在事务内执行的调用。族在这里定死为封闭七格，
// 与 0008 的七张版本表一一对应。
func commandFor(
	kind string,
	raw []byte,
) (func(ctx context.Context, registrar *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error), error) {
	switch kind {
	case kindNode:
		var payload nodePayload
		if err := decodeStrict(raw, &payload); err != nil {
			return nil, err
		}
		tenant, err := domain.NewTenantID(payload.TenantID)
		if err != nil {
			return nil, err
		}
		command := application.RegisterNodeVersionCommand{
			TenantID: tenant,
			Node: ports.NodeDefinitionVersion{
				Code:             payload.Code,
				Version:          payload.Version,
				BusinessTimezone: payload.BusinessTimezone,
				EffectiveFrom:    payload.EffectiveFrom,
				EffectiveTo:      timeOf(payload.EffectiveTo),
				HasEffectiveTo:   payload.EffectiveTo != nil,
			},
		}
		return func(ctx context.Context, registrar *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			return registrar.RegisterNodeVersion(ctx, command)
		}, nil
	case kindConnection:
		var payload connectionPayload
		if err := decodeStrict(raw, &payload); err != nil {
			return nil, err
		}
		tenant, err := domain.NewTenantID(payload.TenantID)
		if err != nil {
			return nil, err
		}
		command := application.RegisterConnectionVersionCommand{
			TenantID: tenant,
			Connection: ports.ConnectionDefinitionVersion{
				Code:             payload.Code,
				Version:          payload.Version,
				FromNode:         payload.FromNode,
				ToNode:           payload.ToNode,
				BusinessTimezone: payload.BusinessTimezone,
				EffectiveFrom:    payload.EffectiveFrom,
				EffectiveTo:      timeOf(payload.EffectiveTo),
				HasEffectiveTo:   payload.EffectiveTo != nil,
			},
		}
		return func(ctx context.Context, registrar *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			return registrar.RegisterConnectionVersion(ctx, command)
		}, nil
	case kindLine:
		var payload linePayload
		if err := decodeStrict(raw, &payload); err != nil {
			return nil, err
		}
		tenant, err := domain.NewTenantID(payload.TenantID)
		if err != nil {
			return nil, err
		}
		command := application.RegisterLineVersionCommand{
			TenantID: tenant,
			Line: ports.LineDefinitionVersion{
				Code:             payload.Code,
				Version:          payload.Version,
				Segments:         payload.Segments,
				BusinessTimezone: payload.BusinessTimezone,
				ApplicableScope:  payload.ApplicableScope,
				EffectiveFrom:    payload.EffectiveFrom,
				EffectiveTo:      timeOf(payload.EffectiveTo),
				HasEffectiveTo:   payload.EffectiveTo != nil,
			},
		}
		return func(ctx context.Context, registrar *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			return registrar.RegisterLineVersion(ctx, command)
		}, nil
	case kindServiceArea:
		var payload areaPayload
		if err := decodeStrict(raw, &payload); err != nil {
			return nil, err
		}
		tenant, err := domain.NewTenantID(payload.TenantID)
		if err != nil {
			return nil, err
		}
		command := application.RegisterServiceAreaVersionCommand{
			TenantID: tenant,
			Area: ports.ServiceAreaDefinitionVersion{
				Code:           payload.Code,
				Version:        payload.Version,
				EffectiveFrom:  payload.EffectiveFrom,
				EffectiveTo:    timeOf(payload.EffectiveTo),
				HasEffectiveTo: payload.EffectiveTo != nil,
			},
		}
		return func(ctx context.Context, registrar *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			return registrar.RegisterServiceAreaVersion(ctx, command)
		}, nil
	case kindServiceCalendar:
		var payload calendarPayload
		if err := decodeStrict(raw, &payload); err != nil {
			return nil, err
		}
		tenant, err := domain.NewTenantID(payload.TenantID)
		if err != nil {
			return nil, err
		}
		targetKind, err := ports.CatalogTargetKindFrom(payload.TargetKind)
		if err != nil {
			return nil, err
		}
		command := application.RegisterServiceCalendarVersionCommand{
			TenantID: tenant,
			Calendar: ports.ServiceCalendarDefinitionVersion{
				TargetKind:     targetKind,
				TargetCode:     payload.TargetCode,
				Version:        payload.Version,
				EffectiveFrom:  payload.EffectiveFrom,
				EffectiveTo:    timeOf(payload.EffectiveTo),
				HasEffectiveTo: payload.EffectiveTo != nil,
			},
		}
		return func(ctx context.Context, registrar *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			return registrar.RegisterServiceCalendarVersion(ctx, command)
		}, nil
	case kindAvailabilityAdjustment:
		var payload adjustmentPayload
		if err := decodeStrict(raw, &payload); err != nil {
			return nil, err
		}
		tenant, err := domain.NewTenantID(payload.TenantID)
		if err != nil {
			return nil, err
		}
		targetKind, err := ports.CatalogTargetKindFrom(payload.TargetKind)
		if err != nil {
			return nil, err
		}
		adjustmentKind, err := ports.AvailabilityAdjustmentKindFrom(payload.Kind)
		if err != nil {
			return nil, err
		}
		command := application.RegisterAvailabilityAdjustmentCommand{
			TenantID: tenant,
			Adjustment: ports.AvailabilityAdjustmentStatement{
				Code:        payload.Code,
				Version:     payload.Version,
				TargetKind:  targetKind,
				TargetCode:  payload.TargetCode,
				Kind:        adjustmentKind,
				Source:      payload.Source,
				EffectiveAt: payload.EffectiveAt,
				LiftedAt:    timeOf(payload.LiftedAt),
				HasLiftedAt: payload.LiftedAt != nil,
			},
		}
		return func(ctx context.Context, registrar *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			return registrar.RegisterAvailabilityAdjustment(ctx, command)
		}, nil
	case kindRouteStrategy:
		var payload strategyPayload
		if err := decodeStrict(raw, &payload); err != nil {
			return nil, err
		}
		tenant, err := domain.NewTenantID(payload.TenantID)
		if err != nil {
			return nil, err
		}
		command := application.RegisterRouteStrategyVersionCommand{
			TenantID: tenant,
			Strategy: ports.RouteStrategyDefinitionVersion{
				Code:            payload.Code,
				Version:         payload.Version,
				ApplicableScope: payload.ApplicableScope,
				EffectiveFrom:   payload.EffectiveFrom,
				EffectiveTo:     timeOf(payload.EffectiveTo),
				HasEffectiveTo:  payload.EffectiveTo != nil,
			},
		}
		return func(ctx context.Context, registrar *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			return registrar.RegisterRouteStrategyVersion(ctx, command)
		}, nil
	default:
		return nil, fmt.Errorf("未知登记族 %q（支持 %s / %s / %s / %s / %s / %s / %s）",
			kind, kindNode, kindConnection, kindLine, kindServiceArea,
			kindServiceCalendar, kindAvailabilityAdjustment, kindRouteStrategy)
	}
}

// decodeStrict 拒未知字段：打错的键静默丢弃，会让操作员以为登进去的比实际多。
func decodeStrict(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

// timeOf 把可选时刻译回值语义；在不在场由调用处的 Has 位携带。
func timeOf(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

// 七族登记行的 JSON 形状。可选终点用指针表达「不在场」——零时刻是合法的绝对时刻，
// 不能兼作「没有终点」。

type nodePayload struct {
	TenantID         string     `json:"tenant_id"`
	Code             string     `json:"code"`
	Version          int32      `json:"version"`
	BusinessTimezone string     `json:"business_timezone"`
	EffectiveFrom    time.Time  `json:"effective_from"`
	EffectiveTo      *time.Time `json:"effective_to"`
}

type connectionPayload struct {
	TenantID         string     `json:"tenant_id"`
	Code             string     `json:"code"`
	Version          int32      `json:"version"`
	FromNode         string     `json:"from_node"`
	ToNode           string     `json:"to_node"`
	BusinessTimezone string     `json:"business_timezone"`
	EffectiveFrom    time.Time  `json:"effective_from"`
	EffectiveTo      *time.Time `json:"effective_to"`
}

type linePayload struct {
	TenantID         string     `json:"tenant_id"`
	Code             string     `json:"code"`
	Version          int32      `json:"version"`
	Segments         []string   `json:"segments"`
	BusinessTimezone string     `json:"business_timezone"`
	ApplicableScope  string     `json:"applicable_scope"`
	EffectiveFrom    time.Time  `json:"effective_from"`
	EffectiveTo      *time.Time `json:"effective_to"`
}

type areaPayload struct {
	TenantID      string     `json:"tenant_id"`
	Code          string     `json:"code"`
	Version       int32      `json:"version"`
	EffectiveFrom time.Time  `json:"effective_from"`
	EffectiveTo   *time.Time `json:"effective_to"`
}

type calendarPayload struct {
	TenantID      string     `json:"tenant_id"`
	TargetKind    string     `json:"target_kind"`
	TargetCode    string     `json:"target_code"`
	Version       int32      `json:"version"`
	EffectiveFrom time.Time  `json:"effective_from"`
	EffectiveTo   *time.Time `json:"effective_to"`
}

type adjustmentPayload struct {
	TenantID    string     `json:"tenant_id"`
	Code        string     `json:"code"`
	Version     int32      `json:"version"`
	TargetKind  string     `json:"target_kind"`
	TargetCode  string     `json:"target_code"`
	Kind        string     `json:"kind"`
	Source      string     `json:"source"`
	EffectiveAt time.Time  `json:"effective_at"`
	LiftedAt    *time.Time `json:"lifted_at"`
}

type strategyPayload struct {
	TenantID        string     `json:"tenant_id"`
	Code            string     `json:"code"`
	Version         int32      `json:"version"`
	ApplicableScope string     `json:"applicable_scope"`
	EffectiveFrom   time.Time  `json:"effective_from"`
	EffectiveTo     *time.Time `json:"effective_to"`
}
