// Tipagens TypeScript espelhando os modelos Go do OpenIDM.

export type DownloadStatus =
  | "queued"
  | "probing"
  | "downloading"
  | "paused"
  | "assembling"
  | "completed"
  | "error"
  | "cancelled";

export type SegmentStatus = "pending" | "downloading" | "completed" | "error";

export interface Segment {
  id: number;
  downloadId: string;
  index: number;
  start: number;
  current: number;
  end: number;
  status: SegmentStatus;
  workerId: number;
  speedBps: number;
  reusedConn: boolean;
  lastError?: string;
}

export interface ConnectionEvent {
  timestamp: string;
  workerId: number;
  message: string;
  level: "info" | "warn" | "error" | "reuse";
}

export interface Download {
  id: string;
  url: string;
  fileName: string;
  saveDir: string;
  targetPath: string;
  description: string;
  categoryId: string;
  queueId: string;
  totalSize: number;
  downloadedSize: number;
  progress: number;
  speedBps: number;
  etaSeconds: number;
  status: DownloadStatus;
  acceptRanges: boolean;
  maxConnections: number;
  activeWorkers: number;
  degradedSingle: boolean;
  degradedReason?: string;
  retryCount: number;
  errorMessage?: string;
  etag?: string;
  lastModified?: string;
  speedLimitBps: number;
  openOnComplete: boolean;
  segments: Segment[];
  events?: ConnectionEvent[];
  createdAt: string;
  updatedAt: string;
  completedAt?: string;
}

export interface Category {
  id: string;
  name: string;
  slug: string;
  extensions: string[];
  defaultDir: string;
  icon: string;
}

export interface Queue {
  id: string;
  name: string;
  isMain: boolean;
  isSync: boolean;
  maxConcurrent: number;
  startTime: string;
  stopTime: string;
  trafficQuotaMB: number;
  usedTrafficBytes: number;
  retryIntervalSec: number;
  maxRetries: number;
  shutdownOnFinish: boolean;
  runCommandOnFinish: string;
  active: boolean;
}

export interface SiteRule {
  id: number;
  hostPattern: string;
  maxConnections: number;
  ignoreCapture: boolean;
  retryLimit: number;
  retryIntervalSec: number;
}

export interface SiteLogin {
  id: number;
  hostPattern: string;
  username: string;
  password: string;
  authType: "basic" | "bearer" | "ftp";
}

export type AppLanguage = "pt-BR" | "en-US";

export interface AppSettings {
  language?: AppLanguage | string;
  maxConnectionsPerDownload: number;
  minSplitSizeBytes: number;
  globalSpeedLimitBps: number;
  defaultSaveDir: string;
  startWithWindows?: boolean;
  monitorClipboard: boolean;
  captureExtensions: string[];
  ignoreModifierKey: string;
  proxyType: "none" | "system" | "http" | "socks5" | "pac";
  proxyHost: string;
  proxyPort: number;
  proxyUser: string;
  proxyPass: string;
  proxyPacUrl?: string;
  antivirusEnabled: boolean;
  antivirusPath: string;
  antivirusArgs: string;
  checkpointIntervalSec: number;
  cardPosX?: number;
  cardPosY?: number;
  cardPosSaved?: boolean;
}

export interface AddDownloadRequest {
  url: string;
  referrer?: string;
  fileName: string;
  saveDir: string;
  description: string;
  categoryId: string;
  queueId: string;
  maxConnections: number;
  startNow: boolean;
  username?: string;
  password?: string;
  speedLimitBps?: number;
  openOnComplete?: boolean;
}

export interface CapturePrompt {
  url: string;
  fileName: string;
  categoryId: string;
  categoryName: string;
  saveDir: string;
  targetPath: string;
  referrer?: string;
  source: "browser" | "clipboard" | "cli" | "manual";
  totalSize: number;
  acceptRanges: boolean;
  probed: boolean;
  probeError?: string;
}

export interface BrowserInfo {
  id: string;
  name: string;
  exePath: string;
}

export interface InstallOptions {
  installDir: string;
  sourceDir?: string;
  language?: AppLanguage | string;
  startWithWindows: boolean;
  startInTray: boolean;
  createDesktopShortcut: boolean;
  createStartMenu: boolean;
  registerUninstaller: boolean;
  installBrowserExt: boolean;
  launchBrowserExtSetup: boolean;
  launchAppAfterInstall: boolean;
  chromeExtId?: string;
  stopRunningInstances?: boolean;
}

export interface InstallResult {
  success: boolean;
  installDir: string;
  appExePath: string;
  nativeHostExePath: string;
  installerExePath: string;
  iconPath: string;
  extensionDir: string;
  extensionSetupHtml: string;
  chromeManifestPath: string;
  firefoxManifestPath: string;
  extensionIds: string[];
  autoStartEnabled: boolean;
  autoStartCommand: string;
  desktopShortcutPath?: string;
  startMenuShortcutDir?: string;
  uninstallerRegKey?: string;
  browsersDetected: BrowserInfo[];
  stepsCompleted: string[];
}

export interface InstallationStatus {
  installed: boolean;
  installDir: string;
  activeExePath: string;
  iconPath: string;
  extensionDir: string;
  extensionSetupHtml: string;
  extensionIds: string[];
  autoStartEnabled: boolean;
  autoStartCommand: string;
  desktopShortcutExist: boolean;
  startMenuExist: boolean;
  uninstallerExist: boolean;
  nativeHostRegistered: boolean;
  browsersDetected: BrowserInfo[];
}


