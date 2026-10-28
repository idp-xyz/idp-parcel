import { TechnicalDetails, type TechnicalRow } from '../../components/TechnicalDetails';
import { StatusPill } from '../../components/StatusPill';
import { labelFor } from '../../vocabulary/status';
import { formatInstant } from '../catalogue-view';
import type { RecordedJudgmentsRecord, ReviewControlItemRecord } from './api';
import { ParcelNo } from './refs';
import { controlFailureDispositionLabels } from './presentation';

// 已记录的权威判断三组,复核页与授权处置页共用一份呈现:两页审的都是判断表上这批行,
// 分两处写就得在两处约定哪一份是准的。屏上按运营的问法分三段:能否送达、付款控制、采用的商务条款;
// 判断标识、依据引用与时点语义进技术详情。

/**
 * 一项控制的一行:种类与结论经词典译;受限项上的两格采用引用(正文登记的失败处置与责任引用,ADR-0132 决定三、四)
 * 有才显——成立项与采用之前记下的受限项两格缺席,不写「无」冒充登记过。
 */
function controlItemText(item: ReviewControlItemRecord): string {
  const parts = [`${labelFor(item.kind)}:${labelFor(item.conclusion)}`];
  if (item.failureDisposition) {
    parts.push(`不通过时${controlFailureDispositionLabels[item.failureDisposition] ?? item.failureDisposition}`);
  }
  if (item.responsibility) parts.push(`责任方 ${item.responsibility}`);
  return parts.join(' · ');
}

function technicalRowsOf(recorded: RecordedJudgmentsRecord): TechnicalRow[] {
  const rows: TechnicalRow[] = [];
  recorded.reachability.forEach((row, index) => {
    rows.push({ label: `可达性 ${index + 1}`, value: [row.parcelId, row.value, row.judgmentId ?? '', row.basis ?? ''].filter(Boolean).join(' · ') });
  });
  const control = recorded.financialControl;
  if (control) {
    rows.push({ label: '财务控制', value: [control.outcome, control.resultId ?? '', control.basis ?? ''].filter(Boolean).join(' · ') });
    control.items.forEach((item, index) => {
      rows.push({ label: `控制项 ${index + 1}`, value: [item.kind, `#${item.order}`, item.conclusion, item.basis ?? '', item.failureDisposition ?? '', item.responsibility ?? ''].filter(Boolean).join(' · ') });
    });
  }
  rows.push({ label: '采用的商务解析', value: recorded.adoptedResolutionId ?? '' });
  return rows;
}

/**
 * 已记录的权威判断。三组各自缺席时如实说「还没判」——财务控制与采用解析整格缺席是端点的约定
 * (不造空结果冒充判断过),折成「无」会让「问过了,答案是没有」与「还没问」在同一句话里分不开。
 */
export function recordedJudgmentsBlock(recorded: RecordedJudgmentsRecord) {
  const control = recorded.financialControl;
  return (
    <div className="flex flex-col gap-2 text-[12px]">
      <div>
        <span className="text-idpxyz-textMuted">能否送达</span>
        {recorded.reachability.length === 0 ? (
          <p className="mt-1 text-idpxyz-textMuted">还没判。</p>
        ) : (
          <ul className="mt-1 flex flex-col gap-0.5">
            {recorded.reachability.map((row) => (
              <li key={`${row.parcelId}:${row.judgmentId ?? row.basis ?? row.value}`} className="flex flex-wrap items-center gap-2">
                <ParcelNo parcelId={row.parcelId} />
                <StatusPill code={row.value} />
                {row.asOfAt ? <span className="text-idpxyz-textMuted">截至 {formatInstant(row.asOfAt)}</span> : null}
              </li>
            ))}
          </ul>
        )}
      </div>
      <div>
        <span className="text-idpxyz-textMuted">付款控制</span>
        {control ? (
          <>
            <p className="mt-1">
              <StatusPill code={control.outcome} scope="financial-control" />
            </p>
            {/* 逐项控制项:`明确无控制`没有项,列表照实为空,不写「无」。 */}
            {control.items.length > 0 && (
              <ul className="mt-1 flex flex-col gap-0.5">
                {control.items.map((item) => (
                  <li key={`${item.kind}:${item.order}`}>{controlItemText(item)}</li>
                ))}
              </ul>
            )}
          </>
        ) : (
          <p className="mt-1">还没判。</p>
        )}
      </div>
      <div>
        <span className="text-idpxyz-textMuted">采用的商务条款</span>
        <p className="mt-1">{recorded.adoptedResolutionId ? '已采用(条款引用见技术详情)' : '还没采用。'}</p>
      </div>
      <TechnicalDetails rows={technicalRowsOf(recorded)} title="判断的技术详情" />
    </div>
  );
}
