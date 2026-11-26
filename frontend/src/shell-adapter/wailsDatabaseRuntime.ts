import * as api from '../../wailsjs/go/main/App';
import type {
  DatabaseRuntime,
  SharedDatabaseEntry,
  SharedDatabaseImportResult,
} from '../../../frontend-shell/src/ui/database/types';

type WailsSharedDBWindow = {
  go?: {
    main?: {
      App?: {
        GetSharedDatabases?: () => Promise<string>;
        ImportSharedDatabase?: (entryKey: string) => Promise<SharedDatabaseImportResult>;
        RemoveSharedDatabase?: (entryKey: string) => Promise<string>;
      };
    };
  };
};

export const wailsDatabaseRuntime: DatabaseRuntime = {
  jdbcStatus: api.DatabaseJDBCStatus,
  importJDBC: api.DatabaseImportJDBC,
  deleteJDBC: api.DatabaseDeleteJDBC,
  chooseJDBCJars: api.DatabaseChooseJDBCJars,
  profiles: api.DatabaseProfiles,
  password: api.DatabasePassword,
  groups: api.DatabaseGroups,
  createGroup: api.DatabaseCreateGroup,
  renameGroup: api.DatabaseRenameGroup,
  deleteGroup: api.DatabaseDeleteGroup,
  save: api.DatabaseSave,
  test: api.DatabaseTest,
  remove: api.DatabaseDelete,
  gateways: api.DatabaseGateways,
  connect: api.DatabaseConnect,
  disconnect: api.DatabaseDisconnect,
  schemas: api.DatabaseSchemas,
  tables: api.DatabaseTables,
  structure: api.DatabaseStructure,
  generateSQL: api.DatabaseGenerateSQL,
  browse: api.DatabaseBrowse,
  query: api.DatabaseQuery,
  cancel: api.DatabaseCancel,
  shared: {
    async list() {
      const raw = await (window as WailsSharedDBWindow).go?.main?.App?.GetSharedDatabases?.();
      if (!raw) return { enabled: false, databases: [] };
      const parsed = JSON.parse(raw);
      return {
        enabled: !!parsed?.enabled,
        databases: (Array.isArray(parsed?.databases) ? parsed.databases : []) as SharedDatabaseEntry[],
      };
    },
    async importToLocal(entryKey: string) {
      const res = await (window as WailsSharedDBWindow).go?.main?.App?.ImportSharedDatabase?.(entryKey);
      return res ?? { success: false, message: '当前环境不支持导入共享数据库' };
    },
    async remove(entryKey: string) {
      const err = await (window as WailsSharedDBWindow).go?.main?.App?.RemoveSharedDatabase?.(entryKey);
      if (err) throw new Error(err);
    },
  },
};
