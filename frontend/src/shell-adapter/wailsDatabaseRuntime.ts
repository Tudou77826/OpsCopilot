import * as api from '../../wailsjs/go/main/App';
import type { DatabaseRuntime } from '../../../frontend-shell/src/ui/database/types';

export const wailsDatabaseRuntime: DatabaseRuntime = {
  jdbcStatus: api.DatabaseJDBCStatus,
  importJDBC: api.DatabaseImportJDBC,
  deleteJDBC: api.DatabaseDeleteJDBC,
  chooseJDBCJars: api.DatabaseChooseJDBCJars,
  profiles: api.DatabaseProfiles,
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
};
