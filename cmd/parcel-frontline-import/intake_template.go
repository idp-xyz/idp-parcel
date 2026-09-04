package main

import (
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// intakeTemplateVersion 是收寄汇总模板的版本号。它进两处来源标记（见 intakeSourceID 与
// intakeEvidence），因此换模板列就必须换号——同一个号下两种列形状会让落库的标记说谎。
const intakeTemplateVersion = "INTAKE-1"

// 收寄模板独有的列；与集运共有的那几列在 template.go。
const (
	columnNode          = "node"
	columnDeliveredBy   = "deliveredBy"
	columnHandlingUnit  = "handlingUnit"
	columnMark          = "mark"
	columnClaim         = "claim"
	columnRefusalReason = "refusalReason"
)

var intakeColumns = []string{
	columnTemplateVersion, columnBatchRef, columnFactRef, columnOperator,
	columnNode, columnDeliveredBy, columnHandlingUnit, columnMark,
	columnClaim, columnEvidenceRef, columnRefusalReason, columnOccurredAt,
}

// 接收观察三态的模板词表。词表在这里就核——走到编排才被拒会答成「未知接收观察」的
// 未决，而它明明是模板填错（用法错误）。
const (
	claimReceived = "RECEIVED"
	claimRefused  = "REFUSED"
	claimScanOnly = "SCAN_ONLY"
)

// ErrIntakeTemplate 是收寄模板整体不合格的根错误：形状错、批次不一致、行事实号重复、
// 任一行译装不出。
var ErrIntakeTemplate = errors.New("parcel-frontline-import: 收寄模板不合格")

var intakeShape = templateShape{
	version: intakeTemplateVersion,
	columns: intakeColumns,
	invalid: ErrIntakeTemplate,
}

// intakeRow 是一行已译装好的收寄事实：模板行号（给内勤对表用）、行事实号与交编排的
// 命令。命令里的来源身份与证据引用已带导入标记。
type intakeRow struct {
	Line     int
	FactRef  string
	Operator string
	Command  application.ReceiveDeliveredUnitCommand
}

// intakeBatch 是一份译装完成的收寄模板：批次号只有一个，行按模板顺序。
type intakeBatch struct {
	BatchRef string
	Rows     []intakeRow
}

// decodeIntakeTemplate 把 CSV 模板译装成逐行命令。零默认：缺格拒、词表外拒、时间解析
// 不出拒；`tenant` 来自 CLI 参数而不是模板——租户是最高数据隔离边界（ADR-0003），由受控
// 运维给出，不让内勤填也不让文件自报。
func decodeIntakeTemplate(tenant domain.TenantID, raw []byte) (intakeBatch, error) {
	batch := intakeBatch{}
	batchRef, err := decodeTemplate(intakeShape, raw, func(record templateRecord) error {
		row, err := intakeRowFrom(tenant, record.batchRef, record.factRef, record.line, record.cell)
		if err != nil {
			return err
		}
		batch.Rows = append(batch.Rows, row)
		return nil
	})
	if err != nil {
		return intakeBatch{}, err
	}
	batch.BatchRef = batchRef
	return batch, nil
}

// intakeRowFrom 译装一行。三态各自的必填与禁填在这里核清：`REFUSED`/`SCAN_ONLY` 行
// 给了 evidenceRef 是拒不是忽略——既有记录形状没有地方放它，静默丢掉会让内勤以为
// 证据登进去了。
func intakeRowFrom(
	tenant domain.TenantID,
	batchRef, factRef string,
	line int,
	cell func(string) string,
) (intakeRow, error) {
	operator := cell(columnOperator)
	if operator == "" {
		return intakeRow{}, fmt.Errorf("%s 为空", columnOperator)
	}
	node, err := domain.NewNodeReference(cell(columnNode))
	if err != nil {
		return intakeRow{}, fmt.Errorf("%s 为空", columnNode)
	}
	deliveredBy, err := domain.NewDeliveringPartyReference(cell(columnDeliveredBy))
	if err != nil {
		return intakeRow{}, fmt.Errorf("%s 为空", columnDeliveredBy)
	}
	unit, err := domain.NewHandlingUnitID(cell(columnHandlingUnit))
	if err != nil {
		return intakeRow{}, fmt.Errorf("%s 为空", columnHandlingUnit)
	}
	occurredAt, err := parseOccurredAt(cell)
	if err != nil {
		return intakeRow{}, err
	}

	command := application.ReceiveDeliveredUnitCommand{
		TenantID:    tenant,
		SourceID:    intakeSourceID(batchRef, factRef),
		Node:        node,
		DeliveredBy: deliveredBy,
		Unit:        unit,
		Mark:        ports.ExternalMarkObservation{Mark: cell(columnMark)},
		OccurredAt:  occurredAt,
	}

	evidenceRef := cell(columnEvidenceRef)
	refusalReason := cell(columnRefusalReason)
	switch claim := cell(columnClaim); claim {
	case claimReceived:
		if command.Mark.Mark == "" {
			return intakeRow{}, fmt.Errorf("%s 行 %s 为空——身份核对靠它", claimReceived, columnMark)
		}
		if evidenceRef == "" {
			return intakeRow{}, fmt.Errorf("%s 行 %s 为空——没有明确接收证据就不是节点收寄，请改 %s", claimReceived, columnEvidenceRef, claimScanOnly)
		}
		if refusalReason != "" {
			return intakeRow{}, fmt.Errorf("%s 行不得带 %s", claimReceived, columnRefusalReason)
		}
		evidence, err := domain.NewReceptionEvidenceReference(intakeEvidence(operator, evidenceRef))
		if err != nil {
			return intakeRow{}, err
		}
		command.Claim = application.ExplicitReception
		command.Evidence = evidence
	case claimRefused:
		if refusalReason == "" {
			return intakeRow{}, fmt.Errorf("%s 行 %s 为空", claimRefused, columnRefusalReason)
		}
		if evidenceRef != "" {
			return intakeRow{}, fmt.Errorf("%s 行的 %s 无处可登（既有未收寄记录只留拒收原因），请留空", claimRefused, columnEvidenceRef)
		}
		command.Claim = application.ExplicitRefusal
		command.Refusal = refusalReason
	case claimScanOnly:
		if command.Mark.Mark == "" {
			return intakeRow{}, fmt.Errorf("%s 行 %s 为空——扫描观察的就是标识", claimScanOnly, columnMark)
		}
		if evidenceRef != "" || refusalReason != "" {
			return intakeRow{}, fmt.Errorf("%s 行不得带 %s 或 %s——有接收证据请改 %s", claimScanOnly, columnEvidenceRef, columnRefusalReason, claimReceived)
		}
		command.Claim = application.ScanOnlyObservation
	default:
		return intakeRow{}, fmt.Errorf("%s=%q 不在词表 %s / %s / %s 内", columnClaim, claim, claimReceived, claimRefused, claimScanOnly)
	}

	return intakeRow{Line: line, FactRef: factRef, Operator: operator, Command: command}, nil
}

// intakeSourceID 铸导入事实的来源身份。它与录入者无关：同一张纸单被两个内勤各录一次，
// 是同一条事实的重放而不是两条事实——ADR-0023 把「真实重扫还是重发」的判断交给在场的
// 记录方，纸单流水号就是内勤这一侧的那个判断。
func intakeSourceID(batchRef, factRef string) string {
	return importMarker(intakeTemplateVersion, batchRef, factRef)
}

// intakeEvidence 铸明确接收的证据引用。`ReceptionEvidenceReference` 点名的「受控接收
// 记录」在过渡期就是这一行模板：谁录的、抄自哪张纸面证据，都在引用里。
func intakeEvidence(operator, evidenceRef string) string {
	return importMarker(intakeTemplateVersion, operator, evidenceRef)
}
