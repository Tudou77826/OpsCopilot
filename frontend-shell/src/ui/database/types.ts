export interface DatabaseProfile {
  id: string;
  revision: number;
  name: string;
  environment: string;
  group?: string;
  engine: string;
  driverId?: string;
  jdbcAllowPublicKeyRetrieval?: boolean;
  host: string;
  port: number;
  user: string;
  database: string;
  tls: string;
  readOnly: boolean;
  gatewayId: string;
  gatewayMode?: string;
  passwordSet: boolean;
}
export interface DatabaseProfileInput extends DatabaseProfile {
  password?: string;
}
export interface DatabaseSession {
  id: string;
  profileId: string;
  version: string;
  database: string;
  readOnly: boolean;
}
export interface DatabaseColumn {
  name: string;
  type: string;
}
export interface DatabaseResultSet {
  statement: string;
  columns: DatabaseColumn[];
  rows: (string | null)[][];
  affectedRows: number;
  truncated: boolean;
}
export interface DatabaseQueryResult {
  sets: DatabaseResultSet[];
  durationMs: number;
  error: string;
  cancelled: boolean;
  disconnected: boolean;
}
export interface DatabaseQuery {
  sessionId: string;
  requestId: string;
  database: string;
  sql: string;
  mode?: 'statement' | 'selection' | 'script';
  cursorOffset?: number;
  limit: number;
  timeoutSeconds: number;
}

export interface SQLExecution {
  mode: 'statement' | 'selection' | 'script';
  sql?: string;
  cursorOffset?: number;
}
export interface DatabaseTable {
  name: string;
  type: string;
  comment: string;
}
export interface DatabaseTableRequest {
  requestId: string;
  sessionId: string;
  database: string;
  table: string;
  offset: number;
  limit: number;
}
export interface DatabaseStructure {
  columns: DatabaseResultSet;
  indexes: DatabaseResultSet;
  ddl: string;
}
export interface DatabaseGateway {
  id: string;
  name: string;
  host: string;
}
export interface DatabaseRuntime {
  jdbcStatus(): Promise<DatabaseJDBCStatus>;
  importJDBC(input: DatabaseJDBCImport): Promise<DatabaseJDBCDriver>;
  deleteJDBC(id: string): Promise<void>;
  chooseJDBCJars(): Promise<string[]>;
  profiles(): Promise<DatabaseProfile[]>;
  groups(): Promise<string[]>;
  createGroup(name: string): Promise<void>;
  renameGroup(oldName: string, newName: string): Promise<void>;
  deleteGroup(name: string): Promise<void>;
  save(input: DatabaseProfileInput): Promise<DatabaseProfile>;
  test(input: DatabaseProfileInput): Promise<string>;
  remove(id: string, revision: number): Promise<void>;
  gateways(): Promise<DatabaseGateway[]>;
  connect(id: string): Promise<DatabaseSession>;
  disconnect(id: string): Promise<void>;
  schemas(id: string): Promise<string[]>;
  tables(id: string, database: string): Promise<DatabaseTable[]>;
  structure(request: DatabaseTableRequest): Promise<DatabaseStructure>;
  generateSQL(request: DatabaseSQLTemplate): Promise<string>;
  browse(request: DatabaseTableRequest): Promise<DatabaseQueryResult>;
  query(request: DatabaseQuery): Promise<DatabaseQueryResult>;
  cancel(id: string): Promise<boolean>;
}

export interface DatabaseJDBCDriver {
  id: string;
  name: string;
  className: string;
  urlTemplate: string;
  dialect: string;
  version: string;
  jars: { name: string; sha256: string }[];
}
export interface DatabaseJDBCStatus {
  available: boolean;
  message: string;
  drivers: DatabaseJDBCDriver[];
}
export interface DatabaseJDBCImport {
  name: string;
  className: string;
  urlTemplate: string;
  dialect: string;
  paths: string[];
}

export interface DatabaseSQLTemplate {
  sessionId: string;
  database: string;
  table: string;
  kind: 'name' | 'qualified-name' | 'select' | 'insert' | 'update' | 'delete';
  column?: string;
}
