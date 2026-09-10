// 凭证 / 税费付款协作 / 税费付款核对三册读签的行判读(票 sa-cc/10)。抽成纯函数是为了让
// 「哪一格不得被译顺」能在 run-tests 里钉住(判据同 pricing/coverage-rows.ts):页面组件只管
// 取数与呈现,判读全在这里。

import type { CredentialRecord, DutyCollaborationRecord, DutyVerificationRecord } from './api';
import { formatInstant, formatRange } from '../catalogue-view';
import {
  dutyCoverageLabels,
  dutyDeltaLabels,
  dutyFactValidityLabels,
  dutyObligationKindLabels,
  labelOf,
} from './presentation';

export interface RegisterRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

/**
 * 次数额度一格的说法。缺席是「来源未提供」(query_credentials.go 把领域 Uses 的第二个返回值
 * 落成缺席 / 在场);0 在领域就约定为未提供,传输层从不送它——一次不合契约把 0 送过来,
 * 这里同样答未提供,不显成「0 次」:那是页面凭空造出的「额度已用尽」,而余额根本不是本册
 * 登记内容。
 */
export function credentialUsesLabel(uses?: number): string {
  if (uses === undefined || uses === 0) return '来源未提供';
  return `${uses} 次`;
}

// 有效期两端合成一段区间、登记时间另列:三个时刻是三件事(0014 自注),混进一列读者就分不出
// 「何时登进来」与「何时有效」。
export function credentialRows(records: CredentialRecord[]): RegisterRow[] {
  return records.map((record) => ({
    // 行键循库主键 (tenant, credential_id):一身份一版。
    key: `credential:${record.credential}`,
    values: {
      credential: record.credential,
      issuer: record.issuer,
      holder: record.holder,
      procedure: record.procedure,
      validity: formatRange(record.validFrom, record.validTo),
      uses: credentialUsesLabel(record.uses),
      registeredAt: formatInstant(record.registeredAt),
    },
  }));
}

// 义务依据一列按格取字段:核定税费格显税费引用,明确无需付款格显无需付款依据。**不借字段**
// ——本格缺了就显缺席,不拿另一格的顶上:两格同在或同缺都立不起领域对象,服务端读口本该已
// 抛,真到了这里就是一行不合契约的响应,让它露出来。
function collaborationBasis(record: DutyCollaborationRecord): string {
  if (record.kind === 'ASSESSED_DUTY') return record.duty ?? '—';
  if (record.kind === 'EXPLICITLY_NOT_REQUIRED') return record.noPayBasis ?? '—';
  return record.duty ?? record.noPayBasis ?? '—';
}

export function collaborationRows(records: DutyCollaborationRecord[]): RegisterRow[] {
  return records.map((record) => ({
    // 行键循库主键 (tenant, scope_ref, duty_ref):无需付款格的税费引用在键上是空串(0016 自注)。
    key: `collaboration:${record.scope}:${record.duty ?? ''}`,
    values: {
      scope: record.scope,
      kind: labelOf(dutyObligationKindLabels, record.kind),
      basis: collaborationBasis(record),
      obligor: record.obligor,
      requirement: record.requirement,
      target: record.target,
      formedAt: formatInstant(record.formedAt),
    },
  }));
}

// 三轴三列各译各的词表,行上没有任何合成列(ADR-0137 决定三):在这里折一次,页面就成了第二
// 处判断权威,而放行门禁那一道怎么读三态是按监管程序登记进来的规则。labelOf 对集外取值原样
// 回显,坏数据露出来而不是被译顺。
export function verificationRows(records: DutyVerificationRecord[]): RegisterRow[] {
  return records.map((record) => ({
    // 行键循库主键 (tenant, duty_ref, funds_ref, scope_ref, version_digest):同三维键的多版本
    // 各自成行,不折「当前版」。
    key: `verification:${record.duty}:${record.funds}:${record.scope}:${record.version}`,
    values: {
      duty: record.duty,
      funds: record.funds,
      scope: record.scope,
      version: record.version,
      coverage: labelOf(dutyCoverageLabels, record.coverage),
      delta: labelOf(dutyDeltaLabels, record.delta),
      validity: labelOf(dutyFactValidityLabels, record.validity),
      basis: record.basis,
      verifiedAt: formatInstant(record.verifiedAt),
    },
  }));
}
