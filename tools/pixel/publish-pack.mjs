import {mkdtempSync,writeFileSync,copyFileSync,mkdirSync,rmSync} from 'node:fs';
import {resolve,dirname,basename,sep} from 'node:path';
import {tmpdir} from 'node:os';
import {validateCompanionPack} from '../checks/check-companion-assets.mjs';

/** Validate every candidate byte before changing the current runtime pack. */
export function publishCompanionPack(manifest,binaries,output) {
 // Keep candidates outside the runtime tree: concurrent asset gates enumerate pack folders.
 const stagingRoot=resolve(tmpdir()),candidate=mkdtempSync(resolve(stagingRoot,'opscopilot-companion-'));
 try {
  for(const asset of Object.values(manifest.assets)) {
   if(!/^[a-z0-9-]+\.png$/.test(asset.path))throw new Error('Unsafe candidate filename');
   const bytes=binaries.get(asset.path);if(!bytes)throw new Error('Missing candidate bytes: '+asset.path);
   writeFileSync(resolve(candidate,asset.path),bytes);
  }
  const result=validateCompanionPack(manifest,candidate);
  const entries=Object.entries(manifest.assets);
  writeFileSync(resolve(candidate,'catalog.ts'),entries.map(([id,a],i)=>`import asset${i} from './${a.path}';`).join('\n')+'\nexport const images:Record<string,string>={\n'+entries.map(([id],i)=>`'${id}':asset${i},`).join('\n')+'\n};\n');
  writeFileSync(resolve(candidate,'manifest.json'),JSON.stringify(manifest,null,2)+'\n');
  mkdirSync(output,{recursive:true});
  for(const path of [...Object.values(manifest.assets).map(a=>a.path),'catalog.ts','manifest.json'])copyFileSync(resolve(candidate,path),resolve(output,path));
  return result;
 }finally{
  // Only remove the unique staging directory created by this invocation.
  if(dirname(candidate)!==stagingRoot||!candidate.startsWith(stagingRoot+sep)||!basename(candidate).startsWith('opscopilot-companion-'))throw new Error('Invalid candidate cleanup path');
  rmSync(candidate,{recursive:true,force:true});
 }
}
