import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';
import { environment } from '../../environments/environment';
import { Camera, Position, SystemStatus } from './models';

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
}
