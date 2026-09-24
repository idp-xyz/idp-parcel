// parcel-commercial 是商业权威依据的受控发布入口（syn-wall-door-audit 票 03 件 3）。
//
// 它是登记口不是后台 CRUD：只按 UC-PC-001 的用例走——发布批逐项独立成败（AT-PC-011）、
// 重复返回原结果、冲突绝不覆盖、未决如实报回；另带消费方解析键登记（票 03 件 2，
// ADR-0025 的消费方实例半边）。声明内容全部来自输入文件，进程不内置任何生产默认。
//
// 选受控 CLI 而不是 HTTP 端点：发布是低频、人审驱动的运维动作，进程口的职责只是把
// 已批准的批文交给用例并如实回显落点；不占 parcel-api 的端点面（那两份接线文件由
// 占号纪律管着，见 docs/agents/parallel-sessions.md）。
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	pspartycommercial "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

const envDatabaseDSN = "IDP_PARCEL_POSTGRES_DSN"

// 退出码把「谁来做下一步」分开：0 = 全部落定（含重复重放）；1 = 技术失败，运维重试；
// 2 = 有未决或冲突，商业责任方要看报告。
const (
	exitLanded    = 0
	exitTechnical = 1
	exitAttention = 2
)

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, getenv func(string) string, out, errOut io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(errOut, "用法：parcel-commercial <publish|register-resolution-key|register-parties|deactivate-party-identity|register-products|register-registration-number-types> -input <file>")
		return exitTechnical
	}
	switch args[0] {
	case "publish":
		return runPublish(ctx, args[1:], getenv, out, errOut)
	case "register-resolution-key":
		return runRegisterResolutionKey(ctx, args[1:], getenv, out, errOut)
	case "register-parties":
		return runRegisterParties(ctx, args[1:], getenv, out, errOut)
	case "deactivate-party-identity":
		return runDeactivatePartyIdentity(ctx, args[1:], getenv, out, errOut)
	case "register-products":
		return runRegisterProducts(ctx, args[1:], getenv, out, errOut)
	case "register-registration-number-types":
		return runRegisterRegistrationNumberTypes(ctx, args[1:], getenv, out, errOut)
	default:
		fmt.Fprintf(errOut, "未知子命令 %q；可用：publish、register-resolution-key、register-parties、deactivate-party-identity、register-products、register-registration-number-types\n", args[0])
		return exitTechnical
	}
}

func runPublish(ctx context.Context, args []string, getenv func(string) string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("publish", flag.ContinueOnError)
	flags.SetOutput(errOut)
	input := flags.String("input", "", "发布批 JSON 文件路径")
	if err := flags.Parse(args); err != nil {
		return exitTechnical
	}
	if *input == "" {
		fmt.Fprintln(errOut, "publish 需要 -input <file>")
		return exitTechnical
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(errOut, "读发布批：%v\n", err)
		return exitTechnical
	}
	commands, err := publishCommandsFromJSON(raw)
	if err != nil {
		fmt.Fprintf(errOut, "%v\n", err)
		return exitTechnical
	}

	db, cleanup, err := openDatabase(ctx, getenv)
	if err != nil {
		fmt.Fprintf(errOut, "%v\n", err)
		return exitTechnical
	}
	defer cleanup()

	registry, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		fmt.Fprintf(errOut, "构造发布登记册：%v\n", err)
		return exitTechnical
	}
	// 受控批量口与在线口消费同一个发布用例，「参数已登记」续办信封（ADR-0094 决定四）也同样从这里
	// 发：时点策略经 CLI 登记上去，停在`等待运营登记`的委托同样要有信来推。
	store, err := outbox.NewStore(db)
	if err != nil {
		fmt.Fprintf(errOut, "构造 Outbox：%v\n", err)
		return exitTechnical
	}
	registrationHandoff, err := pcpostgres.NewOutboxOperatorRegistrationCompletedHandoff(db, store, systemClock{})
	if err != nil {
		fmt.Fprintf(errOut, "构造参数已登记交接：%v\n", err)
		return exitTechnical
	}
	handler := pcapplication.NewPublishCommercialAuthorityHandler(registry, systemClock{}, registrationHandoff)

	// 批不是聚合（AT-PC-011）：逐项各起事务，前项已落库的不因后项失败被撤出；
	// 后项装载的整册天然看得见前项，指名引用因此可以在一批内前后相依。
	attention := false
	for index, command := range commands {
		var result pcapplication.PublishCommercialAuthorityResult
		err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			var handleErr error
			result, handleErr = handler.Handle(txCtx, command)
			return handleErr
		})
		label := fmt.Sprintf("第 %d/%d 项 %s %s/%s", index+1, len(commands),
			command.Spec.Kind, command.Spec.ObjectID, command.Spec.Version)
		if err != nil {
			// 技术失败停批：剩余项一项没试，重跑同一批即可续办——已落定的项会以
			// 重复回答，不会二次发布。
			fmt.Fprintf(errOut, "%s：技术失败，批在此停下：%v\n", label, err)
			return exitTechnical
		}
		fmt.Fprintf(out, "%s：%s%s\n", label, result.Outcome(), publishDetail(result))
		switch result.Outcome() {
		case pcapplication.CommercialPublicationPending,
			pcapplication.CommercialPublicationConflicted,
			pcapplication.CommercialPublicationNotAccepted:
			// `未受理`（ADR-0126 Decision 二）：这一项一个字节没写，批文里声明的摘要与算出的对不上——
			// 要人改批文，与未决、冲突同抬 attention；退 0 会让一批里没进去的那一项静静消失。
			attention = true
		}
		for _, report := range result.Declarations() {
			if report.Outcome == pcports.DeclarationContentConflict {
				attention = true
			}
		}
	}
	if attention {
		return exitAttention
	}
	return exitLanded
}

func publishDetail(result pcapplication.PublishCommercialAuthorityResult) string {
	detail := ""
	if cause := result.PendingCause(); cause != nil {
		detail += fmt.Sprintf("（原因：%v）", cause)
	}
	if cause := result.RefusalCause(); cause != nil {
		// 两个串都打出来：抄算出的那一个就是恢复动作（ADR-0126 Decision 二）。
		detail += fmt.Sprintf("（原因：%v）", cause)
		if declared, computed, reconciled := result.DigestReconciliation(); reconciled {
			detail += fmt.Sprintf("；声明的摘要 %s，算出的摘要 %s", declared, computed)
		}
	}
	reports := result.Declarations()
	if len(reports) > 0 {
		detail += "；声明："
		for index, report := range reports {
			if index > 0 {
				detail += " "
			}
			detail += fmt.Sprintf("%s=%s", report.Channel, report.Outcome)
		}
	}
	return detail
}

func runRegisterResolutionKey(ctx context.Context, args []string, getenv func(string) string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("register-resolution-key", flag.ContinueOnError)
	flags.SetOutput(errOut)
	input := flags.String("input", "", "解析键登记 JSON 文件路径")
	if err := flags.Parse(args); err != nil {
		return exitTechnical
	}
	if *input == "" {
		fmt.Fprintln(errOut, "register-resolution-key 需要 -input <file>")
		return exitTechnical
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(errOut, "读登记文件：%v\n", err)
		return exitTechnical
	}
	registration, err := keyRegistrationFromJSON(raw)
	if err != nil {
		fmt.Fprintf(errOut, "%v\n", err)
		return exitTechnical
	}

	db, cleanup, err := openDatabase(ctx, getenv)
	if err != nil {
		fmt.Fprintf(errOut, "%v\n", err)
		return exitTechnical
	}
	defer cleanup()

	store, err := pspostgres.NewCommercialResolutionKeyStore(db)
	if err != nil {
		fmt.Fprintf(errOut, "构造解析键持久化面：%v\n", err)
		return exitTechnical
	}
	keys, err := pspartycommercial.NewCommercialResolutionKeys(store)
	if err != nil {
		fmt.Fprintf(errOut, "构造解析键登记面：%v\n", err)
		return exitTechnical
	}
	var outcome pspartycommercial.ResolutionKeySaveOutcome
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		var registerErr error
		outcome, registerErr = keys.Register(txCtx, registration)
		return registerErr
	}); err != nil {
		fmt.Fprintf(errOut, "登记解析键：%v\n", err)
		return exitTechnical
	}
	fmt.Fprintf(out, "解析键登记 %s/%s：%s\n",
		registration.TenantID, registration.CustomerAccountID, outcome)
	if outcome == pspartycommercial.ResolutionKeyContentConflict {
		return exitAttention
	}
	return exitLanded
}

func openDatabase(ctx context.Context, getenv func(string) string) (*bentopg.DB, func(), error) {
	dsn := getenv(envDatabaseDSN)
	if dsn == "" {
		return nil, nil, fmt.Errorf("parcel-commercial: %s is required", envDatabaseDSN)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("parcel-commercial: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("parcel-commercial: ping: %w", err)
	}
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("parcel-commercial: framework db: %w", err)
	}
	return db, pool.Close, nil
}
