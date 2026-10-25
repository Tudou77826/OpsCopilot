import type {
  DatabaseProfile,
  DatabaseSession,
  DatabaseQueryResult,
} from "../ui/database/types";

export interface DatabaseBlock {
  id: string;
  purpose: string;
  sql: string;
  at: string;
  status: string;
  result: DatabaseQueryResult;
  error?: string;
}
export interface AssistantDatabaseWorkspace {
  id: string;
  profile: DatabaseProfile;
  session: DatabaseSession;
  schemas?: string[];
  sql: string;
  blocks: DatabaseBlock[];
  ownerTurn: string;
  pending: string;
  connected: boolean;
  dismissed: boolean;
  revision: number;
  revealBlock?: string;
  revealNonce?: string;
}
export interface DatabaseWorkspaceUpdate {
  revision: number;
  items: AssistantDatabaseWorkspace[];
}
export interface ManagedDatabase {
  workspace: AssistantDatabaseWorkspace;
  editing: boolean;
  onEdit(sql: string): void;
  onPresented(sql: string): void;
}
