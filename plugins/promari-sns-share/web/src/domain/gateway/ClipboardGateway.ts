/** The ability to put text on the clipboard. Infrastructure implements it. */
export interface ClipboardGateway {
  write(text: string): Promise<void>;
  /** Optional clipboard fallback, such as a URL prompt. */
  fallback?(text: string): void;
}
