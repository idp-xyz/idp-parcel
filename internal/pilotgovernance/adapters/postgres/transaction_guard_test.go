package postgres_test

import (
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// PBC-08 行为面负向证据（bento-gate-reeval 票 02）：留痕写口在无事务上下文必须被
// RequireExecutor 拒绝。留痕完整性校验在守卫之前（留白拒绝是它自己的门），所以这里
// 用一条完整轨迹让请求走到守卫——断言只看 ErrTransactionRequired。
func TestChannelExecutionWritesRefuseToRunOutsideATransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	ctx := t.Context()

	executions, err := adapter.NewChannelExecutions(db)
	if err != nil {
		t.Fatalf("构造留痕库：%v", err)
	}
	trace := adapter.ChannelExecution{
		Command:         "register-authority-interval",
		RecordReference: "interval-ntx",
		OSUser:          "operator",
		Hostname:        "workstation",
		Outcome:         "REGISTERED",
		ExecutedAt:      time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC),
	}
	if err := executions.Append(ctx, trace); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务追加留痕应返回 ErrTransactionRequired，实得：%v", err)
	}
}
