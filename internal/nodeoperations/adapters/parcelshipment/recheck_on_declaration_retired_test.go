package parcelshipment_test

import (
	"errors"
	"testing"

	noinbox "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/inbox"
	noparcel "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/parcelshipment"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
)

// Covers: ADR-0211 决定三——PS 声明撤除门铃到重核编排的那一层翻译，与关联已形成那一只同形：租户与号码原样交给按号重核，重核的
// 错误原样交回，消费门据此回滚、重投。夹具全为合成（S 级）。
func TestARetiredDeclarationAsksTheRecheckByTheVerbatimNumber(t *testing.T) {
	recheck := &recheckByMarkDouble{}
	adapter, err := noparcel.NewRecheckOnDeclarationRetiredAdapter(recheck)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := adapter.HandleRetiredCustomerParcelNumberDeclaration(t.Context(),
		noinbox.RetiredCustomerParcelNumberDeclaration{TenantID: "SYN-TENANT-NRI07", Number: " SYN-MARK-NRI07\t"}); err != nil {
		t.Fatalf("转交：%v", err)
	}
	if len(recheck.asked) != 1 || recheck.asked[0] != "SYN-TENANT-NRI07| SYN-MARK-NRI07\t" {
		t.Fatalf("按号重核被问 %q, want 原样的租户与号码各一次", recheck.asked)
	}

	recheck.err = application.ErrParcelIdentityUnreadable
	err = adapter.HandleRetiredCustomerParcelNumberDeclaration(t.Context(),
		noinbox.RetiredCustomerParcelNumberDeclaration{TenantID: "SYN-TENANT-NRI07", Number: "SYN-MARK-NRI07"})
	if !errors.Is(err, application.ErrParcelIdentityUnreadable) {
		t.Fatalf("重核的错误被吞了：%v", err)
	}
	if _, err := noparcel.NewRecheckOnDeclarationRetiredAdapter(nil); err == nil {
		t.Fatal("缺重核编排却构造成功")
	}
}
