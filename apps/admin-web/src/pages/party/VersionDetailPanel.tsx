import type { ReactNode } from 'react';
import { ExternalLink, X } from 'lucide-react';
import { Button, Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { WorkspaceTabLabel } from '../../templates';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails, type TechnicalRow } from '../../components/TechnicalDetails';
import { DetailRow } from './detail-primitives';
import type { HistoryEntry } from './version-history';

/** 详情签按列竖排要的最小形：目录表的列（ListColumn 与 WorkbenchColumn 都够）。 */
export interface DetailColumn<Row> {
  id?: string;
  key?: string;
  header: ReactNode;
  render: (row: Row) => ReactNode;
}

/**
 * 商业版本目录行点开的右栏详情（票 commercial-registers-edit-containers/01，从 VersionDetailDrawer 换容器而来；内容口径照
 * 票 party-commercial-uiux/21）：头部是对象标识、版本号、状态与动作（发新版本、收尾…），两签「详情」把这一行的所有列竖着列全，
 * 「版本历史」列这个对象的各版。住在工作台右栏而不是浮窗：与公司主体、往来单位同一形态，方向键换行、保活、缓存随工作台来。
 *
 * 详情不另写一份按册的转写，直接用目录表自己的列（表头作标签、单元格作值）；操作列不进详情。历史里点某一版就把栏切到那一版。
 */
export function VersionDetailPanel<Row>({
  row,
  columns,
  title,
  subtitle,
  status,
  history,
  currentVersion,
  onSelectVersion,
  actions,
  readOnly = false,
  emptyHistoryNote,
  technical,
  onOpenObjectPage,
  onClose,
}: {
  row: Row;
  columns: DetailColumn<Row>[];
  title: (row: Row) => { objectId: string; version: string };
  subtitle: (row: Row) => string;
  status?: (row: Row) => string | undefined;
  history: (row: Row) => HistoryEntry[];
  currentVersion: (row: Row) => string;
  onSelectVersion: (version: string) => void;
  /** 头部的动作按钮，由页面摆；没有写授予（readOnly）时不摆（ADR-0315）。 */
  actions?: (row: Row) => ReactNode;
  readOnly?: boolean;
  emptyHistoryNote?: string;
  technical?: (row: Row) => TechnicalRow[];
  /** 给了就在头部摆「打开对象页」（票 commercial-version-object-pages/02）；没有自己版本的行（随所属版本发布的声明）不给。 */
  onOpenObjectPage?: () => void;
  onClose: () => void;
}) {
  const head = title(row);
  return (
    <div className="flex h-full w-full flex-col bg-idpxyz-bg" aria-label={`版本 ${head.objectId} ${head.version}`}>
      <div className="space-y-2 border-b border-idpxyz-border px-5 pb-3 pt-4">
        <div className="flex items-start justify-between gap-3">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <h2 className="truncate font-mono type-title font-semibold text-idpxyz-textBright">{head.objectId}</h2>
            <span className="font-mono type-body leading-4 text-idpxyz-textMuted">{head.version}</span>
            {status?.(row) ? <StatusPill code={status(row) as string} /> : null}
          </div>
          <div className="flex shrink-0 items-center gap-1.5">
            {actions && !readOnly ? actions(row) : null}
            {onOpenObjectPage ? (
              <Button className="text-idpxyz-breadcrumb" variant="ghost" size="icon" onClick={onOpenObjectPage} aria-label="打开对象页" title="打开对象页">
                <ExternalLink className="h-4 w-4" aria-hidden="true" />
              </Button>
            ) : null}
            <Button className="text-idpxyz-breadcrumb" variant="ghost" size="icon" onClick={onClose} aria-label="关闭详情" title="关闭（Esc）">
              <X className="h-4 w-4" aria-hidden="true" />
            </Button>
          </div>
        </div>
        <p className="type-body text-idpxyz-textMuted">{subtitle(row)}</p>
      </div>
      <Tabs key={`${head.objectId}@${head.version}`} defaultValue="detail" className="flex min-h-0 flex-1 flex-col gap-0">
        <TabsList className="mx-5 mt-2 h-9 w-auto shrink-0 gap-5">
          <TabsTrigger value="detail">
            <WorkspaceTabLabel id="summary">详情</WorkspaceTabLabel>
          </TabsTrigger>
          <TabsTrigger value="history">
            <WorkspaceTabLabel id="profile-versions">版本历史</WorkspaceTabLabel>
          </TabsTrigger>
        </TabsList>
        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-3">
          <TabsContent value="detail" className="mt-0">
            <dl>
              {columns
                .filter((column) => (column.id ?? column.key) !== 'new-version' && typeof column.header === 'string')
                .map((column) => (
                  <DetailRow key={column.id ?? column.key} label={column.header as string}>
                    {column.render(row)}
                  </DetailRow>
                ))}
            </dl>
            {technical ? <TechnicalDetails className="mt-3" rows={technical(row)} /> : null}
          </TabsContent>
          <TabsContent value="history" className="mt-0">
            <HistoryList entries={history(row)} current={currentVersion(row)} onSelect={onSelectVersion} emptyNote={emptyHistoryNote} />
          </TabsContent>
        </div>
      </Tabs>
    </div>
  );
}

/** 版本历史清单：一版一行，正在查看的那版标出；点另一版切过去。抽屉与右栏共用。 */
export function HistoryList({
  entries,
  current,
  onSelect,
  emptyNote = '这一行随所属版本发布，没有自己的版本历史。',
}: {
  entries: HistoryEntry[];
  current: string;
  onSelect: (version: string) => void;
  emptyNote?: string;
}) {
  if (entries.length === 0) {
    return <p className="py-3 type-body text-idpxyz-textMuted">{emptyNote}</p>;
  }
  return (
    <ol className="py-2 flex flex-col gap-2">
      {entries.map((entry) => (
        <li key={entry.version}>
          <button
            type="button"
            aria-current={entry.version === current ? 'true' : undefined}
            className={`w-full rounded border px-3 py-2 text-left hover:bg-idpxyz-hover ${
              entry.version === current ? 'border-idpxyz-accent' : 'border-idpxyz-border'
            }`}
            onClick={() => onSelect(entry.version)}
          >
            <span className="flex flex-wrap items-center gap-2">
              <span className="font-mono type-value font-medium text-idpxyz-text">{entry.label ?? entry.version}</span>
              {entry.status ? <StatusPill code={entry.status} /> : null}
              {entry.version === current ? <span className="type-caption text-idpxyz-accent">正在查看</span> : null}
            </span>
            <span className="mt-1 block font-mono type-body text-idpxyz-textMuted">{entry.interval}</span>
            {entry.closure ? <span className="mt-0.5 block type-body text-idpxyz-text">{entry.closure}</span> : null}
            {entry.note ? <span className="mt-0.5 block type-body text-idpxyz-text">{entry.note}</span> : null}
          </button>
        </li>
      ))}
    </ol>
  );
}
