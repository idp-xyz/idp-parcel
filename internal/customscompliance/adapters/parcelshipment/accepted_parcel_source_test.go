package parcelshipment_test

import (
	"context"
	"errors"
	"testing"

	ccparcelshipment "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/parcelshipment"
	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

type acceptedTargetsDouble struct {
	targets map[string]psdomain.CurrentAcceptedParcelTarget
	err     error
}

func (double acceptedTargetsDouble) FindCurrentAcceptedByParcel(
	_ context.Context,
	_ psdomain.TenantID,
	parcel psdomain.DeclaredParcelID,
) (psdomain.CurrentAcceptedParcelTarget, bool, error) {
	if double.err != nil {
		return psdomain.CurrentAcceptedParcelTarget{}, false, double.err
	}
	target, found := double.targets[parcel.String()]
	return target, found, nil
}

func must[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return value
}

// Covers: AT-CC-437（翻译半边）——包裹的客户归属取提供方那份已接受委托的客户账户，来源资料引用取（委托，当前提交版本）；
// 查不到答不在任何已接受委托里；提供方两份都声明了这件包裹是它的坏状态，作 error 交回、不挑一份。
func TestTheAcceptedRequestGivesCustomerAndSourceAndAmbiguityIsNotResolvedHere(t *testing.T) {
	identity, err := psdomain.NewSourceIdentity(
		must(t, psdomain.NewTenantID, "SYN-TENANT-01"),
		must(t, psdomain.NewCustomerAccountID, "SYN-ACCOUNT-01"),
		must(t, psdomain.NewSource, "SYN-SOURCE"),
		must(t, psdomain.NewSourceRequestKey, "SYN-KEY-1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	target, err := psdomain.NewCurrentAcceptedParcelTarget(identity,
		must(t, psdomain.NewShipmentRequestID, "SHR-1"), must(t, psdomain.NewSubmissionVersionID, "SUBV-2"))
	if err != nil {
		t.Fatal(err)
	}
	sources, err := ccparcelshipment.NewAcceptedParcelSources(acceptedTargetsDouble{targets: map[string]psdomain.CurrentAcceptedParcelTarget{"DPCL-1": target}})
	if err != nil {
		t.Fatal(err)
	}
	tenant := must(t, ccdomain.NewTenantID, "SYN-TENANT-01")

	source, found, err := sources.FindAcceptedParcelSource(context.Background(), tenant, "DPCL-1")
	if err != nil || !found || source.Customer != "SYN-ACCOUNT-01" || source.SourceRef != "SHR-1/SUBV-2" {
		t.Fatalf("source = %+v found = %v err = %v", source, found, err)
	}
	if _, found, err := sources.FindAcceptedParcelSource(context.Background(), tenant, "DPCL-9"); err != nil || found {
		t.Fatalf("未接受包裹：found = %v err = %v", found, err)
	}

	ambiguous, err := ccparcelshipment.NewAcceptedParcelSources(acceptedTargetsDouble{err: psdomain.ErrAmbiguousParcelTarget})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ambiguous.FindAcceptedParcelSource(context.Background(), tenant, "DPCL-1"); !errors.Is(err, psdomain.ErrAmbiguousParcelTarget) {
		t.Fatalf("歧义：err = %v", err)
	}
}
