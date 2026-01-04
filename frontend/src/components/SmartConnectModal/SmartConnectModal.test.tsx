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
        expect(screen.queryByText('Smart Connect (AI)')).not.toBeInTheDocument();
    });

    it('renders input area when open and no results', () => {
        render(<SmartConnectModal {...defaultProps} />);
        expect(screen.getByText('New Connection')).toBeInTheDocument();
        expect(screen.getByPlaceholderText(/AI Magic/i)).toBeInTheDocument();
        expect(screen.getByText('Analyze')).toBeInTheDocument();
    });

    it('calls onParse when Analyze is clicked', async () => {
        const mockResult = [{ host: '10.0.0.1', port: 22, user: 'root', name: 'Web Server' }];
        mockOnParse.mockResolvedValue(mockResult);

        render(<SmartConnectModal {...defaultProps} />);
        
        const input = screen.getByPlaceholderText(/AI Magic/i);
        fireEvent.change(input, { target: { value: 'Connect to web server' } });
        
        const analyzeBtn = screen.getByText('Analyze');
        fireEvent.click(analyzeBtn);

        expect(mockOnParse).toHaveBeenCalledWith('Connect to web server');
        
        await waitFor(() => {
            // Note: Header changed to "Connections (1)"
            expect(screen.getByText('Connections (1)')).toBeInTheDocument();
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
        const input = screen.getByPlaceholderText(/AI Magic/i);
        fireEvent.change(input, { target: { value: 'Connect' } });
        fireEvent.click(screen.getByText('Analyze'));

        await waitFor(() => {
            expect(screen.getByText('Connections (1)')).toBeInTheDocument();
        });

        // For single result, it is auto-expanded, so we don't need to click Edit Details
        // But the new UI uses an icon button for toggle (✏️)
        // Actually, logic says: if (parsedConfigs.length === 0 && configsWithName.length === 1) { setExpandedIndices(new Set([0])); }
        // So it should be expanded.

        // Find Host input (it's inside the expanded card)
        const hostInput = screen.getByLabelText('Host');
        const nameInput = screen.getByPlaceholderText('Connection Name') as HTMLInputElement;

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
        fireEvent.change(screen.getByPlaceholderText(/AI Magic/i), { target: { value: 'Connect' } });
        fireEvent.click(screen.getByText('Analyze'));
        await waitFor(() => screen.getByText('Connections (1)'));

        // Auto-expanded for single result
        
        const hostInput = screen.getByLabelText('Host');
        const nameInput = screen.getByPlaceholderText('Connection Name') as HTMLInputElement;

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
        
        fireEvent.change(screen.getByPlaceholderText(/AI Magic/i), { target: { value: 'Connect' } });
        fireEvent.click(screen.getByText('Analyze'));
        await waitFor(() => screen.getByText('Connections (1)'));

        // Auto-expanded

        // Check bastion toggle
        const bastionCheckbox = screen.getByLabelText('Use Bastion Host');
        expect(bastionCheckbox).not.toBeChecked();

        // Enable bastion
        fireEvent.click(bastionCheckbox);
        expect(bastionCheckbox).toBeChecked();

        // Check if bastion fields appear
        expect(screen.getByLabelText('Bastion Host')).toBeInTheDocument();
        expect(screen.getByLabelText('Bastion User')).toBeInTheDocument();
    });

    it('allows adding manual connection', async () => {
        render(<SmartConnectModal {...defaultProps} />);
        
        // Initial state: empty list
        // Note: With new UI, "Found 0 connections" might be visible or a "No connections yet" message.
        // Let's check for the "Add Manual Entry" button
        const addBtn = screen.getByText('+ Add Manual Entry');
        fireEvent.click(addBtn);

        // Should now have 1 item
        // The item is auto-expanded, so we should see "Host" label
        expect(screen.getByLabelText('Host')).toBeInTheDocument();
        expect(screen.getByDisplayValue('New Connection')).toBeInTheDocument();
        
        // Fill it out
        fireEvent.change(screen.getByLabelText('Host'), { target: { value: 'manual-host' } });
        
        // Connect
        const connectBtn = screen.getByText('Connect Selected (1)');
        fireEvent.click(connectBtn);

        expect(mockOnConnect).toHaveBeenCalledWith([
            expect.objectContaining({ host: 'manual-host', name: 'manual-host' })
        ]);
    });
});
