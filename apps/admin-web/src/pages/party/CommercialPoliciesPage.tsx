import { useEffect, useState, type ReactNode } from 'react';
import { useRetryWhenChanged } from '../../query/answers';
import { useRegisterList } from './register-list';
import { useOwnAddressChange } from '../../templates/use-own-address-change';
import { usePushedAddress } from '../../templates/use-pushed-address';
import { Button, ConfirmDialog } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { WorkbenchPageTemplate, type WorkbenchColumn } from '../../templates';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import { primaryActionClass } from '../../components/primary-action';
import { ADDRESS_KEYWORD_PARAM, hashWithQueryValue } from '../../templates/address-query';
import { useAddressKeyword } from '../../templates/use-address-keyword';
import { RegistrationPanel, chipClass } from '../../components/registration';
import { catalogueViewState } from '../catalogue-view';
import {
  commercialPolicyLoaders,
  commercialRegistrationEndpoints,
  declarationLandingLabels,
  publicationOutcomeLabels,
  registerCommercial,
  type CommercialPolicyKind,
  type CommercialPolicyListResponseBody,
} from './api';
import { nextRequest, newVersionFrom, type NewVersionRequest, type NewVersionStart } from './new-version';
import { useReplaceDraftGuard } from './ReplaceDraftGuard';
import { hasUnsavedDraft, publicationGroup, registrationGroup, useUnsavedGuard } from './unsaved-guard';
import { policyKnownVersions } from './known-versions';
import { closableKeys, closureKeyOf, closureTargetOf, type ClosureTarget } from './version-closure';
import { VersionClosureDialog } from './VersionClosureDialog';
import { VersionDetailPanel } from './VersionDetailPanel';
import { RegisterView } from './RegisterView';
import { versionHistoryOf } from './version-history';
import { acceptanceRulePackageDraftFromRecord } from './acceptance-rule-package-form';
import { authorizationRuleDraftFromRecord } from './authorization-rule-form';
import { creditPolicyDraftFromRecord } from './credit-policy-form';
import { customerServiceRuleDraftFromRecord } from './customer-service-rule-form';
import { preAcceptanceFinancialControlPolicyDraftFromRecord } from './pre-acceptance-financial-control-policy-form';
import { pricePolicyDraftFromRecord } from './price-policy-form';
import { settlementPolicyDraftFromRecord } from './settlement-policy-form';
import { useVisited } from './keep-alive-tabs';
import { VersionStatusChips } from './VersionStatusChips';
import {
  groupedByObject,
  hasVersionStatus,
  matchesVersionStatus,
  versionCountSummary,
  versionStatusCounts,
  type VersionFacts,
  type VersionStatusFilter,
} from './version-groups';
import { useNameDirectory, withRefLinks } from './RefLink';
import type { NameDirectory } from './ref-links';
import {
  policiesHash,
  policiesRegisterOf,
  policiesTabOf,
  publishTargetOf,
  type PoliciesTab,
} from './commercial-policies-address';
import { withStatusBadge } from './detail-primitives';
import { CreditPolicyPublicationForm } from './CreditPolicyPublicationForm';
import { AuthorizationRulePublicationForm } from './AuthorizationRulePublicationForm';
import { SettlementPolicyPublicationForm } from './SettlementPolicyPublicationForm';
import { PricePolicyPublicationForm } from './PricePolicyPublicationForm';
import { AcceptanceRulePackagePublicationForm } from './AcceptanceRulePackagePublicationForm';
import { PreAcceptanceFinancialControlPolicyPublicationForm } from './PreAcceptanceFinancialControlPolicyPublicationForm';
import { CustomerServiceRulePublicationForm } from './CustomerServiceRulePublicationForm';
import { kindColumns, rowsOf, type PolicyRow, type PolicySource } from './policy-rows';
import {
  commercialPolicyKinds,
  policyKindLabels,
  policyKindSources,
  problemNote,
  registrationSnapshotHints,
  registrationTitles,
} from './presentation';
import { useMayWriteRegistry, useRegistryWriteGrant } from '../use-operator-grants';
import { useRefdataNames } from '../use-refdata-names';

const info = moduleInfoById['commercial-policies'];

/**
 * 选册的一排按钮：政策册签与发布签共用，册名同一组。放在列表模板之外、能换行——模板的主筛选位不换行，九本册挤在那里
 * 曾逐字竖排、把右端计数压到工具位上。
 */
function RegisterChooser({
  value,
  onChange,
}: {
  value: CommercialPolicyKind;
  onChange: (kind: CommercialPolicyKind) => void;
}) {
  return (
    <div role="group" aria-label="政策册" className="flex flex-wrap items-center gap-2 px-4 pt-3 shrink-0">
      {commercialPolicyKinds.map((candidate) => (
        <button
          key={candidate}
          type="button"
          aria-pressed={candidate === value}
          className={`${chipClass(candidate === value)} whitespace-nowrap`}
          onClick={() => onChange(candidate)}
        >
          {policyKindLabels[candidate]}
        </button>
      ))}
    </div>
  );
}

// 各政策册独立请求、独立列形；同页切换不把各类对象折成一份「大配置」。行与列的转写在 policy-rows.ts，本组件只管取数与渲染。
// 列表标题就是选中的册名、说明就是这本册的册说明（policyKindSources）：页面层的话收进选册那一排，不再占一行被截断。
function PolicyRegisters({
  kind,
  reloadKey: externalReloadKey,
  directory,
  onStartNewVersion,
  onOpenPublish,
}: {
  kind: CommercialPolicyKind;
  reloadKey: number;
  directory: NameDirectory;
  /** 右栏头部的「发新版本」：把这一行的记录带去登记视图预填（票 party-commercial-uiux/14）。 */
  onStartNewVersion: (source: PolicySource) => void;
  /** 页头主动作：进「发布新版本」登记视图，选中当前这本册。 */
  onOpenPublish: () => void;
}) {
  // 检索词在地址里（`?q=`）：合同上的规则包、控制策略链接带着它过来，筛到那一条。换册时由页面清掉（见 usePoliciesPlace）。
  const [search, setSearch] = useAddressKeyword();
  const [statusFilter, setStatusFilter] = useState<VersionStatusFilter>('all');
  // 没有登记册配置写授予的操作者不摆发新版本与收尾（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  // 点行上的「收尾…」：记下是哪一版，抽屉据此重读册面（票 party-commercial-uiux/16）。
  const [closing, setClosing] = useState<ClosureTarget | null>(null);
  // 点行打开的版本详情：记行键（票 party-commercial-uiux/21）。
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  // 每册一个读函数（commercialPolicyLoaders）：换册就是另一份缓存，上一册的答案不会显在这一册下。
  const { answer, retry, pending } = useRegisterList(commercialPolicyLoaders[kind]);
  useRetryWhenChanged(externalReloadKey, retry);
  // 行里的币种经 currencyLabel 译名；订阅名表只为名表读回时重绘这些行。
  useRefdataNames();
  const rows = answer?.kind === 'outcome' ? rowsOf(answer.body) : [];
  const needle = search.trim().toLowerCase();
  const searchedRows = needle
    ? rows.filter((row) =>
        Object.values(row.values).some((value) => value.toLowerCase().includes(needle)),
      )
    : rows;
  // 同一对象的各版排在一起、新版在上（声明类两册按所属版本）；只有带状态的册才有「在用」筛选（票 party-commercial-uiux/08）。
  const withStatus = hasVersionStatus(rows, (row) => row.version);
  const visibleRows = groupedByObject(
    searchedRows.filter((row) => !withStatus || matchesVersionStatus(policyFacts(row), statusFilter)),
    policyFacts,
  );
  const closable = closableKeys(answer?.kind === 'outcome' ? policyKnownVersions(answer.body) : []);
  // 目录表与右栏详情用同一份列（引用链接与状态徽章已套好）；工作台的列按 key 认，照列的 id 搬过来。九本册列形各异，
  // 右栏打开时统一只留前三列（对象、版本与状态一类的主身份列），其余收起。
  const detailColumns = withRefLinks(withStatusBadge(kindColumns[kind]), directory);
  const workbenchColumns: WorkbenchColumn<PolicyRow>[] = detailColumns.map((column, index) => ({
    key: column.id,
    header: column.header,
    render: column.render,
    hideWhenDetailOpen: index >= 3,
  }));
  const historyPool = rows.flatMap((row) => (row.source ? [row.source.record] : []));
  const answered = answer?.kind === 'outcome';

  return (
    <>
      <WorkbenchPageTemplate<PolicyRow>
        title={policyKindLabels[kind]}
        viewStateKey={`commercial-policies:${kind}`}
        subtitle={policyKindSources[kind]}
        onRefresh={retry}
        refreshing={pending}
        primaryAction={
          mayWrite ? (
            <Button variant="default" className={primaryActionClass} onClick={onOpenPublish}>
              发布新版本
            </Button>
          ) : undefined
        }
        search={{ value: search, onChange: setSearch, placeholder: '搜规则编号、服务产品、客户合同' }}
        extraFilters={
          answered && withStatus ? (
            <VersionStatusChips counts={versionStatusCounts(rows, policyFacts)} value={statusFilter} onChange={setStatusFilter} />
          ) : undefined
        }
        // 计数只在拿到业务答案后显示，未配置态不报「0 条」（与 pricing 两页同一守卫）。
        toolbarRight={answered ? versionCountSummary(rows.length, visibleRows.length, '条', '') : undefined}
        columns={workbenchColumns}
        rows={visibleRows}
        rowKey={(row) => row.key}
        selectedKey={selectedKey}
        onSelect={setSelectedKey}
        noMatchText="没有匹配的规则"
        onClearFilters={() => {
          setSearch('');
          setStatusFilter('all');
        }}
        renderDetail={(key) => {
          const row = rows.find((candidate) => candidate.key === key);
          if (!row) return null;
          const target = policyClosureTarget(row.source);
          return (
            <VersionDetailPanel<PolicyRow>
              row={row}
              columns={detailColumns}
              title={(record) => ({ objectId: record.version?.objectId ?? record.key, version: record.version?.version ?? '' })}
              subtitle={() => policyKindLabels[kind]}
              technical={(record) => [
                { label: '对象标识', value: record.version?.objectId ?? '' },
                { label: '版本号', value: record.version?.version ?? '' },
                { label: '适用范围', value: record.source && 'scope' in record.source.record ? String(record.source.record.scope ?? '') : '' },
              ]}
              status={(record) => record.version?.status}
              history={(record) => (record.source ? versionHistoryOf(historyPool, record.source.record) : [])}
              currentVersion={(record) => record.version?.version ?? ''}
              onSelectVersion={(version) => {
                const next = rows.find((candidate) => candidate.version?.objectId === row.version?.objectId && candidate.version?.version === version);
                if (next) setSelectedKey(next.key);
              }}
              readOnly={!mayWrite}
              actions={(record) => (
                <>
                  {record.source ? (
                    <Button
                      variant="outline"
                      size="sm"
                      className="text-[12px]"
                      onClick={() => {
                        if (record.source) onStartNewVersion(record.source);
                      }}
                    >
                      发新版本
                    </Button>
                  ) : null}
                  {target !== null && closable.has(closureKeyOf(target)) ? (
                    <Button variant="outline" size="sm" className="text-[12px]" onClick={() => setClosing(target)}>
                      收尾…
                    </Button>
                  ) : null}
                </>
              )}
              onClose={() => setSelectedKey(null)}
            />
          );
        }}
        viewState={catalogueViewState(answer, rows.length, retry, {
          module: info,
          endpoint: `GET /commercial-policies?kind=${kind}`,
          emptyTitle: `还没有任何${policyKindLabels[kind]}`,
          emptyDescription: '这本册还是空的；用页头的「发布新版本」发布第一版。',
        })}
      />
      <VersionClosureDialog target={closing} onClose={() => setClosing(null)} onClosed={retry} />
    </>
  );
}

/** 行对应的收尾目标：只有读面带状态的三本册有；别的册与声明类两册没有（它们读不出哪一版还在参选）。 */
function policyClosureTarget(source: PolicySource | undefined): ClosureTarget | null {
  if (!source) return null;
  switch (source.kind) {
    case 'AUTHORIZATION_RULE':
    case 'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY':
    case 'CUSTOMER_SERVICE_RULE':
      return closureTargetOf(source.kind, policyKindLabels[source.kind], source.record);
    default:
      return null;
  }
}

// 声明类两册的行按所属版本给了版本信息；万一没有（行模型新加一册忘了给），按行键单独成组，不报错。
const policyFacts = (row: PolicyRow): VersionFacts => row.version ?? { objectId: row.key, version: '' };

/** 「发布新版本」签：选中的册有自己的表单就摆那张表单；随所属版本发布的两本说清去哪发。 */
function PublishPane({
  register,
  onRegisterChange,
  onPublished,
  request,
}: {
  register: CommercialPolicyKind;
  onRegisterChange: (kind: CommercialPolicyKind) => void;
  onPublished: (kind: CommercialPolicyKind) => void;
  /** 最近一次「基于此版本发新版本」的发起；那一册的表单按它预填并重建。 */
  request: NewVersionRequest<PolicySource> | null;
}) {
  const target = publishTargetOf(register);
  const seen = useVisited(register);
  let body: ReactNode = null;
  if (target.kind === 'with-rule-package') {
    body = (
      <WhereToPublish
        text="接受判断时点策略没有自己的版本，随接单规则包一起发布：在接单规则包的发布表单里填写。"
        action="去发布接单规则包"
        onAction={() => onRegisterChange('ACCEPTANCE_RULE_PACKAGE')}
      />
    );
  } else if (target.kind === 'with-contract') {
    body = (
      <WhereToPublish
        text="接受前财务控制是客户合同上按费用范围的约定，随合同一起发布：在客户与合同页的「签订合同」里填写。控制具体怎么做，在「接受前财务控制策略」这本册发布。"
        action="去客户与合同"
        onAction={() => {
          window.location.hash = '#/party-contracts';
        }}
      />
    );
  }
  return (
    <div className="flex-1 flex flex-col overflow-auto">
      <RegisterChooser value={register} onChange={onRegisterChange} />
      {body}
      {/* 换册不丢草稿：访问过的册的表单一直挂着、只藏起来（票 party-commercial-uiux/09）；各册草稿各在各的表单里，不互相带。 */}
      {commercialPolicyKinds
        .filter((kind) => seen(kind) && publishTargetOf(kind).kind === 'form')
        .map((kind) => (
          // key 带上发起序号：同一册再发起一次基于某版的预填，表单整个重建，不留上一次的预览与草稿。
          <div key={`${kind}:${request?.source.kind === kind ? request.token : 0}`} className={kind === register ? 'contents' : 'hidden'}>
            {publicationFormOf(kind, () => onPublished(kind), request)}
          </div>
        ))}
    </div>
  );
}

function WhereToPublish({ text, action, onAction }: { text: string; action: string; onAction: () => void }) {
  return (
    <div className="m-4 rounded border border-idpxyz-border p-4 flex flex-col items-start gap-3 text-[13px] text-idpxyz-text">
      <p>{text}</p>
      <Button variant="outline" onClick={onAction}>
        {action}
      </Button>
    </div>
  );
}

/**
 * 册 → 那一册的发布表单。发布落定后读面要切到的册由调用方按册名给：价格政策表单发的是发布类别 PRICE_RULE，结果显示在
 * 册名 PRICE_POLICY 那本册（两条分类轴，票 admin-write-faces/14），这里按册名进、按册名回，不经类别码。
 */
function publicationFormOf(
  register: CommercialPolicyKind,
  onPublished: () => void,
  request: NewVersionRequest<PolicySource> | null,
): ReactNode {
  switch (register) {
    case 'CREDIT_POLICY':
      return <CreditPolicyPublicationForm onPublished={onPublished} start={startFor(request, 'CREDIT_POLICY', creditPolicyDraftFromRecord)} />;
    case 'AUTHORIZATION_RULE':
      return (
        <AuthorizationRulePublicationForm
          onPublished={onPublished}
          start={startFor(request, 'AUTHORIZATION_RULE', authorizationRuleDraftFromRecord)}
        />
      );
    case 'SETTLEMENT_POLICY':
      return (
        <SettlementPolicyPublicationForm
          onPublished={onPublished}
          start={startFor(request, 'SETTLEMENT_POLICY', settlementPolicyDraftFromRecord)}
        />
      );
    case 'PRICE_POLICY':
      return <PricePolicyPublicationForm onPublished={onPublished} start={startFor(request, 'PRICE_POLICY', pricePolicyDraftFromRecord)} />;
    case 'ACCEPTANCE_RULE_PACKAGE':
      return (
        <AcceptanceRulePackagePublicationForm
          onPublished={onPublished}
          start={startFor(request, 'ACCEPTANCE_RULE_PACKAGE', acceptanceRulePackageDraftFromRecord, rulePackageCaution)}
        />
      );
    case 'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY':
      return (
        <PreAcceptanceFinancialControlPolicyPublicationForm
          onPublished={onPublished}
          start={startFor(
            request,
            'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY',
            preAcceptanceFinancialControlPolicyDraftFromRecord,
          )}
        />
      );
    case 'CUSTOMER_SERVICE_RULE':
      return (
        <CustomerServiceRulePublicationForm
          onPublished={onPublished}
          start={startFor(request, 'CUSTOMER_SERVICE_RULE', customerServiceRuleDraftFromRecord)}
        />
      );
    case 'PRE_ACCEPTANCE_CONTROL':
    case 'AS_OF_POLICY':
      return null;
  }
}

// 接单规则包的读口只带正文与收寄资格、终局规则，不带下面四节声明；它们在发布表单里是「本节留空即未声明」，预填后全是空的。
// 上一版声明过的，新版本不自己补就丢了——读面补不回来（要后端目录行带出），只能提醒。
const rulePackageCaution =
  '读面不带这个规则包的「接受判断时点策略」「接受内容」「面单有效期」「资料修订」四节声明，预填后这四节是空的（留空即未声明）。上一版声明过的，请在新版本里自己补上，否则新版本等于没有这些声明。';

/** 册 → 来源记录的类型；按册名索引，泛型里取记录类型比对联合做 Extract 稳。 */
type PolicyRecordOf = { [S in PolicySource as S['kind']]: S['record'] };

/** 最近一次发起若就是这一册的，还原出该册表单的起点；否则没有（表单从空白起）。 */
function startFor<K extends keyof PolicyRecordOf, Draft>(
  request: NewVersionRequest<PolicySource> | null,
  kind: K,
  draftOf: (record: PolicyRecordOf[K]) => Draft,
  caution?: string,
): NewVersionStart<Draft> | undefined {
  const source = request?.source;
  if (!source || source.kind !== kind) return undefined;
  const record = source.record as PolicyRecordOf[K];
  const identity = record as { objectId: string; version: string };
  return {
    draft: draftOf(record),
    from: newVersionFrom(policyKindLabels[kind], identity.objectId, identity.version),
    caution,
  };
}

/** 签与册的地址同步：读自 hash、写回 hash（replaceState，不压后退记录、不触发壳层的 hashchange），做法同 useAddressKeyword。 */
function usePoliciesPlace(): [PoliciesTab, CommercialPolicyKind, (tab: PoliciesTab, register: CommercialPolicyKind) => void] {
  const [tab, setTab] = useState<PoliciesTab>(() => policiesTabOf(window.location.hash));
  const [register, setRegister] = useState<CommercialPolicyKind>(() => policiesRegisterOf(window.location.hash));

  useOwnAddressChange(() => {
    setTab(policiesTabOf(window.location.hash));
    setRegister(policiesRegisterOf(window.location.hash));
  });

  const place = (nextTab: PoliciesTab, nextRegister: CommercialPolicyKind) => {
    const registerChanged = nextRegister !== register;
    setTab(nextTab);
    setRegister(nextRegister);
    const hash = window.location.hash;
    if (!hash.startsWith('#/')) return;
    // 换册清掉检索词：上一本册的词留着会把新册筛空，读起来像新册没有内容。
    const base = registerChanged ? hashWithQueryValue(hash, ADDRESS_KEYWORD_PARAM, '') : hash;
    window.history.replaceState(window.history.state, '', policiesHash(base, { tab: nextTab, register: nextRegister }));
  };
  return [tab, register, place];
}

/**
 * 商业规则与策略（party-commercial）：政策册工作台，外加「发布新版本」「JSON 发布（高级）」两个登记视图。
 *
 * 发布摆在这里而不另开专页，是票 admin-write-faces/03 的裁决：发布对象里多数类的结果显示在本页的册里，写签跟着读签走。
 * 发布视图与政策册共用同一组册名与同一个选中的册（票 party-commercial-uiux/05），换册只替换地址（usePoliciesPlace）。
 *
 * 容器照 commercial-registers-edit-containers 的决定（票 04）：行详情在工作台右栏，两张写面是由地址决定的整页登记视图
 * （`?tab=publish` / `?tab=json`，键沿用此前的签名），进视图压一条历史、出来后退或替换；视图里任一册的表单有没提交的输入时
 * 「返回列表」先问一句；收尾在对话框。JSON 发布按 ADR-0101 决定一是受控批量口的在线镜像，不是配置员的主路径，标「高级」。
 */
export function CommercialPoliciesPage() {
  const grant = useRegistryWriteGrant();
  const mayWrite = grant.mayWrite;
  const [placedTab, register, place] = usePoliciesPlace();
  // 地址里的签可能是别人发来的发布视图链接：没有登记写授予时两个视图不摆，落回政策册，不留一块空白。
  const tab = mayWrite || placedTab === 'registers' ? placedTab : 'registers';
  // 查看者拿着视图地址进来：授予已答出、明确没有写授予时把键清掉（替换地址）。加载期不动。
  useEffect(() => {
    if (placedTab !== 'registers' && grant.settled && !grant.mayWrite) place('registers', register);
  }, [placedTab, grant.settled, grant.mayWrite]);
  const [reloadKey, setReloadKey] = useState(0);
  const directory = useNameDirectory();
  // 从政策册压进视图的用后退回册；刷新、深链、或压完之后地址被别处动过的（切过标签）用替换——判法在 usePushedAddress。
  // 视图里换册走 usePoliciesPlace 的 replaceState，不经 hashchange，后退照常可用。
  const pushed = usePushedAddress();
  const openView = (next: PoliciesTab, nextRegister: CommercialPolicyKind) =>
    pushed.push(policiesHash(window.location.hash, { tab: next, register: nextRegister }));
  const [confirmingLeave, setConfirmingLeave] = useState(false);
  const leaveView = () => {
    setConfirmingLeave(false);
    if (pushed.back()) return;
    place('registers', register);
  };
  // JSON 发布的快照框报上来的「有没提交的输入」：记进登记簿（刷新、关页也会被拦）。
  const [jsonUnsaved, setJsonUnsaved] = useState(false);
  useUnsavedGuard(registrationGroup('publication-json'), tab === 'json' && jsonUnsaved);
  // 发布视图里任一册的表单有没提交的输入时先问一句（票 party-commercial-uiux/17 的登记簿；各册表单在视图里保活）；JSON 视图照快照框报的问。
  const requestLeave = () => {
    const unsaved =
      tab === 'publish' ? commercialPolicyKinds.some((kind) => hasUnsavedDraft(publicationGroup(kind))) : tab === 'json' && jsonUnsaved;
    if (unsaved) setConfirmingLeave(true);
    else leaveView();
  };
  // 右栏「发新版本」：记下是哪一版，进发布视图、选中那一册；那一册的表单据此预填重建（票 party-commercial-uiux/14）。
  const [request, setRequest] = useState<NewVersionRequest<PolicySource> | null>(null);
  const beginNewVersion = (source: PolicySource) => {
    setRequest((previous) => nextRequest(previous, source));
    if (tab === 'publish') place('publish', source.kind);
    else openView('publish', source.kind);
  };
  // 目标那一册的发布表单里已填未发布的草稿要被换掉时先问一句（票 party-commercial-uiux/17）。
  const replaceGuard = useReplaceDraftGuard<PolicySource>(
    (source) => publicationGroup(source.kind),
    beginNewVersion,
    '发布新版本',
  );
  // 发布落定一版：读面那一册重读，回到政策册时结果已在；仍停在视图里，让人看完答复。
  const notePublished = (kind: CommercialPolicyKind) => {
    if (kind === register) setReloadKey((value) => value + 1);
  };

  if (tab === 'publish' || tab === 'json') {
    return (
      <RegisterView
        title={tab === 'publish' ? '发布新版本' : 'JSON 发布（高级）'}
        subtitle={tab === 'publish' ? `当前册：${policyKindLabels[register]}；表单 → 预览 → 存为待批准 → 批准 → 发布` : registrationSnapshotHints.publication}
        onBack={requestLeave}
      >
        {tab === 'publish' ? (
          <PublishPane register={register} onRegisterChange={(kind) => place('publish', kind)} onPublished={notePublished} request={request} />
        ) : (
          <RegistrationPanel
            moduleId="commercial-policies"
            title={registrationTitles.publication}
            endpoint={`POST ${commercialRegistrationEndpoints.publication}`}
            snapshotHint={registrationSnapshotHints.publication}
            submit={(snapshot) => registerCommercial('publication', snapshot)}
            outcomeLabels={publicationOutcomeLabels}
            declarationLandingLabels={declarationLandingLabels}
            problemNote={problemNote}
            onUnsavedChange={setJsonUnsaved}
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
      <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 pr-4">
        <RegisterChooser value={register} onChange={(kind) => place('registers', kind)} />
        {mayWrite ? (
          <Button variant="ghost" size="sm" className="mt-3 text-[12px] text-idpxyz-breadcrumb" onClick={() => openView('json', register)}>
            JSON 发布（高级）
          </Button>
        ) : null}
      </div>
      {/* 按册重挂：检索词从地址重读（换册时已清掉），不带上一本册的词。 */}
      <PolicyRegisters
        key={register}
        kind={register}
        reloadKey={reloadKey}
        directory={directory}
        onStartNewVersion={replaceGuard.start}
        onOpenPublish={() => openView('publish', register)}
      />
    </div>
  );
}
