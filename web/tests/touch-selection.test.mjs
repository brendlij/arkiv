import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { parse, compileScript } from '@vue/compiler-sfc';
import ts from 'typescript';
import { effectScope } from 'vue';
const text=await readFile(new URL('../src/components/GalleryGrid.vue',import.meta.url),'utf8');
const {descriptor}=parse(text);
let source=ts.transpileModule(compileScript(descriptor,{id:'touch-test'}).content,{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText;
source=source.replaceAll("from 'vue'",`from "${import.meta.resolve('vue')}"`).replaceAll('from "vue"',`from "${import.meta.resolve('vue')}"`).replace(/import .* from "\.\.\/api";/,'const api=()=>{throw new Error("Unexpected API mutation")}; const thumbnail=()=>"";').replace(/import Icon from "\.\/Icon.vue";/,'const Icon={};');
const {default: component}=await import('data:text/javascript;base64,'+Buffer.from(source).toString('base64'));
let hit=0, frame, scrolled=0;
globalThis.requestAnimationFrame=callback=>{frame=callback;return 1;};
globalThis.cancelAnimationFrame=()=>{frame=undefined;};
globalThis.window={innerHeight:800,scrollBy:(_,y)=>{scrolled+=y;}};
globalThis.document={elementFromPoint:()=>({closest:()=>({dataset:{photoIndex:String(hit)}})})};
const event=(x=30,y=300,count=1)=>({touches:Array.from({length:count},()=>({clientX:x,clientY:y})),preventDefault(){this.prevented=true;}});
const waitHold=()=>new Promise(resolve=>setTimeout(resolve,430));
test('touch hold paints a reversible range, edge-scrolls and suppresses accidental opens; swipes scroll normally',async()=>{
 const scope=effectScope();const emitted=[];
 const g=scope.run(()=>component.setup({assets:Array.from({length:8},(_,id)=>({id})),loading:false,trash:false},{expose:()=>{},emit:(...v)=>emitted.push(v)}));
 g.gridRoot.value={contains:()=>true,querySelector:()=>({getBoundingClientRect:()=>({bottom:100})})};
 try {
  g.hold(1,event());g.holdMove(event(30,330));await waitHold();
  assert.equal(g.selected.value.size,0,'ordinary swipe must not select');
  g.hold(1,event());await waitHold();assert.deepEqual([...g.selected.value],[1]);
  hit=4;const move=event(150,350);g.holdMove(move);
  assert.equal(move.prevented,true);assert.deepEqual([...g.selected.value],[1,2,3,4]);
  hit=2;g.holdMove(event(80,350));assert.deepEqual([...g.selected.value],[1,2]);
  g.holdMove(event(80,770));frame();assert.ok(scrolled>0,'edge drag scrolls');
  g.endHold(event());g.open(2,{});assert.equal(emitted.length,0);assert.deepEqual([...g.selected.value],[1,2]);
  g.hold(6,event());await waitHold();hit=7;g.holdMove(event(200,350));g.endHold(event());
  assert.deepEqual([...g.selected.value],[1,2,6,7],'keeps earlier selected items');
  g.hold(0,event());g.holdMove(event(30,300,2));await waitHold();assert.deepEqual([...g.selected.value],[1,2,6,7],'second finger cancels hold');
  g.clear();g.hold(0,event());g.endHold(event());g.open(0,{});assert.deepEqual(emitted,[['open',0]],'short tap opens photo');
 } finally {g.clear();scope.stop();}
});
