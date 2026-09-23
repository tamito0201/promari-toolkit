/**
 * Infrastructure implementations of domain interfaces that run without a browser.
 */
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { UnknownServiceError } from '../src/domain/repository/ShareServiceRepository.ts';
import { specShareServiceRepository } from '../src/infrastructure/SpecShareServiceRepository.ts';
import { SPECS } from './domain.test.ts';

describe('specShareServiceRepository', () => {
  it('ドメインのリポジトリとして、指定した順に共有先を返す', () => {
    const repository = specShareServiceRepository(SPECS);
    assert.deepEqual(repository.keys(), SPECS.map((s) => s.key));
    assert.equal(repository.has('missing'), false);
    assert.deepEqual(repository.resolve(['copy', 'x']).map((s) => s.key), ['copy', 'x']);
  });
  it('表示用メタデータを持ち込まず、不変の値オブジェクトを返す', () => {
    const service = specShareServiceRepository(SPECS).resolve(['x'])[0]!;
    assert.deepEqual(Object.keys(service).sort(), ['action', 'key', 'shareUrl']);
    assert.ok(Object.isFrozen(service));
  });
  it('未知の名前は例外（黙って読み飛ばさない）', () => {
    assert.throws(() => specShareServiceRepository(SPECS).resolve(['nope']), UnknownServiceError);
  });
});
