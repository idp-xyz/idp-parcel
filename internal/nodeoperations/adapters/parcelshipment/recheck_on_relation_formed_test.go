package parcelshipment_test

import (
	"context"
	"errors"
	"testing"

	noinbox "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/inbox"
	noparcel "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/parcelshipment"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件证 PS 门铃到重核编排的那一层翻译（ADR-0199 决定三）：门铃里的租户与号码原样交给按号重核，重核的错误原样交回，
// 消费门据此回滚、重投。夹具全为合成（S 级）。

type recheckByMarkDouble struct {
	asked []string
	err   error
}

func (double *recheckByMarkDouble) RecheckByMark(
	_ context.Context, tenant nodomain.TenantID, mark string,
) ([]application.RecheckPendingIdentityResult, error) {
	double.asked = append(double.asked, tenant.String()+"|"+mark)
	return nil, double.err
}

func TestADoorbellAsksTheRecheckByTheVerbatimNumber(t *testing.T) {
	recheck := &recheckByMarkDouble{}
	adapter, err := noparcel.NewRecheckOnRelationFormedAdapter(recheck)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := adapter.HandleFormedExternalIdentifierRelation(t.Context(),
		noinbox.FormedExternalIdentifierRelation{TenantID: "SYN-TENANT-NRS03", Number: " SYN-MARK-NRS03\t"}); err != nil {
		t.Fatalf("转交：%v", err)
	}
	if len(recheck.asked) != 1 || recheck.asked[0] != "SYN-TENANT-NRS03| SYN-MARK-NRS03\t" {
		t.Fatalf("按号重核被问 %q, want 原样的租户与号码各一次", recheck.asked)
	}

	recheck.err = application.ErrParcelIdentityUnreadable
	err = adapter.HandleFormedExternalIdentifierRelation(t.Context(),
		noinbox.FormedExternalIdentifierRelation{TenantID: "SYN-TENANT-NRS03", Number: "SYN-MARK-NRS03"})
	if !errors.Is(err, application.ErrParcelIdentityUnreadable) {
		t.Fatalf("重核的错误被吞了：%v", err)
	}
	if _, err := noparcel.NewRecheckOnRelationFormedAdapter(nil); err == nil {
		t.Fatal("缺重核编排却构造成功")
	}
}
