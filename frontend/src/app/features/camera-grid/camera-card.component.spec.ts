import { ComponentFixture, TestBed } from '@angular/core/testing';
import { CameraCardComponent } from './camera-card.component';
import { Camera } from '../../core/models';

function makeCamera(overrides: Partial<Camera> = {}): Camera {
  return {
    id: 'camera1',
    name: 'camera1',
    sourceUrl: 'rtmp://mediamtx:1935/camera1',
    status: 'online',
    obsSourceCreated: true,
    isLive: false,
    lastSeenAt: '2026-07-16T14:00:00Z',
    ...overrides,
  };
}

describe('CameraCardComponent', () => {
  let fixture: ComponentFixture<CameraCardComponent>;

  beforeEach(() => {
    TestBed.configureTestingModule({ imports: [CameraCardComponent] });
    fixture = TestBed.createComponent(CameraCardComponent);
  });

  it('renders the online badge and an enabled button when the camera is online', () => {
    fixture.componentRef.setInput('camera', makeCamera({ status: 'online' }));
    fixture.detectChanges();

    const el: HTMLElement = fixture.nativeElement;
    expect(el.textContent).toContain('Online');
    const button = el.querySelector('button') as HTMLButtonElement;
    expect(button.disabled).toBe(false);
  });

  it('renders the offline badge and disables the button when the camera is offline', () => {
    fixture.componentRef.setInput('camera', makeCamera({ status: 'offline' }));
    fixture.detectChanges();

    const el: HTMLElement = fixture.nativeElement;
    expect(el.textContent).toContain('Offline');
    const button = el.querySelector('button') as HTMLButtonElement;
    expect(button.disabled).toBe(true);
  });

  it('shows a live badge and disables the button when the camera is already live', () => {
    fixture.componentRef.setInput('camera', makeCamera({ isLive: true }));
    fixture.detectChanges();

    const el: HTMLElement = fixture.nativeElement;
    expect(el.textContent).toContain('NO AR');
    const button = el.querySelector('button') as HTMLButtonElement;
    expect(button.disabled).toBe(true);
  });

  it('emits goLive with the camera id when the button is clicked', () => {
    fixture.componentRef.setInput('camera', makeCamera({ id: 'camera2', status: 'online' }));
    fixture.detectChanges();

    let emitted: string | undefined;
    fixture.componentInstance.goLive.subscribe((id: string) => (emitted = id));

    const button = fixture.nativeElement.querySelector('button') as HTMLButtonElement;
    button.click();

    expect(emitted).toBe('camera2');
  });
});
