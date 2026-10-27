import { useEffect, useState } from 'react';
import { Button, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { ListPageTemplate, useAddressKeyword, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { RegistrationPanel } from '../../components/registration';
import type { ApiResult } from '../catalogue-api';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { CustomerContractPublicationForm } from './CustomerContractPublicationForm';
import { nextRequest, newVersionFrom, type NewVersionRequest } from './new-version';
import { withNewVersionAction } from './NewVersionAction';
import { useReplaceDraftGuard } from './ReplaceDraftGuard';
import { publicationGroup } from './unsaved-guard';
import { contractKnownVersions } from './known-versions';
import { closableKeys, closureKeyOf, closureTargetOf, type ClosureTarget } from './version-closure';
import { VersionClosureDrawer } from './VersionClosureDrawer';
import { VersionDetailDrawer } from './VersionDetailDrawer';
import { versionHistoryOf } from './version-history';
import { customerContractDraftFromRecord } from './customer-contract-form';
import { KeepAliveTabs, KeepAliveTabsContent } from './keep-alive-tabs';
import { VersionStatusChips } from './VersionStatusChips';
import {
  groupedByObject,
  matchesVersionStatus,
  versionCountSummary,
  versionStatusCounts,
  type VersionFacts,
  type VersionStatusFilter,
} from './version-groups';
import { RefLink } from './RefLink';
import { loadCustomerAccountSuggestions } from './reference-inputs';
import { useLoaded } from './PublicationFormFields';
import { statusBadge } from './detail-primitives';
import { DeliveryConditionsCell } from './DeliveryConditionsCell';
import { catalogueViewState, formatInstant, formatRange } from '../catalogue-view';
import {
  commercialRegistrationEndpoints,
  listCustomerAccounts,
  listCustomerContracts,
  partyIdentityOutcomeLabels,
  registerCommercial,
  type CustomerAccountRecord,
  type CustomerContractListResponseBody,
  type CustomerContractRecord,
} from './api';
import {
  commercialStatusLabels,
  identityStatusLabels,
  labelOf,
  problemNote,
  registrationSnapshotHints,
  registrationTitles,
} from './presentation';
import { useMayWriteRegistry } from '../use-operator-grants';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['party-contracts'];

// 合同读面带出正文里指名的货主客户账户（customerAccountId）；责任法人合同自己没有，随接单规则包（规则包行上有）。账户的客户
// 名称取自客户账户册的第一页，读不到就只显账户标识，不猜名字。账户册自己在本页「客户账户」签。
const columns: ListColumn<CustomerContractRecord>[] = [
  {
    id: 'contract',
    header: '合同 / 版本',
    render: (row) => (
      <div className="min-w-48">
        <p className="font-mono font-medium text-idpxyz-text">{row.objectId}</p>
        <p className="mt-0.5 font-mono text-xs text-idpxyz-textMuted">{row.version}</p>
      </div>
    ),
  },
  {
    id: 'scope',
    header: '适用范围',
    className: 'font-mono text-xs',
    render: (row) => row.scope,
  },
  {
    id: 'status',
    header: '状态',
    render: (row) => statusBadge(commercialStatusLabels, row.status),
  },
  {
    id: 'rule-package',
    header: '接单规则包',
    className: 'font-mono text-xs',
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
    id: 'bindings',
    header: '接受前财务控制约定',
    className: 'min-w-64',
    render: (row) => <ControlBindings row={row} />,
  },
  {
    // 合同层交付条件（0030，票 admin-write-faces/25）：与正文是两层各自可缺，这一格不看 contentRegistered，只看键在不在。
    id: 'delivery-conditions',
    header: '交付条件（合同层）',
    className: 'min-w-64',
    render: (row) => <DeliveryConditionsCell conditions={row.deliveryConditions} />,
  },
  {
    id: 'effective',
    header: '有效区间',
    className: 'min-w-64 font-mono text-xs',
    render: (row) => formatRange(row.effectiveStartsAt, row.effectiveEndsAt),
  },
  {
    id: 'published-at',
    header: '发布时间',
    className: 'min-w-44 font-mono text-xs',
    render: (row) => formatInstant(row.publishedAt),
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
    return <span className="text-idpxyz-textMuted">已登记，未对任何费用范围作约定</span>;
  }
  return (
    <ul className="space-y-0.5">
      {row.bindings.map((binding) => (
        <li key={binding.chargeScope} className="text-xs">
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

/** 「货主客户账户」一列：账户标识为主，客户名称取自账户册已取回的那一页（取不到不显名称，不猜）。 */
function customerAccountColumn(names: ReadonlyMap<string, string>): ListColumn<CustomerContractRecord> {
  return {
    id: 'customer-account',
    header: '货主客户账户',
    className: 'font-mono text-xs',
    render: (row) => {
      if (row.customerAccountId) {
        const name = names.get(row.customerAccountId);
        return (
          <div className="min-w-40">
            <p>{row.customerAccountId}</p>
            {name ? <p className="mt-0.5 font-sans text-idpxyz-textMuted">{name}</p> : null}
          </div>
        );
      }
      return <span className="font-sans text-idpxyz-textMuted">{row.contentRegistered ? '—' : '正文未登记'}</span>;
    },
  };
}

/**
 * 客户合同版本册：合同版本到期或被后续版本替代，不改变已经接受委托所保存的合同依据。
 * 查阅面本身不设动作；发布走本页「发布合同版本」签（票 admin-write-faces/10），那一签的载体到达发布那一步时
 * 由页面把 `publishedKey` 拨一格，本表随之重读——结果在同页目录（含绑定列）立刻可见。
 */
function CustomerContractsTable({
  publishedKey,
  onStartNewVersion,
}: {
  publishedKey: number;
  /** 点行上的「发新版本」：把这一版带去发布签预填（票 party-commercial-uiux/14）。 */
  onStartNewVersion: (record: CustomerContractRecord) => void;
}) {
  // 检索词在地址里（`?q=`）：别处的引用链接带着它过来，筛到那一条。与客户账户签共用同一个检索词——它们在一页上。
  const [statusFilter, setStatusFilter] = useState<VersionStatusFilter>('all');
  const [search, setSearch] = useAddressKeyword();
  const [reloadKey, setReloadKey] = useState(0);
  // 没有登记册配置写授予的操作者不摆发新版本与收尾（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  const [answer, setAnswer] = useState<ApiResult<CustomerContractListResponseBody> | null>(null);
  // 账户标识旁的客户名称：账户册的第一页，读不到就没有名称。
  const accountsAnswer = useLoaded(loadCustomerAccountSuggestions);
  const accountNames = new Map(
    accountsAnswer?.kind === 'outcome'
      ? accountsAnswer.body.accounts.flatMap((account) =>
          account.customerPartyNameKnown && account.customerPartyName ? [[account.accountId, account.customerPartyName] as const] : [],
        )
      : [],
  );
  // 账户列紧跟状态列；按列标识找位置，不按下标数。
  const afterStatus = columns.findIndex((column) => column.id === 'status') + 1;
  const contractColumns = [...columns.slice(0, afterStatus), customerAccountColumn(accountNames), ...columns.slice(afterStatus)];
  // 点行上的「收尾…」：记下是哪一版，抽屉据此重读册面（票 party-commercial-uiux/16）。
  const [closing, setClosing] = useState<ClosureTarget | null>(null);
  // 点行打开的版本详情：记行键（票 party-commercial-uiux/21）。
  const [selectedKey, setSelectedKey] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listCustomerContracts().then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey, publishedKey]);

  const contracts = answer?.kind === 'outcome' ? answer.body.contracts : [];
  const needle = search.trim().toLowerCase();
  // 过滤只在已取回的这一页数据上做，不下推成查询参数——那要改端点契约。
  const searchedContracts = needle
    ? contracts.filter((row) =>
        [row.objectId, row.version, row.scope, row.status, row.rulePackageId ?? '', row.customerAccountId ?? ''].some((value) =>
          value.toLowerCase().includes(needle),
        ),
      )
    : contracts;
  // 同一合同的各版排在一起、新版在上；「在用」只看服务端答的状态（票 party-commercial-uiux/08）。
  const visibleContracts = groupedByObject(
    searchedContracts.filter((row) => matchesVersionStatus(contractFacts(row), statusFilter)),
    contractFacts,
  );
  const retry = () => setReloadKey((value) => value + 1);
  const closable = closableKeys(answer?.kind === 'outcome' ? contractKnownVersions(answer.body) : []);
  const keyOf = (row: CustomerContractRecord) => `${row.objectId}@${row.version}`;
  const selected = contracts.find((row) => keyOf(row) === selectedKey) ?? null;

  return (
    <>
      <ListPageTemplate<CustomerContractRecord>
        title={info.title}
        description="客户合同的各个版本：货主客户账户、合同约定的接单规则包与接受前财务控制、交付条件与有效区间。"
        search={{
          value: search,
          onChange: setSearch,
          placeholder: '搜索合同、版本、适用范围、客户账户或规则包',
        }}
        filterSummary={
          // 计数只在拿到业务答案后显示：未配置态与错误态下报「0 份」会与状态区
          // 「这不是目录为空」直接矛盾（README 列表页上列通则第六条）。
          answer?.kind === 'outcome'
            ? versionCountSummary(contracts.length, visibleContracts.length, '份', '合同版本')
            : undefined
        }
        filters={
          answer?.kind === 'outcome' && contracts.length > 0 ? (
            <VersionStatusChips
              counts={versionStatusCounts(contracts, contractFacts)}
              value={statusFilter}
              onChange={setStatusFilter}
            />
          ) : undefined
        }
        emptyRowsNote="这组条件下没有匹配的合同版本"
        columns={withNewVersionAction(contractColumns, {
          enabled: mayWrite,
          labelOf: (row) => `客户合同 ${row.objectId} ${row.version}`,
          onStart: onStartNewVersion,
          closure: {
            canClose: (row) => closable.has(closureKeyOf(row)),
            onClose: (row) => setClosing(closureTargetOf('CUSTOMER_CONTRACT', '客户合同', row)),
          },
        })}
        rows={visibleContracts}
        rowKey={keyOf}
        onRowClick={(row) => setSelectedKey(keyOf(row))}
        viewState={catalogueViewState(answer, contracts.length, retry, {
          module: info,
          endpoint: 'GET /commercial-customer-contracts',
          emptyTitle: '当前租户尚无客户合同版本',
          emptyDescription: '还没有发布任何客户合同。在「发布合同版本」签发布第一份；本页不会预置合同。',
        })}
      />
      <VersionDetailDrawer<CustomerContractRecord>
        row={selected}
        columns={contractColumns}
        title={(row) => ({ objectId: row.objectId, version: row.version })}
        subtitle={(row) => `客户合同 · 适用范围 ${row.scope}`}
        status={(row) => row.status}
        history={(row) => versionHistoryOf(contracts, row)}
        currentVersion={(row) => row.version}
        onSelectVersion={(version) => {
          const next = contracts.find((row) => row.objectId === selected?.objectId && row.scope === selected?.scope && row.version === version);
          if (next) setSelectedKey(keyOf(next));
        }}
        readOnly={!mayWrite}
        actions={(row) => (
          <>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setSelectedKey(null);
                onStartNewVersion(row);
              }}
            >
              发新版本
            </Button>
            {closable.has(closureKeyOf(row)) ? (
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setSelectedKey(null);
                  setClosing(closureTargetOf('CUSTOMER_CONTRACT', '客户合同', row));
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

// 客户账户册的列，与法人册同形：账户钉在一个客户参与方身份上，名称从参与方册转写；
// 停用两件（时点 + 依据）与状态同格——停用签的快照每项必填 basis，读面若只显时点，
// 写进去的依据就无处可见，停用签也就不能按「写签跟着读签走」落在这一页。
const accountColumns: ListColumn<CustomerAccountRecord>[] = [
  {
    id: 'account',
    header: '账户标识 / 修订',
    render: (row) => (
      <div className="min-w-40">
        <p className="font-mono font-medium text-idpxyz-text">{row.accountId}</p>
        <p className="mt-0.5 font-mono text-xs text-idpxyz-textMuted">r{row.revision}</p>
      </div>
    ),
  },
  {
    id: 'customer-party',
    header: '客户参与方',
    // 名称在参与方册上（账户不抄第二份）；customerPartyNameKnown 为假是写入门失败才会
    // 出现的悬空引用，如实标出让人去查写侧，不补占位文本冒充名称。
    render: (row) => (
      <div className="min-w-36">
        <p className="font-mono text-xs text-idpxyz-textMuted">{row.customerPartyId}</p>
        {row.customerPartyNameKnown ? (
          <p className="mt-0.5 font-medium text-idpxyz-text">{row.customerPartyName}</p>
        ) : (
          <p className="mt-0.5 text-xs text-idpxyz-textMuted">参与方册查无此身份</p>
        )}
      </div>
    ),
  },
  {
    id: 'status',
    header: '状态',
    align: 'center',
    render: (row) => (
      <div>
        <p>{statusBadge(identityStatusLabels, row.status)}</p>
        {row.deactivatedAt ? (
          <p className="mt-0.5 font-mono text-xs text-idpxyz-textMuted">
            自 {formatInstant(row.deactivatedAt)}
          </p>
        ) : null}
        {row.deactivationBasis ? (
          <p className="mt-0.5 font-mono text-xs text-idpxyz-textMuted">{row.deactivationBasis}</p>
        ) : null}
      </div>
    ),
  },
  {
    id: 'basis',
    header: '登记依据',
    className: 'font-mono text-xs',
    render: (row) => row.basis,
  },
  {
    id: 'effective-from',
    header: '生效自',
    className: 'min-w-44 font-mono text-xs',
    render: (row) => formatInstant(row.effectiveFrom),
  },
  {
    id: 'registered-at',
    header: '登记时间',
    className: 'min-w-44 font-mono text-xs',
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
function CustomerAccountsTable() {
  const [search, setSearch] = useAddressKeyword();
  const conditions = { q: useSettledKeyword(search) };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listCustomerAccounts);
  const accounts = answer?.kind === 'outcome' ? answer.body.accounts : [];

  return (
    <ListPageTemplate<CustomerAccountRecord>
      title={info.title}
      description="货主客户账户，每个账户显示最新一笔登记修订；名称取自所属业务参与方，状态按打开页面时的时点判断。"
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索账户、客户参与方或名称',
      }}
      filterSummary={answer?.kind === 'outcome' ? `本页 ${accounts.length} 个货主客户账户` : undefined}
      columns={accountColumns}
      rows={accounts}
      rowKey={(row) => row.accountId}
      emptyRowsNote="当前检索条件下没有匹配的货主客户账户"
      pagination={pagination}
      viewState={catalogueViewState(answer, accounts.length, retry, {
        module: info,
        endpoint: 'GET /commercial-customer-accounts',
        emptyTitle: '当前租户尚无货主客户账户登记',
        emptyDescription:
          '还没有登记任何货主客户账户。在「登记账户」签登记第一个；本页不会预置账户。',
        narrowed: catalogueConditionsNarrow(conditions),
      })}
    />
  );
}

/**
 * 客户与合同（party-commercial）：合同版本册、货主客户账户册，外加账户的登记签。
 *
 * 两册同页分签而不并表：合同是商业版本（草稿→发布→退役），账户是参与方身份（登记→
 * 生效→停用），两套状态代数并进一张表，同一个「已生效」会在两种含义间相互冒充。
 *
 * 登记签只装账户一册（ADR-0085；票 admin-write-faces/02 把它记为「无读面故不摆」的缺口，
 * 本页读面落地即摆）。合同版本的运营主路径是「发布合同版本」签（票 admin-write-faces/10：逐字段表单 →
 * 预览摘要 → 存为待批准 → 批准 → 发布，ADR-0126）；受控 JSON 镜像口留作高级口（票 03）。停用不摆这里——它是
 * 一个命令带三种身份，读得最全的业务参与方页收下它（同法人页那条理由）；本页身份状态格把停用两件都渲染
 * 出来，只是为了让走那一签停掉的账户在这里看得见结果。
 *
 * 墙降之前登记与发布四口必然答 403「接入渠道未配置」，那是诚实答案；墙降当天在装配点换真 Intake
 * 即点亮，本页一行不用改。
 */
export function PartyContractsPage() {
  const mayWrite = useMayWriteRegistry();
  // 发布签的载体到达「发布」那一步时拨一格，合同表随之重读（含绑定列）。
  const [publishedKey, setPublishedKey] = useState(0);
  // 点合同行上的「发新版本」：记下是哪一版，切到发布签；发布表单据此预填并重建（票 party-commercial-uiux/14）。
  const [placedTab, setTab] = useState('contracts');
  // 答案到达之前入口照摆，到达后才知道没有登记写授予：停在被收起的签上的话落回读签，不留一块空白。
  const tab = !mayWrite && (placedTab === 'publish-contract' || placedTab === 'register') ? 'contracts' : placedTab;
  const [request, setRequest] = useState<NewVersionRequest<CustomerContractRecord> | null>(null);
  const beginNewVersion = (record: CustomerContractRecord) => {
    setRequest((previous) => nextRequest(previous, record));
    setTab('publish-contract');
  };
  // 发布签里已填未发布的草稿要被换掉时先问一句（票 party-commercial-uiux/17）。
  const replaceGuard = useReplaceDraftGuard<CustomerContractRecord>(
    () => publicationGroup('CUSTOMER_CONTRACT'),
    beginNewVersion,
    '发布合同版本',
  );
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      {replaceGuard.dialog}
      <KeepAliveTabs value={tab} onValueChange={setTab} className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="contracts">客户合同</TabsTrigger>
          {mayWrite ? <TabsTrigger value="publish-contract">发布合同版本</TabsTrigger> : null}
          <TabsTrigger value="accounts">客户账户</TabsTrigger>
          {mayWrite ? <TabsTrigger value="register">登记账户</TabsTrigger> : null}
        </TabsList>
        <KeepAliveTabsContent
          value="contracts"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <CustomerContractsTable publishedKey={publishedKey} onStartNewVersion={replaceGuard.start} />
        </KeepAliveTabsContent>
        {mayWrite ? (
          <KeepAliveTabsContent
            value="publish-contract"
            className="flex-1 flex flex-col overflow-auto data-[state=inactive]:hidden"
          >
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
          </KeepAliveTabsContent>
        ) : null}
        <KeepAliveTabsContent
          value="accounts"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <CustomerAccountsTable />
        </KeepAliveTabsContent>
        {mayWrite ? (
          <KeepAliveTabsContent
            value="register"
            className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
          >
            <RegistrationPanel
              moduleId="party-contracts"
              title={registrationTitles['customer-account']}
              endpoint={`POST ${commercialRegistrationEndpoints['customer-account']}`}
              snapshotHint={registrationSnapshotHints['customer-account']}
              submit={(snapshot) => registerCommercial('customer-account', snapshot)}
              outcomeLabels={partyIdentityOutcomeLabels}
              problemNote={problemNote}
            />
          </KeepAliveTabsContent>
        ) : null}
      </KeepAliveTabs>
    </div>
  );
}
