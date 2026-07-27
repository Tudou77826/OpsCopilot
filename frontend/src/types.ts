// 协议类型。空值或 'ssh' 走 SSH;'telnet' 走 Telnet。
// 与后端 remote.ProtocolSSH / remote.ProtocolTelnet 对齐。
export type Protocol = 'ssh' | 'telnet';

export interface ConnectionConfig {
    name?: string;
    protocol?: Protocol;
    host: string;
    port: number;
    user: string;
    password?: string;
    rootPassword?: string;
    bastion?: ConnectionConfig;
    group?: string;
}

// 协议默认端口。切换协议时若当前端口等于旧协议默认值则自动切换。
export const PROTOCOL_DEFAULT_PORT: Record<Protocol, number> = {
    ssh: 22,
    telnet: 23,
};

// 协议中文标签(用于 chip 展示)。
export const PROTOCOL_LABEL: Record<Protocol, string> = {
    ssh: 'SSH',
    telnet: 'Telnet',
};

// 归一化 protocol:空值视为 'ssh'(与后端 NormalizedProtocol 对齐)。
export function normalizeProtocol(p?: Protocol | string): Protocol {
    return p === 'telnet' ? 'telnet' : 'ssh';
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
