/** A share operation the host page may count. Not proof that the post was published. */
export interface ShareActivity {
  readonly destination: string;
  readonly url: string;
  readonly placement: string;
}

/** Tells the host page that a share operation happened. Infrastructure implements it. */
export interface ShareActivityPublisher {
  publish(activity: ShareActivity): void;
}
