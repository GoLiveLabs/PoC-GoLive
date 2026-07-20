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

export type WsEventType = 'cameras.updated' | 'system.status' | 'positions.updated' | 'error';

export interface WsEnvelope<T = unknown> {
  type: WsEventType;
  payload: T;
}

export interface ErrorPayload {
  message: string;
}

export type ConnectionState = 'connecting' | 'open' | 'closed';
