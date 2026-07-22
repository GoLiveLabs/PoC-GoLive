export type CameraStatus = 'online' | 'offline';

export interface Camera {
  id: string;
  name: string;
  sourceUrl: string;
  status: CameraStatus;
  lastSeenAt: string;
}

export interface SystemStatus {
  obsConnected: boolean;
  mediaServerConnected: boolean;
  streaming: boolean;
  activeSceneName: string;
}

export interface Position {
  id: string;
  name: string;
  cameraId: string;
  isAudioSource: boolean;
}

export interface Scene {
  id: string;
  name: string;
  positionIds: string[];
}

export type LiveKind = '' | 'camera' | 'scene';

export interface LiveState {
  previewKind: LiveKind;
  previewId: string;
  liveKind: LiveKind;
  liveId: string;
}

export type DestinationState = 'connected' | 'failed' | 'stopped';

export interface DestinationStatus {
  liveId: string;
  platformName: string;
  state: DestinationState;
  lastError: string;
}

export interface BroadcastStatus {
  activeClientId: string | null;
  running: boolean;
  destinations: DestinationStatus[];
}

export interface Client {
  id: string;
  name: string;
  email?: string;
  createdAt: string;
  updatedAt: string;
}

export interface Page<T> {
  data: T[];
  nextCursor: string | null;
  hasMore: boolean;
}

export type WsEventType =
  | 'cameras.updated'
  | 'system.status'
  | 'positions.updated'
  | 'scenes.updated'
  | 'live.updated'
  | 'broadcast.status'
  | 'error';

export interface WsEnvelope<T = unknown> {
  type: WsEventType;
  payload: T;
}

export interface ErrorPayload {
  message: string;
}

export type ConnectionState = 'connecting' | 'open' | 'closed';
