// demo-seeds/migrate 是演示种子包的迁移施加助手，**仅限隔离环境**。
//
// 迁移计划（internal/platform/migrate）刻意没有生产入口——迁移作业的连接生命周期、
// 凭据与部署窗口归部署方，不归应用进程。隔离演示库没有那套部署面，本助手替它补上
// 「连接并施加计划」这一步；它不属于任何生产路径，也绝不能指向生产库。
//
// -reset 把计划里的全部 schema 连带 parcel_migration 历史一起 DROP CASCADE 后重迁，
// 供「干净库复灌」使用；这是演示库才允许的破坏性动作，助手不做任何「这是不是演示库」
// 的猜测，指错库的后果由指库的人担——与登记口「不猜连接串」同一条纪律。
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

func main() {
	reset := flag.Bool("reset", false, "先 DROP 全部 parcel schema（CASCADE）再重新施加迁移计划——仅限隔离演示库")
	flag.Parse()

	dsn := os.Getenv("IDP_PARCEL_POSTGRES_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "IDP_PARCEL_POSTGRES_DSN 未设置——助手不猜连接串")
		os.Exit(1)
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接数据库：%v\n", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close(ctx) }()

	if *reset {
		for _, schema := range migrate.Schemas() {
			// schema 名来自迁移计划常量，绝不来自输入，因此可以直接拼接。
			if _, err := conn.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`); err != nil {
				fmt.Fprintf(os.Stderr, "重置 schema %s：%v\n", schema, err)
				os.Exit(1)
			}
			fmt.Printf("已重置 schema %s\n", schema)
		}
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := migrate.Run(ctx, conn, logger); err != nil {
		fmt.Fprintf(os.Stderr, "施加迁移计划：%v\n", err)
		os.Exit(1)
	}
}
