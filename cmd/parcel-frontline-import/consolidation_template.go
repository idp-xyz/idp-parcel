package main

import (
	"errors"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// consolidationTemplateVersion 是集运汇总模板的版本号。它进两处来源标记（见
// consolidationSourceID 与 consolidationEvidence），换模板列就必须换号——理由同收寄口。
const consolidationTemplateVersion = "CONSOLIDATION-1"

// 集运模板独有的列；与收寄共有的那几列在 template.go。
//
// performedBy 与 operator 分立：前者是现场动手装箱封签的人（domain.PerformingPartyReference，
// UC-NO-003 结果契约要保存的`执行方`），后者是把纸单录进模板的内勤（记录方，落证据引用）。
// 同一次集运里两者可以不是同一个人，合成一列就再也分不开——与收寄口「来源身份与录入者
// 无关」是同一条理由的另一面。
const (
	columnPerformedBy = "performedBy"
	columnAction      = "action"
	columnUnitRef     = "unitRef"
	columnAssetRef    = "assetRef"
	columnMemberUnit  = "memberUnit"
	columnSealRef     = "sealRef"
	columnBasisRef    = "basisRef"
)

var consolidationColumns = []string{
	columnTemplateVersion, columnBatchRef, columnFactRef, columnOperator, columnPerformedBy,
	columnAction, columnUnitRef, columnAssetRef, columnMemberUnit, columnSealRef, columnBasisRef,
	columnEvidenceRef, columnOccurredAt,
}

// actionSpecificColumns 是只对部分动作有意义的四列；每种动作用哪几列见 actionShapes，
// 其余在该动作的行上必须留空。
var actionSpecificColumns = []string{columnAssetRef, columnMemberUnit, columnSealRef, columnBasisRef}

// ErrConsolidationTemplate 是集运模板整体不合格的根错误：形状错、批次不一致、行事实号
// 重复、任一行译装不出。
var ErrConsolidationTemplate = errors.New("parcel-frontline-import: 集运模板不合格")

var consolidationShape = templateShape{
	version: consolidationTemplateVersion,
	columns: consolidationColumns,
	invalid: ErrConsolidationTemplate,
}

// consolidationActions 是动作列的词表。直接用领域 ConsolidationActionKind 的字面名而不另造
// 一套模板词：来源事实登记的 action 列写的就是这几个名，模板、报告与登记三处同词，取证时
// 不必再对一张译表。
var consolidationActions = map[string]domain.ConsolidationActionKind{
	domain.OpenUnitAction.String():     domain.OpenUnitAction,
	domain.AddMemberAction.String():    domain.AddMemberAction,
	domain.RemoveMemberAction.String(): domain.RemoveMemberAction,
	domain.SealUnitAction.String():     domain.SealUnitAction,
	domain.UnsealUnitAction.String():   domain.UnsealUnitAction,
	domain.CloseUnitAction.String():    domain.CloseUnitAction,
}

// actionShape 是一种动作在四个专属列上的必填与可选；不在两者之内的专属列即禁填。
type actionShape struct {
	required []string
	optional []string
}

// actionShapes 逐动作列出专属列的去向，是模板说明「六种动作各自填什么」那张表的机器侧。
// 关闭格的 basisRef 可选：它是剩余成员的处置转移依据，成员已全部移出时本就没有东西要处置；
// 「有成员却无依据」不在模板层判——那要知道单元此刻的成员，是领域的事
// （ErrMembersStillContained），本口译成`被拒`。
var actionShapes = map[domain.ConsolidationActionKind]actionShape{
	domain.OpenUnitAction:     {required: []string{columnAssetRef}},
	domain.AddMemberAction:    {required: []string{columnMemberUnit}},
	domain.RemoveMemberAction: {required: []string{columnMemberUnit}},
	domain.SealUnitAction:     {required: []string{columnSealRef, columnBasisRef}},
	domain.UnsealUnitAction:   {required: []string{columnBasisRef}},
	domain.CloseUnitAction:    {optional: []string{columnBasisRef}},
}

// consolidationActionVocabulary 按领域枚举顺序列出词表，给报错用。
func consolidationActionVocabulary() string {
	names := make([]string, 0, len(consolidationActions))
	for kind := domain.OpenUnitAction; kind <= domain.CloseUnitAction; kind++ {
		names = append(names, kind.String())
	}
	return strings.Join(names, " / ")
}

// columnOwners 列出哪几种动作用得着某一列，给禁填的报错指路——内勤要分清的是抄错了动作
// 还是抄串了列。
func columnOwners(column string) string {
	var owners []string
	for kind := domain.OpenUnitAction; kind <= domain.CloseUnitAction; kind++ {
		shape := actionShapes[kind]
		for _, name := range append(append([]string(nil), shape.required...), shape.optional...) {
			if name == column {
				owners = append(owners, kind.String())
				break
			}
		}
	}
	return strings.Join(owners, " / ")
}

// consolidationRow 是一行已译装好的集运事实：模板行号（给内勤对表用）、行事实号、录入者、
// 动作，以及交编排的命令。
//
// Command 是六个命令类型之一（application.OpenUnitCommand … CloseUnitCommand），由 Action 决定，
// submit 按类型分派。不摊成一个带可选字段的大结构——应用层把六口分成六个类型正是为了让
// 「这一格该不该填」由类型说话，本口的行不该把它们重新压回一个；模板那一层的必填与禁填
// 已在 consolidationRowFrom 核清，命令里因此不会出现「这一格不该有却有」的值。
type consolidationRow struct {
	Line     int
	FactRef  string
	Operator string
	Action   domain.ConsolidationActionKind
	Command  any
}

// consolidationBatch 是一份译装完成的集运模板：批次号只有一个，行按模板顺序。
//
// 顺序就是作业顺序：一行一笔事务按模板顺序推进，开启在前、移入在后、封装再后，与现场
// 发生的先后一致。本口不按 occurredAt 重排——内勤抄的时间可能只到分钟，同一分钟里两行的
// 先后只有纸单上的顺序说得清。
type consolidationBatch struct {
	BatchRef string
	Rows     []consolidationRow
}

// decodeConsolidationTemplate 把 CSV 模板译装成逐行命令。零默认：缺格拒、词表外拒、时间
// 解析不出拒；`tenant` 来自 CLI 参数而不是模板（ADR-0003，同收寄口）。
func decodeConsolidationTemplate(tenant domain.TenantID, raw []byte) (consolidationBatch, error) {
	batch := consolidationBatch{}
	batchRef, err := decodeTemplate(consolidationShape, raw, func(record templateRecord) error {
		row, err := consolidationRowFrom(tenant, record.batchRef, record.factRef, record.line, record.cell)
		if err != nil {
			return err
		}
		batch.Rows = append(batch.Rows, row)
		return nil
	})
	if err != nil {
		return consolidationBatch{}, err
	}
	batch.BatchRef = batchRef
	return batch, nil
}

// consolidationRowFrom 译装一行。六种动作各自的必填与禁填在这里核清；禁填是拒不是忽略——
// 给了值说明这一行抄错了动作，静默丢掉会让内勤以为登进去了。
//
// 来源四格（domain.WorkFactSource）每行都必备：集运六口把来源身份兼作幂等键，缺任一格
// 编排答`未受理`，而那明明是模板填错（用法错误），在这里拦。
func consolidationRowFrom(
	tenant domain.TenantID,
	batchRef, factRef string,
	line int,
	cell func(string) string,
) (consolidationRow, error) {
	operator := cell(columnOperator)
	if operator == "" {
		return consolidationRow{}, fmt.Errorf("%s 为空", columnOperator)
	}
	performedBy, err := domain.NewPerformingPartyReference(cell(columnPerformedBy))
	if err != nil {
		return consolidationRow{}, fmt.Errorf("%s 为空——集运作业事实必须记执行方（UC-NO-003 结果契约）", columnPerformedBy)
	}
	unit, err := domain.NewConsolidationUnitID(cell(columnUnitRef))
	if err != nil {
		return consolidationRow{}, fmt.Errorf("%s 为空", columnUnitRef)
	}
	evidenceRef := cell(columnEvidenceRef)
	if evidenceRef == "" {
		return consolidationRow{}, fmt.Errorf("%s 为空——集运作业事实必须带证据（UC-NO-003 结果契约）", columnEvidenceRef)
	}
	occurredAt, err := parseOccurredAt(cell)
	if err != nil {
		return consolidationRow{}, err
	}
	evidence, err := domain.NewExecutionEvidenceReference(consolidationEvidence(operator, evidenceRef))
	if err != nil {
		return consolidationRow{}, err
	}
	source, err := domain.NewWorkFactSource(consolidationSourceID(batchRef, factRef), performedBy, evidence, occurredAt)
	if err != nil {
		return consolidationRow{}, err
	}

	actionName := cell(columnAction)
	action, known := consolidationActions[actionName]
	if !known {
		return consolidationRow{}, fmt.Errorf("%s=%q 不在词表 %s 内", columnAction, actionName, consolidationActionVocabulary())
	}
	if err := checkActionColumns(action, cell); err != nil {
		return consolidationRow{}, err
	}

	row := consolidationRow{Line: line, FactRef: factRef, Operator: operator, Action: action}
	switch action {
	case domain.OpenUnitAction:
		asset, err := domain.NewCarrierAssetReference(cell(columnAssetRef))
		if err != nil {
			return consolidationRow{}, err
		}
		row.Command = application.OpenUnitCommand{TenantID: tenant, Unit: unit, Asset: asset, Source: source}
	case domain.AddMemberAction, domain.RemoveMemberAction:
		member, err := domain.NewHandlingUnitID(cell(columnMemberUnit))
		if err != nil {
			return consolidationRow{}, err
		}
		if action == domain.AddMemberAction {
			row.Command = application.AddMemberCommand{TenantID: tenant, Unit: unit, Member: member, Source: source}
		} else {
			row.Command = application.RemoveMemberCommand{TenantID: tenant, Unit: unit, Member: member, Source: source}
		}
	case domain.SealUnitAction:
		seal, err := domain.NewSealReference(cell(columnSealRef))
		if err != nil {
			return consolidationRow{}, err
		}
		basis, err := domain.NewWorkBasisReference(cell(columnBasisRef))
		if err != nil {
			return consolidationRow{}, err
		}
		row.Command = application.SealUnitCommand{TenantID: tenant, Unit: unit, Seal: seal, Basis: basis, Source: source}
	case domain.UnsealUnitAction:
		basis, err := domain.NewWorkBasisReference(cell(columnBasisRef))
		if err != nil {
			return consolidationRow{}, err
		}
		row.Command = application.UnsealUnitCommand{TenantID: tenant, Unit: unit, Basis: basis, Source: source}
	case domain.CloseUnitAction:
		command := application.CloseUnitCommand{TenantID: tenant, Unit: unit, Source: source}
		if basisRef := cell(columnBasisRef); basisRef != "" {
			disposition, err := domain.NewWorkBasisReference(basisRef)
			if err != nil {
				return consolidationRow{}, err
			}
			command.Disposition = disposition
		}
		row.Command = command
	}
	return row, nil
}

// checkActionColumns 按 actionShapes 核一行的专属列：必填为空拒，禁填有值拒。
func checkActionColumns(action domain.ConsolidationActionKind, cell func(string) string) error {
	shape := actionShapes[action]
	allowed := map[string]bool{}
	for _, column := range shape.required {
		allowed[column] = true
		if cell(column) == "" {
			return fmt.Errorf("%s 行 %s 为空", action, column)
		}
	}
	for _, column := range shape.optional {
		allowed[column] = true
	}
	for _, column := range actionSpecificColumns {
		if !allowed[column] && cell(column) != "" {
			return fmt.Errorf("%s 行不得带 %s（这一格只在 %s 行有意义），请留空", action, column, columnOwners(column))
		}
	}
	return nil
}

// consolidationSourceID 铸导入事实的来源身份 FTI/CONSOLIDATION-1/<批次>/<行事实号>。与录入者
// 无关，理由同 intakeSourceID；集运六口把它兼作幂等键，同一行事实号重录即重放。
func consolidationSourceID(batchRef, factRef string) string {
	return importMarker(consolidationTemplateVersion, batchRef, factRef)
}

// consolidationEvidence 铸作业事实的证据引用 FTI/CONSOLIDATION-1/<录入者>/<纸面证据号>。
// domain.ExecutionEvidenceReference 要的是「一次执行事实的证据」，过渡期就是这一行模板与它
// 抄自的那张作业单；录入者是记录方，落在这里而不落在来源身份里。
func consolidationEvidence(operator, evidenceRef string) string {
	return importMarker(consolidationTemplateVersion, operator, evidenceRef)
}
