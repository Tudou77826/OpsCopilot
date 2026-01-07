export interface ConnectionConfig {
    name?: string;
    host: string;
    port: number;
    user: string;
    password?: string;
    rootPassword?: string;
    bastion?: ConnectionConfig;
    group?: string;
}
