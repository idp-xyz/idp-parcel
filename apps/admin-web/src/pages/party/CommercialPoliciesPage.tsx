import { useEffect, useState, type ReactNode } from 'react';
import { Button, Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { ListPageTemplate } from '../../templates';
import { ADDRESS_KEYWORD_PARAM, hashWithQueryValue } from '../../templates/address-query';
import { useAddressKeyword } from '../../templates/use-address-keyword';
import { RegistrationPanel, chipClass } from '../../components/registration';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState } from '../catalogue-view';
import {
  commercialRegistrationEndpoints,
  declarationLandingLabels,
  listCommercialPolicies,
  publicationOutcomeLabels,
  registerCommercial,
  type CommercialPolicyKind,
  type CommercialPolicyListResponseBody,
} from './api';
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
import { kindColumns, rowsOf, type PolicyRow } from './policy-rows';
import {
  commercialPolicyKinds,
  policyKindLabels,
  policyKindSources,
  problemNote,
  registrationSnapshotHints,
  registrationTitles,
} from './presentation';

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
}: {
  kind: CommercialPolicyKind;
  reloadKey: number;
  directory: NameDirectory;
}) {
  // 检索词在地址里（`?q=`）：合同上的规则包、控制策略链接带着它过来，筛到那一条。换册时由页面清掉（见 usePoliciesPlace）。
  const [search, setSearch] = useAddressKeyword();
  const [statusFilter, setStatusFilter] = useState<VersionStatusFilter>('all');
  const [reloadKey, setReloadKey] = useState(0);
  const [loaded, setLoaded] = useState<{
    kind: CommercialPolicyKind;
    answer: ApiResult<CommercialPolicyListResponseBody>;
  } | null>(null);

  useEffect(() => {
    let cancelled = false;
    void listCommercialPolicies(kind).then((answer) => {
      if (!cancelled) setLoaded({ kind, answer });
    });
    return () => {
      cancelled = true;
    };
  }, [kind, reloadKey, externalReloadKey]);

  const answer = loaded?.kind === kind ? loaded.answer : null;
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
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<PolicyRow>
      title={policyKindLabels[kind]}
      description={policyKindSources[kind]}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索政策对象、范围或引用',
      }}
      filterSummary={
        // 计数只在拿到业务答案后显示，未配置态不报「0 条」（与 pricing 两页同一守卫）。
        answer?.kind === 'outcome' ? versionCountSummary(rows.length, visibleRows.length, '条', '') : undefined
      }
      filters={
        answer?.kind === 'outcome' && withStatus ? (
          <VersionStatusChips
            counts={versionStatusCounts(rows, policyFacts)}
            value={statusFilter}
            onChange={setStatusFilter}
          />
        ) : undefined
      }
      emptyRowsNote="这组条件下没有匹配的行"
      columns={withRefLinks(withStatusBadge(kindColumns[kind]), directory)}
      rows={visibleRows}
      rowKey={(row) => row.key}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: `GET /commercial-policies?kind=${kind}`,
        emptyTitle: `当前租户尚无${policyKindLabels[kind]}`,
        emptyDescription: '这本册还没有任何版本；本页不会替你生成默认政策。',
      })}
    />
  );
}

// 声明类两册的行按所属版本给了版本信息；万一没有（行模型新加一册忘了给），按行键单独成组，不报错。
const policyFacts = (row: PolicyRow): VersionFacts => row.version ?? { objectId: row.key, version: '' };

/** 「发布新版本」签：选中的册有自己的表单就摆那张表单；随所属版本发布的两本说清去哪发。 */
function PublishPane({
  register,
  onRegisterChange,
  onPublished,
}: {
  register: CommercialPolicyKind;
  onRegisterChange: (kind: CommercialPolicyKind) => void;
  onPublished: (kind: CommercialPolicyKind) => void;
}) {
  const target = publishTargetOf(register);
  let body: ReactNode;
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
        text="接受前财务控制是客户合同上按费用范围的约定，随合同一起发布：在客户与合同页的「发布合同版本」签里填写。控制具体怎么做，在「接受前财务控制策略」这本册发布。"
        action="去客户与合同"
        onAction={() => {
          window.location.hash = '#/party-contracts';
        }}
      />
    );
  } else {
    body = publicationFormOf(register, () => onPublished(register));
  }
  return (
    <div className="flex-1 flex flex-col overflow-auto">
      <RegisterChooser value={register} onChange={onRegisterChange} />
      {body}
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
function publicationFormOf(register: CommercialPolicyKind, onPublished: () => void): ReactNode {
  switch (register) {
    case 'CREDIT_POLICY':
      return <CreditPolicyPublicationForm onPublished={onPublished} />;
    case 'AUTHORIZATION_RULE':
      return <AuthorizationRulePublicationForm onPublished={onPublished} />;
    case 'SETTLEMENT_POLICY':
      return <SettlementPolicyPublicationForm onPublished={onPublished} />;
    case 'PRICE_POLICY':
      return <PricePolicyPublicationForm onPublished={onPublished} />;
    case 'ACCEPTANCE_RULE_PACKAGE':
      return <AcceptanceRulePackagePublicationForm onPublished={onPublished} />;
    case 'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY':
      return <PreAcceptanceFinancialControlPolicyPublicationForm onPublished={onPublished} />;
    case 'CUSTOMER_SERVICE_RULE':
      return <CustomerServiceRulePublicationForm onPublished={onPublished} />;
    case 'PRE_ACCEPTANCE_CONTROL':
    case 'AS_OF_POLICY':
      return null;
  }
}

/** 签与册的地址同步：读自 hash、写回 hash（replaceState，不压后退记录、不触发壳层的 hashchange），做法同 useAddressKeyword。 */
function usePoliciesPlace(): [PoliciesTab, CommercialPolicyKind, (tab: PoliciesTab, register: CommercialPolicyKind) => void] {
  const [tab, setTab] = useState<PoliciesTab>(() => policiesTabOf(window.location.hash));
  const [register, setRegister] = useState<CommercialPolicyKind>(() => policiesRegisterOf(window.location.hash));

  useEffect(() => {
    const onHashChange = () => {
      setTab(policiesTabOf(window.location.hash));
      setRegister(policiesRegisterOf(window.location.hash));
    };
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

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
 * 商业规则与策略（party-commercial）：政策册、发布新版本、JSON 发布三签。
 *
 * 发布摆在这里而不另开专页，是票 admin-write-faces/03 的裁决：发布对象里多数类的结果显示在本页的册里，写签跟着读签走。
 * 发布签与政策册签共用同一组册名与同一个选中的册（票 party-commercial-uiux/05）：此前七个「发布 X 版本」签各占一签、
 * 与读签册名对不上，十个签在常见宽度下挤成两行。
 *
 * JSON 发布签按 ADR-0101 决定一是受控批量口的在线镜像，不是配置员的主路径，标「高级」列在最后。
 */
export function CommercialPoliciesPage() {
  const [tab, register, place] = usePoliciesPlace();
  const [reloadKey, setReloadKey] = useState(0);
  const directory = useNameDirectory();
  // 发布落定一版：读面那一册重读，切回政策册签时结果已在；仍停在发布签，让人看完答复。
  const notePublished = (kind: CommercialPolicyKind) => {
    if (kind === register) setReloadKey((value) => value + 1);
  };

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs
        value={tab}
        onValueChange={(next) => place(next as PoliciesTab, register)}
        className="flex-1 flex flex-col overflow-hidden gap-0"
      >
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="registers">政策册</TabsTrigger>
          <TabsTrigger value="publish">发布新版本</TabsTrigger>
          <TabsTrigger value="json">JSON 发布（高级）</TabsTrigger>
        </TabsList>
        <TabsContent
          value="registers"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <RegisterChooser value={register} onChange={(kind) => place('registers', kind)} />
          {/* 按册重挂：检索词从地址重读（换册时已清掉），不带上一本册的词。 */}
          <PolicyRegisters key={register} kind={register} reloadKey={reloadKey} directory={directory} />
        </TabsContent>
        <TabsContent
          value="publish"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <PublishPane register={register} onRegisterChange={(kind) => place('publish', kind)} onPublished={notePublished} />
        </TabsContent>
        <TabsContent
          value="json"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <RegistrationPanel
            moduleId="commercial-policies"
            title={registrationTitles.publication}
            endpoint={`POST ${commercialRegistrationEndpoints.publication}`}
            snapshotHint={registrationSnapshotHints.publication}
            submit={(snapshot) => registerCommercial('publication', snapshot)}
            outcomeLabels={publicationOutcomeLabels}
            declarationLandingLabels={declarationLandingLabels}
            problemNote={problemNote}
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}
