import React from 'react';
import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import BottomBar, { BOTTOM_BAR_TIPS } from './BottomBar';

describe('BottomBar', () => {
    it('shows the first terminal tip', () => {
        render(<BottomBar />);
        expect(screen.getByTestId('bottom-bar-tip')).toHaveTextContent(BOTTOM_BAR_TIPS[0]);
    });
});
