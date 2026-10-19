package main

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"go.idp.xyz/idp-bento-go/eventing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 收寄汇总模板的版本号。它进两处来源标记（见 intakeSourceID 与 intakeEvidence），因此换模板列就必须换号——同一个号下两种列形状会让
// 落库的标记说谎。INTAKE-2 比 INTAKE-1 多交付范围依据引用一列（ADR-0212 决定七）；INTAKE-1 照旧认，读作没有依据：已经按它填好的
// 纸单汇总不必重抄，重跑同一份文件也还是同一组来源标记。
const (
	intakeTemplateVersion             = "INTAKE-2"
	intakeTemplateVersionWithoutBasis = "INTAKE-1"
)

// 收寄模板独有的列；与集运共有的那几列在 template.go。
const (
	columnNode               = "node"
	columnDeliveredBy        = "deliveredBy"
	columnHandlingUnit       = "handlingUnit"
	columnMark               = "mark"
	columnClaim              = "claim"
	columnRefusalReason      = "refusalReason"
	columnDeliveryScopeBasis = "deliveryScopeBasis"
)

var intakeColumnsWithoutBasis = []string{
	columnTemplateVersion, columnBatchRef, columnFactRef, columnOperator,
	columnNode, columnDeliveredBy, columnHandlingUnit, columnMark,
	columnClaim, columnEvidenceRef, columnRefusalReason, columnOccurredAt,
}

var intakeColumns = append(append([]string{}, intakeColumnsWithoutBasis...), columnDeliveryScopeBasis)

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

var (
	intakeShape = templateShape{
		version: intakeTemplateVersion,
		columns: intakeColumns,
		invalid: ErrIntakeTemplate,
	}
	intakeShapeWithoutBasis = templateShape{
		version: intakeTemplateVersionWithoutBasis,
		columns: intakeColumnsWithoutBasis,
		invalid: ErrIntakeTemplate,
	}
)

// intakeShapeOf 按表头认模板版本：表头有依据那一列就是 INTAKE-2，否则按 INTAKE-1 核。只看表头不看行：骨架随后逐行核版本号，行上的号
// 与表头的列形状对不上照样整批拒。表头读不出来时交 INTAKE-2，由骨架报出读不到表头的原错。
func intakeShapeOf(raw []byte) templateShape {
	header, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf")))).Read()
	if err != nil {
		return intakeShape
	}
	for _, name := range header {
		if strings.TrimSpace(name) == columnDeliveryScopeBasis {
			return intakeShape
		}
	}
	return intakeShapeWithoutBasis
}

// intakeRow 是一行已译装好的收寄事实：模板行号（给内勤对表用）、行事实号与交编排的
// 命令。命令里的来源身份与证据引用已带导入标记。
type intakeRow struct {
	Line     int
	FactRef  string
	Operator string
	Command  application.ReceiveDeliveredUnitCommand
}

// intakeBatch 是一份译装完成的收寄模板：批次号只有一个，行按模板顺序，Version 是这份文件的模板版本。
type intakeBatch struct {
	BatchRef string
	Version  string
	Rows     []intakeRow
}

// decodeIntakeTemplate 把 CSV 模板译装成逐行命令。零默认：缺格拒、词表外拒、时间解析
// 不出拒；`tenant` 来自 CLI 参数而不是模板——租户是最高数据隔离边界（ADR-0003），由受控
// 运维给出，不让内勤填也不让文件自报。
func decodeIntakeTemplate(tenant domain.TenantID, raw []byte) (intakeBatch, error) {
	shape := intakeShapeOf(raw)
	batch := intakeBatch{Version: shape.version}
	batchRef, err := decodeTemplate(shape, raw, func(record templateRecord) error {
		row, err := intakeRowFrom(tenant, shape.version, record.batchRef, record.factRef, record.line, record.cell)
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
	version, batchRef, factRef string,
	line int,
	cell func(string) string,
) (intakeRow, error) {
	sourceID := intakeSourceID(version, batchRef, factRef)
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
	if version == intakeTemplateVersion {
		if command.ScopeBasis, err = intakeScopeBasis(cell(columnDeliveryScopeBasis)); err != nil {
			return intakeRow{}, err
		}
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
		evidence, err := domain.NewReceptionEvidenceReference(intakeEvidence(version, operator, evidenceRef))
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
func intakeSourceID(version, batchRef, factRef string) string {
	return importMarker(version, batchRef, factRef)
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
func intakeEvidence(version, operator, evidenceRef string) string {
	return importMarker(version, operator, evidenceRef)
}

// intakeScopeBasis 译依据那一格（ADR-0212 决定七）：「种类:标识」，种类取 DELIVERY_PRE_ADVICE / DELIVERY_HANDOVER_NOTE，标识是冒号之后的
// 全部（交接单标识由设备签发，可能自带冒号）。空格即没有依据。给了就要给全：缺种类、种类认不出、标识空都整批拒——静默丢掉，内勤以为
// 依据登进去了，这一行却按没带依据落范围未证。整格已由骨架去过两端空白，标识本身不再去。
func intakeScopeBasis(raw string) (domain.DeliveryScopeBasisReference, error) {
	if raw == "" {
		return domain.DeliveryScopeBasisReference{}, nil
	}
	kindName, identifier, found := strings.Cut(raw, ":")
	if !found {
		return domain.DeliveryScopeBasisReference{}, fmt.Errorf("%s=%q 要写成「种类:标识」，种类是 %s 或 %s",
			columnDeliveryScopeBasis, raw, domain.BasisDeliveryPreAdvice, domain.BasisDeliveryHandoverNote)
	}
	kind, err := domain.ParseDeliveryScopeBasisKind(kindName)
	if err != nil {
		return domain.DeliveryScopeBasisReference{}, fmt.Errorf("%s=%q 的种类不在词表 %s / %s 内",
			columnDeliveryScopeBasis, raw, domain.BasisDeliveryPreAdvice, domain.BasisDeliveryHandoverNote)
	}
	basis, err := domain.NewDeliveryScopeBasisReference(kind, identifier)
	if err != nil {
		return domain.DeliveryScopeBasisReference{}, fmt.Errorf("%s=%q 的标识为空", columnDeliveryScopeBasis, raw)
	}
	return basis, nil
}
