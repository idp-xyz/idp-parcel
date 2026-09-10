import { formatInstant } from '../catalogue-view';
import type { RecordedJudgmentsRecord, ReviewControlItemRecord } from './api';
import { controlFailureDispositionLabels, withCode } from './presentation';

// 已记录的权威判断三组，复核页与授权处置页共用一份呈现：两页审的都是判断表上这批行，
// 分两处写就得在两处约定哪一份是准的。

/**
 * 一项控制的一行：种类、判断顺序、结论、依据原词直显；受限项上的两格采用引用（正文登记的失败处置与
 * 责任引用，ADR-0132 决定三、四）有才显——成立项与采用之前记下的受限项两格缺席，不写「无」冒充登记过。
 */
function controlItemText(item: ReviewControlItemRecord): string {
  const parts = [`${item.kind} · 第 ${item.order} 项 · ${item.conclusion}`];
  if (item.basis) parts.push(`依据 ${item.basis}`);
  if (item.failureDisposition) {
    parts.push(
      `失败处置 ${withCode(controlFailureDispositionLabels[item.failureDisposition], item.failureDisposition)}`,
    );
  }
  if (item.responsibility) parts.push(`责任 ${item.responsibility}`);
  return parts.join(' · ');
}

/**
 * 已记录的权威判断。三组各自缺席时如实说「尚未形成」——财务控制与采用解析整格缺席是端点的约定
 * （不造空结果冒充判断过），折成「无」会让「问过了，答案是没有」与「还没问」在同一句话里分不开。
 */
export function recordedJudgmentsBlock(recorded: RecordedJudgmentsRecord) {
  const control = recorded.financialControl;
  return (
    <div className="flex flex-col gap-2 text-[12px]">
      <div>
        <span className="text-idpxyz-textMuted">可达性判断</span>
        {recorded.reachability.length === 0 ? (
          <p className="mt-1 text-idpxyz-textMuted">尚未形成。</p>
        ) : (
          <ul className="mt-1 flex flex-col gap-0.5">
            {recorded.reachability.map((row) => (
              <li key={`${row.parcelId}:${row.judgmentId ?? row.basis ?? row.value}`}>
                <span className="font-mono">{row.parcelId}</span>
                {' · '}
                <span className="font-mono">{row.value}</span>
                {row.basis ? ` · 依据 ${row.basis}` : ''}
                {row.asOfAt ? ` · 截至 ${formatInstant(row.asOfAt)}` : ''}
              </li>
            ))}
          </ul>
        )}
      </div>
      <div>
        <span className="text-idpxyz-textMuted">财务控制</span>
        {control ? (
          <>
            <p className="mt-1">
              {`${control.outcome}${control.basis ? ` · 依据 ${control.basis}` : ''}`}
            </p>
            {/* 逐项控制项：`明确无控制`没有项，列表照实为空，不写「无」。 */}
            {control.items.length > 0 && (
              <ul className="mt-1 flex flex-col gap-0.5">
                {control.items.map((item) => (
                  <li key={`${item.kind}:${item.order}`} className="font-mono">
                    {controlItemText(item)}
                  </li>
                ))}
              </ul>
            )}
          </>
        ) : (
          <p className="mt-1">尚未形成。</p>
        )}
      </div>
      <div>
        <span className="text-idpxyz-textMuted">采用商务解析</span>
        <p className="mt-1 font-mono">{recorded.adoptedResolutionId ?? '尚未形成。'}</p>
      </div>
    </div>
  );
}
