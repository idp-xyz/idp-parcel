// parcel-access-register 是操作者册的受控登记口（票 operator-channel/01）：登记操作者主体、按能力面
// 授予、撤销授予。
//
// 定位是 ADR-0085 决定一的「受控批量口」（ADR-0100 决定六），与将来的在线口消费同一个登记口
// accessidentity.OperatorRegistrar，答复代数一致。批文每一格都来自输入文件，进程不内置任何生产默认。
// 演示租户只经它登 SYN- 合成主体，证据层级只记 S（参数登记册 PAR-INT-08）。
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

const usage = "用法：parcel-access-register <operator-register|operator-grant|operator-revoke> -input <file>"

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
	default:
		fmt.Fprintf(errOut, "未知子命令 %q；%s\n", args[0], usage)
		return exitTechnical
	}
	return runBatch(ctx, args[0], args[1:], translate, getenv, out, errOut)
}

// batchItem 是翻译产物里的一项：标签供回显，apply 对登记口做这一项的那一次登记。
type batchItem struct {
	label string
	apply func(ctx context.Context, registrar accessidentity.OperatorRegistrar) (accessidentity.OperatorRegistrationOutcome, error)
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
	registry, err := aipostgres.NewOperatorRegistry(db)
	if err != nil {
		fmt.Fprintf(errOut, "构造操作者册：%v\n", err)
		return exitTechnical
	}

	// 批不是聚合：逐项各起事务，某项被拒不撤已落的前项，重跑同一批已落项以重放回答
	// （AT-PC-011 同款纪律）。后项看得见前项，同一批里先登主体、后授予也成立。
	attention := false
	for index, item := range items {
		var outcome accessidentity.OperatorRegistrationOutcome
		err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			var applyErr error
			outcome, applyErr = item.apply(txCtx, registry)
			return applyErr
		})
		label := fmt.Sprintf("第 %d/%d 项 %s", index+1, len(items), item.label)
		if err != nil {
			fmt.Fprintf(errOut, "%s：技术失败，批在此停下：%v\n", label, err)
			return exitTechnical
		}
		fmt.Fprintf(out, "%s：%s\n", label, outcome)
		if !landed(outcome) {
			attention = true
		}
	}
	if attention {
		return exitAttention
	}
	return exitLanded
}

// landed 只认两格为落定；其余一律抬 attention，包括将来新增而此处没跟上的答复——漏认的方向
// 是「多请一次人看」，不是「静静退 0」。
func landed(outcome accessidentity.OperatorRegistrationOutcome) bool {
	switch outcome {
	case accessidentity.OperatorRegistrationRecorded, accessidentity.OperatorRegistrationAlreadyRegistered:
		return true
	}
	return false
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
