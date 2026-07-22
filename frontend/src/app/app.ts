import { Component, OnDestroy, OnInit, inject, signal } from '@angular/core';
import { ApiService } from './core/api.service';
import { Client, LiveKind } from './core/models';
import { WebSocketService } from './core/websocket.service';
import { BroadcastComponent } from './features/broadcast/broadcast.component';
import { CameraGridComponent } from './features/camera-grid/camera-grid.component';
import { ControlBarComponent } from './features/control-bar/control-bar.component';
import { LiveControlComponent } from './features/live-control/live-control.component';
import { PositionAdminComponent } from './features/positions/position-admin.component';
import { SceneAdminComponent } from './features/scenes/scene-admin.component';

@Component({
  selector: 'app-root',
  standalone: true,
  imports: [
    CameraGridComponent,
    ControlBarComponent,
    PositionAdminComponent,
    SceneAdminComponent,
    LiveControlComponent,
    BroadcastComponent,
  ],
  templateUrl: './app.html',
  styleUrl: './app.css',
})
export class App implements OnInit, OnDestroy {
  private readonly api = inject(ApiService);
  protected readonly ws = inject(WebSocketService);

  protected readonly clients = signal<Client[]>([]);

  ngOnInit(): void {
    this.ws.connect();
    this.loadClients();
  }

  private loadClients(): void {
    this.api.listClients().subscribe({
      next: (page) => this.clients.set(page.data),
      error: (err) => this.ws.lastError.set(err?.error?.message ?? 'Erro ao carregar clientes'),
    });
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

  onCreateScene(event: { name: string; positionIds: string[] }): void {
    this.api.createScene(event.name, event.positionIds).subscribe({
      error: (err) => this.ws.lastError.set(err?.error?.message ?? 'Erro ao criar cena'),
    });
  }

  onRenameScene(event: { id: string; name: string }): void {
    this.api.updateScene(event.id, { name: event.name }).subscribe({
      error: (err) => this.ws.lastError.set(err?.error?.message ?? 'Erro ao renomear cena'),
    });
  }

  onUpdateScenePositions(event: { id: string; positionIds: string[] }): void {
    this.api.updateScene(event.id, { positionIds: event.positionIds }).subscribe({
      error: (err) => this.ws.lastError.set(err?.error?.message ?? 'Erro ao atualizar posições da cena'),
    });
  }

  onDeleteScene(id: string): void {
    this.api.deleteScene(id).subscribe({
      error: (err) => this.ws.lastError.set(err?.error?.message ?? 'Erro ao excluir cena'),
    });
  }

  onPreviewSelect(event: { kind: LiveKind; id: string }): void {
    this.api.setPreview(event.kind, event.id).subscribe({
      error: (err) => this.ws.lastError.set(err?.error?.message ?? 'Erro ao colocar em prévia'),
    });
  }

  onPreviewCamera(cameraId: string): void {
    this.onPreviewSelect({ kind: 'camera', id: cameraId });
  }

  onCut(): void {
    this.api.cut().subscribe({
      error: (err) => this.ws.lastError.set(err?.error?.message ?? 'Erro ao cortar'),
    });
  }

  onSetActiveClient(clientId: string): void {
    this.api.setActiveClient(clientId).subscribe({
      error: (err) => this.ws.lastError.set(err?.error?.message ?? 'Erro ao selecionar cliente'),
    });
  }

  onStartBroadcast(): void {
    this.api.startBroadcast().subscribe({
      error: (err) => this.ws.lastError.set(err?.error?.message ?? 'Erro ao iniciar transmissão'),
    });
  }

  onStopBroadcast(): void {
    this.api.stopBroadcast().subscribe({
      error: (err) => this.ws.lastError.set(err?.error?.message ?? 'Erro ao parar transmissão'),
    });
  }

  onRestartDestination(liveId: string): void {
    this.api.restartDestination(liveId).subscribe({
      error: (err) => this.ws.lastError.set(err?.error?.message ?? 'Erro ao reiniciar destino'),
    });
  }

  dismissError(): void {
    this.ws.lastError.set(null);
  }

  private lastError(err: unknown): void {
    this.ws.lastError.set((err as { error?: { message?: string } })?.error?.message ?? 'Erro ao remover câmera da posição');
  }
}
