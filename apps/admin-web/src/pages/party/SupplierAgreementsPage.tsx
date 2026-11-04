import { useEffect, useRef, useState } from 'react';
import { useRetryWhenChanged } from '../../query/answers';
import { useRegisterList } from './register-list';
import { Button, ConfirmDialog } from '@idpxyz/ui-primitives';
import { WorkbenchPageTemplate, type WorkbenchColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import { primaryActionClass } from '../../components/primary-action';
import { catalogueViewState } from '../catalogue-view';
import { useAddressKeyword } from '../../templates/use-address-keyword';
import { useOwnAddressChange } from '../../templates/use-own-address-change';
import { listSupplierAgreements, type SupplierAgreementRecord } from './api';
import { nextRequest, newVersionFrom, type NewVersionRequest } from './new-version';
import { useReplaceDraftGuard } from './ReplaceDraftGuard';
import { hasUnsavedDraft, publicationGroup } from './unsaved-guard';
import { supplierAgreementDraftFromRecord } from './supplier-agreement-form';
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
  supplierAgreementTechnicalRows,
  type SupplierAgreementRow,
} from './supplier-agreement-rows';
import { SupplierAgreementPublicationForm } from './SupplierAgreementPublicationForm';
import { agreementKnownVersions } from './known-versions';
import { closableKeys, closureKeyOf, closureTargetOf, type ClosureTarget } from './version-closure';
import { VersionClosureDialog } from './VersionClosureDialog';
import { VersionDetailPanel } from './VersionDetailPanel';
import { RegisterView } from './RegisterView';
import { supplierAgreementsViewHash, supplierAgreementsViewOf, type SupplierAgreementsView } from './supplier-agreements-address';
import { versionHistoryOf } from './version-history';
import { useMayWriteRegistry, useRegistryWriteGrant } from '../use-operator-grants';
import { entryHint } from '../operator-grants';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['supplier-agreements'];

// 列与行转写在 supplier-agreement-rows.ts（票 admin-write-faces/19）：版本壳之外，0021 正文逐键上列，正文在不在由
// 服务端的 contentRegistered 说，不拿正文键的有无去推。壳在正文缺是合法状态——壳可先入册，正文随「签订协议」登记视图登记——
// 目录里两态各有各的样子（「未登记」/「已登记」），布尔为真而键缺则如实点名为响应不合契约。
// 骨架期这里列过的结算条件仍不上：0021 正文里没有它（结算方式归「商业规则与策略」页的结算政策册），读口没有透，
// 不是转写漏了；采购价格条件在正文里是采购定价方案的引用串，按引用上列、不读方案内容。

// 右栏打开时收起的次要列：窄栏里仍读得清协议、供应商、我方与状态。
const hiddenWhenDetailOpen = new Set(['purchasePlan', 'agreementEffective', 'contentRegistered']);

// 目录读面。reloadToken 由页面在登记视图落定后递增，让回到目录时立刻刷新可见（票 admin-write-faces/11 完成判据）；
// 自己的「重试」另有一把键，两把任一变都重取。
function SupplierAgreementWorkbench({
  reloadToken,
  onStartNewVersion,
  onOpenPublish,
}: {
  reloadToken: number;
  /** 右栏头部的「签新版本」：把这一版带去登记视图预填（票 party-commercial-uiux/14）。 */
  onStartNewVersion: (record: SupplierAgreementRecord) => void;
  onOpenPublish: () => void;
}) {
  // 检索词在地址里（`?q=`）：别处的引用链接带着它过来，筛到那一条；刷新与换标签回来也还在。
  const [search, setSearch] = useAddressKeyword();
  const directory = useNameDirectory();
  const [statusFilter, setStatusFilter] = useState<VersionStatusFilter>('all');
  // 没有登记册配置写授予的操作者不摆发新版本与收尾（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  const { answer, retry, pending } = useRegisterList(listSupplierAgreements);
  useRetryWhenChanged(reloadToken, retry);
  // 右栏里的「收尾…」：记下是哪一版，对话框据此重读册面（票 party-commercial-uiux/16）。
  const [closing, setClosing] = useState<ClosureTarget | null>(null);
  // 右栏里的那一行：行键，与详情抽屉时代同一个键（票 party-commercial-uiux/21）。
  const [selectedKey, setSelectedKey] = useState<string | null>(null);

  const rows = answer?.kind === 'outcome' ? supplierAgreementRowsOf(answer.body) : [];
  const needle = search.trim().toLowerCase();
  // 过滤只在已取回的这一页数据上做，不下推成查询参数——那要改端点契约。搜的是转写后的各格，
  // 正文列（供应商、方案引用）因此也搜得到。
  const searchedRows = needle
    ? rows.filter((row) => Object.values(row.values).some((value) => value.toLowerCase().includes(needle)))
    : rows;
  // 同一协议的各版排在一起、新版在上；「在用」只看服务端答的状态（票 party-commercial-uiux/08）。
  const visibleRows = groupedByObject(
    searchedRows.filter((row) => matchesVersionStatus(row.version, statusFilter)),
    (row) => row.version,
  );
  const closable = closableKeys(answer?.kind === 'outcome' ? agreementKnownVersions(answer.body) : []);
  // 目录表与右栏详情用同一份列（引用链接与状态徽章已套好）；工作台的列按 key 认，照列的 id 搬过来。
  const detailColumns = withRefLinks(withStatusBadge(supplierAgreementColumns), directory);
  const workbenchColumns: WorkbenchColumn<SupplierAgreementRow>[] = detailColumns.map((column) => ({
    key: column.id,
    header: column.header,
    render: column.render,
    hideWhenDetailOpen: hiddenWhenDetailOpen.has(column.id),
  }));
  const answered = answer?.kind === 'outcome';

  return (
    <>
      <WorkbenchPageTemplate<SupplierAgreementRow>
        title={info.title}
        viewStateKey="supplier-agreements"
        subtitle="与承运商、代理等供应商签的协议：谁、按哪份采购价格表、有效到什么时候；要改条款就签新版本"
        onRefresh={retry}
        refreshing={pending}
        primaryAction={
          mayWrite ? (
            <Button variant="default" className={primaryActionClass} onClick={onOpenPublish}>
              签订协议
            </Button>
          ) : undefined
        }
        search={{ value: search, onChange: setSearch, placeholder: '搜索协议、供应商或价格表' }}
        extraFilters={
          answered && rows.length > 0 ? (
            <VersionStatusChips counts={versionStatusCounts(rows, (row) => row.version)} value={statusFilter} onChange={setStatusFilter} />
          ) : undefined
        }
        // 计数只在拿到业务答案后显示：未配置态与错误态下报「0 份」会与状态区「这不是目录为空」直接矛盾。
        toolbarRight={answered ? versionCountSummary(rows.length, visibleRows.length, '份', '协议版本') : undefined}
        columns={workbenchColumns}
        rows={visibleRows}
        rowKey={(row) => row.key}
        selectedKey={selectedKey}
        onSelect={setSelectedKey}
        noMatchText="这组条件下没有匹配的协议版本"
        onClearFilters={() => {
          setSearch('');
          setStatusFilter('all');
        }}
        renderDetail={(key) => {
          const row = rows.find((candidate) => candidate.key === key);
          return row ? (
            <VersionDetailPanel<SupplierAgreementRow>
              row={row}
              columns={detailColumns}
              title={(record) => ({ objectId: record.source.objectId, version: record.source.version })}
              subtitle={(record) => `供应商协议 · ${record.values.contentRegistered === '已登记' ? '正文已登记' : record.values.contentRegistered}`}
              technical={supplierAgreementTechnicalRows}
              status={(record) => record.source.status}
              history={(record) => versionHistoryOf(rows.map((candidate) => candidate.source), record.source)}
              currentVersion={(record) => record.source.version}
              onSelectVersion={(version) => {
                const next = rows.find(
                  (candidate) =>
                    candidate.source.objectId === row.source.objectId &&
                    candidate.source.scope === row.source.scope &&
                    candidate.source.version === version,
                );
                if (next) setSelectedKey(next.key);
              }}
              readOnly={!mayWrite}
              actions={(record) => (
                <>
                  <Button variant="outline" size="sm" className="text-[12px]" onClick={() => onStartNewVersion(record.source)}>
                    签新版本
                  </Button>
                  {closable.has(closureKeyOf(record.source)) ? (
                    <Button
                      variant="outline"
                      size="sm"
                      className="text-[12px]"
                      onClick={() => setClosing(closureTargetOf('SUPPLIER_AGREEMENT', '供应商协议', record.source))}
                    >
                      收尾…
                    </Button>
                  ) : null}
                </>
              )}
              onClose={() => setSelectedKey(null)}
            />
          ) : null;
        }}
        viewState={catalogueViewState(answer, rows.length, retry, {
          module: info,
          endpoint: 'GET /commercial-supplier-agreements',
          emptyTitle: '还没有签订任何供应商协议',
          emptyDescription: `找承运商、代理走货要先有协议。${entryHint(mayWrite, '用页头的「签订协议」签第一份。')}`,
        })}
      />
      <VersionClosureDialog target={closing} onClose={() => setClosing(null)} onClosed={retry} />
    </>
  );
}

/**
 * 供应商协议（party-commercial）。行对象是供应商商业协议版本：它定义可复用的采购条件、价格和结算责任，不等于一次实际运输
 * 委托、订舱、履约事实或供应商账单——实际委托快照归 transport-fulfillment，成本与应付归 settlement-accounting。
 *
 * 容器照 commercial-registers-edit-containers 的决定：目录是工作台、行详情在右栏，「签订协议」是由地址决定的整页登记视图
 * （`?tab=publish`），收尾在对话框。写签跟着读签走：发布落定的版本就在目录里，页面借 reloadToken 让它回到目录时立刻可见。
 * 受控发布的 JSON 镜像口留在「商业规则与策略」页作高级口，本页不重复摆。
 */
export function SupplierAgreementsPage() {
  const grant = useRegistryWriteGrant();
  const mayWrite = grant.mayWrite;
  const [reloadToken, setReloadToken] = useState(0);
  // 地址是位置的唯一权威：视图从 hash 判（supplier-agreements-address.ts），只读本标签名下的地址变化。
  const [hash, setHash] = useState(() => window.location.hash);
  useOwnAddressChange(() => setHash(window.location.hash));
  const requested = supplierAgreementsViewOf(hash);
  // 答案到达之前入口照摆，到达后才知道没有登记写授予：停在登记视图上的话落回目录，不留一块空白。
  const view: SupplierAgreementsView = mayWrite ? requested : 'catalogue';
  // 查看者拿着登记视图的地址进来：授予已答出、明确没有写授予时把键清掉（替换地址）。加载期不动。
  useEffect(() => {
    if (requested === 'publish' && grant.settled && !grant.mayWrite) {
      const next = supplierAgreementsViewHash(window.location.hash, 'catalogue');
      window.location.replace(next);
      setHash(next);
    }
  }, [requested, grant.settled, grant.mayWrite]);
  // 本次挂载里是不是从目录压进登记视图的：是就用后退回目录；刷新或从别的标签直接落在视图上的用替换。
  const pushedView = useRef(false);
  const openPublish = () => {
    pushedView.current = true;
    window.location.hash = supplierAgreementsViewHash(window.location.hash, 'publish');
  };
  const [confirmingLeave, setConfirmingLeave] = useState(false);
  const leaveView = () => {
    setConfirmingLeave(false);
    if (pushedView.current) {
      pushedView.current = false;
      window.history.back();
      return;
    }
    const next = supplierAgreementsViewHash(window.location.hash, 'catalogue');
    window.location.replace(next);
    setHash(next);
  };
  // 登记视图里的发布表单有没提交的输入时先问一句（票 party-commercial-uiux/17 的登记簿）。
  const requestLeave = () => (hasUnsavedDraft(publicationGroup('SUPPLIER_AGREEMENT')) ? setConfirmingLeave(true) : leaveView());

  // 右栏「签新版本」：记下是哪一版，进登记视图；发布表单据此预填并重建（票 party-commercial-uiux/14）。
  const [request, setRequest] = useState<NewVersionRequest<SupplierAgreementRecord> | null>(null);
  const beginNewVersion = (record: SupplierAgreementRecord) => {
    setRequest((previous) => nextRequest(previous, record));
    if (view !== 'publish') openPublish();
  };
  // 登记视图里已填未发布的草稿要被换掉时先问一句（票 party-commercial-uiux/17）。
  const replaceGuard = useReplaceDraftGuard<SupplierAgreementRecord>(
    () => publicationGroup('SUPPLIER_AGREEMENT'),
    beginNewVersion,
    '签订协议',
  );

  if (view === 'publish') {
    return (
      <RegisterView title="签订协议" subtitle="供应商、我方公司主体、采购价格表与有效期；表单 → 预览 → 存为待批准 → 批准 → 发布" onBack={requestLeave}>
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
        <ConfirmDialog
          renderFooter={confirmDialogFooter}
          open={confirmingLeave}
          tone="warning"
          title="离开这一页？"
          message="有内容还没有发布，离开后填写的内容不会保留。"
          confirmLabel="离开"
          cancelLabel="继续填写"
          onConfirm={leaveView}
          onCancel={() => setConfirmingLeave(false)}
        />
      </RegisterView>
    );
  }

  return (
    <>
      {replaceGuard.dialog}
      <SupplierAgreementWorkbench reloadToken={reloadToken} onStartNewVersion={replaceGuard.start} onOpenPublish={openPublish} />
    </>
  );
}
