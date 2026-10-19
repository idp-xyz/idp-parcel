package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// ActionAdmissionJudgements 实现 ports.ActionAdmissionJudgementStore（0024 建表，ADR-0206）。只建立不更新：标识由
// 内容派生，同一请求在同样三源上重放撞主键，ON CONFLICT DO NOTHING 折成`已记录`；三源变了标识就变，另成一行。
// 不入队——本册今天没有订阅者，写口也就不碰 Outbox。
type ActionAdmissionJudgements struct {
	db *bentopg.DB
}

func NewActionAdmissionJudgements(db *bentopg.DB) (*ActionAdmissionJudgements, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &ActionAdmissionJudgements{db: db}, nil
}

var _ ports.ActionAdmissionJudgementStore = (*ActionAdmissionJudgements)(nil)

type admissionZoneRow struct {
	Node       string `json:"node"`
	Zone       string `json:"zone,omitempty"`
	Registered bool   `json:"registered"`
}

type admissionBasisRow struct {
	Cause     string   `json:"cause"`
	Scope     string   `json:"scope,omitempty"`
	Procedure string   `json:"procedure,omitempty"`
	Sources   []string `json:"sources"`
}

type admissionFindingRow struct {
	Action string              `json:"action"`
	Answer string              `json:"answer"`
	Bases  []admissionBasisRow `json:"bases"`
}

// SaveActionAdmissionJudgement 走 RequireExecutor：判断与它的冲突判定读回要在同一事务里看同一份快照。词表外的取值
// 在这里就拒——落到库上 CHECK 才被拦，答案会滑成「依赖故障」，而那明明是编程错误。
func (store *ActionAdmissionJudgements) SaveActionAdmissionJudgement(
	ctx context.Context,
	tenant domain.TenantID,
	judgementID string,
	judgement domain.ActionAdmissionJudgement,
) (ports.ActionAdmissionSaveOutcome, error) {
	intent := judgement.Intent()
	if intent.Kind().String() == "" || judgement.CrossZone().String() == "" || judgement.Answer().String() == "" {
		return ports.ActionAdmissionSaveOutcomeInvalid, fmt.Errorf("save action admission judgement: an enumeration is outside its vocabulary")
	}
	zones := make([]admissionZoneRow, 0, len(judgement.Zones()))
	for _, zone := range judgement.Zones() {
		zones = append(zones, admissionZoneRow{Node: zone.Node.String(), Zone: zone.Zone.String(), Registered: zone.Registered})
	}
	findings := make([]admissionFindingRow, 0, len(judgement.Findings()))
	for _, finding := range judgement.Findings() {
		row := admissionFindingRow{Action: finding.Action.String(), Answer: finding.Answer.String()}
		if row.Action == "" || row.Answer == "" {
			return ports.ActionAdmissionSaveOutcomeInvalid, fmt.Errorf("save action admission judgement: a finding is outside its vocabulary")
		}
		for _, basis := range finding.Bases {
			if basis.Cause.String() == "" {
				return ports.ActionAdmissionSaveOutcomeInvalid, fmt.Errorf("save action admission judgement: a basis cause is outside its vocabulary")
			}
			row.Bases = append(row.Bases, admissionBasisRow{
				Cause:     basis.Cause.String(),
				Scope:     basis.Scope.String(),
				Procedure: basis.Procedure.String(),
				Sources:   append([]string{}, basis.Sources...),
			})
		}
		findings = append(findings, row)
	}
	zonesJSON, err := json.Marshal(zones)
	if err != nil {
		return ports.ActionAdmissionSaveOutcomeInvalid, fmt.Errorf("save action admission judgement: %w", err)
	}
	findingsJSON, err := json.Marshal(findings)
	if err != nil {
		return ports.ActionAdmissionSaveOutcomeInvalid, fmt.Errorf("save action admission judgement: %w", err)
	}
	var onward *string
	if node, ok := intent.Onward(); ok {
		value := node.String()
		onward = &value
	}

	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ActionAdmissionSaveOutcomeInvalid, fmt.Errorf("save action admission judgement: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.action_admission_judgement
			(tenant_id, judgement_id, object_ref, intent_kind, node_ref, onward_node_ref,
			 action_at, cross_zone, answer, zones, findings, judged_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT DO NOTHING`,
		tenant.String(), judgementID, judgement.Object().String(), intent.Kind().String(),
		intent.Node().String(), onward, judgement.At().UTC(), judgement.CrossZone().String(),
		judgement.Answer().String(), zonesJSON, findingsJSON, judgement.JudgedAt().UTC(),
	)
	if err != nil {
		return ports.ActionAdmissionSaveOutcomeInvalid, fmt.Errorf("save action admission judgement: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ActionAdmissionAlreadyRecorded, nil
	}
	return ports.ActionAdmissionSaved, nil
}

func (store *ActionAdmissionJudgements) FindActionAdmissionJudgedAt(
	ctx context.Context,
	tenant domain.TenantID,
	judgementID string,
) (time.Time, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("find action admission judgement: %w", err)
	}
	var judgedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT judged_at FROM customs_compliance.action_admission_judgement
		  WHERE tenant_id = $1 AND judgement_id = $2`,
		tenant.String(), judgementID,
	).Scan(&judgedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("find action admission judgement: %w", err)
	}
	return judgedAt.UTC(), true, nil
}
