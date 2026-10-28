// 清关区（customs-compliance 的合规规则库、案件配置册、门禁条件册、口岸与申报路径册）页面口径：册名、登记面的标题与
// 填写提示、错误码的人话。状态与枚举码的中文与色调一律在 src/vocabulary/status.ts（界面用语词典，一处定义），页面经
// labelFor / StatusPill 取，本文件不再另存一份。

import type {
  CaseRegisterRegistry,
  ComplianceRegistry,
  CustomsRegistrationKind,
  DutyRegistrationKind,
  PortsPathsRegistry,
} from './api';

export const registryLabels: Record<ComplianceRegistry, string> = {
  'case-requirement': '何时要立清关单',
  interpretation: '海关回执怎么读',
};

export const caseRegisterLabels: Record<CaseRegisterRegistry, string> = {
  readiness: '申报就绪判断',
  'submission-authority': '提交授权',
  'closure-obligation': '关闭核对清单',
};

export const portsPathsRegistryLabels: Record<PortsPathsRegistry, string> = {
  'candidate-port': '可用口岸',
  'declaration-path': '申报路径',
};

export function labelOf(table: Record<string, string>, code: string): string {
  return table[code] ?? code;
}

// 错误码说人话：说哪里错、怎么改，不说实现细节。
export const problemCodeNotes: Record<string, string> = {
  METHOD_NOT_ALLOWED: '这个入口不接受这种提交方式，请刷新页面后重试；仍然不行请联系系统管理员。',
  MALFORMED_REQUEST: '填写的内容格式不对，系统读不出一笔完整的登记；请按提示检查各项后再提交。',
  INTAKE_FAILED: '登录信息没能核对通过，这次没有登记任何内容；请稍后重试。',
  NO_ANSWER_FORMED: '系统暂时没能处理这笔登记，是否登记成功还不确定；请稍后刷新查看，必要时重新提交同一份。',
  OUTSIDE_ADMISSION_SCOPE: '当前阶段还没有放开这项登记；什么都没有保存，请联系负责阶段管理的同事。',
  IDENTITY_DEPENDENCY_UNAVAILABLE: '暂时无法核对登录身份，请稍后重试。',
};

export function problemNote(code: string): string {
  return problemCodeNotes[code] ?? `这次提交没有成功（错误码 ${code}）；请记下关联标识联系系统管理员。`;
}

// ——以下为登记签的页面口径（ADR-0085）。标题说这个任务做什么，不叫「登记 ×× 版本」。

export const registrationTitles: Record<CustomsRegistrationKind, string> = {
  'interpretation-rule': '新增海关回执读法',
  'case-requirement': '设定何时要立清关单',
  'gate-catalog': '新增放行检查项目',
  'candidate-port': '新增可用口岸',
  'declaration-path': '新增申报路径',
  'regulatory-credential': '登记监管凭证',
};

export const dutyRegistrationTitles: Record<DutyRegistrationKind, string> = {
  'duty-collaboration': '登记税费付款事项',
  'duty-payment-verification': '登记税费付款核对',
};

// 填写提示：只说填什么、填错了会怎样。形状与接口、受控 CLI 吃的同一份，这一句留给对接的人。
function hint(kind: string, fields: string): string {
  return `${fields} 内容与接口、命令行工具（${kind}）吃的同一份；所属公司由系统记下，不用填。`;
}

export const registrationSnapshotHints: Record<CustomsRegistrationKind, string> = {
  'interpretation-rule': hint(
    'interpretation-rule',
    '请填 tenantId、resultLayer（海关结果层：REGULATORY_RECEIPT 海关已接收 / BUSINESS_ACCEPTANCE 海关已受理 / PROCESS_DECISION 过程决定 / ' +
      'ASSESSED_DUTY 核定税费 / RELEASE_RESULT 放行结果 / DISPOSITION_DECISION 处置决定）、jurisdictionRef（辖区）、ruleRef（读法引用）、appliesFrom（从何时起用）。' +
      '不填截止时间：要换读法就登一条更晚起用的，旧的自动到那时为止。',
  ),
  'case-requirement': hint(
    'case-requirement',
    '请填 tenantId、jurisdictionRef（辖区）、direction（IMPORT 进口 / EXPORT 出口）、procedureRef（监管程序）、required（true 要立 / false 不用立，必填）、basis（依据，必填）。' +
      '同一个辖区、方向、程序只能有一条；要改就先核对已有的那条。',
  ),
  'gate-catalog': hint(
    'gate-catalog',
    '请填 tenantId、scopeRef（申报范围）、action（检查哪一步：OUTBOUND_RELEASE 出库 / LOADING_DEPARTURE 装载出发 / CROSS_CUSTOMS_MOVEMENT 跨关区移动 / FINAL_DELIVERY 交付）、' +
      'boundaryRef（监管边界）、registeredAt。这里登的是「这一步要做放行检查」；每项检查的结论另行登记。',
  ),
  'candidate-port': hint('candidate-port', '请填 tenantId、portRef（口岸）、appliesFrom（从何时起可用）。不填截止时间：要停用就登一条更晚起用的新版本。'),
  'declaration-path': hint(
    'declaration-path',
    '请填 tenantId、pathRef（路径）、portRef（经哪个口岸）、direction（IMPORT 进口 / EXPORT 出口）、declarationMode（申报模式）、appliesFrom（从何时起用）。',
  ),
  'regulatory-credential':
    '请照凭证原件填 credentialId（凭证号）、issuerRef（签发机构）、holderRef（持有人）、procedureRef（适用程序）、validFrom / validTo（有效期两端）、uses（次数额度，原件没写就不填）。' +
    '一张凭证一条记录；换期限或额度就是另一张凭证，请另行登记。登记人由系统记下。',
};

export const dutyRegistrationSnapshotHints: Record<DutyRegistrationKind, string> = {
  'duty-collaboration': hint(
    'duty-collaboration',
    '请填 tenantId、kind（ASSESSED_DUTY 按海关核定税费付，带 dutyRef；或 EXPLICITLY_NOT_REQUIRED 明确无需付款，带 noPayBasis）、scopeRef（申报范围）、' +
      'obligorRef（法定义务人）、requirementRef（付款要求来源）、targetRef（交接给谁）。' +
      '海关还没核定、也没有无需付款依据时，系统会答「还在等税费结果」，等结果到了重新提交同一份即可。',
  ),
  'duty-payment-verification': hint(
    'duty-payment-verification',
    '请填 tenantId、dutyRef（税费版本）、fundsRef 与 fundsVersion（对的是哪一笔收付款的哪一版）、scopeRef（申报范围）、procedureRef（按哪个程序的规则判）、' +
      'coverage（NONE 无覆盖 / PARTIAL 部分 / COVERED 已覆盖）、delta（NO_DELTA 无差额 / SHORT 不足 / EXCESS 超额 / PENDING 待确认）、' +
      'validity（VALID 有效 / INVALIDATED 失效 / CONFLICTING 冲突 / PENDING 待确认）、basis（凭什么把这笔钱对到这笔税费）。' +
      '三项结论请分别填，系统不会按金额相等替你判；收付款或付款事项还没登记时系统会说明，登好后重新提交同一份。',
  ),
};
