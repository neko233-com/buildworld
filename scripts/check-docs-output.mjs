import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFile } from 'node:fs/promises';

const root = new URL('../', import.meta.url);
const asset = 'img/screenshots/build-details-2026-07-22.png';
const source = await readFile(new URL(`docs-site/static/${asset}`, root));
assert.equal(source.subarray(0, 8).toString('hex'), '89504e470d0a1a0a', 'Screenshot must be a PNG');
assert.equal(source.readUInt32BE(16), 1440, 'Screenshot width changed');
assert.equal(source.readUInt32BE(20), 1000, 'Screenshot height changed');
const digest = (bytes) => createHash('sha256').update(bytes).digest('hex');
const built = await readFile(new URL(`docs-site/build/${asset}`, root));
assert.equal(digest(built), digest(source), 'Published screenshot differs from the source PNG');

for (const file of ['README.md', 'README_CN.md']) {
  const text = await readFile(new URL(file, root), 'utf8');
  assert.ok(text.includes(`](docs-site/static/${asset})`), `${file}: screenshot is not embedded`);
  assert.ok(text.includes('2026-07-22'), `${file}: capture date is missing`);
}
for (const file of ['index.html', 'screenshots/index.html', 'zh-Hans/index.html', 'zh-Hans/screenshots/index.html']) {
  const html = await readFile(new URL(`docs-site/build/${file}`, root), 'utf8');
  const imageTag = html.match(/<img\b[^>]*>/g)?.find((tag) => tag.includes('build-details-2026-07-22'));
  assert.ok(imageTag, `${file}: screenshot is missing`);
  const imageUrl = imageTag.match(/\bsrc="([^"]+)"/)?.[1];
  assert.ok(imageUrl?.startsWith('/buildworld/'), `${file}: screenshot base URL is incorrect`);
  // Docusaurus emits a content-hashed image URL. Verify its bytes, not just its filename.
  const emittedImage = await readFile(new URL(`docs-site/build/${imageUrl.slice('/buildworld/'.length)}`, root));
  assert.equal(digest(emittedImage), digest(source), `${file}: embedded screenshot differs from the original`);
  const linkTag = html.match(/<a\b[^>]*>/g)?.find((tag) => tag.includes('build-details-2026-07-22'));
  const fullSizeUrl = linkTag?.match(/\bhref="([^"]+)"/)?.[1];
  assert.ok(fullSizeUrl?.startsWith('/buildworld/'), `${file}: full-size image base URL is incorrect`);
  const fullSizeImage = await readFile(new URL(`docs-site/build/${fullSizeUrl.slice('/buildworld/'.length)}`, root));
  assert.equal(digest(fullSizeImage), digest(source), `${file}: full-size image differs from the original`);
  assert.ok(html.includes('2026-07-22'), `${file}: capture date is missing`);
}
console.log('Documentation assets verified: both READMEs, 4 localized HTML pages, original PNG SHA-256', digest(source));
