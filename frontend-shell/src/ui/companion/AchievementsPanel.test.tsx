import React from 'react';
import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import AchievementsPanel from './AchievementsPanel';
import type { AchievementStatus, SocialHost } from '../../companion/socialHost';
import type { MedalDefinition, MedalKind } from '../../companion/types';

afterEach(cleanup);
const kinds: MedalKind[]=['knowledge-archive','script-created','quick-command-created','case-completed','knowledge-read','script-used','quick-command-used','active-day'];
const names=['知识沉淀者','脚本工匠','指令工匠','排查记录者','知识探索者','自动化实践者','快捷指令实践者','持续实践者'];
const catalog: MedalDefinition[]=kinds.map((kind,i)=>({kind,name:names[i],unit:'项',group:'practice',thresholds:[1,10,50]}));
function setup(bound=false){
  let current: AchievementStatus={summary:{ruleVersion:1,progress:kinds.map(kind=>({kind,count:1,medal:{kind,tier:'bronze',count:1},nextThreshold:10}))},pending:0,repairs:0,selected:[],profilePending:false,
    ...(bound?{binding:{baseUrl:'https://internal',member:{userId:'alice',organizationId:'org',displayName:'Alice',participating:true}}}:{})};
  const host:SocialHost={status:vi.fn(async()=>current),catalog:vi.fn(async()=>catalog),connect:vi.fn(async()=>current),sync:vi.fn(async()=>current),participation:vi.fn(async()=>{}),showcase:vi.fn(async()=>current),
    rankings:vi.fn(async query=>({...query,ruleVersion:1,snapshotId:'snapshot',total:0,entries:[]})),profile:vi.fn(async()=>({userId:'alice',displayName:'Alice',medals:[],achievements:current.summary}))};
  render(<AchievementsPanel host={host}/>);return {host,update:(next:Partial<AchievementStatus>)=>{current={...current,...next};}};
}
it('loads local medals without connecting, limits showcase to three, and sends only kinds',async()=>{
  const {host}=setup();await screen.findByRole('button',{name:/知识沉淀者/});
  for(const name of names.slice(0,3))fireEvent.click(screen.getByRole('button',{name:new RegExp(name)}));
  expect(screen.getByRole('button',{name:/排查记录者/})).toBeDisabled();
  fireEvent.click(screen.getByRole('button',{name:'保存展示选择'}));
  await waitFor(()=>expect(host.showcase).toHaveBeenCalledWith(kinds.slice(0,3)));
  expect(host.connect).not.toHaveBeenCalled();expect(host.participation).not.toHaveBeenCalled();expect(host.rankings).not.toHaveBeenCalled();
});
it('connects with explicit local attribution and clears credentials',async()=>{
  const {host}=setup();await screen.findByRole('button',{name:/知识沉淀者/});
  fireEvent.change(screen.getByLabelText('排名服务地址'),{target:{value:'https://internal'}});
  fireEvent.change(screen.getByLabelText('成员凭据'),{target:{value:'secret'}});
  fireEvent.click(screen.getByLabelText('将这份本地使用记录归属到我的成员账号'));
  fireEvent.click(screen.getByRole('button',{name:'连接成员账号'}));
  await waitFor(()=>expect(host.connect).toHaveBeenCalledWith('https://internal','secret',true));
  await waitFor(()=>expect(screen.getByLabelText('成员凭据')).toHaveValue(''));
  expect(host.participation).not.toHaveBeenCalled();
});
it('reports offline withdrawal as pending and keeps a manual synchronization path',async()=>{
  const {host,update}=setup(true);await screen.findByText('Alice',{exact:true});
  (host.participation as ReturnType<typeof vi.fn>).mockImplementation(async()=>{update({participationPending:false,binding:{baseUrl:'https://internal',member:{userId:'alice',organizationId:'org',displayName:'Alice',participating:false}}});throw new Error('离线');});
  fireEvent.click(screen.getByRole('button',{name:'退出排名'}));
  await screen.findByText(/公开资料可能仍可见/);
  expect(screen.getByRole('alert')).toHaveTextContent('离线');
  expect(screen.getByRole('button',{name:'立即同步'})).toBeEnabled();
});
it('opens a member profile through the service and provides no interaction actions',async()=>{
  const {host}=setup(true);await screen.findByText('Alice',{exact:true});
  fireEvent.click(screen.getByRole('button',{name:'查看我的资料'}));
  await waitFor(()=>expect(host.profile).toHaveBeenCalledWith('alice'));
  expect(await screen.findByRole('region',{name:'成员资料'})).toBeTruthy();
  expect(screen.queryByRole('button',{name:/点赞|私信|鼓励|加好友/})).toBeNull();
});
