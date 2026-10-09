// parcel-access-register 是 accessidentity 两本册的受控登记口：操作者册（票 operator-channel/01）登记操作者主体、
// 按能力面授予、撤销授予；集成客户端册（票 operator-channel/11）登记客户端主体、按事实类型授予、撤销授予。
//
// 定位是 ADR-0085 决定一的「受控批量口」（ADR-0100 决定六），与将来的在线口消费同一组登记口
// accessidentity.OperatorRegistrar、accessidentity.IntegrationClientRegistrar，答复代数一致。批文每一格都来自
// 输入文件，进程不内置任何生产默认。演示租户只经它登 SYN- 合成主体，证据层级只记 S（参数登记册 PAR-INT-08、
// PAR-INT-09）。
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipostgres "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

const envDatabaseDSN = "IDP_PARCEL_POSTGRES_DSN"

// 退出码把「谁来做下一步」分开：0 = 全部落定（含重放）；1 = 技术失败，运维重试；2 = 有冲突或被拒，
// 管操作者与授予的人要看报告。
const (
	exitLanded    = 0
	exitTechnical = 1
	exitAttention = 2
)

const usage = "用法：parcel-access-register <operator-register|operator-grant|operator-revoke|" +
	"integration-client-register|integration-client-grant|integration-client-revoke> -input <file>"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, getenv func(string) string, out, errOut io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(errOut, usage)
		return exitTechnical
	}
	var translate func([]byte) ([]batchItem, error)
	switch args[0] {
	case "operator-register":
		translate = operatorBatchFromJSON
	case "operator-grant":
		translate = grantBatchFromJSON
	case "operator-revoke":
		translate = revocationBatchFromJSON
	case "integration-client-register":
		translate = integrationClientBatchFromJSON
	case "integration-client-grant":
		translate = integrationClientGrantBatchFromJSON
	case "integration-client-revoke":
		translate = integrationClientRevocationBatchFromJSON
	default:
		fmt.Fprintf(errOut, "未知子命令 %q；%s\n", args[0], usage)
		return exitTechnical
	}
	return runBatch(ctx, args[0], args[1:], translate, getenv, out, errOut)
}

// registers 是一批里各项可能要落的两本册的登记口；每一项只用它那一本。
type registers struct {
	operators accessidentity.OperatorRegistrar
	clients   accessidentity.IntegrationClientRegistrar
}

// outcome 是一项登记的答复名，与它算不算落定。两本册的答复各是各的类型，回显与退出码只要这两样。
type outcome struct {
	name   string
	landed bool
}

// batchItem 是翻译产物里的一项：标签供回显，apply 对登记口做这一项的那一次登记。
type batchItem struct {
	label string
	apply func(ctx context.Context, registers registers) (outcome, error)
}

func runBatch(
	ctx context.Context,
	name string,
	args []string,
	translate func([]byte) ([]batchItem, error),
	getenv func(string) string,
	out, errOut io.Writer,
) int {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(errOut)
	input := flags.String("input", "", "登记批 JSON 文件路径")
	if err := flags.Parse(args); err != nil {
		return exitTechnical
	}
	if *input == "" {
		fmt.Fprintf(errOut, "%s 需要 -input <file>\n", name)
		return exitTechnical
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(errOut, "读登记批：%v\n", err)
		return exitTechnical
	}
	items, err := translate(raw)
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
	operators, err := aipostgres.NewOperatorRegistry(db)
	if err != nil {
		fmt.Fprintf(errOut, "构造操作者册：%v\n", err)
		return exitTechnical
	}
	clients, err := aipostgres.NewIntegrationClientRegistry(db)
	if err != nil {
		fmt.Fprintf(errOut, "构造集成客户端册：%v\n", err)
		return exitTechnical
	}
	books := registers{operators: operators, clients: clients}

	// 批不是聚合：逐项各起事务，某项被拒不撤已落的前项，重跑同一批已落项以重放回答
	// （AT-PC-011 同款纪律）。后项看得见前项，同一批里先登主体、后授予也成立。
	attention := false
	for index, item := range items {
		var result outcome
		err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			var applyErr error
			result, applyErr = item.apply(txCtx, books)
			return applyErr
		})
		label := fmt.Sprintf("第 %d/%d 项 %s", index+1, len(items), item.label)
		if err != nil {
			fmt.Fprintf(errOut, "%s：技术失败，批在此停下：%v\n", label, err)
			return exitTechnical
		}
		fmt.Fprintf(out, "%s：%s\n", label, result.name)
		if !result.landed {
			attention = true
		}
	}
	if attention {
		return exitAttention
	}
	return exitLanded
}

// operatorOutcome 与 clientOutcome 只认两格为落定；其余一律抬 attention，包括将来新增而此处没跟上的答复——漏认的
// 方向是「多请一次人看」，不是「静静退 0」。
func operatorOutcome(answer accessidentity.OperatorRegistrationOutcome) outcome {
	switch answer {
	case accessidentity.OperatorRegistrationRecorded, accessidentity.OperatorRegistrationAlreadyRegistered:
		return outcome{name: answer.String(), landed: true}
	}
	return outcome{name: answer.String()}
}

func clientOutcome(answer accessidentity.IntegrationClientRegistrationOutcome) outcome {
	switch answer {
	case accessidentity.IntegrationClientRegistrationRecorded, accessidentity.IntegrationClientRegistrationAlreadyRegistered:
		return outcome{name: answer.String(), landed: true}
	}
	return outcome{name: answer.String()}
}

func openDatabase(ctx context.Context, getenv func(string) string) (*bentopg.DB, func(), error) {
	dsn := getenv(envDatabaseDSN)
	if dsn == "" {
		return nil, nil, fmt.Errorf("parcel-access-register: %s is required", envDatabaseDSN)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("parcel-access-register: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("parcel-access-register: ping: %w", err)
	}
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("parcel-access-register: framework db: %w", err)
	}
	return db, pool.Close, nil
}
