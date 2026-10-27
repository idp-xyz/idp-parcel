import type { ReactNode } from 'react';
import { Drawer, DrawerBody, DrawerHeader, Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import type { ListColumn } from '../../templates';
import { DetailRow, statusBadge } from './detail-primitives';
import { commercialStatusLabels } from './presentation';
import type { HistoryEntry } from './version-history';

/**
 * 商业版本目录行点开的详情抽屉（票 party-commercial-uiux/21）：两栏，「详情」把这一行的所有列竖着列全，「版本历史」列这个
 * 对象的各个版本。
 *
 * 详情不另写一份按册的转写，直接用目录表自己的列（表头作标签、单元格作值）：表格里窄列放不下、被截断的长内容在这里竖排
 * 摊开，引用的链接与状态徽章照旧；表上有的这里都有，不会两处各说各的。操作列（发新版本）不进详情，那几个动作由页面经
 * `actions` 交进抽屉头部。历史里点某一版就把抽屉切到那一版的详情。
 */
export function VersionDetailDrawer<Row>({
  row,
  columns,
  title,
  subtitle,
  status,
  history,
  currentVersion,
  onSelectVersion,
  actions,
  emptyHistoryNote,
  onClose,
}: {
  row: Row | null;
  /** 目录表的列（含引用链接、状态徽章的转写），操作列由本组件略去。 */
  columns: ListColumn<Row>[];
  /** 头部大字：对象标识与版本号。 */
  title: (row: Row) => { objectId: string; version: string };
  /** 头部小字，例「客户合同 · 适用范围 SYN-SCOPE-01」。 */
  subtitle: (row: Row) => string;
  /** 服务端状态码；读面没有状态的册不给。 */
  status?: (row: Row) => string | undefined;
  /** 同一对象的各版，新的在前；没有历史的行（随版本发布的声明册）给空数组。 */
  history: (row: Row) => HistoryEntry[];
  currentVersion: (row: Row) => string;
  onSelectVersion: (version: string) => void;
  /** 头部的动作按钮（发新版本、收尾……），由页面摆。 */
  actions?: (row: Row) => ReactNode;
  /** 历史栏为空时的说明；缺省说这一行随所属版本发布。读面不给历史的册（产品—渠道映射）另说。 */
  emptyHistoryNote?: string;
  onClose: () => void;
}) {
  return (
    <Drawer open={row !== null} onOpenChange={(open) => (open ? undefined : onClose())} aria-label="版本详情">
      {row ? (
        <>
          <DrawerHeader>
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-mono font-medium text-idpxyz-text">{title(row).objectId}</span>
              <span className="font-mono text-xs text-idpxyz-textMuted">{title(row).version}</span>
              {status?.(row) ? statusBadge(commercialStatusLabels, status(row) as string) : null}
            </div>
            <p className="mt-0.5 text-[12px] text-idpxyz-textMuted">{subtitle(row)}</p>
            {actions ? <div className="mt-2 flex flex-wrap items-center gap-2">{actions(row)}</div> : null}
          </DrawerHeader>
          <DrawerBody>
            <Tabs key={`${title(row).objectId}@${title(row).version}`} defaultValue="detail" className="gap-0">
              <TabsList>
                <TabsTrigger value="detail">详情</TabsTrigger>
                <TabsTrigger value="history">版本历史</TabsTrigger>
              </TabsList>
              <TabsContent value="detail">
                <dl>
                  {columns
                    .filter((column) => column.id !== 'new-version' && typeof column.header === 'string')
                    .map((column) => (
                      <DetailRow key={column.id} label={column.header as string}>
                        {column.render(row)}
                      </DetailRow>
                    ))}
                </dl>
              </TabsContent>
              <TabsContent value="history">
                <HistoryList
                  entries={history(row)}
                  current={currentVersion(row)}
                  onSelect={onSelectVersion}
                  emptyNote={emptyHistoryNote}
                />
              </TabsContent>
            </Tabs>
          </DrawerBody>
        </>
      ) : null}
    </Drawer>
  );
}

function HistoryList({
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
    return <p className="py-3 text-[12px] text-idpxyz-textMuted">{emptyNote}</p>;
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
              <span className="font-mono text-[13px] font-medium text-idpxyz-text">{entry.label ?? entry.version}</span>
              {entry.status ? statusBadge(commercialStatusLabels, entry.status) : null}
              {entry.version === current ? <span className="text-[11px] text-idpxyz-accent">正在查看</span> : null}
            </span>
            <span className="mt-1 block font-mono text-[12px] text-idpxyz-textMuted">{entry.interval}</span>
            {entry.closure ? <span className="mt-0.5 block text-[12px] text-idpxyz-text">{entry.closure}</span> : null}
            {entry.note ? <span className="mt-0.5 block text-[12px] text-idpxyz-text">{entry.note}</span> : null}
          </button>
        </li>
      ))}
    </ol>
  );
}
