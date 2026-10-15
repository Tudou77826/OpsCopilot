import { CompanionSnapshot, CompanionHatch, CompanionRename } from '../../wailsjs/go/main/App';
import { makeCompanionHost } from '../../../frontend-shell/src/companion/host';

export const wailsCompanionHost = makeCompanionHost((method, params = {}) => {
  if (method === 'snapshot') return CompanionSnapshot();
  if (method === 'hatch') return CompanionHatch(Number(params.revision));
  if (method === 'rename') return CompanionRename(String(params.name), Number(params.revision));
  throw new Error('未知伙伴操作');
});
