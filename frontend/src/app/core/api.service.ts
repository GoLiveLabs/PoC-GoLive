import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';
import { environment } from '../../environments/environment';
import { Camera, SystemStatus } from './models';

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

  setLive(cameraId: string): Observable<SystemStatus> {
    return this.http.post<SystemStatus>(`${this.baseUrl}/cameras/${cameraId}/live`, {});
  }

  sync(): Observable<Camera[]> {
    return this.http.post<Camera[]>(`${this.baseUrl}/sync`, {});
  }
}
