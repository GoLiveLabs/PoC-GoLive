import { Component, input, output, computed } from '@angular/core';
import { ConnectionState, Position } from '../../core/models';

@Component({
  selector: 'app-control-bar',
  standalone: true,
  templateUrl: './control-bar.component.html',
  styleUrl: './control-bar.component.css',
})
export class ControlBarComponent {
  positions = input.required<Position[]>();
  connectionState = input<ConnectionState>('connecting');
  sync = output<void>();

  readonly audioPosition = computed(() => this.positions().find((p) => p.isAudioSource) ?? null);

  onSyncClick(): void {
    this.sync.emit();
  }
}
