import { render, screen, fireEvent } from '@testing-library/react';
import ConnectionModal from './ConnectionModal';
import { vi, describe, it, expect } from 'vitest';

describe('ConnectionModal', () => {
  it('renders nothing when not open', () => {
    render(<ConnectionModal isOpen={false} onClose={vi.fn()} onConnect={vi.fn()} />);
    expect(screen.queryByText('New Connection')).not.toBeInTheDocument();
  });

  it('renders form fields when open', () => {
    render(<ConnectionModal isOpen={true} onClose={vi.fn()} onConnect={vi.fn()} />);
    expect(screen.getByText('New Connection')).toBeInTheDocument();
    expect(screen.getByLabelText('Host')).toBeInTheDocument();
    expect(screen.getByLabelText('Port')).toBeInTheDocument();
    expect(screen.getByLabelText('User')).toBeInTheDocument();
    expect(screen.getByLabelText('Password')).toBeInTheDocument();
    // New fields
    expect(screen.getByLabelText('Root Password (Optional)')).toBeInTheDocument();
    expect(screen.getByText('Bastion Host (Optional)')).toBeInTheDocument();
  });

  it('renders Name field', () => {
    render(<ConnectionModal isOpen={true} onClose={vi.fn()} onConnect={vi.fn()} />);
    expect(screen.getByLabelText('Name (Optional)')).toBeInTheDocument();
  });

  it('syncs Name with Host if Name is untouched', () => {
    render(<ConnectionModal isOpen={true} onClose={vi.fn()} onConnect={vi.fn()} />);
    
    const hostInput = screen.getByLabelText('Host');
    const nameInput = screen.getByLabelText('Name (Optional)') as HTMLInputElement;

    // Type in host
    fireEvent.change(hostInput, { target: { value: '192.168.1.1' } });
    expect(nameInput.value).toBe('192.168.1.1');

    // Type more
    fireEvent.change(hostInput, { target: { value: '192.168.1.100' } });
    expect(nameInput.value).toBe('192.168.1.100');
  });

  it('does NOT sync Name with Host if Name has been manually edited', () => {
    render(<ConnectionModal isOpen={true} onClose={vi.fn()} onConnect={vi.fn()} />);
    
    const hostInput = screen.getByLabelText('Host');
    const nameInput = screen.getByLabelText('Name (Optional)') as HTMLInputElement;

    // Manually edit name
    fireEvent.change(nameInput, { target: { value: 'My Server' } });

    // Type in host
    fireEvent.change(hostInput, { target: { value: '192.168.1.1' } });
    
    // Name should remain 'My Server'
    expect(nameInput.value).toBe('My Server');
  });

  it('submits Name in config', () => {
    const onConnect = vi.fn();
    render(<ConnectionModal isOpen={true} onClose={vi.fn()} onConnect={onConnect} />);

    fireEvent.change(screen.getByLabelText('Name (Optional)'), { target: { value: 'Production DB' } });
    fireEvent.change(screen.getByLabelText('Host'), { target: { value: '10.0.0.1' } });
    
    const form = screen.getByText('New Connection').closest('div')?.querySelector('form');
    if (form) {
        fireEvent.submit(form);
    }

    expect(onConnect).toHaveBeenCalledWith(expect.objectContaining({
        name: 'Production DB',
        host: '10.0.0.1'
    }));
  });

  it('submits correct data structure with recursion', () => {
    const onConnect = vi.fn();
    render(<ConnectionModal isOpen={true} onClose={vi.fn()} onConnect={onConnect} />);

    // Fill main config
    fireEvent.change(screen.getByLabelText('Host'), { target: { value: 'target-host' } });
    fireEvent.change(screen.getByLabelText('Root Password (Optional)'), { target: { value: 'root-secret' } });

    // Toggle bastion
    // Note: Use fireEvent.click on the checkbox specifically, or ensure the label click propagates
    // Debugging: let's verify checkbox state
    const checkbox = screen.getByLabelText('Bastion Host (Optional)');
    fireEvent.click(checkbox);
    
    // Fill bastion config
    // Note: In a real test we might need test-ids to distinguish bastion fields from main fields if labels are same
    // For simplicity, we assume unique labels or test-ids in implementation
    const bastionHostInputs = screen.getAllByLabelText('Host');
    // Ensure the second input (bastion) is found before firing event
    if (bastionHostInputs.length > 1) {
        fireEvent.change(bastionHostInputs[1], { target: { value: 'jump-host' } });
    } else {
        throw new Error("Bastion host input not found");
    }

    // Submit form (use submit button)
    // Note: Since handleSubmit calls preventDefault, and we are using fireEvent.click on submit button,
    // this might not trigger form submission in JSDOM environment properly if the button is outside form or similar.
    // Let's try firing submit on the form directly as a robust fallback.
    const form = screen.getByText('New Connection').closest('div')?.querySelector('form');
    if (form) {
        fireEvent.submit(form);
    } else {
        // Fallback to button click if form not found via DOM traversal (unlikely)
        const submitBtn = screen.getByRole('button', { name: /connect/i });
        fireEvent.click(submitBtn);
    }

    expect(onConnect).toHaveBeenCalledWith(expect.objectContaining({
        host: 'target-host',
        rootPassword: 'root-secret',
        bastion: expect.objectContaining({
            host: 'jump-host'
        })
    }));
  });
});
