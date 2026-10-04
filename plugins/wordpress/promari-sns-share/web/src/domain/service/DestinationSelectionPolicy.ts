/**
 * Domain service: choose the primary and secondary destinations to show.
 */
import type { ShareDestinationRepository } from '../repository/ShareDestinationRepository.ts';
import { ShareAction } from '../model/ShareAction.ts';
import { type Placement } from '../model/Placement.ts';
import { type DestinationSelectionSettings } from '../model/DestinationSelectionSettings.ts';

export interface DestinationSelection {
  readonly primary: readonly string[];
  readonly secondary: readonly string[];
  /** Configured keys with no destination (typos, removed destinations). They are left out, not rendered. */
  readonly unknown: readonly string[];
}

export class DestinationSelectionPolicy {
  /**
   * Select destinations for the placement and device capabilities. Native sharing requires
   * navigator.share. Floating bars honor destination overrides, exclusions, and the secondary limit.
   * Unknown keys are dropped from both rows and reported, so one typo in the configuration
   * does not stop the whole bar from rendering.
   */
  static select(
    { destinations, secondary, buttons, floating }: DestinationSelectionSettings,
    { placement, canNativeShare, repository }: { placement: Placement; canNativeShare: boolean; repository: ShareDestinationRepository },
  ): DestinationSelection {
    const isFloating = placement === 'floating';
    const configuredPrimary = isFloating && floating.destinations.length ? floating.destinations : destinations;
    const visible = secondary
      .filter((key) => repository.has(key))
      .filter((key) => canNativeShare || repository.resolve([key])[0]?.action !== ShareAction.Native)
      .filter((key) => !isFloating || (buttons[key]?.floating ?? true));
    return {
      primary: configuredPrimary.filter((key) => repository.has(key)),
      secondary: isFloating ? visible.slice(0, floating.secondaryMax) : visible,
      unknown: [...new Set([...configuredPrimary, ...secondary])].filter((key) => !repository.has(key)),
    };
  }
}
