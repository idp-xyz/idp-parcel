// 关务记录四口的行判读（清关单 / 报关批次 / 申报资料 / 申报记录 / 海关结果 / 放行记录）。抽成纯函数，让「哪一格不得被译顺」能
// 在 node:test 里钉住：技术回执、海关接收、海关受理、放行是分层保存的外部结果，申报记录逐层各占一列，不由别的层推出来。
//
// 清关单列表上的「最新进展」是例外：它取这张清关单下已有事实里最靠后的那一层（有放行就说已放行，只有发送就说已发送），
// 是给运营一眼看到「走到哪了」的进度词，不是把各层合并成一个状态机——各层事实仍在申报记录与海关结果两签逐层可查。

import type {
  AdmissionJudgementBody,
  CustomsCaseBody,
  ExternalResultBody,
  SubmissionAttemptBody,
  SubmissionVersionBody,
} from './api';
import { formatInstant } from '../catalogue-view';
import { labelFor } from '../../vocabulary/status';
import type { RegisterRow } from './register-rows';
import { shortenEmbeddedPrincipalRefs } from '../../components/principal-ref';

const dash = '—';

/** 几个包裹号一格：一票一行，多票时逐个列出。 */
function parcelList(parcels: string[]): string {
  return parcels.join('\n') || dash;
}

/** 一张清关单下的申报记录与海关结果：按报关批次归属，结果再按归属的申报版本或范围归属。 */
function caseFacts(record: CustomsCaseBody, submissions: SubmissionVersionBody[], results: ExternalResultBody[]) {
  const units = new Set(record.units.map((unit) => unit.unit));
  const mine = submissions.filter((version) => units.has(version.unit));
  const versions = new Set(mine.map((version) => version.version));
  const myResults = results.filter(
    (result) => (result.submissionVersion && versions.has(result.submissionVersion)) || (result.scope && units.has(result.scope)),
  );
  return { mine, myResults };
}

/**
 * 清关单的最新进展码（词与色调在 vocabulary/status.ts）：已关案 > 放行结果 > 海关处置 > 海关受理 > 海关接收 > 发送失败 >
 * 已发送 > 待发送 > 待申报。取的是已有事实里最靠后的一层。
 */
export function caseProgress(record: CustomsCaseBody, submissions: SubmissionVersionBody[], results: ExternalResultBody[]): string {
  if (record.closure) return 'CUSTOMS_CASE_CLOSED';
  const { mine, myResults } = caseFacts(record, submissions, results);
  const release = myResults.find((result) => result.release);
  if (release?.release) return release.release.kind === 'FULL' ? 'CUSTOMS_RELEASED' : release.release.kind;
  for (const layer of ['DISPOSITION_DECISION', 'BUSINESS_ACCEPTANCE', 'REGULATORY_RECEIPT']) {
    if (myResults.some((result) => result.layer === layer)) return layer;
  }
  const attempts = mine.flatMap((version) => version.attempts);
  if (attempts.some((attempt) => attempt.sendFact?.outcome === 'FAILED' || attempt.result === 'FAILED')) return 'DECLARATION_SEND_FAILED';
  if (attempts.some((attempt) => attempt.sendFact)) return 'DECLARATION_SENT';
  if (attempts.length > 0 || mine.length > 0) return 'DECLARATION_PENDING_SEND';
  return 'CUSTOMS_PENDING_DECLARATION';
}

/** 清关单上最后一次有事实落下的时刻：建案、批次形成、申报固定、发送、海关结果、关案里最晚的那个。 */
export function caseUpdatedAt(record: CustomsCaseBody, submissions: SubmissionVersionBody[], results: ExternalResultBody[]): string {
  const { mine, myResults } = caseFacts(record, submissions, results);
  const instants = [
    record.establishedAt,
    ...record.units.map((unit) => unit.formedAt),
    ...mine.map((version) => version.fixedAt),
    ...mine.flatMap((version) => version.attempts.map((attempt) => attempt.sendFact?.sentAt ?? attempt.sentAt ?? '')),
    ...myResults.map((result) => result.recordedAt),
    record.closure?.closedAt ?? '',
  ].filter(Boolean);
  const sorted = instants.sort();
  return sorted[sorted.length - 1] ?? record.establishedAt;
}

export function caseRows(cases: CustomsCaseBody[], submissions: SubmissionVersionBody[] = [], results: ExternalResultBody[] = []): RegisterRow[] {
  return cases.map((record) => ({
    key: `case:${record.case}`,
    values: {
      id: record.case,
      parcels: parcelList(record.parcels.map((parcel) => parcel.parcel)),
      customers: record.parcels.map((parcel) => parcel.customer).join('\n') || dash,
      jurisdiction: record.jurisdiction,
      direction: labelFor(record.direction),
      procedure: record.procedure,
      obligation: record.obligation,
      units: record.units.map((unit) => unit.unit).join('\n') || '尚无报关批次',
      progress: caseProgress(record, submissions, results),
      updatedAt: formatInstant(caseUpdatedAt(record, submissions, results)),
      // 缺席就是未关案，如实说；不写「状态未知」。
      closure: record.closure
        ? `已关案：${shortenEmbeddedPrincipalRefs(record.closure.decidedBy)}（${formatInstant(record.closure.closedAt)}），核对 ${record.closure.itemCount} 项` +
          (record.closure.reopenCount > 0 ? `，重开 ${record.closure.reopenCount} 次` : '')
        : '未关案',
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
        jurisdiction: record.jurisdiction,
        direction: labelFor(record.direction),
        procedure: unit.procedure,
        members: parcelList(unit.members),
        formedAt: formatInstant(unit.formedAt),
        replaces: unit.replacesUnit ?? dash,
      },
    })),
  );
}

/** 申报资料一行即一个申报版本固定的那份资料：资料引用、角色快照引用与内容摘要进技术详情。 */
export function snapshotRows(submissions: SubmissionVersionBody[]): RegisterRow[] {
  return submissions.map((version) => ({
    key: `snapshot:${version.version}`,
    values: {
      version: version.version,
      unit: version.unit,
      procedure: version.procedure,
      dossier: version.dossier,
      roles: version.roles,
      digest: version.contentDigest,
      members: parcelList(version.members),
      current: version.isCurrent ? '当前版' : '已被更正取代',
      fixedAt: formatInstant(version.fixedAt),
    },
  }));
}

/** 发送事实一格：缺席是还没有越过发送边界（待发送），不写成「失败」。 */
export function attemptLabel(attempt: SubmissionAttemptBody): string {
  const result = labelFor(attempt.result);
  const sent = attempt.sendFact
    ? `，发送事实 ${labelFor(attempt.sendFact.outcome)}（${formatInstant(attempt.sendFact.sentAt)}）`
    : '，尚无发送事实';
  return `#${attempt.sequence} → ${attempt.target}：${result}${sent}`;
}

export function receiptLabel(attempt: SubmissionAttemptBody): string {
  if (attempt.receipts.length === 0) return '尚无技术回执';
  return attempt.receipts
    .map((receipt) => `${labelFor(receipt.outcome, 'customs-receipt')}（${formatInstant(receipt.receivedAt)}）`)
    .join('；');
}

function layerLabel(results: ExternalResultBody[], layer: string): string {
  const matched = results.filter((result) => result.layer === layer);
  if (matched.length === 0) return '尚无结果';
  return matched.map((result) => `${result.rawSemantics || result.source}${result.layerConflict ? '（同层冲突）' : ''}`).join('；');
}

/** 最近一次发送的一句话：给申报记录列表上的「发送」一格。 */
export function latestSendLabel(version: SubmissionVersionBody): string {
  const latest = version.attempts[version.attempts.length - 1];
  if (!latest) return '尚未发送';
  if (!latest.sendFact) return `第 ${latest.sequence} 次：${labelFor(latest.result)}`;
  return `第 ${latest.sequence} 次：${labelFor(latest.sendFact.outcome)}（${formatInstant(latest.sendFact.sentAt)}）`;
}

/**
 * 申报记录一行：把海关结果按「归属的申报版本」挂回去，海关接收、海关受理各占一列，任一层缺失就写「尚无结果」，
 * 不由别的层推出来。
 */
export function submissionRows(submissions: SubmissionVersionBody[], results: ExternalResultBody[]): RegisterRow[] {
  return submissions.map((version) => {
    const mine = results.filter((result) => result.submissionVersion === version.version);
    return {
      key: `submission:${version.version}`,
      values: {
        version: version.version,
        unit: version.unit,
        procedure: version.procedure,
        members: parcelList(version.members),
        authority: shortenEmbeddedPrincipalRefs(version.authority),
        readinessBasis: version.readinessBasis,
        current: version.isCurrent ? '当前版' : '已被更正取代',
        correctedFrom: version.correctedFrom ?? '',
        latestSend: latestSendLabel(version),
        attempts: version.attempts.map(attemptLabel).join('\n') || '尚无提交尝试',
        receipts: version.attempts.map(receiptLabel).join('\n') || dash,
        channelMessages: version.attempts.map((attempt) => attempt.sendFact?.channelMessage ?? '').filter(Boolean).join('；'),
        received: layerLabel(mine, 'REGULATORY_RECEIPT'),
        accepted: layerLabel(mine, 'BUSINESS_ACCEPTANCE'),
        fixedAt: formatInstant(version.fixedAt),
      },
    };
  });
}

/** 放行结果与其它海关结果：逐份上列；归属不上的响应只留原始语义，不猜版本。 */
export function externalResultRows(results: ExternalResultBody[]): RegisterRow[] {
  return results.map((result) => ({
    key: `result:${result.source}`,
    values: {
      source: result.source,
      layer: result.layer ? labelFor(result.layer) : '未能归属',
      layerCode: result.layer ?? '',
      version: result.unattributable
        ? `未能归属${result.claimedVersion ? `（来源声称 ${result.claimedVersion}）` : ''}`
        : (result.submissionVersion ?? dash),
      scope: result.scope ?? dash,
      semantics: result.rawSemantics || dash,
      rule: result.rule ?? '',
      releaseKind: result.release?.kind ?? '',
      release: result.release
        ? `${labelFor(result.release.kind)}｜${shortenEmbeddedPrincipalRefs(result.release.authority)}｜范围 ${result.release.scope}` +
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
      id: judgement.judgement,
      object: judgement.object,
      intent: labelFor(judgement.intent),
      route: judgement.onwardNode ? `${judgement.node} → ${judgement.onwardNode}` : judgement.node,
      crossZone: labelFor(judgement.crossZone),
      answerCode: judgement.answer,
      answer: labelFor(judgement.answer),
      // 逐动作答复与依据原样列出，整次答复已在左一列；依据来源不再翻译。
      findings: judgement.findings
        .map(
          (finding) =>
            `${labelFor(finding.action)}：${labelFor(finding.answer)}` +
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
