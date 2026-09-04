// parcel-pricing-register 是计价配置的受控登记口（票 07/08 的进程级件④）：运营方
// 操作员在数据库网络内手跑，不监听任何端口。价卡与序列登记另有在线口——按 ADR-0085
// 进 parcel-api 端点表、带未配置格等真渠道证据；本工具保留为受控批量口，两口消费
// 同一登记用例，答案代数一致。
//
// 输入是领域折装的登记快照 JSON（价卡：MarshalPriceCardRegistration 的产物；序列：
// MarshalReferenceSeriesRegistration 的产物），由受控管道从源文件产出。装载时执行
// 领域重建门：规范化形状不被本构建支持、内容摘要自校不过、登记立不住，都在入库前
// 拒绝。本工具不携带任何默认取值——实例内容全属实例半边（PAR-SET-02/03/11 待提供），
// 机制先行。
//
// 第三种登记 reference-series-review 是序列版本复核（ADR-0099 决定二）：输入不是折装
// 快照而是一份复核文档（租户、序列标识、版本号、复核责任方、结论、依据、可选的复核
// 时刻），四眼门与在册与否由复核用例判。种子与补录历史复核走这里；在线口另按 ADR-0085
// 进端点表。
//
// 本工具假设业务 schema 已由迁移作业施加，不自行迁移。
//
// 退出码：0 = 已登记/幂等重放；1 = 用法或输入不合法（含重建门拒绝）；2 = 登记册
// 治理答案（版本内容冲突/规范化形状不同/复核的版本不在册/复核责任方就是登记责任方
// ——原行未被顶替，人工续办）；3 = 未决（依赖故障，登记与否未知）。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

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
	kindPriceCard                = "price-card"
	kindReferenceSeries          = "reference-series"
	kindReferenceSeriesReview    = "reference-series-review"
	kindReferenceCatalogue       = "reference-catalogue"
	kindReferenceCatalogueReview = "reference-catalogue-review"
)

// priceCardRegistrar、referenceSeriesRegistrar 与 referenceSeriesReviewer 把三个用例
// 收窄成本工具消费的形状，测试用替身顶上。
type priceCardRegistrar interface {
	Handle(ctx context.Context, command application.RegisterPriceCardCommand) (application.RegisterPriceCardOutcome, error)
}

type referenceSeriesRegistrar interface {
	Handle(ctx context.Context, command application.RegisterReferenceSeriesCommand) (application.RegisterReferenceSeriesOutcome, error)
}

type referenceSeriesReviewer interface {
	Handle(ctx context.Context, command application.ReviewReferenceSeriesCommand) (application.ReviewReferenceSeriesOutcome, error)
}

// catalogueRegistrars 是计价参考目录（ADR-0109）的登记与复核两个用例在本工具里的形状。第四、五种登记
// 与序列那两种同形：目录输入是 MarshalReferenceCatalogueRegistration 的折装快照（万行级映射由受控管道
// 从源表产出，不在这里逐字段组），复核输入是复核文档。
type catalogueRegistrars struct {
	register interface {
		Handle(ctx context.Context, command application.RegisterReferenceCatalogueCommand) (application.RegisterReferenceCatalogueOutcome, error)
	}
	review interface {
		Handle(ctx context.Context, command application.ReviewReferenceCatalogueCommand) (application.ReviewReferenceCatalogueOutcome, error)
	}
}

// catalogueReviewDocument 是目录复核文档的线格式，与序列复核文档只差键名。
type catalogueReviewDocument struct {
	Tenant           string `json:"tenant"`
	CatalogueID      string `json:"catalogueId"`
	CatalogueVersion string `json:"catalogueVersion"`
	Reviewer         string `json:"reviewer"`
	Decision         string `json:"decision"`
	Basis            string `json:"basis"`
	ReviewedAt       string `json:"reviewedAt,omitempty"`
}

func parseCatalogueReviewDocument(raw []byte) (application.ReviewReferenceCatalogueCommand, error) {
	var document catalogueReviewDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return application.ReviewReferenceCatalogueCommand{}, fmt.Errorf("复核文档不是合法 JSON：%w", err)
	}
	tenant, err := domain.NewTenantID(document.Tenant)
	if err != nil {
		return application.ReviewReferenceCatalogueCommand{}, fmt.Errorf("复核文档 tenant：%w", err)
	}
	command := application.ReviewReferenceCatalogueCommand{
		Tenant:           tenant,
		CatalogueID:      document.CatalogueID,
		CatalogueVersion: document.CatalogueVersion,
		Reviewer:         document.Reviewer,
		Decision:         domain.SeriesReviewDecision(document.Decision),
		Basis:            document.Basis,
	}
	if document.ReviewedAt != "" {
		reviewedAt, err := time.Parse(time.RFC3339, document.ReviewedAt)
		if err != nil {
			return application.ReviewReferenceCatalogueCommand{}, fmt.Errorf("复核文档 reviewedAt 须为 RFC 3339：%w", err)
		}
		command.ReviewedAt = reviewedAt
	}
	return command, nil
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// reviewDocument 是复核文档的线格式。reviewedAt 可缺：缺时由用例取时钟当下。
type reviewDocument struct {
	Tenant        string `json:"tenant"`
	SeriesID      string `json:"seriesId"`
	SeriesVersion string `json:"seriesVersion"`
	Reviewer      string `json:"reviewer"`
	Decision      string `json:"decision"`
	Basis         string `json:"basis"`
	ReviewedAt    string `json:"reviewedAt,omitempty"`
}

// parseReviewDocument 把复核文档译成用例命令。只做形状翻译（时刻按 RFC 3339 解），
// 四件齐不齐、结论在不在封闭集、四眼门都留给用例与领域判。
func parseReviewDocument(raw []byte) (application.ReviewReferenceSeriesCommand, error) {
	var document reviewDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return application.ReviewReferenceSeriesCommand{}, fmt.Errorf("复核文档不是合法 JSON：%w", err)
	}
	tenant, err := domain.NewTenantID(document.Tenant)
	if err != nil {
		return application.ReviewReferenceSeriesCommand{}, fmt.Errorf("复核文档 tenant：%w", err)
	}
	command := application.ReviewReferenceSeriesCommand{
		Tenant:        tenant,
		SeriesID:      document.SeriesID,
		SeriesVersion: document.SeriesVersion,
		Reviewer:      document.Reviewer,
		Decision:      domain.SeriesReviewDecision(document.Decision),
		Basis:         document.Basis,
	}
	if document.ReviewedAt != "" {
		reviewedAt, err := time.Parse(time.RFC3339, document.ReviewedAt)
		if err != nil {
			return application.ReviewReferenceSeriesCommand{}, fmt.Errorf("复核文档 reviewedAt 须为 RFC 3339：%w", err)
		}
		command.ReviewedAt = reviewedAt
	}
	return command, nil
}

func main() {
	kind := flag.String("kind", "", "登记种类：price-card、reference-series、reference-series-review、reference-catalogue 或 reference-catalogue-review")
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
	reviews, err := adapter.NewReferenceSeriesReviews(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "构造序列复核册：%v\n", err)
		os.Exit(exitUndecided)
	}
	catalogues, err := adapter.NewReferenceCatalogueVersions(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "构造目录登记册：%v\n", err)
		os.Exit(exitUndecided)
	}
	catalogueReviews, err := adapter.NewReferenceCatalogueReviews(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "构造目录复核册：%v\n", err)
		os.Exit(exitUndecided)
	}

	message, code := execute(ctx, *kind, raw,
		application.NewRegisterPriceCardHandler(application.RegisterPriceCardDeps{Catalog: cards}),
		application.NewRegisterReferenceSeriesHandler(application.RegisterReferenceSeriesDeps{Register: series}),
		application.NewReviewReferenceSeriesHandler(application.ReviewReferenceSeriesDeps{
			Versions: series, Reviews: reviews, Clock: systemClock{},
		}),
		catalogueRegistrars{
			register: application.NewRegisterReferenceCatalogueHandler(application.RegisterReferenceCatalogueDeps{Register: catalogues}),
			review: application.NewReviewReferenceCatalogueHandler(application.ReviewReferenceCatalogueDeps{
				Versions: catalogues, Reviews: catalogueReviews, Clock: systemClock{},
			}),
		},
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
	reviewer referenceSeriesReviewer,
	catalogues catalogueRegistrars,
	transactor bentoapp.Transactor,
) (string, int) {
	switch kind {
	case kindReferenceCatalogue:
		registration, err := domain.RehydrateReferenceCatalogueRegistration(raw)
		if err != nil {
			return fmt.Sprintf("reference-catalogue: 快照重建被拒：%v", err), exitUsage
		}
		var outcome application.RegisterReferenceCatalogueOutcome
		err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			result, err := catalogues.register.Handle(txCtx, application.RegisterReferenceCatalogueCommand{Registration: registration})
			outcome = result
			return err
		})
		if err != nil {
			return fmt.Sprintf("reference-catalogue: 未决：%v", err), exitUndecided
		}
		return "reference-catalogue: " + outcome.String(), referenceCatalogueExitCode(outcome)
	case kindReferenceCatalogueReview:
		command, err := parseCatalogueReviewDocument(raw)
		if err != nil {
			return fmt.Sprintf("reference-catalogue-review: %v", err), exitUsage
		}
		var outcome application.ReviewReferenceCatalogueOutcome
		err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			result, err := catalogues.review.Handle(txCtx, command)
			outcome = result
			return err
		})
		if err != nil {
			return fmt.Sprintf("reference-catalogue-review: 未决：%v", err), exitUndecided
		}
		return "reference-catalogue-review: " + outcome.String(), referenceCatalogueReviewExitCode(outcome)
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
	case kindReferenceSeriesReview:
		command, err := parseReviewDocument(raw)
		if err != nil {
			return fmt.Sprintf("reference-series-review: %v", err), exitUsage
		}
		var outcome application.ReviewReferenceSeriesOutcome
		err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			result, err := reviewer.Handle(txCtx, command)
			outcome = result
			return err
		})
		if err != nil {
			return fmt.Sprintf("reference-series-review: 未决：%v", err), exitUndecided
		}
		return "reference-series-review: " + outcome.String(), referenceSeriesReviewExitCode(outcome)
	default:
		return fmt.Sprintf("未知登记种类 %q（支持 %s / %s / %s / %s / %s）", kind,
			kindPriceCard, kindReferenceSeries, kindReferenceSeriesReview, kindReferenceCatalogue, kindReferenceCatalogueReview), exitUsage
	}
}

func referenceCatalogueExitCode(outcome application.RegisterReferenceCatalogueOutcome) int {
	switch outcome {
	case application.ReferenceCatalogueRecorded, application.ReferenceCatalogueAlreadyOnRegister:
		return exitRegistered
	case application.ReferenceCatalogueRegistrationConflict, application.ReferenceCatalogueRegistrationIncomparable:
		return exitGovernance
	case application.ReferenceCatalogueRegistrationNotAccepted:
		return exitUsage
	default:
		return exitUndecided
	}
}

func referenceCatalogueReviewExitCode(outcome application.ReviewReferenceCatalogueOutcome) int {
	switch outcome {
	case application.CatalogueReviewRecorded, application.CatalogueReviewAlreadyOnRegister:
		return exitRegistered
	case application.CatalogueReviewConflict, application.CatalogueReviewVersionUnknown, application.CatalogueReviewNeedsAnotherReviewer:
		return exitGovernance
	case application.CatalogueReviewNotAccepted:
		return exitUsage
	default:
		return exitUndecided
	}
}

// referenceSeriesReviewExitCode：版本不在册与四眼不满足都是复核册的治理答案——什么都没
// 被写、也不是本工具用错了，操作者拿答案去登记或换人。
func referenceSeriesReviewExitCode(outcome application.ReviewReferenceSeriesOutcome) int {
	switch outcome {
	case application.SeriesReviewRecorded, application.SeriesReviewAlreadyOnRegister:
		return exitRegistered
	case application.SeriesReviewConflict, application.SeriesReviewVersionUnknown, application.SeriesReviewNeedsAnotherReviewer:
		return exitGovernance
	case application.SeriesReviewNotAccepted:
		return exitUsage
	default:
		return exitUndecided
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

// 静态钉住三个真实用例满足登记口的窄接口——接口漂移在编译期暴露。
var (
	_ priceCardRegistrar       = (*application.RegisterPriceCardHandler)(nil)
	_ referenceSeriesRegistrar = (*application.RegisterReferenceSeriesHandler)(nil)
	_ referenceSeriesReviewer  = (*application.ReviewReferenceSeriesHandler)(nil)
)
