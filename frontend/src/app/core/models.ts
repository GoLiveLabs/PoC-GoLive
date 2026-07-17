export type CameraStatus = 'online' | 'offline';

export interface Camera {
  id: string;
  name: string;
  sourceUrl: string;
  status: CameraStatus;
  obsSourceCreated: boolean;
  isLive: boolean;
  lastSeenAt: string;
}

export interface SystemStatus {
  obsConnected: boolean;
  mediaServerConnected: boolean;
  streaming: boolean;
  activeSceneName: string;
  liveCameraId: string;
}

export type WsEventType = 'cameras.updated' | 'system.status' | 'error';

export interface WsEnvelope<T = unknown> {
  type: WsEventType;
  payload: T;
}

export interface ErrorPayload {
  message: string;
}

export type ConnectionState = 'connecting' | 'open' | 'closed';
