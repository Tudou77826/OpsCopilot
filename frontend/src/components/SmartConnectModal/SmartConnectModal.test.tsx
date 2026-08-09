import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import SmartConnectModal from './SmartConnectModal';
import { vi, describe, it, expect } from 'vitest';

describe('SmartConnectModal', () => {
    const mockOnClose = vi.fn();
    const mockOnConnect = vi.fn();
    const mockOnParse = vi.fn();

    const defaultProps = {
        isOpen: true,
        onClose: mockOnClose,
        onConnect: mockOnConnect,
        onParse: mockOnParse,
    };

    beforeEach(() => {
        vi.clearAllMocks();
    });

    const getConnectInput = () => screen.getByPlaceholderText(/例如：/);

    it('renders nothing when not open', () => {
        render(<SmartConnectModal {...defaultProps} isOpen={false} />);
        expect(screen.queryByText('新建连接')).not.toBeInTheDocument();
    });

    it('renders input area when open and no results', () => {
        render(<SmartConnectModal {...defaultProps} />);
        expect(screen.getByText('新建连接')).toBeInTheDocument();
        expect(getConnectInput()).toBeInTheDocument();
        expect(screen.getByText('智能分析')).toBeInTheDocument();
    });

    it('fills the input from a quick example', () => {
        render(<SmartConnectModal {...defaultProps} />);

        fireEvent.click(screen.getByText('填入跳板机模板'));

        expect(getConnectInput()).toHaveValue(
            [
                '跳板机：<BastionIp>',
                'sopuser / changeme_123',
                '',
                '目标：<TargetIp1~4>',
                'sopuser / changeme_123',
                'root / changeme_123'
            ].join('\n')
        );
    });

    it('calls onParse when Analyze is clicked', async () => {
        const mockResult = [{ host: '10.0.0.1', port: 22, user: 'root', name: 'Web Server' }];
        mockOnParse.mockResolvedValue(mockResult);

        render(<SmartConnectModal {...defaultProps} />);
        
        const input = getConnectInput();
        fireEvent.change(input, { target: { value: 'Connect to web server' } });
        
        const analyzeBtn = screen.getByText('智能分析');
        fireEvent.click(analyzeBtn);

        expect(mockOnParse).toHaveBeenCalledWith('Connect to web server');
        
        await waitFor(() => {
            expect(screen.getByText('连接列表 (1)')).toBeInTheDocument();
        });
        
        // Verify result display
        expect(screen.getByDisplayValue('Web Server')).toBeInTheDocument();
        expect(screen.getByText('10.0.0.1')).toBeInTheDocument();
    });

    it('syncs Name with Host automatically when Name is default', async () => {
        const mockResult = [{ host: '10.0.0.1', port: 22, user: 'root', name: '10.0.0.1' }];
        mockOnParse.mockResolvedValue(mockResult);

        render(<SmartConnectModal {...defaultProps} />);
        
        // Trigger parse flow
        const input = getConnectInput();
        fireEvent.change(input, { target: { value: 'Connect' } });
        fireEvent.click(screen.getByText('智能分析'));

        await waitFor(() => {
            expect(screen.getByText('连接列表 (1)')).toBeInTheDocument();
        });

        const hostInput = screen.getByLabelText('主机地址');
        const nameInput = screen.getByPlaceholderText('连接名称') as HTMLInputElement;

        // Verify initial state
        expect(hostInput).toHaveValue('10.0.0.1');
        expect(nameInput).toHaveValue('10.0.0.1');

        // Change Host -> Name should sync
        fireEvent.change(hostInput, { target: { value: '10.0.0.2' } });
        expect(hostInput).toHaveValue('10.0.0.2');
        expect(nameInput).toHaveValue('10.0.0.2');
    });

    it('does NOT sync Name when Name is manually modified', async () => {
        const mockResult = [{ host: '10.0.0.1', port: 22, user: 'root', name: '10.0.0.1' }];
        mockOnParse.mockResolvedValue(mockResult);

        render(<SmartConnectModal {...defaultProps} />);
        
        // Trigger parse flow
        fireEvent.change(getConnectInput(), { target: { value: 'Connect' } });
        fireEvent.click(screen.getByText('智能分析'));
        await waitFor(() => screen.getByText('连接列表 (1)'));

        // Auto-expanded for single result
        
        const hostInput = screen.getByLabelText('主机地址');
        const nameInput = screen.getByPlaceholderText('连接名称') as HTMLInputElement;

        // Manually change Name
        fireEvent.change(nameInput, { target: { value: 'My Custom Server' } });
        expect(nameInput).toHaveValue('My Custom Server');

        // Change Host -> Name should NOT change
        fireEvent.change(hostInput, { target: { value: '10.0.0.99' } });
        expect(hostInput).toHaveValue('10.0.0.99');
        expect(nameInput).toHaveValue('My Custom Server');
    });

    it('handles Bastion toggle correctly', async () => {
        const mockResult = [{ host: '10.0.0.1', port: 22, user: 'root' }]; // No bastion initially
        mockOnParse.mockResolvedValue(mockResult);

        render(<SmartConnectModal {...defaultProps} />);
        
        fireEvent.change(getConnectInput(), { target: { value: 'Connect' } });
        fireEvent.click(screen.getByText('智能分析'));
        await waitFor(() => screen.getByText('连接列表 (1)'));

        // Auto-expanded

        // Check bastion toggle
        const bastionCheckbox = screen.getByLabelText(/使用跳板机/i);
        expect(bastionCheckbox).not.toBeChecked();

        // Enable bastion
        fireEvent.click(bastionCheckbox);
        expect(bastionCheckbox).toBeChecked();

        // Check if bastion fields appear
        expect(screen.getByLabelText('跳板机主机')).toBeInTheDocument();
        expect(screen.getByLabelText('跳板机用户')).toBeInTheDocument();
    });

    it('allows adding manual connection', async () => {
        render(<SmartConnectModal {...defaultProps} />);
        
        const addBtn = screen.getByText('手动添加');
        fireEvent.click(addBtn);

        // Should now have 1 item
        expect(screen.getByLabelText('主机地址')).toBeInTheDocument();
        expect(screen.getByDisplayValue('新连接')).toBeInTheDocument();
        
        // Fill it out
        fireEvent.change(screen.getByLabelText('主机地址'), { target: { value: 'manual-host' } });
        
        // Connect
        const connectBtn = screen.getByText('连接选中项 (1)');
        fireEvent.click(connectBtn);

        expect(mockOnConnect).toHaveBeenCalledWith([
            expect.objectContaining({ host: 'manual-host', name: 'manual-host' })
        ]);
    });

    it('handles null response from onParse gracefully', async () => {
        // Simulate backend returning null (which might happen on certain errors or empty results)
        mockOnParse.mockResolvedValue(null);

        render(<SmartConnectModal {...defaultProps} />);
        
        const input = getConnectInput();
        fireEvent.change(input, { target: { value: 'Connect to nowhere' } });
        
        const analyzeBtn = screen.getByText('智能分析');
        fireEvent.click(analyzeBtn);

        expect(mockOnParse).toHaveBeenCalledWith('Connect to nowhere');
        
        // Should show error message instead of crashing
        await waitFor(() => {
            expect(screen.getByText(/未识别到连接信息/i)).toBeInTheDocument();
        });
    });

    it('handles TLS timeout error gracefully', async () => {
        // Simulate backend network error
        mockOnParse.mockRejectedValue(new Error('Post "https://...": net/http: TLS handshake timeout'));

        render(<SmartConnectModal {...defaultProps} />);

        const input = getConnectInput();
        fireEvent.change(input, { target: { value: 'Connect to somewhere' } });
        fireEvent.click(screen.getByText('智能分析'));

        // Should show friendly error message
        await waitFor(() => {
            expect(screen.getByText(/连接超时：无法连接到 AI 服务/i)).toBeInTheDocument();
        });
    });

    it('keeps input text after a successful parse (#53-2)', async () => {
        // 解析成功后不应清空输入框：用户可能只是打错字，想改完重新点解析。
        mockOnParse.mockResolvedValue([{ host: '10.0.0.1', port: 22, user: 'root', name: '10.0.0.1' }]);

        render(<SmartConnectModal {...defaultProps} />);

        const input = getConnectInput();
        fireEvent.change(input, { target: { value: '连接到 10.0.0.1' } });
        fireEvent.click(screen.getByText('智能分析'));

        await waitFor(() => expect(screen.getByText('连接列表 (1)')).toBeInTheDocument());

        // 解析成功后输入框文本应保留（此时输入框已切换为 compact 形态，placeholder 变化）
        const compactInput = screen.getByPlaceholderText(/继续输入连接描述/);
        expect(compactInput).toHaveValue('连接到 10.0.0.1');
    });

    it('prefills configs from initialConfigs prop (#53-1)', () => {
        // 连接失败带回配置：打开时用 initialConfigs 预填列表、全选并展开（单条）。
        render(
            <SmartConnectModal
                {...defaultProps}
                initialConfigs={[{ host: '192.168.1.1', port: 22, user: 'root', name: '失败连接' }]}
            />
        );

        // 预填的配置应出现在列表中并被选中
        expect(screen.getByText('连接列表 (1)')).toBeInTheDocument();
        expect(screen.getByText('已选 1')).toBeInTheDocument();
        expect(screen.getByDisplayValue('失败连接')).toBeInTheDocument();
        // 单条应自动展开编辑表单
        expect(screen.getByLabelText('主机地址')).toHaveValue('192.168.1.1');
    });
});
