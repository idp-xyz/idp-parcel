package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证解析键登记面（票 03 件 2）：登记后 FormResolutionKey
// 折出完整闭包解析键；无行是显式未配置（formed=false，不是 error）；重放与冲突分格且
// 冲突一行不动；租户与客户各自圈定；锚点与依据种类不设任何默认。

var resolutionKeyAnchorAt = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

func newResolutionKeys(t *testing.T) (*adapter.CommercialResolutionKeys, bentoapp.Transactor) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	keys, err := adapter.NewCommercialResolutionKeys(db)
	if err != nil {
		t.Fatalf("构造解析键登记面：%v", err)
	}
	return keys, db.Transactor()
}

func keyRegistration(t *testing.T, tenant, customer, scope string) adapter.ResolutionKeyRegistration {
	t.Helper()
	return adapter.ResolutionKeyRegistration{
		TenantID:          psValue(t, psdomain.NewTenantID, tenant),
		CustomerAccountID: psValue(t, psdomain.NewCustomerAccountID, customer),
		Scope:             psValue(t, pcdomain.NewCommercialScopeReference, scope),
		LegalEntity:       psValue(t, pcdomain.NewLegalEntityReference, "legal-1"),
		AnchorPolicy:      psValue(t, pcdomain.NewAnchorPolicyVersion, "anchor-policy/v1"),
		AnchorAt:          resolutionKeyAnchorAt,
		RequiredBases: []pcdomain.CommercialObjectKind{
			pcdomain.CustomerContractObject,
			pcdomain.AcceptanceRulePackageObject,
			pcdomain.ServiceProductObject,
		},
	}
}

func basisQuery(t *testing.T, tenant, customer string) psports.CommercialBasisQuery {
	t.Helper()
	return psports.CommercialBasisQuery{
		Identity:          identity(t, tenant, customer, "portal", "req-1"),
		ShipmentRequestID: psValue(t, psdomain.NewShipmentRequestID, "SHIP-1"),
		SubmissionVersion: psValue(t, psdomain.NewSubmissionVersionID, "SUB-1"),
	}
}

func mustRegisterKey(
	t *testing.T,
	transactor bentoapp.Transactor,
	keys *adapter.CommercialResolutionKeys,
	registration adapter.ResolutionKeyRegistration,
	want adapter.ResolutionKeySaveOutcome,
) {
	t.Helper()
	mustWithinTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		outcome, err := keys.Register(txCtx, registration)
		if err != nil {
			return err
		}
		if outcome != want {
			t.Fatalf("register outcome = %s, want %s", outcome, want)
		}
		return nil
	})
}

// Covers: 票 03 件 2——登记面四项（范围/法人候选/锚点策略/必需依据种类）落库后，
// FormResolutionKey 折出的键最小身份成立、目的钉在接受控制、锚点取登记值而不是任何时钟。
func TestARegisteredKeyFormsACompleteResolutionKey(t *testing.T) {
	keys, transactor := newResolutionKeys(t)
	mustRegisterKey(t, transactor, keys, keyRegistration(t, "tenant-1", "customer-1", "scope-1"), adapter.ResolutionKeySaved)

	key, formed, err := keys.FormResolutionKey(t.Context(), basisQuery(t, "tenant-1", "customer-1"))
	if err != nil {
		t.Fatalf("FormResolutionKey：%v", err)
	}
	if !formed {
		t.Fatal("登记过的键没形成")
	}
	if !key.MinimumIdentityEstablished() {
		t.Fatal("形成的键最小身份不成立")
	}
	if key.Scope.String() != "scope-1" || key.LegalEntityCandidate.String() != "legal-1" {
		t.Fatalf("范围/法人候选变形：%q %q", key.Scope, key.LegalEntityCandidate)
	}
	if key.Purpose != pcdomain.AcceptanceControlPurpose {
		t.Fatalf("purpose = %v, want 接受控制", key.Purpose)
	}
	if !key.Anchor.At().Equal(resolutionKeyAnchorAt) ||
		key.Anchor.PolicyVersion().String() != "anchor-policy/v1" {
		t.Fatalf("锚点没有取登记值：%v %q", key.Anchor.At(), key.Anchor.PolicyVersion())
	}
	if len(key.RequiredBases) != 3 {
		t.Fatalf("必需依据 = %v, want 3 项", key.RequiredBases)
	}
}

// Covers: 「今天 nil 即显式未配置」的登记面版本——无行交回 formed=false 且无错误，
// 与读取失败分格；他租户与他客户的登记互不可见。
func TestAnUnregisteredCustomerFormsNothing(t *testing.T) {
	keys, transactor := newResolutionKeys(t)

	if _, formed, err := keys.FormResolutionKey(t.Context(), basisQuery(t, "tenant-1", "customer-1")); err != nil || formed {
		t.Fatalf("formed=%v err=%v；未登记应是显式未配置，不是错误", formed, err)
	}

	mustRegisterKey(t, transactor, keys, keyRegistration(t, "tenant-1", "customer-1", "scope-1"), adapter.ResolutionKeySaved)

	t.Run("他租户", func(t *testing.T) {
		if _, formed, err := keys.FormResolutionKey(t.Context(), basisQuery(t, "tenant-b", "customer-1")); err != nil || formed {
			t.Fatalf("formed=%v err=%v；他租户看见了本租户的登记", formed, err)
		}
	})
	t.Run("他客户", func(t *testing.T) {
		if _, formed, err := keys.FormResolutionKey(t.Context(), basisQuery(t, "tenant-1", "customer-b")); err != nil || formed {
			t.Fatalf("formed=%v err=%v；他客户看见了本客户的登记", formed, err)
		}
	})
}

// Covers: 登记的落点代数——同（租户+客户）同参数是重放，异参数是冲突且原登记一行不动：
// 换范围或换锚点是换一套解析口径，要走显式新决定，不静默覆盖。
func TestKeyRegistrationReplayAndConflictSplitByContent(t *testing.T) {
	keys, transactor := newResolutionKeys(t)
	original := keyRegistration(t, "tenant-1", "customer-1", "scope-1")
	mustRegisterKey(t, transactor, keys, original, adapter.ResolutionKeySaved)
	mustRegisterKey(t, transactor, keys, original, adapter.ResolutionKeyAlreadyRegistered)

	changed := keyRegistration(t, "tenant-1", "customer-1", "scope-ANOTHER")
	mustRegisterKey(t, transactor, keys, changed, adapter.ResolutionKeyContentConflict)

	key, formed, err := keys.FormResolutionKey(t.Context(), basisQuery(t, "tenant-1", "customer-1"))
	if err != nil || !formed {
		t.Fatalf("formed=%v err=%v", formed, err)
	}
	if key.Scope.String() != "scope-1" {
		t.Fatalf("冲突写入改动了原登记：%q", key.Scope)
	}
}

// Covers: 登记不设默认——锚点零值、空依据集合、集合外与需要额外选择维度的种类都在
// 触库前被拒；写入拒绝在事务之外运行。
func TestKeyRegistrationRefusesDefaultsAndBareCalls(t *testing.T) {
	keys, transactor := newResolutionKeys(t)

	t.Run("锚点零值", func(t *testing.T) {
		broken := keyRegistration(t, "tenant-1", "customer-1", "scope-1")
		broken.AnchorAt = time.Time{}
		mustWithinTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
			if _, err := keys.Register(txCtx, broken); err == nil {
				t.Fatal("零值锚点被登记了——那就是等人来补默认")
			}
			return nil
		})
	})

	t.Run("空依据集合", func(t *testing.T) {
		broken := keyRegistration(t, "tenant-1", "customer-1", "scope-1")
		broken.RequiredBases = nil
		mustWithinTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
			if _, err := keys.Register(txCtx, broken); err == nil {
				t.Fatal("零必需依据的键被登记了")
			}
			return nil
		})
	})

	t.Run("结算政策要选择器", func(t *testing.T) {
		broken := keyRegistration(t, "tenant-1", "customer-1", "scope-1")
		broken.RequiredBases = append(broken.RequiredBases, pcdomain.SettlementPolicyObject)
		mustWithinTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
			if _, err := keys.Register(txCtx, broken); err == nil {
				t.Fatal("结算政策进了不带选择器的登记面——那个键永远立不起来")
			}
			return nil
		})
	})

	t.Run("无事务拒", func(t *testing.T) {
		if _, err := keys.Register(t.Context(), keyRegistration(t, "tenant-1", "customer-1", "scope-1")); !errors.Is(err, bentopg.ErrTransactionRequired) {
			t.Errorf("无事务登记应返回 ErrTransactionRequired，实得：%v", err)
		}
	})
}

func psValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}
