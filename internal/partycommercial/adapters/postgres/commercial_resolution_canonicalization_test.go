package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件钉解析快照的规范化版本（ADR-0014 Decision）：摘要串与快照文档都带产生它们的版本；撞键先比版本再比摘要；
// 读回按版本分派，本构建不认得的版本响亮失败。版本键缺席的行是快照接版本之前写下的，按它自己那一形比、按第一版读。
//
// 两个钉住的串是 main `32dc1d98d`（快照接版本之前）对同一夹具实算出的无前缀摘要：无版本那一形若与之前写下的字节差
// 一个字节，存量行重放就答成冲突，先红的是这里。
const (
	unversionedDigestOfUniqueClosure     = "72236e55ae994bb6300503d5bac672160b4269a1d3d640c211ff29cd55eb37c0"
	unversionedDigestOfFixedBaseAccepted = "96273b6f2939904aea1db3e1255d8409efe22ac2764eb39e1fd21fef03c796c6"
)

func TestANewSnapshotCarriesItsCanonicalization(t *testing.T) {
	repository, transactor, pool := newResolutions(t)
	closure := uniqueClosure(t)
	mustSaveResolution(t, transactor, t.Context(), repository, closure)

	if digest := storedContentDigest(t, pool, closure); !strings.HasPrefix(digest, "PCR-1:") {
		t.Fatalf("content_digest = %q, want PCR-1:<hex>——不带版本的摘要是不完整的（ADR-0014）", digest)
	}
	if shape := storedSnapshotShape(t, pool, closure); shape != "PCR-1" {
		t.Fatalf("snapshot.canonicalization = %q, want PCR-1", shape)
	}
}

func TestASameVersionReplayIsRecordedAndADifferentDigestIsAConflict(t *testing.T) {
	repository, transactor, pool := newResolutions(t)
	closure := uniqueClosure(t)
	mustSaveResolution(t, transactor, t.Context(), repository, closure)

	if outcome := saveOutcome(t, transactor, repository, closure); outcome != ports.ResolutionAlreadyRecorded {
		t.Fatalf("同版本重放 = %s, want ALREADY_RECORDED", outcome)
	}

	setStoredDigest(t, pool, closure, "PCR-1:forged")
	if outcome := saveOutcome(t, transactor, repository, closure); outcome != ports.ResolutionContentConflict {
		t.Fatalf("同版本异摘要 = %s, want CONTENT_CONFLICT", outcome)
	}
}

// Covers: ADR-0014 Decision「回放按原……规范化版本重新规范化，再比对摘要」——册上是无版本那一形，本构建写 PCR-1，
// 两版不同不是冲突：按无版本那一形重算本次闭包，同则重放、异才冲突。底座一格那份夹具钉的是 code/61 之后的字节。
func TestAnUnversionedRowIsComparedInItsOwnShape(t *testing.T) {
	for name, fixture := range map[string]struct {
		pinned string
		fix    func(t *testing.T) (domain.CommercialClosure, *adapter.CommercialResolutions, bentoapp.Transactor, *pgxpool.Pool)
	}{
		"只有合同与规则包": {unversionedDigestOfUniqueClosure, func(t *testing.T) (domain.CommercialClosure, *adapter.CommercialResolutions, bentoapp.Transactor, *pgxpool.Pool) {
			repository, transactor, pool := newResolutions(t)
			closure := uniqueClosure(t)
			mustSaveResolution(t, transactor, t.Context(), repository, closure)
			return closure, repository, transactor, pool
		}},
		"合同版客户服务规则带固定的底座": {unversionedDigestOfFixedBaseAccepted, func(t *testing.T) (domain.CommercialClosure, *adapter.CommercialResolutions, bentoapp.Transactor, *pgxpool.Pool) {
			base := newFixedBaseFixture(t)
			base.seed(t)
			return base.accept(t), base.resolutions, base.transactor, base.pool
		}},
	} {
		t.Run(name, func(t *testing.T) {
			closure, repository, transactor, pool := fixture.fix(t)
			regressToUnversioned(t, pool, closure, fixture.pinned)

			if outcome := saveOutcome(t, transactor, repository, closure); outcome != ports.ResolutionAlreadyRecorded {
				t.Fatalf("无版本行上同内容重放 = %s, want ALREADY_RECORDED", outcome)
			}
			if digest := storedContentDigest(t, pool, closure); digest != fixture.pinned {
				t.Fatalf("重放改写了存量行的摘要：%q", digest)
			}

			setStoredDigest(t, pool, closure, "forged-unversioned")
			if outcome := saveOutcome(t, transactor, repository, closure); outcome != ports.ResolutionContentConflict {
				t.Fatalf("无版本行上异内容 = %s, want CONTENT_CONFLICT", outcome)
			}
		})
	}
}

// Covers: 票 code/80「版本键缺席的快照是本票之前的形，按第一版读」——底座一格在第一版之内照旧可缺（ADR-0268 决定三），
// 在场的就照读，不因缺版本键而丢。
func TestAnUnversionedSnapshotReadsBackAsTheFirstVersion(t *testing.T) {
	base := newFixedBaseFixture(t)
	base.seed(t)
	held := base.accept(t)
	regressToUnversioned(t, base.pool, held, unversionedDigestOfFixedBaseAccepted)

	legacy := base.reload(t, held.ResolutionID())
	adopted, _ := legacy.AdoptedFor(domain.CustomerServiceRuleObject)
	if product, hasBase, fixed := adopted.InheritedProductBase(); !fixed || !hasBase || product.Version().String() != "v1" {
		t.Fatalf("无版本快照读回的底座：在场=%v 已固定=%v 版本=%s，want 已固定的 v1", hasBase, fixed, product.Version())
	}
}

// Covers: ADR-0014 Decision「规范化版本不同不是冲突，也不得据此判定重放失败」与票 code/80「遇到本构建不认得的版本响亮
// 失败」——滚动发布时新构建写下的行，旧构建既比不了也读不懂：答 error，不答冲突挡接受，也不答重放冒认。
func TestAVersionThisBuildDoesNotKnowIsNeitherAConflictNorAReplay(t *testing.T) {
	repository, transactor, pool := newResolutions(t)
	closure := uniqueClosure(t)
	mustSaveResolution(t, transactor, t.Context(), repository, closure)
	if _, err := pool.Exec(t.Context(),
		`UPDATE party_commercial.commercial_resolution
		    SET snapshot = jsonb_set(snapshot, '{canonicalization}', '"PCR-9"'),
		        content_digest = 'PCR-9:written-by-a-later-build'
		  WHERE tenant_id = $1 AND resolution_id = $2`,
		closure.ResolutionKey().TenantID.String(), closure.ResolutionID().String(),
	); err != nil {
		t.Fatalf("改写成后来版本：%v", err)
	}

	var outcome ports.ResolutionSaveOutcome
	err := transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Save(txCtx, closure)
		return err
	})
	if !errors.Is(err, domain.ErrCanonicalizationUnsupported) {
		t.Fatalf("撞键遇到未知版本：outcome=%s err=%v，want ErrCanonicalizationUnsupported", outcome, err)
	}
	if outcome == ports.ResolutionContentConflict || outcome == ports.ResolutionAlreadyRecorded {
		t.Fatalf("未知版本答成了 %s", outcome)
	}

	if _, found, err := repository.LoadResolution(t.Context(), closure.ResolutionKey().TenantID, closure.ResolutionID()); !errors.Is(err, domain.ErrCanonicalizationUnsupported) || found {
		t.Fatalf("读回未知版本：found=%v err=%v，want ErrCanonicalizationUnsupported", found, err)
	}
}

// Covers: 票 code/80「按版本键分派解码……不按旧形读、不丢键」——第一版文档里出现本构建不认得的键，是形状变了却没换号，
// 读回报错而不是丢掉那一格。
func TestAFirstVersionSnapshotWithAKeyThisBuildDoesNotKnowFailsLoudly(t *testing.T) {
	for name, path := range map[string]string{
		"顶层":   "{laterKey}",
		"已采用项": "{adopted,0,laterKey}",
	} {
		t.Run(name, func(t *testing.T) {
			repository, transactor, pool := newResolutions(t)
			closure := uniqueClosure(t)
			mustSaveResolution(t, transactor, t.Context(), repository, closure)
			if _, err := pool.Exec(t.Context(),
				`UPDATE party_commercial.commercial_resolution
				    SET snapshot = jsonb_set(snapshot, $3::text[], '"later"')
				  WHERE tenant_id = $1 AND resolution_id = $2`,
				closure.ResolutionKey().TenantID.String(), closure.ResolutionID().String(), path,
			); err != nil {
				t.Fatalf("加一个未知键：%v", err)
			}
			if _, found, err := repository.LoadResolution(t.Context(), closure.ResolutionKey().TenantID, closure.ResolutionID()); err == nil || found {
				t.Fatalf("带未知键的第一版快照读回成功：found=%v", found)
			}
		})
	}
}

// 快照文档说的版本与摘要串带的版本必须是同一个：两处各说一版的行不是本适配器写下的，读回时无从知道该信哪一处。
func TestASnapshotWhoseVersionDisagreesWithItsDigestIsNotRead(t *testing.T) {
	for name, tamper := range map[string]string{
		"快照有版本、摘要无版本": `UPDATE party_commercial.commercial_resolution SET content_digest = 'bare'
			WHERE tenant_id = $1 AND resolution_id = $2`,
		"快照无版本、摘要有版本": `UPDATE party_commercial.commercial_resolution SET snapshot = snapshot - 'canonicalization'
			WHERE tenant_id = $1 AND resolution_id = $2`,
	} {
		t.Run(name, func(t *testing.T) {
			repository, transactor, pool := newResolutions(t)
			closure := uniqueClosure(t)
			mustSaveResolution(t, transactor, t.Context(), repository, closure)
			if _, err := pool.Exec(t.Context(), tamper,
				closure.ResolutionKey().TenantID.String(), closure.ResolutionID().String()); err != nil {
				t.Fatalf("改写：%v", err)
			}
			if _, found, err := repository.LoadResolution(t.Context(), closure.ResolutionKey().TenantID, closure.ResolutionID()); err == nil || found {
				t.Fatalf("版本两说的行读回成功：found=%v", found)
			}
		})
	}
}

func saveOutcome(
	t *testing.T,
	transactor bentoapp.Transactor,
	repository *adapter.CommercialResolutions,
	closure domain.CommercialClosure,
) ports.ResolutionSaveOutcome {
	t.Helper()
	var outcome ports.ResolutionSaveOutcome
	mustWithinResolutionTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		outcome, err = repository.Save(txCtx, closure)
		return err
	})
	return outcome
}

// regressToUnversioned 把一行改成快照接版本之前写下的样子：快照没有版本键，摘要是无前缀的串。
func regressToUnversioned(t *testing.T, pool *pgxpool.Pool, closure domain.CommercialClosure, digest string) {
	t.Helper()
	tag, err := pool.Exec(t.Context(),
		`UPDATE party_commercial.commercial_resolution
		    SET snapshot = snapshot - 'canonicalization', content_digest = $3
		  WHERE tenant_id = $1 AND resolution_id = $2`,
		closure.ResolutionKey().TenantID.String(), closure.ResolutionID().String(), digest)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("改写成无版本行：rows=%d err=%v", tag.RowsAffected(), err)
	}
}

func setStoredDigest(t *testing.T, pool *pgxpool.Pool, closure domain.CommercialClosure, digest string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`UPDATE party_commercial.commercial_resolution
		    SET content_digest = $3
		  WHERE tenant_id = $1 AND resolution_id = $2`,
		closure.ResolutionKey().TenantID.String(), closure.ResolutionID().String(), digest,
	); err != nil {
		t.Fatalf("改写摘要：%v", err)
	}
}

func storedSnapshotShape(t *testing.T, pool *pgxpool.Pool, closure domain.CommercialClosure) string {
	t.Helper()
	var shape *string
	if err := pool.QueryRow(t.Context(),
		`SELECT snapshot->>'canonicalization'
		   FROM party_commercial.commercial_resolution
		  WHERE tenant_id = $1 AND resolution_id = $2`,
		closure.ResolutionKey().TenantID.String(), closure.ResolutionID().String(),
	).Scan(&shape); err != nil {
		t.Fatalf("读快照版本：%v", err)
	}
	if shape == nil {
		return ""
	}
	return *shape
}
