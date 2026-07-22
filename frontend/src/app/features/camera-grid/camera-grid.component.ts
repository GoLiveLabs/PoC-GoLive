import { Component, input, output } from '@angular/core';
import { Camera, Position } from '../../core/models';
import { CameraCardComponent } from './camera-card.component';

@Component({
  selector: 'app-camera-grid',
  standalone: true,
  imports: [CameraCardComponent],
  templateUrl: './camera-grid.component.html',
  styleUrl: './camera-grid.component.css',
})
export class CameraGridComponent {
  cameras = input.required<Camera[]>();
  positions = input.required<Position[]>();

  assign = output<{ positionId: string; cameraId: string }>();
  unassign = output<string>();
  preview = output<string>();
}
