import { IJsonModel } from 'flexlayout-react';

export const createInitialLayout = (): IJsonModel => ({
    global: {
        tabEnableClose: true,
        tabSetEnableMaximize: false,
        enableEdgeDock: false,
        tabEnableRename: true,
        tabEnableDrag: true,
        tabSetEnableDrag: true,
        tabSetEnableDrop: true,
        tabEnableRenderOnDemand: false,
    },
    borders: [],
    layout: {
        type: "row",
        weight: 100,
        children: [],
    },
});
