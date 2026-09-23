/**
 * Domain service: choose the primary and secondary services to show.
 */
import type { ShareServiceRepository } from '../repository/ShareServiceRepository.ts';
import { Action, type Placement, type SelectionConfig } from '../model/ShareTypes.ts';

export interface Selection {
  readonly primary: readonly string[];
  readonly secondary: readonly string[];
}

export class ServiceSelectionPolicy {
  /**
   * Select services for the placement and device capabilities. Native sharing requires
   * navigator.share. Floating bars honor service overrides, exclusions, and the secondary limit.
   */
  static select(
    { services, secondary, buttons, floating }: SelectionConfig,
    { placement, canNativeShare, repository }: { placement: Placement; canNativeShare: boolean; repository: ShareServiceRepository },
  ): Selection {
    const isFloating = placement === 'floating';
    const visible = secondary
      .filter((key) => repository.has(key))
      .filter((key) => canNativeShare || repository.resolve([key])[0]?.action !== Action.Native)
      .filter((key) => !isFloating || (buttons[key]?.floating ?? true));
    return {
      primary: isFloating && floating.services.length ? floating.services : services,
      secondary: isFloating ? visible.slice(0, floating.secondaryMax) : visible,
    };
  }
}
