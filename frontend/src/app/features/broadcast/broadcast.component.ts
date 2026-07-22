import { Component, computed, input, output } from '@angular/core';
import { BroadcastStatus, Client } from '../../core/models';

@Component({
  selector: 'app-broadcast',
  standalone: true,
  templateUrl: './broadcast.component.html',
  styleUrl: './broadcast.component.css',
})
export class BroadcastComponent {
  clients = input.required<Client[]>();
  status = input<BroadcastStatus | null>(null);
  onAir = input<boolean>(false);

  setActiveClient = output<string>();
  start = output<void>();
  stop = output<void>();
  restart = output<string>();

  readonly activeClientId = computed(() => this.status()?.activeClientId ?? '');
  readonly running = computed(() => this.status()?.running ?? false);
  readonly destinations = computed(() => this.status()?.destinations ?? []);

  readonly canStart = computed(() => !!this.activeClientId() && this.onAir() && !this.running());

  onClientChange(clientId: string): void {
    if (!clientId) {
      return;
    }
    this.setActiveClient.emit(clientId);
  }

  onStart(): void {
    if (!this.canStart()) {
      return;
    }
    this.start.emit();
  }

  onStop(): void {
    this.stop.emit();
  }

  onRestart(liveId: string): void {
    this.restart.emit(liveId);
  }
}
