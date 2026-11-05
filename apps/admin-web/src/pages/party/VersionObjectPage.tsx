import type { ReactNode } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { DetailPageTemplate } from '../../templates';
import type { TemplateViewState } from '../../templates/state-slot';
import { StatusPill } from '../../components/StatusPill';
import type { TechnicalRow } from '../../components/TechnicalDetails';
import { DetailRow } from './detail-primitives';
import { HistoryList, type DetailColumn } from './VersionDetailPanel';
import type { HistoryEntry } from './version-history';

/**
 * 商业版本的对象页（票 commercial-version-object-pages/02）：与右栏详情 VersionDetailPanel 同一份内容——壳（标识、版本、状态）、
 * 这一版的各格（目录表的列竖排）、版本历史、技术详情——只换容器：整页、有自己的地址，能另开标签、深链与分享。
 * 内容从按身份读回的那一版来（新鲜），历史从目录来（同一对象的各版在目录里）；两者的列与转写都是目录那一套，不另写。
 *
 * 动作由页面摆（收尾…）；「发新版本」的预填状态住在目录页，这里只有「在目录中查看」把人带回去。
 */
export function VersionObjectPage<Row>({
  kindLabel,
  objectId,
  version,
  row,
  columns,
  status,
  technical,
  history,
  onSelectVersion,
  actions,
  viewState,
  onBack,
}: {
  kindLabel: string;
  objectId: string;
  version: string;
  /** 读回的那一版，按目录行的形转写好；没读到时为 null，页面按 viewState 显状态。 */
  row: Row | null;
  columns: DetailColumn<Row>[];
  status?: (row: Row) => string | undefined;
  technical?: (row: Row) => TechnicalRow[];
  history: HistoryEntry[];
  onSelectVersion: (version: string) => void;
  actions?: ReactNode;
  viewState: TemplateViewState;
  onBack: () => void;
}) {
  const fields = row
    ? columns
        .filter((column) => (column.id ?? column.key) !== 'new-version' && typeof column.header === 'string')
        .map((column) => ({ label: column.header as string, value: column.render(row) }))
    : [];
  return (
    <DetailPageTemplate
      title={kindLabel}
      identifier={`${objectId} ${version}`}
      status={row && status?.(row) ? <StatusPill code={status(row) as string} /> : undefined}
      headerActions={
        <>
          {actions}
          <Button variant="outline" size="sm" className="type-body text-idpxyz-breadcrumb" onClick={onBack}>
            在目录中查看
          </Button>
        </>
      }
      basicTitle="这一版"
      basicFields={fields}
      sections={
        row
          ? [
              {
                id: 'history',
                title: '版本历史',
                content: <HistoryList entries={history} current={version} onSelect={onSelectVersion} />,
              },
              // 整页有的是地方：技术详情直接摊开成一区，不再折叠（右栏里折叠是因为栏窄）。
              ...(technical
                ? [
                    {
                      id: 'technical',
                      title: '技术详情',
                      content: (
                        <dl>
                          {technical(row).map((entry) => (
                            <DetailRow key={entry.label} label={entry.label} mono>
                              {entry.value}
                            </DetailRow>
                          ))}
                        </dl>
                      ),
                    },
                  ]
                : []),
            ]
          : []
      }
      viewState={viewState}
    />
  );
}
