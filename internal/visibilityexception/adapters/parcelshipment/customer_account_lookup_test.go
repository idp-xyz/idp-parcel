package parcelshipment_test

import (
	"context"
	"errors"
	"testing"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/parcelshipment"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证账户反查适配器的三格译法与哨兵映射（ADR-0060 的零/一/多）：零行如实
// found=false 不分成因、恰一行取来源身份上的账户、多行译成 VE 具名哨兵且 PS 哨兵
// 不外泄。归属判断本身在 PS 的读口测试里，这里只证翻译。

func lookupValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

type targetFinderDouble struct {
	target     psdomain.CurrentAcceptedParcelTarget
	found      bool
	err        error
	calls      int
	lastTenant psdomain.TenantID
	lastParcel psdomain.DeclaredParcelID
}

func (double *targetFinderDouble) FindCurrentAcceptedByParcel(
	_ context.Context, tenant psdomain.TenantID, parcel psdomain.DeclaredParcelID,
) (psdomain.CurrentAcceptedParcelTarget, bool, error) {
	double.calls++
	double.lastTenant = tenant
	double.lastParcel = parcel
	if double.err != nil {
		return psdomain.CurrentAcceptedParcelTarget{}, false, double.err
	}
	return double.target, double.found, nil
}

func acceptedTarget(t *testing.T) psdomain.CurrentAcceptedParcelTarget {
	t.Helper()
	identity, err := psdomain.NewSourceIdentity(
		lookupValue(t, psdomain.NewTenantID, "tenant-1"),
		lookupValue(t, psdomain.NewCustomerAccountID, "account-9"),
		lookupValue(t, psdomain.NewSource, "API"),
		lookupValue(t, psdomain.NewSourceRequestKey, "request-1"),
	)
	if err != nil {
		t.Fatalf("构造来源身份：%v", err)
	}
	target, err := psdomain.NewCurrentAcceptedParcelTarget(
		identity,
		lookupValue(t, psdomain.NewShipmentRequestID, "shipment-1"),
		lookupValue(t, psdomain.NewSubmissionVersionID, "submission/v1"),
	)
	if err != nil {
		t.Fatalf("构造反查目标：%v", err)
	}
	return target
}

func newLookupSubject(t *testing.T, finder *targetFinderDouble) *adapter.ParcelCustomerAccountLookup {
	t.Helper()
	subject, err := adapter.NewParcelCustomerAccountLookup(finder)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	return subject
}

func lookupTenant(t *testing.T) vedomain.TenantID {
	t.Helper()
	return lookupValue(t, vedomain.NewTenantID, "tenant-1")
}

func lookupParcel(t *testing.T) vedomain.TrackedParcelReference {
	t.Helper()
	return lookupValue(t, vedomain.NewTrackedParcelReference, "parcel-1")
}

// 零行是如实的空白：不派生、不发明账户、不报错。追踪包裹引用可能装着集运单元号，
// 零行不是缺陷——成因区分是接线票的事，这里只证「查过没有」与「没查成」分得开。
func TestAMissingParcelTargetIsAnHonestNotFound(t *testing.T) {
	subject := newLookupSubject(t, &targetFinderDouble{})

	account, found, err := subject.FindCustomerAccount(t.Context(), lookupTenant(t), lookupParcel(t))
	if err != nil {
		t.Fatalf("err = %v; 零行不是错误", err)
	}
	if found {
		t.Fatal("零行交回了 found=true")
	}
	if account != (vedomain.CustomerAccountReference{}) {
		t.Fatalf("零行发明了账户 %q", account.String())
	}
}

func TestASingleAcceptedTargetYieldsItsCustomerAccount(t *testing.T) {
	finder := &targetFinderDouble{target: acceptedTarget(t), found: true}
	subject := newLookupSubject(t, finder)

	account, found, err := subject.FindCustomerAccount(t.Context(), lookupTenant(t), lookupParcel(t))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !found {
		t.Fatal("恰一行却交回 found=false")
	}
	if account.String() != "account-9" {
		t.Fatalf("账户 = %q, want 来源身份上的 account-9", account.String())
	}
	if finder.lastTenant.String() != "tenant-1" || finder.lastParcel.String() != "parcel-1" {
		t.Fatalf("反查键 = (%q, %q), want 两维原样翻译",
			finder.lastTenant.String(), finder.lastParcel.String())
	}
}

// 多行是未决/待确认（UC-VE-008 AT-VE-152），不是「无视图」：调用方只见 VE 具名哨兵，
// PS 的 ErrAmbiguousParcelTarget 不外泄——放行它会让编排绕过本上下文的词汇直接判
// 邻居的错误。
func TestAnAmbiguousTargetBecomesTheVENamedSentinel(t *testing.T) {
	subject := newLookupSubject(t, &targetFinderDouble{err: psdomain.ErrAmbiguousParcelTarget})

	account, found, err := subject.FindCustomerAccount(t.Context(), lookupTenant(t), lookupParcel(t))
	if !errors.Is(err, adapter.ErrAmbiguousCustomerAccount) {
		t.Fatalf("err = %v, want ErrAmbiguousCustomerAccount", err)
	}
	if errors.Is(err, psdomain.ErrAmbiguousParcelTarget) {
		t.Fatal("PS 哨兵外泄了——跨上下文翻译白做")
	}
	if found || account != (vedomain.CustomerAccountReference{}) {
		t.Fatalf("多行任选了账户：found = %v account = %q", found, account.String())
	}
}

// 依赖调不通不属三格里的任何一格：译成 found=false 会把「没查成」说成「查过没有」。
// 底层错误原样上抛供调用方形成未决。
func TestADependencyFailureIsNotTranslatedIntoAnyCell(t *testing.T) {
	broken := errors.New("store unavailable")
	subject := newLookupSubject(t, &targetFinderDouble{err: broken})

	_, found, err := subject.FindCustomerAccount(t.Context(), lookupTenant(t), lookupParcel(t))
	if !errors.Is(err, broken) {
		t.Fatalf("err = %v, want 底层错误原样在链上", err)
	}
	if errors.Is(err, adapter.ErrAmbiguousCustomerAccount) ||
		errors.Is(err, adapter.ErrCustomerAccountUntranslatable) {
		t.Fatalf("依赖失败被译成了哨兵：%v", err)
	}
	if found {
		t.Fatal("依赖失败交回了 found=true")
	}
}

func TestAnUntranslatableQueryKeepsItsSentinel(t *testing.T) {
	finder := &targetFinderDouble{target: acceptedTarget(t), found: true}
	subject := newLookupSubject(t, finder)

	for name, run := range map[string]func() (vedomain.CustomerAccountReference, bool, error){
		"空租户": func() (vedomain.CustomerAccountReference, bool, error) {
			return subject.FindCustomerAccount(t.Context(), vedomain.TenantID{}, lookupParcel(t))
		},
		"空包裹": func() (vedomain.CustomerAccountReference, bool, error) {
			return subject.FindCustomerAccount(t.Context(), lookupTenant(t), vedomain.TrackedParcelReference{})
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := run()
			if !errors.Is(err, adapter.ErrCustomerAccountUntranslatable) {
				t.Fatalf("err = %v, want ErrCustomerAccountUntranslatable", err)
			}
		})
	}
	if finder.calls != 0 {
		t.Fatal("查询维译不出来不该走到反查")
	}
}

// 正常途径的答复经 NewCurrentAcceptedParcelTarget 验过非空；零值目标意味着实现方
// 绕过了构造器，译不出账户时保持哨兵而不是交回空账户。
func TestAnUntranslatableAnswerKeepsItsSentinel(t *testing.T) {
	subject := newLookupSubject(t,
		&targetFinderDouble{target: psdomain.CurrentAcceptedParcelTarget{}, found: true})

	_, found, err := subject.FindCustomerAccount(t.Context(), lookupTenant(t), lookupParcel(t))
	if !errors.Is(err, adapter.ErrCustomerAccountUntranslatable) {
		t.Fatalf("err = %v, want ErrCustomerAccountUntranslatable", err)
	}
	if found {
		t.Fatal("译不出账户却交回 found=true")
	}
}
