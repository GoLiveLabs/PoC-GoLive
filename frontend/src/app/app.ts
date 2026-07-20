import { Component, OnDestroy, OnInit, inject } from '@angular/core';
import { ApiService } from './core/api.service';
import { WebSocketService } from './core/websocket.service';
import { CameraGridComponent } from './features/camera-grid/camera-grid.component';
import { ControlBarComponent } from './features/control-bar/control-bar.component';
import { PositionAdminComponent } from './features/positions/position-admin.component';

@Component({
  selector: 'app-root',
  standalone: true,
  imports: [CameraGridComponent, ControlBarComponent, PositionAdminComponent],
  templateUrl: './app.html',
  styleUrl: './app.css',
})
export class App implements OnInit, OnDestroy {
  private readonly api = inject(ApiService);
  protected readonly ws = inject(WebSocketService);

  ngOnInit(): void {
    this.ws.connect();
  }

  ngOnDestroy(): void {
    this.ws.disconnect();
  }

  onSync(): void {
    this.api.sync().subscribe();
  }

  onCreatePosition(event: { name: string }): void {
    this.api.createPosition(event.name).subscribe({
      error: (err) => this.ws.lastError.set(err?.error?.message ?? 'Erro ao criar posição'),
    });
  }

  onRenamePosition(event: { id: string; name: string }): void {
    this.api.renamePosition(event.id, event.name).subscribe({
      error: (err) => this.ws.lastError.set(err?.error?.message ?? 'Erro ao renomear posição'),
    });
  }

  onDeletePosition(id: string): void {
    this.api.deletePosition(id).subscribe({
      error: (err) => this.ws.lastError.set(err?.error?.message ?? 'Erro ao excluir posição'),
    });
  }

  onAssignCamera(event: { positionId: string; cameraId: string }): void {
    this.api.assignCamera(event.positionId, event.cameraId).subscribe({
      error: (err) => this.ws.lastError.set(err?.error?.message ?? 'Erro ao atribuir câmera'),
    });
  }

  onUnassignCamera(positionId: string): void {
    this.api.unassignPosition(positionId).subscribe({
      error: (err) => this.lastError(err),
    });
  }

  onSetAudioSource(positionId: string): void {
    this.api.setAudioPosition(positionId).subscribe({
      error: (err) => this.ws.lastError.set(err?.error?.message ?? 'Erro ao definir fonte de áudio'),
    });
  }

  dismissError(): void {
    this.ws.lastError.set(null);
  }

  private lastError(err: unknown): void {
    this.ws.lastError.set((err as { error?: { message?: string } })?.error?.message ?? 'Erro ao remover câmera da posição');
  }
}
