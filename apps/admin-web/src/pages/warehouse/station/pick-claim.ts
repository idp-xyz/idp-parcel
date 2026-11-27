// 拣货工位上「领这个波次」之后屏上的那一句（纯 TypeScript，node:test 直接测）。
//
// 到 Go 端核过的规则（用例是 application.WaveHandler.ClaimWave；口在 warehousehttp.NewClaimWaveEndpoint，挂拣货那一种
// 作业事实）：没人领的波次归领的这个人；本来就是自己的答已有结果；已经派给了别人答 WAVE_ASSIGNED_TO_OTHER、带着派给了谁；
// 已完成、已取消的波次不能领（STATUS_MISMATCH）；没有这个波次答 WAVE_NOT_FOUND；波次不属于这台设备绑的仓答内容不合。
// 领了之后，别人拣这个波次里待执行的任务后端不收；主管可以在波次页改派或取消指派。

import type { ApiResult } from '../../catalogue-api';
import type { WaveClaimBody } from '../api';
import { noOperatorNames, operatorLabel, type OperatorNames } from '../../operator-access/operator-names';
import { describeReadFailure } from './work-fact-submit';

export function waveClaimNote(answer: ApiResult<WaveClaimBody>, names: OperatorNames = noOperatorNames): string {
  if (answer.kind !== 'outcome') return `没领成：${describeReadFailure(answer)}。再点一次。`;
  switch (answer.body.outcome) {
    case 'ASSIGNED':
      return '领到了，这个波次归你拣。';
    case 'EXISTING_RESULT':
      return '这个波次本来就是你的。';
    case 'WAVE_ASSIGNED_TO_OTHER':
      return `已经派给了 ${operatorLabel(answer.body.assignee, names) ?? '别人'}，没有领成。要改派找主管在波次页上改。`;
    case 'STATUS_MISMATCH':
      return '这个波次已经拣完或已取消，不用领了。';
    case 'WAVE_NOT_FOUND':
      return '没有这个波次。';
    case 'CONTENT_INVALID':
      return '这个波次不属于这台设备绑的仓，领不了。';
    case 'NOT_CONFIGURED':
      return '未配置：本租户还没有登记仓。';
    default:
      return `后端答了一个这里不认识的结果（${answer.body.outcome}），没有当成领到。`;
  }
}
