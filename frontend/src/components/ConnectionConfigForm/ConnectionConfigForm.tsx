import React from 'react';
import { ConnectionConfig, Protocol, PROTOCOL_DEFAULT_PORT, normalizeProtocol } from '../../types';

type Props = {
    config: ConnectionConfig;
    onChange: (next: ConnectionConfig) => void;
    idPrefix?: string;
    showName?: boolean;
    showGroup?: boolean;
};

const ConnectionConfigForm: React.FC<Props> = ({ config, onChange, idPrefix = 'cfg', showName = true, showGroup = true }) => {
    // 当前协议(归一化:空值视为 ssh)
    const protocol: Protocol = normalizeProtocol(config.protocol);
    // telnet 模式:隐藏 Root 密码(无标准 sudo)和 bastion(无标准堡垒机)
    const isTelnet = protocol === 'telnet';

    const updateRoot = (field: keyof ConnectionConfig, value: any) => {
        if (field === 'host') {
            const next: ConnectionConfig = { ...config, host: value };
            if (!config.name || config.name === config.host || config.name === '新连接') {
                next.name = value;
            }
            onChange(next);
            return;
        }
        onChange({ ...config, [field]: value });
    };

    // 切换协议时的联动:
    //  - 端口:若当前端口等于旧协议默认值,自动切到新协议默认值;
    //    用户手改过的端口(非默认值)则保留,避免覆盖用户意图。
    //  - 不清空 rootPassword/bastion 字段值,仅 UI 隐藏(切回 SSH 时仍在)。
    const handleProtocolChange = (next: Protocol) => {
        if (next === protocol) return;
        const updated: ConnectionConfig = { ...config, protocol: next };
        const oldDefault = PROTOCOL_DEFAULT_PORT[protocol];
        const newDefault = PROTOCOL_DEFAULT_PORT[next];
        if (config.port === oldDefault || !config.port) {
            updated.port = newDefault;
        }
        onChange(updated);
    };

    const updateBastion = (field: keyof ConnectionConfig, value: any) => {
        const bastion = config.bastion ?? { host: '', port: 22, user: '', name: 'Bastion' };
        onChange({ ...config, bastion: { ...bastion, [field]: value } });
    };

    const renderField = (label: string, value: string | number, onValueChange: (val: string) => void, type: string = 'text', placeholder: string = '', id?: string) => (
        <div style={styles.fieldGroup}>
            <label style={styles.fieldLabel} htmlFor={id}>{label}</label>
            <input
                id={id}
                type={type}
                value={value}
                onChange={(e) => onValueChange(e.target.value)}
                style={styles.input}
                placeholder={placeholder}
            />
        </div>
    );

    return (
        <div>
            {showName && (
                <div style={styles.row}>
                    <div style={{ flex: 1 }}>
                        {renderField('连接名称', config.name || '', (v) => updateRoot('name', v), 'text', '', `${idPrefix}-name`)}
                    </div>
                </div>
            )}

            {showGroup && (
                <div style={styles.row}>
                    <div style={{ flex: 1 }}>
                        {renderField('分组', config.group || '', (v) => updateRoot('group', v), 'text', '可选（用于会话管理分组）', `${idPrefix}-group`)}
                    </div>
                </div>
            )}

            {/* 协议选择:SSH / Telnet。切换时联动端口默认值与字段显隐。 */}
            <div style={styles.row}>
                <div style={styles.fieldGroup}>
                    <label style={styles.fieldLabel}>协议</label>
                    <div style={styles.protocolSwitch}>
                        <button
                            type="button"
                            style={protocol === 'ssh' ? styles.protocolBtnActive : styles.protocolBtn}
                            onClick={() => handleProtocolChange('ssh')}
                        >
                            SSH
                        </button>
                        <button
                            type="button"
                            style={protocol === 'telnet' ? styles.protocolBtnActive : styles.protocolBtn}
                            onClick={() => handleProtocolChange('telnet')}
                        >
                            Telnet
                        </button>
                    </div>
                </div>
            </div>

            <div style={styles.row}>
                <div style={{ flex: 2 }}>
                    {renderField('主机地址', config.host, (v) => updateRoot('host', v), 'text', '', `${idPrefix}-host`)}
                </div>
                <div style={{ flex: 1 }}>
                    {renderField('端口', config.port, (v) => updateRoot('port', parseInt(v) || PROTOCOL_DEFAULT_PORT[protocol]), 'number', '', `${idPrefix}-port`)}
                </div>
            </div>
            <div style={styles.row}>
                <div style={{ flex: 1 }}>
                    {renderField('用户名', config.user, (v) => updateRoot('user', v), 'text', '', `${idPrefix}-user`)}
                </div>
                <div style={{ flex: 1 }}>
                    {renderField('密码', config.password || '', (v) => updateRoot('password', v), 'password', '', `${idPrefix}-password`)}
                </div>
            </div>
            {/* Root 密码:仅 SSH 显示(telnet 无标准 sudo 流程)。值保留在对象里,切回 SSH 仍在。 */}
            {!isTelnet && (
                <div style={styles.row}>
                    <div style={{ flex: 1 }}>
                        {renderField('Root 密码', config.rootPassword || '', (v) => updateRoot('rootPassword', v), 'password', '可选 (用于 sudo)', `${idPrefix}-root-password`)}
                    </div>
                </div>
            )}

            {/* 跳板机:仅 SSH 显示(telnet 实践不走 SSH 堡垒机)。值保留,切回 SSH 仍在。 */}
            {!isTelnet && (
                <div style={styles.bastionSection}>
                    <label style={styles.bastionHeader}>
                        <input
                            type="checkbox"
                            checked={!!config.bastion}
                            onChange={(e) => {
                                if (e.target.checked) {
                                    updateBastion('host', '');
                                } else {
                                    const next = { ...config };
                                    delete next.bastion;
                                    onChange(next);
                                }
                            }}
                            style={{ marginRight: '8px' }}
                        />
                        <span>使用跳板机 (Bastion)</span>
                    </label>
                    {config.bastion && (
                        <div style={styles.bastionBody}>
                            <div style={styles.row}>
                                <div style={{ flex: 2 }}>
                                    {renderField('跳板机主机', config.bastion.host, (v) => updateBastion('host', v), 'text', '', `${idPrefix}-bastion-host`)}
                                </div>
                                <div style={{ flex: 1 }}>
                                    {renderField('跳板机端口', config.bastion.port, (v) => updateBastion('port', parseInt(v) || 22), 'number', '', `${idPrefix}-bastion-port`)}
                                </div>
                            </div>
                            <div style={styles.row}>
                                <div style={{ flex: 1 }}>
                                    {renderField('跳板机用户', config.bastion.user, (v) => updateBastion('user', v), 'text', '', `${idPrefix}-bastion-user`)}
                                </div>
                                <div style={{ flex: 1 }}>
                                    {renderField('跳板机密码', config.bastion.password || '', (v) => updateBastion('password', v), 'password', '', `${idPrefix}-bastion-password`)}
                                </div>
                            </div>
                        </div>
                    )}
                </div>
            )}
        </div>
    );
};

const styles = {
    row: {
        display: 'flex',
        gap: '12px',
        marginBottom: '12px',
    },
    fieldGroup: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '4px',
    },
    fieldLabel: {
        fontSize: '0.8rem',
        color: '#ccc',
    },
    protocolSwitch: {
        display: 'flex',
        gap: '0',
        borderRadius: '4px',
        overflow: 'hidden' as const,
        border: '1px solid #444',
        width: 'fit-content',
        // 与 input 等高,避免表单内高度参差
        height: '34px',
    },
    // 协议按钮用 segmented control 风格:激活态用浅底 + 白字(不用主操作蓝,
    // 那是"执行动作"的主按钮专用色,这里只是配置选择,视觉层级要低一档)。
    protocolBtn: {
        padding: '0 16px',
        backgroundColor: '#1e1e1e',
        color: '#9a9a9a',
        border: 'none',
        cursor: 'pointer',
        fontSize: '0.85rem',
        height: '100%',
    },
    protocolBtnActive: {
        padding: '0 16px',
        backgroundColor: '#3a3d41',
        color: '#fff',
        border: 'none',
        cursor: 'pointer',
        fontSize: '0.85rem',
        height: '100%',
    },
    input: {
        padding: '8px',
        borderRadius: '4px',
        border: '1px solid #444',
        backgroundColor: '#1e1e1e',
        color: '#fff',
        fontSize: '0.9rem',
        boxSizing: 'border-box' as const,
        outline: 'none',
    },
    bastionSection: {
        marginTop: '8px',
        border: '1px solid #444',
        borderRadius: '4px',
        overflow: 'hidden',
    },
    bastionHeader: {
        display: 'flex',
        alignItems: 'center',
        padding: '10px',
        backgroundColor: '#333',
        cursor: 'pointer',
        userSelect: 'none' as const,
    },
    bastionBody: {
        padding: '12px',
        backgroundColor: '#2a2a2a',
    },
};

export default ConnectionConfigForm;
