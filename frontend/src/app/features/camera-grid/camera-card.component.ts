import { Component, computed, input, output } from '@angular/core';
import { Camera, Position } from '../../core/models';

const NONE_OPTION = '';

@Component({
  selector: 'app-camera-card',
  standalone: true,
  templateUrl: './camera-card.component.html',
  styleUrl: './camera-card.component.css',
})
export class CameraCardComponent {
  camera = input.required<Camera>();
  positions = input.required<Position[]>();

  assign = output<{ positionId: string; cameraId: string }>();
  unassign = output<string>();
  preview = output<string>();

  readonly noneOption = NONE_OPTION;

  readonly currentPositionId = computed(() => {
    const camera = this.camera();
    return this.positions().find((position) => position.cameraId === camera.id)?.id ?? NONE_OPTION;
  });

  onAssignmentChange(positionId: string): void {
    if (this.camera().status === 'offline') {
      return;
    }
    const previousPositionId = this.currentPositionId();
    if (positionId === NONE_OPTION) {
      if (previousPositionId !== NONE_OPTION) {
        this.unassign.emit(previousPositionId);
      }
      return;
    }
    this.assign.emit({ positionId, cameraId: this.camera().id });
  }

  onPreview(): void {
    if (this.camera().status === 'offline') {
      return;
    }
    this.preview.emit(this.camera().id);
  }
}
