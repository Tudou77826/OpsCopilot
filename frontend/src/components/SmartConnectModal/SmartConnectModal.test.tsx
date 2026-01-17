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

    it('renders nothing when not open', () => {
        render(<SmartConnectModal {...defaultProps} isOpen={false} />);
        expect(screen.queryByText('新建连接')).not.toBeInTheDocument();
    });

    it('renders input area when open and no results', () => {
        render(<SmartConnectModal {...defaultProps} />);
        expect(screen.getByText('新建连接')).toBeInTheDocument();
        expect(screen.getByPlaceholderText(/你可以使用自然输入连接要求/i)).toBeInTheDocument();
        expect(screen.getByText('智能分析')).toBeInTheDocument();
    });

    it('calls onParse when Analyze is clicked', async () => {
        const mockResult = [{ host: '10.0.0.1', port: 22, user: 'root', name: 'Web Server' }];
        mockOnParse.mockResolvedValue(mockResult);

        render(<SmartConnectModal {...defaultProps} />);
        
        const input = screen.getByPlaceholderText(/你可以使用自然输入连接要求/i);
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
        const input = screen.getByPlaceholderText(/你可以使用自然输入连接要求/i);
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
        fireEvent.change(screen.getByPlaceholderText(/你可以使用自然输入连接要求/i), { target: { value: 'Connect' } });
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
        
        fireEvent.change(screen.getByPlaceholderText(/你可以使用自然输入连接要求/i), { target: { value: 'Connect' } });
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
        
        const addBtn = screen.getByText('+ 手动添加');
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
        
        const input = screen.getByPlaceholderText(/你可以使用自然输入连接要求/i);
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
        
        const input = screen.getByPlaceholderText(/你可以使用自然输入连接要求/i);
        fireEvent.change(input, { target: { value: 'Connect to somewhere' } });
        fireEvent.click(screen.getByText('智能分析'));

        // Should show friendly error message
        await waitFor(() => {
            expect(screen.getByText(/连接超时：无法连接到 AI 服务/i)).toBeInTheDocument();
        });
    });
});
