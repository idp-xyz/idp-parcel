package main

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"go.idp.xyz/idp-bento-go/eventing"

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
	sourceID := intakeSourceID(batchRef, factRef)
	if err := intakeSourceIDCarriable(batchRef, factRef, sourceID); err != nil {
		return intakeRow{}, err
	}
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

	mark := cell(columnMark)
	if err := intakeMarkStorable(mark); err != nil {
		return intakeRow{}, err
	}
	command := application.ReceiveDeliveredUnitCommand{
		TenantID:    tenant,
		SourceID:    sourceID,
		Node:        node,
		DeliveredBy: deliveredBy,
		Unit:        unit,
		Mark:        ports.ExternalMarkObservation{Mark: mark},
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

// intakeSourceIDCarriable 核铸出的来源标识进不进得了收寄意图信封。来源标识原样做信封的 Subject（NO 的
// OutboxNodeIntakeHandoff），越过框架的 eventing.MaxSubjectLength 或带控制字符，信封就被确定性拒收。导入路的编排把入队
// 失败吞成续办、收寄照常提交，报告让内勤重跑同一文件——可这一行重跑多少次都撞同一道拒收，续办永远续不上。所以在译装时
// 就按坏行整批拒，让内勤改批次号或行事实号；不动编排吞错那一半，导入路仍要续办。设备收寄口（nodeopshttp 的
// receptionCommand）按同一个上限拒成坏报文，两口的数都取框架常量，不另写。
//
// 判的是铸出的整条标识而不是两格各自的长度：上限压在拼好的那一串上，标记前缀与模板版本也占字节，单看两格判不准。
func intakeSourceIDCarriable(batchRef, factRef, sourceID string) error {
	if len(sourceID) > eventing.MaxSubjectLength {
		return fmt.Errorf("%s 与 %s 合起来太长：拼出的来源标识 %d 字节，收寄意图最多带 %d 字节（%s %d 字节，%s %d 字节）——请缩短批次号或行事实号",
			columnBatchRef, columnFactRef, len(sourceID), eventing.MaxSubjectLength,
			columnBatchRef, len(batchRef), columnFactRef, len(factRef))
	}
	if strings.IndexFunc(sourceID, unicode.IsControl) < 0 {
		return nil
	}
	// 标识里其余各段是本口自己的常量，控制字符只能来自这两格；点名是哪一格，内勤才知道去改哪一列。
	column, value := columnFactRef, factRef
	if strings.IndexFunc(batchRef, unicode.IsControl) >= 0 {
		column, value = columnBatchRef, batchRef
	}
	return fmt.Errorf("%s=%q 带控制字符（单元格内换行、Tab 之类），拼出的来源标识交不出收寄意图——请删掉批次号或行事实号里的不可见字符",
		column, value)
}

// intakeMarkStorable 核观察号码那一格存不存得下。收寄记录原样保全这一格（ADR-0199 决定一），身份核对也拿它原样问 PS 的
// 关联册；Postgres 的 text 与 jsonb 都存不下 NUL，放进编排那一行只会回滚记未决，重跑同一文件多少次都一样。所以在译装时按
// 坏行整批拒，让内勤核对纸单重录这一格。设备收寄口（nodeopshttp 的 receptionCommand）对同一格答坏报文，两口同拒 NUL、
// 只拒 NUL。
func intakeMarkStorable(mark string) error {
	if !strings.ContainsRune(mark, 0) {
		return nil
	}
	return fmt.Errorf("%s=%q 带 NUL 字符，收寄记录存不下这个号——请核对纸单，删掉标识里的不可见字符后重录", columnMark, mark)
}

// intakeEvidence 铸明确接收的证据引用。`ReceptionEvidenceReference` 点名的「受控接收
// 记录」在过渡期就是这一行模板：谁录的、抄自哪张纸面证据，都在引用里。
func intakeEvidence(operator, evidenceRef string) string {
	return importMarker(intakeTemplateVersion, operator, evidenceRef)
}
