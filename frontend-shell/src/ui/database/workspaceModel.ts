import type {
  DatabaseSession,
  DatabaseStructure,
  DatabaseQueryResult,
  DatabaseTable,
} from './types';

export interface ConnectionState {
  session: DatabaseSession;
  schemas: string[];
  tables: Record<string, DatabaseTable[]>;
  structures: Record<string, DatabaseStructure>;
}
export interface QueryTab {
  id: string;
  profileId: string;
  schema: string;
  title: string;
  sql: string;
  table?: string;
  offset: number;
  view: 'query' | 'data' | 'columns' | 'indexes' | 'ddl';
  result?: DatabaseQueryResult;
  structure?: DatabaseStructure;
  resultIndex: number;
  pending?: string;
  error?: string;
}
export type DatabaseNode =
  | { kind: 'connection'; profileId: string }
  | { kind: 'database'; profileId: string; database: string }
  | {
      kind: 'table';
      profileId: string;
      database: string;
      table: string;
      tableType: string;
    }
  | {
      kind: 'column';
      profileId: string;
      database: string;
      table: string;
      column: string;
    };

// JSON tuples avoid collisions when identifiers themselves contain punctuation.
export const nodeKey = (node: DatabaseNode) => JSON.stringify(node);
export const objectKey = (database: string, table: string) =>
  JSON.stringify([database, table]);
export const errorText = (e: unknown) => String(e).replace(/^Error:\s*/, '');
export const systemDatabase = (name: string) =>
  ['information_schema', 'mysql', 'sys', 'performance_schema'].includes(name);
