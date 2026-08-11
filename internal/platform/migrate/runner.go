package migrate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
)

// historyDDL 记录实际施加了什么。除了迁移 ID，还记规范校验和、框架版本与真实
// schema，使一个已部署的数据库能追回到不可变工件。
const historyDDL = `
CREATE TABLE IF NOT EXISTS ` + SchemaHistory + `.applied_migration (
    migration_id       text        NOT NULL PRIMARY KEY,
    sequence           integer     NOT NULL,
    origin             text        NOT NULL,
    canonical_checksum text        NOT NULL,
    framework_version  text,
    target_schema      text        NOT NULL,
    applied_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT applied_migration_origin_known
        CHECK (origin IN ('FRAMEWORK', 'PARCEL')),
    CONSTRAINT applied_migration_framework_version_present
        CHECK (origin <> 'FRAMEWORK' OR framework_version IS NOT NULL)
)`

// advisoryLockKey 让并发的迁移作业排队而不是各跑各的。取值任意但必须稳定，
// 换一个值等于换一把锁，两边就都拿得到。
const advisoryLockKey int64 = 6_820_251_101

// ErrChecksumDrift 表示某个已施加的迁移与本次构建携带的工件不再一致。它阻断本次
// 运行，既不重放也不静默接受一份被改写过的模板。
var ErrChecksumDrift = errors.New("migrate: 已施加迁移的校验和与当前工件不一致")

// Run 对一条连接施加完整的 Parcel 迁移计划。连接生命周期、凭据与部署窗口由调用方
// 拥有；本包的任何东西都不可从应用进程到达。
//
// 不引入独立的版本计数器：`applied_migration` 已经记下每一步是否施加过，再加一个
// 计数器等于给同一件事立第二个口径，两者一旦不一致就没人说得清该信哪个。
func Run(ctx context.Context, conn *pgx.Conn, logger *slog.Logger) error {
	plan, err := Plan()
	if err != nil {
		return err
	}

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockKey); err != nil {
		return fmt.Errorf("migrate: 取迁移锁：%w", err)
	}
	defer func() {
		// 解锁失败不改变本次结果：会话结束时锁必然释放。
		_, _ = conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, advisoryLockKey)
	}()

	if err := prepare(ctx, conn); err != nil {
		return err
	}

	recorded, err := readHistory(ctx, conn)
	if err != nil {
		return err
	}
	if err := verifyNoDrift(plan, recorded); err != nil {
		return err
	}

	applied := 0
	for index, step := range plan {
		if _, done := recorded[step.ID]; done {
			continue
		}
		if err := applyStep(ctx, conn, index+1, step); err != nil {
			return err
		}
		logger.Info("已施加迁移",
			"sequence", index+1, "migration", step.ID, "schema", step.Schema)
		applied++
	}

	logger.Info("迁移计划完成",
		"steps", len(plan), "applied", applied, "framework_version", FrameworkVersion)
	return nil
}

func prepare(ctx context.Context, conn *pgx.Conn) error {
	for _, schema := range Schemas() {
		// schema 名来自本包常量，绝不来自输入，因此可以直接拼接。
		if _, err := conn.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS `+schema); err != nil {
			return fmt.Errorf("migrate: 创建 schema %s：%w", schema, err)
		}
	}
	if _, err := conn.Exec(ctx, historyDDL); err != nil {
		return fmt.Errorf("migrate: 创建迁移历史表：%w", err)
	}
	return nil
}

func readHistory(ctx context.Context, conn *pgx.Conn) (map[string]string, error) {
	rows, err := conn.Query(ctx,
		`SELECT migration_id, canonical_checksum FROM `+SchemaHistory+`.applied_migration`)
	if err != nil {
		return nil, fmt.Errorf("migrate: 读迁移历史：%w", err)
	}
	defer rows.Close()

	recorded := make(map[string]string)
	for rows.Next() {
		var id, checksum string
		if err := rows.Scan(&id, &checksum); err != nil {
			return nil, fmt.Errorf("migrate: 扫描迁移历史：%w", err)
		}
		recorded[id] = checksum
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("migrate: 读迁移历史：%w", err)
	}
	return recorded, nil
}

// verifyNoDrift 拿每条已记录的迁移与本次构建里的工件比对。被复制改写过的框架模板
// 会在这里失败，而不是等到 `CheckSchema` 报出一个看不出来源的形状差异。
func verifyNoDrift(plan []Step, recorded map[string]string) error {
	for _, step := range plan {
		checksum, present := recorded[step.ID]
		if present && checksum != step.Checksum {
			return fmt.Errorf("%w：%s 记录为 %s，工件为 %s",
				ErrChecksumDrift, step.ID, checksum, step.Checksum)
		}
	}
	return nil
}

// applyStep 在同一个事务里施加 SQL 并写下历史记录。分成两个事务会让历史撒谎：
// 施加成功而记录失败时，下一次运行会把它当作没做过而重放。
//
// **这是一条约束，不是实现细节：本执行器只接受能在事务块内运行的迁移。**
// PostgreSQL 从原理上拒绝在事务块内执行 `CREATE INDEX CONCURRENTLY`、
// `REINDEX CONCURRENTLY`、`VACUUM`、`ALTER SYSTEM` 等语句，这类迁移到这里会直接
// 报错。而其中 `CREATE INDEX CONCURRENTLY` 正是给已有大表加索引又不锁写的标准
// 做法，业务表长大后几乎必然要用。
//
// 届时不要顺手把那一条挑出事务——那等于重新打开上面刚排掉的撞谎窗口，且是在没有
// 设计的情况下打开。它是一个**显式未决**：真要支持，得同时定出该条迁移的失败后
// 人工对账口径，那是一次要摆上台面的取舍。
func applyStep(ctx context.Context, conn *pgx.Conn, sequence int, step Step) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("migrate: 开启 %s 的事务：%w", step.ID, err)
	}
	defer func() {
		_ = tx.Rollback(context.WithoutCancel(ctx))
	}()

	if _, err := tx.Exec(ctx, step.UpSQL); err != nil {
		return fmt.Errorf("migrate: 施加 %s：%w", step.ID, err)
	}

	var frameworkVersion *string
	if step.Origin == OriginFramework {
		version := step.FrameworkVersion
		frameworkVersion = &version
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO `+SchemaHistory+`.applied_migration
			(migration_id, sequence, origin, canonical_checksum, framework_version, target_schema)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		step.ID, sequence, string(step.Origin), step.Checksum, frameworkVersion, step.Schema,
	); err != nil {
		return fmt.Errorf("migrate: 记录 %s：%w", step.ID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("migrate: 提交 %s：%w", step.ID, err)
	}
	return nil
}
