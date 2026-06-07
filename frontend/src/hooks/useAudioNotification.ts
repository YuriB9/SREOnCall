import { useCallback } from 'react'

export function useAudioNotification() {
  return useCallback(() => {
    if (document.hidden) return
    try {
      const ctx = new AudioContext()
      const osc = ctx.createOscillator()
      const gain = ctx.createGain()
      osc.connect(gain)
      gain.connect(ctx.destination)
      osc.frequency.value = 880
      gain.gain.setValueAtTime(0.3, ctx.currentTime)
      gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 0.4)
      osc.start(ctx.currentTime)
      osc.stop(ctx.currentTime + 0.4)
      osc.onended = () => void ctx.close()
    } catch {
      // unavailable in restricted/test environments
    }
  }, [])
}
