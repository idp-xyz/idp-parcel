package parcelshipment_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/parcelshipment"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/veconsume"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证接受决定 → 客户归属确立补派生（UC-VE-008 AT-VE-169）的消费侧形状：
// 拒绝态是业务终局不动读口；接受态按（租户+委托）取声明清单、逐成员读当前投影，
// 有投影才重走派生；账户维走 PS 反查口（权威），信封账户只作一致性校验，不匹配
// 一律硬失败；成员未决整封回滚（ADR-0066 先例）。

// declaredParcelsDouble 答（租户+委托）→ 成员清单三格。
type declaredParcelsDouble struct {
	members []psdomain.DeclaredParcelID
	found   bool
	err     error
	calls   int
}

func (double *declaredParcelsDouble) FindCurrentDeclaredParcels(
	_ context.Context, _ psdomain.TenantID, _ psdomain.ShipmentRequestID,
) ([]psdomain.DeclaredParcelID, bool, error) {
	double.calls++
	if double.err != nil {
		return nil, false, double.err
	}
	return double.members, double.found, nil
}

// projectionByParcelDouble 按包裹答当前投影——多成员场景要求逐成员可区分。
type projectionByParcelDouble struct {
	byParcel map[string]vedomain.TrackingProjection
	err      error
}

func (double *projectionByParcelDouble) FindCurrent(
	_ context.Context, _ vedomain.TenantID, parcel vedomain.TrackedParcelReference,
) (vedomain.TrackingProjection, bool, error) {
	if double.err != nil {
		return vedomain.TrackingProjection{}, false, double.err
	}
	projection, found := double.byParcel[parcel.String()]
	return projection, found, nil
}

// accountByParcelDouble 按包裹答账户三格。
type accountByParcelDouble struct {
	byParcel map[string]vedomain.CustomerAccountReference
	err      error
	calls    int
}

func (double *accountByParcelDouble) FindCustomerAccount(
	_ context.Context, _ vedomain.TenantID, parcel vedomain.TrackedParcelReference,
) (vedomain.CustomerAccountReference, bool, error) {
	double.calls++
	if double.err != nil {
		return vedomain.CustomerAccountReference{}, false, double.err
	}
	account, found := double.byParcel[parcel.String()]
	return account, found, nil
}

func declaredMembers(t *testing.T, raws ...string) []psdomain.DeclaredParcelID {
	t.Helper()
	members := make([]psdomain.DeclaredParcelID, 0, len(raws))
	for _, raw := range raws {
		members = append(members, finalValue(t, psdomain.NewDeclaredParcelID, raw))
	}
	return members
}

func acceptedDecision(state string) veinbox.FormedAcceptanceDecision {
	return veinbox.FormedAcceptanceDecision{
		TenantID:          "tenant-1",
		CustomerAccountID: "customer-1",
		ShipmentRequestID: "request-1",
		State:             state,
	}
}

func acceptanceSubject(
	t *testing.T,
	parcels *declaredParcelsDouble,
	projections *projectionByParcelDouble,
	accounts *accountByParcelDouble,
	derive adapter.CustomerViewDeriveHandler,
) *adapter.DeriveCustomerViewOnAcceptanceAdapter {
	t.Helper()
	subject, err := adapter.NewDeriveCustomerViewOnAcceptanceAdapter(parcels, projections, accounts, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	return subject
}

func accountsAnswering(t *testing.T, byParcel map[string]string) *accountByParcelDouble {
	t.Helper()
	answers := make(map[string]vedomain.CustomerAccountReference, len(byParcel))
	for parcel, account := range byParcel {
		answers[parcel] = finalValue(t, vedomain.NewCustomerAccountReference, account)
	}
	return &accountByParcelDouble{byParcel: answers}
}

// Covers: 信封覆盖接受与拒绝两种走向，VE 侧只应对接受态动作——拒绝是终局答案，
// 入账收工，不读任何读口。
func TestARejectedDecisionIsBusinessFinalWithoutAnyReads(t *testing.T) {
	parcels := &declaredParcelsDouble{}
	accounts := &accountByParcelDouble{}
	derive := &viewDeriveCounting{}
	subject := acceptanceSubject(t, parcels, &projectionByParcelDouble{}, accounts, derive)

	if err := subject.HandleFormedAcceptanceDecision(t.Context(), acceptedDecision("REJECTED")); err != nil {
		t.Fatalf("拒绝态应入账收工：%v", err)
	}
	if parcels.calls != 0 || accounts.calls != 0 || derive.calls != 0 {
		t.Fatalf("拒绝态不该动任何读口：清单 = %d，账户 = %d，派生 = %d",
			parcels.calls, accounts.calls, derive.calls)
	}
}

// Covers: 封闭集合外的状态字响亮报错——静默入账会把一份说不清自己是什么的信封吞掉。
func TestADecisionStateOutsideTheClosedSetIsLoud(t *testing.T) {
	for _, state := range []string{"SUBMITTED", "WITHDRAWN", "accepted"} {
		t.Run(state, func(t *testing.T) {
			derive := &viewDeriveCounting{}
			subject := acceptanceSubject(t,
				&declaredParcelsDouble{found: true}, &projectionByParcelDouble{},
				&accountByParcelDouble{}, derive)

			err := subject.HandleFormedAcceptanceDecision(t.Context(), acceptedDecision(state))
			if !errors.Is(err, adapter.ErrAcceptanceDecisionUntranslatable) {
				t.Fatalf("err = %v, want ErrAcceptanceDecisionUntranslatable", err)
			}
			if derive.calls != 0 {
				t.Fatal("集合外状态不该走到派生")
			}
		})
	}
}

func TestAnUntranslatableAcceptanceReferenceKeepsItsSentinel(t *testing.T) {
	for name, formed := range map[string]veinbox.FormedAcceptanceDecision{
		"空租户": {CustomerAccountID: "customer-1", ShipmentRequestID: "request-1", State: "ACCEPTED"},
		"空账户": {TenantID: "tenant-1", ShipmentRequestID: "request-1", State: "ACCEPTED"},
		"空委托": {TenantID: "tenant-1", CustomerAccountID: "customer-1", State: "ACCEPTED"},
	} {
		t.Run(name, func(t *testing.T) {
			derive := &viewDeriveCounting{}
			subject := acceptanceSubject(t,
				&declaredParcelsDouble{found: true}, &projectionByParcelDouble{},
				&accountByParcelDouble{}, derive)

			err := subject.HandleFormedAcceptanceDecision(t.Context(), formed)
			if !errors.Is(err, adapter.ErrAcceptanceDecisionUntranslatable) {
				t.Fatalf("err = %v, want ErrAcceptanceDecisionUntranslatable", err)
			}
			if derive.calls != 0 {
				t.Fatal("引用译不出来不该走到派生")
			}
		})
	}
}

func TestAnUnavailableDeclaredParcelsViewIsContinuableUndecided(t *testing.T) {
	derive := &viewDeriveCounting{}
	subject := acceptanceSubject(t,
		&declaredParcelsDouble{err: errors.New("ps store unavailable")},
		&projectionByParcelDouble{}, &accountByParcelDouble{}, derive)

	err := subject.HandleFormedAcceptanceDecision(t.Context(), acceptedDecision("ACCEPTED"))
	if !errors.Is(err, adapter.ErrDeclaredParcelsUnavailable) {
		t.Fatalf("err = %v, want ErrDeclaredParcelsUnavailable", err)
	}
	if derive.calls != 0 {
		t.Fatal("清单读口调不通不该走到派生")
	}
}

// Covers: 接受信封与委托行同一事务入库——信封在而行不在是仓储不变量已破，响亮报错
// 而不是当可见性滞后等下去。
func TestAMissingDelegationRowIsInconsistentNotUndecided(t *testing.T) {
	derive := &viewDeriveCounting{}
	subject := acceptanceSubject(t,
		&declaredParcelsDouble{}, &projectionByParcelDouble{}, &accountByParcelDouble{}, derive)

	err := subject.HandleFormedAcceptanceDecision(t.Context(), acceptedDecision("ACCEPTED"))
	if !errors.Is(err, adapter.ErrAcceptanceRecordInconsistent) {
		t.Fatalf("err = %v, want ErrAcceptanceRecordInconsistent", err)
	}
	if derive.calls != 0 {
		t.Fatal("委托行不在不该走到派生")
	}
}

// Covers: 常态空转——正常序（先接受委托、后包裹流转）里按包裹读不到当前投影，
// 什么也不做：不反查账户、不派生，整封入账收工。
func TestParcelsWithoutProjectionsAreSkippedQuietly(t *testing.T) {
	accounts := &accountByParcelDouble{}
	derive := &viewDeriveCounting{}
	subject := acceptanceSubject(t,
		&declaredParcelsDouble{members: declaredMembers(t, "parcel-1", "parcel-2"), found: true},
		&projectionByParcelDouble{byParcel: map[string]vedomain.TrackingProjection{}},
		accounts, derive)

	if err := subject.HandleFormedAcceptanceDecision(t.Context(), acceptedDecision("ACCEPTED")); err != nil {
		t.Fatalf("无投影应整封入账收工：%v", err)
	}
	if accounts.calls != 0 {
		t.Fatal("无投影不该反查账户")
	}
	if derive.calls != 0 {
		t.Fatal("无投影不该走到派生")
	}
}

func TestAnUnreadableProjectionStoreIsContinuableUndecidedOnAcceptance(t *testing.T) {
	derive := &viewDeriveCounting{}
	subject := acceptanceSubject(t,
		&declaredParcelsDouble{members: declaredMembers(t, "parcel-1"), found: true},
		&projectionByParcelDouble{err: errors.New("projection store unavailable")},
		&accountByParcelDouble{}, derive)

	err := subject.HandleFormedAcceptanceDecision(t.Context(), acceptedDecision("ACCEPTED"))
	if !errors.Is(err, adapter.ErrDerivedProjectionUnreadable) {
		t.Fatalf("err = %v, want ErrDerivedProjectionUnreadable", err)
	}
	if derive.calls != 0 {
		t.Fatal("投影库调不通不该走到派生")
	}
}

// Covers: 恰一行且信封一致——账户取自反查口（权威），与当前投影一起原样进派生命令，
// 视图发布成功入账。
func TestAProjectedParcelRederivesTheCustomerView(t *testing.T) {
	projection := builtProjection(t, "projection-1", "parcel-1")
	views := &customerViewStoreDouble{byKey: map[string]vedomain.CustomerTrackingView{}}
	handler := realViewDeriveHandler(views, viewPolicyDouble{configured: false}, viewDownstreamDouble{})
	subject := acceptanceSubject(t,
		&declaredParcelsDouble{members: declaredMembers(t, "parcel-1"), found: true},
		&projectionByParcelDouble{byParcel: map[string]vedomain.TrackingProjection{"parcel-1": projection}},
		accountsAnswering(t, map[string]string{"parcel-1": "customer-1"}),
		handler)

	if err := subject.HandleFormedAcceptanceDecision(t.Context(), acceptedDecision("ACCEPTED")); err != nil {
		t.Fatalf("补派生应入账：%v", err)
	}
	view, found := views.byKey["tenant-1/customer-1/parcel-1"]
	if !found {
		t.Fatal("客户视图没落库——归属确立没触发视图形成（AT-VE-169）")
	}
	if view.BasedOn().String() != "projection-1" {
		t.Fatalf("视图采用版本 = %s, want projection-1（按当前投影形成）", view.BasedOn())
	}
}

// Covers: 成员循环只对有投影的成员动作——一份委托里流转过的包裹补派生，没流转的
// 跳过，互不牵连。
func TestOnlyProjectedMembersRederive(t *testing.T) {
	projection := builtProjection(t, "projection-2", "parcel-2")
	accounts := accountsAnswering(t, map[string]string{"parcel-2": "customer-1"})
	derive := &viewDeriveCounting{}
	subject := acceptanceSubject(t,
		&declaredParcelsDouble{members: declaredMembers(t, "parcel-1", "parcel-2"), found: true},
		&projectionByParcelDouble{byParcel: map[string]vedomain.TrackingProjection{"parcel-2": projection}},
		accounts, derive)

	err := subject.HandleFormedAcceptanceDecision(t.Context(), acceptedDecision("ACCEPTED"))
	if !errors.Is(err, veconsume.ErrUnexpectedCustomerViewOutcome) {
		// viewDeriveCounting 交回零值结果（封闭集合外），到这里说明派生确实只被调了那一次。
		t.Fatalf("err = %v, want ErrUnexpectedCustomerViewOutcome", err)
	}
	if accounts.calls != 1 {
		t.Fatalf("账户反查次数 = %d, want 1——无投影的成员不该反查", accounts.calls)
	}
	if derive.calls != 1 {
		t.Fatalf("派生调用次数 = %d, want 1", derive.calls)
	}
	if derive.last.Projection.Version().String() != "projection-2" ||
		derive.last.Customer.String() != "customer-1" || derive.last.TenantID.String() != "tenant-1" {
		t.Fatalf("命令走样：%+v", derive.last)
	}
}

// Covers: 账户反查多行——机制拒绝自动采认（AT-VE-152、ADR-0060），具名哨兵原样上抛
// 落未决，这封信如实卡着，不任选也不折成「无视图」。
func TestAnAmbiguousAccountStaysUndecidedOnAcceptance(t *testing.T) {
	derive := &viewDeriveCounting{}
	subject := acceptanceSubject(t,
		&declaredParcelsDouble{members: declaredMembers(t, "parcel-1"), found: true},
		&projectionByParcelDouble{byParcel: map[string]vedomain.TrackingProjection{
			"parcel-1": builtProjection(t, "projection-1", "parcel-1"),
		}},
		&accountByParcelDouble{err: fmt.Errorf("%w: parcel %q",
			adapter.ErrAmbiguousCustomerAccount, "parcel-1")},
		derive)

	err := subject.HandleFormedAcceptanceDecision(t.Context(), acceptedDecision("ACCEPTED"))
	if !errors.Is(err, adapter.ErrAmbiguousCustomerAccount) {
		t.Fatalf("err = %v, want ErrAmbiguousCustomerAccount", err)
	}
	if derive.calls != 0 {
		t.Fatal("歧义不该走到派生——不得任选账户")
	}
}

func TestAnUnavailableAccountLookupIsContinuableUndecidedOnAcceptance(t *testing.T) {
	derive := &viewDeriveCounting{}
	subject := acceptanceSubject(t,
		&declaredParcelsDouble{members: declaredMembers(t, "parcel-1"), found: true},
		&projectionByParcelDouble{byParcel: map[string]vedomain.TrackingProjection{
			"parcel-1": builtProjection(t, "projection-1", "parcel-1"),
		}},
		&accountByParcelDouble{err: errors.New("ps store unavailable")},
		derive)

	err := subject.HandleFormedAcceptanceDecision(t.Context(), acceptedDecision("ACCEPTED"))
	if !errors.Is(err, adapter.ErrCustomerAccountUnavailable) {
		t.Fatalf("err = %v, want ErrCustomerAccountUnavailable", err)
	}
	if derive.calls != 0 {
		t.Fatal("反查读口调不通不该走到派生")
	}
}

// Covers: 反查零行在本路是仓储不变量已破，不是「还没有委托」——成员刚从这份已接受
// 委托自己的清单里读出来，同一投影列的反方向读却说没有，两次读自相矛盾。
func TestAProjectedParcelWithoutAnAcceptedTargetIsInconsistent(t *testing.T) {
	derive := &viewDeriveCounting{}
	subject := acceptanceSubject(t,
		&declaredParcelsDouble{members: declaredMembers(t, "parcel-1"), found: true},
		&projectionByParcelDouble{byParcel: map[string]vedomain.TrackingProjection{
			"parcel-1": builtProjection(t, "projection-1", "parcel-1"),
		}},
		&accountByParcelDouble{byParcel: map[string]vedomain.CustomerAccountReference{}},
		derive)

	err := subject.HandleFormedAcceptanceDecision(t.Context(), acceptedDecision("ACCEPTED"))
	if !errors.Is(err, adapter.ErrAcceptanceRecordInconsistent) {
		t.Fatalf("err = %v, want ErrAcceptanceRecordInconsistent", err)
	}
	if derive.calls != 0 {
		t.Fatal("零行不该走到派生")
	}
}

// Covers: 信封 CustomerAccountID 只作一致性校验——反查口是账户维的权威，不匹配一律
// 硬失败（基线上结构不可达，闸门仍须在：绕过领域直写库的迁移或人工 SQL 同样归这里）。
func TestAnAccountMismatchIsAHardFailure(t *testing.T) {
	derive := &viewDeriveCounting{}
	subject := acceptanceSubject(t,
		&declaredParcelsDouble{members: declaredMembers(t, "parcel-1"), found: true},
		&projectionByParcelDouble{byParcel: map[string]vedomain.TrackingProjection{
			"parcel-1": builtProjection(t, "projection-1", "parcel-1"),
		}},
		accountsAnswering(t, map[string]string{"parcel-1": "customer-2"}),
		derive)

	err := subject.HandleFormedAcceptanceDecision(t.Context(), acceptedDecision("ACCEPTED"))
	if !errors.Is(err, adapter.ErrCustomerAccountMismatch) {
		t.Fatalf("err = %v, want ErrCustomerAccountMismatch", err)
	}
	if derive.calls != 0 {
		t.Fatal("账户不匹配不该走到派生——不得采认任何一边")
	}
}

// Covers: 一封信一笔事务（ADR-0066 先例）——任一成员停在未决，整封报错回滚，重投
// 从头再跑；已派生成员靠派生编排的幂等（同投影版本答已有结果）扛重跑。
func TestAnUndecidedMemberRollsBackTheWholeEnvelope(t *testing.T) {
	views := &customerViewStoreDouble{
		byKey:   map[string]vedomain.CustomerTrackingView{},
		findErr: errors.New("view store down"),
	}
	handler := realViewDeriveHandler(views, viewPolicyDouble{configured: false}, viewDownstreamDouble{})
	subject := acceptanceSubject(t,
		&declaredParcelsDouble{members: declaredMembers(t, "parcel-1", "parcel-2"), found: true},
		&projectionByParcelDouble{byParcel: map[string]vedomain.TrackingProjection{
			"parcel-1": builtProjection(t, "projection-1", "parcel-1"),
			"parcel-2": builtProjection(t, "projection-2", "parcel-2"),
		}},
		accountsAnswering(t, map[string]string{"parcel-1": "customer-1", "parcel-2": "customer-1"}),
		handler)

	err := subject.HandleFormedAcceptanceDecision(t.Context(), acceptedDecision("ACCEPTED"))
	if !errors.Is(err, veconsume.ErrCustomerViewUndecided) {
		t.Fatalf("err = %v, want ErrCustomerViewUndecided", err)
	}
}

// Covers: 视图已发布但意图没交出去——拦在入账前，否则视图在、下游 outbox 永久缺。
func TestAPendingViewHandoffBlocksAcceptanceConsumption(t *testing.T) {
	views := &customerViewStoreDouble{byKey: map[string]vedomain.CustomerTrackingView{}}
	handler := realViewDeriveHandler(views, viewPolicyDouble{configured: false},
		viewDownstreamDouble{err: errors.New("outbox down")})
	subject := acceptanceSubject(t,
		&declaredParcelsDouble{members: declaredMembers(t, "parcel-1"), found: true},
		&projectionByParcelDouble{byParcel: map[string]vedomain.TrackingProjection{
			"parcel-1": builtProjection(t, "projection-1", "parcel-1"),
		}},
		accountsAnswering(t, map[string]string{"parcel-1": "customer-1"}),
		handler)

	err := subject.HandleFormedAcceptanceDecision(t.Context(), acceptedDecision("ACCEPTED"))
	if !errors.Is(err, veconsume.ErrCustomerViewHandoffPending) {
		t.Fatalf("err = %v, want ErrCustomerViewHandoffPending", err)
	}
}
