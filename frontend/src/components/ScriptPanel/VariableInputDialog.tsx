import React, { useEffect, useState } from 'react';

interface ScriptVariable {
    name: string;
    display_name: string;
    default_value: string;
    required: boolean;
    description: string;
}

interface VariableInputDialogProps {
    isOpen: boolean;
    variables: ScriptVariable[];
    onSubmit: (values: Record<string, string>) => void;
    onCancel: () => void;
}

const VariableInputDialog: React.FC<VariableInputDialogProps> = ({
    isOpen,
    variables,
    onSubmit,
    onCancel,
}) => {
    const [values, setValues] = useState<Record<string, string>>({});
    const [errors, setErrors] = useState<Record<string, string>>({});

    // 每次打开对话框时，用默认值初始化
    useEffect(() => {
        if (isOpen) {
            const init: Record<string, string> = {};
            variables.forEach((v) => {
                init[v.name] = v.default_value || '';
            });
            setValues(init);
            setErrors({});
        }
    }, [isOpen, variables]);

    if (!isOpen) return null;

    const handleSubmit = () => {
        const newErrors: Record<string, string> = {};
        variables.forEach((v) => {
            if (v.required && !values[v.name]?.trim()) {
                newErrors[v.name] = '此变量为必填项';
            }
        });

        if (Object.keys(newErrors).length > 0) {
            setErrors(newErrors);
            return;
        }

        setErrors({});
        onSubmit(values);
    };

    return (
        <div style={styles.overlay}>
            <div style={styles.modal}>
                <div style={styles.header}>
                    <h2 style={styles.title}>设置脚本变量</h2>
                    <button style={styles.closeButton} onClick={onCancel}>x</button>
                </div>

                <div style={styles.body}>
                    <p style={styles.hint}>请为脚本中的变量设置值：</p>

                    {variables.map((v) => (
                        <div key={v.name} style={styles.fieldGroup}>
                            <label style={styles.label}>
                                {v.display_name || v.name}
                                {v.required && <span style={styles.required}>*</span>}
                            </label>
                            {v.description && (
                                <span style={styles.description}>{v.description}</span>
                            )}
                            <input
                                style={{
                                    ...styles.input,
                                    borderColor: errors[v.name] ? '#f44336' : '#3e3e42',
                                }}
                                type="text"
                                value={values[v.name] || ''}
                                onChange={(e) =>
                                    setValues({ ...values, [v.name]: e.target.value })
                                }
                                placeholder={v.default_value || `输入 ${v.display_name || v.name}`}
                            />
                            {errors[v.name] && (
                                <span style={styles.error}>{errors[v.name]}</span>
                            )}
                        </div>
                    ))}
                </div>

                <div style={styles.footer}>
                    <button style={styles.cancelButton} onClick={onCancel}>
                        取消
                    </button>
                    <button style={styles.submitButton} onClick={handleSubmit}>
                        开始回放
                    </button>
                </div>
            </div>
        </div>
    );
};

const styles: Record<string, React.CSSProperties> = {
    overlay: {
        position: 'fixed' as const,
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.75)',
        backdropFilter: 'blur(4px)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 6000,
    },
    modal: {
        width: '480px',
        maxHeight: '70vh',
        backgroundColor: '#1e1e1e',
        borderRadius: '12px',
        border: '1px solid #3e3e42',
        boxShadow: '0 20px 60px rgba(0, 0, 0, 0.5)',
        display: 'flex',
        flexDirection: 'column' as const,
    },
    header: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        padding: '16px 20px',
        borderBottom: '1px solid #3e3e42',
    },
    title: {
        margin: 0,
        fontSize: '16px',
        fontWeight: 600,
        color: '#ffffff',
    },
    closeButton: {
        width: '32px',
        height: '32px',
        padding: 0,
        backgroundColor: 'transparent',
        border: 'none',
        color: '#858585',
        fontSize: '18px',
        cursor: 'pointer',
        borderRadius: '6px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
    },
    body: {
        flex: 1,
        overflowY: 'auto' as const,
        padding: '20px',
    },
    hint: {
        margin: '0 0 16px 0',
        fontSize: '13px',
        color: '#858585',
    },
    fieldGroup: {
        marginBottom: '16px',
    },
    label: {
        display: 'block',
        fontSize: '13px',
        color: '#cccccc',
        marginBottom: '4px',
        fontWeight: 500,
    },
    required: {
        color: '#f44336',
        marginLeft: '4px',
    },
    description: {
        display: 'block',
        fontSize: '11px',
        color: '#757575',
        marginBottom: '4px',
    },
    input: {
        width: '100%',
        padding: '8px 12px',
        backgroundColor: '#252526',
        border: '1px solid #3e3e42',
        borderRadius: '6px',
        color: '#ffffff',
        fontSize: '13px',
        boxSizing: 'border-box' as const,
        outline: 'none',
    },
    error: {
        display: 'block',
        fontSize: '11px',
        color: '#f44336',
        marginTop: '4px',
    },
    footer: {
        display: 'flex',
        justifyContent: 'flex-end',
        gap: '10px',
        padding: '16px 20px',
        borderTop: '1px solid #3e3e42',
    },
    cancelButton: {
        padding: '8px 16px',
        backgroundColor: 'transparent',
        color: '#cccccc',
        border: '1px solid #4d4d4d',
        borderRadius: '6px',
        cursor: 'pointer',
        fontSize: '13px',
    },
    submitButton: {
        padding: '8px 16px',
        backgroundColor: '#007acc',
        color: '#ffffff',
        border: 'none',
        borderRadius: '6px',
        cursor: 'pointer',
        fontSize: '13px',
        fontWeight: 500,
    },
};

export default VariableInputDialog;
