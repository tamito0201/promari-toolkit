/**
 * Application ports implemented by infrastructure. Tests supply plain objects.
 */
export interface ClipboardPort {
  write(text: string): Promise<void>;
  /**
 * Optional clipboard fallback, such as a URL prompt.
 */
  fallback?(text: string): void;
}

export interface SharerPort {
  readonly available: boolean;
  share(data: { title: string; url: string }): Promise<void>;
}

export interface PopupPort {
  /**
 * Return true if the popup opened; otherwise allow normal link navigation.
 */
  open(href: string, size: { width: number; height: number }): boolean;
}

export interface NotifierPort {
  notify(text: string, target?: string): void;
}

export interface TrackDetail {
  readonly service: string;
  readonly url: string;
  readonly placement: string;
}

export interface TrackerPort {
  track(detail: TrackDetail): void;
}

export interface Ports {
  readonly clipboard: ClipboardPort;
  readonly sharer: SharerPort;
  readonly popup: PopupPort;
  readonly notifier: NotifierPort;
  readonly tracker: TrackerPort;
}
