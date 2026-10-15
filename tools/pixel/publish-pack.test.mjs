import {test} from 'node:test';
import assert from 'node:assert/strict';
import {mkdtempSync,mkdirSync,writeFileSync,readFileSync,readdirSync,rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {resolve,basename,dirname} from 'node:path';
import {publishCompanionPack} from './publish-pack.mjs';
test('an invalid candidate preserves current files and keeps staging outside runtime packs',()=>{
 const root=mkdtempSync(resolve(tmpdir(),'companion-publish-')),output=resolve(root,'lumen-v8');
 try{
  const pack=JSON.parse(readFileSync('frontend-shell/src/companion/packs/lumen-v8/manifest.json','utf8'));
  mkdirSync(output);for(const file of ['manifest.json','catalog.ts','lumen-idle-0.png'])writeFileSync(resolve(output,file),'approved:'+file);
  const bytes=new Map(Object.values(pack.assets).map(a=>[a.path,Buffer.from('invalid candidate PNG')]));
  assert.throws(()=>publishCompanionPack(pack,bytes,output),/PNG/);
  for(const file of ['manifest.json','catalog.ts','lumen-idle-0.png'])assert.equal(readFileSync(resolve(output,file),'utf8'),'approved:'+file);
  assert.deepEqual(readdirSync(root),['lumen-v8']);
 }finally{
  if(dirname(root)!==resolve(tmpdir())||!basename(root).startsWith('companion-publish-'))throw new Error('Invalid test cleanup path');
  rmSync(root,{recursive:true,force:true});
 }
});
