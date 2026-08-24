package main

import (
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: `/shipment-requests/parcel-cancellations` 的第二参是真编排——委托仓储、
// 收寄采认读口、取消决定库、PCXL 标识签发与 Outbox 意图交付在真实 PostgreSQL 上装得
// 起来。三个钉：指名不存在的委托如实答未受理（真仓储查过，查无与编号不符同答，
// AT-PS-090 的隔离面——不是未决，不造委托）；授权目录未配置（PAR-COM-17 实例半边，
// 采用版本缺席）时停在 AUTHORITY_UNCONFIGURED 而非 AUTHORITY_UNAVAILABLE——恢复动作
// 是登记规则不是修服务，且绝不默认任何角色可/不可取消；未决什么也不落库——重放同一
// 份输入仍走整套判断，不走已有结果。测试输入是隔离合成，只记 `S`，不进生产装配。
func TestTheWiredCancellationAnswersHonestlyAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	cancellation, err := buildCancellationOrchestration(db)
	if err != nil {
		t.Fatalf("装配取消编排：%v", err)
	}

	identity := cancellationSeedIdentity(t, "SYN-SUBMIT-KEY-1")
	absent, err := cancellation.Handle(t.Context(), cancelParcelCommand(t, cancellationSeedIdentity(t, "SYN-SUBMIT-KEY-ABSENT")))
	if err != nil {
		t.Fatalf("指名不存在的委托：%v", err)
	}
	if got := absent.Outcome(); got != shipmentapp.CancellationNotAccepted {
		t.Fatalf("outcome = %v, want REQUEST_NOT_ACCEPTED——委托查无是提交矛盾，不是未决", got)
	}

	seedSubmittedShipmentRequest(t, db, identity)

	command := cancelParcelCommand(t, identity)
	undecided, err := cancellation.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("授权目录未配置不该以 error 交回（那是 5xx，不是未决）：%v", err)
	}
	if got := undecided.Outcome(); got != shipmentapp.CancellationUndecided {
		t.Fatalf("outcome = %v, want CANCELLATION_UNDECIDED——目录未配置既不是拒绝也不是放行", got)
	}
	if got := undecided.UndecidedReason(); got != shipmentapp.CancelAuthorityUnconfigured {
		t.Fatalf("reason = %v, want AUTHORITY_UNCONFIGURED——答成 UNAVAILABLE 会把「等租户登记」指成「修授权服务」", got)
	}
	if undecided.ContinuationReference().String() == "" {
		t.Fatal("未决没带续办引用，调用方无从查询原次尝试")
	}

	// 未决不落库：重放同一份输入仍走整套判断停在同一格，而不是读回一份「已有结果」
	// ——有就说明未决那轮把半份判断写进去了。
	replay, err := cancellation.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("重放同一份输入：%v", err)
	}
	if got := replay.Outcome(); got != shipmentapp.CancellationUndecided {
		t.Fatalf("outcome = %v, want CANCELLATION_UNDECIDED——未决那轮不该留下任何记录", got)
	}
}

func cancellationSeedIdentity(t *testing.T, requestKey string) domain.SourceIdentity {
	t.Helper()
	identity, err := domain.NewSourceIdentity(
		mustValue(t, domain.NewTenantID, "SYN-TENANT-1"),
		mustValue(t, domain.NewCustomerAccountID, "SYN-CUSTOMER-1"),
		mustValue(t, domain.NewSource, "SYN-SOURCE-A"),
		mustValue(t, domain.NewSourceRequestKey, requestKey),
	)
	if err != nil {
		t.Fatalf("new source identity: %v", err)
	}
	return identity
}

func cancelParcelCommand(t *testing.T, identity domain.SourceIdentity) shipmentapp.CancelParcelCommand {
	t.Helper()
	return shipmentapp.CancelParcelCommand{
		Identity:          identity,
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "SYN-REQUEST-1"),
		Parcel:            mustValue(t, domain.NewDeclaredParcelID, "SYN-PARCEL-1"),
		Requester:         mustValue(t, domain.NewCancellationRequesterReference, "SYN-CUSTOMER-1"),
		Reason:            mustValue(t, domain.NewCancellationReasonReference, "SYN-CUSTOMER-CHANGED-MIND"),
		RequestedAt:       time.Date(2026, 8, 24, 11, 0, 0, 0, time.UTC),
	}
}
