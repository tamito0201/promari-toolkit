/** The device share sheet. Infrastructure implements it. */
export interface NativeShareGateway {
  readonly available: boolean;
  share(data: { title: string; url: string }): Promise<void>;
}
