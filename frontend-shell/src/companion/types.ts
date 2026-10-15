/** Local companion and social capabilities have independent lifecycles.
 * Work settlement has no frontend port. A social adapter supplies an
 * authenticated user; callers never specify a sender or a growth reward.
 */
export type CompanionStage = 'child' | 'growing' | 'adult';
export type CompanionPalette = 'mint' | 'amber' | 'lavender';
export interface CompanionCard {
  speciesId: string;
  name: string;
  paletteId: CompanionPalette;
  /** Absent only in historical public cards awaiting their next authenticated update. */
  level?: number;
}
export interface CompanionSnapshot {
  schemaVersion: 1;
  ruleVersion: 1;
  revision: number;
  pet: { id: string; speciesId: string; name: string; paletteId: CompanionPalette; xp: number; adoptedAt: string } | null;
  level: number;
  levelXP: number;
  levelXPRequired: number;
  todayXP?: number;
  dailyXPMax?: number;
  hatchCost: number;
  canHatch: boolean;
}
export interface CompanionHost {
  snapshot(): Promise<CompanionSnapshot>;
  hatch(revision: number): Promise<CompanionSnapshot>;
  rename(name: string, revision: number): Promise<CompanionSnapshot>;
}
export type MedalKind =
  | 'knowledge-archive' | 'script-created' | 'quick-command-created'
  | 'case-completed' | 'knowledge-read' | 'script-used'
  | 'quick-command-used' | 'active-day';
export interface MedalDefinition {
  kind: MedalKind;
  name: string;
  unit: string;
  group: 'creation' | 'practice' | 'consistency';
  thresholds: [number, number, number];
}
export interface Medal {
  kind: MedalKind;
  tier: 'bronze' | 'silver' | 'gold';
  count: number;
}
export interface AchievementSummary {
  ruleVersion: 1;
  progress: Array<{
    kind: MedalKind;
    count: number;
    medal?: Medal;
    nextThreshold?: number;
  }>;
}
export interface CompanionProfile {
  userId: string;
  displayName: string;
  companion?: CompanionCard;
  /** Explicitly selected, service-derived aggregate medals. */
  medals: Medal[];
}
export interface PublicCompanionProfile extends CompanionProfile {
  /** Published aggregate counts only; never raw confirmation records. */
  achievements: AchievementSummary;
}
export interface RankingQuery {
  kind: MedalKind;
  offset: number;
  limit: number;
  /** Absent on first page; reuse response snapshot for subsequent pages. */
  snapshotId?: string;
}
export interface RankingPage {
  kind: MedalKind;
  ruleVersion: 1;
  snapshotId: string;
  offset: number;
  total: number;
  entries: Array<{ rank: number; count: number; profile: CompanionProfile }>;
}
/** Concrete local ledger and authenticated ranking capabilities. */
export type { SocialHost as CompanionSocialHost, AchievementStatus } from './socialHost';
