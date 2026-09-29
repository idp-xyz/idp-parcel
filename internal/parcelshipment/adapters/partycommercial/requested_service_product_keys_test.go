package partycommercial_test

import (
	"context"
	"errors"
	"testing"

	pspartycommercial "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件钉票 psb/17 的键来源半边：登记面折出的闭包键上叠委托声明的服务产品。

type formedKeyDouble struct {
	key    pcdomain.ClosureResolutionKey
	formed bool
	err    error
}

func (double formedKeyDouble) FormResolutionKey(
	context.Context, psports.CommercialBasisQuery,
) (pcdomain.ClosureResolutionKey, bool, error) {
	return double.key, double.formed, double.err
}

type productSourceDouble struct {
	product psdomain.DeclaredServiceProduct
	err     error
	calls   int
}

func (double *productSourceDouble) RequestedServiceProduct(
	context.Context, psports.CommercialBasisQuery,
) (psdomain.DeclaredServiceProduct, error) {
	double.calls++
	return double.product, double.err
}

func declaredProduct(t *testing.T, value string) psdomain.DeclaredServiceProduct {
	t.Helper()
	entry, err := psdomain.NewCanonicalContentEntry(psdomain.RequestedServiceProductEntryName, value)
	if err != nil {
		t.Fatalf("条目：%v", err)
	}
	return psdomain.RequestedServiceProductOf([]psdomain.CanonicalContentEntry{entry})
}

func keyRequiring(kinds ...pcdomain.CommercialObjectKind) pcdomain.ClosureResolutionKey {
	return pcdomain.ClosureResolutionKey{Purpose: pcdomain.AcceptanceControlPurpose, RequiredBases: kinds}
}

func productKeys(t *testing.T, keys formedKeyDouble, products *productSourceDouble) *pspartycommercial.RequestedServiceProductKeys {
	t.Helper()
	adapter, err := pspartycommercial.NewRequestedServiceProductKeys(keys, products)
	if err != nil {
		t.Fatalf("new requested service product keys: %v", err)
	}
	return adapter
}

func TestTheDeclaredServiceProductIsLaidOnTheFormedKey(t *testing.T) {
	products := &productSourceDouble{product: declaredProduct(t, "SYN-PROD-CN-SG-EXPRESS")}
	adapter := productKeys(t, formedKeyDouble{
		key:    keyRequiring(pcdomain.CustomerContractObject, pcdomain.ServiceProductObject),
		formed: true,
	}, products)

	key, formed, err := adapter.FormResolutionKey(context.Background(), psports.CommercialBasisQuery{})
	if err != nil || !formed {
		t.Fatalf("formed = %v err = %v", formed, err)
	}
	if key.ServiceProduct.String() != "SYN-PROD-CN-SG-EXPRESS" {
		t.Fatalf("键上声明的产品 = %q", key.ServiceProduct.String())
	}
}

func TestAnUndeclaredServiceProductLeavesTheKeyAsFormed(t *testing.T) {
	adapter := productKeys(t, formedKeyDouble{
		key:    keyRequiring(pcdomain.ServiceProductObject),
		formed: true,
	}, &productSourceDouble{})

	key, formed, err := adapter.FormResolutionKey(context.Background(), psports.CommercialBasisQuery{})
	if err != nil || !formed {
		t.Fatalf("formed = %v err = %v", formed, err)
	}
	if key.ServiceProduct.String() != "" {
		t.Fatalf("没声明却叠上了 %q", key.ServiceProduct.String())
	}
}

// 闭包不要服务产品依据时不去读声明：叠上去的键立不起来（最小身份不成立），读了也用不上。
func TestAKeyThatDoesNotRequestAServiceProductIsNotTouched(t *testing.T) {
	products := &productSourceDouble{product: declaredProduct(t, "SYN-PROD-CN-SG-EXPRESS")}
	adapter := productKeys(t, formedKeyDouble{key: keyRequiring(pcdomain.CustomerContractObject), formed: true}, products)

	key, formed, err := adapter.FormResolutionKey(context.Background(), psports.CommercialBasisQuery{})
	if err != nil || !formed || key.ServiceProduct.String() != "" {
		t.Fatalf("key product = %q formed = %v err = %v", key.ServiceProduct.String(), formed, err)
	}
	if products.calls != 0 {
		t.Fatalf("不要服务产品依据也去读了声明 %d 次", products.calls)
	}
}

// 登记面没折出键（未配置）或折键失败时原样交回，不去读声明：未配置与读不到分格的责任在登记面。
func TestAnUnformedOrFailedKeyPassesThroughUntouched(t *testing.T) {
	products := &productSourceDouble{product: declaredProduct(t, "SYN-PROD-CN-SG-EXPRESS")}
	unformed := productKeys(t, formedKeyDouble{formed: false}, products)
	if _, formed, err := unformed.FormResolutionKey(context.Background(), psports.CommercialBasisQuery{}); formed || err != nil {
		t.Fatalf("未配置 → formed = %v err = %v", formed, err)
	}
	boom := errors.New("registration unreadable")
	failed := productKeys(t, formedKeyDouble{err: boom}, products)
	if _, _, err := failed.FormResolutionKey(context.Background(), psports.CommercialBasisQuery{}); !errors.Is(err, boom) {
		t.Fatalf("折键失败没有原样上抛：%v", err)
	}
	if products.calls != 0 {
		t.Fatalf("键没折出来也去读了声明 %d 次", products.calls)
	}
}

// 读不到声明是依赖故障，不是「客户没声明」：压成未声明会让同一范围两个产品的委托换一个冲突答复，恢复动作指错。
func TestAnUnreadableDeclarationIsAnErrorNotAnUndeclaredProduct(t *testing.T) {
	boom := errors.New("request store unreadable")
	adapter := productKeys(t, formedKeyDouble{key: keyRequiring(pcdomain.ServiceProductObject), formed: true},
		&productSourceDouble{err: boom})

	if _, _, err := adapter.FormResolutionKey(context.Background(), psports.CommercialBasisQuery{}); !errors.Is(err, boom) {
		t.Fatalf("读声明失败没有上抛：%v", err)
	}
}

// 全空白的声明在 party-commercial 的对象标识里没有对应（它拒空白标识）。今天唯一的接单入口不产出这种条目，
// 真出现就是某个入口写坏了，响亮报出，不静默当成没声明。
func TestABlankDeclarationIsUntranslatable(t *testing.T) {
	adapter := productKeys(t, formedKeyDouble{key: keyRequiring(pcdomain.ServiceProductObject), formed: true},
		&productSourceDouble{product: declaredProduct(t, "   ")})

	if _, _, err := adapter.FormResolutionKey(context.Background(), psports.CommercialBasisQuery{}); err == nil {
		t.Fatal("全空白的声明被叠上了键或被当成了没声明")
	}
}

func TestRequestedServiceProductKeysRefuseMissingCollaborators(t *testing.T) {
	if _, err := pspartycommercial.NewRequestedServiceProductKeys(nil, &productSourceDouble{}); err == nil {
		t.Fatal("没有登记面也立起来了")
	}
	if _, err := pspartycommercial.NewRequestedServiceProductKeys(formedKeyDouble{}, nil); err == nil {
		t.Fatal("没有声明读口也立起来了")
	}
}
