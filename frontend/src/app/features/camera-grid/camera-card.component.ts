import { Component, input, output } from '@angular/core';
import { Camera } from '../../core/models';

@Component({
  selector: 'app-camera-card',
  standalone: true,
  templateUrl: './camera-card.component.html',
  styleUrl: './camera-card.component.css',
})
export class CameraCardComponent {
  camera = input.required<Camera>();
  goLive = output<string>();

  onGoLiveClick(): void {
    this.goLive.emit(this.camera().id);
  }
}
