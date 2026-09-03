import { useEffect, useState } from 'react';
import type { ApiResult } from '../catalogue-api';
import {
  listReferenceSeriesCoverage,
  type ReferenceSeriesCoverageListResponseBody,
} from './api';
import { coverageNote, coverageRowOf, type CoverageRow } from './coverage-rows';
import { labelOf, seriesKindLabels } from './presentation';

/**
 * 参考序列页顶部的覆盖地平线摘要条（票 pricing-reference-series-operations/05 第 3 项）。
 *
 * **它存在的理由是让缺口在当天被看见，而不是月底对账时**——所以它摆在目录之上而不是另开
 * 一页。判读全在 `coverage-rows.ts`（有 node:test 钉着），本组件只负责摆。
 *
 * **三件不做，逐条写明，因为它们看起来都像是顺手就能加的：**
 *
 * 1. **不标黄不标红。** 「剩余低于 N 天告警」的 N 是租户参数（票面第 4 项：机制只把数算
 *    出来摆在读面，阈值留在配置里显式未决）。没有 N 而先上颜色，等于替租户定了一个。
 * 2. **不把两个待办计数合成一个。** 等复核人与等登记方更正的续办动作不同，合成之后看的人
 *    不知道该去找谁。
 * 3. **不隐藏 `asOf`。** 「在用」只对某一刻成立；不把那一刻说出来，这一条摘要就成了一个
 *    看起来永久的权威答案。同一条判据下目录页的状态列被裁为不含「在用」——那一页没有正当
 *    的时刻源，本端点有，代价就是把它显出来。
 */
export function CoverageSummaryBar() {
  const [answer, setAnswer] =
    useState<ApiResult<ReferenceSeriesCoverageListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    void listReferenceSeriesCoverage().then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  if (answer === null) return null;

  if (answer.kind !== 'outcome') {
    // 摘要条取不到不挡目录：下面那张表有自己的三态呈现，这里再铺一遍同样的话是噪声。
    // 但**未配置要说一句**——静默消失会让人以为这一册没有覆盖问题，而实际上是没问过。
    return (
      <div className="shrink-0 px-4 pt-3 text-xs text-idpxyz-textMuted">
        {answer.kind === 'unconfigured'
          ? '覆盖摘要未取到：接入渠道未配置（403）。这是诚实答案不是「本册无缺口」——今天没有问到。'
          : '覆盖摘要未取到；下方目录仍如实呈现各版本。'}
      </div>
    );
  }

  const asOf = answer.body.asOf;
  const rows = answer.body.series.map((record) => coverageRowOf(record, asOf));
  if (rows.length === 0) {
    return (
      <div className="shrink-0 px-4 pt-3 text-xs text-idpxyz-textMuted">
        当前租户内尚无参考序列，覆盖摘要为空——空是正常业务答案，不是故障。
      </div>
    );
  }

  return (
    <div className="shrink-0 px-4 pt-3">
      <div className="flex items-baseline justify-between">
        <span className="text-xs text-idpxyz-textMuted">
          覆盖地平线 —— 一条序列一格
        </span>
        <span className="font-mono text-[11px] text-idpxyz-textMuted">
          在用判定基准时刻 asOf {asOf}
        </span>
      </div>
      <div className="mt-2 flex flex-wrap gap-2">
        {rows.map((row) => (
          <CoverageChip key={`${row.seriesId}/${row.kind}`} row={row} />
        ))}
      </div>
      <p className="mt-2 text-[11px] text-idpxyz-textMuted">
        告警阈值未配置（实例半边）：本条只把数摆出来，不判紧急、不标色——「剩余低于几天算
        告警」属租户治理参数，产品不替它定一个默认值。
      </p>
    </div>
  );
}

function CoverageChip({ row }: { row: CoverageRow }) {
  return (
    <div className="rounded border border-idpxyz-border px-3 py-2 text-xs">
      <div className="font-mono text-[12px] text-idpxyz-accent">{row.seriesId}</div>
      <div className="text-[11px] text-idpxyz-textMuted">
        {labelOf(seriesKindLabels, row.kind)} · 已登记 {row.registeredVersionCount} 版
      </div>
      <div className="mt-1">
        {coverageNote(row.coverage)}
        {row.coverage.inForceVersion ? (
          <span className="ml-1 font-mono text-[11px] text-idpxyz-textMuted">
            （{row.coverage.inForceVersion}）
          </span>
        ) : null}
      </div>
      {row.pending.attention ? (
        <div className="mt-1 text-[11px] text-idpxyz-textMuted">
          {/* 两格分列而不是相加：一格等复核人、一格等登记方更正。 */}
          待复核 {row.pending.unreviewed} 版 · 已退回待更正 {row.pending.returned} 版
        </div>
      ) : null}
      <div className="mt-1 text-[11px] text-idpxyz-textMuted">
        {row.lastReview
          ? `最近复核 ${row.lastReview.at}（${row.lastReview.decision}）`
          : '尚无复核记录'}
      </div>
    </div>
  );
}
