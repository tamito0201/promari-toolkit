/**
 * Infrastructure implementations of domain interfaces that run without a browser.
 */
import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { UnknownShareDestinationError } from '../src/domain/repository/ShareDestinationRepository.ts';
import { ShareDestination } from '../src/domain/model/ShareDestination.ts';
import { InMemoryShareDestinationRepository } from '../src/infrastructure/InMemoryShareDestinationRepository.ts';
import type { ShareDestinationSpec } from '../src/domain/model/ShareDestinationSpec.ts';
import { SPECS } from './domain.test.ts';

describe('InMemoryShareDestinationRepository', () => {
  it('ドメインのリポジトリとして、指定した順に共有先を返す', () => {
    const repository = new InMemoryShareDestinationRepository(SPECS);
    assert.deepEqual(repository.keys(), SPECS.map((s: ShareDestinationSpec) => s.key));
    assert.equal(repository.has('missing'), false);
    assert.deepEqual(repository.resolve(['copy', 'x']).map((s) => s.key), ['copy', 'x']);
  });
  it('表示用メタデータを持ち込まず、不変の値オブジェクトを返す', () => {
    const destination = new InMemoryShareDestinationRepository(SPECS).resolve(['x'])[0]!;
    assert.ok(destination instanceof ShareDestination);
    assert.equal('appearance' in destination, false);
    assert.ok(Object.isFrozen(destination));
  });
  it('未知の名前は例外（黙って読み飛ばさない）', () => {
    assert.throws(() => new InMemoryShareDestinationRepository(SPECS).resolve(['nope']), UnknownShareDestinationError);
  });
});
