import type { AchievementSummary, MedalDefinition, MedalKind, RankingQuery, RankingPage, PublicCompanionProfile } from './types';

export interface AchievementStatus {
  summary: AchievementSummary;
  binding?: { baseUrl: string; member: { userId: string; organizationId: string; displayName: string; participating: boolean } };
  pending: number;
  repairs: number;
  participationPending?: boolean;
  selected: MedalKind[];
  profilePending: boolean;
}
export interface SocialHost {
  status(): Promise<AchievementStatus>;
  catalog(): Promise<MedalDefinition[]>;
  connect(baseUrl: string, credential: string, claimLocal: boolean): Promise<AchievementStatus>;
  sync(): Promise<AchievementStatus>;
  participation(enabled: boolean): Promise<void>;
  showcase(selected: MedalKind[]): Promise<AchievementStatus>;
  rankings(query: RankingQuery): Promise<RankingPage>;
  profile(userId: string): Promise<PublicCompanionProfile>;
}
export function makeSocialHost(call: (method: string, params?: Record<string, unknown>) => Promise<unknown>): SocialHost {
  const invoke = <T,>(method: string, params?: Record<string, unknown>) => call(method, params) as Promise<T>;
  return {
    status: () => invoke('status'), catalog: () => invoke('catalog'),
    connect: (baseUrl, credential, claimLocal) => invoke('connect', { baseUrl, credential, claimLocal }),
    sync: () => invoke('sync'), participation: enabled => invoke('participation', { enabled }),
    showcase: selected => invoke('showcase', { selected }),
    rankings: query => invoke('rankings', { ...query }), profile: userId => invoke('profile', { userId }),
  };
}
