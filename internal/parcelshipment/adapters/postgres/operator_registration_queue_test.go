package postgres_test

import (
	"context"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件对真实 PostgreSQL 证「等待运营登记」队列读口（ADR-0094 Decision 五，票
// first-tenant-runway/07）：等待态经 AwaitOperatorRegistration + Save 落库后按租户列得出来，
// 带的正是重驱接受判断链要的那几样；不在等的、不在`已提交`的、别的租户的都不上列。
//
// 它同时替 TestTaskWaitingOnProjectionMirrorsEveryResumePath 补上那条注释预告的半句：第四格
// 的等待态今天有一条真路径走 Save 写下 4，不再只靠裸写证库面镜像。

// Covers: ADR-0094 Decision 五 — 停在`等待运营登记`的`已提交`委托按租户列得出来，老的在前；
// 一行带来源身份、委托标识、当前提交版本与成员清单，都逐字段过领域构造门。
func TestOperatorRegistrationQueueListsParkedSubmittedRequestsOldestFirst(t *testing.T) {
	repository, transactor, pool := newShipmentRequests(t)
	ctx := t.Context()
	tenant := psTenant(t, "tenant-1")

	// 三份委托：一份不在等；两份停在`等待运营登记`，其中后建的那份提交时间更早，用来证排序
	// 按 submitted_at 而不是按写入顺序。
	idle := submittedShipmentRequest(t, "orq-idle", "REQ-ORQ-IDLE")
	mustInsert(t, transactor, ctx, repository, idle)

	parkOnOperatorRegistration(t, transactor, ctx, repository, "orq-later", "REQ-ORQ-LATER")
	earlier := parkOnOperatorRegistration(t, transactor, ctx, repository, "orq-earlier", "REQ-ORQ-EARLIER")
	if _, err := pool.Exec(ctx,
		`UPDATE parcel_shipment.shipment_request SET submitted_at = $1
		  WHERE shipment_request_id = 'REQ-ORQ-EARLIER'`,
		submittedAtFixture.Add(-time.Hour)); err != nil {
		t.Fatalf("回拨提交时间：%v", err)
	}

	records, err := repository.ListWaitingOnOperatorRegistration(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2（不在等的那份不该上列）", len(records))
	}
	if got := records[0].ShipmentRequestID.String(); got != "REQ-ORQ-EARLIER" {
		t.Fatalf("first = %s, want REQ-ORQ-EARLIER——队列要老的在前", got)
	}
	if got := records[1].ShipmentRequestID.String(); got != "REQ-ORQ-LATER" {
		t.Fatalf("second = %s, want REQ-ORQ-LATER", got)
	}

	first := records[0]
	wantIdentity := earlier.CurrentSubmissionVersion().SourceSubmission().Identity()
	if first.Identity != wantIdentity {
		t.Fatalf("identity = %#v, want %#v——重驱要的是完整来源身份", first.Identity, wantIdentity)
	}
	if first.SubmissionVersion != earlier.CurrentSubmissionVersion().VersionID() {
		t.Fatalf("submission version = %s, want %s", first.SubmissionVersion, earlier.CurrentSubmissionVersion().VersionID())
	}
	if len(first.DeclaredParcelIDs) != 2 ||
		first.DeclaredParcelIDs[0].String() != "parcel-1" ||
		first.DeclaredParcelIDs[1].String() != "parcel-2" {
		t.Fatalf("declared parcels = %v, want [parcel-1 parcel-2]", first.DeclaredParcelIDs)
	}
	if !first.SubmittedAt.Equal(submittedAtFixture.Add(-time.Hour)) {
		t.Fatalf("submitted at = %v, want %v", first.SubmittedAt, submittedAtFixture.Add(-time.Hour))
	}

	// 分页可重复：limit 1 取到的正是老的那一份。
	page, err := repository.ListWaitingOnOperatorRegistration(ctx, tenant, 1)
	if err != nil {
		t.Fatalf("list limit 1: %v", err)
	}
	if len(page) != 1 || page[0].ShipmentRequestID.String() != "REQ-ORQ-EARLIER" {
		t.Fatalf("page = %v, want just REQ-ORQ-EARLIER", page)
	}
}

// Covers: 队列谓词的两条边 — 以租户为键，别的租户什么都看不到；等待态只在`已提交`委托上有队列
// 语义，一份留着第四格快照却已不在`已提交`的行不上列（state 上榜的理由）。
func TestOperatorRegistrationQueueIsScopedByTenantAndSubmittedState(t *testing.T) {
	repository, transactor, pool := newShipmentRequests(t)
	ctx := t.Context()

	parkOnOperatorRegistration(t, transactor, ctx, repository, "orq-scope", "REQ-ORQ-SCOPE")

	others, err := repository.ListWaitingOnOperatorRegistration(ctx, psTenant(t, "tenant-2"), 10)
	if err != nil {
		t.Fatalf("list other tenant: %v", err)
	}
	if len(others) != 0 {
		t.Fatalf("另一个租户列到了 %d 行——队列越过了租户边界", len(others))
	}

	// 裸写把状态挪出`已提交`而保留投影列上的第四格：仓储的终态转移各自清零等待态，正常路径
	// 造不出这一行，所以只能裸写；它证的是谓词，不是领域。
	if _, err := pool.Exec(ctx,
		`UPDATE parcel_shipment.shipment_request SET state = $1
		  WHERE shipment_request_id = 'REQ-ORQ-SCOPE'`,
		uint8(domain.ShipmentRequestWithdrawn)); err != nil {
		t.Fatalf("裸写状态：%v", err)
	}
	stale, err := repository.ListWaitingOnOperatorRegistration(ctx, psTenant(t, "tenant-1"), 10)
	if err != nil {
		t.Fatalf("list after state change: %v", err)
	}
	if len(stale) != 0 {
		t.Fatalf("一份不在`已提交`的委托仍在队列上（%d 行）", len(stale))
	}
}

// Covers: limit 非正是调用方编程错误，判据同 ListVisible——不静默取空页。
func TestOperatorRegistrationQueueRejectsANonPositiveLimit(t *testing.T) {
	repository, _, _ := newShipmentRequests(t)
	if _, err := repository.ListWaitingOnOperatorRegistration(t.Context(), psTenant(t, "tenant-1"), 0); err == nil {
		t.Fatal("limit 0 被静默接受了")
	}
}

// parkOnOperatorRegistration 建一份`已提交`委托并经领域转移 + Save 把它停在`等待运营登记`上，
// 交回停等后的那一份。走真路径而不裸写投影列：本文件要证的正是这条路径今天存在。
func parkOnOperatorRegistration(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	repository *adapter.ShipmentRequests,
	key, requestID string,
) domain.ShipmentRequest {
	t.Helper()
	request := submittedShipmentRequest(t, key, requestID)
	identity := request.CurrentSubmissionVersion().SourceSubmission().Identity()
	mustInsert(t, transactor, ctx, repository, request)
	// 与编排同一条路：先按来源身份读回（拿到落库后的版本号），再转移、再 Save——直接拿建单前
	// 的那份去 Save 会撞`版本冲突`，那是乐观版本在守，不是这里要证的东西。
	loaded, found, err := repository.FindBySourceIdentity(ctx, identity)
	if err != nil || !found {
		t.Fatalf("read back %s: found=%v err=%v", requestID, found, err)
	}
	parked, err := loaded.AwaitOperatorRegistration()
	if err != nil {
		t.Fatalf("await operator registration: %v", err)
	}
	mustSave(t, transactor, ctx, repository, identity, parked)
	return parked
}
