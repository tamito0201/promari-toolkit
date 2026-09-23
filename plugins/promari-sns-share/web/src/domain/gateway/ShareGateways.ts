/**
 * Interfaces for the outside capabilities that sharing needs. The domain states what
 * it needs; infrastructure implements each interface with browser APIs, and tests
 * supply plain objects.
 */
export interface ClipboardGateway {
  write(text: string): Promise<void>;
  /** Optional clipboard fallback, such as a URL prompt. */
  fallback?(text: string): void;
}

export interface NativeShareGateway {
  readonly available: boolean;
  share(data: { title: string; url: string }): Promise<void>;
}

export interface PopupGateway {
  /** Return true if the popup opened; otherwise allow normal link navigation. */
  open(href: string, size: { width: number; height: number }): boolean;
}

/** A share operation the host page may count. Not proof that the post was published. */
export interface ShareActivity {
  readonly service: string;
  readonly url: string;
  readonly placement: string;
}

export interface ShareActivityPublisher {
  publish(activity: ShareActivity): void;
}

export interface ShareGateways {
  readonly clipboard: ClipboardGateway;
  readonly nativeShare: NativeShareGateway;
  readonly popup: PopupGateway;
  readonly activity: ShareActivityPublisher;
}
