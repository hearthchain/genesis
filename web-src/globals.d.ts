// Ambient types shared by every page script. This file emits nothing; the
// scripts stay plain global IIFEs communicating through window.HearthAPI.

interface ErrorEnvelope {
  error?: { code: string; message: string };
}

interface ApiResult<T = unknown> {
  status: number;
  body: T;
}

interface ChainStats {
  burnedBaseUnits: string;
  pendingBaseUnits: string;
  burnsByStatus: Record<string, number>;
}

interface StatsResponse extends ErrorEnvelope {
  totalCreditMicro: string;
  totalCredit: string;
  merkleRoot: string;
  participants: number;
  bindings: number;
  pendingSources: number;
  blockedSources: number;
  chains: Record<string, ChainStats> | null;
  windows: Record<string, { startHeight: number; endHeight: number }>;
}

interface PreviewLayer {
  amountBaseUnits: number;
  since: string;
  weekEnd: string;
  priceMicroUsd: number;
  creditMicro: string;
}

interface PreviewResponse extends ErrorEnvelope {
  address: string;
  status: string;
  layers: PreviewLayer[] | null;
  minimumCreditMicro: string;
  minimumCredit: string;
}

interface CabinetBurn {
  txId: string;
  chain: string;
  source: string;
  amountBaseUnits: number;
  height: number;
  status: string;
  creditMicro?: string;
}

interface AddressResponse extends ErrorEnvelope {
  hearthAddress: string;
  minimumCreditMicro: string;
  minimumCredit: string;
  bindings: string[];
  burns: CabinetBurn[] | null;
}

interface KeeperPublicState {
  account: { address: string } | null;
}

interface KeeperWalletApi {
  publicState(): Promise<KeeperPublicState>;
  signCustomData(data: { version: 1; binary: string }): Promise<{ publicKey: string; signature: string }>;
  signAndPublishTransaction(tx: { type: number; data: unknown }): Promise<string>;
}

interface HearthAPI {
  apiGet<T = unknown>(path: string): Promise<ApiResult<T> | null>;
  apiPost<T = unknown>(path: string, payload: unknown): Promise<ApiResult<T> | null>;
  fmtUnits(str: string, decimals: number): string;
  fmtWaves(wavelets: string): string;
  fmtCredit(decimalStr: string): string;
  fmtMicroUsd(micro: string): string;
  errorMessage(res: ApiResult<ErrorEnvelope> | null): string;
  degrade(): void;
}

interface Window {
  HEARTH_API_BASE?: string;
  HearthAPI: HearthAPI;
  KeeperWallet?: KeeperWalletApi;
}
