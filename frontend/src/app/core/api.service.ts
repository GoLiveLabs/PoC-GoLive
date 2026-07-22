import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';
import { environment } from '../../environments/environment';
import {
  BroadcastStatus,
  Camera,
  Client,
  LiveKind,
  LiveState,
  Page,
  Position,
  Scene,
  SystemStatus,
} from './models';

@Injectable({ providedIn: 'root' })
export class ApiService {
  private readonly http = inject(HttpClient);
  private readonly baseUrl = environment.apiBaseUrl;

  getCameras(): Observable<Camera[]> {
    return this.http.get<Camera[]>(`${this.baseUrl}/cameras`);
  }

  getStatus(): Observable<SystemStatus> {
    return this.http.get<SystemStatus>(`${this.baseUrl}/status`);
  }

  sync(): Observable<Camera[]> {
    return this.http.post<Camera[]>(`${this.baseUrl}/sync`, {});
  }

  getPositions(): Observable<Position[]> {
    return this.http.get<Position[]>(`${this.baseUrl}/positions`);
  }

  createPosition(name: string): Observable<Position> {
    return this.http.post<Position>(`${this.baseUrl}/positions`, { name });
  }

  renamePosition(id: string, name: string): Observable<Position> {
    return this.http.patch<Position>(`${this.baseUrl}/positions/${id}`, { name });
  }

  deletePosition(id: string): Observable<void> {
    return this.http.delete<void>(`${this.baseUrl}/positions/${id}`);
  }

  assignCamera(positionId: string, cameraId: string): Observable<Position> {
    return this.http.post<Position>(`${this.baseUrl}/positions/${positionId}/camera`, { cameraId });
  }

  unassignPosition(positionId: string): Observable<Position> {
    return this.http.delete<Position>(`${this.baseUrl}/positions/${positionId}/camera`);
  }

  setAudioPosition(positionId: string): Observable<Position> {
    return this.http.post<Position>(`${this.baseUrl}/positions/${positionId}/audio`, {});
  }

  listScenes(): Observable<Scene[]> {
    return this.http.get<Scene[]>(`${this.baseUrl}/scenes`);
  }

  createScene(name: string, positionIds: string[]): Observable<Scene> {
    return this.http.post<Scene>(`${this.baseUrl}/scenes`, { name, positionIds });
  }

  updateScene(id: string, changes: { name?: string; positionIds?: string[] }): Observable<Scene> {
    return this.http.patch<Scene>(`${this.baseUrl}/scenes/${id}`, changes);
  }

  deleteScene(id: string): Observable<void> {
    return this.http.delete<void>(`${this.baseUrl}/scenes/${id}`);
  }

  getLive(): Observable<LiveState> {
    return this.http.get<LiveState>(`${this.baseUrl}/live`);
  }

  setPreview(kind: LiveKind, id: string): Observable<LiveState> {
    return this.http.post<LiveState>(`${this.baseUrl}/live/preview`, { kind, id });
  }

  cut(): Observable<LiveState> {
    return this.http.post<LiveState>(`${this.baseUrl}/live/cut`, {});
  }

  getBroadcast(): Observable<BroadcastStatus> {
    return this.http.get<BroadcastStatus>(`${this.baseUrl}/broadcast`);
  }

  setActiveClient(clientId: string): Observable<BroadcastStatus> {
    return this.http.post<BroadcastStatus>(`${this.baseUrl}/broadcast/client`, { clientId });
  }

  startBroadcast(): Observable<BroadcastStatus> {
    return this.http.post<BroadcastStatus>(`${this.baseUrl}/broadcast/start`, {});
  }

  stopBroadcast(): Observable<BroadcastStatus> {
    return this.http.post<BroadcastStatus>(`${this.baseUrl}/broadcast/stop`, {});
  }

  restartDestination(liveId: string): Observable<BroadcastStatus> {
    return this.http.post<BroadcastStatus>(
      `${this.baseUrl}/broadcast/destinations/${liveId}/restart`,
      {},
    );
  }

  listClients(): Observable<Page<Client>> {
    return this.http.get<Page<Client>>(`${this.baseUrl}/clients`);
  }
}
