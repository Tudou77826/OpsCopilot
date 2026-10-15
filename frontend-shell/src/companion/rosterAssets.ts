import type { CompanionContentPack } from './content';
import lumenManifest from './packs/lumen-v8/manifest.json';
import { images as lumenImages } from './packs/lumen-v8/catalog';
import emberManifest from './packs/ember-v7/manifest.json';
import { images as emberImages } from './packs/ember-v7/catalog';
import mossManifest from './packs/moss-v7/manifest.json';
import { images as mossImages } from './packs/moss-v7/catalog';
import boltManifest from './packs/bolt-v7/manifest.json';
import { images as boltImages } from './packs/bolt-v7/catalog';
import astraManifest from './packs/astra-v7/manifest.json';
import { images as astraImages } from './packs/astra-v7/catalog';
import tideManifest from './packs/tide-v7/manifest.json';
import { images as tideImages } from './packs/tide-v7/catalog';
import sageManifest from './packs/sage-v7/manifest.json';
import { images as sageImages } from './packs/sage-v7/catalog';
export const rosterPacks: Record<string, {pack: CompanionContentPack; images: Record<string,string>}> = {
  lumen: {pack:lumenManifest as unknown as CompanionContentPack,images:lumenImages},
  ember: {pack:emberManifest as unknown as CompanionContentPack,images:emberImages},
  moss: {pack:mossManifest as unknown as CompanionContentPack,images:mossImages},
  bolt: {pack:boltManifest as unknown as CompanionContentPack,images:boltImages},
  astra: {pack:astraManifest as unknown as CompanionContentPack,images:astraImages},
  tide: {pack:tideManifest as unknown as CompanionContentPack,images:tideImages},
  sage: {pack:sageManifest as unknown as CompanionContentPack,images:sageImages},
};
