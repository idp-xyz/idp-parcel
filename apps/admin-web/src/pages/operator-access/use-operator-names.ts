import { useEffect, useState } from 'react';
import { listOperatorNames } from './api';
import { noOperatorNames, operatorNamesOf, type OperatorNames } from './operator-names';

/**
 * 本租户登了姓名的操作者，给拣货人、放波次人这些格示成人名用。
 *
 * 读不到（没有查阅读授予、渠道未配置、网络不通）就是空表：这些格照旧示 sub，不为一个辅助的名字把页面挡住，也不弹错。
 * 每次挂载各读一遍、不留缓存：姓名可以改可以删，留着旧的会把已经删掉的姓名继续摆出来。
 */
export function useOperatorNames(): OperatorNames {
  const [names, setNames] = useState<OperatorNames>(noOperatorNames);
  useEffect(() => {
    let cancelled = false;
    void listOperatorNames().then((answer) => {
      if (cancelled || answer.kind !== 'outcome') return;
      setNames(operatorNamesOf(answer.body.names));
    });
    return () => {
      cancelled = true;
    };
  }, []);
  return names;
}
