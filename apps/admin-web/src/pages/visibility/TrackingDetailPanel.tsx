import type { ReactNode } from 'react';
import { X } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails, type TechnicalRow } from '../../components/TechnicalDetails';
import { objectNouns } from '../../vocabulary/objects';
import { formatInstant } from '../moment';
import { sourceContextLabels } from './presentation';
import { currentStatusLabelOf, type TrackingRow } from './tracking-list';

// 物流动态工作台的详情栏：头部运单号 + 当前状态胶囊，三格事实（更新时间 / 动态条数 / 被替代条数），正文按服务链顺序列各源
// 最后一步——「节点 · 已收寄 · 时刻」每条一句人话；投影版本、前身、里程碑原码与各源事实引用进底部「技术详情」。
// 内容全部取自投影读模型，不为详情栏另开读口；时间轴跨源合并的读口是第二步的事（设计源第八节）。

export function TrackingDetailPanel({ row, onClose }: { row: TrackingRow; onClose: () => void }) {
  const technical: TechnicalRow[] = [
    { label: '包裹标识', value: row.parcelId },
    { label: '投影版本', value: row.projectionVersion },
    { label: '前身版本', value: row.priorVersion ?? '' },
    { label: '里程碑码', value: row.latestMilestone },
    { label: '替代关系', value: row.supersededCount > 0 ? `${row.supersededCount} 条被替代` : '' },
    ...row.latestBySource.map((entry) => ({
      label: `${sourceContextLabels[entry.source] ?? entry.source}事实`,
      value: `${entry.kind} · ${entry.fact} @ ${entry.factVersion}`,
    })),
  ];

  return (
    <div className="flex h-full w-full flex-col bg-idpxyz-bg" aria-label={`${objectNouns.trackingUpdates} ${row.parcelId}`}>
      <div className="space-y-3 border-b border-idpxyz-border px-5 pb-3 pt-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="truncate font-mono text-[16px] font-semibold text-idpxyz-textBright" title={row.parcelId}>
                {row.parcelId}
              </h2>
              {row.latestMilestone ? <StatusPill code={row.latestMilestone} /> : <StatusPill label={currentStatusLabelOf(row)} tone="muted" />}
            </div>
            <p className="mt-0.5 text-[12px] text-idpxyz-breadcrumb">
              {objectNouns.updatedAt}{' '}
              <time className="font-mono" dateTime={row.derivedAt} title={row.derivedAt}>
                {formatInstant(row.derivedAt)}
              </time>
            </p>
          </div>
          <Button
            className="shrink-0 text-idpxyz-breadcrumb"
            variant="ghost"
            size="icon"
            onClick={onClose}
            aria-label="关闭详情"
            title="关闭（Esc）"
          >
            <X className="h-4 w-4" aria-hidden="true" />
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-3 gap-2.5 border-b border-idpxyz-border px-5 py-3">
        <Fact label="当前有效动态">{row.currentCount} 条</Fact>
        <Fact label="已被更正">{row.supersededCount} 条</Fact>
        <Fact label="来源段数">{row.latestBySource.length}</Fact>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
        <h3 className="text-[12px] font-medium text-idpxyz-breadcrumb">各段最后一步</h3>
        {row.latestBySource.length === 0 ? (
          <p className="mt-2 text-[12px] text-idpxyz-breadcrumb">还没有任何动态。</p>
        ) : (
          <ol className="mt-2 space-y-2">
            {row.latestBySource.map((entry) => (
              <li key={`${entry.source}-${entry.fact}`} className="flex items-baseline gap-2 text-[13px]">
                <span className="w-[3em] shrink-0 text-idpxyz-breadcrumb">{sourceContextLabels[entry.source] ?? entry.source}</span>
                <StatusPill code={entry.kind} />
                <time className="ml-auto shrink-0 font-mono text-[12px] text-idpxyz-breadcrumb" dateTime={entry.occurredAt} title={entry.occurredAt}>
                  {formatInstant(entry.occurredAt)}
                </time>
              </li>
            ))}
          </ol>
        )}
        <p className="mt-3 text-[12px] leading-5 text-idpxyz-breadcrumb">
          动态按业务发生时间排列；同一件事有更正时只显最新一版，被更正的版本数在上方「已被更正」。
        </p>
        <TechnicalDetails className="mt-4" rows={technical} />
      </div>
    </div>
  );
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="min-w-0 rounded-md border border-idpxyz-border bg-idpxyz-sidebar px-3 py-2">
      <p className="text-[12px] text-idpxyz-breadcrumb">{label}</p>
      <p className="mt-0.5 break-words text-[12px] font-semibold leading-snug text-idpxyz-textBright">{children}</p>
    </div>
  );
}
