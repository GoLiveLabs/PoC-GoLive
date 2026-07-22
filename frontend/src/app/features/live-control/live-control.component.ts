import { Component, computed, input, output } from '@angular/core';
import { Camera, LiveKind, LiveState, Scene } from '../../core/models';

@Component({
  selector: 'app-live-control',
  standalone: true,
  templateUrl: './live-control.component.html',
  styleUrl: './live-control.component.css',
})
export class LiveControlComponent {
  cameras = input.required<Camera[]>();
  scenes = input.required<Scene[]>();
  liveState = input<LiveState | null>(null);

  previewSelect = output<{ kind: LiveKind; id: string }>();
  cut = output<void>();

  readonly hasPreview = computed(() => {
    const state = this.liveState();
    return !!state && state.previewKind !== '';
  });

  readonly onAirLabel = computed(() => this.describe(this.liveState()?.liveKind, this.liveState()?.liveId));
  readonly previewLabel = computed(() =>
    this.describe(this.liveState()?.previewKind, this.liveState()?.previewId),
  );

  isLive(kind: LiveKind, id: string): boolean {
    const state = this.liveState();
    return !!state && state.liveKind === kind && state.liveId === id;
  }

  isPreview(kind: LiveKind, id: string): boolean {
    const state = this.liveState();
    return !!state && state.previewKind === kind && state.previewId === id;
  }

  selectCameraPreview(cameraId: string): void {
    this.previewSelect.emit({ kind: 'camera', id: cameraId });
  }

  selectScenePreview(sceneId: string): void {
    this.previewSelect.emit({ kind: 'scene', id: sceneId });
  }

  onCut(): void {
    if (!this.hasPreview()) {
      return;
    }
    this.cut.emit();
  }

  private describe(kind: LiveKind | undefined, id: string | undefined): string {
    if (!kind || !id) {
      return 'nada';
    }
    if (kind === 'camera') {
      return this.cameras().find((c) => c.id === id)?.name ?? id;
    }
    return this.scenes().find((s) => s.id === id)?.name ?? id;
  }
}
