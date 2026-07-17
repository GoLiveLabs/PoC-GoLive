import { Component, input, output } from '@angular/core';
import { Camera } from '../../core/models';
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
  goLive = output<string>();
}
