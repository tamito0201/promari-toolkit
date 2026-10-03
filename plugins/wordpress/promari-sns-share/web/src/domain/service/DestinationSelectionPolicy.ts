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
}

export class DestinationSelectionPolicy {
  /**
   * Select destinations for the placement and device capabilities. Native sharing requires
   * navigator.share. Floating bars honor destination overrides, exclusions, and the secondary limit.
   */
  static select(
    { destinations, secondary, buttons, floating }: DestinationSelectionSettings,
    { placement, canNativeShare, repository }: { placement: Placement; canNativeShare: boolean; repository: ShareDestinationRepository },
  ): DestinationSelection {
    const isFloating = placement === 'floating';
    const visible = secondary
      .filter((key) => repository.has(key))
      .filter((key) => canNativeShare || repository.resolve([key])[0]?.action !== ShareAction.Native)
      .filter((key) => !isFloating || (buttons[key]?.floating ?? true));
    return {
      primary: isFloating && floating.destinations.length ? floating.destinations : destinations,
      secondary: isFloating ? visible.slice(0, floating.secondaryMax) : visible,
    };
  }
}
