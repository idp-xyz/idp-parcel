package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 区间更正册的持久化面（ADR-0038 / D-5）。装载不在这里而在 CommercialPublications.LoadForScope
// ——更正随整册一次取回，理由见 ports.PublicationRegistry 的注释。这里只有写侧与把库里
// 聚好的更正数组折回领域对象那一步。

type correctionDocument struct {
	StartsAt    time.Time  `json:"startsAt"`
	EndsAt      *time.Time `json:"endsAt"`
	Reference   string     `json:"reference"`
	CorrectedAt time.Time  `json:"correctedAt"`
}

// SaveValidityCorrection 登记一条区间更正。只增不覆盖：同内容撞唯一键是重放，不同内容
// 是该版本的下一条更正。没有「内容冲突」格——异更正合法。
func (repository *CommercialPublications) SaveValidityCorrection(
	ctx context.Context,
	correction domain.ValidityCorrection,
) (ports.ValidityCorrectionSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ValidityCorrectionSaveOutcomeInvalid, fmt.Errorf("save validity correction: %w", err)
	}

	reference := correction.Reference().String()
	if reference == "" || correction.CorrectedAt().IsZero() {
		// 零值更正过不了 CorrectEffectiveInterval，走到这里说明调用方绕过了构造函数。
		// 拦在写入之前，否则库里会多一行读不回领域对象的记录。
		return ports.ValidityCorrectionSaveOutcomeInvalid,
			fmt.Errorf("save validity correction: 更正引用或更正时刻缺失，未经 CorrectEffectiveInterval 构造的更正不入册")
	}

	var endsAt *time.Time
	if end, bounded := correction.CorrectedInterval().EndsAt(); bounded {
		utc := end.UTC()
		endsAt = &utc
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.commercial_validity_correction
			(tenant_id, object_kind, object_id, version_label,
			 corrected_starts_at, corrected_ends_at, correction_ref, corrected_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT ON CONSTRAINT commercial_validity_correction_same_content DO NOTHING`,
		correction.Tenant().String(),
		uint8(correction.Kind()),
		correction.ObjectID().String(),
		correction.Version().String(),
		correction.CorrectedInterval().StartsAt().UTC(),
		endsAt,
		reference,
		correction.CorrectedAt().UTC(),
	)
	if err != nil {
		return ports.ValidityCorrectionSaveOutcomeInvalid, fmt.Errorf("save validity correction: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return ports.ValidityCorrectionSaved, nil
	}
	return ports.ValidityCorrectionAlreadyRegistered, nil
}

// registerValidityCorrections 把一个版本下按登记序排好的更正数组挂回登记册。
//
// 空数组是「这个版本没更正」，不是缺陷。取值领域不认则整次数组上抛，不跳过某一条：
// 跳过会让登记册少一份更正，解析因此按另一条（或原区间）选用，一次坏数据会伪装成
// 合法的历史缺席。
func registerValidityCorrections(
	registry *domain.CommercialRegistry,
	version domain.CommercialVersion,
	raw []byte,
) error {
	if len(raw) == 0 || string(raw) == "[]" || string(raw) == "null" {
		return nil
	}
	var items []correctionDocument
	if err := json.Unmarshal(raw, &items); err != nil {
		return fmt.Errorf("更正快照不是本适配器写下的形状：%w", err)
	}
	for _, item := range items {
		if err := registerValidityCorrection(registry, version, item); err != nil {
			return err
		}
	}
	return nil
}

func registerValidityCorrection(
	registry *domain.CommercialRegistry,
	version domain.CommercialVersion,
	item correctionDocument,
) error {
	reference, err := domain.NewValidityCorrectionReference(item.Reference)
	if err != nil {
		return err
	}
	endsAt := time.Time{}
	if item.EndsAt != nil {
		endsAt = *item.EndsAt
	}
	interval, err := domain.NewEffectiveInterval(item.StartsAt, endsAt)
	if err != nil {
		return err
	}
	correction, err := version.CorrectEffectiveInterval(interval, reference, item.CorrectedAt)
	if err != nil {
		return err
	}
	if _, err := registry.RegisterValidityCorrection(correction); err != nil {
		return err
	}
	return nil
}
