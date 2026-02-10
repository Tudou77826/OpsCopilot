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

export enum SessionStatus {
    CONNECTED = 'connected',
    DISCONNECTED = 'disconnected',
}

export interface SessionDisconnectedEvent {
    sessionId: string;
    reason: string;
    message: string;
    timestamp: number;
}
