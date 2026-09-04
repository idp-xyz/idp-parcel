package postgres_test

import (
	"errors"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// PBC-08 行为面负向证据（bento-gate-reeval 票 02）：声明发布面的四个漏网写口在无事务
// 上下文必须被 RequireExecutor 拒绝。拒绝先于任何入参解读，所以传零值就够；本类型
// 其余 Save* 写口的同款证据在既有测试文件里，这里只补漏网的。
func TestDeclarationContentWritesRefuseToRunOutsideATransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	ctx := t.Context()

	publications, err := adapter.NewCommercialPublications(db)
	if err != nil {
		t.Fatalf("构造声明写入面：%v", err)
	}
	if _, err := publications.SavePricePolicy(ctx, domain.CommercialPricePolicy{}, 0, domain.PlanBindingConversionNone); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记价格规则正文应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := publications.SaveServiceProduct(ctx, domain.ServiceProduct{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记服务产品形态应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := publications.SaveSettlementPolicy(ctx, domain.SettlementPolicy{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记结算政策应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := publications.SaveValidityCorrection(ctx, domain.ValidityCorrection{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记有效性更正应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := publications.SaveCreditPolicy(ctx, domain.CreditPolicy{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记信用政策正文应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := publications.SaveSupplierAgreement(ctx, domain.SupplierAgreement{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记供应商协议正文应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := publications.SavePricePolicyCaliber(ctx, domain.PricePolicyCaliber{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记价格政策口径应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := publications.SaveCustomerServiceRule(ctx, domain.CustomerServiceRuleVersion{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记客户服务规则正文应返回 ErrTransactionRequired，实得：%v", err)
	}
}
