import { useState } from 'react';
import type { ProductTab } from './ProductChrome';

/** Shared interaction rules; hosts choose their available tabs and initial state. */
export function useProductNavigation(initial: { sidebarOpen: boolean; quickOpen: boolean; tab?: ProductTab }) {
  const [sidebarOpen, setSidebarOpen] = useState(initial.sidebarOpen);
  const [tab, setTab] = useState<ProductTab>(initial.tab ?? 'sessions');
  const [quickOpen, setQuickOpen] = useState(initial.quickOpen);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const toggleSidebar = (next: ProductTab) => {
    setSidebarOpen(open => next === tab ? !open : true);
    setTab(next);
  };
  return { sidebarOpen, setSidebarOpen, tab, setTab, quickOpen, setQuickOpen, settingsOpen, setSettingsOpen, toggleSidebar };
}
