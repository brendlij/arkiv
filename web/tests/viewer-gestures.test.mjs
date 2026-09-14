import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { parse, compileScript } from '@vue/compiler-sfc';
import ts from 'typescript';
import { effectScope } from 'vue';
const {descriptor}=parse(await readFile(new URL('../src/components/AssetViewer.vue',import.meta.url),'utf8'));
let source=ts.transpileModule(compileScript(descriptor,{id:'viewer-test'}).content,{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText;
source=source.replace(/from ['"]vue['"]/g,`from "${import.meta.resolve('vue')}"`).replace(/import .* from "\.\.\/api";/,'const api=()=>{};const thumbnail=()=>"";').replace(/import (\w+) from "\.\/[^\"]+\.vue";/g,'const $1={};');
globalThis.matchMedia=()=>({matches:true});
const {default:component}=await import('data:text/javascript;base64,'+Buffer.from(source).toString('base64'));
const target={closest:()=>null,tagName:'IMG',getBoundingClientRect:()=>({bottom:700})};
const touch=(x,y,tag=target)=>({touches:[{clientX:x,clientY:y}],changedTouches:[{clientX:x,clientY:y}],target:tag,preventDefault(){this.prevented=true;}});
function swipe(g,x,y,toX,toY,tag=target){g.touchBegin(touch(x,y,tag));g.touchMove(touch(toX,toY,tag));const end=touch(toX,toY,tag);end.touches=[];g.touchEnd(end);}
test('fullscreen swipe navigation, downward dismissal and zoom protection',()=>{
 const scope=effectScope(), emitted=[];
 const g=scope.run(()=>component.setup({assets:[{id:1,mediaType:'image'},{id:2,mediaType:'video'},{id:3,mediaType:'image'}],index:1},{expose:()=>{},emit:(...e)=>emitted.push(e)}));
 try {
  swipe(g,250,300,80,305);assert.deepEqual(emitted.pop(),['navigate',2]);
  swipe(g,80,300,250,305,{...target,tagName:'VIDEO'});assert.deepEqual(emitted.pop(),['navigate',0]);
  swipe(g,160,250,165,410);assert.deepEqual(emitted.pop(),['close']);
  g.scale.value=2;swipe(g,250,300,80,300);assert.equal(emitted.length,0,'pan must not navigate');
  g.scale.value=1;swipe(g,160,300,165,250);assert.equal(emitted.length,0,'upward swipe does not dismiss');
  g.touchBegin(touch(200,300));g.touchMove(touch(100,300));assert.ok(g.swipeX.value<0);g.cancelTouch();assert.equal(g.swipeX.value,0);
 } finally {clearTimeout(g.idleTimer);clearTimeout(g.tapTimer);scope.stop();}
});
