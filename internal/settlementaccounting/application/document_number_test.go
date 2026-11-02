package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉对账单、费用争议与审核应付的取号编排（ADR-0355 决定三、七、八、九；票 document-numbering/05）：号在确定落单的那一步取，答业务
// 答案而不落单的各路不取；日期所用时区取结算账户对账安排的业务时区；撞号答「编号规则撞号」、不落单；时区未配置时争议与应付照常
// 开立而不带号，对账单交回错误（它的单号是寻址键）；重放答原号、不再取号。真库上的事务参与、并发、租户隔离与周期重置由
// adapters/postgres 的 document_number_test 钉。

// Covers: `AT-SA-222`（编排半边）——单号在勾稽过了之后按编号规则取，日期按这一户对账安排的业务时区；本期无费用、勾稽不平、发布
// 意图冲突都不取号；撞号不落单、不交意图。
func TestAStatementTakesItsNumberOnlyWhenItIsPublished(t *testing.T) {
	fixture := newStatementFixture(t)
	fixture.arrangements.register(t, "ACCOUNT/syn-1", "Asia/Shanghai", 18*60)

	imbalanced := publishCommand(t)
	imbalanced.DeclaredTotalMinor = 19999
	if result, err := fixture.handler.Publish(context.Background(), imbalanced); err != nil || result.Outcome() != application.StatementImbalanceOutcome {
		t.Fatalf("imbalance: %v %q", err, result.Outcome())
	}
	if len(fixture.numbers.requests) != 0 {
		t.Fatal("勾稽不平答业务答案、不落单，却先取了号")
	}

	published, err := fixture.handler.Publish(context.Background(), publishCommand(t))
	if err != nil || published.Outcome() != application.StatementPublished {
		t.Fatalf("publish: %v %q", err, published.Outcome())
	}
	record, _ := published.Statement()
	if record.Key.Number.String() != "STMT-2026-08-001" || record.Statement.Number() != record.Key.Number ||
		record.DraftVersion.String() != "draft/v1" {
		t.Fatalf("record = %+v", record.Key)
	}
	requests := fixture.numbers.requestsOf(domain.CustomerStatementDocumentNumberType.Code)
	if len(requests) != 1 || requests[0].IssuerTimeZone != "Asia/Shanghai" || requests[0].Tenant.String() != "tenant-1" {
		t.Fatalf("取号请求 = %+v，要按结算账户对账安排的业务时区取一次", requests)
	}

	nothing := publishCommand(t)
	nothing.Version = "draft/v2"
	nothing.DeclaredTotalMinor = 0
	if result, err := fixture.handler.Publish(context.Background(), nothing); err != nil || result.Outcome() != application.StatementNoChargeInPeriod {
		t.Fatalf("nothing in period: %v %q", err, result.Outcome())
	}
	if len(fixture.numbers.requests) != 1 {
		t.Fatal("本期无费用答业务答案、不落单，却取了号")
	}

	t.Run("a collision publishes nothing", func(t *testing.T) {
		collided := newStatementFixture(t)
		collided.numbers.outcome = ports.DocumentNumberRuleCollision
		result, err := collided.handler.Publish(context.Background(), publishCommand(t))
		if err != nil || result.Outcome() != application.StatementNumberingRuleCollision || result.Outcome().String() != "NUMBERING_RULE_COLLISION" {
			t.Fatalf("collision: %v %q", err, result.Outcome())
		}
		if _, has := result.Statement(); has || len(collided.statements.records) != 0 || len(collided.handoff.intents) != 0 {
			t.Fatal("撞号还落了单或交了意图")
		}
	})

	t.Run("a statement without a business time zone is a contradiction", func(t *testing.T) {
		unzoned := newStatementFixture(t)
		unzoned.numbers.outcome = ports.DocumentNumberTimeZoneUnconfigured
		_, err := unzoned.handler.Publish(context.Background(), publishCommand(t))
		if !errors.Is(err, application.ErrStatementNumberingTimeZoneMissing) || len(unzoned.statements.records) != 0 {
			t.Fatalf("err = %v：对账单号是寻址键，不能不带号发布", err)
		}
	})

	t.Run("a save lost after the number was taken rolls back", func(t *testing.T) {
		raced := newStatementFixture(t)
		raced.statements.raceSaved = true
		if _, err := raced.handler.Publish(context.Background(), publishCommand(t)); !errors.Is(err, application.ErrNumberedIssueInterrupted) {
			t.Fatalf("err = %v，号已取走就要交回错误让事务回滚", err)
		}
	})

	t.Run("a numbering failure is returned for the transaction to roll back", func(t *testing.T) {
		failing := newStatementFixture(t)
		failing.numbers.err = errors.New("sequence down")
		if _, err := failing.handler.Publish(context.Background(), publishCommand(t)); err == nil || len(failing.statements.records) != 0 {
			t.Fatalf("err = %v", err)
		}
	})
}

// Covers: ADR-0355 Consequences「本决定生效前截出的对账单，其意图照样能查到」——之前手工发布、没记下截止版本的单，按内容认：内容
// 相同答原单原号、不取号；内容不同照新意图发，已在那张单上的费用照样被排除。
func TestAStatementPublishedBeforeNumberingIsFoundByItsIntent(t *testing.T) {
	fixture := newStatementFixture(t)
	if _, err := fixture.handler.Publish(context.Background(), publishCommand(t)); err != nil {
		t.Fatal(err)
	}
	// 把那一张改成升级前的样子：手工填的单号、没记截止版本。
	var legacy ports.StatementRecord
	for key, record := range fixture.statements.records {
		delete(fixture.statements.records, key)
		legacy = record
	}
	statement, err := domain.RehydratePublishedStatement(domain.RehydratePublishedStatementSpec{
		Number: billValue(t, domain.NewStatementNumber, "SYN-STMT-MANUAL-1"), Account: legacy.Statement.Account(),
		Period: legacy.Statement.Period(), Currency: legacy.Statement.Currency(), Lines: legacy.Statement.Lines(),
		TotalMinor: legacy.Statement.TotalMinor(), PublishedAt: legacy.Statement.PublishedAt(),
	})
	if err != nil {
		t.Fatal(err)
	}
	legacy.Key.Number, legacy.Statement, legacy.DraftVersion = statement.Number(), statement, domain.StatementDraftVersion{}
	fixture.statements.records[statementKey(legacy.Key)] = legacy
	taken := len(fixture.numbers.requests)

	replay, err := fixture.handler.Publish(context.Background(), publishCommand(t))
	if err != nil || replay.Outcome() != application.StatementExistingResult {
		t.Fatalf("replay: %v %q", err, replay.Outcome())
	}
	if record, _ := replay.Statement(); record.Key.Number.String() != "SYN-STMT-MANUAL-1" || len(fixture.numbers.requests) != taken {
		t.Fatalf("升级前那张单按意图答原号：%s，取号 %d 次", record.Key.Number, len(fixture.numbers.requests)-taken)
	}

	changed := publishCommand(t)
	changed.ChargeIDs = []string{"charge-A"}
	changed.DeclaredTotalMinor = 0
	result, err := fixture.handler.Publish(context.Background(), changed)
	if err != nil || result.Outcome() != application.StatementNoChargeInPeriod {
		t.Fatalf("changed content: %v %q：认不出是同一意图，照新意图走，已出账的照样排除", err, result.Outcome())
	}
}

// Covers: `AT-SA-230`（编排半边）——争议登记时取争议号，日期按被争对账单那个结算账户对账安排的业务时区；重放答原来那一项连同
// 它的号、不再取号；领域门拒了的（金额越行、单已作废）不取号；时区未配置照常登记、记无号原因；撞号不登记。
func TestADisputeTakesItsNumberWhenItIsRegistered(t *testing.T) {
	fixture := newStatementFixture(t)
	fixture.arrangements.register(t, "ACCOUNT/syn-1", "Asia/Shanghai", 18*60)
	if _, err := fixture.handler.Publish(context.Background(), publishCommand(t)); err != nil {
		t.Fatal(err)
	}
	tenant, _ := domain.NewTenantID("tenant-1")
	open := application.OpenDisputeCommand{
		TenantID: tenant, Dispute: "dispute-1", Number: "STMT-2026-08-001", ChargeID: "charge-A",
		DisputedMinor: 5000, Reason: "weight", OpenedAt: statementPublishAt.Add(time.Hour),
	}
	fixture.clock.at = statementPublishAt.Add(2 * time.Hour)

	over := open
	over.Dispute, over.DisputedMinor = "dispute-over", 12001
	if result, err := fixture.handler.OpenDispute(context.Background(), over); err != nil || result.Outcome() != application.StatementNotAccepted {
		t.Fatalf("over: %v %q", err, result.Outcome())
	}
	if len(fixture.numbers.requestsOf(domain.StatementDisputeDocumentNumberType.Code)) != 0 {
		t.Fatal("未受理的登记取了号")
	}

	opened, err := fixture.handler.OpenDispute(context.Background(), open)
	if err != nil || opened.Outcome() != application.DisputeOpened {
		t.Fatalf("open: %v %q", err, opened.Outcome())
	}
	record, _ := opened.Dispute()
	if number, numbered := record.Numbering.Number(); !numbered || number.String() != "DSP-2026-09-001" {
		t.Fatalf("争议号 = %+v", record.Numbering)
	}
	requests := fixture.numbers.requestsOf(domain.StatementDisputeDocumentNumberType.Code)
	if len(requests) != 1 || requests[0].IssuerTimeZone != "Asia/Shanghai" {
		t.Fatalf("取号请求 = %+v，要按被争对账单那个账户的业务时区", requests)
	}

	replay, err := fixture.handler.OpenDispute(context.Background(), open)
	if err != nil || replay.Outcome() != application.DisputeExistingResult {
		t.Fatalf("replay: %v %q", err, replay.Outcome())
	}
	if again, _ := replay.Dispute(); again.Numbering != record.Numbering ||
		len(fixture.numbers.requestsOf(domain.StatementDisputeDocumentNumberType.Code)) != 1 {
		t.Fatal("重放换了号或又取了号")
	}

	t.Run("an unconfigured time zone still registers without a number", func(t *testing.T) {
		fixture.numbers.outcome = ports.DocumentNumberTimeZoneUnconfigured
		defer func() { fixture.numbers.outcome = ports.DocumentNumberOutcomeInvalid }()
		unnumbered := open
		unnumbered.Dispute = "dispute-unnumbered"
		result, err := fixture.handler.OpenDispute(context.Background(), unnumbered)
		if err != nil || result.Outcome() != application.DisputeOpened {
			t.Fatalf("open: %v %q", err, result.Outcome())
		}
		record, _ := result.Dispute()
		if absence, absent := record.Numbering.Absence(); !absent || absence != domain.NumberingTimeZoneUnconfigured {
			t.Fatalf("无号原因 = %+v", record.Numbering)
		}
	})

	t.Run("a collision registers nothing", func(t *testing.T) {
		fixture.numbers.outcome = ports.DocumentNumberRuleCollision
		defer func() { fixture.numbers.outcome = ports.DocumentNumberOutcomeInvalid }()
		collided := open
		collided.Dispute = "dispute-collided"
		result, err := fixture.handler.OpenDispute(context.Background(), collided)
		if err != nil || result.Outcome() != application.StatementNumberingRuleCollision {
			t.Fatalf("collision: %v %q", err, result.Outcome())
		}
		if _, saved := fixture.disputes.records["tenant-1|dispute-collided"]; saved {
			t.Fatal("撞号还登记了争议")
		}
	})
}

// Covers: `AT-SA-231`（编排半边）——形成审核应付时取应付号：应付账户登了对账安排就按它的业务时区，没登交空、由编号组件退到租户的
// 单据编号时区；重放答原应付连同它的号、不再取号；行不可审与要升级不取号；先排队再查重；时区未配置照常形成、记无号原因；撞号不
// 形成应付。
func TestAnAuditedPayableTakesItsNumberWhenItIsFormed(t *testing.T) {
	fixture := receivedFixture(t)

	if result, err := fixture.handler.Audit(context.Background(), auditCommand(t, "bill-1", "line-2", "payable-2")); err != nil ||
		result.Outcome() != application.LineNotAuditable {
		t.Fatalf("variance line: %v %q", err, result.Outcome())
	}
	if len(fixture.numbers.requests) != 0 {
		t.Fatal("行不可审却取了号")
	}

	formed, err := fixture.handler.Audit(context.Background(), auditCommand(t, "bill-1", "line-1", "payable-1"))
	if err != nil || formed.Outcome() != application.PayableFormed {
		t.Fatalf("audit: %v %q", err, formed.Outcome())
	}
	record, _ := formed.Payable()
	if number, numbered := record.Numbering.Number(); !numbered || number.String() != "AP-2026-09-001" {
		t.Fatalf("应付号 = %+v", record.Numbering)
	}
	if requests := fixture.numbers.requestsOf(domain.AuditedPayableDocumentNumberType.Code); len(requests) != 1 || requests[0].IssuerTimeZone != "" {
		t.Fatalf("取号请求 = %+v，应付账户没登对账安排就交空", requests)
	}
	if calls := fixture.payables.calls; calls[2] != "lock payable-1 bill-1/line-1" || calls[3] != "find payable-1" {
		t.Fatalf("calls = %v，要先排队再查重", fixture.payables.calls)
	}

	replay, err := fixture.handler.Audit(context.Background(), auditCommand(t, "bill-1", "line-1", "payable-1"))
	if err != nil || replay.Outcome() != application.PayableExistingResult {
		t.Fatalf("replay: %v %q", err, replay.Outcome())
	}
	if again, _ := replay.Payable(); again.Numbering != record.Numbering || len(fixture.numbers.requests) != 1 {
		t.Fatal("重放换了号或又取了号")
	}

	t.Run("an arranged payable account dates the number in its time zone", func(t *testing.T) {
		arranged := receivedFixture(t)
		arranged.arrangements.register(t, "supplier-account-1", "Europe/Berlin", 0)
		if _, err := arranged.handler.Audit(context.Background(), auditCommand(t, "bill-1", "line-1", "payable-1")); err != nil {
			t.Fatal(err)
		}
		if requests := arranged.numbers.requests; len(requests) != 1 || requests[0].IssuerTimeZone != "Europe/Berlin" {
			t.Fatalf("取号请求 = %+v", requests)
		}
	})

	t.Run("an unconfigured time zone still forms the payable without a number", func(t *testing.T) {
		unnumbered := receivedFixture(t)
		unnumbered.numbers.outcome = ports.DocumentNumberTimeZoneUnconfigured
		result, err := unnumbered.handler.Audit(context.Background(), auditCommand(t, "bill-1", "line-1", "payable-1"))
		if err != nil || result.Outcome() != application.PayableFormed {
			t.Fatalf("audit: %v %q", err, result.Outcome())
		}
		record, _ := result.Payable()
		if absence, absent := record.Numbering.Absence(); !absent || absence != domain.NumberingTimeZoneUnconfigured {
			t.Fatalf("无号原因 = %+v", record.Numbering)
		}
	})

	t.Run("a collision forms nothing", func(t *testing.T) {
		collided := receivedFixture(t)
		collided.numbers.outcome = ports.DocumentNumberRuleCollision
		result, err := collided.handler.Audit(context.Background(), auditCommand(t, "bill-1", "line-1", "payable-1"))
		if err != nil || result.Outcome() != application.PayableNumberingRuleCollision || result.Outcome().String() != "NUMBERING_RULE_COLLISION" {
			t.Fatalf("collision: %v %q", err, result.Outcome())
		}
		if len(collided.payables.records) != 0 || len(collided.handoff.intents) != 1 {
			t.Fatal("撞号还形成了应付或交了应付意图")
		}
	})

	t.Run("an escalated amount takes no number", func(t *testing.T) {
		escalated := receivedFixture(t)
		ceiling, err := domain.NewAuditEscalationCeiling(1)
		if err != nil {
			t.Fatal(err)
		}
		escalated.escalation.ceiling = ceiling
		if result, err := escalated.handler.Audit(context.Background(), auditCommand(t, "bill-1", "line-1", "payable-1")); err != nil ||
			result.Outcome() != application.AuditMustEscalate || len(escalated.numbers.requests) != 0 {
			t.Fatalf("escalate: %v %q 取号 %d 次", err, result.Outcome(), len(escalated.numbers.requests))
		}
	})
}
