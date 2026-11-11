package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件钉补充摘要 PSU-1 收平台段（ADR-0376 决定二，票 declared-goods-lines/10）：没带平台段的补充逐字节同前，已存的补充摘要与
// 重放照旧成立；带了平台段就进摘要，同一补充请求标识改了平台段答来源冲突。PSU-1 与提交的 PSC-1 共用编码（ADR-0373 决定八），
// 平台段那两键本来就在编码里，补充口这一次只是把它交进去。

func psu1Spec(t *testing.T) domain.SubmissionPayloadSpec {
	t.Helper()
	reference, err := domain.NewShipmentRequestID("SYN-CUSTREF-1")
	if err != nil {
		t.Fatal(err)
	}
	parcel, err := domain.NewDeclaredParcelID("SYN-PCLREF-1")
	if err != nil {
		t.Fatal(err)
	}
	basis, err := domain.NewSubmissionVersionID("SYN-VERSION-A")
	if err != nil {
		t.Fatal(err)
	}
	service, err := domain.NewCanonicalContentEntry(domain.RequestedServiceProductEntryName, "SYN-PROD-CN-SG-EXPRESS")
	if err != nil {
		t.Fatal(err)
	}
	return domain.SubmissionPayloadSpec{
		RequestReference: reference, BasisVersion: basis,
		DeclaredParcelIDs: []domain.DeclaredParcelID{parcel},
		Service:           []domain.CanonicalContentEntry{service},
	}
}

// psu1Golden 取证于补充口收平台段之前的提交（22a710c40）：同一份规格在那里算出的就是这一串。
const psu1Golden = "PSU-1:6597db4a1ee310ca643b57f76861ca6cf143c60ec77ab9f17ce9ff4f5bdf74ca"

// Covers: 没带平台段的补充摘要与改动之前一字不差；带了平台段摘要就变，箱标号换一个号摘要也变。
func TestASupplementWithoutAPlatformSectionKeepsItsDigestAndOneWithItDiffers(t *testing.T) {
	plain, err := domain.CanonicalizeCustomerSupplement(psu1Spec(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := plain.Digest().String(); got != psu1Golden {
		t.Fatalf("digest = %s, want %s（补充口收平台段改了旧载荷的摘要，已存的重放判断会失效）", got, psu1Golden)
	}

	withPlatform := func(label string) domain.PayloadDigest {
		spec := psu1Spec(t)
		spec.PlatformShipment = platformShipment(t, "FBA15SYNTEST", "")
		spec.PlatformBoxLabels = []domain.PlatformBoxLabelDeclaration{boxLabel(t, "SYN-PCLREF-1", label)}
		canonical, err := domain.CanonicalizeCustomerSupplement(spec)
		if err != nil {
			t.Fatal(err)
		}
		return canonical.Digest()
	}
	first, second := withPlatform("FBA15SYNTESTU000001"), withPlatform("FBA15SYNTESTU000002")
	if first == plain.Digest() || first == second {
		t.Fatal("平台段没进补充摘要：同一补充请求标识改了箱标号会被当成重放")
	}
}
