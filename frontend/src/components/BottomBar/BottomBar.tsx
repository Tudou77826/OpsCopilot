import Shared from '../../../../frontend-shell/src/ui/product/BottomBar';
export { BOTTOM_BAR_TIPS, BOTTOM_BAR_TIP_INTERVAL_MS } from '../../../../frontend-shell/src/ui/product/BottomBar';
const loadVersion = async () => {
  // @ts-ignore Wails host binding
  return await window.go?.main?.App?.GetVersion?.() ?? '';
};
export default function BottomBar() { return <Shared loadVersion={loadVersion} />; }
