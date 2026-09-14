import React, { createContext, useContext } from 'react';

/** Host-owned DOM scope. Defaults preserve the standalone Wails application. */
export const ShellSurface = createContext<{ portalRoot: HTMLElement; styleRoot: HTMLElement | ShadowRoot } | null>(null);
export function usePortalRoot() { return useContext(ShellSurface)?.portalRoot ?? document.body; }
export function useStyleRoot() { return useContext(ShellSurface)?.styleRoot ?? document.head; }
