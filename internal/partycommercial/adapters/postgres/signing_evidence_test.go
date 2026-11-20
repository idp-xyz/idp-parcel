package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

func newSigningEvidences(t *testing.T) (*adapter.SigningEvidences, *adapter.OperationsCatalogue, bentoapp.Transactor, *bentopg.DB) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := adapter.NewSigningEvidences(db)
	if err != nil {
		t.Fatalf("构造签署证据册：%v", err)
	}
	catalogue, err := adapter.NewOperationsCatalogue(db)
	if err != nil {
		t.Fatalf("构造目录：%v", err)
	}
	return store, catalogue, db.Transactor(), db
}

func insertSigningTarget(t *testing.T, db *bentopg.DB, transactor bentoapp.Transactor, kind domain.CommercialObjectKind, objectID, version string, draft bool) {
	t.Helper()
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	mustWithinPublicationTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		executor, err := db.RequireExecutor(txCtx)
		if err != nil {
			return err
		}
		if draft {
			_, err = executor.Exec(txCtx,
				`INSERT INTO party_commercial.publication_draft (
				    tenant_id, object_kind, object_id, version_label, scope_ref,
				    effective_starts_at, declared_references, content_canonicalization, content_digest,
				    content_document, submitter_ref, submitted_at, status)
				 VALUES ($1, $2, $3, $4, 'scope-1', $5, '[]'::jsonb, 'PCC-1', 'PCC-1:abc',
				         '{}'::jsonb, 'op-1', $5, 1)`,
				"tenant-1", int16(kind), objectID, version, at)
			return err
		}
		_, err = executor.Exec(txCtx,
			`INSERT INTO party_commercial.commercial_version (
			    tenant_id, object_kind, object_id, version_label, scope_ref, content_digest,
			    effective_starts_at, status, snapshot, published_at)
			 VALUES ($1, $2, $3, $4, 'scope-1', 'PCC-1:abc', $5, 3, '{}'::jsonb, $5)`,
			"tenant-1", int16(kind), objectID, version, at)
		return err
	})
}

func signingEvidenceOn(t *testing.T, id, objectID, version, corrects, digest string) domain.SigningEvidence {
	t.Helper()
	var prior domain.SigningEvidenceID
	if corrects != "" {
		prior = pcValue(t, domain.NewSigningEvidenceID, corrects)
	}
	file, err := domain.NewSigningFile("signed.pdf", digest, "EVD-RESTRICTED-01")
	if err != nil {
		t.Fatalf("签署件：%v", err)
	}
	evidence, err := domain.NewSigningEvidence(
		pcValue(t, domain.NewSigningEvidenceID, id),
		pcTenant(t, "tenant-1"),
		domain.CustomerContractObject,
		pcValue(t, domain.NewCommercialObjectID, objectID),
		pcValue(t, domain.NewCommercialVersionLabel, version),
		time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		"甲方", "",
		file,
		domain.SigningEvidenceOfflineSigned,
		prior,
		pcValue(t, domain.NewOperatorSubjectReference, "op-recorder"),
		time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("证据 %s：%v", id, err)
	}
	return evidence
}

func saveSigningEvidence(t *testing.T, transactor bentoapp.Transactor, store *adapter.SigningEvidences, evidence domain.SigningEvidence) ports.SigningEvidenceSaveOutcome {
	t.Helper()
	var outcome ports.SigningEvidenceSaveOutcome
	mustWithinPublicationTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		outcome, err = store.SaveSigningEvidence(txCtx, evidence)
		return err
	})
	return outcome
}

func TestSigningEvidenceRegistersCorrectsOnceAndStaysBesideTheVersion(t *testing.T) {
	store, catalogue, transactor, db := newSigningEvidences(t)
	ctx := t.Context()
	insertSigningTarget(t, db, transactor, domain.CustomerContractObject, "contract-1", "v1", false)
	insertSigningTarget(t, db, transactor, domain.CustomerContractObject, "contract-1", "v2", false)
	insertSigningTarget(t, db, transactor, domain.CustomerContractObject, "contract-draft", "v1", true)

	tenant := pcTenant(t, "tenant-1")
	contract := pcValue(t, domain.NewCommercialObjectID, "contract-1")
	version := pcValue(t, domain.NewCommercialVersionLabel, "v1")
	before, found, err := catalogue.FindCustomerContract(ctx, tenant, contract, version)
	if err != nil || !found || before.SigningEvidenceRegistered {
		t.Fatalf("登记前目录 = found %v registered %v err %v", found, before.SigningEvidenceRegistered, err)
	}

	digest := strings.Repeat("ab", 32)
	original := signingEvidenceOn(t, "evidence-1", "contract-1", "v1", "", digest)
	if outcome := saveSigningEvidence(t, transactor, store, original); outcome != ports.SigningEvidenceSaved {
		t.Fatalf("首登 = %s", outcome)
	}
	if outcome := saveSigningEvidence(t, transactor, store, original); outcome != ports.SigningEvidenceAlreadyRegistered {
		t.Fatalf("重放 = %s", outcome)
	}
	changed := signingEvidenceOn(t, "evidence-1", "contract-1", "v1", "", strings.Repeat("cd", 32))
	if outcome := saveSigningEvidence(t, transactor, store, changed); outcome != ports.SigningEvidenceContentConflict {
		t.Fatalf("异内容 = %s", outcome)
	}

	correction := signingEvidenceOn(t, "evidence-2", "contract-1", "v1", "evidence-1", strings.Repeat("ef", 32))
	if outcome := saveSigningEvidence(t, transactor, store, correction); outcome != ports.SigningEvidenceSaved {
		t.Fatalf("更正 = %s", outcome)
	}
	again := signingEvidenceOn(t, "evidence-3", "contract-1", "v1", "evidence-1", strings.Repeat("12", 32))
	if outcome := saveSigningEvidence(t, transactor, store, again); outcome != ports.SigningEvidenceAlreadyCorrected {
		t.Fatalf("再更正 = %s", outcome)
	}
	cross := signingEvidenceOn(t, "evidence-4", "contract-1", "v2", "evidence-1", strings.Repeat("34", 32))
	if outcome := saveSigningEvidence(t, transactor, store, cross); outcome != ports.SigningEvidenceCrossVersion {
		t.Fatalf("跨版本 = %s", outcome)
	}

	listed, err := store.ListSigningEvidence(ctx, tenant, domain.CustomerContractObject, contract, version)
	if err != nil || len(listed) != 2 {
		t.Fatalf("历史 = %d, %v", len(listed), err)
	}
	if listed[0].ID().String() != "evidence-1" || listed[1].ID().String() != "evidence-2" {
		t.Fatalf("历史顺序 = %s %s", listed[0].ID(), listed[1].ID())
	}
	after, found, err := catalogue.FindCustomerContract(ctx, tenant, contract, version)
	if err != nil || !found || !after.SigningEvidenceRegistered {
		t.Fatalf("登记后目录 = found %v registered %v err %v", found, after.SigningEvidenceRegistered, err)
	}

	missing := signingEvidenceOn(t, "evidence-missing", "contract-absent", "v1", "", digest)
	if outcome := saveSigningEvidence(t, transactor, store, missing); outcome != ports.SigningEvidenceTargetNotFound {
		t.Fatalf("目标不在 = %s", outcome)
	}
	draftOnly := signingEvidenceOn(t, "evidence-draft", "contract-draft", "v1", "", digest)
	if outcome := saveSigningEvidence(t, transactor, store, draftOnly); outcome != ports.SigningEvidenceSaved {
		t.Fatalf("只在草稿上 = %s", outcome)
	}
}

func TestSigningEvidenceWritesRefuseToRunOutsideATransaction(t *testing.T) {
	store, _, _, _ := newSigningEvidences(t)
	ctx := t.Context()
	if _, err := store.SaveSigningEvidence(ctx, domain.SigningEvidence{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记证据：%v", err)
	}
	if _, err := store.SaveSigningEvidenceRequirement(ctx, domain.SigningEvidenceRequirement{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记要求：%v", err)
	}
}
