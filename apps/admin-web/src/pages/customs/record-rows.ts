// 关务记录四口的行判读(票 demo-2026-10-30/01)。抽成纯函数,让「哪一格不得被译顺」能在 run-tests 里钉住:
// 技术回执、监管接收、业务受理、放行是分层保存的外部结果,页面逐层各占一列,不折出「清关成功」之类的总状态。

import type {
  AdmissionJudgementBody,
  CustomsCaseBody,
  ExternalResultBody,
  SubmissionAttemptBody,
  SubmissionVersionBody,
} from './api';
import { formatInstant } from '../catalogue-view';
import {
  admissionAnswerLabels,
  admissionIntentLabels,
  attemptResultLabels,
  crossZoneLabels,
  directionLabels,
  guardedActionLabels,
  labelOf,
  releaseKindLabels,
  resultLayerLabels,
  technicalReceiptLabels,
} from './presentation';
import type { RegisterRow } from './register-rows';
import { shortenEmbeddedPrincipalRefs } from '../../components/principal-ref';

const dash = '—';

export function caseRows(cases: CustomsCaseBody[]): RegisterRow[] {
  return cases.map((record) => ({
    key: `case:${record.case}`,
    values: {
      id: record.case,
      jurisdiction: record.jurisdiction,
      direction: labelOf(directionLabels, record.direction),
      procedure: record.procedure,
      obligation: record.obligation,
      parcels: record.parcels.map((parcel) => parcel.parcel).join('、') || dash,
      units: record.units.map((unit) => unit.unit).join('、') || '尚无申报单元',
      // 缺席就是未关闭,如实说;不写「状态未知」。
      closure: record.closure
        ? `已关闭：${record.closure.decidedBy}（${formatInstant(record.closure.closedAt)}），依据 ${record.closure.itemCount} 项` +
          (record.closure.reopenCount > 0 ? `，重开 ${record.closure.reopenCount} 次` : '')
        : '未关闭',
      establishedAt: formatInstant(record.establishedAt),
    },
  }));
}

export function unitRows(cases: CustomsCaseBody[]): RegisterRow[] {
  return cases.flatMap((record) =>
    record.units.map((unit) => ({
      key: `unit:${unit.unit}`,
      values: {
        id: unit.unit,
        caseRef: record.case,
        procedure: unit.procedure,
        members: unit.members.join('、') || dash,
        formedAt: formatInstant(unit.formedAt),
        replaces: unit.replacesUnit ?? dash,
      },
    })),
  );
}

/** 资料快照一行即一个提交版本固定的那份资料:资料引用、角色快照引用与内容摘要。 */
export function snapshotRows(submissions: SubmissionVersionBody[]): RegisterRow[] {
  return submissions.map((version) => ({
    key: `snapshot:${version.version}`,
    values: {
      version: version.version,
      unit: version.unit,
      dossier: version.dossier,
      roles: version.roles,
      digest: version.contentDigest,
      members: version.members.join('、') || dash,
      current: version.isCurrent ? '当前版' : '已被更正取代',
      fixedAt: formatInstant(version.fixedAt),
    },
  }));
}

/** 发送事实一格:缺席是还没有越过发送边界(已登记待发送),不写成「失败」。 */
export function attemptLabel(attempt: SubmissionAttemptBody): string {
  const result = labelOf(attemptResultLabels, attempt.result);
  const sent = attempt.sendFact
    ? `，发送事实 ${labelOf(attemptResultLabels, attempt.sendFact.outcome)}（${formatInstant(attempt.sendFact.sentAt)}）`
    : '，尚无发送事实';
  return `#${attempt.sequence} → ${attempt.target}：${result}${sent}`;
}

export function receiptLabel(attempt: SubmissionAttemptBody): string {
  if (attempt.receipts.length === 0) return '尚无技术回执';
  return attempt.receipts
    .map((receipt) => `${labelOf(technicalReceiptLabels, receipt.outcome)}（${formatInstant(receipt.receivedAt)}）`)
    .join('；');
}

function layerLabel(results: ExternalResultBody[], layer: string): string {
  const matched = results.filter((result) => result.layer === layer);
  if (matched.length === 0) return '尚无结果';
  return matched
    .map((result) => `${result.rawSemantics || result.source}${result.layerConflict ? '（同层冲突）' : ''}`)
    .join('；');
}

/**
 * 提交版本一行:把外部结果按「归属的提交版本」挂回去,监管接收、业务受理各占一列,
 * 任一层缺失就写「尚无结果」,不由别的层推出来。
 */
export function submissionRows(
  submissions: SubmissionVersionBody[],
  results: ExternalResultBody[],
): RegisterRow[] {
  return submissions.map((version) => {
    const mine = results.filter((result) => result.submissionVersion === version.version);
    return {
      key: `submission:${version.version}`,
      values: {
        version: version.version,
        unit: version.unit,
        procedure: version.procedure,
        authority: shortenEmbeddedPrincipalRefs(version.authority),
        current: version.isCurrent ? '当前版' : '已被更正取代',
        attempts: version.attempts.map(attemptLabel).join('\n') || '尚无提交尝试',
        receipts: version.attempts.map(receiptLabel).join('\n') || dash,
        received: layerLabel(mine, 'REGULATORY_RECEIPT'),
        accepted: layerLabel(mine, 'BUSINESS_ACCEPTANCE'),
        fixedAt: formatInstant(version.fixedAt),
      },
    };
  });
}

/** 放行结果与其它外部监管结果:逐份上列;归属不上的响应只留原始语义,不猜版本。 */
export function externalResultRows(results: ExternalResultBody[]): RegisterRow[] {
  return results.map((result) => ({
    key: `result:${result.source}`,
    values: {
      source: result.source,
      layer: result.layer ? labelOf(resultLayerLabels, result.layer) : '未能归属',
      version: result.unattributable
        ? `未能归属${result.claimedVersion ? `（来源声称 ${result.claimedVersion}）` : ''}`
        : (result.submissionVersion ?? dash),
      semantics: result.rawSemantics || dash,
      release: result.release
        ? `${labelOf(releaseKindLabels, result.release.kind)}｜${shortenEmbeddedPrincipalRefs(result.release.authority)}｜范围 ${result.release.scope}` +
          (result.release.condition ? `｜条件 ${result.release.condition}` : '')
        : dash,
      flags:
        [
          result.layerConflict ? '同层冲突' : '',
          result.correctsSource ? `更正 ${result.correctsSource}${result.correctionInForce ? '（已生效）' : ''}` : '',
        ]
          .filter(Boolean)
          .join('；') || dash,
      occurredAt: result.occurredAt ? formatInstant(result.occurredAt) : dash,
      recordedAt: formatInstant(result.recordedAt),
    },
  }));
}

export function judgementRows(judgements: AdmissionJudgementBody[]): RegisterRow[] {
  return judgements.map((judgement) => ({
    key: `judgement:${judgement.judgement}`,
    values: {
      object: judgement.object,
      intent: labelOf(admissionIntentLabels, judgement.intent),
      route: judgement.onwardNode ? `${judgement.node} → ${judgement.onwardNode}` : judgement.node,
      crossZone: labelOf(crossZoneLabels, judgement.crossZone),
      answer: labelOf(admissionAnswerLabels, judgement.answer),
      // 逐动作答复与依据原样列出,折叠(整次答复)已在左一列;依据来源不再翻译。
      findings: judgement.findings
        .map(
          (finding) =>
            `${labelOf(guardedActionLabels, finding.action)}：${labelOf(admissionAnswerLabels, finding.answer)}` +
            (finding.bases.length > 0
              ? `（依据 ${finding.bases.map((basis) => `${basis.cause}${basis.sources.length ? ` ${basis.sources.join('、')}` : ''}`).join('；')}）`
              : ''),
        )
        .join('\n'),
      actionAt: formatInstant(judgement.actionAt),
      judgedAt: formatInstant(judgement.judgedAt),
    },
  }));
}
