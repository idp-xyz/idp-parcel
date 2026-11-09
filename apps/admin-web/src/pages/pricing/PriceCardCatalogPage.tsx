import { useEffect, useState } from 'react';
import { Button, Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { WorkbenchPageTemplate, type WorkbenchChip, type WorkbenchColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { RegistrationPanel } from '../../components/registration';
import { StatusPill } from '../../components/StatusPill';
import { priceDirectionLabels, priceCardPurposeLabels } from '../../vocabulary/price-card';
import { useAddressKeyword } from '../../templates/use-address-keyword';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { catalogueViewState } from '../catalogue-view';
import { InstantRange } from '../party/detail-primitives';
import { CursorPager } from '../shipment-request/cursor-pager';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { useMayWriteRegistry } from '../use-operator-grants';
import { PriceCardEditorPanel } from './card-editor/PriceCardEditorPanel';
import { PriceCardDraftsPanel } from './PriceCardDraftsPanel';
import { PriceCardImportPanel } from './PriceCardImportPanel';
import { PriceCardDetailPanel, PriceCardUnits } from './PriceCardDetailPanel';
import { problemNote } from './presentation';
import { priceCardStatus, priceCardStatusLabel } from './price-card-content';
import {
  priceCardListConditions,
  priceCardNoMatchNote,
  priceCardRowKey,
  priceCardsOfStatus,
  priceCardStatusFilterOptions,
  priceCardStatusNoMatchNote,
  priceCardStatusScopeNote,
  type PriceCardStatusFilter,
} from './price-card-list';
import { listPriceCards, registerPriceCard, registrationOutcomeLabels, type PriceCardRecord } from './api';

const info = moduleInfoById['price-card-catalog'];

// 列只留运营认得出一张卡所需的几样：方案、方向、范围、币种与单位、适用期、状态。标识之外的系统引用（规范化版本、内容摘要、
// 源文件身份、发布授权、登记责任方）都不在列上，进右栏底部的「技术详情」。
const columns: WorkbenchColumn<PriceCardRecord>[] = [
  {
    key: 'plan',
    header: '价卡',
    render: (row) => (
      <div className="min-w-36">
        <div className="break-all font-medium text-[color:var(--ds-text-link-hover)]">{row.planId}</div>
        <div className="mt-0.5 type-body text-idpxyz-breadcrumb">版本 {row.planVersion}</div>
      </div>
    ),
  },
  {
    key: 'direction',
    header: '方向',
    render: (row) => (
      <div className="whitespace-nowrap">
        <div>{priceDirectionLabels[row.direction] ?? row.direction}</div>
        <div className="type-body text-idpxyz-breadcrumb">{priceCardPurposeLabels[row.purpose] ?? row.purpose}</div>
      </div>
    ),
  },
  { key: 'scope', header: '适用范围', render: (row) => <span className="break-all">{row.scope}</span> },
  {
    key: 'units',
    header: '币种与单位',
    hideWhenDetailOpen: true,
    render: (row) => <PriceCardUnits row={row} />,
  },
  {
    key: 'period',
    header: '适用期',
    hideWhenDetailOpen: true,
    render: (row) => (
      <span className="font-mono type-body">
        <InstantRange from={row.effectiveFrom} to={row.effectiveTo} />
      </span>
    ),
  },
  {
    key: 'status',
    header: '状态',
    render: (row) => {
      const status = priceCardStatusLabel(priceCardStatus(row, new Date()));
      return <StatusPill label={status.label} tone={status.tone} />;
    },
  },
];

/**
 * 价格表：已登记价卡版本的工作台。单击一行在右栏看这张卡有哪些费用项、每项怎么收、什么时候收（价卡内容读口）。
 * 新建、导入、草稿与高级 JSON 四签在同页其余签里。
 *
 * 读口已迁到 ADR-0144，按游标翻页（工具条右端）。检索词（地址上的 `?q=`，停稳后）下推成 q，覆盖方案标识、版本与适用范围；
 * 次序是读口的缺省序（新登在前）。状态胶囊一半下推：读口只认退役与否（retired），「已退役」按整本册筛；其余四态是页面拿此刻现算的，
 * 先下推 retired=false、再只筛已取回的这一页，旁边照实写这几格只筛本页，也不带计数（price-card-list 的 priceCardListConditions）。
 * 换胶囊即换条件，回第一页。读口另有方向一维，本页没有对应的筛选条，不另加。导入签与草稿签今天在发行方未设时答 403
 * 「接入渠道未配置」，那是诚实答案，页面不拿演示数据顶替。
 */
function PriceCardCatalogTable({ mayWrite, onCreate }: { mayWrite: boolean; onCreate: () => void }) {
  const [search, setSearch] = useAddressKeyword();
  const [status, setStatus] = useState<PriceCardStatusFilter>('ALL');
  const conditions = priceCardListConditions(useSettledKeyword(search), status);
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listPriceCards);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);

  const rows = answer?.kind === 'outcome' ? answer.body.cards : [];
  const visible = priceCardsOfStatus(rows, status, new Date());
  const answered = answer?.kind === 'outcome';
  const scopeNote = priceCardStatusScopeNote(answered, rows.length, visible.length, status);

  const statusChips: WorkbenchChip[] = priceCardStatusFilterOptions().map((option) => ({
    id: option.value,
    label: option.label,
    active: status === option.value,
    onClick: () => setStatus(option.value === 'ALL' || status === option.value ? 'ALL' : option.value),
  }));

  return (
    <WorkbenchPageTemplate<PriceCardRecord>
      title={info.title}
      subtitle="对客户的售价与对供应商的成本价：点开一张价卡，看它收哪些费、怎么收、什么时候收"
      onRefresh={retry}
      refreshing={answer === null}
      search={{ value: search, onChange: setSearch, placeholder: '搜价卡编号或适用范围…' }}
      statusChips={statusChips}
      extraFilters={scopeNote ? <span className="shrink-0 type-caption text-idpxyz-textMuted">{scopeNote}</span> : undefined}
      toolbarRight={pagination ? <CursorPager pagination={pagination} /> : undefined}
      primaryAction={
        mayWrite ? (
          <Button variant="default" className={primaryActionClass} onClick={onCreate}>
            新建价卡
          </Button>
        ) : undefined
      }
      columns={columns}
      rows={visible}
      rowKey={priceCardRowKey}
      selectedKey={selectedKey}
      onSelect={setSelectedKey}
      // 这一页有行却被页内那四态筛空，说的只是这一页；读口按检索词与退役与否答出零行才是这组条件下没有。
      noMatchText={rows.length > 0 ? priceCardStatusNoMatchNote : priceCardNoMatchNote}
      onClearFilters={() => {
        setSearch('');
        setStatus('ALL');
      }}
      renderDetail={(key) => {
        const row = visible.find((card) => priceCardRowKey(card) === key);
        return row ? <PriceCardDetailPanel row={row} onClose={() => setSelectedKey(null)} /> : null;
      }}
      // 筛出为空不是空态：viewState 按这一页答回的行数与下推的条件判，表格区另显 noMatchText。
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /pricing-price-cards',
        emptyTitle: '还没有价卡',
        emptyDescription: mayWrite
          ? '点「新建价卡」逐项填写，或在「导入价卡」签上传 Excel；批准、发布后就会出现在这里。'
          : '请有价卡维护权限的同事新建或导入价卡；批准、发布后就会出现在这里。',
        narrowed: catalogueConditionsNarrow(conditions),
      })}
    />
  );
}

export function PriceCardCatalogPage() {
  // 没有登记册配置写授予的操作者不摆「新建价卡」与导入入口：写口照旧自己判授予，这里只是呈现（ADR-0315）。
  const mayWrite = useMayWriteRegistry();
  const [tab, setTab] = useState('catalog');
  // 授予答出来之后才知道没有写授予：停在「新建价卡」的话回到目录，不留一个空签。
  // 导入签与高级签同属写入口，一并收起；草稿签是查阅面，留着（批准 / 发布按钮在签内另按授予收起）。
  useEffect(() => {
    if (!mayWrite && (tab === 'create' || tab === 'import' || tab === 'advanced')) setTab('catalog');
  }, [mayWrite, tab]);
  // 「新建价卡」存下草稿后让「草稿与审批」签重取一次：签的内容不卸载，不通知就停在旧册上。
  const [draftsSignal, setDraftsSignal] = useState(0);
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs value={tab} onValueChange={setTab} className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="catalog">全部价卡</TabsTrigger>
          {mayWrite ? <TabsTrigger value="create">新建价卡</TabsTrigger> : null}
          {mayWrite ? <TabsTrigger value="import">导入价卡</TabsTrigger> : null}
          <TabsTrigger value="drafts">草稿与审批</TabsTrigger>
          {mayWrite ? <TabsTrigger value="advanced">高级：JSON 导入</TabsTrigger> : null}
        </TabsList>
        <TabsContent
          value="catalog"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <PriceCardCatalogTable mayWrite={mayWrite} onCreate={() => setTab('create')} />
        </TabsContent>
        {mayWrite ? (
          // forceMount：切到别的签时表单不卸载，填了一半的内容与预览结果留着，切回来接着填。
          <TabsContent
            value="create"
            forceMount
            className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
          >
            <PriceCardEditorPanel onOpenDrafts={() => setTab('drafts')} onDraftSaved={() => setDraftsSignal((value) => value + 1)} />
          </TabsContent>
        ) : null}
        {mayWrite ? (
          <TabsContent
            value="import"
            className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
          >
            <PriceCardImportPanel />
          </TabsContent>
        ) : null}
        <TabsContent
          value="drafts"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <PriceCardDraftsPanel refreshSignal={draftsSignal} />
        </TabsContent>
        {mayWrite ? (
          <TabsContent
            value="advanced"
            className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
          >
            {/* 形状与受控登记口 parcel-pricing-register -kind price-card 同一份，由产品定义（ADR-0101）。 */}
            <RegistrationPanel
              moduleId="price-card-catalog"
              title="高级：用 JSON 直接登记价卡"
              endpoint="POST /pricing-price-card-registrations"
              snapshotHint="给系统对接或批量迁移用：贴入整份价卡 JSON 直接登记，跳过草稿与审批，格式与命令行导入工具相同。日常新建或修改价卡，请用「新建价卡」或「导入价卡」。"
              submit={registerPriceCard}
              outcomeLabels={registrationOutcomeLabels}
              problemNote={problemNote}
            />
          </TabsContent>
        ) : null}
      </Tabs>
    </div>
  );
}
