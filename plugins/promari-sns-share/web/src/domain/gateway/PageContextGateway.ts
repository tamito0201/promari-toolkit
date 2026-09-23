import type { PageInfo } from '../model/ShareTypes.ts';

/** Reads the page being shared. Infrastructure implements it. */
export interface PageContextGateway {
  read(): PageInfo;
}
