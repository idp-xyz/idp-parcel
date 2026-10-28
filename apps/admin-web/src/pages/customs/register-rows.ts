// 凭证 / 税费付款事项 / 税费付款核对三册读签的行判读。抽成纯函数是为了让「哪一格不得被译顺」能在 run-tests 里钉住：
// 页面组件只管取数与呈现，判读全在这里。状态码的中文经 vocabulary/status.ts 的 labelFor 取。

import type { CredentialRecord, DutyCollaborationRecord, DutyVerificationRecord } from './api';
import { formatInstant, formatRange } from '../catalogue-view';
import { labelFor } from '../../vocabulary/status';

export interface RegisterRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

/**
 * 次数额度一格的说法。缺席是「来源未提供」；0 在领域就约定为未提供，传输层从不送它——一次不合契约把 0 送过来，这里同样
 * 答未提供，不显成「0 次」：那是页面凭空造出的「额度已用尽」，而余额根本不是本册登记内容。
 */
export function credentialUsesLabel(uses?: number): string {
  if (uses === undefined || uses === 0) return '来源未提供';
  return `${uses} 次`;
}

/**
 * 登记人一格。缺席是那一路不记登记人（接口或受控 CLI 送入，ADR-0236 决定三），照实说「未记」并点出缘由，不显成空白
 * ——空白读起来像渲染丢了东西，也不显成「无」——那会被读成没有人登记过。
 */
export function credentialRegistrantLabel(evidence?: string): string {
  return evidence ? evidence : '未记(非手工登记)';
}

// 有效期两端合成一段区间、登记时间另列：三个时刻是三件事，混进一列读者就分不出「何时登进来」与「何时有效」。
export function credentialRows(records: CredentialRecord[]): RegisterRow[] {
  return records.map((record) => ({
    // 行键循库主键 (tenant, credential_id)：一身份一版。
    key: `credential:${record.credential}`,
    values: {
      credential: record.credential,
      issuer: record.issuer,
      holder: record.holder,
      procedure: record.procedure,
      validity: formatRange(record.validFrom, record.validTo),
      uses: credentialUsesLabel(record.uses),
      registeredAt: formatInstant(record.registeredAt),
      registeredBy: credentialRegistrantLabel(record.operationEvidence),
    },
  }));
}

// 义务依据一列按格取字段：核定税费格显税费引用，明确无需付款格显无需付款依据。**不借字段**——本格缺了就显缺席，
// 不拿另一格的顶上：两格同在或同缺都立不起领域对象，真到了这里就是一行不合契约的响应，让它露出来。
function collaborationBasis(record: DutyCollaborationRecord): string {
  if (record.kind === 'ASSESSED_DUTY') return record.duty ?? '—';
  if (record.kind === 'EXPLICITLY_NOT_REQUIRED') return record.noPayBasis ?? '—';
  return record.duty ?? record.noPayBasis ?? '—';
}

export function collaborationRows(records: DutyCollaborationRecord[]): RegisterRow[] {
  return records.map((record) => ({
    // 行键循库主键 (tenant, scope_ref, duty_ref)：无需付款格的税费引用在键上是空串。
    key: `collaboration:${record.scope}:${record.duty ?? ''}`,
    values: {
      scope: record.scope,
      kindCode: record.kind,
      kind: labelFor(record.kind),
      basis: collaborationBasis(record),
      obligor: record.obligor,
      requirement: record.requirement,
      target: record.target,
      formedAt: formatInstant(record.formedAt),
    },
  }));
}

// 三轴三列各译各的词，行上没有任何合成列（ADR-0137 决定三）：在这里折一次，页面就成了第二处判断权威，而放行检查那一道
// 怎么读三态是按监管程序登记进来的规则。labelFor 对集外取值原样回显，坏数据露出来而不是被译顺。
export function verificationRows(records: DutyVerificationRecord[]): RegisterRow[] {
  return records.map((record) => ({
    // 行键循库主键 (tenant, duty_ref, funds_ref, scope_ref, version_digest)：同三维键的多版本各自成行，不折「当前版」。
    key: `verification:${record.duty}:${record.funds}:${record.scope}:${record.version}`,
    values: {
      duty: record.duty,
      funds: record.funds,
      fundsVersion: record.fundsVersion,
      scope: record.scope,
      procedure: record.procedure,
      version: record.version,
      coverageCode: record.coverage,
      deltaCode: record.delta,
      validityCode: record.validity,
      coverage: labelFor(record.coverage, 'duty-coverage'),
      delta: labelFor(record.delta),
      validity: labelFor(record.validity),
      basis: record.basis,
      verifiedAt: formatInstant(record.verifiedAt),
    },
  }));
}
