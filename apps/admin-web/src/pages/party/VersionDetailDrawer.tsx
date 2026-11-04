import type { ReactNode } from 'react';
import { Drawer, DrawerBody, DrawerHeader, Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { WorkspaceTabLabel, type ListColumn } from '../../templates';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails, type TechnicalRow } from '../../components/TechnicalDetails';
import { DetailRow } from './detail-primitives';
import type { HistoryEntry } from './version-history';
import { HistoryList } from './VersionDetailPanel';

/**
 * 商业版本目录行点开的详情抽屉（票 party-commercial-uiux/21；正按 commercial-registers-edit-containers 逐页换成右栏 VersionDetailPanel，
 * 最后一页换完即删）：两栏，「详情」把这一行的所有列竖着列全，「版本历史」列这个
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
  readOnly = false,
  emptyHistoryNote,
  technical,
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
  /** 操作者没有登记册配置写授予时传 true：不摆头部动作（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。 */
  readOnly?: boolean;
  /** 历史栏为空时的说明；缺省说这一行随所属版本发布。读面不给历史的册（产品—渠道映射）另说。 */
  emptyHistoryNote?: string;
  /** 这一行排查用的字（内容摘要、批准引用、租户……），收进详情签底部的「技术详情」；不给就不摆。 */
  technical?: (row: Row) => TechnicalRow[];
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
              {status?.(row) ? <StatusPill code={status(row) as string} /> : null}
            </div>
            <p className="mt-0.5 text-[12px] text-idpxyz-textMuted">{subtitle(row)}</p>
            {actions && !readOnly ? <div className="mt-2 flex flex-wrap items-center gap-2">{actions(row)}</div> : null}
          </DrawerHeader>
          <DrawerBody>
            <Tabs key={`${title(row).objectId}@${title(row).version}`} defaultValue="detail" className="gap-0">
              {/* 图标取对象工作区签的固定表（票 company-subject-ux/10），词照抽屉的口径：「详情」是这一行的概要，
                  「版本历史」是这个对象的版本链，与公司主体的「资料版本」同一种内容。 */}
              <TabsList>
                <TabsTrigger value="detail">
                  <WorkspaceTabLabel id="summary">详情</WorkspaceTabLabel>
                </TabsTrigger>
                <TabsTrigger value="history">
                  <WorkspaceTabLabel id="profile-versions">版本历史</WorkspaceTabLabel>
                </TabsTrigger>
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
                {technical ? <TechnicalDetails className="mt-3" rows={technical(row)} /> : null}
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

