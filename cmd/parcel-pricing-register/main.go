// parcel-pricing-register 是计价配置的受控登记口（票 07/08 的进程级件④）：运营方
// 操作员在数据库网络内手跑，不监听任何端口——价卡与序列登记是治理动作，不是在线
// 请求面，走独立进程而不进 parcel-api 的端点表。
//
// 输入是领域折装的登记快照 JSON（价卡：MarshalPriceCardRegistration 的产物；序列：
// MarshalReferenceSeriesRegistration 的产物），由受控管道从源文件产出。装载时执行
// 领域重建门：规范化形状不被本构建支持、内容摘要自校不过、登记立不住，都在入库前
// 拒绝。本工具不携带任何默认取值——实例内容全属实例半边（PAR-SET-02/03/11 待提供），
// 机制先行。
//
// 本工具假设业务 schema 已由迁移作业施加，不自行迁移。
//
// 退出码：0 = 已登记/幂等重放；1 = 用法或输入不合法（含重建门拒绝）；2 = 登记册
// 治理答案（版本内容冲突/规范化形状不同——原行未被顶替，人工续办）；3 = 未决
// （依赖故障，登记与否未知）。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

const (
	exitRegistered = 0
	exitUsage      = 1
	exitGovernance = 2
	exitUndecided  = 3
)

const (
	kindPriceCard       = "price-card"
	kindReferenceSeries = "reference-series"
)

// priceCardRegistrar 与 referenceSeriesRegistrar 把两个登记用例收窄成本工具消费的
// 形状，测试用替身顶上。
type priceCardRegistrar interface {
	Handle(ctx context.Context, command application.RegisterPriceCardCommand) (application.RegisterPriceCardOutcome, error)
}

type referenceSeriesRegistrar interface {
	Handle(ctx context.Context, command application.RegisterReferenceSeriesCommand) (application.RegisterReferenceSeriesOutcome, error)
}

func main() {
	kind := flag.String("kind", "", "登记种类：price-card 或 reference-series")
	file := flag.String("file", "", "登记快照 JSON 路径")
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
		fmt.Fprintf(os.Stderr, "读取登记快照：%v\n", err)
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
	cards, err := adapter.NewPriceCards(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "构造价卡仓储：%v\n", err)
		os.Exit(exitUndecided)
	}
	series, err := adapter.NewReferenceSeriesVersions(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "构造序列登记册：%v\n", err)
		os.Exit(exitUndecided)
	}

	message, code := execute(ctx, *kind, raw,
		application.NewRegisterPriceCardHandler(application.RegisterPriceCardDeps{Catalog: cards}),
		application.NewRegisterReferenceSeriesHandler(application.RegisterReferenceSeriesDeps{Register: series}),
		db.Transactor(),
	)
	fmt.Println(message)
	os.Exit(code)
}

// execute 把一份登记快照推进到登记册答案：按种类走重建门，再在事务内交给登记用例，
// 最后把应用答案译成退出码。治理答案（冲突/形状不同）不是失败——原行未被顶替，
// 操作者拿答案续办。
func execute(
	ctx context.Context,
	kind string,
	raw []byte,
	cards priceCardRegistrar,
	series referenceSeriesRegistrar,
	transactor bentoapp.Transactor,
) (string, int) {
	switch kind {
	case kindPriceCard:
		registration, err := domain.RehydratePriceCardRegistration(raw)
		if err != nil {
			return fmt.Sprintf("price-card: 快照重建被拒：%v", err), exitUsage
		}
		var outcome application.RegisterPriceCardOutcome
		err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			result, err := cards.Handle(txCtx, application.RegisterPriceCardCommand{Registration: registration})
			outcome = result
			return err
		})
		if err != nil {
			return fmt.Sprintf("price-card: 未决：%v", err), exitUndecided
		}
		return "price-card: " + outcome.String(), priceCardExitCode(outcome)
	case kindReferenceSeries:
		registration, err := domain.RehydrateReferenceSeriesRegistration(raw)
		if err != nil {
			return fmt.Sprintf("reference-series: 快照重建被拒：%v", err), exitUsage
		}
		var outcome application.RegisterReferenceSeriesOutcome
		err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			result, err := series.Handle(txCtx, application.RegisterReferenceSeriesCommand{Registration: registration})
			outcome = result
			return err
		})
		if err != nil {
			return fmt.Sprintf("reference-series: 未决：%v", err), exitUndecided
		}
		return "reference-series: " + outcome.String(), referenceSeriesExitCode(outcome)
	default:
		return fmt.Sprintf("未知登记种类 %q（支持 %s / %s）", kind, kindPriceCard, kindReferenceSeries), exitUsage
	}
}

func priceCardExitCode(outcome application.RegisterPriceCardOutcome) int {
	switch outcome {
	case application.PriceCardRecorded, application.PriceCardAlreadyOnRegister:
		return exitRegistered
	case application.PriceCardRegistrationConflict, application.PriceCardRegistrationIncomparable:
		return exitGovernance
	case application.PriceCardRegistrationNotAccepted:
		return exitUsage
	default:
		return exitUndecided
	}
}

func referenceSeriesExitCode(outcome application.RegisterReferenceSeriesOutcome) int {
	switch outcome {
	case application.ReferenceSeriesRecorded, application.ReferenceSeriesAlreadyOnRegister:
		return exitRegistered
	case application.ReferenceSeriesRegistrationConflict, application.ReferenceSeriesRegistrationIncomparable:
		return exitGovernance
	case application.ReferenceSeriesRegistrationNotAccepted:
		return exitUsage
	default:
		return exitUndecided
	}
}

// 静态钉住两个真实用例满足登记口的窄接口——接口漂移在编译期暴露。
var (
	_ priceCardRegistrar       = (*application.RegisterPriceCardHandler)(nil)
	_ referenceSeriesRegistrar = (*application.RegisterReferenceSeriesHandler)(nil)
)
