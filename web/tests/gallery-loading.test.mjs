import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import ts from 'typescript';
import { effectScope } from 'vue';

const moduleURL = source => 'data:text/javascript;base64,' + Buffer.from(source).toString('base64');
const compile = source => ts.transpileModule(source, { compilerOptions: {module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022} }).outputText;
const apiURL = moduleURL(compile(await readFile(new URL('../src/api/index.ts', import.meta.url), 'utf8')));
let source = compile(await readFile(new URL('../src/composables/useGallery.ts', import.meta.url), 'utf8'));
source = source.replace('from "vue"', `from "${import.meta.resolve('vue')}"`).replace('from "../api"', `from "${apiURL}"`);
globalThis.localStorage = {getItem: () => null, setItem: () => {}};
globalThis.matchMedia = () => ({matches:false});
globalThis.document = {documentElement: {dataset:{}}};
globalThis.window = {scrollTo: () => {}};
const {useGallery} = await import(moduleURL(source));

test('append keeps existing media, deduplicates requests and ignores stale navigation responses', async () => {
  let calls = [], resolvePage;
  globalThis.fetch = async url => {
    calls.push(url);
    if (!url.includes('cursor=')) return Response.json({items:[{id:1}],nextCursor:'second'});
    return new Promise(resolve => {resolvePage=resolve;});
  };
  const scope=effectScope();
  const g=scope.run(useGallery);
  try {
    await g.load();
    assert.deepEqual(g.assets.value.map(a=>a.id),[1]);
    const pending=g.loadMore();
    await g.loadMore();
    assert.equal(calls.length,2);
    assert.equal(g.loading.value,false);
    assert.equal(g.loadingMore.value,true);
    assert.deepEqual(g.assets.value.map(a=>a.id),[1]);
    resolvePage(Response.json({items:[{id:1},{id:2}],nextCursor:'third'}));
    await pending;
    assert.deepEqual(g.assets.value.map(a=>a.id),[1,2]);
    const stale=g.loadMore();
    g.navigate('Maps');
    resolvePage(Response.json({items:[{id:3}],nextCursor:''}));
    await stale;
    assert.deepEqual(g.assets.value,[]);
    assert.equal(g.loadingMore.value,false);
    g.view.value='Photos';
    await g.load();
    const failure=g.loadMore();
    resolvePage(new Response('temporary failure',{status:500}));
    await failure;
    assert.deepEqual(g.assets.value.map(a=>a.id),[1]);
    assert.equal(g.nextCursor.value,'second');
    assert.equal(g.loadingMore.value,false);
    const retry=g.loadMore();
    resolvePage(Response.json({items:[{id:2}],nextCursor:''}));
    await retry;
    assert.deepEqual(g.assets.value.map(a=>a.id),[1,2]);
    assert.equal(g.nextCursor.value,'');
  } finally {scope.stop();}
});
