// parcel-pricing-feed 是计价参考序列的来源喂价受控批量口（ADR-0099 决定六；票
// pricing-reference-series-operations/06），与 parcel-pricing-register 同款：运营方在数据库网络内
// 手跑或由外部调度，不监听任何端口。它做两件事：
//
//   - `-kind feed -tenant T -series S -source-root DIR`：按该租户对该序列的当前来源连接器绑定抓一次
//     （抓取 → 存放本体 → 转录整版重述 → 走同一登记用例 → 免复核声明为「是」时由连接器身份代写
//     复核）。首个连接器是以受控目录文件为源的 FileConnector，-source-root 即受控目录；出网连接器
//     留 draft，绑定声明了本工具未装配的连接器种类时如实答连接器不可用。
//   - `-kind source-connector-binding -file binding.json`：登记一版来源连接器绑定。绑定是实例半边
//     （连接器种类、序列标识、来源标识与定位符、口径引用、登记责任方、抓取节律、免人工复核三格），
//     本工具不带任何默认声明——免复核声明缺省即拒，出厂零绑定。口径引用按 ADR-0108 是三元身份加
//     一枚可选指纹：文档手上有摘要就写 fingerprint，没有就省，本工具不替它铸任何令牌。
//
// 抓取失败不补数、不沿用旧值、不登空版本：缺口留给评价挂起，一条可观察记录写到标准错误
// （ADR-0095 两层里的观察那层；告警通道属实例半边）。本工具假设业务 schema 已由迁移作业施加。
//
// 退出码：0 = 已登记/幂等重放；1 = 用法或输入不合法；2 = 治理答案（无绑定/连接器未装配/同版本
// 内容冲突/免复核代写复核被拒/绑定同版本异声明——库里原行未被顶替，人工续办）；3 = 未决（依赖
// 故障，做到哪一步未知）；4 = 来源失败（抓不到或装不进，缺口已留，见可观察记录）。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/sourcefeed"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

const (
	exitRegistered    = 0
	exitUsage         = 1
	exitGovernance    = 2
	exitUndecided     = 3
	exitSourceFailure = 4
)

const (
	kindFeed    = "feed"
	kindBinding = "source-connector-binding"
)

// feeder 与 bindingRegistrar 把两个用例收窄成本工具消费的形状，测试用替身顶上。
type feeder interface {
	Handle(ctx context.Context, command application.FeedReferenceSeriesCommand) (application.FeedReferenceSeriesResult, error)
}

type bindingRegistrar interface {
	Handle(ctx context.Context, command application.RegisterSourceConnectorBindingCommand) (application.RegisterSourceConnectorBindingOutcome, error)
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// bindingDocument 是绑定文档的线格式，字段与 domain.SourceConnectorBindingSpec 一一对应。
type bindingDocument struct {
	Tenant           string                     `json:"tenant"`
	SeriesID         string                     `json:"seriesId"`
	BindingVersion   string                     `json:"bindingVersion"`
	ConnectorKind    string                     `json:"connectorKind"`
	SourceIdentifier string                     `json:"sourceIdentifier"`
	SourceLocator    string                     `json:"sourceLocator"`
	SeriesKind       string                     `json:"seriesKind"`
	QuoteBasis       *bindingQuoteBasisDocument `json:"quoteBasis,omitempty"`
	Registrant       string                     `json:"registrant"`
	Cadence          string                     `json:"cadence,omitempty"`
	ReviewExemption  string                     `json:"reviewExemption"`
}

type bindingQuoteBasisDocument struct {
	PolicyID      string `json:"policyId"`
	PolicyVersion string `json:"policyVersion"`
	Fingerprint   string `json:"fingerprint,omitempty"`
}

// parseBindingDocument 只做形状翻译；声明齐不齐、汇率有没有口径、免复核声明在不在封闭集都留给领域
// 构造门判——这里不替任何一格填默认。
func parseBindingDocument(raw []byte) (domain.SourceConnectorBinding, error) {
	var document bindingDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return domain.SourceConnectorBinding{}, fmt.Errorf("绑定文档不是合法 JSON：%w", err)
	}
	tenant, err := domain.NewTenantID(document.Tenant)
	if err != nil {
		return domain.SourceConnectorBinding{}, fmt.Errorf("绑定文档 tenant：%w", err)
	}
	spec := domain.SourceConnectorBindingSpec{
		Tenant:           tenant,
		SeriesID:         document.SeriesID,
		Version:          document.BindingVersion,
		ConnectorKind:    document.ConnectorKind,
		SourceIdentifier: document.SourceIdentifier,
		SourceLocator:    document.SourceLocator,
		SeriesKind:       domain.ReferenceSeriesKind(document.SeriesKind),
		Registrant:       document.Registrant,
		Cadence:          document.Cadence,
		ReviewExemption:  domain.ReviewExemption(document.ReviewExemption),
	}
	if document.QuoteBasis != nil {
		basis, err := domain.NewVersionReferenceWithFingerprint(domain.ArtifactCommercialPolicy,
			document.QuoteBasis.PolicyID, document.QuoteBasis.PolicyVersion, document.QuoteBasis.Fingerprint)
		if err != nil {
			return domain.SourceConnectorBinding{}, fmt.Errorf("绑定文档 quoteBasis（policyId、policyVersion 必备，fingerprint 可省）：%w", err)
		}
		spec.QuoteBasis = basis
	}
	binding, err := domain.NewSourceConnectorBinding(spec)
	if err != nil {
		return domain.SourceConnectorBinding{}, fmt.Errorf("绑定立不住：%w", err)
	}
	return binding, nil
}

// assembly 是本工具装配出的两个用例。
type assembly struct {
	feed     *application.FeedReferenceSeriesHandler
	bindings *application.RegisterSourceConnectorBindingHandler
}

// assemble 接线：真库适配器、FileConnector（sourceRoot 为空时不装配——只登绑定不抓取的那次运行
// 用不到它，而连接器不猜读取根）、原地引用工件存放、装配方注入的可观察记录口。
func assemble(db *bentopg.DB, sourceRoot string, clock ports.Clock, observer ports.SourceFeedObserver) (assembly, error) {
	series, err := adapter.NewReferenceSeriesVersions(db)
	if err != nil {
		return assembly{}, fmt.Errorf("构造序列登记册：%w", err)
	}
	reviews, err := adapter.NewReferenceSeriesReviews(db)
	if err != nil {
		return assembly{}, fmt.Errorf("构造序列复核册：%w", err)
	}
	bindings, err := adapter.NewSourceConnectorBindings(db)
	if err != nil {
		return assembly{}, fmt.Errorf("构造绑定登记册：%w", err)
	}
	var connectors []ports.SourceConnector
	if sourceRoot != "" {
		files, err := sourcefeed.NewFileConnector(sourceRoot, clock)
		if err != nil {
			return assembly{}, fmt.Errorf("构造 FileConnector：%w", err)
		}
		connectors = append(connectors, files)
	}
	registry, err := sourcefeed.NewConnectorRegistry(connectors...)
	if err != nil {
		return assembly{}, fmt.Errorf("构造连接器登记表：%w", err)
	}
	return assembly{
		feed: application.NewFeedReferenceSeriesHandler(application.FeedReferenceSeriesDeps{
			Bindings:   bindings,
			Connectors: registry,
			Artifacts:  sourcefeed.NewInPlaceArtifactStore(),
			Latest:     series,
			Register:   series,
			Versions:   series,
			Reviews:    reviews,
			Clock:      clock,
			Observer:   observer,
		}),
		bindings: application.NewRegisterSourceConnectorBindingHandler(application.RegisterSourceConnectorBindingDeps{Bindings: bindings}),
	}, nil
}

// logObserver 把喂价失败的观察写成一行结构化日志到标准错误：怎么出声、出声到哪里归装配方
// （ADR-0095 决定三）。
func logObserver(logger *slog.Logger) ports.SourceFeedObserver {
	return func(observation ports.SourceFeedObservation) {
		attributes := []any{
			"tenant", observation.Tenant.String(),
			"series", observation.SeriesID,
			"binding", observation.BindingVersion,
			"connector", observation.ConnectorKind,
			"locator", observation.Locator,
			"stage", observation.Stage.String(),
		}
		if observation.Err != nil {
			attributes = append(attributes, "err", observation.Err.Error())
		}
		if observation.Answer != "" {
			attributes = append(attributes, "answer", observation.Answer)
		}
		logger.Warn("source feed left a gap", attributes...)
	}
}

// executeInput 是一次运行的全部输入：喂价要租户与序列，登绑定要文档字节。
type executeInput struct {
	kind     string
	tenant   string
	seriesID string
	raw      []byte
}

func main() {
	kind := flag.String("kind", "", "运行种类：feed 或 source-connector-binding")
	tenant := flag.String("tenant", "", "feed：租户标识")
	series := flag.String("series", "", "feed：序列标识")
	sourceRoot := flag.String("source-root", "", "feed：FileConnector 的受控目录")
	file := flag.String("file", "", "source-connector-binding：绑定文档 JSON 路径")
	flag.Parse()

	dsn := os.Getenv("IDP_PARCEL_POSTGRES_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "IDP_PARCEL_POSTGRES_DSN 未设置——喂价口不猜连接串")
		os.Exit(exitUsage)
	}
	input := executeInput{kind: *kind, tenant: *tenant, seriesID: *series}
	switch *kind {
	case kindFeed:
		if *sourceRoot == "" {
			fmt.Fprintln(os.Stderr, "-source-root 未指定——FileConnector 不猜受控目录")
			os.Exit(exitUsage)
		}
	case kindBinding:
		if *file == "" {
			fmt.Fprintln(os.Stderr, "-file 未指定")
			os.Exit(exitUsage)
		}
		raw, err := os.ReadFile(*file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "读取绑定文档：%v\n", err)
			os.Exit(exitUsage)
		}
		input.raw = raw
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
	wired, err := assemble(db, *sourceRoot, systemClock{}, logObserver(slog.New(slog.NewTextHandler(os.Stderr, nil))))
	if err != nil {
		fmt.Fprintf(os.Stderr, "装配：%v\n", err)
		os.Exit(exitUndecided)
	}

	message, code := execute(ctx, input, wired.feed, wired.bindings, db.Transactor())
	fmt.Println(message)
	os.Exit(code)
}

// execute 把一次运行推进到答案：喂价整条链在一个事务内（登记与代写的复核同生共死；失败即整笔
// 回滚，缺口留得干净），登绑定同样在事务内；最后把应用答案译成退出码。
func execute(
	ctx context.Context,
	input executeInput,
	feed feeder,
	bindings bindingRegistrar,
	transactor bentoapp.Transactor,
) (string, int) {
	switch input.kind {
	case kindFeed:
		tenant, err := domain.NewTenantID(input.tenant)
		if err != nil {
			return fmt.Sprintf("feed: -tenant：%v", err), exitUsage
		}
		if input.seriesID == "" {
			return "feed: -series 未指定", exitUsage
		}
		var result application.FeedReferenceSeriesResult
		err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			answer, err := feed.Handle(txCtx, application.FeedReferenceSeriesCommand{Tenant: tenant, SeriesID: input.seriesID})
			result = answer
			return err
		})
		if err != nil {
			return fmt.Sprintf("feed: 未决：%v", err), exitUndecided
		}
		return describeFeed(result), feedExitCode(result)
	case kindBinding:
		binding, err := parseBindingDocument(input.raw)
		if err != nil {
			return fmt.Sprintf("source-connector-binding: %v", err), exitUsage
		}
		var outcome application.RegisterSourceConnectorBindingOutcome
		err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			answer, err := bindings.Handle(txCtx, application.RegisterSourceConnectorBindingCommand{Binding: binding})
			outcome = answer
			return err
		})
		if err != nil {
			return fmt.Sprintf("source-connector-binding: 未决：%v", err), exitUndecided
		}
		return "source-connector-binding: " + outcome.String(), bindingExitCode(outcome)
	default:
		return fmt.Sprintf("未知运行种类 %q（支持 %s / %s）", input.kind, kindFeed, kindBinding), exitUsage
	}
}

func describeFeed(result application.FeedReferenceSeriesResult) string {
	message := "feed: " + result.Outcome.String()
	if result.Reference != (domain.VersionReference{}) {
		message += fmt.Sprintf(" %s@%s", result.Reference.ID(), result.Reference.Version())
	}
	if result.ReviewRequested {
		message += " review=" + result.Review.String()
	}
	return message
}

// feedExitCode：来源失败与治理答案分两个码——调度方对「来源那边坏了」与「要人裁」的处置不同。
// 登了但免复核代写的复核被拒（四眼门等）归治理：版本在册而没进在用，要人看。
func feedExitCode(result application.FeedReferenceSeriesResult) int {
	switch result.Outcome {
	case application.FeedVersionRegistered:
		if result.ReviewRequested && result.Review != application.SeriesReviewRecorded && result.Review != application.SeriesReviewAlreadyOnRegister {
			return exitGovernance
		}
		return exitRegistered
	case application.FeedVersionReplayed:
		return exitRegistered
	case application.FeedBindingUnknown, application.FeedConnectorUnavailable, application.FeedRegistrationRefused:
		return exitGovernance
	case application.FeedFetchFailed, application.FeedTranscriptionRefused:
		return exitSourceFailure
	case application.FeedNotAccepted:
		return exitUsage
	default:
		return exitUndecided
	}
}

func bindingExitCode(outcome application.RegisterSourceConnectorBindingOutcome) int {
	switch outcome {
	case application.SourceConnectorBindingRecorded, application.SourceConnectorBindingAlreadyOnRegister:
		return exitRegistered
	case application.SourceConnectorBindingRegistrationConflict:
		return exitGovernance
	case application.SourceConnectorBindingNotAccepted:
		return exitUsage
	default:
		return exitUndecided
	}
}

// 静态钉住两个真实用例满足本工具的窄接口——接口漂移在编译期暴露。
var (
	_ feeder           = (*application.FeedReferenceSeriesHandler)(nil)
	_ bindingRegistrar = (*application.RegisterSourceConnectorBindingHandler)(nil)
)
