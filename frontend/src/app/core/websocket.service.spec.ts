import { WebSocketService } from './websocket.service';
import { BroadcastStatus, Camera, LiveState, Position, Scene, SystemStatus } from './models';

describe('WebSocketService', () => {
  let service: WebSocketService;

  beforeEach(() => {
    service = new WebSocketService();
  });

  it('starts with empty cameras and null status', () => {
    expect(service.cameras()).toEqual([]);
    expect(service.positions()).toEqual([]);
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
        lastSeenAt: '2026-07-16T14:00:00Z',
      },
    ];
    service.handleMessage(JSON.stringify({ type: 'cameras.updated', payload: cams }));

    expect(service.cameras()).toEqual(cams);
  });

  it('applies a positions.updated event to the positions signal', () => {
    const positions: Position[] = [
      { id: 'position1', name: 'Posição 1', cameraId: 'camera1', isAudioSource: true },
    ];
    service.handleMessage(JSON.stringify({ type: 'positions.updated', payload: positions }));

    expect(service.positions()).toEqual(positions);
  });

  it('starts with empty scenes and null live/broadcast state', () => {
    expect(service.scenes()).toEqual([]);
    expect(service.liveState()).toBeNull();
    expect(service.broadcastStatus()).toBeNull();
  });

  it('applies a scenes.updated event to the scenes signal', () => {
    const scenes: Scene[] = [{ id: 'scene1', name: 'Cena 1', positionIds: ['pos1', 'pos2'] }];
    service.handleMessage(JSON.stringify({ type: 'scenes.updated', payload: scenes }));

    expect(service.scenes()).toEqual(scenes);
  });

  it('applies a live.updated event to the liveState signal', () => {
    const live: LiveState = {
      previewKind: 'camera',
      previewId: 'camera1',
      liveKind: 'scene',
      liveId: 'scene1',
    };
    service.handleMessage(JSON.stringify({ type: 'live.updated', payload: live }));

    expect(service.liveState()).toEqual(live);
  });

  it('applies a broadcast.status event to the broadcastStatus signal', () => {
    const status: BroadcastStatus = {
      activeClientId: 'client1',
      running: true,
      destinations: [
        { liveId: 'live1', platformName: 'YouTube', state: 'connected', lastError: '' },
      ],
    };
    service.handleMessage(JSON.stringify({ type: 'broadcast.status', payload: status }));

    expect(service.broadcastStatus()).toEqual(status);
  });

  it('applies a system.status event to the systemStatus signal', () => {
    const status: SystemStatus = {
      obsConnected: true,
      mediaServerConnected: true,
      streaming: true,
      activeSceneName: 'Program',
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
