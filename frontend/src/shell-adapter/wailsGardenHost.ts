import type { GardenChangeSignalPort, GardenHost, GardenSnapshotPort } from '@opscopilot/shell-terminal/ui';

type WailsWindow = Window & {
    go?: { main?: { App?: {
        GardenSnapshot?: () => Promise<GardenSnapshotPort>;
        GardenSignal?: () => Promise<GardenChangeSignalPort | null>;
        GardenPurchase?: (itemId: string, price: number, initialLevel: number) => Promise<{ balance: number; specimen: NonNullable<GardenSnapshotPort['specimens']>[number] }>;
        GardenPlace?: (instanceId: string, x: number, y: number, scale: number, flipX: boolean) => Promise<GardenSnapshotPort>;
        GardenStow?: (instanceId: string) => Promise<GardenSnapshotPort>;
    } } };
};

type WailsApp = NonNullable<NonNullable<NonNullable<WailsWindow['go']>['main']>['App']>;

function app(): WailsApp | undefined {
    return (window as WailsWindow).go?.main?.App;
}

/**
 * Wails 花园宿主适配器：与 Teams 插件的 gardenHost 消费同一个 GardenHost 端口与
 * 同一个 GardenPanel 组件，差异只在 RPC 通道（这里走 Wails 绑定，插件走 sidecar）。
 * 方法缺失时显式报错，不做静默降级——能力探测由 GardenPanel 的"当前宿主未提供
 * 花园能力"分支承担（宿主未挂 GardenPanel 就不会创建本适配器）。
 */
export const wailsGardenHost: GardenHost = {
    snapshot(): Promise<GardenSnapshotPort> {
        const fn = app()?.GardenSnapshot;
        if (typeof fn !== 'function') {
            return Promise.reject(new Error('当前宿主未提供 GardenSnapshot'));
        }
        return fn.call(app());
    },
    signal(): Promise<GardenChangeSignalPort | null> {
        const fn = app()?.GardenSignal;
        if (typeof fn !== 'function') {
            return Promise.reject(new Error('当前宿主未提供 GardenSignal'));
        }
        return fn.call(app());
    },
    purchase(itemId, price, initialLevel) {
        const fn = app()?.GardenPurchase;
        if (typeof fn !== 'function') return Promise.reject(new Error('当前宿主未提供 GardenPurchase'));
        return fn.call(app(), itemId, price, initialLevel);
    },
    place(instanceId, x, y, scale, flipX) {
        const fn = app()?.GardenPlace;
        if (typeof fn !== 'function') return Promise.reject(new Error('当前宿主未提供 GardenPlace'));
        return fn.call(app(), instanceId, x, y, scale, flipX);
    },
    stow(instanceId) {
        const fn = app()?.GardenStow;
        if (typeof fn !== 'function') return Promise.reject(new Error('当前宿主未提供 GardenStow'));
        return fn.call(app(), instanceId);
    },
};
