import { Component, OnDestroy, OnInit, inject } from '@angular/core';
import { ApiService } from './core/api.service';
import { WebSocketService } from './core/websocket.service';
import { CameraGridComponent } from './features/camera-grid/camera-grid.component';
import { ControlBarComponent } from './features/control-bar/control-bar.component';

@Component({
  selector: 'app-root',
  standalone: true,
  imports: [CameraGridComponent, ControlBarComponent],
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

  onGoLive(cameraId: string): void {
    this.api.setLive(cameraId).subscribe();
  }

  onSync(): void {
    this.api.sync().subscribe();
  }

  dismissError(): void {
    this.ws.lastError.set(null);
  }
}
