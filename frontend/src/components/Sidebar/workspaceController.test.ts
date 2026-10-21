import { describe, expect, it, vi } from 'vitest';
import { waitFor } from '@testing-library/react';
import { startWorkspaceController, type WorkspaceHost, type WorkspacePresentation, type WorkspaceSnapshot, type WorkspaceView } from '../../../../frontend-shell/src/workspace/controller';

function fixture(action: 'open' | 'select' = 'open') {
    const tab = {tabId:'new-tab',connectionId:'actual-connection',title:'SSH 节点',status:'connected',ready:false};
    const command:WorkspacePresentation={id:'request',action,tab};
    let commands=[command];
    const snapshot:WorkspaceSnapshot={tabs:[],active:{tabId:'',connectionId:''}};
    const host:WorkspaceHost={attach:vi.fn(async()=>'page'),detach:vi.fn(async()=>undefined),pull:vi.fn(async()=>commands),acknowledge:vi.fn(async()=>{commands=[];})};
    const view:WorkspaceView={snapshot:()=>snapshot,open:vi.fn(newTab=>{snapshot.tabs.push({...newTab});}),select:vi.fn(()=>false),discard:vi.fn()};
    return {host,view,tab,snapshot,cancelRequest(){commands=[];}};
}
describe('工作台呈现回执',()=>{
    it('连接成功不等于页签已呈现，等待真实选中和终端挂载后才确认',async()=>{
        const f=fixture();
        const dispose=startWorkspaceController(f.host,()=>f.view);
        try{
            await waitFor(()=>expect(f.view.open).toHaveBeenCalledOnce());
            expect(f.host.acknowledge).not.toHaveBeenCalled();
            f.snapshot.tabs[0].ready=true;
            f.snapshot.active=f.tab;
            await waitFor(()=>expect(f.host.acknowledge).toHaveBeenCalledWith('page','request',true));
            expect(f.view.open).toHaveBeenCalledOnce();
            expect(f.view.discard).not.toHaveBeenCalled();
        }finally{dispose();}
    });
    it('创建尚未呈现时被取消，移除未提交的新页签而不确认成功',async()=>{
        const f=fixture();const dispose=startWorkspaceController(f.host,()=>f.view);
        try{
            await waitFor(()=>expect(f.view.open).toHaveBeenCalledOnce());
            f.cancelRequest();
            await waitFor(()=>expect(f.view.discard).toHaveBeenCalledWith(f.tab));
            expect(f.host.acknowledge).not.toHaveBeenCalled();
        }finally{dispose();}
    });
    it('选择已有页签失败时不关闭用户已有终端',async()=>{
        const f=fixture('select');f.snapshot.tabs=[f.tab];const dispose=startWorkspaceController(f.host,()=>f.view);
        try{
            await waitFor(()=>expect(f.view.select).toHaveBeenCalled());
            f.cancelRequest();
            await new Promise(resolve=>setTimeout(resolve,250));
            expect(f.view.open).not.toHaveBeenCalled();expect(f.view.discard).not.toHaveBeenCalled();
        }finally{dispose();}
    });
    it('回执结果未知时关闭刚创建的准确连接，避免后台在被移除的页签里继续操作',async()=>{
        const f=fixture();f.host.acknowledge=vi.fn(async()=>{f.cancelRequest();throw Error('receipt lost');});
        const dispose=startWorkspaceController(f.host,()=>f.view);
        try{
            await waitFor(()=>expect(f.view.open).toHaveBeenCalled());
            f.snapshot.tabs[0].ready=true;f.snapshot.active=f.tab;
            await waitFor(()=>expect(f.view.discard).toHaveBeenCalledWith(f.tab));
        }finally{dispose();}
    });
    it('页面退出后迟到的注册只注销原编号，不继续拉取或创建页签',async()=>{
        const f=fixture();let resolve!:(client:string)=>void;
        f.host.attach=vi.fn(()=>new Promise<string>(value=>{resolve=value;}));
        const dispose=startWorkspaceController(f.host,()=>f.view);dispose();resolve('old-page');
        await waitFor(()=>expect(f.host.detach).toHaveBeenCalledWith('old-page'));
        expect(f.host.pull).not.toHaveBeenCalled();expect(f.view.open).not.toHaveBeenCalled();
    });
});
