// parcel-network-register 是版本化网络目录的受控登记口（票 04 件②）：运营方操作员
// 在数据库网络内手跑，不监听任何端口——网络定义登记是治理动作，不是在线请求面，
// 走独立进程而不进 parcel-api 的端点表（先例：parcel-pricing-register，「是登记口
// 不是后台 CRUD」）。
//
// 输入是一份登记行 JSON（-kind 指族，-file 指路径），未知字段一律拒绝——打错的键
// 静默丢弃会让操作员以为登进去的比实际多。译装后交给登记用例过受理门，缺件在入库前
// 拒绝并指名差哪格。本工具不携带任何默认取值——目录内容是租户取值，形状归产品；
// 本口登的是库里已有的列，不多不少：服务区域的覆盖与节点角色、路由策略的排序形态已有
// 列，日历内容与策略其余规则正文的列还不存在。
//
// 登记的是 0008 的七张版本表（与后续迁移扩的列）。网络证据视图从这份目录折出事实
// （ADR-0148 决定六）：登进一版适用于某服务目的的路由策略，这个范围就不再答`未配置`。
// 0007 的定义登记册不再被读，也不在本口。
//
// 本工具假设业务 schema 已由迁移作业施加，不自行迁移。
//
// 退出码：0 = 已登记；1 = 用法或输入不合法（含受理门指名拒绝）；2 = 治理答案（原行
// 不被顶替，续办属治理裁决）；3 = 未决（依赖故障或撞上库上防线，登记与否未知）。
//
// 退出码 2 只出在自动改路事实那一族。0008 的七族没有治理格——重复版本号由主键挡
// （ADR-0068 Consequences），落在未决的错误文本里由人按约束名续办（换号或核对内容），
// 工具不替登记方决定哪个版本号是对的。0009 的事实目录不同：它的登记用例自己就答
// `已存在`与`内容冲突`（同键同版本重登不覆盖，内容之争没有「后到为准」），把这两格
// 折成失败会让操作员以为重试有用。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
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
	exitAttention  = 2
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
	kindAutoRerouteFacts       = "auto-reroute-facts"
)

// supportedKinds 只为 `-kind` 的用法文本与未知族的错误文本服务。列成一处是因为这两段
// 文本此前各自抄了一遍族名，而新增一族时漏改其中一段不会有任何东西变红。
var supportedKinds = []string{
	kindNode, kindConnection, kindLine, kindServiceArea,
	kindServiceCalendar, kindAvailabilityAdjustment, kindRouteStrategy,
	kindAutoRerouteFacts,
}

// systemClock 是 Clock 端口的生产实现，与各进程级入口同形。事实登记的时刻由用例取
// 时钟，不由登记行携带——登记方说了不算「这是什么时候陈述的」。
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// registrars 收本口要用到的登记用例。两族不合并成一个接口：目录七族的答案代数是
// `已登记`/`被拒`两格，事实目录多出`已存在`与`内容冲突`两格治理答案，合并要先造一个
// 两边都不自然的结果类型，而那正好会把「重试有用」与「重试没用」抹平成一格。
type registrars struct {
	catalog          *application.NetworkCatalogRegistration
	autoRerouteFacts *application.RegisterAutoRerouteFactsHandler
}

func main() {
	kind := flag.String("kind", "", "登记族："+strings.Join(supportedKinds, " / "))
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
	facts, err := adapter.NewAutoRerouteFactsCatalog(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "构造自动改路事实目录：%v\n", err)
		os.Exit(exitUndecided)
	}

	message, code := execute(ctx, *kind, raw, registrars{
		catalog: registration,
		autoRerouteFacts: application.NewRegisterAutoRerouteFactsHandler(
			application.RegisterAutoRerouteFactsDeps{Registry: facts, Clock: systemClock{}},
		),
	}, db.Transactor())
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
	reg registrars,
	transactor bentoapp.Transactor,
) (string, int) {
	// 事实目录那一族在这里分出去而不是并进 commandFor：两族的结果类型与答案代数都不
	// 同，硬并要先把治理答案挤进一个只有两格的类型里。族路由仍只有这一处。
	if kind == kindAutoRerouteFacts {
		return executeAutoRerouteFacts(ctx, raw, reg.autoRerouteFacts, transactor)
	}

	dispatch, err := commandFor(kind, raw)
	if err != nil {
		return fmt.Sprintf("%s: 译装被拒：%v", kind, err), exitUsage
	}

	var result application.RegisterCatalogResult
	err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		outcome, err := dispatch(txCtx, reg.catalog)
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
		area := ports.ServiceAreaDefinitionVersion{
			Code:           payload.Code,
			Version:        payload.Version,
			EffectiveFrom:  payload.EffectiveFrom,
			EffectiveTo:    timeOf(payload.EffectiveTo),
			HasEffectiveTo: payload.EffectiveTo != nil,
		}
		if coverage := payload.Coverage; coverage != nil {
			area.HasCoverage = true
			area.CoverageCountry = coverage.Country
			area.PostalPrefixes = coverage.PostalPrefixes
			area.OriginNodes = coverage.OriginNodes
			area.DestinationNodes = coverage.DestinationNodes
		}
		command := application.RegisterServiceAreaVersionCommand{TenantID: tenant, Area: area}
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
		form := domain.RankingFormUndeclared
		if payload.RankingForm != nil {
			if form, err = domain.RankingFormFrom(*payload.RankingForm); err != nil {
				return nil, err
			}
		}
		command := application.RegisterRouteStrategyVersionCommand{
			TenantID: tenant,
			Strategy: ports.RouteStrategyDefinitionVersion{
				Code:            payload.Code,
				Version:         payload.Version,
				ApplicableScope: payload.ApplicableScope,
				RankingForm:     form,
				EffectiveFrom:   payload.EffectiveFrom,
				EffectiveTo:     timeOf(payload.EffectiveTo),
				HasEffectiveTo:  payload.EffectiveTo != nil,
			},
		}
		return func(ctx context.Context, registrar *application.NetworkCatalogRegistration) (application.RegisterCatalogResult, error) {
			return registrar.RegisterRouteStrategyVersion(ctx, command)
		}, nil
	default:
		return nil, fmt.Errorf("未知登记族 %q（支持 %s）", kind, strings.Join(supportedKinds, " / "))
	}
}

// executeAutoRerouteFacts 把一版自动改路四条件事实推进到应用答案。它与七族那条路平行
// 而不共用，因为答案代数多两格：`已存在`与`内容冲突`都不是失败——原行不被顶替，续办
// 属治理裁决，折成退出码 1 会让操作员以为改一改输入重试就成。
func executeAutoRerouteFacts(
	ctx context.Context,
	raw []byte,
	handler *application.RegisterAutoRerouteFactsHandler,
	transactor bentoapp.Transactor,
) (string, int) {
	var payload autoRerouteFactsPayload
	if err := decodeStrict(raw, &payload); err != nil {
		return fmt.Sprintf("%s: 译装被拒：%v", kindAutoRerouteFacts, err), exitUsage
	}
	command, err := payload.command()
	if err != nil {
		return fmt.Sprintf("%s: 译装被拒：%v", kindAutoRerouteFacts, err), exitUsage
	}

	var result application.RegisterAutoRerouteFactsResult
	err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		outcome, err := handler.Handle(txCtx, command)
		result = outcome
		return err
	})
	if err != nil {
		return fmt.Sprintf("%s: 未决：%v", kindAutoRerouteFacts, err), exitUndecided
	}
	switch result.Outcome() {
	case application.AutoRerouteFactsAccepted:
		return kindAutoRerouteFacts + ": " + result.Outcome().String(), exitRegistered
	case application.AutoRerouteFactsAlreadyExists, application.AutoRerouteFactsConflict:
		return kindAutoRerouteFacts + ": " + result.Outcome().String(), exitAttention
	case application.AutoRerouteFactsRefused:
		return fmt.Sprintf("%s: %s(%s)", kindAutoRerouteFacts, result.Outcome(), result.RefusalReason()), exitUsage
	default:
		// 用例交回一个它自己都不认识的格是实现坏了，不是业务答案。
		return fmt.Sprintf("%s: 未知应用结果 %d", kindAutoRerouteFacts, result.Outcome()), exitUndecided
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
	// Coverage 缺席即这版没登覆盖；给了就由受理门按覆盖文法与节点角色逐格核。
	Coverage *areaCoveragePayload `json:"coverage"`
}

type areaCoveragePayload struct {
	Country          string   `json:"country"`
	PostalPrefixes   []string `json:"postal_prefixes"`
	OriginNodes      []string `json:"origin_nodes"`
	DestinationNodes []string `json:"destination_nodes"`
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
	// RankingForm 缺席即这一版没有声明排序形态；给了就必须是族内的词，空词同样拒。
	RankingForm *string `json:"ranking_form"`
}

// autoRerouteFactsPayload 是四条件事实登记行的 JSON 形状。判断键六维逐个到达——键是
// 包裹级判断范围，不是一个可缩写的标识；两份清单以原始字符串到达，翻译成领域引用由
// 受理门做。登记时刻不在这里：它由用例取时钟。
type autoRerouteFactsPayload struct {
	TenantID           string `json:"tenant_id"`
	CustomerAccountID  string `json:"customer_account_id"`
	ShipmentRequestID  string `json:"shipment_request_id"`
	AcceptanceBaseline string `json:"acceptance_baseline"`
	DeclaredParcelID   string `json:"declared_parcel_id"`
	ServicePurpose     string `json:"service_purpose"`

	Version                     int      `json:"version"`
	PolicyAllowsAutomatic       bool     `json:"policy_allows_automatic"`
	AtControlledNode            bool     `json:"at_controlled_node"`
	OnlyUnexecutedAffected      bool     `json:"only_unexecuted_affected"`
	UnresolvedRestrictions      []string `json:"unresolved_restrictions"`
	OutstandingResponsibilities []string `json:"outstanding_responsibilities"`
	StrategyBasis               string   `json:"strategy_basis"`
}

// command 把登记行译成命令。六维各自过自己的构造器：键不成立在这里就断，不留给受理门
// 用一个笼统的`键不完整`回答——那格是给「维度确实缺了」用的，不是给「这一维写坏了」。
func (payload autoRerouteFactsPayload) command() (application.RegisterAutoRerouteFactsCommand, error) {
	none := application.RegisterAutoRerouteFactsCommand{}
	tenant, err := domain.NewTenantID(payload.TenantID)
	if err != nil {
		return none, err
	}
	account, err := domain.NewCustomerAccountID(payload.CustomerAccountID)
	if err != nil {
		return none, err
	}
	request, err := domain.NewShipmentRequestID(payload.ShipmentRequestID)
	if err != nil {
		return none, err
	}
	baseline, err := domain.NewAcceptanceBaselineReference(payload.AcceptanceBaseline)
	if err != nil {
		return none, err
	}
	parcel, err := domain.NewDeclaredParcelID(payload.DeclaredParcelID)
	if err != nil {
		return none, err
	}
	purpose, err := domain.NewServicePurpose(payload.ServicePurpose)
	if err != nil {
		return none, err
	}
	return application.RegisterAutoRerouteFactsCommand{
		Key: domain.InitialRouteJudgmentKey{
			TenantID:           tenant,
			CustomerAccountID:  account,
			ShipmentRequestID:  request,
			AcceptanceBaseline: baseline,
			DeclaredParcelID:   parcel,
			ServicePurpose:     purpose,
		},
		Version:                     payload.Version,
		PolicyAllowsAutomatic:       payload.PolicyAllowsAutomatic,
		AtControlledNode:            payload.AtControlledNode,
		OnlyUnexecutedAffected:      payload.OnlyUnexecutedAffected,
		UnresolvedRestrictions:      payload.UnresolvedRestrictions,
		OutstandingResponsibilities: payload.OutstandingResponsibilities,
		StrategyBasis:               payload.StrategyBasis,
	}, nil
}
