/**
 * レイヤード＋DDDの依存方向を、フォルダー名だけでなく型import・再exportまで含めて検査する。
 * presentation → application → domain ← infrastructure。domain が持つリポジトリと
 * ゲートウェイのインターフェースを infrastructure が実装する。
 * composition は四層の外に置く組み立て役で、DIコンテナ（InversifyJS）を使ってよい唯一の場所。
 */
import assert from 'node:assert/strict';
import { readFileSync, readdirSync, mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { spawnSync } from 'node:child_process';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, it } from 'node:test';
import { parse } from '@babel/parser';

const root = fileURLToPath(new URL('../src/', import.meta.url));
const allowed: Readonly<Record<string, readonly string[]>> = {
  domain: ['domain'],
  application: ['application', 'domain'],
  infrastructure: ['infrastructure', 'domain'],
  presentation: ['presentation', 'application'],
  composition: ['composition', 'domain', 'application', 'infrastructure', 'presentation'],
};
// 外部パッケージを読み込んでよい層。四層はDIコンテナを知らず、コンストラクタで依存を受け取る。
const packages: Readonly<Record<string, readonly string[]>> = { composition: ['inversify'] };
const LAYERS = ['domain', 'application', 'infrastructure', 'presentation'];
const files = (directory: string): string[] => readdirSync(directory, { withFileTypes: true })
  .flatMap(entry => entry.isDirectory() ? files(join(directory, entry.name)) : entry.name.endsWith('.ts') ? [join(directory, entry.name)] : []);

function violations(file: string, text: string): string[] {
  const layer = relative(root, file).split('/')[0]!;
  const errors: string[] = [];
  const source = parse(text, { sourceType: 'module', plugins: ['typescript'], createImportExpressions: true });
  const check = (name: string): void => {
    if (!name.startsWith('.')) {
      if (!packages[layer]?.includes(name)) errors.push(`${layer} → ${name}`);
      return;
    }
    const target = relative(root, resolve(dirname(file), name)).split('/')[0]!;
    if (!allowed[layer]?.includes(target)) errors.push(`${layer} → ${name}`);
  };
  const record = (value: unknown): Record<string, unknown> =>
    value && typeof value === 'object' ? value as Record<string, unknown> : {};
  const visit = (value: unknown): void => {
    if (Array.isArray(value)) { value.forEach(visit); return; }
    const node = record(value);
    if (['ImportDeclaration', 'ExportNamedDeclaration', 'ExportAllDeclaration'].includes(String(node['type']))) {
      const name = record(node['source'])['value'];
      if (typeof name === 'string') check(name);
    }
    if (node['type'] === 'TSImportType') {
      const name = record(node['argument'])['value'];
      if (typeof name === 'string') check(name);
    }
    if (node['type'] === 'TSImportEqualsDeclaration' || node['type'] === 'ImportExpression' ||
        (node['type'] === 'CallExpression' && record(node['callee'])['name'] === 'require')) errors.push('静的な層境界を迂回する読み込み');
    if (layer === 'presentation' && node['type'] === 'Identifier' &&
        ['navigator', 'fetch', 'localStorage', 'sessionStorage', 'XMLHttpRequest'].includes(String(node['name']))) errors.push('表示層に外部接続API: '+node['name']);
    Object.values(node).filter(x => x && typeof x === 'object').forEach(visit);
  };
  if (source.comments?.some(c => /<reference\s/.test(c.value))) errors.push('ソースからのコンパイル環境の上書き');
  visit(source);
  return errors;
}

describe('レイヤード＋DDDの依存境界', () => {
  it('型import・再exportを含めて許可した方向だけに依存する', () => {
    for (const layer of Object.keys(allowed)) for (const file of files(join(root, layer)))
      assert.deepEqual(violations(file, readFileSync(file, 'utf8')), [], relative(root, file));
  });
  it('4層と組み立て役のファイル名はクラス・型の名前と同じ大文字始まりにそろえる', () => {
    const misnamed = Object.keys(allowed).flatMap(layer => files(join(root, layer)))
      .map(file => relative(root, file))
      .filter(file => !/^[A-Z][A-Za-z0-9]*\.ts$/.test(file.split('/').at(-1)!));
    assert.deepEqual(misnamed, []);
  });
  it('infrastructureの実装は技術名で呼び、domainの約束（〜Gateway）と名前を分ける', () => {
    const gatewayNamed = files(join(root, 'infrastructure')).map(file => relative(root, file))
      .filter(file => /Gateway\.ts$/.test(file));
    assert.deepEqual(gatewayNamed, []);
  });
  it('起動時の入口はコンテナを直接使わず、組み立て役を通す', () => {
    const entry = parse(readFileSync(join(root, 'index.ts'), 'utf8'), { sourceType: 'module', plugins: ['typescript'] });
    const sources = entry.program.body.flatMap(node => node.type === 'ImportDeclaration' ? [node.source.value] : []);
    assert.ok(sources.includes('./composition/ShareContainer.ts'));
    assert.deepEqual(sources.filter(source => !source.startsWith('.')), []);
    assert.deepEqual(sources.filter(source => source.startsWith('./infrastructure/')), [], '具体的な実装の選択は composition へ');
  });
  it('禁止した境界を越える型・再export・動的読み込みを検出する', () => {
    for (const [layer, text] of [
      ['domain', "import type { X } from '../application/HandleShareClickUseCase.ts';"],
      ['infrastructure', "import type { ShareSettings } from '../application/ShareSettings.ts';"],
      ['presentation', "import type { ShareDestination } from '../domain/model/ShareDestination.ts';"],
      ['application', "export { x } from '../infrastructure/BrowserClipboard.ts';"],
      ['presentation', "type X = import('../infrastructure/BrowserClipboard.ts').X;"],
      ['infrastructure', "import { x } from '../presentation/ShareBarView.ts';"],
      ['application', "import('../infrastructure/BrowserClipboard.ts');"],
      ['application', "import fs from 'node:fs';"],
      ['presentation', 'navigator.clipboard.writeText("x");'],
      ['domain', '/// <reference lib="dom" />'],
      // DIコンテナは組み立て役だけが使う。四層が読み込めば、コンテナへの依存が内側へ漏れる。
      ...LAYERS.map(layer => [layer, "import { Container } from 'inversify';"]),
      ['composition', "import { container } from 'tsyringe';"],
      ['domain', "import type { ShareContainer } from '../composition/ShareContainer.ts';"],
      ['presentation', "import { TOKENS } from '../composition/InjectionTokens.ts';"],
    ]) assert.ok(violations(join(root, layer!, 'fixture.ts'), text!).length, text);
  });
  it('内側の型検査はDOMもNodeの型もなく成功し、Elementを混ぜると失敗する', () => {
    const project = fileURLToPath(new URL('../../', import.meta.url));
    const compiler = join(project, 'node_modules/.bin/tsc');
    const configPath = join(project, 'tsconfig.core.json');
    const config = JSON.parse(readFileSync(configPath, 'utf8'));
    assert.deepEqual(config.compilerOptions.lib, ['ES2022']);
    assert.deepEqual(config.compilerOptions.types, []);
    const valid = spawnSync(compiler, ['-p', configPath], { encoding: 'utf8' });
    assert.equal(valid.status, 0, valid.stdout + valid.stderr);
    const directory = mkdtempSync(join(tmpdir(), 'share-core-probe-'));
    try {
      writeFileSync(join(directory, 'probe.ts'), 'export interface Leak { target: Element; browser: Window; }');
      writeFileSync(join(directory, 'tsconfig.json'), JSON.stringify({ extends: configPath, include: ['./probe.ts'] }));
      const invalid = spawnSync(compiler, ['-p', join(directory, 'tsconfig.json')], { encoding: 'utf8' });
      assert.notEqual(invalid.status, 0);
      assert.match(invalid.stdout, /Cannot find name 'Element'/);
      assert.match(invalid.stdout, /Cannot find name 'Window'/);
    } finally { rmSync(directory, { recursive: true, force: true }); }

  });
});
