import { Component, input, output } from '@angular/core';
import { ConnectionState, SystemStatus } from '../../core/models';

@Component({
  selector: 'app-control-bar',
  standalone: true,
  templateUrl: './control-bar.component.html',
  styleUrl: './control-bar.component.css',
})
export class ControlBarComponent {
  systemStatus = input<SystemStatus | null>(null);
  connectionState = input<ConnectionState>('connecting');
  sync = output<void>();

  onSyncClick(): void {
    this.sync.emit();
  }
}
