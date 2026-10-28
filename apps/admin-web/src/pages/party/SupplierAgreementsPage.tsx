import { useEffect, useState } from 'react';
import { Button, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { ListPageTemplate } from '../../templates';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState } from '../catalogue-view';
import { useAddressKeyword } from '../../templates/use-address-keyword';
import { listSupplierAgreements, type SupplierAgreementListResponseBody, type SupplierAgreementRecord } from './api';
import { nextRequest, newVersionFrom, type NewVersionRequest } from './new-version';
import { withNewVersionAction } from './NewVersionAction';
import { useReplaceDraftGuard } from './ReplaceDraftGuard';
import { publicationGroup } from './unsaved-guard';
import { supplierAgreementDraftFromRecord } from './supplier-agreement-form';
import { KeepAliveTabs, KeepAliveTabsContent } from './keep-alive-tabs';
import { VersionStatusChips } from './VersionStatusChips';
import {
  groupedByObject,
  matchesVersionStatus,
  versionCountSummary,
  versionStatusCounts,
  type VersionStatusFilter,
} from './version-groups';
import { useNameDirectory, withRefLinks } from './RefLink';
import { withStatusBadge } from './detail-primitives';
import {
  supplierAgreementColumns,
  supplierAgreementRowsOf,
  type SupplierAgreementRow,
} from './supplier-agreement-rows';
import { SupplierAgreementPublicationForm } from './SupplierAgreementPublicationForm';
import { agreementKnownVersions } from './known-versions';
import { closableKeys, closureKeyOf, closureTargetOf, type ClosureTarget } from './version-closure';
import { VersionClosureDrawer } from './VersionClosureDrawer';
import { VersionDetailDrawer } from './VersionDetailDrawer';
import { versionHistoryOf } from './version-history';
import { useMayWriteRegistry } from '../use-operator-grants';
import { entryHint } from '../operator-grants';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['supplier-agreements'];

// 列与行转写在 supplier-agreement-rows.ts（票 admin-write-faces/19）：版本壳之外，0021 正文逐键上列，正文在不在由
// 服务端的 contentRegistered 说，不拿正文键的有无去推。壳在正文缺是合法状态——壳可先入册，正文随「发布协议版本」
// 签登记——目录里两态各有各的样子（「未登记」/「已登记」），布尔为真而键缺则如实点名为响应不合契约。
// 骨架期这里列过的结算条件仍不上：0021 正文里没有它（结算方式归「商业规则与策略」页的结算政策册），读口没有透，
// 不是转写漏了；采购价格条件在正文里是采购定价方案的引用串，按引用上列、不读方案内容。

// 目录读面。reloadToken 由页面在发布签落定后递增，让同页目录立刻刷新可见（票 admin-write-faces/11 完成判据）；
// 自己的「重试」另有一把键，两把任一变都重取。
function SupplierAgreementCatalogue({
  reloadToken,
  onStartNewVersion,
}: {
  reloadToken: number;
  /** 点行上的「发新版本」：把这一版带去发布签预填（票 party-commercial-uiux/14）。 */
  onStartNewVersion: (record: SupplierAgreementRecord) => void;
}) {
  // 检索词在地址里（`?q=`）：别处的引用链接带着它过来，筛到那一条；刷新与换标签回来也还在。
  const [search, setSearch] = useAddressKeyword();
  const directory = useNameDirectory();
  const [statusFilter, setStatusFilter] = useState<VersionStatusFilter>('all');
  const [reloadKey, setReloadKey] = useState(0);
  // 没有登记册配置写授予的操作者不摆发新版本与收尾（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  const [answer, setAnswer] = useState<ApiResult<SupplierAgreementListResponseBody> | null>(null);
  // 点行上的「收尾…」：记下是哪一版，抽屉据此重读册面（票 party-commercial-uiux/16）。
  const [closing, setClosing] = useState<ClosureTarget | null>(null);
  // 点行打开的版本详情：记行键（票 party-commercial-uiux/21）。
  const [selectedKey, setSelectedKey] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listSupplierAgreements().then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey, reloadToken]);

  const rows = answer?.kind === 'outcome' ? supplierAgreementRowsOf(answer.body) : [];
  const needle = search.trim().toLowerCase();
  // 过滤只在已取回的这一页数据上做，不下推成查询参数——那要改端点契约。搜的是转写后的各格，
  // 正文列（供应商、方案引用）因此也搜得到。
  const searchedRows = needle
    ? rows.filter((row) =>
        Object.values(row.values).some((value) => value.toLowerCase().includes(needle)),
      )
    : rows;
  // 同一协议的各版排在一起、新版在上；「在用」只看服务端答的状态（票 party-commercial-uiux/08）。
  const visibleRows = groupedByObject(
    searchedRows.filter((row) => matchesVersionStatus(row.version, statusFilter)),
    (row) => row.version,
  );
  const retry = () => setReloadKey((value) => value + 1);
  const closable = closableKeys(answer?.kind === 'outcome' ? agreementKnownVersions(answer.body) : []);
  // 目录表与详情抽屉用同一份列（引用链接与状态徽章已套好）。
  const detailColumns = withRefLinks(withStatusBadge(supplierAgreementColumns), directory);
  const selected = rows.find((row) => row.key === selectedKey) ?? null;

  return (
    <>
      <ListPageTemplate<SupplierAgreementRow>
        title={info.title}
        description="供应商协议的各个版本：供应商、责任法人、采购方案、协议范围与有效区间。正文还没随发布登记的版本，「正文」列显示「未登记」。"
        search={{
          value: search,
          onChange: setSearch,
          placeholder: '搜索协议、版本、范围、供应商或方案引用',
        }}
        filterSummary={
          // 计数只在拿到业务答案后显示：未配置态与错误态下报「0 份」会与状态区
          // 「这不是目录为空」直接矛盾（README 列表页上列通则「计数摘要必须带 outcome 守卫」）。
          answer?.kind === 'outcome' ? versionCountSummary(rows.length, visibleRows.length, '份', '协议版本') : undefined
        }
        filters={
          answer?.kind === 'outcome' && rows.length > 0 ? (
            <VersionStatusChips
              counts={versionStatusCounts(rows, (row) => row.version)}
              value={statusFilter}
              onChange={setStatusFilter}
            />
          ) : undefined
        }
        emptyRowsNote="这组条件下没有匹配的协议版本"
        columns={withNewVersionAction(detailColumns, {
          enabled: mayWrite,
          labelOf: (row) => `供应商协议 ${row.version.objectId} ${row.version.version}`,
          onStart: (row) => onStartNewVersion(row.source),
          closure: {
            canClose: (row) => closable.has(closureKeyOf(row.source)),
            onClose: (row) => setClosing(closureTargetOf('SUPPLIER_AGREEMENT', '供应商协议', row.source)),
          },
        })}
        rows={visibleRows}
        rowKey={(row) => row.key}
        onRowClick={(row) => setSelectedKey(row.key)}
        viewState={catalogueViewState(answer, rows.length, retry, {
          module: info,
          endpoint: 'GET /commercial-supplier-agreements',
          emptyTitle: '当前租户尚无供应商协议版本',
          emptyDescription: `还没有任何供应商协议；本页不会预置供应商或采购条件。${entryHint(mayWrite, '在「发布协议版本」签发布第一份。')}`,
        })}
      />
      <VersionDetailDrawer<SupplierAgreementRow>
        row={selected}
        columns={detailColumns}
        title={(row) => ({ objectId: row.source.objectId, version: row.source.version })}
        subtitle={(row) => `供应商协议 · 适用范围 ${row.source.scope}`}
        status={(row) => row.source.status}
        history={(row) => versionHistoryOf(rows.map((candidate) => candidate.source), row.source)}
        currentVersion={(row) => row.source.version}
        onSelectVersion={(version) => {
          const next = rows.find(
            (row) =>
              row.source.objectId === selected?.source.objectId &&
              row.source.scope === selected?.source.scope &&
              row.source.version === version,
          );
          if (next) setSelectedKey(next.key);
        }}
        readOnly={!mayWrite}
        actions={(row) => (
          <>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setSelectedKey(null);
                onStartNewVersion(row.source);
              }}
            >
              发新版本
            </Button>
            {closable.has(closureKeyOf(row.source)) ? (
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setSelectedKey(null);
                  setClosing(closureTargetOf('SUPPLIER_AGREEMENT', '供应商协议', row.source));
                }}
              >
                收尾…
              </Button>
            ) : null}
          </>
        )}
        onClose={() => setSelectedKey(null)}
      />
      <VersionClosureDrawer target={closing} onClose={() => setClosing(null)} onClosed={retry} />
    </>
  );
}

/**
 * 供应商协议（party-commercial）。行对象是供应商商业协议版本：它定义可复用的
 * 采购条件、价格和结算责任，不等于一次实际运输委托、订舱、履约事实或供应商
 * 账单——实际委托快照归 transport-fulfillment，成本与应付归 settlement-accounting。
 *
 * 两签：目录读面，与「发布协议版本」的逐字段表单（票 admin-write-faces/11；ADR-0101 决定八）。写签跟着读签走：
 * 发布落定的版本就在左签目录里，页面借 reloadToken 让它立刻可见。受控发布的 JSON 镜像签留在「商业规则与策略」页
 * 作高级口，本页不重复摆。
 */
export function SupplierAgreementsPage() {
  const mayWrite = useMayWriteRegistry();
  const [reloadToken, setReloadToken] = useState(0);
  // 点协议行上的「发新版本」：记下是哪一版，切到发布签；发布表单据此预填并重建（票 party-commercial-uiux/14）。
  const [placedTab, setTab] = useState('catalogue');
  // 答案到达之前入口照摆，到达后才知道没有登记写授予：停在被收起的签上的话落回读签，不留一块空白。
  const tab = !mayWrite && placedTab === 'publish' ? 'catalogue' : placedTab;
  const [request, setRequest] = useState<NewVersionRequest<SupplierAgreementRecord> | null>(null);
  const beginNewVersion = (record: SupplierAgreementRecord) => {
    setRequest((previous) => nextRequest(previous, record));
    setTab('publish');
  };
  // 发布签里已填未发布的草稿要被换掉时先问一句（票 party-commercial-uiux/17）。
  const replaceGuard = useReplaceDraftGuard<SupplierAgreementRecord>(
    () => publicationGroup('SUPPLIER_AGREEMENT'),
    beginNewVersion,
    '发布协议版本',
  );
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      {replaceGuard.dialog}
      <KeepAliveTabs value={tab} onValueChange={setTab} className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="catalogue">协议目录</TabsTrigger>
          {mayWrite ? <TabsTrigger value="publish">发布协议版本</TabsTrigger> : null}
        </TabsList>
        <KeepAliveTabsContent
          value="catalogue"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <SupplierAgreementCatalogue reloadToken={reloadToken} onStartNewVersion={replaceGuard.start} />
        </KeepAliveTabsContent>
        {mayWrite ? (
          <KeepAliveTabsContent
            value="publish"
            className="flex-1 flex flex-col overflow-auto data-[state=inactive]:hidden"
          >
            <SupplierAgreementPublicationForm
              // 同一版再发起一次、或换一版，表单整个重建，不留上一次的预览与草稿。
              key={request?.token ?? 0}
              onPublished={() => setReloadToken((value) => value + 1)}
              start={
                request
                  ? {
                      draft: supplierAgreementDraftFromRecord(request.source),
                      from: newVersionFrom('供应商协议', request.source.objectId, request.source.version),
                    }
                  : undefined
              }
            />
          </KeepAliveTabsContent>
        ) : null}
      </KeepAliveTabs>
    </div>
  );
}
