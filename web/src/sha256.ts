// Incremental SHA-256 keeps hashing bounded to one upload chunk in memory and
// works on self-hosted HTTP origins where Web Crypto may be unavailable.
const K = new Uint32Array([
  0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1,
  0x923f82a4, 0xab1c5ed5, 0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3,
  0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174, 0xe49b69c1, 0xefbe4786,
  0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
  0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147,
  0x06ca6351, 0x14292967, 0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13,
  0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85, 0xa2bfe8a1, 0xa81a664b,
  0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
  0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a,
  0x5b9cca4f, 0x682e6ff3, 0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208,
  0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
]);
const rotate = (x: number, n: number) => (x >>> n) | (x << (32 - n));
export class Sha256 {
  private state = new Uint32Array([
    0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c,
    0x1f83d9ab, 0x5be0cd19,
  ]);
  private buffer = new Uint8Array(64);
  private words = new Uint32Array(64);
  private buffered = 0;
  private bytes = 0;
  private finished = false;
  update(data: Uint8Array) {
    if (this.finished) throw new Error("Hash already finalized");
    this.bytes += data.length;
    let offset = 0;
    if (this.buffered) {
      const take = Math.min(64 - this.buffered, data.length);
      this.buffer.set(data.subarray(0, take), this.buffered);
      this.buffered += take;
      offset = take;
      if (this.buffered === 64) {
        this.block(this.buffer, 0);
        this.buffered = 0;
      }
    }
    for (; offset + 64 <= data.length; offset += 64) this.block(data, offset);
    if (offset < data.length) {
      this.buffer.set(data.subarray(offset));
      this.buffered = data.length - offset;
    }
    return this;
  }
  private block(data: Uint8Array, offset: number) {
    const w = this.words;
    for (let i = 0; i < 16; i++) {
      const j = offset + i * 4;
      w[i] =
        ((data[j]! << 24) |
          (data[j + 1]! << 16) |
          (data[j + 2]! << 8) |
          data[j + 3]!) >>>
        0;
    }
    for (let i = 16; i < 64; i++) {
      const x = w[i - 15]!,
        y = w[i - 2]!;
      w[i] =
        (w[i - 16]! +
          (rotate(x, 7) ^ rotate(x, 18) ^ (x >>> 3)) +
          w[i - 7]! +
          (rotate(y, 17) ^ rotate(y, 19) ^ (y >>> 10))) >>>
        0;
    }
    let a = this.state[0]!,
      b = this.state[1]!,
      c = this.state[2]!,
      d = this.state[3]!,
      e = this.state[4]!,
      f = this.state[5]!,
      g = this.state[6]!,
      h = this.state[7]!;
    for (let i = 0; i < 64; i++) {
      const t1 =
        (h +
          (rotate(e, 6) ^ rotate(e, 11) ^ rotate(e, 25)) +
          ((e & f) ^ (~e & g)) +
          K[i]! +
          w[i]!) >>>
        0;
      const t2 =
        ((rotate(a, 2) ^ rotate(a, 13) ^ rotate(a, 22)) +
          ((a & b) ^ (a & c) ^ (b & c))) >>>
        0;
      h = g;
      g = f;
      f = e;
      e = (d + t1) >>> 0;
      d = c;
      c = b;
      b = a;
      a = (t1 + t2) >>> 0;
    }
    this.state[0] = (this.state[0]! + a) >>> 0;
    this.state[1] = (this.state[1]! + b) >>> 0;
    this.state[2] = (this.state[2]! + c) >>> 0;
    this.state[3] = (this.state[3]! + d) >>> 0;
    this.state[4] = (this.state[4]! + e) >>> 0;
    this.state[5] = (this.state[5]! + f) >>> 0;
    this.state[6] = (this.state[6]! + g) >>> 0;
    this.state[7] = (this.state[7]! + h) >>> 0;
  }
  hex() {
    if (this.finished) throw new Error("Hash already finalized");
    const bytes = this.bytes;
    const tail = new Uint8Array(
      this.buffered < 56 ? 64 - this.buffered : 128 - this.buffered,
    );
    tail[0] = 0x80;
    const view = new DataView(tail.buffer);
    view.setUint32(tail.length - 8, Math.floor(bytes / 0x20000000));
    view.setUint32(tail.length - 4, (bytes * 8) >>> 0);
    this.update(tail);
    this.finished = true;
    return Array.from(this.state, (v) => v.toString(16).padStart(8, "0")).join(
      "",
    );
  }
}
export async function hashFile(
  file: Blob,
  progress: (value: number) => void,
  stopped: () => boolean,
) {
  const hash = new Sha256();
  const size = 4 * 1024 * 1024;
  for (let offset = 0; offset < file.size; offset += size) {
    if (stopped()) throw new Error("Paused");
    hash.update(
      new Uint8Array(await file.slice(offset, offset + size).arrayBuffer()),
    );
    progress(
      Math.round((Math.min(offset + size, file.size) / file.size) * 100),
    );
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
  if (stopped()) throw new Error("Paused");
  return hash.hex();
}
