import { useEffect, useState } from 'react';
import type { ApiResult } from '../catalogue-api';
import {
  countPendingSeriesEvaluations,
  listReferenceSeriesCoverage,
  type PendingSeriesEvaluationsResponseBody,
  type ReferenceSeriesCoverageListResponseBody,
} from './api';
import { coverageNote, coverageRowOf, type CoverageRow } from './coverage-rows';
import { sameSeries, type SeriesKey } from './catalogue-filter';
import { pendingEvaluationsCellOf, pendingEvaluationsNote } from './pending-evaluations';
import { labelOf, seriesKindLabels } from './presentation';

interface CoverageSummaryBarProps {
  /**
   * 「一键跳到该序列的复核动作」的落法：点一格，页面把下方目录只看到这条序列（标识 + 种类），
   * 行上的「复核」按钮就收到眼前；再点同一格取消。传 `null` 即取消。
   *
   * 为什么不是直接开复核面板：复核面板吃的是（标识 + 版本 + 登记责任方），而覆盖响应一条序列
   * 一行、没有责任方，「今天没有在用版本」那一格更没有可指的版本——摘要条拿不出一个能直接开
   * 面板的目标，硬拼一个就是替人挑了要复核哪一版。收窄目录让人自己挑，是不造数据的那条路。
   */
  onSelectSeries?: (key: SeriesKey | null) => void;
  /** 当前只看的那条；由页面持有，摘要条只据它高亮与切换。 */
  focusedSeries?: SeriesKey | null;
}

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
export function CoverageSummaryBar({
  onSelectSeries,
  focusedSeries = null,
}: CoverageSummaryBarProps = {}) {
  const [answer, setAnswer] =
    useState<ApiResult<ReferenceSeriesCoverageListResponseBody> | null>(null);
  const [pendingAnswer, setPendingAnswer] =
    useState<ApiResult<PendingSeriesEvaluationsResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    void listReferenceSeriesCoverage().then((result) => {
      if (!cancelled) setAnswer(result);
    });
    // 挂起评价数走另一只读口（ADR-0105 Decision 五：伴生读端口，不拓宽覆盖读口）；两次请求各回各的
    // asOf，摆在一起时各标各的时刻，不假装是同一刻。
    void countPendingSeriesEvaluations().then((result) => {
      if (!cancelled) setPendingAnswer(result);
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
        <PendingEvaluationsCell answer={pendingAnswer} />
      </div>
    );
  }

  const asOf = answer.body.asOf;
  const rows = answer.body.series.map((record) => coverageRowOf(record, asOf));
  if (rows.length === 0) {
    return (
      <div className="shrink-0 px-4 pt-3 text-xs text-idpxyz-textMuted">
        当前租户内尚无参考序列，覆盖摘要为空——空是正常业务答案，不是故障。
        <PendingEvaluationsCell answer={pendingAnswer} />
      </div>
    );
  }

  return (
    <div className="shrink-0 px-4 pt-3">
      <div className="flex items-baseline justify-between">
        <span className="text-xs text-idpxyz-textMuted">
          覆盖地平线 —— 一条序列一格
          {onSelectSeries ? (focusedSeries ? '；下方目录只看所选' : '；点一格只看它') : null}
          {onSelectSeries && focusedSeries ? (
            <button
              type="button"
              className="ml-2 underline"
              onClick={() => onSelectSeries(null)}
            >
              取消只看
            </button>
          ) : null}
        </span>
        <span className="font-mono text-[11px] text-idpxyz-textMuted">
          在用判定基准时刻 asOf {asOf}
        </span>
      </div>
      <div className="mt-2 flex flex-wrap gap-2">
        {rows.map((row) => {
          const focused = focusedSeries !== null && sameSeries(row, focusedSeries);
          return (
            <CoverageChip
              key={`${row.seriesId}/${row.kind}`}
              row={row}
              focused={focused}
              // 再点已选中的那格即取消，与「取消只看」同义；不选中的格点了就换成它。
              onSelect={
                onSelectSeries
                  ? () => onSelectSeries(focused ? null : { seriesId: row.seriesId, kind: row.kind })
                  : undefined
              }
            />
          );
        })}
      </div>
      <PendingEvaluationsCell answer={pendingAnswer} />
      <p className="mt-2 text-[11px] text-idpxyz-textMuted">
        告警阈值未配置（实例半边）：本条只把数摆出来，不判紧急、不标色——「剩余低于几天算
        告警」属租户治理参数，产品不替它定一个默认值。
      </p>
    </div>
  );
}

/**
 * 「挂起评价数」那一格（ADR-0105 Decision 五；票 05 第 2 项）。判读在 `pending-evaluations.ts`，
 * 这里只摆。**不摆 0 占位**：请求未回、403、故障三种形态都不显示数字，措辞明说「没问到」；只有服务端
 * 作答且为空才说「没有」。
 *
 * 数的是问题项子表有行的评价——子表随 ADR-0105 落地且不回填，此前落册的待判断评价不在这个数里，
 * 文案如实写出，不包装成「全部挂起评价」。**不做「跳到登记」**：一格数指不出该登记哪条序列的哪一版，
 * 硬拼一个目标就是替人挑（理由同摘要条不直接开复核面板那条）。
 */
function PendingEvaluationsCell({
  answer,
}: {
  answer: ApiResult<PendingSeriesEvaluationsResponseBody> | null;
}) {
  const cell = pendingEvaluationsCellOf(answer);
  return (
    <div className="mt-2 text-[11px] text-idpxyz-textMuted">
      <span>{pendingEvaluationsNote(cell)}</span>
      {cell.state === 'counted' && cell.byKind.length > 0 ? (
        <span className="ml-1">
          {cell.byKind.map((count) => (
            <span key={count.kind} className="mr-2 font-mono">
              {labelOf(seriesKindLabels, count.kind)} {count.evaluationCount}
            </span>
          ))}
        </span>
      ) : null}
      {cell.state === 'counted' ? (
        <span className="ml-1 font-mono">（asOf {cell.asOf}；只计 ADR-0105 子表落地后写入的评价，不回填）</span>
      ) : null}
    </div>
  );
}

function CoverageChip({
  row,
  focused,
  onSelect,
}: {
  row: CoverageRow;
  focused: boolean;
  onSelect?: () => void;
}) {
  const body = (
    <>
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
    </>
  );

  // 高亮只标「选中」，不标颜色等级——这一格仍然不判紧急（阈值未配置，见上）。
  const frame = `rounded border px-3 py-2 text-xs text-left ${
    focused ? 'border-idpxyz-accent' : 'border-idpxyz-border'
  }`;
  if (!onSelect) return <div className={frame}>{body}</div>;
  return (
    <button
      type="button"
      className={frame}
      onClick={onSelect}
      aria-pressed={focused}
      title={focused ? '取消只看这条序列' : '在下方目录中只看这条序列'}
    >
      {body}
    </button>
  );
}
