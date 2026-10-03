/** 共有先の選択に必要な値だけを表す。描画設定は含めない。 */
export interface DestinationSelectionSettings {
  readonly destinations: readonly string[];
  readonly secondary: readonly string[];
  readonly buttons: Readonly<Record<string, { readonly floating?: boolean }>>;
  readonly floating: { readonly destinations: readonly string[]; readonly secondaryMax: number };
}
