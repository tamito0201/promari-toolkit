import type { SharedPage } from '../model/SharedPage.ts';

/** Reads the page being shared. Infrastructure implements it. */
export interface SharedPageGateway {
  read(): SharedPage;
}
