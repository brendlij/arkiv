import test from 'node:test';
import assert from 'node:assert/strict';
import { createHash, randomBytes } from 'node:crypto';
import { Sha256 } from '../src/sha256.ts';

test('streaming SHA-256 matches independent Node digests at padding and chunk boundaries',()=>{
 for(const size of [0,1,55,56,63,64,65,127,128,1024,1048593]){
  const data=randomBytes(size), expected=createHash('sha256').update(data).digest('hex');
  for(const stride of [1,63,64,4093,8*1024*1024]){
   const hash=new Sha256();for(let i=0;i<data.length;i+=stride)hash.update(data.subarray(i,i+stride));
   assert.equal(hash.hex(),expected,`size ${size}, stride ${stride}`);
  }
 }
});

test('streaming SHA-256 carries the bit length correctly beyond 512 MiB',()=>{
 const block=Buffer.alloc(1024*1024,0x5a),hash=new Sha256(),reference=createHash('sha256');
 for(let i=0;i<513;i++){hash.update(block);reference.update(block);}
 assert.equal(hash.hex(),reference.digest('hex'));
});
