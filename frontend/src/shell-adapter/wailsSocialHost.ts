import { AchievementCatalog, AchievementStatus, SocialConnect, SocialSync, SocialParticipation, SocialShowcase, SocialRankings, SocialProfile } from '../../wailsjs/go/main/App';
import { makeSocialHost } from '../../../frontend-shell/src/companion/socialHost';
import type { MedalKind } from '../../../frontend-shell/src/companion/types';

export const wailsSocialHost = makeSocialHost(async (method, params = {}) => {
  switch(method) {
    case 'status': return AchievementStatus();
    case 'catalog': return AchievementCatalog();
    case 'connect': return SocialConnect(String(params.baseUrl), String(params.credential), params.claimLocal === true);
    case 'sync': return SocialSync();
    case 'participation': return SocialParticipation(params.enabled === true);
    case 'showcase': return SocialShowcase(params.selected as MedalKind[]);
    case 'rankings': return SocialRankings(params.kind as MedalKind, Number(params.offset), Number(params.limit), String(params.snapshotId ?? ''));
    case 'profile': return SocialProfile(String(params.userId));
    default: throw new Error('未知奖章操作');
  }
});
