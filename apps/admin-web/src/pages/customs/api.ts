// 本目录 fetch 出口:合规规则库查阅(GET /customs-compliance-rules,ADR-0077、
// 票 master-data-wiring/04)。两本册子按 ?registry= 分派。

let apiBase = '';

export function configureCustomsApi(options: { basePrefix: string }): void {
  apiBase = options.basePrefix;
}

export type ComplianceRegistry = 'case-requirement' | 'interpretation';

export interface CaseRequirementRuleRecord {
  jurisdiction: string;
  direction: string;
  procedure: string;
  required: boolean;
  basis: string;
}

export interface InterpretationRuleRecord {
  layer: string;
  jurisdiction: string;
  rule: string;
  appliesFrom: string;
  appliesUntil?: string;
}

export interface CaseRequirementListResponseBody {
  outcome: 'CASE_REQUIREMENT_RULES_LISTED';
  rules: CaseRequirementRuleRecord[];
}

export interface InterpretationListResponseBody {
  outcome: 'INTERPRETATION_RULES_LISTED';
  rules: InterpretationRuleRecord[];
}

export type ComplianceRulesListResponseBody =
  | CaseRequirementListResponseBody
  | InterpretationListResponseBody;

export type ApiResult<Body> =
  | { kind: 'outcome'; status: number; body: Body }
  | { kind: 'unconfigured' }
  | { kind: 'callerProblem'; status: number; code: string }
  | { kind: 'noAnswer'; status: number; code: string }
  | { kind: 'transport'; message: string };

export function listComplianceRules(
  registry: ComplianceRegistry,
): Promise<ApiResult<ComplianceRulesListResponseBody>> {
  return exchange<ComplianceRulesListResponseBody>(
    `/customs-compliance-rules?registry=${encodeURIComponent(registry)}`,
    { method: 'GET' },
  );
}

async function exchange<Body>(path: string, init: RequestInit): Promise<ApiResult<Body>> {
  let response: Response;
  try {
    response = await fetch(apiBase + path, init);
  } catch (cause) {
    return {
      kind: 'transport',
      message: cause instanceof Error ? cause.message : String(cause),
    };
  }

  let parsed: unknown;
  try {
    parsed = await response.json();
  } catch {
    return {
      kind: 'transport',
      message: `响应不是 JSON(HTTP ${response.status}),请求可能未到达 parcel-api`,
    };
  }

  if (response.ok) {
    return { kind: 'outcome', status: response.status, body: parsed as Body };
  }

  const code =
    (parsed as { error?: { code?: string } } | null)?.error?.code ?? 'UNKNOWN';
  if (response.status === 403 && code === 'ACCESS_CHANNEL_NOT_CONFIGURED') {
    return { kind: 'unconfigured' };
  }
  if (response.status >= 500) {
    return { kind: 'noAnswer', status: response.status, code };
  }
  return { kind: 'callerProblem', status: response.status, code };
}
