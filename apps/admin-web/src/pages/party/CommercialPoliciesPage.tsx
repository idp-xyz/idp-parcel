import { useEffect, useState } from 'react';
import { moduleInfoById } from '../../navigation';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant, formatRange } from '../catalogue-view';
import {
  listCommercialPolicies,
  type AuthorizationRuleRecord,
  type CommercialPolicyKind,
  type CommercialPolicyListResponseBody,
} from './api';
import {
  commercialDirectionLabels,
  commercialPolicyKinds,
  commercialStatusLabels,
  controlRequirementLabels,
  finalOutcomeLabels,
  intakeSourceLabels,
  labelOf,
  policyKindLabels,
  settlementMethodLabels,
} from './presentation';

const info = moduleInfoById['commercial-policies'];

interface PolicyRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

function col(id: string, header: string, mono = false): ListColumn<PolicyRow> {
  return {
    id,
    header,
    className: mono ? 'font-mono text-xs' : undefined,
    render: (row) => row.values[id] ?? '—',
  };
}

// 按 kind 换列(MCP-3 裁决⑦):六种册子的行形状互不相同,列向各随其册。种类命名
// 册子而非商业对象类别;信用政策没有独立正文册,封闭集里如实没有它,页面不预留格。
const kindColumns: Record<CommercialPolicyKind, ListColumn<PolicyRow>[]> = {
  ACCEPTANCE_RULE_PACKAGE: [
    col('identity', '规则包 / 版本', true),
    col('serviceProduct', '服务产品', true),
    col('contract', '客户合同', true),
    col('legalEntity', '责任法人', true),
    col('scope', '适用范围', true),
    col('rules', '组装规则(类别:引用)', true),
    col('allowedIntakeSources', '允许收寄来源'),
    col('intakeQualifications', '收寄硬资格', true),
    col('finalRules', '终局规则(结果:分类)', true),
    col('effective', '有效区间', true),
    col('declaredAt', '声明时间', true),
  ],
  PRE_ACCEPTANCE_CONTROL: [
    col('contract', '客户合同 / 版本', true),
    col('requirement', '控制要求'),
    col('notApplicableBasis', '不适用依据', true),
    col('declaredAt', '声明时间', true),
  ],
  PRICE_POLICY: [
    col('identity', '政策对象 / 版本', true),
    col('direction', '政策方向'),
    col('planRef', '定价方案引用', true),
    col('planDirection', '方案方向'),
    col('bindingConversion', '绑定转换', true),
    col('policyScope', '适用范围', true),
    col('effective', '有效区间', true),
    col('registeredAt', '登记时间', true),
  ],
  SETTLEMENT_POLICY: [
    col('identity', '政策对象 / 版本', true),
    col('method', '结算方式'),
    col('legalEntity', '责任法人', true),
    col('counterparty', '相对方', true),
    col('contractLabel', '合同标签', true),
    col('chargeScope', '费用范围', true),
    col('currency', '币种', true),
    col('effective', '有效区间', true),
    col('registeredAt', '登记时间', true),
  ],
  AS_OF_POLICY: [
    col('package', '规则包 / 版本', true),
    col('judgmentType', '判断类型', true),
    col('semanticsRef', '时点语义引用', true),
    col('policyVersion', '时点政策版本', true),
    col('declaredAt', '声明时间', true),
  ],
  // 两个请求方各占一列,不并成「取消授权」一栏:「客户可取消」与「运营可取消」是两条
  // 独立授权,合成一栏读不出哪一方缺席。
  AUTHORIZATION_RULE: [
    col('identity', '授权规则 / 版本', true),
    col('scope', '适用范围', true),
    col('status', '生命周期状态'),
    col('customerCancellation', '客户取消授权'),
    col('operationsCancellation', '运营取消授权'),
    col('effective', '有效区间', true),
    col('publishedAt', '发布时间', true),
  ],
};

const chipClass = (active: boolean) =>
  `px-2.5 py-1 text-[12px] rounded border ${
    active
      ? 'border-idpxyz-accent text-idpxyz-accent'
      : 'border-idpxyz-border text-idpxyz-textMuted hover:bg-idpxyz-hover'
  }`;

// 两族阶段内容声明的三栏各有自己的「空」。「未声明」与「声明了但为空」在数组长度上
// 撞成同一签名,靠服务端给的 *Declared 布尔分——两者的恢复动作相反(前者去登记声明,
// 后者无事可做)。
//
// 允许来源与终局结果两栏的「已声明却为空」按领域规矩根本不该出现(两处都要求至少
// 一行),所以那句写成「已声明,正文为空」而不是一句无害的空话:它是一份坏数据的
// 如实呈现,不该读起来像正常态。资格引用那栏不同——显式声明「无硬资格」是合法的。
function declaredList(declared: boolean, values: string[], emptyNote: string): string {
  if (!declared) return '未声明';
  if (values.length === 0) return emptyNote;
  return values.join('、');
}

// 取消授权按请求方逐格作答,三态各有各的说法。中间那态最容易写错:目录在场而这一方
// 没有行,是这份目录说出的真话——该请求方不许取消——不是配置缺件。把它显示成空白或
// 「未声明」会让人去补一份已经写好的目录,而那份目录正是拒绝的依据。
function cancellationCell(record: AuthorizationRuleRecord, party: string): string {
  if (!record.cancellationAuthorityDeclared) return '未声明';
  const declaration = record.cancellationAuthorities.find((entry) => entry.party === party);
  return declaration ? `允许:${declaration.ruleReference}` : '不许取消';
}

// 响应体按 kind 判别(api.ts 的联合),各分支读各自的行形;判断类型与绑定转换是
// 开放引用集,按原词展示不配词表。
function rowsOf(body: CommercialPolicyListResponseBody): PolicyRow[] {
  switch (body.kind) {
    case 'ACCEPTANCE_RULE_PACKAGE':
      return body.policies.map((record) => ({
        key: `package:${record.objectId}@${record.version}`,
        values: {
          identity: `${record.objectId}@${record.version}`,
          serviceProduct: record.serviceProduct,
          contract: record.contract,
          legalEntity: record.legalEntity,
          scope: record.scope,
          rules: record.rules
            .map((rule) => `${rule.category}:${rule.reference}`)
            .join('、'),
          allowedIntakeSources: declaredList(
            record.intakeQualificationDeclared,
            record.allowedIntakeSources.map((source) => labelOf(intakeSourceLabels, source)),
            '已声明,正文为空',
          ),
          intakeQualifications: declaredList(
            record.intakeQualificationDeclared,
            record.intakeQualificationRefs,
            '已声明,无硬资格',
          ),
          finalRules: declaredList(
            record.finalRulesDeclared,
            record.finalRules.map(
              (final) => `${labelOf(finalOutcomeLabels, final.outcome)}:${final.finalKind}`,
            ),
            '已声明,正文为空',
          ),
          effective: formatRange(record.effectiveStartsAt, record.effectiveEndsAt),
          declaredAt: formatInstant(record.declaredAt),
        },
      }));
    case 'PRE_ACCEPTANCE_CONTROL':
      return body.policies.map((record) => ({
        key: `control:${record.contractObjectId}@${record.contractVersion}`,
        values: {
          contract: `${record.contractObjectId}@${record.contractVersion}`,
          requirement: labelOf(controlRequirementLabels, record.requirement),
          notApplicableBasis: record.notApplicableBasis ?? '—',
          declaredAt: formatInstant(record.declaredAt),
        },
      }));
    case 'PRICE_POLICY':
      return body.policies.map((record) => ({
        key: `price:${record.objectId}@${record.version}`,
        values: {
          identity: `${record.objectId}@${record.version}`,
          direction: labelOf(commercialDirectionLabels, record.direction),
          planRef: record.planRef,
          planDirection: labelOf(commercialDirectionLabels, record.planDirection),
          bindingConversion: record.bindingConversion,
          policyScope: record.policyScope,
          effective: formatRange(record.effectiveStartsAt, record.effectiveEndsAt),
          registeredAt: formatInstant(record.registeredAt),
        },
      }));
    case 'SETTLEMENT_POLICY':
      return body.policies.map((record) => ({
        key: `settlement:${record.objectId}@${record.version}`,
        values: {
          identity: `${record.objectId}@${record.version}`,
          method: labelOf(settlementMethodLabels, record.method),
          legalEntity: record.legalEntity,
          counterparty: record.counterparty,
          contractLabel: record.contractLabel,
          chargeScope: record.chargeScope,
          currency: record.currency,
          effective: formatRange(record.effectiveStartsAt, record.effectiveEndsAt),
          registeredAt: formatInstant(record.registeredAt),
        },
      }));
    case 'AS_OF_POLICY':
      return body.policies.map((record) => ({
        key: `as-of:${record.rulePackageObjectId}@${record.rulePackageVersion}:${record.judgmentType}`,
        values: {
          package: `${record.rulePackageObjectId}@${record.rulePackageVersion}`,
          judgmentType: record.judgmentType,
          semanticsRef: record.semanticsRef,
          policyVersion: record.policyVersion,
          declaredAt: formatInstant(record.declaredAt),
        },
      }));
    case 'AUTHORIZATION_RULE':
      return body.policies.map((record) => ({
        key: `authz:${record.objectId}@${record.version}`,
        values: {
          identity: `${record.objectId}@${record.version}`,
          scope: record.scope,
          status: labelOf(commercialStatusLabels, record.status),
          customerCancellation: cancellationCell(record, 'CUSTOMER'),
          operationsCancellation: cancellationCell(record, 'OPERATIONS'),
          effective: formatRange(record.effectiveStartsAt, record.effectiveEndsAt),
          publishedAt: formatInstant(record.publishedAt),
        },
      }));
  }
}

// 各政策册独立请求、独立列形;同页切换不把五类对象折成一份「大配置」。
export function CommercialPoliciesPage() {
  const [kind, setKind] = useState<CommercialPolicyKind>('ACCEPTANCE_RULE_PACKAGE');
  const [search, setSearch] = useState('');
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
  }, [kind, reloadKey]);

  const answer = loaded?.kind === kind ? loaded.answer : null;
  const rows = answer?.kind === 'outcome' ? rowsOf(answer.body) : [];
  const needle = search.trim().toLowerCase();
  const visibleRows = needle
    ? rows.filter((row) =>
        Object.values(row.values).some((value) => value.toLowerCase().includes(needle)),
      )
    : rows;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<PolicyRow>
      title={info.title}
      description={`${info.owner}——六类政策册分别查阅,重叠候选仍是适用冲突而非「同时生效」;信用政策无独立正文册,如实不上列。接单规则包一栏另列挂在同一版本上的收寄资格与终局规则声明,授权规则一栏按请求方逐格列出取消授权`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索政策对象、范围或引用',
      }}
      filters={
        <>
          {commercialPolicyKinds.map((candidate) => (
            <button
              key={candidate}
              type="button"
              className={chipClass(candidate === kind)}
              onClick={() => setKind(candidate)}
            >
              {policyKindLabels[candidate]}
            </button>
          ))}
        </>
      }
      filterSummary={
        // 计数只在拿到业务答案后显示,未配置态不报「0 条」(与 pricing 两页同一守卫)。
        answer?.kind === 'outcome' ? `${policyKindLabels[kind]} ${rows.length} 条` : undefined
      }
      columns={kindColumns[kind]}
      rows={visibleRows}
      rowKey={(row) => row.key}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: `GET /commercial-policies?kind=${kind}`,
        emptyTitle: `当前租户尚无${policyKindLabels[kind]}`,
        emptyDescription: '读取入口已配置,但该政策册为空;页面不会生成默认政策。',
      })}
    />
  );
}
