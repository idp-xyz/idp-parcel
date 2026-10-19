package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件钉设备收寄路上意图入队失败的去向（票 operator-channel/45）：响应里没有续办这一格，设备收到 2xx 就出队，所以意图
// 没交出去的收寄不提交——整笔回滚、答 500，设备重发时收寄与意图在同一笔里落地（UC-NO-002 步骤 8）。编排与库走生产装配
// 的依赖，只把意图交付换成会失败的那一只。夹具全为合成（S 级）。

// errIntakeHandoffDown 是注入的入队失败。它是 Go 层错误、不碰库：入队报 SQL 级错误时事务已中止，那一格本来就整笔回滚，
// 测不出编排吞错之后的去向。
var errIntakeHandoffDown = errors.New("SYN 注入：意图入队失败")

// failingIntakeHandoff 先答 failures 次注入的失败，之后转给生产的意图交付。
type failingIntakeHandoff struct {
	inner    noports.NodeIntakeHandoff
	failures int
}

func (handoff *failingIntakeHandoff) HandOffNodeIntake(ctx context.Context, intent noports.NodeIntakeHandoffIntent) error {
	if handoff.failures > 0 {
		handoff.failures--
		return errIntakeHandoffDown
	}
	return handoff.inner.HandOffNodeIntake(ctx, intent)
}

// receptionWithFailingHandoff 在真库上备一件带客户包裹件号、已接受的包裹，交回照生产依赖装的收寄编排——意图交付换成先失败
// failures 次的那一只——与一条按那个号码明确接收的命令。只有形成了的收寄才交意图，所以命令要唯一识别得出包裹。
func receptionWithFailingHandoff(
	t *testing.T,
	sourceID string,
	failures int,
) (*pgxpool.Pool, nodeopshttp.ReceptionHandler, nodeopsapp.ReceiveDeliveredUnitCommand) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	acceptSubmitted(t, db, submitWithCustomerParcelNumber(t, db, "SYN-CUSTOMER-PARCEL-0001"))
	deps, err := buildReceptionDeps(db)
	if err != nil {
		t.Fatalf("装配收寄依赖：%v", err)
	}
	deps.Downstream = &failingIntakeHandoff{inner: deps.Downstream, failures: failures}
	reception, err := receptionWithin(db, deps)
	if err != nil {
		t.Fatalf("装配收寄编排：%v", err)
	}
	command := receptionCommand(t, sourceID, nodeopsapp.ExplicitReception)
	command.Mark = noports.ExternalMarkObservation{Mark: "SYN-CUSTOMER-PARCEL-0001"}
	return pool, reception, command
}

// Covers: 票 operator-channel/45 裁定一——设备收寄路不接受意图未交出的结果。入队以 Go 层错误失败：端点答 500
// NO_ANSWER_FORMED、不带 outcome，收寄零行、意图零封；再失败一次时编排交回的错包着那次入队的错。入队恢复后同一来源重投：
// 答 201 INTAKE_FORMED、带关联，收寄一行、意图一封——重发时收寄重新形成，意图与它同一笔入队。
func TestTheWiredReceptionRollsBackWhenItsIntentIsNotHandedOff(t *testing.T) {
	pool, reception, command := receptionWithFailingHandoff(t, "SYN-DELIVERY-OC45-ROLLBACK", 2)

	failed := postWiredReception(t, reception, command)
	if failed.status != http.StatusInternalServerError || !strings.Contains(failed.raw, "NO_ANSWER_FORMED") || failed.Outcome != "" {
		t.Fatalf("意图没交出去 %d %s（收寄 %d 行、意图 %d 封）, want 500 NO_ANSWER_FORMED 且不带 outcome——2xx 会让设备出队，没人再重投",
			failed.status, failed.raw, receptionRows(t, pool, command.SourceID), nodeIntakeIntents(t, pool))
	}
	if got := receptionRows(t, pool, command.SourceID); got != 0 {
		t.Fatalf("收寄 %d 行, want 0——意图没跟上的收寄不该提交", got)
	}
	if got := nodeIntakeIntents(t, pool); got != 0 {
		t.Fatalf("意图 %d 封, want 0", got)
	}
	if _, err := reception.Handle(t.Context(), command); !errors.Is(err, errIntakeHandoffDown) {
		t.Fatalf("意图没交出去时编排交回 err = %v, want 包着那次入队的错", err)
	}

	retried := postWiredReception(t, reception, command)
	if retried.status != http.StatusCreated || retried.Outcome != "INTAKE_FORMED" || retried.Association == "" {
		t.Fatalf("入队恢复后重投 %d %s, want 201 INTAKE_FORMED 带关联", retried.status, retried.raw)
	}
	if got := receptionRows(t, pool, command.SourceID); got != 1 {
		t.Fatalf("收寄 %d 行, want 1", got)
	}
	if got := nodeIntakeIntents(t, pool); got != 1 {
		t.Fatalf("意图 %d 封, want 1——收寄与意图该在同一笔里落地", got)
	}
}

// Covers: 票 operator-channel/45 裁定三末条的另一半——收寄口把来源标识限在收寄意图信封的 Subject 上限以内（nodeopshttp 的
// receptionCommand），这里钉信封真收得下上限上那一个：生产装配上，来源标识恰好 eventing.MaxSubjectLength 字节的一次
// 唯一识别收寄答 201 INTAKE_FORMED、意图一封。意图交付若改了 Subject 的取法（加前缀之类），这一格先变 500，接入那道
// 上限就得跟着改。
func TestTheWiredReceptionHandsOffTheIntentForTheLongestSourceIDTheIntakeAccepts(t *testing.T) {
	const prefix = "SYN-DELIVERY-OC45/"
	longest := prefix + strings.Repeat("9", eventing.MaxSubjectLength-len(prefix))
	pool, reception, command := wiredIdentifiedReception(t, longest)

	answer := postWiredReception(t, reception, command)
	if answer.status != http.StatusCreated || answer.Outcome != "INTAKE_FORMED" {
		t.Fatalf("上限上的来源标识 %d %s, want 201 INTAKE_FORMED", answer.status, answer.raw)
	}
	if got := nodeIntakeIntents(t, pool); got != 1 {
		t.Fatalf("意图 %d 封, want 1", got)
	}
}
