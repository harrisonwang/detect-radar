
export interface DeviceSignals {
  maxTouchPoints: number;
  orientation: 'portrait' | 'landscape';
  gyroscopeSupported: boolean;
  screenSize: string;
  battery: { supported: boolean; level?: number; charging?: boolean };
}

async function readBattery(): Promise<DeviceSignals['battery']> {
  try {
    const anyNav = navigator as unknown as {
      getBattery?: () => Promise<{ level: number; charging: boolean }>;
    };
    if (typeof anyNav.getBattery !== 'function') {
      return { supported: false };
    }
    const b = await anyNav.getBattery();
    return { supported: true, level: Math.round(b.level * 100), charging: b.charging };
  } catch {
    return { supported: false };
  }
}

export async function collectDeviceSignals(): Promise<DeviceSignals> {
  if (typeof window === 'undefined' || typeof screen === 'undefined') {
    return {
      maxTouchPoints: 0,
      orientation: 'landscape',
      gyroscopeSupported: false,
      screenSize: '0 x 0',
      battery: { supported: false },
    };
  }

  const w = screen.width;
  const h = screen.height;
  const orientationType = screen.orientation?.type ?? '';
  const orientation: DeviceSignals['orientation'] = orientationType
    ? orientationType.startsWith('portrait')
      ? 'portrait'
      : 'landscape'
    : h >= w
      ? 'portrait'
      : 'landscape';

  return {
    maxTouchPoints: navigator.maxTouchPoints || 0,
    orientation,
    gyroscopeSupported: typeof DeviceOrientationEvent !== 'undefined',
    screenSize: `${w} x ${h}`,
    battery: await readBattery(),
  };
}
