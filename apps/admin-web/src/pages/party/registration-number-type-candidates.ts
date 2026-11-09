import { useAnswer } from '../../query/answers';
import type { ApiResult } from '../catalogue-api';
import type { CatalogueConditions } from '../catalogue-query';
import { listRegistrationNumberTypes, type RegistrationNumberTypeListResponseBody } from './api';
import type { RegistrationNumberTypeLayer } from './legal-entity-identity';
import { partyAnswersDomain } from './party-writes';

// 登记表单里注册号类型候选的取法（票 catalogue-read-pagination-rollout/14）。目录读口按 ADR-0144 一页一页答，拿全册第一页挑候选，
// 类型多过一页就挑不全；读口为此声明了 layer 与 countryCode 维（后端 RegistrationNumberTypePages 的注释），表单按层与国家 / 地区
// 让服务端收窄。收窄之后仍只取一页：某一国某一层的类型多过一页时，候选照旧缺后面那些。
//
// 国家 / 地区候选是身份层类型登过的国家码（legal-entity-form.ts 的 identityCountryOptions），按 layer 收窄后取第一页；身份层类型
// 多过一页时国家候选会缺，选单照旧能手填。类型码转写成名（useTypeNameOf 等）不走这里，照旧读全册第一页。

/** 某一层、某一国家 / 地区的类型。国家码去首尾空白，与 identityTypeOptions、profileTaxTypeOptions 比国家的口径一样。 */
export function registrationNumberTypeCandidateConditions(layer: RegistrationNumberTypeLayer, country: string): CatalogueConditions {
  return { filters: { layer: [layer], countryCode: [country.trim()] } };
}

/** 国家 / 地区候选那一问：身份层的类型，不分国家。模块级函数——取数层按读函数本身认缓存。 */
export function listIdentityRegistrationNumberTypes(): Promise<ApiResult<RegistrationNumberTypeListResponseBody>> {
  return listRegistrationNumberTypes({ filters: { layer: ['IDENTITY'] } });
}

/**
 * 某一层、某一国家 / 地区的类型候选。国家还没填时不问、答 null；这个 null 别直接交给选单——选单把 null 读成加载中，会一直显
 * 「正在读」，「先选注册地」那句由调用方说。键挂在参与方域下：参与方写口整域作废时它跟着过期。
 */
export function useRegistrationNumberTypeCandidates(
  layer: RegistrationNumberTypeLayer,
  country: string,
): ApiResult<RegistrationNumberTypeListResponseBody> | null {
  const code = country.trim();
  const { answer } = useAnswer(
    [partyAnswersDomain, 'registration-number-type-candidates', layer, code],
    () => listRegistrationNumberTypes(registrationNumberTypeCandidateConditions(layer, code)),
    { enabled: code !== '' },
  );
  return code === '' ? null : answer;
}
