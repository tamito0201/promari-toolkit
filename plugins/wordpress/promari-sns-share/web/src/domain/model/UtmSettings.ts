/** UTM parameters added to shared URLs. */
export interface UtmSettings {
  readonly enabled: boolean;
  readonly source: string;
  readonly medium: string;
  readonly campaign: string;
  readonly content: string;
}
