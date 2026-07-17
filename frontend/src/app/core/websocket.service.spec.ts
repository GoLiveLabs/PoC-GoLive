import { WebSocketService } from './websocket.service';
import { Camera, SystemStatus } from './models';

describe('WebSocketService', () => {
  let service: WebSocketService;

  beforeEach(() => {
    service = new WebSocketService();
  });

  it('starts with empty cameras and null status', () => {
    expect(service.cameras()).toEqual([]);
    expect(service.systemStatus()).toBeNull();
    expect(service.lastError()).toBeNull();
  });

  it('applies a cameras.updated event to the cameras signal', () => {
    const cams: Camera[] = [
      {
        id: 'camera1',
        name: 'camera1',
        sourceUrl: 'rtmp://mediamtx:1935/camera1',
        status: 'online',
        obsSourceCreated: true,
        isLive: false,
        lastSeenAt: '2026-07-16T14:00:00Z',
      },
    ];
    service.handleMessage(JSON.stringify({ type: 'cameras.updated', payload: cams }));

    expect(service.cameras()).toEqual(cams);
  });

  it('applies a system.status event to the systemStatus signal', () => {
    const status: SystemStatus = {
      obsConnected: true,
      mediaServerConnected: true,
      streaming: true,
      activeSceneName: 'Program',
      liveCameraId: 'camera1',
    };
    service.handleMessage(JSON.stringify({ type: 'system.status', payload: status }));

    expect(service.systemStatus()).toEqual(status);
  });

  it('applies an error event to the lastError signal', () => {
    service.handleMessage(JSON.stringify({ type: 'error', payload: { message: 'algo deu errado' } }));

    expect(service.lastError()).toBe('algo deu errado');
  });

  it('ignores malformed messages without throwing', () => {
    expect(() => service.handleMessage('not json')).not.toThrow();
    expect(service.cameras()).toEqual([]);
  });
});
