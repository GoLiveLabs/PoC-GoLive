import { Injectable, signal } from '@angular/core';
import { environment } from '../../environments/environment';
import { Camera, ConnectionState, ErrorPayload, SystemStatus, WsEnvelope } from './models';

const INITIAL_RECONNECT_DELAY_MS = 1000;
const MAX_RECONNECT_DELAY_MS = 30000;

@Injectable({ providedIn: 'root' })
export class WebSocketService {
  readonly cameras = signal<Camera[]>([]);
  readonly systemStatus = signal<SystemStatus | null>(null);
  readonly connectionState = signal<ConnectionState>('connecting');
  readonly lastError = signal<string | null>(null);

  private socket: WebSocket | null = null;
  private reconnectDelay = INITIAL_RECONNECT_DELAY_MS;
  private reconnectTimer: ReturnType<typeof setTimeout> | undefined;
  private manuallyClosed = false;

  connect(): void {
    this.manuallyClosed = false;
    this.open();
  }

  disconnect(): void {
    this.manuallyClosed = true;
    clearTimeout(this.reconnectTimer);
    this.socket?.close();
  }

  private open(): void {
    this.connectionState.set('connecting');
    const url = `${environment.wsUrl}?api_token=${environment.apiToken}`;
    const socket = new WebSocket(url);
    this.socket = socket;

    socket.onopen = () => {
      this.connectionState.set('open');
      this.reconnectDelay = INITIAL_RECONNECT_DELAY_MS;
    };

    socket.onmessage = (ev: MessageEvent) => {
      this.handleMessage(ev.data);
    };

    socket.onclose = () => {
      this.connectionState.set('closed');
      if (!this.manuallyClosed) {
        this.scheduleReconnect();
      }
    };

    socket.onerror = () => {
      socket.close();
    };
  }

  private scheduleReconnect(): void {
    clearTimeout(this.reconnectTimer);
    this.reconnectTimer = setTimeout(() => this.open(), this.reconnectDelay);
    this.reconnectDelay = Math.min(this.reconnectDelay * 2, MAX_RECONNECT_DELAY_MS);
  }

  /** Exposed for tests: applies a raw WS message to the service's signals. */
  handleMessage(raw: string): void {
    let envelope: WsEnvelope;
    try {
      envelope = JSON.parse(raw);
    } catch {
      return;
    }

    switch (envelope.type) {
      case 'cameras.updated':
        this.cameras.set(envelope.payload as Camera[]);
        break;
      case 'system.status':
        this.systemStatus.set(envelope.payload as SystemStatus);
        break;
      case 'error':
        this.lastError.set((envelope.payload as ErrorPayload).message);
        break;
    }
  }
}
