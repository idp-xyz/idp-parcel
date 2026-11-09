import { useEffect, useState } from 'react';
import { partyAnswersDomain } from './party-writes';
import { useAnswer } from '../../query/answers';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { CursorPager, pastFirstPage } from '../shipment-request/cursor-pager';
import { Button, ConfirmDialog } from '@idpxyz/ui-primitives';
import { WorkbenchPageTemplate, type WorkbenchColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { RegistrationPanel } from '../../components/registration';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import { primaryActionClass } from '../../components/primary-action';
import { catalogueViewState, formatInstant, formatRange } from '../catalogue-view';
import { useAddressKeyword } from '../../templates/use-address-keyword';
import { useOwnAddressChange } from '../../templates/use-own-address-change';
import { usePushedAddress } from '../../templates/use-pushed-address';
import {
  commercialRegistrationEndpoints,
  listProductChannelMappingRevisions,
  listProductChannelMappings,
  productChannelOutcomeLabels,
  registerCommercial,
  type ProductChannelMappingListResponseBody,
  type ProductChannelMappingRecord,
  type ProductChannelMappingRevisionListResponseBody,
} from './api';
import { RefLink } from './RefLink';
import { VersionDetailPanel, type DetailColumn } from './VersionDetailPanel';
import { RegisterView } from './RegisterView';
import { mappingHistoryOf } from './mapping-history';
import { problemNote, registrationSnapshotHints, registrationTitles, updateOrdinalLabel } from './presentation';
import { registrationGroup, useUnsavedGuard } from './unsaved-guard';
import { useRegistryWriteGrant } from '../use-operator-grants';
import { entryHint } from '../operator-grants';
import {
  channelProductCatalogViewHash,
  channelProductCatalogViewOf,
  type ChannelProductCatalogView,
} from './channel-product-catalog-address';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['channel-product-catalog'];

// 行对象是产品—渠道映射的最新登记修订（票 admin-remainder-mechanism-batch/02）。
// 骨架期这里列过渠道服务方与商业适用性——渠道产品目录身份正文不在本上下文预造
// （ADR-0072：渠道本体等 PAR-INT-01 的接入证据），映射册只持标识引用，接线时按
// 「不以空列伪装已实现」撤下，渠道本体列随渠道接入另立。
// 主列只留运营要看的：服务产品、能走的渠道、有效期、登记时间；映射标识、修订号与登记依据进右栏详情的「技术详情」。
const columns: DetailColumn<ProductChannelMappingRecord>[] = [
  {
    id: 'product',
    header: '服务产品',
    render: (row) => (
      <div className="min-w-44">
        <p>
          <RefLink kind="service-product" id={row.productObjectId} />
        </p>
        <p className="mt-0.5 font-mono type-body leading-4 text-idpxyz-textMuted">{row.productVersionLabel}</p>
      </div>
    ),
  },
  {
    id: 'channels',
    header: '可走的渠道',
    // 空数组是显式登记的“未配置”声明（该产品尚无可用渠道候选），如实显示这句话
    // 而不是留白——留白读起来像数据缺件，而这格恰恰是登记者说出的内容。
    render: (row) =>
      row.channels.length > 0 ? (
        <div className="min-w-44 font-mono type-body leading-4">
          {row.channels.map((channel) => (
            <p key={channel}>{channel}</p>
          ))}
        </div>
      ) : (
        <span className="text-idpxyz-textMuted">未配置：这个产品还没有可用渠道</span>
      ),
  },
  {
    id: 'effective',
    header: '有效期',
    render: (row) => <span className="font-mono type-body leading-4">{formatRange(row.effectiveStartsAt, row.effectiveEndsAt)}</span>,
  },
  {
    id: 'registered-at',
    header: '登记时间',
    render: (row) => <span className="font-mono type-body leading-4">{formatInstant(row.registeredAt)}</span>,
  },
];
// 工作台的列按 key 认，照上面的 id 搬；右栏开时只留服务产品与渠道两列。
const workbenchColumns: WorkbenchColumn<ProductChannelMappingRecord>[] = columns.map((column, index) => ({
  key: column.id ?? String(index),
  header: column.header,
  render: column.render,
  hideWhenDetailOpen: index >= 2,
}));

/**
 * 渠道产品目录（party-commercial）。行对象是产品—渠道映射的最新登记修订：服务产品
 * 版本 × 渠道产品标识引用 × 有效区间。映射只定义新渠道决策的候选范围，不把候选
 * 伪装成实际选择——某次交易实际用了哪个渠道由拥有该交易的上下文记录；调整绑定或
 * 区间形成新修订，不覆盖历史。
 *
 * 目录是工作台，行详情在右栏（票 commercial-registers-edit-containers/05）；右栏里点历史的某一笔，详情换成那一笔修订。
 *
 * 读口已迁到 ADR-0144（票 catalogue-read-pagination-rollout/11）：检索词沿地址上的 `?q=` 停稳后下推，翻页走游标，换检索词回第一页；
 * 次序是读口的缺省序，本册不设筛选维。检索覆盖哪几格以读口为准。
 */
function ProductChannelMappingWorkbench({ mayWrite, onOpenRegister }: { mayWrite: boolean; onOpenRegister: () => void }) {
  // 检索词在地址里（`?q=`）：别处的引用链接带着它过来，筛到那一条；刷新与换标签回来也还在。
  const [search, setSearch] = useAddressKeyword();
  // 点行打开的映射详情：记映射标识（票 party-commercial-uiux/21）。
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const conditions = { q: useSettledKeyword(search) };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listProductChannelMappings);

  const mappings = answer?.kind === 'outcome' ? answer.body.mappings : [];

  // 历史里点的是哪一笔修订；没点就看最新一笔（目录行本身）。
  const [revisionNo, setRevisionNo] = useState<number | null>(null);
  // 点开一行才读这个映射的整条修订链；键带映射标识，换行就是另一份，上一行的答案后到也落不到这一行上。
  const revisions = useAnswer(
    [partyAnswersDomain, 'product-channel-mapping', selectedId ?? '', 'revisions'],
    () => listProductChannelMappingRevisions(selectedId ?? ''),
    { enabled: selectedId !== null },
  ).answer;
  const chain =
    revisions?.kind === 'outcome' && revisions.body.mappingId === selectedId ? revisions.body.revisions : null;
  const historyNote =
    revisions === null
      ? '正在读取变更记录…'
      : chain === null
        ? '没能读到变更记录，稍后重新打开这个产品的渠道设置再看。'
        : '这个产品的渠道设置还没有改过。';
  const select = (key: string | null) => {
    setSelectedId(key);
    setRevisionNo(null);
  };

  return (
    <WorkbenchPageTemplate<ProductChannelMappingRecord>
      title={info.title}
      viewStateKey="channel-product-catalog"
      // 渠道本体还没有读口，渠道只能按标识显示。
      subtitle="每个服务产品能走哪些渠道平台的产品；下单择优只在这些渠道里选"
      onRefresh={retry}
      refreshing={answer === null}
      primaryAction={
        mayWrite ? (
          <Button variant="default" className={primaryActionClass} onClick={onOpenRegister}>
            设置产品渠道
          </Button>
        ) : undefined
      }
      search={{ value: search, onChange: setSearch, placeholder: '搜索服务产品或渠道' }}
      toolbarRight={pagination ? <CursorPager pagination={pagination} /> : undefined}
      columns={workbenchColumns}
      rows={mappings}
      rowKey={(row) => row.mappingId}
      selectedKey={selectedId}
      onSelect={select}
      noMatchText="没有匹配的产品渠道设置"
      onClearFilters={() => setSearch('')}
      renderDetail={(key) => {
        const latest = mappings.find((row) => row.mappingId === key);
        if (!latest) return null;
        const selected = (revisionNo !== null ? chain?.find((row) => row.revision === revisionNo) : null) ?? latest;
        return (
          <VersionDetailPanel<ProductChannelMappingRecord>
            row={selected}
            columns={columns}
            title={(row) => ({ objectId: row.productObjectId, version: row.productVersionLabel })}
            subtitle={(row) => `产品渠道设置 · ${updateOrdinalLabel(row.revision)}`}
            technical={(row) => [
              { label: '映射标识', value: row.mappingId },
              { label: '修订号', value: String(row.revision) },
              { label: '产品标识', value: row.productObjectId },
              { label: '登记依据', value: row.basis },
            ]}
            history={() => (chain ? mappingHistoryOf(chain) : [])}
            currentVersion={(row) => String(row.revision)}
            onSelectVersion={(version) => setRevisionNo(Number(version))}
            emptyHistoryNote={historyNote}
            onClose={() => select(null)}
          />
        );
      }}
      viewState={catalogueViewState(answer, mappings.length, retry, {
        module: info,
        endpoint: 'GET /commercial-product-channel-mappings',
        emptyTitle: '还没有给任何产品设置渠道',
        emptyDescription: `产品没有可走的渠道，渠道择优就无从选起。${entryHint(mayWrite, '用页头的「设置产品渠道」登记第一条。')}`,
        narrowed: catalogueConditionsNarrow(conditions) || pastFirstPage(pagination),
      })}
    />
  );
}

/**
 * 渠道产品目录：查阅映射册，外加「设置产品渠道」登记视图（ADR-0085，票 admin-write-faces/02 切片 02c）。
 *
 * 登记视图不是「编辑映射」的表单：映射按修订推进，调整绑定或区间翻旧插新、不覆盖行，
 * 改指产品则是另一笔映射——所以这里只有登记一个动作，没有行级编辑或删除面。墙降之前
 * 它必然答 403「接入渠道未配置」，那是诚实答案；墙降当天在装配点换真 Intake 即点亮，
 * 本页一行不用改。
 *
 * 视图由地址决定（`?tab=register`，键沿用此前那个签的值；票 commercial-registers-edit-containers/05）：进视图压一条历史、
 * 出来后退或替换；快照框里有没提交的输入时「返回列表」先问。
 */
export function ChannelProductCatalogPage() {
  // 没有登记册配置写授予的操作者不摆登记入口（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const grant = useRegistryWriteGrant();
  const mayWrite = grant.mayWrite;
  // 地址是位置的唯一权威：视图从 hash 判，只读本标签名下的地址变化。
  const [hash, setHash] = useState(() => window.location.hash);
  useOwnAddressChange(() => setHash(window.location.hash));
  const requested = channelProductCatalogViewOf(hash);
  // 答案到达之前入口照摆，到达后才知道没有登记写授予：停在登记视图上的话落回目录，不留一块空白。
  const view: ChannelProductCatalogView = mayWrite ? requested : 'catalogue';
  // 查看者拿着登记视图的地址进来：授予已答出、明确没有写授予时把键清掉（替换地址）。加载期不动。
  useEffect(() => {
    if (requested === 'register' && grant.settled && !grant.mayWrite) {
      const next = channelProductCatalogViewHash(window.location.hash, 'catalogue');
      window.location.replace(next);
      setHash(next);
    }
  }, [requested, grant.settled, grant.mayWrite]);
  // 从目录压进登记视图的用后退回目录；刷新、深链、或压完之后地址被别处动过的（切过标签）用替换——判法在 usePushedAddress。
  const pushed = usePushedAddress();
  const openRegister = () => pushed.push(channelProductCatalogViewHash(window.location.hash, 'register'));
  const [confirmingLeave, setConfirmingLeave] = useState(false);
  const leaveView = () => {
    setConfirmingLeave(false);
    if (pushed.back()) return;
    const next = channelProductCatalogViewHash(window.location.hash, 'catalogue');
    window.location.replace(next);
    setHash(next);
  };
  // 快照框报上来的「有没提交的输入」：记进登记簿（刷新、关页也会被拦），「返回列表」前照它问。
  const [unsaved, setUnsaved] = useState(false);
  useUnsavedGuard(registrationGroup('product-channel-mapping'), view === 'register' && unsaved);
  const requestLeave = () => (unsaved ? setConfirmingLeave(true) : leaveView());

  if (view === 'register') {
    return (
      <RegisterView title={registrationTitles['product-channel-mapping']} subtitle={registrationSnapshotHints['product-channel-mapping']} onBack={requestLeave}>
        <RegistrationPanel
          moduleId="channel-product-catalog"
          title={registrationTitles['product-channel-mapping']}
          endpoint={`POST ${commercialRegistrationEndpoints['product-channel-mapping']}`}
          snapshotHint={registrationSnapshotHints['product-channel-mapping']}
          submit={(snapshot) => registerCommercial('product-channel-mapping', snapshot)}
          outcomeLabels={productChannelOutcomeLabels}
          problemNote={problemNote}
          onUnsavedChange={setUnsaved}
        />
        <ConfirmDialog
          renderFooter={confirmDialogFooter}
          open={confirmingLeave}
          tone="warning"
          title="离开这一页？"
          message="有内容还没有提交，离开后填写的内容不会保留。"
          confirmLabel="离开"
          cancelLabel="继续填写"
          onConfirm={leaveView}
          onCancel={() => setConfirmingLeave(false)}
        />
      </RegisterView>
    );
  }

  return <ProductChannelMappingWorkbench mayWrite={mayWrite} onOpenRegister={openRegister} />;
}
