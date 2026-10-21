package registrationjson

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// evaluationResumeDocument 是运营续办口的输入（ADR-0229 决定六）：方向、点名一件包裹（SELL）或一版发生项（BUY），以及运营
// 人员给的续办引用（工单号或决定标识）。同一个引用重跑是重放。
type evaluationResumeDocument struct {
	TenantID          string `json:"tenantId"`
	Direction         string `json:"direction"`
	Parcel            string `json:"parcel"`
	Occurrence        string `json:"occurrence"`
	OccurrenceVersion string `json:"occurrenceVersion"`
	ResumeReference   string `json:"resumeReference"`
}

// EvaluationResumeFromJSON 译装一次运营续办。方向与点名对不上（SELL 点了发生项、BUY 没给版本）在这里就拒，不进事务。
func EvaluationResumeFromJSON(raw []byte) (application.OperatorResumeEvaluationCommand, error) {
	none := application.OperatorResumeEvaluationCommand{}
	var document evaluationResumeDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("评价续办输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, fmt.Errorf("tenantId：%w", err)
	}
	direction, err := domain.EvaluationDirectionFromName(document.Direction)
	if err != nil {
		return none, fmt.Errorf("direction（SELL 或 BUY）：%w", err)
	}
	if document.ResumeReference == "" {
		return none, fmt.Errorf("resumeReference：续办引用必填，同一个引用重跑才认得出是重放")
	}
	command := application.OperatorResumeEvaluationCommand{TenantID: tenant, Direction: direction, Reference: document.ResumeReference}
	switch direction {
	case domain.SellEvaluationDirection:
		if document.Occurrence != "" || document.OccurrenceVersion != "" {
			return none, fmt.Errorf("SELL 续办点名包裹，不收 occurrence")
		}
		if command.Parcel, err = domain.NewParcelReference(document.Parcel); err != nil {
			return none, fmt.Errorf("parcel：%w", err)
		}
	default:
		if document.Parcel != "" {
			return none, fmt.Errorf("BUY 续办点名发生项，不收 parcel")
		}
		if command.Occurrence, err = domain.NewChargeOccurrenceID(document.Occurrence); err != nil {
			return none, fmt.Errorf("occurrence：%w", err)
		}
		if command.OccurrenceVersion, err = domain.NewOccurrenceVersion(document.OccurrenceVersion); err != nil {
			return none, fmt.Errorf("occurrenceVersion：%w", err)
		}
	}
	return command, nil
}
