import { useEffect, useState } from 'react';
import { useAnswer, useRetryWhenChanged } from '../../query/answers';
import { partyAnswersDomain } from './party-writes';
import { commercialVersionReadbackEndpointNote, readCommercialVersion, readVersionOf } from './commercial-version-api';
import { VersionObjectPage } from './VersionObjectPage';
import { versionObjectHash, versionObjectOf, type VersionObjectAddress } from './version-object-address';
import { ADDRESS_KEYWORD_PARAM, hashWithQueryValue } from '../../templates/address-query';
import { useRegisterList } from './register-list';
import { Button, ConfirmDialog, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { ListPageTemplate, WorkbenchPageTemplate, useAddressKeyword, type ListColumn, type WorkbenchColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { CustomerAccountRegistrationForm } from './CustomerAccountRegistrationForm';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import { primaryActionClass } from '../../components/primary-action';
import { useOwnAddressChange } from '../../templates/use-own-address-change';
import { usePushedAddress } from '../../templates/use-pushed-address';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { CustomerContractPublicationForm } from './CustomerContractPublicationForm';
import { nextRequest, newVersionFrom, type NewVersionRequest } from './new-version';
import { useReplaceDraftGuard } from './ReplaceDraftGuard';
import { hasUnsavedDraft, publicationGroup, registrationGroup } from './unsaved-guard';
import { contractKnownVersions } from './known-versions';
import { closableKeys, closureKeyOf, closureTargetOf, type ClosureTarget } from './version-closure';
import { VersionClosureDialog } from './VersionClosureDialog';
import { VersionDetailPanel } from './VersionDetailPanel';
import { RegisterView } from './RegisterView';
import {
  isPartyContractsRegisterView,
  partyContractsListOf,
  partyContractsViewHash,
  partyContractsViewOf,
  type PartyContractsView,
} from './party-contracts-address';
import { versionHistoryOf } from './version-history';
import { customerContractDraftFromRecord } from './customer-contract-form';
import { KeepAliveTabs, KeepAliveTabsContent } from './keep-alive-tabs';
import { VersionStatusChips } from './VersionStatusChips';
import {
  groupedByObject,
  versionListConditions,
  versionPoolOf,
  type VersionFacts,
  type VersionStatusFilter,
} from './version-groups';
import { CursorPager, pastFirstPage } from '../shipment-request/cursor-pager';
import { RefLink } from './RefLink';
import { loadCustomerAccountSuggestions } from './reference-inputs';
import { useLoaded } from './PublicationFormFields';
import { StatusPill } from '../../components/StatusPill';
import { DeliveryConditionsCell } from './DeliveryConditionsCell';
import { catalogueViewState, formatInstant, formatRange } from '../catalogue-view';
import {
  listBusinessParties,
  listCustomerAccounts,
  listCustomerContracts,
  type CustomerAccountRecord,
  type CustomerContractListResponseBody,
  type CustomerContractRecord,
} from './api';
import { labelOf, partyNameUnknownNote, registrationTitles } from './presentation';

// 抽屉副题里给状态补一句「所以呢」：运营看到「已退役」要知道的是它不再用于新单。
const contractStatusNotes: Record<string, string> = {
  DRAFT: '还没发布，不作数',
  PUBLISHED: '已发布，到生效时间起作数',
  EFFECTIVE: '现在按这一版办',
  EXPIRED: '已到期，新单不再按它办',
  RETIRED: '已退役，新单不再按它办',
  SUPERSEDED: '已被新版本替代',
};
import { useMayWriteRegistry, useRegistryWriteGrant } from '../use-operator-grants';
import { entryHint } from '../operator-grants';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['party-contracts'];

// 合同读面带出正文里指名的货主客户账户（customerAccountId）；责任法人合同自己没有，随接单规则包（规则包行上有）。账户的客户
// 名称取自客户账户册的第一页，读不到就只显账户标识，不猜名字。账户册自己在本页「客户账户」签。
// 主列只留运营要看的：合同与版本、客户、状态、规则与约定、有效期；适用范围、各标识进右栏详情的「技术详情」。
// 右栏打开时收起次要列（hideWhenDetailOpen），窄栏里仍读得清主身份与状态。
const columns: WorkbenchColumn<CustomerContractRecord>[] = [
  {
    key: 'contract',
    header: '合同',
    render: (row) => (
      <div className="min-w-40">
        <p className="font-mono font-medium text-idpxyz-text">{row.objectId}</p>
        <p className="mt-0.5 type-body leading-4 text-idpxyz-textMuted">版本 {row.version}</p>
      </div>
    ),
  },
  {
    key: 'status',
    header: '状态',
    render: (row) => <StatusPill code={row.status} />,
  },
  {
    key: 'rule-package',
    header: '接单规则包',
    // 正文未登记时不显示空白：空白读起来像「没引用规则包」，而合同正文一旦登记
    // 规则包必存（库上 NOT NULL），两者是不同的事实。
    render: (row) =>
      row.contentRegistered ? (
        row.rulePackageId ? <RefLink kind="rule-package" id={row.rulePackageId} /> : '—'
      ) : (
        <span className="font-sans text-idpxyz-textMuted">正文未登记</span>
      ),
  },
  {
    key: 'bindings',
    header: '接单前财务控制',
    hideWhenDetailOpen: true,
    render: (row) => <ControlBindings row={row} />,
  },
  {
    // 合同层交付条件（0030，票 admin-write-faces/25）：与正文是两层各自可缺，这一格不看 contentRegistered，只看键在不在。
    key: 'delivery-conditions',
    header: '交付条件',
    hideWhenDetailOpen: true,
    render: (row) => <DeliveryConditionsCell conditions={row.deliveryConditions} />,
  },
  {
    key: 'effective',
    header: '有效期',
    hideWhenDetailOpen: true,
    render: (row) => <span className="font-mono type-body leading-4">{formatRange(row.effectiveStartsAt, row.effectiveEndsAt)}</span>,
  },
  {
    key: 'published-at',
    header: '发布时间',
    hideWhenDetailOpen: true,
    render: (row) => <span className="font-mono type-body leading-4">{formatInstant(row.publishedAt)}</span>,
  },
];

const contractFacts = (row: CustomerContractRecord): VersionFacts => ({
  objectId: row.objectId,
  version: row.version,
  status: row.status,
  effectiveStartsAt: row.effectiveStartsAt,
});

// 零绑定有两种，说法必须分开：没有正文行是「正文未登记」（去登记正文），有正文行
// 而零子行是「已登记，未作约定」（无事可做，且该范围答「不存在」而不是「不适用」）。
// 服务端为此专门给了 contentRegistered，页面照它分，不看数组长度。
function ControlBindings({ row }: { row: CustomerContractRecord }) {
  if (!row.contentRegistered) {
    return <span className="text-idpxyz-textMuted">正文未登记</span>;
  }
  if (row.bindings.length === 0) {
    return <span className="text-idpxyz-textMuted">未对任何费用范围作约定</span>;
  }
  return (
    <ul className="space-y-0.5">
      {row.bindings.map((binding) => (
        <li key={binding.chargeScope} className="type-body leading-4">
          <span className="font-mono text-idpxyz-text">{binding.chargeScope}</span>
          <span className="text-idpxyz-textMuted"> · </span>
          {binding.policyId ? (
            <RefLink kind="financial-control-policy" id={binding.policyId} />
          ) : (
            <span className="text-idpxyz-textMuted">
              不适用（{binding.inapplicabilityBasis}）
            </span>
          )}
        </li>
      ))}
    </ul>
  );
}

/** 「客户」一列：客户名称为主（取自账户册已取回的那一页），账户编码为辅；取不到名称时只显编码，不猜。 */
function customerAccountColumn(names: ReadonlyMap<string, string>): WorkbenchColumn<CustomerContractRecord> {
  return {
    key: 'customer-account',
    header: '客户',
    render: (row) => {
      if (row.customerAccountId) {
        const name = names.get(row.customerAccountId);
        return (
          <div className="min-w-40">
            {name ? <p className="font-medium text-idpxyz-text">{name}</p> : null}
            <p className={`font-mono ${name ? 'mt-0.5 text-idpxyz-textMuted' : 'text-idpxyz-text'}`}>{row.customerAccountId}</p>
          </div>
        );
      }
      return <span className="font-sans text-idpxyz-textMuted">{row.contentRegistered ? '—' : '正文未登记'}</span>;
    },
  };
}

/**
 * 客户合同版本册：合同版本到期或被后续版本替代，不改变已经接受委托所保存的合同依据。
 * 目录是工作台，行详情在右栏（票 commercial-registers-edit-containers/02）；发布走「签订合同」登记视图，那一视图的载体到达
 * 发布那一步时由页面把 `publishedKey` 拨一格，本表随之重读——回到目录（含绑定列）立刻可见。
 */
/** 目录表与详情共用的列：合同列之后插客户列（账户标识旁的客户名称取账户册的第一页，读不到就没有名称）。 */
function useContractColumns(): WorkbenchColumn<CustomerContractRecord>[] {
  const accountsAnswer = useLoaded(loadCustomerAccountSuggestions);
  const accountNames = new Map(
    accountsAnswer?.kind === 'outcome'
      ? accountsAnswer.body.accounts.flatMap((account) =>
          account.customerPartyNameKnown && account.customerPartyName ? [[account.accountId, account.customerPartyName] as const] : [],
        )
      : [],
  );
  // 客户列紧跟合同列：运营先认合同、再认客户；按列标识找位置，不按下标数。
  const afterContract = columns.findIndex((column) => column.key === 'contract') + 1;
  return [...columns.slice(0, afterContract), customerAccountColumn(accountNames), ...columns.slice(afterContract)];
}

/** 一版客户合同的对象页（票 commercial-version-object-pages/02）：内容按身份读回，历史与可收尾判断从目录来。 */
function CustomerContractObjectPage({ objectId, version }: VersionObjectAddress) {
  const mayWrite = useMayWriteRegistry();
  const contractColumns = useContractColumns();
  const readback = useAnswer(
    [partyAnswersDomain, 'commercial-version', 'CUSTOMER_CONTRACT', objectId, version],
    () => readCommercialVersion<CustomerContractRecord>('CUSTOMER_CONTRACT', objectId, version),
    { fresh: true },
  );
  const row = readVersionOf(readback.answer);
  const list = useRegisterList(listCustomerContracts);
  const contracts = list.answer?.kind === 'outcome' ? list.answer.body.contracts : [];
  const closable = closableKeys(list.answer?.kind === 'outcome' ? contractKnownVersions(list.answer.body) : []);
  const [closing, setClosing] = useState<ClosureTarget | null>(null);
  const historyPool =
    row && !contracts.some((candidate) => candidate.objectId === row.objectId && candidate.version === row.version) ? [...contracts, row] : contracts;
  const refresh = () => {
    readback.retry();
    list.retry();
  };
  return (
    <>
      <VersionObjectPage<CustomerContractRecord>
        kindLabel="客户合同"
        objectId={objectId}
        version={version}
        row={row}
        columns={contractColumns}
        status={(record) => record.status}
        technical={(record) => [
          { label: '合同标识', value: record.objectId },
          { label: '版本号', value: record.version },
          { label: '适用范围', value: record.scope },
          { label: '客户账户标识', value: record.customerAccountId ?? '' },
          { label: '接单规则包标识', value: record.rulePackageId ?? '' },
          { label: '正文登记时间', value: record.declaredAt ?? '' },
        ]}
        history={row ? versionHistoryOf(historyPool, row) : []}
        onSelectVersion={(next) => {
          window.location.hash = versionObjectHash('party-contracts', objectId, next);
        }}
        actions={
          row && mayWrite && closable.has(closureKeyOf(row)) ? (
            <Button variant="outline" size="sm" className="type-body" onClick={() => setClosing(closureTargetOf('CUSTOMER_CONTRACT', '客户合同', row))}>
              收尾…
            </Button>
          ) : undefined
        }
        viewState={catalogueViewState(readback.answer, row ? 1 : 0, readback.retry, {
          module: info,
          endpoint: commercialVersionReadbackEndpointNote,
          emptyTitle: '没有这一版',
          emptyDescription: '这个合同标识与版本号在本租户的册上不存在；地址可能抄错，或这一版尚未发布。',
        })}
        onBack={() => {
          window.location.hash = hashWithQueryValue('#/party-contracts', ADDRESS_KEYWORD_PARAM, objectId);
        }}
      />
      <VersionClosureDialog target={closing} onClose={() => setClosing(null)} onClosed={refresh} />
    </>
  );
}

function CustomerContractsWorkbench({
  publishedKey,
  onStartNewVersion,
  onOpenView,
}: {
  publishedKey: number;
  /** 右栏头部的「签新版本」：把这一版带去登记视图预填（票 party-commercial-uiux/14）。 */
  onStartNewVersion: (record: CustomerContractRecord) => void;
  onOpenView: (view: PartyContractsView) => void;
}) {
  // 检索词在地址里（`?q=`）：别处的引用链接带着它过来，筛到那一条。与客户账户签共用同一个检索词——它们在一页上。
  const [statusFilter, setStatusFilter] = useState<VersionStatusFilter>('all');
  const [search, setSearch] = useAddressKeyword();
  // 没有登记册配置写授予的操作者不摆发新版本与收尾（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  // 读口已迁到 ADR-0144（票 catalogue-read-pagination-rollout/11）：检索词停稳后下推，「在用 / 其余」下推成 status 维，按合同号
  // 翻页（version-groups.ts）；换条件回第一页。检索覆盖哪几格以读口为准。
  const conditions = versionListConditions(useSettledKeyword(search), statusFilter);
  const narrowed = catalogueConditionsNarrow(conditions);
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listCustomerContracts);
  // 这一页可能带着条件、也只是一页，缺同一合同被筛掉或落在页外的版本：右栏的历史与「收尾…」再并进不带条件的那一页（取数层
  // 缓存，发布表单与收尾对话框读的是同一份）。写口答到后那一页随参与方整域作废自己重取，游标这一页由写后的 retry 重取。
  const unfiltered = useRegisterList(listCustomerContracts);
  useRetryWhenChanged(publishedKey, retry);
  const contractColumns = useContractColumns();
  // 右栏里的「收尾…」：记下是哪一版，对话框据此重读册面（票 party-commercial-uiux/16）。
  const [closing, setClosing] = useState<ClosureTarget | null>(null);
  // 右栏里的那一行：行键，与详情抽屉时代同一个键（票 party-commercial-uiux/21）。
  const [selectedKey, setSelectedKey] = useState<string | null>(null);

  const contracts = answer?.kind === 'outcome' ? answer.body.contracts : [];
  const unfilteredBody = unfiltered.answer?.kind === 'outcome' ? unfiltered.answer.body : null;
  const keyOf = (row: CustomerContractRecord) => `${row.objectId}@${row.version}`;
  // 同一合同的各版排在一起、新版在上；「在用」只看服务端答的状态（票 party-commercial-uiux/08）。
  const visibleContracts = groupedByObject(contracts, contractFacts);
  const pool = versionPoolOf(contracts, unfilteredBody?.contracts ?? [], keyOf);
  const closable = closableKeys([
    ...(answer?.kind === 'outcome' ? contractKnownVersions(answer.body) : []),
    ...(unfilteredBody ? contractKnownVersions(unfilteredBody) : []),
  ]);

  return (
    <>
      <WorkbenchPageTemplate<CustomerContractRecord>
        title={info.title}
        viewStateKey="party-contracts"
        subtitle="每份客户合同签了什么、什么时候生效：客户、接单规则、财务控制约定与交付条件；要改条款就签新版本"
        onRefresh={retry}
        refreshing={answer === null}
        primaryAction={
          mayWrite ? (
            <Button variant="default" className={primaryActionClass} onClick={() => onOpenView('publish-contract')}>
              签订合同
            </Button>
          ) : undefined
        }
        search={{ value: search, onChange: setSearch, placeholder: '搜索合同、客户账户或规则包' }}
        // 胶囊常摆：筛空了也要能点回「全部」。
        extraFilters={<VersionStatusChips value={statusFilter} onChange={setStatusFilter} />}
        toolbarRight={pagination ? <CursorPager pagination={pagination} /> : undefined}
        columns={contractColumns}
        rows={visibleContracts}
        rowKey={keyOf}
        selectedKey={selectedKey}
        onSelect={setSelectedKey}
        noMatchText="这组条件下没有匹配的合同版本"
        onClearFilters={() => {
          setSearch('');
          setStatusFilter('all');
        }}
        renderDetail={(key) => {
          const row = pool.find((candidate) => keyOf(candidate) === key);
          return row ? (
            <VersionDetailPanel<CustomerContractRecord>
              row={row}
              columns={contractColumns}
              title={(record) => ({ objectId: record.objectId, version: record.version })}
              subtitle={(record) => `客户合同 · ${labelOf(contractStatusNotes, record.status)}`}
              technical={(record) => [
                { label: '合同标识', value: record.objectId },
                { label: '版本号', value: record.version },
                { label: '适用范围', value: record.scope },
                { label: '客户账户标识', value: record.customerAccountId ?? '' },
                { label: '接单规则包标识', value: record.rulePackageId ?? '' },
                { label: '正文登记时间', value: record.declaredAt ?? '' },
              ]}
              status={(record) => record.status}
              history={(record) => versionHistoryOf(pool, record)}
              currentVersion={(record) => record.version}
              onSelectVersion={(version) => {
                const next = pool.find(
                  (candidate) => candidate.objectId === row.objectId && candidate.scope === row.scope && candidate.version === version,
                );
                if (next) setSelectedKey(keyOf(next));
              }}
              readOnly={!mayWrite}
              onOpenObjectPage={() => {
                window.location.hash = versionObjectHash('party-contracts', row.objectId, row.version);
              }}
              actions={(record) => (
                <>
                  <Button variant="outline" size="sm" className="type-body" onClick={() => onStartNewVersion(record)}>
                    签新版本
                  </Button>
                  {closable.has(closureKeyOf(record)) ? (
                    <Button
                      variant="outline"
                      size="sm"
                      className="type-body"
                      onClick={() => setClosing(closureTargetOf('CUSTOMER_CONTRACT', '客户合同', record))}
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
        viewState={catalogueViewState(answer, contracts.length, retry, {
          module: info,
          endpoint: 'GET /commercial-customer-contracts',
          emptyTitle: '还没有签订任何客户合同',
          emptyDescription: `客户下单要按合同办。${entryHint(mayWrite, '用页头的「签订合同」签第一份。')}`,
          narrowed: narrowed || pastFirstPage(pagination),
        })}
      />
      <VersionClosureDialog target={closing} onClose={() => setClosing(null)} onClosed={retry} />
    </>
  );
}

// 客户账户册的列，与法人册同形：账户钉在一个客户参与方身份上，名称从参与方册转写；
// 停用两件（时点 + 依据）与状态同格——停用签的快照每项必填 basis，读面若只显时点，
// 写进去的依据就无处可见，停用签也就不能按「写签跟着读签走」落在这一页。
const accountColumns: ListColumn<CustomerAccountRecord>[] = [
  {
    id: 'customer-party',
    header: '客户',
    // 名称在参与方册上（账户不抄第二份）；customerPartyNameKnown 为假是写入门失败才会
    // 出现的悬空引用，如实标出让人去查写侧，不补占位文本冒充名称。
    render: (row) => (
      <div className="min-w-36">
        {row.customerPartyNameKnown ? (
          <p className="font-medium text-idpxyz-text">{row.customerPartyName}</p>
        ) : (
          <p className="type-body leading-4 text-idpxyz-textMuted">{partyNameUnknownNote}</p>
        )}
        <p className="mt-0.5 font-mono type-body leading-4 text-idpxyz-textMuted">{row.customerPartyId}</p>
      </div>
    ),
  },
  {
    id: 'account',
    header: '账户编码',
    className: 'font-mono type-body leading-4',
    render: (row) => row.accountId,
  },
  {
    id: 'status',
    header: '状态',
    align: 'center',
    render: (row) => (
      <div>
        <p>
          <StatusPill code={row.status} />
        </p>
        {row.deactivatedAt ? (
          <p className="mt-0.5 font-mono type-body leading-4 text-idpxyz-textMuted">
            自 {formatInstant(row.deactivatedAt)}
          </p>
        ) : null}
        {row.deactivationBasis ? (
          <p className="mt-0.5 font-mono type-body leading-4 text-idpxyz-textMuted">{row.deactivationBasis}</p>
        ) : null}
      </div>
    ),
  },
  {
    id: 'effective-from',
    header: '生效时间',
    className: 'min-w-44 font-mono type-body leading-4',
    render: (row) => formatInstant(row.effectiveFrom),
  },
  {
    id: 'registered-at',
    header: '登记时间',
    className: 'min-w-44 font-mono type-body leading-4',
    render: (row) => formatInstant(row.registeredAt),
  },
];

/**
 * 货主客户账户册（票 admin-write-faces/04）。行对象是账户的最新登记修订：账户是运营集团
 * 租户内面向一个货主客户建立的业务隔离与商业关系边界（ADR-0003 三级边界的第三级），必须
 * 显式关联其客户参与方，名称从参与方册转写；登记按修订版本化不可覆盖，停用形成新修订。
 *
 * 落在本页而不是业务参与方页：那页的所有权语言是「角色中立的业务参与方身份」，而账户
 * 引用一个身份、定义里就带着角色（货主客户）——与责任法人落自己的页同一条理由。本页的
 * 所有权语言原句第一项就是「客户账户」，合同的相对方也正是它。
 *
 * 读口已迁到 ADR-0144（票 catalogue-read-pagination/02、05）：检索词沿地址上的 `?q=` 停稳后下推，翻页走游标，换检索词回第一页；
 * 次序是读口的缺省序，本签不另排。不在答回来的那一页上再筛——截断之后再筛，答出的「没有」可能是假的。检索覆盖哪几格以读口为准。
 */
function CustomerAccountsTable({ onOpenView }: { onOpenView: (view: PartyContractsView) => void }) {
  const mayWrite = useMayWriteRegistry();
  const [search, setSearch] = useAddressKeyword();
  const conditions = { q: useSettledKeyword(search) };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listCustomerAccounts);
  const accounts = answer?.kind === 'outcome' ? answer.body.accounts : [];

  return (
    <ListPageTemplate<CustomerAccountRecord>
      title={info.title}
      description="每个客户账户属于哪家客户、什么时候生效；运单、合同与对账都挂在账户上"
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索客户或账户编码',
      }}
      filterSummary={answer?.kind === 'outcome' ? `本页 ${accounts.length} 个客户账户` : undefined}
      headerActions={
        mayWrite ? (
          <Button variant="default" className={primaryActionClass} onClick={() => onOpenView('register')}>
            新增客户账户
          </Button>
        ) : undefined
      }
      columns={accountColumns}
      rows={accounts}
      rowKey={(row) => row.accountId}
      emptyRowsNote="当前检索条件下没有匹配的客户账户"
      pagination={pagination}
      viewState={catalogueViewState(answer, accounts.length, retry, {
        module: info,
        endpoint: 'GET /commercial-customer-accounts',
        emptyTitle: '还没有登记任何客户账户',
        emptyDescription: `客户下单、对账都要先有账户。${entryHint(mayWrite, '用页头的「新增客户账户」登记第一个。')}`,
        narrowed: catalogueConditionsNarrow(conditions),
      })}
    />
  );
}

const viewTitles: Record<'publish-contract' | 'register', { title: string; subtitle: string }> = {
  'publish-contract': { title: '签订合同', subtitle: '客户、接单规则、财务控制约定与交付条件；表单 → 预览 → 存为待批准 → 批准 → 发布' },
  register: { title: registrationTitles['customer-account'], subtitle: '账户钉在一个客户参与方身份上，名称从往来单位转写' },
};

/**
 * 客户与合同（party-commercial）：合同版本册与货主客户账户册两本列表分签，外加「签订合同」「新增客户账户」两个登记视图。
 *
 * 两册同页分签而不并表：合同是商业版本（草稿→发布→退役），账户是参与方身份（登记→生效→停用），两套状态代数并进一张表，
 * 同一个「已生效」会在两种含义间相互冒充。
 *
 * 容器照 commercial-registers-edit-containers 的决定：合同行详情在工作台右栏（VersionDetailPanel），两张表单是由地址决定的整页
 * 登记视图（RegisterView，`?tab=publish-contract` / `?tab=register`），不再是列表旁边的签；收尾在对话框。两本列表之间换签只替换
 * 地址；进登记视图压一条历史，出来后退或替换（与往来单位、服务产品页同一套）；有没提交的发布草稿时「返回列表」先问一句。
 *
 * 合同版本的运营主路径是「签订合同」（票 admin-write-faces/10：逐字段表单 → 预览摘要 → 存为待批准 → 批准 → 发布，ADR-0126）；
 * 受控 JSON 镜像口留作高级口（票 03）。停用不摆这里——它是一个命令带三种身份，读得最全的往来单位页收下它；本页身份状态格把
 * 停用两件都渲染出来，只是为了让走那一处停掉的账户在这里看得见结果。
 *
 * 墙降之前登记与发布四口必然答 403「接入渠道未配置」，那是诚实答案；墙降当天在装配点换真 Intake 即点亮，本页一行不用改。
 */
export function PartyContractsPage() {
  const grant = useRegistryWriteGrant();
  const mayWrite = grant.mayWrite;
  // 发布视图的载体到达「发布」那一步时拨一格，合同表随之重读（含绑定列）。
  const [publishedKey, setPublishedKey] = useState(0);
  // 地址是位置的唯一权威：视图从 hash 判（party-contracts-address.ts），只读本标签名下的地址变化。
  const [hash, setHash] = useState(() => window.location.hash);
  useOwnAddressChange(() => setHash(window.location.hash));
  // 对象地址先认（票 commercial-version-object-pages/02）。
  const object = versionObjectOf(hash, 'party-contracts');
  const requested = partyContractsViewOf(hash);
  // 答案到达之前入口照摆，到达后才知道没有登记写授予：停在登记视图上的话落回对应的列表，不留一块空白。
  const view: PartyContractsView = !mayWrite && isPartyContractsRegisterView(requested) ? partyContractsListOf(requested) : requested;
  // 查看者拿着登记视图的地址进来：授予已答出、明确没有写授予时把键清掉（替换地址）。加载期不动。
  useEffect(() => {
    if (isPartyContractsRegisterView(requested) && grant.settled && !grant.mayWrite) {
      const next = partyContractsViewHash(window.location.hash, partyContractsListOf(requested));
      window.location.replace(next);
      setHash(next);
    }
  }, [requested, grant.settled, grant.mayWrite]);
  const replaceView = (next: PartyContractsView) => {
    const target = partyContractsViewHash(window.location.hash, next);
    window.history.replaceState(window.history.state, '', target);
    setHash(target);
  };
  // 从列表压进登记视图的用后退回列表；刷新、深链、或压完之后地址被别处动过的（切过标签）用替换——判法在 usePushedAddress。
  const pushed = usePushedAddress();
  const openView = (next: PartyContractsView) => pushed.push(partyContractsViewHash(window.location.hash, next));
  const [confirmingLeave, setConfirmingLeave] = useState(false);
  const leaveView = () => {
    setConfirmingLeave(false);
    if (pushed.back()) return;
    replaceView(partyContractsListOf(view));
  };
  // 「新增客户账户」视图的表单要账户册（修订号建议）与往来单位册（所属客户的候选）：经取数层读，与各页同一份缓存。
  const accountsList = useRegisterList(listCustomerAccounts);
  const partiesList = useRegisterList(listBusinessParties);
  const knownAccounts = accountsList.answer?.kind === 'outcome' ? accountsList.answer.body.accounts : null;
  const partiesAnswer = partiesList.answer;
  const retryAccounts = accountsList.retry;
  // 登记视图里的发布表单有没提交的输入时先问一句（票 party-commercial-uiux/17 的登记簿）。
  // 「新增客户账户」视图如今是结构化表单，它也登记进簿（useRegistrationForm 自带），一并问。
  const requestLeave = () =>
    (view === 'publish-contract' && hasUnsavedDraft(publicationGroup('CUSTOMER_CONTRACT'))) ||
    (view === 'register' && hasUnsavedDraft(registrationGroup('customer-account')))
      ? setConfirmingLeave(true)
      : leaveView();

  // 右栏「签新版本」：记下是哪一版，进登记视图；发布表单据此预填并重建（票 party-commercial-uiux/14）。
  const [request, setRequest] = useState<NewVersionRequest<CustomerContractRecord> | null>(null);
  const beginNewVersion = (record: CustomerContractRecord) => {
    setRequest((previous) => nextRequest(previous, record));
    if (view !== 'publish-contract') openView('publish-contract');
  };
  // 登记视图里已填未发布的草稿要被换掉时先问一句（票 party-commercial-uiux/17）。
  const replaceGuard = useReplaceDraftGuard<CustomerContractRecord>(
    () => publicationGroup('CUSTOMER_CONTRACT'),
    beginNewVersion,
    '签订合同',
  );

  if (object) return <CustomerContractObjectPage key={`${object.objectId}@${object.version}`} {...object} />;

  if (isPartyContractsRegisterView(view)) {
    const heading = viewTitles[view];
    return (
      <RegisterView title={heading.title} subtitle={heading.subtitle} onBack={requestLeave}>
        {view === 'publish-contract' ? (
          <CustomerContractPublicationForm
            // 同一版再发起一次、或换一版，表单整个重建，不留上一次的预览与草稿。
            key={request?.token ?? 0}
            onPublished={() => setPublishedKey((value) => value + 1)}
            start={
              request
                ? {
                    draft: customerContractDraftFromRecord(request.source),
                    from: newVersionFrom('客户合同', request.source.objectId, request.source.version),
                  }
                : undefined
            }
          />
        ) : (
          <CustomerAccountRegistrationForm
            knownAccounts={knownAccounts}
            parties={partiesAnswer}
            onRegistered={retryAccounts}
          />
        )}
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
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      {replaceGuard.dialog}
      {/* 两本列表分签：换签只替换地址（不压历史）；访问过的签保活，切回来筛选与右栏还在。 */}
      <KeepAliveTabs value={view} onValueChange={(next) => replaceView(next as PartyContractsView)} className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="contracts">客户合同</TabsTrigger>
          <TabsTrigger value="accounts">客户账户</TabsTrigger>
        </TabsList>
        <KeepAliveTabsContent value="contracts" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <CustomerContractsWorkbench publishedKey={publishedKey} onStartNewVersion={replaceGuard.start} onOpenView={openView} />
        </KeepAliveTabsContent>
        <KeepAliveTabsContent value="accounts" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <CustomerAccountsTable onOpenView={openView} />
        </KeepAliveTabsContent>
      </KeepAliveTabs>
    </div>
  );
}
