package main

import (
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/collectionremittance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证 buildRegistrar 装配的整条链（隔离合成 S）：七个命令各自
// 贯通「译装 → 用例 → 真库」，重放与内容冲突在真库上分得开，四层来源在真库上仍不互相
// 推导，余额守卫与批次封口按当刻账面生效，冲正是追加而不是删除。
//
// 本口与写口适配器各持一份私有词表映射（领域 String() 与迁移 CHECK），本用例把两份
// 一起钉在同一个词表上——任一份漂了，这里就红。
func TestCollectionRegisterVerticalOnRealPostgres(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registrar, err := buildRegistrar(db)
	if err != nil {
		t.Fatalf("装配登记口：%v", err)
	}
	ctx := t.Context()

	mustExecute := func(command, raw string, wantCode int, wantAnswer string) string {
		t.Helper()
		message, code := execute(ctx, command, []byte(raw), registrar)
		if code != wantCode {
			t.Fatalf("%s 退出码 = %d（%s），要 %d", command, code, message, wantCode)
		}
		if wantAnswer != "" && !strings.Contains(message, wantAnswer) {
			t.Fatalf("%s 答复 = %q，要含 %s", command, message, wantAnswer)
		}
		return message
	}

	// 分户账开立：四维键 + 受托依据。重放是已存在，换受托依据是冲突。
	subledger := func(basis string) string {
		return `{
			"tenantId": "SYN-T1", "customerRef": "SYN-CUST-1", "legalEntityRef": "SYN-ENTITY-1",
			"currency": "EUR", "channelRef": "SYN-CHANNEL-1",
			"custodyBasisRef": "` + basis + `", "openedAt": "2026-08-24T00:00:00Z"
		}`
	}
	mustExecute(commandSubledger, subledger("SYN-COD-V1"), exitRegistered, "REGISTERED")
	mustExecute(commandSubledger, subledger("SYN-COD-V1"), exitRegistered, "EXISTING")
	mustExecute(commandSubledger, subledger("SYN-COD-V2"), exitNeedsReview, "CONTENT_CONFLICT")

	// 代收指令：随委托指定金额与币种，分户账四维随指令固定。
	instruction := func(amount string) string {
		return `{
			"tenantId": "SYN-T1", "instructionRef": "SYN-INSTR-1",
			"parcelRef": "SYN-PARCEL-1", "requirementRef": "SYN-COD-REQ-V1",
			"customerRef": "SYN-CUST-1", "legalEntityRef": "SYN-ENTITY-1",
			"currency": "EUR", "channelRef": "SYN-CHANNEL-1",
			"amountMinor": ` + amount + `, "instructedAt": "2026-08-24T00:30:00Z"
		}`
	}
	mustExecute(commandInstruction, instruction("12000"), exitRegistered, "REGISTERED")
	mustExecute(commandInstruction, instruction("12000"), exitRegistered, "EXISTING")
	mustExecute(commandInstruction, instruction("13000"), exitNeedsReview, "CONTENT_CONFLICT")

	// 孤儿事实：指令不在册时答依据不在册，不等到撞库上的外键才折成依赖故障。
	orphan := `{
		"tenantId": "SYN-T1", "factRef": "SYN-FACT-ORPHAN", "instructionRef": "SYN-INSTR-NEVER",
		"sourceLayer": "RECIPIENT_PAYMENT", "evidenceRef": "SYN-EV-0",
		"currency": "EUR", "amountMinor": 12000, "occurredAt": "2026-08-24T01:00:00Z"
	}`
	mustExecute(commandFact, orphan, exitNeedsReview, "BASIS_MISSING")

	// 四层来源各自登记，互不覆盖。
	fact := func(ref, layer, amount string) string {
		return `{
			"tenantId": "SYN-T1", "factRef": "` + ref + `", "instructionRef": "SYN-INSTR-1",
			"sourceLayer": "` + layer + `", "evidenceRef": "SYN-EV-` + ref + `",
			"currency": "EUR", "amountMinor": ` + amount + `,
			"occurredAt": "2026-08-24T01:00:00Z"
		}`
	}
	mustExecute(commandFact, fact("SYN-FACT-POD", "RECIPIENT_PAYMENT", "12000"), exitRegistered, "REGISTERED")
	mustExecute(commandFact, fact("SYN-FACT-POD", "RECIPIENT_PAYMENT", "12000"), exitRegistered, "EXISTING")
	mustExecute(commandFact, fact("SYN-FACT-POD", "RECIPIENT_PAYMENT", "10000"), exitNeedsReview, "CONTENT_CONFLICT")
	mustExecute(commandFact, fact("SYN-FACT-CREDIT", "OPERATOR_BANK_CREDIT", "10000"), exitRegistered, "REGISTERED")

	posting := func(ref, from, to, amount, basisKind, basis, at string) string {
		return `{
			"tenantId": "SYN-T1", "postingRef": "` + ref + `",
			"fromPosition": "` + from + `", "toPosition": "` + to + `",
			"amountMinor": ` + amount + `, "basisKind": "` + basisKind + `",
			"basisRef": "` + basis + `", "postedAt": "` + at + `"
		}`
	}

	// 入账位置由事实的来源层级定：收件人付款只到渠道在途，落到待清分被拒。
	mustExecute(commandPosting,
		posting("SYN-POST-BAD", "EXTERNAL_SOURCE", "AWAITING_ALLOCATION", "12000",
			"COLLECTION_FACT", "SYN-FACT-POD", "2026-08-24T01:10:00Z"),
		exitUsage, "NOT_ACCEPTED")
	mustExecute(commandPosting,
		posting("SYN-POST-1", "EXTERNAL_SOURCE", "IN_TRANSIT_AT_CHANNEL", "12000",
			"COLLECTION_FACT", "SYN-FACT-POD", "2026-08-24T01:10:00Z"),
		exitRegistered, "REGISTERED")
	mustExecute(commandPosting,
		posting("SYN-POST-1", "EXTERNAL_SOURCE", "IN_TRANSIT_AT_CHANNEL", "12000",
			"COLLECTION_FACT", "SYN-FACT-POD", "2026-08-24T01:10:00Z"),
		exitRegistered, "EXISTING")

	// 只有真实到账那一层撑得起渠道在途 → 待清分，且实收 10000 少于指令 12000。
	mustExecute(commandPosting,
		posting("SYN-POST-2", "IN_TRANSIT_AT_CHANNEL", "AWAITING_ALLOCATION", "10000",
			"COLLECTION_FACT", "SYN-FACT-POD", "2026-08-24T02:00:00Z"),
		exitUsage, "NOT_ACCEPTED")
	mustExecute(commandPosting,
		posting("SYN-POST-2", "IN_TRANSIT_AT_CHANNEL", "AWAITING_ALLOCATION", "10000",
			"COLLECTION_FACT", "SYN-FACT-CREDIT", "2026-08-24T02:00:00Z"),
		exitRegistered, "REGISTERED")

	// 清分只凭代收指令；透支答余额不足（自己一格，不混进未决）。
	mustExecute(commandPosting,
		posting("SYN-POST-BIG", "AWAITING_ALLOCATION", "PAYABLE_TO_CUSTOMER", "12000",
			"ALLOCATION", "SYN-INSTR-1", "2026-08-24T03:00:00Z"),
		exitUnderfunded, "UNDERFUNDED")
	mustExecute(commandPosting,
		posting("SYN-POST-3", "AWAITING_ALLOCATION", "PAYABLE_TO_CUSTOMER", "8000",
			"ALLOCATION", "SYN-INSTR-1", "2026-08-24T03:00:00Z"),
		exitRegistered, "REGISTERED")

	// 差异事项只登事项；差额落账另有一笔以它为依据的记账，金额必须与事项一致。
	discrepancy := `{
		"tenantId": "SYN-T1", "discrepancyRef": "SYN-DIFF-1", "instructionRef": "SYN-INSTR-1",
		"kind": "SHORTFALL", "currency": "EUR", "amountMinor": 2000,
		"basisRef": "SYN-CHANNEL-REPORT-1", "observedAt": "2026-08-25T00:00:00Z"
	}`
	mustExecute(commandDiscrepancy, discrepancy, exitRegistered, "REGISTERED")
	mustExecute(commandDiscrepancy, discrepancy, exitRegistered, "EXISTING")
	mustExecute(commandPosting,
		posting("SYN-POST-DIFF-BAD", "AWAITING_ALLOCATION", "SHORTFALL", "1500",
			"DISCREPANCY", "SYN-DIFF-1", "2026-08-25T01:00:00Z"),
		exitUsage, "NOT_ACCEPTED")
	mustExecute(commandPosting,
		posting("SYN-POST-DIFF", "AWAITING_ALLOCATION", "SHORTFALL", "2000",
			"DISCREPANCY", "SYN-DIFF-1", "2026-08-25T01:00:00Z"),
		exitRegistered, "REGISTERED")

	// 回汇批次：账未开立时依据不在册；形成后收汇付记账，交出主张后封口。
	batch := func(ref, customer, through string) string {
		return `{
			"tenantId": "SYN-T1", "batchRef": "` + ref + `", "customerRef": "` + customer + `",
			"legalEntityRef": "SYN-ENTITY-1", "currency": "EUR", "channelRef": "SYN-CHANNEL-1",
			"collectedThrough": "` + through + `", "formedAt": "2026-08-31T01:00:00Z"
		}`
	}
	mustExecute(commandBatch, batch("SYN-BATCH-NOLEDGER", "SYN-CUST-NEVER", "2026-08-31T00:00:00Z"),
		exitNeedsReview, "BASIS_MISSING")
	mustExecute(commandBatch, batch("SYN-BATCH-1", "SYN-CUST-1", "2026-08-31T00:00:00Z"),
		exitRegistered, "REGISTERED")
	mustExecute(commandBatch, batch("SYN-BATCH-1", "SYN-CUST-1", "2026-08-31T00:00:00Z"),
		exitRegistered, "EXISTING")
	mustExecute(commandBatch, batch("SYN-BATCH-1", "SYN-CUST-1", "2026-09-07T00:00:00Z"),
		exitNeedsReview, "CONTENT_CONFLICT")

	mustExecute(commandPosting,
		posting("SYN-POST-4", "PAYABLE_TO_CUSTOMER", "REMITTED", "5000",
			"REMITTANCE_BATCH", "SYN-BATCH-1", "2026-08-31T02:00:00Z"),
		exitRegistered, "REGISTERED")

	handOver := `{"tenantId": "SYN-T1", "batchRef": "SYN-BATCH-1"}`
	mustExecute(commandHandOver, handOver, exitRegistered, "HANDED_OVER")
	mustExecute(commandHandOver, handOver, exitRegistered, "ALREADY_HANDED_OVER")
	mustExecute(commandHandOver, `{"tenantId": "SYN-T1", "batchRef": "SYN-BATCH-NEVER"}`,
		exitNeedsReview, "BASIS_MISSING")

	// 交出主张之后不再收成员：成员只增不改的界就在那一刻。
	mustExecute(commandPosting,
		posting("SYN-POST-LATE", "PAYABLE_TO_CUSTOMER", "REMITTED", "3000",
			"REMITTANCE_BATCH", "SYN-BATCH-1", "2026-08-31T04:00:00Z"),
		exitNeedsReview, "BATCH_CLOSED")

	// 冲正是追加不是删除：反向同额落账，原记账仍在账上。
	mustExecute(commandPosting,
		posting("SYN-POST-FIX-BAD", "PAYABLE_TO_CUSTOMER", "AWAITING_ALLOCATION", "1000",
			"CORRECTION", "SYN-POST-3", "2026-08-31T05:00:00Z"),
		exitUsage, "NOT_ACCEPTED")
	mustExecute(commandPosting,
		posting("SYN-POST-FIX", "PAYABLE_TO_CUSTOMER", "AWAITING_ALLOCATION", "8000",
			"CORRECTION", "SYN-POST-3", "2026-08-31T05:00:00Z"),
		exitUnderfunded, "UNDERFUNDED")

	// 账面终态由真库记账派生，分配守恒在真库上核得住。
	view, err := adapter.NewSubledgerBalanceView(db)
	if err != nil {
		t.Fatalf("构造分户账读口：%v", err)
	}
	tenant, err := domain.NewTenantID("SYN-T1")
	if err != nil {
		t.Fatalf("构造租户标识：%v", err)
	}
	key := verticalLedgerKey(t)
	balance, opened, err := view.LoadBalance(ctx, tenant, key)
	if err != nil || !opened {
		t.Fatalf("读回账面：opened=%v err=%v", opened, err)
	}
	// 入账 12000；渠道在途剩 2000，待清分 10000-8000-2000=0，应付客户 8000-5000=3000，
	// 已汇付 5000，短款 2000。
	wants := map[domain.FundPosition]int64{
		domain.PositionInTransitAtChannel: 2000,
		domain.PositionAwaitingAllocation: 0,
		domain.PositionPayableToCustomer:  3000,
		domain.PositionRemitted:           5000,
		domain.PositionShortfall:          2000,
		domain.PositionSurplus:            0,
	}
	var held int64
	for position, want := range wants {
		got := balance.At(position)
		if got != want {
			t.Fatalf("%s 余额 = %d，要 %d", position, got, want)
		}
		held += got
	}
	if balance.IntakeTotal() != 12000 || held != balance.IntakeTotal() {
		t.Fatalf("分配守恒破了：账内合计 %d，入账 %d", held, balance.IntakeTotal())
	}
	if balance.PostingCount() != 5 {
		t.Fatalf("记账笔数 = %d，要 5——落账的每一笔都还在账上", balance.PostingCount())
	}
}

func verticalLedgerKey(t *testing.T) domain.SubledgerKey {
	t.Helper()
	customer, err := domain.NewCustomerReference("SYN-CUST-1")
	if err != nil {
		t.Fatalf("构造客户引用：%v", err)
	}
	entity, err := domain.NewLegalEntityReference("SYN-ENTITY-1")
	if err != nil {
		t.Fatalf("构造法人引用：%v", err)
	}
	currency, err := domain.NewCurrencyCode("EUR")
	if err != nil {
		t.Fatalf("构造币种：%v", err)
	}
	channel, err := domain.NewCollectionChannelReference("SYN-CHANNEL-1")
	if err != nil {
		t.Fatalf("构造渠道引用：%v", err)
	}
	key, err := domain.NewSubledgerKey(customer, entity, currency, channel)
	if err != nil {
		t.Fatalf("构造分户账键：%v", err)
	}
	return key
}
