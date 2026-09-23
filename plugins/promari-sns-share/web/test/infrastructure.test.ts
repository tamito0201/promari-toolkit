/**
 * Infrastructure implementations of domain interfaces that run without a browser.
 */
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { UnknownServiceError } from '../src/domain/repository/ShareServiceRepository.ts';
import { ShareService } from '../src/domain/model/ShareService.ts';
import { SpecShareServiceRepository } from '../src/infrastructure/SpecShareServiceRepository.ts';
import type { ServiceSpec } from '../src/domain/model/ShareTypes.ts';
import { SPECS } from './domain.test.ts';

describe('SpecShareServiceRepository', () => {
  it('ドメインのリポジトリとして、指定した順に共有先を返す', () => {
    const repository = new SpecShareServiceRepository(SPECS);
    assert.deepEqual(repository.keys(), SPECS.map((s: ServiceSpec) => s.key));
    assert.equal(repository.has('missing'), false);
    assert.deepEqual(repository.resolve(['copy', 'x']).map((s) => s.key), ['copy', 'x']);
  });
  it('表示用メタデータを持ち込まず、不変の値オブジェクトを返す', () => {
    const service = new SpecShareServiceRepository(SPECS).resolve(['x'])[0]!;
    assert.ok(service instanceof ShareService);
    assert.equal('appearance' in service, false);
    assert.ok(Object.isFrozen(service));
  });
  it('未知の名前は例外（黙って読み飛ばさない）', () => {
    assert.throws(() => new SpecShareServiceRepository(SPECS).resolve(['nope']), UnknownServiceError);
  });
});
